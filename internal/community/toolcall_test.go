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
)

func TestResolveToolCallNamesWhyACallCannotRun(t *testing.T) {
	t.Parallel()
	definitions := map[string]ToolDefinition{"eco__market": {Name: "eco__market"}}
	cases := []struct {
		name string
		call chatCalledFunction
		want string
	}{
		{"runnable", chatCalledFunction{Name: "eco__market", Arguments: `{"item":"iron"}`}, ""},
		{"empty arguments", chatCalledFunction{Name: "eco__market"}, ""},
		{"unnamed", chatCalledFunction{Arguments: "{}"}, toolCallUnnamed},
		{"unavailable", chatCalledFunction{Name: "get_stores", Arguments: "{}"}, toolCallUnavailable},
		{"bad arguments", chatCalledFunction{Name: "eco__market", Arguments: "{item:"}, toolCallBadArgs},
	}
	for _, tc := range cases {
		_, _, got := resolveToolCall(chatToolCall{ID: "c1", Function: tc.call}, definitions)
		if got != tc.want {
			t.Errorf("%s: reason = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A backend that streams a call without an id still gets a well-formed next
// request, with the assistant call and its result sharing one id (#8071).
func TestCompleteGivesAnIDlessToolCallAnID(t *testing.T) {
	t.Parallel()
	requests := &atomic.Int32{}
	paired := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"tool_calls":[{"id":"","type":"function",` +
					`"function":{"name":"eco__market","arguments":"{}"}}]}}]}`,
			))
			return
		}
		var payload chatRequest
		_ = json.NewDecoder(request.Body).Decode(&payload)
		count := len(payload.Messages)
		assistant, result := payload.Messages[count-2], payload.Messages[count-1]
		paired <- len(assistant.ToolCalls) == 1 &&
			assistant.ToolCalls[0].ID != "" &&
			assistant.ToolCalls[0].ID == result.ToolCallID
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"Market read."}}]}`))
	}))
	defer server.Close()

	calls := &atomic.Int32{}
	client := ProxyClient{
		BaseURL:    server.URL,
		Model:      "selected-model",
		AuditRole:  "community",
		Tools:      twoServerSession{calls: calls},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
	result, err := client.Complete(context.Background(), TurnPrompt{System: "s", Message: "u"}, "request")
	if err != nil {
		t.Fatalf("Complete error = %v", err)
	}
	if result.Content != "Market read." || calls.Load() != 1 {
		t.Fatalf("Content = %q, tool calls = %d", result.Content, calls.Load())
	}
	if !<-paired {
		t.Fatal("the assistant tool call and its result did not share a synthesized id")
	}
}

// A back-to-back identical call is answered from the last result and not run,
// so a model looping on one empty search is told so (#8071).
func TestCompleteAnswersABackToBackRepeatWithoutRunningIt(t *testing.T) {
	t.Parallel()
	requests := &atomic.Int32{}
	notice := make(chan string, 1)
	call := `{"choices":[{"message":{"tool_calls":[{"id":"c%d","type":"function",` +
		`"function":{"name":"eco__market","arguments":"{\"item\":\"iron\"}"}}]}}]}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch requests.Add(1) {
		case 1, 2:
			_, _ = fmt.Fprintf(writer, call, requests.Load())
		default:
			var payload chatRequest
			_ = json.NewDecoder(request.Body).Decode(&payload)
			text, _ := payload.Messages[len(payload.Messages)-1].Content.(string)
			notice <- text
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"Done."}}]}`))
		}
	}))
	defer server.Close()

	calls := &atomic.Int32{}
	client := ProxyClient{
		BaseURL:    server.URL,
		Model:      "selected-model",
		AuditRole:  "community",
		Tools:      twoServerSession{calls: calls},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
	if _, err := client.Complete(context.Background(), TurnPrompt{System: "s", Message: "u"}, "request"); err != nil {
		t.Fatalf("Complete error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("the repeated call ran %d times, want 1", calls.Load())
	}
	if text := <-notice; !strings.Contains(text, "was not run again") {
		t.Fatalf("repeat answer = %q", text)
	}
}

// readOnlySession declares every tool read-only, the way eco-app's are.
type readOnlySession struct{ twoServerSession }

func (s readOnlySession) Open(context.Context) (ToolSession, error) { return s, nil }

func (s readOnlySession) Tools() []ToolDefinition {
	tools := s.twoServerSession.Tools()
	for index := range tools {
		tools[index].ReadOnly = true
	}
	return tools
}

// After reads only, a summoned turn never gets a blank, on any transport
// (#8326). A call that may have spoken keeps the chosen silence (#895).
func TestAnEmptyReplyIsRepairedAfterReadsOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		readOnly bool
		want     string
	}{
		{true, "Iron trades at 3."},
		{false, ""},
	}
	for _, tc := range cases {
		requests := &atomic.Int32{}
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			switch requests.Add(1) {
			case 1:
				_, _ = writer.Write([]byte(`{"choices":[{"message":{"tool_calls":[{"id":"c1","type":"function",` +
					`"function":{"name":"eco__market","arguments":"{}"}}]}}]}`))
			case 2:
				_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
			default:
				_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"Iron trades at 3."}}]}`))
			}
		}))
		var tools ToolProvider = twoServerSession{calls: &atomic.Int32{}}
		if tc.readOnly {
			tools = readOnlySession{twoServerSession{calls: &atomic.Int32{}}}
		}
		client := ProxyClient{
			BaseURL:    server.URL,
			Model:      "selected-model",
			AuditRole:  "community",
			Tools:      tools,
			HTTPClient: &http.Client{Timeout: 5 * time.Second},
		}
		result, err := client.Complete(context.Background(), TurnPrompt{System: "s", Message: "u"}, "request")
		server.Close()
		if err != nil {
			t.Fatalf("readOnly=%v: Complete error = %v", tc.readOnly, err)
		}
		if result.Content != tc.want {
			t.Fatalf("readOnly=%v: Content = %q, want %q", tc.readOnly, result.Content, tc.want)
		}
	}
}
