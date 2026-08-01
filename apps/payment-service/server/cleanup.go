package server

import (
	"context"
	"database/sql"
	"time"

	"go.uber.org/zap"
)

// CleanupConfig holds configuration for the cleanup worker.
type CleanupConfig struct {
	Interval      time.Duration // How often to run cleanup
	OrphanedAfter time.Duration // How old a pending intent must be to be considered orphaned
}

// StartCleanupWorker runs a background loop that periodically expires orphaned
// payment intents. An orphaned payment intent is one with status 'pending',
// no provider_payment_id, and created_at older than the configured threshold.
// This handles the case where the app crashes between INSERT and provider call.
func StartCleanupWorker(ctx context.Context, db *sql.DB, circuits *CircuitBreakers, logger *zap.Logger, cfg CleanupConfig) {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	logger.Info("Payment intent cleanup worker started",
		zap.Duration("interval", cfg.Interval),
		zap.Duration("orphaned_after", cfg.OrphanedAfter))

	for {
		select {
		case <-ctx.Done():
			logger.Info("Payment intent cleanup worker stopped")
			return
		case <-ticker.C:
			expireOrphanedPaymentIntents(ctx, db, circuits, logger, cfg.OrphanedAfter)
		}
	}
}

func expireOrphanedPaymentIntents(ctx context.Context, db *sql.DB, circuits *CircuitBreakers, logger *zap.Logger, orphanedAfter time.Duration) {
	cutoff := time.Now().Add(-orphanedAfter)

	var count int64
	err := circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		result, e := db.ExecContext(ctx, `
			UPDATE payment_intents
			SET status = 'expired', updated_at = NOW()
			WHERE status = 'pending'
			  AND provider_payment_id IS NULL
			  AND created_at < $1
		`, cutoff)
		if e != nil {
			return e
		}
		count, _ = result.RowsAffected()
		return nil
	})

	if err != nil {
		logger.Error("Failed to expire orphaned payment intents",
			zap.Error(err),
			zap.Time("cutoff", cutoff))
		return
	}

	if count > 0 {
		logger.Info("Expired orphaned payment intents",
			zap.Int64("count", count),
			zap.Time("cutoff", cutoff))
	}
}
