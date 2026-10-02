package community

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
	"go.opentelemetry.io/otel/attribute"
)

// slackEvent is one summon as Slack delivered it, before any gate has run.
type slackEvent struct {
	Team     string
	Channel  string
	User     string
	TS       string
	ThreadTS string
	Text     string
	// Direct is a message.im event rather than an app_mention.
	Direct  bool
	BotID   string
	SubType string
	Edited  bool
}

// slackSource delivers Slack events to the transport. The real one speaks
// Socket Mode, which needs no public endpoint.
type slackSource interface {
	// Start connects and delivers until stop runs. It fails when the
	// connection cannot be made, so a bad token stops the process.
	Start(ctx context.Context, deliver func(slackEvent)) (stop func(), err error)
}

// slackTransport is Slack as a Transport. It admits an event through the same
// limiter and drain the Discord path uses, then runs the shared turn.
type slackTransport struct {
	agent  *Agent
	api    slackAPI
	source slackSource
	policy *SlackAccessPolicy
	names  *slackNames
	seen   *seenMessages
	bot    slackIdentity
}

func newSlackTransport(
	agent *Agent, api slackAPI, source slackSource, policy *SlackAccessPolicy,
) *slackTransport {
	return &slackTransport{
		agent:  agent,
		api:    api,
		source: source,
		policy: policy,
		names:  newSlackNames(api),
		seen:   newSeenMessages(1024),
	}
}

func (*slackTransport) Name() string { return transportSlack }

// Start proves the bot token and that its workspace is in the policy, then
// connects. A policy that can never match would otherwise fail silent.
func (t *slackTransport) Start(ctx context.Context) (func(), error) {
	identity, err := t.api.AuthTest(ctx)
	if err != nil {
		return nil, fmt.Errorf("slack auth: %w", err)
	}
	if _, listed := t.policy.byWorkspace[identity.TeamID]; !listed {
		return nil, fmt.Errorf("the bot's workspace is not in the slack access policy")
	}
	t.bot = identity
	stop, err := t.source.Start(ctx, t.deliver)
	if err != nil {
		return nil, fmt.Errorf("slack connect: %w", err)
	}
	t.agent.telemetry.Info(ctx, "slack.ready", slog.String("audit_role", t.agent.cfg.Definition.AuditRole))
	return stop, nil
}

// eligible rejects what can never be a summon. Bot-authored messages never are,
// so there is no agent-to-agent exchange on Slack, and an edit is not new.
func (t *slackTransport) eligible(event slackEvent) bool {
	return event.Channel != "" && event.TS != "" && event.User != "" &&
		event.BotID == "" && event.SubType == "" && !event.Edited &&
		event.User != t.bot.UserID
}

// deliver gates one event in cost order and runs the turn off the event loop.
func (t *slackTransport) deliver(event slackEvent) {
	if !t.eligible(event) {
		return
	}
	a, ctx := t.agent, context.Background()
	reason, workspace := t.policy.Evaluate(event.Team, event.Channel, event.User, event.Direct)
	if reason != accessAllowed {
		a.telemetry.RecordAccess(ctx, string(reason))
		return
	}
	if !t.seen.Add(event.Channel + ":" + event.TS) {
		return
	}
	kind, summon := contextKindGuild, summonMentioned
	if event.Direct {
		kind, summon = contextKindDM, summonDirect
	}
	a.telemetry.RecordSummon(ctx, string(summon), kind)
	if !a.drain.enter() {
		a.telemetry.RecordAccess(ctx, string(accessDeniedDraining))
		return
	}
	a.telemetry.RecordAccess(ctx, string(accessAllowed))
	go func() {
		defer a.drain.leave()
		t.handle(event, workspace.Overrides())
	}()
}

func (t *slackTransport) turnFor(event slackEvent) *slackTurn {
	return &slackTurn{
		api:       t.api,
		names:     t.names,
		botUserID: t.bot.UserID,
		team:      event.Team,
		channel:   event.Channel,
		user:      event.User,
		ts:        event.TS,
		threadTS:  event.ThreadTS,
		text:      event.Text,
		direct:    event.Direct,
		limit:     t.agent.cfg.Definition.MaxContextMessages,
	}
}

func (t *slackTransport) handle(event slackEvent, override *RateLimitOverride) {
	a, turn := t.agent, t.turnFor(event)
	defer func() {
		if recovered := recover(); recovered != nil {
			a.telemetry.RecordFailure(context.Background(), "panic")
			a.telemetry.Error(context.Background(), "slack.turn.panicked",
				slog.String("error_type", "turn_panicked"))
			_ = a.notifyFailure(context.Background(), turn, noticeTurnCrashed)
		}
	}()
	contextKey := "slack:" + event.Team
	if event.Direct {
		contextKey = "slack:" + event.Channel
	}
	decision := a.limiter.Admit(admissionRequest{
		UserKey:    "slack:" + event.Team + ":" + event.User,
		ContextKey: contextKey,
		Queued:     true,
		Override:   override,
	})
	if decision.Outcome.denied() {
		t.onDenied(turn, decision)
		return
	}
	defer a.limiter.Release()
	a.telemetry.RecordAdmission(context.Background(), string(admissionAccepted), transportSlack)
	receiveCtx, receiveSpan := a.telemetry.StartSpan(
		a.drain.root(), "slack.receive",
		append(turn.SpanAttributes(), attribute.String("slack.message.ts", event.TS))...,
	)
	defer receiveSpan.End()
	if err := a.runSerialized(receiveCtx, turn); err != nil {
		a.telemetry.MarkSpanError(receiveSpan, exceptionTurnFailed)
		a.telemetry.Error(receiveCtx, "slack.turn.failed", append(
			[]slog.Attr{slog.String("error_type", "turn_failed")},
			turnFailureAttrs(err)...,
		)...)
	}
}

// onDenied marks a refused summon and tells the member at most once per notify
// window, so a flooder gains no amplifier.
func (t *slackTransport) onDenied(turn *slackTurn, decision admissionDecision) {
	a, ctx := t.agent, context.Background()
	a.telemetry.RecordAdmission(ctx, string(decision.Outcome), transportSlack)
	a.telemetry.Info(ctx, "turn.input.denied",
		slog.String("transport", transportSlack),
		slog.String("outcome", string(decision.Outcome)),
		slog.Bool("notified", decision.Notify),
	)
	a.react(ctx, turn, reactionRefused)
	if !decision.Notify {
		return
	}
	if err := turn.Reply(ctx, noticeWithTrace(ctx, cooldownNotice(decision.RetryAfter))); err != nil {
		a.telemetry.RecordFailure(ctx, "reply")
	}
}

// slackSocketSource is Socket Mode over slack-go.
type slackSocketSource struct {
	client *slack.Client
}

func (s slackSocketSource) Start(
	ctx context.Context, deliver func(slackEvent),
) (func(), error) {
	socket := socketmode.New(s.client)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	// A fatal auth error comes back from RunContext without an event, so the
	// return value is what reports a rejected token.
	ended := make(chan error, 1)
	go func() {
		defer close(done)
		ended <- socket.RunContext(runCtx)
	}()
	stop := func() {
		cancel()
		<-done
	}
	connected := make(chan error, 1)
	go func() {
		first := true
		for {
			select {
			case <-runCtx.Done():
				return
			case event := <-socket.Events:
				switch event.Type {
				case socketmode.EventTypeConnected:
					if first {
						first = false
						connected <- nil
					}
				case socketmode.EventTypeInvalidAuth, socketmode.EventTypeConnectionError:
					if first {
						first = false
						connected <- fmt.Errorf("socket mode %s", event.Type)
					}
				case socketmode.EventTypeEventsAPI:
					handleSocketEvent(socket, event, deliver)
				}
			}
		}
	}()
	select {
	case err := <-ended:
		stop()
		if err == nil {
			err = fmt.Errorf("socket mode ended before it connected")
		}
		return nil, err
	case err := <-connected:
		if err != nil {
			stop()
			return nil, err
		}
		return stop, nil
	case <-time.After(slackConnectWait):
		stop()
		return nil, fmt.Errorf("no connection within %s", slackConnectWait)
	case <-ctx.Done():
		stop()
		return nil, ctx.Err()
	}
}

// handleSocketEvent acknowledges the envelope first, before any gate or turn,
// then hands the event to the transport.
func handleSocketEvent(socket *socketmode.Client, event socketmode.Event, deliver func(slackEvent)) {
	apiEvent, ok := event.Data.(slackevents.EventsAPIEvent)
	if event.Request != nil {
		_ = socket.Ack(*event.Request)
	}
	if !ok {
		return
	}
	if mapped, ok := slackEventFrom(apiEvent); ok {
		deliver(mapped)
	}
}

// slackEventFrom maps a callback to the two events a summon can arrive as:
// an app_mention in a channel, and a message.im direct message.
func slackEventFrom(apiEvent slackevents.EventsAPIEvent) (slackEvent, bool) {
	if apiEvent.Type != slackevents.CallbackEvent {
		return slackEvent{}, false
	}
	switch inner := apiEvent.InnerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		return slackEvent{
			Team: apiEvent.TeamID, Channel: inner.Channel, User: inner.User,
			TS: inner.TimeStamp, ThreadTS: inner.ThreadTimeStamp, Text: inner.Text,
			BotID: inner.BotID, Edited: inner.Edited != nil,
		}, true
	case *slackevents.MessageEvent:
		if inner.ChannelType != "im" {
			return slackEvent{}, false
		}
		return slackEvent{
			Team: apiEvent.TeamID, Channel: inner.Channel, User: inner.User,
			TS: inner.TimeStamp, ThreadTS: inner.ThreadTimeStamp, Text: inner.Text,
			Direct: true, BotID: inner.BotID, SubType: inner.SubType,
		}, true
	}
	return slackEvent{}, false
}

var _ Transport = (*slackTransport)(nil)
