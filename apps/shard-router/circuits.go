package main

import (
	"container/list"
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
	DepDatabase        = "database"
	DepDatabaseReplica = "database-replica"
	DepRedis           = "redis"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = circuitbreaker.ErrCircuitOpen
)

// CircuitBreakers holds all circuit breakers for the shard-router service.
type CircuitBreakers struct {
	// Database circuit breaker for primary database calls (source of truth for shard config)
	Database *circuitbreaker.CircuitBreaker

	// DatabaseReplica circuit breaker for replica database reads
	DatabaseReplica *circuitbreaker.CircuitBreaker

	// Redis circuit breaker for Redis cache operations
	Redis *circuitbreaker.CircuitBreaker

	logger *zap.Logger

	// In-memory fallback cache for graceful degradation
	shardInfoCache *ShardInfoCache
}

// ShardInfoCache provides in-memory fallback for shard information when both
// database and Redis are unavailable. This is the last resort cache.
// Uses a doubly-linked list for O(1) LRU eviction instead of O(n) map scan.
type ShardInfoCache struct {
	mu sync.RWMutex

	// Cached shard assignments: contestID -> list element (value is *lruEntry)
	assignments map[string]*list.Element
	// LRU order: front = most recently used, back = oldest
	lruOrder *list.List

	// Cached shard list
	shards    []*Shard
	shardsAt  time.Time
	shardsTTL time.Duration

	// Configuration
	maxAssignments int
	assignmentTTL  time.Duration
}

// lruEntry holds the key and cached assignment together inside the linked list.
type lruEntry struct {
	contestID  string
	Assignment *ShardAssignment
	CachedAt   time.Time
	ExpiresAt  time.Time
}

// CachedShardAssignment represents a cached shard assignment with metadata.
type CachedShardAssignment struct {
	Assignment *ShardAssignment
	CachedAt   time.Time
	ExpiresAt  time.Time
}

// NewShardInfoCache creates a new in-memory shard info cache.
func NewShardInfoCache(maxAssignments int, assignmentTTL, shardsTTL time.Duration) *ShardInfoCache {
	return &ShardInfoCache{
		assignments:    make(map[string]*list.Element),
		lruOrder:       list.New(),
		maxAssignments: maxAssignments,
		assignmentTTL:  assignmentTTL,
		shardsTTL:      shardsTTL,
	}
}

// GetAssignment retrieves a cached shard assignment.
// Returns nil if not found or expired.
func (c *ShardInfoCache) GetAssignment(contestID string) *ShardAssignment {
	c.mu.RLock()
	defer c.mu.RUnlock()

	elem, ok := c.assignments[contestID]
	if !ok {
		return nil
	}

	entry := elem.Value.(*lruEntry)

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil
	}

	return entry.Assignment
}

// SetAssignment stores a shard assignment in the cache.
func (c *ShardInfoCache) SetAssignment(contestID string, assignment *ShardAssignment) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// If already exists, update in place and move to front
	if elem, ok := c.assignments[contestID]; ok {
		entry := elem.Value.(*lruEntry)
		entry.Assignment = assignment
		entry.CachedAt = now
		entry.ExpiresAt = now.Add(c.assignmentTTL)
		c.lruOrder.MoveToFront(elem)
		return
	}

	// Evict oldest (back of list) if at capacity — O(1)
	if c.lruOrder.Len() >= c.maxAssignments {
		c.evictOldest()
	}

	entry := &lruEntry{
		contestID:  contestID,
		Assignment: assignment,
		CachedAt:   now,
		ExpiresAt:  now.Add(c.assignmentTTL),
	}
	elem := c.lruOrder.PushFront(entry)
	c.assignments[contestID] = elem
}

// evictOldest removes the oldest entry (back of list). O(1). Must be called with lock held.
func (c *ShardInfoCache) evictOldest() {
	back := c.lruOrder.Back()
	if back == nil {
		return
	}
	entry := back.Value.(*lruEntry)
	delete(c.assignments, entry.contestID)
	c.lruOrder.Remove(back)
}

// GetShards retrieves the cached shard list.
// Returns nil if not found or expired.
func (c *ShardInfoCache) GetShards() []*Shard {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.shards == nil || time.Since(c.shardsAt) > c.shardsTTL {
		return nil
	}

	// Return a copy to avoid race conditions
	shardsCopy := make([]*Shard, len(c.shards))
	for i, shard := range c.shards {
		shardCopy := *shard
		shardsCopy[i] = &shardCopy
	}
	return shardsCopy
}

// SetShards stores the shard list in the cache.
func (c *ShardInfoCache) SetShards(shards []*Shard) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Store copies to avoid race conditions
	c.shards = make([]*Shard, len(shards))
	for i, shard := range shards {
		shardCopy := *shard
		c.shards[i] = &shardCopy
	}
	c.shardsAt = time.Now()
}

// GetShard retrieves a specific shard by ID from the cached list.
func (c *ShardInfoCache) GetShard(shardID string) *Shard {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.shards == nil || time.Since(c.shardsAt) > c.shardsTTL {
		return nil
	}

	for _, shard := range c.shards {
		if shard.ID == shardID {
			shardCopy := *shard
			return &shardCopy
		}
	}
	return nil
}

// InvalidateAssignment removes a specific assignment from the cache.
func (c *ShardInfoCache) InvalidateAssignment(contestID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.assignments[contestID]; ok {
		c.lruOrder.Remove(elem)
		delete(c.assignments, contestID)
	}
}

// InvalidateAll clears all cached data.
func (c *ShardInfoCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.assignments = make(map[string]*list.Element)
	c.lruOrder.Init()
	c.shards = nil
}

// Stats returns cache statistics.
func (c *ShardInfoCache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	shardsValid := c.shards != nil && time.Since(c.shardsAt) <= c.shardsTTL

	return map[string]interface{}{
		"assignments_count": len(c.assignments),
		"assignments_max":   c.maxAssignments,
		"shards_cached":     len(c.shards),
		"shards_valid":      shardsValid,
	}
}

// CircuitBreakerConfig holds configuration for circuit breaker initialization.
type CircuitBreakerConfig struct {
	// Logger for circuit breaker events
	Logger *zap.Logger

	// ShardCacheMaxAssignments is the max number of assignments to cache in-memory
	ShardCacheMaxAssignments int

	// ShardCacheAssignmentTTL is how long assignment cache entries remain valid
	ShardCacheAssignmentTTL time.Duration

	// ShardCacheShardsTTL is how long the shard list cache remains valid
	ShardCacheShardsTTL time.Duration

	// OnStateChange is called when any circuit breaker changes state
	OnStateChange func(name string, from, to circuitbreaker.State)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		ShardCacheMaxAssignments: 10000,
		ShardCacheAssignmentTTL:  5 * time.Minute,
		ShardCacheShardsTTL:      30 * time.Second,
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

	// Database primary circuit - source of truth for shard configuration
	dbConfig := circuitbreaker.DatabaseCircuitConfig(DepDatabase)
	dbConfig.OnStateChange = stateChangeHandler

	// Database replica circuit - for read queries, can fallback to primary
	dbReplicaConfig := circuitbreaker.DatabaseReplicaCircuitConfig(DepDatabaseReplica)
	dbReplicaConfig.OnStateChange = stateChangeHandler

	// Redis circuit - for caching shard info
	redisConfig := circuitbreaker.RedisCircuitConfig(DepRedis)
	redisConfig.OnStateChange = stateChangeHandler

	return &CircuitBreakers{
		Database:        circuitbreaker.New(dbConfig),
		DatabaseReplica: circuitbreaker.New(dbReplicaConfig),
		Redis:           circuitbreaker.New(redisConfig),
		logger:          logger,
		shardInfoCache: NewShardInfoCache(
			cfg.ShardCacheMaxAssignments,
			cfg.ShardCacheAssignmentTTL,
			cfg.ShardCacheShardsTTL,
		),
	}
}

// ExecuteDatabase executes a database operation on the primary with circuit breaker protection.
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

// ExecuteReplica executes a read operation on the replica with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteReplica(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.DatabaseReplica.ExecuteWithContext(ctx, fn)
}

// ExecuteReplicaWithResult executes a replica read operation that returns a result.
func (cb *CircuitBreakers) ExecuteReplicaWithResult(ctx context.Context, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.DatabaseReplica.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})
	return result, err
}

// ExecuteWithReplica tries replica first, falls back to primary if replica circuit is open.
// This is the recommended method for read operations that can tolerate slight staleness.
func (cb *CircuitBreakers) ExecuteWithReplica(
	ctx context.Context,
	replicaFn func(ctx context.Context) error,
	primaryFn func(ctx context.Context) error,
) error {
	// Try replica first
	err := cb.DatabaseReplica.ExecuteWithContext(ctx, replicaFn)

	// If replica circuit is open, fallback to primary
	if errors.Is(err, ErrCircuitOpen) {
		cb.logger.Info("Replica circuit open, falling back to primary")
		return cb.Database.ExecuteWithContext(ctx, primaryFn)
	}

	return err
}

// ExecuteWithReplicaResult tries replica first, falls back to primary if replica circuit is open.
// Returns the result from whichever source succeeds.
func (cb *CircuitBreakers) ExecuteWithReplicaResult(
	ctx context.Context,
	replicaFn func(ctx context.Context) (interface{}, error),
	primaryFn func(ctx context.Context) (interface{}, error),
) (interface{}, error) {
	var result interface{}

	// Try replica first
	err := cb.DatabaseReplica.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = replicaFn(ctx)
		return fnErr
	})

	// If replica circuit is open, fallback to primary
	if errors.Is(err, ErrCircuitOpen) {
		cb.logger.Info("Replica circuit open, falling back to primary for read")
		err = cb.Database.ExecuteWithContext(ctx, func(ctx context.Context) error {
			var fnErr error
			result, fnErr = primaryFn(ctx)
			return fnErr
		})
	}

	return result, err
}

// ExecuteRedis executes a Redis operation with circuit breaker protection.
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

// ExecuteWithCacheFallback executes a cache operation with fallback to in-memory cache.
// This provides three-tier caching: Redis -> In-memory -> Source
func (cb *CircuitBreakers) ExecuteWithCacheFallback(
	ctx context.Context,
	redisFn func(ctx context.Context) (interface{}, error),
	memoryCacheFn func() interface{},
	sourceFn func(ctx context.Context) (interface{}, error),
	cacheResultFn func(result interface{}),
) (interface{}, error) {
	// Try Redis first
	result, err := cb.ExecuteRedisWithResult(ctx, redisFn)
	if err == nil && result != nil {
		return result, nil
	}

	// If Redis failed, try in-memory cache
	if errors.Is(err, ErrCircuitOpen) {
		cb.logger.Debug("Redis circuit open, trying in-memory cache")
	}

	memResult := memoryCacheFn()
	if memResult != nil {
		return memResult, nil
	}

	// Fall back to source (database)
	result, err = sourceFn(ctx)
	if err != nil {
		return nil, err
	}

	// Cache the result in both Redis and memory
	if cacheResultFn != nil && result != nil {
		cacheResultFn(result)
	}

	return result, nil
}

// CanReadFromReplica returns true if the replica circuit is not open.
// Use this to decide whether to attempt a replica read.
func (cb *CircuitBreakers) CanReadFromReplica() bool {
	return cb.DatabaseReplica.State() != circuitbreaker.StateOpen
}

// CanUseCache returns true if the Redis circuit is not open.
// Use this to decide whether to attempt cache operations.
func (cb *CircuitBreakers) CanUseCache() bool {
	return cb.Redis.State() != circuitbreaker.StateOpen
}

// GetCachedAssignment returns a cached assignment from in-memory cache.
func (cb *CircuitBreakers) GetCachedAssignment(contestID string) *ShardAssignment {
	return cb.shardInfoCache.GetAssignment(contestID)
}

// SetCachedAssignment stores an assignment in the in-memory cache.
func (cb *CircuitBreakers) SetCachedAssignment(contestID string, assignment *ShardAssignment) {
	cb.shardInfoCache.SetAssignment(contestID, assignment)
}

// GetCachedShards returns the cached shard list from in-memory cache.
func (cb *CircuitBreakers) GetCachedShards() []*Shard {
	return cb.shardInfoCache.GetShards()
}

// SetCachedShards stores the shard list in the in-memory cache.
func (cb *CircuitBreakers) SetCachedShards(shards []*Shard) {
	cb.shardInfoCache.SetShards(shards)
}

// GetCachedShard returns a specific shard from the in-memory cache.
func (cb *CircuitBreakers) GetCachedShard(shardID string) *Shard {
	return cb.shardInfoCache.GetShard(shardID)
}

// InvalidateCachedAssignment removes an assignment from the in-memory cache.
func (cb *CircuitBreakers) InvalidateCachedAssignment(contestID string) {
	cb.shardInfoCache.InvalidateAssignment(contestID)
}

// InvalidateAllCached clears all in-memory cached data.
func (cb *CircuitBreakers) InvalidateAllCached() {
	cb.shardInfoCache.InvalidateAll()
}

// CircuitHealth represents the health status of circuit breakers.
type CircuitHealth struct {
	Database        CircuitStatus `json:"database"`
	DatabaseReplica CircuitStatus `json:"database_replica"`
	Redis           CircuitStatus `json:"redis"`
	Overall         string        `json:"overall"`
	Degraded        bool          `json:"degraded"`
	CanServe        bool          `json:"can_serve"`
}

// CircuitStatus represents the status of a single circuit breaker.
type CircuitStatus struct {
	State        string `json:"state"`
	FailureCount int    `json:"failure_count"`
	Critical     bool   `json:"critical"`
	Healthy      bool   `json:"healthy"`
}

// IsHealthy returns true if the service can serve requests.
// For shard-router, we're healthy if we can serve from database OR cache.
// This allows serving requests from Redis cache even when database is down.
func (cb *CircuitBreakers) IsHealthy() bool {
	dbOpen := cb.Database.State() == circuitbreaker.StateOpen
	redisOpen := cb.Redis.State() == circuitbreaker.StateOpen

	// We can serve if either database OR Redis is available
	// Database is preferred but Redis cache can serve read requests
	return !dbOpen || !redisOpen
}

// CanServeFromCache returns true if we can serve requests from cache.
// This is used when database is unavailable.
func (cb *CircuitBreakers) CanServeFromCache() bool {
	return cb.Redis.State() != circuitbreaker.StateOpen
}

// CanServeFromMemory returns true if we have valid in-memory cached data.
// This is the last resort when both database and Redis are unavailable.
func (cb *CircuitBreakers) CanServeFromMemory() bool {
	return cb.shardInfoCache.GetShards() != nil
}

// IsDegraded returns true if any circuit breaker is not in closed state.
func (cb *CircuitBreakers) IsDegraded() bool {
	return cb.Database.State() != circuitbreaker.StateClosed ||
		cb.DatabaseReplica.State() != circuitbreaker.StateClosed ||
		cb.Redis.State() != circuitbreaker.StateClosed
}

// GetHealth returns detailed health status of all circuit breakers.
func (cb *CircuitBreakers) GetHealth() CircuitHealth {
	dbState := cb.Database.State()
	dbReplicaState := cb.DatabaseReplica.State()
	redisState := cb.Redis.State()

	health := CircuitHealth{
		Database: CircuitStatus{
			State:        dbState.String(),
			FailureCount: cb.Database.FailureCount(),
			Critical:     false, // Not critical alone - can serve from cache
			Healthy:      dbState != circuitbreaker.StateOpen,
		},
		DatabaseReplica: CircuitStatus{
			State:        dbReplicaState.String(),
			FailureCount: cb.DatabaseReplica.FailureCount(),
			Critical:     false, // Can fallback to primary
			Healthy:      dbReplicaState != circuitbreaker.StateOpen,
		},
		Redis: CircuitStatus{
			State:        redisState.String(),
			FailureCount: cb.Redis.FailureCount(),
			Critical:     false, // Not critical alone - can serve from database
			Healthy:      redisState != circuitbreaker.StateOpen,
		},
		CanServe: cb.IsHealthy(),
	}

	// Determine overall status
	// For shard-router: healthy if DB OR Redis available
	if cb.IsHealthy() {
		if cb.IsDegraded() {
			health.Overall = "degraded"
			health.Degraded = true
		} else {
			health.Overall = "healthy"
			health.Degraded = false
		}
	} else {
		// Both DB and Redis are down - check in-memory cache
		if cb.CanServeFromMemory() {
			health.Overall = "critical-degraded"
			health.Degraded = true
			health.CanServe = true
		} else {
			health.Overall = "unhealthy"
			health.Degraded = true
		}
	}

	return health
}

// GetMetrics returns metrics for all circuit breakers.
func (cb *CircuitBreakers) GetMetrics() map[string]circuitbreaker.Metrics {
	return map[string]circuitbreaker.Metrics{
		DepDatabase:        cb.Database.Metrics(),
		DepDatabaseReplica: cb.DatabaseReplica.Metrics(),
		DepRedis:           cb.Redis.Metrics(),
	}
}

// GetCacheStats returns statistics for the in-memory cache.
func (cb *CircuitBreakers) GetCacheStats() map[string]interface{} {
	return cb.shardInfoCache.Stats()
}

// Reset resets all circuit breakers to closed state.
// Use with caution - only for administrative purposes.
func (cb *CircuitBreakers) Reset() {
	cb.Database.Reset()
	cb.DatabaseReplica.Reset()
	cb.Redis.Reset()
	cb.logger.Info("All circuit breakers reset")
}

// ResetCircuit resets a specific circuit breaker by name.
func (cb *CircuitBreakers) ResetCircuit(name string) error {
	switch name {
	case DepDatabase:
		cb.Database.Reset()
	case DepDatabaseReplica:
		cb.DatabaseReplica.Reset()
	case DepRedis:
		cb.Redis.Reset()
	default:
		return errors.New("unknown circuit breaker: " + name)
	}
	cb.logger.Info("Circuit breaker reset", zap.String("circuit", name))
	return nil
}

// AllCircuits returns a slice of all circuit breakers for iteration.
func (cb *CircuitBreakers) AllCircuits() []*circuitbreaker.CircuitBreaker {
	return []*circuitbreaker.CircuitBreaker{
		cb.Database,
		cb.DatabaseReplica,
		cb.Redis,
	}
}

// Status returns a map of circuit breaker names to their current states.
func (cb *CircuitBreakers) Status() map[string]string {
	status := make(map[string]string)
	for _, c := range cb.AllCircuits() {
		status[c.Name()] = c.State().String()
	}
	return status
}
