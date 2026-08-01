package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultMailerinoBaseURL = "https://api.mailerino.com"
	DefaultResendBaseURL    = "https://api.resend.com"

	securityEmailResponseLimit = 64 << 10
)

var (
	// ErrSecurityEmailConfiguration is deliberately free of credential values.
	ErrSecurityEmailConfiguration = errors.New("security email provider configuration is invalid")
	// ErrSecurityEmailDelivery is deliberately generic so provider bodies and keys
	// cannot flow into logs, telemetry, or client responses.
	ErrSecurityEmailDelivery = errors.New("security email delivery was not accepted")
)

// SecurityEmailMessage is a provider-neutral security email. Codes are generated
// and lifecycle-managed by the caller; providers only deliver rendered content.
type SecurityEmailMessage struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// SecurityEmailProvider is the explicit delivery boundary used for verification
// and password-reset messages.
type SecurityEmailProvider interface {
	SendSecurityEmail(context.Context, SecurityEmailMessage) error
	ProviderName() string
}

// HTTPDoer is the smallest HTTP client surface required by provider adapters.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// SecurityEmailHTTPConfig contains shared, non-secret HTTP behavior.
type SecurityEmailHTTPConfig struct {
	BaseURL             string
	APIKey              string
	From                string
	Client              HTTPDoer
	Timeout             time.Duration
	AllowHTTPForTesting bool
}

// MailerinoSecurityEmailProvider sends Iranian-user security email through the
// documented Mailerino v1 send endpoint.
type MailerinoSecurityEmailProvider struct {
	baseURL string
	apiKey  string
	from    string
	client  HTTPDoer
}

// ResendSecurityEmailProvider sends supported non-Iranian-user security email
// through the documented Resend emails endpoint.
type ResendSecurityEmailProvider struct {
	baseURL string
	apiKey  string
	from    string
	client  HTTPDoer
}

// NewMailerinoSecurityEmailProvider constructs the Mailerino adapter.
func NewMailerinoSecurityEmailProvider(cfg SecurityEmailHTTPConfig) (*MailerinoSecurityEmailProvider, error) {
	normalized, err := normalizeSecurityEmailHTTPConfig(cfg, DefaultMailerinoBaseURL)
	if err != nil {
		return nil, err
	}
	return &MailerinoSecurityEmailProvider{
		baseURL: normalized.BaseURL,
		apiKey:  normalized.APIKey,
		from:    normalized.From,
		client:  normalized.Client,
	}, nil
}

// NewResendSecurityEmailProvider constructs the Resend adapter.
func NewResendSecurityEmailProvider(cfg SecurityEmailHTTPConfig) (*ResendSecurityEmailProvider, error) {
	normalized, err := normalizeSecurityEmailHTTPConfig(cfg, DefaultResendBaseURL)
	if err != nil {
		return nil, err
	}
	return &ResendSecurityEmailProvider{
		baseURL: normalized.BaseURL,
		apiKey:  normalized.APIKey,
		from:    normalized.From,
		client:  normalized.Client,
	}, nil
}

func normalizeSecurityEmailHTTPConfig(cfg SecurityEmailHTTPConfig, defaultBaseURL string) (SecurityEmailHTTPConfig, error) {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !(cfg.AllowHTTPForTesting && parsed.Scheme == "http")) {
		return SecurityEmailHTTPConfig{}, ErrSecurityEmailConfiguration
	}
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.From) == "" {
		return SecurityEmailHTTPConfig{}, ErrSecurityEmailConfiguration
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: cfg.Timeout}
	}
	return cfg, nil
}

func (p *MailerinoSecurityEmailProvider) ProviderName() string { return "mailerino" }

func (p *MailerinoSecurityEmailProvider) SendSecurityEmail(ctx context.Context, message SecurityEmailMessage) error {
	payload := struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Subject string `json:"subject"`
		Text    string `json:"text,omitempty"`
		HTML    string `json:"html,omitempty"`
	}{
		From: p.from, To: message.To, Subject: message.Subject, Text: message.Text, HTML: message.HTML,
	}
	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := p.send(ctx, "/v1/send", payload, http.StatusOK, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.ID) == "" || !strings.EqualFold(result.Status, "queued") {
		return ErrSecurityEmailDelivery
	}
	return nil
}

func (p *MailerinoSecurityEmailProvider) send(
	ctx context.Context,
	path string,
	payload any,
	successStatus int,
	result any,
) error {
	return sendSecurityEmailHTTP(ctx, p.client, p.baseURL+path, p.apiKey, payload, successStatus, result)
}

func (p *ResendSecurityEmailProvider) ProviderName() string { return "resend" }

func (p *ResendSecurityEmailProvider) SendSecurityEmail(ctx context.Context, message SecurityEmailMessage) error {
	payload := struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text,omitempty"`
		HTML    string   `json:"html,omitempty"`
	}{
		From: p.from, To: []string{message.To}, Subject: message.Subject, Text: message.Text, HTML: message.HTML,
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := sendSecurityEmailHTTP(ctx, p.client, p.baseURL+"/emails", p.apiKey, payload, 0, &result); err != nil {
		return err
	}
	if strings.TrimSpace(result.ID) == "" {
		return ErrSecurityEmailDelivery
	}
	return nil
}

func sendSecurityEmailHTTP(
	ctx context.Context,
	client HTTPDoer,
	endpoint string,
	apiKey string,
	payload any,
	successStatus int,
	result any,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return ErrSecurityEmailDelivery
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrSecurityEmailDelivery
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tragge-security-email/1")

	resp, err := client.Do(req)
	if err != nil {
		return ErrSecurityEmailDelivery
	}
	defer resp.Body.Close()

	ok := resp.StatusCode == successStatus
	if successStatus == 0 {
		ok = resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated
	}
	if !ok {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, securityEmailResponseLimit))
		return ErrSecurityEmailDelivery
	}

	decoder := json.NewDecoder(io.LimitReader(resp.Body, securityEmailResponseLimit))
	if err := decoder.Decode(result); err != nil {
		return ErrSecurityEmailDelivery
	}
	return nil
}

// RenderSecurityCodeEmail renders an existing embedded template without coupling
// security delivery to a specific provider SDK.
func RenderSecurityCodeEmail(purpose, code, lang string) (SecurityEmailMessage, error) {
	var (
		templateName string
		subject      string
		data         any
	)
	switch purpose {
	case "email_verification":
		templateName = "email_verification"
		subject = "Verify Your Tragge Email Address"
		if lang == "fa" {
			templateName = "email_verification_fa"
			subject = "Tragge email verification"
		}
		data = EmailVerificationData{VerificationCode: code, Lang: lang}
	case "password_reset":
		templateName = "password_reset_code"
		subject = "Tragge Password Reset Code"
		data = PasswordResetCodeData{Code: code, Lang: lang}
	default:
		return SecurityEmailMessage{}, fmt.Errorf("%w: unknown purpose", ErrEmailTemplateError)
	}

	templates, err := getCachedTemplates()
	if err != nil {
		return SecurityEmailMessage{}, ErrEmailTemplateError
	}
	tmpl, ok := templates[templateName]
	if !ok {
		return SecurityEmailMessage{}, ErrEmailTemplateError
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return SecurityEmailMessage{}, ErrEmailTemplateError
	}
	return SecurityEmailMessage{Subject: subject, HTML: rendered.String()}, nil
}
