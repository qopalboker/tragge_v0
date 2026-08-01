package providers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"testing"
)

func TestPayment4MapStatus(t *testing.T) {
	p := &Payment4{}

	tests := []struct {
		input    string
		expected PaymentStatus
	}{
		{"PAID", PaymentStatusFinished},
		{"CONFIRMED", PaymentStatusFinished},
		{"PENDING", PaymentStatusPending},
		{"WAITING", PaymentStatusPending},
		{"EXPIRED", PaymentStatusExpired},
		{"FAILED", PaymentStatusFailed},
		{"CANCELLED", PaymentStatusFailed},
		{"PARTIALLY_PAID", PaymentStatusPending},
		{"", PaymentStatusPending},
		{"unknown", PaymentStatusPending},
		// Case insensitivity
		{"paid", PaymentStatusFinished},
		{"confirmed", PaymentStatusFinished},
		{"failed", PaymentStatusFailed},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := p.mapStatus(tc.input)
			if got != tc.expected {
				t.Errorf("mapStatus(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestPayment4VerifySignature(t *testing.T) {
	p := &Payment4{
		apiKey:    "test-key",
		ipnSecret: "test-secret",
	}

	// Build a test payload
	payload := []byte(`{"amount":1.00,"currency":"USD","orderId":"order-123","paymentUid":"pay-456","status":"PAID"}`)

	// Compute expected HMAC-SHA256 with sorted keys (payload is already sorted)
	canonicalJSON := sortedJSON(map[string]interface{}{
		"amount":     1.0,
		"currency":   "USD",
		"orderId":    "order-123",
		"paymentUid": "pay-456",
		"status":     "PAID",
	})
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write([]byte(canonicalJSON))
	validSig := hex.EncodeToString(mac.Sum(nil))

	// Test valid signature
	headers := map[string]string{
		"x-payment4-signature": validSig,
	}
	event, err := p.VerifyWebhook(context.Background(), headers, payload)
	if err != nil {
		t.Fatalf("VerifyWebhook() with valid signature error = %v", err)
	}
	if event.Status != PaymentStatusFinished {
		t.Errorf("expected status finished, got %s", event.Status)
	}
	if event.ProviderPaymentID != "pay-456" {
		t.Errorf("expected paymentUid pay-456, got %s", event.ProviderPaymentID)
	}
	if event.OrderID != "order-123" {
		t.Errorf("expected orderId order-123, got %s", event.OrderID)
	}
	if event.AmountCents != 100 {
		t.Errorf("expected AmountCents 100, got %d", event.AmountCents)
	}

	// Test invalid signature
	headers["x-payment4-signature"] = "invalid-signature"
	_, err = p.VerifyWebhook(context.Background(), headers, payload)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for bad signature, got %v", err)
	}

	// Test missing signature with ipnSecret configured — should fail (signature required)
	headersNoSig := map[string]string{}
	_, err = p.VerifyWebhook(context.Background(), headersNoSig, payload)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature for missing signature when ipnSecret is set, got %v", err)
	}

	// Test missing signature with NO ipnSecret — should succeed
	pNoSecret := &Payment4{
		apiKey: "test-key",
	}
	event, err = pNoSecret.VerifyWebhook(context.Background(), headersNoSig, payload)
	if err != nil {
		t.Fatalf("VerifyWebhook() with missing signature and no ipnSecret should succeed, got error = %v", err)
	}
	if event.Status != PaymentStatusFinished {
		t.Errorf("expected status finished even without signature, got %s", event.Status)
	}
}

func TestPayment4SupportedCurrencies(t *testing.T) {
	p := &Payment4{}
	currencies := p.SupportedCurrencies()

	expected := []string{"USD", "EUR", "TRY", "GBP", "AED", "IRT"}
	if len(currencies) != len(expected) {
		t.Fatalf("expected %d currencies, got %d", len(expected), len(currencies))
	}

	for i, c := range currencies {
		if c != expected[i] {
			t.Errorf("currency[%d] = %q, want %q", i, c, expected[i])
		}
	}
}

func TestPayment4AmountConversion(t *testing.T) {
	tests := []struct {
		name      string
		cents     int64
		wantDolls float64
		wantCents int64
	}{
		{"one dollar", 100, 1.00, 100},
		{"typical", 1999, 19.99, 1999},
		{"one cent", 1, 0.01, 1},
		{"zero", 0, 0.00, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Cents -> dollars
			dollars := float64(tc.cents) / 100.0
			if dollars != tc.wantDolls {
				t.Errorf("cents->dollars: %d -> %f, want %f", tc.cents, dollars, tc.wantDolls)
			}

			// Dollars -> cents (roundtrip)
			gotCents := int64(math.Round(dollars * 100))
			if gotCents != tc.wantCents {
				t.Errorf("dollars->cents: %f -> %d, want %d", dollars, gotCents, tc.wantCents)
			}
		})
	}
}

func TestNewPayment4BaseURL(t *testing.T) {
	// sandbox=false, no custom URL -> production
	p1 := NewPayment4(Payment4Config{APIKey: "key", Sandbox: false})
	if p1.baseURL != "https://api.payment4.com/api/v1" {
		t.Errorf("production URL = %q, want %q", p1.baseURL, "https://api.payment4.com/api/v1")
	}

	// sandbox=true, no custom URL -> sandbox
	p2 := NewPayment4(Payment4Config{APIKey: "key", Sandbox: true})
	if p2.baseURL != "https://api.payment4.com/api/v1/sandbox" {
		t.Errorf("sandbox URL = %q, want %q", p2.baseURL, "https://api.payment4.com/api/v1/sandbox")
	}

	// Custom BaseURL overrides both
	customURL := "https://custom.example.com/v2"
	p3 := NewPayment4(Payment4Config{APIKey: "key", Sandbox: true, BaseURL: customURL})
	if p3.baseURL != customURL {
		t.Errorf("custom URL = %q, want %q", p3.baseURL, customURL)
	}

	p4 := NewPayment4(Payment4Config{APIKey: "key", Sandbox: false, BaseURL: customURL})
	if p4.baseURL != customURL {
		t.Errorf("custom URL (no sandbox) = %q, want %q", p4.baseURL, customURL)
	}
}
