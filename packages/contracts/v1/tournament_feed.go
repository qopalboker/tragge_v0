package v1

// Tournament feed WebSocket message types.
const (
	TournamentMsgNew              = "tournament.new"
	TournamentMsgUpdated          = "tournament.updated"
	TournamentMsgPrizePoolChanged = "tournament.prize_pool_changed"
	TournamentMsgStatusChanged    = "tournament.status_changed"
	TournamentMsgEnded            = "tournament.ended"
)

// TournamentFeedMessage is the envelope for all tournament feed WebSocket messages.
type TournamentFeedMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	Ts      int64       `json:"ts"`
}

// TournamentSnapshot is the full state sent on client subscribe to tournament:{id}.
type TournamentSnapshot struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Status              string `json:"status"`
	MarketType          string `json:"market_type"`
	DurationType        string `json:"duration_type"`
	StartTimeUTC        string `json:"start_time_utc"`
	StartTimeIRST       string `json:"start_time_irst"`
	EndTimeUTC          string `json:"end_time_utc"`
	EndTimeIRST         string `json:"end_time_irst"`
	EntryFeeCents       int    `json:"entry_fee_cents"`
	IsFree              bool   `json:"is_free"`
	PrizePoolCents      int64  `json:"prize_pool_cents"`
	CurrentParticipants int    `json:"current_participants"`
	MaxParticipants     *int   `json:"max_participants,omitempty"`
	TimeRemainingMs     int64  `json:"time_remaining_ms"`
	ServerTimeUTC       string `json:"server_time_utc"`
}

// TournamentNewPayload is the payload for tournament.new messages.
type TournamentNewPayload struct {
	ContestID    string `json:"contest_id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	MarketType   string `json:"market_type"`
	DurationType string `json:"duration_type"`
	StartTimeUTC string `json:"start_time_utc"`
	EndTimeUTC   string `json:"end_time_utc"`
	EntryFeeCents int   `json:"entry_fee_cents"`
	IsFree       bool   `json:"is_free"`
}

// TournamentPrizePoolChangedPayload is the payload for tournament.prize_pool_changed messages.
type TournamentPrizePoolChangedPayload struct {
	ContestID           string `json:"contest_id"`
	PrizePoolCents      int64  `json:"prize_pool_cents"`
	CurrentParticipants int    `json:"current_participants"`
	Event               string `json:"event"` // "participant_joined" or "participant_left"
}

// TournamentStatusChangedPayload is the payload for tournament.status_changed messages.
type TournamentStatusChangedPayload struct {
	ContestID      string `json:"contest_id"`
	PreviousStatus string `json:"previous_status,omitempty"`
	NewStatus      string `json:"new_status"`
}

// TournamentEndedPayload is the payload for tournament.ended messages.
type TournamentEndedPayload struct {
	ContestID string `json:"contest_id"`
	Reason    string `json:"reason"` // "completed" or "cancelled"
}
