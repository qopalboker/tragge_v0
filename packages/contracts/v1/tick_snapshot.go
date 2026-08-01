package v1

// SymbolTick represents tick data for a single symbol.
type SymbolTick struct {
	Symbol    string  `json:"symbol"`
	Bid       float64 `json:"bid"`
	Ask       float64 `json:"ask"`
	Last      float64 `json:"last"`
	Timestamp int64   `json:"timestamp,omitempty"` // Unix milliseconds; omitted when zero for backwards compatibility
	Volume    float64 `json:"volume,omitempty"`    // Trade volume from provider; zero means unknown
}

// TickSnapshot represents a market tick snapshot containing bid/ask/last prices.
type TickSnapshot struct {
	Ts      int64        `json:"ts"`
	Symbols []SymbolTick `json:"symbols"`
}
