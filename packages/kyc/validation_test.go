package kyc

import (
	"errors"
	"testing"
)

func TestValidateIranianNationalCode(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantErr error
	}{
		// Valid codes
		{"valid_code_0012345687", "0012345687", nil},
		{"valid_code_0499370899", "0499370899", nil},
		{"valid_code_1234567891", "1234567891", nil},

		// Invalid: wrong length
		{"too_short", "12345", ErrInvalidNationalCode},
		{"too_long", "12345678901", ErrInvalidNationalCode},
		{"empty", "", ErrInvalidNationalCode},

		// Invalid: non-digit characters
		{"alpha_chars", "123456789a", ErrInvalidNationalCode},
		{"with_space", "012345678 ", ErrInvalidNationalCode},

		// Invalid: all same digits
		{"all_zeros", "0000000000", ErrInvalidNationalCode},
		{"all_ones", "1111111111", ErrInvalidNationalCode},
		{"all_nines", "9999999999", ErrInvalidNationalCode},

		// Invalid: bad checksum
		{"bad_checksum", "0012345688", ErrInvalidNationalCode},
		{"bad_checksum_2", "1234567890", ErrInvalidNationalCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIranianNationalCode(tt.code)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateIranianNationalCode(%q) = %v, want nil", tt.code, err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("ValidateIranianNationalCode(%q) = %v, want %v", tt.code, err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidateIranianPhoneNumber(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		wantErr error
	}{
		// Valid formats
		{"local_format", "09123456789", nil},
		{"international_plus", "+989123456789", nil},
		{"international_no_plus", "989123456789", nil},

		// Invalid: wrong prefix
		{"wrong_prefix_08", "08123456789", ErrInvalidPhoneNumber},
		{"wrong_prefix_07", "07123456789", ErrInvalidPhoneNumber},
		{"no_prefix", "1234567890", ErrInvalidPhoneNumber},

		// Invalid: wrong length
		{"local_too_short", "0912345678", ErrInvalidPhoneNumber},
		{"local_too_long", "091234567890", ErrInvalidPhoneNumber},
		{"intl_plus_too_short", "+98912345678", ErrInvalidPhoneNumber},
		{"intl_plus_too_long", "+9891234567890", ErrInvalidPhoneNumber},
		{"intl_no_plus_too_short", "98912345678", ErrInvalidPhoneNumber},
		{"intl_no_plus_too_long", "9891234567890", ErrInvalidPhoneNumber},

		// Invalid: non-digit characters
		{"alpha_in_local", "0912345678a", ErrInvalidPhoneNumber},
		{"alpha_in_intl", "+98912345678a", ErrInvalidPhoneNumber},

		// Invalid: empty
		{"empty", "", ErrInvalidPhoneNumber},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIranianPhoneNumber(tt.phone)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateIranianPhoneNumber(%q) = %v, want nil", tt.phone, err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("ValidateIranianPhoneNumber(%q) = %v, want %v", tt.phone, err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidateCardNumber(t *testing.T) {
	tests := []struct {
		name    string
		card    string
		wantErr error
	}{
		// Valid Luhn-16 card numbers
		{"valid_visa", "4539578763621486", nil},
		{"valid_with_spaces", "4539 5787 6362 1486", nil},
		{"valid_with_dashes", "4539-5787-6362-1486", nil},

		// Invalid: wrong length
		{"too_short", "453957876362148", ErrInvalidCardNumber},
		{"too_long", "45395787636214861", ErrInvalidCardNumber},
		{"empty", "", ErrInvalidCardNumber},

		// Invalid: non-digit
		{"alpha_chars", "453957876362148a", ErrInvalidCardNumber},

		// Invalid: bad Luhn
		{"bad_luhn", "4539578763621487", ErrInvalidCardNumber},
		{"all_zeros", "0000000000000000", nil}, // all zeros passes Luhn
		{"sequential", "1234567890123456", ErrInvalidCardNumber},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCardNumber(tt.card)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateCardNumber(%q) = %v, want nil", tt.card, err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("ValidateCardNumber(%q) = %v, want %v", tt.card, err, tt.wantErr)
				}
			}
		})
	}
}
