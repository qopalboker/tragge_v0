package server

import (
	"os"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port         string
	PostgresDSN  string
	RedisAddr    string
	KafkaBrokers []string

	// Market hours configuration
	MarketHoursConfigPath string
	MarketHoursEnabled    bool

	// Consumer configuration
	ConsumerGroup string
	OrdersTopic   string
	TicksTopic    string

	// Producer topics
	FillsTopic          string
	PositionsTopic      string
	PnLDeltasTopic      string
	OrderAcksTopic      string
	PositionClosedTopic string
	OrderCancelledTopic string
	AlertsTopic         string

	// Consumer topics for close position, cancel order, and modify TP/SL
	ClosePositionsTopic string
	CancelOrdersTopic   string
	ModifyTPSLTopic     string

	// Shard configuration
	ShardID            int
	ShardCount         int
	ShardEnabled       bool
	ShardAssignmentTTL time.Duration

	// Partition-aware consumer configuration
	PartitionAwareEnabled bool          // Enable partition-aware consumption
	TotalPartitions       int           // Total partitions in orders topic (e.g., 16)
	LagMonitorInterval    time.Duration // Interval for lag metric updates
	PodName               string        // Kubernetes pod name (for shard ID detection)

	// Notification configuration
	NotificationEnabled      bool   // Enable notifications
	NotificationAsync        bool   // Enable async notification sending
	NotificationAsyncWorkers int    // Number of async workers
	NotificationQueueSize    int    // Async queue size
	DiscordWebhookURL        string // Discord webhook URL
	ResendAPIKey             string // Resend API key for email
	ResendFromEmail          string // From email address for Resend
	NotificationRecipients   string // Comma-separated email recipients
	Environment              string // Environment name (development/staging/production)

	// QTY scoring configuration
	QtyMinPerTrade   int64 // Minimum QTY per trade (e.g., 100)
	QtyMaxPctOfTotal int   // Maximum QTY as percentage of total (e.g., 50 for 50%)

	// Real-time score updates
	UnrealizedBroadcastInterval time.Duration // Interval for broadcasting unrealized score updates (default: 1s)

	// Stale price configuration
	MaxPriceAgeMarket  time.Duration // Maximum price age for market order execution (default: 30s)
	MaxPriceAgePending time.Duration // Maximum price age for pending order triggers (default: 60s)

	// Per-asset-class price freshness thresholds
	MaxPriceAgeOpenCrypto  time.Duration // Max price age for opening crypto positions (default: 30s)
	MaxPriceAgeOpenForex   time.Duration // Max price age for opening forex positions (default: 60s)
	MaxPriceAgeCloseCrypto time.Duration // Max price age for closing crypto positions (default: 60s)
	MaxPriceAgeCloseForex  time.Duration // Max price age for closing forex positions (default: 120s)

	// Price freshness monitor configuration
	PriceFreshnessMonitorEnabled   bool          // Enable price freshness monitoring (default: true)
	PriceFreshnessCheckInterval    time.Duration // Interval between freshness checks (default: 5s)
	PriceFreshnessWarningThreshold time.Duration // Threshold for warning (emit gauge) (default: 30s)
	PriceFreshnessAlertThreshold   time.Duration // Threshold for alert (publish to Kafka) (default: 60s)

	// WAL persistence configuration
	WALPersistPath string // Path to WAL file for crash recovery; empty = in-memory only

	// Cache configuration
	ContestCacheTTL      time.Duration // TTL for contest cache entries (default: 30s)
	ParticipantCacheTTL  time.Duration // TTL for participant cache entries (default: 60s)
	CacheCleanupInterval time.Duration // Interval for cache eviction sweeps (default: 60s)
	CacheEnabled         bool          // Kill switch to disable caching entirely (default: true)

	// Rate limiting configuration
	RateLimits RateLimitConfig
}

func loadConfig() *Config {
	port := os.Getenv("TRADING_ENGINE_PORT")
	if port == "" {
		port = config.GetEnv("PORT", "8085")
	}
	postgresDSN := secrets.BuildPostgresDSN()
	redisAddr := config.GetEnv("REDIS_ADDR", "localhost:6379")

	brokersStr := config.GetEnv("KAFKA_BROKERS", "localhost:9092")
	brokers := strings.Split(brokersStr, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	// Get pod name for shard ID detection
	podName := config.GetEnv("POD_NAME", "")

	// Determine shard ID - try POD_NAME first, then SHARD_ID env var
	shardID := config.GetEnvInt("SHARD_ID", 0)
	if podName != "" {
		if parsedShardID, err := GetShardIDFromPodName(podName); err == nil {
			shardID = parsedShardID
		}
	}

	return &Config{
		Port:                  port,
		PostgresDSN:           postgresDSN,
		RedisAddr:             redisAddr,
		KafkaBrokers:          brokers,
		MarketHoursConfigPath: config.GetEnv("MARKET_HOURS_CONFIG_PATH", "market_hours.json"),
		MarketHoursEnabled:    config.GetEnvBool("MARKET_HOURS_ENABLED", true),
		ConsumerGroup:         config.GetEnv("CONSUMER_GROUP", "trading-engine"),
		OrdersTopic:         config.GetEnv("ORDERS_TOPIC", "orders.v1"),
		TicksTopic:          config.GetEnv("TICKS_TOPIC", "ticks.v1"),
		FillsTopic:          config.GetEnv("FILLS_TOPIC", "fills.v1"),
		PositionsTopic:      config.GetEnv("POSITIONS_TOPIC", "positions.v1"),
		PnLDeltasTopic:      config.GetEnv("PNL_DELTAS_TOPIC", "pnl_deltas.v1"),
		OrderAcksTopic:      config.GetEnv("ORDER_ACKS_TOPIC", "order_acks.v1"),
		PositionClosedTopic: config.GetEnv("POSITION_CLOSED_TOPIC", "position_closed.v1"),
		OrderCancelledTopic: config.GetEnv("ORDER_CANCELLED_TOPIC", "order_cancelled.v1"),
		AlertsTopic:         config.GetEnv("ALERTS_TOPIC", "alerts.v1"),
		ClosePositionsTopic: config.GetEnv("CLOSE_POSITIONS_TOPIC", "close_positions.v1"),
		CancelOrdersTopic:   config.GetEnv("CANCEL_ORDERS_TOPIC", "cancel_orders.v1"),
		ModifyTPSLTopic:     config.GetEnv("MODIFY_TPSL_TOPIC", "modify_tpsl.v1"),
		ShardID:             shardID,
		ShardCount:          config.GetEnvInt("SHARD_COUNT", 1),
		ShardEnabled:        config.GetEnvBool("SHARD_ENABLED", false),
		ShardAssignmentTTL:  getEnvDuration("SHARD_ASSIGNMENT_TTL", 1*time.Hour),

		// Partition-aware consumer configuration
		PartitionAwareEnabled: config.GetEnvBool("PARTITION_AWARE_ENABLED", false),
		TotalPartitions:       config.GetEnvInt("TOTAL_PARTITIONS", 16),
		LagMonitorInterval:    getEnvDuration("LAG_MONITOR_INTERVAL", 30*time.Second),
		PodName:               podName,

		// Notification configuration
		NotificationEnabled:      config.GetEnvBool("NOTIFICATION_ENABLED", true),
		NotificationAsync:        config.GetEnvBool("NOTIFICATION_ASYNC", true),
		NotificationAsyncWorkers: config.GetEnvInt("NOTIFICATION_ASYNC_WORKERS", 5),
		NotificationQueueSize:    config.GetEnvInt("NOTIFICATION_ASYNC_QUEUE_SIZE", 100),
		DiscordWebhookURL:        config.GetEnv("DISCORD_WEBHOOK_URL", ""),
		ResendAPIKey:             config.GetEnv("RESEND_API_KEY", ""),
		ResendFromEmail:          config.GetEnv("RESEND_FROM_EMAIL", "onboarding@resend.dev"),
		NotificationRecipients:   config.GetEnv("NOTIFICATION_EMAIL_RECIPIENTS", ""),
		Environment:              config.GetEnv("ENVIRONMENT", "development"),

		// QTY scoring configuration
		QtyMinPerTrade:   int64(config.GetEnvInt("QTY_MIN_PER_TRADE", 100)),
		QtyMaxPctOfTotal: config.GetEnvInt("QTY_MAX_PCT_OF_TOTAL", 50),

		// Real-time score updates (broadcast unrealized scores every second)
		UnrealizedBroadcastInterval: getEnvDuration("UNREALIZED_BROADCAST_INTERVAL", 1*time.Second),

		// Stale price configuration
		MaxPriceAgeMarket:  getEnvDuration("MAX_PRICE_AGE_MARKET", 30*time.Second),
		MaxPriceAgePending: getEnvDuration("MAX_PRICE_AGE_PENDING", 60*time.Second),

		// Per-asset-class price freshness thresholds
		MaxPriceAgeOpenCrypto:  getEnvDuration("MAX_PRICE_AGE_OPEN_CRYPTO", 30*time.Second),
		MaxPriceAgeOpenForex:   getEnvDuration("MAX_PRICE_AGE_OPEN_FOREX", 60*time.Second),
		MaxPriceAgeCloseCrypto: getEnvDuration("MAX_PRICE_AGE_CLOSE_CRYPTO", 60*time.Second),
		MaxPriceAgeCloseForex:  getEnvDuration("MAX_PRICE_AGE_CLOSE_FOREX", 120*time.Second),

		// Price freshness monitor configuration
		PriceFreshnessMonitorEnabled:   config.GetEnvBool("PRICE_FRESHNESS_MONITOR_ENABLED", true),
		PriceFreshnessCheckInterval:    getEnvDuration("PRICE_FRESHNESS_CHECK_INTERVAL", 5*time.Second),
		PriceFreshnessWarningThreshold: getEnvDuration("PRICE_FRESHNESS_WARNING_THRESHOLD", 30*time.Second),
		PriceFreshnessAlertThreshold:   getEnvDuration("PRICE_FRESHNESS_ALERT_THRESHOLD", 60*time.Second),

		// WAL persistence configuration
		WALPersistPath: config.GetEnv("WAL_PERSIST_PATH", ""),

		// Cache configuration
		ContestCacheTTL:      getEnvDuration("CONTEST_CACHE_TTL", 30*time.Second),
		ParticipantCacheTTL:  getEnvDuration("PARTICIPANT_CACHE_TTL", 60*time.Second),
		CacheCleanupInterval: getEnvDuration("CACHE_CLEANUP_INTERVAL", 60*time.Second),
		CacheEnabled:         config.GetEnvBool("CACHE_ENABLED", true),

		// Rate limiting configuration
		RateLimits: RateLimitConfig{
			UserPerSecond:           config.GetEnvInt("RATE_LIMIT_USER_PER_SECOND", 10),
			UserPerMinute:           config.GetEnvInt("RATE_LIMIT_USER_PER_MINUTE", 100),
			ContestPerSecond:        config.GetEnvInt("RATE_LIMIT_CONTEST_PER_SECOND", 500),
			GlobalPerSecond:         config.GetEnvInt("RATE_LIMIT_GLOBAL_PER_SECOND", 5000),
			DynamicContestLimits:    config.GetEnvBool("RATE_LIMIT_DYNAMIC_CONTEST_LIMITS", true),
			ContestLimitBaseRate:    config.GetEnvInt("RATE_LIMIT_CONTEST_BASE_RATE", 100),
			ContestLimitMultiplier:  config.GetEnvInt("RATE_LIMIT_CONTEST_MULTIPLIER", 2),
			ContestLimitRefreshSecs: config.GetEnvInt("RATE_LIMIT_CONTEST_REFRESH_SECS", 300),
		},
	}
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		panic("required environment variable not set: " + key)
	}
	return val
}

// Duration helpers
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

