package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Parsaeffatravesh/tragge/apps/user-bff/internal/service"
	"github.com/Parsaeffatravesh/tragge/packages/audit"
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
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
	"github.com/Parsaeffatravesh/tragge/packages/sms"
	"github.com/Parsaeffatravesh/tragge/packages/storage"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Config holds application configuration.
type Config struct {
	Port                string
	PostgresDSN         string
	PostgresReplicaDSNs []string
	AuthContext         auth.ContextConfig
	RedisAddr           string

	// Rate limiting configuration
	AuthRateLimit         int           // requests per window for auth endpoints
	AuthRateWindow        time.Duration // time window for auth rate limiting
	ContestJoinRateLimit  int           // requests per window for contest join
	ContestJoinRateWindow time.Duration // time window for contest join

	// Security-code delivery configuration. Provider credentials and the HMAC
	// key are separate trust domains and support Docker/Kubernetes _FILE loading.
	SecurityCodeHashSecret string
	MailerinoAPIKey        string
	MailerinoFrom          string
	MailerinoBaseURL       string
	ResendAPIKey           string
	EmailFrom              string
	ResendBaseURL          string
	EmailFromAmbiguous     bool
	KaveNegarAPIKey        string
	SMSEnabled             bool
	SMSProviderMode        string
	SMSSender              string
	SMSTemplate            string
	FrontendURL            string

	// Google OAuth configuration
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	// Jibit KYC configuration
	JibitAPIKey    string
	JibitSecretKey string
	JibitBaseURL   string

	// S3/MinIO storage configuration
	S3Endpoint     string
	S3AccessKey    string
	S3SecretKey    string
	S3Region       string
	S3Bucket       string
	S3UseSSL       bool
	S3PublicURL    string
	S3KYCBucket    string
	S3TicketBucket string

	// TOTP encryption key (32-byte AES-256 key for encrypting TOTP secrets at rest)
	TOTPEncryptionKey []byte

	// ARCaptcha configuration (invisible CAPTCHA for registration)
	ARCaptchaSiteKey   string
	ARCaptchaSecretKey string
}

// contestsCache provides simple in-memory caching for contests list.
type contestsCache struct {
	mu        sync.RWMutex
	data      []ContestResponse
	cachedAt  time.Time
	expiresAt time.Time
}

// App holds application dependencies.
type App struct {
	pool          *db.Pool
	redis         *pkgredis.Client
	auth          *auth.Auth
	wallet        *wallet.Service
	email         *notification.EmailNotifier // non-security notifications only
	securityEmail securityEmailSender
	codeHasher    *securityCodeHasher
	codeClock     securityCodeClock
	config        *Config
	contestsCache *contestsCache
	obs           *observability.Observability
	circuits      *CircuitBreakers
	oauthService  *service.OAuthService // OAuth user operations
	jibitKYC      *kyc.JibitKYCProvider // Jibit identity verification
	objectStorage storage.ObjectStore   // S3/MinIO object storage for avatar uploads
	kycStorage    storage.ObjectStore   // S3/MinIO object storage for KYC documents (private bucket)

	// TOTP encryption key (32-byte AES-256 key for encrypting TOTP secrets at rest)
	totpEncryptionKey []byte

	// SMS OTP service for phone-based authentication
	otpService *sms.OTPService

	// JWT token blacklist for immediate revocation on logout
	tokenBlacklist *auth.TokenBlacklist

	// Security audit logger
	auditLogger *audit.Logger

	// Rate limiters
	authLimiter                  *ratelimit.UserRateLimiter    // IP-based limiter for auth endpoints
	contestJoinLimiter           *ratelimit.UserRateLimiter    // User-based limiter for contest join
	failedLoginTracker           *failedLoginTracker           // Tracks failed login attempts for progressive delays
	distributedLoginLockout      *ratelimit.LoginLockout       // Redis-backed IP/account lockout
	emailVerificationRateLimiter *emailVerificationRateLimiter // Rate limiter for email verification resend requests
	verifyCodeRateLimiter        *verifyCodeRateLimiter        // Rate limiter for verify-email code attempts
	passwordChangeRateLimiter    *passwordChangeRateLimiter    // Rate limiter for password change requests
}

// failedLoginTracker tracks failed login attempts per IP for progressive delays.
// This provides brute-force protection beyond simple rate limiting.
type failedLoginTracker struct {
	mu       sync.RWMutex
	attempts map[string]*loginAttemptInfo
	done     chan struct{}
}

type loginAttemptInfo struct {
	count       int
	lastFailed  time.Time
	lockedUntil time.Time
}

// newFailedLoginTracker creates a new failed login tracker.
func newFailedLoginTracker() *failedLoginTracker {
	tracker := &failedLoginTracker{
		attempts: make(map[string]*loginAttemptInfo),
		done:     make(chan struct{}),
	}
	// Start cleanup goroutine
	go tracker.cleanupLoop()
	return tracker
}

// recordFailure records a failed login attempt and returns the delay to apply.
func (t *failedLoginTracker) recordFailure(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	info, exists := t.attempts[key]
	if !exists {
		info = &loginAttemptInfo{}
		t.attempts[key] = info
	}

	info.count++
	info.lastFailed = time.Now()

	// Progressive delays based on failure count:
	// 1-2 failures: no delay
	// 3-4 failures: 1 second delay
	// 5-6 failures: 5 seconds delay
	// 7-9 failures: 15 seconds delay
	// 10+ failures: 30 seconds lockout
	var delay time.Duration
	switch {
	case info.count < 3:
		delay = 0
	case info.count < 5:
		delay = 1 * time.Second
	case info.count < 7:
		delay = 5 * time.Second
	case info.count < 10:
		delay = 15 * time.Second
	default:
		delay = 30 * time.Second
	}

	if delay > 0 {
		info.lockedUntil = time.Now().Add(delay)
	}

	return delay
}

// checkLocked checks if the key is currently locked out.
func (t *failedLoginTracker) checkLocked(key string) (bool, time.Duration) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	info, exists := t.attempts[key]
	if !exists {
		return false, 0
	}

	if info.lockedUntil.After(time.Now()) {
		return true, time.Until(info.lockedUntil)
	}
	return false, 0
}

// recordSuccess clears the failure count on successful login.
func (t *failedLoginTracker) recordSuccess(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.attempts, key)
}

// cleanupLoop periodically removes old entries.
func (t *failedLoginTracker) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.cleanup()
		case <-t.done:
			return
		}
	}
}

// stop signals the cleanup goroutine to exit.
func (t *failedLoginTracker) stop() { close(t.done) }

// cleanup removes entries older than 1 hour.
func (t *failedLoginTracker) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	threshold := time.Now().Add(-1 * time.Hour)
	for key, info := range t.attempts {
		if info.lastFailed.Before(threshold) {
			delete(t.attempts, key)
		}
	}
}

// emailVerificationRateLimiter limits email verification resend requests per user.
// Max 3 requests per user per hour.
type emailVerificationRateLimiter struct {
	mu       sync.RWMutex
	requests map[string][]time.Time
	done     chan struct{}
}

// newEmailVerificationRateLimiter creates a new email verification rate limiter.
func newEmailVerificationRateLimiter() *emailVerificationRateLimiter {
	limiter := &emailVerificationRateLimiter{
		requests: make(map[string][]time.Time),
		done:     make(chan struct{}),
	}
	go limiter.cleanupLoop()
	return limiter
}

// isAllowedWithRetry checks if a resend verification request is allowed for the given user ID.
// Returns (allowed, retryAfterSeconds).
func (e *emailVerificationRateLimiter) isAllowedWithRetry(userID string) (bool, int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

	// Clean up old requests for this user
	var recentRequests []time.Time
	for _, t := range e.requests[userID] {
		if t.After(oneHourAgo) {
			recentRequests = append(recentRequests, t)
		}
	}
	e.requests[userID] = recentRequests

	// Check if under limit (3 per hour)
	if len(recentRequests) >= 3 {
		// Find oldest recent request — next allowed is oldest + 1 hour
		oldest := recentRequests[0]
		for _, t := range recentRequests {
			if t.Before(oldest) {
				oldest = t
			}
		}
		retryAfter := int(time.Until(oldest.Add(1 * time.Hour)).Seconds())
		if retryAfter < 0 {
			retryAfter = 0
		}
		return false, retryAfter
	}

	// Record this request
	e.requests[userID] = append(e.requests[userID], now)
	return true, 0
}

// cleanupLoop periodically removes old entries.
func (e *emailVerificationRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.cleanup()
		case <-e.done:
			return
		}
	}
}

// stop signals the cleanup goroutine to exit.
func (e *emailVerificationRateLimiter) stop() { close(e.done) }

// cleanup removes entries older than 1 hour.
func (e *emailVerificationRateLimiter) cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()

	oneHourAgo := time.Now().Add(-1 * time.Hour)
	for userID, requests := range e.requests {
		var recentRequests []time.Time
		for _, t := range requests {
			if t.After(oneHourAgo) {
				recentRequests = append(recentRequests, t)
			}
		}
		if len(recentRequests) == 0 {
			delete(e.requests, userID)
		} else {
			e.requests[userID] = recentRequests
		}
	}
}

// verifyCodeRateLimiter limits verify-email attempts per user.
// Max 10 attempts per user per minute.
type verifyCodeRateLimiter struct {
	mu       sync.RWMutex
	requests map[string][]time.Time
	done     chan struct{}
}

func newVerifyCodeRateLimiter() *verifyCodeRateLimiter {
	limiter := &verifyCodeRateLimiter{
		requests: make(map[string][]time.Time),
		done:     make(chan struct{}),
	}
	go limiter.cleanupLoop()
	return limiter
}

func (v *verifyCodeRateLimiter) isAllowed(userID string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	var recent []time.Time
	for _, t := range v.requests[userID] {
		if t.After(oneMinuteAgo) {
			recent = append(recent, t)
		}
	}
	v.requests[userID] = recent

	if len(recent) >= 10 {
		return false
	}

	v.requests[userID] = append(v.requests[userID], now)
	return true
}

func (v *verifyCodeRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			v.cleanup()
		case <-v.done:
			return
		}
	}
}

func (v *verifyCodeRateLimiter) stop() { close(v.done) }

func (v *verifyCodeRateLimiter) cleanup() {
	v.mu.Lock()
	defer v.mu.Unlock()
	oneMinuteAgo := time.Now().Add(-1 * time.Minute)
	for userID, requests := range v.requests {
		var recent []time.Time
		for _, t := range requests {
			if t.After(oneMinuteAgo) {
				recent = append(recent, t)
			}
		}
		if len(recent) == 0 {
			delete(v.requests, userID)
		} else {
			v.requests[userID] = recent
		}
	}
}

// passwordChangeRateLimiter limits password change requests per user.
// Max 5 requests per user per hour.
type passwordChangeRateLimiter struct {
	mu       sync.RWMutex
	requests map[string][]time.Time
	done     chan struct{}
}

// newPasswordChangeRateLimiter creates a new password change rate limiter.
func newPasswordChangeRateLimiter() *passwordChangeRateLimiter {
	limiter := &passwordChangeRateLimiter{
		requests: make(map[string][]time.Time),
		done:     make(chan struct{}),
	}
	go limiter.cleanupLoop()
	return limiter
}

// isAllowed checks if a password change request is allowed for the given user ID.
// Returns true if allowed, false if rate limited.
func (p *passwordChangeRateLimiter) isAllowed(userID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

	// Clean up old requests for this user
	var recentRequests []time.Time
	for _, t := range p.requests[userID] {
		if t.After(oneHourAgo) {
			recentRequests = append(recentRequests, t)
		}
	}
	p.requests[userID] = recentRequests

	// Check if under limit (5 per hour)
	if len(recentRequests) >= 5 {
		return false
	}

	// Record this request
	p.requests[userID] = append(p.requests[userID], now)
	return true
}

// cleanupLoop periodically removes old entries.
func (p *passwordChangeRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanup()
		case <-p.done:
			return
		}
	}
}

// stop signals the cleanup goroutine to exit.
func (p *passwordChangeRateLimiter) stop() { close(p.done) }

// cleanup removes entries older than 1 hour.
func (p *passwordChangeRateLimiter) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	oneHourAgo := time.Now().Add(-1 * time.Hour)
	for userID, requests := range p.requests {
		var recentRequests []time.Time
		for _, t := range requests {
			if t.After(oneHourAgo) {
				recentRequests = append(recentRequests, t)
			}
		}
		if len(recentRequests) == 0 {
			delete(p.requests, userID)
		} else {
			p.requests[userID] = recentRequests
		}
	}
}

// Run starts the user-bff service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil, nil)
}

// RunWithSharedDeps starts the user-bff service, optionally using shared resources.
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

	securityEnvironment, err := resolveSecurityEnvironment(os.Getenv("ENVIRONMENT"), os.Getenv("APP_ENV"))
	if err != nil {
		log.Fatal("FATAL: ambiguous security-code environment configuration")
	}
	cfg := loadConfig()
	edgeEnvironment, edgeErr := validation.LoadAndValidateEdgeEnvironment(os.Getenv)
	if edgeErr != nil {
		log.Fatal("invalid edge security configuration")
	}
	if err := validateSecurityDeliveryConfig(securityEnvironment, cfg); err != nil {
		log.Fatal("FATAL: invalid security-code delivery configuration")
	}

	// Initialize observability
	ctx := context.Background()
	obs, err := observability.New(ctx, observability.Config{
		Service:              "user-bff",
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
	dbPoolMetrics := observability.NewDBPoolMetrics(obs.Metrics.Registry(), "user_bff")
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
			redisCtx, redisCancel := context.WithTimeout(context.Background(), 2*time.Second)
			if pingErr := redisClient.Ping(redisCtx).Err(); pingErr != nil {
				log.Warn("Redis connection failed, session management will be disabled", zap.Error(pingErr))
				redisClient.Close()
				redisClient = nil
			} else {
				log.Info("Connected to Redis",
					zap.String("mode", string(redisClient.Mode())))
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

	// Seed default admin and test users on startup
	seedAdminUsers(ctx, pool.Primary(), log)

	// Initialize wallet service
	walletService := wallet.NewService(pool.Primary())

	// Initialize rate limiters
	authLimiter := ratelimit.NewUserLimiter(ratelimit.Config{
		Rate:      cfg.AuthRateLimit,
		Window:    cfg.AuthRateWindow,
		BurstSize: cfg.AuthRateLimit, // No burst allowed for auth
		KeyPrefix: "rl:auth:user:",
	})

	contestJoinLimiter := ratelimit.NewUserLimiter(ratelimit.Config{
		Rate:      cfg.ContestJoinRateLimit,
		Window:    cfg.ContestJoinRateWindow,
		BurstSize: cfg.ContestJoinRateLimit / 2, // Allow some burst
		KeyPrefix: "rl:join:",
	})

	log.Info("Rate limiters initialized",
		zap.Int("auth_rate", cfg.AuthRateLimit),
		zap.Duration("auth_window", cfg.AuthRateWindow),
		zap.Int("contest_join_rate", cfg.ContestJoinRateLimit),
		zap.Duration("contest_join_window", cfg.ContestJoinRateWindow))

	codeHasher, err := newSecurityCodeHasher(cfg.SecurityCodeHashSecret)
	if err != nil {
		log.Fatal("Invalid security-code hashing configuration", zap.Error(err))
	}

	// Security email uses explicit country routing and provider-neutral adapters.
	// No provider is synthesized when credentials are absent.
	var securityEmail securityEmailSender
	if cfg.MailerinoAPIKey != "" && cfg.ResendAPIKey != "" {
		mailerinoProvider, providerErr := notification.NewMailerinoSecurityEmailProvider(notification.SecurityEmailHTTPConfig{
			BaseURL: cfg.MailerinoBaseURL,
			APIKey:  cfg.MailerinoAPIKey,
			From:    cfg.MailerinoFrom,
		})
		if providerErr != nil {
			log.Fatal("Invalid Mailerino security email configuration")
		}
		resendProvider, providerErr := notification.NewResendSecurityEmailProvider(notification.SecurityEmailHTTPConfig{
			BaseURL: cfg.ResendBaseURL,
			APIKey:  cfg.ResendAPIKey,
			From:    cfg.EmailFrom,
		})
		if providerErr != nil {
			log.Fatal("Invalid Resend security email configuration")
		}
		securityEmail = &countrySecurityEmailRouter{mailerino: mailerinoProvider, resend: resendProvider}
		log.Info("Security email providers initialized")
	} else {
		log.Warn("Security email providers are incomplete; security email delivery is unavailable")
	}

	// Initialize the optional operational email notifier. Security-code delivery
	// uses the fail-closed, country-routed providers constructed above.
	var emailNotifier *notification.EmailNotifier
	if cfg.ResendAPIKey != "" {
		var err error
		emailNotifier, err = notification.NewEmailNotifier(notification.EmailConfig{
			APIKey:    cfg.ResendAPIKey,
			FromEmail: cfg.EmailFrom,
		}, log.Logger)
		if err != nil {
			log.Warn("Failed to initialize operational email notifier",
				zap.Error(err))
		} else {
			log.Info("Email notifier initialized",
				zap.String("from", cfg.EmailFrom))
		}
	} else {
		log.Warn("RESEND_API_KEY not configured; operational email notifications are unavailable")
	}

	// Initialize OAuth service
	oauthService := service.NewOAuthService(pool, log.Logger)

	// Initialize Jibit KYC provider (optional - graceful degradation if not configured)
	var jibitKYCProvider *kyc.JibitKYCProvider
	if cfg.JibitAPIKey != "" && cfg.JibitSecretKey != "" {
		jibitKYCProvider = kyc.NewJibitKYCProvider(kyc.JibitKYCConfig{
			APIKey:    cfg.JibitAPIKey,
			SecretKey: cfg.JibitSecretKey,
			BaseURL:   cfg.JibitBaseURL,
		})
		log.Info("Jibit KYC provider initialized")
	} else {
		log.Warn("JIBIT_API_KEY or JIBIT_SECRET_KEY not configured, Jibit KYC verification will be unavailable")
	}

	// Initialize object storage for avatar uploads. storage.New auto-selects
	// the local-filesystem backend in dev (STORAGE_BACKEND=local) and S3/MinIO
	// in production. Avatar bucket is public (PublicURL set).
	var objectStore storage.ObjectStore
	avatarStore, err := storage.New(ctx, storage.Config{
		Endpoint:        cfg.S3Endpoint,
		AccessKeyID:     cfg.S3AccessKey,
		SecretAccessKey: cfg.S3SecretKey,
		Region:          cfg.S3Region,
		Bucket:          cfg.S3Bucket,
		UseSSL:          cfg.S3UseSSL,
		PublicURL:       cfg.S3PublicURL,
	})
	if err != nil {
		log.Warn("Failed to initialize avatar storage, avatar uploads will be unavailable", zap.Error(err))
	} else {
		objectStore = avatarStore
		log.Info("Avatar storage initialized",
			zap.String("backend", storage.BackendName(avatarStore)),
			zap.String("bucket", cfg.S3Bucket))
	}

	// Initialize object storage for KYC documents. Private=true keeps the S3
	// bucket policy private; the local backend serves only via authenticated
	// admin handlers, so the flag is a no-op there.
	var kycStore storage.ObjectStore
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
		log.Warn("Failed to initialize KYC storage, KYC uploads will be unavailable", zap.Error(err))
	} else {
		kycStore = kycObjectStore
		log.Info("KYC storage initialized",
			zap.String("backend", storage.BackendName(kycObjectStore)),
			zap.String("bucket", cfg.S3KYCBucket))
	}

	var distributedLoginLockout *ratelimit.LoginLockout
	if redisClient != nil {
		distributedLoginLockout, err = ratelimit.NewLoginLockout(redisClient.UniversalClient, ratelimit.LockoutConfig{
			Namespace: "user", Threshold: 8, LockFor: 15 * time.Minute, Retention: time.Hour,
		})
		if err != nil {
			log.Fatal("invalid User login lockout configuration")
		}
	}

	app := &App{
		pool:                         pool,
		redis:                        redisClient,
		auth:                         authService,
		wallet:                       walletService,
		email:                        emailNotifier,
		securityEmail:                securityEmail,
		codeHasher:                   codeHasher,
		codeClock:                    systemSecurityCodeClock{},
		config:                       cfg,
		contestsCache:                &contestsCache{},
		obs:                          obs,
		circuits:                     circuits,
		oauthService:                 oauthService,
		jibitKYC:                     jibitKYCProvider,
		objectStorage:                objectStore,
		kycStorage:                   kycStore,
		totpEncryptionKey:            cfg.TOTPEncryptionKey,
		authLimiter:                  authLimiter,
		contestJoinLimiter:           contestJoinLimiter,
		failedLoginTracker:           newFailedLoginTracker(),
		distributedLoginLockout:      distributedLoginLockout,
		emailVerificationRateLimiter: newEmailVerificationRateLimiter(),
		verifyCodeRateLimiter:        newVerifyCodeRateLimiter(),
		passwordChangeRateLimiter:    newPasswordChangeRateLimiter(),
	}

	// Initialize security audit logger
	app.auditLogger = audit.New(audit.Config{
		DB:     pool.Primary(),
		Logger: log.Logger,
	})
	defer app.auditLogger.Shutdown()

	// Set up JWT blacklist for immediate token revocation on logout
	if redisClient != nil && redisClient.Client() != nil {
		app.tokenBlacklist = auth.NewTokenBlacklistWithPrefix(redisClient.Client(), auth.UserRevocationPrefix)
		app.auth.Middleware.SetTokenBlacklist(app.tokenBlacklist)
	}

	// P0-5: Set up DB-backed password_changed_at check for session invalidation fallback
	app.auth.Middleware.SetPasswordChangedAtFunc(func(ctx context.Context, userID string) (*time.Time, error) {
		var pwChangedAt sql.NullTime
		err := app.pool.Replica().QueryRowContext(ctx,
			`SELECT password_changed_at FROM users WHERE id = $1`, userID,
		).Scan(&pwChangedAt)
		if err != nil || !pwChangedAt.Valid {
			return nil, err
		}
		return &pwChangedAt.Time, nil
	})

	// Initialize SMS OTP service (KaveNegar only; no mock/logging fallback).
	if redisClient != nil && cfg.SMSEnabled && cfg.KaveNegarAPIKey != "" {
		smsProvider := &cbSMSProvider{
			inner: sms.NewKaveNegar(sms.Config{
				APIKey:   cfg.KaveNegarAPIKey,
				Sender:   cfg.SMSSender,
				Template: cfg.SMSTemplate,
				Enabled:  true,
			}),
			circuits: app.circuits,
		}
		otpService, otpErr := sms.NewOTPService(smsProvider, redisClient.Client(), sms.DefaultOTPConfig(cfg.SecurityCodeHashSecret))
		if otpErr != nil {
			log.Fatal("Invalid SMS OTP configuration", zap.Error(otpErr))
		}
		app.otpService = otpService
		log.Info("SMS provider initialized (KaveNegar only)")
	} else if cfg.SMSEnabled {
		log.Warn("SMS delivery enabled but unavailable; OTP endpoints will fail closed")
	}

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
	edgePolicy := ratelimit.NewPolicyMiddleware(edgeRedis, ratelimit.PoliciesForService("user"), nil, func(class ratelimit.EndpointClass, reason string) {
		log.Warn("Edge security request denied", zap.String("policy_class", string(class)), zap.String("reason", reason))
	})

	// Middleware stack (order matters)
	r.Use(validation.RequestIDMiddleware)                            // Request ID tracking
	r.Use(validation.CORSMiddleware(validation.UserBFFCORSConfig())) // CORS handling
	r.Use(validation.CSRFMiddleware(validation.UserBFFCSRFConfig())) // CSRF protection
	r.Use(validation.SecurityHeadersMiddleware)                      // Security headers
	r.Use(edgePolicy.Handler)                                        // Distributed edge abuse controls
	r.Use(auth.RedactSecurityCredentialsForTelemetry)                // Hide session credentials from telemetry
	r.Use(obs.Middleware.Middleware)                                 // Observability (logging, tracing)
	r.Use(sentryHandler.Handle)                                      // Sentry panic capture
	r.Use(auth.RestoreSecurityCredentialsAfterTelemetry)             // Restore secure headers for auth handlers
	r.Use(obs.Middleware.Recovery)                                   // Sanitized panic recovery
	r.Use(middleware.Timeout(30 * time.Second))                      // Request timeout
	r.Use(validation.ContentTypeMiddleware)
	// Note: MaxBytesMiddleware is applied per-route to allow larger file uploads for KYC
	r.Use(validation.SanitizeFormMiddleware) // Input sanitization

	// Default body size limit middleware (applied to most routes)
	defaultBodyLimit := validation.MaxBytesMiddleware(edgeEnvironment.DefaultBodyBytes)
	// Larger body size limit for file upload routes
	fileUploadBodyLimit := validation.MaxBytesMiddleware(edgeEnvironment.UploadBodyBytes)

	// Health check endpoints (no body limit needed)
	r.Get("/healthz", app.handleHealthz)
	r.Get("/api/user/healthz", app.handleHealthz)
	r.Get("/readyz", app.handleReadyz)
	r.Get("/health/circuits", app.handleCircuitHealth)
	r.With(validation.InternalOnlyMiddleware).Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		obs.MetricsHandler().ServeHTTP(w, r)
	})

	// KYC submit route with larger body size limit (must be defined before /api/user to take precedence)
	// This route needs to accept files up to 35MB total (3 files × 10MB + overhead)
	r.Route("/api/user/kyc/submit", func(r chi.Router) {
		r.Use(fileUploadBodyLimit)
		r.Use(app.authMiddleware)
		r.Post("/", app.handleKYCSubmit)
	})

	// Jibit KYC verification routes (must be defined before /api/user to take precedence)
	r.Route("/api/user/kyc", func(r chi.Router) {
		r.Use(app.authMiddleware)

		// KYC status check
		r.Get("/status", app.handleKYCStatus)

		// Step 1: Shahkar phone+national code verification (JSON body, default limit)
		r.With(defaultBodyLimit).Post("/verify-phone", app.handleKYCVerifyPhone)

		// Step 2: Biometric face verification (multipart with selfie image)
		r.With(fileUploadBodyLimit).Post("/verify-face", app.handleKYCVerifyFace)

		// Step 3: National card OCR (multipart with card image)
		r.With(fileUploadBodyLimit).Post("/verify-card", app.handleKYCVerifyCard)
	})

	// Support ticket routes with larger body size limit for file uploads (must be defined before /api/user)
	r.Route("/api/user/me/tickets", func(r chi.Router) {
		r.Use(fileUploadBodyLimit)
		r.Use(app.authMiddleware)
		r.Post("/", app.handleCreateTicket_Support)
		r.Get("/", app.handleListUserTickets)
		r.Get("/unread-count", app.handleUnreadTicketCount)
		r.With(validation.ValidatePathUUID("attachmentId")).Get("/attachment/{attachmentId}", app.handleGetTicketAttachment)
		r.Route("/{ticketId}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("ticketId"))
			r.Get("/", app.handleGetTicketDetail)
			r.Post("/messages", app.handleSendTicketMessage)
			r.Post("/close", app.handleCloseTicket_Support)
		})
	})

	// API routes (with default 1MB body limit)
	r.Route("/api/user", func(r chi.Router) {
		r.Use(defaultBodyLimit) // Apply default 1MB body limit to all /api/user routes

		// Auth routes (public) - with IP-based rate limiting
		r.Group(func(r chi.Router) {
			// Apply rate limiting middleware for auth endpoints
			authRateLimitMiddleware := ratelimit.NewMiddleware(
				app.authLimiter,
				ratelimit.WithKeyExtractor(ratelimit.IPExtractor),
				ratelimit.WithLimitHitHandler(app.handleAuthRateLimitExceeded),
			)
			r.Use(authRateLimitMiddleware.Handler)

			r.Post("/auth/register", app.handleRegister)
			r.Post("/auth/login", app.handleLogin)
			r.Post("/auth/refresh", app.handleRefresh)
			// OTP-based password reset (3-step flow)
			r.Post("/auth/forgot-password/request", app.handleForgotPasswordRequest)
			r.Post("/auth/forgot-password/verify", app.handleForgotPasswordVerify)
			r.Post("/auth/forgot-password/reset", app.handleForgotPasswordReset)

			// Google OAuth routes
			r.Get("/auth/google", app.handleGoogleAuth)
			r.Post("/auth/google/callback", app.handleGoogleCallback)

			// Auth ticket exchange (public - ticket is the credential)
			r.Post("/auth/exchange-ticket", app.handleExchangeTicket)

			// 2FA login verification (public - ticket is the credential)
			r.Post("/auth/2fa/login", app.handle2FALoginVerify)

			// Phone OTP authentication
			r.Post("/auth/send-otp", app.handleSendOTP)
			r.Post("/auth/verify-otp", app.handleVerifyOTP)
			r.Post("/auth/register-phone", app.handleRegisterWithPhone)
		})

		// Public contest listing (cacheable)
		r.Get("/contests", app.handleListContests)
		r.Get("/contests/free", app.handleListFreeContests)
		r.Get("/contests/calendar", app.handleCalendar)
		r.Route("/contests/{id}", func(r chi.Router) {
			r.Use(validation.ValidatePathUUID("id"))
			r.Get("/", app.handleGetContestDetails)
			r.Get("/participants", app.handleGetContestParticipants)
			r.Get("/leaderboard", app.handleGetContestLeaderboard)
			r.Get("/prize-preview", app.handlePrizePreview)

			// Contest trade history: public for completed contests, self-only for active contests
			r.Group(func(r chi.Router) {
				r.Use(app.optionalAuthMiddleware)
				r.Get("/trades/{userId}", app.handleGetContestUserTrades)
			})
		})

		// Tournament API aliases (Task 8.1, 8.2)
		// These route to enhanced handlers with cursor pagination, sorting, Redis caching, and IRST/Jalali times
		r.Group(func(r chi.Router) {
			r.Use(app.optionalAuthMiddleware)
			r.Get("/tournaments", app.handleListTournaments)
			r.Get("/tournaments/calendar", app.handleTournamentCalendar)
			r.With(validation.ValidatePathUUID("id")).Get("/tournaments/{id}", app.handleGetTournamentDetails)
		})

		// Public user stats and leaderboard
		r.With(validation.ValidatePathUUID("userId")).Get("/stats/{userId}", app.handleGetUserStats)
		r.Get("/global-leaderboard", app.handleGlobalLeaderboard)
		r.With(validation.ValidatePathUUID("userId")).Get("/score-history/{userId}", app.handleGetScoreHistory)

		// Public referral validation
		r.Get("/referral/validate", app.handleValidateReferral)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(app.authMiddleware)
			r.Use(edgePolicy.ActorHandler)
			// Short-lived auth ticket for cross-origin navigation
			r.Post("/auth/ticket", app.handleCreateTicket)

			r.Get("/me", app.handleMe)
			r.Put("/me/profile", app.handleUpdateProfile)
			r.Post("/me/avatar", app.handleUploadAvatar)
			r.Post("/me/avatar/select", app.handleSelectAvatar)
			r.Get("/avatars", app.handleListAvatars)
			r.Put("/me/password", app.handleChangePassword)
			r.Post("/auth/verify-email", app.handleVerifyEmail)
			r.Post("/auth/resend-verification", app.handleResendVerification)
			r.Post("/auth/send-verification", app.handleSendVerification)
			r.Post("/auth/verify-code", app.handleVerifyCode)
			r.Get("/me/history", app.handleContestHistory)
			r.Get("/me/contest-history", app.handleContestHistory)
			r.Get("/me/tournaments", app.handleMyTournaments)
			r.Get("/me/contest-stats", app.handleGetContestStats)
			r.Get("/me/stats", app.handleGetMyStats)
			r.Get("/me/score-history", app.handleGetMyScoreHistory)

			// Referral code application (for OAuth signups)
			r.Post("/referral/apply", app.handleApplyReferral)
			r.Get("/leaderboard", app.handleLeaderboard)

			// Contest actions requiring auth (join, leave, results).
			// Registered as individual routes to avoid remounting /contests/{id}
			// which is already mounted above for public handlers.
			contestJoinRateLimitMiddleware := ratelimit.NewMiddleware(
				app.contestJoinLimiter,
				ratelimit.WithKeyExtractor(ratelimit.UserIDExtractor),
				ratelimit.WithLimitHitHandler(app.handleContestJoinRateLimitExceeded),
			)
			contestIDValidator := validation.ValidatePathUUID("id")
			r.With(contestIDValidator, contestJoinRateLimitMiddleware.Handler).Post("/contests/{id}/join", app.handleJoinContest)
			r.With(contestIDValidator).Post("/contests/{id}/leave", app.handleLeaveContest)
			r.With(contestIDValidator).Get("/contests/{id}/my-result", app.handleGetContestMyResult)
			r.With(contestIDValidator).Get("/contests/{id}/my-trades", app.handleGetContestMyTrades)

			// Session management routes
			r.Post("/logout", app.handleLogout)
			r.Get("/me/sessions", app.handleGetSessions)
			r.With(validation.ValidatePathUUID("session_id")).Delete("/me/sessions/{session_id}", app.handleDeleteSession)
			r.Delete("/me/sessions", app.handleDeleteSessions)

			// P2-P3-4: Two-Factor Authentication (2FA) routes
			r.Post("/me/2fa/setup", app.handle2FASetup)
			r.Post("/me/2fa/verify", app.handle2FAVerify)
			r.Post("/me/2fa/disable", app.handle2FADisable)
			r.Get("/me/2fa/status", app.handle2FAStatus)

			// Wallet routes
			r.Get("/wallet", app.handleGetWallet)
			r.Get("/wallet/history", app.handleGetWalletHistory)

			// Affiliate routes
			r.Get("/affiliate", app.handleGetAffiliateStats)
			r.Get("/affiliate/referrals", app.handleGetAffiliateReferrals)

			// Affiliate activation routes
			r.Get("/me/affiliate/status", app.handleGetAffiliateStatus)
			r.Post("/me/affiliate/request-activation", app.handleRequestAffiliateActivation)

			// In-app notification routes
			r.Get("/me/notifications", app.handleGetNotifications)
			r.Get("/me/notifications/unread-count", app.handleGetUnreadNotificationCount)
			r.With(validation.ValidatePathUUID("id")).Post("/me/notifications/{id}/read", app.handleMarkNotificationRead)
			r.Post("/me/notifications/read-all", app.handleMarkAllNotificationsRead)
			r.With(validation.ValidatePathUUID("id")).Delete("/me/notifications/{id}", app.handleDeleteNotification)

			// Notification preferences
			r.Get("/me/notification-preferences", app.handleGetNotificationPreferences)
			r.Put("/me/notification-preferences", app.handleUpdateNotificationPreferences)

			// Support tickets are defined outside /api/user with fileUploadBodyLimit
		})
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

	// Start periodic cleanup of expired email verification tokens
	app.startTokenCleanup()

	// Startup check: warn if any TOTP secrets are stored in plaintext
	infra.SafeGo(log.Logger, "totp-plaintext-check", func() {
		var plaintextCount int
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer checkCancel()
		err := pool.Replica().QueryRowContext(checkCtx,
			`SELECT count(*) FROM users WHERE totp_secret IS NOT NULL AND totp_secret NOT LIKE 'enc:%' AND totp_enabled = true`,
		).Scan(&plaintextCount)
		if err != nil {
			log.Warn("Failed to check for plaintext TOTP secrets", zap.Error(err))
		} else if plaintextCount > 0 {
			log.Error("SECURITY: Found plaintext TOTP secrets in database — set TOTP_ENCRYPTION_KEY and re-encrypt",
				zap.Int("count", plaintextCount))
		}
	})

	// Start server in goroutine
	go func() {
		log.Info("Starting user-bff", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("Server error", zap.Error(err))
		}
	}()

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

	// Cleanup rate limiters
	if err := authLimiter.Close(); err != nil {
		log.Error("Failed to close auth limiter", zap.Error(err))
	}
	if err := contestJoinLimiter.Close(); err != nil {
		log.Error("Failed to close contest join limiter", zap.Error(err))
	}

	// Stop custom rate limiter cleanup goroutines
	app.failedLoginTracker.stop()
	app.emailVerificationRateLimiter.stop()
	app.verifyCodeRateLimiter.stop()
	app.passwordChangeRateLimiter.stop()

	// Stop user cache cleanup goroutine
	app.circuits.userCache.Stop()

	log.Info("Server exited")
}

// startTokenCleanup runs a periodic job to clean up old email verification tokens.
func (a *App) startTokenCleanup() {
	infra.SafeGo(a.log(), "email-token-cleanup", func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.cleanupExpiredTokens()
			case <-a.emailVerificationRateLimiter.done:
				// Reuse the done channel as a shutdown signal
				return
			}
		}
	})
}

func (a *App) cleanupExpiredTokens() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Delete old email_verification_tokens (legacy table)
	result, err := a.pool.Primary().ExecContext(ctx,
		`DELETE FROM email_verification_tokens
		 WHERE (expires_at < NOW() OR used_at IS NOT NULL)
		 AND created_at < NOW() - INTERVAL '24 hours'`,
	)
	if err != nil {
		a.log().Error("Failed to cleanup expired email verification tokens", zap.Error(err))
	} else if rowsAffected, _ := result.RowsAffected(); rowsAffected > 0 {
		a.log().Info("Cleaned up expired email verification tokens",
			zap.Int64("deleted", rowsAffected))
	}

	// Delete old verification_codes (unified table)
	result2, err := a.pool.Primary().ExecContext(ctx,
		`DELETE FROM verification_codes WHERE expires_at < NOW() - INTERVAL '24 hours'`,
	)
	if err != nil {
		a.log().Error("Failed to cleanup expired verification codes", zap.Error(err))
	} else if rowsAffected, _ := result2.RowsAffected(); rowsAffected > 0 {
		a.log().Info("Cleaned up expired verification codes",
			zap.Int64("deleted", rowsAffected))
	}
}

func loadConfig() *Config {
	port := os.Getenv("USER_BFF_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8081"
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

	// Load only the explicit User trust-domain configuration. No generic JWT
	// secret or cross-context refresh fallback is accepted.
	authIsolation := auth.LoadIsolationConfig(os.Getenv("ENVIRONMENT"), os.Getenv, secrets.Load)
	userAuthContext := authIsolation.User

	// Load TOTP encryption key for encrypting 2FA secrets at rest (AES-256-GCM)
	var totpEncryptionKey []byte
	if totpKeyHex := secrets.Load("TOTP_ENCRYPTION_KEY"); totpKeyHex != "" {
		var parseErr error
		totpEncryptionKey, parseErr = auth.ParseTOTPEncryptionKey(totpKeyHex)
		if parseErr != nil {
			log.Fatalf("FATAL: invalid TOTP_ENCRYPTION_KEY: %v", parseErr)
		}
		log.Println("TOTP encryption key loaded successfully")
	} else {
		if config.IsProduction() {
			log.Fatal("FATAL: TOTP_ENCRYPTION_KEY must be set in production")
		}
		log.Println("WARNING: TOTP_ENCRYPTION_KEY not set, TOTP secrets will be stored in plaintext")
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		if config.IsProduction() {
			log.Fatal("FATAL: REDIS_ADDR must be set in production")
		}
		redisAddr = "localhost:6379"
		log.Println("WARNING: REDIS_ADDR not set, using localhost:6379")
	}

	// Rate limiting configuration with secure defaults
	// Auth rate limit: 5 requests per minute per IP (brute force protection)
	authRateLimit := 5
	if v := os.Getenv("AUTH_RATE_LIMIT"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil && limit > 0 {
			authRateLimit = limit
		}
	}

	authRateWindow := time.Minute
	if v := os.Getenv("AUTH_RATE_WINDOW"); v != "" {
		if window, err := time.ParseDuration(v); err == nil && window > 0 {
			authRateWindow = window
		}
	}

	// Contest join rate limit: 10 requests per minute per user
	contestJoinRateLimit := 10
	if v := os.Getenv("CONTEST_JOIN_RATE_LIMIT"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil && limit > 0 {
			contestJoinRateLimit = limit
		}
	}

	contestJoinRateWindow := time.Minute
	if v := os.Getenv("CONTEST_JOIN_RATE_WINDOW"); v != "" {
		if window, err := time.ParseDuration(v); err == nil && window > 0 {
			contestJoinRateWindow = window
		}
	}

	environment, environmentErr := resolveSecurityEnvironment(os.Getenv("ENVIRONMENT"), os.Getenv("APP_ENV"))
	production := environmentErr != nil || environment == "production" || environment == "staging"

	// Security-code secrets support _FILE through packages/secrets. A clearly
	// marked local-only hashing key is used only outside production.
	securityCodeHashSecret := secrets.Load("SECURITY_CODE_HASH_SECRET")
	if securityCodeHashSecret == "" && !production {
		securityCodeHashSecret = localOnlySecurityCodeHashSecret
		log.Println("WARNING: using local-only SECURITY_CODE_HASH_SECRET")
	}
	mailerinoAPIKey := secrets.Load("MAILERINO_API_KEY")
	mailerinoFrom := strings.TrimSpace(os.Getenv("MAILERINO_FROM_EMAIL"))
	mailerinoBaseURL := strings.TrimSpace(os.Getenv("MAILERINO_BASE_URL"))
	if mailerinoBaseURL == "" {
		mailerinoBaseURL = notification.DefaultMailerinoBaseURL
	}
	resendAPIKey := secrets.Load("RESEND_API_KEY")
	resendFrom := strings.TrimSpace(os.Getenv("RESEND_FROM_EMAIL"))
	legacyEmailFrom := strings.TrimSpace(os.Getenv("EMAIL_FROM"))
	emailFromAmbiguous := resendFrom != "" && legacyEmailFrom != "" && resendFrom != legacyEmailFrom
	emailFrom := resendFrom
	if emailFrom == "" {
		emailFrom = legacyEmailFrom
	}
	if emailFrom == "" && !production {
		emailFrom = "noreply@localhost.invalid"
	}
	resendBaseURL := strings.TrimSpace(os.Getenv("RESEND_BASE_URL"))
	if resendBaseURL == "" {
		resendBaseURL = notification.DefaultResendBaseURL
	}
	kavenegarAPIKey := secrets.Load("KAVENEGAR_API_KEY")
	smsEnabled := !strings.EqualFold(strings.TrimSpace(os.Getenv("SMS_ENABLED")), "false")
	smsProviderMode := strings.TrimSpace(os.Getenv("SMS_PROVIDER"))
	if smsProviderMode == "" {
		smsProviderMode = "kavenegar"
	}
	smsTemplate := strings.TrimSpace(os.Getenv("SMS_TEMPLATE"))
	if smsTemplate == "" && !production {
		smsTemplate = "tragge-verify"
	}
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}

	// Google OAuth configuration (loaded via secrets package)
	googleClientID := secrets.GetGoogleClientID()
	googleClientSecret := secrets.GetGoogleClientSecret()
	googleRedirectURI := os.Getenv("GOOGLE_REDIRECT_URI")
	if googleRedirectURI == "" {
		// Note: Redirect goes to frontend callback page (GET), which then POSTs to /api/user/auth/google/callback
		googleRedirectURI = "http://localhost:8080/user/auth/google/callback"
	}

	// Jibit KYC configuration
	jibitAPIKey := secrets.Load("JIBIT_API_KEY")
	jibitSecretKey := secrets.Load("JIBIT_SECRET_KEY")
	jibitBaseURL := os.Getenv("JIBIT_BASE_URL") // defaults to https://napi.jibit.ir

	// ARCaptcha configuration (invisible CAPTCHA for registration)
	arcaptchaSiteKey := os.Getenv("ARCAPTCHA_SITE_KEY")
	arcaptchaSecretKey := secrets.Load("ARCAPTCHA_SECRET_KEY")

	// S3/MinIO storage configuration
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
	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		s3Bucket = "tragge-avatars"
	}
	s3UseSSL := os.Getenv("S3_USE_SSL") == "true"
	s3PublicURL := os.Getenv("S3_PUBLIC_URL")
	s3KYCBucket := os.Getenv("S3_KYC_BUCKET")
	if s3KYCBucket == "" {
		s3KYCBucket = "tragge-kyc"
	}
	s3TicketBucket := os.Getenv("S3_TICKET_BUCKET")
	if s3TicketBucket == "" {
		s3TicketBucket = "tragge-tickets"
	}

	return &Config{
		Port:                   port,
		PostgresDSN:            postgresDSN,
		PostgresReplicaDSNs:    postgresReplicaDSNs,
		AuthContext:            userAuthContext,
		RedisAddr:              redisAddr,
		AuthRateLimit:          authRateLimit,
		AuthRateWindow:         authRateWindow,
		ContestJoinRateLimit:   contestJoinRateLimit,
		ContestJoinRateWindow:  contestJoinRateWindow,
		SecurityCodeHashSecret: securityCodeHashSecret,
		MailerinoAPIKey:        mailerinoAPIKey,
		MailerinoFrom:          mailerinoFrom,
		MailerinoBaseURL:       mailerinoBaseURL,
		ResendAPIKey:           resendAPIKey,
		EmailFrom:              emailFrom,
		ResendBaseURL:          resendBaseURL,
		EmailFromAmbiguous:     emailFromAmbiguous,
		KaveNegarAPIKey:        kavenegarAPIKey,
		SMSEnabled:             smsEnabled,
		SMSProviderMode:        smsProviderMode,
		SMSSender:              strings.TrimSpace(os.Getenv("SMS_SENDER")),
		SMSTemplate:            smsTemplate,
		FrontendURL:            frontendURL,
		GoogleClientID:         googleClientID,
		GoogleClientSecret:     googleClientSecret,
		GoogleRedirectURI:      googleRedirectURI,
		JibitAPIKey:            jibitAPIKey,
		JibitSecretKey:         jibitSecretKey,
		JibitBaseURL:           jibitBaseURL,
		S3Endpoint:             s3Endpoint,
		S3AccessKey:            s3AccessKey,
		S3SecretKey:            s3SecretKey,
		S3Region:               s3Region,
		S3Bucket:               s3Bucket,
		S3UseSSL:               s3UseSSL,
		S3PublicURL:            s3PublicURL,
		S3KYCBucket:            s3KYCBucket,
		S3TicketBucket:         s3TicketBucket,
		TOTPEncryptionKey:      totpEncryptionKey,
		ARCaptchaSiteKey:       arcaptchaSiteKey,
		ARCaptchaSecretKey:     arcaptchaSecretKey,
	}
}

// authMiddleware wraps the auth package middleware.
func (a *App) authMiddleware(next http.Handler) http.Handler {
	return a.auth.Middleware.RequireAuth(next)
}

// optionalAuthMiddleware wraps the auth package optional auth middleware.
func (a *App) optionalAuthMiddleware(next http.Handler) http.Handler {
	return a.auth.Middleware.OptionalAuth(next)
}

// handleAuthRateLimitExceeded handles rate limit exceeded for auth endpoints.
func (a *App) handleAuthRateLimitExceeded(w http.ResponseWriter, r *http.Request, info ratelimit.LimitInfo) {
	clientIP := ratelimit.IPExtractor(r)
	a.log().Warn("Auth rate limit exceeded",
		zap.String("ip", clientIP),
		zap.String("path", r.URL.Path),
		zap.Int("remaining", info.Remaining),
		zap.Duration("retry_after", info.RetryAfter))

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("Retry-After", strconv.Itoa(int(info.RetryAfter.Seconds())+1))
	w.WriteHeader(http.StatusTooManyRequests)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":       "too many requests",
		"message":     msg.TooManyAuthAttempts,
		"retry_after": int(info.RetryAfter.Seconds()) + 1,
	})
}

// handleContestJoinRateLimitExceeded handles rate limit exceeded for contest join.
func (a *App) handleContestJoinRateLimitExceeded(w http.ResponseWriter, r *http.Request, info ratelimit.LimitInfo) {
	userID := auth.GetUserID(r.Context())
	a.log().Warn("Contest join rate limit exceeded",
		zap.String("user_id", userID),
		zap.Int("remaining", info.Remaining),
		zap.Duration("retry_after", info.RetryAfter))

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("Retry-After", strconv.Itoa(int(info.RetryAfter.Seconds())+1))
	w.WriteHeader(http.StatusTooManyRequests)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":       "too many requests",
		"message":     msg.TooManyContestJoins,
		"retry_after": int(info.RetryAfter.Seconds()) + 1,
	})
}

// getClientIP extracts the client IP address from the request using the
// shared trusted-proxy-aware helper.
func getClientIP(r *http.Request) string {
	return validation.ExtractClientIP(r)
}

// log returns the observability logger.
func (a *App) log() *zap.Logger {
	return a.obs.Logger.Logger
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
		"service":   "user-bff",
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

	// Check SMS provider health (optional - not critical, email auth still works)
	if a.otpService != nil {
		if err := a.otpService.Provider().HealthCheck(); err != nil {
			response["sms"] = "unavailable"
			a.log().Warn("KaveNegar health check failed", zap.Error(err))
			if response["status"] == "ready" {
				response["status"] = "degraded"
			}
		} else {
			response["sms"] = "healthy"
		}
	} else {
		response["sms"] = "not_configured"
	}

	// Check Redis connectivity (optional - used for session management)
	if a.redis != nil {
		if err := a.redis.Ping(ctx).Err(); err != nil {
			response["redis"] = "unavailable"
			// Redis is not critical for user-bff, just mark as degraded
			if response["status"] == "ready" {
				response["status"] = "degraded"
				response["message"] = "redis unavailable (session management degraded)"
			}
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

// RegisterRequest is the request body for registration.
type RegisterRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	Country      string `json:"country"`
	ReferralCode string `json:"ref,omitempty"` // Optional referral code
	AgreeTerms   bool   `json:"agree_terms"`
	AgeConfirm   bool   `json:"age_confirm"`
	CaptchaToken string `json:"captcha_token"` // ARCaptcha verification token
}

// LoginRequest is the request body for login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RefreshRequest is the request body for token refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshResponse is the response for successful token refresh.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds until access token expires
}

// ForgotPasswordRequest is the request body for requesting a password reset.
type ForgotPasswordRequest struct {
	Identifier   string `json:"identifier"`    // username, phone, or email
	CaptchaToken string `json:"captcha_token"` // ARCaptcha verification token
}

// ForgotPasswordVerifyRequest is the request body for verifying a password reset code.
type ForgotPasswordVerifyRequest struct {
	ResetToken string `json:"reset_token"`
	Code       string `json:"code"`
}

// ForgotPasswordResetRequest is the request body for setting a new password after code verification.
type ForgotPasswordResetRequest struct {
	PasswordSetToken string `json:"password_set_token"`
	NewPassword      string `json:"new_password"`
	ConfirmPassword  string `json:"confirm_password"`
}

// ChangePasswordRequest is the request body for changing password while authenticated.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

// AuthResponse is the response for successful authentication.
type AuthResponse struct {
	AccessToken          string    `json:"access_token"`
	RefreshToken         string    `json:"refresh_token"`
	ExpiresAt            time.Time `json:"expires_at"`
	EmailVerified        bool      `json:"email_verified"`
	RetryAfterSeconds    int       `json:"retry_after_seconds,omitempty"`
	RequiresVerification bool      `json:"requires_verification,omitempty"`
	AvailableMethods     []string  `json:"available_methods,omitempty"`
	MaskedPhone          string    `json:"masked_phone,omitempty"`
	MaskedEmail          string    `json:"masked_email,omitempty"`
}

// AuthTicketResponse is the response for creating a short-lived auth ticket.
type AuthTicketResponse struct {
	Ticket string `json:"ticket"`
}

// ExchangeTicketRequest is the request body for exchanging a ticket for a token.
type ExchangeTicketRequest struct {
	Ticket string `json:"ticket"`
}

// ExchangeTicketResponse is the response for successful ticket exchange.
type ExchangeTicketResponse struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken *string `json:"refresh_token"`
}

// UserResponse is the response for /me endpoint.
type UserResponse struct {
	UserID        string    `json:"user_id"`
	Email         string    `json:"email"`
	Roles         []string  `json:"roles"`
	EmailVerified bool      `json:"email_verified"`
	PhoneVerified bool      `json:"phone_verified"`
	Username      *string   `json:"username,omitempty"`
	DisplayName   *string   `json:"display_name,omitempty"`
	AvatarURL     *string   `json:"avatar_url,omitempty"`
	Bio           *string   `json:"bio,omitempty"`
	Country       *string   `json:"country,omitempty"`
	Phone         *string   `json:"phone,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// UpdateProfileRequest is the request body for profile updates.
type UpdateProfileRequest struct {
	Username    *string `json:"username,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	Bio         *string `json:"bio,omitempty"`
	Country     *string `json:"country,omitempty"`
	Phone       *string `json:"phone,omitempty"`
}

// UpdateProfileResponse is the response for profile updates.
type UpdateProfileResponse struct {
	UserID      string  `json:"user_id"`
	Username    *string `json:"username,omitempty"`
	DisplayName *string `json:"display_name,omitempty"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
	Bio         *string `json:"bio,omitempty"`
	Country     *string `json:"country,omitempty"`
	Phone       *string `json:"phone,omitempty"`
}

// AvatarUploadResponse is the response for avatar uploads.
type AvatarUploadResponse struct {
	AvatarURL string `json:"avatar_url"`
}

// SelectAvatarRequest is the request body for selecting a predefined avatar.
type SelectAvatarRequest struct {
	AvatarID string `json:"avatar_id"`
}

// SelectAvatarResponse is the response for predefined avatar selection.
type SelectAvatarResponse struct {
	AvatarID  string `json:"avatar_id"`
	AvatarURL string `json:"avatar_url"`
}

// PredefinedAvatar represents a preset avatar option.
type PredefinedAvatar struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
	BgColor     string `json:"bg_color"`
	Path        string `json:"path"`
	SortOrder   int    `json:"sort_order"`
}

// ContestSymbol represents a symbol available in a contest.
type ContestSymbol struct {
	Symbol  string `json:"symbol"`
	Enabled bool   `json:"enabled"`
}

// ContestResponse represents a contest for listing.
type ContestResponse struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Description          string          `json:"description,omitempty"`
	StartsAt             time.Time       `json:"starts_at"`
	EndsAt               time.Time       `json:"ends_at"`
	Status               string          `json:"status"`
	EntryFee             int             `json:"entry_fee_cents"`
	QtyTotal             int64           `json:"qty_total"`
	DurationType         string          `json:"duration_type"`
	AssetClass           string          `json:"asset_class"`
	DurationMinutes      int             `json:"duration_minutes"`
	MinParticipants      int             `json:"min_participants"`
	MaxParticipants      *int            `json:"max_participants,omitempty"`
	RegistrationDeadline *time.Time      `json:"registration_deadline,omitempty"`
	CommissionRate       float64         `json:"commission_rate"`
	IsFree               bool            `json:"is_free"`
	ParticipantCount     int             `json:"participant_count"`
	Rules                json.RawMessage `json:"rules,omitempty"`
	Symbols              []ContestSymbol `json:"symbols"`
}

// ContestDetailsResponse represents detailed contest information.
type ContestDetailsResponse struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Status              string   `json:"status"`
	MarketType          string   `json:"market_type"`
	AssetClass          string   `json:"asset_class"`
	DurationType        string   `json:"duration_type"`
	StartTime           string   `json:"start_time"`
	EndTime             string   `json:"end_time"`
	EntryFeeCents       int      `json:"entry_fee_cents"`
	IsFree              bool     `json:"is_free"`
	PrizePoolCents      int      `json:"prize_pool_cents"`
	AvailableQty        int64    `json:"available_qty"`
	MaxParticipants     *int     `json:"max_participants,omitempty"`
	CurrentParticipants int      `json:"current_participants"`
	UserJoined          bool     `json:"user_joined"`
	Symbols             []string `json:"symbols"`

	// P2-P3-2: Fee transparency
	CommissionRate  float64 `json:"commission_rate"` // Platform commission percentage (e.g. 20.0 = 20%)
	GrossPrizeCents int     `json:"gross_prize_pool_cents"`

	// P2-P3-3: Server time for countdown synchronization
	ServerTime string `json:"server_time"` // ISO8601 timestamp for client clock-drift correction
}

// JoinContestResponse is the response for joining a contest.
type JoinContestResponse struct {
	ContestID     string    `json:"contest_id"`
	UserID        string    `json:"user_id"`
	JoinedAt      time.Time `json:"joined_at"`
	QtyTotal      int64     `json:"qty_total"`
	QtyAvailable  int64     `json:"qty_available"`
	AlreadyJoined bool      `json:"already_joined"`

	// P2-P3-2: Fee transparency fields
	EntryFeeCents  int   `json:"entry_fee_cents,omitempty"`
	PlatformFeeBps int   `json:"platform_fee_bps,omitempty"`
	NetPrizeCents  int64 `json:"net_prize_pool_estimate_cents,omitempty"` // Estimated net prize pool at join time
}

// ParticipantEntry represents a single participant in a contest.
type ParticipantEntry struct {
	UserID          string    `json:"user_id"`
	Username        string    `json:"username"`
	JoinedAt        time.Time `json:"joined_at"`
	QtyTotal        int64     `json:"qty_total"`
	QtyAvailable    int64     `json:"qty_available"`
	TotalScore      float64   `json:"total_score"`
	FinalRank       *int      `json:"final_rank"`
	FinalPrizeCents *int      `json:"final_prize_cents"`
}

// ContestParticipantsResponse is the response for listing contest participants.
type ContestParticipantsResponse struct {
	Participants []ParticipantEntry `json:"participants"`
	Total        int                `json:"total"`
}

// LeaderboardEntry represents a single entry in the leaderboard.
type LeaderboardEntry struct {
	Rank       int     `json:"rank"`
	UserID     string  `json:"user_id"`
	TotalScore float64 `json:"total_score"`
}

// LeaderboardResponse is the response for leaderboard queries.
type LeaderboardResponse struct {
	ContestID string             `json:"contest_id"`
	Entries   []LeaderboardEntry `json:"entries"`
}

// ContestLeaderboardEntry represents a single entry in the contest leaderboard.
// TODO: The frontend ContestResultsPage needs additional fields to function correctly:
//   - user_id (string): Required for highlighting the current user's row and "Jump to My Rank"
//   - trade_count (int): Required for displaying trades column in the ranking table
//
// Add these to the struct and the SQL query in handleGetContestLeaderboard.
type ContestLeaderboardEntry struct {
	Position    int     `json:"position"`
	UserID      string  `json:"user_id"`
	Username    string  `json:"username"`
	PnlPercent  float64 `json:"pnl_percent"`
	RewardCents int     `json:"reward_cents"`
	TradeCount  int     `json:"trade_count"`
}

// ContestLeaderboardResponse is the response for contest leaderboard queries.
type ContestLeaderboardResponse struct {
	Leaderboard       []ContestLeaderboardEntry `json:"leaderboard"`
	TotalParticipants int                       `json:"total_participants"`
	PrizePoolCents    int                       `json:"prize_pool_cents"`
}

// ContestMyResultResponse is the response for a user's result in a specific contest.
type ContestMyResultResponse struct {
	ContestID         string  `json:"contest_id"`
	UserID            string  `json:"user_id"`
	FinalRank         int     `json:"final_rank"`
	TotalParticipants int     `json:"total_participants"`
	TotalScore        float64 `json:"total_score"`
	PnlPercent        float64 `json:"pnl_percent"`
	RewardCents       int     `json:"reward_cents"`
	TradeCount        int     `json:"trade_count"`
	WinningTrades     int     `json:"winning_trades"`
	LosingTrades      int     `json:"losing_trades"`
	BestTradePnl      float64 `json:"best_trade_pnl"`
	WorstTradePnl     float64 `json:"worst_trade_pnl"`
}

// ContestTrade represents a single trade in a contest trade history.
type ContestTrade struct {
	TradeID    string   `json:"trade_id"`
	Symbol     string   `json:"symbol"`
	Side       string   `json:"side"`
	Qty        int64    `json:"qty"`
	EntryPrice float64  `json:"entry_price"`
	ExitPrice  *float64 `json:"exit_price,omitempty"`
	Pnl        *float64 `json:"pnl,omitempty"`
	PnlPercent *float64 `json:"pnl_percent,omitempty"`
	OpenedAt   string   `json:"opened_at"`
	ClosedAt   *string  `json:"closed_at,omitempty"`
	Status     string   `json:"status"`
}

// ContestTradeSummary contains summary statistics for trades in a contest.
type ContestTradeSummary struct {
	TotalTrades   int     `json:"total_trades"`
	WinningTrades int     `json:"winning_trades"`
	LosingTrades  int     `json:"losing_trades"`
	TotalPnl      float64 `json:"total_pnl"`
	AvgWin        float64 `json:"avg_win"`
	AvgLoss       float64 `json:"avg_loss"`
}

// ContestTradesResponse is the response for contest trade history queries.
type ContestTradesResponse struct {
	Trades  []ContestTrade      `json:"trades"`
	Total   int                 `json:"total"`
	Summary ContestTradeSummary `json:"summary"`
}

// ContestHistoryEntry represents a contest the user participated in.
type ContestHistoryEntry struct {
	ContestID         string    `json:"contest_id"`
	ContestName       string    `json:"contest_name"`
	Status            string    `json:"status"`
	StartsAt          string    `json:"starts_at"`
	EndsAt            string    `json:"ends_at"`
	JoinedAt          time.Time `json:"joined_at"`
	TotalScore        float64   `json:"total_score"`
	PnlPercent        *float64  `json:"pnl_percent,omitempty"`
	FinalRank         *int      `json:"final_rank,omitempty"`
	TotalParticipants int       `json:"total_participants"`
	FinalPrizeCents   *int      `json:"final_prize_cents,omitempty"`
	TradeCount        int       `json:"trade_count"`
	DurationType      *string   `json:"duration_type,omitempty"`
	MarketType        *string   `json:"market_type,omitempty"`
}

// ContestHistoryResponse is the response for user contest history.
type ContestHistoryResponse struct {
	Contests []ContestHistoryEntry `json:"contests"`
	Total    int                   `json:"total"`
	Page     int                   `json:"page"`
	PerPage  int                   `json:"per_page"`
}

// MyTournamentEntry is a single tournament the user has joined.
type MyTournamentEntry struct {
	ContestID         string   `json:"contest_id"`
	ContestName       string   `json:"contest_name"`
	Status            string   `json:"status"`
	StartsAt          string   `json:"starts_at"`
	EndsAt            string   `json:"ends_at"`
	EntryFeeCents     int      `json:"entry_fee_cents"`
	TotalScore        float64  `json:"total_score"`
	FinalRank         *int     `json:"final_rank,omitempty"`
	FinalPrizeCents   *int     `json:"final_prize_cents,omitempty"`
	TotalParticipants int      `json:"total_participants"`
	AssetClass        *string  `json:"asset_class,omitempty"`
	DurationType      *string  `json:"duration_type,omitempty"`
	IsFree            bool     `json:"is_free"`
	QtyTotal          int64    `json:"qty_total"`
	PnlPercent        *float64 `json:"pnl_percent,omitempty"`
}

// MyTournamentCounts holds the count of contests per status category.
type MyTournamentCounts struct {
	Active    int `json:"active"`
	Upcoming  int `json:"upcoming"`
	Completed int `json:"completed"`
	Cancelled int `json:"cancelled"`
}

// MyTournamentsResponse is the response for GET /me/tournaments.
type MyTournamentsResponse struct {
	Contests []MyTournamentEntry `json:"contests"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PerPage  int                 `json:"per_page"`
	Counts   MyTournamentCounts  `json:"counts"`
}

// WalletResponse is the response for wallet queries.
type WalletResponse struct {
	UserID       string `json:"user_id"`
	BalanceCents int64  `json:"balance_cents"`
	Currency     string `json:"currency"`
	Status       string `json:"status"`
}

// WalletHistoryEntry represents a single ledger entry.
type WalletHistoryEntry struct {
	ID                string  `json:"id"`
	Type              string  `json:"type"`
	AmountCents       int64   `json:"amount_cents"`
	BalanceAfterCents int64   `json:"balance_after_cents"`
	RefType           *string `json:"ref_type,omitempty"`
	RefID             *string `json:"ref_id,omitempty"`
	Description       *string `json:"description,omitempty"`
	ReasonCode        *string `json:"reason_code,omitempty"`
	CreatedAt         string  `json:"created_at"`
	Status            *string `json:"status,omitempty"`
	AdminComment      *string `json:"admin_comment,omitempty"`
}

// WalletHistoryResponse is the response for wallet history queries.
type WalletHistoryResponse struct {
	Entries      []WalletHistoryEntry `json:"entries"`
	Total        int                  `json:"total"`
	BalanceCents int64                `json:"balance_cents"`
	Page         int                  `json:"page"`
	HasMore      bool                 `json:"has_more"`
}

// ReferralValidateResponse is the response for referral code validation.
type ReferralValidateResponse struct {
	Valid        bool    `json:"valid"`
	ReferrerName *string `json:"referrer_name,omitempty"`
}

// AffiliateStatsResponse is the response for affiliate stats.
type AffiliateStatsResponse struct {
	ReferralCode       string `json:"referral_code"`
	TotalReferrals     int    `json:"total_referrals"`
	QualifiedReferrals int    `json:"qualified_referrals"`
	TotalEarnedCents   int64  `json:"total_earned_cents"`
	PendingEarnedCents int64  `json:"pending_earned_cents"`
	CommissionRateBps  int    `json:"commission_rate_bps"`
	IsActive           bool   `json:"is_active"`
}

// ReferralEntry represents a single referral for listing.
type ReferralEntry struct {
	Email       string     `json:"email"` // Masked email
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	QualifiedAt *time.Time `json:"qualified_at,omitempty"`
}

// AffiliateReferralsResponse is the response for affiliate referrals listing.
type AffiliateReferralsResponse struct {
	Referrals  []ReferralEntry `json:"referrals"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// ReferralApplyRequest is the request body for applying a referral code.
type ReferralApplyRequest struct {
	Code string `json:"code"`
}

// ReferralApplyResponse is the response for applying a referral code.
type ReferralApplyResponse struct {
	Applied bool   `json:"applied"`
	Message string `json:"message"`
}

// UserContestStatsResponse is the response for user contest statistics dashboard.
type UserContestStatsResponse struct {
	TotalContests    int     `json:"total_contests"`
	TotalWins        int     `json:"total_wins"`
	TotalPrizesCents int64   `json:"total_prizes_cents"`
	WinRate          float64 `json:"win_rate"`
	BestRank         int     `json:"best_rank"`
	AverageRank      float64 `json:"average_rank"`
	TotalPnl         float64 `json:"total_pnl"`
	FavoriteMarket   *string `json:"favorite_market,omitempty"`
}

// AffiliateStatusStats contains affiliate statistics for the status response.
type AffiliateStatusStats struct {
	TotalReferrals     int   `json:"total_referrals"`
	QualifiedReferrals int   `json:"qualified_referrals"`
	TotalEarned        int64 `json:"total_earned"`
	PendingEarnings    int64 `json:"pending_earnings"`
}

// AffiliateStatusResponse is the response for affiliate activation status.
type AffiliateStatusResponse struct {
	Status      string                `json:"status"` // inactive, pending, active, rejected
	Code        string                `json:"code"`
	Stats       *AffiliateStatusStats `json:"stats,omitempty"`
	RequestedAt *time.Time            `json:"requested_at,omitempty"`
	ApprovedAt  *time.Time            `json:"approved_at,omitempty"`
}

// UserStatsResponse represents user statistics for the global leaderboard.
type UserStatsResponse struct {
	UserID                  string  `json:"user_id"`
	TotalContests           int     `json:"total_contests"`
	TotalWins               int     `json:"total_wins"`
	TotalTop3               int     `json:"total_top3"`
	TotalScore              float64 `json:"total_score"`
	TraggePoint             float64 `json:"tragge_point"`
	WinRate                 float64 `json:"win_rate"`
	AvgTradeDurationSeconds int     `json:"avg_trade_duration_seconds"`
	BestMarket              *string `json:"best_market,omitempty"`
	BestMarketPnL           float64 `json:"best_market_pnl"`
	TotalTrades             int64   `json:"total_trades"`
	TotalPnL                float64 `json:"total_pnl"`
}

// GlobalLeaderboardEntry represents a user's rank in the global leaderboard.
type GlobalLeaderboardEntry struct {
	Rank          int     `json:"rank"`
	UserID        string  `json:"user_id"`
	Username      string  `json:"username,omitempty"`
	TraggePoint   float64 `json:"tragge_point"`
	TotalContests int     `json:"total_contests"`
	TotalWins     int     `json:"total_wins"`
	TotalTop3     int     `json:"total_top3"`
	WinRate       float64 `json:"win_rate"`
}

// GlobalLeaderboardResponse is the response for global leaderboard queries.
type GlobalLeaderboardResponse struct {
	Entries   []GlobalLeaderboardEntry `json:"entries"`
	UserRank  *int                     `json:"user_rank,omitempty"`
	UserScore *float64                 `json:"user_score,omitempty"`
}

// ScoreHistoryEntry represents a user's performance in a single contest.
type ScoreHistoryEntry struct {
	ContestID               string  `json:"contest_id"`
	ContestName             string  `json:"contest_name"`
	Rank                    int     `json:"rank"`
	Score                   float64 `json:"score"`
	Participants            int     `json:"participants"`
	PnL                     float64 `json:"pnl"`
	TradesCount             int     `json:"trades_count"`
	AvgTradeDurationSeconds int     `json:"avg_trade_duration_seconds"`
	TopSymbol               *string `json:"top_symbol,omitempty"`
	TopSymbolPnL            float64 `json:"top_symbol_pnl"`
	CreatedAt               string  `json:"created_at"`
}

// ScoreHistoryResponse is the response for score history queries.
type ScoreHistoryResponse struct {
	Entries []ScoreHistoryEntry `json:"entries"`
}

// KYCStatusResponse is the response for GET /api/user/kyc/status.
type KYCStatusResponse struct {
	Status          string     `json:"status"`
	FirstName       *string    `json:"first_name,omitempty"`
	LastName        *string    `json:"last_name,omitempty"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`

	// Jibit verification step statuses
	ShahkarVerified bool     `json:"shahkar_verified"`
	FaceVerified    bool     `json:"face_verified"`
	FaceMatchScore  *float64 `json:"face_match_score,omitempty"`
	LivenessScore   *float64 `json:"liveness_score,omitempty"`
	CardOCRVerified bool     `json:"card_ocr_verified"`

	// Pre-populated data for rejected resubmission
	FatherNamePrefill      *string `json:"father_name,omitempty"`
	NationalCodePrefill    *string `json:"national_code_manual,omitempty"`
	DateOfBirthPrefill     *string `json:"date_of_birth,omitempty"`
	PhonePrefill           *string `json:"phone,omitempty"`
	CityPrefill            *string `json:"city,omitempty"`
	AddressPrefill         *string `json:"address_line1,omitempty"`
	PostalCodePrefill      *string `json:"postal_code,omitempty"`
	ProvincePrefill        *string `json:"province,omitempty"`
	DocumentTypePrefill    *string `json:"document_type,omitempty"`
	DocumentNumberPrefill  *string `json:"document_number,omitempty"`
	BirthCertNumberPrefill *string `json:"birth_certificate_number,omitempty"`
	BirthCertSerialPrefill *string `json:"birth_certificate_serial,omitempty"`

	// Rejection details
	RejectionFields        []string          `json:"rejection_fields,omitempty"`
	RejectionFieldMessages map[string]string `json:"rejection_field_messages,omitempty"`
}

// KYCSubmitResponse is the response for POST /api/user/kyc/submit.
type KYCSubmitResponse struct {
	Message             string `json:"message"`
	Status              string `json:"status"`
	EstimatedReviewTime string `json:"estimated_review_time"`
}

// ============================================================================
// CALENDAR API TYPES
// ============================================================================

// CalendarContest represents a contest in the calendar view.
type CalendarContest struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Type            string               `json:"type"`
	AssetClass      string               `json:"asset_class"`
	EntryFee        float64              `json:"entry_fee"`
	PrizePool       float64              `json:"prize_pool"`
	DurationMinutes int                  `json:"duration_minutes"`
	StartsAt        string               `json:"starts_at"`
	EndsAt          string               `json:"ends_at"`
	Status          string               `json:"status"`
	Participants    CalendarParticipants `json:"participants"`
	UserRegistered  bool                 `json:"user_registered"`
}

// CalendarParticipants represents participant counts for a calendar contest.
type CalendarParticipants struct {
	Current int  `json:"current"`
	Max     *int `json:"max,omitempty"`
}

// CalendarResponse is the response for GET /api/user/contests/calendar.
type CalendarResponse struct {
	From     string            `json:"from"`
	To       string            `json:"to"`
	Contests []CalendarContest `json:"contests"`
	Total    int               `json:"total"`
}

// CalendarGroupedResponse is the response for calendar with grouping.
type CalendarGroupedResponse struct {
	From   string          `json:"from"`
	To     string          `json:"to"`
	Groups []CalendarGroup `json:"groups"`
	Total  int             `json:"total"`
}

// CalendarGroup represents a group of contests (by day, type, or asset).
type CalendarGroup struct {
	Key      string            `json:"key"`   // date (YYYY-MM-DD), type name, or asset class
	Label    string            `json:"label"` // Human-readable label
	Contests []CalendarContest `json:"contests"`
	Count    int               `json:"count"`
}

// ContestTemplate represents a tournament template for generating contests.
type ContestTemplate struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	Type            string  `json:"type"`
	AssetClass      string  `json:"asset_class"`
	DurationMinutes int     `json:"duration_minutes"`
	EntryFeeCents   int     `json:"entry_fee_cents"`
	MaxParticipants *int    `json:"max_participants,omitempty"`
	RecurrenceRule  *string `json:"recurrence_rule,omitempty"`
	IsActive        bool    `json:"is_active"`
}

// GeneratedContest represents a contest instance generated from a ContestTemplate.
type GeneratedContest struct {
	TemplateID      string    `json:"template_id"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	AssetClass      string    `json:"asset_class"`
	EntryFeeCents   int       `json:"entry_fee_cents"`
	DurationMinutes int       `json:"duration_minutes"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	MaxParticipants *int      `json:"max_participants,omitempty"`
	Status          string    `json:"status"`
}
