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
	"reflect"
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
			Identity:      "Sirens Deep of Coilyco",
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
	if strings.Contains(agent.systemPrompt, "Do not adopt or express a personality") {
		t.Fatal("HTTP-only agent did not load the social profile")
	}
	if !strings.Contains(agent.systemPrompt, "CoilyCo general-purpose response rules") {
		t.Fatal("HTTP-only agent did not load its local voice policy")
	}
	// "Sirens" is no longer a leak signal: it is in this profile's own identity
	// and in the harness name. The community profile's surface still is.
	for _, forbidden := range []string{"Eco", "Sirens Echo", "#bots", "Forgejo"} {
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
		cfg:   Config{DiscordChannelIDs: []string{channelID}},
		scope: newChannelScope(16),
	}
	agent.ensureRuntimeDefaults()
	inScope := func(channel string) bool {
		origin := summonContext{
			Kind:      contextKindGuild,
			GuildID:   guildID,
			ChannelID: channel,
		}
		decision := agent.access.Evaluate(origin, "member-1", nil, nil)
		if decision.Reason != accessNeedsThreadRef {
			return decision.allowed()
		}
		if cached, known := agent.scope.Get(channel); known {
			return cached
		}
		return agent.resolveScope(session, origin, decision.Guild)
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
	} {
		if got := inScope(testCase.id); got != testCase.allowed {
			t.Fatalf("%s: inScope = %v, want %v", testCase.name, got, testCase.allowed)
		}
	}
	if _, known := agent.scope.Get(threadID); !known {
		t.Fatal("thread scope decision was not cached")
	}
	if _, known := agent.scope.Get(foreignID); !known {
		t.Fatal("denied thread was not cached, so every message would repeat the lookup")
	}
	if _, known := agent.scope.Get(channelID); known {
		t.Fatal("configured channel took the resolver path")
	}
}

func TestSummonContextKeySharesOneBudgetPerGuild(t *testing.T) {
	t.Parallel()
	first := summonContext{Kind: contextKindGuild, GuildID: "g1", ChannelID: "c1"}
	second := summonContext{Kind: contextKindGuild, GuildID: "g1", ChannelID: "c2"}
	other := summonContext{Kind: contextKindGuild, GuildID: "g2", ChannelID: "c1"}
	if first.Key() != second.Key() {
		t.Fatalf("channels in one guild must share a key, got %q and %q", first.Key(), second.Key())
	}
	if first.Key() == other.Key() {
		t.Fatal("separate guilds must not share one budget")
	}
	dm := summonContext{Kind: contextKindDM, ChannelID: "c1"}
	if dm.Key() == other.Key() {
		t.Fatal("a direct message must not share a guild's budget")
	}
}

func TestDirectMessagesSummonWithoutAMention(t *testing.T) {
	t.Parallel()
	const botID = "bot-1"
	state := discordgo.NewState()
	state.User = &discordgo.User{ID: botID}
	session := &discordgo.Session{State: state}
	author := &discordgo.User{ID: "member-1"}

	summoned, lookup := summonedBool(summonedLocally(session, &discordgo.Message{
		ChannelID: "dm-1",
		Author:    author,
	}))
	if !summoned || lookup {
		t.Fatalf(
			"a direct message must summon without a mention, got summoned=%v lookup=%v",
			summoned, lookup,
		)
	}

	// The guild path keeps its mention gate, which is what keeps a busy
	// channel quiet.
	summoned, lookup = summonedBool(summonedLocally(session, &discordgo.Message{
		GuildID:   "guild-1",
		ChannelID: "channel-1",
		Author:    author,
	}))
	if summoned || lookup {
		t.Fatalf(
			"an unmentioned guild message must not summon, got summoned=%v lookup=%v",
			summoned, lookup,
		)
	}
}

func TestDiscordMessageSpanAttributesUseStringIdentifiers(t *testing.T) {
	t.Parallel()
	at := discordLocation{GuildID: "guild", ChannelID: "channel"}
	got := make(map[string]string)
	for _, item := range discordMessageSpanAttributes("process", at, "message") {
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
	// Compared whole, because a subset check cannot see an identifier that
	// nobody meant to export. See AGENTS.md on what a turn retains.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("attributes = %v, want %v", got, want)
	}

	withoutMessage := discordMessageSpanAttributes("send", at, "")
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
			"manual-request": {Content: `Echo is ready.`},
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

// turnAgent builds an agent whose model always succeeds, so HTTP edge tests
// measure the edge rather than the completion path.
func turnAgent(cfg Config) *Agent {
	cfg.Definition.MaxContextMessages = 12
	return &Agent{
		cfg: cfg,
		completions: fakeCompletionClient{responses: map[string]CompletionResult{
			"manual-request": {Content: `Echo is ready.`},
		}},
		systemPrompt: "neutral model policy and local knowledge",
		telemetry:    telemetryOrNoop(nil),
		slots:        make(chan struct{}, 1),
	}
}

func turnRequest() *http.Request {
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/turn",
		strings.NewReader(`{"request_id":"manual-request","author":"tester","content":"Are you ready?"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	return request
}

func TestHTTPTurnAppliesTheAdmissionPolicy(t *testing.T) {
	t.Parallel()
	agent := turnAgent(Config{
		RateLimit: RateLimitPolicy{PerUser: RateLimit{Burst: 1, Every: time.Hour}},
	})
	handler := agent.HTTPHandler()

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, turnRequest())
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, turnRequest())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", second.Code, http.StatusTooManyRequests)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Fatal("a rate-limited response must tell the caller when to retry")
	}

	// A distinct caller has its own budget rather than inheriting the denial.
	distinct := turnRequest()
	distinct.Header.Set("X-Sirens-Caller", "second-client")
	third := httptest.NewRecorder()
	handler.ServeHTTP(third, distinct)
	if third.Code != http.StatusOK {
		t.Fatalf("distinct caller status = %d, body = %s", third.Code, third.Body.String())
	}
}

// An accepted turn must return its queue slot, or the deployment stops
// answering after MaxPending requests have merely completed.
func TestHTTPTurnReleasesItsQueueSlot(t *testing.T) {
	t.Parallel()
	agent := turnAgent(Config{RateLimit: RateLimitPolicy{MaxPending: 1}})
	handler := agent.HTTPHandler()

	for attempt := 0; attempt < 4; attempt++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, turnRequest())
		if recorder.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, body = %s", attempt, recorder.Code, recorder.Body.String())
		}
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
			"remote-request": {Content: `Echo is ready.`},
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

type fixtureTurn struct {
	requestID string
	requester string
	history   []TranscriptEntry
	current   TranscriptEntry
	reply     string
}

func (t *fixtureTurn) RequestID() string {
	return t.requestID
}

func (t *fixtureTurn) Requester() string {
	if t.requester == "" {
		return "318190481467244544"
	}
	return t.requester
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
) (ToolResult, error) {
	if name != "eco__get_eco_server_status" {
		return ToolResult{}, fmt.Errorf("unexpected tool %q", name)
	}
	if len(arguments) != 0 {
		return ToolResult{}, fmt.Errorf("unexpected arguments %#v", arguments)
	}
	return ToolResult{Text: "online"}, nil
}

func (fixtureToolSession) Grounding() []GroundingDocument {
	return nil
}

func (fixtureToolSession) Guidance() []ServerGuidance { return nil }

func (fixtureToolSession) Unavailable() []string {
	return nil
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
			if len(body.Messages) != 4 {
				t.Fatalf("messages = %#v", body.Messages)
			}
			turnContext, ok := body.Messages[2].Content.(string)
			if !ok {
				t.Errorf("turn context type = %T", body.Messages[2].Content)
			}
			for _, expected := range []string{
				"first member: Is Eco online?",
				"Sirens Echo: I can check if you summon me.",
				"The request that follows is from current member.",
			} {
				if !strings.Contains(turnContext, expected) {
					t.Errorf("context missing %q:\n%s", expected, turnContext)
				}
			}
			// The last user message is exactly what the member typed, which is
			// what agentproxy.user_message reads downstream. See #104.
			if got := body.Messages[3].Content; got != "Can you check now?" {
				t.Errorf("final user message = %#v", got)
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
				`{"choices":[{"message":{"content":"Eco is online now."}}]}`,
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
	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	// The turn called a tool, so the reply carries its disclosure footer. This
	// is the end-to-end proof it reaches a member. See sirens-echo#385.
	wantReply := "Eco is online now.\n\n> 🔨 ✅ `eco.get_eco_server_status`"
	if turn.reply != wantReply {
		t.Fatalf("reply = %q, want %q", turn.reply, wantReply)
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
		`"reply_bytes":58`,
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

// Issue 88: the harness header asserted discord for every deployment, so an
// HTTP-only profile reported a Discord surface it does not have.
func TestDeploymentHarnessFollowsTheConfiguredIngress(t *testing.T) {
	t.Parallel()
	if got := deploymentHarness(Config{DiscordEnabled: true}); got != transportDiscord {
		t.Errorf("Discord deployment harness = %q, want %q", got, transportDiscord)
	}
	if got := deploymentHarness(Config{DiscordEnabled: false}); got != transportHTTP {
		t.Errorf("HTTP-only deployment harness = %q, want %q", got, transportHTTP)
	}
}

func editSession(botID string) *discordgo.Session {
	state := discordgo.NewState()
	state.User = &discordgo.User{ID: botID}
	return &discordgo.Session{State: state}
}

func editEvent(message *discordgo.Message) *discordgo.MessageUpdate {
	return &discordgo.MessageUpdate{Message: message}
}

// An edit that newly names the service is the whole point of the handler.
func TestEditSummonsOnNewMention(t *testing.T) {
	t.Parallel()
	const botID = "bot-1"
	edited := time.Now()
	event := editEvent(&discordgo.Message{
		GuildID:         "guild-1",
		ChannelID:       "channel-1",
		Author:          &discordgo.User{ID: "member-1"},
		Mentions:        []*discordgo.User{{ID: botID}},
		EditedTimestamp: &edited,
	})
	if !editSummons(editSession(botID), event) {
		t.Fatal("an edit that adds the mention must summon")
	}
}

// Discord emits the same event when it resolves a link preview. That carries no
// edit timestamp and must stay silent even though the mention is present.
func TestEditIgnoresLinkUnfurl(t *testing.T) {
	t.Parallel()
	const botID = "bot-1"
	event := editEvent(&discordgo.Message{
		GuildID:   "guild-1",
		ChannelID: "channel-1",
		Author:    &discordgo.User{ID: "member-1"},
		Mentions:  []*discordgo.User{{ID: botID}},
	})
	if editSummons(editSession(botID), event) {
		t.Fatal("a link unfurl must not summon")
	}
}

// A partial payload without a guild id must not fall through to the direct
// message path, which summons without a mention and uses a different policy.
func TestEditIgnoresMissingGuild(t *testing.T) {
	t.Parallel()
	const botID = "bot-1"
	edited := time.Now()
	event := editEvent(&discordgo.Message{
		ChannelID:       "channel-1",
		Author:          &discordgo.User{ID: "member-1"},
		Mentions:        []*discordgo.User{{ID: botID}},
		EditedTimestamp: &edited,
	})
	if editSummons(editSession(botID), event) {
		t.Fatal("an edit without a guild id must not summon")
	}
}

func TestEditIgnoresUnmentionedAndEmptyEvents(t *testing.T) {
	t.Parallel()
	const botID = "bot-1"
	session := editSession(botID)
	edited := time.Now()
	unmentioned := editEvent(&discordgo.Message{
		GuildID:         "guild-1",
		ChannelID:       "channel-1",
		Author:          &discordgo.User{ID: "member-1"},
		Mentions:        []*discordgo.User{{ID: "someone-else"}},
		EditedTimestamp: &edited,
	})
	if editSummons(session, unmentioned) {
		t.Fatal("an edit naming someone else must not summon")
	}
	if editSummons(session, nil) || editSummons(session, editEvent(nil)) {
		t.Fatal("an empty update must not summon")
	}
}

// Replying to one of this service's own messages is a summon without a mention.
func TestReplyToServiceMessageSummons(t *testing.T) {
	t.Parallel()
	const botID = "bot-1"
	session := editSession(botID)
	author := &discordgo.User{ID: "member-1"}

	summoned, lookup := summonedBool(summonedLocally(session, &discordgo.Message{
		GuildID:           "guild-1",
		ChannelID:         "channel-1",
		Author:            author,
		ReferencedMessage: &discordgo.Message{Author: &discordgo.User{ID: botID}},
	}))
	if !summoned || lookup {
		t.Fatalf("a reply to the service must summon, got summoned=%v lookup=%v", summoned, lookup)
	}

	// A reply aimed at another member is not a summon.
	summoned, lookup = summonedBool(summonedLocally(session, &discordgo.Message{
		GuildID:           "guild-1",
		ChannelID:         "channel-1",
		Author:            author,
		ReferencedMessage: &discordgo.Message{Author: &discordgo.User{ID: "member-2"}},
	}))
	if summoned || lookup {
		t.Fatalf("a reply to a member must not summon, got summoned=%v lookup=%v", summoned, lookup)
	}

	// An unresolved reference defers to the rate-limited lookup rather than
	// guessing either way.
	summoned, lookup = summonedBool(summonedLocally(session, &discordgo.Message{
		GuildID:          "guild-1",
		ChannelID:        "channel-1",
		Author:           author,
		MessageReference: &discordgo.MessageReference{MessageID: "m-9"},
	}))
	if summoned || !lookup {
		t.Fatalf("an unresolved reply must defer to lookup, got summoned=%v lookup=%v", summoned, lookup)
	}
}
