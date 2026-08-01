package v1

// ClosePositionRequest represents a request to close an existing position.
// This message is published to the close_positions.v1 topic and consumed by the trading engine.
//
// Fields:
//   - PositionID: UUID of the position to close
//   - UserID: UUID of the authenticated user (must own the position)
//   - ContestID: UUID of the contest context
//   - Symbol: Trading symbol (e.g., "AAPL", "BTCUSD")
//   - Side: The position's side (BUY for long, SELL for short) - needed for closing logic
//   - QtyToClose: Quantity to close. If 0, closes the entire position
//   - CloseReason: Why the position is being closed (user_requested, tp_triggered, sl_triggered, contest_ended)
//   - ClientTs: Client-side timestamp in milliseconds for latency tracking
type ClosePositionRequest struct {
	PositionID  string    `json:"position_id"`            // UUID of the position to close
	UserID      string    `json:"user_id"`                // UUID of the authenticated user
	ContestID   string    `json:"contest_id"`             // UUID of the contest context
	Symbol      string    `json:"symbol"`                 // Trading symbol
	Side        OrderSide `json:"side"`                   // Position side (BUY=long, SELL=short)
	QtyToClose  int64     `json:"qty_to_close,omitempty"` // Quantity to close (0 = close all)
	CloseReason string    `json:"close_reason"`           // Reason: user_requested, tp_triggered, sl_triggered, contest_ended
	ClientTs    int64     `json:"client_ts"`              // Client timestamp in milliseconds
}

// CloseReason constants define the valid reasons for closing a position.
const (
	// CloseReasonUserRequested indicates the user manually requested to close the position.
	CloseReasonUserRequested = "user_requested"
	// CloseReasonTPTriggered indicates the take-profit price was reached.
	CloseReasonTPTriggered = "tp_triggered"
	// CloseReasonSLTriggered indicates the stop-loss price was reached.
	CloseReasonSLTriggered = "sl_triggered"
	// CloseReasonContestEnded indicates the contest ended and all positions were auto-closed.
	CloseReasonContestEnded = "contest_ended"
)
