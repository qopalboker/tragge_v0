// Package main provides an order load testing tool for the tragge trading platform.
// It simulates N users placing orders and measures order-to-fill latency.
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
	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
)

// Config holds the load test configuration.
type Config struct {
	// Connection settings
	NumUsers       int
	UserBFFURL     string
	TradeBFFWSURL  string
	TradeBFFAPIURL string
	KafkaBrokers   string
	ContestID      string

	// Shard settings
	ShardAware      bool   // Enable shard-aware mode
	ShardID         int    // Specific shard to target (-1 for auto)
	ShardRouterURL  string // Shard router URL for discovery
	ReportPerShard  bool   // Report metrics per shard

	// Authentication
	EmailPrefix string
	Password    string

	// Test settings
	Duration         time.Duration
	OrdersPerUser    int
	OrderInterval    time.Duration
	Symbols          []string
	ConnectTimeout   time.Duration
	ReadTimeout      time.Duration
}

// Metrics tracks all load test measurements.
type Metrics struct {
	// Order metrics
	ordersSent        atomic.Int64
	ordersAccepted    atomic.Int64
	ordersRejected    atomic.Int64
	fillsReceived     atomic.Int64

	// Latency tracking
	orderToFillLatencies []time.Duration
	latencyMu            sync.Mutex

	// Error metrics
	orderErrors     atomic.Int64
	wsErrors        atomic.Int64
	authErrors      atomic.Int64

	// Timing
	startTime time.Time

	// Per-shard metrics (when shard-aware mode is enabled)
	perShardMetrics map[int]*ShardMetrics
	shardMu         sync.RWMutex
}

// ShardMetrics tracks metrics for a single shard.
type ShardMetrics struct {
	ShardID          int
	OrdersSent       atomic.Int64
	FillsReceived    atomic.Int64
	OrderErrors      atomic.Int64
	Latencies        []time.Duration
	LatencyMu        sync.Mutex
}

// ShardInfo represents shard discovery information.
type ShardInfo struct {
	ShardID int    `json:"shard_id"`
	Address string `json:"address"`
	Status  string `json:"status"`
}

// User represents a simulated user.
type User struct {
	ID          string
	Email       string
	AccessToken string
	WSConn      *websocket.Conn
	OrderTimes  sync.Map // orderID -> send time
	ShardID     int      // Assigned shard ID (for shard-aware mode)
}

// LoginRequest represents the login API request.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest represents the registration API request.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
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
	StopPrice  *float64            `json:"stop_price,omitempty"`
	TakeProfit *float64            `json:"take_profit,omitempty"`
	StopLoss   *float64            `json:"stop_loss,omitempty"`
}

// OrderResponse represents the order submission response.
type OrderResponse struct {
	OrderID string `json:"order_id"`
}

// WSMessage represents a WebSocket message.
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Phase   string          `json:"phase,omitempty"`
}

func main() {
	cfg := parseFlags()

	if err := run(cfg); err != nil {
		log.Fatalf("Load test failed: %v", err)
	}
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.IntVar(&cfg.NumUsers, "users", 10, "Number of simulated users")
	flag.StringVar(&cfg.UserBFFURL, "user-bff", "http://localhost:8081", "User BFF base URL")
	flag.StringVar(&cfg.TradeBFFWSURL, "trade-bff-ws", "ws://localhost:8082", "Trade BFF WebSocket base URL")
	flag.StringVar(&cfg.TradeBFFAPIURL, "trade-bff-api", "http://localhost:8082", "Trade BFF API base URL")
	flag.StringVar(&cfg.KafkaBrokers, "kafka", "localhost:9092", "Kafka brokers (comma-separated)")
	flag.StringVar(&cfg.ContestID, "contest-id", "", "Contest ID (required)")
	flag.StringVar(&cfg.EmailPrefix, "email-prefix", "loadtest", "Email prefix for test users")
	flag.StringVar(&cfg.Password, "password", "loadtest123!", "Password for test users")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.IntVar(&cfg.OrdersPerUser, "orders-per-user", 10, "Number of orders each user places")
	flag.DurationVar(&cfg.OrderInterval, "order-interval", 1*time.Second, "Interval between orders")
	flag.DurationVar(&cfg.ConnectTimeout, "connect-timeout", 10*time.Second, "Connection timeout")
	flag.DurationVar(&cfg.ReadTimeout, "read-timeout", 65*time.Second, "Read timeout")

	// Shard-aware flags
	flag.BoolVar(&cfg.ShardAware, "shard-aware", false, "Enable shard-aware mode")
	flag.IntVar(&cfg.ShardID, "shard-id", -1, "Target specific shard ID (-1 for auto-discovery)")
	flag.StringVar(&cfg.ShardRouterURL, "shard-router", "http://localhost:8090", "Shard router URL")
	flag.BoolVar(&cfg.ReportPerShard, "report-per-shard", false, "Report metrics per shard")

	var symbolsStr string
	flag.StringVar(&symbolsStr, "symbols", "AAPL,GOOGL,MSFT,AMZN,TSLA", "Trading symbols (comma-separated)")

	flag.Parse()

	if cfg.ContestID == "" {
		fmt.Fprintln(os.Stderr, "Error: -contest-id is required")
		flag.Usage()
		os.Exit(1)
	}

	// Parse symbols
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
				// Trim whitespace
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

func run(cfg *Config) error {
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
	fmt.Println("=== Order Load Test ===")
	fmt.Printf("Users:           %d\n", cfg.NumUsers)
	fmt.Printf("Orders/User:     %d\n", cfg.OrdersPerUser)
	fmt.Printf("Order Interval:  %s\n", cfg.OrderInterval)
	fmt.Printf("Duration:        %s\n", cfg.Duration)
	fmt.Printf("Contest ID:      %s\n", cfg.ContestID)
	fmt.Printf("Symbols:         %v\n", cfg.Symbols)
	fmt.Printf("User BFF:        %s\n", cfg.UserBFFURL)
	fmt.Printf("Trade BFF API:   %s\n", cfg.TradeBFFAPIURL)
	fmt.Printf("Trade BFF WS:    %s\n", cfg.TradeBFFWSURL)
	if cfg.ShardAware {
		fmt.Printf("Shard Aware:     %v\n", cfg.ShardAware)
		fmt.Printf("Target Shard:    %d (auto=%v)\n", cfg.ShardID, cfg.ShardID == -1)
		fmt.Printf("Shard Router:    %s\n", cfg.ShardRouterURL)
		fmt.Printf("Per-Shard Report:%v\n", cfg.ReportPerShard)
	}
	fmt.Println()

	// Initialize metrics
	metrics := &Metrics{
		orderToFillLatencies: make([]time.Duration, 0, cfg.NumUsers*cfg.OrdersPerUser),
		startTime:            time.Now(),
		perShardMetrics:      make(map[int]*ShardMetrics),
	}

	// Discover shard for contest if shard-aware mode is enabled
	var contestShardID int = -1
	if cfg.ShardAware {
		if cfg.ShardID >= 0 {
			contestShardID = cfg.ShardID
		} else {
			// Auto-discover shard from router
			shardInfo, err := discoverShard(ctx, cfg, cfg.ContestID)
			if err != nil {
				log.Printf("Warning: Could not discover shard for contest: %v (continuing without shard info)", err)
			} else {
				contestShardID = shardInfo.ShardID
				fmt.Printf("Discovered shard %d for contest %s\n", contestShardID, cfg.ContestID)
			}
		}
		// Initialize shard metrics
		if contestShardID >= 0 {
			metrics.perShardMetrics[contestShardID] = &ShardMetrics{
				ShardID:   contestShardID,
				Latencies: make([]time.Duration, 0),
			}
		}
	}

	// Step 1: Create and authenticate users
	fmt.Printf("Creating and authenticating %d users...\n", cfg.NumUsers)
	users, err := createUsers(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create users: %w", err)
	}
	fmt.Printf("Successfully authenticated %d/%d users\n", len(users), cfg.NumUsers)

	if len(users) == 0 {
		return fmt.Errorf("no users authenticated, cannot proceed")
	}

	// Step 2: Connect users to WebSocket
	fmt.Println("Connecting users to WebSocket...")
	connectedUsers := connectUsersToWS(ctx, cfg, users, metrics)
	fmt.Printf("Connected %d/%d users to WebSocket\n", len(connectedUsers), len(users))

	if len(connectedUsers) == 0 {
		return fmt.Errorf("no WebSocket connections established")
	}

	// Cleanup WebSocket connections at end
	defer func() {
		for _, user := range connectedUsers {
			if user.WSConn != nil {
				user.WSConn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					time.Now().Add(time.Second),
				)
				user.WSConn.Close()
			}
		}
	}()

	// Step 3: Start WebSocket message readers
	fmt.Println("Starting WebSocket message handlers...")
	var wg sync.WaitGroup
	for _, user := range connectedUsers {
		wg.Add(1)
		go func(u *User) {
			defer wg.Done()
			readWSMessages(ctx, u, metrics, cfg.ReadTimeout)
		}(user)
	}

	// Step 4: Start placing orders
	fmt.Printf("Starting order placement (duration: %s)...\n", cfg.Duration)
	metrics.startTime = time.Now()

	testCtx, testCancel := context.WithTimeout(ctx, cfg.Duration)
	defer testCancel()

	eg, egCtx := errgroup.WithContext(testCtx)
	eg.SetLimit(cfg.NumUsers * 2) // Allow some parallelism

	for _, user := range connectedUsers {
		user := user
		eg.Go(func() error {
			placeOrders(egCtx, cfg, user, metrics)
			return nil
		})
	}

	// Progress reporter
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-testCtx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(metrics.startTime)
				sent := metrics.ordersSent.Load()
				fills := metrics.fillsReceived.Load()
				fmt.Printf("[%s] Orders Sent: %d, Fills: %d, Errors: %d\n",
					elapsed.Round(time.Second),
					sent, fills, metrics.orderErrors.Load())
			}
		}
	}()

	// Wait for order placement to complete
	eg.Wait()

	// Wait a bit for remaining fills to arrive
	fmt.Println("Waiting for remaining fills...")
	time.Sleep(3 * time.Second)

	// Cancel context to stop WS readers
	cancel()

	// Wait for WS readers to finish
	wg.Wait()

	// Print results
	printResults(cfg, metrics)

	return nil
}

func createUsers(ctx context.Context, cfg *Config) ([]*User, error) {
	users := make([]*User, 0, cfg.NumUsers)
	client := &http.Client{Timeout: 30 * time.Second}

	for i := 0; i < cfg.NumUsers; i++ {
		email := fmt.Sprintf("%s_%d@loadtest.example.com", cfg.EmailPrefix, i)

		// Try to register first
		user, err := registerUser(ctx, client, cfg, email)
		if err != nil {
			// Registration failed, try login (user might exist)
			user, err = loginUser(ctx, client, cfg, email)
			if err != nil {
				log.Printf("Failed to authenticate user %s: %v", email, err)
				continue
			}
		}

		// Join contest
		err = joinContest(ctx, client, cfg, user)
		if err != nil {
			log.Printf("Failed to join contest for user %s: %v", email, err)
			// Continue anyway - user might already be joined
		}

		users = append(users, user)
	}

	return users, nil
}

func registerUser(ctx context.Context, client *http.Client, cfg *Config, email string) (*User, error) {
	reqBody := RegisterRequest{
		Email:    email,
		Password: cfg.Password,
	}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, cfg.UserBFFURL+"/api/user/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registration failed (status %d): %s", resp.StatusCode, string(bodyBytes))
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

func loginUser(ctx context.Context, client *http.Client, cfg *Config, email string) (*User, error) {
	reqBody := LoginRequest{
		Email:    email,
		Password: cfg.Password,
	}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, cfg.UserBFFURL+"/api/user/auth/login", bytes.NewReader(body))
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

func joinContest(ctx context.Context, client *http.Client, cfg *Config, user *User) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.UserBFFURL+"/api/user/contests/"+cfg.ContestID+"/join", nil)
	req.Header.Set("Authorization", "Bearer "+user.AccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("join contest failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func connectUsersToWS(ctx context.Context, cfg *Config, users []*User, metrics *Metrics) []*User {
	var connected []*User
	var mu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, 50) // Limit concurrent connections

	for _, user := range users {
		wg.Add(1)
		go func(u *User) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			conn, err := connectToWS(ctx, cfg, u.AccessToken)
			if err != nil {
				metrics.wsErrors.Add(1)
				log.Printf("WS connection failed for %s: %v", u.Email, err)
				return
			}

			u.WSConn = conn

			mu.Lock()
			connected = append(connected, u)
			mu.Unlock()
		}(user)
	}

	wg.Wait()
	return connected
}

func connectToWS(ctx context.Context, cfg *Config, token string) (*websocket.Conn, error) {
	wsURL, _ := url.Parse(cfg.TradeBFFWSURL + "/ws/trade")
	q := wsURL.Query()
	q.Set("contest_id", cfg.ContestID)
	wsURL.RawQuery = q.Encode()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := websocket.Dialer{
		HandshakeTimeout: cfg.ConnectTimeout,
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

func readWSMessages(ctx context.Context, user *User, metrics *Metrics, readTimeout time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if user.WSConn == nil {
			return
		}

		user.WSConn.SetReadDeadline(time.Now().Add(readTimeout))
		_, message, err := user.WSConn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			// Timeout or other error, continue if context not done
			continue
		}

		receiveTime := time.Now()

		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		// Track fill events for latency calculation
		if msg.Type == "fill" {
			var fill contracts.FillEvent
			if err := json.Unmarshal(msg.Payload, &fill); err != nil {
				continue
			}

			metrics.fillsReceived.Add(1)

			// Calculate latency
			if sendTimeVal, ok := user.OrderTimes.Load(fill.OrderID); ok {
				sendTime := sendTimeVal.(time.Time)
				latency := receiveTime.Sub(sendTime)

				metrics.latencyMu.Lock()
				metrics.orderToFillLatencies = append(metrics.orderToFillLatencies, latency)
				metrics.latencyMu.Unlock()
			}
		}
	}
}

func placeOrders(ctx context.Context, cfg *Config, user *User, metrics *Metrics) {
	client := &http.Client{Timeout: 30 * time.Second}

	for i := 0; i < cfg.OrdersPerUser; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Place order
		orderID := placeOrder(ctx, client, cfg, user, metrics)
		if orderID != "" {
			user.OrderTimes.Store(orderID, time.Now())
		}

		// Wait before next order
		select {
		case <-ctx.Done():
			return
		case <-time.After(cfg.OrderInterval):
		}
	}
}

func placeOrder(ctx context.Context, client *http.Client, cfg *Config, user *User, metrics *Metrics) string {
	// Random symbol and side
	symbol := cfg.Symbols[rand.Intn(len(cfg.Symbols))]
	side := contracts.OrderSideBuy
	if rand.Intn(2) == 0 {
		side = contracts.OrderSideSell
	}

	// Random quantity
	qty := rand.Intn(100) + 1

	orderReq := OrderSubmitRequest{
		ContestID: cfg.ContestID,
		Symbol:    symbol,
		Side:      side,
		Type:      contracts.OrderTypeMarket,
		Qty:       qty,
	}

	body, _ := json.Marshal(orderReq)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.TradeBFFAPIURL+"/api/trade/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+user.AccessToken)

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

	// Store send time for latency calculation
	user.OrderTimes.Store(orderResp.OrderID, sendTime)

	return orderResp.OrderID
}

func printResults(cfg *Config, metrics *Metrics) {
	elapsed := time.Since(metrics.startTime)

	fmt.Println()
	fmt.Println("=== Order Load Test Results ===")
	fmt.Println()

	// Order metrics
	sent := metrics.ordersSent.Load()
	fills := metrics.fillsReceived.Load()

	fmt.Println("Order Metrics:")
	fmt.Printf("  Orders Sent:     %d\n", sent)
	fmt.Printf("  Fills Received:  %d\n", fills)
	fmt.Printf("  Order Errors:    %d\n", metrics.orderErrors.Load())
	fmt.Printf("  WS Errors:       %d\n", metrics.wsErrors.Load())
	if sent > 0 {
		fillRate := float64(fills) / float64(sent) * 100
		fmt.Printf("  Fill Rate:       %.1f%%\n", fillRate)
	}
	fmt.Println()

	// Throughput
	ordersPerSec := float64(sent) / elapsed.Seconds()
	fmt.Println("Throughput:")
	fmt.Printf("  Duration:        %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  Orders/sec:      %.2f\n", ordersPerSec)
	fmt.Println()

	// Latency percentiles
	if len(metrics.orderToFillLatencies) > 0 {
		p50, p95, p99 := calculatePercentiles(metrics.orderToFillLatencies)
		fmt.Println("Order-to-Fill Latency:")
		fmt.Printf("  Samples:         %d\n", len(metrics.orderToFillLatencies))
		fmt.Printf("  p50:             %s\n", p50.Round(time.Microsecond))
		fmt.Printf("  p95:             %s\n", p95.Round(time.Microsecond))
		fmt.Printf("  p99:             %s\n", p99.Round(time.Microsecond))

		// Also calculate min, max, avg
		min, max, avg := calculateStats(metrics.orderToFillLatencies)
		fmt.Printf("  min:             %s\n", min.Round(time.Microsecond))
		fmt.Printf("  max:             %s\n", max.Round(time.Microsecond))
		fmt.Printf("  avg:             %s\n", avg.Round(time.Microsecond))
	} else {
		fmt.Println("Order-to-Fill Latency:")
		fmt.Println("  No latency data collected (no fills received)")
	}
	fmt.Println()

	// Per-shard metrics (when shard-aware mode is enabled)
	if cfg.ShardAware && cfg.ReportPerShard && len(metrics.perShardMetrics) > 0 {
		fmt.Println("Per-Shard Metrics:")
		fmt.Println("+---------+----------+--------+--------+----------+")
		fmt.Println("| Shard   | Orders   | Fills  | Errors | Rate     |")
		fmt.Println("+---------+----------+--------+--------+----------+")

		for shardID, sm := range metrics.perShardMetrics {
			shardSent := sm.OrdersSent.Load()
			shardFills := sm.FillsReceived.Load()
			shardErrors := sm.OrderErrors.Load()
			var fillRate float64
			if shardSent > 0 {
				fillRate = float64(shardFills) / float64(shardSent) * 100
			}
			fmt.Printf("| Shard %-2d| %-8d | %-6d | %-6d | %5.1f%%   |\n",
				shardID, shardSent, shardFills, shardErrors, fillRate)
		}
		fmt.Println("+---------+----------+--------+--------+----------+")
		fmt.Println()

		// Per-shard latency
		fmt.Println("Per-Shard Latency:")
		fmt.Println("+---------+-----------+-----------+-----------+-----------+")
		fmt.Println("| Shard   | p50       | p95       | p99       | avg       |")
		fmt.Println("+---------+-----------+-----------+-----------+-----------+")

		for shardID, sm := range metrics.perShardMetrics {
			if len(sm.Latencies) > 0 {
				p50, p95, p99 := calculatePercentiles(sm.Latencies)
				_, _, avg := calculateStats(sm.Latencies)
				fmt.Printf("| Shard %-2d| %-9s | %-9s | %-9s | %-9s |\n",
					shardID,
					p50.Round(time.Microsecond),
					p95.Round(time.Microsecond),
					p99.Round(time.Microsecond),
					avg.Round(time.Microsecond))
			} else {
				fmt.Printf("| Shard %-2d| %-9s | %-9s | %-9s | %-9s |\n",
					shardID, "N/A", "N/A", "N/A", "N/A")
			}
		}
		fmt.Println("+---------+-----------+-----------+-----------+-----------+")
		fmt.Println()
	}
}

// discoverShard queries the shard router to discover shard assignment for a contest.
func discoverShard(ctx context.Context, cfg *Config, contestID string) (*ShardInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		cfg.ShardRouterURL+"/shard/"+contestID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("shard discovery failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var shardInfo ShardInfo
	if err := json.NewDecoder(resp.Body).Decode(&shardInfo); err != nil {
		return nil, err
	}

	return &shardInfo, nil
}

// recordShardMetric records a metric for a specific shard.
func (m *Metrics) recordShardMetric(shardID int, sent, fills, errors int64, latency time.Duration) {
	m.shardMu.Lock()
	defer m.shardMu.Unlock()

	sm, exists := m.perShardMetrics[shardID]
	if !exists {
		sm = &ShardMetrics{
			ShardID:   shardID,
			Latencies: make([]time.Duration, 0),
		}
		m.perShardMetrics[shardID] = sm
	}

	if sent > 0 {
		sm.OrdersSent.Add(sent)
	}
	if fills > 0 {
		sm.FillsReceived.Add(fills)
	}
	if errors > 0 {
		sm.OrderErrors.Add(errors)
	}
	if latency > 0 {
		sm.LatencyMu.Lock()
		sm.Latencies = append(sm.Latencies, latency)
		sm.LatencyMu.Unlock()
	}
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
