package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

const (
	// Redis keys for spread configuration (must match market-ingestor)
	RedisSpreadConfigKey = "market:spread_config"
	RedisSpreadPubSubKey = "market:spread_updates"
)

// SpreadConfig holds the spread configuration for all symbols.
type SpreadConfig struct {
	DefaultSpreadBps   int            `json:"default_spread_bps"`
	SymbolSpreads      map[string]int `json:"symbol_spreads"`
	AssetClassDefaults map[string]int `json:"asset_class_defaults"`
}

// SpreadResponse represents the response for spread API endpoints.
type SpreadResponse struct {
	Config      SpreadConfig `json:"config"`
	LastUpdated string       `json:"last_updated,omitempty"`
}

// SymbolSpreadRequest represents a request to update a symbol's spread.
type SymbolSpreadRequest struct {
	SpreadBps int `json:"spread_bps"`
}

// SymbolSpreadResponse represents the response after updating a symbol's spread.
type SymbolSpreadResponse struct {
	Symbol    string `json:"symbol"`
	SpreadBps int    `json:"spread_bps"`
	Message   string `json:"message"`
}

// handleGetSpreads returns the current spread configuration.
// GET /api/admin/spreads
func (a *App) handleGetSpreads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": adminMsg.RedisUnavailable,
		})
		return
	}

	// Get spread config from Redis
	config, err := a.getSpreadConfigFromRedis(ctx)
	if err != nil {
		a.log().Warn("Failed to get spread config from Redis", zap.Error(err))
		// Return default config
		config = &SpreadConfig{
			DefaultSpreadBps:   10,
			SymbolSpreads:      make(map[string]int),
			AssetClassDefaults: make(map[string]int),
		}
	}

	resp := SpreadResponse{
		Config:      *config,
		LastUpdated: time.Now().UTC().Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleGetSymbolSpread returns the spread for a specific symbol.
// GET /api/admin/spreads/{symbol}
func (a *App) handleGetSymbolSpread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))

	if symbol == "" {
		validation.WriteBadRequest(w, "symbol is required")
		return
	}

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": adminMsg.RedisUnavailable,
		})
		return
	}

	config, err := a.getSpreadConfigFromRedis(ctx)
	if err != nil {
		a.log().Warn("Failed to get spread config from Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": adminMsg.InternalError,
		})
		return
	}

	// Look up spread: symbol-specific -> asset class -> default
	spreadBps := config.DefaultSpreadBps
	source := "default"

	if spread, ok := config.SymbolSpreads[symbol]; ok {
		spreadBps = spread
		source = "symbol"
	} else {
		// Check asset class defaults
		assetClass := classifySymbol(symbol)
		if spread, ok := config.AssetClassDefaults[string(assetClass)]; ok {
			spreadBps = spread
			source = string(assetClass)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"symbol":      symbol,
		"spread_bps":  spreadBps,
		"source":      source,
		"spread_pct":  float64(spreadBps) / 100.0,
		"half_spread": float64(spreadBps) / 2 / 10000.0,
	})
}

// handleUpdateSymbolSpread updates the spread for a specific symbol.
// PUT /api/admin/spreads/{symbol}
func (a *App) handleUpdateSymbolSpread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	actorUserID := auth.GetUserID(ctx)

	if symbol == "" {
		validation.WriteBadRequest(w, "symbol is required")
		return
	}

	var req SymbolSpreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate spread value
	v := validation.New()
	if req.SpreadBps < 0 {
		v.AddError("spread_bps", "min", "spread_bps must be non-negative")
	}
	if req.SpreadBps > 1000 { // Max 10% spread
		v.AddError("spread_bps", "max", adminMsg.SpreadBPSMax)
	}
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": adminMsg.RedisUnavailable,
		})
		return
	}

	// Get current config
	config, err := a.getSpreadConfigFromRedis(ctx)
	if err != nil {
		// Create new config if not exists
		config = &SpreadConfig{
			DefaultSpreadBps:   10,
			SymbolSpreads:      make(map[string]int),
			AssetClassDefaults: make(map[string]int),
		}
	}

	// Update symbol spread
	if config.SymbolSpreads == nil {
		config.SymbolSpreads = make(map[string]int)
	}
	config.SymbolSpreads[symbol] = req.SpreadBps

	// Save to Redis
	if err := a.saveSpreadConfigToRedis(ctx, config); err != nil {
		a.log().Error("Failed to save spread config to Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": adminMsg.InternalError,
		})
		return
	}

	// Notify market-ingestor instances
	if err := a.notifySpreadUpdate(ctx); err != nil {
		a.log().Warn("Failed to notify spread update", zap.Error(err))
	}

	// Log audit entry
	a.logAuditEvent(ctx, actorUserID, "spread.symbol.updated", "spread_config", symbol, map[string]interface{}{
		"symbol":     symbol,
		"spread_bps": req.SpreadBps,
	})

	a.log().Info("Updated symbol spread",
		zap.String("symbol", symbol),
		zap.Int("spread_bps", req.SpreadBps),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, SymbolSpreadResponse{
		Symbol:    symbol,
		SpreadBps: req.SpreadBps,
		Message:   adminMsg.SpreadUpdated,
	})
}

// handleDeleteSymbolSpread removes the spread override for a specific symbol.
// DELETE /api/admin/spreads/{symbol}
func (a *App) handleDeleteSymbolSpread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbol := strings.ToUpper(chi.URLParam(r, "symbol"))
	actorUserID := auth.GetUserID(ctx)

	if symbol == "" {
		validation.WriteBadRequest(w, "symbol is required")
		return
	}

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": adminMsg.RedisUnavailable,
		})
		return
	}

	// Get current config
	config, err := a.getSpreadConfigFromRedis(ctx)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": adminMsg.SpreadNotFound,
		})
		return
	}

	// Check if symbol has override
	if _, ok := config.SymbolSpreads[symbol]; !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": adminMsg.SpreadNotFound,
		})
		return
	}

	// Delete symbol override
	delete(config.SymbolSpreads, symbol)

	// Save to Redis
	if err := a.saveSpreadConfigToRedis(ctx, config); err != nil {
		a.log().Error("Failed to save spread config to Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": adminMsg.InternalError,
		})
		return
	}

	// Notify market-ingestor instances
	if err := a.notifySpreadUpdate(ctx); err != nil {
		a.log().Warn("Failed to notify spread update", zap.Error(err))
	}

	// Log audit entry
	a.logAuditEvent(ctx, actorUserID, "spread.symbol.deleted", "spread_config", symbol, map[string]interface{}{
		"symbol": symbol,
	})

	a.log().Info("Deleted symbol spread override",
		zap.String("symbol", symbol),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, map[string]string{
		"message": adminMsg.SpreadDeleted,
		"symbol":  symbol,
	})
}

// handleUpdateDefaultSpread updates the default spread.
// PUT /api/admin/spreads/defaults
func (a *App) handleUpdateDefaultSpread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var req struct {
		DefaultSpreadBps int `json:"default_spread_bps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate
	v := validation.New()
	if req.DefaultSpreadBps < 1 {
		v.AddError("default_spread_bps", "min", adminMsg.DefaultSpreadBPSMin)
	}
	if req.DefaultSpreadBps > 1000 {
		v.AddError("default_spread_bps", "max", adminMsg.DefaultSpreadBPSMax)
	}
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": adminMsg.RedisUnavailable,
		})
		return
	}

	config, err := a.getSpreadConfigFromRedis(ctx)
	if err != nil {
		config = &SpreadConfig{
			DefaultSpreadBps:   10,
			SymbolSpreads:      make(map[string]int),
			AssetClassDefaults: make(map[string]int),
		}
	}

	oldDefault := config.DefaultSpreadBps
	config.DefaultSpreadBps = req.DefaultSpreadBps

	if err := a.saveSpreadConfigToRedis(ctx, config); err != nil {
		a.log().Error("Failed to save spread config to Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": adminMsg.InternalError,
		})
		return
	}

	if err := a.notifySpreadUpdate(ctx); err != nil {
		a.log().Warn("Failed to notify spread update", zap.Error(err))
	}

	a.logAuditEvent(ctx, actorUserID, "spread.default.updated", "spread_config", "default", map[string]interface{}{
		"old_value": oldDefault,
		"new_value": req.DefaultSpreadBps,
	})

	a.log().Info("Updated default spread",
		zap.Int("old_default", oldDefault),
		zap.Int("new_default", req.DefaultSpreadBps),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":            adminMsg.DefaultSpreadUpdated,
		"default_spread_bps": req.DefaultSpreadBps,
	})
}

// handleUpdateAssetClassSpread updates the spread for an asset class.
// PUT /api/admin/spreads/asset-class/{class}
func (a *App) handleUpdateAssetClassSpread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	assetClass := chi.URLParam(r, "class")
	actorUserID := auth.GetUserID(ctx)

	validClasses := map[string]bool{
		"forex_major": true, "forex_minor": true, "forex_exotic": true,
		"crypto_major": true, "crypto_alt": true, "commodities": true,
		"equities": true,
	}
	if !validClasses[assetClass] {
		validation.WriteBadRequest(w, "invalid asset class")
		return
	}

	var req struct {
		SpreadBps int `json:"spread_bps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	v := validation.New()
	if req.SpreadBps < 0 {
		v.AddError("spread_bps", "min", "spread_bps must be non-negative")
	}
	if req.SpreadBps > 1000 {
		v.AddError("spread_bps", "max", adminMsg.SpreadBPSMax)
	}
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": adminMsg.RedisUnavailable,
		})
		return
	}

	config, err := a.getSpreadConfigFromRedis(ctx)
	if err != nil {
		config = &SpreadConfig{
			DefaultSpreadBps:   10,
			SymbolSpreads:      make(map[string]int),
			AssetClassDefaults: make(map[string]int),
		}
	}

	if config.AssetClassDefaults == nil {
		config.AssetClassDefaults = make(map[string]int)
	}
	config.AssetClassDefaults[assetClass] = req.SpreadBps

	if err := a.saveSpreadConfigToRedis(ctx, config); err != nil {
		a.log().Error("Failed to save spread config to Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": adminMsg.InternalError,
		})
		return
	}

	if err := a.notifySpreadUpdate(ctx); err != nil {
		a.log().Warn("Failed to notify spread update", zap.Error(err))
	}

	a.logAuditEvent(ctx, actorUserID, "spread.asset_class.updated", "spread_config", assetClass, map[string]interface{}{
		"asset_class": assetClass,
		"spread_bps":  req.SpreadBps,
	})

	a.log().Info("Updated asset class spread",
		zap.String("asset_class", assetClass),
		zap.Int("spread_bps", req.SpreadBps),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message":     adminMsg.AssetSpreadUpdated,
		"asset_class": assetClass,
		"spread_bps":  req.SpreadBps,
	})
}

// handleBulkUpdateSpreads allows bulk updating of spreads.
// PUT /api/admin/spreads/bulk
func (a *App) handleBulkUpdateSpreads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actorUserID := auth.GetUserID(ctx)

	var req struct {
		DefaultSpreadBps   *int           `json:"default_spread_bps,omitempty"`
		SymbolSpreads      map[string]int `json:"symbol_spreads,omitempty"`
		AssetClassDefaults map[string]int `json:"asset_class_defaults,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		validation.WriteBadRequest(w, "invalid request body")
		return
	}

	// Validate
	v := validation.New()
	if req.DefaultSpreadBps != nil {
		if *req.DefaultSpreadBps < 1 {
			v.AddError("default_spread_bps", "min", adminMsg.DefaultSpreadBPSMin)
		}
		if *req.DefaultSpreadBps > 1000 {
			v.AddError("default_spread_bps", "max", "default_spread_bps cannot exceed 1000")
		}
	}
	for symbol, spread := range req.SymbolSpreads {
		if spread < 0 || spread > 1000 {
			v.AddError("symbol_spreads."+symbol, "range", "spread must be between 0 and 1000")
		}
	}
	for assetClass, spread := range req.AssetClassDefaults {
		if spread < 0 || spread > 1000 {
			v.AddError("asset_class_defaults."+assetClass, "range", "spread must be between 0 and 1000")
		}
	}
	if v.HasErrors() {
		validation.WriteValidationError(w, v.Errors())
		return
	}

	if a.redis == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": adminMsg.RedisUnavailable,
		})
		return
	}

	config, err := a.getSpreadConfigFromRedis(ctx)
	if err != nil {
		config = &SpreadConfig{
			DefaultSpreadBps:   10,
			SymbolSpreads:      make(map[string]int),
			AssetClassDefaults: make(map[string]int),
		}
	}

	// Apply updates
	if req.DefaultSpreadBps != nil {
		config.DefaultSpreadBps = *req.DefaultSpreadBps
	}
	for symbol, spread := range req.SymbolSpreads {
		if config.SymbolSpreads == nil {
			config.SymbolSpreads = make(map[string]int)
		}
		config.SymbolSpreads[strings.ToUpper(symbol)] = spread
	}
	for assetClass, spread := range req.AssetClassDefaults {
		if config.AssetClassDefaults == nil {
			config.AssetClassDefaults = make(map[string]int)
		}
		config.AssetClassDefaults[assetClass] = spread
	}

	if err := a.saveSpreadConfigToRedis(ctx, config); err != nil {
		a.log().Error("Failed to save spread config to Redis", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": adminMsg.InternalError,
		})
		return
	}

	if err := a.notifySpreadUpdate(ctx); err != nil {
		a.log().Warn("Failed to notify spread update", zap.Error(err))
	}

	a.logAuditEvent(ctx, actorUserID, "spread.bulk.updated", "spread_config", "bulk", map[string]interface{}{
		"default_updated":       req.DefaultSpreadBps != nil,
		"symbols_updated":       len(req.SymbolSpreads),
		"asset_classes_updated": len(req.AssetClassDefaults),
	})

	a.log().Info("Bulk updated spreads",
		zap.Int("symbols_count", len(req.SymbolSpreads)),
		zap.Int("asset_classes_count", len(req.AssetClassDefaults)),
		zap.String("actor", actorUserID))

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": adminMsg.SpreadUpdated,
		"config":  config,
	})
}

// getSpreadConfigFromRedis retrieves the spread configuration from Redis.
func (a *App) getSpreadConfigFromRedis(ctx context.Context) (*SpreadConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	redisResult, err := a.circuits.ExecuteRedisWithResult(ctx,
		func(ctx context.Context) (interface{}, error) {
			return a.redis.Client().Get(ctx, RedisSpreadConfigKey).Bytes()
		},
	)
	if err != nil {
		return nil, err
	}
	data := redisResult.([]byte)

	var config SpreadConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// saveSpreadConfigToRedis saves the spread configuration to Redis.
func (a *App) saveSpreadConfigToRedis(ctx context.Context, config *SpreadConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return a.circuits.ExecuteRedis(ctx, func(ctx context.Context) error {
		return a.redis.Client().Set(ctx, RedisSpreadConfigKey, data, 0).Err()
	})
}

// notifySpreadUpdate publishes a notification to Redis pub/sub.
func (a *App) notifySpreadUpdate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return a.circuits.ExecuteRedis(ctx, func(ctx context.Context) error {
		return a.redis.Client().Publish(ctx, RedisSpreadPubSubKey, "update").Err()
	})
}

// AssetClass represents the classification of a financial instrument.
type AssetClass string

const (
	AssetClassForexMajor  AssetClass = "forex_major"
	AssetClassForexMinor  AssetClass = "forex_minor"
	AssetClassForexExotic AssetClass = "forex_exotic"
	AssetClassCryptoMajor AssetClass = "crypto_major"
	AssetClassCryptoAlt   AssetClass = "crypto_alt"
	AssetClassCommodities AssetClass = "commodities"
	AssetClassEquities    AssetClass = "equities"
	AssetClassUnknown     AssetClass = "unknown"
)

// classifySymbol determines the asset class of a symbol based on its name.
func classifySymbol(symbol string) AssetClass {
	symbol = strings.ToUpper(symbol)

	// Forex major pairs
	forexMajors := []string{
		"EURUSD", "USDJPY", "GBPUSD", "USDCHF", "USDCAD", "AUDUSD", "NZDUSD",
	}
	for _, major := range forexMajors {
		if symbol == major {
			return AssetClassForexMajor
		}
	}

	// Forex minor pairs
	forexMinors := []string{
		"EURGBP", "EURJPY", "EURCHF", "EURAUD", "EURCAD", "EURNZD",
		"GBPJPY", "GBPCHF", "GBPAUD", "GBPCAD", "GBPNZD",
		"AUDJPY", "AUDNZD", "AUDCHF", "AUDCAD",
		"NZDJPY", "NZDCHF", "NZDCAD",
		"CADJPY", "CADCHF",
		"CHFJPY",
	}
	for _, minor := range forexMinors {
		if symbol == minor {
			return AssetClassForexMinor
		}
	}

	// Forex exotic pairs
	exoticCurrencies := []string{"TRY", "ZAR", "MXN", "PLN", "HUF", "CZK", "SEK", "NOK", "DKK", "SGD", "HKD", "THB", "INR", "BRL", "RUB"}
	for _, exotic := range exoticCurrencies {
		if strings.Contains(symbol, exotic) {
			return AssetClassForexExotic
		}
	}

	// Crypto major
	if strings.Contains(symbol, "BTC") || strings.Contains(symbol, "ETH") {
		return AssetClassCryptoMajor
	}

	// Crypto alt
	cryptoPrefixes := []string{"XRP", "LTC", "ADA", "DOT", "LINK", "SOL", "AVAX", "DOGE", "SHIB", "MATIC", "UNI", "ATOM"}
	for _, prefix := range cryptoPrefixes {
		if strings.HasPrefix(symbol, prefix) {
			return AssetClassCryptoAlt
		}
	}

	// Commodities
	commodities := []string{"XAUUSD", "XAGUSD", "XPTUSD", "XPDUSD", "WTIUSD", "BRNUSD", "NGAS", "COPPER", "GOLD", "SILVER"}
	for _, comm := range commodities {
		if symbol == comm || strings.HasPrefix(symbol, comm) {
			return AssetClassCommodities
		}
	}

	// Check for forex pattern
	if len(symbol) == 6 && isForexPair(symbol) {
		return AssetClassForexMinor
	}

	// Default to equities
	if len(symbol) <= 5 && isAllLetters(symbol) {
		return AssetClassEquities
	}

	return AssetClassUnknown
}

// isForexPair checks if a symbol looks like a forex pair.
func isForexPair(symbol string) bool {
	if len(symbol) != 6 {
		return false
	}
	currencies := []string{"USD", "EUR", "GBP", "JPY", "CHF", "CAD", "AUD", "NZD", "CNH", "CNY"}
	first := symbol[:3]
	second := symbol[3:]
	foundFirst, foundSecond := false, false
	for _, curr := range currencies {
		if first == curr {
			foundFirst = true
		}
		if second == curr {
			foundSecond = true
		}
	}
	return foundFirst && foundSecond
}

// isAllLetters checks if a string contains only letters.
func isAllLetters(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// Ensure strconv is used (for potential future use)
var _ = strconv.Itoa
