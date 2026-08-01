package providers

import (
	"math"
	"testing"
)

// TestFloatToCentsRounding verifies that math.Round correctly handles
// floating-point edge cases when converting dollar amounts to cents.
// This guards against BUG #313 (float64 precision loss).
func TestFloatToCentsRounding(t *testing.T) {
	tests := []struct {
		name     string
		amount   float64
		wantCent int64
	}{
		{"exact", 10.00, 1000},
		{"typical", 19.99, 1999},
		{"one cent", 0.01, 1},
		{"zero", 0.00, 0},
		{"large", 99999.99, 9999999},
		{"half cent rounds up", 10.005, 1001},
		{"three decimals round down", 10.004, 1000},
		{"known float issue 19.99", 19.99, 1999},
		{"known float issue 0.1+0.2", 0.1 + 0.2, 30},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := int64(math.Round(tc.amount * 100))
			if got != tc.wantCent {
				t.Errorf("int64(math.Round(%v * 100)) = %d, want %d", tc.amount, got, tc.wantCent)
			}
		})
	}
}

func TestNowPaymentsMapStatus(t *testing.T) {
	n := &NowPayments{}

	tests := []struct {
		input    string
		expected PaymentStatus
	}{
		{"waiting", PaymentStatusWaiting},
		{"confirming", PaymentStatusConfirming},
		{"confirmed", PaymentStatusConfirmed},
		{"sending", PaymentStatusSending},
		{"finished", PaymentStatusFinished},
		{"failed", PaymentStatusFailed},
		{"refunded", PaymentStatusRefunded},
		{"expired", PaymentStatusExpired},
		{"partially_paid", PaymentStatusConfirming},
		{"unknown_status", PaymentStatusPending},
		{"", PaymentStatusPending},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := n.mapStatus(tc.input)
			if got != tc.expected {
				t.Errorf("mapStatus(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
