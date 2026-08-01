package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Parsaeffatravesh/tragge/packages/notification"
)

type recordingSecurityEmailProvider struct {
	name    string
	calls   int
	message notification.SecurityEmailMessage
	err     error
}

func (p *recordingSecurityEmailProvider) ProviderName() string { return p.name }
func (p *recordingSecurityEmailProvider) SendSecurityEmail(_ context.Context, message notification.SecurityEmailMessage) error {
	p.calls++
	p.message = message
	return p.err
}

func TestSecurityCodeHasherBindsEveryContextDimension(t *testing.T) {
	hasher, err := newSecurityCodeHasher("test-only-security-code-hmac-key-0123456789-ABCDEFG")
	if err != nil {
		t.Fatal(err)
	}
	base := hasher.Digest("email_verification", "user-a", "a@example.test", "email", "", "123456")
	tests := []struct {
		name    string
		purpose string
		user    string
		dest    string
		channel string
		context string
		code    string
	}{
		{"purpose", "password_reset", "user-a", "a@example.test", "email", "", "123456"},
		{"user", "email_verification", "user-b", "a@example.test", "email", "", "123456"},
		{"destination", "email_verification", "user-a", "b@example.test", "email", "", "123456"},
		{"channel", "email_verification", "user-a", "a@example.test", "sms", "", "123456"},
		{"request context", "email_verification", "user-a", "a@example.test", "email", "ctx", "123456"},
		{"code", "email_verification", "user-a", "a@example.test", "email", "", "654321"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := hasher.Digest(test.purpose, test.user, test.dest, test.channel, test.context, test.code)
			if got == base {
				t.Fatal("digest did not bind changed context")
			}
		})
	}
	if !hasher.Matches(base, "email_verification", "user-a", "a@example.test", "email", "", "123456") {
		t.Fatal("expected constant-time comparison to accept the original context")
	}
	if hasher.Matches(base, "email_verification", "user-a", "a@example.test", "email", "", "000000") {
		t.Fatal("wrong code accepted")
	}
}

func TestCountrySecurityEmailRouting(t *testing.T) {
	mailerino := &recordingSecurityEmailProvider{name: "mailerino"}
	resend := &recordingSecurityEmailProvider{name: "resend"}
	router := &countrySecurityEmailRouter{mailerino: mailerino, resend: resend}
	message := notification.SecurityEmailMessage{To: "recipient@example.test", Subject: "fixture"}

	if err := router.Send(context.Background(), "ir", message); err != nil {
		t.Fatal(err)
	}
	if mailerino.calls != 1 || resend.calls != 0 {
		t.Fatal("Iranian country did not route exclusively to Mailerino")
	}
	if err := router.Send(context.Background(), "DE", message); err != nil {
		t.Fatal(err)
	}
	if mailerino.calls != 1 || resend.calls != 1 {
		t.Fatal("non-Iranian country did not route exclusively to Resend")
	}
	for _, invalid := range []string{"", "Iran", "ZZ", "1R"} {
		if err := router.Send(context.Background(), invalid, message); !errors.Is(err, errUnsupportedCountry) {
			t.Fatalf("%q did not fail closed: %v", invalid, err)
		}
	}
}

func TestProductionSecurityDeliveryConfig(t *testing.T) {
	valid := &Config{
		SecurityCodeHashSecret: "fixture-only-random-looking-security-key-01aB!92zQ#4vL",
		MailerinoAPIKey:        "mlr_live_fixture_01",
		MailerinoFrom:          "security@fixture.invalid",
		MailerinoBaseURL:       notification.DefaultMailerinoBaseURL,
		ResendAPIKey:           "re_live_fixture_01",
		EmailFrom:              "security@fixture.invalid",
		ResendBaseURL:          notification.DefaultResendBaseURL,
		SMSEnabled:             true,
		KaveNegarAPIKey:        "kavenegar-production-fixture-01",
		SMSTemplate:            "tragge-verify",
		SMSProviderMode:        "kavenegar",
	}
	if err := validateSecurityDeliveryConfig("production", valid); err != nil {
		t.Fatalf("valid isolated production fixture rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing hash secret", func(c *Config) { c.SecurityCodeHashSecret = "" }},
		{"weak hash secret", func(c *Config) { c.SecurityCodeHashSecret = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }},
		{"placeholder hash secret", func(c *Config) { c.SecurityCodeHashSecret = "CHANGE_ME_security_code_hash_secret_0123456789" }},
		{"missing Mailerino key", func(c *Config) { c.MailerinoAPIKey = "" }},
		{"missing Mailerino sender", func(c *Config) { c.MailerinoFrom = "" }},
		{"missing Resend key", func(c *Config) { c.ResendAPIKey = "" }},
		{"missing Resend sender", func(c *Config) { c.EmailFrom = "" }},
		{"demo Resend sender", func(c *Config) { c.EmailFrom = "onboarding@resend.dev" }},
		{"missing KaveNegar key", func(c *Config) { c.KaveNegarAPIKey = "" }},
		{"missing SMS template", func(c *Config) { c.SMSTemplate = "" }},
		{"ambiguous sender variables", func(c *Config) { c.EmailFromAmbiguous = true }},
		{"hash key reused as provider key", func(c *Config) { c.MailerinoAPIKey = c.SecurityCodeHashSecret }},
		{"mock SMS provider", func(c *Config) { c.SMSProviderMode = "mock" }},
		{"HTTP Mailerino base URL", func(c *Config) { c.MailerinoBaseURL = "http://api.mailerino.invalid" }},
		{"HTTP Resend base URL", func(c *Config) { c.ResendBaseURL = "http://api.resend.invalid" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copy := *valid
			test.mutate(&copy)
			if err := validateSecurityDeliveryConfig("production", &copy); err == nil {
				t.Fatal("invalid production configuration was accepted")
			}
		})

		localFake := *valid
		localFake.SMSProviderMode = "fake"
		localFake.MailerinoAPIKey = "local-only-fake-mailerino"
		localFake.ResendAPIKey = "local-only-fake-resend"
		if err := validateSecurityDeliveryConfig("test", &localFake); err != nil {
			t.Fatal("explicit local fake configuration was rejected outside production")
		}
		if err := validateSecurityDeliveryConfig("production", &localFake); !errors.Is(err, errSecurityCodeConfiguration) ||
			strings.Contains(err.Error(), localFake.MailerinoAPIKey) || strings.Contains(err.Error(), localFake.ResendAPIKey) {
			t.Fatal("production fake rejection was absent or exposed a credential")
		}
	}
}
