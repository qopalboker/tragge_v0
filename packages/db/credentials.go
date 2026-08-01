// Package db provides database utilities including credential management
// for PostgreSQL with role-based access control.
package db

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"unicode"
)

// UserRole represents the type of database user for role-based access.
type UserRole string

const (
	// UserRoleAdmin is for administrative operations (migrations, DDL).
	// Use this for database migrations and schema changes.
	UserRoleAdmin UserRole = "admin"

	// UserRoleApp is for application operations (CRUD on app tables).
	// Use this for normal application database access.
	UserRoleApp UserRole = "app"

	// UserRoleReadonly is for read-only operations (replicas, reporting).
	// Use this for read-heavy operations that don't need write access.
	UserRoleReadonly UserRole = "readonly"
)

// CredentialConfig holds the configuration for database credentials.
type CredentialConfig struct {
	// Host is the database server hostname.
	Host string

	// Port is the database server port.
	Port string

	// Database is the database name.
	Database string

	// SSLMode is the SSL connection mode (disable, require, verify-ca, verify-full).
	SSLMode string

	// AdminUser is the administrative user name.
	AdminUser string

	// AdminPassword is the administrative user password.
	AdminPassword string

	// AppUser is the application user name.
	AppUser string

	// AppPassword is the application user password.
	AppPassword string

	// ReadonlyUser is the read-only user name.
	ReadonlyUser string

	// ReadonlyPassword is the read-only user password.
	ReadonlyPassword string

	// SSLRootCert is the path to the SSL root certificate (optional).
	SSLRootCert string

	// SSLCert is the path to the SSL client certificate (optional).
	SSLCert string

	// SSLKey is the path to the SSL client key (optional).
	SSLKey string
}

// DefaultCredentialConfig returns a CredentialConfig with default values.
func DefaultCredentialConfig() CredentialConfig {
	return CredentialConfig{
		Host:         "localhost",
		Port:         "5432",
		Database:     "app",
		SSLMode:      "require",
		AdminUser:    "tragge_admin",
		AppUser:      "tragge_app",
		ReadonlyUser: "tragge_readonly",
	}
}

// CredentialConfigFromEnv creates a CredentialConfig from environment variables.
// It supports both direct environment variables and Docker secrets (_FILE suffix).
//
// Environment variables:
//   - POSTGRES_HOST (default: localhost)
//   - POSTGRES_PORT (default: 5432)
//   - POSTGRES_DB (default: app)
//   - POSTGRES_SSLMODE (default: require)
//   - POSTGRES_ADMIN_USER (default: tragge_admin)
//   - POSTGRES_ADMIN_PASSWORD or POSTGRES_ADMIN_PASSWORD_FILE
//   - POSTGRES_APP_USER (default: tragge_app)
//   - POSTGRES_APP_PASSWORD or POSTGRES_APP_PASSWORD_FILE
//   - POSTGRES_READONLY_USER (default: tragge_readonly)
//   - POSTGRES_READONLY_PASSWORD or POSTGRES_READONLY_PASSWORD_FILE
//   - POSTGRES_SSLROOTCERT (optional, for verify-ca/verify-full)
//   - POSTGRES_SSLCERT (optional, for client certificate auth)
//   - POSTGRES_SSLKEY (optional, for client certificate auth)
func CredentialConfigFromEnv() CredentialConfig {
	cfg := DefaultCredentialConfig()

	// Host configuration
	if v := os.Getenv("POSTGRES_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("POSTGRES_PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("POSTGRES_DB"); v != "" {
		cfg.Database = v
	}
	if v := os.Getenv("POSTGRES_SSLMODE"); v != "" {
		cfg.SSLMode = v
	}

	// Warn if SSL is disabled in production
	if cfg.SSLMode == "disable" {
		env := os.Getenv("ENVIRONMENT")
		if env != "development" && env != "local" && env != "test" {
			log.Println("[SECURITY WARNING] POSTGRES_SSLMODE=disable in non-development environment. " +
				"Database connections are unencrypted. Set POSTGRES_SSLMODE=require or verify-full for production.")
		}
	}

	// Admin user
	if v := os.Getenv("POSTGRES_ADMIN_USER"); v != "" {
		cfg.AdminUser = v
	}
	cfg.AdminPassword = readSecret("POSTGRES_ADMIN_PASSWORD")

	// App user
	if v := os.Getenv("POSTGRES_APP_USER"); v != "" {
		cfg.AppUser = v
	}
	cfg.AppPassword = readSecret("POSTGRES_APP_PASSWORD")

	// Readonly user
	if v := os.Getenv("POSTGRES_READONLY_USER"); v != "" {
		cfg.ReadonlyUser = v
	}
	cfg.ReadonlyPassword = readSecret("POSTGRES_READONLY_PASSWORD")

	// SSL certificates
	cfg.SSLRootCert = os.Getenv("POSTGRES_SSLROOTCERT")
	cfg.SSLCert = os.Getenv("POSTGRES_SSLCERT")
	cfg.SSLKey = os.Getenv("POSTGRES_SSLKEY")

	// Backwards compatibility: if no role-based passwords are set,
	// try the legacy POSTGRES_PASSWORD
	if cfg.AdminPassword == "" && cfg.AppPassword == "" && cfg.ReadonlyPassword == "" {
		legacyPassword := readSecret("POSTGRES_PASSWORD")
		if legacyPassword != "" {
			cfg.AdminPassword = legacyPassword
			cfg.AppPassword = legacyPassword
			cfg.ReadonlyPassword = legacyPassword
		}
	}

	return cfg
}

// DSN returns the PostgreSQL connection string for the specified user role.
func (c CredentialConfig) DSN(role UserRole) string {
	var user, password string

	switch role {
	case UserRoleAdmin:
		user = c.AdminUser
		password = c.AdminPassword
	case UserRoleApp:
		user = c.AppUser
		password = c.AppPassword
	case UserRoleReadonly:
		user = c.ReadonlyUser
		password = c.ReadonlyPassword
	default:
		// Default to app user
		user = c.AppUser
		password = c.AppPassword
	}

	return c.buildDSN(user, password)
}

// AdminDSN returns the DSN for administrative operations.
func (c CredentialConfig) AdminDSN() string {
	return c.DSN(UserRoleAdmin)
}

// AppDSN returns the DSN for application operations.
func (c CredentialConfig) AppDSN() string {
	return c.DSN(UserRoleApp)
}

// ReadonlyDSN returns the DSN for read-only operations.
func (c CredentialConfig) ReadonlyDSN() string {
	return c.DSN(UserRoleReadonly)
}

// buildDSN constructs the PostgreSQL connection string.
func (c CredentialConfig) buildDSN(user, password string) string {
	// URL encode password to handle special characters
	encodedPassword := url.QueryEscape(password)

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		user, encodedPassword, c.Host, c.Port, c.Database)

	// Add SSL parameters
	params := []string{}
	if c.SSLMode != "" {
		params = append(params, "sslmode="+c.SSLMode)
	}
	if c.SSLRootCert != "" {
		params = append(params, "sslrootcert="+c.SSLRootCert)
	}
	if c.SSLCert != "" {
		params = append(params, "sslcert="+c.SSLCert)
	}
	if c.SSLKey != "" {
		params = append(params, "sslkey="+c.SSLKey)
	}

	if len(params) > 0 {
		dsn += "?" + strings.Join(params, "&")
	}

	return dsn
}

// Validate checks if the credential configuration is valid.
func (c CredentialConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("POSTGRES_HOST is required")
	}
	if c.Port == "" {
		return fmt.Errorf("POSTGRES_PORT is required")
	}
	if c.Database == "" {
		return fmt.Errorf("POSTGRES_DB is required")
	}

	// Validate at least one user has credentials
	hasAdmin := c.AdminUser != "" && c.AdminPassword != ""
	hasApp := c.AppUser != "" && c.AppPassword != ""
	hasReadonly := c.ReadonlyUser != "" && c.ReadonlyPassword != ""

	if !hasAdmin && !hasApp && !hasReadonly {
		return fmt.Errorf("at least one database user must have credentials configured")
	}

	// Validate SSL mode
	validSSLModes := map[string]bool{
		"disable":     true,
		"prefer":      true,
		"require":     true,
		"verify-ca":   true,
		"verify-full": true,
	}
	if !validSSLModes[c.SSLMode] {
		return fmt.Errorf("invalid POSTGRES_SSLMODE: %s (must be disable, prefer, require, verify-ca, or verify-full)", c.SSLMode)
	}

	// Reject weak SSL modes in production — only require, verify-ca, verify-full are allowed.
	// An empty ENVIRONMENT is treated as production for safety.
	env := os.Getenv("ENVIRONMENT")
	isNonProd := env == "development" || env == "local" || env == "test"
	if c.SSLMode == "disable" || c.SSLMode == "prefer" {
		if !isNonProd {
			return fmt.Errorf("POSTGRES_SSLMODE=%s is not allowed in production; use require, verify-ca, or verify-full", c.SSLMode)
		}
	}

	// Validate SSL certificates for strict modes
	if c.SSLMode == "verify-ca" || c.SSLMode == "verify-full" {
		if c.SSLRootCert == "" {
			return fmt.Errorf("POSTGRES_SSLROOTCERT is required for sslmode=%s", c.SSLMode)
		}
	}

	return nil
}

// HasRole returns true if credentials are configured for the specified role.
func (c CredentialConfig) HasRole(role UserRole) bool {
	switch role {
	case UserRoleAdmin:
		return c.AdminUser != "" && c.AdminPassword != ""
	case UserRoleApp:
		return c.AppUser != "" && c.AppPassword != ""
	case UserRoleReadonly:
		return c.ReadonlyUser != "" && c.ReadonlyPassword != ""
	default:
		return false
	}
}

// readSecret reads a secret from either an environment variable or a file.
// It first checks for a _FILE suffixed variable pointing to a file,
// then falls back to the direct environment variable.
func readSecret(name string) string {
	// Check for file-based secret first
	filePath := os.Getenv(name + "_FILE")
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	// Fall back to direct environment variable
	return os.Getenv(name)
}

// MaskPassword returns a masked version of the DSN for logging.
func MaskPassword(dsn string) string {
	// Parse the DSN URL
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}

	// Mask the password by replacing it in the original string
	if _, hasPassword := u.User.Password(); hasPassword {
		u.User = url.UserPassword(u.User.Username(), "****")
		// Reconstruct manually to avoid URL-encoding the asterisks
		masked := u.Scheme + "://" + u.User.Username() + ":****@" + u.Host + u.RequestURI()
		return masked
	}

	return u.String()
}

// SSLConfig holds SSL/TLS configuration for database connections.
type SSLConfig struct {
	// Mode is the SSL connection mode.
	Mode string

	// RootCert is the path to the CA certificate.
	RootCert string

	// ClientCert is the path to the client certificate.
	ClientCert string

	// ClientKey is the path to the client private key.
	ClientKey string
}

// SSLConfigFromEnv creates an SSLConfig from environment variables.
func SSLConfigFromEnv() SSLConfig {
	return SSLConfig{
		Mode:       getEnvOrDefault("POSTGRES_SSLMODE", "require"),
		RootCert:   os.Getenv("POSTGRES_SSLROOTCERT"),
		ClientCert: os.Getenv("POSTGRES_SSLCERT"),
		ClientKey:  os.Getenv("POSTGRES_SSLKEY"),
	}
}

// Params returns SSL parameters for a connection string.
func (s SSLConfig) Params() map[string]string {
	params := map[string]string{
		"sslmode": s.Mode,
	}
	if s.RootCert != "" {
		params["sslrootcert"] = s.RootCert
	}
	if s.ClientCert != "" {
		params["sslcert"] = s.ClientCert
	}
	if s.ClientKey != "" {
		params["sslkey"] = s.ClientKey
	}
	return params
}

// MinPasswordLength is the minimum acceptable password length for database credentials.
const MinPasswordLength = 20

// PasswordIssue describes a problem found with a database password.
type PasswordIssue struct {
	Role    UserRole
	Message string
}

// ValidatePasswordStrength checks loaded passwords against minimum security requirements.
// Returns a list of issues found. Returns nil if all passwords are acceptable.
// Only validates roles that have a password configured (non-empty).
func (c CredentialConfig) ValidatePasswordStrength() []PasswordIssue {
	var issues []PasswordIssue

	type rolePassword struct {
		role     UserRole
		password string
	}

	passwords := []rolePassword{
		{UserRoleAdmin, c.AdminPassword},
		{UserRoleApp, c.AppPassword},
		{UserRoleReadonly, c.ReadonlyPassword},
	}

	for _, rp := range passwords {
		if rp.password == "" {
			continue
		}
		issues = append(issues, checkPassword(rp.role, rp.password)...)
	}

	return issues
}

// checkPassword validates a single password and returns any issues found.
func checkPassword(role UserRole, password string) []PasswordIssue {
	var issues []PasswordIssue

	if len(password) < MinPasswordLength {
		issues = append(issues, PasswordIssue{
			Role:    role,
			Message: fmt.Sprintf("too short (%d chars, minimum %d)", len(password), MinPasswordLength),
		})
	}

	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}

	if !hasUpper {
		issues = append(issues, PasswordIssue{
			Role:    role,
			Message: "missing uppercase letters",
		})
	}
	if !hasLower {
		issues = append(issues, PasswordIssue{
			Role:    role,
			Message: "missing lowercase letters",
		})
	}
	if !hasDigit {
		issues = append(issues, PasswordIssue{
			Role:    role,
			Message: "missing digits",
		})
	}

	lower := strings.ToLower(password)
	weakPatterns := []string{"password", "123456", "qwerty", "admin", "letmein", "welcome"}
	for _, pattern := range weakPatterns {
		if strings.Contains(lower, pattern) {
			issues = append(issues, PasswordIssue{
				Role:    role,
				Message: fmt.Sprintf("contains common weak pattern %q", pattern),
			})
			break
		}
	}

	return issues
}

// WarnWeakPasswords logs warnings for any passwords that don't meet minimum strength requirements.
// Intended to be called during service initialization. The logFunc should accept a format string
// and arguments (e.g., log.Printf or fmt.Printf).
func (c CredentialConfig) WarnWeakPasswords(logFunc func(string, ...interface{})) {
	issues := c.ValidatePasswordStrength()
	for _, issue := range issues {
		logFunc("SECURITY WARNING: %s role password: %s", issue.Role, issue.Message)
	}
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(name, defaultValue string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return defaultValue
}
