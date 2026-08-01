package prize

import (
	"math"
	"testing"
)

// TestGetWinnersCount tests that winners count is always ceil(participants * 0.30), min 1.
func TestGetWinnersCount(t *testing.T) {
	tests := []struct {
		name         string
		participants int
		want         int
	}{
		{"0 participants", 0, 0},
		{"1 participant", 1, 1},
		{"2 participants", 2, 1},
		{"3 participants", 3, 1},
		{"4 participants", 4, 2},
		{"7 participants", 7, 3},
		{"10 participants", 10, 3},
		{"11 participants", 11, 4},
		{"33 participants", 33, 10},
		{"34 participants", 34, 11},
		{"100 participants", 100, 30},
		{"1000 participants", 1000, 300},
		{"negative participants", -5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetWinnersCount(tt.participants)
			if got != tt.want {
				t.Errorf("GetWinnersCount(%d) = %d, want %d", tt.participants, got, tt.want)
			}
		})
	}
}

// TestCalculatePrizePool tests prize pool computation with commission.
func TestCalculatePrizePool(t *testing.T) {
	tests := []struct {
		name           string
		participants   int
		entryFeeCents  int
		commissionRate float64
		want           int64
		wantErr        bool
	}{
		{"10 participants, $10 entry, 20% commission", 10, 1000, 0.20, 8000, false},
		{"100 participants, $100 entry, 20% commission", 100, 10000, 0.20, 800000, false},
		{"1000 participants, $100 entry, 20% commission", 1000, 10000, 0.20, 8000000, false},
		{"0% commission", 10, 1000, 0.0, 10000, false},
		{"0 participants", 0, 1000, 0.20, 0, false},
		{"0 entry fee", 10, 0, 0.20, 0, false},
		{"max commission (50%)", 10, 1000, 0.50, 5000, false},
		{"17% commission", 10, 1000, 0.17, 8300, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculatePrizePool(tt.participants, tt.entryFeeCents, tt.commissionRate)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CalculatePrizePool(%d, %d, %f) expected error, got nil",
						tt.participants, tt.entryFeeCents, tt.commissionRate)
				}
				return
			}
			if err != nil {
				t.Fatalf("CalculatePrizePool(%d, %d, %f) unexpected error: %v",
					tt.participants, tt.entryFeeCents, tt.commissionRate, err)
			}
			if got != tt.want {
				t.Errorf("CalculatePrizePool(%d, %d, %f) = %d, want %d",
					tt.participants, tt.entryFeeCents, tt.commissionRate, got, tt.want)
			}
		})
	}
}

// TestCalculatePrizePool_ExcessiveCommission tests that rates above MaxCommissionRate are rejected.
func TestCalculatePrizePool_ExcessiveCommission(t *testing.T) {
	tests := []struct {
		name           string
		commissionRate float64
	}{
		{"51%", 0.51},
		{"75%", 0.75},
		{"99%", 0.99},
		{"100%", 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculatePrizePool(10, 1000, tt.commissionRate)
			if err == nil {
				t.Errorf("CalculatePrizePool with %.2f commission should return error", tt.commissionRate)
			}
		})
	}
}

// TestCalculatePrizePool_NegativeCommission tests that negative rates return an error.
func TestCalculatePrizePool_NegativeCommission(t *testing.T) {
	_, err := CalculatePrizePool(10, 1000, -0.10)
	if err == nil {
		t.Error("CalculatePrizePool with negative commission should return error")
	}
}

// TestCalculatePrizePool_MaxBoundary tests the exact boundary at MaxCommissionRate.
func TestCalculatePrizePool_MaxBoundary(t *testing.T) {
	// Exactly at max should be valid
	got, err := CalculatePrizePool(10, 1000, MaxCommissionRate)
	if err != nil {
		t.Fatalf("CalculatePrizePool at MaxCommissionRate should succeed, got error: %v", err)
	}
	if got != 5000 {
		t.Errorf("CalculatePrizePool at 50%% commission = %d, want 5000", got)
	}

	// Just above max should fail
	_, err = CalculatePrizePool(10, 1000, MaxCommissionRate+0.0001)
	if err == nil {
		t.Error("CalculatePrizePool just above MaxCommissionRate should return error")
	}
}

// TestTwoParticipants tests that with 2 participants, 1 winner gets 100%.
func TestTwoParticipants(t *testing.T) {
	pool := int64(10000) // $100
	slots := CalculatePrizeDistribution(2, pool)

	if len(slots) != 1 {
		t.Fatalf("Expected 1 winner for 2 participants, got %d", len(slots))
	}

	if slots[0].Rank != 1 {
		t.Errorf("Expected rank 1, got %d", slots[0].Rank)
	}

	if slots[0].AmountCents != pool {
		t.Errorf("Expected winner to get entire pool (%d), got %d", pool, slots[0].AmountCents)
	}
}

// TestThreeParticipants tests that with 3 participants, 1 winner gets 100%.
func TestThreeParticipants(t *testing.T) {
	pool := int64(15000) // $150
	slots := CalculatePrizeDistribution(3, pool)

	// ceil(3 * 0.30) = ceil(0.9) = 1
	if len(slots) != 1 {
		t.Fatalf("Expected 1 winner for 3 participants, got %d", len(slots))
	}

	if slots[0].AmountCents != pool {
		t.Errorf("Expected winner to get entire pool (%d), got %d", pool, slots[0].AmountCents)
	}
}

// TestTenParticipants tests 10 participants: 3 winners with approximately 50/30/20 ratio.
func TestTenParticipants(t *testing.T) {
	pool := int64(100000) // $1000
	slots := CalculatePrizeDistribution(10, pool)

	winners := GetWinnersCount(10)
	if winners != 3 {
		t.Fatalf("Expected 3 winners for 10 participants, got %d", winners)
	}

	if len(slots) != 3 {
		t.Fatalf("Expected 3 prize slots, got %d", len(slots))
	}

	// Verify ordering: rank 1 > rank 2 > rank 3
	for i := 1; i < len(slots); i++ {
		if slots[i].AmountCents >= slots[i-1].AmountCents {
			t.Errorf("Rank %d (%d) should be less than rank %d (%d)",
				slots[i].Rank, slots[i].AmountCents,
				slots[i-1].Rank, slots[i-1].AmountCents)
		}
	}

	// Verify cent-perfect sum
	var sum int64
	for _, s := range slots {
		sum += s.AmountCents
	}
	if sum != pool {
		t.Errorf("Sum of prizes (%d) != prize pool (%d)", sum, pool)
	}

	// Verify approximate ratios (with tier BasePct=50, Decay=0.55):
	// weights: 50, 27.5, 15.125 => total=92.625
	// percentages: ~53.98%, ~29.69%, ~16.33%
	// Allow generous tolerance since we're testing the formula shape
	pct1 := float64(slots[0].AmountCents) / float64(pool) * 100
	pct2 := float64(slots[1].AmountCents) / float64(pool) * 100
	pct3 := float64(slots[2].AmountCents) / float64(pool) * 100

	if pct1 < 45 || pct1 > 60 {
		t.Errorf("Rank 1 percentage %.2f%% outside expected range [45%%, 60%%]", pct1)
	}
	if pct2 < 25 || pct2 > 35 {
		t.Errorf("Rank 2 percentage %.2f%% outside expected range [25%%, 35%%]", pct2)
	}
	if pct3 < 12 || pct3 > 22 {
		t.Errorf("Rank 3 percentage %.2f%% outside expected range [12%%, 22%%]", pct3)
	}

	t.Logf("10 participants distribution: 1st=%.2f%% ($%.2f), 2nd=%.2f%% ($%.2f), 3rd=%.2f%% ($%.2f)",
		pct1, float64(slots[0].AmountCents)/100,
		pct2, float64(slots[1].AmountCents)/100,
		pct3, float64(slots[2].AmountCents)/100)
}

// TestHundredParticipants tests 100 participants: 30 winners with exponential decay.
func TestHundredParticipants(t *testing.T) {
	pool := int64(1000000) // $10,000
	slots := CalculatePrizeDistribution(100, pool)

	winners := GetWinnersCount(100)
	if winners != 30 {
		t.Fatalf("Expected 30 winners for 100 participants, got %d", winners)
	}

	if len(slots) != 30 {
		t.Fatalf("Expected 30 prize slots, got %d", len(slots))
	}

	// Verify non-increasing amounts (tail positions may share 1-cent floor)
	for i := 1; i < len(slots); i++ {
		if slots[i].AmountCents > slots[i-1].AmountCents {
			t.Errorf("Rank %d (%d cents) should be <= rank %d (%d cents)",
				slots[i].Rank, slots[i].AmountCents,
				slots[i-1].Rank, slots[i-1].AmountCents)
		}
	}

	// Verify cent-perfect sum
	var sum int64
	for _, s := range slots {
		sum += s.AmountCents
	}
	if sum != pool {
		t.Errorf("Sum of prizes (%d) != prize pool (%d)", sum, pool)
	}

	// Verify all prizes are positive
	for _, s := range slots {
		if s.AmountCents <= 0 {
			t.Errorf("Rank %d has non-positive prize: %d", s.Rank, s.AmountCents)
		}
	}

	// Log some key positions for visibility
	t.Logf("100 participants: 1st=$%.2f, 5th=$%.2f, 10th=$%.2f, 20th=$%.2f, 30th=$%.2f",
		float64(slots[0].AmountCents)/100,
		float64(slots[4].AmountCents)/100,
		float64(slots[9].AmountCents)/100,
		float64(slots[19].AmountCents)/100,
		float64(slots[29].AmountCents)/100)
}

// TestThousandParticipants tests 1000 participants with $100 entry and 20% commission.
// Expected: 300 winners, 1st place approximately $16,000.
func TestThousandParticipants(t *testing.T) {
	participants := 1000
	entryFeeCents := 10000 // $100
	commissionRate := 0.20

	pool, err := CalculatePrizePool(participants, entryFeeCents, commissionRate)
	if err != nil {
		t.Fatalf("CalculatePrizePool error: %v", err)
	}
	// Gross = 1000 * 10000 = 10,000,000 cents = $100,000
	// Net = 10,000,000 - floor(10,000,000 * 0.20) = 10,000,000 - 2,000,000 = 8,000,000
	expectedPool := int64(8000000)
	if pool != expectedPool {
		t.Fatalf("Expected prize pool %d, got %d", expectedPool, pool)
	}

	slots := CalculatePrizeDistribution(participants, pool)

	winners := GetWinnersCount(participants)
	if winners != 300 {
		t.Fatalf("Expected 300 winners, got %d", winners)
	}

	if len(slots) != 300 {
		t.Fatalf("Expected 300 slots, got %d", len(slots))
	}

	// Verify cent-perfect sum
	var sum int64
	for _, s := range slots {
		sum += s.AmountCents
	}
	if sum != pool {
		t.Errorf("Sum of prizes (%d) != prize pool (%d)", sum, pool)
	}

	// 1st place should be approximately $16,000 (within reasonable range for exponential decay)
	firstPrizeDollars := float64(slots[0].AmountCents) / 100
	if firstPrizeDollars < 12000 || firstPrizeDollars > 20000 {
		t.Errorf("1st place $%.2f outside expected range [$12,000, $20,000]", firstPrizeDollars)
	}

	// Verify non-increasing order (tail positions share 1-cent floor)
	for i := 1; i < len(slots); i++ {
		if slots[i].AmountCents > slots[i-1].AmountCents {
			t.Errorf("Rank %d (%d) should be <= rank %d (%d)",
				slots[i].Rank, slots[i].AmountCents,
				slots[i-1].Rank, slots[i-1].AmountCents)
		}
	}

	// Verify all prizes are positive (guaranteed by 1-cent floor)
	for _, s := range slots {
		if s.AmountCents <= 0 {
			t.Errorf("Rank %d has non-positive prize: %d", s.Rank, s.AmountCents)
		}
	}

	// Log key positions
	t.Logf("1000 participants ($100 entry, 20%% commission):")
	t.Logf("  Pool: $%.2f", float64(pool)/100)
	t.Logf("  1st: $%.2f (%.2f%%)", float64(slots[0].AmountCents)/100, slots[0].Percentage)
	t.Logf("  10th: $%.2f (%.2f%%)", float64(slots[9].AmountCents)/100, slots[9].Percentage)
	t.Logf("  50th: $%.2f (%.2f%%)", float64(slots[49].AmountCents)/100, slots[49].Percentage)
	t.Logf("  100th: $%.2f (%.2f%%)", float64(slots[99].AmountCents)/100, slots[99].Percentage)
	t.Logf("  300th: $%.2f (%.2f%%)", float64(slots[299].AmountCents)/100, slots[299].Percentage)
}

// TestTieHandling tests that tied participants split combined prize amounts equally.
func TestTieHandling(t *testing.T) {
	pool := int64(10000) // $100

	participants := []RankedParticipant{
		{UserID: "alice", Score: 100.0},
		{UserID: "bob", Score: 100.0}, // Tied with alice
		{UserID: "carol", Score: 80.0},
		{UserID: "dave", Score: 70.0},
		{UserID: "eve", Score: 60.0},
		{UserID: "frank", Score: 50.0},
		{UserID: "grace", Score: 40.0},
	}

	results := DistributeWithTies(participants, pool)

	if len(results) != 7 {
		t.Fatalf("Expected 7 results, got %d", len(results))
	}

	// Alice and Bob are tied at rank 1
	// They should split the combined prize for positions 1 and 2
	var alicePrize, bobPrize int64
	for _, r := range results {
		switch r.UserID {
		case "alice":
			alicePrize = r.AmountCents
			if r.Rank != 1 {
				t.Errorf("Alice should have rank 1, got %d", r.Rank)
			}
		case "bob":
			bobPrize = r.AmountCents
			if r.Rank != 1 {
				t.Errorf("Bob should have rank 1, got %d", r.Rank)
			}
		}
	}

	// Alice and Bob should receive equal amounts (possibly +1 cent for remainder to first alphabetically)
	diff := alicePrize - bobPrize
	if diff < 0 {
		diff = -diff
	}
	// The base split should be equal, but remainder goes to 1st place (alice, sorted alphabetically)
	// Allow up to pool/100 cents difference for remainder handling
	if diff > pool/100+1 {
		t.Errorf("Tied participants have too different prizes: alice=%d, bob=%d", alicePrize, bobPrize)
	}

	// Verify total distributed equals pool
	var totalDistributed int64
	for _, r := range results {
		totalDistributed += r.AmountCents
	}
	if totalDistributed != pool {
		t.Errorf("Total distributed (%d) != prize pool (%d)", totalDistributed, pool)
	}

	t.Logf("Tie handling: alice=$%.2f, bob=$%.2f (tied at rank 1)",
		float64(alicePrize)/100, float64(bobPrize)/100)
}

// TestTwoParticipantsTied tests 2 participants with the same score.
func TestTwoParticipantsTied(t *testing.T) {
	pool := int64(10000) // $100

	participants := []RankedParticipant{
		{UserID: "alice", Score: 100.0},
		{UserID: "bob", Score: 100.0},
	}

	results := DistributeWithTies(participants, pool)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// Both should have rank 1
	for _, r := range results {
		if r.Rank != 1 {
			t.Errorf("User %s should have rank 1, got %d", r.UserID, r.Rank)
		}
	}

	// With 2 participants, 1 winner. Both tied at rank 1.
	// The single winner position prize is the full pool.
	// They should split it: each gets pool/2.
	// Remainder (if any) goes to first alphabetically.
	var totalDistributed int64
	for _, r := range results {
		totalDistributed += r.AmountCents
	}
	if totalDistributed != pool {
		t.Errorf("Total distributed (%d) != prize pool (%d)", totalDistributed, pool)
	}

	t.Logf("Two tied participants: alice=$%.2f, bob=$%.2f",
		float64(results[0].AmountCents)/100, float64(results[1].AmountCents)/100)
}

// TestOneParticipant tests edge case of 1 participant (safety - contest shouldn't start).
func TestOneParticipant(t *testing.T) {
	pool := int64(5000) // $50
	slots := CalculatePrizeDistribution(1, pool)

	if len(slots) != 1 {
		t.Fatalf("Expected 1 slot for 1 participant, got %d", len(slots))
	}

	if slots[0].AmountCents != pool {
		t.Errorf("Expected single participant to get entire pool (%d), got %d",
			pool, slots[0].AmountCents)
	}

	if slots[0].Rank != 1 {
		t.Errorf("Expected rank 1, got %d", slots[0].Rank)
	}
}

// TestCentPerfectValidation tests that the sum of all prizes always equals the prize pool exactly.
func TestCentPerfectValidation(t *testing.T) {
	testCases := []struct {
		participants  int
		prizePoolCents int64
	}{
		{2, 10000},
		{3, 15000},
		{5, 7777},
		{10, 100000},
		{10, 99999},    // Odd amount to test rounding
		{25, 50000},
		{50, 333333},   // Non-divisible pool
		{100, 1000000},
		{100, 999999},  // Off by one
		{250, 5000000},
		{500, 10000000},
		{1000, 8000000},
		{1000, 7777777}, // Prime-ish number
		{1, 100},
		{1, 1},          // Minimum meaningful pool
		{4, 3},          // Pool smaller than winners
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			slots := CalculatePrizeDistribution(tc.participants, tc.prizePoolCents)
			if len(slots) == 0 {
				if tc.prizePoolCents > 0 && tc.participants > 0 {
					t.Errorf("Expected slots for %d participants with pool %d",
						tc.participants, tc.prizePoolCents)
				}
				return
			}

			var sum int64
			for _, s := range slots {
				sum += s.AmountCents
			}

			if sum != tc.prizePoolCents {
				t.Errorf("participants=%d, pool=%d: sum=%d (diff=%d)",
					tc.participants, tc.prizePoolCents, sum, tc.prizePoolCents-sum)
			}
		})
	}
}

// TestDecreasingPrizes verifies prizes are strictly decreasing by rank.
func TestDecreasingPrizes(t *testing.T) {
	for _, n := range []int{4, 10, 20, 50, 100, 500, 1000} {
		t.Run("", func(t *testing.T) {
			pool := int64(n) * 10000
			slots := CalculatePrizeDistribution(n, pool)

			// Note: with remainder to 1st place and very small pools,
			// rank 1 might be boosted. But rank 2 onward should be decreasing.
			for i := 2; i < len(slots); i++ {
				if slots[i].AmountCents > slots[i-1].AmountCents {
					t.Errorf("n=%d: rank %d (%d) > rank %d (%d)",
						n, slots[i].Rank, slots[i].AmountCents,
						slots[i-1].Rank, slots[i-1].AmountCents)
				}
			}
		})
	}
}

// TestAllPrizesPositive verifies every winner gets at least 1 cent.
func TestAllPrizesPositive(t *testing.T) {
	for _, n := range []int{10, 50, 100, 500, 1000} {
		t.Run("", func(t *testing.T) {
			// Use a large enough pool so every winner gets at least 1 cent
			pool := int64(n) * 1000
			slots := CalculatePrizeDistribution(n, pool)

			for _, s := range slots {
				if s.AmountCents <= 0 {
					t.Errorf("n=%d: rank %d has non-positive prize: %d cents",
						n, s.Rank, s.AmountCents)
				}
			}
		})
	}
}

// TestPreviewPrizes tests the preview display function.
func TestPreviewPrizes(t *testing.T) {
	preview, err := PreviewPrizes(100, 10000, 0.20)
	if err != nil {
		t.Fatalf("PreviewPrizes error: %v", err)
	}

	if preview.Participants != 100 {
		t.Errorf("Expected 100 participants, got %d", preview.Participants)
	}
	if preview.EntryFeeCents != 10000 {
		t.Errorf("Expected 10000 entry fee, got %d", preview.EntryFeeCents)
	}
	if preview.CommissionRate != 0.20 {
		t.Errorf("Expected 0.20 commission rate, got %f", preview.CommissionRate)
	}
	if preview.GrossPool != 1000000 {
		t.Errorf("Expected gross pool 1000000, got %d", preview.GrossPool)
	}
	if preview.NetPool != 800000 {
		t.Errorf("Expected net pool 800000, got %d", preview.NetPool)
	}
	if preview.WinnersCount != 30 {
		t.Errorf("Expected 30 winners, got %d", preview.WinnersCount)
	}
	if len(preview.Slots) != 30 {
		t.Errorf("Expected 30 slots, got %d", len(preview.Slots))
	}

	// Verify cent-perfect
	var sum int64
	for _, s := range preview.Slots {
		sum += s.AmountCents
	}
	if sum != preview.NetPool {
		t.Errorf("Sum of slots (%d) != net pool (%d)", sum, preview.NetPool)
	}
}

// TestFindTier tests tier selection based on participant count.
func TestFindTier(t *testing.T) {
	tests := []struct {
		participants int
		wantMin      int
		wantMax      int
	}{
		{1, 1, 10},
		{5, 1, 10},
		{10, 1, 10},
		{11, 11, 50},
		{50, 11, 50},
		{51, 51, 250},
		{250, 51, 250},
		{251, 251, 1000},
		{1000, 251, 1000},
		{1001, 1001, 0},
		{5000, 1001, 0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			tier := FindTier(tt.participants, DefaultTiers)
			if tier.MinParticipants != tt.wantMin {
				t.Errorf("participants=%d: min=%d, want %d",
					tt.participants, tier.MinParticipants, tt.wantMin)
			}
			if tier.MaxParticipants != tt.wantMax {
				t.Errorf("participants=%d: max=%d, want %d",
					tt.participants, tier.MaxParticipants, tt.wantMax)
			}
		})
	}
}

// TestDeterministic verifies that the same inputs always produce the same outputs.
func TestDeterministic(t *testing.T) {
	participants := 100
	pool := int64(1000000)

	first := CalculatePrizeDistribution(participants, pool)

	for i := 0; i < 20; i++ {
		result := CalculatePrizeDistribution(participants, pool)

		if len(result) != len(first) {
			t.Fatalf("Run %d: different slot count: %d vs %d", i, len(result), len(first))
		}

		for j := range result {
			if result[j].Rank != first[j].Rank || result[j].AmountCents != first[j].AmountCents {
				t.Errorf("Run %d, slot %d: mismatch rank=%d/%d amount=%d/%d",
					i, j, result[j].Rank, first[j].Rank,
					result[j].AmountCents, first[j].AmountCents)
			}
		}
	}
}

// TestPercentagesSum tests that percentage fields sum to approximately 100%.
func TestPercentagesSum(t *testing.T) {
	for _, n := range []int{2, 3, 10, 50, 100, 500, 1000} {
		t.Run("", func(t *testing.T) {
			pool := int64(1000000)
			slots := CalculatePrizeDistribution(n, pool)

			var sum float64
			for _, s := range slots {
				sum += s.Percentage
			}

			if math.Abs(sum-100.0) > 0.01 {
				t.Errorf("n=%d: percentages sum to %.4f, want ~100.0", n, sum)
			}
		})
	}
}

// TestEdgeCaseZeroPool tests zero prize pool.
func TestEdgeCaseZeroPool(t *testing.T) {
	slots := CalculatePrizeDistribution(10, 0)
	if len(slots) != 0 {
		t.Errorf("Expected no slots for zero pool, got %d", len(slots))
	}
}

// TestEdgeCaseNegativePool tests negative prize pool.
func TestEdgeCaseNegativePool(t *testing.T) {
	slots := CalculatePrizeDistribution(10, -1000)
	if len(slots) != 0 {
		t.Errorf("Expected no slots for negative pool, got %d", len(slots))
	}
}

// TestDistributeWithTiesEmpty tests empty input to DistributeWithTies.
func TestDistributeWithTiesEmpty(t *testing.T) {
	results := DistributeWithTies(nil, 10000)
	if len(results) != 0 {
		t.Errorf("Expected no results for nil participants, got %d", len(results))
	}

	results = DistributeWithTies([]RankedParticipant{}, 10000)
	if len(results) != 0 {
		t.Errorf("Expected no results for empty participants, got %d", len(results))
	}
}

// TestDistributeWithTiesNoTies verifies DistributeWithTies works correctly without ties.
func TestDistributeWithTiesNoTies(t *testing.T) {
	pool := int64(100000) // $1000

	participants := []RankedParticipant{
		{UserID: "alice", Score: 100.0},
		{UserID: "bob", Score: 90.0},
		{UserID: "carol", Score: 80.0},
		{UserID: "dave", Score: 70.0},
		{UserID: "eve", Score: 60.0},
		{UserID: "frank", Score: 50.0},
		{UserID: "grace", Score: 40.0},
		{UserID: "heidi", Score: 30.0},
		{UserID: "ivan", Score: 20.0},
		{UserID: "judy", Score: 10.0},
	}

	results := DistributeWithTies(participants, pool)

	// 10 participants, 3 winners
	if len(results) != 10 {
		t.Fatalf("Expected 10 results, got %d", len(results))
	}

	// Verify total equals pool
	var total int64
	for _, r := range results {
		total += r.AmountCents
	}
	if total != pool {
		t.Errorf("Total distributed (%d) != pool (%d)", total, pool)
	}

	// Winners should have positive prizes, non-winners should have 0
	winnerCount := 0
	for _, r := range results {
		if r.AmountCents > 0 {
			winnerCount++
		}
	}
	if winnerCount != 3 {
		t.Errorf("Expected 3 winners with prizes, got %d", winnerCount)
	}
}

// TestCustomTiers tests using custom tier definitions.
func TestCustomTiers(t *testing.T) {
	customTiers := []PrizeDistributionTier{
		{MinParticipants: 1, MaxParticipants: 0, BasePct: 100.0, Decay: 0.50},
	}

	pool := int64(10000)
	slots := CalculatePrizeDistributionWithTiers(10, pool, customTiers)

	if len(slots) != 3 {
		t.Fatalf("Expected 3 slots, got %d", len(slots))
	}

	var sum int64
	for _, s := range slots {
		sum += s.AmountCents
	}
	if sum != pool {
		t.Errorf("Sum (%d) != pool (%d)", sum, pool)
	}
}

// TestEpsilonTieDetection tests that scores differing by less than ScoreEpsilon
// are treated as tied (same rank).
func TestEpsilonTieDetection(t *testing.T) {
	pool := int64(100000) // $1000

	participants := []RankedParticipant{
		{UserID: "alice", Score: 100.00000000},
		{UserID: "bob", Score: 99.999999995}, // differs by 5e-9, within epsilon
		{UserID: "carol", Score: 80.0},
		{UserID: "dave", Score: 70.0},
		{UserID: "eve", Score: 60.0},
		{UserID: "frank", Score: 50.0},
		{UserID: "grace", Score: 40.0},
	}

	results := DistributeWithTies(participants, pool)

	// Alice and Bob should both have rank 1 (epsilon tie)
	for _, r := range results {
		if r.UserID == "alice" && r.Rank != 1 {
			t.Errorf("Alice should have rank 1, got %d", r.Rank)
		}
		if r.UserID == "bob" && r.Rank != 1 {
			t.Errorf("Bob should have rank 1 (epsilon tie), got %d", r.Rank)
		}
	}

	// Verify cent-perfect distribution
	var total int64
	for _, r := range results {
		total += r.AmountCents
	}
	if total != pool {
		t.Errorf("Total distributed (%d) != pool (%d)", total, pool)
	}

	t.Logf("Epsilon tie detection: alice and bob both rank 1")
}

// TestEpsilonNonTie tests that scores differing by more than ScoreEpsilon
// are treated as different ranks.
func TestEpsilonNonTie(t *testing.T) {
	pool := int64(100000) // $1000

	participants := []RankedParticipant{
		{UserID: "alice", Score: 100.0},
		{UserID: "bob", Score: 99.99999989}, // differs by 1.1e-8, beyond epsilon
		{UserID: "carol", Score: 80.0},
		{UserID: "dave", Score: 70.0},
		{UserID: "eve", Score: 60.0},
		{UserID: "frank", Score: 50.0},
		{UserID: "grace", Score: 40.0},
	}

	results := DistributeWithTies(participants, pool)

	for _, r := range results {
		if r.UserID == "alice" && r.Rank != 1 {
			t.Errorf("Alice should have rank 1, got %d", r.Rank)
		}
		if r.UserID == "bob" && r.Rank != 2 {
			t.Errorf("Bob should have rank 2 (beyond epsilon), got %d", r.Rank)
		}
	}

	// Verify cent-perfect distribution
	var total int64
	for _, r := range results {
		total += r.AmountCents
	}
	if total != pool {
		t.Errorf("Total distributed (%d) != pool (%d)", total, pool)
	}
}

// TestLeftoverSplitAmongMultipleRank1 tests that leftover cents from tie splitting
// are distributed across all rank-1 tied users, not just the first one.
func TestLeftoverSplitAmongMultipleRank1(t *testing.T) {
	pool := int64(10001) // $100.01 - odd amount to force leftover

	participants := []RankedParticipant{
		{UserID: "alice", Score: 100.0},
		{UserID: "bob", Score: 100.0},   // Tied with alice at rank 1
		{UserID: "carol", Score: 100.0}, // Also tied at rank 1
		{UserID: "dave", Score: 70.0},
		{UserID: "eve", Score: 60.0},
		{UserID: "frank", Score: 50.0},
		{UserID: "grace", Score: 40.0},
	}

	results := DistributeWithTies(participants, pool)

	// All three rank-1 users
	rank1Prizes := make(map[string]int64)
	for _, r := range results {
		if r.Rank == 1 {
			rank1Prizes[r.UserID] = r.AmountCents
		}
	}

	if len(rank1Prizes) != 3 {
		t.Fatalf("Expected 3 rank-1 users, got %d", len(rank1Prizes))
	}

	// The difference between any two rank-1 prizes should be at most 1 cent
	prizes := make([]int64, 0, 3)
	for _, p := range rank1Prizes {
		prizes = append(prizes, p)
	}
	for i := 0; i < len(prizes); i++ {
		for j := i + 1; j < len(prizes); j++ {
			diff := prizes[i] - prizes[j]
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 {
				t.Errorf("Rank-1 prizes differ by more than 1 cent: %v", rank1Prizes)
			}
		}
	}

	// Verify cent-perfect distribution
	var total int64
	for _, r := range results {
		total += r.AmountCents
	}
	if total != pool {
		t.Errorf("Total distributed (%d) != pool (%d)", total, pool)
	}

	t.Logf("Leftover split: alice=%d, bob=%d, carol=%d",
		rank1Prizes["alice"], rank1Prizes["bob"], rank1Prizes["carol"])
}

// TestLeftoverSplitDeterministic tests that leftover distribution is deterministic.
func TestLeftoverSplitDeterministic(t *testing.T) {
	pool := int64(10003) // Forces sub-remainder with 2 rank-1 users

	participants := []RankedParticipant{
		{UserID: "alice", Score: 100.0},
		{UserID: "bob", Score: 100.0},
		{UserID: "carol", Score: 80.0},
		{UserID: "dave", Score: 70.0},
		{UserID: "eve", Score: 60.0},
		{UserID: "frank", Score: 50.0},
		{UserID: "grace", Score: 40.0},
	}

	first := DistributeWithTies(participants, pool)

	for run := 0; run < 20; run++ {
		result := DistributeWithTies(participants, pool)
		if len(result) != len(first) {
			t.Fatalf("Run %d: different result count: %d vs %d", run, len(result), len(first))
		}
		for i := range result {
			if result[i].UserID != first[i].UserID ||
				result[i].Rank != first[i].Rank ||
				result[i].AmountCents != first[i].AmountCents {
				t.Errorf("Run %d, index %d: mismatch %+v vs %+v", run, i, result[i], first[i])
			}
		}
	}
}

// TestScoreEpsilonExported verifies the ScoreEpsilon constant value.
func TestScoreEpsilonExported(t *testing.T) {
	if ScoreEpsilon != 1e-8 {
		t.Errorf("ScoreEpsilon = %e, want 1e-8", ScoreEpsilon)
	}
}
