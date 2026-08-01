package v1

// CancelOrderRequest represents a request to cancel a pending order.
// This message is published to the cancel_orders.v1 topic when a user requests cancellation.
//
// Fields:
//   - OrderID: UUID of the order to cancel
//   - UserID: UUID of the order owner (must match the original order)
//   - ContestID: UUID of the contest context
//   - Symbol: Trading symbol of the order
//   - Qty: Quantity to release back to available balance
//   - CancelReason: Why the order is being cancelled (see CancelReason constants)
//   - ClientTs: Client-side timestamp in milliseconds when the request was initiated
//
// Cancel Reasons:
//   - "user_requested" - User manually cancelled the order
//   - "contest_ended" - Auto-cancelled when contest ends
//   - "insufficient_funds" - Balance check failed during order processing
//   - "expired" - Order reached its expiration time
type CancelOrderRequest struct {
	OrderID      string       `json:"order_id"`      // UUID of the order to cancel
	UserID       string       `json:"user_id"`       // UUID of the order owner
	ContestID    string       `json:"contest_id"`    // UUID of the contest context
	Symbol       string       `json:"symbol"`        // Trading symbol
	Qty          int64        `json:"qty"`           // Quantity to release back
	CancelReason CancelReason `json:"cancel_reason"` // Reason for cancellation
	ClientTs     int64        `json:"client_ts"`     // Client timestamp in milliseconds
}
