package server

import (
	"sync"
	"testing"
	"time"
)

func TestTokenBucket_BasicOperation(t *testing.T) {
	// Create a bucket with 5 tokens, refilling at 5 per second
	tb := NewTokenBucket(5, 5)

	// Should be able to consume 5 tokens immediately
	for i := 0; i < 5; i++ {
		allowed, retryAfter := tb.TryConsume()
		if !allowed {
			t.Errorf("Expected token %d to be allowed, got denied with retry after %v", i+1, retryAfter)
		}
	}

	// 6th token should be denied
	allowed, retryAfter := tb.TryConsume()
	if allowed {
		t.Error("Expected 6th token to be denied")
	}
	if retryAfter <= 0 {
		t.Error("Expected positive retry-after duration")
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	// Create a bucket with 2 tokens, refilling at 10 per second
	tb := NewTokenBucket(2, 10)

	// Consume both tokens
	tb.TryConsume()
	tb.TryConsume()

	// Should be denied
	allowed, _ := tb.TryConsume()
	if allowed {
		t.Error("Expected token to be denied after consuming all")
	}

	// Wait for refill (100ms should give us 1 token)
	time.Sleep(110 * time.Millisecond)

	// Should be allowed now
	allowed, _ = tb.TryConsume()
	if !allowed {
		t.Error("Expected token to be allowed after refill")
	}
}

func TestSlidingWindowCounter_BasicOperation(t *testing.T) {
	// Create a counter with 1 second window, max 5 requests
	sw := NewSlidingWindowCounter(time.Second, 5)

	// Should allow 5 requests
	for i := 0; i < 5; i++ {
		allowed, retryAfter := sw.TryIncrement()
		if !allowed {
			t.Errorf("Expected request %d to be allowed, got denied with retry after %v", i+1, retryAfter)
		}
	}

	// 6th request should be denied
	allowed, retryAfter := sw.TryIncrement()
	if allowed {
		t.Error("Expected 6th request to be denied")
	}
	if retryAfter <= 0 {
		t.Error("Expected positive retry-after duration")
	}
}

func TestSlidingWindowCounter_WindowSliding(t *testing.T) {
	// Create a counter with 100ms window, max 5 requests
	sw := NewSlidingWindowCounter(100*time.Millisecond, 5)

	// Use up all 5 requests
	for i := 0; i < 5; i++ {
		sw.TryIncrement()
	}

	// Should be denied
	allowed, _ := sw.TryIncrement()
	if allowed {
		t.Error("Expected request to be denied after max")
	}

	// Wait for window to slide
	time.Sleep(110 * time.Millisecond)

	// Should be allowed now (new window)
	allowed, _ = sw.TryIncrement()
	if !allowed {
		t.Error("Expected request to be allowed after window slides")
	}
}

func TestUserRateLimiter_CombinedLimits(t *testing.T) {
	// Create a user limiter: 3 per second, 10 per minute
	ul := NewUserRateLimiter(3, 10)

	// Should allow 3 requests quickly (per-second limit)
	for i := 0; i < 3; i++ {
		allowed, _ := ul.TryConsume()
		if !allowed {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 4th request should be denied (per-second limit hit)
	allowed, _ := ul.TryConsume()
	if allowed {
		t.Error("Expected 4th request to be denied due to per-second limit")
	}
}

func TestOrderRateLimiter_MultiTier(t *testing.T) {
	config := RateLimitConfig{
		UserPerSecond:    5,
		UserPerMinute:    50,
		ContestPerSecond: 10,
		GlobalPerSecond:  100,
	}

	rl := NewOrderRateLimiter(config, nil)
	contestID := "test-contest"
	userID := "test-user"

	// Should allow 5 requests (user per-second limit)
	for i := 0; i < 5; i++ {
		result := rl.Check(contestID, userID)
		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed, got scope %s", i+1, result.Scope)
		}
	}

	// 6th request should be denied with user scope
	result := rl.Check(contestID, userID)
	if result.Allowed {
		t.Error("Expected 6th request to be denied")
	}
	if result.Scope != RateLimitScopeUser {
		t.Errorf("Expected user scope, got %s", result.Scope)
	}
}

func TestOrderRateLimiter_ContestLimit(t *testing.T) {
	config := RateLimitConfig{
		UserPerSecond:    100, // High user limit
		UserPerMinute:    1000,
		ContestPerSecond: 5, // Low contest limit
		GlobalPerSecond:  1000,
	}

	rl := NewOrderRateLimiter(config, nil)
	contestID := "test-contest"

	// Different users should all count toward contest limit
	for i := 0; i < 5; i++ {
		userID := "user-" + string(rune('a'+i))
		result := rl.Check(contestID, userID)
		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed, got scope %s", i+1, result.Scope)
		}
	}

	// Next request from any user should be denied with contest scope
	result := rl.Check(contestID, "another-user")
	if result.Allowed {
		t.Error("Expected request to be denied due to contest limit")
	}
	if result.Scope != RateLimitScopeContest {
		t.Errorf("Expected contest scope, got %s", result.Scope)
	}
}

func TestOrderRateLimiter_GlobalLimit(t *testing.T) {
	config := RateLimitConfig{
		UserPerSecond:    100,
		UserPerMinute:    1000,
		ContestPerSecond: 100,
		GlobalPerSecond:  5, // Low global limit
	}

	rl := NewOrderRateLimiter(config, nil)

	// Different contests and users should all count toward global limit
	for i := 0; i < 5; i++ {
		contestID := "contest-" + string(rune('a'+i))
		userID := "user-" + string(rune('a'+i))
		result := rl.Check(contestID, userID)
		if !result.Allowed {
			t.Errorf("Expected request %d to be allowed, got scope %s", i+1, result.Scope)
		}
	}

	// Next request should be denied with global scope
	result := rl.Check("contest-z", "user-z")
	if result.Allowed {
		t.Error("Expected request to be denied due to global limit")
	}
	if result.Scope != RateLimitScopeGlobal {
		t.Errorf("Expected global scope, got %s", result.Scope)
	}
}

func TestOrderRateLimiter_Cleanup(t *testing.T) {
	config := DefaultRateLimitConfig()
	rl := NewOrderRateLimiter(config, nil)

	// Make some requests to create limiters
	rl.Check("contest-1", "user-1")
	rl.Check("contest-1", "user-2")
	rl.Check("contest-2", "user-1")

	stats := rl.GetStats()
	if stats["user_limiters_count"].(int) != 3 {
		t.Errorf("Expected 3 user limiters, got %d", stats["user_limiters_count"].(int))
	}
	if stats["contest_limiters_count"].(int) != 2 {
		t.Errorf("Expected 2 contest limiters, got %d", stats["contest_limiters_count"].(int))
	}

	// Remove user
	rl.RemoveUser("contest-1", "user-1")
	stats = rl.GetStats()
	if stats["user_limiters_count"].(int) != 2 {
		t.Errorf("Expected 2 user limiters after removal, got %d", stats["user_limiters_count"].(int))
	}

	// Remove contest
	rl.RemoveContest("contest-1")
	stats = rl.GetStats()
	if stats["user_limiters_count"].(int) != 1 {
		t.Errorf("Expected 1 user limiter after contest removal, got %d", stats["user_limiters_count"].(int))
	}
	if stats["contest_limiters_count"].(int) != 1 {
		t.Errorf("Expected 1 contest limiter after contest removal, got %d", stats["contest_limiters_count"].(int))
	}
}

func TestOrderRateLimiter_Concurrent(t *testing.T) {
	config := RateLimitConfig{
		UserPerSecond:    100,
		UserPerMinute:    1000,
		ContestPerSecond: 100,
		GlobalPerSecond:  50, // This is our limiting factor
	}

	rl := NewOrderRateLimiter(config, nil)

	// Run concurrent requests from many goroutines
	var wg sync.WaitGroup
	allowedCount := 0
	deniedCount := 0
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			contestID := "contest-" + string(rune('a'+idx%10))
			userID := "user-" + string(rune('a'+idx%10))
			result := rl.Check(contestID, userID)

			mu.Lock()
			if result.Allowed {
				allowedCount++
			} else {
				deniedCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Global limit is 50 tokens/second. Token bucket refill during goroutine
	// scheduling delays may grant a few extra tokens beyond the initial 50.
	if allowedCount < 50 || allowedCount > 55 {
		t.Errorf("Expected 50-55 allowed (global limit 50 + possible refill), got %d", allowedCount)
	}
	if allowedCount+deniedCount != 100 {
		t.Errorf("Expected total 100, got allowed=%d denied=%d", allowedCount, deniedCount)
	}
}

func TestRateLimitResult_Error(t *testing.T) {
	result := RateLimitResult{
		Allowed:    false,
		Scope:      RateLimitScopeUser,
		ContestID:  "test-contest",
		RetryAfter: 500 * time.Millisecond,
	}

	errMsg := result.Error()
	if errMsg == "" {
		t.Error("Expected error message for denied result")
	}
	if errMsg != "RATE_LIMITED: user limit exceeded, retry after 500ms" {
		t.Errorf("Unexpected error message: %s", errMsg)
	}

	// Allowed result should have empty error
	result.Allowed = true
	if result.Error() != "" {
		t.Error("Expected empty error message for allowed result")
	}
}
