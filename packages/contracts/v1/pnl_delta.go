package v1

// PnLDelta represents a PnL score change event for leaderboard updates.
// Scoring uses Tralent-like formula:
//   - LONG:  pct_change = (exit_price - entry_price) / entry_price * 100
//   - SHORT: pct_change = (entry_price - exit_price) / entry_price * 100
//   - trade_score = qty_used * pct_change
//   - total_score = sum(realized trade_scores) + unrealized_score
//
// Note: All float64 fields are maintained for backward compatibility with existing consumers.
// The *Decimal string fields contain the high-precision decimal values (8 decimal places)
// and should be preferred for accurate score calculations.
type PnLDelta struct {
	UserID          string  `json:"user_id"`
	ContestID       string  `json:"contest_id"`
	DeltaScore      float64 `json:"delta_score"`       // Change in realized score (float64 for backward compat)
	RealizedScore   float64 `json:"realized_score"`    // Sum of all realized trade scores (float64 for backward compat)
	UnrealizedScore float64 `json:"unrealized_score"`  // Mark-to-market score for open positions (float64 for backward compat)
	TotalScore      float64 `json:"total_score"`       // RealizedScore + UnrealizedScore (float64 for backward compat)
	Ts              int64   `json:"ts"`

	// P3-1: Monotonic sequence number for ordering PnL deltas per user+contest.
	// Consumers should process deltas in SeqNum order and discard out-of-order deltas.
	// SeqNum is 0 for legacy messages that predate this field.
	SeqNum uint64 `json:"seq_num,omitempty"`

	// High-precision decimal string fields (8 decimal places)
	// These fields should be preferred for accurate score calculations.
	// Consumers should parse these strings using a decimal library (e.g., shopspring/decimal).
	DeltaScoreDecimal      string `json:"delta_score_decimal,omitempty"`      // High-precision delta score
	RealizedScoreDecimal   string `json:"realized_score_decimal,omitempty"`   // High-precision realized score
	UnrealizedScoreDecimal string `json:"unrealized_score_decimal,omitempty"` // High-precision unrealized score
	TotalScoreDecimal      string `json:"total_score_decimal,omitempty"`      // High-precision total score
}
