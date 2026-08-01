package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"go.uber.org/zap"
)

// ShardMetrics holds metrics for a specific shard.
type ShardMetrics struct {
	ShardID           string  `json:"shard_id"`
	AssignedContests  int     `json:"assigned_contests"`
	ActiveConnections int     `json:"active_connections"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	AvgLatencyMs      float64 `json:"avg_latency_ms"`
	P99LatencyMs      float64 `json:"p99_latency_ms"`
	ErrorRate         float64 `json:"error_rate"`
	HealthStatus      string  `json:"health_status"`
}

// ShardHealthReport holds aggregate health data for all shards.
type ShardHealthReport struct {
	TotalShards     int           `json:"total_shards"`
	HealthyShards   int           `json:"healthy_shards"`
	UnhealthyShards int           `json:"unhealthy_shards"`
	DrainingShards  int           `json:"draining_shards"`
	UptimePercent   float64       `json:"uptime_percent"`
	FailoverCount   int           `json:"failover_count"`
	RoutingErrors   int           `json:"routing_errors"`
	IncidentCount   int           `json:"incident_count"`
	Period          time.Duration `json:"period"`
}

// AlertAggregator aggregates similar alerts to prevent notification spam.
type AlertAggregator struct {
	mu           sync.Mutex
	aggregations map[string]*aggregatedAlert
	window       time.Duration
}

type aggregatedAlert struct {
	key            string
	count          int
	firstOccurred  time.Time
	lastOccurred   time.Time
	lastSent       time.Time
	lastError      error
	additionalInfo map[string]string
}

// NewAlertAggregator creates a new alert aggregator with the given window.
func NewAlertAggregator(window time.Duration) *AlertAggregator {
	return &AlertAggregator{
		aggregations: make(map[string]*aggregatedAlert),
		window:       window,
	}
}

// alertTTL is the maximum age for an alert aggregation entry before it is evicted.
const alertTTL = 1 * time.Hour

// ShouldSend checks if an alert should be sent and tracks the occurrence.
// Stale aggregation entries older than alertTTL are evicted to prevent unbounded memory growth.
func (a *AlertAggregator) ShouldSend(key string, err error, info map[string]string) (shouldSend bool, count int, firstOccurred time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

	// Periodically evict stale entries to prevent memory leak
	a.evictStaleLocked(now)

	agg, exists := a.aggregations[key]
	if !exists {
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

	agg.count++
	agg.lastOccurred = now
	agg.lastError = err
	if info != nil {
		agg.additionalInfo = info
	}

	if now.Sub(agg.lastSent) >= a.window {
		count := agg.count
		firstOccurred := agg.firstOccurred
		agg.count = 0
		agg.firstOccurred = now
		agg.lastSent = now
		return true, count, firstOccurred
	}

	return false, agg.count, agg.firstOccurred
}

// evictStaleLocked removes aggregation entries that haven't been updated for alertTTL.
// Must be called with a.mu held.
func (a *AlertAggregator) evictStaleLocked(now time.Time) {
	for key, agg := range a.aggregations {
		if now.Sub(agg.lastOccurred) > alertTTL {
			delete(a.aggregations, key)
		}
	}
}

// Reset clears aggregation for a specific key.
func (a *AlertAggregator) Reset(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.aggregations, key)
}

// Notifier wraps the notification service with shard-router specific methods.
type Notifier struct {
	svc        *notification.Service
	aggregator *AlertAggregator
	config     Config
	logger     *zap.Logger
}

// NewNotifier creates a new Notifier instance.
func NewNotifier(svc *notification.Service, config Config, logger *zap.Logger) *Notifier {
	return &Notifier{
		svc:        svc,
		aggregator: NewAlertAggregator(5 * time.Minute),
		config:     config,
		logger:     logger.With(zap.String("component", "notifier")),
	}
}

// initNotifications initializes the notification service.
func initNotifications(ctx context.Context, cfg Config, logger *zap.Logger, registry interface{ Register(interface{}) error }) *notification.Service {
	// Skip if Discord and Resend are not configured
	if cfg.DiscordWebhookURL == "" && cfg.ResendAPIKey == "" {
		logger.Info("Notifications disabled (no Discord or Resend configured)")
		return nil
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
		ServiceName:     "shard-router",
		EmailRecipients: emailRecipients,
		Discord: notification.DiscordConfig{
			Enabled:    cfg.DiscordWebhookURL != "",
			WebhookURL: cfg.DiscordWebhookURL,
			Username:   "shard-router",
		},
		Email: notification.EmailConfig{
			Enabled:    cfg.ResendAPIKey != "",
			APIKey:     cfg.ResendAPIKey,
			FromEmail:  cfg.ResendFromEmail,
			Recipients: emailRecipients,
		},
	}

	// Create notification service
	svc, err := notification.NewService(ctx, svcCfg, logger, nil)
	if err != nil {
		logger.Warn("Failed to initialize notification service", zap.Error(err))
		return nil
	}

	logger.Info("Notification service initialized",
		zap.Bool("discord", svc.HasDiscord()),
		zap.Bool("email", svc.HasEmail()),
		zap.Bool("async", cfg.NotificationAsync))

	return svc
}

// sendStartupNotification sends a notification when the service starts.
func (n *Notifier) sendStartupNotification(activeShards, totalShards int) {
	if n.svc == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Shard Router Started",
		Message:  fmt.Sprintf("Shard router started with %d/%d active shards", activeShards, totalShards),
		Service:  "shard-router",
		Metadata: map[string]string{
			"port":          n.config.Port,
			"active_shards": fmt.Sprintf("%d", activeShards),
			"total_shards":  fmt.Sprintf("%d", totalShards),
			"virtual_nodes": fmt.Sprintf("%d", n.config.VirtualNodes),
			"event":         "startup",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendShutdownNotification sends a notification when the service is shutting down.
func (n *Notifier) sendShutdownNotification() {
	if n.svc == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Shard Router Shutting Down",
		Message:  "Shard router is performing graceful shutdown",
		Service:  "shard-router",
		Metadata: map[string]string{
			"event": "shutdown",
		},
	}

	// Send synchronously to ensure it's delivered before shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := n.svc.SendAlert(ctx, alert); err != nil {
		n.logger.Warn("Failed to send shutdown notification", zap.Error(err))
	}
}

// sendShardUnhealthy sends an alert when a shard becomes unhealthy.
// CRITICAL if multiple shards are unhealthy (> 50%), HIGH if single shard unhealthy for > 2 minutes.
func (n *Notifier) sendShardUnhealthy(shardID string, reason string, affectedContests []string, totalShards, unhealthyCount int) {
	if n.svc == nil || n.aggregator == nil {
		return
	}

	key := fmt.Sprintf("shard_unhealthy:%s", shardID)
	info := map[string]string{
		"shard_id": shardID,
		"reason":   reason,
	}

	shouldSend, count, firstOccurred := n.aggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	// Determine severity based on impact
	var severity notification.Severity
	var title string

	unhealthyPercent := float64(unhealthyCount) / float64(totalShards) * 100
	if unhealthyPercent > 50 {
		// CRITICAL: Multiple shards unhealthy (> 50% of shards)
		severity = notification.SeverityCritical
		title = fmt.Sprintf("CRITICAL: Multiple Shards Unhealthy (%.0f%%)", unhealthyPercent)
	} else if count > 1 || time.Since(firstOccurred) > n.config.UnhealthyShardDuration {
		// HIGH: Single shard unhealthy for > 2 minutes
		severity = notification.SeverityHigh
		title = fmt.Sprintf("Shard Unhealthy: %s", shardID)
	} else {
		// MEDIUM: Initial detection
		severity = notification.SeverityMedium
		title = fmt.Sprintf("Shard Health Warning: %s", shardID)
	}

	contestList := "none"
	if len(affectedContests) > 0 {
		if len(affectedContests) > 5 {
			contestList = fmt.Sprintf("%s... and %d more", strings.Join(affectedContests[:5], ", "), len(affectedContests)-5)
		} else {
			contestList = strings.Join(affectedContests, ", ")
		}
	}

	var message string
	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("Shard %s has been unhealthy for %v (%d occurrences). Reason: %s. Affected contests: %s",
			shardID, duration.Round(time.Second), count, reason, contestList)
	} else {
		message = fmt.Sprintf("Shard %s is unhealthy. Reason: %s. Affected contests: %s",
			shardID, reason, contestList)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    title,
		Message:  message,
		Service:  "shard-router",
		Metadata: map[string]string{
			"shard_id":           shardID,
			"reason":             reason,
			"affected_contests":  fmt.Sprintf("%d", len(affectedContests)),
			"total_shards":       fmt.Sprintf("%d", totalShards),
			"unhealthy_shards":   fmt.Sprintf("%d", unhealthyCount),
			"unhealthy_percent":  fmt.Sprintf("%.1f", unhealthyPercent),
			"occurrence_count":   fmt.Sprintf("%d", count),
			"event":              "shard_unhealthy",
		},
	}

	// For critical alerts, send synchronously
	if severity == notification.SeverityCritical {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := n.svc.SendAlert(ctx, alert); err != nil {
			n.logger.Error("Failed to send critical shard unhealthy alert",
				zap.String("shard_id", shardID),
				zap.Error(err))
		}
	} else {
		n.svc.SendAlertAsync(alert)
	}
}

// sendShardRecovered sends an alert when a shard recovers after an outage.
// MEDIUM severity for recovery notifications.
func (n *Notifier) sendShardRecovered(shardID string, downtime time.Duration) {
	if n.svc == nil {
		return
	}

	// Clear the unhealthy aggregation
	if n.aggregator != nil {
		n.aggregator.Reset(fmt.Sprintf("shard_unhealthy:%s", shardID))
	}

	// Only send notification for significant downtime (> 30s)
	if downtime < 30*time.Second {
		return
	}

	var severity notification.Severity
	if downtime > 5*time.Minute {
		severity = notification.SeverityMedium
	} else {
		severity = notification.SeverityLow
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("Shard Recovered: %s", shardID),
		Message:  fmt.Sprintf("Shard %s has recovered after %v downtime", shardID, downtime.Round(time.Second)),
		Service:  "shard-router",
		Metadata: map[string]string{
			"shard_id":         shardID,
			"downtime_seconds": fmt.Sprintf("%.0f", downtime.Seconds()),
			"event":            "shard_recovered",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendShardFailover sends an alert when a shard failover occurs.
// HIGH severity for failover events.
func (n *Notifier) sendShardFailover(fromShard, toShard string, reason string) {
	if n.svc == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityHigh,
		Title:    "Shard Failover Occurred",
		Message:  fmt.Sprintf("Failover from shard %s to shard %s. Reason: %s", fromShard, toShard, reason),
		Service:  "shard-router",
		Metadata: map[string]string{
			"from_shard": fromShard,
			"to_shard":   toShard,
			"reason":     reason,
			"event":      "shard_failover",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendAllShardsHealthy sends a periodic health report when all shards are healthy.
// LOW severity for healthy status reports.
func (n *Notifier) sendAllShardsHealthy(totalShards, activeConnections int) {
	if n.svc == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "All Shards Healthy",
		Message:  fmt.Sprintf("All %d shards are healthy with %d active connections", totalShards, activeConnections),
		Service:  "shard-router",
		Metadata: map[string]string{
			"total_shards":       fmt.Sprintf("%d", totalShards),
			"active_connections": fmt.Sprintf("%d", activeConnections),
			"event":              "all_shards_healthy",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendShardOverloaded sends an alert when a shard is overloaded.
// MEDIUM severity for load warnings, HIGH if severe.
func (n *Notifier) sendShardOverloaded(shardID string, metrics ShardMetrics) {
	if n.svc == nil || n.aggregator == nil {
		return
	}

	key := fmt.Sprintf("shard_overloaded:%s", shardID)
	info := map[string]string{
		"shard_id": shardID,
		"rps":      fmt.Sprintf("%.2f", metrics.RequestsPerSecond),
	}

	shouldSend, count, _ := n.aggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	// If error rate is high or latency is very high, escalate to HIGH
	if metrics.ErrorRate > 0.1 || metrics.P99LatencyMs > 500 {
		severity = notification.SeverityHigh
	} else {
		severity = notification.SeverityMedium
	}

	message := fmt.Sprintf("Shard %s is overloaded. RPS: %.2f, P99 Latency: %.2fms, Error Rate: %.2f%%, Connections: %d",
		shardID, metrics.RequestsPerSecond, metrics.P99LatencyMs, metrics.ErrorRate*100, metrics.ActiveConnections)

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("Shard Overloaded: %s", shardID),
		Message:  message,
		Service:  "shard-router",
		Metadata: map[string]string{
			"shard_id":           shardID,
			"assigned_contests":  fmt.Sprintf("%d", metrics.AssignedContests),
			"active_connections": fmt.Sprintf("%d", metrics.ActiveConnections),
			"rps":                fmt.Sprintf("%.2f", metrics.RequestsPerSecond),
			"avg_latency_ms":     fmt.Sprintf("%.2f", metrics.AvgLatencyMs),
			"p99_latency_ms":     fmt.Sprintf("%.2f", metrics.P99LatencyMs),
			"error_rate":         fmt.Sprintf("%.4f", metrics.ErrorRate),
			"occurrence_count":   fmt.Sprintf("%d", count),
			"event":              "shard_overloaded",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendRoutingError sends an alert when a routing error occurs.
// Aggregates errors to prevent spam.
func (n *Notifier) sendRoutingError(userID, contestID string, err error) {
	if n.svc == nil || n.aggregator == nil {
		return
	}

	key := "routing_error"
	info := map[string]string{
		"user_id":    userID,
		"contest_id": contestID,
		"error":      err.Error(),
	}

	shouldSend, count, firstOccurred := n.aggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	if count > 10 {
		severity = notification.SeverityHigh
	} else if count > 3 {
		severity = notification.SeverityMedium
	} else {
		severity = notification.SeverityLow
	}

	var message string
	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("Routing errors occurred (%d times in %v). Latest: user=%s, contest=%s. Error: %v",
			count, duration.Round(time.Second), userID, contestID, err)
	} else {
		message = fmt.Sprintf("Routing error for user %s, contest %s: %v", userID, contestID, err)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    "Routing Error",
		Message:  message,
		Service:  "shard-router",
		Metadata: map[string]string{
			"user_id":          userID,
			"contest_id":       contestID,
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "routing_error",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendCacheInconsistency sends an alert when cache inconsistency is detected.
// HIGH severity as this can affect routing accuracy.
func (n *Notifier) sendCacheInconsistency(details string) {
	if n.svc == nil || n.aggregator == nil {
		return
	}

	key := "cache_inconsistency"
	info := map[string]string{
		"details": details,
	}

	shouldSend, count, firstOccurred := n.aggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	var message string
	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("Cache inconsistency detected (%d occurrences in %v). Details: %s",
			count, duration.Round(time.Second), details)
	} else {
		message = fmt.Sprintf("Cache inconsistency detected: %s", details)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityHigh,
		Title:    "Cache Inconsistency Detected",
		Message:  message,
		Service:  "shard-router",
		Metadata: map[string]string{
			"details":          details,
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "cache_inconsistency",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendNoHealthyShardsAvailable sends a CRITICAL alert when no healthy shards are available.
func (n *Notifier) sendNoHealthyShardsAvailable(totalShards int) {
	if n.svc == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityCritical,
		Title:    "CRITICAL: No Healthy Shards Available",
		Message:  fmt.Sprintf("No healthy shards available for routing! Total shards: %d. All traffic will fail!", totalShards),
		Service:  "shard-router",
		Metadata: map[string]string{
			"total_shards": fmt.Sprintf("%d", totalShards),
			"event":        "no_healthy_shards",
		},
	}

	// Send synchronously for critical alerts
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.svc.SendAlert(ctx, alert); err != nil {
		n.logger.Error("Failed to send no healthy shards alert", zap.Error(err))
	}
}

// sendShardAssignmentCorrupted sends a CRITICAL alert when shard assignment data is corrupted/missing.
func (n *Notifier) sendShardAssignmentCorrupted(details string) {
	if n.svc == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityCritical,
		Title:    "CRITICAL: Shard Assignment Data Corrupted",
		Message:  fmt.Sprintf("Shard assignment data is corrupted or missing! Details: %s. Manual intervention required!", details),
		Service:  "shard-router",
		Metadata: map[string]string{
			"details": details,
			"event":   "shard_assignment_corrupted",
		},
	}

	// Send synchronously for critical alerts
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := n.svc.SendAlert(ctx, alert); err != nil {
		n.logger.Error("Failed to send shard assignment corrupted alert", zap.Error(err))
	}
}

// sendCacheMissRateHigh sends a HIGH alert when cache miss rate exceeds threshold.
func (n *Notifier) sendCacheMissRateHigh(missRate float64, threshold float64) {
	if n.svc == nil || n.aggregator == nil {
		return
	}

	key := "cache_miss_rate_high"
	info := map[string]string{
		"miss_rate": fmt.Sprintf("%.2f", missRate),
		"threshold": fmt.Sprintf("%.2f", threshold),
	}

	shouldSend, count, _ := n.aggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityHigh,
		Title:    "High Cache Miss Rate",
		Message:  fmt.Sprintf("Cache miss rate (%.1f%%) exceeds threshold (%.1f%%). Routing performance may be degraded.", missRate*100, threshold*100),
		Service:  "shard-router",
		Metadata: map[string]string{
			"miss_rate":        fmt.Sprintf("%.4f", missRate),
			"threshold":        fmt.Sprintf("%.4f", threshold),
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "cache_miss_rate_high",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendShardLoadImbalance sends a MEDIUM alert when load imbalance is detected.
func (n *Notifier) sendShardLoadImbalance(heavyShardID string, lightShardID string, ratio float64) {
	if n.svc == nil || n.aggregator == nil {
		return
	}

	key := "load_imbalance"
	info := map[string]string{
		"heavy_shard": heavyShardID,
		"light_shard": lightShardID,
		"ratio":       fmt.Sprintf("%.2f", ratio),
	}

	shouldSend, count, _ := n.aggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityMedium,
		Title:    "Shard Load Imbalance Detected",
		Message:  fmt.Sprintf("Shard %s has %.1fx more load than shard %s. Consider rebalancing.", heavyShardID, ratio, lightShardID),
		Service:  "shard-router",
		Metadata: map[string]string{
			"heavy_shard":      heavyShardID,
			"light_shard":      lightShardID,
			"ratio":            fmt.Sprintf("%.2f", ratio),
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "load_imbalance",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendRoutingLatencyHigh sends a MEDIUM alert when P99 routing latency exceeds threshold.
func (n *Notifier) sendRoutingLatencyHigh(p99LatencyMs float64, thresholdMs float64) {
	if n.svc == nil || n.aggregator == nil {
		return
	}

	key := "routing_latency_high"
	info := map[string]string{
		"p99_latency_ms": fmt.Sprintf("%.2f", p99LatencyMs),
		"threshold_ms":   fmt.Sprintf("%.2f", thresholdMs),
	}

	shouldSend, count, _ := n.aggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityMedium,
		Title:    "High Routing Latency",
		Message:  fmt.Sprintf("P99 routing latency (%.2fms) exceeds threshold (%.2fms)", p99LatencyMs, thresholdMs),
		Service:  "shard-router",
		Metadata: map[string]string{
			"p99_latency_ms":   fmt.Sprintf("%.2f", p99LatencyMs),
			"threshold_ms":     fmt.Sprintf("%.2f", thresholdMs),
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "routing_latency_high",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendShardAdded sends a LOW alert when a new shard is added to the cluster.
func (n *Notifier) sendShardAdded(shardID, address string, totalShards int) {
	if n.svc == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityLow,
		Title:    fmt.Sprintf("New Shard Added: %s", shardID),
		Message:  fmt.Sprintf("New shard %s (%s) added to cluster. Total shards: %d", shardID, address, totalShards),
		Service:  "shard-router",
		Metadata: map[string]string{
			"shard_id":     shardID,
			"address":      address,
			"total_shards": fmt.Sprintf("%d", totalShards),
			"event":        "shard_added",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendShardRebalancingCompleted sends a LOW alert when shard rebalancing is completed.
func (n *Notifier) sendShardRebalancingCompleted(movedContests int, duration time.Duration) {
	if n.svc == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityLow,
		Title:    "Shard Rebalancing Completed",
		Message:  fmt.Sprintf("Shard rebalancing completed. Moved %d contests in %v", movedContests, duration.Round(time.Millisecond)),
		Service:  "shard-router",
		Metadata: map[string]string{
			"moved_contests":   fmt.Sprintf("%d", movedContests),
			"duration_seconds": fmt.Sprintf("%.2f", duration.Seconds()),
			"event":            "rebalancing_completed",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// sendDailyHealthReport sends a daily summary of shard health.
// Only sends if there were incidents during the period.
func (n *Notifier) sendDailyHealthReport(report ShardHealthReport) {
	if n.svc == nil {
		return
	}

	// Only send if there were incidents
	if report.IncidentCount == 0 && report.FailoverCount == 0 && report.RoutingErrors == 0 {
		return
	}

	var severity notification.Severity
	if report.UptimePercent < 99.0 || report.FailoverCount > 3 {
		severity = notification.SeverityMedium
	} else {
		severity = notification.SeverityInfo
	}

	message := fmt.Sprintf("Daily shard health report: %d/%d shards healthy, %.2f%% uptime, %d failovers, %d routing errors, %d incidents",
		report.HealthyShards, report.TotalShards, report.UptimePercent, report.FailoverCount, report.RoutingErrors, report.IncidentCount)

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    "Daily Shard Health Report",
		Message:  message,
		Service:  "shard-router",
		Metadata: map[string]string{
			"total_shards":     fmt.Sprintf("%d", report.TotalShards),
			"healthy_shards":   fmt.Sprintf("%d", report.HealthyShards),
			"unhealthy_shards": fmt.Sprintf("%d", report.UnhealthyShards),
			"draining_shards":  fmt.Sprintf("%d", report.DrainingShards),
			"uptime_percent":   fmt.Sprintf("%.2f", report.UptimePercent),
			"failover_count":   fmt.Sprintf("%d", report.FailoverCount),
			"routing_errors":   fmt.Sprintf("%d", report.RoutingErrors),
			"incident_count":   fmt.Sprintf("%d", report.IncidentCount),
			"period_hours":     fmt.Sprintf("%.0f", report.Period.Hours()),
			"event":            "daily_health_report",
		},
	}

	n.svc.SendAlertAsync(alert)
}

// Shutdown gracefully shuts down the notification service.
func (n *Notifier) Shutdown(ctx context.Context) error {
	if n.svc == nil {
		return nil
	}
	return n.svc.Shutdown(ctx)
}
