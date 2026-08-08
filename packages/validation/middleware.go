package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ===========================================
// Request ID Middleware
// ===========================================

// RequestIDKey is the context key for request ID.
type requestIDKeyType struct{}

var RequestIDKey = requestIDKeyType{}

// RequestIDHeader is the header name for request ID.
const RequestIDHeader = "X-Request-ID"

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// RequestIDMiddleware adds a request ID to each request and response.
// It uses the incoming X-Request-ID header if present, otherwise generates a new UUID.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		} else {
			// Sanitize incoming request ID
			requestID = SanitizeHeaderValue(requestID)
			// Validate it's a valid UUID, otherwise generate new one
			if _, err := uuid.Parse(requestID); err != nil {
				requestID = uuid.New().String()
			}
		}

		// Add to response header
		w.Header().Set(RequestIDHeader, requestID)
		// Propagate the validated/generated value to downstream observability.
		r.Header.Set(RequestIDHeader, requestID)

		// Add to context
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ===========================================
// Rate Limiting Middleware
// ===========================================

// RateLimiter implements a simple in-memory token bucket rate limiter.
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*bucket
	rate        int           // tokens per interval
	interval    time.Duration // refill interval
	burst       int           // max bucket size
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

type bucket struct {
	tokens   int
	lastFill time.Time
	mu       sync.Mutex
}

// NewRateLimiter creates a new rate limiter.
// rate: number of requests allowed per interval
// interval: time period for rate calculation
// burst: maximum number of requests that can be made at once
func NewRateLimiter(rate int, interval time.Duration, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets:     make(map[string]*bucket),
		rate:        rate,
		interval:    interval,
		burst:       burst,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request from the given key should be allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	b, exists := rl.buckets[key]
	if !exists {
		b = &bucket{
			tokens:   rl.burst,
			lastFill: time.Now(),
		}
		rl.buckets[key] = b
	}
	rl.mu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(b.lastFill)
	tokensToAdd := int(elapsed/rl.interval) * rl.rate
	if tokensToAdd > 0 {
		b.tokens += tokensToAdd
		if b.tokens > rl.burst {
			b.tokens = rl.burst
		}
		b.lastFill = now
	}

	// Check if we have tokens available
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// Close stops the cleanup goroutine and releases resources.
func (rl *RateLimiter) Close() {
	close(rl.stopCleanup)
	<-rl.cleanupDone
}

// cleanup periodically removes stale buckets.
func (rl *RateLimiter) cleanup() {
	defer close(rl.cleanupDone)

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCleanup:
			return
		case <-ticker.C:
			rl.cleanupStale()
		}
	}
}

// cleanupStale removes buckets that haven't been used in 10 minutes.
func (rl *RateLimiter) cleanupStale() {
	now := time.Now()
	rl.mu.Lock()
	var staleKeys []string
	for key, b := range rl.buckets {
		b.mu.Lock()
		if now.Sub(b.lastFill) > 10*time.Minute {
			staleKeys = append(staleKeys, key)
		}
		b.mu.Unlock()
	}
	for _, key := range staleKeys {
		delete(rl.buckets, key)
	}
	rl.mu.Unlock()
}

// RateLimitMiddleware creates middleware that rate limits by client IP.
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := ExtractClientIP(r)

			if !limiter.Allow(clientIP) {
				WriteRateLimitExceeded(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ===========================================
// Security Headers Middleware
// ===========================================

// SecurityHeadersMiddleware adds security headers to responses.
// These are defense-in-depth headers in addition to nginx.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict browser features
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// HSTS — only when request is over HTTPS (behind TLS termination or direct)
		if IsSecureRequest(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-site")

		// Cache control and CSP for API responses
		if isAPIRequest(r) {
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		}

		next.ServeHTTP(w, r)
	})
}

// isAPIRequest checks if the request is for an API endpoint.
func isAPIRequest(r *http.Request) bool {
	return len(r.URL.Path) >= 4 && r.URL.Path[:4] == "/api"
}

// ===========================================
// Request Size Limiting
// ===========================================

// MaxBytesReader wraps the request body with a size limit.
// Returns an error handler if the body is too large.
func MaxBytesMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	if maxBytes < 1024 || maxBytes > 64*1024*1024 {
		panic("request body limit must be between 1 KiB and 64 MiB")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength < -1 || r.ContentLength > maxBytes {
				WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body too large")
				return
			}
			if len(r.TransferEncoding) > 1 || (len(r.TransferEncoding) == 1 && !strings.EqualFold(r.TransferEncoding[0], "chunked")) {
				WriteError(w, http.StatusBadRequest, "TRANSFER_ENCODING_INVALID", "Invalid request framing")
				return
			}

			if r.Body != nil {
				bounded := http.MaxBytesReader(w, r.Body, maxBytes+1)
				body, err := io.ReadAll(bounded)
				_ = bounded.Close()
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) || int64(len(body)) > maxBytes {
					WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body too large")
					return
				}
				if err != nil {
					WriteBadRequest(w, "invalid request body")
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				r.ContentLength = int64(len(body))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ContentTypeMiddleware permits only the bounded request encodings used by the
// Platform API. Empty bodies need no Content-Type.
func ContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isStateChangingMethod(r.Method) || r.ContentLength == 0 {
			next.ServeHTTP(w, r)
			return
		}
		mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
		switch mediaType {
		case "application/json", "application/x-www-form-urlencoded", "multipart/form-data", "application/octet-stream":
			next.ServeHTTP(w, r)
		default:
			WriteError(w, http.StatusUnsupportedMediaType, "CONTENT_TYPE_UNSUPPORTED", "Unsupported request content type")
		}
	})
}

// DecodeJSON decodes one bounded JSON value and turns MaxBytesReader overflow
// into the canonical 413 response. Handlers can adopt it incrementally without
// duplicating edge-policy error details.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			WriteError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body too large")
			return err
		}
		WriteBadRequest(w, "invalid request body")
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		WriteBadRequest(w, "request body must contain one JSON value")
		return errors.New("multiple JSON values")
	}
	return nil
}

// ===========================================
// Input Sanitization Middleware
// ===========================================

// SanitizedRequest wraps an http.Request with sanitized values.
type SanitizedRequest struct {
	*http.Request
	sanitizedForm bool
}

// SanitizeFormMiddleware sanitizes form values from POST requests.
func SanitizeFormMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse form if it's a POST/PUT/PATCH request
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
				if err := r.ParseForm(); err == nil {
					// Sanitize form values
					for key, values := range r.Form {
						for i, v := range values {
							r.Form[key][i] = SanitizeString(v)
						}
					}
					for key, values := range r.PostForm {
						for i, v := range values {
							r.PostForm[key][i] = SanitizeString(v)
						}
					}
				}
			}
		}

		// Sanitize query parameters
		query := r.URL.Query()
		for key, values := range query {
			for i, v := range values {
				query[key][i] = SanitizeString(v)
			}
		}
		r.URL.RawQuery = query.Encode()

		next.ServeHTTP(w, r)
	})
}

// ===========================================
// Internal-Only Middleware
// ===========================================

// InternalOnlyMiddleware restricts access to requests from loopback or
// private (RFC 1918 / RFC 4193) IP addresses. This is intended for
// endpoints like /metrics that should only be scraped by internal
// infrastructure (e.g. Prometheus) and not exposed to the public internet.
func InternalOnlyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := net.ParseIP(extractRequestIP(r))
		if ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractRequestIP extracts the client IP from the request using the
// shared trusted-proxy-aware helper.
func extractRequestIP(r *http.Request) string {
	return ExtractClientIP(r)
}

// ValidatePathUUID returns a middleware that validates a named URL parameter
// is a valid UUID. Returns 400 Bad Request if invalid.
func ValidatePathUUID(paramName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := chi.URLParam(r, paramName)
			if id == "" {
				WriteBadRequest(w, paramName+" is required")
				return
			}
			if _, err := uuid.Parse(id); err != nil {
				WriteBadRequest(w, "invalid "+paramName+" format")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
