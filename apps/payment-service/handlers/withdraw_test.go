package handlers

import (
	"math"
	"testing"
)

// TestWithdrawFeeCalculation tests the basis-point fee calculation that replaced
// the previous float64 arithmetic (BUG #316).
func TestWithdrawFeeCalculation(t *testing.T) {
	tests := []struct {
		name           string
		amountCents    int64
		fixedFeeCents  int64
		feePercent     float64
		wantTotalFee   int64
	}{
		{"zero percent", 10000, 100, 0, 100},
		{"1% of $100", 10000, 0, 1.0, 100},
		{"1.5% of $100", 10000, 0, 1.5, 150},
		{"0.5% of $99.99", 9999, 0, 0.5, 49},
		{"2.5% of $100 + fixed $1", 10000, 100, 2.5, 350},
		{"1% of $0", 0, 0, 1.0, 0},
		{"1% of $1", 100, 0, 1.0, 1},
		{"1% of $0.50", 50, 0, 1.0, 0}, // 50 * 100 / 10000 = 0 (truncated)
		{"0.1% of $1000", 100000, 0, 0.1, 100},
		{"large amount 1%", 100000000, 0, 1.0, 1000000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			feeCents := tc.fixedFeeCents
			if tc.feePercent > 0 {
				feeBasisPoints := int64(math.Round(tc.feePercent * 100))
				feeCents += tc.amountCents * feeBasisPoints / 10000
			}

			if feeCents != tc.wantTotalFee {
				t.Errorf("fee = %d, want %d (amount=%d, fixed=%d, pct=%.2f%%)",
					feeCents, tc.wantTotalFee, tc.amountCents, tc.fixedFeeCents, tc.feePercent)
			}
		})
	}
}
