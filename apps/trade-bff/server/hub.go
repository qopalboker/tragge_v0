package server

import (
	"context"
	"encoding/json"
	"log"
	"runtime"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/vmihailenco/msgpack/v5"
)

// ====================
// Hub (Connection Manager)
// ====================

// Hub manages all active WebSocket connections
//
// Lock ordering (to prevent deadlocks, always acquire in this order):
//   1. clientsMu
//   2. contestClientsMu
//   3. userClientsMu
//   4. contestSymbols (internal RWMutex)
type Hub struct {
	// Registered clients by userID for user-specific messages
	clients   map[*Client]bool
	clientsMu sync.RWMutex

	// Contest ID to clients mapping for contest-specific lookups
	contestClients   map[string]map[*Client]bool
	contestClientsMu sync.RWMutex

	// User ID to clients mapping for user-specific events
	userClients   map[string][]*Client
	userClientsMu sync.RWMutex

	// Contest symbol cache: contestID → set of allowed symbols
	contestSymbols *contestSymbolCache

	// Contest asset class cache: contestID → asset_class string
	contestAssetClasses *contestAssetClassCache

	// App reference for database access (set via SetApp after App initialization)
	app *App

	// Channel for registering clients
	register chan *Client

	// Channel for unregistering clients
	unregister chan *Client

	// Price book for tick data
	priceBook *PriceBook

	// Metrics
	metrics        *Metrics
	contestMetrics *ContestMetrics

	// Message batching and delta compression
	batcher      *MessageBatcher
	deltaEncoder *DeltaEncoder

	// Broadcast worker pool for high-concurrency message delivery
	workerPool *BroadcastWorkerPool

	// Configuration
	broadcastInterval      time.Duration
	maxSymbolsPerTick      int
	broadcastWorkers       int // Number of workers for broadcast pool
	maxConnectionsPerUser  int // P1-1: Max WebSocket connections per user (0 = unlimited)

	// Context for shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// NewHub creates a new Hub with optional worker pool configuration
// If broadcastWorkers is 0, it defaults to 2 workers per CPU
func NewHub(priceBook *PriceBook, metrics *Metrics, broadcastInterval time.Duration, maxSymbols int, broadcastWorkers int) *Hub {
	ctx, cancel := context.WithCancel(context.Background())

	// Calculate default workers if not specified
	if broadcastWorkers <= 0 {
		broadcastWorkers = runtime.NumCPU() * 2
	}

	return &Hub{
		clients:        make(map[*Client]bool),
		contestClients: make(map[string]map[*Client]bool),
		userClients:    make(map[string][]*Client),
		contestSymbols:       newContestSymbolCache(5 * time.Minute),
		contestAssetClasses: newContestAssetClassCache(10 * time.Minute),
		register:             make(chan *Client, 256),
		unregister:           make(chan *Client, 256),
		priceBook:            priceBook,
		metrics:              metrics,
		batcher:              NewMessageBatcher(maxSymbols, broadcastInterval),
		deltaEncoder:         NewDeltaEncoder(),
		workerPool:           NewBroadcastWorkerPool(broadcastWorkers),
		broadcastInterval:    broadcastInterval,
		maxSymbolsPerTick:    maxSymbols,
		broadcastWorkers:     broadcastWorkers,
		maxConnectionsPerUser: 3, // P1-1: Default 3 connections per user
		ctx:                  ctx,
		cancel:               cancel,
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	ticker := time.NewTicker(h.broadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			h.closeAllClients()
			return

		case client := <-h.register:
			h.addClient(client)
			// Send full sync to new client (initial tick snapshot, filtered by contest symbols)
			h.sendFullSyncToClientFiltered(client)

		case client := <-h.unregister:
			h.removeClient(client)
			// Clean up delta encoder state for disconnected user
			h.deltaEncoder.RemoveUser(client.userID)

		case <-ticker.C:
			h.broadcastBatchedUpdatesContestAware()
		}
	}
}

// Stop stops the hub and its worker pool
func (h *Hub) Stop() {
	h.cancel()
	if h.workerPool != nil {
		h.workerPool.Stop()
	}
}

// addClient registers a new client
// Lock ordering: clientsMu → contestClientsMu → userClientsMu
// Evicted clients are removed from ALL three maps before Close() is called,
// so that no broadcast or lookup can reach a closed client.
func (h *Hub) addClient(client *Client) {
	h.clientsMu.Lock()
	h.clients[client] = true
	h.clientsMu.Unlock()

	h.contestClientsMu.Lock()
	isFirstForContest := h.contestClients[client.contestID] == nil
	if isFirstForContest {
		h.contestClients[client.contestID] = make(map[*Client]bool)
	}
	h.contestClients[client.contestID][client] = true
	h.contestClientsMu.Unlock()

	// P1-1: Enforce per-user connection limit with FIFO eviction
	var toEvict []*Client
	h.userClientsMu.Lock()
	if h.maxConnectionsPerUser > 0 && len(h.userClients[client.userID]) >= h.maxConnectionsPerUser {
		// Evict the oldest connection (first element)
		toEvict = append(toEvict, h.userClients[client.userID][0])
		h.userClients[client.userID] = h.userClients[client.userID][1:]
	}
	h.userClients[client.userID] = append(h.userClients[client.userID], client)
	h.userClientsMu.Unlock()

	// Remove evicted clients from ALL maps before closing (lock ordering preserved)
	if len(toEvict) > 0 {
		h.clientsMu.Lock()
		for _, ev := range toEvict {
			delete(h.clients, ev)
		}
		h.clientsMu.Unlock()

		h.contestClientsMu.Lock()
		for _, ev := range toEvict {
			if set, exists := h.contestClients[ev.contestID]; exists {
				delete(set, ev)
				if len(set) == 0 {
					delete(h.contestClients, ev.contestID)
				}
			}
		}
		h.contestClientsMu.Unlock()

		// Close evicted connections outside all locks and update metrics
		for _, ev := range toEvict {
			log.Printf("Evicting oldest connection for user=%s (per-user limit=%d)", client.userID, h.maxConnectionsPerUser)
			h.metrics.wsConnections.Add(-1)
			h.contestMetrics.Dec(ev.contestID)
			ev.Close()
		}
	}

	h.metrics.wsConnections.Add(1)
	h.contestMetrics.Inc(client.contestID)

	// Pre-load the symbols cache when the first client for a contest connects
	if isFirstForContest {
		go h.loadContestSymbols(client.contestID)
	}

	log.Printf("Client registered: user=%s contest=%s (total=%d)",
		client.userID, client.contestID, h.metrics.wsConnections.Load())
}

// removeClient unregisters a client
// Lock ordering: clientsMu → contestClientsMu → userClientsMu
func (h *Hub) removeClient(client *Client) {
	h.clientsMu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		h.clientsMu.Unlock()

		var contestFullyRemoved bool
		h.contestClientsMu.Lock()
		if set, exists := h.contestClients[client.contestID]; exists {
			delete(set, client)
			if len(set) == 0 {
				delete(h.contestClients, client.contestID)
				contestFullyRemoved = true
			}
		}
		h.contestClientsMu.Unlock()

		// Clean up contest symbols cache when the last client for a contest disconnects
		if contestFullyRemoved {
			h.contestSymbols.delete(client.contestID)
			h.contestMetrics.DeleteContest(client.contestID)
		} else {
			h.contestMetrics.Dec(client.contestID)
		}

		h.userClientsMu.Lock()
		clients := h.userClients[client.userID]
		for i, c := range clients {
			if c == client {
				h.userClients[client.userID] = append(clients[:i], clients[i+1:]...)
				break
			}
		}
		if len(h.userClients[client.userID]) == 0 {
			delete(h.userClients, client.userID)
		}
		h.userClientsMu.Unlock()

		h.metrics.wsConnections.Add(-1)
		log.Printf("Client unregistered: user=%s contest=%s (total=%d)",
			client.userID, client.contestID, h.metrics.wsConnections.Load())
	} else {
		h.clientsMu.Unlock()
	}
}

// DisconnectUser finds and closes all WebSocket connections for a specific user+contest.
// Used for cross-pod disconnect signaling when another pod takes over the connection.
func (h *Hub) DisconnectUser(userID, contestID string) {
	h.userClientsMu.RLock()
	clients := h.userClients[userID]
	var toClose []*Client
	for _, c := range clients {
		if c.contestID == contestID {
			toClose = append(toClose, c)
		}
	}
	h.userClientsMu.RUnlock()

	for _, c := range toClose {
		log.Printf("Cross-pod disconnect: closing stale connection for user=%s contest=%s", userID, contestID)
		c.Close()
	}
}

// closeAllClients closes all client connections
// Lock ordering: clientsMu → contestClientsMu → userClientsMu
func (h *Hub) closeAllClients() {
	h.clientsMu.Lock()

	for client := range h.clients {
		client.Close()
	}
	h.clients = make(map[*Client]bool)

	h.contestClientsMu.Lock()
	h.contestClients = make(map[string]map[*Client]bool)
	h.contestClientsMu.Unlock()

	h.userClientsMu.Lock()
	h.userClients = make(map[string][]*Client)
	h.userClientsMu.Unlock()

	h.clientsMu.Unlock()

	log.Println("All WebSocket clients closed")
}

// GetContestClientCount returns the number of clients connected to a specific contest.
func (h *Hub) GetContestClientCount(contestID string) int {
	h.contestClientsMu.RLock()
	count := len(h.contestClients[contestID])
	h.contestClientsMu.RUnlock()
	return count
}

// GetActiveContests returns a list of contest IDs that have at least one connected client.
func (h *Hub) GetActiveContests() []string {
	h.contestClientsMu.RLock()
	contests := make([]string, 0, len(h.contestClients))
	for contestID := range h.contestClients {
		contests = append(contests, contestID)
	}
	h.contestClientsMu.RUnlock()
	return contests
}

// getContestClients returns a snapshot slice of clients connected to a specific contest.
// The returned slice is safe to use outside the lock.
func (h *Hub) getContestClients(contestID string) []*Client {
	h.contestClientsMu.RLock()
	set := h.contestClients[contestID]
	if len(set) == 0 {
		h.contestClientsMu.RUnlock()
		return nil
	}
	clients := make([]*Client, 0, len(set))
	for client := range set {
		clients = append(clients, client)
	}
	h.contestClientsMu.RUnlock()
	return clients
}

// broadcastBatchedUpdates sends batched tick updates to all clients
// This is more efficient than individual tick_snapshot messages for high concurrency
// Clients using MessagePack encoding receive binary-encoded data for bandwidth savings
func (h *Hub) broadcastBatchedUpdates() {
	start := time.Now()

	// Get latest prices and add to batcher
	snapshot := h.priceBook.GetSnapshot(h.maxSymbolsPerTick)
	if len(snapshot.Symbols) == 0 {
		return
	}

	// Add ticks to batcher buffer
	h.batcher.AddTickSnapshot(snapshot)

	// Flush the batch - this creates a single batched message
	tickBatch := h.batcher.FlushTicks()
	if tickBatch == nil {
		return
	}

	// Serialize to JSON (for JSON clients and backward compatibility)
	jsonData, err := json.Marshal(tickBatch)
	if err != nil {
		log.Printf("Failed to marshal tick batch to JSON: %v", err)
		return
	}

	// Serialize to MessagePack (for MessagePack clients - ~40-50% smaller)
	msgpackData, err := msgpack.Marshal(tickBatch)
	if err != nil {
		log.Printf("Failed to marshal tick batch to MessagePack: %v", err)
		// Fall back to JSON-only broadcast
		msgpackData = nil
	}

	// Broadcast to all clients using worker pool for high concurrency
	h.clientsMu.RLock()
	clientCount := len(h.clients)
	if h.workerPool != nil && clientCount > 10 {
		// Use worker pool for large number of clients (reduces syscall overhead)
		clients := make([]*Client, 0, clientCount)
		for client := range h.clients {
			clients = append(clients, client)
		}
		h.clientsMu.RUnlock()
		h.workerPool.BroadcastTickBatchToAll(clients, jsonData, msgpackData)
	} else {
		// Direct send for small number of clients (lower overhead)
		for client := range h.clients {
			client.SendTickBatch(jsonData, msgpackData)
		}
		h.clientsMu.RUnlock()
	}

	elapsed := time.Since(start).Milliseconds()
	h.metrics.RecordBroadcast(elapsed)

	// Track bandwidth savings (tick_batch vs individual tick_snapshot)
	// Original: ~100 bytes per tick × 30 symbols = ~3000 bytes
	// Batched: ~100 bytes per tick × 30 symbols + ~50 bytes overhead = ~3050 bytes
	// But only 1 message instead of potentially 30 messages
	h.metrics.wsBandwidthSaved.Add(int64(len(snapshot.Symbols)-1) * 50) // ~50 bytes message overhead per avoided message

	if clientCount > 0 && elapsed > 50 {
		log.Printf("Broadcast tick_batch seq=%d to %d clients in %dms (symbols=%d, json=%d bytes, msgpack=%d bytes, workers=%d)",
			tickBatch.Sequence, clientCount, elapsed, tickBatch.Count, len(jsonData), len(msgpackData), h.broadcastWorkers)
	}
}

// sendFullSyncToClient sends initial state to a newly connected client
// Uses MessagePack encoding for clients that request it
func (h *Hub) sendFullSyncToClient(client *Client) {
	// Send current tick snapshot as a full sync
	snapshot := h.priceBook.GetSnapshot(h.maxSymbolsPerTick)
	if len(snapshot.Symbols) == 0 {
		return
	}

	// Create a batched message with sequence 0 to indicate initial sync
	initialBatch := &BatchedMessage{
		Type:     "tick_batch",
		Sequence: h.batcher.GetSequence(), // Current sequence so client knows where to start
		Count:    len(snapshot.Symbols),
		Data:     TickBatchData{Symbols: snapshot.Symbols},
		Ts:       snapshot.Ts,
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(initialBatch)
	if err != nil {
		log.Printf("Failed to marshal initial sync to JSON: %v", err)
		return
	}

	// Serialize to MessagePack for clients that request it
	var msgpackData []byte
	if client.encoding == EncodingMsgPack {
		msgpackData, err = msgpack.Marshal(initialBatch)
		if err != nil {
			log.Printf("Failed to marshal initial sync to MessagePack: %v", err)
			// Fall back to JSON
			msgpackData = nil
		}
	}

	client.SendTickBatch(jsonData, msgpackData)
	encoding := "json"
	if client.encoding == EncodingMsgPack {
		encoding = "msgpack"
	}
	log.Printf("Sent initial sync to user=%s contest=%s (symbols=%d, seq=%d, encoding=%s)",
		client.userID, client.contestID, len(snapshot.Symbols), initialBatch.Sequence, encoding)
}

// broadcastTickSnapshot sends the latest tick snapshot to all clients (legacy method)
// Deprecated: Use broadcastBatchedUpdates for better performance
func (h *Hub) broadcastTickSnapshot() {
	start := time.Now()

	snapshot := h.priceBook.GetSnapshot(h.maxSymbolsPerTick)
	if len(snapshot.Symbols) == 0 {
		return
	}

	msg := WSMessage{
		Type:    "tick_snapshot",
		Payload: snapshot,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal tick snapshot: %v", err)
		return
	}

	h.clientsMu.RLock()
	clientCount := len(h.clients)
	for client := range h.clients {
		client.SendMessage(data)
	}
	h.clientsMu.RUnlock()

	elapsed := time.Since(start).Milliseconds()
	h.metrics.RecordBroadcast(elapsed)

	if clientCount > 0 && elapsed > 50 {
		log.Printf("Broadcast tick_snapshot to %d clients in %dms (symbols=%d)",
			clientCount, elapsed, len(snapshot.Symbols))
	}
}

// SendToUser sends a message to all connections for a specific user
func (h *Hub) SendToUser(userID string, msg *WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal user message: %v", err)
		return
	}

	h.userClientsMu.RLock()
	clients := h.userClients[userID]
	for _, client := range clients {
		client.SendCriticalMessage(data)
	}
	h.userClientsMu.RUnlock()
}

// SendToContest sends a message to all connections for a specific contest
func (h *Hub) SendToContest(contestID string, msg *WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Failed to marshal contest message: %v", err)
		return
	}

	clients := h.getContestClients(contestID)

	// Use worker pool for large number of clients
	if h.workerPool != nil && len(clients) > 10 {
		h.workerPool.BroadcastCriticalToAll(clients, data)
	} else {
		for _, client := range clients {
			client.SendCriticalMessage(data)
		}
	}
}

// ====================
// WebSocket Messages
// ====================

// WebSocket message types
const (
	MsgTypeOrderRequest = "order_request"
	MsgTypeOrderAck     = "order_ack"
	MsgTypeOrderReject  = "order_reject"
)

// WSMessage represents a WebSocket message envelope
type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
	Phase   string      `json:"phase,omitempty"` // For backward compatibility
}

// WSOrderRequest represents an incoming order via WebSocket
type WSOrderRequest struct {
	Type       string              `json:"type"` // "order_request"
	RequestID  string              `json:"request_id"`
	Symbol     string              `json:"symbol"`
	Side       contracts.OrderSide `json:"side"`       // "BUY" or "SELL"
	OrderType  contracts.OrderType `json:"order_type"` // "MARKET", "BUY_LIMIT", etc.
	Qty        int64               `json:"qty"`
	LimitPrice *float64            `json:"limit_price,omitempty"`
	StopPrice  *float64            `json:"stop_price,omitempty"`
	TakeProfit *float64            `json:"take_profit,omitempty"`
	StopLoss   *float64            `json:"stop_loss,omitempty"`
}

// WSOrderAck represents order acknowledgment sent to client
type WSOrderAck struct {
	Type      string `json:"type"`       // "order_ack"
	RequestID string `json:"request_id"` // Client-provided request ID
	OrderID   string `json:"order_id"`   // Server-generated order ID
	Status    string `json:"status"`     // "accepted"
}

// WSRateLimitInfo contains rate limit metadata for rejection responses.
type WSRateLimitInfo struct {
	Scope        string `json:"scope"`          // "user", "contest", or "global"
	Limit        int    `json:"limit"`          // Maximum requests allowed
	Window       string `json:"window"`         // Window duration (e.g., "1s")
	RetryAfterMs int64  `json:"retry_after_ms"` // Milliseconds until next request allowed
}

// WSOrderReject represents order rejection sent to client
type WSOrderReject struct {
	Type      string           `json:"type"`                 // "order_reject"
	RequestID string           `json:"request_id"`           // Client-provided request ID
	Code      string           `json:"code"`                 // Error code
	Message   string           `json:"message"`              // Human-readable error message
	RateLimit *WSRateLimitInfo `json:"rate_limit,omitempty"` // Present only for RATE_LIMITED rejections
}
