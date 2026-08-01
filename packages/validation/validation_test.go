package validation

import (
	"math"
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		wantOk   bool
		wantNorm string
	}{
		{"valid email", "user@example.com", true, "user@example.com"},
		{"valid email with subdomain", "user@mail.example.com", true, "user@mail.example.com"},
		{"uppercase email normalized", "USER@EXAMPLE.COM", true, "user@example.com"},
		{"email with whitespace", "  user@example.com  ", true, "user@example.com"},
		{"invalid - no @", "userexample.com", false, "userexample.com"},
		{"invalid - no domain", "user@", false, "user@"},
		{"invalid - no local part", "@example.com", false, "@example.com"},
		{"empty string", "", false, ""},
		{"invalid - double dots", "user..name@example.com", false, "user..name@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNorm, gotOk := ValidateEmail(tt.email)
			if gotOk != tt.wantOk {
				t.Errorf("ValidateEmail() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
			if gotNorm != tt.wantNorm {
				t.Errorf("ValidateEmail() gotNorm = %v, want %v", gotNorm, tt.wantNorm)
			}
		})
	}
}

func TestValidateUUID(t *testing.T) {
	tests := []struct {
		name     string
		uuid     string
		wantOk   bool
		wantNorm string
	}{
		{"valid uuid", "550e8400-e29b-41d4-a716-446655440000", true, "550e8400-e29b-41d4-a716-446655440000"},
		{"uppercase uuid", "550E8400-E29B-41D4-A716-446655440000", true, "550e8400-e29b-41d4-a716-446655440000"},
		{"uuid with whitespace", "  550e8400-e29b-41d4-a716-446655440000  ", true, "550e8400-e29b-41d4-a716-446655440000"},
		{"invalid - too short", "550e8400-e29b-41d4-a716", false, "550e8400-e29b-41d4-a716"},
		{"invalid - wrong characters", "550e8400-xxxx-41d4-a716-446655440000", false, "550e8400-xxxx-41d4-a716-446655440000"},
		{"empty string", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNorm, gotOk := ValidateUUID(tt.uuid)
			if gotOk != tt.wantOk {
				t.Errorf("ValidateUUID() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
			if gotNorm != tt.wantNorm {
				t.Errorf("ValidateUUID() gotNorm = %v, want %v", gotNorm, tt.wantNorm)
			}
		})
	}
}

func TestValidatePrice(t *testing.T) {
	constraints := DefaultPriceConstraints()

	tests := []struct {
		name    string
		price   float64
		wantOk  bool
		wantMsg string
	}{
		{"valid price", 150.00, true, ""},
		{"valid minimum", 0.0001, true, ""},
		{"valid large price", 999_999_999.99, true, ""},
		{"invalid - zero", 0.0, false, "price cannot be zero"},
		{"invalid - negative", -10.00, false, "price cannot be negative"},
		{"invalid - too small", 0.00001, false, "price must be at least 0.0001"},
		{"invalid - too large", 2_000_000_000, false, "price cannot exceed 1000000000.00"},
		{"invalid - NaN", math.NaN(), false, "price cannot be NaN"},
		{"invalid - positive Inf", math.Inf(1), false, "price cannot be infinite"},
		{"invalid - negative Inf", math.Inf(-1), false, "price cannot be infinite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, gotOk, gotMsg := ValidatePrice(tt.price, constraints)
			if gotOk != tt.wantOk {
				t.Errorf("ValidatePrice() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
			if gotMsg != tt.wantMsg {
				t.Errorf("ValidatePrice() gotMsg = %v, want %v", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestValidateQuantity(t *testing.T) {
	constraints := DefaultQuantityConstraints()

	tests := []struct {
		name    string
		qty     int64
		wantOk  bool
		wantMsg string
	}{
		{"valid quantity", 100, true, ""},
		{"valid minimum", 1, true, ""},
		{"valid large quantity", 999_999_999, true, ""},
		{"invalid - zero", 0, false, "quantity cannot be zero"},
		{"invalid - negative", -10, false, "quantity cannot be negative"},
		{"invalid - too large", 2_000_000_000, false, "quantity cannot exceed 1000000000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, gotOk, gotMsg := ValidateQuantity(tt.qty, constraints)
			if gotOk != tt.wantOk {
				t.Errorf("ValidateQuantity() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
			if gotMsg != tt.wantMsg {
				t.Errorf("ValidateQuantity() gotMsg = %v, want %v", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestValidateSymbol(t *testing.T) {
	tests := []struct {
		name     string
		symbol   string
		wantOk   bool
		wantNorm string
	}{
		{"valid symbol", "AAPL", true, "AAPL"},
		{"lowercase normalized", "aapl", true, "AAPL"},
		{"symbol with whitespace", "  AAPL  ", true, "AAPL"},
		{"valid long symbol", "GOOGL", true, "GOOGL"},
		{"single char symbol", "A", true, "A"},
		{"invalid - too long", "VERYLONGSYMBOL", false, "VERYLONGSYMBOL"},
		{"invalid - numbers", "AAPL1", false, "AAPL1"},
		{"valid forex pair", "EUR/USD", true, "EUR/USD"},
		{"valid crypto pair", "BTC/USD", true, "BTC/USD"},
		{"valid metal pair", "XAU/USD", true, "XAU/USD"},
		{"lowercase forex pair", "eur/usd", true, "EUR/USD"},
		{"invalid double slash", "EUR//USD", false, "EUR//USD"},
		{"invalid trailing slash", "EUR/", false, "EUR/"},
		{"invalid - special chars", "AA-PL", false, "AA-PL"},
		{"empty string", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNorm, gotOk := ValidateSymbol(tt.symbol)
			if gotOk != tt.wantOk {
				t.Errorf("ValidateSymbol() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
			if gotNorm != tt.wantNorm {
				t.Errorf("ValidateSymbol() gotNorm = %v, want %v", gotNorm, tt.wantNorm)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	defaultConstraints := DefaultPasswordConstraints()
	strictConstraints := StrictPasswordConstraints()

	tests := []struct {
		name        string
		password    string
		constraints PasswordConstraints
		wantOk      bool
	}{
		{"default - valid complex", "MyP@ssw0rd1", defaultConstraints, true},
		{"default - valid long complex", "Veryl0ng!password", defaultConstraints, true},
		{"default - too short", "P@ss1!", defaultConstraints, false},
		{"default - missing uppercase", "password1!", defaultConstraints, false},
		{"default - missing lowercase", "PASSWORD1!", defaultConstraints, false},
		{"default - missing digit", "Password!!", defaultConstraints, false},
		{"default - missing special", "Password123", defaultConstraints, false},
		{"default - all lowercase no digit", "aaaaaaaaaa", defaultConstraints, false},
		{"strict - valid complex", "MyP@ssw0rd123!", strictConstraints, true},
		{"strict - missing uppercase", "myp@ssw0rd123!", strictConstraints, false},
		{"strict - missing lowercase", "MYP@SSW0RD123!", strictConstraints, false},
		{"strict - missing digit", "MyP@ssword!!!", strictConstraints, false},
		{"strict - missing special", "MyPassw0rd123", strictConstraints, false},
		{"strict - too short", "MyP@ss1!", strictConstraints, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOk, _ := ValidatePassword(tt.password, tt.constraints)
			if gotOk != tt.wantOk {
				t.Errorf("ValidatePassword() gotOk = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func TestValidateIranPhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{"standard format", "09121234567", "+989121234567", false},
		{"with +98 prefix", "+989121234567", "+989121234567", false},
		{"with 98 prefix", "989121234567", "+989121234567", false},
		{"with 0098 prefix", "00989121234567", "+989121234567", false},
		{"with dashes", "0912-123-4567", "+989121234567", false},
		{"with spaces", "0912 123 4567", "+989121234567", false},
		{"irancell", "09351234567", "+989351234567", false},
		{"rightel", "09221234567", "+989221234567", false},
		{"invalid - too short", "091212345", "", true},
		{"invalid - too long", "091212345678", "", true},
		{"invalid - landline", "02112345678", "", true},
		{"invalid - random", "1234567890", "", true},
		{"empty string", "", "", true},
		{"invalid - letters", "0912abcdefg", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateIranPhone(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for input %q, got result %q", tt.input, result)
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error for input %q: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("For input %q: expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestValidator(t *testing.T) {
	t.Run("accumulates errors", func(t *testing.T) {
		v := New()

		v.Email("email", "invalid")
		v.UUID("user_id", "not-a-uuid")
		v.Symbol("symbol", "TOO_LONG_SYMBOL_NAME")

		if !v.HasErrors() {
			t.Error("Expected errors to be recorded")
		}

		errors := v.Errors()
		if len(errors) != 3 {
			t.Errorf("Expected 3 errors, got %d", len(errors))
		}
	})

	t.Run("no errors for valid input", func(t *testing.T) {
		v := New()

		v.Email("email", "user@example.com")
		v.UUID("user_id", "550e8400-e29b-41d4-a716-446655440000")
		v.Symbol("symbol", "AAPL")

		if v.HasErrors() {
			t.Errorf("Expected no errors, got %v", v.Errors())
		}
	})

	t.Run("reset clears errors", func(t *testing.T) {
		v := New()

		v.Email("email", "invalid")
		if !v.HasErrors() {
			t.Error("Expected errors before reset")
		}

		v.Reset()
		if v.HasErrors() {
			t.Error("Expected no errors after reset")
		}
	})
}
