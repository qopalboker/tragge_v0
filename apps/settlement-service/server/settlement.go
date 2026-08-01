package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	prizedistribution "github.com/Parsaeffatravesh/tragge/packages/scoring/distribution"
	"github.com/Parsaeffatravesh/tragge/packages/domain/traggepoint"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/shopspring/decimal"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// SettlementService handles the complete settlement process for a contest.
type SettlementService struct {
	app *App
}

// NewSettlementService creates a new settlement service.
func NewSettlementService(app *App) *SettlementService {
	return &SettlementService{app: app}
}

// SettleContestWithRetry attempts to settle a contest with retries.
// It acquires a PostgreSQL advisory lock on a pinned database connection,
// holds it across all retry attempts, and releases it when done.
func (s *SettlementService) SettleContestWithRetry(ctx context.Context, contestID string) error {
	// Pin a single DB connection for the advisory lock lifecycle.
	// Advisory locks are session-level in PostgreSQL, so we must ensure
	// lock and unlock happen on the same connection.
	conn, err := s.app.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire db connection for advisory lock: %w", err)
	}
	defer conn.Close()

	lockKey := hashString(contestID)
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&locked); err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	if !locked {
		s.app.log().Info("Settlement already in progress, skipping",
			zap.String("contest_id", contestID))
		return fmt.Errorf("settlement already in progress for contest %s", contestID)
	}
	defer conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, lockKey)

	for attempt := 1; attempt <= s.app.config.MaxRetries; attempt++ {
		s.app.log().Info("Starting settlement attempt",
			zap.String("contest_id", contestID),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", s.app.config.MaxRetries))

		err := s.settleContestAttempt(ctx, contestID)
		if err == nil {
			return nil
		}

		s.app.log().Error("Settlement attempt failed",
			zap.String("contest_id", contestID),
			zap.Int("attempt", attempt),
			zap.Error(err))

		if attempt < s.app.config.MaxRetries {
			delay := s.app.config.RetryBaseDelay * time.Duration(1<<uint(attempt-1))
			jitter := time.Duration(rand.Int63n(int64(delay / 4)))
			delay += jitter
			s.app.log().Info("Waiting before retry",
				zap.String("contest_id", contestID),
				zap.Duration("delay", delay))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// All retries exhausted — mark as failed (lock is still held)
	s.app.log().Error("Settlement failed after all retries",
		zap.String("contest_id", contestID),
		zap.Int("attempts", s.app.config.MaxRetries))

	settlement, err := s.app.getSettlementRecord(ctx, contestID)
	if err == nil && settlement != nil {
		s.app.updateSettlementFailed(ctx, settlement.ID, "max retries exceeded")
		s.publishSettlementFailed(ctx, contestID, settlement.ID, "max retries exceeded", s.app.config.MaxRetries, false)
	}

	return fmt.Errorf("settlement failed after %d attempts", s.app.config.MaxRetries)
}

// settleContestAttempt performs a single settlement attempt for a contest.
// The caller must hold the advisory lock for the contest.
func (s *SettlementService) settleContestAttempt(ctx context.Context, contestID string) error {
	startTime := time.Now()

	// 1. Get or create settlement record (lock managed by caller)
	settlement, err := s.app.getSettlementRecord(ctx, contestID)
	if err != nil {
		return fmt.Errorf("get settlement: %w", err)
	}

	// 2. Check if already completed (idempotency)
	if settlement.Status == "completed" {
		s.app.log().Info("Settlement already completed, skipping",
			zap.String("contest_id", contestID),
			zap.String("settlement_id", settlement.ID))
		return nil
	}

	// 3. Get contest info
	contestInfo, err := s.app.getContestInfo(ctx, contestID)
	if err != nil {
		return fmt.Errorf("get contest info: %w", err)
	}

	// 4. Get participants
	participants, err := s.app.getParticipants(ctx, contestID)
	if err != nil {
		return fmt.Errorf("get participants: %w", err)
	}

	if len(participants) == 0 {
		s.app.log().Info("No participants in contest, marking as completed",
			zap.String("contest_id", contestID))
		s.app.updateSettlementCompleted(ctx, settlement.ID)
		s.app.updateContestStatus(ctx, contestID, "completed")
		return nil
	}

	// 5. Mark settlement as started
	if err := s.app.updateSettlementStarted(ctx, settlement.ID, len(participants)); err != nil {
		return fmt.Errorf("update settlement started: %w", err)
	}

	s.app.logSettlementEvent(ctx, settlement.ID, contestID, "started", map[string]interface{}{
		"total_participants": len(participants),
	}, nil)

	s.publishSettlementStarted(ctx, contestID, settlement.ID, len(participants))

	// 6. Update contest status to settling
	if err := s.app.updateContestStatus(ctx, contestID, "settling"); err != nil {
		s.app.log().Warn("Failed to update contest status to settling", zap.Error(err))
	}

	// 7. Close all positions
	positionsClosed, ordersCancelled, snapshotPrices, err := s.closeAllPositions(ctx, contestID, settlement.ID)
	if err != nil {
		errMsg := err.Error()
		s.app.logSettlementEvent(ctx, settlement.ID, contestID, "positions_close_failed", nil, &errMsg)
		return fmt.Errorf("close positions: %w", err)
	}

	if err := s.app.updateSettlementPositionsClosed(ctx, settlement.ID, positionsClosed, ordersCancelled, snapshotPrices); err != nil {
		return fmt.Errorf("update positions closed: %w", err)
	}

	s.app.logSettlementEvent(ctx, settlement.ID, contestID, "positions_closed", map[string]interface{}{
		"positions_closed": positionsClosed,
		"orders_cancelled": ordersCancelled,
	}, nil)

	s.publishPositionsClosed(ctx, contestID, settlement.ID, positionsClosed, ordersCancelled, snapshotPrices)

	// 8. Refresh participant data after position closes
	participants, err = s.app.getParticipants(ctx, contestID)
	if err != nil {
		return fmt.Errorf("refresh participants: %w", err)
	}

	// 9. Calculate final rankings
	rankings := s.calculateRankings(ctx, contestID, participants)

	if err := s.app.updateSettlementRankingsCalculated(ctx, settlement.ID); err != nil {
		return fmt.Errorf("update rankings calculated: %w", err)
	}

	s.app.logSettlementEvent(ctx, settlement.ID, contestID, "rankings_calculated", map[string]interface{}{
		"total_ranked": len(rankings),
	}, nil)

	s.publishRankingsCalculated(ctx, contestID, settlement.ID, len(participants), rankings)

	// 10. Calculate prizes
	prizePool, prizes := s.calculatePrizes(rankings, contestInfo)

	if err := s.app.updateSettlementPrizesCalculated(ctx, settlement.ID,
		prizePool.Gross, prizePool.Net, prizePool.PlatformFee, len(prizes)); err != nil {
		return fmt.Errorf("update prizes calculated: %w", err)
	}

	s.app.logSettlementEvent(ctx, settlement.ID, contestID, "prizes_calculated", map[string]interface{}{
		"prize_pool_gross": prizePool.Gross,
		"prize_pool_net":   prizePool.Net,
		"platform_fee":     prizePool.PlatformFee,
		"winners_count":    len(prizes),
	}, nil)

	s.publishPrizesCalculated(ctx, contestID, settlement.ID, prizePool, prizes)

	// 11. Distribute prizes and update rankings
	totalDistributed, failedPrizes, err := s.distributePrizes(ctx, contestID, settlement.ID, rankings, prizes)
	if err != nil {
		errMsg := err.Error()
		s.app.logSettlementEvent(ctx, settlement.ID, contestID, "prizes_distribution_failed", nil, &errMsg)
		return fmt.Errorf("distribute prizes: %w", err)
	}

	if err := s.app.updateSettlementPrizesDistributed(ctx, settlement.ID, totalDistributed); err != nil {
		return fmt.Errorf("update prizes distributed: %w", err)
	}

	s.app.logSettlementEvent(ctx, settlement.ID, contestID, "prizes_distributed", map[string]interface{}{
		"total_distributed": totalDistributed,
		"successful":        len(prizes) - len(failedPrizes),
		"failed":            len(failedPrizes),
	}, nil)

	s.publishPrizesDistributed(ctx, contestID, settlement.ID, len(prizes)-len(failedPrizes), len(failedPrizes), totalDistributed, failedPrizes)

	// 12. Update T-Points
	if err := s.updateTraggePoints(ctx, contestID, rankings, len(participants), contestInfo.EndTime); err != nil {
		s.app.log().Warn("Failed to update T-Points", zap.Error(err))
		// Don't fail settlement for T-Point updates
	}

	// 13. Send prize winner notification emails and create in-app notifications
	// Run in background to not block settlement completion
	prizeEmailCtx, cancelEmail := context.WithTimeout(context.Background(), 30*time.Second)
	s.app.wg.Add(1)
	infra.SafeGo(s.app.log(), "prize-winner-emails", func() {
		defer s.app.wg.Done()
		defer cancelEmail()
		s.sendPrizeWinnerEmails(prizeEmailCtx, contestID, contestInfo, prizes, rankings)
	})

	notifCtx, cancelNotif := context.WithTimeout(context.Background(), 10*time.Second)
	s.app.wg.Add(1)
	infra.SafeGo(s.app.log(), "prize-won-notifications", func() {
		defer s.app.wg.Done()
		defer cancelNotif()
		s.createPrizeWonInAppNotifications(notifCtx, contestID, contestInfo, prizes)
	})

	// 13.5. Publish per-user settlement notifications to notifications.v1
	// This enables trade-bff to push real-time WebSocket notifications to users
	s.publishUserSettlementNotifications(ctx, contestID, contestInfo, prizes, rankings, participants)

	// 14. Mark settlement as completed
	if err := s.app.updateSettlementCompleted(ctx, settlement.ID); err != nil {
		return fmt.Errorf("update settlement completed: %w", err)
	}

	// 15. Update contest status to completed
	if err := s.app.updateContestStatus(ctx, contestID, "completed"); err != nil {
		s.app.log().Warn("Failed to update contest status to completed", zap.Error(err))
	}

	// 16. Log completion
	duration := time.Since(startTime)
	s.app.logSettlementEvent(ctx, settlement.ID, contestID, "completed", map[string]interface{}{
		"duration_ms": duration.Milliseconds(),
	}, nil)

	s.publishSettlementCompleted(ctx, contestID, settlement.ID, len(participants), len(prizes),
		totalDistributed, prizePool.PlatformFee, positionsClosed, ordersCancelled, duration)

	s.app.log().Info("Settlement completed successfully",
		zap.String("contest_id", contestID),
		zap.String("settlement_id", settlement.ID),
		zap.Int("participants", len(participants)),
		zap.Int("winners", len(prizes)),
		zap.Int64("distributed", totalDistributed),
		zap.Duration("duration", duration))

	return nil
}

// closeAllPositions cancels pending orders first, waits for them to clear, then
// closes open positions. This ordering prevents a pending order from triggering
// after position closure starts, which would create an orphan position.
func (s *SettlementService) closeAllPositions(ctx context.Context, contestID, settlementID string) (int, int, map[string]contracts.Price, error) {
	cancelReq := contracts.CancelAllOrdersRequest{
		ContestID: contestID,
		Reason:    contracts.CancelReasonContestEnded,
		Ts:        time.Now().UnixMilli(),
	}

	closeReq := contracts.ClosePositionsRequest{
		ContestID: contestID,
		Reason:    "contest_settlement",
		Ts:        time.Now().UnixMilli(),
	}

	snapshotPrices := make(map[string]contracts.Price)
	const maxAttempts = 3

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Read counts at the start of each attempt (they may change between attempts)
		initialPositions, err := s.app.getOpenPositionsCount(ctx, contestID)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("get open positions count: %w", err)
		}
		initialOrders, err := s.app.getPendingOrdersCount(ctx, contestID)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("get pending orders count: %w", err)
		}

		// --- Step 1: Cancel pending orders FIRST ---
		cancelJSON, _ := json.Marshal(cancelReq)
		doneCh := make(chan struct{})
		s.app.kafka.Produce(ctx, &kgo.Record{
			Topic: s.app.config.CancelOrdersTopic,
			Key:   []byte(contestID),
			Value: cancelJSON,
		}, func(_ *kgo.Record, _ error) { close(doneCh) })

		select {
		case <-doneCh:
		case <-time.After(10 * time.Second):
			s.app.log().Warn("Timeout waiting for cancel produce ack",
				zap.String("contest_id", contestID))
		case <-ctx.Done():
			return 0, 0, nil, ctx.Err()
		}

		// --- Step 2: Wait for orders to cancel ---
		cancelDeadline := time.After(15 * time.Second)
		cancelTicker := time.NewTicker(500 * time.Millisecond)

	waitCancel:
		for {
			select {
			case <-cancelDeadline:
				s.app.log().Warn("Timeout waiting for orders to cancel, proceeding to close positions",
					zap.String("contest_id", contestID),
					zap.Int("attempt", attempt))
				break waitCancel
			case <-cancelTicker.C:
				pending, err := s.app.getPendingOrdersCount(ctx, contestID)
				if err != nil {
					s.app.log().Warn("Failed to check pending orders count during cancel wait",
						zap.String("contest_id", contestID),
						zap.Error(err))
				} else if pending == 0 {
					break waitCancel
				}
			case <-ctx.Done():
				cancelTicker.Stop()
				return 0, 0, nil, ctx.Err()
			}
		}
		cancelTicker.Stop()

		// --- Step 3: Close positions (safe — no pending orders) ---
		closeJSON, _ := json.Marshal(closeReq)
		closeDoneCh := make(chan struct{})
		s.app.kafka.Produce(ctx, &kgo.Record{
			Topic: s.app.config.ClosePositionsTopic,
			Key:   []byte(contestID),
			Value: closeJSON,
		}, func(_ *kgo.Record, _ error) { close(closeDoneCh) })

		select {
		case <-closeDoneCh:
		case <-time.After(10 * time.Second):
			s.app.log().Warn("Timeout waiting for close positions produce ack",
				zap.String("contest_id", contestID))
		case <-ctx.Done():
			return 0, 0, nil, ctx.Err()
		}

		// --- Step 4: Wait for positions to close ---
		pollInterval := time.Duration(attempt) * time.Second
		posTicker := time.NewTicker(pollInterval)
		timeout := time.After(s.app.config.PositionCloseTimeout)

	waitPositions:
		for {
			select {
			case <-timeout:
				posTicker.Stop()
				s.app.log().Warn("Timeout waiting for positions to close",
					zap.String("contest_id", contestID),
					zap.Int("attempt", attempt),
					zap.Int("max_attempts", maxAttempts))
				break waitPositions

			case <-posTicker.C:
				openPositions, err := s.app.getOpenPositionsCount(ctx, contestID)
				if err != nil {
					s.app.log().Warn("Failed to check open positions count during close wait",
						zap.String("contest_id", contestID),
						zap.Error(err))
					continue
				}
				pendingOrders, err := s.app.getPendingOrdersCount(ctx, contestID)
				if err != nil {
					s.app.log().Warn("Failed to check pending orders count during close wait",
						zap.String("contest_id", contestID),
						zap.Error(err))
					continue
				}

				if openPositions == 0 && pendingOrders == 0 {
					posTicker.Stop()

					// Compute actual counts after confirmed closure
					positionsClosed := initialPositions - openPositions
					ordersCancelled := initialOrders - pendingOrders

					// Snapshot current market prices from Redis
					snapshotPrices = s.snapshotPricesFromRedis(ctx, contestID)

					s.app.log().Info("All positions closed and orders cancelled",
						zap.String("contest_id", contestID),
						zap.Int("positions_closed", positionsClosed),
						zap.Int("orders_cancelled", ordersCancelled),
						zap.Int("snapshot_symbols", len(snapshotPrices)))
					return positionsClosed, ordersCancelled, snapshotPrices, nil
				}

			case <-ctx.Done():
				posTicker.Stop()
				return 0, 0, nil, ctx.Err()
			}
		}

		if attempt < maxAttempts {
			s.app.log().Info("Retrying cancel-then-close sequence",
				zap.String("contest_id", contestID),
				zap.Int("next_attempt", attempt+1))
		}
	}

	return 0, 0, nil, fmt.Errorf("timeout: positions/orders not closed after %d attempts of %v each for contest %s",
		maxAttempts, s.app.config.PositionCloseTimeout, contestID)
}

// snapshotPricesFromRedis fetches the latest market prices for all symbols
// that had positions in the given contest from Redis "prices:latest" hash.
func (s *SettlementService) snapshotPricesFromRedis(ctx context.Context, contestID string) map[string]contracts.Price {
	symbols, err := s.app.getContestSymbols(ctx, contestID)
	if err != nil || len(symbols) == 0 {
		if err != nil {
			s.app.log().Warn("Failed to get contest symbols for price snapshot",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
		return make(map[string]contracts.Price)
	}

	prices := make(map[string]contracts.Price, len(symbols))

	// Use HMGET for a single round-trip to Redis
	result, err := s.app.redis.HMGet(ctx, "prices:latest", symbols...).Result()
	if err != nil {
		s.app.log().Warn("Failed to fetch snapshot prices from Redis",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return prices
	}

	for i, val := range result {
		if val == nil {
			continue
		}
		strVal, ok := val.(string)
		if !ok {
			continue
		}

		var tick struct {
			Last float64 `json:"last"`
			Bid  float64 `json:"bid,omitempty"`
			Ask  float64 `json:"ask,omitempty"`
		}
		if err := json.Unmarshal([]byte(strVal), &tick); err != nil {
			s.app.log().Warn("Failed to unmarshal price data",
				zap.String("symbol", symbols[i]),
				zap.Error(err))
			continue
		}

		prices[symbols[i]] = contracts.Price{
			Bid:  tick.Bid,
			Ask:  tick.Ask,
			Last: tick.Last,
		}
	}

	return prices
}

// calculateRankings calculates final rankings with tie handling.
func (s *SettlementService) calculateRankings(ctx context.Context, contestID string, participants []Participant) []contracts.FinalRanking {
	// Sort by final score descending
	sort.Slice(participants, func(i, j int) bool {
		return participants[i].FinalScore > participants[j].FinalScore
	})

	rankings := make([]contracts.FinalRanking, 0, len(participants))
	currentRank := 1
	prevScore := math.MaxFloat64

	for i, p := range participants {
		// Get trade stats
		totalTrades, winningTrades, tradeStatsErr := s.app.getUserTradeStats(ctx, contestID, p.UserID)
		if tradeStatsErr != nil {
			s.app.log().Warn("Failed to get trade stats for user, using zero values",
				zap.String("contest_id", contestID),
				zap.String("user_id", p.UserID),
				zap.Error(tradeStatsErr))
		}

		// Calculate win rate
		var winRate float64
		if totalTrades > 0 {
			winRate = (float64(winningTrades) / float64(totalTrades)) * 100
		}

		// Handle ties: same score = same rank
		if p.FinalScore < prevScore {
			currentRank = i + 1
		}

		// Count ties
		tiedCount := 0
		for j := range participants {
			if j != i && participants[j].FinalScore == p.FinalScore {
				tiedCount++
			}
		}

		ranking := contracts.FinalRanking{
			UserID:        p.UserID,
			Rank:          currentRank,
			TiedWithCount: tiedCount,
			FinalScore:    p.FinalScore,
			RealizedScore: p.RealizedScore,
			TotalTrades:   totalTrades,
			WinningTrades: winningTrades,
			WinRate:       winRate,
		}

		rankings = append(rankings, ranking)
		prevScore = p.FinalScore
	}

	return rankings
}

// PrizePool holds prize pool calculations.
type PrizePool struct {
	Gross       int64
	Net         int64
	PlatformFee int64
}

// calculatePrizes calculates prize allocations for winners using the unified
// Power Law formula from the shared prizedistribution package.
func (s *SettlementService) calculatePrizes(rankings []contracts.FinalRanking, contestInfo *ContestInfo) (PrizePool, []contracts.PrizeAllocation) {
	if len(rankings) == 0 || contestInfo.EntryFeeCents <= 0 {
		return PrizePool{}, nil
	}

	participantsCount := len(rankings)
	prizePoolGross := int64(participantsCount) * contestInfo.EntryFeeCents

	var prizePoolNet int64
	var platformFee int64

	// Use stored prize pool from join-time accumulation (authoritative source).
	// Only fall back to recalculation if stored values are missing (legacy contests).
	if contestInfo.PrizePoolNetCents > 0 {
		prizePoolNet = contestInfo.PrizePoolNetCents
		platformFee = contestInfo.CommissionAmount
		// Sanity check: stored pool should be <= gross
		if prizePoolNet > prizePoolGross {
			s.app.log().Warn("Stored prize pool exceeds calculated gross, falling back to calculation",
				zap.Int64("stored_net", prizePoolNet),
				zap.Int64("calculated_gross", prizePoolGross))
			prizePoolNet = 0 // Force recalculation
		}
	}

	if prizePoolNet <= 0 {
		// Fallback: recalculate (legacy contests without stored pool)
		platformFeeBps := contestInfo.PlatformFeeBps
		if platformFeeBps == 0 {
			platformFeeBps = s.app.config.PlatformFeeBps
		}
		platformFee = (prizePoolGross * int64(platformFeeBps)) / 10000
		prizePoolNet = prizePoolGross - platformFee
	}

	pool := PrizePool{
		Gross:       prizePoolGross,
		Net:         prizePoolNet,
		PlatformFee: platformFee,
	}

	// Use shared formula for winners count and distribution
	cfg := prizedistribution.ConfigFromEnv()
	winnersCount := prizedistribution.GetWinnersCount(participantsCount, cfg.WinnerPercent)
	if winnersCount == 0 {
		winnersCount = 1
	}

	// Calculate distribution using shared Power Law formula
	shares := prizedistribution.CalculatePrizeDistribution(prizePoolNet, winnersCount, cfg.Alpha)
	if len(shares) == 0 {
		return pool, nil
	}

	// Map shares to rankings (by rank)
	shareByRank := make(map[int]prizedistribution.PrizeShare, len(shares))
	for _, s := range shares {
		shareByRank[s.Rank] = s
	}

	prizes := make([]contracts.PrizeAllocation, 0, len(shares))
	for i := 0; i < len(rankings) && i < winnersCount; i++ {
		ranking := rankings[i]
		share, ok := shareByRank[ranking.Rank]
		if !ok {
			// For tied ranks, use the rank's share if available
			share, ok = shareByRank[i+1]
			if !ok {
				continue
			}
		}

		if share.AmountCents <= 0 {
			continue
		}

		prizes = append(prizes, contracts.PrizeAllocation{
			UserID:      ranking.UserID,
			Rank:        ranking.Rank,
			AmountCents: share.AmountCents,
			Percentage:  share.Percentage,
			Status:      "pending",
		})
	}

	return pool, prizes
}

// distributePrizes distributes prizes to winners' wallets.
// Phase 1: Bulk insert rankings and update participants in a single transaction.
// Phase 2: Credit each prize in its own transaction for partial-progress resilience.
func (s *SettlementService) distributePrizes(ctx context.Context, contestID, settlementID string, rankings []contracts.FinalRanking, prizes []contracts.PrizeAllocation) (int64, []contracts.PrizeAllocation, error) {
	// Phase 1: Insert rankings and update participants (bulk, idempotent via ON CONFLICT)
	prizeMap := make(map[string]contracts.PrizeAllocation)
	for _, prize := range prizes {
		prizeMap[prize.UserID] = prize
	}

	rankTx, err := s.app.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("begin rankings transaction: %w", err)
	}
	defer rankTx.Rollback()

	for _, ranking := range rankings {
		if err := s.app.insertFinalRanking(ctx, rankTx, settlementID, contestID, ranking); err != nil {
			return 0, nil, fmt.Errorf("insert final ranking for user %s: %w", ranking.UserID, err)
		}

		prizeCents := int64(0)
		if prize, ok := prizeMap[ranking.UserID]; ok {
			prizeCents = prize.AmountCents
		}

		if err := s.app.updateParticipantFinalRank(ctx, rankTx, contestID, ranking.UserID, ranking.Rank, prizeCents, ranking.FinalScore); err != nil {
			return 0, nil, fmt.Errorf("update participant final rank for user %s: %w", ranking.UserID, err)
		}
	}

	if err := rankTx.Commit(); err != nil {
		return 0, nil, fmt.Errorf("commit rankings transaction: %w", err)
	}

	// Phase 2: Credit each prize in its own transaction for partial-progress resilience
	var totalDistributed int64
	failedPrizes := make([]contracts.PrizeAllocation, 0)

	for i := range prizes {
		prize := &prizes[i]

		tx, err := s.app.db.BeginTx(ctx, nil)
		if err != nil {
			s.app.log().Error("Failed to begin prize transaction",
				zap.String("user_id", prize.UserID),
				zap.Error(err))
			prize.Status = "failed"
			errMsg := err.Error()
			prize.ErrorMessage = errMsg
			failedPrizes = append(failedPrizes, *prize)
			s.app.metrics.PrizesDistributed.WithLabelValues("failed").Inc()
			continue
		}

		// Insert prize distribution record
		if err := s.app.insertPrizeDistribution(ctx, tx, settlementID, contestID, *prize); err != nil {
			s.app.log().Warn("Failed to insert prize distribution",
				zap.String("user_id", prize.UserID),
				zap.Error(err))
		}

		// Credit wallet with idempotency protection
		ledgerEntry, err := s.app.wallet.CreditPrizeIdempotent(ctx, tx, prize.UserID, contestID, prize.Rank, prize.AmountCents)
		if err != nil {
			var dupErr *wallet.DuplicatePrizeCreditError
			if errors.As(err, &dupErr) {
				s.app.log().Info("Prize already credited (idempotent skip)",
					zap.String("user_id", prize.UserID),
					zap.Int("rank", prize.Rank),
					zap.String("idempotency_key", dupErr.IdempotencyKey))
				ledgerEntry = dupErr.ExistingEntry
			} else {
				s.app.log().Warn("Failed to credit prize to wallet",
					zap.String("user_id", prize.UserID),
					zap.Int64("amount", prize.AmountCents),
					zap.Error(err))

				prize.Status = "failed"
				errMsg := err.Error()
				prize.ErrorMessage = errMsg
				s.app.updatePrizeDistributionStatus(ctx, tx, contestID, prize.UserID, "failed", nil, &errMsg)
				tx.Commit() // Commit the failed status update
				failedPrizes = append(failedPrizes, *prize)
				s.app.metrics.PrizesDistributed.WithLabelValues("failed").Inc()
				continue
			}
		}

		prize.Status = "credited"
		if ledgerEntry != nil {
			prize.LedgerEntryID = ledgerEntry.ID
		}
		s.app.updatePrizeDistributionStatus(ctx, tx, contestID, prize.UserID, "credited", &prize.LedgerEntryID, nil)

		if err := tx.Commit(); err != nil {
			s.app.log().Error("Failed to commit prize transaction",
				zap.String("user_id", prize.UserID),
				zap.Error(err))
			prize.Status = "failed"
			errMsg := err.Error()
			prize.ErrorMessage = errMsg
			failedPrizes = append(failedPrizes, *prize)
			s.app.metrics.PrizesDistributed.WithLabelValues("failed").Inc()
			continue
		}

		totalDistributed += prize.AmountCents
		s.app.metrics.PrizesDistributed.WithLabelValues("credited").Inc()
		s.app.metrics.PrizesTotalAmount.Add(float64(prize.AmountCents))

		s.app.log().Info("Credited prize to wallet",
			zap.String("user_id", prize.UserID),
			zap.Int("rank", prize.Rank),
			zap.Int64("amount", prize.AmountCents))
	}

	return totalDistributed, failedPrizes, nil
}

// updateTraggePoints updates T-Points for all participants.
func (s *SettlementService) updateTraggePoints(ctx context.Context, contestID string, rankings []contracts.FinalRanking, totalParticipants int, contestEndTime time.Time) error {
	tx, err := s.app.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	for i := range rankings {
		ranking := &rankings[i]

		// Calculate T-Point contribution
		contributionDec := traggepoint.CalculateContribution(
			decimal.NewFromFloat(ranking.FinalScore),
			ranking.Rank,
			totalParticipants,
			contestEndTime,
		)
		contribution := traggepoint.ToFloat64(contributionDec)
		ranking.TraggePointContribution = contribution

		if contribution <= 0 {
			continue
		}

		// Atomically increment T-Point to avoid lost-update races
		newScore, err := s.app.incrementUserTraggePoint(ctx, tx, ranking.UserID, contribution)
		if err != nil {
			s.app.log().Warn("Failed to update T-Point",
				zap.String("user_id", ranking.UserID),
				zap.Error(err))
			continue
		}

		// Publish T-Point update event
		s.publishTraggePointUpdate(ctx, ranking.UserID, contestID, contribution, newScore)
	}

	return tx.Commit()
}

// Event publishing methods

func (s *SettlementService) publishSettlementStarted(ctx context.Context, contestID, settlementID string, totalParticipants int) {
	event := contracts.SettlementStartedEvent{
		ContestID:         contestID,
		SettlementID:      settlementID,
		TotalParticipants: totalParticipants,
		Ts:                time.Now().UnixMilli(),
	}
	s.publishEvent(ctx, "settlement_started", event)
}

func (s *SettlementService) publishPositionsClosed(ctx context.Context, contestID, settlementID string, positionsClosed, ordersCancelled int, snapshotPrices map[string]contracts.Price) {
	event := contracts.PositionsClosedEvent{
		ContestID:       contestID,
		SettlementID:    settlementID,
		PositionsClosed: positionsClosed,
		OrdersCancelled: ordersCancelled,
		SnapshotPrices:  snapshotPrices,
		Ts:              time.Now().UnixMilli(),
	}
	s.publishEvent(ctx, "positions_closed", event)
}

func (s *SettlementService) publishRankingsCalculated(ctx context.Context, contestID, settlementID string, totalParticipants int, rankings []contracts.FinalRanking) {
	// Only include top 100 in the event
	topRankings := rankings
	if len(topRankings) > 100 {
		topRankings = topRankings[:100]
	}

	event := contracts.RankingsCalculatedEvent{
		ContestID:         contestID,
		SettlementID:      settlementID,
		TotalParticipants: totalParticipants,
		Rankings:          topRankings,
		Ts:                time.Now().UnixMilli(),
	}
	s.publishEvent(ctx, "rankings_calculated", event)
}

func (s *SettlementService) publishPrizesCalculated(ctx context.Context, contestID, settlementID string, pool PrizePool, prizes []contracts.PrizeAllocation) {
	event := contracts.PrizesCalculatedEvent{
		ContestID:      contestID,
		SettlementID:   settlementID,
		PrizePoolGross: pool.Gross,
		PrizePoolNet:   pool.Net,
		PlatformFee:    pool.PlatformFee,
		WinnersCount:   len(prizes),
		Allocations:    prizes,
		Ts:             time.Now().UnixMilli(),
	}
	s.publishEvent(ctx, "prizes_calculated", event)
}

func (s *SettlementService) publishPrizesDistributed(ctx context.Context, contestID, settlementID string, successfulCredits, failedCredits int, totalDistributed int64, failedAllocations []contracts.PrizeAllocation) {
	event := contracts.PrizesDistributedEvent{
		ContestID:         contestID,
		SettlementID:      settlementID,
		SuccessfulCredits: successfulCredits,
		FailedCredits:     failedCredits,
		TotalDistributed:  totalDistributed,
		FailedAllocations: failedAllocations,
		Ts:                time.Now().UnixMilli(),
	}
	s.publishEvent(ctx, "prizes_distributed", event)
}

func (s *SettlementService) publishSettlementCompleted(ctx context.Context, contestID, settlementID string, totalParticipants, totalWinners int, totalDistributed, platformFee int64, positionsClosed, ordersCancelled int, duration time.Duration) {
	event := contracts.SettlementCompletedEvent{
		ContestID:         contestID,
		SettlementID:      settlementID,
		TotalParticipants: totalParticipants,
		TotalWinners:      totalWinners,
		TotalDistributed:  totalDistributed,
		PlatformFee:       platformFee,
		PositionsClosed:   positionsClosed,
		OrdersCancelled:   ordersCancelled,
		DurationMs:        duration.Milliseconds(),
		Ts:                time.Now().UnixMilli(),
	}
	s.publishEvent(ctx, "settlement_completed", event)
}

func (s *SettlementService) publishSettlementFailed(ctx context.Context, contestID, settlementID, errorMessage string, attemptCount int, willRetry bool) {
	event := contracts.SettlementFailedEvent{
		ContestID:    contestID,
		SettlementID: settlementID,
		Status:       contracts.SettlementStatusFailed,
		ErrorMessage: errorMessage,
		AttemptCount: attemptCount,
		WillRetry:    willRetry,
		Ts:           time.Now().UnixMilli(),
	}
	s.publishEvent(ctx, "settlement_failed", event)
}

func (s *SettlementService) publishTraggePointUpdate(ctx context.Context, userID, contestID string, contribution, newTotal float64) {
	event := contracts.TraggePointUpdateEvent{
		UserID:              userID,
		ContestID:           contestID,
		PointContribution:   contribution,
		NewTotalTraggePoint: newTotal,
		Ts:                  time.Now().UnixMilli(),
	}
	s.publishEvent(ctx, "tragge_point_update", event)
}

func (s *SettlementService) publishEvent(ctx context.Context, eventType string, event interface{}) {
	data, err := json.Marshal(event)
	if err != nil {
		s.app.log().Warn("Failed to marshal event", zap.String("type", eventType), zap.Error(err))
		return
	}

	record := &kgo.Record{
		Topic: s.app.config.SettlementEventsTopic,
		Key:   []byte(eventType),
		Value: data,
	}

	s.app.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			s.app.log().Warn("Failed to publish event", zap.String("type", eventType), zap.Error(err))
		}
	})
}

// publishUserSettlementNotifications publishes per-user notifications to
// notifications.v1 so that trade-bff can push them via WebSocket.
// Winners get a "prize_won" notification and all participants get a "contest_result" notification.
func (s *SettlementService) publishUserSettlementNotifications(ctx context.Context, contestID string, contestInfo *ContestInfo, prizes []contracts.PrizeAllocation, rankings []contracts.FinalRanking, participants []Participant) {
	if s.app.config.NotificationsTopic == "" {
		return
	}

	// Build a ranking map for quick lookup
	rankByUser := make(map[string]contracts.FinalRanking, len(rankings))
	for _, r := range rankings {
		rankByUser[r.UserID] = r
	}

	// Build a prize map for quick lookup
	prizeByUser := make(map[string]contracts.PrizeAllocation, len(prizes))
	for _, p := range prizes {
		prizeByUser[p.UserID] = p
	}

	var records []*kgo.Record

	// Prize notifications for winners
	for _, prize := range prizes {
		notif := contracts.SettlementNotification{
			UserID:      prize.UserID,
			Type:        "prize_won",
			ContestID:   contestID,
			ContestName: contestInfo.Name,
			Data: map[string]interface{}{
				"rank":               prize.Rank,
				"amount_cents":       prize.AmountCents,
				"total_winners":      len(prizes),
				"total_participants": len(participants),
			},
			Ts: time.Now().UnixMilli(),
		}

		data, err := json.Marshal(notif)
		if err != nil {
			s.app.log().Warn("Failed to marshal prize_won notification", zap.Error(err))
			continue
		}

		records = append(records, &kgo.Record{
			Topic: s.app.config.NotificationsTopic,
			Key:   []byte(prize.UserID),
			Value: data,
		})
	}

	// Contest result notifications for all participants
	for _, participant := range participants {
		ranking, hasRank := rankByUser[participant.UserID]
		userRank := 0
		finalScore := participant.FinalScore
		if hasRank {
			userRank = ranking.Rank
			finalScore = ranking.FinalScore
		}

		notif := contracts.SettlementNotification{
			UserID:      participant.UserID,
			Type:        "contest_result",
			ContestID:   contestID,
			ContestName: contestInfo.Name,
			Data: map[string]interface{}{
				"rank":               userRank,
				"total_participants": len(participants),
				"final_score":        finalScore,
			},
			Ts: time.Now().UnixMilli(),
		}

		data, err := json.Marshal(notif)
		if err != nil {
			s.app.log().Warn("Failed to marshal contest_result notification", zap.Error(err))
			continue
		}

		records = append(records, &kgo.Record{
			Topic: s.app.config.NotificationsTopic,
			Key:   []byte(participant.UserID),
			Value: data,
		})
	}

	// Produce all notifications
	for _, record := range records {
		s.app.kafka.Produce(ctx, record, func(r *kgo.Record, err error) {
			if err != nil {
				s.app.log().Warn("Failed to publish settlement notification",
					zap.String("topic", s.app.config.NotificationsTopic),
					zap.Error(err))
			}
		})
	}

	s.app.log().Info("Published settlement notifications",
		zap.String("contest_id", contestID),
		zap.Int("prize_won", len(prizes)),
		zap.Int("contest_result", len(participants)))
}

// sendPrizeWinnerEmails sends congratulations emails to all prize winners.
func (s *SettlementService) sendPrizeWinnerEmails(ctx context.Context, contestID string, contestInfo *ContestInfo, prizes []contracts.PrizeAllocation, rankings []contracts.FinalRanking) {
	// Skip if email notifier is not configured
	if s.app.emailNotifier == nil {
		s.app.log().Debug("Email notifier not configured, skipping prize winner emails")
		return
	}

	if len(prizes) == 0 {
		return
	}

	// Get winner user IDs
	winnerIDs := make([]string, len(prizes))
	prizeMap := make(map[string]contracts.PrizeAllocation)
	for i, prize := range prizes {
		winnerIDs[i] = prize.UserID
		prizeMap[prize.UserID] = prize
	}

	// Get winner details including emails
	winners, err := s.app.getPrizeWinnerEmails(ctx, contestID, winnerIDs)
	if err != nil {
		s.app.log().Warn("Failed to get prize winner emails",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return
	}

	// Filter winners by email notification preferences
	emailEnabledMap, _ := prefs.IsEnabledBatch(ctx, s.app.db, winnerIDs, inapp.NotifTypePrizeWon, "email")

	// Get total participants count
	totalParticipants, _ := s.app.getTotalParticipants(ctx, contestID)

	// Build ranking map for T-Point info
	rankingMap := make(map[string]contracts.FinalRanking)
	for _, r := range rankings {
		rankingMap[r.UserID] = r
	}

	// Build results URL
	resultsURL := fmt.Sprintf("%s/contests/%s/results", getBaseURL(s.app.config.Environment), contestID)

	// Prepare recipient list
	recipients := make([]notification.PrizeWonRecipient, 0, len(winners))
	for _, winner := range winners {
		prize, ok := prizeMap[winner.UserID]
		if !ok || !emailEnabledMap[winner.UserID] {
			continue
		}

		// Format prize amount (cents to dollars)
		prizeAmount := fmt.Sprintf("$%.2f", float64(prize.AmountCents)/100.0)

		// Format final P&L
		var finalPnL string
		if winner.FinalScore >= 0 {
			finalPnL = fmt.Sprintf("+$%.2f", winner.FinalScore)
		} else {
			finalPnL = fmt.Sprintf("-$%.2f", -winner.FinalScore)
		}

		// Get T-Point gain
		var traggeScoreGain string
		if ranking, ok := rankingMap[winner.UserID]; ok && ranking.TraggePointContribution > 0 {
			traggeScoreGain = fmt.Sprintf("%.1f", ranking.TraggePointContribution)
		}

		recipients = append(recipients, notification.PrizeWonRecipient{
			Email: winner.Email,
			Data: notification.PrizeWonData{
				UserName:          winner.UserName,
				ContestID:         contestID,
				ContestName:       contestInfo.Name,
				FinalRank:         prize.Rank,
				TotalParticipants: totalParticipants,
				PrizeAmount:       prizeAmount,
				FinalPnL:          finalPnL,
				TraggePointGain:   traggeScoreGain,
				ResultsURL:        resultsURL,
			},
		})
	}

	if len(recipients) == 0 {
		return
	}

	s.app.log().Info("Sending prize winner emails",
		zap.String("contest_id", contestID),
		zap.Int("recipients", len(recipients)))

	// Send emails in batches (to avoid overwhelming the email service)
	batchSize := s.app.config.PrizeEmailBatchSize
	for i := 0; i < len(recipients); i += batchSize {
		end := i + batchSize
		if end > len(recipients) {
			end = len(recipients)
		}
		batch := recipients[i:end]

		result := s.app.emailNotifier.SendPrizeWonBatch(ctx, batch)

		s.app.log().Info("Prize winner email batch sent",
			zap.String("contest_id", contestID),
			zap.Int("batch_start", i),
			zap.Int("batch_size", len(batch)),
			zap.Int("successful", len(result.Successful)),
			zap.Int("failed", len(result.Failed)))

		// Log failures
		for _, fail := range result.Failed {
			s.app.log().Warn("Failed to send prize winner email",
				zap.String("recipient", fail.Recipient),
				zap.Error(fail.Error))
		}
	}
}

// getBaseURL returns the base URL for the frontend based on the environment.
func getBaseURL(env string) string {
	switch env {
	case "production":
		return "https://tragge.com"
	case "staging":
		return "https://staging.tragge.com"
	default:
		return "http://localhost:5173"
	}
}

// createPrizeWonInAppNotifications creates in-app notifications for all prize winners.
func (s *SettlementService) createPrizeWonInAppNotifications(ctx context.Context, contestID string, contestInfo *ContestInfo, prizes []contracts.PrizeAllocation) {
	if len(prizes) == 0 {
		return
	}

	s.app.log().Info("Creating prize won in-app notifications",
		zap.String("contest_id", contestID),
		zap.Int("winners", len(prizes)))

	// Filter winners by in-app notification preferences
	winnerIDs := make([]string, len(prizes))
	for i, p := range prizes {
		winnerIDs[i] = p.UserID
	}
	enabledMap, _ := prefs.IsEnabledBatch(ctx, s.app.db, winnerIDs, inapp.NotifTypePrizeWon, "in_app")

	successCount := 0
	for _, prize := range prizes {
		if !enabledMap[prize.UserID] {
			continue
		}
		err := inapp.CreatePrizeWonNotification(ctx, s.app.db, prize.UserID, contestID, contestInfo.Name, prize.Rank, prize.AmountCents, "IRR")
		if err != nil {
			s.app.log().Warn("Failed to create prize won notification",
				zap.String("contest_id", contestID),
				zap.String("user_id", prize.UserID),
				zap.Error(err))
		} else {
			successCount++
		}
	}

	s.app.log().Info("Created prize won in-app notifications",
		zap.String("contest_id", contestID),
		zap.Int("created", successCount),
		zap.Int("total", len(prizes)))
}
