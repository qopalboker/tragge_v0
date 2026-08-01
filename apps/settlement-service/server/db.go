package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"go.uber.org/zap"
)

// SnapshotPricesJSON is a map type that implements sql.Scanner/driver.Valuer for JSONB columns.
type SnapshotPricesJSON map[string]contracts.Price

func (s *SnapshotPricesJSON) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	data, ok := src.([]byte)
	if !ok {
		return fmt.Errorf("SnapshotPricesJSON.Scan: expected []byte, got %T", src)
	}
	return json.Unmarshal(data, s)
}

func (s SnapshotPricesJSON) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// hashString returns a stable int64 hash for advisory lock keys.
func hashString(s string) int64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return int64(h.Sum64())
}

// ContestInfo holds contest details needed for settlement.
type ContestInfo struct {
	ID                string
	Name              string
	EntryFeeCents     int64
	PlatformFeeBps    int
	PrizePoolNetCents int64 // Accumulated during joins — authoritative source
	CommissionAmount  int64 // Accumulated during joins
	StartTime         time.Time
	EndTime           time.Time
	Status            string
}

// Participant represents a contest participant with their final scores.
type Participant struct {
	UserID        string
	FinalScore    float64
	RealizedScore float64
	QtyTotal      int64
	QtyAvailable  int64
}

// Settlement represents a settlement record.
type Settlement struct {
	ID                    string
	ContestID             string
	Status                string
	StartedAt             sql.NullTime
	PositionsClosedAt     sql.NullTime
	RankingsCalculatedAt  sql.NullTime
	PrizesDistributedAt   sql.NullTime
	CompletedAt           sql.NullTime
	TotalParticipants     int
	TotalPositionsClosed  int
	TotalOrdersCancelled  int
	TotalWinners          int
	PrizePoolGrossCents   int64
	PrizePoolNetCents     int64
	TotalDistributedCents int64
	PlatformFeeCents      int64
	AttemptCount          int
	LastError             sql.NullString
	FailedAt              sql.NullTime
	SnapshotPrices        SnapshotPricesJSON
	SnapshotTakenAt       sql.NullTime
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// getContestInfo retrieves contest details from the database.
func (a *App) getContestInfo(ctx context.Context, contestID string) (*ContestInfo, error) {
	var info ContestInfo
	var platformFeeBps sql.NullInt32
	var name sql.NullString
	var startTime, endTime sql.NullTime

	var prizePoolNetCents, commissionAmount sql.NullInt64

	err := a.db.QueryRowContext(ctx,
		`SELECT id, name, entry_fee_cents, COALESCE(platform_fee_bps, 0),
		        COALESCE(prize_pool_net_cents, 0), COALESCE(commission_amount, 0),
		        starts_at, ends_at, status
		 FROM contests WHERE id = $1`,
		contestID,
	).Scan(&info.ID, &name, &info.EntryFeeCents, &platformFeeBps,
		&prizePoolNetCents, &commissionAmount,
		&startTime, &endTime, &info.Status)

	if err != nil {
		return nil, err
	}

	if name.Valid {
		info.Name = name.String
	} else {
		info.Name = contestID
	}

	if platformFeeBps.Valid {
		info.PlatformFeeBps = int(platformFeeBps.Int32)
	} else {
		info.PlatformFeeBps = a.config.PlatformFeeBps
	}

	if prizePoolNetCents.Valid {
		info.PrizePoolNetCents = prizePoolNetCents.Int64
	}
	if commissionAmount.Valid {
		info.CommissionAmount = commissionAmount.Int64
	}

	if startTime.Valid {
		info.StartTime = startTime.Time
	}
	if endTime.Valid {
		info.EndTime = endTime.Time
	}

	return &info, nil
}

// getSettlementRecord gets existing settlement or creates a new one.
// Does NOT acquire any advisory lock — the caller is responsible for lock management.
func (a *App) getSettlementRecord(ctx context.Context, contestID string) (*Settlement, error) {
	var settlement Settlement

	// Try to get existing settlement
	err := a.db.QueryRowContext(ctx,
		`SELECT id, contest_id, status, started_at, positions_closed_at, rankings_calculated_at,
		        prizes_distributed_at, completed_at, total_participants, total_positions_closed,
		        total_orders_cancelled, total_winners, prize_pool_gross_cents, prize_pool_net_cents,
		        total_distributed_cents, platform_fee_cents, attempt_count, last_error, failed_at,
		        snapshot_prices, snapshot_taken_at, created_at, updated_at
		 FROM contest_settlements WHERE contest_id = $1`,
		contestID,
	).Scan(
		&settlement.ID, &settlement.ContestID, &settlement.Status,
		&settlement.StartedAt, &settlement.PositionsClosedAt, &settlement.RankingsCalculatedAt,
		&settlement.PrizesDistributedAt, &settlement.CompletedAt, &settlement.TotalParticipants,
		&settlement.TotalPositionsClosed, &settlement.TotalOrdersCancelled, &settlement.TotalWinners,
		&settlement.PrizePoolGrossCents, &settlement.PrizePoolNetCents, &settlement.TotalDistributedCents,
		&settlement.PlatformFeeCents, &settlement.AttemptCount, &settlement.LastError,
		&settlement.FailedAt, &settlement.SnapshotPrices, &settlement.SnapshotTakenAt,
		&settlement.CreatedAt, &settlement.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		// Create new settlement with ON CONFLICT for extra safety
		var id string
		err = a.db.QueryRowContext(ctx,
			`INSERT INTO contest_settlements (contest_id, status)
			 VALUES ($1, 'pending')
			 ON CONFLICT (contest_id) DO UPDATE SET updated_at = NOW()
			 RETURNING id`,
			contestID,
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("failed to create settlement: %w", err)
		}

		// Re-fetch the full settlement record
		return a.fetchSettlement(ctx, contestID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get settlement: %w", err)
	}

	return &settlement, nil
}

// fetchSettlement retrieves a settlement record by contest ID.
func (a *App) fetchSettlement(ctx context.Context, contestID string) (*Settlement, error) {
	var settlement Settlement
	err := a.db.QueryRowContext(ctx,
		`SELECT id, contest_id, status, started_at, positions_closed_at, rankings_calculated_at,
		        prizes_distributed_at, completed_at, total_participants, total_positions_closed,
		        total_orders_cancelled, total_winners, prize_pool_gross_cents, prize_pool_net_cents,
		        total_distributed_cents, platform_fee_cents, attempt_count, last_error, failed_at,
		        snapshot_prices, snapshot_taken_at, created_at, updated_at
		 FROM contest_settlements WHERE contest_id = $1`,
		contestID,
	).Scan(
		&settlement.ID, &settlement.ContestID, &settlement.Status,
		&settlement.StartedAt, &settlement.PositionsClosedAt, &settlement.RankingsCalculatedAt,
		&settlement.PrizesDistributedAt, &settlement.CompletedAt, &settlement.TotalParticipants,
		&settlement.TotalPositionsClosed, &settlement.TotalOrdersCancelled, &settlement.TotalWinners,
		&settlement.PrizePoolGrossCents, &settlement.PrizePoolNetCents, &settlement.TotalDistributedCents,
		&settlement.PlatformFeeCents, &settlement.AttemptCount, &settlement.LastError,
		&settlement.FailedAt, &settlement.SnapshotPrices, &settlement.SnapshotTakenAt,
		&settlement.CreatedAt, &settlement.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch settlement: %w", err)
	}
	return &settlement, nil
}

// updateSettlementStatus updates the settlement status.
func (a *App) updateSettlementStatus(ctx context.Context, settlementID string, status string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE contest_settlements SET status = $1::settlement_status WHERE id = $2`,
		status, settlementID,
	)
	return err
}

// updateSettlementStarted marks settlement as started.
func (a *App) updateSettlementStarted(ctx context.Context, settlementID string, totalParticipants int) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE contest_settlements
		 SET status = 'in_progress', started_at = NOW(), total_participants = $2, attempt_count = attempt_count + 1
		 WHERE id = $1`,
		settlementID, totalParticipants,
	)
	return err
}

// updateSettlementPositionsClosed marks positions as closed.
func (a *App) updateSettlementPositionsClosed(ctx context.Context, settlementID string, positionsClosed, ordersCancelled int, snapshotPrices map[string]contracts.Price) error {
	pricesJSON, _ := json.Marshal(snapshotPrices)
	_, err := a.db.ExecContext(ctx,
		`UPDATE contest_settlements
		 SET positions_closed_at = NOW(), total_positions_closed = $2, total_orders_cancelled = $3,
		     snapshot_prices = $4, snapshot_taken_at = NOW()
		 WHERE id = $1`,
		settlementID, positionsClosed, ordersCancelled, pricesJSON,
	)
	return err
}

// updateSettlementRankingsCalculated marks rankings as calculated.
func (a *App) updateSettlementRankingsCalculated(ctx context.Context, settlementID string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE contest_settlements SET rankings_calculated_at = NOW() WHERE id = $1`,
		settlementID,
	)
	return err
}

// updateSettlementPrizesCalculated updates prize pool information.
func (a *App) updateSettlementPrizesCalculated(ctx context.Context, settlementID string, prizePoolGross, prizePoolNet, platformFee int64, winnersCount int) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE contest_settlements
		 SET prize_pool_gross_cents = $2, prize_pool_net_cents = $3, platform_fee_cents = $4, total_winners = $5
		 WHERE id = $1`,
		settlementID, prizePoolGross, prizePoolNet, platformFee, winnersCount,
	)
	return err
}

// updateSettlementPrizesDistributed marks prizes as distributed.
func (a *App) updateSettlementPrizesDistributed(ctx context.Context, settlementID string, totalDistributed int64) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE contest_settlements
		 SET prizes_distributed_at = NOW(), total_distributed_cents = $2
		 WHERE id = $1`,
		settlementID, totalDistributed,
	)
	return err
}

// updateSettlementCompleted marks settlement as completed.
func (a *App) updateSettlementCompleted(ctx context.Context, settlementID string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE contest_settlements SET status = 'completed', completed_at = NOW() WHERE id = $1`,
		settlementID,
	)
	return err
}

// updateSettlementFailed marks settlement as failed.
func (a *App) updateSettlementFailed(ctx context.Context, settlementID string, errMsg string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE contest_settlements SET status = 'failed', last_error = $2, failed_at = NOW() WHERE id = $1`,
		settlementID, errMsg,
	)
	return err
}

// updateContestStatus updates the contest status.
func (a *App) updateContestStatus(ctx context.Context, contestID string, status string) error {
	_, err := a.db.ExecContext(ctx,
		`UPDATE contests SET status = $1::contest_status WHERE id = $2`,
		status, contestID,
	)
	return err
}

// getParticipants retrieves all participants for a contest.
func (a *App) getParticipants(ctx context.Context, contestID string) ([]Participant, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT user_id, total_score, COALESCE(
			(SELECT SUM(realized_score) FROM positions WHERE contest_id = cp.contest_id AND user_id = cp.user_id),
			0
		) as realized_score, qty_total, qty_available
		 FROM contest_participants cp
		 WHERE contest_id = $1`,
		contestID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []Participant
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.UserID, &p.FinalScore, &p.RealizedScore, &p.QtyTotal, &p.QtyAvailable); err != nil {
			a.log().Warn("Failed to scan participant row, skipping",
				zap.String("contest_id", contestID),
				zap.Error(err))
			continue
		}
		participants = append(participants, p)
	}

	return participants, rows.Err()
}

// getOpenPositionsCount returns the count of open positions for a contest.
func (a *App) getOpenPositionsCount(ctx context.Context, contestID string) (int, error) {
	var count int
	err := a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM positions WHERE contest_id = $1 AND closed_at IS NULL`,
		contestID,
	).Scan(&count)
	return count, err
}

// getPendingOrdersCount returns the count of pending orders for a contest.
func (a *App) getPendingOrdersCount(ctx context.Context, contestID string) (int, error) {
	var count int
	err := a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM orders WHERE contest_id = $1 AND status IN ('pending', 'open')`,
		contestID,
	).Scan(&count)
	return count, err
}

// getContestSymbols returns distinct symbols that have positions in a contest.
func (a *App) getContestSymbols(ctx context.Context, contestID string) ([]string, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT DISTINCT symbol FROM positions WHERE contest_id = $1`,
		contestID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			continue
		}
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}

// getUserTradeStats retrieves trade statistics for a user in a contest.
func (a *App) getUserTradeStats(ctx context.Context, contestID, userID string) (totalTrades int, winningTrades int, err error) {
	// Count total filled trades
	err = a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM orders WHERE contest_id = $1 AND user_id = $2 AND status = 'filled'`,
		contestID, userID,
	).Scan(&totalTrades)
	if err != nil {
		return 0, 0, err
	}

	// Count winning trades (positions with positive realized_score)
	err = a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM positions WHERE contest_id = $1 AND user_id = $2 AND realized_score > 0`,
		contestID, userID,
	).Scan(&winningTrades)
	if err != nil {
		return 0, 0, err
	}

	return totalTrades, winningTrades, nil
}

// insertFinalRanking inserts a final ranking record.
func (a *App) insertFinalRanking(ctx context.Context, tx *sql.Tx, settlementID, contestID string, ranking contracts.FinalRanking) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO final_rankings (settlement_id, contest_id, user_id, rank, tied_with_count,
		                             final_score, realized_score, total_trades, winning_trades,
		                             win_rate, tragge_point_contribution)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (contest_id, user_id) DO UPDATE SET
		     rank = EXCLUDED.rank,
		     tied_with_count = EXCLUDED.tied_with_count,
		     final_score = EXCLUDED.final_score,
		     realized_score = EXCLUDED.realized_score,
		     total_trades = EXCLUDED.total_trades,
		     winning_trades = EXCLUDED.winning_trades,
		     win_rate = EXCLUDED.win_rate,
		     tragge_point_contribution = EXCLUDED.tragge_point_contribution`,
		settlementID, contestID, ranking.UserID, ranking.Rank, ranking.TiedWithCount,
		ranking.FinalScore, ranking.RealizedScore, ranking.TotalTrades, ranking.WinningTrades,
		ranking.WinRate, ranking.TraggePointContribution,
	)
	return err
}

// insertPrizeDistribution inserts a prize distribution record.
func (a *App) insertPrizeDistribution(ctx context.Context, tx *sql.Tx, settlementID, contestID string, prize contracts.PrizeAllocation) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO prize_distributions (settlement_id, contest_id, user_id, rank, final_score,
		                                  prize_amount_cents, prize_percentage, status)
		 VALUES ($1, $2, $3, $4,
		         (SELECT final_score FROM final_rankings WHERE contest_id = $2 AND user_id = $3),
		         $5, $6, 'pending')
		 ON CONFLICT (contest_id, user_id) DO UPDATE SET
		     prize_amount_cents = EXCLUDED.prize_amount_cents,
		     prize_percentage = EXCLUDED.prize_percentage,
		     status = 'pending'`,
		settlementID, contestID, prize.UserID, prize.Rank, prize.AmountCents, prize.Percentage,
	)
	return err
}

// updatePrizeDistributionStatus updates the status of a prize distribution.
func (a *App) updatePrizeDistributionStatus(ctx context.Context, tx *sql.Tx, contestID, userID string, status string, ledgerEntryID *string, errMsg *string) error {
	var creditedAt interface{}
	if status == "credited" {
		creditedAt = time.Now()
	}

	_, err := tx.ExecContext(ctx,
		`UPDATE prize_distributions
		 SET status = $3::prize_status, credited_at = $4, ledger_entry_id = $5, error_message = $6
		 WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID, status, creditedAt, ledgerEntryID, errMsg,
	)
	return err
}

// updateParticipantFinalRank updates the final rank for a participant.
func (a *App) updateParticipantFinalRank(ctx context.Context, tx *sql.Tx, contestID, userID string, rank int, prizeCents int64, finalScore float64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE contest_participants
		 SET final_rank = $3, final_prize_cents = $4, total_score = $5
		 WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID, rank, prizeCents, finalScore,
	)
	return err
}

// logSettlementEvent logs a settlement event.
func (a *App) logSettlementEvent(ctx context.Context, settlementID, contestID, eventType string, eventData map[string]interface{}, errMsg *string) error {
	dataJSON, _ := json.Marshal(eventData)
	_, err := a.db.ExecContext(ctx,
		`INSERT INTO settlement_events (settlement_id, contest_id, event_type, event_data, error_message)
		 VALUES ($1, $2, $3, $4, $5)`,
		settlementID, contestID, eventType, dataJSON, errMsg,
	)
	return err
}

// incrementUserTraggePoint atomically increments the T-Point for a user
// and returns the new score. This avoids lost-update races from concurrent settlements.
func (a *App) incrementUserTraggePoint(ctx context.Context, tx *sql.Tx, userID string, contribution float64) (float64, error) {
	var newScore float64
	err := tx.QueryRowContext(ctx,
		`UPDATE users SET tragge_point = COALESCE(tragge_point, 0) + $2 WHERE id = $1 RETURNING tragge_point`,
		userID, contribution,
	).Scan(&newScore)
	return newScore, err
}

// PrizeWinnerInfo holds prize winner details for email notifications.
type PrizeWinnerInfo struct {
	UserID            string
	Email             string
	UserName          string
	FinalRank         int
	PrizeAmountCents  int64
	FinalScore        float64
	TraggePointGain   float64
	TotalParticipants int
}

// getPrizeWinnerEmails retrieves email addresses and details for prize winners.
func (a *App) getPrizeWinnerEmails(ctx context.Context, contestID string, userIDs []string) ([]PrizeWinnerInfo, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}

	// Build placeholders for IN clause
	placeholders := make([]string, len(userIDs))
	args := make([]interface{}, len(userIDs)+1)
	args[0] = contestID
	for i, id := range userIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}

	query := fmt.Sprintf(`
		SELECT u.id, u.email, COALESCE(u.display_name, u.email) as display_name,
		       COALESCE(cp.final_rank, 0) as final_rank,
		       COALESCE(cp.final_prize_cents, 0) as final_prize_cents,
		       COALESCE(cp.total_score, 0) as final_score,
		       COALESCE(fr.tragge_point_contribution, 0) as tragge_contribution
		FROM users u
		JOIN contest_participants cp ON cp.user_id = u.id AND cp.contest_id = $1
		LEFT JOIN final_rankings fr ON fr.user_id = u.id AND fr.contest_id = $1
		WHERE u.id IN (%s)
		ORDER BY cp.final_rank ASC`,
		strings.Join(placeholders, ","),
	)

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var winners []PrizeWinnerInfo
	for rows.Next() {
		var w PrizeWinnerInfo
		if err := rows.Scan(&w.UserID, &w.Email, &w.UserName, &w.FinalRank, &w.PrizeAmountCents, &w.FinalScore, &w.TraggePointGain); err != nil {
			continue
		}
		winners = append(winners, w)
	}

	return winners, rows.Err()
}

// getTotalParticipants returns the total number of participants for a contest.
func (a *App) getTotalParticipants(ctx context.Context, contestID string) (int, error) {
	var count int
	err := a.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1`,
		contestID,
	).Scan(&count)
	return count, err
}

