package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
)

type memoryWSTicketStore struct {
	mu      sync.Mutex
	values  map[string]string
	lastKey string
	lastTTL time.Duration
}

func newMemoryWSTicketStore() *memoryWSTicketStore {
	return &memoryWSTicketStore{values: make(map[string]string)}
}

func (s *memoryWSTicketStore) Put(_ context.Context, key, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	s.lastKey = key
	s.lastTTL = ttl
	return nil
}

func (s *memoryWSTicketStore) Take(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return "", errWSTicketInvalid
	}
	delete(s.values, key)
	return value, nil
}

func (s *memoryWSTicketStore) mutate(key string, mutate func(*wsTicketRecord)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var record wsTicketRecord
	if err := json.Unmarshal([]byte(s.values[key]), &record); err != nil {
		panic(err)
	}
	mutate(&record)
	encoded, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	s.values[key] = string(encoded)
}

type memoryWSSessions struct {
	mu       sync.Mutex
	sessions map[string]*auth.Session
}

func (s *memoryWSSessions) Get(_ context.Context, sessionID string) (*auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, auth.ErrSessionNotFound
	}
	copy := *session
	return &copy, nil
}

func newDeterministicWSTicketService(store *memoryWSTicketStore, sessions *memoryWSSessions, now time.Time) *wsTicketService {
	service := newWSTicketService(store, sessions, auth.ContextUser)
	service.now = func() time.Time { return now }
	service.random = bytes.NewReader(append(bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32)...))
	return service
}

func TestWSTicketIssueAndConsumeIsHashedBoundAndSingleUse(t *testing.T) {
	store := newMemoryWSTicketStore()
	sessions := &memoryWSSessions{sessions: map[string]*auth.Session{
		"session-user-1": {ID: "session-user-1", UserID: "user-1"},
	}}
	now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
	service := newDeterministicWSTicketService(store, sessions, now)

	issue, err := service.Issue(context.Background(), "user-1", "session-user-1", "contest-1")
	if err != nil {
		t.Fatal(err)
	}
	if issue.ExpiresIn != 10 || store.lastTTL != defaultWSTicketTTL {
		t.Fatalf("unexpected ticket TTL: response=%d store=%s", issue.ExpiresIn, store.lastTTL)
	}
	if strings.Contains(store.lastKey, issue.Ticket) {
		t.Fatal("raw WebSocket ticket appeared in persistence key")
	}
	stored := store.values[store.lastKey]
	if strings.Contains(stored, issue.Ticket) || strings.Contains(stored, issue.Binding) {
		t.Fatal("recoverable WebSocket credential appeared in persistence value")
	}

	userID, err := service.Consume(context.Background(), issue.Ticket, issue.Binding, "contest-1")
	if err != nil || userID != "user-1" {
		t.Fatalf("consume = (%q, %v), want user-1", userID, err)
	}
	if _, err := service.Consume(context.Background(), issue.Ticket, issue.Binding, "contest-1"); !errors.Is(err, errWSTicketInvalid) {
		t.Fatalf("replay error = %v, want invalid ticket", err)
	}
}

func TestWSTicketRejectsWrongBindingPurposeContextResourceAndSession(t *testing.T) {
	tests := []struct {
		name       string
		binding    func(*wsTicketIssue) string
		contestID  string
		mutate     func(*wsTicketRecord)
		invalidate func(*memoryWSSessions)
		advance    time.Duration
	}{
		{name: "wrong binding", binding: func(*wsTicketIssue) string { return strings.Repeat("a", 64) }, contestID: "contest-1"},
		{name: "wrong resource", binding: func(i *wsTicketIssue) string { return i.Binding }, contestID: "contest-2"},
		{name: "wrong purpose", binding: func(i *wsTicketIssue) string { return i.Binding }, contestID: "contest-1", mutate: func(r *wsTicketRecord) { r.Purpose = "download" }},
		{name: "wrong context", binding: func(i *wsTicketIssue) string { return i.Binding }, contestID: "contest-1", mutate: func(r *wsTicketRecord) { r.Context = auth.ContextAdmin }},
		{name: "wrong user session", binding: func(i *wsTicketIssue) string { return i.Binding }, contestID: "contest-1", invalidate: func(s *memoryWSSessions) { s.sessions["session-user-1"].UserID = "user-2" }},
		{name: "revoked session", binding: func(i *wsTicketIssue) string { return i.Binding }, contestID: "contest-1", invalidate: func(s *memoryWSSessions) { delete(s.sessions, "session-user-1") }},
		{name: "expired", binding: func(i *wsTicketIssue) string { return i.Binding }, contestID: "contest-1", advance: defaultWSTicketTTL},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newMemoryWSTicketStore()
			sessions := &memoryWSSessions{sessions: map[string]*auth.Session{
				"session-user-1": {ID: "session-user-1", UserID: "user-1"},
			}}
			now := time.Date(2026, time.July, 26, 12, 0, 0, 0, time.UTC)
			service := newDeterministicWSTicketService(store, sessions, now)
			issue, err := service.Issue(context.Background(), "user-1", "session-user-1", "contest-1")
			if err != nil {
				t.Fatal(err)
			}
			if test.mutate != nil {
				store.mutate(store.lastKey, test.mutate)
			}
			if test.invalidate != nil {
				test.invalidate(sessions)
			}
			service.now = func() time.Time { return now.Add(test.advance) }

			if _, err := service.Consume(context.Background(), issue.Ticket, test.binding(issue), test.contestID); !errors.Is(err, errWSTicketInvalid) {
				t.Fatalf("consume error = %v, want invalid ticket", err)
			}
		})
	}
}

func TestWSTicketConcurrentConsumptionAllowsExactlyOne(t *testing.T) {
	store := newMemoryWSTicketStore()
	sessions := &memoryWSSessions{sessions: map[string]*auth.Session{
		"session-user-1": {ID: "session-user-1", UserID: "user-1"},
	}}
	service := newWSTicketService(store, sessions, auth.ContextUser)
	issue, err := service.Issue(context.Background(), "user-1", "session-user-1", "contest-1")
	if err != nil {
		t.Fatal(err)
	}

	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.Consume(context.Background(), issue.Ticket, issue.Binding, "contest-1"); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful consumes = %d, want 1", got)
	}
}

func TestWSTicketBindingCookieSecurity(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment string
		requestURL  string
		forwarded   string
		wantSecure  bool
	}{
		{name: "production", environment: "production", requestURL: "http://example.invalid/ws/trade", wantSecure: true},
		{name: "forwarded https", environment: "development", requestURL: "http://example.invalid/ws/trade", forwarded: "https", wantSecure: true},
		{name: "local http", environment: "test", requestURL: "http://example.invalid/ws/trade", wantSecure: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.requestURL, nil)
			if test.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", test.forwarded)
			}
			rec := httptest.NewRecorder()
			setWSTicketBindingCookie(rec, strings.Repeat("b", 64), secureWSTicketCookie(test.environment, req), defaultWSTicketTTL)
			cookies := rec.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("cookies = %d, want 1", len(cookies))
			}
			cookie := cookies[0]
			if cookie.Name != wsTicketBindingCookieName || cookie.Path != wsTicketBindingCookiePath || !cookie.HttpOnly ||
				cookie.SameSite != http.SameSiteStrictMode || cookie.Secure != test.wantSecure || cookie.MaxAge != 10 {
				t.Fatalf("unexpected binding cookie attributes: name=%q path=%q httpOnly=%t secure=%t sameSite=%d maxAge=%d", cookie.Name, cookie.Path, cookie.HttpOnly, cookie.Secure, cookie.SameSite, cookie.MaxAge)
			}
		})
	}
}
func TestAuthenticateWebSocketRequestRejectsQueryJWTAndPreservesUserIsolation(t *testing.T) {
	sessions := &memoryWSSessions{sessions: map[string]*auth.Session{
		"user-session": {ID: "user-session", UserID: "user-1"},
	}}
	userTokens := auth.NewTokenService(&auth.JWTConfig{
		Secret: []byte("user-access-fixture-secret"), RefreshSecret: []byte("user-refresh-fixture-secret"),
		Issuer: auth.IssuerUser, Audience: []string{auth.AudienceUser}, Context: auth.ContextUser,
		AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour,
	})
	adminTokens := auth.NewTokenService(&auth.JWTConfig{
		Secret: []byte("admin-access-fixture-secret"), RefreshSecret: []byte("admin-refresh-fixture-secret"),
		Issuer: auth.IssuerAdmin, Audience: []string{auth.AudienceAdmin}, Context: auth.ContextAdmin,
		AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour,
	})
	userPair, err := userTokens.GenerateTokenPairWithSession("user-1", []string{"USER"}, "user-session")
	if err != nil {
		t.Fatal(err)
	}
	adminPair, err := adminTokens.GenerateTokenPairWithSession("admin-1", []string{"SUPPORT_ADMIN"}, "admin-session")
	if err != nil {
		t.Fatal(err)
	}
	access := &wsAccessAuthenticator{tokens: userTokens, sessions: sessions}

	tests := []struct {
		name   string
		query  string
		header string
		wantOK bool
	}{
		{name: "user header", header: "Bearer " + userPair.AccessToken, wantOK: true},
		{name: "user query token", query: "token=" + userPair.AccessToken},
		{name: "user query access token", query: "access_token=" + userPair.AccessToken},
		{name: "user query jwt", query: "jwt=" + userPair.AccessToken},
		{name: "query token plus valid header", query: "token=" + userPair.AccessToken, header: "Bearer " + userPair.AccessToken},
		{name: "admin header in user context", header: "Bearer " + adminPair.AccessToken},
		{name: "missing credential"},
		{name: "malformed header", header: "Bearer malformed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := "/ws/trade?contest_id=contest-1"
			if test.query != "" {
				path += "&" + test.query
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			if test.header != "" {
				req.Header.Set("Authorization", test.header)
			}
			userID, err := authenticateWebSocketRequest(context.Background(), req, "contest-1", nil, access)
			if test.wantOK {
				if err != nil || userID != "user-1" {
					t.Fatalf("authenticate = (%q, %v), want user-1", userID, err)
				}
				return
			}
			if !errors.Is(err, errWSTicketInvalid) {
				t.Fatalf("authenticate error = %v, want invalid credential", err)
			}
		})
	}
}

func TestAuthenticateWebSocketRequestAcceptsBoundTicketAndRejectsReplay(t *testing.T) {
	store := newMemoryWSTicketStore()
	sessions := &memoryWSSessions{sessions: map[string]*auth.Session{
		"user-session": {ID: "user-session", UserID: "user-1"},
	}}
	service := newWSTicketService(store, sessions, auth.ContextUser)
	issue, err := service.Issue(context.Background(), "user-1", "user-session", "contest-1")
	if err != nil {
		t.Fatal(err)
	}

	makeRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/ws/trade?contest_id=contest-1&ticket="+issue.Ticket, nil)
		req.AddCookie(&http.Cookie{Name: wsTicketBindingCookieName, Value: issue.Binding, Path: wsTicketBindingCookiePath})
		return req
	}
	userID, err := authenticateWebSocketRequest(context.Background(), makeRequest(), "contest-1", service, nil)
	if err != nil || userID != "user-1" {
		t.Fatalf("ticket authenticate = (%q, %v), want user-1", userID, err)
	}
	if _, err := authenticateWebSocketRequest(context.Background(), makeRequest(), "contest-1", service, nil); !errors.Is(err, errWSTicketInvalid) {
		t.Fatalf("replay error = %v, want invalid ticket", err)
	}
}

func TestHandleWSTicketIssuesOnlyBoundedNoStoreArtifacts(t *testing.T) {
	store := newMemoryWSTicketStore()
	sessions := &memoryWSSessions{sessions: map[string]*auth.Session{
		"user-session": {ID: "user-session", UserID: "user-1"},
	}}
	service := newWSTicketService(store, sessions, auth.ContextUser)
	app := &App{wsTickets: service, config: &Config{Environment: "production"}}
	claims := &auth.Claims{UserID: "user-1", SessionID: "user-session", AuthContext: auth.ContextUser}
	ctx := context.WithValue(context.Background(), auth.UserIDKey, "user-1")
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
	req := httptest.NewRequest(http.MethodPost, "/api/trade/ws-ticket", strings.NewReader(`{"contest_id":"contest-1"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()

	app.handleWSTicket(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	var response struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !isOpaqueCredential(response.Ticket) || response.ExpiresIn != 10 {
		t.Fatal("ticket response was not a 10-second opaque credential")
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != wsTicketBindingCookieName || !cookie.HttpOnly || !cookie.Secure ||
		cookie.Path != wsTicketBindingCookiePath || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("ticket binding cookie does not have the required production isolation attributes")
	}
	if strings.Contains(rec.Body.String(), cookie.Value) {
		t.Fatal("HttpOnly binding credential appeared in response body")
	}
}

func TestWSTicketTelemetryRedactionPreservesHandshakeContext(t *testing.T) {
	store := newMemoryWSTicketStore()
	sessions := &memoryWSSessions{sessions: map[string]*auth.Session{
		"user-session": {ID: "user-session", UserID: "user-1"},
	}}
	service := newWSTicketService(store, sessions, auth.ContextUser)
	issue, err := service.Issue(context.Background(), "user-1", "user-session", "contest-1")
	if err != nil {
		t.Fatal(err)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ticket"); got != auth.RedactedCredentialValue {
			t.Fatalf("telemetry-visible ticket = %q, want redacted", got)
		}
		userID, err := authenticateWebSocketRequest(r.Context(), r, "contest-1", service, nil)
		if err != nil || userID != "user-1" {
			t.Fatalf("redacted request authentication = (%q, %v)", userID, err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/ws/trade?contest_id=contest-1&ticket="+issue.Ticket, nil)
	req.AddCookie(&http.Cookie{Name: wsTicketBindingCookieName, Value: issue.Binding, Path: wsTicketBindingCookiePath})
	rec := httptest.NewRecorder()
	redactWSTicketForTelemetry(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
