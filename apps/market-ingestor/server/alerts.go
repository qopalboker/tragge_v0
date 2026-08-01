package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/notification"
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
		ServiceName:     "market-ingestor",
		EmailRecipients: emailRecipients,
		Discord: notification.DiscordConfig{
			Enabled:    cfg.DiscordWebhookURL != "",
			WebhookURL: cfg.DiscordWebhookURL,
			Username:   "market-ingestor",
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

	// Determine API key status (valid/invalid without exposing keys)
	twelveDataStatus := "not configured"
	if len(a.config.TwelveDataAPIKeys) > 0 {
		twelveDataStatus = fmt.Sprintf("%d key(s) configured", len(a.config.TwelveDataAPIKeys))
	}

	massiveStatus := "not configured"
	if len(a.config.MassiveAPIKeys) > 0 {
		massiveStatus = fmt.Sprintf("%d key(s) configured", len(a.config.MassiveAPIKeys))
	}

	metadata := map[string]string{
		"port":              a.config.Port,
		"provider_mode":     string(a.config.MarketProvider),
		"symbols":           strings.Join(a.config.Symbols, ", "),
		"twelvedata_status": twelveDataStatus,
		"massive_status":    massiveStatus,
		"failover_timeout":  a.config.FailoverTimeout.String(),
		"auto_switchback":   fmt.Sprintf("%v", a.config.AutoSwitchback),
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Market Ingestor Started",
		Message:  fmt.Sprintf("Market ingestor started on port %s with %d symbols", a.config.Port, len(a.config.Symbols)),
		Service:  "market-ingestor",
		Metadata: metadata,
	}

	a.notifications.SendAlertAsync(alert)
}

// sendShutdownNotification sends a notification when the service is shutting down.
func (a *App) sendShutdownNotification() {
	if a.notifications == nil {
		return
	}

	currentProvider := ""
	if a.providerManager != nil {
		currentProvider = string(a.providerManager.CurrentProvider())
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Market Ingestor Shutting Down",
		Message:  "Market ingestor is performing graceful shutdown",
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"current_provider": currentProvider,
		},
	}

	// Send synchronously to ensure it's delivered before shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.notifications.SendAlert(ctx, alert); err != nil {
		a.log().Warn("Failed to send shutdown notification", zap.Error(err))
	}
}

// sendDataFeedDisconnected sends an alert when a data feed is disconnected.
func (a *App) sendDataFeedDisconnected(provider string, err error) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("feed_disconnected:%s", provider)
	info := map[string]string{
		"provider": provider,
		"error":    err.Error(),
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var message string
	var severity notification.Severity

	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("Data feed disconnection for %s (%d occurrences in last %v): %v", provider, count, duration.Round(time.Second), err)
		severity = notification.SeverityHigh
	} else {
		message = fmt.Sprintf("Data feed disconnected from %s: %v", provider, err)
		severity = notification.SeverityMedium
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("Data Feed Disconnected: %s", provider),
		Message:  message,
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"provider":         provider,
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendDataFeedReconnected sends an alert when a data feed is reconnected.
func (a *App) sendDataFeedReconnected(provider string, downtime time.Duration) {
	if a.notifications == nil {
		return
	}

	// Clear the disconnection aggregation
	if a.alertAggregator != nil {
		a.alertAggregator.Reset(fmt.Sprintf("feed_disconnected:%s", provider))
	}

	// Only send notification for reconnections if downtime was significant (> 30s)
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
		Title:    fmt.Sprintf("Data Feed Reconnected: %s", provider),
		Message:  fmt.Sprintf("Data feed reconnected to %s after %v downtime", provider, downtime.Round(time.Second)),
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"provider":         provider,
			"downtime_seconds": fmt.Sprintf("%.0f", downtime.Seconds()),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendProviderFailover sends an alert when a provider failover occurs.
func (a *App) sendProviderFailover(from, to, reason string) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityMedium,
		Title:    "Provider Failover Occurred",
		Message:  fmt.Sprintf("Switched from %s to %s: %s", from, to, reason),
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"from_provider": from,
			"to_provider":   to,
			"reason":        reason,
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendProviderSwitchback sends an alert when switching back to primary provider.
func (a *App) sendProviderSwitchback(to string) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityLow,
		Title:    "Provider Switchback",
		Message:  fmt.Sprintf("Switched back to primary provider: %s", to),
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"provider": to,
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendAllProvidersDown sends a critical alert when all data providers are down.
func (a *App) sendAllProvidersDown(err error) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := "all_providers_down"
	info := map[string]string{
		"error": err.Error(),
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var message string
	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("All data providers are down (%d occurrences in last %v). No market data available!", count, duration.Round(time.Second))
	} else {
		message = "All data providers are down. No market data available! Trading may be impacted."
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityCritical,
		Title:    "CRITICAL: All Data Providers Down",
		Message:  message,
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendPriceAnomalyDetected sends an alert when a price anomaly is detected.
func (a *App) sendPriceAnomalyDetected(symbol string, expectedPrice, actualPrice, deviationPercent float64) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("price_anomaly:%s", symbol)
	info := map[string]string{
		"symbol":            symbol,
		"expected_price":    fmt.Sprintf("%.4f", expectedPrice),
		"actual_price":      fmt.Sprintf("%.4f", actualPrice),
		"deviation_percent": fmt.Sprintf("%.2f%%", deviationPercent),
	}

	shouldSend, count, _ := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	if deviationPercent > 20 {
		severity = notification.SeverityHigh
	} else {
		severity = notification.SeverityMedium
	}

	var message string
	if count > 1 {
		message = fmt.Sprintf("Price anomaly detected for %s (%d occurrences): %.2f%% deviation (expected ~%.4f, got %.4f)",
			symbol, count, deviationPercent, expectedPrice, actualPrice)
	} else {
		message = fmt.Sprintf("Price anomaly detected for %s: %.2f%% deviation from expected range (expected ~%.4f, got %.4f)",
			symbol, deviationPercent, expectedPrice, actualPrice)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("Price Anomaly: %s", symbol),
		Message:  message,
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"symbol":            symbol,
			"expected_price":    fmt.Sprintf("%.4f", expectedPrice),
			"actual_price":      fmt.Sprintf("%.4f", actualPrice),
			"deviation_percent": fmt.Sprintf("%.2f", deviationPercent),
			"occurrence_count":  fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendRateLimitHit sends an alert when a rate limit is hit.
func (a *App) sendRateLimitHit(provider string, retryAfter time.Duration) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("rate_limit:%s", provider)
	info := map[string]string{
		"provider":    provider,
		"retry_after": retryAfter.String(),
	}

	shouldSend, count, _ := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	if count > 5 {
		severity = notification.SeverityHigh
	} else {
		severity = notification.SeverityMedium
	}

	var message string
	if count > 1 {
		message = fmt.Sprintf("Rate limit hit for %s (%d times). Retry after: %v", provider, count, retryAfter)
	} else {
		message = fmt.Sprintf("Rate limit hit for %s. Retry after: %v", provider, retryAfter)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("Rate Limit Hit: %s", provider),
		Message:  message,
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"provider":         provider,
			"retry_after":      retryAfter.String(),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendAPIKeyExhausted sends an alert when all API keys for a provider are exhausted.
func (a *App) sendAPIKeyExhausted(provider string) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityHigh,
		Title:    fmt.Sprintf("API Keys Exhausted: %s", provider),
		Message:  fmt.Sprintf("All API keys for %s have been exhausted or are invalid. No backup keys available.", provider),
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"provider": provider,
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendPriceDataGap sends an alert when there's a gap in price data for active symbols.
func (a *App) sendPriceDataGap(symbol string, gap time.Duration) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("price_gap:%s", symbol)
	info := map[string]string{
		"symbol": symbol,
		"gap":    gap.String(),
	}

	shouldSend, count, _ := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	if gap > 5*time.Minute {
		severity = notification.SeverityCritical
	} else if gap > 1*time.Minute {
		severity = notification.SeverityHigh
	} else {
		severity = notification.SeverityMedium
	}

	var message string
	if count > 1 {
		message = fmt.Sprintf("Price data gap for %s: no data for %v (%d occurrences)", symbol, gap.Round(time.Second), count)
	} else {
		message = fmt.Sprintf("Price data gap detected for %s: no data received for %v", symbol, gap.Round(time.Second))
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("Price Data Gap: %s", symbol),
		Message:  message,
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"symbol":           symbol,
			"gap_seconds":      fmt.Sprintf("%.0f", gap.Seconds()),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendWebSocketError sends an alert when a WebSocket error occurs.
func (a *App) sendWebSocketError(provider string, err error) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("ws_error:%s", provider)
	info := map[string]string{
		"provider": provider,
		"error":    err.Error(),
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var message string
	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("WebSocket error for %s (%d occurrences in last %v): %v", provider, count, duration.Round(time.Second), err)
	} else {
		message = fmt.Sprintf("WebSocket error for %s: %v", provider, err)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityMedium,
		Title:    fmt.Sprintf("WebSocket Error: %s", provider),
		Message:  message,
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"provider":         provider,
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendKafkaPublishError sends an alert when Kafka publishing fails.
func (a *App) sendKafkaPublishError(symbol string, err error) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := "kafka_publish_error"
	info := map[string]string{
		"symbol": symbol,
		"error":  err.Error(),
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	if count > 10 {
		severity = notification.SeverityCritical
	} else if count > 3 {
		severity = notification.SeverityHigh
	} else {
		severity = notification.SeverityMedium
	}

	var message string
	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("Kafka publish failures (%d occurrences in last %v). Latest for symbol %s: %v", count, duration.Round(time.Second), symbol, err)
	} else {
		message = fmt.Sprintf("Kafka publish failed for symbol %s: %v", symbol, err)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    "Kafka Publish Error",
		Message:  message,
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"symbol":           symbol,
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendRedisPublishError sends an alert when Redis publishing fails.
func (a *App) sendRedisPublishError(symbol string, err error) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := "redis_publish_error"
	info := map[string]string{
		"symbol": symbol,
		"error":  err.Error(),
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var message string
	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("Redis publish failures (%d occurrences in last %v). Latest for symbol %s: %v", count, duration.Round(time.Second), symbol, err)
	} else {
		message = fmt.Sprintf("Redis publish failed for symbol %s: %v", symbol, err)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityMedium,
		Title:    "Redis Publish Error",
		Message:  message,
		Service:  "market-ingestor",
		Metadata: map[string]string{
			"symbol":           symbol,
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// ProviderAlertHandler interface implementation for App

// OnDisconnected is called when a provider disconnects.
func (a *App) OnDisconnected(provider string, err error) {
	a.sendDataFeedDisconnected(provider, err)
}

// OnReconnected is called when a provider reconnects.
func (a *App) OnReconnected(provider string, downtime time.Duration) {
	a.sendDataFeedReconnected(provider, downtime)
}

// OnFailover is called when a failover occurs.
func (a *App) OnFailover(from, to, reason string) {
	a.sendProviderFailover(from, to, reason)
}

// OnSwitchback is called when switching back to primary provider.
func (a *App) OnSwitchback(to string) {
	a.sendProviderSwitchback(to)
}

// OnAllProvidersDown is called when all providers are down.
func (a *App) OnAllProvidersDown(err error) {
	a.sendAllProvidersDown(err)
}
