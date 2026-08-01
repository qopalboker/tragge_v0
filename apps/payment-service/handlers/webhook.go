package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"go.uber.org/zap"
)

// ParseCIDRs parses a list of CIDR strings into net.IPNet values.
// Invalid CIDRs are logged and skipped.
func ParseCIDRs(cidrs []string, logger *zap.Logger) []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Try as a single IP (add /32 or /128)
			ip := net.ParseIP(cidr)
			if ip == nil {
				logger.Warn("Invalid CIDR in whitelist, skipping",
					zap.String("cidr", cidr), zap.Error(err))
				continue
			}
			if ip.To4() != nil {
				_, ipNet, _ = net.ParseCIDR(cidr + "/32")
			} else {
				_, ipNet, _ = net.ParseCIDR(cidr + "/128")
			}
		}
		if ipNet != nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}

// IPWhitelistMiddleware returns an HTTP middleware that rejects requests from
// IPs not in the allowed CIDR list. If allowedCIDRs is empty, all requests
// are allowed (with a warning log on the first request).
func IPWhitelistMiddleware(allowedCIDRs []*net.IPNet, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowedCIDRs) == 0 {
				// No whitelist configured — allow through (graceful degradation)
				next.ServeHTTP(w, r)
				return
			}

			clientIP := extractClientIP(r)
			ip := net.ParseIP(clientIP)
			if ip == nil {
				logger.Warn("Could not parse client IP for whitelist check",
					zap.String("raw_ip", clientIP))
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			for _, cidr := range allowedCIDRs {
				if cidr.Contains(ip) {
					next.ServeHTTP(w, r)
					return
				}
			}

			logger.Warn("Webhook request from non-whitelisted IP",
				zap.String("client_ip", clientIP),
				zap.String("path", r.URL.Path))
			http.Error(w, "Forbidden", http.StatusForbidden)
		})
	}
}

// extractClientIP extracts the client IP from the request using the
// shared trusted-proxy-aware helper.
func extractClientIP(r *http.Request) string {
	return validation.ExtractClientIP(r)
}

// WebhookHandler handles payment provider webhooks
type WebhookHandler struct {
	db                 *sql.DB
	walletService      *wallet.Service
	registry           *providers.ProviderRegistry
	emailNotifier      *notification.EmailNotifier
	logger             *zap.Logger
	successRedirectURL string
	cancelRedirectURL  string
	circuits           DatabaseCircuitExecutor
	security           *WebhookSecurity
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler(db *sql.DB, walletService *wallet.Service, registry *providers.ProviderRegistry, emailNotifier *notification.EmailNotifier, logger *zap.Logger, successRedirectURL, cancelRedirectURL string, circuits DatabaseCircuitExecutor, security ...*WebhookSecurity) *WebhookHandler {
	var webhookSecurity *WebhookSecurity
	if len(security) > 0 {
		webhookSecurity = security[0]
	}
	return &WebhookHandler{
		db:                 db,
		walletService:      walletService,
		registry:           registry,
		emailNotifier:      emailNotifier,
		logger:             logger,
		successRedirectURL: successRedirectURL,
		cancelRedirectURL:  cancelRedirectURL,
		circuits:           circuits,
		security:           webhookSecurity,
	}
}

// HandleNowPaymentsWebhook handles POST /webhooks/nowpayments
func (h *WebhookHandler) HandleNowPaymentsWebhook(w http.ResponseWriter, r *http.Request) {
	h.handleWebhook(w, r, providers.ProviderNowPayments)
}

// handleWebhook is the common webhook handling logic
func (h *WebhookHandler) handleWebhook(w http.ResponseWriter, r *http.Request, providerType providers.ProviderType) {
	ctx := r.Context()

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read webhook body", zap.Error(err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get headers — normalize keys to lowercase so providers can use
	// consistent lowercase lookups (e.g. "x-nowpayments-sig").
	headers := make(map[string]string)
	for key := range r.Header {
		headers[strings.ToLower(key)] = r.Header.Get(key)
	}

	h.logger.Info("Received webhook",
		zap.String("provider", string(providerType)),
		zap.Int("body_length", len(body)))

	// Get provider
	provider, ok := h.registry.Get(providerType)
	if !ok {
		h.logger.Error("Provider not found", zap.String("provider", string(providerType)))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Verify and parse webhook
	event, err := provider.VerifyWebhook(ctx, headers, body)
	if err != nil {
		h.logger.Warn("Webhook verification failed",
			zap.Error(err),
			zap.String("provider", string(providerType)))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if h.security == nil {
		h.logger.Warn("Webhook security policy unavailable", zap.String("provider", string(providerType)))
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	if err := h.security.Validate(ctx, providerType, headers, body, event); err != nil {
		h.logger.Warn("Webhook security policy rejected request",
			zap.String("provider", string(providerType)),
			zap.String("reason", "freshness_or_replay"))
		if errors.Is(err, errWebhookStore) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, errWebhookReplay) {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	h.logger.Info("Webhook verified",
		zap.String("provider", string(providerType)),
		zap.String("provider_payment_id", event.ProviderPaymentID),
		zap.String("order_id", event.OrderID),
		zap.String("status", string(event.Status)))

	// Process the webhook event
	if err := h.processWebhookEvent(ctx, event); err != nil {
		h.logger.Error("Failed to process webhook event",
			zap.Error(err),
			zap.String("provider", string(providerType)),
			zap.String("order_id", event.OrderID))
		// Return 200 anyway to prevent retries for business logic errors
	}

	// Return success
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// processWebhookEvent processes a verified webhook event atomically.
//
// All operations (status check, wallet credit, status update) happen within a
// single database transaction with SELECT FOR UPDATE to prevent race conditions
// where concurrent webhook retries could credit a wallet twice.
func (h *WebhookHandler) processWebhookEvent(ctx context.Context, event *providers.WebhookEvent) error {
	// Begin transaction for atomic processing (protected by circuit breaker)
	var tx *sql.Tx
	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var e error
		tx, e = h.db.BeginTx(ctx, nil)
		return e
	})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Lock and read payment intent atomically with FOR UPDATE
	var paymentIntent struct {
		ID           string
		UserID       string
		Status       string
		AmountCents  int64
		Currency     string
		Provider     string
		MetadataJSON sql.NullString
	}

	if event.OrderID != "" {
		err = tx.QueryRowContext(ctx, `
			SELECT id, user_id, status, amount_cents, currency, provider, metadata_json
			FROM payment_intents
			WHERE id = $1
			FOR UPDATE
		`, event.OrderID).Scan(&paymentIntent.ID, &paymentIntent.UserID, &paymentIntent.Status, &paymentIntent.AmountCents, &paymentIntent.Currency, &paymentIntent.Provider, &paymentIntent.MetadataJSON)
	} else {
		err = tx.QueryRowContext(ctx, `
			SELECT id, user_id, status, amount_cents, currency, provider, metadata_json
			FROM payment_intents
			WHERE provider_payment_id = $1 AND provider = $2
			FOR UPDATE
		`, event.ProviderPaymentID, string(event.Provider)).Scan(&paymentIntent.ID, &paymentIntent.UserID, &paymentIntent.Status, &paymentIntent.AmountCents, &paymentIntent.Currency, &paymentIntent.Provider, &paymentIntent.MetadataJSON)
	}

	if err != nil {
		if err == sql.ErrNoRows {
			h.logger.Warn("Payment intent not found for webhook",
				zap.String("order_id", event.OrderID),
				zap.String("provider_payment_id", event.ProviderPaymentID))
			return nil // Not an error - might be duplicate or old webhook
		}
		return err
	}

	// Skip if already processed — idempotent success
	if paymentIntent.Status == "succeeded" || paymentIntent.Status == "failed" || paymentIntent.Status == "refunded" || paymentIntent.Status == "expired" {
		h.logger.Info("Duplicate webhook detected: payment already processed",
			zap.String("payment_intent_id", paymentIntent.ID),
			zap.String("provider", string(event.Provider)),
			zap.String("current_status", paymentIntent.Status))
		return nil
	}

	// Amount verification — compare the webhook amount against the stored
	// payment intent amount as defense-in-depth against spoofed callbacks.
	if event.AmountCents > 0 {
		if err := h.verifyWebhookAmount(event, paymentIntent.AmountCents, paymentIntent.MetadataJSON); err != nil {
			h.logger.Error("Webhook amount verification failed",
				zap.Error(err),
				zap.String("payment_intent_id", paymentIntent.ID),
				zap.String("provider", string(event.Provider)),
				zap.Int64("event_amount", event.AmountCents),
				zap.Int64("intent_amount", paymentIntent.AmountCents))
			return err
		}
	}

	// Check exchange rate staleness for fiat deposits — block auto-credit if too stale
	if paymentIntent.Provider == "jibit" && paymentIntent.MetadataJSON.Valid {
		var metadata map[string]interface{}
		if json.Unmarshal([]byte(paymentIntent.MetadataJSON.String), &metadata) == nil {
			if rateFetchedStr, ok := metadata["rate_fetched"].(string); ok {
				if rateFetched, parseErr := time.Parse(time.RFC3339, rateFetchedStr); parseErr == nil {
					staleness := time.Since(rateFetched)
					// Hard block: if rate is older than max staleness, do NOT auto-credit.
					// The inquiry worker or admin can manually resolve these.
					maxStaleness := 6 * time.Hour
					if maxStaleEnv := os.Getenv("MAX_EXCHANGE_RATE_STALENESS"); maxStaleEnv != "" {
						if d, err := time.ParseDuration(maxStaleEnv); err == nil {
							maxStaleness = d
						}
					}
					if staleness > maxStaleness {
						h.logger.Error("Exchange rate too stale — blocking auto-credit, requires manual review",
							zap.Duration("staleness", staleness),
							zap.Duration("max_staleness", maxStaleness),
							zap.String("payment_intent_id", paymentIntent.ID),
							zap.Any("exchange_rate", metadata["exchange_rate"]))
						return fmt.Errorf("exchange rate staleness %v exceeds maximum %v", staleness, maxStaleness)
					}
					if staleness > 30*time.Minute {
						h.logger.Warn("Exchange rate may be stale at webhook time",
							zap.Duration("staleness", staleness),
							zap.String("payment_intent_id", paymentIntent.ID),
							zap.Any("exchange_rate", metadata["exchange_rate"]))
					}
				}
			}
		}
	}

	// Map provider status to intent status
	newStatus := mapProviderStatusToIntent(event.Status)

	// If payment succeeded, credit wallet within the SAME transaction
	if event.Status == providers.PaymentStatusFinished || event.Status == providers.PaymentStatusConfirmed {
		txWrapper := &wallet.TxAdapter{Tx: tx}
		if err := h.creditWallet(ctx, txWrapper, paymentIntent.ID, paymentIntent.UserID, paymentIntent.AmountCents); err != nil {
			h.logger.Error("Failed to credit wallet",
				zap.Error(err),
				zap.String("payment_intent_id", paymentIntent.ID),
				zap.String("user_id", paymentIntent.UserID))
			return err
		}
		newStatus = "succeeded"

		// Log exchange rate info for fiat deposits (audit trail)
		if paymentIntent.Provider == "jibit" && paymentIntent.MetadataJSON.Valid {
			var metadata map[string]interface{}
			if json.Unmarshal([]byte(paymentIntent.MetadataJSON.String), &metadata) == nil {
				if rate, ok := metadata["exchange_rate"]; ok {
					h.logger.Info("Fiat deposit confirmed with exchange rate",
						zap.Any("exchange_rate", rate),
						zap.Int64("amount_usd_cents", paymentIntent.AmountCents),
						zap.Any("amount_irr", metadata["amount_irr"]),
						zap.Any("rate_source", metadata["rate_source"]))
				}
			}
		}

	}

	// Update payment intent status within the same transaction
	var completedAt interface{}
	if newStatus == "succeeded" || newStatus == "failed" || newStatus == "refunded" || newStatus == "expired" {
		completedAt = time.Now()
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE payment_intents
		SET status = $1, completed_at = $2, updated_at = $3
		WHERE id = $4
	`, newStatus, completedAt, time.Now(), paymentIntent.ID)
	if err != nil {
		h.logger.Error("Failed to update payment intent",
			zap.Error(err),
			zap.String("payment_intent_id", paymentIntent.ID))
		return err
	}

	// Commit the entire operation atomically
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit webhook transaction: %w", err)
	}

	h.logger.Info("Payment intent updated",
		zap.String("payment_intent_id", paymentIntent.ID),
		zap.String("status", newStatus))

	// Create in-app notifications after commit (best-effort, non-blocking)
	if newStatus == "succeeded" {
		// Send deposit confirmation email (non-blocking)
		infra.SafeGo(h.logger, "deposit-confirmation-email", func() {
			h.sendDepositConfirmationEmail(context.Background(), paymentIntent.UserID, paymentIntent.ID, paymentIntent.AmountCents, paymentIntent.Currency)
		})
		infra.SafeGo(h.logger, "deposit-confirmed-notification", func() {
			notifCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			enabled, _ := prefs.IsEnabled(notifCtx, h.db, paymentIntent.UserID, inapp.NotifTypeDepositConfirmed, "in_app")
			if !enabled {
				return
			}

			// Fetch current wallet balance for notification metadata
			var newBalanceCents int64
			_ = h.db.QueryRowContext(notifCtx, `SELECT balance_cents FROM wallets WHERE user_id = $1`, paymentIntent.UserID).Scan(&newBalanceCents)

			if err := inapp.CreateDepositConfirmedNotification(notifCtx, h.db, paymentIntent.UserID, paymentIntent.AmountCents, paymentIntent.Currency, paymentIntent.Provider, paymentIntent.ID, newBalanceCents); err != nil {
				h.logger.Error("Failed to create deposit success notification",
					zap.Error(err),
					zap.String("user_id", paymentIntent.UserID),
					zap.String("payment_intent_id", paymentIntent.ID))
			}
		})
	} else if newStatus == "failed" {
		infra.SafeGo(h.logger, "deposit-failed-notification", func() {
			notifCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			enabled, _ := prefs.IsEnabled(notifCtx, h.db, paymentIntent.UserID, inapp.NotifTypeDepositFailed, "in_app")
			if !enabled {
				return
			}

			if err := inapp.CreateDepositFailedNotification(notifCtx, h.db, paymentIntent.UserID, paymentIntent.AmountCents, paymentIntent.Currency, paymentIntent.Provider, paymentIntent.ID); err != nil {
				h.logger.Error("Failed to create deposit failure notification",
					zap.Error(err),
					zap.String("user_id", paymentIntent.UserID),
					zap.String("payment_intent_id", paymentIntent.ID))
			}
		})
	}

	return nil
}

// creditWallet credits the user's wallet with the deposit amount using an
// idempotency key for double-credit protection. It accepts an external
// TxExecutor so the credit is part of the caller's transaction.
func (h *WebhookHandler) creditWallet(ctx context.Context, tx wallet.TxExecutor, paymentIntentID, userID string, amountCents int64) error {
	refType := wallet.LedgerRefTypePaymentIntent
	idempotencyKey := fmt.Sprintf("deposit:%s", paymentIntentID)

	_, err := h.walletService.CreditIdempotent(ctx, tx, userID, amountCents,
		wallet.LedgerTypeDeposit, &refType, &paymentIntentID, nil, idempotencyKey)
	if err != nil {
		// DuplicateCreditError means the ledger entry already exists — this is
		// a second layer of defense. Log it but treat as success.
		if dupErr, ok := err.(*wallet.DuplicateCreditError); ok {
			h.logger.Warn("Duplicate deposit credit detected via idempotency key",
				zap.String("payment_intent_id", paymentIntentID),
				zap.String("idempotency_key", dupErr.IdempotencyKey))
			return nil
		}
		return err
	}

	return nil
}

// verifyWebhookAmount checks that the amount from the webhook matches what we
// expect. For Jibit (IRR), compares against stored IRR metadata for USD→IRR
// conversions. For all providers, compares event amount against intent amount.
// A 1% tolerance is applied to handle minor rounding differences.
func (h *WebhookHandler) verifyWebhookAmount(event *providers.WebhookEvent, intentAmountCents int64, metadataJSON sql.NullString) error {
	// Jibit-specific: check IRR amount from metadata for USD→IRR conversions
	if event.Provider == providers.ProviderJibit && metadataJSON.Valid {
		var metadata map[string]interface{}
		if json.Unmarshal([]byte(metadataJSON.String), &metadata) == nil {
			if amountIRR, ok := metadata["amount_irr"]; ok {
				var expectedIRR int64
				switch v := amountIRR.(type) {
				case float64:
					expectedIRR = int64(v)
				case json.Number:
					if n, err := v.Int64(); err == nil {
						expectedIRR = n
					}
				}
				if expectedIRR > 0 {
					if !amountsMatch(event.AmountCents, expectedIRR) {
						return fmt.Errorf("webhook amount mismatch (%s): received %d, expected %d (IRR from metadata)",
							event.Provider, event.AmountCents, expectedIRR)
					}
					return nil
				}
			}
		}
	}

	// General verification for all providers: compare webhook amount vs stored intent amount
	if !amountsMatch(event.AmountCents, intentAmountCents) {
		return fmt.Errorf("webhook amount mismatch (%s): received %d cents, expected %d cents",
			event.Provider, event.AmountCents, intentAmountCents)
	}
	return nil
}

// amountsMatch returns true if two amounts are within tolerance of each other.
// Uses the smaller of 1% relative tolerance or an absolute cap to prevent
// large absolute differences on high-value transactions.
func amountsMatch(actual, expected int64) bool {
	if actual == expected {
		return true
	}
	if expected == 0 {
		return actual == 0
	}
	diff := math.Abs(float64(actual - expected))
	pctTolerance := math.Abs(float64(expected)) * 0.01 // 1% relative
	// Absolute cap: 1000 cents (10 USD / ~500,000 IRR) to prevent
	// large absolute differences on high-value transactions.
	const absoluteCapCents int64 = 1000
	absCap := float64(absoluteCapCents)
	tolerance := math.Min(pctTolerance, absCap)
	return diff <= tolerance
}

// HandleJibitCallback handles Jibit redirect callbacks after payment on PSP page.
// Jibit POSTs purchaseId, amount, status to the callbackUrl as form-urlencoded.
// This also processes the webhook verification inline.
func (h *WebhookHandler) HandleJibitCallback(w http.ResponseWriter, r *http.Request) {
	// Read body for POST callbacks
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("Failed to read Jibit callback body", zap.Error(err))
		body = nil
	}

	// Extract purchaseId from body or query params
	purchaseID := r.URL.Query().Get("purchaseId")
	status := r.URL.Query().Get("status")

	if len(body) > 0 {
		contentType := r.Header.Get("Content-Type")
		if strings.Contains(contentType, "application/x-www-form-urlencoded") {
			// Jibit PPG v3 sends callbacks as form-urlencoded
			if values, err := url.ParseQuery(string(body)); err == nil {
				if v := values.Get("purchaseId"); v != "" {
					purchaseID = v
				}
				if v := values.Get("status"); v != "" {
					status = v
				}
			}
		} else {
			// Fallback to JSON parsing
			var callbackData struct {
				PurchaseID interface{} `json:"purchaseId"`
				Status     string      `json:"status"`
			}
			if json.Unmarshal(body, &callbackData) == nil {
				if callbackData.PurchaseID != nil {
					purchaseID = fmt.Sprintf("%v", callbackData.PurchaseID)
				}
				if callbackData.Status != "" {
					status = callbackData.Status
				}
			}
		}
	}

	h.logger.Info("Jibit callback received",
		zap.String("purchase_id", purchaseID),
		zap.String("status", status))

	// Process as webhook if we have body data
	if len(body) > 0 {
		// Normalize headers to lowercase for consistent provider lookups
		headers := make(map[string]string)
		for key := range r.Header {
			headers[strings.ToLower(key)] = r.Header.Get(key)
		}

		provider, ok := h.registry.Get(providers.ProviderJibit)
		if ok {
			event, err := provider.VerifyWebhook(r.Context(), headers, body)
			if err != nil {
				h.logger.Warn("Jibit callback verification failed", zap.Error(err))
			} else {
				if err := h.processWebhookEvent(r.Context(), event); err != nil {
					h.logger.Error("Failed to process Jibit callback event", zap.Error(err))
				}
			}
		}
	}

	// If no body was processed (pure GET redirect), verify status server-side
	// before deciding redirect URL to prevent spoofed success redirects.
	if len(body) == 0 && purchaseID != "" {
		provider, ok := h.registry.Get(providers.ProviderJibit)
		if ok {
			statusResp, err := provider.GetPaymentStatus(r.Context(), purchaseID)
			if err != nil {
				h.logger.Warn("Failed to verify Jibit payment status for redirect",
					zap.Error(err),
					zap.String("purchase_id", purchaseID))
				// Default to cancel redirect on verification failure
				status = "FAILED"
			} else {
				mappedStatus := providers.MapStatusToIntentStatus(statusResp.Status)
				if mappedStatus == "succeeded" {
					status = "SUCCESSFUL"
				} else {
					status = "FAILED"
				}
			}
		}
	}

	// Redirect to frontend with status using configured redirect URLs
	var redirectURL string
	if status == "SUCCESSFUL" {
		redirectURL = h.successRedirectURL
	} else {
		redirectURL = h.cancelRedirectURL
	}

	// Append purchase_id query param to the configured URL
	// URL-encode purchaseID to prevent query parameter injection
	escapedPurchaseID := url.QueryEscape(purchaseID)
	if redirectURL != "" {
		if strings.Contains(redirectURL, "?") {
			redirectURL += "&purchase_id=" + escapedPurchaseID
		} else {
			redirectURL += "?purchase_id=" + escapedPurchaseID
		}
	} else {
		// Fallback to relative path if redirect URLs are not configured
		redirectStatus := "0"
		if status == "SUCCESSFUL" {
			redirectStatus = "1"
		}
		redirectURL = "/payment/result?status=" + redirectStatus + "&purchase_id=" + escapedPurchaseID
	}
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// sendDepositConfirmationEmail sends a deposit confirmation email to the user
func (h *WebhookHandler) sendDepositConfirmationEmail(ctx context.Context, userID, transactionID string, amountCents int64, currency string) {
	// Skip if email notifier is not configured
	if h.emailNotifier == nil {
		h.logger.Debug("Email notifier not configured, skipping deposit confirmation email")
		return
	}

	// Create a new context with timeout for email operations
	emailCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get user email
	var email string
	var userName sql.NullString
	err := h.circuits.ExecuteDatabase(emailCtx, func(ctx context.Context) error {
		return h.db.QueryRowContext(ctx, `
			SELECT email, display_name FROM users WHERE id = $1
		`, userID).Scan(&email, &userName)
	})
	if err != nil {
		h.logger.Error("Failed to get user email for deposit confirmation",
			zap.Error(err),
			zap.String("user_id", userID))
		return
	}

	// Get new wallet balance
	var balanceCents int64
	err = h.circuits.ExecuteDatabase(emailCtx, func(ctx context.Context) error {
		return h.db.QueryRowContext(ctx, `
			SELECT balance_cents FROM wallets WHERE user_id = $1
		`, userID).Scan(&balanceCents)
	})
	if err != nil {
		h.logger.Warn("Failed to get wallet balance for deposit confirmation",
			zap.Error(err),
			zap.String("user_id", userID))
		balanceCents = 0 // Continue with zero if balance can't be fetched
	}

	// Format amounts with currency-aware formatting
	amount := inapp.FormatAmount(amountCents, currency)
	newBalance := inapp.FormatAmount(balanceCents, currency)

	// Get wallet URL from environment or use default
	walletURL := os.Getenv("WALLET_URL")
	if walletURL == "" {
		walletURL = os.Getenv("FRONTEND_URL")
		if walletURL != "" {
			walletURL = walletURL + "/wallet"
		}
	}

	// Prepare email data
	emailData := notification.DepositConfirmedData{
		Amount:        amount,
		NewBalance:    newBalance,
		Date:          time.Now().UTC().Format("January 2, 2006 at 3:04 PM UTC"),
		TransactionID: transactionID,
		WalletURL:     walletURL,
	}

	// Add user name if available
	if userName.Valid && userName.String != "" {
		emailData.UserName = userName.String
	}

	// Send the email
	if err := h.emailNotifier.SendDepositConfirmed(emailCtx, email, emailData); err != nil {
		h.logger.Error("Failed to send deposit confirmation email",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("email", email))
		return
	}

	h.logger.Info("Deposit confirmation email sent",
		zap.String("user_id", userID),
		zap.String("email", email),
		zap.String("amount", amount))
}
