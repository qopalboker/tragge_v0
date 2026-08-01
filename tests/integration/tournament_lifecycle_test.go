package integration

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Parsaeffatravesh/tragge/packages/scoring/prize"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
)

// ---------------------------------------------------------------------------
// TestTournamentLifecycle_QuickTournament_FullCycle
//
// End-to-end test that drives a contest through its complete lifecycle:
//
//   Step 1  – Create contest in draft state
//   Step 2  – Transition draft → scheduled → registration_open
//   Step 3  – Users join (fund wallet, deduct entry fee, join)
//   Step 4  – Verify prize pool is locked at correct amounts
//   Step 5  – Transition to running
//   Step 6  – Populate leaderboard with final scores
//   Step 7  – Transition to settling, create settlement record
//   Step 8  – Calculate prizes and write final_rankings + prize_distributions
//   Step 9  – Credit wallets (idempotently)
//   Step 10 – Verify final state: contest completed, wallets correct,
//             settlement record matches, audit trail complete
// ---------------------------------------------------------------------------

func TestTournamentLifecycle_QuickTournament_FullCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	baseEnv := SetupTestEnv(t, ctx)
	defer baseEnv.Cleanup(t, ctx)

	walletSvc := wallet.NewService(baseEnv.DB)

	const (
		numUsers       = 10
		entryFeeCents  = 50000  // 500.00 in major units
		commissionRate = 0.20   // 20%
		platformFeeBps = 2000   // 20% in basis points
		qtyTotal       = 100000
	)

	// ---------------------------------------------------------------
	// Step 1: Create contest in draft state
	// ---------------------------------------------------------------
	t.Log("Step 1: Creating contest in draft state")

	var contestID string
	err := baseEnv.DB.QueryRowContext(ctx,
		`INSERT INTO contests (
			name, starts_at, ends_at, status, qty_total,
			entry_fee_cents, platform_fee_bps
		) VALUES (
			$1,
			NOW() + INTERVAL '1 hour',
			NOW() + INTERVAL '2 hours',
			'draft',
			$2, $3, $4
		) RETURNING id`,
		"Lifecycle E2E Contest", qtyTotal, entryFeeCents, platformFeeBps,
	).Scan(&contestID)
	require.NoError(t, err, "failed to create contest")
	t.Logf("  Contest created: %s", contestID)

	// Record the initial status history entry
	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO contest_status_history (id, contest_id, from_status, to_status, reason)
		 VALUES ($1, $2, NULL, 'draft', 'Contest created')`,
		uuid.NewString(), contestID,
	)
	require.NoError(t, err)

	// Verify draft state
	var status string
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT status FROM contests WHERE id = $1`, contestID,
	).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "draft", status)

	// ---------------------------------------------------------------
	// Step 2: Transition draft → scheduled → registration_open
	// ---------------------------------------------------------------
	t.Log("Step 2: Transitioning through status states")

	transitions := []struct {
		from, to, reason string
	}{
		{"draft", "scheduled", "Contest published"},
		{"scheduled", "registration_open", "Registration opened"},
	}

	for _, tr := range transitions {
		_, err = baseEnv.DB.ExecContext(ctx,
			`UPDATE contests SET status = $1 WHERE id = $2`, tr.to, contestID)
		require.NoError(t, err)

		_, err = baseEnv.DB.ExecContext(ctx,
			`INSERT INTO contest_status_history (id, contest_id, from_status, to_status, reason)
			 VALUES ($1, $2, $3, $4, $5)`,
			uuid.NewString(), contestID, tr.from, tr.to, tr.reason,
		)
		require.NoError(t, err)
	}

	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT status FROM contests WHERE id = $1`, contestID,
	).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "registration_open", status)

	// ---------------------------------------------------------------
	// Step 3: Users join (fund, deduct entry fee, join)
	// ---------------------------------------------------------------
	t.Log("Step 3: Creating users and joining contest")

	type participant struct {
		userID string
		score  float64
	}
	participants := make([]participant, numUsers)

	for i := 0; i < numUsers; i++ {
		email := fmt.Sprintf("lifecycle-user-%d-%s@test.com", i, uuid.NewString()[:8])
		userID := baseEnv.CreateTestUser(ctx, t, email, "$argon2id$test")
		// Score: 1000, 900, 800, ..., 100
		participants[i] = participant{userID: userID, score: float64((numUsers - i) * 100)}

		// Fund wallet
		tx, err := baseEnv.DB.BeginTx(ctx, nil)
		require.NoError(t, err)
		refType := wallet.LedgerRefTypeContest
		refID := "deposit-" + uuid.NewString()
		desc := "Test deposit"
		_, err = walletSvc.Credit(ctx, tx, userID, entryFeeCents*2, wallet.LedgerTypeDeposit, &refType, &refID, &desc)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		// Deduct entry fee
		tx, err = baseEnv.DB.BeginTx(ctx, nil)
		require.NoError(t, err)
		_, err = walletSvc.DeductContestEntryFeeWithName(ctx, tx, userID, contestID, "Lifecycle E2E Contest", entryFeeCents)
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		// Join contest
		baseEnv.JoinContest(ctx, t, contestID, userID, int64(qtyTotal))
	}

	// Verify participant count
	var participantCount int
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1`, contestID,
	).Scan(&participantCount)
	require.NoError(t, err)
	assert.Equal(t, numUsers, participantCount)

	// ---------------------------------------------------------------
	// Step 4: Verify prize pool calculations
	// ---------------------------------------------------------------
	t.Log("Step 4: Verifying prize pool calculations")

	grossPool := int64(numUsers) * entryFeeCents
	netPool, err := prize.CalculatePrizePool(numUsers, entryFeeCents, commissionRate)
	require.NoError(t, err, "CalculatePrizePool should not error")
	platformFee := grossPool - netPool
	winnersCount := prize.GetWinnersCount(numUsers)

	assert.Equal(t, int64(500000), grossPool, "gross pool = 10 * 50000")
	assert.Equal(t, int64(400000), netPool, "net pool = 500000 - 20%")
	assert.Equal(t, int64(100000), platformFee, "platform fee = 20%")
	assert.Equal(t, 3, winnersCount, "winners = ceil(10 * 0.30)")

	// Lock the prize pool (simulating what happens at contest start)
	slots := prize.CalculatePrizeDistribution(numUsers, netPool)
	require.Len(t, slots, winnersCount)

	// Verify cent-perfect distribution
	var totalSlotCents int64
	for _, s := range slots {
		totalSlotCents += s.AmountCents
	}
	assert.Equal(t, netPool, totalSlotCents, "slot sum must equal net pool exactly")

	// Insert prize lock record
	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO contest_prize_locks (
			id, contest_id, total_participants,
			prize_pool_gross_cents, prize_pool_net_cents, platform_fee_cents,
			commission_rate, winners_count, distribution_json, locked_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, NOW())`,
		uuid.NewString(), contestID, numUsers,
		grossPool, netPool, platformFee,
		commissionRate*100, winnersCount, "[]",
	)
	require.NoError(t, err)

	// ---------------------------------------------------------------
	// Step 5: Transition to running
	// ---------------------------------------------------------------
	t.Log("Step 5: Transitioning to running")

	_, err = baseEnv.DB.ExecContext(ctx,
		`UPDATE contests SET status = 'running', started_at = NOW() WHERE id = $1`, contestID)
	require.NoError(t, err)

	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO contest_status_history (id, contest_id, from_status, to_status, reason)
		 VALUES ($1, $2, 'registration_open', 'running', 'Contest started')`,
		uuid.NewString(), contestID,
	)
	require.NoError(t, err)

	// ---------------------------------------------------------------
	// Step 6: Populate leaderboard with final scores
	// ---------------------------------------------------------------
	t.Log("Step 6: Populating Redis leaderboard with scores")

	lbKey := "lb:" + contestID
	for _, p := range participants {
		err = baseEnv.RedisClient.ZAdd(ctx, lbKey, redis.Z{Score: p.score, Member: p.userID}).Err()
		require.NoError(t, err)
	}

	// Verify leaderboard
	members, err := baseEnv.RedisClient.ZRevRangeWithScores(ctx, lbKey, 0, -1).Result()
	require.NoError(t, err)
	assert.Len(t, members, numUsers)
	// Top scorer should be first
	assert.Equal(t, float64(1000), members[0].Score)
	assert.Equal(t, participants[0].userID, members[0].Member.(string))

	// ---------------------------------------------------------------
	// Step 7: Transition to settling, create settlement record
	// ---------------------------------------------------------------
	t.Log("Step 7: Transitioning to settling and creating settlement")

	_, err = baseEnv.DB.ExecContext(ctx,
		`UPDATE contests SET status = 'settling', ended_at = NOW() WHERE id = $1`, contestID)
	require.NoError(t, err)

	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO contest_status_history (id, contest_id, from_status, to_status, reason)
		 VALUES ($1, $2, 'running', 'settling', 'Trading period ended')`,
		uuid.NewString(), contestID,
	)
	require.NoError(t, err)

	// Create settlement record
	settlementID := uuid.NewString()
	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO contest_settlements (
			id, contest_id, status, started_at,
			total_participants, total_winners,
			prize_pool_gross_cents, prize_pool_net_cents,
			platform_fee_cents, attempt_count
		) VALUES ($1, $2, 'in_progress', NOW(), $3, $4, $5, $6, $7, 1)`,
		settlementID, contestID, numUsers, winnersCount,
		grossPool, netPool, platformFee,
	)
	require.NoError(t, err)

	// Record settlement event: started
	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO settlement_events (id, settlement_id, contest_id, event_type)
		 VALUES ($1, $2, $3, 'started')`,
		uuid.NewString(), settlementID, contestID,
	)
	require.NoError(t, err)

	// ---------------------------------------------------------------
	// Step 8: Calculate prizes and write final_rankings + prize_distributions
	// ---------------------------------------------------------------
	t.Log("Step 8: Writing final rankings and prize distributions")

	// Write final rankings for all participants
	for rank, p := range participants {
		_, err = baseEnv.DB.ExecContext(ctx,
			`INSERT INTO final_rankings (
				id, settlement_id, contest_id, user_id,
				rank, final_score, realized_score, unrealized_score,
				total_trades, winning_trades, total_positions
			) VALUES ($1, $2, $3, $4, $5, $6, $7, 0, 10, 5, 3)`,
			uuid.NewString(), settlementID, contestID, p.userID,
			rank+1, p.score, p.score,
		)
		require.NoError(t, err, "failed to insert ranking for user %d", rank+1)
	}

	// Record settlement event: rankings_calculated
	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO settlement_events (id, settlement_id, contest_id, event_type)
		 VALUES ($1, $2, $3, 'rankings_calculated')`,
		uuid.NewString(), settlementID, contestID,
	)
	require.NoError(t, err)

	_, err = baseEnv.DB.ExecContext(ctx,
		`UPDATE contest_settlements SET rankings_calculated_at = NOW() WHERE id = $1`,
		settlementID,
	)
	require.NoError(t, err)

	// Write prize distributions for winners only
	for i, slot := range slots {
		prizeStatus := "pending"
		_, err = baseEnv.DB.ExecContext(ctx,
			`INSERT INTO prize_distributions (
				id, settlement_id, contest_id, user_id,
				rank, final_score, prize_amount_cents, prize_percentage, status
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			uuid.NewString(), settlementID, contestID, participants[i].userID,
			slot.Rank, participants[i].score, slot.AmountCents, slot.Percentage, prizeStatus,
		)
		require.NoError(t, err, "failed to insert prize distribution for rank %d", slot.Rank)
	}

	// ---------------------------------------------------------------
	// Step 9: Credit wallets (idempotently)
	// ---------------------------------------------------------------
	t.Log("Step 9: Crediting prize wallets")

	var totalDistributed int64
	for i, slot := range slots {
		tx, txErr := baseEnv.DB.BeginTx(ctx, nil)
		require.NoError(t, txErr)

		_, txErr = walletSvc.CreditPrizeIdempotent(ctx, tx, participants[i].userID, contestID, slot.Rank, slot.AmountCents)
		require.NoError(t, txErr)
		require.NoError(t, tx.Commit())

		totalDistributed += slot.AmountCents

		// Update prize_distributions status
		_, err = baseEnv.DB.ExecContext(ctx,
			`UPDATE prize_distributions SET status = 'credited', credited_at = NOW()
			 WHERE contest_id = $1 AND user_id = $2`,
			contestID, participants[i].userID,
		)
		require.NoError(t, err)
	}

	// Record settlement event: prizes_distributed
	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO settlement_events (id, settlement_id, contest_id, event_type)
		 VALUES ($1, $2, $3, 'prizes_distributed')`,
		uuid.NewString(), settlementID, contestID,
	)
	require.NoError(t, err)

	// Update settlement record
	_, err = baseEnv.DB.ExecContext(ctx,
		`UPDATE contest_settlements SET
			prizes_distributed_at = NOW(),
			total_distributed_cents = $1,
			status = 'completed',
			completed_at = NOW()
		 WHERE id = $2`,
		totalDistributed, settlementID,
	)
	require.NoError(t, err)

	// Record settlement event: completed
	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO settlement_events (id, settlement_id, contest_id, event_type)
		 VALUES ($1, $2, $3, 'completed')`,
		uuid.NewString(), settlementID, contestID,
	)
	require.NoError(t, err)

	// ---------------------------------------------------------------
	// Step 10: Final verification
	// ---------------------------------------------------------------
	t.Log("Step 10: Verifying final state")

	// Transition contest to completed
	_, err = baseEnv.DB.ExecContext(ctx,
		`UPDATE contests SET status = 'completed', settled_at = NOW() WHERE id = $1`, contestID)
	require.NoError(t, err)

	_, err = baseEnv.DB.ExecContext(ctx,
		`INSERT INTO contest_status_history (id, contest_id, from_status, to_status, reason)
		 VALUES ($1, $2, 'settling', 'completed', 'Settlement completed')`,
		uuid.NewString(), contestID,
	)
	require.NoError(t, err)

	// 10a: Verify contest final status
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT status FROM contests WHERE id = $1`, contestID,
	).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "completed", status, "contest should be completed")

	// 10b: Verify winner wallets received correct prizes
	for i, slot := range slots {
		var balance int64
		err = baseEnv.DB.QueryRowContext(ctx,
			`SELECT balance_cents FROM wallets WHERE user_id = $1`,
			participants[i].userID,
		).Scan(&balance)
		require.NoError(t, err)

		// Expected: initial deposit (entryFeeCents*2) - entry fee + prize
		expectedBalance := entryFeeCents - entryFeeCents + slot.AmountCents // deposit*2 - entry_fee = entry_fee; wallet auto-creates with deposit
		// Actually: funded with entryFeeCents*2, deducted entryFeeCents → remaining entryFeeCents + prize
		expectedBalance = entryFeeCents + slot.AmountCents
		assert.Equal(t, expectedBalance, balance,
			"winner rank %d should have remaining deposit + prize", slot.Rank)
	}

	// 10c: Verify non-winner wallets only have the remaining deposit
	for i := winnersCount; i < numUsers; i++ {
		var balance int64
		err = baseEnv.DB.QueryRowContext(ctx,
			`SELECT balance_cents FROM wallets WHERE user_id = $1`,
			participants[i].userID,
		).Scan(&balance)
		require.NoError(t, err)
		assert.Equal(t, entryFeeCents, balance,
			"non-winner %d should only have remaining deposit", i+1)
	}

	// 10d: Verify settlement record
	var (
		settlementStatus                         string
		settGross, settNet, settDistributed, settFee sql.NullInt64
		settWinners, settParticipants              sql.NullInt32
		settCompletedAt                            sql.NullTime
	)
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT status, prize_pool_gross_cents, prize_pool_net_cents,
		        total_distributed_cents, platform_fee_cents,
		        total_winners, total_participants, completed_at
		 FROM contest_settlements WHERE contest_id = $1`, contestID,
	).Scan(&settlementStatus, &settGross, &settNet, &settDistributed, &settFee,
		&settWinners, &settParticipants, &settCompletedAt)
	require.NoError(t, err)
	assert.Equal(t, "completed", settlementStatus)
	assert.Equal(t, grossPool, settGross.Int64)
	assert.Equal(t, netPool, settNet.Int64)
	assert.Equal(t, totalDistributed, settDistributed.Int64)
	assert.Equal(t, platformFee, settFee.Int64)
	assert.Equal(t, int32(winnersCount), settWinners.Int32)
	assert.Equal(t, int32(numUsers), settParticipants.Int32)
	assert.True(t, settCompletedAt.Valid, "completed_at should be set")

	// 10e: Verify final rankings count
	var rankingsCount int
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM final_rankings WHERE contest_id = $1`, contestID,
	).Scan(&rankingsCount)
	require.NoError(t, err)
	assert.Equal(t, numUsers, rankingsCount, "should have rankings for all participants")

	// 10f: Verify prize distributions count and all credited
	var prizeDistCount int
	var allCredited bool
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        BOOL_AND(status = 'credited')
		 FROM prize_distributions WHERE contest_id = $1`, contestID,
	).Scan(&prizeDistCount, &allCredited)
	require.NoError(t, err)
	assert.Equal(t, winnersCount, prizeDistCount, "should have prize records for winners only")
	assert.True(t, allCredited, "all prizes should be credited")

	// 10g: Verify total distributed equals net pool
	var totalFromPrizeDist int64
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(prize_amount_cents), 0) FROM prize_distributions WHERE contest_id = $1`,
		contestID,
	).Scan(&totalFromPrizeDist)
	require.NoError(t, err)
	assert.Equal(t, netPool, totalFromPrizeDist, "total prize distributions should equal net pool")

	// 10h: Verify status history audit trail (should have 5 transitions)
	var historyCount int
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contest_status_history WHERE contest_id = $1`, contestID,
	).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 5, historyCount,
		"should have 5 status transitions: draft→scheduled→reg_open→running→settling→completed")

	// Verify the transition sequence
	rows, err := baseEnv.DB.QueryContext(ctx,
		`SELECT to_status FROM contest_status_history
		 WHERE contest_id = $1 ORDER BY created_at`, contestID)
	require.NoError(t, err)
	defer rows.Close()

	expectedStatuses := []string{"draft", "scheduled", "registration_open", "running", "settling", "completed"}
	var actualStatuses []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		actualStatuses = append(actualStatuses, s)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, expectedStatuses, actualStatuses,
		"status transitions should follow the correct lifecycle sequence")

	// 10i: Verify settlement events audit trail
	var eventCount int
	err = baseEnv.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM settlement_events WHERE settlement_id = $1`, settlementID,
	).Scan(&eventCount)
	require.NoError(t, err)
	assert.Equal(t, 4, eventCount,
		"should have 4 settlement events: started, rankings_calculated, prizes_distributed, completed")

	// Verify settlement event sequence
	eventRows, err := baseEnv.DB.QueryContext(ctx,
		`SELECT event_type FROM settlement_events
		 WHERE settlement_id = $1 ORDER BY created_at`, settlementID)
	require.NoError(t, err)
	defer eventRows.Close()

	expectedEvents := []string{"started", "rankings_calculated", "prizes_distributed", "completed"}
	var actualEvents []string
	for eventRows.Next() {
		var e string
		require.NoError(t, eventRows.Scan(&e))
		actualEvents = append(actualEvents, e)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, expectedEvents, actualEvents,
		"settlement events should follow the correct sequence")

	// 10j: Verify commission amount
	assert.Equal(t, netPool, totalDistributed, "distributed amount should equal net pool")
	assert.Equal(t, grossPool, netPool+platformFee, "gross = net + fee")

	t.Logf("Tournament lifecycle E2E test completed successfully!")
	t.Logf("  Contest: %s", contestID)
	t.Logf("  Participants: %d, Winners: %d", numUsers, winnersCount)
	t.Logf("  Gross pool: %d, Net pool: %d, Fee: %d", grossPool, netPool, platformFee)
	t.Logf("  Total distributed: %d", totalDistributed)
}

