// Package integration provides integration tests for the notification system.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"go.uber.org/zap/zaptest"
)

// ============================================================================
// Test Helpers
// ============================================================================

// MockDiscordServer creates a mock Discord webhook server for testing.
type MockDiscordServer struct {
	Server         *httptest.Server
	ReceivedCount  atomic.Int64
	ReceivedAlerts []notification.Alert
	ReceivedInfos  []notification.Info
	mu             sync.Mutex
	ShouldFail     bool
	FailureCode    int
	Delay          time.Duration
}

// NewMockDiscordServer creates a new mock Discord webhook server.
func NewMockDiscordServer() *MockDiscordServer {
	mds := &MockDiscordServer{
		ReceivedAlerts: make([]notification.Alert, 0),
		ReceivedInfos:  make([]notification.Info, 0),
	}

	mds.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mds.Delay > 0 {
			time.Sleep(mds.Delay)
		}

		if mds.ShouldFail {
			code := mds.FailureCode
			if code == 0 {
				code = http.StatusInternalServerError
			}
			w.WriteHeader(code)
			return
		}

		mds.ReceivedCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))

	return mds
}

// Close shuts down the mock server.
func (m *MockDiscordServer) Close() {
	m.Server.Close()
}

// Reset clears all received data.
func (m *MockDiscordServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReceivedCount.Store(0)
	m.ReceivedAlerts = m.ReceivedAlerts[:0]
	m.ReceivedInfos = m.ReceivedInfos[:0]
	m.ShouldFail = false
	m.FailureCode = 0
	m.Delay = 0
}

// loadTestFixture loads a JSON test fixture from testdata directory.
func loadTestFixture(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("testdata/%s", filename))
	if err != nil {
		t.Fatalf("Failed to load test fixture %s: %v", filename, err)
	}
	return data
}

// ============================================================================
// Real Discord Integration Tests (requires environment variable)
// ============================================================================

// TestNotification_Discord_RealWebhook tests against a real Discord webhook.
// Requires DISCORD_TEST_WEBHOOK_URL environment variable.
// Use a dedicated test channel to avoid spam in production channels.
func TestNotification_Discord_RealWebhook(t *testing.T) {
	webhookURL := os.Getenv("DISCORD_TEST_WEBHOOK_URL")
	if webhookURL == "" {
		t.Skip("DISCORD_TEST_WEBHOOK_URL not set - skipping real Discord webhook test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := zaptest.NewLogger(t)

	// Create Discord notifier with real webhook
	discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
		WebhookURL: webhookURL,
		Username:   "tragge-integration-test",
		Enabled:    true,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create Discord notifier: %v", err)
	}

	// Create notification service
	svc, err := notification.NewWithLogger(notification.Config{
		Service:            "integration-test",
		Enabled:            true,
		Async:              false, // Use sync for deterministic testing
		RateLimitPerMinute: 60,
		MinSeverity:        notification.SeverityInfo,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create notification service: %v", err)
	}
	defer svc.Shutdown(ctx)

	svc.RegisterSender(discordNotifier)

	t.Run("SendAlert_Critical", func(t *testing.T) {
		alert := notification.SystemAlert(
			notification.SeverityCritical,
			"[TEST] Critical Alert",
			"This is an integration test alert - please ignore",
		).WithMetadata("test_run_id", t.Name())

		err := svc.SendAlertSync(ctx, alert)
		if err != nil {
			t.Errorf("Failed to send critical alert: %v", err)
		}
	})

	t.Run("SendAlert_High", func(t *testing.T) {
		alert := notification.SystemAlert(
			notification.SeverityHigh,
			"[TEST] High Priority Alert",
			"This is a high priority integration test alert",
		).WithMetadata("test_run_id", t.Name())

		err := svc.SendAlertSync(ctx, alert)
		if err != nil {
			t.Errorf("Failed to send high priority alert: %v", err)
		}
	})

	t.Run("SendInfo", func(t *testing.T) {
		info := notification.NewInfo(
			"[TEST] Info Notification",
			"This is an integration test info notification",
		).WithMetadata("test_run_id", t.Name())

		err := svc.SendInfoSync(ctx, info)
		if err != nil {
			t.Errorf("Failed to send info notification: %v", err)
		}
	})

	t.Run("SendContestAlert", func(t *testing.T) {
		alert := notification.ContestAlert(
			notification.SeverityMedium,
			"[TEST] Contest Started",
			"Integration test contest has started",
			"test-contest-123",
		).WithMetadata("test_run_id", t.Name())

		err := svc.SendAlertSync(ctx, alert)
		if err != nil {
			t.Errorf("Failed to send contest alert: %v", err)
		}
	})

	t.Run("SendBugAlert", func(t *testing.T) {
		alert := notification.BugAlert(
			notification.SeverityLow,
			"[TEST] Bug Detected",
			"This is a test bug alert for integration testing",
		).WithMetadata("test_run_id", t.Name()).
			WithTracing("trace-123", "span-456")

		err := svc.SendAlertSync(ctx, alert)
		if err != nil {
			t.Errorf("Failed to send bug alert: %v", err)
		}
	})
}

// TestNotification_Resend_RealEmail tests against real Resend API.
// Requires RESEND_TEST_API_KEY and RESEND_TEST_EMAIL environment variables.
func TestNotification_Resend_RealEmail(t *testing.T) {
	apiKey := os.Getenv("RESEND_TEST_API_KEY")
	testEmail := os.Getenv("RESEND_TEST_EMAIL")

	if apiKey == "" {
		t.Skip("RESEND_TEST_API_KEY not set - skipping real email test")
	}
	if testEmail == "" {
		t.Skip("RESEND_TEST_EMAIL not set - skipping real email test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := zaptest.NewLogger(t)

	// Create notification service with email config
	svc, err := notification.NewWithLogger(notification.Config{
		Service: "integration-test",
		Enabled: true,
		Async:   false,
		Email: notification.EmailConfig{
			APIKey:     apiKey,
			FromEmail:  "test@resend.dev",
			Recipients: []string{testEmail},
			Enabled:    true,
		},
		RateLimitPerMinute: 10,
		MinSeverity:        notification.SeverityInfo,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create notification service: %v", err)
	}
	defer svc.Shutdown(ctx)

	t.Run("SendAlert_Email", func(t *testing.T) {
		alert := notification.SystemAlert(
			notification.SeverityHigh,
			"[TEST] Email Alert",
			"This is an integration test email alert - please ignore",
		).WithService("integration-test").
			WithMetadata("test_run_id", t.Name())

		err := svc.SendAlertSync(ctx, alert)
		if err != nil {
			t.Errorf("Failed to send email alert: %v", err)
		}
	})

	t.Run("SendInfo_Email", func(t *testing.T) {
		info := notification.NewInfo(
			"[TEST] Email Info",
			"This is an integration test email notification",
		).WithService("integration-test").
			WithMetadata("test_run_id", t.Name())

		err := svc.SendInfoSync(ctx, info)
		if err != nil {
			t.Errorf("Failed to send email info: %v", err)
		}
	})
}

// ============================================================================
// Service Integration Tests (with mocked external services)
// ============================================================================

// TestNotification_ServiceIntegration tests the full notification flow with mocked services.
func TestNotification_ServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create mock Discord server
	mockDiscord := NewMockDiscordServer()
	defer mockDiscord.Close()

	logger := zaptest.NewLogger(t)

	t.Run("FullFlow_SyncMode", func(t *testing.T) {
		mockDiscord.Reset()

		// Create notification service in sync mode
		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "test-service",
			Enabled:            true,
			Async:              false,
			RateLimitPerMinute: 100,
			MinSeverity:        notification.SeverityInfo,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}
		defer svc.Shutdown(ctx)

		// Create and register Discord notifier
		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Send various alerts
		alerts := []notification.Alert{
			notification.SystemAlert(notification.SeverityCritical, "Critical Alert", "Test critical"),
			notification.SystemAlert(notification.SeverityHigh, "High Alert", "Test high"),
			notification.BugAlert(notification.SeverityMedium, "Bug Alert", "Test bug"),
			notification.ContestAlert(notification.SeverityLow, "Contest Alert", "Test contest", "c-123"),
		}

		for _, alert := range alerts {
			if err := svc.SendAlertSync(ctx, alert); err != nil {
				t.Errorf("Failed to send alert %s: %v", alert.Title, err)
			}
		}

		// Verify all alerts were received
		count := mockDiscord.ReceivedCount.Load()
		if count != int64(len(alerts)) {
			t.Errorf("Expected %d alerts received, got %d", len(alerts), count)
		}
	})

	t.Run("FullFlow_AsyncMode", func(t *testing.T) {
		mockDiscord.Reset()

		// Create notification service in async mode
		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "test-service",
			Enabled:            true,
			Async:              true,
			QueueSize:          100,
			RateLimitPerMinute: 100,
			MinSeverity:        notification.SeverityInfo,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}

		// Create and register Discord notifier
		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Send alerts asynchronously
		numAlerts := 10
		for i := 0; i < numAlerts; i++ {
			alert := notification.SystemAlert(
				notification.SeverityInfo,
				fmt.Sprintf("Async Alert %d", i),
				"Test async alert",
			)
			if err := svc.SendAlert(ctx, alert); err != nil {
				t.Errorf("Failed to queue alert %d: %v", i, err)
			}
		}

		// Shutdown and wait for queue to drain
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
		defer shutdownCancel()
		if err := svc.Shutdown(shutdownCtx); err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}

		// Verify all alerts were received
		count := mockDiscord.ReceivedCount.Load()
		if count != int64(numAlerts) {
			t.Errorf("Expected %d alerts received, got %d", numAlerts, count)
		}
	})

	t.Run("SeverityFiltering", func(t *testing.T) {
		mockDiscord.Reset()

		// Create service with minimum severity of Medium
		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "test-service",
			Enabled:            true,
			Async:              false,
			RateLimitPerMinute: 100,
			MinSeverity:        notification.SeverityMedium,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}
		defer svc.Shutdown(ctx)

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Send alerts of various severities
		svc.SendAlertSync(ctx, notification.SystemAlert(notification.SeverityCritical, "Critical", "Should send"))
		svc.SendAlertSync(ctx, notification.SystemAlert(notification.SeverityHigh, "High", "Should send"))
		svc.SendAlertSync(ctx, notification.SystemAlert(notification.SeverityMedium, "Medium", "Should send"))
		svc.SendAlertSync(ctx, notification.SystemAlert(notification.SeverityLow, "Low", "Should NOT send"))
		svc.SendAlertSync(ctx, notification.SystemAlert(notification.SeverityInfo, "Info", "Should NOT send"))

		// Only 3 alerts should be received (Critical, High, Medium)
		count := mockDiscord.ReceivedCount.Load()
		if count != 3 {
			t.Errorf("Expected 3 alerts (filtered by severity), got %d", count)
		}
	})

	t.Run("ErrorHandling_ServerFailure", func(t *testing.T) {
		mockDiscord.Reset()
		mockDiscord.ShouldFail = true
		mockDiscord.FailureCode = http.StatusServiceUnavailable

		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "test-service",
			Enabled:            true,
			Async:              false,
			RateLimitPerMinute: 100,
			MinSeverity:        notification.SeverityInfo,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}
		defer svc.Shutdown(ctx)

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		alert := notification.SystemAlert(notification.SeverityCritical, "Test Alert", "Test")
		err = svc.SendAlertSync(ctx, alert)

		// Should get an error when server fails
		if err == nil {
			t.Error("Expected error when server fails, got nil")
		}
	})

	t.Run("DisabledService", func(t *testing.T) {
		mockDiscord.Reset()

		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "test-service",
			Enabled:            false, // Disabled
			Async:              false,
			RateLimitPerMinute: 100,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}
		defer svc.Shutdown(ctx)

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		alert := notification.SystemAlert(notification.SeverityCritical, "Test Alert", "Test")
		err = svc.SendAlertSync(ctx, alert)

		if err != notification.ErrNotEnabled {
			t.Errorf("Expected ErrNotEnabled, got %v", err)
		}

		// No alerts should be received
		count := mockDiscord.ReceivedCount.Load()
		if count != 0 {
			t.Errorf("Expected 0 alerts when disabled, got %d", count)
		}
	})
}

// ============================================================================
// Load Tests
// ============================================================================

// TestNotification_UnderLoad tests notification system under high load.
func TestNotification_UnderLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Create mock Discord server with slight delay to simulate network latency
	mockDiscord := NewMockDiscordServer()
	mockDiscord.Delay = 10 * time.Millisecond
	defer mockDiscord.Close()

	logger := zaptest.NewLogger(t)

	t.Run("HighVolume_100Notifications", func(t *testing.T) {
		mockDiscord.Reset()

		// Create notification service with async mode for high volume
		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "load-test",
			Enabled:            true,
			Async:              true,
			QueueSize:          200,
			RateLimitPerMinute: 200, // High limit for load test
			MinSeverity:        notification.SeverityInfo,
			Timeout:            30 * time.Second,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "load-test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Send 100 notifications rapidly
		numNotifications := 100
		startTime := time.Now()

		for i := 0; i < numNotifications; i++ {
			alert := notification.SystemAlert(
				notification.SeverityInfo,
				fmt.Sprintf("Load Test Alert %d", i),
				"Testing high volume notification delivery",
			)
			if err := svc.SendAlert(ctx, alert); err != nil {
				// Some may be rate limited, that's expected
				if err != notification.ErrRateLimited && err != notification.ErrQueueFull {
					t.Errorf("Unexpected error for alert %d: %v", i, err)
				}
			}
		}

		// Shutdown and wait for queue to drain
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 60*time.Second)
		defer shutdownCancel()
		if err := svc.Shutdown(shutdownCtx); err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}

		duration := time.Since(startTime)
		count := mockDiscord.ReceivedCount.Load()

		t.Logf("Sent %d notifications in %v", count, duration)
		t.Logf("Throughput: %.2f notifications/sec", float64(count)/duration.Seconds())

		// At least some notifications should have been delivered
		if count < 50 {
			t.Errorf("Expected at least 50 notifications delivered, got %d", count)
		}
	})

	t.Run("ConcurrentSenders", func(t *testing.T) {
		mockDiscord.Reset()

		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "concurrent-test",
			Enabled:            true,
			Async:              true,
			QueueSize:          500,
			RateLimitPerMinute: 500,
			MinSeverity:        notification.SeverityInfo,
			Timeout:            30 * time.Second,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "concurrent-test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Launch multiple goroutines sending notifications concurrently
		numGoroutines := 10
		notificationsPerGoroutine := 10
		var wg sync.WaitGroup

		startTime := time.Now()

		for g := 0; g < numGoroutines; g++ {
			wg.Add(1)
			go func(goroutineID int) {
				defer wg.Done()
				for i := 0; i < notificationsPerGoroutine; i++ {
					alert := notification.SystemAlert(
						notification.SeverityInfo,
						fmt.Sprintf("Concurrent Alert G%d-N%d", goroutineID, i),
						"Testing concurrent notification delivery",
					)
					svc.SendAlert(ctx, alert) // Ignore errors for this test
				}
			}(g)
		}

		wg.Wait()

		// Shutdown and wait for queue to drain
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 60*time.Second)
		defer shutdownCancel()
		if err := svc.Shutdown(shutdownCtx); err != nil {
			t.Errorf("Shutdown failed: %v", err)
		}

		duration := time.Since(startTime)
		count := mockDiscord.ReceivedCount.Load()

		t.Logf("Concurrent test: %d notifications from %d goroutines in %v",
			count, numGoroutines, duration)

		// At least half should be delivered
		expectedMin := int64(numGoroutines * notificationsPerGoroutine / 2)
		if count < expectedMin {
			t.Errorf("Expected at least %d notifications, got %d", expectedMin, count)
		}
	})

	t.Run("RateLimitingVerification", func(t *testing.T) {
		mockDiscord.Reset()

		// Create service with low rate limit
		rateLimit := 10
		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "rate-limit-test",
			Enabled:            true,
			Async:              false, // Sync mode to count immediately
			RateLimitPerMinute: rateLimit,
			MinSeverity:        notification.SeverityInfo,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}
		defer svc.Shutdown(ctx)

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "rate-limit-test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Send more notifications than the rate limit
		totalAttempts := 20
		successCount := 0
		rateLimitedCount := 0

		for i := 0; i < totalAttempts; i++ {
			alert := notification.SystemAlert(
				notification.SeverityInfo,
				fmt.Sprintf("Rate Limit Test %d", i),
				"Testing rate limiting",
			)
			err := svc.SendAlertSync(ctx, alert)
			if err == nil {
				successCount++
			} else if err == notification.ErrRateLimited {
				rateLimitedCount++
			} else {
				t.Errorf("Unexpected error: %v", err)
			}
		}

		t.Logf("Rate limit test: %d successful, %d rate limited out of %d attempts",
			successCount, rateLimitedCount, totalAttempts)

		// Should have some rate limited
		if rateLimitedCount == 0 {
			t.Error("Expected some notifications to be rate limited")
		}

		// Success count should be approximately equal to rate limit
		if successCount > rateLimit+2 { // Allow some margin
			t.Errorf("Expected at most ~%d successful sends, got %d", rateLimit, successCount)
		}
	})

	t.Run("LatencyDistribution", func(t *testing.T) {
		mockDiscord.Reset()
		mockDiscord.Delay = 50 * time.Millisecond // Simulate realistic latency

		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "latency-test",
			Enabled:            true,
			Async:              false,
			RateLimitPerMinute: 100,
			MinSeverity:        notification.SeverityInfo,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}
		defer svc.Shutdown(ctx)

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "latency-test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Measure latencies
		var latencies []time.Duration
		numSamples := 20

		for i := 0; i < numSamples; i++ {
			alert := notification.SystemAlert(
				notification.SeverityInfo,
				fmt.Sprintf("Latency Test %d", i),
				"Measuring send latency",
			)

			start := time.Now()
			err := svc.SendAlertSync(ctx, alert)
			latency := time.Since(start)

			if err != nil {
				if err != notification.ErrRateLimited {
					t.Errorf("Unexpected error: %v", err)
				}
				continue
			}
			latencies = append(latencies, latency)
		}

		if len(latencies) == 0 {
			t.Fatal("No successful latency samples collected")
		}

		// Calculate statistics
		var totalLatency time.Duration
		var minLatency, maxLatency time.Duration = latencies[0], latencies[0]

		for _, l := range latencies {
			totalLatency += l
			if l < minLatency {
				minLatency = l
			}
			if l > maxLatency {
				maxLatency = l
			}
		}

		avgLatency := totalLatency / time.Duration(len(latencies))

		t.Logf("Latency distribution (n=%d):", len(latencies))
		t.Logf("  Min: %v", minLatency)
		t.Logf("  Max: %v", maxLatency)
		t.Logf("  Avg: %v", avgLatency)

		// Latency should be at least the mock delay
		if avgLatency < mockDiscord.Delay {
			t.Errorf("Average latency %v is less than expected minimum %v",
				avgLatency, mockDiscord.Delay)
		}
	})
}

// ============================================================================
// Test Fixture Tests
// ============================================================================

// TestNotification_Fixtures tests using JSON fixtures.
func TestNotification_Fixtures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fixture test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	mockDiscord := NewMockDiscordServer()
	defer mockDiscord.Close()

	logger := zaptest.NewLogger(t)

	svc, err := notification.NewWithLogger(notification.Config{
		Service:            "fixture-test",
		Enabled:            true,
		Async:              false,
		RateLimitPerMinute: 60,
		MinSeverity:        notification.SeverityInfo,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create notification service: %v", err)
	}
	defer svc.Shutdown(ctx)

	discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
		WebhookURL: mockDiscord.Server.URL,
		Username:   "fixture-test-bot",
		Enabled:    true,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create Discord notifier: %v", err)
	}
	svc.RegisterSender(discordNotifier)

	// Test fixtures
	fixtures := []struct {
		name     string
		filename string
		isAlert  bool
	}{
		{"BugAlert", "sample_bug_alert.json", true},
		{"ContestAlert", "sample_contest_alert.json", true},
		{"SystemAlert", "sample_system_alert.json", true},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			// Check if fixture file exists
			if _, err := os.Stat(fmt.Sprintf("testdata/%s", fixture.filename)); os.IsNotExist(err) {
				t.Skipf("Fixture file %s not found", fixture.filename)
			}

			data := loadTestFixture(t, fixture.filename)

			if fixture.isAlert {
				var alert notification.Alert
				if err := json.Unmarshal(data, &alert); err != nil {
					t.Fatalf("Failed to unmarshal fixture: %v", err)
				}

				// Send the alert
				if err := svc.SendAlertSync(ctx, alert); err != nil {
					t.Errorf("Failed to send alert from fixture: %v", err)
				}
			}
		})
	}
}

// ============================================================================
// Circuit Breaker Tests
// ============================================================================

// TestNotification_CircuitBreaker tests circuit breaker behavior.
func TestNotification_CircuitBreaker(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping circuit breaker test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mockDiscord := NewMockDiscordServer()
	defer mockDiscord.Close()

	logger := zaptest.NewLogger(t)

	t.Run("CircuitOpens_AfterFailures", func(t *testing.T) {
		mockDiscord.Reset()
		mockDiscord.ShouldFail = true
		mockDiscord.FailureCode = http.StatusInternalServerError

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "circuit-test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}

		// Send multiple failing requests to trigger circuit breaker
		failedCount := 0
		circuitOpenCount := 0

		for i := 0; i < 10; i++ {
			alert := notification.SystemAlert(
				notification.SeverityHigh,
				fmt.Sprintf("Circuit Test %d", i),
				"Testing circuit breaker",
			)

			err := discordNotifier.SendAlert(ctx, alert)
			if err != nil {
				if err == notification.ErrDiscordCircuitOpen {
					circuitOpenCount++
				} else {
					failedCount++
				}
			}
		}

		t.Logf("Circuit breaker test: %d failed, %d circuit open errors", failedCount, circuitOpenCount)

		// After multiple failures, circuit should be open
		if circuitOpenCount == 0 {
			t.Error("Expected circuit breaker to open after failures")
		}
	})

	t.Run("CircuitRecovers", func(t *testing.T) {
		mockDiscord.Reset()
		mockDiscord.ShouldFail = true
		mockDiscord.FailureCode = http.StatusInternalServerError

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "recovery-test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}

		// Trigger circuit breaker to open
		for i := 0; i < 5; i++ {
			alert := notification.SystemAlert(notification.SeverityHigh, "Fail", "Testing")
			discordNotifier.SendAlert(ctx, alert)
		}

		// Reset the mock server to succeed
		mockDiscord.ShouldFail = false

		// Reset circuit breaker for testing
		discordNotifier.ResetCircuitBreaker()

		// Verify requests succeed now
		alert := notification.SystemAlert(notification.SeverityHigh, "Recovery Test", "Should succeed")
		err = discordNotifier.SendAlert(ctx, alert)
		if err != nil {
			t.Errorf("Expected success after circuit breaker reset, got: %v", err)
		}

		// Verify notification was received
		count := mockDiscord.ReceivedCount.Load()
		if count < 1 {
			t.Error("Expected at least 1 notification after recovery")
		}
	})
}

// ============================================================================
// Graceful Shutdown Tests
// ============================================================================

// TestNotification_GracefulShutdown tests graceful shutdown behavior.
func TestNotification_GracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping shutdown test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mockDiscord := NewMockDiscordServer()
	mockDiscord.Delay = 50 * time.Millisecond // Add delay to make queue drain observable
	defer mockDiscord.Close()

	logger := zaptest.NewLogger(t)

	t.Run("QueueDrainsOnShutdown", func(t *testing.T) {
		mockDiscord.Reset()

		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "shutdown-test",
			Enabled:            true,
			Async:              true,
			QueueSize:          100,
			RateLimitPerMinute: 200,
			MinSeverity:        notification.SeverityInfo,
			Timeout:            30 * time.Second,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "shutdown-test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Queue up notifications
		numNotifications := 20
		for i := 0; i < numNotifications; i++ {
			alert := notification.SystemAlert(
				notification.SeverityInfo,
				fmt.Sprintf("Shutdown Test %d", i),
				"Testing graceful shutdown",
			)
			svc.SendAlert(ctx, alert)
		}

		// Get count before shutdown
		countBeforeShutdown := mockDiscord.ReceivedCount.Load()

		// Shutdown with sufficient timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
		defer shutdownCancel()

		err = svc.Shutdown(shutdownCtx)
		if err != nil {
			t.Errorf("Shutdown returned error: %v", err)
		}

		// Check final count
		countAfterShutdown := mockDiscord.ReceivedCount.Load()

		t.Logf("Notifications: %d before shutdown, %d after shutdown",
			countBeforeShutdown, countAfterShutdown)

		// More notifications should be processed during shutdown
		if countAfterShutdown <= countBeforeShutdown {
			t.Error("Expected queue to drain during shutdown")
		}
	})

	t.Run("ShutdownTimeout", func(t *testing.T) {
		mockDiscord.Reset()
		mockDiscord.Delay = 500 * time.Millisecond // Long delay

		svc, err := notification.NewWithLogger(notification.Config{
			Service:            "timeout-test",
			Enabled:            true,
			Async:              true,
			QueueSize:          100,
			RateLimitPerMinute: 200,
			MinSeverity:        notification.SeverityInfo,
			Timeout:            30 * time.Second,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create notification service: %v", err)
		}

		discordNotifier, err := notification.NewDiscordNotifier(notification.DiscordConfig{
			WebhookURL: mockDiscord.Server.URL,
			Username:   "timeout-test-bot",
			Enabled:    true,
		}, logger)
		if err != nil {
			t.Fatalf("Failed to create Discord notifier: %v", err)
		}
		svc.RegisterSender(discordNotifier)

		// Queue up many notifications
		for i := 0; i < 50; i++ {
			alert := notification.SystemAlert(
				notification.SeverityInfo,
				fmt.Sprintf("Timeout Test %d", i),
				"Testing shutdown timeout",
			)
			svc.SendAlert(ctx, alert)
		}

		// Shutdown with short timeout (should timeout)
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer shutdownCancel()

		err = svc.Shutdown(shutdownCtx)
		if err == nil {
			// It's OK if shutdown completes quickly
			t.Log("Shutdown completed within timeout")
		} else if err == context.DeadlineExceeded {
			t.Log("Shutdown timed out as expected")
		} else {
			t.Logf("Shutdown returned: %v", err)
		}
	})
}

// ============================================================================
// Noop Notifier Tests
// ============================================================================

// TestNotification_NoopNotifier tests the no-op notifier implementation.
func TestNotification_NoopNotifier(t *testing.T) {
	ctx := context.Background()

	noop := notification.NewNoop()

	// All operations should succeed without errors
	alert := notification.SystemAlert(notification.SeverityCritical, "Test", "Test")
	info := notification.NewInfo("Test", "Test")

	if err := noop.SendAlert(ctx, alert); err != nil {
		t.Errorf("SendAlert should succeed: %v", err)
	}

	if err := noop.SendInfo(ctx, info); err != nil {
		t.Errorf("SendInfo should succeed: %v", err)
	}

	if err := noop.SendAlertSync(ctx, alert); err != nil {
		t.Errorf("SendAlertSync should succeed: %v", err)
	}

	if err := noop.SendInfoSync(ctx, info); err != nil {
		t.Errorf("SendInfoSync should succeed: %v", err)
	}

	if err := noop.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown should succeed: %v", err)
	}
}
