package sms

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	CanonicalOTPTTL         = 10 * time.Minute
	CanonicalOTPCooldown    = 60 * time.Second
	CanonicalOTPMaxAttempts = 5
	otpReservationTTL       = 30 * time.Second
	phoneAuthPurpose        = "phone_auth"
)

var (
	OTPCodeRegex = regexp.MustCompile(`^\d{6}$`)

	ErrOTPConfiguration = errors.New("SMS OTP configuration is invalid")
	ErrOTPUnavailable   = errors.New("SMS OTP security state is unavailable")
	ErrOTPDelivery      = errors.New("SMS OTP delivery was not accepted")
	ErrOTPCooldown      = errors.New("SMS OTP resend cooldown is active")
	ErrOTPExpired       = errors.New("SMS OTP is expired or unavailable")
	ErrOTPExhausted     = errors.New("SMS OTP attempts are exhausted")
)

type otpStore interface {
	Reserve(context.Context, string, string, time.Duration) (time.Duration, error)
	Activate(context.Context, string, string, string, time.Duration, time.Duration) error
	Cancel(context.Context, string, string) error
	Load(context.Context, string) (string, error)
	RecordFailure(context.Context, string, string, int, time.Duration) (int, error)
	Consume(context.Context, string, string) (bool, error)
}

type redisOTPStore struct {
	client redis.Cmdable
}

// OTPService handles CSPRNG generation, fail-closed provider delivery, and
// replay-safe Redis verification for phone authentication.
type OTPService struct {
	provider SMSProvider
	store    otpStore
	key      []byte
}

type OTPConfig struct {
	TTL         time.Duration
	Cooldown    time.Duration
	MaxAttempts int
	HashSecret  string
}

func DefaultOTPConfig(hashSecret string) OTPConfig {
	return OTPConfig{
		TTL: CanonicalOTPTTL, Cooldown: CanonicalOTPCooldown,
		MaxAttempts: CanonicalOTPMaxAttempts, HashSecret: hashSecret,
	}
}

func NewOTPService(provider SMSProvider, rdb redis.Cmdable, cfg OTPConfig) (*OTPService, error) {
	if rdb == nil {
		return nil, ErrOTPConfiguration
	}
	return newOTPServiceWithStore(provider, &redisOTPStore{client: rdb}, cfg)
}

func newOTPServiceWithStore(provider SMSProvider, store otpStore, cfg OTPConfig) (*OTPService, error) {
	if provider == nil || store == nil || cfg.TTL != CanonicalOTPTTL ||
		cfg.Cooldown != CanonicalOTPCooldown || cfg.MaxAttempts != CanonicalOTPMaxAttempts ||
		len(cfg.HashSecret) < 32 {
		return nil, ErrOTPConfiguration
	}
	return &OTPService{provider: provider, store: store, key: []byte(cfg.HashSecret)}, nil
}

func (s *OTPService) Provider() SMSProvider { return s.provider }

func GenerateCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(900000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()+100000), nil
}

func otpScope(phone string) string {
	sum := sha256.Sum256([]byte(phoneAuthPurpose + "\x00" + strings.TrimSpace(phone)))
	return "security-code:sms:phone-auth:" + hex.EncodeToString(sum[:])
}

func (s *OTPService) digest(phone, code string) string {
	mac := hmac.New(sha256.New, s.key)
	for _, value := range []string{phoneAuthPurpose, strings.TrimSpace(phone), "sms", code} {
		_, _ = fmt.Fprintf(mac, "%d:", len(value))
		_, _ = mac.Write([]byte(value))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *OTPService) SendOTP(ctx context.Context, phone string) error {
	code, err := GenerateCode()
	if err != nil {
		return ErrOTPUnavailable
	}
	reservationBytes := make([]byte, 24)
	if _, err := rand.Read(reservationBytes); err != nil {
		return ErrOTPUnavailable
	}
	reservation := hex.EncodeToString(reservationBytes)
	scope := otpScope(phone)

	if _, err := s.store.Reserve(ctx, scope, reservation, otpReservationTTL); err != nil {
		if errors.Is(err, ErrOTPCooldown) {
			return ErrOTPCooldown
		}
		return ErrOTPUnavailable
	}
	start := time.Now()
	if err := s.provider.SendOTP(ctx, phone, code); err != nil {
		otpSentTotal.WithLabelValues("failure").Inc()
		smsLatency.Observe(time.Since(start).Seconds())
		_ = s.store.Cancel(ctx, scope, reservation)
		return ErrOTPDelivery
	}
	if err := s.store.Activate(
		ctx, scope, reservation, s.digest(phone, code),
		CanonicalOTPTTL, CanonicalOTPCooldown,
	); err != nil {
		return ErrOTPUnavailable
	}
	otpSentTotal.WithLabelValues("success").Inc()
	smsLatency.Observe(time.Since(start).Seconds())
	return nil
}

func (s *OTPService) VerifyOTP(ctx context.Context, phone, code string) (bool, error) {
	if !OTPCodeRegex.MatchString(code) {
		return false, nil
	}
	scope := otpScope(phone)
	stored, err := s.store.Load(ctx, scope)
	if err != nil {
		if errors.Is(err, ErrOTPExpired) {
			otpVerifiedTotal.WithLabelValues("expired").Inc()
			return false, ErrOTPExpired
		}
		return false, ErrOTPUnavailable
	}
	candidate := s.digest(phone, code)
	if len(stored) != len(candidate) || subtle.ConstantTimeCompare([]byte(stored), []byte(candidate)) != 1 {
		remaining, err := s.store.RecordFailure(
			ctx, scope, stored, CanonicalOTPMaxAttempts, CanonicalOTPTTL,
		)
		if err != nil {
			return false, ErrOTPUnavailable
		}
		if remaining == 0 {
			otpVerifiedTotal.WithLabelValues("blocked").Inc()
			return false, ErrOTPExhausted
		}
		otpVerifiedTotal.WithLabelValues("failure").Inc()
		return false, nil
	}
	consumed, err := s.store.Consume(ctx, scope, stored)
	if err != nil {
		return false, ErrOTPUnavailable
	}
	if !consumed {
		return false, ErrOTPExpired
	}
	otpVerifiedTotal.WithLabelValues("success").Inc()
	return true, nil
}

var reserveOTPScript = redis.NewScript(`
local cooldown = KEYS[1] .. ":cooldown"
local pending = KEYS[1] .. ":pending"
if redis.call("EXISTS", cooldown) == 1 then
  return redis.call("TTL", cooldown)
end
if redis.call("EXISTS", pending) == 1 then
  return -1
end
redis.call("SET", pending, ARGV[1], "EX", ARGV[2], "NX")
return 0
`)

var activateOTPScript = redis.NewScript(`
local pending = KEYS[1] .. ":pending"
if redis.call("GET", pending) ~= ARGV[1] then
  return 0
end
redis.call("SET", KEYS[1] .. ":code", ARGV[2], "EX", ARGV[3])
redis.call("SET", KEYS[1] .. ":cooldown", "1", "EX", ARGV[4])
redis.call("DEL", KEYS[1] .. ":attempts", pending)
return 1
`)

var cancelOTPReservationScript = redis.NewScript(`
local pending = KEYS[1] .. ":pending"
if redis.call("GET", pending) == ARGV[1] then
  redis.call("SET", KEYS[1] .. ":cooldown", "1", "EX", ARGV[2])
  redis.call("DEL", pending)
  return 1
end
return 0
`)

var recordOTPFailureScript = redis.NewScript(`
local code = KEYS[1] .. ":code"
local attemptsKey = KEYS[1] .. ":attempts"
if redis.call("GET", code) ~= ARGV[1] then
  return -1
end
local attempts = redis.call("INCR", attemptsKey)
redis.call("EXPIRE", attemptsKey, ARGV[3])
if attempts >= tonumber(ARGV[2]) then
  redis.call("DEL", code, attemptsKey, KEYS[1] .. ":cooldown")
  return 0
end
return tonumber(ARGV[2]) - attempts
`)

var consumeOTPScript = redis.NewScript(`
local code = KEYS[1] .. ":code"
if redis.call("GET", code) ~= ARGV[1] then
  return 0
end
redis.call("DEL", code, KEYS[1] .. ":attempts", KEYS[1] .. ":cooldown")
return 1
`)

func (s *redisOTPStore) Reserve(ctx context.Context, scope, reservation string, ttl time.Duration) (time.Duration, error) {
	result, err := reserveOTPScript.Run(ctx, s.client, []string{scope}, reservation, int(ttl.Seconds())).Int64()
	if err != nil {
		return 0, ErrOTPUnavailable
	}
	if result != 0 {
		if result < 1 {
			result = 1
		}
		return time.Duration(result) * time.Second, ErrOTPCooldown
	}
	return 0, nil
}

func (s *redisOTPStore) Activate(
	ctx context.Context,
	scope, reservation, digest string,
	ttl, cooldown time.Duration,
) error {
	result, err := activateOTPScript.Run(
		ctx, s.client, []string{scope}, reservation, digest,
		int(ttl.Seconds()), int(cooldown.Seconds()),
	).Int64()
	if err != nil || result != 1 {
		return ErrOTPUnavailable
	}
	return nil
}

func (s *redisOTPStore) Cancel(ctx context.Context, scope, reservation string) error {
	if _, err := cancelOTPReservationScript.Run(ctx, s.client, []string{scope}, reservation, int(CanonicalOTPCooldown.Seconds())).Result(); err != nil {
		return ErrOTPUnavailable
	}
	return nil
}

func (s *redisOTPStore) Load(ctx context.Context, scope string) (string, error) {
	value, err := s.client.Get(ctx, scope+":code").Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrOTPExpired
	}
	if err != nil {
		return "", ErrOTPUnavailable
	}
	return value, nil
}

func (s *redisOTPStore) RecordFailure(
	ctx context.Context,
	scope, expected string,
	maxAttempts int,
	ttl time.Duration,
) (int, error) {
	result, err := recordOTPFailureScript.Run(
		ctx, s.client, []string{scope}, expected, maxAttempts, int(ttl.Seconds()),
	).Int64()
	if err != nil || result < 0 {
		return 0, ErrOTPUnavailable
	}
	return int(result), nil
}

func (s *redisOTPStore) Consume(ctx context.Context, scope, expected string) (bool, error) {
	result, err := consumeOTPScript.Run(ctx, s.client, []string{scope}, expected).Int64()
	if err != nil {
		return false, ErrOTPUnavailable
	}
	return result == 1, nil
}
