// Package statemachine provides contest lifecycle state machine functionality.
package statemachine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/IBM/sarama"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/scoring/prize"
	"github.com/Parsaeffatravesh/tragge/packages/scoring"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ContestHandlers provides detailed handlers for contest lifecycle events.
type ContestHandlers struct {
	pool          *db.Pool
	kafkaProducer sarama.SyncProducer
	logger        *zap.Logger
	config        *HandlersConfig
	redisClient   redis.UniversalClient
}

// SetRedisClient sets the Redis client for fetching live market prices.
func (h *ContestHandlers) SetRedisClient(client redis.UniversalClient) {
	h.redisClient = client
}

// HandlersConfig holds configuration for contest handlers.
type HandlersConfig struct {
	// Kafka topics
	ContestEventsTopic   string
	NotificationsTopic   string
	ClosePositionsTopic  string
	CancelOrdersTopic    string
	LeaderboardInitTopic string
	PnLDeltasTopic       string

	// Default prize distribution (top 30% of participants)
	WinnerPercentage float64
}

// DefaultHandlersConfig returns default configuration.
func DefaultHandlersConfig() *HandlersConfig {
	return &HandlersConfig{
		ContestEventsTopic:   "contests.v1",
		NotificationsTopic:   "notifications.v1",
		ClosePositionsTopic:  "close_positions.v1",
		CancelOrdersTopic:    "cancel_orders.v1",
		LeaderboardInitTopic: "leaderboard_init.v1",
		PnLDeltasTopic:       "pnl.v1",
		WinnerPercentage:     0.30,
	}
}

// NewContestHandlers creates a new ContestHandlers instance.
func NewContestHandlers(pool *db.Pool, kafkaProducer sarama.SyncProducer, logger *zap.Logger, config *HandlersConfig) *ContestHandlers {
	if config == nil {
		config = DefaultHandlersConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if kafkaProducer == nil {
		logger.Warn("ContestHandlers created with nil Kafka producer - contest events will not be published")
	}
	return &ContestHandlers{
		pool:          pool,
		kafkaProducer: kafkaProducer,
		logger:        logger,
		config:        config,
	}
}

// ============================================================================
// CONTEST START HANDLER (OnRunning)
// ============================================================================

// HandleContestStart handles all side effects when a contest transitions to RUNNING.
func (h *ContestHandlers) HandleContestStart(ctx context.Context, result *TransitionResult) error {
	if h.pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	contestID := result.Contest.ID
	startTime := time.Now()

	h.logger.Info("Handling contest start",
		zap.String("contest_id", contestID),
		zap.String("name", result.Contest.Name),
		zap.Int("participants", result.Contest.CurrentParticipants))

	// 1. Initialize participant trading states
	if err := h.initializeParticipantStates(ctx, result.Contest); err != nil {
		h.logger.Error("Failed to initialize participant states",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return fmt.Errorf("initialize participant states: %w", err)
	}

	// 2. Lock prizes based on final participant count
	if err := h.lockPrizes(ctx, result.Contest); err != nil {
		h.logger.Error("Failed to lock prizes",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return fmt.Errorf("lock prizes: %w", err)
	}

	// 3. Enable trading for the contest (via Kafka event)
	if err := h.enableContestTrading(ctx, result.Contest); err != nil {
		h.logger.Error("Failed to enable contest trading",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Non-fatal: continue with other operations
	}

	// 4. Initialize leaderboard in Redis
	if err := h.initializeLeaderboard(ctx, result.Contest); err != nil {
		h.logger.Error("Failed to initialize leaderboard",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Non-fatal: leaderboard-worker will handle missing entries
	}

	// 5. Send notifications to all participants
	if err := h.notifyContestStart(ctx, result.Contest); err != nil {
		h.logger.Error("Failed to send contest start notifications",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Non-fatal: notifications are best-effort
	}

	// 6. Broadcast WebSocket event to all connected clients
	if err := h.broadcastContestEvent(ctx, result.Contest, contracts.ContestEventStarted,
		fmt.Sprintf("Contest %s has started! Good luck!", result.Contest.Name)); err != nil {
		h.logger.Error("Failed to broadcast contest start event",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Non-fatal: websocket broadcast is best-effort
	}

	h.logger.Info("Contest start handling completed",
		zap.String("contest_id", contestID),
		zap.Duration("duration", time.Since(startTime)))

	return nil
}

// initializeParticipantStates sets up trading state for all participants.
func (h *ContestHandlers) initializeParticipantStates(ctx context.Context, contest *Contest) error {
	// Begin transaction for atomic initialization
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update all participants with initial QTY allocation
	result, err := tx.ExecContext(ctx, `
		UPDATE contest_participants
		SET
			qty_total = $1,
			qty_available = $1,
			total_score = 0,
			final_rank = NULL,
			final_prize_cents = NULL
		WHERE contest_id = $2
	`, contest.QtyTotal, contest.ID)
	if err != nil {
		return fmt.Errorf("update participants: %w", err)
	}

	affected, _ := result.RowsAffected()

	// Clear any pre-existing positions (safety measure)
	_, err = tx.ExecContext(ctx, `
		UPDATE positions
		SET closed_at = NOW()
		WHERE contest_id = $1 AND closed_at IS NULL
	`, contest.ID)
	if err != nil {
		h.logger.Warn("Failed to clear pre-existing positions",
			zap.String("contest_id", contest.ID),
			zap.Error(err))
	}

	// Clear any pre-existing pending orders (safety measure)
	_, err = tx.ExecContext(ctx, `
		UPDATE orders
		SET status = 'cancelled', updated_at = NOW()
		WHERE contest_id = $1 AND status IN ('pending', 'open')
	`, contest.ID)
	if err != nil {
		h.logger.Warn("Failed to clear pre-existing pending orders",
			zap.String("contest_id", contest.ID),
			zap.Error(err))
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	h.logger.Info("Initialized participant trading states",
		zap.String("contest_id", contest.ID),
		zap.Int64("participants", affected),
		zap.Int64("qty_total", contest.QtyTotal))

	return nil
}

// lockPrizes calculates and persists the prize distribution at contest start.
// Once locked, prizes are read from the contest_prize_locks table rather than
// being recalculated dynamically. This guarantees participants see the same
// prize breakdown throughout the contest and settlement uses the same numbers.
func (h *ContestHandlers) lockPrizes(ctx context.Context, contest *Contest) error {
	participants := contest.CurrentParticipants
	if participants <= 0 {
		h.logger.Info("Skipping prize lock: no participants",
			zap.String("contest_id", contest.ID))
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

	// Serialize the distribution for storage
	distributionJSON, err := json.Marshal(slots)
	if err != nil {
		return fmt.Errorf("marshal distribution: %w", err)
	}

	// Persist inside a transaction
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert the prize lock row
	_, err = tx.ExecContext(ctx, `
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

	// Update the contests row with lock timestamp and net pool
	_, err = tx.ExecContext(ctx, `
		UPDATE contests
		SET prizes_locked_at = NOW(),
		    prize_pool_net_cents = $1
		WHERE id = $2
	`, netPool, contest.ID)
	if err != nil {
		return fmt.Errorf("update contests prize lock: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit prize lock: %w", err)
	}

	h.logger.Info("Prizes locked for contest",
		zap.String("contest_id", contest.ID),
		zap.Int("participants", participants),
		zap.Int64("gross_pool_cents", grossPool),
		zap.Int64("net_pool_cents", netPool),
		zap.Int64("platform_fee_cents", platformFee),
		zap.Int("winners", winnersCount))

	// Publish prize_locked Kafka event (best-effort)
	if err := h.publishPrizeLocked(ctx, contest, participants, grossPool, netPool, platformFee, winnersCount, slots); err != nil {
		h.logger.Error("Failed to publish prize_locked event",
			zap.String("contest_id", contest.ID),
			zap.Error(err))
		// Non-fatal: the lock is persisted in DB regardless
	}

	return nil
}

// publishPrizeLocked publishes a prize_locked event to Kafka for WebSocket broadcast.
func (h *ContestHandlers) publishPrizeLocked(ctx context.Context, contest *Contest, participants int, grossPool, netPool, platformFee int64, winnersCount int, slots []prize.PrizeSlot) error {
	if h.kafkaProducer == nil {
		return nil
	}

	// Build top-3 summary for the broadcast
	top3 := make([]contracts.RankPrizeBrief, 0, 3)
	for i := 0; i < len(slots) && i < 3; i++ {
		top3 = append(top3, contracts.RankPrizeBrief{
			Rank:        slots[i].Rank,
			AmountCents: slots[i].AmountCents,
		})
	}

	var firstPrizeCents int64
	if len(slots) > 0 {
		firstPrizeCents = slots[0].AmountCents
	}

	event := contracts.ContestEvent{
		Type:      contracts.ContestEventPrizeLocked,
		ContestID: contest.ID,
		Name:      contest.Name,
		EndsAt:    contest.EndsAt.UnixMilli(),
		Message:   "Prize pool has been locked",
		Metadata: map[string]any{
			"total_participants":     participants,
			"prize_pool_gross_cents": grossPool,
			"prize_pool_net_cents":   netPool,
			"platform_fee_cents":     platformFee,
			"winners_count":          winnersCount,
			"first_prize_cents":      firstPrizeCents,
			"top_3_prizes":           top3,
		},
		Ts: time.Now().UnixMilli(),
	}

	return h.publishEvent(h.config.ContestEventsTopic, contest.ID, event)
}

// enableContestTrading publishes an event to enable trading for the contest.
func (h *ContestHandlers) enableContestTrading(ctx context.Context, contest *Contest) error {
	if h.kafkaProducer == nil {
		return nil
	}

	event := contracts.ContestEvent{
		Type:      contracts.ContestEventStarted,
		ContestID: contest.ID,
		Name:      contest.Name,
		EndsAt:    contest.EndsAt.UnixMilli(),
		Message:   "Trading is now enabled",
		Metadata: map[string]any{
			"starts_at":    contest.StartsAt.UnixMilli(),
			"participants": contest.CurrentParticipants,
			"qty_total":    contest.QtyTotal,
		},
		Ts: time.Now().UnixMilli(),
	}

	return h.publishEvent(h.config.ContestEventsTopic, contest.ID, event)
}

// initializeLeaderboard initializes the leaderboard for the contest.
func (h *ContestHandlers) initializeLeaderboard(ctx context.Context, contest *Contest) error {
	if h.kafkaProducer == nil {
		return nil
	}

	// Get all participants
	rows, err := h.pool.Replica().QueryContext(ctx, `
		SELECT user_id FROM contest_participants WHERE contest_id = $1
	`, contest.ID)
	if err != nil {
		return fmt.Errorf("query participants: %w", err)
	}
	defer rows.Close()

	var participants []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			continue
		}
		participants = append(participants, userID)
	}

	// Publish leaderboard initialization event
	event := map[string]any{
		"type":          "init",
		"contest_id":    contest.ID,
		"participants":  participants,
		"initial_score": 0,
		"ts":            time.Now().UnixMilli(),
	}

	return h.publishEvent(h.config.LeaderboardInitTopic, contest.ID, event)
}

// notifyContestStart sends notifications to all participants.
func (h *ContestHandlers) notifyContestStart(ctx context.Context, contest *Contest) error {
	notification := contracts.ContestNotification{
		Type:      contracts.NotificationContestStarted,
		ContestID: contest.ID,
		Title:     fmt.Sprintf("Contest Started: %s", contest.Name),
		Body:      fmt.Sprintf("Contest %s has started! Good luck trading!", contest.Name),
		Data: map[string]any{
			"contest_name": contest.Name,
			"ends_at":      contest.EndsAt.UnixMilli(),
			"qty_total":    contest.QtyTotal,
		},
		Channels: []contracts.NotificationChannel{
			contracts.ChannelPush,
			contracts.ChannelInApp,
		},
		Priority: contracts.PriorityHigh,
		Ts:       time.Now().UnixMilli(),
	}

	return h.publishEvent(h.config.NotificationsTopic, contest.ID, notification)
}

// broadcastContestEvent broadcasts a contest event for WebSocket delivery.
func (h *ContestHandlers) broadcastContestEvent(ctx context.Context, contest *Contest, eventType contracts.ContestEventType, message string) error {
	if h.kafkaProducer == nil {
		return nil
	}

	event := contracts.ContestEvent{
		Type:      eventType,
		ContestID: contest.ID,
		Name:      contest.Name,
		EndsAt:    contest.EndsAt.UnixMilli(),
		Message:   message,
		Ts:        time.Now().UnixMilli(),
	}

	return h.publishEvent(h.config.ContestEventsTopic, contest.ID, event)
}

// ============================================================================
// CONTEST END HANDLER (OnSettling)
// ============================================================================

// HandleContestEnd handles all side effects when a contest transitions to SETTLING.
func (h *ContestHandlers) HandleContestEnd(ctx context.Context, result *TransitionResult) error {
	if h.pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	contestID := result.Contest.ID
	startTime := time.Now()

	h.logger.Info("Handling contest end",
		zap.String("contest_id", contestID),
		zap.String("name", result.Contest.Name))

	// 1. Stop accepting new trades
	if err := h.stopContestTrading(ctx, result.Contest); err != nil {
		h.logger.Error("Failed to stop contest trading",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Continue: critical operation, but we'll handle positions anyway
	}

	// 2. Cancel all pending orders (limit/stop orders)
	cancelledCount, err := h.cancelAllPendingOrders(ctx, contestID)
	if err != nil {
		h.logger.Error("Failed to cancel pending orders",
			zap.String("contest_id", contestID),
			zap.Error(err))
	} else {
		h.logger.Info("Cancelled pending orders",
			zap.String("contest_id", contestID),
			zap.Int64("count", cancelledCount))
	}

	// 3. Force close all open positions at same timestamp for fairness
	closedCount, err := h.forceCloseAllPositions(ctx, contestID)
	if err != nil {
		h.logger.Error("Failed to close positions",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return fmt.Errorf("force close positions: %w", err)
	}
	h.logger.Info("Closed all positions",
		zap.String("contest_id", contestID),
		zap.Int64("count", closedCount))

	// 4. Calculate final rankings
	rankings, err := h.calculateFinalRankings(ctx, contestID)
	if err != nil {
		h.logger.Error("Failed to calculate rankings",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return fmt.Errorf("calculate rankings: %w", err)
	}
	h.logger.Info("Calculated final rankings",
		zap.String("contest_id", contestID),
		zap.Int("count", len(rankings)))

	// 5. Update leaderboard with final scores
	if err := h.updateFinalLeaderboard(ctx, contestID, rankings); err != nil {
		h.logger.Error("Failed to update final leaderboard",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	// 6. Notify users that trading has ended
	if err := h.notifyTradingEnd(ctx, result.Contest); err != nil {
		h.logger.Error("Failed to send trading end notifications",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	// 7. Broadcast WebSocket event
	if err := h.broadcastContestEvent(ctx, result.Contest, contracts.ContestEventTradingEnded,
		"Trading has ended. Calculating final results..."); err != nil {
		h.logger.Error("Failed to broadcast trading end event",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	h.logger.Info("Contest end handling completed",
		zap.String("contest_id", contestID),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int64("positions_closed", closedCount),
		zap.Int64("orders_cancelled", cancelledCount))

	return nil
}

// stopContestTrading publishes an event to stop trading for the contest.
func (h *ContestHandlers) stopContestTrading(ctx context.Context, contest *Contest) error {
	if h.kafkaProducer == nil {
		return nil
	}

	event := contracts.ContestEvent{
		Type:      contracts.ContestEventTradingEnded,
		ContestID: contest.ID,
		Name:      contest.Name,
		Message:   "Trading has been disabled",
		Metadata: map[string]any{
			"ended_at": time.Now().UnixMilli(),
		},
		Ts: time.Now().UnixMilli(),
	}

	return h.publishEvent(h.config.ContestEventsTopic, contest.ID, event)
}

// cancelAllPendingOrders cancels all pending orders and returns reserved QTY.
func (h *ContestHandlers) cancelAllPendingOrders(ctx context.Context, contestID string) (int64, error) {
	// Begin transaction
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get pending orders to return QTY
	rows, err := tx.QueryContext(ctx, `
		SELECT order_id, user_id, qty
		FROM orders
		WHERE contest_id = $1 AND status IN ('pending', 'open')
	`, contestID)
	if err != nil {
		return 0, fmt.Errorf("query pending orders: %w", err)
	}

	type pendingOrder struct {
		OrderID string
		UserID  string
		Qty     int64
	}
	var orders []pendingOrder
	for rows.Next() {
		var o pendingOrder
		if err := rows.Scan(&o.OrderID, &o.UserID, &o.Qty); err != nil {
			continue
		}
		orders = append(orders, o)
	}
	rows.Close()

	// Cancel all pending orders
	result, err := tx.ExecContext(ctx, `
		UPDATE orders
		SET status = 'cancelled', updated_at = NOW()
		WHERE contest_id = $1 AND status IN ('pending', 'open')
	`, contestID)
	if err != nil {
		return 0, fmt.Errorf("cancel orders: %w", err)
	}
	affected, _ := result.RowsAffected()

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
			h.logger.Warn("Failed to return QTY to user",
				zap.String("contest_id", contestID),
				zap.String("user_id", userID),
				zap.Int64("qty", qty),
				zap.Error(err))
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	// Publish cancellation events via Kafka
	if h.kafkaProducer != nil {
		cancelRequest := contracts.CancelAllOrdersRequest{
			ContestID: contestID,
			Reason:    contracts.CancelReasonContestEnded,
			Ts:        time.Now().UnixMilli(),
		}
		h.publishEvent(h.config.CancelOrdersTopic, contestID, cancelRequest)
	}

	return affected, nil
}

// getLatestFillPrices returns the most recent fill price per symbol for a contest.
// This is used as the best available exit price when closing positions during settlement.
func getLatestFillPrices(ctx context.Context, querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}, contestID string) (map[string]float64, error) {
	rows, err := querier.QueryContext(ctx, `
		SELECT DISTINCT ON (symbol) symbol, fill_price
		FROM fills
		WHERE contest_id = $1
		ORDER BY symbol, created_at DESC
	`, contestID)
	if err != nil {
		return nil, fmt.Errorf("query latest fill prices: %w", err)
	}
	defer rows.Close()

	prices := make(map[string]float64)
	for rows.Next() {
		var symbol string
		var price float64
		if err := rows.Scan(&symbol, &price); err != nil {
			continue
		}
		prices[symbol] = price
	}
	return prices, rows.Err()
}

// MarketPrice represents a live market price from Redis.
type MarketPrice struct {
	Bid  float64 `json:"bid"`
	Ask  float64 `json:"ask"`
	Last float64 `json:"last"`
	Ts   int64   `json:"ts"`
}

// getMarketPricesFromRedis fetches live market prices from Redis for the given symbols.
// Prices are stored by market-ingestor at hash key "prices:latest" with symbol keys.
func getMarketPricesFromRedis(ctx context.Context, client redis.UniversalClient, symbols []string) (map[string]MarketPrice, error) {
	if client == nil || len(symbols) == 0 {
		return nil, nil
	}

	// Deduplicate symbols
	uniqueSymbols := make([]string, 0, len(symbols))
	seen := make(map[string]bool, len(symbols))
	for _, s := range symbols {
		if !seen[s] {
			seen[s] = true
			uniqueSymbols = append(uniqueSymbols, s)
		}
	}

	args := make([]string, len(uniqueSymbols))
	copy(args, uniqueSymbols)
	vals, err := client.HMGet(ctx, "prices:latest", args...).Result()
	if err != nil {
		return nil, fmt.Errorf("redis HMGet prices:latest: %w", err)
	}

	prices := make(map[string]MarketPrice)
	for i, val := range vals {
		if val == nil {
			continue
		}
		str, ok := val.(string)
		if !ok {
			continue
		}
		var mp MarketPrice
		if err := json.Unmarshal([]byte(str), &mp); err != nil {
			continue
		}
		prices[uniqueSymbols[i]] = mp
	}
	return prices, nil
}

// getExitPriceFromMarket returns the appropriate exit price from a MarketPrice based on position side.
// Long positions exit at Bid (selling), Short positions exit at Ask (buying).
func getExitPriceFromMarket(mp MarketPrice, positionSide string) float64 {
	if positionSide == "long" {
		return mp.Bid
	}
	return mp.Ask
}

// positionSideToOrderSide converts a DB position side ("long"/"short") to a contracts OrderSide.
func positionSideToOrderSide(side string) contracts.OrderSide {
	if side == "long" {
		return contracts.OrderSideBuy
	}
	return contracts.OrderSideSell
}

// forceCloseAllPositions closes all open positions at the latest market price,
// calculates realized P&L scores, and updates participant totals.
func (h *ContestHandlers) forceCloseAllPositions(ctx context.Context, contestID string) (int64, error) {
	closeTime := time.Now()

	// Begin transaction
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get all open positions (locked for update)
	rows, err := tx.QueryContext(ctx, `
		SELECT position_id, user_id, symbol, side, qty_open, entry_price, qty_used, realized_score
		FROM positions
		WHERE contest_id = $1 AND closed_at IS NULL
		FOR UPDATE
	`, contestID)
	if err != nil {
		return 0, fmt.Errorf("query open positions: %w", err)
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
		return 0, nil
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
	if h.redisClient != nil {
		var redisErr error
		marketPrices, redisErr = getMarketPricesFromRedis(ctx, h.redisClient, symbols)
		if redisErr != nil {
			h.logger.Warn("Failed to fetch market prices from Redis, falling back to fill prices",
				zap.String("contest_id", contestID),
				zap.Error(redisErr))
		}
	}

	// Fetch fill prices as fallback
	fillPrices, err := getLatestFillPrices(ctx, tx, contestID)
	if err != nil {
		h.logger.Warn("Failed to fetch fill prices, will use entry prices as fallback",
			zap.String("contest_id", contestID),
			zap.Error(err))
		fillPrices = make(map[string]float64)
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
			h.logger.Warn("No market or fill price available, using entry price as exit (break-even)",
				zap.String("position_id", pos.PositionID),
				zap.String("symbol", pos.Symbol))
		}
		if priceSource != "market_price" {
			h.logger.Info("Position exit price source",
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
			return 0, fmt.Errorf("close position %s: %w", pos.PositionID, err)
		}

		// Return qty_used and add score to participant
		_, err = tx.ExecContext(ctx, `
			UPDATE contest_participants
			SET qty_available = qty_available + $1, total_score = total_score + $2
			WHERE contest_id = $3 AND user_id = $4
		`, pos.QtyUsed, scoreDeltaF64, contestID, pos.UserID)
		if err != nil {
			h.logger.Warn("Failed to update participant after position close",
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
				TotalScore:             0, // Calculated by leaderboard-worker
				Ts:                     closeTime.UnixMilli(),
				DeltaScoreDecimal:      scoring.ToString(scoreDelta),
				RealizedScoreDecimal:   scoring.ToString(scoreDelta),
				UnrealizedScoreDecimal: "0.00000000",
				TotalScoreDecimal:      "0.00000000",
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit transaction: %w", err)
	}

	// Publish events after successful commit
	if h.kafkaProducer != nil {
		for _, evt := range deferredEvents {
			// Publish PositionClosedEvent
			eventJSON, err := json.Marshal(evt.positionClosed)
			if err != nil {
				h.logger.Warn("Failed to marshal position closed event", zap.Error(err))
				continue
			}
			msg := &sarama.ProducerMessage{
				Topic: h.config.ClosePositionsTopic,
				Key:   sarama.StringEncoder(contestID),
				Value: sarama.ByteEncoder(eventJSON),
			}
			h.kafkaProducer.SendMessage(msg)

			// Publish PnLDelta
			pnlJSON, err := json.Marshal(evt.pnlDelta)
			if err != nil {
				h.logger.Warn("Failed to marshal PnL delta event", zap.Error(err))
				continue
			}
			pnlMsg := &sarama.ProducerMessage{
				Topic: h.config.PnLDeltasTopic,
				Key:   sarama.StringEncoder(contestID),
				Value: sarama.ByteEncoder(pnlJSON),
			}
			h.kafkaProducer.SendMessage(pnlMsg)
		}
	}

	return int64(len(positions)), nil
}

// calculateFinalRankings calculates and stores final rankings for all participants.
func (h *ContestHandlers) calculateFinalRankings(ctx context.Context, contestID string) ([]contracts.FinalRanking, error) {
	// Get all participants with their scores, sorted by total_score descending
	rows, err := h.pool.Replica().QueryContext(ctx, `
		SELECT user_id, total_score
		FROM contest_participants
		WHERE contest_id = $1
		ORDER BY total_score DESC, joined_at ASC
	`, contestID)
	if err != nil {
		return nil, fmt.Errorf("query participants: %w", err)
	}
	defer rows.Close()

	var rankings []contracts.FinalRanking
	rank := 1
	for rows.Next() {
		var r contracts.FinalRanking
		if err := rows.Scan(&r.UserID, &r.TotalScore); err != nil {
			continue
		}
		r.Rank = rank
		rankings = append(rankings, r)
		rank++
	}

	// Handle ties: participants with the same score get the same rank
	for i := 1; i < len(rankings); i++ {
		if rankings[i].TotalScore == rankings[i-1].TotalScore {
			rankings[i].Rank = rankings[i-1].Rank
		}
	}

	// Store rankings in database
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, r := range rankings {
		_, err := tx.ExecContext(ctx, `
			UPDATE contest_participants
			SET final_rank = $1, total_score = $2
			WHERE contest_id = $3 AND user_id = $4
		`, r.Rank, r.TotalScore, contestID, r.UserID)
		if err != nil {
			h.logger.Warn("Failed to update participant rank",
				zap.String("user_id", r.UserID),
				zap.Error(err))
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return rankings, nil
}

// updateFinalLeaderboard marks the leaderboard as final.
func (h *ContestHandlers) updateFinalLeaderboard(ctx context.Context, contestID string, rankings []contracts.FinalRanking) error {
	// Store final snapshot in database
	snapshotData, _ := json.Marshal(map[string]any{
		"type":     "final",
		"rankings": rankings,
		"ts":       time.Now().UnixMilli(),
	})

	_, err := h.pool.Primary().ExecContext(ctx, `
		INSERT INTO leaderboard_snapshots (contest_id, taken_at, payload_json)
		VALUES ($1, NOW(), $2)
	`, contestID, snapshotData)

	return err
}

// notifyTradingEnd sends notifications that trading has ended.
func (h *ContestHandlers) notifyTradingEnd(ctx context.Context, contest *Contest) error {
	notification := contracts.ContestNotification{
		Type:      contracts.NotificationTradingEnded,
		ContestID: contest.ID,
		Title:     "Trading Has Ended",
		Body:      fmt.Sprintf("Trading in %s has ended. Results are being calculated...", contest.Name),
		Data: map[string]any{
			"contest_name": contest.Name,
		},
		Channels: []contracts.NotificationChannel{
			contracts.ChannelPush,
			contracts.ChannelInApp,
		},
		Priority: contracts.PriorityHigh,
		Ts:       time.Now().UnixMilli(),
	}

	return h.publishEvent(h.config.NotificationsTopic, contest.ID, notification)
}

// ============================================================================
// SETTLEMENT HANDLER (OnCompleted)
// ============================================================================

// HandleSettlement handles all side effects when a contest transitions to COMPLETED.
// It reads prizes from the contest_prize_locks table, handles tied ranks,
// credits wallets atomically, and retries any failed credits.
func (h *ContestHandlers) HandleSettlement(ctx context.Context, result *TransitionResult) error {
	if h.pool == nil {
		return fmt.Errorf("database pool is nil")
	}
	contestID := result.Contest.ID
	startTime := time.Now()

	h.logger.Info("Handling settlement",
		zap.String("contest_id", contestID),
		zap.String("name", result.Contest.Name))

	// 1. Create or resume settlement record
	settlementID, err := h.getOrCreateSettlement(ctx, contestID, result.Contest.CurrentParticipants)
	if err != nil {
		h.logger.Error("Failed to create settlement record",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return fmt.Errorf("create settlement record: %w", err)
	}

	// 2. Log settlement start
	h.logSettlementEvent(ctx, settlementID, contestID, "settlement_started", map[string]any{
		"participants": result.Contest.CurrentParticipants,
		"started_at":   startTime.UnixMilli(),
	}, nil)

	// 3. Get final rankings (should already be calculated in settling phase)
	rankings, err := h.getFinalRankings(ctx, contestID)
	if err != nil {
		h.logger.Error("Failed to get final rankings",
			zap.String("contest_id", contestID),
			zap.Error(err))
		h.logSettlementEvent(ctx, settlementID, contestID, "settlement_failed", nil,
			strPtr(fmt.Sprintf("get final rankings: %v", err)))
		return fmt.Errorf("get final rankings: %w", err)
	}

	// 4. Calculate prize distribution (from locked table or fallback, with tie handling)
	prizes, err := h.calculatePrizeDistribution(ctx, result.Contest, rankings)
	if err != nil {
		h.logger.Error("Failed to calculate prizes",
			zap.String("contest_id", contestID),
			zap.Error(err))
		h.logSettlementEvent(ctx, settlementID, contestID, "settlement_failed", nil,
			strPtr(fmt.Sprintf("calculate prizes: %v", err)))
		return fmt.Errorf("calculate prizes: %w", err)
	}

	// 5. Log prizes calculated
	h.logSettlementEvent(ctx, settlementID, contestID, "prizes_calculated", map[string]any{
		"total_winners":  len(prizes),
		"total_rankings": len(rankings),
	}, nil)

	// 6. Distribute prizes to winners (per-winner transactions)
	distributedCount, failedAllocations, err := h.distributePrizes(ctx, contestID, settlementID, prizes)
	if err != nil {
		h.logger.Error("Failed to distribute prizes",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	// 7. Retry failed credits (one retry pass)
	if len(failedAllocations) > 0 {
		h.logger.Info("Retrying failed prize credits",
			zap.String("contest_id", contestID),
			zap.Int("failed_count", len(failedAllocations)))

		h.logSettlementEvent(ctx, settlementID, contestID, "retry_started", map[string]any{
			"failed_count": len(failedAllocations),
		}, nil)

		retryCount, stillFailed, _ := h.distributePrizes(ctx, contestID, settlementID, failedAllocations)
		distributedCount += retryCount
		failedAllocations = stillFailed

		if len(stillFailed) > 0 {
			h.logger.Warn("Some prizes still failed after retry",
				zap.String("contest_id", contestID),
				zap.Int("still_failed", len(stillFailed)))
		}
	}

	// 8. Mark settlement complete (or partial if some credits failed)
	settlementStatus := "completed"
	if len(failedAllocations) > 0 {
		settlementStatus = "partial"
	}
	h.completeSettlement(ctx, settlementID, settlementStatus, distributedCount)

	// 9. Log settlement completed
	h.logSettlementEvent(ctx, settlementID, contestID, "settlement_completed", map[string]any{
		"distributed_count": distributedCount,
		"failed_count":      len(failedAllocations),
		"status":            settlementStatus,
		"duration_ms":       time.Since(startTime).Milliseconds(),
	}, nil)

	// 10. Update T-Points (global leaderboard)
	if err := h.updateTraggePoints(ctx, contestID, rankings); err != nil {
		h.logger.Error("Failed to update T-Points",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Non-fatal: T-Points can be recalculated
	}

	// 11. Send results notifications
	if err := h.notifyResults(ctx, result.Contest, rankings, prizes); err != nil {
		h.logger.Error("Failed to send results notifications",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	// 12. Archive contest data
	if err := h.archiveContest(ctx, contestID); err != nil {
		h.logger.Error("Failed to archive contest",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	// 13. Broadcast completion event
	if err := h.broadcastContestEvent(ctx, result.Contest, contracts.ContestEventCompleted,
		"Contest has been completed. Check your results!"); err != nil {
		h.logger.Error("Failed to broadcast completion event",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	h.logger.Info("Settlement completed",
		zap.String("contest_id", contestID),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("participants", len(rankings)),
		zap.Int("prizes_distributed", distributedCount),
		zap.Int("prizes_failed", len(failedAllocations)))

	return nil
}

// getFinalRankings retrieves the final rankings from the database.
func (h *ContestHandlers) getFinalRankings(ctx context.Context, contestID string) ([]contracts.FinalRanking, error) {
	rows, err := h.pool.Replica().QueryContext(ctx, `
		SELECT user_id, final_rank, total_score, COALESCE(final_prize_cents, 0)
		FROM contest_participants
		WHERE contest_id = $1 AND final_rank IS NOT NULL
		ORDER BY final_rank ASC
	`, contestID)
	if err != nil {
		return nil, fmt.Errorf("query rankings: %w", err)
	}
	defer rows.Close()

	var rankings []contracts.FinalRanking
	for rows.Next() {
		var r contracts.FinalRanking
		if err := rows.Scan(&r.UserID, &r.Rank, &r.TotalScore, &r.PrizeCents); err != nil {
			continue
		}
		rankings = append(rankings, r)
	}

	return rankings, nil
}

// calculatePrizeDistribution calculates prizes for winners using the locked
// prize table when available, falling back to dynamic calculation for backward
// compatibility. Tied ranks are handled by combining the prize amounts for the
// positions the tied participants occupy and splitting equally.
func (h *ContestHandlers) calculatePrizeDistribution(ctx context.Context, contest *Contest, rankings []contracts.FinalRanking) ([]contracts.PrizeAllocation, error) {
	if len(rankings) == 0 {
		return nil, nil
	}

	var slots []prize.PrizeSlot
	var netPool int64

	// Try to read from contest_prize_locks (locked at contest start)
	var distributionJSON []byte
	err := h.pool.Replica().QueryRowContext(ctx,
		`SELECT prize_pool_net_cents, distribution_json
		 FROM contest_prize_locks
		 WHERE contest_id = $1`,
		contest.ID,
	).Scan(&netPool, &distributionJSON)

	if err == nil && len(distributionJSON) > 0 {
		// Locked distribution exists — use it
		if err := json.Unmarshal(distributionJSON, &slots); err != nil {
			return nil, fmt.Errorf("unmarshal locked distribution: %w", err)
		}
		h.logger.Info("Using locked prize distribution",
			zap.String("contest_id", contest.ID),
			zap.Int64("net_pool_cents", netPool),
			zap.Int("slots", len(slots)))
	} else {
		// Fallback: calculate dynamically (backward compatibility)
		commissionFraction := prize.MustCommissionPercentToFraction(contest.CommissionRate)
		var poolErr error
		netPool, poolErr = prize.CalculatePrizePool(len(rankings), contest.EntryFeeCents, commissionFraction)
		if poolErr != nil {
			return nil, fmt.Errorf("calculate prize pool: %w", poolErr)
		}
		slots = prize.CalculatePrizeDistribution(len(rankings), netPool)
		h.logger.Info("Using calculated prize distribution (no lock found)",
			zap.String("contest_id", contest.ID),
			zap.Int64("net_pool_cents", netPool),
			zap.Int("slots", len(slots)))
	}

	if len(slots) == 0 {
		return nil, nil
	}

	// Build position-to-prize map (1-indexed position -> prize amount)
	positionPrize := make(map[int]int64)
	positionPct := make(map[int]float64)
	for _, s := range slots {
		positionPrize[s.Rank] = s.AmountCents
		positionPct[s.Rank] = s.Percentage
	}
	winnersCount := len(slots)

	// Group rankings by rank to handle ties
	type tieGroup struct {
		rank  int
		users []contracts.FinalRanking
	}
	groupMap := make(map[int]*tieGroup)
	var groupOrder []int
	for _, r := range rankings {
		if g, ok := groupMap[r.Rank]; ok {
			g.users = append(g.users, r)
		} else {
			groupMap[r.Rank] = &tieGroup{rank: r.Rank, users: []contracts.FinalRanking{r}}
			groupOrder = append(groupOrder, r.Rank)
		}
	}
	sort.Ints(groupOrder)

	// Calculate prizes with tie handling.
	// For tied ranks, combine the prize amounts for the sequential positions
	// they occupy and split equally among the tied participants.
	var allocations []contracts.PrizeAllocation
	position := 1 // sequential position for prize slot lookup

	for _, rank := range groupOrder {
		group := groupMap[rank]
		count := len(group.users)

		// Sum up prizes for all positions this tie group occupies
		var combinedPrize int64
		var combinedPct float64
		for i := 0; i < count; i++ {
			pos := position + i
			if pos <= winnersCount {
				combinedPrize += positionPrize[pos]
				combinedPct += positionPct[pos]
			}
		}

		if combinedPrize > 0 {
			// Split equally among tied participants
			eachCents := combinedPrize / int64(count)
			eachPct := combinedPct / float64(count)
			leftover := combinedPrize - (eachCents * int64(count))

			for i, user := range group.users {
				amount := eachCents
				if i == 0 && leftover > 0 {
					amount += leftover // first user in tie group gets leftover cents
				}
				allocations = append(allocations, contracts.PrizeAllocation{
					UserID:      user.UserID,
					Rank:        rank,
					AmountCents: amount,
					Percentage:  eachPct,
					Status:      "pending",
				})
			}
		}

		position += count
	}

	return allocations, nil
}

// distributePrizes credits prizes to winner wallets. Each prize is credited in
// its own transaction so that one failure does not rollback others. Failed
// allocations are returned so the caller can retry them.
func (h *ContestHandlers) distributePrizes(ctx context.Context, contestID, settlementID string, prizes []contracts.PrizeAllocation) (int, []contracts.PrizeAllocation, error) {
	distributedCount := 0
	var failedAllocations []contracts.PrizeAllocation

	for i := range prizes {
		p := &prizes[i]
		if p.AmountCents <= 0 {
			continue
		}

		if err := h.creditSinglePrize(ctx, contestID, settlementID, p); err != nil {
			h.logger.Warn("Failed to credit prize",
				zap.String("contest_id", contestID),
				zap.String("user_id", p.UserID),
				zap.Int("rank", p.Rank),
				zap.Int64("amount_cents", p.AmountCents),
				zap.Error(err))

			p.Status = "failed"
			p.ErrorMessage = err.Error()
			failedAllocations = append(failedAllocations, *p)

			// Log failure to settlement_events
			h.logSettlementEvent(ctx, settlementID, contestID, "prize_credit_failed", map[string]any{
				"user_id":      p.UserID,
				"rank":         p.Rank,
				"amount_cents": p.AmountCents,
			}, strPtr(err.Error()))
			continue
		}

		p.Status = "credited"
		distributedCount++

		h.logger.Info("Prize credited successfully",
			zap.String("contest_id", contestID),
			zap.String("user_id", p.UserID),
			zap.Int("rank", p.Rank),
			zap.Int64("amount_cents", p.AmountCents))
	}

	return distributedCount, failedAllocations, nil
}

// creditSinglePrize atomically credits a single prize to a user's wallet.
// Within one transaction it:
//  1. Checks idempotency via wallet_ledger.idempotency_key
//  2. Updates wallets.balance_cents
//  3. Inserts a wallet_ledger entry
//  4. Records in prize_distributions
//  5. Updates contest_participants.final_prize_cents
//  6. Logs to settlement_events
func (h *ContestHandlers) creditSinglePrize(ctx context.Context, contestID, settlementID string, p *contracts.PrizeAllocation) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	idempotencyKey := fmt.Sprintf("finalization:%s:%s:%d", contestID, p.UserID, p.Rank)

	// ---- Idempotency check ----
	var existingID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM wallet_ledger WHERE idempotency_key = $1`,
		idempotencyKey,
	).Scan(&existingID)
	if err == nil {
		// Already credited — idempotent success
		p.LedgerEntryID = existingID
		p.Status = "credited"
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check existing credit: %w", err)
	}

	// ---- Lock wallet and get current balance ----
	var balanceCents int64
	err = tx.QueryRowContext(ctx,
		`SELECT balance_cents FROM wallets WHERE user_id = $1 FOR UPDATE`,
		p.UserID,
	).Scan(&balanceCents)
	if err != nil {
		return fmt.Errorf("get wallet balance: %w", err)
	}

	newBalance := balanceCents + p.AmountCents

	// ---- Update wallets.balance_cents ----
	_, err = tx.ExecContext(ctx,
		`UPDATE wallets SET balance_cents = $1, updated_at = NOW() WHERE user_id = $2`,
		newBalance, p.UserID,
	)
	if err != nil {
		return fmt.Errorf("update wallet balance: %w", err)
	}

	// ---- Insert wallet_ledger entry with idempotency key ----
	var ledgerID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO wallet_ledger (user_id, type, amount_cents, balance_after_cents, ref_type, ref_id, description, idempotency_key)
		 VALUES ($1, 'prize_credit', $2, $3, 'contest', $4, $5, $6)
		 RETURNING id`,
		p.UserID, p.AmountCents, newBalance, contestID,
		fmt.Sprintf("Prize for contest %s (rank %d)", contestID, p.Rank),
		idempotencyKey,
	).Scan(&ledgerID)
	if err != nil {
		return fmt.Errorf("insert ledger entry: %w", err)
	}
	p.LedgerEntryID = ledgerID

	// ---- Get final score for prize_distributions record ----
	var finalScore float64
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(total_score, 0) FROM contest_participants WHERE contest_id = $1 AND user_id = $2`,
		contestID, p.UserID,
	).Scan(&finalScore)

	// ---- Record in prize_distributions ----
	_, err = tx.ExecContext(ctx,
		`INSERT INTO prize_distributions
			(settlement_id, contest_id, user_id, rank, final_score, prize_amount_cents, prize_percentage, status, credited_at, ledger_entry_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'credited', NOW(), $8)
		 ON CONFLICT (contest_id, user_id) DO UPDATE SET
			prize_amount_cents = EXCLUDED.prize_amount_cents,
			prize_percentage   = EXCLUDED.prize_percentage,
			status             = 'credited',
			credited_at        = NOW(),
			ledger_entry_id    = EXCLUDED.ledger_entry_id`,
		settlementID, contestID, p.UserID, p.Rank, finalScore, p.AmountCents, p.Percentage, ledgerID,
	)
	if err != nil {
		return fmt.Errorf("insert prize distribution: %w", err)
	}

	// ---- Update contest_participants.final_prize_cents ----
	_, err = tx.ExecContext(ctx,
		`UPDATE contest_participants SET final_prize_cents = $1 WHERE contest_id = $2 AND user_id = $3`,
		p.AmountCents, contestID, p.UserID,
	)
	if err != nil {
		return fmt.Errorf("update participant prize: %w", err)
	}

	// ---- Audit trail within the same transaction ----
	eventData, _ := json.Marshal(map[string]any{
		"user_id":         p.UserID,
		"rank":            p.Rank,
		"amount_cents":    p.AmountCents,
		"ledger_entry_id": ledgerID,
		"new_balance":     newBalance,
	})
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO settlement_events (settlement_id, contest_id, event_type, event_data)
		 VALUES ($1, $2, 'prize_credited', $3)`,
		settlementID, contestID, eventData,
	); err != nil {
		return fmt.Errorf("failed to insert settlement event: %w", err)
	}

	return tx.Commit()
}

// updateTraggePoints updates global T-Points based on contest performance.
func (h *ContestHandlers) updateTraggePoints(ctx context.Context, contestID string, rankings []contracts.FinalRanking) error {
	// T-Point contribution is handled by leaderboard-worker
	// This publishes an event to trigger the update
	if h.kafkaProducer == nil {
		return nil
	}

	event := map[string]any{
		"type":       "tragge_point_update",
		"contest_id": contestID,
		"rankings":   rankings,
		"ts":         time.Now().UnixMilli(),
	}

	return h.publishEvent(h.config.NotificationsTopic, contestID, event)
}

// notifyResults sends personalized result notifications to all participants.
func (h *ContestHandlers) notifyResults(ctx context.Context, contest *Contest, rankings []contracts.FinalRanking, prizes []contracts.PrizeAllocation) error {
	// Create a map of user prizes for quick lookup
	prizeMap := make(map[string]int64)
	for _, p := range prizes {
		prizeMap[p.UserID] = p.AmountCents
	}

	for _, r := range rankings {
		var notification contracts.ContestNotification
		prize := prizeMap[r.UserID]

		if prize > 0 {
			// Winner notification
			notification = contracts.ContestNotification{
				Type:      contracts.NotificationPrizeWon,
				ContestID: contest.ID,
				UserID:    r.UserID,
				Title:     "Congratulations! You Won!",
				Body:      fmt.Sprintf("You finished #%d in %s and won $%.2f!", r.Rank, contest.Name, float64(prize)/100),
				Data: map[string]any{
					"contest_name": contest.Name,
					"rank":         r.Rank,
					"score":        r.TotalScore,
					"prize_cents":  prize,
				},
				Channels: []contracts.NotificationChannel{
					contracts.ChannelPush,
					contracts.ChannelEmail,
					contracts.ChannelInApp,
				},
				Priority: contracts.PriorityCritical,
				Ts:       time.Now().UnixMilli(),
			}
		} else {
			// Non-winner notification
			notification = contracts.ContestNotification{
				Type:      contracts.NotificationContestCompleted,
				ContestID: contest.ID,
				UserID:    r.UserID,
				Title:     "Contest Completed",
				Body:      fmt.Sprintf("You finished #%d in %s. Better luck next time!", r.Rank, contest.Name),
				Data: map[string]any{
					"contest_name": contest.Name,
					"rank":         r.Rank,
					"score":        r.TotalScore,
				},
				Channels: []contracts.NotificationChannel{
					contracts.ChannelInApp,
				},
				Priority: contracts.PriorityNormal,
				Ts:       time.Now().UnixMilli(),
			}
		}

		if err := h.publishEvent(h.config.NotificationsTopic, r.UserID, notification); err != nil {
			h.logger.Warn("Failed to send result notification",
				zap.String("user_id", r.UserID),
				zap.Error(err))
		}
	}

	return nil
}

// archiveContest archives contest data and cleans up temporary resources.
func (h *ContestHandlers) archiveContest(ctx context.Context, contestID string) error {
	// Store final state snapshot
	snapshotData, _ := json.Marshal(map[string]any{
		"type":        "archived",
		"archived_at": time.Now().UnixMilli(),
	})

	_, err := h.pool.Primary().ExecContext(ctx, `
		INSERT INTO leaderboard_snapshots (contest_id, taken_at, payload_json)
		VALUES ($1, NOW(), $2)
	`, contestID, snapshotData)
	if err != nil {
		h.logger.Warn("Failed to store archive snapshot",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	// Publish cleanup event for Redis keys
	if h.kafkaProducer != nil {
		event := map[string]any{
			"type":       "cleanup",
			"contest_id": contestID,
			"keys":       []string{fmt.Sprintf("lb:%s", contestID)},
			"ts":         time.Now().UnixMilli(),
		}
		h.publishEvent(h.config.LeaderboardInitTopic, contestID, event)
	}

	return nil
}

// ============================================================================
// SETTLEMENT HELPERS
// ============================================================================

// getOrCreateSettlement creates a new settlement record or resumes an existing one.
// Uses UPSERT so repeated calls are safe (idempotent).
func (h *ContestHandlers) getOrCreateSettlement(ctx context.Context, contestID string, totalParticipants int) (string, error) {
	var settlementID string
	err := h.pool.Primary().QueryRowContext(ctx,
		`INSERT INTO contest_settlements (contest_id, status, started_at, total_participants, attempt_count)
		 VALUES ($1, 'in_progress', NOW(), $2, 1)
		 ON CONFLICT (contest_id) DO UPDATE SET
			status        = 'in_progress',
			started_at    = COALESCE(contest_settlements.started_at, NOW()),
			attempt_count = contest_settlements.attempt_count + 1
		 RETURNING id`,
		contestID, totalParticipants,
	).Scan(&settlementID)
	if err != nil {
		return "", fmt.Errorf("create settlement record: %w", err)
	}
	return settlementID, nil
}

// logSettlementEvent records an event in the settlement_events audit table.
// Failures are logged but never propagated — audit logging must not block
// the settlement flow.
func (h *ContestHandlers) logSettlementEvent(ctx context.Context, settlementID, contestID, eventType string, data map[string]any, errMsg *string) {
	eventData, _ := json.Marshal(data)
	_, err := h.pool.Primary().ExecContext(ctx,
		`INSERT INTO settlement_events (settlement_id, contest_id, event_type, event_data, error_message)
		 VALUES ($1, $2, $3, $4, $5)`,
		settlementID, contestID, eventType, eventData, errMsg,
	)
	if err != nil {
		h.logger.Warn("Failed to log settlement event",
			zap.String("settlement_id", settlementID),
			zap.String("event_type", eventType),
			zap.Error(err))
	}
}

// completeSettlement marks the settlement record as completed (or partial).
func (h *ContestHandlers) completeSettlement(ctx context.Context, settlementID, status string, totalWinners int) {
	_, err := h.pool.Primary().ExecContext(ctx,
		`UPDATE contest_settlements
		 SET status                = $1::settlement_status,
			 prizes_distributed_at = NOW(),
			 completed_at          = NOW(),
			 total_winners         = $2,
			 total_distributed_cents = (
				 SELECT COALESCE(SUM(prize_amount_cents), 0)
				 FROM prize_distributions
				 WHERE settlement_id = $3 AND status = 'credited'
			 )
		 WHERE id = $3`,
		status, totalWinners, settlementID,
	)
	if err != nil {
		h.logger.Warn("Failed to update settlement record",
			zap.String("settlement_id", settlementID),
			zap.Error(err))
	}
}

// strPtr returns a pointer to the given string. Used for nullable SQL parameters.
func strPtr(s string) *string {
	return &s
}

// ============================================================================
// HELPER METHODS
// ============================================================================

// publishEvent publishes an event to a Kafka topic.
func (h *ContestHandlers) publishEvent(topic, key string, event any) error {
	if h.kafkaProducer == nil {
		h.logger.Warn("Kafka producer is nil, skipping event publish",
			zap.String("topic", topic),
			zap.String("key", key))
		return nil
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(key),
		Value: sarama.ByteEncoder(eventJSON),
	}

	_, _, err = h.kafkaProducer.SendMessage(msg)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}
