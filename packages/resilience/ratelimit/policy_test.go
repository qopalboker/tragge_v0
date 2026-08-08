package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"

	"github.com/redis/go-redis/v9"
)

func TestPolicyMiddlewareLimitAndSafeKeys(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	middleware := NewPolicyMiddleware(nil, []Policy{{
		Method: http.MethodPost, PathPrefix: "/login", Class: ClassLogin,
		Limit: 2, Window: time.Minute,
	}}, func() time.Time { return now }, nil)
	counts := map[string]int{}
	var keys []string
	middleware.increment = func(_ context.Context, key string, _ time.Duration) (int, error) {
		keys = append(keys, key)
		counts[key]++
		return counts[key], nil
	}
	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for i, want := range []int{http.StatusNoContent, http.StatusNoContent, http.StatusTooManyRequests} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/login", nil)
		req.RemoteAddr = "203.0.113.19:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("request %d status=%d want=%d", i+1, rec.Code, want)
		}
		if want == http.StatusTooManyRequests && rec.Header().Get("Retry-After") == "" {
			t.Fatal("rate-limit response missing Retry-After")
		}
	}
	for _, key := range keys {
		if strings.Contains(key, "203.0.113.19") {
			t.Fatal("raw identity persisted in rate-limit key")
		}
	}
}

func TestPolicyMiddlewareFailsClosedAndClassifies(t *testing.T) {
	policy := Policy{Method: http.MethodPost, PathPrefix: "/withdraw", Class: ClassWithdrawal, Limit: 1, Window: time.Minute}
	middleware := NewPolicyMiddleware(nil, []Policy{policy}, nil, nil)
	middleware.increment = func(context.Context, string, time.Duration) (int, error) {
		return 0, errors.New("fixture storage failure")
	}
	handler := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/withdraw", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "EDGE_POLICY_UNAVAILABLE") {
		t.Fatalf("storage failure did not fail closed: status=%d body=%s", rec.Code, rec.Body.String())
	}

	required := map[EndpointClass]bool{
		ClassLogin: false, ClassRegistration: false, ClassOTPRequest: false, ClassOTPVerify: false,
		ClassPasswordReset: false, ClassContestJoin: false, ClassOrder: false, ClassCancel: false,
		ClassDeposit: false, ClassWithdrawal: false, ClassAdmin: false, ClassWebhook: false, ClassWebSocket: false,
	}
	for _, service := range []string{serviceUser, serviceAdmin, serviceTrade, servicePayment} {
		for _, candidate := range PoliciesForService(service) {
			if _, ok := required[candidate.Class]; ok {
				required[candidate.Class] = true
			}
		}
	}
	for class, found := range required {
		if !found {
			t.Errorf("required endpoint class %s is not configured", class)
		}
	}
}

func TestActorPolicyUsesAuthenticatedContext(t *testing.T) {
	policy := Policy{Method: http.MethodPost, PathPrefix: "/order", Class: ClassOrder, Limit: 1, Window: time.Minute}
	middleware := NewPolicyMiddleware(nil, []Policy{policy}, func() time.Time {
		return time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	}, nil)
	var capturedKey string
	middleware.increment = func(_ context.Context, key string, _ time.Duration) (int, error) {
		capturedKey = key
		return 1, nil
	}
	handler := middleware.ActorHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	missing := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/order", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missing)
	if missingRec.Code != http.StatusUnauthorized {
		t.Fatalf("missing actor status=%d", missingRec.Code)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/order", nil)
	req = req.WithContext(context.WithValue(req.Context(), auth.UserIDKey, "actor-fixture"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("authenticated actor status=%d", rec.Code)
	}
	if strings.Contains(capturedKey, "actor-fixture") || !strings.Contains(capturedKey, ":actor:") {
		t.Fatalf("actor key not safely namespaced and digested: %q", capturedKey)
	}
}

func TestRedisPolicyAndLoginLockoutIntegration(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("SEC006_REDIS_ADDR"))
	if address == "" {
		t.Skip("SEC006_REDIS_ADDR is required for isolated runtime validation")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close isolated Redis client: %v", err)
		}
	})
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("isolated Redis unavailable: %v", err)
	}
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("isolated Redis cleanup failed: %v", err)
	}
	defer client.FlushDB(ctx)

	classPolicies := map[EndpointClass]Policy{}
	for _, service := range []string{serviceUser, serviceAdmin, serviceTrade, servicePayment} {
		for _, candidate := range PoliciesForService(service) {
			if _, present := classPolicies[candidate.Class]; !present {
				candidate.Limit, candidate.Window = 1, time.Minute
				candidate.Burst, candidate.BurstWindow = 0, 0
				classPolicies[candidate.Class] = candidate
			}
		}
	}
	for _, class := range []EndpointClass{
		ClassPublicRead, ClassLogin, ClassRegistration, ClassOTPRequest, ClassOTPVerify,
		ClassPasswordReset, ClassContestJoin, ClassOrder, ClassCancel, ClassDeposit,
		ClassWithdrawal, ClassAdmin, ClassWebhook, ClassWebSocket,
	} {
		t.Run("distributed endpoint class "+string(class), func(t *testing.T) {
			candidate, present := classPolicies[class]
			if !present {
				t.Fatalf("missing policy for %s", class)
			}
			method := candidate.Method
			if method == "" {
				method = http.MethodGet
			}
			path := candidate.PathPrefix
			if path == "/" {
				path = "/fixture"
			}
			newClassHandler := func() http.Handler {
				middleware := NewPolicyMiddleware(client, []Policy{candidate}, func() time.Time {
					return time.Date(2026, 8, 1, 11, 0, 0, 0, time.UTC)
				}, nil)
				return middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				}))
			}
			for index, handler := range []http.Handler{newClassHandler(), newClassHandler()} {
				req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
				req.RemoteAddr = "198.51.100.44:1234"
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				want := http.StatusNoContent
				if index == 1 {
					want = http.StatusTooManyRequests
				}
				if rec.Code != want {
					t.Fatalf("request %d status=%d want=%d", index+1, rec.Code, want)
				}
			}
		})
	}

	policy := Policy{Method: http.MethodPost, PathPrefix: "/login", Class: ClassLogin, Limit: 2, Window: time.Minute}
	newHandler := func() http.Handler {
		m := NewPolicyMiddleware(client, []Policy{policy}, func() time.Time {
			return time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
		}, nil)
		return m.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	}
	for index, handler := range []http.Handler{newHandler(), newHandler(), newHandler()} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/login", nil)
		req.RemoteAddr = "203.0.113.27:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		want := http.StatusNoContent
		if index == 2 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("distributed request %d status=%d want=%d", index+1, rec.Code, want)
		}
	}

	lockout, err := NewLoginLockout(client, LockoutConfig{
		Namespace: "integration", Threshold: 3, LockFor: 250 * time.Millisecond, Retention: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	const identity = "account:test@example.invalid"
	var failures atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, failureErr := lockout.Failure(ctx, identity); failureErr != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("concurrent lockout writes failed: %d", failures.Load())
	}
	allowed, _, err := lockout.Check(ctx, identity)
	if err != nil || allowed {
		t.Fatalf("threshold did not lock identity: allowed=%v err=%v", allowed, err)
	}
	time.Sleep(300 * time.Millisecond)
	allowed, _, err = lockout.Check(ctx, identity)
	if err != nil || !allowed {
		t.Fatalf("lockout did not expire: allowed=%v err=%v", allowed, err)
	}
	if _, err := lockout.Failure(ctx, identity); err != nil {
		t.Fatal(err)
	}
	if err := lockout.Success(ctx, identity); err != nil {
		t.Fatal(err)
	}
	allowed, _, err = lockout.Check(ctx, identity)
	if err != nil || !allowed {
		t.Fatalf("successful login did not clear lockout state: allowed=%v err=%v", allowed, err)
	}
}
