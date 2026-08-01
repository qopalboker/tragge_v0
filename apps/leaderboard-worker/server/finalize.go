package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"strconv"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/Parsaeffatravesh/tragge/packages/notification"
	"github.com/Parsaeffatravesh/tragge/packages/notification/inapp"
	"github.com/Parsaeffatravesh/tragge/packages/notification/prefs"
	"github.com/Parsaeffatravesh/tragge/packages/domain/traggepoint"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
)

// Task 10.2: Tournament economics Prometheus metrics
var (
	traggeTournamentPrizePool = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tragge_tournament_prize_pool_total",
		Help: "Prize pool amount in cents by tournament type",
	}, []string{"tournament_type"})

	traggeTournamentParticipants = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "tragge_tournament_participants_total",
		Help: "Number of participants by tournament type",
	}, []string{"tournament_type"})

	traggeTournamentCommissionEarned = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tragge_tournament_commission_earned_total",
		Help: "Total commission earned in cents",
	})

	traggeTournamentPayouts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tragge_tournament_payouts_total",
		Help: "Total payouts by rank",
	}, []string{"rank"})

	traggeTournamentPrizePoolMismatch = promauto.NewCounter(prometheus.CounterOpts{
		Name: "tragge_tournament_prize_pool_mismatch_total",
		Help: "Number of prize pool calculation mismatches detected",
	})
)

// ContestInfo holds contest details needed for payout calculation.
type ContestInfo struct {
	ID                string
	Name              string
	Type              string // Tournament type (e.g., "standard", "quick", "weekly")
	EntryFeeCents     int64
	PlatformFeeBps    int
	PrizePoolNetCents int64 // Stored prize pool accumulated during joins
	CommissionAmount  int64 // Stored commission accumulated during joins
	IsFree            bool  // Whether this is a free tournament
	StartTime         time.Time
	EndTime           time.Time
}

// FinalizationState tracks the progress of contest finalization for crash recovery.
type FinalizationState struct {
	ContestID              string
	FinalizationStartedAt  time.Time
	PayoutsCalculated      bool
	PayoutsCalculatedAt    sql.NullTime
	RanksWritten           bool
	RanksWrittenAt         sql.NullTime
	WalletsCredited        bool
	WalletsCreditedat      sql.NullTime
	StatusUpdated          bool
	StatusUpdatedAt        sql.NullTime
	FinalizationCompletedAt sql.NullTime
	ErrorMessage           sql.NullString
	LastErrorAt            sql.NullTime
	RetryCount             int
	Metadata               []byte
}

// getOrCreateFinalizationState retrieves existing finalization state or creates a new one.
// This supports crash recovery by allowing the finalization process to resume.
func (a *App) getOrCreateFinalizationState(ctx context.Context, contestID string) (*FinalizationState, error) {
	state := &FinalizationState{ContestID: contestID}

	// Try to get existing state
	err := a.db.QueryRowContext(ctx, `
		SELECT
			contest_id, finalization_started_at,
			payouts_calculated, payouts_calculated_at,
			ranks_written, ranks_written_at,
			wallets_credited, wallets_credited_at,
			status_updated, status_updated_at,
			finalization_completed_at, error_message, last_error_at, retry_count, metadata
		FROM contest_finalization_state
		WHERE contest_id = $1
	`, contestID).Scan(
		&state.ContestID, &state.FinalizationStartedAt,
		&state.PayoutsCalculated, &state.PayoutsCalculatedAt,
		&state.RanksWritten, &state.RanksWrittenAt,
		&state.WalletsCredited, &state.WalletsCreditedat,
		&state.StatusUpdated, &state.StatusUpdatedAt,
		&state.FinalizationCompletedAt, &state.ErrorMessage, &state.LastErrorAt, &state.RetryCount, &state.Metadata,
	)

	if err == sql.ErrNoRows {
		// Create new state
		state.FinalizationStartedAt = time.Now().UTC()
		_, err = a.db.ExecContext(ctx, `
			INSERT INTO contest_finalization_state (contest_id, finalization_started_at)
			VALUES ($1, $2)
		`, contestID, state.FinalizationStartedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to create finalization state: %w", err)
		}

		// Also record in contest_status_history for audit trail
		a.recordFinalizationHistoryEvent(ctx, contestID, "finalization_started", nil)

		return state, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get finalization state: %w", err)
	}

	// If we're re-entering, increment retry count
	if !state.FinalizationCompletedAt.Valid {
		state.RetryCount++
		_, err = a.db.ExecContext(ctx, `
			UPDATE contest_finalization_state
			SET retry_count = retry_count + 1
			WHERE contest_id = $1
		`, contestID)
		if err != nil {
			a.log().Warn("Failed to update retry count",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}

		a.log().Info("Resuming finalization after crash/retry",
			zap.String("contest_id", contestID),
			zap.Int("retry_count", state.RetryCount),
			zap.Bool("payouts_calculated", state.PayoutsCalculated),
			zap.Bool("ranks_written", state.RanksWritten),
			zap.Bool("wallets_credited", state.WalletsCredited),
			zap.Bool("status_updated", state.StatusUpdated))
	}

	return state, nil
}

// markPayoutsCalculated records that payout calculation is complete.
func (a *App) markPayoutsCalculated(ctx context.Context, contestID string, metadata map[string]interface{}) error {
	metadataJSON, _ := json.Marshal(metadata)
	_, err := a.db.ExecContext(ctx, `
		UPDATE contest_finalization_state
		SET payouts_calculated = TRUE, payouts_calculated_at = NOW(), metadata = $2
		WHERE contest_id = $1
	`, contestID, metadataJSON)
	return err
}

// markRanksWritten records that ranks have been written to the database.
func (a *App) markRanksWritten(ctx context.Context, contestID string) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE contest_finalization_state
		SET ranks_written = TRUE, ranks_written_at = NOW()
		WHERE contest_id = $1
	`, contestID)
	return err
}

// markWalletsCredited records that wallet credits have been processed.
func (a *App) markWalletsCredited(ctx context.Context, contestID string) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE contest_finalization_state
		SET wallets_credited = TRUE, wallets_credited_at = NOW()
		WHERE contest_id = $1
	`, contestID)
	return err
}

// markRanksAndWalletsCredited atomically records that both ranks have been written
// and wallet credits have been processed. This prevents crash-recovery issues where
// one flag could be set without the other.
func (a *App) markRanksAndWalletsCredited(ctx context.Context, contestID string) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE contest_finalization_state
		SET ranks_written = TRUE, ranks_written_at = NOW(),
		    wallets_credited = TRUE, wallets_credited_at = NOW()
		WHERE contest_id = $1
	`, contestID)
	return err
}

// markStatusUpdated records that contest status has been updated.
func (a *App) markStatusUpdated(ctx context.Context, contestID string) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE contest_finalization_state
		SET status_updated = TRUE, status_updated_at = NOW()
		WHERE contest_id = $1
	`, contestID)
	return err
}

// markFinalizationCompleted records that finalization has fully completed.
func (a *App) markFinalizationCompleted(ctx context.Context, contestID string) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE contest_finalization_state
		SET finalization_completed_at = NOW()
		WHERE contest_id = $1
	`, contestID)
	if err != nil {
		return err
	}

	// Extend the leaderboard TTL to 7 days after finalization so the data
	// remains available for queries and audits. The normal 24h TTL is too
	// short — it could expire before consumers read the final standings.
	lbKey := LeaderboardKey(contestID)
	if expErr := a.redis.Expire(ctx, lbKey, 7*24*time.Hour).Err(); expErr != nil {
		a.log().Warn("Failed to extend leaderboard TTL after finalization",
			zap.String("contest_id", contestID),
			zap.String("key", lbKey),
			zap.Error(expErr))
	}

	// Record in contest_status_history for audit trail
	a.recordFinalizationHistoryEvent(ctx, contestID, "finalization_completed", nil)

	return nil
}

// recordFinalizationError records an error that occurred during finalization.
func (a *App) recordFinalizationError(ctx context.Context, contestID string, errMsg string) error {
	_, err := a.db.ExecContext(ctx, `
		UPDATE contest_finalization_state
		SET error_message = $2, last_error_at = NOW()
		WHERE contest_id = $1
	`, contestID, errMsg)
	return err
}

// recordFinalizationHistoryEvent records a finalization event in the contest_status_history table.
func (a *App) recordFinalizationHistoryEvent(ctx context.Context, contestID string, eventType string, metadata map[string]interface{}) {
	metadataJSON, _ := json.Marshal(metadata)

	// Get current contest status
	var currentStatus sql.NullString
	_ = a.db.QueryRowContext(ctx, `SELECT status FROM contests WHERE id = $1`, contestID).Scan(&currentStatus)

	_, err := a.db.ExecContext(ctx, `
		INSERT INTO contest_status_history (contest_id, from_status, to_status, reason, metadata)
		VALUES ($1, $2, $2, $3, $4)
	`, contestID, currentStatus, eventType, metadataJSON)
	if err != nil {
		a.log().Warn("Failed to record finalization history event",
			zap.String("contest_id", contestID),
			zap.String("event_type", eventType),
			zap.Error(err))
	}
}

// startContestStateConsumer starts a Kafka consumer for contest state events.
func (a *App) startContestStateConsumer() {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.log().Error("startContestStateConsumer panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()

	// Create a separate consumer for contest state events
	stateOpts := []kgo.Opt{
		kgo.SeedBrokers(a.config.KafkaBrokers...),
		kgo.ConsumerGroup(a.config.ConsumerGroup + "-contest-state"),
		kgo.ConsumeTopics(a.config.ContestStateTopic),
		kgo.DisableAutoCommit(),
	}
	stateOpts = append(stateOpts, infra.KafkaSecurityOpts()...)
	consumer, err := kgo.NewClient(stateOpts...)
	if err != nil {
		a.log().Error("Failed to create contest state consumer", zap.Error(err))
		return
	}
	defer consumer.Close()

	a.log().Info("Starting contest state consumer",
		zap.String("topic", a.config.ContestStateTopic))

	for {
		select {
		case <-a.ctx.Done():
			a.log().Info("Contest state consumer shutting down")
			return
		default:
		}

		fetches := consumer.PollFetches(a.ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			a.log().Error("Contest state fetch error", zap.Error(err))
			continue
		}

		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			for _, record := range p.Records {
				a.processContestStateRecord(record)
			}

			if err := consumer.CommitUncommittedOffsets(a.ctx); err != nil {
				a.log().Error("Contest state commit error", zap.Error(err))
			}
		})
	}
}

// processContestStateRecord processes a single contest state record.
func (a *App) processContestStateRecord(record *kgo.Record) {
	var state contracts.ContestState
	if err := json.Unmarshal(record.Value, &state); err != nil {
		a.log().Error("Failed to unmarshal ContestState", zap.Error(err))
		return
	}

	a.log().Info("Contest state received",
		zap.String("contest_id", state.ContestID),
		zap.String("phase", string(state.Phase)))

	// Only finalize on settling or cancelled status.
	// Legacy events without Status field fall back to phase-based check (ENDED only, not FROZEN).
	// Note: StatusCompleted is intentionally excluded — finalization should only run once,
	// triggered by the settling transition. The completed transition happens after settling
	// is done and should not re-trigger finalization.
	if state.Status == "" {
		if state.Phase != contracts.ContestPhaseEnded {
			return
		}
	} else if state.Status != contracts.ContestStatusSettling &&
		state.Status != contracts.ContestStatusCancelled {
		return
	}

	// Finalize the contest
	if err := a.finalizeContest(a.ctx, state.ContestID); err != nil {
		a.log().Error("Failed to finalize contest",
			zap.String("contest_id", state.ContestID),
			zap.Error(err))
		return
	}

	a.log().Info("Successfully finalized contest",
		zap.String("contest_id", state.ContestID))
}

// finalizeContest computes final ranks and payouts for a contest.
// This function supports crash recovery by tracking finalization state.
// If the worker crashes mid-finalization, it can resume from the last completed step.
func (a *App) finalizeContest(ctx context.Context, contestID string) error {
	startTime := time.Now()

	// Get or create finalization state for crash recovery
	finState, err := a.getOrCreateFinalizationState(ctx, contestID)
	if err != nil {
		a.sendLeaderboardCalculationError(contestID, fmt.Errorf("failed to get/create finalization state: %w", err))
		return err
	}

	// If already completed, skip
	if finState.FinalizationCompletedAt.Valid {
		a.log().Info("Finalization already completed, skipping",
			zap.String("contest_id", contestID),
			zap.Time("completed_at", finState.FinalizationCompletedAt.Time))
		return nil
	}

	// Set up a goroutine to check if finalization is taking too long
	slowCheckDone := make(chan struct{})
	infra.SafeGo(a.log(), "finalization-timeout-check", func() {
		select {
		case <-time.After(5 * time.Minute):
			elapsed := time.Since(startTime)
			a.sendLeaderboardCalculationTakingTooLong(contestID, elapsed)
		case <-slowCheckDone:
			return
		}
	})
	defer close(slowCheckDone)

	// Get contest info from database
	contestInfo, err := a.getContestInfo(ctx, contestID)
	if err != nil {
		a.recordFinalizationError(ctx, contestID, fmt.Sprintf("failed to get contest info: %v", err))
		a.sendLeaderboardCalculationError(contestID, fmt.Errorf("failed to get contest info: %w", err))
		return err
	}

	// Get all participants from Redis leaderboard (use sharded worker for consistent key format)
	count, err := a.shardedWorker.GetLeaderboardSize(ctx, contestID)
	if err != nil {
		a.recordFinalizationError(ctx, contestID, fmt.Sprintf("failed to get leaderboard size: %v", err))
		a.sendLeaderboardCalculationError(contestID, fmt.Errorf("failed to get leaderboard size: %w", err))
		return err
	}

	// Edge case: 0 participants — mark as cancelled, no payouts
	if count == 0 {
		a.log().Info("No participants in contest, marking as cancelled",
			zap.String("contest_id", contestID))
		if _, err := a.stateMachine.Cancel(ctx, contestID, nil, "No participants in contest"); err != nil {
			a.log().Warn("Failed to mark empty contest as cancelled via state machine",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
		if err := a.markFinalizationCompleted(ctx, contestID); err != nil {
			a.log().Warn("Failed to mark empty contest finalization complete",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
		return nil
	}

	// Get all ranked users (sharded worker decodes tiebreaker for clean DB writes)
	rankedUsers, err := a.shardedWorker.GetTop(ctx, contestID, int(count))
	if err != nil {
		a.recordFinalizationError(ctx, contestID, fmt.Sprintf("failed to get ranked users: %v", err))
		a.sendLeaderboardCalculationError(contestID, fmt.Errorf("failed to get ranked users: %w", err))
		return err
	}

	// Edge case: 1 participant — refund entry fee (no competition occurred)
	if count == 1 && contestInfo.EntryFeeCents > 0 && !contestInfo.IsFree {
		a.log().Info("Only 1 participant in contest, refunding entry fee",
			zap.String("contest_id", contestID))
		user := rankedUsers[0]

		// Store original prize pool data before modifying, so retries can recover
		refundMeta := map[string]interface{}{
			"type":                  "single_participant_refund",
			"user_id":               user.UserID,
			"entry_fee_cents":       contestInfo.EntryFeeCents,
			"prize_pool_net_cents":  contestInfo.PrizePoolNetCents,
			"commission_amount":     contestInfo.CommissionAmount,
		}
		if metaErr := a.markPayoutsCalculated(ctx, contestID, refundMeta); metaErr != nil {
			a.log().Warn("Failed to store refund metadata before transaction",
				zap.String("contest_id", contestID),
				zap.Error(metaErr))
		}

		refundTx, txErr := a.db.BeginTx(ctx, nil)
		if txErr != nil {
			a.recordFinalizationError(ctx, contestID, fmt.Sprintf("failed to begin refund transaction: %v", txErr))
			return txErr
		}
		defer refundTx.Rollback()

		_, refundErr := a.wallet.RefundContestEntryFeeWithReason(ctx, refundTx, user.UserID, contestID,
			contestInfo.Name, contestInfo.EntryFeeCents, wallet.ReasonCodeContestRefundQuorum)
		if refundErr != nil {
			a.recordFinalizationError(ctx, contestID, fmt.Sprintf("failed to refund entry fee to user %s: %v", user.UserID, refundErr))
			return refundErr
		}

		// Reset prize pool and commission on contest since we refunded
		_, err = refundTx.ExecContext(ctx, `
			UPDATE contests SET prize_pool_net_cents = 0, commission_amount = 0 WHERE id = $1
		`, contestID)
		if err != nil {
			a.log().Warn("Failed to reset prize pool after refund",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}

		if err := refundTx.Commit(); err != nil {
			a.recordFinalizationError(ctx, contestID, fmt.Sprintf("failed to commit refund transaction: %v", err))
			return err
		}

		if _, err := a.stateMachine.Cancel(ctx, contestID, nil, "Single participant - entry fee refunded"); err != nil {
			a.log().Warn("Failed to cancel contest via state machine after refund",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
		if err := a.markFinalizationCompleted(ctx, contestID); err != nil {
			a.log().Warn("Failed to mark finalization complete after refund",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}

		a.log().Info("Contest finalized with single participant refund",
			zap.String("contest_id", contestID),
			zap.String("user_id", user.UserID),
			zap.Int64("refund_cents", contestInfo.EntryFeeCents))
		return nil
	}

	// Check for ranking anomalies
	if err := a.validateRanking(contestID, rankedUsers); err != nil {
		a.sendRankingAnomalyDetected(contestID, err.Error())
		// Continue anyway - this is a warning, not a blocker
	}

	// Variables for payout that need to be calculated or retrieved
	var payout *ContestPayout
	var contestAlertInfo ContestInfoAlert

	// STEP 1: Calculate payouts (skip if already done)
	if !finState.PayoutsCalculated {
		// Use stored prize pool if available (accumulated during joins), otherwise calculate
		if contestInfo.PrizePoolNetCents > 0 {
			payout, err = CalculateContestPayoutsWithStoredPool(
				contestID,
				rankedUsers,
				contestInfo.EntryFeeCents,
				contestInfo.PlatformFeeBps,
				contestInfo.PrizePoolNetCents,
			)
		} else {
			payout, err = CalculateContestPayouts(
				contestID,
				rankedUsers,
				contestInfo.EntryFeeCents,
				contestInfo.PlatformFeeBps,
			)
		}
		if err != nil {
			a.recordFinalizationError(ctx, contestID, fmt.Sprintf("failed to calculate payouts: %v", err))
			a.sendLeaderboardCalculationError(contestID, fmt.Errorf("failed to calculate payouts: %w", err))
			return err
		}

		// Store full payout metadata (including individual payouts) for crash recovery.
		// On retry, we recover from this stored data instead of recalculating,
		// which prevents inconsistencies if rankings changed between attempts.
		payoutMetadata := map[string]interface{}{
			"participants_count": payout.ParticipantsCount,
			"prize_pool_gross":   payout.PrizePoolGross,
			"prize_pool_net":     payout.PrizePoolNet,
			"platform_fee_bps":   payout.PlatformFeeBps,
			"winners_count":      payout.WinnersCount,
			"total_paid_out":     payout.TotalPaidOut,
			"payouts":            payout.Payouts,
		}

		if err := a.markPayoutsCalculated(ctx, contestID, payoutMetadata); err != nil {
			a.log().Warn("Failed to mark payouts calculated",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}

		a.log().Info("Contest payout summary",
			zap.String("contest_id", contestID),
			zap.Int("participants", payout.ParticipantsCount),
			zap.Int64("gross", payout.PrizePoolGross),
			zap.Int64("net", payout.PrizePoolNet),
			zap.Int("winners", payout.WinnersCount),
			zap.Int64("paid", payout.TotalPaidOut),
		)
	} else {
		// Recover payouts from stored metadata to ensure consistency.
		// Recalculating could produce different results if rankings changed
		// (e.g., admin DB edit, Redis data change) since the first calculation.
		if finState.Metadata != nil {
			var stored struct {
				ParticipantsCount int            `json:"participants_count"`
				PrizePoolGross    int64          `json:"prize_pool_gross"`
				PrizePoolNet      int64          `json:"prize_pool_net"`
				PlatformFeeBps    int            `json:"platform_fee_bps"`
				WinnersCount      int            `json:"winners_count"`
				TotalPaidOut      int64          `json:"total_paid_out"`
				Payouts           []PayoutResult `json:"payouts"`
			}
			if err := json.Unmarshal(finState.Metadata, &stored); err == nil && len(stored.Payouts) > 0 {
				payout = &ContestPayout{
					ContestID:         contestID,
					ParticipantsCount: stored.ParticipantsCount,
					EntryFeeCents:     contestInfo.EntryFeeCents,
					PlatformFeeBps:    stored.PlatformFeeBps,
					PrizePoolGross:    stored.PrizePoolGross,
					PrizePoolNet:      stored.PrizePoolNet,
					WinnersCount:      stored.WinnersCount,
					Payouts:           stored.Payouts,
					TotalPaidOut:      stored.TotalPaidOut,
				}
				a.log().Info("Recovered payouts from stored metadata",
					zap.String("contest_id", contestID),
					zap.Int("winners", len(stored.Payouts)))
			}
		}
		// If metadata recovery failed, do NOT recalculate — rankings may have changed
		// since the first attempt, which could pay different winners. Require manual intervention.
		if payout == nil {
			errMsg := "could not recover payouts from stored metadata; manual intervention required to prevent paying different winners on retry"
			a.log().Error(errMsg,
				zap.String("contest_id", contestID),
				zap.Int("retry_count", finState.RetryCount))
			a.recordFinalizationError(ctx, contestID, errMsg)
			return fmt.Errorf("%s: contest_id=%s", errMsg, contestID)
		}
	}

	// Record tournament economics metrics
	tournamentType := contestInfo.Type
	if tournamentType == "" {
		tournamentType = "standard"
	}
	traggeTournamentPrizePool.WithLabelValues(tournamentType).Set(float64(payout.PrizePoolNet))
	traggeTournamentParticipants.WithLabelValues(tournamentType).Set(float64(payout.ParticipantsCount))
	commission := payout.PrizePoolGross - payout.PrizePoolNet
	if commission > 0 {
		traggeTournamentCommissionEarned.Add(float64(commission))
	}
	for _, p := range payout.Payouts {
		rankStr := strconv.Itoa(p.Rank)
		if p.Rank > 10 {
			rankStr = "11+"
		}
		traggeTournamentPayouts.WithLabelValues(rankStr).Inc()
	}
	// Mismatch detection: compare stored vs calculated prize pool
	if contestInfo.PrizePoolNetCents > 0 && payout.ParticipantsCount > 0 {
		calculated := CalculatePrizePoolNet(
			CalculatePrizePoolGross(payout.ParticipantsCount, contestInfo.EntryFeeCents),
			contestInfo.PlatformFeeBps)
		if math.Abs(float64(calculated-contestInfo.PrizePoolNetCents)) > float64(contestInfo.EntryFeeCents) {
			traggeTournamentPrizePoolMismatch.Inc()
			a.log().Warn("Prize pool mismatch detected",
				zap.String("contest_id", contestID),
				zap.Int64("stored", contestInfo.PrizePoolNetCents),
				zap.Int64("calculated", calculated))
		}
	}

	// Build alert info for notifications
	contestAlertInfo = a.buildContestAlertInfo(contestInfo, payout)

	// STEP 2: Write final ranks (skip if already done)
	// NOTE: Prize distribution (wallet credits) is handled exclusively by settlement-service
	// to prevent double payouts. Leaderboard-worker only writes ranks and score history.
	if !finState.RanksWritten {
		_, err = a.writeFinalRanksAndPrizesWithTracking(ctx, contestID, rankedUsers, payout, contestInfo)
		if err != nil {
			// Critical: database transaction failed
			a.recordFinalizationError(ctx, contestID, fmt.Sprintf("database transaction failed: %v", err))
			a.sendDatabaseTransactionFailed(contestID, "rank_writing", err)
			return err
		}

		// Mark ranks written and wallets credited (wallets are handled by settlement-service)
		if err := a.markRanksAndWalletsCredited(ctx, contestID); err != nil {
			a.log().Warn("Failed to mark ranks written",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
	} else {
		a.log().Info("Ranks already written, skipping",
			zap.String("contest_id", contestID))
	}

	// STEP 4: Update contest status to completed (skip if already done)
	if !finState.StatusUpdated {
		if _, err := a.stateMachine.Complete(ctx, contestID); err != nil {
			a.recordFinalizationError(ctx, contestID, fmt.Sprintf("failed to complete contest via state machine: %v", err))
			a.sendLeaderboardCalculationError(contestID, fmt.Errorf("failed to complete contest via state machine: %w", err))
			return err
		}

		if err := a.markStatusUpdated(ctx, contestID); err != nil {
			a.log().Warn("Failed to mark status updated",
				zap.String("contest_id", contestID),
				zap.Error(err))
		}
	} else {
		a.log().Info("Status already updated, skipping",
			zap.String("contest_id", contestID))
	}

	// Mark finalization as complete
	if err := a.markFinalizationCompleted(ctx, contestID); err != nil {
		a.log().Warn("Failed to mark finalization complete",
			zap.String("contest_id", contestID),
			zap.Error(err))
	}

	// Build winner info for finalized alert
	var winnerInfo UserInfoAlert
	if len(rankedUsers) > 0 {
		winnerInfo = UserInfoAlert{
			UserID: rankedUsers[0].UserID,
			Rank:   rankedUsers[0].Rank,
			Score:  rankedUsers[0].Score,
		}
		if len(payout.Payouts) > 0 {
			winnerInfo.Prize = float64(payout.Payouts[0].PayoutCents) / 100
		}
	}

	// Build stats for finalized alert
	stats := ContestStatsAlert{
		TotalTrades: 0, // Would need to aggregate from user stats
		TotalVolume: 0,
	}

	a.sendContestFinalized(contestAlertInfo, winnerInfo, stats)

	// Send contest ended emails to ALL participants with personalized results
	a.sendContestEndedEmails(ctx, contestID, contestInfo, rankedUsers, payout)

	// Record settlement and prize distributions for audit
	a.recordSettlementAndPrizeDistributions(ctx, contestID, payout, rankedUsers)

	// Update Redis global leaderboard (leaderboard:global sorted set)
	if err := a.updateGlobalLeaderboard(ctx, contestID, rankedUsers, payout.ParticipantsCount, contestInfo.EndTime); err != nil {
		a.log().Error("Failed to update global leaderboard",
			zap.String("contest_id", contestID),
			zap.Error(err))
		// Non-fatal: global leaderboard can be rebuilt from user_stats
	}

	a.log().Info("Contest finalized successfully",
		zap.String("contest_id", contestID),
		zap.Duration("duration", time.Since(startTime)),
		zap.Int("participants", payout.ParticipantsCount),
		zap.Int("winners", payout.WinnersCount),
		zap.Int("retry_count", finState.RetryCount))

	return nil
}

// buildContestAlertInfo creates a ContestInfoAlert from contest info and payout.
func (a *App) buildContestAlertInfo(contestInfo *ContestInfo, payout *ContestPayout) ContestInfoAlert {
	duration := ""
	if !contestInfo.StartTime.IsZero() && !contestInfo.EndTime.IsZero() {
		duration = contestInfo.EndTime.Sub(contestInfo.StartTime).String()
	}

	return ContestInfoAlert{
		ID:           contestInfo.ID,
		Name:         contestInfo.Name,
		Participants: payout.ParticipantsCount,
		PrizePool:    float64(payout.PrizePoolNet) / 100,
		EntryFee:     float64(contestInfo.EntryFeeCents) / 100,
		StartTime:    contestInfo.StartTime,
		EndTime:      contestInfo.EndTime,
		Duration:     duration,
	}
}

// validateRanking checks for anomalies in the ranking data.
func (a *App) validateRanking(contestID string, rankedUsers []LeaderboardEntry) error {
	if len(rankedUsers) == 0 {
		return nil
	}

	// Check for duplicate ranks
	seenRanks := make(map[int]bool)
	for _, user := range rankedUsers {
		if seenRanks[user.Rank] {
			return fmt.Errorf("duplicate rank %d detected", user.Rank)
		}
		seenRanks[user.Rank] = true
	}

	// Check for gaps in rankings
	for i := 1; i <= len(rankedUsers); i++ {
		if !seenRanks[i] {
			return fmt.Errorf("missing rank %d in leaderboard", i)
		}
	}

	// Check for unreasonable score spreads (e.g., suspicious activity)
	if len(rankedUsers) >= 2 {
		topScore := rankedUsers[0].Score
		secondScore := rankedUsers[1].Score
		if secondScore > 0 && topScore/secondScore > 100 {
			return fmt.Errorf("suspicious score spread: top score (%.2f) is >100x second place (%.2f)", topScore, secondScore)
		}
	}

	return nil
}

// getContestInfo retrieves contest details from the database.
func (a *App) getContestInfo(ctx context.Context, contestID string) (*ContestInfo, error) {
	var info ContestInfo
	var platformFeeBps sql.NullInt32
	var commissionRate sql.NullFloat64
	var name sql.NullString
	var contestType sql.NullString
	var startTime, endTime sql.NullTime
	var prizePoolNetCents sql.NullInt64
	var commissionAmount sql.NullInt64
	var isFree sql.NullBool

	err := a.db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(type, 'standard'), entry_fee_cents, platform_fee_bps, commission_rate, starts_at, ends_at,
		        COALESCE(prize_pool_net_cents, 0), COALESCE(commission_amount, 0), COALESCE(is_free, FALSE)
		 FROM contests WHERE id = $1`,
		contestID,
	).Scan(&info.ID, &name, &contestType, &info.EntryFeeCents, &platformFeeBps, &commissionRate, &startTime, &endTime,
		&prizePoolNetCents, &commissionAmount, &isFree)

	if err != nil {
		return nil, err
	}

	if name.Valid {
		info.Name = name.String
	} else {
		info.Name = contestID // Fallback to ID if name is not set
	}

	if contestType.Valid && contestType.String != "" {
		info.Type = contestType.String
	} else {
		info.Type = "standard"
	}

	// Resolve effective fee using unified priority logic (platform_fee_bps > commission_rate > default)
	var feeBps int
	if platformFeeBps.Valid {
		feeBps = int(platformFeeBps.Int32)
	}
	var rate float64
	if commissionRate.Valid {
		rate = commissionRate.Float64
	}
	info.PlatformFeeBps = ResolveEffectiveFeeBps(feeBps, rate)

	if startTime.Valid {
		info.StartTime = startTime.Time
	}
	if endTime.Valid {
		info.EndTime = endTime.Time
	}

	if prizePoolNetCents.Valid {
		info.PrizePoolNetCents = prizePoolNetCents.Int64
	}
	if commissionAmount.Valid {
		info.CommissionAmount = commissionAmount.Int64
	}
	if isFree.Valid {
		info.IsFree = isFree.Bool
	}

	return &info, nil
}

// UserStats holds statistics for a user in a contest.
type UserStats struct {
	TradesCount          int
	AvgTradeDurationSecs int
	TopSymbol            string
	TopSymbolPnL         float64
	TotalPnL             float64
}

// getUserStatsForContest retrieves user statistics for a specific contest.
// Deprecated: Use getBatchUserStatsForContest for bulk operations during finalization.
func (a *App) getUserStatsForContest(ctx context.Context, tx *sql.Tx, contestID, userID string) (*UserStats, error) {
	stats := &UserStats{}

	// Get total trades count (filled orders)
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM orders WHERE contest_id = $1 AND user_id = $2 AND status = 'filled'`,
		contestID, userID,
	).Scan(&stats.TradesCount)
	if err != nil {
		return nil, err
	}

	// Get average trade duration (time from order creation to fill)
	// We calculate this by joining orders with fills and averaging the time difference
	var avgDuration sql.NullFloat64
	err = tx.QueryRowContext(ctx,
		`SELECT AVG(EXTRACT(EPOCH FROM (f.created_at - o.created_at)))
		 FROM orders o
		 JOIN fills f ON o.order_id = f.order_id
		 WHERE o.contest_id = $1 AND o.user_id = $2`,
		contestID, userID,
	).Scan(&avgDuration)
	if err != nil {
		return nil, err
	}
	if avgDuration.Valid {
		stats.AvgTradeDurationSecs = int(avgDuration.Float64)
	}

	// Get top symbol (symbol with highest realized PnL)
	var topSymbol sql.NullString
	var topSymbolPnL sql.NullFloat64
	err = tx.QueryRowContext(ctx,
		`SELECT symbol, SUM(realized_score) as total_pnl
		 FROM positions
		 WHERE contest_id = $1 AND user_id = $2
		 GROUP BY symbol
		 ORDER BY total_pnl DESC
		 LIMIT 1`,
		contestID, userID,
	).Scan(&topSymbol, &topSymbolPnL)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if topSymbol.Valid {
		stats.TopSymbol = topSymbol.String
	}
	if topSymbolPnL.Valid {
		stats.TopSymbolPnL = topSymbolPnL.Float64
	}

	// Get total PnL from contest_participants
	var totalPnL sql.NullFloat64
	err = tx.QueryRowContext(ctx,
		`SELECT total_score FROM contest_participants WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID,
	).Scan(&totalPnL)
	if err != nil {
		return nil, err
	}
	if totalPnL.Valid {
		stats.TotalPnL = totalPnL.Float64
	}

	return stats, nil
}

// getBatchUserStatsForContest retrieves statistics for all users in a contest
// using batch queries instead of per-user queries. Returns a map of user_id -> *UserStats.
// This replaces N calls to getUserStatsForContest with 3 queries total.
func (a *App) getBatchUserStatsForContest(ctx context.Context, tx *sql.Tx, contestID string) (map[string]*UserStats, error) {
	statsMap := make(map[string]*UserStats)

	// Query 1: Get trade counts and average durations for all users in this contest.
	// Uses a single JOIN across orders→fills grouped by user_id.
	rows, err := tx.QueryContext(ctx,
		`SELECT
			o.user_id,
			COUNT(*) FILTER (WHERE o.status = 'filled') AS trades_count,
			AVG(EXTRACT(EPOCH FROM (f.created_at - o.created_at))) AS avg_duration
		 FROM orders o
		 LEFT JOIN fills f ON o.order_id = f.order_id
		 WHERE o.contest_id = $1
		 GROUP BY o.user_id`,
		contestID,
	)
	if err != nil {
		return nil, fmt.Errorf("batch trades query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID string
		var tradesCount int
		var avgDuration sql.NullFloat64
		if err := rows.Scan(&userID, &tradesCount, &avgDuration); err != nil {
			return nil, fmt.Errorf("batch trades scan: %w", err)
		}
		stats := &UserStats{TradesCount: tradesCount}
		if avgDuration.Valid {
			stats.AvgTradeDurationSecs = int(avgDuration.Float64)
		}
		statsMap[userID] = stats
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("batch trades iterate: %w", err)
	}

	// Query 2: Get top symbol per user (symbol with highest realized PnL).
	// Uses DISTINCT ON to pick the top symbol per user in a single query.
	rows2, err := tx.QueryContext(ctx,
		`SELECT DISTINCT ON (user_id)
			user_id, symbol, total_pnl
		 FROM (
			SELECT user_id, symbol, SUM(realized_score) AS total_pnl
			FROM positions
			WHERE contest_id = $1
			GROUP BY user_id, symbol
		 ) sub
		 ORDER BY user_id, total_pnl DESC`,
		contestID,
	)
	if err != nil {
		return nil, fmt.Errorf("batch top symbol query: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var userID string
		var symbol sql.NullString
		var pnl sql.NullFloat64
		if err := rows2.Scan(&userID, &symbol, &pnl); err != nil {
			return nil, fmt.Errorf("batch top symbol scan: %w", err)
		}
		stats, ok := statsMap[userID]
		if !ok {
			stats = &UserStats{}
			statsMap[userID] = stats
		}
		if symbol.Valid {
			stats.TopSymbol = symbol.String
		}
		if pnl.Valid {
			stats.TopSymbolPnL = pnl.Float64
		}
	}
	if err := rows2.Err(); err != nil {
		return nil, fmt.Errorf("batch top symbol iterate: %w", err)
	}

	// Query 3: Get total PnL from contest_participants for all users.
	rows3, err := tx.QueryContext(ctx,
		`SELECT user_id, total_score
		 FROM contest_participants
		 WHERE contest_id = $1`,
		contestID,
	)
	if err != nil {
		return nil, fmt.Errorf("batch total pnl query: %w", err)
	}
	defer rows3.Close()

	for rows3.Next() {
		var userID string
		var totalPnL sql.NullFloat64
		if err := rows3.Scan(&userID, &totalPnL); err != nil {
			return nil, fmt.Errorf("batch total pnl scan: %w", err)
		}
		stats, ok := statsMap[userID]
		if !ok {
			stats = &UserStats{}
			statsMap[userID] = stats
		}
		if totalPnL.Valid {
			stats.TotalPnL = totalPnL.Float64
		}
	}
	if err := rows3.Err(); err != nil {
		return nil, fmt.Errorf("batch total pnl iterate: %w", err)
	}

	return statsMap, nil
}

// writeFinalRanksAndPrizesWithTracking writes final ranks and prizes to the database,
// credits winners' wallets, and tracks any failed payouts.
func (a *App) writeFinalRanksAndPrizesWithTracking(
	ctx context.Context,
	contestID string,
	rankedUsers []LeaderboardEntry,
	payout *ContestPayout,
	contestInfo *ContestInfo,
) ([]FailedPayout, error) {
	failedPayouts, err := a.writeFinalRanksAndPrizesInternal(ctx, contestID, rankedUsers, payout, contestInfo.EndTime)
	return failedPayouts, err
}

// writeFinalRanksAndPrizes writes final ranks and score history to the database.
// Prize distribution (wallet credits) is handled exclusively by settlement-service.
// Deprecated: Use writeFinalRanksAndPrizesWithTracking for better error tracking.
func (a *App) writeFinalRanksAndPrizes(
	ctx context.Context,
	contestID string,
	rankedUsers []LeaderboardEntry,
	payout *ContestPayout,
) error {
	_, err := a.writeFinalRanksAndPrizesInternal(ctx, contestID, rankedUsers, payout, time.Time{})
	return err
}

// writeFinalRanksAndPrizesInternal writes final ranks and score history to the database.
// Prize distribution (wallet credits) is handled exclusively by settlement-service.
func (a *App) writeFinalRanksAndPrizesInternal(
	ctx context.Context,
	contestID string,
	rankedUsers []LeaderboardEntry,
	payout *ContestPayout,
	contestEndTime time.Time,
) ([]FailedPayout, error) {
	var failedPayouts []FailedPayout

	// Start a transaction
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin transaction",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return failedPayouts, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare the rank+score update statement (always safe to execute)
	rankStmt, err := tx.PrepareContext(ctx,
		`UPDATE contest_participants
		 SET final_rank = $1, total_score = $2
		 WHERE contest_id = $3 AND user_id = $4`,
	)
	if err != nil {
		a.log().Error("Failed to prepare rank update statement",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return failedPayouts, fmt.Errorf("prepare rank statement: %w", err)
	}
	defer rankStmt.Close()

	// NOTE: Prize distribution (wallet credits) is handled exclusively by settlement-service
	// to prevent double payouts. This function only writes ranks and score history.

	// Prepare the user_score_history insert statement (with T-Point contribution)
	historyStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO user_score_history (
			user_id, contest_id, rank, score, participants,
			pnl, trades_count, avg_trade_duration_seconds,
			top_symbol, top_symbol_pnl, score_contribution
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (user_id, contest_id) DO UPDATE SET
			rank = EXCLUDED.rank,
			score = EXCLUDED.score,
			participants = EXCLUDED.participants,
			pnl = EXCLUDED.pnl,
			trades_count = EXCLUDED.trades_count,
			avg_trade_duration_seconds = EXCLUDED.avg_trade_duration_seconds,
			top_symbol = EXCLUDED.top_symbol,
			top_symbol_pnl = EXCLUDED.top_symbol_pnl,
			score_contribution = EXCLUDED.score_contribution`,
	)
	if err != nil {
		a.log().Error("Failed to prepare history statement",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return failedPayouts, fmt.Errorf("prepare history statement: %w", err)
	}
	defer historyStmt.Close()

	// Batch-fetch user stats for all participants in one go (3 queries total
	// instead of 4 queries per user). Falls back to empty stats on error.
	batchStats, err := a.getBatchUserStatsForContest(ctx, tx, contestID)
	if err != nil {
		a.log().Warn("Failed to batch-fetch user stats, will use empty stats",
			zap.String("contest_id", contestID),
			zap.Error(err))
		batchStats = make(map[string]*UserStats)
	}

	// Update each participant's rank and score history
	for _, user := range rankedUsers {
		// Update rank and score
		_, err := rankStmt.ExecContext(ctx, user.Rank, user.Score, contestID, user.UserID)
		if err != nil {
			a.log().Error("Failed to update participant rank",
				zap.String("contest_id", contestID),
				zap.String("user_id", user.UserID),
				zap.Error(err))
			continue
		}

		// Look up pre-fetched user statistics
		userStats := batchStats[user.UserID]
		if userStats == nil {
			userStats = &UserStats{}
		}

		// Calculate T-Point contribution for this contest result
		scoreContribution := traggepoint.CalculateContribution(
			decimal.NewFromFloat(user.Score),
			user.Rank,
			payout.ParticipantsCount,
			contestEndTime,
		)

		// Insert into user_score_history (including T-Point contribution)
		_, err = historyStmt.ExecContext(ctx,
			user.UserID,
			contestID,
			user.Rank,
			user.Score,
			payout.ParticipantsCount,
			userStats.TotalPnL,
			userStats.TradesCount,
			userStats.AvgTradeDurationSecs,
			userStats.TopSymbol,
			userStats.TopSymbolPnL,
			scoreContribution,
		)
		if err != nil {
			a.log().Warn("Failed to insert score history for user",
				zap.String("contest_id", contestID),
				zap.String("user_id", user.UserID),
				zap.Error(err))
			// Continue - this is not critical for contest finalization
		}

		// NOTE: Prize distribution (wallet credits) is handled by settlement-service.
		// Leaderboard-worker only writes ranks and score history to prevent double payouts.
	}

	// Write payout summary to leaderboard_snapshots for audit
	payoutSummary := map[string]interface{}{
		"type":               "final_ranks",
		"participants_count": payout.ParticipantsCount,
		"prize_pool_gross":   payout.PrizePoolGross,
		"prize_pool_net":     payout.PrizePoolNet,
		"platform_fee_bps":   payout.PlatformFeeBps,
		"winners_count":      payout.WinnersCount,
		"total_paid_out":     payout.TotalPaidOut,
		"payouts":            payout.Payouts,
		"wallet_credits":     "delegated_to_settlement_service",
	}
	payoutJSON, _ := json.Marshal(payoutSummary)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO leaderboard_snapshots (contest_id, taken_at, payload_json) VALUES ($1, $2, $3)`,
		contestID, time.Now().UTC(), payoutJSON,
	)
	if err != nil {
		a.log().Error("Failed to insert payout summary",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return failedPayouts, fmt.Errorf("insert payout summary: %w", err)
	}

	a.log().Info("Contest finalization ranks written",
		zap.String("contest_id", contestID),
		zap.Int("ranked_users", len(rankedUsers)))

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit transaction",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return failedPayouts, fmt.Errorf("commit transaction: %w", err)
	}

	return failedPayouts, nil
}

// sendContestEndedEmails sends personalized contest results emails to all participants.
func (a *App) sendContestEndedEmails(
	ctx context.Context,
	contestID string,
	contestInfo *ContestInfo,
	rankedUsers []LeaderboardEntry,
	payout *ContestPayout,
) {
	if a.notifications == nil || !a.notifications.HasEmail() {
		return
	}

	emailNotifier := a.notifications.GetEmailNotifier()
	if emailNotifier == nil {
		return
	}

	// Get user_id -> email mapping for all participants
	emailMap, err := a.getContestParticipantEmailMap(ctx, contestID)
	if err != nil {
		a.log().Error("Failed to get participant emails for contest ended notifications",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return
	}

	if len(emailMap) == 0 {
		a.log().Debug("No participant emails found, skipping contest ended emails",
			zap.String("contest_id", contestID))
		return
	}

	// Filter by email notification preferences
	participantIDs := make([]string, 0, len(emailMap))
	for uid := range emailMap {
		participantIDs = append(participantIDs, uid)
	}
	emailEnabledMap, _ := prefs.IsEnabledBatch(ctx, a.db, participantIDs, inapp.NotifTypeContestCompleted, "email")
	for uid, enabled := range emailEnabledMap {
		if !enabled {
			delete(emailMap, uid)
		}
	}
	if len(emailMap) == 0 {
		a.log().Debug("All participants disabled email for contest results",
			zap.String("contest_id", contestID))
		return
	}

	// Build payout lookup map
	payoutMap := make(map[string]int64)
	for _, p := range payout.Payouts {
		payoutMap[p.UserID] = p.PayoutCents
	}

	// Build results URL
	resultsURL := fmt.Sprintf("%s/contests/%s/results", a.config.TradeFrontendURL, contestID)

	// Build personalized recipient list
	recipients := make([]notification.ContestEndedRecipient, 0, len(rankedUsers))
	for _, user := range rankedUsers {
		email, ok := emailMap[user.UserID]
		if !ok || email == "" {
			continue
		}

		prizeCents := payoutMap[user.UserID]
		formattedPrize := ""
		if prizeCents > 0 {
			formattedPrize = fmt.Sprintf("$%.2f", float64(prizeCents)/100)
		}

		formattedScore := formatPnL(user.Score)

		recipients = append(recipients, notification.ContestEndedRecipient{
			Email: email,
			Data: notification.ContestEndedData{
				ContestID:         contestID,
				ContestName:       contestInfo.Name,
				UserRank:          user.Rank,
				TotalParticipants: payout.ParticipantsCount,
				TotalScore:        user.Score,
				FormattedScore:    formattedScore,
				PrizeWon:          int(prizeCents),
				FormattedPrize:    formattedPrize,
				ResultsURL:        resultsURL,
			},
		})
	}

	if len(recipients) == 0 {
		return
	}

	// Send batch emails
	result := emailNotifier.SendContestEndedBatch(ctx, recipients)

	a.log().Info("Sent contest ended emails",
		zap.String("contest_id", contestID),
		zap.Int("successful", len(result.Successful)),
		zap.Int("failed", len(result.Failed)),
		zap.Int("total_recipients", len(recipients)))
}

// getContestParticipantEmailMap returns a map of user_id -> email for all participants in a contest.
func (a *App) getContestParticipantEmailMap(ctx context.Context, contestID string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rows, err := a.db.QueryContext(ctx,
		`SELECT cp.user_id, u.email FROM contest_participants cp
		 JOIN users u ON cp.user_id = u.id
		 WHERE cp.contest_id = $1 AND u.email IS NOT NULL AND u.email != ''`,
		contestID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query participant emails: %w", err)
	}
	defer rows.Close()

	emailMap := make(map[string]string)
	for rows.Next() {
		var userID, email string
		if err := rows.Scan(&userID, &email); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		emailMap[userID] = email
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return emailMap, nil
}

// formatPnL formats a P&L value as a string with sign and dollar symbol.
func formatPnL(pnl float64) string {
	if pnl >= 0 {
		return fmt.Sprintf("+$%.2f", pnl)
	}
	return fmt.Sprintf("-$%.2f", -pnl)
}

// recordSettlementAndPrizeDistributions records the settlement and individual prize
// distributions in the database for audit purposes.
func (a *App) recordSettlementAndPrizeDistributions(ctx context.Context, contestID string, payout *ContestPayout, rankedUsers []LeaderboardEntry) {
	if payout == nil {
		return
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		a.log().Error("Failed to begin settlement recording transaction",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return
	}
	defer tx.Rollback()

	// Create or update contest_settlements record
	var settlementID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO contest_settlements (
			contest_id, status, started_at, completed_at,
			total_participants, total_winners,
			prize_pool_gross_cents, prize_pool_net_cents,
			total_distributed_cents, platform_fee_cents
		) VALUES ($1, 'completed', NOW(), NOW(), $2, $3, $4, $5, $6, $7)
		ON CONFLICT (contest_id) DO UPDATE SET
			status = 'completed',
			completed_at = NOW(),
			total_participants = EXCLUDED.total_participants,
			total_winners = EXCLUDED.total_winners,
			prize_pool_gross_cents = EXCLUDED.prize_pool_gross_cents,
			prize_pool_net_cents = EXCLUDED.prize_pool_net_cents,
			total_distributed_cents = EXCLUDED.total_distributed_cents,
			platform_fee_cents = EXCLUDED.platform_fee_cents
		RETURNING id
	`, contestID,
		payout.ParticipantsCount,
		payout.WinnersCount,
		payout.PrizePoolGross,
		payout.PrizePoolNet,
		payout.TotalPaidOut,
		payout.PrizePoolGross-payout.PrizePoolNet,
	).Scan(&settlementID)
	if err != nil {
		a.log().Error("Failed to create settlement record",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return
	}

	// Build score lookup map for O(1) access instead of O(n) per winner
	scoreMap := make(map[string]float64, len(rankedUsers))
	for _, ru := range rankedUsers {
		scoreMap[ru.UserID] = ru.Score
	}

	// Record individual prize distributions
	for _, p := range payout.Payouts {
		score := scoreMap[p.UserID]

		var percentage float64
		if payout.PrizePoolNet > 0 {
			percentage = float64(p.PayoutCents) / float64(payout.PrizePoolNet) * 100.0
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO prize_distributions (
				settlement_id, contest_id, user_id, rank, final_score,
				prize_amount_cents, prize_percentage, status, credited_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, 'credited', NOW())
			ON CONFLICT (contest_id, user_id) DO UPDATE SET
				rank = EXCLUDED.rank,
				final_score = EXCLUDED.final_score,
				prize_amount_cents = EXCLUDED.prize_amount_cents,
				prize_percentage = EXCLUDED.prize_percentage,
				status = 'credited',
				credited_at = NOW()
		`, settlementID, contestID, p.UserID, p.Rank, score,
			p.PayoutCents, percentage)
		if err != nil {
			a.log().Warn("Failed to record prize distribution",
				zap.String("contest_id", contestID),
				zap.String("user_id", p.UserID),
				zap.Int("rank", p.Rank),
				zap.Error(err))
		}
	}

	if err := tx.Commit(); err != nil {
		a.log().Error("Failed to commit settlement recording",
			zap.String("contest_id", contestID),
			zap.Error(err))
		return
	}

	a.log().Info("Recorded settlement and prize distributions",
		zap.String("contest_id", contestID),
		zap.String("settlement_id", settlementID),
		zap.Int("prize_count", len(payout.Payouts)))
}

// globalLeaderboardKey is the Redis sorted set key for the platform-wide T-Point leaderboard.
const globalLeaderboardKey = "leaderboard:global"

// updateGlobalLeaderboard updates the Redis global leaderboard sorted set with T-Point
// contributions from the finalized contest. Each participant's T-Point contribution
// is calculated and added (ZINCRBY) to their cumulative score in the global sorted set.
func (a *App) updateGlobalLeaderboard(ctx context.Context, contestID string, rankedUsers []LeaderboardEntry, totalParticipants int, contestEndTime time.Time) error {
	if a.redis == nil {
		return nil
	}

	updated := 0
	for _, user := range rankedUsers {
		contribution := traggepoint.CalculateContribution(
			decimal.NewFromFloat(user.Score),
			user.Rank,
			totalParticipants,
			contestEndTime,
		)

		contributionFloat := traggepoint.ToFloat64(contribution)
		if contributionFloat <= 0 {
			continue
		}

		// ZINCRBY atomically increments the user's global T-Point
		if err := a.redis.ZIncrBy(ctx, globalLeaderboardKey, contributionFloat, user.UserID).Err(); err != nil {
			a.log().Warn("Failed to update global leaderboard for user",
				zap.String("contest_id", contestID),
				zap.String("user_id", user.UserID),
				zap.Error(err))
			continue
		}
		updated++
	}

	a.log().Info("Updated global leaderboard",
		zap.String("contest_id", contestID),
		zap.Int("users_updated", updated),
		zap.Int("total_participants", totalParticipants))

	return nil
}
