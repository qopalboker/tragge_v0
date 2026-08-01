package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/gorilla/websocket"
)

// WSTestServer provides a WebSocket test server for integration tests.
type WSTestServer struct {
	Server     *httptest.Server
	Auth       *auth.Auth
	Env        *TestEnv
	Upgrader   websocket.Upgrader
	Clients    map[*websocket.Conn]string // conn -> userID
	ClientsMu  sync.RWMutex
	PriceBook  map[string]contracts.SymbolTick
	PriceBookMu sync.RWMutex
	BroadcastCh chan []byte
	Done       chan struct{}
}

// NewWSTestServer creates a new WebSocket test server.
func NewWSTestServer(t *testing.T, env *TestEnv) *WSTestServer {
	t.Helper()

	authConfig := auth.DefaultConfig()
	authConfig.JWTSecret = env.JWTSecret
	authService := auth.New(authConfig)

	ws := &WSTestServer{
		Auth: authService,
		Env:  env,
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		Clients:     make(map[*websocket.Conn]string),
		PriceBook:   make(map[string]contracts.SymbolTick),
		BroadcastCh: make(chan []byte, 100),
		Done:        make(chan struct{}),
	}

	// Initialize with some default prices
	ws.PriceBook["AAPL"] = contracts.SymbolTick{Symbol: "AAPL", Bid: 174.50, Ask: 174.55, Last: 174.52}
	ws.PriceBook["GOOGL"] = contracts.SymbolTick{Symbol: "GOOGL", Bid: 139.80, Ask: 139.85, Last: 139.82}
	ws.PriceBook["MSFT"] = contracts.SymbolTick{Symbol: "MSFT", Bid: 379.90, Ask: 380.00, Last: 379.95}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/trade", ws.handleWebSocket)
	mux.HandleFunc("/ws-stats", ws.handleStats)

	ws.Server = httptest.NewServer(mux)

	// Start broadcast loop
	go ws.broadcastLoop()

	return ws
}

// Close shuts down the WebSocket test server.
func (ws *WSTestServer) Close() {
	close(ws.Done)
	ws.ClientsMu.Lock()
	for conn := range ws.Clients {
		conn.Close()
	}
	ws.Clients = make(map[*websocket.Conn]string)
	ws.ClientsMu.Unlock()
	ws.Server.Close()
}

// WSMessage represents a WebSocket message envelope.
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Phase   string          `json:"phase,omitempty"`
}

// handleWebSocket handles WebSocket upgrade and connection.
func (ws *WSTestServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract contest_id from query params
	contestID := r.URL.Query().Get("contest_id")
	if contestID == "" {
		http.Error(w, `{"error": "contest_id is required"}`, http.StatusBadRequest)
		return
	}

	// Authenticate via JWT from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		http.Error(w, `{"error": "authorization header required"}`, http.StatusUnauthorized)
		return
	}

	const bearerPrefix = "Bearer "
	if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
		http.Error(w, `{"error": "invalid authorization header format"}`, http.StatusUnauthorized)
		return
	}
	token := authHeader[len(bearerPrefix):]

	claims, err := ws.Auth.Token.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, `{"error": "invalid or expired token"}`, http.StatusUnauthorized)
		return
	}

	userID := claims.UserID

	// Upgrade to WebSocket
	conn, err := ws.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Register client
	ws.ClientsMu.Lock()
	ws.Clients[conn] = userID
	ws.ClientsMu.Unlock()

	// Send welcome message
	welcomeMsg := WSMessage{
		Type:  "contest_state",
		Phase: "CONNECTING",
	}
	welcomeData, _ := json.Marshal(welcomeMsg)
	conn.WriteMessage(websocket.TextMessage, welcomeData)

	// Handle client messages in separate goroutine
	go ws.readPump(conn, userID, contestID)
}

// readPump reads messages from a WebSocket connection.
func (ws *WSTestServer) readPump(conn *websocket.Conn, userID, contestID string) {
	defer func() {
		ws.ClientsMu.Lock()
		delete(ws.Clients, conn)
		ws.ClientsMu.Unlock()
		conn.Close()
	}()

	conn.SetReadLimit(4096)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}

		// Echo back for testing
		var msg WSMessage
		if json.Unmarshal(message, &msg) == nil && msg.Type == "ping" {
			pong := WSMessage{Type: "pong"}
			data, _ := json.Marshal(pong)
			conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

// broadcastLoop sends periodic tick snapshots to all connected clients.
func (ws *WSTestServer) broadcastLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ws.Done:
			return
		case <-ticker.C:
			ws.broadcastTickSnapshot()
		case data := <-ws.BroadcastCh:
			ws.broadcastToAll(data)
		}
	}
}

// broadcastTickSnapshot sends the current price book to all clients.
func (ws *WSTestServer) broadcastTickSnapshot() {
	ws.PriceBookMu.RLock()
	symbols := make([]contracts.SymbolTick, 0, len(ws.PriceBook))
	for _, tick := range ws.PriceBook {
		symbols = append(symbols, tick)
	}
	ws.PriceBookMu.RUnlock()

	if len(symbols) == 0 {
		return
	}

	snapshot := contracts.TickSnapshot{
		Ts:      time.Now().UnixMilli(),
		Symbols: symbols,
	}

	payload, _ := json.Marshal(snapshot)
	msg := WSMessage{
		Type:    "tick_snapshot",
		Payload: payload,
	}

	data, _ := json.Marshal(msg)
	ws.broadcastToAll(data)
}

// broadcastToAll sends a message to all connected clients.
func (ws *WSTestServer) broadcastToAll(data []byte) {
	ws.ClientsMu.RLock()
	defer ws.ClientsMu.RUnlock()

	for conn := range ws.Clients {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			// Client disconnected
			continue
		}
	}
}

// SendToUser sends a message to a specific user's connections.
func (ws *WSTestServer) SendToUser(userID string, msg *WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	ws.ClientsMu.RLock()
	defer ws.ClientsMu.RUnlock()

	for conn, uid := range ws.Clients {
		if uid == userID {
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			conn.WriteMessage(websocket.TextMessage, data)
		}
	}
}

// UpdatePrice updates a price in the price book.
func (ws *WSTestServer) UpdatePrice(tick contracts.SymbolTick) {
	ws.PriceBookMu.Lock()
	ws.PriceBook[tick.Symbol] = tick
	ws.PriceBookMu.Unlock()
}

// GetConnectionCount returns the number of active connections.
func (ws *WSTestServer) GetConnectionCount() int {
	ws.ClientsMu.RLock()
	defer ws.ClientsMu.RUnlock()
	return len(ws.Clients)
}

// handleStats returns WebSocket statistics.
func (ws *WSTestServer) handleStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"ws_connections": ws.GetConnectionCount(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// ============================================================================
// WebSocket Client Helper
// ============================================================================

// WSClient is a test WebSocket client.
type WSClient struct {
	Conn      *websocket.Conn
	Messages  chan WSMessage
	Done      chan struct{}
	closeOnce sync.Once
}

// NewWSClient creates a new WebSocket test client.
func NewWSClient(t *testing.T, serverURL, contestID, token string) *WSClient {
	t.Helper()

	// Convert http:// to ws://
	wsURL := strings.Replace(serverURL, "http://", "ws://", 1)
	wsURL = wsURL + "/ws/trade?contest_id=" + contestID

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			t.Fatalf("Failed to connect: %v (status: %d)", err, resp.StatusCode)
		}
		t.Fatalf("Failed to connect: %v", err)
	}

	client := &WSClient{
		Conn:     conn,
		Messages: make(chan WSMessage, 100),
		Done:     make(chan struct{}),
	}

	// Start reading messages
	go client.readLoop()

	return client
}

// readLoop reads messages from the WebSocket and sends them to the Messages channel.
func (c *WSClient) readLoop() {
	defer c.Close()

	for {
		select {
		case <-c.Done:
			return
		default:
		}

		c.Conn.SetReadDeadline(time.Now().Add(65 * time.Second))
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		select {
		case c.Messages <- msg:
		default:
			// Channel full, discard old message
			select {
			case <-c.Messages:
			default:
			}
			c.Messages <- msg
		}
	}
}

// Send sends a message to the WebSocket server.
func (c *WSClient) Send(msg WSMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.Conn.WriteMessage(websocket.TextMessage, data)
}

// WaitForMessage waits for a specific message type with timeout.
func (c *WSClient) WaitForMessage(msgType string, timeout time.Duration) (*WSMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case msg := <-c.Messages:
			if msg.Type == msgType {
				return &msg, nil
			}
		}
	}
}

// Close closes the WebSocket connection.
func (c *WSClient) Close() {
	c.closeOnce.Do(func() {
		close(c.Done)
		c.Conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
		c.Conn.Close()
	})
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestWebSocket_Connect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create WebSocket server
	server := NewWSTestServer(t, env)
	defer server.Close()

	// Create test user
	passwordHash, _ := server.Auth.HashPassword("testpassword123")
	userID := env.CreateTestUser(ctx, t, "wstest@example.com", passwordHash)

	// Create contest
	contestID := env.CreateTestContest(ctx, t, "WS Test Contest", "running")
	env.AddContestSymbol(ctx, t, contestID, "AAPL")
	env.JoinContest(ctx, t, contestID, userID, 100000)

	// Generate token
	tokenPair, err := server.Auth.Token.GenerateTokenPair(userID, []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	t.Run("Connect_Success", func(t *testing.T) {
		client := NewWSClient(t, server.Server.URL, contestID, tokenPair.AccessToken)
		defer client.Close()

		// Wait for welcome message
		msg, err := client.WaitForMessage("contest_state", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to receive welcome message: %v", err)
		}

		if msg.Phase != "CONNECTING" {
			t.Errorf("Expected phase CONNECTING, got %s", msg.Phase)
		}

		// Verify connection count
		time.Sleep(100 * time.Millisecond)
		if server.GetConnectionCount() != 1 {
			t.Errorf("Expected 1 connection, got %d", server.GetConnectionCount())
		}
	})

	t.Run("Connect_InvalidToken", func(t *testing.T) {
		wsURL := strings.Replace(server.Server.URL, "http://", "ws://", 1)
		wsURL = wsURL + "/ws/trade?contest_id=" + contestID

		header := http.Header{}
		header.Set("Authorization", "Bearer invalid-token")

		dialer := websocket.Dialer{
			HandshakeTimeout: 5 * time.Second,
		}

		_, resp, err := dialer.Dial(wsURL, header)
		if err == nil {
			t.Error("Expected connection to fail with invalid token")
		}
		if resp != nil && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Connect_MissingContestID", func(t *testing.T) {
		wsURL := strings.Replace(server.Server.URL, "http://", "ws://", 1)
		wsURL = wsURL + "/ws/trade" // No contest_id

		header := http.Header{}
		header.Set("Authorization", "Bearer "+tokenPair.AccessToken)

		dialer := websocket.Dialer{
			HandshakeTimeout: 5 * time.Second,
		}

		_, resp, err := dialer.Dial(wsURL, header)
		if err == nil {
			t.Error("Expected connection to fail without contest_id")
		}
		if resp != nil && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}

func TestWebSocket_ReceiveTicks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create WebSocket server
	server := NewWSTestServer(t, env)
	defer server.Close()

	// Create test user
	passwordHash, _ := server.Auth.HashPassword("testpassword123")
	userID := env.CreateTestUser(ctx, t, "ticktest@example.com", passwordHash)

	// Create contest
	contestID := env.CreateTestContest(ctx, t, "Tick Test Contest", "running")
	env.AddContestSymbol(ctx, t, contestID, "AAPL")
	env.JoinContest(ctx, t, contestID, userID, 100000)

	// Generate token
	tokenPair, err := server.Auth.Token.GenerateTokenPair(userID, []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	t.Run("ReceiveTickSnapshot", func(t *testing.T) {
		client := NewWSClient(t, server.Server.URL, contestID, tokenPair.AccessToken)
		defer client.Close()

		// Skip welcome message
		client.WaitForMessage("contest_state", 5*time.Second)

		// Wait for tick snapshot (broadcasts every 1 second)
		msg, err := client.WaitForMessage("tick_snapshot", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to receive tick snapshot: %v", err)
		}

		// Parse payload
		var snapshot contracts.TickSnapshot
		if err := json.Unmarshal(msg.Payload, &snapshot); err != nil {
			t.Fatalf("Failed to parse tick snapshot: %v", err)
		}

		if len(snapshot.Symbols) == 0 {
			t.Error("Expected at least one symbol in tick snapshot")
		}

		// Verify timestamp
		if snapshot.Ts <= 0 {
			t.Error("Expected valid timestamp")
		}
	})

	t.Run("ReceiveUpdatedPrices", func(t *testing.T) {
		client := NewWSClient(t, server.Server.URL, contestID, tokenPair.AccessToken)
		defer client.Close()

		// Skip initial messages
		client.WaitForMessage("contest_state", 5*time.Second)

		// Update price
		server.UpdatePrice(contracts.SymbolTick{
			Symbol: "AAPL",
			Bid:    180.00,
			Ask:    180.05,
			Last:   180.02,
		})

		// Wait for tick snapshot with updated price
		var found bool
		for i := 0; i < 3; i++ {
			msg, err := client.WaitForMessage("tick_snapshot", 2*time.Second)
			if err != nil {
				continue
			}

			var snapshot contracts.TickSnapshot
			if err := json.Unmarshal(msg.Payload, &snapshot); err != nil {
				continue
			}

			for _, tick := range snapshot.Symbols {
				if tick.Symbol == "AAPL" && tick.Bid == 180.00 {
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if !found {
			t.Error("Expected to receive updated AAPL price")
		}
	})
}

func TestWebSocket_MultipleConnections(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create WebSocket server
	server := NewWSTestServer(t, env)
	defer server.Close()

	// Create multiple users
	numUsers := 5
	clients := make([]*WSClient, numUsers)

	// Create contest
	contestID := env.CreateTestContest(ctx, t, "Multi Test Contest", "running")
	env.AddContestSymbol(ctx, t, contestID, "AAPL")

	for i := 0; i < numUsers; i++ {
		email := "multi" + string(rune('a'+i)) + "@example.com"
		passwordHash, _ := server.Auth.HashPassword("testpassword123")
		userID := env.CreateTestUser(ctx, t, email, passwordHash)
		env.JoinContest(ctx, t, contestID, userID, 100000)

		tokenPair, err := server.Auth.Token.GenerateTokenPair(userID, []string{"user"})
		if err != nil {
			t.Fatalf("Failed to generate token for user %d: %v", i, err)
		}

		client := NewWSClient(t, server.Server.URL, contestID, tokenPair.AccessToken)
		clients[i] = client

		// Wait for welcome message
		client.WaitForMessage("contest_state", 5*time.Second)
	}

	// Cleanup clients
	defer func() {
		for _, client := range clients {
			if client != nil {
				client.Close()
			}
		}
	}()

	// Wait a bit for all connections to register
	time.Sleep(500 * time.Millisecond)

	t.Run("AllClientsConnected", func(t *testing.T) {
		count := server.GetConnectionCount()
		if count != numUsers {
			t.Errorf("Expected %d connections, got %d", numUsers, count)
		}
	})

	t.Run("AllClientsReceiveBroadcast", func(t *testing.T) {
		// Count how many clients receive tick snapshots
		var received atomic.Int32

		var wg sync.WaitGroup
		for i, client := range clients {
			wg.Add(1)
			go func(idx int, c *WSClient) {
				defer wg.Done()
				_, err := c.WaitForMessage("tick_snapshot", 3*time.Second)
				if err == nil {
					received.Add(1)
				}
			}(i, client)
		}

		wg.Wait()

		if received.Load() != int32(numUsers) {
			t.Errorf("Expected %d clients to receive tick snapshot, got %d", numUsers, received.Load())
		}
	})

	t.Run("CloseOneConnection", func(t *testing.T) {
		// Close one client
		clients[0].Close()
		clients[0] = nil

		// Wait for disconnection to be processed
		time.Sleep(500 * time.Millisecond)

		count := server.GetConnectionCount()
		if count != numUsers-1 {
			t.Errorf("Expected %d connections after closing one, got %d", numUsers-1, count)
		}
	})
}

func TestWebSocket_PingPong(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create WebSocket server
	server := NewWSTestServer(t, env)
	defer server.Close()

	// Create test user
	passwordHash, _ := server.Auth.HashPassword("testpassword123")
	userID := env.CreateTestUser(ctx, t, "pingtest@example.com", passwordHash)

	// Create contest
	contestID := env.CreateTestContest(ctx, t, "Ping Test Contest", "running")
	env.AddContestSymbol(ctx, t, contestID, "AAPL")
	env.JoinContest(ctx, t, contestID, userID, 100000)

	// Generate token
	tokenPair, err := server.Auth.Token.GenerateTokenPair(userID, []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	t.Run("SendPing_ReceivePong", func(t *testing.T) {
		client := NewWSClient(t, server.Server.URL, contestID, tokenPair.AccessToken)
		defer client.Close()

		// Skip welcome message
		client.WaitForMessage("contest_state", 5*time.Second)

		// Send ping
		err := client.Send(WSMessage{Type: "ping"})
		if err != nil {
			t.Fatalf("Failed to send ping: %v", err)
		}

		// Wait for pong
		msg, err := client.WaitForMessage("pong", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to receive pong: %v", err)
		}

		if msg.Type != "pong" {
			t.Errorf("Expected type 'pong', got '%s'", msg.Type)
		}
	})
}

func TestWebSocket_UserSpecificMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create WebSocket server
	server := NewWSTestServer(t, env)
	defer server.Close()

	// Create contest
	contestID := env.CreateTestContest(ctx, t, "User Specific Test Contest", "running")
	env.AddContestSymbol(ctx, t, contestID, "AAPL")

	// Create two users
	passwordHash, _ := server.Auth.HashPassword("testpassword123")
	user1ID := env.CreateTestUser(ctx, t, "user1@example.com", passwordHash)
	user2ID := env.CreateTestUser(ctx, t, "user2@example.com", passwordHash)
	env.JoinContest(ctx, t, contestID, user1ID, 100000)
	env.JoinContest(ctx, t, contestID, user2ID, 100000)

	token1, _ := server.Auth.Token.GenerateTokenPair(user1ID, []string{"user"})
	token2, _ := server.Auth.Token.GenerateTokenPair(user2ID, []string{"user"})

	client1 := NewWSClient(t, server.Server.URL, contestID, token1.AccessToken)
	defer client1.Close()
	client2 := NewWSClient(t, server.Server.URL, contestID, token2.AccessToken)
	defer client2.Close()

	// Skip welcome messages
	client1.WaitForMessage("contest_state", 5*time.Second)
	client2.WaitForMessage("contest_state", 5*time.Second)

	t.Run("SendToSpecificUser", func(t *testing.T) {
		// Send message to user1 only
		fillPayload, _ := json.Marshal(map[string]interface{}{
			"fill_id":  "test-fill-123",
			"order_id": "test-order-456",
			"symbol":   "AAPL",
			"qty":      100,
			"price":    175.00,
		})
		server.SendToUser(user1ID, &WSMessage{
			Type:    "fill",
			Payload: fillPayload,
		})

		// User1 should receive the fill
		msg, err := client1.WaitForMessage("fill", 3*time.Second)
		if err != nil {
			t.Fatalf("User1 should have received fill message: %v", err)
		}
		if msg.Type != "fill" {
			t.Errorf("Expected fill message, got %s", msg.Type)
		}

		// User2 should NOT receive the fill (only tick_snapshot)
		msg2, err := client2.WaitForMessage("fill", 1*time.Second)
		if err == nil && msg2.Type == "fill" {
			t.Error("User2 should NOT have received the fill message")
		}
	})
}

func TestWebSocket_ReconnectAfterDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Setup test environment
	env := SetupTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	// Create WebSocket server
	server := NewWSTestServer(t, env)
	defer server.Close()

	// Create test user
	passwordHash, _ := server.Auth.HashPassword("testpassword123")
	userID := env.CreateTestUser(ctx, t, "reconnect@example.com", passwordHash)

	// Create contest
	contestID := env.CreateTestContest(ctx, t, "Reconnect Test Contest", "running")
	env.AddContestSymbol(ctx, t, contestID, "AAPL")
	env.JoinContest(ctx, t, contestID, userID, 100000)

	// Generate token
	tokenPair, _ := server.Auth.Token.GenerateTokenPair(userID, []string{"user"})

	t.Run("ReconnectSuccessfully", func(t *testing.T) {
		// First connection
		client1 := NewWSClient(t, server.Server.URL, contestID, tokenPair.AccessToken)
		client1.WaitForMessage("contest_state", 5*time.Second)

		// Verify connected
		if server.GetConnectionCount() != 1 {
			t.Errorf("Expected 1 connection, got %d", server.GetConnectionCount())
		}

		// Disconnect
		client1.Close()

		// Wait for disconnection
		time.Sleep(500 * time.Millisecond)

		// Verify disconnected
		if server.GetConnectionCount() != 0 {
			t.Errorf("Expected 0 connections, got %d", server.GetConnectionCount())
		}

		// Reconnect
		client2 := NewWSClient(t, server.Server.URL, contestID, tokenPair.AccessToken)
		defer client2.Close()

		msg, err := client2.WaitForMessage("contest_state", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to receive welcome message after reconnect: %v", err)
		}

		if msg.Phase != "CONNECTING" {
			t.Errorf("Expected phase CONNECTING, got %s", msg.Phase)
		}

		// Verify reconnected
		time.Sleep(100 * time.Millisecond)
		if server.GetConnectionCount() != 1 {
			t.Errorf("Expected 1 connection after reconnect, got %d", server.GetConnectionCount())
		}
	})
}
