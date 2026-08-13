package community

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The hour is only safe because a server that has never listed is retried on
// its own trigger. This is that trigger. See sirens-echo#163.
func TestAServerThatNeverListedRetriesRegardlessOfTheInterval(t *testing.T) {
	t.Parallel()
	entry := &supervisedServer{}
	if !entry.needsTools(time.Hour, time.Now()) {
		t.Fatal("a server with no tools waited out the interval, so a failed " +
			"listing would persist for the life of the pod")
	}
	// A server that did list is governed by the interval, which is the whole
	// point of raising it.
	entry.tools = []*mcp.Tool{}
	entry.refreshed = time.Now()
	if entry.needsTools(time.Hour, time.Now()) {
		t.Error("a freshly listed server re-listed inside the interval")
	}
}

func countingRosterServer(t *testing.T, lists *atomic.Int32) string {
	t.Helper()
	return countingRosterServerNamed(t, lists, "status")
}

func countingRosterServerNamed(t *testing.T, lists *atomic.Int32, tool string) string {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "roster-test", Version: "1"}, nil)
	mcp.AddTool(
		server,
		&mcp.Tool{Name: tool, Description: "fixture tool"},
		func(context.Context, *mcp.CallToolRequest, struct{}) (
			*mcp.CallToolResult, any, error,
		) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "ok"}},
			}, nil, nil
		},
	)
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	httpServer := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method == http.MethodPost {
			body, _ := io.ReadAll(request.Body)
			request.Body = io.NopCloser(bytes.NewReader(body))
			if bytes.Contains(body, []byte(`"method":"tools/list"`)) {
				lists.Add(1)
			}
		}
		handler.ServeHTTP(writer, request)
	}))
	t.Cleanup(httpServer.Close)
	return httpServer.URL
}

// An hour of staleness is only tolerable if something can end it early, and the
// agent is what notices a tool it expected is missing.
func TestRefreshRelistsWithoutWaitingOutTheInterval(t *testing.T) {
	t.Parallel()
	var lists atomic.Int32
	url := countingRosterServer(t, &lists)

	provider := &MCPProvider{
		Servers:         []MCPServerDefinition{{Name: "roster", URL: url}},
		RefreshInterval: time.Hour,
	}
	t.Cleanup(func() { _ = provider.Close() })

	openTurn := func() {
		t.Helper()
		session, err := provider.Open(context.Background())
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if err := session.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	openTurn()
	openTurn()
	if got := lists.Load(); got != 1 {
		t.Fatalf("listings across two turns inside the interval = %d, want 1", got)
	}

	if marked := provider.Refresh(); marked != 1 {
		t.Fatalf("Refresh marked %d servers, want 1", marked)
	}
	openTurn()
	if got := lists.Load(); got != 2 {
		t.Errorf("listings after a refresh = %d, want 2", got)
	}
}

// Refresh dials nothing itself, which is why it needs no rate limit of its own:
// repeated calls collapse into the next turn's single listing.
func TestRefreshIsIdempotentUntilTheNextTurn(t *testing.T) {
	t.Parallel()
	var lists atomic.Int32
	url := countingRosterServer(t, &lists)

	provider := &MCPProvider{
		Servers:         []MCPServerDefinition{{Name: "roster", URL: url}},
		RefreshInterval: time.Hour,
	}
	t.Cleanup(func() { _ = provider.Close() })

	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for call := 0; call < 20; call++ {
		provider.Refresh()
	}
	if got := lists.Load(); got != 1 {
		t.Fatalf("twenty refreshes listed %d times before any turn ran, want 1", got)
	}

	session, err = provider.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := lists.Load(); got != 2 {
		t.Errorf("listings after twenty refreshes and one turn = %d, want 2", got)
	}
}

// The tool exists so the model can end the wait, and its result must not imply
// the wait already ended. See sirens-echo#163 and sirens-echo#211.
func TestTheRefreshToolMarksTheRosterAndSaysWhatItDidNotDo(t *testing.T) {
	t.Parallel()
	var lists atomic.Int32
	url := countingRosterServer(t, &lists)

	provider := &MCPProvider{
		Servers:         []MCPServerDefinition{{Name: "roster", URL: url}},
		RefreshInterval: time.Hour,
	}
	t.Cleanup(func() { _ = provider.Close() })

	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	result, err := session.Call(context.Background(), refreshToolProxyName(), nil)
	if err != nil {
		t.Fatalf("Call refresh: %v", err)
	}
	if result.IsError {
		t.Fatalf("refresh reported an error: %q", result.Text)
	}
	if !strings.Contains(result.Text, "still sees the tool list it started with") {
		t.Errorf("result = %q, and a model reading it could claim the list changed", result.Text)
	}
	// The calling turn keeps the list it opened with, which is what the result says.
	if got := lists.Load(); got != 1 {
		t.Errorf("listings during the calling turn = %d, want 1", got)
	}
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err = provider.Open(context.Background()); err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if got := lists.Load(); got != 2 {
		t.Errorf("listings on the turn after the refresh = %d, want 2", got)
	}
}

// An empty roster is a no-tool capability boundary, and a refresh for nothing
// would be a capability claim with no capability behind it.
func TestAnEmptyRosterIsNotOfferedARefresh(t *testing.T) {
	t.Parallel()
	session, err := (&MCPProvider{}).Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if tools := session.Tools(); len(tools) != 0 {
		t.Fatalf("tools = %#v, want none", tools)
	}
	if _, err := session.Call(context.Background(), refreshToolProxyName(), nil); err == nil {
		t.Error("an empty roster served a refresh it never offered")
	}
}

// A server named like the harness would take the refresh's name. Fatal for the
// same reason a roster collision is: one of the two would silently vanish.
func TestAServerCannotTakeTheRefreshToolsName(t *testing.T) {
	t.Parallel()
	var lists atomic.Int32
	url := countingRosterServerNamed(t, &lists, refreshToolTool)

	provider := &MCPProvider{
		Servers: []MCPServerDefinition{{Name: refreshToolServer, URL: url}},
	}
	t.Cleanup(func() { _ = provider.Close() })

	_, err := provider.Open(context.Background())
	if err == nil {
		t.Fatal("a colliding server name opened cleanly, so one tool was dropped in silence")
	}
	if !strings.Contains(err.Error(), "collision") {
		t.Errorf("error = %v, want it to name the collision", err)
	}
}
