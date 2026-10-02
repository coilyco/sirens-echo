package coalesce

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coilyco/sirens-echo/internal/ingest"
)

// scriptedRunner fails a fixed number of times and records every attempt it
// was handed, which is what the ladder's shape is asserted against.
type scriptedRunner struct {
	mu       sync.Mutex
	failures int
	seen     []Attempt
	hold     time.Duration
}

func (s *scriptedRunner) Run(ctx context.Context, batch Batch) error {
	s.mu.Lock()
	s.seen = append(s.seen, batch.Attempt)
	remaining := s.failures
	s.failures--
	hold := s.hold
	s.mu.Unlock()
	if hold > 0 {
		select {
		case <-time.After(hold):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if remaining > 0 {
		return errors.New("model refused the request")
	}
	return nil
}

func (s *scriptedRunner) attempts() []Attempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Attempt(nil), s.seen...)
}

type countingObserver struct {
	discard
	escalated atomic.Int64
	dead      atomic.Int64
	batches   atomic.Int64
	turns     atomic.Int64
}

func (c *countingObserver) Escalated(context.Context)    { c.escalated.Add(1) }
func (c *countingObserver) DeadLettered(context.Context) { c.dead.Add(1) }
func (c *countingObserver) Batch(context.Context, int)   { c.batches.Add(1) }
func (c *countingObserver) Turn(context.Context, string, Tier, time.Duration) {
	c.turns.Add(1)
}

type shelf struct {
	mu    sync.Mutex
	taken []Batch
}

func (s *shelf) Shelve(_ context.Context, batch Batch, _ error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taken = append(s.taken, batch)
}

func (s *shelf) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.taken)
}

func batchFor(author string, seq int64) Batch {
	one := ask(seq, author, "q", base)
	return Batch{
		Tenant:   one.Tenant,
		OpenedAt: base,
		Items:    []Item{{Locus: one.Locus, Text: one.Text, Covers: []ingest.Ask{one}}},
		Attempt:  attemptFor(1),
	}
}

func drain(t *testing.T, pool *Pool, batches ...Batch) {
	t.Helper()
	feed := make(chan Batch, len(batches))
	for _, batch := range batches {
		feed <- batch
	}
	close(feed)
	wait := pool.Start(context.Background(), feed)
	wait()
}

func TestLadderTriesThinkingOffBeforeItEscalates(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{failures: 2}
	counts := &countingObserver{}
	pool := NewPool(DefaultPolicy(), runner, nil, nil, counts)
	drain(t, pool, batchFor("ana", 1))

	seen := runner.attempts()
	if len(seen) != 3 {
		t.Fatalf("ran %d attempts, want the full ladder", len(seen))
	}
	want := []Attempt{
		{Number: 1, Tier: TierStandard, Thinking: true},
		{Number: 2, Tier: TierStandard, Thinking: false},
		{Number: 3, Tier: TierPro, Thinking: false},
	}
	for i, attempt := range seen {
		if attempt != want[i] {
			t.Fatalf("attempt %d was %+v, want %+v", i+1, attempt, want[i])
		}
	}
	if got := counts.escalated.Load(); got != 1 {
		t.Fatalf("counted %d escalations, want exactly the one batch", got)
	}
}

func TestExhaustedLadderShelvesTheBatchAndKeepsDraining(t *testing.T) {
	t.Parallel()
	runner := &scriptedRunner{failures: 99}
	letters := &shelf{}
	counts := &countingObserver{}
	pool := NewPool(DefaultPolicy(), runner, letters, nil, counts)
	drain(t, pool, batchFor("ana", 1), batchFor("bo", 2), batchFor("cy", 3))

	if got := letters.count(); got != 3 {
		t.Fatalf("shelved %d batches, want all 3 that exhausted the ladder", got)
	}
	if got := counts.dead.Load(); got != 3 {
		t.Fatalf("counted %d dead letters, want 3", got)
	}
	if got := len(runner.attempts()); got != 3*MaxAttempts {
		t.Fatalf("ran %d attempts, want %d", got, 3*MaxAttempts)
	}
}

func TestOneWriterPerTenantAndNoneAcrossThem(t *testing.T) {
	t.Parallel()
	var live, peak atomic.Int64
	var perTenant sync.Map
	overlapped := make(chan struct{}, 1)
	runner := runnerFunc(func(_ context.Context, batch Batch) error {
		key := batch.Tenant.Key()
		if _, busy := perTenant.LoadOrStore(key, true); busy {
			select {
			case overlapped <- struct{}{}:
			default:
			}
		}
		now := live.Add(1)
		for {
			high := peak.Load()
			if now <= high || peak.CompareAndSwap(high, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		perTenant.Delete(key)
		live.Add(-1)
		return nil
	})
	pool := NewPool(DefaultPolicy(), runner, nil, nil, nil)
	drain(t, pool,
		batchFor("ana", 1), batchFor("ana", 2), batchFor("ana", 3),
		batchFor("bo", 4), batchFor("cy", 5), batchFor("dee", 6),
	)

	select {
	case <-overlapped:
		t.Fatal("two turns wrote for one member at once")
	default:
	}
	if got := peak.Load(); got < 2 {
		t.Fatalf("peak concurrency was %d, so distinct members serialized", got)
	}
	if got := pool.locks.tracked(); got != 0 {
		t.Fatalf("%d tenant locks outlived their batches", got)
	}
}

func TestEveryTurnCarriesTheHardDeadline(t *testing.T) {
	t.Parallel()
	policy := DefaultPolicy()
	seen := make(chan time.Duration, 1)
	runner := runnerFunc(func(ctx context.Context, _ Batch) error {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("turn ran with no deadline at all")
			return nil
		}
		seen <- time.Until(deadline)
		return nil
	})
	drain(t, NewPool(policy, runner, nil, nil, nil), batchFor("ana", 1))

	got := <-seen
	if got > policy.Deadline || got < policy.Deadline-time.Second {
		t.Fatalf("turn deadline was %s, want about %s", got, policy.Deadline)
	}
}

func TestATimedOutTurnStillEndsInADeadLetter(t *testing.T) {
	t.Parallel()
	policy := DefaultPolicy()
	policy.Deadline = 10 * time.Millisecond
	letters := &shelf{}
	runner := &scriptedRunner{failures: 99, hold: time.Second}
	drain(t, NewPool(policy, runner, letters, nil, nil), batchFor("ana", 1))

	if got := letters.count(); got != 1 {
		t.Fatalf("shelved %d batches, want the one that kept timing out", got)
	}
}

// A cancelled parent is a restart rather than a failing batch, so it must not
// spend the ladder that exists for one.
func TestShutdownDoesNotSpendTheLadder(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &scriptedRunner{}
	letters := &shelf{}
	feed := make(chan Batch, 1)
	feed <- batchFor("ana", 1)
	close(feed)
	pool := NewPool(DefaultPolicy(), runner, letters, nil, nil)
	pool.Start(ctx, feed)()

	if got := len(runner.attempts()); got != 0 {
		t.Fatalf("ran %d attempts during a shutdown", got)
	}
	if got := letters.count(); got != 1 {
		t.Fatalf("shelved %d batches, want the one the restart cut", got)
	}
}

type runnerFunc func(context.Context, Batch) error

func (f runnerFunc) Run(ctx context.Context, batch Batch) error { return f(ctx, batch) }
