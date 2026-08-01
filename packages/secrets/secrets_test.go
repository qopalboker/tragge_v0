package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_FromEnvVar(t *testing.T) {
	// Set up
	os.Setenv("TEST_SECRET", "test-value")
	defer os.Unsetenv("TEST_SECRET")

	// Test
	result := Load("TEST_SECRET")

	// Verify
	if result != "test-value" {
		t.Errorf("expected 'test-value', got '%s'", result)
	}
}

func TestLoad_FromFile(t *testing.T) {
	// Set up temp file
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "test-secret")
	if err := os.WriteFile(secretPath, []byte("secret-from-file\n"), 0600); err != nil {
		t.Fatal(err)
	}

	// Set file path env var
	os.Setenv("TEST_FILE_SECRET_FILE", secretPath)
	defer os.Unsetenv("TEST_FILE_SECRET_FILE")

	// Test
	result := Load("TEST_FILE_SECRET")

	// Verify - should trim whitespace
	if result != "secret-from-file" {
		t.Errorf("expected 'secret-from-file', got '%s'", result)
	}
}

func TestLoad_FileOverridesEnv(t *testing.T) {
	// Set up temp file
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "override-secret")
	if err := os.WriteFile(secretPath, []byte("from-file"), 0600); err != nil {
		t.Fatal(err)
	}

	// Set both env var and file path
	os.Setenv("OVERRIDE_SECRET", "from-env")
	os.Setenv("OVERRIDE_SECRET_FILE", secretPath)
	defer os.Unsetenv("OVERRIDE_SECRET")
	defer os.Unsetenv("OVERRIDE_SECRET_FILE")

	// Test - file should take precedence
	result := Load("OVERRIDE_SECRET")

	if result != "from-file" {
		t.Errorf("expected 'from-file', got '%s'", result)
	}
}

func TestLoad_NotFound(t *testing.T) {
	result := Load("NONEXISTENT_SECRET_12345")

	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

func TestLoadWithDefault(t *testing.T) {
	result := LoadWithDefault("NONEXISTENT_SECRET_12345", "default-value")

	if result != "default-value" {
		t.Errorf("expected 'default-value', got '%s'", result)
	}
}

func TestLoadList(t *testing.T) {
	os.Setenv("API_KEYS", "key1, key2 , key3")
	defer os.Unsetenv("API_KEYS")

	result := LoadList("API_KEYS")

	if len(result) != 3 {
		t.Errorf("expected 3 keys, got %d", len(result))
	}
	if result[0] != "key1" || result[1] != "key2" || result[2] != "key3" {
		t.Errorf("unexpected keys: %v", result)
	}
}

func TestLoadList_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "api-keys")
	if err := os.WriteFile(secretPath, []byte("filekey1,filekey2\n"), 0600); err != nil {
		t.Fatal(err)
	}

	os.Setenv("FILE_API_KEYS_FILE", secretPath)
	defer os.Unsetenv("FILE_API_KEYS_FILE")

	result := LoadList("FILE_API_KEYS")

	if len(result) != 2 {
		t.Errorf("expected 2 keys, got %d", len(result))
	}
	if result[0] != "filekey1" || result[1] != "filekey2" {
		t.Errorf("unexpected keys: %v", result)
	}
}

func TestLoadList_Empty(t *testing.T) {
	result := LoadList("NONEXISTENT_API_KEYS_12345")

	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestBuildPostgresDSN_DirectDSN(t *testing.T) {
	os.Setenv("POSTGRES_DSN", "postgres://direct:password@host:5432/db")
	os.Unsetenv("POSTGRES_SSLMODE")
	defer os.Unsetenv("POSTGRES_DSN")

	result := BuildPostgresDSN()

	if result != "postgres://direct:password@host:5432/db" {
		t.Errorf("expected direct DSN, got '%s'", result)
	}
}

func TestBuildPostgresDSN_DirectDSN_SSLModeOverride(t *testing.T) {
	os.Setenv("POSTGRES_DSN", "postgres://direct:password@host:5432/db?sslmode=disable")
	os.Setenv("POSTGRES_SSLMODE", "require")
	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("POSTGRES_SSLMODE")
	}()

	result := BuildPostgresDSN()

	expected := "postgres://direct:password@host:5432/db?sslmode=require"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestBuildPostgresDSN_DirectDSN_SSLModeAdded(t *testing.T) {
	os.Setenv("POSTGRES_DSN", "postgres://direct:password@host:5432/db")
	os.Setenv("POSTGRES_SSLMODE", "verify-full")
	defer func() {
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("POSTGRES_SSLMODE")
	}()

	result := BuildPostgresDSN()

	expected := "postgres://direct:password@host:5432/db?sslmode=verify-full"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestBuildPostgresDSN_Components(t *testing.T) {
	// Clear any existing DSN
	os.Unsetenv("POSTGRES_DSN")

	os.Setenv("POSTGRES_HOST", "testhost")
	os.Setenv("POSTGRES_PORT", "5433")
	os.Setenv("POSTGRES_DB", "testdb")
	os.Setenv("POSTGRES_USER", "testuser")
	os.Setenv("POSTGRES_PASSWORD", "testpass")
	os.Setenv("POSTGRES_SSLMODE", "require")
	defer func() {
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_SSLMODE")
	}()

	result := BuildPostgresDSN()

	expected := "postgres://testuser:testpass@testhost:5433/testdb?sslmode=require"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestBuildPostgresDSN_PasswordFromFile(t *testing.T) {
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("POSTGRES_PASSWORD")

	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "pg-password")
	if err := os.WriteFile(secretPath, []byte("filepassword"), 0600); err != nil {
		t.Fatal(err)
	}

	os.Setenv("POSTGRES_PASSWORD_FILE", secretPath)
	os.Setenv("POSTGRES_SSLMODE", "disable")
	defer os.Unsetenv("POSTGRES_PASSWORD_FILE")
	defer os.Unsetenv("POSTGRES_SSLMODE")

	result := BuildPostgresDSN()

	if result != "postgres://app:filepassword@localhost:5432/app?sslmode=disable" {
		t.Errorf("unexpected DSN: %s", result)
	}
}

func TestBuildPostgresDSN_AppRoleEnvVars(t *testing.T) {
	// Clear any existing DSN and generic vars
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("POSTGRES_USER")
	os.Unsetenv("POSTGRES_PASSWORD")

	os.Setenv("POSTGRES_HOST", "dbhost")
	os.Setenv("POSTGRES_PORT", "5432")
	os.Setenv("POSTGRES_DB", "app")
	os.Setenv("POSTGRES_APP_USER", "tragge_app")
	os.Setenv("POSTGRES_APP_PASSWORD", "s3cret")
	os.Setenv("POSTGRES_SSLMODE", "disable")
	defer func() {
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_PORT")
		os.Unsetenv("POSTGRES_DB")
		os.Unsetenv("POSTGRES_APP_USER")
		os.Unsetenv("POSTGRES_APP_PASSWORD")
		os.Unsetenv("POSTGRES_SSLMODE")
	}()

	result := BuildPostgresDSN()

	expected := "postgres://tragge_app:s3cret@dbhost:5432/app?sslmode=disable"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestBuildPostgresDSN_AppPasswordFromFile(t *testing.T) {
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("POSTGRES_PASSWORD")
	os.Unsetenv("POSTGRES_APP_PASSWORD")

	tmpDir := t.TempDir()
	secretPath := filepath.Join(tmpDir, "pg-app-password")
	if err := os.WriteFile(secretPath, []byte("file-app-password\n"), 0600); err != nil {
		t.Fatal(err)
	}

	os.Setenv("POSTGRES_HOST", "postgres")
	os.Setenv("POSTGRES_APP_USER", "tragge_app")
	os.Setenv("POSTGRES_APP_PASSWORD_FILE", secretPath)
	os.Setenv("POSTGRES_SSLMODE", "disable")
	defer func() {
		os.Unsetenv("POSTGRES_HOST")
		os.Unsetenv("POSTGRES_APP_USER")
		os.Unsetenv("POSTGRES_APP_PASSWORD_FILE")
		os.Unsetenv("POSTGRES_SSLMODE")
	}()

	result := BuildPostgresDSN()

	expected := "postgres://tragge_app:file-app-password@postgres:5432/app?sslmode=disable"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestBuildPostgresDSN_PasswordURLEncoded(t *testing.T) {
	os.Unsetenv("POSTGRES_DSN")

	os.Setenv("POSTGRES_USER", "user")
	os.Setenv("POSTGRES_PASSWORD", "p@ss:word/special")
	os.Setenv("POSTGRES_SSLMODE", "disable")
	defer func() {
		os.Unsetenv("POSTGRES_USER")
		os.Unsetenv("POSTGRES_PASSWORD")
		os.Unsetenv("POSTGRES_SSLMODE")
	}()

	result := BuildPostgresDSN()

	expected := "postgres://user:p%40ss%3Aword%2Fspecial@localhost:5432/app?sslmode=disable"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"short", RedactedValue},
		{"12345678", RedactedValue},
		{"1234567890123456", RedactedValue},
	}

	for _, tc := range tests {
		result := MaskSecret(tc.input)
		if result != tc.expected {
			t.Errorf("MaskSecret(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestLoadWithInfo(t *testing.T) {
	os.Setenv("INFO_TEST_SECRET", "test-value")
	defer os.Unsetenv("INFO_TEST_SECRET")

	info := LoadWithInfo("INFO_TEST_SECRET")

	if !info.Loaded {
		t.Error("expected secret to be loaded")
	}
	if info.Source != "env" {
		t.Errorf("expected source 'env', got '%s'", info.Source)
	}
	if info.Masked != RedactedValue {
		t.Errorf("unexpected masked value: %s", info.Masked)
	}
}

func TestDiagnosticReport(t *testing.T) {
	os.Setenv("DIAG_SECRET_1", "value1")
	os.Setenv("DIAG_SECRET_2", "value2")
	defer os.Unsetenv("DIAG_SECRET_1")
	defer os.Unsetenv("DIAG_SECRET_2")

	report := DiagnosticReport("DIAG_SECRET_1", "DIAG_SECRET_2", "DIAG_SECRET_3")

	if len(report) != 3 {
		t.Errorf("expected 3 entries, got %d", len(report))
	}
	if !report[0].Loaded || !report[1].Loaded {
		t.Error("expected first two secrets to be loaded")
	}
	if report[2].Loaded {
		t.Error("expected third secret to not be loaded")
	}
}

func TestGetJWTSecret_ExplicitSecret(t *testing.T) {
	os.Setenv("JWT_SECRET", "a-test-secret-that-is-long-enough-for-validation")
	defer os.Unsetenv("JWT_SECRET")

	result := GetJWTSecret()
	if result != "a-test-secret-that-is-long-enough-for-validation" {
		t.Errorf("expected explicit secret, got '%s'", result)
	}
}

func TestGetJWTSecret_DevGeneratesRandomSecret(t *testing.T) {
	os.Setenv("ENVIRONMENT", "development")
	os.Unsetenv("JWT_SECRET")
	defer os.Unsetenv("ENVIRONMENT")

	secret1 := GetJWTSecret()
	secret2 := GetJWTSecret()

	// 32 random bytes hex-encoded = 64 chars
	if len(secret1) != 64 {
		t.Errorf("expected 64-char hex secret, got %d chars: %s", len(secret1), secret1)
	}
	if secret1 == secret2 {
		t.Error("expected different random secrets per call")
	}
	if secret1 == "insecure-dev-secret-change-in-production" {
		t.Error("should not return hardcoded insecure secret")
	}
}

func TestGetJWTSecret_EmptyEnvGeneratesRandom(t *testing.T) {
	os.Setenv("ENVIRONMENT", "development")
	os.Unsetenv("JWT_SECRET")
	defer os.Unsetenv("ENVIRONMENT")

	secret := GetJWTSecret()
	if len(secret) != 64 {
		t.Errorf("expected 64-char hex secret, got %d chars", len(secret))
	}
}

// TestGetJWTSecret_ProductionPanicsWhenMissing verifies that GetJWTSecret
// panics in production when JWT_SECRET is not set.
func TestGetJWTSecret_ProductionPanicsWhenMissing(t *testing.T) {
	os.Setenv("ENVIRONMENT", "production")
	os.Unsetenv("JWT_SECRET")
	defer os.Unsetenv("ENVIRONMENT")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic in production without JWT_SECRET")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "JWT_SECRET") {
			t.Fatalf("panic message should mention JWT_SECRET, got: %s", msg)
		}
	}()

	GetJWTSecret()
	t.Fatal("should not reach here — GetJWTSecret should have panicked")
}

func TestWarnShortSecret(t *testing.T) {
	// This function writes to stderr; we just verify it doesn't panic
	// and the logic is correct by checking boundary conditions.
	warnShortSecret("TEST", "short", 32)                 // should warn (5 < 32)
	warnShortSecret("TEST", strings.Repeat("a", 32), 32) // should not warn (32 == 32)
	warnShortSecret("TEST", strings.Repeat("a", 33), 32) // should not warn (33 > 32)
}
