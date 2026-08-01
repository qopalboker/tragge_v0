package v1

// MarketStatusEvent represents a market status change event sent via WebSocket.
// This is used to notify clients when markets open or close.
type MarketStatusEvent struct {
	Type       string `json:"type"`        // Always "market_status"
	AssetClass string `json:"asset_class"` // "forex", "crypto", "stocks", "commodities", "mixed"
	Status     string `json:"status"`      // "open" or "closed"
	Reason     string `json:"reason,omitempty"`
	ReopensAt  string `json:"reopens_at,omitempty"` // RFC3339 format when market will reopen
	ClosesAt   string `json:"closes_at,omitempty"`  // RFC3339 format when market will close
	Ts         int64  `json:"ts"`
}

// MarketStatus represents the current status of a market.
type MarketStatus struct {
	AssetClass string  `json:"asset_class"`
	IsOpen     bool    `json:"is_open"`
	Reason     string  `json:"reason,omitempty"`
	NextOpen   *string `json:"next_open,omitempty"`  // RFC3339 format
	NextClose  *string `json:"next_close,omitempty"` // RFC3339 format
	Override   *string `json:"override,omitempty"`   // If there's an active override
}

// MarketOverride represents a manual override for special events.
type MarketOverride struct {
	AssetClass string `json:"asset_class"`
	Status     string `json:"status"` // "open" or "closed"
	Reason     string `json:"reason"`
	ExpiresAt  string `json:"expires_at"` // RFC3339 format
	CreatedBy  string `json:"created_by"`
	CreatedAt  string `json:"created_at"` // RFC3339 format
}

// MarketHoursConfig represents the market hours configuration for an asset class.
type MarketHoursConfig struct {
	AssetClass string            `json:"asset_class"`
	OpenTime   *MarketTimeSpec   `json:"open_time,omitempty"`  // nil if always open
	CloseTime  *MarketTimeSpec   `json:"close_time,omitempty"` // nil if never closes
	AlwaysOpen bool              `json:"always_open"`
	Holidays   []string          `json:"holidays"` // List of holiday dates in YYYY-MM-DD format
}

// MarketTimeSpec represents a specific day and time for market open/close.
type MarketTimeSpec struct {
	Day      string `json:"day"`      // Day of week (e.g., "Sunday", "Friday")
	Time     string `json:"time"`     // Time in HH:MM format
	Timezone string `json:"timezone"` // IANA timezone (e.g., "UTC", "America/New_York")
}

// MarketStatusRequest is used by admin to query market status.
type MarketStatusRequest struct {
	AssetClass string `json:"asset_class,omitempty"` // If empty, returns all asset classes
}

// MarketStatusResponse contains the market status for one or more asset classes.
type MarketStatusResponse struct {
	Statuses []MarketStatus `json:"statuses"`
}

// SetOverrideRequest is used by admin to set a manual market override.
type SetOverrideRequest struct {
	AssetClass string `json:"asset_class"`
	Status     string `json:"status"`     // "open" or "closed"
	Reason     string `json:"reason"`
	ExpiresAt  string `json:"expires_at"` // RFC3339 format
}

// ValidateContestTimesRequest is used to validate contest times against market hours.
type ValidateContestTimesRequest struct {
	AssetClass string `json:"asset_class"`
	StartsAt   string `json:"starts_at"` // RFC3339 format
	EndsAt     string `json:"ends_at"`   // RFC3339 format
}

// ValidateContestTimesResponse contains validation results.
type ValidateContestTimesResponse struct {
	Valid  bool   `json:"valid"`
	Reason string `json:"reason,omitempty"` // Why validation failed
}
