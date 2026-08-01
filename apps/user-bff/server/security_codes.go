package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/notification"
)

const (
	securityCodeTTL         = 10 * time.Minute
	securityCodeCooldown    = 60 * time.Second
	securityCodeMaxAttempts = 5

	securityCodePurposeEmailVerification = "email_verification"
	securityCodePurposePhoneVerification = "phone_verification"
	securityCodePurposePasswordReset     = "password_reset"

	localOnlySecurityCodeHashSecret = "LOCAL-ONLY-security-code-hmac-key-2026-DO-NOT-USE-IN-PRODUCTION"
)

var (
	errSecurityCodeConfiguration = errors.New("security-code delivery configuration is invalid")
	errSecurityCodeUnavailable   = errors.New("security-code delivery is unavailable")
	errUnsupportedCountry        = errors.New("country is missing, malformed, or unsupported")
)

type securityCodeClock interface {
	Now() time.Time
}

type systemSecurityCodeClock struct{}

func (systemSecurityCodeClock) Now() time.Time { return time.Now().UTC() }

func (a *App) securityCodeNow() time.Time {
	if a != nil && a.codeClock != nil {
		return a.codeClock.Now().UTC()
	}
	return systemSecurityCodeClock{}.Now()
}

// securityCodeHasher binds a code to its purpose, User, destination, channel,
// and optional request context. The dedicated key is never reused for JWTs or
// provider authentication.
type securityCodeHasher struct {
	key []byte
}

func newSecurityCodeHasher(key string) (*securityCodeHasher, error) {
	if strings.TrimSpace(key) == "" {
		return nil, errSecurityCodeConfiguration
	}
	return &securityCodeHasher{key: []byte(key)}, nil
}

func (h *securityCodeHasher) Digest(purpose, userID, destination, channel, requestContext, code string) string {
	mac := hmac.New(sha256.New, h.key)
	for _, value := range []string{purpose, userID, strings.ToLower(strings.TrimSpace(destination)), channel, requestContext, code} {
		_, _ = fmt.Fprintf(mac, "%d:", len(value))
		_, _ = mac.Write([]byte(value))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *securityCodeHasher) Matches(stored, purpose, userID, destination, channel, requestContext, code string) bool {
	candidate := h.Digest(purpose, userID, destination, channel, requestContext, code)
	if len(candidate) != len(stored) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(stored)) == 1
}

type securityEmailSender interface {
	Send(context.Context, string, notification.SecurityEmailMessage) error
}

type countrySecurityEmailRouter struct {
	mailerino notification.SecurityEmailProvider
	resend    notification.SecurityEmailProvider
}

func (r *countrySecurityEmailRouter) Send(
	ctx context.Context,
	country string,
	message notification.SecurityEmailMessage,
) error {
	country, err := normalizeSupportedCountry(country)
	if err != nil {
		return err
	}
	var provider notification.SecurityEmailProvider
	if country == "IR" {
		provider = r.mailerino
	} else {
		provider = r.resend
	}
	if provider == nil {
		return errSecurityCodeUnavailable
	}
	if err := provider.SendSecurityEmail(ctx, message); err != nil {
		return errSecurityCodeUnavailable
	}
	return nil
}

func normalizeSupportedCountry(country string) (string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if len(country) != 2 || !validCountryCodes[country] {
		return "", errUnsupportedCountry
	}
	return country, nil
}

func resolveSecurityEnvironment(environment, appEnvironment string) (string, error) {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "dev" {
			return "development"
		}
		return value
	}
	environment = normalize(environment)
	appEnvironment = normalize(appEnvironment)
	if environment != "" && appEnvironment != "" && environment != appEnvironment {
		return "", errSecurityCodeConfiguration
	}
	resolved := environment
	if resolved == "" {
		resolved = appEnvironment
	}
	if resolved == "" {
		return "production", nil
	}
	switch resolved {
	case "development", "local", "test", "staging", "production":
		return resolved, nil
	default:
		return "", errSecurityCodeConfiguration
	}
}
func validateSecurityDeliveryConfig(environment string, cfg *Config) error {
	if cfg == nil {
		return errSecurityCodeConfiguration
	}
	production := strings.EqualFold(strings.TrimSpace(environment), "production") ||
		strings.EqualFold(strings.TrimSpace(environment), "staging")

	for _, rawURL := range []string{cfg.MailerinoBaseURL, cfg.ResendBaseURL} {
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || (production && parsed.Scheme != "https") {
			return errSecurityCodeConfiguration
		}
	}
	if production {
		if cfg.EmailFromAmbiguous {
			return errSecurityCodeConfiguration
		}
		for _, otherSecret := range []string{
			cfg.AuthContext.AccessSecret,
			cfg.AuthContext.RefreshSecret,
			cfg.MailerinoAPIKey,
			cfg.ResendAPIKey,
			cfg.KaveNegarAPIKey,
		} {
			if otherSecret != "" && subtle.ConstantTimeCompare([]byte(cfg.SecurityCodeHashSecret), []byte(otherSecret)) == 1 {
				return errSecurityCodeConfiguration
			}
		}
		if !strongSecuritySecret(cfg.SecurityCodeHashSecret) {
			return errSecurityCodeConfiguration
		}
		if placeholderValue(cfg.MailerinoAPIKey) || placeholderValue(cfg.ResendAPIKey) ||
			placeholderValue(cfg.MailerinoFrom) || placeholderValue(cfg.EmailFrom) {
			return errSecurityCodeConfiguration
		}
		if strings.EqualFold(strings.TrimSpace(cfg.EmailFrom), "onboarding@resend.dev") {
			return errSecurityCodeConfiguration
		}
		if cfg.SMSEnabled && (placeholderValue(cfg.KaveNegarAPIKey) || placeholderValue(cfg.SMSTemplate)) {
			return errSecurityCodeConfiguration
		}
		if !strings.EqualFold(strings.TrimSpace(cfg.SMSProviderMode), "kavenegar") {
			return errSecurityCodeConfiguration
		}
	}
	return nil
}

func placeholderValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{
		"change_me", "changeme", "replace_with", "placeholder", "example",
		"your_", "test-key", "fake-key", "dummy", "secret123",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func strongSecuritySecret(secret string) bool {
	if len(secret) < 32 || placeholderValue(secret) {
		return false
	}
	frequencies := make(map[rune]int)
	runes := []rune(secret)
	for _, r := range runes {
		frequencies[r]++
	}
	if len(frequencies) < 16 {
		return false
	}
	var entropy float64
	for _, count := range frequencies {
		p := float64(count) / float64(len(runes))
		entropy -= p * math.Log2(p)
	}
	return entropy >= 3.5
}
