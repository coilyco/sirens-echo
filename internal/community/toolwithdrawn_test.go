package community

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// withdrawnServer asks for a tool on every request that carries no tools
// until it has been told twice that none are available, as askUntil says.
func withdrawnServer(t *testing.T, askUntil int, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		calls.Add(1)
		told := 0
		for _, message := range body.Messages {
			if text, ok := message.Content.(string); ok && strings.Contains(text, "No tools are available now") {
				told++
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		if told >= askUntil {
			_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"Answered from what was gathered."}}]}`))
			return
		}
		_, _ = writer.Write([]byte(
			`{"choices":[{"message":{"tool_calls":[{"id":"c1","type":"function",` +
				`"function":{"name":"probe__look","arguments":"{}"}}]}}]}`,
		))
	}))
}

func withdrawnClient(url string) ProxyClient {
	return ProxyClient{
		BaseURL:     url,
		Model:       "selected-model",
		AuditRole:   "community",
		Attribution: "Sirens Echo",
		Tools:       alwaysOneTool{},
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
	}
}

// ECHO-5: the allowance withdraws tools, the model asks for one anyway, and the
// turn used to fail there with the gathered results thrown away.
func TestAModelAskingForAWithdrawnToolIsAskedForTheAnswer(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := withdrawnServer(t, 1, &calls)
	defer server.Close()

	// One call of allowance leaves no tool round, so tools are withdrawn at once.
	ctx := withTurnBudget(context.Background(), 1)
	result, err := withdrawnClient(server.URL).Complete(ctx, TurnPrompt{System: "s", Message: "u"}, "request")
	if err != nil {
		t.Fatalf("a model that asked once for a withdrawn tool failed the turn: %v", err)
	}
	if !strings.Contains(result.Content, "Answered from what was gathered") {
		t.Errorf("reply = %q, want the answer given after the notice", result.Content)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("model calls = %d, want 2: the refused ask and the answer", got)
	}
}

// The ask is bounded, so a model that never stops asking still ends the turn.
func TestAModelThatKeepsAskingForAWithdrawnToolStillFails(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := withdrawnServer(t, 1<<30, &calls)
	defer server.Close()

	ctx := withTurnBudget(context.Background(), 1)
	_, err := withdrawnClient(server.URL).Complete(ctx, TurnPrompt{System: "s", Message: "u"}, "request")
	if !errors.Is(err, ErrToolRoundsExhausted) {
		t.Fatalf("err = %v, want ErrToolRoundsExhausted", err)
	}
	if got, want := int(calls.Load()), maxWithdrawnAsks+1; got != want {
		t.Errorf("model calls = %d, want %d: the asks plus the one that gave up", got, want)
	}
}
