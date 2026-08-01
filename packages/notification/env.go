package notification

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variable names for notification configuration.
const (
	EnvDiscordWebhookURL        = "DISCORD_WEBHOOK_URL"
	EnvResendAPIKey             = "RESEND_API_KEY"
	EnvResendFromEmail          = "RESEND_FROM_EMAIL"
	EnvNotificationRecipients   = "NOTIFICATION_EMAIL_RECIPIENTS"
	EnvNotificationEnabled      = "NOTIFICATION_ENABLED"
	EnvNotificationAsync        = "NOTIFICATION_ASYNC"
	EnvNotificationAsyncWorkers = "NOTIFICATION_ASYNC_WORKERS"
	EnvNotificationQueueSize    = "NOTIFICATION_ASYNC_QUEUE_SIZE"
	EnvNotificationEnvironment  = "ENVIRONMENT"
	EnvNotificationServiceName  = "SERVICE_NAME"
)

// Default values for notification configuration.
const (
	DefaultFromEmail       = "onboarding@resend.dev"
	DefaultAsyncWorkers    = 5
	DefaultAsyncQueueSize  = 100
	DefaultEnvironment     = "development"
	DefaultShutdownTimeout = 30 * time.Second
)

// Environment configuration errors.
var (
	ErrMissingDiscordWebhook = errors.New("notification: DISCORD_WEBHOOK_URL is required when notifications are enabled but RESEND_API_KEY is not set")
	ErrMissingResendAPIKey   = errors.New("notification: RESEND_API_KEY is required when notifications are enabled but DISCORD_WEBHOOK_URL is not set")
	ErrMissingEmailRecipient = errors.New("notification: NOTIFICATION_EMAIL_RECIPIENTS is required when email notifications are enabled")
	ErrNoChannelsConfigured  = errors.New("notification: at least one notification channel (Discord or Email) must be configured when notifications are enabled")
)

// LoadFromEnv loads ServiceConfig from environment variables.
// It reads all notification-related environment variables and validates the configuration.
// Returns an error if NOTIFICATION_ENABLED=true but required variables are missing.
func LoadFromEnv() (*ServiceConfig, error) {
	cfg := &ServiceConfig{
		Enabled:         getEnvBool(EnvNotificationEnabled, true),
		AsyncEnabled:    getEnvBool(EnvNotificationAsync, true),
		AsyncWorkers:    getEnvInt(EnvNotificationAsyncWorkers, DefaultAsyncWorkers),
		AsyncQueueSize:  getEnvInt(EnvNotificationQueueSize, DefaultAsyncQueueSize),
		Environment:     getEnv(EnvNotificationEnvironment, DefaultEnvironment),
		ServiceName:     getEnv(EnvNotificationServiceName, ""),
		ShutdownTimeout: DefaultShutdownTimeout,
	}

	// Load Discord configuration
	cfg.Discord = DiscordConfig{
		WebhookURL: getEnv(EnvDiscordWebhookURL, ""),
		Username:   "tragge-notifications",
	}
	cfg.Discord.Enabled = cfg.Discord.WebhookURL != ""

	// Load Email configuration
	cfg.Email = EmailConfig{
		APIKey:    getEnv(EnvResendAPIKey, ""),
		FromEmail: getEnv(EnvResendFromEmail, DefaultFromEmail),
	}
	cfg.Email.Enabled = cfg.Email.APIKey != ""

	// Parse email recipients
	recipientsStr := getEnv(EnvNotificationRecipients, "")
	if recipientsStr != "" {
		recipients := strings.Split(recipientsStr, ",")
		for i := range recipients {
			recipients[i] = strings.TrimSpace(recipients[i])
		}
		cfg.Email.Recipients = recipients
		cfg.EmailRecipients = recipients
	}

	// Validate configuration if notifications are enabled
	if cfg.Enabled {
		if err := validateEnabledConfig(cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

// LoadFromEnvWithServiceName loads ServiceConfig from environment variables
// and sets the service name explicitly.
func LoadFromEnvWithServiceName(serviceName string) (*ServiceConfig, error) {
	cfg, err := LoadFromEnv()
	if err != nil {
		return nil, err
	}
	cfg.ServiceName = serviceName
	return cfg, nil
}

// validateEnabledConfig validates that required configuration is present
// when notifications are enabled.
func validateEnabledConfig(cfg *ServiceConfig) error {
	// At least one channel must be configured
	if !cfg.Discord.Enabled && !cfg.Email.Enabled {
		return ErrNoChannelsConfigured
	}

	// If email is enabled, recipients must be configured
	if cfg.Email.Enabled && len(cfg.Email.Recipients) == 0 {
		return ErrMissingEmailRecipient
	}

	return nil
}

// MustLoadFromEnv loads ServiceConfig from environment variables and panics on error.
// Use this for initialization where configuration errors should be fatal.
func MustLoadFromEnv() *ServiceConfig {
	cfg, err := LoadFromEnv()
	if err != nil {
		panic("notification: failed to load configuration: " + err.Error())
	}
	return cfg
}

// MustLoadFromEnvWithServiceName loads ServiceConfig from environment variables
// with a service name and panics on error.
func MustLoadFromEnvWithServiceName(serviceName string) *ServiceConfig {
	cfg, err := LoadFromEnvWithServiceName(serviceName)
	if err != nil {
		panic("notification: failed to load configuration: " + err.Error())
	}
	return cfg
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool returns the boolean value of an environment variable or a default value.
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	// Handle common boolean representations
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return defaultValue
	}
}

// getEnvInt returns the integer value of an environment variable or a default value.
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil && i > 0 {
			return i
		}
	}
	return defaultValue
}
