package community

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const genericFailureReply = "there was an error generating your reply"

const (
	transportDiscord = "discord"
	transportHTTP    = "http"
)

// Agent owns the Sirens Echo Discord session and its outbound boundaries.
type Agent struct {
	cfg               Config
	session           *discordgo.Session
	completions       CompletionClient
	issues            IssueTracker
	systemPrompt      string
	telemetry         *Telemetry
	readinessClient   *http.Client
	readinessEndpoint string
	readinessRoute    string
	readinessTimeout  time.Duration
	slots             chan struct{}
	seen              *seenMessages
	scope             *channelScope
}

// NewAgent builds the independently deployable Sirens Echo runtime.
func NewAgent(cfg Config, telemetry *Telemetry) (*Agent, error) {
	readinessEndpoint, err := agentProxyReadinessEndpoint(
		cfg.AgentProxyURL,
		cfg.AgentProxyModel,
	)
	if err != nil {
		return nil, err
	}
	localSkillpack, err := LoadSkillpack(cfg.Definition.LocalSkillRoots)
	if err != nil {
		return nil, err
	}
	systemPrompt := BuildSystemPrompt(cfg.Definition, localSkillpack)
	if err := ValidateSystemPrompt(cfg.Definition, systemPrompt); err != nil {
		return nil, err
	}
	var session *discordgo.Session
	if cfg.DiscordEnabled {
		session, err = discordgo.New("Bot " + cfg.DiscordToken)
		if err != nil {
			return nil, fmt.Errorf("discord session: %w", err)
		}
		session.Identify.Intents = discordgo.IntentsGuilds |
			discordgo.IntentsGuildMessages |
			discordgo.IntentsMessageContent
	}
	telemetry = telemetryOrNoop(telemetry)
	httpClient := &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: otelhttp.NewTransport(
			http.DefaultTransport,
			otelhttp.WithTracerProvider(telemetry.traceProvider),
			otelhttp.WithPropagators(telemetry.propagator),
		),
	}
	var issueTracker IssueTracker
	if cfg.Definition.IssueTracker != "" {
		trackerURL, trackerErr := mcpServerURL(
			cfg.Definition,
			cfg.Definition.IssueTracker,
		)
		if trackerErr != nil {
			return nil, trackerErr
		}
		issueTracker = ForgejoMCPClient{
			MCPURL:     trackerURL,
			HTTPClient: httpClient,
		}
	}
	agent := &Agent{
		cfg:     cfg,
		session: session,
		completions: ProxyClient{
			BaseURL:       cfg.AgentProxyURL,
			Model:         cfg.AgentProxyModel,
			AuditRole:     cfg.Definition.AuditRole,
			Attribution:   cfg.Definition.Identity,
			ResponseStyle: cfg.Definition.ResponseStyle,
			HTTPClient:    httpClient,
			Tools: MCPProvider{
				Servers:    cfg.Definition.MCPServers,
				HTTPClient: httpClient,
			},
			Telemetry: telemetry,
		},
		issues:            issueTracker,
		systemPrompt:      systemPrompt,
		telemetry:         telemetry,
		readinessClient:   newReadinessHTTPClient(defaultReadinessTimeout),
		readinessEndpoint: readinessEndpoint,
		readinessRoute:    cfg.AgentProxyModel,
		readinessTimeout:  defaultReadinessTimeout,
		slots:             make(chan struct{}, 1),
		seen:              newSeenMessages(1024),
		scope:             newChannelScope(256),
	}
	if session != nil {
		session.AddHandler(agent.onReady)
		session.AddHandler(agent.onMessage)
	}
	return agent, nil
}

func mcpServerURL(definition Definition, name string) (string, error) {
	for _, server := range definition.MCPServers {
		if server.Name == name && server.URL != "" {
			return server.URL, nil
		}
	}
	return "", fmt.Errorf("agent definition requires resolved MCP server %q", name)
}

// Run opens the Gateway session and blocks until shutdown.
func (a *Agent) Run(ctx context.Context) error {
	if a.session != nil {
		if err := a.session.Open(); err != nil {
			return fmt.Errorf("Discord open: %w", err)
		}
		defer a.session.Close()
	}
	httpServer := &http.Server{
		Addr:              a.cfg.HTTPListenAddr,
		Handler:           a.HTTPHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	httpErr := make(chan error, 1)
	go func() {
		httpErr <- httpServer.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP shutdown: %w", err)
		}
		return nil
	case err := <-httpErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP serve: %w", err)
	}
}

func (a *Agent) onReady(_ *discordgo.Session, ready *discordgo.Ready) {
	a.telemetry.Info(
		context.Background(),
		"discord.ready",
		slog.String("identity", a.cfg.Definition.Identity),
		slog.String("discord_user", ready.User.Username),
		slog.String("channel", a.cfg.Definition.Channel),
		slog.String("audit_role", a.cfg.Definition.AuditRole),
	)
}

func (a *Agent) onMessage(session *discordgo.Session, event *discordgo.MessageCreate) {
	message := event.Message
	if message == nil ||
		message.GuildID == "" ||
		message.Author == nil ||
		message.Author.Bot ||
		session.State == nil ||
		session.State.User == nil ||
		message.Author.ID == session.State.User.ID {
		return
	}
	if !a.inScope(session, message.ChannelID) {
		return
	}
	if !a.isSummoned(session, message) || !a.seen.Add(message.ID) {
		return
	}
	go a.handleMessage(session, message)
}

func (a *Agent) inScope(session *discordgo.Session, channelID string) bool {
	if channelID == "" {
		return false
	}
	if channelID == a.cfg.DiscordChannelID {
		return true
	}
	if cached, known := a.scope.Get(channelID); known {
		return cached
	}
	allowed := false
	if channel := resolveChannel(session, channelID); channel != nil {
		allowed = channel.IsThread() && channel.ParentID == a.cfg.DiscordChannelID
		a.scope.Set(channelID, allowed)
	}
	return allowed
}

func resolveChannel(session *discordgo.Session, channelID string) *discordgo.Channel {
	if session.State != nil {
		if channel, err := session.State.Channel(channelID); err == nil && channel != nil {
			return channel
		}
	}
	channel, err := session.Channel(channelID)
	if err != nil {
		return nil
	}
	return channel
}

func (a *Agent) isSummoned(session *discordgo.Session, message *discordgo.Message) bool {
	botID := session.State.User.ID
	for _, mention := range message.Mentions {
		if mention.ID == botID {
			return true
		}
	}
	if message.ReferencedMessage != nil && message.ReferencedMessage.Author != nil {
		return message.ReferencedMessage.Author.ID == botID
	}
	if message.MessageReference == nil || message.MessageReference.MessageID == "" {
		return false
	}
	referenced, err := session.ChannelMessage(message.ChannelID, message.MessageReference.MessageID)
	return err == nil && referenced.Author != nil && referenced.Author.ID == botID
}

func (a *Agent) handleMessage(session *discordgo.Session, message *discordgo.Message) {
	_ = session.ChannelTyping(message.ChannelID)
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.RequestTimeout)
	defer cancel()
	receiveCtx, receiveSpan := a.telemetry.StartSpan(
		ctx,
		"discord.receive",
		discordMessageSpanAttributes(
			"process",
			message.GuildID,
			message.ChannelID,
			message.ID,
		)...,
	)
	defer receiveSpan.End()
	turn := &discordMessageTurn{
		session: session,
		message: message,
		limit:   a.cfg.Definition.MaxContextMessages,
	}
	if err := a.runSerialized(receiveCtx, turn); err != nil {
		a.telemetry.MarkSpanError(receiveSpan, exceptionTurnFailed)
		a.telemetry.Error(receiveCtx, "discord.turn.failed", slog.String("error_type", "turn_failed"))
	}
}

func (a *Agent) runSerialized(ctx context.Context, turn turnIO) error {
	a.slots <- struct{}{}
	defer func() { <-a.slots }()
	return a.runTurn(ctx, turn)
}

type turnIO interface {
	RequestID() string
	Transport() string
	Current() TranscriptEntry
	History(ctx context.Context) ([]TranscriptEntry, error)
	Reply(ctx context.Context, content string) error
}

func (a *Agent) runTurn(ctx context.Context, turn turnIO) (turnErr error) {
	started := time.Now()
	turnCtx, turnSpan := a.telemetry.StartSpan(
		ctx,
		"community.turn",
		attribute.String("sirens_echo.transport", turn.Transport()),
		attribute.String("sirens_echo.request_id", turn.RequestID()),
		attribute.String("agent.role", a.cfg.Definition.AuditRole),
	)
	if turn.Transport() == transportDiscord {
		turnSpan.SetAttributes(attribute.String("discord.channel", a.cfg.Definition.Channel))
	}
	outcome := "ok"
	defer func() {
		if turnErr != nil {
			outcome = "error"
			a.telemetry.MarkSpanError(turnSpan, exceptionTurnFailed)
		}
		a.telemetry.RecordTurn(turnCtx, outcome, time.Since(started))
		turnSpan.End()
	}()

	receiveCtx, receiveSpan := a.telemetry.StartSpan(turnCtx, "community.input")
	current := turn.Current()
	a.telemetry.Info(
		receiveCtx,
		"turn.input.accepted",
		slog.String("transport", turn.Transport()),
		slog.String("request_id", turn.RequestID()),
		slog.Int("input_bytes", len(current.Content)),
	)
	receiveSpan.SetAttributes(attribute.Int("input.bytes", len(current.Content)))
	receiveSpan.End()

	historyCtx, historySpan := a.telemetry.StartSpan(turnCtx, "community.history")
	history, err := turn.History(historyCtx)
	if err != nil {
		a.telemetry.MarkSpanError(historySpan, exceptionHistoryFailed)
		historySpan.End()
		return a.failTurn(turnCtx, turn, "history", err)
	}
	historySpan.SetAttributes(attribute.Int("history.count", len(history)))
	historySpan.End()

	contextCtx, contextSpan := a.telemetry.StartSpan(turnCtx, "context.assemble")
	userPrompt := BuildUserPrompt(history, current)
	systemPrompt := a.systemPrompt
	a.telemetry.Info(
		contextCtx,
		"context.rendered",
		slog.Int("history_count", len(history)),
		slog.Int("system_prompt_bytes", len(systemPrompt)),
		slog.Int("user_prompt_bytes", len(userPrompt)),
	)
	contextSpan.SetAttributes(
		attribute.Int("history.count", len(history)),
		attribute.Int("prompt.system.bytes", len(systemPrompt)),
		attribute.Int("prompt.user.bytes", len(userPrompt)),
	)
	contextSpan.End()

	result, err := a.completions.Complete(
		turnCtx,
		systemPrompt,
		userPrompt,
		turn.RequestID(),
	)
	if err != nil {
		return a.failTurn(turnCtx, turn, "model", err)
	}

	_, validateSpan := a.telemetry.StartSpan(turnCtx, "response.validate")
	decision, err := ParseDecision(result.Content)
	if err == nil {
		err = ValidateGrounding(decision, systemPrompt+"\n"+userPrompt, result.ToolCalls...)
	}
	if err == nil {
		err = ValidateResponseStyle(a.cfg.Definition.ResponseStyle, decision)
	}
	if err == nil && decision.Issue != nil && a.issues == nil {
		err = fmt.Errorf(
			"model returned an issue draft while automatic issue tracking is disabled",
		)
	}
	if err != nil {
		a.telemetry.MarkSpanError(validateSpan, exceptionResponseValidationFailed)
		validateSpan.End()
		return a.failTurn(turnCtx, turn, "validation", err)
	}
	validateSpan.End()

	reply := decision.Reply
	if decision.Issue != nil {
		issueCtx, issueSpan := a.telemetry.StartSpan(turnCtx, "forgejo.issue.ensure")
		issueURL, issueErr := a.issues.EnsureIssue(issueCtx, *decision.Issue)
		if issueErr != nil {
			a.telemetry.RecordFailure(issueCtx, "forgejo")
			a.telemetry.Error(
				issueCtx,
				"forgejo.issue.failed",
				slog.String("error_type", "forgejo_issue_failed"),
			)
			a.telemetry.MarkSpanError(issueSpan, exceptionForgejoIssueFailed)
		} else {
			reply = appendTrackingLink(reply, issueURL)
		}
		issueSpan.End()
	}
	if err := a.sendReply(turnCtx, turn, reply); err != nil {
		return err
	}
	return nil
}

func (a *Agent) failTurn(
	ctx context.Context,
	turn turnIO,
	stage string,
	cause error,
) error {
	a.telemetry.RecordFailure(ctx, stage)
	a.telemetry.Error(
		ctx,
		"turn.stage.failed",
		slog.String("stage", stage),
		slog.String("error_type", stage+"_failed"),
	)
	replyErr := a.sendReply(ctx, turn, genericFailureReply)
	return errors.Join(cause, replyErr)
}

func (a *Agent) sendReply(ctx context.Context, turn turnIO, content string) error {
	replyCtx, replySpan := a.telemetry.StartSpan(ctx, "community.reply")
	a.telemetry.Info(
		replyCtx,
		"turn.reply.ready",
		slog.String("transport", turn.Transport()),
		slog.Int("reply_bytes", len(content)),
	)
	var err error
	if turn.Transport() == transportDiscord {
		discordCtx, discordSpan := a.telemetry.StartSpan(replyCtx, "discord.reply")
		err = turn.Reply(discordCtx, content)
		if err != nil {
			a.telemetry.MarkSpanError(discordSpan, exceptionDiscordReplyFailed)
		}
		discordSpan.End()
	} else {
		err = turn.Reply(replyCtx, content)
	}
	if err != nil {
		a.telemetry.RecordFailure(replyCtx, "reply")
		a.telemetry.MarkSpanError(replySpan, exceptionReplyFailed)
	}
	replySpan.End()
	return err
}

type discordMessageTurn struct {
	session *discordgo.Session
	message *discordgo.Message
	limit   int
}

func (t *discordMessageTurn) RequestID() string {
	return t.message.ID
}

func (t *discordMessageTurn) Transport() string { return transportDiscord }

func (t *discordMessageTurn) Current() TranscriptEntry {
	return TranscriptEntry{
		Author:  displayName(t.message),
		Content: t.message.ContentWithMentionsReplaced(),
	}
}

func (t *discordMessageTurn) History(_ context.Context) ([]TranscriptEntry, error) {
	messages, err := t.session.ChannelMessages(
		t.message.ChannelID,
		t.limit,
		t.message.ID,
		"",
		"",
	)
	if err != nil {
		return nil, err
	}
	history := make([]TranscriptEntry, 0, len(messages))
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Author == nil {
			continue
		}
		history = append(history, TranscriptEntry{
			Author:  displayName(message),
			Content: message.ContentWithMentionsReplaced(),
		})
	}
	return history, nil
}

func (t *discordMessageTurn) Reply(ctx context.Context, content string) error {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(discordMessageSpanAttributes(
		"send",
		t.message.GuildID,
		t.message.ChannelID,
		"",
	)...)
	reply, err := t.session.ChannelMessageSendComplex(t.message.ChannelID, &discordgo.MessageSend{
		Content:   truncateRunes(content, 1990),
		Reference: t.message.SoftReference(),
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: false,
		},
	})
	if reply != nil && reply.ID != "" {
		span.SetAttributes(attribute.String("messaging.message.id", reply.ID))
	}
	return err
}

func discordMessageSpanAttributes(
	operation, guildID, channelID, messageID string,
) []attribute.KeyValue {
	attributes := []attribute.KeyValue{
		attribute.String("messaging.system", "discord"),
		attribute.String("messaging.operation.name", operation),
		attribute.String("messaging.operation.type", operation),
		attribute.String("discord.guild.id", guildID),
		attribute.String("discord.channel.id", channelID),
	}
	if messageID != "" {
		attributes = append(
			attributes,
			attribute.String("messaging.message.id", messageID),
		)
	}
	return attributes
}

func displayName(message *discordgo.Message) string {
	if message.Member != nil && strings.TrimSpace(message.Member.Nick) != "" {
		return message.Member.Nick
	}
	if message.Author != nil {
		if strings.TrimSpace(message.Author.GlobalName) != "" {
			return message.Author.GlobalName
		}
		return message.Author.Username
	}
	return "member"
}

func appendTrackingLink(reply, issueURL string) string {
	suffix := "\n\nTracked for review: <" + issueURL + ">"
	return truncateRunes(reply, 1990-len([]rune(suffix))) + suffix
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

type channelScope struct {
	mu       sync.Mutex
	capacity int
	order    []string
	values   map[string]bool
}

func newChannelScope(capacity int) *channelScope {
	return &channelScope{
		capacity: capacity,
		order:    make([]string, 0, capacity),
		values:   make(map[string]bool, capacity),
	}
}

func (c *channelScope) Get(id string) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	allowed, known := c.values[id]
	return allowed, known
}

func (c *channelScope) Set(id string, allowed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.values[id]; exists {
		return
	}
	c.values[id] = allowed
	c.order = append(c.order, id)
	if len(c.order) > c.capacity {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.values, oldest)
	}
}

type seenMessages struct {
	mu       sync.Mutex
	capacity int
	order    []string
	values   map[string]struct{}
}

func newSeenMessages(capacity int) *seenMessages {
	return &seenMessages{
		capacity: capacity,
		order:    make([]string, 0, capacity),
		values:   make(map[string]struct{}, capacity),
	}
}

func (s *seenMessages) Add(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.values[id]; exists {
		return false
	}
	s.values[id] = struct{}{}
	s.order = append(s.order, id)
	if len(s.order) > s.capacity {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.values, oldest)
	}
	return true
}
