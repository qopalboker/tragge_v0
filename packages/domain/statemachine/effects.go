// Package statemachine provides contest lifecycle state machine functionality.
package statemachine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/IBM/sarama"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/scoring/prize"
	"github.com/Parsaeffatravesh/tragge/packages/scoring"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// SideEffects handles side effects for state transitions.
type SideEffects struct {
	pool             *db.Pool
	kafkaProducer    sarama.SyncProducer
	logger           *zap.Logger
	contestHandlers  *ContestHandlers
	emailNotifier    *notification.EmailNotifier
	contestsURL      string // Base URL for contests page (e.g., "https://tragge.com/contests")
	walletService    *wallet.Service
	redisClient      redis.UniversalClient
}

// NewSideEffects creates a new SideEffects handler.
func NewSideEffects(pool *db.Pool, kafkaProducer sarama.SyncProducer, logger *zap.Logger) *SideEffects {
	if logger == nil {
		logger = zap.NewNop()
	}
	if kafkaProducer == nil {
		logger.Warn("SideEffects created with nil Kafka producer - all event publishing will be skipped")
	}
	return &SideEffects{
		pool:            pool,
		kafkaProducer:   kafkaProducer,
		logger:          logger,
		contestHandlers: NewContestHandlers(pool, kafkaProducer, logger, nil),
	}
}

// NewSideEffectsWithConfig creates a new SideEffects handler with custom configuration.
func NewSideEffectsWithConfig(pool *db.Pool, kafkaProducer sarama.SyncProducer, logger *zap.Logger, handlersConfig *HandlersConfig) *SideEffects {
	if logger == nil {
		logger = zap.NewNop()
	}
	if kafkaProducer == nil {
		logger.Warn("SideEffects created with nil Kafka producer - all event publishing will be skipped")
	}
	return &SideEffects{
		pool:            pool,
		kafkaProducer:   kafkaProducer,
		logger:          logger,
		contestHandlers: NewContestHandlers(pool, kafkaProducer, logger, handlersConfig),
	}
}

// SetEmailNotifier sets the email notifier for sending cancellation emails.
// contestsURL is the base URL for the contests listing page (e.g., "https://tragge.com/contests").
func (se *SideEffects) SetEmailNotifier(notifier *notification.EmailNotifier, contestsURL string) {
	se.emailNotifier = notifier
	se.contestsURL = contestsURL
}

// SetWalletService sets the wallet service for processing refunds atomically.
func (se *SideEffects) SetWalletService(svc *wallet.Service) {
	se.walletService = svc
}

// SetRedisClient sets the Redis client for fetching live market prices during position close.
func (se *SideEffects) SetRedisClient(client redis.UniversalClient) {
	se.redisClient = client
	if se.contestHandlers != nil {
		se.contestHandlers.SetRedisClient(client)
	}
}

// OnScheduled handles side effects when a contest is published (draft -> scheduled).
func (se *SideEffects) OnScheduled(ctx context.Context, result *TransitionResult) error {
	se.logger.Info("Contest published",
		zap.String("contest_id", result.Contest.ID),
		zap.String("name", result.Contest.Name))

	// Contest is now visible to users for registration
	// No immediate side effects needed - the status change makes it discoverable

	return nil
}

// OnRegistrationOpen handles side effects when registration opens for a contest.
func (se *SideEffects) OnRegistrationOpen(ctx context.Context, result *TransitionResult) error {
	se.logger.Info("Registration opened for contest",
		zap.String("contest_id", result.Contest.ID),
		zap.String("name", result.Contest.Name))

	// Notify users that registration is now open
	if err := se.queueNotification(ctx, result.Contest.ID, "registration_open", map[string]any{
		"contest_name": result.Contest.Name,
		"starts_at":    result.Contest.StartsAt,
	}); err != nil {
		se.logger.Error("Failed to queue registration open notification",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
	}

	return nil
}

// OnRegistrationClosed handles side effects when registration closes.
func (se *SideEffects) OnRegistrationClosed(ctx context.Context, result *TransitionResult) error {
	se.logger.Info("Registration closed for contest",
		zap.String("contest_id", result.Contest.ID),
		zap.Int("participants", result.Contest.CurrentParticipants))

	// Check if minimum participants requirement is met
	if result.Contest.CurrentParticipants < result.Contest.MinParticipants {
		se.logger.Warn("Contest has fewer than minimum participants",
			zap.String("contest_id", result.Contest.ID),
			zap.Int("current", result.Contest.CurrentParticipants),
			zap.Int("min", result.Contest.MinParticipants))
		// Note: The scheduler should handle auto-cancellation if needed
	}

	// Send notification to registered users (async via notification queue)
	if err := se.queueNotification(ctx, result.Contest.ID, "registration_closed", map[string]any{
		"contest_name": result.Contest.Name,
		"starts_at":    result.Contest.StartsAt,
		"participants": result.Contest.CurrentParticipants,
	}); err != nil {
		se.logger.Error("Failed to queue registration closed notification",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
	}

	return nil
}

// OnRunning handles side effects when a contest starts or resumes.
func (se *SideEffects) OnRunning(ctx context.Context, result *TransitionResult) error {
	// Check if resuming from paused state
	isResumingFromPause := result.FromStatus == StatusPaused

	if isResumingFromPause {
		se.logger.Info("Contest resumed",
			zap.String("contest_id", result.Contest.ID),
			zap.Time("new_ends_at", result.Contest.EndsAt),
			zap.Duration("total_paused_duration", result.Contest.TotalPausedDuration))

		// Broadcast contest_time_extended event for WebSocket delivery
		if err := se.broadcastTimeExtended(ctx, result.Contest); err != nil {
			se.logger.Error("Failed to broadcast time extended event",
				zap.String("contest_id", result.Contest.ID),
				zap.Error(err))
		}

		// Send notification about resume
		if err := se.queueNotification(ctx, result.Contest.ID, "contest_resumed", map[string]any{
			"contest_name": result.Contest.Name,
			"new_ends_at":  result.Contest.EndsAt,
		}); err != nil {
			se.logger.Error("Failed to queue contest resumed notification",
				zap.String("contest_id", result.Contest.ID),
				zap.Error(err))
		}

		return nil
	}

	se.logger.Info("Contest started",
		zap.String("contest_id", result.Contest.ID),
		zap.Int("participants", result.Contest.CurrentParticipants))

	// Use detailed contest handlers for comprehensive start handling
	if se.contestHandlers != nil {
		if err := se.contestHandlers.HandleContestStart(ctx, result); err != nil {
			se.logger.Error("Contest start handler failed",
				zap.String("contest_id", result.Contest.ID),
				zap.Error(err))
			// Continue with fallback behavior
		} else {
			// Detailed handler succeeded, return early
			return nil
		}
	}

	// Fallback: Basic initialization if detailed handlers not available
	// 1. Initialize all participant trading states (QTY allocation)
	if err := se.initializeParticipantStates(ctx, result.Contest); err != nil {
		return fmt.Errorf("failed to initialize participant states: %w", err)
	}

	// 2. Lock prizes based on final participant count
	if err := se.lockPrizesFallback(ctx, result.Contest); err != nil {
		se.logger.Error("Failed to lock prizes (fallback)",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
		// Non-fatal in fallback path: continue with other operations
	}

	// 3. Initialize leaderboard in Redis (via Kafka event, handled by leaderboard-worker)
	// The ContestState event already published will trigger this

	// 4. Send "Contest Started" notification to all participants
	if err := se.queueNotification(ctx, result.Contest.ID, "contest_started", map[string]any{
		"contest_name": result.Contest.Name,
		"ends_at":      result.Contest.EndsAt,
		"qty_total":    result.Contest.QtyTotal,
	}); err != nil {
		se.logger.Error("Failed to queue contest started notification",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
	}

	return nil
}

// broadcastTimeExtended broadcasts a contest_time_extended event for WebSocket delivery.
func (se *SideEffects) broadcastTimeExtended(ctx context.Context, contest *Contest) error {
	if se.kafkaProducer == nil {
		se.logger.Warn("Kafka producer is nil, skipping time extended broadcast",
			zap.String("contest_id", contest.ID))
		return nil
	}

	event := contracts.ContestEvent{
		Type:      contracts.ContestEventTimeExtended,
		ContestID: contest.ID,
		Name:      contest.Name,
		EndsAt:    contest.EndsAt.UnixMilli(),
		Message:   "Contest time has been extended due to pause",
		Metadata: map[string]any{
			"total_paused_duration_seconds": int64(contest.TotalPausedDuration.Seconds()),
		},
		Ts: time.Now().UnixMilli(),
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal time extended event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: "contests.v1",
		Key:   sarama.StringEncoder(contest.ID),
		Value: sarama.ByteEncoder(eventJSON),
	}

	_, _, err = se.kafkaProducer.SendMessage(msg)
	return err
}

// initializeParticipantStates initializes trading states for all participants.
func (se *SideEffects) initializeParticipantStates(ctx context.Context, contest *Contest) error {
	// Update all participants with initial QTY allocation
	_, err := se.pool.Primary().ExecContext(ctx, `
		UPDATE contest_participants
		SET qty_total = $1, qty_available = $1
		WHERE contest_id = $2
		  AND qty_total = 0
	`, contest.QtyTotal, contest.ID)

	if err != nil {
		return fmt.Errorf("failed to initialize participant QTY: %w", err)
	}

	se.logger.Info("Initialized participant trading states",
		zap.String("contest_id", contest.ID),
		zap.Int64("qty_total", contest.QtyTotal))

	return nil
}

// lockPrizesFallback is a simplified prize lock used when the detailed
// ContestHandlers are not available. It persists the lock row and updates
// the contests table but does not publish a Kafka event.
func (se *SideEffects) lockPrizesFallback(ctx context.Context, contest *Contest) error {
	participants := contest.CurrentParticipants
	if participants <= 0 || contest.EntryFeeCents <= 0 {
		return nil
	}

	commissionFraction := prize.MustCommissionPercentToFraction(contest.CommissionRate)
	grossPool := int64(participants) * int64(contest.EntryFeeCents)
	netPool, err := prize.CalculatePrizePool(participants, contest.EntryFeeCents, commissionFraction)
	if err != nil {
		return fmt.Errorf("calculate prize pool: %w", err)
	}
	platformFee := grossPool - netPool
	winnersCount := prize.GetWinnersCount(participants)
	slots := prize.CalculatePrizeDistribution(participants, netPool)

	distributionJSON, err := json.Marshal(slots)
	if err != nil {
		return fmt.Errorf("marshal distribution: %w", err)
	}

	_, err = se.pool.Primary().ExecContext(ctx, `
		INSERT INTO contest_prize_locks
			(contest_id, total_participants, prize_pool_gross_cents, prize_pool_net_cents,
			 platform_fee_cents, commission_rate, winners_count, distribution_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (contest_id) DO UPDATE SET
			total_participants     = EXCLUDED.total_participants,
			prize_pool_gross_cents = EXCLUDED.prize_pool_gross_cents,
			prize_pool_net_cents   = EXCLUDED.prize_pool_net_cents,
			platform_fee_cents     = EXCLUDED.platform_fee_cents,
			commission_rate        = EXCLUDED.commission_rate,
			winners_count          = EXCLUDED.winners_count,
			distribution_json      = EXCLUDED.distribution_json,
			locked_at              = NOW()
	`, contest.ID, participants, grossPool, netPool,
		platformFee, contest.CommissionRate, winnersCount, distributionJSON)
	if err != nil {
		return fmt.Errorf("insert contest_prize_locks: %w", err)
	}

	_, err = se.pool.Primary().ExecContext(ctx, `
		UPDATE contests
		SET prizes_locked_at = NOW(),
		    prize_pool_net_cents = $1
		WHERE id = $2
	`, netPool, contest.ID)
	if err != nil {
		return fmt.Errorf("update contests prize lock: %w", err)
	}

	se.logger.Info("Prizes locked (fallback)",
		zap.String("contest_id", contest.ID),
		zap.Int("participants", participants),
		zap.Int64("net_pool_cents", netPool),
		zap.Int("winners", winnersCount))

	return nil
}

// OnSettling handles side effects when trading ends and settlement begins.
func (se *SideEffects) OnSettling(ctx context.Context, result *TransitionResult) error {
	se.logger.Info("Contest settling",
		zap.String("contest_id", result.Contest.ID))

	// Use detailed contest handlers for comprehensive end handling
	if se.contestHandlers != nil {
		if err := se.contestHandlers.HandleContestEnd(ctx, result); err != nil {
			se.logger.Error("Contest end handler failed",
				zap.String("contest_id", result.Contest.ID),
				zap.Error(err))
			// Continue with fallback behavior
		} else {
			// Detailed handler succeeded, return early
			return nil
		}
	}

	// Fallback: Basic handling if detailed handlers not available
	// 1. Close all open positions at current market prices
	if err := se.closeAllPositions(ctx, result.Contest.ID); err != nil {
		se.logger.Error("Failed to close positions",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
		// Don't fail - positions can be closed manually
	}

	// 2. Cancel all pending orders
	if err := se.cancelPendingOrders(ctx, result.Contest.ID); err != nil {
		se.logger.Error("Failed to cancel pending orders",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
	}

	// 3. Publish event to trigger leaderboard-worker finalization
	// The ContestState event (FROZEN phase) triggers this

	return nil
}

// closeAllPositions closes all open positions for a contest, calculating realized
// P&L scores from the latest fill prices and updating participant totals.
func (se *SideEffects) closeAllPositions(ctx context.Context, contestID string) error {
	closeTime := time.Now()

	// Begin transaction on primary
	tx, err := se.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Fetch all open positions (locked for update)
	rows, err := tx.QueryContext(ctx, `
		SELECT position_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
		FROM positions
		WHERE contest_id = $1 AND closed_at IS NULL
		FOR UPDATE
	`, contestID)
	if err != nil {
		return fmt.Errorf("query open positions: %w", err)
	}

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
	for rows.Next() {
		var p openPosition
		if err := rows.Scan(&p.PositionID, &p.UserID, &p.Symbol, &p.Side, &p.QtyOpen, &p.EntryPrice, &p.QtyUsed, &p.RealizedScore); err != nil {
			continue
		}
		positions = append(positions, p)
	}
	rows.Close()

	if len(positions) == 0 {
		se.logger.Info("No open positions to close",
			zap.String("contest_id", contestID))
		return nil
	}

	// Fetch live market prices from Redis (primary source)
	symbolSet := make(map[string]bool, len(positions))
	for _, p := range positions {
		symbolSet[p.Symbol] = true
	}
	symbols := make([]string, 0, len(symbolSet))
	for s := range symbolSet {
		symbols = append(symbols, s)
	}

	var marketPrices map[string]MarketPrice
	if se.redisClient != nil {
		var redisErr error
		marketPrices, redisErr = getMarketPricesFromRedis(ctx, se.redisClient, symbols)
		if redisErr != nil {
			se.logger.Warn("Failed to fetch market prices from Redis, falling back to fill prices",
				zap.String("contest_id", contestID),
				zap.Error(redisErr))
		}
	}

	// Fetch fill prices as fallback
	fillPrices, err := getLatestFillPrices(ctx, tx, contestID)
	if err != nil {
		se.logger.Warn("Failed to fetch fill prices, will use entry prices as fallback",
			zap.String("contest_id", contestID),
			zap.Error(err))
		fillPrices = make(map[string]float64)
	}

	// Alert if no price sources are available - all positions will close at entry price
	if len(marketPrices) == 0 && len(fillPrices) == 0 {
		se.logger.Error("No market or fill prices available for settlement, all positions will close at entry price (break-even)",
			zap.String("contest_id", contestID),
			zap.Int("positions_affected", len(positions)))
	}

	// Collect events for post-commit publishing
	type closedPositionEvent struct {
		positionClosed contracts.PositionClosedEvent
		pnlDelta       contracts.PnLDelta
	}
	var deferredEvents []closedPositionEvent

	// Close each position with realized score calculation
	for _, pos := range positions {
		// Determine exit price with 3-tier fallback:
		// 1. Redis market price (Bid for long, Ask for short)
		// 2. Fill price from database
		// 3. Entry price (break-even)
		exitPrice := pos.EntryPrice
		priceSource := "entry_price"
		if mp, ok := marketPrices[pos.Symbol]; ok && mp.Bid > 0 && mp.Ask > 0 {
			exitPrice = getExitPriceFromMarket(mp, pos.Side)
			priceSource = "market_price"
		} else if fp, ok := fillPrices[pos.Symbol]; ok {
			exitPrice = fp
			priceSource = "fill_price"
		} else {
			se.logger.Warn("No market or fill price available, using entry price as exit (break-even)",
				zap.String("position_id", pos.PositionID),
				zap.String("symbol", pos.Symbol))
		}
		if priceSource != "market_price" {
			se.logger.Info("Position exit price source",
				zap.String("position_id", pos.PositionID),
				zap.String("symbol", pos.Symbol),
				zap.String("source", priceSource),
				zap.Float64("exit_price", exitPrice))
		}

		// Calculate realized score using decimal precision
		entryDec := decimal.NewFromFloat(pos.EntryPrice)
		exitDec := decimal.NewFromFloat(exitPrice)
		scoreDelta := scoring.CalculateTradeScoreFromPrices(entryDec, exitDec, pos.QtyUsed, pos.Side)
		scoreDeltaF64 := scoring.ToFloat64(scoreDelta)

		// Update position with close time and realized score
		_, err := tx.ExecContext(ctx, `
			UPDATE positions
			SET closed_at = $1, realized_score = realized_score + $2
			WHERE position_id = $3
		`, closeTime, scoreDeltaF64, pos.PositionID)
		if err != nil {
			return fmt.Errorf("close position %s: %w", pos.PositionID, err)
		}

		// Return qty_used and add score to participant
		_, err = tx.ExecContext(ctx, `
			UPDATE contest_participants
			SET qty_available = qty_available + $1, total_score = total_score + $2
			WHERE contest_id = $3 AND user_id = $4
		`, pos.QtyUsed, scoreDeltaF64, contestID, pos.UserID)
		if err != nil {
			se.logger.Warn("Failed to update participant after position close",
				zap.String("contest_id", contestID),
				zap.String("user_id", pos.UserID),
				zap.Error(err))
		}

		deferredEvents = append(deferredEvents, closedPositionEvent{
			positionClosed: contracts.PositionClosedEvent{
				PositionID:    pos.PositionID,
				UserID:        pos.UserID,
				ContestID:     contestID,
				Symbol:        pos.Symbol,
				Side:          positionSideToOrderSide(pos.Side),
				QtyClosed:     pos.QtyOpen,
				ClosePrice:    exitPrice,
				RealizedPnL:   scoreDeltaF64,
				RealizedScore: scoreDeltaF64,
				CloseReason:   "contest_ended",
				Ts:            closeTime.UnixMilli(),
			},
			pnlDelta: contracts.PnLDelta{
				UserID:                 pos.UserID,
				ContestID:              contestID,
				DeltaScore:             scoreDeltaF64,
				RealizedScore:          scoreDeltaF64,
				UnrealizedScore:        0,
				TotalScore:             0,
				Ts:                     closeTime.UnixMilli(),
				DeltaScoreDecimal:      scoring.ToString(scoreDelta),
				RealizedScoreDecimal:   scoring.ToString(scoreDelta),
				UnrealizedScoreDecimal: "0.00000000",
				TotalScoreDecimal:      "0.00000000",
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	se.logger.Info("Closed open positions with realized scores",
		zap.String("contest_id", contestID),
		zap.Int("count", len(positions)))

	// Publish events after successful commit
	if se.kafkaProducer != nil {
		for _, evt := range deferredEvents {
			eventJSON, err := json.Marshal(evt.positionClosed)
			if err != nil {
				se.logger.Warn("Failed to marshal position closed event", zap.Error(err))
				continue
			}
			msg := &sarama.ProducerMessage{
				Topic: "close_positions.v1",
				Key:   sarama.StringEncoder(contestID),
				Value: sarama.ByteEncoder(eventJSON),
			}
			se.kafkaProducer.SendMessage(msg)

			pnlJSON, err := json.Marshal(evt.pnlDelta)
			if err != nil {
				se.logger.Warn("Failed to marshal PnL delta event", zap.Error(err))
				continue
			}
			pnlMsg := &sarama.ProducerMessage{
				Topic: "pnl.v1",
				Key:   sarama.StringEncoder(contestID),
				Value: sarama.ByteEncoder(pnlJSON),
			}
			se.kafkaProducer.SendMessage(pnlMsg)
		}
	}

	return nil
}

// cancelPendingOrders cancels all pending orders for a contest.
func (se *SideEffects) cancelPendingOrders(ctx context.Context, contestID string) error {
	result, err := se.pool.Primary().ExecContext(ctx, `
		UPDATE orders
		SET status = 'cancelled', updated_at = NOW()
		WHERE contest_id = $1
		  AND status IN ('pending', 'open')
	`, contestID)

	if err != nil {
		return fmt.Errorf("failed to cancel orders: %w", err)
	}

	affected, _ := result.RowsAffected()
	se.logger.Info("Cancelled pending orders",
		zap.String("contest_id", contestID),
		zap.Int64("count", affected))

	// Publish order cancellation events
	if se.kafkaProducer != nil && affected > 0 {
		// Fetch cancelled order IDs and publish cancellation events
		rows, err := se.pool.Replica().QueryContext(ctx, `
			SELECT order_id, user_id
			FROM orders
			WHERE contest_id = $1 AND status = 'cancelled'
		`, contestID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var orderID, userID string
				if err := rows.Scan(&orderID, &userID); err == nil {
					se.publishOrderCancelled(ctx, contestID, orderID, userID)
				}
			}
		}
	}

	return nil
}

// publishOrderCancelled publishes an order cancellation event.
func (se *SideEffects) publishOrderCancelled(ctx context.Context, contestID, orderID, userID string) {
	if se.kafkaProducer == nil {
		se.logger.Warn("Kafka producer is nil, skipping order cancelled publish",
			zap.String("contest_id", contestID),
			zap.String("order_id", orderID))
		return
	}

	reason := string(contracts.CancelReasonContestEnded)
	ack := contracts.OrderAck{
		OrderID: orderID,
		Status:  contracts.OrderStatusRejected,
		Reason:  &reason,
	}

	eventJSON, err := json.Marshal(ack)
	if err != nil {
		return
	}

	msg := &sarama.ProducerMessage{
		Topic: "order_acks.v1",
		Key:   sarama.StringEncoder(orderID),
		Value: sarama.ByteEncoder(eventJSON),
	}

	se.kafkaProducer.SendMessage(msg)
}

// OnCompleted handles side effects when settlement completes.
func (se *SideEffects) OnCompleted(ctx context.Context, result *TransitionResult) error {
	se.logger.Info("Contest completed",
		zap.String("contest_id", result.Contest.ID))

	// Use detailed contest handlers for comprehensive settlement handling
	if se.contestHandlers != nil {
		if err := se.contestHandlers.HandleSettlement(ctx, result); err != nil {
			se.logger.Error("Settlement handler failed",
				zap.String("contest_id", result.Contest.ID),
				zap.Error(err))
			// Continue with fallback behavior
		} else {
			// Detailed handler succeeded, return early
			return nil
		}
	}

	// Fallback: Basic handling if detailed handlers not available
	// 1. Prize distribution is handled by leaderboard-worker
	// 2. T-Point updates are handled by leaderboard-worker

	// 3. Send results notifications to all participants
	if err := se.queueNotification(ctx, result.Contest.ID, "contest_completed", map[string]any{
		"contest_name": result.Contest.Name,
	}); err != nil {
		se.logger.Error("Failed to queue contest completed notification",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
	}

	return nil
}

// OnCancelled handles side effects when a contest is cancelled.
func (se *SideEffects) OnCancelled(ctx context.Context, result *TransitionResult) error {
	se.logger.Info("Contest cancelled",
		zap.String("contest_id", result.Contest.ID),
		zap.String("reason", ptrToString(result.Contest.CancellationReason)))

	// 1. Process refunds for paid contests
	if result.Contest.EntryFeeCents > 0 {
		if err := se.processRefunds(ctx, result.Contest); err != nil {
			se.logger.Error("Failed to process refunds",
				zap.String("contest_id", result.Contest.ID),
				zap.Error(err))
			// Don't fail - refunds can be processed manually
		}
	}

	// 2. Close any open positions (shouldn't be any, but just in case)
	se.closeAllPositions(ctx, result.Contest.ID)

	// 3. Cancel any pending orders
	se.cancelPendingOrders(ctx, result.Contest.ID)

	// 4. Send cancellation emails to all participants
	reason := "No reason provided"
	if result.Contest.CancellationReason != nil {
		reason = *result.Contest.CancellationReason
	}

	se.sendCancellationEmails(ctx, result.Contest, reason)

	// 5. Queue Kafka notification for other services
	if err := se.queueNotification(ctx, result.Contest.ID, "contest_cancelled", map[string]any{
		"contest_name": result.Contest.Name,
		"reason":       reason,
		"refund":       result.Contest.EntryFeeCents > 0,
	}); err != nil {
		se.logger.Error("Failed to queue contest cancelled notification",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
	}

	return nil
}

// sendCancellationEmails sends cancellation notification emails to all contest participants.
func (se *SideEffects) sendCancellationEmails(ctx context.Context, contest *Contest, reason string) {
	if se.emailNotifier == nil {
		se.logger.Debug("Email notifier not configured, skipping cancellation emails",
			zap.String("contest_id", contest.ID))
		return
	}

	// Get participants with their emails and wallet balances
	participants, err := se.getParticipantsForCancellationEmail(ctx, contest.ID)
	if err != nil {
		se.logger.Error("Failed to get participants for cancellation emails",
			zap.String("contest_id", contest.ID),
			zap.Error(err))
		return
	}

	if len(participants) == 0 {
		se.logger.Debug("No participants to notify",
			zap.String("contest_id", contest.ID))
		return
	}

	se.logger.Info("Sending cancellation emails to participants",
		zap.String("contest_id", contest.ID),
		zap.Int("participant_count", len(participants)))

	// Build recipient list with personalized data
	recipients := make([]notification.ContestCancelledRecipient, 0, len(participants))
	hasRefund := contest.EntryFeeCents > 0

	// Format scheduled start time
	var scheduledStart string
	if !contest.StartsAt.IsZero() {
		scheduledStart = contest.StartsAt.Format("January 2, 2006 at 3:04 PM MST")
	}

	for _, p := range participants {
		data := notification.ContestCancelledData{
			UserName:       p.UserName,
			ContestID:      contest.ID,
			ContestName:    contest.Name,
			Reason:         reason,
			ScheduledStart: scheduledStart,
			ContestsURL:    se.contestsURL,
		}

		if hasRefund {
			data.RefundAmount = formatCents(int64(contest.EntryFeeCents))
			data.NewBalance = formatCents(p.WalletBalanceCents)
		}

		recipients = append(recipients, notification.ContestCancelledRecipient{
			Email: p.Email,
			Data:  data,
		})
	}

	// Send emails in batch
	result := se.emailNotifier.SendContestCancelledBatch(ctx, recipients)

	// Log results
	se.logger.Info("Completed sending cancellation emails",
		zap.String("contest_id", contest.ID),
		zap.Int("sent", len(result.Successful)),
		zap.Int("failed", len(result.Failed)))

	for _, failure := range result.Failed {
		se.logger.Error("Failed to send cancellation email",
			zap.String("contest_id", contest.ID),
			zap.String("email", failure.Recipient),
			zap.Error(failure.Error))
	}
}

// participantForCancellation holds participant info needed for cancellation emails.
type participantForCancellation struct {
	UserID             string
	Email              string
	UserName           string
	WalletBalanceCents int64
}

// getParticipantsForCancellationEmail retrieves participants with their emails and wallet balances.
func (se *SideEffects) getParticipantsForCancellationEmail(ctx context.Context, contestID string) ([]participantForCancellation, error) {
	// Query participants with their emails, names, and wallet balances
	rows, err := se.pool.Replica().QueryContext(ctx, `
		SELECT
			u.id,
			u.email,
			COALESCE(u.name, '') as name,
			COALESCE(w.balance_cents, 0) as balance_cents
		FROM contest_participants cp
		JOIN users u ON u.id = cp.user_id
		LEFT JOIN wallets w ON w.user_id = u.id
		WHERE cp.contest_id = $1
	`, contestID)
	if err != nil {
		return nil, fmt.Errorf("failed to query participants: %w", err)
	}
	defer rows.Close()

	var participants []participantForCancellation
	for rows.Next() {
		var p participantForCancellation
		if err := rows.Scan(&p.UserID, &p.Email, &p.UserName, &p.WalletBalanceCents); err != nil {
			se.logger.Error("Failed to scan participant row", zap.Error(err))
			continue
		}
		participants = append(participants, p)
	}

	return participants, rows.Err()
}

// formatCents formats a cent amount as a dollar string (e.g., 1000 -> "$10.00").
func formatCents(cents int64) string {
	dollars := float64(cents) / 100.0
	return fmt.Sprintf("$%.2f", dollars)
}

// processRefunds processes refunds for all participants of a cancelled contest.
// Uses the wallet service with idempotency to prevent double-refunds.
func (se *SideEffects) processRefunds(ctx context.Context, contest *Contest) error {
	// Get all participants
	rows, err := se.pool.Replica().QueryContext(ctx, `
		SELECT user_id
		FROM contest_participants
		WHERE contest_id = $1
	`, contest.ID)
	if err != nil {
		return fmt.Errorf("failed to get participants: %w", err)
	}
	defer rows.Close()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		userIDs = append(userIDs, userID)
	}

	if len(userIDs) == 0 {
		return nil
	}

	// Determine reason code based on cancellation reason
	reasonCode := wallet.ReasonCodeContestRefundQuorum
	if contest.CancellationReason != nil {
		reason := *contest.CancellationReason
		// If cancelled by admin (not due to quorum), use admin reason code
		if reason != "" && !containsQuorumKeyword(reason) {
			reasonCode = wallet.ReasonCodeContestRefundAdmin
		}
	}

	// Wallet service is required for proper refund processing with ledger entries
	if se.walletService == nil {
		return errors.New("wallet service required for refund processing: legacy path removed")
	}
	return se.processRefundsWithWalletService(ctx, contest, userIDs, reasonCode)
}

// processRefundsWithWalletService processes refunds using the wallet service with idempotency.
func (se *SideEffects) processRefundsWithWalletService(ctx context.Context, contest *Contest, userIDs []string, reasonCode wallet.ReasonCode) error {
	var refunded, skipped, failed int

	for _, userID := range userIDs {
		tx, err := se.pool.Begin(ctx)
		if err != nil {
			se.logger.Error("Failed to begin refund transaction",
				zap.String("user_id", userID),
				zap.String("contest_id", contest.ID),
				zap.Error(err))
			failed++
			continue
		}

		_, err = se.walletService.RefundContestEntryFeeIdempotent(
			ctx, tx, userID, contest.ID, contest.Name,
			int64(contest.EntryFeeCents), reasonCode,
		)

		if err != nil {
			tx.Rollback()
			// DuplicateCreditError means already refunded — idempotent success
			if isDuplicateCreditError(err) {
				skipped++
				se.logger.Debug("Refund already processed (idempotent)",
					zap.String("user_id", userID),
					zap.String("contest_id", contest.ID))
				continue
			}
			se.logger.Error("Failed to refund user",
				zap.String("user_id", userID),
				zap.String("contest_id", contest.ID),
				zap.Error(err))
			failed++
			continue
		}

		if err := tx.Commit(); err != nil {
			se.logger.Error("Failed to commit refund",
				zap.String("user_id", userID),
				zap.String("contest_id", contest.ID),
				zap.Error(err))
			failed++
			continue
		}

		refunded++
	}

	se.logger.Info("Processed refunds",
		zap.String("contest_id", contest.ID),
		zap.Int("refunded", refunded),
		zap.Int("skipped_duplicate", skipped),
		zap.Int("failed", failed),
		zap.Int("total_participants", len(userIDs)),
		zap.Int("amount_cents", contest.EntryFeeCents))

	if failed > 0 {
		return fmt.Errorf("failed to refund %d of %d participants", failed, len(userIDs))
	}
	return nil
}

// containsQuorumKeyword checks if the cancellation reason mentions quorum/participant shortage.
func containsQuorumKeyword(reason string) bool {
	lower := strings.ToLower(reason)
	for _, kw := range []string{"minimum participants", "quorum", "not enough participants", "min participants"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// isDuplicateCreditError checks if an error is a duplicate credit error (idempotent success).
func isDuplicateCreditError(err error) bool {
	if err == nil {
		return false
	}
	var dupCredit *wallet.DuplicateCreditError
	if errors.As(err, &dupCredit) {
		return true
	}
	var dupPrize *wallet.DuplicatePrizeCreditError
	return errors.As(err, &dupPrize)
}

// OnPaused handles side effects when a contest is paused.
func (se *SideEffects) OnPaused(ctx context.Context, result *TransitionResult) error {
	se.logger.Info("Contest paused",
		zap.String("contest_id", result.Contest.ID))

	// Notify participants
	if err := se.queueNotification(ctx, result.Contest.ID, "contest_paused", map[string]any{
		"contest_name": result.Contest.Name,
	}); err != nil {
		se.logger.Error("Failed to queue contest paused notification",
			zap.String("contest_id", result.Contest.ID),
			zap.Error(err))
	}

	return nil
}

// queueNotification queues a notification for contest participants.
func (se *SideEffects) queueNotification(ctx context.Context, contestID, notificationType string, data map[string]any) error {
	if se.kafkaProducer == nil {
		se.logger.Warn("Kafka producer is nil, skipping notification publish",
			zap.String("contest_id", contestID),
			zap.String("notification_type", notificationType))
		return nil
	}

	// Publish to notifications topic
	notification := map[string]any{
		"type":       notificationType,
		"contest_id": contestID,
		"data":       data,
		"timestamp":  time.Now().UnixMilli(),
	}

	eventJSON, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: "notifications.v1",
		Key:   sarama.StringEncoder(contestID),
		Value: sarama.ByteEncoder(eventJSON),
	}

	_, _, err = se.kafkaProducer.SendMessage(msg)
	return err
}

// ptrToString safely converts a string pointer to string.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetRegisteredHandlers returns a map of side effect handlers for all states.
func (se *SideEffects) GetRegisteredHandlers() map[ContestStatus]SideEffectHandler {
	return map[ContestStatus]SideEffectHandler{
		StatusScheduled:          se.OnScheduled,
		StatusRegistrationOpen:   se.OnRegistrationOpen,
		StatusRegistrationClosed: se.OnRegistrationClosed,
		StatusRunning:            se.OnRunning,
		StatusSettling:           se.OnSettling,
		StatusCompleted:          se.OnCompleted,
		StatusCancelled:          se.OnCancelled,
		StatusPaused:             se.OnPaused,
	}
}

// schedulerAdvisoryLockKey is the PostgreSQL advisory lock key used to ensure
// only one SchedulerLoop instance processes auto-transitions at a time.
const schedulerAdvisoryLockKey int64 = 42

// SchedulerLoop runs the automatic state transition scheduler.
// It periodically checks for contests that need automatic transitions.
type SchedulerLoop struct {
	sm       *StateMachine
	effects  *SideEffects
	interval time.Duration
	logger   *zap.Logger
	stop     chan struct{}
}

// NewSchedulerLoop creates a new automatic transition scheduler.
func NewSchedulerLoop(sm *StateMachine, effects *SideEffects, interval time.Duration, logger *zap.Logger) *SchedulerLoop {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &SchedulerLoop{
		sm:       sm,
		effects:  effects,
		interval: interval,
		logger:   logger,
		stop:     make(chan struct{}),
	}
}

// Start begins the scheduler loop.
func (sl *SchedulerLoop) Start(ctx context.Context) {
	ticker := time.NewTicker(sl.interval)
	defer ticker.Stop()

	sl.logger.Info("Starting contest state machine scheduler",
		zap.Duration("interval", sl.interval))

	for {
		select {
		case <-ctx.Done():
			return
		case <-sl.stop:
			return
		case <-ticker.C:
			sl.processAutoTransitions(ctx)
		}
	}
}

// Stop stops the scheduler loop.
func (sl *SchedulerLoop) Stop() {
	close(sl.stop)
}

// processAutoTransitions finds and processes automatic state transitions.
// It acquires a PostgreSQL advisory lock to prevent duplicate processing
// across multiple scheduler instances.
func (sl *SchedulerLoop) processAutoTransitions(ctx context.Context) {
	// Acquire advisory lock (non-blocking) to prevent duplicate processing
	var acquired bool
	err := sl.sm.pool.Primary().QueryRowContext(ctx,
		"SELECT pg_try_advisory_lock($1)", schedulerAdvisoryLockKey,
	).Scan(&acquired)
	if err != nil {
		sl.logger.Error("Failed to acquire advisory lock", zap.Error(err))
		return
	}
	if !acquired {
		sl.logger.Debug("Another instance is processing auto-transitions, skipping")
		return
	}
	defer func() {
		if _, err := sl.sm.pool.Primary().ExecContext(ctx,
			"SELECT pg_advisory_unlock($1)", schedulerAdvisoryLockKey,
		); err != nil {
			sl.logger.Error("Failed to release advisory lock", zap.Error(err))
		}
	}()

	candidates, err := sl.sm.FindContestsForAutoTransition(ctx)
	if err != nil {
		sl.logger.Error("Failed to find contests for auto-transition", zap.Error(err))
		return
	}

	for _, candidate := range candidates {
		sl.processCandidate(ctx, candidate)
	}
}

// processCandidate processes a single auto-transition candidate.
func (sl *SchedulerLoop) processCandidate(ctx context.Context, candidate AutoTransitionCandidate) {
	// Validate requirements
	if candidate.SuggestedStatus == StatusRunning {
		if candidate.CurrentParticipants < candidate.MinParticipants {
			sl.logger.Warn("Contest cannot start due to insufficient participants",
				zap.String("contest_id", candidate.ContestID),
				zap.Int("current", candidate.CurrentParticipants),
				zap.Int("min", candidate.MinParticipants))
			// Auto-cancel if minimum not met
			sl.sm.Cancel(ctx, candidate.ContestID, nil,
				fmt.Sprintf("Auto-cancelled: minimum participants not met (%d/%d)",
					candidate.CurrentParticipants, candidate.MinParticipants))
			return
		}
	}

	// Perform the transition
	result, err := sl.sm.Transition(ctx, TransitionRequest{
		ContestID: candidate.ContestID,
		ToStatus:  candidate.SuggestedStatus,
		Reason:    candidate.Reason,
		ActorID:   nil, // Automatic transition
	})

	if err != nil {
		sl.logger.Error("Auto-transition failed",
			zap.String("contest_id", candidate.ContestID),
			zap.String("from", candidate.CurrentStatus.String()),
			zap.String("to", candidate.SuggestedStatus.String()),
			zap.Error(err))
		return
	}

	sl.logger.Info("Auto-transition completed",
		zap.String("contest_id", candidate.ContestID),
		zap.String("from", result.FromStatus.String()),
		zap.String("to", result.ToStatus.String()))
}

// CheckRegistrationCapacity checks if a contest is at capacity.
func CheckRegistrationCapacity(ctx context.Context, pool *db.Pool, contestID string) (bool, error) {
	var currentParticipants int
	var maxParticipants sql.NullInt64

	err := pool.Replica().QueryRowContext(ctx, `
		SELECT current_participants, max_participants
		FROM contests
		WHERE id = $1
	`, contestID).Scan(&currentParticipants, &maxParticipants)

	if err != nil {
		if err == sql.ErrNoRows {
			return false, ErrContestNotFound
		}
		return false, fmt.Errorf("failed to check capacity: %w", err)
	}

	if !maxParticipants.Valid {
		return false, nil // No limit
	}

	return currentParticipants >= int(maxParticipants.Int64), nil
}

// ValidateRegistration checks if a user can register for a contest.
func ValidateRegistration(ctx context.Context, pool *db.Pool, contestID, userID string) error {
	var status string
	var currentParticipants int
	var maxParticipants sql.NullInt64

	err := pool.Replica().QueryRowContext(ctx, `
		SELECT status, current_participants, max_participants
		FROM contests
		WHERE id = $1
	`, contestID).Scan(&status, &currentParticipants, &maxParticipants)

	if err != nil {
		if err == sql.ErrNoRows {
			return ErrContestNotFound
		}
		return fmt.Errorf("failed to get contest: %w", err)
	}

	contestStatus := ContestStatus(status)

	// Check if registration is allowed
	if !contestStatus.AllowsRegistration() {
		return ErrRegistrationClosed
	}

	// Check capacity
	if maxParticipants.Valid && currentParticipants >= int(maxParticipants.Int64) {
		return ErrMaxParticipants
	}

	// Check if user is already registered
	var exists bool
	err = pool.Replica().QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM contest_participants
			WHERE contest_id = $1 AND user_id = $2
		)
	`, contestID, userID).Scan(&exists)

	if err != nil {
		return fmt.Errorf("failed to check registration: %w", err)
	}

	if exists {
		return errors.New("user already registered for this contest")
	}

	return nil
}
