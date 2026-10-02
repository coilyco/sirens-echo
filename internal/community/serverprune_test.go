package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

// twoServerSession offers one tool, guidance line and grounding document per
// server, so a test can see which server's share reached the request.
type twoServerSession struct {
	calls *atomic.Int32
}

func (s twoServerSession) Open(context.Context) (ToolSession, error) { return s, nil }
func (twoServerSession) Unavailable() []string                       { return nil }
func (twoServerSession) Close() error                                { return nil }

func (twoServerSession) Tools() []ToolDefinition {
	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	return []ToolDefinition{
		{Name: "eco__market", Original: "market", Server: "eco", Description: "eco market", InputSchema: schema},
		{Name: "wiki__search", Original: "search", Server: "wiki", Description: "wiki search", InputSchema: schema},
	}
}

func (twoServerSession) Guidance() []ServerGuidance {
	return []ServerGuidance{
		{Server: "eco", Text: "eco-guidance-marker"},
		{Server: "wiki", Text: "wiki-guidance-marker"},
	}
}

func (twoServerSession) Grounding() []GroundingDocument {
	return []GroundingDocument{
		{Server: "eco", URI: "eco://doc", Title: "eco", Text: "eco-grounding-marker"},
		{Server: "wiki", URI: "wiki://doc", Title: "wiki", Text: "wiki-grounding-marker"},
	}
}

func (s twoServerSession) Call(context.Context, string, map[string]any) (ToolResult, error) {
	s.calls.Add(1)
	return ToolResult{Text: "{}"}, nil
}

func TestCompleteLeavesADroppedServerOutOfTheRequest(t *testing.T) {
	t.Parallel()
	var first chatRequest
	var seen atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if seen.CompareAndSwap(false, true) {
			first = body
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"Answer."}}]}`))
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:    server.URL,
		Model:      "selected-model",
		AuditRole:  "community",
		Tools:      twoServerSession{calls: &atomic.Int32{}},
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}
	prompt := TurnPrompt{System: "s", Message: "u"}
	if _, err := client.Complete(withDroppedServers(context.Background(), []string{"wiki"}), prompt, "request"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	names := make([]string, 0, len(first.Tools))
	for _, tool := range first.Tools {
		names = append(names, tool.Function.Name)
	}
	if len(names) != 1 || names[0] != "eco__market" {
		t.Fatalf("request tools = %v, want only eco__market", names)
	}
	var system strings.Builder
	for _, message := range first.Messages {
		if text, ok := message.Content.(string); ok {
			system.WriteString(text)
		}
	}
	for _, want := range []string{"eco-guidance-marker", "eco-grounding-marker"} {
		if !strings.Contains(system.String(), want) {
			t.Errorf("kept server's %q is missing from the request", want)
		}
	}
	for _, unwanted := range []string{"wiki-guidance-marker", "wiki-grounding-marker"} {
		if strings.Contains(system.String(), unwanted) {
			t.Errorf("dropped server's %q still reached the request", unwanted)
		}
	}
}

// A model naming a pruned tool anyway must not reach the server behind it, and
// reads the refusal as a tool result instead of failing the turn (#8071).
func TestCompleteRefusesACallToADroppedServersTool(t *testing.T) {
	t.Parallel()
	requests := &atomic.Int32{}
	refusal := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"tool_calls":[{"id":"c1","type":"function",` +
					`"function":{"name":"wiki__search","arguments":"{}"}}]}}]}`,
			))
			return
		}
		var payload chatRequest
		_ = json.NewDecoder(request.Body).Decode(&payload)
		last := payload.Messages[len(payload.Messages)-1]
		if last.Role == "tool" && last.ToolCallID == "c1" {
			text, _ := last.Content.(string)
			refusal <- text
		}
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"Answered without it."}}]}`))
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
	prompt := TurnPrompt{System: "s", Message: "u"}
	result, err := client.Complete(withDroppedServers(context.Background(), []string{"wiki"}), prompt, "request")
	if err != nil {
		t.Fatalf("Complete error = %v, want the refusal answered and the turn finished", err)
	}
	if result.Content != "Answered without it." {
		t.Fatalf("Content = %q", result.Content)
	}
	if calls.Load() != 0 {
		t.Fatalf("the dropped server was called %d times", calls.Load())
	}
	select {
	case text := <-refusal:
		if !strings.Contains(text, `no tool named "wiki__search"`) {
			t.Fatalf("tool result = %q, want it to name the unavailable tool", text)
		}
	default:
		t.Fatal("the second request carried no tool result answering c1")
	}
}

func TestApplyRouteAnswersDropsOnlyAnExplicitNo(t *testing.T) {
	agent := testJevAgent(t)
	agent.tools = &MCPProvider{Servers: []MCPServerDefinition{{Name: "eco"}, {Name: "wiki"}, {Name: "steam"}}}
	questions, meta := agent.buildRouteQuestions(TranscriptEntry{Content: "hi"}, nil, false, false)
	if len(questions) == 0 {
		t.Fatal("no questions built")
	}
	answers := map[string]systemone.Answer{
		"server:eco":  {Probability: 0.9},
		"server:wiki": {Probability: 0.1},
		// steam unanswered: neither kept nor dropped.
	}
	decision := RouteDecision{Ran: true}
	agent.applyRouteAnswers(&decision, meta, answers, nil)

	if got := strings.Join(decision.Servers, ","); got != "eco" {
		t.Errorf("Servers = %q, want eco", got)
	}
	if got := strings.Join(decision.PrunedServers(), ","); got != "wiki" {
		t.Errorf("PrunedServers = %q, want wiki alone, steam kept on a missing answer", got)
	}
}

func TestPrunedServersKeepsEveryServerWhenTheFamilyDidNotDecide(t *testing.T) {
	agent := testJevAgent(t)
	_, meta := agent.buildRouteQuestions(TranscriptEntry{Content: "hi"}, nil, false, false)

	// Asked, and Jev answered nothing for the family.
	missing := RouteDecision{Ran: true}
	agent.applyRouteAnswers(&missing, meta, map[string]systemone.Answer{}, nil)
	if pruned := missing.PrunedServers(); len(pruned) != 0 {
		t.Errorf("missing answers pruned %v, want nothing", pruned)
	}
	if missing.Fallbacks[RouteFamilyServer] != jevFallbackMissingAnswer {
		t.Errorf("server fallback = %q, want %q", missing.Fallbacks[RouteFamilyServer], jevFallbackMissingAnswer)
	}

	skipped := RouteDecision{DroppedServers: []string{"eco"}}
	if pruned := skipped.PrunedServers(); len(pruned) != 0 {
		t.Errorf("a stage that never ran pruned %v", pruned)
	}

	failed := RouteDecision{Ran: true, DroppedServers: []string{"eco"}}
	failed.fellBackTo(RouteFamilyServer, jevFallbackCallFailed)
	if pruned := failed.PrunedServers(); len(pruned) != 0 {
		t.Errorf("a fallen-back family pruned %v", pruned)
	}
}
