package server

import (
	"testing"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
)

// TestValidateQtyLimits tests the QTY validation logic.
func TestValidateQtyLimits(t *testing.T) {
	tests := []struct {
		name          string
		orderQty      int64
		qtyTotal      int64
		minPerTrade   int64
		maxPctOfTotal int
		expectedError bool
		errorContains string
	}{
		{
			name:          "valid order within limits",
			orderQty:      1000,
			qtyTotal:      10000,
			minPerTrade:   100,
			maxPctOfTotal: 50,
			expectedError: false,
		},
		{
			name:          "order below minimum",
			orderQty:      50,
			qtyTotal:      10000,
			minPerTrade:   100,
			maxPctOfTotal: 50,
			expectedError: true,
			errorContains: "below minimum",
		},
		{
			name:          "order exactly at minimum",
			orderQty:      100,
			qtyTotal:      10000,
			minPerTrade:   100,
			maxPctOfTotal: 50,
			expectedError: false,
		},
		{
			name:          "order exceeds maximum percentage",
			orderQty:      6000,
			qtyTotal:      10000,
			minPerTrade:   100,
			maxPctOfTotal: 50,
			expectedError: true,
			errorContains: "exceeds maximum",
		},
		{
			name:          "order exactly at maximum percentage",
			orderQty:      5000,
			qtyTotal:      10000,
			minPerTrade:   100,
			maxPctOfTotal: 50,
			expectedError: false,
		},
		{
			name:          "zero minimum allows small orders",
			orderQty:      1,
			qtyTotal:      10000,
			minPerTrade:   0,
			maxPctOfTotal: 50,
			expectedError: false,
		},
		{
			name:          "zero max percentage disables max check",
			orderQty:      10000,
			qtyTotal:      10000,
			minPerTrade:   100,
			maxPctOfTotal: 0,
			expectedError: false,
		},
		{
			name:          "100% max allows full allocation",
			orderQty:      10000,
			qtyTotal:      10000,
			minPerTrade:   100,
			maxPctOfTotal: 100,
			expectedError: false,
		},
		{
			name:          "25% max of 100000 total",
			orderQty:      30000,
			qtyTotal:      100000,
			minPerTrade:   100,
			maxPctOfTotal: 25,
			expectedError: true,
			errorContains: "exceeds maximum 25000",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			engine := &Engine{
				config: &Config{
					QtyMinPerTrade:   tc.minPerTrade,
					QtyMaxPctOfTotal: tc.maxPctOfTotal,
				},
			}

			err := engine.validateQtyLimits(tc.orderQty, tc.qtyTotal)

			if tc.expectedError {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tc.errorContains != "" && !contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error to contain %q, got %q", tc.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

// TestQtyReservation tests the QTY reservation and release mechanics.
func TestQtyReservation(t *testing.T) {
	t.Run("reserve and release qty", func(t *testing.T) {
		user := &UserState{
			QtyTotal:     100000,
			QtyAvailable: 100000,
			Positions:    make(map[string]*PositionState),
		}

		// Reserve some QTY
		if !user.ReserveQty(25000) {
			t.Error("expected reserve to succeed")
		}
		if user.GetQtyAvailable() != 75000 {
			t.Errorf("expected 75000 available, got %d", user.GetQtyAvailable())
		}

		// Reserve more QTY
		if !user.ReserveQty(25000) {
			t.Error("expected second reserve to succeed")
		}
		if user.GetQtyAvailable() != 50000 {
			t.Errorf("expected 50000 available, got %d", user.GetQtyAvailable())
		}

		// Try to reserve more than available
		if user.ReserveQty(60000) {
			t.Error("expected reserve to fail for insufficient qty")
		}
		if user.GetQtyAvailable() != 50000 {
			t.Errorf("expected 50000 available after failed reserve, got %d", user.GetQtyAvailable())
		}

		// Release QTY
		user.ReleaseQty(25000)
		if user.GetQtyAvailable() != 75000 {
			t.Errorf("expected 75000 available after release, got %d", user.GetQtyAvailable())
		}
	})

	t.Run("reserve exact available amount", func(t *testing.T) {
		user := &UserState{
			QtyTotal:     100000,
			QtyAvailable: 50000,
			Positions:    make(map[string]*PositionState),
		}

		// Reserve exactly what's available
		if !user.ReserveQty(50000) {
			t.Error("expected reserve to succeed")
		}
		if user.GetQtyAvailable() != 0 {
			t.Errorf("expected 0 available, got %d", user.GetQtyAvailable())
		}
	})
}

// TestQtyScoring tests the Tralent QTY-based scoring formula.
func TestQtyScoring(t *testing.T) {
	tests := []struct {
		name        string
		side        string
		entryPrice  float64
		exitPrice   float64
		qtyUsed     int64
		expected    float64
		description string
	}{
		{
			name:        "long 100 qty 10% gain",
			side:        "long",
			entryPrice:  100.0,
			exitPrice:   110.0,
			qtyUsed:     100,
			expected:    1000.0, // 100 * 10% = 1000 points
			description: "100 qty * 10% gain = 1000 points",
		},
		{
			name:        "long 1000 qty 5% gain",
			side:        "long",
			entryPrice:  100.0,
			exitPrice:   105.0,
			qtyUsed:     1000,
			expected:    5000.0, // 1000 * 5% = 5000 points
			description: "1000 qty * 5% gain = 5000 points",
		},
		{
			name:        "short 500 qty 2% gain",
			side:        "short",
			entryPrice:  100.0,
			exitPrice:   98.0,
			qtyUsed:     500,
			expected:    1000.0, // 500 * 2% = 1000 points
			description: "500 qty * 2% gain = 1000 points",
		},
		{
			name:        "long 10000 qty 0.5% gain",
			side:        "long",
			entryPrice:  100.0,
			exitPrice:   100.5,
			qtyUsed:     10000,
			expected:    5000.0, // 10000 * 0.5% = 5000 points
			description: "large qty small gain",
		},
		{
			name:        "long 100 qty 20% loss",
			side:        "long",
			entryPrice:  100.0,
			exitPrice:   80.0,
			qtyUsed:     100,
			expected:    -2000.0, // 100 * -20% = -2000 points
			description: "100 qty * 20% loss = -2000 points",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateTradeScore(tc.side, tc.entryPrice, tc.exitPrice, tc.qtyUsed)
			if !floatEquals(result, tc.expected) {
				t.Errorf("%s: expected %f, got %f", tc.description, tc.expected, result)
			}
		})
	}
}

// TestUnrealizedScoreCalculation tests unrealized score calculation using Tralent formula.
func TestUnrealizedScoreCalculation(t *testing.T) {
	t.Run("single long position unrealized", func(t *testing.T) {
		user := &UserState{
			QtyTotal:     100000,
			QtyAvailable: 90000,
			Positions: map[string]*PositionState{
				"AAPL": {
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy, // BUY (long)
					QtyOpen:    100,
					EntryPrice: 150.0,
					QtyUsed:    10000,
				},
			},
		}

		getPriceFunc := func(symbol string) (float64, bool) {
			if symbol == "AAPL" {
				return 165.0, true // 10% gain
			}
			return 0, false
		}

		unrealized := user.CalculateUnrealizedScore(getPriceFunc)
		// pct_change = (165 - 150) / 150 * 100 = 10%
		// unrealized_score = 10000 * 10 = 100000
		expected := 100000.0
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})

	t.Run("multiple positions unrealized", func(t *testing.T) {
		user := &UserState{
			QtyTotal:     100000,
			QtyAvailable: 80000,
			Positions: map[string]*PositionState{
				"AAPL": {
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy, // BUY (long)
					QtyOpen:    100,
					EntryPrice: 150.0,
					QtyUsed:    10000,
				},
				"GOOG": {
					PositionID: "pos-2",
					Symbol:     "GOOG",
					Side:       contracts.OrderSideSell, // SELL (short)
					QtyOpen:    50,
					EntryPrice: 100.0,
					QtyUsed:    10000,
				},
			},
		}

		getPriceFunc := func(symbol string) (float64, bool) {
			switch symbol {
			case "AAPL":
				return 165.0, true // 10% gain for long
			case "GOOG":
				return 95.0, true // 5% gain for short (price dropped)
			}
			return 0, false
		}

		unrealized := user.CalculateUnrealizedScore(getPriceFunc)
		// AAPL: 10000 * 10% = 100000
		// GOOG: 10000 * 5% = 50000
		// Total: 150000
		expected := 150000.0
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})

	t.Run("mixed profit and loss", func(t *testing.T) {
		user := &UserState{
			QtyTotal:     100000,
			QtyAvailable: 80000,
			Positions: map[string]*PositionState{
				"AAPL": {
					PositionID: "pos-1",
					Symbol:     "AAPL",
					Side:       contracts.OrderSideBuy, // BUY (long)
					QtyOpen:    100,
					EntryPrice: 150.0,
					QtyUsed:    10000,
				},
				"MSFT": {
					PositionID: "pos-2",
					Symbol:     "MSFT",
					Side:       contracts.OrderSideBuy, // BUY (long)
					QtyOpen:    100,
					EntryPrice: 300.0,
					QtyUsed:    10000,
				},
			},
		}

		getPriceFunc := func(symbol string) (float64, bool) {
			switch symbol {
			case "AAPL":
				return 165.0, true // 10% gain
			case "MSFT":
				return 270.0, true // 10% loss
			}
			return 0, false
		}

		unrealized := user.CalculateUnrealizedScore(getPriceFunc)
		// AAPL: 10000 * 10% = 100000
		// MSFT: 10000 * -10% = -100000
		// Total: 0
		expected := 0.0
		if !floatEquals(unrealized, expected) {
			t.Errorf("expected unrealized score %f, got %f", expected, unrealized)
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
