package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"go.uber.org/zap"
)

// ContestInfoAlert holds contest details for alert notifications.
type ContestInfoAlert struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Participants  int       `json:"participants"`
	PrizePool     float64   `json:"prize_pool"`      // Total prize pool in dollars
	EntryFee      float64   `json:"entry_fee"`       // Entry fee in dollars
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	TradingVolume float64   `json:"trading_volume"`  // Total trading volume
	Duration      string    `json:"duration"`        // Human-readable duration
}

// UserInfoAlert holds user details for alert notifications.
type UserInfoAlert struct {
	UserID   string  `json:"user_id"`
	Username string  `json:"username,omitempty"`
	Rank     int     `json:"rank"`
	Score    float64 `json:"score"`
	Prize    float64 `json:"prize"` // Prize amount in dollars
}

// ContestStatsAlert holds contest statistics for notifications.
type ContestStatsAlert struct {
	TotalTrades      int     `json:"total_trades"`
	TotalVolume      float64 `json:"total_volume"`
	AvgTradesPerUser float64 `json:"avg_trades_per_user"`
	TopSymbols       []string `json:"top_symbols"`
}

// FailedPayout represents a failed payout attempt.
type FailedPayout struct {
	UserID      string `json:"user_id"`
	Rank        int    `json:"rank"`
	AmountCents int64  `json:"amount_cents"`
	Error       string `json:"error"`
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

// ShouldSend checks if an alert should be sent and tracks the occurrence.
func (a *AlertAggregator) ShouldSend(key string, err error, info map[string]string) (shouldSend bool, count int, firstOccurred time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

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

// Reset clears aggregation for a specific key.
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
		ServiceName:     "leaderboard-worker",
		EmailRecipients: emailRecipients,
		Discord: notification.DiscordConfig{
			Enabled:    cfg.DiscordWebhookURL != "",
			WebhookURL: cfg.DiscordWebhookURL,
			Username:   "leaderboard-worker",
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

	metadata := map[string]string{
		"port":              a.config.Port,
		"shard_id":          fmt.Sprintf("%d", a.config.ShardID),
		"shard_count":       fmt.Sprintf("%d", a.config.ShardCount),
		"snapshot_interval": a.config.SnapshotInterval.String(),
		"consumer_group":    a.config.ConsumerGroup,
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityInfo,
		Title:    "Leaderboard Worker Started",
		Message:  fmt.Sprintf("Leaderboard worker started on port %s (shard %d/%d)", a.config.Port, a.config.ShardID, a.config.ShardCount),
		Service:  "leaderboard-worker",
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
		Title:    "Leaderboard Worker Shutting Down",
		Message:  "Leaderboard worker is performing graceful shutdown",
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"shard_id": fmt.Sprintf("%d", a.config.ShardID),
		},
	}

	// Send synchronously to ensure it's delivered before shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.notifications.SendAlert(ctx, alert); err != nil {
		a.log().Warn("Failed to send shutdown notification", zap.Error(err))
	}
}

// sendContestStarted sends a notification when a contest starts.
func (a *App) sendContestStarted(contest ContestInfoAlert) {
	if a.notifications == nil {
		return
	}

	message := fmt.Sprintf("Contest '%s' has started with %d participants and a $%.2f prize pool.",
		contest.Name, contest.Participants, contest.PrizePool)

	alert := notification.Alert{
		Type:     notification.AlertTypeContest,
		Severity: notification.SeverityMedium,
		Title:    fmt.Sprintf("Contest Started: %s", contest.Name),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":   contest.ID,
			"contest_name": contest.Name,
			"participants": fmt.Sprintf("%d", contest.Participants),
			"prize_pool":   fmt.Sprintf("%.2f", contest.PrizePool),
			"entry_fee":    fmt.Sprintf("%.2f", contest.EntryFee),
			"start_time":   contest.StartTime.Format(time.RFC3339),
			"end_time":     contest.EndTime.Format(time.RFC3339),
			"event":        "started",
		},
	}

	a.notifications.SendAlertAsync(alert)

	// Also send to webhook if configured
	a.sendContestWebhook("contest.started", contest, nil, nil)
}

// sendContestEnding sends a notification when a contest is about to end.
func (a *App) sendContestEnding(contest ContestInfoAlert, timeRemaining time.Duration) {
	if a.notifications == nil {
		return
	}

	message := fmt.Sprintf("Contest '%s' will end in %v. %d participants competing for $%.2f prize pool.",
		contest.Name, timeRemaining.Round(time.Minute), contest.Participants, contest.PrizePool)

	alert := notification.Alert{
		Type:     notification.AlertTypeContest,
		Severity: notification.SeverityInfo,
		Title:    fmt.Sprintf("Contest Ending Soon: %s", contest.Name),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":     contest.ID,
			"contest_name":   contest.Name,
			"participants":   fmt.Sprintf("%d", contest.Participants),
			"prize_pool":     fmt.Sprintf("%.2f", contest.PrizePool),
			"time_remaining": timeRemaining.String(),
			"end_time":       contest.EndTime.Format(time.RFC3339),
			"event":          "ending_soon",
		},
	}

	a.notifications.SendAlertAsync(alert)

	// Also send to webhook
	a.sendContestWebhook("contest.ending_soon", contest, nil, nil)
}

// sendContestFinalized sends a notification when a contest is successfully finalized.
func (a *App) sendContestFinalized(contest ContestInfoAlert, winner UserInfoAlert, stats ContestStatsAlert) {
	if a.notifications == nil {
		return
	}

	message := fmt.Sprintf("Contest '%s' has been finalized. Winner: User %s (Rank #%d) with score %.2f. "+
		"%d participants, $%.2f total prize pool distributed.",
		contest.Name, winner.UserID, winner.Rank, winner.Score,
		contest.Participants, contest.PrizePool)

	topSymbolsStr := ""
	if len(stats.TopSymbols) > 0 {
		topSymbolsStr = strings.Join(stats.TopSymbols, ", ")
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeContest,
		Severity: notification.SeverityMedium,
		Title:    fmt.Sprintf("Contest Finalized: %s", contest.Name),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":        contest.ID,
			"contest_name":      contest.Name,
			"participants":      fmt.Sprintf("%d", contest.Participants),
			"prize_pool":        fmt.Sprintf("%.2f", contest.PrizePool),
			"winner_user_id":    winner.UserID,
			"winner_rank":       fmt.Sprintf("%d", winner.Rank),
			"winner_score":      fmt.Sprintf("%.2f", winner.Score),
			"winner_prize":      fmt.Sprintf("%.2f", winner.Prize),
			"total_trades":      fmt.Sprintf("%d", stats.TotalTrades),
			"total_volume":      fmt.Sprintf("%.2f", stats.TotalVolume),
			"top_symbols":       topSymbolsStr,
			"duration":          contest.Duration,
			"event":             "finalized",
		},
	}

	a.notifications.SendAlertAsync(alert)

	// Also send to webhook with full details
	a.sendContestWebhook("contest.finalized", contest, &winner, &stats)
}

// sendPrizeDistributionStarted sends a notification when prize distribution begins.
func (a *App) sendPrizeDistributionStarted(contest ContestInfoAlert, totalPrize float64) {
	if a.notifications == nil {
		return
	}

	message := fmt.Sprintf("Starting prize distribution for contest '%s'. Total prize pool: $%.2f for %d participants.",
		contest.Name, totalPrize, contest.Participants)

	alert := notification.Alert{
		Type:     notification.AlertTypeContest,
		Severity: notification.SeverityMedium,
		Title:    fmt.Sprintf("Prize Distribution Started: %s", contest.Name),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":   contest.ID,
			"contest_name": contest.Name,
			"participants": fmt.Sprintf("%d", contest.Participants),
			"total_prize":  fmt.Sprintf("%.2f", totalPrize),
			"event":        "prize_distribution_started",
		},
	}

	a.notifications.SendAlertAsync(alert)

	// Also send to webhook
	a.sendContestWebhook("prize_distribution.started", contest, nil, nil)
}

// sendPrizeDistributionCompleted sends a notification when prize distribution is completed.
func (a *App) sendPrizeDistributionCompleted(contest ContestInfoAlert, recipients int, totalDistributed float64) {
	if a.notifications == nil {
		return
	}

	message := fmt.Sprintf("Prize distribution completed for contest '%s'. Distributed $%.2f to %d winners.",
		contest.Name, totalDistributed, recipients)

	alert := notification.Alert{
		Type:     notification.AlertTypeContest,
		Severity: notification.SeverityMedium,
		Title:    fmt.Sprintf("Prize Distribution Completed: %s", contest.Name),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":        contest.ID,
			"contest_name":      contest.Name,
			"recipients":        fmt.Sprintf("%d", recipients),
			"total_distributed": fmt.Sprintf("%.2f", totalDistributed),
			"event":             "prize_distribution_completed",
		},
	}

	a.notifications.SendAlertAsync(alert)

	// Also send to webhook
	a.sendContestWebhook("prize_distribution.completed", contest, nil, nil)
}

// sendPrizeDistributionFailed sends a CRITICAL alert when prize distribution fails.
// This triggers BOTH Discord AND Email for maximum visibility (money involved!).
func (a *App) sendPrizeDistributionFailed(contest ContestInfoAlert, err error, failedPayouts []FailedPayout) {
	if a.notifications == nil {
		return
	}

	failedCount := len(failedPayouts)
	var totalFailedAmount int64
	for _, p := range failedPayouts {
		totalFailedAmount += p.AmountCents
	}

	message := fmt.Sprintf("CRITICAL: Prize distribution FAILED for contest '%s'. "+
		"%d payouts failed totaling $%.2f. Error: %v",
		contest.Name, failedCount, float64(totalFailedAmount)/100, err)

	// Build metadata with failed payout details
	metadata := map[string]string{
		"contest_id":          contest.ID,
		"contest_name":        contest.Name,
		"error":               err.Error(),
		"failed_payout_count": fmt.Sprintf("%d", failedCount),
		"failed_amount_cents": fmt.Sprintf("%d", totalFailedAmount),
		"event":               "prize_distribution_failed",
	}

	// Add first few failed payouts to metadata for visibility
	for i, fp := range failedPayouts {
		if i >= 5 {
			break // Limit to first 5 failures in metadata
		}
		metadata[fmt.Sprintf("failed_payout_%d_user", i)] = fp.UserID
		metadata[fmt.Sprintf("failed_payout_%d_amount", i)] = fmt.Sprintf("%d", fp.AmountCents)
		metadata[fmt.Sprintf("failed_payout_%d_error", i)] = fp.Error
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeContest,
		Severity: notification.SeverityCritical, // Critical severity sends to BOTH Discord AND Email
		Title:    fmt.Sprintf("CRITICAL: Prize Distribution Failed: %s", contest.Name),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: metadata,
	}

	// Send synchronously for critical alerts to ensure delivery
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if sendErr := a.notifications.SendAlert(ctx, alert); sendErr != nil {
		a.log().Error("Failed to send prize distribution failure alert",
			zap.String("contest_id", contest.ID),
			zap.Error(sendErr))
	}

	// Also send to webhook
	a.sendContestWebhook("prize_distribution.failed", contest, nil, nil)
}

// sendContestFinalizationFailed sends a CRITICAL alert when contest finalization fails after retries.
func (a *App) sendContestFinalizationFailed(contestID string, err error, attemptCount int) {
	if a.notifications == nil {
		return
	}

	message := fmt.Sprintf("CRITICAL: Contest finalization FAILED for contest %s after %d attempts. Error: %v. "+
		"Manual intervention required!",
		contestID, attemptCount, err)

	alert := notification.Alert{
		Type:     notification.AlertTypeContest,
		Severity: notification.SeverityCritical,
		Title:    fmt.Sprintf("CRITICAL: Contest Finalization Failed: %s", contestID),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":    contestID,
			"error":         err.Error(),
			"attempt_count": fmt.Sprintf("%d", attemptCount),
			"event":         "finalization_failed",
		},
	}

	// Send synchronously for critical alerts
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if sendErr := a.notifications.SendAlert(ctx, alert); sendErr != nil {
		a.log().Error("Failed to send contest finalization failure alert",
			zap.String("contest_id", contestID),
			zap.Error(sendErr))
	}
}

// sendDatabaseTransactionFailed sends a CRITICAL alert when a database transaction fails during payout.
func (a *App) sendDatabaseTransactionFailed(contestID string, operation string, err error) {
	if a.notifications == nil {
		return
	}

	message := fmt.Sprintf("CRITICAL: Database transaction FAILED during %s for contest %s. Error: %v. "+
		"Potential data inconsistency - verify wallet balances!",
		operation, contestID, err)

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityCritical,
		Title:    "CRITICAL: Database Transaction Rollback",
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id": contestID,
			"operation":  operation,
			"error":      err.Error(),
			"event":      "database_transaction_failed",
		},
	}

	// Send synchronously for critical alerts
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if sendErr := a.notifications.SendAlert(ctx, alert); sendErr != nil {
		a.log().Error("Failed to send database transaction failure alert",
			zap.String("contest_id", contestID),
			zap.Error(sendErr))
	}
}

// sendLeaderboardCalculationError sends a HIGH severity alert when leaderboard calculation fails.
func (a *App) sendLeaderboardCalculationError(contestID string, err error) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("leaderboard_calc_error:%s", contestID)
	info := map[string]string{
		"contest_id": contestID,
		"error":      err.Error(),
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var message string
	var severity notification.Severity

	if count > 5 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("Persistent leaderboard calculation errors for contest %s (%d occurrences in %v): %v",
			contestID, count, duration.Round(time.Second), err)
		severity = notification.SeverityHigh
	} else {
		message = fmt.Sprintf("Leaderboard calculation error for contest %s: %v", contestID, err)
		severity = notification.SeverityMedium
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("Leaderboard Calculation Error: %s", contestID),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":       contestID,
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "leaderboard_calculation_error",
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendLeaderboardCalculationTakingTooLong sends a HIGH severity alert when calculation exceeds 5 minutes.
func (a *App) sendLeaderboardCalculationTakingTooLong(contestID string, elapsed time.Duration) {
	if a.notifications == nil {
		return
	}

	message := fmt.Sprintf("Leaderboard calculation for contest %s is taking too long: %v elapsed. "+
		"This may indicate performance issues or data inconsistency.",
		contestID, elapsed.Round(time.Second))

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityHigh,
		Title:    fmt.Sprintf("Slow Leaderboard Calculation: %s", contestID),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":      contestID,
			"elapsed_seconds": fmt.Sprintf("%.0f", elapsed.Seconds()),
			"event":           "calculation_slow",
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendRankingAnomalyDetected sends a HIGH severity alert when ranking anomaly is detected.
func (a *App) sendRankingAnomalyDetected(contestID string, anomaly string) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("ranking_anomaly:%s", contestID)
	info := map[string]string{
		"contest_id": contestID,
		"anomaly":    anomaly,
	}

	shouldSend, count, firstOccurred := a.alertAggregator.ShouldSend(key, nil, info)
	if !shouldSend {
		return
	}

	var message string
	if count > 1 {
		duration := time.Since(firstOccurred)
		message = fmt.Sprintf("Ranking anomaly detected in contest %s (%d occurrences in %v): %s",
			contestID, count, duration.Round(time.Second), anomaly)
	} else {
		message = fmt.Sprintf("Ranking anomaly detected in contest %s: %s", contestID, anomaly)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: notification.SeverityHigh,
		Title:    fmt.Sprintf("Ranking Anomaly Detected: %s", contestID),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":       contestID,
			"anomaly":          anomaly,
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "ranking_anomaly",
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// sendWalletCreditFailed sends a HIGH severity alert when wallet credit fails.
func (a *App) sendWalletCreditFailed(contestID, userID string, amountCents int64, err error) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("wallet_credit_failed:%s", contestID)
	info := map[string]string{
		"contest_id": contestID,
		"user_id":    userID,
		"amount":     fmt.Sprintf("%d", amountCents),
		"error":      err.Error(),
	}

	shouldSend, count, _ := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	if count >= 3 {
		severity = notification.SeverityCritical // Multiple wallet failures is critical
	} else {
		severity = notification.SeverityHigh
	}

	message := fmt.Sprintf("Failed to credit wallet for user %s in contest %s. Amount: $%.2f. Error: %v",
		userID, contestID, float64(amountCents)/100, err)
	if count > 1 {
		message = fmt.Sprintf("%d wallet credit failures for contest %s. Latest: user %s, amount $%.2f. Error: %v",
			count, contestID, userID, float64(amountCents)/100, err)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeTrade,
		Severity: severity,
		Title:    "Wallet Credit Failed",
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":       contestID,
			"user_id":          userID,
			"amount_cents":     fmt.Sprintf("%d", amountCents),
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "wallet_credit_failed",
		},
	}

	// For critical severity, send synchronously
	if severity == notification.SeverityCritical {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if sendErr := a.notifications.SendAlert(ctx, alert); sendErr != nil {
			a.log().Error("Failed to send wallet credit failure alert",
				zap.String("contest_id", contestID),
				zap.Error(sendErr))
		}
	} else {
		a.notifications.SendAlertAsync(alert)
	}
}

// sendSnapshotWriteError sends an alert when snapshot write fails.
func (a *App) sendSnapshotWriteError(contestID string, err error) {
	if a.notifications == nil || a.alertAggregator == nil {
		return
	}

	key := fmt.Sprintf("snapshot_write_error:%s", contestID)
	info := map[string]string{
		"contest_id": contestID,
		"error":      err.Error(),
	}

	shouldSend, count, _ := a.alertAggregator.ShouldSend(key, err, info)
	if !shouldSend {
		return
	}

	var severity notification.Severity
	if count >= 5 {
		severity = notification.SeverityHigh
	} else {
		severity = notification.SeverityMedium
	}

	message := fmt.Sprintf("Failed to write leaderboard snapshot for contest %s: %v", contestID, err)
	if count > 1 {
		message = fmt.Sprintf("Persistent snapshot write failures for contest %s (%d failures): %v", contestID, count, err)
	}

	alert := notification.Alert{
		Type:     notification.AlertTypeSystem,
		Severity: severity,
		Title:    fmt.Sprintf("Snapshot Write Error: %s", contestID),
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"contest_id":       contestID,
			"error":            err.Error(),
			"occurrence_count": fmt.Sprintf("%d", count),
			"event":            "snapshot_write_error",
		},
	}

	a.notifications.SendAlertAsync(alert)
}

// ContestWebhookEvent represents a webhook payload for contest events.
type ContestWebhookEvent struct {
	Event     string            `json:"event"`
	Timestamp time.Time         `json:"timestamp"`
	Contest   ContestInfoAlert  `json:"contest"`
	Winner    *UserInfoAlert    `json:"winner,omitempty"`
	Stats     *ContestStatsAlert `json:"stats,omitempty"`
}

// sendContestWebhook sends contest events to configured webhook URL.
func (a *App) sendContestWebhook(event string, contest ContestInfoAlert, winner *UserInfoAlert, stats *ContestStatsAlert) {
	if a.config.ContestWebhookURL == "" {
		return
	}

	payload := ContestWebhookEvent{
		Event:     event,
		Timestamp: time.Now().UTC(),
		Contest:   contest,
		Winner:    winner,
		Stats:     stats,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		a.log().Error("Failed to marshal webhook payload",
			zap.String("event", event),
			zap.Error(err))
		return
	}

	// Create request
	ctx, cancel := context.WithTimeout(context.Background(), a.config.ContestWebhookTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.ContestWebhookURL, bytes.NewReader(payloadBytes))
	if err != nil {
		a.log().Error("Failed to create webhook request",
			zap.String("event", event),
			zap.Error(err))
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tragge-Event", event)
	req.Header.Set("X-Tragge-Timestamp", payload.Timestamp.Format(time.RFC3339))

	// Sign payload if secret is configured
	if a.config.ContestWebhookSecret != "" {
		signature := signWebhookPayload(payloadBytes, a.config.ContestWebhookSecret)
		req.Header.Set("X-Tragge-Signature", signature)
	}

	// Send webhook asynchronously
	infra.SafeGo(a.log(), "contest-webhook", func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			a.log().Warn("Failed to send webhook",
				zap.String("event", event),
				zap.String("contest_id", contest.ID),
				zap.Error(err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			a.log().Warn("Webhook returned error status",
				zap.String("event", event),
				zap.String("contest_id", contest.ID),
				zap.Int("status", resp.StatusCode))
		} else {
			a.log().Debug("Webhook sent successfully",
				zap.String("event", event),
				zap.String("contest_id", contest.ID),
				zap.Int("status", resp.StatusCode))
		}
	})
}

// signWebhookPayload creates an HMAC-SHA256 signature for webhook payload.
func signWebhookPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// sendDailyLeaderboardSummary sends a daily summary notification for active contests.
func (a *App) sendDailyLeaderboardSummary(contests []ContestInfoAlert) {
	if a.notifications == nil || len(contests) == 0 {
		return
	}

	var contestSummary strings.Builder
	totalParticipants := 0
	totalPrizePool := 0.0

	for _, c := range contests {
		contestSummary.WriteString(fmt.Sprintf("- %s: %d participants, $%.2f pool\n",
			c.Name, c.Participants, c.PrizePool))
		totalParticipants += c.Participants
		totalPrizePool += c.PrizePool
	}

	message := fmt.Sprintf("Daily leaderboard summary: %d active contests, %d total participants, $%.2f total prize pool.\n\n%s",
		len(contests), totalParticipants, totalPrizePool, contestSummary.String())

	alert := notification.Alert{
		Type:     notification.AlertTypeContest,
		Severity: notification.SeverityInfo,
		Title:    "Daily Leaderboard Summary",
		Message:  message,
		Service:  "leaderboard-worker",
		Metadata: map[string]string{
			"active_contests":    fmt.Sprintf("%d", len(contests)),
			"total_participants": fmt.Sprintf("%d", totalParticipants),
			"total_prize_pool":   fmt.Sprintf("%.2f", totalPrizePool),
			"event":              "daily_summary",
		},
	}

	a.notifications.SendAlertAsync(alert)
}
