// Package prizedistribution implements a unified Power Law prize distribution
// formula for trading tournaments. All services (user-bff preview, leaderboard-worker
// payout, settlement-service distribution, and the prize package) delegate to
// this single implementation to guarantee consistent results.
//
// Formula: prize(rank) = PRIZE_POOL × (1/rank^α) / Σ(1/i^α)  where α = DefaultAlpha
package prizedistribution

import (
	"fmt"
	"math"
	"os"
	"strconv"
)

// PrizeShare represents a single prize position in the distribution.
type PrizeShare struct {
	Rank        int     // 1-indexed rank position
	AmountCents int64   // Prize amount in cents
	Percentage  float64 // Percentage of total prize pool (0–100)
}

// Constants
const (
	// DefaultAlpha is the Power Law exponent. Higher values concentrate
	// more prize money toward the top ranks.
	DefaultAlpha = 1.095

	// DefaultWinnerPercent is the fraction of participants who receive prizes (30%).
	DefaultWinnerPercent = 0.30

	// MinWinners is the minimum number of winners for any contest.
	MinWinners = 1
)

// Config holds the parameters for prize distribution.
type Config struct {
	Alpha         float64 // Power Law exponent (from PRIZE_ALPHA, default 1.095)
	WinnerPercent float64 // Fraction of participants who win (from PRIZE_WINNER_PERCENT, default 0.30)
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		Alpha:         DefaultAlpha,
		WinnerPercent: DefaultWinnerPercent,
	}
}

// ConfigFromEnv reads configuration from environment variables, falling back
// to defaults for unset or invalid values.
//
// Environment variables:
//   - PRIZE_ALPHA: Power Law exponent (default 1.095)
//   - PRIZE_WINNER_PERCENT: Winner fraction 0.0–1.0 (default 0.30)
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := os.Getenv("PRIZE_ALPHA"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.Alpha = f
		}
	}

	if v := os.Getenv("PRIZE_WINNER_PERCENT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
			cfg.WinnerPercent = f
		}
	}

	return cfg
}

// GetWinnersCount returns the number of winners for a given participant count.
// Result is ceil(participants * winnerPercent), with a minimum of MinWinners.
func GetWinnersCount(participants int, winnerPercent float64) int {
	if participants <= 0 {
		return 0
	}
	if winnerPercent <= 0 {
		winnerPercent = DefaultWinnerPercent
	}
	if winnerPercent > 1.0 {
		winnerPercent = 1.0
	}
	w := int(math.Ceil(float64(participants) * winnerPercent))
	if w < MinWinners {
		return MinWinners
	}
	if w > participants {
		return participants
	}
	return w
}

// CalculatePrizeDistribution computes the prize breakdown for each winner using
// the Power Law formula:
//
//	weight(rank) = 1 / rank^alpha
//	percentage(rank) = weight(rank) / sum(weights) * 100
//
// The distribution is cent-perfect: the sum of all AmountCents equals exactly
// prizePoolCents. Remainder cents from floor rounding go to 1st place.
// Every winner is guaranteed at least 1 cent.
//
// Returns nil if prizePoolCents <= 0 or numWinners <= 0.
func CalculatePrizeDistribution(prizePoolCents int64, numWinners int, alpha float64) []PrizeShare {
	if prizePoolCents <= 0 || numWinners <= 0 {
		return nil
	}

	if alpha <= 0 {
		alpha = DefaultAlpha
	}

	// Cap winners to pool size (each needs at least 1 cent)
	if int64(numWinners) > prizePoolCents {
		numWinners = int(prizePoolCents)
	}

	// Step 1: Compute harmonic weights: w(i) = 1 / i^α
	weights := make([]float64, numWinners)
	totalWeight := 0.0
	for i := 0; i < numWinners; i++ {
		rank := float64(i + 1)
		weights[i] = 1.0 / math.Pow(rank, alpha)
		totalWeight += weights[i]
	}

	// Step 2: Normalize to percentages (0–100)
	percentages := make([]float64, numWinners)
	for i := range weights {
		percentages[i] = (weights[i] / totalWeight) * 100.0
	}

	// Step 3: Reserve 1 cent per winner as floor guarantee
	floorTotal := int64(numWinners)
	distributable := prizePoolCents - floorTotal
	if distributable < 0 {
		distributable = 0
	}

	// Step 4: Allocate cents: floor(distributable * pct / 100) + 1 cent floor
	shares := make([]PrizeShare, numWinners)
	var allocated int64
	for i := 0; i < numWinners; i++ {
		var decayCents int64
		if distributable > 0 {
			decayCents = int64(math.Floor(float64(distributable) * percentages[i] / 100.0))
		}
		cents := 1 + decayCents
		shares[i] = PrizeShare{
			Rank:        i + 1,
			AmountCents: cents,
			Percentage:  percentages[i],
		}
		allocated += cents
	}

	// Step 5: Remainder to 1st place for cent-perfect distribution
	remainder := prizePoolCents - allocated
	if remainder > 0 && len(shares) > 0 {
		shares[0].AmountCents += remainder
	}

	return shares
}

// CalculatePrizePool computes the net prize pool in cents after commission.
//   - participants: number of contest participants
//   - entryFeeCents: entry fee per participant in cents
//   - commissionRate: platform commission as a fraction (e.g. 0.17 for 17%)
//
// Returns an error if commissionRate is negative or >= 1.0.
func CalculatePrizePool(participants int, entryFeeCents int, commissionRate float64) (int64, error) {
	if participants <= 0 || entryFeeCents <= 0 {
		return 0, nil
	}
	if commissionRate < 0 {
		return 0, fmt.Errorf("prizedistribution: commission rate %.4f is negative", commissionRate)
	}
	if commissionRate >= 1.0 {
		return 0, fmt.Errorf("prizedistribution: commission rate %.4f is >= 1.0", commissionRate)
	}
	gross := int64(participants) * int64(entryFeeCents)
	commission := int64(math.Floor(float64(gross) * commissionRate))
	return gross - commission, nil
}

// CalculatePrizePoolBps computes the net prize pool using basis points for the fee.
//   - participants: number of contest participants
//   - entryFeeCents: entry fee per participant in cents
//   - platformFeeBps: platform fee in basis points (e.g. 1700 for 17%)
//
// Returns 0 if platformFeeBps >= 10000.
func CalculatePrizePoolBps(participants int, entryFeeCents int64, platformFeeBps int) int64 {
	if participants <= 0 || entryFeeCents <= 0 {
		return 0
	}
	if platformFeeBps <= 0 {
		return int64(participants) * entryFeeCents
	}
	if platformFeeBps >= 10000 {
		return 0
	}
	gross := int64(participants) * entryFeeCents
	return (gross * int64(10000-platformFeeBps)) / 10000
}
