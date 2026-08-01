package auth

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/redis/go-redis/v9"
)

// AuthContext is a cryptographic and operational authentication trust domain.
type AuthContext string

const (
	ContextUser  AuthContext = "user"
	ContextAdmin AuthContext = "admin"

	IssuerUser  = "tragge-user-auth"
	IssuerAdmin = "tragge-admin-auth"

	UserSessionPrefix  = "session:user:"
	AdminSessionPrefix = "session:admin:"

	UserRevocationPrefix  = "jwt_blacklist:user:"
	AdminRevocationPrefix = "jwt_blacklist:admin:"

	UserRefreshCookieName      = "refresh_token_user"
	AdminRefreshCookieName     = "refresh_token_admin"
	UserSessionHintCookieName  = "tragge_session_hint_user"
	AdminSessionHintCookieName = "tragge_session_hint_admin"

	UserRefreshCookiePath  = "/api/user/auth"
	AdminRefreshCookiePath = "/api/admin/auth"

	UserCSRFContext  = "csrf:user"
	AdminCSRFContext = "csrf:admin"
)

var (
	ErrInvalidAuthContext   = errors.New("auth: invalid authentication context")
	ErrInvalidAuthIsolation = errors.New("auth: invalid user/admin isolation configuration")
)

// ContextConfig contains one trust domain's complete authentication boundary.
// Secret values must never be logged or included in returned errors.
type ContextConfig struct {
	Context               AuthContext
	AccessSecret          string
	RefreshSecret         string
	Issuer                string
	Audience              string
	SessionPrefix         string
	RevocationPrefix      string
	RefreshCookieName     string
	SessionHintCookieName string
	RefreshCookiePath     string
	CSRFContext           string
	CSRFOrigin            string
}

// IsolationConfig contains both User and Admin trust domains so startup can
// reject cross-domain collisions before either validator is constructed.
type IsolationConfig struct {
	Environment string
	User        ContextConfig
	Admin       ContextConfig
}

// LoadIsolationConfig reads the canonical auth environment variables. The
// secret source should support the repository's *_FILE convention.
func LoadIsolationConfig(environment string, getenv, loadSecret func(string) string) IsolationConfig {
	userIssuer := strings.TrimSpace(getenv("JWT_ISSUER_USER"))
	adminIssuer := strings.TrimSpace(getenv("JWT_ISSUER_ADMIN"))
	userAudience := strings.TrimSpace(getenv("JWT_AUDIENCE_USER"))
	adminAudience := strings.TrimSpace(getenv("JWT_AUDIENCE_ADMIN"))

	if !isProductionEnvironment(environment) {
		if userIssuer == "" {
			userIssuer = IssuerUser
		}
		if adminIssuer == "" {
			adminIssuer = IssuerAdmin
		}
		if userAudience == "" {
			userAudience = AudienceUser
		}
		if adminAudience == "" {
			adminAudience = AudienceAdmin
		}
	}

	return IsolationConfig{
		Environment: environment,
		User: ContextConfig{
			Context:               ContextUser,
			AccessSecret:          loadSecret("JWT_SECRET_USER"),
			RefreshSecret:         loadSecret("JWT_REFRESH_SECRET_USER"),
			Issuer:                userIssuer,
			Audience:              userAudience,
			SessionPrefix:         UserSessionPrefix,
			RevocationPrefix:      UserRevocationPrefix,
			RefreshCookieName:     UserRefreshCookieName,
			SessionHintCookieName: UserSessionHintCookieName,
			RefreshCookiePath:     UserRefreshCookiePath,
			CSRFContext:           UserCSRFContext,
			CSRFOrigin:            strings.TrimSpace(getenv("USER_FRONTEND_ORIGIN")),
		},
		Admin: ContextConfig{
			Context:               ContextAdmin,
			AccessSecret:          loadSecret("JWT_SECRET_ADMIN"),
			RefreshSecret:         loadSecret("JWT_REFRESH_SECRET_ADMIN"),
			Issuer:                adminIssuer,
			Audience:              adminAudience,
			SessionPrefix:         AdminSessionPrefix,
			RevocationPrefix:      AdminRevocationPrefix,
			RefreshCookieName:     AdminRefreshCookieName,
			SessionHintCookieName: AdminSessionHintCookieName,
			RefreshCookiePath:     AdminRefreshCookiePath,
			CSRFContext:           AdminCSRFContext,
			CSRFOrigin:            strings.TrimSpace(getenv("ADMIN_FRONTEND_ORIGIN")),
		},
	}
}

// Validate verifies one trust domain, including production secret strength.
func (c ContextConfig) Validate(environment string) error {
	if c.Context != ContextUser && c.Context != ContextAdmin {
		return ErrInvalidAuthContext
	}
	if err := validateContextShape(c, c.Context); err != nil {
		return err
	}
	if isProductionEnvironment(environment) {
		if strings.TrimSpace(c.CSRFOrigin) == "" {
			return isolationError("%s CSRF origin is required", c.Context)
		}
		if err := validateProductionSecret(fmt.Sprintf("%s access secret", c.Context), c.AccessSecret); err != nil {
			return err
		}
		if err := validateProductionSecret(fmt.Sprintf("%s refresh secret", c.Context), c.RefreshSecret); err != nil {
			return err
		}
	}
	return nil
}

// Validate verifies both trust domains before runtime construction.
func (c IsolationConfig) Validate() error {
	if err := c.User.Validate(c.Environment); err != nil {
		return err
	}
	if err := c.Admin.Validate(c.Environment); err != nil {
		return err
	}

	collisions := []struct {
		name  string
		left  string
		right string
	}{
		{"access secrets", c.User.AccessSecret, c.Admin.AccessSecret},
		{"refresh secrets", c.User.RefreshSecret, c.Admin.RefreshSecret},
		{"issuers", c.User.Issuer, c.Admin.Issuer},
		{"audiences", c.User.Audience, c.Admin.Audience},
		{"session namespaces", c.User.SessionPrefix, c.Admin.SessionPrefix},
		{"revocation namespaces", c.User.RevocationPrefix, c.Admin.RevocationPrefix},
		{"refresh cookie names", c.User.RefreshCookieName, c.Admin.RefreshCookieName},
		{"session hint cookie names", c.User.SessionHintCookieName, c.Admin.SessionHintCookieName},
		{"CSRF contexts", c.User.CSRFContext, c.Admin.CSRFContext},
	}
	for _, collision := range collisions {
		if collision.left == collision.right {
			return isolationError("%s must be distinct", collision.name)
		}
	}

	allSecrets := []struct {
		name  string
		value string
	}{
		{"User access secret", c.User.AccessSecret},
		{"User refresh secret", c.User.RefreshSecret},
		{"Admin access secret", c.Admin.AccessSecret},
		{"Admin refresh secret", c.Admin.RefreshSecret},
	}
	for i := 0; i < len(allSecrets); i++ {
		for j := i + 1; j < len(allSecrets); j++ {
			if allSecrets[i].value == allSecrets[j].value {
				return isolationError("%s and %s must be distinct", allSecrets[i].name, allSecrets[j].name)
			}
		}
	}

	if isProductionEnvironment(c.Environment) {
		if c.User.CSRFOrigin == c.Admin.CSRFOrigin {
			return isolationError("CSRF origins must be distinct")
		}
		for _, secret := range allSecrets {
			if err := validateProductionSecret(secret.name, secret.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateContextShape(config ContextConfig, expected AuthContext) error {
	if config.Context != expected {
		return isolationError("%s context identity is invalid", expected)
	}
	required := []struct {
		name  string
		value string
	}{
		{"access secret", config.AccessSecret},
		{"refresh secret", config.RefreshSecret},
		{"issuer", config.Issuer},
		{"audience", config.Audience},
		{"session namespace", config.SessionPrefix},
		{"revocation namespace", config.RevocationPrefix},
		{"refresh cookie name", config.RefreshCookieName},
		{"session hint cookie name", config.SessionHintCookieName},
		{"refresh cookie path", config.RefreshCookiePath},
		{"CSRF context", config.CSRFContext},
	}

	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return isolationError("%s %s is required", expected, field.name)
		}
	}
	if config.AccessSecret == config.RefreshSecret {
		return isolationError("%s access and refresh secrets must be distinct", expected)
	}
	return nil
}

func validateProductionSecret(name, secret string) error {
	if len(secret) < 32 {
		return isolationError("%s must contain at least 32 bytes", name)
	}

	lower := strings.ToLower(secret)
	for _, marker := range []string{
		"change-me", "changeme", "default", "example", "placeholder",
		"replace-with", "replace_me", "secret-secret", "test-only", "local-only",
	} {
		if strings.Contains(lower, marker) {
			return isolationError("%s must not use a default or placeholder value", name)
		}
	}

	counts := make(map[rune]int)
	var classes [4]bool
	for _, r := range secret {
		counts[r]++
		switch {
		case unicode.IsLower(r):
			classes[0] = true
		case unicode.IsUpper(r):
			classes[1] = true
		case unicode.IsDigit(r):
			classes[2] = true
		default:
			classes[3] = true
		}
	}
	classCount := 0
	for _, present := range classes {
		if present {
			classCount++
		}
	}

	entropy := 0.0
	length := float64(len([]rune(secret)))
	for _, count := range counts {
		probability := float64(count) / length
		entropy -= probability * math.Log2(probability)
	}
	if len(counts) < 12 || classCount < 3 || entropy < 3.5 {
		return isolationError("%s does not meet production entropy checks", name)
	}
	return nil
}

func isolationError(format string, args ...interface{}) error {
	return fmt.Errorf("%w: %s", ErrInvalidAuthIsolation, fmt.Sprintf(format, args...))
}

func isProductionEnvironment(environment string) bool {
	switch strings.ToLower(strings.TrimSpace(environment)) {
	case "development", "local", "test":
		return false
	default:
		return true
	}
}

// NewContext constructs an Auth instance only after validating an explicit
// context. Pair-level collision checks must be performed with IsolationConfig
// before constructing User and Admin together.
func NewContext(contextConfig ContextConfig, redisClient redis.UniversalClient) (*Auth, error) {
	if contextConfig.Context != ContextUser && contextConfig.Context != ContextAdmin {
		return nil, ErrInvalidAuthContext
	}
	if err := validateContextShape(contextConfig, contextConfig.Context); err != nil {
		return nil, err
	}

	config := DefaultConfig()
	config.Context = contextConfig.Context
	config.JWTSecret = contextConfig.AccessSecret
	config.JWTRefreshSecret = contextConfig.RefreshSecret
	config.JWTIssuer = contextConfig.Issuer
	config.JWTAudience = contextConfig.Audience
	config.SessionPrefix = contextConfig.SessionPrefix
	config.RevocationPrefix = contextConfig.RevocationPrefix
	config.Redis = redisClient
	return New(config), nil
}
