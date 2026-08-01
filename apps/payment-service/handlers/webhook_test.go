package handlers

import (
	"testing"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
)

func TestAmountsMatch(t *testing.T) {
	tests := []struct {
		name     string
		actual   int64
		expected int64
		want     bool
	}{
		{"exact match", 5000, 5000, true},
		{"zero match", 0, 0, true},
		{"small diff within pct", 5050, 5000, true},            // 1% of 5000 = 50
		{"small diff over pct", 5051, 5000, false},              // 51 > 50
		{"large amount within pct but over cap", 1050000, 1000000, false}, // 1% = 10000 > cap 1000
		{"large amount within cap", 1000500, 1000000, true},     // 500 < cap 1000
		{"large amount at cap", 1001000, 1000000, true},         // 1000 = cap
		{"large amount over cap", 1001001, 1000000, false},      // 1001 > cap
		{"negative expected", 100, 0, false},
		{"negative difference within tolerance", 99, 100, true}, // diff=1, 1% of 100=1
		{"both zero", 0, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := amountsMatch(tc.actual, tc.expected)
			if got != tc.want {
				t.Errorf("amountsMatch(%d, %d) = %v, want %v", tc.actual, tc.expected, got, tc.want)
			}
		})
	}
}

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		status   string
		terminal bool
	}{
		{"succeeded", true},
		{"failed", true},
		{"refunded", true},
		{"expired", true},
		{"pending", false},
		{"processing", false},
		{"", false},
		{"unknown", false},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			got := isTerminalStatus(tc.status)
			if got != tc.terminal {
				t.Errorf("isTerminalStatus(%q) = %v, want %v", tc.status, got, tc.terminal)
			}
		})
	}
}

func TestMapProviderStatusToIntent(t *testing.T) {
	tests := []struct {
		input    providers.PaymentStatus
		expected string
	}{
		{providers.PaymentStatusPending, "pending"},
		{providers.PaymentStatusWaiting, "pending"},
		{providers.PaymentStatusConfirming, "processing"},
		{providers.PaymentStatusSending, "processing"},
		{providers.PaymentStatusFinished, "succeeded"},
		{providers.PaymentStatusConfirmed, "succeeded"},
		{providers.PaymentStatusFailed, "failed"},
		{providers.PaymentStatusRefunded, "refunded"},
		{providers.PaymentStatusExpired, "expired"},
		{providers.PaymentStatus("unknown"), "pending"},
	}

	for _, tc := range tests {
		t.Run(string(tc.input), func(t *testing.T) {
			got := mapProviderStatusToIntent(tc.input)
			if got != tc.expected {
				t.Errorf("mapProviderStatusToIntent(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}
