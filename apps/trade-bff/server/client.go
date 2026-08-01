package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// ====================
// Encoding Types
// ====================

// EncodingType represents the message encoding format for WebSocket messages
type EncodingType int

const (
	// EncodingJSON uses JSON encoding (default, backward compatible)
	EncodingJSON EncodingType = iota
	// EncodingMsgPack uses MessagePack binary encoding for tick_batch and state_delta
	EncodingMsgPack
)

// ====================
// WebSocket Client
// ====================

// MessageEnvelope wraps a message with its encoding type for the send channel
type MessageEnvelope struct {
	Data     []byte
	IsBinary bool   // true for MessagePack (BinaryMessage), false for JSON (TextMessage)
	MsgType  string // Message type for metrics tracking ("tick_batch", "state_delta", "other")
}

// Client represents a WebSocket client with backpressure support
type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	userID    string
	contestID string

	// App reference for order handling
	app *App

	// Shard routing information
	shardID      string // The shard this client is routed to
	shardAddress string // The address of the trading-engine shard

	// Bounded send queue (size 1 for latest-message-wins backpressure on market data)
	send chan MessageEnvelope

	// Critical message queue (buffered, never-drop semantics for transactional messages)
	criticalSend chan MessageEnvelope

	// Metrics
	metrics *Metrics

	// Compression settings
	compressionEnabled           bool // Whether compression was negotiated for this connection
	minMessageSizeForCompression int  // Minimum message size to apply compression

	// Encoding settings
	encoding EncodingType // Message encoding format (JSON or MessagePack)

	// Participation cache — set once at connection time, avoids DB hit per order
	isParticipant bool

	// Close synchronization
	closeOnce sync.Once
	done      chan struct{}
}

// NewClient creates a new Client
func NewClient(hub *Hub, conn *websocket.Conn, userID, contestID string, metrics *Metrics, compressionEnabled bool, minMsgSize int, app *App, encoding EncodingType) *Client {
	return &Client{
		hub:                          hub,
		conn:                         conn,
		userID:                       userID,
		contestID:                    contestID,
		app:                          app,
		send:                         make(chan MessageEnvelope, 1),  // Bounded queue for market data (drop OK)
		criticalSend:                 make(chan MessageEnvelope, 16), // Buffered queue for critical messages (never drop)
		metrics:                      metrics,
		compressionEnabled:           compressionEnabled,
		minMessageSizeForCompression: minMsgSize,
		encoding:                     encoding,
		done:                         make(chan struct{}),
	}
}

// SendMessage sends a JSON message with backpressure (overwrite strategy)
// This is used for non-critical messages that are always sent as JSON
func (c *Client) SendMessage(msg []byte) {
	c.sendEnvelope(MessageEnvelope{Data: msg, IsBinary: false, MsgType: "other"})
}

// SendCriticalMessage sends a critical message (fills, order_acks, positions, etc.)
// Uses the dedicated critical channel with timeout semantics instead of dropping.
// If the critical queue is full for 5 seconds, the client is considered dead and closed.
func (c *Client) SendCriticalMessage(msg []byte) {
	env := MessageEnvelope{Data: msg, IsBinary: false, MsgType: "critical"}

	// Track bytes metrics
	c.metrics.wsTotalBytesSent.Add(int64(len(env.Data)))
	c.metrics.wsBytesSentJsonOther.Add(int64(len(env.Data)))

	if c.compressionEnabled && len(env.Data) >= c.minMessageSizeForCompression {
		c.metrics.wsCompressedBytesSent.Add(int64(len(env.Data) * 40 / 100))
	} else {
		c.metrics.wsCompressedBytesSent.Add(int64(len(env.Data)))
		if len(env.Data) < c.minMessageSizeForCompression {
			c.metrics.wsCompressionSkipped.Add(1)
		}
	}

	select {
	case c.criticalSend <- env:
		c.metrics.wsCriticalMessagesSent.Add(1)
	case <-c.done:
		// Client is closing, discard
	case <-time.After(5 * time.Second):
		// Critical queue full for too long — client is likely dead
		c.metrics.wsCriticalQueueFull.Add(1)
		log.Printf("Critical send timeout user=%s, closing connection", c.userID)
		c.Close()
	}
}

// SendTickBatch sends a tick_batch message using the client's preferred encoding
// For MessagePack clients, use msgpackData; for JSON clients, use jsonData
func (c *Client) SendTickBatch(jsonData, msgpackData []byte) {
	if c.encoding == EncodingMsgPack && len(msgpackData) > 0 {
		c.sendEnvelope(MessageEnvelope{Data: msgpackData, IsBinary: true, MsgType: "tick_batch"})
	} else {
		c.sendEnvelope(MessageEnvelope{Data: jsonData, IsBinary: false, MsgType: "tick_batch"})
	}
}

// SendStateDelta sends a state_delta message using the client's preferred encoding
func (c *Client) SendStateDelta(jsonData, msgpackData []byte) {
	if c.encoding == EncodingMsgPack && len(msgpackData) > 0 {
		c.sendEnvelope(MessageEnvelope{Data: msgpackData, IsBinary: true, MsgType: "state_delta"})
	} else {
		c.sendEnvelope(MessageEnvelope{Data: jsonData, IsBinary: false, MsgType: "state_delta"})
	}
}

// sendEnvelope handles the actual message queuing with backpressure
func (c *Client) sendEnvelope(env MessageEnvelope) {
	// Track total bytes (uncompressed)
	c.metrics.wsTotalBytesSent.Add(int64(len(env.Data)))

	// Track bytes by encoding and message type
	switch env.MsgType {
	case "tick_batch":
		if env.IsBinary {
			c.metrics.wsBytesSentMsgPackTickBatch.Add(int64(len(env.Data)))
		} else {
			c.metrics.wsBytesSentJsonTickBatch.Add(int64(len(env.Data)))
		}
	case "state_delta":
		if env.IsBinary {
			c.metrics.wsBytesSentMsgPackStateDelta.Add(int64(len(env.Data)))
		} else {
			c.metrics.wsBytesSentJsonStateDelta.Add(int64(len(env.Data)))
		}
	default:
		c.metrics.wsBytesSentJsonOther.Add(int64(len(env.Data)))
	}

	// Estimate compressed size for metrics (actual compression happens in writePump)
	// For typical JSON messages, deflate achieves ~60-70% compression
	if c.compressionEnabled && len(env.Data) >= c.minMessageSizeForCompression {
		// Estimate compressed bytes (conservative estimate: 40% of original)
		c.metrics.wsCompressedBytesSent.Add(int64(len(env.Data) * 40 / 100))
	} else {
		// No compression applied
		c.metrics.wsCompressedBytesSent.Add(int64(len(env.Data)))
		if len(env.Data) < c.minMessageSizeForCompression {
			c.metrics.wsCompressionSkipped.Add(1)
		}
	}

	select {
	case c.send <- env:
		// Message queued successfully
	default:
		// Queue full - drain old message and queue new one
		select {
		case <-c.send:
			c.metrics.wsDroppedMessagesTotal.Add(1)
		default:
		}
		// Try to queue again
		select {
		case c.send <- env:
		default:
			// Should not happen with queue size 1, but handle gracefully
			c.metrics.wsDroppedMessagesTotal.Add(1)
		}
	}
}

// Close closes the client connection
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.conn != nil {
			c.conn.Close()
		}
	})
}

// readPump handles incoming messages from the client
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.Close()
	}()

	const (
		pongWait   = 60 * time.Second
		maxMsgSize = 4096
	)

	c.conn.SetReadLimit(maxMsgSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("WebSocket read error user=%s: %v", c.userID, err)
			}
			return
		}
		// Handle incoming messages (e.g., order placement)
		c.handleMessage(message)
	}
}

// handleMessage processes incoming client messages
func (c *Client) handleMessage(message []byte) {
	// Parse message type
	var baseMsg struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		c.sendOrderReject("", "invalid_json", "Failed to parse message")
		return
	}

	switch baseMsg.Type {
	case MsgTypeOrderRequest:
		c.handleOrderRequest(message)
	default:
		// Log unknown message types for debugging
		if c.app != nil && c.app.obs != nil {
			c.app.obs.Logger.Logger.Debug("Unknown WebSocket message type",
				zap.String("user_id", c.userID),
				zap.String("contest_id", c.contestID),
				zap.String("type", baseMsg.Type))
		}
	}
}

// handleOrderRequest processes an incoming order request via WebSocket
func (c *Client) handleOrderRequest(message []byte) {
	// Check cached participation (set at connection time, avoids DB round-trip per order)
	if !c.isParticipant {
		c.sendOrderReject("", "not_participant", tradeMsg.NotParticipant)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var req WSOrderRequest
	if err := json.Unmarshal(message, &req); err != nil {
		c.sendOrderReject("", "invalid_request", "Failed to parse order request")
		return
	}

	// Generate request ID if not provided
	if req.RequestID == "" {
		req.RequestID = uuid.New().String()
	}

	// Validate and sanitize fields using validation package
	v := validation.New()
	req.Symbol = v.Symbol("symbol", req.Symbol)
	v.Quantity("qty", req.Qty, validation.DefaultQuantityConstraints())

	// Validate optional price fields
	if req.LimitPrice != nil {
		v.PricePtr("limit_price", req.LimitPrice, validation.DefaultPriceConstraints())
	}
	if req.StopPrice != nil {
		v.PricePtr("stop_price", req.StopPrice, validation.DefaultPriceConstraints())
	}
	if req.TakeProfit != nil {
		v.PricePtr("take_profit", req.TakeProfit, validation.DefaultPriceConstraints())
	}
	if req.StopLoss != nil {
		v.PricePtr("stop_loss", req.StopLoss, validation.DefaultPriceConstraints())
	}

	if v.HasErrors() {
		c.sendOrderReject(req.RequestID, "validation_error", v.Errors()[0].Message)
		return
	}

	// Validate order side
	if req.Side != contracts.OrderSideBuy && req.Side != contracts.OrderSideSell {
		c.sendOrderReject(req.RequestID, "invalid_side", "side must be BUY or SELL")
		return
	}

	// Validate order type
	validTypes := map[contracts.OrderType]bool{
		contracts.OrderTypeMarket:    true,
		contracts.OrderTypeBuyLimit:  true,
		contracts.OrderTypeSellLimit: true,
		contracts.OrderTypeBuyStop:   true,
		contracts.OrderTypeSellStop:  true,
	}
	if !validTypes[req.OrderType] {
		c.sendOrderReject(req.RequestID, "invalid_order_type", "order_type must be MARKET, BUY_LIMIT, SELL_LIMIT, BUY_STOP, or SELL_STOP")
		return
	}

	// Validate price fields for pending orders
	if req.OrderType.IsPending() {
		switch req.OrderType {
		case contracts.OrderTypeBuyLimit, contracts.OrderTypeSellLimit:
			if req.LimitPrice == nil || *req.LimitPrice <= 0 {
				c.sendOrderReject(req.RequestID, "missing_limit_price", "limit_price is required for limit orders")
				return
			}
		case contracts.OrderTypeBuyStop, contracts.OrderTypeSellStop:
			if req.StopPrice == nil || *req.StopPrice <= 0 {
				c.sendOrderReject(req.RequestID, "missing_stop_price", "stop_price is required for stop orders")
				return
			}
		}
	}

	// Check contest is running (the contest status was already validated on WebSocket connection,
	// but we verify again for orders to ensure the contest hasn't ended mid-session)
	info, err := c.app.getContestStatus(ctx, c.contestID)
	if err != nil {
		c.app.log().Error("Failed to get contest status for order",
			zap.Error(err),
			zap.String("contest_id", c.contestID))
		c.sendOrderReject(req.RequestID, "internal_error", "Failed to verify contest status")
		return
	}
	if info.Status != "running" {
		c.sendOrderReject(req.RequestID, "contest_not_running", fmt.Sprintf("Contest is not running (status: %s)", info.Status))
		return
	}

	// Generate order ID
	orderID := uuid.New().String()

	// Build OrderRequest for Kafka
	orderReq := contracts.OrderRequest{
		OrderID:    orderID,
		UserID:     c.userID,
		ContestID:  c.contestID,
		Symbol:     req.Symbol,
		Side:       req.Side,
		Type:       req.OrderType,
		Qty:        req.Qty,
		LimitPrice: req.LimitPrice,
		StopPrice:  req.StopPrice,
		TakeProfit: req.TakeProfit,
		StopLoss:   req.StopLoss,
		ClientTs:   time.Now().UnixMilli(),
	}

	// Serialize to JSON
	data, err := json.Marshal(orderReq)
	if err != nil {
		c.app.log().Error("Failed to marshal order request", zap.Error(err))
		c.sendOrderReject(req.RequestID, "internal_error", "Failed to process order")
		return
	}

	// Publish to Kafka with contest_id as partition key
	if c.app.ordersKafka != nil {
		record := &kgo.Record{
			Topic: c.app.config.OrdersTopic,
			Key:   []byte(c.contestID), // Partition key for contest-local ordering
			Value: data,
		}
		results := c.app.ordersKafka.ProduceSync(ctx, record)
		if err := results.FirstErr(); err != nil {
			c.app.log().Error("Failed to publish order to Kafka",
				zap.Error(err),
				zap.String("order_id", orderID),
				zap.String("user_id", c.userID))
			c.sendOrderReject(req.RequestID, "kafka_error", "Failed to submit order")
			return
		}
		c.app.log().Info("Order published to Kafka via WebSocket",
			zap.String("order_id", orderID),
			zap.String("user_id", c.userID),
			zap.String("contest_id", c.contestID),
			zap.String("symbol", req.Symbol),
			zap.String("side", string(req.Side)),
			zap.String("type", string(req.OrderType)))
	} else {
		c.app.log().Warn("Kafka producer not available, order not published",
			zap.String("order_id", orderID))
		c.sendOrderReject(req.RequestID, "service_unavailable", "Order service temporarily unavailable")
		return
	}

	// Send acknowledgment
	c.sendOrderAck(req.RequestID, orderID)
}

// sendOrderAck sends an order acknowledgment to the client
func (c *Client) sendOrderAck(requestID, orderID string) {
	ack := WSOrderAck{
		Type:      MsgTypeOrderAck,
		RequestID: requestID,
		OrderID:   orderID,
		Status:    "accepted",
	}
	data, _ := json.Marshal(ack)
	c.SendCriticalMessage(data)
}

// sendOrderReject sends an order rejection to the client
func (c *Client) sendOrderReject(requestID, code, message string) {
	c.sendOrderRejectWithRateLimit(requestID, code, message, nil)
}

// sendOrderRejectWithRateLimit sends an order rejection with optional rate limit metadata
func (c *Client) sendOrderRejectWithRateLimit(requestID, code, message string, rateLimit *WSRateLimitInfo) {
	reject := WSOrderReject{
		Type:      MsgTypeOrderReject,
		RequestID: requestID,
		Code:      code,
		Message:   message,
		RateLimit: rateLimit,
	}
	data, _ := json.Marshal(reject)
	c.SendCriticalMessage(data)
}

// writeEnvelope writes a single message envelope to the WebSocket connection.
// Returns an error if the write fails.
func (c *Client) writeEnvelope(envelope MessageEnvelope, writeWait time.Duration) error {
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))

	// Conditionally enable/disable compression based on message size
	if c.compressionEnabled {
		c.conn.EnableWriteCompression(len(envelope.Data) >= c.minMessageSizeForCompression)
	}

	// Choose message type based on encoding: Binary for MessagePack, Text for JSON
	msgType := websocket.TextMessage
	if envelope.IsBinary {
		msgType = websocket.BinaryMessage
	}

	return c.conn.WriteMessage(msgType, envelope.Data)
}

// writePump handles outgoing messages to the client.
// Critical messages (fills, order_acks, positions) are prioritized over market data.
func (c *Client) writePump() {
	const (
		writeWait  = 10 * time.Second
		pingPeriod = 54 * time.Second
	)

	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()

	for {
		// Priority: drain all pending critical messages before processing anything else
		for {
			select {
			case envelope := <-c.criticalSend:
				if err := c.writeEnvelope(envelope, writeWait); err != nil {
					log.Printf("WebSocket write error (critical) user=%s: %v", c.userID, err)
					return
				}
			default:
				goto waitForMessages
			}
		}
	waitForMessages:
		select {
		case <-c.done:
			return
		case envelope := <-c.criticalSend:
			if err := c.writeEnvelope(envelope, writeWait); err != nil {
				log.Printf("WebSocket write error (critical) user=%s: %v", c.userID, err)
				return
			}
		case envelope, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.writeEnvelope(envelope, writeWait); err != nil {
				log.Printf("WebSocket write error user=%s: %v", c.userID, err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
