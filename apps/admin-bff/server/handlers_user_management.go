package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// nullIfEmpty returns nil if the string is empty, otherwise returns a pointer to the string.
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// User Management Handlers

// handleListUsers returns a paginated list of users with optional filters.
func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	q := r.URL.Query()

	limit := 50
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	offset := 0
	if o := q.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 && parsed <= 100000 {
			offset = parsed
		}
	}

	search := validation.SanitizeString(q.Get("search"))
	roleFilter := validation.SanitizeString(q.Get("role"))
	statusFilter := validation.SanitizeString(q.Get("status"))

	// Build query
	query := `
		SELECT DISTINCT u.id, u.email, u.status, u.created_at,
			   COALESCE(uv.status, 'none') as kyc_status,
			   ARRAY_AGG(DISTINCT r.name) FILTER (WHERE r.name IS NOT NULL) as roles
		FROM users u
		LEFT JOIN user_roles ur ON u.id = ur.user_id
		LEFT JOIN roles r ON ur.role_id = r.id
		LEFT JOIN user_verification uv ON u.id = uv.user_id
		WHERE 1=1`

	args := []interface{}{}
	argIdx := 1

	if search != "" {
		query += " AND (u.email ILIKE $" + itoa(argIdx) + " OR u.id::text ILIKE $" + itoa(argIdx) + ")"
		args = append(args, "%"+search+"%")
		argIdx++
	}

	if roleFilter != "" {
		query += " AND r.name = $" + itoa(argIdx)
		args = append(args, roleFilter)
		argIdx++
	}

	if statusFilter != "" {
		query += " AND u.status = $" + itoa(argIdx)
		args = append(args, statusFilter)
		argIdx++
	}

	query += " GROUP BY u.id, u.email, u.status, u.created_at, uv.status ORDER BY u.created_at DESC"

	// Get total count
	countQuery := `
		SELECT COUNT(DISTINCT u.id)
		FROM users u
		LEFT JOIN user_roles ur ON u.id = ur.user_id
		LEFT JOIN roles r ON ur.role_id = r.id
		WHERE 1=1`
	countArgs := []interface{}{}
	countArgIdx := 1

	if search != "" {
		countQuery += " AND (u.email ILIKE $" + itoa(countArgIdx) + " OR u.id::text ILIKE $" + itoa(countArgIdx) + ")"
		countArgs = append(countArgs, "%"+search+"%")
		countArgIdx++
	}

	if roleFilter != "" {
		countQuery += " AND r.name = $" + itoa(countArgIdx)
		countArgs = append(countArgs, roleFilter)
		countArgIdx++
	}

	if statusFilter != "" {
		countQuery += " AND u.status = $" + itoa(countArgIdx)
		countArgs = append(countArgs, statusFilter)
		countArgIdx++
	}

	var total int
	if err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, countQuery, countArgs...).Scan(&total)
	}); err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to count users", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Add pagination
	query += " LIMIT $" + itoa(argIdx) + " OFFSET $" + itoa(argIdx+1)
	args = append(args, limit, offset)

	result, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, query, args...)
		},
	)
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		a.log().Error("Failed to query users", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	rows := result.(*sql.Rows)
	defer rows.Close()

	users := []UserResponse{}
	for rows.Next() {
		var user UserResponse
		var rolesArray []sql.NullString
		var kycStatus sql.NullString

		if err := rows.Scan(&user.ID, &user.Email, &user.Status, &user.CreatedAt, &kycStatus, (*pqArray)(&rolesArray)); err != nil {
			a.log().Error("Failed to scan user row", zap.Error(err))
			continue
		}

		if kycStatus.Valid {
			user.KYCStatus = &kycStatus.String
		}

		user.Roles = make([]string, 0)
		for _, r := range rolesArray {
			if r.Valid {
				user.Roles = append(user.Roles, r.String)
			}
		}

		users = append(users, user)
	}

	writeJSON(w, http.StatusOK, UserListResponse{
		Users:  users,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

// handleGetUser returns comprehensive information about a single user.
func (a *App) handleGetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")

	if userID == "" {
		validation.WriteBadRequest(w, "user_id is required")
		return
	}

	var response UserDetailResponse

	// Get user basic info (circuit breaker protected)
	var username, displayName, avatarURL, country sql.NullString
	var emailVerified sql.NullBool
	err := a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT u.id, u.email, COALESCE(u.username, ''), COALESCE(u.display_name, ''),
				   COALESCE(u.avatar_url, ''), u.status, u.created_at,
				   COALESCE(u.country, ''), COALESCE(u.email_verified, false)
			FROM users u
			WHERE u.id = $1`, userID).Scan(
			&response.User.ID, &response.User.Email, &username, &displayName,
			&avatarURL, &response.User.Status, &response.User.CreatedAt,
			&country, &emailVerified)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.UserNotFound})
			return
		}
		a.log().Error("Failed to query user", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	if username.Valid && username.String != "" {
		response.User.Username = &username.String
	}
	if displayName.Valid && displayName.String != "" {
		response.User.DisplayName = &displayName.String
	}
	if avatarURL.Valid && avatarURL.String != "" {
		response.User.AvatarURL = &avatarURL.String
	}
	if country.Valid && country.String != "" {
		response.User.Country = &country.String
	}
	response.User.EmailVerified = emailVerified.Valid && emailVerified.Bool

	// Get user roles (circuit breaker protected)
	rolesResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT r.name FROM roles r
				INNER JOIN user_roles ur ON r.id = ur.role_id
				WHERE ur.user_id = $1`, userID)
		},
	)
	if err != nil {
		a.log().Error("Failed to query user roles", zap.Error(err))
	} else {
		rolesRows := rolesResult.(*sql.Rows)
		defer rolesRows.Close()
		response.Roles = []string{}
		for rolesRows.Next() {
			var role string
			if err := rolesRows.Scan(&role); err == nil {
				response.Roles = append(response.Roles, role)
			}
		}
	}

	// Get KYC information (circuit breaker protected)
	var kycSubmittedAt, kycReviewedAt sql.NullTime
	err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT COALESCE(status::text, 'none'), created_at, verified_at
			FROM user_verification
			WHERE user_id = $1`, userID).Scan(
			&response.KYC.Status, &kycSubmittedAt, &kycReviewedAt)
	})
	if err == sql.ErrNoRows {
		response.KYC.Status = "none"
	} else if err != nil {
		a.log().Warn("Failed to query KYC", zap.Error(err))
		response.KYC.Status = "none"
	} else {
		if kycSubmittedAt.Valid {
			response.KYC.SubmittedAt = &kycSubmittedAt.Time
		}
		if kycReviewedAt.Valid {
			response.KYC.ReviewedAt = &kycReviewedAt.Time
		}
	}

	// Get wallet information (circuit breaker protected)
	err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT COALESCE(balance_cents, 0), COALESCE(currency, 'USD'), COALESCE(status::text, 'active')
			FROM wallets
			WHERE user_id = $1`, userID).Scan(
			&response.Wallet.BalanceCents, &response.Wallet.Currency, &response.Wallet.Status)
	})
	if err == sql.ErrNoRows {
		response.Wallet.BalanceCents = 0
		response.Wallet.Currency = "USD"
		response.Wallet.Status = "active"
	} else if err != nil {
		a.log().Warn("Failed to query wallet", zap.Error(err))
		response.Wallet.BalanceCents = 0
		response.Wallet.Currency = "USD"
		response.Wallet.Status = "active"
	}

	// Get user statistics (circuit breaker protected)
	err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT COALESCE(total_contests, 0), COALESCE(total_wins, 0),
				   COALESCE(tragge_point, 0), COALESCE(total_trades, 0), COALESCE(total_pnl, 0)
			FROM user_stats
			WHERE user_id = $1`, userID).Scan(
			&response.Stats.TotalContests, &response.Stats.TotalWins,
			&response.Stats.TraggePoint, &response.Stats.TotalTrades, &response.Stats.TotalPnL)
	})
	if err != nil {
		// Fallback: calculate from raw data
		_ = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM contest_participants WHERE user_id = $1`, userID).Scan(&response.Stats.TotalContests)
		})
		_ = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COALESCE(SUM(realized_score), 0) FROM positions WHERE user_id = $1`, userID).Scan(&response.Stats.TotalPnL)
		})
	}

	// Get recent contests (last 10)
	response.RecentContests = []UserContestEntry{}
	contestResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
		SELECT c.id, c.name, cp.final_rank, COALESCE(cp.total_score, 0), c.ends_at
		FROM contest_participants cp
		INNER JOIN contests c ON cp.contest_id = c.id
		WHERE cp.user_id = $1
		ORDER BY c.ends_at DESC
		LIMIT 10`, userID)
		},
	)
	if err == nil {
		contestRows := contestResult.(*sql.Rows)
		defer contestRows.Close()
		for contestRows.Next() {
			var entry UserContestEntry
			var rank sql.NullInt32
			if err := contestRows.Scan(&entry.ID, &entry.Name, &rank, &entry.PnL, &entry.Date); err == nil {
				if rank.Valid {
					rankInt := int(rank.Int32)
					entry.Rank = &rankInt
				}
				response.RecentContests = append(response.RecentContests, entry)
			}
		}
	}

	// Get recent wallet transactions (last 20)
	response.RecentTransactions = []UserTransaction{}
	txResult, err := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT id, type::text, amount_cents, created_at, description, reason_code, ref_type::text, ref_id, balance_after_cents
				FROM wallet_ledger
				WHERE user_id = $1
				ORDER BY created_at DESC
				LIMIT 20`, userID)
		},
	)
	if err == nil {
		txRows := txResult.(*sql.Rows)
		defer txRows.Close()
		for txRows.Next() {
			var tx UserTransaction
			var desc, reasonCode, refType, refID sql.NullString
			if err := txRows.Scan(&tx.ID, &tx.Type, &tx.Amount, &tx.Date, &desc, &reasonCode, &refType, &refID, &tx.BalanceAfter); err == nil {
				if desc.Valid && desc.String != "" {
					tx.Description = &desc.String
				}
				if reasonCode.Valid && reasonCode.String != "" {
					tx.ReasonCode = &reasonCode.String
				}
				if refType.Valid && refType.String != "" {
					tx.RefType = &refType.String
				}
				if refID.Valid && refID.String != "" {
					tx.RefID = &refID.String
				}
				response.RecentTransactions = append(response.RecentTransactions, tx)
			}
		}
	}

	// Get affiliate information (circuit breaker protected)
	var affiliateCode sql.NullString
	var affiliateStatus sql.NullString
	err = a.circuits.ExecuteReplica(ctx, func(ctx context.Context) error {
		return a.pool.Replica().QueryRowContext(ctx, `
			SELECT rc.code, COALESCE(rc.activation_status::text, 'inactive'),
				   COUNT(r.id), COALESCE(SUM(CASE WHEN ac.status = 'credited' THEN ac.commission_cents ELSE 0 END), 0)
			FROM referral_codes rc
			LEFT JOIN referrals r ON rc.code = r.code
			LEFT JOIN affiliate_commissions ac ON rc.user_id = ac.referrer_id
			WHERE rc.user_id = $1
			GROUP BY rc.code, rc.activation_status`, userID).Scan(
			&affiliateCode, &affiliateStatus, &response.Affiliate.TotalReferrals, &response.Affiliate.TotalEarned)
	})
	if err == nil {
		if affiliateCode.Valid {
			response.Affiliate.Code = &affiliateCode.String
		}
		if affiliateStatus.Valid {
			response.Affiliate.Status = affiliateStatus.String
		} else {
			response.Affiliate.Status = "inactive"
		}
	} else {
		response.Affiliate.Status = "none"
	}

	// Get active sessions from Redis across both panel namespaces
	// (step 6: user-panel sessions live under session:user:*, admin
	// sessions under session:admin:*). A dual-role account shows up
	// in both lists.
	response.Sessions = []UserSessionInfo{}
	for _, snap := range a.listUserSessionsAllNamespaces(ctx, userID) {
		response.Sessions = append(response.Sessions, UserSessionInfo{
			ID:         snap.SessionID[:8] + "...", // Truncate for privacy
			Device:     snap.DeviceInfo,
			IP:         snap.IPAddress,
			LastActive: snap.LastSeenAt,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// handleUpdateUserRoles updates a user's roles.
func (a *App) handleUpdateUserRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")
	actorUserID := auth.GetUserID(ctx)

	if userID == "" {
		validation.WriteBadRequest(w, "user_id is required")
		return
	}

	// Prevent self-role-modification
	if userID == actorUserID {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": adminMsg.CannotModifyOwnRoles,
		})
		return
	}

	var req UpdateUserRolesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate roles
	validRoles := map[string]bool{auth.RoleUser: true, auth.RoleSupportAdmin: true, auth.RoleSuperAdmin: true}
	for _, role := range req.Roles {
		if !validRoles[role] {
			validation.WriteBadRequest(w, "invalid role: "+role)
			return
		}
	}

	if strings.TrimSpace(req.Reason) == "" {
		a.auditSensitiveDenial(ctx, actorUserID, actionUserRolesUpdate, userID, "mandatory_reason_denied")
		validation.WriteBadRequest(w, "reason is required")
		return
	}
	req.Reason = validation.SanitizeString(req.Reason)

	// Ensure at least 'user' role is assigned
	if len(req.Roles) == 0 {
		req.Roles = []string{"user"}
	}

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

	// Get current roles for audit (circuit breaker protected)
	var oldRoles []string
	rolesResult, rolesErr := a.circuits.ExecuteReplicaWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.pool.Replica().QueryContext(ctx, `
				SELECT r.name FROM roles r
				INNER JOIN user_roles ur ON r.id = ur.role_id
				WHERE ur.user_id = $1`, userID)
		},
	)
	if rolesErr == nil {
		rolesRows := rolesResult.(*sql.Rows)
		defer rolesRows.Close()
		for rolesRows.Next() {
			var role string
			if err := rolesRows.Scan(&role); err == nil {
				oldRoles = append(oldRoles, role)
			}
		}
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

	// Delete existing roles
	_, err = tx.ExecContext(ctx, "DELETE FROM user_roles WHERE user_id = $1", userID)
	if err != nil {
		a.log().Error("Failed to delete user roles", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Insert new roles
	for _, roleName := range req.Roles {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO user_roles (user_id, role_id)
			SELECT $1, id FROM roles WHERE name = $2`, userID, roleName)
		if err != nil {
			a.log().Error("Failed to insert user role", zap.Error(err), zap.String("role", roleName))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Write audit log
	auditPayload := map[string]interface{}{
		"user_id":   userID,
		"old_roles": oldRoles,
		"new_roles": req.Roles,
		"reason":    req.Reason,
	}
	payloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "user.roles.updated", "user", userID, payloadJSON)
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

	// Invalidate every session the target user holds, in both
	// namespaces. Role tokens embed the role list at issue time, so
	// leaving a demoted admin's old session alive would let them keep
	// hitting admin endpoints until their short-lived access token
	// expired (~15min). A user with the admin role revoked might also
	// have a stale admin-panel session and a separate user-panel
	// session; we have to wipe both. This is the #932 requirement the
	// Step 6 split preserved.
	sessionsInvalidated := a.invalidateAllUserSessionsAllNamespaces(ctx, userID)
	if a.reauthentication != nil {
		_ = a.reauthentication.RevokeActor(ctx, userID)
	}

	a.log().Info("User roles updated",
		zap.String("user_id", userID),
		zap.Strings("old_roles", oldRoles),
		zap.Strings("new_roles", req.Roles),
		zap.Int("sessions_invalidated", sessionsInvalidated),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": adminMsg.RolesUpdated,
		"roles":   req.Roles,
	})
}

// handleUpdateUserStatus updates a user's status (activate/suspend).
func (a *App) handleUpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")
	actorUserID := auth.GetUserID(ctx)

	if userID == "" {
		validation.WriteBadRequest(w, "user_id is required")
		return
	}

	var req UpdateUserStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate status
	if req.Status != "active" && req.Status != "suspended" {
		validation.WriteBadRequest(w, "status must be 'active' or 'suspended'")
		return
	}

	// Check if user exists and get current status
	var oldStatus string
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx, "SELECT status FROM users WHERE id = $1", userID).Scan(&oldStatus)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.UserNotFound})
			return
		}
		a.log().Error("Failed to query user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Prevent self-suspension
	if userID == actorUserID && req.Status == "suspended" {
		validation.WriteBadRequest(w, "cannot suspend your own account")
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

	// Update user status
	_, err = tx.ExecContext(ctx, "UPDATE users SET status = $1 WHERE id = $2", req.Status, userID)
	if err != nil {
		a.log().Error("Failed to update user status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	auditPayload := map[string]interface{}{
		"user_id":    userID,
		"old_status": oldStatus,
		"new_status": req.Status,
		"reason":     req.Reason,
	}
	payloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "user.status.updated", "user", userID, payloadJSON)
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

	a.log().Info("User status updated",
		zap.String("user_id", userID),
		zap.String("old_status", oldStatus),
		zap.String("new_status", req.Status),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": adminMsg.StatusUpdated,
		"status":  req.Status,
	})
}

// handleBanUser bans a user account.
func (a *App) handleBanUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")
	actorUserID := auth.GetUserID(ctx)

	if userID == "" {
		validation.WriteBadRequest(w, "user_id is required")
		return
	}

	var req BanUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate duration
	validDurations := map[string]bool{"permanent": true, "7d": true, "30d": true}
	if !validDurations[req.Duration] {
		validation.WriteBadRequest(w, "duration must be 'permanent', '7d', or '30d'")
		return
	}

	if req.Reason == "" {
		validation.WriteBadRequest(w, "reason is required")
		return
	}

	// Sanitize input
	req.Reason = validation.SanitizeString(req.Reason)

	// Check if user exists and get current status
	var oldStatus string
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx, "SELECT status FROM users WHERE id = $1", userID).Scan(&oldStatus)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.UserNotFound})
			return
		}
		a.log().Error("Failed to query user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Prevent self-ban
	if userID == actorUserID {
		validation.WriteBadRequest(w, "cannot ban your own account")
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

	// Calculate ban expiry based on duration
	var banExpiresAt *time.Time
	switch req.Duration {
	case "7d":
		t := time.Now().Add(7 * 24 * time.Hour)
		banExpiresAt = &t
	case "30d":
		t := time.Now().Add(30 * 24 * time.Hour)
		banExpiresAt = &t
		// "permanent" -> nil (no expiry)
	}

	// Update user status to suspended with ban expiry
	_, err = tx.ExecContext(ctx,
		"UPDATE users SET status = 'suspended', ban_expires_at = $1 WHERE id = $2",
		banExpiresAt, userID)
	if err != nil {
		a.log().Error("Failed to update user status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	auditPayload := map[string]interface{}{
		"user_id":        userID,
		"old_status":     oldStatus,
		"new_status":     "suspended",
		"reason":         req.Reason,
		"duration":       req.Duration,
		"ban_expires_at": banExpiresAt,
		"action":         "ban",
	}
	payloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "user.banned", "user", userID, payloadJSON)
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

	// Invalidate all user sessions across both panel namespaces.
	// Fire-and-forget: the DB row is already flipped to banned, so a
	// Redis blip here leaves the user with a few stale sessions that
	// will fail their next /me check against Postgres.
	_ = a.invalidateAllUserSessionsAllNamespaces(ctx, userID)

	a.log().Info("User banned",
		zap.String("user_id", userID),
		zap.String("reason", req.Reason),
		zap.String("duration", req.Duration),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":  adminMsg.UserBanned,
		"duration": req.Duration,
	})
}

// handleUnbanUser unbans a user account.
func (a *App) handleUnbanUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")
	actorUserID := auth.GetUserID(ctx)

	if userID == "" {
		validation.WriteBadRequest(w, "user_id is required")
		return
	}

	var req UnbanUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body for unban
		req = UnbanUserRequest{}
	}

	// Sanitize input
	if req.Reason != "" {
		req.Reason = validation.SanitizeString(req.Reason)
	}

	// Check if user exists and get current status
	var oldStatus string
	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		return a.pool.Primary().QueryRowContext(ctx, "SELECT status FROM users WHERE id = $1", userID).Scan(&oldStatus)
	})
	if err != nil {
		if a.isCircuitError(w, err) {
			return
		}
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": adminMsg.UserNotFound})
			return
		}
		a.log().Error("Failed to query user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Check if user is actually suspended
	if oldStatus != "suspended" {
		validation.WriteBadRequest(w, "user is not banned")
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

	// Update user status to active and clear ban expiry
	_, err = tx.ExecContext(ctx, "UPDATE users SET status = 'active', ban_expires_at = NULL WHERE id = $1", userID)
	if err != nil {
		a.log().Error("Failed to update user status", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Write audit log
	auditPayload := map[string]interface{}{
		"user_id":    userID,
		"old_status": oldStatus,
		"new_status": "active",
		"reason":     req.Reason,
		"action":     "unban",
	}
	payloadJSON, _ := json.Marshal(auditPayload)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "user.unbanned", "user", userID, payloadJSON)
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

	a.log().Info("User unbanned",
		zap.String("user_id", userID),
		zap.String("reason", req.Reason),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": adminMsg.UserUnbanned,
	})
}

// handleTerminateUserSessions terminates all sessions for a user.
func (a *App) handleTerminateUserSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := chi.URLParam(r, "user_id")
	actorUserID := auth.GetUserID(ctx)

	if userID == "" {
		validation.WriteBadRequest(w, "user_id is required")
		return
	}

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

	// Terminate all sessions across both panel namespaces. A dual-role
	// account held as both admin and user gets both sessions killed in
	// a single admin click ÃƒÂ¢Ã¢â€šÂ¬Ã¢â‚¬Â step 6 intent.
	sessionsTerminated := a.invalidateAllUserSessionsAllNamespaces(ctx, userID)

	// Write audit log
	auditPayload := map[string]interface{}{
		"user_id":             userID,
		"sessions_terminated": sessionsTerminated,
	}
	payloadJSON, _ := json.Marshal(auditPayload)
	_ = a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx,
			`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
			 VALUES ($1, $2, $3, $4, $5)`,
			actorUserID, "user.sessions.terminated", "user", userID, payloadJSON)
		return execErr
	})

	a.log().Info("User sessions terminated",
		zap.String("user_id", userID),
		zap.Int("sessions_terminated", sessionsTerminated),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":             adminMsg.SessionsTerminated,
		"sessions_terminated": sessionsTerminated,
	})
}

// PermissionsResponse represents the response for the permissions endpoint.
type PermissionsResponse struct {
	Role         string   `json:"role"`
	Permissions  []string `json:"permissions"`
	IsViewer     bool     `json:"is_viewer"`
	IsAdmin      bool     `json:"is_admin"`
	IsSuperAdmin bool     `json:"is_super_admin"`
}

// handleGetMyPermissions returns the current admin user's role and permissions.
func (a *App) handleGetMyPermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	state, err := a.loadAdminSecurityState(ctx, auth.GetUserID(ctx))
	if err != nil {
		a.log().Error("Failed to query canonical Admin permissions", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}
	canonical, hasAdminRole := canonicalAdminRoles(state.Roles)
	if !canonical || !hasAdminRole {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": adminMsg.AccessDenied})
		return
	}
	role := auth.RoleSupportAdmin
	isSuperAdmin := state.hasRole(auth.RoleSuperAdmin)
	if isSuperAdmin {
		role = auth.RoleSuperAdmin
	}
	writeJSON(w, http.StatusOK, PermissionsResponse{
		Role: role, Permissions: effectiveAdminPermissions(state),
		IsViewer: false, IsAdmin: state.hasRole(auth.RoleSupportAdmin), IsSuperAdmin: isSuperAdmin,
	})
}

// DashboardMetrics represents the aggregated metrics for the admin dashboard.
type DashboardMetrics struct {
	Users     UserMetrics      `json:"users"`
	Contests  ContestMetrics   `json:"contests"`
	Financial FinancialMetrics `json:"financial"`
	Trading   TradingMetrics   `json:"trading"`
	KYC       KYCMetrics       `json:"kyc"`
	Affiliate AffiliateMetrics `json:"affiliate"`
}

// UserMetrics represents user-related metrics.
type UserMetrics struct {
	Total         int64 `json:"total"`
	NewToday      int64 `json:"new_today"`
	NewThisWeek   int64 `json:"new_this_week"`
	NewThisMonth  int64 `json:"new_this_month"`
	VerifiedCount int64 `json:"verified_count"`
}

// ContestMetrics represents contest-related metrics.
type ContestMetrics struct {
	Total          int64 `json:"total"`
	ActiveNow      int64 `json:"active_now"`
	Scheduled      int64 `json:"scheduled"`
	CompletedToday int64 `json:"completed_today"`
}

// FinancialMetrics represents financial metrics.
type FinancialMetrics struct {
	TotalDepositsTodayCents    int64 `json:"total_deposits_today_cents"`
	TotalWithdrawalsTodayCents int64 `json:"total_withdrawals_today_cents"`
	PendingWithdrawalsCount    int64 `json:"pending_withdrawals_count"`
	TotalRevenueCents          int64 `json:"total_revenue_cents"`
}

// TradingMetrics represents trading activity metrics.
type TradingMetrics struct {
	ActiveTradersNow int64 `json:"active_traders_now"`
	OrdersToday      int64 `json:"orders_today"`
	TradesToday      int64 `json:"trades_today"`
}

// KYCMetrics represents KYC-related metrics.
type KYCMetrics struct {
	PendingCount int64 `json:"pending_count"`
}

// AffiliateMetrics represents affiliate program metrics.
type AffiliateMetrics struct {
	PendingActivationCount int64 `json:"pending_activation_count"`
}

// handleGetDashboard returns aggregated metrics for the admin dashboard.
// GET /api/admin/dashboard
func (a *App) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	startOfWeek := startOfDay.AddDate(0, 0, -int(now.Weekday()))
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	metrics := DashboardMetrics{}

	// User metrics - run queries in parallel for better performance
	var wg sync.WaitGroup
	var mu sync.Mutex
	var queryErrors []error
	sem := make(chan struct{}, 5)

	// Helper to safely record errors
	recordError := func(err error, context string) {
		if err != nil {
			mu.Lock()
			queryErrors = append(queryErrors, fmt.Errorf("%s: %w", context, err))
			mu.Unlock()
			a.log().Error("Dashboard query failed", zap.String("context", context), zap.Error(err))
		}
	}

	// Query user metrics
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-user-counts", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var total, newToday, newWeek, newMonth int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT
					COUNT(*),
					COUNT(*) FILTER (WHERE created_at >= $1),
					COUNT(*) FILTER (WHERE created_at >= $2),
					COUNT(*) FILTER (WHERE created_at >= $3)
				FROM users
			`, startOfDay, startOfWeek, startOfMonth).Scan(&total, &newToday, &newWeek, &newMonth)
		})
		recordError(err, "user counts")
		mu.Lock()
		metrics.Users.Total = total
		metrics.Users.NewToday = newToday
		metrics.Users.NewThisWeek = newWeek
		metrics.Users.NewThisMonth = newMonth
		mu.Unlock()
	})

	// Query verified users count
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-verified-users", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var verified int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM user_verification WHERE status = 'verified'
			`).Scan(&verified)
		})
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			recordError(err, "verified users")
		}
		mu.Lock()
		metrics.Users.VerifiedCount = verified
		mu.Unlock()
	})

	// Query contest metrics
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-contest-counts", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var total, active, scheduled int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT
					COUNT(*),
					COUNT(*) FILTER (WHERE status = 'running'),
					COUNT(*) FILTER (WHERE status IN ('scheduled', 'registration_open'))
				FROM contests
			`).Scan(&total, &active, &scheduled)
		})
		recordError(err, "contest counts")
		mu.Lock()
		metrics.Contests.Total = total
		metrics.Contests.ActiveNow = active
		metrics.Contests.Scheduled = scheduled
		mu.Unlock()
	})

	// Query completed today count
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-completed-contests", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var completedToday int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM contests
				WHERE status = 'completed' AND ends_at >= $1 AND ends_at < $2
			`, startOfDay, startOfDay.Add(24*time.Hour)).Scan(&completedToday)
		})
		recordError(err, "completed contests")
		mu.Lock()
		metrics.Contests.CompletedToday = completedToday
		mu.Unlock()
	})

	// Query financial metrics - deposits today
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-deposits-today", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var depositsToday int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COALESCE(SUM(amount_cents), 0) FROM wallet_ledger
				WHERE type = 'deposit' AND created_at >= $1
			`, startOfDay).Scan(&depositsToday)
		})
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			recordError(err, "deposits today")
		}
		mu.Lock()
		metrics.Financial.TotalDepositsTodayCents = depositsToday
		mu.Unlock()
	})

	// Query financial metrics - withdrawals today
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-withdrawals-today", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var withdrawalsToday int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COALESCE(SUM(amount_cents), 0) FROM payouts
				WHERE status = 'succeeded' AND completed_at >= $1
			`, startOfDay).Scan(&withdrawalsToday)
		})
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			recordError(err, "withdrawals today")
		}
		mu.Lock()
		metrics.Financial.TotalWithdrawalsTodayCents = withdrawalsToday
		mu.Unlock()
	})

	// Query pending withdrawals count
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-pending-withdrawals", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var pendingWithdrawals int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM payouts WHERE status = 'pending'
			`).Scan(&pendingWithdrawals)
		})
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			recordError(err, "pending withdrawals")
		}
		mu.Lock()
		metrics.Financial.PendingWithdrawalsCount = pendingWithdrawals
		mu.Unlock()
	})

	// Query total revenue (entry fees collected)
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-total-revenue", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var totalRevenue int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COALESCE(SUM(ABS(amount_cents)), 0) FROM wallet_ledger
				WHERE type = 'contest_entry'
			`).Scan(&totalRevenue)
		})
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			recordError(err, "total revenue")
		}
		mu.Lock()
		metrics.Financial.TotalRevenueCents = totalRevenue
		mu.Unlock()
	})

	// Query trading metrics - orders today
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-orders-today", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var ordersToday int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM orders WHERE created_at >= $1
			`, startOfDay).Scan(&ordersToday)
		})
		recordError(err, "orders today")
		mu.Lock()
		metrics.Trading.OrdersToday = ordersToday
		mu.Unlock()
	})

	// Query trading metrics - trades/fills today
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-trades-today", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var tradesToday int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM fills WHERE created_at >= $1
			`, startOfDay).Scan(&tradesToday)
		})
		recordError(err, "trades today")
		mu.Lock()
		metrics.Trading.TradesToday = tradesToday
		mu.Unlock()
	})

	// Query active traders (unique users with orders in running contests)
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-active-traders", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var activeTraders int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(DISTINCT o.user_id) FROM orders o
				INNER JOIN contests c ON o.contest_id = c.id
				WHERE c.status = 'running' AND o.created_at >= $1
			`, startOfDay).Scan(&activeTraders)
		})
		recordError(err, "active traders")
		mu.Lock()
		metrics.Trading.ActiveTradersNow = activeTraders
		mu.Unlock()
	})

	// Query KYC pending count
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-kyc-pending", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var kycPending int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM user_verification WHERE status IN ('pending', 'under_review')
			`).Scan(&kycPending)
		})
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			recordError(err, "kyc pending")
		}
		mu.Lock()
		metrics.KYC.PendingCount = kycPending
		mu.Unlock()
	})

	// Query affiliate pending activation count
	wg.Add(1)
	sem <- struct{}{}
	infra.SafeGo(a.log(), "dashboard-affiliate-pending", func() {
		defer wg.Done()
		defer func() { <-sem }()
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var affiliatePending int64
		err := a.circuits.ExecuteReplica(queryCtx, func(ctx context.Context) error {
			return a.pool.Replica().QueryRowContext(ctx, `
				SELECT COUNT(*) FROM referral_codes WHERE activation_status = 'pending'
			`).Scan(&affiliatePending)
		})
		if err != nil && !strings.Contains(err.Error(), "does not exist") {
			recordError(err, "affiliate pending")
		}
		mu.Lock()
		metrics.Affiliate.PendingActivationCount = affiliatePending
		mu.Unlock()
	})

	wg.Wait()

	// Log any errors but still return partial data
	if len(queryErrors) > 0 {
		a.log().Warn("Dashboard had partial query failures", zap.Int("error_count", len(queryErrors)))
	}

	writeJSON(w, http.StatusOK, metrics)
}

// adminTxWrapper implements wallet.TxExecutor for database transactions.
type adminTxWrapper struct {
	tx *sql.Tx
}

func (w *adminTxWrapper) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return w.tx.ExecContext(ctx, query, args...)
}

func (w *adminTxWrapper) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return w.tx.QueryRowContext(ctx, query, args...)
}

func (w *adminTxWrapper) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return w.tx.QueryContext(ctx, query, args...)
}

// generateSecurePassword generates a cryptographically secure random password.
func generateSecurePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand failed: %w", err)
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

// handleAdminCreateUser creates a new user from the admin panel.
func (a *App) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var req AdminCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// 1. Validate email
	normalizedEmail, valid := validation.ValidateEmail(req.Email)
	if !valid {
		validation.WriteBadRequest(w, "invalid email address")
		return
	}
	req.Email = validation.SanitizeEmail(normalizedEmail)

	// 2. Handle password: generate if empty, validate if provided
	temporaryPassword := ""
	password := req.Password
	if password == "" {
		var err error
		password, err = generateSecurePassword(16)
		if err != nil {
			a.log().Error("Failed to generate password", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
		temporaryPassword = password
	} else {
		ok, msg := validation.ValidatePassword(password, validation.DefaultPasswordConstraints())
		if !ok {
			validation.WriteBadRequest(w, msg)
			return
		}
	}

	// 3. Validate and default roles
	if len(req.Roles) == 0 {
		req.Roles = []string{"user"}
	}
	validRoles := map[string]bool{auth.RoleUser: true, auth.RoleSupportAdmin: true, auth.RoleSuperAdmin: true}
	for _, role := range req.Roles {
		if !validRoles[role] {
			validation.WriteBadRequest(w, "invalid role: "+role)
			return
		}
	}

	// Deduplicate roles
	seen := make(map[string]bool)
	dedupedRoles := make([]string, 0, len(req.Roles))
	for _, role := range req.Roles {
		if !seen[role] {
			seen[role] = true
			dedupedRoles = append(dedupedRoles, role)
		}
	}
	req.Roles = dedupedRoles

	// 4. Elevated account creation is Super-Admin-only and requires an exact,
	// one-time password-reauthentication grant bound to the normalized email.
	elevated := false
	for _, role := range req.Roles {
		if role == auth.RoleSupportAdmin || role == auth.RoleSuperAdmin {
			elevated = true
			break
		}
	}
	if elevated {
		if strings.TrimSpace(req.Reason) == "" {
			a.auditSensitiveDenial(ctx, actorUserID, actionElevatedUserCreate, req.Email, "mandatory_reason_denied")
			validation.WriteBadRequest(w, "reason is required")
			return
		}
		req.Reason = validation.SanitizeString(req.Reason)
		if err := a.consumeSensitiveGrant(r, actionElevatedUserCreate, req.Email, "users.edit"); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "sensitive action denied"})
			return
		}
	}

	// 5. Hash password
	passwordHash, err := a.auth.HashPassword(password)
	if err != nil {
		a.log().Error("Failed to hash password", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// 6. Transaction: insert user + assign roles
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

	// Insert user
	var userID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, email_verified, status, terms_accepted_at)
		 VALUES ($1, $2, $3, 'active', NOW()) RETURNING id`,
		req.Email, passwordHash, req.EmailVerified,
	).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
			writeJSON(w, http.StatusConflict, map[string]string{"error": adminMsg.UserAlreadyExists})
			return
		}
		a.log().Error("Failed to insert user", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Set display_name if provided
	if req.DisplayName != "" {
		if utf8.RuneCountInString(req.DisplayName) > 100 {
			validation.WriteBadRequest(w, "display_name must be 100 characters or less")
			return
		}
		sanitizedName := validation.SanitizeString(req.DisplayName)
		_, err = tx.ExecContext(ctx, `UPDATE users SET display_name = $1 WHERE id = $2`, sanitizedName, userID)
		if err != nil {
			a.log().Warn("Failed to set display name", zap.Error(err))
		}
	}

	// Assign roles
	for _, roleName := range req.Roles {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO user_roles (user_id, role_id)
			 SELECT $1, id FROM roles WHERE name = $2`, userID, roleName)
		if err != nil {
			a.log().Error("Failed to assign role", zap.Error(err), zap.String("role", roleName))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
			return
		}
	}

	// Audit log
	auditPayload := map[string]interface{}{
		"user_id":        userID,
		"email":          req.Email,
		"roles":          req.Roles,
		"email_verified": req.EmailVerified,
		"created_by":     actorUserID,
		"reason":         req.Reason,
	}
	payloadJSON, _ := json.Marshal(auditPayload)
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorUserID, "user.created_by_admin", "user", userID, payloadJSON); err != nil {
		a.log().Error("Failed to write mandatory user-creation audit", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	// Commit
	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": adminMsg.InternalError})
		return
	}

	a.log().Info("Admin created user",
		zap.String("user_id", userID),
		zap.String("email", req.Email),
		zap.Strings("roles", req.Roles),
		zap.String("actor_id", actorUserID))

	writeJSON(w, http.StatusCreated, AdminCreateUserResponse{
		UserID:            userID,
		Email:             req.Email,
		Roles:             req.Roles,
		TemporaryPassword: temporaryPassword,
		Message:           "user created successfully",
	})
}
