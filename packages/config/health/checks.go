package health

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// DatabaseChecker creates a check function for a SQL database.
func DatabaseChecker(db *sql.DB) CheckFunc {
	return func(ctx context.Context) error {
		if db == nil {
			return fmt.Errorf("database connection is nil")
		}
		return db.PingContext(ctx)
	}
}

// RedisPinger is an interface for Redis clients that support Ping.
type RedisPinger interface {
	Ping(ctx context.Context) error
}

// RedisChecker creates a check function for a Redis client.
func RedisChecker(client RedisPinger) CheckFunc {
	return func(ctx context.Context) error {
		if client == nil {
			return fmt.Errorf("redis client is nil")
		}
		return client.Ping(ctx)
	}
}

// KafkaChecker is an interface for checking Kafka connectivity.
type KafkaChecker interface {
	// Ping checks if the Kafka broker is reachable.
	Ping(ctx context.Context) error
}

// KafkaConnectivityChecker creates a check function for Kafka connectivity.
func KafkaConnectivityChecker(checker KafkaChecker) CheckFunc {
	return func(ctx context.Context) error {
		if checker == nil {
			return fmt.Errorf("kafka checker is nil")
		}
		return checker.Ping(ctx)
	}
}

// CircuitBreakerHealthChecker is an interface for circuit breaker health checks.
type CircuitBreakerHealthChecker interface {
	IsHealthy() bool
}

// CircuitBreakerChecker creates a check function for circuit breakers.
func CircuitBreakerChecker(cb CircuitBreakerHealthChecker) CheckFunc {
	return func(ctx context.Context) error {
		if cb == nil {
			return nil // No circuit breaker configured, assume healthy
		}
		if !cb.IsHealthy() {
			return fmt.Errorf("circuit breakers are in unhealthy state")
		}
		return nil
	}
}

// InitializationChecker creates a check function for service initialization state.
type InitializationChecker interface {
	IsReady() bool
}

// InitChecker creates a check function for service initialization.
func InitChecker(init InitializationChecker) CheckFunc {
	return func(ctx context.Context) error {
		if init == nil {
			return nil
		}
		if !init.IsReady() {
			return fmt.Errorf("service not fully initialized")
		}
		return nil
	}
}

// WebSocketChecker is an interface for WebSocket provider health checks.
type WebSocketChecker interface {
	IsConnected() bool
}

// WebSocketConnectivityChecker creates a check function for WebSocket connections.
func WebSocketConnectivityChecker(ws WebSocketChecker) CheckFunc {
	return func(ctx context.Context) error {
		if ws == nil {
			return fmt.Errorf("websocket checker is nil")
		}
		if !ws.IsConnected() {
			return fmt.Errorf("websocket not connected")
		}
		return nil
	}
}

// ShardChecker is an interface for shard availability checks.
type ShardChecker interface {
	IsHealthy() bool
	ShardCount() int
}

// ShardHealthChecker creates a check function for shard availability.
func ShardHealthChecker(shard ShardChecker) CheckFunc {
	return func(ctx context.Context) error {
		if shard == nil {
			return nil // No sharding configured
		}
		if !shard.IsHealthy() {
			return fmt.Errorf("no active shards available")
		}
		return nil
	}
}

// PoolHealthChecker is an interface for database pool health checks.
type PoolHealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// PoolChecker creates a check function for database pools.
func PoolChecker(pool PoolHealthChecker) CheckFunc {
	return func(ctx context.Context) error {
		if pool == nil {
			return fmt.Errorf("database pool is nil")
		}
		return pool.HealthCheck(ctx)
	}
}

// CacheHealthChecker is an interface for cache health checks.
type CacheHealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// CacheChecker creates a check function for caches.
func CacheChecker(cache CacheHealthChecker) CheckFunc {
	return func(ctx context.Context) error {
		if cache == nil {
			return fmt.Errorf("cache is nil")
		}
		return cache.HealthCheck(ctx)
	}
}

// CustomChecker creates a check function from a simple boolean function.
func CustomChecker(name string, check func() bool) CheckFunc {
	return func(ctx context.Context) error {
		if !check() {
			return fmt.Errorf("%s check failed", name)
		}
		return nil
	}
}

// TimeoutChecker wraps a check function with a specific timeout.
func TimeoutChecker(check CheckFunc, timeout time.Duration) CheckFunc {
	return func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return check(ctx)
	}
}
