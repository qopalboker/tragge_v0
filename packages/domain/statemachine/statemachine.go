// Package statemachine provides contest lifecycle state machine functionality.
// It implements Tralent's contest phases with validation and side effects.
package statemachine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/IBM/sarama"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"go.uber.org/zap"
)

// ContestStatus represents the status of a contest.
type ContestStatus string

const (
	StatusDraft              ContestStatus = "draft"
	StatusScheduled          ContestStatus = "scheduled"
	StatusRegistrationOpen   ContestStatus = "registration_open"
	StatusRegistrationClosed ContestStatus = "registration_closed"
	StatusRunning            ContestStatus = "running"
	StatusPaused             ContestStatus = "paused"
	StatusSettling           ContestStatus = "settling"
	StatusCompleted          ContestStatus = "completed"
	StatusCancelled          ContestStatus = "cancelled"
)

// String returns the string representation of the status.
func (s ContestStatus) String() string {
	return string(s)
}

// IsValid returns true if the status is a valid contest status.
func (s ContestStatus) IsValid() bool {
	switch s {
	case StatusDraft, StatusScheduled, StatusRegistrationOpen, StatusRegistrationClosed,
		StatusRunning, StatusPaused, StatusSettling, StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

// IsFinal returns true if the status is a final state (no transitions out).
func (s ContestStatus) IsFinal() bool {
	switch s {
	case StatusCompleted, StatusCancelled:
		return true
	default:
		return false
	}
}

// IsActive returns true if the contest is in an active trading state.
func (s ContestStatus) IsActive() bool {
	return s == StatusRunning
}

// AllowsRegistration returns true if registration is allowed in this state.
func (s ContestStatus) AllowsRegistration() bool {
	switch s {
	case StatusScheduled, StatusRegistrationOpen:
		return true
	default:
		return false
	}
}

// AllowsTrading returns true if trading is allowed in this state.
func (s ContestStatus) AllowsTrading() bool {
	return s == StatusRunning
}

// validTransitions defines the allowed state transitions.
// Key is the from state, value is a slice of allowed to states.
var validTransitions = map[ContestStatus][]ContestStatus{
	StatusDraft: {
		StatusScheduled, // Admin publishes the contest
		StatusCancelled, // Admin cancels before publishing
	},
	StatusScheduled: {
		StatusRegistrationOpen,   // Registration opens (can be immediate or timed)
		StatusRegistrationClosed, // Deadline passed OR contest full
		StatusCancelled,          // Admin cancels OR min participants not met
	},
	StatusRegistrationOpen: {
		StatusRegistrationClosed, // Deadline passed OR contest full
		StatusCancelled,          // Admin cancels
	},
	StatusRegistrationClosed: {
		StatusRunning,   // Start time reached
		StatusCancelled, // Admin cancels
	},
	StatusRunning: {
		StatusSettling, // End time reached
		StatusPaused,   // Admin pauses (freeze)
	},
	StatusPaused: {
		StatusRunning,  // Admin resumes
		StatusSettling, // Admin ends the contest
	},
	StatusSettling: {
		StatusCompleted, // Settlement done
	},
	// Final states - no transitions allowed
	StatusCompleted: {},
	StatusCancelled: {},
}

// TransitionError represents an error during state transition.
type TransitionError struct {
	ContestID  string
	FromStatus ContestStatus
	ToStatus   ContestStatus
	Reason     string
}

func (e *TransitionError) Error() string {
	return fmt.Sprintf("cannot transition contest %s from %s to %s: %s",
		e.ContestID, e.FromStatus, e.ToStatus, e.Reason)
}

// Common errors
var (
	ErrContestNotFound     = errors.New("contest not found")
	ErrInvalidTransition   = errors.New("invalid state transition")
	ErrMinParticipants     = errors.New("minimum participants not met")
	ErrMaxParticipants     = errors.New("maximum participants reached")
	ErrContestNotStarted   = errors.New("contest has not started yet")
	ErrContestAlreadyEnded = errors.New("contest has already ended")
	ErrRegistrationClosed  = errors.New("registration is closed")
	ErrContestInFinalState = errors.New("contest is in a final state")
)

// Contest represents a contest with its current state.
type Contest struct {
	ID                   string
	Name                 string
	Status               ContestStatus
	StartsAt             time.Time
	EndsAt               time.Time
	RegistrationDeadline *time.Time
	MinParticipants      int
	MaxParticipants      *int
	CurrentParticipants  int
	AutoStart            bool
	PublishedAt          *time.Time
	StartedAt            *time.Time
	EndedAt              *time.Time
	SettledAt            *time.Time
	CancelledAt          *time.Time
	CancellationReason   *string
	QtyTotal             int64
	EntryFeeCents        int
	IsFree               bool          // Free contests bypass min participant checks
	PausedAt             *time.Time    // When contest was paused (nil if not paused)
	TotalPausedDuration  time.Duration // Total accumulated pause duration
	CommissionRate       float64       // Commission rate as percentage (e.g., 20.00 = 20%). Use prize.MustCommissionPercentToFraction() to convert to fraction for prize calculations.
	RegistrationOpensAt  *time.Time   // When registration should auto-open (nil = manual)
}

// TransitionRequest contains the data needed to request a state transition.
type TransitionRequest struct {
	ContestID string
	ToStatus  ContestStatus
	Reason    string
	ActorID   *string // User ID who triggered the transition, nil for automatic
	Metadata  map[string]any
}

// TransitionResult contains the result of a state transition.
type TransitionResult struct {
	Contest    *Contest
	FromStatus ContestStatus
	ToStatus   ContestStatus
	Timestamp  time.Time
}

// SideEffectHandler is called after a successful transition to handle side effects.
type SideEffectHandler func(ctx context.Context, result *TransitionResult) error

// Config holds the configuration for the state machine.
type Config struct {
	// KafkaProducer for publishing contest state events
	KafkaProducer sarama.SyncProducer

	// ContestStateTopic is the Kafka topic for contest state events
	ContestStateTopic string

	// Logger for structured logging
	Logger *zap.Logger

	// SideEffectHandlers are called after successful transitions
	SideEffectHandlers map[ContestStatus]SideEffectHandler
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		ContestStateTopic:  "contests.v1",
		SideEffectHandlers: make(map[ContestStatus]SideEffectHandler),
	}
}

// StateMachine manages contest state transitions.
type StateMachine struct {
	pool   *db.Pool
	config *Config
	logger *zap.Logger
}

// New creates a new StateMachine instance.
func New(pool *db.Pool, config *Config) *StateMachine {
	if config == nil {
		config = DefaultConfig()
	}

	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	if config.KafkaProducer == nil {
		logger.Warn("StateMachine created with nil Kafka producer - contest state events will not be published to Kafka")
	}

	return &StateMachine{
		pool:   pool,
		config: config,
		logger: logger,
	}
}

// CanTransition checks if a transition from one state to another is valid.
func CanTransition(from, to ContestStatus) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}

	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// GetAllowedTransitions returns the list of allowed transitions from a given state.
func GetAllowedTransitions(from ContestStatus) []ContestStatus {
	if allowed, ok := validTransitions[from]; ok {
		result := make([]ContestStatus, len(allowed))
		copy(result, allowed)
		return result
	}
	return nil
}

// GetCurrentStatus retrieves the current status of a contest.
func (sm *StateMachine) GetCurrentStatus(ctx context.Context, contestID string) (ContestStatus, error) {
	var status string
	err := sm.pool.Replica().QueryRowContext(ctx,
		`SELECT status FROM contests WHERE id = $1`,
		contestID,
	).Scan(&status)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrContestNotFound
		}
		return "", fmt.Errorf("failed to get contest status: %w", err)
	}

	return ContestStatus(status), nil
}

// GetContest retrieves the full contest information.
func (sm *StateMachine) GetContest(ctx context.Context, contestID string) (*Contest, error) {
	var contest Contest
	var status string
	var registrationDeadline, publishedAt, startedAt, endedAt, settledAt, cancelledAt, pausedAt, registrationOpensAt sql.NullTime
	var maxParticipants sql.NullInt64
	var cancellationReason sql.NullString
	var totalPausedDuration sql.NullString

	err := sm.pool.Replica().QueryRowContext(ctx, `
		SELECT
			id, name, status, starts_at, ends_at,
			registration_deadline, min_participants, max_participants,
			current_participants, auto_start,
			published_at, started_at, ended_at, settled_at,
			cancelled_at, cancellation_reason,
			qty_total, entry_fee_cents, is_free,
			paused_at, COALESCE(total_paused_duration, '0 seconds')::text,
			commission_rate,
			registration_opens_at
		FROM contests
		WHERE id = $1
	`, contestID).Scan(
		&contest.ID, &contest.Name, &status, &contest.StartsAt, &contest.EndsAt,
		&registrationDeadline, &contest.MinParticipants, &maxParticipants,
		&contest.CurrentParticipants, &contest.AutoStart,
		&publishedAt, &startedAt, &endedAt, &settledAt,
		&cancelledAt, &cancellationReason,
		&contest.QtyTotal, &contest.EntryFeeCents, &contest.IsFree,
		&pausedAt, &totalPausedDuration,
		&contest.CommissionRate,
		&registrationOpensAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrContestNotFound
		}
		return nil, fmt.Errorf("failed to get contest: %w", err)
	}

	contest.Status = ContestStatus(status)

	if registrationDeadline.Valid {
		contest.RegistrationDeadline = &registrationDeadline.Time
	}
	if maxParticipants.Valid {
		v := int(maxParticipants.Int64)
		contest.MaxParticipants = &v
	}
	if publishedAt.Valid {
		contest.PublishedAt = &publishedAt.Time
	}
	if startedAt.Valid {
		contest.StartedAt = &startedAt.Time
	}
	if endedAt.Valid {
		contest.EndedAt = &endedAt.Time
	}
	if settledAt.Valid {
		contest.SettledAt = &settledAt.Time
	}
	if cancelledAt.Valid {
		contest.CancelledAt = &cancelledAt.Time
	}
	if cancellationReason.Valid {
		contest.CancellationReason = &cancellationReason.String
	}
	if pausedAt.Valid {
		contest.PausedAt = &pausedAt.Time
	}
	if totalPausedDuration.Valid {
		contest.TotalPausedDuration = parsePostgresInterval(totalPausedDuration.String)
	}
	if registrationOpensAt.Valid {
		contest.RegistrationOpensAt = &registrationOpensAt.Time
	}

	return &contest, nil
}

// Transition performs a state transition for a contest.
func (sm *StateMachine) Transition(ctx context.Context, req TransitionRequest) (*TransitionResult, error) {
	// Validate the request
	if req.ContestID == "" {
		return nil, errors.New("contest ID is required")
	}
	if !req.ToStatus.IsValid() {
		return nil, fmt.Errorf("invalid target status: %s", req.ToStatus)
	}

	// Begin transaction
	tx, err := sm.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock the contest row for update
	var contest Contest
	var status string
	var registrationDeadline, publishedAt, startedAt, endedAt, settledAt, cancelledAt, pausedAt, registrationOpensAt sql.NullTime
	var maxParticipants sql.NullInt64
	var cancellationReason sql.NullString
	var totalPausedDuration sql.NullString

	err = tx.QueryRowContext(ctx, `
		SELECT
			id, name, status, starts_at, ends_at,
			registration_deadline, min_participants, max_participants,
			current_participants, auto_start,
			published_at, started_at, ended_at, settled_at,
			cancelled_at, cancellation_reason,
			qty_total, entry_fee_cents, is_free,
			paused_at, COALESCE(total_paused_duration, '0 seconds')::text,
			commission_rate,
			registration_opens_at
		FROM contests
		WHERE id = $1
		FOR UPDATE
	`, req.ContestID).Scan(
		&contest.ID, &contest.Name, &status, &contest.StartsAt, &contest.EndsAt,
		&registrationDeadline, &contest.MinParticipants, &maxParticipants,
		&contest.CurrentParticipants, &contest.AutoStart,
		&publishedAt, &startedAt, &endedAt, &settledAt,
		&cancelledAt, &cancellationReason,
		&contest.QtyTotal, &contest.EntryFeeCents, &contest.IsFree,
		&pausedAt, &totalPausedDuration,
		&contest.CommissionRate,
		&registrationOpensAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrContestNotFound
		}
		return nil, fmt.Errorf("failed to lock contest: %w", err)
	}

	contest.Status = ContestStatus(status)
	fromStatus := contest.Status

	// Populate pause fields
	if pausedAt.Valid {
		contest.PausedAt = &pausedAt.Time
	}
	if totalPausedDuration.Valid {
		contest.TotalPausedDuration = parsePostgresInterval(totalPausedDuration.String)
	}
	if registrationOpensAt.Valid {
		contest.RegistrationOpensAt = &registrationOpensAt.Time
	}

	// Check if transition is valid
	if !CanTransition(fromStatus, req.ToStatus) {
		return nil, &TransitionError{
			ContestID:  req.ContestID,
			FromStatus: fromStatus,
			ToStatus:   req.ToStatus,
			Reason:     fmt.Sprintf("transition from %s to %s is not allowed", fromStatus, req.ToStatus),
		}
	}

	// Validate transition-specific conditions
	if err := sm.validateTransition(ctx, &contest, req.ToStatus); err != nil {
		return nil, err
	}

	// Build the update query
	now := time.Now()
	var updateQuery string
	var updateArgs []any

	switch req.ToStatus {
	case StatusScheduled:
		updateQuery = `
			UPDATE contests
			SET status = $1, published_at = $2
			WHERE id = $3`
		updateArgs = []any{req.ToStatus.String(), now, req.ContestID}

	case StatusRegistrationClosed:
		updateQuery = `
			UPDATE contests
			SET status = $1
			WHERE id = $2`
		updateArgs = []any{req.ToStatus.String(), req.ContestID}

	case StatusRunning:
		// Resume from pause: When transitioning from StatusPaused → StatusRunning:
		//
		// 1. Calculate how long the contest was paused:
		//    pauseDuration = now - contest.PausedAt
		//
		// 2. Extend the contest end time by the pause duration:
		//    newEndsAt = contest.EndsAt + pauseDuration
		//    This ensures participants get the full contest duration minus no playing time.
		//
		// 3. Accumulate total paused time:
		//    total_paused_duration += pauseDuration (via SQL addition)
		//    This correctly handles multiple pause/resume cycles because each cycle
		//    only adds its own duration, not the total.
		//
		// 4. Clear paused_at to NULL, ready for the next potential pause.
		//
		// Safety: The transition map prevents paused→paused, so double-pause is impossible.
		// Direct settlement from pause (paused→settling) is handled separately below.
		//
		// Check if resuming from paused state
		if fromStatus == StatusPaused && contest.PausedAt != nil {
			// Calculate pause duration
			pauseDuration := now.Sub(*contest.PausedAt)
			// New ends_at = current ends_at + pause_duration
			newEndsAt := contest.EndsAt.Add(pauseDuration)

			// Store pause metadata in request for history recording
			if req.Metadata == nil {
				req.Metadata = make(map[string]any)
			}
			req.Metadata["pause_duration_seconds"] = int64(pauseDuration.Seconds())
			req.Metadata["new_ends_at"] = newEndsAt.Format(time.RFC3339)
			req.Metadata["previous_ends_at"] = contest.EndsAt.Format(time.RFC3339)

			// Update contest: extend ends_at, add to total_paused_duration, clear paused_at
			updateQuery = `
				UPDATE contests
				SET status = $1,
					ends_at = $2,
					total_paused_duration = total_paused_duration + $3,
					paused_at = NULL
				WHERE id = $4`
			updateArgs = []any{req.ToStatus.String(), newEndsAt, pauseDuration.String(), req.ContestID}

			// Update the contest struct for the result
			contest.EndsAt = newEndsAt
			contest.TotalPausedDuration += pauseDuration
			contest.PausedAt = nil
		} else {
			// Normal start (not resume from pause)
			updateQuery = `
				UPDATE contests
				SET status = $1, started_at = $2
				WHERE id = $3`
			updateArgs = []any{req.ToStatus.String(), now, req.ContestID}
		}

	case StatusSettling:
		// Check if coming from paused state (direct end without resume)
		if fromStatus == StatusPaused && contest.PausedAt != nil {
			// Calculate final pause duration (but don't extend ends_at since contest is ending)
			pauseDuration := now.Sub(*contest.PausedAt)

			// Store pause metadata in request for history recording
			if req.Metadata == nil {
				req.Metadata = make(map[string]any)
			}
			req.Metadata["final_pause_duration_seconds"] = int64(pauseDuration.Seconds())
			req.Metadata["contest_ended_while_paused"] = true

			// Update contest: add to total_paused_duration, clear paused_at (don't extend ends_at)
			updateQuery = `
				UPDATE contests
				SET status = $1,
					ended_at = $2,
					total_paused_duration = total_paused_duration + $3,
					paused_at = NULL
				WHERE id = $4`
			updateArgs = []any{req.ToStatus.String(), now, pauseDuration.String(), req.ContestID}

			// Update the contest struct for the result
			contest.TotalPausedDuration += pauseDuration
			contest.PausedAt = nil
		} else {
			// Normal end (from running state)
			updateQuery = `
				UPDATE contests
				SET status = $1, ended_at = $2
				WHERE id = $3`
			updateArgs = []any{req.ToStatus.String(), now, req.ContestID}
		}

	case StatusCompleted:
		updateQuery = `
			UPDATE contests
			SET status = $1, settled_at = $2
			WHERE id = $3`
		updateArgs = []any{req.ToStatus.String(), now, req.ContestID}

	case StatusCancelled:
		updateQuery = `
			UPDATE contests
			SET status = $1, cancelled_at = $2, cancellation_reason = $3
			WHERE id = $4`
		updateArgs = []any{req.ToStatus.String(), now, req.Reason, req.ContestID}

	case StatusPaused:
		// Pause: Records paused_at = now. The contest status moves to "paused" and
		// trading should be suspended (handled by side effects). The ends_at is NOT
		// modified here — it will be extended when/if the contest resumes.
		//
		// Set paused_at to track when pause started
		updateQuery = `
			UPDATE contests
			SET status = $1, paused_at = $2
			WHERE id = $3`
		updateArgs = []any{req.ToStatus.String(), now, req.ContestID}

		// Update the contest struct for the result
		contest.PausedAt = &now

	default:
		updateQuery = `
			UPDATE contests
			SET status = $1
			WHERE id = $2`
		updateArgs = []any{req.ToStatus.String(), req.ContestID}
	}

	// Execute the update
	_, err = tx.ExecContext(ctx, updateQuery, updateArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to update contest status: %w", err)
	}

	// Record the transition in history
	metadata, _ := json.Marshal(req.Metadata)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO contest_status_history
			(contest_id, from_status, to_status, changed_by, reason, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`,
		req.ContestID,
		fromStatus.String(),
		req.ToStatus.String(),
		req.ActorID,
		req.Reason,
		metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to record status history: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	result := &TransitionResult{
		Contest:    &contest,
		FromStatus: fromStatus,
		ToStatus:   req.ToStatus,
		Timestamp:  now,
	}

	// Update the contest status in the result
	result.Contest.Status = req.ToStatus

	// Publish Kafka event (best-effort, don't fail the transition)
	if sm.config.KafkaProducer != nil {
		if err := sm.publishContestStateEvent(ctx, result); err != nil {
			sm.logger.Error("Failed to publish contest state event",
				zap.String("contest_id", req.ContestID),
				zap.Error(err))
		}
	}

	// Execute side effect handlers (best-effort)
	if handler, ok := sm.config.SideEffectHandlers[req.ToStatus]; ok {
		if err := handler(ctx, result); err != nil {
			sm.logger.Error("Side effect handler failed",
				zap.String("contest_id", req.ContestID),
				zap.String("to_status", req.ToStatus.String()),
				zap.Error(err))
		}
	}

	sm.logger.Info("Contest state transition completed",
		zap.String("contest_id", req.ContestID),
		zap.String("from_status", fromStatus.String()),
		zap.String("to_status", req.ToStatus.String()),
		zap.Any("actor_id", req.ActorID))

	return result, nil
}

// validateTransition performs validation specific to certain transitions.
func (sm *StateMachine) validateTransition(ctx context.Context, contest *Contest, toStatus ContestStatus) error {
	switch toStatus {
	case StatusRunning:
		// Free contests always start regardless of participant count
		if !contest.IsFree {
			// Use actual participant count from contest_participants table
			// because the denormalized current_participants column can be stale.
			var actualParticipants int
			err := sm.pool.Primary().QueryRowContext(ctx,
				`SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1`,
				contest.ID).Scan(&actualParticipants)
			if err != nil {
				// Fall back to denormalized value on query error
				actualParticipants = contest.CurrentParticipants
			}

			if actualParticipants < contest.MinParticipants {
				return &TransitionError{
					ContestID:  contest.ID,
					FromStatus: contest.Status,
					ToStatus:   toStatus,
					Reason: fmt.Sprintf("minimum participants not met: %d/%d",
						actualParticipants, contest.MinParticipants),
				}
			}
		}

	case StatusCancelled:
		// Cancellation is always allowed (from allowed states)
		// No additional validation needed

	case StatusCompleted:
		// Must be in settling state
		if contest.Status != StatusSettling {
			return &TransitionError{
				ContestID:  contest.ID,
				FromStatus: contest.Status,
				ToStatus:   toStatus,
				Reason:     "contest must be in settling state before completion",
			}
		}
	}

	return nil
}

// publishContestStateEvent publishes a contest state event to Kafka.
func (sm *StateMachine) publishContestStateEvent(ctx context.Context, result *TransitionResult) error {
	if sm.config.KafkaProducer == nil {
		sm.logger.Warn("Kafka producer is nil, skipping contest state event publish",
			zap.String("contest_id", result.Contest.ID),
			zap.String("to_status", result.ToStatus.String()))
		return nil
	}

	// Map status to ContestPhase
	var phase contracts.ContestPhase
	switch result.ToStatus {
	case StatusDraft, StatusScheduled, StatusRegistrationOpen, StatusRegistrationClosed:
		phase = contracts.ContestPhaseUpcoming
	case StatusRunning:
		phase = contracts.ContestPhaseLive
	case StatusPaused:
		phase = contracts.ContestPhaseFrozen
	case StatusSettling:
		phase = contracts.ContestPhaseFrozen
	case StatusCompleted, StatusCancelled:
		phase = contracts.ContestPhaseEnded
	default:
		phase = contracts.ContestPhaseUpcoming
	}

	event := contracts.ContestState{
		ContestID: result.Contest.ID,
		Phase:     phase,
		Status:    contracts.ContestStatus(result.ToStatus.String()),
		Ts:        result.Timestamp.UnixMilli(),
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal contest state event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: sm.config.ContestStateTopic,
		Key:   sarama.StringEncoder(result.Contest.ID),
		Value: sarama.ByteEncoder(eventJSON),
	}

	_, _, err = sm.config.KafkaProducer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to publish contest state event: %w", err)
	}

	return nil
}

// Publish is an alias for transitioning from draft to scheduled.
func (sm *StateMachine) Publish(ctx context.Context, contestID string, actorID *string) (*TransitionResult, error) {
	return sm.Transition(ctx, TransitionRequest{
		ContestID: contestID,
		ToStatus:  StatusScheduled,
		ActorID:   actorID,
		Reason:    "Contest published by admin",
	})
}

// CloseRegistration transitions a contest to registration_closed.
func (sm *StateMachine) CloseRegistration(ctx context.Context, contestID string, actorID *string, reason string) (*TransitionResult, error) {
	if reason == "" {
		reason = "Registration closed"
	}
	return sm.Transition(ctx, TransitionRequest{
		ContestID: contestID,
		ToStatus:  StatusRegistrationClosed,
		ActorID:   actorID,
		Reason:    reason,
	})
}

// Start transitions a contest to running.
func (sm *StateMachine) Start(ctx context.Context, contestID string, actorID *string) (*TransitionResult, error) {
	return sm.Transition(ctx, TransitionRequest{
		ContestID: contestID,
		ToStatus:  StatusRunning,
		ActorID:   actorID,
		Reason:    "Contest started",
	})
}

// Pause transitions a contest to paused (freeze).
func (sm *StateMachine) Pause(ctx context.Context, contestID string, actorID *string, reason string) (*TransitionResult, error) {
	if reason == "" {
		reason = "Contest paused by admin"
	}
	return sm.Transition(ctx, TransitionRequest{
		ContestID: contestID,
		ToStatus:  StatusPaused,
		ActorID:   actorID,
		Reason:    reason,
	})
}

// Resume transitions a paused contest back to running.
func (sm *StateMachine) Resume(ctx context.Context, contestID string, actorID *string) (*TransitionResult, error) {
	return sm.Transition(ctx, TransitionRequest{
		ContestID: contestID,
		ToStatus:  StatusRunning,
		ActorID:   actorID,
		Reason:    "Contest resumed by admin",
	})
}

// End transitions a contest to settling.
func (sm *StateMachine) End(ctx context.Context, contestID string, actorID *string) (*TransitionResult, error) {
	return sm.Transition(ctx, TransitionRequest{
		ContestID: contestID,
		ToStatus:  StatusSettling,
		ActorID:   actorID,
		Reason:    "Contest ended",
	})
}

// Complete transitions a contest to completed.
func (sm *StateMachine) Complete(ctx context.Context, contestID string) (*TransitionResult, error) {
	return sm.Transition(ctx, TransitionRequest{
		ContestID: contestID,
		ToStatus:  StatusCompleted,
		Reason:    "Settlement completed",
	})
}

// Cancel transitions a contest to cancelled.
func (sm *StateMachine) Cancel(ctx context.Context, contestID string, actorID *string, reason string) (*TransitionResult, error) {
	if reason == "" {
		reason = "Cancelled by admin"
	}
	return sm.Transition(ctx, TransitionRequest{
		ContestID: contestID,
		ToStatus:  StatusCancelled,
		ActorID:   actorID,
		Reason:    reason,
	})
}

// GetStatusHistory retrieves the status transition history for a contest.
func (sm *StateMachine) GetStatusHistory(ctx context.Context, contestID string) ([]StatusHistoryEntry, error) {
	rows, err := sm.pool.Replica().QueryContext(ctx, `
		SELECT
			id, contest_id, from_status, to_status,
			changed_by, reason, metadata, created_at
		FROM contest_status_history
		WHERE contest_id = $1
		ORDER BY created_at ASC
	`, contestID)
	if err != nil {
		return nil, fmt.Errorf("failed to query status history: %w", err)
	}
	defer rows.Close()

	var history []StatusHistoryEntry
	for rows.Next() {
		var entry StatusHistoryEntry
		var fromStatus, toStatus sql.NullString
		var changedBy sql.NullString
		var reason sql.NullString
		var metadata []byte

		err := rows.Scan(
			&entry.ID, &entry.ContestID, &fromStatus, &toStatus,
			&changedBy, &reason, &metadata, &entry.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan status history row: %w", err)
		}

		if fromStatus.Valid {
			s := ContestStatus(fromStatus.String)
			entry.FromStatus = &s
		}
		entry.ToStatus = ContestStatus(toStatus.String)
		if changedBy.Valid {
			entry.ChangedBy = &changedBy.String
		}
		if reason.Valid {
			entry.Reason = &reason.String
		}
		if len(metadata) > 0 {
			json.Unmarshal(metadata, &entry.Metadata)
		}

		history = append(history, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating status history: %w", err)
	}

	return history, nil
}

// StatusHistoryEntry represents a single status transition in the history.
type StatusHistoryEntry struct {
	ID         string
	ContestID  string
	FromStatus *ContestStatus
	ToStatus   ContestStatus
	ChangedBy  *string
	Reason     *string
	Metadata   map[string]any
	CreatedAt  time.Time
}

// FindContestsForAutoTransition finds contests that need automatic state transitions.
func (sm *StateMachine) FindContestsForAutoTransition(ctx context.Context) ([]AutoTransitionCandidate, error) {
	now := time.Now()
	var candidates []AutoTransitionCandidate

	// Find contests ready to start (registration closed + start time passed).
	// Only registration_closed can transition to running per validTransitions.
	// Use actual participant count from contest_participants table instead of
	// the denormalized current_participants column, which may be stale.
	rows, err := sm.pool.Primary().QueryContext(ctx, `
		SELECT c.id, c.status, c.starts_at, c.min_participants, c.is_free,
		       COUNT(cp.user_id) AS actual_participants
		FROM contests c
		LEFT JOIN contest_participants cp ON cp.contest_id = c.id
		WHERE c.status = 'registration_closed'
		  AND c.auto_start = TRUE
		  AND c.starts_at <= $1
		GROUP BY c.id, c.status, c.starts_at, c.min_participants, c.is_free
	`, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query contests for auto-start: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c AutoTransitionCandidate
		var status string
		err := rows.Scan(&c.ContestID, &status, &c.StartsAt, &c.MinParticipants, &c.IsFree, &c.CurrentParticipants)
		if err != nil {
			continue
		}
		c.CurrentStatus = ContestStatus(status)
		c.SuggestedStatus = StatusRunning
		c.Reason = "Auto-start triggered by schedule"
		candidates = append(candidates, c)
	}
	rows.Close()

	// Find contests ready to end (running + end time passed)
	rows, err = sm.pool.Primary().QueryContext(ctx, `
		SELECT id, status, ends_at, current_participants, min_participants
		FROM contests
		WHERE status = 'running'
		  AND ends_at <= $1
	`, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query contests for auto-end: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c AutoTransitionCandidate
		var status string
		err := rows.Scan(&c.ContestID, &status, &c.EndsAt, &c.CurrentParticipants, &c.MinParticipants)
		if err != nil {
			continue
		}
		c.CurrentStatus = ContestStatus(status)
		c.SuggestedStatus = StatusSettling
		c.Reason = "Auto-end triggered by schedule"
		candidates = append(candidates, c)
	}
	rows.Close()

	// Find contests with registration deadline passed.
	// Include starts_at and is_free so the scheduler can chain the running
	// transition immediately if start time has also passed.
	rows, err = sm.pool.Primary().QueryContext(ctx, `
		SELECT c.id, c.status, c.registration_deadline, c.starts_at, c.is_free,
		       COUNT(cp.user_id) AS actual_participants, c.min_participants
		FROM contests c
		LEFT JOIN contest_participants cp ON cp.contest_id = c.id
		WHERE c.status IN ('scheduled', 'registration_open')
		  AND c.registration_deadline IS NOT NULL
		  AND c.registration_deadline <= $1
		GROUP BY c.id, c.status, c.registration_deadline, c.starts_at, c.is_free, c.min_participants
	`, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query contests for registration close: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c AutoTransitionCandidate
		var status string
		var regDeadline sql.NullTime
		err := rows.Scan(&c.ContestID, &status, &regDeadline, &c.StartsAt, &c.IsFree, &c.CurrentParticipants, &c.MinParticipants)
		if err != nil {
			continue
		}
		c.CurrentStatus = ContestStatus(status)
		c.SuggestedStatus = StatusRegistrationClosed
		c.Reason = "Registration deadline passed"
		if regDeadline.Valid {
			c.RegistrationDeadline = &regDeadline.Time
		}
		candidates = append(candidates, c)
	}

	// Find contests ready to open registration (scheduled + registration_opens_at <= now)
	rows, err = sm.pool.Primary().QueryContext(ctx, `
		SELECT id, status, starts_at, current_participants, min_participants
		FROM contests
		WHERE status = 'scheduled'
		  AND registration_opens_at IS NOT NULL
		  AND registration_opens_at <= $1
	`, now)
	if err != nil {
		return nil, fmt.Errorf("failed to query contests for registration open: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var c AutoTransitionCandidate
		var status string
		err := rows.Scan(&c.ContestID, &status, &c.StartsAt, &c.CurrentParticipants, &c.MinParticipants)
		if err != nil {
			continue
		}
		c.CurrentStatus = ContestStatus(status)
		c.SuggestedStatus = StatusRegistrationOpen
		c.Reason = "Registration opened by schedule"
		candidates = append(candidates, c)
	}

	return candidates, nil
}

// AutoTransitionCandidate represents a contest that may need automatic transition.
type AutoTransitionCandidate struct {
	ContestID            string
	CurrentStatus        ContestStatus
	SuggestedStatus      ContestStatus
	StartsAt             time.Time
	EndsAt               time.Time
	RegistrationDeadline *time.Time
	CurrentParticipants  int
	MinParticipants      int
	IsFree               bool
	Reason               string
}

// parsePostgresInterval parses a PostgreSQL interval string to time.Duration.
// Supports formats like "00:10:00", "00:00:05.123456", "1 day 02:30:00",
// "0 seconds", "-00:05:00", etc.
func parsePostgresInterval(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" || s == "0 seconds" || s == "00:00:00" {
		return 0
	}

	// Handle negative intervals
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = strings.TrimPrefix(s, "-")
	}

	var total time.Duration

	// Extract day component: "N day(s) ..." or "N days ..."
	if idx := strings.Index(s, " day"); idx >= 0 {
		dayStr := s[:idx]
		days, err := strconv.Atoi(strings.TrimSpace(dayStr))
		if err == nil {
			total += time.Duration(days) * 24 * time.Hour
		}
		// Find the rest after "day" or "days"
		after := s[idx:]
		if spaceIdx := strings.Index(after[1:], " "); spaceIdx >= 0 {
			s = strings.TrimSpace(after[1+spaceIdx+1:])
		} else {
			s = ""
		}
	}

	// Try parsing HH:MM:SS or HH:MM:SS.ffffff
	if s != "" {
		if d, ok := parseHMSWithFraction(s); ok {
			total += d
		} else if strings.Contains(s, "second") {
			// Try simple "N second(s)" format
			var seconds int
			if _, err := fmt.Sscanf(s, "%d", &seconds); err == nil {
				total += time.Duration(seconds) * time.Second
			}
		} else if total == 0 {
			// Unrecognized format, return 0
			return 0
		}
	}

	if negative {
		total = -total
	}
	return total
}

// parseHMSWithFraction parses "HH:MM:SS" or "HH:MM:SS.ffffff" into a Duration.
func parseHMSWithFraction(s string) (time.Duration, bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return 0, false
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	mins, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false
	}
	// Seconds may contain fractional part (e.g., "05.123456")
	secs, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, false
	}

	d := time.Duration(hours)*time.Hour +
		time.Duration(mins)*time.Minute +
		time.Duration(secs*float64(time.Second))
	return d, true
}
