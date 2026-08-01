package integration

import (
	"math"
	"testing"

	prizedistribution "github.com/Parsaeffatravesh/tragge/packages/scoring/distribution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrizeDistributionConsistency verifies that for a given contest
// configuration, all three consumers (user-bff preview, leaderboard-worker
// payout, settlement-service settlement) produce identical results because
// they all delegate to the same shared prizedistribution package.
func TestPrizeDistributionConsistency(t *testing.T) {
	testCases := []struct {
		name              string
		participants      int
		entryFeeCents     int64
		platformFeeBps    int
		expectedFirstApprox int64 // approximate 1st place prize for sanity check
	}{
		{
			name:              "reference: 1000 participants, $100 entry, 17% commission",
			participants:      1000,
			entryFeeCents:     10000,
			platformFeeBps:    1700,
			expectedFirstApprox: 1663898, // ~$16,638.98
		},
		{
			name:              "small contest: 10 participants, $10 entry, 20% commission",
			participants:      10,
			entryFeeCents:     1000,
			platformFeeBps:    2000,
			expectedFirstApprox: 0, // just verify consistency
		},
		{
			name:              "medium contest: 50 participants, $25 entry, 15% commission",
			participants:      50,
			entryFeeCents:     2500,
			platformFeeBps:    1500,
			expectedFirstApprox: 0,
		},
		{
			name:              "large contest: 500 participants, $50 entry, 17% commission",
			participants:      500,
			entryFeeCents:     5000,
			platformFeeBps:    1700,
			expectedFirstApprox: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := prizedistribution.Config{
				Alpha:         prizedistribution.DefaultAlpha,
				WinnerPercent: prizedistribution.DefaultWinnerPercent,
			}

			// Step 1: Calculate prize pool (same logic all consumers use)
			prizePoolNet := prizedistribution.CalculatePrizePoolBps(
				tc.participants, tc.entryFeeCents, tc.platformFeeBps,
			)
			require.Greater(t, prizePoolNet, int64(0), "prize pool must be positive")

			// Step 2: Calculate winners count
			winnersCount := prizedistribution.GetWinnersCount(tc.participants, cfg.WinnerPercent)
			require.Greater(t, winnersCount, 0, "winners count must be positive")
			expectedWinners := int(math.Ceil(float64(tc.participants) * cfg.WinnerPercent))
			assert.Equal(t, expectedWinners, winnersCount, "winners count mismatch")

			// Step 3: Calculate distribution (the core shared function)
			shares := prizedistribution.CalculatePrizeDistribution(prizePoolNet, winnersCount, cfg.Alpha)
			require.NotEmpty(t, shares, "shares must not be empty")

			// Verify cent-perfect distribution: sum == prize pool
			var totalCents int64
			for _, s := range shares {
				totalCents += s.AmountCents
			}
			assert.Equal(t, prizePoolNet, totalCents,
				"total distributed (%d) must equal prize pool (%d)", totalCents, prizePoolNet)

			// Verify monotonically decreasing
			for i := 1; i < len(shares); i++ {
				assert.GreaterOrEqual(t, shares[i-1].AmountCents, shares[i].AmountCents,
					"rank %d (%d) should be >= rank %d (%d)",
					shares[i-1].Rank, shares[i-1].AmountCents,
					shares[i].Rank, shares[i].AmountCents)
			}

			// Verify all prizes positive
			for _, s := range shares {
				assert.Greater(t, s.AmountCents, int64(0),
					"rank %d prize must be positive", s.Rank)
			}

			// Verify ranks are sequential starting from 1
			for i, s := range shares {
				assert.Equal(t, i+1, s.Rank, "rank should be sequential")
			}

			// Verify determinism: run again and compare
			shares2 := prizedistribution.CalculatePrizeDistribution(prizePoolNet, winnersCount, cfg.Alpha)
			require.Equal(t, len(shares), len(shares2), "determinism: different number of shares")
			for i := range shares {
				assert.Equal(t, shares[i].AmountCents, shares2[i].AmountCents,
					"determinism: rank %d differs between runs", shares[i].Rank)
			}

			// Verify reference case approximate first place
			if tc.expectedFirstApprox > 0 {
				assert.InDelta(t, tc.expectedFirstApprox, shares[0].AmountCents, 100,
					"1st place should be approximately %d cents", tc.expectedFirstApprox)
			}

			t.Logf("Participants=%d, PrizePool=$%.2f, Winners=%d, 1st=$%.2f, Last=$%.2f",
				tc.participants,
				float64(prizePoolNet)/100,
				winnersCount,
				float64(shares[0].AmountCents)/100,
				float64(shares[len(shares)-1].AmountCents)/100)
		})
	}
}

// TestPrizeDistributionConsistencyAcrossMultipleCalls verifies that repeated
// calls with the same parameters always produce identical results, ensuring
// no hidden state or randomness affects the output.
func TestPrizeDistributionConsistencyAcrossMultipleCalls(t *testing.T) {
	cfg := prizedistribution.ConfigFromEnv()
	prizePool := int64(830000) // $8,300
	winnersCount := prizedistribution.GetWinnersCount(100, cfg.WinnerPercent)

	var baseline []prizedistribution.PrizeShare
	for i := 0; i < 20; i++ {
		shares := prizedistribution.CalculatePrizeDistribution(prizePool, winnersCount, cfg.Alpha)
		if i == 0 {
			baseline = shares
		} else {
			require.Equal(t, len(baseline), len(shares), "run %d: different share count", i)
			for j := range shares {
				assert.Equal(t, baseline[j].AmountCents, shares[j].AmountCents,
					"run %d, rank %d: amount differs", i, shares[j].Rank)
				assert.Equal(t, baseline[j].Percentage, shares[j].Percentage,
					"run %d, rank %d: percentage differs", i, shares[j].Rank)
			}
		}
	}
}
