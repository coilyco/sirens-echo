package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
