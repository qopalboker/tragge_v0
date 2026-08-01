package server

import (
	"sync"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/shopspring/decimal"
)

// UserState tracks per-user trading state within a contest.
type UserState struct {
	QtyTotal      int64                     // Total quantity allocated
	QtyAvailable  int64                     // Available for trading
	RealizedScore float64                   // Sum of all realized trade scores (Tralent formula) - deprecated, use RealizedScoreDecimal
	Positions     map[string]*PositionState // symbol -> position
	PendingOrders map[string]*PendingOrder  // orderID -> pending order
	mu            sync.RWMutex

	// High-precision decimal score (8 decimal places)
	RealizedScoreDecimal decimal.Decimal
}

// PositionState tracks a single position for a user.
type PositionState struct {
	PositionID    string
	Symbol        string
	Side          contracts.OrderSide // BUY = long, SELL = short
	QtyOpen       int64
	EntryPrice    float64
	QtyUsed       int64   // Buying power used
	RealizedScore float64 // Realized P&L - deprecated, use RealizedScoreDecimal

	// High-precision decimal fields (8 decimal places)
	EntryPriceDecimal    decimal.Decimal
	RealizedScoreDecimal decimal.Decimal
}

// PendingOrder tracks a pending order.
type PendingOrder struct {
	OrderID    string
	Symbol     string
	Side       contracts.OrderSide
	Type       contracts.OrderType
	Qty        int64
	QtyFilled  int64
	LimitPrice *float64
	StopPrice  *float64
}

// ContestState tracks all user states within a contest.
type ContestState struct {
	ContestID string
	Users     map[string]*UserState // userID -> UserState
	mu        sync.RWMutex
}

// StateManager manages all contest states.
type StateManager struct {
	contests map[string]*ContestState // contestID -> ContestState
	mu       sync.RWMutex
}

// NewStateManager creates a new state manager.
func NewStateManager() *StateManager {
	return &StateManager{
		contests: make(map[string]*ContestState),
	}
}

// GetOrCreateContest returns the contest state, creating if needed.
func (sm *StateManager) GetOrCreateContest(contestID string) *ContestState {
	sm.mu.RLock()
	cs, exists := sm.contests[contestID]
	sm.mu.RUnlock()

	if exists {
		return cs
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check after acquiring write lock
	if cs, exists = sm.contests[contestID]; exists {
		return cs
	}

	cs = &ContestState{
		ContestID: contestID,
		Users:     make(map[string]*UserState),
	}
	sm.contests[contestID] = cs
	return cs
}

// RemoveContest removes a contest's state from the manager.
// Should be called after a contest reaches a final state (completed/cancelled)
// and a grace period has elapsed to allow in-flight operations to complete.
func (sm *StateManager) RemoveContest(contestID string) {
	sm.mu.Lock()
	delete(sm.contests, contestID)
	sm.mu.Unlock()
}

// GetOrCreateUser returns the user state, creating if needed.
func (cs *ContestState) GetOrCreateUser(userID string, qtyTotal, qtyAvailable int64, realizedScore float64) *UserState {
	cs.mu.RLock()
	us, exists := cs.Users[userID]
	cs.mu.RUnlock()

	if exists {
		return us
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Double-check after acquiring write lock
	if us, exists = cs.Users[userID]; exists {
		return us
	}

	us = &UserState{
		QtyTotal:             qtyTotal,
		QtyAvailable:         qtyAvailable,
		RealizedScore:        realizedScore,
		RealizedScoreDecimal: decimal.NewFromFloat(realizedScore),
		Positions:            make(map[string]*PositionState),
		PendingOrders:        make(map[string]*PendingOrder),
	}
	cs.Users[userID] = us
	return us
}

// GetUser returns the user state if exists.
func (cs *ContestState) GetUser(userID string) (*UserState, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	us, exists := cs.Users[userID]
	return us, exists
}

// UpdateQtyAvailable updates the user's available quantity atomically.
func (us *UserState) UpdateQtyAvailable(delta int64) int64 {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.QtyAvailable += delta
	return us.QtyAvailable
}

// ReserveQty reserves quantity for an order, returning success.
func (us *UserState) ReserveQty(qty int64) bool {
	us.mu.Lock()
	defer us.mu.Unlock()
	if us.QtyAvailable >= qty {
		us.QtyAvailable -= qty
		return true
	}
	return false
}

// ReleaseQty releases reserved quantity back to available.
func (us *UserState) ReleaseQty(qty int64) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.QtyAvailable += qty
}

// GetPosition returns the position for a symbol if exists.
func (us *UserState) GetPosition(symbol string) (*PositionState, bool) {
	us.mu.RLock()
	defer us.mu.RUnlock()
	pos, exists := us.Positions[symbol]
	return pos, exists
}

// SetPosition sets or updates a position.
func (us *UserState) SetPosition(pos *PositionState) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.Positions[pos.Symbol] = pos
}

// RemovePosition removes a position.
func (us *UserState) RemovePosition(symbol string) {
	us.mu.Lock()
	defer us.mu.Unlock()
	delete(us.Positions, symbol)
}

// GetAllPositions returns a copy of all positions.
func (us *UserState) GetAllPositions() []*PositionState {
	us.mu.RLock()
	defer us.mu.RUnlock()
	positions := make([]*PositionState, 0, len(us.Positions))
	for _, pos := range us.Positions {
		// Copy to avoid race conditions
		posCopy := *pos
		positions = append(positions, &posCopy)
	}
	return positions
}

// UpdateRealizedScore updates the realized score atomically.
// Deprecated: Use UpdateRealizedScoreDecimal for better precision.
func (us *UserState) UpdateRealizedScore(newScore float64) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.RealizedScore = newScore
	us.RealizedScoreDecimal = decimal.NewFromFloat(newScore)
}

// UpdateRealizedScoreDecimal updates the realized score with decimal precision.
func (us *UserState) UpdateRealizedScoreDecimal(newScore decimal.Decimal) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.RealizedScoreDecimal = newScore
	us.RealizedScore, _ = newScore.Float64()
}

// GetRealizedScore returns the current realized score.
// Deprecated: Use GetRealizedScoreDecimal for better precision.
func (us *UserState) GetRealizedScore() float64 {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.RealizedScore
}

// GetRealizedScoreDecimal returns the current realized score with decimal precision.
func (us *UserState) GetRealizedScoreDecimal() decimal.Decimal {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.RealizedScoreDecimal
}

// AddRealizedScore adds a delta to the realized score and returns the new value.
// Deprecated: Use AddRealizedScoreDecimal for better precision.
func (us *UserState) AddRealizedScore(delta float64) float64 {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.RealizedScore += delta
	us.RealizedScoreDecimal = us.RealizedScoreDecimal.Add(decimal.NewFromFloat(delta))
	return us.RealizedScore
}

// AddRealizedScoreDecimal adds a delta to the realized score with decimal precision.
func (us *UserState) AddRealizedScoreDecimal(delta decimal.Decimal) decimal.Decimal {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.RealizedScoreDecimal = us.RealizedScoreDecimal.Add(delta)
	us.RealizedScore, _ = us.RealizedScoreDecimal.Float64()
	return us.RealizedScoreDecimal
}

// CalculateUnrealizedScore calculates the unrealized score for all open positions
// using the Tralent formula: pct_change * qty_used
// For LONG: pct_change = (mark_price - entry_price) / entry_price * 100
// For SHORT: pct_change = (entry_price - mark_price) / entry_price * 100
// Deprecated: Use CalculateUnrealizedScoreDecimal for better precision.
func (us *UserState) CalculateUnrealizedScore(getPriceFunc func(symbol string) (float64, bool)) float64 {
	us.mu.RLock()
	defer us.mu.RUnlock()

	var unrealizedScore float64
	for _, pos := range us.Positions {
		markPrice, ok := getPriceFunc(pos.Symbol)
		if !ok || pos.EntryPrice == 0 {
			continue
		}

		var pctChange float64
		if IsLong(pos.Side) { // LONG
			pctChange = (markPrice - pos.EntryPrice) / pos.EntryPrice * 100
		} else { // SHORT
			pctChange = (pos.EntryPrice - markPrice) / pos.EntryPrice * 100
		}

		unrealizedScore += float64(pos.QtyUsed) * pctChange
	}

	return unrealizedScore
}

// CalculateUnrealizedScoreDecimal calculates the unrealized score with decimal precision.
func (us *UserState) CalculateUnrealizedScoreDecimal(getPriceFunc func(symbol string) (float64, bool)) decimal.Decimal {
	us.mu.RLock()
	defer us.mu.RUnlock()

	hundred := decimal.NewFromInt(100)
	unrealizedScore := decimal.Zero

	for _, pos := range us.Positions {
		markPrice, ok := getPriceFunc(pos.Symbol)
		if !ok || pos.EntryPrice == 0 {
			continue
		}

		entryDec := decimal.NewFromFloat(pos.EntryPrice)
		markDec := decimal.NewFromFloat(markPrice)
		qtyDec := decimal.NewFromInt(pos.QtyUsed)

		var pctChange decimal.Decimal
		if IsLong(pos.Side) { // LONG
			pctChange = markDec.Sub(entryDec).Div(entryDec).Mul(hundred)
		} else { // SHORT
			pctChange = entryDec.Sub(markDec).Div(entryDec).Mul(hundred)
		}

		unrealizedScore = unrealizedScore.Add(qtyDec.Mul(pctChange))
	}

	return unrealizedScore.Round(8)
}

// CalculateUnrealizedScoreWithExitPrice calculates unrealized score using proper exit prices.
// Uses bid for LONG exits (you sell at bid) and ask for SHORT exits (you buy at ask).
// This provides more accurate mark-to-market valuation.
func (us *UserState) CalculateUnrealizedScoreWithExitPrice(getExitPriceFunc func(symbol string, side contracts.OrderSide) (float64, bool)) float64 {
	us.mu.RLock()
	defer us.mu.RUnlock()

	var unrealizedScore float64
	for _, pos := range us.Positions {
		// Get appropriate exit price based on position side
		exitPrice, ok := getExitPriceFunc(pos.Symbol, pos.Side)
		if !ok || pos.EntryPrice == 0 {
			continue
		}

		var pctChange float64
		if IsLong(pos.Side) { // LONG - exit at bid
			pctChange = (exitPrice - pos.EntryPrice) / pos.EntryPrice * 100
		} else { // SHORT - exit at ask
			pctChange = (pos.EntryPrice - exitPrice) / pos.EntryPrice * 100
		}

		unrealizedScore += float64(pos.QtyUsed) * pctChange
	}

	return unrealizedScore
}

// CalculateUnrealizedScoreDecimalWithExitPrice calculates unrealized score with decimal precision
// using proper exit prices (bid for LONG, ask for SHORT).
func (us *UserState) CalculateUnrealizedScoreDecimalWithExitPrice(getExitPriceFunc func(symbol string, side contracts.OrderSide) (float64, bool)) decimal.Decimal {
	us.mu.RLock()
	defer us.mu.RUnlock()

	hundred := decimal.NewFromInt(100)
	unrealizedScore := decimal.Zero

	for _, pos := range us.Positions {
		// Get appropriate exit price based on position side
		exitPrice, ok := getExitPriceFunc(pos.Symbol, pos.Side)
		if !ok || pos.EntryPrice == 0 {
			continue
		}

		entryDec := decimal.NewFromFloat(pos.EntryPrice)
		exitDec := decimal.NewFromFloat(exitPrice)
		qtyDec := decimal.NewFromInt(pos.QtyUsed)

		var pctChange decimal.Decimal
		if IsLong(pos.Side) { // LONG - exit at bid
			pctChange = exitDec.Sub(entryDec).Div(entryDec).Mul(hundred)
		} else { // SHORT - exit at ask
			pctChange = entryDec.Sub(exitDec).Div(entryDec).Mul(hundred)
		}

		unrealizedScore = unrealizedScore.Add(qtyDec.Mul(pctChange))
	}

	return unrealizedScore.Round(8)
}

// GetTotalScore returns realized + unrealized score.
// Deprecated: Use GetTotalScoreDecimal for better precision.
func (us *UserState) GetTotalScore(getPriceFunc func(symbol string) (float64, bool)) float64 {
	unrealized := us.CalculateUnrealizedScore(getPriceFunc)
	return us.GetRealizedScore() + unrealized
}

// GetTotalScoreDecimal returns realized + unrealized score with decimal precision.
func (us *UserState) GetTotalScoreDecimal(getPriceFunc func(symbol string) (float64, bool)) decimal.Decimal {
	unrealized := us.CalculateUnrealizedScoreDecimal(getPriceFunc)
	realized := us.GetRealizedScoreDecimal()
	return realized.Add(unrealized).Round(8)
}

// GetQtyAvailable returns the current available quantity.
func (us *UserState) GetQtyAvailable() int64 {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return us.QtyAvailable
}

// AddPendingOrder adds a pending order to the user's state.
func (us *UserState) AddPendingOrder(order *PendingOrder) {
	us.mu.Lock()
	defer us.mu.Unlock()
	us.PendingOrders[order.OrderID] = order
}

// RemovePendingOrder removes a pending order from the user's state.
func (us *UserState) RemovePendingOrder(orderID string) {
	us.mu.Lock()
	defer us.mu.Unlock()
	delete(us.PendingOrders, orderID)
}

// GetPendingOrder returns a pending order if it exists.
func (us *UserState) GetPendingOrder(orderID string) (*PendingOrder, bool) {
	us.mu.RLock()
	defer us.mu.RUnlock()
	order, exists := us.PendingOrders[orderID]
	if !exists {
		return nil, false
	}
	// Return a copy
	orderCopy := *order
	return &orderCopy, true
}

// GetAllPendingOrders returns a copy of all pending orders.
func (us *UserState) GetAllPendingOrders() []*PendingOrder {
	us.mu.RLock()
	defer us.mu.RUnlock()
	orders := make([]*PendingOrder, 0, len(us.PendingOrders))
	for _, order := range us.PendingOrders {
		orderCopy := *order
		orders = append(orders, &orderCopy)
	}
	return orders
}

// HasOpenPositions returns true if the user has any open positions.
func (us *UserState) HasOpenPositions() bool {
	us.mu.RLock()
	defer us.mu.RUnlock()
	return len(us.Positions) > 0
}

// ForEachUser iterates over all users in the contest.
func (cs *ContestState) ForEachUser(fn func(userID string, us *UserState)) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for userID, us := range cs.Users {
		fn(userID, us)
	}
}

// ForEachContest iterates over all contests.
func (sm *StateManager) ForEachContest(fn func(contestID string, cs *ContestState)) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for contestID, cs := range sm.contests {
		fn(contestID, cs)
	}
}
