package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/redis/go-redis/v9"
)

// EndpointClass is a bounded, low-cardinality security-policy category.
type EndpointClass string

const (
	ClassPublicRead    EndpointClass = "public_read"
	ClassLogin         EndpointClass = "login"
	ClassRegistration  EndpointClass = "registration"
	ClassOTPRequest    EndpointClass = "otp_request"
	ClassOTPVerify     EndpointClass = "otp_verify"
	ClassPasswordReset EndpointClass = "password_reset"
	ClassContestJoin   EndpointClass = "contest_join"
	ClassOrder         EndpointClass = "order"
	ClassCancel        EndpointClass = "cancel"
	ClassDeposit       EndpointClass = "deposit"
	ClassWithdrawal    EndpointClass = "withdrawal"
	ClassAdmin         EndpointClass = "admin"
	ClassWebhook       EndpointClass = "webhook"
	ClassWebSocket     EndpointClass = "websocket"
)

// Policy describes one explicit route-class control.
type Policy struct {
	Method      string
	PathPrefix  string
	Class       EndpointClass
	Limit       int
	Window      time.Duration
	Burst       int
	BurstWindow time.Duration
	HighRisk    bool
}

// Clock is injectable for deterministic tests.
type Clock func() time.Time

// DenialObserver receives only safe low-cardinality policy data.
type DenialObserver func(class EndpointClass, reason string)

// PolicyMiddleware provides distributed IP and authenticated-actor rate limits.
// Redis stores only SHA-256 digests under the sec006 namespace.
type PolicyMiddleware struct {
	client    redis.UniversalClient
	policies  []Policy
	clock     Clock
	observe   DenialObserver
	increment func(context.Context, string, time.Duration) (int, error)
}

func NewPolicyMiddleware(client redis.UniversalClient, policies []Policy, clock Clock, observer DenialObserver) *PolicyMiddleware {
	if clock == nil {
		clock = time.Now
	}
	middleware := &PolicyMiddleware{client: client, policies: append([]Policy(nil), policies...), clock: clock, observe: observer}
	middleware.increment = func(ctx context.Context, key string, ttl time.Duration) (int, error) {
		if middleware.client == nil {
			return 0, ErrRedisUnavailable
		}
		return redis.NewScript(policyFixedWindowScript).Run(ctx, middleware.client, []string{key}, ttl.Milliseconds()).Int()
	}
	return middleware
}

const policyFixedWindowScript = `
local current = redis.call('INCR', KEYS[1])
if current == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return current
`

func digestKey(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ToLower(value))))
	return hex.EncodeToString(sum[:])
}

func (m *PolicyMiddleware) policyFor(r *http.Request) (Policy, bool) {
	var selected Policy
	matched := false
	for _, policy := range m.policies {
		if policy.Method != "" && !strings.EqualFold(policy.Method, r.Method) {
			continue
		}
		if !strings.HasPrefix(r.URL.Path, policy.PathPrefix) {
			continue
		}
		if !matched || len(policy.PathPrefix) > len(selected.PathPrefix) {
			selected, matched = policy, true
		}
	}
	return selected, matched
}

func (m *PolicyMiddleware) check(ctx context.Context, policy Policy, dimension, identity string, limit int, window time.Duration) (bool, time.Duration, error) {
	now := m.clock().UTC()
	bucket := now.UnixMilli() / window.Milliseconds()
	key := "sec006:edge:" + string(policy.Class) + ":" + dimension + ":" + digestKey(identity) + ":" + strconv.FormatInt(bucket, 10)
	current, err := m.increment(ctx, key, window*2)
	if err != nil {
		return false, 0, err
	}
	reset := time.Duration((bucket+1)*window.Milliseconds()-now.UnixMilli()) * time.Millisecond
	return current <= limit, reset, nil
}

func (m *PolicyMiddleware) deny(w http.ResponseWriter, class EndpointClass, reason string, retry time.Duration, status int) {
	if m.observe != nil {
		m.observe(class, reason)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()+0.999))))
	}
	w.WriteHeader(status)
	code := "RATE_LIMITED"
	if status == http.StatusServiceUnavailable {
		code = "EDGE_POLICY_UNAVAILABLE"
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "request unavailable", "code": code})
}

// Handler enforces every registered policy by trusted client IP and, after
// authentication middleware has populated context, by actor ID.
func (m *PolicyMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy, ok := m.policyFor(r)
		if !ok {
			m.deny(w, ClassPublicRead, "unclassified_route", 0, http.StatusServiceUnavailable)
			return
		}
		if policy.Limit <= 0 || policy.Window <= 0 {
			m.deny(w, policy.Class, "invalid_policy", 0, http.StatusServiceUnavailable)
			return
		}
		identities := [][2]string{{"ip", validation.ExtractClientIP(r)}}
		if actor := auth.GetUserID(r.Context()); actor != "" {
			identities = append(identities, [2]string{"actor", actor})
		}
		for _, identity := range identities {
			allowed, retry, err := m.check(r.Context(), policy, identity[0], identity[1], policy.Limit, policy.Window)
			if err != nil {
				m.deny(w, policy.Class, "storage_unavailable", 0, http.StatusServiceUnavailable)
				return
			}
			if !allowed {
				m.deny(w, policy.Class, "window_exceeded", retry, http.StatusTooManyRequests)
				return
			}
			if policy.Burst > 0 && policy.BurstWindow > 0 {
				allowed, retry, err = m.check(r.Context(), policy, identity[0]+"_burst", identity[1], policy.Burst, policy.BurstWindow)
				if err != nil {
					m.deny(w, policy.Class, "storage_unavailable", 0, http.StatusServiceUnavailable)
					return
				}
				if !allowed {
					m.deny(w, policy.Class, "burst_exceeded", retry, http.StatusTooManyRequests)
					return
				}
			}
		}
		w.Header().Set("X-RateLimit-Policy", string(policy.Class))
		next.ServeHTTP(w, r)
	})
}

// ActorHandler is mounted after authentication on privileged and
// state-changing route groups. It adds a distributed per-actor dimension
// without trusting client-supplied identity.
func (m *PolicyMiddleware) ActorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		policy, ok := m.policyFor(r)
		if !ok || policy.Limit <= 0 || policy.Window <= 0 {
			m.deny(w, ClassPublicRead, "invalid_actor_policy", 0, http.StatusServiceUnavailable)
			return
		}
		actor := auth.GetUserID(r.Context())
		if actor == "" {
			m.deny(w, policy.Class, "actor_context_missing", 0, http.StatusUnauthorized)
			return
		}
		allowed, retry, err := m.check(r.Context(), policy, "actor", actor, policy.Limit, policy.Window)
		if err != nil {
			m.deny(w, policy.Class, "storage_unavailable", 0, http.StatusServiceUnavailable)
			return
		}
		if !allowed {
			m.deny(w, policy.Class, "window_exceeded", retry, http.StatusTooManyRequests)
			return
		}
		if policy.Burst > 0 && policy.BurstWindow > 0 {
			allowed, retry, err = m.check(r.Context(), policy, "actor_burst", actor, policy.Burst, policy.BurstWindow)
			if err != nil {
				m.deny(w, policy.Class, "storage_unavailable", 0, http.StatusServiceUnavailable)
				return
			}
			if !allowed {
				m.deny(w, policy.Class, "burst_exceeded", retry, http.StatusTooManyRequests)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// PoliciesForService is the canonical explicit endpoint-class registry.
func PoliciesForService(service string) []Policy {
	minute := time.Minute
	common := []Policy{
		{Method: http.MethodGet, PathPrefix: "/health", Class: ClassPublicRead, Limit: 300, Window: minute, Burst: 30, BurstWindow: time.Second},
		{Method: http.MethodGet, PathPrefix: "/ready", Class: ClassPublicRead, Limit: 300, Window: minute, Burst: 30, BurstWindow: time.Second},
		{PathPrefix: "/", Class: ClassPublicRead, Limit: 120, Window: minute, Burst: 20, BurstWindow: time.Second},
	}
	switch service {
	case "user":
		return append(common,
			Policy{Method: http.MethodPost, PathPrefix: "/api/user/auth/login", Class: ClassLogin, Limit: 10, Window: 10 * minute, Burst: 3, BurstWindow: minute, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/user/auth/register", Class: ClassRegistration, Limit: 5, Window: 10 * minute, Burst: 2, BurstWindow: minute, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/user/auth/send-otp", Class: ClassOTPRequest, Limit: 3, Window: 10 * minute, Burst: 1, BurstWindow: minute, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/user/auth/verify-otp", Class: ClassOTPVerify, Limit: 10, Window: 10 * minute, Burst: 3, BurstWindow: minute, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/user/auth/forgot-password", Class: ClassPasswordReset, Limit: 5, Window: 10 * minute, Burst: 2, BurstWindow: minute, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/user/contests/", Class: ClassContestJoin, Limit: 15, Window: minute, Burst: 3, BurstWindow: time.Second, HighRisk: true},
			Policy{PathPrefix: "/api/user/", Class: ClassPublicRead, Limit: 180, Window: minute, Burst: 30, BurstWindow: time.Second})
	case "admin":
		return append(common,
			Policy{Method: http.MethodPost, PathPrefix: "/api/admin/auth/login", Class: ClassLogin, Limit: 5, Window: 10 * minute, Burst: 2, BurstWindow: minute, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/admin/reauthenticate", Class: ClassAdmin, Limit: 5, Window: 5 * minute, Burst: 2, BurstWindow: minute, HighRisk: true},
			Policy{PathPrefix: "/api/admin/", Class: ClassAdmin, Limit: 120, Window: minute, Burst: 20, BurstWindow: time.Second, HighRisk: true})
	case "trade":
		return append(common,
			Policy{Method: http.MethodGet, PathPrefix: "/ws/", Class: ClassWebSocket, Limit: 20, Window: minute, Burst: 3, BurstWindow: time.Second, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/trade/orders", Class: ClassOrder, Limit: 60, Window: minute, Burst: 10, BurstWindow: time.Second, HighRisk: true},
			Policy{Method: http.MethodDelete, PathPrefix: "/api/trade/orders/", Class: ClassCancel, Limit: 90, Window: minute, Burst: 15, BurstWindow: time.Second, HighRisk: true},
			Policy{PathPrefix: "/api/trade/", Class: ClassPublicRead, Limit: 240, Window: minute, Burst: 40, BurstWindow: time.Second})
	case "payment":
		return append(common,
			Policy{Method: http.MethodPost, PathPrefix: "/webhooks/", Class: ClassWebhook, Limit: 120, Window: minute, Burst: 20, BurstWindow: time.Second, HighRisk: true},
			Policy{PathPrefix: "/callback/jibit", Class: ClassWebhook, Limit: 120, Window: minute, Burst: 20, BurstWindow: time.Second, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/payments/deposit/", Class: ClassDeposit, Limit: 10, Window: 10 * minute, Burst: 2, BurstWindow: minute, HighRisk: true},
			Policy{Method: http.MethodPost, PathPrefix: "/api/payments/withdraw/", Class: ClassWithdrawal, Limit: 5, Window: 10 * minute, Burst: 2, BurstWindow: minute, HighRisk: true},
			Policy{PathPrefix: "/api/", Class: ClassPublicRead, Limit: 180, Window: minute, Burst: 30, BurstWindow: time.Second})
	default:
		return common
	}
}
