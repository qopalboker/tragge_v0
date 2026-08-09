package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

const (
	adminReauthenticationHeader = "X-Admin-Reauth-Grant"
	actionWithdrawalComplete    = "withdrawal.complete"
	actionWalletAdjust          = "wallet.adjust"
	actionUserRolesUpdate       = "user.roles.update"
	actionElevatedUserCreate    = "user.create.elevated"
	actionAdminMFAReset         = "admin.mfa.reset"
)

type sensitiveActionSpec struct {
	Permission string
}

var sensitiveAdminActions = map[string]sensitiveActionSpec{
	actionWithdrawalComplete: {Permission: "withdrawals.manage"},
	actionWalletAdjust:       {Permission: "users.wallet.charge"},
	actionUserRolesUpdate:    {Permission: "users.edit"},
	actionElevatedUserCreate: {Permission: "users.edit"},
	actionAdminMFAReset:      {Permission: "users.edit"},
}

type adminSecurityState struct {
	PasswordHash string
	Status       string
	Roles        []string
	Permissions  []string
}

func (s adminSecurityState) fingerprint() string {
	return auth.ReauthenticationSecurityFingerprint(s.PasswordHash, s.Roles, s.Permissions)
}

func (s adminSecurityState) hasRole(want string) bool {
	for _, role := range s.Roles {
		if role == want {
			return true
		}
	}
	return false
}

func effectiveAdminPermissions(state adminSecurityState) []string {
	if state.hasRole(auth.RoleSuperAdmin) {
		return append([]string(nil), state.Permissions...)
	}
	filtered := make([]string, 0, len(state.Permissions))
	for _, permission := range state.Permissions {
		if strings.HasPrefix(permission, "kyc.") || strings.HasPrefix(permission, "support.") || strings.HasPrefix(permission, "tickets.") {
			filtered = append(filtered, permission)
		}
	}
	return filtered
}

func (s adminSecurityState) hasPermission(want string) bool {
	for _, permission := range s.Permissions {
		if permission == want {
			return true
		}
	}
	return false
}

func canonicalAdminRoles(roles []string) (bool, bool) {
	hasAdminRole := false
	for _, role := range roles {
		switch role {
		case auth.RoleUser:
		case auth.RoleSupportAdmin, auth.RoleSuperAdmin:
			hasAdminRole = true
		default:
			return false, false
		}
	}
	return true, hasAdminRole
}

func (a *App) loadAdminSecurityState(ctx context.Context, actorID string) (adminSecurityState, error) {
	var state adminSecurityState
	if a == nil || a.pool == nil || strings.TrimSpace(actorID) == "" {
		return state, errors.New("admin security state unavailable")
	}
	if err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT password_hash, status FROM users WHERE id = $1 AND COALESCE(is_system_account, FALSE) = FALSE`,
		actorID,
	).Scan(&state.PasswordHash, &state.Status); err != nil {
		return state, err
	}

	rows, err := a.pool.Primary().QueryContext(ctx, `
		SELECT DISTINCT r.name, p.name
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		LEFT JOIN role_permissions rp ON rp.role_id = r.id
		LEFT JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1
		ORDER BY r.name, p.name`, actorID)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	roleSet := make(map[string]struct{})
	permissionSet := make(map[string]struct{})
	for rows.Next() {
		var role string
		var permission sql.NullString
		if err := rows.Scan(&role, &permission); err != nil {
			return state, err
		}
		roleSet[role] = struct{}{}
		if permission.Valid {
			permissionSet[permission.String] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return state, err
	}
	for role := range roleSet {
		state.Roles = append(state.Roles, role)
	}
	for permission := range permissionSet {
		state.Permissions = append(state.Permissions, permission)
	}
	sort.Strings(state.Roles)
	sort.Strings(state.Permissions)
	return state, nil
}

func (a *App) validateSensitiveActor(ctx context.Context, actorID, permission string) (adminSecurityState, string, error) {
	state, err := a.loadAdminSecurityState(ctx, actorID)
	if err != nil || state.Status != "active" {
		return state, "security_state_unavailable", errors.New("sensitive action denied")
	}
	canonical, hasAdminRole := canonicalAdminRoles(state.Roles)
	if !canonical || !hasAdminRole || !state.hasRole(auth.RoleSuperAdmin) {
		return state, "role_denied", errors.New("sensitive action denied")
	}
	if !state.hasPermission(permission) {
		return state, "permission_denied", errors.New("sensitive action denied")
	}
	return state, "", nil
}

func (a *App) writeSecurityAudit(ctx context.Context, actorID, action, targetType, targetID string, payload map[string]interface{}) error {
	if a == nil || a.pool == nil {
		return errors.New("audit unavailable")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = a.pool.Primary().ExecContext(ctx,
		`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
		 VALUES ($1, $2, $3, $4, $5)`, actorID, action, targetType, targetID, encoded)
	return err
}

func (a *App) auditSensitiveDenial(ctx context.Context, actorID, action, resourceID, category string) {
	if err := a.writeSecurityAudit(ctx, actorID, "admin.sensitive_action.denied", "security_action", resourceID, map[string]interface{}{
		"action": action, "failure_category": category,
	}); err != nil && a != nil && a.obs != nil {
		a.log().Warn("Sensitive-action denial audit failed", zap.String("action", action), zap.Error(err))
	}
}

func (a *App) reauthenticationExpectation(ctx context.Context, action, resourceID string, state adminSecurityState) auth.ReauthenticationExpectation {
	return auth.ReauthenticationExpectation{
		Context: auth.ContextAdmin, ActorID: auth.GetUserID(ctx), SessionID: auth.GetSessionID(ctx),
		Action: action, ResourceID: resourceID, SecurityFingerprint: state.fingerprint(),
	}
}

type adminReauthenticationRequest struct {
	Password   string `json:"password"`
	Action     string `json:"action"`
	ResourceID string `json:"resource_id"`
}

func (a *App) handleAdminReauthenticate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actorID := auth.GetUserID(ctx)
	sessionID := auth.GetSessionID(ctx)
	var req adminReauthenticationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8*1024)).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request")
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.ResourceID = strings.TrimSpace(req.ResourceID)
	spec, known := sensitiveAdminActions[req.Action]
	if !known || req.Password == "" || req.ResourceID == "" || sessionID == "" || a.reauthentication == nil || a.auth == nil || a.auth.Session == nil {
		a.auditSensitiveDenial(ctx, actorID, req.Action, req.ResourceID, "reauthentication_invalid")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "reauthentication failed"})
		return
	}
	if _, err := a.auth.Session.Get(ctx, sessionID); err != nil {
		a.auditSensitiveDenial(ctx, actorID, req.Action, req.ResourceID, "session_invalid")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "reauthentication failed"})
		return
	}
	state, category, err := a.validateSensitiveActor(ctx, actorID, spec.Permission)
	if err != nil {
		a.auditSensitiveDenial(ctx, actorID, req.Action, req.ResourceID, category)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "reauthentication failed"})
		return
	}
	if err := a.auth.VerifyPassword(req.Password, state.PasswordHash); err != nil {
		_ = a.writeSecurityAudit(ctx, actorID, "admin.reauthentication.failed", "security_action", req.ResourceID,
			map[string]interface{}{"action": req.Action, "failure_category": "invalid_credentials"})
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "reauthentication failed"})
		return
	}
	grant, expiresAt, err := a.reauthentication.Issue(ctx, a.reauthenticationExpectation(ctx, req.Action, req.ResourceID, state))
	if err != nil {
		a.auditSensitiveDenial(ctx, actorID, req.Action, req.ResourceID, "storage_failure")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "reauthentication unavailable"})
		return
	}
	if err := a.writeSecurityAudit(ctx, actorID, "admin.reauthentication.succeeded", "security_action", req.ResourceID,
		map[string]interface{}{"action": req.Action}); err != nil {
		_ = a.reauthentication.RevokeActor(ctx, actorID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "reauthentication unavailable"})
		return
	}
	if err := a.writeSecurityAudit(ctx, actorID, "admin.reauthentication.grant_issued", "security_action", req.ResourceID,
		map[string]interface{}{"action": req.Action, "expires_at": expiresAt.UTC()}); err != nil {
		_ = a.reauthentication.RevokeActor(ctx, actorID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "reauthentication unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"grant": grant, "expires_at": expiresAt.UTC()})
}

func reauthenticationFailureCategory(err error) string {
	switch {
	case errors.Is(err, auth.ErrReauthenticationExpired):
		return "grant_expired"
	case errors.Is(err, auth.ErrReauthenticationReplayed):
		return "grant_replay"
	case errors.Is(err, auth.ErrReauthenticationActorBinding):
		return "wrong_actor_grant"
	case errors.Is(err, auth.ErrReauthenticationSessionBinding):
		return "wrong_session_grant"
	case errors.Is(err, auth.ErrReauthenticationActionBinding):
		return "wrong_action_grant"
	case errors.Is(err, auth.ErrReauthenticationResourceBinding):
		return "wrong_resource_grant"
	case errors.Is(err, auth.ErrReauthenticationContextBinding):
		return "wrong_context_grant"
	case errors.Is(err, auth.ErrReauthenticationStateBinding):
		return "security_state_changed"
	case errors.Is(err, auth.ErrReauthenticationBinding):
		return "grant_binding_mismatch"
	case errors.Is(err, auth.ErrReauthenticationUnavailable):
		return "storage_failure"
	default:
		return "grant_invalid"
	}
}

func (a *App) consumeSensitiveGrant(r *http.Request, action, resourceID, permission string) error {
	ctx := r.Context()
	actorID := auth.GetUserID(ctx)
	if a.reauthentication == nil || a.auth == nil || a.auth.Session == nil {
		a.auditSensitiveDenial(ctx, actorID, action, resourceID, "storage_failure")
		return errors.New("reauthentication unavailable")
	}
	if _, err := a.auth.Session.Get(ctx, auth.GetSessionID(ctx)); err != nil {
		a.auditSensitiveDenial(ctx, actorID, action, resourceID, "session_invalid")
		return errors.New("sensitive action denied")
	}
	state, category, err := a.validateSensitiveActor(ctx, actorID, permission)
	if err != nil {
		a.auditSensitiveDenial(ctx, actorID, action, resourceID, category)
		return err
	}
	grant := strings.TrimSpace(r.Header.Get(adminReauthenticationHeader))
	if grant == "" {
		a.auditSensitiveDenial(ctx, actorID, action, resourceID, "grant_missing")
		return auth.ErrReauthenticationInvalid
	}
	if err := a.reauthentication.Consume(ctx, grant, a.reauthenticationExpectation(ctx, action, resourceID, state)); err != nil {
		a.auditSensitiveDenial(ctx, actorID, action, resourceID, reauthenticationFailureCategory(err))
		return err
	}
	if err := a.writeSecurityAudit(ctx, actorID, "admin.reauthentication.grant_consumed", "security_action", resourceID,
		map[string]interface{}{"action": action}); err != nil {
		return err
	}
	return nil
}

func (a *App) requireSensitiveAction(action, permission, resourceParam string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resourceID := chi.URLParam(r, resourceParam)
			if err := a.consumeSensitiveGrant(r, action, resourceID, permission); err != nil {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "sensitive action denied"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
