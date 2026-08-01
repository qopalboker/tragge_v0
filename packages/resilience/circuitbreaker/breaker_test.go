package circuitbreaker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestNewCircuitBreaker verifies circuit breaker initialization.
func TestNewCircuitBreaker(t *testing.T) {
	cb := New(Config{Name: "test"})

	if cb.State() != StateClosed {
		t.Errorf("expected initial state to be Closed, got %s", cb.State())
	}

	if cb.Name() != "test" {
		t.Errorf("expected name to be 'test', got %s", cb.Name())
	}
}

// TestDefaultConfig verifies default values are applied.
func TestDefaultConfig(t *testing.T) {
	cb := New(Config{Name: "test-defaults"})

	// Execute a successful call to verify defaults work
	err := cb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// TestStateTransitionClosedToOpen verifies the circuit opens after MaxFailures.
func TestStateTransitionClosedToOpen(t *testing.T) {
	cb := New(Config{
		Name:          "test-open",
		MaxFailures:   3,
		FailureWindow: 10 * time.Second,
		ResetTimeout:  100 * time.Millisecond,
	})

	testErr := errors.New("test error")

	// Generate failures
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state to be Open after %d failures, got %s", 3, cb.State())
	}

	// Verify subsequent calls are rejected
	err := cb.Execute(func() error {
		return nil
	})

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

// TestStateTransitionOpenToHalfOpen verifies the circuit transitions to HalfOpen after ResetTimeout.
func TestStateTransitionOpenToHalfOpen(t *testing.T) {
	cb := New(Config{
		Name:          "test-half-open",
		MaxFailures:   2,
		FailureWindow: 10 * time.Second,
		ResetTimeout:  50 * time.Millisecond,
	})

	testErr := errors.New("test error")

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state to be Open, got %s", cb.State())
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Check state - should transition to HalfOpen
	state := cb.State()
	if state != StateHalfOpen {
		t.Errorf("expected state to be HalfOpen after reset timeout, got %s", state)
	}
}

// TestStateTransitionHalfOpenToClosed verifies the circuit closes after successful calls in HalfOpen.
func TestStateTransitionHalfOpenToClosed(t *testing.T) {
	stateChanges := make([]State, 0)
	var mu sync.Mutex

	cb := New(Config{
		Name:             "test-recovery",
		MaxFailures:      2,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 2,
		OnStateChange: func(name string, from, to State) {
			mu.Lock()
			stateChanges = append(stateChanges, to)
			mu.Unlock()
		},
	})

	testErr := errors.New("test error")

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Execute successful calls to recover
	for i := 0; i < 2; i++ {
		err := cb.Execute(func() error {
			return nil
		})
		if err != nil {
			t.Errorf("expected no error during recovery, got %v", err)
		}
	}

	// Small wait for state transition
	time.Sleep(10 * time.Millisecond)

	if cb.State() != StateClosed {
		t.Errorf("expected state to be Closed after successful recovery, got %s", cb.State())
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify state transitions: Open -> HalfOpen -> Closed
	if len(stateChanges) < 2 {
		t.Errorf("expected at least 2 state changes, got %d", len(stateChanges))
	}
}

// TestStateTransitionHalfOpenToOpen verifies the circuit re-opens on failure in HalfOpen.
func TestStateTransitionHalfOpenToOpen(t *testing.T) {
	cb := New(Config{
		Name:             "test-reopen",
		MaxFailures:      2,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 2,
	})

	testErr := errors.New("test error")

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Force state check to trigger HalfOpen
	_ = cb.State()

	// Fail in HalfOpen state
	_ = cb.Execute(func() error {
		return testErr
	})

	if cb.State() != StateOpen {
		t.Errorf("expected state to be Open after failure in HalfOpen, got %s", cb.State())
	}
}

// TestFailureWindow verifies failures outside the window are not counted.
func TestFailureWindow(t *testing.T) {
	cb := New(Config{
		Name:          "test-window",
		MaxFailures:   3,
		FailureWindow: 50 * time.Millisecond,
		ResetTimeout:  100 * time.Millisecond,
	})

	testErr := errors.New("test error")

	// Generate 2 failures
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	// Wait for failures to expire
	time.Sleep(60 * time.Millisecond)

	// This failure should not trip the circuit (only 1 failure in window)
	_ = cb.Execute(func() error {
		return testErr
	})

	if cb.State() != StateClosed {
		t.Errorf("expected state to remain Closed (failures expired), got %s", cb.State())
	}
}

// TestTimeout verifies the timeout functionality.
func TestTimeout(t *testing.T) {
	cb := New(Config{
		Name:        "test-timeout",
		MaxFailures: 5,
		Timeout:     50 * time.Millisecond,
	})

	err := cb.Execute(func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})

	if !errors.Is(err, ErrCircuitTimeout) {
		t.Errorf("expected ErrCircuitTimeout, got %v", err)
	}
}

// TestExecuteWithContext verifies context cancellation is handled.
func TestExecuteWithContext(t *testing.T) {
	cb := New(Config{
		Name:        "test-context",
		MaxFailures: 5,
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running operation
	errCh := make(chan error, 1)
	go func() {
		errCh <- cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
				return nil
			}
		})
	}()

	// Cancel the context
	time.Sleep(10 * time.Millisecond)
	cancel()

	err := <-errCh
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestCustomIsFailure verifies custom failure detection.
func TestCustomIsFailure(t *testing.T) {
	cb := New(Config{
		Name:        "test-custom-failure",
		MaxFailures: 2,
		IsFailure: func(err error) bool {
			// Only count specific errors as failures
			return err != nil && err.Error() == "critical error"
		},
	})

	// Non-critical errors should not count
	for i := 0; i < 5; i++ {
		_ = cb.Execute(func() error {
			return errors.New("minor error")
		})
	}

	if cb.State() != StateClosed {
		t.Errorf("expected state to remain Closed (non-critical errors), got %s", cb.State())
	}

	// Critical errors should count
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return errors.New("critical error")
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state to be Open after critical errors, got %s", cb.State())
	}
}

// TestReset verifies the Reset function.
func TestReset(t *testing.T) {
	cb := New(Config{
		Name:         "test-reset",
		MaxFailures:  2,
		ResetTimeout: 10 * time.Second, // Long timeout
	})

	testErr := errors.New("test error")

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state to be Open, got %s", cb.State())
	}

	// Reset the circuit
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("expected state to be Closed after reset, got %s", cb.State())
	}

	// Verify requests are allowed again
	err := cb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

// TestMetrics verifies metrics are recorded correctly.
func TestMetrics(t *testing.T) {
	cb := New(Config{
		Name:        "test-metrics",
		MaxFailures: 10,
	})

	testErr := errors.New("test error")

	// Generate some successes
	for i := 0; i < 5; i++ {
		_ = cb.Execute(func() error {
			return nil
		})
	}

	// Generate some failures
	for i := 0; i < 3; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	metrics := cb.Metrics()

	if metrics.TotalRequests != 8 {
		t.Errorf("expected TotalRequests=8, got %d", metrics.TotalRequests)
	}

	if metrics.TotalSuccesses != 5 {
		t.Errorf("expected TotalSuccesses=5, got %d", metrics.TotalSuccesses)
	}

	if metrics.TotalFailures != 3 {
		t.Errorf("expected TotalFailures=3, got %d", metrics.TotalFailures)
	}

	if metrics.ConsecutiveFailures != 3 {
		t.Errorf("expected ConsecutiveFailures=3, got %d", metrics.ConsecutiveFailures)
	}
}

// TestConcurrentAccess verifies thread safety under concurrent load.
func TestConcurrentAccess(t *testing.T) {
	cb := New(Config{
		Name:          "test-concurrent",
		MaxFailures:   100,
		FailureWindow: 10 * time.Second,
		ResetTimeout:  50 * time.Millisecond,
	})

	const goroutines = 100
	const callsPerGoroutine = 100

	var wg sync.WaitGroup
	var successCount atomic.Int64
	var failCount atomic.Int64

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				err := cb.Execute(func() error {
					// Alternate between success and failure
					if (id+j)%3 == 0 {
						return errors.New("intentional failure")
					}
					return nil
				})
				if err == nil {
					successCount.Add(1)
				} else {
					failCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	total := successCount.Load() + failCount.Load()
	expectedTotal := int64(goroutines * callsPerGoroutine)

	if total != expectedTotal {
		t.Errorf("expected total calls=%d, got %d", expectedTotal, total)
	}

	// Verify metrics match
	metrics := cb.Metrics()
	if metrics.TotalRequests != expectedTotal {
		t.Errorf("expected TotalRequests=%d, got %d", expectedTotal, metrics.TotalRequests)
	}
}

// TestHalfOpenMaxCalls verifies the limit on calls in HalfOpen state.
func TestHalfOpenMaxCalls(t *testing.T) {
	cb := New(Config{
		Name:             "test-halfopen-limit",
		MaxFailures:      2,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 3, // Higher than HalfOpenMaxCalls so it won't recover
	})

	testErr := errors.New("test error")

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected circuit to be Open, got %s", cb.State())
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Force state check to transition to HalfOpen
	state := cb.State()
	if state != StateHalfOpen {
		t.Fatalf("expected circuit to be HalfOpen, got %s", state)
	}

	// First call should be allowed in HalfOpen
	err1 := cb.Execute(func() error {
		return nil
	})
	if err1 != nil {
		t.Errorf("expected err1 to be nil, got %v", err1)
	}

	// Second call should be allowed
	err2 := cb.Execute(func() error {
		return nil
	})
	if err2 != nil {
		t.Errorf("expected err2 to be nil, got %v", err2)
	}

	// Third call should be rejected (exceeded HalfOpenMaxCalls of 2)
	err3 := cb.Execute(func() error {
		return nil
	})
	if !errors.Is(err3, ErrCircuitOpen) {
		t.Errorf("expected err3 to be ErrCircuitOpen, got %v", err3)
	}
}

// TestStateString verifies State.String() method.
func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.state.String())
		}
	}
}

// TestDatabasePreset verifies the database preset excludes sql.ErrNoRows.
func TestDatabasePreset(t *testing.T) {
	cfg := DatabaseCircuitConfig("test-db")
	cb := New(cfg)

	// sql.ErrNoRows should not be counted as failure
	for i := 0; i < 10; i++ {
		_ = cb.Execute(func() error {
			return sql.ErrNoRows
		})
	}

	if cb.State() != StateClosed {
		t.Errorf("expected state to remain Closed (sql.ErrNoRows not counted), got %s", cb.State())
	}

	// Real errors should be counted
	for i := 0; i < 5; i++ {
		_ = cb.Execute(func() error {
			return errors.New("connection refused")
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("expected state to be Open after real errors, got %s", cb.State())
	}
}

// TestFailureCount verifies the FailureCount method.
func TestFailureCount(t *testing.T) {
	cb := New(Config{
		Name:          "test-failure-count",
		MaxFailures:   10,
		FailureWindow: 100 * time.Millisecond,
	})

	testErr := errors.New("test error")

	// Generate failures
	for i := 0; i < 5; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	count := cb.FailureCount()
	if count != 5 {
		t.Errorf("expected failure count=5, got %d", count)
	}

	// Wait for failures to expire
	time.Sleep(150 * time.Millisecond)

	count = cb.FailureCount()
	if count != 0 {
		t.Errorf("expected failure count=0 after window expiry, got %d", count)
	}
}

// TestOnStateChangeCallback verifies the callback is called on state changes.
func TestOnStateChangeCallback(t *testing.T) {
	var callbackCalled atomic.Int32
	var lastFrom, lastTo atomic.Int32

	cb := New(Config{
		Name:         "test-callback",
		MaxFailures:  2,
		ResetTimeout: 50 * time.Millisecond,
		OnStateChange: func(name string, from, to State) {
			callbackCalled.Add(1)
			lastFrom.Store(int32(from))
			lastTo.Store(int32(to))
		},
	})

	testErr := errors.New("test error")

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return testErr
		})
	}

	// Wait for callback to be invoked
	time.Sleep(20 * time.Millisecond)

	if callbackCalled.Load() != 1 {
		t.Errorf("expected callback to be called once, got %d", callbackCalled.Load())
	}

	from := State(lastFrom.Load())
	to := State(lastTo.Load())
	if from != StateClosed || to != StateOpen {
		t.Errorf("expected transition Closed->Open, got %s->%s", from, to)
	}
}

// TestNoTimeout verifies that zero timeout means no timeout.
func TestNoTimeout(t *testing.T) {
	cb := New(Config{
		Name:        "test-no-timeout",
		MaxFailures: 5,
		Timeout:     0, // No timeout
	})

	start := time.Now()
	err := cb.Execute(func() error {
		time.Sleep(100 * time.Millisecond)
		return nil
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if elapsed < 100*time.Millisecond {
		t.Errorf("expected execution to take at least 100ms, took %v", elapsed)
	}
}

// BenchmarkExecuteSuccess benchmarks successful circuit breaker execution.
func BenchmarkExecuteSuccess(b *testing.B) {
	cb := New(Config{
		Name:        "bench-success",
		MaxFailures: 1000000,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Execute(func() error {
			return nil
		})
	}
}

// BenchmarkExecuteClosed benchmarks circuit breaker overhead when closed.
func BenchmarkExecuteClosed(b *testing.B) {
	cb := New(Config{
		Name:        "bench-closed",
		MaxFailures: 1000000,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Execute(func() error {
				return nil
			})
		}
	})
}

// BenchmarkExecuteOpen benchmarks circuit breaker rejection when open.
func BenchmarkExecuteOpen(b *testing.B) {
	cb := New(Config{
		Name:         "bench-open",
		MaxFailures:  1,
		ResetTimeout: 1 * time.Hour, // Long timeout to stay open
	})

	// Trip the circuit
	_ = cb.Execute(func() error {
		return errors.New("trip")
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cb.Execute(func() error {
			return nil
		})
	}
}

// BenchmarkConcurrentExecute benchmarks concurrent execution.
func BenchmarkConcurrentExecute(b *testing.B) {
	cb := New(Config{
		Name:        "bench-concurrent",
		MaxFailures: 1000000,
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Execute(func() error {
				return nil
			})
		}
	})
}

// TestExecuteWithContextTimeout verifies that a context-aware function
// receives timeout via context and reports ErrCircuitTimeout.
func TestExecuteWithContextTimeout(t *testing.T) {
	cb := New(Config{
		Name:        "test-ctx-timeout",
		MaxFailures: 5,
		Timeout:     50 * time.Millisecond,
	})

	err := cb.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			return nil
		}
	})

	if !errors.Is(err, ErrCircuitTimeout) {
		t.Errorf("expected ErrCircuitTimeout, got %v", err)
	}

	metrics := cb.Metrics()
	if metrics.TotalTimeouts != 1 {
		t.Errorf("expected 1 timeout, got %d", metrics.TotalTimeouts)
	}
}

// TestExecuteWithContextNoGoroutineLeak verifies that ExecuteWithContext
// does not leak goroutines when the function respects context cancellation.
func TestExecuteWithContextNoGoroutineLeak(t *testing.T) {
	cb := New(Config{
		Name:        "test-no-leak",
		MaxFailures: 5,
		Timeout:     50 * time.Millisecond,
	})

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	for i := 0; i < 100; i++ {
		_ = cb.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		})
	}

	// Allow goroutines to settle
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	after := runtime.NumGoroutine()

	// Should not have leaked goroutines (allow small margin for runtime jitter)
	if after-before > 5 {
		t.Errorf("possible goroutine leak: before=%d, after=%d", before, after)
	}
}

// TestRedisNilNotCountedAsFailure verifies the Redis preset excludes redis.Nil errors.
func TestRedisNilNotCountedAsFailure(t *testing.T) {
	cfg := RedisCircuitConfig("test-redis-nil")
	cb := New(cfg)

	// redis.Nil should not be counted as failure
	for i := 0; i < 20; i++ {
		_ = cb.Execute(func() error {
			return redis.Nil
		})
	}

	if cb.State() != StateClosed {
		t.Errorf("expected state to remain Closed (redis.Nil not counted), got %s", cb.State())
	}

	// Also test wrapped redis.Nil (errors.Is traverses the chain)
	for i := 0; i < 20; i++ {
		_ = cb.Execute(func() error {
			return fmt.Errorf("failed to get key: %w", redis.Nil)
		})
	}

	if cb.State() != StateClosed {
		t.Errorf("expected state to remain Closed (wrapped redis.Nil not counted), got %s", cb.State())
	}
}

// TestRedisClusterNilNotCountedAsFailure verifies the Redis Cluster preset excludes redis.Nil errors.
func TestRedisClusterNilNotCountedAsFailure(t *testing.T) {
	cfg := RedisClusterCircuitConfig("test-redis-cluster-nil")
	cb := New(cfg)

	// redis.Nil should not be counted as failure
	for i := 0; i < 20; i++ {
		_ = cb.Execute(func() error {
			return redis.Nil
		})
	}

	if cb.State() != StateClosed {
		t.Errorf("expected state to remain Closed (redis.Nil not counted), got %s", cb.State())
	}
}

// TestHalfOpenRaceCondition verifies that concurrent transitions from
// Open to HalfOpen do not corrupt the halfOpenCalls counter.
func TestHalfOpenRaceCondition(t *testing.T) {
	cb := New(Config{
		Name:             "test-halfopen-race",
		MaxFailures:      2,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     50 * time.Millisecond,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 100, // High threshold so circuit stays HalfOpen
	})

	testErr := errors.New("test error")

	// Trip the circuit
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return testErr })
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected Open, got %s", cb.State())
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Race: many goroutines simultaneously try to enter while Open -> HalfOpen
	var wg sync.WaitGroup
	var allowed atomic.Int32

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Execute(func() error { return nil })
			if err == nil {
				allowed.Add(1)
			}
		}()
	}

	wg.Wait()

	// The first request transitions from Open and is allowed (not counted in halfOpenCalls).
	// Then at most HalfOpenMaxCalls more requests are allowed via the HalfOpen path.
	maxAllowed := int32(cb.config.HalfOpenMaxCalls) + 1
	if allowed.Load() > maxAllowed {
		t.Errorf("expected at most %d allowed calls in HalfOpen, got %d",
			maxAllowed, allowed.Load())
	}
}

