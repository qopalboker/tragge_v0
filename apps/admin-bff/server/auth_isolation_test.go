package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

func adminCookiesByName(cookies []*http.Cookie) map[string]*http.Cookie {
	result := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}

func TestAdminCookiesAreContextScopedAndProductionSecure(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	req := httptest.NewRequest(http.MethodPost, "http://internal/api/admin/auth/login", nil)
	rec := httptest.NewRecorder()
	setAdminRefreshTokenCookie(rec, req, "fixture-token")
	cookies := adminCookiesByName(rec.Result().Cookies())

	refresh := cookies[auth.AdminRefreshCookieName]
	hint := cookies[auth.AdminSessionHintCookieName]
	if refresh == nil || hint == nil {
		t.Fatalf("Admin cookie pair missing: %#v", cookies)
	}
	if refresh.Name == auth.UserRefreshCookieName || hint.Name == auth.UserSessionHintCookieName {
		t.Fatal("Admin flow emitted a User cookie")
	}
	if refresh.Path != auth.AdminRefreshCookiePath || !refresh.HttpOnly || !refresh.Secure || refresh.SameSite != http.SameSiteNoneMode {
		t.Fatalf("unexpected production refresh cookie attributes: %#v", refresh)
	}
	if hint.Path != "/" || hint.HttpOnly || !hint.Secure || hint.SameSite != http.SameSiteNoneMode {
		t.Fatalf("unexpected production hint cookie attributes: %#v", hint)
	}
}

func TestAdminCookieDevelopmentAndLogoutIsolation(t *testing.T) {
	t.Setenv("ENVIRONMENT", "test")
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/admin/auth/logout", nil)
	rec := httptest.NewRecorder()
	clearAdminRefreshTokenCookie(rec, req)
	cookies := adminCookiesByName(rec.Result().Cookies())
	if len(cookies) != 2 {
		t.Fatalf("cleared %d cookies, want 2 Admin cookies", len(cookies))
	}
	for name, cookie := range cookies {
		if name == auth.UserRefreshCookieName || name == auth.UserSessionHintCookieName {
			t.Fatal("Admin logout cleared a User cookie")
		}
		if cookie.MaxAge != -1 || cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("unexpected test logout cookie: %#v", cookie)
		}
	}
}
