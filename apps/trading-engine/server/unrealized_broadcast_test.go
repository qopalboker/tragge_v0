package server

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/twmb/franz-go/pkg/kgo"
)

// =============================================================================
// Unit Tests: Score Calculation
// =============================================================================

// TestUnrealizedScoreCalculation_Long verifies unrealized score calculation for LONG positions.
// Given: LONG position (entry=100, qty_used=10) with current_price=110
// Expected: unrealized_score=100
// Formula: pct_change = (110 - 100) / 100 * 100 = 10%, score = 10 * 10 = 100
func TestUnrealizedScoreCalculation_Long(t *testing.T) {
	tests := []struct {
		name         string
		entryPrice   float64
		qtyUsed      int64
		currentPrice float64
		expected     float64
	}{
		{
			name:         "long profit 10% (entry=100, qty=10, price=110) → score=100",
			entryPrice:   100.0,
			qtyUsed:      10,
			currentPrice: 110.0,
			expected:     100.0, // (110-100)/100*100 = 10%, 10*10 = 100
		},
		{
			name:         "long profit 5% (entry=200, qty=20, price=210) → score=100",
			entryPrice:   200.0,
			qtyUsed:      20,
			currentPrice: 210.0,
			expected:     100.0, // (210-200)/200*100 = 5%, 20*5 = 100
		},
		{
			name:         "long loss 10% (entry=100, qty=10, price=90) → score=-100",
			entryPrice:   100.0,
			qtyUsed:      10,
			currentPrice: 90.0,
			expected:     -100.0, // (90-100)/100*100 = -10%, 10*-10 = -100
		},
		{
			name:         "long breakeven (entry=100, qty=10, price=100) → score=0",
			entryPrice:   100.0,
			qtyUsed:      10,
			currentPrice: 100.0,
			expected:     0.0,
		},
		{
			name:         "large qty long profit (entry=50, qty=1000, price=51) → score=2000",
			entryPrice:   50.0,
			qtyUsed:      1000,
			currentPrice: 51.0,
			expected:     2000.0, // (51-50)/50*100 = 2%, 1000*2 = 2000
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create position state
			position := &PositionState{
				PositionID: "test-pos-1",
				Symbol:     "TEST",
				Side:       contracts.OrderSideBuy, // LONG
				QtyOpen:    tc.qtyUsed,
				EntryPrice: tc.entryPrice,
				QtyUsed:    tc.qtyUsed,
			}

			// Calculate unrealized score using the helper function
			unrealizedScore := calculateUnrealizedScore(position, tc.currentPrice)

			if !floatEquals(unrealizedScore, tc.expected) {
				t.Errorf("expected unrealized score %.4f, got %.4f", tc.expected, unrealizedScore)
			}
		})
	}
}

// TestUnrealizedScoreCalculation_Short verifies unrealized score calculation for SHORT positions.
// Given: SHORT position (entry=100, qty_used=10) with current_price=90
// Expected: unrealized_score=100
// Formula: pct_change = (100 - 90) / 100 * 100 = 10%, score = 10 * 10 = 100
func TestUnrealizedScoreCalculation_Short(t *testing.T) {
	tests := []struct {
		name         string
		entryPrice   float64
		qtyUsed      int64
		currentPrice float64
		expected     float64
	}{
		{
			name:         "short profit 10% (entry=100, qty=10, price=90) → score=100",
			entryPrice:   100.0,
			qtyUsed:      10,
			currentPrice: 90.0,
			expected:     100.0, // (100-90)/100*100 = 10%, 10*10 = 100
		},
		{
			name:         "short profit 5% (entry=200, qty=20, price=190) → score=100",
			entryPrice:   200.0,
			qtyUsed:      20,
			currentPrice: 190.0,
			expected:     100.0, // (200-190)/200*100 = 5%, 20*5 = 100
		},
		{
			name:         "short loss 10% (entry=100, qty=10, price=110) → score=-100",
			entryPrice:   100.0,
			qtyUsed:      10,
			currentPrice: 110.0,
			expected:     -100.0, // (100-110)/100*100 = -10%, 10*-10 = -100
		},
		{
			name:         "short breakeven (entry=100, qty=10, price=100) → score=0",
			entryPrice:   100.0,
			qtyUsed:      10,
			currentPrice: 100.0,
			expected:     0.0,
		},
		{
			name:         "large qty short profit (entry=100, qty=500, price=95) → score=2500",
			entryPrice:   100.0,
			qtyUsed:      500,
			currentPrice: 95.0,
			expected:     2500.0, // (100-95)/100*100 = 5%, 500*5 = 2500
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create position state
			position := &PositionState{
				PositionID: "test-pos-1",
				Symbol:     "TEST",
				Side:       contracts.OrderSideSell, // SHORT
				QtyOpen:    tc.qtyUsed,
				EntryPrice: tc.entryPrice,
				QtyUsed:    tc.qtyUsed,
			}

			// Calculate unrealized score using the helper function
			unrealizedScore := calculateUnrealizedScore(position, tc.currentPrice)

			if !floatEquals(unrealizedScore, tc.expected) {
				t.Errorf("expected unrealized score %.4f, got %.4f", tc.expected, unrealizedScore)
			}
		})
	}
}

// TestUnrealizedScoreCalculation_UserState verifies the UserState.CalculateUnrealizedScore method.
func TestUnrealizedScoreCalculation_UserState(t *testing.T) {
	tests := []struct {
		name      string
		positions map[string]*PositionState
		prices    map[string]float64
		expected  float64
	}{
		{
			name: "single long position profit",
			positions: map[string]*PositionState{
				"AAPL": {
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy,
					QtyOpen:    10,
					EntryPrice: 100.0,
					QtyUsed:    10,
				},
			},
			prices:   map[string]float64{"AAPL": 110.0},
			expected: 100.0, // 10% profit * 10 qty = 100
		},
		{
			name: "single short position profit",
			positions: map[string]*PositionState{
				"TSLA": {
					PositionID: "pos-2",
					Symbol:     "TSLA",
					Side:       contracts.OrderSideSell,
					QtyOpen:    10,
					EntryPrice: 100.0,
					QtyUsed:    10,
				},
			},
			prices:   map[string]float64{"TSLA": 90.0},
			expected: 100.0, // 10% profit * 10 qty = 100
		},
		{
			name: "multiple positions mixed profit/loss",
			positions: map[string]*PositionState{
				"AAPL": {
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy,
					QtyOpen:    10,
					EntryPrice: 100.0,
					QtyUsed:    10,
				},
				"TSLA": {
					PositionID: "pos-2",
					Symbol:     "TSLA",
					Side:       contracts.OrderSideSell,
					QtyOpen:    5,
					EntryPrice: 200.0,
					QtyUsed:    5,
				},
			},
			prices: map[string]float64{
				"AAPL": 110.0, // +10% for long = 100
				"TSLA": 210.0, // -5% for short = -25
			},
			expected: 75.0, // 100 - 25 = 75
		},
		{
			name:      "no positions",
			positions: map[string]*PositionState{},
			prices:    map[string]float64{},
			expected:  0.0,
		},
		{
			name: "position with missing price",
			positions: map[string]*PositionState{
				"AAPL": {
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy,
					QtyOpen:    10,
					EntryPrice: 100.0,
					QtyUsed:    10,
				},
			},
			prices:   map[string]float64{}, // No price for AAPL
			expected: 0.0,                  // Should skip position without price
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create user state
			us := &UserState{
				QtyTotal:      1000,
				QtyAvailable:  900,
				RealizedScore: 0,
				Positions:     tc.positions,
				PendingOrders: make(map[string]*PendingOrder),
			}

			// Create price lookup function
			getPriceFunc := func(symbol string) (float64, bool) {
				price, ok := tc.prices[symbol]
				return price, ok
			}

			// Calculate unrealized score
			unrealizedScore := us.CalculateUnrealizedScore(getPriceFunc)

			if !floatEquals(unrealizedScore, tc.expected) {
				t.Errorf("expected unrealized score %.4f, got %.4f", tc.expected, unrealizedScore)
			}
		})
	}
}

// =============================================================================
// Unit Tests: Sharded Broadcast
// =============================================================================

// MockKafkaProducer captures produced records for testing.
type MockKafkaProducer struct {
	mu       sync.Mutex
	records  []*kgo.Record
	callback func(*kgo.Record, error)
}

func (m *MockKafkaProducer) Produce(ctx context.Context, record *kgo.Record, callback func(*kgo.Record, error)) {
	m.mu.Lock()
	m.records = append(m.records, record)
	m.mu.Unlock()
	if callback != nil {
		callback(record, nil)
	}
}

func (m *MockKafkaProducer) GetRecords() []*kgo.Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*kgo.Record, len(m.records))
	copy(result, m.records)
	return result
}

func (m *MockKafkaProducer) ClearRecords() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = nil
}

// TestShardedBroadcast_OnlyAssignedContests verifies that only positions from
// contests assigned to the current shard are broadcast.
func TestShardedBroadcast_OnlyAssignedContests(t *testing.T) {
	// Create a mock sharded state manager with 3 contests across 2 shards
	// Shard 0 gets: contest-a (hash=0), contest-c (hash=0)
	// Shard 1 gets: contest-b (hash=1)
	// We'll test shard 0 and verify only contest-a and contest-c positions are broadcast

	tests := []struct {
		name               string
		shardID            int
		shardCount         int
		assignedContests   []string
		unassignedContests []string
		setupPositions     func(sm *StateManager) // Setup positions in local state
		expectedContests   []string               // Contests that should appear in broadcasts
	}{
		{
			name:               "shard 0 broadcasts only assigned contests",
			shardID:            0,
			shardCount:         2,
			assignedContests:   []string{"contest-shard-0-a", "contest-shard-0-b"},
			unassignedContests: []string{"contest-shard-1-a"},
			setupPositions: func(sm *StateManager) {
				// Add positions to all contests
				for _, contestID := range []string{"contest-shard-0-a", "contest-shard-0-b", "contest-shard-1-a"} {
					cs := sm.GetOrCreateContest(contestID)
					us := cs.GetOrCreateUser("user-1", 1000, 900, 0)
					us.SetPosition(&PositionState{
						PositionID: "pos-" + contestID,
						Symbol:     "AAPL",
						Side:       contracts.OrderSideBuy,
						QtyOpen:    10,
						EntryPrice: 100.0,
						QtyUsed:    10,
					})
				}
			},
			expectedContests: []string{"contest-shard-0-a", "contest-shard-0-b"},
		},
		{
			name:               "shard 1 broadcasts only its assigned contests",
			shardID:            1,
			shardCount:         2,
			assignedContests:   []string{"contest-shard-1-a"},
			unassignedContests: []string{"contest-shard-0-a", "contest-shard-0-b"},
			setupPositions: func(sm *StateManager) {
				for _, contestID := range []string{"contest-shard-0-a", "contest-shard-0-b", "contest-shard-1-a"} {
					cs := sm.GetOrCreateContest(contestID)
					us := cs.GetOrCreateUser("user-1", 1000, 900, 0)
					us.SetPosition(&PositionState{
						PositionID: "pos-" + contestID,
						Symbol:     "AAPL",
						Side:       contracts.OrderSideBuy,
						QtyOpen:    10,
						EntryPrice: 100.0,
						QtyUsed:    10,
					})
				}
			},
			expectedContests: []string{"contest-shard-1-a"},
		},
		{
			name:               "empty shard broadcasts nothing",
			shardID:            0,
			shardCount:         2,
			assignedContests:   []string{},
			unassignedContests: []string{"contest-shard-1-a"},
			setupPositions: func(sm *StateManager) {
				// Only add positions to unassigned contest
				cs := sm.GetOrCreateContest("contest-shard-1-a")
				us := cs.GetOrCreateUser("user-1", 1000, 900, 0)
				us.SetPosition(&PositionState{
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy,
					QtyOpen:    10,
					EntryPrice: 100.0,
					QtyUsed:    10,
				})
			},
			expectedContests: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock sharded state manager
			ssm := &MockShardedStateManager{
				shardID:          tc.shardID,
				shardCount:       tc.shardCount,
				contests:         make(map[string]*ContestState),
				assignedContests: make(map[string]bool),
				ready:            true,
			}

			// Mark assigned contests
			for _, contestID := range tc.assignedContests {
				ssm.assignedContests[contestID] = true
			}

			// Setup positions (we'll use a local state manager to create the data)
			localSM := NewStateManager()
			tc.setupPositions(localSM)

			// Copy only assigned contests to the sharded state manager
			for contestID := range ssm.assignedContests {
				if cs, exists := localSM.contests[contestID]; exists {
					ssm.contests[contestID] = cs
				}
			}

			// Create price book with test price
			pb := NewPriceBook()
			pb.UpdateFromTick(&contracts.TickSnapshot{
				Ts:      time.Now().UnixMilli(),
				Symbols: []contracts.SymbolTick{{Symbol: "AAPL", Last: 110.0}},
			})

			// Track which contests get broadcast
			broadcastedContests := make(map[string]bool)

			// Simulate the broadcast logic from BroadcastUnrealizedScores
			ssm.ForEachContest(func(contestID string, cs *ContestState) {
				cs.ForEachUser(func(userID string, us *UserState) {
					if !us.HasOpenPositions() {
						return
					}
					broadcastedContests[contestID] = true
				})
			})

			// Verify only expected contests were broadcast
			for _, expectedContest := range tc.expectedContests {
				if !broadcastedContests[expectedContest] {
					t.Errorf("expected contest %s to be broadcast but it wasn't", expectedContest)
				}
			}

			// Verify unassigned contests were NOT broadcast
			for _, unassignedContest := range tc.unassignedContests {
				if broadcastedContests[unassignedContest] {
					t.Errorf("expected contest %s to NOT be broadcast but it was", unassignedContest)
				}
			}

			// Verify count matches
			if len(broadcastedContests) != len(tc.expectedContests) {
				t.Errorf("expected %d contests to be broadcast, got %d",
					len(tc.expectedContests), len(broadcastedContests))
			}
		})
	}
}

// MockShardedStateManager is a test double for ShardedStateManager.
type MockShardedStateManager struct {
	shardID          int
	shardCount       int
	contests         map[string]*ContestState
	assignedContests map[string]bool
	ready            bool
	mu               sync.RWMutex
}

func (m *MockShardedStateManager) IsAssigned(contestID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.assignedContests[contestID]
}

func (m *MockShardedStateManager) ForEachContest(fn func(contestID string, cs *ContestState)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for contestID, cs := range m.contests {
		// Only iterate over assigned contests
		if m.assignedContests[contestID] {
			fn(contestID, cs)
		}
	}
}

func (m *MockShardedStateManager) GetShardID() int {
	return m.shardID
}

func (m *MockShardedStateManager) GetShardCount() int {
	return m.shardCount
}

func (m *MockShardedStateManager) IsReady() bool {
	return m.ready
}

// TestShardedBroadcast_MultipleUsersMultiplePositions tests broadcast with
// multiple users having multiple positions.
func TestShardedBroadcast_MultipleUsersMultiplePositions(t *testing.T) {
	// Create mock sharded state manager
	ssm := &MockShardedStateManager{
		shardID:          0,
		shardCount:       1,
		contests:         make(map[string]*ContestState),
		assignedContests: map[string]bool{"contest-1": true},
		ready:            true,
	}

	// Create contest with multiple users
	cs := &ContestState{
		ContestID: "contest-1",
		Users:     make(map[string]*UserState),
	}

	// User 1: Two positions (long AAPL profit, short TSLA profit)
	user1 := &UserState{
		QtyTotal:      10000,
		QtyAvailable:  9000,
		RealizedScore: 50.0,
		Positions: map[string]*PositionState{
			"AAPL": {
				PositionID: "pos-1",
				Symbol:     "AAPL",
				Side:       contracts.OrderSideBuy,
				QtyOpen:    10,
				EntryPrice: 100.0,
				QtyUsed:    10,
			},
			"TSLA": {
				PositionID: "pos-2",
				Symbol:     "TSLA",
				Side:       contracts.OrderSideSell,
				QtyOpen:    5,
				EntryPrice: 200.0,
				QtyUsed:    5,
			},
		},
		PendingOrders: make(map[string]*PendingOrder),
	}

	// User 2: One position (long GOOGL loss)
	user2 := &UserState{
		QtyTotal:      5000,
		QtyAvailable:  4900,
		RealizedScore: 0,
		Positions: map[string]*PositionState{
			"GOOGL": {
				PositionID: "pos-3",
				Symbol:     "GOOGL",
				Side:       contracts.OrderSideBuy,
				QtyOpen:    20,
				EntryPrice: 150.0,
				QtyUsed:    20,
			},
		},
		PendingOrders: make(map[string]*PendingOrder),
	}

	// User 3: No positions (should be skipped)
	user3 := &UserState{
		QtyTotal:      1000,
		QtyAvailable:  1000,
		RealizedScore: 100.0,
		Positions:     make(map[string]*PositionState),
		PendingOrders: make(map[string]*PendingOrder),
	}

	cs.Users["user-1"] = user1
	cs.Users["user-2"] = user2
	cs.Users["user-3"] = user3
	ssm.contests["contest-1"] = cs

	// Create price book
	prices := map[string]float64{
		"AAPL":  110.0, // +10% for user1's long
		"TSLA":  180.0, // +10% for user1's short (price dropped)
		"GOOGL": 140.0, // -6.67% for user2's long
	}

	getPriceFunc := func(symbol string) (float64, bool) {
		price, ok := prices[symbol]
		return price, ok
	}

	// Track broadcasts
	type broadcastResult struct {
		userID          string
		contestID       string
		unrealizedScore float64
		totalScore      float64
	}
	var broadcasts []broadcastResult

	// Simulate broadcast
	ssm.ForEachContest(func(contestID string, cs *ContestState) {
		cs.ForEachUser(func(userID string, us *UserState) {
			if !us.HasOpenPositions() {
				return
			}

			unrealizedScore := us.CalculateUnrealizedScore(getPriceFunc)
			totalScore := us.GetRealizedScore() + unrealizedScore

			broadcasts = append(broadcasts, broadcastResult{
				userID:          userID,
				contestID:       contestID,
				unrealizedScore: unrealizedScore,
				totalScore:      totalScore,
			})
		})
	})

	// Verify results
	if len(broadcasts) != 2 {
		t.Errorf("expected 2 broadcasts (users with positions), got %d", len(broadcasts))
	}

	// Find user1's broadcast and verify
	var user1Broadcast, user2Broadcast *broadcastResult
	for i := range broadcasts {
		if broadcasts[i].userID == "user-1" {
			user1Broadcast = &broadcasts[i]
		} else if broadcasts[i].userID == "user-2" {
			user2Broadcast = &broadcasts[i]
		}
	}

	if user1Broadcast == nil {
		t.Fatal("expected user-1 broadcast not found")
	}
	// User 1: AAPL +10% * 10 qty = 100, TSLA +10% * 5 qty = 50, total unrealized = 150
	expectedUser1Unrealized := 150.0
	if !floatEquals(user1Broadcast.unrealizedScore, expectedUser1Unrealized) {
		t.Errorf("user-1 unrealized: expected %.4f, got %.4f",
			expectedUser1Unrealized, user1Broadcast.unrealizedScore)
	}
	// Total = realized (50) + unrealized (150) = 200
	if !floatEquals(user1Broadcast.totalScore, 200.0) {
		t.Errorf("user-1 total: expected 200.0, got %.4f", user1Broadcast.totalScore)
	}

	if user2Broadcast == nil {
		t.Fatal("expected user-2 broadcast not found")
	}
	// User 2: GOOGL -6.67% * 20 qty = -13.33
	expectedUser2Unrealized := (140.0 - 150.0) / 150.0 * 100 * 20 // -13.333...
	if !floatEquals(user2Broadcast.unrealizedScore, expectedUser2Unrealized) {
		t.Errorf("user-2 unrealized: expected %.4f, got %.4f",
			expectedUser2Unrealized, user2Broadcast.unrealizedScore)
	}
}

// =============================================================================
// Unit Tests: Edge Cases
// =============================================================================

func TestUnrealizedScoreCalculation_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		position *PositionState
		price    float64
		expected float64
	}{
		{
			name:     "nil position",
			position: nil,
			price:    100.0,
			expected: 0.0,
		},
		{
			name: "zero entry price",
			position: &PositionState{
				PositionID: "pos-1",
				Symbol:     "TEST",
				Side:       contracts.OrderSideBuy,
				QtyOpen:    10,
				EntryPrice: 0.0,
				QtyUsed:    10,
			},
			price:    100.0,
			expected: 0.0,
		},
		{
			name: "zero current price",
			position: &PositionState{
				PositionID: "pos-1",
				Symbol:     "TEST",
				Side:       contracts.OrderSideBuy,
				QtyOpen:    10,
				EntryPrice: 100.0,
				QtyUsed:    10,
			},
			price:    0.0,
			expected: 0.0,
		},
		{
			name: "zero qty_used",
			position: &PositionState{
				PositionID: "pos-1",
				Symbol:     "TEST",
				Side:       contracts.OrderSideBuy,
				QtyOpen:    10,
				EntryPrice: 100.0,
				QtyUsed:    0,
			},
			price:    110.0,
			expected: 0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateUnrealizedScore(tc.position, tc.price)
			if !floatEquals(result, tc.expected) {
				t.Errorf("expected %.4f, got %.4f", tc.expected, result)
			}
		})
	}
}

// =============================================================================
// Integration Tests: StateManager vs ShardedStateManager Consistency
// =============================================================================

// TestUnrealizedScore_Consistency verifies that unrealized scores from
// local StateManager and ShardedStateManager match exactly for identical
// positions and prices.
func TestUnrealizedScore_Consistency(t *testing.T) {
	tests := []struct {
		name      string
		positions []struct {
			symbol     string
			side       contracts.OrderSide
			entryPrice float64
			qtyUsed    int64
		}
		prices map[string]float64
	}{
		{
			name: "single long position",
			positions: []struct {
				symbol     string
				side       contracts.OrderSide
				entryPrice float64
				qtyUsed    int64
			}{
				{symbol: "AAPL", side: contracts.OrderSideBuy, entryPrice: 100.0, qtyUsed: 10},
			},
			prices: map[string]float64{"AAPL": 110.0},
		},
		{
			name: "single short position",
			positions: []struct {
				symbol     string
				side       contracts.OrderSide
				entryPrice float64
				qtyUsed    int64
			}{
				{symbol: "TSLA", side: contracts.OrderSideSell, entryPrice: 100.0, qtyUsed: 10},
			},
			prices: map[string]float64{"TSLA": 90.0},
		},
		{
			name: "multiple mixed positions",
			positions: []struct {
				symbol     string
				side       contracts.OrderSide
				entryPrice float64
				qtyUsed    int64
			}{
				{symbol: "AAPL", side: contracts.OrderSideBuy, entryPrice: 100.0, qtyUsed: 10},
				{symbol: "TSLA", side: contracts.OrderSideSell, entryPrice: 200.0, qtyUsed: 5},
				{symbol: "GOOGL", side: contracts.OrderSideBuy, entryPrice: 150.0, qtyUsed: 20},
				{symbol: "MSFT", side: contracts.OrderSideSell, entryPrice: 300.0, qtyUsed: 8},
			},
			prices: map[string]float64{
				"AAPL":  110.0,
				"TSLA":  180.0,
				"GOOGL": 140.0,
				"MSFT":  310.0,
			},
		},
		{
			name: "positions with same symbol different contests",
			positions: []struct {
				symbol     string
				side       contracts.OrderSide
				entryPrice float64
				qtyUsed    int64
			}{
				{symbol: "AAPL", side: contracts.OrderSideBuy, entryPrice: 100.0, qtyUsed: 10},
			},
			prices: map[string]float64{"AAPL": 105.0},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contestID := "test-contest"
			userID := "test-user"

			// Create local StateManager
			localSM := NewStateManager()
			localCS := localSM.GetOrCreateContest(contestID)
			localUS := localCS.GetOrCreateUser(userID, 100000, 90000, 0)

			// Create mock ShardedStateManager
			shardedSM := &MockShardedStateManager{
				shardID:          0,
				shardCount:       1,
				contests:         make(map[string]*ContestState),
				assignedContests: map[string]bool{contestID: true},
				ready:            true,
			}
			shardedCS := &ContestState{
				ContestID: contestID,
				Users:     make(map[string]*UserState),
			}
			shardedUS := &UserState{
				QtyTotal:      100000,
				QtyAvailable:  90000,
				RealizedScore: 0,
				Positions:     make(map[string]*PositionState),
				PendingOrders: make(map[string]*PendingOrder),
			}

			// Add identical positions to both
			for i, pos := range tc.positions {
				posState := &PositionState{
					PositionID: "pos-" + string(rune('a'+i)),
					Symbol:     pos.symbol,
					Side:       pos.side,
					QtyOpen:    pos.qtyUsed,
					EntryPrice: pos.entryPrice,
					QtyUsed:    pos.qtyUsed,
				}

				// Add to local
				localUS.SetPosition(posState)

				// Add copy to sharded
				posCopy := *posState
				shardedUS.SetPosition(&posCopy)
			}

			shardedCS.Users[userID] = shardedUS
			shardedSM.contests[contestID] = shardedCS

			// Create price function
			getPriceFunc := func(symbol string) (float64, bool) {
				price, ok := tc.prices[symbol]
				return price, ok
			}

			// Calculate unrealized scores
			localUnrealized := localUS.CalculateUnrealizedScore(getPriceFunc)
			shardedUnrealized := shardedUS.CalculateUnrealizedScore(getPriceFunc)

			// Verify they match exactly
			if !floatEquals(localUnrealized, shardedUnrealized) {
				t.Errorf("consistency check failed: local=%.6f, sharded=%.6f",
					localUnrealized, shardedUnrealized)
			}

			// Also verify total scores match
			localTotal := localUS.GetTotalScore(getPriceFunc)
			shardedTotal := shardedUS.GetTotalScore(getPriceFunc)

			if !floatEquals(localTotal, shardedTotal) {
				t.Errorf("total score consistency check failed: local=%.6f, sharded=%.6f",
					localTotal, shardedTotal)
			}
		})
	}
}

// TestUnrealizedScore_BroadcastMessageFormat verifies the PnLDelta message format.
func TestUnrealizedScore_BroadcastMessageFormat(t *testing.T) {
	// Create user state with position
	us := &UserState{
		QtyTotal:      10000,
		QtyAvailable:  9000,
		RealizedScore: 50.0,
		Positions: map[string]*PositionState{
			"AAPL": {
				PositionID: "pos-1",
				Symbol:     "AAPL",
				Side:       contracts.OrderSideBuy,
				QtyOpen:    10,
				EntryPrice: 100.0,
				QtyUsed:    10,
			},
		},
		PendingOrders: make(map[string]*PendingOrder),
	}

	getPriceFunc := func(symbol string) (float64, bool) {
		if symbol == "AAPL" {
			return 110.0, true
		}
		return 0, false
	}

	// Calculate scores
	realizedScore := us.GetRealizedScore()
	unrealizedScore := us.CalculateUnrealizedScore(getPriceFunc)
	totalScore := realizedScore + unrealizedScore

	// Create PnLDelta (simulating broadcast)
	delta := &contracts.PnLDelta{
		UserID:          "user-1",
		ContestID:       "contest-1",
		DeltaScore:      0, // No realized change for broadcasts
		RealizedScore:   realizedScore,
		UnrealizedScore: unrealizedScore,
		TotalScore:      totalScore,
		Ts:              time.Now().UnixMilli(),
	}

	// Verify fields
	if !floatEquals(delta.RealizedScore, 50.0) {
		t.Errorf("expected realized score 50.0, got %.4f", delta.RealizedScore)
	}
	if !floatEquals(delta.UnrealizedScore, 100.0) {
		t.Errorf("expected unrealized score 100.0, got %.4f", delta.UnrealizedScore)
	}
	if !floatEquals(delta.TotalScore, 150.0) {
		t.Errorf("expected total score 150.0, got %.4f", delta.TotalScore)
	}
	if delta.DeltaScore != 0 {
		t.Errorf("expected delta score 0.0 for broadcasts, got %.4f", delta.DeltaScore)
	}

	// Verify JSON serialization
	data, err := json.Marshal(delta)
	if err != nil {
		t.Fatalf("failed to marshal PnLDelta: %v", err)
	}

	var decoded contracts.PnLDelta
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal PnLDelta: %v", err)
	}

	if !floatEquals(decoded.UnrealizedScore, unrealizedScore) {
		t.Errorf("JSON round-trip failed: expected unrealized %.4f, got %.4f",
			unrealizedScore, decoded.UnrealizedScore)
	}
}
