package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"
)

// tokenExpiryBuffer is subtracted from the JWT exp claim to account for
// clock skew and network latency.
const tokenExpiryBuffer = 5 * time.Minute

// defaultTokenTTL is used as a fallback when the JWT exp claim cannot be parsed.
const defaultTokenTTL = 20 * time.Hour

// Jibit implements the Provider interface for Jibit PPG (Proxy Payment Gateway)
type Jibit struct {
	apiKey      string
	secretKey   string
	callbackURL string
	baseURL     string
	httpClient  *http.Client
	circuit     CircuitExecutor

	// Token management
	mu           sync.RWMutex
	accessToken  string
	refreshToken string
	tokenExpiry  time.Time
	sfGroup      singleflight.Group
}

// JibitConfig holds configuration for Jibit provider
type JibitConfig struct {
	APIKey      string
	SecretKey   string
	CallbackURL string
	BaseURL     string // default: https://napi.jibit.ir/ppg/v3
	Circuit     CircuitExecutor
}

// NewJibit creates a new Jibit provider
func NewJibit(cfg JibitConfig) *Jibit {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://napi.jibit.ir/ppg/v3"
	}
	return &Jibit{
		apiKey:      cfg.APIKey,
		secretKey:   cfg.SecretKey,
		callbackURL: cfg.CallbackURL,
		baseURL:     baseURL,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		circuit:     cfg.Circuit,
	}
}

// doHTTP executes an HTTP request through the circuit breaker.
// If no circuit is configured, the request is executed directly.
func (j *Jibit) doHTTP(ctx context.Context, req *http.Request) (*http.Response, error) {
	if j.circuit == nil {
		return j.httpClient.Do(req)
	}
	var resp *http.Response
	err := j.circuit.ExecuteWithContext(ctx, func(_ context.Context) error {
		var e error
		resp, e = j.httpClient.Do(req)
		return e
	})
	return resp, err
}

// Name returns the provider name
func (j *Jibit) Name() ProviderType {
	return ProviderJibit
}

// tokenTimeout is the maximum duration for token generation/refresh HTTP calls
// to prevent holding the write lock indefinitely.
const tokenTimeout = 15 * time.Second

// tokenResult holds the result of a token generation/refresh.
type tokenResult struct {
	accessToken  string
	refreshToken string
	tokenExpiry  time.Time
}

// ensureToken obtains or refreshes the access token if needed.
// Uses singleflight to coalesce concurrent refresh attempts so only one
// goroutine performs the HTTP call while others wait for the shared result.
func (j *Jibit) ensureToken(ctx context.Context) error {
	// Fast path: check if token is still valid under RLock.
	j.mu.RLock()
	valid := j.accessToken != "" && time.Now().Before(j.tokenExpiry)
	j.mu.RUnlock()

	if valid {
		return nil
	}

	// Use singleflight to ensure only one goroutine performs the refresh.
	_, err, _ := j.sfGroup.Do("token", func() (interface{}, error) {
		// Double-check: another goroutine may have refreshed already.
		j.mu.RLock()
		if j.accessToken != "" && time.Now().Before(j.tokenExpiry) {
			j.mu.RUnlock()
			return nil, nil
		}
		j.mu.RUnlock()

		// Perform HTTP token refresh outside any lock.
		tr, err := j.doTokenRefresh(ctx)
		if err != nil {
			return nil, err
		}

		// Write token fields under Lock (brief, no I/O).
		j.mu.Lock()
		j.accessToken = tr.accessToken
		j.refreshToken = tr.refreshToken
		j.tokenExpiry = tr.tokenExpiry
		j.mu.Unlock()

		return nil, nil
	})

	return err
}

// doTokenRefresh performs token generation or refresh without holding any lock.
func (j *Jibit) doTokenRefresh(ctx context.Context) (*tokenResult, error) {
	// Read current refresh token under RLock.
	j.mu.RLock()
	currentRefresh := j.refreshToken
	j.mu.RUnlock()

	// Try to refresh if we have a refresh token.
	if currentRefresh != "" {
		result, err := j.doRefreshToken(ctx, currentRefresh)
		if err == nil {
			return result, nil
		}
		// Refresh failed, fall through to generate new tokens.
	}

	return j.doGenerateToken(ctx)
}

// doGenerateToken obtains a fresh token pair using API key and secret.
func (j *Jibit) doGenerateToken(ctx context.Context) (*tokenResult, error) {
	body, _ := json.Marshal(map[string]string{
		"apiKey":    j.apiKey,
		"secretKey": j.secretKey,
	})

	tctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tctx, "POST", j.baseURL+"/tokens", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jibit token request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.doHTTP(tctx, req)
	if err != nil {
		return nil, fmt.Errorf("jibit token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jibit token response read failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("jibit token error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return parseTokenResult(respBody)
}

// doRefreshToken attempts to refresh the access token using the given refresh token.
func (j *Jibit) doRefreshToken(ctx context.Context, refreshTok string) (*tokenResult, error) {
	body, _ := json.Marshal(map[string]string{
		"refreshToken": refreshTok,
	})

	tctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(tctx, "POST", j.baseURL+"/tokens/refresh", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jibit refresh token request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.doHTTP(tctx, req)
	if err != nil {
		return nil, fmt.Errorf("jibit refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jibit refresh token response read failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("jibit refresh token error: status=%d", resp.StatusCode)
	}

	return parseTokenResult(respBody)
}

// parseTokenResult parses the Jibit token response into a tokenResult.
// It attempts to read the real expiry from the JWT exp claim; falls back to
// a conservative default if parsing fails.
func parseTokenResult(respBody []byte) (*tokenResult, error) {
	var tokenResp struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("jibit token parse failed: %w", err)
	}

	return &tokenResult{
		accessToken:  tokenResp.AccessToken,
		refreshToken: tokenResp.RefreshToken,
		tokenExpiry:  extractTokenExpiry(tokenResp.AccessToken),
	}, nil
}

// extractTokenExpiry parses the JWT access token without signature verification
// (the token is trusted — we just received it from Jibit) and returns the exp
// claim minus a safety buffer. Falls back to defaultTokenTTL on any error.
func extractTokenExpiry(tokenStr string) time.Time {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(tokenStr, jwt.MapClaims{})
	if err != nil {
		return time.Now().Add(defaultTokenTTL)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return time.Now().Add(defaultTokenTTL)
	}

	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return time.Now().Add(defaultTokenTTL)
	}

	return exp.Time.Add(-tokenExpiryBuffer)
}

// getAccessToken returns the current access token under RLock.
func (j *Jibit) getAccessToken() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.accessToken
}

// invalidateToken clears the current access token so the next ensureToken call
// will perform a fresh token refresh.
func (j *Jibit) invalidateToken() {
	j.mu.Lock()
	j.accessToken = ""
	j.tokenExpiry = time.Time{}
	j.mu.Unlock()
}

// doAuthenticatedRequest executes an HTTP request with a valid access token.
// If the server responds with 401 (token expired between ensureToken and the
// actual call), it invalidates the token, refreshes, and retries once.
func (j *Jibit) doAuthenticatedRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	if err := j.ensureToken(ctx); err != nil {
		return nil, err
	}

	makeReq := func() (*http.Request, error) {
		var bodyReader io.Reader
		if body != nil {
			bodyReader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+j.getAccessToken())
		return req, nil
	}

	req, err := makeReq()
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := j.doHTTP(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Token expired between ensureToken and the API call — refresh and retry once.
		resp.Body.Close()
		j.invalidateToken()

		if err := j.ensureToken(ctx); err != nil {
			return nil, fmt.Errorf("token refresh after 401 failed: %w", err)
		}

		req, err = makeReq()
		if err != nil {
			return nil, fmt.Errorf("failed to create retry request: %w", err)
		}

		resp, err = j.doHTTP(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

// jibitPurchaseRequest represents the request to create a purchase
type jibitPurchaseRequest struct {
	Amount                int64  `json:"amount"`
	Wage                  int64  `json:"wage,omitempty"`
	Currency              string `json:"currency"`
	CallbackURL           string `json:"callbackUrl"`
	ClientReferenceNumber string `json:"clientReferenceNumber"`
	UserIdentifier        string `json:"userIdentifier,omitempty"`
	PayerMobileNumber     string `json:"payerMobileNumber,omitempty"`
	Description           string `json:"description,omitempty"`
}

// jibitPurchaseResponse represents the response from creating a purchase
type jibitPurchaseResponse struct {
	PurchaseID            int64  `json:"purchaseId"`
	PurchaseIDStr         string `json:"purchaseIdStr"`
	PspSwitchingURL       string `json:"pspSwitchingUrl"`
	State                 string `json:"state"`
	ClientReferenceNumber string `json:"clientReferenceNumber"`
}

// jibitVerifyResponse represents the response from verifying a purchase
type jibitVerifyResponse struct {
	Status              string `json:"status"`
	Amount              int64  `json:"amount"`
	PspMaskedCardNumber string `json:"pspMaskedCardNumber"`
	PspRrn              string `json:"pspRrn"`
	PspTraceNumber      string `json:"pspTraceNumber"`
}

// jibitFilterResponse represents the response from the filter purchases API
type jibitFilterResponse struct {
	Elements []jibitFilterElement `json:"elements"`
}

// jibitFilterElement represents a single purchase in the filter response
type jibitFilterElement struct {
	PurchaseID          int64  `json:"purchaseId"`
	PurchaseIDStr       string `json:"purchaseIdStr"`
	Amount              int64  `json:"amount"`
	State               string `json:"state"`
	PspMaskedCardNumber string `json:"pspMaskedCardNumber"`
	PspRrn              string `json:"pspRrn"`
	PspTraceNumber      string `json:"pspTraceNumber"`
}

// jibitErrorResponse represents an error envelope returned by Jibit APIs.
type jibitErrorResponse struct {
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// hasErrorCode checks if the error response contains a specific error code.
func (e *jibitErrorResponse) hasErrorCode(code string) bool {
	for _, err := range e.Errors {
		if err.Code == code {
			return true
		}
	}
	return false
}

// CreatePayment creates a Jibit PPG purchase
func (j *Jibit) CreatePayment(ctx context.Context, req *CreatePaymentRequest) (*CreatePaymentResponse, error) {
	purchaseReq := jibitPurchaseRequest{
		Amount:                req.AmountCents, // Already in Rials (IRR)
		Currency:              "IRR",
		CallbackURL:           j.callbackURL,
		ClientReferenceNumber: req.OrderID,
		Description:           req.Description,
	}
	if req.CustomerPhone != "" {
		purchaseReq.PayerMobileNumber = req.CustomerPhone
	}
	if req.UserID != "" {
		purchaseReq.UserIdentifier = req.UserID
	}

	body, err := json.Marshal(purchaseReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := j.doAuthenticatedRequest(ctx, "POST", j.baseURL+"/purchases", body)
	if err != nil {
		return nil, fmt.Errorf("jibit create purchase failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("jibit error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var purchaseResp jibitPurchaseResponse
	if err := json.Unmarshal(respBody, &purchaseResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Prefer string ID from Jibit to avoid int64 overflow for very large purchase IDs
	providerID := purchaseResp.PurchaseIDStr
	if providerID == "" {
		providerID = fmt.Sprintf("%d", purchaseResp.PurchaseID)
	}

	return &CreatePaymentResponse{
		ProviderPaymentID: providerID,
		PaymentURL:        purchaseResp.PspSwitchingURL, // Redirect user here
		Status:            PaymentStatusPending,
		Metadata: map[string]string{
			"state":      purchaseResp.State,
			"purchaseId": providerID,
		},
	}, nil
}

// GetPaymentStatus retrieves the current status of a purchase using the filter API.
// This is a read-only query — unlike /purchases/{id}/verify which is a one-time
// mutation that transitions the purchase state.
func (j *Jibit) GetPaymentStatus(ctx context.Context, providerPaymentID string) (*PaymentStatusResponse, error) {
	filterURL := fmt.Sprintf("%s/purchases?purchaseId=%s&size=1", j.baseURL, providerPaymentID)

	resp, err := j.doAuthenticatedRequest(ctx, "GET", filterURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jibit filter request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var filterResp jibitFilterResponse
	if err := json.Unmarshal(respBody, &filterResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(filterResp.Elements) == 0 {
		return nil, ErrPaymentNotFound
	}

	elem := filterResp.Elements[0]
	return &PaymentStatusResponse{
		ProviderPaymentID: providerPaymentID,
		Status:            j.mapStatus(elem.State),
		AmountCents:       elem.Amount,
		Currency:          "IRR",
		RefNumber:         elem.PspRrn,
		Metadata: map[string]string{
			"masked_card":  elem.PspMaskedCardNumber,
			"rrn":          elem.PspRrn,
			"trace_number": elem.PspTraceNumber,
		},
	}, nil
}

// VerifyWebhook handles Jibit callback (POST to callbackUrl).
// Jibit PPG v3 sends callbacks as application/x-www-form-urlencoded.
// JSON parsing is kept as a fallback for backwards compatibility.
func (j *Jibit) VerifyWebhook(ctx context.Context, headers map[string]string, body []byte) (*WebhookEvent, error) {
	var purchaseID string
	var callbackRaw interface{} // For RawData logging

	contentType := headers["content-type"]
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		// Jibit PPG v3 sends callbacks as form-urlencoded
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, fmt.Errorf("invalid jibit form callback: %w", err)
		}
		purchaseID = values.Get("purchaseId")

		// Store form values for raw data logging
		formData := make(map[string]interface{})
		for key, vals := range values {
			if len(vals) > 0 {
				formData[key] = vals[0]
			}
		}
		callbackRaw = formData
	} else {
		// Fallback to JSON for backwards compatibility
		var callback struct {
			PurchaseID string `json:"purchaseId"`
			Amount     int64  `json:"amount"`
			Status     string `json:"status"`
		}
		if err := json.Unmarshal(body, &callback); err != nil {
			return nil, fmt.Errorf("invalid jibit callback: %w", err)
		}
		purchaseID = callback.PurchaseID

		// Also try numeric purchaseId
		if purchaseID == "" {
			var numericCallback struct {
				PurchaseID int64 `json:"purchaseId"`
			}
			if err := json.Unmarshal(body, &numericCallback); err == nil && numericCallback.PurchaseID != 0 {
				purchaseID = strconv.FormatInt(numericCallback.PurchaseID, 10)
			}
		}

		var rawData map[string]interface{}
		_ = json.Unmarshal(body, &rawData)
		callbackRaw = rawData
	}

	if purchaseID == "" {
		return nil, fmt.Errorf("invalid jibit callback: missing purchaseId")
	}

	// Extract clientReferenceNumber from callback for direct order matching.
	// This is the payment_intent_id we set during CreatePayment.
	// URL-decode in case the value was URL-encoded (per Jibit docs).
	// The form-urlencoded path auto-decodes via url.ParseQuery, but
	// the JSON fallback path does not.
	var clientReferenceNumber string
	if formData, ok := callbackRaw.(map[string]interface{}); ok {
		if crn, ok := formData["clientReferenceNumber"].(string); ok {
			if decoded, err := url.QueryUnescape(crn); err == nil {
				clientReferenceNumber = decoded
			} else {
				clientReferenceNumber = crn
			}
		}
	}

	// Extract callback status to decide whether verification is needed.
	// Jibit sends: SUCCESSFUL, FAILED, or UNKNOWN in the callback.
	var callbackStatus string
	if formData, ok := callbackRaw.(map[string]interface{}); ok {
		if s, ok := formData["status"].(string); ok {
			callbackStatus = s
		}
	}

	// If the callback reports FAILED, skip the verify call entirely.
	// Per Jibit docs, only READY_TO_VERIFY state can be verified.
	if callbackStatus == "FAILED" {
		rawData := map[string]interface{}{
			"callback":        callbackRaw,
			"callback_status": callbackStatus,
		}
		if formData, ok := callbackRaw.(map[string]interface{}); ok {
			for _, key := range []string{"payerIp", "pspHashedCardNumber", "pspName", "pspReferenceNumber", "clientReferenceNumber"} {
				if v, ok := formData[key]; ok {
					rawData[key] = v
				}
			}
		}
		return &WebhookEvent{
			Provider:          ProviderJibit,
			ProviderPaymentID: purchaseID,
			OrderID:           clientReferenceNumber,
			Status:            PaymentStatusFailed,
			Currency:          "IRR",
			RawData:           rawData,
		}, nil
	}

	// Verify the purchase server-side
	verifyURL := fmt.Sprintf("%s/purchases/%s/verify", j.baseURL, purchaseID)

	resp, err := j.doAuthenticatedRequest(ctx, "GET", verifyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jibit verify failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read verify response: %w", err)
	}

	// If verify returns non-2xx, check for payment.already_verified error.
	// When auto-verify is enabled, calling verify again returns this error.
	// Fall back to the filter API to get the actual purchase state.
	if resp.StatusCode >= 400 {
		var errResp jibitErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.hasErrorCode("payment.already_verified") {
			statusResp, err := j.GetPaymentStatus(ctx, purchaseID)
			if err != nil {
				return nil, fmt.Errorf("jibit verify already done, filter fallback failed: %w", err)
			}
			rawData := map[string]interface{}{
				"masked_card":    statusResp.Metadata["masked_card"],
				"rrn":            statusResp.Metadata["rrn"],
				"trace_number":   statusResp.Metadata["trace_number"],
				"callback":       callbackRaw,
				"verify_skipped": "already_verified",
			}
			if formData, ok := callbackRaw.(map[string]interface{}); ok {
				for _, key := range []string{"payerIp", "pspHashedCardNumber", "pspName", "pspReferenceNumber", "clientReferenceNumber"} {
					if v, ok := formData[key]; ok {
						rawData[key] = v
					}
				}
				if s, ok := formData["status"].(string); ok {
					rawData["callback_status"] = s
				}
			}
			return &WebhookEvent{
				Provider:          ProviderJibit,
				ProviderPaymentID: purchaseID,
				OrderID:           clientReferenceNumber,
				Status:            statusResp.Status,
				AmountCents:       statusResp.AmountCents,
				Currency:          "IRR",
				RefNumber:         statusResp.RefNumber,
				RawData:           rawData,
			}, nil
		}
		return nil, fmt.Errorf("jibit verify error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var verifyResp jibitVerifyResponse
	if err := json.Unmarshal(respBody, &verifyResp); err != nil {
		return nil, fmt.Errorf("failed to parse verify response: %w", err)
	}

	status := j.mapStatus(verifyResp.Status)

	// Build RawData with verify response fields and promoted callback fields
	rawData := map[string]interface{}{
		"masked_card":  verifyResp.PspMaskedCardNumber,
		"rrn":          verifyResp.PspRrn,
		"trace_number": verifyResp.PspTraceNumber,
		"callback":     callbackRaw,
	}

	// Promote important callback fields to top level for reconciliation and fraud detection
	if formData, ok := callbackRaw.(map[string]interface{}); ok {
		for _, key := range []string{"payerIp", "pspHashedCardNumber", "pspName", "pspReferenceNumber", "clientReferenceNumber"} {
			if v, ok := formData[key]; ok {
				rawData[key] = v
			}
		}
		// Promote callback status for easier comparison with verify status
		if s, ok := formData["status"].(string); ok {
			rawData["callback_status"] = s
		}
	}

	return &WebhookEvent{
		Provider:          ProviderJibit,
		ProviderPaymentID: purchaseID,
		OrderID:           clientReferenceNumber,
		Status:            status,
		AmountCents:       verifyResp.Amount,
		Currency:          "IRR",
		RefNumber:         verifyResp.PspRrn,
		RawData:           rawData,
	}, nil
}

// CreatePayout creates a bank transfer payout (not supported by Jibit PPG)
func (j *Jibit) CreatePayout(ctx context.Context, req *PayoutRequest) (*PayoutResponse, error) {
	return nil, fmt.Errorf("jibit PPG does not support automated payouts")
}

// GetPayoutStatus retrieves the status of a payout
func (j *Jibit) GetPayoutStatus(ctx context.Context, providerPayoutID string) (*PayoutResponse, error) {
	return nil, fmt.Errorf("jibit PPG does not support automated payouts")
}

// jibitRefundRequest represents the request body for POST /purchases/refund
type jibitRefundRequest struct {
	ClientReferenceNumber string `json:"clientReferenceNumber"`
	PurchaseID            int64  `json:"purchaseId"`
	Amount                int64  `json:"amount"`
	Cancellable           bool   `json:"cancellable"`
}

// jibitReverseRequest represents the request body for POST /purchases/reverse
type jibitReverseRequest struct {
	PurchaseID int64 `json:"purchaseId"`
}

// RefundPayment issues a partial or full refund via Jibit PPG v3.
// POST /purchases/refund
func (j *Jibit) RefundPayment(ctx context.Context, req *RefundRequest) (*RefundResponse, error) {
	purchaseID, err := strconv.ParseInt(req.ProviderPaymentID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid purchaseId %q: %w", req.ProviderPaymentID, err)
	}

	refundReq := jibitRefundRequest{
		ClientReferenceNumber: req.OrderID,
		PurchaseID:            purchaseID,
		Amount:                req.AmountCents,
		Cancellable:           req.Cancellable,
	}

	body, err := json.Marshal(refundReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refund request: %w", err)
	}

	resp, err := j.doAuthenticatedRequest(ctx, "POST", j.baseURL+"/purchases/refund", body)
	if err != nil {
		return nil, fmt.Errorf("jibit refund request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read refund response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jibit refund error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return &RefundResponse{
		Status: PaymentStatusRefunded,
		Metadata: map[string]string{
			"purchaseId": req.ProviderPaymentID,
			"response":   string(respBody),
		},
	}, nil
}

// ReversePayment fully reverses a payment via Jibit PPG v3.
// POST /purchases/reverse — must be called before settlement window closes.
func (j *Jibit) ReversePayment(ctx context.Context, purchaseIDStr string) (*RefundResponse, error) {
	purchaseID, err := strconv.ParseInt(purchaseIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid purchaseId %q: %w", purchaseIDStr, err)
	}

	reverseReq := jibitReverseRequest{
		PurchaseID: purchaseID,
	}

	body, err := json.Marshal(reverseReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reverse request: %w", err)
	}

	resp, err := j.doAuthenticatedRequest(ctx, "POST", j.baseURL+"/purchases/reverse", body)
	if err != nil {
		return nil, fmt.Errorf("jibit reverse request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read reverse response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jibit reverse error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return &RefundResponse{
		Status: PaymentStatusRefunded,
		Metadata: map[string]string{
			"purchaseId": purchaseIDStr,
			"response":   string(respBody),
		},
	}, nil
}

// IsAvailable checks if Jibit is available using the dedicated health endpoint.
// Per Jibit docs, /app/health requires a Bearer JWT token.
func (j *Jibit) IsAvailable(ctx context.Context) bool {
	resp, err := j.doAuthenticatedRequest(ctx, "GET", j.baseURL+"/app/health", nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body) // Drain body for connection reuse

	return resp.StatusCode < 500
}

// SupportedCurrencies returns supported currencies
func (j *Jibit) SupportedCurrencies() []string {
	return []string{"IRR"} // Iranian Rial only
}

// mapStatus maps Jibit purchase states to internal payment status.
// Jibit has two sets of status values:
// 1. Callback redirect (POST to callbackUrl): SUCCESSFUL, FAILED, UNKNOWN
// 2. Purchase state (Filter/Verify API): IN_PROGRESS, READY_TO_VERIFY,
//    SUCCESS, FAILED, EXPIRED, REVERSED, UNKNOWN, MANUALLY_SUCCESS
func (j *Jibit) mapStatus(status string) PaymentStatus {
	switch status {
	case "SUCCESSFUL", "ALREADY_VERIFIED", "SUCCESS", "MANUALLY_SUCCESS":
		return PaymentStatusFinished
	case "FAILED":
		return PaymentStatusFailed
	case "EXPIRED":
		return PaymentStatusExpired
	case "REVERSED":
		return PaymentStatusRefunded
	case "IN_PROGRESS", "READY_TO_VERIFY":
		return PaymentStatusPending
	case "UNKNOWN":
		// Per Jibit docs: UNKNOWN requires periodic inquiry via the filter API
		// until it resolves to a final state. The inquiry worker in inquiry.go
		// handles this by polling processing intents every 5 minutes.
		return PaymentStatusPending
	default:
		return PaymentStatusPending
	}
}
