package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/handlers"
	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/kyc"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/ratelimit"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/Parsaeffatravesh/tragge/packages/wallet/exchangerate"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// App holds application dependencies.
type App struct {
	pool         *db.Pool
	redis        *pkgredis.Client
	auth         *auth.Auth
	wallet       *wallet.Service
	kyc          *kyc.Service
	providers    *providers.ProviderRegistry
	circuits     *CircuitBreakers
	config       *Config
	obs          *observability.Observability
	exchangeRate *exchangerate.Service

	// Handlers
	depositHandler  *handlers.DepositHandler
	withdrawHandler *handlers.WithdrawHandler
	webhookHandler  *handlers.WebhookHandler
	historyHandler  *handlers.HistoryHandler
}

// Run starts the payment-service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil, nil)
}

func registerPaymentWebhookRoutes(r chi.Router, nowPayments http.HandlerFunc) {
	r.Route("/webhooks", func(r chi.Router) {
		// Rate limit webhook endpoints: 60 concurrent requests to prevent abuse.
		r.Use(middleware.Throttle(60))
		r.Post("/nowpayments", nowPayments)
	})
}

// RunWithSharedDeps starts the payment-service, optionally using shared resources.
// When parentCtx is non-nil, the service shuts down when the context is cancelled
// instead of registering its own signal handler. When sharedPool/sharedRedis/sharedAuth
// are non-nil, the service uses those instead of creating its own.
func RunWithSharedDeps(parentCtx context.Context, sharedPool *db.Pool, sharedRedis *pkgredis.Client, sharedAuth *auth.Auth) {
	// Validate critical environment variables in production/staging
	if sharedPool == nil {
		config.MustBeSetAny("database connection", "POSTGRES_DSN", "POSTGRES_HOST")
	}
	if sharedRedis == nil && sharedAuth == nil {
		config.MustBeSet("REDIS_ADDR")
	}

	cfg := loadConfig()
	edgeEnvironment, edgeErr := validation.LoadAndValidateEdgeEnvironment(os.Getenv)
	if edgeErr != nil {
		panic("invalid edge security configuration")
	}
	if edgeEnvironment.Production && (cfg.NowPaymentsIPNSecret == "" || len(cfg.JibitAllowedIPs) == 0) {
		panic("production payment webhook authentication configuration is incomplete")
	}

	// Initialize observability
	ctx := context.Background()
	obs, err := observability.New(ctx, observability.Config{
		Service:              "payment-service",
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

	// Warn if webhook/callback URLs are not configured (required for payment processing)
	if os.Getenv("WEBHOOK_BASE_URL") == "" {
		log.Warn("WEBHOOK_BASE_URL is not set — NOWPayments webhooks will not work")
	}
	if cfg.JibitCallbackURL == "" && cfg.JibitAPIKey != "" {
		log.Warn("JIBIT_CALLBACK_URL is not set but Jibit is configured — Jibit payment callbacks will not work")
	}
	if os.Getenv("SUCCESS_REDIRECT_URL") == "" || os.Getenv("CANCEL_REDIRECT_URL") == "" {
		log.Warn("SUCCESS_REDIRECT_URL or CANCEL_REDIRECT_URL is not set — payment redirect may fail")
	}

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

	// Connect to PostgreSQL
	var pool *db.Pool
	if sharedPool != nil {
		pool = sharedPool
		log.Info("Using shared database pool")
	} else {
		dbMaxOpen := 25
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

	// Connect to Redis (optional)
	var redisClient *pkgredis.Client
	if sharedRedis != nil {
		redisClient = sharedRedis
		log.Info("Using shared Redis client")
	} else if cfg.RedisAddr != "" {
		redisCfg := pkgredis.ConfigFromEnv(os.Getenv)
		if redisCfg.Addr == "" && redisCfg.Mode == pkgredis.ModeStandalone {
			redisCfg.Addr = cfg.RedisAddr
		}
		var redisErr error
		redisClient, redisErr = pkgredis.NewClient(redisCfg)
		if redisErr != nil {
			log.Warn("Failed to create Redis client", zap.Error(redisErr))
		} else {
			redisCtx, redisCancel := context.WithTimeout(context.Background(), 2*time.Second)
			if pingErr := redisClient.Ping(redisCtx).Err(); pingErr != nil {
				log.Warn("Redis connection failed", zap.Error(pingErr))
				redisClient.Close()
				redisClient = nil
			} else {
				log.Info("Connected to Redis", zap.String("mode", string(redisClient.Mode())))
			}
			redisCancel()
		}
	}

	// Initialize auth
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

	// Initialize wallet service
	walletService := wallet.NewService(pool.Primary())

	// Initialize KYC service
	kycService := kyc.NewService(pool.Primary())

	// Initialize payment providers
	providerRegistry := providers.NewProviderRegistry()

	// Register NOWPayments provider
	if cfg.NowPaymentsAPIKey != "" {
		nowPayments := providers.NewNowPayments(providers.NowPaymentsConfig{
			APIKey:    cfg.NowPaymentsAPIKey,
			PublicKey: cfg.NowPaymentsPublicKey,
			IPNSecret: cfg.NowPaymentsIPNSecret,
			BaseURL:   cfg.NowPaymentsBaseURL,
			Sandbox:   cfg.NowPaymentsSandbox,
			Circuit:   circuits.NowPayments,
		})
		providerRegistry.Register(nowPayments)
		log.Info("Registered NOWPayments provider", zap.Bool("sandbox", cfg.NowPaymentsSandbox))
	}

	// Register Jibit PPG provider
	if cfg.JibitAPIKey != "" {
		jibit := providers.NewJibit(providers.JibitConfig{
			APIKey:      cfg.JibitAPIKey,
			SecretKey:   cfg.JibitSecretKey,
			CallbackURL: cfg.JibitCallbackURL,
			BaseURL:     cfg.JibitBaseURL,
			Circuit:     circuits.Jibit,
		})
		providerRegistry.Register(jibit)
		log.Info("Registered Jibit PPG provider")
	}

	// Initialize exchange rate service
	exchangeRateSvc := exchangerate.NewService(exchangerate.Config{
		NobitexBaseURL: cfg.ExchangeRateNobitexURL,
		StaticUSDToIRR: cfg.ExchangeRateStaticUSDIRR,
		CacheTTL:       cfg.ExchangeRateCacheTTL,
		RedisAddr:      cfg.RedisAddr,
	}, log.Logger)

	// Create handlers
	depositConfig := &handlers.DepositConfig{
		MinDepositCents:    cfg.MinDepositCents,
		MaxDepositCents:    cfg.MaxDepositCents,
		MinDepositIRR:      cfg.MinDepositIRR,
		MaxDepositIRR:      cfg.MaxDepositIRR,
		DefaultCurrency:    "USD",
		WebhookBaseURL:     os.Getenv("WEBHOOK_BASE_URL"),
		SuccessRedirectURL: os.Getenv("SUCCESS_REDIRECT_URL"),
		CancelRedirectURL:  os.Getenv("CANCEL_REDIRECT_URL"),
	}
	depositHandler := handlers.NewDepositHandler(pool.Primary(), providerRegistry, exchangeRateSvc, log.Logger, depositConfig, circuits)

	withdrawConfig := &handlers.WithdrawConfig{
		MinWithdrawCents:           cfg.MinWithdrawCents,
		MaxWithdrawCents:           cfg.MaxWithdrawCents,
		WithdrawFeeCents:           cfg.WithdrawFeeCents,
		WithdrawFeePercent:         cfg.WithdrawFeePercent,
		DailyWithdrawAmountCents:   cfg.DailyWithdrawAmountCents,
		MonthlyWithdrawAmountCents: cfg.MonthlyWithdrawAmountCents,
		DailyWithdrawCount:         cfg.DailyWithdrawCount,
		MonthlyWithdrawCount:       cfg.MonthlyWithdrawCount,
	}
	withdrawHandler := handlers.NewWithdrawHandler(pool.Primary(), walletService, kycService, providerRegistry, log.Logger, withdrawConfig, circuits)

	// Initialize email notifier for deposit confirmations (optional)
	var emailNotifier *notification.EmailNotifier
	if resendAPIKey := os.Getenv("RESEND_API_KEY"); resendAPIKey != "" {
		emailCfg := notification.EmailConfig{
			APIKey:    resendAPIKey,
			FromEmail: os.Getenv("EMAIL_FROM"),
			ReplyTo:   os.Getenv("EMAIL_REPLY_TO"),
		}
		var err error
		emailNotifier, err = notification.NewEmailNotifier(emailCfg, log.Logger)
		if err != nil {
			log.Warn("Failed to initialize email notifier, deposit confirmation emails disabled",
				zap.Error(err))
		} else {
			log.Info("Email notifier initialized for deposit confirmations")
		}
	} else {
		log.Info("RESEND_API_KEY not configured, deposit confirmation emails disabled")
	}

	var webhookSecurity *handlers.WebhookSecurity
	if redisClient != nil {
		webhookSecurity, err = handlers.NewWebhookSecurity(redisClient.UniversalClient, getEnvDuration("PAYMENT_WEBHOOK_MAX_AGE", 5*time.Minute), edgeEnvironment.Production, time.Now)
		if err != nil {
			panic("invalid payment webhook replay configuration")
		}
	}
	webhookHandler := handlers.NewWebhookHandler(pool.Primary(), walletService, providerRegistry, emailNotifier, log.Logger, depositConfig.SuccessRedirectURL, depositConfig.CancelRedirectURL, circuits, webhookSecurity)
	historyHandler := handlers.NewHistoryHandler(pool.Primary(), kycService, log.Logger, circuits)

	app := &App{
		pool:            pool,
		redis:           redisClient,
		auth:            authService,
		wallet:          walletService,
		kyc:             kycService,
		providers:       providerRegistry,
		circuits:        circuits,
		config:          cfg,
		obs:             obs,
		exchangeRate:    exchangeRateSvc,
		depositHandler:  depositHandler,
		withdrawHandler: withdrawHandler,
		webhookHandler:  webhookHandler,
		historyHandler:  historyHandler,
	}

	// Start background cleanup worker for orphaned payment intents
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go StartCleanupWorker(cleanupCtx, pool.Primary(), circuits, log.Logger, CleanupConfig{
		Interval:      cfg.CleanupInterval,
		OrphanedAfter: cfg.CleanupOrphanedAfter,
	})

	// Start inquiry worker for stuck Jibit payments (BUG #305)
	inquiryCtx, inquiryCancel := context.WithCancel(context.Background())
	defer inquiryCancel()
	go StartInquiryWorker(inquiryCtx, pool.Primary(), circuits, providerRegistry, walletService, log.Logger, InquiryConfig{
		Interval:  cfg.InquiryInterval,
		MaxAge:    cfg.InquiryMaxAge,
		BatchSize: 50,
	})

	// Start expiry worker for stale payment intents and stuck payouts (BUG #306)
	expiryCtx, expiryCancel := context.WithCancel(context.Background())
	defer expiryCancel()
	go StartExpiryWorker(expiryCtx, pool.Primary(), circuits, providerRegistry, walletService, log.Logger, ExpiryConfig{
		Interval:                 cfg.ExpiryInterval,
		Threshold:                cfg.ExpiryThreshold,
		PayoutThreshold:          cfg.PayoutExpiryThreshold,
		ProcessingAlertThreshold: cfg.ProcessingPayoutAlertThreshold,
		BatchSize:                50,
	})

	// Parse Jibit IP whitelist for webhook security
	jibitCIDRs := handlers.ParseCIDRs(cfg.JibitAllowedIPs, log.Logger)
	if len(jibitCIDRs) > 0 {
		log.Info("Jibit webhook IP whitelist configured",
			zap.Int("cidr_count", len(jibitCIDRs)))
	} else if cfg.JibitAPIKey != "" {
		log.Warn("JIBIT_ALLOWED_IPS not configured — Jibit webhooks accept requests from any IP")
	}
	jibitIPWhitelist := handlers.IPWhitelistMiddleware(jibitCIDRs, log.Logger)

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
	edgePolicy := ratelimit.NewPolicyMiddleware(edgeRedis, ratelimit.PoliciesForService("payment"), nil, func(class ratelimit.EndpointClass, reason string) {
		log.Warn("Edge security request denied", zap.String("policy_class", string(class)), zap.String("reason", reason))
	})

	// Middleware stack
	r.Use(validation.RequestIDMiddleware)
	r.Use(validation.CORSMiddleware(validation.PaymentServiceCORSConfig()))
	r.Use(validation.SecurityHeadersMiddleware)
	r.Use(edgePolicy.Handler)
	r.Use(auth.RedactSecurityCredentialsForTelemetry)
	r.Use(obs.Middleware.Middleware)
	r.Use(sentryHandler.Handle) // Sentry panic capture
	r.Use(auth.RestoreSecurityCredentialsAfterTelemetry)
	r.Use(obs.Middleware.Recovery)
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(validation.MaxBytesMiddleware(edgeEnvironment.DefaultBodyBytes))
	r.Use(validation.ContentTypeMiddleware)

	// Health check endpoints
	r.Get("/healthz", app.handleHealthz)
	r.Get("/readyz", app.handleReadyz)
	r.Get("/health/circuits", app.handleCircuitHealth)
	r.With(validation.InternalOnlyMiddleware).Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		obs.MetricsHandler().ServeHTTP(w, r)
	})

	// Webhook endpoints (public - no auth required)
	registerPaymentWebhookRoutes(r, webhookHandler.HandleNowPaymentsWebhook)

	// Jibit callback endpoint (public - for redirects after payment).
	// Jibit only has a single callbackUrl per purchase; the callback handler
	// verifies the webhook inline and redirects the user to the frontend.
	r.With(jibitIPWhitelist).Get("/callback/jibit", webhookHandler.HandleJibitCallback)
	r.With(jibitIPWhitelist).Post("/callback/jibit", webhookHandler.HandleJibitCallback)

	// Public exchange rate endpoint (no auth required)
	r.Get("/api/payments/exchange-rate", app.handleGetExchangeRate)

	// API routes (protected)
	r.Route("/api/payments", func(r chi.Router) {
		r.Use(app.authMiddleware)
		r.Use(edgePolicy.ActorHandler)

		// Deposit endpoints
		r.Post("/deposit/crypto/create", depositHandler.HandleCreateCryptoDeposit)
		r.Post("/deposit/fiat/create", depositHandler.HandleCreateFiatDeposit)
		r.Get("/deposit/{id}/status", depositHandler.HandleGetDepositStatus)
		r.Get("/status/{purchaseId}", depositHandler.HandleGetPaymentStatusByPurchaseID)

		// Estimate endpoint
		r.Get("/estimate", depositHandler.HandleGetEstimate)

		// Withdraw endpoints
		r.Post("/withdraw/request", withdrawHandler.HandleCreateWithdraw)
		r.Get("/withdraw/{id}/status", withdrawHandler.HandleGetWithdrawStatus)

		// History endpoint
		r.Get("/history", historyHandler.HandleGetHistory)
	})

	// Wallet endpoint (protected)
	r.Route("/api/wallet", func(r chi.Router) {
		r.Use(app.authMiddleware)
		r.Use(edgePolicy.ActorHandler)
		r.Get("/", historyHandler.HandleGetWallet)
	})

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

	// Start server
	infra.SafeGo(log.Logger, "http-server", func() {
		log.Info("Starting payment-service", zap.String("port", cfg.Port))
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

	// Stop background workers
	cleanupCancel()
	inquiryCancel()
	expiryCancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Server forced to shutdown", zap.Error(err))
	}

	log.Info("Server exited")
}

// authMiddleware wraps the auth package middleware.
func (a *App) authMiddleware(next http.Handler) http.Handler {
	return a.auth.Middleware.RequireAuth(next)
}

// handleHealthz is a simple liveness check.
func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz checks database and provider connectivity.
func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	response := map[string]interface{}{
		"status":    "ready",
		"service":   "payment-service",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	httpStatus := http.StatusOK

	// Check circuit breaker health
	if !a.circuits.IsHealthy() {
		response["status"] = "unavailable"
		response["circuits"] = a.circuits.GetHealth()
		response["message"] = "circuit breakers unhealthy"
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	response["circuits"] = "healthy"

	// Check database connectivity
	if err := a.pool.HealthCheck(ctx); err != nil {
		response["status"] = "unavailable"
		response["database"] = "unavailable"
		response["message"] = "database connectivity check failed"
		writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	response["database"] = "healthy"

	// Check payment providers (cached, 30s TTL to avoid excessive external API calls)
	providerStatus := make(map[string]string)
	for _, p := range a.providers.All() {
		if a.providers.IsProviderAvailable(ctx, p.Name()) {
			providerStatus[string(p.Name())] = "available"
		} else {
			providerStatus[string(p.Name())] = "unavailable"
		}
	}
	response["providers"] = providerStatus

	// Check Redis (optional)
	if a.redis != nil {
		if err := a.redis.Ping(ctx).Err(); err != nil {
			response["redis"] = "unavailable"
		} else {
			response["redis"] = "healthy"
		}
	} else {
		response["redis"] = "not_configured"
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

// handleGetExchangeRate returns the current USD to IRR exchange rate.
func (a *App) handleGetExchangeRate(w http.ResponseWriter, r *http.Request) {
	rate, err := a.exchangeRate.GetRate(r.Context())
	if err != nil {
		a.obs.Logger.Logger.Error("Failed to get exchange rate", zap.Error(err))
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "نرخ ارز موقتاً در دسترس نیست",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"usd_to_irr": rate.USDToIRR,
		"usd_to_irt": rate.USDToIRT,
		"source":     rate.Source,
		"fetched_at": rate.FetchedAt.Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
