package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/audit"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (a *App) getUserRoles(ctx context.Context, userID string) ([]string, error) {
	// Use Primary for auth-related queries to ensure latest role assignments
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

	// Default to "user" role if no roles found
	if len(roles) == 0 {
		roles = []string{"user"}
	}

	return roles, nil
}

// getUserPermissions returns the user's permissions based on their roles.
func (a *App) getUserPermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := a.pool.Primary().QueryContext(ctx,
		`SELECT DISTINCT p.name 
		 FROM permissions p
		 INNER JOIN role_permissions rp ON p.id = rp.permission_id
		 INNER JOIN user_roles ur ON rp.role_id = ur.role_id
		 WHERE ur.user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions []string
	for rows.Next() {
		var perm string
		if err := rows.Scan(&perm); err != nil {
			return nil, err
		}
		permissions = append(permissions, perm)
	}

	return permissions, rows.Err()
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeErrorJSON writes an error response with request ID.
func writeErrorJSON(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	requestID := validation.GetRequestID(r.Context())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":      errMsg,
		"request_id": requestID,
	})
}

// handleGetMyStats returns the current user's statistics.
func (a *App) handleGetMyStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	a.handleGetUserStatsInternal(w, r, userID)
}

// handleGetUserStats returns a specific user's statistics (public).
func (a *App) handleGetUserStats(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.UserIDRequired})
		return
	}
	a.handleGetUserStatsInternal(w, r, userID)
}

// handleGetUserStatsInternal is the internal implementation for getting user stats.
func (a *App) handleGetUserStatsInternal(w http.ResponseWriter, r *http.Request, userID string) {
	ctx := r.Context()

	// Query user stats from database (read-only, use replica)
	var stats UserStatsResponse
	var bestMarket sql.NullString
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT
			user_id, total_contests, total_wins, total_top3,
			total_score, tragge_point, win_rate,
			avg_trade_duration_seconds, best_market, best_market_pnl,
			total_trades, total_pnl
		FROM user_stats
		WHERE user_id = $1
	`, userID).Scan(
		&stats.UserID, &stats.TotalContests, &stats.TotalWins, &stats.TotalTop3,
		&stats.TotalScore, &stats.TraggePoint, &stats.WinRate,
		&stats.AvgTradeDurationSeconds, &bestMarket, &stats.BestMarketPnL,
		&stats.TotalTrades, &stats.TotalPnL,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// User has no stats yet - return empty stats
			writeJSON(w, http.StatusOK, UserStatsResponse{
				UserID:      userID,
				TotalScore:  0,
				TraggePoint: 0,
			})
			return
		}
		a.log().Error("Failed to query user stats", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if bestMarket.Valid {
		stats.BestMarket = &bestMarket.String
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleGlobalLeaderboard returns the global leaderboard based on T-Point.
func (a *App) handleGlobalLeaderboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse pagination params
	limit := 100
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

	// Query global leaderboard with usernames (read-only, use replica)
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT us.user_id, COALESCE(u.username, ''), us.tragge_point, us.total_contests, us.total_wins, us.total_top3, us.win_rate
		FROM user_stats us
		LEFT JOIN users u ON us.user_id = u.id
		WHERE us.total_contests > 0
		ORDER BY us.tragge_point DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		a.log().Error("Failed to query global leaderboard", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	entries := []GlobalLeaderboardEntry{}
	rank := offset + 1
	for rows.Next() {
		var entry GlobalLeaderboardEntry
		if err := rows.Scan(&entry.UserID, &entry.Username, &entry.TraggePoint, &entry.TotalContests, &entry.TotalWins, &entry.TotalTop3, &entry.WinRate); err != nil {
			a.log().Error("Failed to scan leaderboard entry", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		entry.Rank = rank
		entries = append(entries, entry)
		rank++
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate global leaderboard", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Build response
	response := GlobalLeaderboardResponse{
		Entries: entries,
	}

	// If user is authenticated, get their rank and score
	userID := auth.GetUserID(ctx)
	if userID != "" {
		var userRank int
		var userScore float64
		err := a.pool.Replica().QueryRowContext(ctx, `
			SELECT rank, tragge_point
			FROM (
				SELECT user_id, tragge_point,
					   ROW_NUMBER() OVER (ORDER BY tragge_point DESC) as rank
				FROM user_stats
				WHERE total_contests > 0
			) ranked
			WHERE user_id = $1
		`, userID).Scan(&userRank, &userScore)
		if err == nil {
			response.UserRank = &userRank
			response.UserScore = &userScore
		}
		// Ignore errors - user may not have any stats yet
	}

	writeJSON(w, http.StatusOK, response)
}

// handleGetMyScoreHistory returns the current user's score history.
func (a *App) handleGetMyScoreHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	a.handleGetScoreHistoryInternal(w, r, userID)
}

// handleGetScoreHistory returns a specific user's score history (public).
func (a *App) handleGetScoreHistory(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.UserIDRequired})
		return
	}
	a.handleGetScoreHistoryInternal(w, r, userID)
}

// handleGetScoreHistoryInternal is the internal implementation for getting score history.
func (a *App) handleGetScoreHistoryInternal(w http.ResponseWriter, r *http.Request, userID string) {
	ctx := r.Context()

	// Parse pagination params
	limit := 50
	offset := 0
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

	// Query score history (read-only, use replica)
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT
			h.contest_id, c.name, h.rank, h.score, h.participants,
			h.pnl, h.trades_count, h.avg_trade_duration_seconds,
			h.top_symbol, h.top_symbol_pnl, h.created_at
		FROM user_score_history h
		JOIN contests c ON c.id = h.contest_id
		WHERE h.user_id = $1
		ORDER BY h.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, limit, offset)
	if err != nil {
		a.log().Error("Failed to query score history", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	entries := []ScoreHistoryEntry{}
	for rows.Next() {
		var entry ScoreHistoryEntry
		var topSymbol sql.NullString
		var createdAt time.Time
		if err := rows.Scan(
			&entry.ContestID, &entry.ContestName, &entry.Rank, &entry.Score, &entry.Participants,
			&entry.PnL, &entry.TradesCount, &entry.AvgTradeDurationSeconds,
			&topSymbol, &entry.TopSymbolPnL, &createdAt,
		); err != nil {
			a.log().Error("Failed to scan score history entry", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		if topSymbol.Valid {
			entry.TopSymbol = &topSymbol.String
		}
		entry.CreatedAt = createdAt.Format(time.RFC3339)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate score history", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if entries == nil {
		entries = []ScoreHistoryEntry{}
	}

	writeJSON(w, http.StatusOK, ScoreHistoryResponse{
		Entries: entries,
	})
}

// SessionResponse represents a user session.
type SessionResponse struct {
	ID         string    `json:"id"`
	DeviceInfo string    `json:"device_info"`
	IPAddress  string    `json:"ip_address"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	IsCurrent  bool      `json:"is_current"`
}

// SessionsResponse is the response for session queries.
type SessionsResponse struct {
	Sessions []SessionResponse `json:"sessions"`
}

// handleGetSessions returns all active sessions for the current user.
func (a *App) handleGetSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	currentSessionID := auth.GetSessionID(ctx)

	// Check if session store is available
	if a.auth.Session == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.SessionNotAvailable,
		})
		return
	}

	// Get all user sessions
	sessions, err := a.auth.Session.GetUserSessions(ctx, userID)
	if err != nil {
		a.log().Error("Failed to get user sessions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Convert to response format
	response := make([]SessionResponse, 0, len(sessions))
	for _, session := range sessions {
		response = append(response, SessionResponse{
			ID:         session.ID,
			DeviceInfo: session.DeviceInfo,
			IPAddress:  session.IPAddress,
			CreatedAt:  session.CreatedAt,
			LastSeenAt: session.LastSeenAt,
			IsCurrent:  session.ID == currentSessionID,
		})
	}

	writeJSON(w, http.StatusOK, SessionsResponse{
		Sessions: response,
	})
}

// handleDeleteSession deletes a specific session (logout from that device).
func (a *App) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	currentSessionID := auth.GetSessionID(ctx)
	sessionIDToDelete := chi.URLParam(r, "session_id")

	if sessionIDToDelete == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.SessionIDRequired})
		return
	}

	// Check if session store is available
	if a.auth.Session == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.SessionNotAvailable,
		})
		return
	}

	// Prevent deleting current session (use /logout for that)
	if sessionIDToDelete == currentSessionID {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": msg.CannotDeleteCurrent,
		})
		return
	}

	// Verify the session belongs to the user
	session, err := a.auth.Session.Get(ctx, sessionIDToDelete)
	if err != nil {
		if errors.Is(err, auth.ErrSessionNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.SessionNotFound})
			return
		}
		a.log().Error("Failed to get session", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	if session.UserID != userID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": msg.SessionNotYours})
		return
	}

	// Delete the session
	if err := a.auth.Session.Delete(ctx, sessionIDToDelete); err != nil {
		a.log().Error("Failed to delete session", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	a.log().Info("Session deleted",
		zap.String("user_id", userID),
		zap.String("deleted_session_id", sessionIDToDelete))

	writeJSON(w, http.StatusOK, map[string]string{"message": msg.SessionDeleted})
}

// handleDeleteSessions deletes all sessions or all except current.
func (a *App) handleDeleteSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	currentSessionID := auth.GetSessionID(ctx)

	// Check if session store is available
	if a.auth.Session == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": msg.SessionNotAvailable,
		})
		return
	}

	// Check if ?all=true is set
	deleteAll := r.URL.Query().Get("all") == "true"

	if deleteAll {
		// Delete all sessions including current
		if err := a.auth.Session.DeleteAllForUser(ctx, userID); err != nil {
			a.log().Error("Failed to delete all sessions", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		a.log().Info("All sessions deleted", zap.String("user_id", userID))
		writeJSON(w, http.StatusOK, map[string]string{"message": msg.AllSessionsDeleted})
	} else {
		// Delete all sessions except current
		if currentSessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": msg.NoCurrentSession,
			})
			return
		}
		if err := a.auth.Session.DeleteAllExcept(ctx, userID, currentSessionID); err != nil {
			a.log().Error("Failed to delete other sessions", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
		a.log().Info("All other sessions deleted",
			zap.String("user_id", userID),
			zap.String("kept_session_id", currentSessionID))
		writeJSON(w, http.StatusOK, map[string]string{"message": msg.OtherSessionsDeleted})
	}
}

// handleLogout handles POST /logout — revokes the current session and blacklists the access token.
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	sessionID := auth.GetSessionID(ctx)
	claims := auth.GetClaims(ctx)

	// Blacklist the access token so it's immediately rejected
	if a.tokenBlacklist != nil && claims != nil && claims.ID != "" && claims.ExpiresAt != nil {
		if err := a.tokenBlacklist.Add(ctx, claims.ID, claims.ExpiresAt.Time); err != nil {
			a.log().Warn("Failed to blacklist access token on logout", zap.Error(err))
			// Continue with logout — token will expire naturally
		}
	}

	// Delete the session
	if a.auth.Session != nil && sessionID != "" {
		if err := a.auth.Session.Delete(ctx, sessionID); err != nil {
			a.log().Warn("Failed to delete session on logout", zap.Error(err))
		}
	}

	a.log().Info("User logged out",
		zap.String("user_id", userID),
		zap.String("session_id", sessionID))
	a.auditLogger.LogFromRequest(r, userID, audit.EventLogout, nil)

	a.clearRefreshTokenCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"message": msg.LoggedOut})
}

// ============================================================================
// TWO-FACTOR AUTHENTICATION (2FA) ENDPOINTS (P2-P3-4)
// ============================================================================

// handleValidateReferral validates a referral code and returns the masked referrer identity.
func (a *App) handleValidateReferral(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")

	if code == "" {
		writeJSON(w, http.StatusOK, ReferralValidateResponse{Valid: false})
		return
	}

	// Normalize code to uppercase
	code = strings.ToUpper(code)

	// Look up the referral code and get referrer info
	var referrerID string
	var isActive bool
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT user_id, is_active FROM referral_codes WHERE code = $1`,
		code,
	).Scan(&referrerID, &isActive)

	if err != nil || !isActive {
		writeJSON(w, http.StatusOK, ReferralValidateResponse{Valid: false})
		return
	}

	// Get referrer's masked email or phone (e.g., "j***@example.com")
	var email, refPhone sql.NullString
	err = a.pool.Replica().QueryRowContext(ctx,
		`SELECT email, phone FROM users WHERE id = $1`,
		referrerID,
	).Scan(&email, &refPhone)

	if err != nil {
		writeJSON(w, http.StatusOK, ReferralValidateResponse{Valid: false})
		return
	}

	// Mask the email, fall back to phone if email is empty
	var maskedIdentity string
	if email.Valid && email.String != "" {
		maskedIdentity = maskEmail(email.String)
	} else if refPhone.Valid && refPhone.String != "" {
		maskedIdentity = maskPhone(refPhone.String)
	} else {
		maskedIdentity = "***"
	}

	writeJSON(w, http.StatusOK, ReferralValidateResponse{
		Valid:        true,
		ReferrerName: &maskedIdentity,
	})
}

// maskEmail masks an email address (e.g., "john@example.com" -> "j***@example.com")
func maskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***"
	}
	localPart := parts[0]
	domain := parts[1]

	if len(localPart) <= 1 {
		return localPart + "***@" + domain
	}
	return localPart[:1] + "***@" + domain
}

// maskPhone masks a phone number (e.g., "+989121234567" -> "+98912***4567")
func maskPhone(phone string) string {
	if len(phone) <= 7 {
		return "***"
	}
	return phone[:len(phone)-7] + "***" + phone[len(phone)-4:]
}

// handleGetAffiliateStats returns the user's affiliate stats.
// GET /api/user/affiliate
func (a *App) handleGetAffiliateStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var stats AffiliateStatsResponse

	// Get the user's referral code and stats from the view
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT
			COALESCE(rc.code, '') as code,
			COALESCE(rc.commission_rate_bps, 500) as commission_rate_bps,
			COALESCE(rc.is_active, true) as is_active,
			COALESCE(stats.total_referrals, 0) as total_referrals,
			COALESCE(stats.qualified_referrals, 0) as qualified_referrals,
			COALESCE(stats.total_earned_cents, 0) as total_earned_cents,
			COALESCE(stats.pending_cents, 0) as pending_cents
		FROM referral_codes rc
		LEFT JOIN affiliate_stats stats ON rc.user_id = stats.referrer_id
		WHERE rc.user_id = $1
	`, userID).Scan(
		&stats.ReferralCode,
		&stats.CommissionRateBps,
		&stats.IsActive,
		&stats.TotalReferrals,
		&stats.QualifiedReferrals,
		&stats.TotalEarnedCents,
		&stats.PendingEarnedCents,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// User doesn't have a referral code yet - this shouldn't happen
			// because codes are auto-generated, but handle gracefully
			writeJSON(w, http.StatusOK, AffiliateStatsResponse{
				ReferralCode:       "",
				CommissionRateBps:  500,
				IsActive:           true,
				TotalReferrals:     0,
				QualifiedReferrals: 0,
				TotalEarnedCents:   0,
				PendingEarnedCents: 0,
			})
			return
		}
		a.log().Error("Failed to query affiliate stats", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// handleGetAffiliateReferrals returns the user's referrals with pagination.
// GET /api/user/affiliate/referrals?page=1&page_size=20
func (a *App) handleGetAffiliateReferrals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Parse pagination params
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Get total count
	var total int
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM referrals WHERE referrer_id = $1
	`, userID).Scan(&total)
	if err != nil {
		a.log().Error("Failed to count referrals", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Get referrals with pagination
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT
			u.email,
			r.status,
			r.created_at,
			r.qualified_at
		FROM referrals r
		JOIN users u ON r.referred_id = u.id
		WHERE r.referrer_id = $1
		ORDER BY r.created_at DESC
		LIMIT $2 OFFSET $3
	`, userID, pageSize, offset)
	if err != nil {
		a.log().Error("Failed to query referrals", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	var referrals []ReferralEntry
	for rows.Next() {
		var entry ReferralEntry
		var email string
		if err := rows.Scan(&email, &entry.Status, &entry.CreatedAt, &entry.QualifiedAt); err != nil {
			a.log().Error("Failed to scan referral row", zap.Error(err))
			continue
		}
		entry.Email = maskEmail(email)
		referrals = append(referrals, entry)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Row iteration error", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}

	writeJSON(w, http.StatusOK, AffiliateReferralsResponse{
		Referrals:  referrals,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// processAffiliateCommission handles commission calculation when a referred user joins a paid contest.
// This function is called within a transaction and logs errors but does not fail the contest join.
func (a *App) processAffiliateCommission(ctx context.Context, tx *db.Transaction, userID, contestID string, entryFeeCents int64) {
	// Check if user was referred
	var referrerID string
	var referralID string
	var referralStatus string
	var commissionRateBps int

	err := tx.QueryRowContext(ctx, `
		SELECT r.referrer_id, r.id, r.status, rc.commission_rate_bps
		FROM referrals r
		JOIN referral_codes rc ON r.code = rc.code
		WHERE r.referred_id = $1 AND rc.is_active = true
	`, userID).Scan(&referrerID, &referralID, &referralStatus, &commissionRateBps)

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			a.log().Warn("Failed to check referral for commission",
				zap.String("user_id", userID),
				zap.Error(err))
		}
		// Not referred or error - just return silently
		return
	}

	// Calculate commission: entry_fee * commission_rate_bps / 10000
	commissionCents := (entryFeeCents * int64(commissionRateBps)) / 10000

	if commissionCents <= 0 {
		return // No commission to pay
	}

	// Check if we already have a commission for this exact contest entry (idempotency)
	var existingCommission int64
	err = tx.QueryRowContext(ctx, `
		SELECT commission_cents FROM affiliate_commissions
		WHERE referred_id = $1 AND source_type = 'contest_entry' AND source_id = $2::uuid
	`, userID, contestID).Scan(&existingCommission)
	if err == nil {
		// Commission already exists - skip
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		a.log().Warn("Failed to check existing commission",
			zap.String("referred_id", userID),
			zap.String("contest_id", contestID),
			zap.Error(err))
		return
	}

	// Insert pending commission
	_, err = tx.ExecContext(ctx, `
		INSERT INTO affiliate_commissions (
			referrer_id, referred_id, source_type, source_id,
			gross_amount_cents, commission_rate_bps, commission_cents, status
		) VALUES ($1, $2, 'contest_entry', $3, $4, $5, $6, 'pending')
	`, referrerID, userID, contestID, entryFeeCents, commissionRateBps, commissionCents)

	if err != nil {
		a.log().Warn("Failed to insert affiliate commission",
			zap.String("referrer_id", referrerID),
			zap.String("referred_id", userID),
			zap.String("contest_id", contestID),
			zap.Int64("commission_cents", commissionCents),
			zap.Error(err))
		return
	}

	// If referral status is 'pending', update to 'qualified' (first paid contest entry)
	if referralStatus == "pending" {
		_, err = tx.ExecContext(ctx, `
			UPDATE referrals
			SET status = 'qualified', qualified_at = NOW()
			WHERE id = $1 AND status = 'pending'
		`, referralID)

		if err != nil {
			a.log().Warn("Failed to update referral to qualified",
				zap.String("referral_id", referralID),
				zap.Error(err))
			// Continue - commission was inserted, this is not critical
		} else {
			a.log().Info("Referral qualified on first paid contest entry",
				zap.String("referral_id", referralID),
				zap.String("referred_id", userID))
		}
	}

	a.log().Info("Affiliate commission created",
		zap.String("referrer_id", referrerID),
		zap.String("referred_id", userID),
		zap.String("contest_id", contestID),
		zap.Int64("commission_cents", commissionCents),
		zap.Int("commission_rate_bps", commissionRateBps))
}

// handleGetAffiliateStatus returns the user's affiliate activation status.
// GET /api/user/me/affiliate/status
func (a *App) handleGetAffiliateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	var response AffiliateStatusResponse
	var activationStatus string
	var requestedAt, approvedAt sql.NullTime
	var totalReferrals, qualifiedReferrals int
	var totalEarnedCents, pendingCents int64

	// Query referral code with activation status and stats
	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT
			COALESCE(rc.code, '') as code,
			COALESCE(rc.activation_status::text, 'inactive') as activation_status,
			rc.activation_requested_at,
			rc.activation_approved_at,
			COALESCE(stats.total_referrals, 0) as total_referrals,
			COALESCE(stats.qualified_referrals, 0) as qualified_referrals,
			COALESCE(stats.total_earned_cents, 0) as total_earned_cents,
			COALESCE(stats.pending_cents, 0) as pending_cents
		FROM referral_codes rc
		LEFT JOIN affiliate_stats stats ON rc.user_id = stats.referrer_id
		WHERE rc.user_id = $1
	`, userID).Scan(
		&response.Code,
		&activationStatus,
		&requestedAt,
		&approvedAt,
		&totalReferrals,
		&qualifiedReferrals,
		&totalEarnedCents,
		&pendingCents,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// User doesn't have a referral code yet
			writeJSON(w, http.StatusOK, AffiliateStatusResponse{
				Status: "inactive",
				Code:   "",
			})
			return
		}
		a.log().Error("Failed to query affiliate status", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	response.Status = activationStatus
	if requestedAt.Valid {
		response.RequestedAt = &requestedAt.Time
	}
	if approvedAt.Valid {
		response.ApprovedAt = &approvedAt.Time
	}

	// Only include stats if activation is active
	if activationStatus == "active" {
		response.Stats = &AffiliateStatusStats{
			TotalReferrals:     totalReferrals,
			QualifiedReferrals: qualifiedReferrals,
			TotalEarned:        totalEarnedCents,
			PendingEarnings:    pendingCents,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// handleRequestAffiliateActivation handles affiliate activation request.
// POST /api/user/me/affiliate/request-activation
func (a *App) handleRequestAffiliateActivation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Check current activation status
	var currentStatus string
	err := a.pool.Primary().QueryRowContext(ctx, `
		SELECT COALESCE(activation_status::text, 'inactive')
		FROM referral_codes
		WHERE user_id = $1
	`, userID).Scan(&currentStatus)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ReferralCodeNotFound})
			return
		}
		a.log().Error("Failed to check activation status", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Only allow request if status is 'inactive' or 'rejected'
	if currentStatus != "inactive" && currentStatus != "rejected" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ActivationAlreadyDone})
		return
	}

	// Update status to pending
	_, err = a.pool.Primary().ExecContext(ctx, `
		UPDATE referral_codes
		SET activation_status = 'pending',
		    activation_requested_at = NOW(),
		    activation_rejected_at = NULL,
		    rejection_reason = NULL
		WHERE user_id = $1
	`, userID)

	if err != nil {
		a.log().Error("Failed to request affiliate activation", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	a.log().Info("Affiliate activation requested", zap.String("user_id", userID))

	writeJSON(w, http.StatusOK, map[string]string{"message": msg.ActivationSubmitted})
}

// FreeContestEntry represents a free contest for the free contests endpoint.
type FreeContestEntry struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AssetClass   string `json:"asset_class"`
	StartsAt     string `json:"starts_at"`
	EndsAt       string `json:"ends_at"`
	Status       string `json:"status"`
	Participants int    `json:"participants"`
	// Computed fields
	StartsInMinutes *int `json:"starts_in_minutes,omitempty"`
	EndsInMinutes   *int `json:"ends_in_minutes,omitempty"`
}

// FreeContestsResponse is the response for the free contests endpoint.
type FreeContestsResponse struct {
	Upcoming []FreeContestEntry `json:"upcoming"`
	Running  []FreeContestEntry `json:"running"`
}

// handleListFreeContests returns free practice contests grouped by status.
// GET /api/user/contests/free
func (a *App) handleListFreeContests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	query := `
		SELECT c.id, c.name, c.asset_class, c.starts_at, c.ends_at, c.status,
		       (SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) as participant_count
		FROM contests c
		WHERE c.is_free = TRUE
		  AND c.status IN ('registration_open', 'scheduled', 'running')
		ORDER BY c.starts_at ASC`

	rows, err := a.pool.Replica().QueryContext(ctx, query)
	if err != nil {
		a.log().Error("Failed to query free contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	now := time.Now().UTC()
	response := FreeContestsResponse{
		Upcoming: []FreeContestEntry{},
		Running:  []FreeContestEntry{},
	}

	for rows.Next() {
		var c FreeContestEntry
		var startsAt, endsAt time.Time
		if err := rows.Scan(&c.ID, &c.Name, &c.AssetClass, &startsAt, &endsAt, &c.Status, &c.Participants); err != nil {
			a.log().Error("Failed to scan free contest", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		c.StartsAt = startsAt.Format(time.RFC3339)
		c.EndsAt = endsAt.Format(time.RFC3339)

		// Calculate time until start/end
		if c.Status == "running" {
			minutesUntilEnd := int(time.Until(endsAt).Minutes())
			if minutesUntilEnd < 0 {
				minutesUntilEnd = 0
			}
			c.EndsInMinutes = &minutesUntilEnd
			response.Running = append(response.Running, c)
		} else {
			// Upcoming (scheduled or registration_open)
			minutesUntilStart := int(time.Until(startsAt).Minutes())
			if minutesUntilStart < 0 {
				minutesUntilStart = 0
			}
			c.StartsInMinutes = &minutesUntilStart
			response.Upcoming = append(response.Upcoming, c)
		}
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate free contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	_ = now // silence unused warning
	writeJSON(w, http.StatusOK, response)
}

// handleCalendar returns contests in a calendar format with filtering and grouping support.
// GET /api/user/contests/calendar
// Query params:
//   - from: start date (YYYY-MM-DD), defaults to today
//   - to: end date (YYYY-MM-DD), defaults to from + 7 days
//   - asset_class: filter by asset class (forex, crypto, stocks, mixed)
//   - type: filter by contest type (rush, standard, tournament, championship, practice)
//   - duration_type: filter by duration (rush_30min, hourly, four_hour, daily, weekly)
//   - min_fee: minimum entry fee in dollars
//   - max_fee: maximum entry fee in dollars
//   - group_by: grouping option (day, type, asset)
//   - registered_only: if true, only show contests user is registered for
func (a *App) handleCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse date parameters
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	var fromDate, toDate time.Time
	var err error

	if fromStr != "" {
		fromDate, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidDateFormat})
			return
		}
	} else {
		fromDate = time.Now().UTC().Truncate(24 * time.Hour)
	}

	if toStr != "" {
		toDate, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.InvalidDateFormat})
			return
		}
		// Set to end of day
		toDate = toDate.Add(24*time.Hour - time.Second)
	} else {
		toDate = fromDate.Add(7 * 24 * time.Hour)
	}

	// Validate date range (max 30 days)
	if toDate.Sub(fromDate) > 30*24*time.Hour {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.DateRangeTooLarge})
		return
	}

	// Parse filter parameters
	assetClass := r.URL.Query().Get("asset_class")
	contestType := r.URL.Query().Get("type")
	durationType := r.URL.Query().Get("duration_type")
	minFeeStr := r.URL.Query().Get("min_fee")
	maxFeeStr := r.URL.Query().Get("max_fee")
	groupBy := r.URL.Query().Get("group_by")
	registeredOnly := r.URL.Query().Get("registered_only") == "true"

	// Get user ID if authenticated (for user_registered flag)
	userID := auth.GetUserID(ctx)

	// Build query
	userRegisteredClause := "FALSE as user_registered"
	var args []interface{}
	dateStartIdx := 1

	if userID != "" {
		userRegisteredClause = "EXISTS(SELECT 1 FROM contest_participants cp WHERE cp.contest_id = c.id AND cp.user_id = $1) as user_registered"
		args = append(args, userID)
		dateStartIdx = 2
	}

	args = append(args, fromDate, toDate)

	query := fmt.Sprintf(`
		SELECT
			c.id, c.name, COALESCE(c.type::text, 'standard'), c.asset_class,
			c.entry_fee_cents, COALESCE(c.duration_minutes, 0), c.starts_at, c.ends_at,
			c.status, c.max_participants, c.commission_rate, COALESCE(c.platform_fee_bps, 0), c.is_free,
			(SELECT COUNT(*) FROM contest_participants cp WHERE cp.contest_id = c.id) as participant_count,
			%s
		FROM contests c
		WHERE c.starts_at >= $%d AND c.starts_at <= $%d
		  AND c.status IN ('scheduled', 'registration_open', 'running')`,
		userRegisteredClause, dateStartIdx, dateStartIdx+1)

	argIdx := dateStartIdx + 2

	// Asset class filter
	if assetClass != "" {
		validClasses := map[string]bool{"forex": true, "crypto": true, "stocks": true, "mixed": true}
		if validClasses[assetClass] {
			query += fmt.Sprintf(" AND c.asset_class = $%d", argIdx)
			args = append(args, assetClass)
			argIdx++
		}
	}

	// Contest type filter
	if contestType != "" {
		validTypes := map[string]bool{"rush": true, "standard": true, "tournament": true, "championship": true, "practice": true}
		if validTypes[contestType] {
			query += fmt.Sprintf(" AND c.type = $%d", argIdx)
			args = append(args, contestType)
			argIdx++
		}
	}

	// Duration type filter
	if durationType != "" {
		validDurations := map[string]bool{"rush_30min": true, "hourly": true, "four_hour": true, "daily": true, "weekly": true}
		if validDurations[durationType] {
			query += fmt.Sprintf(" AND c.duration_type = $%d", argIdx)
			args = append(args, durationType)
			argIdx++
		}
	}

	// Min fee filter (convert dollars to cents)
	if minFeeStr != "" {
		if minFee, err := strconv.ParseFloat(minFeeStr, 64); err == nil && minFee >= 0 {
			minFeeCents := int(minFee * 100)
			query += fmt.Sprintf(" AND c.entry_fee_cents >= $%d", argIdx)
			args = append(args, minFeeCents)
			argIdx++
		}
	}

	// Max fee filter (convert dollars to cents)
	if maxFeeStr != "" {
		if maxFee, err := strconv.ParseFloat(maxFeeStr, 64); err == nil && maxFee >= 0 {
			maxFeeCents := int(maxFee * 100)
			query += fmt.Sprintf(" AND c.entry_fee_cents <= $%d", argIdx)
			args = append(args, maxFeeCents)
			argIdx++
		}
	}

	// Registered only filter
	if registeredOnly && userID != "" {
		query += " AND EXISTS(SELECT 1 FROM contest_participants cp WHERE cp.contest_id = c.id AND cp.user_id = $1)"
	}

	query += " ORDER BY c.starts_at ASC"

	rows, err := a.pool.Replica().QueryContext(ctx, query, args...)
	if err != nil {
		a.log().Error("Failed to query calendar contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}
	defer rows.Close()

	contests := []CalendarContest{}

	for rows.Next() {
		var c CalendarContest
		var entryFeeCents int
		var startsAt, endsAt time.Time
		var maxParticipants sql.NullInt32
		var commissionRate float64
		var platformFeeBps int
		var participantCount int
		var userRegistered bool
		var isFree bool

		if err := rows.Scan(
			&c.ID, &c.Name, &c.Type, &c.AssetClass,
			&entryFeeCents, &c.DurationMinutes, &startsAt, &endsAt,
			&c.Status, &maxParticipants, &commissionRate, &platformFeeBps, &isFree,
			&participantCount, &userRegistered,
		); err != nil {
			a.log().Error("Failed to scan calendar contest", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}

		// Convert cents to dollars
		c.EntryFee = float64(entryFeeCents) / 100.0
		c.StartsAt = startsAt.Format(time.RFC3339)
		c.EndsAt = endsAt.Format(time.RFC3339)
		c.Participants.Current = participantCount
		c.UserRegistered = userRegistered

		if maxParticipants.Valid {
			maxP := int(maxParticipants.Int32)
			c.Participants.Max = &maxP
		}

		// Calculate prize pool using integer math, then convert to dollars
		if isFree {
			c.PrizePool = 0
		} else {
			feeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)
			grossCents := int64(entryFeeCents) * int64(participantCount)
			netCents := (grossCents * int64(10000-feeBps)) / 10000
			c.PrizePool = float64(netCents) / 100.0
		}

		contests = append(contests, c)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Failed to iterate calendar contests", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Format response based on grouping
	fromFormatted := fromDate.Format("2006-01-02")
	toFormatted := toDate.Format("2006-01-02")

	if groupBy != "" {
		groups := a.groupCalendarContests(contests, groupBy)
		response := CalendarGroupedResponse{
			From:   fromFormatted,
			To:     toFormatted,
			Groups: groups,
			Total:  len(contests),
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	response := CalendarResponse{
		From:     fromFormatted,
		To:       toFormatted,
		Contests: contests,
		Total:    len(contests),
	}
	writeJSON(w, http.StatusOK, response)
}

// groupCalendarContests groups contests by the specified criteria.
func (a *App) groupCalendarContests(contests []CalendarContest, groupBy string) []CalendarGroup {
	groupMap := make(map[string]*CalendarGroup)
	var groupOrder []string

	for _, c := range contests {
		var key, label string

		switch groupBy {
		case "day":
			// Parse the starts_at to get the date
			t, err := time.Parse(time.RFC3339, c.StartsAt)
			if err != nil {
				continue
			}
			key = t.Format("2006-01-02")
			label = t.Format("Monday, January 2, 2006")

		case "type":
			key = c.Type
			// Human-readable labels for types
			typeLabels := map[string]string{
				"rush":         "Rush Contests",
				"standard":     "Standard Contests",
				"tournament":   "Tournaments",
				"championship": "Championships",
				"practice":     "Practice",
			}
			label = typeLabels[key]
			if label == "" {
				label = key
			}

		case "asset":
			key = c.AssetClass
			// Human-readable labels for asset classes
			assetLabels := map[string]string{
				"forex":  "Forex",
				"crypto": "Cryptocurrency",
				"stocks": "Stocks",
				"mixed":  "Mixed Assets",
			}
			label = assetLabels[key]
			if label == "" {
				label = key
			}

		default:
			// Invalid group_by, return empty
			return []CalendarGroup{}
		}

		if _, exists := groupMap[key]; !exists {
			groupMap[key] = &CalendarGroup{
				Key:      key,
				Label:    label,
				Contests: []CalendarContest{},
				Count:    0,
			}
			groupOrder = append(groupOrder, key)
		}

		groupMap[key].Contests = append(groupMap[key].Contests, c)
		groupMap[key].Count++
	}

	// Build result maintaining order
	result := make([]CalendarGroup, 0, len(groupOrder))
	for _, key := range groupOrder {
		result = append(result, *groupMap[key])
	}

	return result
}

// generateContestsFromTemplate generates contest instances from a template for a date range.
// This can be used by a background worker to pre-generate scheduled contests.
func GenerateContestsFromTemplate(template ContestTemplate, fromDate, toDate time.Time) []GeneratedContest {
	if template.RecurrenceRule == nil || *template.RecurrenceRule == "" {
		return nil
	}

	rule := *template.RecurrenceRule
	var contests []GeneratedContest

	// Parse recurrence rule
	// Formats: "HOURLY", "DAILY@HH:MM", "WEEKLY@DAY1,DAY2@HH:MM"
	parts := strings.Split(rule, "@")
	frequency := parts[0]

	var scheduleTime time.Time
	var scheduleDays []time.Weekday

	if len(parts) >= 2 {
		// Parse time component
		timePart := parts[len(parts)-1]
		if matched, _ := regexp.MatchString(`^\d{2}:\d{2}$`, timePart); matched {
			t, err := time.Parse("15:04", timePart)
			if err == nil {
				scheduleTime = t
			}
		}

		// Parse day component for weekly
		if frequency == "WEEKLY" && len(parts) >= 2 {
			dayMap := map[string]time.Weekday{
				"SUN": time.Sunday, "MON": time.Monday, "TUE": time.Tuesday,
				"WED": time.Wednesday, "THU": time.Thursday, "FRI": time.Friday, "SAT": time.Saturday,
			}
			dayPart := parts[1]
			for _, day := range strings.Split(dayPart, ",") {
				if wd, ok := dayMap[strings.ToUpper(day)]; ok {
					scheduleDays = append(scheduleDays, wd)
				}
			}
		}
	}

	current := fromDate
	for current.Before(toDate) {
		var shouldGenerate bool
		var contestStart time.Time

		switch frequency {
		case "HOURLY":
			shouldGenerate = true
			contestStart = current

		case "DAILY":
			if scheduleTime.IsZero() {
				contestStart = time.Date(current.Year(), current.Month(), current.Day(), 9, 0, 0, 0, current.Location())
			} else {
				contestStart = time.Date(current.Year(), current.Month(), current.Day(),
					scheduleTime.Hour(), scheduleTime.Minute(), 0, 0, current.Location())
			}
			if contestStart.After(current) || contestStart.Equal(current) {
				shouldGenerate = true
			}

		case "WEEKLY":
			for _, day := range scheduleDays {
				if current.Weekday() == day {
					if scheduleTime.IsZero() {
						contestStart = time.Date(current.Year(), current.Month(), current.Day(), 18, 0, 0, 0, current.Location())
					} else {
						contestStart = time.Date(current.Year(), current.Month(), current.Day(),
							scheduleTime.Hour(), scheduleTime.Minute(), 0, 0, current.Location())
					}
					if contestStart.After(fromDate) || contestStart.Equal(fromDate) {
						shouldGenerate = true
					}
					break
				}
			}
		}

		if shouldGenerate && contestStart.Before(toDate) {
			contestEnd := contestStart.Add(time.Duration(template.DurationMinutes) * time.Minute)
			contest := GeneratedContest{
				TemplateID:      template.ID,
				Name:            template.Name,
				Type:            template.Type,
				AssetClass:      template.AssetClass,
				EntryFeeCents:   template.EntryFeeCents,
				DurationMinutes: template.DurationMinutes,
				StartsAt:        contestStart,
				EndsAt:          contestEnd,
				MaxParticipants: template.MaxParticipants,
				Status:          "scheduled",
			}
			contests = append(contests, contest)
		}

		// Advance to next occurrence
		switch frequency {
		case "HOURLY":
			current = current.Add(time.Hour)
		case "DAILY":
			current = current.Add(24 * time.Hour)
		case "WEEKLY":
			current = current.Add(24 * time.Hour)
		default:
			current = current.Add(24 * time.Hour)
		}
	}

	return contests
}

// =============================================
// In-App Notification Handlers
// =============================================

// NotificationsResponse is the response for the notifications list endpoint.
type NotificationsResponse struct {
	Notifications []inapp.Notification `json:"notifications"`
	Total         int                  `json:"total"`
	UnreadCount   int                  `json:"unread_count"`
}

// handleGetNotifications returns the list of notifications for the authenticated user.
// GET /api/user/me/notifications
// Query params: limit (default 20, max 100), offset (default 0), unread_only (default false)
func (a *App) handleGetNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	unreadOnlyStr := r.URL.Query().Get("unread_only")

	limit := 20
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}

	offset := 0
	if offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	unreadOnly := unreadOnlyStr == "true" || unreadOnlyStr == "1"

	// Get notifications using replica for reads
	notifications, total, err := inapp.GetNotifications(ctx, a.pool.Replica(), userID, limit, offset, unreadOnly)
	if err != nil {
		if errors.Is(err, inapp.ErrPartialScanFailure) {
			// Some rows failed to scan - log but return partial results
			a.log().Warn("Partial notification scan failure", zap.Error(err), zap.String("user_id", userID))
		} else {
			a.log().Error("Failed to get notifications", zap.Error(err), zap.String("user_id", userID))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
			return
		}
	}

	// Get unread count
	unreadCount, err := inapp.GetUnreadCount(ctx, a.pool.Replica(), userID)
	if err != nil {
		a.log().Warn("Failed to get unread count", zap.Error(err), zap.String("user_id", userID))
		unreadCount = 0
	}

	writeJSON(w, http.StatusOK, NotificationsResponse{
		Notifications: notifications,
		Total:         total,
		UnreadCount:   unreadCount,
	})
}

// handleGetUnreadNotificationCount returns just the count of unread notifications.
// GET /api/user/me/notifications/unread-count
// This is a lightweight endpoint for polling.
func (a *App) handleGetUnreadNotificationCount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	count, err := inapp.GetUnreadCount(ctx, a.pool.Replica(), userID)
	if err != nil {
		a.log().Error("Failed to get unread count", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// handleMarkNotificationRead marks a single notification as read.
// POST /api/user/me/notifications/{id}/read
func (a *App) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	notificationID := chi.URLParam(r, "id")

	if notificationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.NotificationIDRequired})
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(notificationID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.NotificationIDInvalid})
		return
	}

	err := inapp.MarkAsRead(ctx, a.pool.Primary(), notificationID, userID)
	if err != nil {
		if errors.Is(err, inapp.ErrNotificationNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.NotificationNotFound})
			return
		}
		if errors.Is(err, inapp.ErrNotificationNotOwned) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": msg.NotificationNotYours})
			return
		}
		a.log().Error("Failed to mark notification as read",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("notification_id", notificationID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// handleMarkAllNotificationsRead marks all unread notifications as read for the user.
// POST /api/user/me/notifications/read-all
func (a *App) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)

	count, err := inapp.MarkAllAsRead(ctx, a.pool.Primary(), userID)
	if err != nil {
		a.log().Error("Failed to mark all notifications as read",
			zap.Error(err),
			zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"count":   count,
	})
}

// handleDeleteNotification deletes a single notification.
// DELETE /api/user/me/notifications/{id}
func (a *App) handleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	notificationID := chi.URLParam(r, "id")

	if notificationID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.NotificationIDRequired})
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(notificationID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.NotificationIDInvalid})
		return
	}

	err := inapp.DeleteNotification(ctx, a.pool.Primary(), notificationID, userID)
	if err != nil {
		if errors.Is(err, inapp.ErrNotificationNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.NotificationNotFound})
			return
		}
		a.log().Error("Failed to delete notification",
			zap.Error(err),
			zap.String("user_id", userID),
			zap.String("notification_id", notificationID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ============================================================================
// Google OAuth Endpoints
// ============================================================================

// oauthStateKey is the Redis key prefix for OAuth state storage.
const oauthStateKeyPrefix = "auth:user:oauth:state:"

// oauthStateTTL is the TTL for OAuth state (5 minutes).
const oauthStateTTL = 5 * time.Minute

// GoogleUserInfo represents the user info returned by Google's userinfo API.
type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

// GoogleAuthRequest is the request body for POST /auth/google/callback.
type GoogleAuthRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// GoogleAuthResponse is the response for Google OAuth endpoints.
type GoogleAuthResponse struct {
	AuthURL string `json:"auth_url,omitempty"`
	State   string `json:"state,omitempty"`
}

// getGoogleOAuthConfig returns the OAuth2 config for Google.
func seedAdminUsers(ctx context.Context, database *sql.DB, log *observability.Logger) {
	type seedUser struct {
		Email, Username, DisplayName, Password string
		Roles                                  []string
	}
	users := []seedUser{
		{"admin@tragge.com", "admin", "Super Admin", "159032000", []string{"super_admin", "admin"}},
		{"user@tragge.com", "user", "Test User", "user123456", []string{"user"}},
	}

	for _, u := range users {
		var exists bool
		if err := database.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM users WHERE email = $1 OR username = $2)",
			u.Email, u.Username).Scan(&exists); err != nil {
			log.Warn("seed: failed to check user", zap.String("email", u.Email), zap.Error(err))
			continue
		}
		if exists {
			log.Info("seed: user already exists, skipping", zap.String("username", u.Username))
			continue
		}

		hash, err := auth.HashPassword(u.Password, nil)
		if err != nil {
			log.Warn("seed: failed to hash password", zap.String("username", u.Username), zap.Error(err))
			continue
		}

		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			log.Warn("seed: failed to begin tx", zap.Error(err))
			continue
		}

		var userID string
		err = tx.QueryRowContext(ctx, `
			INSERT INTO users (email, password_hash, username, display_name, status, email_verified, email_verified_at, terms_accepted_at)
			VALUES ($1, $2, $3, $4, 'active', TRUE, NOW(), NOW()) RETURNING id`,
			u.Email, hash, u.Username, u.DisplayName).Scan(&userID)
		if err != nil {
			tx.Rollback()
			log.Warn("seed: failed to insert user", zap.String("username", u.Username), zap.Error(err))
			continue
		}

		rolesFailed := false
		for _, role := range u.Roles {
			var roleID int
			if err := tx.QueryRowContext(ctx, "SELECT id FROM roles WHERE name = $1", role).Scan(&roleID); err != nil {
				log.Warn("seed: role not found", zap.String("role", role), zap.Error(err))
				rolesFailed = true
				break
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", userID, roleID); err != nil {
				log.Warn("seed: failed to assign role", zap.String("role", role), zap.Error(err))
				rolesFailed = true
				break
			}
		}
		if rolesFailed {
			tx.Rollback()
			continue
		}

		if err := tx.Commit(); err != nil {
			log.Warn("seed: failed to commit", zap.String("username", u.Username), zap.Error(err))
			continue
		}
		log.Info("seed: created user", zap.String("username", u.Username), zap.String("id", userID), zap.Strings("roles", u.Roles))
	}
}
