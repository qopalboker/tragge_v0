package handlers

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/redis/go-redis/v9"
)

type fixtureReplayStore struct {
	created bool
	err     error
	key     string
	ttl     time.Duration
}

func (store *fixtureReplayStore) SetNX(_ context.Context, key string, _ interface{}, ttl time.Duration) *redis.BoolCmd {
	store.key, store.ttl = key, ttl
	return redis.NewBoolResult(store.created, store.err)
}

func TestWebhookSecurityFreshnessReplayAndSafeKey(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	event := &providers.WebhookEvent{
		ProviderPaymentID: "provider-payment-fixture", OrderID: "order-fixture", Status: providers.PaymentStatusFinished,
	}
	body := []byte(`{"order_id":"order-fixture","status":"finished"}`)
	store := &fixtureReplayStore{created: true}
	security, err := NewWebhookSecurity(store, 5*time.Minute, true, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"x-webhook-timestamp": now.Format(time.RFC3339)}
	if err := security.Validate(context.Background(), providers.ProviderNowPayments, headers, body, event); err != nil {
		t.Fatalf("fresh signed event rejected: %v", err)
	}
	if !strings.HasPrefix(store.key, "sec006:webhook:replay:") || strings.Contains(store.key, event.OrderID) {
		t.Fatalf("webhook replay key is not safely digested: %q", store.key)
	}
	if store.ttl != 10*time.Minute {
		t.Fatalf("replay TTL=%s want=10m", store.ttl)
	}

	store.created = false
	if err := security.Validate(context.Background(), providers.ProviderNowPayments, headers, body, event); !errors.Is(err, errWebhookReplay) {
		t.Fatalf("replay accepted: %v", err)
	}
	store.created = true
	stale := map[string]string{"x-webhook-timestamp": now.Add(-6 * time.Minute).Format(time.RFC3339)}
	if err := security.Validate(context.Background(), providers.ProviderNowPayments, stale, body, event); !errors.Is(err, errWebhookTimestamp) {
		t.Fatalf("stale webhook accepted: %v", err)
	}
	if err := security.Validate(context.Background(), providers.ProviderNowPayments, nil, body, event); !errors.Is(err, errWebhookTimestamp) {
		t.Fatalf("missing production timestamp accepted: %v", err)
	}
	store.err = errors.New("fixture unavailable")
	if err := security.Validate(context.Background(), providers.ProviderNowPayments, headers, body, event); !errors.Is(err, errWebhookStore) {
		t.Fatalf("replay-store failure did not fail closed: %v", err)
	}
}

func TestWebhookSecurityAllowsMissingTimestampOnlyWhenExplicitlyNonProduction(t *testing.T) {
	store := &fixtureReplayStore{created: true}
	security, err := NewWebhookSecurity(store, 5*time.Minute, false, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	event := &providers.WebhookEvent{ProviderPaymentID: "fixture", OrderID: "order", Status: providers.PaymentStatusPending}
	if err := security.Validate(context.Background(), providers.ProviderNowPayments, nil, []byte(`{"fixture":true}`), event); err != nil {
		t.Fatalf("explicit non-production timestamp compatibility rejected: %v", err)
	}
}

func TestWebhookSecurityRedisReplayIntegration(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("SEC006_REDIS_ADDR"))
	if address == "" {
		t.Skip("SEC006_REDIS_ADDR is required for isolated runtime validation")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	defer client.Close()
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("isolated Redis unavailable: %v", err)
	}
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	security, err := NewWebhookSecurity(client, 5*time.Minute, true, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	event := &providers.WebhookEvent{ProviderPaymentID: "redis-fixture", OrderID: "order-fixture", Status: providers.PaymentStatusFinished}
	headers := map[string]string{"x-webhook-timestamp": now.Format(time.RFC3339)}
	body := []byte(`{"unique":"sec006-redis-replay-fixture"}`)
	if err := security.Validate(ctx, providers.ProviderNowPayments, headers, body, event); err != nil {
		t.Fatalf("first webhook rejected: %v", err)
	}
	if err := security.Validate(ctx, providers.ProviderNowPayments, headers, body, event); !errors.Is(err, errWebhookReplay) {
		t.Fatalf("Redis replay accepted: %v", err)
	}
}
