package community

import (
	"fmt"
	"testing"
	"time"
)

// Two capacity-bounded caches evict differently. exchangeLimiter evicts by last
// use; rateLimiter evicts by insertion, which reaches the global bucket.

// The global budget survives key rotation. It used to not: the bucket was
// evicted on insertion order and recreated full. Fixed in 8978e97.
func TestGlobalBucketSurvivesKeyRotation(t *testing.T) {
	t.Parallel()
	limiter := newRateLimiter(RateLimitPolicy{
		PerUser: RateLimit{Burst: 100, Every: time.Hour},
		Global:  RateLimit{Burst: 2, Every: time.Hour},
	}, 4)

	admitted := 0
	for i := 1; i <= 9; i++ {
		if limiter.Admit(admissionRequest{UserKey: fmt.Sprintf("u%d", i)}).Outcome == admissionAccepted {
			admitted++
		}
	}

	// No time passes, so a correct limiter admits exactly the burst.
	if admitted != 2 {
		t.Errorf("admitted %d of 9 against a burst of 2 with rotating keys; "+
			"more than the burst means the global bucket is being evicted again",
			admitted)
	}
}

// The per-user tier is bounded on its own, so a rotating caller must not also
// be handed back the tier that bounds the whole process.
func TestGlobalBudgetHoldsWithoutRotation(t *testing.T) {
	t.Parallel()
	limiter := newRateLimiter(RateLimitPolicy{
		PerUser: RateLimit{Burst: 100, Every: time.Hour},
		Global:  RateLimit{Burst: 2, Every: time.Hour},
	}, 4)

	admitted := 0
	for range 9 {
		if limiter.Admit(admissionRequest{UserKey: "steady"}).Outcome == admissionAccepted {
			admitted++
		}
	}
	if admitted != 2 {
		t.Errorf("a single caller was admitted %d times against a burst of 2", admitted)
	}
}

// exchangeLimiter is the same shape done correctly, and it is next door. Its
// eviction reads last use, so churn cannot displace an active key.
func TestExchangeLimiterEvictsTheLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	limiter := newExchangeLimiter(nil)
	if limiter == nil {
		t.Skip("no exchange limiter to check")
	}
	// evictLocked scans for the smallest run.last rather than a queue head, so
	// a change to insertion order shows up here as a failure.
	limiter.mu.Lock()
	limiter.maxKeys = 2
	limiter.runs = map[string]exchangeRun{
		"old":    {last: time.Now().Add(-time.Hour)},
		"recent": {last: time.Now()},
	}
	limiter.evictLocked()
	_, oldSurvived := limiter.runs["old"]
	_, recentSurvived := limiter.runs["recent"]
	limiter.mu.Unlock()

	if oldSurvived {
		t.Error("the least recently used key survived eviction")
	}
	if !recentSurvived {
		t.Error("the most recently used key was evicted")
	}
}
