package sms

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestSEC003RedisOTPLifecycle(t *testing.T) {
	address := os.Getenv("SEC003_REDIS_ADDR")
	if address == "" {
		t.Skip("SEC003_REDIS_ADDR is required for the isolated Redis runtime test")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal("ping isolated Redis test instance:", err)
	}
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatal("clear isolated Redis test database:", err)
	}

	provider := NewFake()
	service, err := NewOTPService(provider, client, DefaultOTPConfig(
		"test-only-Redis-OTP-HMAC-key-0123456789-ABCDEFG",
	))
	if err != nil {
		t.Fatal(err)
	}
	phone := "+989120000000"
	if err := service.SendOTP(ctx, phone); err != nil {
		t.Fatal("issue OTP:", err)
	}
	code := provider.LastCode()
	scope := otpScope(phone)
	stored, err := client.Get(ctx, scope+":code").Result()
	if err != nil {
		t.Fatal("read isolated OTP digest:", err)
	}
	if stored == code || len(stored) != 64 {
		t.Fatal("Redis stored an unsafe OTP representation")
	}
	codeTTL, err := client.TTL(ctx, scope+":code").Result()
	if err != nil || codeTTL <= 9*time.Minute || codeTTL > CanonicalOTPTTL {
		t.Fatalf("unexpected canonical OTP TTL: %s, %v", codeTTL, err)
	}
	cooldownTTL, err := client.TTL(ctx, scope+":cooldown").Result()
	if err != nil || cooldownTTL <= 50*time.Second || cooldownTTL > CanonicalOTPCooldown {
		t.Fatalf("unexpected OTP cooldown TTL: %s, %v", cooldownTTL, err)
	}
	if err := service.SendOTP(ctx, phone); !errors.Is(err, ErrOTPCooldown) {
		t.Fatal("Redis cooldown did not reject resend")
	}

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _ := service.VerifyOTP(ctx, phone, code)
			results <- ok
		}()
	}
	wg.Wait()
	close(results)
	successes := 0
	for ok := range results {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("Redis OTP consume succeeded %d times", successes)
	}
	if ok, err := service.VerifyOTP(ctx, phone, code); ok || !errors.Is(err, ErrOTPExpired) {
		t.Fatal("Redis OTP replay did not fail closed")
	}
}
func TestSEC003RedisOTPFailureAndBindingMatrix(t *testing.T) {
	address := os.Getenv("SEC003_REDIS_ADDR")
	if address == "" {
		t.Skip("SEC003_REDIS_ADDR is required for the isolated Redis runtime test")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatal("ping isolated Redis test instance:", err)
	}

	newService := func(t *testing.T) (*OTPService, *FakeProvider, *redisOTPStore) {
		t.Helper()
		if err := client.FlushDB(ctx).Err(); err != nil {
			t.Fatal("clear isolated Redis test database:", err)
		}
		provider := NewFake()
		store := &redisOTPStore{client: client}
		service, err := newOTPServiceWithStore(provider, store, DefaultOTPConfig(
			"test-only-Redis-OTP-HMAC-key-0123456789-ABCDEFG",
		))
		if err != nil {
			t.Fatal(err)
		}
		return service, provider, store
	}

	t.Run("reservation nonce and failed delivery are fail closed", func(t *testing.T) {
		service, provider, store := newService(t)
		phone := "+989120000100"
		scope := otpScope(phone)
		if _, err := store.Reserve(ctx, scope, "reservation-a", otpReservationTTL); err != nil {
			t.Fatal(err)
		}
		if err := client.Set(ctx, scope+":pending", "reservation-b", otpReservationTTL).Err(); err != nil {
			t.Fatal(err)
		}
		if err := store.Activate(ctx, scope, "reservation-a", "test-digest", CanonicalOTPTTL, CanonicalOTPCooldown); !errors.Is(err, ErrOTPUnavailable) {
			t.Fatalf("stale activation did not fail closed: %v", err)
		}
		if exists := client.Exists(ctx, scope+":code").Val(); exists != 0 {
			t.Fatal("stale activation created a usable code")
		}
		if err := store.Activate(ctx, scope, "reservation-b", "test-digest", CanonicalOTPTTL, CanonicalOTPCooldown); err != nil {
			t.Fatal("matching reservation did not activate:", err)
		}

		if err := client.FlushDB(ctx).Err(); err != nil {
			t.Fatal(err)
		}
		provider.SetError(errors.New("controlled provider rejection"))
		if err := service.SendOTP(ctx, phone); !errors.Is(err, ErrOTPDelivery) {
			t.Fatalf("provider failure result = %v", err)
		}
		if client.Exists(ctx, scope+":code", scope+":pending").Val() != 0 {
			t.Fatal("provider failure left usable or pending state")
		}
		if client.TTL(ctx, scope+":cooldown").Val() <= 0 {
			t.Fatal("failed delivery did not retain abuse-safe cooldown")
		}
		provider.SetError(nil)
		if err := service.SendOTP(ctx, phone); !errors.Is(err, ErrOTPCooldown) {
			t.Fatal("failed-delivery cancellation bypassed cooldown")
		}
	})

	t.Run("attempt exhaustion expiration and replay", func(t *testing.T) {
		service, provider, _ := newService(t)
		phone := "+989120000101"
		if err := service.SendOTP(ctx, phone); err != nil {
			t.Fatal(err)
		}
		code := provider.LastCode()
		for attempt := 1; attempt < CanonicalOTPMaxAttempts; attempt++ {
			if ok, err := service.VerifyOTP(ctx, phone, "000000"); ok || err != nil {
				t.Fatalf("wrong attempt %d: ok=%v err=%v", attempt, ok, err)
			}
		}
		if ok, err := service.VerifyOTP(ctx, phone, "000000"); ok || !errors.Is(err, ErrOTPExhausted) {
			t.Fatalf("attempt exhaustion: ok=%v err=%v", ok, err)
		}
		if ok, err := service.VerifyOTP(ctx, phone, code); ok || !errors.Is(err, ErrOTPExpired) {
			t.Fatalf("exhausted code remained usable: ok=%v err=%v", ok, err)
		}

		service, provider, _ = newService(t)
		if err := service.SendOTP(ctx, phone); err != nil {
			t.Fatal(err)
		}
		code = provider.LastCode()
		if err := client.PExpire(ctx, otpScope(phone)+":code", time.Millisecond).Err(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
		if ok, err := service.VerifyOTP(ctx, phone, code); ok || !errors.Is(err, ErrOTPExpired) {
			t.Fatalf("expired code result: ok=%v err=%v", ok, err)
		}
	})

	t.Run("destination purpose and channel bindings", func(t *testing.T) {
		service, provider, _ := newService(t)
		phone := "+989120000102"
		if err := service.SendOTP(ctx, phone); err != nil {
			t.Fatal(err)
		}
		code := provider.LastCode()
		if ok, err := service.VerifyOTP(ctx, "+989120000103", code); ok || !errors.Is(err, ErrOTPExpired) {
			t.Fatalf("wrong destination result: ok=%v err=%v", ok, err)
		}
		if ok, err := service.VerifyOTP(ctx, phone, code); !ok || err != nil {
			t.Fatalf("destination check mutated valid state: ok=%v err=%v", ok, err)
		}

		for _, binding := range []struct {
			name, purpose, channel string
		}{
			{name: "wrong purpose", purpose: "password_reset", channel: "sms"},
			{name: "wrong channel", purpose: phoneAuthPurpose, channel: "email"},
		} {
			t.Run(binding.name, func(t *testing.T) {
				service, _, _ := newService(t)
				scope := otpScope(phone)
				wrongDigest := testOTPDigest(
					"test-only-Redis-OTP-HMAC-key-0123456789-ABCDEFG",
					binding.purpose, phone, binding.channel, "123456",
				)
				if err := client.Set(ctx, scope+":code", wrongDigest, CanonicalOTPTTL).Err(); err != nil {
					t.Fatal(err)
				}
				if ok, err := service.VerifyOTP(ctx, phone, "123456"); ok || err != nil {
					t.Fatalf("cross-binding result: ok=%v err=%v", ok, err)
				}
			})
		}
	})

	t.Run("resend replacement and atomic concurrency", func(t *testing.T) {
		service, provider, _ := newService(t)
		phone := "+989120000104"
		if err := service.SendOTP(ctx, phone); err != nil {
			t.Fatal(err)
		}
		first := provider.LastCode()
		if err := client.Del(ctx, otpScope(phone)+":cooldown").Err(); err != nil {
			t.Fatal(err)
		}
		if err := service.SendOTP(ctx, phone); err != nil {
			t.Fatal(err)
		}
		second := provider.LastCode()
		if first == second {
			t.Fatal("CSPRNG returned an unusable collision for resend test")
		}
		if ok, err := service.VerifyOTP(ctx, phone, first); ok || err != nil {
			t.Fatalf("replaced code result: ok=%v err=%v", ok, err)
		}
		if ok, err := service.VerifyOTP(ctx, phone, second); !ok || err != nil {
			t.Fatalf("replacement code result: ok=%v err=%v", ok, err)
		}

		service, _, _ = newService(t)
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- service.SendOTP(ctx, phone)
			}()
		}
		wg.Wait()
		close(results)
		successes, cooldowns := 0, 0
		for err := range results {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrOTPCooldown):
				cooldowns++
			default:
				t.Fatalf("concurrent resend result: %v", err)
			}
		}
		if successes != 1 || cooldowns != 1 || client.Exists(ctx, otpScope(phone)+":code").Val() != 1 {
			t.Fatalf("concurrent resend state: successes=%d cooldowns=%d", successes, cooldowns)
		}
	})

	t.Run("namespace representation and Redis failure", func(t *testing.T) {
		service, provider, _ := newService(t)
		phone := "+989120000105"
		if err := service.SendOTP(ctx, phone); err != nil {
			t.Fatal(err)
		}
		code := provider.LastCode()
		keys, err := client.Keys(ctx, "*").Result()
		if err != nil || len(keys) == 0 {
			t.Fatalf("inspect SEC-003 keys: count=%d err=%v", len(keys), err)
		}
		for _, key := range keys {
			if !strings.HasPrefix(key, "security-code:sms:phone-auth:") {
				t.Fatalf("unexpected key namespace: %q", key)
			}
			value, err := client.Get(ctx, key).Result()
			if err == nil && value == code {
				t.Fatal("plaintext OTP was stored in Redis")
			}
		}

		unavailable := redis.NewClient(&redis.Options{
			Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond,
			ReadTimeout: 50 * time.Millisecond, WriteTimeout: 50 * time.Millisecond,
		})
		defer unavailable.Close()
		failedService, err := NewOTPService(NewFake(), unavailable, DefaultOTPConfig(
			"test-only-Redis-OTP-HMAC-key-0123456789-ABCDEFG",
		))
		if err != nil {
			t.Fatal(err)
		}
		if err := failedService.SendOTP(ctx, phone); !errors.Is(err, ErrOTPUnavailable) {
			t.Fatalf("Redis command failure did not fail closed: %v", err)
		}
	})
}

func testOTPDigest(secret, purpose, destination, channel, code string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	for _, value := range []string{purpose, strings.TrimSpace(destination), channel, code} {
		_, _ = fmt.Fprintf(mac, "%d:", len(value))
		_, _ = mac.Write([]byte(value))
	}
	return hex.EncodeToString(mac.Sum(nil))
}
