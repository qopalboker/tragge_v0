package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// BinanceConfig holds configuration for the Binance WebSocket feed.
type BinanceConfig struct {
	BaseURL      string        // default: "wss://stream.binance.com:9443"
	Symbols      []string      // Binance symbol names: ["btcusdt", "ethusdt"]
	USDTUSDRate  float64       // USDT→USD conversion rate
	Enabled      bool
	ReconnectMax time.Duration // max backoff: 60s
}

// BinanceCombinedMsg is the wrapper for Binance combined stream messages.
type BinanceCombinedMsg struct {
	Stream string              `json:"stream"` // "btcusdt@bookTicker"
	Data   BinanceBookTicker   `json:"data"`
}

// BinanceBookTicker represents a Binance bookTicker event.
type BinanceBookTicker struct {
	UpdateID int64  `json:"u"`
	Symbol   string `json:"s"` // "BTCUSDT"
	BidPrice string `json:"b"` // best bid price
	BidQty   string `json:"B"` // best bid qty
	AskPrice string `json:"a"` // best ask price
	AskQty   string `json:"A"` // best ask qty
}

// BinanceCryptoFeed connects to Binance combined WebSocket stream
// and emits ticks via the shared TickHandler.
type BinanceCryptoFeed struct {
	config      BinanceConfig
	tickHandler TickHandler
	registry    *SymbolRegistry
	logger      *zap.Logger
	conn        *websocket.Conn
	connMu      sync.Mutex

	// Symbol mappings: "BTCUSDT" → "BTC/USD" (protected by symMu)
	binanceToCanonical map[string]string
	canonicalToBinance map[string]string
	symMu              sync.RWMutex
	reconnectCh        chan struct{} // signals connectionLoop to reconnect with new symbols

	// Atomic state for thread safety
	connected  atomic.Bool
	lastTick   atomic.Int64
	tickCount  atomic.Int64
	errorCount atomic.Int64

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBinanceCryptoFeed creates a new BinanceCryptoFeed with the given configuration.
func NewBinanceCryptoFeed(config BinanceConfig, tickHandler TickHandler, registry *SymbolRegistry, logger *zap.Logger) *BinanceCryptoFeed {
	if config.BaseURL == "" {
		config.BaseURL = "wss://stream.binance.com:9443"
	}
	if config.USDTUSDRate <= 0 {
		config.USDTUSDRate = 1.0
	}
	if config.ReconnectMax <= 0 {
		config.ReconnectMax = 60 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	feed := &BinanceCryptoFeed{
		config:             config,
		tickHandler:        tickHandler,
		registry:           registry,
		logger:             logger,
		binanceToCanonical: make(map[string]string),
		canonicalToBinance: make(map[string]string),
		reconnectCh:        make(chan struct{}, 1),
		ctx:                ctx,
		cancel:             cancel,
	}

	feed.buildSymbolMappings()
	return feed
}

// buildSymbolMappings populates the bidirectional symbol maps.
func (f *BinanceCryptoFeed) buildSymbolMappings() {
	// If registry has ToBinance mappings, use those
	if f.registry != nil && len(f.registry.ToBinance) > 0 {
		for canonical, bn := range f.registry.ToBinance {
			upper := strings.ToUpper(bn)
			f.binanceToCanonical[upper] = canonical
			f.canonicalToBinance[canonical] = upper
		}
		f.logger.Info("built Binance symbol mappings from registry",
			zap.Int("count", len(f.binanceToCanonical)))
		return
	}

	// Fallback: use config.Symbols with heuristic conversion
	for _, sym := range f.config.Symbols {
		upper := strings.ToUpper(strings.TrimSpace(sym))
		if upper == "" {
			continue
		}
		// Heuristic: BTCUSDT → BTC/USD
		if strings.HasSuffix(upper, "USDT") {
			base := strings.TrimSuffix(upper, "USDT")
			canonical := base + "/USD"
			f.binanceToCanonical[upper] = canonical
			f.canonicalToBinance[canonical] = upper
		}
	}

	f.logger.Info("built Binance symbol mappings from config",
		zap.Int("count", len(f.binanceToCanonical)))
}

// Start begins the WebSocket connection loop in a background goroutine.
func (f *BinanceCryptoFeed) Start() {
	if len(f.binanceToCanonical) == 0 {
		f.logger.Warn("BinanceCryptoFeed: no symbols configured, not starting")
		return
	}

	f.wg.Add(1)
	go f.connectionLoop()

	f.logger.Info("BinanceCryptoFeed started",
		zap.Int("symbols", len(f.binanceToCanonical)),
		zap.Float64("usdt_usd_rate", f.config.USDTUSDRate),
	)
}

// Stop gracefully shuts down the feed.
func (f *BinanceCryptoFeed) Stop() {
	f.cancel()
	f.closeConn()
	f.wg.Wait()
	f.logger.Info("BinanceCryptoFeed stopped",
		zap.Int64("total_ticks", f.tickCount.Load()),
		zap.Int64("total_errors", f.errorCount.Load()),
	)
}

// AddSymbol dynamically adds a crypto symbol and triggers reconnection.
func (f *BinanceCryptoFeed) AddSymbol(canonical string) {
	binance := canonicalToBinanceSymbol(canonical)
	if binance == "" {
		return
	}
	f.symMu.Lock()
	f.binanceToCanonical[binance] = canonical
	f.canonicalToBinance[canonical] = binance
	f.symMu.Unlock()
	f.logger.Info("Binance: added symbol", zap.String("canonical", canonical))
	f.triggerReconnect()
}

// RemoveSymbol dynamically removes a crypto symbol and triggers reconnection.
func (f *BinanceCryptoFeed) RemoveSymbol(canonical string) {
	f.symMu.Lock()
	if binance, ok := f.canonicalToBinance[canonical]; ok {
		delete(f.binanceToCanonical, binance)
		delete(f.canonicalToBinance, canonical)
	}
	f.symMu.Unlock()
	f.logger.Info("Binance: removed symbol", zap.String("canonical", canonical))
	f.triggerReconnect()
}

// canonicalToBinanceSymbol converts "BTC/USD" → "BTCUSDT".
func canonicalToBinanceSymbol(canonical string) string {
	parts := strings.SplitN(canonical, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToUpper(parts[0]) + "USDT"
}

func (f *BinanceCryptoFeed) triggerReconnect() {
	// Non-blocking send; if a reconnect is already pending, skip
	select {
	case f.reconnectCh <- struct{}{}:
	default:
	}
}

// connectionLoop manages the WebSocket lifecycle with reconnection backoff.
func (f *BinanceCryptoFeed) connectionLoop() {
	defer f.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			f.logger.Error("BinanceCryptoFeed.connectionLoop panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	bo := &binanceBackoff{
		initial: 1 * time.Second,
		max:     f.config.ReconnectMax,
		current: 1 * time.Second,
	}

	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		if err := f.connect(); err != nil {
			f.errorCount.Add(1)
			f.connected.Store(false)
			f.logger.Warn("Binance WS connect failed",
				zap.Error(err),
				zap.Duration("retry_in", bo.current))

			select {
			case <-f.ctx.Done():
				return
			case <-time.After(bo.next()):
			}
			continue
		}

		bo.reset()
		f.connected.Store(true)
		f.logger.Info("Binance WS connected",
			zap.Int("symbols", len(f.binanceToCanonical)))

		// Read messages until error, context cancellation, or reconnect signal
		f.readLoop()

		f.connected.Store(false)
		f.closeConn()

		// Brief pause before reconnecting (or immediate if reconnect requested)
		select {
		case <-f.ctx.Done():
			return
		case <-f.reconnectCh:
			f.logger.Info("Binance reconnecting due to symbol change")
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// connect establishes the combined stream WebSocket connection.
func (f *BinanceCryptoFeed) connect() error {
	// Build combined stream URL:
	// wss://stream.binance.com:9443/stream?streams=btcusdt@bookTicker/ethusdt@bookTicker/...
	f.symMu.RLock()
	streams := make([]string, 0, len(f.binanceToCanonical))
	for upper := range f.binanceToCanonical {
		streams = append(streams, strings.ToLower(upper)+"@bookTicker")
	}
	f.symMu.RUnlock()

	if len(streams) == 0 {
		return fmt.Errorf("no symbols to subscribe")
	}

	url := f.config.BaseURL + "/stream?streams=" + strings.Join(streams, "/")

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(f.ctx, url, nil)
	if err != nil {
		return fmt.Errorf("binance ws dial: %w", err)
	}

	// Set read deadline for ping/pong handling
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		return nil
	})
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		// Respond to server pings with pong
		f.connMu.Lock()
		defer f.connMu.Unlock()
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	f.connMu.Lock()
	f.conn = conn
	f.connMu.Unlock()

	return nil
}

// readLoop reads and processes messages from the WebSocket connection.
func (f *BinanceCryptoFeed) readLoop() {
	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		_, message, err := f.conn.ReadMessage()
		if err != nil {
			if f.ctx.Err() != nil {
				return // context cancelled, normal shutdown
			}
			f.errorCount.Add(1)
			f.logger.Warn("Binance WS read error", zap.Error(err))
			return // will trigger reconnect
		}

		// Extend read deadline on successful message
		f.conn.SetReadDeadline(time.Now().Add(30 * time.Second))

		f.handleMessage(message)
	}
}

// handleMessage parses and processes a single WebSocket message.
func (f *BinanceCryptoFeed) handleMessage(data []byte) {
	var msg BinanceCombinedMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		f.logger.Debug("Binance WS unmarshal error", zap.Error(err))
		return
	}

	// Map Binance symbol to canonical
	f.symMu.RLock()
	canonical, ok := f.binanceToCanonical[strings.ToUpper(msg.Data.Symbol)]
	f.symMu.RUnlock()
	if !ok {
		return
	}

	bid, err := strconv.ParseFloat(msg.Data.BidPrice, 64)
	if err != nil || bid <= 0 {
		return
	}
	ask, err := strconv.ParseFloat(msg.Data.AskPrice, 64)
	if err != nil || ask <= 0 {
		return
	}

	// Parse bid/ask quantities as volume proxy (top-of-book liquidity)
	bidQty, _ := strconv.ParseFloat(msg.Data.BidQty, 64)
	askQty, _ := strconv.ParseFloat(msg.Data.AskQty, 64)
	volume := bidQty + askQty

	// Calculate last price as midpoint of bid/ask
	last := (bid + ask) / 2

	// Apply USDT→USD conversion
	rate := f.config.USDTUSDRate
	bid *= rate
	ask *= rate
	last *= rate

	now := time.Now().Unix()
	f.tickHandler(canonical, last, bid, ask, volume, now, "binance")
	f.lastTick.Store(now)
	f.tickCount.Add(1)
}

// closeConn safely closes the WebSocket connection.
func (f *BinanceCryptoFeed) closeConn() {
	f.connMu.Lock()
	defer f.connMu.Unlock()
	if f.conn != nil {
		f.conn.Close()
		f.conn = nil
	}
}

// IsConnected returns whether the feed has an active WebSocket connection.
func (f *BinanceCryptoFeed) IsConnected() bool {
	return f.connected.Load()
}

// LastTickTime returns the Unix timestamp of the last successful tick.
func (f *BinanceCryptoFeed) LastTickTime() int64 {
	return f.lastTick.Load()
}

// Stats returns feed statistics for the health endpoint.
func (f *BinanceCryptoFeed) Stats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":     f.config.Enabled,
		"connected":   f.connected.Load(),
		"tick_count":  f.tickCount.Load(),
		"error_count": f.errorCount.Load(),
		"symbols":     len(f.binanceToCanonical),
		"last_tick":   f.lastTick.Load(),
	}
}

// binanceBackoff implements exponential backoff with jitter for reconnection.
type binanceBackoff struct {
	initial time.Duration
	max     time.Duration
	current time.Duration
}

func (b *binanceBackoff) next() time.Duration {
	d := b.current
	// Add jitter: 0% to +50% of current delay
	jitter := time.Duration(float64(d) * (0.5 + rand.Float64()) * 0.5)
	b.current = b.current * 2
	if b.current > b.max {
		b.current = b.max
	}
	return d + jitter - d/4
}

func (b *binanceBackoff) reset() {
	b.current = b.initial
}
