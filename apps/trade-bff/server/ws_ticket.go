package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	redis "github.com/redis/go-redis/v9"
)

const (
	wsTicketPurpose           = "trade_websocket_handshake"
	wsTicketBindingCookieName = "tragge_ws_ticket_bind_user"
	wsTicketBindingCookiePath = "/ws/trade"
	defaultWSTicketTTL        = 10 * time.Second
)

var (
	errWSTicketInvalid     = errors.New("websocket ticket is invalid or expired")
	errWSTicketUnavailable = errors.New("websocket ticket service is unavailable")
)

type wsTicketStore interface {
	Put(ctx context.Context, key, value string, ttl time.Duration) error
	Take(ctx context.Context, key string) (string, error)
}

type redisWSTicketStore struct {
	client redis.UniversalClient
}

func (s redisWSTicketStore) Put(ctx context.Context, key, value string, ttl time.Duration) error {
	if s.client == nil {
		return errWSTicketUnavailable
	}
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s redisWSTicketStore) Take(ctx context.Context, key string) (string, error) {
	if s.client == nil {
		return "", errWSTicketUnavailable
	}
	value, err := s.client.GetDel(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", errWSTicketInvalid
	}
	return value, err
}

type wsSessionReader interface {
	Get(ctx context.Context, sessionID string) (*auth.Session, error)
}

type wsTicketRecord struct {
	UserID      string           `json:"user_id"`
	SessionID   string           `json:"session_id"`
	ContestID   string           `json:"contest_id"`
	Context     auth.AuthContext `json:"auth_context"`
	Purpose     string           `json:"purpose"`
	BindingHash string           `json:"binding_hash"`
	ExpiresAtMS int64            `json:"expires_at_ms"`
}

type wsTicketIssue struct {
	Ticket    string
	Binding   string
	ExpiresIn int
}

type wsTicketService struct {
	store    wsTicketStore
	sessions wsSessionReader
	context  auth.AuthContext
	ttl      time.Duration
	now      func() time.Time
	random   io.Reader
}

func newWSTicketService(store wsTicketStore, sessions wsSessionReader, authContext auth.AuthContext) *wsTicketService {
	return &wsTicketService{
		store:    store,
		sessions: sessions,
		context:  authContext,
		ttl:      defaultWSTicketTTL,
		now:      time.Now,
		random:   rand.Reader,
	}
}

func (s *wsTicketService) Issue(ctx context.Context, userID, sessionID, contestID string) (*wsTicketIssue, error) {
	if s == nil || s.store == nil || s.sessions == nil || s.context != auth.ContextUser {
		return nil, errWSTicketUnavailable
	}
	if userID == "" || sessionID == "" || contestID == "" {
		return nil, errWSTicketInvalid
	}

	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil || session == nil || session.UserID != userID {
		return nil, errWSTicketInvalid
	}

	ticket, err := randomHex(s.random, 32)
	if err != nil {
		return nil, fmt.Errorf("generate websocket ticket: %w", err)
	}
	binding, err := randomHex(s.random, 32)
	if err != nil {
		return nil, fmt.Errorf("generate websocket ticket binding: %w", err)
	}

	now := s.now()
	record := wsTicketRecord{
		UserID:      userID,
		SessionID:   sessionID,
		ContestID:   contestID,
		Context:     s.context,
		Purpose:     wsTicketPurpose,
		BindingHash: credentialDigest(binding),
		ExpiresAtMS: now.Add(s.ttl).UnixMilli(),
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("encode websocket ticket: %w", err)
	}
	if err := s.store.Put(ctx, s.ticketKey(ticket), string(encoded), s.ttl); err != nil {
		return nil, fmt.Errorf("store websocket ticket: %w", err)
	}

	return &wsTicketIssue{
		Ticket:    ticket,
		Binding:   binding,
		ExpiresIn: int(s.ttl / time.Second),
	}, nil
}

func (s *wsTicketService) Consume(ctx context.Context, ticket, binding, contestID string) (string, error) {
	if s == nil || s.store == nil || s.sessions == nil || s.context != auth.ContextUser {
		return "", errWSTicketUnavailable
	}
	if !isOpaqueCredential(ticket) || !isOpaqueCredential(binding) || contestID == "" {
		return "", errWSTicketInvalid
	}

	encoded, err := s.store.Take(ctx, s.ticketKey(ticket))
	if err != nil {
		return "", errWSTicketInvalid
	}
	var record wsTicketRecord
	if err := json.Unmarshal([]byte(encoded), &record); err != nil {
		return "", errWSTicketInvalid
	}

	bindingDigest := credentialDigest(binding)
	bindingMatches := subtle.ConstantTimeCompare([]byte(record.BindingHash), []byte(bindingDigest)) == 1
	if !bindingMatches || record.Context != s.context || record.Purpose != wsTicketPurpose ||
		record.ContestID != contestID || record.UserID == "" || record.SessionID == "" ||
		s.now().UnixMilli() >= record.ExpiresAtMS {
		return "", errWSTicketInvalid
	}

	session, err := s.sessions.Get(ctx, record.SessionID)
	if err != nil || session == nil || session.UserID != record.UserID {
		return "", errWSTicketInvalid
	}
	return record.UserID, nil
}

func (s *wsTicketService) ticketKey(ticket string) string {
	return "ws_ticket:" + string(s.context) + ":" + credentialDigest(ticket)
}

func randomHex(source io.Reader, byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := io.ReadFull(source, buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func credentialDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func isOpaqueCredential(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

type wsAccessTokenValidator interface {
	ValidateAccessToken(tokenString string) (*auth.Claims, error)
}

type wsAccessAuthenticator struct {
	tokens   wsAccessTokenValidator
	sessions wsSessionReader
}

func (a *wsAccessAuthenticator) Authenticate(ctx context.Context, token string) (string, error) {
	if a == nil || a.tokens == nil || a.sessions == nil || token == "" {
		return "", errWSTicketInvalid
	}
	claims, err := a.tokens.ValidateAccessToken(token)
	if err != nil || claims == nil || claims.AuthContext != auth.ContextUser || claims.SessionID == "" {
		return "", errWSTicketInvalid
	}
	session, err := a.sessions.Get(ctx, claims.SessionID)
	if err != nil || session == nil || session.UserID != claims.UserID {
		return "", errWSTicketInvalid
	}
	return claims.UserID, nil
}

type wsTicketRequestContextKey struct{}

// redactWSTicketForTelemetry removes the bounded ticket from the request URL
// before logging/tracing/panic middleware. The raw value remains in an
// unexported request context only long enough for the handshake validator.
func redactWSTicketForTelemetry(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ticket := r.URL.Query().Get("ticket")
		if ticket == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), wsTicketRequestContextKey{}, ticket)
		clone := r.Clone(ctx)
		urlCopy := *r.URL
		clone.URL = &urlCopy
		query := clone.URL.Query()
		query.Set("ticket", auth.RedactedCredentialValue)
		clone.URL.RawQuery = query.Encode()
		next.ServeHTTP(w, clone)
	})
}

func websocketTicketFromRequest(r *http.Request) string {
	if ticket, ok := r.Context().Value(wsTicketRequestContextKey{}).(string); ok {
		return ticket
	}
	ticket := r.URL.Query().Get("ticket")
	if ticket == auth.RedactedCredentialValue {
		return ""
	}
	return ticket
}
func authenticateWebSocketRequest(ctx context.Context, r *http.Request, contestID string, tickets *wsTicketService, access *wsAccessAuthenticator) (string, error) {
	if auth.HasProhibitedCredentialQuery(r) {
		return "", errWSTicketInvalid
	}
	if ticket := websocketTicketFromRequest(r); ticket != "" {
		bindingCookie, err := r.Cookie(wsTicketBindingCookieName)
		if err != nil || bindingCookie.Value == "" {
			return "", errWSTicketInvalid
		}
		return tickets.Consume(ctx, ticket, bindingCookie.Value, contestID)
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errWSTicketInvalid
	}
	return access.Authenticate(ctx, strings.TrimSpace(parts[1]))
}
func setWSTicketBindingCookie(w http.ResponseWriter, binding string, secure bool, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     wsTicketBindingCookieName,
		Value:    binding,
		Path:     wsTicketBindingCookiePath,
		MaxAge:   int(ttl / time.Second),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearWSTicketBindingCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     wsTicketBindingCookieName,
		Value:    "",
		Path:     wsTicketBindingCookiePath,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0).UTC(),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func secureWSTicketCookie(environment string, r *http.Request) bool {
	normalized := strings.ToLower(strings.TrimSpace(environment))
	if normalized == "production" || normalized == "prod" {
		return true
	}
	if r != nil && r.TLS != nil {
		return true
	}
	if r != nil {
		forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
		return strings.EqualFold(forwardedProto, "https")
	}
	return false
}
