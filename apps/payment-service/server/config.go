package server

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
)

// Config holds application configuration.
type Config struct {
	// Server configuration
	Port string

	// Database configuration
	PostgresDSN         string
	PostgresReplicaDSNs []string

	// Redis configuration
	RedisAddr string

	// User authentication configuration
	AuthContext auth.ContextConfig

	// NOWPayments configuration
	NowPaymentsAPIKey    string
	NowPaymentsPublicKey string
	NowPaymentsIPNSecret string
	NowPaymentsBaseURL   string
	NowPaymentsSandbox   bool

	// Jibit PPG configuration
	JibitAPIKey      string
	JibitSecretKey   string
	JibitCallbackURL string
	JibitBaseURL     string
	JibitAllowedIPs  []string // CIDR ranges for webhook IP whitelist

	// Payment4 configuration
	Payment4APIKey    string
	Payment4IPNSecret string
	Payment4BaseURL   string
	Payment4Sandbox   bool

	// Rate limiting configuration
	PaymentRateLimit  int
	PaymentRateWindow time.Duration

	// Webhook signature verification
	WebhookSecretNowPayments string

	// Payment configuration
	MinDepositCents    int64
	MaxDepositCents    int64
	MinDepositIRR      int64
	MaxDepositIRR      int64
	MinWithdrawCents   int64
	MaxWithdrawCents   int64
	WithdrawFeeCents   int64
	WithdrawFeePercent float64

	// AML withdrawal limits (defaults, per-user overrides in DB)
	DailyWithdrawAmountCents   int64
	MonthlyWithdrawAmountCents int64
	DailyWithdrawCount         int
	MonthlyWithdrawCount       int

	// Exchange rate configuration
	ExchangeRateNobitexURL   string
	ExchangeRateStaticUSDIRR float64
	ExchangeRateCacheTTL     time.Duration

	// Cleanup worker configuration
	CleanupInterval      time.Duration
	CleanupOrphanedAfter time.Duration

	// Inquiry worker configuration (BUG #305: resolve stuck UNKNOWN Jibit payments)
	InquiryInterval time.Duration
	InquiryMaxAge   time.Duration

	// Expiry worker configuration (BUG #306: expire stale payment intents and roll back stuck payouts)
	ExpiryInterval                 time.Duration
	ExpiryThreshold                time.Duration
	PayoutExpiryThreshold          time.Duration
	ProcessingPayoutAlertThreshold time.Duration
}

func loadConfig() *Config {
	port := os.Getenv("PAYMENT_SERVICE_PORT")
	if port == "" {
		port = config.GetEnv("PORT", "8091")
	}

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

	authIsolation := auth.LoadIsolationConfig(os.Getenv("ENVIRONMENT"), os.Getenv, secrets.Load)
	userAuthContext := authIsolation.User

	redisAddr := config.GetEnv("REDIS_ADDR", "localhost:6379")

	// NOWPayments configuration
	nowPaymentsAPIKey := getSecretOrEnv("NOWPAYMENTS_API_KEY", "", "")
	nowPaymentsPublicKey := getSecretOrEnv("NOWPAYMENTS_PUBLIC_KEY", "", "")
	nowPaymentsIPNSecret := getSecretOrEnv("NOWPAYMENTS_IPN_SECRET", "", "")
	nowPaymentsBaseURL := config.GetEnv("NOWPAYMENTS_BASE_URL", "https://api.nowpayments.io/v1")
	nowPaymentsSandbox := config.GetEnv("NOWPAYMENTS_SANDBOX", "false") == "true"

	// Jibit PPG configuration
	jibitAPIKey := getSecretOrEnv("JIBIT_API_KEY", "", "")
	jibitSecretKey := getSecretOrEnv("JIBIT_SECRET_KEY", "", "")
	jibitCallbackURL := config.GetEnv("JIBIT_CALLBACK_URL", "")
	jibitBaseURL := config.GetEnv("JIBIT_BASE_URL", "https://napi.jibit.ir/ppg/v3")

	// Payment4 configuration
	payment4APIKey := getSecretOrEnv("PAYMENT4_API_KEY", "", "")
	payment4IPNSecret := getSecretOrEnv("PAYMENT4_IPN_SECRET", "", "")
	payment4BaseURL := config.GetEnv("PAYMENT4_BASE_URL", "")
	payment4Sandbox := config.GetEnv("PAYMENT4_SANDBOX", "false") == "true"

	// Jibit webhook IP whitelist (comma-separated CIDRs)
	var jibitAllowedIPs []string
	if raw := os.Getenv("JIBIT_ALLOWED_IPS"); raw != "" {
		for _, cidr := range strings.Split(raw, ",") {
			cidr = strings.TrimSpace(cidr)
			if cidr != "" {
				jibitAllowedIPs = append(jibitAllowedIPs, cidr)
			}
		}
	}

	// Rate limiting
	paymentRateLimit := config.GetEnvInt("PAYMENT_RATE_LIMIT", 10)
	paymentRateWindow := getEnvDuration("PAYMENT_RATE_WINDOW", time.Minute)

	// Payment limits
	minDepositCents := config.GetEnvInt64("MIN_DEPOSIT_CENTS", 100)       // $1.00
	maxDepositCents := config.GetEnvInt64("MAX_DEPOSIT_CENTS", 1000000)   // $10,000
	minDepositIRR := config.GetEnvInt64("MIN_DEPOSIT_IRR", 500000)        // 500,000 IRR ≈ $0.60
	maxDepositIRR := config.GetEnvInt64("MAX_DEPOSIT_IRR", 100000000000)  // 100B IRR ≈ $12,000
	minWithdrawCents := config.GetEnvInt64("MIN_WITHDRAW_CENTS", 1000)    // $10.00
	maxWithdrawCents := config.GetEnvInt64("MAX_WITHDRAW_CENTS", 5000000) // $50,000
	withdrawFeeCents := config.GetEnvInt64("WITHDRAW_FEE_CENTS", 0)       // Flat fee
	withdrawFeePercent := getEnvFloat("WITHDRAW_FEE_PERCENT", 0.0)        // Percentage fee

	// AML withdrawal limits
	dailyWithdrawAmountCents := config.GetEnvInt64("DAILY_WITHDRAW_AMOUNT_CENTS", 1000000)     // $10,000/day
	monthlyWithdrawAmountCents := config.GetEnvInt64("MONTHLY_WITHDRAW_AMOUNT_CENTS", 5000000) // $50,000/month
	dailyWithdrawCount := config.GetEnvInt("DAILY_WITHDRAW_COUNT", 3)                          // 3/day
	monthlyWithdrawCount := config.GetEnvInt("MONTHLY_WITHDRAW_COUNT", 10)                     // 10/month

	// Exchange rate configuration
	exchangeRateNobitexURL := config.GetEnv("EXCHANGE_RATE_NOBITEX_URL", "https://apiv2.nobitex.ir")
	exchangeRateStaticUSDIRR := getEnvFloat("EXCHANGE_RATE_STATIC_USD_IRR", 8500000)
	exchangeRateCacheTTL := getEnvDuration("EXCHANGE_RATE_CACHE_TTL", 60*time.Second)

	// Cleanup worker configuration
	cleanupInterval := getEnvDuration("CLEANUP_INTERVAL", 5*time.Minute)
	cleanupOrphanedAfter := getEnvDuration("CLEANUP_ORPHANED_AFTER", 15*time.Minute)

	// Inquiry worker configuration
	inquiryInterval := getEnvDuration("INQUIRY_INTERVAL", 5*time.Minute)
	inquiryMaxAge := getEnvDuration("INQUIRY_MAX_AGE", 24*time.Hour)

	// Expiry worker configuration
	expiryInterval := getEnvDuration("EXPIRY_INTERVAL", 5*time.Minute)
	expiryThreshold := getEnvDuration("EXPIRY_THRESHOLD", 20*time.Minute)
	payoutExpiryThreshold := getEnvDuration("PAYOUT_EXPIRY_THRESHOLD", 60*time.Minute)
	processingPayoutAlertThreshold := getEnvDuration("PROCESSING_PAYOUT_ALERT_THRESHOLD", 48*time.Hour)

	return &Config{
		Port:                           port,
		PostgresDSN:                    postgresDSN,
		PostgresReplicaDSNs:            postgresReplicaDSNs,
		RedisAddr:                      redisAddr,
		AuthContext:                    userAuthContext,
		NowPaymentsAPIKey:              nowPaymentsAPIKey,
		NowPaymentsPublicKey:           nowPaymentsPublicKey,
		NowPaymentsIPNSecret:           nowPaymentsIPNSecret,
		NowPaymentsBaseURL:             nowPaymentsBaseURL,
		NowPaymentsSandbox:             nowPaymentsSandbox,
		JibitAPIKey:                    jibitAPIKey,
		JibitSecretKey:                 jibitSecretKey,
		JibitCallbackURL:               jibitCallbackURL,
		JibitBaseURL:                   jibitBaseURL,
		JibitAllowedIPs:                jibitAllowedIPs,
		Payment4APIKey:                 payment4APIKey,
		Payment4IPNSecret:              payment4IPNSecret,
		Payment4BaseURL:                payment4BaseURL,
		Payment4Sandbox:                payment4Sandbox,
		PaymentRateLimit:               paymentRateLimit,
		PaymentRateWindow:              paymentRateWindow,
		WebhookSecretNowPayments:       nowPaymentsIPNSecret,
		MinDepositCents:                minDepositCents,
		MaxDepositCents:                maxDepositCents,
		MinDepositIRR:                  minDepositIRR,
		MaxDepositIRR:                  maxDepositIRR,
		MinWithdrawCents:               minWithdrawCents,
		MaxWithdrawCents:               maxWithdrawCents,
		WithdrawFeeCents:               withdrawFeeCents,
		WithdrawFeePercent:             withdrawFeePercent,
		DailyWithdrawAmountCents:       dailyWithdrawAmountCents,
		MonthlyWithdrawAmountCents:     monthlyWithdrawAmountCents,
		DailyWithdrawCount:             dailyWithdrawCount,
		MonthlyWithdrawCount:           monthlyWithdrawCount,
		ExchangeRateNobitexURL:         exchangeRateNobitexURL,
		ExchangeRateStaticUSDIRR:       exchangeRateStaticUSDIRR,
		ExchangeRateCacheTTL:           exchangeRateCacheTTL,
		CleanupInterval:                cleanupInterval,
		CleanupOrphanedAfter:           cleanupOrphanedAfter,
		InquiryInterval:                inquiryInterval,
		InquiryMaxAge:                  inquiryMaxAge,
		ExpiryInterval:                 expiryInterval,
		ExpiryThreshold:                expiryThreshold,
		PayoutExpiryThreshold:          payoutExpiryThreshold,
		ProcessingPayoutAlertThreshold: processingPayoutAlertThreshold,
	}
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

// getSecretOrEnv reads from Docker secret file first, then falls back to env var.
func getSecretOrEnv(secretEnvKey, envKey, defaultValue string) string {
	// Try reading from secret file
	secretFileKey := secretEnvKey + "_FILE"
	if secretFile := os.Getenv(secretFileKey); secretFile != "" {
		if data, err := os.ReadFile(secretFile); err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	// Fall back to environment variable
	if secretEnvKey != "" {
		if v := os.Getenv(secretEnvKey); v != "" {
			return v
		}
	}

	if envKey != "" {
		if v := os.Getenv(envKey); v != "" {
			return v
		}
	}

	return defaultValue
}
