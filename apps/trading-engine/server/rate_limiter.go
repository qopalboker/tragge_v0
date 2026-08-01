package server

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// RateLimitScope identifies which rate limit tier was hit.
type RateLimitScope string

const (
	RateLimitScopeUser    RateLimitScope = "user"
	RateLimitScopeContest RateLimitScope = "contest"
	RateLimitScopeGlobal  RateLimitScope = "global"
)

// RateLimitConfig holds configuration for rate limiting.
type RateLimitConfig struct {
	UserPerSecond    int // Max orders per second per user per contest (default: 10)
	UserPerMinute    int // Max orders per minute per user per contest (default: 100)
	ContestPerSecond int // Max orders per second per contest (default: 500, used as fallback when dynamic limits are disabled)
	GlobalPerSecond  int // Max orders per second globally (default: 5000)

	// Dynamic contest rate limiting
	DynamicContestLimits    bool // Enable dynamic per-contest limits based on participant count (default: true)
	ContestLimitBaseRate    int  // Minimum rate for any contest regardless of size (default: 100)
	ContestLimitMultiplier  int  // Orders/sec headroom per participant (default: 2)
	ContestLimitRefreshSecs int  // How often to refresh dynamic limits in seconds (default: 300 = 5 minutes)
}

// DefaultRateLimitConfig returns the default rate limit configuration.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		UserPerSecond:           10,
		UserPerMinute:           100,
		ContestPerSecond:        500,
		GlobalPerSecond:         5000,
		DynamicContestLimits:    true,
		ContestLimitBaseRate:    100,
		ContestLimitMultiplier:  2,
		ContestLimitRefreshSecs: 300,
	}
}

// RateLimitResult contains information about a rate limit check result.
type RateLimitResult struct {
	Allowed    bool
	Scope      RateLimitScope
	ContestID  string
	RetryAfter time.Duration
}

// Error returns an error message for rate limit rejections.
func (r RateLimitResult) Error() string {
	if r.Allowed {
		return ""
	}
	return fmt.Sprintf("RATE_LIMITED: %s limit exceeded, retry after %s", r.Scope, r.RetryAfter.Round(time.Millisecond))
}

// RateLimitMetrics holds Prometheus metrics for rate limiting.
type RateLimitMetrics struct {
	OrdersRateLimited *prometheus.CounterVec
}

// NewRateLimitMetrics creates and registers rate limit metrics.
func NewRateLimitMetrics(registry prometheus.Registerer, namespace string) *RateLimitMetrics {
	metrics := &RateLimitMetrics{
		OrdersRateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "orders_rate_limited_total",
			Help:      "Total number of orders rejected due to rate limiting",
		}, []string{"scope", "contest_id"}),
	}

	registry.MustRegister(metrics.OrdersRateLimited)
	return metrics
}

// TokenBucket implements a token bucket rate limiter.
type TokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket with the given capacity and refill rate.
func NewTokenBucket(maxTokens float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// TryConsume attempts to consume a token. Returns true if successful, false otherwise.
// Also returns the time until a token would be available (retry-after duration).
func (tb *TokenBucket) TryConsume() (bool, time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens = min(tb.maxTokens, tb.tokens+elapsed*tb.refillRate)
	tb.lastRefill = now

	if tb.tokens >= 1 {
		tb.tokens--
		return true, 0
	}

	// Calculate retry-after duration
	deficit := 1 - tb.tokens
	retryAfter := time.Duration(deficit/tb.refillRate*1000) * time.Millisecond
	return false, retryAfter
}

// AtomicTokenBucket is a lock-free token bucket using CAS operations.
// Designed for high-contention scenarios like the global rate limiter.
// Tokens are stored as milli-tokens (tokens * 1000) for sub-token precision
// without floating point.
type AtomicTokenBucket struct {
	milliTokens  atomic.Int64
	lastRefillMs atomic.Int64
	maxMilli     int64 // maxTokens * 1000
	refillPerMs  int64 // milli-tokens added per millisecond
}

// NewAtomicTokenBucket creates a lock-free token bucket.
func NewAtomicTokenBucket(maxTokens, refillRate int) *AtomicTokenBucket {
	b := &AtomicTokenBucket{
		maxMilli:    int64(maxTokens) * 1000,
		refillPerMs: int64(refillRate), // refillRate tokens/sec = refillRate milli-tokens/ms
	}
	b.milliTokens.Store(b.maxMilli)
	b.lastRefillMs.Store(time.Now().UnixMilli())
	return b
}

// TryConsume attempts to consume one token using atomic CAS operations.
// Returns true if successful, false with a retry-after duration otherwise.
func (b *AtomicTokenBucket) TryConsume() (bool, time.Duration) {
	const oneToken int64 = 1000

	// Refill step: exactly one goroutine claims the elapsed time via CAS
	nowMs := time.Now().UnixMilli()
	lastMs := b.lastRefillMs.Load()
	if elapsed := nowMs - lastMs; elapsed > 0 {
		if b.lastRefillMs.CompareAndSwap(lastMs, nowMs) {
			refill := elapsed * b.refillPerMs
			newVal := b.milliTokens.Add(refill)
			// Cap at max (momentary overshoot is harmless)
			if newVal > b.maxMilli {
				b.milliTokens.Add(b.maxMilli - newVal)
			}
		}
	}

	// Consume step: CAS loop to decrement one token
	for i := 0; i < 8; i++ {
		current := b.milliTokens.Load()
		if current < oneToken {
			var retryMs int64 = 1
			if b.refillPerMs > 0 {
				retryMs = (oneToken - current + b.refillPerMs - 1) / b.refillPerMs
				if retryMs < 1 {
					retryMs = 1
				}
			}
			return false, time.Duration(retryMs) * time.Millisecond
		}
		if b.milliTokens.CompareAndSwap(current, current-oneToken) {
			return true, 0
		}
	}

	return false, time.Millisecond
}

// Refund adds one token back (used when a downstream check rejects after global passed).
func (b *AtomicTokenBucket) Refund() {
	newVal := b.milliTokens.Add(1000)
	if newVal > b.maxMilli {
		b.milliTokens.Add(b.maxMilli - newVal)
	}
}

// SlidingWindowCounter implements a sliding window counter rate limiter.
type SlidingWindowCounter struct {
	windowSize  time.Duration
	maxRequests int64
	current     int64     // Current window count
	previous    int64     // Previous window count
	windowStart time.Time // Start of current window
	mu          sync.Mutex
}

// NewSlidingWindowCounter creates a new sliding window counter.
func NewSlidingWindowCounter(windowSize time.Duration, maxRequests int64) *SlidingWindowCounter {
	return &SlidingWindowCounter{
		windowSize:  windowSize,
		maxRequests: maxRequests,
		current:     0,
		previous:    0,
		windowStart: time.Now(),
	}
}

// TryIncrement attempts to increment the counter. Returns true if under limit, false otherwise.
// Also returns the retry-after duration.
func (sw *SlidingWindowCounter) TryIncrement() (bool, time.Duration) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()

	// Calculate how many windows have passed
	elapsed := now.Sub(sw.windowStart)
	windowsPassed := int(elapsed / sw.windowSize)

	if windowsPassed >= 2 {
		// Two or more windows have passed, reset both
		sw.previous = 0
		sw.current = 0
		sw.windowStart = now.Truncate(sw.windowSize)
	} else if windowsPassed == 1 {
		// One window has passed, shift windows
		sw.previous = sw.current
		sw.current = 0
		sw.windowStart = sw.windowStart.Add(sw.windowSize)
	}

	// Calculate weighted count using sliding window approximation
	elapsedInWindow := now.Sub(sw.windowStart)
	weight := 1.0 - (float64(elapsedInWindow) / float64(sw.windowSize))
	count := float64(sw.previous)*weight + float64(sw.current)

	if count >= float64(sw.maxRequests) {
		// Calculate retry-after: time until enough of the previous window slides out
		neededReduction := count - float64(sw.maxRequests) + 1
		timeToWait := time.Duration(neededReduction / float64(sw.maxRequests) * float64(sw.windowSize))
		if timeToWait < time.Millisecond {
			timeToWait = time.Millisecond
		}
		return false, timeToWait
	}

	sw.current++
	return true, 0
}

// UpdateRate updates the maximum requests allowed per window.
// This preserves the current window state (counts, timing) while changing the limit.
func (sw *SlidingWindowCounter) UpdateRate(maxRequests int64) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.maxRequests = maxRequests
}

// MaxRequests returns the current maximum requests per window.
func (sw *SlidingWindowCounter) MaxRequests() int64 {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.maxRequests
}

// UserRateLimiter combines per-second and per-minute rate limits for a user.
type UserRateLimiter struct {
	perSecond  *TokenBucket
	perMinute  *SlidingWindowCounter
	lastAccess atomic.Int64 // Unix timestamp of last access for cleanup
}

// NewUserRateLimiter creates a new user rate limiter.
func NewUserRateLimiter(perSecond, perMinute int) *UserRateLimiter {
	ul := &UserRateLimiter{
		perSecond: NewTokenBucket(float64(perSecond), float64(perSecond)),
		perMinute: NewSlidingWindowCounter(time.Minute, int64(perMinute)),
	}
	ul.lastAccess.Store(time.Now().Unix())
	return ul
}

// TryConsume checks both per-second and per-minute limits.
func (ul *UserRateLimiter) TryConsume() (bool, time.Duration) {
	ul.lastAccess.Store(time.Now().Unix())

	// Check per-second limit first
	if allowed, retryAfter := ul.perSecond.TryConsume(); !allowed {
		return false, retryAfter
	}

	// Check per-minute limit
	if allowed, retryAfter := ul.perMinute.TryIncrement(); !allowed {
		// Refund the per-second token since we're rejecting
		ul.perSecond.mu.Lock()
		ul.perSecond.tokens = min(ul.perSecond.maxTokens, ul.perSecond.tokens+1)
		ul.perSecond.mu.Unlock()
		return false, retryAfter
	}

	return true, 0
}

// LastAccess returns the Unix timestamp of the last access.
func (ul *UserRateLimiter) LastAccess() int64 {
	return ul.lastAccess.Load()
}

// ContestRateLimiter handles per-contest rate limiting.
type ContestRateLimiter struct {
	perSecond  *SlidingWindowCounter
	lastAccess atomic.Int64
}

// NewContestRateLimiter creates a new contest rate limiter.
func NewContestRateLimiter(perSecond int) *ContestRateLimiter {
	cl := &ContestRateLimiter{
		perSecond: NewSlidingWindowCounter(time.Second, int64(perSecond)),
	}
	cl.lastAccess.Store(time.Now().Unix())
	return cl
}

// TryConsume checks the per-second limit for the contest.
func (cl *ContestRateLimiter) TryConsume() (bool, time.Duration) {
	cl.lastAccess.Store(time.Now().Unix())
	return cl.perSecond.TryIncrement()
}

// UpdateRate updates the rate limit for this contest limiter in-place,
// preserving the current window state and ensuring in-flight operations
// that hold a reference to this limiter remain consistent.
func (cl *ContestRateLimiter) UpdateRate(perSecond int) {
	cl.perSecond.UpdateRate(int64(perSecond))
}

// LastAccess returns the Unix timestamp of the last access.
func (cl *ContestRateLimiter) LastAccess() int64 {
	return cl.lastAccess.Load()
}

// ParticipantCountFunc is a callback that returns the number of participants
// for a given contest. Used by the dynamic rate limiter to compute per-contest
// rate limits based on contest size.
type ParticipantCountFunc func(ctx context.Context, contestID string) (int, error)

// OrderRateLimiter is the multi-tier rate limiter for order submissions.
type OrderRateLimiter struct {
	config  RateLimitConfig
	metrics *RateLimitMetrics
	logger  *zap.Logger

	// User rate limiters: key = "contestID:userID"
	userLimiters sync.Map

	// Contest rate limiters: key = contestID
	contestLimiters sync.Map

	// Global rate limiter (lock-free for minimal contention on hot path)
	globalLimiter *AtomicTokenBucket

	// Dynamic contest limiting
	participantCountFn ParticipantCountFunc

	// Cleanup
	stopCleanup chan struct{}
	cleanupWg   sync.WaitGroup
}

// NewOrderRateLimiter creates a new multi-tier order rate limiter.
func NewOrderRateLimiter(config RateLimitConfig, metrics *RateLimitMetrics) *OrderRateLimiter {
	return &OrderRateLimiter{
		config:        config,
		metrics:       metrics,
		globalLimiter: NewAtomicTokenBucket(config.GlobalPerSecond, config.GlobalPerSecond),
		stopCleanup:   make(chan struct{}),
	}
}

// SetLogger sets the logger for the rate limiter.
func (rl *OrderRateLimiter) SetLogger(logger *zap.Logger) {
	rl.logger = logger
}

// SetParticipantCountFunc sets the callback used to look up participant counts
// for dynamic contest rate limiting.
func (rl *OrderRateLimiter) SetParticipantCountFunc(fn ParticipantCountFunc) {
	rl.participantCountFn = fn
}

// SetContestLimit sets or updates the per-contest rate limit for a specific contest.
// If a limiter already exists, it updates the rate in-place to preserve window state
// and ensure in-flight operations remain consistent.
func (rl *OrderRateLimiter) SetContestLimit(contestID string, ratePerSecond int) {
	if v, ok := rl.contestLimiters.Load(contestID); ok {
		// Update in-place to preserve window state
		existing := v.(*ContestRateLimiter)
		existing.UpdateRate(ratePerSecond)
	} else {
		limiter := NewContestRateLimiter(ratePerSecond)
		rl.contestLimiters.Store(contestID, limiter)
	}
	if rl.logger != nil {
		rl.logger.Info("Dynamic contest rate limit set",
			zap.String("contest_id", contestID),
			zap.Int("rate_per_second", ratePerSecond))
	}
}

// computeDynamicLimit calculates the dynamic rate limit for a contest based on
// participant count: max(baseRate, participantCount * multiplier).
func (rl *OrderRateLimiter) computeDynamicLimit(participantCount int) int {
	computed := participantCount * rl.config.ContestLimitMultiplier
	if computed < rl.config.ContestLimitBaseRate {
		return rl.config.ContestLimitBaseRate
	}
	return computed
}

// Check validates an order against all rate limit tiers.
// Returns a RateLimitResult indicating whether the order is allowed.
func (rl *OrderRateLimiter) Check(contestID, userID string) RateLimitResult {
	// 1. Check global rate limit first (fastest check)
	if allowed, retryAfter := rl.globalLimiter.TryConsume(); !allowed {
		rl.recordRateLimited(RateLimitScopeGlobal, contestID)
		return RateLimitResult{
			Allowed:    false,
			Scope:      RateLimitScopeGlobal,
			ContestID:  contestID,
			RetryAfter: retryAfter,
		}
	}

	// 2. Check contest rate limit
	contestLimiter := rl.getOrCreateContestLimiter(contestID)
	if allowed, retryAfter := contestLimiter.TryConsume(); !allowed {
		// Refund global token
		rl.refundGlobalToken()
		rl.recordRateLimited(RateLimitScopeContest, contestID)
		return RateLimitResult{
			Allowed:    false,
			Scope:      RateLimitScopeContest,
			ContestID:  contestID,
			RetryAfter: retryAfter,
		}
	}

	// 3. Check user rate limit (most granular)
	userLimiter := rl.getOrCreateUserLimiter(contestID, userID)
	if allowed, retryAfter := userLimiter.TryConsume(); !allowed {
		// Refund global and contest tokens
		rl.refundGlobalToken()
		rl.refundContestToken(contestID)
		rl.recordRateLimited(RateLimitScopeUser, contestID)
		return RateLimitResult{
			Allowed:    false,
			Scope:      RateLimitScopeUser,
			ContestID:  contestID,
			RetryAfter: retryAfter,
		}
	}

	return RateLimitResult{Allowed: true}
}

// getOrCreateUserLimiter gets or creates a user rate limiter.
func (rl *OrderRateLimiter) getOrCreateUserLimiter(contestID, userID string) *UserRateLimiter {
	key := contestID + ":" + userID
	if v, ok := rl.userLimiters.Load(key); ok {
		return v.(*UserRateLimiter)
	}

	limiter := NewUserRateLimiter(rl.config.UserPerSecond, rl.config.UserPerMinute)
	actual, _ := rl.userLimiters.LoadOrStore(key, limiter)
	return actual.(*UserRateLimiter)
}

// getOrCreateContestLimiter gets or creates a contest rate limiter.
// When dynamic contest limits are enabled and a participant count function is
// available, it computes the rate based on the number of participants.
func (rl *OrderRateLimiter) getOrCreateContestLimiter(contestID string) *ContestRateLimiter {
	if v, ok := rl.contestLimiters.Load(contestID); ok {
		return v.(*ContestRateLimiter)
	}

	rate := rl.config.ContestPerSecond // fallback: static config value

	if rl.config.DynamicContestLimits && rl.participantCountFn != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		count, err := rl.participantCountFn(ctx, contestID)
		cancel()
		if err == nil && count > 0 {
			rate = rl.computeDynamicLimit(count)
			if rl.logger != nil {
				rl.logger.Info("Dynamic contest rate limit computed",
					zap.String("contest_id", contestID),
					zap.Int("participant_count", count),
					zap.Int("rate_per_second", rate))
			}
		} else if err != nil && rl.logger != nil {
			rl.logger.Warn("Failed to get participant count for dynamic rate limit, using static config",
				zap.String("contest_id", contestID),
				zap.Error(err),
				zap.Int("fallback_rate", rate))
		}
	}

	limiter := NewContestRateLimiter(rate)
	actual, _ := rl.contestLimiters.LoadOrStore(contestID, limiter)
	return actual.(*ContestRateLimiter)
}

// refundGlobalToken refunds a token to the global limiter.
func (rl *OrderRateLimiter) refundGlobalToken() {
	rl.globalLimiter.Refund()
}

// refundContestToken refunds a token to a contest's limiter.
func (rl *OrderRateLimiter) refundContestToken(contestID string) {
	if v, ok := rl.contestLimiters.Load(contestID); ok {
		limiter := v.(*ContestRateLimiter)
		limiter.perSecond.mu.Lock()
		if limiter.perSecond.current > 0 {
			limiter.perSecond.current--
		}
		limiter.perSecond.mu.Unlock()
	}
}

// recordRateLimited records a rate limit event in metrics.
func (rl *OrderRateLimiter) recordRateLimited(scope RateLimitScope, contestID string) {
	if rl.metrics != nil {
		rl.metrics.OrdersRateLimited.WithLabelValues(string(scope), contestID).Inc()
	}
}

// RemoveUser removes the rate limiter for a specific user in a contest.
// Call this when a user disconnects.
func (rl *OrderRateLimiter) RemoveUser(contestID, userID string) {
	key := contestID + ":" + userID
	rl.userLimiters.Delete(key)
}

// RemoveContest removes all rate limiters for a contest.
// Call this when a contest ends.
func (rl *OrderRateLimiter) RemoveContest(contestID string) {
	// Remove contest limiter
	rl.contestLimiters.Delete(contestID)

	// Remove all user limiters for this contest
	rl.userLimiters.Range(func(key, value interface{}) bool {
		keyStr := key.(string)
		// Check if key starts with contestID:
		if len(keyStr) > len(contestID) && keyStr[:len(contestID)+1] == contestID+":" {
			rl.userLimiters.Delete(key)
		}
		return true
	})
}

// StartCleanup starts background goroutines that periodically clean up
// stale rate limiter entries (entries not accessed for more than 5 minutes)
// and refresh dynamic contest limits.
func (rl *OrderRateLimiter) StartCleanup(cleanupInterval time.Duration) {
	if cleanupInterval <= 0 {
		cleanupInterval = time.Minute
	}

	rl.cleanupWg.Add(1)
	infra.SafeGo(rl.logger, "rate-limiter-cleanup", func() {
		defer rl.cleanupWg.Done()
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-rl.stopCleanup:
				return
			case <-ticker.C:
				rl.cleanupStaleEntries()
			}
		}
	})

	// Start periodic refresh of dynamic contest limits
	if rl.config.DynamicContestLimits && rl.config.ContestLimitRefreshSecs > 0 {
		refreshInterval := time.Duration(rl.config.ContestLimitRefreshSecs) * time.Second
		rl.cleanupWg.Add(1)
		infra.SafeGo(rl.logger, "rate-limiter-refresh", func() {
			defer rl.cleanupWg.Done()
			ticker := time.NewTicker(refreshInterval)
			defer ticker.Stop()

			for {
				select {
				case <-rl.stopCleanup:
					return
				case <-ticker.C:
					rl.refreshDynamicContestLimits()
				}
			}
		})
	}
}

// refreshDynamicContestLimits recalculates the rate limit for all active
// contest limiters based on the current participant count. This picks up
// new participants who joined mid-contest.
func (rl *OrderRateLimiter) refreshDynamicContestLimits() {
	if rl.participantCountFn == nil {
		return
	}

	rl.contestLimiters.Range(func(key, value interface{}) bool {
		contestID := key.(string)
		existing := value.(*ContestRateLimiter)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		count, err := rl.participantCountFn(ctx, contestID)
		cancel()

		if err != nil {
			if rl.logger != nil {
				rl.logger.Warn("Failed to refresh dynamic contest limit",
					zap.String("contest_id", contestID),
					zap.Error(err))
			}
			return true
		}

		if count > 0 {
			newRate := rl.computeDynamicLimit(count)
			// Update rate in-place to preserve window state and ensure
			// in-flight refunds target the same limiter instance
			existing.UpdateRate(newRate)
			if rl.logger != nil {
				rl.logger.Debug("Refreshed dynamic contest rate limit",
					zap.String("contest_id", contestID),
					zap.Int("participant_count", count),
					zap.Int("rate_per_second", newRate))
			}
		}
		return true
	})
}

// StopCleanup stops the cleanup goroutine.
func (rl *OrderRateLimiter) StopCleanup() {
	close(rl.stopCleanup)
	rl.cleanupWg.Wait()
}

// cleanupStaleEntries removes entries that haven't been accessed in 5 minutes.
func (rl *OrderRateLimiter) cleanupStaleEntries() {
	cutoff := time.Now().Add(-5 * time.Minute).Unix()

	// Clean up stale user limiters
	rl.userLimiters.Range(func(key, value interface{}) bool {
		limiter := value.(*UserRateLimiter)
		if limiter.LastAccess() < cutoff {
			rl.userLimiters.Delete(key)
		}
		return true
	})

	// Clean up stale contest limiters
	rl.contestLimiters.Range(func(key, value interface{}) bool {
		limiter := value.(*ContestRateLimiter)
		if limiter.LastAccess() < cutoff {
			rl.contestLimiters.Delete(key)
		}
		return true
	})
}

// GetContestEffectiveRates returns the effective rate limit for each active contest.
func (rl *OrderRateLimiter) GetContestEffectiveRates() map[string]int64 {
	rates := make(map[string]int64)
	rl.contestLimiters.Range(func(key, value interface{}) bool {
		contestID := key.(string)
		limiter := value.(*ContestRateLimiter)
		rates[contestID] = limiter.perSecond.MaxRequests()
		return true
	})
	return rates
}

// GetStats returns statistics about the rate limiter state.
func (rl *OrderRateLimiter) GetStats() map[string]interface{} {
	userCount := 0
	rl.userLimiters.Range(func(key, value interface{}) bool {
		userCount++
		return true
	})

	contestCount := 0
	rl.contestLimiters.Range(func(key, value interface{}) bool {
		contestCount++
		return true
	})

	globalMilliTokens := rl.globalLimiter.milliTokens.Load()

	return map[string]interface{}{
		"user_limiters_count":    userCount,
		"contest_limiters_count": contestCount,
		"global_tokens":          float64(globalMilliTokens) / 1000.0,
		"config": map[string]interface{}{
			"user_per_second":          rl.config.UserPerSecond,
			"user_per_minute":          rl.config.UserPerMinute,
			"contest_per_second":       rl.config.ContestPerSecond,
			"global_per_second":        rl.config.GlobalPerSecond,
			"dynamic_contest_limits":   rl.config.DynamicContestLimits,
			"contest_limit_base_rate":  rl.config.ContestLimitBaseRate,
			"contest_limit_multiplier": rl.config.ContestLimitMultiplier,
		},
	}
}
