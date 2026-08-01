package v1

import (
	"encoding/json"
	"testing"
)

// --- JSON round-trip tests ---

func TestOrderRequestRoundTrip(t *testing.T) {
	lp := 150.0
	sp := 145.0
	tp := 160.0
	sl := 140.0
	orig := OrderRequest{
		OrderID:    "ord-123",
		UserID:     "user-456",
		ContestID:  "contest-789",
		Symbol:     "AAPL",
		Side:       OrderSideBuy,
		Type:       OrderTypeBuyLimit,
		Qty:        100,
		LimitPrice: &lp,
		StopPrice:  &sp,
		TakeProfit: &tp,
		StopLoss:   &sl,
		ClientTs:   1700000000,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded OrderRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.OrderID != orig.OrderID {
		t.Errorf("OrderID = %s, want %s", decoded.OrderID, orig.OrderID)
	}
	if decoded.UserID != orig.UserID {
		t.Errorf("UserID = %s, want %s", decoded.UserID, orig.UserID)
	}
	if decoded.ContestID != orig.ContestID {
		t.Errorf("ContestID = %s, want %s", decoded.ContestID, orig.ContestID)
	}
	if decoded.Symbol != orig.Symbol {
		t.Errorf("Symbol = %s, want %s", decoded.Symbol, orig.Symbol)
	}
	if decoded.Side != orig.Side {
		t.Errorf("Side = %s, want %s", decoded.Side, orig.Side)
	}
	if decoded.Type != orig.Type {
		t.Errorf("Type = %s, want %s", decoded.Type, orig.Type)
	}
	if decoded.Qty != orig.Qty {
		t.Errorf("Qty = %d, want %d", decoded.Qty, orig.Qty)
	}
	if decoded.LimitPrice == nil || *decoded.LimitPrice != lp {
		t.Errorf("LimitPrice = %v, want %v", decoded.LimitPrice, lp)
	}
	if decoded.TakeProfit == nil || *decoded.TakeProfit != tp {
		t.Errorf("TakeProfit = %v, want %v", decoded.TakeProfit, tp)
	}
	if decoded.StopLoss == nil || *decoded.StopLoss != sl {
		t.Errorf("StopLoss = %v, want %v", decoded.StopLoss, sl)
	}
}

func TestFillEventRoundTrip(t *testing.T) {
	orig := FillEvent{
		FillID:    "fill-1",
		OrderID:   "ord-1",
		UserID:    "user-1",
		ContestID: "contest-1",
		Symbol:    "AAPL",
		Side:      OrderSideSell,
		Qty:       50,
		FillPrice: 155.25,
		Ts:        1700000001,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded FillEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.FillID != orig.FillID {
		t.Errorf("FillID = %s, want %s", decoded.FillID, orig.FillID)
	}
	if decoded.FillPrice != orig.FillPrice {
		t.Errorf("FillPrice = %f, want %f", decoded.FillPrice, orig.FillPrice)
	}
	if decoded.Side != orig.Side {
		t.Errorf("Side = %s, want %s", decoded.Side, orig.Side)
	}
}

func TestTickSnapshotRoundTrip(t *testing.T) {
	orig := TickSnapshot{
		Ts: 1700000002,
		Symbols: []SymbolTick{
			{Symbol: "AAPL", Bid: 150.0, Ask: 150.1, Last: 150.05},
			{Symbol: "GOOGL", Bid: 140.0, Ask: 140.2, Last: 140.1},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded TickSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Ts != orig.Ts {
		t.Errorf("Ts = %d, want %d", decoded.Ts, orig.Ts)
	}
	if len(decoded.Symbols) != 2 {
		t.Fatalf("Symbols length = %d, want 2", len(decoded.Symbols))
	}
	if decoded.Symbols[0].Symbol != "AAPL" {
		t.Errorf("Symbols[0].Symbol = %s, want AAPL", decoded.Symbols[0].Symbol)
	}
}

func TestPnLDeltaRoundTrip(t *testing.T) {
	orig := PnLDelta{
		UserID:          "user-1",
		ContestID:       "contest-1",
		DeltaScore:      10.5,
		RealizedScore:   100.0,
		UnrealizedScore: 50.0,
		TotalScore:      150.0,
		Ts:              1700000003,
		SeqNum:          42,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded PnLDelta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.UserID != orig.UserID {
		t.Errorf("UserID = %s, want %s", decoded.UserID, orig.UserID)
	}
	if decoded.TotalScore != orig.TotalScore {
		t.Errorf("TotalScore = %f, want %f", decoded.TotalScore, orig.TotalScore)
	}
	if decoded.SeqNum != orig.SeqNum {
		t.Errorf("SeqNum = %d, want %d", decoded.SeqNum, orig.SeqNum)
	}
}

func TestContestStateRoundTrip(t *testing.T) {
	orig := ContestState{
		ContestID: "contest-1",
		Phase:     ContestPhaseLive,
		Status:    ContestStatusRunning,
		Ts:        1700000004,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ContestState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ContestID != orig.ContestID {
		t.Errorf("ContestID = %s, want %s", decoded.ContestID, orig.ContestID)
	}
	if decoded.Phase != orig.Phase {
		t.Errorf("Phase = %s, want %s", decoded.Phase, orig.Phase)
	}
	if decoded.Status != orig.Status {
		t.Errorf("Status = %s, want %s", decoded.Status, orig.Status)
	}
}

func TestPositionUpdateRoundTrip(t *testing.T) {
	orig := PositionUpdate{
		UserID:    "user-1",
		ContestID: "contest-1",
		Positions: []Position{
			{
				PositionID: "pos-1",
				Symbol:     "AAPL",
				Side:       OrderSideBuy,
				Qty:        100,
				EntryPrice: 150.0,
				MarkPrice:  155.0,
			},
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded PositionUpdate
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.UserID != orig.UserID {
		t.Errorf("UserID = %s, want %s", decoded.UserID, orig.UserID)
	}
	if len(decoded.Positions) != 1 {
		t.Fatalf("Positions length = %d, want 1", len(decoded.Positions))
	}
	if decoded.Positions[0].Symbol != "AAPL" {
		t.Errorf("Positions[0].Symbol = %s, want AAPL", decoded.Positions[0].Symbol)
	}
}

func TestOrderAckRoundTrip(t *testing.T) {
	reason := "insufficient balance"
	orig := OrderAck{
		OrderID: "ord-1",
		Status:  OrderStatusRejected,
		Reason:  &reason,
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded OrderAck
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.OrderID != orig.OrderID {
		t.Errorf("OrderID = %s, want %s", decoded.OrderID, orig.OrderID)
	}
	if decoded.Status != orig.Status {
		t.Errorf("Status = %s, want %s", decoded.Status, orig.Status)
	}
	if decoded.Reason == nil || *decoded.Reason != reason {
		t.Errorf("Reason = %v, want %s", decoded.Reason, reason)
	}
}

// --- Enum string value verification ---

func TestOrderSideValues(t *testing.T) {
	if string(OrderSideBuy) != "BUY" {
		t.Errorf("OrderSideBuy = %s, want BUY", OrderSideBuy)
	}
	if string(OrderSideSell) != "SELL" {
		t.Errorf("OrderSideSell = %s, want SELL", OrderSideSell)
	}
}

func TestOrderTypeValues(t *testing.T) {
	tests := []struct {
		val  OrderType
		want string
	}{
		{OrderTypeMarket, "MARKET"},
		{OrderTypeBuyLimit, "BUY_LIMIT"},
		{OrderTypeSellLimit, "SELL_LIMIT"},
		{OrderTypeBuyStop, "BUY_STOP"},
		{OrderTypeSellStop, "SELL_STOP"},
	}
	for _, tt := range tests {
		if string(tt.val) != tt.want {
			t.Errorf("OrderType = %s, want %s", tt.val, tt.want)
		}
	}
}

func TestOrderModeValues(t *testing.T) {
	if string(OrderModeMarket) != "MARKET" {
		t.Errorf("OrderModeMarket = %s, want MARKET", OrderModeMarket)
	}
	if string(OrderModePending) != "PENDING" {
		t.Errorf("OrderModePending = %s, want PENDING", OrderModePending)
	}
}

func TestOrderStatusValues(t *testing.T) {
	if string(OrderStatusAccepted) != "ACCEPTED" {
		t.Errorf("OrderStatusAccepted = %s, want ACCEPTED", OrderStatusAccepted)
	}
	if string(OrderStatusRejected) != "REJECTED" {
		t.Errorf("OrderStatusRejected = %s, want REJECTED", OrderStatusRejected)
	}
}

func TestContestPhaseValues(t *testing.T) {
	tests := []struct {
		val  ContestPhase
		want string
	}{
		{ContestPhaseUpcoming, "UPCOMING"},
		{ContestPhaseLive, "LIVE"},
		{ContestPhaseFrozen, "FROZEN"},
		{ContestPhaseEnded, "ENDED"},
		{ContestPhaseCancelled, "CANCELLED"},
	}
	for _, tt := range tests {
		if string(tt.val) != tt.want {
			t.Errorf("ContestPhase = %s, want %s", tt.val, tt.want)
		}
	}
}

func TestContestStatusValid(t *testing.T) {
	validStatuses := []ContestStatus{
		ContestStatusDraft, ContestStatusScheduled, ContestStatusRegistrationOpen,
		ContestStatusRegistrationClosed, ContestStatusRunning, ContestStatusPaused,
		ContestStatusSettling, ContestStatusCompleted, ContestStatusCancelled,
	}
	for _, s := range validStatuses {
		if !s.IsValid() {
			t.Errorf("ContestStatus(%s).IsValid() = false, want true", s)
		}
	}

	if ContestStatus("invalid").IsValid() {
		t.Error("ContestStatus(invalid).IsValid() = true, want false")
	}
}

func TestOrderTypeIsPending(t *testing.T) {
	if OrderTypeMarket.IsPending() {
		t.Error("MARKET should not be pending")
	}
	if !OrderTypeBuyLimit.IsPending() {
		t.Error("BUY_LIMIT should be pending")
	}
	if !OrderTypeSellLimit.IsPending() {
		t.Error("SELL_LIMIT should be pending")
	}
	if !OrderTypeBuyStop.IsPending() {
		t.Error("BUY_STOP should be pending")
	}
	if !OrderTypeSellStop.IsPending() {
		t.Error("SELL_STOP should be pending")
	}
}

func TestContestStatusToPhase(t *testing.T) {
	tests := []struct {
		status ContestStatus
		phase  ContestPhase
	}{
		{ContestStatusDraft, ContestPhaseUpcoming},
		{ContestStatusScheduled, ContestPhaseUpcoming},
		{ContestStatusRunning, ContestPhaseLive},
		{ContestStatusPaused, ContestPhaseFrozen},
		{ContestStatusCompleted, ContestPhaseEnded},
		{ContestStatusCancelled, ContestPhaseCancelled},
	}
	for _, tt := range tests {
		if got := tt.status.ToPhase(); got != tt.phase {
			t.Errorf("%s.ToPhase() = %s, want %s", tt.status, got, tt.phase)
		}
	}
}
