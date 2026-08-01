package kyc

import (
	"testing"
	"time"
)

func TestVerificationResult_CanWithdraw(t *testing.T) {
	tests := []struct {
		name     string
		verified bool
		want     bool
	}{
		{"verified user can withdraw", true, true},
		{"unverified user cannot withdraw", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &VerificationResult{Verified: tt.verified}
			if got := r.CanWithdraw(); got != tt.want {
				t.Errorf("CanWithdraw() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKYCRequiredError_Error(t *testing.T) {
	err := &KYCRequiredError{
		Status:  StatusPending,
		Message: "KYC verification required",
	}
	if err.Error() != "KYC verification required" {
		t.Errorf("Error() = %q, want %q", err.Error(), "KYC verification required")
	}
}

func TestKYCExpiredError_Error(t *testing.T) {
	err := &KYCExpiredError{
		ExpiredAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Message:   "KYC expired",
	}
	if err.Error() != "KYC expired" {
		t.Errorf("Error() = %q, want %q", err.Error(), "KYC expired")
	}
}

func TestStatus_Values(t *testing.T) {
	// Verify status constants match expected string values
	tests := []struct {
		status Status
		want   string
	}{
		{StatusNone, "none"},
		{StatusPending, "pending"},
		{StatusUnderReview, "under_review"},
		{StatusVerified, "verified"},
		{StatusRejected, "rejected"},
		{StatusExpired, "expired"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("Status %v = %q, want %q", tt.status, string(tt.status), tt.want)
		}
	}
}

func TestNewJibitKYCProvider_DefaultBaseURL(t *testing.T) {
	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "key",
		SecretKey: "secret",
	})
	if p.baseURL != defaultJibitBaseURL {
		t.Errorf("expected default base URL %q, got %q", defaultJibitBaseURL, p.baseURL)
	}
}

func TestNewJibitKYCProvider_CustomBaseURL(t *testing.T) {
	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "key",
		SecretKey: "secret",
		BaseURL:   "https://custom.example.com",
	})
	if p.baseURL != "https://custom.example.com" {
		t.Errorf("expected custom base URL, got %q", p.baseURL)
	}
}
