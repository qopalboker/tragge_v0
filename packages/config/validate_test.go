package config

import (
	"os"
	"strings"
	"testing"
)

func TestIsProduction(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{
		{"production", true},
		{"staging", true},
		{"Production", true},
		{"PRODUCTION", true},
		{"Staging", true},
		{"STAGING", true},
		{"development", false},
		{"", true},
		{"test", false},
		{"local", false},
	}
	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			os.Setenv("ENVIRONMENT", tt.env)
			defer os.Unsetenv("ENVIRONMENT")
			if got := IsProduction(); got != tt.want {
				t.Errorf("IsProduction() with ENVIRONMENT=%q = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestValidateRequired_AllPresent(t *testing.T) {
	os.Setenv("TEST_CONFIG_A", "value_a")
	os.Setenv("TEST_CONFIG_B", "value_b")
	defer func() {
		os.Unsetenv("TEST_CONFIG_A")
		os.Unsetenv("TEST_CONFIG_B")
	}()

	missing := ValidateRequired("TEST_CONFIG_A", "TEST_CONFIG_B")
	if len(missing) != 0 {
		t.Errorf("ValidateRequired() = %v, want empty slice", missing)
	}
}

func TestValidateRequired_SomeMissing(t *testing.T) {
	os.Setenv("TEST_CONFIG_A", "value_a")
	os.Unsetenv("TEST_CONFIG_B")
	os.Unsetenv("TEST_CONFIG_C")
	defer os.Unsetenv("TEST_CONFIG_A")

	missing := ValidateRequired("TEST_CONFIG_A", "TEST_CONFIG_B", "TEST_CONFIG_C")
	if len(missing) != 2 {
		t.Fatalf("ValidateRequired() returned %d missing, want 2", len(missing))
	}
	if missing[0] != "TEST_CONFIG_B" || missing[1] != "TEST_CONFIG_C" {
		t.Errorf("ValidateRequired() = %v, want [TEST_CONFIG_B, TEST_CONFIG_C]", missing)
	}
}

func TestValidateRequired_AllMissing(t *testing.T) {
	os.Unsetenv("TEST_CONFIG_X")
	os.Unsetenv("TEST_CONFIG_Y")

	missing := ValidateRequired("TEST_CONFIG_X", "TEST_CONFIG_Y")
	if len(missing) != 2 {
		t.Errorf("ValidateRequired() returned %d missing, want 2", len(missing))
	}
}

func TestValidateRequired_NoKeys(t *testing.T) {
	missing := ValidateRequired()
	if missing != nil {
		t.Errorf("ValidateRequired() with no keys = %v, want nil", missing)
	}
}

func TestValidateAnyRequired_OneSet(t *testing.T) {
	os.Setenv("TEST_CONFIG_P", "value")
	os.Unsetenv("TEST_CONFIG_Q")
	defer os.Unsetenv("TEST_CONFIG_P")

	if !ValidateAnyRequired("TEST_CONFIG_P", "TEST_CONFIG_Q") {
		t.Error("ValidateAnyRequired() should return true when at least one var is set")
	}
}

func TestValidateAnyRequired_NoneSet(t *testing.T) {
	os.Unsetenv("TEST_CONFIG_P")
	os.Unsetenv("TEST_CONFIG_Q")

	if ValidateAnyRequired("TEST_CONFIG_P", "TEST_CONFIG_Q") {
		t.Error("ValidateAnyRequired() should return false when no vars are set")
	}
}

func TestValidateAnyRequired_AllSet(t *testing.T) {
	os.Setenv("TEST_CONFIG_P", "val1")
	os.Setenv("TEST_CONFIG_Q", "val2")
	defer func() {
		os.Unsetenv("TEST_CONFIG_P")
		os.Unsetenv("TEST_CONFIG_Q")
	}()

	if !ValidateAnyRequired("TEST_CONFIG_P", "TEST_CONFIG_Q") {
		t.Error("ValidateAnyRequired() should return true when all vars are set")
	}
}

func TestMustBeSet_Development(t *testing.T) {
	// In development, MustBeSet should not exit even if vars are missing
	os.Setenv("ENVIRONMENT", "development")
	os.Unsetenv("TEST_MUST_SET_VAR")
	defer func() {
		os.Unsetenv("ENVIRONMENT")
	}()

	// This should not panic or exit
	MustBeSet("TEST_MUST_SET_VAR")
}

func TestMustBeSet_NoEnvironment(t *testing.T) {
	// With no ENVIRONMENT set, defaults to production — MustBeSet should panic
	os.Unsetenv("ENVIRONMENT")
	os.Unsetenv("TEST_MUST_SET_VAR")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustBeSet should panic when ENVIRONMENT is unset (defaults to production)")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value should be a string, got %T", r)
		}
		if !strings.Contains(msg, "TEST_MUST_SET_VAR") {
			t.Errorf("panic message should contain variable name, got: %s", msg)
		}
	}()

	MustBeSet("TEST_MUST_SET_VAR")
}

func TestMustBeSetAny_Development(t *testing.T) {
	// In development, MustBeSetAny should not exit even if vars are missing
	os.Setenv("ENVIRONMENT", "development")
	os.Unsetenv("TEST_ANY_A")
	os.Unsetenv("TEST_ANY_B")
	defer func() {
		os.Unsetenv("ENVIRONMENT")
	}()

	// This should not panic or exit
	MustBeSetAny("test connection", "TEST_ANY_A", "TEST_ANY_B")
}

func TestMustBeSet_Production_Panics(t *testing.T) {
	os.Setenv("ENVIRONMENT", "production")
	os.Unsetenv("TEST_MUST_PANIC_VAR")
	defer os.Unsetenv("ENVIRONMENT")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustBeSet should panic in production when vars are missing")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value should be a string, got %T", r)
		}
		if !strings.Contains(msg, "TEST_MUST_PANIC_VAR") {
			t.Errorf("panic message should contain variable name, got: %s", msg)
		}
		if !strings.Contains(msg, "FATAL") {
			t.Errorf("panic message should contain FATAL prefix, got: %s", msg)
		}
	}()

	MustBeSet("TEST_MUST_PANIC_VAR")
}

func TestMustBeSet_Production_NoPanic(t *testing.T) {
	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("TEST_SET_VAR", "present")
	defer func() {
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("TEST_SET_VAR")
	}()

	// Should NOT panic
	MustBeSet("TEST_SET_VAR")
}

func TestMustBeSetAny_Production_Panics(t *testing.T) {
	os.Setenv("ENVIRONMENT", "production")
	os.Unsetenv("TEST_ANY_X")
	os.Unsetenv("TEST_ANY_Y")
	defer os.Unsetenv("ENVIRONMENT")

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustBeSetAny should panic in production when no vars are set")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value should be a string, got %T", r)
		}
		if !strings.Contains(msg, "FATAL") {
			t.Errorf("panic message should contain FATAL prefix, got: %s", msg)
		}
	}()

	MustBeSetAny("test config", "TEST_ANY_X", "TEST_ANY_Y")
}

func TestMustBeSetAny_Production_NoPanic(t *testing.T) {
	os.Setenv("ENVIRONMENT", "production")
	os.Setenv("TEST_ANY_X", "value")
	os.Unsetenv("TEST_ANY_Y")
	defer func() {
		os.Unsetenv("ENVIRONMENT")
		os.Unsetenv("TEST_ANY_X")
	}()

	// Should NOT panic
	MustBeSetAny("test config", "TEST_ANY_X", "TEST_ANY_Y")
}
