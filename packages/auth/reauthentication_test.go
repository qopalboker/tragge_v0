package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type memoryReauthenticationStore struct {
	mu       sync.Mutex
	records  map[string]ReauthenticationGrant
	spent    map[string]bool
	fail     bool
	sequence int
}

func newMemoryReauthenticationStore() *memoryReauthenticationStore {
	return &memoryReauthenticationStore{
		records: make(map[string]ReauthenticationGrant),
		spent:   make(map[string]bool),
	}
}

func (s *memoryReauthenticationStore) Issue(_ context.Context, grant ReauthenticationGrant) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return "", errors.New("store unavailable")
	}
	s.sequence++
	token := fmt.Sprintf("opaque-test-grant-%d", s.sequence)
	s.records[token] = grant
	return token, nil
}

func (s *memoryReauthenticationStore) Consume(_ context.Context, token string) (*ReauthenticationGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return nil, errors.New("store unavailable")
	}
	if s.spent[token] {
		return nil, ErrReauthenticationReplayed
	}
	grant, ok := s.records[token]
	if !ok {
		return nil, ErrReauthenticationInvalid
	}
	delete(s.records, token)
	s.spent[token] = true
	return &grant, nil
}

func (s *memoryReauthenticationStore) RevokeActor(_ context.Context, actorID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("store unavailable")
	}
	for token, grant := range s.records {
		if grant.ActorID == actorID {
			delete(s.records, token)
		}
	}
	return nil
}

func validReauthenticationExpectation() ReauthenticationExpectation {
	return ReauthenticationExpectation{
		Context:             ContextAdmin,
		ActorID:             "admin-1",
		SessionID:           "session-1",
		Action:              "withdrawal.complete",
		ResourceID:          "withdrawal-1",
		SecurityFingerprint: ReauthenticationSecurityFingerprint("password-hash", []string{RoleSuperAdmin}, []string{"withdrawals.manage"}),
	}
}

func TestReauthenticationGrantLifecycle(t *testing.T) {
	t.Parallel()
	store := newMemoryReauthenticationStore()
	service, err := NewReauthenticationService(store, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	expectation := validReauthenticationExpectation()

	token, expiresAt, err := service.Issue(context.Background(), expectation)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" {
		t.Fatal("empty opaque grant")
	}
	if got := expiresAt.Sub(now); got != 5*time.Minute {
		t.Fatalf("TTL = %s", got)
	}
	stored := store.records[token]
	if stored.SessionDigest == expectation.SessionID {
		t.Fatal("raw session ID persisted in grant")
	}
	if stored.SecurityFingerprint == "password-hash" {
		t.Fatal("password hash persisted as fingerprint")
	}

	if err := service.Consume(context.Background(), token, expectation); err != nil {
		t.Fatalf("consume valid grant: %v", err)
	}
	if err := service.Consume(context.Background(), token, expectation); !errors.Is(err, ErrReauthenticationReplayed) {
		t.Fatalf("replay error = %v", err)
	}
}

func TestReauthenticationBindingFailuresConsumeGrant(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*ReauthenticationExpectation)
		want   error
	}{
		{"wrong actor", func(value *ReauthenticationExpectation) { value.ActorID = "admin-2" }, ErrReauthenticationActorBinding},
		{"wrong session", func(value *ReauthenticationExpectation) { value.SessionID = "session-2" }, ErrReauthenticationSessionBinding},
		{"wrong action", func(value *ReauthenticationExpectation) { value.Action = "wallet.adjust" }, ErrReauthenticationActionBinding},
		{"wrong resource", func(value *ReauthenticationExpectation) { value.ResourceID = "withdrawal-2" }, ErrReauthenticationResourceBinding},
		{"password or authorization changed", func(value *ReauthenticationExpectation) {
			value.SecurityFingerprint = ReauthenticationSecurityFingerprint("changed-hash", []string{RoleSuperAdmin}, []string{"withdrawals.manage"})
		}, ErrReauthenticationStateBinding},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newMemoryReauthenticationStore()
			service, err := NewReauthenticationService(store, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			expected := validReauthenticationExpectation()
			token, _, err := service.Issue(context.Background(), expected)
			if err != nil {
				t.Fatal(err)
			}
			wrong := expected
			tc.mutate(&wrong)
			if err := service.Consume(context.Background(), token, wrong); !errors.Is(err, tc.want) || !errors.Is(err, ErrReauthenticationBinding) {
				t.Fatalf("binding error = %v", err)
			}
			if err := service.Consume(context.Background(), token, expected); !errors.Is(err, ErrReauthenticationReplayed) {
				t.Fatalf("mismatched presentation did not burn grant: %v", err)
			}
		})
	}
}

func TestReauthenticationExpiryAndConfiguration(t *testing.T) {
	t.Parallel()
	if _, err := NewReauthenticationService(newMemoryReauthenticationStore(), MaxReauthenticationTTL+time.Second); !errors.Is(err, ErrReauthenticationInvalid) {
		t.Fatalf("accepted overlong TTL: %v", err)
	}
	store := newMemoryReauthenticationStore()
	service, err := NewReauthenticationService(store, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	expectation := validReauthenticationExpectation()
	token, _, err := service.Issue(context.Background(), expectation)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now.Add(time.Minute) }
	if err := service.Consume(context.Background(), token, expectation); !errors.Is(err, ErrReauthenticationExpired) {
		t.Fatalf("expiry error = %v", err)
	}
}

func TestReauthenticationStorageFailureFailsClosed(t *testing.T) {
	t.Parallel()
	store := newMemoryReauthenticationStore()
	store.fail = true
	service, err := NewReauthenticationService(store, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	expectation := validReauthenticationExpectation()
	if _, _, err := service.Issue(context.Background(), expectation); !errors.Is(err, ErrReauthenticationUnavailable) {
		t.Fatalf("issue error = %v", err)
	}
	if err := service.Consume(context.Background(), "opaque", expectation); err == nil {
		t.Fatal("storage failure allowed consume")
	}
}

func TestReauthenticationSecurityFingerprintIsCanonical(t *testing.T) {
	t.Parallel()
	left := ReauthenticationSecurityFingerprint(
		"hash",
		[]string{RoleUser, RoleSuperAdmin},
		[]string{"users.edit", "withdrawals.manage"},
	)
	right := ReauthenticationSecurityFingerprint(
		"hash",
		[]string{RoleSuperAdmin, RoleUser},
		[]string{"withdrawals.manage", "users.edit"},
	)
	if left != right {
		t.Fatal("fingerprint depends on role or permission order")
	}
	if left == ReauthenticationSecurityFingerprint("other", []string{RoleSuperAdmin, RoleUser}, []string{"withdrawals.manage", "users.edit"}) {
		t.Fatal("password change did not alter fingerprint")
	}
}
