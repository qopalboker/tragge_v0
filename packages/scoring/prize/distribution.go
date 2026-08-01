package prize

import (
	"fmt"
	"math"
	"sort"

	prizedistribution "github.com/Parsaeffatravesh/tragge/packages/scoring/distribution"
)

// MaxCommissionRate is the maximum allowed commission rate as a fraction.
// Commission rates above this value are rejected as misconfiguration.
const MaxCommissionRate = 0.50

// ScoreEpsilon is the tolerance for comparing float64 scores.
// Scores within this threshold are considered equal (tied).
// This matches the 8-decimal-place precision used by the scoring package.
const ScoreEpsilon = 1e-8

// scoresEqual returns true if two scores are within ScoreEpsilon of each other.
func scoresEqual(a, b float64) bool {
	return math.Abs(a-b) <= ScoreEpsilon
}

// GetWinnersCount returns the number of winners for a given participant count.
// Delegates to the shared prizedistribution package.
func GetWinnersCount(participants int) int {
	cfg := prizedistribution.ConfigFromEnv()
	return prizedistribution.GetWinnersCount(participants, cfg.WinnerPercent)
}

// CalculatePrizePool computes the net prize pool in cents after commission.
//   - participants: number of contest participants
//   - entryFeeCents: entry fee per participant in cents
//   - commissionRate: platform commission as a fraction (e.g. 0.20 for 20%)
//
// Returns the prize pool available for distribution in cents.
// Returns an error if commissionRate is negative or exceeds MaxCommissionRate.
func CalculatePrizePool(participants int, entryFeeCents int, commissionRate float64) (int64, error) {
	if participants <= 0 || entryFeeCents <= 0 {
		return 0, nil
	}
	if commissionRate < 0 {
		return 0, fmt.Errorf("prize: commission rate %.4f is negative", commissionRate)
	}
	if commissionRate > MaxCommissionRate {
		return 0, fmt.Errorf("prize: commission rate %.4f exceeds maximum allowed %.4f", commissionRate, MaxCommissionRate)
	}
	gross := int64(participants) * int64(entryFeeCents)
	commission := int64(math.Floor(float64(gross) * commissionRate))
	return gross - commission, nil
}

// CalculatePrizeDistribution computes the prize breakdown for each winner position
// using the unified Power Law formula from the shared prizedistribution package.
//
//   - participants: total number of contest participants
//   - prizePoolCents: total prize pool in cents to distribute
//
// Returns a slice of PrizeSlot ordered by rank (1 = first place).
// The sum of all AmountCents equals exactly prizePoolCents (remainder goes to 1st place).
func CalculatePrizeDistribution(participants int, prizePoolCents int64) []PrizeSlot {
	if participants <= 0 || prizePoolCents <= 0 {
		return nil
	}

	cfg := prizedistribution.ConfigFromEnv()
	winners := prizedistribution.GetWinnersCount(participants, cfg.WinnerPercent)
	if winners <= 0 {
		return nil
	}

	shares := prizedistribution.CalculatePrizeDistribution(prizePoolCents, winners, cfg.Alpha)
	if len(shares) == 0 {
		return nil
	}

	// Convert []PrizeShare → []PrizeSlot
	slots := make([]PrizeSlot, len(shares))
	for i, s := range shares {
		slots[i] = PrizeSlot{
			Rank:        s.Rank,
			AmountCents: s.AmountCents,
			Percentage:  s.Percentage,
		}
	}
	return slots
}

// CalculatePrizeDistributionWithTiers is retained for backward compatibility.
// It ignores the tier parameter and delegates to the unified Power Law formula.
//
// Deprecated: Use CalculatePrizeDistribution instead.
func CalculatePrizeDistributionWithTiers(participants int, prizePoolCents int64, _ []PrizeDistributionTier) []PrizeSlot {
	return CalculatePrizeDistribution(participants, prizePoolCents)
}

// PreviewPrizes generates a PrizePreview for pre-start display.
// This shows prospective participants what the prize breakdown would be.
func PreviewPrizes(participants int, entryFeeCents int, commissionRate float64) (PrizePreview, error) {
	pool, err := CalculatePrizePool(participants, entryFeeCents, commissionRate)
	if err != nil {
		return PrizePreview{}, err
	}
	slots := CalculatePrizeDistribution(participants, pool)
	gross := int64(participants) * int64(entryFeeCents)

	return PrizePreview{
		Participants:   participants,
		EntryFeeCents:  entryFeeCents,
		CommissionRate: commissionRate,
		GrossPool:      gross,
		NetPool:        pool,
		WinnersCount:   GetWinnersCount(participants),
		Slots:          slots,
	}, nil
}

// RankedParticipant represents a participant with their score for tie handling.
type RankedParticipant struct {
	UserID string
	Score  float64
	Rank   int // Assigned after sorting; 1-indexed
}

// DistributeWithTies calculates prize distribution handling tied ranks.
// Participants whose scores differ by less than ScoreEpsilon are considered
// tied. Tied participants share the same rank and split the combined prize
// amounts for the positions they occupy equally.
//
// For example, if ranks 2 and 3 are tied, each receives
// (prize_for_rank_2 + prize_for_rank_3) / 2.
//
// Any leftover cents from integer division in tie splitting are distributed
// evenly across all rank-1 users, with any sub-remainder allocated
// round-robin (1 cent each) to maintain cent-perfect distribution.
//
// The returned slice is sorted by rank ascending, then by UserID for determinism.
func DistributeWithTies(participants []RankedParticipant, prizePoolCents int64) []struct {
	UserID      string
	Rank        int
	AmountCents int64
} {
	if len(participants) == 0 || prizePoolCents <= 0 {
		return nil
	}

	// Sort by score descending, then UserID ascending for determinism
	sorted := make([]RankedParticipant, len(participants))
	copy(sorted, participants)
	sort.Slice(sorted, func(i, j int) bool {
		if !scoresEqual(sorted[i].Score, sorted[j].Score) {
			return sorted[i].Score > sorted[j].Score
		}
		return sorted[i].UserID < sorted[j].UserID
	})

	// Assign ranks with ties (equal scores get the same rank)
	ranks := make([]int, len(sorted))
	ranks[0] = 1
	for i := 1; i < len(sorted); i++ {
		if scoresEqual(sorted[i].Score, sorted[i-1].Score) {
			ranks[i] = ranks[i-1]
		} else {
			ranks[i] = i + 1 // Standard competition ranking (1, 2, 2, 4, ...)
		}
	}

	// Get the base prize distribution (without ties)
	slots := CalculatePrizeDistribution(len(sorted), prizePoolCents)
	if len(slots) == 0 {
		return nil
	}

	winnersCount := len(slots)

	// Build a map from rank position (1-indexed) to prize amount
	positionPrize := make(map[int]int64, winnersCount)
	for _, s := range slots {
		positionPrize[s.Rank] = s.AmountCents
	}

	// Group participants by their tied rank
	type tieGroup struct {
		rank    int
		userIDs []string
	}
	groupMap := make(map[int]*tieGroup)
	var groupOrder []int
	for i, r := range ranks {
		if g, ok := groupMap[r]; ok {
			g.userIDs = append(g.userIDs, sorted[i].UserID)
		} else {
			groupMap[r] = &tieGroup{rank: r, userIDs: []string{sorted[i].UserID}}
			groupOrder = append(groupOrder, r)
		}
	}
	sort.Ints(groupOrder)

	// Calculate prizes for each group
	type userPrize struct {
		UserID      string
		Rank        int
		AmountCents int64
	}
	var results []userPrize
	var totalDistributed int64

	for _, rank := range groupOrder {
		group := groupMap[rank]
		count := len(group.userIDs)

		// Sum up the prizes for all positions this tie group occupies
		var combinedPrize int64
		for pos := 0; pos < count; pos++ {
			position := rank + pos
			if position <= winnersCount {
				combinedPrize += positionPrize[position]
			}
		}

		if combinedPrize <= 0 {
			for _, uid := range group.userIDs {
				results = append(results, userPrize{
					UserID:      uid,
					Rank:        rank,
					AmountCents: 0,
				})
			}
			continue
		}

		// Split equally among tied participants
		eachCents := combinedPrize / int64(count)
		for _, uid := range group.userIDs {
			results = append(results, userPrize{
				UserID:      uid,
				Rank:        rank,
				AmountCents: eachCents,
			})
			totalDistributed += eachCents
		}
	}

	// Distribute leftover cents from tie splitting.
	// Give remainder to the highest-ranked winner(s) for cent-perfect totals.
	leftover := prizePoolCents - totalDistributed
	if leftover > 0 {
		// Find the highest-ranked participants (lowest rank number) that received a prize
		var topIndices []int
		bestRank := int(^uint(0) >> 1) // max int
		for i := range results {
			if results[i].AmountCents > 0 && results[i].Rank < bestRank {
				bestRank = results[i].Rank
				topIndices = []int{i}
			} else if results[i].AmountCents > 0 && results[i].Rank == bestRank {
				topIndices = append(topIndices, i)
			}
		}
		if len(topIndices) > 0 {
			countTop := int64(len(topIndices))
			evenSplit := leftover / countTop
			subRemainder := leftover % countTop

			for _, idx := range topIndices {
				results[idx].AmountCents += evenSplit
			}
			for k := int64(0); k < subRemainder; k++ {
				results[topIndices[k]].AmountCents += 1
			}
		}
	}

	// Convert to return type
	out := make([]struct {
		UserID      string
		Rank        int
		AmountCents int64
	}, len(results))
	for i, r := range results {
		out[i].UserID = r.UserID
		out[i].Rank = r.Rank
		out[i].AmountCents = r.AmountCents
	}

	return out
}
