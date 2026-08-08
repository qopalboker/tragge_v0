package server

import (
	"context"
	"errors"
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
	DepNowPayments     = "nowpayments"
	DepJibit           = "jibit"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = circuitbreaker.ErrCircuitOpen
)

// CircuitBreakers holds all circuit breakers for the payment-service.
type CircuitBreakers struct {
	// Database circuit breaker for primary database calls (critical)
	Database *circuitbreaker.CircuitBreaker

	// DatabaseReplica circuit breaker for replica database reads
	DatabaseReplica *circuitbreaker.CircuitBreaker

	// Redis circuit breaker for Redis cache operations
	Redis *circuitbreaker.CircuitBreaker

	// Payment provider circuit breakers
	NowPayments *circuitbreaker.CircuitBreaker
	Jibit       *circuitbreaker.CircuitBreaker

	logger *zap.Logger
}

// CircuitBreakerConfig holds configuration for circuit breaker initialization.
type CircuitBreakerConfig struct {
	Logger        *zap.Logger
	OnStateChange func(name string, from, to circuitbreaker.State)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{}
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

	// Database primary circuit - critical for writes
	dbConfig := circuitbreaker.DatabaseCircuitConfig(DepDatabase)
	dbConfig.OnStateChange = stateChangeHandler

	// Database replica circuit - can fallback to primary
	dbReplicaConfig := circuitbreaker.DatabaseReplicaCircuitConfig(DepDatabaseReplica)
	dbReplicaConfig.OnStateChange = stateChangeHandler

	// Redis circuit - not critical
	redisConfig := circuitbreaker.RedisCircuitConfig(DepRedis)
	redisConfig.OnStateChange = stateChangeHandler

	// Payment provider circuits - external APIs
	nowPaymentsConfig := externalAPICircuitConfig(DepNowPayments)
	nowPaymentsConfig.OnStateChange = stateChangeHandler

	jibitConfig := externalAPICircuitConfig(DepJibit)
	jibitConfig.OnStateChange = stateChangeHandler

	return &CircuitBreakers{
		Database:        circuitbreaker.New(dbConfig),
		DatabaseReplica: circuitbreaker.New(dbReplicaConfig),
		Redis:           circuitbreaker.New(redisConfig),
		NowPayments:     circuitbreaker.New(nowPaymentsConfig),
		Jibit:           circuitbreaker.New(jibitConfig),
		logger:          logger,
	}
}

// externalAPICircuitConfig returns a circuit breaker config for external APIs.
func externalAPICircuitConfig(name string) circuitbreaker.Config {
	return circuitbreaker.Config{
		Name:          name,
		MaxFailures:   5,                // Open after 5 failures
		FailureWindow: 30 * time.Second, // Failures within 30 seconds
		ResetTimeout:  30 * time.Second, // Stay open for 30 seconds
	}
}

// ExecuteDatabase executes a database operation with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteDatabase(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.Database.ExecuteWithContext(ctx, fn)
}

// ExecuteReplica executes a read operation on the replica with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteReplica(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.DatabaseReplica.ExecuteWithContext(ctx, fn)
}

// ExecuteWithReplica tries replica first, falls back to primary if replica circuit is open.
func (cb *CircuitBreakers) ExecuteWithReplica(
	ctx context.Context,
	replicaFn func(ctx context.Context) error,
	primaryFn func(ctx context.Context) error,
) error {
	err := cb.DatabaseReplica.ExecuteWithContext(ctx, replicaFn)
	if errors.Is(err, ErrCircuitOpen) {
		cb.logger.Info("Replica circuit open, falling back to primary")
		return cb.Database.ExecuteWithContext(ctx, primaryFn)
	}
	return err
}

// ExecuteRedis executes a Redis operation with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteRedis(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.Redis.ExecuteWithContext(ctx, fn)
}

// ExecuteNowPayments executes a NOWPayments API call with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteNowPayments(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.NowPayments.ExecuteWithContext(ctx, fn)
}

// ExecuteJibit executes a Jibit API call with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteJibit(ctx context.Context, fn func(ctx context.Context) error) error {
	return cb.Jibit.ExecuteWithContext(ctx, fn)
}

// IsHealthy returns true if all critical circuit breakers are closed.
func (cb *CircuitBreakers) IsHealthy() bool {
	return cb.Database.State() != circuitbreaker.StateOpen
}

// IsDegraded returns true if any circuit breaker is not in closed state.
func (cb *CircuitBreakers) IsDegraded() bool {
	return cb.Database.State() != circuitbreaker.StateClosed ||
		cb.DatabaseReplica.State() != circuitbreaker.StateClosed ||
		cb.Redis.State() != circuitbreaker.StateClosed
}

// CircuitHealth represents the health status of circuit breakers.
type CircuitHealth struct {
	Database        CircuitStatus `json:"database"`
	DatabaseReplica CircuitStatus `json:"database_replica"`
	Redis           CircuitStatus `json:"redis"`
	NowPayments     CircuitStatus `json:"nowpayments"`
	Jibit           CircuitStatus `json:"jibit"`
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

// GetHealth returns detailed health status of all circuit breakers.
func (cb *CircuitBreakers) GetHealth() CircuitHealth {
	health := CircuitHealth{
		Database: CircuitStatus{
			State:        cb.Database.State().String(),
			FailureCount: cb.Database.FailureCount(),
			Critical:     true,
			Healthy:      cb.Database.State() != circuitbreaker.StateOpen,
		},
		DatabaseReplica: CircuitStatus{
			State:        cb.DatabaseReplica.State().String(),
			FailureCount: cb.DatabaseReplica.FailureCount(),
			Critical:     false,
			Healthy:      cb.DatabaseReplica.State() != circuitbreaker.StateOpen,
		},
		Redis: CircuitStatus{
			State:        cb.Redis.State().String(),
			FailureCount: cb.Redis.FailureCount(),
			Critical:     false,
			Healthy:      cb.Redis.State() != circuitbreaker.StateOpen,
		},
		NowPayments: CircuitStatus{
			State:        cb.NowPayments.State().String(),
			FailureCount: cb.NowPayments.FailureCount(),
			Critical:     false,
			Healthy:      cb.NowPayments.State() != circuitbreaker.StateOpen,
		},
		Jibit: CircuitStatus{
			State:        cb.Jibit.State().String(),
			FailureCount: cb.Jibit.FailureCount(),
			Critical:     false,
			Healthy:      cb.Jibit.State() != circuitbreaker.StateOpen,
		},
	}

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

// Reset resets all circuit breakers to closed state.
func (cb *CircuitBreakers) Reset() {
	cb.Database.Reset()
	cb.DatabaseReplica.Reset()
	cb.Redis.Reset()
	cb.NowPayments.Reset()
	cb.Jibit.Reset()
	cb.logger.Info("All circuit breakers reset")
}
