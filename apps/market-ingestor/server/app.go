package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/config"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/secrets"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// ProviderType represents the market data provider type.
type ProviderType string

const (
	ProviderTwelveData ProviderType = "twelvedata"
	ProviderMassive    ProviderType = "massive"
	ProviderFinnhub    ProviderType = "finnhub"
	ProviderAuto       ProviderType = "auto"
)

// wsSentinelDropsTotal counts how many times a disconnect nil sentinel was
// dropped because the provider's msgCh buffer was full.
var wsSentinelDropsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: "market_ingestor",
	Name:      "ws_sentinel_drops_total",
	Help:      "Times a disconnect nil sentinel was dropped because msgCh was full",
}, []string{"provider"})

// tickJob represents a tick that needs to be published to Redis and Kafka.
type tickJob struct {
	symbol string
	tick   *TickData
}

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port              string
	TwelveDataAPIKeys []string // Multiple API keys for rotation
	MassiveAPIKeys    []string // Multiple API keys for rotation
	FinnhubAPIKeys    []string // Multiple API keys for rotation
	RedisAddr         string
	KafkaBrokers      []string
	Symbols           []string
	PostgresDSN       string // PostgreSQL connection string for candle storage
	TicksTopic        string // Kafka topic for publishing tick data (default: "ticks.v1")

	// Provider configuration
	MarketProvider    ProviderType
	FailoverTimeout   time.Duration // Time before switching to fallback provider
	AutoSwitchback    bool          // Whether to switch back to primary when it recovers
	SwitchbackDelay   time.Duration // Delay before attempting to switch back to primary
	HealthCheckPeriod time.Duration // How often to check if primary is healthy

	// Candle aggregation configuration
	CandleFlushInterval time.Duration // How often to flush candles to DB
	CandleBatchSize     int           // Max candles per batch insert
	EnableCandles       bool          // Enable candle aggregation

	// Nobitex crypto feed configuration
	NobitexEnabled      bool          // Enable Nobitex as primary crypto provider
	NobitexToken        string        // Nobitex API auth token
	NobitexBaseURL      string        // Nobitex API base URL
	NobitexPollInterval time.Duration // REST polling interval (default: 2s)
	NobitexUSDTRate     float64       // USDT→USD conversion rate (default: 1.0)

	// Binance crypto feed configuration
	BinanceEnabled  bool    // Enable Binance WebSocket crypto feed
	BinanceBaseURL  string  // Binance WS base URL (default: "wss://stream.binance.com:9443")
	BinanceUSDTRate float64 // USDT→USD conversion rate (default: 1.0)

	// Crypto provider selection: "nobitex", "binance", "both"
	CryptoProvider string

	// Notification configuration
	NotificationEnabled      bool   // Enable notifications
	NotificationAsync        bool   // Enable async notification sending
	NotificationAsyncWorkers int    // Number of async workers
	NotificationQueueSize    int    // Async queue size
	DiscordWebhookURL        string // Discord webhook URL
	ResendAPIKey             string // Resend API key for email
	ResendFromEmail          string // From email address for Resend
	NotificationRecipients   string // Comma-separated email recipients
	Environment              string // Environment name (development/staging/production)

	// Tick publish worker pool configuration
	TickPublishWorkers   int // Number of concurrent publish workers (default: 16)
	TickPublishQueueSize int // Buffered channel capacity (default: 1024)

	// Control API authentication
	ControlAPIKey string // API key required for /control/* endpoints
}

// KeyRotator manages rotating through multiple API keys.
type KeyRotator struct {
	keys         []string
	currentIndex int
	mu           sync.RWMutex
	rotateCount  int64 // Total number of rotations
	logger       *zap.Logger
}

// NewKeyRotator creates a new key rotator with the given keys.
func NewKeyRotator(keys []string, logger *zap.Logger) *KeyRotator {
	if len(keys) == 0 {
		return &KeyRotator{keys: []string{""}, logger: logger}
	}
	return &KeyRotator{keys: keys, logger: logger}
}

// Current returns the current API key.
func (kr *KeyRotator) Current() string {
	kr.mu.RLock()
	defer kr.mu.RUnlock()
	if len(kr.keys) == 0 {
		return ""
	}
	return kr.keys[kr.currentIndex]
}

// Rotate moves to the next API key and returns it.
func (kr *KeyRotator) Rotate() string {
	kr.mu.Lock()
	defer kr.mu.Unlock()
	if len(kr.keys) <= 1 {
		return kr.keys[0]
	}
	kr.currentIndex = (kr.currentIndex + 1) % len(kr.keys)
	kr.rotateCount++
	kr.logger.Info("Rotated to next API key",
		zap.Int("key_index", kr.currentIndex),
		zap.Int64("total_rotations", kr.rotateCount))
	return kr.keys[kr.currentIndex]
}

// Count returns the number of available keys.
func (kr *KeyRotator) Count() int {
	kr.mu.RLock()
	defer kr.mu.RUnlock()
	return len(kr.keys)
}

// RotateCount returns the total number of rotations performed.
func (kr *KeyRotator) RotateCount() int64 {
	kr.mu.RLock()
	defer kr.mu.RUnlock()
	return kr.rotateCount
}

// CurrentIndex returns the current key index.
func (kr *KeyRotator) CurrentIndex() int {
	kr.mu.RLock()
	defer kr.mu.RUnlock()
	return kr.currentIndex
}

// DefaultSpreadBps is the default spread in basis points for synthetic bid/ask derivation.
// This matches the spread used by trading-engine for consistency.
const DefaultSpreadBps = 10 // 0.1% spread (10 basis points)

// TickData represents the internal tick snapshot for a symbol.
type TickData struct {
	Last   float64 `json:"last"`
	Bid    float64 `json:"bid,omitempty"`
	Ask    float64 `json:"ask,omitempty"`
	Volume float64 `json:"volume,omitempty"`
	Ts     int64   `json:"ts"`
}

// MarketProvider is an interface for market data providers.
type MarketProvider interface {
	// Name returns the provider name.
	Name() ProviderType
	// Connect establishes a WebSocket connection.
	Connect(ctx context.Context) error
	// Subscribe subscribes to the given symbols.
	Subscribe(symbols []string) error
	// ReadMessage reads the next message from the WebSocket.
	ReadMessage() ([]byte, error)
	// Close closes the WebSocket connection.
	Close() error
	// IsConnected returns whether the provider is connected.
	IsConnected() bool
	// RotateKey rotates to the next API key (for rate limit handling).
	RotateKey()
	// KeyCount returns the number of available API keys.
	KeyCount() int
}

// TickHandler is called when a tick is received from a provider.
// bid and ask may be zero if the provider only supplies a last price (e.g. TwelveData).
// volume is the trade volume from the provider; zero means unknown.
// source identifies the originating provider (e.g. "massive", "twelvedata", "nobitex").
type TickHandler func(symbol string, price, bid, ask, volume float64, timestamp int64, source string)

// TwelveDataEvent represents incoming WebSocket messages from Twelve Data.
type TwelveDataEvent struct {
	Event     string  `json:"event"`
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	Timestamp int64   `json:"timestamp"`
	Status    string  `json:"status"`
	Message   string  `json:"message"`
}

// MassiveForexQuote represents a forex quote from Massive (ev: "C").
type MassiveForexQuote struct {
	Ev        string  `json:"ev"` // "C"
	Pair      string  `json:"p"`  // "EUR-USD"
	Ask       float64 `json:"a"`  // ask price
	Bid       float64 `json:"b"`  // bid price
	Timestamp int64   `json:"t"`  // Unix ms
	Exchange  int     `json:"x"`  // exchange ID
}

// MassiveCryptoTrade represents a crypto trade from Massive (ev: "XT").
type MassiveCryptoTrade struct {
	Ev        string  `json:"ev"`   // "XT"
	Pair      string  `json:"pair"` // "BTC-USD"
	Price     float64 `json:"p"`    // last price
	Size      float64 `json:"s"`    // trade size
	Timestamp int64   `json:"t"`    // Unix ms
	Exchange  int     `json:"x"`    // exchange ID
	Condition []int   `json:"c"`    // conditions
}

// MassiveCryptoQuote represents a crypto quote from Massive (ev: "XQ").
type MassiveCryptoQuote struct {
	Ev        string  `json:"ev"`   // "XQ"
	Pair      string  `json:"pair"` // "BTC-USD"
	BidPrice  float64 `json:"bp"`   // bid
	BidSize   float64 `json:"bs"`   // bid size
	AskPrice  float64 `json:"ap"`   // ask
	AskSize   float64 `json:"as"`   // ask size
	Timestamp int64   `json:"t"`    // Unix ms
}

// MassiveStatus represents a status message from Massive (ev: "status").
type MassiveStatus struct {
	Ev      string `json:"ev"`     // "status"
	Status  string `json:"status"` // "auth_success", "success"
	Message string `json:"message"`
}

// TwelveDataProvider implements MarketProvider for Twelve Data.
type TwelveDataProvider struct {
	keyRotator *KeyRotator
	conn       *websocket.Conn
	mu         sync.Mutex
	connected  bool
	logger     *zap.Logger
	msgCh      chan []byte
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewTwelveDataProvider creates a new Twelve Data provider.
func NewTwelveDataProvider(keys []string, logger *zap.Logger) *TwelveDataProvider {
	return &TwelveDataProvider{
		keyRotator: NewKeyRotator(keys, logger),
		logger:     logger,
		msgCh:      make(chan []byte, 256),
		stopCh:     make(chan struct{}),
	}
}

func (p *TwelveDataProvider) Name() ProviderType {
	return ProviderTwelveData
}

func (p *TwelveDataProvider) Connect(ctx context.Context) error {
	apiKey := p.keyRotator.Current()
	if apiKey == "" {
		return fmt.Errorf("TWELVEDATA_API_KEYS not configured")
	}

	wsURL := fmt.Sprintf("wss://ws.twelvedata.com/v1/quotes/price?apikey=%s", apiKey)
	p.logger.Info("Connecting",
		zap.Int("key_index", p.keyRotator.CurrentIndex()+1),
		zap.Int("key_count", p.keyRotator.Count()))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("TwelveData dial failed: %w", err)
	}

	p.mu.Lock()
	p.conn = conn
	p.connected = true
	p.mu.Unlock()

	p.logger.Info("WebSocket connected")

	p.wg.Add(1)
	go p.readLoop()

	return nil
}

func (p *TwelveDataProvider) Subscribe(symbols []string) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	subscribeMsg := map[string]interface{}{
		"action": "subscribe",
		"params": map[string]interface{}{
			"symbols": strings.Join(symbols, ","),
		},
	}

	if err := conn.WriteJSON(subscribeMsg); err != nil {
		return fmt.Errorf("TwelveData subscribe failed: %w", err)
	}

	p.logger.Info("Subscribed", zap.Strings("symbols", symbols))
	return nil
}

func (p *TwelveDataProvider) readLoop() {
	defer p.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("TwelveData readLoop panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		p.mu.Lock()
		conn := p.conn
		p.mu.Unlock()

		if conn == nil {
			return
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil {
			p.mu.Lock()
			p.connected = false
			p.mu.Unlock()
			p.logger.Error("Read error", zap.Error(err))
			select {
			case p.msgCh <- nil:
			default:
				p.logger.Warn("Failed to send disconnect sentinel: msgCh full, consumer may miss reconnect")
				wsSentinelDropsTotal.WithLabelValues("twelvedata").Inc()
			}
			return
		}

		select {
		case p.msgCh <- message:
		case <-p.stopCh:
			return
		}
	}
}

func (p *TwelveDataProvider) ReadMessage() ([]byte, error) {
	msg, ok := <-p.msgCh
	if !ok {
		return nil, fmt.Errorf("message channel closed")
	}
	if msg == nil {
		return nil, fmt.Errorf("connection lost")
	}
	return msg, nil
}

func (p *TwelveDataProvider) Close() error {
	select {
	case <-p.stopCh:
		// Already closed
	default:
		close(p.stopCh)
	}

	p.mu.Lock()
	var err error
	if p.conn != nil {
		err = p.conn.Close()
		p.conn = nil
		p.connected = false
	}
	p.mu.Unlock()

	p.wg.Wait()

	// Re-initialize channels for potential reconnect
	p.msgCh = make(chan []byte, 256)
	p.stopCh = make(chan struct{})
	return err
}

func (p *TwelveDataProvider) IsConnected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.connected
}

// RotateKey rotates to the next API key.
func (p *TwelveDataProvider) RotateKey() {
	p.keyRotator.Rotate()
}

// KeyCount returns the number of available API keys.
func (p *TwelveDataProvider) KeyCount() int {
	return p.keyRotator.Count()
}

// ParseTwelveDataMessage parses a TwelveData WebSocket message and returns tick data.
func ParseTwelveDataMessage(data []byte) (symbol string, price float64, ts int64, eventType string, err error) {
	var event TwelveDataEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return "", 0, 0, "", fmt.Errorf("parse error: %w", err)
	}

	switch event.Event {
	case "price":
		ts := event.Timestamp
		if ts == 0 {
			ts = time.Now().Unix()
		}
		return event.Symbol, event.Price, ts, "price", nil
	case "heartbeat":
		return "", 0, 0, "heartbeat", nil
	case "subscribe-status":
		return "", 0, 0, "subscribe-status", nil
	default:
		return "", 0, 0, event.Event, nil
	}
}

// massiveForexCurrencies is the set of currency codes that route through the forex WebSocket.
// Includes standard forex pairs and commodity metals.
var massiveForexCurrencies = map[string]bool{
	// Major currencies
	"USD": true, "EUR": true, "GBP": true, "JPY": true, "CHF": true,
	"AUD": true, "CAD": true, "NZD": true,
	// Minor / exotic
	"SEK": true, "NOK": true, "DKK": true, "SGD": true, "HKD": true,
	"MXN": true, "ZAR": true, "TRY": true, "PLN": true, "CZK": true,
	"HUF": true, "ILS": true, "THB": true, "INR": true, "CNH": true,
	"CNY": true, "KRW": true, "TWD": true, "BRL": true, "RUB": true,
	// Commodity metals (routed via forex WS)
	"XAU": true, "XAG": true, "XPT": true, "XPD": true,
}

// isForexSymbol returns true if the canonical symbol (e.g. "EUR/USD") should
// be routed through the Massive forex WebSocket.
func isForexSymbol(symbol string) bool {
	parts := strings.SplitN(symbol, "/", 2)
	if len(parts) != 2 {
		return false
	}
	base := strings.ToUpper(parts[0])
	quote := strings.ToUpper(parts[1])
	return massiveForexCurrencies[base] && massiveForexCurrencies[quote]
}

// canonicalToMassive converts a canonical symbol to the Massive subscription format.
// Forex:  "EUR/USD" → "C.EUR-USD"
// Crypto: "BTC/USD" → "XQ.BTC-USD"
func canonicalToMassive(symbol string, isFx bool) string {
	pair := strings.ReplaceAll(symbol, "/", "-")
	if isFx {
		return "C." + pair
	}
	return "XQ." + pair
}

// massivePairToCanonical converts a Massive pair back to canonical format.
// "EUR-USD" → "EUR/USD", "BTC-USD" → "BTC/USD"
func massivePairToCanonical(pair string) string {
	return strings.ReplaceAll(pair, "-", "/")
}

// MassiveProvider implements MarketProvider for the Massive.com WebSocket API.
// It maintains separate connections for forex and crypto asset classes.
type MassiveProvider struct {
	keyRotator    *KeyRotator
	forexConn     *websocket.Conn // wss://socket.polygon.io/forex
	cryptoConn    *websocket.Conn // wss://socket.polygon.io/crypto
	mu            sync.Mutex
	connected     bool
	forexSymbols  []string // canonical symbols routed to forex WS
	cryptoSymbols []string // canonical symbols routed to crypto WS
	msgCh         chan []byte
	stopCh        chan struct{}
	wg            sync.WaitGroup
	logger        *zap.Logger
}

// NewMassiveProvider creates a new Massive provider.
func NewMassiveProvider(keys []string, logger *zap.Logger) *MassiveProvider {
	return &MassiveProvider{
		keyRotator: NewKeyRotator(keys, logger),
		msgCh:      make(chan []byte, 256),
		stopCh:     make(chan struct{}),
		logger:     logger,
	}
}

func (p *MassiveProvider) Name() ProviderType {
	return ProviderMassive
}

// authenticateConn sends an auth message and waits for auth_success.
func (p *MassiveProvider) authenticateConn(conn *websocket.Conn, label string) error {
	authMsg := map[string]string{
		"action": "auth",
		"params": p.keyRotator.Current(),
	}
	if err := conn.WriteJSON(authMsg); err != nil {
		return fmt.Errorf("%s auth send failed: %w", label, err)
	}

	// Wait for auth_success (with timeout)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("%s auth read failed: %w", label, err)
		}
		// Messages may arrive as JSON arrays
		events, err := unwrapMassiveMessages(msg)
		if err != nil {
			continue
		}
		for _, raw := range events {
			var status MassiveStatus
			if json.Unmarshal(raw, &status) == nil && status.Ev == "status" {
				if status.Status == "auth_success" {
					p.logger.Info("Authenticated successfully", zap.String("ws", label))
					return nil
				}
				if status.Status == "auth_failed" {
					return fmt.Errorf("%s authentication failed: %s", label, status.Message)
				}
			}
		}
	}
}

func (p *MassiveProvider) Connect(ctx context.Context) error {
	apiKey := p.keyRotator.Current()
	if apiKey == "" {
		return fmt.Errorf("MASSIVE_API_KEYS not configured")
	}

	p.logger.Info("Connecting",
		zap.Int("key_index", p.keyRotator.CurrentIndex()+1),
		zap.Int("key_count", p.keyRotator.Count()))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Connect to forex WebSocket
	forexURL := "wss://socket.polygon.io/forex"
	forexConn, _, err := dialer.DialContext(ctx, forexURL, nil)
	if err != nil {
		return fmt.Errorf("Massive forex dial failed: %w", err)
	}

	if err := p.authenticateConn(forexConn, "forex"); err != nil {
		forexConn.Close()
		return err
	}

	// Connect to crypto WebSocket
	cryptoURL := "wss://socket.polygon.io/crypto"
	cryptoConn, _, err := dialer.DialContext(ctx, cryptoURL, nil)
	if err != nil {
		forexConn.Close()
		return fmt.Errorf("Massive crypto dial failed: %w", err)
	}

	if err := p.authenticateConn(cryptoConn, "crypto"); err != nil {
		forexConn.Close()
		cryptoConn.Close()
		return err
	}

	p.mu.Lock()
	p.forexConn = forexConn
	p.cryptoConn = cryptoConn
	p.connected = true
	// Reset channels for new connection
	p.msgCh = make(chan []byte, 256)
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	// Start reader goroutines to multiplex both connections
	p.wg.Add(2)
	go p.readLoop(forexConn, "forex")
	go p.readLoop(cryptoConn, "crypto")

	p.logger.Info("Both forex and crypto WebSockets connected")
	return nil
}

// readLoop reads messages from a single connection and forwards them to the unified channel.
func (p *MassiveProvider) readLoop(conn *websocket.Conn, label string) {
	defer p.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Massive readLoop panicked",
				zap.Any("panic", r),
				zap.String("label", label),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	for {
		select {
		case <-p.stopCh:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil {
			p.mu.Lock()
			p.connected = false
			p.mu.Unlock()
			// Push a nil-length sentinel so ReadMessage returns an error
			select {
			case p.msgCh <- nil:
			default:
				p.logger.Warn("Failed to send disconnect sentinel: msgCh full, consumer may miss reconnect",
					zap.String("ws", label))
				wsSentinelDropsTotal.WithLabelValues("massive").Inc()
			}
			p.logger.Error("Read error", zap.String("ws", label), zap.Error(err))
			return
		}

		select {
		case p.msgCh <- message:
		case <-p.stopCh:
			return
		}
	}
}

func (p *MassiveProvider) Subscribe(symbols []string) error {
	p.mu.Lock()
	forexConn := p.forexConn
	cryptoConn := p.cryptoConn
	p.mu.Unlock()

	if forexConn == nil || cryptoConn == nil {
		return fmt.Errorf("not connected")
	}

	// Separate symbols into forex and crypto
	var forexSubs, cryptoSubs []string
	p.forexSymbols = nil
	p.cryptoSymbols = nil

	for _, sym := range symbols {
		if isForexSymbol(sym) {
			p.forexSymbols = append(p.forexSymbols, sym)
			forexSubs = append(forexSubs, canonicalToMassive(sym, true))
		} else {
			p.cryptoSymbols = append(p.cryptoSymbols, sym)
			cryptoSubs = append(cryptoSubs, canonicalToMassive(sym, false))
		}
	}

	// Subscribe on forex connection
	if len(forexSubs) > 0 {
		subMsg := map[string]string{
			"action": "subscribe",
			"params": strings.Join(forexSubs, ","),
		}
		if err := forexConn.WriteJSON(subMsg); err != nil {
			return fmt.Errorf("Massive forex subscribe failed: %w", err)
		}
		p.logger.Info("Forex subscribed", zap.Strings("symbols", forexSubs))
	}

	// Subscribe on crypto connection
	if len(cryptoSubs) > 0 {
		subMsg := map[string]string{
			"action": "subscribe",
			"params": strings.Join(cryptoSubs, ","),
		}
		if err := cryptoConn.WriteJSON(subMsg); err != nil {
			return fmt.Errorf("Massive crypto subscribe failed: %w", err)
		}
		p.logger.Info("Crypto subscribed", zap.Strings("symbols", cryptoSubs))
	}

	p.logger.Info("Subscribed",
		zap.Int("forex_count", len(forexSubs)),
		zap.Int("crypto_count", len(cryptoSubs)))
	return nil
}

// SubscribeSymbol subscribes to a single symbol on the active connection.
func (p *MassiveProvider) SubscribeSymbol(symbol string) error {
	p.mu.Lock()
	forexConn := p.forexConn
	cryptoConn := p.cryptoConn
	p.mu.Unlock()

	isFx := isForexSymbol(symbol)
	sub := canonicalToMassive(symbol, isFx)

	if isFx {
		if forexConn == nil {
			return fmt.Errorf("forex connection not available")
		}
		return forexConn.WriteJSON(map[string]string{
			"action": "subscribe",
			"params": sub,
		})
	}
	if cryptoConn == nil {
		return fmt.Errorf("crypto connection not available")
	}
	return cryptoConn.WriteJSON(map[string]string{
		"action": "subscribe",
		"params": sub,
	})
}

// UnsubscribeSymbol unsubscribes from a single symbol on the active connection.
func (p *MassiveProvider) UnsubscribeSymbol(symbol string) error {
	p.mu.Lock()
	forexConn := p.forexConn
	cryptoConn := p.cryptoConn
	p.mu.Unlock()

	isFx := isForexSymbol(symbol)
	sub := canonicalToMassive(symbol, isFx)

	if isFx {
		if forexConn == nil {
			return nil // already disconnected
		}
		return forexConn.WriteJSON(map[string]string{
			"action": "unsubscribe",
			"params": sub,
		})
	}
	if cryptoConn == nil {
		return nil
	}
	return cryptoConn.WriteJSON(map[string]string{
		"action": "unsubscribe",
		"params": sub,
	})
}

func (p *MassiveProvider) ReadMessage() ([]byte, error) {
	msg, ok := <-p.msgCh
	if !ok {
		return nil, fmt.Errorf("message channel closed")
	}
	if msg == nil {
		return nil, fmt.Errorf("connection lost")
	}
	return msg, nil
}

func (p *MassiveProvider) Close() error {
	// Signal reader goroutines to stop
	select {
	case <-p.stopCh:
	default:
		close(p.stopCh)
	}

	p.mu.Lock()
	var firstErr error
	if p.forexConn != nil {
		if err := p.forexConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.forexConn = nil
	}
	if p.cryptoConn != nil {
		if err := p.cryptoConn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.cryptoConn = nil
	}
	p.connected = false
	p.mu.Unlock()

	p.wg.Wait()

	// Re-initialize channels for potential reconnect
	p.msgCh = make(chan []byte, 256)
	p.stopCh = make(chan struct{})
	return firstErr
}

func (p *MassiveProvider) IsConnected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.connected
}

// RotateKey rotates to the next API key.
func (p *MassiveProvider) RotateKey() {
	p.keyRotator.Rotate()
}

// KeyCount returns the number of available API keys.
func (p *MassiveProvider) KeyCount() int {
	return p.keyRotator.Count()
}

// unwrapMassiveMessages handles both single objects and arrays from Massive.
// During high volume, Massive bundles messages as JSON arrays.
func unwrapMassiveMessages(data []byte) ([]json.RawMessage, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("empty message")
	}
	if data[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(data, &arr); err != nil {
			return nil, err
		}
		return arr, nil
	}
	return []json.RawMessage{data}, nil
}

// MassiveTick holds a parsed tick from any Massive event type.
type MassiveTick struct {
	Symbol  string // canonical symbol (e.g. "EUR/USD")
	RawPair string // raw Massive pair before conversion (e.g. "EUR-USD")
	Bid     float64
	Ask     float64
	Last    float64
	Ts      int64 // Unix seconds
}

// ParseMassiveMessage parses a Massive WebSocket message and returns extracted ticks.
func ParseMassiveMessage(data []byte) (ticks []MassiveTick, eventType string, err error) {
	events, err := unwrapMassiveMessages(data)
	if err != nil {
		return nil, "", fmt.Errorf("unwrap error: %w", err)
	}

	for _, raw := range events {
		// Peek at the "ev" field
		var peek struct {
			Ev string `json:"ev"`
		}
		if json.Unmarshal(raw, &peek) != nil {
			continue
		}

		switch peek.Ev {
		case "C": // Forex quote
			var q MassiveForexQuote
			if err := json.Unmarshal(raw, &q); err != nil {
				continue
			}
			last := (q.Bid + q.Ask) / 2
			ts := q.Timestamp / 1000 // ms → s
			if ts == 0 {
				ts = time.Now().Unix()
			}
			ticks = append(ticks, MassiveTick{
				Symbol:  massivePairToCanonical(q.Pair),
				RawPair: q.Pair,
				Bid:     q.Bid,
				Ask:     q.Ask,
				Last:    last,
				Ts:      ts,
			})
			eventType = "quote"

		case "XQ": // Crypto quote
			var q MassiveCryptoQuote
			if err := json.Unmarshal(raw, &q); err != nil {
				continue
			}
			last := (q.BidPrice + q.AskPrice) / 2
			ts := q.Timestamp / 1000
			if ts == 0 {
				ts = time.Now().Unix()
			}
			ticks = append(ticks, MassiveTick{
				Symbol:  massivePairToCanonical(q.Pair),
				RawPair: q.Pair,
				Bid:     q.BidPrice,
				Ask:     q.AskPrice,
				Last:    last,
				Ts:      ts,
			})
			eventType = "quote"

		case "XT": // Crypto trade (fallback: no bid/ask)
			var t MassiveCryptoTrade
			if err := json.Unmarshal(raw, &t); err != nil {
				continue
			}
			ts := t.Timestamp / 1000
			if ts == 0 {
				ts = time.Now().Unix()
			}
			ticks = append(ticks, MassiveTick{
				Symbol:  massivePairToCanonical(t.Pair),
				RawPair: t.Pair,
				Last:    t.Price,
				Ts:      ts,
			})
			eventType = "trade"

		case "status":
			var s MassiveStatus
			if json.Unmarshal(raw, &s) == nil {
				_ = s // Status logged by caller via ProviderManager
			}
			eventType = "status"
		}
	}

	return ticks, eventType, nil
}

// ProviderAlertHandler defines callbacks for provider-related alerts.
type ProviderAlertHandler interface {
	OnDisconnected(provider string, err error)
	OnReconnected(provider string, downtime time.Duration)
	OnFailover(from, to, reason string)
	OnSwitchback(to string)
	OnAllProvidersDown(err error)
}

// ProviderManager manages market data providers with failover support.
type ProviderManager struct {
	config         *Config
	primary        MarketProvider
	fallback       MarketProvider
	current        MarketProvider
	tickHandler    TickHandler
	alertHandler   ProviderAlertHandler
	symbolRegistry *SymbolRegistry
	logger         *zap.Logger

	// Failover state
	failureStart     time.Time
	failureCount     int64
	usingFallback    atomic.Bool
	lastSwitchback   time.Time
	disconnectedAt   time.Time    // Track when disconnection started
	lastTickReceived atomic.Int64 // P2-3: Unix timestamp of last tick (for staleness detection)

	// Synchronization
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Status
	ready  atomic.Bool
	readyC chan struct{}
}

// NewProviderManager creates a new provider manager.
func NewProviderManager(config *Config, tickHandler TickHandler, registry *SymbolRegistry, logger *zap.Logger) *ProviderManager {
	ctx, cancel := context.WithCancel(context.Background())

	pm := &ProviderManager{
		config:         config,
		tickHandler:    tickHandler,
		symbolRegistry: registry,
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
		readyC:         make(chan struct{}, 1),
	}

	// Initialize providers based on configuration
	switch config.MarketProvider {
	case ProviderTwelveData:
		pm.primary = NewTwelveDataProvider(config.TwelveDataAPIKeys, logger.Named("twelvedata"))
		pm.fallback = nil
	case ProviderMassive:
		pm.primary = NewMassiveProvider(config.MassiveAPIKeys, logger.Named("massive"))
		pm.fallback = nil
	case ProviderFinnhub:
		pm.primary = NewFinnhubProvider(config.FinnhubAPIKeys, logger.Named("finnhub"))
		pm.fallback = nil
	case ProviderAuto:
		// Finnhub is primary, TwelveData is fallback
		pm.primary = NewFinnhubProvider(config.FinnhubAPIKeys, logger.Named("finnhub"))
		pm.fallback = NewTwelveDataProvider(config.TwelveDataAPIKeys, logger.Named("twelvedata"))
	default:
		// Default to auto mode
		pm.primary = NewFinnhubProvider(config.FinnhubAPIKeys, logger.Named("finnhub"))
		pm.fallback = NewTwelveDataProvider(config.TwelveDataAPIKeys, logger.Named("twelvedata"))
	}

	pm.current = pm.primary

	return pm
}

// SetAlertHandler sets the alert handler for provider-related alerts.
func (pm *ProviderManager) SetAlertHandler(handler ProviderAlertHandler) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.alertHandler = handler
}

// Start begins the provider management loop.
func (pm *ProviderManager) Start() {
	pm.lastTickReceived.Store(time.Now().Unix())
	pm.wg.Add(1)
	go pm.connectionManager()

	// If auto-switchback is enabled and we have a fallback, start the health checker
	if pm.config.AutoSwitchback && pm.fallback != nil {
		pm.wg.Add(1)
		go pm.primaryHealthChecker()
	}

	// P2-3: Start staleness watchdog (detects silent provider failures)
	if pm.fallback != nil {
		pm.wg.Add(1)
		go pm.stalenessWatchdog()
	}
}

// stalenessWatchdog monitors for silent provider failures (P2-3).
// If no ticks are received for the configured threshold, triggers failover.
func (pm *ProviderManager) stalenessWatchdog() {
	defer pm.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			pm.logger.Error("stalenessWatchdog panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	threshold := pm.config.FailoverTimeout
	if threshold <= 0 {
		threshold = 30 * time.Second
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			lastTick := time.Unix(pm.lastTickReceived.Load(), 0)
			staleness := time.Since(lastTick)
			if staleness > threshold && pm.ready.Load() {
				pm.logger.Warn("No ticks received, triggering failover",
					zap.Duration("staleness", staleness),
					zap.Duration("threshold", threshold))

				pm.mu.RLock()
				handler := pm.alertHandler
				pm.mu.RUnlock()
				if handler != nil {
					handler.OnDisconnected("staleness_watchdog", fmt.Errorf("no ticks for %v", staleness))
				}

				// Force failover without rotating API keys (staleness is not a key issue)
				pm.forceStalenessFailover()
				if pm.shouldSwitchToFallback() {
					pm.switchToFallback()
				} else if pm.usingFallback.Load() {
					// Already on fallback and still stale — all providers are effectively down
					pm.mu.RLock()
					handler := pm.alertHandler
					pm.mu.RUnlock()
					if handler != nil {
						handler.OnAllProvidersDown(fmt.Errorf("fallback provider also stale: no ticks for %v", staleness))
					}
				}
			}
		}
	}
}

// Stop stops the provider manager.
func (pm *ProviderManager) Stop() {
	pm.cancel()
	pm.wg.Wait()

	pm.mu.Lock()
	if pm.current != nil {
		pm.current.Close()
	}
	pm.mu.Unlock()
}

// IsReady returns whether the provider manager has an active connection.
func (pm *ProviderManager) IsReady() bool {
	return pm.ready.Load()
}

// CurrentProvider returns the name of the current provider.
func (pm *ProviderManager) CurrentProvider() ProviderType {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	if pm.current != nil {
		return pm.current.Name()
	}
	return ""
}

// UsingFallback returns whether we're currently using the fallback provider.
func (pm *ProviderManager) UsingFallback() bool {
	return pm.usingFallback.Load()
}

// connectionManager manages the WebSocket connection with reconnection and failover logic.
func (pm *ProviderManager) connectionManager() {
	defer pm.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			pm.logger.Error("connectionManager panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	backoff := newBackoff(1*time.Second, 60*time.Second)

	for {
		select {
		case <-pm.ctx.Done():
			pm.closeCurrentProvider()
			return
		default:
		}

		err := pm.connectAndProcess()
		if err != nil {
			pm.logger.Error("Connection error", zap.Error(err))
			pm.closeCurrentProvider()
			pm.ready.Store(false)

			// Track disconnection time for reconnection alerts
			pm.mu.Lock()
			wasConnected := !pm.disconnectedAt.IsZero() || pm.failureCount == 0
			if pm.disconnectedAt.IsZero() {
				pm.disconnectedAt = time.Now()
			}
			currentProvider := ""
			if pm.current != nil {
				currentProvider = string(pm.current.Name())
			}
			pm.mu.Unlock()

			// Send disconnection alert (first time only)
			if wasConnected {
				pm.mu.RLock()
				handler := pm.alertHandler
				pm.mu.RUnlock()
				if handler != nil {
					handler.OnDisconnected(currentProvider, err)
				}
			}

			// Track failure for failover logic
			pm.recordFailure()

			// Check if we should switch to fallback
			if pm.shouldSwitchToFallback() {
				pm.switchToFallback()
				backoff.reset()
			} else if pm.fallback == nil || pm.usingFallback.Load() {
				// Both providers failed or no fallback available
				pm.mu.RLock()
				handler := pm.alertHandler
				pm.mu.RUnlock()
				if handler != nil && pm.failureCount > 2 {
					handler.OnAllProvidersDown(err)
				}
			}

			delay := backoff.next()
			pm.logger.Info("Reconnecting", zap.Duration("delay", delay))

			select {
			case <-time.After(delay):
			case <-pm.ctx.Done():
				return
			}
			continue
		}

		// Connection successful - calculate downtime and send reconnection alert
		pm.mu.Lock()
		var downtime time.Duration
		if !pm.disconnectedAt.IsZero() {
			downtime = time.Since(pm.disconnectedAt)
			pm.disconnectedAt = time.Time{}
		}
		currentProvider := ""
		if pm.current != nil {
			currentProvider = string(pm.current.Name())
		}
		handler := pm.alertHandler
		pm.mu.Unlock()

		if downtime > 0 && handler != nil {
			handler.OnReconnected(currentProvider, downtime)
		}

		// Reset failure tracking
		pm.clearFailure()
		backoff.reset()
	}
}

// connectAndProcess connects to the current provider and processes messages.
func (pm *ProviderManager) connectAndProcess() error {
	pm.mu.RLock()
	provider := pm.current
	pm.mu.RUnlock()

	if provider == nil {
		return fmt.Errorf("no provider available")
	}

	// Connect
	connectCtx, connectCancel := context.WithTimeout(pm.ctx, 15*time.Second)
	err := provider.Connect(connectCtx)
	connectCancel()

	if err != nil {
		return err
	}

	// Subscribe using symbol registry
	if err := pm.subscribeProvider(provider); err != nil {
		return err
	}

	// Signal ready
	pm.ready.Store(true)
	select {
	case pm.readyC <- struct{}{}:
	default:
	}

	pm.logger.Info("Connected to provider", zap.String("provider", string(provider.Name())))

	// Process messages
	return pm.processMessages(provider)
}

// subscribeProvider subscribes the provider using the symbol registry for provider-specific mappings.
func (pm *ProviderManager) subscribeProvider(provider MarketProvider) error {
	if pm.symbolRegistry == nil {
		// Fallback: use raw canonical symbols (backward compat)
		return provider.Subscribe(pm.config.Symbols)
	}

	switch provider.Name() {
	case ProviderMassive:
		return pm.subscribeMassive(provider.(*MassiveProvider))
	case ProviderTwelveData:
		tdSymbols := pm.symbolRegistry.TwelveDataSubscriptions()
		return provider.Subscribe(tdSymbols)
	case ProviderFinnhub:
		fhSymbols := pm.symbolRegistry.FinnhubSubscriptions()
		if len(fhSymbols) == 0 {
			// Fallback: convert canonical to Finnhub format
			for _, sym := range pm.config.Symbols {
				parts := strings.SplitN(sym, "/", 2)
				if len(parts) == 2 {
					fhSymbols = append(fhSymbols, "OANDA:"+parts[0]+"_"+parts[1])
				}
			}
		}
		return provider.Subscribe(fhSymbols)
	default:
		return provider.Subscribe(pm.config.Symbols)
	}
}

// subscribeMassive subscribes the Massive provider using the registry's pre-classified symbols.
func (pm *ProviderManager) subscribeMassive(provider *MassiveProvider) error {
	provider.mu.Lock()
	forexConn := provider.forexConn
	cryptoConn := provider.cryptoConn
	provider.mu.Unlock()

	if forexConn == nil || cryptoConn == nil {
		return fmt.Errorf("not connected")
	}

	// Use registry-classified symbols
	provider.forexSymbols = pm.symbolRegistry.ForexSymbols
	provider.cryptoSymbols = pm.symbolRegistry.CryptoSymbols

	forexSubs := pm.symbolRegistry.MassiveForexSubscriptions()
	cryptoSubs := pm.symbolRegistry.MassiveCryptoSubscriptions()

	// Skip Massive crypto subscriptions when Nobitex handles crypto
	if pm.config.NobitexEnabled {
		cryptoSubs = nil
		pm.logger.Info("Skipping Massive crypto subscriptions (Nobitex enabled)")
	}

	// Subscribe on forex connection
	if len(forexSubs) > 0 {
		subMsg := map[string]string{
			"action": "subscribe",
			"params": strings.Join(forexSubs, ","),
		}
		if err := forexConn.WriteJSON(subMsg); err != nil {
			return fmt.Errorf("Massive forex subscribe failed: %w", err)
		}
		pm.logger.Info("Massive forex resubscribed", zap.Strings("symbols", forexSubs))
	}

	// Subscribe on crypto connection
	if len(cryptoSubs) > 0 {
		subMsg := map[string]string{
			"action": "subscribe",
			"params": strings.Join(cryptoSubs, ","),
		}
		if err := cryptoConn.WriteJSON(subMsg); err != nil {
			return fmt.Errorf("Massive crypto subscribe failed: %w", err)
		}
		pm.logger.Info("Massive crypto resubscribed", zap.Strings("symbols", cryptoSubs))
	}

	pm.logger.Info("Massive resubscribed",
		zap.Int("forex_count", len(forexSubs)),
		zap.Int("crypto_count", len(cryptoSubs)))
	return nil
}

// processMessages reads and processes messages from the provider.
func (pm *ProviderManager) processMessages(provider MarketProvider) error {
	for {
		select {
		case <-pm.ctx.Done():
			return nil
		default:
		}

		message, err := provider.ReadMessage()
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		pm.handleMessage(provider.Name(), message)
	}
}

// handleMessage processes an incoming message based on provider type.
func (pm *ProviderManager) handleMessage(providerType ProviderType, data []byte) {
	switch providerType {
	case ProviderTwelveData:
		symbol, price, ts, eventType, err := ParseTwelveDataMessage(data)
		if err != nil {
			pm.logger.Warn("TwelveData parse error", zap.Error(err))
			return
		}
		if eventType == "price" && symbol != "" {
			// Map TwelveData symbol back to canonical using registry
			if pm.symbolRegistry != nil {
				symbol = pm.symbolRegistry.TwelveDataToCanonical(symbol)
			}
			// TwelveData only provides last price; bid/ask and volume passed as zero
			pm.tickHandler(symbol, price, 0, 0, 0, ts, "twelvedata")
			pm.lastTickReceived.Store(time.Now().Unix()) // P2-3
		}

	case ProviderMassive:
		ticks, eventType, err := ParseMassiveMessage(data)
		if err != nil {
			pm.logger.Warn("Massive parse error", zap.Error(err))
			return
		}
		if eventType == "quote" || eventType == "trade" {
			for _, tick := range ticks {
				// Map Massive pair back to canonical using registry
				canonicalSymbol := tick.Symbol
				if pm.symbolRegistry != nil && tick.RawPair != "" {
					canonicalSymbol = pm.symbolRegistry.MassiveToCanonical(tick.RawPair)
				}
				pm.tickHandler(canonicalSymbol, tick.Last, tick.Bid, tick.Ask, 0, tick.Ts, "massive")
			}
			if len(ticks) > 0 {
				pm.lastTickReceived.Store(time.Now().Unix()) // P2-3
			}
		}

	case ProviderFinnhub:
		trades, msgType, err := ParseFinnhubMessage(data)
		if err != nil {
			pm.logger.Warn("Finnhub parse error", zap.Error(err))
			return
		}
		if msgType == "trade" {
			for _, trade := range trades {
				symbol := trade.Symbol
				if pm.symbolRegistry != nil {
					symbol = pm.symbolRegistry.FinnhubToCanonical(trade.Symbol)
				}
				// Finnhub timestamp is in milliseconds; convert to seconds
				ts := trade.Timestamp / 1000
				// Finnhub only provides last price (no bid/ask)
				pm.tickHandler(symbol, trade.Price, 0, 0, trade.Volume, ts, "finnhub")
			}
			if len(trades) > 0 {
				pm.lastTickReceived.Store(time.Now().Unix())
			}
		}
	}
}

// recordFailure records a connection failure for failover tracking.
// It also rotates to the next API key if multiple keys are available.
func (pm *ProviderManager) recordFailure() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.failureCount == 0 {
		pm.failureStart = time.Now()
	}
	pm.failureCount++

	// Rotate to next API key if available
	if pm.current != nil && pm.current.KeyCount() > 1 {
		pm.logger.Info("Connection failed, rotating API key",
			zap.String("provider", string(pm.current.Name())))
		pm.current.RotateKey()
	}
}

// clearFailure clears the failure tracking state.
func (pm *ProviderManager) clearFailure() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.failureCount = 0
	pm.failureStart = time.Time{}
}

// forceStalenessFailover forces failover due to tick staleness without rotating API keys,
// since staleness is not an API key issue.
func (pm *ProviderManager) forceStalenessFailover() {
	pm.mu.Lock()
	pm.failureCount = 1
	pm.failureStart = time.Now().Add(-pm.config.FailoverTimeout)
	pm.mu.Unlock()
}

// shouldSwitchToFallback determines if we should switch to the fallback provider.
func (pm *ProviderManager) shouldSwitchToFallback() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Don't switch if we don't have a fallback
	if pm.fallback == nil {
		return false
	}

	// Don't switch if we're already using the fallback
	if pm.usingFallback.Load() {
		return false
	}

	// Check if we've been failing long enough
	if pm.failureCount > 0 && time.Since(pm.failureStart) >= pm.config.FailoverTimeout {
		return true
	}

	return false
}

// switchToFallback switches to the fallback provider.
// The swap is performed before closing the old provider to prevent concurrent
// goroutines (e.g. connectAndProcess) from accessing a closed provider.
func (pm *ProviderManager) switchToFallback() {
	pm.mu.Lock()

	if pm.fallback == nil {
		pm.mu.Unlock()
		return
	}

	fromProvider := string(pm.current.Name())
	toProvider := string(pm.fallback.Name())
	handler := pm.alertHandler

	pm.logger.Warn("Switching to fallback provider",
		zap.String("from", fromProvider),
		zap.String("to", toProvider))

	// Save old provider, swap FIRST so new readers see fallback immediately
	old := pm.current
	pm.current = pm.fallback
	pm.usingFallback.Store(true)
	pm.failureCount = 0
	pm.failureStart = time.Time{}
	pm.mu.Unlock()

	// Close old provider AFTER swap, outside lock
	if old != nil {
		old.Close()
	}

	// Send failover alert
	if handler != nil {
		handler.OnFailover(fromProvider, toProvider, "Primary provider failed, switching to fallback")
	}
}

// switchToPrimary switches back to the primary provider.
// The swap is performed before closing the old provider to prevent concurrent
// goroutines (e.g. connectAndProcess) from accessing a closed provider.
func (pm *ProviderManager) switchToPrimary() {
	pm.mu.Lock()

	if pm.primary == nil {
		pm.mu.Unlock()
		return
	}

	pm.logger.Info("Switching back to primary provider",
		zap.String("provider", string(pm.primary.Name())))

	// Save old provider for deferred close
	old := pm.current

	// Reset primary provider (create new instance to clear state)
	switch pm.primary.Name() {
	case ProviderTwelveData:
		pm.primary = NewTwelveDataProvider(pm.config.TwelveDataAPIKeys, pm.logger.Named("twelvedata"))
	case ProviderMassive:
		pm.primary = NewMassiveProvider(pm.config.MassiveAPIKeys, pm.logger.Named("massive"))
	case ProviderFinnhub:
		pm.primary = NewFinnhubProvider(pm.config.FinnhubAPIKeys, pm.logger.Named("finnhub"))
	}

	toProvider := string(pm.primary.Name())
	handler := pm.alertHandler

	// Swap FIRST so new readers see primary immediately
	pm.current = pm.primary
	pm.usingFallback.Store(false)
	pm.lastSwitchback = time.Now()
	pm.failureCount = 0
	pm.failureStart = time.Time{}
	pm.mu.Unlock()

	// Close old provider AFTER swap, outside lock
	if old != nil {
		old.Close()
	}

	// Send switchback alert
	if handler != nil {
		handler.OnSwitchback(toProvider)
	}
}

// primaryHealthChecker periodically checks if the primary provider is healthy.
func (pm *ProviderManager) primaryHealthChecker() {
	defer pm.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			pm.logger.Error("primaryHealthChecker panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	ticker := time.NewTicker(pm.config.HealthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.checkPrimaryHealth()
		}
	}
}

// checkPrimaryHealth checks if we should switch back to the primary provider.
func (pm *ProviderManager) checkPrimaryHealth() {
	// Only check if we're using the fallback
	if !pm.usingFallback.Load() {
		return
	}

	// Don't switch back too quickly
	pm.mu.RLock()
	timeSinceSwitch := time.Since(pm.lastSwitchback)
	pm.mu.RUnlock()

	if timeSinceSwitch < pm.config.SwitchbackDelay {
		return
	}

	// Try to connect to primary (use the actual primary provider type)
	pm.mu.RLock()
	primaryName := pm.primary.Name()
	pm.mu.RUnlock()

	var testProvider MarketProvider
	switch primaryName {
	case ProviderMassive:
		testProvider = NewMassiveProvider(pm.config.MassiveAPIKeys, pm.logger.Named("massive-healthcheck"))
	case ProviderTwelveData:
		testProvider = NewTwelveDataProvider(pm.config.TwelveDataAPIKeys, pm.logger.Named("twelvedata-healthcheck"))
	case ProviderFinnhub:
		testProvider = NewFinnhubProvider(pm.config.FinnhubAPIKeys, pm.logger.Named("finnhub-healthcheck"))
	default:
		pm.logger.Warn("Unknown primary provider type, skipping health check",
			zap.String("provider", string(primaryName)))
		return
	}
	ctx, cancel := context.WithTimeout(pm.ctx, 10*time.Second)
	err := testProvider.Connect(ctx)
	cancel()

	if err != nil {
		pm.logger.Warn("Primary health check failed", zap.Error(err))
		testProvider.Close()
		return
	}

	// Primary is healthy, switch back
	pm.logger.Info("Primary provider is healthy, switching back")
	testProvider.Close()

	// Signal connection manager to reconnect (it will use primary)
	pm.switchToPrimary()
}

// closeCurrentProvider safely closes the current provider.
func (pm *ProviderManager) closeCurrentProvider() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.current != nil {
		pm.current.Close()
	}
}

// App holds application state and dependencies.
type App struct {
	config           *Config
	redis            *pkgredis.Client
	kafka            *kgo.Client
	db               *sql.DB
	providerManager  *ProviderManager
	obs              *observability.Observability
	notifications    *notification.Service
	alertAggregator  *AlertAggregator
	candleAggregator *CandleAggregator
	spreadManager    *SpreadManager
	nobitexFeed      *NobitexCryptoFeed
	binanceFeed      *BinanceCryptoFeed
	symbolRegistry   *SymbolRegistry

	// activeCryptoProvider tracks the current crypto price source: "nobitex", "binance", "both"
	activeCryptoProvider atomic.Value // stores string

	// Tick publish worker pool
	tickPublishCh chan tickJob

	// In-memory tick storage (1Hz snapshots per symbol)
	ticks        map[string]*TickData
	ticksMu      sync.RWMutex
	dirtySymbols sync.Map // tracks symbols with new ticks since last snapshot

	// Dynamic symbol subscription manager
	dynamicSymbols *DynamicSymbolManager

	// Metrics
	redisPublishFailures prometheus.Counter

	// Shutdown coordination
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Shared resource flags - when true, these resources are shared and should not be closed
	sharedDB    bool
	sharedRedis bool
}

// log returns the logger from observability
func (a *App) log() *zap.Logger {
	return a.obs.Logger.Logger
}

func loadConfig() *Config {
	// Bootstrap logger for config loading (before observability is initialized)
	bootstrapLogger, _ := zap.NewProduction()
	bootstrapLogger = observability.WrapLogger(bootstrapLogger)
	defer bootstrapLogger.Sync()

	port := os.Getenv("MARKET_INGESTOR_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8084"
	}

	// Load API keys using secrets package (supports env vars, files, and Docker secrets)
	twelveDataAPIKeys := secrets.LoadList("TWELVEDATA_API_KEYS")
	massiveAPIKeys := secrets.LoadList("MASSIVE_API_KEYS")
	finnhubAPIKeys := secrets.LoadList("FINNHUB_API_KEYS")

	// Log secret loading diagnostics (masked)
	diagnostics := secrets.DiagnosticReport("TWELVEDATA_API_KEYS", "MASSIVE_API_KEYS", "FINNHUB_API_KEYS")
	for _, diag := range diagnostics {
		if diag.Loaded {
			bootstrapLogger.Info("Secret loaded",
				zap.String("name", diag.Name),
				zap.String("source", diag.Source))
		} else {
			bootstrapLogger.Warn("Secret not found", zap.String("name", diag.Name))
		}
	}

	// Warn if all keys are missing
	if len(twelveDataAPIKeys) == 0 && len(massiveAPIKeys) == 0 && len(finnhubAPIKeys) == 0 {
		bootstrapLogger.Warn("No market data API keys set (TWELVEDATA_API_KEYS, MASSIVE_API_KEYS, FINNHUB_API_KEYS)")
	} else {
		bootstrapLogger.Info("API keys loaded",
			zap.Int("twelvedata_keys", len(twelveDataAPIKeys)),
			zap.Int("massive_keys", len(massiveAPIKeys)),
			zap.Int("finnhub_keys", len(finnhubAPIKeys)))
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		if config.IsProduction() {
			bootstrapLogger.Fatal("REDIS_ADDR must be set in production")
		}
		redisAddr = "localhost:6379"
		bootstrapLogger.Warn("REDIS_ADDR not set, using localhost:6379")
	}

	brokersStr := os.Getenv("KAFKA_BROKERS")
	if brokersStr == "" {
		if config.IsProduction() {
			bootstrapLogger.Fatal("KAFKA_BROKERS must be set in production")
		}
		brokersStr = "localhost:9092"
		bootstrapLogger.Warn("KAFKA_BROKERS not set, using localhost:9092")
	}
	brokers := strings.Split(brokersStr, ",")

	symbolsStr := os.Getenv("SYMBOLS")
	if symbolsStr == "" {
		symbolsStr = "AAPL,MSFT,GOOGL"
	}
	symbols := strings.Split(symbolsStr, ",")
	for i := range symbols {
		symbols[i] = strings.TrimSpace(symbols[i])
	}

	// Provider configuration
	providerStr := strings.ToLower(os.Getenv("MARKET_PROVIDER"))
	var provider ProviderType
	switch providerStr {
	case "twelvedata":
		provider = ProviderTwelveData
	case "massive":
		provider = ProviderMassive
	case "finnhub":
		provider = ProviderFinnhub
	case "auto", "":
		provider = ProviderAuto
	default:
		bootstrapLogger.Warn("Unknown MARKET_PROVIDER, using 'auto'",
			zap.String("value", providerStr))
		provider = ProviderAuto
	}

	// Failover timeout (default 30 seconds)
	failoverTimeoutStr := os.Getenv("FAILOVER_TIMEOUT")
	failoverTimeout := 30 * time.Second
	if failoverTimeoutStr != "" {
		if d, err := time.ParseDuration(failoverTimeoutStr); err == nil {
			failoverTimeout = d
		} else {
			bootstrapLogger.Warn("Invalid FAILOVER_TIMEOUT, using default 30s",
				zap.String("value", failoverTimeoutStr))
		}
	}

	// Auto-switchback (default true)
	autoSwitchbackStr := strings.ToLower(os.Getenv("AUTO_SWITCHBACK"))
	autoSwitchback := true
	if autoSwitchbackStr == "false" || autoSwitchbackStr == "0" || autoSwitchbackStr == "no" {
		autoSwitchback = false
	}

	// Switchback delay (default 60 seconds)
	switchbackDelayStr := os.Getenv("SWITCHBACK_DELAY")
	switchbackDelay := 60 * time.Second
	if switchbackDelayStr != "" {
		if d, err := time.ParseDuration(switchbackDelayStr); err == nil {
			switchbackDelay = d
		} else {
			bootstrapLogger.Warn("Invalid SWITCHBACK_DELAY, using default 60s",
				zap.String("value", switchbackDelayStr))
		}
	}

	// Health check period (default 30 seconds)
	healthCheckPeriodStr := os.Getenv("HEALTH_CHECK_PERIOD")
	healthCheckPeriod := 30 * time.Second
	if healthCheckPeriodStr != "" {
		if d, err := time.ParseDuration(healthCheckPeriodStr); err == nil {
			healthCheckPeriod = d
		} else {
			bootstrapLogger.Warn("Invalid HEALTH_CHECK_PERIOD, using default 30s",
				zap.String("value", healthCheckPeriodStr))
		}
	}

	// PostgreSQL DSN for candle storage
	postgresDSN := secrets.BuildPostgresDSN()

	// Candle flush interval (default 5 seconds)
	candleFlushIntervalStr := os.Getenv("CANDLE_FLUSH_INTERVAL")
	candleFlushInterval := 5 * time.Second
	if candleFlushIntervalStr != "" {
		if d, err := time.ParseDuration(candleFlushIntervalStr); err == nil {
			candleFlushInterval = d
		} else {
			bootstrapLogger.Warn("Invalid CANDLE_FLUSH_INTERVAL, using default 5s",
				zap.String("value", candleFlushIntervalStr))
		}
	}

	// Candle batch size (default 100)
	candleBatchSize := config.GetEnvInt("CANDLE_BATCH_SIZE", 100)

	// Enable candles (default true)
	enableCandles := config.GetEnvBool("ENABLE_CANDLES", true)

	// Nobitex crypto feed configuration
	nobitexEnabled := config.GetEnvBool("NOBITEX_ENABLED", true)
	nobitexToken := secrets.Load("NOBITEX_TOKEN")
	nobitexBaseURL := config.GetEnvString("NOBITEX_BASE_URL", "https://apiv2.nobitex.ir")

	nobitexPollIntervalStr := os.Getenv("NOBITEX_POLL_INTERVAL")
	nobitexPollInterval := 2 * time.Second
	if nobitexPollIntervalStr != "" {
		if d, err := time.ParseDuration(nobitexPollIntervalStr); err == nil {
			nobitexPollInterval = d
		} else {
			bootstrapLogger.Warn("Invalid NOBITEX_POLL_INTERVAL, using default 2s",
				zap.String("value", nobitexPollIntervalStr))
		}
	}

	nobitexUSDTRate := 1.0
	if rateStr := os.Getenv("NOBITEX_USDT_USD_RATE"); rateStr != "" {
		if r, err := strconv.ParseFloat(rateStr, 64); err == nil && r > 0 {
			nobitexUSDTRate = r
		}
	}

	// Binance crypto feed configuration
	binanceEnabled := config.GetEnvBool("BINANCE_ENABLED", false)
	binanceBaseURL := config.GetEnvString("BINANCE_BASE_URL", "wss://stream.binance.com:9443")
	binanceUSDTRate := 1.0
	if rateStr := os.Getenv("BINANCE_USDT_USD_RATE"); rateStr != "" {
		if r, err := strconv.ParseFloat(rateStr, 64); err == nil && r > 0 {
			binanceUSDTRate = r
		}
	}

	// Crypto provider selection: "nobitex" (default), "binance", "both"
	cryptoProvider := config.GetEnvString("CRYPTO_PROVIDER", "nobitex")

	return &Config{
		Port:              port,
		TwelveDataAPIKeys: twelveDataAPIKeys,
		MassiveAPIKeys:    massiveAPIKeys,
		FinnhubAPIKeys:    finnhubAPIKeys,
		RedisAddr:         redisAddr,
		KafkaBrokers:      brokers,
		Symbols:           symbols,
		PostgresDSN:       postgresDSN,
		TicksTopic:        config.GetEnvString("TICKS_TOPIC", "ticks.v1"),
		MarketProvider:    provider,
		FailoverTimeout:   failoverTimeout,
		AutoSwitchback:    autoSwitchback,
		SwitchbackDelay:   switchbackDelay,
		HealthCheckPeriod: healthCheckPeriod,

		// Candle aggregation configuration
		CandleFlushInterval: candleFlushInterval,
		CandleBatchSize:     candleBatchSize,
		EnableCandles:       enableCandles,

		// Nobitex crypto feed configuration
		NobitexEnabled:      nobitexEnabled,
		NobitexToken:        nobitexToken,
		NobitexBaseURL:      nobitexBaseURL,
		NobitexPollInterval: nobitexPollInterval,
		NobitexUSDTRate:     nobitexUSDTRate,

		// Binance crypto feed configuration
		BinanceEnabled:  binanceEnabled,
		BinanceBaseURL:  binanceBaseURL,
		BinanceUSDTRate: binanceUSDTRate,
		CryptoProvider:  cryptoProvider,

		// Notification configuration (using secrets package for sensitive values)
		NotificationEnabled:      config.GetEnvBool("NOTIFICATION_ENABLED", true),
		NotificationAsync:        config.GetEnvBool("NOTIFICATION_ASYNC", true),
		NotificationAsyncWorkers: config.GetEnvInt("NOTIFICATION_ASYNC_WORKERS", 5),
		NotificationQueueSize:    config.GetEnvInt("NOTIFICATION_ASYNC_QUEUE_SIZE", 100),
		DiscordWebhookURL:        secrets.Load("DISCORD_WEBHOOK_URL"),
		ResendAPIKey:             secrets.Load("RESEND_API_KEY"),
		ResendFromEmail:          config.GetEnvString("RESEND_FROM_EMAIL", "onboarding@resend.dev"),
		NotificationRecipients:   os.Getenv("NOTIFICATION_EMAIL_RECIPIENTS"),
		Environment:              config.GetEnvString("ENVIRONMENT", "development"),

		// Tick publish worker pool
		TickPublishWorkers:   config.GetEnvInt("TICK_PUBLISH_WORKERS", 16),
		TickPublishQueueSize: config.GetEnvInt("TICK_PUBLISH_QUEUE_SIZE", 1024),

		// Control API authentication
		ControlAPIKey: os.Getenv("CONTROL_API_KEY"),
	}
}

func mustGetEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		// In development, allow insecure defaults
		if os.Getenv("ENVIRONMENT") == "development" || os.Getenv("ENVIRONMENT") == "" {
			switch key {
			case "POSTGRES_DSN":
				println("WARNING: " + key + " not set, using insecure development default")
				localURL := &url.URL{
					Scheme: "postgres",
					Host:   "localhost:5432",
					Path:   "/app",
					User:   url.UserPassword("app", "app"),
				}
				query := localURL.Query()
				query.Set("sslmode", "disable")
				localURL.RawQuery = query.Encode()
				return localURL.String()
			}
		}
		panic("required environment variable not set: " + key)
	}
	return val
}

// Run starts the market-ingestor service in standalone mode with its own resources.
func Run() {
	RunWithSharedDeps(nil, nil, nil)
}

// RunWithSharedDeps starts the market-ingestor service, optionally using shared resources.
// When parentCtx is non-nil, the service shuts down when the context is cancelled
// instead of registering its own signal handler. When sharedPool is non-nil, the service
// uses pool.Primary() for its *sql.DB instead of creating its own connection.
func RunWithSharedDeps(parentCtx context.Context, sharedPool *db.Pool, sharedRedis *pkgredis.Client) {
	// Validate critical environment variables in production/staging
	if sharedPool == nil {
		config.MustBeSetAny("database connection", "POSTGRES_DSN", "POSTGRES_HOST")
	}
	if sharedRedis == nil {
		config.MustBeSet("REDIS_ADDR")
	}
	config.MustBeSet("KAFKA_BROKERS")

	cfg := loadConfig()

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize observability first
	obs, err := observability.New(ctx, observability.Config{
		Service:              "market-ingestor",
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

	zapLog := obs.Logger.Logger

	// Create a new cancellable context for the application
	ctx, cancel = context.WithCancel(ctx)

	// Validate CONTROL_API_KEY at startup
	knownPlaceholders := map[string]bool{
		"your-control-api-key-here": true,
		"changeme":                  true,
		"test":                      true,
		"secret":                    true,
	}
	if cfg.ControlAPIKey == "" || knownPlaceholders[cfg.ControlAPIKey] {
		if cfg.Environment == "development" || cfg.Environment == "" {
			zapLog.Warn("CONTROL_API_KEY not set or is a placeholder — control endpoints are disabled in development")
			cfg.ControlAPIKey = "" // Disable control endpoints
		} else {
			zapLog.Fatal("CONTROL_API_KEY must be set to a strong random value in non-development environments. Generate with: openssl rand -hex 32")
		}
	} else if len(cfg.ControlAPIKey) < 32 {
		zapLog.Warn("CONTROL_API_KEY is shorter than 32 characters — consider using a stronger key (openssl rand -hex 32)")
	}

	app := &App{
		config: cfg,
		ticks:  make(map[string]*TickData),
		ctx:    ctx,
		cancel: cancel,
		obs:    obs,
	}

	// Register Redis publish failure metric
	app.redisPublishFailures = obs.Metrics.NewCounter(prometheus.CounterOpts{
		Namespace: "market_ingestor",
		Name:      "redis_publish_failures_total",
		Help:      "Total number of Redis publish failures in publishTick",
	})

	// Initialize tick publish worker pool
	app.tickPublishCh = make(chan tickJob, cfg.TickPublishQueueSize)
	for i := 0; i < cfg.TickPublishWorkers; i++ {
		app.wg.Add(1)
		go app.tickPublishWorker()
	}
	zapLog.Info("Tick publish worker pool started",
		zap.Int("workers", cfg.TickPublishWorkers),
		zap.Int("queue_size", cfg.TickPublishQueueSize))

	// Initialize Redis client with HA support (optional - continue if unavailable)
	if sharedRedis != nil {
		app.redis = sharedRedis
		app.sharedRedis = true
		zapLog.Info("Using shared Redis client")
	} else {
		redisCfg := pkgredis.ConfigFromEnv(os.Getenv)
		if redisCfg.Addr == "" && redisCfg.Mode == pkgredis.ModeStandalone {
			redisCfg.Addr = cfg.RedisAddr
		}
		redisClient, redisErr := pkgredis.NewClient(redisCfg)
		if redisErr != nil {
			zapLog.Warn("Failed to create Redis client", zap.Error(redisErr))
		} else {
			app.redis = redisClient
			// Test Redis connection
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			if err := app.redis.Ping(pingCtx).Err(); err != nil {
				zapLog.Warn("Redis not available (will retry on writes)", zap.Error(err))
			} else {
				zapLog.Info("Redis connected successfully",
					zap.String("mode", string(app.redis.Mode())))
			}
			pingCancel()
		}
	}

	// Initialize spread manager for per-symbol spread configuration
	spreadConfigPath := config.GetEnvString("SPREAD_CONFIG_PATH", "spread_config.json")
	spreadMgr, spreadErr := NewSpreadManager(spreadConfigPath, app.redis, zapLog)
	if spreadErr != nil {
		zapLog.Warn("Failed to initialize spread manager, using defaults", zap.Error(spreadErr))
		// Create a minimal spread manager with default config
		spreadMgr = &SpreadManager{
			redis:  app.redis,
			logger: zapLog,
			config: &SpreadConfig{
				DefaultSpreadBps:   DefaultSpreadBps,
				SymbolSpreads:      make(map[string]int),
				AssetClassDefaults: make(map[string]int),
			},
		}
	}
	app.spreadManager = spreadMgr
	app.spreadManager.Start(ctx)
	zapLog.Info("Spread manager initialized",
		zap.Int("default_spread_bps", app.spreadManager.config.DefaultSpreadBps),
		zap.Int("symbol_overrides", len(app.spreadManager.config.SymbolSpreads)))

	// Initialize Kafka producer (optional - continue if unavailable)
	// Optimized for high-throughput tick data publishing
	var kafkaErr error
	kafkaOpts := []kgo.Opt{
		kgo.SeedBrokers(cfg.KafkaBrokers...),
		// Producer optimizations for high-frequency tick data
		kgo.ProducerBatchCompression(kgo.Lz4Compression()), // LZ4: fast compression for tick data
		kgo.ProducerLinger(100 * time.Millisecond),         // Higher linger for better tick batching
		kgo.ProducerBatchMaxBytes(1024 * 1024 * 2),         // 2MB max batch size for tick data
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.RetryTimeout(10 * time.Second),
		kgo.AllowAutoTopicCreation(), // Allow automatic topic creation for ticks.v1
	}
	kafkaOpts = append(kafkaOpts, infra.KafkaSecurityOpts()...)
	app.kafka, kafkaErr = kgo.NewClient(kafkaOpts...)
	if kafkaErr != nil {
		zapLog.Warn("Kafka client creation failed (will skip Kafka publishes)", zap.Error(kafkaErr))
		app.kafka = nil
	} else {
		zapLog.Info("Kafka client initialized (high-throughput mode)")
	}

	// Initialize PostgreSQL (used for symbol loading and candle storage)
	if sharedPool != nil {
		app.db = sharedPool.Primary()
		app.sharedDB = true
		zapLog.Info("Using shared database pool")
	} else {
		localDB, dbErr := sql.Open("postgres", cfg.PostgresDSN)
		if dbErr != nil {
			zapLog.Warn("PostgreSQL connection failed", zap.Error(dbErr))
		} else {
			// Configure connection pool
			localDB.SetMaxOpenConns(10)
			localDB.SetMaxIdleConns(5)
			localDB.SetConnMaxLifetime(5 * time.Minute)

			// Test connection
			pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
			if err := localDB.PingContext(pingCtx); err != nil {
				zapLog.Warn("PostgreSQL ping failed", zap.Error(err))
				localDB.Close()
			} else {
				zapLog.Info("PostgreSQL connected")
				app.db = localDB
			}
			pingCancel()
		}
	}

	// Load symbol registry from database with env var fallback
	if app.db != nil {
		reg, err := loadSymbolsFromDB(app.db)
		if err != nil {
			zapLog.Warn("Failed to load symbols from DB, falling back to SYMBOLS env var",
				zap.Error(err))
			app.symbolRegistry = buildRegistryFromEnv(cfg.Symbols)
		} else {
			// If SYMBOLS env var is explicitly set, use it as a filter on DB symbols
			symbolsEnv := os.Getenv("SYMBOLS")
			if symbolsEnv != "" {
				reg = filterRegistry(reg, cfg.Symbols)
				zapLog.Info("Filtered DB symbols with SYMBOLS env var",
					zap.Int("filtered_count", len(reg.CanonicalSymbols)))
			}
			app.symbolRegistry = reg
			// Override config symbols with DB-loaded canonical symbols
			cfg.Symbols = reg.CanonicalSymbols
		}
	} else {
		zapLog.Warn("Database unavailable at startup, using SYMBOLS env var for symbol configuration")
		app.symbolRegistry = buildRegistryFromEnv(cfg.Symbols)
	}

	// Initialize candle aggregator if database and candles are enabled
	if cfg.EnableCandles && app.db != nil {
		candleConfig := CandleAggregatorConfig{
			FlushInterval:          cfg.CandleFlushInterval,
			BatchSize:              cfg.CandleBatchSize,
			EnableHigherTimeframes: true,
			CheckpointInterval:     10 * time.Second,
		}
		app.candleAggregator = NewCandleAggregator(app.db, app.redis, zapLog, candleConfig)
		app.candleAggregator.Start()
		zapLog.Info("Candle aggregator started",
			zap.Duration("flush_interval", cfg.CandleFlushInterval),
			zap.Int("batch_size", cfg.CandleBatchSize))
	}

	// Initialize notification service
	app.initNotifications(ctx, zapLog)

	// Initialize provider manager
	app.providerManager = NewProviderManager(cfg, app.handleTick, app.symbolRegistry, zapLog)

	// Set alert handler for provider-related notifications
	app.providerManager.SetAlertHandler(app)

	// Log provider configuration
	zapLog.Info("Market provider mode", zap.String("provider", string(cfg.MarketProvider)))
	if cfg.MarketProvider == ProviderAuto {
		zapLog.Info("Failover configuration",
			zap.Duration("failover_timeout", cfg.FailoverTimeout),
			zap.Bool("auto_switchback", cfg.AutoSwitchback))
		if cfg.AutoSwitchback {
			zapLog.Info("Switchback configuration",
				zap.Duration("switchback_delay", cfg.SwitchbackDelay),
				zap.Duration("health_check_period", cfg.HealthCheckPeriod))
		}
	}

	// Start provider manager
	app.providerManager.Start()

	// Initialize crypto feeds (independent of ProviderManager)
	// ProviderManager (Massive/TwelveData) handles forex/commodity/stocks via WebSocket.
	// Crypto is handled by Nobitex (REST) and/or Binance (WS) based on CryptoProvider config.

	// Load crypto provider preference from DB if available
	cryptoProvider := cfg.CryptoProvider
	if app.db != nil {
		var dbProvider string
		err := app.db.QueryRow("SELECT active_provider FROM provider_config WHERE asset_class = 'crypto'").Scan(&dbProvider)
		if err == nil && dbProvider != "" {
			cryptoProvider = dbProvider
			zapLog.Info("Loaded crypto provider from DB", zap.String("provider", cryptoProvider))
		}
	}
	app.activeCryptoProvider.Store(cryptoProvider)

	// Build both feeds (cheap — just struct allocation)
	if cfg.NobitexEnabled && len(app.symbolRegistry.CryptoSymbols) > 0 {
		nobitexCfg := NobitexConfig{
			Token:        cfg.NobitexToken,
			PollInterval: cfg.NobitexPollInterval,
			USDTUSDRate:  cfg.NobitexUSDTRate,
			BaseURL:      cfg.NobitexBaseURL,
			Symbols:      app.symbolRegistry.NobitexSubscriptions(),
			Enabled:      true,
		}
		app.nobitexFeed = NewNobitexCryptoFeed(nobitexCfg, app.handleTick, app.symbolRegistry, zapLog)
	}

	if len(app.symbolRegistry.CryptoSymbols) > 0 {
		binanceCfg := BinanceConfig{
			BaseURL:     cfg.BinanceBaseURL,
			Symbols:     app.symbolRegistry.BinanceSubscriptions(),
			USDTUSDRate: cfg.BinanceUSDTRate,
			Enabled:     cfg.BinanceEnabled || cryptoProvider == "binance" || cryptoProvider == "both",
		}
		app.binanceFeed = NewBinanceCryptoFeed(binanceCfg, app.handleTick, app.symbolRegistry, zapLog)
	}

	// Start based on active crypto provider
	switch cryptoProvider {
	case "binance":
		if app.binanceFeed != nil {
			app.binanceFeed.Start()
		}
		zapLog.Info("Crypto provider: binance (WebSocket)")
	case "both":
		if app.nobitexFeed != nil {
			app.nobitexFeed.Start()
		}
		if app.binanceFeed != nil {
			app.binanceFeed.Start()
		}
		zapLog.Info("Crypto provider: both (nobitex primary, binance fallback)")
	default: // "nobitex"
		if app.nobitexFeed != nil {
			app.nobitexFeed.Start()
			zapLog.Info("Crypto provider: nobitex (REST polling)",
				zap.Duration("poll_interval", cfg.NobitexPollInterval))
		} else {
			zapLog.Info("Nobitex crypto feed disabled",
				zap.Bool("enabled", cfg.NobitexEnabled),
				zap.Int("crypto_symbols", len(app.symbolRegistry.CryptoSymbols)))
		}
	}

	// Initialize dynamic symbol subscription manager
	if app.db != nil {
		app.dynamicSymbols = NewDynamicSymbolManager(app.db, zapLog)

		// Wire providers (MassiveProvider is inside providerManager)
		var massiveProv *MassiveProvider
		if app.providerManager != nil && app.providerManager.primary != nil {
			if mp, ok := app.providerManager.primary.(*MassiveProvider); ok {
				massiveProv = mp
			}
		}
		app.dynamicSymbols.SetProviders(massiveProv, app.nobitexFeed, app.binanceFeed)

		// Start periodic refresh in background
		app.wg.Add(1)
		infra.SafeGo(zapLog, "dynamic-symbols-refresh", func() {
			defer app.wg.Done()
			app.dynamicSymbols.Start(ctx)
		})

		// Start Kafka consumer for contests.v1 events (real-time symbol updates)
		contestsTopic := os.Getenv("CONTEST_STATE_TOPIC")
		if contestsTopic == "" {
			contestsTopic = "contests.v1"
		}
		app.wg.Add(1)
		infra.SafeGo(zapLog, "contest-events-consumer", func() {
			defer app.wg.Done()
			app.consumeContestEvents(ctx, contestsTopic)
		})

		zapLog.Info("Dynamic symbol manager started")
	} else {
		zapLog.Warn("Dynamic symbol manager disabled (no database connection)")
	}

	// Start 1Hz tick publisher
	app.wg.Add(1)
	go app.tickPublisher()

	// Send startup notification
	app.sendStartupNotification()

	// Set up HTTP server
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(obs.Middleware.Middleware)
	r.Use(obs.Middleware.Recovery)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", app.handleHealthz)
	r.Get("/readyz", app.handleReadyz)
	r.With(validation.InternalOnlyMiddleware).Get("/metrics", obs.MetricsHandler().ServeHTTP)

	// Control API for admin-bff to manage providers (API key protected)
	r.Route("/control", func(r chi.Router) {
		r.Use(app.requireControlAPIKey)
		r.Post("/switch-provider", app.handleSwitchProvider)
		r.Post("/reconnect", app.handleReconnect)
		r.Post("/crypto-provider", app.handleSwitchCryptoProvider)
		r.Get("/provider-config", app.handleGetProviderConfig)
	})
	r.Get("/status/subscriptions", app.handleSubscriptionStatus)
	r.Get("/status/symbols", app.handleDynamicSymbolStatus)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server
	infra.SafeGo(zapLog, "market-ingestor-http-server", func() {
		zapLog.Info("Starting market-ingestor", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zapLog.Fatal("Server error", zap.Error(err))
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
	zapLog.Info("Shutting down...")

	// Send shutdown notification
	app.sendShutdownNotification()

	// Cancel context to stop goroutines
	cancel()

	// Stop provider manager
	app.providerManager.Stop()

	// Stop crypto feeds
	if app.nobitexFeed != nil {
		app.nobitexFeed.Stop()
	}
	if app.binanceFeed != nil {
		app.binanceFeed.Stop()
	}

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		zapLog.Warn("Server forced to shutdown", zap.Error(err))
	}

	// Close tick publish channel to signal workers to drain and stop.
	// Must happen after providers stop (no new ticks) and before wg.Wait().
	if app.tickPublishCh != nil {
		close(app.tickPublishCh)
	}

	// Wait for goroutines (including tick publish workers)
	app.wg.Wait()

	// Stop candle aggregator (flushes remaining candles)
	if app.candleAggregator != nil {
		app.candleAggregator.Stop()
	}

	// Stop spread manager (stops Redis pub/sub listener)
	if app.spreadManager != nil {
		app.spreadManager.Stop()
	}

	// Close connections
	if app.kafka != nil {
		app.kafka.Close()
	}
	if app.redis != nil && !app.sharedRedis {
		app.redis.Close()
	}
	if app.db != nil && !app.sharedDB {
		app.db.Close()
	}

	// Shutdown notification service (drain pending notifications)
	if app.notifications != nil {
		notifShutdownCtx, notifShutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := app.notifications.Shutdown(notifShutdownCtx); err != nil {
			zapLog.Warn("Notification service shutdown error", zap.Error(err))
		}
		notifShutdownCancel()
	}

	zapLog.Info("Shutdown complete")
}

// maxPriceByCategory defines reasonable price ceilings per asset class.
var maxPriceByCategory = map[string]float64{
	"forex":     1_000,     // Most forex pairs < 200 (even USD/JPY ~150)
	"crypto":    1_000_000, // BTC can reach hundreds of thousands
	"commodity": 100_000,   // Gold ~2000/oz, but allow headroom
	"stock":     100_000,   // Highest stock prices ~thousands
}

const defaultMaxPrice = 1_000_000 // Fallback ceiling

// isValidPrice returns true if the price is a positive, finite number within a reasonable range.
func isValidPrice(p float64) bool {
	return p > 0 && !math.IsNaN(p) && !math.IsInf(p, 0) && p < 1e7
}

// isValidPriceForSymbol checks the price against a per-asset-class ceiling.
func (a *App) isValidPriceForSymbol(p float64, symbol string) bool {
	if !isValidPrice(p) {
		return false
	}
	category := a.getSymbolCategory(symbol)
	ceiling, ok := maxPriceByCategory[category]
	if !ok {
		ceiling = defaultMaxPrice
	}
	return p < ceiling
}

// handleTick is called when a tick is received from any provider.
// bid and ask may be zero when the provider only supplies a last price.
// volume is the trade volume from the provider; zero means unknown.
// source identifies the originating provider (e.g. "massive", "twelvedata", "nobitex").
func (a *App) handleTick(symbol string, price, bid, ask, volume float64, ts int64, source string) {
	// Reject tick entirely if the last price is invalid.
	if !a.isValidPriceForSymbol(price, symbol) {
		a.log().Warn("Invalid tick price rejected",
			zap.String("symbol", symbol), zap.Float64("price", price))
		return
	}

	if ts == 0 {
		ts = time.Now().Unix()
	}

	// Crypto provider priority: filter ticks based on active crypto provider.
	if a.getSymbolCategory(symbol) == "crypto" {
		activeCrypto, _ := a.activeCryptoProvider.Load().(string)
		switch activeCrypto {
		case "nobitex":
			if source == "binance" {
				return // drop binance ticks when nobitex is active
			}
			// Also drop non-nobitex ticks when nobitex is connected (legacy behavior)
			if source != "nobitex" && a.nobitexFeed != nil && a.nobitexFeed.IsConnected() {
				return
			}
		case "binance":
			if source == "nobitex" {
				return // drop nobitex ticks when binance is active
			}
			if source != "binance" && a.binanceFeed != nil && a.binanceFeed.IsConnected() {
				return
			}
		case "both":
			// In "both" mode, nobitex takes priority when connected
			if source == "binance" && a.nobitexFeed != nil && a.nobitexFeed.IsConnected() {
				return
			}
		default:
			// Default to nobitex priority (legacy behavior)
			if source != "nobitex" && a.nobitexFeed != nil && a.nobitexFeed.IsConnected() {
				return
			}
		}
	}

	// Sanitize bid/ask: reset to 0 if invalid (publishTick derives synthetic spread for zero values).
	if bid > 0 && !a.isValidPriceForSymbol(bid, symbol) {
		bid = 0
	}
	if ask > 0 && !a.isValidPriceForSymbol(ask, symbol) {
		ask = 0
	}
	// Fix inverted bid/ask.
	if bid > 0 && ask > 0 && bid > ask {
		bid, ask = ask, bid
	}

	tick := &TickData{
		Last:   price,
		Bid:    bid,
		Ask:    ask,
		Volume: volume,
		Ts:     ts,
	}

	// Update in-memory store
	a.ticksMu.Lock()
	a.ticks[symbol] = tick
	a.ticksMu.Unlock()

	// Mark symbol as dirty for next snapshot cycle
	a.dirtySymbols.Store(symbol, true)

	// Process tick for candle aggregation
	if a.candleAggregator != nil {
		vol := volume
		if vol <= 0 {
			vol = 1.0 // fallback to tick count when volume unknown
		}
		a.candleAggregator.ProcessTick(symbol, price, ts, vol)
	}

	// Queue tick for publishing via worker pool (non-blocking with backpressure).
	// If the queue is full, drop the tick — the next tick will supersede it.
	select {
	case a.tickPublishCh <- tickJob{symbol: symbol, tick: tick}:
	default:
		a.log().Warn("Tick publish queue full, dropping tick",
			zap.String("symbol", symbol))
	}
}

// deriveSyntheticBidAsk derives bid and ask prices from the last price using a spread.
func deriveSyntheticBidAsk(last float64) (bid, ask float64) {
	halfSpreadPct := float64(DefaultSpreadBps) / 2 / 10000.0
	bid = last * (1 - halfSpreadPct)
	ask = last * (1 + halfSpreadPct)
	return bid, ask
}

// tickPublishWorker is a long-lived goroutine that processes tick publish jobs from the worker pool.
func (a *App) tickPublishWorker() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("tickPublishWorker panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	for job := range a.tickPublishCh {
		a.publishTick(job.symbol, job.tick)
	}
}

// publishTick publishes a tick to Kafka and updates Redis immediately.
// The 1Hz publishSnapshot still runs for batch consistency, but each tick
// also writes to Redis so that any Redis-based price lookups see fresh data.
func (a *App) publishTick(symbol string, tick *TickData) {
	// Resolve bid/ask once: use real values from provider when available,
	// otherwise derive synthetic spread from the last price.
	bid, ask := tick.Bid, tick.Ask
	if bid <= 0 || ask <= 0 {
		// Provider gave only a last price (e.g. TwelveData fallback) — derive synthetic spread
		if a.spreadManager != nil {
			bid, ask = a.spreadManager.DeriveSyntheticBidAsk(symbol, tick.Last)
		} else {
			bid, ask = deriveSyntheticBidAsk(tick.Last)
		}
	}

	// Redis is updated by the 1Hz publishSnapshot using Pipeline batching.
	// Per-tick Redis writes are avoided to prevent excessive load
	// (e.g. 50 symbols × 10 ticks/sec = 500 HSet/sec).
	// The symbol is already marked dirty before this function is called,
	// so the next 1Hz snapshot will pick up the latest price.

	// Publish to Kafka with symbol as partition key
	// This ensures per-symbol ordering and allows trading-engines to subscribe
	// to specific partitions for their assigned symbols.
	// See docs/kafka-partitioning.md for details.
	if a.kafka != nil {
		// Create TickSnapshot in the format expected by trade-bff and trading-engine
		tickSnapshot := contracts.TickSnapshot{
			Ts: tick.Ts,
			Symbols: []contracts.SymbolTick{
				{
					Symbol:    symbol,
					Bid:       bid,
					Ask:       ask,
					Last:      tick.Last,
					Timestamp: tick.Ts * 1000, // Convert Unix seconds to milliseconds
					Volume:    tick.Volume,
				},
			},
		}

		kafkaJSON, err := json.Marshal(tickSnapshot)
		if err != nil {
			a.log().Error("Failed to marshal TickSnapshot", zap.Error(err))
			return
		}

		record := &kgo.Record{
			Topic: a.config.TicksTopic,
			Key:   []byte(symbol), // Partition key for per-symbol ordering
			Value: kafkaJSON,
		}

		a.kafka.Produce(context.Background(), record, func(r *kgo.Record, err error) {
			if err != nil {
				a.log().Error("Kafka publish failed", zap.String("symbol", symbol), zap.Error(err))
			}
		})
	}
}

// tickPublisher runs at 1Hz to publish aggregated tick snapshots.
func (a *App) tickPublisher() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("tickPublisher panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.publishSnapshot()
		}
	}
}

// publishSnapshot publishes a 1Hz snapshot of all symbols.
func (a *App) publishSnapshot() {
	// Collect dirty symbols and clear flags
	dirtySymbols := make([]string, 0, 64)
	a.dirtySymbols.Range(func(key, _ any) bool {
		dirtySymbols = append(dirtySymbols, key.(string))
		a.dirtySymbols.Delete(key)
		return true
	})

	if len(dirtySymbols) == 0 {
		return
	}

	// Read only dirty ticks and resolve bid/ask
	a.ticksMu.RLock()
	ticks := make(map[string]*TickData, len(dirtySymbols))
	for _, symbol := range dirtySymbols {
		v, ok := a.ticks[symbol]
		if !ok {
			continue
		}
		bid, ask := v.Bid, v.Ask
		if bid <= 0 || ask <= 0 {
			if a.spreadManager != nil {
				bid, ask = a.spreadManager.DeriveSyntheticBidAsk(symbol, v.Last)
			} else {
				bid, ask = deriveSyntheticBidAsk(v.Last)
			}
		}
		ticks[symbol] = &TickData{Last: v.Last, Bid: bid, Ask: ask, Volume: v.Volume, Ts: v.Ts}
	}
	a.ticksMu.RUnlock()

	if len(ticks) == 0 {
		return
	}

	// Update Redis with dirty ticks only
	if a.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		pipe := a.redis.Pipeline()
		for symbol, tick := range ticks {
			tickJSON, _ := json.Marshal(tick)
			pipe.HSet(ctx, "prices:latest", symbol, string(tickJSON))
		}
		_, err := pipe.Exec(ctx)
		cancel()
		if err != nil {
			a.log().Error("Redis batch update failed", zap.Error(err))
		}
	}
}

// handleHealthz returns basic health status.
func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleReadyz returns readiness status (WebSocket connected).
func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")

	response := map[string]interface{}{
		"status":    "ready",
		"service":   "market-ingestor",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	httpStatus := http.StatusOK

	// Check WebSocket provider (critical - main purpose of this service)
	ready := a.providerManager.IsReady()
	currentProvider := a.providerManager.CurrentProvider()
	usingFallback := a.providerManager.UsingFallback()

	providerInfo := map[string]interface{}{
		"name":           string(currentProvider),
		"using_fallback": usingFallback,
	}

	if !ready {
		response["status"] = "unavailable"
		providerInfo["status"] = "disconnected"
		response["message"] = "websocket provider not connected"
		httpStatus = http.StatusServiceUnavailable
	} else {
		providerInfo["status"] = "connected"
	}
	response["provider"] = providerInfo

	// Check Redis connectivity (critical - used for tick caching)
	if a.redis != nil {
		if err := a.redis.Ping(ctx).Err(); err != nil {
			response["redis"] = "unavailable"
			if response["status"] == "ready" {
				response["status"] = "degraded"
				response["message"] = "redis unavailable"
			}
		} else {
			response["redis"] = "healthy"
		}
	} else {
		response["redis"] = "not_configured"
	}

	// Check Kafka producer (critical - needed to publish ticks)
	if a.kafka != nil {
		response["kafka"] = "healthy"
	} else {
		response["kafka"] = "unavailable"
		if response["status"] == "ready" {
			response["status"] = "degraded"
			if response["message"] == nil {
				response["message"] = "kafka producer not initialized"
			}
		}
	}

	// Add tick stats
	a.ticksMu.RLock()
	response["tracked_symbols"] = len(a.ticks)
	a.ticksMu.RUnlock()

	// Add candle aggregator stats
	if a.candleAggregator != nil {
		activeCandles := a.candleAggregator.GetActiveCandles()
		totalActive := 0
		for _, resolutions := range activeCandles {
			totalActive += len(resolutions)
		}
		response["candles"] = map[string]interface{}{
			"enabled":        true,
			"active_candles": totalActive,
			"pending_flush":  a.candleAggregator.GetPendingFlushCount(),
		}
	} else {
		response["candles"] = map[string]interface{}{
			"enabled": false,
		}
	}

	// Add database status
	if a.db != nil {
		if err := a.db.PingContext(ctx); err != nil {
			response["database"] = "unavailable"
		} else {
			response["database"] = "healthy"
		}
	} else {
		response["database"] = "not_configured"
	}

	// Nobitex crypto feed status
	if a.nobitexFeed != nil {
		nobitexConnected := a.nobitexFeed.IsConnected()
		response["nobitex"] = map[string]interface{}{
			"connected":  nobitexConnected,
			"last_tick":  a.nobitexFeed.LastTickTime(),
			"tick_count": a.nobitexFeed.Stats()["tick_count"],
		}
		if !nobitexConnected && ready {
			response["status"] = "degraded"
			response["message"] = "nobitex crypto feed disconnected"
		}
	}

	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(response)
}

// SwitchToProvider manually switches to the specified provider.
// Returns an error if the provider name is unknown or the same as the current one.
// The swap is performed before closing the old provider to prevent concurrent
// goroutines (e.g. connectAndProcess) from accessing a closed provider.
func (pm *ProviderManager) SwitchToProvider(providerName ProviderType) error {
	pm.mu.Lock()

	current := pm.current.Name()
	if current == providerName {
		pm.mu.Unlock()
		return fmt.Errorf("already using provider %s", providerName)
	}

	// Save old provider for deferred close
	old := pm.current

	switch providerName {
	case ProviderMassive:
		pm.current = NewMassiveProvider(pm.config.MassiveAPIKeys, pm.logger.Named("massive"))
		pm.usingFallback.Store(pm.primary.Name() != ProviderMassive)
	case ProviderTwelveData:
		pm.current = NewTwelveDataProvider(pm.config.TwelveDataAPIKeys, pm.logger.Named("twelvedata"))
		pm.usingFallback.Store(pm.primary.Name() != ProviderTwelveData)
	case ProviderFinnhub:
		pm.current = NewFinnhubProvider(pm.config.FinnhubAPIKeys, pm.logger.Named("finnhub"))
		pm.usingFallback.Store(pm.primary.Name() != ProviderFinnhub)
	default:
		pm.mu.Unlock()
		return fmt.Errorf("unknown provider: %s", providerName)
	}

	pm.failureCount = 0
	pm.failureStart = time.Time{}
	pm.ready.Store(false) // will be set to true when connection is established
	pm.mu.Unlock()

	// Close old provider AFTER swap, outside lock
	if old != nil {
		old.Close()
	}

	return nil
}

// ForceReconnect closes the current provider connection to trigger a reconnection cycle.
// Close is performed outside the lock to reduce contention.
func (pm *ProviderManager) ForceReconnect() {
	pm.mu.Lock()
	toClose := pm.current
	pm.ready.Store(false)
	pm.mu.Unlock()

	if toClose != nil {
		toClose.Close()
	}
}

// requireControlAPIKey is middleware that validates the X-API-Key header for control endpoints.
// Fails closed: if no API key is configured, all requests are rejected.
func (a *App) requireControlAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.config.ControlAPIKey == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "control API key not configured"})
			return
		}
		apiKey := r.Header.Get("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(apiKey), []byte(a.config.ControlAPIKey)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid or missing API key"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleSwitchProvider handles POST /control/switch-provider?provider=<name>
func (a *App) handleSwitchProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing provider query parameter"})
		return
	}

	providerType := ProviderType(provider)
	if providerType != ProviderMassive && providerType != ProviderTwelveData && providerType != ProviderFinnhub {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid provider, must be 'massive', 'twelvedata', or 'finnhub'"})
		return
	}

	if err := a.providerManager.SwitchToProvider(providerType); err != nil {
		a.log().Error("Failed to switch provider",
			zap.String("provider", provider),
			zap.Error(err))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "provider switch failed"})
		return
	}

	a.log().Info("Provider switched via control API",
		zap.String("new_provider", provider))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"provider": provider,
		"message":  "Provider switched, reconnecting...",
	})
}

// handleReconnect handles POST /control/reconnect
func (a *App) handleReconnect(w http.ResponseWriter, r *http.Request) {
	a.providerManager.ForceReconnect()

	a.log().Info("Force reconnect triggered via control API")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Reconnection triggered",
	})
}

// handleSwitchCryptoProvider handles POST /control/crypto-provider?provider=<name>
func (a *App) handleSwitchCryptoProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing provider query parameter"})
		return
	}

	switch provider {
	case "nobitex":
		if a.binanceFeed != nil {
			a.binanceFeed.Stop()
		}
		if a.nobitexFeed != nil {
			a.nobitexFeed.Start()
		}
	case "binance":
		if a.nobitexFeed != nil {
			a.nobitexFeed.Stop()
		}
		if a.binanceFeed != nil {
			a.binanceFeed.Start()
		}
	case "both":
		if a.nobitexFeed != nil {
			a.nobitexFeed.Start()
		}
		if a.binanceFeed != nil {
			a.binanceFeed.Start()
		}
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid provider, must be 'nobitex', 'binance', or 'both'"})
		return
	}

	a.activeCryptoProvider.Store(provider)

	// Persist to DB
	if a.db != nil {
		_, err := a.db.Exec("UPDATE provider_config SET active_provider=$1, updated_at=NOW() WHERE asset_class='crypto'", provider)
		if err != nil {
			a.log().Warn("Failed to persist crypto provider to DB", zap.Error(err))
		}
	}

	a.log().Info("Crypto provider switched via control API",
		zap.String("new_provider", provider))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "ok",
		"provider": provider,
		"message":  "Crypto provider switched to " + provider,
	})
}

// handleGetProviderConfig handles GET /control/provider-config
func (a *App) handleGetProviderConfig(w http.ResponseWriter, r *http.Request) {
	activeCrypto, _ := a.activeCryptoProvider.Load().(string)
	if activeCrypto == "" {
		activeCrypto = "nobitex"
	}

	nobitexStats := map[string]interface{}{"enabled": false}
	if a.nobitexFeed != nil {
		nobitexStats = a.nobitexFeed.Stats()
	}

	binanceStats := map[string]interface{}{"enabled": false}
	if a.binanceFeed != nil {
		binanceStats = a.binanceFeed.Stats()
	}

	response := map[string]interface{}{
		"crypto": map[string]interface{}{
			"active":    activeCrypto,
			"available": []string{"nobitex", "binance", "both"},
			"nobitex":   nobitexStats,
			"binance":   binanceStats,
		},
		"forex": map[string]interface{}{
			"active":         string(a.providerManager.CurrentProvider()),
			"available":      []string{"massive", "twelvedata", "finnhub"},
			"using_fallback": a.providerManager.UsingFallback(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleSubscriptionStatus handles GET /status/subscriptions
func (a *App) handleSubscriptionStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	currentProvider := a.providerManager.CurrentProvider()
	usingFallback := a.providerManager.UsingFallback()
	ready := a.providerManager.IsReady()

	// Gather available providers
	availableProviders := []string{"massive", "twelvedata", "nobitex", "binance"}

	// Gather per-symbol status
	a.ticksMu.RLock()
	now := time.Now().Unix()
	symbolsTotal := len(a.config.Symbols)
	symbolsReceiving := 0
	symbolsStale := 0

	type symbolStatus struct {
		Symbol   string  `json:"symbol"`
		Bid      float64 `json:"bid"`
		Ask      float64 `json:"ask"`
		Last     float64 `json:"last"`
		Ts       int64   `json:"ts"`
		AgeMs    int64   `json:"age_ms"`
		Provider string  `json:"provider"`
		Status   string  `json:"status"`
		Category string  `json:"category"`
	}

	symbols := make([]symbolStatus, 0, len(a.config.Symbols))
	for _, sym := range a.config.Symbols {
		tick, exists := a.ticks[sym]
		provider := string(currentProvider)
		if a.getSymbolCategory(sym) == "crypto" {
			activeCrypto, _ := a.activeCryptoProvider.Load().(string)
			if activeCrypto != "" {
				provider = activeCrypto
			}
		}
		ss := symbolStatus{
			Symbol:   sym,
			Provider: provider,
			Category: a.getSymbolCategory(sym),
		}

		if exists && tick != nil {
			ss.Bid = tick.Bid
			ss.Ask = tick.Ask
			ss.Last = tick.Last
			ss.Ts = tick.Ts
			ss.AgeMs = (now - tick.Ts) * 1000

			if ss.AgeMs < 30000 {
				ss.Status = "fresh"
				symbolsReceiving++
			} else if ss.AgeMs < 120000 {
				ss.Status = "warning"
				symbolsReceiving++
			} else {
				ss.Status = "stale"
				symbolsStale++
			}
		} else {
			ss.Status = "no_data"
			symbolsStale++
		}
		symbols = append(symbols, ss)
	}
	a.ticksMu.RUnlock()

	// Determine provider-specific status
	massiveStatus := map[string]string{"forex_ws": "disconnected", "crypto_ws": "disconnected"}
	twelvedataStatus := map[string]string{"ws": "disconnected"}
	nobitexStatus := map[string]interface{}{"enabled": false}

	if currentProvider == ProviderMassive && ready {
		massiveStatus["forex_ws"] = "connected"
		massiveStatus["crypto_ws"] = "connected"
	} else if currentProvider == ProviderTwelveData && ready {
		twelvedataStatus["ws"] = "connected"
	}

	// Nobitex feed status
	if a.nobitexFeed != nil {
		nobitexStatus = a.nobitexFeed.Stats()
	}

	binanceStatus := map[string]interface{}{"enabled": false}
	if a.binanceFeed != nil {
		binanceStatus = a.binanceFeed.Stats()
	}

	activeCrypto, _ := a.activeCryptoProvider.Load().(string)
	if activeCrypto == "" {
		activeCrypto = "nobitex"
	}

	response := map[string]interface{}{
		"active_provider":        string(currentProvider),
		"active_crypto_provider": activeCrypto,
		"available_providers":    availableProviders,
		"using_fallback":         usingFallback,
		"massive_status":         massiveStatus,
		"twelvedata_status":      twelvedataStatus,
		"nobitex_status":         nobitexStatus,
		"binance_status":         binanceStatus,
		"symbols_total":          symbolsTotal,
		"symbols_receiving":      symbolsReceiving,
		"symbols_stale":          symbolsStale,
		"ready":                  ready,
		"symbols":                symbols,
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// getSymbolCategory returns the category (forex, crypto, commodity) for a symbol.
func (a *App) getSymbolCategory(symbol string) string {
	if a.symbolRegistry != nil {
		if at, ok := a.symbolRegistry.AssetTypes[symbol]; ok {
			return at
		}
	}
	// Fallback classification
	if strings.Contains(symbol, "BTC") || strings.Contains(symbol, "ETH") ||
		strings.Contains(symbol, "LTC") || strings.Contains(symbol, "XRP") ||
		strings.Contains(symbol, "SOL") || strings.Contains(symbol, "DOGE") ||
		strings.Contains(symbol, "ADA") || strings.Contains(symbol, "DOT") {
		return "crypto"
	}
	if strings.Contains(symbol, "XAU") || strings.Contains(symbol, "XAG") ||
		strings.Contains(symbol, "OIL") || strings.Contains(symbol, "GAS") {
		return "commodity"
	}
	return "forex"
}

// backoff implements exponential backoff with jitter.
type backoff struct {
	initial time.Duration
	max     time.Duration
	current time.Duration
}

func newBackoff(initial, max time.Duration) *backoff {
	return &backoff{
		initial: initial,
		max:     max,
		current: initial,
	}
}

func (b *backoff) next() time.Duration {
	delay := b.current

	// Add jitter (0-25% of current delay)
	jitter := time.Duration(rand.Float64() * 0.25 * float64(b.current))
	delay += jitter

	// Exponential increase
	b.current *= 2
	if b.current > b.max {
		b.current = b.max
	}

	return delay
}

func (b *backoff) reset() {
	b.current = b.initial
}
