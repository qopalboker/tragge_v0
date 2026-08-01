package providers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// Payment4 implements the Provider interface for Payment4 crypto payments.
type Payment4 struct {
	apiKey     string
	ipnSecret  string
	baseURL    string
	sandbox    bool
	httpClient *http.Client
	circuit    CircuitExecutor
	onHTTPCall func(method, endpoint string, duration time.Duration, err error)
}

// Payment4Config holds configuration for the Payment4 provider.
type Payment4Config struct {
	APIKey    string
	IPNSecret string
	BaseURL   string
	Sandbox   bool
	Circuit   CircuitExecutor
	// OnHTTPCall is an optional callback invoked after each HTTP call to Payment4.
	// It receives the HTTP method, endpoint path, duration, and any error.
	OnHTTPCall func(method, endpoint string, duration time.Duration, err error)
}

// NewPayment4 creates a new Payment4 provider.
func NewPayment4(cfg Payment4Config) *Payment4 {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		if cfg.Sandbox {
			baseURL = "https://api.payment4.com/api/v1/sandbox"
		} else {
			baseURL = "https://api.payment4.com/api/v1"
		}
	}

	return &Payment4{
		apiKey:    cfg.APIKey,
		ipnSecret: cfg.IPNSecret,
		baseURL:   baseURL,
		sandbox:   cfg.Sandbox,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		circuit:    cfg.Circuit,
		onHTTPCall: cfg.OnHTTPCall,
	}
}

// doHTTP executes an HTTP request through the circuit breaker.
// If no circuit is configured, the request is executed directly.
func (p *Payment4) doHTTP(ctx context.Context, req *http.Request) (*http.Response, error) {
	start := time.Now()
	var resp *http.Response
	var err error

	if p.circuit == nil {
		resp, err = p.httpClient.Do(req)
	} else {
		err = p.circuit.ExecuteWithContext(ctx, func(_ context.Context) error {
			var e error
			resp, e = p.httpClient.Do(req)
			return e
		})
	}

	if p.onHTTPCall != nil {
		p.onHTTPCall(req.Method, req.URL.Path, time.Since(start), err)
	}

	return resp, err
}

// Name returns the provider name.
func (p *Payment4) Name() ProviderType {
	return ProviderPayment4
}

// payment4CreateRequest represents the request to create a payment.
type payment4CreateRequest struct {
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	OrderID     string  `json:"orderId"`
	Description string  `json:"description,omitempty"`
	CallbackURL string  `json:"callbackUrl,omitempty"`
	SuccessURL  string  `json:"successUrl,omitempty"`
	CancelURL   string  `json:"cancelUrl,omitempty"`
}

// payment4CreateResponse represents the response from creating a payment.
type payment4CreateResponse struct {
	PaymentUID string  `json:"paymentUid"`
	PaymentURL string  `json:"paymentUrl"`
	Status     string  `json:"status"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	ExpiresAt  string  `json:"expiresAt,omitempty"`
}

// payment4StatusResponse represents the payment status response.
type payment4StatusResponse struct {
	PaymentUID  string  `json:"paymentUid"`
	OrderID     string  `json:"orderId"`
	Status      string  `json:"status"`
	Amount      float64 `json:"amount"`
	PaidAmount  float64 `json:"paidAmount"`
	Currency    string  `json:"currency"`
	PayCurrency string  `json:"payCurrency,omitempty"`
}

// CreatePayment creates a new payment via Payment4.
func (p *Payment4) CreatePayment(ctx context.Context, req *CreatePaymentRequest) (*CreatePaymentResponse, error) {
	// Convert cents to dollars
	amount := float64(req.AmountCents) / 100.0

	currency := req.Currency
	if currency == "" {
		currency = "USD"
	}

	cancelURL := req.CancelURL
	if cancelURL == "" {
		cancelURL = req.CallbackURL // Fallback to success URL for backwards compatibility
	}

	createReq := payment4CreateRequest{
		Amount:      amount,
		Currency:    strings.ToUpper(currency),
		OrderID:     req.OrderID,
		Description: req.Description,
		CallbackURL: req.IPNCallbackURL,
		SuccessURL:  req.CallbackURL,
		CancelURL:   cancelURL,
	}

	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/payment/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.doHTTP(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call payment4: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("payment4 error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var createResp payment4CreateResponse
	if err := json.Unmarshal(respBody, &createResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var expiresAt int64
	if createResp.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, createResp.ExpiresAt); err == nil {
			expiresAt = t.Unix()
		}
	}

	return &CreatePaymentResponse{
		ProviderPaymentID: createResp.PaymentUID,
		PaymentURL:        createResp.PaymentURL,
		ExpiresAt:         expiresAt,
		Status:            p.mapStatus(createResp.Status),
		Metadata: map[string]string{
			"currency": createResp.Currency,
			"amount":   fmt.Sprintf("%.2f", createResp.Amount),
		},
	}, nil
}

// GetPaymentStatus retrieves the current status of a payment.
func (p *Payment4) GetPaymentStatus(ctx context.Context, providerPaymentID string) (*PaymentStatusResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/payment/"+providerPaymentID+"/status", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", p.apiKey)

	resp, err := p.doHTTP(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call payment4: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPaymentNotFound
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("payment4 error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var status payment4StatusResponse
	if err := json.Unmarshal(respBody, &status); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &PaymentStatusResponse{
		ProviderPaymentID: status.PaymentUID,
		Status:            p.mapStatus(status.Status),
		AmountCents:       int64(math.Round(status.Amount * 100)),
		Currency:          strings.ToUpper(status.Currency),
		PaidAmountCents:   int64(math.Round(status.PaidAmount * 100)),
		Metadata: map[string]string{
			"order_id":     status.OrderID,
			"pay_currency": status.PayCurrency,
		},
	}, nil
}

// VerifyWebhook verifies the webhook signature and parses the event.
//
// Signature verification is optional: if the x-payment4-signature header is
// absent, the webhook is still processed with a warning log. The critical
// security layer is server-side verification — callers should call
// GetPaymentStatus after processing to confirm with Payment4's API directly.
func (p *Payment4) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	// Parse the body
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse webhook body: %w", err)
	}

	// Check signature
	signature := headers["x-payment4-signature"]
	if p.ipnSecret != "" {
		// IPN secret is configured — signature is REQUIRED
		if signature == "" {
			return nil, fmt.Errorf("x-payment4-signature header missing: %w", ErrInvalidSignature)
		}
		canonicalJSON := sortedJSON(data)
		mac := hmac.New(sha256.New, []byte(p.ipnSecret))
		mac.Write([]byte(canonicalJSON))
		expectedSig := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
			return nil, fmt.Errorf("HMAC signature mismatch: %w", ErrInvalidSignature)
		}
	} else if signature != "" {
		// No secret configured but signature was sent — can't verify
		data["_warning"] = "signature present but no IPN secret configured to verify it"
	}
	// If no secret and no signature — proceed (server-side verification in webhook handler provides security)

	// Parse webhook fields
	paymentUID := ""
	if uid, ok := data["paymentUid"].(string); ok {
		paymentUID = uid
	}

	orderID := ""
	if oid, ok := data["orderId"].(string); ok {
		orderID = oid
	}

	status := ""
	if s, ok := data["status"].(string); ok {
		status = s
	}

	amount := 0.0
	if a, ok := data["amount"].(float64); ok {
		amount = a
	}

	paidAmount := 0.0
	if pa, ok := data["paidAmount"].(float64); ok {
		paidAmount = pa
	}

	currency := "USD"
	if c, ok := data["currency"].(string); ok {
		currency = c
	}

	return &WebhookEvent{
		Provider:          ProviderPayment4,
		ProviderPaymentID: paymentUID,
		OrderID:           orderID,
		Status:            p.mapStatus(status),
		AmountCents:       int64(math.Round(amount * 100)),
		Currency:          strings.ToUpper(currency),
		PaidAmountCents:   int64(math.Round(paidAmount * 100)),
		RawData:           data,
	}, nil
}

// VerifySignature verifies an HMAC-SHA256 signature for a Payment4 webhook payload.
// Exported for testing.
func (p *Payment4) VerifySignature(body []byte, signature string) bool {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return false
	}

	canonicalJSON := sortedJSON(data)
	mac := hmac.New(sha256.New, []byte(p.ipnSecret))
	mac.Write([]byte(canonicalJSON))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// CreatePayout is not supported by Payment4.
func (p *Payment4) CreatePayout(ctx context.Context, req *PayoutRequest) (*PayoutResponse, error) {
	return nil, fmt.Errorf("payment4 does not support payouts")
}

// GetPayoutStatus is not supported by Payment4.
func (p *Payment4) GetPayoutStatus(ctx context.Context, providerPayoutID string) (*PayoutResponse, error) {
	return nil, fmt.Errorf("payment4 does not support payouts")
}

// RefundPayment is not supported by Payment4.
func (p *Payment4) RefundPayment(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	return nil, fmt.Errorf("payment4 does not support refunds")
}

// ReversePayment is not supported by Payment4.
func (p *Payment4) ReversePayment(ctx context.Context, purchaseID string) (*RefundResponse, error) {
	return nil, fmt.Errorf("payment4 does not support payment reversal")
}

// IsAvailable checks if Payment4 is available.
func (p *Payment4) IsAvailable(ctx context.Context) bool {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/status", nil)
	if err != nil {
		return false
	}

	httpReq.Header.Set("x-api-key", p.apiKey)

	resp, err := p.doHTTP(ctx, httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// SupportedCurrencies returns currencies that Payment4 accepts for pricing.
func (p *Payment4) SupportedCurrencies() []string {
	return []string{"USD", "EUR", "TRY", "GBP", "AED", "IRT"}
}

// mapStatus maps Payment4 status strings to internal PaymentStatus.
func (p *Payment4) mapStatus(status string) PaymentStatus {
	switch strings.ToUpper(status) {
	case "PAID", "CONFIRMED":
		return PaymentStatusFinished
	case "PENDING", "WAITING", "PARTIALLY_PAID":
		return PaymentStatusPending
	case "EXPIRED":
		return PaymentStatusExpired
	case "FAILED", "CANCELLED":
		return PaymentStatusFailed
	default:
		return PaymentStatusPending
	}
}
