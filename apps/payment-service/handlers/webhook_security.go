package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
)

const webhookTimestampHeader = "x-webhook-timestamp"

var (
	errWebhookTimestamp = errors.New("webhook timestamp rejected")
	errWebhookReplay    = errors.New("webhook replay rejected")
	errWebhookStore     = errors.New("webhook replay store unavailable")
)

type replayStore interface {
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
}

// WebhookSecurity enforces freshness and replay protection after provider
// signature verification and before any state mutation.
type WebhookSecurity struct {
	store            replayStore
	maxAge           time.Duration
	requireTimestamp bool
	clock            func() time.Time
}

func NewWebhookSecurity(store replayStore, maxAge time.Duration, requireTimestamp bool, clock func() time.Time) (*WebhookSecurity, error) {
	if store == nil || maxAge < time.Minute || maxAge > 30*time.Minute {
		return nil, errWebhookStore
	}
	if clock == nil {
		clock = time.Now
	}
	return &WebhookSecurity{store: store, maxAge: maxAge, requireTimestamp: requireTimestamp, clock: clock}, nil
}

func webhookTimestamp(headers map[string]string, body []byte) (time.Time, bool) {
	for _, name := range []string{webhookTimestampHeader, "x-nowpayments-timestamp"} {
		if value := strings.TrimSpace(headers[name]); value != "" {
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				return parsed, true
			}
			if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
				return time.Unix(unix, 0), true
			}
		}
	}
	var payload map[string]interface{}
	if json.Unmarshal(body, &payload) == nil {
		for _, name := range []string{"updated_at", "created_at", "timestamp"} {
			if value, ok := payload[name].(string); ok {
				if parsed, err := time.Parse(time.RFC3339, value); err == nil {
					return parsed, true
				}
			}
		}
	}
	return time.Time{}, false
}

func (s *WebhookSecurity) Validate(ctx context.Context, provider providers.ProviderType, headers map[string]string, body []byte, event *providers.WebhookEvent) error {
	now := s.clock().UTC()
	timestamp, present := webhookTimestamp(headers, body)
	if s.requireTimestamp && !present {
		return errWebhookTimestamp
	}
	if present {
		age := now.Sub(timestamp.UTC())
		if age < -time.Minute || age > s.maxAge {
			return errWebhookTimestamp
		}
	}
	identity := string(provider) + "|" + event.ProviderPaymentID + "|" + event.OrderID + "|" + string(event.Status)
	if identity == string(provider)+"|||" {
		return errWebhookReplay
	}
	digest := sha256.Sum256(append([]byte(identity+"|"), body...))
	key := "sec006:webhook:replay:" + hex.EncodeToString(digest[:])
	created, err := s.store.SetNX(ctx, key, "1", s.maxAge*2).Result()
	if err != nil {
		return errWebhookStore
	}
	if !created {
		return errWebhookReplay
	}
	return nil
}
