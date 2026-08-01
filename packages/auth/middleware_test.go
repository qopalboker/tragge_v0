package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireAuth(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	pair, _ := tokenService.GenerateTokenPair("user-123", []string{"user"})

	// Handler that checks context values
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID != "user-123" {
			t.Errorf("UserID mismatch: got %s, want user-123", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Request with valid token
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestRequireAuthMissingHeader(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestRequireAuthInvalidToken(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestRequireAuthWrongTokenType(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	pair, _ := tokenService.GenerateTokenPair("user-123", []string{"user"})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	// Use refresh token instead of access token
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.RefreshToken)
	rec := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestRequireRole(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	pair, _ := tokenService.GenerateTokenPair("admin-123", []string{"user", "admin"})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request with admin role
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()

	// Chain: RequireAuth -> RequireRole(admin)
	middleware.RequireAuth(middleware.RequireRole("admin")(handler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestRequireRoleInsufficientPermissions(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	pair, _ := tokenService.GenerateTokenPair("user-123", []string{"user"})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()

	// User doesn't have admin role
	middleware.RequireAuth(middleware.RequireRole("admin")(handler)).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d", rec.Code)
	}
}

func TestRequireAdmin(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	adminPair, _ := tokenService.GenerateTokenPair("admin-123", []string{"admin"})
	userPair, _ := tokenService.GenerateTokenPair("user-123", []string{"user"})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Admin should succeed
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+adminPair.AccessToken)
	rec := httptest.NewRecorder()
	middleware.RequireAuth(middleware.RequireAdmin(handler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Admin should succeed, got status %d", rec.Code)
	}

	// User should fail
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+userPair.AccessToken)
	rec = httptest.NewRecorder()
	middleware.RequireAuth(middleware.RequireAdmin(handler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("User should be forbidden, got status %d", rec.Code)
	}
}

func TestRequireModerator(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	adminPair, _ := tokenService.GenerateTokenPair("admin-123", []string{"admin"})
	modPair, _ := tokenService.GenerateTokenPair("mod-123", []string{"moderator"})
	userPair, _ := tokenService.GenerateTokenPair("user-123", []string{"user"})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	testCases := []struct {
		name   string
		token  string
		expect int
	}{
		{"admin", adminPair.AccessToken, http.StatusOK},
		{"moderator", modPair.AccessToken, http.StatusOK},
		{"user", userPair.AccessToken, http.StatusForbidden},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			middleware.RequireAuth(middleware.RequireModerator(handler)).ServeHTTP(rec, req)
			if rec.Code != tc.expect {
				t.Errorf("Expected status %d, got %d", tc.expect, rec.Code)
			}
		})
	}
}

func TestOptionalAuth(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	pair, _ := tokenService.GenerateTokenPair("user-123", []string{"user"})

	// With token - should have context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID != "user-123" {
			t.Errorf("UserID should be set when token provided")
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()
	middleware.OptionalAuth(handler).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Without token - should still work
	handler2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID != "" {
			t.Errorf("UserID should be empty when no token provided")
		}
		w.WriteHeader(http.StatusOK)
	})

	req = httptest.NewRequest("GET", "/", nil)
	rec = httptest.NewRecorder()
	middleware.OptionalAuth(handler2).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()

	// Empty context
	if GetUserID(ctx) != "" {
		t.Error("GetUserID should return empty string for empty context")
	}
	if GetRoles(ctx) != nil {
		t.Error("GetRoles should return nil for empty context")
	}
	if GetClaims(ctx) != nil {
		t.Error("GetClaims should return nil for empty context")
	}
	if IsAuthenticated(ctx) {
		t.Error("IsAuthenticated should return false for empty context")
	}
	if HasRole(ctx, "admin") {
		t.Error("HasRole should return false for empty context")
	}

	// Context with values
	claims := &Claims{
		UserID: "user-123",
		Roles:  []string{"user", "admin"},
	}
	ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
	ctx = context.WithValue(ctx, RolesKey, claims.Roles)
	ctx = context.WithValue(ctx, ClaimsKey, claims)

	if GetUserID(ctx) != "user-123" {
		t.Error("GetUserID should return user-123")
	}
	if len(GetRoles(ctx)) != 2 {
		t.Error("GetRoles should return 2 roles")
	}
	if !IsAuthenticated(ctx) {
		t.Error("IsAuthenticated should return true")
	}
	if !HasRole(ctx, "admin") {
		t.Error("HasRole should return true for admin")
	}
	if !IsAdmin(ctx) {
		t.Error("IsAdmin should return true")
	}
}

func TestRequireAuthRejectsCredentialQueryParameters(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	pair, err := tokenService.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatal(err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name          string
		queryName     string
		queryValue    string
		authorization string
	}{
		{name: "token", queryName: "token", queryValue: pair.AccessToken},
		{name: "access token", queryName: "access_token", queryValue: pair.AccessToken},
		{name: "jwt", queryName: "jwt", queryValue: pair.AccessToken},
		{name: "auth token alias", queryName: "auth_token", queryValue: pair.AccessToken},
		{name: "session token alias", queryName: "session_token", queryValue: pair.AccessToken},
		{name: "case insensitive alias", queryName: "ToKeN", queryValue: pair.AccessToken},
		{name: "malformed query credential", queryName: "token", queryValue: "malformed-session-credential"},
		{name: "expired query credential", queryName: "token", queryValue: "expired-session-credential"},
		{name: "invalid header cannot fall back", queryName: "token", queryValue: pair.AccessToken, authorization: "Bearer invalid"},
		{name: "valid header still rejects leaked query credential", queryName: "token", queryValue: pair.AccessToken, authorization: "Bearer " + pair.AccessToken},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			query := req.URL.Query()
			query.Set(test.queryName, test.queryValue)
			req.URL.RawQuery = query.Encode()
			if test.authorization != "" {
				req.Header.Set("Authorization", test.authorization)
			}

			rec := httptest.NewRecorder()
			middleware.RequireAuth(handler).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
			if strings.Contains(rec.Body.String(), test.queryValue) {
				t.Fatal("response echoed query credential")
			}
			if !strings.Contains(rec.Body.String(), "url_authentication_unsupported") {
				t.Fatalf("response did not contain safe migration code: %s", rec.Body.String())
			}
		})
	}
}

func TestRequireAuthHeaderStillSucceedsWithoutCredentialQuery(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)
	pair, err := tokenService.GenerateTokenPair("user-123", []string{"user"})
	if err != nil {
		t.Fatal(err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := GetUserID(r.Context()); got != "user-123" {
			t.Fatalf("user ID = %q, want user-123", got)
		}
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/protected?contest_id=contest-123", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec := httptest.NewRecorder()

	middleware.RequireAuth(handler).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
func TestTelemetryMiddlewareRedactsAndRestoresSecurityCredentials(t *testing.T) {
	const authorizationFixture = "Bearer session-credential-fixture"
	const cookieFixture = "refresh_token_user=refresh-fixture; harmless=value"
	const queryFixture = "query-session-credential-fixture"

	application := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != authorizationFixture {
			t.Fatalf("restored Authorization = %q", got)
		}
		if got := r.Header.Get("Cookie"); got != cookieFixture {
			t.Fatalf("restored Cookie = %q", got)
		}
		if got := r.URL.Query().Get("token"); got != RedactedCredentialValue {
			t.Fatalf("query credential restored unexpectedly: %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	telemetry := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatal("telemetry received Authorization header")
		}
		if got := r.Header.Get("Cookie"); got != "" {
			t.Fatal("telemetry received Cookie header")
		}
		if got := r.URL.Query().Get("token"); got != RedactedCredentialValue {
			t.Fatalf("telemetry query value = %q, want redacted", got)
		}
		RestoreSecurityCredentialsAfterTelemetry(application).ServeHTTP(w, r)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected?token="+queryFixture+"&contest_id=contest-1", nil)
	req.Header.Set("Authorization", authorizationFixture)
	req.Header.Set("Cookie", cookieFixture)
	rec := httptest.NewRecorder()
	RedactSecurityCredentialsForTelemetry(telemetry).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if got := req.Header.Get("Authorization"); got != authorizationFixture {
		t.Fatal("original request was mutated")
	}
}
func TestAuthHeaderFormats(t *testing.T) {
	config := DefaultJWTConfig("test-secret")
	tokenService := NewTokenService(config)
	middleware := NewMiddleware(tokenService)

	pair, _ := tokenService.GenerateTokenPair("user-123", []string{"user"})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	testCases := []struct {
		name   string
		header string
		expect int
	}{
		{"valid", "Bearer " + pair.AccessToken, http.StatusOK},
		{"lowercase bearer", "bearer " + pair.AccessToken, http.StatusOK},
		{"no space", "Bearer" + pair.AccessToken, http.StatusUnauthorized},
		{"wrong scheme", "Basic " + pair.AccessToken, http.StatusUnauthorized},
		{"empty token", "Bearer ", http.StatusUnauthorized},
		{"empty header", "", http.StatusUnauthorized},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			middleware.RequireAuth(handler).ServeHTTP(rec, req)
			if rec.Code != tc.expect {
				t.Errorf("Expected status %d, got %d", tc.expect, rec.Code)
			}
		})
	}
}

// TestCrossPanelTokenRejected verifies that explicit User and Admin
// authentication middleware reject the other trust domain before role checks.
func TestCrossPanelTokenRejected(t *testing.T) {
	userAuth, adminAuth, _ := newIsolatedAuthPair(t, nil)

	userPair, err := userAuth.Token.GenerateTokenPair("user-1", []string{"user"})
	if err != nil {
		t.Fatalf("user token gen: %v", err)
	}
	adminPair, err := adminAuth.Token.GenerateTokenPair("admin-1", []string{"admin"})
	if err != nil {
		t.Fatalf("admin token gen: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// A user token must be rejected by admin middleware.
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+userPair.AccessToken)
	rec := httptest.NewRecorder()
	adminAuth.Middleware.RequireAuth(handler).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("user token accepted by admin middleware (got %d, want 401)", rec.Code)
	}

	// An admin token must be rejected by user middleware.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+adminPair.AccessToken)
	rec = httptest.NewRecorder()
	userAuth.Middleware.RequireAuth(handler).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("admin token accepted by user middleware (got %d, want 401)", rec.Code)
	}

	// Sanity: tokens still work against their own panel.
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+userPair.AccessToken)
	rec = httptest.NewRecorder()
	userAuth.Middleware.RequireAuth(handler).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("user token rejected by user middleware (got %d, want 200)", rec.Code)
	}
}
