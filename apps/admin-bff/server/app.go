package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/domain/statemachine"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/ratelimit"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
	"github.com/Parsaeffatravesh/tragge/packages/storage"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Config holds application configuration.
type Config struct {
	Port                 string
	PostgresDSN          string
	PostgresReplicaDSNs  []string
	AuthContext          auth.ContextConfig
	RedisAddr            string
	KafkaBrokers         []string
	InstanceID           string
	ResendAPIKey         string
	ResendFromEmail      string
	NotificationEnabled  bool
	FrontendBaseURL      string
	MarketIngestorURL    string // Base URL for market-ingestor control API
	MarketIngestorAPIKey string // API key for authenticating with market-ingestor

	// S3/MinIO storage configuration for KYC documents
	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	S3Region       string
	S3KYCBucket    string
	S3TicketBucket string
	S3UseSSL       bool

	// S3/MinIO storage configuration for predefined avatar uploads
	S3AvatarBucket    string
	S3AvatarPublicURL string

	// Admin auth security settings
	AdminIPWhitelist    []string      // Optional IP whitelist for admin login
	AdminAuthRateLimit  int           // Rate limit for admin auth endpoints (default: 3)
	AdminAuthRateWindow time.Duration // Rate limit window (default: 1 minute)

	AdminMFA auth.AdminMFAConfig
}

// App holds application dependencies.
type App struct {
	pool                    *db.Pool
	auth                    *auth.Auth
	config                  *Config
	redis                   *pkgredis.Client
	kafkaAdmin              sarama.ClusterAdmin
	kafkaProducer           sarama.SyncProducer
	kafkaMu                 sync.RWMutex
	kafkaOffsets            map[string]map[int32]int64 // topic -> partition -> offset
	obs                     *observability.Observability
	circuits                *CircuitBreakers
	notificationSvc         *notification.Service
	emailNotifier           *notification.EmailNotifier
	marketHours             *MarketHoursManager
	walletService           *wallet.Service
	stateMachine            *statemachine.StateMachine
	marketIngestorClient    *http.Client
	kycStorage              storage.ObjectStore      // S3/MinIO storage for KYC documents
	avatarStorage           storage.ObjectStore      // S3/MinIO storage for predefined avatar uploads
	failedAdminLoginTracker *failedAdminLoginTracker // Tracks failed admin login attempts
	distributedLoginLockout *ratelimit.LoginLockout
	reauthentication        *auth.ReauthenticationService
	mfaChallenges           *auth.RedisAdminMFAChallengeStore
	banExpirySweeper        *banExpirySweeper // Periodically unbans expired temporary bans
}

// log returns the observability logger.
func (a *App) log() *zap.Logger {
	return a.obs.Logger.Logger
}

// logAuditEvent logs an audit event to the audit_logs table.
func (a *App) logAuditEvent(ctx context.Context, actorUserID, action, targetType, targetID string, payload interface{}) {
	payloadJSON, _ := json.Marshal(payload)

	err := a.circuits.ExecuteDatabase(ctx, func(ctx context.Context) error {
		_, execErr := a.pool.Primary().ExecContext(ctx,
			`INSERT INTO audit_logs (actor_user_id, action, target_type, target_id, payload_json)
			 VALUES ($1, $2, $3, $4, $5)`,
			actorUserID, action, targetType, targetID, payloadJSON,
		)
		return execErr
	})
	if err != nil {
		a.log().Warn("Failed to write audit log",
			zap.String("action", action),
			zap.String("target_type", targetType),
			zap.String("target_id", targetID),
			zap.Error(err))
	}
}

// isCircuitError checks if the error is a circuit breaker error and writes the appropriate HTTP response.
func (a *App) isCircuitError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, ErrCircuitOpen) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": adminMsg.ServiceUnavailable})
		return true
	}
	if errors.Is(err, ErrCircuitTimeout) {
		writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": adminMsg.RequestTimeout})
		return true
	}
	return false
}

// Run starts the admin-bff service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil, nil)
}

// RunWithSharedDeps starts the admin-bff service, optionally using shared resources.
// When parentCtx is non-nil, the service shuts down when the context is cancelled
// instead of registering its own signal handler. When sharedPool/sharedRedis/sharedAuth
// are non-nil, the service uses those instead of creating its own.
func RunWithSharedDeps(parentCtx context.Context, sharedPool *db.Pool, sharedRedis *pkgredis.Client, sharedAuth *auth.Auth) {
	// Validate critical environment variables in production/staging
	if sharedPool == nil {
		config.MustBeSetAny("database connection", "POSTGRES_DSN", "POSTGRES_HOST")
	}
	config.MustBeSet("KAFKA_BROKERS")
	if sharedRedis == nil && sharedAuth == nil {
		config.MustBeSet("REDIS_ADDR")
	}

	cfg := loadConfig()
	edgeEnvironment, edgeErr := validation.LoadAndValidateEdgeEnvironment(os.Getenv)
	if edgeErr != nil {
		log.Fatal("invalid edge security configuration")
	}

	// Initialize observability
	ctx := context.Background()
	obs, err := observability.New(ctx, observability.Config{
		Service:              "admin-bff",
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

	log := obs.Logger

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

	// Initialize circuit breakers
	circuitCfg := DefaultCircuitBreakerConfig()
	circuitCfg.Logger = log.Logger
	circuitCfg.OnStateChange = func(name string, from, to circuitbreaker.State) {
		log.Warn("Circuit breaker state changed",
			zap.String("circuit", name),
			zap.String("from", from.String()),
			zap.String("to", to.String()))
	}
	circuits := NewCircuitBreakers(circuitCfg)

	// Connect to PostgreSQL using connection pool with primary/replica support
	var pool *db.Pool
	if sharedPool != nil {
		pool = sharedPool
		log.Info("Using shared database pool")
	} else {
		// Pool settings configurable via env for production tuning (default: 10/5 per pod)
		dbMaxOpen := 10
		if v := os.Getenv("DB_MAX_OPEN_CONNS"); v != "" {
			if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
				dbMaxOpen = n
			}
		}
		dbMaxIdle := 5
		if v := os.Getenv("DB_MAX_IDLE_CONNS"); v != "" {
			if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
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
	dbPoolMetrics := observability.NewDBPoolMetrics(obs.Metrics.Registry(), "admin_bff")
	dbPoolMetrics.AddDB(pool.Primary())
	dbPoolMetrics.Start(10 * time.Second)
	defer dbPoolMetrics.Stop()

	// Connect to Redis with HA support
	var rdb *pkgredis.Client
	if sharedRedis != nil {
		rdb = sharedRedis
		log.Info("Using shared Redis client")
	} else {
		redisCfg := pkgredis.ConfigFromEnv(os.Getenv)
		if redisCfg.Addr == "" && redisCfg.Mode == pkgredis.ModeStandalone {
			redisCfg.Addr = cfg.RedisAddr
		}
		var redisErr error
		rdb, redisErr = pkgredis.NewClient(redisCfg)
		if redisErr != nil {
			log.Warn("Failed to create Redis client", zap.Error(redisErr))
			rdb = nil
		} else if pingErr := rdb.Ping(ctx).Err(); pingErr != nil {
			log.Warn("Failed to connect to Redis, heartbeat features disabled", zap.Error(pingErr))
			rdb.Close()
			rdb = nil
		} else {
			log.Info("Connected to Redis",
				zap.String("mode", string(rdb.Mode())))
		}
	}

	// Connect to Kafka (best-effort, don't fail if unavailable)
	var kafkaAdmin sarama.ClusterAdmin
	if len(cfg.KafkaBrokers) > 0 {
		saramaConfig := sarama.NewConfig()
		saramaConfig.Version = sarama.V2_8_0_0
		kafkaAdmin, err = sarama.NewClusterAdmin(cfg.KafkaBrokers, saramaConfig)
		if err != nil {
			log.Warn("Failed to connect to Kafka, shard info disabled", zap.Error(err))
			kafkaAdmin = nil
		} else {
			log.Info("Connected to Kafka cluster")
		}
	}

	// Create Kafka producer for publishing contest state events
	var kafkaProducer sarama.SyncProducer
	if len(cfg.KafkaBrokers) > 0 {
		producerConfig := sarama.NewConfig()
		producerConfig.Producer.RequiredAcks = sarama.WaitForAll
		producerConfig.Producer.Retry.Max = 5
		producerConfig.Producer.Return.Successes = true
		kafkaProducer, err = sarama.NewSyncProducer(cfg.KafkaBrokers, producerConfig)
		if err != nil {
			log.Warn("Failed to create Kafka producer, contest state events will not be published", zap.Error(err))
			kafkaProducer = nil
		} else {
			log.Info("Kafka producer initialized for contest state events")
		}
	}

	// Initialize auth
	var authService *auth.Auth
	if sharedAuth != nil {
		if sharedAuth.Context() != auth.ContextAdmin {
			log.Fatal("Refusing non-Admin authentication context")
		}
		authService = sharedAuth
		log.Info("Using isolated Admin authentication context")
	} else {
		if err := cfg.AuthContext.Validate(os.Getenv("ENVIRONMENT")); err != nil {
			log.Fatal("Invalid Admin authentication configuration", zap.Error(err))
		}
		var redisUniversal redis.UniversalClient
		if rdb != nil {
			redisUniversal = rdb.UniversalClient
		}
		var authErr error
		authService, authErr = auth.NewContext(cfg.AuthContext, redisUniversal)
		if authErr != nil {
			log.Fatal("Failed to construct Admin authentication context", zap.Error(authErr))
		}
	}

	// Initialize notification service for KYC emails
	var notifSvc *notification.Service
	var emailNotifier *notification.EmailNotifier
	if cfg.ResendAPIKey != "" && cfg.NotificationEnabled {
		notifConfig := notification.ServiceConfig{
			Email: notification.EmailConfig{
				APIKey:    cfg.ResendAPIKey,
				FromEmail: cfg.ResendFromEmail,
				Enabled:   true,
			},
			Enabled:        true,
			AsyncEnabled:   true,
			AsyncWorkers:   5,
			AsyncQueueSize: 100,
			Environment:    os.Getenv("ENVIRONMENT"),
			ServiceName:    "admin-bff",
		}
		var err error
		notifSvc, err = notification.NewService(ctx, notifConfig, log.Logger, prometheus.DefaultRegisterer)
		if err != nil {
			log.Warn("Failed to create notification service, KYC emails disabled", zap.Error(err))
		} else {
			// Create email notifier for direct KYC emails
			emailNotifier, err = notification.NewEmailNotifier(notifConfig.Email, log.Logger)
			if err != nil {
				log.Warn("Failed to create email notifier, KYC emails disabled", zap.Error(err))
			} else {
				log.Info("Notification service initialized for KYC emails")
			}
		}
	} else {
		log.Info("Notification service disabled (RESEND_API_KEY not set or notifications disabled)")
	}

	// Initialize wallet service
	walletSvc := wallet.NewService(pool.Primary())

	// Sensitive-action grants are ephemeral and fail closed when Redis is unavailable.
	var reauthentication *auth.ReauthenticationService
	var mfaChallenges *auth.RedisAdminMFAChallengeStore
	if rdb != nil {
		grantStore := auth.NewRedisReauthenticationGrantStore(rdb.Client(), auth.AdminReauthenticationPrefix)
		reauthentication, err = auth.NewReauthenticationService(grantStore, auth.MaxReauthenticationTTL)
		if err != nil {
			log.Error("Failed to initialize Admin reauthentication", zap.Error(err))
			return
		}
		mfaChallenges = auth.NewRedisAdminMFAChallengeStore(rdb.Client(), auth.AdminMFAChallengePrefix)
	}

	// Initialize object storage for KYC documents. storage.New auto-selects
	// the local-filesystem backend in dev (STORAGE_BACKEND=local) and S3/MinIO
	// in production. Private=true keeps S3 bucket policy private; the local
	// backend serves only via authenticated admin handlers.
	var kycStorage storage.ObjectStore
	kycObjectStore, err := storage.New(ctx, storage.Config{
		Endpoint:        cfg.S3Endpoint,
		AccessKeyID:     cfg.S3AccessKey,
		SecretAccessKey: cfg.S3SecretKey,
		Region:          cfg.S3Region,
		Bucket:          cfg.S3KYCBucket,
		UseSSL:          cfg.S3UseSSL,
		Private:         true,
	})
	if err != nil {
		log.Warn("Failed to initialize KYC storage, KYC document serving will be unavailable", zap.Error(err))
	} else {
		kycStorage = kycObjectStore
		log.Info("KYC storage initialized",
			zap.String("backend", storage.BackendName(kycObjectStore)),
			zap.String("bucket", cfg.S3KYCBucket))
	}

	// Initialize object storage for predefined avatar uploads (public bucket).
	var avatarStorage storage.ObjectStore
	avatarObjectStore, err := storage.New(ctx, storage.Config{
		Endpoint:        cfg.S3Endpoint,
		AccessKeyID:     cfg.S3AccessKey,
		SecretAccessKey: cfg.S3SecretKey,
		Region:          cfg.S3Region,
		Bucket:          cfg.S3AvatarBucket,
		UseSSL:          cfg.S3UseSSL,
		PublicURL:       cfg.S3AvatarPublicURL,
	})
	if err != nil {
		log.Warn("Failed to initialize avatar storage, predefined avatar uploads will be unavailable", zap.Error(err))
	} else {
		avatarStorage = avatarObjectStore
		log.Info("Avatar storage initialized",
			zap.String("backend", storage.BackendName(avatarObjectStore)),
			zap.String("bucket", cfg.S3AvatarBucket))
	}

	var distributedLoginLockout *ratelimit.LoginLockout
	if rdb != nil {
		distributedLoginLockout, err = ratelimit.NewLoginLockout(rdb.UniversalClient, ratelimit.LockoutConfig{
			Namespace: "admin", Threshold: 5, LockFor: 30 * time.Minute, Retention: 2 * time.Hour,
		})
		if err != nil {
			log.Fatal("invalid Admin login lockout configuration")
		}
	}

	app := &App{
		pool:                    pool,
		auth:                    authService,
		config:                  cfg,
		redis:                   rdb,
		kafkaAdmin:              kafkaAdmin,
		kafkaProducer:           kafkaProducer,
		kafkaOffsets:            make(map[string]map[int32]int64),
		obs:                     obs,
		circuits:                circuits,
		notificationSvc:         notifSvc,
		emailNotifier:           emailNotifier,
		walletService:           walletSvc,
		kycStorage:              kycStorage,
		avatarStorage:           avatarStorage,
		failedAdminLoginTracker: newFailedAdminLoginTracker(),
		distributedLoginLockout: distributedLoginLockout,
		reauthentication:        reauthentication,
		mfaChallenges:           mfaChallenges,
	}

	// Initialize ban expiry sweeper to auto-unban expired temporary bans
	app.banExpirySweeper = newBanExpirySweeper(pool.Primary(), obs.Logger.Logger)

	// Initialize shared HTTP client for market-ingestor proxy (no redirects, bounded timeout)
	app.marketIngestorClient = &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Initialize market hours manager
	marketHoursConfigPath := os.Getenv("MARKET_HOURS_CONFIG_PATH")
	if marketHoursConfigPath == "" {
		marketHoursConfigPath = "market_hours.json"
	}
	marketHours, err := NewMarketHoursManager(marketHoursConfigPath, log.Logger)
	if err != nil {
		log.Warn("Failed to initialize market hours manager - market hours endpoints will be unavailable",
			zap.Error(err),
			zap.String("config_path", marketHoursConfigPath))
	} else {
		app.marketHours = marketHours
		log.Info("Market hours manager initialized", zap.String("config_path", marketHoursConfigPath))
	}

	// Initialize the contest state machine (singleton, reused across requests)
	app.initStateMachine()

	// Shutdown notification service on exit
	if notifSvc != nil {
		defer notifSvc.Shutdown(context.Background())
	}

	// Close Kafka producer on exit
	if kafkaProducer != nil {
		defer kafkaProducer.Close()
	}

	// Start heartbeat ticker
	if app.redis != nil {
		go app.startHeartbeat(ctx)
	}

	// Setup router
	r := chi.NewRouter()

	// Create Sentry HTTP handler for panic recovery
	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic: true, // Re-panic after capture so sanitized recovery handles it
	})

	// Middleware stack (order matters)
	// middleware.RealIP removed: it trusts X-Forwarded-For from any source,
	// allowing IP spoofing. Use validation.ExtractClientIP with trusted proxy check instead.
	var edgeRedis redis.UniversalClient
	if rdb != nil {
		edgeRedis = rdb.UniversalClient
	}
	edgePolicy := ratelimit.NewPolicyMiddleware(edgeRedis, ratelimit.PoliciesForService("admin"), nil, func(class ratelimit.EndpointClass, reason string) {
		log.Warn("Edge security request denied", zap.String("policy_class", string(class)), zap.String("reason", reason))
	})
	r.Use(validation.RequestIDMiddleware)                             // Request ID tracking
	r.Use(validation.CORSMiddleware(validation.AdminBFFCORSConfig())) // CORS handling
	r.Use(validation.CSRFMiddleware(validation.AdminBFFCSRFConfig())) // Context-specific CSRF protection
	r.Use(validation.SecurityHeadersMiddleware)                       // Security headers
	r.Use(edgePolicy.Handler)                                         // Distributed edge abuse controls
	r.Use(auth.RedactSecurityCredentialsForTelemetry)                 // Hide session credentials from telemetry
	r.Use(obs.Middleware.Middleware)                                  // Observability (logging, tracing)
	r.Use(sentryHandler.Handle)                                       // Sentry panic capture
	r.Use(auth.RestoreSecurityCredentialsAfterTelemetry)              // Restore secure headers for auth handlers
	r.Use(obs.Middleware.Recovery)                                    // Sanitized panic recovery
	// Note: chi middleware.Timeout removed ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¦Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â its defer cancel() races with
	// pgx row scanning on Docker-proxied connections, causing "context canceled".
	// The http.Server WriteTimeout (15s) provides an equivalent safety net.
	r.Use(validation.MaxBytesMiddleware(edgeEnvironment.DefaultBodyBytes))
	r.Use(validation.ContentTypeMiddleware)
	r.Use(validation.SanitizeFormMiddleware) // Input sanitization

	// Support ticket routes ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¦Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â override body limit to 35MB because file uploads need up to 35MB overhead
	fileUploadBodyLimit := validation.MaxBytesMiddleware(edgeEnvironment.UploadBodyBytes)
	r.Route("/api/admin/tickets", func(r chi.Router) {
		r.Use(fileUploadBodyLimit)
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		r.Get("/", app.handleAdminListTickets)
		r.Get("/stats", app.handleAdminTicketStats)
		r.With(validation.ValidatePathUUID("attachmentId")).Get("/attachment/{attachmentId}", app.handleAdminGetTicketAttachment)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("id"))
			r.Get("/", app.handleAdminGetTicket)
			r.Post("/messages", app.handleAdminSendMessage)
			r.Put("/status", app.handleAdminUpdateStatus)
			r.Put("/assign", app.handleAdminAssignTicket)
			r.Put("/priority", app.handleAdminUpdatePriority)
		})
	})

	// Health check endpoints
	r.Get("/healthz", app.handleHealthz)
	r.Get("/api/admin/healthz", app.handleHealthz)
	r.Get("/readyz", app.handleReadyz)
	r.Get("/health/circuits", app.handleCircuitHealth)
	r.With(validation.InternalOnlyMiddleware).Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		obs.MetricsHandler().ServeHTTP(w, r)
	})

	// Admin auth endpoints - outside RequireAuth (login is unauthenticated)
	adminAuthLimiter := validation.NewRateLimiter(cfg.AdminAuthRateLimit, cfg.AdminAuthRateWindow, cfg.AdminAuthRateLimit)
	r.Route("/api/admin/auth", func(r chi.Router) {
		r.Use(validation.RateLimitMiddleware(adminAuthLimiter))
		r.Post("/login", app.handleAdminLogin)
		r.Post("/mfa/enrollment/start", app.handleAdminMFAEnrollmentStart)
		r.Post("/mfa/enrollment/verify", app.handleAdminMFAEnrollmentVerify)
		r.Post("/mfa/verify", app.handleAdminMFAVerify)
		// /refresh reads the refresh_token_admin cookie ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¦Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â no auth
		// header required. Stays outside RequireAuth so an expired
		// access token can still drive a refresh.
		r.Post("/refresh", app.handleAdminRefresh)
	})

	// API routes - all require canonical support_admin or super_admin access
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)
		r.Use(edgePolicy.ActorHandler)

		// Current user info and permissions
		r.Get("/me", app.handleAdminMe)
		r.Get("/me/permissions", app.handleGetMyPermissions)
		r.Post("/logout", app.handleAdminLogout)
		r.With(validation.RateLimitMiddleware(adminAuthLimiter)).Post("/reauthenticate", app.handleAdminReauthenticate)

		// Dashboard - aggregated metrics (requires at least stats.view)
		r.With(app.auth.Middleware.RequirePermission("stats.view")).Get("/dashboard", app.handleGetDashboard)

		// Contest management - permission-protected
		r.Route("/contests", func(r chi.Router) {
			// List/create operations (no {id} param)
			r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/", app.handleListContests)
			r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/templates", app.handleListContestTemplates)
			r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/templates/{key}", app.handleGetContestTemplate)
			r.With(app.auth.Middleware.RequirePermission("contests.create")).Post("/", app.handleCreateContest)
			r.With(app.auth.Middleware.RequirePermission("contests.create")).Post("/from-template", app.handleCreateContestFromTemplate)

			// Per-contest operations - validate {id} as UUID
			r.Route("/{id}", func(r chi.Router) {
				r.Use(validation.ValidatePathUUID("id"))

				// Read operations - require contests.view
				r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/", app.handleGetContest)
				r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/state", app.handleGetContestState)
				r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/status-history", app.handleGetContestStatusHistory)
				r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/symbols", app.handleGetContestSymbols)
				r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/location", app.handleGetContestLocation)

				// Create/edit operations - require contests.create
				r.With(app.auth.Middleware.RequirePermission("contests.create")).Patch("/", app.handleUpdateContest)
				r.With(app.auth.Middleware.RequirePermission("contests.create")).Delete("/", app.handleDeleteContest)
				r.With(app.auth.Middleware.RequirePermission("contests.create")).Post("/symbols", app.handleAddContestSymbols)

				// Participant management - require contests.manage
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Delete("/participants/{user_id}", app.handleRemoveContestParticipant)

				// Lifecycle operations - require contests.manage
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Post("/freeze", app.handlePauseContest) // Freeze reuses pause handler (state machine validated)
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Post("/publish", app.handlePublishContest)
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Post("/start", app.handleStartContest)
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Post("/end", app.handleEndContest)
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Post("/cancel", app.handleCancelContest)
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Post("/pause", app.handlePauseContest)
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Post("/resume", app.handleResumeContest)
				r.With(app.auth.Middleware.RequirePermission("contests.manage")).Post("/close-registration", app.handleCloseRegistration)
			})
		})

		// Health and shards - permission-protected
		r.Route("/health/shards", func(r chi.Router) {
			r.With(app.auth.Middleware.RequirePermission("shards.view")).Get("/", app.handleGetShards)
			r.With(app.auth.Middleware.RequirePermission("shards.view")).Get("/stats", app.handleGetShardStats)
			r.With(app.auth.Middleware.RequirePermission("shards.view")).Get("/{shardId}", app.handleGetShard)
			r.With(app.auth.Middleware.RequirePermission("shards.manage")).Post("/{shardId}/drain", app.handleDrainShard)
			r.With(app.auth.Middleware.RequirePermission("shards.manage")).Post("/{shardId}/activate", app.handleActivateShard)
		})

		// Audit logs - require audit.view
		r.With(app.auth.Middleware.RequirePermission("audit.view")).Get("/audit", app.handleListAuditLogs)

		// User management - permission-protected
		r.Route("/users", func(r chi.Router) {
			r.With(app.auth.Middleware.RequirePermission("users.view")).Get("/", app.handleListUsers)
			r.With(app.auth.Middleware.RequirePermission("users.edit")).Post("/", app.handleAdminCreateUser)

			// Per-user operations - validate {user_id} as UUID
			r.Route("/{user_id}", func(r chi.Router) {
				r.Use(validation.ValidatePathUUID("user_id"))

				// Read operations
				r.With(app.auth.Middleware.RequirePermission("users.view")).Get("/", app.handleGetUser)
				r.With(app.auth.Middleware.RequirePermission("users.view")).Get("/wallet/history", app.handleGetUserWalletHistory)

				// Edit operations
				r.With(app.auth.Middleware.RequirePermission("users.edit"), app.requireSensitiveAction(actionUserRolesUpdate, "users.edit", "user_id")).Patch("/roles", app.handleUpdateUserRoles)
				r.With(app.auth.Middleware.RequirePermission("users.edit"), app.requireSensitiveAction(actionUserRolesUpdate, "users.edit", "user_id")).Put("/roles", app.handleUpdateUserRoles)
				r.With(app.auth.Middleware.RequirePermission("users.edit")).Patch("/status", app.handleUpdateUserStatus)

				// Ban/Unban operations
				r.With(app.auth.Middleware.RequirePermission("users.edit")).Post("/ban", app.handleBanUser)
				r.With(app.auth.Middleware.RequirePermission("users.edit")).Post("/unban", app.handleUnbanUser)

				// Session management
				r.With(app.auth.Middleware.RequirePermission("users.edit")).Post("/sessions/terminate", app.handleTerminateUserSessions)
				r.With(app.auth.Middleware.RequireSuperAdmin, app.auth.Middleware.RequirePermission("users.edit"), app.requireSensitiveAction(actionAdminMFAReset, "users.edit", "user_id")).Post("/mfa/reset", app.handleAdminMFAReset)

				// Wallet charge - super_admin only
				r.With(app.auth.Middleware.RequirePermission("users.wallet.charge"), app.requireSensitiveAction(actionWalletAdjust, "users.wallet.charge", "user_id")).Post("/wallet/charge", app.handleChargeUserWallet)
			})
		})

		r.With(app.auth.Middleware.RequirePermission("market.view")).Get("/providerconfig", func(w http.ResponseWriter, r *http.Request) {
			app.handleGetProviderConfig(w, r)
		})

		// Market routes ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Â ÃƒÂ¢Ã¢â€šÂ¬Ã¢â€žÂ¢ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã¢â‚¬Â¦Ãƒâ€šÃ‚Â¡ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™Ãƒâ€ Ã¢â‚¬â„¢ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã†â€™Ãƒâ€šÃ‚Â¢ÃƒÆ’Ã‚Â¢ÃƒÂ¢Ã¢â‚¬Å¡Ã‚Â¬Ãƒâ€¦Ã‚Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â¬ÃƒÆ’Ã†â€™ÃƒÂ¢Ã¢â€šÂ¬Ã…Â¡ÃƒÆ’Ã¢â‚¬Å¡Ãƒâ€šÃ‚Â all grouped under /market to avoid chi trie prefix collision
		r.Route("/market", func(r chi.Router) {
			// Market data
			r.With(app.auth.Middleware.RequirePermission("market.view")).Get("/data/status", app.handleGetMarketStatus2)
			r.With(app.auth.Middleware.RequirePermission("market.view")).Get("/data/prices", app.handleGetMarketPrices)
			r.With(app.auth.Middleware.RequirePermission("market.view")).Get("/data/provider-cfg", app.handleGetProviderConfig)
			r.With(app.auth.Middleware.RequirePermission("market.manage")).Post("/data/switch-provider", app.handleSwitchMarketProvider)
			r.With(app.auth.Middleware.RequirePermission("market.manage")).Post("/data/reconnect", app.handleMarketReconnect)
			r.With(app.auth.Middleware.RequirePermission("market.manage")).Post("/data/crypto-provider", app.handleSwitchCryptoProvider)
			r.With(app.auth.Middleware.RequirePermission("market.manage")).Post("/data/forex-provider", app.handleSwitchForexProvider)

			// Market hours
			r.With(app.auth.Middleware.RequirePermission("market.view")).Get("/hours/status", app.handleGetMarketStatus)
			r.With(app.auth.Middleware.RequirePermission("market.view")).Get("/hours/config", app.handleGetMarketHoursConfig)
			r.With(app.auth.Middleware.RequirePermission("market.view")).Get("/hours/overrides", app.handleGetMarketOverrides)
			r.With(app.auth.Middleware.RequirePermission("market.view")).Post("/hours/validate", app.handleValidateContestTimes)
			r.With(app.auth.Middleware.RequirePermission("settings.manage")).Post("/hours/override", app.handleSetMarketOverride)
			r.With(app.auth.Middleware.RequirePermission("settings.manage")).Delete("/hours/override/{asset_class}", app.handleClearMarketOverride)
		})
	})

	// KYC review routes - permission-protected
	r.Route("/api/admin/kyc", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// Read operations - require kyc.view
		r.With(app.auth.Middleware.RequirePermission("kyc.view")).Get("/", app.handleListKYCSubmissions)
		r.With(app.auth.Middleware.RequirePermission("kyc.view")).Get("/pending", app.handleListPendingKYC)
		r.With(app.auth.Middleware.RequirePermission("kyc.view")).Get("/documents/{document_id}/image/{type}", app.handleGetKYCDocumentImage)

		// Review operations - require kyc.review
		r.With(app.auth.Middleware.RequirePermission("kyc.review")).Post("/bulk-auto-approve", app.handleBulkAutoApproveKYC)

		// Per-user KYC operations - validate {user_id} as UUID
		r.Route("/{user_id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("user_id"))
			r.With(app.auth.Middleware.RequirePermission("kyc.view")).Get("/", app.handleGetKYCDetails)
			r.With(app.auth.Middleware.RequirePermission("kyc.review")).Post("/approve", app.handleApproveKYC)
			r.With(app.auth.Middleware.RequirePermission("kyc.review")).Post("/reject", app.handleRejectKYC)
			r.With(app.auth.Middleware.RequirePermission("kyc.review")).Post("/request-info", app.handleRequestKYCInfo)
		})
	})

	// Affiliate management routes - permission-protected
	r.Route("/api/admin/affiliate", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// All affiliate operations require affiliate.manage permission
		r.Use(app.auth.Middleware.RequirePermission("affiliate.manage"))

		// List pending activation requests
		r.Get("/pending", app.handleListPendingAffiliateRequests)

		// Approve/reject activation - validate {user_id} as UUID
		r.Route("/{user_id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("user_id"))
			r.Post("/approve", app.handleApproveAffiliateActivation)
			r.Post("/reject", app.handleRejectAffiliateActivation)
		})
	})

	// Withdrawal management routes - permission-protected
	r.Route("/api/admin/withdrawals", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// Read operations - require withdrawals.view
		r.With(app.auth.Middleware.RequirePermission("withdrawals.view")).Get("/", app.handleListWithdrawals)
		r.With(app.auth.Middleware.RequirePermission("withdrawals.view")).Get("/pending-count", app.handleGetPendingWithdrawalsCount)

		// Per-withdrawal operations - validate {id} as UUID
		r.Route("/{id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("id"))
			r.With(app.auth.Middleware.RequirePermission("withdrawals.view")).Get("/", app.handleGetWithdrawal)
			r.With(app.auth.Middleware.RequirePermission("withdrawals.view")).Post("/comment", app.handleAddWithdrawalComment)
			r.With(app.auth.Middleware.RequirePermission("withdrawals.manage")).Post("/approve", app.handleApproveWithdrawal)
			r.With(app.auth.Middleware.RequirePermission("withdrawals.manage")).Post("/reject", app.handleRejectWithdrawal)
			r.With(app.auth.Middleware.RequirePermission("withdrawals.manage"), app.requireSensitiveAction(actionWithdrawalComplete, "withdrawals.manage", "id")).Post("/complete", app.handleCompleteWithdrawal)
			r.With(app.auth.Middleware.RequirePermission("withdrawals.manage")).Post("/fail", app.handleFailWithdrawal)
		})
	})

	// Financial reports routes - permission-protected
	r.Route("/api/admin/financial", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// All financial operations require financial.view permission
		r.Use(app.auth.Middleware.RequirePermission("financial.view"))

		r.Get("/summary", app.handleGetFinancialSummary)
		r.Get("/deposits", app.handleListDeposits)
		r.Get("/transactions", app.handleListTransactions)
	})

	// Symbol management routes - permission-protected
	r.Route("/api/admin/symbols", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// Read operations - require symbols.view permission
		r.With(app.auth.Middleware.RequirePermission("symbols.view")).Get("/", app.handleListSymbols)
		r.With(app.auth.Middleware.RequirePermission("symbols.view")).Get("/{symbol}", app.handleGetSymbol)

		// Write operations - require symbols.manage permission
		r.With(app.auth.Middleware.RequirePermission("symbols.manage")).Post("/", app.handleCreateSymbol)
		r.With(app.auth.Middleware.RequirePermission("symbols.manage")).Put("/{symbol}", app.handleUpdateSymbol)
	})

	// Email template management routes - permission-protected
	r.Route("/api/admin/email-templates", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// All email template operations require settings.manage permission
		r.Use(app.auth.Middleware.RequirePermission("settings.manage"))

		// List all templates
		r.Get("/", app.handleListEmailTemplates)

		// Get template details
		r.Get("/{slug}", app.handleGetEmailTemplate)

		// Update template
		r.Put("/{slug}", app.handleUpdateEmailTemplate)

		// Reset template to default
		r.Post("/{slug}/reset", app.handleResetEmailTemplate)

		// Preview template
		r.Post("/{slug}/preview", app.handlePreviewEmailTemplate)

		// Template versions
		r.Get("/{slug}/versions", app.handleListTemplateVersions)
		r.Post("/{slug}/versions", app.handleCreateTemplateVersion)
		r.Get("/{slug}/versions/{versionId}", app.handleGetTemplateVersion)
		r.Put("/{slug}/versions/{versionId}", app.handleUpdateTemplateVersion)
		r.Delete("/{slug}/versions/{versionId}", app.handleDeleteTemplateVersion)
		r.Post("/{slug}/versions/{versionId}/activate", app.handleActivateTemplateVersion)
		r.Post("/{slug}/versions/{versionId}/preview", app.handlePreviewTemplateVersion)
	})

	// Calendar management routes - permission-protected
	r.Route("/api/admin/calendar", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// All calendar operations require contests.manage permission
		r.Use(app.auth.Middleware.RequirePermission("contests.manage"))

		// Create calendar entry
		r.Post("/", app.handleCreateCalendarEntry)

		// List calendar entries
		r.Get("/", app.handleListCalendarEntries)

		// Per-entry routes with UUID validation
		r.Route("/{id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("id"))
			r.Get("/", app.handleGetCalendarEntry)
			r.Put("/", app.handleUpdateCalendarEntry)
			r.Delete("/", app.handleDeleteCalendarEntry)
			r.Post("/preview", app.handlePreviewCalendarEntry)
		})
	})

	// Spread configuration routes - permission-protected
	r.Route("/api/admin/spreads", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// All spread operations require settings.manage permission
		r.Use(app.auth.Middleware.RequirePermission("settings.manage"))

		// Get all spreads configuration
		r.Get("/", app.handleGetSpreads)

		// Update default spread
		r.Put("/defaults", app.handleUpdateDefaultSpread)

		// Bulk update spreads
		r.Put("/bulk", app.handleBulkUpdateSpreads)

		// Asset class spread management
		r.Put("/asset-class/{class}", app.handleUpdateAssetClassSpread)

		// Symbol-specific spread management
		r.Get("/{symbol}", app.handleGetSymbolSpread)
		r.Put("/{symbol}", app.handleUpdateSymbolSpread)
		r.Delete("/{symbol}", app.handleDeleteSymbolSpread)
	})

	// Auto-scheduling configuration routes - permission-protected
	r.Route("/api/admin/auto-scheduling", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// Read-only config and upcoming - require contests.view
		r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/config", app.handleGetAutoSchedulingConfig)
		r.With(app.auth.Middleware.RequirePermission("contests.view")).Get("/upcoming", app.handleGetAutoSchedulingUpcoming)
	})

	// Tournament template management routes - permission-protected
	r.Route("/api/admin/templates", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// Read operations - require templates.view
		r.With(app.auth.Middleware.RequirePermission("templates.view")).Get("/", app.handleListTemplates)

		// Write operations - require templates.manage
		r.With(app.auth.Middleware.RequirePermission("templates.manage")).Post("/", app.handleCreateTemplate)

		// Per-template routes with UUID validation
		r.Route("/{id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("id"))
			r.With(app.auth.Middleware.RequirePermission("templates.view")).Get("/", app.handleGetTemplate)
			r.With(app.auth.Middleware.RequirePermission("templates.manage")).Put("/", app.handleUpdateTemplate)
			r.With(app.auth.Middleware.RequirePermission("templates.manage")).Delete("/", app.handleDeleteTemplate)
		})
	})

	// Entry tier management routes - nested under templates
	r.Route("/api/admin/templates/{templateID}/tiers", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)
		r.Use(validation.ValidatePathUUID("templateID"))

		r.With(app.auth.Middleware.RequirePermission("templates.view")).Get("/", app.handleListTiers)
		r.With(app.auth.Middleware.RequirePermission("templates.manage")).Post("/", app.handleCreateTier)
		r.With(app.auth.Middleware.RequirePermission("templates.manage")).Post("/bulk", app.handleBulkCreateTiers)
	})
	r.Route("/api/admin/tiers/{tierID}", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)
		r.Use(validation.ValidatePathUUID("tierID"))

		r.With(app.auth.Middleware.RequirePermission("templates.manage")).Put("/", app.handleUpdateTier)
		r.With(app.auth.Middleware.RequirePermission("templates.manage")).Delete("/", app.handleDeleteTier)
	})

	// Tournament schedule management routes - permission-protected
	r.Route("/api/admin/schedules", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		// Read operations - require schedules.view
		r.With(app.auth.Middleware.RequirePermission("schedules.view")).Get("/", app.handleListSchedules)

		// Write operations - require schedules.manage
		r.With(app.auth.Middleware.RequirePermission("schedules.manage")).Post("/", app.handleCreateSchedule)

		// Per-schedule routes with UUID validation
		r.Route("/{id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("id"))
			r.With(app.auth.Middleware.RequirePermission("schedules.manage")).Put("/", app.handleUpdateSchedule)
			r.With(app.auth.Middleware.RequirePermission("schedules.manage")).Delete("/", app.handleDeleteSchedule)
			r.With(app.auth.Middleware.RequirePermission("schedules.manage")).Post("/pause", app.handlePauseSchedule)
			r.With(app.auth.Middleware.RequirePermission("schedules.manage")).Post("/resume", app.handleResumeSchedule)
		})
	})

	// Avatar management routes - permission-protected
	r.Route("/api/admin/avatars", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		r.With(app.auth.Middleware.RequirePermission("settings.manage")).Get("/", app.handleListAdminAvatars)
		r.With(app.auth.Middleware.RequirePermission("settings.manage")).Post("/", app.handleCreateAdminAvatar)
		r.With(app.auth.Middleware.RequirePermission("settings.manage")).Post("/reorder", app.handleReorderAvatars)

		// Per-avatar routes with UUID validation
		r.Route("/{id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("id"))
			r.With(app.auth.Middleware.RequirePermission("settings.manage")).Put("/", app.handleUpdateAdminAvatar)
			r.With(app.auth.Middleware.RequirePermission("settings.manage")).Post("/image", app.handleReplaceAvatarImage)
			r.With(app.auth.Middleware.RequirePermission("settings.manage")).Delete("/", app.handleDeleteAdminAvatar)
		})
	})

	// Special tournament creation - permission-protected
	r.Route("/api/admin/tournaments", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		r.With(app.auth.Middleware.RequirePermission("contests.create")).Post("/special", app.handleCreateSpecialTournament)
	})

	// Tournament statistics routes - permission-protected
	r.Route("/api/admin/stats/tournaments", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)

		r.With(app.auth.Middleware.RequirePermission("stats.view")).Get("/", app.handleGetTournamentOverviewStats)
		r.Route("/{id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("id"))
			r.With(app.auth.Middleware.RequirePermission("stats.view")).Get("/", app.handleGetTournamentDetailStats)
		})
	})

	// Support ticket routes moved above global body limit middleware (see line ~490)

	// Create server
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    edgeEnvironment.MaxHeaderBytes,
	}

	// Start server in goroutine
	infra.SafeGo(log.Logger, "admin-bff-server", func() {
		log.Info("Starting admin-bff", zap.String("port", cfg.Port))
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
	log.Info("Shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", zap.Error(err))
	}

	// Close rate limiter to stop cleanup goroutine
	adminAuthLimiter.Close()

	// Stop failed login tracker cleanup goroutine
	app.failedAdminLoginTracker.stop()

	// Stop ban expiry sweeper goroutine
	app.banExpirySweeper.stop()

	log.Info("Server exited")
}

func loadConfig() *Config {
	port := os.Getenv("ADMIN_BFF_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8083"
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

	// Load only the explicit Admin trust-domain configuration. No generic JWT
	// secret or cross-context refresh fallback is accepted.
	authIsolation := auth.LoadIsolationConfig(os.Getenv("ENVIRONMENT"), os.Getenv, secrets.Load)
	adminAuthContext := authIsolation.Admin

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		if config.IsProduction() {
			log.Fatal("FATAL: REDIS_ADDR must be set in production")
		}
		redisAddr = "localhost:6379"
		log.Println("WARNING: REDIS_ADDR not set, using localhost:6379")
	}

	kafkaBrokersStr := os.Getenv("KAFKA_BROKERS")
	var kafkaBrokers []string
	if kafkaBrokersStr != "" {
		kafkaBrokers = strings.Split(kafkaBrokersStr, ",")
	} else {
		if config.IsProduction() {
			log.Fatal("FATAL: KAFKA_BROKERS must be set in production")
		}
		kafkaBrokers = []string{"localhost:9092"}
		log.Println("WARNING: KAFKA_BROKERS not set, using localhost:9092")
	}

	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		hostname, _ := os.Hostname()
		instanceID = fmt.Sprintf("admin-bff-%s-%d", hostname, os.Getpid())
	}

	// S3/MinIO storage configuration for KYC documents
	s3Endpoint := os.Getenv("S3_ENDPOINT")
	if s3Endpoint == "" {
		s3Endpoint = "localhost:9000"
	}
	s3AccessKey := secrets.Load("S3_ACCESS_KEY")
	s3SecretKey := secrets.Load("S3_SECRET_KEY")
	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		s3Region = "us-east-1"
	}
	s3KYCBucket := os.Getenv("S3_KYC_BUCKET")
	if s3KYCBucket == "" {
		s3KYCBucket = "tragge-kyc"
	}
	s3TicketBucket := os.Getenv("S3_TICKET_BUCKET")
	if s3TicketBucket == "" {
		s3TicketBucket = "tragge-tickets"
	}
	s3UseSSL := os.Getenv("S3_USE_SSL") == "true"

	s3AvatarBucket := os.Getenv("S3_BUCKET")
	if s3AvatarBucket == "" {
		s3AvatarBucket = "tragge-avatars"
	}
	s3AvatarPublicURL := os.Getenv("S3_PUBLIC_URL")

	// Notification settings
	resendAPIKey := os.Getenv("RESEND_API_KEY")
	resendFromEmail := os.Getenv("RESEND_FROM_EMAIL")
	if resendFromEmail == "" {
		resendFromEmail = "noreply@tragge.com"
	}
	notificationEnabled := os.Getenv("NOTIFICATION_ENABLED") != "false"

	frontendBaseURL := os.Getenv("FRONTEND_BASE_URL")
	if frontendBaseURL == "" {
		frontendBaseURL = "https://tragge.com"
	}

	marketIngestorURL := os.Getenv("MARKET_INGESTOR_URL")
	if marketIngestorURL == "" {
		marketIngestorURL = "http://localhost:8084"
	}

	// Validate market ingestor URL is a parseable URL with http/https scheme
	if parsedURL, parseErr := url.Parse(marketIngestorURL); parseErr != nil {
		log.Fatalf("FATAL: MARKET_INGESTOR_URL is not a valid URL: %v", parseErr)
	} else if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		log.Fatalf("FATAL: MARKET_INGESTOR_URL must use http or https scheme, got: %s", parsedURL.Scheme)
	}

	marketIngestorAPIKey := os.Getenv("MARKET_INGESTOR_API_KEY")

	// Admin auth security settings
	var adminIPWhitelist []string
	if whitelist := os.Getenv("ADMIN_IP_WHITELIST"); whitelist != "" {
		for _, ip := range strings.Split(whitelist, ",") {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				adminIPWhitelist = append(adminIPWhitelist, ip)
			}
		}
	}

	adminAuthRateLimit := 3
	if v := os.Getenv("ADMIN_AUTH_RATE_LIMIT"); v != "" {
		if parsed, parseErr := strconv.Atoi(v); parseErr == nil && parsed > 0 {
			adminAuthRateLimit = parsed
		}
	}

	adminAuthRateWindow := 5 * time.Minute
	if v := os.Getenv("ADMIN_AUTH_RATE_WINDOW"); v != "" {
		if parsed, parseErr := time.ParseDuration(v); parseErr == nil && parsed > 0 {
			adminAuthRateWindow = parsed
		}
	}

	adminMFAConfig, adminMFAErr := auth.ValidateAdminMFAConfig(
		os.Getenv("ENVIRONMENT"),
		secrets.Load("ADMIN_MFA_ENCRYPTION_KEY"),
		secrets.Load("ADMIN_MFA_RECOVERY_PEPPER"),
		os.Getenv("ADMIN_MFA_ISSUER"),
		5*time.Minute,
	)
	if adminMFAErr != nil {
		log.Fatalf("FATAL: invalid Admin MFA configuration: %v", adminMFAErr)
	}

	return &Config{
		Port:                 port,
		PostgresDSN:          postgresDSN,
		PostgresReplicaDSNs:  postgresReplicaDSNs,
		AuthContext:          adminAuthContext,
		RedisAddr:            redisAddr,
		KafkaBrokers:         kafkaBrokers,
		InstanceID:           instanceID,
		ResendAPIKey:         resendAPIKey,
		ResendFromEmail:      resendFromEmail,
		NotificationEnabled:  notificationEnabled,
		FrontendBaseURL:      frontendBaseURL,
		MarketIngestorURL:    marketIngestorURL,
		MarketIngestorAPIKey: marketIngestorAPIKey,
		S3Endpoint:           s3Endpoint,
		S3AccessKey:          s3AccessKey,
		S3SecretKey:          s3SecretKey,
		S3Region:             s3Region,
		S3KYCBucket:          s3KYCBucket,
		S3TicketBucket:       s3TicketBucket,
		S3UseSSL:             s3UseSSL,
		S3AvatarBucket:       s3AvatarBucket,
		S3AvatarPublicURL:    s3AvatarPublicURL,
		AdminIPWhitelist:     adminIPWhitelist,
		AdminAuthRateLimit:   adminAuthRateLimit,
		AdminAuthRateWindow:  adminAuthRateWindow,
		AdminMFA:             adminMFAConfig,
	}
}

// handleHealthz is a simple liveness check.
func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz checks database connectivity (primary and replicas).
func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	response := map[string]interface{}{
		"status":    "ready",
		"service":   "admin-bff",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	httpStatus := http.StatusOK

	// Check circuit breaker health first
	if !a.circuits.IsHealthy() {
		response["status"] = "unavailable"
		response["circuits"] = a.circuits.GetHealth()
		response["message"] = "circuit breakers unhealthy"
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	response["circuits"] = "healthy"

	// Check database connectivity (critical)
	if err := a.pool.HealthCheck(ctx); err != nil {
		response["status"] = "unavailable"
		response["database"] = "unavailable"
		response["message"] = "database connectivity check failed"
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}

	// Get pool stats for detailed health info
	stats := a.pool.Stats()
	replicaLag := a.pool.GetReplicationLag()
	response["database"] = map[string]interface{}{
		"status":          "healthy",
		"primary_conns":   stats.Primary.OpenConnections,
		"replica_count":   len(stats.Replicas),
		"replication_lag": replicaLag,
	}

	// Check Kafka admin connectivity (non-critical - used for admin queries only)
	a.kafkaMu.RLock()
	kafkaAvailable := a.kafkaAdmin != nil
	a.kafkaMu.RUnlock()
	if kafkaAvailable {
		response["kafka"] = "healthy"
	} else {
		response["kafka"] = "unavailable"
		// Kafka is non-critical for admin-bff, so we just mark as degraded
		if response["status"] == "ready" {
			response["status"] = "degraded"
			response["message"] = "kafka admin client unavailable (non-critical)"
		}
	}

	writeJSON(w, httpStatus, response)
}

// handleCircuitHealth returns the health status of all circuit breakers.
func (a *App) handleCircuitHealth(w http.ResponseWriter, r *http.Request) {
	health := a.circuits.GetHealth()

	status := http.StatusOK
	if health.Overall == "unhealthy" {
		status = http.StatusServiceUnavailable
	}

	writeJSON(w, status, health)
}

// Contest request/response types

type CreateContestRequest struct {
	Name                 string     `json:"name"`
	StartsAt             time.Time  `json:"starts_at"`
	EndsAt               *time.Time `json:"ends_at,omitempty"`
	EntryFeeCents        int        `json:"entry_fee_cents"`
	PlatformFeeBps       int        `json:"platform_fee_bps"`
	QtyTotal             int64      `json:"qty_total"`
	Status               string     `json:"status"`
	Description          *string    `json:"description,omitempty"`
	DurationType         string     `json:"duration_type,omitempty"`
	AssetClass           string     `json:"asset_class,omitempty"`
	DurationMinutes      int        `json:"duration_minutes,omitempty"`
	MinParticipants      int        `json:"min_participants,omitempty"`
	MaxParticipants      *int       `json:"max_participants,omitempty"`
	RegistrationDeadline *time.Time `json:"registration_deadline,omitempty"`
	AutoStart            bool       `json:"auto_start,omitempty"`
	CommissionRate       float64    `json:"commission_rate,omitempty"`
	IsFree               bool       `json:"is_free,omitempty"`
	Symbols              []string   `json:"symbols,omitempty"`
}

type CreateContestFromTemplateRequest struct {
	TemplateKey          string     `json:"template_key"`
	Name                 string     `json:"name"`
	StartsAt             time.Time  `json:"starts_at"`
	Description          *string    `json:"description,omitempty"`
	EntryFeeCents        *int       `json:"entry_fee_cents,omitempty"`
	MaxParticipants      *int       `json:"max_participants,omitempty"`
	RegistrationDeadline *time.Time `json:"registration_deadline,omitempty"`
	Symbols              []string   `json:"symbols,omitempty"`
}

type UpdateContestRequest struct {
	Name                 *string    `json:"name,omitempty"`
	StartsAt             *time.Time `json:"starts_at,omitempty"`
	EndsAt               *time.Time `json:"ends_at,omitempty"`
	EntryFeeCents        *int       `json:"entry_fee_cents,omitempty"`
	PlatformFeeBps       *int       `json:"platform_fee_bps,omitempty"`
	QtyTotal             *int64     `json:"qty_total,omitempty"`
	Description          *string    `json:"description,omitempty"`
	AssetClass           *string    `json:"asset_class,omitempty"`
	MinParticipants      *int       `json:"min_participants,omitempty"`
	MaxParticipants      *int       `json:"max_participants,omitempty"`
	RegistrationDeadline *time.Time `json:"registration_deadline,omitempty"`
	AutoStart            *bool      `json:"auto_start,omitempty"`
	CommissionRate       *float64   `json:"commission_rate,omitempty"`
	IsFree               *bool      `json:"is_free,omitempty"`
}

type ContestResponse struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Description          *string    `json:"description,omitempty"`
	StartsAt             time.Time  `json:"starts_at"`
	EndsAt               time.Time  `json:"ends_at"`
	Status               string     `json:"status"`
	EntryFeeCents        int        `json:"entry_fee_cents"`
	PlatformFeeBps       int        `json:"platform_fee_bps"`
	QtyTotal             int64      `json:"qty_total"`
	DurationType         string     `json:"duration_type"`
	AssetClass           string     `json:"asset_class"`
	DurationMinutes      int        `json:"duration_minutes"`
	MinParticipants      int        `json:"min_participants"`
	MaxParticipants      *int       `json:"max_participants,omitempty"`
	ParticipantCount     int        `json:"participant_count"`
	RegistrationDeadline *time.Time `json:"registration_deadline,omitempty"`
	AutoStart            bool       `json:"auto_start"`
	CommissionRate       float64    `json:"commission_rate"`
	IsFree               bool       `json:"is_free"`
	AutoGenerated        bool       `json:"is_auto_generated"`
	TemplateID           *string    `json:"template_id,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

type AuditLogResponse struct {
	ID          string          `json:"id"`
	ActorUserID *string         `json:"actor_user_id,omitempty"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    *string         `json:"target_id,omitempty"`
	PayloadJSON json.RawMessage `json:"payload_json,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// Contest symbols types

type ContestSymbol struct {
	Symbol                   string `json:"symbol"`
	ProviderSymbolTwelveData string `json:"provider_symbol_twelvedata,omitempty"`
	ProviderSymbolMassive    string `json:"provider_symbol_massive,omitempty"`
	Enabled                  bool   `json:"enabled"`
}

type AddContestSymbolsRequest struct {
	Symbols []ContestSymbol `json:"symbols"`
}

type ContestSymbolResponse struct {
	ContestID                string    `json:"contest_id"`
	Symbol                   string    `json:"symbol"`
	ProviderSymbolTwelveData *string   `json:"provider_symbol_twelvedata,omitempty"`
	ProviderSymbolMassive    *string   `json:"provider_symbol_massive,omitempty"`
	Enabled                  bool      `json:"enabled"`
	CreatedAt                time.Time `json:"created_at"`
}

// Master symbol types for symbol management

type SymbolResponse struct {
	Symbol                   string    `json:"symbol"`
	Name                     string    `json:"name"`
	AssetType                string    `json:"asset_type"`
	ProviderSymbolTwelveData *string   `json:"provider_symbol_twelvedata,omitempty"`
	ProviderSymbolMassive    *string   `json:"provider_symbol_massive,omitempty"`
	ProviderSymbolFinnhub    *string   `json:"provider_symbol_finnhub,omitempty"`
	IsActive                 bool      `json:"is_active"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type SymbolListResponse struct {
	Symbols []SymbolResponse `json:"symbols"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

type CreateSymbolRequest struct {
	Symbol                   string  `json:"symbol"`
	Name                     string  `json:"name"`
	AssetType                string  `json:"asset_type"`
	ProviderSymbolTwelveData *string `json:"provider_symbol_twelvedata,omitempty"`
	ProviderSymbolMassive    *string `json:"provider_symbol_massive,omitempty"`
	ProviderSymbolFinnhub    *string `json:"provider_symbol_finnhub,omitempty"`
	IsActive                 *bool   `json:"is_active,omitempty"`
}

type UpdateSymbolRequest struct {
	Name                     *string `json:"name,omitempty"`
	AssetType                *string `json:"asset_type,omitempty"`
	ProviderSymbolTwelveData *string `json:"provider_symbol_twelvedata,omitempty"`
	ProviderSymbolMassive    *string `json:"provider_symbol_massive,omitempty"`
	ProviderSymbolFinnhub    *string `json:"provider_symbol_finnhub,omitempty"`
	IsActive                 *bool   `json:"is_active,omitempty"`
}

// Health/shards types

type InstanceHeartbeat struct {
	InstanceID string    `json:"instance_id"`
	LastSeen   time.Time `json:"last_seen"`
	Status     string    `json:"status"`
}

type TopicPartitionLag struct {
	Topic       string            `json:"topic"`
	Partitions  map[int32]LagInfo `json:"partitions"`
	TotalLag    int64             `json:"total_lag"`
	Available   bool              `json:"available"`
	ErrorReason string            `json:"error_reason,omitempty"`
}

type LagInfo struct {
	Partition     int32 `json:"partition"`
	CurrentOffset int64 `json:"current_offset"`
	HighWaterMark int64 `json:"high_watermark"`
	Lag           int64 `json:"lag"`
}

type ShardsHealthResponse struct {
	Instances      []InstanceHeartbeat `json:"instances"`
	KafkaTopics    []TopicPartitionLag `json:"kafka_topics"`
	KafkaAvailable bool                `json:"kafka_available"`
	RedisAvailable bool                `json:"redis_available"`
	CheckedAt      time.Time           `json:"checked_at"`
}

// Contest location type

type ContestLocationResponse struct {
	ContestID       string `json:"contest_id"`
	Topic           string `json:"topic"`
	Partition       int32  `json:"partition"`
	PartitionKey    string `json:"partition_key"`
	PartitionMethod string `json:"partition_method"`
	Available       bool   `json:"available"`
	ErrorReason     string `json:"error_reason,omitempty"`
}

// KYC types

type KYCPendingSubmission struct {
	UserID          string    `json:"user_id"`
	Email           string    `json:"email"`
	FirstName       *string   `json:"first_name,omitempty"`
	LastName        *string   `json:"last_name,omitempty"`
	DocumentType    *string   `json:"document_type,omitempty"`
	Status          string    `json:"status"`
	SubmittedAt     time.Time `json:"submitted_at"`
	Provider        *string   `json:"provider,omitempty"`
	ShahkarVerified bool      `json:"shahkar_verified"`
	FaceVerified    bool      `json:"face_verified"`
	CardOCRVerified bool      `json:"card_ocr_verified"`
	FaceMatchScore  *float64  `json:"face_match_score,omitempty"`
	AutoApproved    bool      `json:"auto_approved"`
}

type KYCPendingListResponse struct {
	Submissions []KYCPendingSubmission `json:"submissions"`
	Total       int                    `json:"total"`
	Limit       int                    `json:"limit"`
	Offset      int                    `json:"offset"`
}

type KYCVerificationDetails struct {
	UserID                 string     `json:"user_id"`
	Status                 string     `json:"status"`
	FirstName              *string    `json:"first_name,omitempty"`
	LastName               *string    `json:"last_name,omitempty"`
	DateOfBirth            *string    `json:"date_of_birth,omitempty"`
	Nationality            *string    `json:"nationality,omitempty"`
	AddressLine1           *string    `json:"address_line1,omitempty"`
	AddressLine2           *string    `json:"address_line2,omitempty"`
	City                   *string    `json:"city,omitempty"`
	State                  *string    `json:"state,omitempty"`
	PostalCode             *string    `json:"postal_code,omitempty"`
	Country                *string    `json:"country,omitempty"`
	VerifiedAt             *time.Time `json:"verified_at,omitempty"`
	VerifiedBy             *string    `json:"verified_by,omitempty"`
	RejectionReason        *string    `json:"rejection_reason,omitempty"`
	ExpiresAt              *time.Time `json:"expires_at,omitempty"`
	Provider               *string    `json:"provider,omitempty"`
	ProviderVerificationID *string    `json:"provider_verification_id,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`

	// Jibit verification fields
	NationalCode     *string  `json:"national_code,omitempty"`
	Phone            *string  `json:"phone,omitempty"`
	ShahkarVerified  bool     `json:"shahkar_verified"`
	FaceVerified     bool     `json:"face_verified"`
	FaceMatchScore   *float64 `json:"face_match_score,omitempty"`
	LivenessScore    *float64 `json:"liveness_score,omitempty"`
	LivenessResult   *string  `json:"liveness_result,omitempty"`
	CardOCRVerified  bool     `json:"card_ocr_verified"`
	CardSerialNumber *string  `json:"card_serial_number,omitempty"`
	AutoApproved     bool     `json:"auto_approved"`

	// Iranian manual KYC fields
	NationalCodeManual     *string `json:"national_code_manual,omitempty"`
	FatherName             *string `json:"father_name,omitempty"`
	BirthCertificateNumber *string `json:"birth_certificate_number,omitempty"`
	BirthCertificateSerial *string `json:"birth_certificate_serial,omitempty"`
}

type KYCDocument struct {
	ID               string     `json:"id"`
	UserID           string     `json:"user_id"`
	DocumentType     string     `json:"document_type"`
	DocumentNumber   *string    `json:"document_number,omitempty"`
	IssuingCountry   *string    `json:"issuing_country,omitempty"`
	IssueDate        *string    `json:"issue_date,omitempty"`
	ExpiryDate       *string    `json:"expiry_date,omitempty"`
	FrontImageURL    *string    `json:"front_image_url,omitempty"`
	BackImageURL     *string    `json:"back_image_url,omitempty"`
	SelfieURL        *string    `json:"selfie_url,omitempty"`
	SelfieWithDocURL *string    `json:"selfie_with_doc_url,omitempty"`
	Status           string     `json:"status"`
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`
	ReviewedBy       *string    `json:"reviewed_by,omitempty"`
	ReviewNotes      *string    `json:"review_notes,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

type KYCAuditLogEntry struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Action    string          `json:"action"`
	ActorID   *string         `json:"actor_id,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

type KYCDetailsResponse struct {
	User      KYCVerificationDetails `json:"user"`
	Documents []KYCDocument          `json:"documents"`
	AuditLog  []KYCAuditLogEntry     `json:"audit_log"`
	UserEmail string                 `json:"user_email"`
}

type KYCApproveRequest struct {
	Notes *string `json:"notes,omitempty"`
}

type KYCRejectRequest struct {
	Reason         string            `json:"reason"`
	RejectedFields []string          `json:"rejected_fields,omitempty"`
	FieldMessages  map[string]string `json:"field_messages,omitempty"`
}

type KYCRequestInfoRequest struct {
	Message string `json:"message"`
}

// User management types

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Roles     []string  `json:"roles"`
	Status    string    `json:"status"`
	KYCStatus *string   `json:"kyc_status,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type UserDetailResponse struct {
	// Basic user info
	User UserBasicInfo `json:"user"`
	// User roles
	Roles []string `json:"roles"`
	// KYC information
	KYC UserKYCInfo `json:"kyc"`
	// Wallet information
	Wallet UserWalletInfo `json:"wallet"`
	// User statistics
	Stats UserStats `json:"stats"`
	// Recent contest participation
	RecentContests []UserContestEntry `json:"recent_contests"`
	// Recent wallet transactions
	RecentTransactions []UserTransaction `json:"recent_transactions"`
	// Affiliate information
	Affiliate UserAffiliateInfo `json:"affiliate"`
	// Active sessions
	Sessions []UserSessionInfo `json:"sessions"`
}

// UserBasicInfo represents basic user information.
type UserBasicInfo struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Username      *string   `json:"username,omitempty"`
	DisplayName   *string   `json:"display_name,omitempty"`
	AvatarURL     *string   `json:"avatar_url,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	EmailVerified bool      `json:"email_verified"`
	Status        string    `json:"status"`
	Country       *string   `json:"country,omitempty"`
}

// UserKYCInfo represents KYC verification information.
type UserKYCInfo struct {
	Status      string     `json:"status"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
}

// UserWalletInfo represents wallet information.
type UserWalletInfo struct {
	BalanceCents int64  `json:"balance_cents"`
	Currency     string `json:"currency"`
	Status       string `json:"status"`
}

// UserStats represents user statistics.
type UserStats struct {
	TotalContests int     `json:"total_contests"`
	TotalWins     int     `json:"total_wins"`
	TraggePoint   float64 `json:"tragge_point"`
	TotalTrades   int64   `json:"total_trades"`
	TotalPnL      float64 `json:"total_pnl"`
}

// UserContestEntry represents a contest the user participated in.
type UserContestEntry struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	Rank *int      `json:"rank,omitempty"`
	PnL  float64   `json:"pnl"`
	Date time.Time `json:"date"`
}

// UserTransaction represents a wallet transaction.
type UserTransaction struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Amount       int64     `json:"amount"`
	Date         time.Time `json:"date"`
	Description  *string   `json:"description,omitempty"`
	ReasonCode   *string   `json:"reason_code,omitempty"`
	RefType      *string   `json:"ref_type,omitempty"`
	RefID        *string   `json:"ref_id,omitempty"`
	BalanceAfter int64     `json:"balance_after"`
}

// AdminWalletHistoryEntry represents a detailed ledger entry for admin view.
type AdminWalletHistoryEntry struct {
	ID                string  `json:"id"`
	Type              string  `json:"type"`
	AmountCents       int64   `json:"amount_cents"`
	BalanceAfterCents int64   `json:"balance_after_cents"`
	Description       *string `json:"description,omitempty"`
	ReasonCode        *string `json:"reason_code,omitempty"`
	RefType           *string `json:"ref_type,omitempty"`
	RefID             *string `json:"ref_id,omitempty"`
	IdempotencyKey    *string `json:"idempotency_key,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

// AdminWalletHistoryResponse is the paginated response for admin wallet history.
type AdminWalletHistoryResponse struct {
	Entries      []AdminWalletHistoryEntry `json:"entries"`
	Total        int                       `json:"total"`
	BalanceCents int64                     `json:"balance_cents"`
	Currency     string                    `json:"currency"`
	WalletStatus string                    `json:"wallet_status"`
	Page         int                       `json:"page"`
	HasMore      bool                      `json:"has_more"`
}

// UserAffiliateInfo represents affiliate program information.
type UserAffiliateInfo struct {
	Code           *string `json:"code,omitempty"`
	Status         string  `json:"status"`
	TotalReferrals int     `json:"total_referrals"`
	TotalEarned    int64   `json:"total_earned"`
}

// UserSessionInfo represents an active session.
type UserSessionInfo struct {
	ID         string    `json:"id"`
	Device     string    `json:"device"`
	IP         string    `json:"ip"`
	LastActive time.Time `json:"last_active"`
}

// BanUserRequest represents a request to ban a user.
type BanUserRequest struct {
	Reason   string `json:"reason"`
	Duration string `json:"duration"` // "permanent", "7d", "30d"
}

// UnbanUserRequest represents a request to unban a user.
type UnbanUserRequest struct {
	Reason string `json:"reason,omitempty"`
}

type UserListResponse struct {
	Users  []UserResponse `json:"users"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type UpdateUserRolesRequest struct {
	Roles  []string `json:"roles"`
	Reason string   `json:"reason"`
}

type UpdateUserStatusRequest struct {
	Status string `json:"status"` // "active" or "suspended"
	Reason string `json:"reason,omitempty"`
}

// AdminCreateUserRequest is the request body for creating a user from admin panel.
type AdminCreateUserRequest struct {
	Email         string   `json:"email"`
	Password      string   `json:"password"`       // If empty, generate a random temporary password
	DisplayName   string   `json:"display_name"`   // Optional
	Roles         []string `json:"roles"`          // canonical user/support_admin/super_admin roles; default: ["user"]
	EmailVerified bool     `json:"email_verified"` // Skip email verification (admin-created users)
	Reason        string   `json:"reason"`
}

// AdminCreateUserResponse is the response for admin user creation.
type AdminCreateUserResponse struct {
	UserID            string   `json:"user_id"`
	Email             string   `json:"email"`
	Roles             []string `json:"roles"`
	TemporaryPassword string   `json:"temporary_password,omitempty"` // Only returned if password was auto-generated
	Message           string   `json:"message"`
}
