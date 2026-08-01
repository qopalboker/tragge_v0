package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

// TestAdminRefreshPreservesRole verifies the claim made in PR #932: an
// admin session survives a refresh with its admin role intact, so
// admin-only endpoints keep returning 200 after the access token is
// rolled. The negative case confirms that a non-admin session does NOT
// get elevated by the refresh flow.
//
// The test uses a minimal http mux wired to the real auth package
// (session store backed by the testcontainers Redis) rather than
// spinning up user-bff + admin-bff, because the property under test
// lives entirely in packages/auth — roles are read from the session
// record at refresh time, not re-queried from the DB. A heavier
// end-to-end harness would just be slower at measuring the same thing.
func TestAdminRefreshPreservesRole(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Wire the auth package against the testcontainers Redis so that
	// Refresh() actually reads the persisted session record. Without a
	// session store, Refresh falls back to trusting the JWT claims —
	// which bypasses the codepath we want to exercise.
	authConfig := auth.DefaultConfig()
	authConfig.JWTSecret = env.JWTSecret
	authConfig.AccessTokenTTL = 2 * time.Second // short so expiry is cheap to simulate
	authConfig.RefreshTokenTTL = 10 * time.Minute
	authConfig.Redis = env.RedisClient
	authSvc := auth.New(authConfig)

	t.Run("admin role survives refresh and reaches admin endpoint", func(t *testing.T) {
		srv := newAdminRefreshHarness(t, authSvc)
		defer srv.Close()

		// Login as an admin user directly through the auth service. We
		// don't need the full DB-backed login handler here — the
		// property under test is post-login refresh behavior.
		loginTokens, sessionID, err := authSvc.Login(ctx, "user-admin-1", []string{"admin"}, "test-agent", "127.0.0.1")
		if err != nil {
			t.Fatalf("admin login: %v", err)
		}
		if sessionID == "" {
			t.Fatal("expected session ID from admin login; got empty")
		}

		// Force-expire the access token by waiting past TTL + skew. The
		// middleware rejects expired tokens, so this is equivalent to
		// the browser having a stale access token in memory.
		time.Sleep(3 * time.Second)
		if status := srv.callAdminEndpoint(t, loginTokens.AccessToken); status != http.StatusUnauthorized {
			t.Fatalf("expected 401 on expired admin token; got %d", status)
		}

		// Refresh through the shared refresh endpoint (same one admin-bff
		// sessions use — cookie path is /api/user/auth).
		refreshed := srv.callRefresh(t, sessionID, loginTokens.RefreshToken)
		if refreshed.AccessToken == loginTokens.AccessToken {
			t.Fatal("refresh returned the same access token; expected a rolled token")
		}

		// Admin endpoint must accept the freshly-refreshed access token.
		if status := srv.callAdminEndpoint(t, refreshed.AccessToken); status != http.StatusOK {
			t.Fatalf("admin endpoint after refresh: expected 200, got %d", status)
		}
	})

	t.Run("non-admin session refresh does not reach admin endpoint", func(t *testing.T) {
		srv := newAdminRefreshHarness(t, authSvc)
		defer srv.Close()

		// Login as a plain user.
		loginTokens, sessionID, err := authSvc.Login(ctx, "user-regular-1", []string{"user"}, "test-agent", "127.0.0.1")
		if err != nil {
			t.Fatalf("user login: %v", err)
		}

		// Refresh without waiting — negative path doesn't need the
		// expiry step, it just needs to prove that refresh can't
		// launder a user role into an admin role.
		refreshed := srv.callRefresh(t, sessionID, loginTokens.RefreshToken)

		if status := srv.callAdminEndpoint(t, refreshed.AccessToken); status != http.StatusForbidden {
			t.Fatalf("admin endpoint for non-admin after refresh: expected 403, got %d", status)
		}
	})
}

// adminRefreshHarness mounts just enough HTTP surface to exercise the
// refresh-then-call-admin flow: one admin-gated endpoint and one
// refresh endpoint that mirrors user-bff's handleRefresh signature.
type adminRefreshHarness struct {
	server *httptest.Server
	client *http.Client
}

func newAdminRefreshHarness(t *testing.T, authSvc *auth.Auth) *adminRefreshHarness {
	t.Helper()

	mux := http.NewServeMux()

	// GET /api/admin/test — gated by RequireAuth + RequireRole("admin")
	adminOnly := authSvc.Middleware.RequireAuth(
		authSvc.Middleware.RequireRole("admin")(
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}),
		),
	)
	mux.Handle("GET /api/admin/test", adminOnly)

	// POST /api/user/auth/refresh — mirrors user-bff flow
	mux.HandleFunc("POST /api/user/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID    string `json:"session_id"`
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		pair, err := authSvc.Refresh(r.Context(), body.SessionID, body.RefreshToken)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, pair)
	})

	srv := httptest.NewServer(mux)
	return &adminRefreshHarness{
		server: srv,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *adminRefreshHarness) Close() {
	h.server.Close()
}

func (h *adminRefreshHarness) callAdminEndpoint(t *testing.T, accessToken string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.server.URL+"/api/admin/test", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("admin endpoint call: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func (h *adminRefreshHarness) callRefresh(t *testing.T, sessionID, refreshToken string) *auth.TokenPair {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"session_id":    sessionID,
		"refresh_token": refreshToken,
	})
	resp, err := h.client.Post(h.server.URL+"/api/user/auth/refresh", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		t.Fatalf("refresh: status=%d body=%s", resp.StatusCode, string(msg))
	}
	var pair auth.TokenPair
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	return &pair
}
