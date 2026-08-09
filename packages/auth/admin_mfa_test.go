package auth

import (
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

const (
	adminMFATestActor               = "actor"
	adminMFATestBinding             = "binding"
	adminMFATestPermissionUsersEdit = "users.edit"
	adminMFATestStageVerify         = "verify"
)

func strongHex(seed byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	return hex.EncodeToString(raw)
}

func TestValidateAdminMFAConfig(t *testing.T) {
	t.Parallel()
	validKey, validPepper := strongHex(1), strongHex(65)
	tests := []struct {
		name, env, key, pepper, issuer string
		ttl                            time.Duration
		valid                          bool
	}{
		{"valid production", adminMFAProductionEnv, validKey, validPepper, AdminMFAIssuer, 5 * time.Minute, true},
		{"missing key", adminMFAProductionEnv, "", validPepper, AdminMFAIssuer, time.Minute, false},
		{"weak repeated key", adminMFAProductionEnv, strings.Repeat("aa", 32), validPepper, AdminMFAIssuer, time.Minute, false},
		{"equal domains", adminMFAProductionEnv, validKey, validKey, AdminMFAIssuer, time.Minute, false},
		{adminMFAPlaceholderWord, adminMFAProductionEnv, adminMFAPlaceholderMarker, validPepper, AdminMFAIssuer, time.Minute, false},
		{"wrong issuer", adminMFAProductionEnv, validKey, validPepper, "Other", time.Minute, false},
		{"long ttl", "development", validKey, validPepper, AdminMFAIssuer, 6 * time.Minute, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateAdminMFAConfig(tc.env, tc.key, tc.pepper, tc.issuer, tc.ttl)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestAdminMFASecretEncryptionIsStrictAndAuthenticated(t *testing.T) {
	t.Parallel()
	key, _ := hex.DecodeString(strongHex(1))
	secret, err := GenerateAdminTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := EncryptAdminTOTPSecret(secret, key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ciphertext, AdminMFACiphertextPrefix) || strings.Contains(ciphertext, secret) {
		t.Fatal("ciphertext contract violated")
	}
	got, err := DecryptAdminTOTPSecret(ciphertext, key)
	if err != nil || got != secret {
		t.Fatalf("decrypt=%q err=%v", got, err)
	}
	if _, err := DecryptAdminTOTPSecret(secret, key); err == nil {
		t.Fatal("plaintext fallback accepted")
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, AdminMFACiphertextPrefix))
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-1] ^= 0x01
	tampered := AdminMFACiphertextPrefix + base64.RawStdEncoding.EncodeToString(payload)
	if _, err := DecryptAdminTOTPSecret(tampered, key); err == nil {
		t.Fatal("tampering accepted")
	}
}

func TestAdminTOTPCounterAndProvisioningContract(t *testing.T) {
	t.Parallel()
	secretBytes := []byte("12345678901234567890")
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secretBytes)
	// RFC 6238 SHA-1 vector at Unix time 59 is 94287082 for eight digits;
	// the approved six-digit contract takes the same dynamic value modulo 10^6.
	now := time.Unix(59, 0)
	wantCounter := now.Unix() / 30
	code := generateTOTPCode(secretBytes, wantCounter)
	if code != "287082" {
		t.Fatalf("RFC-compatible six-digit vector=%s", code)
	}
	counter, ok := MatchAdminTOTPCounter(secret, code, now)
	if !ok || counter != wantCounter {
		t.Fatalf("counter=%d ok=%v", counter, ok)
	}
	if _, ok := MatchAdminTOTPCounter(secret, "123", now); ok {
		t.Fatal("short code accepted")
	}
	adjacent := generateTOTPCode(secretBytes, wantCounter+1)
	if counter, ok := MatchAdminTOTPCounter(secret, adjacent, now); !ok || counter != wantCounter+1 {
		t.Fatalf("adjacent clock window counter=%d ok=%v", counter, ok)
	}
	outOfWindow := generateTOTPCode(secretBytes, wantCounter+2)
	if _, ok := MatchAdminTOTPCounter(secret, outOfWindow, now); ok {
		t.Fatal("out-of-window code accepted")
	}
	uri := AdminMFAProvisioningURI(AdminMFAIssuer, "root@example.test", secret)
	for _, want := range []string{"otpauth://totp/", "algorithm=SHA1", "digits=6", "period=30"} {
		if !strings.Contains(uri, want) {
			t.Fatalf("URI missing %q", want)
		}
	}
}

func TestAdminMFARecoveryCodesAreRandomAndKeyed(t *testing.T) {
	t.Parallel()
	codes, err := GenerateAdminMFARecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != AdminMFARecoveryCodeCount {
		t.Fatalf("count=%d", len(codes))
	}
	seen := map[string]bool{}
	pepper, _ := hex.DecodeString(strongHex(65))
	for _, code := range codes {
		if seen[code] {
			t.Fatal("duplicate recovery code")
		}
		seen[code] = true
		digest, err := AdminMFARecoveryDigest(code, pepper)
		if err != nil || len(digest) != sha256Size {
			t.Fatalf("digest length=%d err=%v", len(digest), err)
		}
		if strings.Contains(string(digest), code) {
			t.Fatal("plaintext leaked into digest")
		}
	}
}

const sha256Size = 32

func TestRedisAdminMFAChallengeSingleUseAndNonRecoverableKey(t *testing.T) {
	t.Parallel()
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := NewRedisAdminMFAChallengeStore(client, AdminMFAChallengePrefix)
	now := time.Now().UTC()
	token, err := store.Issue(context.Background(), AdminMFAChallenge{UserID: adminMFATestActor, Stage: adminMFATestStageVerify, ClientBinding: adminMFATestBinding, IssuedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range mini.Keys() {
		if strings.Contains(key, token) {
			t.Fatal("raw challenge persisted")
		}
	}
	challenge, err := store.Get(context.Background(), token)
	if err != nil || challenge.UserID != adminMFATestActor {
		t.Fatalf("get=%v err=%v", challenge, err)
	}
	if _, err := store.Consume(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Consume(context.Background(), token); err != ErrAdminMFAReplayed {
		t.Fatalf("replay err=%v", err)
	}
}

func TestRedisAdminMFAChallengeConcurrentConsumeAllowsOne(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := NewRedisAdminMFAChallengeStore(client, AdminMFAChallengePrefix)
	now := time.Now().UTC()
	token, err := store.Issue(context.Background(), AdminMFAChallenge{UserID: adminMFATestActor, Stage: adminMFATestStageVerify, ClientBinding: adminMFATestBinding, IssuedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	var succeeded atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Consume(context.Background(), token); err == nil {
				succeeded.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := succeeded.Load(); got != 1 {
		t.Fatalf("successful consumers=%d", got)
	}
}

func TestRedisAdminMFAChallengeExpiryAndUserRevocation(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	store := NewRedisAdminMFAChallengeStore(client, AdminMFAChallengePrefix)
	now := time.Now().UTC()
	expiring, err := store.Issue(context.Background(), AdminMFAChallenge{UserID: "actor-a", Stage: adminMFATestStageVerify, ClientBinding: adminMFATestBinding, IssuedAt: now, ExpiresAt: now.Add(20 * time.Millisecond)})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := store.Get(context.Background(), expiring); err != ErrAdminMFAExpired {
		t.Fatalf("expired challenge err=%v", err)
	}

	actorA, err := store.Issue(context.Background(), AdminMFAChallenge{UserID: "actor-a", Stage: adminMFATestStageVerify, ClientBinding: adminMFATestBinding, IssuedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	actorB, err := store.Issue(context.Background(), AdminMFAChallenge{UserID: "actor-b", Stage: adminMFATestStageVerify, ClientBinding: adminMFATestBinding, IssuedAt: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeUser(context.Background(), "actor-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), actorA); err != ErrAdminMFAInvalid {
		t.Fatalf("revoked actor challenge err=%v", err)
	}
	if _, err := store.Get(context.Background(), actorB); err != nil {
		t.Fatalf("other actor challenge was revoked: %v", err)
	}
}

func TestSuperAdminMFAAssurancePersistsAcrossRefreshAndMiddleware(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	cfg := DefaultConfig()
	cfg.Context = ContextAdmin
	cfg.JWTSecret = "access-secret-for-tests-only-32-bytes"
	cfg.JWTRefreshSecret = "refresh-secret-for-tests-only-32bytes"
	cfg.JWTIssuer = "tragge-admin-auth"
	cfg.JWTAudience = AudienceAdmin
	cfg.Redis = client
	cfg.SessionPrefix = "session:admin:"
	a := New(cfg)
	pair, sessionID, err := a.LoginWithPermissionsAndMFA(context.Background(), "admin-1", []string{RoleSuperAdmin}, []string{adminMFATestPermissionUsersEdit}, MFAAssuranceSuperAdminTOTPV1, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := a.Refresh(context.Background(), sessionID, pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := a.Token.ValidateAccessToken(refreshed.AccessToken)
	if err != nil || claims.MFAAssurance != MFAAssuranceSuperAdminTOTPV1 || len(claims.Permissions) != 1 {
		t.Fatalf("claims=%+v err=%v", claims, err)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/admin", nil)
	request.Header.Set("Authorization", "Bearer "+refreshed.AccessToken)
	recorder := httptest.NewRecorder()
	a.Middleware.RequireAuth(a.Middleware.RequireAdminAccess(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("MFA request status=%d", recorder.Code)
	}

	legacy, _, err := a.LoginWithPermissions(context.Background(), "admin-2", []string{RoleSuperAdmin}, []string{adminMFATestPermissionUsersEdit}, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	legacyRequest := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/admin", nil)
	legacyRequest.Header.Set("Authorization", "Bearer "+legacy.AccessToken)
	legacyRecorder := httptest.NewRecorder()
	a.Middleware.RequireAuth(a.Middleware.RequireAdminAccess(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))).ServeHTTP(legacyRecorder, legacyRequest)
	if legacyRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("legacy super-admin status=%d", legacyRecorder.Code)
	}
}
