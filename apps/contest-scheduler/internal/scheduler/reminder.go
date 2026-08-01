package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

// Reminder-specific Prometheus metrics
var (
	reminderChecksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contest_scheduler_reminder_checks_total",
		Help: "Total number of reminder check cycles",
	})

	remindersSentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "contest_scheduler_reminders_sent_total",
		Help: "Total number of reminder emails sent",
	}, []string{"status"})

	reminderContestsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contest_scheduler_reminder_contests_processed_total",
		Help: "Total number of contests processed for reminders",
	})

	reminderErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "contest_scheduler_reminder_errors_total",
		Help: "Total number of reminder processing errors",
	})

	reminderBatchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "contest_scheduler_reminder_batch_duration_seconds",
		Help:    "Duration of reminder batch processing",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
	})
)

// ReminderConfig holds configuration for the reminder service.
type ReminderConfig struct {
	// Intervals is a list of durations before contest start at which to send
	// reminders. Must be sorted descending (largest first).
	// Example: [24h, 1h, 15m] sends reminders at 24 hours, 1 hour, and 15 minutes before start.
	Intervals []time.Duration

	// EndIntervals is a list of durations before contest end at which to send
	// reminders. Must be sorted descending (largest first).
	// Example: [15m] sends a reminder 15 minutes before the contest ends.
	EndIntervals []time.Duration

	// CheckInterval is how often to check for contests needing reminders
	CheckInterval time.Duration

	// BatchSize is the number of emails to send concurrently
	BatchSize int

	// TradingBaseURL is the base URL for the trading platform
	TradingBaseURL string
}

// DefaultReminderConfig returns default reminder configuration.
func DefaultReminderConfig() ReminderConfig {
	return ReminderConfig{
		Intervals:      []time.Duration{24 * time.Hour, 1 * time.Hour, 15 * time.Minute},
		EndIntervals:   []time.Duration{24 * time.Hour, 1 * time.Hour, 15 * time.Minute},
		CheckInterval:  1 * time.Minute,
		BatchSize:      50,
		TradingBaseURL: "https://trade.tragge.com",
	}
}

// ReminderService handles sending contest starting reminders.
type ReminderService struct {
	pool          *db.Pool
	emailNotifier *notification.EmailNotifier
	config        ReminderConfig
	logger        *zap.Logger

	running atomic.Bool
	stop    chan struct{}
	wg      sync.WaitGroup
}

// NewReminderService creates a new reminder service.
func NewReminderService(
	pool *db.Pool,
	emailNotifier *notification.EmailNotifier,
	config ReminderConfig,
	logger *zap.Logger,
) *ReminderService {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Ensure at least one interval is configured
	if len(config.Intervals) == 0 {
		config.Intervals = DefaultReminderConfig().Intervals
	}

	return &ReminderService{
		pool:          pool,
		emailNotifier: emailNotifier,
		config:        config,
		logger:        logger,
		stop:          make(chan struct{}),
	}
}

// Start begins the reminder service loop.
func (rs *ReminderService) Start(ctx context.Context) {
	if rs.running.Swap(true) {
		return // Already running
	}

	rs.logger.Info("Starting reminder service",
		zap.Int("start_interval_count", len(rs.config.Intervals)),
		zap.Int("end_interval_count", len(rs.config.EndIntervals)),
		zap.Duration("check_interval", rs.config.CheckInterval))

	for _, interval := range rs.config.Intervals {
		rs.logger.Info("Start reminder interval configured",
			zap.String("tier", formatIntervalKey(interval)),
			zap.Duration("before_start", interval))
	}

	for _, interval := range rs.config.EndIntervals {
		rs.logger.Info("End reminder interval configured",
			zap.String("tier", formatEndIntervalKey(interval)),
			zap.Duration("before_end", interval))
	}

	rs.wg.Add(1)
	go rs.run(ctx)
}

// Stop stops the reminder service gracefully.
func (rs *ReminderService) Stop(ctx context.Context) {
	if !rs.running.Load() {
		return
	}

	rs.logger.Info("Stopping reminder service")

	close(rs.stop)

	done := make(chan struct{})
	infra.SafeGo(rs.logger, "reminder-stop-wait", func() {
		rs.wg.Wait()
		close(done)
	})

	select {
	case <-done:
		rs.logger.Info("Reminder service stopped gracefully")
	case <-ctx.Done():
		rs.logger.Warn("Reminder service stop timed out")
	}

	rs.running.Store(false)
}

// run is the main reminder service loop.
func (rs *ReminderService) run(ctx context.Context) {
	defer rs.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			rs.logger.Error("ReminderService.run panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	ticker := time.NewTicker(rs.config.CheckInterval)
	defer ticker.Stop()

	// Run first check immediately
	rs.checkAndSendReminders(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-rs.stop:
			return
		case <-ticker.C:
			rs.checkAndSendReminders(ctx)
		}
	}
}

// ContestForReminder represents a contest that needs a starting reminder.
type ContestForReminder struct {
	ID               string
	Name             string
	StartsAt         time.Time
	EndsAt           time.Time
	QtyTotal         int64
	ParticipantCount int
}

// checkAndSendReminders finds contests needing reminders for each configured interval.
func (rs *ReminderService) checkAndSendReminders(ctx context.Context) {
	startTime := time.Now()
	reminderChecksTotal.Inc()

	rs.logger.Debug("Running reminder check")

	// Check start reminders
	for _, interval := range rs.config.Intervals {
		tierKey := formatIntervalKey(interval)

		contests, err := rs.findContestsNeedingReminders(ctx, interval, tierKey)
		if err != nil {
			reminderErrorsTotal.Inc()
			rs.logger.Error("Failed to find contests needing start reminders",
				zap.String("tier", tierKey),
				zap.Error(err))
			continue
		}

		if len(contests) == 0 {
			rs.logger.Debug("No contests need start reminders for tier",
				zap.String("tier", tierKey))
			continue
		}

		rs.logger.Info("Found contests needing start reminders",
			zap.String("tier", tierKey),
			zap.Int("count", len(contests)))

		for _, contest := range contests {
			reminderContestsProcessed.Inc()
			rs.processContestReminder(ctx, contest, interval, tierKey)
		}
	}

	// Check end reminders
	for _, interval := range rs.config.EndIntervals {
		tierKey := formatEndIntervalKey(interval)

		contests, err := rs.findContestsNeedingEndReminders(ctx, interval, tierKey)
		if err != nil {
			reminderErrorsTotal.Inc()
			rs.logger.Error("Failed to find contests needing end reminders",
				zap.String("tier", tierKey),
				zap.Error(err))
			continue
		}

		if len(contests) == 0 {
			rs.logger.Debug("No contests need end reminders for tier",
				zap.String("tier", tierKey))
			continue
		}

		rs.logger.Info("Found contests needing end reminders",
			zap.String("tier", tierKey),
			zap.Int("count", len(contests)))

		for _, contest := range contests {
			reminderContestsProcessed.Inc()
			rs.processContestEndReminder(ctx, contest, interval, tierKey)
		}
	}

	duration := time.Since(startTime)
	reminderBatchDuration.Observe(duration.Seconds())
}

// findContestsNeedingReminders finds contests that:
// 1. Start within the reminder interval window (e.g., within next 25 hours for the 24h tier)
// 2. Haven't had a reminder sent for this specific tier yet
// 3. Are in an appropriate status (scheduled, registration_open, registration_closed)
func (rs *ReminderService) findContestsNeedingReminders(ctx context.Context, interval time.Duration, tierKey string) ([]ContestForReminder, error) {
	now := time.Now()
	// Add a 5-minute buffer to ensure we don't miss any contests near the boundary
	windowEnd := now.Add(interval + 5*time.Minute)

	// Find contests starting within the window that don't have a record
	// in contest_reminders_sent for this tier.
	// Use Primary() instead of Replica() because this query checks contest_reminders_sent
	// for deduplication. Writes go to primary via markReminderSent(), and replication lag
	// could cause the replica to miss recently inserted rows — leading to duplicate reminders.
	rows, err := rs.pool.Primary().QueryContext(ctx, `
		SELECT c.id, c.name, c.starts_at, c.ends_at, c.qty_total, c.current_participants
		FROM contests c
		WHERE c.status IN ('scheduled', 'registration_open', 'registration_closed')
		  AND c.starts_at > $1
		  AND c.starts_at <= $2
		  AND NOT EXISTS (
		      SELECT 1 FROM contest_reminders_sent crs
		      WHERE crs.contest_id = c.id AND crs.reminder_type = $3
		  )
		ORDER BY c.starts_at ASC
	`, now, windowEnd, tierKey)

	if err != nil {
		return nil, fmt.Errorf("failed to query contests for reminders (tier=%s): %w", tierKey, err)
	}
	defer rows.Close()

	var contests []ContestForReminder
	for rows.Next() {
		var c ContestForReminder
		if err := rows.Scan(&c.ID, &c.Name, &c.StartsAt, &c.EndsAt, &c.QtyTotal, &c.ParticipantCount); err != nil {
			rs.logger.Error("Failed to scan contest row", zap.Error(err))
			continue
		}
		contests = append(contests, c)
	}

	return contests, rows.Err()
}

// Participant holds participant info for reminders.
type Participant struct {
	UserID string
	Email  string
}

// processContestReminder processes reminders for a single contest at a given tier.
func (rs *ReminderService) processContestReminder(ctx context.Context, contest ContestForReminder, interval time.Duration, tierKey string) {
	rs.logger.Info("Processing reminder for contest",
		zap.String("contest_id", contest.ID),
		zap.String("contest_name", contest.Name),
		zap.String("tier", tierKey),
		zap.Time("starts_at", contest.StartsAt))

	// Get all participants with their emails
	participants, err := rs.getContestParticipants(ctx, contest.ID)
	if err != nil {
		reminderErrorsTotal.Inc()
		rs.logger.Error("Failed to get contest participants",
			zap.String("contest_id", contest.ID),
			zap.Error(err))
		return
	}

	if len(participants) == 0 {
		rs.logger.Warn("No participants found for contest",
			zap.String("contest_id", contest.ID))
		// Still mark as sent to avoid repeated checks
		rs.markReminderSent(ctx, contest.ID, tierKey, 0)
		return
	}

	rs.logger.Info("Sending reminders to participants",
		zap.String("contest_id", contest.ID),
		zap.String("tier", tierKey),
		zap.Int("participant_count", len(participants)))

	// Get contest symbols
	symbols, err := rs.getContestSymbols(ctx, contest.ID)
	if err != nil {
		rs.logger.Warn("Failed to get contest symbols",
			zap.String("contest_id", contest.ID),
			zap.Error(err))
		// Continue without symbols
	}

	// Prepare email data
	emailData := rs.buildEmailData(contest, symbols)

	// Send emails in batches
	rs.sendEmailsInBatches(ctx, contest.ID, tierKey, participants, emailData)

	// Create in-app notifications
	rs.createInAppNotifications(ctx, contest, participants)

	// Mark reminder as sent in the tracking table
	if err := rs.markReminderSent(ctx, contest.ID, tierKey, len(participants)); err != nil {
		reminderErrorsTotal.Inc()
		rs.logger.Error("Failed to mark reminder as sent",
			zap.String("contest_id", contest.ID),
			zap.String("tier", tierKey),
			zap.Error(err))
	}

	// Also update the legacy column for backward compatibility (for the smallest interval)
	if interval == rs.config.Intervals[len(rs.config.Intervals)-1] {
		rs.markLegacyReminderSent(ctx, contest.ID)
	}
}

// getContestParticipants retrieves all participants for a contest with their emails.
func (rs *ReminderService) getContestParticipants(ctx context.Context, contestID string) ([]Participant, error) {
	rows, err := rs.pool.Replica().QueryContext(ctx, `
		SELECT u.id, u.email
		FROM contest_participants cp
		JOIN users u ON u.id = cp.user_id
		WHERE cp.contest_id = $1
	`, contestID)

	if err != nil {
		return nil, fmt.Errorf("failed to query participants: %w", err)
	}
	defer rows.Close()

	var participants []Participant
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.UserID, &p.Email); err != nil {
			rs.logger.Error("Failed to scan participant row", zap.Error(err))
			continue
		}
		participants = append(participants, p)
	}

	return participants, rows.Err()
}

// getContestSymbols retrieves enabled symbols for a contest.
func (rs *ReminderService) getContestSymbols(ctx context.Context, contestID string) ([]string, error) {
	rows, err := rs.pool.Replica().QueryContext(ctx, `
		SELECT symbol
		FROM contest_symbols
		WHERE contest_id = $1 AND enabled = TRUE
		ORDER BY symbol
	`, contestID)

	if err != nil {
		return nil, fmt.Errorf("failed to query contest symbols: %w", err)
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			continue
		}
		symbols = append(symbols, symbol)
	}

	return symbols, rows.Err()
}

// buildEmailData creates the email data structure for the reminder.
func (rs *ReminderService) buildEmailData(contest ContestForReminder, symbols []string) notification.ContestStartingData {
	now := time.Now()
	timeUntilStart := contest.StartsAt.Sub(now)
	duration := contest.EndsAt.Sub(contest.StartsAt)

	// Format time until start
	var timeUntilStartStr string
	if timeUntilStart.Minutes() < 60 {
		timeUntilStartStr = fmt.Sprintf("%.0f minutes", timeUntilStart.Minutes())
	} else if timeUntilStart.Hours() < 24 {
		hours := int(timeUntilStart.Hours())
		mins := int(timeUntilStart.Minutes()) % 60
		if mins > 0 {
			timeUntilStartStr = fmt.Sprintf("%d hours %d minutes", hours, mins)
		} else {
			timeUntilStartStr = fmt.Sprintf("%d hours", hours)
		}
	} else {
		days := int(timeUntilStart.Hours() / 24)
		hours := int(timeUntilStart.Hours()) % 24
		if hours > 0 {
			timeUntilStartStr = fmt.Sprintf("%d days %d hours", days, hours)
		} else {
			timeUntilStartStr = fmt.Sprintf("%d days", days)
		}
	}

	// Format duration
	var durationStr string
	if duration.Hours() < 1 {
		durationStr = fmt.Sprintf("%.0f minutes", duration.Minutes())
	} else if duration.Hours() < 24 {
		hours := int(duration.Hours())
		mins := int(duration.Minutes()) % 60
		if mins > 0 {
			durationStr = fmt.Sprintf("%d hours %d minutes", hours, mins)
		} else {
			durationStr = fmt.Sprintf("%d hours", hours)
		}
	} else {
		days := int(duration.Hours() / 24)
		hours := int(duration.Hours()) % 24
		if hours > 0 {
			durationStr = fmt.Sprintf("%d days %d hours", days, hours)
		} else {
			durationStr = fmt.Sprintf("%d days", days)
		}
	}

	// Format starting balance
	startingBalance := fmt.Sprintf("$%d", contest.QtyTotal/100)

	return notification.ContestStartingData{
		ContestID:        contest.ID,
		ContestName:      contest.Name,
		StartTime:        contest.StartsAt.Format("January 2, 2006 at 3:04 PM MST"),
		EndTime:          contest.EndsAt.Format("January 2, 2006 at 3:04 PM MST"),
		Duration:         durationStr,
		TimeUntilStart:   timeUntilStartStr,
		StartingBalance:  startingBalance,
		ParticipantCount: contest.ParticipantCount,
		Symbols:          symbols,
		TradingURL:       fmt.Sprintf("%s/contest/%s", rs.config.TradingBaseURL, contest.ID),
	}
}

// maxEmailRetries is the maximum number of retry attempts for failed email batches.
const maxEmailRetries = 3

// sendBatchWithRetry attempts to send a batch of emails, retrying failed recipients
// with exponential backoff. It aggregates successful sends across all attempts and
// resets the circuit breaker if it is open before retrying.
func (rs *ReminderService) sendBatchWithRetry(
	ctx context.Context,
	emails []string,
	sendFn func(ctx context.Context, emails []string) *notification.BatchSendResult,
) *notification.BatchSendResult {
	aggregated := &notification.BatchSendResult{}
	remaining := emails

	for attempt := 0; attempt <= maxEmailRetries; attempt++ {
		result := sendFn(ctx, remaining)

		aggregated.Successful = append(aggregated.Successful, result.Successful...)

		if len(result.Failed) == 0 {
			return aggregated
		}

		if attempt == maxEmailRetries {
			// Final attempt — record remaining failures
			aggregated.Failed = result.Failed
			return aggregated
		}

		// Check circuit breaker state — if open, only reset on the first retry.
		// If it trips open again after a reset, stop retrying to respect the breaker.
		if rs.emailNotifier.CircuitBreakerState() == circuitbreaker.StateOpen {
			if attempt == 0 {
				rs.logger.Warn("Email circuit breaker is open, resetting for first retry",
					zap.Int("failed_count", len(result.Failed)))
				rs.emailNotifier.ResetCircuitBreaker()
			} else {
				rs.logger.Warn("Email circuit breaker is open again after reset, aborting retries",
					zap.Int("attempt", attempt+1),
					zap.Int("failed_count", len(result.Failed)))
				aggregated.Failed = result.Failed
				return aggregated
			}
		}

		// Collect failed recipients for retry
		remaining = make([]string, len(result.Failed))
		for i, f := range result.Failed {
			remaining[i] = f.Recipient
		}

		// Exponential backoff: 1s, 2s, 4s
		backoff := time.Duration(1<<uint(attempt)) * time.Second
		rs.logger.Info("Retrying failed email batch",
			zap.Int("attempt", attempt+1),
			zap.Int("failed_count", len(remaining)),
			zap.Duration("backoff", backoff))

		select {
		case <-ctx.Done():
			aggregated.Failed = result.Failed
			return aggregated
		case <-time.After(backoff):
		}
	}

	return aggregated
}

// sendEmailsInBatches sends reminder emails to participants in batches.
func (rs *ReminderService) sendEmailsInBatches(
	ctx context.Context,
	contestID string,
	tierKey string,
	participants []Participant,
	emailData notification.ContestStartingData,
) {
	if rs.emailNotifier == nil {
		rs.logger.Warn("Email notifier not configured, skipping email reminders",
			zap.String("contest_id", contestID))
		return
	}

	// Filter participants by email notification preferences
	pUserIDs := make([]string, len(participants))
	for i, p := range participants {
		pUserIDs[i] = p.UserID
	}
	emailEnabledMap, _ := prefs.IsEnabledBatch(ctx, rs.pool.Replica(), pUserIDs, inapp.NotifTypeContestStarting, "email")

	var filteredParticipants []Participant
	for _, p := range participants {
		if emailEnabledMap[p.UserID] {
			filteredParticipants = append(filteredParticipants, p)
		}
	}

	if len(filteredParticipants) == 0 {
		rs.logger.Debug("All participants disabled email for contest reminders",
			zap.String("contest_id", contestID))
		return
	}

	// Process in batches
	batchSize := rs.config.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	totalSent := 0
	totalFailed := 0

	for i := 0; i < len(filteredParticipants); i += batchSize {
		end := i + batchSize
		if end > len(filteredParticipants) {
			end = len(filteredParticipants)
		}

		batch := filteredParticipants[i:end]
		emails := make([]string, len(batch))
		for j, p := range batch {
			emails[j] = p.Email
		}

		result := rs.sendBatchWithRetry(ctx, emails, func(ctx context.Context, emails []string) *notification.BatchSendResult {
			return rs.emailNotifier.SendContestStartingReminderBatch(ctx, emails, emailData)
		})

		totalSent += len(result.Successful)
		totalFailed += len(result.Failed)

		// Log failures that persisted after retries
		for _, failure := range result.Failed {
			rs.logger.Error("Failed to send reminder email after retries",
				zap.String("contest_id", contestID),
				zap.String("tier", tierKey),
				zap.String("email", failure.Recipient),
				zap.Error(failure.Error))
		}

		// Small delay between batches to avoid rate limiting
		if end < len(participants) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	remindersSentTotal.WithLabelValues("success").Add(float64(totalSent))
	remindersSentTotal.WithLabelValues("failed").Add(float64(totalFailed))

	rs.logger.Info("Completed sending reminder emails",
		zap.String("contest_id", contestID),
		zap.String("tier", tierKey),
		zap.Int("sent", totalSent),
		zap.Int("failed", totalFailed))
}

// createInAppNotifications creates in-app notifications for participants.
func (rs *ReminderService) createInAppNotifications(
	ctx context.Context,
	contest ContestForReminder,
	participants []Participant,
) {
	if len(participants) == 0 {
		return
	}

	// Filter participants by in-app notification preferences
	userIDs := make([]string, len(participants))
	for i, p := range participants {
		userIDs[i] = p.UserID
	}
	enabledMap, _ := prefs.IsEnabledBatch(ctx, rs.pool.Replica(), userIDs, inapp.NotifTypeContestStarting, "in_app")

	// Insert notifications in batches using the inapp package
	batchSize := 100
	successCount := 0
	for i := 0; i < len(participants); i += batchSize {
		end := i + batchSize
		if end > len(participants) {
			end = len(participants)
		}

		batch := participants[i:end]

		// Use a transaction for each batch
		tx, err := rs.pool.Begin(ctx)
		if err != nil {
			rs.logger.Error("Failed to begin transaction for notifications",
				zap.String("contest_id", contest.ID),
				zap.Error(err))
			continue
		}

		for _, p := range batch {
			if !enabledMap[p.UserID] {
				continue
			}
			err := inapp.CreateContestStartingNotification(ctx, tx, p.UserID, contest.ID, contest.Name, contest.StartsAt)
			if err != nil {
				rs.logger.Warn("Failed to insert notification",
					zap.String("contest_id", contest.ID),
					zap.String("user_id", p.UserID),
					zap.Error(err))
			} else {
				successCount++
			}
		}

		if err := tx.Commit(); err != nil {
			rs.logger.Error("Failed to commit notification batch",
				zap.String("contest_id", contest.ID),
				zap.Error(err))
			tx.Rollback()
		}
	}

	rs.logger.Info("Created in-app notifications",
		zap.String("contest_id", contest.ID),
		zap.Int("count", successCount))
}

// markReminderSent records the reminder as sent in the contest_reminders_sent table.
func (rs *ReminderService) markReminderSent(ctx context.Context, contestID, tierKey string, recipientCount int) error {
	_, err := rs.pool.Primary().ExecContext(ctx, `
		INSERT INTO contest_reminders_sent (contest_id, reminder_type, sent_at, recipient_count)
		VALUES ($1, $2, NOW(), $3)
		ON CONFLICT (contest_id, reminder_type) DO NOTHING
	`, contestID, tierKey, recipientCount)

	if err != nil {
		return fmt.Errorf("failed to insert reminder tracking record (tier=%s): %w", tierKey, err)
	}

	rs.logger.Info("Marked reminder as sent",
		zap.String("contest_id", contestID),
		zap.String("tier", tierKey),
		zap.Int("recipient_count", recipientCount))

	return nil
}

// markLegacyReminderSent updates the legacy starting_reminder_sent_at column for backward compatibility.
func (rs *ReminderService) markLegacyReminderSent(ctx context.Context, contestID string) {
	_, err := rs.pool.Primary().ExecContext(ctx, `
		UPDATE contests
		SET starting_reminder_sent_at = NOW()
		WHERE id = $1 AND starting_reminder_sent_at IS NULL
	`, contestID)

	if err != nil {
		rs.logger.Warn("Failed to update legacy reminder column",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}
}

// findContestsNeedingEndReminders finds running contests whose ends_at is approaching.
func (rs *ReminderService) findContestsNeedingEndReminders(ctx context.Context, interval time.Duration, tierKey string) ([]ContestForReminder, error) {
	now := time.Now()
	// Add a 5-minute buffer to ensure we don't miss any contests near the boundary
	windowEnd := now.Add(interval + 5*time.Minute)

	// Find running contests ending within the window that don't have a record
	// in contest_reminders_sent for this end-reminder tier.
	// Use Primary() instead of Replica() because this query checks contest_reminders_sent
	// for deduplication. Writes go to primary via markReminderSent(), and replication lag
	// could cause the replica to miss recently inserted rows — leading to duplicate reminders.
	rows, err := rs.pool.Primary().QueryContext(ctx, `
		SELECT c.id, c.name, c.starts_at, c.ends_at, c.qty_total, c.current_participants
		FROM contests c
		WHERE c.status = 'running'
		  AND c.ends_at > $1
		  AND c.ends_at <= $2
		  AND NOT EXISTS (
		      SELECT 1 FROM contest_reminders_sent crs
		      WHERE crs.contest_id = c.id AND crs.reminder_type = $3
		  )
		ORDER BY c.ends_at ASC
	`, now, windowEnd, tierKey)

	if err != nil {
		return nil, fmt.Errorf("failed to query contests for end reminders (tier=%s): %w", tierKey, err)
	}
	defer rows.Close()

	var contests []ContestForReminder
	for rows.Next() {
		var c ContestForReminder
		if err := rows.Scan(&c.ID, &c.Name, &c.StartsAt, &c.EndsAt, &c.QtyTotal, &c.ParticipantCount); err != nil {
			rs.logger.Error("Failed to scan contest row for end reminder", zap.Error(err))
			continue
		}
		contests = append(contests, c)
	}

	return contests, rows.Err()
}

// processContestEndReminder processes end reminders for a single running contest.
func (rs *ReminderService) processContestEndReminder(ctx context.Context, contest ContestForReminder, interval time.Duration, tierKey string) {
	rs.logger.Info("Processing end reminder for contest",
		zap.String("contest_id", contest.ID),
		zap.String("contest_name", contest.Name),
		zap.String("tier", tierKey),
		zap.Time("ends_at", contest.EndsAt))

	// Get all participants with their emails
	participants, err := rs.getContestParticipants(ctx, contest.ID)
	if err != nil {
		reminderErrorsTotal.Inc()
		rs.logger.Error("Failed to get contest participants for end reminder",
			zap.String("contest_id", contest.ID),
			zap.Error(err))
		return
	}

	if len(participants) == 0 {
		rs.logger.Warn("No participants found for contest end reminder",
			zap.String("contest_id", contest.ID))
		// Still mark as sent to avoid repeated checks
		rs.markReminderSent(ctx, contest.ID, tierKey, 0)
		return
	}

	rs.logger.Info("Sending end reminders to participants",
		zap.String("contest_id", contest.ID),
		zap.String("tier", tierKey),
		zap.Int("participant_count", len(participants)))

	// Get contest symbols
	symbols, err := rs.getContestSymbols(ctx, contest.ID)
	if err != nil {
		rs.logger.Warn("Failed to get contest symbols for end reminder",
			zap.String("contest_id", contest.ID),
			zap.Error(err))
	}

	// Prepare email data
	emailData := rs.buildEndEmailData(contest, symbols)

	// Send emails in batches
	rs.sendEndEmailsInBatches(ctx, contest.ID, tierKey, participants, emailData)

	// Create in-app notifications
	rs.createEndInAppNotifications(ctx, contest, participants)

	// Mark reminder as sent in the tracking table
	if err := rs.markReminderSent(ctx, contest.ID, tierKey, len(participants)); err != nil {
		reminderErrorsTotal.Inc()
		rs.logger.Error("Failed to mark end reminder as sent",
			zap.String("contest_id", contest.ID),
			zap.String("tier", tierKey),
			zap.Error(err))
	}
}

// buildEndEmailData creates the email data structure for an end reminder.
func (rs *ReminderService) buildEndEmailData(contest ContestForReminder, symbols []string) notification.ContestEndingData {
	now := time.Now()
	timeUntilEnd := contest.EndsAt.Sub(now)
	duration := contest.EndsAt.Sub(contest.StartsAt)

	// Format time until end
	timeUntilEndStr := formatHumanDuration(timeUntilEnd)

	// Format duration
	durationStr := formatHumanDuration(duration)

	// Format starting balance
	startingBalance := fmt.Sprintf("$%d", contest.QtyTotal/100)

	return notification.ContestEndingData{
		ContestID:        contest.ID,
		ContestName:      contest.Name,
		EndTime:          contest.EndsAt.Format("January 2, 2006 at 3:04 PM MST"),
		TimeUntilEnd:     timeUntilEndStr,
		Duration:         durationStr,
		StartingBalance:  startingBalance,
		ParticipantCount: contest.ParticipantCount,
		Symbols:          symbols,
		TradingURL:       fmt.Sprintf("%s/contest/%s", rs.config.TradingBaseURL, contest.ID),
	}
}

// sendEndEmailsInBatches sends end reminder emails to participants in batches.
func (rs *ReminderService) sendEndEmailsInBatches(
	ctx context.Context,
	contestID string,
	tierKey string,
	participants []Participant,
	emailData notification.ContestEndingData,
) {
	if rs.emailNotifier == nil {
		rs.logger.Warn("Email notifier not configured, skipping end reminder emails",
			zap.String("contest_id", contestID))
		return
	}

	// Filter participants by email notification preferences
	eUserIDs := make([]string, len(participants))
	for i, p := range participants {
		eUserIDs[i] = p.UserID
	}
	emailEnabledMap, _ := prefs.IsEnabledBatch(ctx, rs.pool.Replica(), eUserIDs, inapp.NotifTypeContestEnding, "email")

	var filteredParticipants []Participant
	for _, p := range participants {
		if emailEnabledMap[p.UserID] {
			filteredParticipants = append(filteredParticipants, p)
		}
	}

	if len(filteredParticipants) == 0 {
		rs.logger.Debug("All participants disabled email for contest ending reminders",
			zap.String("contest_id", contestID))
		return
	}

	batchSize := rs.config.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	totalSent := 0
	totalFailed := 0

	for i := 0; i < len(filteredParticipants); i += batchSize {
		end := i + batchSize
		if end > len(filteredParticipants) {
			end = len(filteredParticipants)
		}

		batch := filteredParticipants[i:end]
		emails := make([]string, len(batch))
		for j, p := range batch {
			emails[j] = p.Email
		}

		result := rs.sendBatchWithRetry(ctx, emails, func(ctx context.Context, emails []string) *notification.BatchSendResult {
			return rs.emailNotifier.SendContestEndingReminderBatch(ctx, emails, emailData)
		})

		totalSent += len(result.Successful)
		totalFailed += len(result.Failed)

		for _, failure := range result.Failed {
			rs.logger.Error("Failed to send end reminder email after retries",
				zap.String("contest_id", contestID),
				zap.String("tier", tierKey),
				zap.String("email", failure.Recipient),
				zap.Error(failure.Error))
		}

		// Small delay between batches to avoid rate limiting
		if end < len(participants) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	remindersSentTotal.WithLabelValues("success").Add(float64(totalSent))
	remindersSentTotal.WithLabelValues("failed").Add(float64(totalFailed))

	rs.logger.Info("Completed sending end reminder emails",
		zap.String("contest_id", contestID),
		zap.String("tier", tierKey),
		zap.Int("sent", totalSent),
		zap.Int("failed", totalFailed))
}

// createEndInAppNotifications creates in-app notifications for contest ending.
func (rs *ReminderService) createEndInAppNotifications(
	ctx context.Context,
	contest ContestForReminder,
	participants []Participant,
) {
	if len(participants) == 0 {
		return
	}

	// Filter participants by in-app notification preferences
	userIDs := make([]string, len(participants))
	for i, p := range participants {
		userIDs[i] = p.UserID
	}
	enabledMap, _ := prefs.IsEnabledBatch(ctx, rs.pool.Replica(), userIDs, inapp.NotifTypeContestEnding, "in_app")

	batchSize := 100
	successCount := 0
	for i := 0; i < len(participants); i += batchSize {
		end := i + batchSize
		if end > len(participants) {
			end = len(participants)
		}

		batch := participants[i:end]

		tx, err := rs.pool.Begin(ctx)
		if err != nil {
			rs.logger.Error("Failed to begin transaction for end notifications",
				zap.String("contest_id", contest.ID),
				zap.Error(err))
			continue
		}

		for _, p := range batch {
			if !enabledMap[p.UserID] {
				continue
			}
			err := inapp.CreateContestEndingNotification(ctx, tx, p.UserID, contest.ID, contest.Name, contest.EndsAt)
			if err != nil {
				rs.logger.Warn("Failed to insert end notification",
					zap.String("contest_id", contest.ID),
					zap.String("user_id", p.UserID),
					zap.Error(err))
			} else {
				successCount++
			}
		}

		if err := tx.Commit(); err != nil {
			rs.logger.Error("Failed to commit end notification batch",
				zap.String("contest_id", contest.ID),
				zap.Error(err))
			tx.Rollback()
		}
	}

	rs.logger.Info("Created in-app end notifications",
		zap.String("contest_id", contest.ID),
		zap.Int("count", successCount))
}

// formatHumanDuration converts a duration to a human-readable string.
func formatHumanDuration(d time.Duration) string {
	if d.Minutes() < 60 {
		return fmt.Sprintf("%.0f minutes", d.Minutes())
	}
	if d.Hours() < 24 {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("%d hours %d minutes", hours, mins)
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%d days %d hours", days, hours)
	}
	return fmt.Sprintf("%d days", days)
}

// formatEndIntervalKey converts a duration to a key prefixed with "end_" for the tracking table.
// Examples: 15m -> "end_15m", 1h -> "end_1h"
func formatEndIntervalKey(d time.Duration) string {
	return "end_" + formatIntervalKey(d)
}

// IsRunning returns whether the reminder service is running.
func (rs *ReminderService) IsRunning() bool {
	return rs.running.Load()
}

// GetStats returns current statistics for the reminder service.
func (rs *ReminderService) GetStats() map[string]interface{} {
	intervals := make([]string, len(rs.config.Intervals))
	for i, d := range rs.config.Intervals {
		intervals[i] = formatIntervalKey(d)
	}
	endIntervals := make([]string, len(rs.config.EndIntervals))
	for i, d := range rs.config.EndIntervals {
		endIntervals[i] = formatEndIntervalKey(d)
	}
	return map[string]interface{}{
		"running":        rs.running.Load(),
		"intervals":      intervals,
		"end_intervals":  endIntervals,
		"check_interval": rs.config.CheckInterval.String(),
		"batch_size":     rs.config.BatchSize,
	}
}

// findContestByID retrieves a single contest by ID (for testing/manual triggers).
func (rs *ReminderService) findContestByID(ctx context.Context, contestID string) (*ContestForReminder, error) {
	var contest ContestForReminder
	err := rs.pool.Replica().QueryRowContext(ctx, `
		SELECT id, name, starts_at, ends_at, qty_total, current_participants
		FROM contests
		WHERE id = $1
	`, contestID).Scan(&contest.ID, &contest.Name, &contest.StartsAt, &contest.EndsAt, &contest.QtyTotal, &contest.ParticipantCount)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("contest not found: %s", contestID)
		}
		return nil, fmt.Errorf("failed to get contest: %w", err)
	}

	return &contest, nil
}

// TriggerReminderForContest manually triggers a reminder for a specific contest.
// This is useful for testing or re-sending reminders.
func (rs *ReminderService) TriggerReminderForContest(ctx context.Context, contestID string) error {
	contest, err := rs.findContestByID(ctx, contestID)
	if err != nil {
		return err
	}

	// Use the smallest (most imminent) interval tier
	tierKey := formatIntervalKey(rs.config.Intervals[len(rs.config.Intervals)-1])
	rs.processContestReminder(ctx, *contest, rs.config.Intervals[len(rs.config.Intervals)-1], tierKey)
	return nil
}

// formatIntervalKey converts a duration to a human-readable key for the tracking table.
// Examples: 24h0m0s -> "24h", 1h0m0s -> "1h", 15m0s -> "15m", 90s -> "90s"
func formatIntervalKey(d time.Duration) string {
	if d >= 24*time.Hour && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
