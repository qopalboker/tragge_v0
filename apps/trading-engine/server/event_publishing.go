package server

import (
	"context"
	"encoding/json"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/shopspring/decimal"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// rejectOrder sends a rejection acknowledgment.
func (e *Engine) rejectOrder(ctx context.Context, order *contracts.OrderRequest, reason string) error {
	e.logger.Warn("Order rejected",
		zap.String("order_id", order.OrderID),
		zap.String("user_id", order.UserID),
		zap.String("contest_id", order.ContestID),
		zap.String("reason", reason))
	return e.acknowledgeOrder(ctx, order, contracts.OrderStatusRejected, &reason)
}

// rejectOrderRateLimited sends a rejection with rate limit metadata.
func (e *Engine) rejectOrderRateLimited(ctx context.Context, order *contracts.OrderRequest, result RateLimitResult) error {
	reason := "RATE_LIMITED"

	// Determine the window and limit based on scope
	var window string
	var limit int
	switch result.Scope {
	case RateLimitScopeUser:
		window = "1s"
		limit = e.config.RateLimits.UserPerSecond
	case RateLimitScopeContest:
		window = "1s"
		limit = e.config.RateLimits.ContestPerSecond
	case RateLimitScopeGlobal:
		window = "1s"
		limit = e.config.RateLimits.GlobalPerSecond
	default:
		window = "1s"
		limit = 10
	}

	rateLimit := &contracts.RateLimitInfo{
		Scope:        string(result.Scope),
		Limit:        limit,
		Window:       window,
		RetryAfterMs: result.RetryAfter.Milliseconds(),
	}

	e.logger.Warn("Order rejected due to rate limit",
		zap.String("order_id", order.OrderID),
		zap.String("user_id", order.UserID),
		zap.String("contest_id", order.ContestID),
		zap.String("scope", string(result.Scope)),
		zap.Int64("retry_after_ms", rateLimit.RetryAfterMs))

	e.emitOrderAckWithUser(ctx, order, contracts.OrderStatusRejected, &reason, rateLimit)
	return nil
}

// acknowledgeOrder sends an order acknowledgment.
func (e *Engine) acknowledgeOrder(ctx context.Context, order *contracts.OrderRequest, status contracts.OrderStatus, reason *string) error {
	e.emitOrderAck(ctx, order.OrderID, status, reason)
	return nil
}

// OrderAckWithUser extends OrderAck with user_id for WebSocket routing.
type OrderAckWithUser struct {
	contracts.OrderAck
	UserID string `json:"user_id"`
}

// emitOrderAck publishes an OrderAck event to Kafka.
func (e *Engine) emitOrderAck(ctx context.Context, orderID string, status contracts.OrderStatus, reason *string) {
	ack := &contracts.OrderAck{
		OrderID: orderID,
		Status:  status,
		Reason:  reason,
	}

	data, err := json.Marshal(ack)
	if err != nil {
		e.logger.Error("Failed to marshal OrderAck",
			zap.String("order_id", orderID),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: e.config.OrderAcksTopic,
		Key:   []byte(orderID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish OrderAck",
				zap.String("order_id", orderID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}

// emitOrderAckWithUser publishes an OrderAck with user_id for WebSocket routing.
func (e *Engine) emitOrderAckWithUser(ctx context.Context, order *contracts.OrderRequest, status contracts.OrderStatus, reason *string, rateLimit *contracts.RateLimitInfo) {
	ack := &OrderAckWithUser{
		OrderAck: contracts.OrderAck{
			OrderID:   order.OrderID,
			Status:    status,
			Reason:    reason,
			RateLimit: rateLimit,
		},
		UserID: order.UserID,
	}

	data, err := json.Marshal(ack)
	if err != nil {
		e.logger.Error("Failed to marshal OrderAckWithUser",
			zap.String("order_id", order.OrderID),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: e.config.OrderAcksTopic,
		Key:   []byte(order.OrderID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish OrderAckWithUser",
				zap.String("order_id", order.OrderID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}

// OrderRejectEvent represents a detailed order rejection event.
type OrderRejectEvent struct {
	OrderID       string  `json:"order_id"`
	UserID        string  `json:"user_id"`
	ContestID     string  `json:"contest_id"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	OrderType     string  `json:"order_type"`
	Qty           int64   `json:"qty"`
	RejectReason  string  `json:"reject_reason"`
	PriceAgeSecs  float64 `json:"price_age_seconds,omitempty"`
	MaxAllowedAge float64 `json:"max_allowed_age_seconds,omitempty"`
	Ts            int64   `json:"ts"`
}

// emitOrderRejectEvent publishes a detailed order rejection event to Kafka.
func (e *Engine) emitOrderRejectEvent(ctx context.Context, order *contracts.OrderRequest, reason string, priceAge time.Duration, maxAllowedAge time.Duration) {
	event := &OrderRejectEvent{
		OrderID:       order.OrderID,
		UserID:        order.UserID,
		ContestID:     order.ContestID,
		Symbol:        order.Symbol,
		Side:          string(order.Side),
		OrderType:     string(order.Type),
		Qty:           order.Qty,
		RejectReason:  reason,
		PriceAgeSecs:  priceAge.Seconds(),
		MaxAllowedAge: maxAllowedAge.Seconds(),
		Ts:            time.Now().UnixMilli(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		e.logger.Error("Failed to marshal OrderRejectEvent",
			zap.String("order_id", order.OrderID),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: e.config.OrderAcksTopic,
		Key:   []byte(order.OrderID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish OrderRejectEvent",
				zap.String("order_id", order.OrderID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}

// emitFillEvent publishes a FillEvent to Kafka.
func (e *Engine) emitFillEvent(ctx context.Context, fillID string, order *contracts.OrderRequest, qty int64, fillPrice float64, ts int64) {
	fill := &contracts.FillEvent{
		FillID:    fillID,
		OrderID:   order.OrderID,
		UserID:    order.UserID,
		ContestID: order.ContestID,
		Symbol:    order.Symbol,
		Side:      order.Side,
		Qty:       qty,
		FillPrice: fillPrice,
		Ts:        ts,
	}

	data, err := json.Marshal(fill)
	if err != nil {
		e.logger.Error("Failed to marshal FillEvent",
			zap.String("fill_id", fillID),
			zap.String("order_id", order.OrderID),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: e.config.FillsTopic,
		Key:   []byte(order.ContestID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish FillEvent",
				zap.String("fill_id", fillID),
				zap.String("order_id", order.OrderID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}

// emitPositionUpdate publishes a PositionUpdate to Kafka.
func (e *Engine) emitPositionUpdate(ctx context.Context, contestID, userID string, userState *UserState, markPrice float64) {
	positions := userState.GetAllPositions()
	contractPositions := make([]contracts.Position, 0, len(positions))

	for _, pos := range positions {
		// Get exit price for this specific position's symbol.
		// Use bid for LONG (you sell at bid to exit) and ask for SHORT (you buy at ask to exit),
		// consistent with BroadcastUnrealizedScores to avoid score jumps after fills.
		posMarkPrice := markPrice // default to passed markPrice
		if exitPrice, ok := e.priceBook.GetExitPrice(pos.Symbol, pos.Side); ok {
			posMarkPrice = exitPrice
		}

		// Calculate unrealized P&L percentage
		var unrealizedPnLPct float64
		if pos.EntryPrice > 0 {
			if IsLong(pos.Side) { // long
				unrealizedPnLPct = (posMarkPrice - pos.EntryPrice) / pos.EntryPrice * 100
			} else { // short
				unrealizedPnLPct = (pos.EntryPrice - posMarkPrice) / pos.EntryPrice * 100
			}
		}

		// Calculate unrealized score using Tralent formula: qty_used * pct_change
		unrealizedScore := float64(pos.QtyUsed) * unrealizedPnLPct

		realizedPnLPct := 0.0
		if pos.QtyUsed > 0 && pos.EntryPrice > 0 {
			realizedPnLPct = pos.RealizedScore / (pos.EntryPrice * float64(pos.QtyUsed)) * 100
		}

		contractPositions = append(contractPositions, contracts.Position{
			PositionID:       pos.PositionID,
			Symbol:           pos.Symbol,
			Side:             pos.Side,
			Qty:              pos.QtyOpen,
			EntryPrice:       pos.EntryPrice,
			MarkPrice:        posMarkPrice,
			UnrealizedPnLPct: unrealizedPnLPct,
			RealizedPnLPct:   realizedPnLPct,
			QtyUsed:          pos.QtyUsed,
			UnrealizedScore:  unrealizedScore,
		})
	}

	update := &contracts.PositionUpdate{
		UserID:    userID,
		ContestID: contestID,
		Positions: contractPositions,
	}

	data, err := json.Marshal(update)
	if err != nil {
		e.logger.Error("Failed to marshal PositionUpdate",
			zap.String("contest_id", contestID),
			zap.String("user_id", userID),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: e.config.PositionsTopic,
		Key:   []byte(contestID + ":" + userID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish PositionUpdate",
				zap.String("contest_id", contestID),
				zap.String("user_id", userID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}

// emitPnLDelta publishes a PnLDelta to Kafka with realized and unrealized scores.
// Includes both float64 (backward compat) and decimal string (high precision) fields.
func (e *Engine) emitPnLDelta(ctx context.Context, contestID, userID string, userState *UserState, deltaScore float64) {
	realizedScore := userState.GetRealizedScore()

	// Get exit price function that returns appropriate price based on position side.
	// For LONG positions, use bid (you sell at bid to exit).
	// For SHORT positions, use ask (you buy at ask to exit).
	// Consistent with BroadcastUnrealizedScores to avoid score jumps after fills.
	getExitPriceFunc := func(symbol string, positionSide contracts.OrderSide) (float64, bool) {
		if exitPrice, ok := e.priceBook.GetExitPrice(symbol, positionSide); ok {
			return exitPrice, true
		}
		// Fallback to Redis (uses real bid/ask when available)
		pd, _, err := e.getPriceDataFromRedis(ctx, symbol)
		if err != nil {
			return 0, false
		}
		return exitPriceFromData(pd, positionSide), true
	}

	unrealizedScore := userState.CalculateUnrealizedScoreWithExitPrice(getExitPriceFunc)
	totalScore := realizedScore + unrealizedScore

	// Calculate decimal versions for high-precision fields
	realizedScoreDecimal := userState.GetRealizedScoreDecimal()
	unrealizedScoreDecimal := userState.CalculateUnrealizedScoreDecimalWithExitPrice(getExitPriceFunc)
	totalScoreDecimal := realizedScoreDecimal.Add(unrealizedScoreDecimal)
	deltaScoreDecimal := decimal.NewFromFloat(deltaScore)

	delta := &contracts.PnLDelta{
		UserID:          userID,
		ContestID:       contestID,
		DeltaScore:      deltaScore,
		RealizedScore:   realizedScore,
		UnrealizedScore: unrealizedScore,
		TotalScore:      totalScore,
		Ts:              time.Now().UnixMilli(),
		SeqNum:          e.pnlSeqNum.Add(1),
		// High-precision decimal string fields (8 decimal places)
		DeltaScoreDecimal:      deltaScoreDecimal.StringFixed(8),
		RealizedScoreDecimal:   realizedScoreDecimal.StringFixed(8),
		UnrealizedScoreDecimal: unrealizedScoreDecimal.StringFixed(8),
		TotalScoreDecimal:      totalScoreDecimal.StringFixed(8),
	}

	data, err := json.Marshal(delta)
	if err != nil {
		e.logger.Error("Failed to marshal PnLDelta",
			zap.String("contest_id", contestID),
			zap.String("user_id", userID),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: e.config.PnLDeltasTopic,
		Key:   []byte(contestID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish PnLDelta",
				zap.String("contest_id", contestID),
				zap.String("user_id", userID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}

// emitPositionClosedEvent publishes a PositionClosedEvent to Kafka.
func (e *Engine) emitPositionClosedEvent(ctx context.Context, positionID, userID, contestID, symbol string,
	side contracts.OrderSide, qtyClosed int64, closePrice, realizedPnL, realizedScore float64, closeReason string, ts int64) {

	event := &contracts.PositionClosedEvent{
		PositionID:    positionID,
		UserID:        userID,
		ContestID:     contestID,
		Symbol:        symbol,
		Side:          side,
		QtyClosed:     qtyClosed,
		ClosePrice:    closePrice,
		RealizedPnL:   realizedPnL,
		RealizedScore: realizedScore,
		CloseReason:   closeReason,
		Ts:            ts,
	}

	data, err := json.Marshal(event)
	if err != nil {
		e.logger.Error("Failed to marshal PositionClosedEvent",
			zap.String("position_id", positionID),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: e.config.PositionClosedTopic,
		Key:   []byte(contestID + ":" + userID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish PositionClosedEvent",
				zap.String("position_id", positionID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}

// emitOrderCancelledEvent publishes an OrderCancelledEvent to Kafka.
func (e *Engine) emitOrderCancelledEvent(ctx context.Context, orderID, userID, contestID, symbol string,
	qtyReleased int64, cancelReason contracts.CancelReason) {

	event := &contracts.OrderCancelledEvent{
		OrderID:      orderID,
		UserID:       userID,
		ContestID:    contestID,
		Symbol:       symbol,
		QtyReleased:  qtyReleased,
		CancelReason: cancelReason,
		Ts:           time.Now().UnixMilli(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		e.logger.Error("Failed to marshal OrderCancelledEvent",
			zap.String("order_id", orderID),
			zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: e.config.OrderCancelledTopic,
		Key:   []byte(contestID + ":" + userID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish OrderCancelledEvent",
				zap.String("order_id", orderID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}

// emitTPSLModifiedEvent publishes a TP/SL modified event to Kafka.
func (e *Engine) emitTPSLModifiedEvent(ctx context.Context, req *contracts.ModifyTPSLRequest) {
	event := map[string]interface{}{
		"event_type":  "tpsl_modified",
		"position_id": req.PositionID,
		"user_id":     req.UserID,
		"contest_id":  req.ContestID,
		"symbol":      req.Symbol,
		"take_profit": req.TakeProfit,
		"stop_loss":   req.StopLoss,
		"ts":          time.Now().UnixMilli(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		e.logger.Error("Failed to marshal TPSLModifiedEvent",
			zap.String("position_id", req.PositionID),
			zap.Error(err))
		return
	}

	// Publish to positions topic so trade-bff can forward to WebSocket clients
	record := &kgo.Record{
		Topic: e.config.PositionsTopic,
		Key:   []byte(req.ContestID + ":" + req.UserID),
		Value: data,
	}

	e.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			e.logger.Error("Failed to publish TPSLModifiedEvent",
				zap.String("position_id", req.PositionID),
				zap.Error(err))
			if e.metrics != nil {
				e.metrics.KafkaProduceFailures.WithLabelValues(record.Topic).Inc()
			}
		}
	})
}
