package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/domain/statemachine"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Prometheus metrics
var (
	checksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contest_scheduler_checks_total",
		Help: "Total number of scheduler check cycles",
	})

	transitionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "contest_scheduler_transitions_total",
		Help: "Total number of state transitions by type",
	}, []string{"from_status", "to_status"})

	transitionsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "contest_scheduler_transitions_failed_total",
		Help: "Total number of failed state transitions by type",
	}, []string{"from_status", "to_status"})

	transitionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "contest_scheduler_transition_duration_seconds",
		Help:    "Duration of state transitions",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 10),
	}, []string{"to_status"})

	locksAcquiredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contest_scheduler_locks_acquired_total",
		Help: "Total number of locks acquired",
	})

	locksFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contest_scheduler_locks_failed_total",
		Help: "Total number of lock acquisitions failed",
	})

	contestsProcessedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contest_scheduler_contests_processed_total",
		Help: "Total number of contests processed",
	})

	errorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contest_scheduler_errors_total",
		Help: "Total number of errors encountered",
	})

	lastCheckTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "contest_scheduler_last_check_timestamp_seconds",
		Help: "Timestamp of the last scheduler check",
	})

	candidatesFound = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "contest_scheduler_candidates_found",
		Help: "Number of candidates found in the last check",
	})

	activeTransitions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "contest_scheduler_active_transitions",
		Help: "Number of transitions currently in progress",
	})

	nextEventSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "contest_scheduler_next_event_seconds",
		Help: "Time in seconds until the next scheduled event",
	})

	checkIntervalSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "contest_scheduler_check_interval_seconds",
		Help: "Current adaptive check interval in seconds",
	})

	// Task 10.1: tragge_scheduler_* metrics
	traggeSchedulerErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tragge_scheduler_errors_total",
		Help: "Total scheduler errors by type",
	}, []string{"error_type"})

	traggeSchedulerTickDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "tragge_scheduler_tick_duration_seconds",
		Help:    "Duration of scheduler check cycles",
		Buckets: prometheus.ExponentialBuckets(0.01, 2, 12),
	})
)

// Config contains scheduler configuration.
type Config struct {
	CheckInterval     time.Duration // MaxCheckInterval for adaptive scheduling (default: 30s, max: 60s)
	MinCheckInterval  time.Duration // Minimum check interval for adaptive scheduling (default: 2s)
	StartBuffer       time.Duration
	SettlementDelay   time.Duration
	MaxConcurrent     int
	MaxRetries        int
	RetryBaseDelay    time.Duration
	RetryMaxDelay     time.Duration
	LockTTL           time.Duration
	LockRetryInterval time.Duration
	InstanceID        string
}

// DefaultConfig returns a default scheduler configuration.
func DefaultConfig() Config {
	return Config{
		CheckInterval:     30 * time.Second, // Acts as MaxCheckInterval
		MinCheckInterval:  2 * time.Second,  // Minimum adaptive check interval
		StartBuffer:       0,
		SettlementDelay:   0,
		MaxConcurrent:     10,
		MaxRetries:        3,
		RetryBaseDelay:    1 * time.Second,
		RetryMaxDelay:     30 * time.Second,
		LockTTL:           60 * time.Second,
		LockRetryInterval: 100 * time.Millisecond,
	}
}

// Scheduler handles automatic contest state transitions.
type Scheduler struct {
	config       Config
	pool         *db.Pool
	stateMachine *statemachine.StateMachine
	sideEffects  *statemachine.SideEffects
	lock         *DistributedLock
	logger       *zap.Logger

	// State
	running atomic.Bool
	stop    chan struct{}
	wg      sync.WaitGroup

	// Adaptive interval state
	currentInterval  time.Duration
	nextEventTime    time.Time
	nextEventContest string

	// Health stats
	mu                  sync.RWMutex
	lastCheck           time.Time
	lastError           error
	checksCount         int64
	transitionsCount    int64
	errorsCount         int64
	avgProcessingTimeMs int64
}

// New creates a new contest scheduler.
func New(
	pool *db.Pool,
	redis redis.UniversalClient,
	smConfig *statemachine.Config,
	cfg Config,
	logger *zap.Logger,
) *Scheduler {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create state machine
	sm := statemachine.New(pool, smConfig)

	// Create side effects handler
	sideEffects := statemachine.NewSideEffects(pool, smConfig.KafkaProducer, logger)
	sideEffects.SetWalletService(wallet.NewService(pool.Primary()))
	sideEffects.SetRedisClient(redis)

	// Create distributed lock
	lock := NewDistributedLock(redis, LockConfig{
		TTL:        cfg.LockTTL,
		InstanceID: cfg.InstanceID,
	}, logger)

	// Ensure MinCheckInterval has a sensible default
	if cfg.MinCheckInterval <= 0 {
		cfg.MinCheckInterval = 2 * time.Second
	}
	// Cap MaxCheckInterval (CheckInterval) at 60s
	if cfg.CheckInterval > 60*time.Second {
		cfg.CheckInterval = 60 * time.Second
	}

	return &Scheduler{
		config:          cfg,
		pool:            pool,
		stateMachine:    sm,
		sideEffects:     sideEffects,
		lock:            lock,
		logger:          logger,
		stop:            make(chan struct{}),
		currentInterval: cfg.CheckInterval, // Start with max interval
	}
}

// Start begins the scheduler loop.
func (s *Scheduler) Start(ctx context.Context) {
	if s.running.Swap(true) {
		return // Already running
	}

	s.logger.Info("Starting contest scheduler with adaptive intervals",
		zap.Duration("max_check_interval", s.config.CheckInterval),
		zap.Duration("min_check_interval", s.config.MinCheckInterval),
		zap.Int("max_concurrent", s.config.MaxConcurrent),
		zap.String("instance_id", s.config.InstanceID))

	s.wg.Add(1)
	go s.run(ctx)
}

// Stop stops the scheduler loop gracefully.
func (s *Scheduler) Stop(ctx context.Context) {
	if !s.running.Load() {
		return
	}

	s.logger.Info("Stopping contest scheduler")

	// Signal stop
	close(s.stop)

	// Wait for goroutine to finish
	done := make(chan struct{})
	infra.SafeGo(s.logger, "scheduler-stop-wait", func() {
		s.wg.Wait()
		close(done)
	})

	// Wait with timeout
	select {
	case <-done:
		s.logger.Info("Scheduler stopped gracefully")
	case <-ctx.Done():
		s.logger.Warn("Scheduler stop timed out")
	}

	// Release all locks held by this instance
	releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.lock.ReleaseAll(releaseCtx); err != nil {
		s.logger.Error("Failed to release locks during shutdown", zap.Error(err))
	}

	s.running.Store(false)
}

// run is the main scheduler loop with adaptive check intervals.
func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("scheduler run panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	// Run first check immediately
	s.checkAndTransition(ctx)

	// Calculate initial adaptive interval
	nextInterval := s.calculateAdaptiveInterval(ctx)
	s.updateCurrentInterval(nextInterval)

	timer := time.NewTimer(nextInterval)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-timer.C:
			s.checkAndTransition(ctx)

			// Calculate next adaptive interval after each check
			nextInterval = s.calculateAdaptiveInterval(ctx)
			s.updateCurrentInterval(nextInterval)

			// Reset timer with new interval
			timer.Reset(nextInterval)
		}
	}
}

// nextEventResult holds the result of querying for the next upcoming event.
type nextEventResult struct {
	TransitionTime time.Time
	ContestID      string
	EventType      string
}

// getNextEventTime queries the database for the soonest upcoming transition time.
func (s *Scheduler) getNextEventTime(ctx context.Context) (*nextEventResult, error) {
	query := `
		SELECT next_transition_at, contest_id, event_type FROM (
			SELECT starts_at AS next_transition_at, id AS contest_id, 'start' AS event_type
			FROM contests
			WHERE status IN ('scheduled', 'registration_open') AND auto_start = TRUE
			UNION ALL
			SELECT starts_at AS next_transition_at, id AS contest_id, 'start' AS event_type
			FROM contests
			WHERE status = 'registration_closed' AND auto_start = TRUE AND starts_at > NOW()
			UNION ALL
			SELECT ends_at AS next_transition_at, id AS contest_id, 'end' AS event_type
			FROM contests
			WHERE status = 'running'
			UNION ALL
			SELECT registration_deadline AS next_transition_at, id AS contest_id, 'registration_close' AS event_type
			FROM contests
			WHERE status IN ('scheduled', 'registration_open') AND registration_deadline IS NOT NULL
			UNION ALL
			SELECT registration_opens_at AS next_transition_at, id AS contest_id, 'registration_open' AS event_type
			FROM contests
			WHERE status = 'scheduled' AND registration_opens_at IS NOT NULL
		) upcoming
		WHERE next_transition_at > NOW()
		ORDER BY next_transition_at ASC
		LIMIT 1
	`

	var result nextEventResult
	err := s.pool.ReadOnly().QueryRowContext(ctx, query).Scan(
		&result.TransitionTime,
		&result.ContestID,
		&result.EventType,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil // No upcoming events
		}
		return nil, fmt.Errorf("failed to query next event time: %w", err)
	}

	return &result, nil
}

// calculateAdaptiveInterval determines the next check interval based on upcoming events.
// Formula: interval = max(min(time_until_next_event / 3, MaxCheckInterval), MinCheckInterval)
func (s *Scheduler) calculateAdaptiveInterval(ctx context.Context) time.Duration {
	maxInterval := s.config.CheckInterval
	minInterval := s.config.MinCheckInterval

	// Query for the next upcoming event
	result, err := s.getNextEventTime(ctx)
	if err != nil {
		s.logger.Warn("Failed to query next event time, using max interval",
			zap.Error(err))
		s.clearNextEvent()
		return maxInterval
	}

	// No upcoming events - use max interval
	if result == nil {
		s.logger.Debug("No upcoming events, using max interval",
			zap.Duration("interval", maxInterval))
		s.clearNextEvent()
		nextEventSeconds.Set(-1) // Indicate no upcoming event
		return maxInterval
	}

	// Calculate time until next event
	timeUntilEvent := time.Until(result.TransitionTime)
	if timeUntilEvent <= 0 {
		// Event is due now or past, check immediately
		s.setNextEvent(result)
		return minInterval
	}

	// Store next event info for health reporting
	s.setNextEvent(result)

	// Update Prometheus gauge
	nextEventSeconds.Set(timeUntilEvent.Seconds())

	// Calculate adaptive interval: time_until_event / 3
	// This ensures we check at least 3 times before the event
	adaptiveInterval := timeUntilEvent / 3

	// Clamp between min and max
	if adaptiveInterval < minInterval {
		adaptiveInterval = minInterval
	}
	if adaptiveInterval > maxInterval {
		adaptiveInterval = maxInterval
	}

	return adaptiveInterval
}

// updateCurrentInterval updates the current interval and logs/records metrics.
func (s *Scheduler) updateCurrentInterval(newInterval time.Duration) {
	s.mu.Lock()
	oldInterval := s.currentInterval
	s.currentInterval = newInterval
	nextEvent := s.nextEventTime
	contestID := s.nextEventContest
	s.mu.Unlock()

	// Update Prometheus gauge
	checkIntervalSeconds.Set(newInterval.Seconds())

	// Log interval changes at DEBUG level
	if oldInterval != newInterval {
		if !nextEvent.IsZero() {
			s.logger.Debug("adaptive interval changed",
				zap.Duration("interval", newInterval),
				zap.Duration("next_event", time.Until(nextEvent)),
				zap.String("contest_id", contestID))
		} else {
			s.logger.Debug("adaptive interval changed",
				zap.Duration("interval", newInterval),
				zap.String("reason", "no_upcoming_events"))
		}
	}
}

// setNextEvent stores the next event info for health reporting.
func (s *Scheduler) setNextEvent(result *nextEventResult) {
	s.mu.Lock()
	s.nextEventTime = result.TransitionTime
	s.nextEventContest = result.ContestID
	s.mu.Unlock()
}

// clearNextEvent clears the stored next event info.
func (s *Scheduler) clearNextEvent() {
	s.mu.Lock()
	s.nextEventTime = time.Time{}
	s.nextEventContest = ""
	s.mu.Unlock()
}

// checkAndTransition checks for contests needing transitions and processes them.
func (s *Scheduler) checkAndTransition(ctx context.Context) {
	startTime := time.Now()
	defer func() {
		traggeSchedulerTickDuration.Observe(time.Since(startTime).Seconds())
	}()

	checksTotal.Inc()
	s.mu.Lock()
	s.checksCount++
	s.lastCheck = startTime
	s.mu.Unlock()

	lastCheckTimestamp.Set(float64(startTime.Unix()))

	s.logger.Debug("Running scheduler check")

	// Find contests needing automatic transitions
	candidates, err := s.stateMachine.FindContestsForAutoTransition(ctx)
	if err != nil {
		s.recordError(err)
		s.logger.Error("Failed to find contests for auto-transition", zap.Error(err))
		return
	}

	candidatesFound.Set(float64(len(candidates)))

	if len(candidates) == 0 {
		s.logger.Debug("No contests need transition")
		return
	}

	s.logger.Info("Found contests for auto-transition",
		zap.Int("count", len(candidates)))

	// Process candidates with concurrency limit
	sem := make(chan struct{}, s.config.MaxConcurrent)
	var wg sync.WaitGroup

	for _, candidate := range candidates {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore

		c := candidate
		infra.SafeGo(s.logger, "scheduler-process-candidate", func() {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			activeTransitions.Inc()
			defer activeTransitions.Dec()

			if err := s.processCandidate(ctx, c); err != nil {
				s.logger.Error("Failed to process candidate",
					zap.String("contest_id", c.ContestID),
					zap.Error(err))
			}
		})
	}

	wg.Wait()

	// Record processing time
	elapsed := time.Since(startTime)
	s.mu.Lock()
	s.avgProcessingTimeMs = (s.avgProcessingTimeMs + elapsed.Milliseconds()) / 2
	s.mu.Unlock()
}

// processCandidate processes a single contest transition candidate.
func (s *Scheduler) processCandidate(ctx context.Context, candidate statemachine.AutoTransitionCandidate) error {
	contestsProcessedTotal.Inc()

	// Try to acquire lock
	acquired, err := s.lock.Acquire(ctx, candidate.ContestID)
	if err != nil {
		s.recordError(err)
		return fmt.Errorf("failed to acquire lock for %s: %w", candidate.ContestID, err)
	}

	if !acquired {
		locksFailedTotal.Inc()
		traggeSchedulerErrorsTotal.WithLabelValues("lock_failed").Inc()
		s.logger.Debug("Lock not acquired, skipping contest",
			zap.String("contest_id", candidate.ContestID))
		return nil
	}

	locksAcquiredTotal.Inc()

	// Release lock when done — use background context because ctx may already be cancelled
	defer func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		if err := s.lock.Release(releaseCtx, candidate.ContestID); err != nil {
			s.logger.Warn("Failed to release lock",
				zap.String("contest_id", candidate.ContestID),
				zap.Error(err))
		}
	}()

	// Process with retry logic
	return s.processCandidateWithRetry(ctx, candidate)
}

// processCandidateWithRetry processes a candidate with exponential backoff retry.
func (s *Scheduler) processCandidateWithRetry(ctx context.Context, candidate statemachine.AutoTransitionCandidate) error {
	var lastErr error

	for attempt := 0; attempt <= s.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff delay with jitter
			delay := s.calculateBackoff(attempt)
			s.logger.Debug("Retrying transition",
				zap.String("contest_id", candidate.ContestID),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}

			// Extend lock before retry
			if extended, err := s.lock.Extend(ctx, candidate.ContestID); err != nil || !extended {
				s.logger.Warn("Failed to extend lock, aborting retry",
					zap.String("contest_id", candidate.ContestID),
					zap.Error(err))
				return fmt.Errorf("failed to extend lock for %s: %w", candidate.ContestID, err)
			}
		}

		err := s.executeTransition(ctx, candidate)
		if err == nil {
			return nil // Success
		}

		lastErr = err
		s.logger.Warn("Transition attempt failed",
			zap.String("contest_id", candidate.ContestID),
			zap.Int("attempt", attempt+1),
			zap.Int("max_retries", s.config.MaxRetries+1),
			zap.Error(err))
	}

	// All retries exhausted
	s.recordError(lastErr)
	traggeSchedulerErrorsTotal.WithLabelValues("transition_failed").Inc()
	transitionsFailedTotal.WithLabelValues(
		candidate.CurrentStatus.String(),
		candidate.SuggestedStatus.String(),
	).Inc()

	return fmt.Errorf("transition failed for %s after %d retries: %w",
		candidate.ContestID, s.config.MaxRetries+1, lastErr)
}

// executeTransition executes a single transition attempt.
func (s *Scheduler) executeTransition(ctx context.Context, candidate statemachine.AutoTransitionCandidate) error {
	startTime := time.Now()

	// Handle special case: auto-start requires minimum participants check.
	// Free contests always start regardless of participant count.
	if candidate.SuggestedStatus == statemachine.StatusRunning && !candidate.IsFree {
		if candidate.CurrentParticipants < candidate.MinParticipants {
			s.logger.Warn("Contest cannot start due to insufficient participants",
				zap.String("contest_id", candidate.ContestID),
				zap.Int("current", candidate.CurrentParticipants),
				zap.Int("min", candidate.MinParticipants))

			// Auto-cancel the contest
			_, err := s.stateMachine.Cancel(ctx, candidate.ContestID, nil,
				fmt.Sprintf("Auto-cancelled: minimum participants not met (%d/%d)",
					candidate.CurrentParticipants, candidate.MinParticipants))

			if err == nil {
				transitionsTotal.WithLabelValues(
					candidate.CurrentStatus.String(),
					statemachine.StatusCancelled.String(),
				).Inc()
			}
			return err
		}
	}

	// Perform the transition
	result, err := s.stateMachine.Transition(ctx, statemachine.TransitionRequest{
		ContestID: candidate.ContestID,
		ToStatus:  candidate.SuggestedStatus,
		Reason:    candidate.Reason,
		ActorID:   nil, // Automatic transition
	})

	if err != nil {
		return fmt.Errorf("transition failed: %w", err)
	}

	// Record metrics
	duration := time.Since(startTime)
	transitionsTotal.WithLabelValues(
		result.FromStatus.String(),
		result.ToStatus.String(),
	).Inc()
	transitionDuration.WithLabelValues(result.ToStatus.String()).Observe(duration.Seconds())

	s.mu.Lock()
	s.transitionsCount++
	s.mu.Unlock()

	s.logger.Info("Auto-transition completed",
		zap.String("contest_id", candidate.ContestID),
		zap.String("from_status", result.FromStatus.String()),
		zap.String("to_status", result.ToStatus.String()),
		zap.Duration("duration", duration))

	// Execute side effects (best effort, don't fail the transition)
	if handler, ok := s.sideEffects.GetRegisteredHandlers()[result.ToStatus]; ok {
		if err := handler(ctx, result); err != nil {
			s.logger.Error("Side effect handler failed",
				zap.String("contest_id", candidate.ContestID),
				zap.String("to_status", result.ToStatus.String()),
				zap.Error(err))
		}
	}

	// If we just closed registration and starts_at has already passed,
	// immediately follow up with the running transition in the same cycle
	// instead of waiting for the next scheduler tick.
	if result.ToStatus == statemachine.StatusRegistrationClosed &&
		!candidate.StartsAt.IsZero() && !candidate.StartsAt.After(time.Now()) {

		s.logger.Info("Contest start time already passed, chaining running transition",
			zap.String("contest_id", candidate.ContestID))

		followUp := candidate
		followUp.CurrentStatus = statemachine.StatusRegistrationClosed
		followUp.SuggestedStatus = statemachine.StatusRunning
		followUp.Reason = "Auto-start triggered by schedule (chained from registration close)"

		if err := s.executeTransition(ctx, followUp); err != nil {
			s.logger.Error("Chained running transition failed",
				zap.String("contest_id", candidate.ContestID),
				zap.Error(err))
			return err
		}
	}

	return nil
}

// calculateBackoff calculates the backoff delay for a given attempt.
func (s *Scheduler) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: base * 2^attempt
	base := s.config.RetryBaseDelay
	max := s.config.RetryMaxDelay

	delay := base * time.Duration(1<<uint(attempt))
	if delay > max {
		delay = max
	}

	// Add jitter (0-25% of delay)
	jitter := time.Duration(rand.Int63n(int64(delay / 4)))
	return delay + jitter
}

// recordError records an error in the scheduler stats.
func (s *Scheduler) recordError(err error) {
	errorsTotal.Inc()
	traggeSchedulerErrorsTotal.WithLabelValues("scheduler").Inc()
	s.mu.Lock()
	s.errorsCount++
	s.lastError = err
	s.mu.Unlock()
}

// Health returns the scheduler's health status.
func (s *Scheduler) Health() any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastErrorStr string
	if s.lastError != nil {
		lastErrorStr = s.lastError.Error()
	}

	var nextEventTime *time.Time
	if !s.nextEventTime.IsZero() {
		nextEventTime = &s.nextEventTime
	}

	return &HealthStatus{
		Running:             s.running.Load(),
		InstanceID:          s.config.InstanceID,
		LastCheck:           s.lastCheck,
		CheckInterval:       s.config.CheckInterval,
		CurrentInterval:     s.currentInterval,
		MinCheckInterval:    s.config.MinCheckInterval,
		MaxCheckInterval:    s.config.CheckInterval,
		NextEventTime:       nextEventTime,
		NextEventContestID:  s.nextEventContest,
		TotalChecks:         s.checksCount,
		TotalTransitions:    s.transitionsCount,
		TotalErrors:         s.errorsCount,
		LastError:           lastErrorStr,
		AvgProcessingTimeMs: s.avgProcessingTimeMs,
	}
}

// HealthStatus contains scheduler health information.
type HealthStatus struct {
	Running             bool          `json:"running"`
	InstanceID          string        `json:"instance_id"`
	LastCheck           time.Time     `json:"last_check"`
	CheckInterval       time.Duration `json:"check_interval_ms"`
	CurrentInterval     time.Duration `json:"current_interval_ms"`
	MinCheckInterval    time.Duration `json:"min_check_interval_ms"`
	MaxCheckInterval    time.Duration `json:"max_check_interval_ms"`
	NextEventTime       *time.Time    `json:"next_event_time,omitempty"`
	NextEventContestID  string        `json:"next_event_contest_id,omitempty"`
	TotalChecks         int64         `json:"total_checks"`
	TotalTransitions    int64         `json:"total_transitions"`
	TotalErrors         int64         `json:"total_errors"`
	LastError           string        `json:"last_error,omitempty"`
	AvgProcessingTimeMs int64         `json:"avg_processing_time_ms"`
}

// IsHealthy returns true if the scheduler is running and has no recent errors.
func (s *Scheduler) IsHealthy() bool {
	if !s.running.Load() {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if last check was within 2x check interval
	if time.Since(s.lastCheck) > s.config.CheckInterval*2 {
		return false
	}

	return true
}
