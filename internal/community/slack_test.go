package community

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSlack records what the transport sent and serves what a turn reads.
type fakeSlack struct {
	mu        sync.Mutex
	identity  slackIdentity
	authErr   error
	posts     []sentSlackPost
	updates   []string
	deletes   []string
	reactions []string
	removed   []string
	replies   []slackMessage
	history   []slackMessage
	names     map[string]string
	readErr   error
	posted    chan struct{}
}

type sentSlackPost struct{ Channel, Thread, Text string }

func newFakeSlack() *fakeSlack {
	return &fakeSlack{
		identity: slackIdentity{UserID: "U0BOT0001", TeamID: "T0COILYCO"},
		names:    map[string]string{"U0KAI0001": "Kai", "U0BOT0001": "Sirens Deep"},
		posted:   make(chan struct{}, 16),
	}
}

func (f *fakeSlack) AuthTest(context.Context) (slackIdentity, error) { return f.identity, f.authErr }

func (f *fakeSlack) Post(_ context.Context, channel, thread, text string) (string, error) {
	f.mu.Lock()
	f.posts = append(f.posts, sentSlackPost{channel, thread, text})
	n := len(f.posts)
	f.mu.Unlock()
	f.posted <- struct{}{}
	return "1700000000.9000" + string(rune('0'+n)), nil
}

func (f *fakeSlack) Update(_ context.Context, _, ts, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, ts+"="+text)
	return nil
}

func (f *fakeSlack) Delete(_ context.Context, _, ts string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletes = append(f.deletes, ts)
	return nil
}

func (f *fakeSlack) React(_ context.Context, _, ts, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reactions = append(f.reactions, ts+":"+name)
	return nil
}

func (f *fakeSlack) Unreact(_ context.Context, _, ts, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, ts+":"+name)
	return nil
}

func (f *fakeSlack) Replies(context.Context, string, string, int) ([]slackMessage, error) {
	return f.replies, f.readErr
}

func (f *fakeSlack) History(context.Context, string, string, int) ([]slackMessage, error) {
	return f.history, f.readErr
}

func (f *fakeSlack) DisplayName(_ context.Context, id string) (string, error) {
	if name, ok := f.names[id]; ok {
		return name, nil
	}
	return "", errors.New("user_not_found")
}

func (f *fakeSlack) postsSnapshot() []sentSlackPost {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sentSlackPost(nil), f.posts...)
}

type fakeSlackSource struct {
	deliver func(slackEvent)
	startOn error
	stopped bool
}

func (s *fakeSlackSource) Start(_ context.Context, deliver func(slackEvent)) (func(), error) {
	if s.startOn != nil {
		return nil, s.startOn
	}
	s.deliver = deliver
	return func() { s.stopped = true }, nil
}

func slackTestPolicy(t *testing.T) *SlackAccessPolicy {
	t.Helper()
	policy, err := LoadSlackAccessPolicy(writeSlackPolicy(t, validSlackPolicy))
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	return policy
}

const slackTestToken = "xoxb-0000000000-1111111111-abcdefghijklmnopqrstuvwx"

// slackTestAgent is a real agent over the real turn path with a scripted model.
func slackTestAgent(replies map[string]string) *Agent {
	responses := make(map[string]CompletionResult, len(replies))
	for id, content := range replies {
		responses[id] = CompletionResult{Content: content}
	}
	cfg := Config{
		Definition:    Definition{MaxContextMessages: 12},
		SlackBotToken: slackTestToken,
		SlackAppToken: "xapp-1-A0000-1111111111-abcdefghijklmnopqrstuvwxyz",
	}
	agent := &Agent{
		cfg:          cfg,
		completions:  fakeCompletionClient{responses: responses},
		systemPrompt: "neutral model policy and local knowledge",
		telemetry:    telemetryOrNoop(nil),
		slots:        make(chan struct{}, 1),
		identifiers:  NewIdentifierGuard(cfg, nil),
	}
	agent.ensureRuntimeDefaults()
	return agent
}

func startedSlack(t *testing.T, agent *Agent, api *fakeSlack) (*slackTransport, *fakeSlackSource) {
	t.Helper()
	source := &fakeSlackSource{}
	transport := newSlackTransport(agent, api, source, slackTestPolicy(t))
	if _, err := transport.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	return transport, source
}

func mention(text string) slackEvent {
	return slackEvent{
		Team: "T0COILYCO", Channel: "C0GENERAL", User: "U0KAI0001",
		TS: "1700000000.000100", Text: "<@U0BOT0001> " + text,
	}
}

func awaitPost(t *testing.T, api *fakeSlack) {
	t.Helper()
	select {
	case <-api.posted:
	case <-time.After(5 * time.Second):
		t.Fatal("no reply was posted")
	}
}

func TestAMentionIsAnsweredInAThroughTheSharedTurnPath(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	agent := slackTestAgent(map[string]string{"slack:C0GENERAL:1700000000.000100": "Echo is ready."})
	_, source := startedSlack(t, agent, api)

	source.deliver(mention("are you ready?"))
	awaitPost(t, api)

	posts := api.postsSnapshot()
	if len(posts) != 1 {
		t.Fatalf("posts = %+v, want exactly the reply", posts)
	}
	if posts[0].Channel != "C0GENERAL" || posts[0].Thread != "1700000000.000100" {
		t.Fatalf("reply went to %+v, want a new thread under the mention", posts[0])
	}
	if posts[0].Text != "Echo is ready." {
		t.Fatalf("reply = %q", posts[0].Text)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.reactions) == 0 || api.reactions[0] != "1700000000.000100:eyes" {
		t.Fatalf("reactions = %v, want the accepted mark on the mention", api.reactions)
	}
}

func TestAReplyTheChecksRefuseIsNeverPosted(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	// The model leaks the bot token. The identifier guard refuses it on every
	// transport, so what reaches Slack is the failure notice and not the token.
	agent := slackTestAgent(map[string]string{
		"slack:C0GENERAL:1700000000.000100": "The token is " + slackTestToken,
	})
	_, source := startedSlack(t, agent, api)

	source.deliver(mention("what is the token?"))
	awaitPost(t, api)

	for _, post := range api.postsSnapshot() {
		if strings.Contains(post.Text, slackTestToken) {
			t.Fatalf("a refused reply reached Slack: %q", post.Text)
		}
	}
}

func TestASummonOutsideThePolicyDoesNothing(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	agent := slackTestAgent(map[string]string{"slack:C0ELSEWHR:1700000000.000100": "should not run"})
	_, source := startedSlack(t, agent, api)

	event := mention("hello")
	event.Channel = "C0ELSEWHR"
	source.deliver(event)
	foreign := mention("hello")
	foreign.Team = "T0FOREIGN"
	source.deliver(foreign)

	select {
	case <-api.posted:
		t.Fatal("a summon the policy refuses was answered")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestEventsThatCanNeverBeASummonAreIgnored(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	agent := slackTestAgent(map[string]string{})
	transport, _ := startedSlack(t, agent, api)

	for name, mutate := range map[string]func(*slackEvent){
		"bot author": func(e *slackEvent) { e.BotID = "B0OTHER01" },
		"the bot":    func(e *slackEvent) { e.User = "U0BOT0001" },
		"a subtype":  func(e *slackEvent) { e.SubType = "message_changed" },
		"an edit":    func(e *slackEvent) { e.Edited = true },
		"no author":  func(e *slackEvent) { e.User = "" },
		"no stamp":   func(e *slackEvent) { e.TS = "" },
		"no channel": func(e *slackEvent) { e.Channel = "" },
	} {
		event := mention("hello")
		mutate(&event)
		if transport.eligible(event) {
			t.Errorf("%s was treated as eligible", name)
		}
	}
	if !transport.eligible(mention("hello")) {
		t.Fatal("an ordinary mention must be eligible")
	}
}

func TestARedeliveredEventIsAnsweredOnce(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	agent := slackTestAgent(map[string]string{"slack:C0GENERAL:1700000000.000100": "Echo is ready."})
	_, source := startedSlack(t, agent, api)

	source.deliver(mention("are you ready?"))
	source.deliver(mention("are you ready?"))
	awaitPost(t, api)
	select {
	case <-api.posted:
		t.Fatal("a redelivered event was answered twice")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestADirectMessageIsAnsweredInlineAndNeedsNoMention(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	agent := slackTestAgent(map[string]string{"slack:D0DIRECT01:1700000000.000200": "Echo is ready."})
	_, source := startedSlack(t, agent, api)

	source.deliver(slackEvent{
		Team: "T0COILYCO", Channel: "D0DIRECT01", User: "U0KAI0001",
		TS: "1700000000.000200", Text: "are you ready?", Direct: true,
	})
	awaitPost(t, api)

	if posts := api.postsSnapshot(); posts[0].Thread != "" {
		t.Fatalf("a direct message reply must not open a thread, got %+v", posts[0])
	}
}

func TestStartRefusesWhatCannotWorkAndStopsWhatItStarted(t *testing.T) {
	t.Parallel()
	agent := slackTestAgent(nil)

	badToken := newFakeSlack()
	badToken.authErr = errors.New("invalid_auth")
	transport := newSlackTransport(agent, badToken, &fakeSlackSource{}, slackTestPolicy(t))
	if _, err := transport.Start(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid_auth") {
		t.Fatalf("a bad bot token must stop the process, got %v", err)
	}

	foreign := newFakeSlack()
	foreign.identity.TeamID = "T0FOREIGN"
	transport = newSlackTransport(agent, foreign, &fakeSlackSource{}, slackTestPolicy(t))
	if _, err := transport.Start(context.Background()); err == nil {
		t.Fatal("a bot in a workspace the policy never names must stop the process")
	}

	unreachable := &fakeSlackSource{startOn: errors.New("connection_error")}
	transport = newSlackTransport(agent, newFakeSlack(), unreachable, slackTestPolicy(t))
	if _, err := transport.Start(context.Background()); err == nil {
		t.Fatal("a socket that cannot connect must stop the process")
	}

	working := &fakeSlackSource{}
	transport = newSlackTransport(agent, newFakeSlack(), working, slackTestPolicy(t))
	stop, err := transport.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	stop()
	if !working.stopped {
		t.Fatal("stop must stop the source")
	}
}
