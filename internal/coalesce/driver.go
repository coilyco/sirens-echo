package coalesce

import (
	"context"
	"time"

	"github.com/coilyco/sirens-echo/internal/ingest"
)

// Source is the ask buffer the coalescer drains, declared as a shape so the
// window can be driven by a test feeder as well as by the real queue.
type Source interface {
	TryNext() (ingest.Ask, bool)
	Signal() <-chan struct{}
	Done() <-chan struct{}
	Depth() int
}

// Coalescer moves asks from the queue through the window and emits batches.
// Every timing rule it applies belongs to Window, which is what tests exercise.
type Coalescer struct {
	window *Window
	source Source
	out    chan Batch
	clock  func() time.Time
}

// NewCoalescer wires a coalescer onto a source. The batch channel is buffered
// by one window's worth, so a flush does not block on a busy pool immediately.
func NewCoalescer(policy Policy, source Source, clock func() time.Time) *Coalescer {
	resolved := policy.withDefaults()
	return &Coalescer{
		window: NewWindow(resolved, source.Depth),
		source: source,
		out:    make(chan Batch, resolved.WideBatch),
		clock:  clock,
	}
}

// Batches is the channel the pool drains. It closes when the coalescer stops,
// which is what lets the pool's wait function return.
func (c *Coalescer) Batches() <-chan Batch { return c.out }

func (c *Coalescer) now() time.Time {
	if c.clock != nil {
		return c.clock().UTC()
	}
	return time.Now().UTC()
}

// Run drives the window until the context ends or the source closes and
// drains. Whatever the window still holds is flushed before it returns.
func (c *Coalescer) Run(ctx context.Context) {
	defer close(c.out)
	// Never drained. Since Go 1.23 the channel is unbuffered, so the old
	// `if !Stop() { <-C }` idiom blocks forever and Reset needs no drain.
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	for {
		c.fill(ctx)
		deadline, open := c.window.Deadline()
		if !open {
			if !c.idle(ctx) {
				c.emit(ctx, c.window.Flush())
				return
			}
			continue
		}
		wait := deadline.Sub(c.now())
		if wait <= 0 {
			c.emit(ctx, c.window.Flush())
			continue
		}
		timer.Reset(wait)
		select {
		case <-c.source.Signal():
			timer.Stop()
		case <-timer.C:
		case <-ctx.Done():
			c.emit(context.WithoutCancel(ctx), c.window.Flush())
			return
		}
	}
}

// fill drains everything already queued into the window, flushing whenever K
// is reached, so a burst becomes whole batches rather than one oversized one.
func (c *Coalescer) fill(ctx context.Context) {
	for {
		ask, ok := c.source.TryNext()
		if !ok {
			return
		}
		c.window.Offer(ask)
		if c.window.Due(c.now()) {
			c.emit(ctx, c.window.Flush())
		}
	}
}

// idle waits for the next ask when no window is open. It reports false when
// nothing further is coming, which is the source closing or the context ending.
func (c *Coalescer) idle(ctx context.Context) bool {
	select {
	case <-c.source.Signal():
		return true
	case <-c.source.Done():
		// Re-checked rather than trusted, so a close racing a push still hands
		// the pushed ask to a batch.
		return c.source.Depth() > 0
	case <-ctx.Done():
		return false
	}
}

// emit hands batches to the pool in flush order. A saturated pool blocks here,
// which is the backpressure the queue's own bound absorbs on the ingress side.
func (c *Coalescer) emit(ctx context.Context, batches []Batch) {
	for _, batch := range batches {
		select {
		case c.out <- batch:
		case <-ctx.Done():
			return
		}
	}
}
