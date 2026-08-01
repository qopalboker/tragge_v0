package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestJibitMapStatus(t *testing.T) {
	j := &Jibit{}

	tests := []struct {
		input    string
		expected PaymentStatus
	}{
		// Callback redirect statuses
		{"SUCCESSFUL", PaymentStatusFinished},
		// Verify API statuses
		{"ALREADY_VERIFIED", PaymentStatusFinished},
		{"SUCCESS", PaymentStatusFinished},
		{"MANUALLY_SUCCESS", PaymentStatusFinished},
		// Failed
		{"FAILED", PaymentStatusFailed},
		// Expired
		{"EXPIRED", PaymentStatusExpired},
		// Reversed
		{"REVERSED", PaymentStatusRefunded},
		// In-progress states
		{"IN_PROGRESS", PaymentStatusPending},
		{"READY_TO_VERIFY", PaymentStatusPending},
		// Unknown — needs inquiry, treat as pending
		{"UNKNOWN", PaymentStatusPending},
		// Unrecognised value falls back to pending
		{"SOMETHING_NEW", PaymentStatusPending},
		{"", PaymentStatusPending},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := j.mapStatus(tc.input)
			if got != tc.expected {
				t.Errorf("mapStatus(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// buildUnsignedJWT creates a minimal unsigned JWT with the given claims for testing.
// The token is formatted as header.payload.signature where signature is empty.
func buildUnsignedJWT(claims map[string]interface{}) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + payloadEnc + "."
}

func TestExtractTokenExpiry(t *testing.T) {
	// Valid JWT with exp claim 1 hour from now
	exp := time.Now().Add(1 * time.Hour)
	token := buildUnsignedJWT(map[string]interface{}{
		"exp": exp.Unix(),
		"sub": "test",
	})

	got := extractTokenExpiry(token)
	expected := exp.Add(-tokenExpiryBuffer)

	// Allow 2 seconds of tolerance for test execution time
	diff := math.Abs(float64(got.Sub(expected).Milliseconds()))
	if diff > 2000 {
		t.Errorf("extractTokenExpiry() = %v, want ~%v (diff: %v)", got, expected, got.Sub(expected))
	}
}

func TestExtractTokenExpiryWithFarFutureExp(t *testing.T) {
	// JWT with 24h expiry (typical Jibit token)
	exp := time.Now().Add(24 * time.Hour)
	token := buildUnsignedJWT(map[string]interface{}{
		"exp": exp.Unix(),
	})

	got := extractTokenExpiry(token)
	expected := exp.Add(-tokenExpiryBuffer)

	diff := math.Abs(float64(got.Sub(expected).Milliseconds()))
	if diff > 2000 {
		t.Errorf("extractTokenExpiry() = %v, want ~%v", got, expected)
	}
}

func TestExtractTokenExpiryFallback(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"garbage", "not-a-jwt"},
		{"missing exp claim", buildUnsignedJWT(map[string]interface{}{"sub": "test"})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now().Add(defaultTokenTTL)
			got := extractTokenExpiry(tc.token)
			after := time.Now().Add(defaultTokenTTL)

			if got.Before(before.Add(-time.Second)) || got.After(after.Add(time.Second)) {
				t.Errorf("extractTokenExpiry(%q) = %v, want ~%v (defaultTokenTTL fallback)",
					tc.token, got, before)
			}
		})
	}
}

func TestDoAuthenticatedRequest401Retry(t *testing.T) {
	var callCount atomic.Int32

	// Mock server: first API call returns 401, second returns 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tokens" || r.URL.Path == "/tokens/refresh" {
			// Token generation/refresh endpoint
			w.Header().Set("Content-Type", "application/json")
			exp := time.Now().Add(1 * time.Hour)
			token := buildUnsignedJWT(map[string]interface{}{"exp": exp.Unix()})
			fmt.Fprintf(w, `{"accessToken":"%s","refreshToken":"refresh-tok"}`, token)
			return
		}

		count := callCount.Add(1)
		if count == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"errors":[{"code":"token.verification_failed"}]}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	j := NewJibit(JibitConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})

	resp, err := j.doAuthenticatedRequest(context.Background(), "GET", srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("doAuthenticatedRequest() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}

	if got := callCount.Load(); got != 2 {
		t.Errorf("expected 2 API calls (1 fail + 1 retry), got %d", got)
	}
}

func TestDoAuthenticatedRequestNoRetryOnSuccess(t *testing.T) {
	var callCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tokens" || r.URL.Path == "/tokens/refresh" {
			w.Header().Set("Content-Type", "application/json")
			exp := time.Now().Add(1 * time.Hour)
			token := buildUnsignedJWT(map[string]interface{}{"exp": exp.Unix()})
			fmt.Fprintf(w, `{"accessToken":"%s","refreshToken":"refresh-tok"}`, token)
			return
		}

		callCount.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	j := NewJibit(JibitConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})

	resp, err := j.doAuthenticatedRequest(context.Background(), "GET", srv.URL+"/test", nil)
	if err != nil {
		t.Fatalf("doAuthenticatedRequest() error = %v", err)
	}
	defer resp.Body.Close()

	if got := callCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 API call (no retry needed), got %d", got)
	}
}

// jibitTestServer creates a mock Jibit server with token endpoints and custom route handler.
func jibitTestServer(handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tokens" || r.URL.Path == "/tokens/refresh" {
			w.Header().Set("Content-Type", "application/json")
			exp := time.Now().Add(1 * time.Hour)
			token := buildUnsignedJWT(map[string]interface{}{"exp": exp.Unix()})
			fmt.Fprintf(w, `{"accessToken":"%s","refreshToken":"refresh-tok"}`, token)
			return
		}
		handler(w, r)
	}))
}

func TestVerifyWebhookAlreadyVerified(t *testing.T) {
	srv := jibitTestServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/verify") {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"errors":[{"code":"payment.already_verified","message":"Payment already verified"}]}`))
			return
		}
		if r.URL.Path == "/purchases" && r.URL.Query().Get("purchaseId") != "" {
			w.Write([]byte(`{"elements":[{"purchaseId":123,"amount":50000,"state":"SUCCESS","pspRrn":"ref123","pspMaskedCardNumber":"6037-****-1234","pspTraceNumber":"trace456"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	j := NewJibit(JibitConfig{APIKey: "k", SecretKey: "s", BaseURL: srv.URL})

	body := []byte(`purchaseId=123&amount=50000&status=SUCCESSFUL&clientReferenceNumber=order-1`)
	headers := map[string]string{"content-type": "application/x-www-form-urlencoded"}

	event, err := j.VerifyWebhook(context.Background(), headers, body)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.Status != PaymentStatusFinished {
		t.Errorf("expected status finished, got %s", event.Status)
	}
	if event.OrderID != "order-1" {
		t.Errorf("expected OrderID order-1, got %s", event.OrderID)
	}
	if event.RawData["verify_skipped"] != "already_verified" {
		t.Error("expected verify_skipped=already_verified in RawData")
	}
	if event.AmountCents != 50000 {
		t.Errorf("expected AmountCents 50000, got %d", event.AmountCents)
	}
}

func TestVerifyWebhookSkipsVerifyOnFailed(t *testing.T) {
	var verifyCalled atomic.Bool

	srv := jibitTestServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/verify") {
			verifyCalled.Store(true)
			w.Write([]byte(`{"status":"FAILED","amount":50000}`))
			return
		}
	})
	defer srv.Close()

	j := NewJibit(JibitConfig{APIKey: "k", SecretKey: "s", BaseURL: srv.URL})

	body := []byte(`purchaseId=123&amount=50000&status=FAILED&clientReferenceNumber=order-1`)
	headers := map[string]string{"content-type": "application/x-www-form-urlencoded"}

	event, err := j.VerifyWebhook(context.Background(), headers, body)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.Status != PaymentStatusFailed {
		t.Errorf("expected status failed, got %s", event.Status)
	}
	if verifyCalled.Load() {
		t.Error("verify API should not have been called for FAILED callback")
	}
	if event.RawData["callback_status"] != "FAILED" {
		t.Error("expected callback_status=FAILED in RawData")
	}
}

func TestCreatePaymentUsesStringID(t *testing.T) {
	srv := jibitTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/purchases" && r.Method == "POST" {
			w.Write([]byte(`{"purchaseId":12345,"purchaseIdStr":"12345","pspSwitchingUrl":"https://psp.example.com/pay","state":"IN_PROGRESS"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	j := NewJibit(JibitConfig{APIKey: "k", SecretKey: "s", BaseURL: srv.URL})

	resp, err := j.CreatePayment(context.Background(), &CreatePaymentRequest{
		OrderID:     "order-1",
		AmountCents: 50000,
		Currency:    "IRR",
	})
	if err != nil {
		t.Fatalf("CreatePayment() error = %v", err)
	}
	if resp.ProviderPaymentID != "12345" {
		t.Errorf("expected ProviderPaymentID '12345', got %s", resp.ProviderPaymentID)
	}

	// Test fallback when purchaseIdStr is empty
	srv2 := jibitTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/purchases" && r.Method == "POST" {
			w.Write([]byte(`{"purchaseId":67890,"pspSwitchingUrl":"https://psp.example.com/pay","state":"IN_PROGRESS"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv2.Close()

	j2 := NewJibit(JibitConfig{APIKey: "k", SecretKey: "s", BaseURL: srv2.URL})
	resp2, err := j2.CreatePayment(context.Background(), &CreatePaymentRequest{
		OrderID:     "order-2",
		AmountCents: 60000,
		Currency:    "IRR",
	})
	if err != nil {
		t.Fatalf("CreatePayment() fallback error = %v", err)
	}
	if resp2.ProviderPaymentID != "67890" {
		t.Errorf("expected fallback ProviderPaymentID '67890', got %s", resp2.ProviderPaymentID)
	}
}

func TestVerifyWebhookJSONFallbackDecodesClientRef(t *testing.T) {
	srv := jibitTestServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/verify") {
			w.Write([]byte(`{"status":"SUCCESSFUL","amount":50000,"pspRrn":"ref123"}`))
			return
		}
	})
	defer srv.Close()

	j := NewJibit(JibitConfig{APIKey: "k", SecretKey: "s", BaseURL: srv.URL})

	// JSON body with URL-encoded clientReferenceNumber
	body := []byte(`{"purchaseId":"123","amount":50000,"status":"SUCCESSFUL","clientReferenceNumber":"order%2F1%3Fref%3Dabc"}`)
	headers := map[string]string{"content-type": "application/json"}

	event, err := j.VerifyWebhook(context.Background(), headers, body)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.OrderID != "order/1?ref=abc" {
		t.Errorf("expected decoded OrderID 'order/1?ref=abc', got %q", event.OrderID)
	}
}

func TestIsAvailableUsesAuth(t *testing.T) {
	var authHeader atomic.Value

	srv := jibitTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/health" {
			authHeader.Store(r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"UP"}`))
			return
		}
	})
	defer srv.Close()

	j := NewJibit(JibitConfig{APIKey: "k", SecretKey: "s", BaseURL: srv.URL})

	available := j.IsAvailable(context.Background())
	if !available {
		t.Error("expected IsAvailable to return true")
	}

	auth, _ := authHeader.Load().(string)
	if !strings.HasPrefix(auth, "Bearer ") {
		t.Errorf("expected Authorization header with Bearer token, got %q", auth)
	}
}

func TestVerifyWebhookPromotesCallbackStatus(t *testing.T) {
	srv := jibitTestServer(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/verify") {
			w.Write([]byte(`{"status":"SUCCESSFUL","amount":50000,"pspRrn":"ref123"}`))
			return
		}
	})
	defer srv.Close()

	j := NewJibit(JibitConfig{APIKey: "k", SecretKey: "s", BaseURL: srv.URL})

	body := []byte(`purchaseId=123&amount=50000&status=SUCCESSFUL&clientReferenceNumber=order-1`)
	headers := map[string]string{"content-type": "application/x-www-form-urlencoded"}

	event, err := j.VerifyWebhook(context.Background(), headers, body)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	cs, ok := event.RawData["callback_status"].(string)
	if !ok || cs != "SUCCESSFUL" {
		t.Errorf("expected callback_status=SUCCESSFUL in RawData, got %v", event.RawData["callback_status"])
	}
}

func TestJibitErrorResponseHasErrorCode(t *testing.T) {
	resp := jibitErrorResponse{
		Errors: []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{
			{Code: "payment.already_verified", Message: "Payment already verified"},
		},
	}
	if !resp.hasErrorCode("payment.already_verified") {
		t.Error("expected hasErrorCode to return true for payment.already_verified")
	}
	if resp.hasErrorCode("some.other.error") {
		t.Error("expected hasErrorCode to return false for some.other.error")
	}

	empty := jibitErrorResponse{}
	if empty.hasErrorCode("anything") {
		t.Error("expected empty error response to return false")
	}
}
