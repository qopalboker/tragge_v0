package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/config"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// App holds application state and dependencies.
type App struct {
	config              *Config
	db                  *sql.DB
	dbPool              *db.Pool // Used when sharding is enabled
	redis               *pkgredis.Client
	kafka               *kgo.Client
	ticksKafka          *kgo.Client // Separate client for ticks consumer
	closePositionsKafka *kgo.Client // Separate client for close positions consumer
	cancelOrdersKafka   *kgo.Client // Separate client for cancel orders consumer
	modifyTPSLKafka     *kgo.Client // Separate client for modify TP/SL consumer
	engine              *Engine
	shardedState        *ShardedStateManager // Used when sharding is enabled
	obs                 *observability.Observability
	notifications       *notification.Service
	circuits            *CircuitBreakers

	// Partition-aware consumers
	shardedOrdersConsumer         *ShardedConsumer   // Used when partition-aware mode is enabled
	shardedClosePositionsConsumer *ShardedConsumer   // Used when partition-aware mode is enabled
	shardedCancelOrdersConsumer   *ShardedConsumer   // Used when partition-aware mode is enabled
	shardedModifyTPSLConsumer     *ShardedConsumer   // Used when partition-aware mode is enabled
	broadcastTicksConsumer        *BroadcastConsumer // Used when partition-aware mode is enabled
	consumerMetrics               *ShardedConsumerMetrics

	// Price freshness monitor
	priceFreshnessMonitor *PriceFreshnessMonitor

	// State consistency checker
	stateConsistencyChecker *StateConsistencyChecker

	// Contest trading state (per-instance, not package-level)
	contestTrading   map[string]bool
	contestTradingMu sync.RWMutex

	// State
	ready  atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Shared resource flags - when true, these resources are shared and should not be closed by this service
	sharedDB    bool
	sharedRedis bool
}

// log returns the logger from observability
func (a *App) log() *zap.Logger {
	return a.obs.Logger.Logger
}

// Run starts the trading-engine service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil)
}

// RunWithSharedDeps starts the trading-engine service, optionally using shared resources.
// When parentCtx is non-nil, the service shuts down when the context is cancelled
// instead of registering its own signal handler. When sharedPool is non-nil, the service
// uses pool.Primary() for its *sql.DB instead of creating its own connection.
func RunWithSharedDeps(parentCtx context.Context, sharedPool *db.Pool, sharedRedis *pkgredis.Client) {
	// Validate critical environment variables in production/staging
	if sharedPool == nil {
		config.MustBeSetAny("database connection", "POSTGRES_DSN", "POSTGRES_HOST")
	}
	if sharedRedis == nil {
		config.MustBeSet("REDIS_ADDR")
	}
	config.MustBeSet("KAFKA_BROKERS")

	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize observability first
	obs, err := observability.New(ctx, observability.Config{
		Service:              "trading-engine",
		Env:                  os.Getenv("ENVIRONMENT"),
		Version:              os.Getenv("VERSION"),
		OTLPEndpoint:         os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		EnableGoMetrics:      true,
		EnableProcessMetrics: true,
	})
	if err != nil {
		panic("failed to initialize observability: " + err.Error())
	}
	defer obs.Shutdown(context.Background())

	log := obs.Logger.Logger

	// Initialize circuit breakers
	circuits := NewCircuitBreakers(log)

	// Create a new cancellable context for the application
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	app := &App{
		config:         cfg,
		ctx:            ctx,
		cancel:         cancel,
		obs:            obs,
		circuits:       circuits,
		contestTrading: make(map[string]bool),
	}

	// Initialize PostgreSQL
	if sharedPool != nil {
		app.db = sharedPool.Primary()
		app.sharedDB = true
		log.Info("Using shared database pool")
	} else {
		var dbErr error
		app.db, dbErr = sql.Open("pgx", cfg.PostgresDSN)
		if dbErr != nil {
			log.Fatal("Failed to open database", zap.Error(dbErr))
		}
		app.db.SetMaxOpenConns(config.GetEnvInt("DB_MAX_OPEN_CONNS", 25))
		app.db.SetMaxIdleConns(config.GetEnvInt("DB_MAX_IDLE_CONNS", 10))
		app.db.SetConnMaxLifetime(5 * time.Minute)
		defer app.db.Close()

		// Test database connection
		dbCtx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := app.db.PingContext(dbCtx); err != nil {
			log.Fatal("Failed to connect to database", zap.Error(err))
		}
		dbCancel()
		log.Info("PostgreSQL connected successfully",
			zap.Int("max_open_conns", config.GetEnvInt("DB_MAX_OPEN_CONNS", 25)),
			zap.Int("max_idle_conns", config.GetEnvInt("DB_MAX_IDLE_CONNS", 10)))
	}

	// Register DB connection pool metrics (collected every 10s)
	dbPoolMetrics := observability.NewDBPoolMetrics(obs.Metrics.Registry(), "trading_engine")
	dbPoolMetrics.AddDB(app.db)
	dbPoolMetrics.Start(10 * time.Second)
	defer dbPoolMetrics.Stop()

	// Initialize Redis with HA support
	if sharedRedis != nil {
		app.redis = sharedRedis
		app.sharedRedis = true
		log.Info("Using shared Redis client")
	} else {
		redisCfg := pkgredis.ConfigFromEnv(os.Getenv)
		if redisCfg.Addr == "" && redisCfg.Mode == pkgredis.ModeStandalone {
			redisCfg.Addr = cfg.RedisAddr
		}
		redisClient, redisErr := pkgredis.NewClient(redisCfg)
		if redisErr != nil {
			log.Warn("Failed to create Redis client", zap.Error(redisErr))
		} else {
			app.redis = redisClient
			// Test Redis connection
			redisCtx, redisCancel := context.WithTimeout(ctx, 5*time.Second)
			if err := app.redis.Ping(redisCtx).Err(); err != nil {
				log.Warn("Redis not available (market prices may not be available)", zap.Error(err))
			} else {
				log.Info("Redis connected successfully",
					zap.String("mode", string(app.redis.Mode())))
			}
			redisCancel()
		}
	}

	// Initialize notification service
	app.initNotifications(ctx, log)

	// Initialize Kafka consumers based on partition-aware mode
	var kafkaErr error
	if cfg.PartitionAwareEnabled {
		// Partition-aware mode: use ShardedConsumer for orders
		log.Info("Initializing partition-aware Kafka consumers",
			zap.Int("shard_id", cfg.ShardID),
			zap.Int("shard_count", cfg.ShardCount),
			zap.Int("total_partitions", cfg.TotalPartitions))

		// Register consumer metrics
		app.consumerMetrics = NewShardedConsumerMetrics(obs.Metrics.Registry(), "trading_engine")

		// Create sharded consumer for orders (partition-aware)
		app.shardedOrdersConsumer, kafkaErr = NewShardedConsumer(ShardedConsumerConfig{
			ShardID:           cfg.ShardID,
			ShardCount:        cfg.ShardCount,
			TotalPartitions:   cfg.TotalPartitions,
			Brokers:           cfg.KafkaBrokers,
			Topic:             cfg.OrdersTopic,
			ConsumerGroup:     cfg.ConsumerGroup,
			Logger:            log,
			DisableAutoCommit: true,
		})
		if kafkaErr != nil {
			log.Fatal("Failed to create sharded orders consumer", zap.Error(kafkaErr))
		}
		app.shardedOrdersConsumer.SetMetrics(app.consumerMetrics)

		// Start lag monitoring
		app.shardedOrdersConsumer.StartLagMonitor(cfg.LagMonitorInterval)

		// Use the sharded consumer's client as the main kafka client (for producing)
		app.kafka = app.shardedOrdersConsumer.Client()

		partitions := app.shardedOrdersConsumer.Partitions()
		log.Info("Sharded orders consumer initialized",
			zap.String("consumer_group", cfg.ConsumerGroup),
			zap.String("topic", cfg.OrdersTopic),
			zap.Int32s("assigned_partitions", partitions))

		// Create broadcast consumer for ticks (all shards need price data)
		instanceID := cfg.PodName
		if instanceID == "" {
			instanceID = fmt.Sprintf("shard-%d", cfg.ShardID)
		}
		app.broadcastTicksConsumer, kafkaErr = NewBroadcastConsumer(BroadcastConsumerConfig{
			Brokers:    cfg.KafkaBrokers,
			Topic:      cfg.TicksTopic,
			InstanceID: instanceID,
			Logger:     log,
		})
		if kafkaErr != nil {
			log.Fatal("Failed to create broadcast ticks consumer", zap.Error(kafkaErr))
		}

		// Use broadcast consumer's client for ticks
		app.ticksKafka = app.broadcastTicksConsumer.Client()

		log.Info("Broadcast ticks consumer initialized",
			zap.String("topic", cfg.TicksTopic),
			zap.String("instance_id", instanceID))

		// Create sharded consumer for close positions (partition-aware)
		app.shardedClosePositionsConsumer, kafkaErr = NewShardedConsumer(ShardedConsumerConfig{
			ShardID:           cfg.ShardID,
			ShardCount:        cfg.ShardCount,
			TotalPartitions:   cfg.TotalPartitions,
			Brokers:           cfg.KafkaBrokers,
			Topic:             cfg.ClosePositionsTopic,
			ConsumerGroup:     cfg.ConsumerGroup + "-close-positions",
			Logger:            log,
			DisableAutoCommit: true,
		})
		if kafkaErr != nil {
			log.Fatal("Failed to create sharded close positions consumer", zap.Error(kafkaErr))
		}
		app.shardedClosePositionsConsumer.SetMetrics(app.consumerMetrics)
		app.shardedClosePositionsConsumer.StartLagMonitor(cfg.LagMonitorInterval)
		log.Info("Sharded close positions consumer initialized",
			zap.String("topic", cfg.ClosePositionsTopic),
			zap.Int32s("assigned_partitions", app.shardedClosePositionsConsumer.Partitions()))

		// Create sharded consumer for cancel orders (partition-aware)
		app.shardedCancelOrdersConsumer, kafkaErr = NewShardedConsumer(ShardedConsumerConfig{
			ShardID:           cfg.ShardID,
			ShardCount:        cfg.ShardCount,
			TotalPartitions:   cfg.TotalPartitions,
			Brokers:           cfg.KafkaBrokers,
			Topic:             cfg.CancelOrdersTopic,
			ConsumerGroup:     cfg.ConsumerGroup + "-cancel-orders",
			Logger:            log,
			DisableAutoCommit: true,
		})
		if kafkaErr != nil {
			log.Fatal("Failed to create sharded cancel orders consumer", zap.Error(kafkaErr))
		}
		app.shardedCancelOrdersConsumer.SetMetrics(app.consumerMetrics)
		app.shardedCancelOrdersConsumer.StartLagMonitor(cfg.LagMonitorInterval)
		log.Info("Sharded cancel orders consumer initialized",
			zap.String("topic", cfg.CancelOrdersTopic),
			zap.Int32s("assigned_partitions", app.shardedCancelOrdersConsumer.Partitions()))

		// Create sharded consumer for modify TP/SL (partition-aware)
		app.shardedModifyTPSLConsumer, kafkaErr = NewShardedConsumer(ShardedConsumerConfig{
			ShardID:           cfg.ShardID,
			ShardCount:        cfg.ShardCount,
			TotalPartitions:   cfg.TotalPartitions,
			Brokers:           cfg.KafkaBrokers,
			Topic:             cfg.ModifyTPSLTopic,
			ConsumerGroup:     cfg.ConsumerGroup + "-modify-tpsl",
			Logger:            log,
			DisableAutoCommit: true,
		})
		if kafkaErr != nil {
			log.Fatal("Failed to create sharded modify TP/SL consumer", zap.Error(kafkaErr))
		}
		app.shardedModifyTPSLConsumer.SetMetrics(app.consumerMetrics)
		app.shardedModifyTPSLConsumer.StartLagMonitor(cfg.LagMonitorInterval)
		log.Info("Sharded modify TP/SL consumer initialized",
			zap.String("topic", cfg.ModifyTPSLTopic),
			zap.Int32s("assigned_partitions", app.shardedModifyTPSLConsumer.Partitions()))
	} else {
		// Standard mode: use regular Kafka consumer groups
		secOpts := infra.KafkaSecurityOpts()

		// Optimized settings for high throughput (1000+ concurrent users)
		ordersOpts := []kgo.Opt{
			kgo.SeedBrokers(cfg.KafkaBrokers...),
			kgo.ConsumerGroup(cfg.ConsumerGroup),
			kgo.ConsumeTopics(cfg.OrdersTopic),
			// Producer optimizations for high throughput
			kgo.ProducerBatchCompression(kgo.Lz4Compression()), // LZ4: fast compression, good throughput
			kgo.ProducerLinger(50 * time.Millisecond),          // Increased batch window for better batching
			kgo.ProducerBatchMaxBytes(1024 * 1024),             // 1MB max batch size
			kgo.RequiredAcks(kgo.AllISRAcks()),
			kgo.RetryTimeout(10 * time.Second),
			// Consumer optimizations
			kgo.DisableAutoCommit(),
			kgo.FetchMaxBytes(1024 * 1024 * 5),      // 5MB max fetch per partition
			kgo.FetchMaxPartitionBytes(1024 * 1024), // 1MB per partition
		}
		ordersOpts = append(ordersOpts, secOpts...)
		app.kafka, kafkaErr = kgo.NewClient(ordersOpts...)
		if kafkaErr != nil {
			log.Fatal("Failed to create Kafka client", zap.Error(kafkaErr))
		}
		log.Info("Kafka orders client initialized (high-throughput mode)",
			zap.String("consumer_group", cfg.ConsumerGroup),
			zap.String("topic", cfg.OrdersTopic))

		// Initialize separate Kafka consumer for ticks with optimized settings
		ticksOpts := []kgo.Opt{
			kgo.SeedBrokers(cfg.KafkaBrokers...),
			kgo.ConsumerGroup(cfg.ConsumerGroup + "-ticks"),
			kgo.ConsumeTopics(cfg.TicksTopic),
			kgo.DisableAutoCommit(),
			// Consumer optimizations for tick data (high volume)
			kgo.FetchMaxBytes(1024 * 1024 * 10),         // 10MB max fetch (ticks are high volume)
			kgo.FetchMaxPartitionBytes(1024 * 1024 * 2), // 2MB per partition
			kgo.FetchMinBytes(1024),                     // Minimum fetch size to batch more records
		}
		ticksOpts = append(ticksOpts, secOpts...)
		app.ticksKafka, kafkaErr = kgo.NewClient(ticksOpts...)
		if kafkaErr != nil {
			log.Fatal("Failed to create ticks Kafka client", zap.Error(kafkaErr))
		}
		log.Info("Kafka ticks client initialized (high-throughput mode)",
			zap.String("consumer_group", cfg.ConsumerGroup+"-ticks"),
			zap.String("topic", cfg.TicksTopic))

		// Initialize separate Kafka consumer for close positions
		closePosOpts := []kgo.Opt{
			kgo.SeedBrokers(cfg.KafkaBrokers...),
			kgo.ConsumerGroup(cfg.ConsumerGroup + "-close-positions"),
			kgo.ConsumeTopics(cfg.ClosePositionsTopic),
			kgo.DisableAutoCommit(),
			kgo.FetchMaxBytes(1024 * 1024),         // 1MB max fetch
			kgo.FetchMaxPartitionBytes(256 * 1024), // 256KB per partition
		}
		closePosOpts = append(closePosOpts, secOpts...)
		app.closePositionsKafka, kafkaErr = kgo.NewClient(closePosOpts...)
		if kafkaErr != nil {
			log.Fatal("Failed to create close positions Kafka client", zap.Error(kafkaErr))
		}
		log.Info("Kafka close positions client initialized",
			zap.String("consumer_group", cfg.ConsumerGroup+"-close-positions"),
			zap.String("topic", cfg.ClosePositionsTopic))

		// Initialize separate Kafka consumer for cancel orders
		cancelOpts := []kgo.Opt{
			kgo.SeedBrokers(cfg.KafkaBrokers...),
			kgo.ConsumerGroup(cfg.ConsumerGroup + "-cancel-orders"),
			kgo.ConsumeTopics(cfg.CancelOrdersTopic),
			kgo.DisableAutoCommit(),
			kgo.FetchMaxBytes(1024 * 1024),         // 1MB max fetch
			kgo.FetchMaxPartitionBytes(256 * 1024), // 256KB per partition
		}
		cancelOpts = append(cancelOpts, secOpts...)
		app.cancelOrdersKafka, kafkaErr = kgo.NewClient(cancelOpts...)
		if kafkaErr != nil {
			log.Fatal("Failed to create cancel orders Kafka client", zap.Error(kafkaErr))
		}
		log.Info("Kafka cancel orders client initialized",
			zap.String("consumer_group", cfg.ConsumerGroup+"-cancel-orders"),
			zap.String("topic", cfg.CancelOrdersTopic))

		// Initialize separate Kafka consumer for modify TP/SL
		modifyOpts := []kgo.Opt{
			kgo.SeedBrokers(cfg.KafkaBrokers...),
			kgo.ConsumerGroup(cfg.ConsumerGroup + "-modify-tpsl"),
			kgo.ConsumeTopics(cfg.ModifyTPSLTopic),
			kgo.DisableAutoCommit(),
			kgo.FetchMaxBytes(1024 * 1024),         // 1MB max fetch
			kgo.FetchMaxPartitionBytes(256 * 1024), // 256KB per partition
		}
		modifyOpts = append(modifyOpts, secOpts...)
		app.modifyTPSLKafka, kafkaErr = kgo.NewClient(modifyOpts...)
		if kafkaErr != nil {
			log.Fatal("Failed to create modify TP/SL Kafka client", zap.Error(kafkaErr))
		}
		log.Info("Kafka modify TP/SL client initialized",
			zap.String("consumer_group", cfg.ConsumerGroup+"-modify-tpsl"),
			zap.String("topic", cfg.ModifyTPSLTopic))
	}

	// Initialize trading engine (with or without sharding)
	if cfg.ShardEnabled {
		log.Info("Sharding enabled",
			zap.Int("shard_id", cfg.ShardID),
			zap.Int("shard_count", cfg.ShardCount))

		// Create db.Pool for sharded state manager
		dbPool, err := db.NewPool(ctx, db.Config{
			PrimaryDSN:      cfg.PostgresDSN,
			MaxOpenConns:    config.GetEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    config.GetEnvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: 5 * time.Minute,
		})
		if err != nil {
			log.Fatal("Failed to create database pool", zap.Error(err))
		}
		app.dbPool = dbPool

		// Create sharded state manager
		app.shardedState = NewShardedStateManager(cfg.ShardID, cfg.ShardCount, dbPool, app.redis)

		// Warm up the sharded state manager
		warmUpCtx, warmUpCancel := context.WithTimeout(ctx, 30*time.Second)
		if err := app.shardedState.WarmUp(warmUpCtx); err != nil {
			log.Warn("Failed to warm up sharded state", zap.Error(err))
			// Send notification for shard warmup failure
			app.sendShardStateError(err, cfg.ShardID)
		} else {
			stats := app.shardedState.GetStats()
			log.Info("Sharded state warmed up",
				zap.Int("assigned_contests", stats.AssignedContests),
				zap.Int("loaded_contests", stats.LoadedContests),
				zap.Duration("warmup_duration", stats.WarmUpDuration))
		}
		warmUpCancel()

		// Create sharded engine
		app.engine = NewShardedEngine(app.db, app.redis, app.kafka, cfg, app.shardedState, log)
	} else {
		// Create regular engine
		app.engine = NewEngine(app.db, app.redis, app.kafka, cfg, log)
	}

	// Initialize engine metrics
	engineMetrics := NewEngineMetrics(obs.Metrics.Registry(), "trading_engine")
	app.engine.SetMetrics(engineMetrics)
	log.Info("Engine metrics initialized")

	// Log cache configuration
	log.Info("Cache configuration",
		zap.Bool("cache_enabled", cfg.CacheEnabled),
		zap.Duration("contest_cache_ttl", cfg.ContestCacheTTL),
		zap.Duration("participant_cache_ttl", cfg.ParticipantCacheTTL),
		zap.Duration("cache_cleanup_interval", cfg.CacheCleanupInterval))

	// Initialize rate limiter with metrics
	rateLimitMetrics := NewRateLimitMetrics(obs.Metrics.Registry(), "trading_engine")
	rateLimiter := NewOrderRateLimiter(cfg.RateLimits, rateLimitMetrics)
	rateLimiter.SetLogger(log)
	// Provide participant count lookup for dynamic contest rate limiting
	rateLimiter.SetParticipantCountFunc(func(ctx context.Context, contestID string) (int, error) {
		return GetContestParticipantCount(ctx, app.db, contestID)
	})
	app.engine.SetRateLimiter(rateLimiter)
	rateLimiter.StartCleanup(time.Minute)
	log.Info("Rate limiter initialized",
		zap.Int("user_per_second", cfg.RateLimits.UserPerSecond),
		zap.Int("user_per_minute", cfg.RateLimits.UserPerMinute),
		zap.Int("contest_per_second", cfg.RateLimits.ContestPerSecond),
		zap.Int("global_per_second", cfg.RateLimits.GlobalPerSecond),
		zap.Bool("dynamic_contest_limits", cfg.RateLimits.DynamicContestLimits),
		zap.Int("contest_limit_base_rate", cfg.RateLimits.ContestLimitBaseRate),
		zap.Int("contest_limit_multiplier", cfg.RateLimits.ContestLimitMultiplier),
		zap.Int("contest_limit_refresh_secs", cfg.RateLimits.ContestLimitRefreshSecs))

	// Start periodic cache & rate limiter metrics collection (every 10s)
	app.engine.StartCacheMetricsLoop()
	log.Info("Cache metrics collection started")

	// Initialize market hours checker
	if cfg.MarketHoursEnabled {
		marketHours, err := NewMarketHoursChecker(cfg.MarketHoursConfigPath, log)
		if err != nil {
			log.Warn("Failed to initialize market hours checker - market hours enforcement disabled",
				zap.Error(err),
				zap.String("config_path", cfg.MarketHoursConfigPath))
		} else {
			app.engine.SetMarketHoursChecker(marketHours)
			// Start monitoring market status changes
			marketHours.StartMonitor(ctx, time.Minute)
			log.Info("Market hours checker initialized",
				zap.String("config_path", cfg.MarketHoursConfigPath))
		}
	} else {
		log.Info("Market hours enforcement disabled")
	}

	// Initialize WAL (Write-Ahead Log) for state consistency
	app.engine.InitWAL(log)

	// Initialize state consistency checker
	app.stateConsistencyChecker = NewStateConsistencyChecker(
		app.db,
		app.engine.state,
		app.shardedState,
		app.engine.GetWAL(),
		log,
		cfg.ShardEnabled,
	)
	log.Info("State consistency checker initialized")

	// Replay pending WAL entries on startup
	replayCtx, replayCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := app.engine.ReplayWAL(replayCtx); err != nil {
		log.Warn("Failed to replay pending WAL entries", zap.Error(err))
	}
	replayCancel()

	// Compact WAL file after replay to remove committed/rolled-back entries
	if wal := app.engine.GetWAL(); wal != nil {
		if err := wal.Compact(); err != nil {
			log.Warn("Failed to compact WAL file", zap.Error(err))
		}
	}

	// Reload pending order book from database (P0-2)
	reloadCtx, reloadCancel := context.WithTimeout(ctx, 15*time.Second)
	if err := app.engine.pendingBook.ReloadFromDB(reloadCtx, app.db, log); err != nil {
		log.Error("Failed to reload pending order book from database", zap.Error(err))
	}
	reloadCancel()

	// Start position lock cleanup goroutine
	app.engine.StartPositionLockCleanup(ctx)

	// Initialize and start price freshness monitor
	if cfg.PriceFreshnessMonitorEnabled {
		app.priceFreshnessMonitor = NewPriceFreshnessMonitor(
			app.engine.priceBook,
			engineMetrics,
			app.kafka,
			cfg,
			log,
		)
		app.priceFreshnessMonitor.Start()
		log.Info("Price freshness monitor initialized and started")
	}

	// Start Kafka consumers
	app.wg.Add(5)
	go app.consumeOrders()
	go app.consumeTicks()
	go app.consumeClosePositions()
	go app.consumeCancelOrders()
	go app.consumeModifyTPSL()

	// Start contest operations consumers (bulk close positions, cancel orders, state events)
	app.wg.Add(3)
	go app.consumeContestClosePositions()
	go app.consumeContestCancelOrders()
	go app.consumeContestStateEvents()

	// Start periodic unrealized score broadcast
	app.wg.Add(1)
	go app.broadcastUnrealizedScores()

	// Mark as ready
	app.ready.Store(true)

	// Send startup notification
	app.sendStartupNotification()

	// Set up HTTP server
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(obs.Middleware.Middleware)
	r.Use(obs.Middleware.Recovery)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", app.handleHealthz)
	r.Get("/readyz", app.handleReadyz)
	r.Get("/health/circuits", app.circuits.HandleCircuitHealth())
	r.Get("/health/prices", app.handlePriceFreshness)
	r.Get("/health/state-consistency", app.handleStateConsistency)
	r.With(validation.InternalOnlyMiddleware).Handle("/metrics", obs.MetricsHandler())
	r.Get("/shards", app.handleShardStats)
	r.Get("/rate-limits", app.handleRateLimiterStats)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	infra.SafeGo(log, "http-server", func() {
		log.Info("Starting trading-engine", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("Server error", zap.Error(err))
		}
	})

	// Wait for shutdown signal (from parent context or OS signal)
	if parentCtx != nil {
		<-parentCtx.Done()
	} else {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
	}
	log.Info("Shutting down...")

	// Send shutdown notification
	app.sendShutdownNotification()

	// Mark as not ready
	app.ready.Store(false)

	// Stop price freshness monitor
	if app.priceFreshnessMonitor != nil {
		app.priceFreshnessMonitor.Stop()
	}

	// Stop position lock cleanup
	if app.engine != nil {
		app.engine.StopPositionLockCleanup()
	}

	// Stop rate limiter cleanup
	if app.engine != nil && app.engine.GetRateLimiter() != nil {
		app.engine.GetRateLimiter().StopCleanup()
	}

	// Stop cache metrics collection
	if app.engine != nil {
		app.engine.StopCacheMetricsLoop()
	}

	// Cancel context to stop goroutines
	cancel()

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("Server forced to shutdown", zap.Error(err))
	}

	// Wait for goroutines
	app.wg.Wait()

	// Close connections
	if app.config.PartitionAwareEnabled {
		// Close partition-aware consumers
		if app.shardedModifyTPSLConsumer != nil {
			app.shardedModifyTPSLConsumer.Close()
		}
		if app.shardedCancelOrdersConsumer != nil {
			app.shardedCancelOrdersConsumer.Close()
		}
		if app.shardedClosePositionsConsumer != nil {
			app.shardedClosePositionsConsumer.Close()
		}
		if app.broadcastTicksConsumer != nil {
			app.broadcastTicksConsumer.Close()
		}
		if app.shardedOrdersConsumer != nil {
			app.shardedOrdersConsumer.Close()
		}
	} else {
		// Close standard consumers
		if app.modifyTPSLKafka != nil {
			app.modifyTPSLKafka.Close()
		}
		if app.cancelOrdersKafka != nil {
			app.cancelOrdersKafka.Close()
		}
		if app.closePositionsKafka != nil {
			app.closePositionsKafka.Close()
		}
		if app.ticksKafka != nil {
			app.ticksKafka.Close()
		}
		if app.kafka != nil {
			app.kafka.Close()
		}
	}
	if app.redis != nil && !app.sharedRedis {
		app.redis.Close()
	}
	if app.dbPool != nil {
		app.dbPool.Close()
	}
	if app.db != nil && !app.sharedDB {
		app.db.Close()
	}

	// Close WAL file handle
	if app.engine != nil {
		if wal := app.engine.GetWAL(); wal != nil {
			if err := wal.Close(); err != nil {
				log.Warn("Failed to close WAL file", zap.Error(err))
			}
		}
	}

	// Shutdown notification service (drain pending notifications)
	if app.notifications != nil {
		notifShutdownCtx, notifShutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := app.notifications.Shutdown(notifShutdownCtx); err != nil {
			log.Warn("Notification service shutdown error", zap.Error(err))
		}
		notifShutdownCancel()
	}

	log.Info("Shutdown complete")
}

// consumeOrders consumes order requests from Kafka.
func (a *App) consumeOrders() {
	defer a.wg.Done()

	if a.config.PartitionAwareEnabled && a.shardedOrdersConsumer != nil {
		a.consumeOrdersPartitionAware()
	} else {
		a.consumeOrdersStandard()
	}
}

// consumeOrdersStandard uses standard consumer group-based consumption.
func (a *App) consumeOrdersStandard() {
	a.log().Info("Starting standard order consumer", zap.String("topic", a.config.OrdersTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Order consumer shutting down")
			return
		default:
		}

		fetches := a.kafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processOrderRecord(record)
			}

			// Commit offsets after processing
			if err := a.kafka.CommitUncommittedOffsets(a.ctx); err != nil {
				a.log().Error("Commit error", zap.Error(err))
			}
		})
	}
}

// consumeOrdersPartitionAware uses partition-aware consumption for sharded deployments.
func (a *App) consumeOrdersPartitionAware() {
	partitions := a.shardedOrdersConsumer.Partitions()
	a.log().Info("Starting partition-aware order consumer",
		zap.String("topic", a.config.OrdersTopic),
		zap.Int("shard_id", a.config.ShardID),
		zap.Int32s("assigned_partitions", partitions))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Partition-aware order consumer shutting down")
			return
		default:
		}

		fetches := a.shardedOrdersConsumer.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				startTime := time.Now()

				a.processOrderRecord(record)

				// Record metrics
				processingTime := time.Since(startTime)
				a.shardedOrdersConsumer.RecordProcessed(p.Partition, record.Offset, processingTime)
			}

			// Commit offsets after processing
			if err := a.shardedOrdersConsumer.CommitOffsets(a.ctx); err != nil {
				a.log().Error("Commit error", zap.Error(err))
			}
		})
	}
}

// processOrderRecord processes a single order record from Kafka.
func (a *App) processOrderRecord(record *kgo.Record) {
	var order contracts.OrderRequest
	if err := json.Unmarshal(record.Value, &order); err != nil {
		a.log().Error("Failed to unmarshal order", zap.Error(err))
		return
	}

	// Process the order
	if err := a.engine.ProcessOrder(a.ctx, &order); err != nil {
		a.log().Error("Failed to process order", zap.String("order_id", order.OrderID), zap.Error(err))
		// Send notification for persistent order processing failures
		a.sendOrderProcessingError(err, &order)
	}
}

// consumeTicks consumes tick snapshots from Kafka for pending order and TP/SL evaluation.
func (a *App) consumeTicks() {
	defer a.wg.Done()

	a.log().Info("Starting ticks consumer", zap.String("topic", a.config.TicksTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Ticks consumer shutting down")
			return
		default:
		}

		fetches := a.ticksKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Ticks fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processTickRecord(record)
			}

			// Commit offsets after processing
			if err := a.ticksKafka.CommitUncommittedOffsets(a.ctx); err != nil {
				a.log().Error("Ticks commit error", zap.Error(err))
			}
		})
	}
}

// processTickRecord processes a single tick record from Kafka.
func (a *App) processTickRecord(record *kgo.Record) {
	var tick contracts.TickSnapshot
	if err := json.Unmarshal(record.Value, &tick); err != nil {
		a.log().Error("Failed to unmarshal tick", zap.Error(err))
		return
	}

	// Process the tick (updates price book and evaluates pending orders/TP/SL)
	a.engine.ProcessTick(a.ctx, &tick)
}

// broadcastUnrealizedScores periodically broadcasts unrealized score updates.
func (a *App) broadcastUnrealizedScores() {
	defer a.wg.Done()

	interval := a.config.UnrealizedBroadcastInterval
	if interval <= 0 {
		interval = 1 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	a.log().Info("Starting unrealized score broadcaster", zap.Duration("interval", interval))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Unrealized score broadcaster shutting down")
			return
		case <-ticker.C:
			a.engine.BroadcastUnrealizedScores(a.ctx)
		}
	}
}

// handleHealthz returns basic health status.
func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleReadyz returns readiness status.
func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	response := map[string]interface{}{
		"status":    "ready",
		"service":   "trading-engine",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	httpStatus := http.StatusOK

	// Check if we're ready (initialization complete)
	if !a.ready.Load() {
		response["status"] = "unavailable"
		response["message"] = "service not initialized"
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Check circuit breaker health
	if !a.circuits.IsHealthy() {
		response["status"] = "unavailable"
		response["circuits"] = a.circuits.GetStatus()
		response["message"] = "circuit breakers unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}
	response["circuits"] = "healthy"

	// Check database connection (critical)
	if err := a.db.PingContext(ctx); err != nil {
		response["status"] = "unavailable"
		response["database"] = "unavailable"
		response["message"] = "database connectivity check failed"
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}
	response["database"] = "healthy"

	// Check Redis connection (critical - used for price book and order state)
	if err := a.redis.Ping(ctx).Err(); err != nil {
		response["status"] = "unavailable"
		response["redis"] = "unavailable"
		response["message"] = "redis connectivity check failed"
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}
	response["redis"] = "healthy"

	// Check shard readiness if sharding is enabled
	if a.config.ShardEnabled && a.shardedState != nil {
		if !a.shardedState.IsReady() {
			response["status"] = "unavailable"
			response["shard"] = "not_ready"
			response["message"] = "shard not ready"
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(response)
			return
		}
		response["shard"] = map[string]interface{}{
			"status":      "healthy",
			"shard_id":    a.config.ShardID,
			"shard_count": a.config.ShardCount,
		}
	}

	// Add Kafka consumer info
	response["kafka"] = map[string]interface{}{
		"status":         "healthy",
		"consumer_group": a.config.ConsumerGroup,
		"orders_topic":   a.config.OrdersTopic,
	}

	if a.config.PartitionAwareEnabled {
		kafkaInfo := response["kafka"].(map[string]interface{})
		kafkaInfo["partition_aware_enabled"] = true
		kafkaInfo["total_partitions"] = a.config.TotalPartitions
		if a.shardedOrdersConsumer != nil {
			stats := a.shardedOrdersConsumer.GetStats()
			kafkaInfo["assigned_partitions"] = stats.AssignedPartitions
		}
	}

	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}

// handleShardStats returns shard statistics.
func (a *App) handleShardStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{}

	// Add partition-aware consumer stats
	if a.config.PartitionAwareEnabled && a.shardedOrdersConsumer != nil {
		consumerStats := a.shardedOrdersConsumer.GetStats()
		response["partition_aware"] = map[string]interface{}{
			"enabled":             true,
			"shard_id":            consumerStats.ShardID,
			"shard_count":         consumerStats.ShardCount,
			"topic":               consumerStats.Topic,
			"consumer_group":      consumerStats.ConsumerGroup,
			"assigned_partitions": consumerStats.AssignedPartitions,
		}
	} else {
		response["partition_aware"] = map[string]interface{}{
			"enabled": false,
		}
	}

	// Add shard state stats
	if a.config.ShardEnabled && a.shardedState != nil {
		stats := a.shardedState.GetStats()
		response["shard_state"] = map[string]interface{}{
			"enabled":                true,
			"shard_id":               stats.ShardID,
			"shard_count":            stats.ShardCount,
			"assigned_contests":      stats.AssignedContests,
			"loaded_contests":        stats.LoadedContests,
			"total_users":            stats.TotalUsers,
			"total_positions":        stats.TotalPositions,
			"total_pending_orders":   stats.TotalPendingOrders,
			"last_assignment_reload": stats.LastAssignmentReload,
			"warmup_duration_ms":     stats.WarmUpDuration.Milliseconds(),
			"ready":                  stats.Ready,
		}
	} else {
		response["shard_state"] = map[string]interface{}{
			"enabled": false,
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleRateLimiterStats returns rate limiter statistics.
func (a *App) handleRateLimiterStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rateLimiter := a.engine.GetRateLimiter()
	if rateLimiter == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "rate limiter not initialized",
			"enabled": false,
		})
		return
	}

	response := rateLimiter.GetStats()
	response["enabled"] = true
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handlePriceFreshness returns the freshness status of all tracked price symbols.
func (a *App) handlePriceFreshness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if a.priceFreshnessMonitor == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "price freshness monitor not enabled",
			"enabled": false,
		})
		return
	}

	response := a.priceFreshnessMonitor.GetFreshnessStatus()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleStateConsistency handles requests for state consistency checks.
// It compares in-memory positions against database records and reports any divergence.
func (a *App) handleStateConsistency(w http.ResponseWriter, r *http.Request) {
	if a.stateConsistencyChecker == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "state consistency checker not initialized",
			"enabled": false,
		})
		return
	}

	a.stateConsistencyChecker.HandleStateConsistency(w, r)
}

// consumeClosePositions consumes close position requests from Kafka.
func (a *App) consumeClosePositions() {
	defer a.wg.Done()

	// In partition-aware mode, we use the same consumer group as orders
	// since close positions should be routed to the same shard as orders
	if a.config.PartitionAwareEnabled {
		a.consumeClosePositionsPartitionAware()
	} else {
		a.consumeClosePositionsStandard()
	}
}

// consumeClosePositionsStandard uses standard consumer group-based consumption.
func (a *App) consumeClosePositionsStandard() {
	a.log().Info("Starting close positions consumer", zap.String("topic", a.config.ClosePositionsTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Close positions consumer shutting down")
			return
		default:
		}

		fetches := a.closePositionsKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Close positions fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processClosePositionRecord(record)
			}

			// Commit offsets after processing
			if err := a.closePositionsKafka.CommitUncommittedOffsets(a.ctx); err != nil {
				a.log().Error("Close positions commit error", zap.Error(err))
			}
		})
	}
}

// consumeClosePositionsPartitionAware uses partition-aware consumption for sharded deployments.
// Uses the pre-initialized shardedClosePositionsConsumer with the same partition assignment as orders.
func (a *App) consumeClosePositionsPartitionAware() {
	partitions := a.shardedClosePositionsConsumer.Partitions()
	a.log().Info("Starting close positions consumer (partition-aware mode)",
		zap.String("topic", a.config.ClosePositionsTopic),
		zap.Int("shard_id", a.config.ShardID),
		zap.Int32s("assigned_partitions", partitions))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Close positions consumer shutting down")
			return
		default:
		}

		fetches := a.shardedClosePositionsConsumer.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Close positions fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processClosePositionRecord(record)
			}

			if err := a.shardedClosePositionsConsumer.CommitOffsets(a.ctx); err != nil {
				a.log().Error("Close positions commit error", zap.Error(err))
			}
		})
	}
}

// processClosePositionRecord processes a single close position record from Kafka.
func (a *App) processClosePositionRecord(record *kgo.Record) {
	var req contracts.ClosePositionRequest
	if err := json.Unmarshal(record.Value, &req); err != nil {
		a.log().Error("Failed to unmarshal close position request", zap.Error(err))
		return
	}

	// Process the close position request
	if err := a.engine.ProcessClosePosition(a.ctx, &req); err != nil {
		a.log().Error("Failed to process close position request",
			zap.String("position_id", req.PositionID),
			zap.String("user_id", req.UserID),
			zap.String("contest_id", req.ContestID),
			zap.Error(err))
	}
}

// consumeCancelOrders consumes cancel order requests from Kafka.
func (a *App) consumeCancelOrders() {
	defer a.wg.Done()

	// In partition-aware mode, we use the same consumer group as orders
	// since cancel orders should be routed to the same shard as orders
	if a.config.PartitionAwareEnabled {
		a.consumeCancelOrdersPartitionAware()
	} else {
		a.consumeCancelOrdersStandard()
	}
}

// consumeCancelOrdersStandard uses standard consumer group-based consumption.
func (a *App) consumeCancelOrdersStandard() {
	a.log().Info("Starting cancel orders consumer", zap.String("topic", a.config.CancelOrdersTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Cancel orders consumer shutting down")
			return
		default:
		}

		fetches := a.cancelOrdersKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Cancel orders fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processCancelOrderRecord(record)
			}

			// Commit offsets after processing
			if err := a.cancelOrdersKafka.CommitUncommittedOffsets(a.ctx); err != nil {
				a.log().Error("Cancel orders commit error", zap.Error(err))
			}
		})
	}
}

// consumeCancelOrdersPartitionAware uses partition-aware consumption for sharded deployments.
// Uses the pre-initialized shardedCancelOrdersConsumer with the same partition assignment as orders.
func (a *App) consumeCancelOrdersPartitionAware() {
	partitions := a.shardedCancelOrdersConsumer.Partitions()
	a.log().Info("Starting cancel orders consumer (partition-aware mode)",
		zap.String("topic", a.config.CancelOrdersTopic),
		zap.Int("shard_id", a.config.ShardID),
		zap.Int32s("assigned_partitions", partitions))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Cancel orders consumer shutting down")
			return
		default:
		}

		fetches := a.shardedCancelOrdersConsumer.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Cancel orders fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processCancelOrderRecord(record)
			}

			if err := a.shardedCancelOrdersConsumer.CommitOffsets(a.ctx); err != nil {
				a.log().Error("Cancel orders commit error", zap.Error(err))
			}
		})
	}
}

// processCancelOrderRecord processes a single cancel order record from Kafka.
func (a *App) processCancelOrderRecord(record *kgo.Record) {
	var req contracts.CancelOrderRequest
	if err := json.Unmarshal(record.Value, &req); err != nil {
		a.log().Error("Failed to unmarshal cancel order request", zap.Error(err))
		return
	}

	// Process the cancel order request
	if err := a.engine.ProcessCancelOrder(a.ctx, &req); err != nil {
		a.log().Error("Failed to process cancel order request",
			zap.String("order_id", req.OrderID),
			zap.String("user_id", req.UserID),
			zap.String("contest_id", req.ContestID),
			zap.Error(err))
	}
}

// consumeModifyTPSL consumes modify TP/SL requests from Kafka.
func (a *App) consumeModifyTPSL() {
	defer a.wg.Done()

	// In partition-aware mode, we use the same partition assignment logic
	if a.config.PartitionAwareEnabled {
		a.consumeModifyTPSLPartitionAware()
	} else {
		a.consumeModifyTPSLStandard()
	}
}

// consumeModifyTPSLStandard uses standard consumer group-based consumption.
func (a *App) consumeModifyTPSLStandard() {
	a.log().Info("Starting modify TP/SL consumer", zap.String("topic", a.config.ModifyTPSLTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Modify TP/SL consumer shutting down")
			return
		default:
		}

		fetches := a.modifyTPSLKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Modify TP/SL fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processModifyTPSLRecord(record)
			}

			// Commit offsets after processing
			if err := a.modifyTPSLKafka.CommitUncommittedOffsets(a.ctx); err != nil {
				a.log().Error("Modify TP/SL commit error", zap.Error(err))
			}
		})
	}
}

// consumeModifyTPSLPartitionAware uses partition-aware consumption for sharded deployments.
// Uses the pre-initialized shardedModifyTPSLConsumer with the same partition assignment as orders.
func (a *App) consumeModifyTPSLPartitionAware() {
	partitions := a.shardedModifyTPSLConsumer.Partitions()
	a.log().Info("Starting modify TP/SL consumer (partition-aware mode)",
		zap.String("topic", a.config.ModifyTPSLTopic),
		zap.Int("shard_id", a.config.ShardID),
		zap.Int32s("assigned_partitions", partitions))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Modify TP/SL consumer shutting down")
			return
		default:
		}

		fetches := a.shardedModifyTPSLConsumer.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Modify TP/SL fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processModifyTPSLRecord(record)
			}

			if err := a.shardedModifyTPSLConsumer.CommitOffsets(a.ctx); err != nil {
				a.log().Error("Modify TP/SL commit error", zap.Error(err))
			}
		})
	}
}

// processModifyTPSLRecord processes a single modify TP/SL record from Kafka.
func (a *App) processModifyTPSLRecord(record *kgo.Record) {
	var req contracts.ModifyTPSLRequest
	if err := json.Unmarshal(record.Value, &req); err != nil {
		a.log().Error("Failed to unmarshal modify TP/SL request", zap.Error(err))
		return
	}

	// Process the modify TP/SL request
	if err := a.engine.ProcessModifyTPSL(a.ctx, &req); err != nil {
		a.log().Error("Failed to process modify TP/SL request",
			zap.String("position_id", req.PositionID),
			zap.String("user_id", req.UserID),
			zap.String("contest_id", req.ContestID),
			zap.Error(err))
	}
}
