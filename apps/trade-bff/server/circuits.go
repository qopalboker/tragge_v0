package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"go.uber.org/zap"
)

// Circuits holds all circuit breakers for the trade-bff service.
type Circuits struct {
	// Database circuit breaker for primary database calls
	Database *circuitbreaker.CircuitBreaker

	// DatabaseReplica circuit breaker for replica database reads
	DatabaseReplica *circuitbreaker.CircuitBreaker

	// Redis circuit breaker for Redis cache operations
	Redis *circuitbreaker.CircuitBreaker

	// Kafka circuit breaker for Kafka producer operations
	Kafka *circuitbreaker.CircuitBreaker

	// ShardRouter circuit breaker for shard-router service calls
	ShardRouter *circuitbreaker.CircuitBreaker

	// Logger for circuit breaker events
	logger *zap.Logger
}

// NewCircuits creates a new Circuits instance with all circuit breakers configured.
func NewCircuits(logger *zap.Logger, onStateChange func(name string, from, to circuitbreaker.State)) *Circuits {
	// Create default state change handler if not provided
	if onStateChange == nil {
		onStateChange = func(name string, from, to circuitbreaker.State) {
			if logger != nil {
				logger.Warn("Circuit breaker state changed",
					zap.String("name", name),
					zap.String("from", from.String()),
					zap.String("to", to.String()))
			}
		}
	}

	// Database circuit - uses database preset
	dbConfig := circuitbreaker.DatabaseCircuitConfig("postgres")
	dbConfig.OnStateChange = onStateChange

	// Database replica circuit - slightly more lenient
	dbReplicaConfig := circuitbreaker.DatabaseReplicaCircuitConfig("postgres-replica")
	dbReplicaConfig.OnStateChange = onStateChange

	// Redis circuit - uses Redis preset
	redisConfig := circuitbreaker.RedisCircuitConfig("redis")
	redisConfig.OnStateChange = onStateChange

	// Kafka circuit - uses Kafka preset
	kafkaConfig := circuitbreaker.KafkaCircuitConfig("kafka")
	kafkaConfig.OnStateChange = onStateChange

	// Shard router circuit - uses shard router preset
	shardConfig := circuitbreaker.ShardRouterCircuitConfig("shard-router")
	shardConfig.OnStateChange = onStateChange

	return &Circuits{
		Database:        circuitbreaker.New(dbConfig),
		DatabaseReplica: circuitbreaker.New(dbReplicaConfig),
		Redis:           circuitbreaker.New(redisConfig),
		Kafka:           circuitbreaker.New(kafkaConfig),
		ShardRouter:     circuitbreaker.New(shardConfig),
		logger:          logger,
	}
}

// AllCircuits returns a slice of all circuit breakers for iteration.
func (c *Circuits) AllCircuits() []*circuitbreaker.CircuitBreaker {
	return []*circuitbreaker.CircuitBreaker{
		c.Database,
		c.DatabaseReplica,
		c.Redis,
		c.Kafka,
		c.ShardRouter,
	}
}

// Status returns a map of circuit breaker names to their current states.
func (c *Circuits) Status() map[string]string {
	status := make(map[string]string)
	for _, cb := range c.AllCircuits() {
		status[cb.Name()] = cb.State().String()
	}
	return status
}

// Metrics returns a map of circuit breaker names to their metrics.
func (c *Circuits) Metrics() map[string]circuitbreaker.Metrics {
	metrics := make(map[string]circuitbreaker.Metrics)
	for _, cb := range c.AllCircuits() {
		metrics[cb.Name()] = cb.Metrics()
	}
	return metrics
}

// IsHealthy returns true if all critical circuits are closed or half-open.
// Open circuits for critical dependencies (Database, Kafka) indicate unhealthy state.
func (c *Circuits) IsHealthy() bool {
	// Critical circuits that affect core functionality
	criticalCircuits := []*circuitbreaker.CircuitBreaker{
		c.Database,
		c.Kafka,
	}

	for _, cb := range criticalCircuits {
		if cb.State() == circuitbreaker.StateOpen {
			return false
		}
	}
	return true
}

// Reset resets all circuit breakers to their initial closed state.
// This should only be used for administrative purposes.
func (c *Circuits) Reset() {
	for _, cb := range c.AllCircuits() {
		cb.Reset()
	}
	if c.logger != nil {
		c.logger.Info("All circuit breakers reset")
	}
}

// CircuitHealthResponse represents the health status of all circuits.
type CircuitHealthResponse struct {
	Status   string                        `json:"status"`
	Healthy  bool                          `json:"healthy"`
	Circuits map[string]CircuitInfo        `json:"circuits"`
	Metrics  map[string]CircuitMetricsInfo `json:"metrics,omitempty"`
}

// CircuitInfo represents the status of a single circuit.
type CircuitInfo struct {
	State        string `json:"state"`
	FailureCount int    `json:"failure_count"`
}

// CircuitMetricsInfo represents metrics for a single circuit.
type CircuitMetricsInfo struct {
	TotalRequests        int64     `json:"total_requests"`
	TotalSuccesses       int64     `json:"total_successes"`
	TotalFailures        int64     `json:"total_failures"`
	TotalRejections      int64     `json:"total_rejections"`
	TotalTimeouts        int64     `json:"total_timeouts"`
	ConsecutiveSuccesses int64     `json:"consecutive_successes"`
	ConsecutiveFailures  int64     `json:"consecutive_failures"`
	LastFailureTime      time.Time `json:"last_failure_time,omitempty"`
	LastSuccessTime      time.Time `json:"last_success_time,omitempty"`
	StateChanges         int64     `json:"state_changes"`
}

// HandleCircuitHealth handles GET /health/circuits
// Returns the health status of all circuit breakers.
func (c *Circuits) HandleCircuitHealth(w http.ResponseWriter, r *http.Request) {
	includeMetrics := r.URL.Query().Get("metrics") == "true"

	response := CircuitHealthResponse{
		Status:   "ok",
		Healthy:  c.IsHealthy(),
		Circuits: make(map[string]CircuitInfo),
	}

	if !response.Healthy {
		response.Status = "degraded"
	}

	for _, cb := range c.AllCircuits() {
		response.Circuits[cb.Name()] = CircuitInfo{
			State:        cb.State().String(),
			FailureCount: cb.FailureCount(),
		}
	}

	if includeMetrics {
		response.Metrics = make(map[string]CircuitMetricsInfo)
		for _, cb := range c.AllCircuits() {
			m := cb.Metrics()
			response.Metrics[cb.Name()] = CircuitMetricsInfo{
				TotalRequests:        m.TotalRequests,
				TotalSuccesses:       m.TotalSuccesses,
				TotalFailures:        m.TotalFailures,
				TotalRejections:      m.TotalRejections,
				TotalTimeouts:        m.TotalTimeouts,
				ConsecutiveSuccesses: m.ConsecutiveSuccesses,
				ConsecutiveFailures:  m.ConsecutiveFailures,
				LastFailureTime:      m.LastFailureTime,
				LastSuccessTime:      m.LastSuccessTime,
				StateChanges:         m.StateChanges,
			}
		}
	}

	// Return 503 if unhealthy
	if !response.Healthy {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(response)
}

// HandleCircuitReset handles POST /admin/circuits/reset
// Resets all circuit breakers (admin only).
func (c *Circuits) HandleCircuitReset(w http.ResponseWriter, r *http.Request) {
	c.Reset()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": tradeMsg.CircuitsReset,
	})
}

// CachedBalance represents a cached user balance for fallback.
type CachedBalance struct {
	Total     float64   `json:"total"`
	Available float64   `json:"available"`
	CachedAt  time.Time `json:"cached_at"`
}

// BalanceCache provides fallback balance data when database is unavailable.
type BalanceCache struct {
	mu    sync.RWMutex
	cache map[string]CachedBalance
}

// NewBalanceCache creates a new balance cache.
func NewBalanceCache() *BalanceCache {
	return &BalanceCache{
		cache: make(map[string]CachedBalance),
	}
}

// Set stores a balance in the cache.
func (bc *BalanceCache) Set(userID, contestID string, total, available float64) {
	key := userID + ":" + contestID
	bc.mu.Lock()
	bc.cache[key] = CachedBalance{
		Total:     total,
		Available: available,
		CachedAt:  time.Now(),
	}
	bc.mu.Unlock()
}

// Get retrieves a balance from the cache.
func (bc *BalanceCache) Get(userID, contestID string) (*CachedBalance, bool) {
	key := userID + ":" + contestID
	bc.mu.RLock()
	balance, ok := bc.cache[key]
	bc.mu.RUnlock()
	if !ok {
		return nil, false
	}
	// Consider cache entries older than 5 minutes as stale
	if time.Since(balance.CachedAt) > 5*time.Minute {
		return nil, false
	}
	return &balance, true
}

// ExecuteWithFallback executes a function with circuit breaker protection
// and returns cached data if the circuit is open.
func ExecuteWithFallback[T any](
	ctx context.Context,
	cb *circuitbreaker.CircuitBreaker,
	fn func(context.Context) (T, error),
	fallback func() (T, error),
) (T, error) {
	var result T

	err := cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})

	if err != nil {
		if circuitbreaker.ErrCircuitOpen == err && fallback != nil {
			return fallback()
		}
		return result, err
	}

	return result, nil
}
