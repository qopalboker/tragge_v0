package validation

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

var errInvalidCORSConfig = errors.New("invalid CORS configuration")

const (
	edgeContextUser       = "user"
	edgeContextAdmin      = "admin"
	edgeContextTrade      = "trade"
	edgeContextPayment    = "payment"
	environmentProduction = "production"
)

// ValidateCORSConfig rejects ambiguous or unsafe origin policy.
func ValidateCORSConfig(config CORSConfig, production bool) error {
	if config.AllowCredentials {
		for _, origin := range config.AllowedOrigins {
			if origin == "*" {
				return fmt.Errorf("%w: credentialed wildcard origin", errInvalidCORSConfig)
			}
		}
	}
	for _, origin := range config.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.Hostname() == "" || parsed.Path != "" || parsed.RawQuery != "" ||
			parsed.Fragment != "" || parsed.User != nil || parsed.Opaque != "" ||
			strings.Contains(origin, "*") {
			return fmt.Errorf("%w: malformed origin", errInvalidCORSConfig)
		}
	}
	if production && len(config.AllowedOrigins) == 0 {
		return fmt.Errorf("%w: production origins are required", errInvalidCORSConfig)
	}
	return nil
}

// ===========================================
// CORS Middleware
// ===========================================

// CORSConfig holds CORS configuration options.
type CORSConfig struct {
	// AllowedOrigins is a list of origins that are allowed to access the resource.
	// Use "*" to allow all origins (not recommended for production with credentials).
	AllowedOrigins []string

	// AllowedMethods is a list of HTTP methods allowed for cross-origin requests.
	AllowedMethods []string

	// AllowedHeaders is a list of headers that can be used in requests.
	AllowedHeaders []string

	// ExposedHeaders is a list of headers that the browser is allowed to access.
	ExposedHeaders []string

	// AllowCredentials indicates whether the request can include credentials.
	AllowCredentials bool

	// MaxAge indicates how long (in seconds) the results of a preflight request can be cached.
	MaxAge int
}

// DefaultCORSConfig returns a secure default CORS configuration.
// Override for production with specific allowed origins.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{}, // Must be explicitly set
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Accept",
			"Authorization",
			"Content-Type",
			"X-Request-ID",
			"X-Contest-ID",
			"X-Requested-With",
		},
		ExposedHeaders: []string{
			"X-Request-ID",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			"Retry-After",
		},
		AllowCredentials: true,
		MaxAge:           86400, // 24 hours
	}
}

// CORSConfigFromEnv creates a CORS configuration from environment variables.
// Environment variables:
//   - CORS_ALLOWED_ORIGINS: comma-separated list of allowed origins
//   - CORS_ALLOW_CREDENTIALS: "true" or "false"
//   - CORS_MAX_AGE: preflight cache duration in seconds
func CORSConfigFromEnv() CORSConfig {
	config := DefaultCORSConfig()

	// Parse allowed origins from environment
	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		config.AllowedOrigins = parseCommaSeparated(origins)
	} else {
		// Default development origins — only in explicitly non-production environments
		env := os.Getenv("ENVIRONMENT")
		if env == "development" || env == "local" || env == "test" {
			config.AllowedOrigins = []string{
				"http://localhost:5173",
				"http://127.0.0.1:5173",
				"http://localhost:8080",
				"http://127.0.0.1:8080",
			}
			// Auto-detect current Codespace for specific allowed origins.
			// Post-consolidation the frontend is a single Vite server on 5173;
			// 5174/5175 no longer host anything, so they're dropped here.
			if codespaceName := os.Getenv("CODESPACE_NAME"); codespaceName != "" {
				config.AllowedOrigins = append(config.AllowedOrigins,
					fmt.Sprintf("https://%s-5173.app.github.dev", codespaceName),
					fmt.Sprintf("https://%s-8080.app.github.dev", codespaceName),
				)
			}
		}
	}

	// Parse allow credentials
	if creds := os.Getenv("CORS_ALLOW_CREDENTIALS"); creds == "false" {
		config.AllowCredentials = false
	}

	return config
}

func corsConfigForContext(context string) CORSConfig {
	config := CORSConfigFromEnv()
	contextKeys := map[string]string{
		edgeContextUser:    "USER_CORS_ALLOWED_ORIGINS",
		edgeContextAdmin:   "ADMIN_CORS_ALLOWED_ORIGINS",
		edgeContextTrade:   "TRADE_CORS_ALLOWED_ORIGINS",
		edgeContextPayment: "PAYMENT_CORS_ALLOWED_ORIGINS",
	}
	key := contextKeys[strings.ToLower(strings.TrimSpace(context))]
	if origins := strings.TrimSpace(os.Getenv(key)); origins != "" {
		config.AllowedOrigins = parseCommaSeparated(origins)
	} else {
		legacy := "USER_FRONTEND_ORIGIN"
		if context == edgeContextAdmin {
			legacy = "ADMIN_FRONTEND_ORIGIN"
		}
		if origin := strings.TrimSpace(os.Getenv(legacy)); origin != "" {
			config.AllowedOrigins = []string{origin}
		}
	}
	return config
}

// parseCommaSeparated splits a comma-separated string into a slice.
func parseCommaSeparated(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// CORSMiddleware creates CORS middleware with the given configuration.
func CORSMiddleware(config CORSConfig) func(http.Handler) http.Handler {
	production := func() bool {
		env := strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT")))
		return env == "" || env == environmentProduction || env == "staging"
	}()
	if err := ValidateCORSConfig(config, production); err != nil {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				WriteError(w, http.StatusServiceUnavailable, "EDGE_CONFIG_INVALID", "Request unavailable")
			})
		}
	}
	// Build sets for O(1) lookup
	allowedOriginSet := make(map[string]bool)
	for _, origin := range config.AllowedOrigins {
		allowedOriginSet[origin] = true
	}

	// Pre-compute header values
	allowMethods := strings.Join(config.AllowedMethods, ", ")
	allowHeaders := strings.Join(config.AllowedHeaders, ", ")
	exposeHeaders := strings.Join(config.ExposedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			appendVary(w.Header(), "Origin")
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			isAllowed := false
			isWildcard := false
			if origin != "" {
				for _, allowed := range config.AllowedOrigins {
					if allowed == "*" {
						isAllowed = true
						isWildcard = true
						break
					}
					if allowed == origin {
						isAllowed = true
						break
					}
					if !production && MatchWildcardOrigin(origin, allowed) {
						isAllowed = true
						break
					}
				}
			}

			// If origin is allowed, set CORS headers
			if isAllowed && origin != "" {
				// Reflect the specific origin (don't use * with credentials)
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")

				if config.AllowCredentials && !isWildcard {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}

				if exposeHeaders != "" {
					w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
				}
			}

			if origin != "" && !isAllowed {
				WriteError(w, http.StatusForbidden, "CORS_ORIGIN_DENIED", "Cross-origin request denied")
				return
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				appendVary(w.Header(), "Access-Control-Request-Method")
				appendVary(w.Header(), "Access-Control-Request-Headers")
				requestedMethod := r.Header.Get("Access-Control-Request-Method")
				requestedHeaders := r.Header.Values("Access-Control-Request-Headers")
				if !isAllowed || !containsFold(config.AllowedMethods, requestedMethod) ||
					!headersAllowed(config.AllowedHeaders, requestedHeaders) {
					WriteError(w, http.StatusForbidden, "CORS_PREFLIGHT_DENIED", "Cross-origin request denied")
					return
				}
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)

				if config.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
				}

				// Return 204 No Content for preflight
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func appendVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, item := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(item), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, strings.TrimSpace(wanted)) {
			return true
		}
	}
	return false
}

func headersAllowed(allowed []string, requested []string) bool {
	for _, line := range requested {
		for _, name := range strings.Split(line, ",") {
			if strings.TrimSpace(name) != "" && !containsFold(allowed, name) {
				return false
			}
		}
	}
	return true
}

// ===========================================
// Production CORS Configurations
// ===========================================

// UserBFFCORSConfig returns CORS configuration for user-bff.
func UserBFFCORSConfig() CORSConfig {
	return corsConfigForContext(edgeContextUser)
}

// TradeBFFCORSConfig returns CORS configuration for trade-bff.
func TradeBFFCORSConfig() CORSConfig {
	config := corsConfigForContext(edgeContextTrade)
	// trade-bff needs WebSocket upgrade support
	config.AllowedHeaders = append(config.AllowedHeaders,
		"Sec-WebSocket-Key",
		"Sec-WebSocket-Version",
		"Sec-WebSocket-Protocol",
		"Sec-WebSocket-Extensions",
		"Upgrade",
		"Connection",
	)
	return config
}

// AdminBFFCORSConfig returns CORS configuration for admin-bff.
func AdminBFFCORSConfig() CORSConfig {
	config := corsConfigForContext(edgeContextAdmin)
	// admin-bff can be more restrictive
	// Only allow admin frontend origin in production (empty ENVIRONMENT = production)
	if env := os.Getenv("ENVIRONMENT"); env != "development" && env != "local" && env != "test" {
		if adminOrigin := os.Getenv("ADMIN_FRONTEND_ORIGIN"); adminOrigin != "" {
			config.AllowedOrigins = []string{adminOrigin}
		}
	}
	return config
}

// PaymentServiceCORSConfig is intentionally separate from browser BFF policy.
func PaymentServiceCORSConfig() CORSConfig {
	return corsConfigForContext(edgeContextPayment)
}
