package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ChargeWalletRequest represents a wallet charge request.
type ChargeWalletRequest struct {
	Amount       int64  `json:"amount"` // Amount in cents
	Reason       string `json:"reason"`
	ConfirmDebit bool   `json:"confirm_debit"` // Must be true for negative (debit) amounts
}

// handleChargeUserWallet charges a user's wallet (super_admin only).
// Uses wallet.Service for proper ledger entry creation.
func (a *App) handleChargeUserWallet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")
	actorUserID := auth.GetUserID(ctx)

	if userID == "" {
		validation.WriteBadRequest(w, "user_id is required")
		return
	}

	var req ChargeWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	if req.Amount == 0 {
		validation.WriteBadRequest(w, "amount is required and must be non-zero")
		return
	}

	if req.Amount > 10_000_000 || req.Amount < -10_000_000 { // $100,000 limit
		validation.WriteBadRequest(w, "amount exceeds allowed limit")
		return
	}

	if req.Amount < 0 && !req.ConfirmDebit {
		validation.WriteBadRequest(w, "negative amounts require confirm_debit: true")
		return
	}

	if req.Reason == "" {
		a.auditSensitiveDenial(ctx, actorUserID, actionWalletAdjust, userID, "mandatory_reason_denied")
		validation.WriteBadRequest(w, "reason is required")
		return
	}

	// Sanitize input
	req.Reason = validation.SanitizeString(req.Reason)

	// Check if user exists (circuit breaker protected)
	var exists bool
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", userID).Scan(&exists)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.UserNotFound})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.UserNotFound})
		return
	}

	// Begin transaction
	var tx *sql.Tx
	err = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	txWrapper := &adminTxWrapper{tx: tx}

	// Build description with admin charge label
	description := fmt.Sprintf("Ã˜Â´Ã˜Â§Ã˜Â±ÃšËœ Ã˜ÂªÃ™Ë†Ã˜Â³Ã˜Â· Ã˜Â§Ã˜Â¯Ã™â€¦Ã›Å’Ã™â€ : %s", req.Reason)

	// Use wallet.Service for proper ledger entry
	refType := wallet.LedgerRefTypeAdminAction
	reasonCode := wallet.ReasonCodeWalletTopup
	idempotencyKey := fmt.Sprintf("admin_charge:%s:%s:%d", actorUserID, userID, time.Now().UnixNano())

	var entry *wallet.LedgerEntry
	if req.Amount > 0 {
		entry, err = a.walletService.CreditIdempotentWithReason(
			ctx, txWrapper, userID, req.Amount,
			wallet.LedgerTypeDeposit, &refType, nil, &description, &reasonCode, idempotencyKey,
		)
		if err != nil {
			// Check for duplicate (idempotency hit)
			if _, ok := err.(*wallet.DuplicateCreditError); ok {
				a.log().Warn("Duplicate admin charge detected",
					zap.String("user_id", userID))
				writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.DuplicateCharge})
				return
			}
			// Check for wallet not found Ã¢â‚¬â€ create wallet first (within transaction)
			if _, ok := err.(*wallet.WalletNotFoundError); ok {
				_, createErr := a.walletService.CreateWalletTx(ctx, txWrapper, userID)
				if createErr != nil {
					a.log().Error("Failed to create wallet for user", zap.Error(createErr))
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.CreateWalletFailed})
					return
				}
				// Retry credit after wallet creation
				entry, err = a.walletService.CreditIdempotentWithReason(
					ctx, txWrapper, userID, req.Amount,
					wallet.LedgerTypeDeposit, &refType, nil, &description, &reasonCode, idempotencyKey,
				)
				if err != nil {
					if _, ok := err.(*wallet.DuplicateCreditError); ok {
						a.log().Warn("Duplicate admin charge detected after wallet creation")
						writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.DuplicateCharge})
						return
					}
					a.log().Error("Failed to credit wallet after creation", zap.Error(err))
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ChargeWalletFailed})
					return
				}
			} else {
				a.log().Error("Failed to credit wallet", zap.Error(err))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ChargeWalletFailed})
				return
			}
		}
	} else {
		// Negative amount Ã¢â‚¬â€ debit (adjustment)
		absAmount := -req.Amount
		entry, err = a.walletService.DebitWithReason(
			ctx, txWrapper, userID, absAmount,
			wallet.LedgerTypeAdjustment, &refType, nil, &description, &reasonCode,
		)
		if err != nil {
			if _, ok := err.(*wallet.InsufficientBalanceError); ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InsufficientBalance})
				return
			}
			if _, ok := err.(*wallet.WalletNotFoundError); ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.WalletNoWallet})
				return
			}
			a.log().Error("Failed to debit wallet", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.ChargeWalletFailed})
			return
		}
	}

	// Write audit log
	auditPayload := map[string]interface{}{
		"user_id":         userID,
		"amount":          req.Amount,
		"reason":          req.Reason,
		"description":     description,
		"new_balance":     entry.BalanceAfterCents,
		"ledger_entry_id": entry.ID,
		"ip_address":      getAdminClientIP(r),
		"user_agent":      r.UserAgent(),
	}
	payloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "user.wallet.charged", "user", userID, payloadJSON)
	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.log().Info("User wallet charged by admin",
		zap.String("user_id", userID),
		zap.Int64("amount", req.Amount),
		zap.String("reason", req.Reason),
		zap.String("description", description),
		zap.Int64("new_balance", entry.BalanceAfterCents),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":         adminMsg.WalletCharged,
		"new_balance":     entry.BalanceAfterCents,
		"ledger_entry_id": entry.ID,
	})
}

// handleGetUserWalletHistory returns paginated wallet history for admin view.
func (a *App) handleGetUserWalletHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")

	if userID == "" {
		validation.WriteBadRequest(w, "user_id is required")
		return
	}

	// Parse pagination params
	limit := 50
	offset := 0
	page := 1
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
			offset = (p - 1) * limit
		}
	}

	// Optional type filter
	typeFilter := r.URL.Query().Get("type")

	// Validate type filter against known ledger types
	validTypes := map[string]bool{
		"deposit": true, "withdrawal": true, "contest_entry": true,
		"contest_refund": true, "prize_credit": true, "adjustment": true,
		"affiliate_commission": true, "withdraw_fee": true,
		"withdrawal_refund": true, "withdraw_fee_refund": true,
	}
	if typeFilter != "" && !validTypes[typeFilter] {
		typeFilter = "" // invalid type Ã¢â€ â€™ show all
	}

	// Get wallet info
	var balanceCents int64
	var currency, walletStatus string
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx,
			`SELECT COALESCE(balance_cents, 0), COALESCE(currency, 'USD'), COALESCE(status::text, 'active')
			 FROM wallets WHERE user_id = $1`, userID).Scan(&balanceCents, &currency, &walletStatus)
	})
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.WalletNotFound})
			return
		}
		a.log().Error("Failed to get wallet", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Build query with optional type filter
	query := `SELECT id, type::text, amount_cents, balance_after_cents, description, reason_code, ref_type::text, ref_id, idempotency_key, created_at
			  FROM wallet_ledger WHERE user_id = $1`
	countQuery := `SELECT COUNT(*) FROM wallet_ledger WHERE user_id = $1`
	args := []interface{}{userID}
	countArgs := []interface{}{userID}

	if typeFilter != "" {
		query += ` AND type = $2`
		countQuery += ` AND type = $2`
		args = append(args, typeFilter)
		countArgs = append(countArgs, typeFilter)
	}

	query += ` ORDER BY created_at DESC LIMIT $` + strconv.Itoa(len(args)+1) + ` OFFSET $` + strconv.Itoa(len(args)+2)
	args = append(args, limit, offset)

	// Get total count
	var total int
	_ = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	})

	// Get entries
	var entries []AdminWalletHistoryEntry
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, query, args...)
		},
	)
	if err != nil {
		a.log().Error("Failed to query wallet history", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	rows := result.(*sql.Rows)
	defer rows.Close()

	for rows.Next() {
		var e AdminWalletHistoryEntry
		var desc, reasonCode, refType, refID, idempotencyKey sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &e.Type, &e.AmountCents, &e.BalanceAfterCents,
			&desc, &reasonCode, &refType, &refID, &idempotencyKey, &createdAt); err != nil {
			a.log().Error("Failed to scan wallet history entry", zap.Error(err))
			continue
		}
		if desc.Valid && desc.String != "" {
			e.Description = &desc.String
		}
		if reasonCode.Valid && reasonCode.String != "" {
			e.ReasonCode = &reasonCode.String
		}
		if refType.Valid && refType.String != "" {
			e.RefType = &refType.String
		}
		if refID.Valid && refID.String != "" {
			e.RefID = &refID.String
		}
		if idempotencyKey.Valid && idempotencyKey.String != "" {
			e.IdempotencyKey = &idempotencyKey.String
		}
		e.CreatedAt = createdAt.Format(time.RFC3339)
		entries = append(entries, e)
	}

	if entries == nil {
		entries = []AdminWalletHistoryEntry{}
	}

	writeJSON(w, http.StatusOK, AdminWalletHistoryResponse{
		Entries:      entries,
		Total:        total,
		BalanceCents: balanceCents,
		Currency:     currency,
		WalletStatus: walletStatus,
		Page:         page,
		HasMore:      offset+limit < total,
	})
}

// ============================================================================
// Affiliate Management Handlers
// ============================================================================

// PendingAffiliateRequest represents a pending affiliate activation request.
type PendingAffiliateRequest struct {
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	Code        string    `json:"code"`
	RequestedAt time.Time `json:"requested_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// handleListPendingAffiliateRequests lists all pending affiliate activation requests.
// GET /api/admin/affiliate/pending
func (a *App) handleListPendingAffiliateRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT
					rc.user_id,
					u.email,
					rc.code,
					rc.activation_requested_at,
					rc.created_at
				FROM referral_codes rc
				JOIN users u ON rc.user_id = u.id
				WHERE rc.activation_status = 'pending'
				ORDER BY rc.activation_requested_at ASC
			`)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query pending affiliate requests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var requests []PendingAffiliateRequest
	for rows.Next() {
		var req PendingAffiliateRequest
		var requestedAt sql.NullTime
		if err := rows.Scan(&req.UserID, &req.Email, &req.Code, &requestedAt, &req.CreatedAt); err != nil {
			a.log().Error("Failed to scan pending affiliate request", zap.Error(err))
			continue
		}
		if requestedAt.Valid {
			req.RequestedAt = requestedAt.Time
		}
		requests = append(requests, req)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Row iteration error", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if requests == nil {
		requests = []PendingAffiliateRequest{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"requests": requests,
		"total":    len(requests),
	})
}

// handleApproveAffiliateActivation approves an affiliate activation request.
// POST /api/admin/affiliate/{user_id}/approve
func (a *App) handleApproveAffiliateActivation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")
	actorUserID := auth.GetUserID(ctx)

	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.UserIDRequired})
		return
	}

	// Check if referral code exists and is pending (circuit breaker protected)
	var currentStatus string
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx, `
			SELECT COALESCE(activation_status::text, 'inactive')
			FROM referral_codes
			WHERE user_id = $1
		`, userID).Scan(&currentStatus)
	})

	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ReferralNotFound})
			return
		}
		a.log().Error("Failed to check activation status", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ActivationNotPending})
		return
	}

	// Approve activation (circuit breaker protected)
	err = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx, `
			UPDATE referral_codes
			SET activation_status = 'active',
			    is_active = TRUE,
			    activation_approved_at = NOW()
			WHERE user_id = $1
		`, userID)
		return execErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to approve affiliate activation", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Log audit entry (circuit breaker protected)
	_ = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx, `
			INSERT INTO audit_logs (user_id, action, target_type, target_id, details)
			VALUES ($1, 'affiliate_activation_approved', 'user', $2, $3)
		`, actorUserID, userID, `{"status": "approved"}`)
		return execErr
	})

	a.log().Info("Affiliate activation approved",
		zap.String("user_id", userID),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.ActivationApproved})
}

// RejectAffiliateRequest is the request body for rejecting an affiliate activation.
type RejectAffiliateRequest struct {
	Reason string `json:"reason"`
}

// handleRejectAffiliateActivation rejects an affiliate activation request.
// POST /api/admin/affiliate/{user_id}/reject
func (a *App) handleRejectAffiliateActivation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")
	actorUserID := auth.GetUserID(ctx)

	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.UserIDRequired})
		return
	}

	// Parse request body
	var req RejectAffiliateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body - reason is optional
		req.Reason = ""
	}

	// Check if referral code exists and is pending (circuit breaker protected)
	var currentStatus string
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx, `
			SELECT COALESCE(activation_status::text, 'inactive')
			FROM referral_codes
			WHERE user_id = $1
		`, userID).Scan(&currentStatus)
	})

	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.ReferralNotFound})
			return
		}
		a.log().Error("Failed to check activation status", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.ActivationNotPending})
		return
	}

	// Reject activation (circuit breaker protected)
	err = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx, `
			UPDATE referral_codes
			SET activation_status = 'rejected',
			    is_active = FALSE,
			    activation_rejected_at = NOW(),
			    rejection_reason = $2
			WHERE user_id = $1
		`, userID, sql.NullString{String: req.Reason, Valid: req.Reason != ""})
		return execErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to reject affiliate activation", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Log audit entry (circuit breaker protected)
	details := fmt.Sprintf(`{"status": "rejected", "reason": %q}`, req.Reason)
	_ = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx, `
			INSERT INTO audit_logs (user_id, action, target_type, target_id, details)
			VALUES ($1, 'affiliate_activation_rejected', 'user', $2, $3)
		`, actorUserID, userID, details)
		return execErr
	})

	a.log().Info("Affiliate activation rejected",
		zap.String("user_id", userID),
		zap.String("actor_id", actorUserID),
		zap.String("reason", req.Reason))

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.ActivationRejected})
}

// ============================================================================
// WITHDRAWAL MANAGEMENT HANDLERS
// ============================================================================

// WithdrawalListResponse represents the response for listing withdrawals.
type WithdrawalListResponse struct {
	Withdrawals []WithdrawalItem `json:"withdrawals"`
	Total       int              `json:"total"`
	Page        int              `json:"page"`
	PerPage     int              `json:"per_page"`
}

// WithdrawalItem represents a withdrawal in the list response.
type WithdrawalItem struct {
	ID              string                 `json:"id"`
	User            WithdrawalUser         `json:"user"`
	AmountCents     int64                  `json:"amount_cents"`
	Currency        string                 `json:"currency"`
	Status          string                 `json:"status"`
	DestinationType *string                `json:"destination_type,omitempty"`
	DestinationInfo map[string]interface{} `json:"destination_info,omitempty"`
	AdminComment    *string                `json:"admin_comment,omitempty"`
	ReviewedBy      *string                `json:"reviewed_by,omitempty"`
	ReviewedAt      *time.Time             `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
}

// WithdrawalUser represents user info in withdrawal response.
type WithdrawalUser struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	Username *string `json:"username,omitempty"`
}

// WithdrawalDetailResponse represents the full withdrawal details.
type WithdrawalDetailResponse struct {
	ID              string                 `json:"id"`
	User            WithdrawalUserDetail   `json:"user"`
	AmountCents     int64                  `json:"amount_cents"`
	Currency        string                 `json:"currency"`
	Status          string                 `json:"status"`
	Provider        *string                `json:"provider,omitempty"`
	DestinationType *string                `json:"destination_type,omitempty"`
	DestinationInfo map[string]interface{} `json:"destination_info,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	AdminComment    *string                `json:"admin_comment,omitempty"`
	ReviewedBy      *string                `json:"reviewed_by,omitempty"`
	ReviewerEmail   *string                `json:"reviewer_email,omitempty"`
	ReviewedAt      *time.Time             `json:"reviewed_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	CompletedAt     *time.Time             `json:"completed_at,omitempty"`
	AuditHistory    []WithdrawalAuditEntry `json:"audit_history"`
}

// WithdrawalUserDetail represents detailed user info in withdrawal response.
type WithdrawalUserDetail struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	Username      *string `json:"username,omitempty"`
	FullName      *string `json:"full_name,omitempty"`
	WalletBalance int64   `json:"wallet_balance"`
	KYCStatus     *string `json:"kyc_status,omitempty"`
}

// WithdrawalAuditEntry represents an audit log entry for a withdrawal.
type WithdrawalAuditEntry struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	ActorID    string    `json:"actor_id"`
	ActorEmail string    `json:"actor_email"`
	Details    string    `json:"details"`
	CreatedAt  time.Time `json:"created_at"`
}

// WithdrawalActionRequest represents the request body for approve/reject/comment actions.
type WithdrawalActionRequest struct {
	Comment string `json:"comment"`
}

// handleListWithdrawals returns a paginated list of withdrawals.
// GET /api/admin/withdrawals
func (a *App) handleListWithdrawals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	status := r.URL.Query().Get("status")
	userID := r.URL.Query().Get("user_id")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	limit := 20

	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	// Build query with filters
	query := `
		SELECT p.id, p.user_id, u.email, u.username, p.amount_cents, p.currency,
		       p.status, p.destination_type, p.destination_info_json,
		       p.admin_comment, p.reviewed_by, p.reviewed_at, p.created_at, p.completed_at
		FROM payouts p
		JOIN users u ON u.id = p.user_id
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM payouts p WHERE 1=1`
	args := []interface{}{}
	countArgs := []interface{}{}
	argIdx := 1

	if status != "" {
		query += fmt.Sprintf(" AND p.status = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND p.status = $%d", argIdx)
		args = append(args, status)
		countArgs = append(countArgs, status)
		argIdx++
	}

	if userID != "" {
		query += fmt.Sprintf(" AND p.user_id = $%d", argIdx)
		countQuery += fmt.Sprintf(" AND p.user_id = $%d", argIdx)
		args = append(args, userID)
		countArgs = append(countArgs, userID)
		argIdx++
	}

	query += " ORDER BY p.created_at DESC"
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	// Get total count (circuit breaker protected)
	var total int
	if err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	}); err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to count withdrawals", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Query withdrawals (circuit breaker protected)
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, query, args...)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query withdrawals", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	withdrawals := []WithdrawalItem{}
	for rows.Next() {
		var item WithdrawalItem
		var userID, userEmail string
		var username sql.NullString
		var destType, adminComment, reviewedBy sql.NullString
		var destInfoJSON []byte
		var reviewedAt, completedAt sql.NullTime

		err := rows.Scan(
			&item.ID, &userID, &userEmail, &username,
			&item.AmountCents, &item.Currency, &item.Status,
			&destType, &destInfoJSON, &adminComment, &reviewedBy,
			&reviewedAt, &item.CreatedAt, &completedAt,
		)
		if err != nil {
			a.log().Error("Failed to scan withdrawal row", zap.Error(err))
			continue
		}

		item.User = WithdrawalUser{
			ID:    userID,
			Email: userEmail,
		}
		if username.Valid {
			item.User.Username = &username.String
		}

		if destType.Valid {
			item.DestinationType = &destType.String
		}
		if len(destInfoJSON) > 0 {
			_ = json.Unmarshal(destInfoJSON, &item.DestinationInfo)
		}
		if adminComment.Valid {
			item.AdminComment = &adminComment.String
		}
		if reviewedBy.Valid {
			item.ReviewedBy = &reviewedBy.String
		}
		if reviewedAt.Valid {
			item.ReviewedAt = &reviewedAt.Time
		}
		if completedAt.Valid {
			item.CompletedAt = &completedAt.Time
		}

		withdrawals = append(withdrawals, item)
	}

	writeJSON(w, http.StatusOK, WithdrawalListResponse{
		Withdrawals: withdrawals,
		Total:       total,
		Page:        page,
		PerPage:     limit,
	})
}

// handleGetPendingWithdrawalsCount returns the count of pending withdrawals.
// GET /api/admin/withdrawals/pending-count
func (a *App) handleGetPendingWithdrawalsCount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var count int
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx,
			`SELECT COUNT(*) FROM payouts WHERE status = 'pending'`,
		).Scan(&count)
	})

	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to count pending withdrawals", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// handleGetWithdrawal returns full details of a withdrawal.
// GET /api/admin/withdrawals/{id}
func (a *App) handleGetWithdrawal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	withdrawalID := chi.URLParam(r, "id")

	if withdrawalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.WithdrawalIDRequired})
		return
	}

	// Query withdrawal details with user info
	var resp WithdrawalDetailResponse
	var userID, userEmail string
	var username, fullName, kycStatus sql.NullString
	var walletBalance int64
	var provider, destType, adminComment, reviewedBy, reviewerEmail sql.NullString
	var destInfoJSON, metadataJSON []byte
	var reviewedAt, completedAt sql.NullTime

	query := `
		SELECT p.id, p.user_id, u.email, u.username, u.full_name,
		       COALESCE(w.balance_cents, 0), uv.status,
		       p.amount_cents, p.currency, p.status, p.provider,
		       p.destination_type, p.destination_info_json, p.metadata_json,
		       p.admin_comment, p.reviewed_by, reviewer.email,
		       p.reviewed_at, p.created_at, p.updated_at, p.completed_at
		FROM payouts p
		JOIN users u ON u.id = p.user_id
		LEFT JOIN wallets w ON w.user_id = p.user_id
		LEFT JOIN user_verification uv ON uv.user_id = p.user_id
		LEFT JOIN users reviewer ON reviewer.id = p.reviewed_by
		WHERE p.id = $1
	`

	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, query, withdrawalID).Scan(
			&resp.ID, &userID, &userEmail, &username, &fullName,
			&walletBalance, &kycStatus,
			&resp.AmountCents, &resp.Currency, &resp.Status, &provider,
			&destType, &destInfoJSON, &metadataJSON,
			&adminComment, &reviewedBy, &reviewerEmail,
			&reviewedAt, &resp.CreatedAt, &resp.UpdatedAt, &completedAt,
		)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.WithdrawalNotFound})
			return
		}
		a.log().Error("Failed to get withdrawal", zap.Error(err), zap.String("id", withdrawalID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Build response
	resp.User = WithdrawalUserDetail{
		ID:            userID,
		Email:         userEmail,
		WalletBalance: walletBalance,
	}
	if username.Valid {
		resp.User.Username = &username.String
	}
	if fullName.Valid {
		resp.User.FullName = &fullName.String
	}
	if kycStatus.Valid {
		resp.User.KYCStatus = &kycStatus.String
	}

	if provider.Valid {
		resp.Provider = &provider.String
	}
	if destType.Valid {
		resp.DestinationType = &destType.String
	}
	if len(destInfoJSON) > 0 {
		_ = json.Unmarshal(destInfoJSON, &resp.DestinationInfo)
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &resp.Metadata)
	}
	if adminComment.Valid {
		resp.AdminComment = &adminComment.String
	}
	if reviewedBy.Valid {
		resp.ReviewedBy = &reviewedBy.String
	}
	if reviewerEmail.Valid {
		resp.ReviewerEmail = &reviewerEmail.String
	}
	if reviewedAt.Valid {
		resp.ReviewedAt = &reviewedAt.Time
	}
	if completedAt.Valid {
		resp.CompletedAt = &completedAt.Time
	}

	// Get audit history (circuit breaker protected)
	auditResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT al.id, al.action, al.actor_user_id, u.email, COALESCE(al.payload_json::text, '{}'), al.created_at
				FROM audit_logs al
				LEFT JOIN users u ON u.id = al.actor_user_id
				WHERE al.target_type = 'payout' AND al.target_id = $1
				ORDER BY al.created_at DESC
				LIMIT 50
			`, withdrawalID)
		},
	)
	if err != nil {
		a.log().Warn("Failed to get withdrawal audit history", zap.Error(err))
	} else {
		auditRows := auditResult.(*sql.Rows)
		defer auditRows.Close()
		resp.AuditHistory = []WithdrawalAuditEntry{}
		for auditRows.Next() {
			var entry WithdrawalAuditEntry
			var actorEmail sql.NullString
			if err := auditRows.Scan(&entry.ID, &entry.Action, &entry.ActorID, &actorEmail, &entry.Details, &entry.CreatedAt); err != nil {
				continue
			}
			if actorEmail.Valid {
				entry.ActorEmail = actorEmail.String
			}
			resp.AuditHistory = append(resp.AuditHistory, entry)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleApproveWithdrawal approves a pending withdrawal.
// POST /api/admin/withdrawals/{id}/approve
func (a *App) handleApproveWithdrawal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	withdrawalID := chi.URLParam(r, "id")
	actorUserID := auth.GetUserID(ctx)

	if withdrawalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.WithdrawalIDRequired})
		return
	}

	var req WithdrawalActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Begin transaction
	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check withdrawal status and lock for update
	var currentStatus string
	var userID string
	var amountCents int64
	err = tx.QueryRowContext(ctx,
		`SELECT status, user_id, amount_cents FROM payouts WHERE id = $1 FOR UPDATE`,
		withdrawalID,
	).Scan(&currentStatus, &userID, &amountCents)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.WithdrawalNotFound})
			return
		}
		a.log().Error("Failed to get withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": adminMsg.WithdrawalNotPending,
		})
		return
	}

	// Update withdrawal to processing (or succeeded for manual payouts)
	_, err = tx.ExecContext(ctx, `
		UPDATE payouts
		SET status = 'processing',
		    admin_comment = COALESCE($1, admin_comment),
		    reviewed_by = $2,
		    reviewed_at = NOW(),
		    updated_at = NOW()
		WHERE id = $3
	`, sql.NullString{String: req.Comment, Valid: req.Comment != ""}, actorUserID, withdrawalID)

	if err != nil {
		a.log().Error("Failed to approve withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Create audit log entry
	auditPayload := map[string]interface{}{
		"withdrawal_id":   withdrawalID,
		"action":          "withdrawal.approved",
		"previous_status": currentStatus,
		"new_status":      "processing",
		"comment":         req.Comment,
		"user_id":         userID,
		"amount_cents":    amountCents,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "withdrawal.approved", "payout", withdrawalID, auditPayloadJSON)

	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Send email notification asynchronously
	if a.emailNotifier != nil {
		infra.SafeGo(a.log(), "withdrawal-approval-email", func() {
			asyncCtx, asyncCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer asyncCancel()
			// Get user email
			var userEmail, userName sql.NullString
			err := a.pool.Replica().QueryRowContext(asyncCtx,
				`SELECT email, full_name FROM users WHERE id = $1`, userID,
			).Scan(&userEmail, &userName)
			if err != nil {
				a.log().Error("Failed to get user email for withdrawal notification", zap.Error(err), zap.String("user_id", userID))
				return
			}

			if !userEmail.Valid || userEmail.String == "" {
				return
			}

			emailData := notification.WithdrawalApprovedData{
				UserName:     userName.String,
				Amount:       fmt.Sprintf("$%.2f", float64(amountCents)/100),
				AdminComment: req.Comment,
				DashboardURL: a.config.FrontendBaseURL + "/wallet",
			}

			emailEnabled, _ := prefs.IsEnabled(asyncCtx, a.pool.Replica(), userID, "withdrawal_update", "email")
			if !emailEnabled {
				return
			}

			if err := a.emailNotifier.SendWithdrawalApproved(asyncCtx, userEmail.String, emailData); err != nil {
				a.log().Error("Failed to send withdrawal approval email",
					zap.Error(err),
					zap.String("user_id", userID),
					zap.String("email", userEmail.String))
			} else {
				a.log().Info("Withdrawal approval email sent", zap.String("withdrawal_id", withdrawalID))
			}
		})
	}

	a.log().Info("Withdrawal approved",
		zap.String("withdrawal_id", withdrawalID),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":       adminMsg.WithdrawalApproved,
		"withdrawal_id": withdrawalID,
		"status":        "processing",
	})
}

// handleRejectWithdrawal rejects a pending withdrawal and refunds the user.
// POST /api/admin/withdrawals/{id}/reject
func (a *App) handleRejectWithdrawal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	withdrawalID := chi.URLParam(r, "id")
	actorUserID := auth.GetUserID(ctx)

	if withdrawalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.WithdrawalIDRequired})
		return
	}

	var req WithdrawalActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate rejection reason is provided
	v := validation.New()
	req.Comment = v.String("comment", req.Comment, validation.StringConstraints{
		Required:  true,
		MinLength: 1,
		MaxLength: 1000,
		TrimSpace: true,
	})
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Begin transaction
	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Check withdrawal status and lock for update
	var currentStatus string
	var userID string
	var amountCents int64
	var currency string
	var metadataJSON sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT status, user_id, amount_cents, currency, metadata_json FROM payouts WHERE id = $1 FOR UPDATE`,
		withdrawalID,
	).Scan(&currentStatus, &userID, &amountCents, &currency, &metadataJSON)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.WithdrawalNotFound})
			return
		}
		a.log().Error("Failed to get withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "pending" && currentStatus != "processing" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": adminMsg.WithdrawalCannotReject,
		})
		return
	}

	// Extract fee_cents from metadata (fee was deducted alongside the withdrawal amount)
	var feeCents int64
	if metadataJSON.Valid && metadataJSON.String != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
			if fc, ok := metadata["fee_cents"].(float64); ok {
				feeCents = int64(fc)
			}
		}
	}

	// Update withdrawal to rejected
	_, err = tx.ExecContext(ctx, `
		UPDATE payouts
		SET status = 'rejected',
		    admin_comment = $1,
		    reviewed_by = $2,
		    reviewed_at = NOW(),
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE id = $3
	`, req.Comment, actorUserID, withdrawalID)

	if err != nil {
		a.log().Error("Failed to reject withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Refund using wallet service with idempotency protection
	txWrapper := &wallet.TxAdapter{Tx: tx}
	refType := wallet.LedgerRefTypePayout
	refundDesc := "Withdrawal request rejected - funds returned"
	idempotencyKey := fmt.Sprintf("admin_rejection_refund:%s", withdrawalID)

	_, refundErr := a.walletService.CreditIdempotent(ctx, txWrapper, userID, amountCents,
		wallet.LedgerTypeWithdrawalRefund, &refType, &withdrawalID, &refundDesc, idempotencyKey)
	if refundErr != nil {
		if _, ok := refundErr.(*wallet.DuplicateCreditError); ok {
			a.log().Warn("Duplicate rejection refund detected", zap.String("withdrawal_id", withdrawalID))
		} else {
			a.log().Error("Failed to refund wallet", zap.Error(refundErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.RefundFailed})
			return
		}
	}

	// Refund fee if applicable
	if feeCents > 0 {
		feeDesc := "Withdrawal fee refunded - request rejected"
		feeIdempotencyKey := fmt.Sprintf("admin_rejection_fee_refund:%s", withdrawalID)
		_, feeErr := a.walletService.CreditIdempotent(ctx, txWrapper, userID, feeCents,
			wallet.LedgerTypeWithdrawFeeRefund, &refType, &withdrawalID, &feeDesc, feeIdempotencyKey)
		if feeErr != nil {
			if _, ok := feeErr.(*wallet.DuplicateCreditError); !ok {
				a.log().Error("Failed to refund fee", zap.Error(feeErr))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.RefundFeeFailed})
				return
			}
		}
	}

	totalRefunded := amountCents + feeCents

	// Create audit log entry
	auditPayload := map[string]interface{}{
		"withdrawal_id":        withdrawalID,
		"action":               "withdrawal.rejected",
		"previous_status":      currentStatus,
		"new_status":           "rejected",
		"reason":               req.Comment,
		"user_id":              userID,
		"amount_cents":         amountCents,
		"fee_cents":            feeCents,
		"total_refunded_cents": totalRefunded,
		"refunded":             true,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "withdrawal.rejected", "payout", withdrawalID, auditPayloadJSON)

	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Send email notification asynchronously
	if a.emailNotifier != nil {
		rejectionReason := req.Comment
		infra.SafeGo(a.log(), "withdrawal-rejection-email", func() {
			asyncCtx, asyncCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer asyncCancel()
			// Get user email
			var userEmail, userName sql.NullString
			err := a.pool.Replica().QueryRowContext(asyncCtx,
				`SELECT email, full_name FROM users WHERE id = $1`, userID,
			).Scan(&userEmail, &userName)
			if err != nil {
				a.log().Error("Failed to get user email for withdrawal notification", zap.Error(err), zap.String("user_id", userID))
				return
			}

			if !userEmail.Valid || userEmail.String == "" {
				return
			}

			emailData := notification.WithdrawalRejectedData{
				UserName:     userName.String,
				Amount:       fmt.Sprintf("$%.2f", float64(amountCents+feeCents)/100),
				Reason:       rejectionReason,
				DashboardURL: a.config.FrontendBaseURL + "/wallet",
			}

			emailEnabled, _ := prefs.IsEnabled(asyncCtx, a.pool.Replica(), userID, "withdrawal_update", "email")
			if !emailEnabled {
				return
			}

			if err := a.emailNotifier.SendWithdrawalRejected(asyncCtx, userEmail.String, emailData); err != nil {
				a.log().Error("Failed to send withdrawal rejection email",
					zap.Error(err),
					zap.String("user_id", userID),
					zap.String("email", userEmail.String))
			} else {
				a.log().Info("Withdrawal rejection email sent", zap.String("withdrawal_id", withdrawalID))
			}
		})
	}

	a.log().Info("Withdrawal rejected",
		zap.String("withdrawal_id", withdrawalID),
		zap.String("actor_id", actorUserID),
		zap.String("reason", req.Comment),
		zap.Int64("refunded_cents", amountCents),
		zap.Int64("fee_refunded_cents", feeCents),
		zap.Int64("total_refunded_cents", totalRefunded))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":              adminMsg.WithdrawalRejected,
		"withdrawal_id":        withdrawalID,
		"status":               "rejected",
		"refunded_cents":       amountCents,
		"fee_refunded_cents":   feeCents,
		"total_refunded_cents": totalRefunded,
	})
}

// handleAddWithdrawalComment adds an internal comment to a withdrawal without changing status.
// POST /api/admin/withdrawals/{id}/comment
func (a *App) handleAddWithdrawalComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	withdrawalID := chi.URLParam(r, "id")
	actorUserID := auth.GetUserID(ctx)

	if withdrawalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.WithdrawalIDRequired})
		return
	}

	var req WithdrawalActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate comment is provided
	v := validation.New()
	req.Comment = v.String("comment", req.Comment, validation.StringConstraints{
		Required:  true,
		MinLength: 1,
		MaxLength: 2000,
		TrimSpace: true,
	})
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Check if withdrawal exists (circuit breaker protected)
	var currentStatus string
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx,
			`SELECT status FROM payouts WHERE id = $1`, withdrawalID,
		).Scan(&currentStatus)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.WithdrawalNotFound})
			return
		}
		a.log().Error("Failed to get withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Update the comment (circuit breaker protected)
	err = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx, `
			UPDATE payouts
			SET admin_comment = $1,
			    updated_at = NOW()
			WHERE id = $2
		`, req.Comment, withdrawalID)
		return execErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to update withdrawal comment", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Create audit log entry (circuit breaker protected)
	auditPayload := map[string]interface{}{
		"withdrawal_id": withdrawalID,
		"action":        "withdrawal.commented",
		"comment":       req.Comment,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	_ = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx,
			`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
			 VALUES ($1, $2, $3, $4, $5)`,
			actorUserID, "withdrawal.commented", "payout", withdrawalID, auditPayloadJSON)
		return execErr
	})

	a.log().Info("Withdrawal comment added",
		zap.String("withdrawal_id", withdrawalID),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":       adminMsg.CommentAdded,
		"withdrawal_id": withdrawalID,
	})
}

// handleCompleteWithdrawal marks a processing withdrawal as completed.
// This is used after the admin has manually executed the bank transfer or crypto payout.
// POST /api/admin/withdrawals/{id}/complete
func (a *App) handleCompleteWithdrawal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	withdrawalID := chi.URLParam(r, "id")
	actorUserID := auth.GetUserID(ctx)

	if withdrawalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.WithdrawalIDRequired})
		return
	}

	var req struct {
		Comment       string `json:"comment"`
		TransactionID string `json:"transaction_id"` // Bank ref number or crypto tx hash
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}
	req.Comment = validation.SanitizeString(req.Comment)
	req.TransactionID = validation.SanitizeString(req.TransactionID)
	if strings.TrimSpace(req.Comment) == "" {
		a.auditSensitiveDenial(ctx, actorUserID, actionWithdrawalComplete, withdrawalID, "mandatory_reason_denied")
		validation.WriteBadRequest(w, "reason is required")
		return
	}
	if strings.TrimSpace(req.TransactionID) == "" {
		a.auditSensitiveDenial(ctx, actorUserID, actionWithdrawalComplete, withdrawalID, "required_reference_denied")
		validation.WriteBadRequest(w, "transaction_id is required")
		return
	}

	// Begin transaction
	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Lock and check current status
	var currentStatus string
	var userID string
	var amountCents int64
	var currency string
	err = tx.QueryRowContext(ctx,
		`SELECT status, user_id, amount_cents, currency FROM payouts WHERE id = $1 FOR UPDATE`,
		withdrawalID,
	).Scan(&currentStatus, &userID, &amountCents, &currency)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.WithdrawalNotFound})
			return
		}
		a.log().Error("Failed to get withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "processing" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": adminMsg.WithdrawalMustBeProcessing,
		})
		return
	}

	// Build comment with transaction ID if provided
	comment := req.Comment
	if req.TransactionID != "" {
		if comment != "" {
			comment = fmt.Sprintf("%s | Tx: %s", comment, req.TransactionID)
		} else {
			comment = fmt.Sprintf("Tx: %s", req.TransactionID)
		}
	}

	// Update payout to succeeded
	_, err = tx.ExecContext(ctx, `
		UPDATE payouts
		SET status = 'succeeded',
		    admin_comment = COALESCE(NULLIF($1, ''), admin_comment),
		    reviewed_by = $2,
		    reviewed_at = NOW(),
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE id = $3
	`, comment, actorUserID, withdrawalID)

	if err != nil {
		a.log().Error("Failed to complete withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Audit log
	auditPayload := map[string]interface{}{
		"withdrawal_id":   withdrawalID,
		"action":          "withdrawal.completed",
		"previous_status": currentStatus,
		"new_status":      "succeeded",
		"comment":         comment,
		"transaction_id":  req.TransactionID,
		"user_id":         userID,
		"amount_cents":    amountCents,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "withdrawal.completed", "payout", withdrawalID, auditPayloadJSON)

	if err != nil {
		a.log().Error("Failed to write audit log", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Send email notification asynchronously
	if a.emailNotifier != nil {
		infra.SafeGo(a.log(), "withdrawal-completed-email", func() {
			asyncCtx, asyncCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer asyncCancel()
			var userEmail, userName sql.NullString
			err := a.pool.Replica().QueryRowContext(asyncCtx,
				`SELECT email, full_name FROM users WHERE id = $1`, userID,
			).Scan(&userEmail, &userName)
			if err != nil || !userEmail.Valid || userEmail.String == "" {
				return
			}

			emailEnabled, _ := prefs.IsEnabled(asyncCtx, a.pool.Replica(), userID, "withdrawal_update", "email")
			if !emailEnabled {
				return
			}

			emailData := notification.WithdrawalCompletedData{
				UserName:     userName.String,
				Amount:       fmt.Sprintf("$%.2f", float64(amountCents)/100),
				DashboardURL: a.config.FrontendBaseURL + "/wallet",
			}

			if err := a.emailNotifier.SendWithdrawalCompleted(asyncCtx, userEmail.String, emailData); err != nil {
				a.log().Error("Failed to send withdrawal completed email",
					zap.Error(err), zap.String("user_id", userID))
			}
		})
	}

	a.log().Info("Withdrawal completed",
		zap.String("withdrawal_id", withdrawalID),
		zap.String("actor_id", actorUserID),
		zap.String("transaction_id", req.TransactionID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":       adminMsg.WithdrawalCompleted,
		"withdrawal_id": withdrawalID,
		"status":        "succeeded",
	})
}

// handleFailWithdrawal marks a processing withdrawal as failed and refunds the user.
// Used when admin tried to execute the payout but it failed at the bank/provider level.
// POST /api/admin/withdrawals/{id}/fail
func (a *App) handleFailWithdrawal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	withdrawalID := chi.URLParam(r, "id")
	actorUserID := auth.GetUserID(ctx)

	if withdrawalID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.WithdrawalIDRequired})
		return
	}

	var req WithdrawalActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Require reason
	v := validation.New()
	req.Comment = v.String("comment", req.Comment, validation.StringConstraints{
		Required:  true,
		MinLength: 1,
		MaxLength: 1000,
		TrimSpace: true,
	})
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	// Begin transaction
	var tx *sql.Tx
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var beginErr error
		tx, beginErr = a.pool.Primary().BeginTx(ctx, nil)
		return beginErr
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to begin transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	defer tx.Rollback()

	// Lock and check
	var currentStatus string
	var userID string
	var amountCents int64
	var currency string
	var metadataJSON sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT status, user_id, amount_cents, currency, metadata_json FROM payouts WHERE id = $1 FOR UPDATE`,
		withdrawalID,
	).Scan(&currentStatus, &userID, &amountCents, &currency, &metadataJSON)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.WithdrawalNotFound})
			return
		}
		a.log().Error("Failed to get withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if currentStatus != "processing" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": adminMsg.WithdrawalMustBeProcessing,
		})
		return
	}

	// Extract fee
	var feeCents int64
	if metadataJSON.Valid && metadataJSON.String != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
			if fc, ok := metadata["fee_cents"].(float64); ok {
				feeCents = int64(fc)
			}
		}
	}

	// Update to failed
	_, err = tx.ExecContext(ctx, `
		UPDATE payouts
		SET status = 'failed',
		    admin_comment = $1,
		    reviewed_by = $2,
		    reviewed_at = NOW(),
		    updated_at = NOW(),
		    completed_at = NOW()
		WHERE id = $3
	`, req.Comment, actorUserID, withdrawalID)

	if err != nil {
		a.log().Error("Failed to fail withdrawal", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Refund using wallet service with idempotency
	txWrapper := &wallet.TxAdapter{Tx: tx}
	refType := wallet.LedgerRefTypePayout
	refundDesc := "Withdrawal failed - funds returned"
	idempotencyKey := fmt.Sprintf("admin_withdrawal_refund:%s", withdrawalID)

	_, refundErr := a.walletService.CreditIdempotent(ctx, txWrapper, userID, amountCents,
		wallet.LedgerTypeWithdrawalRefund, &refType, &withdrawalID, &refundDesc, idempotencyKey)
	if refundErr != nil {
		if _, ok := refundErr.(*wallet.DuplicateCreditError); ok {
			a.log().Warn("Duplicate refund detected", zap.String("withdrawal_id", withdrawalID))
		} else {
			a.log().Error("Failed to refund wallet", zap.Error(refundErr))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.RefundFailed})
			return
		}
	}

	// Refund fee if applicable
	if feeCents > 0 {
		feeDesc := "Withdrawal fee refunded - payout failed"
		feeIdempotencyKey := fmt.Sprintf("admin_withdrawal_fee_refund:%s", withdrawalID)
		_, feeErr := a.walletService.CreditIdempotent(ctx, txWrapper, userID, feeCents,
			wallet.LedgerTypeWithdrawFeeRefund, &refType, &withdrawalID, &feeDesc, feeIdempotencyKey)
		if feeErr != nil {
			if _, ok := feeErr.(*wallet.DuplicateCreditError); !ok {
				a.log().Error("Failed to refund fee", zap.Error(feeErr))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.RefundFeeFailed})
				return
			}
		}
	}

	totalRefunded := amountCents + feeCents

	// Audit log
	auditPayload := map[string]interface{}{
		"withdrawal_id":        withdrawalID,
		"action":               "withdrawal.failed",
		"previous_status":      currentStatus,
		"new_status":           "failed",
		"reason":               req.Comment,
		"user_id":              userID,
		"amount_cents":         amountCents,
		"fee_cents":            feeCents,
		"total_refunded_cents": totalRefunded,
	}
	auditPayloadJSON, _ := json.Marshal(auditPayload)
	_, _ = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "withdrawal.failed", "payout", withdrawalID, auditPayloadJSON)

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Send email notification
	if a.emailNotifier != nil {
		infra.SafeGo(a.log(), "withdrawal-failed-email", func() {
			asyncCtx, asyncCancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer asyncCancel()
			var userEmail, userName sql.NullString
			err := a.pool.Replica().QueryRowContext(asyncCtx,
				`SELECT email, full_name FROM users WHERE id = $1`, userID,
			).Scan(&userEmail, &userName)
			if err != nil || !userEmail.Valid || userEmail.String == "" {
				return
			}

			emailEnabled, _ := prefs.IsEnabled(asyncCtx, a.pool.Replica(), userID, "withdrawal_update", "email")
			if !emailEnabled {
				return
			}

			// Reuse rejection template Ã¢â‚¬â€ failed payout has the same user impact
			emailData := notification.WithdrawalRejectedData{
				UserName:     userName.String,
				Amount:       fmt.Sprintf("$%.2f", float64(amountCents)/100),
				Reason:       "Payout could not be processed. Funds have been returned to your wallet.",
				DashboardURL: a.config.FrontendBaseURL + "/wallet",
			}

			if err := a.emailNotifier.SendWithdrawalRejected(asyncCtx, userEmail.String, emailData); err != nil {
				a.log().Error("Failed to send withdrawal failed email",
					zap.Error(err), zap.String("user_id", userID))
			}
		})
	}

	a.log().Info("Withdrawal marked as failed and refunded",
		zap.String("withdrawal_id", withdrawalID),
		zap.String("actor_id", actorUserID),
		zap.Int64("refunded_cents", totalRefunded))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":        adminMsg.WithdrawalFailed,
		"withdrawal_id":  withdrawalID,
		"status":         "failed",
		"refunded_cents": totalRefunded,
	})
}
