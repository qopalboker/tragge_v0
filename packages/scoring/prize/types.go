// Package prize implements the Tralent-model prize distribution engine.
// It calculates prize pools and distributes winnings to the top 30% of
// participants using an exponential decay formula, with tier-based
// configuration for different contest sizes.
package prize

// PrizeSlot represents a single prize position in the distribution.
type PrizeSlot struct {
	Rank        int     // 1-indexed rank position
	AmountCents int64   // Prize amount in cents
	Percentage  float64 // Percentage of total prize pool
}

// PrizePreview contains pre-start display information about prize distribution.
type PrizePreview struct {
	Participants   int         // Number of participants
	EntryFeeCents  int         // Entry fee in cents
	CommissionRate float64     // Commission/platform fee rate as fraction (0.0–1.0). Use CommissionPercentToFraction() to convert from percentage.
	GrossPool      int64       // Total collected entry fees in cents
	NetPool        int64       // Prize pool after commission in cents
	WinnersCount   int         // Number of winners (top 30%)
	Slots          []PrizeSlot // Prize breakdown per rank
}

// TiedPrizeSlot represents a prize position for participants who share a rank.
type TiedPrizeSlot struct {
	Rank        int     // Shared rank position
	Count       int     // Number of participants sharing this rank
	TotalCents  int64   // Total prize cents for all tied participants at this rank
	EachCents   int64   // Prize cents each tied participant receives
	Percentage  float64 // Combined percentage of total prize pool
}
