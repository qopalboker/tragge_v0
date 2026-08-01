package sms

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestOTPGenerationAndAtRestRepresentation(t *testing.T) {
	for i := 0; i < 128; i++ {
		code, err := GenerateCode()
		if err != nil {
			t.Fatal(err)
		}
		if !OTPCodeRegex.MatchString(code) {
			t.Fatalf("generated code does not meet six-digit policy")
		}
	}

	provider := NewFake()
	store := &fakeOTPStore{}
	service := newTestOTPService(t, provider, store)
	if err := service.SendOTP(context.Background(), "+989120000000"); err != nil {
		t.Fatal(err)
	}
	if store.digest == "" || store.digest == provider.LastCode() || len(store.digest) != 64 {
		t.Fatal("active OTP was not stored as an HMAC-SHA-256 digest")
	}
}

func TestOTPBindingsAndResendReplacement(t *testing.T) {
	provider := NewFake()
	store := &fakeOTPStore{}
	service := newTestOTPService(t, provider, store)
	phone := "+989120000000"
	if err := service.SendOTP(context.Background(), phone); err != nil {
		t.Fatal(err)
	}
	firstCode := provider.LastCode()

	if ok, err := service.VerifyOTP(context.Background(), "+989120000001", firstCode); err != nil || ok {
		t.Fatalf("code crossed destination binding: ok=%v err=%v", ok, err)
	}

	store.cooldown = false
	if err := service.SendOTP(context.Background(), phone); err != nil {
		t.Fatal(err)
	}
	secondCode := provider.LastCode()
	for attempts := 0; firstCode == secondCode && attempts < 10; attempts++ {
		store.cooldown = false
		if err := service.SendOTP(context.Background(), phone); err != nil {
			t.Fatal(err)
		}
		secondCode = provider.LastCode()
	}
	if firstCode == secondCode {
		t.Fatal("unable to obtain a distinct replacement code after bounded attempts")
	}
	if ok, err := service.VerifyOTP(context.Background(), phone, firstCode); err != nil || ok {
		t.Fatalf("replaced code remained usable: ok=%v err=%v", ok, err)
	}
	if ok, err := service.VerifyOTP(context.Background(), phone, secondCode); err != nil || !ok {
		t.Fatalf("replacement code was not usable: ok=%v err=%v", ok, err)
	}
}

func TestOTPRequiresConfiguredHashingKey(t *testing.T) {
	provider := NewFake()
	store := &fakeOTPStore{}
	first := newTestOTPService(t, provider, store)
	phone := "+989120000000"
	if err := first.SendOTP(context.Background(), phone); err != nil {
		t.Fatal(err)
	}
	code := provider.LastCode()
	second, err := newOTPServiceWithStore(NewFake(), store, OTPConfig{
		TTL: CanonicalOTPTTL, Cooldown: CanonicalOTPCooldown,
		MaxAttempts: CanonicalOTPMaxAttempts,
		HashSecret:  "different-test-only-HMAC-key-0123456789-ABCDEFG",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := second.VerifyOTP(context.Background(), phone, code); err != nil || ok {
		t.Fatalf("OTP validated without the configured hashing key: ok=%v err=%v", ok, err)
	}
}

func TestOTPConcurrentIssueCreatesOneActiveCode(t *testing.T) {
	provider := NewFake()
	store := &fakeOTPStore{}
	service := newTestOTPService(t, provider, store)

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- service.SendOTP(context.Background(), "+989120000000")
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	cooldowns := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrOTPCooldown):
			cooldowns++
		default:
			t.Fatalf("unexpected concurrent issue result: %v", err)
		}
	}
	if successes != 1 || cooldowns != 1 || store.activations != 1 {
		t.Fatalf("concurrent issue was not singular: successes=%d cooldowns=%d activations=%d",
			successes, cooldowns, store.activations)
	}
}
