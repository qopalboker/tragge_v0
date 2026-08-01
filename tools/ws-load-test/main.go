// Package main provides a WebSocket load testing tool for the tragge trading platform.
// It measures connection latency, message throughput, and reliability metrics.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
)

// Config holds the load test configuration.
type Config struct {
	// Connection settings
	NumConnections int
	UserBFFURL     string
	TradeBFFWSURL  string
	ContestID      string

	// Authentication
	Email    string
	Password string

	// Test settings
	Duration       time.Duration
	RampUpDuration time.Duration
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration

	// Compression settings
	EnableCompression bool
}

// Metrics tracks all load test measurements.
type Metrics struct {
	// Connection metrics
	connectLatencies []time.Duration
	connectMu        sync.Mutex
	connectSuccesses atomic.Int64
	connectFailures  atomic.Int64

	// Message metrics
	messagesReceived atomic.Int64
	bytesReceived    atomic.Int64

	// Error metrics
	readErrors   atomic.Int64
	readTimeouts atomic.Int64
	disconnects  atomic.Int64
	pingFailures atomic.Int64

	// Latency tracking (per message type)
	tickLatencies []time.Duration
	tickMu        sync.Mutex

	// Compression metrics
	compressedConnections   atomic.Int64
	uncompressedConnections atomic.Int64

	// Start time for rate calculations
	startTime time.Time
}

// LoginRequest represents the login API request body.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse represents the login API response.
type LoginResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// WSMessage represents a WebSocket message envelope.
type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	Phase   string          `json:"phase,omitempty"`
}

// TickPayload represents the tick_snapshot payload.
type TickPayload struct {
	Timestamp int64        `json:"ts"`
	Symbols   []TickSymbol `json:"symbols"`
}

// TickSymbol represents a single symbol in a tick snapshot.
type TickSymbol struct {
	Symbol string  `json:"symbol"`
	Bid    float64 `json:"bid"`
	Ask    float64 `json:"ask"`
	Last   float64 `json:"last"`
}

func main() {
	cfg := parseFlags()

	if err := run(cfg); err != nil {
		log.Fatalf("Load test failed: %v", err)
	}
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.IntVar(&cfg.NumConnections, "n", 100, "Number of WebSocket connections to open")
	flag.StringVar(&cfg.UserBFFURL, "user-bff", "http://localhost:8081", "User BFF base URL")
	flag.StringVar(&cfg.TradeBFFWSURL, "trade-bff-ws", "ws://localhost:8082", "Trade BFF WebSocket base URL")
	flag.StringVar(&cfg.ContestID, "contest-id", "", "Contest ID for WebSocket connections (required)")
	flag.StringVar(&cfg.Email, "email", "", "Login email (required)")
	flag.StringVar(&cfg.Password, "password", "", "Login password (required)")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.DurationVar(&cfg.RampUpDuration, "ramp-up", 10*time.Second, "Ramp-up duration to open all connections")
	flag.DurationVar(&cfg.ConnectTimeout, "connect-timeout", 10*time.Second, "Connection timeout")
	flag.DurationVar(&cfg.ReadTimeout, "read-timeout", 65*time.Second, "Read timeout (should be > server ping interval)")
	flag.BoolVar(&cfg.EnableCompression, "compression", true, "Enable WebSocket compression (permessage-deflate)")

	flag.Parse()

	if cfg.ContestID == "" {
		fmt.Fprintln(os.Stderr, "Error: -contest-id is required")
		flag.Usage()
		os.Exit(1)
	}
	if cfg.Email == "" {
		fmt.Fprintln(os.Stderr, "Error: -email is required")
		flag.Usage()
		os.Exit(1)
	}
	if cfg.Password == "" {
		fmt.Fprintln(os.Stderr, "Error: -password is required")
		flag.Usage()
		os.Exit(1)
	}

	return cfg
}

func run(cfg *Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("\nReceived shutdown signal, closing connections...")
		cancel()
	}()

	// Print configuration
	fmt.Println("=== WebSocket Load Test ===")
	fmt.Printf("Connections:   %d\n", cfg.NumConnections)
	fmt.Printf("Contest ID:    %s\n", cfg.ContestID)
	fmt.Printf("Duration:      %s\n", cfg.Duration)
	fmt.Printf("Ramp-up:       %s\n", cfg.RampUpDuration)
	fmt.Printf("User BFF:      %s\n", cfg.UserBFFURL)
	fmt.Printf("Trade BFF WS:  %s\n", cfg.TradeBFFWSURL)
	fmt.Printf("Compression:   %v\n", cfg.EnableCompression)
	fmt.Println()

	// Step 1: Authenticate
	fmt.Println("Authenticating...")
	token, err := login(cfg)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	fmt.Println("Authentication successful!")
	fmt.Println()

	// Step 2: Initialize metrics
	metrics := &Metrics{
		connectLatencies: make([]time.Duration, 0, cfg.NumConnections),
		tickLatencies:    make([]time.Duration, 0, cfg.NumConnections*int(cfg.Duration.Seconds())),
		startTime:        time.Now(),
	}

	// Step 3: Open connections with ramp-up
	fmt.Printf("Opening %d connections over %s...\n", cfg.NumConnections, cfg.RampUpDuration)

	conns := make([]*websocket.Conn, 0, cfg.NumConnections)
	var connsMu sync.Mutex

	rampUpInterval := cfg.RampUpDuration / time.Duration(cfg.NumConnections)
	if rampUpInterval < time.Millisecond {
		rampUpInterval = time.Millisecond
	}

	// Use errgroup for controlled concurrency during connection phase
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(50) // Limit concurrent connection attempts

	for i := 0; i < cfg.NumConnections; i++ {
		i := i
		eg.Go(func() error {
			// Ramp-up delay
			delay := time.Duration(i) * rampUpInterval
			select {
			case <-egCtx.Done():
				return nil
			case <-time.After(delay):
			}

			conn, latency, err := connectWS(egCtx, cfg, token, metrics)
			if err != nil {
				metrics.connectFailures.Add(1)
				log.Printf("Connection %d failed: %v", i+1, err)
				return nil // Don't fail the whole group
			}

			metrics.connectSuccesses.Add(1)
			metrics.connectMu.Lock()
			metrics.connectLatencies = append(metrics.connectLatencies, latency)
			metrics.connectMu.Unlock()

			connsMu.Lock()
			conns = append(conns, conn)
			connsMu.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("connection phase failed: %w", err)
	}

	connectedCount := len(conns)
	fmt.Printf("Connected: %d/%d (%.1f%%)\n",
		connectedCount, cfg.NumConnections,
		float64(connectedCount)/float64(cfg.NumConnections)*100)
	fmt.Println()

	if connectedCount == 0 {
		return fmt.Errorf("no connections established")
	}

	// Step 4: Start reading messages from all connections
	fmt.Printf("Running load test for %s...\n", cfg.Duration)
	metrics.startTime = time.Now()

	var wg sync.WaitGroup
	testCtx, testCancel := context.WithTimeout(ctx, cfg.Duration)
	defer testCancel()

	for i, conn := range conns {
		wg.Add(1)
		go func(id int, c *websocket.Conn) {
			defer wg.Done()
			readMessages(testCtx, c, metrics, cfg.ReadTimeout)
		}(i, conn)
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
				msgs := metrics.messagesReceived.Load()
				rate := float64(msgs) / elapsed.Seconds()
				fmt.Printf("[%s] Messages: %d (%.1f/s), Errors: %d, Timeouts: %d, Disconnects: %d\n",
					elapsed.Round(time.Second),
					msgs, rate,
					metrics.readErrors.Load(),
					metrics.readTimeouts.Load(),
					metrics.disconnects.Load())
			}
		}
	}()

	// Wait for test to complete
	<-testCtx.Done()

	// Step 5: Close all connections
	fmt.Println("\nClosing connections...")
	for _, conn := range conns {
		conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
		conn.Close()
	}

	// Wait for readers to finish
	wg.Wait()

	// Step 6: Print results
	printResults(cfg, metrics)

	return nil
}

func login(cfg *Config) (string, error) {
	reqBody := LoginRequest{
		Email:    cfg.Email,
		Password: cfg.Password,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	loginURL := cfg.UserBFFURL + "/api/user/auth/login"
	req, err := http.NewRequest(http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(respBody, &loginResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return loginResp.AccessToken, nil
}

func connectWS(ctx context.Context, cfg *Config, token string, metrics *Metrics) (*websocket.Conn, time.Duration, error) {
	wsURL, err := url.Parse(cfg.TradeBFFWSURL + "/ws/trade")
	if err != nil {
		return nil, 0, fmt.Errorf("parse WS URL: %w", err)
	}
	q := wsURL.Query()
	q.Set("contest_id", cfg.ContestID)
	wsURL.RawQuery = q.Encode()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := websocket.Dialer{
		HandshakeTimeout: cfg.ConnectTimeout,
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
		// Enable compression negotiation (permessage-deflate, RFC 7692)
		EnableCompression: cfg.EnableCompression,
	}

	start := time.Now()
	conn, resp, err := dialer.DialContext(ctx, wsURL.String(), header)
	latency := time.Since(start)

	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, 0, fmt.Errorf("dial failed (status %d): %s: %w", resp.StatusCode, string(body), err)
		}
		return nil, 0, fmt.Errorf("dial failed: %w", err)
	}

	// Check if compression was negotiated by inspecting the response headers
	compressionNegotiated := false
	if resp != nil {
		extensions := resp.Header.Get("Sec-WebSocket-Extensions")
		compressionNegotiated = extensions != "" && cfg.EnableCompression
		resp.Body.Close()
	}

	// Track compression status
	if compressionNegotiated {
		metrics.compressedConnections.Add(1)
	} else {
		metrics.uncompressedConnections.Add(1)
	}

	return conn, latency, nil
}

func readMessages(ctx context.Context, conn *websocket.Conn, metrics *Metrics, readTimeout time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline
		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			metrics.disconnects.Add(1)
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				metrics.disconnects.Add(1)
				return
			}
			if isTimeoutError(err) {
				metrics.readTimeouts.Add(1)
				continue
			}
			metrics.readErrors.Add(1)
			return
		}

		receiveTime := time.Now()
		metrics.messagesReceived.Add(1)
		metrics.bytesReceived.Add(int64(len(message)))

		// Parse message for latency tracking
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		if msg.Type == "tick_snapshot" {
			var payload TickPayload
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				if payload.Timestamp > 0 {
					serverTime := time.UnixMilli(payload.Timestamp)
					latency := receiveTime.Sub(serverTime)
					if latency > 0 && latency < 10*time.Second { // Sanity check
						metrics.tickMu.Lock()
						metrics.tickLatencies = append(metrics.tickLatencies, latency)
						metrics.tickMu.Unlock()
					}
				}
			}
		}
	}
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	// Check for net.Error timeout
	type timeoutError interface {
		Timeout() bool
	}
	if te, ok := err.(timeoutError); ok {
		return te.Timeout()
	}
	return false
}

func printResults(cfg *Config, metrics *Metrics) {
	elapsed := time.Since(metrics.startTime)

	fmt.Println()
	fmt.Println("=== Load Test Results ===")
	fmt.Println()

	// Connection results
	fmt.Println("Connection Metrics:")
	fmt.Printf("  Attempted:     %d\n", cfg.NumConnections)
	fmt.Printf("  Successful:    %d\n", metrics.connectSuccesses.Load())
	fmt.Printf("  Failed:        %d\n", metrics.connectFailures.Load())

	if len(metrics.connectLatencies) > 0 {
		p50, p95, p99 := calculatePercentiles(metrics.connectLatencies)
		fmt.Printf("  Latency p50:   %s\n", p50.Round(time.Millisecond))
		fmt.Printf("  Latency p95:   %s\n", p95.Round(time.Millisecond))
		fmt.Printf("  Latency p99:   %s\n", p99.Round(time.Millisecond))
	}
	fmt.Println()

	// Compression metrics
	compressedConns := metrics.compressedConnections.Load()
	uncompressedConns := metrics.uncompressedConnections.Load()
	totalConns := compressedConns + uncompressedConns

	fmt.Println("Compression Metrics:")
	fmt.Printf("  Enabled:       %v\n", cfg.EnableCompression)
	fmt.Printf("  Compressed:    %d\n", compressedConns)
	fmt.Printf("  Uncompressed:  %d\n", uncompressedConns)
	if totalConns > 0 {
		compressionRate := float64(compressedConns) / float64(totalConns) * 100
		fmt.Printf("  Negotiated:    %.1f%%\n", compressionRate)
	}
	fmt.Println()

	// Message throughput
	msgs := metrics.messagesReceived.Load()
	bytes := metrics.bytesReceived.Load()
	rate := float64(msgs) / elapsed.Seconds()

	fmt.Println("Message Throughput:")
	fmt.Printf("  Duration:      %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  Total msgs:    %d\n", msgs)
	fmt.Printf("  Total bytes:   %.2f MB\n", float64(bytes)/(1024*1024))
	fmt.Printf("  Rate:          %.2f msgs/sec\n", rate)
	fmt.Printf("  Throughput:    %.2f KB/sec\n", float64(bytes)/1024/elapsed.Seconds())
	if compressedConns > 0 {
		// Note: bytesReceived is the decompressed size received by the client
		// The actual network bytes are lower due to compression
		fmt.Printf("  (Note: bytes shown are after decompression)\n")
	}
	fmt.Println()

	// Tick latency (end-to-end from server timestamp)
	if len(metrics.tickLatencies) > 0 {
		p50, p95, p99 := calculatePercentiles(metrics.tickLatencies)
		fmt.Println("Tick Delivery Latency (server timestamp to client receive):")
		fmt.Printf("  Samples:       %d\n", len(metrics.tickLatencies))
		fmt.Printf("  p50:           %s\n", p50.Round(time.Microsecond))
		fmt.Printf("  p95:           %s\n", p95.Round(time.Microsecond))
		fmt.Printf("  p99:           %s\n", p99.Round(time.Microsecond))
		fmt.Println()
	}

	// Error metrics
	fmt.Println("Error Metrics:")
	fmt.Printf("  Read errors:   %d\n", metrics.readErrors.Load())
	fmt.Printf("  Read timeouts: %d\n", metrics.readTimeouts.Load())
	fmt.Printf("  Disconnects:   %d\n", metrics.disconnects.Load())
	fmt.Printf("  Ping failures: %d\n", metrics.pingFailures.Load())
	fmt.Println()

	// Success rate
	totalErrors := metrics.readErrors.Load() + metrics.readTimeouts.Load() + metrics.disconnects.Load()
	if msgs > 0 {
		successRate := float64(msgs) / float64(msgs+totalErrors) * 100
		fmt.Printf("Success Rate:    %.2f%%\n", successRate)
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
	p99 = sorted[int(float64(n)*0.99)]

	return
}
