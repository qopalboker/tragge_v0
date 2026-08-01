package v1

// SettlementStatus represents the status of a settlement process.
type SettlementStatus string

const (
	SettlementStatusPending    SettlementStatus = "pending"
	SettlementStatusInProgress SettlementStatus = "in_progress"
	SettlementStatusCompleted  SettlementStatus = "completed"
	SettlementStatusFailed     SettlementStatus = "failed"
	SettlementStatusPartial    SettlementStatus = "partial"
)

// SettlementRequest is published when a contest needs to be settled.
type SettlementRequest struct {
	ContestID string `json:"contest_id"`
	Reason    string `json:"reason,omitempty"` // e.g., "contest_ended", "admin_triggered"
	Ts        int64  `json:"ts"`
}

// SettlementStartedEvent is published when settlement begins.
type SettlementStartedEvent struct {
	ContestID         string `json:"contest_id"`
	SettlementID      string `json:"settlement_id"`
	TotalParticipants int    `json:"total_participants"`
	Ts                int64  `json:"ts"`
}

// PositionsClosedEvent is published when all positions have been closed.
type PositionsClosedEvent struct {
	ContestID       string            `json:"contest_id"`
	SettlementID    string            `json:"settlement_id"`
	PositionsClosed int               `json:"positions_closed"`
	OrdersCancelled int               `json:"orders_cancelled"`
	SnapshotPrices  map[string]Price  `json:"snapshot_prices"` // symbol -> price
	Ts              int64             `json:"ts"`
}

// Price represents bid/ask/last prices at snapshot time.
type Price struct {
	Bid  float64 `json:"bid"`
	Ask  float64 `json:"ask"`
	Last float64 `json:"last"`
}

// FinalRanking represents a user's final ranking in a contest.
type FinalRanking struct {
	UserID                    string  `json:"user_id"`
	Rank                      int     `json:"rank"`
	TiedWithCount             int     `json:"tied_with_count"` // Number of users with same rank
	FinalScore                float64 `json:"final_score"`
	RealizedScore             float64 `json:"realized_score"`
	TotalTrades               int     `json:"total_trades"`
	WinningTrades             int     `json:"winning_trades"`
	WinRate                   float64 `json:"win_rate"`
	TraggePointContribution   float64 `json:"tragge_point_contribution"`
        TotalScore                float64 `json:"total_score"`
        PrizeCents                int64   `json:"prize_cents,omitempty"`
}

// RankingsCalculatedEvent is published when final rankings have been determined.
type RankingsCalculatedEvent struct {
	ContestID         string         `json:"contest_id"`
	SettlementID      string         `json:"settlement_id"`
	TotalParticipants int            `json:"total_participants"`
	Rankings          []FinalRanking `json:"rankings"` // Top N rankings (e.g., top 100)
	Ts                int64          `json:"ts"`
}

// PrizeAllocation represents a prize allocated to a user.
type PrizeAllocation struct {
	UserID          string  `json:"user_id"`
	Rank            int     `json:"rank"`
	AmountCents     int64   `json:"amount_cents"`
	Percentage      float64 `json:"percentage"` // Percentage of prize pool
	Status          string  `json:"status"`     // pending, credited, failed
	LedgerEntryID   string  `json:"ledger_entry_id,omitempty"`
	ErrorMessage    string  `json:"error_message,omitempty"`
}

// PrizesCalculatedEvent is published when prizes have been calculated.
type PrizesCalculatedEvent struct {
	ContestID          string            `json:"contest_id"`
	SettlementID       string            `json:"settlement_id"`
	PrizePoolGross     int64             `json:"prize_pool_gross_cents"`
	PrizePoolNet       int64             `json:"prize_pool_net_cents"`
	PlatformFee        int64             `json:"platform_fee_cents"`
	WinnersCount       int               `json:"winners_count"`
	Allocations        []PrizeAllocation `json:"allocations"`
	Ts                 int64             `json:"ts"`
}

// PrizesDistributedEvent is published when prizes have been credited to wallets.
type PrizesDistributedEvent struct {
	ContestID         string            `json:"contest_id"`
	SettlementID      string            `json:"settlement_id"`
	SuccessfulCredits int               `json:"successful_credits"`
	FailedCredits     int               `json:"failed_credits"`
	TotalDistributed  int64             `json:"total_distributed_cents"`
	FailedAllocations []PrizeAllocation `json:"failed_allocations,omitempty"`
	Ts                int64             `json:"ts"`
}

// SettlementCompletedEvent is published when the entire settlement process is complete.
type SettlementCompletedEvent struct {
	ContestID            string `json:"contest_id"`
	SettlementID         string `json:"settlement_id"`
	TotalParticipants    int    `json:"total_participants"`
	TotalWinners         int    `json:"total_winners"`
	TotalDistributed     int64  `json:"total_distributed_cents"`
	PlatformFee          int64  `json:"platform_fee_cents"`
	PositionsClosed      int    `json:"positions_closed"`
	OrdersCancelled      int    `json:"orders_cancelled"`
	DurationMs           int64  `json:"duration_ms"` // Total settlement time
	Ts                   int64  `json:"ts"`
}

// SettlementFailedEvent is published when settlement fails.
type SettlementFailedEvent struct {
	ContestID    string           `json:"contest_id"`
	SettlementID string           `json:"settlement_id"`
	Status       SettlementStatus `json:"status"`
	ErrorMessage string           `json:"error_message"`
	AttemptCount int              `json:"attempt_count"`
	WillRetry    bool             `json:"will_retry"`
	Ts           int64            `json:"ts"`
}

// ClosePositionsRequest is the request to close all positions for a contest.
// This is consumed by the trading-engine.
type ClosePositionsRequest struct {
	ContestID  string   `json:"contest_id"`
	Reason     string   `json:"reason"`
	ClosePrice *float64 `json:"close_price,omitempty"` // Optional fixed price for all
	Ts         int64    `json:"ts"`
}

// CancelAllOrdersRequest is the request to cancel all pending orders for a contest.
// This is consumed by the trading-engine.
type CancelAllOrdersRequest struct {
	ContestID string       `json:"contest_id"`
	Reason    CancelReason `json:"reason"`
	Ts        int64        `json:"ts"`
}

// TraggePointUpdateEvent is published when a user's T-Point is updated.
type TraggePointUpdateEvent struct {
	UserID                   string  `json:"user_id"`
	ContestID                string  `json:"contest_id"`
	PointContribution        float64 `json:"point_contribution"`
	NewTotalTraggePoint      float64 `json:"new_total_tragge_point"`
	Ts                       int64   `json:"ts"`
}

// SettlementNotification represents a notification to be sent to a user.
type SettlementNotification struct {
	UserID      string                 `json:"user_id"`
	Type        string                 `json:"type"` // contest_ended, prize_won, contest_result
	ContestID   string                 `json:"contest_id"`
	ContestName string                 `json:"contest_name"`
	Data        map[string]interface{} `json:"data"`
	Ts          int64                  `json:"ts"`
}
