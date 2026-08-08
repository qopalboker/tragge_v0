package server

import (
	"net/http"
	"os"
	"testing"
)

func TestCheckWebSocketOrigin(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")
	tests := []struct {
		name           string
		origin         string
		allowedOrigins string // ALLOWED_ORIGINS env var value
		codespaceName  string // CODESPACE_NAME env var value
		want           bool
	}{
		{
			name:   "no origin header is rejected",
			origin: "",
			want:   false,
		},
		{
			name:           "allowed origin passes",
			origin:         "http://localhost:5173",
			allowedOrigins: "http://localhost:5173,http://localhost:5174",
			want:           true,
		},
		{
			name:           "second allowed origin passes",
			origin:         "http://localhost:5174",
			allowedOrigins: "http://localhost:5173,http://localhost:5174",
			want:           true,
		},
		{
			name:           "disallowed origin is rejected",
			origin:         "http://evil.com",
			allowedOrigins: "http://localhost:5173,http://localhost:5174",
			want:           false,
		},
		{
			name:           "wildcard origin pattern is rejected",
			origin:         "https://myspace-5173.app.github.dev",
			allowedOrigins: "https://*.app.github.dev",
			want:           false,
		},
		{
			name:           "wildcard origin pattern rejects non-match",
			origin:         "https://evil.com",
			allowedOrigins: "https://*.app.github.dev",
			want:           false,
		},
		{
			name:   "dev default allows localhost when ALLOWED_ORIGINS not set",
			origin: "http://localhost:5173",
			want:   true,
		},
		{
			name:   "dev default allows 127.0.0.1 when ALLOWED_ORIGINS not set",
			origin: "http://127.0.0.1:8080",
			want:   true,
		},
		{
			name:           "explicit codespace origin is allowed",
			origin:         "https://myspace-5173.app.github.dev",
			allowedOrigins: "https://myspace-5173.app.github.dev",
			want:           true,
		},
		{
			name:   "dev default rejects unknown origin when ALLOWED_ORIGINS not set",
			origin: "http://evil.com",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("USER_FRONTEND_ORIGIN", "")
			t.Setenv("TRADE_CORS_ALLOWED_ORIGINS", "")
			if tt.allowedOrigins != "" {
				t.Setenv("TRADE_CORS_ALLOWED_ORIGINS", tt.allowedOrigins)
			}
			// Set environment
			if tt.allowedOrigins != "" {
				os.Setenv("ALLOWED_ORIGINS", tt.allowedOrigins)
			} else {
				os.Unsetenv("ALLOWED_ORIGINS")
			}
			defer os.Unsetenv("ALLOWED_ORIGINS")

			if tt.codespaceName != "" {
				os.Setenv("CODESPACE_NAME", tt.codespaceName)
			} else {
				os.Unsetenv("CODESPACE_NAME")
			}
			defer os.Unsetenv("CODESPACE_NAME")

			r, _ := http.NewRequest("GET", "/ws/trade", nil)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}

			got := checkWebSocketOrigin(r)
			if got != tt.want {
				t.Errorf("checkWebSocketOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}
