package ratelimit

import (
	"sync"
	"time"
)

// UserRateLimiter is a thread-safe, in-memory rate limiter that maintains
// per-user rate limits using a token bucket algorithm. It automatically
// cleans up inactive user entries to prevent memory leaks.
type UserRateLimiter struct {
	config   Config
	limiters sync.Map // map[string]*tokenBucket
	metrics  *Metrics

	// cleanup management
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// tokenBucket implements a token bucket rate limiter for a single user.
type tokenBucket struct {
	tokens     float64
	lastUpdate time.Time
	lastAccess time.Time
	mu         sync.Mutex

	// config cached from parent
	rate      float64
	burstSize float64
	window    time.Duration
}

// NewUserLimiter creates a new in-memory user rate limiter.
func NewUserLimiter(cfg Config) *UserRateLimiter {
	cfg = cfg.WithDefaults()

	ul := &UserRateLimiter{
		config:      cfg,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go ul.cleanupLoop()

	return ul
}

// NewUserLimiterWithMetrics creates a new user rate limiter with Prometheus metrics.
func NewUserLimiterWithMetrics(cfg Config, metrics *Metrics) *UserRateLimiter {
	ul := NewUserLimiter(cfg)
	ul.metrics = metrics
	return ul
}

// Allow checks if a single request is allowed for the given user ID.
func (ul *UserRateLimiter) Allow(userID string) bool {
	return ul.AllowN(userID, 1)
}

// AllowN checks if n requests are allowed for the given user ID.
func (ul *UserRateLimiter) AllowN(userID string, n int) bool {
	if userID == "" || n <= 0 {
		return false
	}

	bucket := ul.getBucket(userID)
	allowed := bucket.takeN(n)

	if ul.metrics != nil {
		if allowed {
			ul.metrics.RecordAllowed(ul.config.KeyPrefix, userID)
		} else {
			ul.metrics.RecordExceeded(ul.config.KeyPrefix, userID)
		}
	}

	return allowed
}

// Reset clears the rate limit state for the given user ID.
func (ul *UserRateLimiter) Reset(userID string) {
	ul.limiters.Delete(userID)
}

// Remaining returns the number of remaining requests for the given user ID.
func (ul *UserRateLimiter) Remaining(userID string) int {
	bucket := ul.getBucket(userID)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.refill()
	return int(bucket.tokens)
}

// RetryAfter returns the duration until the next request is allowed.
// Returns 0 if requests are currently allowed.
func (ul *UserRateLimiter) RetryAfter(userID string) time.Duration {
	bucket := ul.getBucket(userID)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.refill()
	if bucket.tokens >= 1 {
		return 0
	}

	// Calculate time to get one token
	// bucket.rate is tokens per nanosecond, so tokensNeeded/rate gives nanoseconds
	tokensNeeded := 1 - bucket.tokens
	return time.Duration(tokensNeeded / bucket.rate)
}

// Check returns detailed rate limit information without consuming tokens.
func (ul *UserRateLimiter) Check(userID string) LimitInfo {
	bucket := ul.getBucket(userID)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.refill()

	remaining := int(bucket.tokens)
	allowed := remaining >= 1

	var retryAfter time.Duration
	if !allowed {
		tokensNeeded := 1 - bucket.tokens
		retryAfter = time.Duration(tokensNeeded / bucket.rate)
	}

	return LimitInfo{
		Allowed:    allowed,
		Limit:      int(bucket.burstSize),
		Remaining:  remaining,
		ResetAt:    bucket.lastUpdate.Add(bucket.window),
		RetryAfter: retryAfter,
	}
}

// Close stops the cleanup goroutine and releases resources.
func (ul *UserRateLimiter) Close() error {
	close(ul.stopCleanup)
	<-ul.cleanupDone
	return nil
}

// ActiveUsers returns the number of users currently being tracked.
func (ul *UserRateLimiter) ActiveUsers() int {
	count := 0
	ul.limiters.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// getBucket retrieves or creates a token bucket for the user.
func (ul *UserRateLimiter) getBucket(userID string) *tokenBucket {
	if existing, ok := ul.limiters.Load(userID); ok {
		bucket := existing.(*tokenBucket)
		bucket.mu.Lock()
		bucket.lastAccess = time.Now()
		bucket.mu.Unlock()
		return bucket
	}

	// Create new bucket
	bucket := &tokenBucket{
		tokens:     float64(ul.config.BurstSize),
		lastUpdate: time.Now(),
		lastAccess: time.Now(),
		rate:       float64(ul.config.Rate) / float64(ul.config.Window),
		burstSize:  float64(ul.config.BurstSize),
		window:     ul.config.Window,
	}

	// Store or load existing (handles race condition)
	actual, loaded := ul.limiters.LoadOrStore(userID, bucket)
	if loaded {
		return actual.(*tokenBucket)
	}

	return bucket
}

// takeN attempts to take n tokens from the bucket.
func (b *tokenBucket) takeN(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.refill()
	b.lastAccess = time.Now()

	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}
	return false
}

// refill adds tokens based on elapsed time (must be called with lock held).
func (b *tokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.lastUpdate)

	// Add tokens based on elapsed time
	tokensToAdd := float64(elapsed) * b.rate
	b.tokens = min(b.tokens+tokensToAdd, b.burstSize)
	b.lastUpdate = now
}

// cleanupLoop periodically removes inactive user entries.
func (ul *UserRateLimiter) cleanupLoop() {
	defer close(ul.cleanupDone)

	ticker := time.NewTicker(ul.config.CleanupInterval)
	defer ticker.Stop()

	// Inactive threshold: 2x the window duration
	inactiveThreshold := ul.config.Window * 2
	if inactiveThreshold < time.Minute {
		inactiveThreshold = time.Minute
	}

	for {
		select {
		case <-ul.stopCleanup:
			return
		case <-ticker.C:
			ul.cleanup(inactiveThreshold)
		}
	}
}

// cleanup removes entries that haven't been accessed recently.
func (ul *UserRateLimiter) cleanup(threshold time.Duration) {
	now := time.Now()
	var toDelete []string

	ul.limiters.Range(func(key, value interface{}) bool {
		bucket := value.(*tokenBucket)
		bucket.mu.Lock()
		inactive := now.Sub(bucket.lastAccess) > threshold
		bucket.mu.Unlock()

		if inactive {
			toDelete = append(toDelete, key.(string))
		}
		return true
	})

	for _, key := range toDelete {
		ul.limiters.Delete(key)
	}
}

// MultiLimiter combines multiple rate limiters and requires all to allow.
type MultiLimiter struct {
	limiters []RateLimiter
}

// NewMultiLimiter creates a limiter that requires all sub-limiters to allow.
func NewMultiLimiter(limiters ...RateLimiter) *MultiLimiter {
	return &MultiLimiter{limiters: limiters}
}

// Allow returns true only if all limiters allow the request.
func (ml *MultiLimiter) Allow(key string) bool {
	return ml.AllowN(key, 1)
}

// AllowN returns true only if all limiters allow n requests.
// It first checks that all limiters have sufficient capacity before consuming
// tokens from any of them, preventing token loss on partial failures.
func (ml *MultiLimiter) AllowN(key string, n int) bool {
	// Phase 1: Check all limiters have capacity (without consuming)
	for _, limiter := range ml.limiters {
		if limiter.Remaining(key) < n {
			return false
		}
	}
	// Phase 2: Consume from all limiters
	for _, limiter := range ml.limiters {
		if !limiter.AllowN(key, n) {
			return false
		}
	}
	return true
}

// Reset clears all limiters for the key.
func (ml *MultiLimiter) Reset(key string) {
	for _, limiter := range ml.limiters {
		limiter.Reset(key)
	}
}

// Remaining returns the minimum remaining across all limiters.
func (ml *MultiLimiter) Remaining(key string) int {
	if len(ml.limiters) == 0 {
		return 0
	}

	minRemaining := ml.limiters[0].Remaining(key)
	for _, limiter := range ml.limiters[1:] {
		if r := limiter.Remaining(key); r < minRemaining {
			minRemaining = r
		}
	}
	return minRemaining
}

// RetryAfter returns the maximum retry duration across all limiters.
func (ml *MultiLimiter) RetryAfter(key string) time.Duration {
	var maxRetry time.Duration
	for _, limiter := range ml.limiters {
		if r := limiter.RetryAfter(key); r > maxRetry {
			maxRetry = r
		}
	}
	return maxRetry
}

// leakyBucketEntry holds state for a single key in the LeakyBucket.
type leakyBucketEntry struct {
	tokens     int64
	lastLeak   int64 // UnixNano
	lastAccess int64 // UnixNano
	mu         sync.Mutex
}

// LeakyBucket implements a leaky bucket rate limiter for smoothing request rates.
// It maintains per-key buckets so that each key has independent rate limiting.
type LeakyBucket struct {
	rate       float64 // tokens per nanosecond
	bucketSize int64
	window     time.Duration
	buckets    sync.Map // map[string]*leakyBucketEntry

	// cleanup management
	cleanupInterval time.Duration
	stopCleanup     chan struct{}
	cleanupDone     chan struct{}
}

// NewLeakyBucket creates a new leaky bucket rate limiter.
func NewLeakyBucket(cfg Config) *LeakyBucket {
	cfg = cfg.WithDefaults()
	lb := &LeakyBucket{
		rate:            float64(cfg.Rate) / float64(cfg.Window),
		bucketSize:      int64(cfg.BurstSize),
		window:          cfg.Window,
		cleanupInterval: cfg.CleanupInterval,
		stopCleanup:     make(chan struct{}),
		cleanupDone:     make(chan struct{}),
	}

	go lb.cleanupLoop()

	return lb
}

// Allow checks if a request is allowed.
func (lb *LeakyBucket) Allow(key string) bool {
	return lb.AllowN(key, 1)
}

// getEntry retrieves or creates an entry for the given key.
func (lb *LeakyBucket) getEntry(key string) *leakyBucketEntry {
	if existing, ok := lb.buckets.Load(key); ok {
		return existing.(*leakyBucketEntry)
	}

	now := time.Now().UnixNano()
	entry := &leakyBucketEntry{
		tokens:     0,
		lastLeak:   now,
		lastAccess: now,
	}

	actual, loaded := lb.buckets.LoadOrStore(key, entry)
	if loaded {
		return actual.(*leakyBucketEntry)
	}
	return entry
}

// AllowN checks if n requests are allowed.
func (lb *LeakyBucket) AllowN(key string, n int) bool {
	entry := lb.getEntry(key)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now().UnixNano()

	// Leak tokens based on elapsed time
	elapsed := now - entry.lastLeak
	leaked := int64(float64(elapsed) * lb.rate)

	currentTokens := max(int64(0), entry.tokens-leaked)

	// Check if we can add n tokens
	if currentTokens+int64(n) > lb.bucketSize {
		return false
	}

	entry.tokens = currentTokens + int64(n)
	entry.lastLeak = now
	entry.lastAccess = now
	return true
}

// Reset clears the bucket for the given key.
func (lb *LeakyBucket) Reset(key string) {
	lb.buckets.Delete(key)
}

// Remaining returns the remaining capacity for the given key.
func (lb *LeakyBucket) Remaining(key string) int {
	existing, ok := lb.buckets.Load(key)
	if !ok {
		return int(lb.bucketSize)
	}

	entry := existing.(*leakyBucketEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now().UnixNano()
	elapsed := now - entry.lastLeak
	leaked := int64(float64(elapsed) * lb.rate)
	currentTokens := max(int64(0), entry.tokens-leaked)

	return int(lb.bucketSize - currentTokens)
}

// RetryAfter returns time until bucket has capacity for the given key.
func (lb *LeakyBucket) RetryAfter(key string) time.Duration {
	existing, ok := lb.buckets.Load(key)
	if !ok {
		return 0
	}

	entry := existing.(*leakyBucketEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now().UnixNano()
	elapsed := now - entry.lastLeak
	leaked := int64(float64(elapsed) * lb.rate)
	currentTokens := max(int64(0), entry.tokens-leaked)

	if currentTokens < lb.bucketSize {
		return 0
	}
	tokensToLeak := float64(currentTokens - lb.bucketSize + 1)
	return time.Duration(tokensToLeak / lb.rate)
}

// Close stops the cleanup goroutine and releases resources.
func (lb *LeakyBucket) Close() error {
	close(lb.stopCleanup)
	<-lb.cleanupDone
	return nil
}

// cleanupLoop periodically removes inactive key entries.
func (lb *LeakyBucket) cleanupLoop() {
	defer close(lb.cleanupDone)

	ticker := time.NewTicker(lb.cleanupInterval)
	defer ticker.Stop()

	threshold := lb.window * 2
	if threshold < time.Minute {
		threshold = time.Minute
	}

	for {
		select {
		case <-lb.stopCleanup:
			return
		case <-ticker.C:
			lb.cleanup(threshold)
		}
	}
}

// cleanup removes entries that haven't been accessed recently.
func (lb *LeakyBucket) cleanup(threshold time.Duration) {
	cutoff := time.Now().Add(-threshold).UnixNano()
	var toDelete []string

	lb.buckets.Range(func(key, value interface{}) bool {
		entry := value.(*leakyBucketEntry)
		entry.mu.Lock()
		inactive := entry.lastAccess < cutoff
		entry.mu.Unlock()

		if inactive {
			toDelete = append(toDelete, key.(string))
		}
		return true
	})

	for _, key := range toDelete {
		lb.buckets.Delete(key)
	}
}

