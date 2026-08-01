package server

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// FinnhubProvider implements MarketProvider for Finnhub.io WebSocket API.
type FinnhubProvider struct {
	keyRotator *KeyRotator
	conn       *websocket.Conn
	mu         sync.Mutex
	connected  bool
	logger     *zap.Logger
	msgCh      chan []byte
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewFinnhubProvider creates a new Finnhub provider.
func NewFinnhubProvider(keys []string, logger *zap.Logger) *FinnhubProvider {
	return &FinnhubProvider{
		keyRotator: NewKeyRotator(keys, logger),
		logger:     logger,
		msgCh:      make(chan []byte, 256),
		stopCh:     make(chan struct{}),
	}
}

func (p *FinnhubProvider) Name() ProviderType {
	return ProviderFinnhub
}

func (p *FinnhubProvider) Connect(ctx context.Context) error {
	apiKey := p.keyRotator.Current()
	if apiKey == "" {
		return fmt.Errorf("FINNHUB_API_KEYS not configured")
	}

	wsURL := fmt.Sprintf("wss://ws.finnhub.io?token=%s", apiKey)
	p.logger.Info("Connecting",
		zap.Int("key_index", p.keyRotator.CurrentIndex()+1),
		zap.Int("key_count", p.keyRotator.Count()))

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("Finnhub dial failed: %w", err)
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

func (p *FinnhubProvider) Subscribe(symbols []string) error {
	p.mu.Lock()
	conn := p.conn
	p.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	for _, sym := range symbols {
		msg := map[string]string{"type": "subscribe", "symbol": sym}
		if err := conn.WriteJSON(msg); err != nil {
			return fmt.Errorf("Finnhub subscribe failed for %s: %w", sym, err)
		}
	}

	p.logger.Info("Subscribed", zap.Int("symbol_count", len(symbols)))
	return nil
}

func (p *FinnhubProvider) readLoop() {
	defer p.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Finnhub readLoop panicked",
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
				wsSentinelDropsTotal.WithLabelValues("finnhub").Inc()
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

func (p *FinnhubProvider) ReadMessage() ([]byte, error) {
	msg, ok := <-p.msgCh
	if !ok {
		return nil, fmt.Errorf("message channel closed")
	}
	if msg == nil {
		return nil, fmt.Errorf("connection lost")
	}
	return msg, nil
}

func (p *FinnhubProvider) Close() error {
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

func (p *FinnhubProvider) IsConnected() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.connected
}

// RotateKey rotates to the next API key.
func (p *FinnhubProvider) RotateKey() {
	p.keyRotator.Rotate()
}

// KeyCount returns the number of available API keys.
func (p *FinnhubProvider) KeyCount() int {
	return p.keyRotator.Count()
}

// FinnhubMessage represents a Finnhub WebSocket message.
type FinnhubMessage struct {
	Data []FinnhubTrade `json:"data"`
	Type string         `json:"type"`
}

// FinnhubTrade represents a single trade in a Finnhub message.
type FinnhubTrade struct {
	Price     float64 `json:"p"`
	Symbol    string  `json:"s"`
	Timestamp int64   `json:"t"`
	Volume    float64 `json:"v"`
}

// ParseFinnhubMessage parses a Finnhub WebSocket message.
// Returns the trades, the message type ("trade" or "ping"), and any error.
func ParseFinnhubMessage(data []byte) ([]FinnhubTrade, string, error) {
	var msg FinnhubMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, "", fmt.Errorf("parse error: %w", err)
	}
	if msg.Type == "ping" {
		return nil, "ping", nil
	}
	return msg.Data, msg.Type, nil
}
