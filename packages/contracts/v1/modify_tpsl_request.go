package v1

// ModifyTPSLRequest represents a request to modify take profit and/or stop loss
// levels for an existing open position.
// This message is published to the orders.v1 topic and consumed by the trading engine.
//
// Fields:
//   - PositionID: UUID of the position to modify
//   - UserID: UUID of the authenticated user (must own the position)
//   - ContestID: UUID of the contest context
//   - Symbol: Trading symbol (e.g., "AAPL", "BTCUSD")
//   - TakeProfit: New take profit price (nil to remove TP)
//   - StopLoss: New stop loss price (nil to remove SL)
//   - ClientTs: Client-side timestamp in milliseconds for latency tracking
type ModifyTPSLRequest struct {
	PositionID string   `json:"position_id"` // UUID of the position to modify
	UserID     string   `json:"user_id"`     // UUID of the authenticated user
	ContestID  string   `json:"contest_id"`  // UUID of the contest context
	Symbol     string   `json:"symbol"`      // Trading symbol
	TakeProfit *float64 `json:"take_profit"` // New take profit price (null to remove)
	StopLoss   *float64 `json:"stop_loss"`   // New stop loss price (null to remove)
	ClientTs   int64    `json:"client_ts"`   // Client timestamp in milliseconds
}
