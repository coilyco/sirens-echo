package community

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
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
		if body.ResponseFormat.Type != "json_object" {
			t.Errorf("response format = %#v", body.ResponseFormat)
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
			`{"choices":[{"message":{"content":"{\"reply\":\"Request received.\",\"issue\":null}"}}]}`,
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
	got, err := client.Complete(context.Background(), "system", "user", "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `{"reply":"Request received.","issue":null}` {
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
		if body.ResponseFormat.Type != "json_object" {
			t.Errorf("response format = %#v", body.ResponseFormat)
		}
		currentRound := round.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		switch currentRound {
		case 1:
			if len(body.Tools) != 1 {
				t.Errorf("tool count = %d", len(body.Tools))
			} else if body.Tools[0].Function.Name != "eco__get_eco_server_status" {
				t.Errorf("tool name = %q", body.Tools[0].Function.Name)
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
				`{"choices":[{"message":{"content":[{"type":"output_text","text":"{\"reply\":\"Eco is online now.\",\"issue\":null}"}]}}]}`,
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
		Tools: MCPProvider{
			Servers: []MCPServerDefinition{
				{Name: "eco", URL: mcpHTTP.URL},
			},
			HTTPClient: &http.Client{Timeout: time.Second},
		},
	}
	got, err := client.Complete(context.Background(), "system", "user", "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `{"reply":"Eco is online now.","issue":null}` {
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
				`{"choices":[{"message":{"content":"{\"reply\":\"Eco is online now.\",\"issue\":null}"}}]}`,
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
	got, err := client.Complete(context.Background(), "system", "user", "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `{"reply":"Eco is online now.","issue":null}` {
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

func TestProxyClientRepairsInvalidStructuredOutputOnce(t *testing.T) {
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
				"{\"choices\":[{\"message\":{\"content\":\"```json\\n{\\\"reply\\\":\\\"broken\\\" local,\\\"issue\\\":null}\\n```\"}}]}",
			))
		case 2:
			repairPrompt := ""
			if len(body.Messages) == 4 {
				repairPrompt, _ = body.Messages[3].Content.(string)
			}
			if len(body.Messages) != 4 ||
				body.Messages[2].Role != "assistant" ||
				body.Messages[3].Role != "user" ||
				!strings.Contains(repairPrompt, "valid JSON object") {
				t.Errorf("repair messages = %#v", body.Messages)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"{\"reply\":\"repaired\",\"issue\":null}"}}]}`,
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
	got, err := client.Complete(context.Background(), "system", "user", "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `{"reply":"repaired","issue":null}` {
		t.Fatalf("completion = %q", got.Content)
	}
	if calls := round.Load(); calls != 2 {
		t.Fatalf("model rounds = %d", calls)
	}
}

func TestProxyClientRepairsEmptyStructuredOutputOnce(t *testing.T) {
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
				!strings.Contains(repairPrompt, "valid JSON object") {
				t.Errorf("repair messages = %#v", body.Messages)
			}
			if len(body.Tools) != 0 {
				t.Errorf("repair tools = %#v", body.Tools)
			}
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"{\"reply\":\"repaired\",\"issue\":null}"}}]}`,
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
	got, err := client.Complete(context.Background(), "system", "user", "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `{"reply":"repaired","issue":null}` {
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
				`{"choices":[{"message":{"content":"{\"reply\":\"Hey there! 🫡\",\"issue\":null}"}}]}`,
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
				`{"choices":[{"message":{"content":"{\"reply\":\"Request received.\",\"issue\":null}"}}]}`,
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
	got, err := client.Complete(context.Background(), "system", "user", "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `{"reply":"Request received.","issue":null}` {
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
				`{"choices":[{"message":{"content":"Hey there!"}}]}`,
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
				`{"choices":[{"message":{"content":"{\"reply\":\"Hey! I found it for you.\",\"issue\":null}"}}]}`,
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
	got, err := client.Complete(context.Background(), "system", "user", "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.Content != `{"reply":"Hey! I found it for you.","issue":null}` {
		t.Fatalf("completion = %q", got.Content)
	}
}

func TestProxyClientRejectsInvalidStructuredOutputAfterRepair(t *testing.T) {
	t.Parallel()
	var round atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		round.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(
			`{"choices":[{"message":{"content":"not JSON"}}]}`,
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
	_, err := client.Complete(context.Background(), "system", "user", "request")
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
				`{"choices":[{"message":{"content":"not JSON"}}]}`,
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
	_, err := client.Complete(context.Background(), "system", "user", "request")
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
	if _, err := client.Complete(context.Background(), "system", "user", "request"); err == nil {
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
