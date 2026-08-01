package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

func cookiesByName(cookies []*http.Cookie) map[string]*http.Cookie {
	result := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}

func TestUserCookiesAreContextScopedAndProductionSecure(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")
	req := httptest.NewRequest(http.MethodPost, "http://internal/api/user/auth/login", nil)
	rec := httptest.NewRecorder()
	(&App{}).setRefreshTokenCookie(rec, req, "fixture-token")
	cookies := cookiesByName(rec.Result().Cookies())

	refresh := cookies[auth.UserRefreshCookieName]
	hint := cookies[auth.UserSessionHintCookieName]
	if refresh == nil || hint == nil {
		t.Fatalf("User cookie pair missing: %#v", cookies)
	}
	if refresh.Name == auth.AdminRefreshCookieName || hint.Name == auth.AdminSessionHintCookieName {
		t.Fatal("User flow emitted an Admin cookie")
	}
	if refresh.Path != auth.UserRefreshCookiePath || !refresh.HttpOnly || !refresh.Secure || refresh.SameSite != http.SameSiteNoneMode {
		t.Fatalf("unexpected production refresh cookie attributes: %#v", refresh)
	}
	if hint.Path != "/" || hint.HttpOnly || !hint.Secure || hint.SameSite != http.SameSiteNoneMode {
		t.Fatalf("unexpected production hint cookie attributes: %#v", hint)
	}
}

func TestUserCookieDevelopmentAndLogoutIsolation(t *testing.T) {
	t.Setenv("ENVIRONMENT", "test")
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/user/auth/logout", nil)
	rec := httptest.NewRecorder()
	(&App{}).clearRefreshTokenCookie(rec, req)
	cookies := cookiesByName(rec.Result().Cookies())
	if len(cookies) != 2 {
		t.Fatalf("cleared %d cookies, want 2 User cookies", len(cookies))
	}
	for name, cookie := range cookies {
		if name == auth.AdminRefreshCookieName || name == auth.AdminSessionHintCookieName {
			t.Fatal("User logout cleared an Admin cookie")
		}
		if cookie.MaxAge != -1 || cookie.Secure || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("unexpected test logout cookie: %#v", cookie)
		}
	}
}
