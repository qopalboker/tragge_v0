// Package circuitbreaker provides a production-ready circuit breaker implementation
// to prevent cascade failures when external dependencies fail.
//
// The circuit breaker has three states:
//   - Closed: Normal operation, requests pass through
//   - Open: Circuit tripped, requests fail fast with ErrCircuitOpen
//   - HalfOpen: Testing recovery, limited requests allowed
//
// Example usage:
//
//	cb := circuitbreaker.New(circuitbreaker.Config{
//	    Name:         "postgres",
//	    MaxFailures:  5,
//	    ResetTimeout: 30 * time.Second,
//	})
//
//	err := cb.Execute(func() error {
//	    return db.Ping()
//	})
//
//	if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
//	    // Handle circuit open - use fallback or return degraded response
//	}
package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open and requests are rejected.
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrCircuitTimeout is returned when the wrapped function exceeds the configured timeout.
	ErrCircuitTimeout = errors.New("circuit breaker timeout")
)

// State represents the current state of a circuit breaker.
type State int32

const (
	// StateClosed is the normal state where requests pass through.
	StateClosed State = iota
	// StateOpen is the tripped state where requests fail fast.
	StateOpen
	// StateHalfOpen is the recovery testing state where limited requests are allowed.
	StateHalfOpen
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds the configuration for a CircuitBreaker.
type Config struct {
	// Name identifies this circuit breaker in logs and metrics.
	Name string

	// MaxFailures is the number of failures before opening the circuit.
	// Default: 5
	MaxFailures int

	// FailureWindow is the time window for counting failures.
	// Failures older than this are not counted.
	// Default: 10s
	FailureWindow time.Duration

	// ResetTimeout is the time to wait before transitioning from Open to HalfOpen.
	// Default: 30s
	ResetTimeout time.Duration

	// HalfOpenMaxCalls is the maximum number of test calls allowed in HalfOpen state.
	// Default: 3
	HalfOpenMaxCalls int

	// SuccessThreshold is the number of consecutive successes needed to close the circuit
	// when in HalfOpen state.
	// Default: 2
	SuccessThreshold int

	// Timeout is the maximum duration for wrapped calls. 0 means no timeout.
	Timeout time.Duration

	// OnStateChange is called when the circuit breaker changes state.
	OnStateChange func(name string, from, to State)

	// IsFailure determines whether an error should be counted as a failure.
	// If nil, any non-nil error is considered a failure.
	IsFailure func(err error) bool
}

// setDefaults applies default values to the config.
func (c *Config) setDefaults() {
	if c.MaxFailures <= 0 {
		c.MaxFailures = 5
	}
	if c.FailureWindow <= 0 {
		c.FailureWindow = 10 * time.Second
	}
	if c.ResetTimeout <= 0 {
		c.ResetTimeout = 30 * time.Second
	}
	if c.HalfOpenMaxCalls <= 0 {
		c.HalfOpenMaxCalls = 3
	}
	if c.SuccessThreshold <= 0 {
		c.SuccessThreshold = 2
	}
	if c.IsFailure == nil {
		c.IsFailure = func(err error) bool {
			return err != nil
		}
	}
}

// Metrics holds operational metrics for a circuit breaker.
type Metrics struct {
	// TotalRequests is the total number of requests attempted.
	TotalRequests int64
	// TotalSuccesses is the total number of successful requests.
	TotalSuccesses int64
	// TotalFailures is the total number of failed requests.
	TotalFailures int64
	// TotalRejections is the total number of requests rejected due to open circuit.
	TotalRejections int64
	// TotalTimeouts is the total number of requests that timed out.
	TotalTimeouts int64
	// ConsecutiveSuccesses is the current count of consecutive successes.
	ConsecutiveSuccesses int64
	// ConsecutiveFailures is the current count of consecutive failures.
	ConsecutiveFailures int64
	// LastFailureTime is the time of the last failure.
	LastFailureTime time.Time
	// LastSuccessTime is the time of the last success.
	LastSuccessTime time.Time
	// StateChanges is the total number of state changes.
	StateChanges int64
}

// failureRecord tracks a failure event with its timestamp.
type failureRecord struct {
	timestamp time.Time
}

// CircuitBreaker implements the circuit breaker pattern for fault tolerance.
type CircuitBreaker struct {
	config Config

	// State management
	state           atomic.Int32
	lastStateChange time.Time

	// Failure tracking
	failures   []failureRecord
	failuresMu sync.Mutex

	// HalfOpen state tracking
	halfOpenCalls     atomic.Int32
	halfOpenSuccesses atomic.Int32

	// Metrics
	totalRequests   atomic.Int64
	totalSuccesses  atomic.Int64
	totalFailures   atomic.Int64
	totalRejections atomic.Int64
	totalTimeouts   atomic.Int64
	consecutiveSucc atomic.Int64
	consecutiveFail atomic.Int64
	stateChanges    atomic.Int64
	lastFailureTime atomic.Int64 // Unix nano
	lastSuccessTime atomic.Int64 // Unix nano

	// Mutex for state transitions
	mu sync.Mutex
}

// New creates a new CircuitBreaker with the given configuration.
func New(cfg Config) *CircuitBreaker {
	cfg.setDefaults()

	cb := &CircuitBreaker{
		config:          cfg,
		failures:        make([]failureRecord, 0, cfg.MaxFailures),
		lastStateChange: time.Now(),
	}
	cb.state.Store(int32(StateClosed))

	// Initialize Prometheus metrics for this circuit breaker
	initMetrics(cfg.Name)

	return cb
}

// Execute runs the given function through the circuit breaker.
// Returns ErrCircuitOpen if the circuit is open.
// Returns ErrCircuitTimeout if the function exceeds the configured timeout.
//
// When Timeout is configured, a goroutine is used to enforce the deadline since
// fn does not receive a context. If fn blocks beyond the timeout, the goroutine
// will remain alive until fn returns. For leak-free timeout handling, use
// ExecuteWithContext with a context-aware function instead.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if cb.config.Timeout > 0 {
		// Wrap non-context-aware fn in a goroutine so timeout can be enforced.
		return cb.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
			resultCh := make(chan error, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						resultCh <- fmt.Errorf("panic in circuit breaker: %v", r)
					}
				}()
				resultCh <- fn()
			}()
			select {
			case err := <-resultCh:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
	return cb.ExecuteWithContext(context.Background(), func(ctx context.Context) error {
		return fn()
	})
}

// ExecuteWithContext runs the given function with context through the circuit breaker.
//
// fn is executed directly (not in a separate goroutine), so there is no goroutine leak
// on timeout. fn receives execCtx which includes any configured timeout deadline and
// must honor context cancellation/deadline for timely return.
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, fn func(context.Context) error) error {
	start := time.Now()
	cb.totalRequests.Add(1)
	recordRequest(cb.config.Name)

	// Check if request is allowed
	if !cb.allowRequest() {
		cb.totalRejections.Add(1)
		recordRejection(cb.config.Name)
		return ErrCircuitOpen
	}

	// Apply timeout if configured
	var execCtx context.Context
	var cancel context.CancelFunc
	if cb.config.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, cb.config.Timeout)
	} else {
		execCtx = ctx
	}

	// Execute fn directly — no goroutine, no leak.
	// fn receives execCtx and must honor context for timeout to work.
	err := fn(execCtx)

	// Detect timeout: if context deadline was exceeded, record as timeout.
	if execCtx.Err() == context.DeadlineExceeded {
		cb.totalTimeouts.Add(1)
		recordTimeout(cb.config.Name)
		err = ErrCircuitTimeout
	}

	// Record the result
	duration := time.Since(start)
	if cb.config.IsFailure(err) {
		cb.recordFailure()
		recordCallDuration(cb.config.Name, "failure", duration)
	} else {
		cb.recordSuccess()
		recordCallDuration(cb.config.Name, "success", duration)
	}

	// Cancel timeout context on error to release resources immediately.
	// On success, don't cancel — the caller may hold resources (e.g. *sql.Rows)
	// that depend on the context remaining valid. The timeout will naturally
	// expire and release resources.
	if err != nil && cancel != nil {
		cancel()
	}

	return err
}

// allowRequest determines if a request should be allowed based on current state.
func (cb *CircuitBreaker) allowRequest() bool {
	state := State(cb.state.Load())

	switch state {
	case StateClosed:
		return true

	case StateOpen:
		// Check if reset timeout has elapsed
		cb.mu.Lock()
		defer cb.mu.Unlock()

		// Re-check state under lock — another goroutine may have already transitioned
		currentState := State(cb.state.Load())
		if currentState != StateOpen {
			if currentState == StateHalfOpen {
				calls := cb.halfOpenCalls.Add(1)
				return calls <= int32(cb.config.HalfOpenMaxCalls)
			}
			return currentState == StateClosed
		}

		if time.Since(cb.lastStateChange) >= cb.config.ResetTimeout {
			// Reset counters before transition so the new state is not visible
			// to other goroutines until counters are clean.
			// Start at 1 to count this request that triggers the transition.
			cb.halfOpenCalls.Store(1)
			cb.halfOpenSuccesses.Store(0)
			cb.transitionTo(StateHalfOpen)
			return true
		}
		return false

	case StateHalfOpen:
		// Allow limited requests in half-open state
		calls := cb.halfOpenCalls.Add(1)
		return calls <= int32(cb.config.HalfOpenMaxCalls)

	default:
		return false
	}
}

// recordSuccess records a successful request.
func (cb *CircuitBreaker) recordSuccess() {
	cb.totalSuccesses.Add(1)
	cb.consecutiveSucc.Add(1)
	cb.consecutiveFail.Store(0)
	cb.lastSuccessTime.Store(time.Now().UnixNano())
	recordSuccess(cb.config.Name)
	UpdateFailureRate(cb.config.Name, cb.totalSuccesses.Load(), cb.totalFailures.Load())

	state := State(cb.state.Load())
	if state == StateHalfOpen {
		successes := cb.halfOpenSuccesses.Add(1)
		if successes >= int32(cb.config.SuccessThreshold) {
			cb.mu.Lock()
			cb.transitionTo(StateClosed)
			cb.clearFailures()
			cb.mu.Unlock()
		}
	}
}

// recordFailure records a failed request.
func (cb *CircuitBreaker) recordFailure() {
	now := time.Now()
	cb.totalFailures.Add(1)
	cb.consecutiveFail.Add(1)
	cb.consecutiveSucc.Store(0)
	cb.lastFailureTime.Store(now.UnixNano())
	recordFailure(cb.config.Name)
	UpdateFailureRate(cb.config.Name, cb.totalSuccesses.Load(), cb.totalFailures.Load())

	state := State(cb.state.Load())

	switch state {
	case StateClosed:
		cb.failuresMu.Lock()
		// Add failure record
		cb.failures = append(cb.failures, failureRecord{timestamp: now})
		// Remove old failures outside the window
		windowStart := now.Add(-cb.config.FailureWindow)
		cb.pruneFailures(windowStart)
		failureCount := len(cb.failures)
		cb.failuresMu.Unlock()

		// Check if we should trip the circuit
		if failureCount >= cb.config.MaxFailures {
			cb.mu.Lock()
			cb.transitionTo(StateOpen)
			cb.mu.Unlock()
		}

	case StateHalfOpen:
		// Any failure in half-open state trips the circuit again
		cb.mu.Lock()
		cb.transitionTo(StateOpen)
		cb.mu.Unlock()
	}
}

// pruneFailures removes failures older than windowStart.
// Must be called with failuresMu held.
func (cb *CircuitBreaker) pruneFailures(windowStart time.Time) {
	i := 0
	for ; i < len(cb.failures); i++ {
		if cb.failures[i].timestamp.After(windowStart) {
			break
		}
	}
	if i > 0 {
		cb.failures = cb.failures[i:]
	}
}

// clearFailures removes all failure records.
// Must be called with mu held.
func (cb *CircuitBreaker) clearFailures() {
	cb.failuresMu.Lock()
	cb.failures = cb.failures[:0]
	cb.failuresMu.Unlock()
}

// transitionTo changes the circuit breaker state.
// Must be called with mu held.
func (cb *CircuitBreaker) transitionTo(newState State) {
	oldState := State(cb.state.Load())
	if oldState == newState {
		return
	}

	cb.state.Store(int32(newState))
	cb.lastStateChange = time.Now()
	cb.stateChanges.Add(1)
	recordStateChange(cb.config.Name, newState)

	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.config.Name, oldState, newState)
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() State {
	// Check if we should transition from Open to HalfOpen
	state := State(cb.state.Load())
	if state == StateOpen {
		cb.mu.Lock()
		// Re-check state under lock — another goroutine may have already transitioned
		state = State(cb.state.Load())
		if state == StateOpen && time.Since(cb.lastStateChange) >= cb.config.ResetTimeout {
			// Reset counters before transition. Unlike allowRequest(), State()
			// is only a query — no actual call is being made, so start at 0.
			cb.halfOpenCalls.Store(0)
			cb.halfOpenSuccesses.Store(0)
			cb.transitionTo(StateHalfOpen)
			state = StateHalfOpen
		} else {
			state = State(cb.state.Load())
		}
		cb.mu.Unlock()
	}
	return state
}

// Metrics returns the current metrics for the circuit breaker.
func (cb *CircuitBreaker) Metrics() Metrics {
	lastFail := cb.lastFailureTime.Load()
	lastSucc := cb.lastSuccessTime.Load()

	m := Metrics{
		TotalRequests:        cb.totalRequests.Load(),
		TotalSuccesses:       cb.totalSuccesses.Load(),
		TotalFailures:        cb.totalFailures.Load(),
		TotalRejections:      cb.totalRejections.Load(),
		TotalTimeouts:        cb.totalTimeouts.Load(),
		ConsecutiveSuccesses: cb.consecutiveSucc.Load(),
		ConsecutiveFailures:  cb.consecutiveFail.Load(),
		StateChanges:         cb.stateChanges.Load(),
	}

	if lastFail > 0 {
		m.LastFailureTime = time.Unix(0, lastFail)
	}
	if lastSucc > 0 {
		m.LastSuccessTime = time.Unix(0, lastSucc)
	}

	return m
}

// Reset forces the circuit breaker back to the Closed state.
// This should only be used for administrative purposes or testing.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transitionTo(StateClosed)
	cb.clearFailures()
	cb.halfOpenCalls.Store(0)
	cb.halfOpenSuccesses.Store(0)
	cb.consecutiveSucc.Store(0)
	cb.consecutiveFail.Store(0)
}

// Name returns the name of this circuit breaker.
func (cb *CircuitBreaker) Name() string {
	return cb.config.Name
}

// FailureCount returns the number of failures in the current window.
func (cb *CircuitBreaker) FailureCount() int {
	cb.failuresMu.Lock()
	defer cb.failuresMu.Unlock()

	windowStart := time.Now().Add(-cb.config.FailureWindow)
	cb.pruneFailures(windowStart)
	return len(cb.failures)
}
