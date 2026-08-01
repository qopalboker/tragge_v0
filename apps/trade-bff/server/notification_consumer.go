package server

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/config"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// NotificationConsumer handles consumption of per-user settlement notifications
// from the notifications.v1 topic and pushes them via WebSocket.
type NotificationConsumer struct {
	kafkaClient *kgo.Client
	hub         *Hub
	log         *zap.Logger
	ctx         context.Context
}

// NewNotificationConsumer creates a new notification consumer.
func NewNotificationConsumer(kafkaClient *kgo.Client, hub *Hub, log *zap.Logger, ctx context.Context) *NotificationConsumer {
	return &NotificationConsumer{
		kafkaClient: kafkaClient,
		hub:         hub,
		log:         log,
		ctx:         ctx,
	}
}

// Run starts the consumer loop.
func (c *NotificationConsumer) Run() {
	c.log.Info("Starting notification consumer")

	for {
		select {
		case <-c.ctx.Done():
			c.log.Info("Notification consumer shutting down")
			return
		default:
		}

		fetches := c.kafkaClient.PollFetches(c.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			c.log.Error("Notification fetch error", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				c.processRecord(record)
			}
		})

		if err := c.kafkaClient.CommitUncommittedOffsets(c.ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				c.log.Error("Notification commit error", zap.Error(err))
			}
		}
	}
}

// processRecord processes a single notification record.
func (c *NotificationConsumer) processRecord(record *kgo.Record) {
	var notif contracts.SettlementNotification
	if err := json.Unmarshal(record.Value, &notif); err != nil {
		c.log.Error("Failed to unmarshal settlement notification", zap.Error(err))
		return
	}

	if notif.UserID == "" {
		c.log.Warn("Received notification with empty user_id")
		return
	}

	wsMsg := &WSMessage{
		Type: "notification",
		Payload: map[string]interface{}{
			"type":         notif.Type,
			"contest_id":   notif.ContestID,
			"contest_name": notif.ContestName,
			"data":         notif.Data,
			"timestamp":    notif.Ts,
		},
	}

	c.hub.SendToUser(notif.UserID, wsMsg)

	c.log.Debug("Pushed settlement notification via WebSocket",
		zap.String("user_id", notif.UserID),
		zap.String("type", notif.Type),
		zap.String("contest_id", notif.ContestID))
}

// initNotificationConsumer initializes the Kafka client for settlement notifications.
func (a *App) initNotificationConsumer() error {
	topic := config.GetEnv("NOTIFICATIONS_TOPIC", "notifications.v1")

	// Skip if topic is explicitly disabled
	if topic == "" {
		a.log().Info("Notification consumer disabled (empty topic)")
		return nil
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-notifications"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024),
		kgo.FetchMinBytes(1),
	}
	opts = append(opts, infra.KafkaSecurityOpts()...)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		return err
	}

	a.log().Info("Kafka notification consumer initialized",
		zap.String("group", a.config.ConsumerGroup+"-notifications"),
		zap.String("topic", topic))

	a.wg.Add(1)
	infra.SafeGo(a.log(), "kafka-notification-consumer", func() {
		defer a.wg.Done()
		defer client.Close()

		consumer := NewNotificationConsumer(client, a.hub, a.log(), a.ctx)
		consumer.Run()
	})

	return nil
}

