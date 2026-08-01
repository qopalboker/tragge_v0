package notification

import (
	"os"
	"strconv"
	"time"
)

// Config holds configuration for the notification service.
//
// Deprecated: Use ServiceConfig with NewService instead. Config is used by the legacy
// NotificationService. See ConfigToServiceConfig for migration.
type Config struct {
	// Service is the name of the service using notifications (for context in alerts)
	Service string

	// Enabled controls whether notifications are sent
	// Default: value of NOTIFICATION_ENABLED env var, or true
	Enabled bool

	// Async controls whether notifications are sent asynchronously
	// When true, SendAlert/SendInfo return immediately and send in background
	// Default: value of NOTIFICATION_ASYNC env var, or true
	Async bool

	// RateLimitPerMinute limits the number of notifications per minute
	// Default: value of NOTIFICATION_RATE_LIMIT env var, or 30
	RateLimitPerMinute int

	// Discord configuration
	Discord DiscordConfig

	// Email configuration
	Email EmailConfig

	// Timeout for synchronous sends
	// Default: 10 seconds
	Timeout time.Duration

	// QueueSize is the size of the async notification queue
	// Default: 1000
	QueueSize int

	// MinSeverity is the minimum severity level to send
	// Alerts below this level are discarded
	// Default: SeverityInfo (send all)
	MinSeverity Severity
}

// DiscordConfig holds Discord webhook configuration.
type DiscordConfig struct {
	// WebhookURL is the Discord webhook URL
	// Default: value of DISCORD_WEBHOOK_URL env var
	WebhookURL string

	// Enabled controls whether Discord notifications are sent
	// Default: true if WebhookURL is set
	Enabled bool

	// Username is the bot username shown in Discord
	// Default: "tragge-notifications"
	Username string

	// AvatarURL is the bot avatar URL (optional)
	AvatarURL string
}

// EmailConfig holds email (Resend) configuration.
type EmailConfig struct {
	// APIKey is the Resend API key
	// Default: value of RESEND_API_KEY env var
	APIKey string

	// FromEmail is the sender email address
	// Default: value of RESEND_FROM_EMAIL env var, or "onboarding@resend.dev"
	FromEmail string

	// ReplyTo is the reply-to email address (optional)
	// Default: empty (no reply-to set)
	ReplyTo string

	// Enabled controls whether email notifications are sent
	// Default: true if APIKey is set
	Enabled bool

	// Recipients is the list of email addresses to notify
	// Default: empty (must be configured)
	Recipients []string
}

// DefaultConfig returns a Config with default values applied.
func DefaultConfig() Config {
	return applyDefaults(Config{})
}

// applyDefaults applies default values to the config from environment variables.
// Environment variables override programmatic values only when explicitly set.
func applyDefaults(cfg Config) Config {
	// Main notification settings — only read env var if it's explicitly set.
	// If env var is set, use its value. If not set, keep cfg.Enabled as-is
	// (which defaults to false for zero-value Config, i.e. DefaultConfig sets true below).
	if envEnabled := os.Getenv("NOTIFICATION_ENABLED"); envEnabled != "" {
		cfg.Enabled = envEnabled == "true" || envEnabled == "1"
	} else if !cfg.Enabled {
		// No env var and not explicitly set — default to true
		cfg.Enabled = true
	}

	if envAsync := os.Getenv("NOTIFICATION_ASYNC"); envAsync != "" {
		cfg.Async = envAsync == "true" || envAsync == "1"
	} else if !cfg.Async {
		cfg.Async = true
	}

	if cfg.RateLimitPerMinute == 0 {
		if rateLimit := os.Getenv("NOTIFICATION_RATE_LIMIT"); rateLimit != "" {
			if parsed, err := strconv.Atoi(rateLimit); err == nil && parsed > 0 {
				cfg.RateLimitPerMinute = parsed
			}
		}
		if cfg.RateLimitPerMinute == 0 {
			cfg.RateLimitPerMinute = 30
		}
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	if cfg.QueueSize == 0 {
		cfg.QueueSize = 1000
	}

	if cfg.MinSeverity == "" {
		cfg.MinSeverity = SeverityInfo
	}

	// Discord settings
	if cfg.Discord.WebhookURL == "" {
		cfg.Discord.WebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")
	}
	if cfg.Discord.WebhookURL != "" && !cfg.Discord.Enabled {
		cfg.Discord.Enabled = true
	}
	if cfg.Discord.Username == "" {
		cfg.Discord.Username = "tragge-notifications"
	}

	// Email (Resend) settings
	if cfg.Email.APIKey == "" {
		cfg.Email.APIKey = os.Getenv("RESEND_API_KEY")
	}
	if cfg.Email.FromEmail == "" {
		cfg.Email.FromEmail = os.Getenv("RESEND_FROM_EMAIL")
		if cfg.Email.FromEmail == "" {
			cfg.Email.FromEmail = "onboarding@resend.dev"
		}
	}
	if cfg.Email.APIKey != "" && !cfg.Email.Enabled {
		cfg.Email.Enabled = true
	}

	return cfg
}

// Validate checks if the configuration is valid.
func (c Config) Validate() error {
	if !c.MinSeverity.IsValid() {
		return ErrInvalidSeverity
	}
	if c.RateLimitPerMinute < 0 {
		return ErrInvalidRateLimit
	}
	if c.QueueSize < 0 {
		return ErrInvalidQueueSize
	}
	return nil
}

// HasChannels returns true if at least one notification channel is configured.
func (c Config) HasChannels() bool {
	return c.Discord.Enabled || c.Email.Enabled
}

// ConfigToServiceConfig converts a legacy Config to a ServiceConfig.
// Fields unique to ServiceConfig are set to sensible defaults:
// AsyncWorkers=5, ShutdownTimeout=30s, Environment="development".
func ConfigToServiceConfig(old Config) ServiceConfig {
	return ServiceConfig{
		Discord:         old.Discord,
		Email:           old.Email,
		Enabled:         old.Enabled,
		AsyncEnabled:    old.Async,
		AsyncWorkers:    5,
		AsyncQueueSize:  old.QueueSize,
		Environment:     "development",
		ServiceName:     old.Service,
		EmailRecipients: old.Email.Recipients,
		ShutdownTimeout: 30 * time.Second,
	}
}
