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
	"go.opentelemetry.io/otel/codes"
)

const genericFailureReply = "I could not produce a grounded answer right now. Please try again shortly."

// Agent owns the Sirens Echo Discord session and its outbound boundaries.
type Agent struct {
	cfg          Config
	session      *discordgo.Session
	completions  CompletionClient
	issues       IssueTracker
	systemPrompt string
	bundle       AOSBundle
	telemetry    *Telemetry
	slots        chan struct{}
	seen         *seenMessages
}

// NewAgent builds the independently deployable Sirens Echo runtime.
func NewAgent(cfg Config, telemetry *Telemetry) (*Agent, error) {
	bundle, err := LoadAOSBundle(cfg.AOSBundlePath, cfg.Definition.Role)
	if err != nil {
		return nil, err
	}
	localSkillpack, err := LoadSkillpack(cfg.Definition.LocalSkillRoots)
	if err != nil {
		return nil, err
	}
	session, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return nil, fmt.Errorf("discord session: %w", err)
	}
	session.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent
	telemetry = telemetryOrNoop(telemetry)
	httpClient := &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: otelhttp.NewTransport(
			http.DefaultTransport,
		),
	}
	agent := &Agent{
		cfg:     cfg,
		session: session,
		completions: ProxyClient{
			BaseURL:    cfg.AgentProxyURL,
			Route:      cfg.AgentProxyRoute,
			Intent:     cfg.Intent,
			Harness:    cfg.Definition.Harness,
			HTTPClient: httpClient,
			Tools: MCPProvider{
				Servers:    cfg.Definition.MCPServers,
				HTTPClient: httpClient,
			},
			Telemetry: telemetry,
		},
		issues: ForgejoClient{
			BaseURL:    cfg.ForgejoBaseURL,
			Token:      cfg.ForgejoToken,
			Owner:      cfg.TrackerOwner,
			Repo:       cfg.TrackerRepo,
			HTTPClient: httpClient,
		},
		systemPrompt: BuildSystemPrompt(cfg.Definition, bundle.Context, localSkillpack),
		bundle:       bundle,
		telemetry:    telemetry,
		slots:        make(chan struct{}, 1),
		seen:         newSeenMessages(1024),
	}
	session.AddHandler(agent.onReady)
	session.AddHandler(agent.onMessage)
	return agent, nil
}

// Run opens the Gateway session and blocks until shutdown.
func (a *Agent) Run(ctx context.Context) error {
	if err := a.session.Open(); err != nil {
		return fmt.Errorf("Discord open: %w", err)
	}
	defer a.session.Close()
	<-ctx.Done()
	return nil
}

func (a *Agent) onReady(_ *discordgo.Session, ready *discordgo.Ready) {
	a.telemetry.Info(
		context.Background(),
		"discord.ready",
		slog.String("identity", a.cfg.Definition.Identity),
		slog.String("discord_user", ready.User.Username),
		slog.String("channel", a.cfg.Definition.Channel),
		slog.String("aos_role", a.bundle.Manifest.Role),
		slog.Any("aos_personalities", a.bundle.Manifest.Personalities),
		slog.Any("aos_sources", a.bundle.Manifest.Sources),
	)
}

func (a *Agent) onMessage(session *discordgo.Session, event *discordgo.MessageCreate) {
	message := event.Message
	if message == nil ||
		message.GuildID == "" ||
		message.ChannelID != a.cfg.DiscordChannelID ||
		message.Author == nil ||
		message.Author.Bot ||
		session.State.User == nil ||
		message.Author.ID == session.State.User.ID {
		return
	}
	if !a.isSummoned(session, message) || !a.seen.Add(message.ID) {
		return
	}
	go a.handleMessage(session, message)
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
	a.slots <- struct{}{}
	defer func() { <-a.slots }()

	_ = session.ChannelTyping(message.ChannelID)
	ctx, cancel := context.WithTimeout(context.Background(), a.cfg.RequestTimeout)
	defer cancel()
	turn := &discordMessageTurn{
		session: session,
		message: message,
		limit:   a.cfg.Definition.MaxContextMessages,
	}
	if err := a.runTurn(ctx, turn); err != nil {
		a.telemetry.Error(ctx, "discord.turn.failed", slog.String("error", err.Error()))
	}
}

type turnIO interface {
	RequestID() string
	Current() TranscriptEntry
	History(ctx context.Context) ([]TranscriptEntry, error)
	Reply(ctx context.Context, content string) error
}

func (a *Agent) runTurn(ctx context.Context, turn turnIO) (turnErr error) {
	started := time.Now()
	turnCtx, turnSpan := a.telemetry.StartSpan(
		ctx,
		"discord.turn",
		attribute.String("discord.channel", a.cfg.Definition.Channel),
		attribute.String("agent.role", a.cfg.Definition.Role),
	)
	outcome := "ok"
	defer func() {
		if turnErr != nil {
			outcome = "error"
			turnSpan.RecordError(turnErr)
			turnSpan.SetStatus(codes.Error, turnErr.Error())
		}
		a.telemetry.RecordTurn(turnCtx, outcome, time.Since(started))
		turnSpan.End()
	}()

	receiveCtx, receiveSpan := a.telemetry.StartSpan(turnCtx, "discord.receive")
	current := turn.Current()
	a.telemetry.Info(
		receiveCtx,
		"discord.input",
		slog.String("member_input", current.Content),
	)
	receiveSpan.SetAttributes(attribute.Int("discord.message.bytes", len(current.Content)))
	receiveSpan.End()

	historyCtx, historySpan := a.telemetry.StartSpan(turnCtx, "discord.history")
	history, err := turn.History(historyCtx)
	if err != nil {
		historySpan.RecordError(err)
		historySpan.SetStatus(codes.Error, err.Error())
		historySpan.End()
		return a.failTurn(turnCtx, turn, "history", err)
	}
	historySpan.SetAttributes(attribute.Int("discord.history.count", len(history)))
	historySpan.End()

	contextCtx, contextSpan := a.telemetry.StartSpan(turnCtx, "context.assemble")
	userPrompt := BuildUserPrompt(history, current)
	_, bundleSpan := a.telemetry.StartSpan(contextCtx, "aos.bundle.load")
	systemPrompt := a.systemPrompt
	bundleSpan.SetAttributes(
		attribute.String("aos.bundle.role", a.bundle.Manifest.Role),
		attribute.Int("aos.bundle.personality.count", len(a.bundle.Manifest.Personalities)),
		attribute.Int("aos.bundle.source.count", len(a.bundle.Manifest.Sources)),
	)
	bundleSpan.End()
	a.telemetry.Info(
		contextCtx,
		"context.rendered",
		slog.String("system_prompt", systemPrompt),
		slog.String("user_prompt", userPrompt),
	)
	contextSpan.SetAttributes(
		attribute.Int("discord.history.count", len(history)),
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
		err = ValidateGrounding(decision, systemPrompt+"\n"+userPrompt)
	}
	if err != nil {
		validateSpan.RecordError(err)
		validateSpan.SetStatus(codes.Error, err.Error())
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
				slog.String("error", issueErr.Error()),
			)
			issueSpan.RecordError(issueErr)
			issueSpan.SetStatus(codes.Error, issueErr.Error())
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
		"discord.turn.stage.failed",
		slog.String("stage", stage),
		slog.String("error", cause.Error()),
	)
	replyErr := a.sendReply(ctx, turn, genericFailureReply)
	return errors.Join(cause, replyErr)
}

func (a *Agent) sendReply(ctx context.Context, turn turnIO, content string) error {
	replyCtx, replySpan := a.telemetry.StartSpan(ctx, "discord.reply")
	a.telemetry.Info(
		replyCtx,
		"discord.reply",
		slog.String("final_reply", content),
	)
	err := turn.Reply(replyCtx, content)
	if err != nil {
		a.telemetry.RecordFailure(replyCtx, "reply")
		replySpan.RecordError(err)
		replySpan.SetStatus(codes.Error, err.Error())
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

func (t *discordMessageTurn) Reply(_ context.Context, content string) error {
	_, err := t.session.ChannelMessageSendComplex(t.message.ChannelID, &discordgo.MessageSend{
		Content:   truncateRunes(content, 1990),
		Reference: t.message.SoftReference(),
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse:       []discordgo.AllowedMentionType{},
			RepliedUser: false,
		},
	})
	return err
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
