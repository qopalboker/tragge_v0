package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/IBM/sarama"
	"github.com/Parsaeffatravesh/tragge/apps/contest-scheduler/internal/health"
	"github.com/Parsaeffatravesh/tragge/apps/contest-scheduler/internal/scheduler"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/domain/statemachine"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// Run starts the contest-scheduler service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil)
}

// RunWithSharedDeps starts the contest-scheduler service, optionally using shared resources.
// When parentCtx is non-nil, the service shuts down when the context is cancelled
// instead of registering its own signal handler.
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

	// Initialize observability
	obs, err := observability.New(ctx, observability.Config{
		Service:              "contest-scheduler",
		Env:                  cfg.Environment,
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

	log.Info("Starting contest-scheduler",
		zap.String("instance_id", cfg.InstanceID),
		zap.String("environment", cfg.Environment))

	// Initialize database pool with read/write splitting
	var pool *db.Pool
	if sharedPool != nil {
		pool = sharedPool
		log.Info("Using shared database pool")
	} else {
		dbCfg := db.ConfigFromEnv(os.Getenv)
		dbCfg.PrimaryDSN = cfg.PostgresDSN
		if cfg.PostgresReplica != "" {
			dbCfg.ReplicaDSNs = []string{cfg.PostgresReplica}
		}

		var poolErr error
		pool, poolErr = db.NewPool(ctx, dbCfg)
		if poolErr != nil {
			log.Fatal("Failed to create database pool", zap.Error(poolErr))
		}
		defer pool.Close()

		// Verify database connection
		dbCtx, dbCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := pool.HealthCheck(dbCtx); err != nil {
			log.Fatal("Failed to connect to database", zap.Error(err))
		}
		dbCancel()
		log.Info("PostgreSQL connected successfully")
	}

	// Initialize Redis with HA support (standalone, sentinel, or cluster)
	var redisClient *pkgredis.Client
	if sharedRedis != nil {
		redisClient = sharedRedis
		log.Info("Using shared Redis client")
	} else {
		redisCfg := pkgredis.ConfigFromEnv(os.Getenv)

		var redisErr error
		redisClient, redisErr = pkgredis.NewClient(redisCfg)
		if redisErr != nil {
			log.Fatal("Failed to create Redis client", zap.Error(redisErr))
		}
		defer redisClient.Close()

		// Verify Redis connection
		redisCtx, redisCancel := context.WithTimeout(ctx, 5*time.Second)
		if err := redisClient.Ping(redisCtx).Err(); err != nil {
			log.Fatal("Failed to connect to Redis", zap.Error(err))
		}
		redisCancel()
		log.Info("Redis connected successfully",
			zap.String("mode", string(redisClient.Mode())))
	}

	// Initialize Kafka producer (for publishing contest state events)
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Producer.RequiredAcks = sarama.WaitForAll
	kafkaConfig.Producer.Retry.Max = 5
	kafkaConfig.Producer.Return.Successes = true

	kafkaProducer, err := sarama.NewSyncProducer(cfg.KafkaBrokers, kafkaConfig)
	if err != nil {
		log.Fatal("Failed to create Kafka producer", zap.Error(err))
	}
	defer kafkaProducer.Close()
	log.Info("Kafka producer initialized",
		zap.Strings("brokers", cfg.KafkaBrokers),
		zap.String("topic", cfg.ContestStateTopic))

	// Configure state machine
	smConfig := &statemachine.Config{
		KafkaProducer:     kafkaProducer,
		ContestStateTopic: cfg.ContestStateTopic,
		Logger:            log,
	}

	// Configure scheduler with adaptive check intervals
	schedulerCfg := scheduler.Config{
		CheckInterval:     cfg.CheckInterval,    // Acts as MaxCheckInterval
		MinCheckInterval:  cfg.MinCheckInterval, // Minimum adaptive interval
		StartBuffer:       cfg.StartBuffer,
		SettlementDelay:   cfg.SettlementDelay,
		MaxConcurrent:     cfg.MaxConcurrent,
		MaxRetries:        cfg.MaxRetries,
		RetryBaseDelay:    cfg.RetryBaseDelay,
		RetryMaxDelay:     cfg.RetryMaxDelay,
		LockTTL:           cfg.LockTTL,
		LockRetryInterval: cfg.LockRetryInterval,
		InstanceID:        cfg.InstanceID,
	}

	// Create scheduler
	sched := scheduler.New(pool, redisClient.UniversalClient, smConfig, schedulerCfg, log)

	// Start scheduler
	sched.Start(ctx)

	// Initialize calendar processor if enabled
	var calendarProc *scheduler.CalendarProcessor
	if cfg.CalendarEnabled {
		calendarCfg := scheduler.CalendarConfig{
			CheckInterval: cfg.CalendarCheckInterval,
			LockTTL:       cfg.CalendarLockTTL,
		}
		// Create a StateMachine for the calendar processor so it can perform
		// proper state transitions with side effects instead of direct SQL inserts.
		calendarSM := statemachine.New(pool, smConfig)
		calendarProc = scheduler.NewCalendarProcessor(pool, redisClient.UniversalClient, calendarSM, calendarCfg, log)
		calendarProc.Start(ctx)
		log.Info("Calendar processor started",
			zap.Duration("check_interval", cfg.CalendarCheckInterval))
	} else {
		log.Info("Calendar processor disabled")
	}

	// Initialize reminder service if enabled
	var reminderSvc *scheduler.ReminderService
	if cfg.ReminderEnabled {
		// Initialize email notifier for reminders
		var emailNotifier *notification.EmailNotifier
		if cfg.ResendAPIKey != "" {
			emailCfg := notification.EmailConfig{
				APIKey:    cfg.ResendAPIKey,
				FromEmail: cfg.ResendFromEmail,
			}
			var err error
			emailNotifier, err = notification.NewEmailNotifier(emailCfg, log)
			if err != nil {
				log.Error("Failed to create email notifier for reminders, emails will be disabled",
					zap.Error(err))
			} else {
				// Connect the template override store so the multi-version
				// template system works (picks up active versions from DB).
				templateStore := notification.NewDBTemplateStore(pool.Primary())
				emailNotifier.SetOverrideStore(templateStore)
				log.Info("Email notifier initialized for contest reminders (with template overrides)")
			}
		} else {
			log.Warn("RESEND_API_KEY not configured, reminder emails will be disabled")
		}

		// Create reminder service with configurable intervals
		reminderCfg := scheduler.ReminderConfig{
			Intervals:      cfg.ReminderIntervals,
			EndIntervals:   cfg.EndReminderIntervals,
			CheckInterval:  cfg.ReminderInterval,
			BatchSize:      cfg.ReminderBatchSize,
			TradingBaseURL: cfg.TradingBaseURL,
		}
		reminderSvc = scheduler.NewReminderService(pool, emailNotifier, reminderCfg, log)
		reminderSvc.Start(ctx)
		log.Info("Reminder service started",
			zap.Int("intervals", len(cfg.ReminderIntervals)),
			zap.Duration("check_interval", cfg.ReminderInterval))
	} else {
		log.Info("Reminder service disabled")
	}

	// Initialize cleanup service if enabled
	var cleanupSvc *scheduler.CleanupService
	if cfg.CleanupEnabled {
		// Create a state machine instance for the cleanup service
		cleanupSM := statemachine.New(pool, smConfig)
		cleanupCfg := scheduler.CleanupConfig{
			ArchiveAfterDays: cfg.CleanupArchiveDays,
			CheckInterval:    1 * time.Hour,
			LockTTL:          cfg.CleanupLockTTL,
			Timezone:         cfg.CleanupTimezone,
			RunHour:          cfg.CleanupRunHour,
			RunMinute:        cfg.CleanupRunMinute,
			InstanceID:       cfg.InstanceID,
		}
		cleanupSvc = scheduler.NewCleanupService(pool, redisClient.UniversalClient, cleanupSM, cleanupCfg, log)
		cleanupSvc.Start(ctx)
		log.Info("Cleanup service started",
			zap.Int("archive_after_days", cfg.CleanupArchiveDays),
			zap.String("timezone", cfg.CleanupTimezone),
			zap.Int("run_hour", cfg.CleanupRunHour))
	} else {
		log.Info("Cleanup service disabled")
	}

	// Create health handler
	healthHandler := health.NewHandler(pool, redisClient.UniversalClient, sched, log)
	if calendarProc != nil {
		healthHandler.SetCalendarProcessor(calendarProc)
	}

	// Set up HTTP server
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(obs.Middleware.Middleware)
	r.Use(obs.Middleware.Recovery)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health endpoints
	r.Get("/healthz", healthHandler.HandleHealthz)
	r.Get("/readyz", healthHandler.HandleReadyz)
	r.Get("/health/scheduler", healthHandler.HandleSchedulerHealth)
	r.Get("/health/calendar", healthHandler.HandleCalendarHealth)
	r.With(validation.InternalOnlyMiddleware).Handle("/metrics", obs.MetricsHandler())

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	infra.SafeGo(log, "contest-scheduler-http-server", func() {
		log.Info("Starting HTTP server", zap.String("port", cfg.Port))
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
	log.Info("Received shutdown signal")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop accepting new HTTP requests
	log.Info("Stopping HTTP server")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Warn("Server forced to shutdown", zap.Error(err))
	}

	// Stop the calendar processor if running
	if calendarProc != nil && calendarProc.IsRunning() {
		log.Info("Stopping calendar processor")
		calendarProc.Stop(shutdownCtx)
	}

	// Stop the reminder service if running
	if reminderSvc != nil && reminderSvc.IsRunning() {
		log.Info("Stopping reminder service")
		reminderSvc.Stop(shutdownCtx)
	}

	// Stop the cleanup service if running
	if cleanupSvc != nil && cleanupSvc.IsRunning() {
		log.Info("Stopping cleanup service")
		cleanupSvc.Stop(shutdownCtx)
	}

	// Stop the scheduler (waits for in-progress transitions)
	log.Info("Stopping scheduler")
	sched.Stop(shutdownCtx)

	// Cancel main context
	cancel()

	log.Info("Shutdown complete")
}
