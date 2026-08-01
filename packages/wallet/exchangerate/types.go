package exchangerate

import "time"

// Redis cache key for exchange rate
const CacheKey = "exchange_rate:usd_irr"

// Default values
const (
	DefaultCacheTTL     = 60 * time.Second
	DefaultNobitexURL   = "https://apiv2.nobitex.ir"
	DefaultStaticUSDIRR = 8500000.0 // 1 USD = 8,500,000 IRR (update periodically)
)

// Rate represents a USD to IRR/IRT exchange rate.
type Rate struct {
	USDToIRR  float64   `json:"usd_to_irr"`
	USDToIRT  float64   `json:"usd_to_irt"`
	Source    string    `json:"source"`     // "nobitex", "static", "cached"
	FetchedAt time.Time `json:"fetched_at"`
}

// Config holds configuration for the exchange rate service.
type Config struct {
	NobitexBaseURL string
	StaticUSDToIRR float64
	CacheTTL       time.Duration
	RedisAddr      string // Optional Redis address for distributed cache
}
