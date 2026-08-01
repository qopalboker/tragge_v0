package server

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// startStuckDetector periodically checks for stuck settlements and retries them.
func (a *App) startStuckDetector(ctx context.Context) {
	defer a.wg.Done()

	ticker := time.NewTicker(a.config.StuckCheckInterval)
	defer ticker.Stop()

	a.log().Info("Stuck settlement detector started",
		zap.Duration("check_interval", a.config.StuckCheckInterval),
		zap.Duration("stuck_threshold", a.config.StuckThreshold),
		zap.Duration("orphaned_settling_threshold", a.config.OrphanedSettlingThreshold),
		zap.Int("max_retries", a.config.MaxRetries))

	for {
		select {
		case <-ctx.Done():
			a.log().Info("Stuck settlement detector stopped")
			return
		case <-ticker.C:
			a.detectAndRecoverStuckSettlements(ctx)
			a.detectOrphanedSettlingContests(ctx)
			a.detectFailedSettlingContests(ctx)
		}
	}
}

// detectAndRecoverStuckSettlements finds settlements stuck in "in_progress" and retries or fails them.
func (a *App) detectAndRecoverStuckSettlements(ctx context.Context) {
	// Find settlements stuck in "in_progress" that are retryable
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, contest_id, attempt_count
		FROM contest_settlements
		WHERE status = 'in_progress'
		  AND updated_at < NOW() - $1::interval
		  AND attempt_count < $2
		FOR UPDATE SKIP LOCKED`,
		fmt.Sprintf("%d seconds", int(a.config.StuckThreshold.Seconds())), a.config.MaxRetries)
	if err != nil {
		a.log().Error("Failed to query stuck settlements", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, contestID string
		var attempts int
		if err := rows.Scan(&id, &contestID, &attempts); err != nil {
			a.log().Error("Failed to scan stuck settlement row", zap.Error(err))
			continue
		}

		a.metrics.StuckSettlementsDetected.Inc()
		a.log().Warn("Retrying stuck settlement",
			zap.String("settlement_id", id),
			zap.String("contest_id", contestID),
			zap.Int("attempt", attempts+1),
			zap.Int("max_retries", a.config.MaxRetries))

		// Reset to pending for retry
		_, err := a.db.ExecContext(ctx, `
			UPDATE contest_settlements
			SET status = 'pending', updated_at = NOW()
			WHERE id = $1`, id)
		if err != nil {
			a.log().Error("Failed to reset stuck settlement", zap.Error(err))
			continue
		}

		// Re-trigger settlement in background (deduplication prevents double-runs)
		a.tryStartSettlement(contestID, "stuck_detector")
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating stuck settlements", zap.Error(err))
	}

	// Mark settlements that have exhausted all retries as failed
	result, err := a.db.ExecContext(ctx, `
		UPDATE contest_settlements
		SET status = 'failed', last_error = 'stuck settlement exceeded max retries', failed_at = NOW(), updated_at = NOW()
		WHERE status = 'in_progress'
		  AND updated_at < NOW() - $1::interval
		  AND attempt_count >= $2`,
		fmt.Sprintf("%d seconds", int(a.config.StuckThreshold.Seconds())), a.config.MaxRetries)
	if err != nil {
		a.log().Error("Failed to mark exhausted stuck settlements as failed", zap.Error(err))
		return
	}

	if affected, _ := result.RowsAffected(); affected > 0 {
		a.log().Warn("Marked stuck settlements as failed (max retries exceeded)",
			zap.Int64("count", affected))
	}
}

// detectOrphanedSettlingContests finds contests stuck in "settling" status
// that have no corresponding settlement record. This happens when the Kafka
// message from the scheduler is lost (consumer lag, restart, crash) and
// settlement-service never receives the trigger.
func (a *App) detectOrphanedSettlingContests(ctx context.Context) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT c.id
		FROM contests c
		LEFT JOIN contest_settlements cs ON cs.contest_id = c.id
		WHERE c.status = 'settling'
		  AND cs.id IS NULL
		  AND c.updated_at < NOW() - $1::interval`,
		fmt.Sprintf("%d seconds", int(a.config.OrphanedSettlingThreshold.Seconds())))
	if err != nil {
		a.log().Error("Failed to query orphaned settling contests", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var contestID string
		if err := rows.Scan(&contestID); err != nil {
			a.log().Error("Failed to scan orphaned settling contest row", zap.Error(err))
			continue
		}

		a.metrics.OrphanedSettlingDetected.Inc()
		a.log().Warn("Detected orphaned settling contest (no settlement record), triggering settlement",
			zap.String("contest_id", contestID),
			zap.Duration("threshold", a.config.OrphanedSettlingThreshold))

		a.tryStartSettlement(contestID, "orphaned_settling_detector")
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating orphaned settling contests", zap.Error(err))
	}
}

// detectFailedSettlingContests logs an alert for contests that have
// status = 'settling' but their settlement record has status = 'failed'.
// These require manual investigation and intervention.
func (a *App) detectFailedSettlingContests(ctx context.Context) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT c.id, cs.id, cs.last_error
		FROM contests c
		INNER JOIN contest_settlements cs ON cs.contest_id = c.id
		WHERE c.status = 'settling'
		  AND cs.status = 'failed'`)
	if err != nil {
		a.log().Error("Failed to query failed settling contests", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var contestID, settlementID string
		var lastError sql.NullString
		if err := rows.Scan(&contestID, &settlementID, &lastError); err != nil {
			a.log().Error("Failed to scan failed settling contest row", zap.Error(err))
			continue
		}

		a.metrics.FailedSettlingDetected.Inc()
		a.log().Error("ALERT: Contest stuck in settling with failed settlement — requires manual investigation",
			zap.String("contest_id", contestID),
			zap.String("settlement_id", settlementID),
			zap.String("last_error", lastError.String))
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Error iterating failed settling contests", zap.Error(err))
	}
}
