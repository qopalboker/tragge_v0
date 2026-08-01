// Package traggepoint provides calculation logic for the Tragge Point (T-Point) system.
//
// Tragge Point is a global reputation point that considers:
// 1. Trading performance (P&L results)
// 2. Number of participants in tournaments entered
// 3. Rank achieved in each tournament
// 4. Time decay (older results contribute less)
//
// The formula is:
//
//	TraggePoint = Σ (TournamentScore × ParticipantMultiplier × RankBonus × DecayFactor)
//
// Where:
//   - TournamentScore = Final P&L score in that tournament (only positive scores contribute)
//   - ParticipantMultiplier = log10(participants) / log10(1000), clamped to [0.1, 1.5]
//   - RankBonus = 1.0 + (0.5 × (1 - rank/total)), clamped to [1.0, 1.5]
//   - DecayFactor = 0.5 ^ (days_elapsed / half_life_days)
//
// This means winning a 1000-person tournament is worth more than winning a 10-person tournament
// with the same P&L score, and recent results weigh more than older ones.
package traggepoint

import (
	"math"
	"time"

	"github.com/shopspring/decimal"
)

const (
	// MinParticipantMultiplier is the minimum value for the participant multiplier.
	MinParticipantMultiplier = 0.1

	// MaxParticipantMultiplier is the maximum value for the participant multiplier.
	MaxParticipantMultiplier = 1.5

	// MinRankBonus is the minimum value for the rank bonus.
	MinRankBonus = 1.0

	// MaxRankBonus is the maximum value for the rank bonus.
	MaxRankBonus = 1.5

	// BaseParticipants is the base for the logarithmic participant multiplier (1000 participants = 1.0x).
	BaseParticipants = 1000

	// DefaultDecayHalfLifeDays is the default number of days for a contribution to halve.
	DefaultDecayHalfLifeDays = 180

	// MinDecayHalfLifeDays is the minimum allowed half-life to prevent division by zero.
	MinDecayHalfLifeDays = 1

	// Precision is the number of decimal places for internal calculations.
	Precision = 8
)

var (
	decZero     = decimal.Zero
	decOne      = decimal.NewFromInt(1)
	decHalf     = decimal.NewFromFloat(0.5)
	decPointOne = decimal.NewFromFloat(0.1)
	decOneFive  = decimal.NewFromFloat(1.5)
)

// Option configures a Calculator.
type Option func(*Calculator)

// WithDecayHalfLife sets the decay half-life in days.
func WithDecayHalfLife(days int) Option {
	return func(c *Calculator) {
		if days >= MinDecayHalfLifeDays {
			c.decayHalfLifeDays = days
		}
	}
}

// Calculator provides methods for calculating Tragge Point contributions.
type Calculator struct {
	decayHalfLifeDays int
}

// NewCalculator creates a new Tragge Point calculator.
func NewCalculator(opts ...Option) *Calculator {
	c := &Calculator{
		decayHalfLifeDays: DefaultDecayHalfLifeDays,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ContestResult represents the result of a user in a single contest.
type ContestResult struct {
	// TournamentScore is the final P&L score achieved in the tournament.
	TournamentScore decimal.Decimal
	// Rank is the user's final rank (1 = first place).
	Rank int
	// TotalParticipants is the total number of participants in the tournament.
	TotalParticipants int
	// CompletedAt is when the contest ended. Used for time-based decay.
	// If zero, no decay is applied to this result.
	CompletedAt time.Time
}

// ScoreBreakdown provides detailed breakdown of a point contribution.
type ScoreBreakdown struct {
	// BaseScore is the tournament score used for calculation.
	BaseScore decimal.Decimal
	// ParticipantMultiplier is the multiplier based on tournament size.
	ParticipantMultiplier decimal.Decimal
	// RankBonus is the bonus based on rank achieved.
	RankBonus decimal.Decimal
	// DecayFactor is the time-based decay multiplier.
	DecayFactor decimal.Decimal
	// FinalContribution is the final calculated contribution.
	FinalContribution decimal.Decimal
	// SkipReason explains why FinalContribution is zero, if applicable.
	// Empty string means the contribution was calculated normally.
	// Possible values: "negative_or_zero_score", "invalid_rank_or_participants".
	SkipReason string
}

// CalculateContribution calculates the Tragge Point contribution for a single contest result.
// Returns zero for invalid inputs or negative/zero tournament scores.
func (c *Calculator) CalculateContribution(result ContestResult, now time.Time) decimal.Decimal {
	breakdown := c.CalculateContributionWithBreakdown(result, now)
	return breakdown.FinalContribution
}

// CalculateContributionWithBreakdown calculates the contribution with a detailed breakdown.
func (c *Calculator) CalculateContributionWithBreakdown(result ContestResult, now time.Time) ScoreBreakdown {
	breakdown := ScoreBreakdown{
		BaseScore:             result.TournamentScore,
		ParticipantMultiplier: decOne,
		RankBonus:             decOne,
		DecayFactor:           decOne,
		FinalContribution:     decZero,
	}

	// Only positive scores contribute to reputation
	if result.TournamentScore.LessThanOrEqual(decZero) {
		breakdown.SkipReason = "negative_or_zero_score"
		return breakdown
	}

	// Validate inputs
	if result.Rank <= 0 || result.TotalParticipants <= 0 {
		breakdown.SkipReason = "invalid_rank_or_participants"
		return breakdown
	}

	// Calculate participant multiplier: log10(participants) / log10(1000)
	breakdown.ParticipantMultiplier = CalculateParticipantMultiplier(result.TotalParticipants)

	// Calculate rank bonus: 1.0 + (0.5 * (1 - rank/total))
	breakdown.RankBonus = CalculateRankBonus(result.Rank, result.TotalParticipants)

	// Calculate time-based decay factor
	breakdown.DecayFactor = c.CalculateDecayFactor(result.CompletedAt, now)

	// Calculate final contribution
	breakdown.FinalContribution = result.TournamentScore.
		Mul(breakdown.ParticipantMultiplier).
		Mul(breakdown.RankBonus).
		Mul(breakdown.DecayFactor).
		Round(Precision)

	return breakdown
}

// CalculateDecayFactor returns the decay multiplier for a contest completed at completedAt
// evaluated at time now. Uses exponential half-life: 0.5 ^ (days_elapsed / half_life_days).
// Returns 1.0 (no decay) if completedAt is zero or in the future.
func (c *Calculator) CalculateDecayFactor(completedAt time.Time, now time.Time) decimal.Decimal {
	if completedAt.IsZero() || now.Before(completedAt) {
		return decOne
	}

	daysElapsed := now.Sub(completedAt).Hours() / 24.0
	halfLife := float64(c.decayHalfLifeDays)

	// 0.5 ^ (days_elapsed / half_life)
	factor := math.Pow(0.5, daysElapsed/halfLife)
	return decimal.NewFromFloat(factor).Round(Precision)
}

// CalculateParticipantMultiplier calculates the multiplier based on tournament size.
// Uses log10(participants) / log10(1000) formula, clamped to [0.1, 1.5].
func CalculateParticipantMultiplier(totalParticipants int) decimal.Decimal {
	if totalParticipants <= 0 {
		return decPointOne
	}

	// log10(participants) / log10(1000)
	mult := math.Log10(float64(totalParticipants)) / math.Log10(float64(BaseParticipants))
	result := decimal.NewFromFloat(mult)

	if result.LessThan(decPointOne) {
		return decPointOne
	}
	if result.GreaterThan(decOneFive) {
		return decOneFive
	}
	return result.Round(Precision)
}

// CalculateRankBonus calculates the bonus based on rank achieved.
// Uses 1.0 + (0.5 * (1 - rank/total)) formula, clamped to [1.0, 1.5].
func CalculateRankBonus(rank, totalParticipants int) decimal.Decimal {
	if rank <= 0 || totalParticipants <= 0 {
		return decOne
	}

	rankRatio := decimal.NewFromInt(int64(rank)).Div(decimal.NewFromInt(int64(totalParticipants)))

	// 1.0 + (0.5 * (1.0 - rank/total))
	bonus := decOne.Add(decHalf.Mul(decOne.Sub(rankRatio)))

	if bonus.LessThan(decOne) {
		return decOne
	}
	if bonus.GreaterThan(decOneFive) {
		return decOneFive
	}
	return bonus.Round(Precision)
}

// CalculateTotalPoint calculates the total Tragge Point from multiple contest results.
func (c *Calculator) CalculateTotalPoint(results []ContestResult, now time.Time) decimal.Decimal {
	total := decZero
	for _, result := range results {
		total = total.Add(c.CalculateContribution(result, now))
	}
	return total.Round(Precision)
}

// CalculateTotalScore is an alias for CalculateTotalPoint for backward compatibility.
func (c *Calculator) CalculateTotalScore(results []ContestResult, now time.Time) decimal.Decimal {
	return c.CalculateTotalPoint(results, now)
}

// UserStats represents aggregated user statistics for display.
type UserStats struct {
	TotalContests    int
	TotalWins        int
	TotalTop3        int
	TotalScore       decimal.Decimal
	TraggePoint      decimal.Decimal
	WinRate          decimal.Decimal
	BestRank         int
	TotalPnL         decimal.Decimal
	TotalTrades      int
	AvgTradeDuration int
	BestMarket       string
	BestMarketPnL    decimal.Decimal
}

// CalculateWinRate calculates the win rate percentage.
func CalculateWinRate(wins, totalContests int) decimal.Decimal {
	if totalContests <= 0 {
		return decZero
	}
	return decimal.NewFromInt(int64(wins)).
		Div(decimal.NewFromInt(int64(totalContests))).
		Mul(decimal.NewFromInt(100)).
		Round(Precision)
}

// ToFloat64 converts a decimal score to float64 for API/Kafka boundaries.
func ToFloat64(d decimal.Decimal) float64 {
	f, _ := d.Float64()
	return f
}

// FromFloat64 converts a float64 to decimal.
func FromFloat64(f float64) decimal.Decimal {
	return decimal.NewFromFloat(f)
}

// ToString converts a decimal score to its string representation.
func ToString(d decimal.Decimal) string {
	return d.StringFixed(Precision)
}

// Global calculator instance for convenience.
var defaultCalculator = NewCalculator()

// CalculateContribution calculates contribution using the default calculator.
// Uses time.Now() as the reference time for decay.
func CalculateContribution(tournamentScore decimal.Decimal, rank, totalParticipants int, completedAt time.Time) decimal.Decimal {
	return defaultCalculator.CalculateContribution(ContestResult{
		TournamentScore:   tournamentScore,
		Rank:              rank,
		TotalParticipants: totalParticipants,
		CompletedAt:       completedAt,
	}, time.Now())
}

// CalculateContributionWithBreakdown calculates contribution with breakdown using the default calculator.
// Uses time.Now() as the reference time for decay.
func CalculateContributionWithBreakdown(tournamentScore decimal.Decimal, rank, totalParticipants int, completedAt time.Time) ScoreBreakdown {
	return defaultCalculator.CalculateContributionWithBreakdown(ContestResult{
		TournamentScore:   tournamentScore,
		Rank:              rank,
		TotalParticipants: totalParticipants,
		CompletedAt:       completedAt,
	}, time.Now())
}
