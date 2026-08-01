package validation

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestIDMiddlewarePropagatesValidatedHeader(t *testing.T) {
	const requestID = "95de37b7-a527-45d4-b6ac-e424df1577cb"
	var downstreamHeader string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstreamHeader = r.Header.Get(RequestIDHeader)
		if GetRequestID(r.Context()) != requestID {
			t.Fatalf("context request ID mismatch: %q", GetRequestID(r.Context()))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, requestID)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if downstreamHeader != requestID || recorder.Header().Get(RequestIDHeader) != requestID {
		t.Fatalf("request ID not propagated: downstream=%q response=%q", downstreamHeader, recorder.Header().Get(RequestIDHeader))
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(2, time.Second, 2)
	defer rl.Close()

	// Should allow first 2 requests (burst)
	if !rl.Allow("key1") {
		t.Error("expected first request to be allowed")
	}
	if !rl.Allow("key1") {
		t.Error("expected second request to be allowed")
	}
	// Third request should be denied (burst exhausted)
	if rl.Allow("key1") {
		t.Error("expected third request to be denied")
	}
}

func TestRateLimiterClose(t *testing.T) {
	rl := NewRateLimiter(10, time.Second, 10)

	// Close should not hang
	done := make(chan struct{})
	go func() {
		rl.Close()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return within 2 seconds - goroutine likely leaked")
	}
}

func TestRateLimiterSeparateKeys(t *testing.T) {
	rl := NewRateLimiter(1, time.Second, 1)
	defer rl.Close()

	if !rl.Allow("key1") {
		t.Error("expected key1 first request allowed")
	}
	if !rl.Allow("key2") {
		t.Error("expected key2 first request allowed (separate bucket)")
	}
	// key1 should now be exhausted
	if rl.Allow("key1") {
		t.Error("expected key1 second request denied")
	}
}
