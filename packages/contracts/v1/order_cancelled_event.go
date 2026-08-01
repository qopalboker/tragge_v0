package v1

// OrderCancelledEvent represents an event emitted when an order is cancelled.
// This message is published to the order_cancelled.v1 topic after an order cancellation is processed.
//
// Fields:
//   - OrderID: UUID of the cancelled order
//   - UserID: UUID of the order owner
//   - ContestID: UUID of the contest context
//   - Symbol: Trading symbol of the cancelled order
//   - QtyReleased: Quantity that was released back to available balance
//   - CancelReason: Why the order was cancelled (see CancelReason constants)
//   - Ts: Server-side timestamp in milliseconds when the cancellation was executed
//
// Cancel Reasons:
//   - "user_requested" - User manually cancelled the order
//   - "contest_ended" - Auto-cancelled when contest ends
//   - "insufficient_funds" - Balance check failed during order processing
//   - "expired" - Order reached its expiration time
type OrderCancelledEvent struct {
	OrderID      string       `json:"order_id"`      // UUID of the cancelled order
	UserID       string       `json:"user_id"`       // UUID of the order owner
	ContestID    string       `json:"contest_id"`    // UUID of the contest context
	Symbol       string       `json:"symbol"`        // Trading symbol
	QtyReleased  int64        `json:"qty_released"`  // Quantity released back to balance
	CancelReason CancelReason `json:"cancel_reason"` // Reason for cancellation
	Ts           int64        `json:"ts"`            // Server timestamp in milliseconds
}
