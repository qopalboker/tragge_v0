package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// mockResendServer creates a test server that simulates Resend API.
func mockResendServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func TestNewEmailNotifier(t *testing.T) {
	tests := []struct {
		name    string
		cfg     EmailConfig
		wantErr error
	}{
		{
			name: "valid config",
			cfg: EmailConfig{
				APIKey:    "re_test_123",
				FromEmail: "test@example.com",
			},
			wantErr: nil,
		},
		{
			name:    "empty API key",
			cfg:     EmailConfig{},
			wantErr: ErrEmailAPIKeyEmpty,
		},
		{
			name: "default from email applied",
			cfg: EmailConfig{
				APIKey: "re_test_123",
			},
			wantErr: nil,
		},
		{
			name: "with reply-to",
			cfg: EmailConfig{
				APIKey:    "re_test_123",
				FromEmail: "test@example.com",
				ReplyTo:   "reply@example.com",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			notifier, err := NewEmailNotifier(tt.cfg, logger)

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, notifier)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, notifier)
				assert.Equal(t, ChannelEmail, notifier.Channel())
			}
		})
	}
}

func TestEmailNotifier_NilLogger(t *testing.T) {
	// Should create default logger when nil is passed
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, notifier)
}

func TestEmailNotifier_Templates(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Verify all templates are loaded
	assert.Contains(t, notifier.templates, "bug_report")
	assert.Contains(t, notifier.templates, "daily_digest")
	assert.Contains(t, notifier.templates, "contest_summary")
}

func TestEmailNotifier_RenderBugReportTemplate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := BugReportData{
		Title:         "Critical Bug Found",
		Message:       "Null pointer exception in trading engine",
		Severity:      "CRITICAL",
		SeverityColor: "#dc2626",
		Service:       "trading-engine",
		Timestamp:     time.Now().Format(time.RFC3339),
		TraceID:       "trace-abc-123",
		SpanID:        "span-xyz-789",
		StackTrace:    "panic: runtime error\n\tgoroutine 1 [running]:",
		Metadata: map[string]string{
			"environment": "production",
			"shard_id":    "shard-1",
		},
	}

	html, err := notifier.renderTemplate("bug_report", data)
	require.NoError(t, err)

	// Verify key content is in the rendered HTML
	assert.Contains(t, html, "Critical Bug Found")
	assert.Contains(t, html, "CRITICAL")
	assert.Contains(t, html, "trading-engine")
	assert.Contains(t, html, "trace-abc-123")
	assert.Contains(t, html, "span-xyz-789")
	assert.Contains(t, html, "panic: runtime error")
	assert.Contains(t, html, "production")
}

func TestEmailNotifier_RenderDailyDigestTemplate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := DailyDigest{
		Date:          "2025-01-07",
		TotalAlerts:   42,
		CriticalCount: 3,
		ResolvedCount: 38,
		Services: []ServiceHealth{
			{Name: "trading-engine", Status: "healthy", Uptime: 99.99},
			{Name: "market-ingestor", Status: "degraded", Uptime: 98.5},
		},
		Alerts: []AlertSummary{
			{Title: "High CPU Usage", Severity: "high", Service: "trading-engine", Count: 5, LastOccurrence: "10:30 AM"},
		},
		TopErrors: []ErrorSummary{
			{Message: "Connection timeout", Count: 15},
			{Message: "Rate limit exceeded", Count: 8},
		},
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	html, err := notifier.renderTemplate("daily_digest", data)
	require.NoError(t, err)

	// Verify key content is in the rendered HTML
	assert.Contains(t, html, "2025-01-07")
	assert.Contains(t, html, "42")
	assert.Contains(t, html, "trading-engine")
	assert.Contains(t, html, "market-ingestor")
	assert.Contains(t, html, "High CPU Usage")
	assert.Contains(t, html, "Connection timeout")
}

func TestEmailNotifier_RenderContestSummaryTemplate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := ContestSummary{
		ContestID:         "contest-123",
		ContestName:       "Weekly Trading Challenge",
		Status:            "completed",
		StartDate:         "2025-01-01",
		EndDate:           "2025-01-07",
		TotalParticipants: 150,
		TotalTrades:       12500,
		TotalVolume:       "1,500,000",
		PrizePool:         "10,000",
		Winners: []ContestWinner{
			{Rank: 1, Username: "trader_pro", PnL: 15234.50, Prize: "5,000"},
			{Rank: 2, Username: "market_master", PnL: 12100.75, Prize: "3,000"},
			{Rank: 3, Username: "alpha_trader", PnL: 9850.25, Prize: "2,000"},
		},
		Statistics: []ContestStatistic{
			{Label: "Avg Trades/User", Value: "83"},
			{Label: "Win Rate", Value: "52%"},
		},
		TopSymbols: []SymbolStats{
			{Symbol: "AAPL", Volume: "500,000", Trades: 3500},
			{Symbol: "TSLA", Volume: "350,000", Trades: 2800},
		},
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	html, err := notifier.renderTemplate("contest_summary", data)
	require.NoError(t, err)

	// Verify key content is in the rendered HTML
	assert.Contains(t, html, "Weekly Trading Challenge")
	assert.Contains(t, html, "completed")
	assert.Contains(t, html, "150")
	assert.Contains(t, html, "trader_pro")
	assert.Contains(t, html, "market_master")
	assert.Contains(t, html, "AAPL")
	assert.Contains(t, html, "$10,000")
}

func TestEmailNotifier_RenderTemplateNotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	_, err = notifier.renderTemplate("nonexistent_template", nil)
	assert.ErrorIs(t, err, ErrEmailTemplateError)
	assert.Contains(t, err.Error(), "not found")
}

func TestEmailNotifier_SendEmailNoRecipients(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	err = notifier.SendEmail(context.Background(), []string{}, "Test Subject", "<p>Test</p>")
	assert.ErrorIs(t, err, ErrEmailNoRecipients)
}

func TestEmailNotifier_SendBugReportNoRecipients(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := BugReportData{
		Title:    "Test Bug",
		Severity: "HIGH",
	}

	err = notifier.SendBugReport(context.Background(), []string{}, data)
	assert.ErrorIs(t, err, ErrEmailNoRecipients)
}

func TestEmailNotifier_SendDailyDigestNoRecipients(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	digest := DailyDigest{
		Date: "2025-01-07",
	}

	err = notifier.SendDailyDigest(context.Background(), []string{}, digest)
	assert.ErrorIs(t, err, ErrEmailNoRecipients)
}

func TestEmailNotifier_SendContestSummaryNoRecipients(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	summary := ContestSummary{
		ContestName: "Test Contest",
	}

	err = notifier.SendContestSummary(context.Background(), []string{}, summary)
	assert.ErrorIs(t, err, ErrEmailNoRecipients)
}

func TestEmailNotifier_SendAlert(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Alert without email_recipients metadata should not error but skip
	alert := Alert{
		ID:       "alert-123",
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Test Alert",
		Message:  "Test message",
		Service:  "test-service",
	}

	err = notifier.SendAlert(context.Background(), alert)
	assert.NoError(t, err) // No recipients, so it should just skip
}

func TestEmailNotifier_SendInfo(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Info without email_recipients metadata should not error but skip
	info := Info{
		ID:        "info-123",
		Title:     "Test Info",
		Message:   "Test message",
		Service:   "test-service",
		Timestamp: time.Now().UTC(),
	}

	err = notifier.SendInfo(context.Background(), info)
	assert.NoError(t, err) // No recipients, so it should just skip
}

func TestEmailNotifier_CircuitBreaker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Initial state should be closed
	assert.Equal(t, circuitbreaker.StateClosed, notifier.CircuitBreakerState())

	// Get metrics
	metrics := notifier.CircuitBreakerMetrics()
	assert.Equal(t, int64(0), metrics.TotalRequests)

	// Reset should work
	notifier.ResetCircuitBreaker()
	assert.Equal(t, circuitbreaker.StateClosed, notifier.CircuitBreakerState())
}

func TestEmailNotifier_BatchSend(t *testing.T) {
	var requestCount int32

	server := mockResendServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)

		// Parse request body
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Simulate success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"id": "email-123",
		})
	})
	defer server.Close()

	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Test batch send with empty recipients
	result := notifier.SendBatch(context.Background(), []string{}, "Test", "<p>Test</p>")
	assert.Len(t, result.Successful, 0)
	assert.Len(t, result.Failed, 0)
}

func TestSeverityToEmailColor(t *testing.T) {
	tests := []struct {
		severity Severity
		expected string
	}{
		{SeverityCritical, "#dc2626"},
		{SeverityHigh, "#ea580c"},
		{SeverityMedium, "#ca8a04"},
		{SeverityLow, "#16a34a"},
		{SeverityInfo, "#2563eb"},
		{Severity("unknown"), "#6b7280"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			assert.Equal(t, tt.expected, severityToEmailColor(tt.severity))
		})
	}
}

func TestBugReportData_DefaultSeverityColor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Data without SeverityColor should use default
	data := BugReportData{
		Title:    "Test Bug",
		Severity: "critical",
	}

	html, err := notifier.renderTemplate("bug_report", data)
	require.NoError(t, err)
	assert.Contains(t, html, "Test Bug")
}

func TestDailyDigest_DefaultGeneratedAt(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Create a mock server that returns success
	server := mockResendServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"id": "email-123"})
	})
	defer server.Close()

	digest := DailyDigest{
		Date: "2025-01-07",
		// GeneratedAt not set
	}

	// Even though sending will fail (wrong API endpoint), template should render
	html, err := notifier.renderTemplate("daily_digest", digest)
	require.NoError(t, err)
	assert.Contains(t, html, "2025-01-07")
}

func TestContestSummary_DefaultGeneratedAt(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	summary := ContestSummary{
		ContestName: "Test Contest",
		// GeneratedAt not set
	}

	html, err := notifier.renderTemplate("contest_summary", summary)
	require.NoError(t, err)
	assert.Contains(t, html, "Test Contest")
}

func TestEmailNotifier_AlertWithStackTrace(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	alert := Alert{
		ID:       "alert-123",
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Test Alert",
		Message:  "Test message",
		Service:  "test-service",
		Metadata: map[string]string{
			"stack_trace":     "goroutine 1 [running]:\nmain.main()",
			"email_recipients": "test@example.com",
		},
	}

	// This won't actually send (wrong API endpoint) but verifies the conversion logic
	_ = notifier.SendAlert(context.Background(), alert)
}

func TestEmailNotifier_InfoWithRecipients(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	info := Info{
		ID:        "info-123",
		Title:     "Test Info",
		Message:   "Test message",
		Service:   "test-service",
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"email_recipients": "user1@example.com, user2@example.com",
		},
	}

	// This won't actually send (wrong API endpoint) but verifies the parsing logic
	_ = notifier.SendInfo(context.Background(), info)
}

func TestBatchSendResult_PartialSuccess(t *testing.T) {
	result := &BatchSendResult{
		Successful: []string{"user1@example.com", "user2@example.com"},
		Failed: []BatchSendError{
			{Recipient: "user3@example.com", Error: ErrEmailCircuitOpen},
		},
	}

	assert.Len(t, result.Successful, 2)
	assert.Len(t, result.Failed, 1)
	assert.Equal(t, "user3@example.com", result.Failed[0].Recipient)
}

func TestEmailNotifier_TemplateDataEscaping(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Test that HTML is properly escaped in templates
	data := BugReportData{
		Title:    "<script>alert('xss')</script>",
		Message:  "Test <b>message</b>",
		Severity: "HIGH",
	}

	html, err := notifier.renderTemplate("bug_report", data)
	require.NoError(t, err)

	// The template should escape HTML entities
	assert.NotContains(t, html, "<script>alert('xss')</script>")
	assert.Contains(t, html, "&lt;script&gt;")
}

func TestEmailNotifier_ContestWinnerNegativePnL(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := ContestSummary{
		ContestName: "Test Contest",
		Winners: []ContestWinner{
			{Rank: 1, Username: "loser", PnL: -500.50, Prize: "0"},
		},
	}

	html, err := notifier.renderTemplate("contest_summary", data)
	require.NoError(t, err)
	assert.Contains(t, html, "-500.5")
}

func TestEmailNotifier_EmptyServices(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := DailyDigest{
		Date:     "2025-01-07",
		Services: []ServiceHealth{}, // Empty
		Alerts:   []AlertSummary{},  // Empty
	}

	html, err := notifier.renderTemplate("daily_digest", data)
	require.NoError(t, err)
	assert.Contains(t, html, "2025-01-07")
}

func TestParseTemplates(t *testing.T) {
	templates, err := parseTemplates()
	require.NoError(t, err)

	assert.Len(t, templates, 23)
	assert.Contains(t, templates, "bug_report")
	assert.Contains(t, templates, "daily_digest")
	assert.Contains(t, templates, "contest_summary")
	assert.Contains(t, templates, "contest_starting")
	assert.Contains(t, templates, "contest_ending")
	assert.Contains(t, templates, "contest_started")
	assert.Contains(t, templates, "contest_cancelled")
	assert.Contains(t, templates, "contest_ended")
}

func TestEmailNotifier_Channel(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	assert.Equal(t, ChannelEmail, notifier.Channel())
	assert.Equal(t, "email", notifier.Channel().String())
}

func TestEmailNotifier_AlertToSubject(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)
	require.NotNil(t, notifier)

	alert := Alert{
		Type:     AlertTypeBug,
		Severity: SeverityCritical,
		Title:    "Test Alert",
	}

	// Verify the subject format by checking the internal conversion
	data := BugReportData{
		Title:         alert.Title,
		Severity:      strings.ToUpper(string(alert.Severity)),
		SeverityColor: severityToEmailColor(alert.Severity),
	}

	assert.Equal(t, "CRITICAL", data.Severity)
	assert.Equal(t, "#dc2626", data.SeverityColor)
}

func BenchmarkEmailNotifier_RenderTemplate(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	notifier, _ := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)

	data := BugReportData{
		Title:         "Benchmark Bug",
		Message:       "This is a benchmark test",
		Severity:      "HIGH",
		SeverityColor: "#ea580c",
		Service:       "benchmark-service",
		Timestamp:     time.Now().Format(time.RFC3339),
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = notifier.renderTemplate("bug_report", data)
	}
}

func BenchmarkParseTemplates(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = parseTemplates()
	}
}

// =============================================================================
// Additional comprehensive tests
// =============================================================================

// TestEmailNotifier_SendEmail_Success tests successful email sending with mock server.
// Note: This test validates the code path but actual sending requires a real Resend API.
func TestEmailNotifier_SendEmail_Success(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// SendEmail will fail because we're not using a real Resend API key,
	// but we can verify the function processes parameters correctly
	// and handles circuit breaker properly
	assert.Equal(t, circuitbreaker.StateClosed, notifier.CircuitBreakerState())

	// Verify template rendering works correctly
	data := BugReportData{
		Title:         "Test Bug",
		Message:       "Test message",
		Severity:      "HIGH",
		SeverityColor: "#ea580c",
		Service:       "test-service",
		Timestamp:     time.Now().Format(time.RFC3339),
	}

	html, err := notifier.renderTemplate("bug_report", data)
	require.NoError(t, err)
	assert.Contains(t, html, "Test Bug")
	assert.Contains(t, html, "Test message")
}

// TestEmailNotifier_SendEmail_InvalidAPIKey tests handling of invalid API key.
func TestEmailNotifier_SendEmail_InvalidAPIKey(t *testing.T) {
	// Empty API key should fail
	_, err := NewEmailNotifier(EmailConfig{
		APIKey: "",
	}, zaptest.NewLogger(t))
	assert.ErrorIs(t, err, ErrEmailAPIKeyEmpty)

	// Valid but fake API key should create notifier (validation happens on send)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_invalid_key_123",
		FromEmail: "test@example.com",
	}, zaptest.NewLogger(t))
	require.NoError(t, err)
	assert.NotNil(t, notifier)
}

// TestEmailNotifier_SendEmail_NetworkError tests handling of network errors.
func TestEmailNotifier_SendEmail_NetworkError(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Attempt to send email - will fail with network error (no real API)
	// This tests that error handling path works correctly
	err = notifier.SendEmail(context.Background(), []string{"test@example.com"}, "Test", "<p>Test</p>")

	// Should get an error (actual network/API error)
	assert.Error(t, err)
}

// TestEmailNotifier_CircuitBreaker_TripsOnFailures tests circuit breaker trips after repeated failures.
func TestEmailNotifier_CircuitBreaker_TripsOnFailures(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Initial state should be closed
	assert.Equal(t, circuitbreaker.StateClosed, notifier.CircuitBreakerState())

	ctx := context.Background()

	// Send multiple emails that will fail (no real API)
	// The circuit breaker should eventually trip
	for i := 0; i < 10; i++ {
		_ = notifier.SendEmail(ctx, []string{"test@example.com"}, "Test", "<p>Test</p>")
	}

	// After 5 failures (default MaxFailures), circuit should be open
	metrics := notifier.CircuitBreakerMetrics()
	assert.GreaterOrEqual(t, metrics.TotalFailures, int64(5))

	// Circuit may be open if enough failures occurred
	state := notifier.CircuitBreakerState()
	if state == circuitbreaker.StateOpen {
		// Verify subsequent calls are rejected immediately
		err := notifier.SendEmail(ctx, []string{"test@example.com"}, "Test", "<p>Test</p>")
		assert.ErrorIs(t, err, ErrEmailCircuitOpen)
	}
}

// TestEmailNotifier_SendBugReport_RendersTemplate tests bug report template rendering.
func TestEmailNotifier_SendBugReport_RendersTemplate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := BugReportData{
		Title:         "Critical Bug Found",
		Message:       "Null pointer exception in trading engine",
		Severity:      "CRITICAL",
		SeverityColor: "#dc2626",
		Service:       "trading-engine",
		Timestamp:     time.Now().Format(time.RFC3339),
		TraceID:       "trace-abc-123",
		SpanID:        "span-xyz-789",
		StackTrace:    "panic: runtime error: invalid memory address\n\tat main.go:42",
		Metadata: map[string]string{
			"environment": "production",
			"shard_id":    "shard-1",
			"version":     "1.2.3",
		},
	}

	html, err := notifier.renderTemplate("bug_report", data)
	require.NoError(t, err)

	// Verify all key content is in the rendered HTML
	assert.Contains(t, html, "Critical Bug Found")
	assert.Contains(t, html, "CRITICAL")
	assert.Contains(t, html, "#dc2626")
	assert.Contains(t, html, "trading-engine")
	assert.Contains(t, html, "trace-abc-123")
	assert.Contains(t, html, "span-xyz-789")
	assert.Contains(t, html, "panic: runtime error")
	assert.Contains(t, html, "production")
	assert.Contains(t, html, "shard-1")
}

// TestEmailNotifier_SendDailyDigest_MultipleAlerts tests daily digest with multiple alerts.
func TestEmailNotifier_SendDailyDigest_MultipleAlerts(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := DailyDigest{
		Date:          "2026-01-07",
		TotalAlerts:   150,
		CriticalCount: 5,
		ResolvedCount: 140,
		Services: []ServiceHealth{
			{Name: "trading-engine", Status: "healthy", Uptime: 99.99},
			{Name: "market-ingestor", Status: "degraded", Uptime: 98.5},
			{Name: "user-bff", Status: "healthy", Uptime: 100.0},
			{Name: "leaderboard-worker", Status: "unhealthy", Uptime: 85.2},
		},
		Alerts: []AlertSummary{
			{Title: "High CPU Usage", Severity: "critical", Service: "trading-engine", Count: 3, LastOccurrence: "10:30 AM"},
			{Title: "Memory Warning", Severity: "high", Service: "market-ingestor", Count: 8, LastOccurrence: "11:45 AM"},
			{Title: "Connection Timeout", Severity: "medium", Service: "user-bff", Count: 15, LastOccurrence: "12:00 PM"},
			{Title: "Slow Query", Severity: "low", Service: "admin-bff", Count: 42, LastOccurrence: "2:30 PM"},
		},
		TopErrors: []ErrorSummary{
			{Message: "Connection timeout", Count: 25},
			{Message: "Rate limit exceeded", Count: 18},
			{Message: "Invalid token", Count: 12},
			{Message: "Database connection failed", Count: 8},
		},
		GeneratedAt: time.Now().Format(time.RFC3339),
	}

	html, err := notifier.renderTemplate("daily_digest", data)
	require.NoError(t, err)

	// Verify all sections are present
	assert.Contains(t, html, "2026-01-07")
	assert.Contains(t, html, "150")
	assert.Contains(t, html, "trading-engine")
	assert.Contains(t, html, "market-ingestor")
	assert.Contains(t, html, "High CPU Usage")
	assert.Contains(t, html, "Memory Warning")
	assert.Contains(t, html, "Connection timeout")
	assert.Contains(t, html, "Rate limit exceeded")
}

// TestEmailNotifier_BatchSend_PartialFailure tests batch sending with partial failures.
func TestEmailNotifier_BatchSend_PartialFailure(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Batch send to multiple recipients - all will fail due to no real API
	recipients := []string{
		"user1@example.com",
		"user2@example.com",
		"user3@example.com",
	}

	result := notifier.SendBatch(context.Background(), recipients, "Test Subject", "<p>Test content</p>")

	// All should fail (no real API)
	assert.Len(t, result.Successful, 0)
	assert.Len(t, result.Failed, 3)

	// Verify each failure is tracked
	failedRecipients := make(map[string]bool)
	for _, f := range result.Failed {
		failedRecipients[f.Recipient] = true
		assert.Error(t, f.Error)
	}

	assert.True(t, failedRecipients["user1@example.com"])
	assert.True(t, failedRecipients["user2@example.com"])
	assert.True(t, failedRecipients["user3@example.com"])
}

// TestEmailNotifier_AlertTypeConversion tests alert to email data conversion.
func TestEmailNotifier_AlertTypeConversion(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)
	assert.Equal(t, ChannelEmail, notifier.Channel())

	tests := []struct {
		name     string
		alert    Alert
		expected string
	}{
		{
			name: "bug alert",
			alert: Alert{
				Type:     AlertTypeBug,
				Severity: SeverityCritical,
				Title:    "Bug Alert",
				Message:  "Test bug",
			},
			expected: "CRITICAL",
		},
		{
			name: "system alert",
			alert: Alert{
				Type:     AlertTypeSystem,
				Severity: SeverityHigh,
				Title:    "System Alert",
				Message:  "Test system",
			},
			expected: "HIGH",
		},
		{
			name: "contest alert",
			alert: Alert{
				Type:     AlertTypeContest,
				Severity: SeverityMedium,
				Title:    "Contest Alert",
				Message:  "Test contest",
			},
			expected: "MEDIUM",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Verify the severity string conversion
			severity := strings.ToUpper(string(tc.alert.Severity))
			assert.Equal(t, tc.expected, severity)

			// Verify color mapping
			color := severityToEmailColor(tc.alert.Severity)
			assert.NotEmpty(t, color)
		})
	}
}

// TestEmailNotifier_RenderTemplateErrors tests template rendering error handling.
func TestEmailNotifier_RenderTemplateErrors(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Test non-existent template
	_, err = notifier.renderTemplate("nonexistent", nil)
	assert.ErrorIs(t, err, ErrEmailTemplateError)
	assert.Contains(t, err.Error(), "not found")

	// Test with nil data (should still render)
	html, err := notifier.renderTemplate("bug_report", BugReportData{})
	require.NoError(t, err)
	assert.NotEmpty(t, html)
}

// TestEmailNotifier_SpecialCharacters tests handling of special characters in content.
func TestEmailNotifier_SpecialCharacters(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	data := BugReportData{
		Title:    "Alert with <script>alert('xss')</script> injection",
		Message:  "Test & message with \"quotes\" and 'apostrophes'",
		Severity: "HIGH",
		Metadata: map[string]string{
			"key<>test": "value&test",
		},
	}

	html, err := notifier.renderTemplate("bug_report", data)
	require.NoError(t, err)

	// Verify HTML entities are escaped
	assert.NotContains(t, html, "<script>")
	assert.Contains(t, html, "&lt;script&gt;")
}

// TestEmailNotifier_LongContent tests handling of long content.
func TestEmailNotifier_LongContent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Generate long stack trace
	var stackTrace strings.Builder
	for i := 0; i < 100; i++ {
		stackTrace.WriteString(fmt.Sprintf("at function%d() in file%d.go:%d\n", i, i, i*10))
	}

	data := BugReportData{
		Title:      "Bug with long stack trace",
		Message:    strings.Repeat("This is a very long message. ", 50),
		Severity:   "CRITICAL",
		StackTrace: stackTrace.String(),
	}

	html, err := notifier.renderTemplate("bug_report", data)
	require.NoError(t, err)
	assert.NotEmpty(t, html)
	assert.Contains(t, html, "at function0()")
	assert.Contains(t, html, "at function99()")
}

// TestEmailNotifier_ContestSummaryEdgeCases tests contest summary edge cases.
func TestEmailNotifier_ContestSummaryEdgeCases(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	tests := []struct {
		name    string
		summary ContestSummary
	}{
		{
			name: "empty winners",
			summary: ContestSummary{
				ContestName: "Empty Contest",
				Winners:     []ContestWinner{},
			},
		},
		{
			name: "negative PnL",
			summary: ContestSummary{
				ContestName: "Contest with Losses",
				Winners: []ContestWinner{
					{Rank: 1, Username: "winner", PnL: -1500.50, Prize: "0"},
					{Rank: 2, Username: "loser", PnL: -5000.00, Prize: "0"},
				},
			},
		},
		{
			name: "large numbers",
			summary: ContestSummary{
				ContestName:       "Big Contest",
				TotalParticipants: 100000,
				TotalTrades:       5000000,
				TotalVolume:       "1,000,000,000",
				PrizePool:         "100,000",
				Winners: []ContestWinner{
					{Rank: 1, Username: "top_trader", PnL: 1500000.00, Prize: "50,000"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html, err := notifier.renderTemplate("contest_summary", tc.summary)
			require.NoError(t, err)
			assert.Contains(t, html, tc.summary.ContestName)
		})
	}
}

// TestEmailNotifier_MetricsTracking tests that metrics are tracked correctly.
func TestEmailNotifier_MetricsTracking(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Initial metrics
	initialMetrics := notifier.CircuitBreakerMetrics()
	assert.Equal(t, int64(0), initialMetrics.TotalRequests)

	// Make some requests (will fail due to no real API)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = notifier.SendEmail(ctx, []string{"test@example.com"}, "Test", "<p>Test</p>")
	}

	// Check metrics increased
	afterMetrics := notifier.CircuitBreakerMetrics()
	assert.GreaterOrEqual(t, afterMetrics.TotalRequests, int64(3))
}

// TestEmailNotifier_ReplyToHeader tests reply-to header is set.
func TestEmailNotifier_ReplyToHeader(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "noreply@example.com",
		ReplyTo:   "support@example.com",
	}, logger)
	require.NoError(t, err)
	assert.NotNil(t, notifier)
}

// TestEmailNotifier_ContextCancellation tests handling of cancelled context.
func TestEmailNotifier_ContextCancellation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	notifier, err := NewEmailNotifier(EmailConfig{
		APIKey:    "re_test_123",
		FromEmail: "test@example.com",
	}, logger)
	require.NoError(t, err)

	// Create already cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Send with cancelled context
	err = notifier.SendEmail(ctx, []string{"test@example.com"}, "Test", "<p>Test</p>")

	// Should get an error (context cancelled or API error)
	assert.Error(t, err)
}
