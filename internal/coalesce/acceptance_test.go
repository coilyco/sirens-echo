package coalesce

import (
	"context"
	"testing"
	"time"

	"github.com/coilyco/sirens-echo/internal/ingest"
)

// The lane's arithmetic, as constants so a test failure names which number
// moved. See the package comment and docs/sirens-echo-admission.md.
const (
	arrivalEvery  = 30 * time.Second
	servedIn      = 12 * time.Second
	simulatedFor  = 10 * time.Minute
	simulationHop = time.Second
)

// lane simulates the whole pipeline on virtual time: arrivals feed a queue, the
// window batches, and a fixed pool serves one batch per turn.
type lane struct {
	window  *Window
	policy  Policy
	queued  []ingest.Ask
	ready   []Batch
	busy    []time.Time
	flushed map[int64]time.Time
	peak    int
}

func newLane(policy Policy) *lane {
	resolved := policy.withDefaults()
	l := &lane{
		policy:  resolved,
		busy:    make([]time.Time, resolved.Workers),
		flushed: make(map[int64]time.Time),
	}
	l.window = NewWindow(resolved, func() int { return len(l.queued) })
	return l
}

func (l *lane) arrive(ask ingest.Ask) { l.queued = append(l.queued, ask) }

// step advances the lane one tick: queued asks enter the window, a due window
// flushes, and any free worker picks up the next batch.
func (l *lane) step(now time.Time) {
	for len(l.queued) > 0 {
		ask := l.queued[0]
		l.queued = l.queued[1:]
		l.window.Offer(ask)
		if l.window.Due(now) {
			l.flush(now)
		}
	}
	if l.window.Due(now) {
		l.flush(now)
	}
	for worker := range l.busy {
		if len(l.ready) == 0 {
			break
		}
		if l.busy[worker].After(now) {
			continue
		}
		l.ready = l.ready[1:]
		l.busy[worker] = now.Add(servedIn)
	}
	if depth := l.backlog(); depth > l.peak {
		l.peak = depth
	}
}

func (l *lane) flush(now time.Time) {
	for _, batch := range l.window.Flush() {
		for _, ask := range batch.Asks() {
			l.flushed[ask.Seq] = now
		}
		l.ready = append(l.ready, batch)
	}
}

// backlog is every ask that has arrived and is not yet inside a running turn.
func (l *lane) backlog() int {
	depth := len(l.queued) + l.window.Pending()
	for _, batch := range l.ready {
		depth += batch.Size()
	}
	return depth
}

// The demand this workstream exists for: two arrivals a minute with irregular
// bursts must drain, rather than growing a queue that never recovers.
func TestDesignLoadDrainsWithNoGrowingBacklog(t *testing.T) {
	t.Parallel()
	l := newLane(DefaultPolicy())
	bursts := map[int]int{90: 3, 240: 5, 470: 4}
	var seq int64
	firstHalf, secondHalf := 0, 0
	for tick := 0; tick <= int(simulatedFor/simulationHop); tick++ {
		now := base.Add(time.Duration(tick) * simulationHop)
		if tick > 0 && time.Duration(tick)*simulationHop%arrivalEvery == 0 {
			seq++
			l.arrive(ask(seq, "ana", "steady ask", now))
		}
		for i := 0; i < bursts[tick]; i++ {
			seq++
			l.arrive(ask(seq, "bo", "burst ask", now))
		}
		l.step(now)
		if tick < int(simulatedFor/simulationHop)/2 {
			firstHalf = max(firstHalf, l.backlog())
		} else {
			secondHalf = max(secondHalf, l.backlog())
		}
	}
	for tick := 0; tick < 120; tick++ {
		l.step(base.Add(simulatedFor + time.Duration(tick)*simulationHop))
	}
	t.Logf("fed %d asks, peak backlog %d, early %d, late %d", seq, l.peak, firstHalf, secondHalf)
	if got := l.backlog(); got != 0 {
		t.Fatalf("%d asks were still waiting after the feed stopped", got)
	}
	if secondHalf > firstHalf {
		t.Fatalf("backlog grew: peaked at %d early and %d late", firstHalf, secondHalf)
	}
	if l.peak > DefaultHighWater {
		t.Fatalf("peak backlog %d crossed the high-water mark under design load", l.peak)
	}
}

// No ask may wait past the age cap for a batch. The cap bounds the wait for a
// flush, which is the part the window controls; the turn's own time is its own.
func TestNoAskWaitsPastTheAgeCap(t *testing.T) {
	t.Parallel()
	l := newLane(DefaultPolicy())
	arrivedAt := make(map[int64]time.Time)
	var seq int64
	for tick := 0; tick <= int(simulatedFor/simulationHop); tick++ {
		now := base.Add(time.Duration(tick) * simulationHop)
		if tick%10 == 0 {
			seq++
			l.arrive(ask(seq, "ana", "q", now))
			arrivedAt[seq] = now
		}
		l.step(now)
	}
	for tick := 0; tick < 200; tick++ {
		l.step(base.Add(simulatedFor + time.Duration(tick)*simulationHop))
	}
	worst := time.Duration(0)
	for id, at := range arrivedAt {
		flushed, ok := l.flushed[id]
		if !ok {
			t.Fatalf("ask %d never reached a batch", id)
		}
		waited := flushed.Sub(at)
		if waited > DefaultAgeCap {
			t.Fatalf("ask %d waited %s for a batch, past the %s cap", id, waited, DefaultAgeCap)
		}
		if waited > worst {
			worst = waited
		}
	}
	t.Logf("%d asks, worst wait for a batch %s against a %s cap",
		len(arrivedAt), worst, DefaultAgeCap)
}

// One member firing eight comments in five seconds is one conversation, and it
// must not cost eight turns.
func TestBurstFromOneMemberCoalesces(t *testing.T) {
	t.Parallel()
	w := NewWindow(DefaultPolicy(), nil)
	batches := 0
	for i := 0; i < 8; i++ {
		now := base.Add(time.Duration(i) * 625 * time.Millisecond)
		w.Offer(ask(int64(i+1), "ana", "ask number "+string(rune('a'+i)), now))
		if w.Due(now) {
			batches += len(w.Flush())
		}
	}
	batches += len(w.Flush())
	if batches > 3 {
		t.Fatalf("a burst of 8 became %d batches, want at most 3", batches)
	}
	if batches < 2 {
		t.Fatalf("a burst of 8 became %d batches, so K stopped bounding size", batches)
	}
}

// The same burst across eight members is eight conversations, so the floor is
// the member count. Tested because the figure above reads as universal.
func TestBurstFromDistinctMembersShardsRatherThanMerging(t *testing.T) {
	t.Parallel()
	w := NewWindow(DefaultPolicy(), nil)
	names := []string{"ana", "bo", "cy", "dee", "eli", "fern", "gil", "hal"}
	batches := 0
	for i, who := range names {
		now := base.Add(time.Duration(i) * 625 * time.Millisecond)
		w.Offer(ask(int64(i+1), who, "q", now))
		if w.Due(now) {
			batches += len(w.Flush())
		}
	}
	batches += len(w.Flush())
	if batches != len(names) {
		t.Fatalf("got %d batches for %d members, want one each", batches, len(names))
	}
}

// Killing ingress must not strand acknowledged work. Everything already queued
// reaches a batch and the pool's channel closes, so a restart is clean.
func TestIngressLossDrainsWhatWasAlreadyAcknowledged(t *testing.T) {
	t.Parallel()
	queue := ingest.NewQueue(64)
	in := ingest.NewIngress(queue, nil, nil, nil, nil)
	for i := 0; i < 12; i++ {
		in.Submit(context.Background(), member("ana"), "thread-1", "q", nil)
	}
	policy := DefaultPolicy()
	policy.Window = 20 * time.Millisecond
	policy.WideWindow = 20 * time.Millisecond
	c := NewCoalescer(policy, queue, nil)

	go func() {
		queue.Close()
		c.Run(context.Background())
	}()

	covered := 0
	for batch := range c.Batches() {
		covered += batch.Size()
	}
	if covered != 12 {
		t.Fatalf("%d of 12 acknowledged asks reached a batch", covered)
	}
}

// Widening is the lane's answer to a backlog, and the brief's one escalation
// condition is a feed that still cannot drain after it engages.
func TestWideningEngagesUnderLoadAndTheLaneStillDrains(t *testing.T) {
	t.Parallel()
	l := newLane(DefaultPolicy())
	var seq int64
	for tick := 0; tick < 120; tick++ {
		now := base.Add(time.Duration(tick) * simulationHop)
		seq++
		l.arrive(ask(seq, "ana", "q", now))
		seq++
		l.arrive(ask(seq, "bo", "q", now))
		l.step(now)
	}
	if l.peak <= DefaultHighWater {
		t.Fatalf("peak backlog %d never crossed the high-water mark, so widening was not exercised", l.peak)
	}
	for tick := 0; tick < 600; tick++ {
		l.step(base.Add(120*simulationHop + time.Duration(tick)*simulationHop))
	}
	if got := l.backlog(); got != 0 {
		t.Fatalf("%d asks still waiting after widening engaged, which is the one wake condition", got)
	}
	t.Logf("fed %d asks at 2 a second, peak backlog %d, drained to zero", seq, l.peak)
}
