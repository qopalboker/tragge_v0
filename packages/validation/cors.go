package validation

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

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
				"http://localhost:5173", // frontend
				"http://localhost:8080", // gateway
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
	// Build sets for O(1) lookup
	allowedOriginSet := make(map[string]bool)
	for _, origin := range config.AllowedOrigins {
		allowedOriginSet[origin] = true
	}

	// Pre-compute header values
	allowMethods := strings.Join(config.AllowedMethods, ", ")
	allowHeaders := strings.Join(config.AllowedHeaders, ", ")
	exposeHeaders := strings.Join(config.ExposedHeaders, ", ")

	// Warn about insecure wildcard + credentials combination
	hasWildcard := false
	for _, origin := range config.AllowedOrigins {
		if origin == "*" {
			hasWildcard = true
			break
		}
	}
	if hasWildcard && config.AllowCredentials {
		log.Println("[SECURITY WARNING] CORS: wildcard origin '*' combined with AllowCredentials=true " +
			"allows any website to make credentialed requests. This is a security risk in production. " +
			"Use explicit origins instead.")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
					// Support wildcard subdomains: *.example.com or https://*.example.com
					if MatchWildcardOrigin(origin, allowed) {
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

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				if isAllowed {
					w.Header().Set("Access-Control-Allow-Methods", allowMethods)
					w.Header().Set("Access-Control-Allow-Headers", allowHeaders)

					if config.MaxAge > 0 {
						w.Header().Set("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
					}
				}

				// Return 204 No Content for preflight
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ===========================================
// Production CORS Configurations
// ===========================================

// UserBFFCORSConfig returns CORS configuration for user-bff.
func UserBFFCORSConfig() CORSConfig {
	config := CORSConfigFromEnv()
	// user-bff specific headers if needed
	return config
}

// TradeBFFCORSConfig returns CORS configuration for trade-bff.
func TradeBFFCORSConfig() CORSConfig {
	config := CORSConfigFromEnv()
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
	config := CORSConfigFromEnv()
	// admin-bff can be more restrictive
	// Only allow admin frontend origin in production (empty ENVIRONMENT = production)
	if env := os.Getenv("ENVIRONMENT"); env != "development" && env != "local" && env != "test" {
		if adminOrigin := os.Getenv("ADMIN_FRONTEND_ORIGIN"); adminOrigin != "" {
			config.AllowedOrigins = []string{adminOrigin}
		}
	}
	return config
}
