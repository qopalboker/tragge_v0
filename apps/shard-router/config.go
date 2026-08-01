package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/ratelimit"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
)

// Config holds all configuration for the shard-router service.
type Config struct {
	// Server configuration
	Port            string
	ShutdownTimeout time.Duration

	// Database configuration
	DB db.Config

	// Redis configuration
	Redis redis.Config

	// Observability configuration
	Observability observability.Config

	// Rate limiting configuration
	RateLimit ratelimit.Config

	// Shard router specific
	CacheTTL           time.Duration
	ShardRefreshPeriod time.Duration
	VirtualNodes       int

	// Notification configuration
	NotificationEnabled      bool
	NotificationAsync        bool
	NotificationAsyncWorkers int
	NotificationQueueSize    int
	DiscordWebhookURL        string
	ResendAPIKey             string
	ResendFromEmail          string
	NotificationRecipients   string
	Environment              string

	// Authentication
	AuthContext auth.ContextConfig

	// Health monitoring thresholds
	UnhealthyShardDuration  time.Duration // Duration before considering a shard unhealthy (default: 2m)
	CacheMissRateThreshold  float64       // Cache miss rate threshold for alerts (default: 0.5 = 50%)
	RoutingLatencyThreshold time.Duration // P99 routing latency threshold (default: 100ms)
	LoadImbalanceThreshold  float64       // Load imbalance threshold (default: 2.0 = 2x)
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() Config {
	environment := getEnv("ENVIRONMENT", "development")
	authIsolation := auth.LoadIsolationConfig(environment, os.Getenv, secrets.Load)
	if err := authIsolation.Validate(); err != nil {
		panic("shard-router: invalid authentication isolation configuration: " + err.Error())
	}

	return Config{
		Port:            getEnv("PORT", "8090"),
		ShutdownTimeout: getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second),

		DB: db.Config{
			PrimaryDSN:        secrets.BuildPostgresDSN(),
			ReplicaDSNs:       getReplicaDSNs(),
			MaxOpenConns:      getIntEnv("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:      getIntEnv("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime:   getDurationEnv("DB_CONN_MAX_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime:   getDurationEnv("DB_CONN_MAX_IDLE_TIME", 1*time.Minute),
			MaxReplicationLag: getDurationEnv("DB_MAX_REPLICATION_LAG", 10*time.Second),
			LagCheckInterval:  getDurationEnv("DB_LAG_CHECK_INTERVAL", 5*time.Second),
			RetryOnLag:        getBoolEnv("DB_RETRY_ON_LAG", true),
		},

		Redis: redis.ConfigFromEnv(os.Getenv),

		Observability: observability.Config{
			Service:              "shard-router",
			Env:                  getEnv("ENVIRONMENT", "development"),
			Version:              getEnv("VERSION", "0.1.0"),
			LogLevel:             getEnv("LOG_LEVEL", "info"),
			Development:          getBoolEnv("DEVELOPMENT", false),
			OTLPEndpoint:         getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			OTLPInsecure:         getBoolEnv("OTEL_EXPORTER_OTLP_INSECURE", true),
			SampleRatio:          getFloatEnv("OTEL_SAMPLE_RATIO", 0.1),
			EnableGoMetrics:      true,
			EnableProcessMetrics: true,
		},

		RateLimit: ratelimit.Config{
			Rate:            getIntEnv("RATE_LIMIT_RATE", 1000),
			Window:          getDurationEnv("RATE_LIMIT_WINDOW", time.Minute),
			BurstSize:       getIntEnv("RATE_LIMIT_BURST", 100),
			CleanupInterval: getDurationEnv("RATE_LIMIT_CLEANUP", 5*time.Minute),
			KeyPrefix:       "rl:shard:",
		},

		CacheTTL:           getDurationEnv("CACHE_TTL", 5*time.Minute),
		ShardRefreshPeriod: getDurationEnv("SHARD_REFRESH_PERIOD", 30*time.Second),
		VirtualNodes:       getIntEnv("VIRTUAL_NODES", 100),

		// Notification configuration
		NotificationEnabled:      getBoolEnv("NOTIFICATION_ENABLED", true),
		NotificationAsync:        getBoolEnv("NOTIFICATION_ASYNC", true),
		NotificationAsyncWorkers: getIntEnv("NOTIFICATION_ASYNC_WORKERS", 5),
		NotificationQueueSize:    getIntEnv("NOTIFICATION_ASYNC_QUEUE_SIZE", 100),
		DiscordWebhookURL:        getEnv("DISCORD_WEBHOOK_URL", ""),
		ResendAPIKey:             getEnv("RESEND_API_KEY", ""),
		ResendFromEmail:          getEnv("RESEND_FROM_EMAIL", "onboarding@resend.dev"),
		NotificationRecipients:   getEnv("NOTIFICATION_EMAIL_RECIPIENTS", ""),
		Environment:              environment,

		// Authentication
		AuthContext: authIsolation.Admin,

		// Health monitoring thresholds
		UnhealthyShardDuration:  getDurationEnv("UNHEALTHY_SHARD_DURATION", 2*time.Minute),
		CacheMissRateThreshold:  getFloatEnv("CACHE_MISS_RATE_THRESHOLD", 0.5),
		RoutingLatencyThreshold: getDurationEnv("ROUTING_LATENCY_THRESHOLD", 100*time.Millisecond),
		LoadImbalanceThreshold:  getFloatEnv("LOAD_IMBALANCE_THRESHOLD", 2.0),
	}
}

// getReplicaDSNs returns replica DSNs from environment.
func getReplicaDSNs() []string {
	replicaDSN := os.Getenv("POSTGRES_REPLICA_DSN")
	if replicaDSN == "" {
		return nil
	}
	// Support multiple replicas separated by comma
	return strings.Split(replicaDSN, ",")
}

// mustGetEnv returns the value of an environment variable or panics if not set.
func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic("required environment variable not set: " + key)
	}
	return val
}

// getEnv returns the value of an environment variable or a default value.
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// getIntEnv returns an integer environment variable or a default value.
func getIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// getFloatEnv returns a float64 environment variable or a default value.
func getFloatEnv(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

// getBoolEnv returns a boolean environment variable or a default value.
func getBoolEnv(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

// getDurationEnv returns a time.Duration environment variable or a default value.
func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
