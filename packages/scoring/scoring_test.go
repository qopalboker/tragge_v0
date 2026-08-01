package scoring

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
)

// floatEquals compares two float64 values with a small tolerance.
func floatEquals(a, b float64) bool {
	const epsilon = 1e-10
	return math.Abs(a-b) < epsilon
}

// TestCalculatePctChange_Long tests percentage change for long positions.
func TestCalculatePctChange_Long(t *testing.T) {
	tests := []struct {
		name       string
		entryPrice float64
		exitPrice  float64
		expected   float64
	}{
		{"10% gain", 100.0, 110.0, 10.0},
		{"10% loss", 100.0, 90.0, -10.0},
		{"0% change", 100.0, 100.0, 0.0},
		{"50% gain", 100.0, 150.0, 50.0},
		{"50% loss", 100.0, 50.0, -50.0},
		{"very small gain 0.001%", 100.0, 100.001, 0.001},
		{"very small loss 0.001%", 100.0, 99.999, -0.001},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := decimal.NewFromFloat(tc.entryPrice)
			exit := decimal.NewFromFloat(tc.exitPrice)
			result := CalculatePctChange(entry, exit, SideLong)
			resultFloat := ToFloat64(result)

			if !floatEquals(resultFloat, tc.expected) {
				t.Errorf("expected %f, got %f", tc.expected, resultFloat)
			}
		})
	}
}

// TestCalculatePctChange_Short tests percentage change for short positions.
func TestCalculatePctChange_Short(t *testing.T) {
	tests := []struct {
		name       string
		entryPrice float64
		exitPrice  float64
		expected   float64
	}{
		{"10% gain (price down)", 100.0, 90.0, 10.0},
		{"10% loss (price up)", 100.0, 110.0, -10.0},
		{"0% change", 100.0, 100.0, 0.0},
		{"50% gain (price down)", 100.0, 50.0, 50.0},
		{"50% loss (price up)", 100.0, 150.0, -50.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := decimal.NewFromFloat(tc.entryPrice)
			exit := decimal.NewFromFloat(tc.exitPrice)
			result := CalculatePctChange(entry, exit, SideShort)
			resultFloat := ToFloat64(result)

			if !floatEquals(resultFloat, tc.expected) {
				t.Errorf("expected %f, got %f", tc.expected, resultFloat)
			}
		})
	}
}

// TestCalculatePctChange_ZeroEntry tests handling of zero entry price.
func TestCalculatePctChange_ZeroEntry(t *testing.T) {
	entry := decimal.Zero
	exit := decimal.NewFromFloat(100.0)
	result := CalculatePctChange(entry, exit, SideLong)

	if !result.IsZero() {
		t.Errorf("expected 0, got %s", result.String())
	}
}

// TestCalculateTradeScore tests the trade score calculation.
func TestCalculateTradeScore(t *testing.T) {
	tests := []struct {
		name      string
		qtyUsed   int64
		pctChange float64
		expected  float64
	}{
		{"1 qty, 10% gain", 1, 10.0, 10.0},
		{"10 qty, 10% gain", 10, 10.0, 100.0},
		{"100 qty, -5% loss", 100, -5.0, -500.0},
		{"1000000 qty, 0.001% gain", 1000000, 0.001, 1000.0},
		{"1000000 qty, 1% gain", 1000000, 1.0, 1000000.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qty := decimal.NewFromInt(tc.qtyUsed)
			pct := decimal.NewFromFloat(tc.pctChange)
			result := CalculateTradeScore(qty, pct)
			resultFloat := ToFloat64(result)

			if !floatEquals(resultFloat, tc.expected) {
				t.Errorf("expected %f, got %f", tc.expected, resultFloat)
			}
		})
	}
}

// TestCalculateTradeScoreFromPrices tests the combined trade score calculation.
func TestCalculateTradeScoreFromPrices(t *testing.T) {
	tests := []struct {
		name       string
		entryPrice float64
		exitPrice  float64
		qtyUsed    int64
		side       string
		expected   float64
	}{
		{"long 10% gain, qty=1", 100.0, 110.0, 1, SideLong, 10.0},
		{"long 10% gain, qty=10", 100.0, 110.0, 10, SideLong, 100.0},
		{"short 10% gain, qty=10", 100.0, 90.0, 10, SideShort, 100.0},
		{"long 1% loss, qty=100", 100.0, 99.0, 100, SideLong, -100.0},
		{"very large qty", 100.0, 101.0, 1000000, SideLong, 1000000.0}, // 1% * 1000000 = 1000000
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := decimal.NewFromFloat(tc.entryPrice)
			exit := decimal.NewFromFloat(tc.exitPrice)
			result := CalculateTradeScoreFromPrices(entry, exit, tc.qtyUsed, tc.side)
			resultFloat := ToFloat64(result)

			if !floatEquals(resultFloat, tc.expected) {
				t.Errorf("expected %f, got %f", tc.expected, resultFloat)
			}
		})
	}
}

// TestCalculateTotalScore tests the total score calculation.
func TestCalculateTotalScore(t *testing.T) {
	scores := []decimal.Decimal{
		decimal.NewFromFloat(10.0),
		decimal.NewFromFloat(20.0),
		decimal.NewFromFloat(-5.0),
	}
	unrealized := decimal.NewFromFloat(15.0)

	result := CalculateTotalScore(scores, unrealized)
	resultFloat := ToFloat64(result)
	expected := 40.0 // 10 + 20 - 5 + 15

	if !floatEquals(resultFloat, expected) {
		t.Errorf("expected %f, got %f", expected, resultFloat)
	}
}

// TestVerySmallPriceChanges tests precision with very small price changes.
func TestVerySmallPriceChanges(t *testing.T) {
	// 0.001% price change
	entryPrice := decimal.NewFromFloat(100.0)
	exitPrice := decimal.NewFromFloat(100.001)

	pctChange := CalculatePctChange(entryPrice, exitPrice, SideLong)
	expected := decimal.NewFromFloat(0.001)

	if !pctChange.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), pctChange.String())
	}

	// With qty = 1000000, score should be 1000
	// 0.001% * 1000000 = 0.001 * 1000000 = 1000
	score := CalculateTradeScoreFromPrices(entryPrice, exitPrice, 1000000, SideLong)
	scoreFloat := ToFloat64(score)
	expectedScore := 1000.0 // 0.001% * 1000000 = 1000

	if !floatEquals(scoreFloat, expectedScore) {
		t.Errorf("expected %f, got %f", expectedScore, scoreFloat)
	}
}

// TestVeryLargeQty tests precision with very large quantities (1,000,000).
func TestVeryLargeQty(t *testing.T) {
	entryPrice := decimal.NewFromFloat(100.0)
	exitPrice := decimal.NewFromFloat(101.0)

	// 1% gain with 1,000,000 qty = 1,000,000 score
	// pct_change = (101-100)/100 * 100 = 1%
	// score = 1,000,000 * 1 = 1,000,000
	score := CalculateTradeScoreFromPrices(entryPrice, exitPrice, 1000000, SideLong)
	scoreFloat := ToFloat64(score)
	expected := 1000000.0

	if !floatEquals(scoreFloat, expected) {
		t.Errorf("expected %f, got %f", expected, scoreFloat)
	}
}

// TestSumming10000Scores tests accumulation precision over 10,000+ trades.
// This is a critical test for the decimal precision improvement.
func TestSumming10000Scores(t *testing.T) {
	const numTrades = 10001

	// Create 10,001 small trade scores
	scores := make([]decimal.Decimal, numTrades)
	for i := 0; i < numTrades; i++ {
		// Each trade has a small score of 0.00000001 (1e-8)
		scores[i] = decimal.NewFromFloat(0.00000001)
	}

	result := CalculateRealizedTotal(scores)
	resultFloat := ToFloat64(result)

	// Expected: 10001 * 0.00000001 = 0.00010001
	expected := 0.00010001

	if !floatEquals(resultFloat, expected) {
		t.Errorf("expected %f, got %f (difference: %e)", expected, resultFloat, expected-resultFloat)
	}
}

// TestSumming10000LargerScores tests accumulation with larger scores.
func TestSumming10000LargerScores(t *testing.T) {
	const numTrades = 10000

	// Create 10,000 trade scores with alternating +1 and -1
	scores := make([]decimal.Decimal, numTrades)
	for i := 0; i < numTrades; i++ {
		if i%2 == 0 {
			scores[i] = decimal.NewFromFloat(1.123456789)
		} else {
			scores[i] = decimal.NewFromFloat(-1.123456789)
		}
	}

	result := CalculateRealizedTotal(scores)
	resultFloat := ToFloat64(result)

	// Expected: 0 (equal positive and negative)
	expected := 0.0

	if !floatEquals(resultFloat, expected) {
		t.Errorf("expected %f, got %f (difference: %e)", expected, resultFloat, expected-resultFloat)
	}
}

// TestFloat64AccumulationError demonstrates why decimal is needed.
// This test shows the precision loss that occurs with float64.
func TestFloat64AccumulationError(t *testing.T) {
	const numTrades = 10000

	// Using float64 (old method)
	var float64Sum float64
	for i := 0; i < numTrades; i++ {
		float64Sum += 0.1
	}
	float64Sum -= 1000.0 // Should be 0

	// Using decimal (new method)
	scores := make([]decimal.Decimal, numTrades)
	for i := 0; i < numTrades; i++ {
		scores[i] = decimal.NewFromFloat(0.1)
	}
	decimalSum := CalculateRealizedTotal(scores)
	decimalSum = decimalSum.Sub(decimal.NewFromInt(1000))

	decimalFloat := ToFloat64(decimalSum)

	// Float64 will have error, decimal should be exact
	t.Logf("float64 sum (should be 0): %e", float64Sum)
	t.Logf("decimal sum (should be 0): %e", decimalFloat)

	// Decimal should be exactly 0
	if !decimalSum.IsZero() {
		t.Logf("Note: decimal result is %s, which rounds to %f", decimalSum.String(), decimalFloat)
	}
}

// TestDecimalVsFloat64Comparison compares float64 and decimal for edge cases.
func TestDecimalVsFloat64Comparison(t *testing.T) {
	tests := []struct {
		name       string
		entryPrice float64
		exitPrice  float64
		qtyUsed    int64
		side       string
	}{
		{"very small change", 100.0, 100.001, 1000000, SideLong},
		{"large qty with small pct", 0.00001, 0.00001001, 1000000, SideLong},
		{"high precision prices", 123.456789, 123.457890, 100000, SideLong},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Calculate with decimal
			entry := decimal.NewFromFloat(tc.entryPrice)
			exit := decimal.NewFromFloat(tc.exitPrice)
			decimalResult := CalculateTradeScoreFromPrices(entry, exit, tc.qtyUsed, tc.side)
			decimalFloat := ToFloat64(decimalResult)

			// Calculate with float64 (old method)
			var pctChange float64
			if tc.side == SideLong {
				pctChange = (tc.exitPrice - tc.entryPrice) / tc.entryPrice * 100
			} else {
				pctChange = (tc.entryPrice - tc.exitPrice) / tc.entryPrice * 100
			}
			float64Result := float64(tc.qtyUsed) * pctChange

			t.Logf("decimal: %f, float64: %f, diff: %e",
				decimalFloat, float64Result, math.Abs(decimalFloat-float64Result))
		})
	}
}

// TestWeightedAveragePrice tests weighted average calculation for position additions.
func TestWeightedAveragePrice(t *testing.T) {
	tests := []struct {
		name          string
		oldEntry      float64
		oldQty        int64
		newPrice      float64
		newQty        int64
		expectedEntry float64
	}{
		{"equal weights", 100.0, 50, 110.0, 50, 105.0},
		{"more old qty", 100.0, 75, 110.0, 25, 102.5},
		{"more new qty", 100.0, 25, 110.0, 75, 107.5},
		{"same price", 100.0, 50, 100.0, 50, 100.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldEntry := decimal.NewFromFloat(tc.oldEntry)
			newPrice := decimal.NewFromFloat(tc.newPrice)

			result := CalculateWeightedAveragePrice(oldEntry, tc.oldQty, newPrice, tc.newQty)
			resultFloat := ToFloat64(result)

			if !floatEquals(resultFloat, tc.expectedEntry) {
				t.Errorf("expected %f, got %f", tc.expectedEntry, resultFloat)
			}
		})
	}
}

// TestStringConversions tests string-based decimal conversions.
func TestStringConversions(t *testing.T) {
	// Test ToString
	score := decimal.NewFromFloat(123.456789)
	str := ToString(score)
	if str != "123.45678900" {
		t.Errorf("expected 123.45678900, got %s", str)
	}

	// Test FromString
	parsed, err := FromString("123.45678900")
	if err != nil {
		t.Errorf("FromString returned unexpected error: %v", err)
	}
	if !parsed.Equal(decimal.NewFromFloat(123.456789)) {
		t.Errorf("FromString failed: got %s", parsed.String())
	}

	// Test FromString with invalid input
	invalid, err := FromString("not-a-number")
	if err == nil {
		t.Error("expected error for invalid string, got nil")
	}
	if !invalid.IsZero() {
		t.Errorf("expected zero for invalid string, got %s", invalid.String())
	}
}

// TestAddScore tests incremental score addition.
func TestAddScore(t *testing.T) {
	existing := decimal.NewFromFloat(100.12345678)
	delta := decimal.NewFromFloat(50.87654321)

	result := AddScore(existing, delta)
	expected := decimal.NewFromFloat(150.99999999)

	if !result.Equal(expected) {
		t.Errorf("expected %s, got %s", expected.String(), result.String())
	}
}

// TestCalculatePctChange_NegativeEntry tests handling of negative entry price.
func TestCalculatePctChange_NegativeEntry(t *testing.T) {
	entry := decimal.NewFromFloat(-100.0)
	exit := decimal.NewFromFloat(110.0)

	resultLong := CalculatePctChange(entry, exit, SideLong)
	if !resultLong.IsZero() {
		t.Errorf("expected Zero for negative entry (long), got %s", resultLong.String())
	}

	resultShort := CalculatePctChange(entry, exit, SideShort)
	if !resultShort.IsZero() {
		t.Errorf("expected Zero for negative entry (short), got %s", resultShort.String())
	}
}

// TestCalculatePctChange_InvalidSide tests that invalid side values return Zero.
func TestCalculatePctChange_InvalidSide(t *testing.T) {
	entry := decimal.NewFromFloat(100.0)
	exit := decimal.NewFromFloat(110.0)

	invalidSides := []string{"Long", "SHORT", "BUY", "SELL", "invalid", "", "LONG"}
	for _, side := range invalidSides {
		t.Run(side, func(t *testing.T) {
			result := CalculatePctChange(entry, exit, side)
			if !result.IsZero() {
				t.Errorf("expected Zero for invalid side %q, got %s", side, result.String())
			}
		})
	}
}

// TestValidateSide tests the ValidateSide helper function.
func TestValidateSide(t *testing.T) {
	tests := []struct {
		side  string
		valid bool
	}{
		{SideLong, true},
		{SideShort, true},
		{"Long", false},
		{"SHORT", false},
		{"BUY", false},
		{"SELL", false},
		{"", false},
		{"invalid", false},
		{"LONG", false},
	}

	for _, tc := range tests {
		t.Run(tc.side, func(t *testing.T) {
			result := ValidateSide(tc.side)
			if result != tc.valid {
				t.Errorf("ValidateSide(%q) = %v, want %v", tc.side, result, tc.valid)
			}
		})
	}
}

// TestCalculateTradeScoreFromPrices_NegativeEntry tests negative entry price handling.
func TestCalculateTradeScoreFromPrices_NegativeEntry(t *testing.T) {
	entry := decimal.NewFromFloat(-100.0)
	exit := decimal.NewFromFloat(110.0)

	result := CalculateTradeScoreFromPrices(entry, exit, 10, SideLong)
	if !result.IsZero() {
		t.Errorf("expected Zero for negative entry, got %s", result.String())
	}
}

// TestCalculateTradeScoreFromPrices_InvalidSide tests invalid side handling.
func TestCalculateTradeScoreFromPrices_InvalidSide(t *testing.T) {
	entry := decimal.NewFromFloat(100.0)
	exit := decimal.NewFromFloat(110.0)

	result := CalculateTradeScoreFromPrices(entry, exit, 10, "BUY")
	if !result.IsZero() {
		t.Errorf("expected Zero for invalid side, got %s", result.String())
	}
}

// TestCalculateUnrealizedScore_NegativePrices tests negative price handling.
func TestCalculateUnrealizedScore_NegativePrices(t *testing.T) {
	tests := []struct {
		name         string
		entryPrice   float64
		currentPrice float64
		side         string
	}{
		{"negative entry", -100.0, 110.0, SideLong},
		{"negative current", 100.0, -110.0, SideLong},
		{"both negative", -100.0, -110.0, SideLong},
		{"negative entry short", -100.0, 90.0, SideShort},
		{"negative current short", 100.0, -90.0, SideShort},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry := decimal.NewFromFloat(tc.entryPrice)
			current := decimal.NewFromFloat(tc.currentPrice)
			result := CalculateUnrealizedScore(entry, current, 10, tc.side)
			if !result.IsZero() {
				t.Errorf("expected Zero, got %s", result.String())
			}
		})
	}
}

// TestCalculateUnrealizedScore_InvalidSide tests invalid side handling.
func TestCalculateUnrealizedScore_InvalidSide(t *testing.T) {
	entry := decimal.NewFromFloat(100.0)
	current := decimal.NewFromFloat(110.0)

	result := CalculateUnrealizedScore(entry, current, 10, "SELL")
	if !result.IsZero() {
		t.Errorf("expected Zero for invalid side, got %s", result.String())
	}
}

// BenchmarkDecimalVsFloat64 compares performance of decimal vs float64.
func BenchmarkDecimalVsFloat64(b *testing.B) {
	b.Run("float64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			entryPrice := 100.0
			exitPrice := 101.0
			qtyUsed := int64(1000)
			pctChange := (exitPrice - entryPrice) / entryPrice * 100
			_ = float64(qtyUsed) * pctChange
		}
	})

	b.Run("decimal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			entry := decimal.NewFromFloat(100.0)
			exit := decimal.NewFromFloat(101.0)
			_ = CalculateTradeScoreFromPrices(entry, exit, 1000, SideLong)
		}
	})
}

// BenchmarkLargeAccumulation benchmarks accumulating many scores.
func BenchmarkLargeAccumulation(b *testing.B) {
	const numScores = 10000
	scores := make([]decimal.Decimal, numScores)
	for i := 0; i < numScores; i++ {
		scores[i] = decimal.NewFromFloat(0.12345678)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CalculateRealizedTotal(scores)
	}
}
