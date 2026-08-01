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
	DepKafkaAdmin      = "kafka-admin"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = circuitbreaker.ErrCircuitOpen
	// ErrCircuitTimeout is returned when the wrapped function exceeds the configured timeout
	ErrCircuitTimeout = circuitbreaker.ErrCircuitTimeout
)

// CircuitBreakers holds all circuit breakers for the admin-bff service.
type CircuitBreakers struct {
	// Database circuit breaker for primary database calls (critical)
	Database *circuitbreaker.CircuitBreaker

	// DatabaseReplica circuit breaker for replica database reads (can fallback to primary)
	DatabaseReplica *circuitbreaker.CircuitBreaker

	// Redis circuit breaker for Redis heartbeat operations (not critical)
	Redis *circuitbreaker.CircuitBreaker

	// KafkaAdmin circuit breaker for Kafka admin operations (not critical, monitoring only)
	KafkaAdmin *circuitbreaker.CircuitBreaker

	logger *zap.Logger

	// Fallback caches for graceful degradation
	contestsCache *CachedContests
}

// CachedContests provides fallback contest data when database is unavailable.
type CachedContests struct {
	mu        sync.RWMutex
	data      []ContestResponse
	cachedAt  time.Time
	expiresAt time.Time
}

// CircuitBreakerConfig holds configuration for circuit breaker initialization.
type CircuitBreakerConfig struct {
	// Logger for circuit breaker events
	Logger *zap.Logger

	// ContestsCacheTTL is how long contests cache remains valid
	ContestsCacheTTL time.Duration

	// OnStateChange is called when any circuit breaker changes state
	OnStateChange func(name string, from, to circuitbreaker.State)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		ContestsCacheTTL: 30 * time.Second,
	}
}

// KafkaAdminCircuitConfig returns a Config optimized for Kafka admin operations.
// This is more lenient since admin operations are only used for monitoring
// and are not critical for service functionality.
//
// Configuration rationale:
//   - MaxFailures: 15 - very lenient, allow many failures before tripping
//   - FailureWindow: 60s - longer window for monitoring operations
//   - ResetTimeout: 30s - quick recovery attempts
//   - HalfOpenMaxCalls: 3 - test recovery with moderate requests
//   - SuccessThreshold: 2 - require 2 successes for recovery
//   - Timeout: 15s - Kafka admin operations can be slow
func KafkaAdminCircuitConfig(name string) circuitbreaker.Config {
	return circuitbreaker.Config{
		Name:             name,
		MaxFailures:      15,
		FailureWindow:    60 * time.Second,
		ResetTimeout:     30 * time.Second,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 2,
		Timeout:          15 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
				return false
			}
			return true
		},
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

	// Database primary circuit - critical for writes and contest management
	dbConfig := circuitbreaker.DatabaseCircuitConfig(DepDatabase)
	dbConfig.OnStateChange = stateChangeHandler

	// Database replica circuit - can fallback to primary
	dbReplicaConfig := circuitbreaker.DatabaseReplicaCircuitConfig(DepDatabaseReplica)
	dbReplicaConfig.OnStateChange = stateChangeHandler

	// Redis circuit - not critical, used for heartbeats only
	redisConfig := circuitbreaker.RedisCircuitConfig(DepRedis)
	redisConfig.OnStateChange = stateChangeHandler

	// Kafka admin circuit - not critical, only used for shard monitoring
	// Use more lenient config since it's just for monitoring
	kafkaAdminConfig := KafkaAdminCircuitConfig(DepKafkaAdmin)
	kafkaAdminConfig.OnStateChange = stateChangeHandler

	return &CircuitBreakers{
		Database:        circuitbreaker.New(dbConfig),
		DatabaseReplica: circuitbreaker.New(dbReplicaConfig),
		Redis:           circuitbreaker.New(redisConfig),
		KafkaAdmin:      circuitbreaker.New(kafkaAdminConfig),
		logger:          logger,
		contestsCache:   &CachedContests{},
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

// ExecuteKafkaAdmin executes a Kafka admin operation with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteKafkaAdmin(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.KafkaAdmin.ExecuteWithContext(ctx, fn)
}

// ExecuteKafkaAdminWithResult executes a Kafka admin operation that returns a result.
func (cb *CircuitBreakers) ExecuteKafkaAdminWithResult(ctx context.Context, fn func(ctx context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.KafkaAdmin.ExecuteWithContext(ctx, func(ctx context.Context) error {
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

// CanUseKafkaAdmin returns true if the Kafka admin circuit is not open.
// Use this to decide whether to attempt Kafka admin operations.
func (cb *CircuitBreakers) CanUseKafkaAdmin() bool {
	return cb.KafkaAdmin.State() != circuitbreaker.StateOpen
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

// CircuitHealth represents the health status of circuit breakers.
type CircuitHealth struct {
	Database        CircuitStatus `json:"database"`
	DatabaseReplica CircuitStatus `json:"database_replica"`
	Redis           CircuitStatus `json:"redis"`
	KafkaAdmin      CircuitStatus `json:"kafka_admin"`
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
// Only the primary database is critical for admin-bff health checks.
func (cb *CircuitBreakers) IsHealthy() bool {
	// Only primary database is critical
	return cb.Database.State() != circuitbreaker.StateOpen
}

// IsDegraded returns true if any circuit breaker is not in closed state.
func (cb *CircuitBreakers) IsDegraded() bool {
	return cb.Database.State() != circuitbreaker.StateClosed ||
		cb.DatabaseReplica.State() != circuitbreaker.StateClosed ||
		cb.Redis.State() != circuitbreaker.StateClosed ||
		cb.KafkaAdmin.State() != circuitbreaker.StateClosed
}

// GetHealth returns detailed health status of all circuit breakers.
func (cb *CircuitBreakers) GetHealth() CircuitHealth {
	dbState := cb.Database.State()
	dbReplicaState := cb.DatabaseReplica.State()
	redisState := cb.Redis.State()
	kafkaAdminState := cb.KafkaAdmin.State()

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
			Critical:     false, // Redis is not critical - heartbeats can degrade
			Healthy:      redisState != circuitbreaker.StateOpen,
		},
		KafkaAdmin: CircuitStatus{
			State:        kafkaAdminState.String(),
			FailureCount: cb.KafkaAdmin.FailureCount(),
			Critical:     false, // Kafka admin is not critical - monitoring only
			Healthy:      kafkaAdminState != circuitbreaker.StateOpen,
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
		DepKafkaAdmin:      cb.KafkaAdmin.Metrics(),
	}
}

// Reset resets all circuit breakers to closed state.
// Use with caution - only for administrative purposes.
func (cb *CircuitBreakers) Reset() {
	cb.Database.Reset()
	cb.DatabaseReplica.Reset()
	cb.Redis.Reset()
	cb.KafkaAdmin.Reset()
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
	case DepKafkaAdmin:
		cb.KafkaAdmin.Reset()
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
		cb.KafkaAdmin,
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
