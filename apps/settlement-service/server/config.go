package server

import (
	"os"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
)

// Config holds the configuration for the settlement service.
type Config struct {
	// Server
	Port string

	// Database
	PostgresDSN string

	// Redis
	RedisAddr string

	// Kafka
	KafkaBrokers  []string
	ConsumerGroup string

	// Topics - Consume
	ContestStateTopic   string
	SettlementReqTopic  string
	PositionClosedTopic string

	// Topics - Produce
	SettlementEventsTopic string
	ClosePositionsTopic   string
	CancelOrdersTopic     string
	NotificationsTopic    string

	// Settlement configuration
	MaxRetries           int
	RetryBaseDelay       time.Duration
	SettlementTimeout    time.Duration
	PositionCloseTimeout time.Duration // Max wait time per attempt for positions to close
	WinnersPercentage    int           // Percentage of participants who win (default 30%)
	PlatformFeeBps       int           // Platform fee in basis points (default 1700 = 17%)
	PrizeEmailBatchSize  int           // Batch size for sending prize winner emails (default 50)

	// Stuck settlement detection
	StuckCheckInterval        time.Duration // How often to check for stuck settlements (default 5m)
	StuckThreshold            time.Duration // How long before a settlement is considered stuck (default 10m)
	OrphanedSettlingThreshold time.Duration // How long before a settling contest without settlement record is considered orphaned (default 5m)

	// Notification
	NotificationEnabled bool
	DiscordWebhookURL   string
	ResendAPIKey        string

	// Environment and Admin authentication trust context
	Environment string
	AuthContext auth.ContextConfig
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() *Config {
	environment := config.GetEnv("ENVIRONMENT", "development")
	authIsolation := auth.LoadIsolationConfig(environment, os.Getenv, secrets.Load)
	if err := authIsolation.Validate(); err != nil {
		panic("settlement-service: invalid authentication isolation configuration: " + err.Error())
	}

	return &Config{
		// Server
		Port: settlementPort(),

		// Database
		PostgresDSN: secrets.BuildPostgresDSN(),

		// Redis
		RedisAddr: config.GetEnv("REDIS_ADDR", "localhost:6379"),

		// Kafka
		KafkaBrokers:  strings.Split(config.GetEnv("KAFKA_BROKERS", "localhost:9092"), ","),
		ConsumerGroup: config.GetEnv("CONSUMER_GROUP", "settlement-service"),

		// Topics - Consume
		ContestStateTopic:   config.GetEnv("CONTEST_STATE_TOPIC", "contests.v1"),
		SettlementReqTopic:  config.GetEnv("SETTLEMENT_REQ_TOPIC", "settlement_requests.v1"),
		PositionClosedTopic: config.GetEnv("POSITION_CLOSED_TOPIC", "position_closed.v1"),

		// Topics - Produce
		SettlementEventsTopic: config.GetEnv("SETTLEMENT_EVENTS_TOPIC", "settlement_events.v1"),
		ClosePositionsTopic:   config.GetEnv("CLOSE_POSITIONS_TOPIC", "contest_close_positions.v1"),
		CancelOrdersTopic:     config.GetEnv("CANCEL_ORDERS_TOPIC", "contest_cancel_orders.v1"),
		NotificationsTopic:    config.GetEnv("NOTIFICATIONS_TOPIC", "notifications.v1"),

		// Settlement configuration
		MaxRetries:           config.GetEnvInt("SETTLEMENT_MAX_RETRIES", 3),
		RetryBaseDelay:       time.Duration(config.GetEnvInt("SETTLEMENT_RETRY_DELAY_SECONDS", 10)) * time.Second,
		SettlementTimeout:    time.Duration(config.GetEnvInt("SETTLEMENT_TIMEOUT_MINUTES", 10)) * time.Minute,
		PositionCloseTimeout: time.Duration(config.GetEnvInt("POSITION_CLOSE_TIMEOUT_SECONDS", 120)) * time.Second,
		WinnersPercentage:    config.GetEnvInt("WINNERS_PERCENTAGE", 30),
		PlatformFeeBps:       config.GetEnvInt("PLATFORM_FEE_BPS", 1700),
		PrizeEmailBatchSize:  config.GetEnvInt("PRIZE_EMAIL_BATCH_SIZE", 50),

		// Stuck settlement detection
		StuckCheckInterval:        time.Duration(config.GetEnvInt("STUCK_CHECK_INTERVAL_MINUTES", 5)) * time.Minute,
		StuckThreshold:            time.Duration(config.GetEnvInt("STUCK_THRESHOLD_MINUTES", 10)) * time.Minute,
		OrphanedSettlingThreshold: time.Duration(config.GetEnvInt("ORPHANED_SETTLING_THRESHOLD_MINUTES", 5)) * time.Minute,

		// Notification
		NotificationEnabled: config.GetEnv("NOTIFICATION_ENABLED", "true") == "true",
		DiscordWebhookURL:   config.GetEnv("DISCORD_WEBHOOK_URL", ""),
		ResendAPIKey:        config.GetEnv("RESEND_API_KEY", ""),

		// Environment and Admin authentication trust context
		Environment: environment,
		AuthContext: authIsolation.Admin,
	}
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic("required environment variable not set: " + key)
	}
	return val
}

func settlementPort() string {
	if p := os.Getenv("SETTLEMENT_SERVICE_PORT"); p != "" {
		return p
	}
	return config.GetEnv("PORT", "8087")
}
