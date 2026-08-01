package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// ====================
// IRST Timezone Helper
// ====================

// tehranLoc is the Asia/Tehran timezone, which is DST-aware:
// IRST (UTC+03:30) in winter, IRDT (UTC+04:30) in summer (~Mar 21 to ~Sep 21).
var tehranLoc *time.Location

func init() {
	var err error
	tehranLoc, err = time.LoadLocation("Asia/Tehran")
	if err != nil {
		log.Fatalf("Failed to load Asia/Tehran timezone: %v", err)
	}
}

// ====================
// Tournament Feed Hub
// ====================

// TournamentFeedHub manages WebSocket clients subscribed to tournament feeds.
// It handles two channel types:
//   - "tournaments:feed" — global feed for new tournaments and status changes
//   - "tournament:{id}" — per-tournament updates (prize pool, participants, status)
type TournamentFeedHub struct {
	// Global feed subscribers
	feedClients   map[*FeedClient]bool
	feedClientsMu sync.RWMutex

	// Per-tournament subscribers: contestID -> set of clients
	tournamentClients   map[string]map[*FeedClient]bool
	tournamentClientsMu sync.RWMutex

	// All connected clients (for connection limit enforcement)
	allClients   map[*FeedClient]bool
	allClientsMu sync.RWMutex

	maxConnections int // max concurrent feed connections (0 = unlimited)

	app *App
	log *zap.Logger
}

// NewTournamentFeedHub creates a new tournament feed hub.
func NewTournamentFeedHub(app *App, log *zap.Logger) *TournamentFeedHub {
	return &TournamentFeedHub{
		feedClients:       make(map[*FeedClient]bool),
		tournamentClients: make(map[string]map[*FeedClient]bool),
		allClients:        make(map[*FeedClient]bool),
		maxConnections:    1000,
		app:               app,
		log:               log,
	}
}

// registerClient adds a client to the all-clients set. Returns false if the limit is reached.
func (h *TournamentFeedHub) registerClient(client *FeedClient) bool {
	h.allClientsMu.Lock()
	defer h.allClientsMu.Unlock()
	if h.maxConnections > 0 && len(h.allClients) >= h.maxConnections {
		return false
	}
	h.allClients[client] = true
	return true
}

// unregisterClient removes a client from the all-clients set.
func (h *TournamentFeedHub) unregisterClient(client *FeedClient) {
	h.allClientsMu.Lock()
	delete(h.allClients, client)
	h.allClientsMu.Unlock()
}

// addFeedClient registers a client for the global tournament feed.
func (h *TournamentFeedHub) addFeedClient(client *FeedClient) {
	h.feedClientsMu.Lock()
	h.feedClients[client] = true
	h.feedClientsMu.Unlock()
}

// removeFeedClient unregisters a client from the global feed.
func (h *TournamentFeedHub) removeFeedClient(client *FeedClient) {
	h.feedClientsMu.Lock()
	delete(h.feedClients, client)
	h.feedClientsMu.Unlock()
}

// addTournamentClient registers a client for a specific tournament.
func (h *TournamentFeedHub) addTournamentClient(contestID string, client *FeedClient) {
	h.tournamentClientsMu.Lock()
	if h.tournamentClients[contestID] == nil {
		h.tournamentClients[contestID] = make(map[*FeedClient]bool)
	}
	h.tournamentClients[contestID][client] = true
	h.tournamentClientsMu.Unlock()
}

// removeTournamentClient unregisters a client from a specific tournament.
func (h *TournamentFeedHub) removeTournamentClient(contestID string, client *FeedClient) {
	h.tournamentClientsMu.Lock()
	if clients, ok := h.tournamentClients[contestID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.tournamentClients, contestID)
		}
	}
	h.tournamentClientsMu.Unlock()
}

// removeClientFromAll removes a client from all subscriptions and the connection registry.
func (h *TournamentFeedHub) removeClientFromAll(client *FeedClient) {
	h.removeFeedClient(client)

	h.tournamentClientsMu.Lock()
	for contestID, clients := range h.tournamentClients {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.tournamentClients, contestID)
		}
	}
	h.tournamentClientsMu.Unlock()

	h.unregisterClient(client)
}

// BroadcastToFeed sends a message to all global feed subscribers.
func (h *TournamentFeedHub) BroadcastToFeed(msg *contracts.TournamentFeedMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("Failed to marshal tournament feed message", zap.Error(err))
		return
	}

	h.feedClientsMu.RLock()
	clients := make([]*FeedClient, 0, len(h.feedClients))
	for c := range h.feedClients {
		clients = append(clients, c)
	}
	h.feedClientsMu.RUnlock()

	for _, client := range clients {
		client.send(data)
	}
}

// BroadcastToTournament sends a message to all subscribers of a specific tournament.
func (h *TournamentFeedHub) BroadcastToTournament(contestID string, msg *contracts.TournamentFeedMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		h.log.Error("Failed to marshal tournament message", zap.Error(err))
		return
	}

	h.tournamentClientsMu.RLock()
	clients := make([]*FeedClient, 0)
	if m, ok := h.tournamentClients[contestID]; ok {
		for c := range m {
			clients = append(clients, c)
		}
	}
	h.tournamentClientsMu.RUnlock()

	for _, client := range clients {
		client.send(data)
	}
}

// ConnectionCount returns the total number of feed connections.
func (h *TournamentFeedHub) ConnectionCount() int {
	h.allClientsMu.RLock()
	count := len(h.allClients)
	h.allClientsMu.RUnlock()
	return count
}

// ====================
// Feed Client
// ====================

// FeedClient is a lightweight WebSocket client for tournament feed subscriptions.
type FeedClient struct {
	conn       *websocket.Conn
	sendCh     chan []byte
	hub        *TournamentFeedHub
	subscribed map[string]bool // channel names subscribed to
	mu         sync.Mutex
	done       chan struct{}

	// Rate limiting for subscribe/unsubscribe (max 10 per minute)
	subActions    []time.Time
	subRateLimit  int           // max actions per window
	subRateWindow time.Duration // sliding window duration
}

// newFeedClient creates a new feed client.
func newFeedClient(conn *websocket.Conn, hub *TournamentFeedHub) *FeedClient {
	return &FeedClient{
		conn:          conn,
		sendCh:        make(chan []byte, 64),
		hub:           hub,
		subscribed:    make(map[string]bool),
		done:          make(chan struct{}),
		subRateLimit:  10,
		subRateWindow: time.Minute,
	}
}

// checkSubRateLimit returns true if the subscribe/unsubscribe action is allowed.
// Must be called while c.mu is held.
func (c *FeedClient) checkSubRateLimit() bool {
	now := time.Now()
	cutoff := now.Add(-c.subRateWindow)

	// Remove expired entries
	valid := 0
	for _, t := range c.subActions {
		if t.After(cutoff) {
			c.subActions[valid] = t
			valid++
		}
	}
	c.subActions = c.subActions[:valid]

	if len(c.subActions) >= c.subRateLimit {
		return false
	}
	c.subActions = append(c.subActions, now)
	return true
}

// send queues a message for delivery to the client.
func (c *FeedClient) send(data []byte) {
	select {
	case c.sendCh <- data:
	default:
		// Drop message if buffer full
	}
}

// readPump reads messages from the WebSocket connection.
func (c *FeedClient) readPump() {
	defer func() {
		c.hub.removeClientFromAll(c)
		c.conn.Close()
		close(c.done)
	}()

	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		c.handleMessage(message)
	}
}

// writePump writes messages to the WebSocket connection.
func (c *FeedClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	// conn.Close() is handled by readPump

	for {
		select {
		case msg, ok := <-c.sendCh:
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

// FeedSubscribeMessage is the message sent by clients to subscribe/unsubscribe.
type FeedSubscribeMessage struct {
	Type    string `json:"type"`    // "subscribe" or "unsubscribe"
	Channel string `json:"channel"` // "tournaments:feed" or "tournament:{id}"
}

// handleMessage processes an incoming client message.
func (c *FeedClient) handleMessage(data []byte) {
	var msg FeedSubscribeMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "subscribe":
		c.handleSubscribe(msg.Channel)
	case "unsubscribe":
		c.handleUnsubscribe(msg.Channel)
	}
}

// handleSubscribe subscribes the client to a channel.
func (c *FeedClient) handleSubscribe(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.subscribed[channel] {
		return // Already subscribed
	}

	if !c.checkSubRateLimit() {
		return // Rate limited
	}

	c.subscribed[channel] = true

	if channel == "tournaments:feed" {
		c.hub.addFeedClient(c)
		// Send snapshot of active tournaments
		infra.SafeGo(c.hub.log, "feed-snapshot", func() { c.sendFeedSnapshot() })
	} else if strings.HasPrefix(channel, "tournament:") {
		contestID := strings.TrimPrefix(channel, "tournament:")
		if contestID != "" {
			c.hub.addTournamentClient(contestID, c)
			// Send snapshot of this tournament
			infra.SafeGo(c.hub.log, "tournament-snapshot", func() { c.sendTournamentSnapshot(contestID) })
		}
	}
}

// handleUnsubscribe unsubscribes the client from a channel.
func (c *FeedClient) handleUnsubscribe(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.subscribed[channel] {
		return
	}

	if !c.checkSubRateLimit() {
		return // Rate limited
	}

	delete(c.subscribed, channel)

	if channel == "tournaments:feed" {
		c.hub.removeFeedClient(c)
	} else if strings.HasPrefix(channel, "tournament:") {
		contestID := strings.TrimPrefix(channel, "tournament:")
		if contestID != "" {
			c.hub.removeTournamentClient(contestID, c)
		}
	}
}

// calculatePrizePool computes the net prize pool after platform fees.
func calculatePrizePool(entryFeeCents int, participantCount int, platformFeeBps int, commissionRate float64) int64 {
	feeBps := platformFeeBps
	if feeBps <= 0 && commissionRate > 0 {
		feeBps = int(commissionRate * 100)
	}
	if feeBps <= 0 {
		feeBps = 2000 // Default 20%
	}
	gross := int64(entryFeeCents) * int64(participantCount)
	return (gross * int64(10000-feeBps)) / 10000
}

// sendFeedSnapshot sends the current state of active/upcoming tournaments.
func (c *FeedClient) sendFeedSnapshot() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := c.hub.app.pool.Replica().QueryContext(ctx, `
		SELECT c.id, c.name, c.status, c.asset_class, c.duration_type,
		       c.starts_at, c.ends_at, c.entry_fee_cents, c.is_free,
		       c.commission_rate, COALESCE(c.platform_fee_bps, 0), c.max_participants,
		       COUNT(cp.contest_id) as participant_count
		FROM contests c
		LEFT JOIN contest_participants cp ON cp.contest_id = c.id
		WHERE c.status IN ('registration_open', 'scheduled', 'running')
		GROUP BY c.id, c.name, c.status, c.asset_class, c.duration_type,
		         c.starts_at, c.ends_at, c.entry_fee_cents, c.is_free,
		         c.commission_rate, c.platform_fee_bps, c.max_participants
		ORDER BY c.starts_at ASC
		LIMIT 50
	`)
	if err != nil {
		c.hub.log.Error("Failed to query tournaments for snapshot", zap.Error(err))
		return
	}
	defer rows.Close()

	now := time.Now().UTC()
	var snapshots []contracts.TournamentSnapshot

	for rows.Next() {
		var snap contracts.TournamentSnapshot
		var startsAt, endsAt time.Time
		var commissionRate float64
		var platformFeeBps int
		var participantCount int
		var maxParts sql.NullInt32

		if err := rows.Scan(
			&snap.ID, &snap.Name, &snap.Status, &snap.MarketType, &snap.DurationType,
			&startsAt, &endsAt, &snap.EntryFeeCents, &snap.IsFree,
			&commissionRate, &platformFeeBps, &maxParts, &participantCount,
		); err != nil {
			c.hub.log.Error("Failed to scan tournament snapshot", zap.Error(err))
			return
		}

		snap.CurrentParticipants = participantCount
		snap.StartTimeUTC = startsAt.Format(time.RFC3339)
		snap.StartTimeIRST = startsAt.In(tehranLoc).Format("2006-01-02 15:04 MST")
		snap.EndTimeUTC = endsAt.Format(time.RFC3339)
		snap.EndTimeIRST = endsAt.In(tehranLoc).Format("2006-01-02 15:04 MST")
		snap.ServerTimeUTC = now.Format(time.RFC3339)

		if maxParts.Valid {
			maxP := int(maxParts.Int32)
			snap.MaxParticipants = &maxP
		}

		// Calculate time remaining
		remaining := endsAt.Sub(now)
		if remaining > 0 {
			snap.TimeRemainingMs = remaining.Milliseconds()
		}

		// Calculate prize pool
		if !snap.IsFree && snap.EntryFeeCents > 0 && participantCount > 0 {
			snap.PrizePoolCents = calculatePrizePool(snap.EntryFeeCents, participantCount, platformFeeBps, commissionRate)
		}

		snapshots = append(snapshots, snap)
	}

	// Send snapshot message
	msg := contracts.TournamentFeedMessage{
		Type:    "tournaments.snapshot",
		Payload: snapshots,
		Ts:      now.UnixMilli(),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	c.send(data)
}

// sendTournamentSnapshot sends the current state of a specific tournament.
func (c *FeedClient) sendTournamentSnapshot(contestID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var snap contracts.TournamentSnapshot
	var startsAt, endsAt time.Time
	var commissionRate float64
	var platformFeeBps int
	var participantCount int
	var maxParts sql.NullInt32

	err := c.hub.app.pool.Replica().QueryRowContext(ctx, `
		SELECT c.id, c.name, c.status, c.asset_class, c.duration_type,
		       c.starts_at, c.ends_at, c.entry_fee_cents, c.is_free,
		       c.commission_rate, COALESCE(c.platform_fee_bps, 0), c.max_participants,
		       COUNT(cp.contest_id) as participant_count
		FROM contests c
		LEFT JOIN contest_participants cp ON cp.contest_id = c.id
		WHERE c.id = $1
		GROUP BY c.id, c.name, c.status, c.asset_class, c.duration_type,
		         c.starts_at, c.ends_at, c.entry_fee_cents, c.is_free,
		         c.commission_rate, c.platform_fee_bps, c.max_participants
	`, contestID).Scan(
		&snap.ID, &snap.Name, &snap.Status, &snap.MarketType, &snap.DurationType,
		&startsAt, &endsAt, &snap.EntryFeeCents, &snap.IsFree,
		&commissionRate, &platformFeeBps, &maxParts, &participantCount,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg, _ := json.Marshal(map[string]string{"error": tradeMsg.TournamentNotFound})
			c.send(errMsg)
		}
		return
	}

	now := time.Now().UTC()
	snap.CurrentParticipants = participantCount
	snap.StartTimeUTC = startsAt.Format(time.RFC3339)
	snap.StartTimeIRST = startsAt.In(tehranLoc).Format("2006-01-02 15:04 MST")
	snap.EndTimeUTC = endsAt.Format(time.RFC3339)
	snap.EndTimeIRST = endsAt.In(tehranLoc).Format("2006-01-02 15:04 MST")
	snap.ServerTimeUTC = now.Format(time.RFC3339)

	if maxParts.Valid {
		maxP := int(maxParts.Int32)
		snap.MaxParticipants = &maxP
	}

	remaining := endsAt.Sub(now)
	if remaining > 0 {
		snap.TimeRemainingMs = remaining.Milliseconds()
	}

	if !snap.IsFree && snap.EntryFeeCents > 0 && participantCount > 0 {
		snap.PrizePoolCents = calculatePrizePool(snap.EntryFeeCents, participantCount, platformFeeBps, commissionRate)
	}

	msg := contracts.TournamentFeedMessage{
		Type:    "tournament.snapshot",
		Payload: snap,
		Ts:      now.UnixMilli(),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	c.send(data)
}

// ====================
// Tournament Feed Subscriber (Redis Pub/Sub)
// ====================

// TournamentFeedSubscriber listens for tournament feed events via Redis pub/sub.
type TournamentFeedSubscriber struct {
	hub *TournamentFeedHub
	app *App
	log *zap.Logger
	ctx context.Context
}

// NewTournamentFeedSubscriber creates a new subscriber.
func NewTournamentFeedSubscriber(app *App, hub *TournamentFeedHub, log *zap.Logger, ctx context.Context) *TournamentFeedSubscriber {
	return &TournamentFeedSubscriber{
		hub: hub,
		app: app,
		log: log,
		ctx: ctx,
	}
}

// Run subscribes to Redis pub/sub for tournament feed events.
func (s *TournamentFeedSubscriber) Run() {
	s.log.Info("Starting tournament feed subscriber via Redis pub/sub")

	pubsub := s.app.redis.Client().PSubscribe(s.ctx, "tournament_feed:*")
	defer pubsub.Close()

	ch := pubsub.Channel()

	for {
		select {
		case <-s.ctx.Done():
			s.log.Info("Tournament feed subscriber shutting down")
			return
		case msg, ok := <-ch:
			if !ok {
				s.log.Info("Tournament feed subscriber channel closed")
				return
			}

			// Parse the payload
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				s.log.Warn("Failed to unmarshal tournament feed payload",
					zap.Error(err), zap.String("channel", msg.Channel))
				continue
			}

			msgType, _ := payload["type"].(string)
			contestID, _ := payload["contest_id"].(string)

			now := time.Now().UnixMilli()

			feedMsg := &contracts.TournamentFeedMessage{
				Type:    msgType,
				Payload: payload,
				Ts:      now,
			}

			// Determine routing
			channel := msg.Channel
			if strings.HasSuffix(channel, ":global") {
				// Global feed message → broadcast to all feed subscribers
				s.hub.BroadcastToFeed(feedMsg)
			} else if contestID != "" {
				// Per-tournament message → broadcast to tournament subscribers
				s.hub.BroadcastToTournament(contestID, feedMsg)
				// Also broadcast to global feed
				s.hub.BroadcastToFeed(feedMsg)
			}

			s.log.Debug("Tournament feed message routed",
				zap.String("type", msgType),
				zap.String("contest_id", contestID),
				zap.String("channel", channel))
		}
	}
}

// ====================
// WebSocket Handler
// ====================

// handleTournamentFeedWS handles WebSocket connections for the tournament feed.
func (a *App) handleTournamentFeedWS(w http.ResponseWriter, r *http.Request) {
	// Check connection limit before upgrading
	if a.tournamentFeedHub.maxConnections > 0 {
		count := a.tournamentFeedHub.ConnectionCount()
		if count >= a.tournamentFeedHub.maxConnections {
			a.log().Warn("Tournament feed connection limit reached",
				zap.Int("current", count),
				zap.Int("max", a.tournamentFeedHub.maxConnections))
			http.Error(w, "Too many connections", http.StatusServiceUnavailable)
			return
		}
	}

	// Upgrade to WebSocket (no auth required - public feed)
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     checkWebSocketOrigin,
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.log().Error("Failed to upgrade tournament feed WebSocket", zap.Error(err))
		return
	}

	client := newFeedClient(conn, a.tournamentFeedHub)

	// Register client and enforce connection limit
	if !a.tournamentFeedHub.registerClient(client) {
		a.log().Warn("Tournament feed connection limit reached (race)",
			zap.Int("max", a.tournamentFeedHub.maxConnections))
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "Too many connections"))
		conn.Close()
		return
	}

	a.log().Info("Tournament feed client connected",
		zap.String("remote_addr", r.RemoteAddr))

	// Start read and write pumps
	infra.SafeGo(a.log(), "feed-write-pump", func() { client.writePump() })
	infra.SafeGo(a.log(), "feed-read-pump", func() { client.readPump() })
}

// ====================
// Initialization
// ====================

// initTournamentFeed initializes the tournament feed hub and subscriber.
func (a *App) initTournamentFeed() {
	a.tournamentFeedHub = NewTournamentFeedHub(a, a.log())

	// Start Redis pub/sub subscriber
	if a.redis != nil {
		a.wg.Add(1)
		infra.SafeGo(a.log(), "tournament-feed-subscriber", func() {
			defer a.wg.Done()
			subscriber := NewTournamentFeedSubscriber(a, a.tournamentFeedHub, a.log(), a.ctx)
			subscriber.Run()
		})
		a.log().Info("Tournament feed subscriber started")
	} else {
		a.log().Warn("Tournament feed subscriber not started (Redis not available)")
	}
}

// forwardContestEventToFeed forwards a contest lifecycle event to the tournament feed.
// This is called by ContestEventsConsumer.processRecord.
func (h *TournamentFeedHub) forwardContestEvent(event contracts.ContestEvent) {
	now := time.Now().UnixMilli()

	switch event.Type {
	case contracts.ContestEventStarted:
		msg := &contracts.TournamentFeedMessage{
			Type: contracts.TournamentMsgStatusChanged,
			Payload: contracts.TournamentStatusChangedPayload{
				ContestID: event.ContestID,
				NewStatus: "running",
			},
			Ts: now,
		}
		h.BroadcastToFeed(msg)
		h.BroadcastToTournament(event.ContestID, msg)

	case contracts.ContestEventCompleted:
		msg := &contracts.TournamentFeedMessage{
			Type: contracts.TournamentMsgEnded,
			Payload: contracts.TournamentEndedPayload{
				ContestID: event.ContestID,
				Reason:    "completed",
			},
			Ts: now,
		}
		h.BroadcastToFeed(msg)
		h.BroadcastToTournament(event.ContestID, msg)

	case contracts.ContestEventCancelled:
		msg := &contracts.TournamentFeedMessage{
			Type: contracts.TournamentMsgEnded,
			Payload: contracts.TournamentEndedPayload{
				ContestID: event.ContestID,
				Reason:    "cancelled",
			},
			Ts: now,
		}
		h.BroadcastToFeed(msg)
		h.BroadcastToTournament(event.ContestID, msg)

	case contracts.ContestEventPaused, contracts.ContestEventResumed:
		status := "paused"
		if event.Type == contracts.ContestEventResumed {
			status = "running"
		}
		msg := &contracts.TournamentFeedMessage{
			Type: contracts.TournamentMsgStatusChanged,
			Payload: contracts.TournamentStatusChangedPayload{
				ContestID: event.ContestID,
				NewStatus: status,
			},
			Ts: now,
		}
		h.BroadcastToFeed(msg)
		h.BroadcastToTournament(event.ContestID, msg)

	case contracts.ContestEventUpdated:
		msg := &contracts.TournamentFeedMessage{
			Type:    contracts.TournamentMsgUpdated,
			Payload: map[string]interface{}{"contest_id": event.ContestID, "metadata": event.Metadata},
			Ts:      now,
		}
		h.BroadcastToFeed(msg)
		h.BroadcastToTournament(event.ContestID, msg)
	}
}

// Ensure imports are used
var _ = fmt.Sprintf
