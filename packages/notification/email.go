package notification

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/resend/resend-go/v2"
	"go.uber.org/zap"
)

//go:embed templates/*.html
var templateFS embed.FS

// Cached parsed templates — parsed once via sync.Once since embed.FS content is immutable.
var (
	cachedTemplates    map[string]*template.Template
	cachedTemplatesErr error
	templateOnce       sync.Once
)

// Common errors for Email notifier.
var (
	ErrEmailAPIKeyEmpty   = errors.New("email: API key is empty")
	ErrEmailFromEmpty     = errors.New("email: from email is empty")
	ErrEmailNoRecipients  = errors.New("email: no recipients specified")
	ErrEmailCircuitOpen   = errors.New("email: circuit breaker is open")
	ErrEmailTemplateError = errors.New("email: template execution error")
)

// BugReportData contains data for rendering a bug report email.
type BugReportData struct {
	Title         string
	Message       string
	Severity      string
	SeverityColor string
	Service       string
	Timestamp     string
	TraceID       string
	SpanID        string
	StackTrace    string
	Metadata      map[string]string
}

// DailyDigest contains data for rendering a daily digest email.
type DailyDigest struct {
	Date          string
	TotalAlerts   int
	CriticalCount int
	ResolvedCount int
	Services      []ServiceHealth
	Alerts        []AlertSummary
	TopErrors     []ErrorSummary
	GeneratedAt   string
}

// ServiceHealth represents the health status of a service.
type ServiceHealth struct {
	Name   string
	Status string // "healthy", "degraded", "unhealthy"
	Uptime float64
}

// AlertSummary represents a summarized alert for the digest.
type AlertSummary struct {
	Title          string
	Severity       string
	Service        string
	Count          int
	LastOccurrence string
}

// ErrorSummary represents a summarized error for the digest.
type ErrorSummary struct {
	Message string
	Count   int
}

// ContestSummary contains data for rendering a contest summary email.
type ContestSummary struct {
	ContestID         string
	ContestName       string
	Status            string
	StartDate         string
	EndDate           string
	TotalParticipants int
	TotalTrades       int
	TotalVolume       string
	PrizePool         string
	Winners           []ContestWinner
	Statistics        []ContestStatistic
	TopSymbols        []SymbolStats
	GeneratedAt       string
}

// ContestWinner represents a winner in the contest.
type ContestWinner struct {
	Rank     int
	Username string
	PnL      float64
	Prize    string
}

// ContestStatistic represents a statistic about the contest.
type ContestStatistic struct {
	Label string
	Value string
}

// SymbolStats represents trading statistics for a symbol.
type SymbolStats struct {
	Symbol string
	Volume string
	Trades int
}

// KYCApprovedData contains data for rendering a KYC approved email.
type KYCApprovedData struct {
	UserName     string
	ExpiresAt    string
	DashboardURL string
}

// KYCRejectedData contains data for rendering a KYC rejected email.
type KYCRejectedData struct {
	UserName        string
	Reason          string
	VerificationURL string
	RejectedFields  []string          // field names that need correction
	FieldMessages   map[string]string // per-field rejection messages
}

// KYCInfoRequestData contains data for rendering a KYC info request email.
type KYCInfoRequestData struct {
	UserName        string
	Message         string
	VerificationURL string
}

// PasswordResetData contains data for rendering a password reset email.
type PasswordResetData struct {
	UserName string
	ResetURL string
}

// PasswordResetCodeData contains data for rendering a password reset code email.
type PasswordResetCodeData struct {
	Code string
	Lang string // Language code: "en" or "fa" (default)
}

// PasswordChangedData contains data for rendering a password changed notification email.
type PasswordChangedData struct {
	Method string // "forgot_password" or "panel_change" — rendered as display text
}

// EmailVerificationData contains data for rendering an email verification email.
type EmailVerificationData struct {
	UserName         string
	VerificationCode string // 6-digit OTP code
	Lang             string // Language code: "en" (default) or "fa" (Farsi)
}

// WelcomeEmailData contains data for rendering a welcome email.
type WelcomeEmailData struct {
	UserEmail        string // User's email address
	VerificationCode string // 6-digit OTP code (empty if email already verified)
	DashboardURL     string // Link to user dashboard
	Lang             string // Language code: "en" (default) or "fa" (Farsi)
}

// WithdrawalApprovedData contains data for rendering a withdrawal approved email.
type WithdrawalApprovedData struct {
	UserName     string
	Amount       string // Formatted amount with currency symbol (e.g., "$100.00")
	AdminComment string // Optional admin comment
	DashboardURL string
}

// WithdrawalRejectedData contains data for rendering a withdrawal rejected email.
type WithdrawalRejectedData struct {
	UserName     string
	Amount       string // Formatted amount with currency symbol (e.g., "$100.00")
	Reason       string // Rejection reason
	DashboardURL string
}

// WithdrawalProcessingData contains data for rendering a withdrawal processing email.
type WithdrawalProcessingData struct {
	UserName     string
	Amount       string
	DashboardURL string
}

// WithdrawalCompletedData contains data for rendering a withdrawal completed email.
type WithdrawalCompletedData struct {
	UserName     string
	Amount       string
	DashboardURL string
}

// DepositConfirmedData contains data for rendering a deposit confirmed email.
type DepositConfirmedData struct {
	UserName      string // Optional user name
	Amount        string // Formatted amount with currency symbol (e.g., "$100.00")
	NewBalance    string // User's new wallet balance after deposit
	Date          string // Formatted date of the deposit
	TransactionID string // Transaction/payment intent ID
	WalletURL     string // Link to wallet page
}

// ContestStartedData contains data for rendering a contest started notification email.
type ContestStartedData struct {
	ContestName string // Contest name
	ContestID   string // Contest ID
	TradeURL    string // Link to trade panel
	EndsAt      string // Formatted contest end time
}

// ContestEndingData contains data for rendering a contest ending reminder email.
type ContestEndingData struct {
	ContestID        string   // Contest ID
	ContestName      string   // Contest name
	EndTime          string   // Formatted end time
	TimeUntilEnd     string   // Time until end (e.g., "15 minutes")
	Duration         string   // Duration string (e.g., "2 hours", "1 day")
	StartingBalance  string   // Starting virtual balance (e.g., "$100,000")
	ParticipantCount int      // Number of participants
	Symbols          []string // Available trading symbols
	TradingURL       string   // Link to trading interface
}

// ContestStartingData contains data for rendering a contest starting reminder email.
type ContestStartingData struct {
	ContestID        string   // Contest ID
	ContestName      string   // Contest name
	StartTime        string   // Formatted start time
	EndTime          string   // Formatted end time
	Duration         string   // Duration string (e.g., "2 hours", "1 day")
	TimeUntilStart   string   // Time until start (e.g., "15 minutes")
	StartingBalance  string   // Starting virtual balance (e.g., "$100,000")
	ParticipantCount int      // Number of participants
	Symbols          []string // Available trading symbols
	TradingURL       string   // Link to trading interface
}

// PrizeWonData contains data for rendering a prize won email.
type PrizeWonData struct {
	UserName          string // Optional user name
	ContestID         string // Contest ID
	ContestName       string // Contest name
	FinalRank         int    // User's final rank in the contest
	TotalParticipants int    // Total number of participants
	PrizeAmount       string // Formatted prize amount with currency symbol (e.g., "$100.00")
	FinalPnL          string // User's final P&L (e.g., "+$1,234.56" or "-$567.89")
	TraggePointGain   string // T-Point gained (e.g., "15.5")
	ResultsURL        string // Link to contest results page
}

// ContestEndedData contains data for rendering a contest ended email sent to ALL participants.
type ContestEndedData struct {
	UserName          string  // Optional user name
	ContestID         string  // Contest ID
	ContestName       string  // Contest name
	UserRank          int     // User's final rank
	TotalParticipants int     // Total number of participants
	TotalScore        float64 // User's total P&L score
	FormattedScore    string  // Formatted P&L (e.g., "+$1,234.56" or "-$567.89")
	PrizeWon          int     // Prize won in cents, 0 if no prize
	FormattedPrize    string  // Formatted prize amount (e.g., "$100.00"), empty if no prize
	ResultsURL        string  // Link to contest results page
}

// ContestCancelledData contains data for rendering a contest cancelled email.
type ContestCancelledData struct {
	UserName       string // Optional user name
	ContestID      string // Contest ID
	ContestName    string // Contest name
	Reason         string // Cancellation reason
	ScheduledStart string // Originally scheduled start time (optional)
	RefundAmount   string // Formatted refund amount with currency symbol (e.g., "$10.00"), empty if no refund
	NewBalance     string // User's new wallet balance after refund
	ContestsURL    string // Link to browse other contests
}

// BatchSendResult represents the result of sending emails to multiple recipients.
type BatchSendResult struct {
	Successful []string
	Failed     []BatchSendError
}

// BatchSendError represents a failed email send attempt.
type BatchSendError struct {
	Recipient string
	Error     error
}

// EmailNotifier sends notifications via email using Resend.
type EmailNotifier struct {
	client         *resend.Client
	fromEmail      string
	replyTo        string
	logger         *zap.Logger
	circuitBreaker *circuitbreaker.CircuitBreaker
	templates      map[string]*template.Template
	overrideStore  TemplateOverrideStore
	mu             sync.RWMutex
}

// NewEmailNotifier creates a new EmailNotifier with the given configuration.
func NewEmailNotifier(cfg EmailConfig, logger *zap.Logger) (*EmailNotifier, error) {
	if cfg.APIKey == "" {
		return nil, ErrEmailAPIKeyEmpty
	}

	if cfg.FromEmail == "" {
		cfg.FromEmail = "onboarding@resend.dev"
	}

	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("email: failed to create logger: %w", err)
		}
	}
	logger = ensureRedactingLogger(logger)
	logger = logger.With(zap.String("component", "email-notifier"))

	// Create Resend client
	client := resend.NewClient(cfg.APIKey)

	// Create circuit breaker with config for Resend (lower rate limits than Discord)
	cb := circuitbreaker.New(circuitbreaker.Config{
		Name:         "resend-email",
		MaxFailures:  5,
		ResetTimeout: 120 * time.Second,
		Timeout:      30 * time.Second,
		OnStateChange: func(name string, from, to circuitbreaker.State) {
			logger.Info("circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		},
	})

	// Parse embedded templates (cached via sync.Once)
	templates, err := getCachedTemplates()
	if err != nil {
		return nil, fmt.Errorf("email: failed to parse templates: %w", err)
	}

	return &EmailNotifier{
		client:         client,
		fromEmail:      cfg.FromEmail,
		replyTo:        cfg.ReplyTo,
		logger:         logger,
		circuitBreaker: cb,
		templates:      templates,
	}, nil
}

// getCachedTemplates returns the parsed templates, parsing them only on the first call.
func getCachedTemplates() (map[string]*template.Template, error) {
	templateOnce.Do(func() {
		cachedTemplates, cachedTemplatesErr = parseTemplates()
	})
	return cachedTemplates, cachedTemplatesErr
}

// parseTemplates parses all embedded HTML templates.
func parseTemplates() (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template)

	templateFiles := []string{
		"templates/bug_report.html",
		"templates/daily_digest.html",
		"templates/contest_summary.html",
		"templates/contest_starting.html",
		"templates/contest_ending.html",
		"templates/contest_started.html",
		"templates/contest_cancelled.html",
		"templates/kyc_approved.html",
		"templates/kyc_rejected.html",
		"templates/kyc_info_request.html",
		"templates/password_reset.html",
		"templates/password_reset_code.html",
		"templates/email_verification.html",
		"templates/email_verification_fa.html",
		"templates/welcome.html",
		"templates/withdrawal_approved.html",
		"templates/withdrawal_rejected.html",
		"templates/withdrawal_processing.html",
		"templates/withdrawal_completed.html",
		"templates/deposit_confirmed.html",
		"templates/prize_won.html",
		"templates/contest_ended.html",
		"templates/password_changed.html",
	}

	for _, file := range templateFiles {
		content, err := templateFS.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read template %s: %w", file, err)
		}

		tmpl, err := template.New(file).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", file, err)
		}

		// Extract template name without path and extension
		name := strings.TrimPrefix(file, "templates/")
		name = strings.TrimSuffix(name, ".html")
		templates[name] = tmpl
	}

	return templates, nil
}

// Channel returns the channel type (implements ChannelSender).
func (e *EmailNotifier) Channel() Channel {
	return ChannelEmail
}

// SendAlert sends an alert notification via email (implements ChannelSender).
func (e *EmailNotifier) SendAlert(ctx context.Context, alert Alert) error {
	// Convert alert to bug report format
	data := BugReportData{
		Title:         alert.Title,
		Message:       alert.Message,
		Severity:      strings.ToUpper(string(alert.Severity)),
		SeverityColor: severityToEmailColor(alert.Severity),
		Service:       alert.Service,
		Timestamp:     alert.Timestamp.Format(time.RFC3339),
		TraceID:       alert.TraceID,
		SpanID:        alert.SpanID,
		Metadata:      alert.Metadata,
	}

	// Extract stack trace from metadata if present
	if stackTrace, ok := alert.Metadata["stack_trace"]; ok {
		data.StackTrace = stackTrace
		delete(data.Metadata, "stack_trace")
	}

	subject := fmt.Sprintf("[%s] %s - %s", data.Severity, alert.Type.String(), alert.Title)

	html, err := e.renderTemplate("bug_report", data)
	if err != nil {
		return err
	}

	// Get recipients from metadata or use empty slice (caller should provide)
	var recipients []string
	if to, ok := alert.Metadata["email_recipients"]; ok {
		recipients = strings.Split(to, ",")
		for i := range recipients {
			recipients[i] = strings.TrimSpace(recipients[i])
		}
	}

	if len(recipients) == 0 {
		e.logger.Debug("no email recipients specified for alert",
			zap.String("alert_id", alert.ID))
		return nil
	}

	return e.SendEmail(ctx, recipients, subject, html)
}

// SendInfo sends an info notification via email (implements ChannelSender).
func (e *EmailNotifier) SendInfo(ctx context.Context, info Info) error {
	subject := fmt.Sprintf("[INFO] %s", info.Title)

	// Create a simple HTML email for info
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: sans-serif; padding: 20px;">
<h2>%s</h2>
<p>%s</p>
<hr>
<p style="color: #666; font-size: 12px;">Service: %s | Time: %s</p>
</body>
</html>`,
		template.HTMLEscapeString(info.Title),
		template.HTMLEscapeString(info.Message),
		template.HTMLEscapeString(info.Service),
		info.Timestamp.Format(time.RFC3339),
	)

	// Get recipients from metadata
	var recipients []string
	if to, ok := info.Metadata["email_recipients"]; ok {
		recipients = strings.Split(to, ",")
		for i := range recipients {
			recipients[i] = strings.TrimSpace(recipients[i])
		}
	}

	if len(recipients) == 0 {
		e.logger.Debug("no email recipients specified for info",
			zap.String("info_id", info.ID))
		return nil
	}

	return e.SendEmail(ctx, recipients, subject, html)
}

// SendEmail sends a raw HTML email to the specified recipients.
func (e *EmailNotifier) SendEmail(ctx context.Context, to []string, subject, html string) error {
	if len(to) == 0 {
		return ErrEmailNoRecipients
	}

	// Execute through circuit breaker
	err := e.circuitBreaker.ExecuteWithContext(ctx, func(ctx context.Context) error {
		params := &resend.SendEmailRequest{
			From:    e.fromEmail,
			To:      to,
			Subject: subject,
			Html:    html,
		}

		if e.replyTo != "" {
			params.ReplyTo = e.replyTo
		}

		sent, err := e.client.Emails.Send(params)
		if err != nil {
			e.logger.Error("failed to send email",
				zap.Strings("to", to),
				zap.String("subject", subject),
				zap.Error(err))
			return err
		}

		e.logger.Info("email sent successfully",
			zap.String("id", sent.Id),
			zap.Strings("to", to),
			zap.String("subject", subject))

		return nil
	})

	if err != nil {
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
			e.logger.Warn("circuit breaker is open, skipping email")
			return ErrEmailCircuitOpen
		}
		return fmt.Errorf("email: failed to send: %w", err)
	}

	return nil
}

// SendBugReport sends a bug report email using the bug report template.
func (e *EmailNotifier) SendBugReport(ctx context.Context, to []string, data BugReportData) error {
	if len(to) == 0 {
		return ErrEmailNoRecipients
	}

	// Set default severity color if not provided
	if data.SeverityColor == "" {
		data.SeverityColor = severityToEmailColor(Severity(strings.ToLower(data.Severity)))
	}

	html, err := e.renderTemplate("bug_report", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("[BUG] [%s] %s", data.Severity, data.Title)

	return e.SendEmail(ctx, to, subject, html)
}

// SendDailyDigest sends a daily digest email using the daily digest template.
func (e *EmailNotifier) SendDailyDigest(ctx context.Context, to []string, digest DailyDigest) error {
	if len(to) == 0 {
		return ErrEmailNoRecipients
	}

	// Set generated time if not provided
	if digest.GeneratedAt == "" {
		digest.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}

	html, err := e.renderTemplate("daily_digest", digest)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Daily Digest - %s", digest.Date)

	return e.SendEmail(ctx, to, subject, html)
}

// SendContestSummary sends a contest summary email using the contest summary template.
func (e *EmailNotifier) SendContestSummary(ctx context.Context, to []string, summary ContestSummary) error {
	if len(to) == 0 {
		return ErrEmailNoRecipients
	}

	// Set generated time if not provided
	if summary.GeneratedAt == "" {
		summary.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}

	html, err := e.renderTemplate("contest_summary", summary)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Contest Summary - %s", summary.ContestName)

	return e.SendEmail(ctx, to, subject, html)
}

// SendBatch sends emails to multiple recipients with proper error handling.
// Returns partial success information.
func (e *EmailNotifier) SendBatch(ctx context.Context, recipients []string, subject, html string) *BatchSendResult {
	result := &BatchSendResult{
		Successful: make([]string, 0),
		Failed:     make([]BatchSendError, 0),
	}

	if len(recipients) == 0 {
		return result
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, recipient := range recipients {
		wg.Add(1)
		go func(to string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					defer mu.Unlock()
					result.Failed = append(result.Failed, BatchSendError{
						Recipient: to,
						Error:     redactedPanicError(r),
					})
				}
			}()

			err := e.SendEmail(ctx, []string{to}, subject, html)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.Failed = append(result.Failed, BatchSendError{
					Recipient: to,
					Error:     err,
				})
			} else {
				result.Successful = append(result.Successful, to)
			}
		}(recipient)
	}

	wg.Wait()
	return result
}

// renderTemplate renders an HTML template with the given data.
// If a template override store is configured, it checks for a custom template first.
func (e *EmailNotifier) renderTemplate(name string, data interface{}) (string, error) {
	return e.renderTemplateWithContext(context.Background(), name, data)
}

// renderTemplateWithContext renders an HTML template with the given data and context.
// Resolution order:
// 1. Active version from email_template_versions (composed from html_body + css + fonts)
// 2. Custom override from email_templates.html_content
// 3. Embedded default template
func (e *EmailNotifier) renderTemplateWithContext(ctx context.Context, name string, data interface{}) (string, error) {
	if e.overrideStore != nil {
		// Step 1: Check for an active template version
		activeVersion, err := e.overrideStore.GetActiveVersion(ctx, name)
		if err != nil {
			e.logger.Warn("failed to check active template version, continuing fallback",
				zap.String("template", name),
				zap.Error(err))
		} else if activeVersion != nil {
			// Compose full HTML from version components
			composed := ComposeEmailHTMLFromJSON(activeVersion.HTMLBody, activeVersion.CSSContent, activeVersion.FontConfig)
			result, err := executeSandboxedTemplate(name, composed, data)
			if err != nil {
				e.logger.Error("failed to execute active version template, falling back",
					zap.String("template", name),
					zap.Error(err))
			} else {
				return result, nil
			}
		}

		// Step 2: Fall back to email_templates.html_content
		customContent, found, err := e.overrideStore.GetTemplate(ctx, name)
		if err != nil {
			e.logger.Warn("failed to check template override, using default",
				zap.String("template", name),
				zap.Error(err))
		} else if found && customContent != "" {
			result, err := executeSandboxedTemplate(name, customContent, data)
			if err != nil {
				e.logger.Error("failed to execute custom template, falling back to default",
					zap.String("template", name),
					zap.Error(err))
			} else {
				return result, nil
			}
		}
	}

	// Step 3: Fall back to embedded default template
	e.mu.RLock()
	tmpl, ok := e.templates[name]
	e.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: template %s not found", ErrEmailTemplateError, name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("%w: %v", ErrEmailTemplateError, err)
	}

	return buf.String(), nil
}

// SetOverrideStore sets the template override store for custom templates.
func (e *EmailNotifier) SetOverrideStore(store TemplateOverrideStore) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.overrideStore = store
}

// GetDefaultTemplate returns the embedded default template content by slug.
func (e *EmailNotifier) GetDefaultTemplate(slug string) (string, error) {
	content, err := templateFS.ReadFile("templates/" + slug + ".html")
	if err != nil {
		return "", fmt.Errorf("template %s not found: %w", slug, err)
	}
	return string(content), nil
}

// GetTemplateNames returns a list of all available template names (slugs).
func (e *EmailNotifier) GetTemplateNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make([]string, 0, len(e.templates))
	for name := range e.templates {
		names = append(names, name)
	}
	return names
}

// RenderTemplatePreview renders a template with sample data for preview purposes.
// It uses the provided htmlContent if not empty, otherwise uses the current template.
func (e *EmailNotifier) RenderTemplatePreview(ctx context.Context, slug, htmlContent string) (string, error) {
	// Get sample data for the template
	sampleData := e.getSampleDataForTemplate(slug)

	if htmlContent != "" {
		// Parse and render the provided HTML content
		tmpl, err := template.New(slug).Parse(htmlContent)
		if err != nil {
			return "", fmt.Errorf("invalid template syntax: %w", err)
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, sampleData); err != nil {
			return "", fmt.Errorf("template execution error: %w", err)
		}
		return buf.String(), nil
	}

	// Use current template (with override if exists)
	return e.renderTemplateWithContext(ctx, slug, sampleData)
}

// getSampleDataForTemplate returns sample data for previewing a template.
func (e *EmailNotifier) getSampleDataForTemplate(slug string) interface{} {
	switch slug {
	case "welcome":
		return WelcomeEmailData{
			UserEmail:        "user@example.com",
			VerificationCode: "847293",
			DashboardURL:     "https://tragge.com/dashboard",
			Lang:             "en",
		}
	case "email_verification":
		return EmailVerificationData{
			UserName:         "John Doe",
			VerificationCode: "847293",
		}
	case "password_reset":
		return PasswordResetData{
			UserName: "John Doe",
			ResetURL: "https://tragge.com/reset?token=sample123",
		}
	case "kyc_approved":
		return KYCApprovedData{
			UserName:     "John Doe",
			ExpiresAt:    "December 31, 2026",
			DashboardURL: "https://tragge.com/dashboard",
		}
	case "kyc_rejected":
		return KYCRejectedData{
			UserName:        "John Doe",
			Reason:          "Document is unreadable or blurry",
			VerificationURL: "https://tragge.com/kyc",
		}
	case "kyc_info_request":
		return KYCInfoRequestData{
			UserName:        "John Doe",
			Message:         "Please provide a clearer image of your ID document.",
			VerificationURL: "https://tragge.com/kyc",
		}
	case "deposit_confirmed":
		return DepositConfirmedData{
			UserName:      "John Doe",
			Amount:        "$100.00",
			NewBalance:    "$500.00",
			Date:          "January 15, 2026",
			TransactionID: "txn_abc123xyz",
			WalletURL:     "https://tragge.com/wallet",
		}
	case "withdrawal_approved":
		return WithdrawalApprovedData{
			UserName:     "John Doe",
			Amount:       "$50.00",
			AdminComment: "Approved for payout",
			DashboardURL: "https://tragge.com/dashboard",
		}
	case "withdrawal_rejected":
		return WithdrawalRejectedData{
			UserName:     "John Doe",
			Amount:       "$50.00",
			Reason:       "Insufficient verification documents",
			DashboardURL: "https://tragge.com/dashboard",
		}
	case "withdrawal_processing":
		return WithdrawalProcessingData{
			UserName:     "John Doe",
			Amount:       "$50.00",
			DashboardURL: "https://tragge.com/dashboard",
		}
	case "withdrawal_completed":
		return WithdrawalCompletedData{
			UserName:     "John Doe",
			Amount:       "$50.00",
			DashboardURL: "https://tragge.com/dashboard",
		}
	case "contest_starting":
		return ContestStartingData{
			ContestID:        "contest_123",
			ContestName:      "Weekly Trading Championship",
			StartTime:        "January 20, 2026 10:00 AM UTC",
			EndTime:          "January 20, 2026 6:00 PM UTC",
			Duration:         "8 hours",
			TimeUntilStart:   "15 minutes",
			StartingBalance:  "$100,000",
			ParticipantCount: 150,
			Symbols:          []string{"AAPL", "GOOGL", "MSFT", "TSLA"},
			TradingURL:       "https://tragge.com/trade",
		}
	case "contest_ending":
		return ContestEndingData{
			ContestID:        "contest_123",
			ContestName:      "Weekly Trading Championship",
			EndTime:          "January 20, 2026 6:00 PM UTC",
			TimeUntilEnd:     "15 minutes",
			Duration:         "8 hours",
			StartingBalance:  "$100,000",
			ParticipantCount: 150,
			Symbols:          []string{"AAPL", "GOOGL", "MSFT", "TSLA"},
			TradingURL:       "https://tragge.com/trade",
		}
	case "contest_started":
		return ContestStartedData{
			ContestName: "Weekly Trading Championship",
			ContestID:   "contest_123",
			TradeURL:    "https://tragge.com/trade",
			EndsAt:      "January 20, 2026 6:00 PM UTC",
		}
	case "contest_cancelled":
		return ContestCancelledData{
			UserName:       "John Doe",
			ContestID:      "contest_123",
			ContestName:    "Weekly Trading Championship",
			Reason:         "Insufficient participants",
			ScheduledStart: "January 20, 2026 10:00 AM UTC",
			RefundAmount:   "$10.00",
			NewBalance:     "$510.00",
			ContestsURL:    "https://tragge.com/contests",
		}
	case "contest_summary":
		return ContestSummary{
			ContestID:         "contest_123",
			ContestName:       "Weekly Trading Championship",
			Status:            "completed",
			StartDate:         "January 20, 2026",
			EndDate:           "January 20, 2026",
			TotalParticipants: 150,
			TotalTrades:       5234,
			TotalVolume:       "$15,234,567.89",
			PrizePool:         "$1,000.00",
			Winners: []ContestWinner{
				{Rank: 1, Username: "trader1", PnL: 15234.56, Prize: "$500.00"},
				{Rank: 2, Username: "trader2", PnL: 12345.67, Prize: "$300.00"},
				{Rank: 3, Username: "trader3", PnL: 9876.54, Prize: "$200.00"},
			},
			Statistics: []ContestStatistic{
				{Label: "Average Trades", Value: "35"},
				{Label: "Top Symbol", Value: "AAPL"},
			},
			TopSymbols: []SymbolStats{
				{Symbol: "AAPL", Volume: "$5,000,000", Trades: 1500},
				{Symbol: "GOOGL", Volume: "$3,000,000", Trades: 1000},
			},
			GeneratedAt: "January 21, 2026 12:00 AM UTC",
		}
	case "contest_ended":
		return ContestEndedData{
			UserName:          "John Doe",
			ContestID:         "contest_123",
			ContestName:       "Weekly Trading Championship",
			UserRank:          12,
			TotalParticipants: 150,
			TotalScore:        1234.56,
			FormattedScore:    "+$1,234.56",
			PrizeWon:          0,
			FormattedPrize:    "",
			ResultsURL:        "https://tragge.com/contests/contest_123/results",
		}
	case "prize_won":
		return PrizeWonData{
			UserName:          "John Doe",
			ContestID:         "contest_123",
			ContestName:       "Weekly Trading Championship",
			FinalRank:         1,
			TotalParticipants: 150,
			PrizeAmount:       "$500.00",
			FinalPnL:          "+$15,234.56",
			TraggePointGain:   "25.5",
			ResultsURL:        "https://tragge.com/contests/contest_123/results",
		}
	case "daily_digest":
		return DailyDigest{
			Date:          "January 15, 2026",
			TotalAlerts:   12,
			CriticalCount: 0,
			ResolvedCount: 10,
			Services: []ServiceHealth{
				{Name: "trading-engine", Status: "healthy", Uptime: 99.99},
				{Name: "market-ingestor", Status: "healthy", Uptime: 99.95},
			},
			Alerts: []AlertSummary{
				{Title: "High memory usage", Severity: "medium", Service: "trading-engine", Count: 2, LastOccurrence: "2 hours ago"},
			},
			TopErrors: []ErrorSummary{
				{Message: "Connection timeout", Count: 5},
			},
			GeneratedAt: "January 15, 2026 11:59 PM UTC",
		}
	case "bug_report":
		return BugReportData{
			Title:         "Database Connection Error",
			Message:       "Failed to connect to primary database after 3 retries",
			Severity:      "HIGH",
			SeverityColor: "#ea580c",
			Service:       "user-bff",
			Timestamp:     "2026-01-15T10:30:00Z",
			TraceID:       "trace_abc123",
			SpanID:        "span_xyz789",
			StackTrace:    "at db.connect() line 45\nat main.go line 123",
			Metadata: map[string]string{
				"host":    "db-primary.tragge.internal",
				"retries": "3",
			},
		}
	default:
		return map[string]string{
			"UserName": "John Doe",
			"Message":  "Sample message content",
		}
	}
}

// severityToEmailColor returns the HTML color code for a severity level.
func severityToEmailColor(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "#dc2626" // Red
	case SeverityHigh:
		return "#ea580c" // Orange
	case SeverityMedium:
		return "#ca8a04" // Yellow
	case SeverityLow:
		return "#16a34a" // Green
	case SeverityInfo:
		return "#2563eb" // Blue
	default:
		return "#6b7280" // Gray
	}
}

// CircuitBreakerState returns the current state of the circuit breaker.
func (e *EmailNotifier) CircuitBreakerState() circuitbreaker.State {
	return e.circuitBreaker.State()
}

// CircuitBreakerMetrics returns metrics from the circuit breaker.
func (e *EmailNotifier) CircuitBreakerMetrics() circuitbreaker.Metrics {
	return e.circuitBreaker.Metrics()
}

// ResetCircuitBreaker resets the circuit breaker to closed state.
func (e *EmailNotifier) ResetCircuitBreaker() {
	e.circuitBreaker.Reset()
}

// SendKYCApproved sends a KYC approved notification email.
func (e *EmailNotifier) SendKYCApproved(ctx context.Context, to string, data KYCApprovedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default dashboard URL if not provided
	if data.DashboardURL == "" {
		data.DashboardURL = "#"
	}

	html, err := e.renderTemplate("kyc_approved", data)
	if err != nil {
		return err
	}

	subject := "Your Identity Verification is Approved!"

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendKYCRejected sends a KYC rejected notification email.
func (e *EmailNotifier) SendKYCRejected(ctx context.Context, to string, data KYCRejectedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default verification URL if not provided
	if data.VerificationURL == "" {
		data.VerificationURL = "#"
	}

	html, err := e.renderTemplate("kyc_rejected", data)
	if err != nil {
		return err
	}

	subject := "Identity Verification Update Required"

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendKYCInfoRequest sends a KYC info request notification email.
func (e *EmailNotifier) SendKYCInfoRequest(ctx context.Context, to string, data KYCInfoRequestData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default verification URL if not provided
	if data.VerificationURL == "" {
		data.VerificationURL = "#"
	}

	html, err := e.renderTemplate("kyc_info_request", data)
	if err != nil {
		return err
	}

	subject := "Additional Information Required for Verification"

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendPasswordReset sends a password reset email.
func (e *EmailNotifier) SendPasswordReset(ctx context.Context, to string, data PasswordResetData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	if data.ResetURL == "" {
		return fmt.Errorf("email: reset URL is required")
	}

	html, err := e.renderTemplate("password_reset", data)
	if err != nil {
		return err
	}

	subject := "Reset Your Tragge Password"

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendPasswordResetCode sends a password reset code email (OTP-based flow).
// Supports English (lang="en") and Farsi (default) templates.
func (e *EmailNotifier) SendPasswordResetCode(ctx context.Context, to string, data PasswordResetCodeData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}
	if data.Code == "" {
		return fmt.Errorf("email: reset code is required")
	}

	templateName := "password_reset_code"
	subject := "کد بازیابی رمز عبور Tragge"
	if data.Lang == "en" {
		templateName = "password_reset_code_en"
		subject = "Tragge Password Reset Code"
	}

	html, err := e.renderTemplate(templateName, data)
	if err != nil {
		// Fallback to default template if language-specific template doesn't exist
		html, err = e.renderTemplate("password_reset_code", data)
		if err != nil {
			return err
		}
	}
	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendPasswordChanged sends a password changed notification email.
func (e *EmailNotifier) SendPasswordChanged(ctx context.Context, to string, data PasswordChangedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}
	// Map method to Persian display text
	methodDisplay := "تغییر از پنل کاربری"
	if data.Method == "forgot_password" {
		methodDisplay = "بازیابی رمز عبور"
	}
	data.Method = methodDisplay

	html, err := e.renderTemplate("password_changed", data)
	if err != nil {
		return err
	}
	subject := "تغییر رمز عبور حساب Tragge"
	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendEmailVerification sends an email verification email.
// Supports English (default) and Farsi (lang="fa") templates.
func (e *EmailNotifier) SendEmailVerification(ctx context.Context, to string, data EmailVerificationData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	if data.VerificationCode == "" {
		return fmt.Errorf("email: verification code is required")
	}

	templateName := "email_verification"
	subject := "Verify Your Tragge Email Address"
	if data.Lang == "fa" {
		templateName = "email_verification_fa"
		subject = "تأیید ایمیل تریج"
	}

	html, err := e.renderTemplate(templateName, data)
	if err != nil {
		return err
	}

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendWelcomeEmail sends a welcome email to a new user.
// If VerificationCode is provided, the email will include the 6-digit OTP code.
// Supports English (default) and Farsi (lang="fa").
func (e *EmailNotifier) SendWelcomeEmail(ctx context.Context, to string, data WelcomeEmailData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default dashboard URL if not provided
	if data.DashboardURL == "" {
		data.DashboardURL = "#"
	}

	// Default to English if no language specified
	if data.Lang == "" {
		data.Lang = "en"
	}

	html, err := e.renderTemplate("welcome", data)
	if err != nil {
		return err
	}

	// Use appropriate subject based on language
	var subject string
	if data.Lang == "fa" {
		subject = "به Tragge خوش آمدید!"
	} else {
		subject = "Welcome to Tragge!"
	}

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendWithdrawalApproved sends a withdrawal approved notification email.
func (e *EmailNotifier) SendWithdrawalApproved(ctx context.Context, to string, data WithdrawalApprovedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default dashboard URL if not provided
	if data.DashboardURL == "" {
		data.DashboardURL = "#"
	}

	html, err := e.renderTemplate("withdrawal_approved", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Withdrawal Approved - %s", data.Amount)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendWithdrawalRejected sends a withdrawal rejected notification email.
func (e *EmailNotifier) SendWithdrawalRejected(ctx context.Context, to string, data WithdrawalRejectedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default dashboard URL if not provided
	if data.DashboardURL == "" {
		data.DashboardURL = "#"
	}

	html, err := e.renderTemplate("withdrawal_rejected", data)
	if err != nil {
		return err
	}

	subject := "Withdrawal Request Update"

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendWithdrawalProcessing sends a withdrawal processing notification email.
func (e *EmailNotifier) SendWithdrawalProcessing(ctx context.Context, to string, data WithdrawalProcessingData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default dashboard URL if not provided
	if data.DashboardURL == "" {
		data.DashboardURL = "#"
	}

	html, err := e.renderTemplate("withdrawal_processing", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Withdrawal Processing - %s", data.Amount)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendWithdrawalCompleted sends a withdrawal completed notification email.
func (e *EmailNotifier) SendWithdrawalCompleted(ctx context.Context, to string, data WithdrawalCompletedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default dashboard URL if not provided
	if data.DashboardURL == "" {
		data.DashboardURL = "#"
	}

	html, err := e.renderTemplate("withdrawal_completed", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Withdrawal Completed - %s", data.Amount)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendDepositConfirmed sends a deposit confirmed notification email.
func (e *EmailNotifier) SendDepositConfirmed(ctx context.Context, to string, data DepositConfirmedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default wallet URL if not provided
	if data.WalletURL == "" {
		data.WalletURL = "#"
	}

	html, err := e.renderTemplate("deposit_confirmed", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Deposit Confirmed - %s", data.Amount)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendContestStartingReminder sends a contest starting reminder email.
func (e *EmailNotifier) SendContestStartingReminder(ctx context.Context, to string, data ContestStartingData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default trading URL if not provided
	if data.TradingURL == "" {
		data.TradingURL = "#"
	}

	html, err := e.renderTemplate("contest_starting", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Your Contest Starts Soon - %s", data.ContestName)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendContestStartingReminderBatch sends contest starting reminder emails to multiple recipients.
// Returns partial success information for batch processing.
func (e *EmailNotifier) SendContestStartingReminderBatch(ctx context.Context, recipients []string, data ContestStartingData) *BatchSendResult {
	if len(recipients) == 0 {
		return &BatchSendResult{
			Successful: make([]string, 0),
			Failed:     make([]BatchSendError, 0),
		}
	}

	// Set default trading URL if not provided
	if data.TradingURL == "" {
		data.TradingURL = "#"
	}

	html, err := e.renderTemplate("contest_starting", data)
	if err != nil {
		// Return all as failed
		result := &BatchSendResult{
			Successful: make([]string, 0),
			Failed:     make([]BatchSendError, 0, len(recipients)),
		}
		for _, r := range recipients {
			result.Failed = append(result.Failed, BatchSendError{
				Recipient: r,
				Error:     err,
			})
		}
		return result
	}

	subject := fmt.Sprintf("Your Contest Starts Soon - %s", data.ContestName)

	return e.SendBatch(ctx, recipients, subject, html)
}

// SendContestEndingReminder sends a contest ending reminder email.
func (e *EmailNotifier) SendContestEndingReminder(ctx context.Context, to string, data ContestEndingData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default trading URL if not provided
	if data.TradingURL == "" {
		data.TradingURL = "#"
	}

	html, err := e.renderTemplate("contest_ending", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Contest Ending Soon — %s", data.ContestName)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendContestEndingReminderBatch sends contest ending reminder emails to multiple recipients.
// Returns partial success information for batch processing.
func (e *EmailNotifier) SendContestEndingReminderBatch(ctx context.Context, recipients []string, data ContestEndingData) *BatchSendResult {
	if len(recipients) == 0 {
		return &BatchSendResult{
			Successful: make([]string, 0),
			Failed:     make([]BatchSendError, 0),
		}
	}

	// Set default trading URL if not provided
	if data.TradingURL == "" {
		data.TradingURL = "#"
	}

	html, err := e.renderTemplate("contest_ending", data)
	if err != nil {
		// Return all as failed
		result := &BatchSendResult{
			Successful: make([]string, 0),
			Failed:     make([]BatchSendError, 0, len(recipients)),
		}
		for _, r := range recipients {
			result.Failed = append(result.Failed, BatchSendError{
				Recipient: r,
				Error:     err,
			})
		}
		return result
	}

	subject := fmt.Sprintf("Contest Ending Soon — %s", data.ContestName)

	return e.SendBatch(ctx, recipients, subject, html)
}

// SendContestStarted sends a contest started notification email to a single recipient.
func (e *EmailNotifier) SendContestStarted(ctx context.Context, to string, data ContestStartedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default trade URL if not provided
	if data.TradeURL == "" {
		data.TradeURL = "#"
	}

	html, err := e.renderTemplate("contest_started", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Contest Started — %s is Live!", data.ContestName)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// SendContestStartedBatch sends contest started notification emails to multiple recipients.
// All recipients receive the same content. Returns partial success information.
func (e *EmailNotifier) SendContestStartedBatch(ctx context.Context, recipients []string, data ContestStartedData) *BatchSendResult {
	if len(recipients) == 0 {
		return &BatchSendResult{
			Successful: make([]string, 0),
			Failed:     make([]BatchSendError, 0),
		}
	}

	// Set default trade URL if not provided
	if data.TradeURL == "" {
		data.TradeURL = "#"
	}

	html, err := e.renderTemplate("contest_started", data)
	if err != nil {
		// Return all as failed
		result := &BatchSendResult{
			Successful: make([]string, 0),
			Failed:     make([]BatchSendError, 0, len(recipients)),
		}
		for _, r := range recipients {
			result.Failed = append(result.Failed, BatchSendError{
				Recipient: r,
				Error:     err,
			})
		}
		return result
	}

	subject := fmt.Sprintf("Contest Started — %s is Live!", data.ContestName)

	return e.SendBatch(ctx, recipients, subject, html)
}

// SendPrizeWon sends a prize won notification email to a single recipient.
func (e *EmailNotifier) SendPrizeWon(ctx context.Context, to string, data PrizeWonData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default results URL if not provided
	if data.ResultsURL == "" {
		data.ResultsURL = "#"
	}

	html, err := e.renderTemplate("prize_won", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Congratulations! You Won a Prize - %s", data.ContestName)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// PrizeWonRecipient represents a prize winner with their email and prize data.
type PrizeWonRecipient struct {
	Email string
	Data  PrizeWonData
}

// SendPrizeWonBatch sends prize won notification emails to multiple recipients.
// Each recipient has their own personalized data (rank, prize amount, etc.).
// Returns partial success information for batch processing.
func (e *EmailNotifier) SendPrizeWonBatch(ctx context.Context, recipients []PrizeWonRecipient) *BatchSendResult {
	result := &BatchSendResult{
		Successful: make([]string, 0),
		Failed:     make([]BatchSendError, 0),
	}

	if len(recipients) == 0 {
		return result
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, recipient := range recipients {
		wg.Add(1)
		go func(r PrizeWonRecipient) {
			defer wg.Done()
			defer func() {
				if rv := recover(); rv != nil {
					mu.Lock()
					defer mu.Unlock()
					result.Failed = append(result.Failed, BatchSendError{
						Recipient: r.Email,
						Error:     redactedPanicError(rv),
					})
				}
			}()

			err := e.SendPrizeWon(ctx, r.Email, r.Data)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.Failed = append(result.Failed, BatchSendError{
					Recipient: r.Email,
					Error:     err,
				})
			} else {
				result.Successful = append(result.Successful, r.Email)
			}
		}(recipient)
	}

	wg.Wait()
	return result
}

// SendContestCancelled sends a contest cancelled notification email to a single recipient.
func (e *EmailNotifier) SendContestCancelled(ctx context.Context, to string, data ContestCancelledData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default contests URL if not provided
	if data.ContestsURL == "" {
		data.ContestsURL = "#"
	}

	html, err := e.renderTemplate("contest_cancelled", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Contest Cancelled — Refund Issued - %s", data.ContestName)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// ContestCancelledRecipient represents a participant with their email and cancellation data.
type ContestCancelledRecipient struct {
	Email string
	Data  ContestCancelledData
}

// SendContestCancelledBatch sends contest cancelled notification emails to multiple recipients.
// Each recipient has their own personalized data (refund amount, new balance, etc.).
// Returns partial success information for batch processing.
func (e *EmailNotifier) SendContestCancelledBatch(ctx context.Context, recipients []ContestCancelledRecipient) *BatchSendResult {
	result := &BatchSendResult{
		Successful: make([]string, 0),
		Failed:     make([]BatchSendError, 0),
	}

	if len(recipients) == 0 {
		return result
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, recipient := range recipients {
		wg.Add(1)
		go func(r ContestCancelledRecipient) {
			defer wg.Done()
			defer func() {
				if rv := recover(); rv != nil {
					mu.Lock()
					defer mu.Unlock()
					result.Failed = append(result.Failed, BatchSendError{
						Recipient: r.Email,
						Error:     redactedPanicError(rv),
					})
				}
			}()

			err := e.SendContestCancelled(ctx, r.Email, r.Data)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.Failed = append(result.Failed, BatchSendError{
					Recipient: r.Email,
					Error:     err,
				})
			} else {
				result.Successful = append(result.Successful, r.Email)
			}
		}(recipient)
	}

	wg.Wait()
	return result
}

// SendContestEnded sends a contest ended notification email to a single participant.
// This email is sent to ALL participants (not just winners) with their personalized results.
func (e *EmailNotifier) SendContestEnded(ctx context.Context, to string, data ContestEndedData) error {
	if to == "" {
		return ErrEmailNoRecipients
	}

	// Set default results URL if not provided
	if data.ResultsURL == "" {
		data.ResultsURL = "#"
	}

	html, err := e.renderTemplate("contest_ended", data)
	if err != nil {
		return err
	}

	subject := fmt.Sprintf("Contest Results — %s", data.ContestName)

	return e.SendEmail(ctx, []string{to}, subject, html)
}

// ContestEndedRecipient represents a participant with their email and contest results data.
type ContestEndedRecipient struct {
	Email string
	Data  ContestEndedData
}

// SendContestEndedBatch sends contest ended notification emails to multiple recipients.
// Each recipient has their own personalized data (rank, score, prize, etc.).
// Returns partial success information for batch processing.
func (e *EmailNotifier) SendContestEndedBatch(ctx context.Context, recipients []ContestEndedRecipient) *BatchSendResult {
	result := &BatchSendResult{
		Successful: make([]string, 0),
		Failed:     make([]BatchSendError, 0),
	}

	if len(recipients) == 0 {
		return result
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, recipient := range recipients {
		wg.Add(1)
		go func(r ContestEndedRecipient) {
			defer wg.Done()
			defer func() {
				if rv := recover(); rv != nil {
					mu.Lock()
					defer mu.Unlock()
					result.Failed = append(result.Failed, BatchSendError{
						Recipient: r.Email,
						Error:     redactedPanicError(rv),
					})
				}
			}()

			err := e.SendContestEnded(ctx, r.Email, r.Data)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				result.Failed = append(result.Failed, BatchSendError{
					Recipient: r.Email,
					Error:     err,
				})
			} else {
				result.Successful = append(result.Successful, r.Email)
			}
		}(recipient)
	}

	wg.Wait()
	return result
}

// executeSandboxedTemplate parses and executes a DB-sourced template in a
// restricted environment: no custom FuncMap and only flat, safe values are
// exposed to the template (no methods, no nested structs).
func executeSandboxedTemplate(name, content string, data interface{}) (string, error) {
	tmpl, err := template.New(name).Funcs(template.FuncMap{}).Parse(content)
	if err != nil {
		return "", fmt.Errorf("sandboxed template parse error: %w", err)
	}

	safe := safeTemplateData(data)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, safe); err != nil {
		return "", fmt.Errorf("sandboxed template execute error: %w", err)
	}
	return buf.String(), nil
}

// safeTemplateData converts a struct (or map) into a flat map[string]interface{}
// containing only primitive values (string, int, float, bool, time.Time) and
// map[string]string. This prevents DB-sourced templates from calling methods on
// rich objects.
func safeTemplateData(data interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	if data == nil {
		return result
	}

	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return result
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			fv := v.Field(i)
			if isSafeValue(fv) {
				result[field.Name] = fv.Interface()
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if key.Kind() == reflect.String {
				mv := v.MapIndex(key)
				if isSafeValue(mv) {
					result[key.String()] = mv.Interface()
				}
			}
		}
	}

	return result
}

// isSafeValue returns true if the value is a primitive type safe for template
// interpolation (no callable methods that could be exploited).
func isSafeValue(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Struct:
		if v.Type() == reflect.TypeOf(time.Time{}) {
			return true
		}
		return false
	case reflect.Map:
		if v.Type().Key().Kind() == reflect.String && v.Type().Elem().Kind() == reflect.String {
			return true
		}
		return false
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.String {
			return true
		}
		return false
	default:
		return false
	}
}
