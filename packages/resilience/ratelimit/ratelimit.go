// Package ratelimit provides user-based rate limiting for the trading platform.
// It supports both in-memory and Redis-backed rate limiting with sliding window
// algorithms for accurate per-minute limits.
package ratelimit

import (
	"context"
	"os"
	"strconv"
	"time"
)

// RateLimiter defines the interface for rate limiting operations.
type RateLimiter interface {
	// Allow checks if a single request is allowed for the given key.
	Allow(key string) bool

	// AllowN checks if n requests are allowed for the given key.
	AllowN(key string, n int) bool

	// Reset clears the rate limit state for the given key.
	Reset(key string)

	// Remaining returns the number of remaining requests for the given key.
	Remaining(key string) int

	// RetryAfter returns the duration until the next request is allowed.
	// Returns 0 if requests are currently allowed.
	RetryAfter(key string) time.Duration
}

// DistributedRateLimiter extends RateLimiter with context support for distributed systems.
type DistributedRateLimiter interface {
	// AllowCtx checks if a single request is allowed, with context for cancellation.
	AllowCtx(ctx context.Context, key string) (bool, error)

	// AllowNCtx checks if n requests are allowed, with context for cancellation.
	AllowNCtx(ctx context.Context, key string, n int) (bool, error)

	// ResetCtx clears the rate limit state for the given key.
	ResetCtx(ctx context.Context, key string) error

	// RemainingCtx returns the number of remaining requests for the given key.
	RemainingCtx(ctx context.Context, key string) (int, error)

	// RetryAfterCtx returns the duration until the next request is allowed.
	RetryAfterCtx(ctx context.Context, key string) (time.Duration, error)

	// Close releases any resources held by the limiter.
	Close() error
}

// Config holds the configuration for a rate limiter.
type Config struct {
	// Rate is the number of requests allowed per window.
	Rate int

	// Window is the time window for rate limiting.
	Window time.Duration

	// BurstSize is the maximum number of requests that can be made in a burst.
	// For token bucket limiters, this is the bucket capacity.
	// For sliding window limiters, this allows temporary bursting above the rate.
	BurstSize int

	// CleanupInterval is how often to clean up expired entries (in-memory only).
	// Defaults to Window if not set.
	CleanupInterval time.Duration

	// KeyPrefix is an optional prefix for Redis keys (Redis only).
	KeyPrefix string
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Rate <= 0 {
		return ErrInvalidRate
	}
	if c.Window <= 0 {
		return ErrInvalidWindow
	}
	if c.BurstSize < 0 {
		return ErrInvalidBurst
	}
	return nil
}

// WithDefaults returns a copy of the config with default values applied.
func (c Config) WithDefaults() Config {
	if c.BurstSize == 0 {
		c.BurstSize = c.Rate
	}
	if c.CleanupInterval == 0 {
		c.CleanupInterval = c.Window
	}
	return c
}

// Predefined configurations for common use cases.
var (
	// OrderLimitConfig is the default configuration for order placement limits.
	// 10 orders per minute with burst of 5.
	OrderLimitConfig = Config{
		Rate:      10,
		Window:    time.Minute,
		BurstSize: 5,
		KeyPrefix: "rl:order:",
	}

	// APILimitConfig is the default configuration for general API limits.
	// 100 requests per minute with burst of 20.
	APILimitConfig = Config{
		Rate:      100,
		Window:    time.Minute,
		BurstSize: 20,
		KeyPrefix: "rl:api:",
	}

	// WebSocketLimitConfig is the default configuration for WebSocket message limits.
	// 50 messages per second with burst of 10.
	WebSocketLimitConfig = Config{
		Rate:      50,
		Window:    time.Second,
		BurstSize: 10,
		KeyPrefix: "rl:ws:",
	}
)

// ConfigFromEnv creates a Config from environment variables with the given prefix.
// For example, with prefix "ORDER_LIMIT":
//   - ORDER_LIMIT_RATE: requests per window
//   - ORDER_LIMIT_WINDOW: window duration (e.g., "1m", "1s")
//   - ORDER_LIMIT_BURST: burst size
//   - ORDER_LIMIT_KEY_PREFIX: Redis key prefix
func ConfigFromEnv(prefix string, defaults Config) Config {
	cfg := defaults

	if v := os.Getenv(prefix + "_RATE"); v != "" {
		if rate, err := strconv.Atoi(v); err == nil && rate > 0 {
			cfg.Rate = rate
		}
	}

	if v := os.Getenv(prefix + "_WINDOW"); v != "" {
		if window, err := time.ParseDuration(v); err == nil && window > 0 {
			cfg.Window = window
		}
	}

	if v := os.Getenv(prefix + "_BURST"); v != "" {
		if burst, err := strconv.Atoi(v); err == nil && burst >= 0 {
			cfg.BurstSize = burst
		}
	}

	if v := os.Getenv(prefix + "_KEY_PREFIX"); v != "" {
		cfg.KeyPrefix = v
	}

	return cfg.WithDefaults()
}

// LimitType represents the type of rate limit being applied.
type LimitType string

const (
	// LimitTypeOrder is for order placement limits.
	LimitTypeOrder LimitType = "order"

	// LimitTypeAPI is for general API limits.
	LimitTypeAPI LimitType = "api"

	// LimitTypeWebSocket is for WebSocket message limits.
	LimitTypeWebSocket LimitType = "websocket"
)

// LimitInfo contains information about the current rate limit state.
type LimitInfo struct {
	// Allowed indicates if the request was allowed.
	Allowed bool

	// Limit is the maximum number of requests per window.
	Limit int

	// Remaining is the number of remaining requests in the current window.
	Remaining int

	// ResetAt is when the current window resets.
	ResetAt time.Time

	// RetryAfter is how long to wait before retrying (if not allowed).
	RetryAfter time.Duration
}

// Result wraps a rate limit decision with metadata.
type Result struct {
	LimitInfo

	// Key is the rate limit key that was checked.
	Key string

	// LimitType is the type of limit that was applied.
	LimitType LimitType
}
