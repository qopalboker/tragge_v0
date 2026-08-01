// Package main provides an order burst load testing tool for the tragge trading platform.
// It simulates high-frequency order placement across multiple contests simultaneously.
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

// Config holds the load test configuration.
type Config struct {
	// Target settings
	TargetOrdersPerSecond int      // Orders per second to achieve
	NumContests           int      // Number of contests to spread orders across
	ContestIDs            []string // Specific contest IDs (optional, will be discovered if empty)

	// Connection settings
	UsersPerContest int
	UserBFFURL      string
	TradeBFFWSURL   string
	TradeBFFAPIURL  string

	// Authentication
	EmailPrefix string
	Password    string

	// Test settings
	Duration        time.Duration
	Symbols         []string
	ConnectTimeout  time.Duration
	ReadTimeout     time.Duration
	RampUpDuration  time.Duration
}

// Metrics tracks all load test measurements.
type Metrics struct {
	// Order metrics
	ordersSent     atomic.Int64
	ordersAccepted atomic.Int64
	ordersRejected atomic.Int64
	fillsReceived  atomic.Int64

	// Latency tracking
	orderLatencies []time.Duration
	latencyMu      sync.Mutex

	// Per-contest metrics
	perContestOrders map[string]*atomic.Int64
	perContestFills  map[string]*atomic.Int64
	contestMu        sync.RWMutex

	// Error metrics
	orderErrors atomic.Int64
	wsErrors    atomic.Int64
	authErrors  atomic.Int64

	// Rate tracking
	ordersThisSecond atomic.Int64
	lastSecond       time.Time
	actualRates      []float64
	ratesMu          sync.Mutex

	// Timing
	startTime time.Time
}

// ContestUser represents a user participating in a specific contest.
type ContestUser struct {
	ID          string
	Email       string
	AccessToken string
	WSConn      *websocket.Conn
	ContestID   string
	OrderTimes  sync.Map // orderID -> send time
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

	flag.IntVar(&cfg.TargetOrdersPerSecond, "orders-per-second", 100, "Target orders per second")
	flag.IntVar(&cfg.NumContests, "num-contests", 10, "Number of contests to spread orders across")
	flag.IntVar(&cfg.UsersPerContest, "users-per-contest", 5, "Number of users per contest")
	flag.StringVar(&cfg.UserBFFURL, "user-bff", "http://localhost:8081", "User BFF base URL")
	flag.StringVar(&cfg.TradeBFFWSURL, "trade-bff-ws", "ws://localhost:8082", "Trade BFF WebSocket base URL")
	flag.StringVar(&cfg.TradeBFFAPIURL, "trade-bff-api", "http://localhost:8082", "Trade BFF API base URL")
	flag.StringVar(&cfg.EmailPrefix, "email-prefix", "burst", "Email prefix for test users")
	flag.StringVar(&cfg.Password, "password", "bursttest123!", "Password for test users")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.DurationVar(&cfg.ConnectTimeout, "connect-timeout", 10*time.Second, "Connection timeout")
	flag.DurationVar(&cfg.ReadTimeout, "read-timeout", 65*time.Second, "Read timeout")
	flag.DurationVar(&cfg.RampUpDuration, "ramp-up", 10*time.Second, "Ramp-up duration")

	var contestIDsStr, symbolsStr string
	flag.StringVar(&contestIDsStr, "contest-ids", "", "Contest IDs (comma-separated, required)")
	flag.StringVar(&symbolsStr, "symbols", "AAPL,GOOGL,MSFT,AMZN,TSLA", "Trading symbols (comma-separated)")

	flag.Parse()

	if contestIDsStr == "" {
		fmt.Fprintln(os.Stderr, "Error: -contest-ids is required")
		flag.Usage()
		os.Exit(1)
	}

	cfg.ContestIDs = parseCommaSeparated(contestIDsStr)
	cfg.Symbols = parseCommaSeparated(symbolsStr)

	if len(cfg.ContestIDs) == 0 {
		fmt.Fprintln(os.Stderr, "Error: at least one contest ID is required")
		flag.Usage()
		os.Exit(1)
	}

	// Adjust num contests if fewer provided
	if len(cfg.ContestIDs) < cfg.NumContests {
		cfg.NumContests = len(cfg.ContestIDs)
	}

	return cfg
}

func parseCommaSeparated(s string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > start {
				item := s[start:i]
				for len(item) > 0 && item[0] == ' ' {
					item = item[1:]
				}
				for len(item) > 0 && item[len(item)-1] == ' ' {
					item = item[:len(item)-1]
				}
				if len(item) > 0 {
					result = append(result, item)
				}
			}
			start = i + 1
		}
	}
	return result
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
	fmt.Println("=== Order Burst Load Test ===")
	fmt.Printf("Target Rate:     %d orders/sec\n", cfg.TargetOrdersPerSecond)
	fmt.Printf("Contests:        %d\n", cfg.NumContests)
	fmt.Printf("Users/Contest:   %d\n", cfg.UsersPerContest)
	fmt.Printf("Total Users:     %d\n", cfg.NumContests*cfg.UsersPerContest)
	fmt.Printf("Duration:        %s\n", cfg.Duration)
	fmt.Printf("Ramp-up:         %s\n", cfg.RampUpDuration)
	fmt.Printf("Contest IDs:     %v\n", cfg.ContestIDs[:cfg.NumContests])
	fmt.Printf("Symbols:         %v\n", cfg.Symbols)
	fmt.Println()

	// Initialize metrics
	metrics := &Metrics{
		orderLatencies:   make([]time.Duration, 0, cfg.TargetOrdersPerSecond*int(cfg.Duration.Seconds())),
		perContestOrders: make(map[string]*atomic.Int64),
		perContestFills:  make(map[string]*atomic.Int64),
		actualRates:      make([]float64, 0, int(cfg.Duration.Seconds())),
		startTime:        time.Now(),
		lastSecond:       time.Now(),
	}

	// Initialize per-contest metrics
	for _, cid := range cfg.ContestIDs[:cfg.NumContests] {
		metrics.perContestOrders[cid] = &atomic.Int64{}
		metrics.perContestFills[cid] = &atomic.Int64{}
	}

	// Step 1: Create and authenticate users for each contest
	fmt.Printf("Creating and authenticating %d users across %d contests...\n",
		cfg.NumContests*cfg.UsersPerContest, cfg.NumContests)
	users, err := createUsersForContests(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create users: %w", err)
	}
	fmt.Printf("Successfully authenticated %d users\n", len(users))

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
		go func(u *ContestUser) {
			defer wg.Done()
			readWSMessages(ctx, u, metrics, cfg.ReadTimeout)
		}(user)
	}

	// Step 4: Calculate order distribution
	// Orders per contest per second = target_rate / num_contests
	ordersPerContestPerSecond := float64(cfg.TargetOrdersPerSecond) / float64(cfg.NumContests)
	intervalBetweenOrders := time.Duration(float64(time.Second) / float64(cfg.TargetOrdersPerSecond))

	fmt.Printf("Orders per contest/sec: %.2f\n", ordersPerContestPerSecond)
	fmt.Printf("Order interval: %s\n", intervalBetweenOrders)
	fmt.Println()

	// Step 5: Start the order burst
	fmt.Printf("Starting order burst for %s...\n", cfg.Duration)
	metrics.startTime = time.Now()

	testCtx, testCancel := context.WithTimeout(ctx, cfg.Duration)
	defer testCancel()

	// Rate tracker goroutine
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-testCtx.Done():
				return
			case <-ticker.C:
				currentRate := float64(metrics.ordersThisSecond.Swap(0))
				metrics.ratesMu.Lock()
				metrics.actualRates = append(metrics.actualRates, currentRate)
				metrics.ratesMu.Unlock()
			}
		}
	}()

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
				errors := metrics.orderErrors.Load()
				actualRate := float64(sent) / elapsed.Seconds()
				fmt.Printf("[%s] Orders: %d (%.1f/s target: %d/s), Fills: %d, Errors: %d\n",
					elapsed.Round(time.Second), sent, actualRate, cfg.TargetOrdersPerSecond, fills, errors)
			}
		}
	}()

	// Order placement goroutine
	eg, egCtx := errgroup.WithContext(testCtx)
	eg.SetLimit(cfg.TargetOrdersPerSecond * 2) // Allow some headroom

	// Distribute orders across users using a round-robin approach
	orderTicker := time.NewTicker(intervalBetweenOrders)
	defer orderTicker.Stop()

	userIdx := 0
	for {
		select {
		case <-egCtx.Done():
			goto done
		case <-orderTicker.C:
		}

		// Select user in round-robin fashion
		user := connectedUsers[userIdx%len(connectedUsers)]
		userIdx++

		eg.Go(func() error {
			placeOrderBurst(egCtx, cfg, user, metrics)
			return nil
		})
	}

done:
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

func createUsersForContests(ctx context.Context, cfg *Config) ([]*ContestUser, error) {
	users := make([]*ContestUser, 0, cfg.NumContests*cfg.UsersPerContest)
	client := &http.Client{Timeout: 30 * time.Second}

	for contestIdx := 0; contestIdx < cfg.NumContests; contestIdx++ {
		contestID := cfg.ContestIDs[contestIdx]

		for userIdx := 0; userIdx < cfg.UsersPerContest; userIdx++ {
			email := fmt.Sprintf("%s_c%d_u%d@loadtest.example.com", cfg.EmailPrefix, contestIdx, userIdx)

			// Try to register first
			user, err := registerUser(ctx, client, cfg, email, contestID)
			if err != nil {
				// Registration failed, try login
				user, err = loginUser(ctx, client, cfg, email, contestID)
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
	}

	return users, nil
}

func registerUser(ctx context.Context, client *http.Client, cfg *Config, email, contestID string) (*ContestUser, error) {
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

	return &ContestUser{
		ID:          uuid.New().String(),
		Email:       email,
		AccessToken: authResp.AccessToken,
		ContestID:   contestID,
	}, nil
}

func loginUser(ctx context.Context, client *http.Client, cfg *Config, email, contestID string) (*ContestUser, error) {
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

	return &ContestUser{
		ID:          uuid.New().String(),
		Email:       email,
		AccessToken: authResp.AccessToken,
		ContestID:   contestID,
	}, nil
}

func joinContest(ctx context.Context, client *http.Client, cfg *Config, user *ContestUser) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		cfg.UserBFFURL+"/api/user/contests/"+user.ContestID+"/join", nil)
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

func connectUsersToWS(ctx context.Context, cfg *Config, users []*ContestUser, metrics *Metrics) []*ContestUser {
	var connected []*ContestUser
	var mu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, 50) // Limit concurrent connections

	for _, user := range users {
		wg.Add(1)
		go func(u *ContestUser) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			conn, err := connectToWS(ctx, cfg, u.AccessToken, u.ContestID)
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

func connectToWS(ctx context.Context, cfg *Config, token, contestID string) (*websocket.Conn, error) {
	wsURL, _ := url.Parse(cfg.TradeBFFWSURL + "/ws/trade")
	q := wsURL.Query()
	q.Set("contest_id", contestID)
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

func readWSMessages(ctx context.Context, user *ContestUser, metrics *Metrics, readTimeout time.Duration) {
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

			// Update per-contest fills
			if counter, ok := metrics.perContestFills[user.ContestID]; ok {
				counter.Add(1)
			}

			// Calculate latency
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

func placeOrderBurst(ctx context.Context, cfg *Config, user *ContestUser, metrics *Metrics) {
	client := &http.Client{Timeout: 30 * time.Second}

	select {
	case <-ctx.Done():
		return
	default:
	}

	// Random symbol and side
	symbol := cfg.Symbols[rand.Intn(len(cfg.Symbols))]
	side := contracts.OrderSideBuy
	if rand.Intn(2) == 0 {
		side = contracts.OrderSideSell
	}

	// Random quantity
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
		cfg.TradeBFFAPIURL+"/api/trade/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+user.AccessToken)

	sendTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		metrics.orderErrors.Add(1)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		metrics.orderErrors.Add(1)
		return
	}

	var orderResp OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&orderResp); err != nil {
		metrics.orderErrors.Add(1)
		return
	}

	metrics.ordersSent.Add(1)
	metrics.ordersThisSecond.Add(1)

	// Update per-contest orders
	if counter, ok := metrics.perContestOrders[user.ContestID]; ok {
		counter.Add(1)
	}

	// Store send time for latency calculation
	user.OrderTimes.Store(orderResp.OrderID, sendTime)
}

func printResults(cfg *Config, metrics *Metrics) {
	elapsed := time.Since(metrics.startTime)

	fmt.Println()
	fmt.Println("=== Order Burst Load Test Results ===")
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
	actualRate := float64(sent) / elapsed.Seconds()
	fmt.Println("Throughput:")
	fmt.Printf("  Duration:        %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  Target Rate:     %d orders/sec\n", cfg.TargetOrdersPerSecond)
	fmt.Printf("  Actual Rate:     %.2f orders/sec\n", actualRate)
	fmt.Printf("  Achievement:     %.1f%%\n", actualRate/float64(cfg.TargetOrdersPerSecond)*100)
	fmt.Println()

	// Rate distribution
	metrics.ratesMu.Lock()
	if len(metrics.actualRates) > 0 {
		avgRate, minRate, maxRate := calculateRateStats(metrics.actualRates)
		fmt.Println("Rate Distribution:")
		fmt.Printf("  Avg Rate:        %.2f orders/sec\n", avgRate)
		fmt.Printf("  Min Rate:        %.2f orders/sec\n", minRate)
		fmt.Printf("  Max Rate:        %.2f orders/sec\n", maxRate)
		fmt.Println()
	}
	metrics.ratesMu.Unlock()

	// Latency percentiles
	if len(metrics.orderLatencies) > 0 {
		p50, p95, p99 := calculatePercentiles(metrics.orderLatencies)
		min, max, avg := calculateStats(metrics.orderLatencies)

		fmt.Println("Order-to-Fill Latency:")
		fmt.Printf("  Samples:         %d\n", len(metrics.orderLatencies))
		fmt.Printf("  p50:             %s\n", p50.Round(time.Microsecond))
		fmt.Printf("  p95:             %s\n", p95.Round(time.Microsecond))
		fmt.Printf("  p99:             %s\n", p99.Round(time.Microsecond))
		fmt.Printf("  min:             %s\n", min.Round(time.Microsecond))
		fmt.Printf("  max:             %s\n", max.Round(time.Microsecond))
		fmt.Printf("  avg:             %s\n", avg.Round(time.Microsecond))
	} else {
		fmt.Println("Order-to-Fill Latency:")
		fmt.Println("  No latency data collected (no fills received)")
	}
	fmt.Println()

	// Per-contest breakdown
	fmt.Println("Per-Contest Breakdown:")
	fmt.Println("+----------------+----------+--------+----------+")
	fmt.Println("| Contest        | Orders   | Fills  | Rate     |")
	fmt.Println("+----------------+----------+--------+----------+")

	for contestID, orderCount := range metrics.perContestOrders {
		fillCount := metrics.perContestFills[contestID]
		orders := orderCount.Load()
		fills := fillCount.Load()
		var rate float64
		if orders > 0 {
			rate = float64(fills) / float64(orders) * 100
		}
		// Truncate contest ID for display
		displayID := contestID
		if len(displayID) > 14 {
			displayID = displayID[:11] + "..."
		}
		fmt.Printf("| %-14s | %-8d | %-6d | %5.1f%%   |\n",
			displayID, orders, fills, rate)
	}
	fmt.Println("+----------------+----------+--------+----------+")
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

func calculateRateStats(rates []float64) (avg, min, max float64) {
	if len(rates) == 0 {
		return
	}

	min = rates[0]
	max = rates[0]
	var total float64

	for _, r := range rates {
		if r < min {
			min = r
		}
		if r > max {
			max = r
		}
		total += r
	}

	avg = total / float64(len(rates))
	return
}
