package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"go.uber.org/zap"
)

const (
	// Redis keys for spread configuration
	RedisSpreadConfigKey    = "market:spread_config"
	RedisSpreadSymbolPrefix = "market:spread:symbol:"
	RedisSpreadPubSubKey    = "market:spread_updates"
)

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

// SpreadConfig holds the spread configuration for all symbols.
type SpreadConfig struct {
	DefaultSpreadBps   int            `json:"default_spread_bps"`
	SymbolSpreads      map[string]int `json:"symbol_spreads"`
	AssetClassDefaults map[string]int `json:"asset_class_defaults"`
}

// SpreadManager manages per-symbol spread configurations with Redis synchronization.
type SpreadManager struct {
	config     *SpreadConfig
	redis      *pkgredis.Client
	logger     *zap.Logger
	mu         sync.RWMutex
	configPath string
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewSpreadManager creates a new spread manager.
func NewSpreadManager(configPath string, redis *pkgredis.Client, logger *zap.Logger) (*SpreadManager, error) {
	sm := &SpreadManager{
		redis:      redis,
		logger:     logger,
		configPath: configPath,
		stopCh:     make(chan struct{}),
		config: &SpreadConfig{
			DefaultSpreadBps:   DefaultSpreadBps,
			SymbolSpreads:      make(map[string]int),
			AssetClassDefaults: make(map[string]int),
		},
	}

	// Try to load from Redis first
	if err := sm.loadFromRedis(); err != nil {
		sm.logger.Info("No config in Redis, loading from file", zap.Error(err))
		// Load from file
		if err := sm.loadFromFile(); err != nil {
			return nil, fmt.Errorf("failed to load spread config: %w", err)
		}
		// Persist to Redis
		if err := sm.saveToRedis(); err != nil {
			sm.logger.Warn("Failed to persist config to Redis", zap.Error(err))
		}
	}

	return sm, nil
}

// Start begins listening for Redis pub/sub updates.
func (sm *SpreadManager) Start(ctx context.Context) {
	sm.wg.Add(1)
	go sm.subscribeToUpdates(ctx)
}

// Stop stops the spread manager.
func (sm *SpreadManager) Stop() {
	close(sm.stopCh)
	sm.wg.Wait()
}

// loadFromFile loads the spread configuration from a JSON file.
func (sm *SpreadManager) loadFromFile() error {
	data, err := os.ReadFile(sm.configPath)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	var config SpreadConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}

	// Normalize symbol names to uppercase
	normalizedSpreads := make(map[string]int)
	for symbol, spread := range config.SymbolSpreads {
		normalizedSpreads[strings.ToUpper(symbol)] = spread
	}
	config.SymbolSpreads = normalizedSpreads

	sm.mu.Lock()
	sm.config = &config
	sm.mu.Unlock()

	sm.logger.Info("Loaded spread config from file",
		zap.Int("default_bps", config.DefaultSpreadBps),
		zap.Int("symbol_overrides", len(config.SymbolSpreads)),
		zap.Int("asset_class_defaults", len(config.AssetClassDefaults)))

	return nil
}

// loadFromRedis loads the spread configuration from Redis.
func (sm *SpreadManager) loadFromRedis() error {
	if sm.redis == nil {
		return fmt.Errorf("redis client not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := sm.redis.Client().Get(ctx, RedisSpreadConfigKey).Bytes()
	if err != nil {
		return fmt.Errorf("get config from redis: %w", err)
	}

	var config SpreadConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse redis config: %w", err)
	}

	sm.mu.Lock()
	sm.config = &config
	sm.mu.Unlock()

	sm.logger.Info("Loaded spread config from Redis",
		zap.Int("default_bps", config.DefaultSpreadBps),
		zap.Int("symbol_overrides", len(config.SymbolSpreads)),
		zap.Int("asset_class_defaults", len(config.AssetClassDefaults)))

	return nil
}

// saveToRedis persists the current spread configuration to Redis.
func (sm *SpreadManager) saveToRedis() error {
	if sm.redis == nil {
		return fmt.Errorf("redis client not available")
	}

	sm.mu.RLock()
	data, err := json.Marshal(sm.config)
	sm.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sm.redis.Client().Set(ctx, RedisSpreadConfigKey, data, 0).Err(); err != nil {
		return fmt.Errorf("set config in redis: %w", err)
	}

	return nil
}

// subscribeToUpdates subscribes to Redis pub/sub for spread config updates.
// It automatically reconnects with exponential backoff if the connection drops.
func (sm *SpreadManager) subscribeToUpdates(ctx context.Context) {
	defer sm.wg.Done()

	if sm.redis == nil {
		sm.logger.Warn("Redis not available, spread updates disabled")
		return
	}

	const (
		initialBackoff    = 500 * time.Millisecond
		maxBackoff        = 30 * time.Second
		backoffMultiplier = 2.0
	)

	backoff := initialBackoff

	for {
		// Check for shutdown before attempting to subscribe
		select {
		case <-ctx.Done():
			return
		case <-sm.stopCh:
			return
		default:
		}

		sm.logger.Info("Subscribing to spread config updates")
		pubsub := sm.redis.Client().Subscribe(ctx, RedisSpreadPubSubKey)
		ch := pubsub.Channel()

		sm.logger.Info("Subscribed to spread config updates")

		// Reset backoff on successful subscription
		backoff = initialBackoff

		disconnected := false
		for !disconnected {
			select {
			case <-ctx.Done():
				pubsub.Close()
				return
			case <-sm.stopCh:
				pubsub.Close()
				return
			case msg, ok := <-ch:
				if !ok {
					// Channel closed — connection lost
					sm.logger.Warn("Redis pub/sub channel closed, will reconnect")
					disconnected = true
					break
				}
				if msg == nil {
					continue
				}
				sm.logger.Info("Received spread config update notification")
				if err := sm.loadFromRedis(); err != nil {
					sm.logger.Error("Failed to reload config from Redis", zap.Error(err))
				}
			}
		}

		pubsub.Close()

		// Wait with backoff before reconnecting
		sm.logger.Info("Reconnecting to spread config pub/sub",
			zap.Duration("backoff", backoff))

		select {
		case <-ctx.Done():
			return
		case <-sm.stopCh:
			return
		case <-time.After(backoff):
		}

		// Increase backoff for next attempt
		backoff = time.Duration(float64(backoff) * backoffMultiplier)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// GetSpreadBps returns the spread in basis points for a given symbol.
// It follows the lookup order: symbol-specific -> asset class default -> global default.
func (sm *SpreadManager) GetSpreadBps(symbol string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	symbol = strings.ToUpper(symbol)

	// 1. Check symbol-specific spread
	if spread, ok := sm.config.SymbolSpreads[symbol]; ok {
		return spread
	}

	// 2. Check asset class default
	assetClass := classifySymbol(symbol)
	if spread, ok := sm.config.AssetClassDefaults[string(assetClass)]; ok {
		return spread
	}

	// 3. Return global default
	return sm.config.DefaultSpreadBps
}

// DeriveSyntheticBidAsk derives bid and ask prices from the last price using the symbol's spread.
func (sm *SpreadManager) DeriveSyntheticBidAsk(symbol string, last float64) (bid, ask float64) {
	spreadBps := sm.GetSpreadBps(symbol)
	halfSpreadPct := float64(spreadBps) / 2 / 10000.0
	bid = last * (1 - halfSpreadPct)
	ask = last * (1 + halfSpreadPct)
	return bid, ask
}

// SetSymbolSpread updates the spread for a specific symbol and persists to Redis.
func (sm *SpreadManager) SetSymbolSpread(ctx context.Context, symbol string, spreadBps int) error {
	symbol = strings.ToUpper(symbol)

	sm.mu.Lock()
	if sm.config.SymbolSpreads == nil {
		sm.config.SymbolSpreads = make(map[string]int)
	}
	sm.config.SymbolSpreads[symbol] = spreadBps
	sm.mu.Unlock()

	// Persist to Redis
	if err := sm.saveToRedis(); err != nil {
		return fmt.Errorf("save to redis: %w", err)
	}

	// Notify other instances
	if sm.redis != nil {
		if err := sm.redis.Client().Publish(ctx, RedisSpreadPubSubKey, "update").Err(); err != nil {
			sm.logger.Warn("Failed to publish spread update notification", zap.Error(err))
		}
	}

	sm.logger.Info("Updated symbol spread",
		zap.String("symbol", symbol),
		zap.Int("spread_bps", spreadBps))

	return nil
}

// DeleteSymbolSpread removes the spread override for a specific symbol.
func (sm *SpreadManager) DeleteSymbolSpread(ctx context.Context, symbol string) error {
	symbol = strings.ToUpper(symbol)

	sm.mu.Lock()
	delete(sm.config.SymbolSpreads, symbol)
	sm.mu.Unlock()

	// Persist to Redis
	if err := sm.saveToRedis(); err != nil {
		return fmt.Errorf("save to redis: %w", err)
	}

	// Notify other instances
	if sm.redis != nil {
		if err := sm.redis.Client().Publish(ctx, RedisSpreadPubSubKey, "update").Err(); err != nil {
			sm.logger.Warn("Failed to publish spread update notification", zap.Error(err))
		}
	}

	sm.logger.Info("Deleted symbol spread override", zap.String("symbol", symbol))

	return nil
}

// GetConfig returns a copy of the current spread configuration.
func (sm *SpreadManager) GetConfig() SpreadConfig {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Deep copy
	config := SpreadConfig{
		DefaultSpreadBps:   sm.config.DefaultSpreadBps,
		SymbolSpreads:      make(map[string]int),
		AssetClassDefaults: make(map[string]int),
	}
	for k, v := range sm.config.SymbolSpreads {
		config.SymbolSpreads[k] = v
	}
	for k, v := range sm.config.AssetClassDefaults {
		config.AssetClassDefaults[k] = v
	}
	return config
}

// SetDefaultSpread updates the default spread and persists to Redis.
func (sm *SpreadManager) SetDefaultSpread(ctx context.Context, spreadBps int) error {
	sm.mu.Lock()
	sm.config.DefaultSpreadBps = spreadBps
	sm.mu.Unlock()

	if err := sm.saveToRedis(); err != nil {
		return fmt.Errorf("save to redis: %w", err)
	}

	if sm.redis != nil {
		if err := sm.redis.Client().Publish(ctx, RedisSpreadPubSubKey, "update").Err(); err != nil {
			sm.logger.Warn("Failed to publish spread update notification", zap.Error(err))
		}
	}

	sm.logger.Info("Updated default spread", zap.Int("spread_bps", spreadBps))
	return nil
}

// SetAssetClassDefault updates the default spread for an asset class and persists to Redis.
func (sm *SpreadManager) SetAssetClassDefault(ctx context.Context, assetClass string, spreadBps int) error {
	sm.mu.Lock()
	if sm.config.AssetClassDefaults == nil {
		sm.config.AssetClassDefaults = make(map[string]int)
	}
	sm.config.AssetClassDefaults[assetClass] = spreadBps
	sm.mu.Unlock()

	if err := sm.saveToRedis(); err != nil {
		return fmt.Errorf("save to redis: %w", err)
	}

	if sm.redis != nil {
		if err := sm.redis.Client().Publish(ctx, RedisSpreadPubSubKey, "update").Err(); err != nil {
			sm.logger.Warn("Failed to publish spread update notification", zap.Error(err))
		}
	}

	sm.logger.Info("Updated asset class default spread",
		zap.String("asset_class", assetClass),
		zap.Int("spread_bps", spreadBps))
	return nil
}

// classifySymbol determines the asset class of a symbol based on its name.
func classifySymbol(symbol string) AssetClass {
	symbol = strings.ToUpper(symbol)
	symbol = strings.ReplaceAll(symbol, "/", "")

	// Forex major pairs (include USD and one of: EUR, GBP, JPY, CHF, CAD, AUD, NZD)
	forexMajors := []string{
		"EURUSD", "USDJPY", "GBPUSD", "USDCHF", "USDCAD", "AUDUSD", "NZDUSD",
	}
	for _, major := range forexMajors {
		if symbol == major {
			return AssetClassForexMajor
		}
	}

	// Forex minor pairs (crosses without USD)
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

	// Forex exotic pairs (with emerging market currencies)
	exoticCurrencies := []string{"TRY", "ZAR", "MXN", "PLN", "HUF", "CZK", "SEK", "NOK", "DKK", "SGD", "HKD", "THB", "INR", "BRL", "RUB", "KRW", "CNY", "CNH"}
	for _, exotic := range exoticCurrencies {
		if strings.Contains(symbol, exotic) {
			return AssetClassForexExotic
		}
	}

	// Crypto major (BTC, ETH)
	if strings.HasPrefix(symbol, "BTC") || strings.HasSuffix(symbol, "BTC") ||
		strings.HasPrefix(symbol, "ETH") || strings.HasSuffix(symbol, "ETH") {
		if strings.Contains(symbol, "BTC") {
			return AssetClassCryptoMajor
		}
		if strings.Contains(symbol, "ETH") {
			return AssetClassCryptoMajor
		}
	}

	// Crypto alt (other cryptocurrencies)
	cryptoSuffixes := []string{"USD", "USDT", "USDC", "EUR", "GBP"}
	cryptoPrefixes := []string{"XRP", "LTC", "ADA", "DOT", "LINK", "SOL", "AVAX", "DOGE", "SHIB", "MATIC", "UNI", "ATOM"}
	for _, prefix := range cryptoPrefixes {
		if strings.HasPrefix(symbol, prefix) {
			for _, suffix := range cryptoSuffixes {
				if strings.HasSuffix(symbol, suffix) {
					return AssetClassCryptoAlt
				}
			}
		}
	}

	// Commodities
	commodities := []string{"XAUUSD", "XAGUSD", "XPTUSD", "XPDUSD", "WTIUSD", "BRNUSD", "NGAS", "COPPER", "GOLD", "SILVER"}
	for _, comm := range commodities {
		if symbol == comm || strings.HasPrefix(symbol, comm) {
			return AssetClassCommodities
		}
	}

	// Check for common forex pattern (6-letter pairs)
	if len(symbol) == 6 && isForexPair(symbol) {
		return AssetClassForexMinor
	}

	// Default to equities for stock-like symbols
	if len(symbol) <= 5 && isAllLetters(symbol) {
		return AssetClassEquities
	}

	return AssetClassUnknown
}

// isForexPair checks if a symbol looks like a forex pair (two 3-letter currency codes).
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
