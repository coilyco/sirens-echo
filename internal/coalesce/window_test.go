package coalesce

import (
	"strings"
	"testing"
	"time"

	"github.com/coilyco/sirens-echo/internal/ingest"
)

var base = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

// member builds a tenant for one Discord member, which is the shard: guild and
// channel already group the conversation before the coalescer sees it.
func member(name string) ingest.Tenant {
	return ingest.Tenant{
		Surface: ingest.SurfaceDiscord,
		Guild:   "guild-1",
		Channel: "channel-1",
		Author:  name,
	}
}

func ask(seq int64, who, text string, at time.Time) ingest.Ask {
	return ingest.Ask{Seq: seq, Tenant: member(who), Locus: "thread-1", Text: text, At: at}
}

func TestWindowClosesAtTheSpanWhenKIsNeverReached(t *testing.T) {
	t.Parallel()
	w := NewWindow(DefaultPolicy(), nil)
	w.Offer(ask(1, "ana", "first", base))
	if w.Due(base.Add(DefaultWindow - time.Second)) {
		t.Fatal("window closed before its span elapsed")
	}
	if !w.Due(base.Add(DefaultWindow)) {
		t.Fatal("window stayed open past its span")
	}
}

func TestWindowClosesAtKBeforeTheSpan(t *testing.T) {
	t.Parallel()
	w := NewWindow(DefaultPolicy(), nil)
	for i := 0; i < DefaultBatch; i++ {
		w.Offer(ask(int64(i+1), "ana", "q", base.Add(time.Duration(i)*time.Second)))
	}
	if !w.Due(base.Add(4 * time.Second)) {
		t.Fatalf("window held %d asks without closing at K=%d", w.Pending(), DefaultBatch)
	}
}

// A steady stream must not postpone the flush. That is the difference between
// bounded coalescing and a debounce, and why W starts at the first pending ask.
func TestSteadyStreamCannotPostponeTheFlush(t *testing.T) {
	t.Parallel()
	policy := DefaultPolicy()
	policy.Batch = 1000
	policy.WideBatch = 1000
	w := NewWindow(policy, nil)
	for i := 0; i < 60; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		w.Offer(ask(int64(i+1), "ana", "q", at))
		if w.Due(at) {
			if at.Sub(base) < DefaultWindow {
				t.Fatalf("closed at %s, before the span", at.Sub(base))
			}
			return
		}
	}
	t.Fatal("a continuous stream held the window open past its span")
}

func TestAgeCapPullsTheDeadlineIn(t *testing.T) {
	t.Parallel()
	policy := DefaultPolicy()
	policy.Window = 10 * time.Minute
	policy.WideWindow = 10 * time.Minute
	w := NewWindow(policy, nil)
	w.Offer(ask(1, "ana", "q", base))
	deadline, open := w.Deadline()
	if !open {
		t.Fatal("window reports closed after an offer")
	}
	if want := base.Add(DefaultAgeCap); !deadline.Equal(want) {
		t.Fatalf("deadline %s, want the age cap at %s", deadline, want)
	}
}

func TestWideningReadsTheBacklogAtWindowOpen(t *testing.T) {
	t.Parallel()
	depth := DefaultHighWater + 1
	w := NewWindow(DefaultPolicy(), func() int { return depth })
	w.Offer(ask(1, "ana", "q", base))
	if w.Due(base.Add(DefaultWindow)) {
		t.Fatal("a widened window closed at the narrow span")
	}
	if !w.Due(base.Add(DefaultWideWindow)) {
		t.Fatal("a widened window stayed open past the wide span")
	}
	// The backlog draining mid-window must not move a span already resolved,
	// or a window would shorten under a member who is still typing.
	depth = 0
	if _, open := w.Deadline(); !open {
		t.Fatal("window closed when the backlog drained")
	}
	if w.Due(base.Add(DefaultWindow)) {
		t.Fatal("span moved after the window opened")
	}
}

func TestNarrowWindowReturnsWhenTheBacklogDrains(t *testing.T) {
	t.Parallel()
	depth := DefaultHighWater + 1
	w := NewWindow(DefaultPolicy(), func() int { return depth })
	w.Offer(ask(1, "ana", "q", base))
	w.Flush()
	depth = 0
	w.Offer(ask(2, "ana", "q", base))
	if !w.Due(base.Add(DefaultWindow)) {
		t.Fatal("the window stayed wide after the backlog drained")
	}
}

func TestDedupeCollapsesWorkAndKeepsEveryAsk(t *testing.T) {
	t.Parallel()
	w := NewWindow(DefaultPolicy(), nil)
	w.Offer(ask(1, "ana", "Fix the header", base))
	w.Offer(ask(2, "ana", "fix the header!", base.Add(time.Second)))
	w.Offer(ask(3, "ana", "and the footer", base.Add(2*time.Second)))
	batches := w.Flush()
	if len(batches) != 1 {
		t.Fatalf("got %d batches for one member, want 1", len(batches))
	}
	if got := len(batches[0].Items); got != 2 {
		t.Fatalf("got %d distinct items, want 2", got)
	}
	if got := batches[0].Size(); got != 3 {
		t.Fatalf("batch covers %d asks, want all 3", got)
	}
}

func TestGroupingShardsByMemberAndOrdersFIFO(t *testing.T) {
	t.Parallel()
	w := NewWindow(DefaultPolicy(), nil)
	w.Offer(ask(1, "ana", "ana one", base))
	w.Offer(ask(2, "bo", "bo one", base.Add(time.Second)))
	w.Offer(ask(3, "ana", "ana two", base.Add(2*time.Second)))
	batches := w.Flush()
	if len(batches) != 2 {
		t.Fatalf("got %d batches for two members, want 2", len(batches))
	}
	if batches[0].Tenant.Author != "ana" || batches[1].Tenant.Author != "bo" {
		t.Fatalf("batches out of arrival order: %s then %s",
			batches[0].Tenant.Author, batches[1].Tenant.Author)
	}
	if got := batches[0].Size(); got != 2 {
		t.Fatalf("ana's batch covers %d asks, want 2", got)
	}
}

func TestFlushResetsTheWindow(t *testing.T) {
	t.Parallel()
	w := NewWindow(DefaultPolicy(), nil)
	w.Offer(ask(1, "ana", "q", base))
	w.Flush()
	if w.Pending() != 0 {
		t.Fatalf("%d asks survived the flush", w.Pending())
	}
	if _, open := w.Deadline(); open {
		t.Fatal("window still open after a flush")
	}
	if w.Due(base.Add(time.Hour)) {
		t.Fatal("a closed window reported due")
	}
}

func TestCriteriaNamesEveryCoveredComment(t *testing.T) {
	t.Parallel()
	w := NewWindow(DefaultPolicy(), nil)
	w.Offer(ask(1, "ana", "first", base))
	w.Offer(ask(2, "ana", "second", base.Add(time.Second)))
	criteria := w.Flush()[0].Criteria()
	for _, want := range []string{"#1", "#2", "first", "second", "Done means"} {
		if !strings.Contains(criteria, want) {
			t.Fatalf("criteria omits %q:\n%s", want, criteria)
		}
	}
}
