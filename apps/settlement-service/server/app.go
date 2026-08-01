package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// App holds application dependencies.
type App struct {
	config            *Config
	db                *sql.DB
	redis             *redis.Client
	kafka             *kgo.Client
	wallet            *wallet.Service
	emailNotifier     *notification.EmailNotifier
	settlement        *SettlementService
	obs               *observability.Observability
	metrics           *SettlementMetrics
	auth              *auth.Auth
	ready             atomic.Bool
	ctx               context.Context
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	activeSettlements sync.Map // contestID -> struct{}, prevents duplicate settlement goroutines

	// Shared resource flags
	sharedDB bool
}

// Run starts the settlement-service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil)
}

// RunWithSharedDeps starts the settlement-service, optionally using shared resources.
// When parentCtx is non-nil, the service shuts down when the context is cancelled
// instead of registering its own signal handler. When sharedPool is non-nil, the service
// uses pool.Primary() for its *sql.DB instead of creating its own connection.
// Note: sharedRedis is accepted for interface consistency but settlement-service
// uses *redis.Client directly, so Redis sharing is not supported yet.
func RunWithSharedDeps(parentCtx context.Context, sharedPool *db.Pool, sharedRedis *pkgredis.Client) {
	// Validate critical environment variables in production/staging
	if sharedPool == nil {
		config.MustBeSetAny("database connection", "POSTGRES_DSN", "POSTGRES_HOST")
	}
	config.MustBeSet("REDIS_ADDR", "KAFKA_BROKERS")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app := &App{
		config: LoadConfig(),
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize observability
	obs, err := observability.New(ctx, observability.Config{
		Service:              "settlement-service",
		Env:                  os.Getenv("ENVIRONMENT"),
		Version:              os.Getenv("VERSION"),
		OTLPEndpoint:         os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		EnableGoMetrics:      true,
		EnableProcessMetrics: true,
	})
	if err != nil {
		panic("failed to initialize observability: " + err.Error())
	}
	app.obs = obs
	app.metrics = NewSettlementMetrics(obs.Metrics.Registry())
	defer obs.Shutdown(context.Background())

	// Initialize the explicit Admin authentication trust context.
	app.auth, err = auth.NewContext(app.config.AuthContext, nil)
	if err != nil {
		panic("settlement-service: failed to initialize Admin authentication context: " + err.Error())
	}

	app.log().Info("Starting settlement service",
		zap.String("port", app.config.Port),
		zap.Strings("kafka_brokers", app.config.KafkaBrokers))

	// Initialize PostgreSQL
	if sharedPool != nil {
		app.db = sharedPool.Primary()
		app.sharedDB = true
		app.log().Info("Using shared database pool")
	} else {
		localDB, err := sql.Open("pgx", app.config.PostgresDSN)
		if err != nil {
			app.log().Fatal("Failed to open database", zap.Error(err))
		}
		app.db = localDB
		localDB.SetMaxOpenConns(10)
		localDB.SetMaxIdleConns(5)
		localDB.SetConnMaxLifetime(5 * time.Minute)

		dbCtx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := localDB.PingContext(dbCtx); err != nil {
			dbCancel()
			app.log().Fatal("Failed to connect to database", zap.Error(err))
		}
		dbCancel()
		app.log().Info("Connected to PostgreSQL")
	}

	// Initialize Redis
	app.redis = redis.NewClient(&redis.Options{
		Addr:     app.config.RedisAddr,
		Password: secrets.Load("REDIS_PASSWORD"),
	})
	redisCtx, redisCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := app.redis.Ping(redisCtx).Err(); err != nil {
		redisCancel()
		app.log().Fatal("Failed to connect to Redis", zap.Error(err))
	}
	redisCancel()
	app.log().Info("Connected to Redis")

	// Initialize Kafka producer
	prodOpts := []kgo.Opt{
		kgo.SeedBrokers(app.config.KafkaBrokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	}
	prodOpts = append(prodOpts, infra.KafkaSecurityOpts()...)
	kafka, err := kgo.NewClient(prodOpts...)
	if err != nil {
		app.log().Fatal("Failed to create Kafka producer", zap.Error(err))
	}
	app.kafka = kafka
	app.log().Info("Kafka producer initialized")

	// Initialize wallet service
	app.wallet = wallet.NewService(app.db)

	// Initialize email notifier (optional - only if Resend API key is configured)
	if app.config.ResendAPIKey != "" {
		emailNotifier, err := notification.NewEmailNotifier(notification.EmailConfig{
			APIKey:    app.config.ResendAPIKey,
			FromEmail: config.GetEnv("EMAIL_FROM", "noreply@tragge.com"),
			ReplyTo:   config.GetEnv("EMAIL_REPLY_TO", ""),
		}, app.log())
		if err != nil {
			app.log().Warn("Failed to initialize email notifier", zap.Error(err))
		} else {
			app.emailNotifier = emailNotifier
			app.log().Info("Email notifier initialized")
		}
	} else {
		app.log().Info("Email notifier disabled (RESEND_API_KEY not set)")
	}

	// Initialize settlement service
	app.settlement = NewSettlementService(app)

	// Start consumers and background tasks
	app.wg.Add(3)
	go app.consumeContestStateEvents()
	go app.consumeSettlementRequests()
	go app.startStuckDetector(ctx)

	// Mark as ready
	app.ready.Store(true)
	app.log().Info("Settlement service is ready")

	// Start HTTP server
	r := chi.NewRouter()
	r.Use(obs.Middleware.Middleware)
	r.Use(obs.Middleware.Recovery)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/healthz", app.handleHealthz)
	r.Get("/readyz", app.handleReadyz)
	r.With(validation.InternalOnlyMiddleware).Handle("/metrics", obs.MetricsHandler())

	// Admin endpoints (requires authentication + admin role)
	r.Route("/admin", func(r chi.Router) {
		r.Use(app.auth.Middleware.RequireAuth)
		r.Use(app.auth.Middleware.RequireAdminAccess)
		r.Post("/settle/{contestID}", app.handleTriggerSettlement)
	})

	server := &http.Server{
		Addr:         ":" + app.config.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	infra.SafeGo(app.log(), "http-server", func() {
		app.log().Info("HTTP server starting", zap.String("port", app.config.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			app.log().Fatal("HTTP server error", zap.Error(err))
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

	app.log().Info("Shutting down settlement service...")
	app.ready.Store(false)
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		app.log().Error("HTTP server shutdown error", zap.Error(err))
	}

	app.wg.Wait()
	app.kafka.Close()
	app.redis.Close()
	if !app.sharedDB {
		app.db.Close()
	}

	app.log().Info("Settlement service stopped")
}

func (a *App) log() *zap.Logger {
	return a.obs.Logger.Logger
}

// tryStartSettlement attempts to start a settlement goroutine for the given contest.
// If a settlement is already in-flight for this contest, the duplicate is skipped.
// Returns true if a new goroutine was launched.
func (a *App) tryStartSettlement(contestID, source string) bool {
	if _, loaded := a.activeSettlements.LoadOrStore(contestID, struct{}{}); loaded {
		a.log().Info("Settlement already in-flight, skipping duplicate trigger",
			zap.String("contest_id", contestID),
			zap.String("source", source))
		return false
	}

	a.metrics.SettlementsStarted.WithLabelValues(source).Inc()
	a.metrics.ActiveSettlements.Inc()

	a.wg.Add(1)
	infra.SafeGo(a.log(), "settlement-contest", func() {
		defer a.wg.Done()
		defer a.activeSettlements.Delete(contestID)
		defer a.metrics.ActiveSettlements.Dec()

		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), a.config.SettlementTimeout)
		defer cancel()

		err := a.settlement.SettleContestWithRetry(ctx, contestID)
		duration := time.Since(start).Seconds()

		if err != nil {
			status := "failed"
			if errors.Is(err, context.DeadlineExceeded) {
				status = "timeout"
				a.log().Error("Settlement timed out",
					zap.String("contest_id", contestID),
					zap.String("source", source),
					zap.Duration("timeout", a.config.SettlementTimeout))
			} else {
				a.log().Error("Settlement failed",
					zap.String("contest_id", contestID),
					zap.String("source", source),
					zap.Error(err))
			}
			a.metrics.SettlementsCompleted.WithLabelValues(status).Inc()
			a.metrics.SettlementDuration.WithLabelValues(status).Observe(duration)
		} else {
			a.metrics.SettlementsCompleted.WithLabelValues("completed").Inc()
			a.metrics.SettlementDuration.WithLabelValues("completed").Observe(duration)
		}
	})

	return true
}

// HTTP handlers

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !a.ready.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
		return
	}

	// Check dependencies
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Check database
	if err := a.db.PingContext(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "database unavailable"})
		return
	}

	// Check Redis
	if err := a.redis.Ping(ctx).Err(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "redis unavailable"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func (a *App) handleTriggerSettlement(w http.ResponseWriter, r *http.Request) {
	contestID := chi.URLParam(r, "contestID")
	if contestID == "" {
		http.Error(w, "contest_id is required", http.StatusBadRequest)
		return
	}

	if _, valid := validation.ValidateUUID(contestID); !valid {
		http.Error(w, "invalid contest_id format", http.StatusBadRequest)
		return
	}

	a.log().Info("Manual settlement triggered", zap.String("contest_id", contestID))

	if !a.tryStartSettlement(contestID, "admin_trigger") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"status":     "already_running",
			"contest_id": contestID,
			"message":    "settlement already in progress",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "accepted",
		"contest_id": contestID,
		"message":    "settlement process started",
	})
}

// Kafka consumers

func (a *App) consumeContestStateEvents() {
	defer a.wg.Done()

	stateOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-contest-state"),
		kgo.ConsumeTopics(a.config.ContestStateTopic),
		kgo.DisableAutoCommit(),
	}
	stateOpts = append(stateOpts, infra.KafkaSecurityOpts()...)
	consumer, err := kgo.NewClient(stateOpts...)
	if err != nil {
		a.log().Error("Failed to create contest state consumer", zap.Error(err))
		return
	}
	defer consumer.Close()

	a.log().Info("Starting contest state consumer", zap.String("topic", a.config.ContestStateTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Contest state consumer shutting down")
			return
		default:
		}

		fetches := consumer.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Contest state fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processContestStateRecord(record)
			}

			if err := consumer.CommitUncommittedOffsets(a.ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					a.log().Error("Contest state commit error", zap.Error(err))
				}
			}
		})
	}
}

func (a *App) processContestStateRecord(record *kgo.Record) {
	var state contracts.ContestState
	if err := json.Unmarshal(record.Value, &state); err != nil {
		a.log().Error("Failed to unmarshal ContestState", zap.Error(err))
		return
	}

	if state.ContestID == "" {
		a.log().Error("Received ContestState with empty contest_id")
		return
	}

	a.log().Info("Received contest state event",
		zap.String("contest_id", state.ContestID),
		zap.String("phase", string(state.Phase)),
		zap.String("status", string(state.Status)))

	// Trigger settlement on ENDED phase or settling status
	if state.Phase == contracts.ContestPhaseEnded ||
		state.Status == contracts.ContestStatusSettling {

		a.log().Info("Triggering settlement from contest state",
			zap.String("contest_id", state.ContestID))

		a.tryStartSettlement(state.ContestID, "contest_state_event")
	}
}

func (a *App) consumeSettlementRequests() {
	defer a.wg.Done()

	reqOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-settlement-req"),
		kgo.ConsumeTopics(a.config.SettlementReqTopic),
		kgo.DisableAutoCommit(),
	}
	reqOpts = append(reqOpts, infra.KafkaSecurityOpts()...)
	consumer, err := kgo.NewClient(reqOpts...)
	if err != nil {
		a.log().Error("Failed to create settlement request consumer", zap.Error(err))
		return
	}
	defer consumer.Close()

	a.log().Info("Starting settlement request consumer", zap.String("topic", a.config.SettlementReqTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Settlement request consumer shutting down")
			return
		default:
		}

		fetches := consumer.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Settlement request fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processSettlementRequest(record)
			}

			if err := consumer.CommitUncommittedOffsets(a.ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					a.log().Error("Settlement request commit error", zap.Error(err))
				}
			}
		})
	}
}

func (a *App) processSettlementRequest(record *kgo.Record) {
	var req contracts.SettlementRequest
	if err := json.Unmarshal(record.Value, &req); err != nil {
		a.log().Error("Failed to unmarshal SettlementRequest", zap.Error(err))
		return
	}

	a.log().Info("Received settlement request",
		zap.String("contest_id", req.ContestID),
		zap.String("reason", req.Reason))

	a.tryStartSettlement(req.ContestID, "settlement_request")
}
