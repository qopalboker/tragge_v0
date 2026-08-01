package validation

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

func runCSRF(config CSRFConfig, origin string) int {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/api/context/action", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	CSRFMiddleware(config)(next).ServeHTTP(rec, req)
	return rec.Code
}

func TestUserAdminCSRFContextsAreIsolated(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("ALLOWED_ORIGINS", "https://shared.example.invalid")
	t.Setenv("USER_FRONTEND_ORIGIN", "https://app.example.invalid")
	t.Setenv("ADMIN_FRONTEND_ORIGIN", "https://admin.example.invalid")

	userConfig := UserBFFCSRFConfig()
	adminConfig := AdminBFFCSRFConfig()
	if userConfig.Context != auth.UserCSRFContext || adminConfig.Context != auth.AdminCSRFContext {
		t.Fatal("User/Admin CSRF context identifiers are not explicit")
	}
	if userConfig.Context == adminConfig.Context {
		t.Fatal("User/Admin CSRF contexts collide")
	}

	tests := []struct {
		name   string
		config CSRFConfig
		origin string
		want   int
	}{
		{"User origin satisfies User", userConfig, "https://app.example.invalid", http.StatusNoContent},
		{"Admin origin satisfies Admin", adminConfig, "https://admin.example.invalid", http.StatusNoContent},
		{"User origin rejected by Admin", adminConfig, "https://app.example.invalid", http.StatusForbidden},
		{"Admin origin rejected by User", userConfig, "https://admin.example.invalid", http.StatusForbidden},
		{"User context rejects missing origin", userConfig, "", http.StatusForbidden},
		{"Admin context rejects missing origin", adminConfig, "", http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runCSRF(test.config, test.origin); got != test.want {
				t.Fatalf("status=%d want=%d", got, test.want)
			}
		})
	}
}
