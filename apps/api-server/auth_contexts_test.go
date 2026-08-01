package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

func isolatedEnvironment() map[string]string {
	return map[string]string{
		"ENVIRONMENT":              "production",
		"JWT_SECRET_USER":          "F1x!Ua#9Qv$2Lm^8Za&4Nc*7Rt@5Wp_K3dB6",
		"JWT_REFRESH_SECRET_USER":  "G2y@Ub$8Rw%3Mn&7Yb*5Od!6Su#4Xq_L9eC1",
		"JWT_SECRET_ADMIN":         "H3z#Ac%7Sx^4No*6Xc!8Pe@5Tv$3Yr_M2fD9",
		"JWT_REFRESH_SECRET_ADMIN": "J4w$Ad^6Ty&5Op!9Wd@7Qf#4Uw%2Zs_N8gE3",
		"JWT_ISSUER_USER":          auth.IssuerUser,
		"JWT_ISSUER_ADMIN":         auth.IssuerAdmin,
		"JWT_AUDIENCE_USER":        auth.AudienceUser,
		"JWT_AUDIENCE_ADMIN":       auth.AudienceAdmin,
		"USER_FRONTEND_ORIGIN":     "https://app.example.invalid",
		"ADMIN_FRONTEND_ORIGIN":    "https://admin.example.invalid",
	}
}

func loadTestIsolation(t *testing.T, values map[string]string) auth.IsolationConfig {
	t.Helper()
	lookup := func(name string) string { return values[name] }
	config, err := loadAuthIsolationConfig(lookup, lookup)
	if err != nil {
		t.Fatalf("load isolation config: %v", err)
	}
	return config
}

func TestMergedAPIConstructsSeparateAuthContexts(t *testing.T) {
	config := loadTestIsolation(t, isolatedEnvironment())
	userAuth, adminAuth, err := buildAuthContexts(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if userAuth == adminAuth {
		t.Fatal("merged API reused one generic authentication singleton")
	}
	if userAuth.Context() != auth.ContextUser || adminAuth.Context() != auth.ContextAdmin {
		t.Fatal("merged API constructed the wrong authentication contexts")
	}

	userPair, _ := userAuth.Token.GenerateTokenPair("user-1", []string{"user"})
	adminPair, _ := adminAuth.Token.GenerateTokenPair("admin-1", []string{"support_admin"})
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	tests := []struct {
		name       string
		middleware func(http.Handler) http.Handler
		token      string
		want       int
	}{
		{"User token to User endpoint", userAuth.Middleware.RequireAuth, userPair.AccessToken, http.StatusNoContent},
		{"Admin token to Admin endpoint", adminAuth.Middleware.RequireAuth, adminPair.AccessToken, http.StatusNoContent},
		{"User token to Admin endpoint", adminAuth.Middleware.RequireAuth, userPair.AccessToken, http.StatusUnauthorized},
		{"Admin token to User endpoint", userAuth.Middleware.RequireAuth, adminPair.AccessToken, http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/representative", nil)
			req.Header.Set("Authorization", "Bearer "+test.token)
			rec := httptest.NewRecorder()
			test.middleware(endpoint).ServeHTTP(rec, req)
			if rec.Code != test.want {
				t.Fatalf("status=%d want=%d body=%q", rec.Code, test.want, rec.Body.String())
			}
			if test.want == http.StatusUnauthorized && (strings.Contains(rec.Body.String(), "signature") || strings.Contains(rec.Body.String(), "audience") || strings.Contains(rec.Body.String(), "context")) {
				t.Fatal("client response leaked cryptographic validation detail")
			}
		})
	}
}

func TestMergedAPIProductionStartupRejectsInvalidIsolation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]string)
	}{
		{"missing User secret", func(v map[string]string) { delete(v, "JWT_SECRET_USER") }},
		{"equal secrets", func(v map[string]string) { v["JWT_SECRET_ADMIN"] = v["JWT_SECRET_USER"] }},
		{"missing audience", func(v map[string]string) { delete(v, "JWT_AUDIENCE_ADMIN") }},
		{"equal audience", func(v map[string]string) { v["JWT_AUDIENCE_ADMIN"] = v["JWT_AUDIENCE_USER"] }},
		{"equal CSRF origin", func(v map[string]string) { v["ADMIN_FRONTEND_ORIGIN"] = v["USER_FRONTEND_ORIGIN"] }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := isolatedEnvironment()
			test.mutate(values)
			lookup := func(name string) string { return values[name] }
			if _, err := loadAuthIsolationConfig(lookup, lookup); err == nil {
				t.Fatal("invalid production startup configuration accepted")
			}
		})
	}
}

func TestMergedAPIProductionStartupAcceptsValidIsolation(t *testing.T) {
	config := loadTestIsolation(t, isolatedEnvironment())
	userAuth, adminAuth, err := buildAuthContexts(config, nil)
	if err != nil {
		t.Fatalf("valid startup config rejected: %v", err)
	}
	if userAuth.Context() == adminAuth.Context() {
		t.Fatal("valid startup produced colliding contexts")
	}
}
