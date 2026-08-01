// Package scoring provides fixed-precision arithmetic for trade score calculations.
// All internal calculations use decimal.Decimal with 8 decimal places of precision
// to prevent floating-point accumulation errors over thousands of trades.
package scoring

import (
	"fmt"

	"github.com/shopspring/decimal"
)

const (
	// Precision is the number of decimal places used for internal calculations.
	Precision = 8

	// SideLong represents a long position.
	SideLong = "long"

	// SideShort represents a short position.
	SideShort = "short"
)

var (
	// Hundred is a constant for percentage calculations.
	Hundred = decimal.NewFromInt(100)

	// Zero is a decimal zero value.
	Zero = decimal.Zero
)

// ValidateSide returns true if the given side is a valid position side
// (either SideLong or SideShort).
func ValidateSide(side string) bool {
	return side == SideLong || side == SideShort
}

// CalculatePctChange calculates the percentage change between entry and exit prices.
// For LONG: pct_change = (exit_price - entry_price) / entry_price * 100
// For SHORT: pct_change = (entry_price - exit_price) / entry_price * 100
// Returns Zero if entryPrice is zero or negative, or if side is not SideLong/SideShort.
// Returns the result with 8 decimal places of precision.
func CalculatePctChange(entryPrice, exitPrice decimal.Decimal, side string) decimal.Decimal {
	if entryPrice.IsZero() || entryPrice.IsNegative() {
		return Zero
	}

	var pctChange decimal.Decimal
	switch side {
	case SideLong:
		// LONG: pct_change = (exit_price - entry_price) / entry_price * 100
		pctChange = exitPrice.Sub(entryPrice).Div(entryPrice).Mul(Hundred)
	case SideShort:
		// SHORT: pct_change = (entry_price - exit_price) / entry_price * 100
		pctChange = entryPrice.Sub(exitPrice).Div(entryPrice).Mul(Hundred)
	default:
		return Zero
	}

	return pctChange.Round(Precision)
}

// CalculateTradeScore calculates the trade score using the Tralent formula.
// trade_score = qty_used * pct_change
// Returns the result with 8 decimal places of precision.
func CalculateTradeScore(qtyUsed, pctChange decimal.Decimal) decimal.Decimal {
	return qtyUsed.Mul(pctChange).Round(Precision)
}

// CalculateTotalScore calculates the total score by summing all realized trade scores
// and adding the unrealized score.
// total_score = sum(realized_trade_scores) + unrealized_score
// Returns the result with 8 decimal places of precision.
func CalculateTotalScore(realizedScores []decimal.Decimal, unrealizedScore decimal.Decimal) decimal.Decimal {
	total := Zero
	for _, score := range realizedScores {
		total = total.Add(score)
	}
	total = total.Add(unrealizedScore)
	return total.Round(Precision)
}

// CalculateRealizedTotal sums all realized trade scores.
// Returns the result with 8 decimal places of precision.
func CalculateRealizedTotal(realizedScores []decimal.Decimal) decimal.Decimal {
	total := Zero
	for _, score := range realizedScores {
		total = total.Add(score)
	}
	return total.Round(Precision)
}

// AddScore adds a score delta to an existing score.
// Returns the result with 8 decimal places of precision.
func AddScore(existing, delta decimal.Decimal) decimal.Decimal {
	return existing.Add(delta).Round(Precision)
}

// CalculateTradeScoreFromPrices calculates the trade score in a single call.
// This combines CalculatePctChange and CalculateTradeScore for convenience.
// Returns Zero if entryPrice is zero or negative, or if side is invalid.
// Formula: qty_used * ((exit_price - entry_price) / entry_price * 100) for long
//
//	qty_used * ((entry_price - exit_price) / entry_price * 100) for short
func CalculateTradeScoreFromPrices(entryPrice, exitPrice decimal.Decimal, qtyUsed int64, side string) decimal.Decimal {
	if entryPrice.IsZero() || entryPrice.IsNegative() {
		return Zero
	}

	qtyDecimal := decimal.NewFromInt(qtyUsed)
	pctChange := CalculatePctChange(entryPrice, exitPrice, side)
	return CalculateTradeScore(qtyDecimal, pctChange)
}

// CalculateUnrealizedScore calculates the unrealized score for an open position.
// For LONG: unrealized_score = qty_used * ((current_price - entry_price) / entry_price * 100)
// For SHORT: unrealized_score = qty_used * ((entry_price - current_price) / entry_price * 100)
// Returns Zero if entryPrice or currentPrice is zero or negative, or if side is invalid.
func CalculateUnrealizedScore(entryPrice, currentPrice decimal.Decimal, qtyUsed int64, side string) decimal.Decimal {
	if entryPrice.IsZero() || entryPrice.IsNegative() || currentPrice.IsZero() || currentPrice.IsNegative() {
		return Zero
	}

	qtyDecimal := decimal.NewFromInt(qtyUsed)
	pctChange := CalculatePctChange(entryPrice, currentPrice, side)
	return CalculateTradeScore(qtyDecimal, pctChange)
}

// ToFloat64 converts a decimal score to float64 for API/Kafka boundaries.
// This should only be used when sending data to external systems that require float64.
func ToFloat64(score decimal.Decimal) float64 {
	f, _ := score.Float64()
	return f
}

// FromFloat64 converts a float64 to decimal.
// This should be used when receiving data from external systems.
func FromFloat64(f float64) decimal.Decimal {
	return decimal.NewFromFloat(f)
}

// FromInt64 converts an int64 to decimal.
func FromInt64(i int64) decimal.Decimal {
	return decimal.NewFromInt(i)
}

// ToString converts a decimal score to string representation.
// This is useful for JSON serialization where string is preferred over float.
func ToString(score decimal.Decimal) string {
	return score.StringFixed(Precision)
}

// FromString parses a string into a decimal.
// Returns an error if the string cannot be parsed, preventing silent data loss
// from corrupted or invalid score strings (e.g., from Kafka).
func FromString(s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Zero, fmt.Errorf("invalid score string %q: %w", s, err)
	}
	return d, nil
}

// CalculateWeightedAveragePrice calculates the weighted average entry price
// when adding to an existing position.
// new_entry = (old_entry * old_qty + new_price * new_qty) / (old_qty + new_qty)
func CalculateWeightedAveragePrice(oldEntryPrice decimal.Decimal, oldQty int64, newPrice decimal.Decimal, newQty int64) decimal.Decimal {
	oldQtyDec := decimal.NewFromInt(oldQty)
	newQtyDec := decimal.NewFromInt(newQty)
	totalQty := oldQtyDec.Add(newQtyDec)

	if totalQty.IsZero() {
		return Zero
	}

	numerator := oldEntryPrice.Mul(oldQtyDec).Add(newPrice.Mul(newQtyDec))
	return numerator.Div(totalQty).Round(Precision)
}
