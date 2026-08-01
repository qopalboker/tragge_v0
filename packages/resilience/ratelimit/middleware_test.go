package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

// mockLimiter is a test implementation of RateLimiter.
type mockLimiter struct {
	allowed     bool
	remaining   int
	retryAfter  time.Duration
	allowCalls  int
	allowNCalls int
	resetCalls  int
}

func (m *mockLimiter) Allow(key string) bool {
	m.allowCalls++
	return m.allowed
}

func (m *mockLimiter) AllowN(key string, n int) bool {
	m.allowNCalls++
	return m.allowed
}

func (m *mockLimiter) Reset(key string) {
	m.resetCalls++
}

func (m *mockLimiter) Remaining(key string) int {
	return m.remaining
}

func (m *mockLimiter) RetryAfter(key string) time.Duration {
	return m.retryAfter
}

func TestMiddleware_AllowedRequest(t *testing.T) {
	limiter := &mockLimiter{allowed: true, remaining: 5}
	middleware := NewMiddleware(limiter,
		WithKeyExtractor(func(r *http.Request) string { return "test-user" }),
	)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if limiter.allowCalls != 1 {
		t.Errorf("Expected 1 Allow call, got %d", limiter.allowCalls)
	}
}

func TestMiddleware_DeniedRequest(t *testing.T) {
	limiter := &mockLimiter{allowed: false, remaining: 0, retryAfter: 30 * time.Second}
	middleware := NewMiddleware(limiter,
		WithKeyExtractor(func(r *http.Request) string { return "test-user" }),
	)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", rec.Code)
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("Expected Retry-After header")
	}
}

func TestMiddleware_SkipPaths(t *testing.T) {
	limiter := &mockLimiter{allowed: true}
	middleware := NewMiddleware(limiter,
		WithKeyExtractor(func(r *http.Request) string { return "test-user" }),
		WithSkipPaths("/healthz", "/readyz"),
	)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request to skipped path
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if limiter.allowCalls != 0 {
		t.Errorf("Expected 0 Allow calls for skipped path, got %d", limiter.allowCalls)
	}
}

func TestMiddleware_SkipPrefixes(t *testing.T) {
	limiter := &mockLimiter{allowed: true}
	middleware := NewMiddleware(limiter,
		WithKeyExtractor(func(r *http.Request) string { return "test-user" }),
		WithSkipPrefixes("/internal/", "/metrics"),
	)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request to skipped prefix
	req := httptest.NewRequest("GET", "/internal/debug", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if limiter.allowCalls != 0 {
		t.Errorf("Expected 0 Allow calls for skipped prefix, got %d", limiter.allowCalls)
	}
}

func TestMiddleware_NoKey(t *testing.T) {
	limiter := &mockLimiter{allowed: true}
	middleware := NewMiddleware(limiter,
		WithKeyExtractor(func(r *http.Request) string { return "" }), // No key extracted
	)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should pass through without rate limiting
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	if limiter.allowCalls != 0 {
		t.Errorf("Expected 0 Allow calls when no key, got %d", limiter.allowCalls)
	}
}

func TestMiddleware_CustomLimitHandler(t *testing.T) {
	limiter := &mockLimiter{allowed: false}
	customHandlerCalled := false

	middleware := NewMiddleware(limiter,
		WithKeyExtractor(func(r *http.Request) string { return "test-user" }),
		WithLimitHitHandler(func(w http.ResponseWriter, r *http.Request, info LimitInfo) {
			customHandlerCalled = true
			w.WriteHeader(http.StatusServiceUnavailable)
		}),
	)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !customHandlerCalled {
		t.Error("Custom handler should have been called")
	}

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", rec.Code)
	}
}

func TestUserIDExtractor(t *testing.T) {
	// Create request with user ID in context
	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), auth.UserIDKey, "user-123")
	req = req.WithContext(ctx)

	userID := UserIDExtractor(req)

	if userID != "user-123" {
		t.Errorf("Expected user-123, got %s", userID)
	}
}

func TestUserIDExtractor_NoUser(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)

	userID := UserIDExtractor(req)

	if userID != "" {
		t.Errorf("Expected empty string, got %s", userID)
	}
}

func TestIPExtractor(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		trusted    string
		expected   string
	}{
		{
			name:       "X-Forwarded-For",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"},
			remoteAddr: "10.0.0.1:1234",
			trusted:    "10.0.0.0/8,5.6.7.8/32",
			expected:   "1.2.3.4",
		},
		{
			name:       "X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "1.2.3.4"},
			remoteAddr: "10.0.0.1:1234",
			trusted:    "10.0.0.0/8",
			expected:   "1.2.3.4",
		},
		{
			name:       "RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "1.2.3.4:5678",
			expected:   "1.2.3.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXY_CIDRS", tt.trusted)
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			ip := IPExtractor(req)
			if ip != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, ip)
			}
		})
	}
}

func TestCompositeExtractor(t *testing.T) {
	// With user ID
	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), auth.UserIDKey, "user-123")
	req = req.WithContext(ctx)

	key := CompositeExtractor(req)
	if key != "user:user-123" {
		t.Errorf("Expected user:user-123, got %s", key)
	}

	// Without user ID (falls back to IP)
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:5678"

	key = CompositeExtractor(req)
	if key != "ip:1.2.3.4" {
		t.Errorf("Expected ip:1.2.3.4, got %s", key)
	}
}

func TestPerEndpointMiddleware(t *testing.T) {
	defaultLimiter := &mockLimiter{allowed: true}
	orderLimiter := &mockLimiter{allowed: false, retryAfter: 10 * time.Second}

	middleware := NewPerEndpointMiddleware(PerEndpointConfig{
		DefaultLimiter: defaultLimiter,
		EndpointLimits: map[string]RateLimiter{
			"POST /api/orders": orderLimiter,
		},
		KeyExtractor: func(r *http.Request) string { return "test-user" },
	})

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test order endpoint (uses order limiter)
	req := httptest.NewRequest("POST", "/api/orders", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429 for order endpoint, got %d", rec.Code)
	}

	// Test other endpoint (uses default limiter)
	req = httptest.NewRequest("GET", "/api/users", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200 for default endpoint, got %d", rec.Code)
	}
}

func TestRateLimitHeaders(t *testing.T) {
	limiter := &mockLimiter{allowed: true, remaining: 42}
	middleware := NewMiddleware(limiter,
		WithKeyExtractor(func(r *http.Request) string { return "test-user" }),
	)

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	remaining := rec.Header().Get("X-RateLimit-Remaining")
	if remaining != "42" {
		t.Errorf("Expected X-RateLimit-Remaining 42, got %s", remaining)
	}
}

func TestContextKeyExtractor(t *testing.T) {
	type ctxKey string
	const apiKey ctxKey = "api_key"

	extractor := ContextKeyExtractor(apiKey)

	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), apiKey, "key-12345")
	req = req.WithContext(ctx)

	key := extractor(req)
	if key != "key-12345" {
		t.Errorf("Expected key-12345, got %s", key)
	}
}

func TestHeaderExtractor(t *testing.T) {
	extractor := HeaderExtractor("X-API-Key")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "secret-key")

	key := extractor(req)
	if key != "secret-key" {
		t.Errorf("Expected secret-key, got %s", key)
	}
}

func TestWithRateLimitInfo(t *testing.T) {
	info := LimitInfo{
		Allowed:   true,
		Remaining: 10,
		Limit:     100,
	}

	ctx := WithRateLimitInfo(context.Background(), info)
	retrieved, ok := GetRateLimitInfo(ctx)

	if !ok {
		t.Error("Expected to retrieve rate limit info")
	}

	if retrieved.Remaining != 10 {
		t.Errorf("Expected Remaining 10, got %d", retrieved.Remaining)
	}
}

func TestCheckMiddleware(t *testing.T) {
	limiter := NewUserLimiter(Config{
		Rate:      100,
		Window:    time.Second,
		BurstSize: 50,
	})
	defer limiter.Close()

	middleware := CheckMiddleware(limiter, func(r *http.Request) string {
		return "test-user"
	})

	var capturedInfo LimitInfo
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, ok := GetRateLimitInfo(r.Context())
		if ok {
			capturedInfo = info
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedInfo.Remaining == 0 {
		t.Error("Expected non-zero remaining in context")
	}
}

func TestConditionalMiddleware(t *testing.T) {
	limiter := &mockLimiter{allowed: false}

	// Only apply rate limiting to POST requests
	condition := func(r *http.Request) bool {
		return r.Method == "POST"
	}

	middleware := ConditionalMiddleware(condition, limiter,
		WithKeyExtractor(func(r *http.Request) string { return "test-user" }),
	)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET should pass through
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET should pass through, got %d", rec.Code)
	}

	// POST should be rate limited
	req = httptest.NewRequest("POST", "/test", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("POST should be rate limited, got %d", rec.Code)
	}
}

func TestTimeBasedMiddleware(t *testing.T) {
	peakLimiter := &mockLimiter{allowed: false, retryAfter: time.Second}
	offPeakLimiter := &mockLimiter{allowed: true}

	// Configure so current hour is either peak or off-peak based on actual time
	now := time.Now().UTC()
	currentHour := now.Hour()

	middleware := NewTimeBasedMiddleware(TimeBasedConfig{
		PeakLimiter:    peakLimiter,
		OffPeakLimiter: offPeakLimiter,
		PeakStartHour:  currentHour,       // Start at current hour
		PeakEndHour:    (currentHour + 1), // End 1 hour later
		KeyExtractor:   func(r *http.Request) string { return "test-user" },
		Location:       time.UTC,
	})

	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// During peak hours, should be rate limited (peakLimiter.allowed = false)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Expected rate limited during peak, got %d", rec.Code)
	}
}
