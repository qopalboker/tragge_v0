package v1

// ContestEvent represents a contest lifecycle event for WebSocket broadcast.
type ContestEvent struct {
	Type      ContestEventType `json:"type"`
	ContestID string           `json:"contest_id"`
	Name      string           `json:"name,omitempty"`
	EndsAt    int64            `json:"ends_at,omitempty"`
	Message   string           `json:"message,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	Ts        int64            `json:"ts"`
}

// ContestEventType represents the type of contest event.
type ContestEventType string

const (
	ContestEventStarted      ContestEventType = "contest_started"
	ContestEventPaused       ContestEventType = "contest_paused"
	ContestEventResumed      ContestEventType = "contest_resumed"
	ContestEventTimeExtended ContestEventType = "contest_time_extended"
	ContestEventTradingEnded ContestEventType = "trading_ended"
	ContestEventSettling     ContestEventType = "contest_settling"
	ContestEventCompleted    ContestEventType = "contest_completed"
	ContestEventCancelled    ContestEventType = "contest_cancelled"
	ContestEventResultsReady ContestEventType = "results_ready"
	ContestEventUpdated      ContestEventType = "contest_updated"
	ContestEventPrizeLocked  ContestEventType = "prize_locked"
)

// ContestUpdatePayload represents a real-time contest update broadcast
// when a participant joins or leaves (used for live prize recalculation).
type ContestUpdatePayload struct {
	Type                string          `json:"type"`                  // always "contest_updated"
	ContestID           string          `json:"contest_id"`
	Event               string          `json:"event"`                 // "participant_joined" or "participant_left"
	CurrentParticipants int             `json:"current_participants"`
	PrizePoolCents      int64           `json:"prize_pool_cents"`
	WinnersCount        int             `json:"winners_count"`
	FirstPrizeCents     int64           `json:"first_prize_cents"`
	TotalPrizeCents     int64           `json:"total_prize_cents"`
	Top3Prizes          []RankPrizeBrief `json:"top_3_prizes"`
	Ts                  int64           `json:"ts"`
}

// RankPrizeBrief is a minimal rank+amount pair for broadcast.
type RankPrizeBrief struct {
	Rank       int   `json:"rank"`
	AmountCents int64 `json:"amount_cents"`
}

// ContestNotification represents a notification to be sent to contest participants.
type ContestNotification struct {
	Type      ContestNotificationType `json:"type"`
	ContestID string                  `json:"contest_id"`
	UserID    string                  `json:"user_id,omitempty"` // Empty for broadcast to all participants
	Title     string                  `json:"title"`
	Body      string                  `json:"body"`
	Data      map[string]any          `json:"data,omitempty"`
	Channels  []NotificationChannel   `json:"channels"` // push, email, in_app
	Priority  NotificationPriority    `json:"priority"`
	Ts        int64                   `json:"ts"`
}

// ContestNotificationType represents the type of contest notification.
type ContestNotificationType string

const (
	NotificationContestStarted   ContestNotificationType = "contest_started"
	NotificationContestEnding    ContestNotificationType = "contest_ending"
	NotificationTradingEnded     ContestNotificationType = "trading_ended"
	NotificationResultsReady     ContestNotificationType = "results_ready"
	NotificationContestCancelled ContestNotificationType = "contest_cancelled"
	NotificationPrizeWon         ContestNotificationType = "prize_won"
	NotificationContestCompleted ContestNotificationType = "contest_completed"
)

// NotificationChannel represents a notification delivery channel.
type NotificationChannel string

const (
	ChannelPush  NotificationChannel = "push"
	ChannelEmail NotificationChannel = "email"
	ChannelInApp NotificationChannel = "in_app"
)

// NotificationPriority represents notification priority level.
type NotificationPriority string

const (
	PriorityLow      NotificationPriority = "low"
	PriorityNormal   NotificationPriority = "normal"
	PriorityHigh     NotificationPriority = "high"
	PriorityCritical NotificationPriority = "critical"
)

// ContestResults represents the final results of a contest.
type ContestResults struct {
	ContestID         string         `json:"contest_id"`
	ContestName       string         `json:"contest_name"`
	TotalParticipants int            `json:"total_participants"`
	WinnersCount      int            `json:"winners_count"`
	PrizePoolCents    int64          `json:"prize_pool_cents"`
	TotalPaidCents    int64          `json:"total_paid_cents"`
	Rankings          []FinalRanking `json:"rankings"`
	FinalizedAt       int64          `json:"finalized_at"`
}


