package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/domain"
	"github.com/Parsaeffatravesh/tragge/packages/domain/statemachine"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Calendar-specific Prometheus metrics
var (
	calendarContestsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "calendar_contests_created_total",
		Help: "Total number of contests created from calendar entries",
	})

	calendarProcessingErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "calendar_processing_errors_total",
		Help: "Total number of errors during calendar processing",
	})

	calendarChecksTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "calendar_processor_checks_total",
		Help: "Total number of calendar processor check cycles",
	})

	calendarEntriesProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "calendar_processor_entries_processed_total",
		Help: "Total number of calendar entries processed",
	})

	calendarProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "calendar_processor_duration_seconds",
		Help:    "Duration of calendar processing cycles",
		Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
	})

	calendarLastCheckTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "calendar_processor_last_check_timestamp_seconds",
		Help: "Timestamp of the last calendar processor check",
	})

	calendarEntriesDue = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "calendar_processor_entries_due",
		Help: "Number of calendar entries due in the last check",
	})

	// Task 10.1: tragge_scheduler_* metrics
	traggeSchedulerTournamentsCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tragge_scheduler_tournaments_created_total",
		Help: "Total tournaments created by template and market type",
	}, []string{"template_type", "market_type"})

	traggeSchedulerDedupHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tragge_scheduler_dedup_hits_total",
		Help: "Total duplicate contest creations prevented",
	})

	traggeSchedulerActiveSchedules = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tragge_scheduler_active_schedules",
		Help: "Number of active tournament schedules",
	})
)

// CalendarConfig holds configuration for the calendar processor.
type CalendarConfig struct {
	// CheckInterval is how often to check for due calendar entries (default: 60s)
	CheckInterval time.Duration

	// LockTTL is the TTL for the distributed lock
	LockTTL time.Duration
}

// DefaultCalendarConfig returns default calendar processor configuration.
func DefaultCalendarConfig() CalendarConfig {
	return CalendarConfig{
		CheckInterval: 60 * time.Second,
		LockTTL:       60 * time.Second,
	}
}

// CalendarProcessor handles creating contests based on calendar/recurrence rules.
type CalendarProcessor struct {
	pool         *db.Pool
	redis        redis.UniversalClient
	stateMachine *statemachine.StateMachine
	config       CalendarConfig
	logger        *zap.Logger
	lock          *DistributedLock

	running atomic.Bool
	stop    chan struct{}
	wg      sync.WaitGroup

	// Stats
	mu               sync.RWMutex
	lastCheck        time.Time
	lastError        error
	contestsCreated  int64
	errorsCount      int64
	entriesProcessed int64
}

// CalendarEntry represents a tournament template with recurrence rule.
type CalendarEntry struct {
	ID               string
	Name             string
	Description      sql.NullString
	DurationMinutes  int
	IsFree           bool
	EntryFeeCents    int
	QtyTotal         int64
	SymbolsJSON      string
	PrizeDistJSON    sql.NullString
	MaxParticipants  sql.NullInt32
	AssetClass       string
	CommissionRate   float64
	MinParticipants  int
	AutoStart        bool
	RecurrenceRule   string
	NextOccurrenceAt time.Time
	TemplateKey      sql.NullString
	Type             sql.NullString
}

// NewCalendarProcessor creates a new calendar processor.
func NewCalendarProcessor(
	pool *db.Pool,
	redis redis.UniversalClient,
	sm *statemachine.StateMachine,
	config CalendarConfig,
	logger *zap.Logger,
) *CalendarProcessor {
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create distributed lock for calendar processing
	lock := NewDistributedLock(redis, LockConfig{
		TTL:        config.LockTTL,
		InstanceID: "calendar-processor",
	}, logger)

	return &CalendarProcessor{
		pool:         pool,
		redis:        redis,
		stateMachine: sm,
		config:       config,
		logger:       logger,
		lock:         lock,
		stop:         make(chan struct{}),
	}
}

// Start begins the calendar processor loop.
func (cp *CalendarProcessor) Start(ctx context.Context) {
	if cp.running.Swap(true) {
		return // Already running
	}

	cp.logger.Info("Starting calendar processor",
		zap.Duration("check_interval", cp.config.CheckInterval))

	cp.wg.Add(1)
	go cp.run(ctx)
}

// Stop stops the calendar processor gracefully.
func (cp *CalendarProcessor) Stop(ctx context.Context) {
	if !cp.running.Load() {
		return
	}

	cp.logger.Info("Stopping calendar processor")

	close(cp.stop)

	done := make(chan struct{})
	infra.SafeGo(cp.logger, "calendar-stop-wait", func() {
		cp.wg.Wait()
		close(done)
	})

	select {
	case <-done:
		cp.logger.Info("Calendar processor stopped gracefully")
	case <-ctx.Done():
		cp.logger.Warn("Calendar processor stop timed out")
	}

	cp.running.Store(false)
}

// run is the main calendar processor loop.
func (cp *CalendarProcessor) run(ctx context.Context) {
	defer cp.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			cp.logger.Error("CalendarProcessor.run panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	ticker := time.NewTicker(cp.config.CheckInterval)
	defer ticker.Stop()

	// Run first check immediately
	cp.processCalendarEvents(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-cp.stop:
			return
		case <-ticker.C:
			cp.processCalendarEvents(ctx)
		}
	}
}

// processCalendarEvents finds and processes due calendar entries.
func (cp *CalendarProcessor) processCalendarEvents(ctx context.Context) {
	startTime := time.Now()
	calendarChecksTotal.Inc()
	calendarLastCheckTimestamp.Set(float64(startTime.Unix()))

	cp.mu.Lock()
	cp.lastCheck = startTime
	cp.mu.Unlock()

	cp.logger.Debug("Running calendar processor check")

	// Try to acquire the global calendar-processor lock
	acquired, err := cp.lock.Acquire(ctx, "calendar-processor")
	if err != nil {
		cp.recordError(err)
		cp.logger.Error("Failed to acquire calendar-processor lock", zap.Error(err))
		return
	}

	if !acquired {
		cp.logger.Debug("Calendar-processor lock not acquired, another instance is processing")
		return
	}

	// Release lock when done
	defer func() {
		if err := cp.lock.Release(ctx, "calendar-processor"); err != nil {
			cp.logger.Warn("Failed to release calendar-processor lock", zap.Error(err))
		}
	}()

	// Find due calendar entries
	entries, err := cp.findDueCalendarEntries(ctx)
	if err != nil {
		cp.recordError(err)
		cp.logger.Error("Failed to find due calendar entries", zap.Error(err))
		return
	}

	calendarEntriesDue.Set(float64(len(entries)))

	// Update active schedules gauge
	var activeCount int64
	if countErr := cp.pool.Replica().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tournament_templates WHERE is_active = TRUE AND auto_create = TRUE AND recurrence_rule IS NOT NULL`,
	).Scan(&activeCount); countErr == nil {
		traggeSchedulerActiveSchedules.Set(float64(activeCount))
	}

	if len(entries) == 0 {
		cp.logger.Debug("No calendar entries due")
		return
	}

	cp.logger.Info("Found due calendar entries", zap.Int("count", len(entries)))

	// Process each entry
	for _, entry := range entries {
		calendarEntriesProcessed.Inc()
		cp.mu.Lock()
		cp.entriesProcessed++
		cp.mu.Unlock()

		if err := cp.processCalendarEntry(ctx, entry); err != nil {
			cp.recordError(err)
			cp.logger.Error("Failed to process calendar entry",
				zap.String("template_id", entry.ID),
				zap.String("template_name", entry.Name),
				zap.Error(err))
			// Continue processing other entries - don't skip permanently
		}
	}

	duration := time.Since(startTime)
	calendarProcessingDuration.Observe(duration.Seconds())
}

// findDueCalendarEntries queries for templates with due next_occurrence_at.
func (cp *CalendarProcessor) findDueCalendarEntries(ctx context.Context) ([]CalendarEntry, error) {
	now := time.Now().UTC()

	// Use Primary() instead of Replica() because this query checks next_occurrence_at
	// for scheduling. Writes go to primary via updateNextOccurrence(), and replication lag
	// could cause the replica to still see the old value — leading to duplicate contests.
	rows, err := cp.pool.Primary().QueryContext(ctx, `
		SELECT
			id, name, description, duration_minutes, is_free, entry_fee_cents,
			qty_total, symbols_json, prize_distribution_json, max_participants,
			asset_class, commission_rate, min_participants, auto_start,
			recurrence_rule, next_occurrence_at, template_key, type
		FROM tournament_templates
		WHERE is_active = TRUE
		  AND auto_create = TRUE
		  AND recurrence_rule IS NOT NULL
		  AND next_occurrence_at IS NOT NULL
		  AND next_occurrence_at <= $1
		ORDER BY next_occurrence_at ASC
	`, now)

	if err != nil {
		return nil, fmt.Errorf("failed to query due calendar entries: %w", err)
	}
	defer rows.Close()

	var entries []CalendarEntry
	for rows.Next() {
		var e CalendarEntry
		if err := rows.Scan(
			&e.ID, &e.Name, &e.Description, &e.DurationMinutes, &e.IsFree, &e.EntryFeeCents,
			&e.QtyTotal, &e.SymbolsJSON, &e.PrizeDistJSON, &e.MaxParticipants,
			&e.AssetClass, &e.CommissionRate, &e.MinParticipants, &e.AutoStart,
			&e.RecurrenceRule, &e.NextOccurrenceAt, &e.TemplateKey, &e.Type,
		); err != nil {
			cp.logger.Error("Failed to scan calendar entry row", zap.Error(err))
			continue
		}
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// EntryTier represents an active entry tier for a template.
type EntryTier struct {
	ID                      string
	EntryFee                int64
	Label                   string
	IsFree                  bool
	SortOrder               int
	QtyTotalOverride        *int64
	MaxParticipantsOverride *int32
	CommissionRateOverride  *float64
	HasPrizeOverride        bool
}

// processCalendarEntry creates contest(s) from the template and updates next_occurrence_at.
// If the template has active tiers, one contest is created per tier.
// Otherwise, falls back to legacy single-contest creation.
func (cp *CalendarProcessor) processCalendarEntry(ctx context.Context, entry CalendarEntry) error {
	cp.logger.Info("Processing calendar entry",
		zap.String("template_id", entry.ID),
		zap.String("template_name", entry.Name),
		zap.String("recurrence_rule", entry.RecurrenceRule),
		zap.Time("next_occurrence_at", entry.NextOccurrenceAt))

	startsAt := entry.NextOccurrenceAt
	endsAt := startsAt.Add(time.Duration(entry.DurationMinutes) * time.Minute)

	// Fetch active tiers for this template
	tiers, err := cp.fetchActiveTiers(ctx, entry.ID)
	if err != nil {
		cp.logger.Error("Failed to fetch tiers, falling back to legacy",
			zap.String("template_id", entry.ID), zap.Error(err))
		tiers = nil
	}

	if len(tiers) == 0 {
		// LEGACY: No tiers — create single contest as before
		return cp.processLegacyEntry(ctx, entry, startsAt, endsAt)
	}

	// MULTI-TIER: create one contest per active tier
	var createdCount int
	for _, tier := range tiers {
		contestID, createErr := cp.createContestFromTier(ctx, entry, tier, startsAt, endsAt)
		if createErr != nil {
			cp.logger.Error("Failed to create contest for tier",
				zap.String("tier_id", tier.ID),
				zap.Int64("entry_fee", tier.EntryFee),
				zap.Error(createErr))
			calendarProcessingErrorsTotal.Inc()
			continue // Don't block other tiers
		}
		if contestID != "" {
			createdCount++
			calendarContestsCreatedTotal.Inc()

			templateType := "standard"
			if entry.Type.Valid && entry.Type.String != "" {
				templateType = entry.Type.String
			}
			marketType := entry.AssetClass
			if marketType == "" {
				marketType = "unknown"
			}
			traggeSchedulerTournamentsCreated.WithLabelValues(templateType, marketType).Inc()

			cp.mu.Lock()
			cp.contestsCreated++
			cp.mu.Unlock()
		}
	}

	cp.logger.Info("Multi-tier creation complete",
		zap.String("template_id", entry.ID),
		zap.Int("tiers", len(tiers)),
		zap.Int("created", createdCount))

	// Update next_occurrence_at based on recurrence pattern
	if err := cp.updateNextOccurrence(ctx, entry); err != nil {
		cp.logger.Error("Failed to update next_occurrence_at",
			zap.String("template_id", entry.ID),
			zap.Error(err))
	}

	return nil
}

// processLegacyEntry handles the original single-contest creation path.
func (cp *CalendarProcessor) processLegacyEntry(ctx context.Context, entry CalendarEntry, startsAt, endsAt time.Time) error {
	// Dedup check
	var existingCount int
	err := cp.pool.Primary().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM contests
		WHERE template_id = $1
		  AND starts_at >= $2
		  AND starts_at < $3
	`, entry.ID, startsAt.Add(-1*time.Minute), startsAt.Add(1*time.Minute)).Scan(&existingCount)

	if err != nil {
		return fmt.Errorf("failed to check existing contests: %w", err)
	}

	if existingCount > 0 {
		traggeSchedulerDedupHits.Inc()
		cp.logger.Warn("Contest already exists for this calendar entry time slot",
			zap.String("template_id", entry.ID),
			zap.Time("starts_at", startsAt))
		return cp.updateNextOccurrence(ctx, entry)
	}

	contestID, err := cp.createContestFromTemplate(ctx, entry, startsAt, endsAt)
	if err != nil {
		return fmt.Errorf("failed to create contest: %w", err)
	}

	calendarContestsCreatedTotal.Inc()

	templateType := "standard"
	if entry.Type.Valid && entry.Type.String != "" {
		templateType = entry.Type.String
	}
	marketType := entry.AssetClass
	if marketType == "" {
		marketType = "unknown"
	}
	traggeSchedulerTournamentsCreated.WithLabelValues(templateType, marketType).Inc()

	cp.mu.Lock()
	cp.contestsCreated++
	cp.mu.Unlock()

	cp.logger.Info("Created contest from calendar entry (legacy)",
		zap.String("contest_id", contestID),
		zap.String("template_id", entry.ID),
		zap.String("template_name", entry.Name),
		zap.Time("starts_at", startsAt),
		zap.Time("ends_at", endsAt))

	if err := cp.updateNextOccurrence(ctx, entry); err != nil {
		cp.logger.Error("Failed to update next_occurrence_at",
			zap.String("template_id", entry.ID),
			zap.Error(err))
	}

	return nil
}

// fetchActiveTiers queries active entry tiers for a template.
func (cp *CalendarProcessor) fetchActiveTiers(ctx context.Context, templateID string) ([]EntryTier, error) {
	rows, err := cp.pool.Primary().QueryContext(ctx, `
		SELECT id, entry_fee, COALESCE(label, ''), is_free, sort_order,
		       qty_total_override, max_participants_override,
		       commission_rate_override, has_prize_override
		FROM template_entry_tiers
		WHERE template_id = $1 AND is_active = TRUE
		ORDER BY sort_order ASC, entry_fee ASC
	`, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tiers: %w", err)
	}
	defer rows.Close()

	var tiers []EntryTier
	for rows.Next() {
		var t EntryTier
		if err := rows.Scan(
			&t.ID, &t.EntryFee, &t.Label, &t.IsFree, &t.SortOrder,
			&t.QtyTotalOverride, &t.MaxParticipantsOverride,
			&t.CommissionRateOverride, &t.HasPrizeOverride,
		); err != nil {
			cp.logger.Error("Failed to scan tier row", zap.Error(err))
			continue
		}
		tiers = append(tiers, t)
	}

	return tiers, rows.Err()
}

// createContestFromTier creates a contest for a specific entry tier.
func (cp *CalendarProcessor) createContestFromTier(
	ctx context.Context,
	entry CalendarEntry,
	tier EntryTier,
	startsAt, endsAt time.Time,
) (string, error) {
	// Dedup check using tier_id + starts_at
	var existingCount int
	err := cp.pool.Primary().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM contests
		WHERE tier_id = $1
		  AND starts_at >= $2
		  AND starts_at < $3
	`, tier.ID, startsAt.Add(-1*time.Minute), startsAt.Add(1*time.Minute)).Scan(&existingCount)
	if err != nil {
		return "", fmt.Errorf("failed to check existing tier contests: %w", err)
	}
	if existingCount > 0 {
		traggeSchedulerDedupHits.Inc()
		cp.logger.Debug("Contest already exists for this tier time slot",
			zap.String("tier_id", tier.ID),
			zap.Time("starts_at", startsAt))
		return "", nil // Skip, not an error
	}

	// Resolve overrides from tier or fall back to template values
	entryFeeCents := int(tier.EntryFee)
	qtyTotal := entry.QtyTotal
	if tier.QtyTotalOverride != nil {
		qtyTotal = *tier.QtyTotalOverride
	}
	commissionRate := entry.CommissionRate
	if tier.CommissionRateOverride != nil {
		commissionRate = *tier.CommissionRateOverride
	}
	isFree := tier.IsFree
	maxParticipants := entry.MaxParticipants
	if tier.MaxParticipantsOverride != nil {
		maxParticipants = sql.NullInt32{Int32: *tier.MaxParticipantsOverride, Valid: true}
	}

	// Build contest name with tier label
	tierLabel := tier.Label
	if tierLabel == "" {
		tierLabel = fmt.Sprintf("%d", tier.EntryFee)
	}
	contestName := fmt.Sprintf("%s [%s] - %s", entry.Name, tierLabel, startsAt.Format("Jan 2 15:04 UTC"))

	// Begin transaction
	tx, err := cp.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	contestID := uuid.New().String()

	targetStatus := statemachine.StatusScheduled
	registrationDeadline := startsAt
	if startsAt.After(time.Now().UTC()) {
		targetStatus = statemachine.StatusRegistrationOpen
	}

	description := ""
	if entry.Description.Valid {
		description = entry.Description.String
	}

	var maxPart *int
	if maxParticipants.Valid {
		val := int(maxParticipants.Int32)
		maxPart = &val
	}

	contestType := "standard"
	if entry.Type.Valid {
		contestType = entry.Type.String
	}

	durationType := getDurationTypeFromMinutes(entry.DurationMinutes)

	// Insert contest with tier_id
	_, err = tx.ExecContext(ctx, `
		INSERT INTO contests (
			id, name, description, starts_at, ends_at, status, entry_fee_cents,
			platform_fee_bps, qty_total, duration_type, asset_class, duration_minutes,
			min_participants, max_participants, registration_deadline, auto_start,
			commission_rate, is_free, auto_generated, template_id, tier_id, type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
	`,
		contestID,
		contestName,
		description,
		startsAt,
		endsAt,
		"draft",
		entryFeeCents,
		0,
		qtyTotal,
		durationType,
		entry.AssetClass,
		entry.DurationMinutes,
		entry.MinParticipants,
		maxPart,
		registrationDeadline,
		entry.AutoStart,
		commissionRate,
		isFree,
		true,
		entry.ID,
		tier.ID,
		contestType,
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert tier contest: %w", err)
	}

	// Parse and insert symbols
	var symbols []string
	if jsonErr := json.Unmarshal([]byte(entry.SymbolsJSON), &symbols); jsonErr != nil {
		// JSON parse failed — treat the raw value as a single symbol only if it's valid
		raw := strings.TrimSpace(entry.SymbolsJSON)
		if _, valid := validation.ValidateSymbol(raw); valid {
			symbols = []string{raw}
		} else {
			return "", fmt.Errorf("invalid symbols data (not valid JSON array and not a valid symbol): %s", raw)
		}
	}

	// Validate every symbol before inserting
	for i, s := range symbols {
		normalized, valid := validation.ValidateSymbol(s)
		if !valid {
			return "", fmt.Errorf("invalid symbol at index %d: %q", i, s)
		}
		symbols[i] = normalized
	}

	for _, symbol := range symbols {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO contest_symbols (contest_id, symbol, enabled)
			VALUES ($1, $2, TRUE)
			ON CONFLICT (contest_id, symbol) DO NOTHING
		`, contestID, symbol); err != nil {
			return "", fmt.Errorf("failed to insert contest symbol %s: %w", symbol, err)
		}
	}

	// Auto-register bot for free contests
	if isFree {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available, is_system)
			 VALUES ($1, $2, $3, $3, TRUE)
			 ON CONFLICT (contest_id, user_id) DO NOTHING`,
			contestID, domain.TBotUserID, qtyTotal); err != nil {
			return "", fmt.Errorf("failed to register bot for free contest: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit tier contest: %w", err)
	}

	// State machine transitions
	transitionReason := fmt.Sprintf("Auto-created from template tier: %s [%s]", entry.Name, tierLabel)

	_, err = cp.stateMachine.Transition(ctx, statemachine.TransitionRequest{
		ContestID: contestID,
		ToStatus:  statemachine.StatusScheduled,
		Reason:    transitionReason,
	})
	if err != nil {
		cp.logger.Error("Failed to transition tier contest to scheduled",
			zap.String("contest_id", contestID), zap.Error(err))
		return contestID, nil
	}

	if targetStatus == statemachine.StatusRegistrationOpen {
		_, err = cp.stateMachine.Transition(ctx, statemachine.TransitionRequest{
			ContestID: contestID,
			ToStatus:  statemachine.StatusRegistrationOpen,
			Reason:    "Registration opened immediately on creation",
		})
		if err != nil {
			cp.logger.Error("Failed to transition tier contest to registration_open",
				zap.String("contest_id", contestID), zap.Error(err))
		}
	}

	cp.logger.Info("Created contest from tier",
		zap.String("contest_id", contestID),
		zap.String("tier_id", tier.ID),
		zap.String("tier_label", tierLabel),
		zap.Int64("entry_fee", tier.EntryFee))

	return contestID, nil
}

// createContestFromTemplate creates a new contest from a template.
func (cp *CalendarProcessor) createContestFromTemplate(
	ctx context.Context,
	entry CalendarEntry,
	startsAt, endsAt time.Time,
) (string, error) {
	tx, err := cp.pool.Primary().BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	contestID := uuid.New().String()

	// Determine target status - registration_open if registration should start now
	targetStatus := statemachine.StatusScheduled
	registrationDeadline := startsAt

	// If contest starts in the future, open registration immediately
	if startsAt.After(time.Now().UTC()) {
		targetStatus = statemachine.StatusRegistrationOpen
	}

	// Generate contest name with timestamp
	contestName := fmt.Sprintf("%s - %s", entry.Name, startsAt.Format("Jan 2 15:04 UTC"))

	// Get description
	description := ""
	if entry.Description.Valid {
		description = entry.Description.String
	}

	// Get max participants
	var maxParticipants *int
	if entry.MaxParticipants.Valid {
		val := int(entry.MaxParticipants.Int32)
		maxParticipants = &val
	}

	// Get contest type
	contestType := "standard"
	if entry.Type.Valid {
		contestType = entry.Type.String
	}

	// Determine duration_type based on duration
	durationType := getDurationTypeFromMinutes(entry.DurationMinutes)

	// Insert the contest
	_, err = tx.ExecContext(ctx, `
		INSERT INTO contests (
			id, name, description, starts_at, ends_at, status, entry_fee_cents,
			platform_fee_bps, qty_total, duration_type, asset_class, duration_minutes,
			min_participants, max_participants, registration_deadline, auto_start,
			commission_rate, is_free, auto_generated, template_id, type
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
	`,
		contestID,
		contestName,
		description,
		startsAt,
		endsAt,
		"draft",
		entry.EntryFeeCents,
		0, // platform_fee_bps (using commission_rate instead)
		entry.QtyTotal,
		durationType,
		entry.AssetClass,
		entry.DurationMinutes,
		entry.MinParticipants,
		maxParticipants,
		registrationDeadline,
		entry.AutoStart,
		entry.CommissionRate,
		entry.IsFree,
		true, // auto_generated
		entry.ID,
		contestType,
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert contest: %w", err)
	}

	// Parse and insert symbols
	var symbols []string
	if err := json.Unmarshal([]byte(entry.SymbolsJSON), &symbols); err != nil {
		cp.logger.Warn("Failed to parse symbols JSON, using as single symbol",
			zap.String("template_id", entry.ID),
			zap.Error(err))
		symbols = []string{entry.SymbolsJSON}
	}

	for _, symbol := range symbols {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO contest_symbols (contest_id, symbol, enabled)
			VALUES ($1, $2, TRUE)
			ON CONFLICT (contest_id, symbol) DO NOTHING
		`, contestID, symbol)
		if err != nil {
			cp.logger.Warn("Failed to insert contest symbol",
				zap.String("contest_id", contestID),
				zap.String("symbol", symbol),
				zap.Error(err))
		}
	}

	// Auto-register system bot for free contests so min_participants is met
	if entry.IsFree {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available, is_system)
			 VALUES ($1, $2, $3, $3, TRUE)
			 ON CONFLICT (contest_id, user_id) DO NOTHING`,
			contestID, domain.TBotUserID, entry.QtyTotal)
		if err != nil {
			cp.logger.Warn("Failed to register bot for free contest",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
	}

	// Commit transaction (contest is in draft status)
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Use state machine to transition through proper lifecycle with side effects.
	// This ensures all handlers (notifications, Kafka events) execute correctly.
	transitionReason := fmt.Sprintf("Auto-created from calendar template: %s", entry.Name)

	// Transition draft → scheduled
	_, err = cp.stateMachine.Transition(ctx, statemachine.TransitionRequest{
		ContestID: contestID,
		ToStatus:  statemachine.StatusScheduled,
		Reason:    transitionReason,
	})
	if err != nil {
		cp.logger.Error("Failed to transition contest to scheduled",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return contestID, fmt.Errorf("failed to transition to scheduled: %w", err)
	}

	// If registration should open now, transition scheduled → registration_open
	if targetStatus == statemachine.StatusRegistrationOpen {
		_, err = cp.stateMachine.Transition(ctx, statemachine.TransitionRequest{
			ContestID: contestID,
			ToStatus:  statemachine.StatusRegistrationOpen,
			Reason:    "Registration opened immediately on creation",
		})
		if err != nil {
			cp.logger.Error("Failed to transition contest to registration_open",
				zap.String("contest_id", contestID),
				zap.Error(err))
			// Don't return error - contest is created and scheduled successfully
		}
	}

	return contestID, nil
}

// updateNextOccurrence calculates and updates the next occurrence time.
func (cp *CalendarProcessor) updateNextOccurrence(ctx context.Context, entry CalendarEntry) error {
	nextOccurrence, err := calculateNextOccurrence(entry.RecurrenceRule, entry.NextOccurrenceAt)
	if err != nil {
		return fmt.Errorf("failed to calculate next occurrence: %w", err)
	}

	_, err = cp.pool.Primary().ExecContext(ctx, `
		UPDATE tournament_templates
		SET next_occurrence_at = $1, last_generated_at = NOW()
		WHERE id = $2
	`, nextOccurrence, entry.ID)

	if err != nil {
		return fmt.Errorf("failed to update next_occurrence_at: %w", err)
	}

	cp.logger.Info("Updated next_occurrence_at",
		zap.String("template_id", entry.ID),
		zap.Time("previous", entry.NextOccurrenceAt),
		zap.Time("next", nextOccurrence))

	return nil
}

// calculateNextOccurrence calculates the next occurrence based on recurrence rule.
// Supported formats:
// - "HOURLY" - every hour
// - "DAILY@HH:MM" - daily at specific time
// - "WEEKLY@DAY1,DAY2@HH:MM" - weekly on specific days at specific time
// - "MONTHLY@DD@HH:MM" - monthly on specific day at specific time
func calculateNextOccurrence(rule string, from time.Time) (time.Time, error) {
	rule = strings.ToUpper(strings.TrimSpace(rule))
	parts := strings.Split(rule, "@")

	switch parts[0] {
	case "HOURLY":
		return from.Add(1 * time.Hour), nil

	case "DAILY":
		// DAILY@HH:MM
		if len(parts) < 2 {
			return from.Add(24 * time.Hour), nil
		}
		timeOfDay, err := parseTimeOfDay(parts[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid time format in DAILY rule: %w", err)
		}
		next := time.Date(from.Year(), from.Month(), from.Day()+1,
			timeOfDay.Hour(), timeOfDay.Minute(), 0, 0, time.UTC)
		return next, nil

	case "WEEKLY":
		// WEEKLY@MON,WED,FRI@HH:MM
		if len(parts) < 3 {
			return from.Add(7 * 24 * time.Hour), nil
		}
		days := parseDaysOfWeek(parts[1])
		if len(days) == 0 {
			days = []time.Weekday{from.Weekday()}
		}
		timeOfDay, err := parseTimeOfDay(parts[2])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid time format in WEEKLY rule: %w", err)
		}
		return findNextWeeklyOccurrence(from, days, timeOfDay), nil

	case "MONTHLY":
		// MONTHLY@DD@HH:MM
		if len(parts) < 3 {
			return from.AddDate(0, 1, 0), nil
		}
		dayOfMonth, err := strconv.Atoi(parts[1])
		if err != nil || dayOfMonth < 1 || dayOfMonth > 31 {
			dayOfMonth = from.Day()
		}
		timeOfDay, err := parseTimeOfDay(parts[2])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid time format in MONTHLY rule: %w", err)
		}
		return findNextMonthlyOccurrence(from, dayOfMonth, timeOfDay), nil

	default:
		// Default to hourly if unknown
		return from.Add(1 * time.Hour), nil
	}
}

// parseTimeOfDay parses "HH:MM" format.
func parseTimeOfDay(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid time format: %s", s)
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, fmt.Errorf("invalid hour: %s", parts[0])
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return time.Time{}, fmt.Errorf("invalid minute: %s", parts[1])
	}

	return time.Date(0, 1, 1, hour, minute, 0, 0, time.UTC), nil
}

// parseDaysOfWeek parses "MON,WED,FRI" format.
func parseDaysOfWeek(s string) []time.Weekday {
	dayMap := map[string]time.Weekday{
		"SUN":       time.Sunday,
		"MON":       time.Monday,
		"TUE":       time.Tuesday,
		"WED":       time.Wednesday,
		"THU":       time.Thursday,
		"FRI":       time.Friday,
		"SAT":       time.Saturday,
		"SUNDAY":    time.Sunday,
		"MONDAY":    time.Monday,
		"TUESDAY":   time.Tuesday,
		"WEDNESDAY": time.Wednesday,
		"THURSDAY":  time.Thursday,
		"FRIDAY":    time.Friday,
		"SATURDAY":  time.Saturday,
	}

	var days []time.Weekday
	parts := strings.Split(strings.ToUpper(s), ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if day, ok := dayMap[p]; ok {
			days = append(days, day)
		}
	}
	return days
}

// findNextWeeklyOccurrence finds the next occurrence for weekly recurrence.
func findNextWeeklyOccurrence(from time.Time, days []time.Weekday, timeOfDay time.Time) time.Time {
	// Sort days to ensure consistent ordering
	sortedDays := make([]time.Weekday, len(days))
	copy(sortedDays, days)

	// Start from the next day
	candidate := from.Add(time.Minute) // Add a minute to ensure we move past current time

	// Try up to 8 days ahead (a full week plus one day for safety)
	for i := 0; i < 8; i++ {
		checkDate := candidate.AddDate(0, 0, i)
		checkDate = time.Date(checkDate.Year(), checkDate.Month(), checkDate.Day(),
			timeOfDay.Hour(), timeOfDay.Minute(), 0, 0, time.UTC)

		// Check if this day is in our target days
		for _, targetDay := range sortedDays {
			if checkDate.Weekday() == targetDay && checkDate.After(from) {
				return checkDate
			}
		}
	}

	// Fallback: just add a week
	return time.Date(from.Year(), from.Month(), from.Day()+7,
		timeOfDay.Hour(), timeOfDay.Minute(), 0, 0, time.UTC)
}

// findNextMonthlyOccurrence finds the next occurrence for monthly recurrence.
func findNextMonthlyOccurrence(from time.Time, dayOfMonth int, timeOfDay time.Time) time.Time {
	// Try current month first
	candidate := time.Date(from.Year(), from.Month(), dayOfMonth,
		timeOfDay.Hour(), timeOfDay.Minute(), 0, 0, time.UTC)

	// If the day doesn't exist in current month (e.g., Feb 31), use last day
	for candidate.Day() != dayOfMonth {
		dayOfMonth--
		candidate = time.Date(from.Year(), from.Month(), dayOfMonth,
			timeOfDay.Hour(), timeOfDay.Minute(), 0, 0, time.UTC)
	}

	if candidate.After(from) {
		return candidate
	}

	// Move to next month
	nextMonth := from.AddDate(0, 1, 0)
	targetDay := dayOfMonth

	// Handle months with fewer days
	lastDayOfMonth := time.Date(nextMonth.Year(), nextMonth.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if targetDay > lastDayOfMonth {
		targetDay = lastDayOfMonth
	}

	return time.Date(nextMonth.Year(), nextMonth.Month(), targetDay,
		timeOfDay.Hour(), timeOfDay.Minute(), 0, 0, time.UTC)
}

// getDurationTypeFromMinutes converts duration in minutes to duration_type enum.
func getDurationTypeFromMinutes(minutes int) string {
	switch {
	case minutes <= 30:
		return "rush_30min"
	case minutes <= 60:
		return "hourly"
	case minutes <= 240:
		return "4hour"
	case minutes <= 1440:
		return "daily"
	default:
		return "weekly"
	}
}

// recordError records an error in the processor stats.
func (cp *CalendarProcessor) recordError(err error) {
	calendarProcessingErrorsTotal.Inc()
	traggeSchedulerErrorsTotal.WithLabelValues("calendar").Inc()
	cp.mu.Lock()
	cp.errorsCount++
	cp.lastError = err
	cp.mu.Unlock()
}

// IsRunning returns whether the calendar processor is running.
func (cp *CalendarProcessor) IsRunning() bool {
	return cp.running.Load()
}

// GetStats returns current statistics for the calendar processor.
func (cp *CalendarProcessor) GetStats() map[string]interface{} {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	var lastErrorStr string
	if cp.lastError != nil {
		lastErrorStr = cp.lastError.Error()
	}

	return map[string]interface{}{
		"running":           cp.running.Load(),
		"check_interval":    cp.config.CheckInterval.String(),
		"last_check":        cp.lastCheck,
		"contests_created":  cp.contestsCreated,
		"entries_processed": cp.entriesProcessed,
		"errors_count":      cp.errorsCount,
		"last_error":        lastErrorStr,
	}
}

// CalendarHealth contains calendar processor health information.
type CalendarHealth struct {
	Running          bool      `json:"running"`
	LastCheck        time.Time `json:"last_check"`
	CheckInterval    string    `json:"check_interval"`
	ContestsCreated  int64     `json:"contests_created"`
	EntriesProcessed int64     `json:"entries_processed"`
	ErrorsCount      int64     `json:"errors_count"`
	LastError        string    `json:"last_error,omitempty"`
}

// Health returns the calendar processor's health status.
func (cp *CalendarProcessor) Health() interface{} {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	var lastErrorStr string
	if cp.lastError != nil {
		lastErrorStr = cp.lastError.Error()
	}

	return CalendarHealth{
		Running:          cp.running.Load(),
		LastCheck:        cp.lastCheck,
		CheckInterval:    cp.config.CheckInterval.String(),
		ContestsCreated:  cp.contestsCreated,
		EntriesProcessed: cp.entriesProcessed,
		ErrorsCount:      cp.errorsCount,
		LastError:        lastErrorStr,
	}
}

// IsHealthy returns true if the calendar processor is running and has no recent errors.
func (cp *CalendarProcessor) IsHealthy() bool {
	if !cp.running.Load() {
		return false
	}

	cp.mu.RLock()
	defer cp.mu.RUnlock()

	// Check if last check was within 2x check interval
	if time.Since(cp.lastCheck) > cp.config.CheckInterval*2 {
		return false
	}

	return true
}
