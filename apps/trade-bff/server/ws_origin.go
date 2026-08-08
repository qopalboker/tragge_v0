package server

import (
	"net/http"
	"strings"

	"github.com/Parsaeffatravesh/tragge/packages/validation"
)

// checkWebSocketOrigin validates the Origin header for WebSocket upgrade requests.
// It accepts only an exact origin from the Trade context configuration.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	for _, allowed := range validation.TradeBFFCORSConfig().AllowedOrigins {
		if !strings.Contains(allowed, "*") && origin == strings.TrimSpace(allowed) {
			return true
		}
	}
	return false
}
