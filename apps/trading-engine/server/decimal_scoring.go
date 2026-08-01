package server

import (
	"github.com/Parsaeffatravesh/tragge/packages/scoring"
	"github.com/shopspring/decimal"
)

// DecimalScore holds both the decimal value and its float64 representation.
// This is used for internal calculations while maintaining backward compatibility.
type DecimalScore struct {
	Decimal decimal.Decimal
	Float64 float64
}

// NewDecimalScore creates a new DecimalScore from a decimal value.
func NewDecimalScore(d decimal.Decimal) DecimalScore {
	return DecimalScore{
		Decimal: d,
		Float64: scoring.ToFloat64(d),
	}
}

// NewDecimalScoreFromFloat creates a new DecimalScore from a float64 value.
func NewDecimalScoreFromFloat(f float64) DecimalScore {
	d := scoring.FromFloat64(f)
	return DecimalScore{
		Decimal: d,
		Float64: f,
	}
}

// Zero returns a zero DecimalScore.
func ZeroDecimalScore() DecimalScore {
	return DecimalScore{
		Decimal: decimal.Zero,
		Float64: 0,
	}
}

// Add adds a delta to the score and returns a new DecimalScore.
func (ds DecimalScore) Add(delta DecimalScore) DecimalScore {
	result := ds.Decimal.Add(delta.Decimal)
	return NewDecimalScore(result)
}

// String returns the decimal string representation with 8 decimal places.
func (ds DecimalScore) String() string {
	return scoring.ToString(ds.Decimal)
}

// calculateTradeScoreDecimal calculates the trade score using decimal precision.
// This replaces the float64 calculateTradeScore function for internal calculations.
// Returns both decimal and float64 values for backward compatibility.
func calculateTradeScoreDecimal(side string, entryPrice, exitPrice float64, qtyUsed int64) DecimalScore {
	entry := scoring.FromFloat64(entryPrice)
	exit := scoring.FromFloat64(exitPrice)

	scoringSide := PositionSideToScoringSide(side)

	result := scoring.CalculateTradeScoreFromPrices(entry, exit, qtyUsed, scoringSide)
	return NewDecimalScore(result)
}

// calculateUnrealizedScoreDecimal calculates the unrealized score for a single position.
// Returns both decimal and float64 values for backward compatibility.
func calculateUnrealizedScoreDecimal(entryPrice, currentPrice float64, qtyUsed int64, isLong bool) DecimalScore {
	entry := scoring.FromFloat64(entryPrice)
	current := scoring.FromFloat64(currentPrice)

	side := scoring.SideShort
	if isLong {
		side = scoring.SideLong
	}

	result := scoring.CalculateUnrealizedScore(entry, current, qtyUsed, side)
	return NewDecimalScore(result)
}

// calculateTotalScoreDecimal calculates the total score from realized and unrealized components.
func calculateTotalScoreDecimal(realizedScore, unrealizedScore DecimalScore) DecimalScore {
	result := scoring.CalculateTotalScore(
		[]decimal.Decimal{realizedScore.Decimal},
		unrealizedScore.Decimal,
	)
	return NewDecimalScore(result)
}

// addRealizedScoreDecimal adds a delta to the realized score.
func addRealizedScoreDecimal(existing, delta DecimalScore) DecimalScore {
	result := scoring.AddScore(existing.Decimal, delta.Decimal)
	return NewDecimalScore(result)
}

// calculateWeightedAverageEntryDecimal calculates the weighted average entry price
// when adding to an existing position.
func calculateWeightedAverageEntryDecimal(oldEntry float64, oldQty int64, newPrice float64, newQty int64) float64 {
	oldEntryDec := scoring.FromFloat64(oldEntry)
	newPriceDec := scoring.FromFloat64(newPrice)

	result := scoring.CalculateWeightedAveragePrice(oldEntryDec, oldQty, newPriceDec, newQty)
	return scoring.ToFloat64(result)
}
