package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Helper functions

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func itoa(i int) string {
	return strconv.Itoa(i)
}

func joinStrings(strs []string, sep string) string {
	return strings.Join(strs, sep)
}

// pqArray is a helper type for scanning PostgreSQL arrays
type pqArray []sql.NullString

func (a *pqArray) Scan(src interface{}) error {
	if src == nil {
		*a = nil
		return nil
	}

	switch v := src.(type) {
	case []byte:
		return a.scanString(string(v))
	case string:
		return a.scanString(v)
	default:
		return fmt.Errorf("pqArray: cannot convert %T to []sql.NullString", src)
	}
}

func (a *pqArray) scanString(s string) error {
	// Handle empty array
	if s == "{}" || s == "" {
		*a = []sql.NullString{}
		return nil
	}

	// Remove curly braces
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")

	if s == "" {
		*a = []sql.NullString{}
		return nil
	}

	// Split by comma
	parts := strings.Split(s, ",")
	result := make([]sql.NullString, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		// Handle NULL
		if p == "NULL" {
			result[i] = sql.NullString{Valid: false}
		} else {
			// Remove quotes if present
			p = strings.Trim(p, "\"")
			result[i] = sql.NullString{String: p, Valid: true}
		}
	}
	*a = result
	return nil
}

// =============================================================================
// Financial Reports Handlers
// =============================================================================

// FinancialDataPoint represents a single data point in a time series
type FinancialDataPoint struct {
	Date        string `json:"date"`
	AmountCents int64  `json:"amount_cents"`
}

// FinancialSummaryResponse represents the financial summary response
type FinancialSummaryResponse struct {
	Deposits    []FinancialDataPoint `json:"deposits"`
	Withdrawals []FinancialDataPoint `json:"withdrawals"`
	EntryFees   []FinancialDataPoint `json:"entry_fees"`
	PrizesPaid  []FinancialDataPoint `json:"prizes_paid"`
	NetRevenue  []FinancialDataPoint `json:"net_revenue"`
	Totals      FinancialTotals      `json:"totals"`
}

// FinancialTotals represents aggregated financial totals
type FinancialTotals struct {
	TotalDepositsCents    int64 `json:"total_deposits_cents"`
	TotalWithdrawalsCents int64 `json:"total_withdrawals_cents"`
	TotalEntryFeesCents   int64 `json:"total_entry_fees_cents"`
	TotalPrizesPaidCents  int64 `json:"total_prizes_paid_cents"`
	NetRevenueCents       int64 `json:"net_revenue_cents"`
}

// DepositItem represents a deposit in the list
type DepositItem struct {
	ID          string      `json:"id"`
	User        DepositUser `json:"user"`
	AmountCents int64       `json:"amount_cents"`
	Currency    string      `json:"currency"`
	Provider    string      `json:"provider"`
	Status      string      `json:"status"`
	CreatedAt   time.Time   `json:"created_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
}

// DepositUser represents user info in deposit
type DepositUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
}

// DepositListResponse represents the deposit list response
type DepositListResponse struct {
	Deposits []DepositItem `json:"deposits"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PerPage  int           `json:"per_page"`
}

// TransactionItem represents a transaction in the list
type TransactionItem struct {
	ID          string          `json:"id"`
	User        TransactionUser `json:"user"`
	Type        string          `json:"type"`
	AmountCents int64           `json:"amount_cents"`
	RefType     string          `json:"ref_type,omitempty"`
	RefID       string          `json:"ref_id,omitempty"`
	Description string          `json:"description,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// TransactionUser represents user info in transaction
type TransactionUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
}

// TransactionListResponse represents the transaction list response
type TransactionListResponse struct {
	Transactions []TransactionItem `json:"transactions"`
	Total        int               `json:"total"`
	Page         int               `json:"page"`
	PerPage      int               `json:"per_page"`
}

// handleGetFinancialSummary returns aggregated financial data for charts
func (a *App) handleGetFinancialSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	granularity := r.URL.Query().Get("granularity")

	// Default to last 30 days
	to := time.Now()
	from := to.AddDate(0, 0, -30)

	if fromStr != "" {
		if parsed, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = parsed
		}
	}
	if toStr != "" {
		if parsed, err := time.Parse("2006-01-02", toStr); err == nil {
			to = parsed.Add(24*time.Hour - time.Second) // End of day
		}
	}

	// Default granularity is day
	if granularity == "" {
		granularity = "day"
	}

	// Determine date_trunc format
	var truncFmt string
	switch granularity {
	case "week":
		truncFmt = "week"
	case "month":
		truncFmt = "month"
	default:
		truncFmt = "day"
	}

	// Query deposits grouped by date
	depositsQuery := fmt.Sprintf(`
		SELECT date_trunc('%s', created_at) as period, COALESCE(SUM(amount_cents), 0) as total
		FROM payment_intents
		WHERE status = 'succeeded'
		AND created_at >= $1 AND created_at <= $2
		GROUP BY period
		ORDER BY period
	`, truncFmt)

	deposits, err := a.queryFinancialDataPoints(ctx, depositsQuery, from, to)
	if err != nil {
		a.log().Error("Failed to query deposits", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.DepositsFailed})
		return
	}

	// Query withdrawals grouped by date
	withdrawalsQuery := fmt.Sprintf(`
		SELECT date_trunc('%s', created_at) as period, COALESCE(SUM(amount_cents), 0) as total
		FROM payouts
		WHERE status IN ('succeeded', 'processing')
		AND created_at >= $1 AND created_at <= $2
		GROUP BY period
		ORDER BY period
	`, truncFmt)

	withdrawals, err := a.queryFinancialDataPoints(ctx, withdrawalsQuery, from, to)
	if err != nil {
		a.log().Error("Failed to query withdrawals", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.WithdrawalsFailed})
		return
	}

	// Query entry fees grouped by date
	entryFeesQuery := fmt.Sprintf(`
		SELECT date_trunc('%s', created_at) as period, COALESCE(SUM(amount_cents), 0) as total
		FROM wallet_ledger
		WHERE type = 'contest_entry'
		AND created_at >= $1 AND created_at <= $2
		GROUP BY period
		ORDER BY period
	`, truncFmt)

	entryFees, err := a.queryFinancialDataPoints(ctx, entryFeesQuery, from, to)
	if err != nil {
		a.log().Error("Failed to query entry fees", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.EntryFeesFailed})
		return
	}

	// Query prizes paid grouped by date
	prizesQuery := fmt.Sprintf(`
		SELECT date_trunc('%s', created_at) as period, COALESCE(SUM(ABS(amount_cents)), 0) as total
		FROM wallet_ledger
		WHERE type = 'prize'
		AND created_at >= $1 AND created_at <= $2
		GROUP BY period
		ORDER BY period
	`, truncFmt)

	prizesPaid, err := a.queryFinancialDataPoints(ctx, prizesQuery, from, to)
	if err != nil {
		a.log().Error("Failed to query prizes", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.PrizesFailed})
		return
	}

	// Calculate totals
	var totals FinancialTotals
	for _, d := range deposits {
		totals.TotalDepositsCents += d.AmountCents
	}
	for _, w := range withdrawals {
		totals.TotalWithdrawalsCents += w.AmountCents
	}
	for _, e := range entryFees {
		totals.TotalEntryFeesCents += e.AmountCents
	}
	for _, p := range prizesPaid {
		totals.TotalPrizesPaidCents += p.AmountCents
	}
	totals.NetRevenueCents = totals.TotalDepositsCents + totals.TotalEntryFeesCents - totals.TotalWithdrawalsCents - totals.TotalPrizesPaidCents

	// Calculate net revenue per period (deposits + entry_fees - withdrawals - prizes)
	netRevenue := a.calculateNetRevenue(deposits, withdrawals, entryFees, prizesPaid)

	response := FinancialSummaryResponse{
		Deposits:    deposits,
		Withdrawals: withdrawals,
		EntryFees:   entryFees,
		PrizesPaid:  prizesPaid,
		NetRevenue:  netRevenue,
		Totals:      totals,
	}

	writeJSON(w, http.StatusOK, response)
}

// queryFinancialDataPoints executes a query and returns data points
func (a *App) queryFinancialDataPoints(ctx context.Context, query string, from, to time.Time) ([]FinancialDataPoint, error) {
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, query, from, to)
		},
	)
	if err != nil {
		return nil, err
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var points []FinancialDataPoint
	for rows.Next() {
		var period time.Time
		var total int64
		if err := rows.Scan(&period, &total); err != nil {
			return nil, err
		}
		points = append(points, FinancialDataPoint{
			Date:        period.Format("2006-01-02"),
			AmountCents: total,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return points, nil
}

// calculateNetRevenue calculates net revenue per period
func (a *App) calculateNetRevenue(deposits, withdrawals, entryFees, prizesPaid []FinancialDataPoint) []FinancialDataPoint {
	// Create a map of all dates
	dateMap := make(map[string]int64)

	for _, d := range deposits {
		dateMap[d.Date] += d.AmountCents
	}
	for _, e := range entryFees {
		dateMap[e.Date] += e.AmountCents
	}
	for _, w := range withdrawals {
		dateMap[w.Date] -= w.AmountCents
	}
	for _, p := range prizesPaid {
		dateMap[p.Date] -= p.AmountCents
	}

	// Convert to sorted slice
	var result []FinancialDataPoint
	for date, amount := range dateMap {
		result = append(result, FinancialDataPoint{
			Date:        date,
			AmountCents: amount,
		})
	}

	// Sort by date (ISO 8601 format ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â lexicographic = chronological)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})

	return result
}

// handleListDeposits returns a paginated list of deposits
func (a *App) handleListDeposits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	page := 1
	perPage := 20

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			perPage = l
		}
	}

	status := r.URL.Query().Get("status")
	userID := r.URL.Query().Get("user_id")

	// Build query
	baseQuery := `
		FROM payment_intents p
		JOIN users u ON p.user_id = u.id
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	if status != "" {
		baseQuery += fmt.Sprintf(" AND p.status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}
	if userID != "" {
		baseQuery += fmt.Sprintf(" AND p.user_id = $%d", argIdx)
		args = append(args, userID)
		argIdx++
	}

	// Count total (circuit breaker protected)
	countQuery := "SELECT COUNT(*) " + baseQuery
	var total int
	if err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, countQuery, args...).Scan(&total)
	}); err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to count deposits", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.DepositsFailed})
		return
	}

	// Query deposits (circuit breaker protected)
	offset := (page - 1) * perPage
	selectQuery := fmt.Sprintf(`
		SELECT p.id, p.user_id, u.email, COALESCE(u.username, ''), p.amount_cents, p.currency, COALESCE(p.provider, ''), p.status, p.created_at, p.completed_at
		%s
		ORDER BY p.created_at DESC
		LIMIT $%d OFFSET $%d
	`, baseQuery, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, selectQuery, args...)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query deposits", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.DepositsFailed})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var deposits []DepositItem
	for rows.Next() {
		var d DepositItem
		var completedAt sql.NullTime
		if err := rows.Scan(
			&d.ID, &d.User.ID, &d.User.Email, &d.User.Username,
			&d.AmountCents, &d.Currency, &d.Provider, &d.Status,
			&d.CreatedAt, &completedAt,
		); err != nil {
			a.log().Error("Failed to scan deposit", zap.Error(err))
			continue
		}
		if completedAt.Valid {
			d.CompletedAt = &completedAt.Time
		}
		deposits = append(deposits, d)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate deposits", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.DepositsFailed})
		return
	}

	response := DepositListResponse{
		Deposits: deposits,
		Total:    total,
		Page:     page,
		PerPage:  perPage,
	}

	writeJSON(w, http.StatusOK, response)
}

// handleListTransactions returns a paginated list of all wallet transactions
func (a *App) handleListTransactions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	page := 1
	perPage := 20

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			perPage = l
		}
	}

	txType := r.URL.Query().Get("type")
	userID := r.URL.Query().Get("user_id")

	// Build query
	baseQuery := `
		FROM wallet_ledger l
		JOIN users u ON l.user_id = u.id
		WHERE 1=1
	`
	args := []interface{}{}
	argIdx := 1

	if txType != "" {
		baseQuery += fmt.Sprintf(" AND l.type = $%d", argIdx)
		args = append(args, txType)
		argIdx++
	}
	if userID != "" {
		baseQuery += fmt.Sprintf(" AND l.user_id = $%d", argIdx)
		args = append(args, userID)
		argIdx++
	}

	// Count total (circuit breaker protected)
	countQuery := "SELECT COUNT(*) " + baseQuery
	var total int
	if err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, countQuery, args...).Scan(&total)
	}); err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to count transactions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TransactionsFailed})
		return
	}

	// Query transactions (circuit breaker protected)
	offset := (page - 1) * perPage
	selectQuery := fmt.Sprintf(`
		SELECT l.id, l.user_id, u.email, COALESCE(u.username, ''), l.type, l.amount_cents,
		       COALESCE(l.ref_type::text, ''), COALESCE(l.ref_id::text, ''), COALESCE(l.description, ''), l.created_at
		%s
		ORDER BY l.created_at DESC
		LIMIT $%d OFFSET $%d
	`, baseQuery, argIdx, argIdx+1)
	args = append(args, perPage, offset)

	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, selectQuery, args...)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query transactions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TransactionsFailed})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var transactions []TransactionItem
	for rows.Next() {
		var t TransactionItem
		if err := rows.Scan(
			&t.ID, &t.User.ID, &t.User.Email, &t.User.Username,
			&t.Type, &t.AmountCents, &t.RefType, &t.RefID,
			&t.Description, &t.CreatedAt,
		); err != nil {
			a.log().Error("Failed to scan transaction", zap.Error(err))
			continue
		}
		transactions = append(transactions, t)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate transactions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.TransactionsFailed})
		return
	}

	response := TransactionListResponse{
		Transactions: transactions,
		Total:        total,
		Page:         page,
		PerPage:      perPage,
	}

	writeJSON(w, http.StatusOK, response)
}

// handleListSymbols returns a paginated list of symbols with optional filters.
func (a *App) handleListSymbols(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	limit := 50
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Filter parameters
	assetType := r.URL.Query().Get("asset_type")
	isActive := r.URL.Query().Get("is_active")
	search := r.URL.Query().Get("search")

	// Build query
	baseQuery := " FROM symbols WHERE 1=1"
	args := []interface{}{}
	argIdx := 1

	if assetType != "" {
		baseQuery += fmt.Sprintf(" AND asset_type::text = $%d", argIdx)
		args = append(args, assetType)
		argIdx++
	}
	if isActive != "" {
		active := isActive == "true"
		baseQuery += fmt.Sprintf(" AND is_active = $%d", argIdx)
		args = append(args, active)
		argIdx++
	}
	if search != "" {
		baseQuery += fmt.Sprintf(" AND (symbol ILIKE $%d OR name ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	// Count total (circuit breaker protected)
	countQuery := "SELECT COUNT(*)" + baseQuery
	var total int
	if err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, countQuery, args...).Scan(&total)
	}); err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to count symbols", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Query symbols (circuit breaker protected)
	selectQuery := fmt.Sprintf(`
		SELECT symbol, name, asset_type::text, provider_symbol_twelvedata, provider_symbol_massive, provider_symbol_finnhub, is_active, created_at, updated_at
		%s
		ORDER BY sort_order ASC, symbol ASC
		LIMIT $%d OFFSET $%d
	`, baseQuery, argIdx, argIdx+1)
	args = append(args, limit, offset)

	var symbols []SymbolResponse
	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			rows, err := a.pool.Replica().QueryContext(ctx, selectQuery, args...)
			if err != nil {
				return nil, err
			}
			defer rows.Close()

			var results []SymbolResponse
			for rows.Next() {
				var s SymbolResponse
				if err := rows.Scan(&s.Symbol, &s.Name, &s.AssetType, &s.ProviderSymbolTwelveData, &s.ProviderSymbolMassive, &s.ProviderSymbolFinnhub, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
					a.log().Error("Failed to scan symbol", zap.Error(err))
					continue
				}
				results = append(results, s)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return results, nil
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query symbols", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	symbols = result.([]SymbolResponse)

	response := SymbolListResponse{
		Symbols: symbols,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetSymbol returns a single symbol by its identifier.
func (a *App) handleGetSymbol(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if symbol == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SymbolRequired})
		return
	}

	ctx := r.Context()

	var s SymbolResponse
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx,
			`SELECT symbol, name, asset_type::text, provider_symbol_twelvedata, provider_symbol_massive, provider_symbol_finnhub, is_active, created_at, updated_at
			 FROM symbols WHERE symbol = $1`,
			symbol,
		).Scan(&s.Symbol, &s.Name, &s.AssetType, &s.ProviderSymbolTwelveData, &s.ProviderSymbolMassive, &s.ProviderSymbolFinnhub, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	})

	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.SymbolNotFound})
		return
	}
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query symbol", zap.String("symbol", symbol), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, s)
}

// handleCreateSymbol creates a new symbol in the master symbols table.
func (a *App) handleCreateSymbol(w http.ResponseWriter, r *http.Request) {
	var req CreateSymbolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Validate required fields
	v := validation.New()
	v.Required("symbol", req.Symbol)
	v.Required("name", req.Name)
	v.Required("asset_type", req.AssetType)
	v.MaxLength("symbol", req.Symbol, 20)
	v.MaxLength("name", req.Name, 100)
	v.In("asset_type", req.AssetType, []string{"stock", "crypto", "forex", "commodity"})

	if !v.Valid() {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": v.Errors()})
		return
	}

	// Sanitize inputs
	req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	req.Name = validation.SanitizeString(req.Name)

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	// Default is_active to true if not specified
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
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

	// Insert symbol
	var s SymbolResponse
	err = tx.QueryRowContext(ctx,
		`INSERT INTO symbols (symbol, name, asset_type, provider_symbol_twelvedata, provider_symbol_massive, provider_symbol_finnhub, is_active)
		 VALUES ($1, $2, $3::asset_type, $4, $5, $6, $7)
		 RETURNING symbol, name, asset_type::text, provider_symbol_twelvedata, provider_symbol_massive, provider_symbol_finnhub, is_active, created_at, updated_at`,
		req.Symbol, req.Name, req.AssetType, req.ProviderSymbolTwelveData, req.ProviderSymbolMassive, req.ProviderSymbolFinnhub, isActive,
	).Scan(&s.Symbol, &s.Name, &s.AssetType, &s.ProviderSymbolTwelveData, &s.ProviderSymbolMassive, &s.ProviderSymbolFinnhub, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.SymbolExists})
			return
		}
		a.log().Error("Failed to create symbol", zap.String("symbol", req.Symbol), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(req)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "symbol.created", "symbol", req.Symbol, payloadJSON,
	)
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

	writeJSON(w, http.StatusCreated, s)
}

// handleUpdateSymbol updates an existing symbol in the master symbols table.
func (a *App) handleUpdateSymbol(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if symbol == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SymbolRequired})
		return
	}

	var req UpdateSymbolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Validate optional fields if provided
	v := validation.New()
	if req.Name != nil {
		v.MaxLength("name", *req.Name, 100)
	}
	if req.AssetType != nil {
		v.In("asset_type", *req.AssetType, []string{"stock", "crypto", "forex", "commodity"})
	}

	if !v.Valid() {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": v.Errors()})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

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

	// Check if symbol exists
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM symbols WHERE symbol = $1)`, symbol).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check symbol existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.SymbolNotFound})
		return
	}

	// Build dynamic update query
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, validation.SanitizeString(*req.Name))
		argIdx++
	}
	if req.AssetType != nil {
		setClauses = append(setClauses, fmt.Sprintf("asset_type = $%d::asset_type", argIdx))
		args = append(args, *req.AssetType)
		argIdx++
	}
	if req.ProviderSymbolTwelveData != nil {
		setClauses = append(setClauses, fmt.Sprintf("provider_symbol_twelvedata = $%d", argIdx))
		args = append(args, nullIfEmpty(*req.ProviderSymbolTwelveData))
		argIdx++
	}
	if req.ProviderSymbolMassive != nil {
		setClauses = append(setClauses, fmt.Sprintf("provider_symbol_massive = $%d", argIdx))
		args = append(args, nullIfEmpty(*req.ProviderSymbolMassive))
		argIdx++
	}
	if req.ProviderSymbolFinnhub != nil {
		setClauses = append(setClauses, fmt.Sprintf("provider_symbol_finnhub = $%d", argIdx))
		args = append(args, nullIfEmpty(*req.ProviderSymbolFinnhub))
		argIdx++
	}
	if req.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *req.IsActive)
		argIdx++
	}

	if len(setClauses) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.NoFieldsToUpdate})
		return
	}

	// Add symbol to args for WHERE clause
	args = append(args, symbol)

	// Update symbol
	query := fmt.Sprintf(`
		UPDATE symbols SET %s
		WHERE symbol = $%d
		RETURNING symbol, name, asset_type::text, provider_symbol_twelvedata, provider_symbol_massive, provider_symbol_finnhub, is_active, created_at, updated_at`,
		strings.Join(setClauses, ", "), argIdx)

	var s SymbolResponse
	err = tx.QueryRowContext(ctx, query, args...).Scan(
		&s.Symbol, &s.Name, &s.AssetType, &s.ProviderSymbolTwelveData, &s.ProviderSymbolMassive, &s.ProviderSymbolFinnhub, &s.IsActive, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		a.log().Error("Failed to update symbol", zap.String("symbol", symbol), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(req)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "symbol.updated", "symbol", symbol, payloadJSON,
	)
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

	writeJSON(w, http.StatusOK, s)
}

// ============================================================================
// EMAIL TEMPLATE HANDLERS
// ============================================================================

// EmailTemplateListResponse represents the list of email templates.
type EmailTemplateListResponse struct {
	Templates []EmailTemplateListItem `json:"templates"`
}

// EmailTemplateListItem represents a template in the list.
type EmailTemplateListItem struct {
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	Variables   string  `json:"variables"`
	HasCustom   bool    `json:"has_custom"`
	UpdatedAt   string  `json:"updated_at"`
	UpdatedBy   *string `json:"updated_by,omitempty"`
}

// EmailTemplateDetailResponse represents detailed template information.
type EmailTemplateDetailResponse struct {
	Slug        string  `json:"slug"`
	Subject     string  `json:"subject,omitempty"`
	Description string  `json:"description"`
	Variables   string  `json:"variables"`
	HTMLContent string  `json:"html_content"`
	IsDefault   bool    `json:"is_default"`
	UpdatedAt   string  `json:"updated_at"`
	UpdatedBy   *string `json:"updated_by,omitempty"`
}

// UpdateEmailTemplateRequest represents a request to update a template.
type UpdateEmailTemplateRequest struct {
	HTMLContent string `json:"html_content"`
}

// PreviewEmailTemplateRequest represents a request to preview a template.
type PreviewEmailTemplateRequest struct {
	HTMLContent string `json:"html_content"`
}

// PreviewEmailTemplateResponse represents the preview response.
type PreviewEmailTemplateResponse struct {
	RenderedHTML string `json:"rendered_html"`
}

// handleListEmailTemplates returns a list of all email templates.
func (a *App) handleListEmailTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT
					slug,
					COALESCE(description, ''),
					COALESCE(variables, ''),
					CASE WHEN html_content IS NOT NULL AND html_content != '' THEN true ELSE false END as has_custom,
					updated_at,
					updated_by::text
				FROM email_templates
				ORDER BY slug
			`)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query email templates", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	var templates []EmailTemplateListItem
	for rows.Next() {
		var t EmailTemplateListItem
		var updatedAt time.Time
		if err := rows.Scan(&t.Slug, &t.Description, &t.Variables, &t.HasCustom, &updatedAt, &t.UpdatedBy); err != nil {
			a.log().Error("Failed to scan template", zap.Error(err))
			continue
		}
		t.UpdatedAt = updatedAt.Format(time.RFC3339)
		templates = append(templates, t)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate templates", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, EmailTemplateListResponse{Templates: templates})
}

// handleGetEmailTemplate returns detailed information about a specific template.
func (a *App) handleGetEmailTemplate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	ctx := r.Context()

	var t EmailTemplateDetailResponse
	var subject, htmlContent sql.NullString
	var updatedAt time.Time

	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT
				slug,
				subject,
				COALESCE(description, ''),
				COALESCE(variables, ''),
				html_content,
				updated_at,
				updated_by::text
			FROM email_templates
			WHERE slug = $1
		`, slug).Scan(&t.Slug, &subject, &t.Description, &t.Variables, &htmlContent, &updatedAt, &t.UpdatedBy)
	})

	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query template", zap.String("slug", slug), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	t.UpdatedAt = updatedAt.Format(time.RFC3339)

	if subject.Valid {
		t.Subject = subject.String
	}

	// If there's a custom template, use it; otherwise, load the default
	if htmlContent.Valid && htmlContent.String != "" {
		t.HTMLContent = htmlContent.String
		t.IsDefault = false
	} else {
		// Load default template from embedded files
		if a.emailNotifier != nil {
			defaultContent, err := a.emailNotifier.GetDefaultTemplate(slug)
			if err != nil {
				a.log().Warn("Failed to load default template", zap.String("slug", slug), zap.Error(err))
				t.HTMLContent = ""
			} else {
				t.HTMLContent = defaultContent
			}
		}
		t.IsDefault = true
	}

	writeJSON(w, http.StatusOK, t)
}

// handleUpdateEmailTemplate updates the HTML content of a template.
func (a *App) handleUpdateEmailTemplate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	var req UpdateEmailTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Validate
	v := validation.New()
	v.Required("html_content", req.HTMLContent)
	if !v.Valid() {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": v.Errors()})
		return
	}

	// Basic HTML validation
	if !strings.Contains(req.HTMLContent, "<html") && !strings.Contains(req.HTMLContent, "<!DOCTYPE") && !strings.Contains(req.HTMLContent, "<body") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.HTMLContentInvalid})
		return
	}

	// Defense-in-depth: strip dangerous HTML patterns
	req.HTMLContent = validation.SanitizeRichHTML(req.HTMLContent)

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

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

	// Check if template exists
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM email_templates WHERE slug = $1)`, slug).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check template existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	// Update template
	_, err = tx.ExecContext(ctx, `
		UPDATE email_templates
		SET html_content = $1, updated_by = $2, updated_at = NOW()
		WHERE slug = $3
	`, req.HTMLContent, actorUserID, slug)

	if err != nil {
		a.log().Error("Failed to update template", zap.String("slug", slug), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(map[string]string{"slug": slug, "action": "updated"})
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "email_template.updated", "email_template", slug, payloadJSON,
	)
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

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.TemplateUpdated})
}

// handleResetEmailTemplate resets a template to its default embedded version.
func (a *App) handleResetEmailTemplate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

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

	// Check if template exists
	var exists bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM email_templates WHERE slug = $1)`, slug).Scan(&exists)
	if err != nil {
		a.log().Error("Failed to check template existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	// Reset template (clear html_content to use default)
	_, err = tx.ExecContext(ctx, `
		UPDATE email_templates
		SET html_content = NULL, updated_by = $1, updated_at = NOW()
		WHERE slug = $2
	`, actorUserID, slug)

	if err != nil {
		a.log().Error("Failed to reset template", zap.String("slug", slug), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	payloadJSON, _ := json.Marshal(map[string]string{"slug": slug, "action": "reset"})
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "email_template.reset", "email_template", slug, payloadJSON,
	)
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

	writeJSON(w, http.StatusOK, map[string]string{"message": adminMsg.TemplateResetDefault})
}

// handlePreviewEmailTemplate renders a template with sample data for preview.
func (a *App) handlePreviewEmailTemplate(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.SlugRequired})
		return
	}

	var req PreviewEmailTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body - will preview current template
		req.HTMLContent = ""
	}

	ctx := r.Context()

	// Check if template exists (circuit breaker protected)
	var exists bool
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM email_templates WHERE slug = $1)`, slug).Scan(&exists)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to check template existence", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.TemplateNotFound})
		return
	}

	// Check if email notifier is available
	if a.emailNotifier == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.EmailNotConfigured})
		return
	}

	// Render preview
	renderedHTML, err := a.emailNotifier.RenderTemplatePreview(ctx, slug, req.HTMLContent)
	if err != nil {
		a.log().Error("Failed to render template preview",
			zap.String("slug", slug),
			zap.String("request_id", r.Header.Get("X-Request-ID")),
			zap.Error(err))
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidRequest})
		return
	}

	writeJSON(w, http.StatusOK, PreviewEmailTemplateResponse{RenderedHTML: renderedHTML})
}

// AutoSchedulingConfig represents the auto-scheduling configuration.
type AutoSchedulingConfig struct {
	Enabled          bool     `json:"enabled"`
	IntervalMinutes  int      `json:"interval_minutes"`
	DurationMinutes  int      `json:"duration_minutes"`
	AssetClasses     []string `json:"asset_classes"`
	WeekdaysOnly     bool     `json:"weekdays_only"`
	ActiveHoursStart int      `json:"active_hours_start"`
	ActiveHoursEnd   int      `json:"active_hours_end"`
	LeadTimeMinutes  int      `json:"lead_time_minutes"`
}

// AutoSchedulingUpcomingContest represents an upcoming auto-generated contest.
type AutoSchedulingUpcomingContest struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	AssetClass       string    `json:"asset_class"`
	TemplateID       *string   `json:"template_id,omitempty"`
	IsAutoGenerated  bool      `json:"is_auto_generated"`
	ParticipantCount int       `json:"participant_count"`
}

// handleGetAutoSchedulingConfig returns the current auto-scheduling configuration.
// GET /api/admin/auto-scheduling/config
// This is read-only; configuration is managed via environment variables.
func (a *App) handleGetAutoSchedulingConfig(w http.ResponseWriter, r *http.Request) {
	// Read configuration from environment variables (same vars as free-contest-generator service)
	enabled := os.Getenv("FREE_CONTEST_ENABLED") == "true"

	intervalMinutes := 60
	if v := os.Getenv("FREE_CONTEST_INTERVAL_MINUTES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			intervalMinutes = parsed
		}
	}

	durationMinutes := 60
	if v := os.Getenv("FREE_CONTEST_DURATION_MINUTES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			durationMinutes = parsed
		}
	}

	assetClasses := []string{"forex", "crypto"}
	if v := os.Getenv("FREE_CONTEST_ASSET_CLASSES"); v != "" {
		parts := strings.Split(v, ",")
		parsed := make([]string, 0, len(parts))
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				parsed = append(parsed, trimmed)
			}
		}
		if len(parsed) > 0 {
			assetClasses = parsed
		}
	}

	weekdaysOnly := true
	if v := os.Getenv("FREE_CONTEST_WEEKDAYS_ONLY"); v == "false" {
		weekdaysOnly = false
	}

	activeHoursStart := 6
	if v := os.Getenv("FREE_CONTEST_START_HOUR_UTC"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 && parsed <= 23 {
			activeHoursStart = parsed
		}
	}

	activeHoursEnd := 22
	if v := os.Getenv("FREE_CONTEST_END_HOUR_UTC"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 && parsed <= 23 {
			activeHoursEnd = parsed
		}
	}

	leadTimeMinutes := 5
	if v := os.Getenv("FREE_CONTEST_LEAD_TIME_MINUTES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			leadTimeMinutes = parsed
		}
	}

	config := AutoSchedulingConfig{
		Enabled:          enabled,
		IntervalMinutes:  intervalMinutes,
		DurationMinutes:  durationMinutes,
		AssetClasses:     assetClasses,
		WeekdaysOnly:     weekdaysOnly,
		ActiveHoursStart: activeHoursStart,
		ActiveHoursEnd:   activeHoursEnd,
		LeadTimeMinutes:  leadTimeMinutes,
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"config": config,
	})
}

// handleGetAutoSchedulingUpcoming returns upcoming auto-generated contests.
// GET /api/admin/auto-scheduling/upcoming
func (a *App) handleGetAutoSchedulingUpcoming(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT c.id, c.name, c.status, c.starts_at, c.ends_at, c.asset_class,
				       c.template_id, c.auto_generated,
				       (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) AS participant_count
				FROM contests c
				WHERE c.auto_generated = TRUE
				  AND c.starts_at > NOW()
				ORDER BY c.starts_at ASC
				LIMIT 50
			`)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query upcoming auto-generated contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	contests := make([]AutoSchedulingUpcomingContest, 0)
	for rows.Next() {
		var c AutoSchedulingUpcomingContest
		if err := rows.Scan(&c.ID, &c.Name, &c.Status, &c.StartsAt, &c.EndsAt,
			&c.AssetClass, &c.TemplateID, &c.IsAutoGenerated, &c.ParticipantCount); err != nil {
			a.log().Error("Failed to scan upcoming contest", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		contests = append(contests, c)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate upcoming contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"contests": contests,
	})
}

// ===========================================
// Admin Login Types, Helpers, and Handler
// ===========================================

// adminLoginRequest is the request body for admin login.
type adminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// adminAuthResponse is the response for successful admin authentication.
type adminAuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Step 7: admin cookies carry the _admin suffix so they don't collide
// with user-panel cookies when both panels share an eTLD+1. The cookie
// Path is scoped to /api/admin/auth so the browser only sends the admin
// refresh token to admin-bff ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â the shared /api/user/auth/refresh
// endpoint no longer handles admin refresh post the JWT audience split
// (step 5) and the path scoping is a belt-and-braces defence.
const (
	adminRefreshTokenCookieName = auth.AdminRefreshCookieName
	adminSessionHintCookieName  = auth.AdminSessionHintCookieName
	adminSessionHintCookieValue = "1"
	adminSessionCookieTTLSecs   = 7 * 24 * 3600
	adminRefreshCookiePath      = auth.AdminRefreshCookiePath
)

// resolveAdminCookieSecurity picks SameSite + Secure for the admin
// refresh-token cookie pair from the request rather than a static env
// var. Mirrors the user-bff logic (see PR #936): the same workspace
// can be reached over HTTP (local 127.0.0.1) and HTTPS (Codespaces
// public URL) simultaneously, and a blanket "production ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ Secure"
// rule would silently drop cookies on the HTTP origin.
func resolveAdminCookieSecurity(r *http.Request) (http.SameSite, bool) {
	if config.IsProduction() {
		return http.SameSiteNoneMode, true
	}
	isHTTPS := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	if isHTTPS {
		return http.SameSiteNoneMode, true
	}
	return http.SameSiteLaxMode, false
}

// setAdminRefreshTokenCookie issues the httpOnly refresh_token_admin
// cookie plus the non-HttpOnly session hint. Keeping this alongside the
// admin handler keeps admin-bff self-contained.
func setAdminRefreshTokenCookie(w http.ResponseWriter, r *http.Request, refreshToken string) {
	sameSite, secure := resolveAdminCookieSecurity(r)

	http.SetCookie(w, &http.Cookie{
		Name:     adminRefreshTokenCookieName,
		Value:    refreshToken,
		Path:     adminRefreshCookiePath,
		MaxAge:   adminSessionCookieTTLSecs,
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionHintCookieName,
		Value:    adminSessionHintCookieValue,
		Path:     "/",
		MaxAge:   adminSessionCookieTTLSecs,
		Secure:   secure,
		HttpOnly: false,
		SameSite: sameSite,
	})
}

// clearAdminRefreshTokenCookie removes both the refresh_token_admin
// cookie and the paired session hint. Call from explicit logout and
// (optionally) from refresh failure. Attributes must mirror
// setAdminRefreshTokenCookie ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â a mismatch leaves the original cookie
// intact and the browser keeps sending it.
func clearAdminRefreshTokenCookie(w http.ResponseWriter, r *http.Request) {
	sameSite, secure := resolveAdminCookieSecurity(r)

	http.SetCookie(w, &http.Cookie{
		Name:     adminRefreshTokenCookieName,
		Value:    "",
		Path:     adminRefreshCookiePath,
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: true,
		SameSite: sameSite,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionHintCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   secure,
		HttpOnly: false,
		SameSite: sameSite,
	})
}

// failedAdminLoginTracker tracks failed admin login attempts per IP with stricter thresholds.
type failedAdminLoginTracker struct {
	mu       sync.RWMutex
	attempts map[string]*adminLoginAttemptInfo
	done     chan struct{}
}

type adminLoginAttemptInfo struct {
	count       int
	lastFailed  time.Time
	lockedUntil time.Time
}

func newFailedAdminLoginTracker() *failedAdminLoginTracker {
	tracker := &failedAdminLoginTracker{
		attempts: make(map[string]*adminLoginAttemptInfo),
		done:     make(chan struct{}),
	}
	go tracker.cleanupLoop()
	return tracker
}

// recordFailure records a failed admin login attempt. Stricter than user login:
// 1 failure: no delay, 2 failures: 2s, 3-4 failures: 10s, 5+ failures: 60s lockout.
func (t *failedAdminLoginTracker) recordFailure(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, exists := t.attempts[key]
	if !exists {
		info = &adminLoginAttemptInfo{}
		t.attempts[key] = info
	}

	info.count++
	info.lastFailed = time.Now()

	var delay time.Duration
	switch {
	case info.count < 2:
		delay = 0
	case info.count < 3:
		delay = 2 * time.Second
	case info.count < 5:
		delay = 10 * time.Second
	default:
		delay = 60 * time.Second
	}

	if delay > 0 {
		info.lockedUntil = time.Now().Add(delay)
	}

	return delay
}

func (t *failedAdminLoginTracker) checkLocked(key string) (bool, time.Duration) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	info, exists := t.attempts[key]
	if !exists {
		return false, 0
	}

	if info.lockedUntil.After(time.Now()) {
		return true, time.Until(info.lockedUntil)
	}
	return false, 0
}

func (t *failedAdminLoginTracker) recordSuccess(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, key)
}

func (t *failedAdminLoginTracker) cleanupLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: failedAdminLoginTracker.cleanupLoop panicked: %s\n%s", observability.RedactPanic(r), observability.RedactText(string(debug.Stack())))
		}
	}()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.cleanup()
		case <-t.done:
			return
		}
	}
}

// stop signals the cleanup goroutine to exit.
func (t *failedAdminLoginTracker) stop() { close(t.done) }

func (t *failedAdminLoginTracker) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	threshold := time.Now().Add(-1 * time.Hour)
	for key, info := range t.attempts {
		if info.lastFailed.Before(threshold) {
			delete(t.attempts, key)
		}
	}
}

// banExpirySweeper periodically checks for expired temporary bans and restores users to active status.
type banExpirySweeper struct {
	db   *sql.DB
	log  *zap.Logger
	done chan struct{}
}

func newBanExpirySweeper(database *sql.DB, logger *zap.Logger) *banExpirySweeper {
	s := &banExpirySweeper{
		db:   database,
		log:  logger,
		done: make(chan struct{}),
	}
	go s.loop()
	return s
}

func (s *banExpirySweeper) loop() {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("banExpirySweeper loop panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.sweep()
		case <-s.done:
			return
		}
	}
}

func (s *banExpirySweeper) sweep() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.db.ExecContext(ctx,
		`UPDATE users SET status = 'active', ban_expires_at = NULL
		 WHERE status = 'suspended' AND ban_expires_at IS NOT NULL AND ban_expires_at <= NOW()`)
	if err != nil {
		s.log.Error("Ban expiry sweep failed", zap.Error(err))
		return
	}

	if affected, _ := result.RowsAffected(); affected > 0 {
		s.log.Info("Expired bans cleared", zap.Int64("count", affected))
	}
}

// stop signals the ban expiry sweeper goroutine to exit.
func (s *banExpirySweeper) stop() { close(s.done) }

// getAdminClientIP extracts the client IP using trusted proxy validation.
func getAdminClientIP(r *http.Request) string {
	return validation.ExtractClientIP(r)
}

// getAdminUserRoles fetches roles for a user from the database.
func (a *App) getAdminUserRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := a.pool.Primary().QueryContext(ctx,
		`SELECT r.name FROM roles r
		 INNER JOIN user_roles ur ON ur.role_id = r.id
		 WHERE ur.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return roles, nil
}

// isIPAllowed checks if the client IP is in the admin whitelist.
// Returns true if whitelist is empty (not configured) or IP matches.
func (a *App) isIPAllowed(clientIP string) bool {
	if len(a.config.AdminIPWhitelist) == 0 {
		return true
	}
	for _, allowed := range a.config.AdminIPWhitelist {
		if clientIP == allowed {
			return true
		}
	}
	return false
}

// handleAdminLogin handles admin-specific login with stricter security controls.
// POST /api/admin/auth/login
func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	clientIP := getAdminClientIP(r)
	userAgent := r.Header.Get("User-Agent")
	if a.distributedLoginLockout == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
		return
	}
	if allowed, retryAfter, err := a.distributedLoginLockout.Check(r.Context(), "ip:"+clientIP); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
		return
	} else if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": adminMsg.TooManyLoginAttempts})
		return
	}

	// Check IP whitelist
	if !a.isIPAllowed(clientIP) {
		a.log().Warn("Admin login blocked - IP not in whitelist",
			zap.String("ip", clientIP))
		a.logAuditEvent(r.Context(), "", "admin.login.blocked", "ip_whitelist", clientIP,
			map[string]string{"ip": clientIP, "reason": "ip_not_whitelisted"})
		writeJSON(w, http.StatusForbidden, map[string]string{"error": adminMsg.AccessDenied})
		return
	}

	// Check if client IP is currently locked out
	if locked, retryAfter := a.failedAdminLoginTracker.checkLocked(clientIP); locked {
		a.log().Warn("Admin login blocked due to too many failed attempts",
			zap.String("ip", clientIP),
			zap.Duration("retry_after", retryAfter))
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
			"error":       adminMsg.TooManyLoginAttempts,
			"message":     adminMsg.AccountLocked,
			"retry_after": int(retryAfter.Seconds()) + 1,
		})
		return
	}

	var req adminLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.InvalidBody})
		return
	}

	// Validate and sanitize input
	v := validation.New()
	req.Email = v.Email("email", req.Email)
	v.Required("password", req.Password)

	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	req.Email = validation.SanitizeEmail(req.Email)

	ctx := r.Context()
	lockoutIdentities := []string{"ip:" + clientIP, "account:" + req.Email}
	if allowed, retryAfter, err := a.distributedLoginLockout.Check(ctx, lockoutIdentities...); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
		return
	} else if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": adminMsg.TooManyLoginAttempts})
		return
	}

	// Look up user
	var userID, passwordHash string
	var isSystemAccount bool
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT id, password_hash, COALESCE(is_system_account, FALSE) FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &passwordHash, &isSystemAccount)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Dummy bcrypt compare so user-not-found takes the same time as
			// a real password verify. Without this, an attacker can probe
			// which emails exist by response latency alone.
			_ = auth.VerifyPassword(req.Password, auth.DummyHash)

			if _, lockErr := a.distributedLoginLockout.Failure(ctx, lockoutIdentities...); lockErr != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
				return
			}
			delay := a.failedAdminLoginTracker.recordFailure(clientIP)
			a.log().Warn("Failed admin login attempt - user not found",
				zap.String("email", req.Email),
				zap.String("ip", clientIP))
			a.logAuditEvent(ctx, "", "admin.login.failed", "auth", "",
				map[string]string{"email": req.Email, "ip": clientIP, "user_agent": userAgent, "reason": "user_not_found"})

			if delay > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
				writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
					"error":       adminMsg.TooManyLoginAttempts,
					"message":     adminMsg.TooManyLoginAttempts,
					"retry_after": int(delay.Seconds()) + 1,
				})
				return
			}
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": adminMsg.InvalidCredentials})
			return
		}
		a.log().Error("Failed to query user for admin login", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Block system accounts
	if isSystemAccount {
		a.log().Warn("Admin login attempt for system account blocked",
			zap.String("user_id", userID),
			zap.String("ip", clientIP))
		a.logAuditEvent(ctx, userID, "admin.login.failed", "auth", userID,
			map[string]string{"ip": clientIP, "user_agent": userAgent, "reason": "system_account"})
		writeJSON(w, http.StatusForbidden, map[string]string{"error": adminMsg.SystemAccountBlocked})
		return
	}

	// Verify password
	if err := a.auth.VerifyPassword(req.Password, passwordHash); err != nil {
		if _, lockErr := a.distributedLoginLockout.Failure(ctx, lockoutIdentities...); lockErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
			return
		}
		delay := a.failedAdminLoginTracker.recordFailure(clientIP)
		a.log().Warn("Failed admin login attempt - invalid password",
			zap.String("user_id", userID),
			zap.String("email", req.Email),
			zap.String("ip", clientIP))
		a.logAuditEvent(ctx, userID, "admin.login.failed", "auth", userID,
			map[string]string{"email": req.Email, "ip": clientIP, "user_agent": userAgent, "reason": "invalid_password"})

		if delay > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
			writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
				"error":       adminMsg.TooManyLoginAttempts,
				"message":     adminMsg.TooManyLoginAttempts,
				"retry_after": int(delay.Seconds()) + 1,
			})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": adminMsg.InvalidCredentials})
		return
	}

	// Load the current authoritative roles and permissions. Only the canonical
	// support_admin and super_admin roles may enter the Admin trust domain;
	// deprecated or unknown elevated roles fail closed.
	securityState, err := a.loadAdminSecurityState(ctx, userID)
	if err != nil {
		a.log().Error("Failed to load Admin authorization state", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	roles := securityState.Roles
	canonical, hasAdminRole := canonicalAdminRoles(roles)
	if !canonical || !hasAdminRole || securityState.Status != "active" {
		a.log().Warn("Non-canonical Admin login rejected",
			zap.String("user_id", userID), zap.String("ip", clientIP), zap.Strings("roles", roles))
		a.logAuditEvent(ctx, userID, "admin.login.non_admin_attempt", "auth", userID,
			map[string]string{"email": req.Email, "ip": clientIP, "user_agent": userAgent})
		if _, lockErr := a.distributedLoginLockout.Failure(ctx, lockoutIdentities...); lockErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
			return
		}
		delay := a.failedAdminLoginTracker.recordFailure(clientIP)
		if delay > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(delay.Seconds())+1))
			writeJSON(w, http.StatusTooManyRequests, map[string]interface{}{
				"error": adminMsg.TooManyLoginAttempts, "message": adminMsg.TooManyLoginAttempts,
				"retry_after": int(delay.Seconds()) + 1,
			})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": adminMsg.InvalidCredentials})
		return
	}

	// Super Admin password verification establishes only the first factor. No
	// access/refresh token or server session exists until the Admin-only MFA
	// enrollment or verification challenge succeeds. The password step must not
	// clear MFA failure counters; only completed MFA may clear them.
	if securityState.hasRole(auth.RoleSuperAdmin) {
		if a.mfaChallenges == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
			return
		}
		var enrolled bool
		if err := a.pool.Primary().QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM admin_mfa_credentials WHERE user_id=$1)`, userID).Scan(&enrolled); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
			return
		}
		stage := "verify"
		if !enrolled {
			stage = "enroll"
		}
		challenge, expiresAt, err := a.issueAdminMFAChallenge(ctx, r, userID, req.Email, securityState, stage, "")
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
			return
		}
		a.logAuditEvent(ctx, userID, "admin.mfa.challenge.issued", "auth", userID, map[string]string{"stage": stage})
		writeJSON(w, http.StatusAccepted, adminMFALoginResponse{MFARequired: true, EnrollmentRequired: !enrolled, Challenge: challenge, ExpiresAt: expiresAt})
		return
	}

	// Support Admin uses the isolated Admin password session but never acquires
	// Super Admin privileges or an MFA assurance claim.
	if err := a.distributedLoginLockout.Success(ctx, lockoutIdentities...); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.InternalError})
		return
	}
	a.failedAdminLoginTracker.recordSuccess(clientIP)

	deviceInfo := userAgent
	tokenPair, _, err := a.auth.LoginWithPermissions(ctx, userID, roles, effectiveAdminPermissions(securityState), deviceInfo, clientIP)
	if err != nil {
		a.log().Error("Failed to generate admin login tokens", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Audit log successful admin login
	a.logAuditEvent(ctx, userID, "admin.login.success", "auth", userID,
		map[string]string{"email": req.Email, "ip": clientIP, "user_agent": userAgent})

	a.log().Info("Admin user logged in successfully",
		zap.String("user_id", userID),
		zap.String("email", req.Email),
		zap.String("ip", clientIP))

	setAdminRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, adminAuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: "", // Sent via httpOnly cookie
		ExpiresAt:    tokenPair.ExpiresAt,
	})
}

// handleAdmin2FALoginVerify is a retired legacy symbol retained only to make its
// fail-closed removal explicit. No route registers it; SEC-007 uses the dedicated
// Admin MFA enrollment and verification handlers instead.
func (a *App) handleAdmin2FALoginVerify(w http.ResponseWriter, r *http.Request) {
	if r != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	clientIP := getAdminClientIP(r)

	// IP whitelist check
	if !a.isIPAllowed(clientIP) {
		a.log().Warn("Admin 2FA login blocked - IP not in whitelist",
			zap.String("ip", clientIP))
		writeJSON(w, http.StatusForbidden, map[string]string{"error": adminMsg.AccessDenied})
		return
	}

	ctx := r.Context()

	var req struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Ticket == "" || req.Code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TwoFATicketRequired})
		return
	}

	// Validate 6-digit code
	if len(req.Code) != 6 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TwoFACodeDigits})
		return
	}
	for _, c := range req.Code {
		if c < '0' || c > '9' {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": adminMsg.TwoFACodeDigits})
			return
		}
	}

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.TwoFAUnavailable})
		return
	}

	// Retrieve and delete ticket (single-use)
	ticketKey := fmt.Sprintf("auth:admin:2fa:ticket:%s", req.Ticket)
	ticketDataStr, err := a.redis.GetDel(ctx, ticketKey).Result()
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": adminMsg.InvalidOrExpiredTicket})
		return
	}

	var ticketData struct {
		UserID     string   `json:"user_id"`
		Roles      []string `json:"roles"`
		IP         string   `json:"ip"`
		DeviceInfo string   `json:"device_info"`
	}
	if err := json.Unmarshal([]byte(ticketDataStr), &ticketData); err != nil {
		a.log().Error("Failed to unmarshal admin 2FA ticket", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Brute force protection per user
	attemptKey := fmt.Sprintf("auth:admin:2fa:attempts:%s", ticketData.UserID)
	attempts, _ := a.redis.Get(ctx, attemptKey).Int()
	if attempts >= 5 {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": adminMsg.TwoFATooMany,
		})
		return
	}

	// Get TOTP secret and verify
	var totpSecret sql.NullString
	err = a.pool.Primary().QueryRowContext(ctx,
		`SELECT totp_secret FROM users WHERE id = $1`, ticketData.UserID,
	).Scan(&totpSecret)
	if err != nil || !totpSecret.Valid || totpSecret.String == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": adminMsg.TwoFANotConfigured})
		return
	}

	// The retired path cannot decrypt either legacy plaintext or shared-domain
	// ciphertext. This assignment keeps the unreachable historical body buildable
	// without preserving an authentication bypass.
	plaintextSecret := ""

	if !auth.VerifyTOTP(plaintextSecret, req.Code, time.Now()) {
		a.redis.Incr(ctx, attemptKey)
		a.redis.Expire(ctx, attemptKey, 15*time.Minute)
		a.logAuditEvent(ctx, ticketData.UserID, "admin.login.2fa_failed", "auth", ticketData.UserID,
			map[string]string{"ip": clientIP})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": adminMsg.TwoFAInvalidCode})
		return
	}

	// Success ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬ÃƒÂ¢Ã¢â€šÂ¬Ã‚Â clear counters
	a.redis.Del(ctx, attemptKey)
	a.redis.Del(ctx, fmt.Sprintf("auth:admin:2fa:pending:%s", ticketData.UserID))
	a.failedAdminLoginTracker.recordSuccess(clientIP)

	// Generate tokens with session
	tokenPair, _, err := a.auth.Login(ctx, ticketData.UserID, ticketData.Roles, ticketData.DeviceInfo, ticketData.IP)
	if err != nil {
		a.log().Error("Failed to generate admin tokens after 2FA", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.logAuditEvent(ctx, ticketData.UserID, "admin.login.success", "auth", ticketData.UserID,
		map[string]string{"ip": clientIP, "2fa_verified": "true"})

	a.log().Info("Admin user logged in successfully with 2FA",
		zap.String("user_id", ticketData.UserID),
		zap.String("ip", clientIP))

	setAdminRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, adminAuthResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: "", // Sent via httpOnly cookie
		ExpiresAt:    tokenPair.ExpiresAt,
	})
}
