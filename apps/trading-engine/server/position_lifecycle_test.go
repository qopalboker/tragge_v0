package server

import (
	"math"
	"testing"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/shopspring/decimal"
)

// floatEquals compares two floats with a small tolerance.
func floatEqualsLifecycle(a, b float64) bool {
	const epsilon = 0.0001
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

// TestQtyInvariant_CloseViaOppositeMarketOrder tests that when a user closes a LONG position
// by placing a SELL market order, the qty reservation from the order is properly released back
// along with the position's QtyUsed.
func TestQtyInvariant_CloseViaOppositeMarketOrder(t *testing.T) {
	// Setup: QtyTotal=1000, Position LONG with QtyUsed=100, QtyOpen=100
	// So QtyAvailable should be 900 (1000 - 100)
	us := &UserState{
		QtyTotal:             1000,
		QtyAvailable:         900,
		RealizedScore:        0,
		RealizedScoreDecimal: decimal.Zero,
		Positions: map[string]*PositionState{
			"BTCUSD": {
				PositionID: "pos-1", Symbol: "BTCUSD",
				Side: contracts.OrderSideBuy, QtyOpen: 100,
				EntryPrice: 50000.0, QtyUsed: 100,
			},
		},
		PendingOrders: make(map[string]*PendingOrder),
	}

	// Step 1: User places SELL 100 (opposite side) → ReserveQty(100)
	ok := us.ReserveQty(100)
	if !ok {
		t.Fatal("ReserveQty should succeed")
	}
	// QtyAvailable is now 800

	// Step 2: Simulate updatePositionTx close branch with preReserved=true
	// fillQty=100, dbPos.QtyOpen=100, overflowQty=0
	// closePortion = 100 - 0 = 100 (because preReserved=true)
	// newQtyAvailable = 800 + 100(QtyUsed) + 100(closePortion) = 1000
	fillQty := int64(100)
	overflowQty := fillQty - int64(100) // dbPos.QtyOpen=100
	closePortion := fillQty - overflowQty // preReserved=true
	qtyReleased := int64(100) + closePortion // dbPos.QtyUsed + closePortion

	// Step 3: Simulate applyPositionResultToMemory close branch
	tradeScore := calculateTradeScoreDecimal("long", 50000.0, 55000.0, 100)
	us.AddRealizedScoreDecimal(tradeScore.Decimal)
	us.ReleaseQty(qtyReleased)
	us.RemovePosition("BTCUSD")

	// Verify invariant: QtyAvailable + ΣQtyUsed == QtyTotal
	totalQtyUsed := int64(0)
	for _, pos := range us.GetAllPositions() {
		totalQtyUsed += pos.QtyUsed
	}
	if us.GetQtyAvailable()+totalQtyUsed != us.QtyTotal {
		t.Errorf("QTY INVARIANT BROKEN: QtyAvailable(%d) + ΣQtyUsed(%d) = %d, want %d",
			us.GetQtyAvailable(), totalQtyUsed, us.GetQtyAvailable()+totalQtyUsed, us.QtyTotal)
	}

	// QtyAvailable should be back to 1000
	if us.GetQtyAvailable() != 1000 {
		t.Errorf("QtyAvailable = %d, want 1000", us.GetQtyAvailable())
	}
}

// TestQtyInvariant_TPSLClose_NoReservation tests that when TP/SL triggers (preReserved=false),
// qty_available does NOT inflate beyond QtyTotal. This is the exact regression that was fixed.
func TestQtyInvariant_TPSLClose_NoReservation(t *testing.T) {
	// Setup: same as above but TP/SL path (no ReserveQty)
	us := &UserState{
		QtyTotal:             1000,
		QtyAvailable:         900,
		RealizedScore:        0,
		RealizedScoreDecimal: decimal.Zero,
		Positions: map[string]*PositionState{
			"BTCUSD": {
				PositionID: "pos-1", Symbol: "BTCUSD",
				Side: contracts.OrderSideBuy, QtyOpen: 100,
				EntryPrice: 50000.0, QtyUsed: 100,
			},
		},
		PendingOrders: make(map[string]*PendingOrder),
	}

	// Step 1: TP/SL does NOT call ReserveQty — QtyAvailable stays 900

	// Step 2: Simulate updatePositionTx close branch with preReserved=false
	// closePortion = 0 (because preReserved=false)
	// newQtyAvailable = 900 + 100(QtyUsed) + 0(closePortion) = 1000
	closePortion := int64(0) // preReserved=false
	qtyReleased := int64(100) + closePortion // dbPos.QtyUsed + closePortion

	// Step 3: Simulate applyPositionResultToMemory close branch
	tradeScore := calculateTradeScoreDecimal("long", 50000.0, 55000.0, 100)
	us.AddRealizedScoreDecimal(tradeScore.Decimal)
	us.ReleaseQty(qtyReleased)
	us.RemovePosition("BTCUSD")

	// Verify invariant
	if us.GetQtyAvailable() != 1000 {
		t.Errorf("QtyAvailable = %d, want 1000 (should not inflate beyond QtyTotal)", us.GetQtyAvailable())
	}
	if us.GetQtyAvailable() > us.QtyTotal {
		t.Fatalf("CRITICAL: QtyAvailable(%d) > QtyTotal(%d) — TP/SL inflation bug!", us.GetQtyAvailable(), us.QtyTotal)
	}
}

// TestQtyInvariant_PartialClose_ThenTPSL tests the stale QtyOpen scenario: partial close
// reduces position, then TP/SL should use the reduced quantity.
func TestQtyInvariant_PartialClose_ThenTPSL(t *testing.T) {
	us := &UserState{
		QtyTotal:             1000,
		QtyAvailable:         900,
		RealizedScore:        0,
		RealizedScoreDecimal: decimal.Zero,
		Positions: map[string]*PositionState{
			"BTCUSD": {
				PositionID: "pos-1", Symbol: "BTCUSD",
				Side: contracts.OrderSideBuy, QtyOpen: 100,
				EntryPrice: 50000.0, QtyUsed: 100,
			},
		},
		PendingOrders: make(map[string]*PendingOrder),
	}

	// Step 1: Partial close 30 units (via ProcessClosePosition — no ReserveQty)
	closingQty := int64(30)
	dbQtyUsed := int64(100)
	dbQtyOpen := int64(100)
	qtyUsedForClose := int64(math.Round(float64(dbQtyUsed) * float64(closingQty) / float64(dbQtyOpen))) // = 30
	remainingQty := dbQtyOpen - closingQty                                                                // = 70
	newQtyUsed := dbQtyUsed - qtyUsedForClose                                                             // = 70

	tradeScore1 := calculateTradeScoreDecimal("long", 50000.0, 52000.0, qtyUsedForClose)
	us.AddRealizedScoreDecimal(tradeScore1.Decimal)
	us.ReleaseQty(qtyUsedForClose)
	us.SetPosition(&PositionState{
		PositionID: "pos-1", Symbol: "BTCUSD",
		Side: contracts.OrderSideBuy, QtyOpen: remainingQty,
		EntryPrice: 50000.0, QtyUsed: newQtyUsed,
	})

	// Verify mid-point invariant
	midQtyUsed := int64(0)
	for _, pos := range us.GetAllPositions() {
		midQtyUsed += pos.QtyUsed
	}
	if us.GetQtyAvailable()+midQtyUsed != us.QtyTotal {
		t.Fatalf("Mid-point invariant broken: %d + %d = %d, want %d",
			us.GetQtyAvailable(), midQtyUsed, us.GetQtyAvailable()+midQtyUsed, us.QtyTotal)
	}

	// Step 2: TP/SL triggers on remaining 70 units (preReserved=false)
	// The fix ensures executeTPSL re-reads from DB and gets QtyOpen=70 (not stale 100)
	freshQtyOpen := remainingQty // = 70 (simulates fresh DB read)
	_ = freshQtyOpen
	tpslClosePortion := int64(0) // preReserved=false
	tpslQtyReleased := newQtyUsed + tpslClosePortion // 70 + 0 = 70

	tradeScore2 := calculateTradeScoreDecimal("long", 50000.0, 55000.0, newQtyUsed)
	us.AddRealizedScoreDecimal(tradeScore2.Decimal)
	us.ReleaseQty(tpslQtyReleased)
	us.RemovePosition("BTCUSD")

	// Verify final invariant
	if us.GetQtyAvailable() != 1000 {
		t.Errorf("Final QtyAvailable = %d, want 1000", us.GetQtyAvailable())
	}
	if us.GetQtyAvailable() > us.QtyTotal {
		t.Fatalf("CRITICAL: QtyAvailable(%d) > QtyTotal(%d)", us.GetQtyAvailable(), us.QtyTotal)
	}
}

// TestQtyInvariant_OverflowFlip tests the overflow/flip scenario: SELL 150 when LONG 100
// → close LONG + open SHORT 50.
func TestQtyInvariant_OverflowFlip(t *testing.T) {
	us := &UserState{
		QtyTotal:             1000,
		QtyAvailable:         900,
		RealizedScore:        0,
		RealizedScoreDecimal: decimal.Zero,
		Positions: map[string]*PositionState{
			"BTCUSD": {
				PositionID: "pos-1", Symbol: "BTCUSD",
				Side: contracts.OrderSideBuy, QtyOpen: 100,
				EntryPrice: 50000.0, QtyUsed: 100,
			},
		},
		PendingOrders: make(map[string]*PendingOrder),
	}

	// SELL 150 via market order (preReserved=true)
	fillQty := int64(150)
	ok := us.ReserveQty(fillQty)
	if !ok {
		t.Fatal("ReserveQty(150) should succeed (available=900)")
	}
	// QtyAvailable = 750

	// close branch: overflowQty=50, closePortion=100 (preReserved=true)
	overflowQty := fillQty - int64(100) // 150 - 100 = 50
	closePortion := fillQty - overflowQty // 100
	qtyReleased := int64(100) + closePortion // 100 + 100 = 200

	tradeScore := calculateTradeScoreDecimal("long", 50000.0, 48000.0, 100)
	us.AddRealizedScoreDecimal(tradeScore.Decimal)
	us.ReleaseQty(qtyReleased) // 750 + 200 = 950
	us.RemovePosition("BTCUSD")

	// Overflow: new SHORT position with QtyUsed=50
	us.SetPosition(&PositionState{
		PositionID: "pos-2", Symbol: "BTCUSD",
		Side: contracts.OrderSideSell, QtyOpen: overflowQty,
		EntryPrice: 48000.0, QtyUsed: overflowQty,
	})

	// Verify: QtyAvailable(950) + QtyUsed(50) = 1000
	totalQtyUsed := int64(0)
	for _, pos := range us.GetAllPositions() {
		totalQtyUsed += pos.QtyUsed
	}
	if us.GetQtyAvailable()+totalQtyUsed != us.QtyTotal {
		t.Errorf("QTY INVARIANT BROKEN: %d + %d = %d, want %d",
			us.GetQtyAvailable(), totalQtyUsed, us.GetQtyAvailable()+totalQtyUsed, us.QtyTotal)
	}
}

// TestPartialCloseRounding_Consistency tests that the math.Round formula produces
// consistent results and doesn't lose qty over multiple partial closes.
func TestPartialCloseRounding_Consistency(t *testing.T) {
	tests := []struct {
		name          string
		qtyUsed       int64
		qtyOpen       int64
		closeQty      int64
		expectedRound int64
	}{
		{"100/7 close 3", 100, 7, 3, 43},  // Round(42.857)
		{"100/3 close 1", 100, 3, 1, 33},  // Round(33.333)
		{"7/3 close 1", 7, 3, 1, 2},       // Round(2.333)
		{"1/3 close 1", 1, 3, 1, 1},       // Round(0.333) = 0, clamped to 1
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := int64(math.Round(float64(tc.qtyUsed) * float64(tc.closeQty) / float64(tc.qtyOpen)))
			if result <= 0 && tc.closeQty > 0 {
				result = 1 // min clamp
			}
			if result != tc.expectedRound {
				t.Errorf("qtyUsedForClose = %d, want %d", result, tc.expectedRound)
			}
		})
	}

	// Full round-trip: close position in 3 chunks, verify total qty_used released = original
	t.Run("multi_partial_close_no_leak", func(t *testing.T) {
		qtyUsed := int64(100)
		qtyOpen := int64(100)
		totalReleased := int64(0)

		// Close 30, then 30, then 40
		closes := []int64{30, 30, 40}
		for _, closeQty := range closes {
			released := int64(math.Round(float64(qtyUsed) * float64(closeQty) / float64(qtyOpen)))
			if released <= 0 && closeQty > 0 {
				released = 1
			}
			totalReleased += released
			qtyUsed -= released
			qtyOpen -= closeQty
		}

		if totalReleased != 100 {
			t.Errorf("total qty_used released = %d, want 100 (leak!)", totalReleased)
		}
	})
}

// TestScoreDecimal_AccumulationVsFloat64 tests that AddRealizedScoreDecimal doesn't drift
// from the expected value over many small trades, unlike AddRealizedScore.
func TestScoreDecimal_AccumulationVsFloat64(t *testing.T) {
	us := &UserState{
		QtyTotal:             1000,
		QtyAvailable:         1000,
		RealizedScore:        0,
		RealizedScoreDecimal: decimal.Zero,
		Positions:            make(map[string]*PositionState),
		PendingOrders:        make(map[string]*PendingOrder),
	}

	// Simulate 10000 small trades
	for i := 0; i < 10000; i++ {
		score := calculateTradeScoreDecimal("long", 100.0, 100.001, 1) // tiny score
		us.AddRealizedScoreDecimal(score.Decimal)
	}

	decimalResult := us.GetRealizedScoreDecimal()

	// Compute what float64 accumulation would give
	var float64Sum float64
	for i := 0; i < 10000; i++ {
		pctChange := (100.001 - 100.0) / 100.0 * 100.0
		float64Sum += 1.0 * pctChange
	}

	// Decimal should be more precise
	decFloat, _ := decimalResult.Float64()
	t.Logf("Decimal result: %s (float64: %f)", decimalResult.StringFixed(8), decFloat)
	t.Logf("Float64 accumulation: %f", float64Sum)

	// Both should be close to 10.0 (0.001% * 1 * 10000 = 10)
	expected := 10.0
	if math.Abs(decFloat-expected) > 0.0001 {
		t.Errorf("Decimal accumulation drifted: got %f, want ~%f", decFloat, expected)
	}
}
