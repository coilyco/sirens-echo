package community

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Absence of logs is not evidence, and it is indistinguishable from absence of
// work. See docs/sirens-echo-observability.md.

// heartbeatEvery is short enough that a five minute alert window sees several
// beats, and long enough that a quiet night costs almost nothing.
const heartbeatEvery = time.Minute

// heartbeat counts what the gateway saw since the last beat. Counts rather than
// totals, so the record is a rate and needs no baseline to read.
type heartbeat struct {
	observed atomic.Int64
	admitted atomic.Int64
	replied  atomic.Int64
	// lastBeat is unix nanoseconds, seeded at start so a beat that never comes
	// still ages. Zero means unset, as in a bare literal.
	lastBeat atomic.Int64
}

func newHeartbeat(now time.Time) *heartbeat {
	h := &heartbeat{}
	h.lastBeat.Store(now.UnixNano())
	return h
}

// age is the time since the last beat, and false when there is no gateway.
func (h *heartbeat) age(now time.Time) (time.Duration, bool) {
	if h == nil || h.lastBeat.Load() == 0 {
		return 0, false
	}
	return now.Sub(time.Unix(0, h.lastBeat.Load())), true
}

// A nil heartbeat is the HTTP-only deployment, which opens no gateway.
func (h *heartbeat) observe() {
	if h != nil {
		h.observed.Add(1)
	}
}

func (h *heartbeat) admit() {
	if h != nil {
		h.admitted.Add(1)
	}
}

func (h *heartbeat) reply() {
	if h != nil {
		h.replied.Add(1)
	}
}

// beat drains the counters, so a reader sees the interval rather than a total.
func (h *heartbeat) beat(ctx context.Context, telemetry *Telemetry) {
	telemetry.Info(
		ctx,
		"discord.heartbeat",
		slog.Int64("messages_observed", h.observed.Swap(0)),
		slog.Int64("turns_admitted", h.admitted.Swap(0)),
		slog.Int64("replies_sent", h.replied.Swap(0)),
		slog.Int64("interval_seconds", int64(heartbeatEvery/time.Second)),
	)
	h.lastBeat.Store(time.Now().UnixNano())
}

// watchGateway emits a beat until the context ends. A positive signal is what
// separates a quiet guild from a process nobody can see has stopped.
func (a *Agent) watchGateway(ctx context.Context) func() {
	if a.beats == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.beats.beat(ctx, a.telemetry)
			}
		}
	}()
	return func() { close(done) }
}
