package server

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"go.uber.org/zap"
)

// ContestParticipantSubscriber listens for participant join/leave events
// via Redis pub/sub and broadcasts updates to all WebSocket clients
// viewing that contest. This enables real-time prize pool recalculation.
type ContestParticipantSubscriber struct {
	hub *Hub
	app *App
	log *zap.Logger
	ctx context.Context
}

// NewContestParticipantSubscriber creates a new subscriber.
func NewContestParticipantSubscriber(app *App, hub *Hub, log *zap.Logger, ctx context.Context) *ContestParticipantSubscriber {
	return &ContestParticipantSubscriber{
		hub: hub,
		app: app,
		log: log,
		ctx: ctx,
	}
}

// Run subscribes to Redis pub/sub using pattern matching for all contest channels
// and broadcasts received events to WebSocket clients.
func (s *ContestParticipantSubscriber) Run() {
	s.log.Info("Starting contest participant subscriber via Redis pub/sub")

	// Subscribe to all contest_updates:* channels using pattern subscribe
	pubsub := s.app.redis.Client().PSubscribe(s.ctx, "contest_updates:*")
	defer pubsub.Close()

	ch := pubsub.Channel()

	for {
		select {
		case <-s.ctx.Done():
			s.log.Info("Contest participant subscriber shutting down")
			return
		case msg, ok := <-ch:
			if !ok {
				s.log.Info("Contest participant subscriber channel closed")
				return
			}

			// Extract contest ID from channel name: "contest_updates:{contest_id}"
			parts := strings.SplitN(msg.Channel, ":", 2)
			if len(parts) != 2 {
				s.log.Warn("Invalid channel format", zap.String("channel", msg.Channel))
				continue
			}
			contestID := parts[1]

			// Parse the payload
			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				s.log.Warn("Failed to unmarshal contest update payload",
					zap.Error(err), zap.String("channel", msg.Channel))
				continue
			}

			s.log.Info("Broadcasting contest participant update",
				zap.String("contest_id", contestID),
				zap.String("event", stringFromPayload(payload, "event")))

			// Broadcast to all WebSocket clients connected to this contest
			wsMsg := &WSMessage{
				Type:    "contest_updated",
				Payload: payload,
			}
			s.hub.SendToContest(contestID, wsMsg)
		}
	}
}

// stringFromPayload extracts a string value from a map payload.
func stringFromPayload(payload map[string]interface{}, key string) string {
	if v, ok := payload[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// initContestParticipantSubscriber starts the Redis pub/sub subscriber
// for contest participant events.
func (a *App) initContestParticipantSubscriber() {
	a.wg.Add(1)
	infra.SafeGo(a.log(), "contest-participant-subscriber", func() {
		defer a.wg.Done()
		subscriber := NewContestParticipantSubscriber(a, a.hub, a.log(), a.ctx)
		subscriber.Run()
	})
	a.log().Info("Contest participant subscriber initialized (Redis pub/sub)")
}
