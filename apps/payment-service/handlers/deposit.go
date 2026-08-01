package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/wallet/exchangerate"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// DatabaseCircuitExecutor wraps database operations through a circuit breaker.
// Satisfied by *CircuitBreakers from the main package.
type DatabaseCircuitExecutor interface {
	ExecuteDatabase(ctx context.Context, fn func(ctx context.Context) error) error
}

// DepositHandler handles deposit-related requests
type DepositHandler struct {
	db           *sql.DB
	registry     *providers.ProviderRegistry
	exchangeRate *exchangerate.Service
	logger       *zap.Logger
	config       *DepositConfig
	circuits     DatabaseCircuitExecutor
	metrics      PaymentMetricsObserver
}

// DepositConfig holds configuration for deposit operations
type DepositConfig struct {
	MinDepositCents    int64
	MaxDepositCents    int64
	MinDepositIRR      int64
	MaxDepositIRR      int64
	DefaultCurrency    string
	WebhookBaseURL     string
	SuccessRedirectURL string
	CancelRedirectURL  string
}

// NewDepositHandler creates a new deposit handler
func NewDepositHandler(db *sql.DB, registry *providers.ProviderRegistry, exchangeRateSvc *exchangerate.Service, logger *zap.Logger, config *DepositConfig, circuits DatabaseCircuitExecutor, metrics PaymentMetricsObserver) *DepositHandler {
	return &DepositHandler{
		db:           db,
		registry:     registry,
		exchangeRate: exchangeRateSvc,
		logger:       logger,
		config:       config,
		circuits:     circuits,
		metrics:      metrics,
	}
}

// CreateDepositRequest represents the request body for creating a deposit
type CreateDepositRequest struct {
	AmountCents    int64  `json:"amount_cents"`
	AmountUSDCents int64  `json:"amount_usd_cents,omitempty"` // USD amount for fiat gateway (auto-converts to IRR)
	Currency       string `json:"currency,omitempty"`
	Provider       string `json:"provider"`                      // nowpayments, jibit
	PayCurrency    string `json:"pay_currency,omitempty"`        // For crypto: BTC, ETH, etc.
	CustomerPhone  string `json:"customer_phone,omitempty"`
}

// CreateDepositResponse represents the response for creating a deposit
type CreateDepositResponse struct {
	PaymentIntentID string            `json:"payment_intent_id"`
	PaymentURL      string            `json:"payment_url"`
	PayAddress      string            `json:"pay_address,omitempty"`
	PayAmount       float64           `json:"pay_amount,omitempty"`
	PayCurrency     string            `json:"pay_currency,omitempty"`
	QRCode          string            `json:"qr_code,omitempty"`
	ExpiresAt       int64             `json:"expires_at,omitempty"`
	Status          string            `json:"status"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// HandleCreateCryptoDeposit handles POST /api/payments/deposit/crypto/create
func (h *DepositHandler) HandleCreateCryptoDeposit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var req CreateDepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate provider for crypto deposits
	if req.Provider == "" {
		req.Provider = "nowpayments" // Default for backwards compatibility
	}
	if req.Provider != "nowpayments" && req.Provider != "payment4" {
		writeErrorJSON(w, http.StatusBadRequest, "invalid provider, must be nowpayments or payment4")
		return
	}

	// Only validate AllowedCryptoCurrencies when provider is nowpayments.
	// Payment4 handles currency selection on their hosted payment page.
	if req.Provider == "nowpayments" {
		if req.PayCurrency == "" {
			req.PayCurrency = "usdttrc20"
		}
		if _, ok := providers.AllowedCryptoCurrencies[strings.ToLower(req.PayCurrency)]; !ok {
			writeErrorJSON(w, http.StatusBadRequest, "invalid crypto currency. Supported: USDT TRC20 (usdttrc20), TRX (trx)")
			return
		}
		req.PayCurrency = strings.ToLower(req.PayCurrency)
	}

	h.handleCreateDeposit(w, r, userID, &req)
}

// HandleGetEstimate handles GET /api/payments/estimate?amount=50&currency=usdttrc20
func (h *DepositHandler) HandleGetEstimate(w http.ResponseWriter, r *http.Request) {
	// Payment4 does not support estimates — crypto conversion is handled on their hosted page
	if r.URL.Query().Get("provider") == "payment4" {
		writeErrorJSON(w, http.StatusBadRequest, "estimates not supported for payment4 provider")
		return
	}

	amountStr := r.URL.Query().Get("amount")
	currency := r.URL.Query().Get("currency")

	if amountStr == "" {
		writeErrorJSON(w, http.StatusBadRequest, "amount query parameter is required")
		return
	}

	var amountUSD float64
	if _, err := fmt.Sscanf(amountStr, "%f", &amountUSD); err != nil || amountUSD <= 0 {
		writeErrorJSON(w, http.StatusBadRequest, "amount must be a positive number")
		return
	}

	if currency == "" {
		currency = "usdttrc20"
	}
	currency = strings.ToLower(currency)
	if _, ok := providers.AllowedCryptoCurrencies[currency]; !ok {
		writeErrorJSON(w, http.StatusBadRequest, "invalid crypto currency. Supported: USDT TRC20 (usdttrc20), TRX (trx)")
		return
	}

	// Get the NowPayments provider
	provider, ok := h.registry.Get(providers.ProviderNowPayments)
	if !ok {
		writeErrorJSON(w, http.StatusServiceUnavailable, "crypto payment provider not configured")
		return
	}

	np, ok := provider.(*providers.NowPayments)
	if !ok {
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	estimate, err := np.GetEstimate(r.Context(), amountUSD, currency)
	if err != nil {
		h.logger.Error("Failed to get estimate", zap.Error(err))
		writeErrorJSON(w, http.StatusBadGateway, "failed to get estimate from provider")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"currency_from":    estimate.CurrencyFrom,
		"amount_from":      estimate.AmountFrom,
		"currency_to":      estimate.CurrencyTo,
		"estimated_amount": estimate.EstimatedAmount,
	})
}

// HandleCreateFiatDeposit handles POST /api/payments/deposit/fiat/create
//
// Supports two modes for Jibit (fiat) deposits:
//   - amount_usd_cents: User specifies USD amount. System converts to IRR using
//     real-time exchange rate and sends IRR to Jibit. Wallet credits in USD.
//   - amount_cents: Legacy mode. Amount is sent directly to Jibit as-is.
func (h *DepositHandler) HandleCreateFiatDeposit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var req CreateDepositRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate provider is a fiat provider
	if req.Provider != "jibit" {
		writeErrorJSON(w, http.StatusBadRequest, "invalid provider, must be jibit")
		return
	}

	// Validate that at least one amount field is provided
	if req.AmountUSDCents <= 0 && req.AmountCents <= 0 {
		writeErrorJSON(w, http.StatusBadRequest, "either amount_usd_cents or amount_cents is required and must be positive")
		return
	}

	// If AmountUSDCents is provided, convert USD to IRR for Jibit
	if req.AmountUSDCents > 0 && h.exchangeRate != nil {
		rate, err := h.exchangeRate.GetRate(ctx)
		if err != nil {
			h.logger.Error("Failed to get exchange rate for fiat deposit", zap.Error(err))
			writeErrorJSON(w, http.StatusServiceUnavailable, "نرخ ارز موقتاً در دسترس نیست")
			return
		}

		// Convert USD cents to IRR for Jibit gateway
		amountIRR := exchangerate.ConvertUSDToIRR(req.AmountUSDCents, rate)

		h.logger.Info("Fiat deposit USD→IRR conversion",
			zap.Int64("amount_usd_cents", req.AmountUSDCents),
			zap.Int64("amount_irr", amountIRR),
			zap.Float64("exchange_rate", rate.USDToIRR),
			zap.String("rate_source", rate.Source))

		// Store the USD amount as what will credit the wallet
		req.AmountCents = req.AmountUSDCents
		req.Currency = "USD"

		h.handleCreateFiatDepositWithConversion(w, r, userID, &req, amountIRR, rate)
		return
	}

	h.handleCreateDeposit(w, r, userID, &req)
}

// handleCreateDeposit is the common deposit creation logic
func (h *DepositHandler) handleCreateDeposit(w http.ResponseWriter, r *http.Request, userID string, req *CreateDepositRequest) {
	ctx := r.Context()

	// Currency-aware amount validation: use IRR limits for Jibit provider,
	// USD cents limits for everything else.
	v := validation.New()
	if req.Provider == string(providers.ProviderJibit) && h.config.MinDepositIRR > 0 {
		// Jibit sends amounts in Rials — validate against IRR limits
		if req.AmountCents < h.config.MinDepositIRR {
			v.AddError("amount_cents", "min_deposit", "amount is below minimum")
		}
		if h.config.MaxDepositIRR > 0 && req.AmountCents > h.config.MaxDepositIRR {
			v.AddError("amount_cents", "max_deposit", "amount exceeds maximum")
		}
	} else {
		if req.AmountCents < h.config.MinDepositCents {
			v.AddError("amount_cents", "min_deposit", "amount is below minimum")
		}
		if req.AmountCents > h.config.MaxDepositCents {
			v.AddError("amount_cents", "max_deposit", "amount exceeds maximum")
		}
	}
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Set default currency
	currency := req.Currency
	if currency == "" {
		currency = h.config.DefaultCurrency
	}

	// Get provider
	providerType := providers.ProviderType(req.Provider)
	provider, ok := h.registry.Get(providerType)
	if !ok {
		writeErrorJSON(w, http.StatusBadRequest, "unsupported provider")
		return
	}

	// Check if provider is available
	if !provider.IsAvailable(ctx) {
		writeErrorJSON(w, http.StatusServiceUnavailable, "درگاه پرداخت موقتاً در دسترس نیست")
		return
	}

	// Create payment intent in database
	paymentIntentID := uuid.New().String()
	now := time.Now()

	metadataJSON, _ := json.Marshal(map[string]string{
		"provider":     req.Provider,
		"pay_currency": req.PayCurrency,
	})

	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, e := h.db.ExecContext(ctx, `
			INSERT INTO payment_intents (id, user_id, provider, amount_cents, currency, status, metadata_json, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $7)
		`, paymentIntentID, userID, req.Provider, req.AmountCents, currency, metadataJSON, now)
		return e
	})
	if err != nil {
		h.logger.Error("Failed to create payment intent", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Create payment with provider
	providerReq := &providers.CreatePaymentRequest{
		AmountCents:    req.AmountCents,
		Currency:       currency,
		UserID:         userID,
		OrderID:        paymentIntentID,
		Description:    "Deposit to Tragge Wallet",
		CallbackURL:    h.config.SuccessRedirectURL,
		CancelURL:      h.config.CancelRedirectURL,
		IPNCallbackURL: h.config.WebhookBaseURL + "/webhooks/" + req.Provider,
		PayCurrency:    req.PayCurrency,
		CustomerPhone:  req.CustomerPhone,
	}

	providerResp, err := provider.CreatePayment(ctx, providerReq)
	if err != nil {
		h.logger.Error("Failed to create payment with provider",
			zap.Error(err),
			zap.String("provider", req.Provider),
			zap.String("payment_intent_id", paymentIntentID))

		// Update payment intent status to failed
		_ = h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
			_, e := h.db.ExecContext(ctx, `
				UPDATE payment_intents SET status = 'failed', updated_at = $1 WHERE id = $2
			`, time.Now(), paymentIntentID)
			return e
		})

		writeErrorJSON(w, http.StatusBadGateway, "failed to create payment with provider")
		return
	}

	// Record successful payment creation metric
	if providerType == providers.ProviderPayment4 && h.metrics != nil {
		h.metrics.OnPayment4Created()
	}

	// Update payment intent with provider payment ID
	err = h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, e := h.db.ExecContext(ctx, `
			UPDATE payment_intents
			SET provider_payment_id = $1, status = 'processing', updated_at = $2
			WHERE id = $3
		`, providerResp.ProviderPaymentID, time.Now(), paymentIntentID)
		return e
	})
	if err != nil {
		h.logger.Error("Failed to update payment intent", zap.Error(err))
	}

	writeJSON(w, http.StatusCreated, CreateDepositResponse{
		PaymentIntentID: paymentIntentID,
		PaymentURL:      providerResp.PaymentURL,
		PayAddress:      providerResp.PayAddress,
		PayAmount:       providerResp.PayAmount,
		PayCurrency:     providerResp.PayCurrency,
		QRCode:          providerResp.QRCode,
		ExpiresAt:       providerResp.ExpiresAt,
		Status:          string(providerResp.Status),
		Metadata:        providerResp.Metadata,
	})
}

// handleCreateFiatDepositWithConversion handles fiat deposits where USD is
// converted to IRR. The payment_intent stores USD cents (for wallet credit),
// while the Jibit provider receives the IRR amount.
func (h *DepositHandler) handleCreateFiatDepositWithConversion(
	w http.ResponseWriter, r *http.Request,
	userID string, req *CreateDepositRequest,
	amountIRR int64, rate *exchangerate.Rate,
) {
	ctx := r.Context()

	// Validate USD amount
	v := validation.New()
	if req.AmountCents < h.config.MinDepositCents {
		v.AddError("amount_usd_cents", "min_deposit", "amount is below minimum")
	}
	if req.AmountCents > h.config.MaxDepositCents {
		v.AddError("amount_usd_cents", "max_deposit", "amount exceeds maximum")
	}
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Get provider
	providerType := providers.ProviderType(req.Provider)
	provider, ok := h.registry.Get(providerType)
	if !ok {
		writeErrorJSON(w, http.StatusBadRequest, "unsupported provider")
		return
	}
	if !provider.IsAvailable(ctx) {
		writeErrorJSON(w, http.StatusServiceUnavailable, "درگاه پرداخت موقتاً در دسترس نیست")
		return
	}

	// Create payment intent — stores USD amount (what credits the wallet)
	paymentIntentID := uuid.New().String()
	now := time.Now()

	metadataJSON, _ := json.Marshal(map[string]interface{}{
		"provider":      req.Provider,
		"amount_irr":    amountIRR,
		"exchange_rate": rate.USDToIRR,
		"rate_source":   rate.Source,
		"rate_fetched":  rate.FetchedAt.Format(time.RFC3339),
	})

	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, e := h.db.ExecContext(ctx, `
			INSERT INTO payment_intents (id, user_id, provider, amount_cents, currency, status, metadata_json, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $7)
		`, paymentIntentID, userID, req.Provider, req.AmountCents, "USD", metadataJSON, now)
		return e
	})
	if err != nil {
		h.logger.Error("Failed to create payment intent", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Create payment with Jibit — send IRR amount to the gateway
	providerReq := &providers.CreatePaymentRequest{
		AmountCents:    amountIRR, // IRR amount for Jibit
		Currency:       "IRR",
		UserID:         userID,
		OrderID:        paymentIntentID,
		Description:    "Deposit to Tragge Wallet",
		CallbackURL:    h.config.SuccessRedirectURL,
		CancelURL:      h.config.CancelRedirectURL,
		IPNCallbackURL: h.config.WebhookBaseURL + "/webhooks/" + req.Provider,
		CustomerPhone:  req.CustomerPhone,
	}

	providerResp, err := provider.CreatePayment(ctx, providerReq)
	if err != nil {
		h.logger.Error("Failed to create Jibit payment",
			zap.Error(err),
			zap.String("payment_intent_id", paymentIntentID),
			zap.Int64("amount_irr", amountIRR))

		_ = h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
			_, e := h.db.ExecContext(ctx, `
				UPDATE payment_intents SET status = 'failed', updated_at = $1 WHERE id = $2
			`, time.Now(), paymentIntentID)
			return e
		})

		writeErrorJSON(w, http.StatusBadGateway, "failed to create payment with provider")
		return
	}

	// Record successful payment creation metric
	if providerType == providers.ProviderPayment4 && h.metrics != nil {
		h.metrics.OnPayment4Created()
	}

	// Update payment intent with provider payment ID
	err = h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, e := h.db.ExecContext(ctx, `
			UPDATE payment_intents
			SET provider_payment_id = $1, status = 'processing', updated_at = $2
			WHERE id = $3
		`, providerResp.ProviderPaymentID, time.Now(), paymentIntentID)
		return e
	})
	if err != nil {
		h.logger.Error("Failed to update payment intent", zap.Error(err))
	}

	writeJSON(w, http.StatusCreated, CreateDepositResponse{
		PaymentIntentID: paymentIntentID,
		PaymentURL:      providerResp.PaymentURL,
		Status:          string(providerResp.Status),
		Metadata:        providerResp.Metadata,
	})
}

// HandleGetDepositStatus handles GET /api/payments/deposit/{id}/status
func (h *DepositHandler) HandleGetDepositStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Extract payment intent ID from URL path parameter
	paymentIntentID := chi.URLParam(r, "id")
	if paymentIntentID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "payment intent id is required")
		return
	}

	// Get payment intent from database
	var intent struct {
		ID                string
		UserID            string
		Provider          string
		ProviderPaymentID sql.NullString
		AmountCents       int64
		Currency          string
		Status            string
		CompletedAt       sql.NullTime
	}

	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return h.db.QueryRowContext(ctx, `
			SELECT id, user_id, provider, provider_payment_id, amount_cents, currency, status, completed_at
			FROM payment_intents
			WHERE id = $1
		`, paymentIntentID).Scan(
			&intent.ID, &intent.UserID, &intent.Provider,
			&intent.ProviderPaymentID, &intent.AmountCents,
			&intent.Currency, &intent.Status, &intent.CompletedAt,
		)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErrorJSON(w, http.StatusNotFound, "payment intent not found")
			return
		}
		h.logger.Error("Failed to get payment intent", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Verify ownership
	if intent.UserID != userID {
		writeErrorJSON(w, http.StatusForbidden, "access denied")
		return
	}

	// If we have a provider payment ID and status is processing, check with provider.
	// Terminal statuses (succeeded, failed, refunded, expired) are NOT persisted here
	// to avoid a race condition: if we set "succeeded" without crediting the wallet,
	// the webhook will later skip the credit due to its idempotency check.
	// Terminal transitions are handled exclusively by the webhook or inquiry/expiry workers.
	var displayStatus string
	if intent.ProviderPaymentID.Valid && intent.Status == "processing" {
		providerType := providers.ProviderType(intent.Provider)
		if provider, ok := h.registry.Get(providerType); ok {
			if status, err := provider.GetPaymentStatus(ctx, intent.ProviderPaymentID.String); err == nil {
				newStatus := mapProviderStatusToIntent(status.Status)
				if newStatus != intent.Status {
					if isTerminalStatus(newStatus) {
						// Return the real status to the client but do NOT persist it.
						displayStatus = newStatus
						h.logger.Info("Polling detected terminal status, deferring to webhook/worker",
							zap.String("payment_intent_id", paymentIntentID),
							zap.String("provider_status", string(status.Status)),
							zap.String("mapped_status", newStatus))
					} else {
						h.updatePaymentIntentStatus(ctx, paymentIntentID, newStatus)
						intent.Status = newStatus
					}
				}
			}
		}
	}

	responseStatus := intent.Status
	if displayStatus != "" {
		responseStatus = displayStatus
	}

	response := map[string]interface{}{
		"payment_intent_id": intent.ID,
		"amount_cents":      intent.AmountCents,
		"currency":          intent.Currency,
		"status":            responseStatus,
		"provider":          intent.Provider,
	}

	if intent.CompletedAt.Valid {
		response["completed_at"] = intent.CompletedAt.Time.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, response)
}

// HandleGetPaymentStatusByPurchaseID handles GET /api/payments/status/{purchaseId}
// Looks up a payment intent by provider_payment_id (external purchase ID) and returns its status.
// Requires authentication and verifies ownership to prevent IDOR.
func (h *DepositHandler) HandleGetPaymentStatusByPurchaseID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	purchaseID := chi.URLParam(r, "purchaseId")
	if purchaseID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "purchase id is required")
		return
	}

	// Look up payment intent by provider_payment_id
	var intent struct {
		ID                string
		UserID            string
		Provider          string
		ProviderPaymentID sql.NullString
		AmountCents       int64
		Currency          string
		Status            string
		CompletedAt       sql.NullTime
	}

	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return h.db.QueryRowContext(ctx, `
			SELECT id, user_id, provider, provider_payment_id, amount_cents, currency, status, completed_at
			FROM payment_intents
			WHERE provider_payment_id = $1
		`, purchaseID).Scan(
			&intent.ID, &intent.UserID, &intent.Provider,
			&intent.ProviderPaymentID, &intent.AmountCents,
			&intent.Currency, &intent.Status, &intent.CompletedAt,
		)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErrorJSON(w, http.StatusNotFound, "payment not found")
			return
		}
		h.logger.Error("Failed to get payment intent by purchase id", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Verify ownership
	if intent.UserID != userID {
		writeErrorJSON(w, http.StatusForbidden, "access denied")
		return
	}

	// If status is processing, check with provider for latest status.
	// Same as HandleGetDepositStatus: do not persist terminal statuses to avoid
	// the race condition where webhook skips wallet credit.
	var displayStatus string
	if intent.ProviderPaymentID.Valid && intent.Status == "processing" {
		providerType := providers.ProviderType(intent.Provider)
		if provider, ok := h.registry.Get(providerType); ok {
			if status, err := provider.GetPaymentStatus(ctx, intent.ProviderPaymentID.String); err == nil {
				newStatus := mapProviderStatusToIntent(status.Status)
				if newStatus != intent.Status {
					if isTerminalStatus(newStatus) {
						displayStatus = newStatus
						h.logger.Info("Polling detected terminal status, deferring to webhook/worker",
							zap.String("payment_intent_id", intent.ID),
							zap.String("provider_status", string(status.Status)),
							zap.String("mapped_status", newStatus))
					} else {
						h.updatePaymentIntentStatus(ctx, intent.ID, newStatus)
						intent.Status = newStatus
					}
				}
			}
		}
	}

	responseStatus := intent.Status
	if displayStatus != "" {
		responseStatus = displayStatus
	}

	response := map[string]interface{}{
		"payment_intent_id": intent.ID,
		"amount_cents":      intent.AmountCents,
		"currency":          intent.Currency,
		"status":            responseStatus,
		"provider":          intent.Provider,
	}

	if intent.CompletedAt.Valid {
		response["completed_at"] = intent.CompletedAt.Time.Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, response)
}

// updatePaymentIntentStatus updates the payment intent status in the database
func (h *DepositHandler) updatePaymentIntentStatus(ctx context.Context, paymentIntentID, status string) {
	var completedAt interface{}
	if status == "succeeded" || status == "failed" || status == "refunded" || status == "expired" {
		completedAt = time.Now()
	}

	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, e := h.db.ExecContext(ctx, `
			UPDATE payment_intents
			SET status = $1, completed_at = $2, updated_at = $3
			WHERE id = $4
		`, status, completedAt, time.Now(), paymentIntentID)
		return e
	})
	if err != nil {
		h.logger.Error("Failed to update payment intent status",
			zap.Error(err),
			zap.String("payment_intent_id", paymentIntentID),
			zap.String("status", status))
	}
}

// mapProviderStatusToIntent maps provider payment status to payment intent status.
// Delegates to providers.MapStatusToIntentStatus for a single source of truth.
func mapProviderStatusToIntent(status providers.PaymentStatus) string {
	return providers.MapStatusToIntentStatus(status)
}

// isTerminalStatus returns true if the status is a terminal state that should
// only be persisted by the webhook handler or background workers (which perform
// wallet credit atomically). Polling endpoints must not persist these statuses
// to avoid a race condition where the webhook skips wallet credit due to
// idempotency checks.
func isTerminalStatus(status string) bool {
	switch status {
	case "succeeded", "failed", "refunded", "expired":
		return true
	default:
		return false
	}
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeErrorJSON writes an error JSON response
func writeErrorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
