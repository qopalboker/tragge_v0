package v1

// Position represents a single trading position.
type Position struct {
	PositionID       string    `json:"position_id"`
	Symbol           string    `json:"symbol"`
	Side             OrderSide `json:"side"`
	Qty              int64     `json:"qty"`
	EntryPrice       float64   `json:"entry_price"`
	MarkPrice        float64   `json:"mark_price"`
	UnrealizedPnLPct float64   `json:"unrealized_pnl_pct"`
	RealizedPnLPct   float64   `json:"realized_pnl_pct"`
	QtyUsed          int64     `json:"qty_used"`
	// UnrealizedScore is the unrealized P&L score using Tralent formula:
	// For LONG: pct_change = (mark_price - entry_price) / entry_price * 100
	// For SHORT: pct_change = (entry_price - mark_price) / entry_price * 100
	// unrealized_score = qty_used * pct_change
	UnrealizedScore float64 `json:"unrealized_score"`
}

// PositionUpdate represents a user position update event.
type PositionUpdate struct {
	UserID    string     `json:"user_id"`
	ContestID string     `json:"contest_id"`
	Positions []Position `json:"positions"`
}
