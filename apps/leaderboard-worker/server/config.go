package server

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port        string
	PostgresDSN string
	RedisAddr   string

	// Redis Cluster configuration
	RedisMode           string   // "standalone", "sentinel", or "cluster"
	RedisClusterAddrs   []string // Cluster node addresses
	RedisSentinelAddrs  []string // Sentinel addresses
	RedisSentinelMaster string   // Sentinel master name
	RedisPassword       string

	// Kafka configuration
	KafkaBrokers       []string
	ConsumerGroup      string
	PnLDeltasTopic     string
	ContestStateTopic  string
	NotificationsTopic string

	// Snapshot configuration
	SnapshotInterval     time.Duration
	SnapshotTopN         int
	FullSnapshotInterval time.Duration // Safety-net interval to write snapshots for ALL contests regardless of dirty flags

	// Sharding configuration
	ShardCount             int
	ShardID                int           // This instance's shard ID (for StatefulSet)
	CacheTTL               time.Duration // Cache TTL for aggregated leaderboards
	SignificantScoreChange float64       // Score delta that triggers cache invalidation

	// PnL delta batching configuration
	PnLBatchSize         int           // Max records to batch before flushing (default: 50)
	PnLBatchTimeout      time.Duration // Max time to wait for a full batch (default: 100ms)
	DisableStandaloneRedis bool        // Skip standalone lb:contestID writes when sharded worker is active (default: false)

	// Notification configuration
	NotificationEnabled      bool
	NotificationAsync        bool
	NotificationAsyncWorkers int
	NotificationQueueSize    int
	DiscordWebhookURL        string
	ResendAPIKey             string
	ResendFromEmail          string
	NotificationRecipients   string // Comma-separated list of email recipients
	Environment              string // "development", "staging", "production"

	// Trade frontend URL for email links
	TradeFrontendURL string // Base URL of the trade frontend (e.g., "https://tragge.com/trade")

	// Contest webhook configuration (for external integrations)
	ContestWebhookURL     string        // URL to POST contest events to
	ContestWebhookTimeout time.Duration // Timeout for webhook requests
	ContestWebhookSecret  string        // Secret for signing webhook payloads
}

func loadConfig() *Config {
	port := os.Getenv("LEADERBOARD_WORKER_PORT")
	if port == "" {
		port = config.GetEnv("PORT", "8086")
	}
	postgresDSN := secrets.BuildPostgresDSN()
	redisAddr := config.GetEnv("REDIS_ADDR", "localhost:6379")

	// Parse Kafka brokers
	brokersStr := config.GetEnv("KAFKA_BROKERS", "localhost:9092")
	brokers := splitAndTrim(brokersStr, ",")

	// Parse Redis cluster addresses
	redisClusterAddrs := splitAndTrim(config.GetEnv("REDIS_CLUSTER_ADDRS", ""), ",")
	redisSentinelAddrs := splitAndTrim(config.GetEnv("REDIS_SENTINEL_ADDRS", ""), ",")

	snapshotInterval := getEnvDuration("SNAPSHOT_INTERVAL", 30*time.Second)
	snapshotTopN := config.GetEnvInt("SNAPSHOT_TOP_N", 100)
	fullSnapshotInterval := getEnvDuration("FULL_SNAPSHOT_INTERVAL", 5*time.Minute)

	// Sharding configuration
	shardCount := config.GetEnvInt("SHARD_COUNT", 6)
	shardID := config.GetEnvInt("SHARD_ID", 0)
	cacheTTL := getEnvDuration("CACHE_TTL", 30*time.Second)
	significantScoreChange := getEnvFloat("SIGNIFICANT_SCORE_CHANGE", 100.0)

	return &Config{
		Port:                   port,
		PostgresDSN:            postgresDSN,
		RedisAddr:              redisAddr,
		RedisMode:              config.GetEnv("REDIS_MODE", "standalone"),
		RedisClusterAddrs:      redisClusterAddrs,
		RedisSentinelAddrs:     redisSentinelAddrs,
		RedisSentinelMaster:    config.GetEnv("REDIS_SENTINEL_MASTER", "mymaster"),
		RedisPassword:          secrets.LoadWithDefault("REDIS_PASSWORD", ""),
		KafkaBrokers:           brokers,
		ConsumerGroup:          config.GetEnv("CONSUMER_GROUP", "leaderboard-worker"),
		PnLDeltasTopic:         config.GetEnv("PNL_DELTAS_TOPIC", "pnl_deltas.v1"),
		ContestStateTopic:      config.GetEnv("CONTEST_STATE_TOPIC", "contests.v1"),
		NotificationsTopic:    config.GetEnv("NOTIFICATIONS_TOPIC", "notifications.v1"),
		SnapshotInterval:       snapshotInterval,
		SnapshotTopN:           snapshotTopN,
		FullSnapshotInterval:   fullSnapshotInterval,
		ShardCount:             shardCount,
		ShardID:                shardID,
		CacheTTL:               cacheTTL,
		SignificantScoreChange: significantScoreChange,
		PnLBatchSize:           config.GetEnvInt("PNL_BATCH_SIZE", 50),
		PnLBatchTimeout:        getEnvDuration("PNL_BATCH_TIMEOUT", 100*time.Millisecond),
		DisableStandaloneRedis: config.GetEnvBool("DISABLE_STANDALONE_REDIS", true),

		// Notification configuration
		NotificationEnabled:      config.GetEnvBool("NOTIFICATION_ENABLED", true),
		NotificationAsync:        config.GetEnvBool("NOTIFICATION_ASYNC", true),
		NotificationAsyncWorkers: config.GetEnvInt("NOTIFICATION_ASYNC_WORKERS", 5),
		NotificationQueueSize:    config.GetEnvInt("NOTIFICATION_QUEUE_SIZE", 100),
		DiscordWebhookURL:        config.GetEnv("DISCORD_WEBHOOK_URL", ""),
		ResendAPIKey:             config.GetEnv("RESEND_API_KEY", ""),
		ResendFromEmail:          config.GetEnv("RESEND_FROM_EMAIL", "alerts@tragge.com"),
		NotificationRecipients:   config.GetEnv("NOTIFICATION_EMAIL_RECIPIENTS", ""),
		Environment:              config.GetEnv("ENVIRONMENT", "development"),

		// Trade frontend URL
		TradeFrontendURL: config.GetEnv("TRADE_FRONTEND_URL", "https://tragge.com/trade"),

		// Contest webhook configuration
		ContestWebhookURL:     config.GetEnv("CONTEST_WEBHOOK_URL", ""),
		ContestWebhookTimeout: getEnvDuration("CONTEST_WEBHOOK_TIMEOUT", 10*time.Second),
		ContestWebhookSecret:  config.GetEnv("CONTEST_WEBHOOK_SECRET", ""),
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

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}
