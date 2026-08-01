package server

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
)

// Config holds the contest scheduler configuration.
type Config struct {
	// HTTP server port
	Port string

	// Database configuration
	PostgresDSN     string
	PostgresReplica string

	// Redis configuration
	RedisAddr           string
	RedisPassword       string
	RedisMode           string   // "standalone", "sentinel", or "cluster"
	RedisClusterAddrs   []string // Cluster node addresses
	RedisSentinelAddrs  []string // Sentinel addresses
	RedisSentinelMaster string   // Sentinel master name

	// Kafka configuration
	KafkaBrokers      []string
	ContestStateTopic string

	// Scheduler configuration
	CheckInterval     time.Duration // Max check interval for adaptive scheduling (default: 30s, max: 60s)
	MinCheckInterval  time.Duration // Min check interval for adaptive scheduling (default: 2s)
	StartBuffer       time.Duration // Start contest X seconds before scheduled time (default: 0)
	SettlementDelay   time.Duration // Wait X seconds after end before settling (default: 0)
	MaxConcurrent     int           // Max contests to process simultaneously (default: 10)
	MaxRetries        int           // Max retries for failed transitions (default: 3)
	RetryBaseDelay    time.Duration // Base delay between retries (default: 1s)
	RetryMaxDelay     time.Duration // Maximum delay between retries (default: 30s)
	LockTTL           time.Duration // Redis lock TTL (default: 60s)
	LockRetryInterval time.Duration // Interval to retry acquiring lock (default: 100ms)

	// Reminder configuration
	ReminderEnabled      bool            // Enable contest starting reminders (default: true)
	ReminderIntervals    []time.Duration // Intervals before start to send reminders (default: 24h, 1h, 15m)
	EndReminderIntervals []time.Duration // Intervals before end to send reminders (default: 15m)
	ReminderWindow       time.Duration   // Deprecated: use ReminderIntervals. Kept for backward compat.
	ReminderInterval     time.Duration   // How often to check for reminders (default: 1m)
	ReminderBatchSize    int             // Number of emails per batch (default: 50)
	TradingBaseURL       string          // Base URL for trading platform
	ResendAPIKey         string          // Resend API key for sending emails
	ResendFromEmail      string          // From email address for reminders

	// Calendar processor configuration
	CalendarEnabled       bool          // Enable calendar-based contest creation (default: true)
	CalendarCheckInterval time.Duration // How often to check for due calendar entries (default: 60s)
	CalendarLockTTL       time.Duration // TTL for calendar processor distributed lock (default: 60s)

	// Cleanup/archival configuration
	CleanupEnabled      bool          // Enable daily tournament cleanup and archival (default: true)
	CleanupArchiveDays  int           // Days after completion before archiving (default: 30)
	CleanupTimezone     string        // IANA timezone for cleanup scheduling (default: "Asia/Tehran")
	CleanupRunHour      int           // Hour (0-23) in timezone to run cleanup (default: 3)
	CleanupRunMinute    int           // Minute (0-59) to run cleanup (default: 0)
	CleanupLockTTL      time.Duration // Redis lock TTL for cleanup job (default: 5m)

	// Instance identification for distributed locking
	InstanceID string

	// Environment
	Environment string // "development", "staging", "production"
}

// loadConfig loads configuration from environment variables.
func loadConfig() *Config {
	port := os.Getenv("CONTEST_SCHEDULER_PORT")
	if port == "" {
		port = config.GetEnv("PORT", "8088")
	}
	postgresDSN := secrets.BuildPostgresDSN()

	redisAddr := config.GetEnv("REDIS_ADDR", "localhost:6379")
	brokersStr := config.GetEnv("KAFKA_BROKERS", "localhost:9092")
	brokers := splitAndTrim(brokersStr, ",")

	// Redis cluster/sentinel addresses
	redisClusterAddrs := splitAndTrim(config.GetEnv("REDIS_CLUSTER_ADDRS", ""), ",")
	redisSentinelAddrs := splitAndTrim(config.GetEnv("REDIS_SENTINEL_ADDRS", ""), ",")

	// Generate instance ID if not provided
	instanceID := config.GetEnv("INSTANCE_ID", "")
	if instanceID == "" {
		hostname, _ := os.Hostname()
		if hostname != "" {
			instanceID = hostname
		} else {
			instanceID = "contest-scheduler-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		}
	}

	return &Config{
		Port:                port,
		PostgresDSN:         postgresDSN,
		PostgresReplica:     config.GetEnv("POSTGRES_REPLICA_DSN", ""),
		RedisAddr:           redisAddr,
		RedisPassword:       secrets.Load("REDIS_PASSWORD"),
		RedisMode:           config.GetEnv("REDIS_MODE", "standalone"),
		RedisClusterAddrs:   redisClusterAddrs,
		RedisSentinelAddrs:  redisSentinelAddrs,
		RedisSentinelMaster: config.GetEnv("REDIS_SENTINEL_MASTER", "mymaster"),
		KafkaBrokers:        brokers,
		ContestStateTopic:   config.GetEnv("CONTEST_STATE_TOPIC", "contests.v1"),

		// Scheduler configuration with sensible defaults (adaptive interval scheduling)
		CheckInterval:     getEnvDuration("CHECK_INTERVAL", 30*time.Second),      // Max interval
		MinCheckInterval:  getEnvDuration("MIN_CHECK_INTERVAL", 2*time.Second),   // Min interval
		StartBuffer:       getEnvDuration("START_BUFFER", 0),
		SettlementDelay:   getEnvDuration("SETTLEMENT_DELAY", 0),
		MaxConcurrent:     config.GetEnvInt("MAX_CONCURRENT", 10),
		MaxRetries:        config.GetEnvInt("MAX_RETRIES", 3),
		RetryBaseDelay:    getEnvDuration("RETRY_BASE_DELAY", 1*time.Second),
		RetryMaxDelay:     getEnvDuration("RETRY_MAX_DELAY", 30*time.Second),
		LockTTL:           getEnvDuration("LOCK_TTL", 120*time.Second),
		LockRetryInterval: getEnvDuration("LOCK_RETRY_INTERVAL", 100*time.Millisecond),

		// Reminder configuration
		ReminderEnabled:      config.GetEnvBool("REMINDER_ENABLED", true),
		ReminderIntervals:    getEnvDurations("REMINDER_INTERVALS", []time.Duration{24 * time.Hour, 1 * time.Hour, 15 * time.Minute}),
		EndReminderIntervals: getEnvDurations("END_REMINDER_INTERVALS", []time.Duration{15 * time.Minute}),
		ReminderWindow:       getEnvDuration("REMINDER_WINDOW", 15*time.Minute), // backward compat
		ReminderInterval:     getEnvDuration("REMINDER_INTERVAL", 1*time.Minute),
		ReminderBatchSize:    config.GetEnvInt("REMINDER_BATCH_SIZE", 50),
		TradingBaseURL:       config.GetEnv("TRADING_BASE_URL", "https://trade.tragge.com"),
		ResendAPIKey:         config.GetEnv("RESEND_API_KEY", ""),
		ResendFromEmail:      config.GetEnv("RESEND_FROM_EMAIL", "noreply@tragge.com"),

		// Calendar processor configuration
		CalendarEnabled:       config.GetEnvBool("CALENDAR_ENABLED", true),
		CalendarCheckInterval: getEnvDuration("CALENDAR_CHECK_INTERVAL", 60*time.Second),
		CalendarLockTTL:       getEnvDuration("CALENDAR_LOCK_TTL", 60*time.Second),

		// Cleanup/archival configuration
		CleanupEnabled:     config.GetEnvBool("CLEANUP_ENABLED", true),
		CleanupArchiveDays: config.GetEnvInt("CLEANUP_ARCHIVE_DAYS", 30),
		CleanupTimezone:    config.GetEnv("CLEANUP_TIMEZONE", "Asia/Tehran"),
		CleanupRunHour:     config.GetEnvInt("CLEANUP_RUN_HOUR", 3),
		CleanupRunMinute:   config.GetEnvInt("CLEANUP_RUN_MINUTE", 0),
		CleanupLockTTL:     getEnvDuration("CLEANUP_LOCK_TTL", 5*time.Minute),

		InstanceID:  instanceID,
		Environment: config.GetEnv("ENVIRONMENT", "development"),
	}
}

func splitAndTrim(s string, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic("required environment variable not set: " + key)
	}
	return val
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

// getEnvDurations parses a comma-separated list of durations from an env var.
// Example: REMINDER_INTERVALS=24h,1h,15m
func getEnvDurations(key string, defaultValue []time.Duration) []time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	parts := strings.Split(value, ",")
	var durations []time.Duration
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		d, err := time.ParseDuration(p)
		if err != nil {
			continue
		}
		durations = append(durations, d)
	}

	if len(durations) == 0 {
		return defaultValue
	}

	// Sort descending (largest interval first)
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] > durations[j]
	})

	return durations
}
