package validation

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSWildcardCredentials(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}

	handler := CORSMiddleware(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unsafe CORS configuration status=%d want=%d", rec.Code, http.StatusServiceUnavailable)
	}
	// Credentials header should NOT be set for wildcard match
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("expected no credentials header for wildcard, got %q", got)
	}
}

func TestCORSSpecificOriginCredentials(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}

	handler := CORSMiddleware(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("expected specific origin, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected credentials=true for specific origin, got %q", got)
	}
}

func TestCORSUnmatchedOrigin(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{"GET"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}

	handler := CORSMiddleware(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("unmatched origin status=%d want=%d", rec.Code, http.StatusForbidden)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no CORS headers for unmatched origin, got %q", got)
	}
}

func TestCORSPreflightRequest(t *testing.T) {
	config := CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           3600,
	}

	handler := CORSMiddleware(config)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods header")
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("expected max-age 3600, got %q", got)
	}
}
