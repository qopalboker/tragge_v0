package validation

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

// ===========================================
// CSRF Protection Middleware
// ===========================================

// CSRFConfig holds CSRF protection configuration.
type CSRFConfig struct {
	// Context identifies the User or Admin CSRF trust boundary.
	Context string

	// AllowedOrigins is a list of origins allowed for state-changing requests.
	// Loaded from ALLOWED_ORIGINS env var if not explicitly set.
	AllowedOrigins []string

	// SkipPaths is a list of path prefixes to skip CSRF validation.
	// Typically used for webhook endpoints from external services.
	SkipPaths []string

	// RequireXRequestedWith requires the X-Requested-With header for state-changing requests.
	RequireXRequestedWith bool

	// TrustedProxies indicates if we're behind trusted proxies (use X-Forwarded-Proto).
	TrustedProxies bool

	// CookieNames identifies credentials that make a browser request CSRF
	// relevant. Bearer-only service clients are not forced into browser CSRF.
	CookieNames []string
}

// DefaultCSRFConfig returns a secure default CSRF configuration.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		AllowedOrigins:        []string{},
		SkipPaths:             []string{},
		RequireXRequestedWith: true,
		TrustedProxies:        true,
	}
}

// CSRFConfigFromEnv creates a CSRF configuration from environment variables.
// Environment variables:
//   - ALLOWED_ORIGINS: comma-separated list of allowed origins
//   - CSRF_SKIP_PATHS: comma-separated list of path prefixes to skip (default: /webhooks/)
func CSRFConfigFromEnv() CSRFConfig {
	config := DefaultCSRFConfig()

	// Parse allowed origins from environment
	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		config.AllowedOrigins = parseCommaSeparated(origins)
	} else {
		// Default development origins — only in explicitly non-production environments
		env := os.Getenv("ENVIRONMENT")
		if env == "development" || env == "local" || env == "test" {
			config.AllowedOrigins = []string{
				"http://localhost:5173", // frontend
				"http://localhost:8080", // gateway
			}
			// Auto-detect current Codespace for specific allowed origins
			if codespaceName := os.Getenv("CODESPACE_NAME"); codespaceName != "" {
				config.AllowedOrigins = append(config.AllowedOrigins,
					fmt.Sprintf("https://%s-5173.app.github.dev", codespaceName),
					fmt.Sprintf("https://%s-5174.app.github.dev", codespaceName),
					fmt.Sprintf("https://%s-5175.app.github.dev", codespaceName),
					fmt.Sprintf("https://%s-8080.app.github.dev", codespaceName),
				)
			}
		}
	}

	// Parse skip paths from environment
	if skipPaths := os.Getenv("CSRF_SKIP_PATHS"); skipPaths != "" {
		config.SkipPaths = parseCommaSeparated(skipPaths)
	}

	return config
}

// CSRFMiddleware creates CSRF protection middleware.
// It validates:
// 1. Origin/Referer header matches allowed origins for state-changing requests
// 2. X-Requested-With: XMLHttpRequest header is present (if configured)
//
// State-changing methods: POST, PUT, DELETE, PATCH
func CSRFMiddleware(config CSRFConfig) func(http.Handler) http.Handler {
	// Build set for O(1) lookup
	allowedOriginSet := make(map[string]bool)
	for _, origin := range config.AllowedOrigins {
		allowedOriginSet[origin] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only validate state-changing methods
			if !isStateChangingMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			// Check if path should be skipped (e.g., webhooks)
			for _, skipPath := range config.SkipPaths {
				if strings.HasPrefix(r.URL.Path, skipPath) {
					next.ServeHTTP(w, r)
					return
				}
			}

			browserCredential := false
			for _, name := range config.CookieNames {
				if cookie, err := r.Cookie(name); err == nil && cookie.Value != "" {
					browserCredential = true
					break
				}
			}
			if !browserCredential && strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("Authorization"))), "bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			// Validate X-Requested-With header for browser credential contexts.
			if config.RequireXRequestedWith {
				xrw := r.Header.Get("X-Requested-With")
				if xrw != "XMLHttpRequest" {
					WriteError(w, http.StatusForbidden, "CSRF_HEADER_MISSING",
						"Missing or invalid X-Requested-With header")
					return
				}
			}

			// Get the origin from Origin header, fallback to Referer
			origin := r.Header.Get("Origin")
			if origin == "" {
				referer := r.Header.Get("Referer")
				if referer != "" {
					if parsed, err := url.Parse(referer); err == nil {
						origin = parsed.Scheme + "://" + parsed.Host
					}
				}
			}

			// If no origin/referer and we have allowed origins configured,
			// this might be a same-origin request from older browsers.
			// For defense-in-depth with JWT, we still require X-Requested-With.
			if origin == "" {
				WriteError(w, http.StatusForbidden, "CSRF_ORIGIN_MISSING",
					"Origin header required for state-changing requests")
				return
			}

			// Validate origin against allowed list
			if !isOriginAllowed(origin, config.AllowedOrigins, allowedOriginSet) {
				WriteError(w, http.StatusForbidden, "CSRF_ORIGIN_INVALID",
					"Origin not allowed")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isStateChangingMethod returns true if the HTTP method can change state.
func isStateChangingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	default:
		return false
	}
}

// isOriginAllowed checks if the origin is in the allowed list.
// Supports exact matches and wildcard subdomains in two formats:
//   - "*.example.com" matches "https://foo.example.com"
//   - "https://*.example.com" matches "https://foo.example.com" (scheme-aware)
func isOriginAllowed(origin string, allowedOrigins []string, allowedSet map[string]bool) bool {
	// Exact match
	if allowedSet[origin] {
		return true
	}

	// Check wildcard patterns
	for _, allowed := range allowedOrigins {
		if MatchWildcardOrigin(origin, allowed) {
			return true
		}
	}

	return false
}

// MatchWildcardOrigin checks if origin matches a wildcard pattern.
// Supports "*.example.com" and "https://*.example.com" formats.
func MatchWildcardOrigin(origin, pattern string) bool {
	// Handle scheme-prefixed wildcards: "https://*.example.com"
	if idx := strings.Index(pattern, "://*."); idx != -1 {
		scheme := pattern[:idx+3] // "https://"
		domain := pattern[idx+4:] // ".example.com"
		if !strings.HasPrefix(origin, scheme) {
			return false
		}
		host := origin[len(scheme):] // "foo.example.com"
		if !strings.HasSuffix(host, domain) || len(host) <= len(domain) {
			return false
		}
		subdomain := host[:len(host)-len(domain)]
		// Only allow single-level subdomains (no dots = no nested)
		return !strings.Contains(subdomain, ".")
	}

	// Handle bare wildcards: "*.example.com"
	if strings.HasPrefix(pattern, "*.") {
		domain := pattern[1:] // ".example.com"
		if strings.HasSuffix(origin, domain) {
			prefix := origin[:len(origin)-len(domain)]
			if strings.HasSuffix(prefix, "://") {
				return true
			}
			// For "https://sub.example.com" matching "*.example.com":
			// prefix would be "https://sub" — check only single-level
			if idx := strings.LastIndex(prefix, "://"); idx != -1 {
				sub := prefix[idx+3:]
				return len(sub) > 0 && !strings.Contains(sub, ".")
			}
		}
	}

	return false
}

// ===========================================
// BFF-specific CSRF Configurations
// ===========================================

// UserBFFCSRFConfig returns the User-origin CSRF trust context.
func UserBFFCSRFConfig() CSRFConfig {
	config := CSRFConfigFromEnv()
	config.Context = auth.UserCSRFContext
	config.SkipPaths = nil
	config.CookieNames = []string{auth.UserRefreshCookieName}
	config.AllowedOrigins = UserBFFCORSConfig().AllowedOrigins
	return config
}

// TradeBFFCSRFConfig returns CSRF configuration for trade-bff.
func TradeBFFCSRFConfig() CSRFConfig {
	config := CSRFConfigFromEnv()
	config.Context = auth.UserCSRFContext
	config.SkipPaths = nil
	config.CookieNames = []string{auth.UserRefreshCookieName}
	config.AllowedOrigins = TradeBFFCORSConfig().AllowedOrigins
	return config
}

// AdminBFFCSRFConfig returns the Admin-origin CSRF trust context.
func AdminBFFCSRFConfig() CSRFConfig {
	config := CSRFConfigFromEnv()
	config.Context = auth.AdminCSRFContext
	config.SkipPaths = nil
	config.CookieNames = []string{auth.AdminRefreshCookieName}
	config.AllowedOrigins = AdminBFFCORSConfig().AllowedOrigins
	return config
}

// ===========================================
// Secure Cookie Helpers
// ===========================================

// SecureCookieConfig holds secure cookie configuration.
type SecureCookieConfig struct {
	Name     string
	Value    string
	Path     string
	MaxAge   int
	Secure   bool
	HttpOnly bool
	SameSite http.SameSite
}

// DefaultSecureCookieConfig returns secure defaults for cookies.
func DefaultSecureCookieConfig() SecureCookieConfig {
	// Determine if we're in production (HTTPS) — empty ENVIRONMENT defaults to production
	env := os.Getenv("ENVIRONMENT")
	secure := env != "development" && env != "local" && env != "test"

	return SecureCookieConfig{
		Path:     "/",
		MaxAge:   0, // Session cookie
		Secure:   secure,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

// SetSecureCookie sets a cookie with secure defaults.
func SetSecureCookie(w http.ResponseWriter, config SecureCookieConfig) {
	cookie := &http.Cookie{
		Name:     config.Name,
		Value:    config.Value,
		Path:     config.Path,
		MaxAge:   config.MaxAge,
		Secure:   config.Secure,
		HttpOnly: config.HttpOnly,
		SameSite: config.SameSite,
	}
	http.SetCookie(w, cookie)
}

// DeleteSecureCookie removes a cookie by setting MaxAge to -1.
func DeleteSecureCookie(w http.ResponseWriter, name, path string) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		Secure:   func() bool { e := os.Getenv("ENVIRONMENT"); return e != "development" && e != "local" && e != "test" }(),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, cookie)
}
