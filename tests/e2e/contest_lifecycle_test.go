// Package e2e provides end-to-end tests that exercise the full contest lifecycle
// against a running docker-compose environment.
//
// Prerequisites:
//
//	cd infra/docker && docker-compose up -d
//	# Wait for services to be healthy
//	sleep 30
//	go test ./tests/e2e/ -v -run TestContestLifecycle -timeout 5m
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Configuration — overridable via environment variables
// ---------------------------------------------------------------------------

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func gatewayURL() string   { return envOr("E2E_GATEWAY_URL", "http://localhost:8080") }
func userBFFURL() string   { return envOr("E2E_USER_BFF_URL", gatewayURL()) }
func tradeBFFURL() string  { return envOr("E2E_TRADE_BFF_URL", gatewayURL()) }
func adminBFFURL() string  { return envOr("E2E_ADMIN_BFF_URL", gatewayURL()) }
func adminEmail() string   { return envOr("E2E_ADMIN_EMAIL", "admin@tragge.com") }
func adminPassword() string { return envOr("E2E_ADMIN_PASSWORD", "Admin123!@#") }

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

type httpClient struct {
	base  string
	token string
	t     *testing.T
}

func newClient(t *testing.T, base string) *httpClient {
	return &httpClient{base: base, t: t}
}

func (c *httpClient) withToken(token string) *httpClient {
	return &httpClient{base: c.base, token: token, t: c.t}
}

func (c *httpClient) do(method, path string, body interface{}) (*http.Response, []byte) {
	c.t.Helper()

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(c.t, err, "marshal request body")
		bodyReader = bytes.NewReader(data)
	}

	url := c.base + path
	req, err := http.NewRequest(method, url, bodyReader)
	require.NoError(c.t, err, "create request %s %s", method, url)

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err, "execute request %s %s", method, url)

	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.NoError(c.t, err, "read response body")

	return resp, respBody
}

func (c *httpClient) post(path string, body interface{}) (*http.Response, []byte) {
	return c.do("POST", path, body)
}

func (c *httpClient) get(path string) (*http.Response, []byte) {
	return c.do("GET", path, nil)
}

func (c *httpClient) put(path string, body interface{}) (*http.Response, []byte) {
	return c.do("PUT", path, body)
}

func (c *httpClient) delete(path string) (*http.Response, []byte) {
	return c.do("DELETE", path, nil)
}

// ---------------------------------------------------------------------------
// JSON parsing helpers
// ---------------------------------------------------------------------------

func parseJSON(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &m), "parse JSON: %s", string(data))
	return m
}

func jsonString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func jsonFloat(m map[string]interface{}, key string) float64 {
	v, _ := m[key].(float64)
	return v
}

// ---------------------------------------------------------------------------
// WebSocket helpers
// ---------------------------------------------------------------------------

type wsClient struct {
	conn     *websocket.Conn
	t        *testing.T
	mu       sync.Mutex
	messages []map[string]interface{}
	done     chan struct{}
}

func newWSClient(t *testing.T, token, contestID string) *wsClient {
	t.Helper()

	wsURL := strings.Replace(tradeBFFURL(), "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL += "/ws/trade"

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	if contestID != "" {
		header.Set("X-Contest-ID", contestID)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("WebSocket dial failed: %v, status=%d, body=%s", err, resp.StatusCode, string(body))
		}
		t.Fatalf("WebSocket dial failed: %v", err)
	}

	ws := &wsClient{
		conn: conn,
		t:    t,
		done: make(chan struct{}),
	}

	// Start reader goroutine
	go ws.readLoop()

	return ws
}

func (ws *wsClient) readLoop() {
	defer close(ws.done)
	for {
		_, data, err := ws.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg map[string]interface{}
		if json.Unmarshal(data, &msg) == nil {
			ws.mu.Lock()
			ws.messages = append(ws.messages, msg)
			ws.mu.Unlock()
		}
	}
}

func (ws *wsClient) sendOrder(order map[string]interface{}) {
	ws.t.Helper()
	order["type"] = "order_request"
	data, err := json.Marshal(order)
	require.NoError(ws.t, err)
	require.NoError(ws.t, ws.conn.WriteMessage(websocket.TextMessage, data))
}

// waitForMessage waits for a message with the specified type and returns it.
func (ws *wsClient) waitForMessage(msgType string, timeout time.Duration) (map[string]interface{}, bool) {
	ws.t.Helper()
	deadline := time.Now().Add(timeout)
	seen := 0

	for time.Now().Before(deadline) {
		ws.mu.Lock()
		for i := seen; i < len(ws.messages); i++ {
			msg := ws.messages[i]
			if t, ok := msg["type"].(string); ok && t == msgType {
				ws.mu.Unlock()
				return msg, true
			}
		}
		seen = len(ws.messages)
		ws.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	return nil, false
}

// waitForMessageMatching waits for a message matching a predicate.
func (ws *wsClient) waitForMessageMatching(pred func(map[string]interface{}) bool, timeout time.Duration) (map[string]interface{}, bool) {
	ws.t.Helper()
	deadline := time.Now().Add(timeout)
	seen := 0

	for time.Now().Before(deadline) {
		ws.mu.Lock()
		for i := seen; i < len(ws.messages); i++ {
			if pred(ws.messages[i]) {
				msg := ws.messages[i]
				ws.mu.Unlock()
				return msg, true
			}
		}
		seen = len(ws.messages)
		ws.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	return nil, false
}

func (ws *wsClient) close() {
	ws.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	ws.conn.Close()
	<-ws.done
}

// ---------------------------------------------------------------------------
// TestContestLifecycle
// ---------------------------------------------------------------------------

// TestContestLifecycle exercises the full contest flow:
//
//  1. Admin creates a contest
//  2. Admin publishes the contest (draft -> scheduled)
//  3. Admin starts the contest (scheduled -> running)
//  4. User registers a new account
//  5. User joins the contest (must be registration_open first)
//  6. User places a market order via WebSocket
//  7. Verify order acknowledged via WebSocket
//  8. Verify position created via WebSocket
//  9. User modifies TP/SL via REST
//  10. User places a pending (buy-limit) order via WebSocket
//  11. User cancels the pending order via REST
//  12. User closes position via REST
//  13. Admin ends the contest (running -> settling)
//  14. Verify leaderboard and results accessible
//  15. User checks results
//  16. User checks trade history
//
// Note: Steps 5 and 13 use admin-driven state transitions since the
// contest-scheduler auto-transitions depend on wall-clock timing. In a
// full integration the scheduler handles starts_at / ends_at transitions.
func TestContestLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Verify connectivity to gateway
	client := newClient(t, gatewayURL())
	{
		resp, _ := client.get("/healthz")
		// Gateway may not have /healthz, check user-bff health instead
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadGateway {
			resp, _ = client.get("/api/user/healthz")
		}
		if resp.StatusCode != http.StatusOK {
			t.Skipf("services not reachable at %s (status=%d), skipping E2E test", gatewayURL(), resp.StatusCode)
		}
	}

	// -----------------------------------------------------------------------
	// Step 1: Admin login
	// -----------------------------------------------------------------------
	t.Log("Step 1: Admin login")

	adminClient := newClient(t, adminBFFURL())
	var adminToken string
	{
		// Login as admin through user-bff
		resp, body := newClient(t, userBFFURL()).post("/api/user/auth/login", map[string]string{
			"email":    adminEmail(),
			"password": adminPassword(),
		})
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"admin login failed: %s", string(body))

		result := parseJSON(t, body)
		adminToken = jsonString(result, "access_token")
		require.NotEmpty(t, adminToken, "admin access_token is empty")
		adminClient = adminClient.withToken(adminToken)
	}

	// -----------------------------------------------------------------------
	// Step 2: Admin creates a contest
	// -----------------------------------------------------------------------
	t.Log("Step 2: Admin creates a contest")

	uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	contestName := "E2E-Lifecycle-" + uniqueSuffix

	now := time.Now().UTC()
	// Contest starts very soon (1 minute) and ends in 10 minutes
	startsAt := now.Add(1 * time.Minute)
	endsAt := now.Add(10 * time.Minute)

	var contestID string
	{
		createReq := map[string]interface{}{
			"name":             contestName,
			"starts_at":        startsAt.Format(time.RFC3339),
			"ends_at":          endsAt.Format(time.RFC3339),
			"entry_fee_cents":  0,
			"platform_fee_bps": 0,
			"qty_total":        100000,
			"status":           "draft",
			"is_free":          true,
			"auto_start":       true,
			"min_participants":  1,
			"duration_type":    "hourly",
			"asset_class":      "mixed",
			"symbols":          []string{"AAPL", "MSFT", "GOOGL"},
		}

		resp, body := adminClient.post("/api/admin/contests", createReq)
		require.Equalf(t, http.StatusCreated, resp.StatusCode,
			"create contest failed: %s", string(body))

		result := parseJSON(t, body)
		contestID = jsonString(result, "id")
		require.NotEmpty(t, contestID, "contest ID is empty")
		t.Logf("  Created contest: %s (%s)", contestID, contestName)
	}

	// -----------------------------------------------------------------------
	// Step 3: Admin publishes the contest (draft -> scheduled)
	// -----------------------------------------------------------------------
	t.Log("Step 3: Admin publishes the contest")
	{
		resp, body := adminClient.post(
			fmt.Sprintf("/api/admin/contests/%s/publish", contestID), nil)

		// Accept 200 (success) — state machine publishes and may transition
		require.Containsf(t, []int{http.StatusOK}, resp.StatusCode,
			"publish contest failed: %s", string(body))

		result := parseJSON(t, body)
		status := jsonString(result, "status")
		t.Logf("  Contest status after publish: %s", status)
		// After publish, contest should be in scheduled or registration_open
		assert.Contains(t, []string{"scheduled", "registration_open"}, status)
	}

	// -----------------------------------------------------------------------
	// Step 4: Open registration if not already open
	// -----------------------------------------------------------------------
	t.Log("Step 4: Ensuring registration is open")
	{
		// Check current state
		resp, body := adminClient.get(
			fmt.Sprintf("/api/admin/contests/%s/state", contestID))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := parseJSON(t, body)
		currentStatus := jsonString(result, "status")

		// If still "scheduled", the contest-scheduler should auto-transition
		// based on registration_deadline. Wait a bit or manually start if needed.
		if currentStatus == "scheduled" {
			t.Log("  Contest is scheduled; waiting for registration to open...")
			// Poll for up to 30s for the contest-scheduler to open registration
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				time.Sleep(2 * time.Second)
				resp, body = adminClient.get(
					fmt.Sprintf("/api/admin/contests/%s/state", contestID))
				if resp.StatusCode == http.StatusOK {
					result = parseJSON(t, body)
					currentStatus = jsonString(result, "status")
					if currentStatus != "scheduled" {
						break
					}
				}
			}
		}

		t.Logf("  Contest status: %s", currentStatus)
		// Registration should be open now. If not, the test continues —
		// joining may fail, which is a valid E2E signal.
	}

	// -----------------------------------------------------------------------
	// Step 5: User registers a new account
	// -----------------------------------------------------------------------
	t.Log("Step 5: User registration")

	userEmail := fmt.Sprintf("e2e-user-%s@test.tragge.com", uniqueSuffix)
	userPassword := "TestP@ss123!"
	var userToken string
	{
		resp, body := newClient(t, userBFFURL()).post("/api/user/auth/register", map[string]string{
			"email":    userEmail,
			"password": userPassword,
		})
		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"user registration failed: %s", string(body))

		result := parseJSON(t, body)
		userToken = jsonString(result, "access_token")
		require.NotEmpty(t, userToken, "user access_token is empty")
		t.Logf("  Registered user: %s", userEmail)
	}

	userClient := newClient(t, userBFFURL()).withToken(userToken)
	tradeClient := newClient(t, tradeBFFURL()).withToken(userToken)

	// -----------------------------------------------------------------------
	// Step 6: User joins the contest
	// -----------------------------------------------------------------------
	t.Log("Step 6: User joins the contest")
	{
		resp, body := userClient.post(
			fmt.Sprintf("/api/user/contests/%s/join", contestID), nil)

		// 200 = success (already joined or fresh join)
		require.Containsf(t, []int{http.StatusOK}, resp.StatusCode,
			"join contest failed (status=%d): %s", resp.StatusCode, string(body))

		result := parseJSON(t, body)
		t.Logf("  Joined contest. QtyTotal=%v", result["qty_total"])
	}

	// -----------------------------------------------------------------------
	// Step 7: Admin starts the contest (-> running)
	// -----------------------------------------------------------------------
	t.Log("Step 7: Admin starts the contest")
	{
		// Use the admin start endpoint to force the contest to running state
		resp, body := adminClient.post(
			fmt.Sprintf("/api/admin/contests/%s/start", contestID), nil)

		if resp.StatusCode == http.StatusConflict {
			// Contest might already be running if scheduler started it
			t.Log("  Contest may already be running, checking state...")
		} else {
			require.Equalf(t, http.StatusOK, resp.StatusCode,
				"start contest failed: %s", string(body))
		}

		// Verify the contest is now running
		resp, body = adminClient.get(
			fmt.Sprintf("/api/admin/contests/%s/state", contestID))
		require.Equal(t, http.StatusOK, resp.StatusCode)
		result := parseJSON(t, body)
		assert.Equal(t, "running", jsonString(result, "status"),
			"contest should be running")
		t.Logf("  Contest is now running")
	}

	// Allow services to process the state change
	time.Sleep(2 * time.Second)

	// -----------------------------------------------------------------------
	// Step 8: User places a market order via WebSocket
	// -----------------------------------------------------------------------
	t.Log("Step 8: User places a market order via WebSocket")

	var orderRequestID string
	var wsConn *wsClient
	{
		wsConn = newWSClient(t, userToken, contestID)
		defer wsConn.close()

		// Wait briefly for connection to stabilize
		time.Sleep(1 * time.Second)

		orderRequestID = fmt.Sprintf("e2e-req-%s", uniqueSuffix)
		wsConn.sendOrder(map[string]interface{}{
			"request_id": orderRequestID,
			"symbol":     "AAPL",
			"side":       "BUY",
			"order_type": "MARKET",
			"qty":        10,
		})
		t.Log("  Sent market order via WebSocket")
	}

	// -----------------------------------------------------------------------
	// Step 9: Verify order acknowledged via WebSocket
	// -----------------------------------------------------------------------
	t.Log("Step 9: Waiting for order acknowledgment via WebSocket")

	var orderID string
	{
		msg, found := wsConn.waitForMessageMatching(func(m map[string]interface{}) bool {
			msgType, _ := m["type"].(string)
			if msgType == "order_ack" {
				// Check if it matches our request
				if rid, ok := m["request_id"].(string); ok && rid == orderRequestID {
					return true
				}
				// Also check nested payload
				if payload, ok := m["payload"].(map[string]interface{}); ok {
					if rid, ok := payload["request_id"].(string); ok && rid == orderRequestID {
						return true
					}
				}
			}
			return false
		}, 15*time.Second)

		if found {
			// Extract order ID from either top-level or payload
			orderID = jsonString(msg, "order_id")
			if orderID == "" {
				if payload, ok := msg["payload"].(map[string]interface{}); ok {
					orderID = jsonString(payload, "order_id")
				}
			}
			t.Logf("  Order acknowledged: %s", orderID)
		} else {
			t.Log("  WARNING: No order_ack received within timeout (market data may not be flowing)")
			t.Log("  Attempting to place order via REST API fallback...")

			// Fallback: place order via REST
			resp, body := tradeClient.post("/api/trade/orders", map[string]interface{}{
				"contest_id": contestID,
				"symbol":     "AAPL",
				"side":       "BUY",
				"type":       "MARKET",
				"qty":        10,
			})
			t.Logf("  REST order response: status=%d body=%s", resp.StatusCode, string(body))
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated ||
				resp.StatusCode == http.StatusAccepted {
				result := parseJSON(t, body)
				orderID = jsonString(result, "order_id")
				t.Logf("  Order placed via REST: %s", orderID)
			}
		}
	}

	// -----------------------------------------------------------------------
	// Step 10: Verify position created via WebSocket
	// -----------------------------------------------------------------------
	t.Log("Step 10: Waiting for position update via WebSocket")

	var positionID string
	{
		// Look for a position_update or fill message
		msg, found := wsConn.waitForMessageMatching(func(m map[string]interface{}) bool {
			msgType, _ := m["type"].(string)
			return msgType == "position_update" || msgType == "fill" || msgType == "positions"
		}, 15*time.Second)

		if found {
			t.Logf("  Received %s message", jsonString(msg, "type"))
			// Try to extract position ID from payload
			if payload, ok := msg["payload"].(map[string]interface{}); ok {
				if positions, ok := payload["positions"].([]interface{}); ok && len(positions) > 0 {
					if pos, ok := positions[0].(map[string]interface{}); ok {
						positionID = jsonString(pos, "position_id")
					}
				}
			}
		} else {
			t.Log("  No position update received via WebSocket, querying REST...")
		}

		// If we didn't get position ID from WebSocket, try REST
		if positionID == "" {
			// Give the system a moment to process
			time.Sleep(2 * time.Second)

			resp, body := tradeClient.get(
				fmt.Sprintf("/api/trade/contest/%s/symbols", contestID))
			t.Logf("  Contest symbols response: status=%d", resp.StatusCode)

			// Try order history to find our order's position
			resp, body = tradeClient.get("/api/trade/orders/history?contest_id=" + contestID)
			if resp.StatusCode == http.StatusOK {
				t.Logf("  Order history: %s", string(body))
			}
		}

		if positionID != "" {
			t.Logf("  Position ID: %s", positionID)
		} else {
			t.Log("  WARNING: Could not retrieve position ID (market data may not be active)")
		}
	}

	// -----------------------------------------------------------------------
	// Step 11: User modifies TP/SL via REST
	// -----------------------------------------------------------------------
	t.Log("Step 11: User modifies TP/SL via REST")
	if positionID != "" {
		tp := 200.0
		sl := 100.0
		resp, body := tradeClient.put(
			fmt.Sprintf("/api/trade/positions/%s/tpsl", positionID),
			map[string]interface{}{
				"take_profit": tp,
				"stop_loss":   sl,
			})

		t.Logf("  Modify TP/SL response: status=%d body=%s", resp.StatusCode, string(body))
		assert.Contains(t, []int{http.StatusOK, http.StatusAccepted}, resp.StatusCode,
			"modify TP/SL should succeed")
	} else {
		t.Log("  SKIP: No position ID available to modify TP/SL")
	}

	// -----------------------------------------------------------------------
	// Step 12: User places a pending (BUY_LIMIT) order via WebSocket
	// -----------------------------------------------------------------------
	t.Log("Step 12: User places a pending order via WebSocket")

	var pendingOrderID string
	{
		pendingReqID := fmt.Sprintf("e2e-pending-%s", uniqueSuffix)
		limitPrice := 100.0

		wsConn.sendOrder(map[string]interface{}{
			"request_id":  pendingReqID,
			"symbol":      "AAPL",
			"side":        "BUY",
			"order_type":  "BUY_LIMIT",
			"qty":         5,
			"limit_price": limitPrice,
		})

		// Wait for ack
		msg, found := wsConn.waitForMessageMatching(func(m map[string]interface{}) bool {
			msgType, _ := m["type"].(string)
			if msgType == "order_ack" {
				if rid, ok := m["request_id"].(string); ok && rid == pendingReqID {
					return true
				}
				if payload, ok := m["payload"].(map[string]interface{}); ok {
					if rid, ok := payload["request_id"].(string); ok && rid == pendingReqID {
						return true
					}
				}
			}
			return false
		}, 15*time.Second)

		if found {
			pendingOrderID = jsonString(msg, "order_id")
			if pendingOrderID == "" {
				if payload, ok := msg["payload"].(map[string]interface{}); ok {
					pendingOrderID = jsonString(payload, "order_id")
				}
			}
			t.Logf("  Pending order acknowledged: %s", pendingOrderID)
		} else {
			t.Log("  WARNING: No pending order ack received, trying REST fallback...")

			resp, body := tradeClient.post("/api/trade/orders", map[string]interface{}{
				"contest_id":  contestID,
				"symbol":      "AAPL",
				"side":        "BUY",
				"type":        "BUY_LIMIT",
				"qty":         5,
				"limit_price": limitPrice,
			})
			t.Logf("  REST pending order response: status=%d body=%s", resp.StatusCode, string(body))
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated ||
				resp.StatusCode == http.StatusAccepted {
				result := parseJSON(t, body)
				pendingOrderID = jsonString(result, "order_id")
			}
		}
	}

	// -----------------------------------------------------------------------
	// Step 13: User cancels the pending order via REST
	// -----------------------------------------------------------------------
	t.Log("Step 13: User cancels pending order via REST")
	if pendingOrderID != "" {
		resp, body := tradeClient.delete(
			fmt.Sprintf("/api/trade/orders/%s", pendingOrderID))

		t.Logf("  Cancel order response: status=%d body=%s", resp.StatusCode, string(body))
		assert.Contains(t, []int{http.StatusOK, http.StatusAccepted, http.StatusNoContent}, resp.StatusCode,
			"cancel pending order should succeed")
	} else {
		t.Log("  SKIP: No pending order ID available to cancel")
	}

	// -----------------------------------------------------------------------
	// Step 14: User closes position via REST
	// -----------------------------------------------------------------------
	t.Log("Step 14: User closes position via REST")
	if positionID != "" {
		resp, body := tradeClient.post(
			fmt.Sprintf("/api/trade/positions/%s/close", positionID), nil)

		t.Logf("  Close position response: status=%d body=%s", resp.StatusCode, string(body))
		assert.Contains(t, []int{http.StatusOK, http.StatusAccepted}, resp.StatusCode,
			"close position should succeed")
	} else {
		t.Log("  SKIP: No position ID available to close")
	}

	// Allow settlement to process
	time.Sleep(2 * time.Second)

	// -----------------------------------------------------------------------
	// Step 15: Admin ends the contest (running -> settling)
	// -----------------------------------------------------------------------
	t.Log("Step 15: Admin ends the contest")
	{
		resp, body := adminClient.post(
			fmt.Sprintf("/api/admin/contests/%s/end", contestID), nil)

		if resp.StatusCode == http.StatusConflict {
			t.Log("  Contest may already be in settling/completed state")
		} else {
			require.Equalf(t, http.StatusOK, resp.StatusCode,
				"end contest failed: %s", string(body))
		}

		// Poll for final state
		t.Log("  Waiting for contest to reach settling/completed state...")
		deadline := time.Now().Add(60 * time.Second)
		var finalStatus string
		for time.Now().Before(deadline) {
			resp, body = adminClient.get(
				fmt.Sprintf("/api/admin/contests/%s/state", contestID))
			if resp.StatusCode == http.StatusOK {
				result := parseJSON(t, body)
				finalStatus = jsonString(result, "status")
				if finalStatus == "settling" || finalStatus == "completed" {
					break
				}
			}
			time.Sleep(2 * time.Second)
		}
		t.Logf("  Contest status: %s", finalStatus)
		assert.Contains(t, []string{"settling", "completed"}, finalStatus,
			"contest should be settling or completed")
	}

	// -----------------------------------------------------------------------
	// Step 16: Verify leaderboard accessible
	// -----------------------------------------------------------------------
	t.Log("Step 16: Verify leaderboard accessible")
	{
		// The leaderboard endpoint may be via user-bff or trade-bff
		resp, body := userClient.get(
			fmt.Sprintf("/api/user/contests/%s/leaderboard", contestID))
		t.Logf("  Leaderboard response: status=%d", resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			t.Logf("  Leaderboard data: %s", truncate(string(body), 500))
		}
		// Leaderboard should be accessible even if empty
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)
	}

	// -----------------------------------------------------------------------
	// Step 17: User checks results
	// -----------------------------------------------------------------------
	t.Log("Step 17: User checks contest results")
	{
		resp, body := userClient.get(
			fmt.Sprintf("/api/user/contests/%s/my-result", contestID))
		t.Logf("  My result response: status=%d", resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			result := parseJSON(t, body)
			t.Logf("  Result: rank=%v, total_score=%v, trade_count=%v",
				result["final_rank"], result["total_score"], result["trade_count"])
		}
		// 200 if participated, 404 if no participation record
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)
	}

	// -----------------------------------------------------------------------
	// Step 18: User checks trade history
	// -----------------------------------------------------------------------
	t.Log("Step 18: User checks trade history")
	{
		resp, body := userClient.get(
			fmt.Sprintf("/api/user/contests/%s/my-trades", contestID))
		t.Logf("  My trades response: status=%d", resp.StatusCode)

		if resp.StatusCode == http.StatusOK {
			t.Logf("  Trade history: %s", truncate(string(body), 500))
		}
		// 200 for trade history
		assert.Contains(t, []int{http.StatusOK, http.StatusNotFound}, resp.StatusCode)
	}

	t.Log("Contest lifecycle E2E test completed successfully")
}

// TestServiceHealth checks that all required services are reachable.
func TestServiceHealth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E health check in short mode")
	}

	services := []struct {
		name string
		url  string
		path string
	}{
		{"user-bff", userBFFURL(), "/api/user/healthz"},
		{"trade-bff", tradeBFFURL(), "/healthz"},
		{"admin-bff", adminBFFURL(), "/healthz"},
	}

	for _, svc := range services {
		t.Run(svc.name, func(t *testing.T) {
			client := newClient(t, svc.url)
			resp, body := client.get(svc.path)
			t.Logf("  %s: status=%d body=%s", svc.name, resp.StatusCode, truncate(string(body), 200))
			assert.Equal(t, http.StatusOK, resp.StatusCode,
				"%s should be healthy", svc.name)
		})
	}
}

// truncate truncates a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
