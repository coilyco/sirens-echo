package coalesce

import (
	"sort"
	"time"

	"github.com/coilyco/sirens-echo/internal/ingest"
)

// Window is the batcher, written as a state machine its caller steps rather
// than as a goroutine, so every timing rule here is tested without a clock.
type Window struct {
	policy  Policy
	backlog func() int
	pending []ingest.Ask
	// openedAt is the first pending ask's arrival, which W measures from.
	// Measuring from the newest would be a debounce a stream could postpone.
	openedAt time.Time
	open     bool
	// span and capacity resolve when the window opens, so widening applies to
	// the window that opened under load and does not move under one.
	span     time.Duration
	capacity int
}

// NewWindow builds a batcher. backlog reports asks waiting outside the window,
// which widening reads, and nil means the window's own pending count is all.
func NewWindow(policy Policy, backlog func() int) *Window {
	if backlog == nil {
		backlog = func() int { return 0 }
	}
	return &Window{policy: policy.withDefaults(), backlog: backlog}
}

// Offer adds an ask, opening the window when the queue goes empty to not.
func (w *Window) Offer(ask ingest.Ask) {
	w.pending = append(w.pending, ask)
	if w.open {
		return
	}
	w.open = true
	w.openedAt = ask.At
	depth := w.Depth()
	w.span = w.policy.window(depth)
	w.capacity = w.policy.batch(depth)
}

// Depth is the backlog widening compares against the high-water mark.
func (w *Window) Depth() int { return len(w.pending) + w.backlog() }

// Pending counts asks held for the next flush.
func (w *Window) Pending() int { return len(w.pending) }

// Deadline is when the window must close if no further ask arrives. The age
// cap can pull it in, so a starved ask is never held for the whole span.
func (w *Window) Deadline() (time.Time, bool) {
	if !w.open {
		return time.Time{}, false
	}
	deadline := w.openedAt.Add(w.span)
	if promoted := w.pending[0].At.Add(w.policy.AgeCap); promoted.Before(deadline) {
		deadline = promoted
	}
	return deadline, true
}

// Due reports whether the window should close now: K asks accumulated, W
// elapsed, or an ask aged past the cap, whichever came first.
func (w *Window) Due(now time.Time) bool {
	if !w.open {
		return false
	}
	if len(w.pending) >= w.capacity {
		return true
	}
	deadline, _ := w.Deadline()
	return !now.Before(deadline)
}

// Flush closes the window and returns its batches, grouped by tenant and
// ordered by the earliest ask each holds. An empty window returns nothing.
func (w *Window) Flush() []Batch {
	if !w.open || len(w.pending) == 0 {
		w.reset()
		return nil
	}
	openedAt := w.openedAt
	asks := w.pending
	w.reset()
	batches := group(asks, openedAt)
	sort.Slice(batches, func(i, j int) bool { return batches[i].first() < batches[j].first() })
	return batches
}

func (w *Window) reset() {
	w.pending = nil
	w.open = false
	w.openedAt = time.Time{}
	w.span = 0
	w.capacity = 0
}

// group shards by tenant, dedupes by normalized text inside each shard, and
// orders the surviving items by locus so one target's asks read together.
func group(asks []ingest.Ask, openedAt time.Time) []Batch {
	order := make([]string, 0, len(asks))
	byTenant := make(map[string][]ingest.Ask, len(asks))
	for _, ask := range asks {
		key := ask.Tenant.Key()
		if _, seen := byTenant[key]; !seen {
			order = append(order, key)
		}
		byTenant[key] = append(byTenant[key], ask)
	}
	batches := make([]Batch, 0, len(order))
	for _, key := range order {
		shard := byTenant[key]
		batches = append(batches, Batch{
			Tenant:   shard[0].Tenant,
			OpenedAt: openedAt,
			Items:    dedupe(shard),
			Attempt:  attemptFor(1),
		})
	}
	return batches
}

// dedupe collapses asks that normalize to the same request at the same locus.
// Each collapsed ask stays on the item, because each is still owed an answer.
func dedupe(asks []ingest.Ask) []Item {
	index := make(map[string]int, len(asks))
	items := make([]Item, 0, len(asks))
	for _, ask := range asks {
		key := fingerprint(ask.Locus, ask.Text)
		if at, seen := index[key]; seen {
			items[at].Covers = append(items[at].Covers, ask)
			continue
		}
		index[key] = len(items)
		items = append(items, Item{Locus: ask.Locus, Text: ask.Text, Covers: []ingest.Ask{ask}})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Locus < items[j].Locus })
	return items
}
