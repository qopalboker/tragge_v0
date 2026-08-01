package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"go.uber.org/zap"
)

func (a *App) handleGetWallet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	walletInfo, err := a.wallet.GetWallet(ctx, userID)
	if err != nil {
		if _, ok := err.(*wallet.WalletNotFoundError); ok {
			// Create wallet if it doesn't exist (should be auto-created by trigger, but just in case)
			walletInfo, err = a.wallet.CreateWallet(ctx, userID)
			if err != nil {
				a.log().Error("Failed to create wallet", zap.Error(err))
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
				return
			}
		} else {
			a.log().Error("Failed to get wallet", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
	}

	writeJSON(w, http.StatusOK, WalletResponse{
		UserID:       walletInfo.UserID,
		BalanceCents: walletInfo.BalanceCents,
		Currency:     walletInfo.Currency,
		Status:       string(walletInfo.Status),
	})
}

// handleGetWalletHistory returns the user's wallet transaction history.
func (a *App) handleGetWalletHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

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

	// Parse optional type filter with validation
	typeFilter := r.URL.Query().Get("type")
	validTypes := map[string]bool{
		"deposit": true, "withdrawal": true, "contest_entry": true,
		"contest_refund": true, "prize_credit": true, "adjustment": true,
		"affiliate_commission": true, "withdraw_fee": true,
		"withdrawal_refund": true, "withdraw_fee_refund": true,
	}
	if typeFilter != "" && !validTypes[typeFilter] {
		typeFilter = "" // invalid type → show all
	}

	// Fetch entries with optional type filter
	entries, err := a.wallet.GetLedgerEntriesByType(ctx, userID, typeFilter, limit, offset)
	if err != nil {
		a.log().Error("Failed to get wallet history", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Get total count (matching same type filter) and balance
	total, _ := a.wallet.CountLedgerEntriesByType(ctx, userID, typeFilter)
	var balanceCents int64
	if walletInfo, err := a.wallet.GetWallet(ctx, userID); err == nil {
		balanceCents = walletInfo.BalanceCents
	}

	// Convert to response format
	var historyEntries []WalletHistoryEntry
	var payoutIDs []string
	payoutIndexMap := make(map[string]int) // maps payout ID to index in historyEntries

	for _, e := range entries {
		entry := WalletHistoryEntry{
			ID:                e.ID,
			Type:              string(e.Type),
			AmountCents:       e.AmountCents,
			BalanceAfterCents: e.BalanceAfterCents,
			CreatedAt:         e.CreatedAt.Format(time.RFC3339),
		}
		if e.RefType != nil {
			refType := string(*e.RefType)
			entry.RefType = &refType
		}
		if e.RefID != nil {
			entry.RefID = e.RefID
		}
		if e.Description != nil {
			entry.Description = e.Description
		}
		if e.ReasonCode != nil {
			rc := string(*e.ReasonCode)
			entry.ReasonCode = &rc
		}

		// Track withdrawal entries with payout references for status lookup
		if string(e.Type) == "withdrawal" && e.RefType != nil && *e.RefType == wallet.LedgerRefTypePayout && e.RefID != nil {
			payoutIDs = append(payoutIDs, *e.RefID)
			payoutIndexMap[*e.RefID] = len(historyEntries)
		}

		historyEntries = append(historyEntries, entry)
	}

	// Fetch payout status and admin comments for withdrawal entries
	if len(payoutIDs) > 0 {
		payoutDetails, err := a.getPayoutDetails(ctx, payoutIDs)
		if err != nil {
			a.log().Warn("Failed to fetch payout details", zap.Error(err))
			// Continue without payout details rather than failing the whole request
		} else {
			for payoutID, details := range payoutDetails {
				if idx, ok := payoutIndexMap[payoutID]; ok {
					historyEntries[idx].Status = &details.Status
					if details.AdminComment != nil && *details.AdminComment != "" {
						historyEntries[idx].AdminComment = details.AdminComment
					}
				}
			}
		}
	}

	if historyEntries == nil {
		historyEntries = []WalletHistoryEntry{}
	}

	writeJSON(w, http.StatusOK, WalletHistoryResponse{
		Entries:      historyEntries,
		Total:        total,
		BalanceCents: balanceCents,
		Page:         page,
		HasMore:      offset+limit < total,
	})
}

// payoutDetail holds status and admin comment for a payout.
type payoutDetail struct {
	Status       string
	AdminComment *string
}

// getPayoutDetails fetches status and admin_comment for the given payout IDs.
func (a *App) getPayoutDetails(ctx context.Context, payoutIDs []string) (map[string]payoutDetail, error) {
	if len(payoutIDs) == 0 {
		return nil, nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(payoutIDs))
	args := make([]interface{}, len(payoutIDs))
	for i, id := range payoutIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT id, status, admin_comment
		FROM payouts
		WHERE id IN (%s)
	`, strings.Join(placeholders, ", "))

	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query payouts: %w", err)
	}
	defer rows.Close()

	result := make(map[string]payoutDetail)
	for rows.Next() {
		var id, status string
		var adminComment sql.NullString
		if err := rows.Scan(&id, &status, &adminComment); err != nil {
			return nil, fmt.Errorf("scan payout: %w", err)
		}
		detail := payoutDetail{Status: status}
		if adminComment.Valid {
			detail.AdminComment = &adminComment.String
		}
		result[id] = detail
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate payouts: %w", err)
	}

	return result, nil
}
