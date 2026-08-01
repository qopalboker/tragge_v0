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

// ContestEventsConsumer handles consumption of contest lifecycle events
// and broadcasts them to connected WebSocket clients.
type ContestEventsConsumer struct {
	kafkaClient       *kgo.Client
	hub               *Hub
	tournamentFeedHub *TournamentFeedHub // Tournament feed hub for forwarding events (Task 8.3)
	log               *zap.Logger
	ctx               context.Context
}

// NewContestEventsConsumer creates a new contest events consumer.
func NewContestEventsConsumer(kafkaClient *kgo.Client, hub *Hub, tournamentFeedHub *TournamentFeedHub, log *zap.Logger, ctx context.Context) *ContestEventsConsumer {
	return &ContestEventsConsumer{
		kafkaClient:       kafkaClient,
		hub:               hub,
		tournamentFeedHub: tournamentFeedHub,
		log:               log,
		ctx:               ctx,
	}
}

// Run starts the consumer loop.
func (c *ContestEventsConsumer) Run() {
	c.log.Info("Starting contest events consumer")

	for {
		select {
		case <-c.ctx.Done():
			c.log.Info("Contest events consumer shutting down")
			return
		default:
		}

		fetches := c.kafkaClient.PollFetches(c.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			c.log.Error("Contest events fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				c.processRecord(record)
			}
		})

		if err := c.kafkaClient.CommitUncommittedOffsets(c.ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				c.log.Error("Contest events commit error", zap.Error(err))
			}
		}
	}
}

// processRecord processes a single contest event record.
func (c *ContestEventsConsumer) processRecord(record *kgo.Record) {
	var event contracts.ContestEvent
	if err := json.Unmarshal(record.Value, &event); err != nil {
		c.log.Error("Failed to unmarshal contest event", zap.Error(err))
		return
	}

	c.log.Info("Processing contest event",
		zap.String("type", string(event.Type)),
		zap.String("contest_id", event.ContestID),
		zap.String("message", event.Message))

	// Broadcast to all clients connected to this contest
	wsMsg := &WSMessage{
		Type: string(event.Type),
		Payload: map[string]interface{}{
			"contest_id": event.ContestID,
			"name":       event.Name,
			"message":    event.Message,
			"ends_at":    event.EndsAt,
			"timestamp":  event.Ts,
			"metadata":   event.Metadata,
		},
	}

	c.hub.SendToContest(event.ContestID, wsMsg)

	// For certain events, also send to all connected clients (broadcast)
	switch event.Type {
	case contracts.ContestEventStarted:
		// Log start for monitoring
		c.log.Info("Contest started - broadcasting to participants",
			zap.String("contest_id", event.ContestID),
			zap.String("name", event.Name))

	case contracts.ContestEventTradingEnded:
		// Log end for monitoring
		c.log.Info("Trading ended - broadcasting to participants",
			zap.String("contest_id", event.ContestID))

	case contracts.ContestEventSettling:
		// Log settling phase
		c.log.Info("Contest settling - calculating results",
			zap.String("contest_id", event.ContestID))

	case contracts.ContestEventCompleted:
		// Log completion
		c.log.Info("Contest completed - results ready",
			zap.String("contest_id", event.ContestID))

	case contracts.ContestEventCancelled:
		// Log cancellation
		c.log.Warn("Contest cancelled",
			zap.String("contest_id", event.ContestID),
			zap.String("message", event.Message))

	case contracts.ContestEventPaused:
		// Log pause
		c.log.Info("Contest paused",
			zap.String("contest_id", event.ContestID))

	case contracts.ContestEventResumed:
		// Log resume
		c.log.Info("Contest resumed",
			zap.String("contest_id", event.ContestID))

	case contracts.ContestEventUpdated:
		// Invalidate symbol cache so fresh data is loaded on next access
		c.hub.InvalidateContestSymbolsCache(event.ContestID)
		c.log.Info("Contest updated - invalidated symbol cache",
			zap.String("contest_id", event.ContestID))
	}

	// Forward event to tournament feed hub (Task 8.3)
	if c.tournamentFeedHub != nil {
		c.tournamentFeedHub.forwardContestEvent(event)
	}
}

// initContestEventsConsumer initializes the Kafka client for contest events.
func (a *App) initContestEventsConsumer() error {
	topic := config.GetEnv("CONTEST_STATE_TOPIC", "contests.v1")

	opts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-contest-events"),
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

	a.log().Info("Kafka contest events consumer initialized",
		zap.String("group", a.config.ConsumerGroup+"-contest-events"),
		zap.String("topic", topic))

	// Store client for cleanup and start consumer
	a.wg.Add(1)
	infra.SafeGo(a.log(), "kafka-contest-events-consumer", func() {
		defer a.wg.Done()
		defer client.Close()

		consumer := NewContestEventsConsumer(client, a.hub, a.tournamentFeedHub, a.log(), a.ctx)
		consumer.Run()
	})

	return nil
}

// BroadcastContestEvent broadcasts a contest event to all connected clients.
// This is called when events need to be broadcast locally without going through Kafka.
func (h *Hub) BroadcastContestEvent(event *contracts.ContestEvent) {
	if event == nil {
		return
	}

	wsMsg := &WSMessage{
		Type: string(event.Type),
		Payload: map[string]interface{}{
			"contest_id": event.ContestID,
			"name":       event.Name,
			"message":    event.Message,
			"ends_at":    event.EndsAt,
			"timestamp":  event.Ts,
			"metadata":   event.Metadata,
		},
	}

	h.SendToContest(event.ContestID, wsMsg)
}

// BroadcastContestResults broadcasts contest results to all participants.
func (h *Hub) BroadcastContestResults(contestID string, results *contracts.ContestResults) {
	if results == nil {
		return
	}

	wsMsg := &WSMessage{
		Type: "results_ready",
		Payload: map[string]interface{}{
			"contest_id":         results.ContestID,
			"contest_name":       results.ContestName,
			"total_participants": results.TotalParticipants,
			"winners_count":      results.WinnersCount,
			"prize_pool_cents":   results.PrizePoolCents,
			"total_paid_cents":   results.TotalPaidCents,
			"finalized_at":       results.FinalizedAt,
			"timestamp":          time.Now().UnixMilli(),
		},
	}

	h.SendToContest(contestID, wsMsg)
}

// SendContestNotificationToUser sends a contest notification to a specific user.
func (h *Hub) SendContestNotificationToUser(userID string, notification *contracts.ContestNotification) {
	if notification == nil {
		return
	}

	// Only send in-app notifications through WebSocket
	hasInApp := false
	for _, ch := range notification.Channels {
		if ch == contracts.ChannelInApp {
			hasInApp = true
			break
		}
	}

	if !hasInApp {
		return
	}

	wsMsg := &WSMessage{
		Type: "notification",
		Payload: map[string]interface{}{
			"type":       notification.Type,
			"contest_id": notification.ContestID,
			"title":      notification.Title,
			"body":       notification.Body,
			"data":       notification.Data,
			"priority":   notification.Priority,
			"timestamp":  notification.Ts,
		},
	}

	h.SendToUser(userID, wsMsg)
}

