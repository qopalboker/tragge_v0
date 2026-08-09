package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenType distinguishes between access and refresh tokens.
type TokenType string

// MFAAssurance identifies a server-issued authentication assurance level.
// It is cryptographically signed in JWTs and mirrored in the durable session;
// clients cannot opt into an assurance level with a request header.
type MFAAssurance string

const (
	// AccessToken is a short-lived token for API access.
	AccessToken TokenType = "access"
	// RefreshToken is a long-lived token for obtaining new access tokens.
	RefreshToken TokenType = "refresh"
	// MFAAssuranceSuperAdminTOTPV1 is the first Admin-only MFA contract.
	MFAAssuranceSuperAdminTOTPV1 MFAAssurance = "super_admin_totp_v1"
)

// Claims represents the JWT claims for authentication tokens.
type Claims struct {
	jwt.RegisteredClaims
	UserID       string       `json:"user_id"`
	Roles        []string     `json:"roles"`
	Permissions  []string     `json:"permissions,omitempty"`
	TokenType    TokenType    `json:"token_type"`
	AuthContext  AuthContext  `json:"auth_context,omitempty"`
	SessionID    string       `json:"session_id,omitempty"`
	MFAAssurance MFAAssurance `json:"mfa_assurance,omitempty"`
}

// JWTConfig holds configuration for JWT token generation.
type JWTConfig struct {
	// Secret is the signing key for HS256 access tokens.
	Secret []byte
	// RefreshSecret is the signing key for HS256 refresh tokens.
	// If empty, falls back to Secret for backward compatibility.
	RefreshSecret []byte
	// Issuer is the token issuer (iss claim).
	Issuer string
	// Audience is the list of intended token recipients (aud claim).
	Audience []string
	// Context is the cryptographic trust domain expected in auth_context.
	Context AuthContext
	// AccessTokenTTL is the duration for access tokens. Default: 15 minutes.
	AccessTokenTTL time.Duration
	// RefreshTokenTTL is the duration for refresh tokens. Default: 7 days.
	RefreshTokenTTL time.Duration
}

// refreshSecret returns the secret used to sign/verify refresh tokens.
// Falls back to Secret if RefreshSecret is not configured.
func (c *JWTConfig) refreshSecret() []byte {
	if len(c.RefreshSecret) > 0 {
		return c.RefreshSecret
	}
	return c.Secret
}

// DefaultJWTConfig returns a JWTConfig with default TTL values.
// You must set the Secret before using.
func DefaultJWTConfig(secret string) *JWTConfig {
	return &JWTConfig{
		Secret:          []byte(secret),
		Issuer:          "tragge",
		Audience:        []string{"tragge-api"},
		AccessTokenTTL:  15 * time.Minute, // 15 minutes - short-lived for security
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

var (
	// ErrInvalidToken indicates the token is malformed or signature is invalid.
	ErrInvalidToken = errors.New("auth: invalid token")
	// ErrExpiredToken indicates the token has expired.
	ErrExpiredToken = errors.New("auth: token has expired")
	// ErrInvalidTokenType indicates an unexpected token type.
	ErrInvalidTokenType = errors.New("auth: invalid token type")
	// ErrMissingClaims indicates required claims are missing.
	ErrMissingClaims = errors.New("auth: missing required claims")
)

// TokenService handles JWT token operations.
type TokenService struct {
	config *JWTConfig
}

// NewTokenService creates a new TokenService with the given configuration.
func NewTokenService(config *JWTConfig) *TokenService {
	return &TokenService{config: config}
}

// TokenPair represents a pair of access and refresh tokens.
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	SessionID    string    `json:"session_id,omitempty"`
}

// GenerateTokenPair creates both access and refresh tokens for a user.
func (s *TokenService) GenerateTokenPair(userID string, roles []string) (*TokenPair, error) {
	return s.GenerateTokenPairWithSessionAndPermissions(userID, roles, nil, "")
}

// GenerateTokenPairWithPermissions creates tokens with permissions included.
func (s *TokenService) GenerateTokenPairWithPermissions(userID string, roles []string, permissions []string) (*TokenPair, error) {
	return s.GenerateTokenPairWithSessionAndPermissions(userID, roles, permissions, "")
}

// GenerateTokenPairWithSession creates both access and refresh tokens with session ID.
func (s *TokenService) GenerateTokenPairWithSession(userID string, roles []string, sessionID string) (*TokenPair, error) {
	return s.GenerateTokenPairWithSessionAndPermissions(userID, roles, nil, sessionID)
}

// GenerateTokenPairWithSessionAndPermissions creates tokens with session ID and permissions.
func (s *TokenService) GenerateTokenPairWithSessionAndPermissions(userID string, roles []string, permissions []string, sessionID string) (*TokenPair, error) {
	return s.GenerateTokenPairWithSessionPermissionsAndMFA(userID, roles, permissions, sessionID, "")
}

// GenerateTokenPairWithSessionPermissionsAndMFA creates a pair whose MFA
// assurance is bound to the same server-side session.
func (s *TokenService) GenerateTokenPairWithSessionPermissionsAndMFA(userID string, roles []string, permissions []string, sessionID string, assurance MFAAssurance) (*TokenPair, error) {
	now := time.Now()

	accessToken, err := s.generateToken(userID, roles, permissions, AccessToken, sessionID, assurance, now)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.generateToken(userID, roles, permissions, RefreshToken, sessionID, assurance, now)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    now.Add(s.config.AccessTokenTTL),
		SessionID:    sessionID,
	}, nil
}

// GenerateAccessToken creates a new access token for a user.
func (s *TokenService) GenerateAccessToken(userID string, roles []string) (string, error) {
	return s.generateToken(userID, roles, nil, AccessToken, "", "", time.Now())
}

// GenerateRefreshToken creates a new refresh token for a user.
func (s *TokenService) GenerateRefreshToken(userID string, roles []string) (string, error) {
	return s.generateToken(userID, roles, nil, RefreshToken, "", "", time.Now())
}

// generateJTI creates a cryptographically secure unique JWT ID.
func generateJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *TokenService) generateToken(userID string, roles []string, permissions []string, tokenType TokenType, sessionID string, assurance MFAAssurance, now time.Time) (string, error) {
	var ttl time.Duration
	if tokenType == AccessToken {
		ttl = s.config.AccessTokenTTL
	} else {
		ttl = s.config.RefreshTokenTTL
	}

	jti, err := generateJTI()
	if err != nil {
		return "", fmt.Errorf("failed to generate JTI: %w", err)
	}

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Issuer:    s.config.Issuer,
			Subject:   userID,
			Audience:  jwt.ClaimStrings(s.config.Audience),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
		UserID:       userID,
		Roles:        roles,
		Permissions:  permissions,
		TokenType:    tokenType,
		AuthContext:  s.config.Context,
		SessionID:    sessionID,
		MFAAssurance: assurance,
	}

	// TODO: Consider migrating from HS256 to RS256 for asymmetric verification.
	// RS256 allows services to verify tokens with only a public key,
	// reducing blast radius if a single service is compromised.
	var secret []byte
	if tokenType == AccessToken {
		secret = s.config.Secret
	} else {
		secret = s.config.refreshSecret()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// validateTokenWithSecret parses and validates a JWT token string using the given secret.
func (s *TokenService) validateTokenWithSecret(tokenString string, secret []byte) (*Claims, error) {
	parserOpts := []jwt.ParserOption{jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()})}
	if len(s.config.Audience) > 0 {
		parserOpts = append(parserOpts, jwt.WithAudience(s.config.Audience[0]))
	}
	if s.config.Issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(s.config.Issuer))
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secret, nil
	}, parserOpts...)

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	if claims.UserID == "" {
		return nil, ErrMissingClaims
	}
	if len(s.config.Audience) > 0 {
		if len(claims.Audience) != 1 || claims.Audience[0] != s.config.Audience[0] {
			return nil, ErrInvalidToken
		}
	}
	if s.config.Context != "" && claims.AuthContext != s.config.Context {
		return nil, ErrInvalidToken
	}
	if claims.MFAAssurance != "" && claims.MFAAssurance != MFAAssuranceSuperAdminTOTPV1 {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// ValidateToken parses and validates a JWT token string using the access token secret.
// Returns the claims if valid, or an error if invalid/expired.
func (s *TokenService) ValidateToken(tokenString string) (*Claims, error) {
	return s.validateTokenWithSecret(tokenString, s.config.Secret)
}

// ValidateAccessToken validates a token and ensures it's an access token.
func (s *TokenService) ValidateAccessToken(tokenString string) (*Claims, error) {
	claims, err := s.validateTokenWithSecret(tokenString, s.config.Secret)
	if err != nil {
		return nil, err
	}

	if claims.TokenType != AccessToken {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// ValidateRefreshToken validates a token and ensures it's a refresh token.
func (s *TokenService) ValidateRefreshToken(tokenString string) (*Claims, error) {
	claims, err := s.validateTokenWithSecret(tokenString, s.config.refreshSecret())
	if err != nil {
		return nil, err
	}

	if claims.TokenType != RefreshToken {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// HasRole checks if the claims contain a specific role.
func (c *Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the claims contain any of the specified roles.
func (c *Claims) HasAnyRole(roles ...string) bool {
	for _, role := range roles {
		if c.HasRole(role) {
			return true
		}
	}
	return false
}

// IsAdmin checks if the claims indicate admin access.
func (c *Claims) IsAdmin() bool {
	return c.HasRole("admin")
}

// IsModerator checks if the claims indicate moderator access.
func (c *Claims) IsModerator() bool {
	return c.HasRole("moderator")
}

// IsSuperAdmin checks if the claims indicate super admin access.
func (c *Claims) IsSuperAdmin() bool {
	return c.HasRole("super_admin")
}

// IsViewer checks if the claims indicate viewer access.
func (c *Claims) IsViewer() bool {
	return c.HasRole("viewer")
}

// HasPermission checks if the claims contain a specific permission.
func (c *Claims) HasPermission(permission string) bool {
	for _, p := range c.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// HasAnyPermission checks if the claims contain any of the specified permissions.
func (c *Claims) HasAnyPermission(permissions ...string) bool {
	for _, permission := range permissions {
		if c.HasPermission(permission) {
			return true
		}
	}
	return false
}

// HasAllPermissions checks if the claims contain all of the specified permissions.
func (c *Claims) HasAllPermissions(permissions ...string) bool {
	for _, permission := range permissions {
		if !c.HasPermission(permission) {
			return false
		}
	}
	return true
}

// GetPermissions returns the list of permissions.
func (c *Claims) GetPermissions() []string {
	return c.Permissions
}
