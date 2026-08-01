package ratelimit

import (
	"sync"
	"time"
)

// WebSocketLimiter provides rate limiting for WebSocket messages.
// It's optimized for high-frequency message checking with minimal overhead.
type WebSocketLimiter struct {
	config    Config
	limiters  sync.Map // map[string]*wsTokenBucket
	metrics   *Metrics
	stopCh    chan struct{}
	stoppedCh chan struct{}
}

// wsTokenBucket is an optimized token bucket for WebSocket messages.
// It uses atomic operations where possible for better performance.
type wsTokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	lastUpdate int64 // Unix nanoseconds
	lastAccess int64 // Unix nanoseconds

	// Pre-calculated values for fast refill
	tokensPerNano float64
	maxTokens     float64
}

// NewWebSocketLimiter creates a new WebSocket rate limiter.
// Default configuration: 50 messages/second with burst of 10.
func NewWebSocketLimiter(cfg Config) *WebSocketLimiter {
	cfg = cfg.WithDefaults()

	wsl := &WebSocketLimiter{
		config:    cfg,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}

	go wsl.cleanupLoop()

	return wsl
}

// NewWebSocketLimiterWithMetrics creates a WebSocket limiter with metrics.
func NewWebSocketLimiterWithMetrics(cfg Config, metrics *Metrics) *WebSocketLimiter {
	wsl := NewWebSocketLimiter(cfg)
	wsl.metrics = metrics
	return wsl
}

// Allow checks if a message is allowed for the given connection/user.
func (wsl *WebSocketLimiter) Allow(key string) bool {
	return wsl.AllowN(key, 1)
}

// AllowN checks if n messages are allowed.
func (wsl *WebSocketLimiter) AllowN(key string, n int) bool {
	if key == "" || n <= 0 {
		return false
	}

	bucket := wsl.getBucket(key)
	allowed := bucket.take(n)

	if wsl.metrics != nil {
		if allowed {
			wsl.metrics.RecordAllowed(wsl.config.KeyPrefix, key)
		} else {
			wsl.metrics.RecordExceeded(wsl.config.KeyPrefix, key)
		}
	}

	return allowed
}

// Reset clears the rate limit for a key.
func (wsl *WebSocketLimiter) Reset(key string) {
	wsl.limiters.Delete(key)
}

// Remaining returns remaining messages allowed.
func (wsl *WebSocketLimiter) Remaining(key string) int {
	bucket := wsl.getBucket(key)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.refill()
	return int(bucket.tokens)
}

// RetryAfter returns duration until next message is allowed.
func (wsl *WebSocketLimiter) RetryAfter(key string) time.Duration {
	bucket := wsl.getBucket(key)
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	bucket.refill()
	if bucket.tokens >= 1 {
		return 0
	}

	tokensNeeded := 1 - bucket.tokens
	return time.Duration(tokensNeeded / bucket.tokensPerNano)
}

// Close stops the cleanup goroutine.
func (wsl *WebSocketLimiter) Close() error {
	close(wsl.stopCh)
	<-wsl.stoppedCh
	return nil
}

// ConnectionCount returns the number of active connections being tracked.
func (wsl *WebSocketLimiter) ConnectionCount() int {
	count := 0
	wsl.limiters.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// getBucket retrieves or creates a bucket for the key.
func (wsl *WebSocketLimiter) getBucket(key string) *wsTokenBucket {
	if existing, ok := wsl.limiters.Load(key); ok {
		bucket := existing.(*wsTokenBucket)
		bucket.mu.Lock()
		bucket.lastAccess = time.Now().UnixNano()
		bucket.mu.Unlock()
		return bucket
	}

	now := time.Now().UnixNano()
	bucket := &wsTokenBucket{
		tokens:        float64(wsl.config.BurstSize),
		lastUpdate:    now,
		lastAccess:    now,
		tokensPerNano: float64(wsl.config.Rate) / float64(wsl.config.Window),
		maxTokens:     float64(wsl.config.BurstSize),
	}

	actual, loaded := wsl.limiters.LoadOrStore(key, bucket)
	if loaded {
		return actual.(*wsTokenBucket)
	}

	return bucket
}

// take attempts to take n tokens.
func (b *wsTokenBucket) take(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.refill()
	b.lastAccess = time.Now().UnixNano()

	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}
	return false
}

// refill adds tokens based on elapsed time (must hold lock).
func (b *wsTokenBucket) refill() {
	now := time.Now().UnixNano()
	elapsed := now - b.lastUpdate

	tokensToAdd := float64(elapsed) * b.tokensPerNano
	b.tokens = min(b.tokens+tokensToAdd, b.maxTokens)
	b.lastUpdate = now
}

// cleanupLoop removes inactive connections.
func (wsl *WebSocketLimiter) cleanupLoop() {
	defer close(wsl.stoppedCh)

	ticker := time.NewTicker(wsl.config.CleanupInterval)
	defer ticker.Stop()

	// Clean up connections inactive for 2x the window
	threshold := wsl.config.Window * 2
	if threshold < 10*time.Second {
		threshold = 10 * time.Second
	}

	for {
		select {
		case <-wsl.stopCh:
			return
		case <-ticker.C:
			wsl.cleanup(threshold)
		}
	}
}

func (wsl *WebSocketLimiter) cleanup(threshold time.Duration) {
	cutoff := time.Now().Add(-threshold).UnixNano()
	var toDelete []string

	wsl.limiters.Range(func(key, value interface{}) bool {
		bucket := value.(*wsTokenBucket)
		bucket.mu.Lock()
		inactive := bucket.lastAccess < cutoff
		bucket.mu.Unlock()

		if inactive {
			toDelete = append(toDelete, key.(string))
		}
		return true
	})

	for _, key := range toDelete {
		wsl.limiters.Delete(key)
	}
}

// ConnectionLimiter tracks and limits messages per WebSocket connection.
// It maintains state for a single connection.
type ConnectionLimiter struct {
	mu         sync.Mutex
	tokens     float64
	lastUpdate time.Time
	config     Config
	metrics    *Metrics
	connID     string
}

// NewConnectionLimiter creates a limiter for a single connection.
func NewConnectionLimiter(connID string, cfg Config) *ConnectionLimiter {
	cfg = cfg.WithDefaults()

	return &ConnectionLimiter{
		tokens:     float64(cfg.BurstSize),
		lastUpdate: time.Now(),
		config:     cfg,
		connID:     connID,
	}
}

// NewConnectionLimiterWithMetrics creates a connection limiter with metrics.
func NewConnectionLimiterWithMetrics(connID string, cfg Config, metrics *Metrics) *ConnectionLimiter {
	cl := NewConnectionLimiter(connID, cfg)
	cl.metrics = metrics
	return cl
}

// Allow checks if a message is allowed.
func (cl *ConnectionLimiter) Allow() bool {
	return cl.AllowN(1)
}

// AllowN checks if n messages are allowed.
func (cl *ConnectionLimiter) AllowN(n int) bool {
	if n <= 0 {
		return false
	}

	cl.mu.Lock()
	defer cl.mu.Unlock()

	cl.refill()

	if cl.tokens >= float64(n) {
		cl.tokens -= float64(n)
		if cl.metrics != nil {
			cl.metrics.RecordAllowed(cl.config.KeyPrefix, cl.connID)
		}
		return true
	}

	if cl.metrics != nil {
		cl.metrics.RecordExceeded(cl.config.KeyPrefix, cl.connID)
	}
	return false
}

// Remaining returns remaining messages allowed.
func (cl *ConnectionLimiter) Remaining() int {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	cl.refill()
	return int(cl.tokens)
}

// RetryAfter returns duration until next message is allowed.
func (cl *ConnectionLimiter) RetryAfter() time.Duration {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	cl.refill()
	if cl.tokens >= 1 {
		return 0
	}

	rate := float64(cl.config.Rate) / float64(cl.config.Window)
	tokensNeeded := 1 - cl.tokens
	return time.Duration(tokensNeeded / rate * float64(time.Second))
}

// Reset resets the limiter to full capacity.
func (cl *ConnectionLimiter) Reset() {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	cl.tokens = float64(cl.config.BurstSize)
	cl.lastUpdate = time.Now()
}

func (cl *ConnectionLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(cl.lastUpdate)

	rate := float64(cl.config.Rate) / float64(cl.config.Window)
	tokensToAdd := float64(elapsed) * rate

	cl.tokens = min(cl.tokens+tokensToAdd, float64(cl.config.BurstSize))
	cl.lastUpdate = now
}

// MessageTypeRateLimiter applies different limits based on message type.
type MessageTypeRateLimiter struct {
	defaultLimiter RateLimiter
	typeLimiters   map[string]RateLimiter
	mu             sync.RWMutex
}

// NewMessageTypeRateLimiter creates a limiter with per-message-type limits.
func NewMessageTypeRateLimiter(defaultLimiter RateLimiter) *MessageTypeRateLimiter {
	return &MessageTypeRateLimiter{
		defaultLimiter: defaultLimiter,
		typeLimiters:   make(map[string]RateLimiter),
	}
}

// SetTypeLimit sets a limiter for a specific message type.
func (mtl *MessageTypeRateLimiter) SetTypeLimit(messageType string, limiter RateLimiter) {
	mtl.mu.Lock()
	defer mtl.mu.Unlock()
	mtl.typeLimiters[messageType] = limiter
}

// Allow checks if a message of the given type is allowed for the key.
func (mtl *MessageTypeRateLimiter) Allow(key, messageType string) bool {
	mtl.mu.RLock()
	limiter, ok := mtl.typeLimiters[messageType]
	mtl.mu.RUnlock()

	if !ok {
		limiter = mtl.defaultLimiter
	}

	if limiter == nil {
		return true
	}

	return limiter.Allow(key)
}

// AllowN checks if n messages are allowed.
func (mtl *MessageTypeRateLimiter) AllowN(key, messageType string, n int) bool {
	mtl.mu.RLock()
	limiter, ok := mtl.typeLimiters[messageType]
	mtl.mu.RUnlock()

	if !ok {
		limiter = mtl.defaultLimiter
	}

	if limiter == nil {
		return true
	}

	return limiter.AllowN(key, n)
}

// BurstController handles message bursts gracefully.
// It allows temporary bursts while maintaining long-term rate limits.
type BurstController struct {
	shortTermLimiter RateLimiter // Per-second limit
	longTermLimiter  RateLimiter // Per-minute limit
}

// NewBurstController creates a controller with short and long term limits.
func NewBurstController(shortTerm, longTerm Config) *BurstController {
	return &BurstController{
		shortTermLimiter: NewUserLimiter(shortTerm),
		longTermLimiter:  NewUserLimiter(longTerm),
	}
}

// Allow checks both short-term and long-term limits.
// It checks both limiters have capacity before consuming tokens from either,
// preventing token loss when one limiter rejects after the other has consumed.
func (bc *BurstController) Allow(key string) bool {
	if bc.longTermLimiter.Remaining(key) < 1 || bc.shortTermLimiter.Remaining(key) < 1 {
		return false
	}
	if !bc.longTermLimiter.Allow(key) {
		return false
	}
	return bc.shortTermLimiter.Allow(key)
}

// AllowN checks if n messages are allowed.
// It checks both limiters have capacity before consuming tokens from either,
// preventing token loss when one limiter rejects after the other has consumed.
func (bc *BurstController) AllowN(key string, n int) bool {
	if bc.longTermLimiter.Remaining(key) < n || bc.shortTermLimiter.Remaining(key) < n {
		return false
	}
	if !bc.longTermLimiter.AllowN(key, n) {
		return false
	}
	return bc.shortTermLimiter.AllowN(key, n)
}

// Reset resets both limiters.
func (bc *BurstController) Reset(key string) {
	bc.shortTermLimiter.Reset(key)
	bc.longTermLimiter.Reset(key)
}

// Remaining returns the minimum of both limiters.
func (bc *BurstController) Remaining(key string) int {
	short := bc.shortTermLimiter.Remaining(key)
	long := bc.longTermLimiter.Remaining(key)
	if short < long {
		return short
	}
	return long
}

// RetryAfter returns the maximum retry time from both limiters.
func (bc *BurstController) RetryAfter(key string) time.Duration {
	short := bc.shortTermLimiter.RetryAfter(key)
	long := bc.longTermLimiter.RetryAfter(key)
	if short > long {
		return short
	}
	return long
}

