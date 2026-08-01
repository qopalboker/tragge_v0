package server

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"go.uber.org/zap"
)

// Dependency names for circuit breakers
const (
	DepDatabase     = "database"
	DepRedis        = "redis"
	DepRedisCluster = "redis-cluster"
	DepKafka        = "kafka"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = circuitbreaker.ErrCircuitOpen
)

// CircuitBreakers holds all circuit breakers for the leaderboard-worker service.
type CircuitBreakers struct {
	Database     *circuitbreaker.CircuitBreaker
	Redis        *circuitbreaker.CircuitBreaker
	RedisCluster *circuitbreaker.CircuitBreaker
	Kafka        *circuitbreaker.CircuitBreaker

	logger *zap.Logger

	// In-memory fallback cache when Redis is unavailable
	fallbackCache *FallbackLeaderboardCache
}

// FallbackLeaderboardCache provides in-memory caching when Redis is unavailable.
// This allows the service to continue serving stale data during Redis outages.
type FallbackLeaderboardCache struct {
	mu       sync.RWMutex
	entries  map[string]*CachedLeaderboard // contestID -> cached entries
	maxSize  int
	ttl      time.Duration
	lastSeen map[string]time.Time // Track when entries were last accessed
}

// NewFallbackLeaderboardCache creates a new fallback cache with the given configuration.
func NewFallbackLeaderboardCache(maxSize int, ttl time.Duration) *FallbackLeaderboardCache {
	return &FallbackLeaderboardCache{
		entries:  make(map[string]*CachedLeaderboard),
		maxSize:  maxSize,
		ttl:      ttl,
		lastSeen: make(map[string]time.Time),
	}
}

// Get retrieves cached leaderboard entries for a contest.
// Returns nil if not found or expired.
func (c *FallbackLeaderboardCache) Get(contestID string) *CachedLeaderboard {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, ok := c.entries[contestID]
	if !ok {
		return nil
	}

	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	return cached
}

// Set stores leaderboard entries in the fallback cache.
func (c *FallbackLeaderboardCache) Set(contestID string, entries []LeaderboardEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest entries if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	now := time.Now()
	c.entries[contestID] = &CachedLeaderboard{
		Entries:   entries,
		CachedAt:  now,
		ExpiresAt: now.Add(c.ttl),
	}
	c.lastSeen[contestID] = now
}

// evictOldest removes the least recently accessed entry.
// Must be called with mu held.
func (c *FallbackLeaderboardCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, t := range c.lastSeen {
		if oldestKey == "" || t.Before(oldestTime) {
			oldestKey = key
			oldestTime = t
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
		delete(c.lastSeen, oldestKey)
	}
}

// Invalidate removes a specific contest from the cache.
func (c *FallbackLeaderboardCache) Invalidate(contestID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, contestID)
	delete(c.lastSeen, contestID)
}

// Clear removes all entries from the cache.
func (c *FallbackLeaderboardCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CachedLeaderboard)
	c.lastSeen = make(map[string]time.Time)
}

// Size returns the current number of cached entries.
func (c *FallbackLeaderboardCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Cleanup removes expired entries from the cache.
func (c *FallbackLeaderboardCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for contestID, cached := range c.entries {
		if now.After(cached.ExpiresAt) {
			delete(c.entries, contestID)
			delete(c.lastSeen, contestID)
		}
	}
}

// CircuitBreakerConfig holds configuration for circuit breaker initialization.
type CircuitBreakerConfig struct {
	// Logger for circuit breaker events
	Logger *zap.Logger

	// FallbackCacheSize is the max number of contests to cache for fallback
	FallbackCacheSize int

	// FallbackCacheTTL is how long fallback entries remain valid
	FallbackCacheTTL time.Duration

	// OnStateChange is called when any circuit breaker changes state
	OnStateChange func(name string, from, to circuitbreaker.State)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FallbackCacheSize: 1000,
		FallbackCacheTTL:  5 * time.Minute,
	}
}

// NewCircuitBreakers creates and initializes all circuit breakers for the service.
func NewCircuitBreakers(cfg CircuitBreakerConfig) *CircuitBreakers {
	logger := cfg.Logger
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	logger = observability.WrapLogger(logger)

	stateChangeHandler := cfg.OnStateChange
	if stateChangeHandler == nil {
		stateChangeHandler = func(name string, from, to circuitbreaker.State) {
			logger.Warn("Circuit breaker state changed",
				zap.String("circuit", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		}
	}

	// Database circuit - non-critical (snapshots can be delayed)
	// Using lenient config since DB is only used for periodic snapshots
	dbConfig := circuitbreaker.LenientCircuitConfig(DepDatabase)
	dbConfig.OnStateChange = stateChangeHandler

	// Redis standalone circuit - critical for backward compatibility
	redisConfig := circuitbreaker.RedisCircuitConfig(DepRedis)
	redisConfig.OnStateChange = stateChangeHandler

	// Redis Cluster circuit - critical for sharded leaderboards
	redisClusterConfig := circuitbreaker.RedisClusterCircuitConfig(DepRedisCluster)
	redisClusterConfig.OnStateChange = stateChangeHandler

	// Kafka consumer circuit - critical for processing PnL deltas
	kafkaConfig := circuitbreaker.KafkaConsumerCircuitConfig(DepKafka)
	kafkaConfig.OnStateChange = stateChangeHandler

	return &CircuitBreakers{
		Database:      circuitbreaker.New(dbConfig),
		Redis:         circuitbreaker.New(redisConfig),
		RedisCluster:  circuitbreaker.New(redisClusterConfig),
		Kafka:         circuitbreaker.New(kafkaConfig),
		logger:        logger,
		fallbackCache: NewFallbackLeaderboardCache(cfg.FallbackCacheSize, cfg.FallbackCacheTTL),
	}
}

// ExecuteDatabase executes a database operation with circuit breaker protection.
// Database operations are non-critical - failures don't affect service health.
func (cb *CircuitBreakers) ExecuteDatabase(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.Database.ExecuteWithContext(ctx, fn)
}

// ExecuteDatabaseWithResult executes a database operation that returns a result.
func (cb *CircuitBreakers) ExecuteDatabaseWithResult(ctx context.Context, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.Database.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})
	return result, err
}

// ExecuteRedis executes a Redis operation with circuit breaker protection.
// Falls back to in-memory cache if circuit is open.
func (cb *CircuitBreakers) ExecuteRedis(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.Redis.ExecuteWithContext(ctx, fn)
}

// ExecuteRedisWithResult executes a Redis operation that returns a result.
func (cb *CircuitBreakers) ExecuteRedisWithResult(ctx context.Context, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.Redis.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})
	return result, err
}

// ExecuteRedisCluster executes a Redis Cluster operation with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteRedisCluster(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.RedisCluster.ExecuteWithContext(ctx, fn)
}

// ExecuteRedisClusterWithResult executes a Redis Cluster operation that returns a result.
func (cb *CircuitBreakers) ExecuteRedisClusterWithResult(ctx context.Context, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.RedisCluster.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})
	return result, err
}

// ExecuteKafka executes a Kafka operation with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteKafka(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.Kafka.ExecuteWithContext(ctx, fn)
}

// ExecuteKafkaWithResult executes a Kafka operation that returns a result.
func (cb *CircuitBreakers) ExecuteKafkaWithResult(ctx context.Context, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.Kafka.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})
	return result, err
}

// GetLeaderboardWithFallback attempts to get leaderboard from Redis, falls back to cache.
func (cb *CircuitBreakers) GetLeaderboardWithFallback(
	ctx context.Context,
	contestID string,
	fetchFn func(ctx context.Context) ([]LeaderboardEntry, error),
) ([]LeaderboardEntry, error) {
	var entries []LeaderboardEntry

	err := cb.RedisCluster.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fetchErr error
		entries, fetchErr = fetchFn(ctx)
		if fetchErr == nil && len(entries) > 0 {
			// Update fallback cache on successful fetch
			cb.fallbackCache.Set(contestID, entries)
		}
		return fetchErr
	})

	// If circuit is open or failed, try fallback cache
	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			cb.logger.Warn("Redis cluster circuit open, using fallback cache",
				zap.String("contest_id", contestID))
		}

		if cached := cb.fallbackCache.Get(contestID); cached != nil {
			cb.logger.Info("Serving from fallback cache",
				zap.String("contest_id", contestID),
				zap.Time("cached_at", cached.CachedAt))
			return cached.Entries, nil
		}

		return nil, err
	}

	return entries, nil
}

// UpdateScoreWithFallback updates score in Redis with fallback handling.
func (cb *CircuitBreakers) UpdateScoreWithFallback(
	ctx context.Context,
	contestID string,
	userID string,
	updateFn func(ctx context.Context) error,
) error {
	err := cb.RedisCluster.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return updateFn(ctx)
	})

	if err != nil {
		if errors.Is(err, ErrCircuitOpen) {
			cb.logger.Warn("Redis cluster circuit open, score update delayed",
				zap.String("contest_id", contestID),
				zap.String("user_id", userID))
			// Invalidate cache since we couldn't update
			cb.fallbackCache.Invalidate(contestID)
		}
		return err
	}

	return nil
}

// CircuitHealth represents the health status of circuit breakers.
type CircuitHealth struct {
	Database     CircuitStatus `json:"database"`
	Redis        CircuitStatus `json:"redis"`
	RedisCluster CircuitStatus `json:"redis_cluster"`
	Kafka        CircuitStatus `json:"kafka"`
	Overall      string        `json:"overall"`
	Degraded     bool          `json:"degraded"`
}

// CircuitStatus represents the status of a single circuit breaker.
type CircuitStatus struct {
	State        string `json:"state"`
	FailureCount int    `json:"failure_count"`
	Critical     bool   `json:"critical"`
	Healthy      bool   `json:"healthy"`
}

// IsHealthy returns true if all critical circuit breakers are closed.
// Redis (both standalone and cluster) and Kafka are critical.
// Database is non-critical (snapshots can be delayed).
func (cb *CircuitBreakers) IsHealthy() bool {
	// Redis and Kafka are critical
	redisHealthy := cb.Redis.State() != circuitbreaker.StateOpen
	redisClusterHealthy := cb.RedisCluster.State() != circuitbreaker.StateOpen
	kafkaHealthy := cb.Kafka.State() != circuitbreaker.StateOpen

	// All critical circuits must be healthy
	return redisHealthy && redisClusterHealthy && kafkaHealthy
}

// IsDegraded returns true if any circuit breaker is not in closed state.
func (cb *CircuitBreakers) IsDegraded() bool {
	return cb.Database.State() != circuitbreaker.StateClosed ||
		cb.Redis.State() != circuitbreaker.StateClosed ||
		cb.RedisCluster.State() != circuitbreaker.StateClosed ||
		cb.Kafka.State() != circuitbreaker.StateClosed
}

// GetHealth returns detailed health status of all circuit breakers.
func (cb *CircuitBreakers) GetHealth() CircuitHealth {
	dbState := cb.Database.State()
	redisState := cb.Redis.State()
	redisClusterState := cb.RedisCluster.State()
	kafkaState := cb.Kafka.State()

	health := CircuitHealth{
		Database: CircuitStatus{
			State:        dbState.String(),
			FailureCount: cb.Database.FailureCount(),
			Critical:     false, // Database is non-critical
			Healthy:      true,  // Always healthy since non-critical
		},
		Redis: CircuitStatus{
			State:        redisState.String(),
			FailureCount: cb.Redis.FailureCount(),
			Critical:     true,
			Healthy:      redisState != circuitbreaker.StateOpen,
		},
		RedisCluster: CircuitStatus{
			State:        redisClusterState.String(),
			FailureCount: cb.RedisCluster.FailureCount(),
			Critical:     true,
			Healthy:      redisClusterState != circuitbreaker.StateOpen,
		},
		Kafka: CircuitStatus{
			State:        kafkaState.String(),
			FailureCount: cb.Kafka.FailureCount(),
			Critical:     true,
			Healthy:      kafkaState != circuitbreaker.StateOpen,
		},
	}

	// Determine overall status
	if cb.IsHealthy() {
		if cb.IsDegraded() {
			health.Overall = "degraded"
			health.Degraded = true
		} else {
			health.Overall = "healthy"
			health.Degraded = false
		}
	} else {
		health.Overall = "unhealthy"
		health.Degraded = true
	}

	return health
}

// GetMetrics returns metrics for all circuit breakers.
func (cb *CircuitBreakers) GetMetrics() map[string]circuitbreaker.Metrics {
	return map[string]circuitbreaker.Metrics{
		DepDatabase:     cb.Database.Metrics(),
		DepRedis:        cb.Redis.Metrics(),
		DepRedisCluster: cb.RedisCluster.Metrics(),
		DepKafka:        cb.Kafka.Metrics(),
	}
}

// Reset resets all circuit breakers to closed state.
// Use with caution - only for administrative purposes.
func (cb *CircuitBreakers) Reset() {
	cb.Database.Reset()
	cb.Redis.Reset()
	cb.RedisCluster.Reset()
	cb.Kafka.Reset()
	cb.logger.Info("All circuit breakers reset")
}

// ResetCircuit resets a specific circuit breaker by name.
func (cb *CircuitBreakers) ResetCircuit(name string) error {
	switch name {
	case DepDatabase:
		cb.Database.Reset()
	case DepRedis:
		cb.Redis.Reset()
	case DepRedisCluster:
		cb.RedisCluster.Reset()
	case DepKafka:
		cb.Kafka.Reset()
	default:
		return errors.New("unknown circuit breaker: " + name)
	}
	cb.logger.Info("Circuit breaker reset", zap.String("circuit", name))
	return nil
}

// FallbackCache returns the fallback cache for direct access if needed.
func (cb *CircuitBreakers) FallbackCache() *FallbackLeaderboardCache {
	return cb.fallbackCache
}

// StartFallbackCacheCleanup starts periodic cleanup of the fallback cache.
func (cb *CircuitBreakers) StartFallbackCacheCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cb.fallbackCache.Cleanup()
		}
	}
}
