package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	AdminMFACiphertextPrefix  = "enc:admin-mfa:v1:"
	AdminMFAChallengePrefix   = "mfa:admin:v1:"
	AdminMFAIssuer            = "Tragge Admin"
	AdminMFAMaxChallengeTTL   = 5 * time.Minute
	AdminMFARecoveryCodeCount = 10
	adminMFAProductionEnv     = "production"
	adminMFAPlaceholderMarker = "changeme"
	adminMFAPlaceholderWord   = "placeholder"
)

var (
	ErrAdminMFAInvalid     = errors.New("auth: invalid admin MFA credential")
	ErrAdminMFAUnavailable = errors.New("auth: admin MFA storage unavailable")
	ErrAdminMFAExpired     = errors.New("auth: admin MFA challenge expired")
	ErrAdminMFAReplayed    = errors.New("auth: admin MFA challenge replayed")
)

// AdminMFAConfig is deliberately separate from legacy/shared TOTP settings.
type AdminMFAConfig struct {
	EncryptionKey  []byte
	RecoveryPepper []byte
	Issuer         string
	ChallengeTTL   time.Duration
}

func ValidateAdminMFAConfig(environment, encryptionKeyHex, recoveryPepperHex, issuer string, ttl time.Duration) (AdminMFAConfig, error) {
	key, err := parseStrongAdminMFASecret("ADMIN_MFA_ENCRYPTION_KEY", encryptionKeyHex)
	if err != nil {
		return AdminMFAConfig{}, err
	}
	pepper, err := parseStrongAdminMFASecret("ADMIN_MFA_RECOVERY_PEPPER", recoveryPepperHex)
	if err != nil {
		return AdminMFAConfig{}, err
	}
	if hmac.Equal(key, pepper) {
		return AdminMFAConfig{}, errors.New("admin MFA encryption key and recovery pepper must differ")
	}
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = AdminMFAIssuer
	}
	if ttl <= 0 || ttl > AdminMFAMaxChallengeTTL {
		return AdminMFAConfig{}, errors.New("admin MFA challenge TTL must be between one second and five minutes")
	}
	if isAdminMFAProductionEnvironment(environment) && issuer != AdminMFAIssuer {
		return AdminMFAConfig{}, errors.New("production Admin MFA issuer must use the approved value")
	}
	return AdminMFAConfig{EncryptionKey: key, RecoveryPepper: pepper, Issuer: issuer, ChallengeTTL: ttl}, nil
}

func parseStrongAdminMFASecret(name, encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	for _, marker := range []string{"example", adminMFAPlaceholderWord, adminMFAPlaceholderMarker, "default", "test-secret", "local-secret"} {
		if strings.Contains(strings.ToLower(encoded), marker) {
			return nil, fmt.Errorf("%s must not use a placeholder", name)
		}
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("%s must be 32 random bytes encoded as 64 hexadecimal characters", name)
	}
	allSame := true
	for _, value := range decoded[1:] {
		if value != decoded[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return nil, fmt.Errorf("%s is not unpredictable", name)
	}
	return decoded, nil
}

func isAdminMFAProductionEnvironment(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case adminMFAProductionEnv, "prod":
		return true
	default:
		return false
	}
}

func GenerateAdminTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func EncryptAdminTOTPSecret(secret string, key []byte) (string, error) {
	if len(key) != 32 || strings.TrimSpace(secret) == "" {
		return "", ErrAdminMFAInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrAdminMFAInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrAdminMFAInvalid
	}
	nonce := make([]byte, gcm.NonceSize(), gcm.NonceSize()+len(secret)+gcm.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(secret), []byte(AdminMFACiphertextPrefix))
	return AdminMFACiphertextPrefix + base64.RawStdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func DecryptAdminTOTPSecret(stored string, key []byte) (string, error) {
	if len(key) != 32 || !strings.HasPrefix(stored, AdminMFACiphertextPrefix) {
		return "", ErrAdminMFAInvalid
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, AdminMFACiphertextPrefix))
	if err != nil {
		return "", ErrAdminMFAInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrAdminMFAInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(payload) < gcm.NonceSize() {
		return "", ErrAdminMFAInvalid
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte(AdminMFACiphertextPrefix))
	if err != nil {
		return "", ErrAdminMFAInvalid
	}
	return string(plaintext), nil
}

// MatchAdminTOTPCounter returns the exact accepted counter so the database can
// reject reuse atomically. It intentionally keeps the RFC 6238 +/- one-step window.
func MatchAdminTOTPCounter(secret, code string, now time.Time) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	secretBytes, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return 0, false
	}
	current := now.Unix() / 30
	for _, offset := range []int64{0, -1, 1} {
		counter := current + offset
		if hmac.Equal([]byte(generateTOTPCode(secretBytes, counter)), []byte(code)) {
			return counter, true
		}
	}
	return 0, false
}

func AdminMFAProvisioningURI(issuer, account, secret string) string {
	label := url.PathEscape(strings.TrimSpace(issuer) + ":" + strings.TrimSpace(account))
	query := url.Values{"secret": {secret}, "issuer": {strings.TrimSpace(issuer)}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + query.Encode()
}

func GenerateAdminMFARecoveryCodes() ([]string, error) {
	codes := make([]string, 0, AdminMFARecoveryCodeCount)
	for len(codes) < AdminMFARecoveryCodeCount {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		value := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
		codes = append(codes, value[:8]+"-"+value[8:])
	}
	return codes, nil
}

func AdminMFARecoveryDigest(code string, pepper []byte) ([]byte, error) {
	if len(pepper) != 32 || strings.TrimSpace(code) == "" {
		return nil, ErrAdminMFAInvalid
	}
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	mac := hmac.New(sha256.New, pepper)
	_, _ = mac.Write([]byte("tragge/admin-mfa/recovery/v1\x00" + normalized))
	return mac.Sum(nil), nil
}

type AdminMFAChallenge struct {
	UserID              string    `json:"user_id"`
	Email               string    `json:"email"`
	Roles               []string  `json:"roles"`
	Permissions         []string  `json:"permissions"`
	SecurityFingerprint string    `json:"security_fingerprint"`
	Stage               string    `json:"stage"`
	SecretCiphertext    string    `json:"secret_ciphertext,omitempty"`
	ClientBinding       string    `json:"client_binding"`
	IssuedAt            time.Time `json:"issued_at"`
	ExpiresAt           time.Time `json:"expires_at"`
}

type RedisAdminMFAChallengeStore struct {
	client redis.UniversalClient
	prefix string
}

func NewRedisAdminMFAChallengeStore(client redis.UniversalClient, prefix string) *RedisAdminMFAChallengeStore {
	if prefix == "" {
		prefix = AdminMFAChallengePrefix
	}
	return &RedisAdminMFAChallengeStore{client: client, prefix: prefix}
}

func (s *RedisAdminMFAChallengeStore) Issue(ctx context.Context, challenge AdminMFAChallenge) (string, error) {
	if s == nil || s.client == nil || challenge.UserID == "" || challenge.Stage == "" || !challenge.ExpiresAt.After(challenge.IssuedAt) {
		return "", ErrAdminMFAInvalid
	}
	payload, err := json.Marshal(challenge)
	if err != nil {
		return "", ErrAdminMFAInvalid
	}
	for attempt := 0; attempt < 3; attempt++ {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		token := base64.RawURLEncoding.EncodeToString(raw)
		digest := ReauthenticationBindingDigest(token)
		created, err := s.client.SetNX(ctx, s.key(digest), payload, time.Until(challenge.ExpiresAt)+time.Minute).Result()
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrAdminMFAUnavailable, err)
		}
		if created {
			return token, nil
		}
	}
	return "", ErrAdminMFAUnavailable
}

func (s *RedisAdminMFAChallengeStore) Get(ctx context.Context, token string) (*AdminMFAChallenge, error) {
	if s == nil || s.client == nil || strings.TrimSpace(token) == "" {
		return nil, ErrAdminMFAInvalid
	}
	payload, err := s.client.Get(ctx, s.key(ReauthenticationBindingDigest(token))).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrAdminMFAInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAdminMFAUnavailable, err)
	}
	var challenge AdminMFAChallenge
	if json.Unmarshal(payload, &challenge) != nil {
		return nil, ErrAdminMFAInvalid
	}
	if !time.Now().UTC().Before(challenge.ExpiresAt) {
		return nil, ErrAdminMFAExpired
	}
	return &challenge, nil
}

var consumeAdminMFAChallengeScript = redis.NewScript(`
local payload = redis.call("GET", KEYS[1])
if not payload then
  if redis.call("EXISTS", KEYS[2]) == 1 then return "__REPLAY__" end
  return "__MISSING__"
end
redis.call("DEL", KEYS[1])
redis.call("SET", KEYS[2], "1", "PX", ARGV[1])
return payload
`)

func (s *RedisAdminMFAChallengeStore) Consume(ctx context.Context, token string) (*AdminMFAChallenge, error) {
	if s == nil || s.client == nil || strings.TrimSpace(token) == "" {
		return nil, ErrAdminMFAInvalid
	}
	digest := ReauthenticationBindingDigest(token)
	result, err := consumeAdminMFAChallengeScript.Run(ctx, s.client, []string{s.key(digest), s.spentKey(digest)}, (AdminMFAMaxChallengeTTL + time.Minute).Milliseconds()).Text()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAdminMFAUnavailable, err)
	}
	if result == "__REPLAY__" {
		return nil, ErrAdminMFAReplayed
	}
	if result == "__MISSING__" {
		return nil, ErrAdminMFAInvalid
	}
	var challenge AdminMFAChallenge
	if json.Unmarshal([]byte(result), &challenge) != nil {
		return nil, ErrAdminMFAInvalid
	}
	if !time.Now().UTC().Before(challenge.ExpiresAt) {
		return nil, ErrAdminMFAExpired
	}
	return &challenge, nil
}

func (s *RedisAdminMFAChallengeStore) RevokeUser(ctx context.Context, userID string) error {
	if s == nil || s.client == nil || strings.TrimSpace(userID) == "" {
		return ErrAdminMFAInvalid
	}
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, s.prefix+"*:challenge", 100).Result()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAdminMFAUnavailable, err)
		}
		for _, key := range keys {
			payload, err := s.client.Get(ctx, key).Bytes()
			if err != nil {
				continue
			}
			var challenge AdminMFAChallenge
			if json.Unmarshal(payload, &challenge) == nil && challenge.UserID == userID {
				if err := s.client.Del(ctx, key).Err(); err != nil {
					return fmt.Errorf("%w: %v", ErrAdminMFAUnavailable, err)
				}
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

func (s *RedisAdminMFAChallengeStore) key(digest string) string {
	return s.prefix + digest + ":challenge"
}
func (s *RedisAdminMFAChallengeStore) spentKey(digest string) string {
	return s.prefix + digest + ":spent"
}
