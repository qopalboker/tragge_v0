package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"go.uber.org/zap"
)

// Step 5/7: admin-bff owns its own refresh/logout/me endpoints now
// that admin tokens are signed with JWT_SECRET_ADMIN and scoped to
// aud=admin. The user-bff's /api/user/auth/refresh no longer works
// for admin sessions because the aud claim would mismatch and the
// refresh secret is separate.

// handleAdminRefresh exchanges the refresh_token_admin cookie for a
// fresh admin token pair. Mirrors the user-bff flow but reads the
// admin-scoped cookie and writes a new one back via
// setAdminRefreshTokenCookie.
//
// 401 responses deliberately do NOT clear the cookie â€” cookies are
// origin-wide and a sibling tab may have just refreshed successfully.
// The client decides when to clear state.
func (a *App) handleAdminRefresh(w http.ResponseWriter, r *http.Request) {
	refreshTokenValue := ""
	if cookie, err := r.Cookie(adminRefreshTokenCookieName); err == nil && cookie.Value != "" {
		refreshTokenValue = cookie.Value
	}

	if refreshTokenValue == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token missing"})
		return
	}

	ctx := r.Context()

	claims, err := a.auth.Token.ValidateRefreshToken(refreshTokenValue)
	if err != nil {
		a.log().Warn("Invalid admin refresh token",
			zap.Error(err),
			zap.String("ip", getAdminClientIP(r)))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
		return
	}

	tokenPair, err := a.auth.Refresh(ctx, claims.SessionID, refreshTokenValue)
	if err != nil {
		a.log().Warn("Admin token refresh failed",
			zap.Error(err),
			zap.String("user_id", claims.UserID),
			zap.String("session_id", claims.SessionID),
			zap.String("ip", getAdminClientIP(r)))
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh failed"})
		return
	}

	a.log().Info("Admin token refreshed",
		zap.String("user_id", claims.UserID),
		zap.String("session_id", claims.SessionID),
		zap.String("ip", getAdminClientIP(r)))

	setAdminRefreshTokenCookie(w, r, tokenPair.RefreshToken)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": tokenPair.AccessToken,
		"expires_at":   tokenPair.ExpiresAt,
	})
}

// handleAdminLogout revokes the current admin session and clears the
// refresh_token_admin / tragge_session_hint_admin cookies.
func (a *App) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actorID := auth.GetUserID(ctx)
	sessionID := auth.GetSessionID(ctx)

	if sessionID != "" && a.auth.Session != nil {
		if err := a.auth.Session.Delete(ctx, sessionID); err != nil {
			a.log().Warn("Failed to delete admin session on logout",
				zap.Error(err),
				zap.String("session_id", sessionID))
			// Fall through â€” cookie clearing still runs, the client
			// has already wiped its in-memory state.
		}
	}
	if actorID != "" && a.reauthentication != nil {
		if err := a.reauthentication.RevokeActor(ctx, actorID); err != nil {
			a.log().Warn("Failed to revoke Admin reauthentication grants on logout", zap.Error(err))
		}
	}
	if actorID != "" && a.mfaChallenges != nil {
		if err := a.mfaChallenges.RevokeUser(ctx, actorID); err != nil {
			a.log().Warn("Failed to revoke Admin MFA challenges on logout", zap.Error(err))
		}
	}

	clearAdminRefreshTokenCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// adminMeResponse mirrors the minimal shape the admin-frontend reads
// from /api/admin/me. Kept lean: display_name + avatar + roles is
// enough for the sidebar; deep profile edits live in the user panel.
type adminMeResponse struct {
	UserID      string   `json:"user_id"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	Username    string   `json:"username,omitempty"`
	DisplayName string   `json:"display_name,omitempty"`
	AvatarURL   string   `json:"avatar_url,omitempty"`
	CreatedAt   string   `json:"created_at"`
}

// handleAdminMe returns the current admin user's basic profile. Called
// after login/refresh to populate the sidebar.
func (a *App) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := auth.GetUserID(ctx)
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthenticated"})
		return
	}

	var (
		email       string
		username    sql.NullString
		displayName sql.NullString
		avatarURL   sql.NullString
		createdAt   string
	)

	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT u.email,
		       u.username,
		       u.display_name,
		       u.avatar_url,
		       u.created_at
		FROM users u
		WHERE u.id = $1`, userID).
		Scan(&email, &username, &displayName, &avatarURL, &createdAt)
	if err != nil {
		a.log().Error("Failed to load admin /me", zap.Error(err), zap.String("user_id", userID))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	roles := loadAdminUserRoles(ctx, a, userID)

	resp := adminMeResponse{
		UserID:    userID,
		Email:     email,
		Roles:     roles,
		CreatedAt: createdAt,
	}
	if username.Valid {
		resp.Username = username.String
	}
	if displayName.Valid {
		resp.DisplayName = displayName.String
	}
	if avatarURL.Valid {
		resp.AvatarURL = avatarURL.String
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// loadAdminUserRoles fetches a user's roles via the replica pool.
// Errors surface as an empty slice â€” the caller is expected to pair
// this with a separate identity check (ValidateToken already did).
func loadAdminUserRoles(ctx context.Context, a *App, userID string) []string {
	rows, err := a.pool.Replica().QueryContext(ctx, `
		SELECT r.name FROM roles r
		INNER JOIN user_roles ur ON r.id = ur.role_id
		WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	roles := []string{}
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err == nil {
			roles = append(roles, role)
		}
	}
	return roles
}
