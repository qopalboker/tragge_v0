package v1

// ContestState represents a contest state change event.
type ContestState struct {
	ContestID string        `json:"contest_id"`
	Phase     ContestPhase  `json:"phase"`
	Status    ContestStatus `json:"status,omitempty"` // Detailed status (optional for backward compatibility)
	Reason    string        `json:"reason,omitempty"` // Reason for state change (e.g., cancellation reason)
	Ts        int64         `json:"ts"`
}

// ContestStateExtended represents a contest state change event with full lifecycle metadata.
type ContestStateExtended struct {
	ContestID           string        `json:"contest_id"`
	Phase               ContestPhase  `json:"phase"`
	Status              ContestStatus `json:"status"`
	PreviousStatus      ContestStatus `json:"previous_status,omitempty"`
	Reason              string        `json:"reason,omitempty"`
	ActorID             string        `json:"actor_id,omitempty"`     // User who triggered the change
	CurrentParticipants int           `json:"current_participants"`
	MinParticipants     int           `json:"min_participants"`
	MaxParticipants     *int          `json:"max_participants,omitempty"`
	Ts                  int64         `json:"ts"`
}
