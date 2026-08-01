package sms

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kavenegar/kavenegar-go"
)

const kaveNegarRequestTimeout = 10 * time.Second

var ErrKaveNegarUnavailable = errors.New("SMS provider did not accept delivery")

// KaveNegarProvider sends SMS via KaveNegar API.
type KaveNegarProvider struct {
	api      *kavenegar.Kavenegar
	client   *kavenegar.Client
	sender   string
	template string
}

// NewKaveNegar creates a new KaveNegar SMS provider with an explicit bounded
// HTTP client. Provider response details and credentials never cross this
// adapter boundary.
func NewKaveNegar(cfg Config) *KaveNegarProvider {
	return newKaveNegar(cfg, kaveNegarRequestTimeout)
}

func newKaveNegar(cfg Config, timeout time.Duration) *KaveNegarProvider {
	if timeout <= 0 {
		timeout = kaveNegarRequestTimeout
	}
	client := kavenegar.NewClient(cfg.APIKey)
	client.BaseClient = &http.Client{Timeout: timeout}
	return &KaveNegarProvider{
		api:      kavenegar.NewWithClient(client),
		client:   client,
		sender:   cfg.Sender,
		template: cfg.Template,
	}
}

// SendOTP sends only through the configured Verify.Lookup template. There is
// intentionally no direct-message fallback: production OTP delivery must fail
// closed when the approved provider path is unavailable.
func (k *KaveNegarProvider) SendOTP(ctx context.Context, phone string, code string) error {
	if err := ctx.Err(); err != nil || k.template == "" {
		return ErrKaveNegarUnavailable
	}
	_, err := k.api.Verify.Lookup(phone, k.template, code, nil)
	if err != nil {
		return ErrKaveNegarUnavailable
	}
	if err := ctx.Err(); err != nil {
		return ErrKaveNegarUnavailable
	}
	return nil
}

// SendMessage sends an arbitrary non-code SMS message.
func (k *KaveNegarProvider) SendMessage(ctx context.Context, phone string, message string) error {
	if err := ctx.Err(); err != nil {
		return ErrKaveNegarUnavailable
	}
	_, err := k.api.Message.Send(k.sender, []string{phone}, message, nil)
	if err != nil {
		return ErrKaveNegarUnavailable
	}
	return nil
}

// HealthCheck verifies KaveNegar API connectivity without exposing diagnostics.
func (k *KaveNegarProvider) HealthCheck() error {
	_, err := k.api.Account.Info()
	if err != nil {
		return ErrKaveNegarUnavailable
	}
	return nil
}
