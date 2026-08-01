package server

import (
	"context"
	"database/sql"
	"time"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"go.uber.org/zap"
)

// InquiryConfig holds configuration for the inquiry worker.
type InquiryConfig struct {
	Interval  time.Duration // How often to run inquiry
	MaxAge    time.Duration // Only inquiry intents newer than this
	BatchSize int           // Max intents per cycle
}

// StartInquiryWorker runs a background loop that periodically queries payment
// intents stuck in "processing" status and calls each provider's status API
// to check if they have resolved to a final state. Currently handles Jibit
// UNKNOWN status requiring periodic inquiry.
func StartInquiryWorker(
	ctx context.Context,
	db *sql.DB,
	circuits *CircuitBreakers,
	registry *providers.ProviderRegistry,
	walletService *wallet.Service,
	logger *zap.Logger,
	cfg InquiryConfig,
) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	logger.Info("Payment inquiry worker started",
		zap.Duration("interval", cfg.Interval),
		zap.Duration("max_age", cfg.MaxAge),
		zap.Int("batch_size", cfg.BatchSize))

	for {
		select {
		case <-ctx.Done():
			logger.Info("Payment inquiry worker stopped")
			return
		case <-ticker.C:
			inquireStuckJibitPayments(ctx, db, circuits, registry, walletService, logger, cfg)
		}
	}
}

// stuckPaymentIntent represents a payment intent row selected for inquiry.
type stuckPaymentIntent struct {
	ID                string
	UserID            string
	ProviderPaymentID string
	AmountCents       int64
	Currency          string
	MetadataJSON      sql.NullString
}

func inquireStuckJibitPayments(
	ctx context.Context,
	db *sql.DB,
	circuits *CircuitBreakers,
	registry *providers.ProviderRegistry,
	walletService *wallet.Service,
	logger *zap.Logger,
	cfg InquiryConfig,
) {
	provider, ok := registry.Get(providers.ProviderJibit)
	if !ok {
		return // Jibit not configured
	}

	cutoff := time.Now().Add(-cfg.MaxAge)

	var intents []stuckPaymentIntent
	err := circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		rows, e := db.QueryContext(ctx, `
			SELECT id, user_id, provider_payment_id, amount_cents, currency, metadata_json
			FROM payment_intents
			WHERE status = 'processing'
			  AND provider = 'jibit'
			  AND provider_payment_id IS NOT NULL
			  AND created_at > $1
			ORDER BY created_at ASC
			LIMIT $2
		`, cutoff, cfg.BatchSize)
		if e != nil {
			return e
		}
		defer rows.Close()

		for rows.Next() {
			var pi stuckPaymentIntent
			if err := rows.Scan(&pi.ID, &pi.UserID, &pi.ProviderPaymentID,
				&pi.AmountCents, &pi.Currency, &pi.MetadataJSON); err != nil {
				return err
			}
			intents = append(intents, pi)
		}
		return rows.Err()
	})
	if err != nil {
		logger.Error("Failed to query stuck Jibit payment intents",
			zap.Error(err))
		return
	}

	if len(intents) == 0 {
		return
	}

	logger.Info("Inquiring stuck Jibit payment intents",
		zap.Int("count", len(intents)))

	for _, pi := range intents {
		// Check context cancellation between inquiries
		if ctx.Err() != nil {
			return
		}

		processStuckIntent(ctx, db, circuits, provider, walletService, logger, pi)

		// Small delay between API calls to avoid hammering Jibit
		time.Sleep(200 * time.Millisecond)
	}
}

func processStuckIntent(
	ctx context.Context,
	db *sql.DB,
	circuits *CircuitBreakers,
	provider providers.Provider,
	walletService *wallet.Service,
	logger *zap.Logger,
	pi stuckPaymentIntent,
) {
	providerName := string(provider.Name())

	statusResp, err := provider.GetPaymentStatus(ctx, pi.ProviderPaymentID)
	if err != nil {
		logger.Warn("Inquiry failed for payment",
			zap.Error(err),
			zap.String("provider", providerName),
			zap.String("payment_intent_id", pi.ID),
			zap.String("provider_payment_id", pi.ProviderPaymentID))
		return
	}

	intentStatus := mapProviderStatusToIntentStatus(statusResp.Status)

	// Still not resolved — skip until next cycle
	if intentStatus == "pending" || intentStatus == "processing" {
		logger.Debug("Payment still unresolved",
			zap.String("provider", providerName),
			zap.String("payment_intent_id", pi.ID),
			zap.String("provider_status", string(statusResp.Status)),
			zap.String("intent_status", intentStatus))
		return
	}

	logger.Info("Inquiry resolved stuck payment",
		zap.String("provider", providerName),
		zap.String("payment_intent_id", pi.ID),
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
		logger.Error("Failed to begin inquiry transaction",
			zap.Error(err),
			zap.String("provider", providerName),
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
		logger.Error("Failed to lock payment intent for inquiry",
			zap.Error(err),
			zap.String("provider", providerName),
			zap.String("payment_intent_id", pi.ID))
		return
	}

	// Double-check: may have been resolved by a webhook while we were querying
	if currentStatus == "succeeded" || currentStatus == "failed" || currentStatus == "refunded" || currentStatus == "expired" {
		logger.Info("Payment intent already resolved before inquiry update",
			zap.String("provider", providerName),
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
				logger.Warn("Duplicate deposit credit detected by inquiry worker",
					zap.String("provider", providerName),
					zap.String("payment_intent_id", pi.ID))
			} else {
				logger.Error("Failed to credit wallet from inquiry",
					zap.Error(err),
					zap.String("provider", providerName),
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
		logger.Error("Failed to update payment intent from inquiry",
			zap.Error(err),
			zap.String("provider", providerName),
			zap.String("payment_intent_id", pi.ID))
		return
	}

	if err := tx.Commit(); err != nil {
		logger.Error("Failed to commit inquiry transaction",
			zap.Error(err),
			zap.String("provider", providerName),
			zap.String("payment_intent_id", pi.ID))
		return
	}

	logger.Info("Inquiry worker resolved payment intent",
		zap.String("provider", providerName),
		zap.String("payment_intent_id", pi.ID),
		zap.String("status", intentStatus))
}

// mapProviderStatusToIntentStatus maps provider payment status to payment intent status.
// Delegates to providers.MapStatusToIntentStatus for a single source of truth.
func mapProviderStatusToIntentStatus(status providers.PaymentStatus) string {
	return providers.MapStatusToIntentStatus(status)
}
