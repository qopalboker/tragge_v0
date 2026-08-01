package prize

import prizedistribution "github.com/Parsaeffatravesh/tragge/packages/scoring/distribution"

// WinnersPercentage is the fraction of participants who receive prizes.
// Delegates to the shared prizedistribution package.
const WinnersPercentage = prizedistribution.DefaultWinnerPercent

// MinWinners is the minimum number of winners for any contest.
const MinWinners = prizedistribution.MinWinners

// PrizeDistributionTier defines the exponential decay parameters for a
// participant count range.
//
// Deprecated: The new Power Law formula in packages/prize-distribution does not
// use tier-based configuration. This type is retained for backward compatibility
// with CalculatePrizeDistributionWithTiers.
type PrizeDistributionTier struct {
	MinParticipants int
	MaxParticipants int
	BasePct         float64
	Decay           float64
}

// DefaultTiers is retained for backward compatibility.
//
// Deprecated: The new Power Law formula does not use tiers.
var DefaultTiers = []PrizeDistributionTier{
	{MinParticipants: 1, MaxParticipants: 10, BasePct: 50.0, Decay: 0.55},
	{MinParticipants: 11, MaxParticipants: 50, BasePct: 35.0, Decay: 0.70},
	{MinParticipants: 51, MaxParticipants: 250, BasePct: 25.0, Decay: 0.80},
	{MinParticipants: 251, MaxParticipants: 1000, BasePct: 18.0, Decay: 0.80},
	{MinParticipants: 1001, MaxParticipants: 0, BasePct: 14.0, Decay: 0.82},
}

// FindTier returns the tier that applies for the given participant count.
//
// Deprecated: The new Power Law formula does not use tiers.
func FindTier(participants int, tiers []PrizeDistributionTier) PrizeDistributionTier {
	for _, t := range tiers {
		if participants >= t.MinParticipants {
			if t.MaxParticipants == 0 || participants <= t.MaxParticipants {
				return t
			}
		}
	}
	if len(tiers) > 0 {
		return tiers[len(tiers)-1]
	}
	return PrizeDistributionTier{
		MinParticipants: 1,
		MaxParticipants: 0,
		BasePct:         50.0,
		Decay:           0.55,
	}
}
