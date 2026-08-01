package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// mockDiscordServer creates a test server that simulates Discord webhook API.
func mockDiscordServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestNewDiscordNotifier(t *testing.T) {
	tests := []struct {
		name    string
		cfg     DiscordConfig
		wantErr error
	}{
		{
			name: "valid config",
			cfg: DiscordConfig{
				WebhookURL: "https://discord.com/api/webhooks/123/abc",
				Username:   "test-bot",
			},
			wantErr: nil,
		},
		{
			name:    "empty webhook URL",
			cfg:     DiscordConfig{},
			wantErr: ErrDiscordWebhookEmpty,
		},
		{
			name: "default username applied",
			cfg: DiscordConfig{
				WebhookURL: "https://discord.com/api/webhooks/123/abc",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			notifier, err := NewDiscordNotifier(tt.cfg, logger)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, notifier)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, notifier)
				assert.Equal(t, ChannelDiscord, notifier.Channel())
			}
		})
	}
}

func TestDiscordNotifier_SendMessage(t *testing.T) {
	var receivedBody map[string]interface{}
	var requestCount int32

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		err := json.NewDecoder(r.Body).Decode(&receivedBody)
		require.NoError(t, err)

		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = notifier.SendMessage(ctx, "Hello, Discord!")
	assert.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
	assert.Equal(t, "test-bot", receivedBody["username"])
	assert.Equal(t, "Hello, Discord!", receivedBody["content"])
}

func TestDiscordNotifier_SendEmbed(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&receivedBody)
		require.NoError(t, err)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	embed := DiscordEmbed{
		Title:       "Test Alert",
		Description: "This is a test alert",
		Color:       ColorCritical,
		Timestamp:   time.Now().UTC(),
		Footer:      "Test Footer",
		Fields: []DiscordEmbedField{
			{Name: "Field1", Value: "Value1", Inline: true},
			{Name: "Field2", Value: "Value2", Inline: false},
		},
	}

	ctx := context.Background()
	err = notifier.SendEmbed(ctx, embed)
	assert.NoError(t, err)

	// Verify embed was included
	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)

	embedData := embeds[0].(map[string]interface{})
	assert.Equal(t, "Test Alert", embedData["title"])
	assert.Equal(t, "This is a test alert", embedData["description"])
	assert.Equal(t, fmt.Sprintf("%d", ColorCritical), embedData["color"])
}

func TestDiscordNotifier_SendAlert(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&receivedBody)
		require.NoError(t, err)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	alert := Alert{
		ID:       "test-123",
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Critical Bug Detected",
		Message:  "Something went wrong",
		Service:  "trading-engine",
		Metadata: map[string]string{
			"environment": "production",
			"shard_id":    "shard-1",
			"error_code":  "E001",
		},
		Timestamp: time.Now().UTC(),
	}

	ctx := context.Background()
	err = notifier.SendAlert(ctx, alert)
	assert.NoError(t, err)

	// Verify embed structure
	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)

	embedData := embeds[0].(map[string]interface{})
	assert.Contains(t, embedData["title"], "Critical Bug Detected")
	assert.Equal(t, fmt.Sprintf("%d", ColorCritical), embedData["color"])

	// Verify fields
	fields := embedData["fields"].([]interface{})
	assert.GreaterOrEqual(t, len(fields), 3) // At least type, severity, service
}

func TestDiscordNotifier_SendInfo(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&receivedBody)
		require.NoError(t, err)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	info := Info{
		ID:        "info-123",
		Title:     "Deployment Complete",
		Message:   "Version 1.2.3 deployed successfully",
		Service:   "user-bff",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"version": "1.2.3",
		},
	}

	ctx := context.Background()
	err = notifier.SendInfo(ctx, info)
	assert.NoError(t, err)

	// Verify embed structure
	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)

	embedData := embeds[0].(map[string]interface{})
	assert.Equal(t, "Deployment Complete", embedData["title"])
	assert.Equal(t, fmt.Sprintf("%d", ColorInfo), embedData["color"])
}

func TestDiscordNotifier_RateLimiting(t *testing.T) {
	var requestCount int32

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Exhaust rate limit (30 messages)
	for i := 0; i < 30; i++ {
		err := notifier.SendMessage(ctx, "Test message")
		assert.NoError(t, err)
	}

	// 31st message should be rate limited
	err = notifier.SendMessage(ctx, "Test message")
	assert.ErrorIs(t, err, ErrDiscordRateLimited)
	assert.Equal(t, int32(30), atomic.LoadInt32(&requestCount))
}

func TestDiscordNotifier_CircuitBreaker(t *testing.T) {
	var requestCount int32

	// Server that always fails
	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Trigger failures to open circuit (max 3 failures)
	for i := 0; i < 3; i++ {
		_ = notifier.SendMessage(ctx, "Test message")
	}

	// Circuit should now be open
	err = notifier.SendMessage(ctx, "Test message")
	assert.ErrorIs(t, err, ErrDiscordCircuitOpen)

	// Verify state
	assert.Equal(t, circuitbreaker.StateOpen, notifier.CircuitBreakerState())
}

func TestDiscordNotifier_CircuitBreakerRecovery(t *testing.T) {
	var failRequests atomic.Bool
	failRequests.Store(true)

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		if failRequests.Load() {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Trigger failures to open circuit
	for i := 0; i < 3; i++ {
		_ = notifier.SendMessage(ctx, "Test message")
	}

	// Circuit should be open
	assert.Equal(t, circuitbreaker.StateOpen, notifier.CircuitBreakerState())

	// Reset circuit breaker manually (simulating timeout)
	notifier.ResetCircuitBreaker()
	failRequests.Store(false)

	// Should work now
	err = notifier.SendMessage(ctx, "Test message")
	assert.NoError(t, err)
}

func TestSeverityToColor(t *testing.T) {
	tests := []struct {
		severity Severity
		expected int
	}{
		{SeverityCritical, ColorCritical},
		{SeverityHigh, ColorHigh},
		{SeverityMedium, ColorMedium},
		{SeverityLow, ColorLow},
		{SeverityInfo, ColorInfo},
		{Severity("unknown"), ColorInfo}, // Default to info
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			assert.Equal(t, tt.expected, severityToColor(tt.severity))
		})
	}
}

func TestFormatFieldName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user_id", "User Id"},
		{"contest_id", "Contest Id"},
		{"shard_id", "Shard Id"},
		{"simple", "Simple"},
		{"UPPERCASE", "UPPERCASE"},
		{"mixed_CASE_test", "Mixed CASE Test"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatFieldName(tt.input))
		})
	}
}

func TestDiscordRateLimiter(t *testing.T) {
	limiter := newDiscordRateLimiter(5) // 5 requests per minute

	// Should allow first 5 requests
	for i := 0; i < 5; i++ {
		assert.True(t, limiter.allow(), "request %d should be allowed", i+1)
	}

	// 6th request should be denied
	assert.False(t, limiter.allow())
}

func TestDiscordRateLimiter_TokenRefill(t *testing.T) {
	limiter := newDiscordRateLimiter(60) // 60 requests per minute = 1 per second

	// Exhaust tokens
	for i := 0; i < 60; i++ {
		limiter.allow()
	}

	// Should be empty
	assert.False(t, limiter.allow())

	// Simulate time passing (manually update lastRefill)
	limiter.mu.Lock()
	limiter.lastRefill = time.Now().Add(-2 * time.Second)
	limiter.mu.Unlock()

	// Should have refilled ~2 tokens
	assert.True(t, limiter.allow())
	assert.True(t, limiter.allow())
}

func TestDiscordNotifier_ConcurrentAccess(t *testing.T) {
	var requestCount int32

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()
	var wg sync.WaitGroup

	// Send 20 concurrent messages (within rate limit)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = notifier.SendMessage(ctx, "Concurrent message")
		}(i)
	}

	wg.Wait()

	// All should succeed (within rate limit of 30)
	assert.Equal(t, int32(20), atomic.LoadInt32(&requestCount))
}

func TestDiscordNotifier_AlertToEmbed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: "https://example.com/webhook",
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	alert := Alert{
		ID:       "test-123",
		Type:     AlertTypeSystem,
		Severity: SeverityHigh,
		Title:    "System Alert",
		Message:  "CPU usage exceeded threshold",
		Service:  "market-ingestor",
		TraceID:  "trace-abc-123",
		Metadata: map[string]string{
			"environment": "production",
			"shard_id":    "shard-2",
			"cpu_usage":   "95%",
		},
		Timestamp: time.Now().UTC(),
	}

	embed := notifier.alertToEmbed(alert)

	assert.Contains(t, embed.Title, "System Alert")
	assert.Contains(t, embed.Title, "HIGH")
	assert.Equal(t, ColorHigh, embed.Color)
	assert.Equal(t, "CPU usage exceeded threshold", embed.Description)
	assert.NotEmpty(t, embed.Footer)
	assert.False(t, embed.Timestamp.IsZero())

	// Verify fields exist
	fieldNames := make([]string, len(embed.Fields))
	for i, f := range embed.Fields {
		fieldNames[i] = f.Name
	}

	assert.Contains(t, fieldNames, "Type")
	assert.Contains(t, fieldNames, "Severity")
	assert.Contains(t, fieldNames, "Service")
	assert.Contains(t, fieldNames, "Environment")
	assert.Contains(t, fieldNames, "Shard ID")
	assert.Contains(t, fieldNames, "Trace ID")
	assert.Contains(t, fieldNames, "Cpu Usage")
}

func TestDiscordNotifier_InfoToEmbed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: "https://example.com/webhook",
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	info := Info{
		ID:      "info-456",
		Title:   "Contest Started",
		Message: "Weekly trading contest has begun",
		Service: "admin-bff",
		Metadata: map[string]string{
			"contest_id": "contest-789",
			"duration":   "7 days",
		},
		Timestamp: time.Now().UTC(),
	}

	embed := notifier.infoToEmbed(info)

	assert.Equal(t, "Contest Started", embed.Title)
	assert.Equal(t, ColorInfo, embed.Color)
	assert.Equal(t, "Weekly trading contest has begun", embed.Description)
	assert.NotEmpty(t, embed.Footer)

	// Verify service field exists
	fieldNames := make([]string, len(embed.Fields))
	for i, f := range embed.Fields {
		fieldNames[i] = f.Name
	}

	assert.Contains(t, fieldNames, "Service")
	assert.Contains(t, fieldNames, "Contest Id")
	assert.Contains(t, fieldNames, "Duration")
}

func TestDiscordNotifier_SendBugAlert(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	alert := BugAlert(SeverityHigh, "Bug Found", "Null pointer exception")
	err = notifier.SendBugAlert(context.Background(), alert)
	assert.NoError(t, err)

	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)
}

func TestDiscordNotifier_SendSystemAlert(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	alert := SystemAlert(SeverityCritical, "System Down", "Database connection lost")
	err = notifier.SendSystemAlert(context.Background(), alert)
	assert.NoError(t, err)

	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)
}

func TestDiscordNotifier_SendContestAlert(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	alert := ContestAlert(SeverityMedium, "Contest Ending Soon", "Contest ends in 1 hour", "contest-123")
	err = notifier.SendContestAlert(context.Background(), alert)
	assert.NoError(t, err)

	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)
}

func TestDiscordNotifier_NilLogger(t *testing.T) {
	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	// Should create default logger when nil is passed
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, notifier)

	// Should work without errors
	err = notifier.SendMessage(context.Background(), "Test")
	assert.NoError(t, err)
}

func TestDiscordNotifier_ContextCancellation(t *testing.T) {
	// Server with delay
	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = notifier.SendMessage(ctx, "Test")
	// The circuit breaker will handle context cancellation
	assert.Error(t, err)
}

func TestDiscordNotifier_CircuitBreakerMetrics(t *testing.T) {
	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	// Initial state
	assert.Equal(t, circuitbreaker.StateClosed, notifier.CircuitBreakerState())

	// Send some messages
	for i := 0; i < 5; i++ {
		_ = notifier.SendMessage(context.Background(), "Test")
	}

	// Check metrics
	metrics := notifier.CircuitBreakerMetrics()
	assert.GreaterOrEqual(t, metrics.TotalRequests, int64(5))
	assert.GreaterOrEqual(t, metrics.TotalSuccesses, int64(5))
}

func TestDiscordNotifier_AvatarURL(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
		AvatarURL:  "https://example.com/avatar.png",
	}, logger)
	require.NoError(t, err)

	err = notifier.SendMessage(context.Background(), "Test")
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/avatar.png", receivedBody["avatar_url"])
}

// TestDiscordNotifier_SendMessage_Success tests successful message sending.
func TestDiscordNotifier_SendMessage_Success(t *testing.T) {
	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var payload map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)
		assert.Equal(t, "Test message", payload["content"])

		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
	}, zap.NewNop())
	require.NoError(t, err)

	err = notifier.SendMessage(context.Background(), "Test message")
	assert.NoError(t, err)
}

// TestDiscordNotifier_SendMessage_NetworkError tests handling of network errors.
func TestDiscordNotifier_SendMessage_NetworkError(t *testing.T) {
	// Use a server that immediately closes connections
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force close the connection to simulate network error
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = notifier.SendMessage(ctx, "Test message")

	// Should return an error (connection closed)
	assert.Error(t, err)
}

// TestDiscordNotifier_SendMessage_RateLimited tests rate limiting behavior.
func TestDiscordNotifier_SendMessage_RateLimited(t *testing.T) {
	var requestCount int32

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Exhaust rate limit (30 messages)
	for i := 0; i < 30; i++ {
		err := notifier.SendMessage(ctx, "Test message")
		require.NoError(t, err, "request %d should succeed", i+1)
	}

	// 31st message should be rate limited
	err = notifier.SendMessage(ctx, "Test message")
	assert.ErrorIs(t, err, ErrDiscordRateLimited)
	assert.Equal(t, int32(30), atomic.LoadInt32(&requestCount))
}

// TestDiscordNotifier_SendEmbed_ValidEmbed tests sending a valid embed message.
func TestDiscordNotifier_SendEmbed_ValidEmbed(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		err := json.NewDecoder(r.Body).Decode(&receivedBody)
		require.NoError(t, err)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	embed := DiscordEmbed{
		Title:       "Test Alert",
		Description: "This is a test alert",
		Color:       ColorCritical,
		Timestamp:   time.Now().UTC(),
		Footer:      "Test Footer",
		Fields: []DiscordEmbedField{
			{Name: "Field1", Value: "Value1", Inline: true},
			{Name: "Field2", Value: "Value2", Inline: false},
		},
	}

	ctx := context.Background()
	err = notifier.SendEmbed(ctx, embed)
	assert.NoError(t, err)

	// Verify embed was included
	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)

	embedData := embeds[0].(map[string]interface{})
	assert.Equal(t, "Test Alert", embedData["title"])
	assert.Equal(t, "This is a test alert", embedData["description"])
	assert.Equal(t, fmt.Sprintf("%d", ColorCritical), embedData["color"])

	// Verify fields are present
	fields := embedData["fields"].([]interface{})
	assert.Len(t, fields, 2)
}

// TestDiscordNotifier_SendBugAlert_FormatsCorrectly tests bug alert formatting.
func TestDiscordNotifier_SendBugAlert_FormatsCorrectly(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	alert := Alert{
		ID:       "bug-123",
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Critical Bug Found",
		Message:  "Null pointer exception in trading engine",
		Service:  "trading-engine",
		TraceID:  "trace-abc-123",
		SpanID:   "span-xyz-789",
		Metadata: map[string]string{
			"environment": "production",
			"shard_id":    "shard-1",
			"version":     "1.2.3",
		},
		Timestamp: time.Now().UTC(),
	}

	err = notifier.SendBugAlert(context.Background(), alert)
	assert.NoError(t, err)

	// Verify embed structure
	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)

	embedData := embeds[0].(map[string]interface{})

	// Verify title contains severity and original title
	assert.Contains(t, embedData["title"].(string), "HIGH")
	assert.Contains(t, embedData["title"].(string), "Critical Bug Found")

	// Verify color is correct for HIGH severity
	assert.Equal(t, fmt.Sprintf("%d", ColorHigh), embedData["color"])

	// Verify description
	assert.Equal(t, "Null pointer exception in trading engine", embedData["description"])

	// Verify fields exist
	fields := embedData["fields"].([]interface{})
	fieldNames := make(map[string]bool)
	for _, f := range fields {
		field := f.(map[string]interface{})
		fieldNames[field["name"].(string)] = true
	}

	assert.True(t, fieldNames["Type"], "Type field should exist")
	assert.True(t, fieldNames["Severity"], "Severity field should exist")
	assert.True(t, fieldNames["Service"], "Service field should exist")
	assert.True(t, fieldNames["Environment"], "Environment field should exist")
	assert.True(t, fieldNames["Shard ID"], "Shard ID field should exist")
	assert.True(t, fieldNames["Trace ID"], "Trace ID field should exist")
}

// TestDiscordNotifier_CircuitBreaker_TripsOnFailures tests circuit breaker trips after max failures.
func TestDiscordNotifier_CircuitBreaker_TripsOnFailures(t *testing.T) {
	var requestCount int32

	// Server that always fails
	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Trigger failures to open circuit (max 3 failures)
	for i := 0; i < 3; i++ {
		err := notifier.SendMessage(ctx, "Test message")
		assert.Error(t, err)
	}

	// Circuit should now be open
	assert.Equal(t, circuitbreaker.StateOpen, notifier.CircuitBreakerState())

	// Subsequent requests should fail fast with ErrDiscordCircuitOpen
	err = notifier.SendMessage(ctx, "Test message")
	assert.ErrorIs(t, err, ErrDiscordCircuitOpen)

	// Verify only 3 actual requests were made (not more)
	assert.Equal(t, int32(3), atomic.LoadInt32(&requestCount))
}

// TestDiscordNotifier_CircuitBreaker_RecoverAfterTimeout tests circuit breaker recovery.
func TestDiscordNotifier_CircuitBreaker_RecoverAfterTimeout(t *testing.T) {
	var failRequests atomic.Bool
	failRequests.Store(true)

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		if failRequests.Load() {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Trigger failures to open circuit
	for i := 0; i < 3; i++ {
		_ = notifier.SendMessage(ctx, "Test message")
	}

	// Circuit should be open
	assert.Equal(t, circuitbreaker.StateOpen, notifier.CircuitBreakerState())

	// Reset circuit breaker manually (simulating reset timeout elapsed)
	notifier.ResetCircuitBreaker()
	failRequests.Store(false)

	// Circuit should be closed and allow requests again
	assert.Equal(t, circuitbreaker.StateClosed, notifier.CircuitBreakerState())

	// Request should succeed now
	err = notifier.SendMessage(ctx, "Test message")
	assert.NoError(t, err)
}

// TestDiscordNotifier_RateLimiter_BlocksExcessRequests tests that rate limiter blocks excess requests.
func TestDiscordNotifier_RateLimiter_BlocksExcessRequests(t *testing.T) {
	limiter := newDiscordRateLimiter(10) // 10 requests per minute

	// Should allow first 10 requests
	for i := 0; i < 10; i++ {
		assert.True(t, limiter.allow(), "request %d should be allowed", i+1)
	}

	// 11th request should be blocked
	assert.False(t, limiter.allow(), "request 11 should be blocked")
	assert.False(t, limiter.allow(), "request 12 should be blocked")
}

// TestDiscordNotifier_ServerError tests handling of server errors (5xx).
func TestDiscordNotifier_ServerError(t *testing.T) {
	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = notifier.SendMessage(ctx, "Test message")

	// Should return an error for 5xx status
	assert.Error(t, err)
}

// TestDiscordNotifier_EmbedWithoutTimestamp tests embed without timestamp.
func TestDiscordNotifier_EmbedWithoutTimestamp(t *testing.T) {
	var receivedBody map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	embed := DiscordEmbed{
		Title:       "Test Alert",
		Description: "This is a test",
		Color:       ColorInfo,
		// No timestamp, no footer
	}

	err = notifier.SendEmbed(context.Background(), embed)
	assert.NoError(t, err)

	// Verify embed was sent
	embeds := receivedBody["embeds"].([]interface{})
	assert.Len(t, embeds, 1)

	embedData := embeds[0].(map[string]interface{})
	// Footer should be nil or not present since no timestamp and no footer text
	assert.Nil(t, embedData["footer"])
}

// TestDiscordNotifier_AllSeverityColors tests all severity levels produce correct colors.
func TestDiscordNotifier_AllSeverityColors(t *testing.T) {
	var receivedBodies []map[string]interface{}

	server := mockDiscordServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedBodies = append(receivedBodies, body)
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)
	require.NoError(t, err)

	severities := []struct {
		severity Severity
		color    int
	}{
		{SeverityCritical, ColorCritical},
		{SeverityHigh, ColorHigh},
		{SeverityMedium, ColorMedium},
		{SeverityLow, ColorLow},
		{SeverityInfo, ColorInfo},
	}

	for _, tc := range severities {
		alert := Alert{
			Type:      AlertTypeSystem,
			Severity:  tc.severity,
			Title:     fmt.Sprintf("%s Alert", tc.severity),
			Message:   "Test message",
			Timestamp: time.Now().UTC(),
		}

		err := notifier.SendAlert(context.Background(), alert)
		require.NoError(t, err, "failed to send %s alert", tc.severity)
	}

	// Verify colors
	for i, tc := range severities {
		embeds := receivedBodies[i]["embeds"].([]interface{})
		embedData := embeds[0].(map[string]interface{})
		assert.Equal(t, fmt.Sprintf("%d", tc.color), embedData["color"], "wrong color for %s", tc.severity)
	}
}

func BenchmarkDiscordNotifier_SendMessage(b *testing.B) {
	server := mockDiscordServer(&testing.T{}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	logger, _ := zap.NewProduction()
	notifier, _ := NewDiscordNotifier(DiscordConfig{
		WebhookURL: server.URL,
		Username:   "test-bot",
	}, logger)

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N && i < 30; i++ { // Limited by rate limit
		_ = notifier.SendMessage(ctx, "Benchmark message")
	}
}
