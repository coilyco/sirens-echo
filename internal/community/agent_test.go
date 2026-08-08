package community

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestNewAgentSupportsHTTPOnlySocialDeployment(t *testing.T) {
	t.Parallel()
	repoRoot := filepath.Join("..", "..")
	agent, err := NewAgent(Config{
		Definition: Definition{
			Identity:      "CoilyCo",
			AuditRole:     "general",
			ResponseStyle: ResponseStyleSocial,
			LocalSkillRoots: []string{
				filepath.Join(repoRoot, ".agents", "skills", "coilyco-general"),
			},
		},
		InstanceName:    "sirens-deep",
		DiscordEnabled:  false,
		AgentProxyURL:   "http://agent-proxy:8080",
		AgentProxyModel: "sirens-echo/deepseek",
		HTTPListenAddr:  "127.0.0.1:0",
		RequestTimeout:  time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if agent.session != nil {
		t.Fatal("HTTP-only agent created a Discord session")
	}
	if !strings.Contains(agent.systemPrompt, "warm and lively general-purpose assistant") {
		t.Fatal("HTTP-only agent did not load the social profile")
	}
	for _, forbidden := range []string{"Eco", "Sirens", "#bots", "Forgejo"} {
		if strings.Contains(agent.systemPrompt, forbidden) {
			t.Fatalf("HTTP-only agent retained %q", forbidden)
		}
	}
}

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

func TestChannelScopeCachesAndEvicts(t *testing.T) {
	t.Parallel()
	scope := newChannelScope(2)
	if _, known := scope.Get("one"); known {
		t.Fatal("empty scope reported a known channel")
	}
	scope.Set("one", true)
	allowed, known := scope.Get("one")
	if !known || !allowed {
		t.Fatal("scope did not retain an allowed channel")
	}
	scope.Set("two", false)
	scope.Set("three", false)
	if _, known := scope.Get("one"); known {
		t.Fatal("oldest channel was not evicted")
	}
}

func TestInScopeAcceptsThreadsUnderTheConfiguredChannel(t *testing.T) {
	t.Parallel()
	const (
		guildID   = "guild-1"
		channelID = "channel-1"
		otherID   = "channel-2"
		threadID  = "thread-1"
		foreignID = "thread-2"
	)
	state := discordgo.NewState()
	if err := state.GuildAdd(&discordgo.Guild{ID: guildID}); err != nil {
		t.Fatalf("GuildAdd: %v", err)
	}
	for _, channel := range []*discordgo.Channel{
		{ID: channelID, GuildID: guildID, Type: discordgo.ChannelTypeGuildText},
		{ID: otherID, GuildID: guildID, Type: discordgo.ChannelTypeGuildText},
		{
			ID:       threadID,
			GuildID:  guildID,
			Type:     discordgo.ChannelTypeGuildPublicThread,
			ParentID: channelID,
		},
		{
			ID:       foreignID,
			GuildID:  guildID,
			Type:     discordgo.ChannelTypeGuildPublicThread,
			ParentID: otherID,
		},
	} {
		if err := state.ChannelAdd(channel); err != nil {
			t.Fatalf("ChannelAdd %s: %v", channel.ID, err)
		}
	}
	session := &discordgo.Session{State: state}
	agent := &Agent{
		cfg:   Config{DiscordChannelID: channelID},
		scope: newChannelScope(16),
	}
	for _, testCase := range []struct {
		name    string
		id      string
		allowed bool
	}{
		{name: "configured channel", id: channelID, allowed: true},
		{name: "thread under configured channel", id: threadID, allowed: true},
		{name: "thread under another channel", id: foreignID, allowed: false},
		{name: "another channel", id: otherID, allowed: false},
		{name: "empty channel", id: "", allowed: false},
	} {
		if got := agent.inScope(session, testCase.id); got != testCase.allowed {
			t.Fatalf("%s: inScope = %v, want %v", testCase.name, got, testCase.allowed)
		}
	}
	if _, known := agent.scope.Get(threadID); !known {
		t.Fatal("thread scope decision was not cached")
	}
	if _, known := agent.scope.Get(channelID); known {
		t.Fatal("configured channel took the resolver path")
	}
}

func TestDiscordMessageSpanAttributesUseStringIdentifiers(t *testing.T) {
	t.Parallel()
	got := make(map[string]string)
	for _, item := range discordMessageSpanAttributes(
		"process",
		"guild",
		"channel",
		"message",
	) {
		got[string(item.Key)] = item.Value.AsString()
	}
	want := map[string]string{
		"messaging.system":         "discord",
		"messaging.operation.name": "process",
		"messaging.operation.type": "process",
		"messaging.message.id":     "message",
		"discord.guild.id":         "guild",
		"discord.channel.id":       "channel",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}

	withoutMessage := discordMessageSpanAttributes("send", "guild", "channel", "")
	for _, item := range withoutMessage {
		if item.Key == "messaging.message.id" {
			t.Fatal("empty message ID was exported")
		}
	}
}

func TestHTTPHandlerHealthz(t *testing.T) {
	t.Parallel()
	agent := &Agent{}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	agent.HTTPHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != `{"ok":true}` {
		t.Fatalf("body = %q", body)
	}
}

func TestHTTPHandlerRejectsEmptyTurn(t *testing.T) {
	t.Parallel()
	agent := &Agent{}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/turn",
		strings.NewReader(`{"content":"   "}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	agent.HTTPHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestHTTPHandlerRunsTheSharedTurnPath(t *testing.T) {
	t.Parallel()
	agent := &Agent{
		cfg: Config{Definition: Definition{MaxContextMessages: 12}},
		completions: fakeCompletionClient{responses: map[string]CompletionResult{
			"manual-request": {Content: `{"reply":"Echo is ready.","issue":null}`},
		}},
		systemPrompt: "neutral model policy and local knowledge",
		telemetry:    telemetryOrNoop(nil),
		slots:        make(chan struct{}, 1),
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/turn",
		strings.NewReader(`{"request_id":"manual-request","author":"tester","content":"Are you ready?"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	agent.HTTPHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response httpTurnResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Reply != "Echo is ready." {
		t.Fatalf("reply = %q", response.Reply)
	}
}

func TestHTTPHandlerContinuesRemoteTraceContext(t *testing.T) {
	t.Parallel()
	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = traceProvider.Shutdown(context.Background())
	})
	telemetry, err := newTelemetry(
		slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		traceProvider,
		metricnoop.NewMeterProvider(),
	)
	if err != nil {
		t.Fatalf("newTelemetry: %v", err)
	}
	agent := &Agent{
		cfg: Config{Definition: Definition{
			AuditRole:          "community",
			MaxContextMessages: 12,
		}},
		completions: fakeCompletionClient{responses: map[string]CompletionResult{
			"remote-request": {Content: `{"reply":"Echo is ready.","issue":null}`},
		}},
		systemPrompt: "neutral model policy and local knowledge",
		telemetry:    telemetry,
		slots:        make(chan struct{}, 1),
	}
	const remoteTraceID = "1234567890abcdef1234567890abcdef"
	const remoteSpanID = "1234567890abcdef"
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/turn",
		strings.NewReader(
			`{"request_id":"remote-request","author":"tester","content":"Are you ready?"}`,
		),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("traceparent", "00-"+remoteTraceID+"-"+remoteSpanID+"-01")
	recorderHTTP := httptest.NewRecorder()

	agent.HTTPHandler().ServeHTTP(recorderHTTP, request)

	if recorderHTTP.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorderHTTP.Code, recorderHTTP.Body.String())
	}
	var serverSpan, turnSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		switch {
		case span.Name() == "POST /v1/turn" && span.SpanKind() == trace.SpanKindServer:
			serverSpan = span
		case span.Name() == "community.turn":
			turnSpan = span
		}
	}
	if serverSpan == nil || turnSpan == nil {
		t.Fatalf("missing HTTP or turn span: %#v", recorder.Ended())
	}
	if got := serverSpan.SpanContext().TraceID().String(); got != remoteTraceID {
		t.Fatalf("server trace = %s, want %s", got, remoteTraceID)
	}
	if serverSpan.Parent().SpanID().String() != remoteSpanID {
		t.Fatalf("server parent = %s, want %s", serverSpan.Parent().SpanID(), remoteSpanID)
	}
	if turnSpan.SpanContext().TraceID() != serverSpan.SpanContext().TraceID() ||
		turnSpan.Parent().SpanID() != serverSpan.SpanContext().SpanID() {
		t.Fatalf("community turn is not a child of the HTTP server span")
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

func TestRunTurnRejectsIssueWhenAutomaticTrackingIsDisabled(t *testing.T) {
	t.Parallel()
	agent := &Agent{
		cfg: Config{Definition: Definition{ResponseStyle: ResponseStyleSocial}},
		completions: fakeCompletionClient{responses: map[string]CompletionResult{
			"request": {
				Content: `{"reply":"The answer is unknown.","issue":{"kind":"knowledge-gap","title":"Unknown answer","body":"The available context does not contain the answer."}}`,
			},
		}},
		systemPrompt: "general model policy",
		telemetry:    telemetryOrNoop(nil),
		slots:        make(chan struct{}, 1),
	}
	turn := &fixtureTurn{
		requestID: "request",
		current:   TranscriptEntry{Author: "user", Content: "What is the answer?"},
	}

	err := agent.runTurn(context.Background(), turn)
	if err == nil || !strings.Contains(err.Error(), "automatic issue tracking is disabled") {
		t.Fatalf("runTurn error = %v", err)
	}
	if turn.reply != genericFailureReply {
		t.Fatalf("reply = %q", turn.reply)
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

func (t *fixtureTurn) Transport() string { return transportDiscord }

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
	var proxyTraceParent atomic.Value
	proxyHTTP := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		proxyTraceParent.Store(request.Header.Get("traceparent"))
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
			AuditRole: "community",
			Identity:  "Sirens Echo",
			Channel:   "#bots",
		}},
		completions: ProxyClient{
			BaseURL:     proxyHTTP.URL,
			Model:       "selected-model",
			AuditRole:   "community",
			Attribution: "Sirens Echo",
			HTTPClient: &http.Client{
				Timeout: time.Second,
				Transport: otelhttp.NewTransport(
					http.DefaultTransport,
					otelhttp.WithTracerProvider(traceProvider),
					otelhttp.WithPropagators(telemetry.propagator),
				),
			},
			Tools:     fixtureToolProvider{},
			Telemetry: telemetry,
		},
		systemPrompt: "neutral model policy and local knowledge",
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
		"community.turn":    false,
		"community.input":   false,
		"community.history": false,
		"context.assemble":  false,
		"mcp.tools.list":    false,
		"model.chat":        false,
		"mcp.tool.call":     false,
		"response.validate": false,
		"community.reply":   false,
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
	if traceParent, _ := proxyTraceParent.Load().(string); !strings.Contains(traceParent, traceID) {
		t.Fatalf("Agent Proxy traceparent = %q, want trace %s", traceParent, traceID)
	}

	logged := logs.String()
	for _, expected := range []string{
		`"msg":"turn.input.accepted"`,
		`"input_bytes":18`,
		`"msg":"context.rendered"`,
		`"history_count":2`,
		`"system_prompt_bytes":`,
		`"user_prompt_bytes":`,
		`"msg":"model.request"`,
		`"request_bytes":`,
		`"msg":"mcp.tool.input"`,
		`"input_bytes":2`,
		`"msg":"mcp.tool.result"`,
		`"result_bytes":`,
		`"msg":"model.response"`,
		`"response_bytes":`,
		`"msg":"turn.reply.ready"`,
		`"reply_bytes":18`,
		`"trace_id":"`,
		`"span_id":"`,
	} {
		if !strings.Contains(logged, expected) {
			t.Errorf("metadata logs missing %q:\n%s", expected, logged)
		}
	}
	for _, forbidden := range []string{
		`"member_input"`,
		`"system_prompt"`,
		`"user_prompt"`,
		`"tool_schemas"`,
		`"request":`,
		`"response":`,
		`"input":`,
		`"result":`,
		`"final_reply"`,
		"Can you check now?",
		"neutral model policy and local knowledge",
		"Eco is online now.",
	} {
		if strings.Contains(logged, forbidden) {
			t.Errorf("metadata logs contain %q:\n%s", forbidden, logged)
		}
	}
}
