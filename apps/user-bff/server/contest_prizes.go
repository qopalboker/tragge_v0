package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	prizedistribution "github.com/Parsaeffatravesh/tragge/packages/scoring/distribution"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// PrizePreviewResponse is the response for the prize preview endpoint.
type PrizePreviewResponse struct {
	ContestID           string             `json:"contest_id"`
	CurrentParticipants int                `json:"current_participants"`
	MinParticipants     int                `json:"min_participants"`
	QuorumMet           bool               `json:"quorum_met"`
	EntryFeeCents       int                `json:"entry_fee_cents"`
	CommissionRate      float64            `json:"commission_rate"`
	PrizePoolCents      int64              `json:"prize_pool_cents"`
	WinnersCount        int                `json:"winners_count"`
	Prizes              []PrizeRankPreview `json:"prizes"`
	Status              string             `json:"status"`
	Message             string             `json:"message"`
}

// PrizeRankPreview represents a single rank's prize in the preview.
type PrizeRankPreview struct {
	Rank        int     `json:"rank"`
	AmountCents int64   `json:"amount_cents"`
	Percentage  float64 `json:"percentage"`
}

// DefaultPlatformFeeBps is the default platform fee (20% = 2000 bps).
const DefaultPlatformFeeBps = 2000

// ResolveEffectiveFeeBps determines the effective platform fee in basis points
// from the two possibly-set database columns on the contests table.
//
// Priority:
//  1. platform_fee_bps if > 0 (already in bps, most precise)
//  2. commission_rate converted to bps via math.Round(rate * 100)
//  3. DefaultPlatformFeeBps (2000 = 20%)
func ResolveEffectiveFeeBps(platformFeeBps int, commissionRate float64) int {
	if platformFeeBps > 0 {
		return platformFeeBps
	}
	if commissionRate > 0 {
		bps := int(math.Round(commissionRate * 100))
		if bps > 0 && bps <= 10000 {
			return bps
		}
	}
	return DefaultPlatformFeeBps
}

// CalculatePrizeDistribution computes the full prize table for a contest
// using the unified Power Law formula from the shared prizedistribution package.
func CalculatePrizeDistribution(
	participantsCount int,
	entryFeeCents int,
	platformFeeBps int,
) (prizePoolCents int64, winnersCount int, prizes []PrizeRankPreview) {
	if participantsCount <= 0 || entryFeeCents <= 0 {
		return 0, 0, nil
	}

	cfg := prizedistribution.ConfigFromEnv()

	// Net prize pool after commission
	if platformFeeBps <= 0 {
		platformFeeBps = DefaultPlatformFeeBps
	}
	if platformFeeBps >= 10000 {
		return 0, 0, nil
	}
	net := prizedistribution.CalculatePrizePoolBps(participantsCount, int64(entryFeeCents), platformFeeBps)

	// Winners count
	winnersCount = prizedistribution.GetWinnersCount(participantsCount, cfg.WinnerPercent)

	// Calculate distribution using shared Power Law formula
	shares := prizedistribution.CalculatePrizeDistribution(net, winnersCount, cfg.Alpha)
	if len(shares) == 0 {
		return net, winnersCount, nil
	}

	prizes = make([]PrizeRankPreview, len(shares))
	for i, s := range shares {
		prizes[i] = PrizeRankPreview{
			Rank:        s.Rank,
			AmountCents: s.AmountCents,
			Percentage:  math.Round(s.Percentage*100) / 100,
		}
	}

	return net, winnersCount, prizes
}

// handlePrizePreview returns the prize distribution preview for a contest.
func (a *App) handlePrizePreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	contestID := chi.URLParam(r, "id")

	if contestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg.ContestIDRequired})
		return
	}

	// Query contest details
	var status string
	var entryFeeCents int
	var platformFeeBps int
	var minParticipants int
	var commissionRate float64

	err := a.pool.Replica().QueryRowContext(ctx, `
		SELECT status, entry_fee_cents, COALESCE(platform_fee_bps, 0),
		       COALESCE(min_participants, 2), COALESCE(commission_rate, 0)
		FROM contests WHERE id = $1
	`, contestID).Scan(&status, &entryFeeCents, &platformFeeBps, &minParticipants, &commissionRate)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": msg.ContestNotFound})
			return
		}
		a.log().Error("Failed to query contest for prize preview", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Get current participant count
	var currentParticipants int
	err = a.pool.Replica().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1`,
		contestID,
	).Scan(&currentParticipants)
	if err != nil {
		a.log().Error("Failed to count participants", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg.InternalError})
		return
	}

	// Resolve effective fee using unified priority logic
	effectiveFeeBps := ResolveEffectiveFeeBps(platformFeeBps, commissionRate)

	quorumMet := currentParticipants >= minParticipants

	// Calculate prize distribution using shared formula
	prizePoolCents, winnersCount, prizes := CalculatePrizeDistribution(
		currentParticipants, entryFeeCents, effectiveFeeBps,
	)

	message := fmt.Sprintf("Prizes calculated based on %d current participants", currentParticipants)
	if !quorumMet {
		message = fmt.Sprintf("Minimum %d participants required. Currently %d. Auto-refund if quorum not met.", minParticipants, currentParticipants)
	}

	writeJSON(w, http.StatusOK, PrizePreviewResponse{
		ContestID:           contestID,
		CurrentParticipants: currentParticipants,
		MinParticipants:     minParticipants,
		QuorumMet:           quorumMet,
		EntryFeeCents:       entryFeeCents,
		CommissionRate:      commissionRate,
		PrizePoolCents:      prizePoolCents,
		WinnersCount:        winnersCount,
		Prizes:              prizes,
		Status:              status,
		Message:             message,
	})
}

// formatPrizeFarsi formats a prize amount in Rials to a Farsi-readable string in Tomans.
// For example: 80000000 Rials -> "۸ میلیون تومان"
func formatPrizeFarsi(amountCents int64) string {
	// Convert Rials to Tomans (1 Toman = 10 Rials)
	tomans := amountCents / 10

	// Persian digit mapping
	persianDigits := []rune{'۰', '۱', '۲', '۳', '۴', '۵', '۶', '۷', '۸', '۹'}
	toPersian := func(n int64) string {
		if n == 0 {
			return "۰"
		}
		s := fmt.Sprintf("%d", n)
		result := make([]rune, len(s))
		for i, c := range s {
			if c >= '0' && c <= '9' {
				result[i] = persianDigits[c-'0']
			} else {
				result[i] = c
			}
		}
		return string(result)
	}

	switch {
	case tomans >= 1_000_000_000:
		billions := tomans / 1_000_000_000
		remainder := (tomans % 1_000_000_000) / 100_000_000
		if remainder > 0 {
			return fmt.Sprintf("%s.%s میلیارد تومان", toPersian(billions), toPersian(remainder))
		}
		return fmt.Sprintf("%s میلیارد تومان", toPersian(billions))
	case tomans >= 1_000_000:
		millions := tomans / 1_000_000
		remainder := (tomans % 1_000_000) / 100_000
		if remainder > 0 {
			return fmt.Sprintf("%s.%s میلیون تومان", toPersian(millions), toPersian(remainder))
		}
		return fmt.Sprintf("%s میلیون تومان", toPersian(millions))
	case tomans >= 1_000:
		thousands := tomans / 1_000
		return fmt.Sprintf("%s هزار تومان", toPersian(thousands))
	default:
		return fmt.Sprintf("%s تومان", toPersian(tomans))
	}
}

// buildContestUpdatePayload creates the payload for a Redis pub/sub contest update broadcast.
func (a *App) buildContestUpdatePayload(contestID string, event string, currentParticipants int, entryFeeCents int, platformFeeBps int, storedPrizePoolCents int64) ([]byte, error) {
	// Use stored prize pool if available, otherwise calculate
	prizePoolCents, winnersCount, prizes := CalculatePrizeDistribution(
		currentParticipants, entryFeeCents, platformFeeBps,
	)
	if storedPrizePoolCents > 0 {
		prizePoolCents = storedPrizePoolCents
	}

	var firstPrizeCents int64
	type top3Prize struct {
		Rank        int   `json:"rank"`
		AmountCents int64 `json:"amount_cents"`
	}
	var top3 []top3Prize

	for _, p := range prizes {
		if p.Rank == 1 {
			firstPrizeCents = p.AmountCents
		}
		if p.Rank <= 3 {
			top3 = append(top3, top3Prize{Rank: p.Rank, AmountCents: p.AmountCents})
		}
	}

	payload := map[string]interface{}{
		"type":                 "contest_updated",
		"contest_id":           contestID,
		"event":                event,
		"current_participants": currentParticipants,
		"participant_count":    currentParticipants,
		"prize_pool_cents":     prizePoolCents,
		"formatted_prize":      formatPrizeFarsi(prizePoolCents),
		"winners_count":        winnersCount,
		"first_prize_cents":    firstPrizeCents,
		"total_prize_cents":    prizePoolCents,
		"top_3_prizes":         top3,
		"ts":                   time.Now().UnixMilli(),
	}

	return json.Marshal(payload)
}

// publishContestUpdate publishes a contest participant update via Redis pub/sub.
func (a *App) publishContestUpdate(contestID string, event string, entryFeeCents int, platformFeeBps int) {
	if a.redis == nil {
		return
	}

	defer func() {
		if r := recover(); r != nil {
			a.log().Error("Panic in publishContestUpdate",
				zap.Any("panic", r),
				zap.String("contest_id", contestID),
				zap.String("event", event))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Get current participant count and stored prize pool
	var currentParticipants int
	var storedPrizePoolCents int64
	err := a.pool.Primary().QueryRowContext(ctx,
		`SELECT current_participants, COALESCE(prize_pool_net_cents, 0) FROM contests WHERE id = $1`,
		contestID,
	).Scan(&currentParticipants, &storedPrizePoolCents)
	if err != nil {
		a.log().Warn("Failed to read contest data for broadcast",
			zap.Error(err), zap.String("contest_id", contestID))
		return
	}

	data, err := a.buildContestUpdatePayload(contestID, event, currentParticipants, entryFeeCents, platformFeeBps, storedPrizePoolCents)
	if err != nil {
		a.log().Warn("Failed to build contest update payload",
			zap.Error(err), zap.String("contest_id", contestID))
		return
	}

	channel := "contest_updates:" + contestID
	if err := a.redis.Client().Publish(ctx, channel, string(data)).Err(); err != nil {
		a.log().Warn("Failed to publish contest update",
			zap.Error(err), zap.String("contest_id", contestID))
		return
	}

	a.log().Info("Published contest update",
		zap.String("contest_id", contestID),
		zap.String("event", event),
		zap.Int("current_participants", currentParticipants),
		zap.Int64("prize_pool_cents", storedPrizePoolCents))
}
