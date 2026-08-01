package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/ratelimit"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg := LoadConfig()

	// Initialize context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize observability (logging, metrics, tracing)
	obs, err := observability.New(ctx, cfg.Observability)
	if err != nil {
		panic("failed to initialize observability: " + err.Error())
	}
	defer obs.Shutdown(context.Background())

	logger := obs.Logger.Logger

	// Initialize circuit breakers
	circuitCfg := DefaultCircuitBreakerConfig()
	circuitCfg.Logger = logger
	circuitCfg.OnStateChange = func(name string, from, to circuitbreaker.State) {
		logger.Warn("Circuit breaker state changed",
			zap.String("circuit", name),
			zap.String("from", from.String()),
			zap.String("to", to.String()))
	}
	circuits := NewCircuitBreakers(circuitCfg)

	logger.Info("starting shard-router service",
		zap.String("port", cfg.Port),
		zap.String("environment", cfg.Observability.Env),
		zap.String("version", cfg.Observability.Version),
	)

	// Initialize database pool with replica support
	dbPool, err := db.NewPool(ctx, cfg.DB)
	if err != nil {
		logger.Fatal("failed to initialize database pool", zap.Error(err))
	}
	defer dbPool.Close()

	logger.Info("database pool initialized",
		zap.Int("replica_count", len(cfg.DB.ReplicaDSNs)),
	)

	// Initialize Redis client
	redisClient, err := redis.NewClient(cfg.Redis)
	if err != nil {
		logger.Fatal("failed to initialize redis client", zap.Error(err))
	}
	defer redisClient.Close()

	// Verify Redis connection
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("failed to connect to redis", zap.Error(err))
	}

	logger.Info("redis client initialized",
		zap.String("mode", string(redisClient.Mode())),
	)

	// Initialize shard cache
	cache := NewShardCache(redisClient, cfg.CacheTTL, logger)

	// Initialize shard router
	router := NewShardRouter(cfg.VirtualNodes, logger)

	// Load shards from database (using read replica if available)
	if err := router.LoadShards(ctx, dbPool.Replica()); err != nil {
		logger.Fatal("failed to load shards from database", zap.Error(err))
	}

	// Initialize cached router with DB-pinned contest stickiness
	cachedRouter := NewCachedRouter(router, cache, dbPool, logger)

	// Initialize notification service
	var notifSvc *notification.Service
	var notifier *Notifier
	notifSvc = initNotifications(ctx, cfg, logger, nil)
	if notifSvc != nil {
		notifier = NewNotifier(notifSvc, cfg, logger)
	}

	// Initialize handlers
	isDev := cfg.Environment == "development" || cfg.Environment == "local"
	handlers := NewHandlers(router, cachedRouter, cache, dbPool, logger, notifier, circuits, isDev)

	// Initialize the explicit Admin authentication trust context.
	authService, err := auth.NewContext(cfg.AuthContext, nil)
	if err != nil {
		logger.Fatal("Failed to initialize Admin authentication context", zap.Error(err))
	}

	// Initialize rate limiter
	limiter := ratelimit.NewUserLimiter(cfg.RateLimit)
	defer limiter.Close()

	// Setup HTTP router
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(obs.Middleware.Middleware)
	r.Use(obs.Middleware.Recovery)

	// Rate limiting for API routes (skip health checks)
	r.Use(ratelimit.RateLimitMiddleware(
		limiter,
		ratelimit.WithKeyExtractor(ratelimit.IPExtractor),
		ratelimit.WithSkipPaths("/healthz", "/readyz", "/metrics"),
	))

	// Health check endpoints (no rate limiting)
	r.Get("/healthz", handlers.HealthzHandler)
	r.Get("/readyz", handlers.ReadyzHandler)
	r.Get("/health/circuits", handlers.CircuitHealthHandler)

	// Metrics endpoint
	r.With(validation.InternalOnlyMiddleware).Get("/metrics", obs.MetricsHandler().ServeHTTP)

	// API routes
	r.Route("/shard", func(r chi.Router) {
		r.Get("/{contestID}", handlers.GetShardHandler)
	})

	r.Route("/shards", func(r chi.Router) {
		r.Get("/", handlers.ListShardsHandler)
		r.Get("/{shardID}", handlers.GetShardInfoHandler)
		r.Group(func(r chi.Router) {
			r.Use(authService.Middleware.RequireAuth)
			r.Use(authService.Middleware.RequireAdmin)
			r.Post("/", handlers.AddShardHandler)
			r.Post("/{shardID}/drain", handlers.DrainShardHandler)
			r.Delete("/{shardID}", handlers.RemoveShardHandler)
		})
	})

	// Create HTTP server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start background shard refresh
	refreshDone := make(chan struct{})
	infra.SafeGo(logger, "shard-refresh", func() {
		defer close(refreshDone)
		ticker := time.NewTicker(cfg.ShardRefreshPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := router.LoadShards(ctx, dbPool.Replica()); err != nil {
					logger.Error("failed to refresh shards", zap.Error(err))
				} else {
					logger.Debug("shards refreshed successfully")
				}
				// Invalidate shard list cache after refresh
				if err := cache.InvalidateShardList(ctx); err != nil {
					logger.Warn("failed to invalidate shard list cache", zap.Error(err))
				}
			}
		}
	})

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	infra.SafeGo(logger, "shard-router-http-server", func() {
		logger.Info("HTTP server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	})

	// Send startup notification
	if notifier != nil {
		notifier.sendStartupNotification(router.ShardCount(), len(router.ListShards()))
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("received shutdown signal", zap.String("signal", sig.String()))
	case err := <-serverErr:
		logger.Error("server error", zap.Error(err))
	}

	// Send shutdown notification
	if notifier != nil {
		notifier.sendShutdownNotification()
	}

	// Cancel context to stop background tasks
	cancel()

	// Wait for background tasks to complete
	<-refreshDone

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	logger.Info("shutting down HTTP server")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown HTTP server gracefully", zap.Error(err))
	}

	// Shutdown notification service (drain pending notifications)
	if notifier != nil {
		notifShutdownCtx, notifShutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := notifier.Shutdown(notifShutdownCtx); err != nil {
			logger.Warn("notification service shutdown error", zap.Error(err))
		}
		notifShutdownCancel()
	}

	logger.Info("shard-router service stopped")
}
