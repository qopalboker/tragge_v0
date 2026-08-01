package db

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCredentialConfigFromEnv(t *testing.T) {
	// Save original environment
	origEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range origEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	// Clear environment
	os.Clearenv()

	// Set test environment
	os.Setenv("POSTGRES_HOST", "testhost")
	os.Setenv("POSTGRES_PORT", "5433")
	os.Setenv("POSTGRES_DB", "testdb")
	os.Setenv("POSTGRES_SSLMODE", "require")
	os.Setenv("POSTGRES_ADMIN_USER", "test_admin")
	os.Setenv("POSTGRES_ADMIN_PASSWORD", "admin_pass")
	os.Setenv("POSTGRES_APP_USER", "test_app")
	os.Setenv("POSTGRES_APP_PASSWORD", "app_pass")
	os.Setenv("POSTGRES_READONLY_USER", "test_readonly")
	os.Setenv("POSTGRES_READONLY_PASSWORD", "readonly_pass")

	cfg := CredentialConfigFromEnv()

	if cfg.Host != "testhost" {
		t.Errorf("Host = %s, want testhost", cfg.Host)
	}
	if cfg.Port != "5433" {
		t.Errorf("Port = %s, want 5433", cfg.Port)
	}
	if cfg.Database != "testdb" {
		t.Errorf("Database = %s, want testdb", cfg.Database)
	}
	if cfg.SSLMode != "require" {
		t.Errorf("SSLMode = %s, want require", cfg.SSLMode)
	}
	if cfg.AdminUser != "test_admin" {
		t.Errorf("AdminUser = %s, want test_admin", cfg.AdminUser)
	}
	if cfg.AdminPassword != "admin_pass" {
		t.Errorf("AdminPassword = %s, want admin_pass", cfg.AdminPassword)
	}
}

func TestCredentialConfigDSN(t *testing.T) {
	cfg := CredentialConfig{
		Host:             "localhost",
		Port:             "5432",
		Database:         "app",
		SSLMode:          "require",
		AdminUser:        "admin",
		AdminPassword:    "adminpass",
		AppUser:          "appuser",
		AppPassword:      "apppass",
		ReadonlyUser:     "readonly",
		ReadonlyPassword: "readonlypass",
	}

	tests := []struct {
		role     UserRole
		wantUser string
	}{
		{UserRoleAdmin, "admin"},
		{UserRoleApp, "appuser"},
		{UserRoleReadonly, "readonly"},
	}

	for _, tt := range tests {
		dsn := cfg.DSN(tt.role)
		if !strings.Contains(dsn, tt.wantUser+":") {
			t.Errorf("DSN(%v) = %s, want to contain %s:", tt.role, dsn, tt.wantUser)
		}
		if !strings.Contains(dsn, "sslmode=require") {
			t.Errorf("DSN(%v) = %s, want to contain sslmode=require", tt.role, dsn)
		}
	}
}

func TestCredentialConfigDSNWithSpecialChars(t *testing.T) {
	cfg := CredentialConfig{
		Host:          "localhost",
		Port:          "5432",
		Database:      "app",
		SSLMode:       "disable",
		AppUser:       "appuser",
		AppPassword:   "pass@word#123!",
	}

	dsn := cfg.AppDSN()

	// Password should be URL encoded
	if !strings.Contains(dsn, "pass%40word%23123%21") {
		t.Errorf("DSN = %s, want URL-encoded password", dsn)
	}
}

func TestCredentialConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     CredentialConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: CredentialConfig{
				Host:         "localhost",
				Port:         "5432",
				Database:     "app",
				SSLMode:      "disable",
				AppUser:      "app",
				AppPassword:  "pass",
			},
			wantErr: false,
		},
		{
			name: "missing host",
			cfg: CredentialConfig{
				Port:        "5432",
				Database:    "app",
				SSLMode:     "disable",
				AppUser:     "app",
				AppPassword: "pass",
			},
			wantErr: true,
		},
		{
			name: "missing credentials",
			cfg: CredentialConfig{
				Host:     "localhost",
				Port:     "5432",
				Database: "app",
				SSLMode:  "disable",
			},
			wantErr: true,
		},
		{
			name: "invalid ssl mode",
			cfg: CredentialConfig{
				Host:        "localhost",
				Port:        "5432",
				Database:    "app",
				SSLMode:     "invalid",
				AppUser:     "app",
				AppPassword: "pass",
			},
			wantErr: true,
		},
		{
			name: "verify-full without root cert",
			cfg: CredentialConfig{
				Host:        "localhost",
				Port:        "5432",
				Database:    "app",
				SSLMode:     "verify-full",
				AppUser:     "app",
				AppPassword: "pass",
			},
			wantErr: true,
		},
		{
			name: "verify-full with root cert",
			cfg: CredentialConfig{
				Host:        "localhost",
				Port:        "5432",
				Database:    "app",
				SSLMode:     "verify-full",
				SSLRootCert: "/path/to/ca.crt",
				AppUser:     "app",
				AppPassword: "pass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSSLModeDisableInProduction(t *testing.T) {
	// Save original environment
	origEnv := os.Getenv("ENVIRONMENT")
	defer os.Setenv("ENVIRONMENT", origEnv)

	cfg := CredentialConfig{
		Host:        "localhost",
		Port:        "5432",
		Database:    "app",
		SSLMode:     "disable",
		AppUser:     "app",
		AppPassword: "pass",
	}

	// Should succeed in non-production
	os.Setenv("ENVIRONMENT", "development")
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should succeed in development, got: %v", err)
	}

	// Should fail in production
	os.Setenv("ENVIRONMENT", "production")
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with SSLMode=disable in production")
	}

	// Should also fail with "prod" shorthand
	os.Setenv("ENVIRONMENT", "prod")
	if err := cfg.Validate(); err == nil {
		t.Error("Validate() should fail with SSLMode=disable in prod")
	}

	// Should succeed with require in production
	cfg.SSLMode = "require"
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() should succeed with SSLMode=require in prod, got: %v", err)
	}
}

func TestCredentialConfigHasRole(t *testing.T) {
	cfg := CredentialConfig{
		AdminUser:     "admin",
		AdminPassword: "pass",
		AppUser:       "app",
		// No app password
		ReadonlyUser:     "readonly",
		ReadonlyPassword: "pass",
	}

	if !cfg.HasRole(UserRoleAdmin) {
		t.Error("HasRole(Admin) = false, want true")
	}
	if cfg.HasRole(UserRoleApp) {
		t.Error("HasRole(App) = true, want false (no password)")
	}
	if !cfg.HasRole(UserRoleReadonly) {
		t.Error("HasRole(Readonly) = false, want true")
	}
}

func TestMaskPassword(t *testing.T) {
	tests := []struct {
		dsn      string
		wantMask string
	}{
		{
			dsn:      "postgres://user:secretpass@localhost:5432/db",
			wantMask: "postgres://user:****@localhost:5432/db",
		},
		{
			dsn:      "postgres://user:pass@word@localhost:5432/db",
			wantMask: "postgres://user:****@localhost:5432/db",
		},
		{
			dsn:      "postgres://user@localhost:5432/db",
			wantMask: "postgres://user@localhost:5432/db",
		},
	}

	for _, tt := range tests {
		masked := MaskPassword(tt.dsn)
		if masked != tt.wantMask {
			t.Errorf("MaskPassword(%s) = %s, want %s", tt.dsn, masked, tt.wantMask)
		}
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name       string
		cfg        CredentialConfig
		wantIssues int
	}{
		{
			name: "strong passwords - no issues",
			cfg: CredentialConfig{
				AppUser:     "app",
				AppPassword: "aB3xK9mNpR7wR2yT5vU8jL4hG6fD0eC1sAzXcVbNmJwHrTy",
			},
			wantIssues: 0,
		},
		{
			name: "short password",
			cfg: CredentialConfig{
				AppUser:     "app",
				AppPassword: "Parsa1590320",
			},
			wantIssues: 1, // too short
		},
		{
			name: "missing uppercase",
			cfg: CredentialConfig{
				AppUser:     "app",
				AppPassword: "abcdefghijklmnopqrst1234",
			},
			wantIssues: 1, // no uppercase
		},
		{
			name: "missing lowercase",
			cfg: CredentialConfig{
				AppUser:     "app",
				AppPassword: "ABCDEFGHIJKLMNOPQRST1234",
			},
			wantIssues: 1, // no lowercase
		},
		{
			name: "missing digits",
			cfg: CredentialConfig{
				AppUser:     "app",
				AppPassword: "AbcdefghijklmnopqrstUvwx",
			},
			wantIssues: 1, // no digits
		},
		{
			name: "contains weak pattern",
			cfg: CredentialConfig{
				AppUser:     "app",
				AppPassword: "MySecurepassword12345678",
			},
			wantIssues: 1, // contains "password"
		},
		{
			name: "multiple issues - short and weak",
			cfg: CredentialConfig{
				AppUser:     "app",
				AppPassword: "password1",
			},
			wantIssues: 3, // short + no uppercase + weak pattern
		},
		{
			name: "empty password skipped",
			cfg: CredentialConfig{
				AppUser: "app",
				// No password set
			},
			wantIssues: 0,
		},
		{
			name: "multiple roles checked",
			cfg: CredentialConfig{
				AdminUser:        "admin",
				AdminPassword:    "short1A", // short
				AppUser:          "app",
				AppPassword:      "aB3xK9mNpQ7wR2yT5vU8jL4", // strong
				ReadonlyUser:     "readonly",
				ReadonlyPassword: "weak", // short + missing classes
			},
			wantIssues: 4, // admin: short; readonly: short + no upper + no digit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := tt.cfg.ValidatePasswordStrength()
			if len(issues) != tt.wantIssues {
				t.Errorf("ValidatePasswordStrength() returned %d issues, want %d", len(issues), tt.wantIssues)
				for _, issue := range issues {
					t.Logf("  %s: %s", issue.Role, issue.Message)
				}
			}
		})
	}
}

func TestWarnWeakPasswords(t *testing.T) {
	cfg := CredentialConfig{
		AppUser:     "app",
		AppPassword: "weak",
	}

	var warnings []string
	logFunc := func(format string, args ...interface{}) {
		warnings = append(warnings, fmt.Sprintf(format, args...))
	}

	cfg.WarnWeakPasswords(logFunc)

	if len(warnings) == 0 {
		t.Error("WarnWeakPasswords() produced no warnings for weak password")
	}

	// Verify warnings mention "SECURITY WARNING"
	for _, w := range warnings {
		if !strings.Contains(w, "SECURITY WARNING") {
			t.Errorf("Warning %q does not contain 'SECURITY WARNING'", w)
		}
	}
}

func TestBackwardsCompatibility(t *testing.T) {
	// Save original environment
	origEnv := os.Environ()
	defer func() {
		os.Clearenv()
		for _, e := range origEnv {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				os.Setenv(parts[0], parts[1])
			}
		}
	}()

	// Clear environment
	os.Clearenv()

	// Set only legacy POSTGRES_PASSWORD
	os.Setenv("POSTGRES_HOST", "localhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_DB", "app")
	os.Setenv("POSTGRES_PASSWORD", "legacy_pass")

	cfg := CredentialConfigFromEnv()

	// All roles should use the legacy password
	if cfg.AdminPassword != "legacy_pass" {
		t.Errorf("AdminPassword = %s, want legacy_pass", cfg.AdminPassword)
	}
	if cfg.AppPassword != "legacy_pass" {
		t.Errorf("AppPassword = %s, want legacy_pass", cfg.AppPassword)
	}
	if cfg.ReadonlyPassword != "legacy_pass" {
		t.Errorf("ReadonlyPassword = %s, want legacy_pass", cfg.ReadonlyPassword)
	}
}
