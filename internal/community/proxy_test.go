package community

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProxyClientSendsBoundedCommunityRequest(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.Header.Get("X-Ward-Role") != "community" {
			t.Errorf("role header = %q", request.Header.Get("X-Ward-Role"))
		}
		if request.Header.Get("X-Ward-Harness") != "discord" {
			t.Errorf("harness header = %q", request.Header.Get("X-Ward-Harness"))
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		var body chatRequest
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Model != "selected-model" {
			t.Errorf("model = %q", body.Model)
		}
		if body.Metadata.Role != "community" || body.Metadata.Seat != "Sirens Echo" {
			t.Errorf("metadata identity = %#v", body.Metadata)
		}
		if body.Stream {
			t.Error("stream must be false")
		}
		// The reply contract is plain text, so the request must not ask the
		// backend for a JSON object. See coilyco-gaming/sirens-echo#102.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Errorf("decode raw body: %v", err)
		}
		if _, present := fields["response_format"]; present {
			t.Errorf("request carries response_format = %s", fields["response_format"])
		}
		if len(body.Messages) != 2 ||
			body.Messages[0].Role != "system" ||
			body.Messages[1].Role != "user" {
			t.Errorf("messages = %#v", body.Messages)
		}
		if len(body.Tools) != 0 {
			t.Errorf("tools = %#v", body.Tools)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(
			`{"choices":[{"message":{"content":"Request received."}}]}`,
		))
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:     server.URL,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		Harness:     transportDiscord,
		HTTPClient:  &http.Client{Timeout: time.Second},
	}
	got, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `Request received.` {
		t.Fatalf("completion = %q", got.Content)
	}
	if len(got.ToolCalls) != 0 {
		t.Fatalf("tool calls = %#v", got.ToolCalls)
	}
}

func TestProxyClientDiscoversCallsAndContinuesWithEcoMCP(t *testing.T) {
	t.Parallel()
	mcpServer := mcp.NewServer(
		&mcp.Implementation{Name: "eco-test", Version: "1"},
		nil,
	)
	mcp.AddTool(
		mcpServer,
		&mcp.Tool{
			Name:        "get_eco_server_status",
			Description: "Get current Eco server status.",
		},
		func(
			_ context.Context,
			_ *mcp.CallToolRequest,
			_ struct{},
		) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "online"},
				},
			}, nil, nil
		},
	)
	mcpHTTP := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	))
	defer mcpHTTP.Close()

	var round atomic.Int32
	proxyHTTP := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		currentRound := round.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		switch currentRound {
		case 1:
			// The model is offered the roster plus the harness refresh, and this
			// is the assertion that says so at the wire. See sirens-echo#163.
			if len(body.Tools) != 2 {
				t.Errorf("tool count = %d", len(body.Tools))
			} else if body.Tools[0].Function.Name != "eco__get_eco_server_status" ||
				body.Tools[1].Function.Name != refreshToolProxyName() {
				t.Errorf("tool names = %q, %q",
					body.Tools[0].Function.Name, body.Tools[1].Function.Name)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"eco__get_eco_server_status","arguments":"{}"}}]}}]}`,
			))
		case 2:
			raw, err := json.Marshal(body.Messages)
			if err != nil {
				t.Errorf("marshal messages: %v", err)
			}
			if !strings.Contains(string(raw), `"role":"tool"`) ||
				!strings.Contains(string(raw), "online") {
				t.Errorf("continuation messages = %s", raw)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":[{"type":"output_text","text":"Eco is online now."}]}}]}`,
			))
		default:
			http.Error(writer, "unexpected model round", http.StatusInternalServerError)
		}
	}))
	defer proxyHTTP.Close()

	client := ProxyClient{
		BaseURL:     proxyHTTP.URL,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		HTTPClient:  &http.Client{Timeout: time.Second},
		Tools: &MCPProvider{
			Servers: []MCPServerDefinition{
				{Name: "eco", URL: mcpHTTP.URL},
			},
			HTTPClient: &http.Client{Timeout: time.Second},
		},
	}
	got, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `Eco is online now.` {
		t.Fatalf("completion = %q", got.Content)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("tool calls = %#v", got.ToolCalls)
	}
	if got.ToolCalls[0].Name != "eco__get_eco_server_status" ||
		!strings.Contains(got.ToolCalls[0].Result, "online") {
		t.Fatalf("tool call = %#v", got.ToolCalls[0])
	}
	if calls := round.Load(); calls != 2 {
		t.Fatalf("model rounds = %d", calls)
	}
}

func TestProxyClientPreservesDeepSeekReasoningContentAcrossToolCall(t *testing.T) {
	t.Parallel()
	const reasoning = "The Eco status tool is needed before answering."
	var round atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch round.Add(1) {
		case 1:
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":null,"reasoning_content":"` + reasoning + `","tool_calls":[{"id":"call-1","type":"function","function":{"name":"eco__get_eco_server_status","arguments":"{}"}}]}}]}`,
			))
		case 2:
			if len(body.Messages) != 4 {
				t.Errorf("messages = %#v", body.Messages)
			} else if got := body.Messages[2].ReasoningContent; got != reasoning {
				t.Errorf("reasoning content = %q, want %q", got, reasoning)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"Eco is online now."}}]}`,
			))
		default:
			http.Error(writer, "unexpected model round", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:     server.URL,
		Model:       "deepseek-reasoner",
		AuditRole:   "community",
		Attribution: "CoilyCo",
		HTTPClient:  &http.Client{Timeout: time.Second},
		Tools:       fixtureToolProvider{},
	}
	got, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `Eco is online now.` {
		t.Fatalf("completion = %q", got.Content)
	}
	if calls := round.Load(); calls != 2 {
		t.Fatalf("model rounds = %d", calls)
	}
}

func TestChatContentAcceptsCompatibleShapes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "string", raw: `"hello"`, want: "hello"},
		{name: "null", raw: `null`, want: ""},
		{
			name: "text parts",
			raw:  `[{"type":"text","text":"hello "},{"type":"output_text","text":"world"}]`,
			want: "hello world",
		},
		{name: "object", raw: `{"text":"hello"}`, want: "hello"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var got chatContent
			if err := json.Unmarshal([]byte(test.raw), &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.Text != test.want {
				t.Fatalf("text = %q, want %q", got.Text, test.want)
			}
		})
	}
}

func TestProxyClientRepairsStyleViolationOnce(t *testing.T) {
	t.Parallel()
	var round atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch round.Add(1) {
		case 1:
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"Hey there! Happy to help."}}]}`,
			))
		case 2:
			repairPrompt := ""
			if len(body.Messages) == 4 {
				repairPrompt, _ = body.Messages[3].Content.(string)
			}
			if len(body.Messages) != 4 ||
				body.Messages[2].Role != "assistant" ||
				body.Messages[3].Role != "user" ||
				!strings.Contains(repairPrompt, "violated the required response contract") {
				t.Errorf("repair messages = %#v", body.Messages)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"repaired"}}]}`,
			))
		default:
			http.Error(writer, "unexpected model round", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:     server.URL,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		HTTPClient:  &http.Client{Timeout: time.Second},
	}
	got, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `repaired` {
		t.Fatalf("completion = %q", got.Content)
	}
	if calls := round.Load(); calls != 2 {
		t.Fatalf("model rounds = %d", calls)
	}
}

func TestProxyClientRepairsEmptyReplyOnce(t *testing.T) {
	t.Parallel()
	var round atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch round.Add(1) {
		case 1:
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":null}}]}`,
			))
		case 2:
			repairPrompt := ""
			if len(body.Messages) == 3 {
				repairPrompt, _ = body.Messages[2].Content.(string)
			}
			if len(body.Messages) != 3 ||
				body.Messages[2].Role != "user" ||
				!strings.Contains(repairPrompt, "violated the required response contract") {
				t.Errorf("repair messages = %#v", body.Messages)
			}
			if len(body.Tools) != 0 {
				t.Errorf("repair tools = %#v", body.Tools)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"repaired"}}]}`,
			))
		default:
			http.Error(writer, "unexpected model round", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:     server.URL,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		HTTPClient:  &http.Client{Timeout: time.Second},
	}
	got, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `repaired` {
		t.Fatalf("completion = %q", got.Content)
	}
	if calls := round.Load(); calls != 2 {
		t.Fatalf("model rounds = %d", calls)
	}
}

func TestProxyClientRepairsPersonalityOnce(t *testing.T) {
	t.Parallel()
	var round atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch round.Add(1) {
		case 1:
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"Hey there! 🫡"}}]}`,
			))
		case 2:
			repairPrompt := ""
			if len(body.Messages) == 4 {
				repairPrompt, _ = body.Messages[3].Content.(string)
			}
			if len(body.Messages) != 4 ||
				!strings.Contains(repairPrompt, "neutral, concise") ||
				len(body.Tools) != 0 {
				t.Errorf("repair request = %#v", body)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"Request received."}}]}`,
			))
		default:
			http.Error(writer, "unexpected model round", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:     server.URL,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		HTTPClient:  &http.Client{Timeout: time.Second},
	}
	got, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `Request received.` {
		t.Fatalf("completion = %q", got.Content)
	}
	if calls := round.Load(); calls != 2 {
		t.Fatalf("model rounds = %d", calls)
	}
}

func TestProxyClientSocialRepairPreservesSelectedStyle(t *testing.T) {
	t.Parallel()
	var round atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		switch round.Add(1) {
		case 1:
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"` + strings.Repeat("a", 1801) + `"}}]}`,
			))
		case 2:
			repairPrompt := ""
			if len(body.Messages) == 4 {
				repairPrompt, _ = body.Messages[3].Content.(string)
			}
			if !strings.Contains(repairPrompt, "selected social tone") ||
				strings.Contains(repairPrompt, "neutral, concise") {
				t.Errorf("repair prompt = %q", repairPrompt)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"Hey! I found it for you."}}]}`,
			))
		default:
			http.Error(writer, "unexpected model round", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:       server.URL,
		Model:         "selected-model",
		AuditRole:     "community",
		Attribution:   "Sirens Echo",
		ResponseStyle: ResponseStyleSocial,
		HTTPClient:    &http.Client{Timeout: time.Second},
	}
	got, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `Hey! I found it for you.` {
		t.Fatalf("completion = %q", got.Content)
	}
}

func TestProxyClientRejectsPersistentStyleViolation(t *testing.T) {
	t.Parallel()
	var round atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		round.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(
			`{"choices":[{"message":{"content":"Hey there! Happy to help."}}]}`,
		))
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:     server.URL,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		HTTPClient:  &http.Client{Timeout: time.Second},
	}
	_, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err == nil || !strings.Contains(err.Error(), "invalid response after 1 repair attempt") {
		t.Fatalf("error = %v", err)
	}
	if calls := round.Load(); calls != 2 {
		t.Fatalf("model rounds = %d", calls)
	}
}

func TestProxyClientRejectsToolCallDuringResponseRepair(t *testing.T) {
	t.Parallel()
	var round atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		writer.Header().Set("Content-Type", "application/json")
		switch round.Add(1) {
		case 1:
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"Hey there! Happy to help."}}]}`,
			))
		case 2:
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"unexpected","arguments":"{}"}}]}}]}`,
			))
		default:
			http.Error(writer, "unexpected model round", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:     server.URL,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		HTTPClient:  &http.Client{Timeout: time.Second},
	}
	_, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request")
	if err == nil || !strings.Contains(err.Error(), "tool call during response repair") {
		t.Fatalf("error = %v", err)
	}
	if calls := round.Load(); calls != 2 {
		t.Fatalf("model rounds = %d", calls)
	}
}

func TestProxyClientRejectsNonSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "internal detail should not escape", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := ProxyClient{
		BaseURL:    server.URL,
		Model:      "selected-model",
		HTTPClient: &http.Client{Timeout: time.Second},
	}
	if _, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "request"); err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestProxyToolNameRejectsNamesAgentProxyCannotCarry(t *testing.T) {
	t.Parallel()
	name := strings.Repeat("x", maxProxyToolNameBytes+1)
	if _, err := proxyToolName("eco", name); err == nil {
		t.Fatal("expected overlong tool name error")
	}
	if got, err := proxyToolName("eco", "status/current"); err != nil {
		t.Fatalf("proxyToolName: %v", err)
	} else if got != "eco__status_current" {
		t.Fatalf("name = %q", got)
	}
	if _, err := proxyToolName("", ""); err == nil {
		t.Fatal("expected empty tool name error")
	}
}

func Example_proxyToolName() {
	name, _ := proxyToolName("eco", "get_status")
	fmt.Println(name)
	// Output: eco__get_status
}

// Reproduces the reported failure: the whole budget goes to reasoning_content
// and content comes back empty. See docs/sirens-echo-budget.md.
func TestCompleteRaisesTheBudgetOnTruncatedEmptyContent(t *testing.T) {
	t.Parallel()
	var budgets []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			MaxTokens int `json:"max_tokens"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		budgets = append(budgets, payload.MaxTokens)
		w.Header().Set("Content-Type", "application/json")
		if len(budgets) == 1 {
			// First call burns the budget on reasoning and returns nothing.
			_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length",
				"message":{"content":"","reasoning_content":"thinking"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop",
			"message":{"content":"Recovered."}}]}`))
	}))
	defer server.Close()

	client := ProxyClient{BaseURL: server.URL, Model: "m", HTTPClient: server.Client()}
	result, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "req")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(result.Content, "Recovered.") {
		t.Fatalf("content = %q", result.Content)
	}
	if len(budgets) != 2 {
		t.Fatalf("model calls = %d, want 2", len(budgets))
	}
	if budgets[0] != baseCompletionTokens {
		t.Fatalf("first budget = %d, want %d", budgets[0], baseCompletionTokens)
	}
	if budgets[1] <= budgets[0] {
		t.Fatalf("budget did not rise: %v", budgets)
	}
}

// The escalation is bounded, and the error names the wall rather than
// surfacing as a generic contract failure.
func TestCompleteStopsRaisingAfterTheAllowedAttempts(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length",
			"message":{"content":"","reasoning_content":"thinking"}}]}`))
	}))
	defer server.Close()

	client := ProxyClient{BaseURL: server.URL, Model: "m", HTTPClient: server.Client()}
	_, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "req")
	if err == nil {
		t.Fatal("a permanently truncated completion was accepted")
	}
	if !strings.Contains(err.Error(), "truncated the completion") {
		t.Fatalf("error = %v, want it to name the truncation", err)
	}
	if calls != budgetRaisesAllowed+1 {
		t.Fatalf("model calls = %d, want %d", calls, budgetRaisesAllowed+1)
	}
}

// Truncated content that is not empty is a usable answer, so it must not
// trigger a raise.
func TestCompleteAcceptsTruncatedContentThatIsNotEmpty(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"length",
			"message":{"content":"Partial but usable."}}]}`))
	}))
	defer server.Close()

	client := ProxyClient{BaseURL: server.URL, Model: "m", HTTPClient: server.Client()}
	result, err := client.Complete(context.Background(), TurnPrompt{System: "system", Message: "user"}, "req")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(result.Content, "Partial but usable.") {
		t.Fatalf("content = %q", result.Content)
	}
	if calls != 1 {
		t.Fatalf("model calls = %d, want 1", calls)
	}
}

func TestBoundToolResultCapsReinjectionButKeepsGrounding(t *testing.T) {
	t.Parallel()
	small := strings.Repeat("a", 128)
	if got, trimmed := boundToolResult(small); got != small || trimmed {
		t.Fatal("a small result must pass through untouched")
	}
	huge := strings.Repeat("b", maxToolResultBytes*4)
	got, trimmed := boundToolResult(huge)
	if !trimmed {
		t.Fatal("an oversized result was not bounded")
	}
	if len(got) >= len(huge) {
		t.Fatalf("bounded length %d is not smaller than %d", len(got), len(huge))
	}
	if !strings.Contains(got, "[truncated by the runtime") {
		t.Fatal("the bound is not visible to the model")
	}
	// The magnitude is the part that makes the marker actionable. Without it
	// refetching the same window is a rational move. See issue 258.
	if !strings.Contains(got, fmt.Sprintf("%d of %d bytes delivered", maxToolResultBytes, len(huge))) {
		t.Fatalf("the marker does not say how much was lost: %q", got[len(got)-80:])
	}
}

func TestBoundToolResultHonoursTheByteBudgetOnMultibyteText(t *testing.T) {
	t.Parallel()
	// Three bytes per rune. Slicing runes against a byte cap returned roughly
	// three times the budget.
	huge := strings.Repeat("世", maxToolResultBytes)
	got, trimmed := boundToolResult(huge)
	if !trimmed {
		t.Fatal("an oversized multibyte result was not bounded")
	}
	marker := strings.LastIndex(got, "\n[truncated by the runtime")
	if marker < 0 {
		t.Fatal("the bound is not visible to the model")
	}
	body := got[:marker]
	if len(body) > maxToolResultBytes {
		t.Fatalf("bounded body is %d bytes, want at most %d", len(body), maxToolResultBytes)
	}
	if !utf8.ValidString(body) {
		t.Fatal("the bound cut a rune in half")
	}
}

// alwaysOneTool offers a single tool that always answers, so a test can drive
// the round loop to its ceiling.
type alwaysOneTool struct{}

func (alwaysOneTool) Open(context.Context) (ToolSession, error) { return alwaysOneTool{}, nil }
func (alwaysOneTool) Grounding() []GroundingDocument            { return nil }
func (alwaysOneTool) Unavailable() []string                     { return nil }
func (alwaysOneTool) Close() error                              { return nil }

func (alwaysOneTool) Tools() []ToolDefinition {
	return []ToolDefinition{{
		Name:        "probe__look",
		Original:    "look",
		Server:      "probe",
		Description: "look at something",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}}
}

func (alwaysOneTool) Call(context.Context, string, map[string]any) (ToolResult, error) {
	return ToolResult{Text: `{"result":"a real observation"}`}, nil
}

// Spending the tool budget must answer from the results already gathered rather
// than discarding eight rounds of real tool output. See issue 258.
func TestProxyClientAnswersAfterSpendingTheToolBudget(t *testing.T) {
	t.Parallel()
	var rounds atomic.Int32
	var sawBudgetNotice atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		rounds.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		// Once the tools are withdrawn the model can only answer, which is the
		// behavior under test.
		if len(body.Tools) == 0 {
			for _, message := range body.Messages {
				if text, ok := message.Content.(string); ok &&
					strings.Contains(text, "tool budget for this turn is spent") {
					sawBudgetNotice.Store(true)
				}
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"Partial answer from what was gathered."}}]}`,
			))
			return
		}
		_, _ = writer.Write([]byte(
			`{"choices":[{"message":{"tool_calls":[{"id":"c1","type":"function",` +
				`"function":{"name":"probe__look","arguments":"{}"}}]}}]}`,
		))
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:     server.URL,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		Tools:       alwaysOneTool{},
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	}
	got, err := client.Complete(context.Background(), TurnPrompt{System: "s", Message: "u"}, "request")
	if err != nil {
		t.Fatalf("Complete returned an error instead of a degraded answer: %v", err)
	}
	if got.Content != "Partial answer from what was gathered." {
		t.Fatalf("content = %q", got.Content)
	}
	if !sawBudgetNotice.Load() {
		t.Error("the final call did not carry the spent-budget notice")
	}
	if len(got.ToolCalls) != maxToolRounds {
		t.Errorf("kept %d tool results, want the %d the rounds produced", len(got.ToolCalls), maxToolRounds)
	}
	if calls := rounds.Load(); calls != maxToolRounds+1 {
		t.Errorf("model rounds = %d, want %d", calls, maxToolRounds+1)
	}
}
