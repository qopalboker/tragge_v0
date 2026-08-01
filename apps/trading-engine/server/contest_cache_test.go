package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestContestCache_HitMiss
// ---------------------------------------------------------------------------

func TestContestCache_HitMiss(t *testing.T) {
	cc := NewContestCache(ContestCacheConfig{
		TTL:             5 * time.Second,
		CleanupInterval: 10 * time.Second,
	})
	defer cc.Stop()

	contest := &DBContest{ID: "c1", Status: "running", AssetClass: "crypto"}

	// Miss on a key that was never stored.
	got, hit := cc.Get("c1")
	if hit {
		t.Fatal("expected miss for non-existent key, got hit")
	}
	if got != nil {
		t.Fatal("expected nil on miss")
	}

	// Store and then hit.
	cc.Set("c1", contest)
	got, hit = cc.Get("c1")
	if !hit {
		t.Fatal("expected hit after Set")
	}
	if got.ID != "c1" || got.Status != "running" {
		t.Fatalf("unexpected contest data: %+v", got)
	}

	// Verify stats reflect one miss and one hit.
	stats := cc.Stats()
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry, got %d", stats.Entries)
	}
}

// ---------------------------------------------------------------------------
// TestContestCache_Expiration
// ---------------------------------------------------------------------------

func TestContestCache_Expiration(t *testing.T) {
	cc := NewContestCache(ContestCacheConfig{
		TTL:             100 * time.Millisecond,
		CleanupInterval: 1 * time.Hour, // Don't let cleanup interfere
	})
	defer cc.Stop()

	contest := &DBContest{ID: "c2", Status: "running", AssetClass: "forex"}
	cc.Set("c2", contest)

	// Immediate get should hit.
	_, hit := cc.Get("c2")
	if !hit {
		t.Fatal("expected hit immediately after Set")
	}

	// Wait for TTL to expire.
	time.Sleep(150 * time.Millisecond)

	// Should now be a miss (expired entry).
	_, hit = cc.Get("c2")
	if hit {
		t.Fatal("expected miss after TTL expiration")
	}
}

// ---------------------------------------------------------------------------
// TestContestCache_Invalidation
// ---------------------------------------------------------------------------

func TestContestCache_Invalidation(t *testing.T) {
	cc := NewContestCache(ContestCacheConfig{
		TTL:             5 * time.Second,
		CleanupInterval: 10 * time.Second,
	})
	defer cc.Stop()

	contest := &DBContest{ID: "c3", Status: "scheduled", AssetClass: "crypto"}
	cc.Set("c3", contest)

	// Confirm it's cached.
	_, hit := cc.Get("c3")
	if !hit {
		t.Fatal("expected hit before invalidation")
	}

	// Invalidate and verify miss.
	cc.Invalidate("c3")
	_, hit = cc.Get("c3")
	if hit {
		t.Fatal("expected miss after invalidation")
	}

	// Verify the entry count is 0.
	stats := cc.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after invalidation, got %d", stats.Entries)
	}
}

// ---------------------------------------------------------------------------
// TestParticipantCache_CompositeKey
// ---------------------------------------------------------------------------

func TestParticipantCache_CompositeKey(t *testing.T) {
	pc := NewParticipantCache(ParticipantCacheConfig{
		TTL:             5 * time.Second,
		CleanupInterval: 10 * time.Second,
	})
	defer pc.Stop()

	p1 := &DBParticipant{ContestID: "cA", UserID: "u1", QtyTotal: 1000, QtyAvailable: 500}
	p2 := &DBParticipant{ContestID: "cA", UserID: "u2", QtyTotal: 2000, QtyAvailable: 1500}

	pc.Set("cA", "u1", p1)
	pc.Set("cA", "u2", p2)

	// Each should be independently retrievable.
	got1, hit1 := pc.Get("cA", "u1")
	got2, hit2 := pc.Get("cA", "u2")
	if !hit1 || !hit2 {
		t.Fatal("expected both entries to hit")
	}
	if got1.QtyTotal != 1000 {
		t.Errorf("p1 QtyTotal: want 1000, got %d", got1.QtyTotal)
	}
	if got2.QtyTotal != 2000 {
		t.Errorf("p2 QtyTotal: want 2000, got %d", got2.QtyTotal)
	}

	// Invalidate all participants for contest A (userID = "").
	pc.Invalidate("cA", "")

	_, hit1 = pc.Get("cA", "u1")
	_, hit2 = pc.Get("cA", "u2")
	if hit1 || hit2 {
		t.Fatal("expected both entries to be gone after contest-wide invalidation")
	}

	stats := pc.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after invalidation, got %d", stats.Entries)
	}
}

// ---------------------------------------------------------------------------
// TestCachedLookup_FallsBackToDB
// ---------------------------------------------------------------------------

// cachedContestLookup reproduces the same pattern as Engine.getContestCached
// but uses a pluggable dbLookupFn so we can count DB calls without a real DB.
func cachedContestLookup(
	cache *ContestCache,
	cacheEnabled bool,
	contestID string,
	dbLookupFn func(contestID string) (*DBContest, error),
) (*DBContest, error) {
	if cacheEnabled {
		if cached, hit := cache.Get(contestID); hit {
			return cached, nil
		}
	}
	contest, err := dbLookupFn(contestID)
	if err != nil {
		return nil, err
	}
	if contest != nil && cacheEnabled {
		cache.Set(contestID, contest)
	}
	return contest, nil
}

func TestCachedLookup_FallsBackToDB(t *testing.T) {
	cc := NewContestCache(ContestCacheConfig{
		TTL:             5 * time.Second,
		CleanupInterval: 10 * time.Second,
	})
	defer cc.Stop()

	var dbCalls atomic.Int32

	dbLookup := func(contestID string) (*DBContest, error) {
		dbCalls.Add(1)
		return &DBContest{ID: contestID, Status: "running", AssetClass: "crypto"}, nil
	}

	// First call: cache miss → DB lookup.
	got, err := cachedContestLookup(cc, true, "c10", dbLookup)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "c10" {
		t.Fatalf("expected c10, got %s", got.ID)
	}
	if dbCalls.Load() != 1 {
		t.Fatalf("expected 1 DB call after first lookup, got %d", dbCalls.Load())
	}

	// Second call: cache hit → no DB lookup.
	got, err = cachedContestLookup(cc, true, "c10", dbLookup)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "c10" {
		t.Fatalf("expected c10, got %s", got.ID)
	}
	if dbCalls.Load() != 1 {
		t.Fatalf("expected still 1 DB call after second lookup (cache hit), got %d", dbCalls.Load())
	}
}

// ---------------------------------------------------------------------------
// TestCacheDisabled_AlwaysHitsDB
// ---------------------------------------------------------------------------

func TestCacheDisabled_AlwaysHitsDB(t *testing.T) {
	cc := NewContestCache(ContestCacheConfig{
		TTL:             5 * time.Second,
		CleanupInterval: 10 * time.Second,
	})
	defer cc.Stop()

	var dbCalls atomic.Int32

	dbLookup := func(contestID string) (*DBContest, error) {
		dbCalls.Add(1)
		return &DBContest{ID: contestID, Status: "running", AssetClass: "crypto"}, nil
	}

	// With cacheEnabled=false, every call should hit the DB.
	for i := 0; i < 5; i++ {
		_, err := cachedContestLookup(cc, false, "c20", dbLookup)
		if err != nil {
			t.Fatal(err)
		}
	}

	if dbCalls.Load() != 5 {
		t.Fatalf("expected 5 DB calls with cache disabled, got %d", dbCalls.Load())
	}

	// Cache should still be empty since cacheEnabled was false.
	stats := cc.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 cache entries when cache disabled, got %d", stats.Entries)
	}
}

// ---------------------------------------------------------------------------
// TestDynamicRateLimit_ScalesWithParticipants
// ---------------------------------------------------------------------------

func TestDynamicRateLimit_ScalesWithParticipants(t *testing.T) {
	cfg := RateLimitConfig{
		UserPerSecond:           10,
		UserPerMinute:           100,
		ContestPerSecond:        500,
		GlobalPerSecond:         5000,
		DynamicContestLimits:    true,
		ContestLimitBaseRate:    100,
		ContestLimitMultiplier:  2,
		ContestLimitRefreshSecs: 300,
	}

	rl := NewOrderRateLimiter(cfg, nil)

	// Provide a participant count function.
	participantCounts := map[string]int{
		"contest-small": 10,
		"contest-large": 500,
	}
	rl.SetParticipantCountFunc(func(_ context.Context, contestID string) (int, error) {
		return participantCounts[contestID], nil
	})

	// Trigger limiter creation by calling Check.
	rl.Check("contest-small", "user1")
	rl.Check("contest-large", "user2")

	rates := rl.GetContestEffectiveRates()

	// 10 participants * 2 multiplier = 20 → base rate 100 wins → limit = 100
	smallRate, ok := rates["contest-small"]
	if !ok {
		t.Fatal("missing rate for contest-small")
	}
	if smallRate != 100 {
		t.Errorf("contest-small: expected effective rate 100, got %d", smallRate)
	}

	// 500 participants * 2 multiplier = 1000 → 1000 > base rate 100 → limit = 1000
	largeRate, ok := rates["contest-large"]
	if !ok {
		t.Fatal("missing rate for contest-large")
	}
	if largeRate != 1000 {
		t.Errorf("contest-large: expected effective rate 1000, got %d", largeRate)
	}
}

// ---------------------------------------------------------------------------
// TestConcurrentCacheAccess
// ---------------------------------------------------------------------------

func TestConcurrentCacheAccess(t *testing.T) {
	cc := NewContestCache(ContestCacheConfig{
		TTL:             1 * time.Second,
		CleanupInterval: 500 * time.Millisecond,
	})
	defer cc.Stop()

	pc := NewParticipantCache(ParticipantCacheConfig{
		TTL:             1 * time.Second,
		CleanupInterval: 500 * time.Millisecond,
	})
	defer pc.Stop()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			contestID := "contest"
			userID := "user"

			contest := &DBContest{ID: contestID, Status: "running", AssetClass: "crypto"}
			participant := &DBParticipant{ContestID: contestID, UserID: userID, QtyTotal: 1000}

			// Mix of reads, writes, invalidations.
			for j := 0; j < 50; j++ {
				cc.Set(contestID, contest)
				cc.Get(contestID)
				cc.Stats()
				if j%10 == 0 {
					cc.Invalidate(contestID)
				}

				pc.Set(contestID, userID, participant)
				pc.Get(contestID, userID)
				pc.Stats()
				if j%10 == 0 {
					pc.Invalidate(contestID, "")
				}
			}
		}(i)
	}

	wg.Wait()

	// If we reach here without a data race (when run with -race), the test passes.
	// As a sanity check, stats should still be retrievable.
	cs := cc.Stats()
	ps := pc.Stats()
	t.Logf("Contest cache — hits: %d, misses: %d, entries: %d", cs.Hits, cs.Misses, cs.Entries)
	t.Logf("Participant cache — hits: %d, misses: %d, entries: %d", ps.Hits, ps.Misses, ps.Entries)
}
