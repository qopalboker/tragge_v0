package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/config"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/shopspring/decimal"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// contestTrading and contestTradingMu are now fields on the App struct (main.go)
// to avoid shared package-level state between App instances.

// getDB returns the appropriate *sql.DB for contest operations.
// Uses dbPool.Primary() when sharding is enabled, falls back to direct db connection.
func (a *App) getDB() *sql.DB {
	if a.dbPool != nil {
		return a.dbPool.Primary()
	}
	return a.db
}

// ContestOperationsConsumer handles bulk contest operations like closing all positions
// and cancelling all orders when a contest ends.
type ContestOperationsConsumer struct {
	app *App
}

// NewContestOperationsConsumer creates a new contest operations consumer.
func NewContestOperationsConsumer(app *App) *ContestOperationsConsumer {
	return &ContestOperationsConsumer{app: app}
}

// consumeContestClosePositions consumes bulk close position requests for contest end.
func (a *App) consumeContestClosePositions() {
	defer a.wg.Done()

	topic := config.GetEnv("CONTEST_CLOSE_POSITIONS_TOPIC", "contest_close_positions.v1")
	opts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-contest-close"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024),
		kgo.FetchMinBytes(1),
	}
	opts = append(opts, infra.KafkaSecurityOpts()...)
	client, err := kgo.NewClient(opts...)
	if err != nil {
		a.log().Error("Failed to create contest close positions Kafka client", zap.Error(err))
		return
	}
	defer client.Close()

	a.log().Info("Starting contest close positions consumer", zap.String("topic", topic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Contest close positions consumer shutting down")
			return
		default:
		}

		fetches := client.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Contest close positions fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processContestClosePositionsRequest(record)
			}

			if err := client.CommitUncommittedOffsets(a.ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					a.log().Error("Contest close positions commit error", zap.Error(err))
				}
			}
		})
	}
}

// processContestClosePositionsRequest handles a request to close all positions for a contest.
func (a *App) processContestClosePositionsRequest(record *kgo.Record) {
	var req contracts.ClosePositionsRequest
	if err := json.Unmarshal(record.Value, &req); err != nil {
		a.log().Error("Failed to unmarshal contest close positions request", zap.Error(err))
		return
	}

	a.log().Info("Processing contest close all positions request",
		zap.String("contest_id", req.ContestID),
		zap.String("reason", req.Reason))

	closeTime := time.Now()
	if req.Ts > 0 {
		closeTime = time.UnixMilli(req.Ts)
	}

	// Close all positions for the contest
	closedCount, err := a.forceCloseAllContestPositions(a.ctx, req.ContestID, req.Reason, closeTime, req.ClosePrice)
	if err != nil {
		a.log().Error("Failed to close all positions for contest",
			zap.String("contest_id", req.ContestID),
			zap.Error(err))
		return
	}

	a.log().Info("Closed all positions for contest",
		zap.String("contest_id", req.ContestID),
		zap.Int64("closed_count", closedCount))
}

// closedPositionEvent holds Kafka event data to publish after the batch transaction commits.
type closedPositionEvent struct {
	positionClosed contracts.PositionClosedEvent
}

// forceCloseAllContestPositions closes all open positions for a contest atomically.
// All positions are closed within a single transaction to prevent concurrent modifications.
// Kafka events are published only after the transaction commits successfully.
func (a *App) forceCloseAllContestPositions(ctx context.Context, contestID, reason string, closeTime time.Time, closePrice *float64) (int64, error) {
	type openPosition struct {
		PositionID    string
		UserID        string
		Symbol        string
		Side          string
		QtyOpen       int64
		EntryPrice    float64
		QtyUsed       int64
		RealizedScore float64
	}

	var positions []openPosition
	var deferredEvents []closedPositionEvent

	// Track per-user accumulated realized PnL from this force-close batch
	type userAccum struct {
		deltaScore        float64
		deltaScoreDecimal decimal.Decimal
	}
	userAccums := make(map[string]*userAccum)

	err := WithTransaction(ctx, a.getDB(), func(tx *sql.Tx) error {
		// SELECT FOR UPDATE within the transaction — row locks held until commit
		rows, err := tx.QueryContext(ctx, `
			SELECT position_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
			FROM positions
			WHERE contest_id = $1 AND closed_at IS NULL
			FOR UPDATE
		`, contestID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p openPosition
			if err := rows.Scan(&p.PositionID, &p.UserID, &p.Symbol, &p.Side, &p.QtyOpen, &p.EntryPrice, &p.QtyUsed, &p.RealizedScore); err != nil {
				return fmt.Errorf("scan position: %w", err)
			}
			positions = append(positions, p)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()

		for _, pos := range positions {
			// Get market price if not provided
			var marketPrice float64
			if closePrice != nil {
				marketPrice = *closePrice
			} else {
				// Get from engine's price book using position side
				if a.engine != nil && a.engine.priceBook != nil {
					if price, ok := a.engine.priceBook.GetPrice(pos.Symbol, pos.Side); ok {
						marketPrice = price
					} else {
						// Fallback: use entry price (no gain/loss)
						marketPrice = pos.EntryPrice
						a.log().Warn("No market price available, using entry price",
							zap.String("position_id", pos.PositionID),
							zap.String("symbol", pos.Symbol))
					}
				} else {
					// Fallback: use entry price (no gain/loss)
					marketPrice = pos.EntryPrice
				}
			}

			// Calculate realized PnL using decimal precision
			score := calculateTradeScoreDecimal(pos.Side, pos.EntryPrice, marketPrice, pos.QtyUsed)
			realizedPnL := score.Float64

			// Update position as closed — also set qty_open to 0
			_, err := tx.ExecContext(ctx, `
				UPDATE positions
				SET closed_at = $1, realized_score = realized_score + $2, qty_open = 0
				WHERE position_id = $3
			`, closeTime, realizedPnL, pos.PositionID)
			if err != nil {
				return fmt.Errorf("close position %s: %w", pos.PositionID, err)
			}

			// Return QTY to user and update score
			_, err = tx.ExecContext(ctx, `
				UPDATE contest_participants
				SET qty_available = qty_available + $1, total_score = total_score + $2
				WHERE contest_id = $3 AND user_id = $4
			`, pos.QtyUsed, realizedPnL, contestID, pos.UserID)
			if err != nil {
				return fmt.Errorf("update participant for position %s: %w", pos.PositionID, err)
			}

			// Accumulate per-user realized PnL for the final PnLDelta
			ua, ok := userAccums[pos.UserID]
			if !ok {
				ua = &userAccum{}
				userAccums[pos.UserID] = ua
			}
			ua.deltaScore += realizedPnL
			ua.deltaScoreDecimal = ua.deltaScoreDecimal.Add(score.Decimal)

			// Collect position closed events to publish after commit
			deferredEvents = append(deferredEvents, closedPositionEvent{
				positionClosed: contracts.PositionClosedEvent{
					PositionID:    pos.PositionID,
					UserID:        pos.UserID,
					ContestID:     contestID,
					Symbol:        pos.Symbol,
					Side:          contracts.OrderSide(pos.Side),
					QtyClosed:     pos.QtyOpen,
					ClosePrice:    marketPrice,
					RealizedPnL:   realizedPnL,
					RealizedScore: realizedPnL,
					CloseReason:   reason,
					Ts:            closeTime.UnixMilli(),
				},
			})
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	if len(positions) == 0 {
		return 0, nil
	}

	// Query authoritative total_score from DB for all affected users.
	// The transaction just committed these values, so they reflect the correct final scores.
	// This avoids relying on in-memory state which may not have the user loaded (e.g., after engine restart).
	// Uses batched queries to avoid hitting PostgreSQL parameter limits with large user sets.
	userIDs := make([]string, 0, len(userAccums))
	for uid := range userAccums {
		userIDs = append(userIDs, uid)
	}
	userFinalScores, userFinalScoresDecimal := batchQueryTotalScores(ctx, a.getDB(), a.log(), contestID, userIDs)

	// Synchronize in-memory user state: remove positions, add realized score, release qty
	// This is best-effort for keeping the cache warm for any subsequent operations
	if a.engine != nil && a.engine.state != nil {
		cs := a.engine.state.GetOrCreateContest(contestID)
		if cs != nil {
			for i, pos := range positions {
				if us, ok := cs.GetUser(pos.UserID); ok {
					us.RemovePosition(pos.Symbol)
					us.AddRealizedScoreDecimal(decimal.NewFromFloat(deferredEvents[i].positionClosed.RealizedPnL))
					us.ReleaseQty(pos.QtyUsed)
				}
			}
		}
	}

	// Publish PositionClosedEvent for each position
	for _, evt := range deferredEvents {
		eventJSON, err := json.Marshal(evt.positionClosed)
		if err != nil {
			a.log().Warn("Failed to marshal position closed event", zap.Error(err))
			continue
		}

		record := &kgo.Record{
			Topic: a.config.PositionClosedTopic,
			Key:   []byte(contestID),
			Value: eventJSON,
		}

		if a.kafka != nil {
			a.kafka.Produce(ctx, record, nil)
		}
	}

	// Publish ONE PnLDelta per user with the correct TotalScore from DB.
	// After force-closing all positions, unrealized = 0, so TotalScore = RealizedScore.
	for userID, ua := range userAccums {
		// Use the authoritative total_score from DB (queried after TX commit)
		var totalRealizedScore float64
		var totalRealizedScoreDecimal decimal.Decimal
		if score, ok := userFinalScores[userID]; ok {
			totalRealizedScore = score
			totalRealizedScoreDecimal = userFinalScoresDecimal[userID]
		} else {
			// Fallback to in-memory state if DB query failed
			a.log().Warn("No DB total_score for user during force-close, falling back to in-memory",
				zap.String("user_id", userID),
				zap.String("contest_id", contestID))
			if a.engine != nil && a.engine.state != nil {
				cs := a.engine.state.GetOrCreateContest(contestID)
				if cs != nil {
					if us, ok := cs.GetUser(userID); ok {
						totalRealizedScore = us.GetRealizedScore()
						totalRealizedScoreDecimal = us.GetRealizedScoreDecimal()
					}
				}
			}
		}

		// All positions are closed, so unrealized = 0 and total = realized
		pnlDelta := contracts.PnLDelta{
			UserID:                 userID,
			ContestID:              contestID,
			DeltaScore:             ua.deltaScore,
			TotalScore:             totalRealizedScore,
			RealizedScore:          totalRealizedScore,
			UnrealizedScore:        0,
			Ts:                     closeTime.UnixMilli(),
			SeqNum:                 a.engine.pnlSeqNum.Add(1),
			DeltaScoreDecimal:      ua.deltaScoreDecimal.StringFixed(8),
			RealizedScoreDecimal:   totalRealizedScoreDecimal.StringFixed(8),
			UnrealizedScoreDecimal: "0.00000000",
			TotalScoreDecimal:      totalRealizedScoreDecimal.StringFixed(8),
		}

		pnlJSON, err := json.Marshal(pnlDelta)
		if err != nil {
			a.log().Warn("Failed to marshal PnL delta event",
				zap.String("user_id", userID),
				zap.Error(err))
			continue
		}

		pnlRecord := &kgo.Record{
			Topic: a.config.PnLDeltasTopic,
			Key:   []byte(contestID),
			Value: pnlJSON,
		}

		if a.kafka != nil {
			a.kafka.Produce(ctx, pnlRecord, nil)
		}
	}

	return int64(len(deferredEvents)), nil
}

// consumeContestCancelOrders consumes bulk cancel order requests for contest end.
func (a *App) consumeContestCancelOrders() {
	defer a.wg.Done()

	topic := config.GetEnv("CONTEST_CANCEL_ORDERS_TOPIC", "contest_cancel_orders.v1")
	cancelOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-contest-cancel"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024),
		kgo.FetchMinBytes(1),
	}
	cancelOpts = append(cancelOpts, infra.KafkaSecurityOpts()...)
	client, err := kgo.NewClient(cancelOpts...)
	if err != nil {
		a.log().Error("Failed to create contest cancel orders Kafka client", zap.Error(err))
		return
	}
	defer client.Close()

	a.log().Info("Starting contest cancel orders consumer", zap.String("topic", topic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Contest cancel orders consumer shutting down")
			return
		default:
		}

		fetches := client.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Contest cancel orders fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processContestCancelOrdersRequest(record)
			}

			if err := client.CommitUncommittedOffsets(a.ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					a.log().Error("Contest cancel orders commit error", zap.Error(err))
				}
			}
		})
	}
}

// processContestCancelOrdersRequest handles a request to cancel all pending orders for a contest.
func (a *App) processContestCancelOrdersRequest(record *kgo.Record) {
	var req contracts.CancelAllOrdersRequest
	if err := json.Unmarshal(record.Value, &req); err != nil {
		a.log().Error("Failed to unmarshal contest cancel orders request", zap.Error(err))
		return
	}

	a.log().Info("Processing contest cancel all orders request",
		zap.String("contest_id", req.ContestID),
		zap.String("reason", string(req.Reason)))

	// Cancel all pending orders for the contest
	cancelledCount, err := a.cancelAllContestOrders(a.ctx, req.ContestID, req.Reason)
	if err != nil {
		a.log().Error("Failed to cancel all orders for contest",
			zap.String("contest_id", req.ContestID),
			zap.Error(err))
		return
	}

	a.log().Info("Cancelled all pending orders for contest",
		zap.String("contest_id", req.ContestID),
		zap.Int64("cancelled_count", cancelledCount))
}

// cancelAllContestOrders cancels all pending orders for a contest and returns reserved QTY.
// The SELECT FOR UPDATE and all updates run within a single transaction.
func (a *App) cancelAllContestOrders(ctx context.Context, contestID string, reason contracts.CancelReason) (int64, error) {
	type pendingOrder struct {
		OrderID string
		UserID  string
		Symbol  string
		Qty     int64
	}

	var orders []pendingOrder
	var affected int64
	var cancelTime time.Time

	err := WithTransaction(ctx, a.getDB(), func(tx *sql.Tx) error {
		// SELECT FOR UPDATE within the transaction — row locks held until commit
		rows, err := tx.QueryContext(ctx, `
			SELECT order_id, user_id, symbol, qty
			FROM orders
			WHERE contest_id = $1 AND status IN ('pending', 'open')
			FOR UPDATE
		`, contestID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o pendingOrder
			if err := rows.Scan(&o.OrderID, &o.UserID, &o.Symbol, &o.Qty); err != nil {
				return fmt.Errorf("scan order: %w", err)
			}
			orders = append(orders, o)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()

		if len(orders) == 0 {
			return nil
		}

		cancelTime = time.Now()

		// Cancel all pending orders
		result, err := tx.ExecContext(ctx, `
			UPDATE orders
			SET status = 'cancelled', updated_at = $1
			WHERE contest_id = $2 AND status IN ('pending', 'open')
		`, cancelTime, contestID)
		if err != nil {
			return err
		}
		affected, _ = result.RowsAffected()

		// Return reserved QTY to participants
		qtyByUser := make(map[string]int64)
		for _, o := range orders {
			qtyByUser[o.UserID] += o.Qty
		}

		for userID, qty := range qtyByUser {
			_, err := tx.ExecContext(ctx, `
				UPDATE contest_participants
				SET qty_available = qty_available + $1
				WHERE contest_id = $2 AND user_id = $3
			`, qty, contestID, userID)
			if err != nil {
				return fmt.Errorf("return qty to user %s: %w", userID, err)
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	if len(orders) == 0 {
		return 0, nil
	}

	// Remove from pending order book if engine is available
	if a.engine != nil && a.engine.pendingBook != nil {
		for _, o := range orders {
			a.engine.pendingBook.RemovePendingOrder(o.Symbol, o.OrderID)
		}
	}

	// Synchronize in-memory user state: release reserved qty and remove pending orders
	if a.engine != nil && a.engine.state != nil {
		cs := a.engine.state.GetOrCreateContest(contestID)
		if cs != nil {
			qtyByUser := make(map[string]int64)
			for _, o := range orders {
				qtyByUser[o.UserID] += o.Qty
			}
			for userID, qty := range qtyByUser {
				if us, ok := cs.GetUser(userID); ok {
					us.ReleaseQty(qty)
				}
			}
			for _, o := range orders {
				if us, ok := cs.GetUser(o.UserID); ok {
					us.RemovePendingOrder(o.OrderID)
				}
			}
		}
	}

	// Publish order cancelled events
	for _, o := range orders {
		cancelledEvent := contracts.OrderCancelledEvent{
			OrderID:      o.OrderID,
			UserID:       o.UserID,
			ContestID:    contestID,
			Symbol:       o.Symbol,
			CancelReason: reason,
			Ts:           cancelTime.UnixMilli(),
		}

		eventJSON, err := json.Marshal(cancelledEvent)
		if err != nil {
			continue
		}

		record := &kgo.Record{
			Topic: a.config.OrderCancelledTopic,
			Key:   []byte(contestID),
			Value: eventJSON,
		}

		if a.kafka != nil {
			a.kafka.Produce(ctx, record, nil)
		}
	}

	return affected, nil
}

// SetContestTradingEnabled sets whether trading is enabled for a contest.
// This is called when a contest starts or ends.
func (a *App) SetContestTradingEnabled(contestID string, enabled bool) {
	a.contestTradingMu.Lock()
	defer a.contestTradingMu.Unlock()
	a.contestTrading[contestID] = enabled

	// Invalidate the contest cache so the engine re-fetches fresh state
	if a.engine != nil {
		a.engine.contestCache.Invalidate(contestID)
	}

	a.log().Info("Contest trading status changed",
		zap.String("contest_id", contestID),
		zap.Bool("enabled", enabled))
}

// IsContestTradingEnabled checks if trading is enabled for a contest.
func (a *App) IsContestTradingEnabled(contestID string) bool {
	a.contestTradingMu.RLock()
	defer a.contestTradingMu.RUnlock()
	enabled, exists := a.contestTrading[contestID]
	return !exists || enabled // Default to enabled if not explicitly set
}

// consumeContestStateEvents consumes contest state events to enable/disable trading.
func (a *App) consumeContestStateEvents() {
	defer a.wg.Done()

	topic := config.GetEnv("CONTESTS_TOPIC", "contests.v1")
	stateOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-contest-state"),
		kgo.ConsumeTopics(topic),
		kgo.DisableAutoCommit(),
		kgo.FetchMaxBytes(1024 * 1024),
		kgo.FetchMinBytes(1),
	}
	stateOpts = append(stateOpts, infra.KafkaSecurityOpts()...)
	client, err := kgo.NewClient(stateOpts...)
	if err != nil {
		a.log().Error("Failed to create contest state Kafka client", zap.Error(err))
		return
	}
	defer client.Close()

	a.log().Info("Starting contest state consumer", zap.String("topic", topic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Contest state consumer shutting down")
			return
		default:
		}

		fetches := client.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Contest state fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processContestStateEvent(record)
			}

			if err := client.CommitUncommittedOffsets(a.ctx); err != nil {
				if !errors.Is(err, context.Canceled) {
					a.log().Error("Contest state commit error", zap.Error(err))
				}
			}
		})
	}
}

// processContestStateEvent handles a contest state change event.
func (a *App) processContestStateEvent(record *kgo.Record) {
	var state contracts.ContestState
	if err := json.Unmarshal(record.Value, &state); err != nil {
		a.log().Error("Failed to unmarshal contest state", zap.Error(err))
		return
	}

	a.log().Info("Processing contest state event",
		zap.String("contest_id", state.ContestID),
		zap.String("status", string(state.Status)))

	// Invalidate contest cache so the next lookup re-fetches from DB.
	// This ensures status transitions (e.g., running → completed) are
	// respected immediately by order validation.
	if a.engine != nil {
		a.engine.contestCache.Invalidate(state.ContestID)
		// Also invalidate all participants for this contest so that
		// disqualifications or removals take effect.
		a.engine.participantCache.Invalidate(state.ContestID, "")
		// P2-4: Invalidate symbol cache so hot-reloaded symbols take effect
		// immediately when admin modifies contest symbols.
		if a.engine.symbolCache != nil {
			a.engine.symbolCache.Invalidate(state.ContestID)
		}
	}

	// Enable/disable trading based on contest status
	switch state.Status {
	case contracts.ContestStatusRunning:
		a.SetContestTradingEnabled(state.ContestID, true)
	case contracts.ContestStatusSettling,
		contracts.ContestStatusCompleted,
		contracts.ContestStatusCancelled,
		contracts.ContestStatusPaused:
		a.SetContestTradingEnabled(state.ContestID, false)
	}

	// Evict in-memory state for finalized contests after a grace period
	// to allow in-flight operations (TP/SL, PnL broadcasts) to complete.
	if state.Status.IsFinal() {
		contestID := state.ContestID
		infra.SafeGo(a.engine.logger, "contest-state-eviction", func() {
			time.Sleep(5 * time.Minute)
			if a.engine != nil {
				a.engine.state.RemoveContest(contestID)
				if a.engine.pendingBook != nil {
					removedOrders, removedPositions := a.engine.pendingBook.RemoveAllForContest(contestID)
					a.engine.logger.Info("Cleaned up pendingBook for finalized contest",
						zap.String("contest_id", contestID),
						zap.Int("removed_orders", removedOrders),
						zap.Int("removed_positions", removedPositions))
				}
				a.engine.logger.Info("Evicted contest state after finalization",
					zap.String("contest_id", contestID))
				if a.engine.rateLimiter != nil {
					a.engine.rateLimiter.RemoveContest(contestID)
				}
			}
		})
	}
}

// maxSQLBatchSize is the maximum number of user IDs per batched IN clause query.
// PostgreSQL supports up to 65535 parameters, but smaller batches are more practical.
const maxSQLBatchSize = 500

// batchQueryTotalScores queries total_score for a set of users in batches to avoid
// hitting PostgreSQL parameter limits with large user sets.
func batchQueryTotalScores(ctx context.Context, db *sql.DB, logger *zap.Logger, contestID string, userIDs []string) (map[string]float64, map[string]decimal.Decimal) {
	scores := make(map[string]float64, len(userIDs))
	scoresDecimal := make(map[string]decimal.Decimal, len(userIDs))

	for i := 0; i < len(userIDs); i += maxSQLBatchSize {
		end := i + maxSQLBatchSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		batch := userIDs[i:end]

		placeholders := make([]string, len(batch))
		args := make([]interface{}, 0, len(batch)+1)
		args = append(args, contestID)
		for j, uid := range batch {
			placeholders[j] = fmt.Sprintf("$%d", j+2)
			args = append(args, uid)
		}

		query := `SELECT user_id, total_score FROM contest_participants
			WHERE contest_id = $1 AND user_id IN (` + strings.Join(placeholders, ",") + `)`

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			logger.Error("Failed to query total_score batch",
				zap.String("contest_id", contestID),
				zap.Int("batch_offset", i),
				zap.Error(err))
			continue
		}
		for rows.Next() {
			var uid string
			var score float64
			if err := rows.Scan(&uid, &score); err != nil {
				logger.Warn("Failed to scan total_score row", zap.Error(err))
				continue
			}
			scores[uid] = score
			scoresDecimal[uid] = decimal.NewFromFloat(score)
		}
		rows.Close()
	}

	return scores, scoresDecimal
}
