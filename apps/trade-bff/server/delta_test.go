package server

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewDeltaEncoder(t *testing.T) {
	de := NewDeltaEncoder()
	if de == nil {
		t.Fatal("NewDeltaEncoder returned nil")
	}
	if de.userState == nil {
		t.Error("userState map not initialized")
	}
	if de.metrics == nil {
		t.Error("metrics not initialized")
	}
}

func TestEncodeDelta_FirstSync(t *testing.T) {
	de := NewDeltaEncoder()

	state := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {
				Symbol:        "AAPL",
				Side:          "long",
				UnrealizedPnL: 100.50,
				CurrentPrice:  150.00,
				QtyOpen:       100,
				AvgPrice:      148.00,
			},
		},
		Balance: &BalanceSnapshot{
			Available: 10000,
			Total:     15000,
			Equity:    15100.50,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	delta, hasChanges := de.EncodeDelta("user1", state)

	if !hasChanges {
		t.Error("Expected hasChanges to be true for first sync")
	}
	if delta == nil {
		t.Fatal("Delta should not be nil")
	}
	if !delta.FullSync {
		t.Error("Expected FullSync to be true for first sync")
	}
	if delta.Type != "state_delta" {
		t.Errorf("Expected type 'state_delta', got '%s'", delta.Type)
	}
	if len(delta.Positions) != 1 {
		t.Errorf("Expected 1 position, got %d", len(delta.Positions))
	}
	if delta.Balance == nil {
		t.Error("Expected balance to be included in full sync")
	}
}

func TestEncodeDelta_NoChanges(t *testing.T) {
	de := NewDeltaEncoder()

	state := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {
				Symbol:        "AAPL",
				UnrealizedPnL: 100.50,
				CurrentPrice:  150.00,
				QtyOpen:       100,
			},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	// First call - full sync
	de.EncodeDelta("user1", state)

	// Second call with same state - no changes
	delta, hasChanges := de.EncodeDelta("user1", state)

	if hasChanges {
		t.Error("Expected hasChanges to be false when state unchanged")
	}
	if len(delta.Positions) != 0 {
		t.Errorf("Expected 0 position changes, got %d", len(delta.Positions))
	}
	if delta.FullSync {
		t.Error("FullSync should be false for subsequent calls")
	}
}

func TestEncodeDelta_PositionChange(t *testing.T) {
	de := NewDeltaEncoder()

	// Initial state
	state1 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {
				Symbol:        "AAPL",
				UnrealizedPnL: 100.50,
				CurrentPrice:  150.00,
				QtyOpen:       100,
			},
		},
		Timestamp: time.Now().UnixMilli(),
	}
	de.EncodeDelta("user1", state1)

	// Updated state - PnL changed
	state2 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {
				Symbol:        "AAPL",
				UnrealizedPnL: 150.75, // Changed
				CurrentPrice:  152.00, // Changed
				QtyOpen:       100,    // Unchanged
			},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	delta, hasChanges := de.EncodeDelta("user1", state2)

	if !hasChanges {
		t.Error("Expected hasChanges to be true")
	}
	if len(delta.Positions) != 1 {
		t.Fatalf("Expected 1 position delta, got %d", len(delta.Positions))
	}

	posDelta := delta.Positions[0]
	if posDelta.PositionID != "pos1" {
		t.Errorf("Expected position ID 'pos1', got '%s'", posDelta.PositionID)
	}

	// Check that only changed fields are in the delta
	if _, ok := posDelta.Changes["pnl"]; !ok {
		t.Error("Expected 'pnl' to be in changes")
	}
	if _, ok := posDelta.Changes["cp"]; !ok {
		t.Error("Expected 'cp' (current price) to be in changes")
	}
	if _, ok := posDelta.Changes["qty"]; ok {
		t.Error("'qty' should not be in changes (unchanged)")
	}
}

func TestEncodeDelta_NewPosition(t *testing.T) {
	de := NewDeltaEncoder()

	// Initial state with one position
	state1 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 100},
		},
		Timestamp: time.Now().UnixMilli(),
	}
	de.EncodeDelta("user1", state1)

	// Add a new position
	state2 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 100},
			"pos2": {Symbol: "GOOG", UnrealizedPnL: 200, QtyOpen: 50},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	delta, hasChanges := de.EncodeDelta("user1", state2)

	if !hasChanges {
		t.Error("Expected hasChanges to be true")
	}

	// Find the new position delta
	var newPosDelta *PositionDelta
	for i := range delta.Positions {
		if delta.Positions[i].PositionID == "pos2" {
			newPosDelta = &delta.Positions[i]
			break
		}
	}

	if newPosDelta == nil {
		t.Fatal("New position 'pos2' not found in delta")
	}

	if newPosDelta.Changes["new"] != true {
		t.Error("Expected 'new' flag to be true for new position")
	}
	if newPosDelta.Changes["s"] != "GOOG" {
		t.Error("Expected symbol 'GOOG' in new position")
	}
}

func TestEncodeDelta_ClosedPosition(t *testing.T) {
	de := NewDeltaEncoder()

	// Initial state with two positions
	state1 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 100},
			"pos2": {Symbol: "GOOG", UnrealizedPnL: 200},
		},
		Timestamp: time.Now().UnixMilli(),
	}
	de.EncodeDelta("user1", state1)

	// Remove one position (closed)
	state2 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 100},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	delta, hasChanges := de.EncodeDelta("user1", state2)

	if !hasChanges {
		t.Error("Expected hasChanges to be true")
	}

	// Find the closed position delta
	var closedPosDelta *PositionDelta
	for i := range delta.Positions {
		if delta.Positions[i].PositionID == "pos2" {
			closedPosDelta = &delta.Positions[i]
			break
		}
	}

	if closedPosDelta == nil {
		t.Fatal("Closed position 'pos2' not found in delta")
	}

	if closedPosDelta.Changes["closed"] != true {
		t.Error("Expected 'closed' flag to be true")
	}
}

func TestEncodeDelta_BalanceChange(t *testing.T) {
	de := NewDeltaEncoder()

	// Initial state
	state1 := &UserStateSnapshot{
		Balance: &BalanceSnapshot{
			Available: 10000,
			Total:     15000,
			Equity:    15000,
		},
		Timestamp: time.Now().UnixMilli(),
	}
	de.EncodeDelta("user1", state1)

	// Changed equity
	state2 := &UserStateSnapshot{
		Balance: &BalanceSnapshot{
			Available: 10000, // Unchanged
			Total:     15000, // Unchanged
			Equity:    15500, // Changed
		},
		Timestamp: time.Now().UnixMilli(),
	}

	delta, hasChanges := de.EncodeDelta("user1", state2)

	if !hasChanges {
		t.Error("Expected hasChanges to be true")
	}
	if delta.Balance == nil {
		t.Fatal("Expected balance changes in delta")
	}
	if _, ok := delta.Balance["eq"]; !ok {
		t.Error("Expected 'eq' (equity) in balance changes")
	}
	if _, ok := delta.Balance["avail"]; ok {
		t.Error("'avail' should not be in changes (unchanged)")
	}
}

func TestRemoveUser(t *testing.T) {
	de := NewDeltaEncoder()

	state := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL"},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	de.EncodeDelta("user1", state)

	if de.GetUserCount() != 1 {
		t.Errorf("Expected 1 user, got %d", de.GetUserCount())
	}

	de.RemoveUser("user1")

	if de.GetUserCount() != 0 {
		t.Errorf("Expected 0 users after removal, got %d", de.GetUserCount())
	}

	// After removal, next encode should be a full sync
	delta, _ := de.EncodeDelta("user1", state)
	if !delta.FullSync {
		t.Error("Expected full sync after user was removed and re-added")
	}
}

func TestForceFullSync(t *testing.T) {
	de := NewDeltaEncoder()

	state := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL"},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	// Initial sync
	de.EncodeDelta("user1", state)

	// Force full sync
	de.ForceFullSync("user1")

	// Next encode should be a full sync
	delta, _ := de.EncodeDelta("user1", state)
	if !delta.FullSync {
		t.Error("Expected full sync after ForceFullSync")
	}
}

func TestDeltaGetMetrics(t *testing.T) {
	de := NewDeltaEncoder()

	state1 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 100},
		},
		Timestamp: time.Now().UnixMilli(),
	}

	// Generate some encodings
	de.EncodeDelta("user1", state1)

	state2 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 150}, // Changed
		},
		Timestamp: time.Now().UnixMilli(),
	}
	de.EncodeDelta("user1", state2)

	metrics := de.GetMetrics()

	if metrics["total_encodings"].(int64) != 2 {
		t.Errorf("Expected 2 total encodings, got %v", metrics["total_encodings"])
	}
	if metrics["full_syncs"].(int64) != 1 {
		t.Errorf("Expected 1 full sync, got %v", metrics["full_syncs"])
	}
	if metrics["deltas_generated"].(int64) != 1 {
		t.Errorf("Expected 1 delta generated, got %v", metrics["deltas_generated"])
	}
}

func TestDeltaCompressionEfficiency(t *testing.T) {
	de := NewDeltaEncoder()

	// Create a state with many positions
	state1 := &UserStateSnapshot{
		Positions: make(map[string]*PositionSnapshot),
		Balance: &BalanceSnapshot{
			Available: 100000,
			Total:     150000,
			Equity:    150000,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	for i := 0; i < 10; i++ {
		state1.Positions[string(rune('A'+i))] = &PositionSnapshot{
			Symbol:        string(rune('A' + i)),
			UnrealizedPnL: float64(i * 100),
			CurrentPrice:  float64(100 + i),
			QtyOpen:       int64(100 + i),
		}
	}

	// Initial full sync
	delta1, _ := de.EncodeDelta("user1", state1)
	fullBytes, _ := json.Marshal(delta1)

	// Make small change (only 1 position PnL changed)
	state2 := state1.Clone()
	state2.Positions["A"].UnrealizedPnL = 999
	state2.Timestamp = time.Now().UnixMilli()

	delta2, _ := de.EncodeDelta("user1", state2)
	deltaBytes, _ := json.Marshal(delta2)

	// Delta should be much smaller than full state
	compressionRatio := float64(len(deltaBytes)) / float64(len(fullBytes))
	t.Logf("Full sync size: %d bytes", len(fullBytes))
	t.Logf("Delta size: %d bytes", len(deltaBytes))
	t.Logf("Compression ratio: %.2f%%", compressionRatio*100)

	if compressionRatio > 0.5 {
		t.Errorf("Delta compression ratio too high: %.2f%% (expected < 50%%)", compressionRatio*100)
	}
}

func TestEncodeDelta_FloatEpsilonNoFalsePositive(t *testing.T) {
	de := NewDeltaEncoder()

	state1 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {
				Symbol:        "AAPL",
				UnrealizedPnL: 100.0,
				CurrentPrice:  150.0,
				QtyOpen:       100,
				AvgPrice:      148.0,
			},
		},
		Balance: &BalanceSnapshot{
			Available: 10000,
			Total:     15000,
			Equity:    15000.0,
		},
		Timestamp: time.Now().UnixMilli(),
	}
	de.EncodeDelta("user1", state1)

	// Create state with tiny floating-point drift (below epsilon)
	state2 := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {
				Symbol:        "AAPL",
				UnrealizedPnL: 100.0 + 1e-10,
				CurrentPrice:  150.0 + 1e-10,
				QtyOpen:       100,
				AvgPrice:      148.0 + 1e-10,
			},
		},
		Balance: &BalanceSnapshot{
			Available: 10000,
			Total:     15000,
			Equity:    15000.0 + 1e-10,
		},
		Timestamp: time.Now().UnixMilli(),
	}

	_, hasChanges := de.EncodeDelta("user1", state2)
	if hasChanges {
		t.Error("Expected no changes for sub-epsilon float differences")
	}
}

func TestDeltaMetricsSampling(t *testing.T) {
	de := NewDeltaEncoder()

	baseState := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 0, CurrentPrice: 100, QtyOpen: 100},
		},
		Balance:   &BalanceSnapshot{Available: 10000, Total: 15000, Equity: 15000},
		Timestamp: time.Now().UnixMilli(),
	}
	de.EncodeDelta("user1", baseState)

	// Generate 99 deltas (not enough to trigger sampling)
	for i := 1; i <= 99; i++ {
		s := &UserStateSnapshot{
			Positions: map[string]*PositionSnapshot{
				"pos1": {Symbol: "AAPL", UnrealizedPnL: float64(i), CurrentPrice: 100 + float64(i), QtyOpen: 100},
			},
			Balance:   &BalanceSnapshot{Available: 10000, Total: 15000, Equity: 15000},
			Timestamp: time.Now().UnixMilli(),
		}
		de.EncodeDelta("user1", s)
	}

	metrics := de.GetMetrics()
	if metrics["total_original_bytes"].(int64) != 0 {
		t.Errorf("Expected 0 original bytes before sample threshold, got %v", metrics["total_original_bytes"])
	}

	// 100th delta triggers sampling
	s := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 999, CurrentPrice: 999, QtyOpen: 100},
		},
		Balance:   &BalanceSnapshot{Available: 10000, Total: 15000, Equity: 15000},
		Timestamp: time.Now().UnixMilli(),
	}
	de.EncodeDelta("user1", s)

	metrics = de.GetMetrics()
	if metrics["total_original_bytes"].(int64) == 0 {
		t.Error("Expected non-zero original bytes after sample threshold")
	}
	if metrics["deltas_generated"].(int64) != 100 {
		t.Errorf("Expected 100 deltas generated, got %v", metrics["deltas_generated"])
	}
}

func TestUserStateSnapshot_Clone(t *testing.T) {
	original := &UserStateSnapshot{
		Positions: map[string]*PositionSnapshot{
			"pos1": {Symbol: "AAPL", UnrealizedPnL: 100},
		},
		Balance: &BalanceSnapshot{
			Available: 10000,
		},
		Timestamp: 12345,
	}

	clone := original.Clone()

	// Modify clone
	clone.Positions["pos1"].UnrealizedPnL = 999
	clone.Balance.Available = 99999
	clone.Timestamp = 99999

	// Original should be unchanged
	if original.Positions["pos1"].UnrealizedPnL != 100 {
		t.Error("Original position was modified")
	}
	if original.Balance.Available != 10000 {
		t.Error("Original balance was modified")
	}
	if original.Timestamp != 12345 {
		t.Error("Original timestamp was modified")
	}
}
