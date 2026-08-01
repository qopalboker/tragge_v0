package server

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func (e *Engine) ProcessOrder(ctx context.Context, order *contracts.OrderRequest) error {
	// P1-4: Track order processing duration
	orderStart := time.Now()
	recordOrderMetric := func(result string) {
		if e.metrics != nil {
			duration := time.Since(orderStart).Seconds()
			e.metrics.OrderProcessingDuration.WithLabelValues(string(order.Type), result).Observe(duration)
			e.metrics.OrdersProcessedTotal.WithLabelValues(string(order.Type), result).Inc()
		}
	}

	e.logger.Info("Processing order",
		zap.String("order_id", order.OrderID),
		zap.String("user_id", order.UserID),
		zap.String("contest_id", order.ContestID),
		zap.String("symbol", order.Symbol),
		zap.String("side", string(order.Side)),
		zap.String("type", string(order.Type)),
		zap.Int64("qty", order.Qty))

	// 0. Rate limit check - done BEFORE any validation to minimize wasted computation
	if e.rateLimiter != nil {
		result := e.rateLimiter.Check(order.ContestID, order.UserID)
		if !result.Allowed {
			e.logger.Warn("Order rate limited",
				zap.String("order_id", order.OrderID),
				zap.String("user_id", order.UserID),
				zap.String("contest_id", order.ContestID),
				zap.String("scope", string(result.Scope)),
				zap.Duration("retry_after", result.RetryAfter))
			return e.rejectOrderRateLimited(ctx, order, result)
		}
	}

	// 1. Shard validation - reject orders for contests not assigned to this shard
	if e.shardingEnabled && e.shardedState != nil {
		if err := e.shardedState.RejectIfNotAssigned(order.ContestID); err != nil {
			return e.rejectOrder(ctx, order, fmt.Sprintf("wrong shard: %v", err))
		}
	}

	// 2. Validate contest is running and within time window
	contest, err := e.getContestCached(ctx, order.ContestID)
	if err != nil {
		return e.rejectOrder(ctx, order, fmt.Sprintf("failed to get contest: %v", err))
	}
	if contest == nil {
		return e.rejectOrder(ctx, order, "contest not found")
	}
	if contest.Status != "running" {
		return e.rejectOrder(ctx, order, fmt.Sprintf("contest is not running (status: %s)", contest.Status))
	}
	now := time.Now()
	if contest.StartsAt.Valid && now.Before(contest.StartsAt.Time) {
		return e.rejectOrder(ctx, order, fmt.Sprintf("contest has not started yet (starts at: %s)", contest.StartsAt.Time))
	}
	if contest.EndsAt.Valid && now.After(contest.EndsAt.Time) {
		return e.rejectOrder(ctx, order, fmt.Sprintf("contest has ended (ended at: %s)", contest.EndsAt.Time))
	}

	// 2.5. Market hours validation - check if market is open for this asset class
	// Allow closing existing positions during off-hours so users aren't trapped
	if e.marketHours != nil {
		status := e.marketHours.GetMarketStatus(contest.AssetClass)
		if !status.IsOpen {
			// Check if user is trying to close an existing position
			isClosingPosition := e.isClosingExistingPosition(ctx, order)
			if !isClosingPosition {
				e.logger.Warn("Order rejected due to market closed",
					zap.String("order_id", order.OrderID),
					zap.String("user_id", order.UserID),
					zap.String("contest_id", order.ContestID),
					zap.String("asset_class", contest.AssetClass),
					zap.String("reason", status.Reason))
				return e.rejectOrder(ctx, order, fmt.Sprintf("MARKET_CLOSED: %s market is closed (%s)", contest.AssetClass, status.Reason))
			}
			e.logger.Info("Allowing position close during off-hours",
				zap.String("order_id", order.OrderID),
				zap.String("user_id", order.UserID),
				zap.String("asset_class", contest.AssetClass))
		}
	}

	// 2.6. Validate symbol is allowed in this contest (P0-1)
	symbolAllowed, err := e.isSymbolAllowedInContest(ctx, order.ContestID, order.Symbol)
	if err != nil {
		e.logger.Error("Failed to check contest symbols",
			zap.String("order_id", order.OrderID),
			zap.String("contest_id", order.ContestID),
			zap.String("symbol", order.Symbol),
			zap.Error(err))
		return e.rejectOrder(ctx, order, fmt.Sprintf("failed to validate symbol: %v", err))
	}
	if !symbolAllowed {
		return e.rejectOrder(ctx, order, fmt.Sprintf("symbol %s is not allowed in this contest", order.Symbol))
	}

	// 3. Validate user is participant in contest
	participant, err := e.validateParticipant(ctx, order.ContestID, order.UserID)
	if err != nil {
		return e.rejectOrder(ctx, order, fmt.Sprintf("validation failed: %v", err))
	}
	if participant == nil {
		return e.rejectOrder(ctx, order, "user is not a participant in this contest")
	}

	// 4. Get or create user state
	contestState := e.state.GetOrCreateContest(order.ContestID)
	userState := contestState.GetOrCreateUser(order.UserID, participant.QtyTotal, participant.QtyAvailable, participant.TotalScore)

	// 5. Calculate required quantity (for buying power check)
	requiredQty := order.Qty

	// 6. Validate QTY limits (min/max per trade)
	if err := e.validateQtyLimits(requiredQty, userState.QtyTotal); err != nil {
		return e.rejectOrder(ctx, order, err.Error())
	}

	// 7. Validate qty <= qty_available
	if userState.GetQtyAvailable() < requiredQty {
		return e.rejectOrder(ctx, order, fmt.Sprintf("insufficient quantity available: have %d, need %d", userState.GetQtyAvailable(), requiredQty))
	}

	// 8. Insert order into database
	if err := InsertOrder(ctx, e.db, order.OrderID, order.ContestID, order.UserID, order.Symbol,
		order.Side, order.Type, order.Qty, order.LimitPrice, order.StopPrice, order.TakeProfit, order.StopLoss); err != nil {
		return e.rejectOrder(ctx, order, fmt.Sprintf("failed to insert order: %v", err))
	}

	// 9. Process based on order type
	switch order.Type {
	case contracts.OrderTypeMarket:
		err := e.executeMarketOrder(ctx, order, userState, contest)
		if err != nil {
			recordOrderMetric("error")
		} else {
			recordOrderMetric("filled")
		}
		return err

	case contracts.OrderTypeBuyLimit, contracts.OrderTypeSellLimit,
		contracts.OrderTypeBuyStop, contracts.OrderTypeSellStop:
		// Pending orders - add to pending book and acknowledge
		err := e.processPendingOrder(ctx, order, userState)
		if err != nil {
			recordOrderMetric("error")
		} else {
			recordOrderMetric("accepted")
		}
		return err

	case contracts.OrderTypeLimit, contracts.OrderTypeStop:
		// Legacy types - acknowledge as pending but don't process
		e.logger.Warn("Legacy order type used",
			zap.String("order_id", order.OrderID),
			zap.String("order_type", string(order.Type)))
		recordOrderMetric("accepted")
		return e.acknowledgeOrder(ctx, order, contracts.OrderStatusAccepted, nil)

	default:
		recordOrderMetric("rejected")
		return e.rejectOrder(ctx, order, fmt.Sprintf("unsupported order type: %s", order.Type))
	}
}

// getContestCached returns a contest, checking the in-memory cache first.
// On cache miss it queries the database and stores the result.
// When CacheEnabled is false, it skips the cache entirely and goes straight to the database.
func (e *Engine) getContestCached(ctx context.Context, contestID string) (*DBContest, error) {
	if e.config.CacheEnabled {
		if cached, hit := e.contestCache.Get(contestID); hit {
			return cached, nil
		}
	}

	contest, err := GetContest(ctx, e.db, contestID)
	if err != nil {
		return nil, err
	}
	if contest != nil && e.config.CacheEnabled {
		e.contestCache.Set(contestID, contest)
	}
	return contest, nil
}

// getParticipantCached returns a participant, checking the in-memory cache first.
// On cache miss it queries the database and stores the result.
// When CacheEnabled is false, it skips the cache entirely and goes straight to the database.
func (e *Engine) getParticipantCached(ctx context.Context, contestID, userID string) (*DBParticipant, error) {
	if e.config.CacheEnabled {
		if cached, hit := e.participantCache.Get(contestID, userID); hit {
			return cached, nil
		}
	}

	participant, err := GetParticipant(ctx, e.db, contestID, userID)
	if err != nil {
		return nil, err
	}
	if participant != nil && e.config.CacheEnabled {
		e.participantCache.Set(contestID, userID, participant)
	}
	return participant, nil
}

// validateParticipant checks if the user is a participant in the contest.
// Uses the participant cache to avoid redundant database calls.
func (e *Engine) validateParticipant(ctx context.Context, contestID, userID string) (*DBParticipant, error) {
	return e.getParticipantCached(ctx, contestID, userID)
}

// MaxAllowedQty is the absolute maximum order quantity to prevent overflow.
const MaxAllowedQty int64 = 10_000_000

// validateQtyLimits validates the order quantity against min/max limits.
// Min QTY: Ensures traders don't place tiny orders
// Max QTY: Prevents concentration risk (e.g., max 50% of total in single trade)
func (e *Engine) validateQtyLimits(orderQty, qtyTotal int64) error {
	// P2-2: Reject non-positive quantities
	if orderQty <= 0 {
		return fmt.Errorf("order quantity must be positive, got %d", orderQty)
	}

	// P2-2: Reject quantities exceeding absolute maximum
	if orderQty > MaxAllowedQty {
		return fmt.Errorf("order quantity %d exceeds absolute maximum %d", orderQty, MaxAllowedQty)
	}

	// Validate minimum QTY per trade
	if e.config.QtyMinPerTrade > 0 && orderQty < e.config.QtyMinPerTrade {
		return fmt.Errorf("order quantity %d is below minimum %d", orderQty, e.config.QtyMinPerTrade)
	}

	// Validate maximum QTY per trade (as percentage of total)
	// P2-2: Check for overflow before multiplication
	if e.config.QtyMaxPctOfTotal > 0 && qtyTotal > 0 {
		pct := int64(e.config.QtyMaxPctOfTotal)
		if qtyTotal > (1<<62)/pct {
			// Would overflow - allow the order since qtyTotal is extremely large
			return nil
		}
		maxQty := (qtyTotal * pct) / 100
		if orderQty > maxQty {
			return fmt.Errorf("order quantity %d exceeds maximum %d (%d%% of total %d)",
				orderQty, maxQty, e.config.QtyMaxPctOfTotal, qtyTotal)
		}
	}

	return nil
}

// isClosingExistingPosition checks if an order is closing (or reducing) an existing position.
// This is used to allow position closes during market off-hours.
// A BUY order on a short position or a SELL order on a long position is considered closing.
func (e *Engine) isClosingExistingPosition(ctx context.Context, order *contracts.OrderRequest) bool {
	// Check if user has an existing position in the opposite direction
	position, err := GetOpenPosition(ctx, e.db, order.ContestID, order.UserID, order.Symbol)
	if err != nil || position == nil {
		return false // No existing position to close
	}

	// A SELL order on a long position closes it
	// A BUY order on a short position closes it
	return !IsSameSide(position.Side, order.Side)
}

// executeMarketOrder executes a market order immediately at current price.
func (e *Engine) executeMarketOrder(ctx context.Context, order *contracts.OrderRequest, userState *UserState, contest *DBContest) error {
	// 1. Get current market price with age - prefer price book (bid/ask), fallback to Redis
	price, priceAge, err := e.getMarketPriceWithAge(ctx, order.Symbol, order.Side)
	if err != nil {
		e.logger.Warn("Order rejected due to missing price data",
			zap.String("order_id", order.OrderID),
			zap.String("symbol", order.Symbol),
			zap.String("asset_class", contest.AssetClass),
			zap.Error(err))
		// Update order status to rejected
		if err := UpdateOrderStatus(ctx, e.db, order.OrderID, "rejected", 0); err != nil {
			e.logger.Error("Failed to update order status",
				zap.String("order_id", order.OrderID),
				zap.Error(err))
		}
		if e.metrics != nil {
			e.metrics.OrdersRejectedStalePrice.WithLabelValues(order.ContestID, string(order.Type)).Inc()
		}
		return e.rejectOrder(ctx, order, fmt.Sprintf("No price data available for %s", order.Symbol))
	}

	// 2. Check if price is stale for market orders
	// Determine if this is a position close (opposite side of existing position)
	isClosing := e.isClosingExistingPosition(ctx, order)

	// Use per-asset-class threshold based on open vs close operation
	maxPriceAge := e.getMaxPriceAge(contest.AssetClass, isClosing)

	// Allow per-contest override if available
	if contest.Rules != nil && contest.Rules.MaxPriceAgeMarketSeconds != nil {
		maxPriceAge = time.Duration(*contest.Rules.MaxPriceAgeMarketSeconds) * time.Second
	}

	if maxPriceAge > 0 && priceAge > maxPriceAge {
		// Log the rejection at WARN level with structured fields
		e.logger.Warn("Order rejected due to stale price",
			zap.String("order_id", order.OrderID),
			zap.String("symbol", order.Symbol),
			zap.String("asset_class", contest.AssetClass),
			zap.Bool("is_closing", isClosing),
			zap.Float64("price_age_seconds", priceAge.Seconds()),
			zap.Float64("max_allowed_seconds", maxPriceAge.Seconds()))

		// Update order status to rejected
		if err := UpdateOrderStatus(ctx, e.db, order.OrderID, "rejected", 0); err != nil {
			e.logger.Error("Failed to update order status",
				zap.String("order_id", order.OrderID),
				zap.Error(err))
		}

		// Increment Prometheus counter
		if e.metrics != nil {
			e.metrics.OrdersRejectedStalePrice.WithLabelValues(order.ContestID, string(order.Type)).Inc()
		}

		// Emit order reject event with detailed information
		e.emitOrderRejectEvent(ctx, order, "STALE_PRICE", priceAge, maxPriceAge)

		return e.rejectOrder(ctx, order, fmt.Sprintf("price data is stale (age: %.1fs, max: %.1fs)", priceAge.Seconds(), maxPriceAge.Seconds()))
	}

	// 3. Acquire position lock BEFORE reserving quantity to prevent race conditions.
	// Without the lock, concurrent orders could both successfully reserve qty between
	// the reserve and lock steps, leading to phantom reservations and qty leaks.
	// Lock covers: qty reservation, fill insert, order status update, position update, and participant update.
	unlockPosition, lockErr := e.positionLocks.AcquireLockForSymbolWithTimeout(ctx, order.ContestID, order.UserID, order.Symbol)
	if lockErr != nil {
		e.logger.Error("Failed to acquire position lock for market order",
			zap.String("order_id", order.OrderID),
			zap.String("contest_id", order.ContestID),
			zap.String("user_id", order.UserID),
			zap.String("symbol", order.Symbol),
			zap.Error(lockErr))
		if err := UpdateOrderStatus(ctx, e.db, order.OrderID, "rejected", 0); err != nil {
			e.logger.Error("Failed to update order status", zap.String("order_id", order.OrderID), zap.Error(err))
		}
		return e.rejectOrder(ctx, order, "position lock timeout — please retry")
	}
	defer unlockPosition()

	// 4. Reserve quantity (now serialized by position lock for same-symbol orders)
	if !userState.ReserveQty(order.Qty) {
		if err := UpdateOrderStatus(ctx, e.db, order.OrderID, "rejected", 0); err != nil {
			e.logger.Error("Failed to update order status",
				zap.String("order_id", order.OrderID),
				zap.Error(err))
		}
		return e.rejectOrder(ctx, order, "insufficient quantity available")
	}

	// 5. P2-1: Re-check contest status from DB before committing fill
	// This prevents fills after contest ends (cache may be stale)
	freshContest, err := GetContest(ctx, e.db, order.ContestID)
	if err != nil || freshContest == nil || freshContest.Status != "running" {
		userState.ReleaseQty(order.Qty)
		if err := UpdateOrderStatus(ctx, e.db, order.OrderID, "rejected", 0); err != nil {
			e.logger.Error("Failed to update order status", zap.String("order_id", order.OrderID), zap.Error(err))
		}
		return e.rejectOrder(ctx, order, "contest is no longer running")
	}

	// 5b. Execute the fill
	fillID := uuid.New().String()
	fillPrice := price
	fillQty := order.Qty
	fillTs := time.Now().UnixMilli()

	// 6. Atomic transaction: fill + order status + position + participant
	var posResult *PositionTxResult
	err = WithTransaction(ctx, e.db, func(tx *sql.Tx) error {
		// 6a. Insert fill
		if err := InsertFillTx(ctx, tx, fillID, order.OrderID, order.ContestID, order.UserID, order.Symbol,
			order.Side, fillQty, fillPrice); err != nil {
			return fmt.Errorf("insert fill: %w", err)
		}

		// 6b. Update order status to filled
		if err := UpdateOrderStatusTx(ctx, tx, order.OrderID, "filled", fillQty); err != nil {
			return fmt.Errorf("update order status: %w", err)
		}

		// 6c. Update position (reads + writes within transaction)
		var posErr error
		posResult, posErr = e.updatePositionTx(ctx, tx, order, userState, fillPrice, fillQty, true)
		if posErr != nil {
			return fmt.Errorf("update position: %w", posErr)
		}

		// 6d. Store realized P&L on the fill for close/partial_close branches
		if posResult.Branch == "close" || posResult.Branch == "partial_close" {
			if err := UpdateFillRealizedPnlTx(ctx, tx, fillID, posResult.TradeScore); err != nil {
				return fmt.Errorf("update fill realized_pnl: %w", err)
			}
		}

		// 6e. Update participant qty_available (for "new" and "add" branches only;
		//     "close" and "partial_close" branches already update participant inside updatePositionTx)
		if posResult.Branch == "new" || posResult.Branch == "add" {
			if err := UpdateParticipantQtyAvailableTx(ctx, tx, order.ContestID, order.UserID, userState.GetQtyAvailable()); err != nil {
				return fmt.Errorf("update participant qty: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		// Transaction failed -- rollback quantity reservation
		userState.ReleaseQty(fillQty)
		return fmt.Errorf("market order transaction: %w", err)
	}

	// 7. Apply in-memory state updates AFTER successful DB commit (WAL-protected)
	deltaScore := e.applyPositionResultToMemory(ctx, order, userState, posResult, fillPrice)

	// 8. Track position with TP/SL if applicable
	if order.TakeProfit != nil || order.StopLoss != nil {
		e.trackPositionTPSL(order.ContestID, order.UserID, order.Symbol, order.Side, order.TakeProfit, order.StopLoss)
	}

	// 9. Emit events
	e.emitOrderAck(ctx, order.OrderID, contracts.OrderStatusAccepted, nil)
	e.emitFillEvent(ctx, fillID, order, fillQty, fillPrice, fillTs)
	e.emitPositionUpdate(ctx, order.ContestID, order.UserID, userState, fillPrice)
	e.emitPnLDelta(ctx, order.ContestID, order.UserID, userState, deltaScore)

	e.logger.Info("Market order filled",
		zap.String("order_id", order.OrderID),
		zap.Int64("qty", fillQty),
		zap.Float64("price", fillPrice),
		zap.String("symbol", order.Symbol))
	return nil
}

func (e *Engine) processPendingOrder(ctx context.Context, order *contracts.OrderRequest, userState *UserState) error {
	// Reserve quantity for the pending order
	if !userState.ReserveQty(order.Qty) {
		if err := UpdateOrderStatus(ctx, e.db, order.OrderID, "rejected", 0); err != nil {
			e.logger.Error("Failed to update order status",
				zap.String("order_id", order.OrderID),
				zap.Error(err))
		}
		return e.rejectOrder(ctx, order, "insufficient quantity available")
	}

	// Update order status to 'open' and persist qty_available within a transaction.
	// If the DB write fails, we release the reserved qty.
	if err := WithTransaction(ctx, e.db, func(tx *sql.Tx) error {
		if err := UpdateOrderStatusTx(ctx, tx, order.OrderID, "open", 0); err != nil {
			return err
		}
		return UpdateParticipantQtyAvailableTx(ctx, tx, order.ContestID, order.UserID, userState.GetQtyAvailable())
	}); err != nil {
		userState.ReleaseQty(order.Qty)
		e.logger.Error("Failed to update order status",
			zap.String("order_id", order.OrderID),
			zap.Error(err))
		return e.rejectOrder(ctx, order, "failed to update order status")
	}

	// DB committed successfully — now update in-memory state.
	//
	// NOTE: There is a small window between the DB commit above and the
	// pendingBook.AddPendingOrder() call below. If the engine crashes in this
	// window, DB will have the order as status='open' but pendingBook will be
	// empty. This is intentionally accepted because:
	//   1. PendingOrderBook.ReloadFromDB() restores all pending orders from DB
	//      on engine startup.
	//   2. StateReloader.reloadContest() also reloads pending orders during
	//      divergence recovery.
	// Add to pending order book
	pendingOrder := &PendingOrderInfo{
		OrderID:    order.OrderID,
		ContestID:  order.ContestID,
		UserID:     order.UserID,
		Symbol:     order.Symbol,
		Side:       order.Side,
		Type:       order.Type,
		Qty:        order.Qty,
		LimitPrice: order.LimitPrice,
		StopPrice:  order.StopPrice,
		TakeProfit: order.TakeProfit,
		StopLoss:   order.StopLoss,
	}
	e.pendingBook.AddPendingOrder(pendingOrder)

	// Also add to user's pending orders in state
	userState.AddPendingOrder(&PendingOrder{
		OrderID:    order.OrderID,
		Symbol:     order.Symbol,
		Side:       order.Side,
		Type:       order.Type,
		Qty:        order.Qty,
		QtyFilled:  0,
		LimitPrice: order.LimitPrice,
		StopPrice:  order.StopPrice,
	})

	e.logger.Info("Pending order added",
		zap.String("order_id", order.OrderID),
		zap.String("order_type", string(order.Type)),
		zap.String("symbol", order.Symbol),
		zap.Float64p("limit_price", order.LimitPrice),
		zap.Float64p("stop_price", order.StopPrice))

	// Acknowledge the order as accepted
	return e.acknowledgeOrder(ctx, order, contracts.OrderStatusAccepted, nil)
}

func (e *Engine) executePendingOrder(ctx context.Context, triggered *TriggeredOrder) {
	order := triggered.Order

	e.logger.Info("Pending order triggered",
		zap.String("order_id", order.OrderID),
		zap.Float64("fill_price", triggered.FillPrice),
		zap.String("symbol", order.Symbol))

	// Check if price is stale for pending order triggers
	// Pending orders are opening new positions, so use "open" thresholds
	contest, err := e.getContestCached(ctx, order.ContestID)
	assetClass := ""
	if err == nil && contest != nil {
		assetClass = contest.AssetClass
	}
	maxPriceAge := e.getMaxPriceAge(assetClass, false)

	// Allow per-contest override
	if err == nil && contest != nil && contest.Rules != nil && contest.Rules.MaxPriceAgePendingSeconds != nil {
		maxPriceAge = time.Duration(*contest.Rules.MaxPriceAgePendingSeconds) * time.Second
	}

	priceAge, hasPrice := e.priceBook.GetPriceAge(order.Symbol)
	if maxPriceAge > 0 && hasPrice && priceAge > maxPriceAge {
		// Log the stale price retry at WARN level with structured fields
		e.logger.Warn("Pending order not executed due to stale price, marked for retry",
			zap.String("order_id", order.OrderID),
			zap.String("symbol", order.Symbol),
			zap.String("asset_class", assetClass),
			zap.Float64("price_age_seconds", priceAge.Seconds()),
			zap.Float64("max_allowed_seconds", maxPriceAge.Seconds()))

		// Increment Prometheus counter
		if e.metrics != nil {
			e.metrics.OrdersRejectedStalePrice.WithLabelValues(order.ContestID, string(order.Type)).Inc()
		}

		// Mark the order with stale_price_retry flag and keep it in the pending book
		// (it was already in the pending book, we just set the flag)
		order.StalePriceRetry = true

		// Don't remove from pending book - it will be re-evaluated on the next fresh price
		return
	}

	// Acquire position lock BEFORE accessing user state to prevent race conditions.
	// Without the lock, concurrent pending order executions could read stale state.
	// This mirrors the executeMarketOrder pattern (lock first, then access state).
	// Lock covers: state access, fill insert, order status update, position update, and participant update.
	unlockPosition, lockErr := e.positionLocks.AcquireLockForSymbolWithTimeout(ctx, order.ContestID, order.UserID, order.Symbol)
	if lockErr != nil {
		e.logger.Error("Failed to acquire position lock for pending order",
			zap.String("order_id", order.OrderID),
			zap.String("contest_id", order.ContestID),
			zap.String("user_id", order.UserID),
			zap.String("symbol", order.Symbol),
			zap.Error(lockErr))
		// Leave order in pending book — it will be re-evaluated on the next tick
		return
	}
	defer unlockPosition()

	// Get contest and user state (do NOT remove from pending book yet — wait for DB commit)
	contestState := e.state.GetOrCreateContest(order.ContestID)
	userState, exists := contestState.GetUser(order.UserID)
	if !exists {
		e.logger.Warn("User state not found for pending order",
			zap.String("order_id", order.OrderID),
			zap.String("user_id", order.UserID),
			zap.String("contest_id", order.ContestID))
		return
	}

	// Re-check contest status from DB before committing pending order fill.
	// The contest cache may be stale (up to 30s TTL), so a contest that transitioned
	// from "running" to "settling"/"completed" could still have its pending orders trigger.
	// This mirrors the fresh check done for market orders.
	freshContest, freshErr := GetContest(ctx, e.db, order.ContestID)
	if freshErr != nil || freshContest == nil || freshContest.Status != "running" {
		// Safe to remove now: order will be marked "rejected" in DB, preventing re-execution
		e.pendingBook.RemovePendingOrder(order.Symbol, order.OrderID)
		userState.RemovePendingOrder(order.OrderID)
		// Release the qty that was reserved when the pending order was placed
		userState.ReleaseQty(order.Qty)
		if updateErr := UpdateOrderStatus(ctx, e.db, order.OrderID, "rejected", 0); updateErr != nil {
			e.logger.Error("Failed to update order status",
				zap.String("order_id", order.OrderID),
				zap.Error(updateErr))
		}
		e.logger.Warn("Pending order rejected: contest no longer running",
			zap.String("order_id", order.OrderID),
			zap.String("contest_id", order.ContestID))
		e.rejectOrder(ctx, &contracts.OrderRequest{
			OrderID:   order.OrderID,
			UserID:    order.UserID,
			ContestID: order.ContestID,
			Symbol:    order.Symbol,
			Side:      order.Side,
			Type:      order.Type,
			Qty:       order.Qty,
		}, "contest is no longer running")
		return
	}

	// Create fill data
	fillID := uuid.New().String()
	fillQty := order.Qty
	fillPrice := triggered.FillPrice
	fillTs := time.Now().UnixMilli()

	// Build order request for position update
	orderReq := &contracts.OrderRequest{
		OrderID:    order.OrderID,
		UserID:     order.UserID,
		ContestID:  order.ContestID,
		Symbol:     order.Symbol,
		Side:       order.Side,
		Type:       order.Type,
		Qty:        order.Qty,
		TakeProfit: order.TakeProfit,
		StopLoss:   order.StopLoss,
	}

	// Atomic transaction: fill + order status + position + participant
	// Mirrors the proven executeMarketOrder pattern (resolves TODO at updatePosition).
	var posResult *PositionTxResult
	err = WithTransaction(ctx, e.db, func(tx *sql.Tx) error {
		// Insert fill
		if err := InsertFillTx(ctx, tx, fillID, order.OrderID, order.ContestID, order.UserID, order.Symbol,
			order.Side, fillQty, fillPrice); err != nil {
			return fmt.Errorf("insert fill: %w", err)
		}

		// Update order status to filled
		if err := UpdateOrderStatusTx(ctx, tx, order.OrderID, "filled", fillQty); err != nil {
			return fmt.Errorf("update order status: %w", err)
		}

		// Update position (reads + writes within transaction)
		var posErr error
		posResult, posErr = e.updatePositionTx(ctx, tx, orderReq, userState, fillPrice, fillQty, true)
		if posErr != nil {
			return fmt.Errorf("update position: %w", posErr)
		}

		// Store realized P&L on the fill for close/partial_close branches
		if posResult.Branch == "close" || posResult.Branch == "partial_close" {
			if err := UpdateFillRealizedPnlTx(ctx, tx, fillID, posResult.TradeScore); err != nil {
				return fmt.Errorf("update fill realized_pnl: %w", err)
			}
		}

		// Update participant qty_available for "new" and "add" branches
		if posResult.Branch == "new" || posResult.Branch == "add" {
			if err := UpdateParticipantQtyAvailableTx(ctx, tx, order.ContestID, order.UserID, userState.GetQtyAvailable()); err != nil {
				return fmt.Errorf("update participant qty: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		// Transaction failed — do NOT remove from pending book.
		// Order stays "pending" in both memory and DB, will be re-evaluated on next tick.
		e.logger.Error("Pending order transaction failed, will retry on next tick",
			zap.String("order_id", order.OrderID),
			zap.String("fill_id", fillID),
			zap.Error(err))
		return
	}

	// Transaction committed successfully — now safe to update in-memory state
	e.pendingBook.RemovePendingOrder(order.Symbol, order.OrderID)
	userState.RemovePendingOrder(order.OrderID)

	// Apply position result to in-memory state (WAL-protected)
	deltaScore := e.applyPositionResultToMemory(ctx, orderReq, userState, posResult, fillPrice)

	// Track position with TP/SL if applicable
	if order.TakeProfit != nil || order.StopLoss != nil {
		e.trackPositionTPSL(order.ContestID, order.UserID, order.Symbol, order.Side, order.TakeProfit, order.StopLoss)
	}

	// Emit events
	e.emitFillEvent(ctx, fillID, orderReq, fillQty, fillPrice, fillTs)
	e.emitPositionUpdate(ctx, order.ContestID, order.UserID, userState, fillPrice)
	e.emitPnLDelta(ctx, order.ContestID, order.UserID, userState, deltaScore)

	e.logger.Info("Pending order filled",
		zap.String("order_id", order.OrderID),
		zap.Int64("qty", fillQty),
		zap.Float64("price", fillPrice),
		zap.String("symbol", order.Symbol))
}

// executeTPSL executes a triggered TP/SL by closing the position.
func (e *Engine) executeTPSL(ctx context.Context, triggered *TriggeredTPSL) {
	pos := triggered.Position
	tpslType := "SL"
	if triggered.IsTP {
		tpslType = "TP"
	}

	e.logger.Info("TP/SL triggered",
		zap.String("type", tpslType),
		zap.String("position_id", pos.PositionID),
		zap.Float64("fill_price", triggered.FillPrice),
		zap.Float64p("stop_loss", pos.StopLoss),
		zap.Float64p("take_profit", pos.TakeProfit))

	// Check price freshness for TP/SL (close operation)
	contest, contestErr := e.getContestCached(ctx, pos.ContestID)
	tpslAssetClass := ""
	if contestErr == nil && contest != nil {
		tpslAssetClass = contest.AssetClass
	}
	maxTPSLAge := e.getMaxPriceAge(tpslAssetClass, true)
	tpslPriceAge, hasTpslPrice := e.priceBook.GetPriceAge(pos.Symbol)
	if !hasTpslPrice {
		e.logger.Warn("TP/SL not executed - no price data available",
			zap.String("position_id", pos.PositionID),
			zap.String("symbol", pos.Symbol),
			zap.String("type", tpslType))
		return
	}
	if maxTPSLAge > 0 && tpslPriceAge > maxTPSLAge {
		e.logger.Warn("TP/SL not executed due to stale price, will retry on fresh price",
			zap.String("position_id", pos.PositionID),
			zap.String("symbol", pos.Symbol),
			zap.String("type", tpslType),
			zap.String("asset_class", tpslAssetClass),
			zap.Float64("price_age_seconds", tpslPriceAge.Seconds()),
			zap.Float64("max_allowed_seconds", maxTPSLAge.Seconds()))
		if e.metrics != nil {
			e.metrics.OrdersRejectedStalePrice.WithLabelValues(pos.ContestID, "tpsl_"+tpslType).Inc()
		}
		return
	}

	// Get contest and user state
	contestState := e.state.GetOrCreateContest(pos.ContestID)
	userState, exists := contestState.GetUser(pos.UserID)
	if !exists {
		e.logger.Warn("User state not found for TP/SL",
			zap.String("position_id", pos.PositionID),
			zap.String("user_id", pos.UserID),
			zap.String("contest_id", pos.ContestID))
		return
	}

	// Determine close side (opposite of position side)
	closeSide := OppositeSide(pos.Side)

	// Acquire position lock to prevent race conditions during position modification.
	// Lock covers: order insert, fill insert, position update, and participant update.
	unlockPosition, lockErr := e.positionLocks.AcquireLockForSymbolWithTimeout(ctx, pos.ContestID, pos.UserID, pos.Symbol)
	if lockErr != nil {
		e.logger.Error("Failed to acquire position lock for TP/SL execution",
			zap.String("position_id", pos.PositionID),
			zap.String("contest_id", pos.ContestID),
			zap.String("user_id", pos.UserID),
			zap.String("symbol", pos.Symbol),
			zap.Error(lockErr))
		// TP/SL stays in pending book and will be re-evaluated on next tick
		return
	}
	defer unlockPosition()

	// Don't remove from TP/SL tracking yet — moved to after transaction success,
	// so it can be re-evaluated on the next tick if the transaction fails.

	// Create a synthetic order to close the position
	closeOrderID := uuid.New().String()
	fillID := uuid.New().String()
	fillPrice := triggered.FillPrice

	// Re-read position from DB after acquiring lock to get fresh QtyOpen.
	// The pendingBook's pos.QtyOpen may be stale if a partial close happened
	// between TP/SL evaluation and execution.
	freshPos, freshErr := GetOpenPosition(ctx, e.db, pos.ContestID, pos.UserID, pos.Symbol)
	if freshErr != nil || freshPos == nil {
		// Position was fully closed by another operation while we waited for the lock
		e.pendingBook.RemovePositionTPSL(pos.Symbol, pos.PositionID)
		e.logger.Info("TP/SL skipped - position already closed",
			zap.String("position_id", pos.PositionID))
		return
	}
	fillQty := freshPos.QtyOpen // Use fresh QtyOpen from DB
	fillTs := time.Now().UnixMilli()

	// Create a temporary order request for position update
	orderReq := &contracts.OrderRequest{
		OrderID:   closeOrderID,
		UserID:    pos.UserID,
		ContestID: pos.ContestID,
		Symbol:    pos.Symbol,
		Side:      closeSide,
		Type:      contracts.OrderTypeMarket,
		Qty:       fillQty,
	}

	// Atomic transaction: order + fill + position + participant
	// Mirrors the proven executePendingOrder / executeMarketOrder pattern.
	var posResult *PositionTxResult
	err := WithTransaction(ctx, e.db, func(tx *sql.Tx) error {
		// Insert synthetic close order
		if err := insertOrderTx(ctx, tx, closeOrderID, pos.ContestID, pos.UserID, pos.Symbol,
			closeSide, contracts.OrderTypeMarket, fillQty, nil, nil, nil, nil); err != nil {
			return fmt.Errorf("insert close order: %w", err)
		}

		// Update order status to filled
		if err := updateOrderStatusTx(ctx, tx, closeOrderID, "filled", fillQty); err != nil {
			return fmt.Errorf("update order status: %w", err)
		}

		// Insert fill
		if err := InsertFillTx(ctx, tx, fillID, closeOrderID, pos.ContestID, pos.UserID, pos.Symbol,
			closeSide, fillQty, fillPrice); err != nil {
			return fmt.Errorf("insert fill: %w", err)
		}

		// Update position within transaction
		var posErr error
		posResult, posErr = e.updatePositionTx(ctx, tx, orderReq, userState, fillPrice, fillQty, false)
		if posErr != nil {
			return fmt.Errorf("update position: %w", posErr)
		}

		// Store realized P&L on the fill for close/partial_close branches
		if posResult.Branch == "close" || posResult.Branch == "partial_close" {
			if err := UpdateFillRealizedPnlTx(ctx, tx, fillID, posResult.TradeScore); err != nil {
				return fmt.Errorf("update fill realized_pnl: %w", err)
			}
		}

		// Update participant qty_available for "new" and "add" branches
		if posResult.Branch == "new" || posResult.Branch == "add" {
			if err := UpdateParticipantQtyAvailableTx(ctx, tx, pos.ContestID, pos.UserID,
				userState.GetQtyAvailable()); err != nil {
				return fmt.Errorf("update participant qty: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		// Transaction failed — TP/SL stays in pending book and will be re-evaluated on next tick.
		e.logger.Error("TP/SL transaction failed, will retry on next tick",
			zap.String("position_id", pos.PositionID),
			zap.String("type", tpslType),
			zap.Error(err))
		return
	}

	// Transaction committed — now safe to update in-memory state
	e.pendingBook.RemovePositionTPSL(pos.Symbol, pos.PositionID)

	// Apply position result to in-memory state (WAL-protected)
	deltaScore := e.applyPositionResultToMemory(ctx, orderReq, userState, posResult, fillPrice)

	// Emit events
	e.emitFillEvent(ctx, fillID, orderReq, fillQty, fillPrice, fillTs)
	e.emitPositionUpdate(ctx, pos.ContestID, pos.UserID, userState, fillPrice)
	e.emitPnLDelta(ctx, pos.ContestID, pos.UserID, userState, deltaScore)

	e.logger.Info("Position closed by TP/SL",
		zap.String("position_id", pos.PositionID),
		zap.String("type", tpslType),
		zap.Int64("qty", fillQty),
		zap.Float64("price", fillPrice),
		zap.Float64("score", deltaScore))
}

func (e *Engine) ProcessClosePosition(ctx context.Context, req *contracts.ClosePositionRequest) error {
	e.logger.Info("Processing close position request",
		zap.String("position_id", req.PositionID),
		zap.String("user_id", req.UserID),
		zap.String("contest_id", req.ContestID),
		zap.String("reason", req.CloseReason))

	// 1. Shard validation - reject requests for contests not assigned to this shard
	if e.shardingEnabled && e.shardedState != nil {
		if err := e.shardedState.RejectIfNotAssigned(req.ContestID); err != nil {
			e.logger.Warn("Close position rejected - wrong shard",
				zap.String("position_id", req.PositionID),
				zap.Error(err))
			return fmt.Errorf("wrong shard: %w", err)
		}
	}

	// 2. Validate contest is running
	contest, err := e.getContestCached(ctx, req.ContestID)
	if err != nil {
		e.logger.Error("Close position failed - failed to get contest",
			zap.String("contest_id", req.ContestID),
			zap.Error(err))
		return fmt.Errorf("failed to get contest: %w", err)
	}
	if contest == nil {
		e.logger.Warn("Close position failed - contest not found",
			zap.String("contest_id", req.ContestID))
		return fmt.Errorf("contest not found")
	}
	if contest.Status != "running" {
		e.logger.Warn("Close position failed - contest not running",
			zap.String("contest_id", req.ContestID),
			zap.String("status", contest.Status))
		return fmt.Errorf("contest is not running (status: %s)", contest.Status)
	}

	// 3. Get the position from database to validate it exists and belongs to user
	dbPos, err := GetPositionByID(ctx, e.db, req.PositionID)
	if err != nil {
		e.logger.Error("Close position failed - failed to get position",
			zap.String("position_id", req.PositionID),
			zap.Error(err))
		return fmt.Errorf("failed to get position: %w", err)
	}
	if dbPos == nil {
		// Position not found - may have already been closed
		e.logger.Warn("Close position warning - position not found (may already be closed)",
			zap.String("position_id", req.PositionID))
		return nil
	}

	// 4. Validate position ownership
	if dbPos.UserID != req.UserID {
		e.logger.Warn("Close position failed - user does not own position",
			zap.String("user_id", req.UserID),
			zap.String("position_id", req.PositionID))
		return fmt.Errorf("position does not belong to user")
	}
	if dbPos.ContestID != req.ContestID {
		e.logger.Warn("Close position failed - position not in contest",
			zap.String("position_id", req.PositionID),
			zap.String("contest_id", req.ContestID))
		return fmt.Errorf("position is not in specified contest")
	}

	// 5. Acquire position lock to prevent race conditions during position modification.
	// Lock covers: all position calculations, database transactions, and in-memory state updates.
	unlockPosition, lockErr := e.positionLocks.AcquireLockForSymbolWithTimeout(ctx, req.ContestID, req.UserID, dbPos.Symbol)
	if lockErr != nil {
		e.logger.Error("Failed to acquire position lock for close position",
			zap.String("position_id", req.PositionID),
			zap.String("contest_id", req.ContestID),
			zap.String("user_id", req.UserID),
			zap.String("symbol", dbPos.Symbol),
			zap.Error(lockErr))
		return fmt.Errorf("position lock timeout: %w", lockErr)
	}
	defer unlockPosition()

	// 6. Re-read position to ensure we have fresh data after acquiring lock
	dbPos, err = GetPositionByID(ctx, e.db, req.PositionID)
	if err != nil {
		e.logger.Error("Close position failed - failed to re-read position",
			zap.String("position_id", req.PositionID),
			zap.Error(err))
		return fmt.Errorf("failed to re-read position: %w", err)
	}
	if dbPos == nil {
		// Position was closed by another operation while we were waiting for the lock
		e.logger.Warn("Close position warning - position closed by concurrent operation",
			zap.String("position_id", req.PositionID))
		return nil
	}

	// 7. Get or create user state
	contestState := e.state.GetOrCreateContest(req.ContestID)
	participant, err := e.validateParticipant(ctx, req.ContestID, req.UserID)
	if err != nil {
		e.logger.Error("Close position failed - failed to validate participant",
			zap.String("user_id", req.UserID),
			zap.String("contest_id", req.ContestID),
			zap.Error(err))
		return fmt.Errorf("failed to validate participant: %w", err)
	}
	if participant == nil {
		e.logger.Warn("Close position failed - user is not a participant",
			zap.String("user_id", req.UserID),
			zap.String("contest_id", req.ContestID))
		return fmt.Errorf("user is not a participant in this contest")
	}
	userState := contestState.GetOrCreateUser(req.UserID, participant.QtyTotal, participant.QtyAvailable, participant.TotalScore)

	// 8. Determine close quantity
	qtyToClose := req.QtyToClose
	if qtyToClose <= 0 || qtyToClose >= dbPos.QtyOpen {
		qtyToClose = dbPos.QtyOpen // Full close
	}
	isFullClose := qtyToClose >= dbPos.QtyOpen

	// 9. Get current market price for closing with freshness check
	// For LONG position (side=long): use bid price (selling)
	// For SHORT position (side=short): use ask price (buying)
	var closePrice float64
	positionSide := PositionSideToOrderSide(dbPos.Side)

	// Determine close side (opposite of position side)
	closeSide := OppositeSide(positionSide)

	// Get the appropriate price with age
	closePrice, priceAge, err := e.getMarketPriceWithAge(ctx, dbPos.Symbol, closeSide)
	if err != nil {
		e.logger.Warn("Close position failed - no price data available",
			zap.String("position_id", req.PositionID),
			zap.String("symbol", dbPos.Symbol),
			zap.String("asset_class", contest.AssetClass),
			zap.Error(err))
		return fmt.Errorf("No price data available for %s", dbPos.Symbol)
	}

	// Check price freshness for close operation
	maxCloseAge := e.getMaxPriceAge(contest.AssetClass, true)
	if maxCloseAge > 0 && priceAge > maxCloseAge {
		e.logger.Warn("Close position rejected due to stale price",
			zap.String("position_id", req.PositionID),
			zap.String("symbol", dbPos.Symbol),
			zap.String("asset_class", contest.AssetClass),
			zap.Float64("price_age_seconds", priceAge.Seconds()),
			zap.Float64("max_allowed_seconds", maxCloseAge.Seconds()))
		if e.metrics != nil {
			e.metrics.OrdersRejectedStalePrice.WithLabelValues(req.ContestID, "close_position").Inc()
		}
		return fmt.Errorf("price data is stale (age: %.1fs, max: %.1fs)", priceAge.Seconds(), maxCloseAge.Seconds())
	}

	// 10. Calculate realized P&L using the Tralent formula
	// Calculate proportional qty_used for this close
	var qtyUsedForClose int64
	if isFullClose {
		qtyUsedForClose = dbPos.QtyUsed
	} else {
		// Use float64 division with rounding to avoid integer truncation.
		// Integer division loses fractional qty_used across partial closes
		// (e.g., 100*3/7 = 42 truncated, but should be 43 via rounding).
		qtyUsedForClose = int64(math.Round(float64(dbPos.QtyUsed) * float64(qtyToClose) / float64(dbPos.QtyOpen)))
		if qtyUsedForClose <= 0 && qtyToClose > 0 {
			qtyUsedForClose = 1
		}
	}

	// Calculate trade score using decimal precision formula
	tradeScoreDecimal := calculateTradeScoreDecimal(dbPos.Side, dbPos.EntryPrice, closePrice, qtyUsedForClose)
	tradeScore := tradeScoreDecimal.Float64

	// Calculate realized P&L in currency units (not score)
	var realizedPnL float64
	if dbPos.Side == "long" {
		realizedPnL = (closePrice - dbPos.EntryPrice) * float64(qtyToClose)
	} else {
		realizedPnL = (dbPos.EntryPrice - closePrice) * float64(qtyToClose)
	}

	e.logger.Debug("Closing position",
		zap.String("position_id", req.PositionID),
		zap.Int64("qty_to_close", qtyToClose),
		zap.Bool("is_full_close", isFullClose),
		zap.Float64("price", closePrice),
		zap.Float64("score", tradeScore),
		zap.Float64("pnl", realizedPnL))

	// 11. Create synthetic close order and fill
	closeOrderID := uuid.New().String()
	fillID := uuid.New().String()
	fillTs := time.Now().UnixMilli()

	// 12. Update database atomically
	if isFullClose {
		// Full close
		newPositionRealizedScoreDec := decimal.NewFromFloat(dbPos.RealizedScore).Add(tradeScoreDecimal.Decimal)
		newQtyAvailable := userState.GetQtyAvailable() + dbPos.QtyUsed
		newRealizedScoreDec := userState.GetRealizedScoreDecimal().Add(tradeScoreDecimal.Decimal)

		err = WithTransaction(ctx, e.db, func(tx *sql.Tx) error {
			// Insert synthetic close order
			if err := insertOrderTx(ctx, tx, closeOrderID, req.ContestID, req.UserID, dbPos.Symbol,
				closeSide, contracts.OrderTypeMarket, qtyToClose, nil, nil, nil, nil); err != nil {
				return fmt.Errorf("insert close order: %w", err)
			}

			// Update order status to filled
			if err := updateOrderStatusTx(ctx, tx, closeOrderID, "filled", qtyToClose); err != nil {
				return fmt.Errorf("update order status: %w", err)
			}

			// Insert fill
			if err := InsertFillTx(ctx, tx, fillID, closeOrderID, req.ContestID, req.UserID, dbPos.Symbol,
				closeSide, qtyToClose, closePrice); err != nil {
				return fmt.Errorf("insert fill: %w", err)
			}

			// Store realized P&L on the fill
			if err := UpdateFillRealizedPnlTx(ctx, tx, fillID, tradeScore); err != nil {
				return fmt.Errorf("update fill realized_pnl: %w", err)
			}

			// Close the position
			if err := ClosePositionTx(ctx, tx, req.PositionID, newPositionRealizedScoreDec); err != nil {
				return fmt.Errorf("close position: %w", err)
			}

			// Update participant score and qty_available atomically
			if err := UpdateParticipantQtyAndScoreTx(ctx, tx, req.ContestID, req.UserID, newQtyAvailable, newRealizedScoreDec); err != nil {
				return fmt.Errorf("update participant: %w", err)
			}

			return nil
		})

		if err != nil {
			e.logger.Error("Close position failed - database transaction error",
				zap.String("position_id", req.PositionID),
				zap.Error(err))
			return fmt.Errorf("database transaction failed: %w", err)
		}

		// Update in-memory state after successful transaction (WAL-protected)
		walData := struct {
			ClosedPositionID string  `json:"closed_position_id"`
			Symbol           string  `json:"symbol"`
			TradeScore       float64 `json:"trade_score"`
			QtyReleased      int64   `json:"qty_released"`
		}{
			ClosedPositionID: req.PositionID,
			Symbol:           dbPos.Symbol,
			TradeScore:       tradeScore,
			QtyReleased:      dbPos.QtyUsed,
		}
		e.safeUpdateInMemoryState(ctx, WALOpClosePosition, req.ContestID, req.UserID, dbPos.Symbol, walData, func() error {
			userState.AddRealizedScoreDecimal(tradeScoreDecimal.Decimal)
			userState.ReleaseQty(dbPos.QtyUsed)
			userState.RemovePosition(dbPos.Symbol)
			return nil
		})

		// Remove TP/SL tracking (outside WAL — separate concern)
		e.pendingBook.RemovePositionTPSL(dbPos.Symbol, req.PositionID)

	} else {
		// Partial close
		remainingQty := dbPos.QtyOpen - qtyToClose
		newQtyUsed := dbPos.QtyUsed - qtyUsedForClose
		newPositionRealizedScore := dbPos.RealizedScore + tradeScore // float64 for WAL/PositionState compat
		newPositionRealizedScoreDec := decimal.NewFromFloat(dbPos.RealizedScore).Add(tradeScoreDecimal.Decimal)
		newQtyAvailable := userState.GetQtyAvailable() + qtyUsedForClose
		newUserRealizedScoreDec := userState.GetRealizedScoreDecimal().Add(tradeScoreDecimal.Decimal)

		err = WithTransaction(ctx, e.db, func(tx *sql.Tx) error {
			// Insert synthetic close order
			if err := insertOrderTx(ctx, tx, closeOrderID, req.ContestID, req.UserID, dbPos.Symbol,
				closeSide, contracts.OrderTypeMarket, qtyToClose, nil, nil, nil, nil); err != nil {
				return fmt.Errorf("insert close order: %w", err)
			}

			// Update order status to filled
			if err := updateOrderStatusTx(ctx, tx, closeOrderID, "filled", qtyToClose); err != nil {
				return fmt.Errorf("update order status: %w", err)
			}

			// Insert fill
			if err := InsertFillTx(ctx, tx, fillID, closeOrderID, req.ContestID, req.UserID, dbPos.Symbol,
				closeSide, qtyToClose, closePrice); err != nil {
				return fmt.Errorf("insert fill: %w", err)
			}

			// Store realized P&L on the fill
			if err := UpdateFillRealizedPnlTx(ctx, tx, fillID, tradeScore); err != nil {
				return fmt.Errorf("update fill realized_pnl: %w", err)
			}

			// Update position
			if err := UpdatePositionTx(ctx, tx, req.PositionID, remainingQty, dbPos.EntryPrice, newQtyUsed, newPositionRealizedScoreDec); err != nil {
				return fmt.Errorf("update position: %w", err)
			}

			// Update participant score and qty_available atomically
			if err := UpdateParticipantQtyAndScoreTx(ctx, tx, req.ContestID, req.UserID, newQtyAvailable, newUserRealizedScoreDec); err != nil {
				return fmt.Errorf("update participant: %w", err)
			}

			return nil
		})

		if err != nil {
			e.logger.Error("Close position failed - database transaction error (partial close)",
				zap.String("position_id", req.PositionID),
				zap.Error(err))
			return fmt.Errorf("database transaction failed: %w", err)
		}

		// Update in-memory state after successful transaction (WAL-protected)
		walData := struct {
			PositionID       string  `json:"position_id"`
			Symbol           string  `json:"symbol"`
			Side             string  `json:"side"`
			TradeScore       float64 `json:"trade_score"`
			QtyReleased      int64   `json:"qty_released"`
			RemainingQty     int64   `json:"remaining_qty"`
			NewQtyUsed       int64   `json:"new_qty_used"`
			NewRealizedScore float64 `json:"new_realized_score"`
			EntryPrice       float64 `json:"entry_price"`
		}{
			PositionID:       dbPos.PositionID,
			Symbol:           dbPos.Symbol,
			Side:             dbPos.Side,
			TradeScore:       tradeScore,
			QtyReleased:      qtyUsedForClose,
			RemainingQty:     remainingQty,
			NewQtyUsed:       newQtyUsed,
			NewRealizedScore: newPositionRealizedScore,
			EntryPrice:       dbPos.EntryPrice,
		}
		e.safeUpdateInMemoryState(ctx, WALOpUpdatePosition, req.ContestID, req.UserID, dbPos.Symbol, walData, func() error {
			userState.AddRealizedScoreDecimal(tradeScoreDecimal.Decimal)
			userState.ReleaseQty(qtyUsedForClose)

			userState.SetPosition(&PositionState{
				PositionID:    dbPos.PositionID,
				Symbol:        dbPos.Symbol,
				Side:          positionSide,
				QtyOpen:       remainingQty,
				EntryPrice:    dbPos.EntryPrice,
				QtyUsed:       newQtyUsed,
				RealizedScore: newPositionRealizedScore,
			})

			// Update pendingBook's tracked QtyOpen so TP/SL uses fresh qty
			e.pendingBook.UpdatePositionQty(dbPos.Symbol, dbPos.PositionID, remainingQty)
			return nil
		})
	}

	// 13. Create order request for event emission
	orderReq := &contracts.OrderRequest{
		OrderID:   closeOrderID,
		UserID:    req.UserID,
		ContestID: req.ContestID,
		Symbol:    dbPos.Symbol,
		Side:      closeSide,
		Type:      contracts.OrderTypeMarket,
		Qty:       qtyToClose,
	}

	// 14. Emit events
	e.emitFillEvent(ctx, fillID, orderReq, qtyToClose, closePrice, fillTs)
	e.emitPositionUpdate(ctx, req.ContestID, req.UserID, userState, closePrice)
	e.emitPnLDelta(ctx, req.ContestID, req.UserID, userState, tradeScore)
	e.emitPositionClosedEvent(ctx, req.PositionID, req.UserID, req.ContestID, dbPos.Symbol,
		positionSide, qtyToClose, closePrice, realizedPnL, tradeScore, req.CloseReason, fillTs)

	e.logger.Info("Position closed successfully",
		zap.String("position_id", req.PositionID),
		zap.Int64("qty", qtyToClose),
		zap.Float64("price", closePrice),
		zap.Float64("score", tradeScore),
		zap.String("reason", req.CloseReason))
	return nil
}

// insertOrderTx inserts a new order into the database (transaction-aware).
func insertOrderTx(ctx context.Context, tx *sql.Tx, orderID, contestID, userID, symbol string,
	side contracts.OrderSide, orderType contracts.OrderType, qty int64,
	limitPrice, stopPrice, takeProfit, stopLoss *float64) error {

	dbSide := OrderSideToDBOrderSide(side)

	dbType := "market"
	switch orderType {
	case contracts.OrderTypeLimit, contracts.OrderTypeBuyLimit, contracts.OrderTypeSellLimit:
		dbType = "limit"
	case contracts.OrderTypeStop, contracts.OrderTypeBuyStop, contracts.OrderTypeSellStop:
		dbType = "stop"
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO orders (order_id, contest_id, user_id, symbol, side, type, qty, limit_price, stop_price, take_profit, stop_loss, status)
		VALUES ($1, $2, $3, $4, $5::order_side, $6::order_type, $7, $8, $9, $10, $11, 'pending')
	`, orderID, contestID, userID, symbol, dbSide, dbType, qty, limitPrice, stopPrice, takeProfit, stopLoss)
	return err
}

// updateOrderStatusTx updates the status and qty_filled of an order (transaction-aware).
func updateOrderStatusTx(ctx context.Context, tx *sql.Tx, orderID string, status string, qtyFilled int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE orders
		SET status = $2::order_status, qty_filled = $3
		WHERE order_id = $1
	`, orderID, status, qtyFilled)
	return err
}

func (e *Engine) ProcessCancelOrder(ctx context.Context, req *contracts.CancelOrderRequest) error {
	e.logger.Info("Processing cancel order request",
		zap.String("order_id", req.OrderID),
		zap.String("user_id", req.UserID),
		zap.String("contest_id", req.ContestID),
		zap.String("reason", string(req.CancelReason)))

	// 1. Shard validation - reject requests for contests not assigned to this shard
	if e.shardingEnabled && e.shardedState != nil {
		if err := e.shardedState.RejectIfNotAssigned(req.ContestID); err != nil {
			e.logger.Warn("Cancel order rejected - wrong shard",
				zap.String("order_id", req.OrderID),
				zap.Error(err))
			return fmt.Errorf("wrong shard: %w", err)
		}
	}

	// 2. Get contest state from StateManager
	contestState := e.state.GetOrCreateContest(req.ContestID)

	// 3. Get user state
	userState, exists := contestState.GetUser(req.UserID)
	if !exists {
		// User state not found - order may have been processed elsewhere or already cancelled
		e.logger.Debug("Cancel order - user state not found, assuming order already processed",
			zap.String("order_id", req.OrderID))
		return nil
	}

	// 4. Check if order exists in user's PendingOrders map
	pendingOrder, orderExists := userState.GetPendingOrder(req.OrderID)
	if !orderExists {
		// Order not found in pending orders - might have been filled already
		e.logger.Debug("Cancel order - order not found in pending orders (may have been filled)",
			zap.String("order_id", req.OrderID))
		// Return success - idempotent handling
		return nil
	}

	// 5. Get the order details from pending order
	symbol := pendingOrder.Symbol
	qty := pendingOrder.Qty

	e.logger.Debug("Cancel order - found pending order",
		zap.String("order_id", req.OrderID),
		zap.String("symbol", symbol),
		zap.Int64("qty", qty))

	// 6. DB first: atomic transaction for cancel (order status + participant qty)
	newQtyAvailable := userState.GetQtyAvailable() + qty
	err := WithTransaction(ctx, e.db, func(tx *sql.Tx) error {
		if err := UpdateOrderStatusTx(ctx, tx, req.OrderID, "cancelled", 0); err != nil {
			return fmt.Errorf("update order status: %w", err)
		}
		if err := UpdateParticipantQtyAvailableTx(ctx, tx, req.ContestID, req.UserID, newQtyAvailable); err != nil {
			return fmt.Errorf("update participant qty: %w", err)
		}
		return nil
	})
	if err != nil {
		e.logger.Error("Cancel order DB transaction failed",
			zap.String("order_id", req.OrderID),
			zap.String("contest_id", req.ContestID),
			zap.String("user_id", req.UserID),
			zap.Error(err))
		return fmt.Errorf("cancel order transaction: %w", err)
	}

	// 7. Transaction committed — now safe to update in-memory state
	e.pendingBook.RemovePendingOrder(symbol, req.OrderID)
	userState.RemovePendingOrder(req.OrderID)
	userState.ReleaseQty(qty)

	// 11. Emit OrderCancelledEvent via Kafka
	e.emitOrderCancelledEvent(ctx, req.OrderID, req.UserID, req.ContestID, symbol, qty, req.CancelReason)

	e.logger.Info("Cancel order completed successfully",
		zap.String("order_id", req.OrderID),
		zap.Int64("qty_released", qty),
		zap.String("reason", string(req.CancelReason)))
	return nil
}

func (e *Engine) ProcessModifyTPSL(ctx context.Context, req *contracts.ModifyTPSLRequest) error {
	e.logger.Info("Processing modify TP/SL request",
		zap.String("position_id", req.PositionID),
		zap.String("user_id", req.UserID),
		zap.String("contest_id", req.ContestID),
		zap.Float64p("take_profit", req.TakeProfit),
		zap.Float64p("stop_loss", req.StopLoss))

	// 1. Shard validation - reject requests for contests not assigned to this shard
	if e.shardingEnabled && e.shardedState != nil {
		if err := e.shardedState.RejectIfNotAssigned(req.ContestID); err != nil {
			e.logger.Warn("Modify TP/SL rejected - wrong shard",
				zap.String("position_id", req.PositionID),
				zap.Error(err))
			return fmt.Errorf("wrong shard: %w", err)
		}
	}

	// 2. Validate contest is running
	contest, err := e.getContestCached(ctx, req.ContestID)
	if err != nil {
		e.logger.Error("Modify TP/SL failed - failed to get contest",
			zap.String("contest_id", req.ContestID),
			zap.Error(err))
		return fmt.Errorf("failed to get contest: %w", err)
	}
	if contest == nil {
		e.logger.Warn("Modify TP/SL failed - contest not found",
			zap.String("contest_id", req.ContestID))
		return fmt.Errorf("contest not found")
	}
	if contest.Status != "running" {
		e.logger.Warn("Modify TP/SL failed - contest not running",
			zap.String("contest_id", req.ContestID),
			zap.String("status", contest.Status))
		return fmt.Errorf("contest is not running (status: %s)", contest.Status)
	}

	// 3. Get the position from database to validate it exists and belongs to user
	dbPos, err := GetPositionByID(ctx, e.db, req.PositionID)
	if err != nil {
		e.logger.Error("Modify TP/SL failed - failed to get position",
			zap.String("position_id", req.PositionID),
			zap.Error(err))
		return fmt.Errorf("failed to get position: %w", err)
	}
	if dbPos == nil {
		// Position not found - may have been closed between request and processing
		e.logger.Warn("Modify TP/SL warning - position not found (may have been closed)",
			zap.String("position_id", req.PositionID))
		return nil // Idempotent handling - return success
	}

	// 4. Validate position ownership
	if dbPos.UserID != req.UserID {
		e.logger.Warn("Modify TP/SL failed - user does not own position",
			zap.String("user_id", req.UserID),
			zap.String("position_id", req.PositionID))
		return fmt.Errorf("position does not belong to user")
	}
	if dbPos.ContestID != req.ContestID {
		e.logger.Warn("Modify TP/SL failed - position not in contest",
			zap.String("position_id", req.PositionID),
			zap.String("contest_id", req.ContestID))
		return fmt.Errorf("position is not in specified contest")
	}

	// 5. Update database first (source of truth) before in-memory state
	if err := UpdatePositionTPSL(ctx, e.db, req.PositionID, req.TakeProfit, req.StopLoss); err != nil {
		e.logger.Error("Modify TP/SL failed - database update error",
			zap.String("position_id", req.PositionID),
			zap.Error(err))
		return fmt.Errorf("database update failed: %w", err)
	}

	// 6. Update pending book TP/SL tracking (only after DB succeeds)
	// If the position is already being tracked, update its TP/SL levels
	// If not tracked but TP or SL is set, add it to tracking
	if req.TakeProfit != nil || req.StopLoss != nil {
		// Determine position side for TP/SL tracking
		positionSide := PositionSideToOrderSide(dbPos.Side)

		// Check if position is already being tracked
		if !e.pendingBook.UpdatePositionTPSL(dbPos.Symbol, req.PositionID, req.TakeProfit, req.StopLoss) {
			// Position not tracked yet, add it
			e.pendingBook.AddPositionWithTPSL(&PositionWithTPSL{
				PositionID: req.PositionID,
				ContestID:  req.ContestID,
				UserID:     req.UserID,
				Symbol:     dbPos.Symbol,
				Side:       positionSide,
				QtyOpen:    dbPos.QtyOpen,
				EntryPrice: dbPos.EntryPrice,
				TakeProfit: req.TakeProfit,
				StopLoss:   req.StopLoss,
			})
			e.logger.Debug("Added position to TP/SL tracking",
				zap.String("position_id", req.PositionID),
				zap.Float64p("take_profit", req.TakeProfit),
				zap.Float64p("stop_loss", req.StopLoss))
		} else {
			e.logger.Debug("Updated TP/SL for position",
				zap.String("position_id", req.PositionID),
				zap.Float64p("take_profit", req.TakeProfit),
				zap.Float64p("stop_loss", req.StopLoss))
		}
	} else {
		// Both TP and SL are nil - remove from tracking
		e.pendingBook.UpdatePositionTPSL(dbPos.Symbol, req.PositionID, nil, nil)
		e.logger.Debug("Removed position from TP/SL tracking",
			zap.String("position_id", req.PositionID))
	}

	// 7. Get user state for position update emission
	contestState := e.state.GetOrCreateContest(req.ContestID)
	participant, err := e.validateParticipant(ctx, req.ContestID, req.UserID)
	if err != nil {
		e.logger.Warn("Modify TP/SL warning - failed to validate participant",
			zap.String("user_id", req.UserID),
			zap.Error(err))
		// Continue anyway - DB is already updated
	}

	var userState *UserState
	if participant != nil {
		userState = contestState.GetOrCreateUser(req.UserID, participant.QtyTotal, participant.QtyAvailable, participant.TotalScore)
	} else {
		// Try to get existing user state
		userState, _ = contestState.GetUser(req.UserID)
	}

	// 8. Emit position update event with new TP/SL values
	if userState != nil {
		// Get current mark price for the position
		markPrice := dbPos.EntryPrice
		if price, ok := e.priceBook.GetLast(dbPos.Symbol); ok {
			markPrice = price
		}
		e.emitPositionUpdate(ctx, req.ContestID, req.UserID, userState, markPrice)
	}

	// 9. Emit TP/SL modified event
	e.emitTPSLModifiedEvent(ctx, req)

	e.logger.Info("Modify TP/SL completed successfully",
		zap.String("position_id", req.PositionID),
		zap.Float64p("take_profit", req.TakeProfit),
		zap.Float64p("stop_loss", req.StopLoss))
	return nil
}
