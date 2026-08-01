package kyc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"time"
)

// ErrUnauthorized is returned when the Jibit API responds with HTTP 401.
var ErrUnauthorized = errors.New("jibit: unauthorized (401)")

const (
	defaultJibitBaseURL = "https://napi.jibit.ir"
	tokenEndpoint       = "/ide/v1/tokens/generate"
	refreshEndpoint     = "/ide/v1/tokens/refresh"
	shahkarEndpoint     = "/ide/v1/services/shahkar"
	identityEndpoint    = "/ide/v1/services/identity-info"
	cardToNIDEndpoint   = "/ide/v1/services/card-to-nid"
	faceVerifyEndpoint  = "/ide/v1/services/face-verification"
	cardOCREndpoint     = "/ide/v1/services/national-card-ocr"

	// tokenRefreshBuffer is subtracted from the actual expiry to refresh early.
	tokenRefreshBuffer = 60 * time.Second
)

// JibitKYCConfig holds the configuration for the Jibit KYC provider.
type JibitKYCConfig struct {
	APIKey    string
	SecretKey string
	BaseURL   string // defaults to https://napi.jibit.ir
}

// JibitKYCProvider provides Jibit identity verification services.
type JibitKYCProvider struct {
	apiKey       string
	secretKey    string
	baseURL      string
	httpClient   *http.Client
	mu           sync.RWMutex
	accessToken  string
	refreshToken string
	tokenExpiry  time.Time
}

// NewJibitKYCProvider creates a new Jibit KYC provider.
func NewJibitKYCProvider(cfg JibitKYCConfig) *JibitKYCProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultJibitBaseURL
	}
	return &JibitKYCProvider{
		apiKey:    cfg.APIKey,
		secretKey: cfg.SecretKey,
		baseURL:   baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// invalidateToken clears the cached access token, forcing re-authentication
// on the next ensureToken call.
func (j *JibitKYCProvider) invalidateToken() {
	j.mu.Lock()
	j.accessToken = ""
	j.tokenExpiry = time.Time{}
	j.mu.Unlock()
}

// ensureToken obtains or refreshes the access token if needed.
func (j *JibitKYCProvider) ensureToken(ctx context.Context) error {
	j.mu.RLock()
	valid := j.accessToken != "" && time.Now().Before(j.tokenExpiry)
	j.mu.RUnlock()

	if valid {
		return nil
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	// Double-check after acquiring write lock.
	if j.accessToken != "" && time.Now().Before(j.tokenExpiry) {
		return nil
	}

	// Try to refresh if we have a refresh token.
	if j.refreshToken != "" {
		if err := j.doRefreshToken(ctx); err == nil {
			return nil
		}
		// Refresh failed, fall through to generate new tokens.
	}

	return j.doGenerateToken(ctx)
}

func (j *JibitKYCProvider) doGenerateToken(ctx context.Context) error {
	body, err := json.Marshal(JibitTokenRequest{
		APIKey:    j.apiKey,
		SecretKey: j.secretKey,
	})
	if err != nil {
		return fmt.Errorf("jibit: marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", j.baseURL+tokenEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("jibit: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jibit: token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("jibit: read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp JibitErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		return fmt.Errorf("jibit: token generation failed (status %d): %s", resp.StatusCode, errResp.Message)
	}

	var tokenResp JibitTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return fmt.Errorf("jibit: unmarshal token response: %w", err)
	}

	j.accessToken = tokenResp.AccessToken
	j.refreshToken = tokenResp.RefreshToken
	j.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - tokenRefreshBuffer)

	return nil
}

func (j *JibitKYCProvider) doRefreshToken(ctx context.Context) error {
	body, err := json.Marshal(JibitRefreshRequest{
		AccessToken:  j.accessToken,
		RefreshToken: j.refreshToken,
	})
	if err != nil {
		return fmt.Errorf("jibit: marshal refresh request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", j.baseURL+refreshEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("jibit: create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jibit: refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("jibit: read refresh response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jibit: refresh failed (status %d)", resp.StatusCode)
	}

	var tokenResp JibitTokenResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return fmt.Errorf("jibit: unmarshal refresh response: %w", err)
	}

	j.accessToken = tokenResp.AccessToken
	j.refreshToken = tokenResp.RefreshToken
	j.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second - tokenRefreshBuffer)

	return nil
}

// doJSONRequest performs an authenticated JSON POST request to the Jibit API.
func (j *JibitKYCProvider) doJSONRequest(ctx context.Context, endpoint string, payload interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("jibit: marshal request: %w", err)
	}

	j.mu.RLock()
	token := j.accessToken
	j.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "POST", j.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jibit: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jibit: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jibit: read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		var errResp JibitErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		return nil, fmt.Errorf("jibit: API error (status %d): %s - %s", resp.StatusCode, errResp.Code, errResp.Message)
	}

	return respBody, nil
}

// doJSONRequestWithRetry performs an authenticated JSON request with a single
// retry on 401 Unauthorized. On 401, it invalidates the cached token, calls
// ensureToken to obtain a fresh one, and retries the request once.
func (j *JibitKYCProvider) doJSONRequestWithRetry(ctx context.Context, endpoint string, payload interface{}) ([]byte, error) {
	body, err := j.doJSONRequest(ctx, endpoint, payload)
	if err == nil {
		return body, nil
	}
	if !errors.Is(err, ErrUnauthorized) {
		return nil, err
	}

	j.invalidateToken()
	if err := j.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("jibit: re-auth after 401 failed: %w", err)
	}

	return j.doJSONRequest(ctx, endpoint, payload)
}

// doMultipartRequest performs an authenticated multipart POST request.
func (j *JibitKYCProvider) doMultipartRequest(ctx context.Context, endpoint string, body *bytes.Buffer, contentType string) ([]byte, error) {
	j.mu.RLock()
	token := j.accessToken
	j.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "POST", j.baseURL+endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("jibit: create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jibit: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jibit: read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		var errResp JibitErrorResponse
		_ = json.Unmarshal(respBody, &errResp)
		return nil, fmt.Errorf("jibit: API error (status %d): %s - %s", resp.StatusCode, errResp.Code, errResp.Message)
	}

	return respBody, nil
}

// doMultipartRequestWithRetry performs a multipart request with a single
// retry on 401. The buildBody function is called to construct the request
// body (since the buffer is consumed on each attempt).
func (j *JibitKYCProvider) doMultipartRequestWithRetry(ctx context.Context, endpoint string, buildBody func() (*bytes.Buffer, string, error)) ([]byte, error) {
	buf, contentType, err := buildBody()
	if err != nil {
		return nil, err
	}

	respBody, err := j.doMultipartRequest(ctx, endpoint, buf, contentType)
	if err == nil {
		return respBody, nil
	}
	if !errors.Is(err, ErrUnauthorized) {
		return nil, err
	}

	j.invalidateToken()
	if err := j.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("jibit: re-auth after 401 failed: %w", err)
	}

	buf, contentType, err = buildBody()
	if err != nil {
		return nil, err
	}

	return j.doMultipartRequest(ctx, endpoint, buf, contentType)
}

// VerifyShahkar checks whether a phone number belongs to the given national code.
func (j *JibitKYCProvider) VerifyShahkar(ctx context.Context, phone, nationalCode string) (*ShahkarResult, error) {
	if err := ValidateIranianPhoneNumber(phone); err != nil {
		return nil, fmt.Errorf("jibit shahkar: %w", err)
	}
	if err := ValidateIranianNationalCode(nationalCode); err != nil {
		return nil, fmt.Errorf("jibit shahkar: %w", err)
	}

	if err := j.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("jibit shahkar: %w", err)
	}

	respBody, err := j.doJSONRequestWithRetry(ctx, shahkarEndpoint, ShahkarRequest{
		MobileNumber: phone,
		NationalCode: nationalCode,
	})
	if err != nil {
		return nil, fmt.Errorf("jibit shahkar: %w", err)
	}

	var result ShahkarResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("jibit shahkar: unmarshal response: %w", err)
	}
	return &result, nil
}

// GetIdentityInfo retrieves identity information for a national code and birth date.
func (j *JibitKYCProvider) GetIdentityInfo(ctx context.Context, nationalCode, birthDate string) (*IdentityInfoResult, error) {
	if err := ValidateIranianNationalCode(nationalCode); err != nil {
		return nil, fmt.Errorf("jibit identity-info: %w", err)
	}

	if err := j.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("jibit identity-info: %w", err)
	}

	respBody, err := j.doJSONRequestWithRetry(ctx, identityEndpoint, IdentityInfoRequest{
		NationalCode: nationalCode,
		BirthDate:    birthDate,
	})
	if err != nil {
		return nil, fmt.Errorf("jibit identity-info: %w", err)
	}

	var result IdentityInfoResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("jibit identity-info: unmarshal response: %w", err)
	}
	return &result, nil
}

// VerifyCardToNID checks whether a bank card belongs to the given national code.
func (j *JibitKYCProvider) VerifyCardToNID(ctx context.Context, cardNumber, nationalCode string) (*CardToNIDResult, error) {
	if err := ValidateCardNumber(cardNumber); err != nil {
		return nil, fmt.Errorf("jibit card-to-nid: %w", err)
	}
	if err := ValidateIranianNationalCode(nationalCode); err != nil {
		return nil, fmt.Errorf("jibit card-to-nid: %w", err)
	}

	if err := j.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("jibit card-to-nid: %w", err)
	}

	respBody, err := j.doJSONRequestWithRetry(ctx, cardToNIDEndpoint, CardToNIDRequest{
		CardNumber:   cardNumber,
		NationalCode: nationalCode,
	})
	if err != nil {
		return nil, fmt.Errorf("jibit card-to-nid: %w", err)
	}

	var result CardToNIDResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("jibit card-to-nid: unmarshal response: %w", err)
	}
	return &result, nil
}

// VerifyFace performs biometric face verification with liveness detection.
func (j *JibitKYCProvider) VerifyFace(ctx context.Context, nationalCode string, selfieImage []byte) (*FaceVerificationResult, error) {
	if err := ValidateIranianNationalCode(nationalCode); err != nil {
		return nil, fmt.Errorf("jibit face-verification: %w", err)
	}
	if len(selfieImage) == 0 {
		return nil, fmt.Errorf("jibit face-verification: %w", ErrEmptyImage)
	}

	if err := j.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("jibit face-verification: %w", err)
	}

	buildBody := func() (*bytes.Buffer, string, error) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		if err := writer.WriteField("nationalCode", nationalCode); err != nil {
			return nil, "", fmt.Errorf("jibit face-verification: write nationalCode field: %w", err)
		}
		part, err := writer.CreateFormFile("selfieImage", "selfie.jpg")
		if err != nil {
			return nil, "", fmt.Errorf("jibit face-verification: create form file: %w", err)
		}
		if _, err := part.Write(selfieImage); err != nil {
			return nil, "", fmt.Errorf("jibit face-verification: write selfie data: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, "", fmt.Errorf("jibit face-verification: close writer: %w", err)
		}
		return &buf, writer.FormDataContentType(), nil
	}

	respBody, err := j.doMultipartRequestWithRetry(ctx, faceVerifyEndpoint, buildBody)
	if err != nil {
		return nil, fmt.Errorf("jibit face-verification: %w", err)
	}

	var result FaceVerificationResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("jibit face-verification: unmarshal response: %w", err)
	}
	return &result, nil
}

// OCRNationalCard extracts text from a smart national card front image.
func (j *JibitKYCProvider) OCRNationalCard(ctx context.Context, frontImage []byte) (*NationalCardOCRResult, error) {
	if len(frontImage) == 0 {
		return nil, fmt.Errorf("jibit national-card-ocr: %w", ErrEmptyImage)
	}

	if err := j.ensureToken(ctx); err != nil {
		return nil, fmt.Errorf("jibit national-card-ocr: %w", err)
	}

	buildBody := func() (*bytes.Buffer, string, error) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, err := writer.CreateFormFile("frontImage", "front.jpg")
		if err != nil {
			return nil, "", fmt.Errorf("jibit national-card-ocr: create form file: %w", err)
		}
		if _, err := part.Write(frontImage); err != nil {
			return nil, "", fmt.Errorf("jibit national-card-ocr: write image data: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, "", fmt.Errorf("jibit national-card-ocr: close writer: %w", err)
		}
		return &buf, writer.FormDataContentType(), nil
	}

	respBody, err := j.doMultipartRequestWithRetry(ctx, cardOCREndpoint, buildBody)
	if err != nil {
		return nil, fmt.Errorf("jibit national-card-ocr: %w", err)
	}

	var result NationalCardOCRResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("jibit national-card-ocr: unmarshal response: %w", err)
	}
	return &result, nil
}
