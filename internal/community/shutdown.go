package community

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// A Discord turn runs in its own goroutine, so shutdown has to be told about it
// to wait for it. See docs/sirens-echo-execution.md.

// errShuttingDown is the cancellation cause a drained turn carries. Without it
// the turn sees context.Canceled and cannot say what ended it.
var errShuttingDown = errors.New("the service is shutting down")

// drainState holds the turns in flight and the context they are cancelled from.
// Its zero value is a running service, so a hand-constructed Agent needs no setup.
type drainState struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelCauseFunc
	// closing is guarded by mu, so no Add can race the Wait that follows begin.
	closing  bool
	inFlight sync.WaitGroup
}

// root is the context every Discord turn descends from. Built on first use
// rather than at construction, which is what keeps the zero value working.
func (d *drainState) root() context.Context {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ctx == nil {
		d.ctx, d.cancel = context.WithCancelCause(context.Background())
	}
	return d.ctx
}

// enter admits one turn, or refuses it because the service is draining. Every
// admitted turn owes a leave, or shutdown waits out its whole grace for it.
func (d *drainState) enter() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closing {
		return false
	}
	d.inFlight.Add(1)
	return true
}

// leave releases a turn's hold on shutdown.
func (d *drainState) leave() { d.inFlight.Done() }

// begin stops admitting. It returns only once no enter is mid-flight, so the
// counter cannot rise after this point.
func (d *drainState) begin() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closing = true
}

// draining reports whether shutdown has started.
func (d *drainState) draining() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closing
}

// wait gives the turns in flight until the grace period to answer on their own,
// and reports whether they all did.
func (d *drainState) wait(grace time.Duration) bool {
	settled := make(chan struct{})
	go func() {
		d.inFlight.Wait()
		close(settled)
	}()
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-settled:
		return true
	case <-timer.C:
		return false
	}
}

// stop cancels whatever is still running, naming the restart as the cause so
// the notice a member reads is true.
func (d *drainState) stop() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()
	// A service that never took a turn built no root and has nothing to cancel.
	if cancel != nil {
		cancel(errShuttingDown)
	}
}

// drainTurns stops taking work, gives the turns in flight a bounded chance to
// answer, then cancels the rest so they can say why they stopped.
func (a *Agent) drainTurns(ctx context.Context, httpServer *http.Server) error {
	a.drain.begin()
	// Closed first, so what the queue already holds still becomes batches. Each
	// ask keeps a drain slot until it is answered, which is what waits below.
	if a.lane != nil {
		a.lane.stop()
	}
	grace := a.cfg.ShutdownGrace
	if grace <= 0 {
		grace = defaultShutdownGrace
	}
	// Detached, because the context that reached here is the cancelled signal
	// and a drain bounded by it would be over before it started.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	httpDone := make(chan error, 1)
	go func() { httpDone <- httpServer.Shutdown(shutdownCtx) }()

	settled := a.drain.wait(grace)
	if !settled {
		a.drain.stop()
		// The gateway is still open here, which is the only reason the notice
		// those turns are about to send can reach anybody.
		settled = a.drain.wait(shutdownNoticeGrace)
	}
	httpErr := <-httpDone

	a.telemetry.Info(
		context.WithoutCancel(ctx),
		"shutdown.drained",
		slog.Bool("turns_settled", settled),
		slog.Duration("grace", grace),
	)
	if errors.Is(httpErr, context.DeadlineExceeded) {
		// Connections that outlived the grace are cut, which is the expected end
		// of a roll. Returned, it would reach main as a crash and page Sentry.
		a.telemetry.Warn(
			context.WithoutCancel(ctx),
			"shutdown.http.deadline",
			slog.Duration("grace", grace),
		)
		_ = httpServer.Close()
		return nil
	}
	if httpErr != nil {
		return fmt.Errorf("HTTP shutdown: %w", httpErr)
	}
	return nil
}
