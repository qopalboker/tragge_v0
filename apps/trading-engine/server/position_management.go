package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// PriceData represents cached price data from Redis.
type PriceData struct {
	Last float64 `json:"last"`
	Bid  float64 `json:"bid,omitempty"`
	Ask  float64 `json:"ask,omitempty"`
	Ts   int64   `json:"ts"`
}

// Deprecated: Use calculateTradeScoreDecimal for precise decimal calculations.
// calculateTradeScore calculates the trade score using Tralent formula.
// For LONG: pct_change = (exit_price - entry_price) / entry_price * 100
// For SHORT: pct_change = (entry_price - exit_price) / entry_price * 100
// trade_score = qty_used * pct_change
func calculateTradeScore(side string, entryPrice, exitPrice float64, qtyUsed int64) float64 {
	if entryPrice == 0 {
		return 0
	}
	var pctChange float64
	if side == "long" {
		pctChange = (exitPrice - entryPrice) / entryPrice * 100
	} else {
		pctChange = (entryPrice - exitPrice) / entryPrice * 100
	}
	return float64(qtyUsed) * pctChange
}

// calculateUnrealizedScore calculates the unrealized score for a single position.
// For LONG: unrealized_score = qty_used × ((exit_price - entry_price) / entry_price × 100)
// For SHORT: unrealized_score = qty_used × ((entry_price - exit_price) / entry_price × 100)
//
// IMPORTANT: For accurate mark-to-market valuation, pass the appropriate exit price:
//   - For LONG positions: pass bid price (you would sell at bid to exit)
//   - For SHORT positions: pass ask price (you would buy at ask to exit)
//
// Deprecated: Use UserState.CalculateUnrealizedScoreWithExitPrice for proper bid/ask handling.
func calculateUnrealizedScore(position *PositionState, exitPrice float64) float64 {
	if position == nil || position.EntryPrice == 0 || exitPrice == 0 {
		return 0
	}

	var pctChange float64
	if IsLong(position.Side) { // LONG - exit at bid
		pctChange = (exitPrice - position.EntryPrice) / position.EntryPrice * 100
	} else { // SHORT - exit at ask
		pctChange = (position.EntryPrice - exitPrice) / position.EntryPrice * 100
	}

	return float64(position.QtyUsed) * pctChange
}

// getMarketPrice gets the appropriate fill price for a market order.
// For BUY orders, returns the ask price. For SELL orders, returns the bid price.
// Falls back to Redis if price book doesn't have the data.
func (e *Engine) getMarketPrice(ctx context.Context, symbol string, side contracts.OrderSide) (float64, error) {
	// Try price book first (has bid/ask)
	if price, ok := e.priceBook.GetFillPrice(symbol, side); ok {
		return price, nil
	}

	// Fallback to Redis (uses real bid/ask when available, otherwise last price).
	// Redis data may be up to ~1s stale compared to the in-memory PriceBook.
	e.logger.Warn("PriceBook miss, falling back to Redis", zap.String("symbol", symbol))
	if e.metrics != nil {
		e.metrics.RedisPriceFallbackTotal.Inc()
	}
	pd, _, err := e.getPriceDataFromRedis(ctx, symbol)
	if err != nil {
		return 0, err
	}
	return fillPriceFromData(pd, side), nil
}

// getMarketPriceWithAge gets the appropriate fill price for a market order along with the price age.
// For BUY orders, returns the ask price. For SELL orders, returns the bid price.
// Falls back to Redis if price book doesn't have the data.
func (e *Engine) getMarketPriceWithAge(ctx context.Context, symbol string, side contracts.OrderSide) (float64, time.Duration, error) {
	// Try price book first (has bid/ask with timestamp)
	if price, age, ok := e.priceBook.GetFillPriceDirect(symbol, side); ok {
		return price, age, nil
	}

	// Fallback to Redis (uses real bid/ask when available, otherwise last price)
	pd, age, err := e.getPriceDataFromRedis(ctx, symbol)
	if err != nil {
		return 0, 0, err
	}
	return fillPriceFromData(pd, side), age, nil
}

// getMaxPriceAge returns the appropriate max price age for an operation given the asset class.
// isClose indicates whether this is a position close (true) or open (false).
// Falls back to the generic MaxPriceAgeMarket/MaxPriceAgePending config if the asset class
// is not crypto or forex.
func (e *Engine) getMaxPriceAge(assetClass string, isClose bool) time.Duration {
	switch strings.ToLower(assetClass) {
	case "crypto":
		if isClose {
			return e.config.MaxPriceAgeCloseCrypto
		}
		return e.config.MaxPriceAgeOpenCrypto
	case "forex":
		if isClose {
			return e.config.MaxPriceAgeCloseForex
		}
		return e.config.MaxPriceAgeOpenForex
	default:
		// For unknown asset classes, fall back to generic config
		if isClose {
			return e.config.MaxPriceAgePending
		}
		return e.config.MaxPriceAgeMarket
	}
}

// fillPriceFromData returns the appropriate fill price from PriceData based on order side.
// For BUY orders, returns ask (or last as fallback). For SELL orders, returns bid (or last).
func fillPriceFromData(pd *PriceData, side contracts.OrderSide) float64 {
	if side == contracts.OrderSideBuy {
		if pd.Ask > 0 {
			return pd.Ask
		}
		return pd.Last
	}
	// SELL
	if pd.Bid > 0 {
		return pd.Bid
	}
	return pd.Last
}

// exitPriceFromData returns the appropriate exit (mark-to-market) price from PriceData.
// For LONG positions (BUY side), returns bid (sell at bid to exit).
// For SHORT positions (SELL side), returns ask (buy at ask to exit).
func exitPriceFromData(pd *PriceData, positionSide contracts.OrderSide) float64 {
	if positionSide == contracts.OrderSideBuy { // LONG - exit at bid
		if pd.Bid > 0 {
			return pd.Bid
		}
		return pd.Last
	}
	// SHORT - exit at ask
	if pd.Ask > 0 {
		return pd.Ask
	}
	return pd.Last
}

// getCurrentPrice gets the current price for a symbol from Redis.
func (e *Engine) getCurrentPrice(ctx context.Context, symbol string) (float64, error) {
	price, _, err := e.getCurrentPriceWithAge(ctx, symbol)
	return price, err
}

// getCurrentPriceWithAge gets the current last price for a symbol from Redis with its age.
func (e *Engine) getCurrentPriceWithAge(ctx context.Context, symbol string) (float64, time.Duration, error) {
	pd, age, err := e.getPriceDataFromRedis(ctx, symbol)
	if err != nil {
		return 0, 0, err
	}
	return pd.Last, age, nil
}

// getPriceDataFromRedis fetches the full price data (bid/ask/last) from Redis.
func (e *Engine) getPriceDataFromRedis(ctx context.Context, symbol string) (*PriceData, time.Duration, error) {
	priceJSON, err := e.redis.HGet(ctx, "prices:latest", symbol).Result()
	if err == redis.Nil {
		return nil, 0, fmt.Errorf("no price available for symbol %s", symbol)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("redis error: %w", err)
	}

	var priceData PriceData
	if err := json.Unmarshal([]byte(priceJSON), &priceData); err != nil {
		return nil, 0, fmt.Errorf("parse price data: %w", err)
	}

	// Calculate price age (normalize timestamp to milliseconds for consistency)
	tsMillis := normalizeToMillis(priceData.Ts)
	priceAge := time.Since(time.UnixMilli(tsMillis))

	return &priceData, priceAge, nil
}

// PositionTxResult holds the results of a transactional position update,
// enabling the caller to apply in-memory state changes after commit.
type PositionTxResult struct {
	// Branch indicates which position update path was taken.
	Branch string // "new", "add", "close", "partial_close"

	// Common fields
	PositionID   string
	PositionSide string
	Symbol       string
	EntryPrice   float64

	// For "new" branch
	QtyOpen int64
	QtyUsed int64

	// For "add" branch
	TotalQty      int64
	NewEntryPrice float64
	NewQtyUsed    int64

	// For "close" and "partial_close" branches
	TradeScore               float64
	TradeScoreDecimal        decimal.Decimal
	QtyReleased              int64
	NewQtyAvailable          int64
	NewRealizedScore         float64
	NewPositionRealizedScore float64

	// For "close" with overflow
	OverflowQty        int64
	OverflowPositionID string

	// For "partial_close"
	RemainingQty int64
}

// updatePositionTx performs the database operations for a position update within a transaction.
// It does NOT update in-memory state — the caller must apply PositionTxResult after commit.
func (e *Engine) updatePositionTx(ctx context.Context, tx TxExecutor, order *contracts.OrderRequest,
	userState *UserState, fillPrice float64, fillQty int64, preReserved bool) (*PositionTxResult, error) {

	positionSide := string(OrderSideToPositionSide(order.Side))

	// Read position within the transaction for consistency
	dbPos, err := GetOpenPositionTx(ctx, tx, order.ContestID, order.UserID, order.Symbol)
	if err != nil {
		return nil, fmt.Errorf("get position: %w", err)
	}

	if dbPos == nil {
		// NEW position
		positionID := uuid.New().String()
		qtyUsed := fillQty

		if err := InsertPositionTx(ctx, tx, positionID, order.ContestID, order.UserID, order.Symbol,
			positionSide, fillQty, fillPrice, qtyUsed); err != nil {
			return nil, fmt.Errorf("insert position: %w", err)
		}

		return &PositionTxResult{
			Branch:       "new",
			PositionID:   positionID,
			PositionSide: positionSide,
			Symbol:       order.Symbol,
			QtyOpen:      fillQty,
			EntryPrice:   fillPrice,
			QtyUsed:      qtyUsed,
		}, nil
	}

	isSameSide := IsSameSide(dbPos.Side, order.Side)

	if isSameSide {
		// ADD to existing position
		totalQty := dbPos.QtyOpen + fillQty
		newEntryPrice := calculateWeightedAverageEntryDecimal(dbPos.EntryPrice, dbPos.QtyOpen, fillPrice, fillQty)
		newQtyUsed := dbPos.QtyUsed + fillQty

		if err := UpdatePositionTx(ctx, tx, dbPos.PositionID, totalQty, newEntryPrice, newQtyUsed, decimal.NewFromFloat(dbPos.RealizedScore)); err != nil {
			return nil, fmt.Errorf("update position: %w", err)
		}

		return &PositionTxResult{
			Branch:                   "add",
			PositionID:               dbPos.PositionID,
			PositionSide:             dbPos.Side,
			Symbol:                   order.Symbol,
			TotalQty:                 totalQty,
			NewEntryPrice:            newEntryPrice,
			NewQtyUsed:               newQtyUsed,
			NewPositionRealizedScore: dbPos.RealizedScore,
		}, nil
	}

	// OPPOSITE SIDE
	if fillQty >= dbPos.QtyOpen {
		// FULL CLOSE (possibly with overflow)
		overflowQty := fillQty - dbPos.QtyOpen

		tradeScoreDec := calculateTradeScoreDecimal(dbPos.Side, dbPos.EntryPrice, fillPrice, dbPos.QtyUsed)
		tradeScore := tradeScoreDec.Float64
		newPositionRealizedScore := dbPos.RealizedScore + tradeScore
		newPositionRealizedScoreDec := decimal.NewFromFloat(dbPos.RealizedScore).Add(tradeScoreDec.Decimal)
		// When preReserved is true, the caller (executeMarketOrder/executePendingOrder) already
		// called ReserveQty(fillQty) before entering updatePositionTx. The close portion
		// (min(fillQty, QtyOpen)) does not consume buying power, so we release it back.
		// When preReserved is false (executeTPSL), no reservation was made, so closePortion = 0.
		closePortion := int64(0)
		if preReserved {
			closePortion = fillQty - overflowQty // = min(fillQty, dbPos.QtyOpen)
		}
		newQtyAvailable := userState.GetQtyAvailable() + dbPos.QtyUsed + closePortion
		newRealizedScore := userState.GetRealizedScore() + tradeScore
		newRealizedScoreDec := userState.GetRealizedScoreDecimal().Add(tradeScoreDec.Decimal)

		if err := ClosePositionTx(ctx, tx, dbPos.PositionID, newPositionRealizedScoreDec); err != nil {
			return nil, fmt.Errorf("close position: %w", err)
		}

		if err := UpdateParticipantQtyAndScoreTx(ctx, tx, order.ContestID, order.UserID, newQtyAvailable, newRealizedScoreDec); err != nil {
			return nil, fmt.Errorf("update participant: %w", err)
		}

		var overflowPositionID string
		if overflowQty > 0 {
			overflowPositionID = uuid.New().String()
			if err := InsertPositionTx(ctx, tx, overflowPositionID, order.ContestID, order.UserID, order.Symbol,
				positionSide, overflowQty, fillPrice, overflowQty); err != nil {
				return nil, fmt.Errorf("insert overflow position: %w", err)
			}
		}

		return &PositionTxResult{
			Branch:                   "close",
			PositionID:               dbPos.PositionID,
			PositionSide:             positionSide,
			Symbol:                   order.Symbol,
			EntryPrice:               fillPrice,
			TradeScore:               tradeScore,
			TradeScoreDecimal:        tradeScoreDec.Decimal,
			QtyReleased:              dbPos.QtyUsed + closePortion,
			NewQtyAvailable:          newQtyAvailable,
			NewRealizedScore:         newRealizedScore,
			NewPositionRealizedScore: newPositionRealizedScore,
			OverflowQty:              overflowQty,
			OverflowPositionID:       overflowPositionID,
		}, nil
	}

	// PARTIAL CLOSE
	closingQty := fillQty
	remainingQty := dbPos.QtyOpen - closingQty
	// Use float64 division with rounding to match ProcessClosePosition behavior
	// and prevent integer truncation losing qty across partial closes.
	qtyUsedForClose := int64(math.Round(float64(dbPos.QtyUsed) * float64(closingQty) / float64(dbPos.QtyOpen)))
	if qtyUsedForClose <= 0 && closingQty > 0 {
		qtyUsedForClose = 1
	}

	tradeScoreDec := calculateTradeScoreDecimal(dbPos.Side, dbPos.EntryPrice, fillPrice, qtyUsedForClose)
	tradeScore := tradeScoreDec.Float64
	newQtyUsed := dbPos.QtyUsed - qtyUsedForClose
	newPositionRealizedScore := dbPos.RealizedScore + tradeScore
	newPositionRealizedScoreDec := decimal.NewFromFloat(dbPos.RealizedScore).Add(tradeScoreDec.Decimal)
	// When preReserved is true, the caller already reserved fillQty, so release it back
	// since closing doesn't consume buying power. When false (executeTPSL), skip.
	orderReservationRelease := int64(0)
	if preReserved {
		orderReservationRelease = fillQty
	}
	newQtyAvailable := userState.GetQtyAvailable() + qtyUsedForClose + orderReservationRelease
	newUserRealizedScore := userState.GetRealizedScore() + tradeScore
	newUserRealizedScoreDec := userState.GetRealizedScoreDecimal().Add(tradeScoreDec.Decimal)

	if err := UpdatePositionTx(ctx, tx, dbPos.PositionID, remainingQty, dbPos.EntryPrice, newQtyUsed, newPositionRealizedScoreDec); err != nil {
		return nil, fmt.Errorf("update position: %w", err)
	}

	if err := UpdateParticipantQtyAndScoreTx(ctx, tx, order.ContestID, order.UserID, newQtyAvailable, newUserRealizedScoreDec); err != nil {
		return nil, fmt.Errorf("update participant: %w", err)
	}

	return &PositionTxResult{
		Branch:                   "partial_close",
		PositionID:               dbPos.PositionID,
		PositionSide:             dbPos.Side,
		Symbol:                   order.Symbol,
		EntryPrice:               dbPos.EntryPrice,
		TradeScore:               tradeScore,
		TradeScoreDecimal:        tradeScoreDec.Decimal,
		QtyReleased:              qtyUsedForClose + orderReservationRelease,
		RemainingQty:             remainingQty,
		NewQtyUsed:               newQtyUsed,
		NewQtyAvailable:          newQtyAvailable,
		NewRealizedScore:         newUserRealizedScore,
		NewPositionRealizedScore: newPositionRealizedScore,
	}, nil
}

// applyPositionResultToMemory applies post-commit in-memory state updates based on PositionTxResult.
// Returns the delta score (realized score change) for PnL event emission.
func (e *Engine) applyPositionResultToMemory(ctx context.Context, order *contracts.OrderRequest,
	userState *UserState, result *PositionTxResult, fillPrice float64) float64 {

	var deltaScore float64

	switch result.Branch {
	case "new":
		walData := PositionUpdateData{
			PositionID: result.PositionID,
			Symbol:     result.Symbol,
			Side:       result.PositionSide,
			QtyOpen:    result.QtyOpen,
			EntryPrice: result.EntryPrice,
			QtyUsed:    result.QtyUsed,
		}
		e.safeUpdateInMemoryState(ctx, WALOpCreatePosition, order.ContestID, order.UserID, result.Symbol, walData, func() error {
			userState.SetPosition(&PositionState{
				PositionID:    result.PositionID,
				Symbol:        result.Symbol,
				Side:          order.Side,
				QtyOpen:       result.QtyOpen,
				EntryPrice:    result.EntryPrice,
				QtyUsed:       result.QtyUsed,
				RealizedScore: 0,
			})
			return nil
		})

	case "add":
		walData := PositionUpdateData{
			PositionID:    result.PositionID,
			Symbol:        result.Symbol,
			Side:          result.PositionSide,
			QtyOpen:       result.TotalQty,
			EntryPrice:    result.NewEntryPrice,
			QtyUsed:       result.NewQtyUsed,
			RealizedScore: result.NewPositionRealizedScore,
		}
		e.safeUpdateInMemoryState(ctx, WALOpUpdatePosition, order.ContestID, order.UserID, result.Symbol, walData, func() error {
			userState.SetPosition(&PositionState{
				PositionID:    result.PositionID,
				Symbol:        result.Symbol,
				Side:          order.Side,
				QtyOpen:       result.TotalQty,
				EntryPrice:    result.NewEntryPrice,
				QtyUsed:       result.NewQtyUsed,
				RealizedScore: result.NewPositionRealizedScore,
			})
			return nil
		})

	case "close":
		deltaScore = result.TradeScore
		walData := struct {
			ClosedPositionID string  `json:"closed_position_id"`
			Symbol           string  `json:"symbol"`
			TradeScore       float64 `json:"trade_score"`
			QtyReleased      int64   `json:"qty_released"`
			NewQtyAvailable  int64   `json:"new_qty_available"`
			NewRealizedScore float64 `json:"new_realized_score"`
			OverflowQty      int64   `json:"overflow_qty,omitempty"`
			NewPositionID    string  `json:"new_position_id,omitempty"`
			NewPositionSide  string  `json:"new_position_side,omitempty"`
		}{
			ClosedPositionID: result.PositionID,
			Symbol:           result.Symbol,
			TradeScore:       result.TradeScore,
			QtyReleased:      result.QtyReleased,
			NewQtyAvailable:  result.NewQtyAvailable,
			NewRealizedScore: result.NewRealizedScore,
			OverflowQty:      result.OverflowQty,
			NewPositionID:    result.OverflowPositionID,
			NewPositionSide:  result.PositionSide,
		}
		e.safeUpdateInMemoryState(ctx, WALOpClosePosition, order.ContestID, order.UserID, result.Symbol, walData, func() error {
			userState.AddRealizedScoreDecimal(result.TradeScoreDecimal)
			userState.ReleaseQty(result.QtyReleased)
			userState.RemovePosition(result.Symbol)

			if result.OverflowQty > 0 {
				userState.SetPosition(&PositionState{
					PositionID:    result.OverflowPositionID,
					Symbol:        result.Symbol,
					Side:          order.Side,
					QtyOpen:       result.OverflowQty,
					EntryPrice:    result.EntryPrice,
					QtyUsed:       result.OverflowQty,
					RealizedScore: 0,
				})
			}
			return nil
		})

	case "partial_close":
		deltaScore = result.TradeScore
		orderSide := PositionSideToOrderSide(result.PositionSide)
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
			PositionID:       result.PositionID,
			Symbol:           result.Symbol,
			Side:             result.PositionSide,
			TradeScore:       result.TradeScore,
			QtyReleased:      result.QtyReleased,
			RemainingQty:     result.RemainingQty,
			NewQtyUsed:       result.NewQtyUsed,
			NewRealizedScore: result.NewPositionRealizedScore,
			EntryPrice:       result.EntryPrice,
		}
		e.safeUpdateInMemoryState(ctx, WALOpUpdatePosition, order.ContestID, order.UserID, result.Symbol, walData, func() error {
			userState.AddRealizedScoreDecimal(result.TradeScoreDecimal)
			userState.ReleaseQty(result.QtyReleased)

			userState.SetPosition(&PositionState{
				PositionID:    result.PositionID,
				Symbol:        result.Symbol,
				Side:          orderSide,
				QtyOpen:       result.RemainingQty,
				EntryPrice:    result.EntryPrice,
				QtyUsed:       result.NewQtyUsed,
				RealizedScore: result.NewPositionRealizedScore,
			})

			// Update pendingBook's tracked QtyOpen so TP/SL uses fresh qty
			e.pendingBook.UpdatePositionQty(result.Symbol, result.PositionID, result.RemainingQty)
			return nil
		})
	}

	return deltaScore
}

// trackPositionTPSL adds or updates TP/SL tracking for a position.
func (e *Engine) trackPositionTPSL(contestID, userID, symbol string, side contracts.OrderSide, tp, sl *float64) {
	if tp == nil && sl == nil {
		return
	}

	// Get the current position from the state
	contestState := e.state.GetOrCreateContest(contestID)
	userState, exists := contestState.GetUser(userID)
	if !exists {
		return
	}

	pos, posExists := userState.GetPosition(symbol)
	if !posExists {
		return
	}

	// Add to pending book for TP/SL tracking
	e.pendingBook.AddPositionWithTPSL(&PositionWithTPSL{
		PositionID: pos.PositionID,
		ContestID:  contestID,
		UserID:     userID,
		Symbol:     symbol,
		Side:       side,
		QtyOpen:    pos.QtyOpen,
		EntryPrice: pos.EntryPrice,
		TakeProfit: tp,
		StopLoss:   sl,
	})

	e.logger.Debug("TP/SL tracking added for position",
		zap.String("position_id", pos.PositionID),
		zap.Float64p("take_profit", tp),
		zap.Float64p("stop_loss", sl))
}
