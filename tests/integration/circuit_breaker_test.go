// Package integration provides integration tests for circuit breaker functionality.
package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/resilience"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"go.uber.org/zap"
)

// =============================================================================
// Test Scenario A: Circuit Opens on Failures
// =============================================================================

// TestCircuitOpensOnDatabaseFailures verifies that the circuit breaker opens
// after the configured number of failures in the failure window.
func TestCircuitOpensOnDatabaseFailures(t *testing.T) {
	ctx := context.Background()
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create a circuit breaker with low thresholds for testing
	cfg := circuitbreaker.Config{
		Name:             "test-db",
		MaxFailures:      3,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     2 * time.Second,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 1,
	}
	cb := circuitbreaker.New(cfg)

	// Verify circuit starts closed
	if cb.State() != circuitbreaker.StateClosed {
		t.Fatalf("Expected circuit to start closed, got %s", cb.State())
	}

	// Simulate database failures by using an invalid query
	simulatedError := errors.New("simulated database connection failure")

	// Cause failures to trip the circuit
	for i := 0; i < cfg.MaxFailures; i++ {
		err := cb.Execute(func() error {
			return simulatedError
		})
		if err != simulatedError {
			t.Fatalf("Expected simulated error, got %v", err)
		}
	}

	// Verify circuit is now open
	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open after %d failures, got %s", cfg.MaxFailures, cb.State())
	}

	// Verify new requests are rejected with ErrCircuitOpen
	err := cb.Execute(func() error {
		t.Error("Function should not be called when circuit is open")
		return nil
	})

	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Fatalf("Expected ErrCircuitOpen, got %v", err)
	}

	// Verify metrics
	metrics := cb.Metrics()
	if metrics.TotalFailures < int64(cfg.MaxFailures) {
		t.Errorf("Expected at least %d failures, got %d", cfg.MaxFailures, metrics.TotalFailures)
	}
	if metrics.TotalRejections < 1 {
		t.Errorf("Expected at least 1 rejection, got %d", metrics.TotalRejections)
	}

	t.Logf("Circuit opened after %d failures, rejected %d requests", metrics.TotalFailures, metrics.TotalRejections)
}

// TestCircuitOpensWithRealDatabase tests circuit breaker behavior with actual database failures.
func TestCircuitOpensWithRealDatabase(t *testing.T) {
	ctx := context.Background()
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create circuit breaker for database
	cfg := circuitbreaker.DatabaseCircuitConfig("postgres-test")
	cfg.MaxFailures = 3
	cfg.FailureWindow = 10 * time.Second
	cfg.ResetTimeout = 2 * time.Second

	cb := circuitbreaker.New(cfg)

	// Simulate connection failures by closing the database connection
	env.DB.Close()

	// Attempt operations that will fail
	for i := 0; i < cfg.MaxFailures; i++ {
		cb.Execute(func() error {
			_, err := env.DB.ExecContext(ctx, "SELECT 1")
			return err
		})
	}

	// Verify circuit opened
	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to open after database failures, got %s", cb.State())
	}

	// Verify requests are rejected fast
	start := time.Now()
	err := cb.Execute(func() error {
		time.Sleep(1 * time.Second) // This should not execute
		return nil
	})
	elapsed := time.Since(start)

	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Fatalf("Expected ErrCircuitOpen, got %v", err)
	}

	// Should fail fast (< 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Circuit did not fail fast, took %v", elapsed)
	}

	t.Logf("Circuit opened and rejected request in %v", elapsed)
}

// TestCircuitOpensOnRedisFailures tests circuit breaker with Redis failures.
func TestCircuitOpensOnRedisFailures(t *testing.T) {
	ctx := context.Background()
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	cfg := circuitbreaker.RedisCircuitConfig("redis-test")
	cfg.MaxFailures = 3
	cfg.ResetTimeout = 2 * time.Second

	cb := circuitbreaker.New(cfg)

	// Close Redis connection to simulate failures
	env.RedisClient.Close()

	// Cause failures
	for i := 0; i < cfg.MaxFailures; i++ {
		cb.Execute(func() error {
			return env.RedisClient.Ping(ctx).Err()
		})
	}

	// Verify circuit is open
	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open, got %s", cb.State())
	}

	t.Log("Circuit opened after Redis failures")
}

// =============================================================================
// Test Scenario B: Circuit Recovery (Half-Open to Closed)
// =============================================================================

// TestCircuitRecoveryHalfOpenToClosed verifies the circuit breaker recovery process.
func TestCircuitRecoveryHalfOpenToClosed(t *testing.T) {
	ctx := context.Background()
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Track state changes
	var stateChanges []struct {
		from, to circuitbreaker.State
	}
	var mu sync.Mutex

	cfg := circuitbreaker.Config{
		Name:             "test-recovery",
		MaxFailures:      2,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     1 * time.Second, // Short timeout for testing
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 2, // Need 2 successes to close
		OnStateChange: func(name string, from, to circuitbreaker.State) {
			mu.Lock()
			stateChanges = append(stateChanges, struct {
				from, to circuitbreaker.State
			}{from, to})
			mu.Unlock()
			t.Logf("State changed: %s -> %s", from, to)
		},
	}
	cb := circuitbreaker.New(cfg)

	// Step 1: Trip the circuit
	simulatedError := errors.New("simulated failure")
	for i := 0; i < cfg.MaxFailures; i++ {
		cb.Execute(func() error {
			return simulatedError
		})
	}

	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open, got %s", cb.State())
	}

	// Step 2: Wait for reset timeout
	t.Log("Waiting for reset timeout...")
	time.Sleep(cfg.ResetTimeout + 100*time.Millisecond)

	// Step 3: Verify circuit transitions to half-open on next request
	// First successful request in half-open state
	err := cb.Execute(func() error {
		return nil // Success
	})
	if err != nil {
		t.Fatalf("Expected success in half-open state, got %v", err)
	}

	// Check state - should be half-open after first success if threshold > 1
	state := cb.State()
	if state != circuitbreaker.StateHalfOpen && state != circuitbreaker.StateClosed {
		t.Fatalf("Expected half-open or closed, got %s", state)
	}

	// Step 4: Execute more successful requests to close the circuit
	for i := 0; i < cfg.SuccessThreshold; i++ {
		cb.Execute(func() error {
			return nil
		})
	}

	// Step 5: Verify circuit is closed
	if cb.State() != circuitbreaker.StateClosed {
		t.Fatalf("Expected circuit to be closed after recovery, got %s", cb.State())
	}

	// Verify state change sequence
	mu.Lock()
	defer mu.Unlock()
	if len(stateChanges) < 2 {
		t.Errorf("Expected at least 2 state changes, got %d", len(stateChanges))
	}

	t.Logf("Circuit recovered successfully. State changes: %d", len(stateChanges))
}

// TestCircuitHalfOpenFailsReopens verifies that failure in half-open reopens the circuit.
func TestCircuitHalfOpenFailsReopens(t *testing.T) {
	cfg := circuitbreaker.Config{
		Name:             "test-halfopen-fail",
		MaxFailures:      2,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     500 * time.Millisecond,
		HalfOpenMaxCalls: 3,
		SuccessThreshold: 2,
	}
	cb := circuitbreaker.New(cfg)

	simulatedError := errors.New("simulated failure")

	// Trip the circuit
	for i := 0; i < cfg.MaxFailures; i++ {
		cb.Execute(func() error {
			return simulatedError
		})
	}

	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open, got %s", cb.State())
	}

	// Wait for half-open
	time.Sleep(cfg.ResetTimeout + 100*time.Millisecond)

	// Fail in half-open state
	cb.Execute(func() error {
		return simulatedError
	})

	// Circuit should be back to open
	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to reopen after half-open failure, got %s", cb.State())
	}

	t.Log("Circuit correctly reopened after failure in half-open state")
}

// TestCircuitRecoveryWithRealDatabase tests recovery with actual database.
func TestCircuitRecoveryWithRealDatabase(t *testing.T) {
	ctx := context.Background()
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	cfg := circuitbreaker.Config{
		Name:             "postgres-recovery-test",
		MaxFailures:      2,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     1 * time.Second,
		HalfOpenMaxCalls: 2,
		SuccessThreshold: 1,
		IsFailure: func(err error) bool {
			if err == nil {
				return false
			}
			if errors.Is(err, sql.ErrNoRows) {
				return false
			}
			return true
		},
	}
	cb := circuitbreaker.New(cfg)

	// Trip circuit with simulated errors
	simulatedError := errors.New("simulated failure")
	for i := 0; i < cfg.MaxFailures; i++ {
		cb.Execute(func() error {
			return simulatedError
		})
	}

	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit open, got %s", cb.State())
	}

	// Wait for recovery window
	time.Sleep(cfg.ResetTimeout + 100*time.Millisecond)

	// Execute successful database operation
	err := cb.Execute(func() error {
		_, err := env.DB.ExecContext(ctx, "SELECT 1")
		return err
	})

	if err != nil {
		t.Fatalf("Expected successful execution, got %v", err)
	}

	// Circuit should be closed after success
	if cb.State() != circuitbreaker.StateClosed {
		t.Fatalf("Expected circuit closed after recovery, got %s", cb.State())
	}

	t.Log("Circuit recovered successfully with real database")
}

// =============================================================================
// Test Scenario C: Fallback Behavior
// =============================================================================

// TestFallbackCalledWhenCircuitOpen verifies fallback is invoked when circuit opens.
func TestFallbackCalledWhenCircuitOpen(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	r := resilience.New(resilience.Config{
		ServiceName: "test-service",
		Logger:      logger,
	})

	fallbackCalled := false
	fallbackResult := "fallback-data"

	// Register dependency with fallback
	r.RegisterDependency("test-db", resilience.DatabaseDep,
		resilience.WithFallback(func(ctx context.Context) (any, error) {
			fallbackCalled = true
			return fallbackResult, nil
		}),
	)

	// Get the dependency to access circuit breaker
	dep, _ := r.GetDependency("test-db")
	cb := dep.CircuitBreaker

	// Trip the circuit
	cfg := cb.Metrics() // Use metrics to understand config
	_ = cfg
	simulatedError := errors.New("simulated failure")

	// Trip circuit by causing failures (default MaxFailures is 5 for DatabaseDep)
	for i := 0; i < 5; i++ {
		r.Execute("test-db", func(ctx context.Context) (any, error) {
			return nil, simulatedError
		})
	}

	// Verify circuit is open
	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open, got %s", cb.State())
	}

	// Execute - should use fallback
	result, err := r.Execute("test-db", func(ctx context.Context) (any, error) {
		t.Error("Main function should not be called when circuit is open")
		return nil, nil
	})

	if err != nil {
		t.Fatalf("Expected no error with fallback, got %v", err)
	}

	if !fallbackCalled {
		t.Error("Expected fallback to be called")
	}

	if result != fallbackResult {
		t.Errorf("Expected fallback result %s, got %v", fallbackResult, result)
	}

	t.Log("Fallback correctly called when circuit was open")
}

// TestFallbackReturnsCorrectData verifies fallback returns expected data.
func TestFallbackReturnsCorrectData(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	type CachedUser struct {
		ID    string
		Email string
	}

	cachedUser := CachedUser{ID: "user-123", Email: "test@example.com"}

	r := resilience.New(resilience.Config{
		ServiceName: "test-service",
		Logger:      logger,
	})

	r.RegisterDependency("user-cache", resilience.CacheDep,
		resilience.WithFallback(func(ctx context.Context) (any, error) {
			return cachedUser, nil
		}),
	)

	// Trip the circuit (Redis has MaxFailures=10)
	for i := 0; i < 10; i++ {
		r.Execute("user-cache", func(ctx context.Context) (any, error) {
			return nil, errors.New("cache failure")
		})
	}

	dep, _ := r.GetDependency("user-cache")
	if dep.CircuitBreaker.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open, got %s", dep.CircuitBreaker.State())
	}

	// Execute and verify fallback data
	result, err := r.Execute("user-cache", func(ctx context.Context) (any, error) {
		return nil, errors.New("should not be called")
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	user, ok := result.(CachedUser)
	if !ok {
		t.Fatalf("Expected CachedUser type, got %T", result)
	}

	if user.ID != cachedUser.ID || user.Email != cachedUser.Email {
		t.Errorf("Fallback data mismatch: expected %+v, got %+v", cachedUser, user)
	}

	t.Logf("Fallback returned correct data: %+v", user)
}

// TestFallbackWithError verifies behavior when fallback itself fails.
func TestFallbackWithError(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	fallbackError := errors.New("fallback also failed")

	r := resilience.New(resilience.Config{
		ServiceName: "test-service",
		Logger:      logger,
	})

	r.RegisterDependency("failing-service", resilience.ExternalAPIDep,
		resilience.WithFallback(func(ctx context.Context) (any, error) {
			return nil, fallbackError
		}),
	)

	// Trip the circuit (ExternalAPI has MaxFailures=3)
	for i := 0; i < 3; i++ {
		r.Execute("failing-service", func(ctx context.Context) (any, error) {
			return nil, errors.New("service failure")
		})
	}

	// Execute - fallback should be called and return its error
	_, err := r.Execute("failing-service", func(ctx context.Context) (any, error) {
		return nil, errors.New("should not be called")
	})

	if !errors.Is(err, fallbackError) {
		t.Fatalf("Expected fallback error, got %v", err)
	}

	t.Log("Fallback error correctly propagated")
}

// TestNoFallbackReturnsCircuitOpenError verifies behavior without fallback.
func TestNoFallbackReturnsCircuitOpenError(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	r := resilience.New(resilience.Config{
		ServiceName: "test-service",
		Logger:      logger,
	})

	// Register without fallback
	r.RegisterDependency("no-fallback", resilience.DatabaseDep)

	// Trip the circuit
	for i := 0; i < 5; i++ {
		r.Execute("no-fallback", func(ctx context.Context) (any, error) {
			return nil, errors.New("failure")
		})
	}

	dep, _ := r.GetDependency("no-fallback")
	if dep.CircuitBreaker.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open, got %s", dep.CircuitBreaker.State())
	}

	// Execute - should return ErrCircuitOpen
	_, err := r.Execute("no-fallback", func(ctx context.Context) (any, error) {
		return nil, nil
	})

	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Fatalf("Expected ErrCircuitOpen, got %v", err)
	}

	t.Log("Correctly returned ErrCircuitOpen when no fallback configured")
}

// =============================================================================
// Test Scenario D: Bulkhead Rejection
// =============================================================================

// TestBulkheadRejectsConcurrentRequests verifies bulkhead limits concurrent requests.
func TestBulkheadRejectsConcurrentRequests(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	bulkheadSize := 5

	r := resilience.New(resilience.Config{
		ServiceName:    "test-service",
		Logger:         logger,
		EnableBulkhead: true,
		BulkheadSize:   bulkheadSize,
	})

	r.RegisterDependency("slow-service", resilience.ExternalAPIDep)

	var wg sync.WaitGroup
	var successCount, rejectedCount int64
	startSignal := make(chan struct{})

	// Launch more concurrent requests than bulkhead allows
	totalRequests := bulkheadSize * 2

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Wait for start signal
			<-startSignal

			_, err := r.Execute("slow-service", func(ctx context.Context) (any, error) {
				// Simulate slow operation
				time.Sleep(200 * time.Millisecond)
				return id, nil
			})

			if errors.Is(err, resilience.ErrBulkheadFull) {
				atomic.AddInt64(&rejectedCount, 1)
			} else if err == nil {
				atomic.AddInt64(&successCount, 1)
			}
		}(i)
	}

	// Start all goroutines simultaneously
	close(startSignal)
	wg.Wait()

	t.Logf("Total requests: %d, Successful: %d, Rejected: %d",
		totalRequests, successCount, rejectedCount)

	// At most bulkheadSize requests should succeed simultaneously
	if rejectedCount == 0 {
		t.Error("Expected some requests to be rejected by bulkhead")
	}

	// Some requests should succeed
	if successCount == 0 {
		t.Error("Expected some requests to succeed")
	}

	// Total should equal totalRequests
	if int(successCount+rejectedCount) != totalRequests {
		t.Errorf("Count mismatch: success(%d) + rejected(%d) != total(%d)",
			successCount, rejectedCount, totalRequests)
	}
}

// TestBulkheadWithCustomSize tests custom bulkhead size per dependency.
func TestBulkheadWithCustomSize(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	r := resilience.New(resilience.Config{
		ServiceName:    "test-service",
		Logger:         logger,
		EnableBulkhead: true,
		BulkheadSize:   100, // Default size
	})

	customSize := 2 // Very small bulkhead

	r.RegisterDependency("limited-service", resilience.DatabaseDep,
		resilience.WithBulkheadSize(customSize),
	)

	var wg sync.WaitGroup
	var rejectedCount int64
	startSignal := make(chan struct{})

	// Launch more requests than custom bulkhead allows
	totalRequests := customSize + 3

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startSignal

			_, err := r.Execute("limited-service", func(ctx context.Context) (any, error) {
				time.Sleep(100 * time.Millisecond)
				return nil, nil
			})

			if errors.Is(err, resilience.ErrBulkheadFull) {
				atomic.AddInt64(&rejectedCount, 1)
			}
		}()
	}

	close(startSignal)
	wg.Wait()

	if rejectedCount == 0 {
		t.Error("Expected some requests to be rejected with small bulkhead")
	}

	t.Logf("Custom bulkhead (size=%d) rejected %d requests", customSize, rejectedCount)
}

// TestBulkheadReleasesOnCompletion verifies slots are released after request completion.
func TestBulkheadReleasesOnCompletion(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	bulkheadSize := 2

	r := resilience.New(resilience.Config{
		ServiceName:    "test-service",
		Logger:         logger,
		EnableBulkhead: true,
		BulkheadSize:   bulkheadSize,
	})

	r.RegisterDependency("release-test", resilience.DatabaseDep)

	// Fill the bulkhead
	var wg sync.WaitGroup
	barrier := make(chan struct{})

	for i := 0; i < bulkheadSize; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.Execute("release-test", func(ctx context.Context) (any, error) {
				<-barrier // Wait until signaled
				return nil, nil
			})
		}()
	}

	// Give goroutines time to acquire bulkhead slots
	time.Sleep(50 * time.Millisecond)

	// This request should be rejected (bulkhead full)
	_, err := r.Execute("release-test", func(ctx context.Context) (any, error) {
		return nil, nil
	})

	if !errors.Is(err, resilience.ErrBulkheadFull) {
		t.Errorf("Expected ErrBulkheadFull while bulkhead is full, got %v", err)
	}

	// Release the barrier - allow goroutines to complete
	close(barrier)
	wg.Wait()

	// Now bulkhead should have room
	_, err = r.Execute("release-test", func(ctx context.Context) (any, error) {
		return "success", nil
	})

	if err != nil {
		t.Fatalf("Expected success after bulkhead release, got %v", err)
	}

	t.Log("Bulkhead correctly released slots after completion")
}

// TestBulkheadWithCircuitBreaker verifies bulkhead and circuit breaker work together.
func TestBulkheadWithCircuitBreaker(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	r := resilience.New(resilience.Config{
		ServiceName:    "test-service",
		Logger:         logger,
		EnableBulkhead: true,
		BulkheadSize:   10,
	})

	r.RegisterDependency("combo-test", resilience.DatabaseDep)

	// First, trip the circuit breaker
	for i := 0; i < 5; i++ {
		r.Execute("combo-test", func(ctx context.Context) (any, error) {
			return nil, errors.New("failure")
		})
	}

	dep, _ := r.GetDependency("combo-test")
	if dep.CircuitBreaker.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open, got %s", dep.CircuitBreaker.State())
	}

	// Requests should fail with circuit open (not bulkhead full)
	// Because circuit check happens after bulkhead acquisition
	_, err := r.Execute("combo-test", func(ctx context.Context) (any, error) {
		return nil, nil
	})

	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Fatalf("Expected ErrCircuitOpen, got %v", err)
	}

	t.Log("Circuit breaker and bulkhead work together correctly")
}

// =============================================================================
// Test Scenario E: Service Health Check
// =============================================================================

// TestHealthCircuitsEndpoint tests the /health/circuits endpoint format.
func TestHealthCircuitsEndpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create a Circuits instance similar to trade-bff
	circuits := NewTestCircuits(logger)

	// Create test server
	handler := http.HandlerFunc(circuits.HandleCircuitHealth)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Test basic health check
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to get health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var healthResp CircuitHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify response structure
	if healthResp.Status != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", healthResp.Status)
	}

	if !healthResp.Healthy {
		t.Error("Expected healthy=true")
	}

	if len(healthResp.Circuits) == 0 {
		t.Error("Expected circuits in response")
	}

	// Verify each circuit has expected fields
	for name, info := range healthResp.Circuits {
		if info.State == "" {
			t.Errorf("Circuit %s missing state", name)
		}
		if info.State != "closed" {
			t.Errorf("Expected circuit %s to be closed, got %s", name, info.State)
		}
	}

	t.Logf("Health response: %+v", healthResp)
}

// TestHealthCircuitsWithMetrics tests the metrics query parameter.
func TestHealthCircuitsWithMetrics(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	circuits := NewTestCircuits(logger)

	handler := http.HandlerFunc(circuits.HandleCircuitHealth)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Request with metrics
	resp, err := http.Get(server.URL + "?metrics=true")
	if err != nil {
		t.Fatalf("Failed to get health: %v", err)
	}
	defer resp.Body.Close()

	var healthResp CircuitHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Verify metrics are included
	if healthResp.Metrics == nil {
		t.Error("Expected metrics in response when metrics=true")
	}

	for name, metrics := range healthResp.Metrics {
		t.Logf("Circuit %s metrics: requests=%d, successes=%d, failures=%d",
			name, metrics.TotalRequests, metrics.TotalSuccesses, metrics.TotalFailures)
	}
}

// TestHealthDegradedWhenCircuitOpen tests degraded status when circuit opens.
func TestHealthDegradedWhenCircuitOpen(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	circuits := NewTestCircuits(logger)

	// Trip the database circuit (critical)
	for i := 0; i < 5; i++ {
		circuits.Database.Execute(func() error {
			return errors.New("database failure")
		})
	}

	if circuits.Database.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected database circuit to be open, got %s", circuits.Database.State())
	}

	handler := http.HandlerFunc(circuits.HandleCircuitHealth)
	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to get health: %v", err)
	}
	defer resp.Body.Close()

	// Should return 503 when critical circuit is open
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", resp.StatusCode)
	}

	var healthResp CircuitHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&healthResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if healthResp.Status != "degraded" {
		t.Errorf("Expected status 'degraded', got '%s'", healthResp.Status)
	}

	if healthResp.Healthy {
		t.Error("Expected healthy=false when critical circuit is open")
	}

	// Verify database circuit shows as open
	dbInfo, ok := healthResp.Circuits["postgres"]
	if !ok {
		t.Error("Expected postgres circuit in response")
	} else if dbInfo.State != "open" {
		t.Errorf("Expected postgres circuit to be 'open', got '%s'", dbInfo.State)
	}

	t.Logf("Health degraded response: status=%s, healthy=%v", healthResp.Status, healthResp.Healthy)
}

// TestHealthRecoveryAfterCircuitCloses tests health returns to OK after recovery.
func TestHealthRecoveryAfterCircuitCloses(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	circuits := NewTestCircuitsWithConfig(logger, circuitbreaker.Config{
		Name:             "postgres",
		MaxFailures:      2,
		FailureWindow:    5 * time.Second,
		ResetTimeout:     500 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 1,
	})

	handler := http.HandlerFunc(circuits.HandleCircuitHealth)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Trip the circuit
	for i := 0; i < 2; i++ {
		circuits.Database.Execute(func() error {
			return errors.New("failure")
		})
	}

	// Verify degraded
	resp1, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("get health while degraded: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected 503 when degraded, got %d", resp1.StatusCode)
	}

	// Wait for recovery and execute successful operation
	time.Sleep(600 * time.Millisecond)
	circuits.Database.Execute(func() error {
		return nil
	})

	// Verify recovered
	resp2, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("get health after recovery: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 after recovery, got %d", resp2.StatusCode)
	}

	t.Log("Health correctly reflects circuit recovery")
}

// TestAllCircuitsStatus tests getting status of all circuits.
func TestAllCircuitsStatus(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	r := resilience.New(resilience.Config{
		ServiceName: "test-service",
		Logger:      logger,
	})

	// Register multiple dependencies
	r.RegisterDependency("postgres", resilience.DatabaseDep)
	r.RegisterDependency("redis", resilience.CacheDep)
	r.RegisterDependency("kafka", resilience.MessageQueueDep, resilience.WithCritical(true))

	// Get status
	status := r.GetStatus()

	if len(status) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(status))
	}

	// Verify each dependency
	for name, depStatus := range status {
		if depStatus.State != "closed" {
			t.Errorf("Expected %s to be closed, got %s", name, depStatus.State)
		}
		t.Logf("Dependency %s: type=%s, state=%s, critical=%v",
			name, depStatus.Type, depStatus.State, depStatus.Critical)
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// TestCircuits is a simplified version of Circuits for testing.
type TestCircuits struct {
	Database *circuitbreaker.CircuitBreaker
	Redis    *circuitbreaker.CircuitBreaker
	Kafka    *circuitbreaker.CircuitBreaker
	logger   *zap.Logger
}

// NewTestCircuits creates a TestCircuits instance with default configs.
func NewTestCircuits(logger *zap.Logger) *TestCircuits {
	dbConfig := circuitbreaker.DatabaseCircuitConfig("postgres")
	redisConfig := circuitbreaker.RedisCircuitConfig("redis")
	kafkaConfig := circuitbreaker.KafkaCircuitConfig("kafka")

	return &TestCircuits{
		Database: circuitbreaker.New(dbConfig),
		Redis:    circuitbreaker.New(redisConfig),
		Kafka:    circuitbreaker.New(kafkaConfig),
		logger:   logger,
	}
}

// NewTestCircuitsWithConfig creates TestCircuits with custom database config.
func NewTestCircuitsWithConfig(logger *zap.Logger, dbConfig circuitbreaker.Config) *TestCircuits {
	redisConfig := circuitbreaker.RedisCircuitConfig("redis")
	kafkaConfig := circuitbreaker.KafkaCircuitConfig("kafka")

	return &TestCircuits{
		Database: circuitbreaker.New(dbConfig),
		Redis:    circuitbreaker.New(redisConfig),
		Kafka:    circuitbreaker.New(kafkaConfig),
		logger:   logger,
	}
}

// AllCircuits returns all circuit breakers.
func (c *TestCircuits) AllCircuits() []*circuitbreaker.CircuitBreaker {
	return []*circuitbreaker.CircuitBreaker{
		c.Database,
		c.Redis,
		c.Kafka,
	}
}

// IsHealthy returns true if critical circuits (Database, Kafka) are not open.
func (c *TestCircuits) IsHealthy() bool {
	critical := []*circuitbreaker.CircuitBreaker{c.Database, c.Kafka}
	for _, cb := range critical {
		if cb.State() == circuitbreaker.StateOpen {
			return false
		}
	}
	return true
}

// CircuitHealthResponse represents the health check response format.
type CircuitHealthResponse struct {
	Status   string                        `json:"status"`
	Healthy  bool                          `json:"healthy"`
	Circuits map[string]CircuitInfo        `json:"circuits"`
	Metrics  map[string]CircuitMetricsInfo `json:"metrics,omitempty"`
}

// CircuitInfo represents info about a single circuit.
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

// HandleCircuitHealth handles GET /health/circuits for testing.
func (c *TestCircuits) HandleCircuitHealth(w http.ResponseWriter, r *http.Request) {
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

	if !response.Healthy {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(response)
}

// =============================================================================
// Concurrent and Edge Case Tests
// =============================================================================

// TestConcurrentCircuitBreakerAccess tests thread safety of circuit breaker.
func TestConcurrentCircuitBreakerAccess(t *testing.T) {
	cfg := circuitbreaker.Config{
		Name:             "concurrent-test",
		MaxFailures:      100,
		FailureWindow:    10 * time.Second,
		ResetTimeout:     30 * time.Second,
		HalfOpenMaxCalls: 10,
		SuccessThreshold: 5,
	}
	cb := circuitbreaker.New(cfg)

	var wg sync.WaitGroup
	numGoroutines := 100
	opsPerGoroutine := 100

	// Run concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				if j%2 == 0 {
					cb.Execute(func() error {
						return nil
					})
				} else {
					cb.Execute(func() error {
						return errors.New("error")
					})
				}
				_ = cb.State()
				_ = cb.Metrics()
			}
		}(i)
	}

	wg.Wait()

	metrics := cb.Metrics()
	totalOps := int64(numGoroutines * opsPerGoroutine)

	if metrics.TotalRequests < totalOps {
		t.Errorf("Expected at least %d requests, got %d", totalOps, metrics.TotalRequests)
	}

	t.Logf("Concurrent test completed: %d requests, %d successes, %d failures",
		metrics.TotalRequests, metrics.TotalSuccesses, metrics.TotalFailures)
}

// TestCircuitBreakerTimeout tests the timeout functionality.
func TestCircuitBreakerTimeout(t *testing.T) {
	cfg := circuitbreaker.Config{
		Name:        "timeout-test",
		MaxFailures: 5,
		Timeout:     100 * time.Millisecond,
	}
	cb := circuitbreaker.New(cfg)

	// Execute function that takes longer than timeout
	err := cb.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	if !errors.Is(err, circuitbreaker.ErrCircuitTimeout) {
		t.Fatalf("Expected ErrCircuitTimeout, got %v", err)
	}

	metrics := cb.Metrics()
	if metrics.TotalTimeouts < 1 {
		t.Errorf("Expected at least 1 timeout, got %d", metrics.TotalTimeouts)
	}

	t.Log("Timeout correctly triggered")
}

// TestCircuitBreakerReset tests manual reset functionality.
func TestCircuitBreakerReset(t *testing.T) {
	cfg := circuitbreaker.Config{
		Name:          "reset-test",
		MaxFailures:   2,
		ResetTimeout:  1 * time.Hour, // Long timeout so we test manual reset
		FailureWindow: 10 * time.Second,
	}
	cb := circuitbreaker.New(cfg)

	// Trip the circuit
	for i := 0; i < cfg.MaxFailures; i++ {
		cb.Execute(func() error {
			return errors.New("failure")
		})
	}

	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Expected circuit to be open, got %s", cb.State())
	}

	// Reset manually
	cb.Reset()

	if cb.State() != circuitbreaker.StateClosed {
		t.Fatalf("Expected circuit to be closed after reset, got %s", cb.State())
	}

	// Verify we can execute again
	err := cb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Fatalf("Expected successful execution after reset, got %v", err)
	}

	t.Log("Manual reset works correctly")
}

// TestCustomFailureDetection tests custom IsFailure function.
func TestCustomFailureDetection(t *testing.T) {
	// Define a custom error that should not be counted as failure
	var ErrNotFound = errors.New("not found")

	cfg := circuitbreaker.Config{
		Name:        "custom-failure-test",
		MaxFailures: 2,
		IsFailure: func(err error) bool {
			// Don't count "not found" as a failure
			if errors.Is(err, ErrNotFound) {
				return false
			}
			return err != nil
		},
	}
	cb := circuitbreaker.New(cfg)

	// Execute with "not found" errors - should not trip circuit
	for i := 0; i < 10; i++ {
		cb.Execute(func() error {
			return ErrNotFound
		})
	}

	if cb.State() != circuitbreaker.StateClosed {
		t.Fatalf("Circuit should remain closed for ErrNotFound, got %s", cb.State())
	}

	// Real errors should trip the circuit
	for i := 0; i < cfg.MaxFailures; i++ {
		cb.Execute(func() error {
			return errors.New("real error")
		})
	}

	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Circuit should open for real errors, got %s", cb.State())
	}

	t.Log("Custom failure detection works correctly")
}

// TestFailureWindowExpiration tests that old failures expire.
func TestFailureWindowExpiration(t *testing.T) {
	cfg := circuitbreaker.Config{
		Name:          "window-test",
		MaxFailures:   3,
		FailureWindow: 500 * time.Millisecond, // Short window for testing
		ResetTimeout:  1 * time.Second,
	}
	cb := circuitbreaker.New(cfg)

	// Cause some failures
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("error")
		})
	}

	// Circuit should still be closed (2 < 3)
	if cb.State() != circuitbreaker.StateClosed {
		t.Fatalf("Circuit should be closed with %d failures, got %s", 2, cb.State())
	}

	// Wait for failure window to expire
	time.Sleep(600 * time.Millisecond)

	// Cause more failures - old ones should be expired
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return errors.New("error")
		})
	}

	// Circuit should still be closed (old failures expired)
	if cb.State() != circuitbreaker.StateClosed {
		t.Fatalf("Circuit should remain closed after window expiration, got %s", cb.State())
	}

	// Now cause 3 failures in quick succession
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return errors.New("error")
		})
	}

	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("Circuit should open after %d failures in window, got %s", 3, cb.State())
	}

	t.Log("Failure window expiration works correctly")
}
