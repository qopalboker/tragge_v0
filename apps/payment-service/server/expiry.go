package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"go.uber.org/zap"
)

// ExpiryConfig holds configuration for the expiry worker.
type ExpiryConfig struct {
	Interval                 time.Duration // How often to run the expiry cycle
	Threshold                time.Duration // How old a payment intent must be to be considered stale
	PayoutThreshold          time.Duration // How old a payout must be to be considered stuck
	ProcessingAlertThreshold time.Duration // How old a processing payout must be to trigger an alert
	BatchSize                int           // Max items to process per cycle
}

// StartExpiryWorker runs a background loop that periodically checks for stale
// payment intents and stuck payouts. Unlike the cleanup worker (which handles
// orphaned intents without a provider_payment_id), this worker handles intents
// that DO have a provider_payment_id but remain in a non-terminal state.
// It queries the provider for the actual status and updates accordingly.
// For stuck payouts, it rolls back the debited wallet balance.
func StartExpiryWorker(
	ctx context.Context,
	db *sql.DB,
	circuits *CircuitBreakers,
	registry *providers.ProviderRegistry,
	walletService *wallet.Service,
	logger *zap.Logger,
	cfg ExpiryConfig,
) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	logger.Info("Payment expiry worker started",
		zap.Duration("interval", cfg.Interval),
		zap.Duration("threshold", cfg.Threshold),
		zap.Duration("payout_threshold", cfg.PayoutThreshold),
		zap.Int("batch_size", cfg.BatchSize))

	for {
		select {
		case <-ctx.Done():
			logger.Info("Payment expiry worker stopped")
			return
		case <-ticker.C:
			expireStalePaymentIntents(ctx, db, circuits, registry, walletService, logger, cfg)
			rollbackStuckPayouts(ctx, db, circuits, registry, walletService, logger, cfg)
			alertStuckProcessingPayouts(ctx, db, logger, cfg)
		}
	}
}

// stalePaymentIntent represents a payment intent row selected for expiry processing.
type stalePaymentIntent struct {
	ID                string
	UserID            string
	Provider          string
	ProviderPaymentID string
	AmountCents       int64
	Currency          string
	Status            string
	MetadataJSON      sql.NullString
}

func expireStalePaymentIntents(
	ctx context.Context,
	db *sql.DB,
	circuits *CircuitBreakers,
	registry *providers.ProviderRegistry,
	walletService *wallet.Service,
	logger *zap.Logger,
	cfg ExpiryConfig,
) {
	cutoff := time.Now().Add(-cfg.Threshold)

	var intents []stalePaymentIntent
	err := circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		rows, e := db.QueryContext(ctx, `
			SELECT id, user_id, provider, provider_payment_id, amount_cents, currency, status, metadata_json
			FROM payment_intents
			WHERE status IN ('pending', 'processing')
			  AND provider_payment_id IS NOT NULL
			  AND created_at < $1
			ORDER BY created_at ASC
			LIMIT $2
		`, cutoff, cfg.BatchSize)
		if e != nil {
			return e
		}
		defer rows.Close()

		for rows.Next() {
			var pi stalePaymentIntent
			if err := rows.Scan(&pi.ID, &pi.UserID, &pi.Provider, &pi.ProviderPaymentID,
				&pi.AmountCents, &pi.Currency, &pi.Status, &pi.MetadataJSON); err != nil {
				return err
			}
			intents = append(intents, pi)
		}
		return rows.Err()
	})
	if err != nil {
		logger.Error("Failed to query stale payment intents", zap.Error(err))
		return
	}

	if len(intents) == 0 {
		return
	}

	logger.Info("Processing stale payment intents", zap.Int("count", len(intents)))

	for _, pi := range intents {
		if ctx.Err() != nil {
			return
		}

		processStalePaymentIntent(ctx, db, circuits, registry, walletService, logger, pi)

		// Small delay between API calls to avoid hammering providers
		time.Sleep(200 * time.Millisecond)
	}
}

func processStalePaymentIntent(
	ctx context.Context,
	db *sql.DB,
	circuits *CircuitBreakers,
	registry *providers.ProviderRegistry,
	walletService *wallet.Service,
	logger *zap.Logger,
	pi stalePaymentIntent,
) {
	provider, ok := registry.Get(providers.ProviderType(pi.Provider))
	if !ok {
		logger.Warn("Provider not found for stale payment intent",
			zap.String("payment_intent_id", pi.ID),
			zap.String("provider", pi.Provider))
		return
	}

	// Query provider for actual status
	statusResp, err := provider.GetPaymentStatus(ctx, pi.ProviderPaymentID)
	if err != nil {
		logger.Warn("Failed to query provider for stale intent status",
			zap.Error(err),
			zap.String("payment_intent_id", pi.ID),
			zap.String("provider", pi.Provider),
			zap.String("provider_payment_id", pi.ProviderPaymentID))
		return
	}

	intentStatus := mapProviderStatusToIntentStatus(statusResp.Status)

	// Still not resolved — skip until next cycle
	if intentStatus == "pending" || intentStatus == "processing" {
		logger.Debug("Stale payment intent still unresolved at provider",
			zap.String("payment_intent_id", pi.ID),
			zap.String("provider", pi.Provider),
			zap.String("provider_status", string(statusResp.Status)))
		return
	}

	logger.Info("Expiry worker resolved stale payment intent",
		zap.String("payment_intent_id", pi.ID),
		zap.String("provider", pi.Provider),
		zap.String("provider_status", string(statusResp.Status)),
		zap.String("new_status", intentStatus))

	// Begin transaction for atomic processing
	var tx *sql.Tx
	err = circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var e error
		tx, e = db.BeginTx(ctx, nil)
		return e
	})
	if err != nil {
		logger.Error("Failed to begin expiry transaction",
			zap.Error(err),
			zap.String("payment_intent_id", pi.ID))
		return
	}
	defer tx.Rollback()

	// Lock the row to prevent concurrent webhook processing
	var currentStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM payment_intents WHERE id = $1 FOR UPDATE
	`, pi.ID).Scan(&currentStatus)
	if err != nil {
		logger.Error("Failed to lock payment intent for expiry",
			zap.Error(err),
			zap.String("payment_intent_id", pi.ID))
		return
	}

	// Already resolved by a webhook while we were querying
	if currentStatus == "succeeded" || currentStatus == "failed" || currentStatus == "refunded" || currentStatus == "expired" {
		logger.Info("Payment intent already resolved before expiry update",
			zap.String("payment_intent_id", pi.ID),
			zap.String("current_status", currentStatus))
		return
	}

	// If payment succeeded, credit wallet within the same transaction
	if statusResp.Status == providers.PaymentStatusFinished || statusResp.Status == providers.PaymentStatusConfirmed {
		txWrapper := &wallet.TxAdapter{Tx: tx}
		idempotencyKey := "deposit:" + pi.ID
		refType := wallet.LedgerRefTypePaymentIntent
		_, err := walletService.CreditIdempotent(ctx, txWrapper, pi.UserID, pi.AmountCents,
			wallet.LedgerTypeDeposit, &refType, &pi.ID, nil, idempotencyKey)
		if err != nil {
			if _, ok := err.(*wallet.DuplicateCreditError); ok {
				logger.Warn("Duplicate deposit credit detected by expiry worker",
					zap.String("payment_intent_id", pi.ID))
			} else {
				logger.Error("Failed to credit wallet from expiry worker",
					zap.Error(err),
					zap.String("payment_intent_id", pi.ID))
				return
			}
		}
		intentStatus = "succeeded"
	}

	// Update payment intent status
	var completedAt interface{}
	if intentStatus == "succeeded" || intentStatus == "failed" || intentStatus == "refunded" || intentStatus == "expired" {
		completedAt = time.Now()
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE payment_intents
		SET status = $1, completed_at = $2, updated_at = $3
		WHERE id = $4
	`, intentStatus, completedAt, time.Now(), pi.ID)
	if err != nil {
		logger.Error("Failed to update payment intent from expiry worker",
			zap.Error(err),
			zap.String("payment_intent_id", pi.ID))
		return
	}

	if err := tx.Commit(); err != nil {
		logger.Error("Failed to commit expiry transaction",
			zap.Error(err),
			zap.String("payment_intent_id", pi.ID))
		return
	}

	logger.Info("Expiry worker updated payment intent",
		zap.String("payment_intent_id", pi.ID),
		zap.String("status", intentStatus))
}

// stuckPayout represents a payout row selected for rollback processing.
type stuckPayout struct {
	ID               string
	UserID           string
	AmountCents      int64
	Currency         string
	Provider         string
	ProviderPayoutID sql.NullString
	MetadataJSON     sql.NullString
}

func rollbackStuckPayouts(
	ctx context.Context,
	db *sql.DB,
	circuits *CircuitBreakers,
	registry *providers.ProviderRegistry,
	walletService *wallet.Service,
	logger *zap.Logger,
	cfg ExpiryConfig,
) {
	cutoff := time.Now().Add(-cfg.PayoutThreshold)

	var payouts []stuckPayout
	err := circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		rows, e := db.QueryContext(ctx, `
			SELECT id, user_id, amount_cents, currency, provider, provider_payout_id, metadata_json
			FROM payouts
			WHERE status = 'pending'
			  AND created_at < $1
			ORDER BY created_at ASC
			LIMIT $2
		`, cutoff, cfg.BatchSize)
		if e != nil {
			return e
		}
		defer rows.Close()

		for rows.Next() {
			var p stuckPayout
			if err := rows.Scan(&p.ID, &p.UserID, &p.AmountCents, &p.Currency,
				&p.Provider, &p.ProviderPayoutID, &p.MetadataJSON); err != nil {
				return err
			}
			payouts = append(payouts, p)
		}
		return rows.Err()
	})
	if err != nil {
		logger.Error("Failed to query stuck payouts", zap.Error(err))
		return
	}

	if len(payouts) == 0 {
		return
	}

	logger.Info("Processing stuck payouts", zap.Int("count", len(payouts)))

	for _, p := range payouts {
		if ctx.Err() != nil {
			return
		}

		processStuckPayout(ctx, db, circuits, registry, walletService, logger, p)

		time.Sleep(200 * time.Millisecond)
	}
}

func processStuckPayout(
	ctx context.Context,
	db *sql.DB,
	circuits *CircuitBreakers,
	registry *providers.ProviderRegistry,
	walletService *wallet.Service,
	logger *zap.Logger,
	p stuckPayout,
) {
	// If the payout has a provider payout ID, check with the provider first
	if p.ProviderPayoutID.Valid && p.ProviderPayoutID.String != "" {
		provider, ok := registry.Get(providers.ProviderType(p.Provider))
		if ok {
			payoutResp, err := provider.GetPayoutStatus(ctx, p.ProviderPayoutID.String)
			if err != nil {
				logger.Warn("Failed to query provider for stuck payout, skipping rollback to avoid double-spend",
					zap.Error(err),
					zap.String("payout_id", p.ID),
					zap.String("provider", p.Provider),
					zap.String("provider_payout_id", p.ProviderPayoutID.String))
				// DO NOT proceed with rollback when provider is unreachable
				// and we have a provider_payout_id. The payout may have been
				// processed successfully. We will retry on the next cycle.
				return
			} else {
				status := mapProviderStatusToIntentStatus(payoutResp.Status)
				// If provider says it succeeded or is still processing, don't roll back
				if status == "succeeded" || status == "processing" || status == "pending" {
					logger.Debug("Stuck payout still active at provider, skipping rollback",
						zap.String("payout_id", p.ID),
						zap.String("provider_status", string(payoutResp.Status)))
					return
				}
			}
		}
	}

	logger.Info("Rolling back stuck payout",
		zap.String("payout_id", p.ID),
		zap.String("user_id", p.UserID),
		zap.Int64("amount_cents", p.AmountCents))

	// Begin transaction for atomic rollback
	var tx *sql.Tx
	err := circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var e error
		tx, e = db.BeginTx(ctx, nil)
		return e
	})
	if err != nil {
		logger.Error("Failed to begin payout rollback transaction",
			zap.Error(err),
			zap.String("payout_id", p.ID))
		return
	}
	defer tx.Rollback()

	// Lock the row
	var currentStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM payouts WHERE id = $1 FOR UPDATE
	`, p.ID).Scan(&currentStatus)
	if err != nil {
		logger.Error("Failed to lock payout for rollback",
			zap.Error(err),
			zap.String("payout_id", p.ID))
		return
	}

	// Already resolved
	if currentStatus != "pending" {
		logger.Info("Payout already resolved before rollback",
			zap.String("payout_id", p.ID),
			zap.String("current_status", currentStatus))
		return
	}

	// Credit wallet back (rollback the debit) with idempotency protection
	txWrapper := &wallet.TxAdapter{Tx: tx}
	idempotencyKey := fmt.Sprintf("withdrawal_rollback:%s", p.ID)
	refType := wallet.LedgerRefTypePayout
	desc := "Withdrawal rollback: payout expired"
	_, err = walletService.CreditIdempotent(ctx, txWrapper, p.UserID, p.AmountCents,
		wallet.LedgerTypeWithdrawalRefund, &refType, &p.ID, &desc, idempotencyKey)
	if err != nil {
		if _, ok := err.(*wallet.DuplicateCreditError); ok {
			logger.Warn("Duplicate rollback credit detected",
				zap.String("payout_id", p.ID))
		} else {
			logger.Error("Failed to credit wallet for payout rollback",
				zap.Error(err),
				zap.String("payout_id", p.ID))
			return
		}
	}

	// Also rollback the fee debit if there was a fee
	if p.MetadataJSON.Valid {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(p.MetadataJSON.String), &metadata); err == nil {
			if feeCents, ok := metadata["fee_cents"].(float64); ok && int64(feeCents) > 0 {
				feeIdempotencyKey := fmt.Sprintf("withdrawal_fee_rollback:%s", p.ID)
				feeDesc := "Withdrawal fee rollback: payout expired"
				_, err = walletService.CreditIdempotent(ctx, txWrapper, p.UserID, int64(feeCents),
					wallet.LedgerTypeWithdrawFeeRefund, &refType, &p.ID, &feeDesc, feeIdempotencyKey)
				if err != nil {
					if _, ok := err.(*wallet.DuplicateCreditError); ok {
						logger.Warn("Duplicate fee rollback credit detected",
							zap.String("payout_id", p.ID))
					} else {
						logger.Error("Failed to credit wallet for fee rollback",
							zap.Error(err),
							zap.String("payout_id", p.ID))
						return
					}
				}
			}
		}
	}

	// Update payout status to failed
	_, err = tx.ExecContext(ctx, `
		UPDATE payouts
		SET status = 'failed', completed_at = $1, updated_at = $1
		WHERE id = $2
	`, time.Now(), p.ID)
	if err != nil {
		logger.Error("Failed to update payout status for rollback",
			zap.Error(err),
			zap.String("payout_id", p.ID))
		return
	}

	if err := tx.Commit(); err != nil {
		logger.Error("Failed to commit payout rollback transaction",
			zap.Error(err),
			zap.String("payout_id", p.ID))
		return
	}

	logger.Info("Payout rolled back successfully",
		zap.String("payout_id", p.ID),
		zap.String("user_id", p.UserID),
		zap.Int64("amount_cents", p.AmountCents))
}

// alertStuckProcessingPayouts logs warnings for payouts stuck in 'processing' state
// beyond the configured threshold. These are NOT auto-refunded because an admin
// explicitly approved them — they need manual resolution via /complete or /fail.
func alertStuckProcessingPayouts(
	ctx context.Context,
	db *sql.DB,
	logger *zap.Logger,
	cfg ExpiryConfig,
) {
	threshold := cfg.ProcessingAlertThreshold
	if threshold == 0 {
		threshold = 48 * time.Hour // Default: alert after 48 hours in processing
	}
	cutoff := time.Now().Add(-threshold)

	var count int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM payouts
		WHERE status = 'processing'
		  AND updated_at < $1
	`, cutoff).Scan(&count)

	if err != nil {
		logger.Error("Failed to count stuck processing payouts", zap.Error(err))
		return
	}

	if count > 0 {
		logger.Error("ALERT: payouts stuck in processing state — admin action required",
			zap.Int("count", count),
			zap.Duration("threshold", threshold),
			zap.String("action", "Use /api/admin/withdrawals/{id}/complete or /fail to resolve"))
	}
}
