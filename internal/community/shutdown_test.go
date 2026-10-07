package community

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

// A restart used to end a Discord turn by process exit, so the member kept an
// eyes mark and never got an answer. See sirens-echo#597.

// The headline. A turn the restart cancelled says so, rather than blaming the
// model backend it was in the middle of calling.
func TestADrainedTurnTellsTheMemberTheServiceIsRestarting(t *testing.T) {
	t.Parallel()
	agent := failingAgent(errors.New("unused, the drain ends this turn"))
	root := agent.drain.root()
	turnCtx, cancelTurn := context.WithTimeout(root, time.Minute)
	defer cancelTurn()
	agent.drain.stop()

	turn := &httpTurn{requestID: "drained-turn"}
	err := agent.failTurn(turnCtx, turn, stageModel, context.Canceled)

	if turn.reply != noticeShuttingDown {
		t.Errorf("reply = %q, want %q", turn.reply, noticeShuttingDown)
	}
	if !errors.Is(err, errShuttingDown) {
		t.Errorf("failTurn error = %v, want the restart named", err)
	}
}

// The cause has to survive the turn's own deadline wrapper, which is the layer
// that made context.Canceled the only thing the notice could see.
func TestTheRestartCauseReachesThroughTheTurnDeadline(t *testing.T) {
	t.Parallel()
	var drain drainState
	turnCtx, cancel := context.WithTimeout(drain.root(), time.Hour)
	defer cancel()

	drain.stop()

	if err := turnCtx.Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("turn context error = %v, want it cancelled", err)
	}
	if cause := context.Cause(turnCtx); !errors.Is(cause, errShuttingDown) {
		t.Errorf("cause = %v, want the restart", cause)
	}
}

// A turn whose own deadline fires first is not a restart, or every timeout
// during a quiet hour would be reported as a deploy.
func TestATurnThatExpiresOnItsOwnIsNotARestart(t *testing.T) {
	t.Parallel()
	var drain drainState
	turnCtx, cancel := context.WithTimeout(drain.root(), time.Millisecond)
	defer cancel()
	<-turnCtx.Done()

	if cause := context.Cause(turnCtx); !errors.Is(cause, context.DeadlineExceeded) {
		t.Errorf("cause = %v, want the turn's own deadline", cause)
	}
	if got := turnFailureNotice(stageModel, context.DeadlineExceeded); got != noticeTimedOut {
		t.Errorf("notice = %q, want %q", got, noticeTimedOut)
	}
}

// The point of the grace period. A turn already running holds the drain open
// until it answers, rather than being ended by the process going away.
func TestShutdownWaitsForATurnThatIsStillRunning(t *testing.T) {
	t.Parallel()
	var drain drainState
	if !drain.enter() {
		t.Fatal("a running service refused a turn")
	}

	if drain.wait(20 * time.Millisecond) {
		t.Fatal("the drain settled while a turn was still running")
	}
	drain.leave()
	if !drain.wait(time.Second) {
		t.Error("the drain did not settle after the turn finished")
	}
}

// Draining refuses new work. Without this the wait can be extended forever by
// arrivals, and a rolling deploy never completes.
func TestADrainingServiceTakesNoNewTurns(t *testing.T) {
	t.Parallel()
	var drain drainState
	if !drain.enter() {
		t.Fatal("a running service refused a turn")
	}
	drain.begin()

	if drain.enter() {
		t.Error("a draining service admitted a new turn")
	}
	if !drain.draining() {
		t.Error("draining reported a running service")
	}
	// The one already running is unaffected, which is the whole distinction.
	if drain.wait(20 * time.Millisecond) {
		t.Fatal("the drain abandoned a turn it had already admitted")
	}
	drain.leave()
	if !drain.wait(time.Second) {
		t.Error("the drain did not settle after the admitted turn finished")
	}
}

// Every test and several callers build an Agent by hand, so a drain that needed
// construction would panic on the first message rather than at startup.
func TestTheZeroDrainStateIsARunningService(t *testing.T) {
	t.Parallel()
	agent := &Agent{telemetry: telemetryOrNoop(nil)}

	if !agent.drain.enter() {
		t.Fatal("a hand-constructed Agent refused its first turn")
	}
	if agent.drain.root() == nil {
		t.Error("the turn root was nil")
	}
	agent.drain.leave()
}

// A service that never took a turn has no root to cancel, and shutdown must not
// panic on the way past that.
func TestStoppingAServiceThatNeverTookATurnIsQuiet(t *testing.T) {
	t.Parallel()
	var drain drainState
	drain.begin()
	drain.stop()

	if !drain.wait(time.Second) {
		t.Error("an idle service did not settle")
	}
}

// The failure series has to separate a deploy from a defect, or a rollout
// reads as an outage. See docs/sirens-echo-execution.md.
func TestARestartIsNotCountedAsAStageFailure(t *testing.T) {
	t.Parallel()
	if got := failureCause(errShuttingDown); got != causeShutdown {
		t.Errorf("cause label = %q, want %q", got, causeShutdown)
	}
	if !noticeShape.MatchString(noticeShuttingDown) {
		t.Errorf("notice %q does not match the harness shape", noticeShuttingDown)
	}
}

// A connection outliving the grace is cut and warned about, never returned:
// returned, it reached main as a crash and filed SIRENS-ECHO-1.
func TestAConnectionOutlivingTheGraceIsAWarningNotACrash(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	telemetry, err := newTelemetry(
		slog.New(slog.NewJSONHandler(&logs, nil)),
		tracenoop.NewTracerProvider(),
		metricnoop.NewMeterProvider(),
	)
	if err != nil {
		t.Fatalf("newTelemetry: %v", err)
	}
	agent := &Agent{cfg: Config{ShutdownGrace: 50 * time.Millisecond}, telemetry: telemetry}

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.Serve(listener) }()
	go func() {
		if resp, err := http.Get("http://" + listener.Addr().String()); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started

	if err := agent.drainTurns(context.Background(), server); err != nil {
		t.Fatalf("drainTurns = %v, want nil so main does not report a crash", err)
	}
	out := logs.String()
	if !strings.Contains(out, `"msg":"shutdown.http.deadline"`) || !strings.Contains(out, `"level":"WARN"`) {
		t.Errorf("logs = %s, want shutdown.http.deadline at WARN", out)
	}
}
