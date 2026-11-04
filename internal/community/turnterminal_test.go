package community

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// An accepted mention ends in exactly one logged outcome, so a count of accepted
// turns with no terminal event can be taken and should be zero. See COI-3278.

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// terminalAgent is an agent whose log lines the test reads back.
func terminalAgent(t *testing.T, client CompletionClient) (*Agent, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	telemetry, err := newTelemetry(
		slog.New(slog.NewJSONHandler(logs, nil)),
		provider,
		metricnoop.NewMeterProvider(),
	)
	if err != nil {
		t.Fatalf("telemetry: %v", err)
	}
	agent := &Agent{
		cfg:          Config{Definition: Definition{MaxContextMessages: 12}},
		completions:  client,
		systemPrompt: "neutral model policy and local knowledge",
		telemetry:    telemetry,
		slots:        make(chan struct{}, 1),
	}
	agent.ensureRuntimeDefaults()
	return agent, logs
}

// terminals returns the terminal field of every turn.terminal line, in order.
func terminals(t *testing.T, logs *syncBuffer) []string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(logs.String(), "\n") {
		if !strings.Contains(line, `"turn.terminal"`) {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("log line is not JSON: %v: %s", err, line)
		}
		if record["msg"] != "turn.terminal" {
			continue
		}
		found = append(found, record["terminal"].(string))
	}
	return found
}

func wantOneTerminal(t *testing.T, logs *syncBuffer, want string) {
	t.Helper()
	got := terminals(t, logs)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("turn.terminal = %q, want exactly [%q]", got, want)
	}
}

type failingClient struct{ err error }

func (c failingClient) Complete(context.Context, TurnPrompt, string) (CompletionResult, error) {
	return CompletionResult{}, c.err
}

type panickingClient struct{}

func (panickingClient) Complete(context.Context, TurnPrompt, string) (CompletionResult, error) {
	panic("model client crashed")
}

// unreachableTurn is a transport whose every send fails, so a failure notice cannot land.
type unreachableTurn struct{ *markableTurn }

func (*unreachableTurn) Reply(context.Context, string) error {
	return errors.New("discord refused the send")
}

func TestADeliveredReplyIsTheTerminalEvent(t *testing.T) {
	t.Parallel()
	agent, logs := terminalAgent(t, answeringClient{reply: "iron is 3 coins"})
	turn := &markableTurn{}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	wantOneTerminal(t, logs, terminalReplyDelivered)
}

func TestAReactionAsTheWholeAnswerIsNamedAndNotCountedAsAReply(t *testing.T) {
	t.Parallel()
	agent, logs := terminalAgent(t, answeringClient{reply: "{{react:celebrate}}"})
	turn := &markableTurn{}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if len(turn.replies) != 0 {
		t.Fatalf("a mark answer also posted %q", turn.replies)
	}
	wantOneTerminal(t, logs, terminalReactionOnly)
}

func TestSilenceAfterAWriteToolIsNamedAsChosen(t *testing.T) {
	t.Parallel()
	agent, logs := terminalAgent(t, answeredThroughToolClient{})
	turn := &markableTurn{}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	wantOneTerminal(t, logs, terminalChosenSilence)
}

// A failure notice goes out through the send that records a delivered reply, and
// the failure has to be the ending that stands.
func TestAFailedTurnWhoseNoticeLandedIsNotCountedAsADeliveredReply(t *testing.T) {
	t.Parallel()
	agent, logs := terminalAgent(t, failingClient{err: errors.New("backend returned 500")})
	turn := &markableTurn{}

	if err := agent.runTurn(context.Background(), turn, nil); err == nil {
		t.Fatal("a turn whose model call failed returned no error")
	}
	if len(turn.replies) == 0 {
		t.Fatal("the member was told nothing about the failure")
	}
	wantOneTerminal(t, logs, terminalFailureNotice)
}

func TestAFailedTurnWhoseNoticeAlsoFailedIsCountedAsUnnotified(t *testing.T) {
	t.Parallel()
	agent, logs := terminalAgent(t, failingClient{err: errors.New("backend returned 500")})

	if err := agent.runTurn(context.Background(), &unreachableTurn{&markableTurn{}}, nil); err == nil {
		t.Fatal("a turn that failed twice returned no error")
	}
	wantOneTerminal(t, logs, terminalFailureUnnotified)
}

// The drop path: a reply that assembled to nothing used to be "ok" and show the
// member nothing. It is a failure the member is told about now.
func TestABlankReplyEndsInAVisibleFailure(t *testing.T) {
	t.Parallel()
	agent, logs := terminalAgent(t, answeringClient{})
	turn := &markableTurn{}
	ctx, ending := withTurnEnding(context.Background())

	err := agent.deliverOrReport(ctx, turn, "  \n ", nothingWithheld)

	if !errors.Is(err, ErrReplySilent) {
		t.Fatalf("a blank reply returned %v, want a failure that names ErrReplySilent", err)
	}
	if len(turn.replies) != 1 || strings.TrimSpace(turn.replies[0]) == "" {
		t.Fatalf("the member saw %q, want one visible failure notice", turn.replies)
	}
	if got := ending.get(); got != terminalFailureNotice {
		t.Errorf("ending = %q, want %q", got, terminalFailureNotice)
	}
	if !strings.Contains(logs.String(), `"turn.reply.blank"`) {
		t.Error("the blank was not logged as turn.reply.blank")
	}
}

func TestSendReplyRefusesABlankInsteadOfSendingOrSucceeding(t *testing.T) {
	t.Parallel()
	agent, _ := terminalAgent(t, answeringClient{})
	turn := &markableTurn{}

	err := agent.sendReply(context.Background(), turn, "   ", nothingWithheld)

	if !errors.Is(err, ErrReplySilent) {
		t.Fatalf("sendReply of a blank returned %v, want ErrReplySilent", err)
	}
	if len(turn.replies) != 0 {
		t.Errorf("sendReply posted %q for a blank", turn.replies)
	}
}

// A panic recorded as ok before. It is its own ending now, and still reaches
// handleMessage, which contains it and sends the crash notice.
func TestAPanickingTurnIsNamedAndStillPropagates(t *testing.T) {
	t.Parallel()
	agent, logs := terminalAgent(t, panickingClient{})
	turn := &markableTurn{}

	recovered := func() (recovered any) {
		defer func() { recovered = recover() }()
		_ = agent.runTurn(context.Background(), turn, nil)
		return nil
	}()

	if recovered == nil {
		t.Fatal("the panic was swallowed, so handleMessage would never send the crash notice")
	}
	wantOneTerminal(t, logs, terminalPanicked)
}

func TestEveryEndingIsLoggedExactlyOncePerTurn(t *testing.T) {
	t.Parallel()
	agent, logs := terminalAgent(t, answeringClient{reply: "ok"})
	for range 3 {
		if err := agent.runTurn(context.Background(), &markableTurn{}, nil); err != nil {
			t.Fatalf("runTurn: %v", err)
		}
	}
	if got := terminals(t, logs); len(got) != 3 {
		t.Fatalf("3 turns logged %d terminal events: %q", len(got), got)
	}
}
