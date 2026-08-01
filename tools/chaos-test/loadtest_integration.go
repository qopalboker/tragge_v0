package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// LoadTestConfig holds configuration for load testing
type LoadTestConfig struct {
	BaseURL      string
	Email        string
	Password     string
	ContestID    string
	NumUsers     int
	Duration     time.Duration
	OrderRate    time.Duration // Time between orders per user
}

// LoadTester manages background load generation
type LoadTester struct {
	config  *LoadTestConfig
	metrics *LoadMetrics

	// State
	running atomic.Bool
	wg      sync.WaitGroup
}

// LoadMetrics collects metrics during load testing
type LoadMetrics struct {
	sync.Mutex

	// Request counts
	requestsTotal      atomic.Int64
	requestsSuccessful atomic.Int64
	requestsFailed     atomic.Int64

	// Order tracking
	ordersSent     atomic.Int64
	ordersAcked    atomic.Int64
	ordersFilled   atomic.Int64
	ordersRejected atomic.Int64

	// Latency tracking
	latencies []time.Duration

	// Error tracking
	errors []string
}

// NewLoadTester creates a new load tester
func NewLoadTester(cfg *LoadTestConfig) *LoadTester {
	return &LoadTester{
		config: cfg,
		metrics: &LoadMetrics{
			latencies: make([]time.Duration, 0, 10000),
			errors:    make([]string, 0, 100),
		},
	}
}

// Start begins load generation
func (lt *LoadTester) Start(ctx context.Context, virtualUsers int) error {
	if lt.running.Load() {
		return fmt.Errorf("load test already running")
	}

	lt.running.Store(true)

	// Create virtual users
	for i := 0; i < virtualUsers; i++ {
		lt.wg.Add(1)
		go lt.runVirtualUser(ctx, i)
	}

	return nil
}

// Stop stops load generation
func (lt *LoadTester) Stop() {
	lt.running.Store(false)
	lt.wg.Wait()
}

// GetMetrics returns current metrics
func (lt *LoadTester) GetMetrics() *LoadMetrics {
	return lt.metrics
}

// GetTestMetrics converts LoadMetrics to TestMetrics
func (lt *LoadTester) GetTestMetrics() TestMetrics {
	lt.metrics.Lock()
	defer lt.metrics.Unlock()

	result := TestMetrics{
		RequestsTotal:      lt.metrics.requestsTotal.Load(),
		RequestsSuccessful: lt.metrics.requestsSuccessful.Load(),
		RequestsFailed:     lt.metrics.requestsFailed.Load(),
	}

	// Calculate latency percentiles
	if len(lt.metrics.latencies) > 0 {
		sorted := make([]time.Duration, len(lt.metrics.latencies))
		copy(sorted, lt.metrics.latencies)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i] < sorted[j]
		})

		n := len(sorted)
		result.P50Latency = sorted[int(float64(n)*0.50)]
		result.P95Latency = sorted[int(float64(n)*0.95)]
		result.P99Latency = sorted[int(float64(n)*0.99)]
	}

	return result
}

func (lt *LoadTester) runVirtualUser(ctx context.Context, userID int) {
	defer lt.wg.Done()

	// Authenticate
	token, err := lt.authenticate(ctx)
	if err != nil {
		lt.recordError(fmt.Sprintf("user %d auth failed: %v", userID, err))
		return
	}

	// Connect WebSocket
	conn, err := lt.connectWebSocket(ctx, token)
	if err != nil {
		lt.recordError(fmt.Sprintf("user %d ws connect failed: %v", userID, err))
		return
	}
	defer conn.Close()

	// Start message receiver
	go lt.receiveMessages(ctx, conn, userID)

	// Send orders at configured rate
	ticker := time.NewTicker(lt.config.OrderRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !lt.running.Load() {
				return
			}
			lt.sendOrder(ctx, conn, userID)
		}
	}
}

func (lt *LoadTester) authenticate(ctx context.Context) (string, error) {
	loginURL := fmt.Sprintf("%s/api/user/login", lt.config.BaseURL)

	payload := map[string]string{
		"email":    lt.config.Email,
		"password": lt.config.Password,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		lt.metrics.requestsFailed.Add(1)
		return "", err
	}
	defer resp.Body.Close()

	lt.metrics.requestsTotal.Add(1)

	if resp.StatusCode != http.StatusOK {
		lt.metrics.requestsFailed.Add(1)
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login failed: %s - %s", resp.Status, string(respBody))
	}

	lt.metrics.requestsSuccessful.Add(1)

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.AccessToken, nil
}

func (lt *LoadTester) connectWebSocket(ctx context.Context, token string) (*websocket.Conn, error) {
	wsURL, err := url.Parse(lt.config.BaseURL)
	if err != nil {
		return nil, err
	}

	if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path = "/ws/trade"

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	header.Set("X-Contest-ID", lt.config.ContestID)

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		ReadBufferSize:   4096,
		WriteBufferSize:  4096,
	}

	conn, _, err := dialer.DialContext(ctx, wsURL.String(), header)
	if err != nil {
		lt.metrics.requestsFailed.Add(1)
		return nil, err
	}

	lt.metrics.requestsTotal.Add(1)
	lt.metrics.requestsSuccessful.Add(1)

	return conn, nil
}

func (lt *LoadTester) sendOrder(ctx context.Context, conn *websocket.Conn, userID int) {
	order := contracts.OrderRequest{
		OrderID:   uuid.New().String(),
		UserID:    fmt.Sprintf("user-%d", userID),
		ContestID: lt.config.ContestID,
		Symbol:    "AAPL",
		Side:      contracts.OrderSideBuy,
		Type:      contracts.OrderTypeMarket,
		Qty:       10,
	}

	msg := struct {
		Type    string                  `json:"type"`
		Payload contracts.OrderRequest `json:"payload"`
	}{
		Type:    "order",
		Payload: order,
	}

	start := time.Now()
	if err := conn.WriteJSON(msg); err != nil {
		lt.recordError(fmt.Sprintf("order send failed: %v", err))
		lt.metrics.requestsFailed.Add(1)
		return
	}

	lt.metrics.ordersSent.Add(1)
	lt.metrics.requestsTotal.Add(1)
	lt.metrics.requestsSuccessful.Add(1)

	// Record send latency
	lt.metrics.Lock()
	lt.metrics.latencies = append(lt.metrics.latencies, time.Since(start))
	lt.metrics.Unlock()
}

func (lt *LoadTester) receiveMessages(ctx context.Context, conn *websocket.Conn, userID int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				lt.recordError(fmt.Sprintf("user %d ws read error: %v", userID, err))
			}
			return
		}

		// Parse message type
		var envelope struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}

		switch envelope.Type {
		case "order_ack":
			var ack contracts.OrderAck
			if err := json.Unmarshal(envelope.Payload, &ack); err == nil {
				if ack.Status == "accepted" {
					lt.metrics.ordersAcked.Add(1)
				} else {
					lt.metrics.ordersRejected.Add(1)
				}
			}

		case "fill":
			lt.metrics.ordersFilled.Add(1)
		}
	}
}

func (lt *LoadTester) recordError(err string) {
	lt.metrics.Lock()
	defer lt.metrics.Unlock()

	if len(lt.metrics.errors) < 100 {
		lt.metrics.errors = append(lt.metrics.errors, err)
	}
}

// ChaosLoadTest runs chaos scenarios during active load
type ChaosLoadTest struct {
	loadTester *LoadTester
	scenario   ChaosScenario
	config     *LoadTestConfig
}

// NewChaosLoadTest creates a new chaos load test
func NewChaosLoadTest(scenario ChaosScenario, cfg *LoadTestConfig) *ChaosLoadTest {
	return &ChaosLoadTest{
		loadTester: NewLoadTester(cfg),
		scenario:   scenario,
		config:     cfg,
	}
}

// Run executes the chaos load test
func (clt *ChaosLoadTest) Run(ctx context.Context, clientset interface{}, namespace string) (*TestResult, error) {
	result := &TestResult{
		Scenario:    clt.scenario.Name() + "-under-load",
		Description: clt.scenario.Description() + " (with background load)",
		StartTime:   time.Now(),
		Phases:      make([]PhaseResult, 0),
	}

	defer func() {
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
	}()

	// Phase 1: Baseline (collect metrics under normal conditions)
	fmt.Println("Phase 1: Collecting baseline metrics (2 minutes)...")
	baselinePhase := PhaseResult{Name: "baseline", StartTime: time.Now()}

	baselineCtx, baselineCancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := clt.loadTester.Start(baselineCtx, clt.config.NumUsers); err != nil {
		baselineCancel()
		return nil, fmt.Errorf("failed to start baseline load: %w", err)
	}

	<-baselineCtx.Done()
	baselineCancel()
	clt.loadTester.Stop()

	baselineMetrics := clt.loadTester.GetTestMetrics()
	result.Metrics.ErrorsBeforeChaos = int(baselineMetrics.RequestsFailed)

	baselinePhase.EndTime = time.Now()
	baselinePhase.Duration = baselinePhase.EndTime.Sub(baselinePhase.StartTime)
	baselinePhase.Success = true
	baselinePhase.Details = fmt.Sprintf("Baseline: %d requests, %d errors, p99=%s",
		baselineMetrics.RequestsTotal,
		baselineMetrics.RequestsFailed,
		baselineMetrics.P99Latency)
	result.Phases = append(result.Phases, baselinePhase)

	// Reset metrics for chaos phase
	clt.loadTester.metrics = &LoadMetrics{
		latencies: make([]time.Duration, 0, 10000),
		errors:    make([]string, 0, 100),
	}

	// Phase 2: Chaos injection with load
	fmt.Println("Phase 2: Injecting chaos under load (5 minutes)...")
	chaosPhase := PhaseResult{Name: "chaos-with-load", StartTime: time.Now()}

	chaosCtx, chaosCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer chaosCancel()

	// Start load in background
	if err := clt.loadTester.Start(chaosCtx, clt.config.NumUsers); err != nil {
		return nil, fmt.Errorf("failed to start chaos load: %w", err)
	}

	// Wait 30 seconds for load to stabilize
	time.Sleep(30 * time.Second)

	// Inject chaos
	if err := clt.scenario.Run(chaosCtx); err != nil {
		clt.loadTester.Stop()
		chaosPhase.EndTime = time.Now()
		chaosPhase.Duration = chaosPhase.EndTime.Sub(chaosPhase.StartTime)
		chaosPhase.Success = false
		chaosPhase.Error = err.Error()
		result.Phases = append(result.Phases, chaosPhase)
		result.Success = false
		result.Error = fmt.Sprintf("chaos injection failed: %v", err)
		return result, nil
	}

	// Wait for remainder of chaos phase
	select {
	case <-chaosCtx.Done():
	case <-time.After(3 * time.Minute):
	}

	clt.loadTester.Stop()

	chaosMetrics := clt.loadTester.GetTestMetrics()
	result.Metrics.ErrorsDuringChaos = int(chaosMetrics.RequestsFailed)

	chaosPhase.EndTime = time.Now()
	chaosPhase.Duration = chaosPhase.EndTime.Sub(chaosPhase.StartTime)
	chaosPhase.Success = true
	chaosPhase.Details = fmt.Sprintf("Chaos: %d requests, %d errors, p99=%s",
		chaosMetrics.RequestsTotal,
		chaosMetrics.RequestsFailed,
		chaosMetrics.P99Latency)
	result.Phases = append(result.Phases, chaosPhase)

	// Reset metrics for recovery phase
	clt.loadTester.metrics = &LoadMetrics{
		latencies: make([]time.Duration, 0, 10000),
		errors:    make([]string, 0, 100),
	}

	// Phase 3: Recovery measurement
	fmt.Println("Phase 3: Measuring recovery (2 minutes)...")
	recoveryPhase := PhaseResult{Name: "recovery", StartTime: time.Now()}

	recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 2*time.Minute)

	// Verify scenario recovery
	recoveryStart := time.Now()
	if err := clt.scenario.Verify(recoveryCtx); err != nil {
		recoveryCancel()
		recoveryPhase.EndTime = time.Now()
		recoveryPhase.Duration = recoveryPhase.EndTime.Sub(recoveryPhase.StartTime)
		recoveryPhase.Success = false
		recoveryPhase.Error = err.Error()
		result.Phases = append(result.Phases, recoveryPhase)
		result.Success = false
		result.Error = fmt.Sprintf("recovery verification failed: %v", err)
		return result, nil
	}

	result.Metrics.RecoveryTime = time.Since(recoveryStart)

	// Continue load test during recovery
	if err := clt.loadTester.Start(recoveryCtx, clt.config.NumUsers); err == nil {
		<-recoveryCtx.Done()
		clt.loadTester.Stop()
	}

	recoveryCancel()

	recoveryMetrics := clt.loadTester.GetTestMetrics()
	result.Metrics.ErrorsAfterRecovery = int(recoveryMetrics.RequestsFailed)
	result.Metrics.P50Latency = recoveryMetrics.P50Latency
	result.Metrics.P95Latency = recoveryMetrics.P95Latency
	result.Metrics.P99Latency = recoveryMetrics.P99Latency

	recoveryPhase.EndTime = time.Now()
	recoveryPhase.Duration = recoveryPhase.EndTime.Sub(recoveryPhase.StartTime)
	recoveryPhase.Success = true
	recoveryPhase.Details = fmt.Sprintf("Recovery: %s, %d requests, %d errors, p99=%s",
		result.Metrics.RecoveryTime,
		recoveryMetrics.RequestsTotal,
		recoveryMetrics.RequestsFailed,
		recoveryMetrics.P99Latency)
	result.Phases = append(result.Phases, recoveryPhase)

	// Cleanup
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	clt.scenario.Cleanup(cleanupCtx)
	cleanupCancel()

	// Determine success criteria
	result.Success = result.Metrics.RecoveryTime < 2*time.Minute &&
		result.Metrics.ErrorsAfterRecovery == 0 &&
		!result.Metrics.DataLoss

	if !result.Success {
		reasons := []string{}
		if result.Metrics.RecoveryTime >= 2*time.Minute {
			reasons = append(reasons, fmt.Sprintf("recovery took %s (>2m)", result.Metrics.RecoveryTime))
		}
		if result.Metrics.ErrorsAfterRecovery > 0 {
			reasons = append(reasons, fmt.Sprintf("%d errors after recovery", result.Metrics.ErrorsAfterRecovery))
		}
		if result.Metrics.DataLoss {
			reasons = append(reasons, "data loss detected")
		}
		result.Error = fmt.Sprintf("success criteria not met: %v", reasons)
	}

	return result, nil
}

// RunChaosWithLoad is a helper function to run a chaos scenario with load
func RunChaosWithLoad(ctx context.Context, scenario ChaosScenario, cfg *Config) (*TestResult, error) {
	loadCfg := &LoadTestConfig{
		BaseURL:   cfg.BaseURL,
		Email:     cfg.Email,
		Password:  cfg.Password,
		ContestID: cfg.ContestID,
		NumUsers:  cfg.LoadUsers,
		Duration:  cfg.LoadDuration,
		OrderRate: 100 * time.Millisecond,
	}

	clt := NewChaosLoadTest(scenario, loadCfg)
	return clt.Run(ctx, nil, cfg.Namespace)
}
