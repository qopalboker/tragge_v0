package statemachine

import (
	"context"
	"testing"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

func TestHandlersConfig_Defaults(t *testing.T) {
	cfg := DefaultHandlersConfig()

	if cfg == nil {
		t.Fatal("DefaultHandlersConfig() returned nil")
	}

	if cfg.ContestEventsTopic != "contests.v1" {
		t.Errorf("ContestEventsTopic = %q, want %q", cfg.ContestEventsTopic, "contests.v1")
	}

	if cfg.NotificationsTopic != "notifications.v1" {
		t.Errorf("NotificationsTopic = %q, want %q", cfg.NotificationsTopic, "notifications.v1")
	}

	if cfg.ClosePositionsTopic != "close_positions.v1" {
		t.Errorf("ClosePositionsTopic = %q, want %q", cfg.ClosePositionsTopic, "close_positions.v1")
	}

	if cfg.CancelOrdersTopic != "cancel_orders.v1" {
		t.Errorf("CancelOrdersTopic = %q, want %q", cfg.CancelOrdersTopic, "cancel_orders.v1")
	}

	if cfg.LeaderboardInitTopic != "leaderboard_init.v1" {
		t.Errorf("LeaderboardInitTopic = %q, want %q", cfg.LeaderboardInitTopic, "leaderboard_init.v1")
	}

	if cfg.WinnerPercentage != 0.30 {
		t.Errorf("WinnerPercentage = %f, want %f", cfg.WinnerPercentage, 0.30)
	}
}

func TestContestEventTypes(t *testing.T) {
	tests := []struct {
		eventType contracts.ContestEventType
		expected  string
	}{
		{contracts.ContestEventStarted, "contest_started"},
		{contracts.ContestEventPaused, "contest_paused"},
		{contracts.ContestEventResumed, "contest_resumed"},
		{contracts.ContestEventTimeExtended, "contest_time_extended"},
		{contracts.ContestEventTradingEnded, "trading_ended"},
		{contracts.ContestEventSettling, "contest_settling"},
		{contracts.ContestEventCompleted, "contest_completed"},
		{contracts.ContestEventCancelled, "contest_cancelled"},
		{contracts.ContestEventResultsReady, "results_ready"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.eventType) != tt.expected {
				t.Errorf("ContestEventType = %q, want %q", tt.eventType, tt.expected)
			}
		})
	}
}

func TestContestNotificationTypes(t *testing.T) {
	tests := []struct {
		notifType contracts.ContestNotificationType
		expected  string
	}{
		{contracts.NotificationContestStarted, "contest_started"},
		{contracts.NotificationContestEnding, "contest_ending"},
		{contracts.NotificationTradingEnded, "trading_ended"},
		{contracts.NotificationResultsReady, "results_ready"},
		{contracts.NotificationContestCancelled, "contest_cancelled"},
		{contracts.NotificationPrizeWon, "prize_won"},
		{contracts.NotificationContestCompleted, "contest_completed"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.notifType) != tt.expected {
				t.Errorf("ContestNotificationType = %q, want %q", tt.notifType, tt.expected)
			}
		})
	}
}

func TestNotificationChannels(t *testing.T) {
	tests := []struct {
		channel  contracts.NotificationChannel
		expected string
	}{
		{contracts.ChannelPush, "push"},
		{contracts.ChannelEmail, "email"},
		{contracts.ChannelInApp, "in_app"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.channel) != tt.expected {
				t.Errorf("NotificationChannel = %q, want %q", tt.channel, tt.expected)
			}
		})
	}
}

func TestNotificationPriorities(t *testing.T) {
	tests := []struct {
		priority contracts.NotificationPriority
		expected string
	}{
		{contracts.PriorityLow, "low"},
		{contracts.PriorityNormal, "normal"},
		{contracts.PriorityHigh, "high"},
		{contracts.PriorityCritical, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.priority) != tt.expected {
				t.Errorf("NotificationPriority = %q, want %q", tt.priority, tt.expected)
			}
		})
	}
}

func TestFinalRanking_Struct(t *testing.T) {
	ranking := contracts.FinalRanking{
		UserID:     "user-123",
		Rank:       1,
		TotalScore: 1500.50,
		PrizeCents: 10000,
	}

	if ranking.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", ranking.UserID, "user-123")
	}

	if ranking.Rank != 1 {
		t.Errorf("Rank = %d, want %d", ranking.Rank, 1)
	}

	if ranking.TotalScore != 1500.50 {
		t.Errorf("TotalScore = %f, want %f", ranking.TotalScore, 1500.50)
	}

	if ranking.PrizeCents != 10000 {
		t.Errorf("PrizeCents = %d, want %d", ranking.PrizeCents, 10000)
	}
}

func TestContestResults_Struct(t *testing.T) {
	now := time.Now().UnixMilli()
	results := contracts.ContestResults{
		ContestID:         "contest-123",
		ContestName:       "Test Contest",
		TotalParticipants: 100,
		WinnersCount:      30,
		PrizePoolCents:    1000000,
		TotalPaidCents:    975000,
		Rankings: []contracts.FinalRanking{
			{UserID: "user-1", Rank: 1, TotalScore: 2000.0, PrizeCents: 500000},
			{UserID: "user-2", Rank: 2, TotalScore: 1800.0, PrizeCents: 300000},
		},
		FinalizedAt: now,
	}

	if results.ContestID != "contest-123" {
		t.Errorf("ContestID = %q, want %q", results.ContestID, "contest-123")
	}

	if results.TotalParticipants != 100 {
		t.Errorf("TotalParticipants = %d, want %d", results.TotalParticipants, 100)
	}

	if results.WinnersCount != 30 {
		t.Errorf("WinnersCount = %d, want %d", results.WinnersCount, 30)
	}

	if len(results.Rankings) != 2 {
		t.Errorf("Rankings length = %d, want %d", len(results.Rankings), 2)
	}
}

func TestClosePositionsRequest_Struct(t *testing.T) {
	closePrice := 150.50
	req := contracts.ClosePositionsRequest{
		ContestID:  "contest-123",
		Reason:     "contest_ended",
		ClosePrice: &closePrice,
		Ts:         time.Now().UnixMilli(),
	}

	if req.ContestID != "contest-123" {
		t.Errorf("ContestID = %q, want %q", req.ContestID, "contest-123")
	}

	if req.Reason != "contest_ended" {
		t.Errorf("Reason = %q, want %q", req.Reason, "contest_ended")
	}

	if req.ClosePrice == nil || *req.ClosePrice != closePrice {
		t.Errorf("ClosePrice = %v, want %f", req.ClosePrice, closePrice)
	}
}

func TestCancelAllOrdersRequest_Struct(t *testing.T) {
	req := contracts.CancelAllOrdersRequest{
		ContestID: "contest-123",
		Reason:    contracts.CancelReasonContestEnded,
		Ts:        time.Now().UnixMilli(),
	}

	if req.ContestID != "contest-123" {
		t.Errorf("ContestID = %q, want %q", req.ContestID, "contest-123")
	}

	if req.Reason != contracts.CancelReasonContestEnded {
		t.Errorf("Reason = %q, want %q", req.Reason, contracts.CancelReasonContestEnded)
	}
}

func TestContestEvent_Struct(t *testing.T) {
	now := time.Now().UnixMilli()
	endsAt := time.Now().Add(2 * time.Hour).UnixMilli()

	event := contracts.ContestEvent{
		Type:      contracts.ContestEventStarted,
		ContestID: "contest-123",
		Name:      "Test Contest",
		EndsAt:    endsAt,
		Message:   "Contest has started!",
		Metadata: map[string]any{
			"participants": 100,
			"qty_total":    100000,
		},
		Ts: now,
	}

	if event.Type != contracts.ContestEventStarted {
		t.Errorf("Type = %q, want %q", event.Type, contracts.ContestEventStarted)
	}

	if event.ContestID != "contest-123" {
		t.Errorf("ContestID = %q, want %q", event.ContestID, "contest-123")
	}

	if event.Name != "Test Contest" {
		t.Errorf("Name = %q, want %q", event.Name, "Test Contest")
	}

	if event.EndsAt != endsAt {
		t.Errorf("EndsAt = %d, want %d", event.EndsAt, endsAt)
	}

	if event.Message != "Contest has started!" {
		t.Errorf("Message = %q, want %q", event.Message, "Contest has started!")
	}

	if event.Metadata["participants"] != 100 {
		t.Errorf("Metadata[participants] = %v, want %d", event.Metadata["participants"], 100)
	}
}

func TestContestNotification_Struct(t *testing.T) {
	now := time.Now().UnixMilli()

	notification := contracts.ContestNotification{
		Type:      contracts.NotificationContestStarted,
		ContestID: "contest-123",
		UserID:    "user-456",
		Title:     "Contest Started",
		Body:      "Test Contest has started!",
		Data: map[string]any{
			"contest_name": "Test Contest",
		},
		Channels: []contracts.NotificationChannel{
			contracts.ChannelPush,
			contracts.ChannelInApp,
		},
		Priority: contracts.PriorityHigh,
		Ts:       now,
	}

	if notification.Type != contracts.NotificationContestStarted {
		t.Errorf("Type = %q, want %q", notification.Type, contracts.NotificationContestStarted)
	}

	if notification.ContestID != "contest-123" {
		t.Errorf("ContestID = %q, want %q", notification.ContestID, "contest-123")
	}

	if notification.UserID != "user-456" {
		t.Errorf("UserID = %q, want %q", notification.UserID, "user-456")
	}

	if len(notification.Channels) != 2 {
		t.Errorf("Channels length = %d, want %d", len(notification.Channels), 2)
	}

	if notification.Priority != contracts.PriorityHigh {
		t.Errorf("Priority = %q, want %q", notification.Priority, contracts.PriorityHigh)
	}
}

func TestContestHandlers_New_WithNilConfig(t *testing.T) {
	handlers := NewContestHandlers(nil, nil, nil, nil)

	if handlers == nil {
		t.Fatal("NewContestHandlers() returned nil")
	}

	if handlers.config == nil {
		t.Fatal("handlers.config is nil")
	}

	// Should use default config
	if handlers.config.WinnerPercentage != 0.30 {
		t.Errorf("WinnerPercentage = %f, want %f", handlers.config.WinnerPercentage, 0.30)
	}
}

func TestContestHandlers_New_WithConfig(t *testing.T) {
	customConfig := &HandlersConfig{
		ContestEventsTopic:   "custom_events.v1",
		NotificationsTopic:   "custom_notifications.v1",
		ClosePositionsTopic:  "custom_close.v1",
		CancelOrdersTopic:    "custom_cancel.v1",
		LeaderboardInitTopic: "custom_leaderboard.v1",
		WinnerPercentage:     0.40,
	}

	handlers := NewContestHandlers(nil, nil, nil, customConfig)

	if handlers == nil {
		t.Fatal("NewContestHandlers() returned nil")
	}

	if handlers.config.ContestEventsTopic != "custom_events.v1" {
		t.Errorf("ContestEventsTopic = %q, want %q", handlers.config.ContestEventsTopic, "custom_events.v1")
	}

	if handlers.config.WinnerPercentage != 0.40 {
		t.Errorf("WinnerPercentage = %f, want %f", handlers.config.WinnerPercentage, 0.40)
	}
}

// TestHandleContestStart_NoPool tests that HandleContestStart works without a database pool.
func TestHandleContestStart_NoPool(t *testing.T) {
	handlers := NewContestHandlers(nil, nil, nil, nil)

	contest := &Contest{
		ID:                  "contest-123",
		Name:                "Test Contest",
		QtyTotal:            100000,
		CurrentParticipants: 10,
		EndsAt:              time.Now().Add(2 * time.Hour),
	}

	result := &TransitionResult{
		Contest: contest,
		FromStatus:    StatusRegistrationClosed,
		ToStatus:      StatusRunning,
	}

	ctx := context.Background()

	// This should not panic even without a database pool
	err := handlers.HandleContestStart(ctx, result)
	if err == nil {
		t.Log("HandleContestStart returned no error (expected when pool is nil)")
	}
}

// TestHandleContestEnd_NoPool tests that HandleContestEnd works without a database pool.
func TestHandleContestEnd_NoPool(t *testing.T) {
	handlers := NewContestHandlers(nil, nil, nil, nil)

	contest := &Contest{
		ID:                  "contest-123",
		Name:                "Test Contest",
		QtyTotal:            100000,
		CurrentParticipants: 10,
		EndsAt:              time.Now(),
	}

	result := &TransitionResult{
		Contest: contest,
		FromStatus:    StatusRunning,
		ToStatus:      StatusSettling,
	}

	ctx := context.Background()

	// This should not panic even without a database pool
	err := handlers.HandleContestEnd(ctx, result)
	if err == nil {
		t.Log("HandleContestEnd returned no error (expected when pool is nil)")
	}
}

// TestHandleSettlement_NoPool tests that HandleSettlement works without a database pool.
func TestHandleSettlement_NoPool(t *testing.T) {
	handlers := NewContestHandlers(nil, nil, nil, nil)

	contest := &Contest{
		ID:                  "contest-123",
		Name:                "Test Contest",
		QtyTotal:            100000,
		CurrentParticipants: 10,
		EntryFeeCents:       1000,
	}

	result := &TransitionResult{
		Contest: contest,
		FromStatus:    StatusSettling,
		ToStatus:      StatusCompleted,
	}

	ctx := context.Background()

	// This should not panic even without a database pool
	err := handlers.HandleSettlement(ctx, result)
	if err == nil {
		t.Log("HandleSettlement returned no error (expected when pool is nil)")
	}
}
