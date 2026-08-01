package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// CircuitBreakers holds all circuit breakers for the trading engine.
type CircuitBreakers struct {
	DB            *circuitbreaker.CircuitBreaker
	Redis         *circuitbreaker.CircuitBreaker
	Kafka         *circuitbreaker.CircuitBreaker
	KafkaConsumer *circuitbreaker.CircuitBreaker

	logger *zap.Logger
}

// NewCircuitBreakers creates and configures all circuit breakers for the trading engine.
func NewCircuitBreakers(logger *zap.Logger) *CircuitBreakers {
	cb := &CircuitBreakers{
		logger: logger,
	}

	// Create state change handler that logs transitions
	stateChangeHandler := func(name string, from, to circuitbreaker.State) {
		logger.Warn("Circuit breaker state changed",
			zap.String("circuit", name),
			zap.String("from", from.String()),
			zap.String("to", to.String()),
			zap.Time("timestamp", time.Now()))

		// Log specific messages based on state transition
		switch to {
		case circuitbreaker.StateOpen:
			logger.Error("Circuit breaker OPEN - failing fast",
				zap.String("circuit", name),
				zap.String("previous_state", from.String()))
		case circuitbreaker.StateHalfOpen:
			logger.Info("Circuit breaker entering recovery mode",
				zap.String("circuit", name))
		case circuitbreaker.StateClosed:
			logger.Info("Circuit breaker recovered and closed",
				zap.String("circuit", name))
		}
	}

	// Database circuit breaker
	dbConfig := circuitbreaker.DatabaseCircuitConfig("trading-engine-db")
	dbConfig.OnStateChange = stateChangeHandler
	cb.DB = circuitbreaker.New(dbConfig)

	// Redis circuit breaker
	redisConfig := circuitbreaker.RedisCircuitConfig("trading-engine-redis")
	redisConfig.OnStateChange = stateChangeHandler
	cb.Redis = circuitbreaker.New(redisConfig)

	// Kafka producer circuit breaker
	kafkaConfig := circuitbreaker.KafkaCircuitConfig("trading-engine-kafka")
	kafkaConfig.OnStateChange = stateChangeHandler
	cb.Kafka = circuitbreaker.New(kafkaConfig)

	// Kafka consumer circuit breaker
	kafkaConsumerConfig := circuitbreaker.KafkaConsumerCircuitConfig("trading-engine-kafka-consumer")
	kafkaConsumerConfig.OnStateChange = stateChangeHandler
	cb.KafkaConsumer = circuitbreaker.New(kafkaConsumerConfig)

	logger.Info("Circuit breakers initialized",
		zap.String("db_circuit", cb.DB.Name()),
		zap.String("redis_circuit", cb.Redis.Name()),
		zap.String("kafka_circuit", cb.Kafka.Name()),
		zap.String("kafka_consumer_circuit", cb.KafkaConsumer.Name()))

	return cb
}

// ExecuteWithDB wraps database calls with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteWithDB(ctx context.Context, fn func(context.Context) error) error {
	return cb.DB.ExecuteWithContext(ctx, fn)
}

// ExecuteWithDBResult wraps database calls that return a result with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteWithDBResult(ctx context.Context, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.DB.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})
	return result, err
}

// ExecuteWithRedis wraps Redis calls with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteWithRedis(ctx context.Context, fn func(context.Context) error) error {
	return cb.Redis.ExecuteWithContext(ctx, fn)
}

// ExecuteWithRedisResult wraps Redis calls that return a result with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteWithRedisResult(ctx context.Context, fn func(context.Context) (interface{}, error)) (interface{}, error) {
	var result interface{}
	err := cb.Redis.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})
	return result, err
}

// ExecuteWithKafka wraps Kafka producer calls with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteWithKafka(ctx context.Context, fn func(context.Context) error) error {
	return cb.Kafka.ExecuteWithContext(ctx, fn)
}

// ExecuteWithKafkaConsumer wraps Kafka consumer calls with circuit breaker protection.
func (cb *CircuitBreakers) ExecuteWithKafkaConsumer(ctx context.Context, fn func(context.Context) error) error {
	return cb.KafkaConsumer.ExecuteWithContext(ctx, fn)
}

// IsHealthy checks if all critical circuits are closed or half-open.
// Critical circuits are: Database and Kafka (producer).
// Redis is considered non-critical as the service can operate with degraded functionality.
func (cb *CircuitBreakers) IsHealthy() bool {
	// Database is critical
	if cb.DB.State() == circuitbreaker.StateOpen {
		return false
	}

	// Kafka producer is critical for publishing fills and positions
	if cb.Kafka.State() == circuitbreaker.StateOpen {
		return false
	}

	return true
}

// GetStatus returns the status of all circuit breakers.
func (cb *CircuitBreakers) GetStatus() map[string]CircuitStatus {
	return map[string]CircuitStatus{
		"database":       cb.getCircuitStatus(cb.DB, true),
		"redis":          cb.getCircuitStatus(cb.Redis, false),
		"kafka":          cb.getCircuitStatus(cb.Kafka, true),
		"kafka_consumer": cb.getCircuitStatus(cb.KafkaConsumer, false),
	}
}

// CircuitStatus represents the status of a single circuit breaker.
type CircuitStatus struct {
	Name         string                 `json:"name"`
	State        string                 `json:"state"`
	Critical     bool                   `json:"critical"`
	FailureCount int                    `json:"failure_count"`
	Metrics      circuitbreaker.Metrics `json:"metrics"`
}

// getCircuitStatus returns the status of a circuit breaker.
func (cb *CircuitBreakers) getCircuitStatus(circuit *circuitbreaker.CircuitBreaker, critical bool) CircuitStatus {
	return CircuitStatus{
		Name:         circuit.Name(),
		State:        circuit.State().String(),
		Critical:     critical,
		FailureCount: circuit.FailureCount(),
		Metrics:      circuit.Metrics(),
	}
}

// HandleCircuitHealth returns an HTTP handler for the circuit health endpoint.
func (cb *CircuitBreakers) HandleCircuitHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		healthy := cb.IsHealthy()
		status := cb.GetStatus()

		response := map[string]interface{}{
			"healthy":  healthy,
			"circuits": status,
		}

		if healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(response)
	}
}

// Reset resets all circuit breakers to closed state.
// This should only be used for administrative purposes.
func (cb *CircuitBreakers) Reset() {
	cb.DB.Reset()
	cb.Redis.Reset()
	cb.Kafka.Reset()
	cb.KafkaConsumer.Reset()
	cb.logger.Info("All circuit breakers reset to closed state")
}

// PriceCache provides an in-memory fallback cache for price data when Redis is unavailable.
type PriceCache struct {
	mu     sync.RWMutex
	prices map[string]PriceCacheEntry
	maxAge time.Duration
}

// PriceCacheEntry represents a cached price entry.
type PriceCacheEntry struct {
	Symbol    string          `json:"symbol"`
	Bid       decimal.Decimal `json:"bid"`
	Ask       decimal.Decimal `json:"ask"`
	Last      decimal.Decimal `json:"last"`
	Timestamp time.Time       `json:"timestamp"`
	CachedAt  time.Time       `json:"cached_at"`
}

// NewPriceCache creates a new in-memory price cache.
func NewPriceCache(maxAge time.Duration) *PriceCache {
	if maxAge <= 0 {
		maxAge = 30 * time.Second // Default 30 second cache
	}
	return &PriceCache{
		prices: make(map[string]PriceCacheEntry),
		maxAge: maxAge,
	}
}

// Get retrieves a price from the cache.
// Returns the entry and a boolean indicating if it was found and is still valid.
func (pc *PriceCache) Get(symbol string) (PriceCacheEntry, bool) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	entry, exists := pc.prices[symbol]
	if !exists {
		return PriceCacheEntry{}, false
	}

	// Check if entry has expired
	if time.Since(entry.CachedAt) > pc.maxAge {
		return PriceCacheEntry{}, false
	}

	return entry, true
}

// Set stores a price in the cache.
func (pc *PriceCache) Set(symbol string, bid, ask, last decimal.Decimal, timestamp time.Time) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.prices[symbol] = PriceCacheEntry{
		Symbol:    symbol,
		Bid:       bid,
		Ask:       ask,
		Last:      last,
		Timestamp: timestamp,
		CachedAt:  time.Now(),
	}
}

// Delete removes a price from the cache.
func (pc *PriceCache) Delete(symbol string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	delete(pc.prices, symbol)
}

// Clear removes all prices from the cache.
func (pc *PriceCache) Clear() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.prices = make(map[string]PriceCacheEntry)
}

// Size returns the number of entries in the cache.
func (pc *PriceCache) Size() int {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return len(pc.prices)
}

// Cleanup removes expired entries from the cache.
func (pc *PriceCache) Cleanup() int {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	removed := 0
	now := time.Now()
	for symbol, entry := range pc.prices {
		if now.Sub(entry.CachedAt) > pc.maxAge {
			delete(pc.prices, symbol)
			removed++
		}
	}
	return removed
}

// StartCleanupRoutine starts a background goroutine that periodically cleans up expired entries.
func (pc *PriceCache) StartCleanupRoutine(ctx context.Context, interval time.Duration, logger *zap.Logger) {
	infra.SafeGo(logger, "price-cache-cleanup", func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed := pc.Cleanup()
				if removed > 0 {
					logger.Debug("Price cache cleanup completed",
						zap.Int("removed_entries", removed),
						zap.Int("remaining_entries", pc.Size()))
				}
			}
		}
	})
}

// GetAll returns all valid (non-expired) prices from the cache.
func (pc *PriceCache) GetAll() map[string]PriceCacheEntry {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	result := make(map[string]PriceCacheEntry)
	now := time.Now()
	for symbol, entry := range pc.prices {
		if now.Sub(entry.CachedAt) <= pc.maxAge {
			result[symbol] = entry
		}
	}
	return result
}
