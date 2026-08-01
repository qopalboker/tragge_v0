package prizedistribution

import (
	"math"
	"os"
	"testing"
)

// TestTralentReference validates the reference scenario:
// 1000 participants, $100 entry, 17% commission → 1st ≈ $16,638
func TestTralentReference(t *testing.T) {
	participants := 1000
	entryFeeCents := 10000 // $100
	commissionRate := 0.17

	pool, err := CalculatePrizePool(participants, entryFeeCents, commissionRate)
	if err != nil {
		t.Fatalf("CalculatePrizePool error: %v", err)
	}

	// Gross = 1000 * 10000 = 10,000,000 cents = $100,000
	// Commission = floor(10,000,000 * 0.17) = 1,700,000
	// Net = 10,000,000 - 1,700,000 = 8,300,000 cents = $83,000
	expectedPool := int64(8300000)
	if pool != expectedPool {
		t.Fatalf("Expected prize pool %d, got %d", expectedPool, pool)
	}

	winners := GetWinnersCount(participants, DefaultWinnerPercent)
	if winners != 300 {
		t.Fatalf("Expected 300 winners, got %d", winners)
	}

	shares := CalculatePrizeDistribution(pool, winners, DefaultAlpha)
	if len(shares) != 300 {
		t.Fatalf("Expected 300 shares, got %d", len(shares))
	}

	// Verify 1st place is approximately $16,638 (within ±$500 tolerance)
	firstPrizeDollars := float64(shares[0].AmountCents) / 100.0
	t.Logf("1st place: $%.2f (target ≈ $16,638)", firstPrizeDollars)

	if firstPrizeDollars < 16000 || firstPrizeDollars > 17300 {
		t.Errorf("1st place $%.2f outside expected range [$16,000, $17,300]", firstPrizeDollars)
	}

	// Verify cent-perfect
	var sum int64
	for _, s := range shares {
		sum += s.AmountCents
	}
	if sum != pool {
		t.Errorf("Sum of prizes (%d) != prize pool (%d), diff=%d", sum, pool, pool-sum)
	}

	// Log top 10 for visibility
	for i := 0; i < 10 && i < len(shares); i++ {
		t.Logf("  Rank %d: $%.2f (%.4f%%)", shares[i].Rank, float64(shares[i].AmountCents)/100, shares[i].Percentage)
	}
	t.Logf("  Rank %d: $%.2f", shares[len(shares)-1].Rank, float64(shares[len(shares)-1].AmountCents)/100)
}

// TestCentPerfect verifies that the sum of all prizes equals the pool exactly.
func TestCentPerfect(t *testing.T) {
	testCases := []struct {
		pool    int64
		winners int
	}{
		{10000, 1},
		{10000, 3},
		{99999, 10},
		{100000, 30},
		{333333, 50},
		{1000000, 100},
		{8300000, 300},
		{7777777, 150},
		{3, 2},
		{1, 1},
		{5, 5},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			shares := CalculatePrizeDistribution(tc.pool, tc.winners, DefaultAlpha)
			if len(shares) == 0 {
				if tc.pool > 0 && tc.winners > 0 {
					t.Errorf("Expected shares for pool=%d, winners=%d", tc.pool, tc.winners)
				}
				return
			}

			var sum int64
			for _, s := range shares {
				sum += s.AmountCents
			}
			if sum != tc.pool {
				t.Errorf("pool=%d, winners=%d: sum=%d (diff=%d)", tc.pool, tc.winners, sum, tc.pool-sum)
			}
		})
	}
}

// TestMonotonicallyDecreasing verifies prizes are non-increasing by rank.
func TestMonotonicallyDecreasing(t *testing.T) {
	for _, n := range []int{3, 10, 30, 100, 300, 500} {
		t.Run("", func(t *testing.T) {
			pool := int64(n) * 10000
			shares := CalculatePrizeDistribution(pool, n, DefaultAlpha)

			// Rank 2 onward should be non-increasing (rank 1 may have remainder boost)
			for i := 2; i < len(shares); i++ {
				if shares[i].AmountCents > shares[i-1].AmountCents {
					t.Errorf("n=%d: rank %d (%d) > rank %d (%d)",
						n, shares[i].Rank, shares[i].AmountCents,
						shares[i-1].Rank, shares[i-1].AmountCents)
				}
			}
		})
	}
}

// TestAllPrizesPositive verifies every winner gets at least 1 cent.
func TestAllPrizesPositive(t *testing.T) {
	for _, n := range []int{3, 10, 50, 100, 300} {
		pool := int64(n) * 1000
		shares := CalculatePrizeDistribution(pool, n, DefaultAlpha)
		for _, s := range shares {
			if s.AmountCents <= 0 {
				t.Errorf("n=%d: rank %d has non-positive prize: %d", n, s.Rank, s.AmountCents)
			}
		}
	}
}

// TestEdgeCases tests boundary conditions.
func TestEdgeCases(t *testing.T) {
	// Zero pool
	if shares := CalculatePrizeDistribution(0, 10, DefaultAlpha); shares != nil {
		t.Errorf("Expected nil for zero pool, got %d shares", len(shares))
	}

	// Negative pool
	if shares := CalculatePrizeDistribution(-100, 10, DefaultAlpha); shares != nil {
		t.Errorf("Expected nil for negative pool, got %d shares", len(shares))
	}

	// Zero winners
	if shares := CalculatePrizeDistribution(10000, 0, DefaultAlpha); shares != nil {
		t.Errorf("Expected nil for zero winners, got %d shares", len(shares))
	}

	// 1 participant, 1 winner
	shares := CalculatePrizeDistribution(5000, 1, DefaultAlpha)
	if len(shares) != 1 {
		t.Fatalf("Expected 1 share, got %d", len(shares))
	}
	if shares[0].AmountCents != 5000 {
		t.Errorf("Single winner should get full pool (5000), got %d", shares[0].AmountCents)
	}

	// Pool smaller than winners
	shares = CalculatePrizeDistribution(3, 10, DefaultAlpha)
	if len(shares) != 3 {
		t.Fatalf("Expected 3 shares (capped by pool), got %d", len(shares))
	}
	var sum int64
	for _, s := range shares {
		sum += s.AmountCents
	}
	if sum != 3 {
		t.Errorf("Sum should be 3, got %d", sum)
	}
}

// TestGetWinnersCount tests winner count calculation.
func TestGetWinnersCount(t *testing.T) {
	tests := []struct {
		participants int
		winnerPct    float64
		want         int
	}{
		{0, 0.30, 0},
		{1, 0.30, 1},
		{2, 0.30, 1},
		{3, 0.30, 1},
		{4, 0.30, 2},
		{10, 0.30, 3},
		{100, 0.30, 30},
		{1000, 0.30, 300},
		{-5, 0.30, 0},
		{10, 0.0, 3},  // Falls back to default
		{10, -1, 3},   // Falls back to default
		{10, 1.5, 10}, // Capped at 1.0
		{10, 1.0, 10}, // All participants win
	}

	for _, tt := range tests {
		got := GetWinnersCount(tt.participants, tt.winnerPct)
		if got != tt.want {
			t.Errorf("GetWinnersCount(%d, %.2f) = %d, want %d",
				tt.participants, tt.winnerPct, got, tt.want)
		}
	}
}

// TestCalculatePrizePool tests net pool computation.
func TestCalculatePrizePool(t *testing.T) {
	tests := []struct {
		name    string
		p       int
		fee     int
		comm    float64
		want    int64
		wantErr bool
	}{
		{"basic 20%", 10, 1000, 0.20, 8000, false},
		{"17% commission", 1000, 10000, 0.17, 8300000, false},
		{"0% commission", 10, 1000, 0.0, 10000, false},
		{"0 participants", 0, 1000, 0.20, 0, false},
		{"negative commission", 10, 1000, -0.10, 0, true},
		{"100% commission", 10, 1000, 1.0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculatePrizePool(tt.p, tt.fee, tt.comm)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

// TestCalculatePrizePoolBps tests BPS-based pool computation.
func TestCalculatePrizePoolBps(t *testing.T) {
	tests := []struct {
		p    int
		fee  int64
		bps  int
		want int64
	}{
		{1000, 10000, 1700, 8300000},
		{10, 1000, 2000, 8000},
		{10, 1000, 0, 10000},
		{10, 1000, 10000, 0},
		{0, 1000, 2000, 0},
	}

	for _, tt := range tests {
		got := CalculatePrizePoolBps(tt.p, tt.fee, tt.bps)
		if got != tt.want {
			t.Errorf("CalculatePrizePoolBps(%d, %d, %d) = %d, want %d",
				tt.p, tt.fee, tt.bps, got, tt.want)
		}
	}
}

// TestPercentagesSum verifies percentages sum to approximately 100%.
func TestPercentagesSum(t *testing.T) {
	for _, n := range []int{1, 3, 10, 50, 100, 300} {
		shares := CalculatePrizeDistribution(1000000, n, DefaultAlpha)
		var sum float64
		for _, s := range shares {
			sum += s.Percentage
		}
		if math.Abs(sum-100.0) > 0.01 {
			t.Errorf("n=%d: percentages sum to %.4f, want ~100.0", n, sum)
		}
	}
}

// TestDeterministic verifies same inputs always produce same outputs.
func TestDeterministic(t *testing.T) {
	first := CalculatePrizeDistribution(8300000, 300, DefaultAlpha)
	for i := 0; i < 20; i++ {
		result := CalculatePrizeDistribution(8300000, 300, DefaultAlpha)
		if len(result) != len(first) {
			t.Fatalf("Run %d: different count: %d vs %d", i, len(result), len(first))
		}
		for j := range result {
			if result[j].AmountCents != first[j].AmountCents {
				t.Errorf("Run %d, rank %d: %d vs %d", i, j+1, result[j].AmountCents, first[j].AmountCents)
			}
		}
	}
}

// TestConfigFromEnv tests environment-based configuration.
func TestConfigFromEnv(t *testing.T) {
	// Default
	cfg := DefaultConfig()
	if cfg.Alpha != DefaultAlpha {
		t.Errorf("Default alpha = %f, want %f", cfg.Alpha, DefaultAlpha)
	}
	if cfg.WinnerPercent != DefaultWinnerPercent {
		t.Errorf("Default winner percent = %f, want %f", cfg.WinnerPercent, DefaultWinnerPercent)
	}

	// From env
	os.Setenv("PRIZE_ALPHA", "1.5")
	os.Setenv("PRIZE_WINNER_PERCENT", "0.25")
	defer os.Unsetenv("PRIZE_ALPHA")
	defer os.Unsetenv("PRIZE_WINNER_PERCENT")

	cfg = ConfigFromEnv()
	if cfg.Alpha != 1.5 {
		t.Errorf("Expected alpha 1.5, got %f", cfg.Alpha)
	}
	if cfg.WinnerPercent != 0.25 {
		t.Errorf("Expected winner percent 0.25, got %f", cfg.WinnerPercent)
	}
}

// TestVariousParticipantCounts tests distributions across different contest sizes.
func TestVariousParticipantCounts(t *testing.T) {
	for _, n := range []int{10, 50, 100, 500, 1000, 5000} {
		t.Run("", func(t *testing.T) {
			winners := GetWinnersCount(n, DefaultWinnerPercent)
			pool := int64(n) * 10000 // $100 entry each
			netPool := (pool * int64(10000-1700)) / 10000

			shares := CalculatePrizeDistribution(netPool, winners, DefaultAlpha)
			if len(shares) != winners {
				t.Errorf("n=%d: expected %d shares, got %d", n, winners, len(shares))
			}

			var sum int64
			for _, s := range shares {
				sum += s.AmountCents
				if s.AmountCents <= 0 {
					t.Errorf("n=%d: rank %d has non-positive prize", n, s.Rank)
				}
			}
			if sum != netPool {
				t.Errorf("n=%d: sum=%d != pool=%d", n, sum, netPool)
			}

			t.Logf("n=%d: winners=%d, pool=$%.2f, 1st=$%.2f, last=$%.2f",
				n, winners, float64(netPool)/100,
				float64(shares[0].AmountCents)/100,
				float64(shares[len(shares)-1].AmountCents)/100)
		})
	}
}

// TestZeroAlphaFallback tests that zero or negative alpha falls back to default.
func TestZeroAlphaFallback(t *testing.T) {
	shares0 := CalculatePrizeDistribution(10000, 3, 0)
	sharesDefault := CalculatePrizeDistribution(10000, 3, DefaultAlpha)

	if len(shares0) != len(sharesDefault) {
		t.Fatalf("Different counts: %d vs %d", len(shares0), len(sharesDefault))
	}
	for i := range shares0 {
		if shares0[i].AmountCents != sharesDefault[i].AmountCents {
			t.Errorf("Rank %d: zero alpha gave %d, default gave %d",
				i+1, shares0[i].AmountCents, sharesDefault[i].AmountCents)
		}
	}
}
