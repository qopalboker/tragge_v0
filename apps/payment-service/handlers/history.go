package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/kyc"
	"go.uber.org/zap"
)

// validPaymentStatuses is the set of known payment statuses accepted by the
// history filter. Requests with unknown status values get an empty result.
var validPaymentStatuses = map[string]bool{
	"pending":    true,
	"processing": true,
	"succeeded":  true,
	"failed":     true,
	"refunded":   true,
	"expired":    true,
	"cancelled":  true,
}

// HistoryHandler handles payment history requests
type HistoryHandler struct {
	db         *sql.DB
	kycService *kyc.Service
	logger     *zap.Logger
	circuits   DatabaseCircuitExecutor
}

// NewHistoryHandler creates a new history handler
func NewHistoryHandler(db *sql.DB, kycService *kyc.Service, logger *zap.Logger, circuits DatabaseCircuitExecutor) *HistoryHandler {
	return &HistoryHandler{
		db:         db,
		kycService: kycService,
		logger:     logger,
		circuits:   circuits,
	}
}

// PaymentHistoryEntry represents a single payment history entry
type PaymentHistoryEntry struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	AmountCents   int64   `json:"amount_cents"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	Status        string  `json:"status"`
	Description   string  `json:"description"`
	PaymentMethod string  `json:"payment_method,omitempty"`
	Provider      string  `json:"provider,omitempty"`
	CreatedAt     string  `json:"created_at"`
	CompletedAt   string  `json:"completed_at,omitempty"`
}

// PaymentHistoryResponse represents the response for payment history
type PaymentHistoryResponse struct {
	Transactions []PaymentHistoryEntry `json:"transactions"`
	Total        int                   `json:"total"`
	Page         int                   `json:"page"`
	PerPage      int                   `json:"per_page"`
	HasMore      bool                  `json:"has_more"`
}

// HandleGetHistory handles GET /api/payments/history
func (h *HistoryHandler) HandleGetHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Parse pagination - support both page/per_page and limit/offset
	page := 0
	perPage := 0
	limit := 50
	offset := 0

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if perPageStr := r.URL.Query().Get("per_page"); perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 && pp <= 100 {
			perPage = pp
		}
	}

	if page > 0 {
		// page/per_page mode
		if perPage == 0 {
			perPage = 10
		}
		limit = perPage
		offset = (page - 1) * perPage
	} else {
		// limit/offset mode
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
		perPage = limit
		if limit > 0 {
			page = (offset / limit) + 1
		} else {
			page = 1
		}
	}

	// Parse filter params
	typeFilter := r.URL.Query().Get("type")
	statusFilter := r.URL.Query().Get("status")

	// Validate status filter — only allow known payment statuses to prevent
	// querying with arbitrary strings (defense-in-depth).
	if statusFilter != "" && !validPaymentStatuses[statusFilter] {
		writeJSON(w, http.StatusOK, PaymentHistoryResponse{
			Transactions: []PaymentHistoryEntry{},
			Total:        0,
			Page:         page,
			PerPage:      perPage,
			HasMore:      false,
		})
		return
	}

	// Fetch entries with proper SQL-level pagination
	entries, err := h.fetchEntries(ctx, userID, typeFilter, statusFilter, limit, offset)
	if err != nil {
		h.logger.Error("Failed to fetch payment history", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Get total count
	totalCount, err := h.countEntries(ctx, userID, typeFilter, statusFilter)
	if err != nil {
		h.logger.Error("Failed to count payment history", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// Enrich entries with computed fields
	for i := range entries {
		amt := float64(entries[i].AmountCents) / 100.0
		if entries[i].Type == "withdrawal" {
			amt = -amt
		}
		entries[i].Amount = amt
		entries[i].Description = generateDescription(entries[i].Type, entries[i].Provider)
		entries[i].PaymentMethod = mapPaymentMethod(entries[i].Provider)
	}

	// Ensure non-nil slice for JSON serialization
	if entries == nil {
		entries = []PaymentHistoryEntry{}
	}

	hasMore := offset+len(entries) < totalCount

	writeJSON(w, http.StatusOK, PaymentHistoryResponse{
		Transactions: entries,
		Total:        totalCount,
		Page:         page,
		PerPage:      perPage,
		HasMore:      hasMore,
	})
}

// fetchEntries retrieves payment history entries based on type filter
func (h *HistoryHandler) fetchEntries(ctx context.Context, userID, typeFilter, statusFilter string, limit, offset int) ([]PaymentHistoryEntry, error) {
	switch typeFilter {
	case "deposit":
		return h.querySingleTable(ctx, "payment_intents", "deposit", userID, statusFilter, limit, offset)
	case "withdrawal":
		return h.querySingleTable(ctx, "payouts", "withdrawal", userID, statusFilter, limit, offset)
	case "":
		return h.queryCombined(ctx, userID, statusFilter, limit, offset)
	default:
		// Unknown type filter - no matching entries
		return []PaymentHistoryEntry{}, nil
	}
}

// querySingleTable queries a single table (payment_intents or payouts) for entries
func (h *HistoryHandler) querySingleTable(ctx context.Context, table, entryType, userID, statusFilter string, limit, offset int) ([]PaymentHistoryEntry, error) {
	query := fmt.Sprintf(`
		SELECT id, amount_cents, currency, status, provider, created_at, completed_at
		FROM %s
		WHERE user_id = $1
	`, table)
	args := []interface{}{userID}
	argIdx := 2

	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	return h.scanRows(ctx, query, entryType, args...)
}

// queryCombined queries both tables using UNION ALL with proper SQL-level pagination
func (h *HistoryHandler) queryCombined(ctx context.Context, userID, statusFilter string, limit, offset int) ([]PaymentHistoryEntry, error) {
	var query string
	var args []interface{}

	if statusFilter != "" {
		query = `
			(SELECT id, amount_cents, currency, status, provider, created_at, completed_at, 'deposit'::text as type
			 FROM payment_intents WHERE user_id = $1 AND status = $2)
			UNION ALL
			(SELECT id, amount_cents, currency, status, provider, created_at, completed_at, 'withdrawal'::text as type
			 FROM payouts WHERE user_id = $1 AND status = $2)
			ORDER BY created_at DESC
			LIMIT $3 OFFSET $4
		`
		args = []interface{}{userID, statusFilter, limit, offset}
	} else {
		query = `
			(SELECT id, amount_cents, currency, status, provider, created_at, completed_at, 'deposit'::text as type
			 FROM payment_intents WHERE user_id = $1)
			UNION ALL
			(SELECT id, amount_cents, currency, status, provider, created_at, completed_at, 'withdrawal'::text as type
			 FROM payouts WHERE user_id = $1)
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`
		args = []interface{}{userID, limit, offset}
	}

	var rows *sql.Rows
	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var e error
		rows, e = h.db.QueryContext(ctx, query, args...)
		return e
	})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []PaymentHistoryEntry
	for rows.Next() {
		var entry PaymentHistoryEntry
		var provider sql.NullString
		var completedAt sql.NullTime
		var createdAt time.Time

		if err := rows.Scan(&entry.ID, &entry.AmountCents, &entry.Currency, &entry.Status, &provider, &createdAt, &completedAt, &entry.Type); err != nil {
			return nil, err
		}

		entry.CreatedAt = createdAt.Format(time.RFC3339)
		if provider.Valid {
			entry.Provider = provider.String
		}
		if completedAt.Valid {
			entry.CompletedAt = completedAt.Time.Format(time.RFC3339)
		}

		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// scanRows scans rows from a single-table query
func (h *HistoryHandler) scanRows(ctx context.Context, query, entryType string, args ...interface{}) ([]PaymentHistoryEntry, error) {
	var rows *sql.Rows
	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		var e error
		rows, e = h.db.QueryContext(ctx, query, args...)
		return e
	})
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []PaymentHistoryEntry
	for rows.Next() {
		var entry PaymentHistoryEntry
		var provider sql.NullString
		var completedAt sql.NullTime
		var createdAt time.Time

		if err := rows.Scan(&entry.ID, &entry.AmountCents, &entry.Currency, &entry.Status, &provider, &createdAt, &completedAt); err != nil {
			return nil, err
		}

		entry.Type = entryType
		entry.CreatedAt = createdAt.Format(time.RFC3339)
		if provider.Valid {
			entry.Provider = provider.String
		}
		if completedAt.Valid {
			entry.CompletedAt = completedAt.Time.Format(time.RFC3339)
		}

		entries = append(entries, entry)
	}

	return entries, rows.Err()
}

// countEntries counts total entries matching the filters
func (h *HistoryHandler) countEntries(ctx context.Context, userID, typeFilter, statusFilter string) (int, error) {
	var query string
	var args []interface{}

	statusClause := ""
	if statusFilter != "" {
		statusClause = " AND status = $2"
	}

	switch typeFilter {
	case "deposit":
		query = fmt.Sprintf("SELECT COUNT(*) FROM payment_intents WHERE user_id = $1%s", statusClause)
		args = []interface{}{userID}
		if statusFilter != "" {
			args = append(args, statusFilter)
		}
	case "withdrawal":
		query = fmt.Sprintf("SELECT COUNT(*) FROM payouts WHERE user_id = $1%s", statusClause)
		args = []interface{}{userID}
		if statusFilter != "" {
			args = append(args, statusFilter)
		}
	case "":
		if statusFilter != "" {
			query = `SELECT
				(SELECT COUNT(*) FROM payment_intents WHERE user_id = $1 AND status = $2) +
				(SELECT COUNT(*) FROM payouts WHERE user_id = $1 AND status = $2)`
			args = []interface{}{userID, statusFilter}
		} else {
			query = `SELECT
				(SELECT COUNT(*) FROM payment_intents WHERE user_id = $1) +
				(SELECT COUNT(*) FROM payouts WHERE user_id = $1)`
			args = []interface{}{userID}
		}
	default:
		// Unknown type filter
		return 0, nil
	}

	var count int
	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return h.db.QueryRowContext(ctx, query, args...).Scan(&count)
	})
	return count, err
}

// generateDescription creates a human-readable description for a history entry
func generateDescription(entryType, provider string) string {
	providerName := ""
	switch provider {
	case "jibit":
		providerName = "Jibit"
	case "nowpayments":
		providerName = "NOWPayments"
	case "stripe":
		providerName = "Stripe"
	}

	switch entryType {
	case "deposit":
		if providerName != "" {
			return "Deposit via " + providerName
		}
		return "Deposit"
	case "withdrawal":
		if providerName != "" {
			return "Withdrawal via " + providerName
		}
		return "Withdrawal"
	default:
		return entryType
	}
}

// mapPaymentMethod maps a provider to a user-friendly payment method name
func mapPaymentMethod(provider string) string {
	switch provider {
	case "jibit":
		return "Bank Transfer"
	case "nowpayments":
		return "Cryptocurrency"
	case "stripe":
		return "Credit Card"
	default:
		return ""
	}
}

// WalletResponse represents the response for wallet balance
type WalletResponse struct {
	UserID                 string `json:"user_id"`
	BalanceCents           int64  `json:"balance_cents"`
	Currency               string `json:"currency"`
	Status                 string `json:"status"`
	KYCStatus              string `json:"kyc_status"`
	KYCRequiredForWithdraw bool   `json:"kyc_required_for_withdraw"`
	CanWithdraw            bool   `json:"can_withdraw"`
}

// HandleGetWallet handles GET /api/wallet
func (h *HistoryHandler) HandleGetWallet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var walletData struct {
		UserID       string
		BalanceCents int64
		Currency     string
		Status       string
	}

	err := h.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return h.db.QueryRowContext(ctx, `
			SELECT user_id, balance_cents, currency, status
			FROM wallets
			WHERE user_id = $1
		`, userID).Scan(&walletData.UserID, &walletData.BalanceCents, &walletData.Currency, &walletData.Status)
	})

	// Get KYC status
	kycStatus := kyc.StatusNone
	canWithdraw := false

	if h.kycService != nil {
		kycResult, kycErr := h.kycService.CheckVerification(ctx, userID)
		if kycErr != nil {
			h.logger.Error("Failed to check KYC status", zap.Error(kycErr))
			// Continue without KYC info rather than failing the request
		} else {
			kycStatus = kycResult.Status
			canWithdraw = kycResult.Verified
		}
	}

	if err != nil {
		if err == sql.ErrNoRows {
			// Wallet doesn't exist - return empty wallet with KYC status
			writeJSON(w, http.StatusOK, WalletResponse{
				UserID:                 userID,
				BalanceCents:           0,
				Currency:               "USD",
				Status:                 "active",
				KYCStatus:              string(kycStatus),
				KYCRequiredForWithdraw: true,
				CanWithdraw:            canWithdraw,
			})
			return
		}
		h.logger.Error("Failed to get wallet", zap.Error(err))
		writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// User can withdraw if wallet is active and KYC is verified
	canWithdraw = canWithdraw && walletData.Status == "active"

	writeJSON(w, http.StatusOK, WalletResponse{
		UserID:                 walletData.UserID,
		BalanceCents:           walletData.BalanceCents,
		Currency:               walletData.Currency,
		Status:                 walletData.Status,
		KYCStatus:              string(kycStatus),
		KYCRequiredForWithdraw: true,
		CanWithdraw:            canWithdraw,
	})
}
