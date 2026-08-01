package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	AdminReauthenticationPrefix = "reauth:admin:"
	MaxReauthenticationTTL      = 5 * time.Minute
	reauthenticationGrace       = time.Minute
)

var (
	ErrReauthenticationUnavailable     = errors.New("auth: reauthentication storage unavailable")
	ErrReauthenticationInvalid         = errors.New("auth: invalid reauthentication grant")
	ErrReauthenticationExpired         = errors.New("auth: reauthentication grant expired")
	ErrReauthenticationReplayed        = errors.New("auth: reauthentication grant replayed")
	ErrReauthenticationBinding         = errors.New("auth: reauthentication grant binding mismatch")
	ErrReauthenticationContextBinding  = fmt.Errorf("%w: context", ErrReauthenticationBinding)
	ErrReauthenticationActorBinding    = fmt.Errorf("%w: actor", ErrReauthenticationBinding)
	ErrReauthenticationSessionBinding  = fmt.Errorf("%w: session", ErrReauthenticationBinding)
	ErrReauthenticationActionBinding   = fmt.Errorf("%w: action", ErrReauthenticationBinding)
	ErrReauthenticationResourceBinding = fmt.Errorf("%w: resource", ErrReauthenticationBinding)
	ErrReauthenticationStateBinding    = fmt.Errorf("%w: security state", ErrReauthenticationBinding)
)

type ReauthenticationGrant struct {
	Context             AuthContext `json:"context"`
	ActorID             string      `json:"actor_id"`
	SessionDigest       string      `json:"session_digest"`
	Action              string      `json:"action"`
	ResourceID          string      `json:"resource_id"`
	SecurityFingerprint string      `json:"security_fingerprint"`
	IssuedAt            time.Time   `json:"issued_at"`
	ExpiresAt           time.Time   `json:"expires_at"`
}

type ReauthenticationExpectation struct {
	Context             AuthContext
	ActorID             string
	SessionID           string
	Action              string
	ResourceID          string
	SecurityFingerprint string
}

type ReauthenticationGrantStore interface {
	Issue(context.Context, ReauthenticationGrant) (string, error)
	Consume(context.Context, string) (*ReauthenticationGrant, error)
	RevokeActor(context.Context, string) error
}

type ReauthenticationService struct {
	store ReauthenticationGrantStore
	ttl   time.Duration
	now   func() time.Time
}

func NewReauthenticationService(store ReauthenticationGrantStore, ttl time.Duration) (*ReauthenticationService, error) {
	if store == nil || ttl <= 0 || ttl > MaxReauthenticationTTL {
		return nil, ErrReauthenticationInvalid
	}
	return &ReauthenticationService{store: store, ttl: ttl, now: time.Now}, nil
}

func (s *ReauthenticationService) Issue(
	ctx context.Context,
	expectation ReauthenticationExpectation,
) (string, time.Time, error) {
	if err := validateReauthenticationExpectation(expectation); err != nil {
		return "", time.Time{}, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.ttl)
	token, err := s.store.Issue(ctx, ReauthenticationGrant{
		Context:             expectation.Context,
		ActorID:             expectation.ActorID,
		SessionDigest:       ReauthenticationBindingDigest(expectation.SessionID),
		Action:              expectation.Action,
		ResourceID:          expectation.ResourceID,
		SecurityFingerprint: expectation.SecurityFingerprint,
		IssuedAt:            now,
		ExpiresAt:           expiresAt,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: %v", ErrReauthenticationUnavailable, err)
	}
	return token, expiresAt, nil
}

func (s *ReauthenticationService) Consume(
	ctx context.Context,
	token string,
	expectation ReauthenticationExpectation,
) error {
	if strings.TrimSpace(token) == "" {
		return ErrReauthenticationInvalid
	}
	if err := validateReauthenticationExpectation(expectation); err != nil {
		return err
	}
	grant, err := s.store.Consume(ctx, token)
	if err != nil {
		return err
	}
	if grant == nil {
		return ErrReauthenticationInvalid
	}
	if !s.now().UTC().Before(grant.ExpiresAt) {
		return ErrReauthenticationExpired
	}
	if grant.Context != expectation.Context {
		return ErrReauthenticationContextBinding
	}
	if !constantTimeStringEqual(grant.ActorID, expectation.ActorID) {
		return ErrReauthenticationActorBinding
	}
	if !constantTimeStringEqual(grant.SessionDigest, ReauthenticationBindingDigest(expectation.SessionID)) {
		return ErrReauthenticationSessionBinding
	}
	if !constantTimeStringEqual(grant.Action, expectation.Action) {
		return ErrReauthenticationActionBinding
	}
	if !constantTimeStringEqual(grant.ResourceID, expectation.ResourceID) {
		return ErrReauthenticationResourceBinding
	}
	if !constantTimeStringEqual(grant.SecurityFingerprint, expectation.SecurityFingerprint) {
		return ErrReauthenticationStateBinding
	}
	return nil
}

func (s *ReauthenticationService) RevokeActor(ctx context.Context, actorID string) error {
	if strings.TrimSpace(actorID) == "" {
		return ErrReauthenticationInvalid
	}
	if err := s.store.RevokeActor(ctx, actorID); err != nil {
		return fmt.Errorf("%w: %v", ErrReauthenticationUnavailable, err)
	}
	return nil
}

func validateReauthenticationExpectation(expectation ReauthenticationExpectation) error {
	if expectation.Context != ContextAdmin ||
		strings.TrimSpace(expectation.ActorID) == "" ||
		strings.TrimSpace(expectation.SessionID) == "" ||
		strings.TrimSpace(expectation.Action) == "" ||
		strings.TrimSpace(expectation.ResourceID) == "" ||
		strings.TrimSpace(expectation.SecurityFingerprint) == "" {
		return ErrReauthenticationInvalid
	}
	return nil
}

func ReauthenticationBindingDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func ReauthenticationSecurityFingerprint(passwordHash string, roles, permissions []string) string {
	roles = append([]string(nil), roles...)
	permissions = append([]string(nil), permissions...)
	sort.Strings(roles)
	sort.Strings(permissions)
	value := strings.Join([]string{
		passwordHash,
		strings.Join(roles, "\x1f"),
		strings.Join(permissions, "\x1f"),
	}, "\x1e")
	return ReauthenticationBindingDigest(value)
}

func constantTimeStringEqual(left, right string) bool {
	leftDigest := sha256.Sum256([]byte(left))
	rightDigest := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftDigest[:], rightDigest[:]) == 1
}

type RedisReauthenticationGrantStore struct {
	client redis.UniversalClient
	prefix string
}

func NewRedisReauthenticationGrantStore(client redis.UniversalClient, prefix string) *RedisReauthenticationGrantStore {
	if prefix == "" {
		prefix = AdminReauthenticationPrefix
	}
	return &RedisReauthenticationGrantStore{client: client, prefix: prefix}
}

func (s *RedisReauthenticationGrantStore) Issue(
	ctx context.Context,
	grant ReauthenticationGrant,
) (string, error) {
	if s == nil || s.client == nil {
		return "", ErrReauthenticationUnavailable
	}
	payload, err := json.Marshal(grant)
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 3; attempt++ {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		token := base64.RawURLEncoding.EncodeToString(raw)
		digest := ReauthenticationBindingDigest(token)
		ttl := time.Until(grant.ExpiresAt) + reauthenticationGrace
		if ttl <= 0 {
			return "", ErrReauthenticationExpired
		}
		created, err := s.client.SetNX(ctx, s.grantKey(digest), payload, ttl).Result()
		if err != nil {
			return "", err
		}
		if !created {
			continue
		}
		actorKey := s.actorKey(grant.ActorID)
		if err := s.client.SAdd(ctx, actorKey, digest).Err(); err != nil {
			_ = s.client.Del(ctx, s.grantKey(digest)).Err()
			return "", err
		}
		if err := s.client.Expire(ctx, actorKey, MaxReauthenticationTTL+reauthenticationGrace).Err(); err != nil {
			_ = s.client.Del(ctx, s.grantKey(digest)).Err()
			_ = s.client.SRem(ctx, actorKey, digest).Err()
			return "", err
		}
		return token, nil
	}
	return "", ErrReauthenticationUnavailable
}

var consumeReauthenticationScript = redis.NewScript(`
local payload = redis.call("GET", KEYS[1])
if not payload then
  if redis.call("EXISTS", KEYS[2]) == 1 then
    return "__REPLAY__"
  end
  return "__MISSING__"
end
redis.call("DEL", KEYS[1])
redis.call("SET", KEYS[2], "1", "PX", ARGV[1])
return payload
`)

func (s *RedisReauthenticationGrantStore) Consume(
	ctx context.Context,
	token string,
) (*ReauthenticationGrant, error) {
	if s == nil || s.client == nil || strings.TrimSpace(token) == "" {
		return nil, ErrReauthenticationInvalid
	}
	digest := ReauthenticationBindingDigest(token)
	result, err := consumeReauthenticationScript.Run(
		ctx,
		s.client,
		[]string{s.grantKey(digest), s.spentKey(digest)},
		(MaxReauthenticationTTL + reauthenticationGrace).Milliseconds(),
	).Text()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReauthenticationUnavailable, err)
	}
	switch result {
	case "__REPLAY__":
		return nil, ErrReauthenticationReplayed
	case "__MISSING__":
		return nil, ErrReauthenticationInvalid
	}
	var grant ReauthenticationGrant
	if err := json.Unmarshal([]byte(result), &grant); err != nil {
		return nil, ErrReauthenticationInvalid
	}
	_ = s.client.SRem(ctx, s.actorKey(grant.ActorID), digest).Err()
	return &grant, nil
}

func (s *RedisReauthenticationGrantStore) RevokeActor(ctx context.Context, actorID string) error {
	if s == nil || s.client == nil {
		return ErrReauthenticationUnavailable
	}
	actorKey := s.actorKey(actorID)
	digests, err := s.client.SMembers(ctx, actorKey).Result()
	if err != nil {
		return err
	}
	pipe := s.client.Pipeline()
	for _, digest := range digests {
		pipe.Del(ctx, s.grantKey(digest))
	}
	pipe.Del(ctx, actorKey)
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	return nil
}

func (s *RedisReauthenticationGrantStore) grantKey(digest string) string {
	return s.prefix + "{" + digest + "}:grant"
}

func (s *RedisReauthenticationGrantStore) spentKey(digest string) string {
	return s.prefix + "{" + digest + "}:spent"
}

func (s *RedisReauthenticationGrantStore) actorKey(actorID string) string {
	return s.prefix + "actor:" + ReauthenticationBindingDigest(actorID)
}
