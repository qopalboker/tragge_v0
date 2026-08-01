package server

import (
	"context"
	"database/sql"
	"sync"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"go.uber.org/zap"
)

// PendingOrderInfo holds information about a pending order.
type PendingOrderInfo struct {
	OrderID         string
	ContestID       string
	UserID          string
	Symbol          string
	Side            contracts.OrderSide
	Type            contracts.OrderType
	Qty             int64
	LimitPrice      *float64
	StopPrice       *float64
	TakeProfit      *float64
	StopLoss        *float64
	StalePriceRetry bool // Set to true when order was not executed due to stale price
}

// PositionWithTPSL holds position info with TP/SL levels.
type PositionWithTPSL struct {
	PositionID string
	ContestID  string
	UserID     string
	Symbol     string
	Side       contracts.OrderSide // BUY = long, SELL = short
	QtyOpen    int64
	EntryPrice float64
	TakeProfit *float64
	StopLoss   *float64
}

// PendingOrderBook manages pending orders and positions with TP/SL.
type PendingOrderBook struct {
	// Pending orders indexed by symbol for efficient lookup on tick updates
	ordersBySymbol map[string]map[string]*PendingOrderInfo // symbol -> orderID -> order

	// Positions with TP/SL indexed by symbol
	positionsBySymbol map[string]map[string]*PositionWithTPSL // symbol -> positionID -> position

	mu sync.RWMutex
}

// NewPendingOrderBook creates a new pending order book.
func NewPendingOrderBook() *PendingOrderBook {
	return &PendingOrderBook{
		ordersBySymbol:    make(map[string]map[string]*PendingOrderInfo),
		positionsBySymbol: make(map[string]map[string]*PositionWithTPSL),
	}
}

// Clear removes all pending orders and TP/SL positions from the book.
func (pob *PendingOrderBook) Clear() {
	pob.mu.Lock()
	defer pob.mu.Unlock()
	pob.ordersBySymbol = make(map[string]map[string]*PendingOrderInfo)
	pob.positionsBySymbol = make(map[string]map[string]*PositionWithTPSL)
}

// AddPendingOrder adds a pending order to the book.
func (pob *PendingOrderBook) AddPendingOrder(order *PendingOrderInfo) {
	pob.mu.Lock()
	defer pob.mu.Unlock()

	if pob.ordersBySymbol[order.Symbol] == nil {
		pob.ordersBySymbol[order.Symbol] = make(map[string]*PendingOrderInfo)
	}
	pob.ordersBySymbol[order.Symbol][order.OrderID] = order
}

// RemovePendingOrder removes a pending order from the book.
func (pob *PendingOrderBook) RemovePendingOrder(symbol, orderID string) {
	pob.mu.Lock()
	defer pob.mu.Unlock()

	if symbolOrders, exists := pob.ordersBySymbol[symbol]; exists {
		delete(symbolOrders, orderID)
		if len(symbolOrders) == 0 {
			delete(pob.ordersBySymbol, symbol)
		}
	}
}

// GetPendingOrdersForSymbol returns all pending orders for a symbol.
func (pob *PendingOrderBook) GetPendingOrdersForSymbol(symbol string) []*PendingOrderInfo {
	pob.mu.RLock()
	defer pob.mu.RUnlock()

	symbolOrders, exists := pob.ordersBySymbol[symbol]
	if !exists {
		return nil
	}

	orders := make([]*PendingOrderInfo, 0, len(symbolOrders))
	for _, order := range symbolOrders {
		orderCopy := *order
		orders = append(orders, &orderCopy)
	}
	return orders
}

// AddPositionWithTPSL adds or updates a position with TP/SL levels.
func (pob *PendingOrderBook) AddPositionWithTPSL(pos *PositionWithTPSL) {
	pob.mu.Lock()
	defer pob.mu.Unlock()

	if pob.positionsBySymbol[pos.Symbol] == nil {
		pob.positionsBySymbol[pos.Symbol] = make(map[string]*PositionWithTPSL)
	}
	pob.positionsBySymbol[pos.Symbol][pos.PositionID] = pos
}

// RemovePositionTPSL removes a position from TP/SL tracking.
func (pob *PendingOrderBook) RemovePositionTPSL(symbol, positionID string) {
	pob.mu.Lock()
	defer pob.mu.Unlock()

	if symbolPositions, exists := pob.positionsBySymbol[symbol]; exists {
		delete(symbolPositions, positionID)
		if len(symbolPositions) == 0 {
			delete(pob.positionsBySymbol, symbol)
		}
	}
}

// RemoveAllForContest removes all pending orders and TP/SL positions belonging
// to the given contest. This prevents memory leaks when contests are finalized.
func (pob *PendingOrderBook) RemoveAllForContest(contestID string) (removedOrders int, removedPositions int) {
	pob.mu.Lock()
	defer pob.mu.Unlock()

	for symbol, orders := range pob.ordersBySymbol {
		for orderID, order := range orders {
			if order.ContestID == contestID {
				delete(orders, orderID)
				removedOrders++
			}
		}
		if len(orders) == 0 {
			delete(pob.ordersBySymbol, symbol)
		}
	}

	for symbol, positions := range pob.positionsBySymbol {
		for posID, pos := range positions {
			if pos.ContestID == contestID {
				delete(positions, posID)
				removedPositions++
			}
		}
		if len(positions) == 0 {
			delete(pob.positionsBySymbol, symbol)
		}
	}

	return removedOrders, removedPositions
}

// GetPositionsForSymbol returns all positions with TP/SL for a symbol.
func (pob *PendingOrderBook) GetPositionsForSymbol(symbol string) []*PositionWithTPSL {
	pob.mu.RLock()
	defer pob.mu.RUnlock()

	symbolPositions, exists := pob.positionsBySymbol[symbol]
	if !exists {
		return nil
	}

	positions := make([]*PositionWithTPSL, 0, len(symbolPositions))
	for _, pos := range symbolPositions {
		posCopy := *pos
		positions = append(positions, &posCopy)
	}
	return positions
}

// UpdatePositionQty updates the quantity of a position.
func (pob *PendingOrderBook) UpdatePositionQty(symbol, positionID string, qtyOpen int64) {
	pob.mu.Lock()
	defer pob.mu.Unlock()

	if symbolPositions, exists := pob.positionsBySymbol[symbol]; exists {
		if pos, posExists := symbolPositions[positionID]; posExists {
			pos.QtyOpen = qtyOpen
		}
	}
}

// UpdatePositionTPSL updates the TP/SL levels for a tracked position.
// If both TP and SL are nil, the position is removed from tracking entirely.
// Returns true if the position was found and updated, false otherwise.
func (pob *PendingOrderBook) UpdatePositionTPSL(symbol, positionID string, tp, sl *float64) bool {
	pob.mu.Lock()
	defer pob.mu.Unlock()

	symbolPositions, exists := pob.positionsBySymbol[symbol]
	if !exists {
		return false
	}

	pos, posExists := symbolPositions[positionID]
	if !posExists {
		return false
	}

	// If both TP and SL are nil, remove from tracking
	if tp == nil && sl == nil {
		delete(symbolPositions, positionID)
		if len(symbolPositions) == 0 {
			delete(pob.positionsBySymbol, symbol)
		}
		return true
	}

	// Update TP/SL values
	pos.TakeProfit = tp
	pos.StopLoss = sl
	return true
}

// GetPositionTPSL returns the current TP/SL values for a tracked position.
// Returns nil, nil, false if the position is not being tracked.
func (pob *PendingOrderBook) GetPositionTPSL(symbol, positionID string) (tp, sl *float64, found bool) {
	pob.mu.RLock()
	defer pob.mu.RUnlock()

	symbolPositions, exists := pob.positionsBySymbol[symbol]
	if !exists {
		return nil, nil, false
	}

	pos, posExists := symbolPositions[positionID]
	if !posExists {
		return nil, nil, false
	}

	return pos.TakeProfit, pos.StopLoss, true
}

// TriggeredOrder represents an order that has been triggered.
type TriggeredOrder struct {
	Order     *PendingOrderInfo
	FillPrice float64 // The price at which to fill
}

// TriggeredTPSL represents a TP/SL that has been triggered.
type TriggeredTPSL struct {
	Position  *PositionWithTPSL
	FillPrice float64 // The price at which to fill (may differ from TP/SL level due to slippage)
	IsTP      bool    // true if take profit, false if stop loss
}

// EvaluatePendingOrders evaluates pending orders for a symbol against current bid/ask.
// Returns orders that should be triggered. Evaluates directly under the read lock
// to avoid copying non-triggered orders.
func (pob *PendingOrderBook) EvaluatePendingOrders(symbol string, bid, ask float64) []*TriggeredOrder {
	pob.mu.RLock()
	symbolOrders, exists := pob.ordersBySymbol[symbol]
	if !exists || len(symbolOrders) == 0 {
		pob.mu.RUnlock()
		return nil
	}

	var triggered []*TriggeredOrder

	for _, order := range symbolOrders {
		var shouldTrigger bool
		var fillPrice float64

		switch order.Type {
		case contracts.OrderTypeBuyLimit:
			if order.LimitPrice != nil && ask <= *order.LimitPrice {
				shouldTrigger = true
				fillPrice = ask
			}
		case contracts.OrderTypeSellLimit:
			if order.LimitPrice != nil && bid >= *order.LimitPrice {
				shouldTrigger = true
				fillPrice = bid
			}
		case contracts.OrderTypeBuyStop:
			if order.StopPrice != nil && ask >= *order.StopPrice {
				shouldTrigger = true
				fillPrice = ask
			}
		case contracts.OrderTypeSellStop:
			if order.StopPrice != nil && bid <= *order.StopPrice {
				shouldTrigger = true
				fillPrice = bid
			}
		}

		if shouldTrigger {
			// Only copy triggered orders
			orderCopy := *order
			triggered = append(triggered, &TriggeredOrder{
				Order:     &orderCopy,
				FillPrice: fillPrice,
			})
		}
	}
	pob.mu.RUnlock()

	return triggered
}

// EvaluateTPSL evaluates TP/SL levels for positions against current bid/ask.
// Returns positions that should be closed. Evaluates directly under the read lock
// to avoid copying non-triggered positions.
// Note: Stop loss is NOT guaranteed - if price jumps past SL, fill at first available price.
func (pob *PendingOrderBook) EvaluateTPSL(symbol string, bid, ask float64) []*TriggeredTPSL {
	pob.mu.RLock()
	symbolPositions, exists := pob.positionsBySymbol[symbol]
	if !exists || len(symbolPositions) == 0 {
		pob.mu.RUnlock()
		return nil
	}

	var triggered []*TriggeredTPSL

	for _, pos := range symbolPositions {
		if pos.TakeProfit == nil && pos.StopLoss == nil {
			continue
		}

		var shouldClose bool
		var fillPrice float64
		var isTP bool

		if pos.Side == contracts.OrderSideBuy {
			// LONG position: close by selling
			if pos.TakeProfit != nil && bid >= *pos.TakeProfit {
				shouldClose = true
				fillPrice = bid
				isTP = true
			} else if pos.StopLoss != nil && bid <= *pos.StopLoss {
				shouldClose = true
				fillPrice = bid
				isTP = false
			}
		} else {
			// SHORT position: close by buying
			if pos.TakeProfit != nil && ask <= *pos.TakeProfit {
				shouldClose = true
				fillPrice = ask
				isTP = true
			} else if pos.StopLoss != nil && ask >= *pos.StopLoss {
				shouldClose = true
				fillPrice = ask
				isTP = false
			}
		}

		if shouldClose {
			// Only copy triggered positions
			posCopy := *pos
			triggered = append(triggered, &TriggeredTPSL{
				Position:  &posCopy,
				FillPrice: fillPrice,
				IsTP:      isTP,
			})
		}
	}
	pob.mu.RUnlock()

	return triggered
}

// ReloadFromDB restores pending orders and TP/SL positions from the database on startup.
// This ensures no pending orders or TP/SL are lost across engine restarts.
func (pob *PendingOrderBook) ReloadFromDB(ctx context.Context, db *sql.DB, logger *zap.Logger) error {
	// Load pending orders
	orders, err := GetAllPendingOrders(ctx, db)
	if err != nil {
		return err
	}
	for _, order := range orders {
		pob.AddPendingOrder(order)
	}

	// Load positions with TP/SL
	positions, err := GetAllOpenPositionsWithTPSL(ctx, db)
	if err != nil {
		return err
	}
	for _, pos := range positions {
		pob.AddPositionWithTPSL(pos)
	}

	if logger != nil {
		logger.Info("Recovered pending order book from database",
			zap.Int("pending_orders", len(orders)),
			zap.Int("tpsl_positions", len(positions)))
	}
	return nil
}

// GetAllSymbols returns all symbols with pending orders or positions with TP/SL.
func (pob *PendingOrderBook) GetAllSymbols() []string {
	pob.mu.RLock()
	defer pob.mu.RUnlock()

	symbolSet := make(map[string]bool)
	for symbol := range pob.ordersBySymbol {
		symbolSet[symbol] = true
	}
	for symbol := range pob.positionsBySymbol {
		symbolSet[symbol] = true
	}

	symbols := make([]string, 0, len(symbolSet))
	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}
	return symbols
}
