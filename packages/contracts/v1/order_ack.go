package v1

// RateLimitInfo contains metadata about a rate limit rejection.
type RateLimitInfo struct {
	Scope        string `json:"scope"`          // "user", "contest", or "global"
	Limit        int    `json:"limit"`          // Maximum requests allowed
	Window       string `json:"window"`         // Window duration (e.g., "1s", "1m")
	RetryAfterMs int64  `json:"retry_after_ms"` // Milliseconds until next request allowed
}

// OrderAck represents an acknowledgment response for an order request.
type OrderAck struct {
	OrderID   string         `json:"order_id"`
	Status    OrderStatus    `json:"status"`
	Reason    *string        `json:"reason,omitempty"`
	RateLimit *RateLimitInfo `json:"rate_limit,omitempty"` // Present only for RATE_LIMITED rejections
}
