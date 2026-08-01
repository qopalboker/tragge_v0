package kyc

import (
	"errors"
	"strings"
	"unicode"
)

var (
	// ErrInvalidNationalCode is returned when an Iranian national code fails validation.
	ErrInvalidNationalCode = errors.New("kyc: invalid Iranian national code")
	// ErrInvalidPhoneNumber is returned when an Iranian phone number fails validation.
	ErrInvalidPhoneNumber = errors.New("kyc: invalid Iranian phone number")
	// ErrInvalidCardNumber is returned when a bank card number fails validation.
	ErrInvalidCardNumber = errors.New("kyc: invalid bank card number")
	// ErrEmptyImage is returned when image data is empty.
	ErrEmptyImage = errors.New("kyc: image data is empty")
)

// ValidateIranianNationalCode validates a 10-digit Iranian national code
// using the standard checksum algorithm (weighted sum mod 11).
func ValidateIranianNationalCode(code string) error {
	if len(code) != 10 {
		ValidationFailures.WithLabelValues("national_code", "wrong_length").Inc()
		return ErrInvalidNationalCode
	}
	for _, r := range code {
		if !unicode.IsDigit(r) {
			ValidationFailures.WithLabelValues("national_code", "non_digit").Inc()
			return ErrInvalidNationalCode
		}
	}

	// Reject all-same-digit codes (e.g. "0000000000", "1111111111").
	allSame := true
	for i := 1; i < len(code); i++ {
		if code[i] != code[0] {
			allSame = false
			break
		}
	}
	if allSame {
		ValidationFailures.WithLabelValues("national_code", "all_same").Inc()
		return ErrInvalidNationalCode
	}

	// Checksum: sum of digit[i] * (10 - i) for i = 0..8, then mod 11.
	// If remainder < 2: check digit must equal remainder.
	// If remainder >= 2: check digit must equal 11 - remainder.
	digits := make([]int, 10)
	for i, r := range code {
		digits[i] = int(r - '0')
	}

	sum := 0
	for i := 0; i < 9; i++ {
		sum += digits[i] * (10 - i)
	}
	remainder := sum % 11
	checkDigit := digits[9]

	if remainder < 2 {
		if checkDigit != remainder {
			ValidationFailures.WithLabelValues("national_code", "bad_checksum").Inc()
			return ErrInvalidNationalCode
		}
	} else {
		if checkDigit != 11-remainder {
			ValidationFailures.WithLabelValues("national_code", "bad_checksum").Inc()
			return ErrInvalidNationalCode
		}
	}

	return nil
}

// ValidateIranianPhoneNumber validates an Iranian mobile phone number.
// Accepted formats:
//   - 09XXXXXXXXX  (11 digits, starts with 09)
//   - +989XXXXXXXX (13 chars with leading +, starts with +989)
//   - 989XXXXXXXX  (12 digits, starts with 989)
func ValidateIranianPhoneNumber(phone string) error {
	cleaned := strings.TrimSpace(phone)

	if len(cleaned) == 0 {
		ValidationFailures.WithLabelValues("phone", "empty").Inc()
		return ErrInvalidPhoneNumber
	}

	switch {
	case strings.HasPrefix(cleaned, "+989"):
		if len(cleaned) != 13 {
			ValidationFailures.WithLabelValues("phone", "wrong_length").Inc()
			return ErrInvalidPhoneNumber
		}
		// Validate digits after the +
		for _, r := range cleaned[1:] {
			if !unicode.IsDigit(r) {
				ValidationFailures.WithLabelValues("phone", "non_digit").Inc()
				return ErrInvalidPhoneNumber
			}
		}
	case strings.HasPrefix(cleaned, "989"):
		if len(cleaned) != 12 {
			ValidationFailures.WithLabelValues("phone", "wrong_length").Inc()
			return ErrInvalidPhoneNumber
		}
		for _, r := range cleaned {
			if !unicode.IsDigit(r) {
				ValidationFailures.WithLabelValues("phone", "non_digit").Inc()
				return ErrInvalidPhoneNumber
			}
		}
	case strings.HasPrefix(cleaned, "09"):
		if len(cleaned) != 11 {
			ValidationFailures.WithLabelValues("phone", "wrong_length").Inc()
			return ErrInvalidPhoneNumber
		}
		for _, r := range cleaned {
			if !unicode.IsDigit(r) {
				ValidationFailures.WithLabelValues("phone", "non_digit").Inc()
				return ErrInvalidPhoneNumber
			}
		}
	default:
		ValidationFailures.WithLabelValues("phone", "wrong_prefix").Inc()
		return ErrInvalidPhoneNumber
	}

	return nil
}

// ValidateCardNumber validates a 16-digit bank card number using the Luhn algorithm.
// Spaces and dashes are stripped before validation.
func ValidateCardNumber(cardNumber string) error {
	cleaned := strings.ReplaceAll(strings.TrimSpace(cardNumber), " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")

	if len(cleaned) != 16 {
		ValidationFailures.WithLabelValues("card_number", "wrong_length").Inc()
		return ErrInvalidCardNumber
	}
	for _, r := range cleaned {
		if !unicode.IsDigit(r) {
			ValidationFailures.WithLabelValues("card_number", "non_digit").Inc()
			return ErrInvalidCardNumber
		}
	}

	// Luhn algorithm: starting from the rightmost digit, double every second digit.
	sum := 0
	for i := 0; i < 16; i++ {
		d := int(cleaned[i] - '0')
		// For a 16-digit card number:
		// - Even indices from left (0,2,4,...,14) map to positions 16,14,12,...,2 from right (1-indexed)
		// - These are EVEN positions from right, which are the ones doubled in the Luhn algorithm
		// - This works because len(16) is even, so i%2==0 from left aligns with every-other from right
		if i%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	if sum%10 != 0 {
		ValidationFailures.WithLabelValues("card_number", "bad_luhn").Inc()
		return ErrInvalidCardNumber
	}

	return nil
}
