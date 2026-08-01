package ratelimit

import "errors"

// Error definitions for the ratelimit package.
var (
	// ErrRateLimitExceeded is returned when the rate limit has been exceeded.
	ErrRateLimitExceeded = errors.New("rate limit exceeded")

	// ErrInvalidRate is returned when the rate is invalid (must be > 0).
	ErrInvalidRate = errors.New("rate must be greater than 0")

	// ErrInvalidWindow is returned when the window is invalid (must be > 0).
	ErrInvalidWindow = errors.New("window must be greater than 0")

	// ErrInvalidBurst is returned when the burst size is invalid (must be >= 0).
	ErrInvalidBurst = errors.New("burst size must be non-negative")

	// ErrRedisUnavailable is returned when Redis is not available.
	ErrRedisUnavailable = errors.New("redis unavailable")

	// ErrInvalidKey is returned when the key is empty or invalid.
	ErrInvalidKey = errors.New("key must not be empty")
)
