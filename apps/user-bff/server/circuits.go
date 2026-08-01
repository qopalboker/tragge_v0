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
	DepDatabase        = "database"
	DepDatabaseReplica = "database-replica"
	DepRedis           = "redis"
	DepSMS             = "kavenegar-sms"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = circuitbreaker.ErrCircuitOpen
)

// CircuitBreakers holds all circuit breakers for the user-bff service.
type CircuitBreakers struct {
	// Database circuit breaker for primary database calls (critical)
	Database *circuitbreaker.CircuitBreaker

	// DatabaseReplica circuit breaker for replica database reads (can fallback to primary)
	DatabaseReplica *circuitbreaker.CircuitBreaker

	// Redis circuit breaker for Redis cache/session operations (not critical)
	Redis *circuitbreaker.CircuitBreaker

	// SMS circuit breaker for KaveNegar API calls (not critical)
	SMS *circuitbreaker.CircuitBreaker

	logger *zap.Logger

	// Fallback caches for graceful degradation
	contestsCache *CachedContests
	userCache     *CachedUsers
}

// CachedContests provides fallback contest data when database is unavailable.
type CachedContests struct {
	mu        sync.RWMutex
	data      []ContestResponse
	cachedAt  time.Time
	expiresAt time.Time
}

// CachedUser represents cached user profile data.
type CachedUser struct {
	UserID   string    `json:"user_id"`
	Email    string    `json:"email"`
	CachedAt time.Time `json:"cached_at"`
}

// CachedUsers provides fallback user data when database is unavailable.
type CachedUsers struct {
	mu      sync.RWMutex
	users   map[string]*CachedUser // userID -> cached user
	maxSize int
	ttl     time.Duration
	done    chan struct{}
}

// NewCachedUsers creates a new user cache with the given configuration.
func NewCachedUsers(maxSize int, ttl time.Duration) *CachedUsers {
	c := &CachedUsers{
		users:   make(map[string]*CachedUser),
		maxSize: maxSize,
		ttl:     ttl,
		done:    make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves a cached user profile.
// Returns nil if not found or expired.
func (c *CachedUsers) Get(userID string) *CachedUser {
	c.mu.RLock()
	user, ok := c.users[userID]
	if !ok {
		c.mu.RUnlock()
		return nil
	}

	// Check if expired
	if time.Since(user.CachedAt) > c.ttl {
		c.mu.RUnlock()
		// Lazy eviction: promote to write lock and delete stale entry
		c.mu.Lock()
		if u, exists := c.users[userID]; exists && time.Since(u.CachedAt) > c.ttl {
			delete(c.users, userID)
		}
		c.mu.Unlock()
		return nil
	}

	c.mu.RUnlock()
	return user
}

// Set stores a user profile in the cache.
func (c *CachedUsers) Set(userID, email string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest if at capacity
	if len(c.users) >= c.maxSize && c.users[userID] == nil {
		c.evictOldest()
	}

	c.users[userID] = &CachedUser{
		UserID:   userID,
		Email:    email,
		CachedAt: time.Now(),
	}
}

// evictOldest removes the oldest entry. Must be called with lock held.
func (c *CachedUsers) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, user := range c.users {
		if oldestKey == "" || user.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = user.CachedAt
		}
	}

	if oldestKey != "" {
		delete(c.users, oldestKey)
	}
}

// Clear removes all entries from the cache.
func (c *CachedUsers) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.users = make(map[string]*CachedUser)
}

// cleanupLoop periodically removes expired entries.
func (c *CachedUsers) cleanupLoop() {
	interval := c.ttl / 2
	if interval < time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.done:
			return
		}
	}
}

// cleanupExpired removes all entries that have exceeded the TTL.
func (c *CachedUsers) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, user := range c.users {
		if now.Sub(user.CachedAt) > c.ttl {
			delete(c.users, key)
		}
	}
}

// Stop signals the cleanup goroutine to exit.
func (c *CachedUsers) Stop() { close(c.done) }

// CircuitBreakerConfig holds configuration for circuit breaker initialization.
type CircuitBreakerConfig struct {
	// Logger for circuit breaker events
	Logger *zap.Logger

	// UserCacheSize is the max number of users to cache for fallback
	UserCacheSize int

	// UserCacheTTL is how long user cache entries remain valid
	UserCacheTTL time.Duration

	// ContestsCacheTTL is how long contests cache remains valid
	ContestsCacheTTL time.Duration

	// OnStateChange is called when any circuit breaker changes state
	OnStateChange func(name string, from, to circuitbreaker.State)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		UserCacheSize:    10000,
		UserCacheTTL:     10 * time.Minute,
		ContestsCacheTTL: 30 * time.Second,
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

	// Database primary circuit - critical for writes and auth
	dbConfig := circuitbreaker.DatabaseCircuitConfig(DepDatabase)
	dbConfig.OnStateChange = stateChangeHandler

	// Database replica circuit - can fallback to primary
	dbReplicaConfig := circuitbreaker.DatabaseReplicaCircuitConfig(DepDatabaseReplica)
	dbReplicaConfig.OnStateChange = stateChangeHandler

	// Redis circuit - not critical, used for sessions and caching
	redisConfig := circuitbreaker.RedisCircuitConfig(DepRedis)
	redisConfig.OnStateChange = stateChangeHandler

	// SMS (KaveNegar) circuit - not critical, external API
	smsConfig := circuitbreaker.ExternalAPICircuitConfig(DepSMS)
	smsConfig.MaxFailures = 5
	smsConfig.ResetTimeout = 60 * time.Second
	smsConfig.HalfOpenMaxCalls = 2
	smsConfig.OnStateChange = stateChangeHandler

	return &CircuitBreakers{
		Database:        circuitbreaker.New(dbConfig),
		DatabaseReplica: circuitbreaker.New(dbReplicaConfig),
		Redis:           circuitbreaker.New(redisConfig),
		SMS:             circuitbreaker.New(smsConfig),
		logger:          logger,
		contestsCache:   &CachedContests{},
		userCache:       NewCachedUsers(cfg.UserCacheSize, cfg.UserCacheTTL),
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

// GetCachedContests returns cached contests if available and not expired.
func (cb *CircuitBreakers) GetCachedContests() ([]ContestResponse, bool) {
	cb.contestsCache.mu.RLock()
	defer cb.contestsCache.mu.RUnlock()

	if cb.contestsCache.data == nil || time.Now().After(cb.contestsCache.expiresAt) {
		return nil, false
	}

	return cb.contestsCache.data, true
}

// SetCachedContests stores contests in the fallback cache.
func (cb *CircuitBreakers) SetCachedContests(contests []ContestResponse, ttl time.Duration) {
	cb.contestsCache.mu.Lock()
	defer cb.contestsCache.mu.Unlock()

	now := time.Now()
	cb.contestsCache.data = contests
	cb.contestsCache.cachedAt = now
	cb.contestsCache.expiresAt = now.Add(ttl)
}

// GetCachedUser returns a cached user profile if available.
func (cb *CircuitBreakers) GetCachedUser(userID string) *CachedUser {
	return cb.userCache.Get(userID)
}

// SetCachedUser stores a user profile in the fallback cache.
func (cb *CircuitBreakers) SetCachedUser(userID, email string) {
	cb.userCache.Set(userID, email)
}

// CircuitHealth represents the health status of circuit breakers.
type CircuitHealth struct {
	Database        CircuitStatus `json:"database"`
	DatabaseReplica CircuitStatus `json:"database_replica"`
	Redis           CircuitStatus `json:"redis"`
	SMS             CircuitStatus `json:"sms"`
	Overall         string        `json:"overall"`
	Degraded        bool          `json:"degraded"`
}

// CircuitStatus represents the status of a single circuit breaker.
type CircuitStatus struct {
	State        string `json:"state"`
	FailureCount int    `json:"failure_count"`
	Critical     bool   `json:"critical"`
	Healthy      bool   `json:"healthy"`
}

// IsHealthy returns true if all critical circuit breakers are closed.
// Only the primary database is critical for user-bff health checks.
func (cb *CircuitBreakers) IsHealthy() bool {
	// Only primary database is critical
	return cb.Database.State() != circuitbreaker.StateOpen
}

// IsDegraded returns true if any circuit breaker is not in closed state.
func (cb *CircuitBreakers) IsDegraded() bool {
	return cb.Database.State() != circuitbreaker.StateClosed ||
		cb.DatabaseReplica.State() != circuitbreaker.StateClosed ||
		cb.Redis.State() != circuitbreaker.StateClosed ||
		cb.SMS.State() != circuitbreaker.StateClosed
}

// GetHealth returns detailed health status of all circuit breakers.
func (cb *CircuitBreakers) GetHealth() CircuitHealth {
	dbState := cb.Database.State()
	dbReplicaState := cb.DatabaseReplica.State()
	redisState := cb.Redis.State()
	smsState := cb.SMS.State()

	health := CircuitHealth{
		Database: CircuitStatus{
			State:        dbState.String(),
			FailureCount: cb.Database.FailureCount(),
			Critical:     true, // Primary database is critical
			Healthy:      dbState != circuitbreaker.StateOpen,
		},
		DatabaseReplica: CircuitStatus{
			State:        dbReplicaState.String(),
			FailureCount: cb.DatabaseReplica.FailureCount(),
			Critical:     false, // Replica is not critical - can fallback to primary
			Healthy:      dbReplicaState != circuitbreaker.StateOpen,
		},
		Redis: CircuitStatus{
			State:        redisState.String(),
			FailureCount: cb.Redis.FailureCount(),
			Critical:     false, // Redis is not critical - sessions/cache can degrade
			Healthy:      redisState != circuitbreaker.StateOpen,
		},
		SMS: CircuitStatus{
			State:        smsState.String(),
			FailureCount: cb.SMS.FailureCount(),
			Critical:     false, // SMS is not critical - email auth still works
			Healthy:      smsState != circuitbreaker.StateOpen,
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
		DepDatabase:        cb.Database.Metrics(),
		DepDatabaseReplica: cb.DatabaseReplica.Metrics(),
		DepRedis:           cb.Redis.Metrics(),
		DepSMS:             cb.SMS.Metrics(),
	}
}

// Reset resets all circuit breakers to closed state.
// Use with caution - only for administrative purposes.
func (cb *CircuitBreakers) Reset() {
	cb.Database.Reset()
	cb.DatabaseReplica.Reset()
	cb.Redis.Reset()
	cb.SMS.Reset()
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
	case DepSMS:
		cb.SMS.Reset()
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
		cb.SMS,
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
