package providers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// AllowedCryptoCurrencies defines the only crypto currencies accepted for payment.
// Only USDT on the Tron network (TRC20) and native TRX are supported.
var AllowedCryptoCurrencies = map[string]string{
	"usdttrc20": "USDT (TRC20)",
	"trx":       "TRX",
}

// NowPayments implements the Provider interface for NOWPayments crypto payments
type NowPayments struct {
	apiKey     string
	publicKey  string
	ipnSecret  string
	baseURL    string
	sandbox    bool
	httpClient *http.Client
	circuit    CircuitExecutor
}

// NowPaymentsConfig holds configuration for NOWPayments provider
type NowPaymentsConfig struct {
	APIKey    string
	PublicKey string
	IPNSecret string
	BaseURL   string
	Sandbox   bool
	Circuit   CircuitExecutor
}

// NewNowPayments creates a new NOWPayments provider
func NewNowPayments(cfg NowPaymentsConfig) *NowPayments {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		if cfg.Sandbox {
			baseURL = "https://api-sandbox.nowpayments.io/v1"
		} else {
			baseURL = "https://api.nowpayments.io/v1"
		}
	}

	return &NowPayments{
		apiKey:    cfg.APIKey,
		publicKey: cfg.PublicKey,
		ipnSecret: cfg.IPNSecret,
		baseURL:   baseURL,
		sandbox:   cfg.Sandbox,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		circuit: cfg.Circuit,
	}
}

// doHTTP executes an HTTP request through the circuit breaker.
// If no circuit is configured, the request is executed directly.
func (n *NowPayments) doHTTP(ctx context.Context, req *http.Request) (*http.Response, error) {
	if n.circuit == nil {
		return n.httpClient.Do(req)
	}
	var resp *http.Response
	err := n.circuit.ExecuteWithContext(ctx, func(_ context.Context) error {
		var e error
		resp, e = n.httpClient.Do(req)
		return e
	})
	return resp, err
}

// Name returns the provider name
func (n *NowPayments) Name() ProviderType {
	return ProviderNowPayments
}

// nowPaymentsInvoiceRequest represents request to create an invoice
type nowPaymentsInvoiceRequest struct {
	PriceAmount      float64 `json:"price_amount"`
	PriceCurrency    string  `json:"price_currency"`
	PayCurrency      string  `json:"pay_currency,omitempty"`
	IPNCallbackURL   string  `json:"ipn_callback_url,omitempty"`
	OrderID          string  `json:"order_id,omitempty"`
	OrderDescription string  `json:"order_description,omitempty"`
	SuccessURL       string  `json:"success_url,omitempty"`
	CancelURL        string  `json:"cancel_url,omitempty"`
}

// nowPaymentsInvoiceResponse represents response from creating an invoice
type nowPaymentsInvoiceResponse struct {
	ID              string  `json:"id"`
	InvoiceURL      string  `json:"invoice_url"`
	PaymentStatus   string  `json:"payment_status"`
	PriceAmount     float64 `json:"price_amount"`
	PriceCurrency   string  `json:"price_currency"`
	PayAmount       float64 `json:"pay_amount,omitempty"`
	PayCurrency     string  `json:"pay_currency,omitempty"`
	PayAddress      string  `json:"pay_address,omitempty"`
	ExpirationDate  string  `json:"expiration_estimate_date,omitempty"`
	CreatedAt       string  `json:"created_at"`
}

// nowPaymentsPaymentStatus represents payment status response
type nowPaymentsPaymentStatus struct {
	PaymentID       int64   `json:"payment_id"`
	PaymentStatus   string  `json:"payment_status"`
	PayAddress      string  `json:"pay_address"`
	PayAmount       float64 `json:"pay_amount"`
	ActuallyPaid    float64 `json:"actually_paid"`
	PayCurrency     string  `json:"pay_currency"`
	PriceAmount     float64 `json:"price_amount"`
	PriceCurrency   string  `json:"price_currency"`
	OrderID         string  `json:"order_id"`
}

// CreatePayment creates a new crypto payment invoice
func (n *NowPayments) CreatePayment(ctx context.Context, req *CreatePaymentRequest) (*CreatePaymentResponse, error) {
	// Enforce allowed crypto currencies
	payCurrency := strings.ToLower(req.PayCurrency)
	if payCurrency == "" {
		payCurrency = "usdttrc20" // Default to USDT TRC20
	}
	if _, ok := AllowedCryptoCurrencies[payCurrency]; !ok {
		return nil, fmt.Errorf("unsupported crypto currency: %s. Allowed: usdttrc20, trx", payCurrency)
	}

	// Convert cents to dollars for NOWPayments
	amount := float64(req.AmountCents) / 100.0

	cancelURL := req.CancelURL
	if cancelURL == "" {
		cancelURL = req.CallbackURL // Fallback to success URL for backwards compatibility
	}

	invoiceReq := nowPaymentsInvoiceRequest{
		PriceAmount:      amount,
		PriceCurrency:    "usd", // Always price in USD
		PayCurrency:      payCurrency,
		OrderID:          req.OrderID,
		OrderDescription: req.Description,
		IPNCallbackURL:   req.IPNCallbackURL,
		SuccessURL:       req.CallbackURL,
		CancelURL:        cancelURL,
	}

	body, err := json.Marshal(invoiceReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", n.baseURL+"/invoice", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", n.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := n.doHTTP(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call nowpayments: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("nowpayments error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var invoiceResp nowPaymentsInvoiceResponse
	if err := json.Unmarshal(respBody, &invoiceResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var expiresAt int64
	if invoiceResp.ExpirationDate != "" {
		if t, err := time.Parse(time.RFC3339, invoiceResp.ExpirationDate); err == nil {
			expiresAt = t.Unix()
		}
	}

	return &CreatePaymentResponse{
		ProviderPaymentID: invoiceResp.ID,
		PaymentURL:        invoiceResp.InvoiceURL,
		PayAddress:        invoiceResp.PayAddress,
		PayAmount:         invoiceResp.PayAmount,
		PayCurrency:       invoiceResp.PayCurrency,
		ExpiresAt:         expiresAt,
		Status:            n.mapStatus(invoiceResp.PaymentStatus),
		Metadata: map[string]string{
			"price_currency": invoiceResp.PriceCurrency,
			"price_amount":   fmt.Sprintf("%.2f", invoiceResp.PriceAmount),
		},
	}, nil
}

// GetPaymentStatus retrieves the current status of a payment
func (n *NowPayments) GetPaymentStatus(ctx context.Context, providerPaymentID string) (*PaymentStatusResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", n.baseURL+"/payment/"+providerPaymentID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", n.apiKey)

	resp, err := n.doHTTP(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call nowpayments: %w", err)
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
		return nil, fmt.Errorf("nowpayments error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var status nowPaymentsPaymentStatus
	if err := json.Unmarshal(respBody, &status); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &PaymentStatusResponse{
		ProviderPaymentID: providerPaymentID,
		Status:            n.mapStatus(status.PaymentStatus),
		AmountCents:       int64(math.Round(status.PriceAmount * 100)),
		Currency:          strings.ToUpper(status.PriceCurrency),
		PaidAmountCents:   int64(math.Round(status.ActuallyPaid * 100)),
		Metadata: map[string]string{
			"pay_currency": status.PayCurrency,
			"pay_address":  status.PayAddress,
			"order_id":     status.OrderID,
		},
	}, nil
}

// VerifyWebhook verifies the IPN callback signature and parses the event
func (n *NowPayments) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	// Get signature from header
	signature := headers["x-nowpayments-sig"]
	if signature == "" {
		return nil, ErrInvalidSignature
	}

	// Parse the body to sort keys
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse webhook body: %w", err)
	}

	// Sort keys and create canonical JSON
	canonicalJSON := sortedJSON(data)

	// Calculate HMAC-SHA512
	mac := hmac.New(sha512.New, []byte(n.ipnSecret))
	mac.Write([]byte(canonicalJSON))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return nil, ErrInvalidSignature
	}

	// Parse the webhook data
	paymentID := ""
	if id, ok := data["payment_id"].(float64); ok {
		paymentID = fmt.Sprintf("%.0f", id)
	}
	if id, ok := data["payment_id"].(string); ok {
		paymentID = id
	}

	orderID := ""
	if oid, ok := data["order_id"].(string); ok {
		orderID = oid
	}

	status := ""
	if s, ok := data["payment_status"].(string); ok {
		status = s
	}

	priceAmount := 0.0
	if pa, ok := data["price_amount"].(float64); ok {
		priceAmount = pa
	}

	actuallyPaid := 0.0
	if ap, ok := data["actually_paid"].(float64); ok {
		actuallyPaid = ap
	}

	currency := "usd"
	if c, ok := data["price_currency"].(string); ok {
		currency = c
	}

	return &WebhookEvent{
		Provider:          ProviderNowPayments,
		ProviderPaymentID: paymentID,
		OrderID:           orderID,
		Status:            n.mapStatus(status),
		AmountCents:       int64(math.Round(priceAmount * 100)),
		Currency:          strings.ToUpper(currency),
		PaidAmountCents:   int64(math.Round(actuallyPaid * 100)),
		RawData:           data,
	}, nil
}

// sortedJSON produces a canonical JSON string with deterministic key ordering
// at every nesting level. Required for HMAC signature verification of webhook payloads.
func sortedJSON(v interface{}) string {
	var b strings.Builder
	writeSortedJSON(&b, v)
	return b.String()
}

// writeSortedJSON recursively writes a canonical JSON representation to the builder.
func writeSortedJSON(b *strings.Builder, v interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		b.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			keyJSON, _ := json.Marshal(k)
			b.Write(keyJSON)
			b.WriteString(":")
			writeSortedJSON(b, val[k])
		}
		b.WriteString("}")

	case []interface{}:
		b.WriteString("[")
		for i, elem := range val {
			if i > 0 {
				b.WriteString(",")
			}
			writeSortedJSON(b, elem)
		}
		b.WriteString("]")

	default:
		// Primitives (string, float64, bool, nil) — json.Marshal is deterministic.
		raw, _ := json.Marshal(val)
		b.Write(raw)
	}
}

// CreatePayout creates a crypto payout (withdrawal)
func (n *NowPayments) CreatePayout(ctx context.Context, req *PayoutRequest) (*PayoutResponse, error) {
	// NOWPayments payout API
	payoutReq := map[string]interface{}{
		"address":  req.WalletAddress,
		"currency": strings.ToLower(req.CryptoCurrency),
		"amount":   float64(req.AmountCents) / 100.0,
		"ipn_url":  "", // Optional IPN URL for payout status updates
	}

	body, err := json.Marshal(payoutReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", n.baseURL+"/payout", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", n.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := n.doHTTP(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call nowpayments: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("nowpayments payout error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var payoutResp map[string]interface{}
	if err := json.Unmarshal(respBody, &payoutResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	payoutID := ""
	if id, ok := payoutResp["id"].(string); ok {
		payoutID = id
	}

	return &PayoutResponse{
		ProviderPayoutID: payoutID,
		Status:           PaymentStatusPending,
	}, nil
}

// GetPayoutStatus retrieves the status of a payout
func (n *NowPayments) GetPayoutStatus(ctx context.Context, providerPayoutID string) (*PayoutResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", n.baseURL+"/payout/"+providerPayoutID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", n.apiKey)

	resp, err := n.doHTTP(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call nowpayments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPaymentNotFound
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var payoutResp map[string]interface{}
	if err := json.Unmarshal(respBody, &payoutResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	status := PaymentStatusPending
	if s, ok := payoutResp["status"].(string); ok {
		status = n.mapStatus(s)
	}

	return &PayoutResponse{
		ProviderPayoutID: providerPayoutID,
		Status:           status,
	}, nil
}

// IsAvailable checks if NOWPayments is available
func (n *NowPayments) IsAvailable(ctx context.Context) bool {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", n.baseURL+"/status", nil)
	if err != nil {
		return false
	}

	httpReq.Header.Set("x-api-key", n.apiKey)

	resp, err := n.doHTTP(ctx, httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// SupportedCurrencies returns the price currency. Payments are always priced in USD;
// the user pays in one of the AllowedCryptoCurrencies (USDT TRC20 or TRX).
func (n *NowPayments) SupportedCurrencies() []string {
	return []string{"USD"}
}

// RefundPayment is not supported by NOWPayments.
func (n *NowPayments) RefundPayment(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	return nil, fmt.Errorf("nowpayments does not support refunds")
}

// ReversePayment is not supported by NOWPayments.
func (n *NowPayments) ReversePayment(ctx context.Context, purchaseID string) (*RefundResponse, error) {
	return nil, fmt.Errorf("nowpayments does not support payment reversal")
}

// nowPaymentsEstimateResponse represents the response from the estimate endpoint
type nowPaymentsEstimateResponse struct {
	CurrencyFrom  string  `json:"currency_from"`
	AmountFrom    float64 `json:"amount_from"`
	CurrencyTo    string  `json:"currency_to"`
	EstimatedAmount float64 `json:"estimated_amount"`
}

// GetEstimate returns the estimated crypto amount for a given USD amount and target currency.
func (n *NowPayments) GetEstimate(ctx context.Context, amountUSD float64, cryptoCurrency string) (*nowPaymentsEstimateResponse, error) {
	cryptoCurrency = strings.ToLower(cryptoCurrency)
	if _, ok := AllowedCryptoCurrencies[cryptoCurrency]; !ok {
		return nil, fmt.Errorf("unsupported crypto currency: %s. Allowed: usdttrc20, trx", cryptoCurrency)
	}

	url := fmt.Sprintf("%s/estimate?amount=%.2f&currency_from=usd&currency_to=%s", n.baseURL, amountUSD, cryptoCurrency)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", n.apiKey)

	resp, err := n.doHTTP(ctx, httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call nowpayments estimate: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nowpayments estimate error: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var estimate nowPaymentsEstimateResponse
	if err := json.Unmarshal(respBody, &estimate); err != nil {
		return nil, fmt.Errorf("failed to parse estimate response: %w", err)
	}

	return &estimate, nil
}

// mapStatus maps NOWPayments status to internal status
func (n *NowPayments) mapStatus(status string) PaymentStatus {
	switch strings.ToLower(status) {
	case "waiting":
		return PaymentStatusWaiting
	case "confirming":
		return PaymentStatusConfirming
	case "confirmed":
		return PaymentStatusConfirmed
	case "sending":
		return PaymentStatusSending
	case "partially_paid":
		return PaymentStatusConfirming
	case "finished":
		return PaymentStatusFinished
	case "failed":
		return PaymentStatusFailed
	case "refunded":
		return PaymentStatusRefunded
	case "expired":
		return PaymentStatusExpired
	default:
		return PaymentStatusPending
	}
}
