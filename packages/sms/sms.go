// Package sms provides SMS provider abstraction, OTP generation,
// and Redis-backed verification with rate limiting.
package sms

import "context"

// SMSProvider is the interface for sending SMS messages.
type SMSProvider interface {
	SendOTP(ctx context.Context, phone string, code string) error
	SendMessage(ctx context.Context, phone string, message string) error
	HealthCheck() error
}

// Config holds SMS provider configuration.
type Config struct {
	APIKey   string
	Sender   string // Sender number (dedicated line) — empty = provider default
	Template string // KaveNegar Verify.Lookup template name (e.g. "tragge-verify")
	Enabled  bool   // Set false to disable in dev environments
}
