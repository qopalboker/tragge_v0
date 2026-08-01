package v1

// PositionClosedEvent represents an event emitted when a position is closed.
// This message is published to the position_closed.v1 topic after a position close is executed.
//
// Fields:
//   - PositionID: UUID of the closed position
//   - UserID: UUID of the position owner
//   - ContestID: UUID of the contest context
//   - Symbol: Trading symbol that was closed
//   - Side: The position's side (BUY=long, SELL=short)
//   - QtyClosed: Actual quantity that was closed
//   - ClosePrice: Price at which the position was closed
//   - RealizedPnL: Realized profit/loss in currency units
//   - RealizedScore: Realized score using Tralent formula (qty_used * pct_change)
//   - CloseReason: Why the position was closed
//   - Ts: Server-side timestamp in milliseconds when the close was executed
type PositionClosedEvent struct {
	PositionID    string    `json:"position_id"`    // UUID of the closed position
	UserID        string    `json:"user_id"`        // UUID of the position owner
	ContestID     string    `json:"contest_id"`     // UUID of the contest context
	Symbol        string    `json:"symbol"`         // Trading symbol
	Side          OrderSide `json:"side"`           // Position side (BUY=long, SELL=short)
	QtyClosed     int64     `json:"qty_closed"`     // Quantity that was closed
	ClosePrice    float64   `json:"close_price"`    // Execution price for the close
	RealizedPnL   float64   `json:"realized_pnl"`   // Realized profit/loss in currency units
	RealizedScore float64   `json:"realized_score"` // Realized score (Tralent formula)
	CloseReason   string    `json:"close_reason"`   // Reason: user_requested, tp_triggered, sl_triggered, contest_ended
	Ts            int64     `json:"ts"`             // Server timestamp in milliseconds
}
