package server

import (
	"testing"
)

// TestCalculateWinnersCountRounding tests that winners count uses ceiling rounding.
func TestCalculateWinnersCountRounding(t *testing.T) {
	tests := []struct {
		name              string
		participantsCount int
		winnersPercentage int
		expected          int
	}{
		// Basic cases
		{"10 participants, 30%", 10, 30, 3},
		{"100 participants, 30%", 100, 30, 30},
		{"1000 participants, 30%", 1000, 30, 300},

		// Ceiling rounding cases
		{"1 participant, 30%", 1, 30, 1},
		{"2 participants, 30%", 2, 30, 1},
		{"3 participants, 30%", 3, 30, 1},
		{"4 participants, 30%", 4, 30, 2},
		{"7 participants, 30%", 7, 30, 3},
		{"11 participants, 30%", 11, 30, 4},
		{"33 participants, 30%", 33, 30, 10},
		{"34 participants, 30%", 34, 30, 11},

		// Edge cases
		{"0 participants", 0, 30, 0},
		{"negative participants", -5, 30, 0},
		{"0% winners", 10, 0, 3},
		{"negative percentage", 10, -5, 3},
		{"100% winners", 10, 100, 10},
		{"over 100% winners", 10, 150, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateWinnersCount(tt.participantsCount, tt.winnersPercentage)
			if result != tt.expected {
				t.Errorf("CalculateWinnersCount(%d, %d) = %d, want %d",
					tt.participantsCount, tt.winnersPercentage, result, tt.expected)
			}
		})
	}
}

// TestCalculatePrizePoolGross tests gross prize pool calculation.
func TestCalculatePrizePoolGross(t *testing.T) {
	tests := []struct {
		name              string
		participantsCount int
		entryFeeCents     int64
		expected          int64
	}{
		{"10 participants, $10 entry", 10, 1000, 10000},
		{"100 participants, $25 entry", 100, 2500, 250000},
		{"1 participant, $1 entry", 1, 100, 100},
		{"0 participants", 0, 1000, 0},
		{"free entry", 10, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculatePrizePoolGross(tt.participantsCount, tt.entryFeeCents)
			if result != tt.expected {
				t.Errorf("CalculatePrizePoolGross(%d, %d) = %d, want %d",
					tt.participantsCount, tt.entryFeeCents, result, tt.expected)
			}
		})
	}
}

// TestCalculatePrizePoolNet tests net prize pool calculation with platform fee.
func TestCalculatePrizePoolNet(t *testing.T) {
	tests := []struct {
		name           string
		prizePoolGross int64
		platformFeeBps int
		expected       int64
	}{
		{"$100 gross, 17% fee (1700 bps)", 10000, 1700, 8300},
		{"$1000 gross, 10% fee (1000 bps)", 100000, 1000, 90000},
		{"$250 gross, 25% fee (2500 bps)", 25000, 2500, 18750},
		{"0% fee", 10000, 0, 10000},
		{"negative fee (treated as 0)", 10000, -100, 10000},
		{"100% fee (all to platform)", 10000, 10000, 0},
		{"over 100% fee (capped)", 10000, 15000, 0},
		{"$100.01 gross, 17% fee", 10001, 1700, 8300},
		{"$1 gross, 17% fee", 100, 1700, 83},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculatePrizePoolNet(tt.prizePoolGross, tt.platformFeeBps)
			if result != tt.expected {
				t.Errorf("CalculatePrizePoolNet(%d, %d) = %d, want %d",
					tt.prizePoolGross, tt.platformFeeBps, result, tt.expected)
			}
		})
	}
}

// TestPayoutSumNeverExceedsPrizePool tests that total payouts never exceed prize pool.
func TestPayoutSumNeverExceedsPrizePool(t *testing.T) {
	testCases := []struct {
		participantsCount int
		entryFeeCents     int64
		platformFeeBps    int
	}{
		{10, 1000, 1700},
		{25, 2500, 1500},
		{50, 500, 2000},
		{100, 10000, 1700},
		{7, 999, 1234},
		{13, 1337, 1111},
		{1, 100, 1700},
		{3, 100, 0},
		{3, 100, 9999},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			entries := make([]LeaderboardEntry, tc.participantsCount)
			for i := 0; i < tc.participantsCount; i++ {
				entries[i] = LeaderboardEntry{
					Rank:   i + 1,
					UserID: "user-" + string(rune('a'+i%26)),
					Score:  float64(tc.participantsCount - i),
				}
			}

			prizePoolGross := CalculatePrizePoolGross(tc.participantsCount, tc.entryFeeCents)
			prizePoolNet := CalculatePrizePoolNet(prizePoolGross, tc.platformFeeBps)

			payouts, err := AllocatePayouts(entries, prizePoolNet, tc.platformFeeBps)
			if err != nil {
				t.Fatalf("AllocatePayouts failed: %v", err)
			}

			var totalPaid int64
			for _, p := range payouts {
				totalPaid += p.PayoutCents
			}

			if totalPaid > prizePoolNet {
				t.Errorf("Total paid (%d) exceeds prize pool net (%d) for %d participants",
					totalPaid, prizePoolNet, tc.participantsCount)
			}

			for _, p := range payouts {
				if p.PayoutCents < 0 {
					t.Errorf("Negative payout found: %d for user %s", p.PayoutCents, p.UserID)
				}
			}
		})
	}
}

// TestAllocatePayoutsDeterministic tests that payouts are deterministic.
func TestAllocatePayoutsDeterministic(t *testing.T) {
	entries := []LeaderboardEntry{
		{Rank: 1, UserID: "user-alice", Score: 1000.0},
		{Rank: 2, UserID: "user-bob", Score: 900.0},
		{Rank: 3, UserID: "user-carol", Score: 800.0},
		{Rank: 4, UserID: "user-dave", Score: 700.0},
		{Rank: 5, UserID: "user-eve", Score: 600.0},
		{Rank: 6, UserID: "user-frank", Score: 500.0},
		{Rank: 7, UserID: "user-grace", Score: 400.0},
		{Rank: 8, UserID: "user-heidi", Score: 300.0},
		{Rank: 9, UserID: "user-ivan", Score: 200.0},
		{Rank: 10, UserID: "user-judy", Score: 100.0},
	}

	prizePoolNet := int64(100000) // $1000

	var firstResult []PayoutResult
	for i := 0; i < 10; i++ {
		payouts, err := AllocatePayouts(entries, prizePoolNet, 1700)
		if err != nil {
			t.Fatalf("AllocatePayouts failed: %v", err)
		}

		if i == 0 {
			firstResult = payouts
		} else {
			if len(payouts) != len(firstResult) {
				t.Errorf("Run %d: different number of payouts: got %d, want %d",
					i, len(payouts), len(firstResult))
				continue
			}

			for j := range payouts {
				if payouts[j].PayoutCents != firstResult[j].PayoutCents {
					t.Errorf("Run %d, index %d: different payout: got %d, want %d",
						i, j, payouts[j].PayoutCents, firstResult[j].PayoutCents)
				}
			}
		}
	}
}

// TestAllocatePayoutsCorrectDistribution tests payouts use Power Law formula.
func TestAllocatePayoutsCorrectDistribution(t *testing.T) {
	entries := make([]LeaderboardEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = LeaderboardEntry{
			Rank:   i + 1,
			UserID: "user-" + string(rune('A'+i)),
			Score:  float64(10 - i),
		}
	}

	prizePoolNet := int64(10000) // $100

	payouts, err := AllocatePayouts(entries, prizePoolNet, 1700)
	if err != nil {
		t.Fatalf("AllocatePayouts failed: %v", err)
	}

	// 10 participants, 30% = 3 winners
	if len(payouts) != 3 {
		t.Fatalf("Expected 3 payouts, got %d", len(payouts))
	}

	// Verify prizes are monotonically decreasing
	for i := 1; i < len(payouts); i++ {
		if payouts[i].PayoutCents > payouts[i-1].PayoutCents {
			t.Errorf("Rank %d (%d) > Rank %d (%d)",
				payouts[i].Rank, payouts[i].PayoutCents,
				payouts[i-1].Rank, payouts[i-1].PayoutCents)
		}
	}

	// Verify cent-perfect distribution
	var total int64
	for _, p := range payouts {
		total += p.PayoutCents
	}
	if total != prizePoolNet {
		t.Errorf("Total payouts (%d) != prize pool (%d)", total, prizePoolNet)
	}

	// Log the distribution for visibility
	for _, p := range payouts {
		t.Logf("Rank %d: $%.2f", p.Rank, float64(p.PayoutCents)/100)
	}
}

// TestEmptyInput tests edge cases with empty or nil inputs.
func TestEmptyInput(t *testing.T) {
	t.Run("empty entries", func(t *testing.T) {
		payouts, err := AllocatePayouts([]LeaderboardEntry{}, 10000, 1700)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(payouts) != 0 {
			t.Errorf("Expected no payouts, got %d", len(payouts))
		}
	})

	t.Run("nil entries", func(t *testing.T) {
		payouts, err := AllocatePayouts(nil, 10000, 1700)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(payouts) != 0 {
			t.Errorf("Expected no payouts, got %d", len(payouts))
		}
	})

	t.Run("zero prize pool", func(t *testing.T) {
		entries := []LeaderboardEntry{{Rank: 1, UserID: "user-1", Score: 100}}
		payouts, err := AllocatePayouts(entries, 0, 1700)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(payouts) != 0 {
			t.Errorf("Expected no payouts, got %d", len(payouts))
		}
	})

	t.Run("negative prize pool", func(t *testing.T) {
		entries := []LeaderboardEntry{{Rank: 1, UserID: "user-1", Score: 100}}
		payouts, err := AllocatePayouts(entries, -10000, 1700)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(payouts) != 0 {
			t.Errorf("Expected no payouts, got %d", len(payouts))
		}
	})
}

// TestCalculateContestPayouts tests the full contest payout calculation.
func TestCalculateContestPayouts(t *testing.T) {
	entries := []LeaderboardEntry{
		{Rank: 1, UserID: "user-1", Score: 100},
		{Rank: 2, UserID: "user-2", Score: 90},
		{Rank: 3, UserID: "user-3", Score: 80},
		{Rank: 4, UserID: "user-4", Score: 70},
		{Rank: 5, UserID: "user-5", Score: 60},
		{Rank: 6, UserID: "user-6", Score: 50},
		{Rank: 7, UserID: "user-7", Score: 40},
		{Rank: 8, UserID: "user-8", Score: 30},
		{Rank: 9, UserID: "user-9", Score: 20},
		{Rank: 10, UserID: "user-10", Score: 10},
	}

	payout, err := CalculateContestPayouts("contest-123", entries, 1000, 1700)
	if err != nil {
		t.Fatalf("CalculateContestPayouts failed: %v", err)
	}

	if payout.ContestID != "contest-123" {
		t.Errorf("Expected contest ID 'contest-123', got '%s'", payout.ContestID)
	}
	if payout.ParticipantsCount != 10 {
		t.Errorf("Expected 10 participants, got %d", payout.ParticipantsCount)
	}
	if payout.PrizePoolGross != 10000 {
		t.Errorf("Expected gross 10000, got %d", payout.PrizePoolGross)
	}
	if payout.PrizePoolNet != 8300 {
		t.Errorf("Expected net 8300, got %d", payout.PrizePoolNet)
	}
	if payout.WinnersCount != 3 {
		t.Errorf("Expected 3 winners, got %d", payout.WinnersCount)
	}

	var sum int64
	for _, p := range payout.Payouts {
		sum += p.PayoutCents
	}
	if payout.TotalPaidOut != sum {
		t.Errorf("TotalPaidOut (%d) doesn't match sum of payouts (%d)", payout.TotalPaidOut, sum)
	}
	if payout.TotalPaidOut > payout.PrizePoolNet {
		t.Errorf("TotalPaidOut (%d) exceeds PrizePoolNet (%d)", payout.TotalPaidOut, payout.PrizePoolNet)
	}
}

func TestResolveEffectiveFeeBps(t *testing.T) {
	tests := []struct {
		name           string
		platformFeeBps int
		commissionRate float64
		want           int
	}{
		{"platform_fee_bps takes priority when set", 2500, 20.0, 2500},
		{"falls back to commission_rate when bps is 0", 0, 20.0, 2000},
		{"commission_rate 17.0 converts to 1700 bps", 0, 17.0, 1700},
		{"both zero returns default 2000", 0, 0.0, DefaultPlatformFeeBps},
		{"commission_rate with floating-point rounding", 0, 19.995, 2000},
		{"negative platform_fee_bps ignored", -1, 20.0, 2000},
		{"negative commission_rate ignored", 0, -5.0, DefaultPlatformFeeBps},
		{"commission_rate over 100 ignored", 0, 150.0, DefaultPlatformFeeBps},
		{"small commission_rate 0.5 converts to 50 bps", 0, 0.5, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveEffectiveFeeBps(tt.platformFeeBps, tt.commissionRate)
			if got != tt.want {
				t.Errorf("ResolveEffectiveFeeBps(%d, %f) = %d, want %d",
					tt.platformFeeBps, tt.commissionRate, got, tt.want)
			}
		})
	}
}
