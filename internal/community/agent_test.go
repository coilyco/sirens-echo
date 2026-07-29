package community

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestSeenMessagesDeduplicatesAndEvicts(t *testing.T) {
	t.Parallel()
	seen := newSeenMessages(2)
	if !seen.Add("one") {
		t.Fatal("first message was not accepted")
	}
	if seen.Add("one") {
		t.Fatal("duplicate message was accepted")
	}
	if !seen.Add("two") || !seen.Add("three") {
		t.Fatal("new messages were not accepted")
	}
	if !seen.Add("one") {
		t.Fatal("oldest message was not evicted")
	}
}

func TestAppendTrackingLinkKeepsDiscordLimit(t *testing.T) {
	t.Parallel()
	reply := make([]rune, 2100)
	for index := range reply {
		reply[index] = 'a'
	}
	got := appendTrackingLink(string(reply), "https://forgejo.example/issues/1")
	if len([]rune(got)) > 1990 {
		t.Fatalf("reply length = %d", len([]rune(got)))
	}
}

type fixtureTurn struct {
	requestID string
	history   []TranscriptEntry
	current   TranscriptEntry
	reply     string
}

func (t *fixtureTurn) RequestID() string {
	return t.requestID
}

func (t *fixtureTurn) History(context.Context) ([]TranscriptEntry, error) {
	return append([]TranscriptEntry(nil), t.history...), nil
}

func (t *fixtureTurn) Current() TranscriptEntry {
	return t.current
}

func (t *fixtureTurn) Reply(_ context.Context, content string) error {
	t.reply = content
	return nil
}

type fixtureToolProvider struct{}

func (fixtureToolProvider) Open(context.Context) (ToolSession, error) {
	return fixtureToolSession{}, nil
}

type fixtureToolSession struct{}

func (fixtureToolSession) Tools() []ToolDefinition {
	return []ToolDefinition{
		{
			Name:        "eco__get_eco_server_status",
			Server:      "eco",
			Original:    "get_eco_server_status",
			Description: "Get current Eco server status.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (fixtureToolSession) Call(
	_ context.Context,
	name string,
	arguments map[string]any,
) (string, error) {
	if name != "eco__get_eco_server_status" {
		return "", fmt.Errorf("unexpected tool %q", name)
	}
	if len(arguments) != 0 {
		return "", fmt.Errorf("unexpected arguments %#v", arguments)
	}
	return `{"content":[{"type":"text","text":"online"}]}`, nil
}

func (fixtureToolSession) Close() error {
	return nil
}

func TestRunTurnJoinsHistoryModelToolValidationAndReplyTrace(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
	)
	t.Cleanup(func() {
		_ = traceProvider.Shutdown(context.Background())
	})
	telemetry, err := newTelemetry(
		slog.New(slog.NewJSONHandler(&logs, nil)),
		traceProvider,
		metricnoop.NewMeterProvider(),
	)
	if err != nil {
		t.Fatalf("newTelemetry: %v", err)
	}

	var modelRound atomic.Int32
	proxyHTTP := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		switch modelRound.Add(1) {
		case 1:
			userPrompt, ok := body.Messages[1].Content.(string)
			if !ok {
				t.Errorf("user prompt type = %T", body.Messages[1].Content)
			}
			for _, expected := range []string{
				"first member: Is Eco online?",
				"Sirens Echo: I can check if you summon me.",
				"current member: Can you check now?",
			} {
				if !strings.Contains(userPrompt, expected) {
					t.Errorf("prompt missing %q:\n%s", expected, userPrompt)
				}
			}
			writer.Header().Set("Content-Type", "application/json")
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
				t.Errorf("tool continuation = %s", raw)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(
				`{"choices":[{"message":{"content":"{\"reply\":\"Eco is online now.\",\"issue\":null}"}}]}`,
			))
		default:
			http.Error(writer, "unexpected model round", http.StatusInternalServerError)
		}
	}))
	defer proxyHTTP.Close()

	agent := &Agent{
		cfg: Config{Definition: Definition{
			Role:    "community",
			Seat:    "siren community host",
			Channel: "#bots",
		}},
		completions: ProxyClient{
			BaseURL:    proxyHTTP.URL,
			Route:      "community/conversation-management",
			Intent:     "conversation-management",
			Harness:    "sirens-discord-ops",
			HTTPClient: &http.Client{Timeout: time.Second},
			Tools:      fixtureToolProvider{},
			Telemetry:  telemetry,
		},
		systemPrompt: "verified AOS Community role and local knowledge",
		telemetry:    telemetry,
	}
	turn := &fixtureTurn{
		requestID: "request",
		history: []TranscriptEntry{
			{Author: "first member", Content: "Is Eco online?"},
			{Author: "Sirens Echo", Content: "I can check if you summon me."},
		},
		current: TranscriptEntry{
			Author:  "current member",
			Content: "Can you check now?",
		},
	}
	if err := agent.runTurn(context.Background(), turn); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if turn.reply != "Eco is online now." {
		t.Fatalf("reply = %q", turn.reply)
	}
	if rounds := modelRound.Load(); rounds != 2 {
		t.Fatalf("model rounds = %d", rounds)
	}

	requiredSpans := map[string]bool{
		"discord.turn":      false,
		"discord.receive":   false,
		"discord.history":   false,
		"context.assemble":  false,
		"aos.bundle.load":   false,
		"mcp.tools.list":    false,
		"model.chat":        false,
		"mcp.tool.call":     false,
		"response.validate": false,
		"discord.reply":     false,
	}
	var traceID string
	for _, span := range recorder.Ended() {
		if _, exists := requiredSpans[span.Name()]; exists {
			requiredSpans[span.Name()] = true
		}
		currentTraceID := span.SpanContext().TraceID().String()
		if traceID == "" {
			traceID = currentTraceID
		} else if currentTraceID != traceID {
			t.Fatalf("span %s trace = %s, want %s", span.Name(), currentTraceID, traceID)
		}
	}
	for name, found := range requiredSpans {
		if !found {
			t.Errorf("missing ended span %q", name)
		}
	}
	if traceID == "" || traceID == "00000000000000000000000000000000" {
		t.Fatalf("trace id = %q", traceID)
	}

	logged := logs.String()
	for _, expected := range []string{
		`"msg":"discord.input"`,
		`"member_input":"Can you check now?"`,
		`"msg":"context.rendered"`,
		`"system_prompt":"verified AOS Community role and local knowledge"`,
		`first member: Is Eco online?`,
		`"msg":"model.request"`,
		`eco__get_eco_server_status`,
		`"msg":"mcp.tool.input"`,
		`"msg":"mcp.tool.result"`,
		`online`,
		`"msg":"model.response"`,
		`"msg":"discord.reply"`,
		`"final_reply":"Eco is online now."`,
		`"trace_id":"`,
		`"span_id":"`,
	} {
		if !strings.Contains(logged, expected) {
			t.Errorf("content logs missing %q:\n%s", expected, logged)
		}
	}
}
