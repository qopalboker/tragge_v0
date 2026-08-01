package resilience

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

var (
	// ErrMaxRetriesExceeded is returned when all retry attempts have failed
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
)

// RetryConfig holds configuration for retry behavior
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including initial)
	MaxAttempts int
	// InitialDelay is the delay before the first retry
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// Multiplier is the factor by which delay increases
	Multiplier float64
	// Jitter adds randomness to prevent thundering herd
	Jitter bool
	// RetryIf determines if an error should be retried
	RetryIf func(error) bool
}

// DefaultRetryConfig returns sensible defaults for retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryIf: func(err error) bool {
			// Don't retry context errors
			return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
		},
	}
}

// DatabaseRetryConfig returns retry configuration optimized for database operations
func DatabaseRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryIf: func(err error) bool {
			// Only retry transient errors
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			// Add specific database error checks here
			return true
		},
	}
}

// CacheRetryConfig returns retry configuration optimized for cache operations
func CacheRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       true,
		RetryIf: func(err error) bool {
			return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
		},
	}
}

// MessageQueueRetryConfig returns retry configuration for message queue operations
func MessageQueueRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
		RetryIf: func(err error) bool {
			return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
		},
	}
}

// Retry executes a function with retry logic
func Retry[T any](ctx context.Context, cfg RetryConfig, fn func(ctx context.Context) (T, error)) (T, error) {
	var lastErr error
	var zero T

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// Check context before attempt
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}

		result, err := fn(ctx)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if cfg.RetryIf != nil && !cfg.RetryIf(err) {
			return zero, err
		}

		// Don't sleep after the last attempt
		if attempt < cfg.MaxAttempts-1 {
			delay := calculateDelay(cfg, attempt)
			select {
			case <-ctx.Done():
				return zero, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return zero, errors.Join(ErrMaxRetriesExceeded, lastErr)
}

// RetryVoid executes a function that returns only an error with retry logic
func RetryVoid(ctx context.Context, cfg RetryConfig, fn func(ctx context.Context) error) error {
	_, err := Retry(ctx, cfg, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

// calculateDelay calculates the delay for a retry attempt with exponential backoff
func calculateDelay(cfg RetryConfig, attempt int) time.Duration {
	delay := float64(cfg.InitialDelay) * math.Pow(cfg.Multiplier, float64(attempt))

	// Apply max delay cap
	if delay > float64(cfg.MaxDelay) {
		delay = float64(cfg.MaxDelay)
	}

	// Apply jitter (±25%)
	if cfg.Jitter {
		jitter := delay * 0.25 * (2*rand.Float64() - 1)
		delay += jitter
	}

	return time.Duration(delay)
}

// WithRetry wraps a Resilience execution with retry logic
func (r *Resilience) WithRetry(depName string, retryCfg RetryConfig, fn func(ctx context.Context) (any, error)) (any, error) {
	return r.WithRetryContext(context.Background(), depName, retryCfg, fn)
}

// WithRetryContext wraps a Resilience execution with retry logic and context
func (r *Resilience) WithRetryContext(ctx context.Context, depName string, retryCfg RetryConfig, fn func(ctx context.Context) (any, error)) (any, error) {
	return Retry(ctx, retryCfg, func(ctx context.Context) (any, error) {
		return r.ExecuteWithContext(ctx, depName, fn)
	})
}
