package sms

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeOTPStore struct {
	mu          sync.Mutex
	pending     string
	digest      string
	attempts    int
	cooldown    bool
	fail        error
	activations int
}

func (s *fakeOTPStore) Reserve(_ context.Context, _ string, reservation string, _ time.Duration) (time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return 0, s.fail
	}
	if s.pending != "" || s.cooldown {
		return time.Minute, ErrOTPCooldown
	}
	s.pending = reservation
	return 0, nil
}
func (s *fakeOTPStore) Activate(_ context.Context, _, reservation, digest string, _, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil || s.pending != reservation {
		return ErrOTPUnavailable
	}
	s.pending = ""
	s.digest = digest
	s.attempts = 0
	s.cooldown = true
	s.activations++
	return nil
}
func (s *fakeOTPStore) Cancel(_ context.Context, _, reservation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == reservation {
		s.pending = ""
		s.cooldown = true
	}
	return s.fail
}
func (s *fakeOTPStore) Load(_ context.Context, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return "", s.fail
	}
	if s.digest == "" {
		return "", ErrOTPExpired
	}
	return s.digest, nil
}
func (s *fakeOTPStore) RecordFailure(_ context.Context, _, expected string, max int, _ time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil || s.digest != expected {
		return 0, ErrOTPUnavailable
	}
	s.attempts++
	if s.attempts >= max {
		s.digest = ""
		s.cooldown = false
		return 0, nil
	}
	return max - s.attempts, nil
}
func (s *fakeOTPStore) Consume(_ context.Context, _, expected string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return false, s.fail
	}
	if s.digest == "" || s.digest != expected {
		return false, nil
	}
	s.digest = ""
	s.cooldown = false
	return true, nil
}

func newTestOTPService(t *testing.T, provider SMSProvider, store otpStore) *OTPService {
	t.Helper()
	service, err := newOTPServiceWithStore(provider, store, DefaultOTPConfig(
		"test-only-SMS-OTP-HMAC-key-0123456789-ABCDEFG",
	))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestOTPPolicyIsCanonical(t *testing.T) {
	cfg := DefaultOTPConfig("test-only-SMS-OTP-HMAC-key-0123456789-ABCDEFG")
	if cfg.TTL != 10*time.Minute || cfg.Cooldown != 60*time.Second || cfg.MaxAttempts != 5 {
		t.Fatalf("non-canonical policy: %+v", cfg)
	}
	cfg.TTL = 2 * time.Minute
	if _, err := newOTPServiceWithStore(NewFake(), &fakeOTPStore{}, cfg); !errors.Is(err, ErrOTPConfiguration) {
		t.Fatal("non-canonical TTL was accepted")
	}
}

func TestOTPDeliveryFailureLeavesNoUsableCode(t *testing.T) {
	provider := NewFake()
	provider.SetError(errors.New("provider fixture failure"))
	store := &fakeOTPStore{}
	service := newTestOTPService(t, provider, store)
	if err := service.SendOTP(context.Background(), "+989120000000"); !errors.Is(err, ErrOTPDelivery) {
		t.Fatalf("expected delivery failure, got %v", err)
	}
	if store.digest != "" || store.activations != 0 {
		t.Fatal("failed delivery activated a usable code")
	}
	if err := service.SendOTP(context.Background(), "+989120000000"); !errors.Is(err, ErrOTPCooldown) {
		t.Fatal("failed delivery bypassed the canonical cooldown")
	}
}

func TestOTPSuccessCooldownAttemptsAndReplay(t *testing.T) {
	provider := NewFake()
	store := &fakeOTPStore{}
	service := newTestOTPService(t, provider, store)
	phone := "+989120000000"
	if err := service.SendOTP(context.Background(), phone); err != nil {
		t.Fatal(err)
	}
	if err := service.SendOTP(context.Background(), phone); !errors.Is(err, ErrOTPCooldown) {
		t.Fatalf("expected cooldown, got %v", err)
	}
	for i := 0; i < 4; i++ {
		ok, err := service.VerifyOTP(context.Background(), phone, "000000")
		if err != nil || ok {
			t.Fatalf("wrong attempt %d: ok=%v err=%v", i+1, ok, err)
		}
	}
	if ok, err := service.VerifyOTP(context.Background(), phone, "000000"); ok || !errors.Is(err, ErrOTPExhausted) {
		t.Fatalf("fifth wrong attempt did not exhaust code: ok=%v err=%v", ok, err)
	}

	store.cooldown = false
	if err := service.SendOTP(context.Background(), phone); err != nil {
		t.Fatal(err)
	}
	code := provider.LastCode()
	if ok, err := service.VerifyOTP(context.Background(), phone, code); err != nil || !ok {
		t.Fatalf("valid code failed: ok=%v err=%v", ok, err)
	}
	if ok, err := service.VerifyOTP(context.Background(), phone, code); ok || !errors.Is(err, ErrOTPExpired) {
		t.Fatalf("replay did not fail closed: ok=%v err=%v", ok, err)
	}
}

func TestOTPConcurrentConsumeAllowsExactlyOne(t *testing.T) {
	provider := NewFake()
	store := &fakeOTPStore{}
	service := newTestOTPService(t, provider, store)
	phone := "+989120000000"
	if err := service.SendOTP(context.Background(), phone); err != nil {
		t.Fatal(err)
	}
	code := provider.LastCode()

	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _ := service.VerifyOTP(context.Background(), phone, code)
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
		t.Fatalf("expected exactly one successful consume, got %d", successes)
	}
}

func TestOTPStorageFailureFailsClosed(t *testing.T) {
	store := &fakeOTPStore{fail: errors.New("storage unavailable")}
	service := newTestOTPService(t, NewFake(), store)
	if err := service.SendOTP(context.Background(), "+989120000000"); !errors.Is(err, ErrOTPUnavailable) {
		t.Fatalf("send did not fail closed: %v", err)
	}
	if ok, err := service.VerifyOTP(context.Background(), "+989120000000", "123456"); ok || !errors.Is(err, ErrOTPUnavailable) {
		t.Fatalf("verify did not fail closed: ok=%v err=%v", ok, err)
	}
}
