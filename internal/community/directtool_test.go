package community

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func statusTemplates() []replyTemplate {
	return []replyTemplate{
		{Text: "{{players.online}} players online on day {{cycle.daysRunning}}. The meteor hits in {{cycle.daysUntilMeteor}} days."},
		{Text: "{{players.online}} players online on day {{cycle.daysRunning}}."},
	}
}

func TestRenderReplyTemplateFallsToTheNextEntryOnAMissingValue(t *testing.T) {
	payload := map[string]any{
		"players": map[string]any{"online": float64(0)},
		"cycle":   map[string]any{"daysRunning": float64(12), "daysUntilMeteor": nil},
	}

	got, ok := renderReplyTemplate(statusTemplates(), payload, nil)

	if !ok || got != "0 players online on day 12." {
		t.Fatalf("render = %q, %v, want the no-meteor entry with zero rendered as a value", got, ok)
	}
}

func TestRenderReplyTemplateMatchesTheReferenceNumberFormat(t *testing.T) {
	templates := []replyTemplate{{Text: "{{a}} {{b}} {{c}} {{d}}"}}
	payload := map[string]any{"a": 3.2, "b": 3.0, "c": 2.256, "d": -0.001}

	got, ok := renderReplyTemplate(templates, payload, nil)

	if !ok || got != "3.2 3 2.26 0" {
		t.Fatalf("render = %q, %v, want at most two decimals with trailing zeros trimmed", got, ok)
	}
}

func TestRenderReplyTemplateGatesOnArgsAndReadsThem(t *testing.T) {
	templates := []replyTemplate{{WhenArgs: []string{"item"}, Text: "Cheapest {{args.item}}: {{cheapest.0.price}}."}}
	payload := map[string]any{"cheapest": []any{map[string]any{"price": float64(3)}}}

	if _, ok := renderReplyTemplate(templates, payload, map[string]any{}); ok {
		t.Fatal("rendered without the item argument the template requires")
	}
	got, ok := renderReplyTemplate(templates, payload, map[string]any{"item": "limestone"})
	if !ok || got != "Cheapest limestone: 3." {
		t.Fatalf("render = %q, %v", got, ok)
	}
}

func TestRenderReplyTemplateRefusesBooleansObjectsAndOverlongReplies(t *testing.T) {
	cases := map[string]any{
		"boolean": map[string]any{"v": true},
		"object":  map[string]any{"v": map[string]any{"x": 1.0}},
		"long":    map[string]any{"v": strings.Repeat("x", maxTemplateReplyRunes+1)},
	}
	for name, payload := range cases {
		if got, ok := renderReplyTemplate([]replyTemplate{{Text: "{{v}}"}}, payload, nil); ok {
			t.Errorf("%s: rendered %q, want ineligible", name, got)
		}
	}
}

func TestToolReplyTemplatesSkipsMalformedEntries(t *testing.T) {
	tool := &mcp.Tool{Name: "get_server_status"}
	tool.Meta = mcp.Meta{replyTemplatesMetaKey: []any{
		"not an object",
		map[string]any{"text": "   "},
		map[string]any{"text": "{{players.online}} online.", "when_args": []any{"server", 7}},
	}}

	templates := toolReplyTemplates(tool)

	if len(templates) != 1 || templates[0].Text != "{{players.online}} online." {
		t.Fatalf("templates = %+v, want only the well-formed entry", templates)
	}
	if len(templates[0].WhenArgs) != 1 || templates[0].WhenArgs[0] != "server" {
		t.Errorf("when_args = %v, want the string names only", templates[0].WhenArgs)
	}
}

// statusServer serves one tool whose _meta carries templates and whose result
// carries structuredContent, the shape eco-app publishes.
func statusServer(t *testing.T, templates []any) *MCPProvider {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "eco-test", Version: "1"}, nil)
	tool := &mcp.Tool{Name: "get_server_status", Description: "status", InputSchema: map[string]any{"type": "object"}}
	tool.Meta = mcp.Meta{replyTemplatesMetaKey: templates}
	server.AddTool(tool, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "summary"}},
			StructuredContent: map[string]any{
				"players": map[string]any{"online": 7},
				"cycle":   map[string]any{"daysRunning": 12},
			},
		}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	))
	t.Cleanup(httpServer.Close)
	provider := &MCPProvider{Servers: []MCPServerDefinition{{Name: "eco", URL: httpServer.URL}}}
	t.Cleanup(func() { _ = provider.Close() })
	// One open lists the tools, which is what fills the cache route.jev reads.
	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = session.Close()
	return provider
}

func confidentStatusPick() RouteDecision {
	return RouteDecision{Ran: true, ToolServer: "eco", Tool: "get_server_status", ToolProb: 0.95}
}

func TestDirectToolReplyAnswersFromTheTemplateWhenEnabled(t *testing.T) {
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = statusServer(t, []any{map[string]any{"text": "{{players.online}} players online on day {{cycle.daysRunning}}."}})

	got, ok := directReplyText(agent, confidentStatusPick(), "is the server up")

	if !ok || got != "7 players online on day 12." {
		t.Fatalf("directToolReply = %q, %v, want the rendered template", got, ok)
	}
}

func TestDirectToolReplyDeclinesWhenOffUnconfidentOrArgumentBound(t *testing.T) {
	plain := []any{map[string]any{"text": "{{players.online}} online."}}
	argBound := []any{map[string]any{"text": "{{players.online}} online.", "when_args": []any{"server"}}}
	unconfident := confidentStatusPick()
	unconfident.ToolProb = 0.8

	cases := []struct {
		name      string
		enabled   bool
		templates []any
		route     RouteDecision
	}{
		{"flag off", false, plain, confidentStatusPick()},
		{"pick under the bar", true, plain, unconfident},
		{"template needs an argument", true, argBound, confidentStatusPick()},
		{"no template", true, nil, confidentStatusPick()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := testJevAgent(t)
			agent.cfg.JevDirectTools = tc.enabled
			agent.tools = statusServer(t, tc.templates)
			if got, ok := agent.directToolReply(context.Background(), tc.route, "is the server up"); ok {
				t.Fatalf("directToolReply = %q, want a decline so the model path runs", got)
			}
		})
	}
}

// directReplyText is directToolReply for a message that names one thing: the
// one reply, or a failure when it comes back as several.
func directReplyText(agent *Agent, route RouteDecision, message string) (string, bool) {
	replies, ok := agent.directToolReply(context.Background(), route, message)
	return strings.Join(replies, "\n"), ok
}

// noModelCompletions fails the test's expectation, not the process, when a
// path that promises no model call makes one.
type noModelCompletions struct{ calls atomic.Int32 }

func (c *noModelCompletions) Complete(context.Context, TurnPrompt, string) (CompletionResult, error) {
	c.calls.Add(1)
	return CompletionResult{}, errors.New("unexpected model call")
}

// repliesTurn records each message a turn sends.
type repliesTurn struct {
	httpTurn
	sent []string
}

func (t *repliesTurn) Reply(_ context.Context, content string) error {
	t.sent = append(t.sent, content)
	return nil
}

func TestFinishWithDirectSendsOneMessagePerReply(t *testing.T) {
	cases := map[string][]string{
		"one item":  {"Iron Bar median Spectres by stage: Modern 4 0.58 (181)."},
		"two items": {"Iron Bar median Spectres by stage: Modern 4 0.58 (181).", "Gold Bar median Spectres by stage: Modern 4 1.20 (40)."},
	}
	for name, texts := range cases {
		t.Run(name, func(t *testing.T) {
			agent := testJevAgent(t)
			turn := &repliesTurn{httpTurn: httpTurn{transport: "http"}}

			if err := agent.finishWithDirect(context.Background(), turn, texts); err != nil {
				t.Fatalf("finishWithDirect: %v", err)
			}
			if !reflect.DeepEqual(turn.sent, texts) {
				t.Errorf("sent = %q, want one message per reply %q", turn.sent, texts)
			}
		})
	}
}
