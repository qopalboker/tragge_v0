package server

import (
	"testing"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// TestCalculateTradeScore_Long tests the Tralent scoring formula for long positions.
func TestCalculateTradeScore_Long(t *testing.T) {
	tests := []struct {
		name       string
		entryPrice float64
		exitPrice  float64
		qtyUsed    int64
		expected   float64
	}{
		{
			name:       "long position profit 10%",
			entryPrice: 100.0,
			exitPrice:  110.0,
			qtyUsed:    10,
			// pct_change = (110 - 100) / 100 * 100 = 10%
			// trade_score = 10 * 10 = 100
			expected: 100.0,
		},
		{
			name:       "long position loss 5%",
			entryPrice: 100.0,
			exitPrice:  95.0,
			qtyUsed:    20,
			// pct_change = (95 - 100) / 100 * 100 = -5%
			// trade_score = 20 * (-5) = -100
			expected: -100.0,
		},
		{
			name:       "long position breakeven",
			entryPrice: 100.0,
			exitPrice:  100.0,
			qtyUsed:    50,
			// pct_change = 0%
			// trade_score = 0
			expected: 0.0,
		},
		{
			name:       "long position profit 1%",
			entryPrice: 200.0,
			exitPrice:  202.0,
			qtyUsed:    100,
			// pct_change = (202 - 200) / 200 * 100 = 1%
			// trade_score = 100 * 1 = 100
			expected: 100.0,
		},
		{
			name:       "long position large profit 50%",
			entryPrice: 100.0,
			exitPrice:  150.0,
			qtyUsed:    5,
			// pct_change = (150 - 100) / 100 * 100 = 50%
			// trade_score = 5 * 50 = 250
			expected: 250.0,
		},
		{
			name:       "zero entry price returns zero",
			entryPrice: 0.0,
			exitPrice:  100.0,
			qtyUsed:    10,
			expected:   0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateTradeScore("long", tc.entryPrice, tc.exitPrice, tc.qtyUsed)
			if !floatEquals(result, tc.expected) {
				t.Errorf("expected %f, got %f", tc.expected, result)
			}
		})
	}
}

// TestCalculateTradeScore_Short tests the Tralent scoring formula for short positions.
func TestCalculateTradeScore_Short(t *testing.T) {
	tests := []struct {
		name       string
		entryPrice float64
		exitPrice  float64
		qtyUsed    int64
		expected   float64
	}{
		{
			name:       "short position profit 10%",
			entryPrice: 100.0,
			exitPrice:  90.0,
			qtyUsed:    10,
			// pct_change = (100 - 90) / 100 * 100 = 10%
			// trade_score = 10 * 10 = 100
			expected: 100.0,
		},
		{
			name:       "short position loss 5%",
			entryPrice: 100.0,
			exitPrice:  105.0,
			qtyUsed:    20,
			// pct_change = (100 - 105) / 100 * 100 = -5%
			// trade_score = 20 * (-5) = -100
			expected: -100.0,
		},
		{
			name:       "short position breakeven",
			entryPrice: 100.0,
			exitPrice:  100.0,
			qtyUsed:    50,
			// pct_change = 0%
			// trade_score = 0
			expected: 0.0,
		},
		{
			name:       "short position profit 20%",
			entryPrice: 150.0,
			exitPrice:  120.0,
			qtyUsed:    10,
			// pct_change = (150 - 120) / 150 * 100 = 20%
			// trade_score = 10 * 20 = 200
			expected: 200.0,
		},
		{
			name:       "short position large loss 100% (price doubled)",
			entryPrice: 100.0,
			exitPrice:  200.0,
			qtyUsed:    5,
			// pct_change = (100 - 200) / 100 * 100 = -100%
			// trade_score = 5 * (-100) = -500
			expected: -500.0,
		},
		{
			name:       "zero entry price returns zero",
			entryPrice: 0.0,
			exitPrice:  100.0,
			qtyUsed:    10,
			expected:   0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateTradeScore("short", tc.entryPrice, tc.exitPrice, tc.qtyUsed)
			if !floatEquals(result, tc.expected) {
				t.Errorf("expected %f, got %f", tc.expected, result)
			}
		})
	}
}

// TestUserState_QtyAccounting tests QTY reservation and release.
func TestUserState_QtyAccounting(t *testing.T) {
	t.Run("reserve qty success", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      1000,
			QtyAvailable:  1000,
			RealizedScore: 0,
			Positions:     make(map[string]*PositionState),
			PendingOrders: make(map[string]*PendingOrder),
		}

		// Reserve 100
		ok := us.ReserveQty(100)
		if !ok {
			t.Fatal("expected reservation to succeed")
		}
		if us.GetQtyAvailable() != 900 {
			t.Errorf("expected qty_available 900, got %d", us.GetQtyAvailable())
		}

		// Reserve another 500
		ok = us.ReserveQty(500)
		if !ok {
			t.Fatal("expected reservation to succeed")
		}
		if us.GetQtyAvailable() != 400 {
			t.Errorf("expected qty_available 400, got %d", us.GetQtyAvailable())
		}
	})

	t.Run("reserve qty failure - insufficient", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      100,
			QtyAvailable:  100,
			RealizedScore: 0,
			Positions:     make(map[string]*PositionState),
			PendingOrders: make(map[string]*PendingOrder),
		}

		// Try to reserve more than available
		ok := us.ReserveQty(150)
		if ok {
			t.Fatal("expected reservation to fail")
		}
		if us.GetQtyAvailable() != 100 {
			t.Errorf("expected qty_available to remain 100, got %d", us.GetQtyAvailable())
		}
	})

	t.Run("release qty restores availability", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      1000,
			QtyAvailable:  1000,
			RealizedScore: 0,
			Positions:     make(map[string]*PositionState),
			PendingOrders: make(map[string]*PendingOrder),
		}

		// Reserve 300
		us.ReserveQty(300)
		if us.GetQtyAvailable() != 700 {
			t.Errorf("expected qty_available 700, got %d", us.GetQtyAvailable())
		}

		// Release 300 (simulating position close)
		us.ReleaseQty(300)
		if us.GetQtyAvailable() != 1000 {
			t.Errorf("expected qty_available 1000, got %d", us.GetQtyAvailable())
		}
	})

	t.Run("qty reuse after position close", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      500,
			QtyAvailable:  500,
			RealizedScore: 0,
			Positions:     make(map[string]*PositionState),
			PendingOrders: make(map[string]*PendingOrder),
		}

		// Open first position with 200 qty
		ok := us.ReserveQty(200)
		if !ok {
			t.Fatal("expected first reservation to succeed")
		}
		if us.GetQtyAvailable() != 300 {
			t.Errorf("expected qty_available 300, got %d", us.GetQtyAvailable())
		}

		// Open second position with 250 qty
		ok = us.ReserveQty(250)
		if !ok {
			t.Fatal("expected second reservation to succeed")
		}
		if us.GetQtyAvailable() != 50 {
			t.Errorf("expected qty_available 50, got %d", us.GetQtyAvailable())
		}

		// Close first position - qty should be reusable
		us.ReleaseQty(200)
		if us.GetQtyAvailable() != 250 {
			t.Errorf("expected qty_available 250 after first close, got %d", us.GetQtyAvailable())
		}

		// Open third position with released qty
		ok = us.ReserveQty(200)
		if !ok {
			t.Fatal("expected third reservation with reused qty to succeed")
		}
		if us.GetQtyAvailable() != 50 {
			t.Errorf("expected qty_available 50, got %d", us.GetQtyAvailable())
		}

		// Close second position
		us.ReleaseQty(250)
		if us.GetQtyAvailable() != 300 {
			t.Errorf("expected qty_available 300, got %d", us.GetQtyAvailable())
		}

		// Close third position
		us.ReleaseQty(200)
		if us.GetQtyAvailable() != 500 {
			t.Errorf("expected qty_available back to 500, got %d", us.GetQtyAvailable())
		}
	})
}

// TestUserState_UnrealizedScore tests the calculation of unrealized score.
func TestUserState_UnrealizedScore(t *testing.T) {
	t.Run("long position unrealized profit", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      1000,
			QtyAvailable:  900,
			RealizedScore: 0,
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

		// Price is now 110 (10% profit)
		getPriceFunc := func(symbol string) (float64, bool) {
			if symbol == "AAPL" {
				return 110.0, true
			}
			return 0, false
		}

		unrealized := us.CalculateUnrealizedScore(getPriceFunc)
		// pct_change = (110 - 100) / 100 * 100 = 10%
		// unrealized_score = 10 * 10 = 100
		expected := 100.0
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})

	t.Run("short position unrealized profit", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      1000,
			QtyAvailable:  800,
			RealizedScore: 0,
			Positions: map[string]*PositionState{
				"TSLA": {
					PositionID: "pos-2",
					Symbol:     "TSLA",
					Side:       contracts.OrderSideSell,
					QtyOpen:    20,
					EntryPrice: 200.0,
					QtyUsed:    20,
				},
			},
			PendingOrders: make(map[string]*PendingOrder),
		}

		// Price is now 180 (10% profit for short)
		getPriceFunc := func(symbol string) (float64, bool) {
			if symbol == "TSLA" {
				return 180.0, true
			}
			return 0, false
		}

		unrealized := us.CalculateUnrealizedScore(getPriceFunc)
		// pct_change = (200 - 180) / 200 * 100 = 10%
		// unrealized_score = 20 * 10 = 200
		expected := 200.0
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})

	t.Run("multiple positions", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      1000,
			QtyAvailable:  700,
			RealizedScore: 50.0, // Some realized score from previous trades
			Positions: map[string]*PositionState{
				"AAPL": {
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy,
					QtyOpen:    10,
					EntryPrice: 100.0,
					QtyUsed:    10,
				},
				"GOOGL": {
					PositionID: "pos-2",
					Symbol:     "GOOGL",
					Side:       contracts.OrderSideSell,
					QtyOpen:    5,
					EntryPrice: 150.0,
					QtyUsed:    5,
				},
			},
			PendingOrders: make(map[string]*PendingOrder),
		}

		getPriceFunc := func(symbol string) (float64, bool) {
			switch symbol {
			case "AAPL":
				return 105.0, true // 5% profit for long
			case "GOOGL":
				return 160.0, true // ~6.67% loss for short
			}
			return 0, false
		}

		unrealized := us.CalculateUnrealizedScore(getPriceFunc)
		// AAPL: pct_change = 5%, score = 10 * 5 = 50
		// GOOGL: pct_change = (150-160)/150*100 = -6.67%, score = 5 * (-6.67) = -33.33
		// Total unrealized = 50 - 33.33 = 16.67
		expected := 50.0 - (10.0 / 150.0 * 100.0 * 5.0) // = 50 - 33.333... = 16.666...
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}

		// Total score should be realized + unrealized
		totalScore := us.GetTotalScore(getPriceFunc)
		expectedTotal := 50.0 + expected
		if !floatEquals(totalScore, expectedTotal) {
			t.Errorf("expected total score %f, got %f", expectedTotal, totalScore)
		}
	})
}

// TestUserState_RealizedScore tests realized score accumulation.
func TestUserState_RealizedScore(t *testing.T) {
	t.Run("add realized score", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      1000,
			QtyAvailable:  1000,
			RealizedScore: 0,
			Positions:     make(map[string]*PositionState),
			PendingOrders: make(map[string]*PendingOrder),
		}

		// First trade: +100 score
		newScore := us.AddRealizedScore(100.0)
		if !floatEquals(newScore, 100.0) {
			t.Errorf("expected score 100, got %f", newScore)
		}
		if !floatEquals(us.GetRealizedScore(), 100.0) {
			t.Errorf("expected realized score 100, got %f", us.GetRealizedScore())
		}

		// Second trade: -50 score
		newScore = us.AddRealizedScore(-50.0)
		if !floatEquals(newScore, 50.0) {
			t.Errorf("expected score 50, got %f", newScore)
		}

		// Third trade: +200 score
		newScore = us.AddRealizedScore(200.0)
		if !floatEquals(newScore, 250.0) {
			t.Errorf("expected score 250, got %f", newScore)
		}
	})
}

// TestQtyExceedsAvailable tests that orders are rejected when qty exceeds available.
func TestQtyExceedsAvailable(t *testing.T) {
	t.Run("cannot exceed qty_available", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      100,
			QtyAvailable:  50, // Only 50 available
			RealizedScore: 0,
			Positions:     make(map[string]*PositionState),
			PendingOrders: make(map[string]*PendingOrder),
		}

		// Try to reserve 60 (more than available)
		ok := us.ReserveQty(60)
		if ok {
			t.Fatal("expected reservation to fail when qty > available")
		}

		// Available should remain unchanged
		if us.GetQtyAvailable() != 50 {
			t.Errorf("expected qty_available to remain 50, got %d", us.GetQtyAvailable())
		}

		// Reserve exactly available amount
		ok = us.ReserveQty(50)
		if !ok {
			t.Fatal("expected reservation of exact amount to succeed")
		}
		if us.GetQtyAvailable() != 0 {
			t.Errorf("expected qty_available 0, got %d", us.GetQtyAvailable())
		}

		// Now any reservation should fail
		ok = us.ReserveQty(1)
		if ok {
			t.Fatal("expected reservation to fail when no qty available")
		}
	})

	t.Run("partial close releases proportional qty", func(t *testing.T) {
		us := &UserState{
			QtyTotal:      1000,
			QtyAvailable:  800, // 200 reserved for position
			RealizedScore: 0,
			Positions: map[string]*PositionState{
				"AAPL": {
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy,
					QtyOpen:    100,
					EntryPrice: 100.0,
					QtyUsed:    200, // More qty_used than qty_open is possible
				},
			},
			PendingOrders: make(map[string]*PendingOrder),
		}

		// If we close 50% of the position (50 qty out of 100),
		// we should release 50% of qty_used (100 out of 200)
		closingQty := int64(50)
		qtyOpen := int64(100)
		qtyUsed := int64(200)
		qtyUsedToRelease := (qtyUsed * closingQty) / qtyOpen // = 100

		us.ReleaseQty(qtyUsedToRelease)
		if us.GetQtyAvailable() != 900 {
			t.Errorf("expected qty_available 900 after partial close, got %d", us.GetQtyAvailable())
		}
	})
}

// TestTralentFormulaVerification tests the exact test cases from the Tralent specification.
// These are the verification test cases to ensure formula correctness.
func TestTralentFormulaVerification(t *testing.T) {
	tests := []struct {
		name       string
		side       string
		entryPrice float64
		exitPrice  float64
		qtyUsed    int64
		expected   float64
	}{
		// User-specified verification cases
		{
			name:       "long profit: entry=100, exit=105, qty=5 → +25.00",
			side:       "long",
			entryPrice: 100.0,
			exitPrice:  105.0,
			qtyUsed:    5,
			// pct_change = (105 - 100) / 100 * 100 = 5%
			// score = 5 * 5 = 25
			expected: 25.0,
		},
		{
			name:       "long loss: entry=100, exit=95, qty=5 → -25.00",
			side:       "long",
			entryPrice: 100.0,
			exitPrice:  95.0,
			qtyUsed:    5,
			// pct_change = (95 - 100) / 100 * 100 = -5%
			// score = 5 * (-5) = -25
			expected: -25.0,
		},
		{
			name:       "short profit: entry=100, exit=95, qty=3 → +15.00",
			side:       "short",
			entryPrice: 100.0,
			exitPrice:  95.0,
			qtyUsed:    3,
			// pct_change = (100 - 95) / 100 * 100 = 5%
			// score = 3 * 5 = 15
			expected: 15.0,
		},
		{
			name:       "short loss: entry=100, exit=105, qty=3 → -15.00",
			side:       "short",
			entryPrice: 100.0,
			exitPrice:  105.0,
			qtyUsed:    3,
			// pct_change = (100 - 105) / 100 * 100 = -5%
			// score = 3 * (-5) = -15
			expected: -15.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateTradeScore(tc.side, tc.entryPrice, tc.exitPrice, tc.qtyUsed)
			if !floatEquals(result, tc.expected) {
				t.Errorf("expected %f, got %f", tc.expected, result)
			}
		})
	}
}

// TestTralentExampleCalculation verifies the example from the Tralent specification.
// This tests the total tournament score accumulation.
// Example:
// | Trade | Direction | Entry | Exit | Raw P&L% | QTY | Weighted |
// |-------|-----------|-------|------|----------|-----|----------|
// | 1     | Long      | $100  | $101 | +1.00%   | 5   | +5.00    |
// | 2     | Short     | $100  | $98  | +2.00%   | 3   | +6.00    |
// | 3     | Long      | $100  | $99  | -1.00%   | 2   | -2.00    |
// Total Score: +9.00
func TestTralentExampleCalculation(t *testing.T) {
	trades := []struct {
		direction  string
		entryPrice float64
		exitPrice  float64
		qty        int64
		expected   float64
	}{
		{"long", 100.0, 101.0, 5, 5.0}, // (101-100)/100*100 = 1%, 5*1 = 5
		{"short", 100.0, 98.0, 3, 6.0}, // (100-98)/100*100 = 2%, 3*2 = 6
		{"long", 100.0, 99.0, 2, -2.0}, // (99-100)/100*100 = -1%, 2*(-1) = -2
	}

	var totalScore float64
	for i, trade := range trades {
		score := calculateTradeScore(trade.direction, trade.entryPrice, trade.exitPrice, trade.qty)
		if !floatEquals(score, trade.expected) {
			t.Errorf("trade %d: expected score %f, got %f", i+1, trade.expected, score)
		}
		totalScore += score
	}

	expectedTotal := 9.0 // 5 + 6 + (-2) = 9
	if !floatEquals(totalScore, expectedTotal) {
		t.Errorf("total score: expected %f, got %f", expectedTotal, totalScore)
	}
}

// TestUnrealizedScoreFormula verifies the unrealized score calculation matches Tralent formula.
// Same formula as realized, but uses current market price instead of exit price.
func TestUnrealizedScoreFormula(t *testing.T) {
	t.Run("long unrealized: entry=100, mark=105, qty=5 → +25.00", func(t *testing.T) {
		us := &UserState{
			QtyTotal:     1000,
			QtyAvailable: 995,
			Positions: map[string]*PositionState{
				"TEST": {
					PositionID: "pos-1",
					Symbol:     "TEST",
					Side:       contracts.OrderSideBuy,
					QtyOpen:    5,
					EntryPrice: 100.0,
					QtyUsed:    5,
				},
			},
			PendingOrders: make(map[string]*PendingOrder),
		}

		getPriceFunc := func(symbol string) (float64, bool) {
			if symbol == "TEST" {
				return 105.0, true
			}
			return 0, false
		}

		unrealized := us.CalculateUnrealizedScore(getPriceFunc)
		expected := 25.0 // (105-100)/100*100 = 5%, 5*5 = 25
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})

	t.Run("short unrealized: entry=100, mark=95, qty=3 → +15.00", func(t *testing.T) {
		us := &UserState{
			QtyTotal:     1000,
			QtyAvailable: 997,
			Positions: map[string]*PositionState{
				"TEST": {
					PositionID: "pos-1",
					Symbol:     "TEST",
					Side:       contracts.OrderSideSell,
					QtyOpen:    3,
					EntryPrice: 100.0,
					QtyUsed:    3,
				},
			},
			PendingOrders: make(map[string]*PendingOrder),
		}

		getPriceFunc := func(symbol string) (float64, bool) {
			if symbol == "TEST" {
				return 95.0, true
			}
			return 0, false
		}

		unrealized := us.CalculateUnrealizedScore(getPriceFunc)
		expected := 15.0 // (100-95)/100*100 = 5%, 3*5 = 15
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})

	t.Run("long unrealized loss: entry=100, mark=95, qty=5 → -25.00", func(t *testing.T) {
		us := &UserState{
			QtyTotal:     1000,
			QtyAvailable: 995,
			Positions: map[string]*PositionState{
				"TEST": {
					PositionID: "pos-1",
					Symbol:     "TEST",
					Side:       contracts.OrderSideBuy,
					QtyOpen:    5,
					EntryPrice: 100.0,
					QtyUsed:    5,
				},
			},
			PendingOrders: make(map[string]*PendingOrder),
		}

		getPriceFunc := func(symbol string) (float64, bool) {
			if symbol == "TEST" {
				return 95.0, true
			}
			return 0, false
		}

		unrealized := us.CalculateUnrealizedScore(getPriceFunc)
		expected := -25.0 // (95-100)/100*100 = -5%, 5*(-5) = -25
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})

	t.Run("short unrealized loss: entry=100, mark=105, qty=3 → -15.00", func(t *testing.T) {
		us := &UserState{
			QtyTotal:     1000,
			QtyAvailable: 997,
			Positions: map[string]*PositionState{
				"TEST": {
					PositionID: "pos-1",
					Symbol:     "TEST",
					Side:       contracts.OrderSideSell,
					QtyOpen:    3,
					EntryPrice: 100.0,
					QtyUsed:    3,
				},
			},
			PendingOrders: make(map[string]*PendingOrder),
		}

		getPriceFunc := func(symbol string) (float64, bool) {
			if symbol == "TEST" {
				return 105.0, true
			}
			return 0, false
		}

		unrealized := us.CalculateUnrealizedScore(getPriceFunc)
		expected := -15.0 // (100-105)/100*100 = -5%, 3*(-5) = -15
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})
}

// floatEquals compares two floats with a small tolerance.
func floatEquals(a, b float64) bool {
	const epsilon = 0.0001
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}
