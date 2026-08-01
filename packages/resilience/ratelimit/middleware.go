package ratelimit

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
)

// Middleware provides HTTP middleware for rate limiting.
type Middleware struct {
	limiter       RateLimiter
	keyExtractor  KeyExtractor
	onLimitHit    LimitHitHandler
	skipPaths     map[string]bool
	skipPrefixes  []string
	limitType     LimitType
}

// KeyExtractor extracts the rate limit key from a request.
type KeyExtractor func(r *http.Request) string

// LimitHitHandler is called when a rate limit is exceeded.
type LimitHitHandler func(w http.ResponseWriter, r *http.Request, info LimitInfo)

// MiddlewareOption configures the rate limiting middleware.
type MiddlewareOption func(*Middleware)

// WithKeyExtractor sets a custom key extractor.
func WithKeyExtractor(extractor KeyExtractor) MiddlewareOption {
	return func(m *Middleware) {
		m.keyExtractor = extractor
	}
}

// WithLimitHitHandler sets a custom handler for rate limit exceeded.
func WithLimitHitHandler(handler LimitHitHandler) MiddlewareOption {
	return func(m *Middleware) {
		m.onLimitHit = handler
	}
}

// WithSkipPaths sets paths to skip rate limiting.
func WithSkipPaths(paths ...string) MiddlewareOption {
	return func(m *Middleware) {
		for _, path := range paths {
			m.skipPaths[path] = true
		}
	}
}

// WithSkipPrefixes sets path prefixes to skip rate limiting.
func WithSkipPrefixes(prefixes ...string) MiddlewareOption {
	return func(m *Middleware) {
		m.skipPrefixes = append(m.skipPrefixes, prefixes...)
	}
}

// WithLimitType sets the type of limit for metrics.
func WithLimitType(limitType LimitType) MiddlewareOption {
	return func(m *Middleware) {
		m.limitType = limitType
	}
}

// NewMiddleware creates a new rate limiting middleware.
func NewMiddleware(limiter RateLimiter, opts ...MiddlewareOption) *Middleware {
	m := &Middleware{
		limiter:      limiter,
		keyExtractor: UserIDExtractor,
		onLimitHit:   DefaultLimitHitHandler,
		skipPaths:    make(map[string]bool),
		limitType:    LimitTypeAPI,
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Handler returns an http.Handler middleware that applies rate limiting.
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check skip conditions
		if m.shouldSkip(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Extract rate limit key
		key := m.keyExtractor(r)
		if key == "" {
			// No key extracted, skip rate limiting
			next.ServeHTTP(w, r)
			return
		}

		// Check rate limit
		if !m.limiter.Allow(key) {
			info := LimitInfo{
				Allowed:    false,
				Remaining:  m.limiter.Remaining(key),
				RetryAfter: m.limiter.RetryAfter(key),
			}
			m.onLimitHit(w, r, info)
			return
		}

		// Set rate limit headers
		remaining := m.limiter.Remaining(key)
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

		next.ServeHTTP(w, r)
	})
}

// HandlerFunc returns an http.HandlerFunc version of the middleware.
func (m *Middleware) HandlerFunc(next http.HandlerFunc) http.HandlerFunc {
	return m.Handler(next).ServeHTTP
}

// shouldSkip checks if the request should skip rate limiting.
func (m *Middleware) shouldSkip(r *http.Request) bool {
	path := r.URL.Path

	// Check exact path matches
	if m.skipPaths[path] {
		return true
	}

	// Check prefix matches
	for _, prefix := range m.skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

// UserIDExtractor extracts the user ID from the request context.
// This requires the auth middleware to run first.
func UserIDExtractor(r *http.Request) string {
	return auth.GetUserID(r.Context())
}

// IPExtractor extracts the client IP address using the shared trusted-proxy-aware helper.
func IPExtractor(r *http.Request) string {
	return validation.ExtractClientIP(r)
}

// CompositeExtractor combines user ID and IP for more granular limits.
func CompositeExtractor(r *http.Request) string {
	userID := UserIDExtractor(r)
	if userID != "" {
		return "user:" + userID
	}
	return "ip:" + IPExtractor(r)
}

// EndpointExtractor includes the endpoint in the key for per-endpoint limits.
func EndpointExtractor(r *http.Request) string {
	userID := UserIDExtractor(r)
	if userID == "" {
		return ""
	}
	return userID + ":" + r.Method + ":" + r.URL.Path
}

// DefaultLimitHitHandler returns a 429 response with Retry-After header.
func DefaultLimitHitHandler(w http.ResponseWriter, r *http.Request, info LimitInfo) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Remaining", "0")

	if info.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(info.RetryAfter.Seconds())+1))
	}

	w.WriteHeader(http.StatusTooManyRequests)

	response := map[string]interface{}{
		"error":       "rate limit exceeded",
		"retry_after": info.RetryAfter.Seconds(),
	}

	json.NewEncoder(w).Encode(response)
}

// LimitResponse represents the JSON response for rate limit errors.
type LimitResponse struct {
	Error      string  `json:"error"`
	RetryAfter float64 `json:"retry_after"`
	Limit      int     `json:"limit,omitempty"`
	Remaining  int     `json:"remaining"`
}

// DetailedLimitHitHandler returns a more detailed 429 response.
func DetailedLimitHitHandler(w http.ResponseWriter, r *http.Request, info LimitInfo) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(info.Limit))
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(info.ResetAt.Unix(), 10))

	if info.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(info.RetryAfter.Seconds())+1))
	}

	w.WriteHeader(http.StatusTooManyRequests)

	response := LimitResponse{
		Error:      "rate limit exceeded",
		RetryAfter: info.RetryAfter.Seconds(),
		Limit:      info.Limit,
		Remaining:  0,
	}

	json.NewEncoder(w).Encode(response)
}

// ContextKeyExtractor creates an extractor that gets a key from context.
func ContextKeyExtractor(ctxKey interface{}) KeyExtractor {
	return func(r *http.Request) string {
		if val, ok := r.Context().Value(ctxKey).(string); ok {
			return val
		}
		return ""
	}
}

// HeaderExtractor creates an extractor that gets a key from a header.
func HeaderExtractor(headerName string) KeyExtractor {
	return func(r *http.Request) string {
		return r.Header.Get(headerName)
	}
}

// RateLimitMiddleware is a convenience function to create rate limiting middleware.
func RateLimitMiddleware(limiter RateLimiter, opts ...MiddlewareOption) func(http.Handler) http.Handler {
	m := NewMiddleware(limiter, opts...)
	return m.Handler
}

// PerEndpointMiddleware creates middleware with different limits per endpoint.
type PerEndpointMiddleware struct {
	defaultLimiter RateLimiter
	endpointLimits map[string]RateLimiter
	keyExtractor   KeyExtractor
	onLimitHit     LimitHitHandler
}

// PerEndpointConfig configures per-endpoint rate limits.
type PerEndpointConfig struct {
	DefaultLimiter RateLimiter
	EndpointLimits map[string]RateLimiter // pattern -> limiter
	KeyExtractor   KeyExtractor
	OnLimitHit     LimitHitHandler
}

// NewPerEndpointMiddleware creates middleware with per-endpoint limits.
func NewPerEndpointMiddleware(cfg PerEndpointConfig) *PerEndpointMiddleware {
	m := &PerEndpointMiddleware{
		defaultLimiter: cfg.DefaultLimiter,
		endpointLimits: cfg.EndpointLimits,
		keyExtractor:   cfg.KeyExtractor,
		onLimitHit:     cfg.OnLimitHit,
	}

	if m.keyExtractor == nil {
		m.keyExtractor = UserIDExtractor
	}
	if m.onLimitHit == nil {
		m.onLimitHit = DefaultLimitHitHandler
	}
	if m.endpointLimits == nil {
		m.endpointLimits = make(map[string]RateLimiter)
	}

	return m
}

// Handler returns the middleware handler.
func (m *PerEndpointMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := m.keyExtractor(r)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Find matching limiter
		limiter := m.defaultLimiter
		pattern := r.Method + " " + r.URL.Path

		// Check for exact match first
		if l, ok := m.endpointLimits[pattern]; ok {
			limiter = l
		} else {
			// Check for prefix matches
			for p, l := range m.endpointLimits {
				if strings.HasSuffix(p, "*") {
					prefix := strings.TrimSuffix(p, "*")
					if strings.HasPrefix(pattern, prefix) {
						limiter = l
						break
					}
				}
			}
		}

		if limiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		if !limiter.Allow(key) {
			info := LimitInfo{
				Allowed:    false,
				Remaining:  limiter.Remaining(key),
				RetryAfter: limiter.RetryAfter(key),
			}
			m.onLimitHit(w, r, info)
			return
		}

		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(limiter.Remaining(key)))
		next.ServeHTTP(w, r)
	})
}

// AddEndpointLimit adds or updates a limit for a specific endpoint.
func (m *PerEndpointMiddleware) AddEndpointLimit(pattern string, limiter RateLimiter) {
	m.endpointLimits[pattern] = limiter
}

// contextKey type for rate limit context values.
type contextKey string

const (
	// RateLimitInfoKey is the context key for rate limit info.
	RateLimitInfoKey contextKey = "ratelimit_info"
)

// WithRateLimitInfo adds rate limit info to the context.
func WithRateLimitInfo(ctx context.Context, info LimitInfo) context.Context {
	return context.WithValue(ctx, RateLimitInfoKey, info)
}

// GetRateLimitInfo retrieves rate limit info from the context.
func GetRateLimitInfo(ctx context.Context) (LimitInfo, bool) {
	info, ok := ctx.Value(RateLimitInfoKey).(LimitInfo)
	return info, ok
}

// CheckMiddleware adds rate limit info to context without blocking.
// Useful for logging or displaying remaining limits.
func CheckMiddleware(limiter RateLimiter, keyExtractor KeyExtractor) func(http.Handler) http.Handler {
	if keyExtractor == nil {
		keyExtractor = UserIDExtractor
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyExtractor(r)
			if key != "" {
				info := LimitInfo{
					Remaining: limiter.Remaining(key),
				}
				ctx := WithRateLimitInfo(r.Context(), info)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ConditionalMiddleware applies rate limiting based on a condition.
func ConditionalMiddleware(
	condition func(r *http.Request) bool,
	limiter RateLimiter,
	opts ...MiddlewareOption,
) func(http.Handler) http.Handler {
	m := NewMiddleware(limiter, opts...)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if condition(r) {
				m.Handler(next).ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

// TimeBasedMiddleware applies different limits based on time of day.
type TimeBasedMiddleware struct {
	peakLimiter    RateLimiter
	offPeakLimiter RateLimiter
	peakHours      [2]int // start, end hours (24h format)
	keyExtractor   KeyExtractor
	onLimitHit     LimitHitHandler
	location       *time.Location
}

// TimeBasedConfig configures time-based rate limiting.
type TimeBasedConfig struct {
	PeakLimiter    RateLimiter
	OffPeakLimiter RateLimiter
	PeakStartHour  int // 0-23
	PeakEndHour    int // 0-23
	KeyExtractor   KeyExtractor
	OnLimitHit     LimitHitHandler
	Location       *time.Location
}

// NewTimeBasedMiddleware creates time-aware rate limiting middleware.
func NewTimeBasedMiddleware(cfg TimeBasedConfig) *TimeBasedMiddleware {
	m := &TimeBasedMiddleware{
		peakLimiter:    cfg.PeakLimiter,
		offPeakLimiter: cfg.OffPeakLimiter,
		peakHours:      [2]int{cfg.PeakStartHour, cfg.PeakEndHour},
		keyExtractor:   cfg.KeyExtractor,
		onLimitHit:     cfg.OnLimitHit,
		location:       cfg.Location,
	}

	if m.keyExtractor == nil {
		m.keyExtractor = UserIDExtractor
	}
	if m.onLimitHit == nil {
		m.onLimitHit = DefaultLimitHitHandler
	}
	if m.location == nil {
		m.location = time.UTC
	}

	return m
}

// Handler returns the middleware handler.
func (m *TimeBasedMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := m.keyExtractor(r)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Select limiter based on time
		limiter := m.selectLimiter()
		if limiter == nil {
			next.ServeHTTP(w, r)
			return
		}

		if !limiter.Allow(key) {
			info := LimitInfo{
				Allowed:    false,
				Remaining:  limiter.Remaining(key),
				RetryAfter: limiter.RetryAfter(key),
			}
			m.onLimitHit(w, r, info)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (m *TimeBasedMiddleware) selectLimiter() RateLimiter {
	now := time.Now().In(m.location)
	hour := now.Hour()

	start, end := m.peakHours[0], m.peakHours[1]

	var isPeak bool
	if start <= end {
		isPeak = hour >= start && hour < end
	} else {
		// Wraps around midnight
		isPeak = hour >= start || hour < end
	}

	if isPeak {
		return m.peakLimiter
	}
	return m.offPeakLimiter
}
