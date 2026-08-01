package v1

// ContestTemplate defines a pre-configured contest template for quick creation.
type ContestTemplate struct {
	// Key is the unique identifier for the template (e.g., "crypto_rush_30m")
	Key string `json:"key"`

	// Name is the display name for the template
	Name string `json:"name"`

	// Description explains the contest type
	Description string `json:"description"`

	// DurationType is the category of duration
	DurationType ContestDurationType `json:"duration_type"`

	// DurationMinutes is the exact duration in minutes
	DurationMinutes int `json:"duration_minutes"`

	// EntryFeeCents is the entry fee in cents (500 = $5)
	EntryFeeCents int `json:"entry_fee_cents"`

	// CommissionRate is the platform commission as percentage (20.00 = 20%)
	CommissionRate float64 `json:"commission_rate"`

	// QtyAllocation is the starting virtual currency allocation
	QtyAllocation int64 `json:"qty_allocation"`

	// AssetClass is the category of tradable assets
	AssetClass AssetClass `json:"asset_class"`

	// Symbols is the list of allowed trading symbols
	Symbols []string `json:"symbols"`

	// MaxParticipants is the maximum number of participants (0 = unlimited)
	MaxParticipants int `json:"max_participants"`

	// MinParticipants is the minimum required to start (default 2)
	MinParticipants int `json:"min_participants"`

	// IsFree indicates if this is a free practice contest
	IsFree bool `json:"is_free"`

	// AutoStart indicates if contest starts automatically when conditions are met
	AutoStart bool `json:"auto_start"`
}

// ContestConfig represents the full configuration for a contest.
type ContestConfig struct {
	// DurationType is the duration category
	DurationType ContestDurationType `json:"duration_type"`

	// DurationMinutes is the exact duration (can override DurationType default)
	DurationMinutes int `json:"duration_minutes,omitempty"`

	// EntryFeeCents is the entry fee in cents
	EntryFeeCents int `json:"entry_fee_cents"`

	// CommissionRate is the platform commission as percentage
	CommissionRate float64 `json:"commission_rate"`

	// QtyAllocation is the starting virtual currency allocation
	QtyAllocation int64 `json:"qty_allocation"`

	// AssetClass is the category of tradable assets
	AssetClass AssetClass `json:"asset_class"`

	// Symbols is the list of allowed trading symbols
	Symbols []string `json:"symbols,omitempty"`

	// MaxParticipants is the maximum number of participants (0 = unlimited)
	MaxParticipants int `json:"max_participants,omitempty"`

	// MinParticipants is the minimum required to start
	MinParticipants int `json:"min_participants"`

	// IsFree indicates if this is a free practice contest
	IsFree bool `json:"is_free"`

	// AutoStart indicates if contest starts automatically
	AutoStart bool `json:"auto_start"`
}

// Default symbol sets for each asset class
var (
	// ForexMajorPairs contains the major forex currency pairs
	ForexMajorPairs = []string{
		"EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD",
	}

	// ForexExtendedPairs contains major + cross pairs
	ForexExtendedPairs = []string{
		"EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD",
		"NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY",
		"AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD",
	}

	// ForexFullPairs contains all 23 forex pairs (majors + crosses + metals)
	ForexFullPairs = []string{
		"EUR/USD", "GBP/USD", "USD/JPY", "USD/CHF", "AUD/USD",
		"NZD/USD", "USD/CAD", "EUR/GBP", "EUR/JPY", "GBP/JPY",
		"AUD/JPY", "CHF/JPY", "EUR/AUD", "GBP/AUD", "EUR/CAD",
		"AUD/NZD", "CAD/JPY", "NZD/JPY", "EUR/CHF", "GBP/CHF",
		"GBP/CAD", "XAU/USD", "XAG/USD",
	}

	// CryptoMajorAssets contains top crypto assets
	CryptoMajorAssets = []string{
		"BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD",
	}

	// CryptoExtendedAssets contains 12 crypto assets
	CryptoExtendedAssets = []string{
		"BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD",
		"ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "POL/USD",
		"SHIB/USD", "LTC/USD",
	}

	// CryptoFullAssets contains all 24 crypto assets
	CryptoFullAssets = []string{
		"BTC/USD", "ETH/USD", "SOL/USD", "DOGE/USD", "XRP/USD",
		"ADA/USD", "AVAX/USD", "LINK/USD", "DOT/USD", "POL/USD",
		"SHIB/USD", "LTC/USD", "UNI/USD", "ETC/USD", "XLM/USD",
		"NEAR/USD", "AAVE/USD", "SUI/USD", "PEPE/USD", "ARB/USD",
		"OP/USD", "APT/USD", "INJ/USD", "RENDER/USD",
	}

	// CommodityPairs contains commodity instruments.
	// XAU/USD and XAG/USD route through the Massive forex WebSocket (metals
	// are part of the forex WS using the C.XAU-USD / C.XAG-USD subscription format).
	//
	// TODO(USOIL): Implement TwelveData REST polling adapter for USOIL.
	// USOIL special handling (Decision: Option B — TwelveData REST polling):
	//   Massive forex WS does not support crude oil futures. USOIL prices should
	//   be polled from TwelveData REST API (GET /price?symbol=USOIL) on a 5-10s
	//   interval and injected into the tick pipeline. Until the polling adapter is
	//   implemented, USOIL is excluded from the market-ingestor SYMBOLS env var
	//   but is listed here so contests can reference it once data is available.
	CommodityPairs = []string{
		"XAU/USD", "XAG/USD", "USOIL",
	}

	// StocksUSTop30 contains top 30 US equities
	StocksUSTop30 = []string{
		"AAPL", "MSFT", "GOOGL", "AMZN", "TSLA",
		"META", "NVDA", "BRK.B", "JPM", "JNJ",
		"V", "PG", "UNH", "HD", "MA",
		"DIS", "ADBE", "NFLX", "CRM", "PYPL",
		"INTC", "CSCO", "VZ", "KO", "PFE",
		"MRK", "ABT", "WMT", "NKE", "XOM",
	}
)

// ContestTemplates contains all pre-defined contest templates.
// Use GetContestTemplate to retrieve a specific template by key.
var ContestTemplates = map[string]ContestTemplate{
	// Crypto templates
	"crypto_rush_30m": {
		Key:             "crypto_rush_30m",
		Name:            "Crypto Rush 30min",
		Description:     "Fast-paced 30-minute crypto trading competition",
		DurationType:    ContestDurationRush30Min,
		DurationMinutes: 30,
		EntryFeeCents:   500, // $5
		CommissionRate:  20.00,
		QtyAllocation:   50000, // 10 QTY
		AssetClass:      AssetClassCrypto,
		Symbols:         CryptoMajorAssets,
		MaxParticipants: 100,
		MinParticipants: 2,
		IsFree:          false,
		AutoStart:       false,
	},
	"crypto_hourly": {
		Key:             "crypto_hourly",
		Name:            "Crypto Hourly",
		Description:     "One-hour crypto trading tournament",
		DurationType:    ContestDurationHourly,
		DurationMinutes: 60,
		EntryFeeCents:   1000, // $10
		CommissionRate:  20.00,
		QtyAllocation:   100000,
		AssetClass:      AssetClassCrypto,
		Symbols:         CryptoExtendedAssets[:8],
		MaxParticipants: 200,
		MinParticipants: 2,
		IsFree:          false,
		AutoStart:       false,
	},
	"crypto_4hour": {
		Key:             "crypto_4hour",
		Name:            "Crypto 4-Hour Tournament",
		Description:     "Extended 4-hour crypto competition",
		DurationType:    ContestDurationFourHour,
		DurationMinutes: 240,
		EntryFeeCents:   2500, // $25
		CommissionRate:  20.00,
		QtyAllocation:   150000,
		AssetClass:      AssetClassCrypto,
		Symbols:         CryptoExtendedAssets,
		MaxParticipants: 300,
		MinParticipants: 3,
		IsFree:          false,
		AutoStart:       false,
	},
	"crypto_daily": {
		Key:             "crypto_daily",
		Name:            "Crypto Daily Challenge",
		Description:     "Full-day crypto trading competition with 20 QTY allocation",
		DurationType:    ContestDurationDaily,
		DurationMinutes: 1440,
		EntryFeeCents:   5000, // $50
		CommissionRate:  20.00,
		QtyAllocation:   200000, // 20 QTY
		AssetClass:      AssetClassCrypto,
		Symbols:         CryptoExtendedAssets,
		MaxParticipants: 500,
		MinParticipants: 5,
		IsFree:          false,
		AutoStart:       false,
	},
	"crypto_weekly": {
		Key:             "crypto_weekly",
		Name:            "Crypto Weekly Grand Prix",
		Description:     "Week-long crypto competition for serious traders",
		DurationType:    ContestDurationWeekly,
		DurationMinutes: 10080,
		EntryFeeCents:   10000, // $100
		CommissionRate:  20.00,
		QtyAllocation:   500000,
		AssetClass:      AssetClassCrypto,
		Symbols:         CryptoFullAssets,
		MaxParticipants: 1000,
		MinParticipants: 10,
		IsFree:          false,
		AutoStart:       false,
	},
	"crypto_high_stakes": {
		Key:             "crypto_high_stakes",
		Name:            "High Stakes Crypto",
		Description:     "Professional-level crypto trading competition with $100 entry",
		DurationType:    ContestDurationFourHour,
		DurationMinutes: 240,
		EntryFeeCents:   10000, // $100
		CommissionRate:  20.00,
		QtyAllocation:   200000,
		AssetClass:      AssetClassCrypto,
		Symbols:         CryptoExtendedAssets,
		MaxParticipants: 50,
		MinParticipants: 5,
		IsFree:          false,
		AutoStart:       false,
	},
	"crypto_free_practice": {
		Key:             "crypto_free_practice",
		Name:            "Free Crypto Practice",
		Description:     "Free practice tournament to learn crypto trading",
		DurationType:    ContestDurationHourly,
		DurationMinutes: 60,
		EntryFeeCents:   0,
		CommissionRate:  0,
		QtyAllocation:   100000,
		AssetClass:      AssetClassCrypto,
		Symbols:         CryptoMajorAssets,
		MaxParticipants: 1000,
		MinParticipants: 2,
		IsFree:          true,
		AutoStart:       true,
	},

	// Forex templates
	"forex_rush_30m": {
		Key:             "forex_rush_30m",
		Name:            "Forex Rush 30min",
		Description:     "Quick 30-minute forex trading competition",
		DurationType:    ContestDurationRush30Min,
		DurationMinutes: 30,
		EntryFeeCents:   500, // $5
		CommissionRate:  20.00,
		QtyAllocation:   50000, // 10 QTY
		AssetClass:      AssetClassForex,
		Symbols:         ForexMajorPairs,
		MaxParticipants: 100,
		MinParticipants: 2,
		IsFree:          false,
		AutoStart:       false,
	},
	"forex_hourly": {
		Key:             "forex_hourly",
		Name:            "Forex Hourly",
		Description:     "One-hour forex trading tournament with 15+ pairs",
		DurationType:    ContestDurationHourly,
		DurationMinutes: 60,
		EntryFeeCents:   1000, // $10
		CommissionRate:  20.00,
		QtyAllocation:   100000,
		AssetClass:      AssetClassForex,
		Symbols:         ForexExtendedPairs,
		MaxParticipants: 200,
		MinParticipants: 2,
		IsFree:          false,
		AutoStart:       false,
	},
	"forex_4hour": {
		Key:             "forex_4hour",
		Name:            "Forex 4-Hour Tournament",
		Description:     "Extended 4-hour forex competition with comprehensive pair coverage",
		DurationType:    ContestDurationFourHour,
		DurationMinutes: 240,
		EntryFeeCents:   2500, // $25
		CommissionRate:  20.00,
		QtyAllocation:   150000,
		AssetClass:      AssetClassForex,
		Symbols:         ForexExtendedPairs,
		MaxParticipants: 300,
		MinParticipants: 3,
		IsFree:          false,
		AutoStart:       false,
	},
	"forex_daily": {
		Key:             "forex_daily",
		Name:            "Forex Daily Championship",
		Description:     "24-hour forex trading championship with 33+ pairs",
		DurationType:    ContestDurationDaily,
		DurationMinutes: 1440,
		EntryFeeCents:   5000, // $50
		CommissionRate:  20.00,
		QtyAllocation:   200000, // 20 QTY
		AssetClass:      AssetClassForex,
		Symbols:         ForexFullPairs,
		MaxParticipants: 500,
		MinParticipants: 5,
		IsFree:          false,
		AutoStart:       false,
	},
	"forex_weekly": {
		Key:             "forex_weekly",
		Name:            "Forex Weekly Grand Prix",
		Description:     "Week-long forex competition for serious traders",
		DurationType:    ContestDurationWeekly,
		DurationMinutes: 10080,
		EntryFeeCents:   10000, // $100
		CommissionRate:  20.00,
		QtyAllocation:   500000,
		AssetClass:      AssetClassForex,
		Symbols:         ForexFullPairs,
		MaxParticipants: 1000,
		MinParticipants: 10,
		IsFree:          false,
		AutoStart:       false,
	},
	"forex_high_stakes": {
		Key:             "forex_high_stakes",
		Name:            "High Stakes Forex",
		Description:     "Professional-level forex trading competition with $100 entry",
		DurationType:    ContestDurationFourHour,
		DurationMinutes: 240,
		EntryFeeCents:   10000, // $100
		CommissionRate:  20.00,
		QtyAllocation:   200000,
		AssetClass:      AssetClassForex,
		Symbols:         ForexExtendedPairs,
		MaxParticipants: 50,
		MinParticipants: 5,
		IsFree:          false,
		AutoStart:       false,
	},
	"forex_free_practice": {
		Key:             "forex_free_practice",
		Name:            "Free Forex Practice",
		Description:     "Free practice tournament to learn forex trading",
		DurationType:    ContestDurationHourly,
		DurationMinutes: 60,
		EntryFeeCents:   0,
		CommissionRate:  0,
		QtyAllocation:   100000,
		AssetClass:      AssetClassForex,
		Symbols:         ForexMajorPairs,
		MaxParticipants: 1000,
		MinParticipants: 2,
		IsFree:          true,
		AutoStart:       true,
	},

	// Stocks templates (coming soon)
	"stocks_daily": {
		Key:             "stocks_daily",
		Name:            "US Stocks Daily",
		Description:     "Daily competition trading top 30 US equities",
		DurationType:    ContestDurationDaily,
		DurationMinutes: 1440,
		EntryFeeCents:   5000, // $50
		CommissionRate:  20.00,
		QtyAllocation:   200000,
		AssetClass:      AssetClassStocks,
		Symbols:         StocksUSTop30,
		MaxParticipants: 500,
		MinParticipants: 5,
		IsFree:          false,
		AutoStart:       false,
	},
}

// GetContestTemplate returns a contest template by key.
// Returns nil if the template doesn't exist.
func GetContestTemplate(key string) *ContestTemplate {
	if t, ok := ContestTemplates[key]; ok {
		return &t
	}
	return nil
}

// ListContestTemplates returns all available contest templates.
func ListContestTemplates() []ContestTemplate {
	templates := make([]ContestTemplate, 0, len(ContestTemplates))
	for _, t := range ContestTemplates {
		templates = append(templates, t)
	}
	return templates
}

// ListContestTemplatesByAssetClass returns templates filtered by asset class.
func ListContestTemplatesByAssetClass(assetClass AssetClass) []ContestTemplate {
	var templates []ContestTemplate
	for _, t := range ContestTemplates {
		if t.AssetClass == assetClass {
			templates = append(templates, t)
		}
	}
	return templates
}

// ListContestTemplatesByDuration returns templates filtered by duration type.
func ListContestTemplatesByDuration(durationType ContestDurationType) []ContestTemplate {
	var templates []ContestTemplate
	for _, t := range ContestTemplates {
		if t.DurationType == durationType {
			templates = append(templates, t)
		}
	}
	return templates
}

// ListFreeContestTemplates returns all free practice templates.
func ListFreeContestTemplates() []ContestTemplate {
	var templates []ContestTemplate
	for _, t := range ContestTemplates {
		if t.IsFree {
			templates = append(templates, t)
		}
	}
	return templates
}

// GetDefaultSymbols returns the default symbol list for a given asset class and duration type.
// Shorter durations get fewer symbols; longer durations get the full set.
func GetDefaultSymbols(assetClass AssetClass, durationType ContestDurationType) []string {
	switch assetClass {
	case AssetClassCrypto:
		switch durationType {
		case ContestDurationRush30Min:
			return CryptoMajorAssets
		case ContestDurationHourly, ContestDurationFourHour:
			return CryptoExtendedAssets
		case ContestDurationDaily, ContestDurationWeekly:
			return CryptoFullAssets
		default:
			return CryptoMajorAssets
		}
	case AssetClassForex:
		switch durationType {
		case ContestDurationRush30Min:
			return ForexMajorPairs
		case ContestDurationHourly, ContestDurationFourHour:
			return ForexExtendedPairs
		case ContestDurationDaily, ContestDurationWeekly:
			return ForexFullPairs
		default:
			return ForexMajorPairs
		}
	case AssetClassStocks:
		return StocksUSTop30
	case AssetClassMixed:
		combined := make([]string, 0, len(CryptoMajorAssets)+len(ForexMajorPairs))
		combined = append(combined, CryptoMajorAssets...)
		combined = append(combined, ForexMajorPairs...)
		return combined
	default:
		return nil
	}
}
