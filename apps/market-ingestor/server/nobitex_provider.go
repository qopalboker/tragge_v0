package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// NobitexConfig holds configuration for the Nobitex crypto price feed.
type NobitexConfig struct {
	Token        string        // API auth token (optional for public endpoints)
	PollInterval time.Duration // polling interval, default 2s
	USDTUSDRate  float64       // USDT→USD conversion rate, default 1.0
	BaseURL      string        // API base URL, default "https://apiv2.nobitex.ir"
	Symbols      []string      // Nobitex symbol names e.g. ["BTCUSDT", "ETHUSDT"]
	Enabled      bool
}

// NobitexOrderbookEntry represents a single symbol's orderbook from the Nobitex API.
type NobitexOrderbookEntry struct {
	LastUpdate     int64      `json:"lastUpdate"`
	LastTradePrice string     `json:"lastTradePrice,omitempty"`
	Asks           [][]string `json:"asks"`
	Bids           [][]string `json:"bids"`
}

// NobitexCryptoFeed is an independent REST-polling price feed for crypto symbols
// using the Nobitex exchange API. It runs as a separate goroutine alongside
// ProviderManager and calls the same TickHandler callback.
type NobitexCryptoFeed struct {
	config      NobitexConfig
	tickHandler TickHandler
	registry    *SymbolRegistry
	logger      *zap.Logger
	httpClient  *http.Client

	// Symbol mappings (protected by symMu for dynamic add/remove)
	nobitexToCanonical map[string]string // "BTCUSDT" → "BTC/USD"
	canonicalToNobitex map[string]string // "BTC/USD" → "BTCUSDT"
	symMu              sync.RWMutex

	// Atomic state for thread safety
	connected   atomic.Bool
	lastTick    atomic.Int64  // unix timestamp of last successful tick
	lastPollErr atomic.Value  // stores last error string
	pollCount   atomic.Int64
	tickCount   atomic.Int64
	errorCount  atomic.Int64

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewNobitexCryptoFeed creates a new NobitexCryptoFeed with the given configuration.
func NewNobitexCryptoFeed(config NobitexConfig, tickHandler TickHandler, registry *SymbolRegistry, logger *zap.Logger) *NobitexCryptoFeed {
	if config.PollInterval <= 0 {
		config.PollInterval = 2 * time.Second
	}
	if config.USDTUSDRate <= 0 {
		config.USDTUSDRate = 1.0
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://apiv2.nobitex.ir"
	}

	ctx, cancel := context.WithCancel(context.Background())

	feed := &NobitexCryptoFeed{
		config:      config,
		tickHandler: tickHandler,
		registry:    registry,
		logger:      logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		nobitexToCanonical: make(map[string]string),
		canonicalToNobitex: make(map[string]string),
		ctx:                ctx,
		cancel:             cancel,
	}

	feed.buildSymbolMappings()
	return feed
}

// buildSymbolMappings populates the nobitexToCanonical and canonicalToNobitex maps
// from the SymbolRegistry or config.Symbols with heuristic conversion.
func (f *NobitexCryptoFeed) buildSymbolMappings() {
	// If registry has ToNobitex mappings, use those
	if f.registry != nil && len(f.registry.ToNobitex) > 0 {
		for canonical, nb := range f.registry.ToNobitex {
			upper := strings.ToUpper(nb)
			f.nobitexToCanonical[upper] = canonical
			f.canonicalToNobitex[canonical] = upper
		}
		f.logger.Info("built Nobitex symbol mappings from registry",
			zap.Int("count", len(f.nobitexToCanonical)))
		return
	}

	// Fallback: use config.Symbols with heuristic conversion
	for _, sym := range f.config.Symbols {
		upper := strings.ToUpper(strings.TrimSpace(sym))
		if upper == "" {
			continue
		}
		// Heuristic: BTCUSDT → BTC/USD (strip USDT suffix, add /USD)
		if strings.HasSuffix(upper, "USDT") {
			base := strings.TrimSuffix(upper, "USDT")
			canonical := base + "/USD"
			f.nobitexToCanonical[upper] = canonical
			f.canonicalToNobitex[canonical] = upper
		}
	}

	f.logger.Info("built Nobitex symbol mappings from config",
		zap.Int("count", len(f.nobitexToCanonical)))
}

// Start begins the polling loop in a background goroutine.
func (f *NobitexCryptoFeed) Start() {
	if len(f.nobitexToCanonical) == 0 {
		f.logger.Warn("NobitexCryptoFeed: no symbols configured, not starting")
		return
	}

	f.wg.Add(1)
	go f.pollLoop()

	f.logger.Info("NobitexCryptoFeed started",
		zap.Int("symbols", len(f.nobitexToCanonical)),
		zap.Duration("poll_interval", f.config.PollInterval),
		zap.Float64("usdt_usd_rate", f.config.USDTUSDRate),
	)
}

// Stop gracefully shuts down the polling loop and waits for completion.
func (f *NobitexCryptoFeed) Stop() {
	f.cancel()
	f.wg.Wait()
	f.logger.Info("NobitexCryptoFeed stopped",
		zap.Int64("total_polls", f.pollCount.Load()),
		zap.Int64("total_ticks", f.tickCount.Load()),
		zap.Int64("total_errors", f.errorCount.Load()),
	)
}

// AddSymbol dynamically adds a crypto symbol to the polling filter.
// The canonical format is "BTC/USD" which maps to Nobitex "BTCUSDT".
func (f *NobitexCryptoFeed) AddSymbol(canonical string) {
	nobitex := canonicalToNobitexSymbol(canonical)
	if nobitex == "" {
		return
	}
	f.symMu.Lock()
	defer f.symMu.Unlock()
	f.nobitexToCanonical[nobitex] = canonical
	f.canonicalToNobitex[canonical] = nobitex
	f.logger.Info("Nobitex: added symbol", zap.String("canonical", canonical), zap.String("nobitex", nobitex))
}

// RemoveSymbol dynamically removes a crypto symbol from the polling filter.
func (f *NobitexCryptoFeed) RemoveSymbol(canonical string) {
	f.symMu.Lock()
	defer f.symMu.Unlock()
	if nobitex, ok := f.canonicalToNobitex[canonical]; ok {
		delete(f.nobitexToCanonical, nobitex)
		delete(f.canonicalToNobitex, canonical)
		f.logger.Info("Nobitex: removed symbol", zap.String("canonical", canonical))
	}
}

// canonicalToNobitexSymbol converts "BTC/USD" → "BTCUSDT".
func canonicalToNobitexSymbol(canonical string) string {
	parts := strings.SplitN(canonical, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToUpper(parts[0]) + "USDT"
}

// pollLoop runs the periodic polling cycle until the context is cancelled.
func (f *NobitexCryptoFeed) pollLoop() {
	defer f.wg.Done()

	ticker := time.NewTicker(f.config.PollInterval)
	defer ticker.Stop()

	// Poll immediately on start
	f.poll()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			f.poll()
		}
	}
}

// poll fetches all orderbooks from Nobitex and dispatches ticks for tracked symbols.
func (f *NobitexCryptoFeed) poll() {
	f.pollCount.Add(1)

	orderbooks, err := f.fetchAllOrderbooks()
	if err != nil {
		f.errorCount.Add(1)
		f.connected.Store(false)
		f.lastPollErr.Store(err.Error())
		f.logger.Warn("NobitexCryptoFeed poll failed",
			zap.Error(err),
			zap.Int64("poll_count", f.pollCount.Load()),
		)
		return
	}

	f.connected.Store(true)
	f.lastPollErr.Store("")

	// Take a read lock for the symbol mappings (may be modified by DynamicSymbolManager)
	f.symMu.RLock()
	nobitexMap := make(map[string]string, len(f.nobitexToCanonical))
	for k, v := range f.nobitexToCanonical {
		nobitexMap[k] = v
	}
	f.symMu.RUnlock()

	var emitted int64
	for key, ob := range orderbooks {
		upper := strings.ToUpper(key)

		canonical, tracked := nobitexMap[upper]
		if !tracked {
			continue
		}

		bid, ask, last, err := f.extractPrices(&ob)
		if err != nil {
			f.logger.Debug("NobitexCryptoFeed: skipping symbol due to price extraction error",
				zap.String("symbol", upper),
				zap.Error(err),
			)
			continue
		}

		// Apply USDT→USD conversion
		bid *= f.config.USDTUSDRate
		ask *= f.config.USDTUSDRate
		last *= f.config.USDTUSDRate

		// Convert lastUpdate from milliseconds to Unix seconds
		ts := ob.LastUpdate / 1000
		if ts <= 0 {
			ts = time.Now().Unix()
		}

		f.tickHandler(canonical, last, bid, ask, 0, ts, "nobitex")
		f.lastTick.Store(time.Now().Unix())
		emitted++
	}

	if emitted > 0 {
		f.tickCount.Add(emitted)
	}
}

// fetchAllOrderbooks calls the Nobitex orderbook/all endpoint and returns parsed entries.
func (f *NobitexCryptoFeed) fetchAllOrderbooks() (map[string]NobitexOrderbookEntry, error) {
	url := f.config.BaseURL + "/v3/orderbook/all"

	req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "TraderBot/Tragge")
	if f.config.Token != "" {
		req.Header.Set("Authorization", "Token "+f.config.Token)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("rate limited (HTTP 403)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Read body with 1MB limit
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// First check for status field
	var statusCheck struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &statusCheck); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}
	if statusCheck.Status != "ok" {
		return nil, fmt.Errorf("API returned status: %q", statusCheck.Status)
	}

	// Unmarshal as generic map to iterate symbol keys
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal raw: %w", err)
	}

	result := make(map[string]NobitexOrderbookEntry, len(raw))
	for key, val := range raw {
		if key == "status" {
			continue
		}

		var entry NobitexOrderbookEntry
		if err := json.Unmarshal(val, &entry); err != nil {
			// Skip entries that don't match expected structure
			continue
		}
		result[key] = entry
	}

	return result, nil
}

// extractPrices parses bid, ask, and last prices from a NobitexOrderbookEntry.
func (f *NobitexCryptoFeed) extractPrices(ob *NobitexOrderbookEntry) (bid, ask, last float64, err error) {
	// Parse lastTradePrice
	if ob.LastTradePrice != "" {
		var parseErr error
		last, parseErr = strconv.ParseFloat(ob.LastTradePrice, 64)
		if parseErr != nil {
			f.logger.Warn("failed to parse Nobitex last trade price",
				zap.String("raw_value", ob.LastTradePrice),
				zap.Error(parseErr))
			last = 0
		}
	}

	// Parse best bid (first entry = highest bid)
	if len(ob.Bids) > 0 && len(ob.Bids[0]) > 0 {
		var parseErr error
		bid, parseErr = strconv.ParseFloat(ob.Bids[0][0], 64)
		if parseErr != nil {
			f.logger.Warn("failed to parse Nobitex bid price",
				zap.String("raw_value", ob.Bids[0][0]),
				zap.Error(parseErr))
			bid = 0
		}
	}

	// Parse best ask (first entry = lowest ask)
	if len(ob.Asks) > 0 && len(ob.Asks[0]) > 0 {
		var parseErr error
		ask, parseErr = strconv.ParseFloat(ob.Asks[0][0], 64)
		if parseErr != nil {
			f.logger.Warn("failed to parse Nobitex ask price",
				zap.String("raw_value", ob.Asks[0][0]),
				zap.Error(parseErr))
			ask = 0
		}
	}

	// If last is zero but bid and ask exist, use midpoint
	if last == 0 && bid > 0 && ask > 0 {
		last = (bid + ask) / 2
	}

	// Must have at least one valid price
	if last == 0 && bid == 0 && ask == 0 {
		return 0, 0, 0, fmt.Errorf("no valid prices found")
	}

	return bid, ask, last, nil
}

// IsConnected returns whether the feed is currently connected (last poll succeeded).
func (f *NobitexCryptoFeed) IsConnected() bool {
	return f.connected.Load()
}

// LastTickTime returns the Unix timestamp of the last successful tick emission.
func (f *NobitexCryptoFeed) LastTickTime() int64 {
	return f.lastTick.Load()
}

// Stats returns all feed statistics for the health endpoint.
func (f *NobitexCryptoFeed) Stats() map[string]interface{} {
	f.symMu.RLock()
	symbolCount := len(f.nobitexToCanonical)
	f.symMu.RUnlock()

	stats := map[string]interface{}{
		"connected":   f.connected.Load(),
		"poll_count":  f.pollCount.Load(),
		"tick_count":  f.tickCount.Load(),
		"error_count": f.errorCount.Load(),
		"symbols":     symbolCount,
		"last_tick":   f.lastTick.Load(),
	}

	if v := f.lastPollErr.Load(); v != nil {
		stats["last_error"] = v
	} else {
		stats["last_error"] = ""
	}

	return stats
}

// defaultNobitexCryptoSymbols returns the default list of 24 USDT trading pairs
// available on the Nobitex exchange.
func defaultNobitexCryptoSymbols() []string {
	return []string{
		"BTCUSDT",
		"ETHUSDT",
		"SOLUSDT",
		"DOGEUSDT",
		"XRPUSDT",
		"ADAUSDT",
		"AVAXUSDT",
		"LINKUSDT",
		"DOTUSDT",
		"POLUSDT",
		"SHIBUSDT",
		"LTCUSDT",
		"UNIUSDT",
		"XLMUSDT",
		"NEARUSDT",
		"AAVEUSDT",
		"SUIUSDT",
		"PEPEUSDT",
		"APTUSDT",
		"BCHUSDT",
		"CROUSDT",
		"HBARUSDT",
		"ICPUSDT",
		"VETUSDT",
	}
}
