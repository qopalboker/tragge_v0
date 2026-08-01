package handlers

import "testing"

func TestGenerateDescription(t *testing.T) {
	tests := []struct {
		entryType string
		provider  string
		want      string
	}{
		{"deposit", "jibit", "Deposit via Jibit"},
		{"deposit", "nowpayments", "Deposit via NOWPayments"},
		{"deposit", "stripe", "Deposit via Stripe"},
		{"deposit", "", "Deposit"},
		{"withdrawal", "jibit", "Withdrawal via Jibit"},
		{"withdrawal", "nowpayments", "Withdrawal via NOWPayments"},
		{"withdrawal", "", "Withdrawal"},
		{"other", "", "other"},
	}

	for _, tc := range tests {
		t.Run(tc.entryType+"_"+tc.provider, func(t *testing.T) {
			got := generateDescription(tc.entryType, tc.provider)
			if got != tc.want {
				t.Errorf("generateDescription(%q, %q) = %q, want %q",
					tc.entryType, tc.provider, got, tc.want)
			}
		})
	}
}

func TestMapPaymentMethod(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"jibit", "Bank Transfer"},
		{"nowpayments", "Cryptocurrency"},
		{"stripe", "Credit Card"},
		{"", ""},
		{"unknown", ""},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			got := mapPaymentMethod(tc.provider)
			if got != tc.want {
				t.Errorf("mapPaymentMethod(%q) = %q, want %q", tc.provider, got, tc.want)
			}
		})
	}
}

func TestValidPaymentStatuses(t *testing.T) {
	// Known statuses should be accepted
	known := []string{"pending", "processing", "succeeded", "failed", "refunded", "expired", "cancelled"}
	for _, s := range known {
		if !validPaymentStatuses[s] {
			t.Errorf("validPaymentStatuses[%q] should be true", s)
		}
	}

	// Unknown statuses should be rejected
	unknown := []string{"invalid", "complete", "done", ""}
	for _, s := range unknown {
		if validPaymentStatuses[s] {
			t.Errorf("validPaymentStatuses[%q] should be false", s)
		}
	}
}
