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
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/domain/statemachine"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/resilience"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// App holds application state and dependencies.
type App struct {
	config       *Config
	db           *sql.DB
	redis        *pkgredis.Client
	kafka        *kgo.Client
	wallet       *wallet.Service
	obs          *observability.Observability
	stateMachine *statemachine.StateMachine

	// Sharded leaderboard support
	shardedWorker *ShardedLeaderboardWorker

	// Enhanced leaderboard with score breakdowns and usernames
	enhancedLB *EnhancedLeaderboardManager

	// Notification support
	notifications   *notification.Service
	alertAggregator *AlertAggregator

	// Circuit breakers
	circuits *CircuitBreakers

	// Smart snapshot tracking
	dirtyContestsMu  sync.Mutex
	dirtyContests    map[string]bool // Contests with significant score changes since last snapshot
	lastFullSnapshot time.Time       // Last time a full snapshot was written for all contests

	// State
	ready  atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Shared resource flags
	sharedDB    bool
	sharedRedis bool
}

// log returns the logger from observability
func (a *App) log() *zap.Logger {
	return a.obs.Logger.Logger
}

// Run starts the leaderboard-worker service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil)
}

// RunWithSharedDeps starts the leaderboard-worker service, optionally using shared resources.
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
		Service:              "leaderboard-worker",
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
	circuitCfg := DefaultCircuitBreakerConfig()
	circuitCfg.Logger = log
	circuitCfg.OnStateChange = func(name string, from, to circuitbreaker.State) {
		log.Warn("Circuit breaker state changed",
			zap.String("circuit", name),
			zap.String("from", from.String()),
			zap.String("to", to.String()))
	}
	circuits := NewCircuitBreakers(circuitCfg)

	// Create a new cancellable context for the application
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	app := &App{
		config:        cfg,
		ctx:           ctx,
		cancel:        cancel,
		obs:           obs,
		circuits:      circuits,
		dirtyContests: make(map[string]bool),
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
		app.db.SetMaxOpenConns(config.GetEnvInt("DB_MAX_OPEN_CONNS", 15))
		app.db.SetMaxIdleConns(config.GetEnvInt("DB_MAX_IDLE_CONNS", 5))
		app.db.SetConnMaxLifetime(5 * time.Minute)

		// Test database connection
		dbCtx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := app.db.PingContext(dbCtx); err != nil {
			log.Fatal("Failed to connect to database", zap.Error(err))
		}
		dbCancel()
		log.Info("PostgreSQL connected successfully",
			zap.Int("max_open_conns", config.GetEnvInt("DB_MAX_OPEN_CONNS", 15)),
			zap.Int("max_idle_conns", config.GetEnvInt("DB_MAX_IDLE_CONNS", 5)))
	}

	// Register DB connection pool metrics (collected every 10s)
	dbPoolMetrics := observability.NewDBPoolMetrics(obs.Metrics.Registry(), "leaderboard_worker")
	dbPoolMetrics.AddDB(app.db)
	dbPoolMetrics.Start(10 * time.Second)
	defer dbPoolMetrics.Stop()

	// Initialize wallet service
	app.wallet = wallet.NewService(app.db)

	// Initialize db.Pool for state machine (read/write splitting support)
	var dbPool *db.Pool
	if sharedPool != nil {
		dbPool = sharedPool
		log.Info("Using shared database pool for state machine")
	} else {
		dbPoolCfg := db.ConfigFromEnv(os.Getenv)
		dbPoolCfg.PrimaryDSN = cfg.PostgresDSN
		var poolErr error
		dbPool, poolErr = db.NewPool(ctx, dbPoolCfg)
		if poolErr != nil {
			log.Fatal("Failed to create database pool for state machine", zap.Error(poolErr))
		}
		defer dbPool.Close()
	}

	// Initialize Sarama Kafka producer for state machine event publishing
	saramaConfig := sarama.NewConfig()
	saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
	saramaConfig.Producer.Retry.Max = 5
	saramaConfig.Producer.Return.Successes = true

	saramaProducer, saramaErr := sarama.NewSyncProducer(cfg.KafkaBrokers, saramaConfig)
	if saramaErr != nil {
		log.Fatal("Failed to create Sarama Kafka producer for state machine", zap.Error(saramaErr))
	}
	defer saramaProducer.Close()

	// Initialize state machine for contest status transitions
	smConfig := &statemachine.Config{
		KafkaProducer:     saramaProducer,
		ContestStateTopic: cfg.ContestStateTopic,
		Logger:            log,
	}
	app.stateMachine = statemachine.New(dbPool, smConfig)
	log.Info("State machine initialized for contest status transitions")

	// Initialize Redis with HA support (standalone, sentinel, or cluster)
	if sharedRedis != nil {
		app.redis = sharedRedis
		app.sharedRedis = true
		log.Info("Using shared Redis client")
	} else {
		redisCfg := pkgredis.Config{
			Mode:           pkgredis.Mode(cfg.RedisMode),
			Addr:           cfg.RedisAddr,
			Password:       cfg.RedisPassword,
			ClusterAddrs:   cfg.RedisClusterAddrs,
			SentinelAddrs:  cfg.RedisSentinelAddrs,
			SentinelMaster: cfg.RedisSentinelMaster,
			DialTimeout:    5 * time.Second,
			ReadTimeout:    3 * time.Second,
			WriteTimeout:   3 * time.Second,
			PoolSize:       20,
			MinIdleConns:   5,
		}

		app.redis, err = pkgredis.NewClient(redisCfg)
		if err != nil {
			log.Fatal("Failed to create Redis client", zap.Error(err))
		}

		// Test Redis connection
		redisCtx, redisCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := app.redis.Ping(redisCtx).Err(); err != nil {
			log.Fatal("Failed to connect to Redis", zap.Error(err))
		}
		redisCancel()
		log.Info("Redis connected successfully",
			zap.String("mode", string(app.redis.Mode())))
	}

	// Initialize sharded leaderboard worker
	shardedCfg := ShardedLeaderboardConfig{
		ShardCount:             cfg.ShardCount,
		CacheTTL:               cfg.CacheTTL,
		SignificantScoreChange: cfg.SignificantScoreChange,
	}
	app.shardedWorker = NewShardedLeaderboardWorker(app.redis, shardedCfg)
	log.Info("Sharded leaderboard worker initialized",
		zap.Int("shard_count", cfg.ShardCount),
		zap.Int("shard_id", cfg.ShardID),
		zap.Duration("cache_ttl", cfg.CacheTTL))

	// Initialize enhanced leaderboard manager with score breakdowns and usernames
	app.enhancedLB = NewEnhancedLeaderboardManager(app.redis.Client(), app.db)
	log.Info("Enhanced leaderboard manager initialized")

	// Initialize notification service
	app.initNotifications(ctx, log)

	// Initialize Kafka consumer - optimized for high-throughput PnL delta processing
	var kafkaErr error
	kafkaOpts := []kgo.Opt{
		kgo.SeedBrokers(cfg.KafkaBrokers...),
		kgo.ConsumerGroup(cfg.ConsumerGroup),
		kgo.ConsumeTopics(cfg.PnLDeltasTopic),
		kgo.DisableAutoCommit(),
		// Consumer optimizations for high-volume PnL delta processing
		kgo.FetchMaxBytes(1024 * 1024 * 5),      // 5MB max fetch
		kgo.FetchMaxPartitionBytes(1024 * 1024), // 1MB per partition
		kgo.FetchMinBytes(1024),                 // Batch more records for efficiency
	}
	kafkaOpts = append(kafkaOpts, infra.KafkaSecurityOpts()...)
	app.kafka, kafkaErr = kgo.NewClient(kafkaOpts...)
	if kafkaErr != nil {
		log.Fatal("Failed to create Kafka client", zap.Error(kafkaErr))
	}
	log.Info("Kafka client initialized (high-throughput mode)",
		zap.String("consumer_group", cfg.ConsumerGroup),
		zap.String("topic", cfg.PnLDeltasTopic))

	// Start Kafka consumer for PnL deltas
	app.wg.Add(1)
	go app.consumePnLDeltas()

	// Start Kafka consumer for contest state events
	app.wg.Add(1)
	go app.startContestStateConsumer()

	// Start Kafka consumer for in-app notification events
	app.wg.Add(1)
	go app.startNotificationConsumer()

	// Start snapshot writer
	app.wg.Add(1)
	go app.runSnapshotWriter()

	// Start cache cleanup for sharded worker
	app.wg.Add(1)
	infra.SafeGo(log, "sharded-worker-cache-cleanup", func() {
		defer app.wg.Done()
		app.shardedWorker.StartCacheCleanup(ctx, time.Minute)
	})

	// Start periodic cleanup for enhanced leaderboard username cache
	app.wg.Add(1)
	infra.SafeGo(log, "enhanced-lb-username-cache-cleanup", func() {
		defer app.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.enhancedLB.CleanupUsernameCache()
			}
		}
	})

	// Start affiliate commission crediting job (runs every hour, processes commissions older than 7 days)
	app.wg.Add(1)
	go app.runCommissionCreditingJob()

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
	r.Get("/health/circuits", app.handleCircuitHealth)
	r.With(validation.InternalOnlyMiddleware).Handle("/metrics", obs.MetricsHandler())

	// Cross-shard leaderboard endpoints
	r.Get("/api/v1/leaderboard/global", app.handleGlobalLeaderboard)
	r.Get("/api/v1/leaderboard/{contestID}", app.handleContestLeaderboard)
	r.Get("/api/v1/leaderboard/{contestID}/user/{userID}", app.handleUserRank)

	// Enhanced leaderboard endpoints with score breakdowns and usernames (Tralent-like)
	r.Get("/api/v1/leaderboard/{contestID}/enhanced", app.handleEnhancedLeaderboard)
	r.Get("/api/v1/leaderboard/{contestID}/position", app.handleUserPosition)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	infra.SafeGo(log, "leaderboard-worker-http-server", func() {
		log.Info("Starting leaderboard-worker", zap.String("port", cfg.Port))
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

	// Send shutdown notification before marking not ready
	app.sendShutdownNotification()

	// Mark as not ready
	app.ready.Store(false)

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
	if app.kafka != nil {
		app.kafka.Close()
	}
	if app.redis != nil && !app.sharedRedis {
		app.redis.Close()
	}
	if app.db != nil && !app.sharedDB {
		app.db.Close()
	}

	// Shutdown notification service (allow pending notifications to drain)
	if app.notifications != nil {
		notifyShutdownCtx, notifyShutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := app.notifications.Shutdown(notifyShutdownCtx); err != nil {
			log.Warn("Notification service shutdown error", zap.Error(err))
		}
		notifyShutdownCancel()
	}

	log.Info("Shutdown complete")
}

// consumePnLDeltas consumes PnL delta events from Kafka and updates Redis sorted sets.
// Records are collected into batches and flushed when the batch reaches PnLBatchSize
// or PnLBatchTimeout elapses, whichever comes first. This allows small batches to
// accumulate across partitions for efficiency while ensuring timely processing.
func (a *App) consumePnLDeltas() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("consumePnLDeltas panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	a.log().Info("Starting PnL deltas consumer",
		zap.String("topic", a.config.PnLDeltasTopic),
		zap.Int("batch_size", a.config.PnLBatchSize),
		zap.Duration("batch_timeout", a.config.PnLBatchTimeout),
		zap.Bool("disable_standalone_redis", a.config.DisableStandaloneRedis))

	batch := make([]contracts.PnLDelta, 0, a.config.PnLBatchSize)
	lastFlushTime := time.Now()

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("PnL deltas consumer shutting down")
			// Flush any remaining batch
			if len(batch) > 0 {
				a.processPnLDeltaBatch(batch)
			}
			return
		default:
		}

		fetches := a.kafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				// Flush remaining batch on shutdown
				if len(batch) > 0 {
					a.processPnLDeltaBatch(batch)
				}
				return
			}
			a.log().Error("Fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				var delta contracts.PnLDelta
				if err := json.Unmarshal(record.Value, &delta); err != nil {
					a.log().Error("Failed to unmarshal PnLDelta", zap.Error(err))
					continue
				}
				batch = append(batch, delta)

				// Flush when batch is full
				if len(batch) >= a.config.PnLBatchSize {
					a.processPnLDeltaBatch(batch)
					batch = batch[:0]
					lastFlushTime = time.Now()
				}
			}
		})

		// Time-based flush: if batch has items and timeout has elapsed, flush now
		// to ensure partial batches don't wait indefinitely for more records
		if len(batch) > 0 && time.Since(lastFlushTime) >= a.config.PnLBatchTimeout {
			a.processPnLDeltaBatch(batch)
			batch = batch[:0]
			lastFlushTime = time.Now()
		}

		// Commit offsets after processing
		if err := a.kafka.CommitUncommittedOffsets(a.ctx); err != nil {
			a.log().Error("Commit error", zap.Error(err))
		}
	}
}

// processPnLDeltaBatch processes a batch of PnL delta records using pipelined Redis operations.
// It groups deltas by contestID so each contest gets a single ZADD with multiple members.
func (a *App) processPnLDeltaBatch(deltas []contracts.PnLDelta) {
	if len(deltas) == 0 {
		return
	}

	// Group deltas by contestID. For each contest+user, keep only the latest delta.
	// P3-1: Use SeqNum for ordering - keep the delta with the highest SeqNum per user+contest.
	// This ensures correct ordering even when Kafka delivers messages out of order.
	type userScore struct {
		TotalScore      float64
		RealizedScore   float64
		UnrealizedScore float64
		DeltaScore      float64
		SeqNum          uint64
	}
	contestUpdates := make(map[string]map[string]*userScore) // contestID -> userID -> score

	for i := range deltas {
		d := &deltas[i]
		users, ok := contestUpdates[d.ContestID]
		if !ok {
			users = make(map[string]*userScore)
			contestUpdates[d.ContestID] = users
		}
		existing, exists := users[d.UserID]
		// P3-1: If SeqNum is available, keep highest; otherwise fallback to last-write-wins
		if exists && d.SeqNum > 0 && existing.SeqNum > 0 && d.SeqNum < existing.SeqNum {
			continue // Discard out-of-order delta
		}
		users[d.UserID] = &userScore{
			TotalScore:      d.TotalScore,
			RealizedScore:   d.RealizedScore,
			UnrealizedScore: d.UnrealizedScore,
			DeltaScore:      d.DeltaScore,
			SeqNum:          d.SeqNum,
		}
	}

	// Process each contest: combine sorted set + breakdown writes into single pipeline
	for contestID, users := range contestUpdates {
		updates := make(map[string]float64, len(users))
		for userID, s := range users {
			updates[userID] = s.TotalScore
		}

		// Batch update via sharded worker (pipelined ZADD with hash tags)
		failedUsers, err := a.shardedWorker.BatchUpdateScores(a.ctx, contestID, updates)
		if err != nil {
			a.log().Error("Failed to batch update sharded leaderboard",
				zap.String("contest_id", contestID),
				zap.Int("user_count", len(updates)),
				zap.Int("failed_users", len(failedUsers)),
				zap.Strings("failed_user_ids", failedUsers),
				zap.Error(err))
			a.sendLeaderboardCalculationError(contestID, err)
		}

		// Mark contest as dirty for smart snapshots if any score change is significant
		for _, s := range users {
			if s.DeltaScore >= a.config.SignificantScoreChange || s.DeltaScore <= -a.config.SignificantScoreChange {
				a.markContestDirty(contestID)
				break
			}
		}

		// Standalone Redis for backward compatibility (skip if disabled)
		if !a.config.DisableStandaloneRedis {
			key := leaderboardKey(contestID)
			zMembers := make([]redis.Z, 0, len(users))
			for userID, s := range users {
				zMembers = append(zMembers, redis.Z{
					Score:  s.TotalScore,
					Member: userID,
				})
			}
			if err := a.redis.ZAdd(a.ctx, key, zMembers...).Err(); err != nil {
				a.log().Error("Failed to batch update standalone leaderboard",
					zap.String("contest_id", contestID),
					zap.Int("user_count", len(zMembers)),
					zap.Error(err))
				a.sendLeaderboardCalculationError(contestID, err)
			}
		}

		// Batch update enhanced leaderboard breakdowns (updates in-memory cache too)
		if a.enhancedLB != nil {
			scoreUpdates := make([]ScoreUpdate, 0, len(users))
			for userID, s := range users {
				scoreUpdates = append(scoreUpdates, ScoreUpdate{
					UserID:          userID,
					TotalScore:      s.TotalScore,
					RealizedScore:   s.RealizedScore,
					UnrealizedScore: s.UnrealizedScore,
				})
			}
			if err := a.enhancedLB.BatchUpdateScoresWithBreakdown(a.ctx, contestID, scoreUpdates); err != nil {
				a.log().Warn("Failed to batch update score breakdowns",
					zap.String("contest_id", contestID),
					zap.Int("user_count", len(scoreUpdates)),
					zap.Error(err))
			}
		}

		a.log().Debug("Batch leaderboard update",
			zap.String("contest_id", contestID),
			zap.Int("user_count", len(users)))
	}
}

// markContestDirty marks a contest as having significant score changes since the last snapshot.
func (a *App) markContestDirty(contestID string) {
	a.dirtyContestsMu.Lock()
	a.dirtyContests[contestID] = true
	a.dirtyContestsMu.Unlock()
}

// runSnapshotWriter periodically writes leaderboard snapshots to the database.
// It uses smart snapshot logic: only contests with significant score changes ("dirty" contests)
// get snapshots on each tick. As a safety net, a full snapshot of ALL active contests is written
// every FullSnapshotInterval (default 5 minutes).
func (a *App) runSnapshotWriter() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("runSnapshotWriter panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	ticker := time.NewTicker(a.config.SnapshotInterval)
	defer ticker.Stop()

	a.lastFullSnapshot = time.Now()

	a.log().Info("Starting snapshot writer",
		zap.Duration("interval", a.config.SnapshotInterval),
		zap.Duration("full_snapshot_interval", a.config.FullSnapshotInterval))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Snapshot writer shutting down")
			return
		case <-ticker.C:
			a.writeSnapshots()
		}
	}
}

// writeSnapshots writes snapshots using smart dirty-tracking logic.
// Only contests marked dirty (significant score changes) get snapshots each tick.
// Every FullSnapshotInterval, all active contests get a snapshot regardless of dirty flags.
func (a *App) writeSnapshots() {
	fullSnapshot := time.Since(a.lastFullSnapshot) >= a.config.FullSnapshotInterval

	if fullSnapshot {
		// Safety net: write snapshots for ALL active contests
		// Use sharded worker's key scanner which handles cluster mode correctly
		keys := a.shardedWorker.scanLeaderboardKeys(a.ctx)
		// Also include standalone keys if not disabled
		if !a.config.DisableStandaloneRedis {
			standaloneKeys := a.scanStandaloneLeaderboardKeys(a.ctx)
			keys = append(keys, standaloneKeys...)
		}

		// Deduplicate contest IDs and write snapshots
		allSucceeded := true
		seen := make(map[string]bool)
		for _, key := range keys {
			contestID := extractContestIDFromKey(key)
			if contestID == "" || seen[contestID] {
				continue
			}
			seen[contestID] = true
			if !a.writeContestSnapshot(contestID) {
				allSucceeded = false
			}
		}

		// Only clear dirty flags for successfully written contests.
		// If all succeeded, clear everything and reset the timer.
		// If any failed, those contests stay dirty for retry on next tick.
		if allSucceeded {
			a.dirtyContestsMu.Lock()
			a.dirtyContests = make(map[string]bool)
			a.dirtyContestsMu.Unlock()
			a.lastFullSnapshot = time.Now()
		} else {
			// Partial success: still reset the timer to avoid retrying full snapshot
			// every tick, but don't clear dirty flags so failed contests get retried.
			a.lastFullSnapshot = time.Now()
		}

		a.log().Info("Full snapshot written for all active contests",
			zap.Int("contest_count", len(seen)),
			zap.Bool("all_succeeded", allSucceeded))
		return
	}

	// Incremental: only write snapshots for dirty contests.
	// Extract dirty contest IDs but do NOT clear them yet.
	a.dirtyContestsMu.Lock()
	dirty := make([]string, 0, len(a.dirtyContests))
	for contestID := range a.dirtyContests {
		dirty = append(dirty, contestID)
	}
	a.dirtyContestsMu.Unlock()

	if len(dirty) == 0 {
		return
	}

	// Write snapshots and only clear dirty flags for successful writes.
	written := 0
	for _, contestID := range dirty {
		if a.writeContestSnapshot(contestID) {
			a.dirtyContestsMu.Lock()
			delete(a.dirtyContests, contestID)
			a.dirtyContestsMu.Unlock()
			written++
		}
		// On failure, the contest stays in dirtyContests and will be retried next tick.
	}

	a.log().Debug("Smart snapshot written for dirty contests",
		zap.Int("dirty_count", len(dirty)),
		zap.Int("written_count", written))
}

// writeContestSnapshot writes a snapshot for a specific contest.
// For contests with ≤100 participants, all entries are included in the snapshot
// to preserve complete history. For larger contests, only the top SnapshotTopN
// entries are captured to keep snapshot sizes manageable.
// Returns true if the snapshot was written successfully, false on any error.
func (a *App) writeContestSnapshot(contestID string) bool {
	// Determine how many entries to snapshot based on contest size
	// Use sharded worker for consistent key format (lb:{contestID})
	topN := a.config.SnapshotTopN
	size, err := a.shardedWorker.GetLeaderboardSize(a.ctx, contestID)
	if err == nil && size > 0 && size <= int64(a.config.SnapshotTopN) {
		// Small contest: snapshot all participants
		topN = int(size)
	}

	// Get top N entries (sharded worker decodes tiebreaker for clean scores)
	entries, err := a.shardedWorker.GetTop(a.ctx, contestID, topN)
	if err != nil {
		a.log().Error("Failed to get leaderboard for contest",
			zap.String("contest_id", contestID), zap.Error(err))
		a.sendSnapshotWriteError(contestID, err)
		return false
	}

	if len(entries) == 0 {
		return true // Nothing to write is not a failure
	}

	// Create snapshot payload
	snapshot := LeaderboardSnapshot{
		ContestID: contestID,
		TakenAt:   time.Now().UTC(),
		Entries:   entries,
	}

	payloadJSON, err := json.Marshal(snapshot.Entries)
	if err != nil {
		a.log().Error("Failed to marshal snapshot",
			zap.String("contest_id", contestID), zap.Error(err))
		a.sendSnapshotWriteError(contestID, err)
		return false
	}

	// Insert into database
	_, err = a.db.ExecContext(a.ctx,
		`INSERT INTO leaderboard_snapshots (contest_id, taken_at, payload_json) VALUES ($1, $2, $3)`,
		contestID, snapshot.TakenAt, payloadJSON)
	if err != nil {
		a.log().Error("Failed to insert snapshot",
			zap.String("contest_id", contestID), zap.Error(err))
		a.sendSnapshotWriteError(contestID, err)
		return false
	}

	a.log().Info("Snapshot written",
		zap.String("contest_id", contestID),
		zap.Int("entries", len(entries)))
	return true
}

// scanStandaloneLeaderboardKeys discovers lb:* keys using SCAN instead of KEYS.
// SCAN is non-blocking and iterates the keyspace incrementally, avoiding the
// O(N) blocking behavior of KEYS that can freeze Redis under load.
func (a *App) scanStandaloneLeaderboardKeys(ctx context.Context) []string {
	var keys []string
	iter := a.redis.Scan(ctx, 0, "lb:*", 100).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		a.log().Warn("SCAN error for standalone leaderboard keys", zap.Error(err))
	}
	return keys
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
		"service":   "leaderboard-worker",
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
		response["circuits"] = a.circuits.GetHealth()
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

	// Check Redis connection (critical - used for leaderboard sorted sets)
	if err := a.redis.Ping(ctx).Err(); err != nil {
		response["status"] = "unavailable"
		response["redis"] = "unavailable"
		response["message"] = "redis connectivity check failed"
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(response)
		return
	}
	response["redis"] = "healthy"

	// Check Kafka consumer (critical - needed to consume PnL deltas)
	if a.kafka != nil {
		response["kafka"] = map[string]interface{}{
			"status":         "healthy",
			"consumer_group": a.config.ConsumerGroup,
			"topic":          a.config.PnLDeltasTopic,
		}
	} else {
		response["kafka"] = "unavailable"
		response["status"] = "degraded"
		response["message"] = "kafka consumer not initialized"
	}

	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}

// handleCircuitHealth returns the health status of all circuit breakers.
func (a *App) handleCircuitHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	health := a.circuits.GetHealth()

	status := http.StatusOK
	if health.Overall == "unhealthy" {
		status = http.StatusServiceUnavailable
	}

	w.WriteHeader(status)
	json.NewEncoder(w).Encode(health)
}

// handleGlobalLeaderboard returns the global cross-shard leaderboard.
func (a *App) handleGlobalLeaderboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse limit from query params (default 100)
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	entries, err := a.shardedWorker.GetCrossShardRanking(r.Context(), limit)
	if err != nil {
		a.log().Error("Failed to get global leaderboard", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to retrieve global leaderboard",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"entries": entries,
		"count":   len(entries),
		"limit":   limit,
	})
}

// handleContestLeaderboard returns the leaderboard for a specific contest.
func (a *App) handleContestLeaderboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	contestID := chi.URLParam(r, "contestID")
	if contestID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "contestID is required",
		})
		return
	}

	// Parse limit from query params (default 100)
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	entries, err := a.shardedWorker.GetTop(r.Context(), contestID, limit)
	if err != nil {
		a.log().Error("Failed to get contest leaderboard",
			zap.String("contest_id", contestID),
			zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to retrieve contest leaderboard",
		})
		return
	}

	// Get total count
	total, _ := a.shardedWorker.GetLeaderboardSize(r.Context(), contestID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"contest_id": contestID,
		"entries":    entries,
		"count":      len(entries),
		"total":      total,
		"limit":      limit,
	})
}

// handleUserRank returns a user's rank in a specific contest.
func (a *App) handleUserRank(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	contestID := chi.URLParam(r, "contestID")
	userID := chi.URLParam(r, "userID")

	if contestID == "" || userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "contestID and userID are required",
		})
		return
	}

	// Get user rank
	rank, err := a.shardedWorker.GetUserRank(r.Context(), contestID, userID)
	if err != nil {
		a.log().Error("Failed to get user rank",
			zap.String("contest_id", contestID),
			zap.String("user_id", userID),
			zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to retrieve user rank",
		})
		return
	}

	if rank == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "user not found in leaderboard",
		})
		return
	}

	// Also get surrounding entries for context
	before := 5
	after := 5
	if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
		if b, err := strconv.Atoi(beforeStr); err == nil && b >= 0 && b <= 50 {
			before = b
		}
	}
	if afterStr := r.URL.Query().Get("after"); afterStr != "" {
		if a, err := strconv.Atoi(afterStr); err == nil && a >= 0 && a <= 50 {
			after = a
		}
	}

	surrounding, _ := a.shardedWorker.GetContestLeaderboardWithSurrounding(
		r.Context(), contestID, userID, before, after,
	)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"contest_id":  contestID,
		"user_id":     userID,
		"rank":        rank.Rank,
		"score":       rank.Score,
		"surrounding": surrounding,
	})
}

// ============================================================================
// Affiliate Commission Crediting Job
// ============================================================================

// runCommissionCreditingJob runs a periodic job to credit pending affiliate commissions.
// Commissions are credited after 7 days to allow for refund/chargeback periods.
func (a *App) runCommissionCreditingJob() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("runCommissionCreditingJob panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	// Run every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	a.log().Info("Starting affiliate commission crediting job", zap.Duration("interval", 1*time.Hour))

	// Run once at startup (delayed by 1 minute to let other services initialize)
	select {
	case <-a.ctx.Done():
		a.log().Info("Commission crediting job shutting down before first run")
		return
	case <-time.After(1 * time.Minute):
		a.creditPendingCommissions()
	}

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Commission crediting job shutting down")
			return
		case <-ticker.C:
			a.creditPendingCommissions()
		}
	}
}

// creditPendingCommissions finds and credits pending commissions older than 7 days.
func (a *App) creditPendingCommissions() {
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()

	// Find pending commissions older than 7 days (configurable maturity period)
	maturityDays := 7
	rows, err := a.db.QueryContext(ctx, `
		SELECT id, referrer_id, commission_cents
		FROM affiliate_commissions
		WHERE status = 'pending'
		AND created_at < NOW() - INTERVAL '1 day' * $1
		ORDER BY created_at ASC
		LIMIT 100
	`, maturityDays)

	if err != nil {
		a.log().Error("Failed to query pending commissions", zap.Error(err))
		return
	}
	defer rows.Close()

	type pendingCommission struct {
		ID              string
		ReferrerID      string
		CommissionCents int64
	}

	var commissions []pendingCommission
	for rows.Next() {
		var c pendingCommission
		if err := rows.Scan(&c.ID, &c.ReferrerID, &c.CommissionCents); err != nil {
			a.log().Error("Failed to scan commission row", zap.Error(err))
			continue
		}
		commissions = append(commissions, c)
	}

	if err := rows.Err(); err != nil {
		a.log().Error("Commission row iteration error", zap.Error(err))
		return
	}

	if len(commissions) == 0 {
		a.log().Debug("No pending commissions to credit")
		return
	}

	a.log().Info("Processing pending commissions", zap.Int("count", len(commissions)))

	var credited, failed int
	retryCfg := resilience.DatabaseRetryConfig()
	retryCfg.RetryIf = isRetryableDBError

	for _, c := range commissions {
		err := resilience.RetryVoid(ctx, retryCfg, func(ctx context.Context) error {
			return a.creditSingleCommission(ctx, c.ID, c.ReferrerID, c.CommissionCents)
		})
		if err != nil {
			a.log().Error("Failed to credit commission after retries",
				zap.String("commission_id", c.ID),
				zap.String("referrer_id", c.ReferrerID),
				zap.Int64("commission_cents", c.CommissionCents),
				zap.Error(err))
			failed++
		} else {
			credited++
		}
	}

	a.log().Info("Commission crediting batch complete",
		zap.Int("credited", credited),
		zap.Int("failed", failed))
}

// creditSingleCommission credits a single commission to the referrer's wallet.
func (a *App) creditSingleCommission(ctx context.Context, commissionID, referrerID string, commissionCents int64) error {
	// Begin transaction
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if commission is still pending (concurrent safety)
	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT status FROM affiliate_commissions WHERE id = $1 FOR UPDATE
	`, commissionID).Scan(&status)
	if err != nil {
		return fmt.Errorf("failed to lock commission: %w", err)
	}

	if status != "pending" {
		// Already processed, skip
		return nil
	}

	// Credit to wallet using the wallet service
	_, err = a.wallet.CreditAffiliateCommission(ctx, tx, referrerID, commissionID, commissionCents)
	if err != nil {
		return fmt.Errorf("failed to credit wallet: %w", err)
	}

	// Update commission status to credited
	_, err = tx.ExecContext(ctx, `
		UPDATE affiliate_commissions
		SET status = 'credited', credited_at = NOW()
		WHERE id = $1
	`, commissionID)
	if err != nil {
		return fmt.Errorf("failed to update commission status: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	a.log().Info("Credited affiliate commission",
		zap.String("commission_id", commissionID),
		zap.String("referrer_id", referrerID),
		zap.Int64("commission_cents", commissionCents))

	return nil
}

// isRetryableDBError determines if a database error is transient and should be retried.
// It targets PostgreSQL deadlock (40P01) and serialization failure (40001) errors.
func isRetryableDBError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// 40P01 = deadlock_detected, 40001 = serialization_failure
		return pgErr.Code == "40P01" || pgErr.Code == "40001"
	}
	return false
}

// extractContestIDFromKey extracts the contest ID from Redis key formats.
// Handles both "lb:{contestID}" (sharded) and "lb:contestID" (standalone).
func extractContestIDFromKey(key string) string {
	if !strings.HasPrefix(key, "lb:") {
		return ""
	}
	id := key[3:]
	// Strip hash tag braces from sharded keys
	if strings.HasPrefix(id, "{") && strings.HasSuffix(id, "}") {
		id = id[1 : len(id)-1]
	}
	// Skip shard-specific or sub-keys like lb:{contestID}:shard:0
	if strings.Contains(id, ":") {
		return ""
	}
	if id == "" {
		return ""
	}
	return id
}

// ============================================================================
// Enhanced Leaderboard Handlers (Tralent-like with score breakdowns)
// ============================================================================

// handleEnhancedLeaderboard returns the leaderboard with full score breakdowns and usernames.
// Query params: limit (default 100), offset (default 0), user_id (optional, to include user's rank)
func (a *App) handleEnhancedLeaderboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	contestID := chi.URLParam(r, "contestID")
	if contestID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "contestID is required",
		})
		return
	}

	// Parse query parameters
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Optional: user_id to include their rank in response
	currentUserID := r.URL.Query().Get("user_id")

	if a.enhancedLB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "enhanced leaderboard not available",
		})
		return
	}

	response, err := a.enhancedLB.GetEnhancedLeaderboard(r.Context(), contestID, limit, offset, currentUserID)
	if err != nil {
		a.log().Error("Failed to get enhanced leaderboard",
			zap.String("contest_id", contestID),
			zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to retrieve leaderboard",
		})
		return
	}

	json.NewEncoder(w).Encode(response)
}

// handleUserPosition returns a user's position in the leaderboard with surrounding entries.
// Query params: user_id (required), before (default 5), after (default 5)
func (a *App) handleUserPosition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	contestID := chi.URLParam(r, "contestID")
	if contestID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "contestID is required",
		})
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "user_id is required",
		})
		return
	}

	// Parse before/after counts
	before := 5
	if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
		if b, err := strconv.Atoi(beforeStr); err == nil && b >= 0 && b <= 50 {
			before = b
		}
	}

	after := 5
	if afterStr := r.URL.Query().Get("after"); afterStr != "" {
		if a, err := strconv.Atoi(afterStr); err == nil && a >= 0 && a <= 50 {
			after = a
		}
	}

	if a.enhancedLB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "enhanced leaderboard not available",
		})
		return
	}

	response, err := a.enhancedLB.GetUserLeaderboardPosition(r.Context(), contestID, userID, before, after)
	if err != nil {
		a.log().Error("Failed to get user position",
			zap.String("contest_id", contestID),
			zap.String("user_id", userID),
			zap.Error(err))
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(response)
}
