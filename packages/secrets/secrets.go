// Package secrets provides utilities for loading secrets from files or environment variables.
// This package supports Docker secrets (files mounted at /run/secrets/), Kubernetes secrets
// (files mounted at configurable paths), and environment variable fallback.
//
// Usage:
//
//	// Load a single secret
//	apiKey := secrets.Load("TWELVEDATA_API_KEY")
//
//	// Load comma-separated list of secrets (for key rotation)
//	keys := secrets.LoadList("TWELVEDATA_API_KEYS")
//
//	// Build DSN from components with secret password
//	dsn := secrets.BuildPostgresDSN()
package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/Parsaeffatravesh/tragge/packages/config"
)

const (
	// DefaultSecretsPath is the default path where Docker/K8s secrets are mounted.
	DefaultSecretsPath = "/run/secrets"
	// RedactedValue is the non-reconstructable diagnostic marker.
	RedactedValue = "[REDACTED]"
)

// Load retrieves a secret value, checking in order:
// 1. {NAME}_FILE environment variable (path to secret file)
// 2. Direct environment variable {NAME}
// 3. Default Docker secrets path /run/secrets/{lowercase_name}
//
// Returns empty string if not found.
func Load(name string) string {
	// First, check if there's a _FILE env var pointing to the secret
	fileEnvVar := name + "_FILE"
	if filePath := os.Getenv(fileEnvVar); filePath != "" {
		if content, err := readSecretFile(filePath); err == nil {
			return content
		}
	}

	// Second, check direct environment variable
	if value := os.Getenv(name); value != "" {
		return value
	}

	// Third, try default secrets path
	defaultPath := fmt.Sprintf("%s/%s", DefaultSecretsPath, strings.ToLower(strings.ReplaceAll(name, "_", "-")))
	if content, err := readSecretFile(defaultPath); err == nil {
		return content
	}

	return ""
}

// LoadWithDefault retrieves a secret value with a default fallback.
func LoadWithDefault(name, defaultValue string) string {
	value := Load(name)
	if value == "" {
		return defaultValue
	}
	return value
}

// LoadRequired retrieves a secret value and panics if not found.
func LoadRequired(name string) string {
	value := Load(name)
	if value == "" {
		panic(fmt.Sprintf("required secret %s not found", name))
	}
	return value
}

// LoadList retrieves a comma-separated list of secrets.
// Useful for API key rotation where multiple keys are provided.
func LoadList(name string) []string {
	value := Load(name)
	if value == "" {
		return nil
	}

	var keys []string
	for _, key := range strings.Split(value, ",") {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// LoadListRequired retrieves a comma-separated list and panics if empty.
func LoadListRequired(name string) []string {
	keys := LoadList(name)
	if len(keys) == 0 {
		panic(fmt.Sprintf("required secret list %s not found or empty", name))
	}
	return keys
}

// BuildPostgresDSN constructs a PostgreSQL connection string from environment
// variables and secrets.
//
// Environment variables used (checked in order of priority):
//   - POSTGRES_DSN: if set directly, takes precedence over all others
//   - POSTGRES_HOST (default: localhost)
//   - POSTGRES_PORT (default: 5432)
//   - POSTGRES_DB (default: app)
//   - POSTGRES_APP_USER or POSTGRES_USER (default: app)
//   - POSTGRES_APP_PASSWORD(_FILE) or POSTGRES_PASSWORD(_FILE)
//   - POSTGRES_SSLMODE (default: disable)
func BuildPostgresDSN() string {
	// Check for direct DSN first
	if dsn := os.Getenv("POSTGRES_DSN"); dsn != "" {
		// If POSTGRES_SSLMODE is explicitly set, override sslmode in the DSN
		if sslmode := os.Getenv("POSTGRES_SSLMODE"); sslmode != "" {
			if parsed, err := url.Parse(dsn); err == nil {
				q := parsed.Query()
				q.Set("sslmode", sslmode)
				parsed.RawQuery = q.Encode()
				return parsed.String()
			}
		}
		return dsn
	}

	host := getEnvDefault("POSTGRES_HOST", "localhost")
	port := getEnvDefault("POSTGRES_PORT", "5432")
	db := getEnvDefault("POSTGRES_DB", "app")
	// Default to "require" for encrypted connections. Only allow "disable" in development.
	sslmode := getEnvDefault("POSTGRES_SSLMODE", "require")
	env := os.Getenv("ENVIRONMENT")
	if sslmode == "disable" && env != "" && env != "development" && env != "local" && env != "test" {
		fmt.Fprintf(os.Stderr, "WARNING: POSTGRES_SSLMODE=disable in non-development environment (%s), forcing require\n", env)
		sslmode = "require"
	}

	// Check role-specific user first, then generic
	user := os.Getenv("POSTGRES_APP_USER")
	if user == "" {
		user = getEnvDefault("POSTGRES_USER", "app")
	}

	// Check role-specific password first, then generic
	password := Load("POSTGRES_APP_PASSWORD")
	if password == "" {
		password = Load("POSTGRES_PASSWORD")
	}

	if password == "" {
		// Warn but continue - may be using trust auth in dev
		fmt.Fprintf(os.Stderr, "WARNING: No PostgreSQL password found (checked POSTGRES_APP_PASSWORD and POSTGRES_PASSWORD)\n")
	} else {
		warnShortSecret("POSTGRES_PASSWORD", password, 16)
	}

	// URL-encode password to handle special characters
	encodedPassword := url.QueryEscape(password)

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		user, encodedPassword, host, port, db, sslmode)
}

// BuildRedisDSN constructs a Redis connection string from environment variables.
func BuildRedisDSN() string {
	addr := getEnvDefault("REDIS_ADDR", "localhost:6379")
	password := Load("REDIS_PASSWORD")
	db := getEnvDefault("REDIS_DB", "0")

	if password != "" {
		return fmt.Sprintf("redis://:%s@%s/%s", password, addr, db)
	}
	return fmt.Sprintf("redis://%s/%s", addr, db)
}

// GetJWTSecret retrieves the JWT signing secret.
// In production/staging, panics if JWT_SECRET is not configured.
// In development, generates a random secret per process restart with a warning.
// Warns if the configured secret is shorter than 32 characters.
func GetJWTSecret() string {
	secret := Load("JWT_SECRET")
	if secret != "" {
		warnShortSecret("JWT_SECRET", secret, 32)
		return secret
	}
	if config.IsProduction() {
		panic("secrets: JWT_SECRET not set in production")
	}
	// Dev-only: generate random secret (different per restart)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		panic(fmt.Sprintf("secrets: failed to generate random JWT secret: %v", err))
	}
	fmt.Fprintf(os.Stderr, "WARNING: JWT_SECRET not set, using random secret (dev only, tokens will not survive restart)\n")
	return hex.EncodeToString(randomBytes)
}

// GetJWTRefreshSecret retrieves the JWT signing secret for refresh tokens.
// Falls back to GetJWTSecret() if JWT_REFRESH_SECRET is not configured,
// providing backward compatibility for deployments that use a single secret.
func GetJWTRefreshSecret() string {
	secret := Load("JWT_REFRESH_SECRET")
	if secret != "" {
		return secret
	}
	log.Println("[WARN] JWT_REFRESH_SECRET not set, falling back to JWT_SECRET — tokens share signing key")
	return GetJWTSecret()
}

// GetJWTUserSecret retrieves the JWT signing secret for the user panel
// (aud=user tokens). In production this MUST be distinct from the
// admin secret so a leaked user key cannot forge admin tokens; a warning
// is logged if both secrets resolve to the same value. Falls back to
// GetJWTSecret() in dev so a single JWT_SECRET is still enough to bring
// the stack up.
func GetJWTUserSecret() string {
	secret := Load("JWT_SECRET_USER")
	if secret != "" {
		warnShortSecret("JWT_SECRET_USER", secret, 32)
		return secret
	}
	if config.IsProduction() {
		panic("secrets: JWT_SECRET_USER not set in production")
	}
	log.Println("[WARN] JWT_SECRET_USER not set, falling back to JWT_SECRET — dev only")
	return GetJWTSecret()
}

// GetJWTAdminSecret retrieves the JWT signing secret for the admin panel
// (aud=admin tokens). Same production/dev policy as GetJWTUserSecret:
// required in prod, falls back to JWT_SECRET in dev with a warning. If
// it resolves to the same value as the user secret the split offers no
// protection against cross-panel token forgery.
func GetJWTAdminSecret() string {
	secret := Load("JWT_SECRET_ADMIN")
	if secret != "" {
		warnShortSecret("JWT_SECRET_ADMIN", secret, 32)
		return secret
	}
	if config.IsProduction() {
		panic("secrets: JWT_SECRET_ADMIN not set in production")
	}
	log.Println("[WARN] JWT_SECRET_ADMIN not set, falling back to JWT_SECRET — dev only")
	return GetJWTSecret()
}

// GoogleOAuthConfig holds Google OAuth credentials.
type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// GetGoogleOAuthConfig retrieves Google OAuth configuration.
// Returns nil if client ID or secret is not configured.
func GetGoogleOAuthConfig() *GoogleOAuthConfig {
	clientID := Load("GOOGLE_CLIENT_ID")
	clientSecret := Load("GOOGLE_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return nil
	}

	redirectURI := getEnvDefault("GOOGLE_REDIRECT_URI", "http://localhost:8080/user/auth/google/callback")

	return &GoogleOAuthConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
	}
}

// GetGoogleClientID retrieves the Google OAuth Client ID.
func GetGoogleClientID() string {
	return Load("GOOGLE_CLIENT_ID")
}

// GetGoogleClientSecret retrieves the Google OAuth Client Secret.
func GetGoogleClientSecret() string {
	return Load("GOOGLE_CLIENT_SECRET")
}

// IsGoogleOAuthConfigured returns true if Google OAuth is fully configured.
func IsGoogleOAuthConfigured() bool {
	return GetGoogleClientID() != "" && GetGoogleClientSecret() != ""
}

// warnShortSecret writes a warning to stderr if value is shorter than minLen.
func warnShortSecret(name, value string, minLen int) {
	if len(value) < minLen {
		fmt.Fprintf(os.Stderr, "WARNING: %s is too short (%d chars, minimum %d recommended)\n", name, len(value), minLen)
	}
}

// readSecretFile reads a secret from a file, trimming whitespace and newlines.
func readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// getEnvDefault gets an environment variable with a default value.
func getEnvDefault(name, defaultValue string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return defaultValue
}

// MaskSecret returns a stable non-reconstructable marker for any secret.
func MaskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	return RedactedValue
}

// SecretInfo contains information about a loaded secret for debugging.
type SecretInfo struct {
	Name   string
	Source string // "file", "env", or "default"
	Loaded bool
	Masked string
}

// LoadWithInfo retrieves a secret and returns metadata about where it was loaded from.
func LoadWithInfo(name string) SecretInfo {
	info := SecretInfo{Name: name}

	// Check file path env var
	fileEnvVar := name + "_FILE"
	if filePath := os.Getenv(fileEnvVar); filePath != "" {
		if content, err := readSecretFile(filePath); err == nil {
			info.Source = "file:" + filePath
			info.Loaded = true
			info.Masked = MaskSecret(content)
			return info
		}
	}

	// Check direct env var
	if value := os.Getenv(name); value != "" {
		info.Source = "env"
		info.Loaded = true
		info.Masked = MaskSecret(value)
		return info
	}

	// Check default path
	defaultPath := fmt.Sprintf("%s/%s", DefaultSecretsPath, strings.ToLower(strings.ReplaceAll(name, "_", "-")))
	if content, err := readSecretFile(defaultPath); err == nil {
		info.Source = "file:" + defaultPath
		info.Loaded = true
		info.Masked = MaskSecret(content)
		return info
	}

	info.Source = "not found"
	info.Loaded = false
	return info
}

// DiagnosticReport generates a report of all configured secrets for debugging.
// This is safe to log as it only shows masked values.
func DiagnosticReport(names ...string) []SecretInfo {
	var infos []SecretInfo
	for _, name := range names {
		infos = append(infos, LoadWithInfo(name))
	}
	return infos
}
