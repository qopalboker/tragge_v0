package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	// UserIDKey is the context key for the authenticated user ID.
	UserIDKey contextKey = "user_id"
	// RolesKey is the context key for the authenticated user's roles.
	RolesKey contextKey = "roles"
	// PermissionsKey is the context key for the authenticated user's permissions.
	PermissionsKey contextKey = "permissions"
	// ClaimsKey is the context key for the full JWT claims.
	ClaimsKey contextKey = "claims"
	// SessionIDKey is the context key for the session ID.
	SessionIDKey contextKey = "session_id"
)

// PasswordChangedAtFunc is a function that returns the password_changed_at timestamp for a user.
// Used as a DB-backed fallback for session invalidation when Redis is unavailable (P0-5).
type PasswordChangedAtFunc func(ctx context.Context, userID string) (*time.Time, error)

// Middleware provides HTTP middleware for authentication and authorization.
type Middleware struct {
	tokenService        *TokenService
	sessionStore        *SessionStore
	tokenBlacklist      *TokenBlacklist
	passwordChangedAtFn PasswordChangedAtFunc
	logger              *log.Logger
}

// NewMiddleware creates a new authentication middleware.
func NewMiddleware(tokenService *TokenService) *Middleware {
	return &Middleware{
		tokenService: tokenService,
		sessionStore: nil,
	}
}

// NewMiddlewareWithSession creates a new authentication middleware with session validation.
func NewMiddlewareWithSession(tokenService *TokenService, sessionStore *SessionStore) *Middleware {
	return &Middleware{
		tokenService: tokenService,
		sessionStore: sessionStore,
	}
}

// SetTokenBlacklist sets the token blacklist for checking revoked tokens.
func (m *Middleware) SetTokenBlacklist(blacklist *TokenBlacklist) {
	m.tokenBlacklist = blacklist
}

// SetPasswordChangedAtFunc sets the function to check password_changed_at from the database.
// This provides a fallback for session invalidation when Redis is unavailable (P0-5).
func (m *Middleware) SetPasswordChangedAtFunc(fn PasswordChangedAtFunc) {
	m.passwordChangedAtFn = fn
}

// SetLogger sets an optional logger for the middleware.
// When set, errors from passwordChangedAtFn will be logged instead of silently ignored.
func (m *Middleware) SetLogger(logger *log.Logger) {
	m.logger = logger
}

// RequireAuth returns a middleware that requires a valid access token.
// It extracts the Bearer token from the Authorization header, validates it,
// and sets the user_id and roles in the request context.
// If session management is enabled, it also validates the session and updates last_seen_at.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if HasProhibitedCredentialQuery(r) {
			writeURLAuthUnsupported(w)
			return
		}

		token, err := extractBearerToken(r)
		if err != nil {
			writeUnauthorized(w, "missing or invalid authorization header")
			return
		}

		claims, err := m.tokenService.ValidateAccessToken(token)
		if err != nil {
			if errors.Is(err, ErrExpiredToken) {
				writeUnauthorized(w, "token has expired")
				return
			}
			writeUnauthorized(w, "invalid token")
			return
		}

		// Check JWT blacklist (for tokens revoked by logout)
		if m.tokenBlacklist != nil && claims.ID != "" {
			if m.tokenBlacklist.IsBlacklisted(r.Context(), claims.ID) {
				writeUnauthorized(w, "token has been revoked")
				return
			}
		}

		// P0-5: Check password_changed_at as fallback for session invalidation
		if m.passwordChangedAtFn != nil && claims.IssuedAt != nil {
			pwChangedAt, err := m.passwordChangedAtFn(r.Context(), claims.UserID)
			if err != nil {
				if m.logger != nil {
					m.logger.Printf("auth: passwordChangedAtFn error for user %s: %v", claims.UserID, err)
				}
			} else if pwChangedAt != nil && claims.IssuedAt.Time.Before(*pwChangedAt) {
				writeUnauthorized(w, "token invalidated by password change")
				return
			}
		}

		// If session management is enabled, validate session
		if m.sessionStore != nil && claims.SessionID != "" {
			ctx := r.Context()

			// Check if session exists
			session, err := m.sessionStore.Get(ctx, claims.SessionID)
			if err != nil {
				if errors.Is(err, ErrSessionNotFound) {
					writeUnauthorized(w, "session not found or expired")
					return
				}
				writeUnauthorized(w, "session validation failed")
				return
			}
			if session.UserID != claims.UserID || session.MFAAssurance != claims.MFAAssurance {
				writeUnauthorized(w, "session assurance mismatch")
				return
			}

			// Update last_seen_at timestamp (log errors but don't block request)
			if err := m.sessionStore.UpdateLastSeen(ctx, claims.SessionID); err != nil {
				if m.logger != nil {
					m.logger.Printf("auth: UpdateLastSeen error for session %s: %v", claims.SessionID, err)
				}
			}
		}

		// Set context values
		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, RolesKey, claims.Roles)
		ctx = context.WithValue(ctx, PermissionsKey, claims.Permissions)
		ctx = context.WithValue(ctx, ClaimsKey, claims)
		if claims.SessionID != "" {
			ctx = context.WithValue(ctx, SessionIDKey, claims.SessionID)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireSuperAdminMFA requires the versioned Admin MFA assurance for every
// Super Admin token. Support Admin sessions remain governed by their explicit
// permissions and do not acquire Super Admin authority through this check.
func (m *Middleware) RequireSuperAdminMFA(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := GetClaims(r.Context())
		if claims == nil {
			writeUnauthorized(w, "authentication required")
			return
		}
		if claims.HasRole(RoleSuperAdmin) && claims.MFAAssurance != MFAAssuranceSuperAdminTOTPV1 {
			writeUnauthorized(w, "additional authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuthFunc is a function-based version of RequireAuth for middleware chaining.
func (m *Middleware) RequireAuthFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireAuth(next).ServeHTTP
}

// RequireRole returns a middleware that requires the user to have one of the specified roles.
// This should be used after RequireAuth.
func (m *Middleware) RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				writeForbidden(w, "authentication required")
				return
			}

			if !claims.HasAnyRole(roles...) {
				writeForbidden(w, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRoleFunc is a function-based version of RequireRole.
func (m *Middleware) RequireRoleFunc(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	roleMiddleware := m.RequireRole(roles...)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return roleMiddleware(next).ServeHTTP
	}
}

// RequireAdmin returns a middleware that requires admin role.
func (m *Middleware) RequireAdmin(next http.Handler) http.Handler {
	return m.RequireRole("admin")(next)
}

// RequireAdminFunc is a function-based version of RequireAdmin.
func (m *Middleware) RequireAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireAdmin(next).ServeHTTP
}

// RequireModerator returns a middleware that requires admin or moderator role.
func (m *Middleware) RequireModerator(next http.Handler) http.Handler {
	return m.RequireRole("admin", "moderator")(next)
}

// RequireModeratorFunc is a function-based version of RequireModerator.
func (m *Middleware) RequireModeratorFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireModerator(next).ServeHTTP
}

// RequireSuperAdmin returns a middleware that requires super_admin role.
func (m *Middleware) RequireSuperAdmin(next http.Handler) http.Handler {
	return m.RequireRole(RoleSuperAdmin)(next)
}

// RequireSuperAdminFunc is a function-based version of RequireSuperAdmin.
func (m *Middleware) RequireSuperAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireSuperAdmin(next).ServeHTTP
}

// RequireAdminAccess requires a canonical Admin-panel role. Deprecated
// elevated roles fail closed and must be migrated explicitly.
func (m *Middleware) RequireAdminAccess(next http.Handler) http.Handler {
	return m.RequireRole(RoleSupportAdmin, RoleSuperAdmin)(m.RequireSuperAdminMFA(next))
}

// RequireAdminAccessFunc is a function-based version of RequireAdminAccess.
func (m *Middleware) RequireAdminAccessFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireAdminAccess(next).ServeHTTP
}

// RequirePermission returns a middleware that requires the user to have one of the specified permissions.
// This should be used after RequireAuth.
func (m *Middleware) RequirePermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				writeForbidden(w, "authentication required")
				return
			}

			if !claims.IsSuperAdmin() && !claims.HasAnyPermission(permissions...) {
				writeForbidden(w, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermissionFunc is a function-based version of RequirePermission.
func (m *Middleware) RequirePermissionFunc(permissions ...string) func(http.HandlerFunc) http.HandlerFunc {
	permMiddleware := m.RequirePermission(permissions...)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return permMiddleware(next).ServeHTTP
	}
}

// RequireAllPermissions returns a middleware that requires the user to have all of the specified permissions.
// This should be used after RequireAuth.
func (m *Middleware) RequireAllPermissions(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				writeForbidden(w, "authentication required")
				return
			}

			if !claims.HasAllPermissions(permissions...) {
				writeForbidden(w, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalAuth returns a middleware that extracts auth info if present but doesn't require it.
// Useful for endpoints that behave differently for authenticated vs anonymous users.
func (m *Middleware) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := extractBearerToken(r)
		if err != nil {
			// No token provided, continue without auth context
			next.ServeHTTP(w, r)
			return
		}

		claims, err := m.tokenService.ValidateAccessToken(token)
		if err != nil {
			// Invalid token, continue without auth context
			next.ServeHTTP(w, r)
			return
		}

		// Set context values
		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, RolesKey, claims.Roles)
		ctx = context.WithValue(ctx, PermissionsKey, claims.Permissions)
		ctx = context.WithValue(ctx, ClaimsKey, claims)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID extracts the user ID from the context.
// Returns empty string if not authenticated.
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// GetRoles extracts the roles from the context.
// Returns nil if not authenticated.
func GetRoles(ctx context.Context) []string {
	if roles, ok := ctx.Value(RolesKey).([]string); ok {
		return roles
	}
	return nil
}

// GetClaims extracts the full claims from the context.
// Returns nil if not authenticated.
func GetClaims(ctx context.Context) *Claims {
	if claims, ok := ctx.Value(ClaimsKey).(*Claims); ok {
		return claims
	}
	return nil
}

// GetSessionID extracts the session ID from the context.
// Returns empty string if no session.
func GetSessionID(ctx context.Context) string {
	if sessionID, ok := ctx.Value(SessionIDKey).(string); ok {
		return sessionID
	}
	return ""
}

// IsAuthenticated checks if the request context has valid authentication.
func IsAuthenticated(ctx context.Context) bool {
	return GetUserID(ctx) != ""
}

// HasRole checks if the authenticated user has a specific role.
func HasRole(ctx context.Context, role string) bool {
	claims := GetClaims(ctx)
	if claims == nil {
		return false
	}
	return claims.HasRole(role)
}

// IsAdmin checks if the authenticated user is an admin.
func IsAdmin(ctx context.Context) bool {
	return HasRole(ctx, "admin")
}

// IsModerator checks if the authenticated user is a moderator (or admin).
func IsModerator(ctx context.Context) bool {
	return HasRole(ctx, "admin") || HasRole(ctx, "moderator")
}

// IsSuperAdmin checks if the authenticated user is a super admin.
func IsSuperAdmin(ctx context.Context) bool {
	return HasRole(ctx, RoleSuperAdmin)
}

// IsViewer checks if the authenticated user is a viewer.
func IsViewer(ctx context.Context) bool {
	return HasRole(ctx, "viewer")
}

// HasAdminAccess checks if the authenticated user has admin panel access.
func HasAdminAccess(ctx context.Context) bool {
	return HasRole(ctx, RoleSupportAdmin) || HasRole(ctx, RoleSuperAdmin)
}

// GetPermissions extracts the permissions from the context.
// Returns nil if not authenticated.
func GetPermissions(ctx context.Context) []string {
	if permissions, ok := ctx.Value(PermissionsKey).([]string); ok {
		return permissions
	}
	return nil
}

// HasPermission checks if the authenticated user has a specific permission.
func HasPermission(ctx context.Context, permission string) bool {
	claims := GetClaims(ctx)
	if claims == nil {
		return false
	}
	return claims.HasPermission(permission)
}

// HasAnyPermission checks if the authenticated user has any of the specified permissions.
func HasAnyPermission(ctx context.Context, permissions ...string) bool {
	claims := GetClaims(ctx)
	if claims == nil {
		return false
	}
	return claims.HasAnyPermission(permissions...)
}

// HasAllPermissions checks if the authenticated user has all of the specified permissions.
func HasAllPermissions(ctx context.Context, permissions ...string) bool {
	claims := GetClaims(ctx)
	if claims == nil {
		return false
	}
	return claims.HasAllPermissions(permissions...)
}

var prohibitedCredentialQueryNames = map[string]struct{}{
	"token":         {},
	"access_token":  {},
	"jwt":           {},
	"auth_token":    {},
	"session_token": {},
}

// HasProhibitedCredentialQuery detects reusable session-credential aliases in
// the URL. Presence fails closed even when a secure credential is also sent so
// callers cannot accidentally leak a credential through access logs, tracing,
// browser history, or referrers.
func HasProhibitedCredentialQuery(r *http.Request) bool {
	for name := range r.URL.Query() {
		if _, prohibited := prohibitedCredentialQueryNames[strings.ToLower(name)]; prohibited {
			return true
		}
	}
	return false
}

// RedactedCredentialValue is the non-sensitive placeholder used before a
// request enters logging, tracing, analytics, or panic-reporting middleware.
const RedactedCredentialValue = "[REDACTED]"

type securityCredentialSnapshotKey struct{}

type securityCredentialSnapshot struct {
	authorization string
	cookie        string
}

// RedactSecurityCredentialsForTelemetry removes secure header values and
// replaces prohibited query credential values before observability or panic
// telemetry sees the request. RestoreSecurityCredentialsAfterTelemetry must be
// installed after telemetry and before authentication handlers.
func RedactSecurityCredentialsForTelemetry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snapshot := securityCredentialSnapshot{
			authorization: r.Header.Get("Authorization"),
			cookie:        r.Header.Get("Cookie"),
		}
		clone := r.Clone(context.WithValue(r.Context(), securityCredentialSnapshotKey{}, snapshot))
		clone.Header = r.Header.Clone()
		clone.Header.Del("Authorization")
		clone.Header.Del("Cookie")
		clone.URL = cloneURL(r)
		query := clone.URL.Query()
		for name := range query {
			if _, prohibited := prohibitedCredentialQueryNames[strings.ToLower(name)]; prohibited {
				query[name] = []string{RedactedCredentialValue}
			}
		}
		clone.URL.RawQuery = query.Encode()
		next.ServeHTTP(w, clone)
	})
}

// RestoreSecurityCredentialsAfterTelemetry restores only the secure headers
// captured by RedactSecurityCredentialsForTelemetry. Query credentials remain
// redacted and are rejected by RequireAuth based on their field names.
func RestoreSecurityCredentialsAfterTelemetry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snapshot, _ := r.Context().Value(securityCredentialSnapshotKey{}).(securityCredentialSnapshot)
		clone := r.Clone(r.Context())
		clone.Header = r.Header.Clone()
		if snapshot.authorization != "" {
			clone.Header.Set("Authorization", snapshot.authorization)
		}
		if snapshot.cookie != "" {
			clone.Header.Set("Cookie", snapshot.cookie)
		}
		next.ServeHTTP(w, clone)
	})
}

func cloneURL(r *http.Request) *url.URL {
	if r.URL == nil {
		return &url.URL{}
	}
	copy := *r.URL
	return &copy
}

// extractBearerToken extracts a token only from the Authorization header.
func extractBearerToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token := strings.TrimSpace(parts[1])
			if token != "" {
				return token, nil
			}
		}
	}

	if authHeader == "" {
		return "", errors.New("missing authorization header")
	}
	return "", errors.New("invalid authorization header format")
}

// errorResponse is used for JSON error responses.
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// writeUnauthorized writes a 401 response.
func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: message})
}

func writeURLAuthUnsupported(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(errorResponse{
		Error: "URL authentication is not supported",
		Code:  "url_authentication_unsupported",
	})
}

// writeForbidden writes a 403 response.
func writeForbidden(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	json.NewEncoder(w).Encode(errorResponse{Error: message})
}
