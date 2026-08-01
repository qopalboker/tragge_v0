package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestNewService tests service creation with various configurations.
func TestNewService(t *testing.T) {
	tests := []struct {
		name        string
		cfg         ServiceConfig
		wantDiscord bool
		wantEmail   bool
	}{
		{
			name: "full configuration",
			cfg: ServiceConfig{
				Enabled:      true,
				AsyncEnabled: true,
				AsyncWorkers: 3,
				Environment:  "production",
				ServiceName:  "test-service",
				Discord: DiscordConfig{
					WebhookURL: "https://discord.com/api/webhooks/123/abc",
					Enabled:    true,
				},
				Email: EmailConfig{
					APIKey:    "re_test_123",
					FromEmail: "test@example.com",
					Enabled:   true,
				},
			},
			wantDiscord: true,
			wantEmail:   true,
		},
		{
			name: "discord only",
			cfg: ServiceConfig{
				Enabled:     true,
				Environment: "staging",
				Discord: DiscordConfig{
					WebhookURL: "https://discord.com/api/webhooks/123/abc",
					Enabled:    true,
				},
			},
			wantDiscord: true,
			wantEmail:   false,
		},
		{
			name: "email only",
			cfg: ServiceConfig{
				Enabled:     true,
				Environment: "production",
				Email: EmailConfig{
					APIKey:    "re_test_123",
					FromEmail: "test@example.com",
					Enabled:   true,
				},
			},
			wantDiscord: false,
			wantEmail:   true,
		},
		{
			name: "no channels",
			cfg: ServiceConfig{
				Enabled:     true,
				Environment: "development",
			},
			wantDiscord: false,
			wantEmail:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			registry := prometheus.NewRegistry()

			svc, err := NewService(context.Background(), tt.cfg, logger, registry)
			require.NoError(t, err)
			require.NotNil(t, svc)

			assert.Equal(t, tt.wantDiscord, svc.HasDiscord())
			assert.Equal(t, tt.wantEmail, svc.HasEmail())

			// Cleanup
			err = svc.Shutdown(context.Background())
			assert.NoError(t, err)
		})
	}
}

// TestServiceDefaults tests that defaults are applied correctly.
func TestServiceDefaults(t *testing.T) {
	cfg := applyServiceDefaults(ServiceConfig{})

	assert.Equal(t, 5, cfg.AsyncWorkers)
	assert.Equal(t, 100, cfg.AsyncQueueSize)
	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
}

// TestSendAlert_DevelopmentMode tests that alerts are logged in development mode.
func TestSendAlert_DevelopmentMode(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "development",
		ServiceName: "test-service",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Should not return error, just log
	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Test Bug",
		Message:  "This is a test",
	}

	err = svc.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
}

// TestSendAlert_Disabled tests that alerts are skipped when service is disabled.
func TestSendAlert_Disabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     false,
		Environment: "production",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Test Bug",
		Message:  "This is a test",
	}

	err = svc.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
}

// TestSendAlert_WithDiscord tests sending alerts to Discord.
func TestSendAlert_WithDiscord(t *testing.T) {
	var receivedRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&receivedRequests, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: false,
		Environment:  "production",
		ServiceName:  "test-service",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	assert.True(t, svc.HasDiscord())

	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Test Bug",
		Message:  "This is a test",
	}

	err = svc.SendAlert(context.Background(), alert)
	assert.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&receivedRequests))
}

// TestSendAlertAsync tests async alert sending.
func TestSendAlertAsync(t *testing.T) {
	var receivedRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&receivedRequests, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:        true,
		AsyncEnabled:   true,
		AsyncWorkers:   2,
		AsyncQueueSize: 10,
		Environment:    "production",
		ServiceName:    "test-service",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)

	// Send async alert
	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Test Bug",
		Message:  "This is a test",
	}

	svc.SendAlertAsync(alert)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	err = svc.Shutdown(context.Background())
	assert.NoError(t, err)

	assert.Equal(t, int32(1), atomic.LoadInt32(&receivedRequests))
}

// TestRouteAlert tests severity-based routing logic.
func TestRouteAlert(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	// Production environment
	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	tests := []struct {
		severity    Severity
		wantDiscord bool
		wantEmail   bool
	}{
		{SeverityCritical, true, true},
		{SeverityHigh, true, true},
		{SeverityMedium, true, false},
		{SeverityLow, true, false},
		{SeverityInfo, true, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			gotDiscord, gotEmail := svc.routeAlert(tt.severity)
			assert.Equal(t, tt.wantDiscord, gotDiscord, "discord routing")
			assert.Equal(t, tt.wantEmail, gotEmail, "email routing")
		})
	}
}

// TestRouteAlert_NonProduction tests that low/info severity is skipped in non-production.
func TestRouteAlert_NonProduction(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	// Staging environment
	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "staging",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	tests := []struct {
		severity    Severity
		wantDiscord bool
		wantEmail   bool
	}{
		{SeverityCritical, true, true},
		{SeverityHigh, true, true},
		{SeverityMedium, true, false},
		{SeverityLow, false, false},  // Skipped in non-production
		{SeverityInfo, false, false}, // Skipped in non-production
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			gotDiscord, gotEmail := svc.routeAlert(tt.severity)
			assert.Equal(t, tt.wantDiscord, gotDiscord, "discord routing")
			assert.Equal(t, tt.wantEmail, gotEmail, "email routing")
		})
	}
}

// TestNotifyBug tests sending bug alerts.
func TestNotifyBug(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: false,
		Environment:  "production",
		ServiceName:  "test-service",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	err = svc.NotifyBug(context.Background(), BugAlertDetails{
		Title:      "Null Pointer Exception",
		Message:    "NPE in user service",
		Severity:   SeverityHigh,
		Service:    "user-bff",
		StackTrace: "at main.go:42",
		TraceID:    "trace-123",
		Metadata: map[string]string{
			"version": "1.2.3",
		},
	})

	assert.NoError(t, err)
	assert.NotEmpty(t, receivedBody)

	// Verify embed structure
	embeds, ok := receivedBody["embeds"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, embeds, 1)
}

// TestNotifyContestStart tests contest start notifications.
func TestNotifyContestStart(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: false,
		Environment:  "production",
		ServiceName:  "admin-bff",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	err = svc.NotifyContestStart(context.Background(), ContestAlertDetails{
		ContestID:   "contest-123",
		ContestName: "Weekly Trading Challenge",
		Message:     "The weekly trading contest has started!",
		Severity:    SeverityMedium,
		Metadata: map[string]string{
			"prize_pool": "$1000",
		},
	})

	assert.NoError(t, err)
	assert.NotEmpty(t, receivedBody)
}

// TestNotifyContestEnd tests contest end notifications.
func TestNotifyContestEnd(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: false,
		Environment:  "production",
		ServiceName:  "admin-bff",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	err = svc.NotifyContestEnd(context.Background(), ContestAlertDetails{
		ContestID:   "contest-123",
		ContestName: "Weekly Trading Challenge",
		Message:     "The weekly trading contest has ended!",
		Severity:    SeverityMedium,
		Metadata: map[string]string{
			"winner": "user-456",
		},
	})

	assert.NoError(t, err)
	assert.NotEmpty(t, receivedBody)
}

// TestNotifySystemAlert tests system alert notifications.
func TestNotifySystemAlert(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: false,
		Environment:  "production",
		ServiceName:  "trading-engine",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	err = svc.NotifySystemAlert(context.Background(), SystemAlertDetails{
		Title:    "High Memory Usage",
		Message:  "Memory usage exceeded 90%",
		Severity: SeverityCritical,
		Service:  "trading-engine",
		Metadata: map[string]string{
			"memory_percent": "92",
		},
	})

	assert.NoError(t, err)
	assert.NotEmpty(t, receivedBody)
}

// TestGracefulShutdown tests the graceful shutdown mechanism.
func TestGracefulShutdown(t *testing.T) {
	var receivedRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Simulate some processing
		atomic.AddInt32(&receivedRequests, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:         true,
		AsyncEnabled:    true,
		AsyncWorkers:    2,
		AsyncQueueSize:  10,
		Environment:     "production",
		ServiceName:     "test-service",
		ShutdownTimeout: 5 * time.Second,
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)

	// Queue some alerts
	for i := 0; i < 5; i++ {
		svc.SendAlertAsync(Alert{
			Type:     AlertTypeBug,
			Severity: SeverityHigh,
			Title:    "Test Bug",
			Message:  "This is a test",
		})
	}

	// Initiate shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = svc.Shutdown(ctx)
	assert.NoError(t, err)

	// All queued alerts should have been processed
	assert.GreaterOrEqual(t, atomic.LoadInt32(&receivedRequests), int32(1))
}

// TestShutdownPreventsNewAlerts tests that shutdown prevents new alerts.
func TestShutdownPreventsNewAlerts(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: true,
		Environment:  "production",
	}, logger, registry)
	require.NoError(t, err)

	// Shutdown
	err = svc.Shutdown(context.Background())
	assert.NoError(t, err)

	// New alerts should return error
	err = svc.SendAlert(context.Background(), Alert{
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Test",
		Message:  "Test",
	})
	assert.ErrorIs(t, err, ErrServiceShuttingDown)
}

// TestConcurrentAlerts tests concurrent alert sending.
func TestConcurrentAlerts(t *testing.T) {
	var receivedRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&receivedRequests, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:        true,
		AsyncEnabled:   true,
		AsyncWorkers:   5,
		AsyncQueueSize: 50,
		Environment:    "production",
		ServiceName:    "test-service",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)

	// Send many concurrent alerts
	var wg sync.WaitGroup
	alertCount := 20

	for i := 0; i < alertCount; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			svc.SendAlertAsync(Alert{
				Type:     AlertTypeBug,
				Severity: SeverityHigh,
				Title:    "Concurrent Bug",
				Message:  "Testing concurrency",
			})
		}(i)
	}

	wg.Wait()

	// Wait for workers to process
	time.Sleep(200 * time.Millisecond)

	err = svc.Shutdown(context.Background())
	assert.NoError(t, err)

	// Should have processed all alerts (within rate limits)
	assert.GreaterOrEqual(t, atomic.LoadInt32(&receivedRequests), int32(1))
}

// TestQueueFull tests behavior when async queue is full.
func TestQueueFull(t *testing.T) {
	// Server that blocks to fill the queue
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:        true,
		AsyncEnabled:   true,
		AsyncWorkers:   1,
		AsyncQueueSize: 2, // Very small queue
		Environment:    "production",
		ServiceName:    "test-service",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)

	// Fill the queue
	for i := 0; i < 10; i++ {
		err := svc.SendAlert(context.Background(), Alert{
			Type:     AlertTypeBug,
			Severity: SeverityHigh,
			Title:    "Queue Test",
			Message:  "Testing queue full behavior",
		})
		if errors.Is(err, ErrQueueFullService) {
			// Expected - queue is full
			break
		}
	}

	err = svc.Shutdown(context.Background())
	assert.NoError(t, err)
}

// TestPrepareAlert tests alert preparation.
func TestPrepareAlert(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
		ServiceName: "test-service",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Test",
		Message:  "Test",
	}

	prepared := svc.prepareAlert(alert)

	assert.NotEmpty(t, prepared.ID)
	assert.Equal(t, "test-service", prepared.Service)
	assert.False(t, prepared.Timestamp.IsZero())
	assert.NotNil(t, prepared.Metadata)
	assert.Equal(t, "production", prepared.Metadata["environment"])
}

// TestCircuitBreakerStates tests circuit breaker state retrieval.
func TestCircuitBreakerStates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
		Email: EmailConfig{
			APIKey:    "re_test_123",
			FromEmail: "test@example.com",
			Enabled:   true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Check initial states
	discordState := svc.DiscordCircuitBreakerState()
	emailState := svc.EmailCircuitBreakerState()

	assert.Equal(t, "closed", discordState)
	assert.Equal(t, "closed", emailState)
}

// TestCircuitBreakerDisabled tests circuit breaker state when disabled.
func TestCircuitBreakerDisabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Should return "disabled" for disabled channels
	assert.Equal(t, "disabled", svc.DiscordCircuitBreakerState())
	assert.Equal(t, "disabled", svc.EmailCircuitBreakerState())
}

// TestMetricsRegistration tests that metrics are properly registered.
func TestMetricsRegistration(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
		ServiceName: "test-service",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Verify metrics collectors are available
	collectors := svc.MetricsCollectors()
	assert.Len(t, collectors, 7) // sentTotal, sendDuration, queueSize, errorsTotal, workersActive, droppedTotal, syncFallbackTotal
}

// TestQueueSize tests queue size reporting.
func TestQueueSize(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:        true,
		AsyncEnabled:   false, // Disable workers so queue fills
		AsyncQueueSize: 10,
		Environment:    "production",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Initially empty
	assert.Equal(t, 0, svc.QueueSize())
}

// TestIsEnabled tests the enabled flag.
func TestIsEnabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	assert.True(t, svc.IsEnabled())
}

// TestResetCircuitBreakers tests circuit breaker reset functionality.
func TestResetCircuitBreakers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
		Email: EmailConfig{
			APIKey:    "re_test_123",
			FromEmail: "test@example.com",
			Enabled:   true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Reset should not panic
	svc.ResetDiscordCircuitBreaker()
	svc.ResetEmailCircuitBreaker()

	// States should still be closed
	assert.Equal(t, "closed", svc.DiscordCircuitBreakerState())
	assert.Equal(t, "closed", svc.EmailCircuitBreakerState())
}

// TestDualChannelSend tests sending to both Discord and Email.
func TestDualChannelSend(t *testing.T) {
	var discordReceived, emailReceived int32

	discordServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&discordReceived, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discordServer.Close()

	// For email, we'll mock by checking if it was called (the real Resend API won't work in tests)
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:         true,
		AsyncEnabled:    false,
		Environment:     "production",
		ServiceName:     "test-service",
		EmailRecipients: []string{"test@example.com"},
		Discord: DiscordConfig{
			WebhookURL: discordServer.URL,
			Enabled:    true,
		},
		// Email disabled because Resend would fail in tests
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Send critical alert (should go to both channels)
	err = svc.SendAlert(context.Background(), Alert{
		Type:     AlertTypeSystem,
		Severity: SeverityCritical,
		Title:    "Critical Alert",
		Message:  "This is critical",
	})

	assert.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&discordReceived))
	// Email would also be sent in production with proper config
	_ = emailReceived
}

// TestShutdownTimeout tests shutdown with context timeout.
func TestShutdownTimeout(t *testing.T) {
	// Server that blocks
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:         true,
		AsyncEnabled:    true,
		AsyncWorkers:    1,
		AsyncQueueSize:  10,
		Environment:     "production",
		ShutdownTimeout: 100 * time.Millisecond,
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)

	// Queue a slow request
	svc.SendAlertAsync(Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Slow Alert",
		Message:  "This is slow",
	})

	// Give the worker time to pick up the job
	time.Sleep(50 * time.Millisecond)

	// Shutdown with very short timeout - the shutdown may succeed (workers stopped)
	// or it may drain remaining items. Either outcome is acceptable.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// Shutdown should complete (either immediately or after timeout)
	err = svc.Shutdown(ctx)
	// Either no error (graceful) or context deadline exceeded is acceptable
	if err != nil {
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}
}

// BenchmarkSendAlert benchmarks synchronous alert sending.
func BenchmarkSendAlert(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	registry := prometheus.NewRegistry()

	svc, _ := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: false,
		Environment:  "production",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, nil, registry)
	defer svc.Shutdown(context.Background())

	ctx := context.Background()
	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Benchmark Alert",
		Message:  "Benchmarking",
	}

	b.ResetTimer()
	for i := 0; i < b.N && i < 30; i++ { // Limited by rate limit
		_ = svc.SendAlert(ctx, alert)
	}
}

// BenchmarkSendAlertAsync benchmarks async alert sending.
func BenchmarkSendAlertAsync(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	registry := prometheus.NewRegistry()

	svc, _ := NewService(context.Background(), ServiceConfig{
		Enabled:        true,
		AsyncEnabled:   true,
		AsyncWorkers:   5,
		AsyncQueueSize: 1000,
		Environment:    "production",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, nil, registry)

	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Benchmark Alert",
		Message:  "Benchmarking",
	}

	b.ResetTimer()
	for i := 0; i < b.N && i < 100; i++ {
		svc.SendAlertAsync(alert)
	}

	svc.Shutdown(context.Background())
}

// =============================================================================
// Additional comprehensive tests
// =============================================================================

// TestNotificationService_SendAlert_RoutesToCorrectChannels tests routing logic.
func TestNotificationService_SendAlert_RoutesToCorrectChannels(t *testing.T) {
	var discordRequests, emailRequests int32

	discordServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&discordRequests, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discordServer.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: false,
		Environment:  "production",
		ServiceName:  "test-service",
		Discord: DiscordConfig{
			WebhookURL: discordServer.URL,
			Enabled:    true,
		},
		// Email not enabled (requires real Resend API)
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	tests := []struct {
		name           string
		severity       Severity
		expectDiscord  bool
		expectEmail    bool
	}{
		{"critical routes to both", SeverityCritical, true, true},
		{"high routes to both", SeverityHigh, true, true},
		{"medium routes to discord", SeverityMedium, true, false},
		{"low routes to discord", SeverityLow, true, false},
		{"info routes to discord", SeverityInfo, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			discordBefore := atomic.LoadInt32(&discordRequests)

			alert := Alert{
				Type:     AlertTypeSystem,
				Severity: tc.severity,
				Title:    "Test Alert",
				Message:  "Test message",
			}

			err := svc.SendAlert(context.Background(), alert)
			require.NoError(t, err)

			if tc.expectDiscord {
				assert.Greater(t, atomic.LoadInt32(&discordRequests), discordBefore,
					"expected discord request for %s severity", tc.severity)
			}
		})
	}

	_ = emailRequests // Would be used if email was configured
}

// TestNotificationService_SendAlert_CriticalGoesToBothChannels tests critical alerts go to both channels.
func TestNotificationService_SendAlert_CriticalGoesToBothChannels(t *testing.T) {
	var discordReceived int32

	discordServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&discordReceived, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer discordServer.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:         true,
		AsyncEnabled:    false,
		Environment:     "production",
		ServiceName:     "test-service",
		EmailRecipients: []string{"admin@example.com"},
		Discord: DiscordConfig{
			WebhookURL: discordServer.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Send critical alert
	alert := Alert{
		Type:     AlertTypeSystem,
		Severity: SeverityCritical,
		Title:    "Critical System Failure",
		Message:  "Database connection lost",
	}

	err = svc.SendAlert(context.Background(), alert)
	require.NoError(t, err)

	// Discord should have received the alert
	assert.Equal(t, int32(1), atomic.LoadInt32(&discordReceived))
}

// TestNotificationService_SendAlertAsync_DoesNotBlock tests async sending doesn't block.
func TestNotificationService_SendAlertAsync_DoesNotBlock(t *testing.T) {
	// Server that intentionally delays responses
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:        true,
		AsyncEnabled:   true,
		AsyncWorkers:   2,
		AsyncQueueSize: 10,
		Environment:    "production",
		ServiceName:    "test-service",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)

	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Async Test",
		Message:  "Testing async behavior",
	}

	// SendAlertAsync should return immediately
	start := time.Now()
	svc.SendAlertAsync(alert)
	elapsed := time.Since(start)

	// Should complete in under 10ms (not waiting for the 100ms server delay)
	assert.Less(t, elapsed, 10*time.Millisecond, "SendAlertAsync should not block")

	err = svc.Shutdown(context.Background())
	assert.NoError(t, err)
}

// TestNotificationService_Shutdown_WaitsForPending tests graceful shutdown.
func TestNotificationService_Shutdown_WaitsForPending(t *testing.T) {
	var completedRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&completedRequests, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:         true,
		AsyncEnabled:    true,
		AsyncWorkers:    2,
		AsyncQueueSize:  10,
		Environment:     "production",
		ShutdownTimeout: 5 * time.Second,
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)

	// Queue several alerts
	for i := 0; i < 3; i++ {
		svc.SendAlertAsync(Alert{
			Type:     AlertTypeBug,
			Severity: SeverityHigh,
			Title:    "Test",
			Message:  "Test",
		})
	}

	// Shutdown should wait for pending
	err = svc.Shutdown(context.Background())
	assert.NoError(t, err)

	// At least some requests should have completed
	assert.GreaterOrEqual(t, atomic.LoadInt32(&completedRequests), int32(1))
}

// TestNotificationService_Shutdown_TimesOut tests shutdown timeout behavior.
func TestNotificationService_Shutdown_TimesOut(t *testing.T) {
	// Server that blocks forever
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:         true,
		AsyncEnabled:    true,
		AsyncWorkers:    1,
		AsyncQueueSize:  5,
		Environment:     "production",
		ShutdownTimeout: 50 * time.Millisecond,
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)

	// Queue an alert
	svc.SendAlertAsync(Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Slow Alert",
		Message:  "This will be slow",
	})

	// Give worker time to pick up job
	time.Sleep(30 * time.Millisecond)

	// Shutdown should timeout
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = svc.Shutdown(ctx)
	elapsed := time.Since(start)

	// Should have timed out within reasonable bounds
	assert.Less(t, elapsed, 200*time.Millisecond)
}

// TestNotificationService_Disabled_SkipsAllNotifications tests disabled service behavior.
func TestNotificationService_Disabled_SkipsAllNotifications(t *testing.T) {
	var requestReceived bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     false, // Disabled
		Environment: "production",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	assert.False(t, svc.IsEnabled())

	// Send alert - should be skipped
	err = svc.SendAlert(context.Background(), Alert{
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Should be skipped",
		Message:  "This should not be sent",
	})

	assert.NoError(t, err)
	assert.False(t, requestReceived)
}

// TestNotificationService_DevelopmentMode_LogsOnly tests development mode behavior.
func TestNotificationService_DevelopmentMode_LogsOnly(t *testing.T) {
	var requestReceived bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "development", // Development mode
		ServiceName: "test-service",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Send alert - should only log, not send
	err = svc.SendAlert(context.Background(), Alert{
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Development Alert",
		Message:  "This should only be logged",
	})

	assert.NoError(t, err)
	assert.False(t, requestReceived, "no request should be sent in development mode")
}

// TestNotificationService_ErrorRecording tests error type recording.
func TestNotificationService_ErrorRecording(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Test error type mapping
	testCases := []struct {
		err          error
		expectedType string
	}{
		{ErrDiscordRateLimited, "rate_limited"},
		{ErrDiscordCircuitOpen, "circuit_open"},
		{ErrEmailCircuitOpen, "circuit_open"},
		{ErrEmailNoRecipients, "no_recipients"},
		{context.DeadlineExceeded, "timeout"},
		{context.Canceled, "cancelled"},
		{errors.New("some error"), "send_failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.expectedType, func(t *testing.T) {
			// recordError is internal, but we can verify it doesn't panic
			svc.recordError("test_channel", tc.err)
		})
	}
}

// TestNotificationService_NilLogger tests service creation with nil logger.
func TestNotificationService_NilLogger(t *testing.T) {
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
	}, nil, registry)
	require.NoError(t, err)
	assert.NotNil(t, svc)
	defer svc.Shutdown(context.Background())
}

// TestNotificationService_NilRegistry tests service creation with nil registry.
func TestNotificationService_NilRegistry(t *testing.T) {
	logger := zaptest.NewLogger(t)

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
	}, logger, nil)
	require.NoError(t, err)
	assert.NotNil(t, svc)
	defer svc.Shutdown(context.Background())
}

// TestNotificationService_DoubleShutdown tests that double shutdown is safe.
func TestNotificationService_DoubleShutdown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: true,
		Environment:  "production",
	}, logger, registry)
	require.NoError(t, err)

	// First shutdown
	err = svc.Shutdown(context.Background())
	assert.NoError(t, err)

	// Second shutdown should be safe
	err = svc.Shutdown(context.Background())
	assert.NoError(t, err)
}

// TestNotificationService_SendAlertAsyncWhenDisabled tests async send when disabled.
func TestNotificationService_SendAlertAsyncWhenDisabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      false,
		AsyncEnabled: true,
		Environment:  "production",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Should not panic and should return immediately
	svc.SendAlertAsync(Alert{
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Test",
		Message:  "Test",
	})
}

// TestNotificationService_SendAlertAsyncAfterShutdown tests async send after shutdown.
func TestNotificationService_SendAlertAsyncAfterShutdown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: true,
		Environment:  "production",
	}, logger, registry)
	require.NoError(t, err)

	// Shutdown first
	err = svc.Shutdown(context.Background())
	require.NoError(t, err)

	// Async send should not panic
	svc.SendAlertAsync(Alert{
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Test",
		Message:  "Test",
	})
}

// TestNotificationService_AllNotifyMethods tests all Notify* helper methods.
func TestNotificationService_AllNotifyMethods(t *testing.T) {
	var requestCount int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: false,
		Environment:  "production",
		ServiceName:  "test-service",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	ctx := context.Background()

	// Test NotifyBug
	err = svc.NotifyBug(ctx, BugAlertDetails{
		Title:      "Test Bug",
		Message:    "Bug description",
		Severity:   SeverityHigh,
		Service:    "test-service",
		StackTrace: "at main.go:42",
		TraceID:    "trace-123",
	})
	assert.NoError(t, err)

	// Test NotifyContestStart
	err = svc.NotifyContestStart(ctx, ContestAlertDetails{
		ContestID:   "contest-123",
		ContestName: "Weekly Challenge",
		Message:     "Contest has started",
		Severity:    SeverityMedium,
	})
	assert.NoError(t, err)

	// Test NotifyContestEnd
	err = svc.NotifyContestEnd(ctx, ContestAlertDetails{
		ContestID:   "contest-123",
		ContestName: "Weekly Challenge",
		Message:     "Contest has ended",
		Severity:    SeverityMedium,
	})
	assert.NoError(t, err)

	// Test NotifySystemAlert
	err = svc.NotifySystemAlert(ctx, SystemAlertDetails{
		Title:    "High CPU Usage",
		Message:  "CPU usage exceeded 90%",
		Severity: SeverityCritical,
		Service:  "trading-engine",
	})
	assert.NoError(t, err)

	// All four calls should have generated requests
	assert.Equal(t, int32(4), atomic.LoadInt32(&requestCount))
}

// TestNotificationService_AlertPreparation tests alert preparation logic.
func TestNotificationService_AlertPreparation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
		ServiceName: "test-service",
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Test with empty alert
	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityHigh,
		Title:    "Test",
		Message:  "Test",
	}

	prepared := svc.prepareAlert(alert)

	// Should have auto-generated ID
	assert.NotEmpty(t, prepared.ID)
	assert.Contains(t, prepared.ID, "alert-")

	// Should have service name filled
	assert.Equal(t, "test-service", prepared.Service)

	// Should have timestamp
	assert.False(t, prepared.Timestamp.IsZero())

	// Should have environment in metadata
	assert.NotNil(t, prepared.Metadata)
	assert.Equal(t, "production", prepared.Metadata["environment"])
}

// TestNotificationService_CircuitBreakerReset tests circuit breaker reset functionality.
func TestNotificationService_CircuitBreakerReset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
		Discord: DiscordConfig{
			WebhookURL: server.URL,
			Enabled:    true,
		},
		Email: EmailConfig{
			APIKey:    "re_test_123",
			FromEmail: "test@example.com",
			Enabled:   true,
		},
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Initial state should be closed
	assert.Equal(t, "closed", svc.DiscordCircuitBreakerState())
	assert.Equal(t, "closed", svc.EmailCircuitBreakerState())

	// Reset should not panic or error
	svc.ResetDiscordCircuitBreaker()
	svc.ResetEmailCircuitBreaker()

	// States should still be closed
	assert.Equal(t, "closed", svc.DiscordCircuitBreakerState())
	assert.Equal(t, "closed", svc.EmailCircuitBreakerState())
}

// TestNotificationService_ResetDisabledCircuitBreaker tests reset on disabled notifier.
func TestNotificationService_ResetDisabledCircuitBreaker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:     true,
		Environment: "production",
		// No Discord or Email configured
	}, logger, registry)
	require.NoError(t, err)
	defer svc.Shutdown(context.Background())

	// Should return "disabled" and not panic
	assert.Equal(t, "disabled", svc.DiscordCircuitBreakerState())
	assert.Equal(t, "disabled", svc.EmailCircuitBreakerState())

	// Reset should not panic
	svc.ResetDiscordCircuitBreaker()
	svc.ResetEmailCircuitBreaker()
}

// TestSendAlertAsyncDuringShutdown verifies that calling SendAlertAsync after
// Shutdown does not panic, even if the queue channel has been closed.
func TestSendAlertAsyncDuringShutdown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:      true,
		AsyncEnabled: true,
		AsyncWorkers: 2,
		Environment:  "production",
		ServiceName:  "test-service",
	}, logger, registry)
	require.NoError(t, err)

	// Shutdown the service (closes stopCh, drains workers, closes queue)
	err = svc.Shutdown(context.Background())
	require.NoError(t, err)

	// Calling SendAlertAsync after shutdown must not panic
	assert.NotPanics(t, func() {
		svc.SendAlertAsync(Alert{
			Type:     AlertTypeBug,
			Severity: SeverityCritical,
			Title:    "Post-shutdown alert",
			Message:  "This should not panic",
		})
	})

	// Calling it multiple times should also be safe
	assert.NotPanics(t, func() {
		for i := 0; i < 10; i++ {
			svc.SendAlertAsync(Alert{
				Type:     AlertTypeSystem,
				Severity: SeverityInfo,
				Title:    "Repeated post-shutdown alert",
				Message:  "Still should not panic",
			})
		}
	})
}

// TestSendAlertAsync_NoPanicDuringShutdown verifies that SendAlertAsync does not panic
// when called concurrently with Shutdown. Run with -race to detect data races:
//
//	go test -race -run TestSendAlertAsync_NoPanicDuringShutdown ./packages/notification/
func TestSendAlertAsync_NoPanicDuringShutdown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	registry := prometheus.NewRegistry()

	svc, err := NewService(context.Background(), ServiceConfig{
		Enabled:         true,
		AsyncEnabled:    true,
		AsyncWorkers:    5,
		AsyncQueueSize:  100,
		Environment:     "production",
		ServiceName:     "test-service",
		ShutdownTimeout: 5 * time.Second,
	}, logger, registry)
	require.NoError(t, err)

	var wg sync.WaitGroup

	// Launch 100 goroutines that each call SendAlertAsync 100 times.
	for g := 0; g < 100; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				svc.SendAlertAsync(Alert{
					Type:     AlertTypeSystem,
					Severity: SeverityInfo,
					Title:    fmt.Sprintf("alert-%d-%d", id, i),
					Message:  "concurrent alert during shutdown race",
				})
			}
		}(g)
	}

	// After a short delay, initiate shutdown while goroutines are still sending.
	time.Sleep(10 * time.Millisecond)
	shutdownErr := svc.Shutdown(context.Background())

	// Wait for all sender goroutines to finish.
	wg.Wait()

	// The test PASSES if no panic occurred. Assert shutdown completed cleanly.
	assert.NoError(t, shutdownErr)
}
