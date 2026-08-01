package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestUserLimiter_Allow(t *testing.T) {
	cfg := Config{
		Rate:      5,
		Window:    time.Second,
		BurstSize: 5,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	userID := "user-1"

	// Should allow up to burst size
	for i := 0; i < 5; i++ {
		if !limiter.Allow(userID) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Should deny after burst exceeded
	if limiter.Allow(userID) {
		t.Error("Request after burst should be denied")
	}
}

func TestUserLimiter_AllowN(t *testing.T) {
	cfg := Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 10,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	userID := "user-1"

	// Should allow N requests at once
	if !limiter.AllowN(userID, 5) {
		t.Error("Should allow 5 requests")
	}

	if !limiter.AllowN(userID, 5) {
		t.Error("Should allow another 5 requests")
	}

	// Should deny when exceeding
	if limiter.AllowN(userID, 1) {
		t.Error("Should deny after limit exceeded")
	}
}

func TestUserLimiter_Reset(t *testing.T) {
	cfg := Config{
		Rate:      3,
		Window:    time.Second,
		BurstSize: 3,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	userID := "user-1"

	// Exhaust the limit
	for i := 0; i < 3; i++ {
		limiter.Allow(userID)
	}

	if limiter.Allow(userID) {
		t.Error("Should be rate limited")
	}

	// Reset and try again
	limiter.Reset(userID)

	if !limiter.Allow(userID) {
		t.Error("Should allow after reset")
	}
}

func TestUserLimiter_Remaining(t *testing.T) {
	cfg := Config{
		Rate:      5,
		Window:    time.Second,
		BurstSize: 5,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	userID := "user-1"

	if remaining := limiter.Remaining(userID); remaining != 5 {
		t.Errorf("Expected 5 remaining, got %d", remaining)
	}

	limiter.Allow(userID)
	limiter.Allow(userID)

	if remaining := limiter.Remaining(userID); remaining != 3 {
		t.Errorf("Expected 3 remaining, got %d", remaining)
	}
}

func TestUserLimiter_RetryAfter(t *testing.T) {
	cfg := Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 5,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	userID := "user-1"

	// Should be 0 when tokens available
	if retry := limiter.RetryAfter(userID); retry != 0 {
		t.Errorf("Expected 0 retry, got %v", retry)
	}

	// Exhaust tokens
	for i := 0; i < 5; i++ {
		limiter.Allow(userID)
	}

	// Should be non-zero when exhausted
	retry := limiter.RetryAfter(userID)
	if retry <= 0 {
		t.Error("Expected positive retry duration")
	}
	if retry > time.Second {
		t.Errorf("Retry duration too long: %v", retry)
	}
}

func TestUserLimiter_TokenRefill(t *testing.T) {
	cfg := Config{
		Rate:      100, // 100 per second = 10 per 100ms
		Window:    time.Second,
		BurstSize: 10,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	userID := "user-1"

	// Use all tokens
	for i := 0; i < 10; i++ {
		limiter.Allow(userID)
	}

	if limiter.Allow(userID) {
		t.Error("Should be exhausted")
	}

	// Wait for refill
	time.Sleep(150 * time.Millisecond)

	// Should have refilled some tokens
	if !limiter.Allow(userID) {
		t.Error("Should have tokens after refill")
	}
}

func TestUserLimiter_MultipleUsers(t *testing.T) {
	cfg := Config{
		Rate:      3,
		Window:    time.Second,
		BurstSize: 3,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	// Each user has independent limits
	users := []string{"user-1", "user-2", "user-3"}

	for _, user := range users {
		for i := 0; i < 3; i++ {
			if !limiter.Allow(user) {
				t.Errorf("User %s request %d should be allowed", user, i+1)
			}
		}
		if limiter.Allow(user) {
			t.Errorf("User %s should be rate limited", user)
		}
	}
}

func TestUserLimiter_Concurrent(t *testing.T) {
	cfg := Config{
		Rate:      100,
		Window:    time.Second,
		BurstSize: 100,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	var wg sync.WaitGroup
	var allowed, denied int64
	var mu sync.Mutex

	// 10 goroutines each making 20 requests = 200 requests
	// With 100 token burst, ~100 should be allowed
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				result := limiter.Allow("user-concurrent")
				mu.Lock()
				if result {
					allowed++
				} else {
					denied++
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should have allowed approximately 100
	if allowed < 90 || allowed > 110 {
		t.Errorf("Expected ~100 allowed, got %d allowed, %d denied", allowed, denied)
	}
}

func TestUserLimiter_ActiveUsers(t *testing.T) {
	cfg := Config{
		Rate:      5,
		Window:    time.Second,
		BurstSize: 5,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	if count := limiter.ActiveUsers(); count != 0 {
		t.Errorf("Expected 0 active users, got %d", count)
	}

	limiter.Allow("user-1")
	limiter.Allow("user-2")
	limiter.Allow("user-3")

	if count := limiter.ActiveUsers(); count != 3 {
		t.Errorf("Expected 3 active users, got %d", count)
	}
}

func TestUserLimiter_EmptyUserID(t *testing.T) {
	cfg := Config{
		Rate:      5,
		Window:    time.Second,
		BurstSize: 5,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	if limiter.Allow("") {
		t.Error("Should deny empty user ID")
	}

	if limiter.AllowN("", 1) {
		t.Error("Should deny empty user ID for AllowN")
	}
}

func TestUserLimiter_InvalidN(t *testing.T) {
	cfg := Config{
		Rate:      5,
		Window:    time.Second,
		BurstSize: 5,
	}
	limiter := NewUserLimiter(cfg)
	defer limiter.Close()

	if limiter.AllowN("user-1", 0) {
		t.Error("Should deny n=0")
	}

	if limiter.AllowN("user-1", -1) {
		t.Error("Should deny negative n")
	}
}

func TestMultiLimiter(t *testing.T) {
	shortTerm := NewUserLimiter(Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 5,
	})
	defer shortTerm.Close()

	longTerm := NewUserLimiter(Config{
		Rate:      100,
		Window:    time.Minute,
		BurstSize: 50,
	})
	defer longTerm.Close()

	multi := NewMultiLimiter(shortTerm, longTerm)
	userID := "user-1"

	// Should allow within short-term burst
	for i := 0; i < 5; i++ {
		if !multi.Allow(userID) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Should be blocked by short-term
	if multi.Allow(userID) {
		t.Error("Should be blocked by short-term limit")
	}
}

func TestLeakyBucket(t *testing.T) {
	cfg := Config{
		Rate:      10, // 10 per second
		Window:    time.Second,
		BurstSize: 5,
	}
	lb := NewLeakyBucket(cfg)

	// Should allow up to bucket size
	for i := 0; i < 5; i++ {
		if !lb.Allow("") {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Should deny when full
	if lb.Allow("") {
		t.Error("Should deny when bucket is full")
	}

	// Wait for leak
	time.Sleep(200 * time.Millisecond)

	// Should allow after leak
	if !lb.Allow("") {
		t.Error("Should allow after leak")
	}
}

func TestMultiLimiter_NoTokenLeak(t *testing.T) {
	// Bug 1.1: When the second limiter rejects, the first limiter's tokens
	// should not be consumed.
	shortTerm := NewUserLimiter(Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 5,
	})
	defer shortTerm.Close()

	longTerm := NewUserLimiter(Config{
		Rate:      100,
		Window:    time.Minute,
		BurstSize: 3, // Lower burst than shortTerm — this one will reject first
	})
	defer longTerm.Close()

	multi := NewMultiLimiter(shortTerm, longTerm)
	userID := "user-token-leak"

	// Exhaust long-term (3 burst)
	for i := 0; i < 3; i++ {
		if !multi.Allow(userID) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// Record short-term remaining before the failed request
	shortRemaining := shortTerm.Remaining(userID)

	// This should be rejected by the long-term limiter
	if multi.Allow(userID) {
		t.Error("Should be blocked by long-term limit")
	}

	// Short-term tokens should NOT have been consumed by the failed request
	shortRemainingAfter := shortTerm.Remaining(userID)
	if shortRemainingAfter < shortRemaining {
		t.Errorf("Short-term tokens leaked: had %d, now %d", shortRemaining, shortRemainingAfter)
	}
}

func TestMultiLimiter_AllowN_NoTokenLeak(t *testing.T) {
	// Bug 1.1: AllowN with n>1 should not leak tokens from first limiter
	// when second limiter rejects.
	limiterA := NewUserLimiter(Config{
		Rate:      100,
		Window:    time.Second,
		BurstSize: 10,
	})
	defer limiterA.Close()

	limiterB := NewUserLimiter(Config{
		Rate:      100,
		Window:    time.Second,
		BurstSize: 3, // Only 3 tokens available
	})
	defer limiterB.Close()

	multi := NewMultiLimiter(limiterA, limiterB)
	userID := "user-allowN-leak"

	// Try to take 5 — should fail because limiterB only has 3
	if multi.AllowN(userID, 5) {
		t.Error("Should be blocked by limiterB")
	}

	// limiterA should still have all 10 tokens
	remaining := limiterA.Remaining(userID)
	if remaining != 10 {
		t.Errorf("LimiterA tokens leaked: expected 10, got %d", remaining)
	}
}

func TestLeakyBucket_PerKey(t *testing.T) {
	// Bug 1.3: Different keys should have independent buckets.
	cfg := Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 5,
	}
	lb := NewLeakyBucket(cfg)
	defer lb.Close()

	// Exhaust bucket for key "a"
	for i := 0; i < 5; i++ {
		if !lb.Allow("a") {
			t.Errorf("Key 'a' request %d should be allowed", i+1)
		}
	}

	if lb.Allow("a") {
		t.Error("Key 'a' should be exhausted")
	}

	// Key "b" should still have full capacity
	for i := 0; i < 5; i++ {
		if !lb.Allow("b") {
			t.Errorf("Key 'b' request %d should be allowed (independent of key 'a')", i+1)
		}
	}
}

func TestLeakyBucket_Close(t *testing.T) {
	cfg := Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 5,
	}
	lb := NewLeakyBucket(cfg)

	lb.Allow("test")

	// Close should not panic or deadlock
	if err := lb.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestLeakyBucket_Remaining_PerKey(t *testing.T) {
	cfg := Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 5,
	}
	lb := NewLeakyBucket(cfg)
	defer lb.Close()

	// New key should have full capacity
	if remaining := lb.Remaining("new-key"); remaining != 5 {
		t.Errorf("Expected 5 remaining for new key, got %d", remaining)
	}

	// Use some capacity for one key
	lb.Allow("key-1")
	lb.Allow("key-1")

	// key-1 should have less
	r1 := lb.Remaining("key-1")
	if r1 != 3 {
		t.Errorf("Expected 3 remaining for key-1, got %d", r1)
	}

	// key-2 should be independent and full
	r2 := lb.Remaining("key-2")
	if r2 != 5 {
		t.Errorf("Expected 5 remaining for key-2, got %d", r2)
	}
}

func TestLeakyBucket_Reset_PerKey(t *testing.T) {
	cfg := Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 5,
	}
	lb := NewLeakyBucket(cfg)
	defer lb.Close()

	// Exhaust key
	for i := 0; i < 5; i++ {
		lb.Allow("reset-key")
	}

	if lb.Allow("reset-key") {
		t.Error("Should be exhausted")
	}

	// Reset and verify
	lb.Reset("reset-key")
	if !lb.Allow("reset-key") {
		t.Error("Should be allowed after reset")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name:    "valid config",
			cfg:     Config{Rate: 10, Window: time.Second, BurstSize: 5},
			wantErr: nil,
		},
		{
			name:    "zero rate",
			cfg:     Config{Rate: 0, Window: time.Second},
			wantErr: ErrInvalidRate,
		},
		{
			name:    "negative rate",
			cfg:     Config{Rate: -1, Window: time.Second},
			wantErr: ErrInvalidRate,
		},
		{
			name:    "zero window",
			cfg:     Config{Rate: 10, Window: 0},
			wantErr: ErrInvalidWindow,
		},
		{
			name:    "negative burst",
			cfg:     Config{Rate: 10, Window: time.Second, BurstSize: -1},
			wantErr: ErrInvalidBurst,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_WithDefaults(t *testing.T) {
	cfg := Config{
		Rate:   10,
		Window: time.Second,
	}

	cfg = cfg.WithDefaults()

	if cfg.BurstSize != 10 {
		t.Errorf("Expected BurstSize 10, got %d", cfg.BurstSize)
	}

	if cfg.CleanupInterval != time.Second {
		t.Errorf("Expected CleanupInterval 1s, got %v", cfg.CleanupInterval)
	}
}

func TestConfigFromEnv(t *testing.T) {
	// Set env vars
	t.Setenv("TEST_LIMIT_RATE", "50")
	t.Setenv("TEST_LIMIT_WINDOW", "30s")
	t.Setenv("TEST_LIMIT_BURST", "20")
	t.Setenv("TEST_LIMIT_KEY_PREFIX", "test:")

	defaults := Config{
		Rate:      10,
		Window:    time.Second,
		BurstSize: 5,
	}

	cfg := ConfigFromEnv("TEST_LIMIT", defaults)

	if cfg.Rate != 50 {
		t.Errorf("Expected Rate 50, got %d", cfg.Rate)
	}

	if cfg.Window != 30*time.Second {
		t.Errorf("Expected Window 30s, got %v", cfg.Window)
	}

	if cfg.BurstSize != 20 {
		t.Errorf("Expected BurstSize 20, got %d", cfg.BurstSize)
	}

	if cfg.KeyPrefix != "test:" {
		t.Errorf("Expected KeyPrefix 'test:', got '%s'", cfg.KeyPrefix)
	}
}
