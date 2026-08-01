package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"go.uber.org/zap"
)

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
		ServiceName:     "trading-engine",
		EmailRecipients: emailRecipients,
		Discord: notification.DiscordConfig{
			Enabled:    cfg.DiscordWebhookURL != "",
			WebhookURL: cfg.DiscordWebhookURL,
			Username:   "trading-engine",
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
		"port":     a.config.Port,
		"shard_id": fmt.Sprintf("%d", a.config.ShardID),
		"pod_name": a.config.PodName,
	}

	if a.config.ShardEnabled {
		metadata["shard_enabled"] = "true"
		metadata["shard_count"] = fmt.Sprintf("%d", a.config.ShardCount)
	}

	if a.config.PartitionAwareEnabled {
		metadata["partition_aware"] = "true"
		metadata["total_partitions"] = fmt.Sprintf("%d", a.config.TotalPartitions)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Trading Engine Started",
		Message:  fmt.Sprintf("Trading engine started on port %s", a.config.Port),
		Service:  "trading-engine",
		Metadata: metadata,
	}

	a.notifications.SendAlertAsync(alert)
}

// sendShutdownNotification sends a notification when the service is shutting down.
func (a *App) sendShutdownNotification() {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Trading Engine Shutting Down",
		Message:  "Trading engine is performing graceful shutdown",
		Service:  "trading-engine",
		Metadata: map[string]string{
			"shard_id": fmt.Sprintf("%d", a.config.ShardID),
			"pod_name": a.config.PodName,
		},
	}

	// Send synchronously to ensure it's delivered before shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.notifications.SendAlert(ctx, alert); err != nil {
		a.log().Warn("Failed to send shutdown notification", zap.Error(err))
	}
}

// sendOrderProcessingError sends an alert when order processing fails.
func (a *App) sendOrderProcessingError(err error, order *contracts.OrderRequest) {
	if a.notifications == nil {
		return
	}

	metadata := map[string]string{
		"error": err.Error(),
	}

	if order != nil {
		metadata["order_id"] = order.OrderID
		metadata["user_id"] = order.UserID
		metadata["contest_id"] = order.ContestID
		metadata["symbol"] = order.Symbol
		metadata["side"] = string(order.Side)
		metadata["type"] = string(order.Type)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeTrade,
		Severity: notification.SeverityHigh,
		Title:    "Order Processing Failed",
		Message:  fmt.Sprintf("Failed to process order: %v", err),
		Service:  "trading-engine",
		Metadata: metadata,
	}

	a.notifications.SendAlertAsync(alert)
}

// sendKafkaConsumerError sends an alert when Kafka consumer encounters an error.
func (a *App) sendKafkaConsumerError(err error, topic string) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityHigh,
		Title:    "Kafka Consumer Error",
		Message:  fmt.Sprintf("Kafka consumer error on topic %s: %v", topic, err),
		Service:  "trading-engine",
		Metadata: map[string]string{
			"topic":          topic,
			"error":          err.Error(),
			"consumer_group": a.config.ConsumerGroup,
			"shard_id":       fmt.Sprintf("%d", a.config.ShardID),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendDatabaseError sends an alert when a critical database error occurs.
func (a *App) sendDatabaseError(err error, operation string) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityCritical,
		Title:    "Database Error",
		Message:  fmt.Sprintf("Database error during %s: %v", operation, err),
		Service:  "trading-engine",
		Metadata: map[string]string{
			"operation": operation,
			"error":     err.Error(),
			"shard_id":  fmt.Sprintf("%d", a.config.ShardID),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendShardStateError sends an alert when shard state management fails.
func (a *App) sendShardStateError(err error, shardID int) {
	if a.notifications == nil {
		return
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityHigh,
		Title:    "Shard State Error",
		Message:  fmt.Sprintf("Shard %d state error: %v", shardID, err),
		Service:  "trading-engine",
		Metadata: map[string]string{
			"shard_id":    fmt.Sprintf("%d", shardID),
			"shard_count": fmt.Sprintf("%d", a.config.ShardCount),
			"error":       err.Error(),
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendCircuitBreakerStateChange sends an alert when a circuit breaker changes state.
func (a *App) sendCircuitBreakerStateChange(name string, newState string, isOpen bool) {
	if a.notifications == nil {
		return
	}

	var severity notification.Severity
	var title string
	var message string

	if isOpen {
		severity = notification.SeverityMedium
		title = fmt.Sprintf("Circuit Breaker Opened: %s", name)
		message = fmt.Sprintf("Circuit breaker '%s' has opened due to failures", name)
	} else {
		severity = notification.SeverityInfo
		title = fmt.Sprintf("Circuit Breaker Closed: %s", name)
		message = fmt.Sprintf("Circuit breaker '%s' has recovered and closed", name)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    title,
		Message:  message,
		Service:  "trading-engine",
		Metadata: map[string]string{
			"circuit_breaker": name,
			"state":           newState,
			"shard_id":        fmt.Sprintf("%d", a.config.ShardID),
		},
	}

	a.notifications.SendAlertAsync(alert)
}
