package server

import (
	"compress/flate"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/infra/shard"
	"github.com/Parsaeffatravesh/tragge/packages/infra/wsregistry"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/ratelimit"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	redis "github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// ====================
// Configuration
// ====================

// Config holds application configuration
type Config struct {
	Port                string
	PostgresDSN         string
	PostgresReplicaDSNs []string
	RedisAddr           string
	AuthContext         auth.ContextConfig

	// Kafka configuration
	KafkaBrokers        []string
	ConsumerGroup       string
	TicksTopic          string
	FillsTopic          string
	PositionsTopic      string
	OrderAcksTopic      string
	OrderCancelledTopic string // Consumer topic for order cancelled events
	PnLDeltasTopic      string // Consumer topic for PnL delta events (real-time score updates)
	OrdersTopic         string // Producer topic for order requests
	CancelOrdersTopic   string // Producer topic for cancel order requests
	ClosePositionsTopic string // Producer topic for close position requests
	ModifyTPSLTopic     string // Producer topic for modify TP/SL requests
	NotificationsTopic  string // Consumer topic for per-user settlement notifications

	// Broadcast configuration
	BroadcastInterval time.Duration
	MaxSymbolsPerTick int
	BroadcastWorkers  int // Number of workers for broadcast pool (default: 2 * NumCPU)

	// WebSocket compression configuration
	// EnableCompression enables per-message deflate compression (RFC 7692)
	EnableCompression bool
	// CompressionLevel sets the compression level: flate.BestSpeed (1) to flate.BestCompression (9)
	// Use flate.BestSpeed for high concurrency (lower CPU), flate.DefaultCompression for balanced
	CompressionLevel int
	// MinMessageSizeForCompression is the minimum message size in bytes to apply compression
	// Messages smaller than this threshold are sent uncompressed to avoid overhead
	MinMessageSizeForCompression int

	// Shard routing configuration
	ShardRouterAddr string
	ShardCacheTTL   time.Duration

	// WebSocket session affinity registry configuration
	PodName          string        // Pod name for registry identification
	WSRegistryTTL    time.Duration // TTL for registry entries (default: 1 hour)
	WSRegistryEnable bool          // Enable/disable registry (default: true)

	// Leaderboard broadcast debounce configuration
	LeaderboardBroadcastDebounce  time.Duration // Debounce interval for leaderboard broadcasts (default: 2s)
	LeaderboardBroadcastThreshold float64       // Minimum score change to trigger broadcast (default: 50.0)

	// Notification configuration
	NotificationEnabled      bool   // Enable/disable notifications
	NotificationAsync        bool   // Use async sending
	NotificationAsyncWorkers int    // Number of async workers
	NotificationQueueSize    int    // Async queue size
	NotificationRecipients   string // Comma-separated email recipients
	DiscordWebhookURL        string // Discord webhook URL
	ResendAPIKey             string // Resend API key
	ResendFromEmail          string // Resend from email
	Environment              string // Environment (development, staging, production)

	// Candle aggregation toggle
	// Set to false when market-ingestor handles candle aggregation
	CandleAggregationEnabled bool
}

// ====================
// Application
// ====================

// App holds application dependencies
type App struct {
	pool   *db.Pool
	redis  *pkgredis.Client
	auth   *auth.Auth
	config *Config
	obs    *observability.Observability

	// Service start time for uptime-based metric calculations
	startedAt time.Time

	// Circuit breakers for external dependencies
	circuits *Circuits

	// Shard routing
	shardMiddleware *shard.Middleware
	wsEndpoint      *shard.WebSocketEndpoint

	// WebSocket management
	upgrader   websocket.Upgrader
	hub        *Hub
	priceBook  *PriceBook
	metrics    *Metrics
	wsRegistry *wsregistry.Registry // Session affinity registry
	wsTickets  *wsTicketService
	wsAccess   *wsAccessAuthenticator

	// Candle aggregation
	candleAggregator *CandleAggregator

	// Leaderboard manager for fetching rankings
	leaderboardMgr *LeaderboardManager

	// Kafka clients (consumers)
	ticksKafka          *kgo.Client
	fillsKafka          *kgo.Client
	positionsKafka      *kgo.Client
	orderAcksKafka      *kgo.Client
	orderCancelledKafka *kgo.Client
	pnlDeltasKafka      *kgo.Client // Consumer for real-time PnL/score updates

	// Kafka producer for orders
	ordersKafka *kgo.Client

	// Leaderboard broadcast debounce (per-contest timers)
	lbDebounceMu     sync.Mutex
	lbDebounceTimers map[string]*time.Timer // contestID -> debounce timer

	// Tournament feed hub for real-time tournament browsing updates (Task 8.3)
	tournamentFeedHub *TournamentFeedHub

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Notification service
	notifications   *notification.Service
	alertAggregator *AlertAggregator
	anomalyDetector *ConnectionAnomalyDetector
	latencyTracker  *LatencyTracker
}

// log returns the logger from observability
func (a *App) log() *zap.Logger {
	return a.obs.Logger.Logger
}

// Run starts the trade-bff service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil, nil)
}

// RunWithSharedDeps starts the trade-bff service, optionally using shared resources.
// When parentCtx is non-nil, the service shuts down when the context is cancelled
// instead of registering its own signal handler. When sharedPool/sharedRedis/sharedAuth
// are non-nil, the service uses those instead of creating its own.
func RunWithSharedDeps(parentCtx context.Context, sharedPool *db.Pool, sharedRedis *pkgredis.Client, sharedAuth *auth.Auth) {
	// Validate critical environment variables in production/staging
	if sharedPool == nil {
		config.MustBeSetAny("database connection", "POSTGRES_DSN", "POSTGRES_HOST")
	}
	config.MustBeSet("KAFKA_BROKERS")
	if sharedRedis == nil {
		config.MustBeSet("REDIS_ADDR")
	}

	cfg := loadConfig()
	edgeEnvironment, edgeErr := validation.LoadAndValidateEdgeEnvironment(os.Getenv)
	if edgeErr != nil {
		log.Fatal("invalid edge security configuration")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize observability first
	obs, err := observability.New(ctx, observability.Config{
		Service:              "trade-bff",
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

	// Initialize Sentry for error tracking
	sentryDSN := os.Getenv("SENTRY_DSN")
	if sentryDSN != "" {
		err := sentry.Init(sentry.ClientOptions{
			Dsn:              sentryDSN,
			Environment:      os.Getenv("SENTRY_ENVIRONMENT"),
			Release:          os.Getenv("VERSION"),
			TracesSampleRate: 0.1,
			BeforeSend:       observability.RedactSentryEvent,
		})
		if err != nil {
			log.Warn("Failed to initialize Sentry", zap.Error(err))
		} else {
			log.Info("Sentry initialized", zap.String("environment", os.Getenv("SENTRY_ENVIRONMENT")))
			defer sentry.Flush(2 * time.Second)
		}
	}

	// Create a new cancellable context for the application
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	// Initialize metrics
	metrics := NewMetrics()

	// Initialize price book
	priceBook := NewPriceBook()

	// Initialize hub with worker pool for high-concurrency broadcasts
	hub := NewHub(priceBook, metrics, cfg.BroadcastInterval, cfg.MaxSymbolsPerTick, cfg.BroadcastWorkers)

	// Connect to PostgreSQL using connection pool with primary/replica support
	var pool *db.Pool
	if sharedPool != nil {
		pool = sharedPool
		log.Info("Using shared database pool")
	} else {
		// Pool settings configurable via env for production tuning (default: 15/5 per pod)
		dbMaxOpen := 15
		if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				dbMaxOpen = n
			}
		}
		dbMaxIdle := 5
		if v := os.Getenv("DB_MAX_IDLE_CONNS"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				dbMaxIdle = n
			}
		}
		var poolErr error
		pool, poolErr = db.NewPool(ctx, db.Config{
			PrimaryDSN:      cfg.PostgresDSN,
			ReplicaDSNs:     cfg.PostgresReplicaDSNs,
			MaxOpenConns:    dbMaxOpen,
			MaxIdleConns:    dbMaxIdle,
			ConnMaxLifetime: 5 * time.Minute,
		})
		if poolErr != nil {
			log.Fatal("Failed to create database pool", zap.Error(poolErr))
		}
		defer pool.Close()

		log.Info("Connected to PostgreSQL",
			zap.Int("max_open_conns", dbMaxOpen),
			zap.Int("max_idle_conns", dbMaxIdle),
			zap.Int("replicas", len(cfg.PostgresReplicaDSNs)))
	}

	// Register DB connection pool metrics (collected every 10s)
	dbPoolMetrics := observability.NewDBPoolMetrics(obs.Metrics.Registry(), "trade_bff")
	dbPoolMetrics.AddDB(pool.Primary())
	dbPoolMetrics.Start(10 * time.Second)
	defer dbPoolMetrics.Stop()

	// Connect to Redis with HA support (optional)
	var redisClient *pkgredis.Client
	if sharedRedis != nil {
		redisClient = sharedRedis
		log.Info("Using shared Redis client")
	} else if cfg.RedisAddr != "" {
		// Use ConfigFromEnv to support standalone, sentinel, and cluster modes
		redisCfg := pkgredis.ConfigFromEnv(os.Getenv)
		if redisCfg.Addr == "" && redisCfg.Mode == pkgredis.ModeStandalone {
			redisCfg.Addr = cfg.RedisAddr
		}
		var redisErr error
		redisClient, redisErr = pkgredis.NewClient(redisCfg)
		if redisErr != nil {
			log.Warn("Failed to create Redis client", zap.Error(redisErr))
		} else {
			redisCtx, redisCancel := context.WithTimeout(ctx, 2*time.Second)
			if pingErr := redisClient.Ping(redisCtx).Err(); pingErr != nil {
				log.Warn("Redis connection failed (Redis is optional)", zap.Error(pingErr))
				redisClient.Close()
				redisClient = nil
			} else {
				log.Info("Connected to Redis",
					zap.String("mode", string(redisClient.Mode())))
			}
			redisCancel()
		}
	}

	// Initialize candle aggregator (in-memory only; market-ingestor persists to PostgreSQL)
	candleAggregator := NewCandleAggregator()

	// Initialize leaderboard manager for fetching rankings
	var leaderboardMgr *LeaderboardManager
	if redisClient != nil && pool != nil {
		leaderboardMgr = NewLeaderboardManager(redisClient.UniversalClient, pool.Primary())
		leaderboardMgr.StartCleanup(ctx, log)
		log.Info("Leaderboard manager initialized")
	}

	// Initialize auth service
	var authService *auth.Auth
	if sharedAuth != nil {
		if sharedAuth.Context() != auth.ContextUser {
			log.Fatal("Refusing non-User authentication context")
		}
		authService = sharedAuth
		log.Info("Using isolated User authentication context")
	} else {
		if err := cfg.AuthContext.Validate(os.Getenv("ENVIRONMENT")); err != nil {
			log.Fatal("Invalid User authentication configuration", zap.Error(err))
		}
		var redisUniversal redis.UniversalClient
		if redisClient != nil {
			redisUniversal = redisClient.UniversalClient
		}
		var authErr error
		authService, authErr = auth.NewContext(cfg.AuthContext, redisUniversal)
		if authErr != nil {
			log.Fatal("Failed to construct User authentication context", zap.Error(authErr))
		}
	}

	var wsTicketRedis redis.UniversalClient
	if redisClient != nil {
		wsTicketRedis = redisClient.UniversalClient
	}
	wsTickets := newWSTicketService(redisWSTicketStore{client: wsTicketRedis}, authService.Session, auth.ContextUser)
	wsAccess := &wsAccessAuthenticator{tokens: authService.Token, sessions: authService.Session}

	// Placeholder for app - needed for circuit breaker callback
	var app *App

	// Initialize circuit breakers for external dependencies
	// The callback uses the app pointer which is assigned after app creation
	circuits := NewCircuits(log, func(name string, from, to circuitbreaker.State) {
		log.Warn("Circuit breaker state changed",
			zap.String("circuit", name),
			zap.String("from", from.String()),
			zap.String("to", to.String()))

		// Send alert if app and notifications are initialized
		if app != nil && app.notifications != nil {
			app.sendCircuitBreakerTripped(name, to)

			// Check if all critical circuits are now open
			if to == circuitbreaker.StateOpen && !app.circuits.IsHealthy() {
				app.sendAllCircuitsOpen()
			}
		}
	})
	log.Info("Circuit breakers initialized",
		zap.Strings("circuits", []string{"postgres", "postgres-replica", "redis", "kafka", "shard-router"}))

	// Initialize shard middleware
	shardMiddleware := shard.NewMiddleware(&shard.Config{
		RouterAddr: cfg.ShardRouterAddr,
		Timeout:    2 * time.Second,
		CacheTTL:   cfg.ShardCacheTTL,
		Logger:     log,
	})
	wsEndpoint := shard.DefaultWebSocketEndpoint()

	log.Info("Shard middleware initialized",
		zap.String("router_addr", cfg.ShardRouterAddr),
		zap.Duration("cache_ttl", cfg.ShardCacheTTL))

	app = &App{
		pool:            pool,
		redis:           redisClient,
		auth:            authService,
		config:          cfg,
		obs:             obs,
		startedAt:       time.Now(),
		circuits:        circuits,
		shardMiddleware: shardMiddleware,
		wsEndpoint:      wsEndpoint,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// EnableCompression enables per-message deflate compression (RFC 7692)
			// This negotiates compression with the client during handshake
			EnableCompression: cfg.EnableCompression,
			CheckOrigin:       checkWebSocketOrigin,
		},
		hub:              hub,
		priceBook:        priceBook,
		metrics:          metrics,
		wsTickets:        wsTickets,
		wsAccess:         wsAccess,
		candleAggregator: candleAggregator,
		leaderboardMgr:   leaderboardMgr,
		lbDebounceTimers: make(map[string]*time.Timer),
		ctx:              ctx,
		cancel:           cancel,
	}

	// Give hub access to app for database queries (contest symbol cache)
	hub.SetApp(app)

	// Initialize per-contest Prometheus metrics and register with observability
	contestMetrics := NewContestMetrics()
	hub.contestMetrics = contestMetrics
	if obs.Metrics != nil {
		obs.Metrics.MustRegister(contestMetrics.Collectors()...)
	}

	// Log compression configuration
	log.Info("WebSocket compression configuration",
		zap.Bool("enabled", cfg.EnableCompression),
		zap.Int("compression_level", cfg.CompressionLevel),
		zap.Int("min_message_size", cfg.MinMessageSizeForCompression))

	log.Info("Candle aggregation", zap.Bool("enabled", cfg.CandleAggregationEnabled))

	// Initialize WebSocket session affinity registry
	if cfg.WSRegistryEnable && redisClient != nil {
		app.wsRegistry = wsregistry.New(wsregistry.Config{
			Redis:   redisClient.UniversalClient,
			PodName: cfg.PodName,
			TTL:     cfg.WSRegistryTTL,
			Logger:  log,
		})
		log.Info("WebSocket session affinity registry initialized",
			zap.String("pod_name", cfg.PodName),
			zap.Duration("ttl", cfg.WSRegistryTTL))
	} else {
		log.Info("WebSocket session affinity registry disabled")
	}

	// Initialize notification service
	app.initNotifications(ctx, log)

	// Initialize Kafka consumers
	if len(cfg.KafkaBrokers) > 0 {
		app.initKafkaConsumers()
	} else {
		log.Warn("No Kafka brokers configured, tick streaming disabled")
	}

	// Start hub
	go hub.Run()

	// Start registry heartbeat goroutine
	if app.wsRegistry != nil {
		go app.runRegistryHeartbeat(ctx)
	}

	// Start Kafka consumers
	app.startKafkaConsumers()

	// Start Redis pub/sub subscriber for real-time contest participant updates
	app.initContestParticipantSubscriber()

	// Start tournament feed hub and subscriber (Task 8.3)
	app.initTournamentFeed()

	// Start contest events consumer for real-time contest lifecycle updates
	if len(cfg.KafkaBrokers) > 0 {
		if err := app.initContestEventsConsumer(); err != nil {
			log.Warn("Failed to initialize contest events consumer", zap.Error(err))
		}

		// Start notification consumer for per-user settlement notifications
		if err := app.initNotificationConsumer(); err != nil {
			log.Warn("Failed to initialize notification consumer", zap.Error(err))
		}
	}

	// Start cross-pod disconnect subscriber for WebSocket session takeover
	app.startDisconnectSubscriber()

	// Setup router
	r := chi.NewRouter()

	// Create Sentry HTTP handler for panic recovery
	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic: true, // Re-panic after capture so sanitized recovery handles it
	})

	var edgeRedis redis.UniversalClient
	if redisClient != nil {
		edgeRedis = redisClient.UniversalClient
	}
	edgePolicy := ratelimit.NewPolicyMiddleware(edgeRedis, ratelimit.PoliciesForService("trade"), nil, func(class ratelimit.EndpointClass, reason string) {
		log.Warn("Edge security request denied", zap.String("policy_class", string(class)), zap.String("reason", reason))
	})

	// Middleware stack (order matters)
	r.Use(validation.RequestIDMiddleware)                             // Request ID tracking
	r.Use(validation.CORSMiddleware(validation.TradeBFFCORSConfig())) // CORS handling (includes WebSocket headers)
	r.Use(validation.CSRFMiddleware(validation.TradeBFFCSRFConfig())) // CSRF protection
	r.Use(validation.SecurityHeadersMiddleware)                       // Security headers
	r.Use(edgePolicy.Handler)                                         // Distributed edge abuse controls
	r.Use(redactWSTicketForTelemetry)                                 // Remove bounded ticket from telemetry URL
	r.Use(auth.RedactSecurityCredentialsForTelemetry)                 // Hide session credentials from telemetry
	r.Use(obs.Middleware.Middleware)                                  // Observability (logging, tracing)
	r.Use(sentryHandler.Handle)                                       // Sentry panic capture
	r.Use(auth.RestoreSecurityCredentialsAfterTelemetry)              // Restore secure headers for auth handlers
	r.Use(obs.Middleware.Recovery)                                    // Sanitized panic recovery
	r.Use(middleware.Timeout(30 * time.Second))                       // Request timeout
	r.Use(validation.MaxBytesMiddleware(edgeEnvironment.DefaultBodyBytes))
	r.Use(validation.ContentTypeMiddleware)

	r.Get("/healthz", app.handleHealthz)
	r.Get("/readyz", app.handleReadyz)
	r.With(validation.InternalOnlyMiddleware).Handle("/metrics", obs.MetricsHandler())
	r.Get("/ws-stats", app.handleWSStats)
	r.Get("/ws/trade", app.handleWebSocket)
	r.Get("/ws/tournaments", app.handleTournamentFeedWS) // Tournament feed WebSocket (Task 8.3)

	// Circuit breaker health endpoint
	r.Get("/health/circuits", app.circuits.HandleCircuitHealth)

	// Admin/debug endpoint for contest-level hub status (require admin auth)
	r.Group(func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdmin)
		r.Get("/admin/hub/status", app.handleHubStatus)
	})

	// API routes
	r.Route("/api/trade", func(r chi.Router) {
		// TradingView-compatible candles endpoint
		r.Get("/candles", app.handleCandles)

		// Public contest symbols endpoint
		r.With(validation.ValidatePathUUID("contest_id")).Get("/contest/{contest_id}/symbols", app.handleContestSymbols)

		// Shard discovery endpoint
		r.Get("/shard-info", app.handleShardInfo)

		// Public leaderboard endpoints (anyone can view leaderboards)
		r.Get("/leaderboard", app.handleLeaderboard)
		r.With(validation.ValidatePathUUID("contest_id")).Get("/leaderboard/{contest_id}", app.handleLeaderboardContest)
		r.With(validation.ValidatePathUUID("contest_id")).Get("/leaderboard/{contest_id}/position", app.handleLeaderboardPosition)

		// Protected routes
		r.Group(func(r chi.Router) {
			// All middleware must be defined before routes
			r.Use(app.auth.Middleware.RequireAuth)
			r.Use(edgePolicy.ActorHandler)
			r.Use(app.shardMiddleware.InjectShardContext)
			// WebSocket ticket endpoint (exchanges JWT for short-lived ticket)
			r.Post("/ws-ticket", app.handleWSTicket)
			// User info endpoint
			r.Get("/me", app.handleMe)
			// Balance/QTY info endpoint
			r.Get("/balance", app.handleGetBalance)
			r.Post("/orders", app.handlePlaceOrder)
			r.Get("/orders/history", app.handleOrderHistory)
			r.With(validation.ValidatePathUUID("order_id")).Delete("/orders/{order_id}", app.handleCancelOrder)
			r.With(validation.ValidatePathUUID("position_id")).Post("/positions/{position_id}/close", app.handleClosePosition)
			r.With(validation.ValidatePathUUID("position_id")).Put("/positions/{position_id}/tpsl", app.handleModifyTPSL)

			// Chart drawing persistence
			r.Route("/drawings/{contest_id}", func(r chi.Router) {
				r.Use(validation.ValidatePathUUID("contest_id"))
				r.Get("/", app.handleGetDrawings)
				r.Put("/", app.handleSaveDrawings)
				r.Delete("/", app.handleDeleteDrawings)
			})
		})
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    edgeEnvironment.MaxHeaderBytes,
	}

	// Start server
	infra.SafeGo(log, "trade-bff-server", func() {
		log.Info("trade-bff starting", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("Server error", zap.Error(err))
		}
	})

	// Send startup notification
	app.sendStartupNotification()

	// Wait for shutdown signal (from parent context or OS signal)
	if parentCtx != nil {
		<-parentCtx.Done()
	} else {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
	}

	log.Info("Shutting down server...")

	// Send shutdown notification
	app.sendShutdownNotification()

	// Gracefully shutdown WebSocket connections and clean up registry
	app.gracefulShutdownRegistry()

	// Cancel context to stop all goroutines
	cancel()

	// Stop hub
	hub.Stop()

	// Stop candle aggregator flush worker
	app.candleAggregator.Stop()

	// Wait for Kafka consumers to finish
	app.wg.Wait()

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("Server forced to shutdown", zap.Error(err))
	}

	// Close shard middleware (stop cache cleanup goroutine)
	app.shardMiddleware.Close()

	// Close Kafka clients
	app.closeKafkaClients()

	// Shutdown notification service
	if app.notifications != nil {
		notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := app.notifications.Shutdown(notifyCtx); err != nil {
			log.Warn("Error shutting down notification service", zap.Error(err))
		}
		notifyCancel()
	}

	// Close Redis connection
	if redisClient != nil {
		if err := redisClient.Close(); err != nil {
			log.Warn("Error closing Redis connection", zap.Error(err))
		}
	}

	log.Info("Server exited")
}

// loadConfig loads configuration from environment variables
func loadConfig() *Config {
	port := os.Getenv("TRADE_BFF_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8082"
	}

	// Build PostgreSQL DSN using secrets package (supports _FILE env vars for Docker/K8s secrets)
	postgresDSN := secrets.BuildPostgresDSN()

	// Parse replica DSNs (comma-separated)
	var postgresReplicaDSNs []string
	if replicaDSNs := os.Getenv("POSTGRES_REPLICA_DSNS"); replicaDSNs != "" {
		for _, dsn := range strings.Split(replicaDSNs, ",") {
			dsn = strings.TrimSpace(dsn)
			if dsn != "" {
				postgresReplicaDSNs = append(postgresReplicaDSNs, dsn)
			}
		}
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		if config.IsProduction() {
			log.Fatal("FATAL: REDIS_ADDR must be set in production")
		}
		redisAddr = "localhost:6379"
		log.Println("WARNING: REDIS_ADDR not set, using localhost:6379")
	}

	// Trade endpoints validate the same explicit User trust domain as user-bff.
	authIsolation := auth.LoadIsolationConfig(os.Getenv("ENVIRONMENT"), os.Getenv, secrets.Load)
	userAuthContext := authIsolation.User

	// Kafka configuration
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	var brokers []string
	if kafkaBrokers != "" {
		brokers = []string{kafkaBrokers}
	} else {
		if config.IsProduction() {
			log.Fatal("FATAL: KAFKA_BROKERS must be set in production")
		}
		brokers = []string{"localhost:9092"}
		log.Println("WARNING: KAFKA_BROKERS not set, using localhost:9092")
	}

	consumerGroup := os.Getenv("KAFKA_CONSUMER_GROUP")
	if consumerGroup == "" {
		consumerGroup = "trade-bff"
	}

	// WebSocket compression configuration
	enableCompression := true // Enabled by default
	if v := os.Getenv("WS_COMPRESSION_ENABLED"); v != "" {
		enableCompression = v == "true" || v == "1"
	}

	// Compression level: 1 (BestSpeed) to 9 (BestCompression), default 1 for high concurrency
	compressionLevel := flate.BestSpeed
	if v := os.Getenv("WS_COMPRESSION_LEVEL"); v != "" {
		if level, err := strconv.Atoi(v); err == nil && level >= 1 && level <= 9 {
			compressionLevel = level
		}
	}

	// Minimum message size for compression (default 100 bytes)
	// Messages smaller than this are sent uncompressed to avoid overhead
	minMsgSize := 100
	if v := os.Getenv("WS_COMPRESSION_MIN_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size >= 0 {
			minMsgSize = size
		}
	}

	// Shard router configuration
	shardRouterAddr := os.Getenv("SHARD_ROUTER_ADDR")
	if shardRouterAddr == "" {
		shardRouterAddr = "http://shard-router:8090"
	}

	shardCacheTTL := 30 * time.Second
	if v := os.Getenv("SHARD_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			shardCacheTTL = d
		}
	}

	// WebSocket session affinity registry configuration
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		// Fallback to hostname if POD_NAME not set
		podName, _ = os.Hostname()
	}

	wsRegistryTTL := 1 * time.Hour
	if v := os.Getenv("WS_REGISTRY_TTL"); v != "" {
		if ttl, err := strconv.Atoi(v); err == nil && ttl > 0 {
			wsRegistryTTL = time.Duration(ttl) * time.Second
		}
	}

	wsRegistryEnable := true // Enabled by default
	if v := os.Getenv("WS_REGISTRY_ENABLED"); v != "" {
		wsRegistryEnable = v == "true" || v == "1"
	}

	// Broadcast worker pool configuration (for high concurrency)
	// Default: 2 workers per CPU core
	broadcastWorkers := runtime.NumCPU() * 2
	if v := os.Getenv("BROADCAST_WORKERS"); v != "" {
		if workers, err := strconv.Atoi(v); err == nil && workers > 0 {
			broadcastWorkers = workers
		}
	}

	// Leaderboard broadcast debounce interval
	// Accumulates changes over this window before broadcasting to reduce WebSocket traffic
	lbDebounce := 2 * time.Second
	if v := os.Getenv("LEADERBOARD_BROADCAST_DEBOUNCE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			lbDebounce = d
		}
	}

	// Leaderboard broadcast threshold: minimum score change to trigger a broadcast
	lbThreshold := 50.0
	if v := os.Getenv("LEADERBOARD_BROADCAST_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			lbThreshold = f
		}
	}

	// Notification configuration
	notificationEnabled := true // Enabled by default
	if v := os.Getenv("NOTIFICATION_ENABLED"); v != "" {
		notificationEnabled = v == "true" || v == "1"
	}

	notificationAsync := true // Async by default
	if v := os.Getenv("NOTIFICATION_ASYNC"); v != "" {
		notificationAsync = v == "true" || v == "1"
	}

	notificationAsyncWorkers := 5
	if v := os.Getenv("NOTIFICATION_ASYNC_WORKERS"); v != "" {
		if workers, err := strconv.Atoi(v); err == nil && workers > 0 {
			notificationAsyncWorkers = workers
		}
	}

	notificationQueueSize := 100
	if v := os.Getenv("NOTIFICATION_QUEUE_SIZE"); v != "" {
		if size, err := strconv.Atoi(v); err == nil && size > 0 {
			notificationQueueSize = size
		}
	}

	environment := os.Getenv("ENVIRONMENT")
	if environment == "" {
		environment = "development"
	}

	// Candle aggregation: enabled by default, disabled when env var is explicitly "false"
	candleAggregationEnabled := os.Getenv("CANDLE_AGGREGATION_ENABLED") != "false"

	return &Config{
		Port:                port,
		PostgresDSN:         postgresDSN,
		PostgresReplicaDSNs: postgresReplicaDSNs,
		RedisAddr:           redisAddr,
		AuthContext:         userAuthContext,
		KafkaBrokers:        brokers,
		ConsumerGroup:       consumerGroup,
		TicksTopic:          "ticks.v1",
		FillsTopic:          "fills.v1",
		PositionsTopic:      "positions.v1",
		OrderAcksTopic:      "order_acks.v1",
		OrderCancelledTopic: "order_cancelled.v1",
		PnLDeltasTopic:      "pnl_deltas.v1",
		OrdersTopic:         "orders.v1",
		CancelOrdersTopic:   "cancel_orders.v1",
		ClosePositionsTopic: "close_positions.v1",
		ModifyTPSLTopic:     "modify_tpsl.v1",
		NotificationsTopic:  "notifications.v1",
		BroadcastInterval:   1 * time.Second,
		MaxSymbolsPerTick:   30,
		BroadcastWorkers:    broadcastWorkers,
		// Compression settings
		EnableCompression:            enableCompression,
		CompressionLevel:             compressionLevel,
		MinMessageSizeForCompression: minMsgSize,
		// Shard routing settings
		ShardRouterAddr: shardRouterAddr,
		ShardCacheTTL:   shardCacheTTL,
		// WebSocket session affinity registry settings
		PodName:          podName,
		WSRegistryTTL:    wsRegistryTTL,
		WSRegistryEnable: wsRegistryEnable,
		// Leaderboard broadcast debounce
		LeaderboardBroadcastDebounce:  lbDebounce,
		LeaderboardBroadcastThreshold: lbThreshold,
		// Notification settings
		NotificationEnabled:      notificationEnabled,
		NotificationAsync:        notificationAsync,
		NotificationAsyncWorkers: notificationAsyncWorkers,
		NotificationQueueSize:    notificationQueueSize,
		NotificationRecipients:   os.Getenv("NOTIFICATION_RECIPIENTS"),
		DiscordWebhookURL:        os.Getenv("DISCORD_WEBHOOK_URL"),
		ResendAPIKey:             os.Getenv("RESEND_API_KEY"),
		ResendFromEmail:          os.Getenv("RESEND_FROM_EMAIL"),
		Environment:              environment,
		CandleAggregationEnabled: candleAggregationEnabled,
	}
}

// startDisconnectSubscriber listens for cross-pod disconnect signals via Redis Pub/Sub.
// When another pod takes over a user's WebSocket connection, it publishes a disconnect
// message so this pod can close the stale connection and stop duplicate broadcasts.
func (a *App) startDisconnectSubscriber() {
	if a.wsRegistry == nil {
		return
	}

	msgCh, pubsub, err := a.wsRegistry.SubscribeDisconnects(a.ctx)
	if err != nil {
		a.log().Error("Failed to start disconnect subscriber", zap.Error(err))
		return
	}

	a.wg.Add(1)
	infra.SafeGo(a.log(), "cross-pod-disconnect-subscriber", func() {
		defer a.wg.Done()
		defer pubsub.Close()

		for msg := range msgCh {
			a.log().Info("Received cross-pod disconnect signal",
				zap.String("user_id", msg.UserID),
				zap.String("contest_id", msg.ContestID),
				zap.String("reason", msg.Reason))
			a.hub.DisconnectUser(msg.UserID, msg.ContestID)
		}
	})

	a.log().Info("Cross-pod disconnect subscriber started",
		zap.String("pod", a.config.PodName))
}

// runRegistryHeartbeat periodically refreshes TTLs for all registered connections
func (a *App) runRegistryHeartbeat(ctx context.Context) {
	if a.wsRegistry == nil {
		return
	}

	// Heartbeat interval should be less than TTL to ensure connections don't expire
	interval := a.config.WSRegistryTTL / 4
	if interval < time.Minute {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	a.log().Info("Registry heartbeat started",
		zap.Duration("interval", interval),
		zap.Duration("ttl", a.config.WSRegistryTTL))

	for {
		select {
		case <-ctx.Done():
			a.log().Info("Registry heartbeat stopping")
			return
		case <-ticker.C:
			connections := a.wsRegistry.GetAllMyConnections()
			if len(connections) == 0 {
				continue
			}

			// Build batch for heartbeat
			batch := make([]struct{ UserID, ContestID string }, len(connections))
			for i, conn := range connections {
				batch[i] = struct{ UserID, ContestID string }{conn.UserID, conn.ContestID}
			}

			heartbeatCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := a.wsRegistry.HeartbeatBatch(heartbeatCtx, batch); err != nil {
				a.log().Warn("Registry heartbeat failed", zap.Error(err))
			} else {
				a.log().Debug("Registry heartbeat completed",
					zap.Int("connections", len(connections)))
			}
			cancel()
		}
	}
}

// gracefulShutdownRegistry cleans up all registry entries for this pod
func (a *App) gracefulShutdownRegistry() {
	if a.wsRegistry == nil {
		return
	}

	// Send close message to all connected clients
	a.hub.clientsMu.RLock()
	clientCount := len(a.hub.clients)
	for client := range a.hub.clients {
		// Send a close message to client so they know to reconnect
		client.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"))
	}
	a.hub.clientsMu.RUnlock()

	a.log().Info("Sent close messages to WebSocket clients",
		zap.Int("client_count", clientCount))

	// Wait a bit for clients to receive the message
	time.Sleep(2 * time.Second)

	// Clean up registry entries
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.wsRegistry.CleanupAllMyConnections(cleanupCtx); err != nil {
		a.log().Warn("Failed to cleanup registry connections", zap.Error(err))
	} else {
		a.log().Info("Registry connections cleaned up")
	}
}

// ====================
// HTTP Handlers
// ====================

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	response := map[string]interface{}{
		"status":    "ready",
		"service":   "trade-bff",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	httpStatus := http.StatusOK

	// Check database connectivity (primary and replicas) - critical
	if err := a.pool.HealthCheck(ctx); err != nil {
		response["status"] = "unavailable"
		response["database"] = "unavailable"
		response["message"] = "database connectivity check failed"
		httpStatus = http.StatusServiceUnavailable
	} else {
		stats := a.pool.Stats()
		response["database"] = map[string]interface{}{
			"status":          "healthy",
			"primary_conns":   stats.Primary.OpenConnections,
			"replica_count":   len(stats.Replicas),
			"replication_lag": a.pool.GetReplicationLag(),
		}
	}

	// Check Redis connectivity - critical for WebSocket session registry
	if a.redis != nil {
		if err := a.redis.Ping(ctx).Err(); err != nil {
			response["redis"] = "unavailable"
			if response["status"] == "ready" {
				response["status"] = "degraded"
				response["message"] = "redis unavailable"
			}
		} else {
			response["redis"] = "healthy"
		}
	} else {
		response["redis"] = "not_configured"
	}

	// Check Kafka connectivity - critical for order publishing and event consumption
	kafkaStatus := map[string]interface{}{}
	kafkaHealthy := true

	// Check orders producer (critical - needed to submit orders)
	if a.ordersKafka != nil {
		kafkaStatus["orders_producer"] = "healthy"
	} else {
		kafkaStatus["orders_producer"] = "unavailable"
		kafkaHealthy = false
	}

	// Check consumers (needed for real-time updates)
	consumers := []struct {
		name   string
		client *kgo.Client
	}{
		{"ticks_consumer", a.ticksKafka},
		{"fills_consumer", a.fillsKafka},
		{"positions_consumer", a.positionsKafka},
		{"order_acks_consumer", a.orderAcksKafka},
	}

	for _, c := range consumers {
		if c.client != nil {
			kafkaStatus[c.name] = "healthy"
		} else {
			kafkaStatus[c.name] = "unavailable"
			kafkaHealthy = false
		}
	}

	if kafkaHealthy {
		response["kafka"] = kafkaStatus
	} else {
		response["kafka"] = kafkaStatus
		if response["status"] == "ready" {
			response["status"] = "degraded"
			if response["message"] == nil {
				response["message"] = "some kafka clients unavailable"
			}
		}
	}

	// Check circuit breaker health
	if a.circuits != nil {
		if !a.circuits.IsHealthy() {
			response["circuits"] = a.circuits.Status()
			if response["status"] == "ready" {
				response["status"] = "degraded"
			}
		} else {
			response["circuits"] = "healthy"
		}
	}

	// Add connection count
	response["ws_connections"] = a.metrics.wsConnections.Load()

	writeJSON(w, httpStatus, response)
}

func (a *App) handleWSStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.metrics.GetStats())
}

// handleWSTicket handles POST /api/trade/ws-ticket
// Exchanges a JWT (in Authorization header) for a short-lived, single-use WebSocket ticket.
// This avoids exposing JWTs in WebSocket URLs where they would be logged by nginx,
// browser history, proxies, and error reporting tools.
func (a *App) handleWSTicket(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ContestID string `json:"contest_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ContestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.ContestIDRequired})
		return
	}

	userID := auth.GetUserID(r.Context())
	claims := auth.GetClaims(r.Context())
	if userID == "" || claims == nil || claims.SessionID == "" || claims.AuthContext != auth.ContextUser {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": tradeMsg.AuthRequired})
		return
	}

	issue, err := a.wsTickets.Issue(r.Context(), userID, claims.SessionID, req.ContestID)
	if err != nil {
		if errors.Is(err, errWSTicketInvalid) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": tradeMsg.AuthRequired})
			return
		}
		a.log().Error("Failed to issue WebSocket ticket", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": tradeMsg.InternalError})
		return
	}

	secure := secureWSTicketCookie(a.config.Environment, r)
	setWSTicketBindingCookie(w, issue.Binding, secure, defaultWSTicketTTL)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket":     issue.Ticket,
		"expires_in": issue.ExpiresIn,
	})
}
func (a *App) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract contest_id from query params
	contestID := r.URL.Query().Get("contest_id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.ContestIDRequired})
		return
	}

	// Determine encoding type from query parameter (default: JSON for backward compatibility)
	// Clients can request MessagePack encoding via ?encoding=msgpack for bandwidth optimization
	encoding := EncodingJSON
	encodingParam := strings.ToLower(r.URL.Query().Get("encoding"))
	if encodingParam == "msgpack" || encodingParam == "messagepack" {
		encoding = EncodingMsgPack
	}

	// Reusable session credentials are forbidden in URLs. Browser clients use
	// the bounded ticket + HttpOnly binding cookie; non-browser clients may use
	// a context-specific Authorization header. Both paths revalidate the active
	// User session before the socket reaches contest authorization.
	ticketPresented := websocketTicketFromRequest(r) != ""
	userID, authErr := authenticateWebSocketRequest(r.Context(), r, contestID, a.wsTickets, a.wsAccess)
	if ticketPresented {
		clearWSTicketBindingCookie(w, secureWSTicketCookie(a.config.Environment, r))
	}
	if authErr != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": tradeMsg.AuthRequired})
		return
	}
	// Validate contest is running and within time window
	ctx := r.Context()
	if !a.validateContestRunning(w, ctx, contestID, true) {
		return
	}

	// Validate user is a participant in this contest
	var isParticipant bool
	err := a.pool.Replica().QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id = $1 AND user_id = $2)`,
		contestID, userID,
	).Scan(&isParticipant)
	if err != nil {
		a.log().Error("Failed to check contest participation for WebSocket",
			zap.String("contest_id", contestID),
			zap.String("user_id", userID),
			zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": tradeMsg.InternalError})
		return
	}
	if !isParticipant {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": tradeMsg.NotParticipant})
		return
	}

	// Get shard info for the contest
	shardInfo, err := a.shardMiddleware.GetShardInfo(ctx, contestID)
	if err != nil {
		a.log().Warn("Failed to get shard info for WebSocket connection",
			zap.String("contest_id", contestID),
			zap.String("user_id", userID),
			zap.Error(err))
		// Continue without shard info - connection will use default routing
	}

	// Log shard routing information
	if shardInfo != nil {
		a.log().Info("WebSocket connection routed to shard",
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.String("shard_id", shardInfo.ShardID),
			zap.String("shard_address", shardInfo.Address))
	}

	// Check session affinity registry for existing connection
	if a.wsRegistry != nil {
		if !a.wsRegistry.IsOwnedByMe(ctx, userID, contestID) {
			owner, err := a.wsRegistry.GetOwner(ctx, userID, contestID)
			if err == nil && owner != "" {
				// User has an existing connection on another pod
				// Set header to hint which pod to redirect to (for debugging)
				w.Header().Set("X-WS-Affinity-Pod", owner)
				a.log().Info("Connection exists on another pod, allowing takeover",
					zap.String("user_id", userID),
					zap.String("contest_id", contestID),
					zap.String("existing_pod", owner),
					zap.String("this_pod", a.config.PodName))
				// Note: We allow the connection to proceed with takeover
				// This enables reconnection when previous pod is unavailable
			}
		}
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.log().Error("WebSocket upgrade failed", zap.Error(err))
		return
	}

	// Check if compression was successfully negotiated
	// gorilla/websocket negotiates permessage-deflate if both client and server support it
	compressionEnabled := a.config.EnableCompression

	// Configure per-connection compression settings
	if compressionEnabled {
		// Set compression level for this connection
		// flate.BestSpeed (1) for high concurrency (low CPU), flate.BestCompression (9) for maximum compression
		conn.SetCompressionLevel(a.config.CompressionLevel)
		// Enable write compression (can be toggled per-message in writePump)
		conn.EnableWriteCompression(true)
		a.metrics.wsCompressedConnections.Add(1)
		a.log().Debug("WebSocket connection with compression enabled",
			zap.String("user_id", userID),
			zap.Int("compression_level", a.config.CompressionLevel))
	} else {
		a.metrics.wsUncompressedConnections.Add(1)
	}

	// Create client with bounded send queue, compression settings, and encoding preference
	client := NewClient(a.hub, conn, userID, contestID, a.metrics, compressionEnabled, a.config.MinMessageSizeForCompression, a, encoding)
	client.isParticipant = true // Verified above via DB query

	// Track encoding type metrics
	if encoding == EncodingMsgPack {
		a.metrics.wsMsgPackConnections.Add(1)
		a.log().Debug("WebSocket connection with MessagePack encoding",
			zap.String("user_id", userID),
			zap.String("contest_id", contestID))
	} else {
		a.metrics.wsJsonConnections.Add(1)
	}

	// Store shard info in client for routing decisions
	if shardInfo != nil {
		client.shardID = shardInfo.ShardID
		client.shardAddress = shardInfo.Address
	}

	// Register client
	a.hub.register <- client

	// Register in session affinity registry and setup cleanup on disconnect
	if a.wsRegistry != nil {
		if err := a.wsRegistry.Register(ctx, userID, contestID); err != nil {
			a.log().Warn("Failed to register connection in affinity registry",
				zap.String("user_id", userID),
				zap.String("contest_id", contestID),
				zap.Error(err))
		} else {
			a.log().Debug("Connection registered in affinity registry",
				zap.String("user_id", userID),
				zap.String("contest_id", contestID),
				zap.String("pod", a.config.PodName))
		}

		// Setup cleanup goroutine that unregisters on disconnect
		infra.SafeGo(a.log(), "ws-unregister-cleanup", func() {
			<-client.done
			// Use background context since request context may be cancelled
			unregCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.wsRegistry.Unregister(unregCtx, userID, contestID); err != nil {
				a.log().Warn("Failed to unregister connection from affinity registry",
					zap.String("user_id", userID),
					zap.String("contest_id", contestID),
					zap.Error(err))
			}
		})
	}

	// Send welcome message with shard info
	welcomePayload := map[string]interface{}{
		"status": "CONNECTING",
	}
	if shardInfo != nil {
		welcomePayload["shard_id"] = shardInfo.ShardID
	}
	welcomeMsg := WSMessage{
		Type:    "contest_state",
		Phase:   "CONNECTING",
		Payload: welcomePayload,
	}
	welcomeData, _ := json.Marshal(welcomeMsg)
	client.SendMessage(welcomeData)

	// Start client goroutines with panic recovery
	infra.SafeGo(a.log(), "writePump", func() { client.writePump() })
	infra.SafeGo(a.log(), "readPump", func() { client.readPump() })
}

// handleShardInfo handles GET /api/trade/shard-info
// Returns shard routing information for a contest
func (a *App) handleShardInfo(w http.ResponseWriter, r *http.Request) {
	contestID := r.URL.Query().Get("contest_id")
	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": tradeMsg.ContestIDRequired})
		return
	}

	shardInfo, err := a.shardMiddleware.GetShardInfo(r.Context(), contestID)
	if err != nil {
		a.log().Warn("Failed to get shard info",
			zap.String("contest_id", contestID),
			zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": tradeMsg.ShardInfoUnavailable,
		})
		return
	}

	// Build discovery response
	response := shard.BuildDiscoveryResponse(shardInfo, a.wsEndpoint)
	writeJSON(w, http.StatusOK, response)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}
