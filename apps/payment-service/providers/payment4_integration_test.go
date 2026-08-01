//go:build integration

package providers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"os"
	"testing"
	"time"
)

// loggingTransport wraps http.RoundTripper to log raw HTTP request/response.
type loggingTransport struct {
	transport http.RoundTripper
	t         *testing.T
}

func (lt *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Log the raw request
	reqDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		lt.t.Logf("[REQUEST DUMP ERROR] %v", err)
	} else {
		lt.t.Logf("[RAW REQUEST]\n%s", string(reqDump))
	}

	resp, err := lt.transport.RoundTrip(req)
	if err != nil {
		lt.t.Logf("[TRANSPORT ERROR] %v", err)
		return resp, err
	}

	// Log the raw response
	respDump, dumpErr := httputil.DumpResponse(resp, true)
	if dumpErr != nil {
		lt.t.Logf("[RESPONSE DUMP ERROR] %v", dumpErr)
	} else {
		lt.t.Logf("[RAW RESPONSE]\n%s", string(respDump))
	}

	// We need to re-create the body since DumpResponse consumed it
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return resp, nil
}

func TestPayment4IntegrationCreatePayment(t *testing.T) {
	apiKey := os.Getenv("PAYMENT4_API_KEY")
	if apiKey == "" {
		t.Skip("PAYMENT4_API_KEY not set, skipping integration test")
	}

	// Try the primary base URL
	baseURLs := []string{
		"https://api.payment4.com/api/v1",
		"https://api.payment4.com",
	}

	for _, baseURL := range baseURLs {
		t.Run("baseURL="+baseURL, func(t *testing.T) {
			p := NewPayment4(Payment4Config{
				APIKey:  apiKey,
				BaseURL: baseURL,
				Sandbox: true,
			})

			// Override the HTTP client with logging transport
			p.httpClient = &http.Client{
				Timeout:   30 * time.Second,
				Transport: &loggingTransport{transport: http.DefaultTransport, t: t},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := p.CreatePayment(ctx, &CreatePaymentRequest{
				AmountCents:    100, // $1.00
				Currency:       "USD",
				OrderID:        fmt.Sprintf("integration-test-%d", time.Now().UnixNano()),
				Description:    "Integration test payment",
				IPNCallbackURL: "https://example.com/webhooks/payment4",
				CallbackURL:    "https://example.com/success",
			})
			if err != nil {
				t.Logf("CreatePayment failed for baseURL=%s: %v", baseURL, err)
				return
			}

			t.Logf("CreatePayment response:")
			t.Logf("  ProviderPaymentID: %s", resp.ProviderPaymentID)
			t.Logf("  PaymentURL: %s", resp.PaymentURL)
			t.Logf("  Status: %s", resp.Status)
			t.Logf("  ExpiresAt: %d", resp.ExpiresAt)
			t.Logf("  Metadata: %v", resp.Metadata)

			if resp.ProviderPaymentID == "" {
				t.Error("expected non-empty ProviderPaymentID")
			}

			// Test GetPaymentStatus with the returned payment ID
			t.Run("GetPaymentStatus", func(t *testing.T) {
				statusResp, err := p.GetPaymentStatus(ctx, resp.ProviderPaymentID)
				if err != nil {
					t.Logf("GetPaymentStatus failed: %v", err)
					return
				}

				t.Logf("GetPaymentStatus response:")
				t.Logf("  ProviderPaymentID: %s", statusResp.ProviderPaymentID)
				t.Logf("  Status: %s", statusResp.Status)
				t.Logf("  AmountCents: %d", statusResp.AmountCents)
				t.Logf("  Currency: %s", statusResp.Currency)
				t.Logf("  PaidAmountCents: %d", statusResp.PaidAmountCents)
				t.Logf("  Metadata: %v", statusResp.Metadata)
			})
		})
	}
}

func TestPayment4IntegrationIsAvailable(t *testing.T) {
	apiKey := os.Getenv("PAYMENT4_API_KEY")
	if apiKey == "" {
		t.Skip("PAYMENT4_API_KEY not set, skipping integration test")
	}

	baseURLs := []string{
		"https://api.payment4.com/api/v1",
		"https://api.payment4.com",
	}

	for _, baseURL := range baseURLs {
		t.Run("baseURL="+baseURL, func(t *testing.T) {
			p := NewPayment4(Payment4Config{
				APIKey:  apiKey,
				BaseURL: baseURL,
				Sandbox: true,
			})

			// Override the HTTP client with logging transport
			p.httpClient = &http.Client{
				Timeout:   30 * time.Second,
				Transport: &loggingTransport{transport: http.DefaultTransport, t: t},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			available := p.IsAvailable(ctx)
			t.Logf("IsAvailable for baseURL=%s: %v", baseURL, available)
		})
	}
}
