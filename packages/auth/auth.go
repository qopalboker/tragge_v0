// Package auth provides authentication and authorization utilities for the tragge trading platform.
//
// Features:
//   - Password hashing with Argon2id
//   - JWT access tokens (short-lived) and refresh tokens (long-lived)
//   - Session management with Redis storage
//   - HTTP middleware for authentication and role-based authorization
//
// Example usage:
//
//	config := auth.DefaultJWTConfig("your-secret-key")
//	tokenService := auth.NewTokenService(config)
//	middleware := auth.NewMiddleware(tokenService)
//
//	// Hash password
//	hash, _ := auth.HashPassword("password123", nil)
//
//	// Generate tokens
//	pair, _ := tokenService.GenerateTokenPair("user-123", []string{"user"})
//
//	// Protect routes
//	http.Handle("/api/protected", middleware.RequireAuth(handler))
//	http.Handle("/api/admin", middleware.RequireAuth(middleware.RequireAdmin(handler)))
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Role constants for common roles.
const (
	RoleUser         = "user"
	RoleSupportAdmin = "support_admin"
	RoleSuperAdmin   = "super_admin"

	// RoleAdmin and RoleModerator are retained only so legacy source still
	// compiles while data is migrated. They are not accepted by the canonical
	// Admin access middleware.
	RoleAdmin     = "admin"
	RoleModerator = "moderator"
)

// JWT audience constants. Step 5 split the user and admin panels onto
// separate origins, each with its own signing secret and aud claim. A
// user-panel bundle that ends up talking to admin-bff â€” or vice versa â€”
// fails token validation at the middleware layer because the audiences
// don't match. Callers instantiate TokenService twice, once per audience.
const (
	AudienceUser  = "user"
	AudienceAdmin = "admin"
)

// Auth provides a unified interface for all authentication operations.
type Auth struct {
	Token      *TokenService
	Session    *SessionStore
	Middleware *Middleware
	config     *Config
}

// Config holds all authentication configuration.
type Config struct {
	// Context identifies the cryptographic trust domain. User and Admin
	// contexts must be constructed explicitly with NewContext.
	Context AuthContext

	// JWT configuration
	JWTSecret        string
	JWTRefreshSecret string
	JWTIssuer        string
	// JWTAudience is the single audience this Auth instance issues and
	// validates for. If empty, defaults to "tragge-api" for backward
	// compat (pre-split behaviour). Post-split callers pass either
	// AudienceUser or AudienceAdmin so cross-panel tokens fail at the
	// middleware layer.
	JWTAudience     string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	// Session configuration (supports standalone, sentinel, and cluster modes)
	Redis            redis.UniversalClient
	SessionPrefix    string
	RevocationPrefix string
	SessionTTL       time.Duration

	// Password configuration
	PasswordParams *Argon2idParams
}

// DefaultConfig returns a Config with sensible defaults.
// You must set JWTSecret and Redis before using.
func DefaultConfig() *Config {
	return &Config{
		JWTIssuer:       "tragge",
		AccessTokenTTL:  15 * time.Minute, // 15 minutes - short-lived for security
		RefreshTokenTTL: 7 * 24 * time.Hour,
		SessionPrefix:   "session:",
		SessionTTL:      7 * 24 * time.Hour, // 7 days - match refresh token TTL
		PasswordParams:  DefaultArgon2idParams(),
	}
}

// New creates a new Auth instance with the given configuration.
func New(config *Config) *Auth {
	audience := []string{"tragge-api"}
	if config.JWTAudience != "" {
		audience = []string{config.JWTAudience}
	}
	jwtConfig := &JWTConfig{
		Secret:          []byte(config.JWTSecret),
		RefreshSecret:   []byte(config.JWTRefreshSecret),
		Issuer:          config.JWTIssuer,
		Audience:        audience,
		Context:         config.Context,
		AccessTokenTTL:  config.AccessTokenTTL,
		RefreshTokenTTL: config.RefreshTokenTTL,
	}

	tokenService := NewTokenService(jwtConfig)

	var sessionStore *SessionStore
	var mw *Middleware
	if config.Redis != nil {
		sessionStore = NewSessionStore(&SessionStoreConfig{
			Redis:     config.Redis,
			KeyPrefix: config.SessionPrefix,
			TTL:       config.SessionTTL,
		})
		mw = NewMiddlewareWithSession(tokenService, sessionStore)
	} else {
		mw = NewMiddleware(tokenService)
	}

	return &Auth{
		Token:      tokenService,
		Session:    sessionStore,
		Middleware: mw,
		config:     config,
	}
}

// Context returns the explicit authentication trust domain. Legacy Auth
// instances constructed with New have an empty context.
func (a *Auth) Context() AuthContext {
	if a == nil || a.config == nil {
		return ""
	}
	return a.config.Context
}

// HashPassword hashes a password using the configured Argon2id parameters.
func (a *Auth) HashPassword(password string) (string, error) {
	return HashPassword(password, a.config.PasswordParams)
}

// VerifyPassword verifies a password against an Argon2id hash.
func (a *Auth) VerifyPassword(password, hash string) error {
	return VerifyPassword(password, hash)
}

// Login authenticates a user and returns a token pair and session ID.
// The caller is responsible for verifying the password before calling this.
func (a *Auth) Login(ctx context.Context, userID string, roles []string, deviceInfo, ipAddress string) (*TokenPair, string, error) {
	return a.login(ctx, userID, roles, nil, "", deviceInfo, ipAddress)
}

// LoginWithPermissions creates a session and returns tokens with permissions embedded.
func (a *Auth) LoginWithPermissions(ctx context.Context, userID string, roles []string, permissions []string, deviceInfo, ipAddress string) (*TokenPair, string, error) {
	return a.login(ctx, userID, roles, permissions, "", deviceInfo, ipAddress)
}

// LoginWithPermissionsAndMFA creates an MFA-bound session. Callers must finish
// authoritative MFA verification before invoking it.
func (a *Auth) LoginWithPermissionsAndMFA(ctx context.Context, userID string, roles []string, permissions []string, assurance MFAAssurance, deviceInfo, ipAddress string) (*TokenPair, string, error) {
	if assurance != MFAAssuranceSuperAdminTOTPV1 {
		return nil, "", ErrInvalidToken
	}
	return a.login(ctx, userID, roles, permissions, assurance, deviceInfo, ipAddress)
}

// login is the shared implementation for Login and LoginWithPermissions.
func (a *Auth) login(ctx context.Context, userID string, roles []string, permissions []string, assurance MFAAssurance, deviceInfo, ipAddress string) (*TokenPair, string, error) {
	if a.Session == nil {
		// No session management, just generate tokens
		if assurance != "" {
			pair, err := a.Token.GenerateTokenPairWithSessionPermissionsAndMFA(userID, roles, permissions, "", assurance)
			return pair, "", err
		}
		if len(permissions) > 0 {
			pair, err := a.Token.GenerateTokenPairWithPermissions(userID, roles, permissions)
			return pair, "", err
		}
		pair, err := a.Token.GenerateTokenPair(userID, roles)
		return pair, "", err
	}

	// Create session first to get session ID
	sessionID, err := a.Session.Create(ctx, &Session{
		UserID:       userID,
		Roles:        roles,
		Permissions:  permissions,
		MFAAssurance: assurance,
		DeviceInfo:   deviceInfo,
		IPAddress:    ipAddress,
	})
	if err != nil {
		return nil, "", err
	}

	// Generate tokens with session ID embedded
	var pair *TokenPair
	if len(permissions) > 0 || assurance != "" {
		pair, err = a.Token.GenerateTokenPairWithSessionPermissionsAndMFA(userID, roles, permissions, sessionID, assurance)
	} else {
		pair, err = a.Token.GenerateTokenPairWithSession(userID, roles, sessionID)
	}
	if err != nil {
		// Cleanup session if token generation fails
		a.Session.Delete(ctx, sessionID)
		return nil, "", err
	}

	// Store refresh token in session
	if err := a.Session.Refresh(ctx, sessionID, pair.RefreshToken); err != nil {
		a.Session.Delete(ctx, sessionID)
		return nil, "", fmt.Errorf("failed to store refresh token in session: %w", err)
	}

	return pair, sessionID, nil
}

// Refresh validates a refresh token and generates a new token pair.
func (a *Auth) Refresh(ctx context.Context, sessionID, refreshToken string) (*TokenPair, error) {
	if a.Session == nil {
		// Without session store, just validate the refresh token
		claims, err := a.Token.ValidateRefreshToken(refreshToken)
		if err != nil {
			return nil, err
		}
		return a.Token.GenerateTokenPair(claims.UserID, claims.Roles)
	}

	// Validate the cryptographic refresh context before touching session state.
	// A cross-context token must not trigger reuse detection in the valid
	// session belonging to the other trust domain.
	claims, err := a.Token.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}
	if sessionID == "" || claims.SessionID != sessionID {
		return nil, ErrInvalidToken
	}

	// Validate the namespaced session and refresh-token rotation state.
	session, err := a.Session.ValidateRefreshToken(ctx, sessionID, refreshToken)
	if err != nil {
		return nil, err
	}
	if claims.MFAAssurance != session.MFAAssurance {
		return nil, ErrInvalidToken
	}

	// Generate new token pair with session ID
	pair, err := a.Token.GenerateTokenPairWithSessionPermissionsAndMFA(session.UserID, session.Roles, session.Permissions, sessionID, session.MFAAssurance)
	if err != nil {
		return nil, err
	}

	// Update session with new refresh token
	if err := a.Session.Refresh(ctx, sessionID, pair.RefreshToken); err != nil {
		return nil, err
	}

	return pair, nil
}

// Logout invalidates a session.
func (a *Auth) Logout(ctx context.Context, sessionID string) error {
	if a.Session == nil {
		return nil
	}
	return a.Session.Delete(ctx, sessionID)
}

// LogoutAll invalidates all sessions for a user.
func (a *Auth) LogoutAll(ctx context.Context, userID string) error {
	if a.Session == nil {
		return nil
	}
	return a.Session.DeleteAllForUser(ctx, userID)
}
