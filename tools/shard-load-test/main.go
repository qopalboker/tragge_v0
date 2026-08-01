// Package main provides a shard-aware load testing tool for the tragge trading platform.
// It simulates distributed trading across multiple shards and measures per-shard performance.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
)

// Config holds the shard load test configuration.
type Config struct {
	// Shard settings
	ShardCount       int
	ContestsPerShard int

	// Connection settings
	NumUsersPerShard int
	UserBFFURL       string
	TradeBFFWSURL    string
	TradeBFFAPIURL   string
	ShardRouterURL   string

	// Authentication
	EmailPrefix string
	Password    string

	// Test settings
	Duration       time.Duration
	OrdersPerUser  int
	OrderInterval  time.Duration
	Symbols        []string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration

	// Output settings
	Verbose bool
}

// ShardMetrics tracks metrics for a single shard.
type ShardMetrics struct {
	ShardID int

	// Order metrics
	ordersSent     atomic.Int64
	ordersAccepted atomic.Int64
	ordersRejected atomic.Int64
	fillsReceived  atomic.Int64

	// Latency tracking
	orderLatencies []time.Duration
	latencyMu      sync.Mutex

	// Error metrics
	orderErrors atomic.Int64
	wsErrors    atomic.Int64

	// Timing
	startTime time.Time
}

// GlobalMetrics aggregates metrics across all shards.
type GlobalMetrics struct {
	shardMetrics []*ShardMetrics
	startTime    time.Time
	mu           sync.Mutex
}

// User represents a simulated user.
type User struct {
	ID          string
	Email       string
	AccessToken string
	ShardID     int
	ContestID   string
	WSConn      *websocket.Conn
	OrderTimes  sync.Map
}

// Contest represents a test contest on a shard.
type Contest struct {
	ID      string
	Name    string
	ShardID int
}

// ShardInfo represents shard discovery information.
type ShardInfo struct {
	ShardID int    `json:"shard_id"`
	Address string `json:"address"`
	Status  string `json:"status"`
}

// AuthResponse represents the authentication response.
type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// OrderSubmitRequest represents the order submission request.
type OrderSubmitRequest struct {
	ContestID  string              `json:"contest_id"`
	Symbol     string              `json:"symbol"`
	Side       contracts.OrderSide `json:"side"`
	Type       contracts.OrderType `json:"type"`
	Qty        int64               `json:"qty"`
	LimitPrice *float64            `json:"limit_price,omitempty"`
}

// OrderResponse represents the order submission response.
type OrderResponse struct {
	OrderID string `json:"order_id"`
}

// WSMessage represents a WebSocket message.
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Results holds the final test results.
type Results struct {
	TotalDuration time.Duration
	GlobalStats   GlobalStats
	ShardStats    []ShardStats
}

// GlobalStats holds aggregate statistics.
type GlobalStats struct {
	TotalOrdersSent   int64
	TotalFillsReceived int64
	TotalOrderErrors  int64
	TotalWSErrors     int64
	OrdersPerSecond   float64
	FillRate          float64
}

// ShardStats holds per-shard statistics.
type ShardStats struct {
	ShardID         int
	OrdersSent      int64
	FillsReceived   int64
	OrderErrors     int64
	WSErrors        int64
	OrdersPerSecond float64
	FillRate        float64
	LatencyP50      time.Duration
	LatencyP95      time.Duration
	LatencyP99      time.Duration
	LatencyAvg      time.Duration
	LatencyMin      time.Duration
	LatencyMax      time.Duration
}

// ShardLoadTester orchestrates the shard load test.
type ShardLoadTester struct {
	config   *Config
	metrics  *GlobalMetrics
	users    [][]*User // users per shard
	contests []Contest
}

func main() {
	cfg := parseFlags()

	tester := NewShardLoadTester(cfg)
	results, err := tester.Run()
	if err != nil {
		log.Fatalf("Shard load test failed: %v", err)
	}

	printResults(results)
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.IntVar(&cfg.ShardCount, "shards", 4, "Number of shards")
	flag.IntVar(&cfg.ContestsPerShard, "contests-per-shard", 1, "Number of contests per shard")
	flag.IntVar(&cfg.NumUsersPerShard, "users-per-shard", 10, "Number of users per shard")
	flag.StringVar(&cfg.UserBFFURL, "user-bff", "http://localhost:8081", "User BFF base URL")
	flag.StringVar(&cfg.TradeBFFWSURL, "trade-bff-ws", "ws://localhost:8082", "Trade BFF WebSocket base URL")
	flag.StringVar(&cfg.TradeBFFAPIURL, "trade-bff-api", "http://localhost:8082", "Trade BFF API base URL")
	flag.StringVar(&cfg.ShardRouterURL, "shard-router", "http://localhost:8090", "Shard router base URL")
	flag.StringVar(&cfg.EmailPrefix, "email-prefix", "shardtest", "Email prefix for test users")
	flag.StringVar(&cfg.Password, "password", "shardtest123!", "Password for test users")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.IntVar(&cfg.OrdersPerUser, "orders-per-user", 10, "Number of orders each user places")
	flag.DurationVar(&cfg.OrderInterval, "order-interval", 1*time.Second, "Interval between orders")
	flag.DurationVar(&cfg.ConnectTimeout, "connect-timeout", 10*time.Second, "Connection timeout")
	flag.DurationVar(&cfg.ReadTimeout, "read-timeout", 65*time.Second, "Read timeout")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose output")

	var symbolsStr string
	flag.StringVar(&symbolsStr, "symbols", "AAPL,GOOGL,MSFT,AMZN,TSLA", "Trading symbols")

	flag.Parse()

	cfg.Symbols = parseSymbols(symbolsStr)

	return cfg
}

func parseSymbols(s string) []string {
	var symbols []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				sym := s[start:i]
				for len(sym) > 0 && sym[0] == ' ' {
					sym = sym[1:]
				}
				for len(sym) > 0 && sym[len(sym)-1] == ' ' {
					sym = sym[:len(sym)-1]
				}
				if len(sym) > 0 {
					symbols = append(symbols, sym)
				}
			}
			start = i + 1
		}
	}
	return symbols
}

// NewShardLoadTester creates a new shard load tester.
func NewShardLoadTester(cfg *Config) *ShardLoadTester {
	shardMetrics := make([]*ShardMetrics, cfg.ShardCount)
	for i := 0; i < cfg.ShardCount; i++ {
		shardMetrics[i] = &ShardMetrics{
			ShardID:        i,
			orderLatencies: make([]time.Duration, 0),
		}
	}

	return &ShardLoadTester{
		config: cfg,
		metrics: &GlobalMetrics{
			shardMetrics: shardMetrics,
			startTime:    time.Now(),
		},
		users:    make([][]*User, cfg.ShardCount),
		contests: make([]Contest, 0),
	}
}

// Run executes the shard load test.
func (t *ShardLoadTester) Run() (*Results, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("\nReceived shutdown signal, stopping test...")
		cancel()
	}()

	// Print configuration
	t.printConfig()

	// Step 1: Create contests on each shard
	fmt.Printf("Creating %d contests per shard across %d shards...\n",
		t.config.ContestsPerShard, t.config.ShardCount)
	if err := t.createContests(ctx); err != nil {
		return nil, fmt.Errorf("failed to create contests: %w", err)
	}
	fmt.Printf("Created %d contests total\n", len(t.contests))

	// Step 2: Create and authenticate users for each shard
	fmt.Printf("Creating and authenticating %d users per shard...\n", t.config.NumUsersPerShard)
	if err := t.createUsers(ctx); err != nil {
		return nil, fmt.Errorf("failed to create users: %w", err)
	}

	totalUsers := 0
	for _, shardUsers := range t.users {
		totalUsers += len(shardUsers)
	}
	fmt.Printf("Successfully authenticated %d users total\n", totalUsers)

	// Step 3: Connect users to WebSocket
	fmt.Println("Connecting users to WebSocket...")
	if err := t.connectUsersToWS(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect users: %w", err)
	}

	connectedUsers := 0
	for _, shardUsers := range t.users {
		for _, u := range shardUsers {
			if u.WSConn != nil {
				connectedUsers++
			}
		}
	}
	fmt.Printf("Connected %d users to WebSocket\n", connectedUsers)

	// Cleanup WebSocket connections at end
	defer t.cleanupConnections()

	// Step 4: Start WebSocket message readers
	fmt.Println("Starting WebSocket message handlers...")
	var wg sync.WaitGroup
	for shardID, shardUsers := range t.users {
		for _, user := range shardUsers {
			if user.WSConn == nil {
				continue
			}
			wg.Add(1)
			go func(u *User, sID int) {
				defer wg.Done()
				t.readWSMessages(ctx, u, t.metrics.shardMetrics[sID])
			}(user, shardID)
		}
	}

	// Step 5: Start placing orders
	fmt.Printf("Starting order placement (duration: %s)...\n", t.config.Duration)
	t.metrics.startTime = time.Now()
	for _, sm := range t.metrics.shardMetrics {
		sm.startTime = time.Now()
	}

	testCtx, testCancel := context.WithTimeout(ctx, t.config.Duration)
	defer testCancel()

	eg, egCtx := errgroup.WithContext(testCtx)
	eg.SetLimit(totalUsers * 2)

	for shardID, shardUsers := range t.users {
		for _, user := range shardUsers {
			if user.WSConn == nil {
				continue
			}
			user := user
			shardID := shardID
			eg.Go(func() error {
				t.placeOrders(egCtx, user, t.metrics.shardMetrics[shardID])
				return nil
			})
		}
	}

	// Progress reporter
	go t.reportProgress(testCtx)

	// Wait for order placement to complete
	eg.Wait()

	// Wait for remaining fills
	fmt.Println("Waiting for remaining fills...")
	time.Sleep(3 * time.Second)

	// Cancel context to stop WS readers
	cancel()
	wg.Wait()

	// Calculate results
	return t.calculateResults(), nil
}

func (t *ShardLoadTester) printConfig() {
	fmt.Println("=== Shard Load Test ===")
	fmt.Printf("Shards:            %d\n", t.config.ShardCount)
	fmt.Printf("Contests/Shard:    %d\n", t.config.ContestsPerShard)
	fmt.Printf("Users/Shard:       %d\n", t.config.NumUsersPerShard)
	fmt.Printf("Orders/User:       %d\n", t.config.OrdersPerUser)
	fmt.Printf("Order Interval:    %s\n", t.config.OrderInterval)
	fmt.Printf("Duration:          %s\n", t.config.Duration)
	fmt.Printf("Symbols:           %v\n", t.config.Symbols)
	fmt.Printf("User BFF:          %s\n", t.config.UserBFFURL)
	fmt.Printf("Trade BFF API:     %s\n", t.config.TradeBFFAPIURL)
	fmt.Printf("Trade BFF WS:      %s\n", t.config.TradeBFFWSURL)
	fmt.Printf("Shard Router:      %s\n", t.config.ShardRouterURL)
	fmt.Println()
}

func (t *ShardLoadTester) createContests(ctx context.Context) error {
	client := &http.Client{Timeout: 30 * time.Second}

	for shardID := 0; shardID < t.config.ShardCount; shardID++ {
		for i := 0; i < t.config.ContestsPerShard; i++ {
			contestID := uuid.New().String()
			name := fmt.Sprintf("Shard %d Contest %d", shardID, i)

			// In a real scenario, you would create contests via the admin API
			// For this load test, we'll use mock contest IDs that correspond to shards
			contest := Contest{
				ID:      contestID,
				Name:    name,
				ShardID: shardID,
			}
			t.contests = append(t.contests, contest)

			if t.config.Verbose {
				log.Printf("Created contest %s on shard %d", contestID, shardID)
			}
		}
	}

	// Optionally, try to create contests via API if available
	for _, contest := range t.contests {
		err := t.createContestViaAPI(ctx, client, contest)
		if err != nil && t.config.Verbose {
			log.Printf("Note: Could not create contest via API (mock mode): %v", err)
		}
	}

	return nil
}

func (t *ShardLoadTester) createContestViaAPI(ctx context.Context, client *http.Client, contest Contest) error {
	body := map[string]interface{}{
		"name":        contest.Name,
		"shard_id":    contest.ShardID,
		"starts_at":   time.Now().Add(-time.Hour).Format(time.RFC3339),
		"ends_at":     time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"status":      "running",
		"qty_total":   100000,
		"symbols":     t.config.Symbols,
	}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		t.config.UserBFFURL+"/api/admin/contests", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create contest failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (t *ShardLoadTester) createUsers(ctx context.Context) error {
	client := &http.Client{Timeout: 30 * time.Second}

	for shardID := 0; shardID < t.config.ShardCount; shardID++ {
		shardContests := t.getContestsForShard(shardID)
		if len(shardContests) == 0 {
			continue
		}

		for i := 0; i < t.config.NumUsersPerShard; i++ {
			email := fmt.Sprintf("%s_shard%d_user%d@loadtest.example.com",
				t.config.EmailPrefix, shardID, i)

			user, err := t.registerOrLoginUser(ctx, client, email)
			if err != nil {
				if t.config.Verbose {
					log.Printf("Failed to auth user %s: %v", email, err)
				}
				continue
			}

			// Assign user to a contest on this shard
			contestIdx := i % len(shardContests)
			user.ShardID = shardID
			user.ContestID = shardContests[contestIdx].ID

			// Try to join contest
			if err := t.joinContest(ctx, client, user); err != nil {
				if t.config.Verbose {
					log.Printf("Failed to join contest for user %s: %v", email, err)
				}
			}

			t.users[shardID] = append(t.users[shardID], user)
		}
	}

	return nil
}

func (t *ShardLoadTester) getContestsForShard(shardID int) []Contest {
	var contests []Contest
	for _, c := range t.contests {
		if c.ShardID == shardID {
			contests = append(contests, c)
		}
	}
	return contests
}

func (t *ShardLoadTester) registerOrLoginUser(ctx context.Context, client *http.Client, email string) (*User, error) {
	// Try register first
	user, err := t.registerUser(ctx, client, email)
	if err != nil {
		// Try login
		user, err = t.loginUser(ctx, client, email)
		if err != nil {
			return nil, err
		}
	}
	return user, nil
}

func (t *ShardLoadTester) registerUser(ctx context.Context, client *http.Client, email string) (*User, error) {
	body := map[string]string{
		"email":    email,
		"password": t.config.Password,
	}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		t.config.UserBFFURL+"/api/user/auth/register", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("register failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, err
	}

	return &User{
		Email:       email,
		AccessToken: authResp.AccessToken,
	}, nil
}

func (t *ShardLoadTester) loginUser(ctx context.Context, client *http.Client, email string) (*User, error) {
	body := map[string]string{
		"email":    email,
		"password": t.config.Password,
	}
	data, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		t.config.UserBFFURL+"/api/user/auth/login", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("login failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var authResp AuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		return nil, err
	}

	return &User{
		Email:       email,
		AccessToken: authResp.AccessToken,
	}, nil
}

func (t *ShardLoadTester) joinContest(ctx context.Context, client *http.Client, user *User) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		t.config.UserBFFURL+"/api/user/contests/"+user.ContestID+"/join", nil)
	req.Header.Set("Authorization", "Bearer "+user.AccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("join contest failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (t *ShardLoadTester) connectUsersToWS(ctx context.Context) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 50)

	for shardID := range t.users {
		for _, user := range t.users[shardID] {
			wg.Add(1)
			go func(u *User) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				conn, err := t.connectToWS(ctx, u)
				if err != nil {
					t.metrics.shardMetrics[u.ShardID].wsErrors.Add(1)
					if t.config.Verbose {
						log.Printf("WS connection failed for %s: %v", u.Email, err)
					}
					return
				}
				u.WSConn = conn
			}(user)
		}
	}

	wg.Wait()
	return nil
}

func (t *ShardLoadTester) connectToWS(ctx context.Context, user *User) (*websocket.Conn, error) {
	wsURL, _ := url.Parse(t.config.TradeBFFWSURL + "/ws/trade")
	q := wsURL.Query()
	q.Set("contest_id", user.ContestID)
	wsURL.RawQuery = q.Encode()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+user.AccessToken)
	header.Set("X-Shard-ID", fmt.Sprintf("%d", user.ShardID))

	dialer := websocket.Dialer{
		HandshakeTimeout: t.config.ConnectTimeout,
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
	}

	conn, resp, err := dialer.DialContext(ctx, wsURL.String(), header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("WS dial failed (status %d): %s", resp.StatusCode, string(body))
		}
		return nil, err
	}
	if resp != nil {
		resp.Body.Close()
	}

	return conn, nil
}

func (t *ShardLoadTester) readWSMessages(ctx context.Context, user *User, metrics *ShardMetrics) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if user.WSConn == nil {
			return
		}

		user.WSConn.SetReadDeadline(time.Now().Add(t.config.ReadTimeout))
		_, message, err := user.WSConn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			continue
		}

		receiveTime := time.Now()

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		if msg.Type == "fill" {
			var fill contracts.FillEvent
			if err := json.Unmarshal(msg.Payload, &fill); err != nil {
				continue
			}

			metrics.fillsReceived.Add(1)

			if sendTimeVal, ok := user.OrderTimes.Load(fill.OrderID); ok {
				sendTime := sendTimeVal.(time.Time)
				latency := receiveTime.Sub(sendTime)

				metrics.latencyMu.Lock()
				metrics.orderLatencies = append(metrics.orderLatencies, latency)
				metrics.latencyMu.Unlock()
			}
		}
	}
}

func (t *ShardLoadTester) placeOrders(ctx context.Context, user *User, metrics *ShardMetrics) {
	client := &http.Client{Timeout: 30 * time.Second}

	for i := 0; i < t.config.OrdersPerUser; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		orderID := t.placeOrder(ctx, client, user, metrics)
		if orderID != "" {
			user.OrderTimes.Store(orderID, time.Now())
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(t.config.OrderInterval):
		}
	}
}

func (t *ShardLoadTester) placeOrder(ctx context.Context, client *http.Client, user *User, metrics *ShardMetrics) string {
	symbol := t.config.Symbols[rand.Intn(len(t.config.Symbols))]
	side := contracts.OrderSideBuy
	if rand.Intn(2) == 0 {
		side = contracts.OrderSideSell
	}
	qty := rand.Intn(100) + 1

	orderReq := OrderSubmitRequest{
		ContestID: user.ContestID,
		Symbol:    symbol,
		Side:      side,
		Type:      contracts.OrderTypeMarket,
		Qty:       qty,
	}

	body, _ := json.Marshal(orderReq)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		t.config.TradeBFFAPIURL+"/api/trade/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+user.AccessToken)
	req.Header.Set("X-Shard-ID", fmt.Sprintf("%d", user.ShardID))

	sendTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		metrics.orderErrors.Add(1)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		metrics.orderErrors.Add(1)
		return ""
	}

	var orderResp OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		metrics.orderErrors.Add(1)
		return ""
	}

	metrics.ordersSent.Add(1)
	user.OrderTimes.Store(orderResp.OrderID, sendTime)

	return orderResp.OrderID
}

func (t *ShardLoadTester) reportProgress(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			elapsed := time.Since(t.metrics.startTime)

			var totalSent, totalFills, totalErrors int64
			for _, sm := range t.metrics.shardMetrics {
				totalSent += sm.ordersSent.Load()
				totalFills += sm.fillsReceived.Load()
				totalErrors += sm.orderErrors.Load()
			}

			fmt.Printf("[%s] Total Orders: %d, Fills: %d, Errors: %d | Per Shard: ",
				elapsed.Round(time.Second), totalSent, totalFills, totalErrors)

			for i, sm := range t.metrics.shardMetrics {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Printf("S%d:%d", i, sm.ordersSent.Load())
			}
			fmt.Println()
		}
	}
}

func (t *ShardLoadTester) cleanupConnections() {
	for _, shardUsers := range t.users {
		for _, user := range shardUsers {
			if user.WSConn != nil {
				user.WSConn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					time.Now().Add(time.Second),
				)
				user.WSConn.Close()
			}
		}
	}
}

func (t *ShardLoadTester) calculateResults() *Results {
	elapsed := time.Since(t.metrics.startTime)

	results := &Results{
		TotalDuration: elapsed,
		ShardStats:    make([]ShardStats, t.config.ShardCount),
	}

	var totalSent, totalFills, totalOrderErrors, totalWSErrors int64

	for i, sm := range t.metrics.shardMetrics {
		sent := sm.ordersSent.Load()
		fills := sm.fillsReceived.Load()
		orderErrors := sm.orderErrors.Load()
		wsErrors := sm.wsErrors.Load()

		totalSent += sent
		totalFills += fills
		totalOrderErrors += orderErrors
		totalWSErrors += wsErrors

		stats := ShardStats{
			ShardID:         i,
			OrdersSent:      sent,
			FillsReceived:   fills,
			OrderErrors:     orderErrors,
			WSErrors:        wsErrors,
			OrdersPerSecond: float64(sent) / elapsed.Seconds(),
		}

		if sent > 0 {
			stats.FillRate = float64(fills) / float64(sent) * 100
		}

		if len(sm.orderLatencies) > 0 {
			stats.LatencyP50, stats.LatencyP95, stats.LatencyP99 = calculatePercentiles(sm.orderLatencies)
			stats.LatencyMin, stats.LatencyMax, stats.LatencyAvg = calculateStats(sm.orderLatencies)
		}

		results.ShardStats[i] = stats
	}

	results.GlobalStats = GlobalStats{
		TotalOrdersSent:   totalSent,
		TotalFillsReceived: totalFills,
		TotalOrderErrors:  totalOrderErrors,
		TotalWSErrors:     totalWSErrors,
		OrdersPerSecond:   float64(totalSent) / elapsed.Seconds(),
	}

	if totalSent > 0 {
		results.GlobalStats.FillRate = float64(totalFills) / float64(totalSent) * 100
	}

	return results
}

func printResults(results *Results) {
	fmt.Println()
	fmt.Println("=== Shard Load Test Results ===")
	fmt.Println()

	// Global stats
	fmt.Println("Global Statistics:")
	fmt.Printf("  Duration:          %s\n", results.TotalDuration.Round(time.Millisecond))
	fmt.Printf("  Total Orders:      %d\n", results.GlobalStats.TotalOrdersSent)
	fmt.Printf("  Total Fills:       %d\n", results.GlobalStats.TotalFillsReceived)
	fmt.Printf("  Order Errors:      %d\n", results.GlobalStats.TotalOrderErrors)
	fmt.Printf("  WS Errors:         %d\n", results.GlobalStats.TotalWSErrors)
	fmt.Printf("  Orders/sec:        %.2f\n", results.GlobalStats.OrdersPerSecond)
	fmt.Printf("  Fill Rate:         %.1f%%\n", results.GlobalStats.FillRate)
	fmt.Println()

	// Per-shard stats
	fmt.Println("Per-Shard Statistics:")
	fmt.Println("+---------+----------+--------+--------+----------+-----------+")
	fmt.Println("| Shard   | Orders   | Fills  | Errors | Rate     | Orders/s  |")
	fmt.Println("+---------+----------+--------+--------+----------+-----------+")

	for _, ss := range results.ShardStats {
		fmt.Printf("| Shard %-2d| %-8d | %-6d | %-6d | %5.1f%%   | %-9.2f |\n",
			ss.ShardID, ss.OrdersSent, ss.FillsReceived,
			ss.OrderErrors+ss.WSErrors, ss.FillRate, ss.OrdersPerSecond)
	}
	fmt.Println("+---------+----------+--------+--------+----------+-----------+")
	fmt.Println()

	// Per-shard latency
	fmt.Println("Per-Shard Latency (Order-to-Fill):")
	fmt.Println("+---------+-----------+-----------+-----------+-----------+-----------+")
	fmt.Println("| Shard   | p50       | p95       | p99       | min       | max       |")
	fmt.Println("+---------+-----------+-----------+-----------+-----------+-----------+")

	for _, ss := range results.ShardStats {
		if ss.LatencyP50 > 0 {
			fmt.Printf("| Shard %-2d| %-9s | %-9s | %-9s | %-9s | %-9s |\n",
				ss.ShardID,
				ss.LatencyP50.Round(time.Microsecond),
				ss.LatencyP95.Round(time.Microsecond),
				ss.LatencyP99.Round(time.Microsecond),
				ss.LatencyMin.Round(time.Microsecond),
				ss.LatencyMax.Round(time.Microsecond))
		} else {
			fmt.Printf("| Shard %-2d| %-9s | %-9s | %-9s | %-9s | %-9s |\n",
				ss.ShardID, "N/A", "N/A", "N/A", "N/A", "N/A")
		}
	}
	fmt.Println("+---------+-----------+-----------+-----------+-----------+-----------+")
	fmt.Println()
}

func calculatePercentiles(latencies []time.Duration) (p50, p95, p99 time.Duration) {
	if len(latencies) == 0 {
		return
	}

	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	n := len(sorted)
	p50 = sorted[int(float64(n)*0.50)]
	p95 = sorted[int(float64(n)*0.95)]
	p99Index := int(float64(n) * 0.99)
	if p99Index >= n {
		p99Index = n - 1
	}
	p99 = sorted[p99Index]

	return
}

func calculateStats(latencies []time.Duration) (min, max, avg time.Duration) {
	if len(latencies) == 0 {
		return
	}

	min = latencies[0]
	max = latencies[0]
	var total time.Duration

	for _, l := range latencies {
		if l < min {
			min = l
		}
		if l > max {
			max = l
		}
		total += l
	}

	avg = total / time.Duration(len(latencies))
	return
}
