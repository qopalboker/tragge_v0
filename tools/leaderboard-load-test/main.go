// Package main provides a leaderboard load testing tool for the tragge trading platform.
// It simulates N concurrent users querying the leaderboard endpoint simultaneously.
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
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
)

// Config holds the load test configuration.
type Config struct {
	// Connection settings
	NumConcurrent int
	UserBFFURL    string
	ContestIDs    []string

	// Authentication
	Email    string
	Password string

	// Test settings
	Duration        time.Duration
	RequestInterval time.Duration
	RequestTimeout  time.Duration
}

// Metrics tracks all load test measurements.
type Metrics struct {
	// Request metrics
	requestsSent     atomic.Int64
	requestsSuccess  atomic.Int64
	requestsFailed   atomic.Int64

	// Latency tracking
	latencies   []time.Duration
	latencyMu   sync.Mutex

	// Error tracking
	errors      map[string]int64
	errorsMu    sync.Mutex

	// Per-endpoint metrics
	redisHits   atomic.Int64
	dbFallbacks atomic.Int64

	// Timing
	startTime time.Time
}

// LoginRequest represents the login API request.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents the authentication response.
type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// LeaderboardResponse represents the leaderboard API response.
type LeaderboardResponse struct {
	ContestID string             `json:"contest_id"`
	Entries   []LeaderboardEntry `json:"entries"`
}

// LeaderboardEntry represents a single leaderboard entry.
type LeaderboardEntry struct {
	Rank       int     `json:"rank"`
	UserID     string  `json:"user_id"`
	TotalScore float64 `json:"total_score"`
}

func main() {
	cfg := parseFlags()

	if err := run(cfg); err != nil {
		log.Fatalf("Load test failed: %v", err)
	}
}

func parseFlags() *Config {
	cfg := &Config{}

	flag.IntVar(&cfg.NumConcurrent, "concurrent", 500, "Number of concurrent requests")
	flag.StringVar(&cfg.UserBFFURL, "user-bff", "http://localhost:8081", "User BFF base URL")
	flag.StringVar(&cfg.Email, "email", "", "Login email (required)")
	flag.StringVar(&cfg.Password, "password", "", "Login password (required)")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.DurationVar(&cfg.RequestInterval, "interval", 100*time.Millisecond, "Interval between request batches")
	flag.DurationVar(&cfg.RequestTimeout, "timeout", 30*time.Second, "Request timeout")

	var contestIDsStr string
	flag.StringVar(&contestIDsStr, "contest-ids", "", "Contest IDs (comma-separated, required)")

	flag.Parse()

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
	if contestIDsStr == "" {
		fmt.Fprintln(os.Stderr, "Error: -contest-ids is required")
		flag.Usage()
		os.Exit(1)
	}

	// Parse contest IDs
	cfg.ContestIDs = parseCommaSeparated(contestIDsStr)
	if len(cfg.ContestIDs) == 0 {
		fmt.Fprintln(os.Stderr, "Error: at least one contest ID is required")
		flag.Usage()
		os.Exit(1)
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
				// Trim whitespace
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
	fmt.Println("=== Leaderboard Load Test ===")
	fmt.Printf("Concurrent:      %d\n", cfg.NumConcurrent)
	fmt.Printf("Duration:        %s\n", cfg.Duration)
	fmt.Printf("Request Interval:%s\n", cfg.RequestInterval)
	fmt.Printf("Contest IDs:     %v\n", cfg.ContestIDs)
	fmt.Printf("User BFF:        %s\n", cfg.UserBFFURL)
	fmt.Println()

	// Step 1: Authenticate
	fmt.Println("Authenticating...")
	token, err := login(cfg)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	fmt.Println("Authentication successful!")
	fmt.Println()

	// Initialize metrics
	metrics := &Metrics{
		latencies: make([]time.Duration, 0, cfg.NumConcurrent*int(cfg.Duration.Seconds())),
		errors:    make(map[string]int64),
		startTime: time.Now(),
	}

	// Create HTTP client with connection pooling
	client := &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.NumConcurrent,
			MaxIdleConnsPerHost: cfg.NumConcurrent,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Step 2: Start load test
	fmt.Printf("Starting load test for %s with %d concurrent requests...\n", cfg.Duration, cfg.NumConcurrent)

	testCtx, testCancel := context.WithTimeout(ctx, cfg.Duration)
	defer testCancel()

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
				sent := metrics.requestsSent.Load()
				success := metrics.requestsSuccess.Load()
				failed := metrics.requestsFailed.Load()
				rate := float64(sent) / elapsed.Seconds()
				successRate := float64(0)
				if sent > 0 {
					successRate = float64(success) / float64(sent) * 100
				}
				fmt.Printf("[%s] Requests: %d (%.1f/s), Success: %d (%.1f%%), Failed: %d\n",
					elapsed.Round(time.Second), sent, rate, success, successRate, failed)
			}
		}
	}()

	// Run concurrent requests
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, cfg.NumConcurrent)

	for {
		select {
		case <-testCtx.Done():
			goto done
		default:
		}

		// Launch a batch of concurrent requests
		for i := 0; i < cfg.NumConcurrent; i++ {
			select {
			case <-testCtx.Done():
				goto done
			case semaphore <- struct{}{}:
			}

			wg.Add(1)
			contestID := cfg.ContestIDs[i%len(cfg.ContestIDs)]
			go func(cid string) {
				defer wg.Done()
				defer func() { <-semaphore }()

				queryLeaderboard(testCtx, client, cfg, token, cid, metrics)
			}(contestID)
		}

		// Wait between batches
		select {
		case <-testCtx.Done():
			goto done
		case <-time.After(cfg.RequestInterval):
		}
	}

done:
	// Wait for all requests to complete
	fmt.Println("\nWaiting for pending requests to complete...")
	wg.Wait()

	// Print results
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

	var authResp AuthResponse
	if err := json.Unmarshal(respBody, &authResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return authResp.AccessToken, nil
}

func queryLeaderboard(ctx context.Context, client *http.Client, cfg *Config, token, contestID string, metrics *Metrics) {
	metrics.requestsSent.Add(1)

	url := fmt.Sprintf("%s/api/user/leaderboard?contest_id=%s", cfg.UserBFFURL, contestID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		recordError(metrics, "request_creation_failed")
		metrics.requestsFailed.Add(1)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		recordError(metrics, "request_failed")
		metrics.requestsFailed.Add(1)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		recordError(metrics, "read_body_failed")
		metrics.requestsFailed.Add(1)
		return
	}

	if resp.StatusCode != http.StatusOK {
		recordError(metrics, fmt.Sprintf("status_%d", resp.StatusCode))
		metrics.requestsFailed.Add(1)
		return
	}

	// Parse response to verify it's valid
	var leaderboardResp LeaderboardResponse
	if err := json.Unmarshal(body, &leaderboardResp); err != nil {
		recordError(metrics, "invalid_response")
		metrics.requestsFailed.Add(1)
		return
	}

	// Record success
	metrics.requestsSuccess.Add(1)

	// Record latency
	metrics.latencyMu.Lock()
	metrics.latencies = append(metrics.latencies, latency)
	metrics.latencyMu.Unlock()
}

func recordError(metrics *Metrics, errType string) {
	metrics.errorsMu.Lock()
	metrics.errors[errType]++
	metrics.errorsMu.Unlock()
}

func printResults(cfg *Config, metrics *Metrics) {
	elapsed := time.Since(metrics.startTime)

	fmt.Println()
	fmt.Println("=== Leaderboard Load Test Results ===")
	fmt.Println()

	// Request metrics
	sent := metrics.requestsSent.Load()
	success := metrics.requestsSuccess.Load()
	failed := metrics.requestsFailed.Load()

	fmt.Println("Request Metrics:")
	fmt.Printf("  Total Requests:  %d\n", sent)
	fmt.Printf("  Successful:      %d\n", success)
	fmt.Printf("  Failed:          %d\n", failed)
	if sent > 0 {
		successRate := float64(success) / float64(sent) * 100
		fmt.Printf("  Success Rate:    %.2f%%\n", successRate)
	}
	fmt.Println()

	// Throughput
	requestsPerSec := float64(sent) / elapsed.Seconds()
	fmt.Println("Throughput:")
	fmt.Printf("  Duration:        %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  Requests/sec:    %.2f\n", requestsPerSec)
	fmt.Println()

	// Latency percentiles
	if len(metrics.latencies) > 0 {
		p50, p95, p99 := calculatePercentiles(metrics.latencies)
		min, max, avg := calculateStats(metrics.latencies)

		fmt.Println("Response Latency:")
		fmt.Printf("  Samples:         %d\n", len(metrics.latencies))
		fmt.Printf("  p50:             %s\n", p50.Round(time.Microsecond))
		fmt.Printf("  p95:             %s\n", p95.Round(time.Microsecond))
		fmt.Printf("  p99:             %s\n", p99.Round(time.Microsecond))
		fmt.Printf("  min:             %s\n", min.Round(time.Microsecond))
		fmt.Printf("  max:             %s\n", max.Round(time.Microsecond))
		fmt.Printf("  avg:             %s\n", avg.Round(time.Microsecond))
	} else {
		fmt.Println("Response Latency:")
		fmt.Println("  No latency data collected")
	}
	fmt.Println()

	// Error breakdown
	metrics.errorsMu.Lock()
	if len(metrics.errors) > 0 {
		fmt.Println("Error Breakdown:")
		for errType, count := range metrics.errors {
			fmt.Printf("  %s: %d\n", errType, count)
		}
		fmt.Println()
	}
	metrics.errorsMu.Unlock()
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

// runConcurrentBurst runs a single burst of concurrent requests.
// This is the core of the "500 simultaneous requests" test.
func runConcurrentBurst(ctx context.Context, client *http.Client, cfg *Config, token string, metrics *Metrics) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(cfg.NumConcurrent)

	for i := 0; i < cfg.NumConcurrent; i++ {
		contestID := cfg.ContestIDs[i%len(cfg.ContestIDs)]
		eg.Go(func() error {
			queryLeaderboard(egCtx, client, cfg, token, contestID, metrics)
			return nil
		})
	}

	return eg.Wait()
}
