package v1

// OrderRequest represents a request to place a new order.
type OrderRequest struct {
	OrderID    string    `json:"order_id"`
	UserID     string    `json:"user_id"`
	ContestID  string    `json:"contest_id"`
	Symbol     string    `json:"symbol"`
	Side       OrderSide `json:"side"`
	Type       OrderType `json:"type"`
	Qty        int64     `json:"qty"`
	LimitPrice *float64  `json:"limit_price,omitempty"`
	StopPrice  *float64  `json:"stop_price,omitempty"`
	TakeProfit *float64  `json:"take_profit,omitempty"`
	StopLoss   *float64  `json:"stop_loss,omitempty"`
	ClientTs   int64     `json:"client_ts"`
}
