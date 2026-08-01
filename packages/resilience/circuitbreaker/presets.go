package circuitbreaker

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// DatabaseCircuitConfig returns a Config optimized for database calls.
//
// Configuration rationale:
//   - MaxFailures: 5 - allows some transient errors before tripping
//   - FailureWindow: 10s - short window to detect sustained issues
//   - ResetTimeout: 30s - gives database time to recover
//   - HalfOpenMaxCalls: 3 - test recovery with limited requests
//   - SuccessThreshold: 2 - require 2 successes to confirm recovery
//   - Timeout: 5s - reasonable for most DB queries
//
// The IsFailure function excludes sql.ErrNoRows as it's a normal query result,
// not a database failure.
func DatabaseCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      5,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     30 * time.Second,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 2,
		Timeout:          5 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			// Don't count "no rows" as a failure - it's a valid empty result
			if errors.Is(err, sql.ErrNoRows) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled)
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// DatabaseReplicaCircuitConfig returns a Config optimized for database replica reads.
// More lenient than primary since reads can be retried on other replicas.
func DatabaseReplicaCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      8,
		FailureWindow:    15 * time.Second,
		ResetTimeout:     20 * time.Second,
		HalfOpenMaxCalls: 5,
		SuccessThreshold: 3,
		Timeout:          3 * time.Second, // Lower timeout for reads
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, sql.ErrNoRows) {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// RedisCircuitConfig returns a Config optimized for Redis operations.
//
// Configuration rationale:
//   - MaxFailures: 10 - Redis is typically very fast, allow more failures
//   - FailureWindow: 5s - short window due to Redis's low latency
//   - ResetTimeout: 15s - Redis recovers quickly
//   - HalfOpenMaxCalls: 5 - test recovery with more requests
//   - SuccessThreshold: 3 - require 3 successes for recovery
//   - Timeout: 2s - Redis should be sub-millisecond normally
func RedisCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      10,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     15 * time.Second,
		HalfOpenMaxCalls: 5,
		SuccessThreshold: 3,
		Timeout:          2 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Redis nil is not a failure - it means key doesn't exist
			if errors.Is(err, redis.Nil) {
				return false
			}
			return true
		},
	}
}

// RedisClusterCircuitConfig returns a Config optimized for Redis Cluster.
// Slightly more lenient due to cluster failover capabilities.
func RedisClusterCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      15,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     20 * time.Second,
		HalfOpenMaxCalls: 5,
		SuccessThreshold: 3,
		Timeout:          3 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			if errors.Is(err, redis.Nil) {
				return false
			}
			// MOVED and ASK errors are cluster redirects, not failures
			errStr := err.Error()
			if strings.HasPrefix(errStr, "MOVED") || strings.HasPrefix(errStr, "ASK") {
				return false
			}
			return true
		},
	}
}

// KafkaCircuitConfig returns a Config optimized for Kafka producer operations.
//
// Configuration rationale:
//   - MaxFailures: 3 - Kafka issues are often serious, trip early
//   - FailureWindow: 30s - longer window due to Kafka's batching
//   - ResetTimeout: 60s - Kafka broker recovery takes time
//   - HalfOpenMaxCalls: 2 - conservative testing during recovery
//   - SuccessThreshold: 2 - require 2 successes for recovery
//   - Timeout: 10s - Kafka can be slow during leader election
func KafkaCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      3,
		FailureWindow:    30 * time.Second,
		ResetTimeout:     60 * time.Second,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 2,
		Timeout:          10 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// KafkaConsumerCircuitConfig returns a Config optimized for Kafka consumer operations.
// More lenient than producer since consumer can retry/rebalance.
func KafkaConsumerCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      5,
		FailureWindow:    30 * time.Second,
		ResetTimeout:     45 * time.Second,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 2,
		Timeout:          15 * time.Second, // Higher timeout for poll operations
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// ExternalAPICircuitConfig returns a Config optimized for external HTTP API calls.
//
// Configuration rationale:
//   - MaxFailures: 3 - external APIs can fail, trip early to avoid cascade
//   - FailureWindow: 60s - longer window for external services
//   - ResetTimeout: 120s - external services may take time to recover
//   - HalfOpenMaxCalls: 1 - very conservative recovery testing
//   - SuccessThreshold: 1 - single success is enough to restore confidence
//   - Timeout: 30s - external APIs can be slow
func ExternalAPICircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      3,
		FailureWindow:    60 * time.Second,
		ResetTimeout:     120 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Timeout:          30 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// ShardRouterCircuitConfig returns a Config optimized for shard router service calls.
//
// Configuration rationale:
//   - MaxFailures: 5 - allow some transient failures
//   - FailureWindow: 10s - detect issues quickly
//   - ResetTimeout: 10s - shard router should recover quickly
//   - HalfOpenMaxCalls: 3 - test recovery with moderate traffic
//   - SuccessThreshold: 2 - require 2 successes for recovery
//   - Timeout: 2s - shard lookups should be fast
func ShardRouterCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      5,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     10 * time.Second,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 2,
		Timeout:          2 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// WebSocketCircuitConfig returns a Config optimized for WebSocket connection handling.
// Very conservative since WebSocket failures affect real-time user experience.
func WebSocketCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      3,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     10 * time.Second,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 2,
		Timeout:          5 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// HTTPClientCircuitConfig returns a Config for general HTTP client operations.
func HTTPClientCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      5,
		FailureWindow:    30 * time.Second,
		ResetTimeout:     30 * time.Second,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 2,
		Timeout:          10 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// GRPCCircuitConfig returns a Config optimized for gRPC service calls.
func GRPCCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      5,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     20 * time.Second,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 2,
		Timeout:          5 * time.Second,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, ErrCircuitOpen) {
				return false
			}
			// Don't count context cancellation as failure (user cancelled request)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			return true
		},
	}
}

// AggressiveCircuitConfig returns an aggressive Config that trips quickly.
// Use this for critical dependencies where fast fail-over is essential.
func AggressiveCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      2,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     15 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
		Timeout:          3 * time.Second,
	}
}

// LenientCircuitConfig returns a lenient Config that allows more failures.
// Use this for non-critical dependencies or services with known flakiness.
func LenientCircuitConfig(name string) Config {
	return Config{
		Name:             name,
		MaxFailures:      20,
		FailureWindow:    60 * time.Second,
		ResetTimeout:     30 * time.Second,
		HalfOpenMaxCalls: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
	}
}
