package server

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/Parsaeffatravesh/tragge/packages/validation"
)

// checkWebSocketOrigin validates the Origin header for WebSocket upgrade requests.
// It reads ALLOWED_ORIGINS env var and also allows localhost/Codespaces in development.
// Requests without an Origin header are allowed (same-origin browser behavior).
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // Allow requests without Origin header (e.g., same-origin)
	}

	allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
	if allowedOriginsEnv == "" {
		// Only allow dev fallback in development/local environments.
		// In production, missing ALLOWED_ORIGINS means reject all cross-origin requests.
		env := os.Getenv("ENVIRONMENT")
		if env == "" || env == "development" || env == "local" || env == "test" {
			if strings.HasPrefix(origin, "http://localhost:") ||
				strings.HasPrefix(origin, "https://localhost:") ||
				strings.HasPrefix(origin, "http://127.0.0.1:") ||
				strings.HasPrefix(origin, "https://127.0.0.1:") ||
				checkCodespaceOrigin(origin) {
				return true
			}
		}
		return false
	}

	allowedOrigins := strings.Split(allowedOriginsEnv, ",")
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		if origin == allowed {
			return true
		}
		// Support wildcard patterns like https://*.app.github.dev
		if validation.MatchWildcardOrigin(origin, allowed) {
			return true
		}
	}
	return false
}

// checkCodespaceOrigin validates that the origin belongs to the user's own GitHub Codespace.
func checkCodespaceOrigin(origin string) bool {
	codespaceName := os.Getenv("CODESPACE_NAME")
	if codespaceName == "" {
		return false
	}
	prefix := fmt.Sprintf("https://%s-", codespaceName)
	return strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, ".app.github.dev")
}
