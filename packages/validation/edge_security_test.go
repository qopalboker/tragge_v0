package validation

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrustedProxyClientIPBoundary(t *testing.T) {
	trusted, err := ParseTrustedProxyCIDRs("10.0.0.0/8,2001:db8::/32")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, peer, forwarded, real, want string
	}{
		{"untrusted spoof ignored", "198.51.100.7:443", "203.0.113.8", "", "198.51.100.7"},
		{"trusted chain stops at client", "10.0.0.5:443", "203.0.113.8, 10.0.0.4", "", "203.0.113.8"},
		{"spoofed left edge ignored", "10.0.0.5:443", "192.0.2.9, 203.0.113.8, 10.0.0.4", "", "203.0.113.8"},
		{"trusted ipv6", "[2001:db8::2]:443", "2001:db9::8", "", "2001:db9::8"},
		{"malformed chain fails to peer", "10.0.0.5:443", "not-an-ip", "", "10.0.0.5"},
		{"trusted real ip", "10.0.0.5:443", "", "203.0.113.19", "203.0.113.19"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.peer
			if tc.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tc.forwarded)
			}
			if tc.real != "" {
				req.Header.Set("X-Real-IP", tc.real)
			}
			if got := ExtractClientIPWithProxies(req, trusted); got != tc.want {
				t.Fatalf("client IP=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestSecurityHeadersTrustTransport(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.0.0/8")
	handler := SecurityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	spoofed := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	spoofed.RemoteAddr = "198.51.100.9:443"
	spoofed.Header.Set("X-Forwarded-Proto", "https")
	spoofedRec := httptest.NewRecorder()
	handler.ServeHTTP(spoofedRec, spoofed)
	if spoofedRec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("untrusted forwarding header enabled HSTS")
	}
	for _, name := range []string{"X-Content-Type-Options", "X-Frame-Options", "Content-Security-Policy", "Cross-Origin-Opener-Policy", "Cross-Origin-Resource-Policy"} {
		if spoofedRec.Header().Get(name) == "" {
			t.Fatalf("security header %s missing on error response", name)
		}
	}
	secure := httptest.NewRequest(http.MethodGet, "/api/private", nil)
	secure.TLS = &tls.ConnectionState{}
	secureRec := httptest.NewRecorder()
	handler.ServeHTTP(secureRec, secure)
	if secureRec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("direct TLS response missing HSTS")
	}
}

func TestRequestLimitsFramingAndContentType(t *testing.T) {
	decode := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var value map[string]interface{}
		if DecodeJSON(w, r, &value) == nil {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	handler := MaxBytesMiddleware(1024)(ContentTypeMiddleware(decode))

	below := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	below.Header.Set("Content-Type", "application/json")
	belowRec := httptest.NewRecorder()
	handler.ServeHTTP(belowRec, below)
	if belowRec.Code != http.StatusNoContent {
		t.Fatalf("valid below-limit status=%d", belowRec.Code)
	}

	exact := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x":"`+strings.Repeat("x", 1016)+`"}`))
	exact.Header.Set("Content-Type", "application/json")
	exactRec := httptest.NewRecorder()
	handler.ServeHTTP(exactRec, exact)
	if exactRec.Code != http.StatusNoContent {
		t.Fatalf("exact-boundary status=%d", exactRec.Code)
	}

	oversized := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 1025)))
	oversized.Header.Set("Content-Type", "application/json")
	oversizedRec := httptest.NewRecorder()
	handler.ServeHTTP(oversizedRec, oversized)
	if oversizedRec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("known oversized status=%d", oversizedRec.Code)
	}

	chunked := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x":"`+strings.Repeat("x", 1018)+`"}`))
	chunked.ContentLength = -1
	chunked.TransferEncoding = []string{"chunked"}
	chunked.Header.Set("Content-Type", "application/json")
	chunkedRec := httptest.NewRecorder()
	handler.ServeHTTP(chunkedRec, chunked)
	if chunkedRec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("streamed oversized status=%d", chunkedRec.Code)
	}

	deceptive := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"x":"`+strings.Repeat("x", 1018)+`"}`))
	deceptive.ContentLength = 10
	deceptive.Header.Set("Content-Type", "application/json")
	deceptiveRec := httptest.NewRecorder()
	handler.ServeHTTP(deceptiveRec, deceptive)
	if deceptiveRec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("deceptive Content-Length status=%d", deceptiveRec.Code)
	}

	invalidFraming := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	invalidFraming.ContentLength = -1
	invalidFraming.TransferEncoding = []string{"gzip"}
	invalidFraming.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidFraming)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid framing status=%d", invalidRec.Code)
	}

	wrongType := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	wrongType.Header.Set("Content-Type", "text/plain")
	wrongRec := httptest.NewRecorder()
	handler.ServeHTTP(wrongRec, wrongType)
	if wrongRec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content type status=%d", wrongRec.Code)
	}

	malformed := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	malformed.Header.Set("Content-Type", "application/json")
	malformedRec := httptest.NewRecorder()
	handler.ServeHTTP(malformedRec, malformed)
	if malformedRec.Code != http.StatusBadRequest {
		t.Fatalf("malformed JSON status=%d", malformedRec.Code)
	}
}

func TestCSRFBrowserAndBearerContexts(t *testing.T) {
	config := CSRFConfig{
		Context: "user", AllowedOrigins: []string{"https://user.example.invalid"},
		CookieNames: []string{"user_refresh"}, RequireXRequestedWith: true,
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	request := func(origin, authorization string, cookie bool) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Authorization", authorization)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		if cookie {
			req.AddCookie(&http.Cookie{Name: "user_refresh", Value: "fixture"})
		}
		rec := httptest.NewRecorder()
		CSRFMiddleware(config)(next).ServeHTTP(rec, req)
		return rec.Code
	}
	if got := request("https://user.example.invalid", "", true); got != http.StatusNoContent {
		t.Fatalf("valid browser CSRF status=%d", got)
	}
	if got := request("https://admin.example.invalid", "", true); got != http.StatusForbidden {
		t.Fatalf("cross-context CSRF status=%d", got)
	}
	if got := request("", "Bearer fixture", false); got != http.StatusNoContent {
		t.Fatalf("bearer-only request status=%d", got)
	}
}

func TestUserAndAdminCORSContextsAreExactAndDistinct(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("USER_CORS_ALLOWED_ORIGINS", "https://user.example.invalid")
	t.Setenv("ADMIN_CORS_ALLOWED_ORIGINS", "https://admin.example.invalid")
	user := UserBFFCORSConfig()
	admin := AdminBFFCORSConfig()
	if strings.Join(user.AllowedOrigins, ",") == strings.Join(admin.AllowedOrigins, ",") {
		t.Fatal("User and Admin CORS origins collide")
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	run := func(config CORSConfig, origin string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		CORSMiddleware(config)(next).ServeHTTP(rec, req)
		return rec.Code
	}
	if got := run(user, "https://user.example.invalid"); got != http.StatusNoContent {
		t.Fatalf("User origin status=%d", got)
	}
	if got := run(admin, "https://admin.example.invalid"); got != http.StatusNoContent {
		t.Fatalf("Admin origin status=%d", got)
	}
	if got := run(admin, "https://user.example.invalid"); got != http.StatusForbidden {
		t.Fatalf("User origin on Admin surface status=%d", got)
	}
	if got := run(user, "https://admin.example.invalid"); got != http.StatusForbidden {
		t.Fatalf("Admin origin on User surface status=%d", got)
	}
	if got := run(user, "null"); got != http.StatusForbidden {
		t.Fatalf("null origin status=%d", got)
	}
	if got := run(user, ""); got != http.StatusNoContent {
		t.Fatalf("same-origin request without Origin status=%d", got)
	}
	if err := ValidateCORSConfig(CORSConfig{AllowedOrigins: []string{"https://user@example.invalid"}}, true); err == nil {
		t.Fatal("origin containing userinfo accepted")
	}
}

func TestEdgeEnvironmentProductionValidation(t *testing.T) {
	valid := map[string]string{
		"ENVIRONMENT": "production", "TRUSTED_PROXY_CIDRS": "10.0.0.0/8",
		"USER_CORS_ALLOWED_ORIGINS":    "https://user.example.invalid",
		"ADMIN_CORS_ALLOWED_ORIGINS":   "https://admin.example.invalid",
		"TRADE_CORS_ALLOWED_ORIGINS":   "https://trade.example.invalid",
		"PAYMENT_CORS_ALLOWED_ORIGINS": "https://pay.example.invalid",
		"EDGE_MAX_BODY_BYTES":          "1048576", "EDGE_MAX_UPLOAD_BYTES": "36700160",
	}
	getenv := func(name string) string { return valid[name] }
	if _, err := LoadAndValidateEdgeEnvironment(getenv); err != nil {
		t.Fatalf("valid production environment rejected: %v", err)
	}
	delete(valid, "TRUSTED_PROXY_CIDRS")
	if _, err := LoadAndValidateEdgeEnvironment(getenv); err == nil {
		t.Fatal("missing production proxy policy accepted")
	}
	valid["TRUSTED_PROXY_CIDRS"] = "not-a-cidr"
	if _, err := LoadAndValidateEdgeEnvironment(getenv); err == nil {
		t.Fatal("malformed proxy policy accepted")
	}
	valid["TRUSTED_PROXY_CIDRS"] = "10.0.0.0/8"
	valid["USER_CORS_ALLOWED_ORIGINS"] = "*"
	if _, err := LoadAndValidateEdgeEnvironment(getenv); err == nil {
		t.Fatal("wildcard production origin accepted")
	}
	valid["USER_CORS_ALLOWED_ORIGINS"] = "https://user@example.invalid"
	if _, err := LoadAndValidateEdgeEnvironment(getenv); err == nil {
		t.Fatal("origin containing userinfo accepted by startup validation")
	}
	valid["USER_CORS_ALLOWED_ORIGINS"] = "https://user.example.invalid"
	valid["ADMIN_CORS_ALLOWED_ORIGINS"] = "https://user.example.invalid"
	if _, err := LoadAndValidateEdgeEnvironment(getenv); err == nil {
		t.Fatal("colliding production User/Admin origins accepted")
	}
}
