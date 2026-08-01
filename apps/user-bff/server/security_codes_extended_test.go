package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Parsaeffatravesh/tragge/packages/notification"
)

func TestResolveSecurityEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		appEnv      string
		want        string
		wantErr     bool
	}{
		{name: "unset fails safe", want: "production"},
		{name: "development alias", environment: "dev", want: "development"},
		{name: "matching values", environment: "production", appEnv: "production", want: "production"},
		{name: "ambiguous values", environment: "production", appEnv: "test", wantErr: true},
		{name: "unknown value", environment: "preview", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveSecurityEnvironment(test.environment, test.appEnv)
			if test.wantErr {
				if !errors.Is(err, errSecurityCodeConfiguration) {
					t.Fatalf("expected safe configuration failure, got %v", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("got environment %q and error %v, want %q", got, err, test.want)
			}
		})
	}
}

func TestProductionRejectsUnsupportedSecuritySMSProviders(t *testing.T) {
	valid := validProductionSecurityDeliveryConfig()
	for _, mode := range []string{"", "mock", "fake", "logging", "noop", "smtp", "console"} {
		t.Run(mode, func(t *testing.T) {
			copy := *valid
			copy.SMSProviderMode = mode
			if err := validateSecurityDeliveryConfig("production", &copy); !errors.Is(err, errSecurityCodeConfiguration) {
				t.Fatalf("provider mode %q did not fail closed: %v", mode, err)
			}
		})
	}
	if err := validateSecurityDeliveryConfig("staging", valid); err != nil {
		t.Fatalf("valid staging configuration rejected: %v", err)
	}
}

func TestCountryRouterNeverFallsBackBetweenProviders(t *testing.T) {
	mailerino := &recordingSecurityEmailProvider{name: "mailerino", err: errors.New("fixture delivery rejection")}
	resend := &recordingSecurityEmailProvider{name: "resend"}
	router := &countrySecurityEmailRouter{mailerino: mailerino, resend: resend}
	message := notification.SecurityEmailMessage{To: "recipient@example.test", Subject: "fixture"}
	if err := router.Send(context.Background(), "IR", message); !errors.Is(err, errSecurityCodeUnavailable) {
		t.Fatalf("Mailerino rejection was not sanitized: %v", err)
	}
	if mailerino.calls != 1 || resend.calls != 0 {
		t.Fatal("Iranian provider rejection triggered a cross-provider fallback")
	}

	mailerino.err = nil
	resend.err = errors.New("fixture delivery rejection")
	if err := router.Send(context.Background(), "CA", message); !errors.Is(err, errSecurityCodeUnavailable) {
		t.Fatalf("Resend rejection was not sanitized: %v", err)
	}
	if mailerino.calls != 1 || resend.calls != 1 {
		t.Fatal("foreign provider rejection triggered a cross-provider fallback")
	}
}

func validProductionSecurityDeliveryConfig() *Config {
	return &Config{
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
}
