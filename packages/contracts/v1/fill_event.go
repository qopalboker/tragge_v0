package v1

// FillEvent represents an order fill execution event.
type FillEvent struct {
	FillID    string    `json:"fill_id"`
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	ContestID string    `json:"contest_id"`
	Symbol    string    `json:"symbol"`
	Side      OrderSide `json:"side"`
	Qty       int64     `json:"qty"`
	FillPrice float64   `json:"fill_price"`
	Ts        int64     `json:"ts"`
}
