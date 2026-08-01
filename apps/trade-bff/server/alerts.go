package server

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"go.uber.org/zap"
)

// AlertAggregator aggregates similar alerts to prevent notification spam.
// It uses a time window to collect occurrences and sends a summary.
type AlertAggregator struct {
	mu           sync.Mutex
	aggregations map[string]*aggregatedAlert
	window       time.Duration // Time window for aggregation (e.g., 5 minutes)
}

// aggregatedAlert tracks occurrences of a specific alert type.
type aggregatedAlert struct {
	key            string
	count          int
	firstOccurred  time.Time
	lastOccurred   time.Time
	lastSent       time.Time
	lastError      error
	additionalInfo map[string]string // Additional context from latest occurrence
}

// NewAlertAggregator creates a new alert aggregator with the given window.
func NewAlertAggregator(window time.Duration) *AlertAggregator {
	return &AlertAggregator{
		aggregations: make(map[string]*aggregatedAlert),
		window:       window,
	}
}

// ShouldSend checks if an alert should be sent and tracks the occurrence.
// Returns true if the alert should be sent, along with the aggregated count.
func (a *AlertAggregator) ShouldSend(key string, err error, info map[string]string) (shouldSend bool, count int, firstOccurred time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

	agg, exists := a.aggregations[key]
	if !exists {
		// First occurrence - send immediately
		a.aggregations[key] = &aggregatedAlert{
			key:            key,
			count:          1,
			firstOccurred:  now,
			lastOccurred:   now,
			lastSent:       now,
			lastError:      err,
			additionalInfo: info,
		}
		return true, 1, now
	}

	// Update the aggregation
	agg.count++
	agg.lastOccurred = now
	agg.lastError = err
	if info != nil {
		agg.additionalInfo = info
	}

	// Check if window has elapsed since last send
	if now.Sub(agg.lastSent) >= a.window {
		count := agg.count
		firstOccurred := agg.firstOccurred

		// Reset the aggregation
		agg.count = 0
		agg.firstOccurred = now
		agg.lastSent = now

		return true, count, firstOccurred
	}

	return false, agg.count, agg.firstOccurred
}

// Reset clears aggregation for a specific key (e.g., when issue is resolved).
func (a *AlertAggregator) Reset(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.aggregations, key)
}

// Cleanup removes aggregation entries that have not been active for longer than maxAge.
func (a *AlertAggregator) Cleanup(maxAge time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for key, agg := range a.aggregations {
		if now.Sub(agg.lastOccurred) > maxAge {
			delete(a.aggregations, key)
		}
	}
}

// ConnectionAnomalyDetector detects unusual connection patterns (possible DDoS).
type ConnectionAnomalyDetector struct {
	mu sync.RWMutex

	// Track connections per IP prefix (e.g., /24 for IPv4)
	ipPrefixCounts map[string]*connectionStats

	// Track authentication failures
	authFailures map[string]*authFailureStats

	// Configuration
	connectionThreshold int           // Max connections from single IP prefix
	authFailureWindow   time.Duration // Window for auth failures
	authFailureLimit    int           // Max auth failures before alerting

	// Baseline tracking
	normalConnectionRate atomic.Int64
	peakConnections      atomic.Int64
	lastPeakTime         atomic.Int64
}

// connectionStats tracks connection statistics for an IP prefix.
type connectionStats struct {
	count       int
	firstSeen   time.Time
	lastSeen    time.Time
	connections []time.Time // Recent connection timestamps
}

// authFailureStats tracks authentication failure statistics.
type authFailureStats struct {
	count     int
	firstSeen time.Time
	lastSeen  time.Time
	userIDs   map[string]int // User IDs attempted
	endpoints map[string]int // Endpoints attempted
}

// NewConnectionAnomalyDetector creates a new detector with default thresholds.
func NewConnectionAnomalyDetector() *ConnectionAnomalyDetector {
	return &ConnectionAnomalyDetector{
		ipPrefixCounts:      make(map[string]*connectionStats),
		authFailures:        make(map[string]*authFailureStats),
		connectionThreshold: 50,              // 50 connections from same /24
		authFailureWindow:   5 * time.Minute, // 5 minute window
		authFailureLimit:    20,              // 20 failures triggers alert
	}
}

// TrackConnection tracks a new connection and returns anomaly info if detected.
func (d *ConnectionAnomalyDetector) TrackConnection(remoteAddr string) (isAnomaly bool, prefix string, count int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	prefix = d.getIPPrefix(remoteAddr)
	now := time.Now()

	stats, exists := d.ipPrefixCounts[prefix]
	if !exists {
		d.ipPrefixCounts[prefix] = &connectionStats{
			count:       1,
			firstSeen:   now,
			lastSeen:    now,
			connections: []time.Time{now},
		}
		return false, prefix, 1
	}

	// Clean old connections (older than 1 minute)
	cutoff := now.Add(-1 * time.Minute)
	newConnections := make([]time.Time, 0, len(stats.connections))
	for _, t := range stats.connections {
		if t.After(cutoff) {
			newConnections = append(newConnections, t)
		}
	}
	newConnections = append(newConnections, now)
	stats.connections = newConnections
	stats.count = len(newConnections)
	stats.lastSeen = now

	if stats.count > d.connectionThreshold {
		return true, prefix, stats.count
	}

	return false, prefix, stats.count
}

// TrackAuthFailure tracks an authentication failure.
func (d *ConnectionAnomalyDetector) TrackAuthFailure(remoteAddr, userID, endpoint string) (isSpike bool, count int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	prefix := d.getIPPrefix(remoteAddr)
	now := time.Now()

	stats, exists := d.authFailures[prefix]
	if !exists {
		d.authFailures[prefix] = &authFailureStats{
			count:     1,
			firstSeen: now,
			lastSeen:  now,
			userIDs:   map[string]int{userID: 1},
			endpoints: map[string]int{endpoint: 1},
		}
		return false, 1
	}

	// Reset if outside window
	if now.Sub(stats.firstSeen) > d.authFailureWindow {
		stats.count = 1
		stats.firstSeen = now
		stats.lastSeen = now
		stats.userIDs = map[string]int{userID: 1}
		stats.endpoints = map[string]int{endpoint: 1}
		return false, 1
	}

	stats.count++
	stats.lastSeen = now
	stats.userIDs[userID]++
	stats.endpoints[endpoint]++

	if stats.count > d.authFailureLimit {
		return true, stats.count
	}

	return false, stats.count
}

// UpdatePeakConnections updates peak connection tracking.
func (d *ConnectionAnomalyDetector) UpdatePeakConnections(current int64) (isNewPeak bool) {
	old := d.peakConnections.Load()
	if current > old {
		d.peakConnections.Store(current)
		d.lastPeakTime.Store(time.Now().Unix())
		return true
	}
	return false
}

// GetPeakConnections returns the peak connection count and when it occurred.
func (d *ConnectionAnomalyDetector) GetPeakConnections() (peak int64, when time.Time) {
	peak = d.peakConnections.Load()
	when = time.Unix(d.lastPeakTime.Load(), 0)
	return
}

// getIPPrefix extracts the /24 prefix for IPv4 or /48 for IPv6.
func (d *ConnectionAnomalyDetector) getIPPrefix(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return addr
	}

	if ip.To4() != nil {
		// IPv4: use /24 prefix
		parts := strings.Split(ip.String(), ".")
		if len(parts) >= 3 {
			return strings.Join(parts[:3], ".") + ".0/24"
		}
	} else {
		// IPv6: use /48 prefix
		parts := strings.Split(ip.String(), ":")
		if len(parts) >= 3 {
			return strings.Join(parts[:3], ":") + "::/48"
		}
	}
	return addr
}

// Cleanup removes stale entries from the detector.
func (d *ConnectionAnomalyDetector) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-5 * time.Minute)

	// Clean up IP prefix counts
	for prefix, stats := range d.ipPrefixCounts {
		if stats.lastSeen.Before(cutoff) {
			delete(d.ipPrefixCounts, prefix)
		}
	}

	// Clean up auth failures
	for prefix, stats := range d.authFailures {
		if stats.lastSeen.Before(cutoff) {
			delete(d.authFailures, prefix)
		}
	}
}

// LatencyTracker tracks P99 latency over a rolling window.
type LatencyTracker struct {
	mu            sync.Mutex
	samples       []time.Duration
	sampleSize    int
	highLatencyAt time.Time // When high latency was first detected
}

// NewLatencyTracker creates a new latency tracker.
func NewLatencyTracker(sampleSize int) *LatencyTracker {
	return &LatencyTracker{
		samples:    make([]time.Duration, 0, sampleSize),
		sampleSize: sampleSize,
	}
}

// Record records a latency sample.
func (t *LatencyTracker) Record(latency time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.samples = append(t.samples, latency)
	if len(t.samples) > t.sampleSize {
		t.samples = t.samples[1:]
	}
}

// P99 returns the P99 latency.
func (t *LatencyTracker) P99() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.computeP99Locked()
}

// computeP99Locked computes the P99 latency. Caller must hold t.mu.
func (t *LatencyTracker) computeP99Locked() time.Duration {
	if len(t.samples) == 0 {
		return 0
	}

	sorted := make([]time.Duration, len(t.samples))
	copy(sorted, t.samples)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	idx := int(float64(len(sorted)) * 0.99)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// CheckHighLatency checks if P99 has been high for the sustained duration.
func (t *LatencyTracker) CheckHighLatency(threshold, sustainedDuration time.Duration) (isHigh bool, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	p99 := t.computeP99Locked()

	if p99 > threshold {
		if t.highLatencyAt.IsZero() {
			t.highLatencyAt = time.Now()
		}
		duration = time.Since(t.highLatencyAt)
		return duration >= sustainedDuration, duration
	}

	// Reset if latency is back to normal
	t.highLatencyAt = time.Time{}
	return false, 0
}

// initNotifications initializes the notification service.
func (a *App) initNotifications(ctx context.Context, log *zap.Logger) {
	cfg := a.config

	// Skip if Discord and Resend are not configured
	if cfg.DiscordWebhookURL == "" && cfg.ResendAPIKey == "" {
		log.Info("Notifications disabled (no Discord or Resend configured)")
		return
	}

	// Parse email recipients
	var emailRecipients []string
	if cfg.NotificationRecipients != "" {
		recipients := strings.Split(cfg.NotificationRecipients, ",")
		for _, r := range recipients {
			r = strings.TrimSpace(r)
			if r != "" {
				emailRecipients = append(emailRecipients, r)
			}
		}
	}

	// Build service config
	svcCfg := notification.ServiceConfig{
		Enabled:         cfg.NotificationEnabled,
		AsyncEnabled:    cfg.NotificationAsync,
		AsyncWorkers:    cfg.NotificationAsyncWorkers,
		AsyncQueueSize:  cfg.NotificationQueueSize,
		Environment:     cfg.Environment,
		ServiceName:     "trade-bff",
		EmailRecipients: emailRecipients,
		Discord: notification.DiscordConfig{
			Enabled:    cfg.DiscordWebhookURL != "",
			WebhookURL: cfg.DiscordWebhookURL,
			Username:   "trade-bff",
		},
		Email: notification.EmailConfig{
			Enabled:    cfg.ResendAPIKey != "",
			APIKey:     cfg.ResendAPIKey,
			FromEmail:  cfg.ResendFromEmail,
			Recipients: emailRecipients,
		},
	}

	// Create notification service
	svc, err := notification.NewService(ctx, svcCfg, log, a.obs.Metrics.Registry())
	if err != nil {
		log.Warn("Failed to initialize notification service", zap.Error(err))
		return
	}

	a.notifications = svc

	// Initialize alert aggregator with 5-minute window
	a.alertAggregator = NewAlertAggregator(5 * time.Minute)

	// Initialize anomaly detector
	a.anomalyDetector = NewConnectionAnomalyDetector()

	// Initialize latency tracker (1000 samples)
	a.latencyTracker = NewLatencyTracker(1000)

	// Start cleanup goroutine for anomaly detector
	infra.SafeGo(log, "anomaly-detector-cleanup", func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.anomalyDetector.Cleanup()
				a.alertAggregator.Cleanup(10 * time.Minute)
			}
		}
	})

	log.Info("Notification service initialized",
		zap.Bool("discord", svc.HasDiscord()),
		zap.Bool("email", svc.HasEmail()),
		zap.Bool("async", cfg.NotificationAsync))
}

// sendStartupNotification sends a notification when the service starts.
func (a *App) sendStartupNotification() {
	if a.notifications == nil {
		return
	}

	metadata := map[string]string{
		"port":                a.config.Port,
		"compression_enabled": fmt.Sprintf("%v", a.config.EnableCompression),
		"broadcast_workers":   fmt.Sprintf("%d", a.config.BroadcastWorkers),
		"shard_router_addr":   a.config.ShardRouterAddr,
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Trade BFF Started",
		Message:  fmt.Sprintf("WebSocket gateway started on port %s with %d broadcast workers", a.config.Port, a.config.BroadcastWorkers),
		Service:  "trade-bff",
		Metadata: metadata,
	}

	a.notifications.SendAlertAsync(alert)
}

// sendShutdownNotification sends a notification when the service is shutting down.
func (a *App) sendShutdownNotification() {
	if a.notifications == nil {
		return
	}

	activeConnections := a.metrics.wsConnections.Load()

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Trade BFF Shutting Down",
		Message:  "WebSocket gateway is performing graceful shutdown",
		Service:  "trade-bff",
		Metadata: map[string]string{
			"active_connections": fmt.Sprintf("%d", activeConnections),
		},
	}

	// Send synchronously to ensure it's delivered before shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.notifications.SendAlert(ctx, alert); err != nil {
		a.log().Warn("Failed to send shutdown notification", zap.Error(err))
	}
}

// sendConnectionSpike sends an alert when connection count spikes.
func (a *App) sendConnectionSpike(currentCount int, delta int, duration time.Duration) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := "connection_spike"
	info := map[string]string{
		"current_count": fmt.Sprintf("%d", currentCount),
		"delta":         fmt.Sprintf("%d", delta),
		"duration":      duration.String(),
	}

	shouldSend, count, _ := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	percentIncrease := float64(delta) / float64(currentCount-delta) * 100

	var message string
	if count > 1 {
		message = fmt.Sprintf("Connection spike detected (%d occurrences): %d connections (+%d, %.1f%% increase in %v)",
			count, currentCount, delta, percentIncrease, duration)
	} else {
		message = fmt.Sprintf("Connection spike detected: %d connections (+%d, %.1f%% increase in %v)",
			currentCount, delta, percentIncrease, duration)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityMedium,
		Title:    "WebSocket Connection Spike",
		Message:  message,
		Service:  "trade-bff",
		Metadata: map[string]string{
			"current_connections": fmt.Sprintf("%d", currentCount),
			"delta":               fmt.Sprintf("%d", delta),
			"percent_increase":    fmt.Sprintf("%.1f%%", percentIncrease),
			"occurrence_count":    fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendConnectionDrop sends an alert when connection count drops significantly.
func (a *App) sendConnectionDrop(currentCount int, delta int, reason string) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := "connection_drop"
	info := map[string]string{
		"current_count": fmt.Sprintf("%d", currentCount),
		"delta":         fmt.Sprintf("%d", delta),
		"reason":        reason,
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	previousCount := currentCount + delta
	var percentDrop float64
	if previousCount > 0 {
		percentDrop = float64(delta) / float64(previousCount) * 100
	}

	var message string
	var severity notification.Severity

	if percentDrop > 30 {
		severity = notification.SeverityHigh
		if count > 1 {
			duration := time.Since(firstOccurred)
			message = fmt.Sprintf("SIGNIFICANT connection drop (%d occurrences in %v): %d -> %d connections (%.1f%% drop). Reason: %s",
				count, duration.Round(time.Second), previousCount, currentCount, percentDrop, reason)
		} else {
			message = fmt.Sprintf("SIGNIFICANT connection drop: %d -> %d connections (%.1f%% drop). Reason: %s",
				previousCount, currentCount, percentDrop, reason)
		}
	} else {
		severity = notification.SeverityMedium
		message = fmt.Sprintf("Connection drop detected: %d -> %d connections (%.1f%% drop). Reason: %s",
			previousCount, currentCount, percentDrop, reason)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    "WebSocket Connection Drop",
		Message:  message,
		Service:  "trade-bff",
		Metadata: map[string]string{
			"current_connections":  fmt.Sprintf("%d", currentCount),
			"previous_connections": fmt.Sprintf("%d", previousCount),
			"percent_drop":         fmt.Sprintf("%.1f%%", percentDrop),
			"reason":               reason,
			"occurrence_count":     fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendWebSocketError sends an alert when WebSocket errors occur.
func (a *App) sendWebSocketError(err error, connectionCount int) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := "ws_error"
	info := map[string]string{
		"error":            err.Error(),
		"connection_count": fmt.Sprintf("%d", connectionCount),
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	var message string

	// Calculate error rate (simplified - based on occurrence count vs connections)
	errorRate := float64(count) / float64(connectionCount+1) * 100

	if errorRate > 5 {
		severity = notification.SeverityHigh
	} else {
		severity = notification.SeverityMedium
	}

	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("WebSocket errors detected (%d occurrences in %v, ~%.1f%% error rate): %v",
			count, duration.Round(time.Second), errorRate, err)
	} else {
		message = fmt.Sprintf("WebSocket error: %v (active connections: %d)", err, connectionCount)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    "WebSocket Error",
		Message:  message,
		Service:  "trade-bff",
		Metadata: map[string]string{
			"error":              err.Error(),
			"active_connections": fmt.Sprintf("%d", connectionCount),
			"error_rate":         fmt.Sprintf("%.1f%%", errorRate),
			"occurrence_count":   fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendRateLimitTriggered sends an alert when rate limiting is triggered.
func (a *App) sendRateLimitTriggered(userID string, endpoint string, limit int) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("rate_limit:%s", endpoint)
	info := map[string]string{
		"user_id":  userID,
		"endpoint": endpoint,
		"limit":    fmt.Sprintf("%d", limit),
	}

	shouldSend, count, _ := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	if count > 10 {
		severity = notification.SeverityMedium
	} else {
		severity = notification.SeverityLow
	}

	var message string
	if count > 1 {
		message = fmt.Sprintf("Rate limiting triggered for multiple users (%d times) on endpoint %s",
			count, endpoint)
	} else {
		message = fmt.Sprintf("Rate limiting triggered for user on endpoint %s (limit: %d)",
			endpoint, limit)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    "Rate Limit Triggered",
		Message:  message,
		Service:  "trade-bff",
		Metadata: map[string]string{
			"endpoint":         endpoint,
			"limit":            fmt.Sprintf("%d", limit),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendCircuitBreakerTripped sends an alert when a circuit breaker trips.
func (a *App) sendCircuitBreakerTripped(target string, state circuitbreaker.State) {
	if a.notifications == nil {
		return
	}

	var severity notification.Severity
	var title, message string

	switch state {
	case circuitbreaker.StateOpen:
		// Check if this is a critical circuit
		if target == "postgres" || target == "kafka" {
			severity = notification.SeverityCritical
			title = fmt.Sprintf("CRITICAL: Circuit Breaker Open - %s", target)
			message = fmt.Sprintf("Critical dependency %s circuit breaker is OPEN. Service degradation expected.", target)
		} else {
			severity = notification.SeverityMedium
			title = fmt.Sprintf("Circuit Breaker Tripped: %s", target)
			message = fmt.Sprintf("Circuit breaker for %s has tripped and is now OPEN.", target)
		}
	case circuitbreaker.StateHalfOpen:
		severity = notification.SeverityLow
		title = fmt.Sprintf("Circuit Breaker Recovery: %s", target)
		message = fmt.Sprintf("Circuit breaker for %s is now HALF-OPEN, testing recovery.", target)
	case circuitbreaker.StateClosed:
		severity = notification.SeverityLow
		title = fmt.Sprintf("Circuit Breaker Recovered: %s", target)
		message = fmt.Sprintf("Circuit breaker for %s has recovered and is now CLOSED.", target)
	default:
		return
	}

	// Get circuit breaker metrics
	cbMetrics := a.circuits.Metrics()
	metrics, ok := cbMetrics[target]

	metadata := map[string]string{
		"target": target,
		"state":  state.String(),
	}

	if ok {
		metadata["total_requests"] = fmt.Sprintf("%d", metrics.TotalRequests)
		metadata["total_failures"] = fmt.Sprintf("%d", metrics.TotalFailures)
		metadata["consecutive_failures"] = fmt.Sprintf("%d", metrics.ConsecutiveFailures)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    title,
		Message:  message,
		Service:  "trade-bff",
		Metadata: metadata,
	}

	a.notifications.SendAlertAsync(alert)
}

// sendAllCircuitsOpen sends a critical alert when all circuit breakers are open.
func (a *App) sendAllCircuitsOpen() {
	if a.notifications == nil {
		return
	}

	status := a.circuits.Status()
	openCircuits := []string{}
	for name, state := range status {
		if state == "open" {
			openCircuits = append(openCircuits, name)
		}
	}

	if len(openCircuits) == 0 {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityCritical,
		Title:    "CRITICAL: All Circuit Breakers Open",
		Message:  fmt.Sprintf("All downstream service circuits are OPEN: %s. Service is degraded!", strings.Join(openCircuits, ", ")),
		Service:  "trade-bff",
		Metadata: map[string]string{
			"open_circuits":      strings.Join(openCircuits, ", "),
			"active_connections": fmt.Sprintf("%d", a.metrics.wsConnections.Load()),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendBatcherOverflow sends an alert when the message batcher queue overflows.
func (a *App) sendBatcherOverflow(queueSize int, dropped int) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := "batcher_overflow"
	info := map[string]string{
		"queue_size": fmt.Sprintf("%d", queueSize),
		"dropped":    fmt.Sprintf("%d", dropped),
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	var message string

	if count > 10 || dropped > 100 {
		severity = notification.SeverityCritical
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("CRITICAL: Message queue overflow with data loss! %d messages dropped (%d overflow events in %v)",
			dropped, count, duration.Round(time.Second))
	} else {
		severity = notification.SeverityMedium
		message = fmt.Sprintf("Message queue overflow: %d messages dropped, queue size: %d", dropped, queueSize)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    "Message Batcher Overflow",
		Message:  message,
		Service:  "trade-bff",
		Metadata: map[string]string{
			"queue_size":       fmt.Sprintf("%d", queueSize),
			"dropped_messages": fmt.Sprintf("%d", dropped),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendHighLatency sends an alert when P99 latency is high for sustained period.
func (a *App) sendHighLatency(p99Latency time.Duration, endpoint string) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("high_latency:%s", endpoint)
	info := map[string]string{
		"p99_latency": p99Latency.String(),
		"endpoint":    endpoint,
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	duration := time.Since(firstOccurred)
	var severity notification.Severity

	if duration > 5*time.Minute {
		severity = notification.SeverityHigh
	} else {
		severity = notification.SeverityMedium
	}

	message := fmt.Sprintf("High P99 latency detected on %s: %v (sustained for %v, %d samples)",
		endpoint, p99Latency.Round(time.Millisecond), duration.Round(time.Second), count)

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("High Latency: %s", endpoint),
		Message:  message,
		Service:  "trade-bff",
		Metadata: map[string]string{
			"endpoint":         endpoint,
			"p99_latency_ms":   fmt.Sprintf("%.0f", float64(p99Latency.Milliseconds())),
			"sustained_for":    duration.Round(time.Second).String(),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendServerUnableToAcceptConnections sends a critical alert when the server can't accept connections.
func (a *App) sendServerUnableToAcceptConnections(err error) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityCritical,
		Title:    "CRITICAL: WebSocket Server Unable to Accept Connections",
		Message:  fmt.Sprintf("WebSocket server cannot accept new connections: %v", err),
		Service:  "trade-bff",
		Metadata: map[string]string{
			"error":              err.Error(),
			"active_connections": fmt.Sprintf("%d", a.metrics.wsConnections.Load()),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendPeakConnectionsReached sends an info alert when new peak connections are reached.
func (a *App) sendPeakConnectionsReached(peak int64) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityLow,
		Title:    "Peak Connections Reached",
		Message:  fmt.Sprintf("New peak connection count reached: %d connections", peak),
		Service:  "trade-bff",
		Metadata: map[string]string{
			"peak_connections": fmt.Sprintf("%d", peak),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendCompressionRatioDegraded sends an alert when compression ratio degrades (possible attack).
func (a *App) sendCompressionRatioDegraded(ratio float64, expected float64) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := "compression_degraded"
	info := map[string]string{
		"actual_ratio":   fmt.Sprintf("%.2f", ratio),
		"expected_ratio": fmt.Sprintf("%.2f", expected),
	}

	shouldSend, count, _ := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	message := fmt.Sprintf("Compression ratio degraded: %.1f%% (expected ~%.1f%%). "+
		"This could indicate unusual traffic patterns or a compression attack. (%d occurrences)",
		ratio*100, expected*100, count)

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityLow,
		Title:    "Compression Ratio Degraded",
		Message:  message,
		Service:  "trade-bff",
		Metadata: map[string]string{
			"actual_ratio":     fmt.Sprintf("%.2f", ratio),
			"expected_ratio":   fmt.Sprintf("%.2f", expected),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendConnectionAnomalyDetected sends an alert when connection anomaly is detected (possible DDoS).
func (a *App) sendConnectionAnomalyDetected(ipPrefix string, connectionCount int) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityMedium,
		Title:    "Connection Anomaly Detected",
		Message:  fmt.Sprintf("Unusual connection pattern detected from IP range %s: %d connections in last minute. Possible DDoS attempt.", ipPrefix, connectionCount),
		Service:  "trade-bff",
		Metadata: map[string]string{
			"ip_prefix":        ipPrefix,
			"connection_count": fmt.Sprintf("%d", connectionCount),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendAuthFailureSpike sends an alert when authentication failure spike is detected.
func (a *App) sendAuthFailureSpike(ipPrefix string, failureCount int) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityMedium,
		Title:    "Authentication Failure Spike",
		Message:  fmt.Sprintf("High number of authentication failures from IP range %s: %d failures in last 5 minutes. Possible credential stuffing attack.", ipPrefix, failureCount),
		Service:  "trade-bff",
		Metadata: map[string]string{
			"ip_prefix":     ipPrefix,
			"failure_count": fmt.Sprintf("%d", failureCount),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// getWSMetrics returns current WebSocket metrics for inclusion in alerts.
func (a *App) getWSMetrics() map[string]string {
	stats := a.metrics.GetStats()
	metrics := make(map[string]string)

	metrics["active_connections"] = fmt.Sprintf("%d", stats["ws_connections"])
	uptime := time.Since(a.startedAt).Seconds()
	if uptime < 1 {
		uptime = 1
	}
	metrics["messages_per_second"] = fmt.Sprintf("%.1f", float64(a.hub.batcher.GetSequence())/uptime)
	metrics["compression_ratio"] = fmt.Sprintf("%.2f", stats["ws_compression_ratio"])
	metrics["dropped_messages"] = fmt.Sprintf("%d", stats["ws_dropped_messages_total"])

	return metrics
}

