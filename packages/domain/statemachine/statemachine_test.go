package statemachine

import (
	"testing"
	"time"
)

func TestContestStatus_IsValid(t *testing.T) {
	tests := []struct {
		status ContestStatus
		want   bool
	}{
		{StatusDraft, true},
		{StatusScheduled, true},
		{StatusRegistrationOpen, true},
		{StatusRegistrationClosed, true},
		{StatusRunning, true},
		{StatusPaused, true},
		{StatusSettling, true},
		{StatusCompleted, true},
		{StatusCancelled, true},
		{ContestStatus("invalid"), false},
		{ContestStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("ContestStatus(%q).IsValid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestContestStatus_IsFinal(t *testing.T) {
	tests := []struct {
		status ContestStatus
		want   bool
	}{
		{StatusDraft, false},
		{StatusScheduled, false},
		{StatusRegistrationOpen, false},
		{StatusRegistrationClosed, false},
		{StatusRunning, false},
		{StatusPaused, false},
		{StatusSettling, false},
		{StatusCompleted, true},
		{StatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsFinal(); got != tt.want {
				t.Errorf("ContestStatus(%q).IsFinal() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestContestStatus_AllowsTrading(t *testing.T) {
	tests := []struct {
		status ContestStatus
		want   bool
	}{
		{StatusDraft, false},
		{StatusScheduled, false},
		{StatusRegistrationOpen, false},
		{StatusRegistrationClosed, false},
		{StatusRunning, true},
		{StatusPaused, false},
		{StatusSettling, false},
		{StatusCompleted, false},
		{StatusCancelled, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.AllowsTrading(); got != tt.want {
				t.Errorf("ContestStatus(%q).AllowsTrading() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestContestStatus_AllowsRegistration(t *testing.T) {
	tests := []struct {
		status ContestStatus
		want   bool
	}{
		{StatusDraft, false},
		{StatusScheduled, true},
		{StatusRegistrationOpen, true},
		{StatusRegistrationClosed, false},
		{StatusRunning, false},
		{StatusPaused, false},
		{StatusSettling, false},
		{StatusCompleted, false},
		{StatusCancelled, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.AllowsRegistration(); got != tt.want {
				t.Errorf("ContestStatus(%q).AllowsRegistration() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from ContestStatus
		to   ContestStatus
		want bool
	}{
		// Valid transitions from draft
		{"draft to scheduled", StatusDraft, StatusScheduled, true},
		{"draft to cancelled", StatusDraft, StatusCancelled, true},
		{"draft to running (invalid)", StatusDraft, StatusRunning, false},

		// Valid transitions from scheduled
		{"scheduled to registration_closed", StatusScheduled, StatusRegistrationClosed, true},
		{"scheduled to cancelled", StatusScheduled, StatusCancelled, true},
		{"scheduled to running (invalid)", StatusScheduled, StatusRunning, false},

		// Valid transitions from registration_open
		{"registration_open to registration_closed", StatusRegistrationOpen, StatusRegistrationClosed, true},
		{"registration_open to cancelled", StatusRegistrationOpen, StatusCancelled, true},

		// Valid transitions from registration_closed
		{"registration_closed to running", StatusRegistrationClosed, StatusRunning, true},
		{"registration_closed to cancelled", StatusRegistrationClosed, StatusCancelled, true},
		{"registration_closed to settled (invalid)", StatusRegistrationClosed, StatusSettling, false},

		// Valid transitions from running
		{"running to settling", StatusRunning, StatusSettling, true},
		{"running to paused", StatusRunning, StatusPaused, true},
		{"running to completed (invalid)", StatusRunning, StatusCompleted, false},
		{"running to cancelled (invalid)", StatusRunning, StatusCancelled, false},

		// Valid transitions from paused
		{"paused to running", StatusPaused, StatusRunning, true},
		{"paused to settling", StatusPaused, StatusSettling, true},

		// Valid transitions from settling
		{"settling to completed", StatusSettling, StatusCompleted, true},
		{"settling to cancelled (invalid)", StatusSettling, StatusCancelled, false},

		// No transitions from final states
		{"completed to any (invalid)", StatusCompleted, StatusRunning, false},
		{"cancelled to any (invalid)", StatusCancelled, StatusScheduled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanTransition(tt.from, tt.to); got != tt.want {
				t.Errorf("CanTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestGetAllowedTransitions(t *testing.T) {
	tests := []struct {
		status   ContestStatus
		expected []ContestStatus
	}{
		{
			status:   StatusDraft,
			expected: []ContestStatus{StatusScheduled, StatusCancelled},
		},
		{
			status:   StatusScheduled,
			expected: []ContestStatus{StatusRegistrationOpen, StatusRegistrationClosed, StatusCancelled},
		},
		{
			status:   StatusRunning,
			expected: []ContestStatus{StatusSettling, StatusPaused},
		},
		{
			status:   StatusSettling,
			expected: []ContestStatus{StatusCompleted},
		},
		{
			status:   StatusCompleted,
			expected: []ContestStatus{},
		},
		{
			status:   StatusCancelled,
			expected: []ContestStatus{},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := GetAllowedTransitions(tt.status)
			if len(got) != len(tt.expected) {
				t.Errorf("GetAllowedTransitions(%q) returned %d transitions, want %d",
					tt.status, len(got), len(tt.expected))
				return
			}

			for i, expected := range tt.expected {
				if got[i] != expected {
					t.Errorf("GetAllowedTransitions(%q)[%d] = %q, want %q",
						tt.status, i, got[i], expected)
				}
			}
		})
	}
}

func TestTransitionError_Error(t *testing.T) {
	err := &TransitionError{
		ContestID:  "contest-123",
		FromStatus: StatusDraft,
		ToStatus:   StatusRunning,
		Reason:     "invalid transition",
	}

	expected := "cannot transition contest contest-123 from draft to running: invalid transition"
	if got := err.Error(); got != expected {
		t.Errorf("TransitionError.Error() = %q, want %q", got, expected)
	}
}

func TestContestStatus_String(t *testing.T) {
	tests := []struct {
		status ContestStatus
		want   string
	}{
		{StatusDraft, "draft"},
		{StatusScheduled, "scheduled"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("ContestStatus(%q).String() = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestValidTransitions_Completeness(t *testing.T) {
	// Ensure all valid statuses are covered in the transitions map
	allStatuses := []ContestStatus{
		StatusDraft,
		StatusScheduled,
		StatusRegistrationOpen,
		StatusRegistrationClosed,
		StatusRunning,
		StatusPaused,
		StatusSettling,
		StatusCompleted,
		StatusCancelled,
	}

	for _, status := range allStatuses {
		t.Run(string(status), func(t *testing.T) {
			if _, ok := validTransitions[status]; !ok {
				t.Errorf("Status %q is not covered in validTransitions map", status)
			}
		})
	}
}

func TestContestLifecycle_HappyPath(t *testing.T) {
	// Test the typical happy path: draft -> scheduled -> registration_closed -> running -> settling -> completed
	transitions := []struct {
		from ContestStatus
		to   ContestStatus
	}{
		{StatusDraft, StatusScheduled},
		{StatusScheduled, StatusRegistrationClosed},
		{StatusRegistrationClosed, StatusRunning},
		{StatusRunning, StatusSettling},
		{StatusSettling, StatusCompleted},
	}

	currentStatus := StatusDraft

	for _, tr := range transitions {
		if currentStatus != tr.from {
			t.Errorf("Expected current status to be %q, got %q", tr.from, currentStatus)
		}

		if !CanTransition(tr.from, tr.to) {
			t.Errorf("Expected transition from %q to %q to be valid", tr.from, tr.to)
		}

		currentStatus = tr.to
	}

	if !currentStatus.IsFinal() {
		t.Errorf("Expected final status %q to be marked as final", currentStatus)
	}
}

func TestContestLifecycle_CancellationPath(t *testing.T) {
	// Test cancellation from various states
	cancelableStates := []ContestStatus{
		StatusDraft,
		StatusScheduled,
		StatusRegistrationOpen,
		StatusRegistrationClosed,
	}

	for _, status := range cancelableStates {
		t.Run(string(status), func(t *testing.T) {
			if !CanTransition(status, StatusCancelled) {
				t.Errorf("Expected cancellation from %q to be valid", status)
			}
		})
	}

	// States that cannot be directly cancelled
	nonCancelableStates := []ContestStatus{
		StatusRunning,
		StatusSettling,
		StatusCompleted,
		StatusCancelled,
	}

	for _, status := range nonCancelableStates {
		t.Run(string(status)+"_not_cancelable", func(t *testing.T) {
			if CanTransition(status, StatusCancelled) {
				t.Errorf("Did not expect cancellation from %q to be valid", status)
			}
		})
	}
}

func TestContestLifecycle_PausePath(t *testing.T) {
	// Test pause and resume cycle
	if !CanTransition(StatusRunning, StatusPaused) {
		t.Error("Expected running -> paused transition to be valid")
	}

	if !CanTransition(StatusPaused, StatusRunning) {
		t.Error("Expected paused -> running transition to be valid")
	}

	if !CanTransition(StatusPaused, StatusSettling) {
		t.Error("Expected paused -> settling transition to be valid")
	}
}

func TestParsePostgresInterval(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
	}{
		{"", 0},
		{"0 seconds", 0},
		{"00:00:00", 0},
		{"00:10:00", 10 * time.Minute},
		{"01:30:00", 90 * time.Minute},
		{"00:00:30", 30 * time.Second},
		{"00:00:05.123456", 5*time.Second + 123456*time.Microsecond},
		{"00:00:01.5", 1500 * time.Millisecond},
		{"1 day 02:30:00", 26*time.Hour + 30*time.Minute},
		{"2 days 00:00:00", 48 * time.Hour},
		{"1 day 00:00:05.5", 24*time.Hour + 5500*time.Millisecond},
		{"30 seconds", 30 * time.Second},
		{"1 second", 1 * time.Second},
		{"-00:05:00", -5 * time.Minute},
		{"-1 day 02:00:00", -(26 * time.Hour)},
		{"invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parsePostgresInterval(tt.input)
			if got != tt.want {
				t.Errorf("parsePostgresInterval(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
