package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// NotificationEvent represents a notification event from the notifications.v1 Kafka topic.
type NotificationEvent struct {
	Type      string                 `json:"type"`
	ContestID string                 `json:"contest_id"`
	Data      map[string]interface{} `json:"data"`
	Timestamp int64                  `json:"timestamp"`
}

// notificationMapping defines how a Kafka event type maps to an in-app notification.
type notificationMapping struct {
	notifType string
	title     string
	messageFn func(data map[string]interface{}) string
}

var eventMappings = map[string]notificationMapping{
	"contest_started": {
		notifType: "contest_started",
		title:     "Contest Started!",
		messageFn: func(data map[string]interface{}) string {
			return fmt.Sprintf("%s has started. Start trading now!", contestName(data))
		},
	},
	"contest_completed": {
		notifType: "contest_completed",
		title:     "Contest Ended",
		messageFn: func(data map[string]interface{}) string {
			return fmt.Sprintf("%s has ended. Check your results!", contestName(data))
		},
	},
	"contest_cancelled": {
		notifType: "contest_cancelled",
		title:     "Contest Cancelled",
		messageFn: func(data map[string]interface{}) string {
			reason := "Unknown"
			if r, ok := data["reason"].(string); ok && r != "" {
				reason = r
			}
			return fmt.Sprintf("%s has been cancelled. Reason: %s", contestName(data), reason)
		},
	},
	"registration_closed": {
		notifType: "registration_closed",
		title:     "Registration Closed",
		messageFn: func(data map[string]interface{}) string {
			return fmt.Sprintf("Registration for %s is now closed. Contest starts soon!", contestName(data))
		},
	},
	"contest_paused": {
		notifType: "contest_paused",
		title:     "Contest Paused",
		messageFn: func(data map[string]interface{}) string {
			return fmt.Sprintf("%s has been paused by admin.", contestName(data))
		},
	},
	"contest_resumed": {
		notifType: "contest_resumed",
		title:     "Contest Resumed",
		messageFn: func(data map[string]interface{}) string {
			return fmt.Sprintf("%s has resumed. Trading is active again!", contestName(data))
		},
	},
}

// contestName extracts the contest_name from event data, falling back to "The contest".
func contestName(data map[string]interface{}) string {
	if name, ok := data["contest_name"].(string); ok && name != "" {
		return name
	}
	return "The contest"
}

// notificationConsumerMetrics holds Prometheus metrics for the notification consumer.
type notificationConsumerMetrics struct {
	notificationsCreated *prometheus.CounterVec
	consumerErrors       *prometheus.CounterVec
}

// newNotificationConsumerMetrics creates and registers Prometheus metrics for the notification consumer.
func newNotificationConsumerMetrics(obs metricsRegisterer) *notificationConsumerMetrics {
	created := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "leaderboard_worker",
			Name:      "notifications_created_total",
			Help:      "Total number of in-app notifications created from Kafka events",
		},
		[]string{"event_type"},
	)

	errs := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "leaderboard_worker",
			Name:      "notification_consumer_errors_total",
			Help:      "Total number of errors in the notification consumer",
		},
		[]string{"error_type"},
	)

	obs.MustRegister(created, errs)

	return &notificationConsumerMetrics{
		notificationsCreated: created,
		consumerErrors:       errs,
	}
}

// metricsRegisterer is satisfied by the observability Metrics type.
type metricsRegisterer interface {
	MustRegister(collectors ...prometheus.Collector)
}

// startNotificationConsumer starts a Kafka consumer that reads from the "notifications.v1"
// topic and creates in-app notification rows for each contest participant.
func (a *App) startNotificationConsumer() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("startNotificationConsumer panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	log := a.log()

	// Register Prometheus metrics
	metrics := newNotificationConsumerMetrics(a.obs.Metrics)

	topic := a.config.NotificationsTopic

	// Create a separate Kafka consumer for notification events
	notifOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-notifications"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
	}
	notifOpts = append(notifOpts, infra.KafkaSecurityOpts()...)
	consumer, err := kgo.NewClient(notifOpts...)
	if err != nil {
		log.Error("Failed to create notification consumer", zap.Error(err))
		metrics.consumerErrors.WithLabelValues("consumer_init").Inc()
		return
	}
	defer consumer.Close()

	log.Info("Starting notification consumer", zap.String("topic", topic))

	for {
		select {
		case <-a.ctx.Done():
			log.Info("Notification consumer shutting down")
			return
		default:
		}

		fetches := consumer.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Error("Notification consumer fetch error", zap.Error(err))
			metrics.consumerErrors.WithLabelValues("fetch").Inc()
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processNotificationRecord(record, log, metrics)
			}

			if err := consumer.CommitUncommittedOffsets(a.ctx); err != nil {
				log.Error("Notification consumer commit error", zap.Error(err))
				metrics.consumerErrors.WithLabelValues("commit").Inc()
			}
		})
	}
}

// processNotificationRecord processes a single notification event from Kafka.
func (a *App) processNotificationRecord(record *kgo.Record, log *zap.Logger, metrics *notificationConsumerMetrics) {
	var event NotificationEvent
	if err := json.Unmarshal(record.Value, &event); err != nil {
		log.Error("Failed to unmarshal notification event",
			zap.Error(err),
			zap.ByteString("value", record.Value))
		metrics.consumerErrors.WithLabelValues("unmarshal").Inc()
		return
	}

	mapping, ok := eventMappings[event.Type]
	if !ok {
		log.Warn("Unknown notification event type",
			zap.String("type", event.Type),
			zap.String("contest_id", event.ContestID))
		metrics.consumerErrors.WithLabelValues("unknown_event_type").Inc()
		return
	}

	log.Info("Processing notification event",
		zap.String("type", event.Type),
		zap.String("contest_id", event.ContestID))

	// Query contest participants
	userIDs, err := a.getContestParticipantUserIDs(a.ctx, event.ContestID)
	if err != nil {
		log.Error("Failed to get contest participants",
			zap.String("contest_id", event.ContestID),
			zap.Error(err))
		metrics.consumerErrors.WithLabelValues("query_participants").Inc()
		return
	}

	if len(userIDs) == 0 {
		log.Debug("No participants found for contest, skipping notification",
			zap.String("contest_id", event.ContestID))
		return
	}

	// Build notification content
	title := mapping.title
	message := mapping.messageFn(event.Data)

	// Build metadata with contest_id and extra event data
	metadata := map[string]interface{}{
		"contest_id": event.ContestID,
	}
	for k, v := range event.Data {
		metadata[k] = v
	}

	// Ensure contest_name is present for frontend i18n rendering
	if _, ok := metadata["contest_name"]; !ok {
		var name string
		err := a.db.QueryRowContext(a.ctx,
			"SELECT name FROM contests WHERE id = $1", event.ContestID).Scan(&name)
		if err == nil {
			metadata["contest_name"] = name
		}
	}

	// Filter users by their in-app notification preferences
	enabledMap, _ := prefs.IsEnabledBatch(a.ctx, a.db, userIDs, mapping.notifType, "in_app")
	var filteredIDs []string
	for _, uid := range userIDs {
		if enabledMap[uid] {
			filteredIDs = append(filteredIDs, uid)
		}
	}
	if len(filteredIDs) == 0 {
		log.Debug("All users disabled this notification category",
			zap.String("contest_id", event.ContestID),
			zap.String("event_type", event.Type))
		return
	}

	// Batch insert notifications for filtered participants
	err = inapp.CreateNotificationBatch(a.ctx, a.db, filteredIDs, mapping.notifType, title, message, metadata)
	if err != nil {
		log.Error("Failed to create notification batch",
			zap.String("contest_id", event.ContestID),
			zap.String("event_type", event.Type),
			zap.Int("user_count", len(filteredIDs)),
			zap.Error(err))
		metrics.consumerErrors.WithLabelValues("batch_insert").Inc()
		return
	}

	metrics.notificationsCreated.WithLabelValues(event.Type).Add(float64(len(filteredIDs)))

	log.Info("Created in-app notifications",
		zap.String("contest_id", event.ContestID),
		zap.String("event_type", event.Type),
		zap.Int("user_count", len(filteredIDs)),
		zap.Int("total_participants", len(userIDs)))

	// Send email notifications for contest_started events
	if event.Type == "contest_started" {
		a.sendContestStartedEmails(a.ctx, event, log)
	}
}

// getContestParticipantUserIDs queries all user IDs enrolled in a given contest.
func (a *App) getContestParticipantUserIDs(ctx context.Context, contestID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := a.db.QueryContext(ctx,
		`SELECT user_id FROM contest_participants WHERE contest_id = $1`,
		contestID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query contest participants: %w", err)
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("failed to scan user_id: %w", err)
		}
		userIDs = append(userIDs, uid)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return userIDs, nil
}

// sendContestStartedEmails sends email notifications to all participants when a contest starts.
func (a *App) sendContestStartedEmails(ctx context.Context, event NotificationEvent, log *zap.Logger) {
	if a.notifications == nil || !a.notifications.HasEmail() {
		return
	}

	emailNotifier := a.notifications.GetEmailNotifier()
	if emailNotifier == nil {
		return
	}

	// Get participant user IDs and emails, then filter by email preferences
	userEmails, err := a.getContestParticipantUserEmails(ctx, event.ContestID)
	if err != nil {
		log.Error("Failed to get participant emails for contest_started",
			zap.String("contest_id", event.ContestID),
			zap.Error(err))
		return
	}

	if len(userEmails) == 0 {
		log.Debug("No participant emails found, skipping contest_started email",
			zap.String("contest_id", event.ContestID))
		return
	}

	// Filter by email notification preferences
	ueUserIDs := make([]string, len(userEmails))
	for i, ue := range userEmails {
		ueUserIDs[i] = ue.userID
	}
	emailEnabledMap, _ := prefs.IsEnabledBatch(ctx, a.db, ueUserIDs, "contest_started", "email")
	var emails []string
	for _, ue := range userEmails {
		if emailEnabledMap[ue.userID] {
			emails = append(emails, ue.email)
		}
	}

	if len(emails) == 0 {
		log.Debug("All participants disabled email for this category, skipping contest_started email",
			zap.String("contest_id", event.ContestID))
		return
	}

	// Build email data
	data := notification.ContestStartedData{
		ContestName: contestName(event.Data),
		ContestID:   event.ContestID,
		TradeURL:    a.config.TradeFrontendURL,
	}
	if endsAt, ok := event.Data["ends_at"].(string); ok && endsAt != "" {
		data.EndsAt = endsAt
	}

	// Send batch emails
	result := emailNotifier.SendContestStartedBatch(ctx, emails, data)

	log.Info("Sent contest_started emails",
		zap.String("contest_id", event.ContestID),
		zap.Int("successful", len(result.Successful)),
		zap.Int("failed", len(result.Failed)),
		zap.Int("total_recipients", len(emails)))
}

// getContestParticipantEmails returns the email addresses of all participants in a contest.
func (a *App) getContestParticipantEmails(ctx context.Context, contestID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := a.db.QueryContext(ctx,
		`SELECT u.email FROM contest_participants cp
		 JOIN users u ON cp.user_id = u.id
		 WHERE cp.contest_id = $1 AND u.email IS NOT NULL AND u.email != ''`,
		contestID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query participant emails: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		emails = append(emails, email)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return emails, nil
}

type userEmail struct {
	userID string
	email  string
}

// getContestParticipantUserEmails returns user IDs and emails for all participants in a contest.
func (a *App) getContestParticipantUserEmails(ctx context.Context, contestID string) ([]userEmail, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := a.db.QueryContext(ctx,
		`SELECT u.id, u.email FROM contest_participants cp
		 JOIN users u ON cp.user_id = u.id
		 WHERE cp.contest_id = $1 AND u.email IS NOT NULL AND u.email != ''`,
		contestID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query participant user emails: %w", err)
	}
	defer rows.Close()

	var result []userEmail
	for rows.Next() {
		var ue userEmail
		if err := rows.Scan(&ue.userID, &ue.email); err != nil {
			return nil, fmt.Errorf("failed to scan user email: %w", err)
		}
		result = append(result, ue)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return result, nil
}

// Ensure *sql.DB satisfies the inapp.Execer interface at compile time.
var _ inapp.Execer = (*sql.DB)(nil)
