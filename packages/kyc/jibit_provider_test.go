package kyc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestProvider creates a JibitKYCProvider pointed at the given test server
// with a pre-seeded valid token so ensureToken is already satisfied.
func newTestProvider(serverURL string) *JibitKYCProvider {
	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   serverURL,
	})
	// Pre-seed a valid token so ensureToken doesn't need to call the server.
	p.accessToken = "initial-token"
	p.tokenExpiry = time.Now().Add(1 * time.Hour)
	return p
}

func TestDoJSONRequestWithRetry_Success(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"matched": true})
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)
	body, err := p.doJSONRequestWithRetry(context.Background(), "/test", map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body == nil {
		t.Fatal("expected non-nil body")
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestDoJSONRequestWithRetry_401ThenSuccess(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)

		// Token generation endpoint
		if r.URL.Path == tokenEndpoint {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "new-token",
				RefreshToken: "new-refresh",
				ExpiresIn:    3600,
			})
			return
		}

		// First API call returns 401, second succeeds
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"matched": true})
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)
	body, err := p.doJSONRequestWithRetry(context.Background(), "/test", map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body == nil {
		t.Fatal("expected non-nil body on retry")
	}
}

func TestDoJSONRequestWithRetry_401ThenFailAgain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Token generation returns a token
		if r.URL.Path == tokenEndpoint {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "new-token",
				RefreshToken: "new-refresh",
				ExpiresIn:    3600,
			})
			return
		}
		// Always return 401 for API calls
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)
	_, err := p.doJSONRequestWithRetry(context.Background(), "/test", map[string]string{"key": "val"})
	if err == nil {
		t.Fatal("expected error on double 401")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestDoMultipartRequestWithRetry_401ThenSuccess(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)

		if r.URL.Path == tokenEndpoint {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "new-token",
				RefreshToken: "new-refresh",
				ExpiresIn:    3600,
			})
			return
		}

		// First multipart call returns 401, second succeeds
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"matched": true})
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)

	buildCallCount := 0
	buildBody := func() (*bytes.Buffer, string, error) {
		buildCallCount++
		var buf bytes.Buffer
		buf.WriteString("test-body")
		return &buf, "application/octet-stream", nil
	}

	body, err := p.doMultipartRequestWithRetry(context.Background(), "/test", buildBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body == nil {
		t.Fatal("expected non-nil body on retry")
	}
	if buildCallCount != 2 {
		t.Errorf("expected buildBody called 2 times, got %d", buildCallCount)
	}
}

func TestInvalidateToken(t *testing.T) {
	p := &JibitKYCProvider{
		accessToken: "some-token",
		tokenExpiry: time.Now().Add(1 * time.Hour),
	}

	p.invalidateToken()

	if p.accessToken != "" {
		t.Errorf("expected empty access token, got %q", p.accessToken)
	}
	if !p.tokenExpiry.IsZero() {
		t.Errorf("expected zero token expiry, got %v", p.tokenExpiry)
	}
}

func TestDoJSONRequest_Returns_ErrUnauthorized_On401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)
	_, err := p.doJSONRequest(context.Background(), "/test", map[string]string{"key": "val"})
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got: %v", err)
	}
}

func TestDoJSONRequest_NonRetryableError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(JibitErrorResponse{Code: "BAD_REQ", Message: "bad request"})
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)
	_, err := p.doJSONRequestWithRetry(context.Background(), "/test", map[string]string{"key": "val"})
	if err == nil {
		t.Fatal("expected error on 400")
	}
	// Should NOT be ErrUnauthorized
	if errors.Is(err, ErrUnauthorized) {
		t.Error("400 error should not be ErrUnauthorized")
	}
}

// ---------------------------------------------------------------------------
// Token management tests
// ---------------------------------------------------------------------------

func TestEnsureToken_ValidToken_NoRefresh(t *testing.T) {
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		t.Error("ensureToken should not make HTTP calls when token is valid")
	}))
	defer srv.Close()

	p := newTestProvider(srv.URL)
	err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 0 {
		t.Errorf("expected 0 HTTP calls, got %d", callCount)
	}
}

func TestEnsureToken_ExpiredToken_GeneratesNew(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "fresh-token",
				RefreshToken: "fresh-refresh",
				ExpiresIn:    3600,
			})
			return
		}
		t.Errorf("unexpected request to %s", r.URL.Path)
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})
	// Set expired token, no refresh token
	p.accessToken = "expired-token"
	p.tokenExpiry = time.Now().Add(-1 * time.Hour)

	err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.accessToken != "fresh-token" {
		t.Errorf("expected fresh-token, got %q", p.accessToken)
	}
	if p.refreshToken != "fresh-refresh" {
		t.Errorf("expected fresh-refresh, got %q", p.refreshToken)
	}
}

func TestEnsureToken_ExpiredToken_RefreshesFirst(t *testing.T) {
	var refreshCalled, generateCalled int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == refreshEndpoint {
			atomic.AddInt32(&refreshCalled, 1)
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "refreshed-token",
				RefreshToken: "refreshed-refresh",
				ExpiresIn:    3600,
			})
			return
		}
		if r.URL.Path == tokenEndpoint {
			atomic.AddInt32(&generateCalled, 1)
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "generated-token",
				RefreshToken: "generated-refresh",
				ExpiresIn:    3600,
			})
			return
		}
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})
	p.accessToken = "expired-token"
	p.refreshToken = "old-refresh"
	p.tokenExpiry = time.Now().Add(-1 * time.Hour)

	err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&refreshCalled) != 1 {
		t.Errorf("expected refresh to be called once, got %d", refreshCalled)
	}
	if atomic.LoadInt32(&generateCalled) != 0 {
		t.Error("generate should not be called when refresh succeeds")
	}
	if p.accessToken != "refreshed-token" {
		t.Errorf("expected refreshed-token, got %q", p.accessToken)
	}
}

func TestEnsureToken_RefreshFails_FallsToGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == refreshEndpoint {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path == tokenEndpoint {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "generated-token",
				RefreshToken: "generated-refresh",
				ExpiresIn:    3600,
			})
			return
		}
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})
	p.accessToken = "expired-token"
	p.refreshToken = "bad-refresh"
	p.tokenExpiry = time.Now().Add(-1 * time.Hour)

	err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.accessToken != "generated-token" {
		t.Errorf("expected generated-token, got %q", p.accessToken)
	}
}

func TestEnsureToken_DoubleCheckLocking(t *testing.T) {
	var tokenCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			atomic.AddInt32(&tokenCalls, 1)
			// Small delay to increase chance of concurrent contention
			time.Sleep(10 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "shared-token",
				RefreshToken: "shared-refresh",
				ExpiresIn:    3600,
			})
			return
		}
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})
	// No token set — all goroutines will race to acquire

	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := p.ensureToken(context.Background()); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("ensureToken error: %v", err)
	}

	// Due to double-check locking, only 1 token generation call should be made
	calls := atomic.LoadInt32(&tokenCalls)
	if calls != 1 {
		t.Errorf("expected exactly 1 token generation call (double-check locking), got %d", calls)
	}
}

func TestTokenExpiryBuffer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "token",
				RefreshToken: "refresh",
				ExpiresIn:    3600, // 1 hour
			})
			return
		}
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})

	before := time.Now()
	err := p.ensureToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now()

	// Token expiry should be approximately now + 3600s - 60s = now + 3540s
	expectedLow := before.Add(3600*time.Second - tokenRefreshBuffer - 1*time.Second)
	expectedHigh := after.Add(3600*time.Second - tokenRefreshBuffer + 1*time.Second)

	if p.tokenExpiry.Before(expectedLow) || p.tokenExpiry.After(expectedHigh) {
		t.Errorf("token expiry %v not in expected range [%v, %v] (buffer=%v)",
			p.tokenExpiry, expectedLow, expectedHigh, tokenRefreshBuffer)
	}
}

func TestDoGenerateToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tokenEndpoint {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}

		var req JibitTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			return
		}
		if req.APIKey != "test-key" || req.SecretKey != "test-secret" {
			t.Errorf("unexpected credentials: key=%q secret=%q", req.APIKey, req.SecretKey)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(JibitTokenResponse{
			AccessToken:  "gen-token",
			RefreshToken: "gen-refresh",
			ExpiresIn:    7200,
		})
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})

	err := p.doGenerateToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.accessToken != "gen-token" {
		t.Errorf("expected gen-token, got %q", p.accessToken)
	}
	if p.refreshToken != "gen-refresh" {
		t.Errorf("expected gen-refresh, got %q", p.refreshToken)
	}
}

func TestDoGenerateToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(JibitErrorResponse{Code: "ERR", Message: "server error"})
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{
		APIKey:    "test-key",
		SecretKey: "test-secret",
		BaseURL:   srv.URL,
	})

	err := p.doGenerateToken(context.Background())
	if err == nil {
		t.Fatal("expected error on server error")
	}
	if !strings.Contains(err.Error(), "token generation failed") {
		t.Errorf("expected 'token generation failed' in error, got: %v", err)
	}
}

func TestDoRefreshToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != refreshEndpoint {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(JibitTokenResponse{
			AccessToken:  "ref-token",
			RefreshToken: "ref-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{BaseURL: srv.URL})
	p.accessToken = "old-access"
	p.refreshToken = "old-refresh"

	err := p.doRefreshToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.accessToken != "ref-token" {
		t.Errorf("expected ref-token, got %q", p.accessToken)
	}
	if p.refreshToken != "ref-refresh" {
		t.Errorf("expected ref-refresh, got %q", p.refreshToken)
	}
}

func TestDoRefreshToken_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	p := NewJibitKYCProvider(JibitKYCConfig{BaseURL: srv.URL})
	p.accessToken = "old-access"
	p.refreshToken = "old-refresh"

	err := p.doRefreshToken(context.Background())
	if err == nil {
		t.Fatal("expected error on refresh failure")
	}
	// Tokens should remain unchanged on failure
	if p.accessToken != "old-access" {
		t.Errorf("access token should remain unchanged, got %q", p.accessToken)
	}
}

// ---------------------------------------------------------------------------
// Verification endpoint tests
// ---------------------------------------------------------------------------

// tokenAwareServer returns an httptest server that handles token generation
// and delegates API requests to the given handler.
func tokenAwareServer(t *testing.T, apiHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint || r.URL.Path == refreshEndpoint {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(JibitTokenResponse{
				AccessToken:  "test-token",
				RefreshToken: "test-refresh",
				ExpiresIn:    3600,
			})
			return
		}
		apiHandler(w, r)
	}))
}

func TestVerifyShahkar_Success(t *testing.T) {
	srv := tokenAwareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != shahkarEndpoint {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		var req ShahkarRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ShahkarResult{
			Matched:        true,
			MobileOperator: "MCI",
			TransactionID:  "tx-123",
		})
	})
	defer srv.Close()

	p := newTestProvider(srv.URL)
	result, err := p.VerifyShahkar(context.Background(), "09123456789", "0499370899")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Error("expected matched=true")
	}
	if result.MobileOperator != "MCI" {
		t.Errorf("expected MCI, got %q", result.MobileOperator)
	}
}

func TestVerifyShahkar_InvalidPhone(t *testing.T) {
	p := NewJibitKYCProvider(JibitKYCConfig{BaseURL: "http://unused"})
	_, err := p.VerifyShahkar(context.Background(), "invalid", "0499370899")
	if err == nil {
		t.Fatal("expected error for invalid phone")
	}
	if !errors.Is(err, ErrInvalidPhoneNumber) {
		t.Errorf("expected ErrInvalidPhoneNumber, got: %v", err)
	}
}

func TestVerifyShahkar_InvalidNationalCode(t *testing.T) {
	p := NewJibitKYCProvider(JibitKYCConfig{BaseURL: "http://unused"})
	_, err := p.VerifyShahkar(context.Background(), "09123456789", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid national code")
	}
	if !errors.Is(err, ErrInvalidNationalCode) {
		t.Errorf("expected ErrInvalidNationalCode, got: %v", err)
	}
}

func TestGetIdentityInfo_Success(t *testing.T) {
	srv := tokenAwareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != identityEndpoint {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(IdentityInfoResult{
			FirstName:    "Ali",
			LastName:     "Rezaei",
			FatherName:   "Mohammad",
			Alive:        true,
			NationalCode: "0499370899",
		})
	})
	defer srv.Close()

	p := newTestProvider(srv.URL)
	result, err := p.GetIdentityInfo(context.Background(), "0499370899", "1370/01/01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FirstName != "Ali" {
		t.Errorf("expected Ali, got %q", result.FirstName)
	}
	if result.LastName != "Rezaei" {
		t.Errorf("expected Rezaei, got %q", result.LastName)
	}
	if !result.Alive {
		t.Error("expected alive=true")
	}
}

func TestVerifyCardToNID_Success(t *testing.T) {
	srv := tokenAwareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cardToNIDEndpoint {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CardToNIDResult{Matched: true})
	})
	defer srv.Close()

	p := newTestProvider(srv.URL)
	result, err := p.VerifyCardToNID(context.Background(), "4539578763621486", "0499370899")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Error("expected matched=true")
	}
}

func TestVerifyFace_Success(t *testing.T) {
	srv := tokenAwareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != faceVerifyEndpoint {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		// Verify multipart content type
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %q", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(FaceVerificationResult{
			Matched:        true,
			MatchScore:     0.95,
			LivenessScore:  0.99,
			LivenessResult: "LIVE",
		})
	})
	defer srv.Close()

	p := newTestProvider(srv.URL)
	result, err := p.VerifyFace(context.Background(), "0499370899", []byte("fake-selfie-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Matched {
		t.Error("expected matched=true")
	}
	if result.LivenessResult != "LIVE" {
		t.Errorf("expected LIVE, got %q", result.LivenessResult)
	}
}

func TestVerifyFace_EmptyImage(t *testing.T) {
	p := NewJibitKYCProvider(JibitKYCConfig{BaseURL: "http://unused"})
	_, err := p.VerifyFace(context.Background(), "0499370899", nil)
	if err == nil {
		t.Fatal("expected error for empty image")
	}
	if !errors.Is(err, ErrEmptyImage) {
		t.Errorf("expected ErrEmptyImage, got: %v", err)
	}
}

func TestOCRNationalCard_Success(t *testing.T) {
	srv := tokenAwareServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cardOCREndpoint {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(NationalCardOCRResult{
			NationalCode: "0499370899",
			FirstName:    "Ali",
			LastName:     "Rezaei",
			BirthDate:    "1370/01/01",
			ExpiryDate:   "1405/01/01",
			SerialNumber: "A12345678",
		})
	})
	defer srv.Close()

	p := newTestProvider(srv.URL)
	result, err := p.OCRNationalCard(context.Background(), []byte("fake-card-image"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NationalCode != "0499370899" {
		t.Errorf("expected 0499370899, got %q", result.NationalCode)
	}
	if result.FirstName != "Ali" {
		t.Errorf("expected Ali, got %q", result.FirstName)
	}
}

func TestOCRNationalCard_EmptyImage(t *testing.T) {
	p := NewJibitKYCProvider(JibitKYCConfig{BaseURL: "http://unused"})
	_, err := p.OCRNationalCard(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty image")
	}
	if !errors.Is(err, ErrEmptyImage) {
		t.Errorf("expected ErrEmptyImage, got: %v", err)
	}
}
