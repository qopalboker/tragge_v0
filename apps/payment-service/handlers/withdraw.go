package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/kyc"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// WithdrawHandler handles withdrawal-related requests
type WithdrawHandler struct {
	db            *sql.DB
	walletService *wallet.Service
	kycService    *kyc.Service
	registry      *providers.ProviderRegistry
	logger        *zap.Logger
	config        *WithdrawConfig
	circuits      DatabaseCircuitExecutor
}

// WithdrawConfig holds configuration for withdrawal operations
type WithdrawConfig struct {
	MinWithdrawCents   int64
	MaxWithdrawCents   int64
	WithdrawFeeCents   int64
	WithdrawFeePercent float64
	// AML withdrawal limits (defaults, per-user overrides in DB)
	DailyWithdrawAmountCents   int64
	MonthlyWithdrawAmountCents int64
	DailyWithdrawCount         int
	MonthlyWithdrawCount       int
}

// NewWithdrawHandler creates a new withdrawal handler
func NewWithdrawHandler(db *sql.DB, walletService *wallet.Service, kycService *kyc.Service, registry *providers.ProviderRegistry, logger *zap.Logger, config *WithdrawConfig, circuits DatabaseCircuitExecutor) *WithdrawHandler {
	return &WithdrawHandler{
		db:            db,
		walletService: walletService,
		kycService:    kycService,
		registry:      registry,
		logger:        logger,
		config:        config,
		circuits:      circuits,
	}
}

// WithdrawRequest represents the request body for creating a withdrawal
type WithdrawRequest struct {
	AmountCents     int64  `json:"amount_cents"`
	Currency        string `json:"currency,omitempty"`
	DestinationType string `json:"destination_type"` // bank, crypto

	// Bank transfer fields
	BankAccount   string `json:"bank_account,omitempty"`   // IBAN or account number
	BankName      string `json:"bank_name,omitempty"`
	AccountHolder string `json:"account_holder,omitempty"`

	// Crypto payout fields
	WalletAddress  string `json:"wallet_address,omitempty"`
	CryptoCurrency string `json:"crypto_currency,omitempty"` // BTC, ETH, USDT, etc.
}

// WithdrawResponse represents the response for creating a withdrawal
type WithdrawResponse struct {
	PayoutID       string `json:"payout_id"`
	AmountCents    int64  `json:"amount_cents"`
	FeeCents       int64  `json:"fee_cents"`
	NetAmountCents int64  `json:"net_amount_cents"`
	Currency       string `json:"currency"`
	Status         string `json:"status"`
	EstimatedTime  string `json:"estimated_time,omitempty"`
	KYCStatus      string `json:"kyc_status,omitempty"`
	KYCMessage     string `json:"kyc_message,omitempty"`
}

// WithdrawErrorResponse represents an error response for withdrawal requests
type WithdrawErrorResponse struct {
	Error          string `json:"error"`
	Message        string `json:"message,omitempty"`
	KYCStatus      string `json:"kyc_status,omitempty"`
	KYCMessage     string `json:"kyc_message,omitempty"`
	MinimumCents   int64  `json:"minimum_cents,omitempty"`
	AvailableCents int64  `json:"available_cents,omitempty"`
	RequestedCents int64  `json:"requested_cents,omitempty"`
	// AML limit fields
	LimitType  string `json:"limit_type,omitempty"`
	LimitCents int64  `json:"limit_cents,omitempty"`
	UsedCents  int64  `json:"used_cents,omitempty"`
	LimitCount int64  `json:"limit_count,omitempty"`
	UsedCount  int64  `json:"used_count,omitempty"`
}

// HandleCreateWithdraw handles POST /api/payments/withdraw/request
func (h *WithdrawHandler) HandleCreateWithdraw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var req WithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Check KYC verification status before processing withdrawal
	kycResult, err := h.kycService.CheckVerification(ctx, userID)
	if err != nil {
		h.logger.Error("Failed to check KYC status", zap.Error(err), zap.String("user_id", userID))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if !kycResult.Verified {
		h.logger.Info("Withdrawal blocked: KYC not verified",
			zap.String("user_id", userID),
			zap.String("kyc_status", string(kycResult.Status)))

		writeJSON(w, http.StatusForbidden, WithdrawErrorResponse{
			Error:      "kyc_required",
			Message:    "KYC verification is required for withdrawals",
			KYCStatus:  string(kycResult.Status),
			KYCMessage: kycResult.Message,
		})
		return
	}

	// Check minimum amount first (specific error format)
	if req.AmountCents < h.config.MinWithdrawCents {
		h.logger.Info("Withdrawal blocked: below minimum amount",
			zap.String("user_id", userID),
			zap.Int64("amount_cents", req.AmountCents),
			zap.Int64("minimum_cents", h.config.MinWithdrawCents))

		minDollars := float64(h.config.MinWithdrawCents) / 100.0
		writeJSON(w, http.StatusBadRequest, WithdrawErrorResponse{
			Error:        "minimum_amount",
			Message:      fmt.Sprintf("Minimum withdrawal is $%.2f", minDollars),
			MinimumCents: h.config.MinWithdrawCents,
		})
		return
	}

	// Check withdrawal limits (AML compliance)
	limitErr := h.walletService.CheckWithdrawalLimits(ctx, userID, req.AmountCents, wallet.WithdrawalLimits{
		DailyAmountCents:   h.config.DailyWithdrawAmountCents,
		MonthlyAmountCents: h.config.MonthlyWithdrawAmountCents,
		DailyCount:         h.config.DailyWithdrawCount,
		MonthlyCount:       h.config.MonthlyWithdrawCount,
	})
	if limitErr != nil {
		if limitExceeded, ok := limitErr.(*wallet.WithdrawalLimitExceededError); ok {
			h.logger.Info("Withdrawal blocked: limit exceeded",
				zap.String("user_id", userID),
				zap.String("limit_type", limitExceeded.LimitType),
				zap.Int64("limit_value", limitExceeded.LimitValue),
				zap.Int64("current_usage", limitExceeded.CurrentUsage),
				zap.Int64("requested", limitExceeded.RequestedValue))

			resp := WithdrawErrorResponse{
				Error:   "withdrawal_limit_exceeded",
				Message: limitExceeded.Error(),
			}
			switch limitExceeded.LimitType {
			case "daily_amount", "monthly_amount":
				resp.LimitType = limitExceeded.LimitType
				resp.LimitCents = limitExceeded.LimitValue
				resp.UsedCents = limitExceeded.CurrentUsage
				resp.RequestedCents = limitExceeded.RequestedValue
			case "daily_count", "monthly_count":
				resp.LimitType = limitExceeded.LimitType
				resp.LimitCount = limitExceeded.LimitValue
				resp.UsedCount = limitExceeded.CurrentUsage
			}
			writeJSON(w, http.StatusTooManyRequests, resp)
			return
		}
		h.logger.Error("Failed to check withdrawal limits", zap.Error(limitErr))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Validate request
	v := validation.New()

	if req.AmountCents > h.config.MaxWithdrawCents {
		v.AddError("amount_cents", "max_withdraw", "amount exceeds maximum")
	}

	if req.DestinationType != "bank" && req.DestinationType != "crypto" {
		v.AddError("destination_type", "invalid_destination", "must be 'bank' or 'crypto'")
	}

	if req.DestinationType == "bank" {
		v.Required("bank_account", req.BankAccount)
		v.Required("account_holder", req.AccountHolder)
	}

	if req.DestinationType == "crypto" {
		v.Required("wallet_address", req.WalletAddress)
		v.Required("crypto_currency", req.CryptoCurrency)
	}

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Calculate fee using integer arithmetic to avoid float precision issues.
	// Convert the percentage to basis points (1% = 100 bps) so the remaining
	// math stays in int64. E.g. 1.5% → 150 bps → amount * 150 / 10000.
	feeCents := h.config.WithdrawFeeCents
	if h.config.WithdrawFeePercent > 0 {
		feeBasisPoints := int64(math.Round(h.config.WithdrawFeePercent * 100))
		feeCents += req.AmountCents * feeBasisPoints / 10000
	}
	totalDeductCents := req.AmountCents + feeCents
	netAmountCents := req.AmountCents

	// Generate payout ID early so it can be referenced in ledger entries
	payoutID := uuid.New().String()

	// Set default currency
	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}

	// Begin transaction (protected by circuit breaker)
	var tx *sql.Tx
	err = h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var e error
		tx, e = h.db.BeginTx(ctx, nil)
		return e
	})
	if err != nil {
		h.logger.Error("Failed to begin transaction", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}
	defer tx.Rollback()

	txWrapper := &wallet.TxAdapter{Tx: tx}

	// Pre-check: verify sufficient balance for amount + fee before any debits.
	// GetBalanceTx does SELECT FOR UPDATE, so the row is locked for subsequent Debits.
	if feeCents > 0 {
		balance, balErr := h.walletService.GetBalanceTx(ctx, txWrapper, userID)
		if balErr != nil {
			if _, ok := balErr.(*wallet.WalletFrozenError); ok {
				writeJSON(w, http.StatusForbidden, WithdrawErrorResponse{
					Error:   "wallet_frozen",
					Message: "Your wallet is frozen and cannot process withdrawals",
				})
				return
			}
			if _, ok := balErr.(*wallet.WalletNotFoundError); ok {
				writeJSON(w, http.StatusBadRequest, WithdrawErrorResponse{
					Error:   "insufficient_balance",
					Message: "Wallet not found",
				})
				return
			}
			h.logger.Error("Failed to check balance", zap.Error(balErr))
			writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if balance < totalDeductCents {
			writeJSON(w, http.StatusBadRequest, WithdrawErrorResponse{
				Error:          "insufficient_balance",
				Message:        "Insufficient balance for this withdrawal including fees",
				AvailableCents: balance,
				RequestedCents: totalDeductCents,
			})
			return
		}
	}

	// Debit withdrawal amount from wallet
	refType := wallet.LedgerRefTypePayout
	_, err = h.walletService.Debit(ctx, txWrapper, userID, req.AmountCents, wallet.LedgerTypeWithdrawal, &refType, &payoutID, nil)
	if err != nil {
		if insufficientErr, ok := err.(*wallet.InsufficientBalanceError); ok {
			h.logger.Info("Withdrawal blocked: insufficient balance",
				zap.String("user_id", userID),
				zap.Int64("requested_cents", totalDeductCents),
				zap.Int64("available_cents", insufficientErr.Available))

			writeJSON(w, http.StatusBadRequest, WithdrawErrorResponse{
				Error:          "insufficient_balance",
				Message:        "Insufficient balance for this withdrawal",
				AvailableCents: insufficientErr.Available,
				RequestedCents: totalDeductCents,
			})
			return
		}
		if _, ok := err.(*wallet.WalletFrozenError); ok {
			h.logger.Info("Withdrawal blocked: wallet frozen",
				zap.String("user_id", userID))

			writeJSON(w, http.StatusForbidden, WithdrawErrorResponse{
				Error:   "wallet_frozen",
				Message: "Your wallet is frozen and cannot process withdrawals",
			})
			return
		}
		h.logger.Error("Failed to debit wallet", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Debit withdrawal fee as a separate ledger entry (if applicable)
	if feeCents > 0 {
		feeDesc := "Withdrawal fee"
		_, err = h.walletService.Debit(ctx, txWrapper, userID, feeCents, wallet.LedgerTypeWithdrawFee, &refType, &payoutID, &feeDesc)
		if err != nil {
			if insufficientErr, ok := err.(*wallet.InsufficientBalanceError); ok {
				h.logger.Info("Withdrawal blocked: insufficient balance for fee",
					zap.String("user_id", userID),
					zap.Int64("fee_cents", feeCents),
					zap.Int64("available_cents", insufficientErr.Available))

				writeJSON(w, http.StatusBadRequest, WithdrawErrorResponse{
					Error:          "insufficient_balance",
					Message:        "Insufficient balance for withdrawal fee",
					AvailableCents: insufficientErr.Available,
					RequestedCents: totalDeductCents,
				})
				return
			}
			h.logger.Error("Failed to debit withdrawal fee", zap.Error(err))
			writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}

	// Create payout record
	now := time.Now()

	destinationInfo := map[string]string{
		"destination_type": req.DestinationType,
	}
	if req.DestinationType == "bank" {
		destinationInfo["bank_account"] = req.BankAccount
		destinationInfo["bank_name"] = req.BankName
		destinationInfo["account_holder"] = req.AccountHolder
	} else {
		destinationInfo["wallet_address"] = req.WalletAddress
		destinationInfo["crypto_currency"] = req.CryptoCurrency
	}
	destinationJSON, _ := json.Marshal(destinationInfo)

	metadataJSON, _ := json.Marshal(map[string]interface{}{
		"fee_cents":        feeCents,
		"gross_amount":     req.AmountCents,
		"net_amount":       netAmountCents,
	})

	// Determine provider based on destination type
	var providerName string
	if req.DestinationType == "bank" {
		providerName = string(providers.ProviderJibit)
	} else {
		providerName = string(providers.ProviderNowPayments)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO payouts (id, user_id, amount_cents, currency, status, provider, destination_type, destination_info_json, metadata_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, $8, $9, $9)
	`, payoutID, userID, netAmountCents, currency, providerName, req.DestinationType, destinationJSON, metadataJSON, now)
	if err != nil {
		h.logger.Error("Failed to create payout record", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		h.logger.Error("Failed to commit transaction", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.logger.Info("Withdrawal request created",
		zap.String("payout_id", payoutID),
		zap.String("user_id", userID),
		zap.Int64("amount_cents", req.AmountCents),
		zap.Int64("fee_cents", feeCents),
		zap.String("destination_type", req.DestinationType))

	estimatedTime := "1-3 business days"
	if req.DestinationType == "crypto" {
		estimatedTime = "10-60 minutes"
	}

	writeJSON(w, http.StatusCreated, WithdrawResponse{
		PayoutID:       payoutID,
		AmountCents:    req.AmountCents,
		FeeCents:       feeCents,
		NetAmountCents: netAmountCents,
		Currency:       currency,
		Status:         "pending",
		EstimatedTime:  estimatedTime,
		KYCStatus:      string(kycResult.Status),
		KYCMessage:     "KYC verified",
	})
}

// HandleGetWithdrawStatus handles GET /api/payments/withdraw/{id}/status
func (h *WithdrawHandler) HandleGetWithdrawStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Extract payout ID from URL path parameter
	payoutID := chi.URLParam(r, "id")
	if payoutID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "payout id is required")
		return
	}

	// Get payout from database
	var payout struct {
		ID              string
		UserID          string
		AmountCents     int64
		Currency        string
		Status          string
		DestinationType string
		CompletedAt     sql.NullTime
		MetadataJSON    sql.NullString
	}

	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return h.db.QueryRowContext(ctx, `
			SELECT id, user_id, amount_cents, currency, status, destination_type, completed_at, metadata_json
			FROM payouts
			WHERE id = $1
		`, payoutID).Scan(
			&payout.ID, &payout.UserID, &payout.AmountCents,
			&payout.Currency, &payout.Status, &payout.DestinationType,
			&payout.CompletedAt, &payout.MetadataJSON,
		)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErrorJSON(w, http.StatusNotFound, "payout not found")
			return
		}
		h.logger.Error("Failed to get payout", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Verify ownership
	if payout.UserID != userID {
		writeErrorJSON(w, http.StatusForbidden, "access denied")
		return
	}

	response := map[string]interface{}{
		"payout_id":        payout.ID,
		"amount_cents":     payout.AmountCents,
		"currency":         payout.Currency,
		"status":           payout.Status,
		"destination_type": payout.DestinationType,
	}

	if payout.CompletedAt.Valid {
		response["completed_at"] = payout.CompletedAt.Time.Format(time.RFC3339)
	}

	// Parse metadata for fee info
	if payout.MetadataJSON.Valid {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(payout.MetadataJSON.String), &metadata); err == nil {
			if feeCents, ok := metadata["fee_cents"].(float64); ok {
				response["fee_cents"] = int64(feeCents)
			}
		}
	}

	writeJSON(w, http.StatusOK, response)
}

