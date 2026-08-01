package notification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// =============================================================================
// Mock Discord Server
// =============================================================================

// MockDiscordServer simulates Discord webhook API for testing.
type MockDiscordServer struct {
	*httptest.Server
	mu             sync.Mutex
	Requests       []MockDiscordRequest
	ResponseStatus int
	ResponseDelay  time.Duration
	FailAfter      int // Fail after N successful requests (0 = never fail)
	requestCount   int32
}

// MockDiscordRequest represents a captured Discord webhook request.
type MockDiscordRequest struct {
	Method      string
	ContentType string
	Body        map[string]interface{}
	Timestamp   time.Time
}

// NewMockDiscordServer creates a new mock Discord server.
func NewMockDiscordServer(t *testing.T) *MockDiscordServer {
	mock := &MockDiscordServer{
		Requests:       make([]MockDiscordRequest, 0),
		ResponseStatus: http.StatusNoContent,
	}

	mock.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		count := atomic.AddInt32(&mock.requestCount, 1)

		// Check if we should fail
		if mock.FailAfter > 0 && int(count) > mock.FailAfter {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Apply delay
		if mock.ResponseDelay > 0 {
			time.Sleep(mock.ResponseDelay)
		}

		// Capture request
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			body = make(map[string]interface{})
		}

		mock.Requests = append(mock.Requests, MockDiscordRequest{
			Method:      r.Method,
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
			Timestamp:   time.Now(),
		})

		w.WriteHeader(mock.ResponseStatus)
	}))

	return mock
}

// RequestCount returns the number of requests received.
func (m *MockDiscordServer) RequestCount() int {
	return int(atomic.LoadInt32(&m.requestCount))
}

// LastRequest returns the last received request.
func (m *MockDiscordServer) LastRequest() *MockDiscordRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Requests) == 0 {
		return nil
	}
	return &m.Requests[len(m.Requests)-1]
}

// Reset clears all captured requests.
func (m *MockDiscordServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Requests = make([]MockDiscordRequest, 0)
	atomic.StoreInt32(&m.requestCount, 0)
}

// =============================================================================
// Mock Resend Client (interface mock)
// =============================================================================

// MockResendClient simulates Resend API for testing.
type MockResendClient struct {
	mu             sync.Mutex
	Emails         []MockEmailRequest
	ShouldFail     bool
	FailError      error
	ResponseDelay  time.Duration
	requestCount   int32
}

// MockEmailRequest represents a captured email send request.
type MockEmailRequest struct {
	To        []string
	From      string
	Subject   string
	HTML      string
	Timestamp time.Time
}

// NewMockResendClient creates a new mock Resend client.
func NewMockResendClient() *MockResendClient {
	return &MockResendClient{
		Emails: make([]MockEmailRequest, 0),
	}
}

// Send simulates sending an email.
func (m *MockResendClient) Send(from string, to []string, subject, html string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.AddInt32(&m.requestCount, 1)

	if m.ResponseDelay > 0 {
		time.Sleep(m.ResponseDelay)
	}

	if m.ShouldFail {
		return m.FailError
	}

	m.Emails = append(m.Emails, MockEmailRequest{
		To:        to,
		From:      from,
		Subject:   subject,
		HTML:      html,
		Timestamp: time.Now(),
	})

	return nil
}

// RequestCount returns the number of send attempts.
func (m *MockResendClient) RequestCount() int {
	return int(atomic.LoadInt32(&m.requestCount))
}

// LastEmail returns the last sent email.
func (m *MockResendClient) LastEmail() *MockEmailRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Emails) == 0 {
		return nil
	}
	return &m.Emails[len(m.Emails)-1]
}

// Reset clears all captured emails.
func (m *MockResendClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Emails = make([]MockEmailRequest, 0)
	atomic.StoreInt32(&m.requestCount, 0)
}

// =============================================================================
// Test Alert Generators
// =============================================================================

// TestAlertGenerators provides factory functions for creating test alerts.
type TestAlertGenerators struct{}

// NewTestAlertGenerators creates a new TestAlertGenerators instance.
func NewTestAlertGenerators() *TestAlertGenerators {
	return &TestAlertGenerators{}
}

// BugAlert creates a test bug alert.
func (g *TestAlertGenerators) BugAlert(severity Severity) Alert {
	return Alert{
		ID:       "test-bug-alert",
		Type:     AlertTypeBug,
		Severity: severity,
		Title:    "Test Bug Alert",
		Message:  "This is a test bug alert message",
		Service:  "test-service",
		TraceID:  "trace-test-123",
		SpanID:   "span-test-456",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"environment": "test",
			"version":     "1.0.0",
		},
	}
}

// SystemAlert creates a test system alert.
func (g *TestAlertGenerators) SystemAlert(severity Severity) Alert {
	return Alert{
		ID:       "test-system-alert",
		Type:     AlertTypeSystem,
		Severity: severity,
		Title:    "Test System Alert",
		Message:  "This is a test system alert message",
		Service:  "test-service",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"environment": "test",
			"cpu_usage":   "85%",
			"memory":      "70%",
		},
	}
}

// ContestAlert creates a test contest alert.
func (g *TestAlertGenerators) ContestAlert(severity Severity, contestID string) Alert {
	return Alert{
		ID:       "test-contest-alert",
		Type:     AlertTypeContest,
		Severity: severity,
		Title:    "Test Contest Alert",
		Message:  "This is a test contest alert message",
		Service:  "admin-bff",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"environment": "test",
			"contest_id":  contestID,
			"event":       "start",
		},
	}
}

// TradeAlert creates a test trade alert.
func (g *TestAlertGenerators) TradeAlert(severity Severity, userID, orderID string) Alert {
	return Alert{
		ID:       "test-trade-alert",
		Type:     AlertTypeTrade,
		Severity: severity,
		Title:    "Test Trade Alert",
		Message:  "This is a test trade alert message",
		Service:  "trading-engine",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"environment": "test",
			"user_id":     userID,
			"order_id":    orderID,
		},
	}
}

// UserAlert creates a test user alert.
func (g *TestAlertGenerators) UserAlert(severity Severity, userID string) Alert {
	return Alert{
		ID:       "test-user-alert",
		Type:     AlertTypeUser,
		Severity: severity,
		Title:    "Test User Alert",
		Message:  "This is a test user alert message",
		Service:  "user-bff",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"environment": "test",
			"user_id":     userID,
		},
	}
}

// CriticalAlert creates a critical severity alert.
func (g *TestAlertGenerators) CriticalAlert() Alert {
	return g.SystemAlert(SeverityCritical)
}

// HighAlert creates a high severity alert.
func (g *TestAlertGenerators) HighAlert() Alert {
	return g.SystemAlert(SeverityHigh)
}

// MediumAlert creates a medium severity alert.
func (g *TestAlertGenerators) MediumAlert() Alert {
	return g.SystemAlert(SeverityMedium)
}

// LowAlert creates a low severity alert.
func (g *TestAlertGenerators) LowAlert() Alert {
	return g.SystemAlert(SeverityLow)
}

// InfoAlert creates an info severity alert.
func (g *TestAlertGenerators) InfoAlert() Alert {
	return g.SystemAlert(SeverityInfo)
}

// =============================================================================
// Test Info Generators
// =============================================================================

// InfoNotification creates a test info notification.
func (g *TestAlertGenerators) InfoNotification() Info {
	return Info{
		ID:        "test-info",
		Title:     "Test Info Notification",
		Message:   "This is a test info notification",
		Service:   "test-service",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"environment": "test",
			"version":     "1.0.0",
		},
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

// AssertAlertContains verifies an alert contains expected values.
func AssertAlertContains(t *testing.T, alert Alert, expectedType AlertType, expectedSeverity Severity) {
	t.Helper()
	require.Equal(t, expectedType, alert.Type, "alert type mismatch")
	require.Equal(t, expectedSeverity, alert.Severity, "alert severity mismatch")
}

// WaitForCondition waits for a condition to be true with timeout.
func WaitForCondition(timeout time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// CreateTestServiceConfig creates a ServiceConfig for testing.
func CreateTestServiceConfig(discordURL string) ServiceConfig {
	return ServiceConfig{
		Enabled:        true,
		AsyncEnabled:   false,
		AsyncWorkers:   2,
		AsyncQueueSize: 10,
		Environment:    "test",
		ServiceName:    "test-service",
		Discord: DiscordConfig{
			WebhookURL: discordURL,
			Enabled:    true,
			Username:   "test-bot",
		},
		ShutdownTimeout: 5 * time.Second,
	}
}

// CreateTestEmailConfig creates an EmailConfig for testing.
func CreateTestEmailConfig() EmailConfig {
	return EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
		ReplyTo:   "support@example.com",
		Enabled:   true,
	}
}

// CreateTestDiscordConfig creates a DiscordConfig for testing.
func CreateTestDiscordConfig(webhookURL string) DiscordConfig {
	return DiscordConfig{
		WebhookURL: webhookURL,
		Enabled:    true,
		Username:   "test-bot",
		AvatarURL:  "https://example.com/avatar.png",
	}
}
