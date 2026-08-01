package server

import (
	"context"
	"encoding/json"
	"errors"
	"runtime/debug"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// ====================
// Kafka Consumers
// ====================

func (a *App) initKafkaConsumers() {
	var err error
	secOpts := infra.KafkaSecurityOpts()

	// Ticks consumer - optimized for high volume tick data
	tickOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-ticks"),
		kgo.ConsumeTopics(a.config.TicksTopic),
		kgo.DisableAutoCommit(),
		// High-throughput consumer settings for tick data
		kgo.FetchMaxBytes(1024 * 1024 * 10),        // 10MB max fetch (ticks are high volume)
		kgo.FetchMaxPartitionBytes(1024 * 1024 * 2), // 2MB per partition
		kgo.FetchMinBytes(1024),                     // Batch more records
	}
	tickOpts = append(tickOpts, secOpts...)
	a.ticksKafka, err = kgo.NewClient(tickOpts...)
	if err != nil {
		a.log().Warn("Failed to create ticks Kafka client", zap.Error(err))
	} else {
		a.log().Info("Kafka ticks consumer initialized (high-throughput mode)",
			zap.String("group", a.config.ConsumerGroup+"-ticks"),
			zap.String("topic", a.config.TicksTopic))
	}

	// Fills consumer - optimized for moderate throughput
	fillOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-fills"),
		kgo.ConsumeTopics(a.config.FillsTopic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024 * 5),       // 5MB max fetch
		kgo.FetchMaxPartitionBytes(1024 * 1024), // 1MB per partition
	}
	fillOpts = append(fillOpts, secOpts...)
	a.fillsKafka, err = kgo.NewClient(fillOpts...)
	if err != nil {
		a.log().Warn("Failed to create fills Kafka client", zap.Error(err))
	} else {
		a.log().Info("Kafka fills consumer initialized (high-throughput mode)",
			zap.String("group", a.config.ConsumerGroup+"-fills"),
			zap.String("topic", a.config.FillsTopic))
	}

	// Positions consumer - optimized for moderate throughput
	posOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-positions"),
		kgo.ConsumeTopics(a.config.PositionsTopic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024 * 5),       // 5MB max fetch
		kgo.FetchMaxPartitionBytes(1024 * 1024), // 1MB per partition
	}
	posOpts = append(posOpts, secOpts...)
	a.positionsKafka, err = kgo.NewClient(posOpts...)
	if err != nil {
		a.log().Warn("Failed to create positions Kafka client", zap.Error(err))
	} else {
		a.log().Info("Kafka positions consumer initialized (high-throughput mode)",
			zap.String("group", a.config.ConsumerGroup+"-positions"),
			zap.String("topic", a.config.PositionsTopic))
	}

	// Order acks consumer - optimized for low-latency delivery
	ackOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-acks"),
		kgo.ConsumeTopics(a.config.OrderAcksTopic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024), // 1MB max fetch (acks are small)
		kgo.FetchMinBytes(1),           // Don't wait, deliver immediately
	}
	ackOpts = append(ackOpts, secOpts...)
	a.orderAcksKafka, err = kgo.NewClient(ackOpts...)
	if err != nil {
		a.log().Warn("Failed to create order acks Kafka client", zap.Error(err))
	} else {
		a.log().Info("Kafka order acks consumer initialized (low-latency mode)",
			zap.String("group", a.config.ConsumerGroup+"-acks"),
			zap.String("topic", a.config.OrderAcksTopic))
	}

	// Order cancelled consumer - optimized for low-latency delivery
	cancelOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-cancelled"),
		kgo.ConsumeTopics(a.config.OrderCancelledTopic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024), // 1MB max fetch (cancelled events are small)
		kgo.FetchMinBytes(1),           // Don't wait, deliver immediately
	}
	cancelOpts = append(cancelOpts, secOpts...)
	a.orderCancelledKafka, err = kgo.NewClient(cancelOpts...)
	if err != nil {
		a.log().Warn("Failed to create order cancelled Kafka client", zap.Error(err))
	} else {
		a.log().Info("Kafka order cancelled consumer initialized (low-latency mode)",
			zap.String("group", a.config.ConsumerGroup+"-cancelled"),
			zap.String("topic", a.config.OrderCancelledTopic))
	}

	// PnL deltas consumer - real-time score updates for frontend
	pnlOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-pnl"),
		kgo.ConsumeTopics(a.config.PnLDeltasTopic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024 * 5),       // 5MB max fetch
		kgo.FetchMaxPartitionBytes(1024 * 1024), // 1MB per partition
		kgo.FetchMinBytes(1),                    // Low latency for real-time updates
	}
	pnlOpts = append(pnlOpts, secOpts...)
	a.pnlDeltasKafka, err = kgo.NewClient(pnlOpts...)
	if err != nil {
		a.log().Warn("Failed to create PnL deltas Kafka client", zap.Error(err))
	} else {
		a.log().Info("Kafka PnL deltas consumer initialized (real-time mode)",
			zap.String("group", a.config.ConsumerGroup+"-pnl"),
			zap.String("topic", a.config.PnLDeltasTopic))
	}

	// Orders producer - optimized for high throughput with compression
	prodOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerLinger(50 * time.Millisecond),           // Increased for better batching
		kgo.ProducerBatchCompression(kgo.Lz4Compression()), // LZ4: fast compression
		kgo.ProducerBatchMaxBytes(1024 * 1024),             // 1MB max batch
	}
	prodOpts = append(prodOpts, secOpts...)
	a.ordersKafka, err = kgo.NewClient(prodOpts...)
	if err != nil {
		a.log().Warn("Failed to create orders Kafka producer", zap.Error(err))
	} else {
		a.log().Info("Kafka orders producer initialized", zap.String("topic", a.config.OrdersTopic))
	}
}

func (a *App) startKafkaConsumers() {
	if a.ticksKafka != nil {
		a.wg.Add(1)
		go a.consumeTicks()
	}
	if a.fillsKafka != nil {
		a.wg.Add(1)
		go a.consumeFills()
	}
	if a.positionsKafka != nil {
		a.wg.Add(1)
		go a.consumePositions()
	}
	if a.orderAcksKafka != nil {
		a.wg.Add(1)
		go a.consumeOrderAcks()
	}
	if a.orderCancelledKafka != nil {
		a.wg.Add(1)
		go a.consumeOrderCancelled()
	}
	if a.pnlDeltasKafka != nil {
		a.wg.Add(1)
		go a.consumePnLDeltas()
	}
}

func (a *App) closeKafkaClients() {
	if a.ticksKafka != nil {
		a.ticksKafka.Close()
	}
	if a.fillsKafka != nil {
		a.fillsKafka.Close()
	}
	if a.positionsKafka != nil {
		a.positionsKafka.Close()
	}
	if a.orderAcksKafka != nil {
		a.orderAcksKafka.Close()
	}
	if a.orderCancelledKafka != nil {
		a.orderCancelledKafka.Close()
	}
	if a.pnlDeltasKafka != nil {
		a.pnlDeltasKafka.Close()
	}
	if a.ordersKafka != nil {
		a.ordersKafka.Close()
	}
}

// consumeTicks consumes tick snapshots from Kafka and updates the price book
func (a *App) consumeTicks() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("consumeTicks panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	a.log().Info("Starting ticks consumer", zap.String("topic", a.config.TicksTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Ticks consumer shutting down")
			return
		default:
		}

		fetches := a.ticksKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Ticks fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				var tick contracts.TickSnapshot
				if err := json.Unmarshal(record.Value, &tick); err != nil {
					a.log().Error("Failed to unmarshal tick", zap.Error(err))
					continue
				}
				a.priceBook.UpdateBatch(&tick)

				// Aggregate ticks into candles (disabled when market-ingestor handles aggregation)
				if a.config.CandleAggregationEnabled {
					for _, st := range tick.Symbols {
						ts := st.Timestamp
						if ts == 0 {
							ts = tick.Ts * 1000
						}
						vol := st.Volume
						if vol <= 0 {
							vol = 1.0 // fallback to tick count when volume unknown
						}
						a.candleAggregator.ProcessTick(st.Symbol, st.Last, ts, vol)
					}
				}
			}
		})

		if err := a.ticksKafka.CommitUncommittedOffsets(a.ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				a.log().Error("Ticks commit error", zap.Error(err))
			}
		}
	}
}

// consumeFills consumes fill events and sends to appropriate users
func (a *App) consumeFills() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("consumeFills panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	a.log().Info("Starting fills consumer", zap.String("topic", a.config.FillsTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Fills consumer shutting down")
			return
		default:
		}

		fetches := a.fillsKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Fills fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				var fill contracts.FillEvent
				if err := json.Unmarshal(record.Value, &fill); err != nil {
					a.log().Error("Failed to unmarshal fill", zap.Error(err))
					continue
				}

				// Send to the specific user
				a.hub.SendToUser(fill.UserID, &WSMessage{
					Type:    "fill",
					Payload: fill,
				})
			}
		})

		if err := a.fillsKafka.CommitUncommittedOffsets(a.ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				a.log().Error("Fills commit error", zap.Error(err))
			}
		}
	}
}

// consumePositions consumes position updates and sends to appropriate users
func (a *App) consumePositions() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("consumePositions panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	a.log().Info("Starting positions consumer", zap.String("topic", a.config.PositionsTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Positions consumer shutting down")
			return
		default:
		}

		fetches := a.positionsKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Positions fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				var pos contracts.PositionUpdate
				if err := json.Unmarshal(record.Value, &pos); err != nil {
					a.log().Error("Failed to unmarshal position", zap.Error(err))
					continue
				}

				// Send to the specific user
				a.hub.SendToUser(pos.UserID, &WSMessage{
					Type:    "position_update",
					Payload: pos,
				})
			}
		})

		if err := a.positionsKafka.CommitUncommittedOffsets(a.ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				a.log().Error("Positions commit error", zap.Error(err))
			}
		}
	}
}

// consumeOrderAcks consumes order acknowledgments and sends to appropriate users
func (a *App) consumeOrderAcks() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("consumeOrderAcks panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	a.log().Info("Starting order acks consumer", zap.String("topic", a.config.OrderAcksTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Order acks consumer shutting down")
			return
		default:
		}

		fetches := a.orderAcksKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Order acks fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				// OrderAck with user_id and rate limit info
				var ack struct {
					contracts.OrderAck
					UserID string `json:"user_id"`
				}
				if err := json.Unmarshal(record.Value, &ack); err != nil {
					a.log().Error("Failed to unmarshal order ack", zap.Error(err))
					continue
				}

				if ack.UserID == "" {
					continue
				}

				// Handle rejection with rate limit metadata
				if ack.Status == contracts.OrderStatusRejected {
					var rateLimit *WSRateLimitInfo
					if ack.RateLimit != nil {
						rateLimit = &WSRateLimitInfo{
							Scope:        ack.RateLimit.Scope,
							Limit:        ack.RateLimit.Limit,
							Window:       ack.RateLimit.Window,
							RetryAfterMs: ack.RateLimit.RetryAfterMs,
						}
					}

					reason := "Unknown error"
					if ack.Reason != nil {
						reason = *ack.Reason
					}

					// Create the WebSocket rejection message
					reject := WSOrderReject{
						Type:      MsgTypeOrderReject,
						RequestID: "", // We don't have request_id at this point - order_id will be used
						Code:      reason,
						Message:   reason,
						RateLimit: rateLimit,
					}

					// Include order_id in the message for client-side correlation
					rejectWithOrderID := struct {
						WSOrderReject
						OrderID string `json:"order_id"`
					}{
						WSOrderReject: reject,
						OrderID:       ack.OrderID,
					}

					a.hub.SendToUser(ack.UserID, &WSMessage{Type: "order_reject", Payload: rejectWithOrderID})
				} else {
					// For accepted orders, send the standard ack
					a.hub.SendToUser(ack.UserID, &WSMessage{
						Type:    "order_ack",
						Payload: ack.OrderAck,
					})
				}
			}
		})

		if err := a.orderAcksKafka.CommitUncommittedOffsets(a.ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				a.log().Error("Order acks commit error", zap.Error(err))
			}
		}
	}
}

// consumeOrderCancelled consumes order cancelled events and sends to appropriate users
func (a *App) consumeOrderCancelled() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("consumeOrderCancelled panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	a.log().Info("Starting order cancelled consumer", zap.String("topic", a.config.OrderCancelledTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Order cancelled consumer shutting down")
			return
		default:
		}

		fetches := a.orderCancelledKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Order cancelled fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				var cancelled contracts.OrderCancelledEvent
				if err := json.Unmarshal(record.Value, &cancelled); err != nil {
					a.log().Error("Failed to unmarshal order cancelled event", zap.Error(err))
					continue
				}

				// Send to the specific user
				a.hub.SendToUser(cancelled.UserID, &WSMessage{
					Type:    "order_cancelled",
					Payload: cancelled,
				})
			}
		})

		if err := a.orderCancelledKafka.CommitUncommittedOffsets(a.ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				a.log().Error("Order cancelled commit error", zap.Error(err))
			}
		}
	}
}

// consumePnLDeltas consumes PnL delta events and sends real-time score updates to users
func (a *App) consumePnLDeltas() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("consumePnLDeltas panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	a.log().Info("Starting PnL deltas consumer",
		zap.String("topic", a.config.PnLDeltasTopic),
		zap.Duration("leaderboard_debounce", a.config.LeaderboardBroadcastDebounce))

	defer a.stopAllLeaderboardDebounceTimers()

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("PnL deltas consumer shutting down")
			return
		default:
		}

		fetches := a.pnlDeltasKafka.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("PnL deltas fetch error", zap.Error(err))
			continue
		}

		// Track contests with significant changes for debounced leaderboard broadcasts
		contestsWithChanges := make(map[string]bool)

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				var pnl contracts.PnLDelta
				if err := json.Unmarshal(record.Value, &pnl); err != nil {
					a.log().Error("Failed to unmarshal PnL delta", zap.Error(err))
					continue
				}

				// Send to the specific user for real-time score updates
				a.hub.SendToUser(pnl.UserID, &WSMessage{
					Type:    "pnl_delta",
					Payload: pnl,
				})

				// Track significant score changes for leaderboard broadcasts
				if pnl.DeltaScore >= a.config.LeaderboardBroadcastThreshold || pnl.DeltaScore <= -a.config.LeaderboardBroadcastThreshold {
					contestsWithChanges[pnl.ContestID] = true
				}
			}
		})

		if err := a.pnlDeltasKafka.CommitUncommittedOffsets(a.ctx); err != nil {
			if !errors.Is(err, context.Canceled) {
				a.log().Error("PnL deltas commit error", zap.Error(err))
			}
		}

		// Schedule debounced leaderboard broadcasts for affected contests
		for contestID := range contestsWithChanges {
			a.scheduleLeaderboardBroadcast(contestID)
		}
	}
}

// scheduleLeaderboardBroadcast schedules a debounced leaderboard broadcast for a contest.
// If a timer already exists for the contest, the pending broadcast will include the latest data
// when it fires. If no timer exists, a new one is started with the configured debounce interval.
func (a *App) scheduleLeaderboardBroadcast(contestID string) {
	a.lbDebounceMu.Lock()
	defer a.lbDebounceMu.Unlock()

	// If a timer is already running for this contest, let it fire — it will
	// fetch the latest top 10 at that point, so new changes are absorbed.
	if _, exists := a.lbDebounceTimers[contestID]; exists {
		return
	}

	// Start a new debounce timer for this contest
	timer := time.AfterFunc(a.config.LeaderboardBroadcastDebounce, func() {
		a.fireLeaderboardBroadcast(contestID)
	})
	a.lbDebounceTimers[contestID] = timer
}

// fireLeaderboardBroadcast executes the actual leaderboard broadcast for a contest
// and removes the debounce timer.
func (a *App) fireLeaderboardBroadcast(contestID string) {
	// Remove the timer entry first so new changes can start a fresh debounce cycle
	a.lbDebounceMu.Lock()
	delete(a.lbDebounceTimers, contestID)
	a.lbDebounceMu.Unlock()

	// Check if we're shutting down
	select {
	case <-a.ctx.Done():
		return
	default:
	}

	payload := map[string]interface{}{
		"contest_id": contestID,
		"timestamp":  time.Now().UnixMilli(),
	}

	// Include top 10 entries and total participants if leaderboard manager is available
	if a.leaderboardMgr != nil {
		top10, err := a.leaderboardMgr.GetTop10Cached(a.ctx, contestID)
		if err != nil {
			a.log().Warn("Failed to get top 10 for leaderboard broadcast",
				zap.String("contest_id", contestID),
				zap.Error(err),
			)
		} else {
			payload["top_entries"] = top10.Entries
			payload["total_participants"] = top10.TotalParticipants
		}
	}

	a.hub.SendToContest(contestID, &WSMessage{
		Type:    "leaderboard_updated",
		Payload: payload,
	})
}

// stopAllLeaderboardDebounceTimers stops all pending debounce timers.
// Called during shutdown to prevent goroutine leaks.
func (a *App) stopAllLeaderboardDebounceTimers() {
	a.lbDebounceMu.Lock()
	defer a.lbDebounceMu.Unlock()

	for contestID, timer := range a.lbDebounceTimers {
		timer.Stop()
		delete(a.lbDebounceTimers, contestID)
	}
	a.log().Info("Stopped all leaderboard debounce timers")
}
