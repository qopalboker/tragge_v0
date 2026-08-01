// Package integration provides integration tests for the leaderboard-worker finalization recovery.
package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Parsaeffatravesh/tragge/packages/wallet"
)

// FinalizationTestEnv extends TestEnv with finalization-specific helpers.
type FinalizationTestEnv struct {
	*TestEnv
	walletService *wallet.Service
}

// SetupFinalizationTestEnv creates a test environment for finalization tests.
func SetupFinalizationTestEnv(t *testing.T, ctx context.Context) *FinalizationTestEnv {
	t.Helper()
	env := SetupTestEnv(t, ctx)
	return &FinalizationTestEnv{
		TestEnv:       env,
		walletService: wallet.NewService(env.DB),
	}
}

// CreateTestContestWithFees creates a contest with entry fee and platform fee.
func (env *FinalizationTestEnv) CreateTestContestWithFees(
	ctx context.Context,
	t *testing.T,
	name string,
	status string,
	entryFeeCents int64,
	platformFeeBps int,
) string {
	t.Helper()

	var contestID string
	err := env.DB.QueryRowContext(ctx,
		`INSERT INTO contests (name, starts_at, ends_at, status, qty_total, entry_fee_cents, platform_fee_bps)
		 VALUES ($1, NOW() - INTERVAL '2 hours', NOW() - INTERVAL '1 hour', $2, 100000, $3, $4)
		 RETURNING id`,
		name, status, entryFeeCents, platformFeeBps,
	).Scan(&contestID)
	if err != nil {
		t.Fatalf("Failed to create test contest: %v", err)
	}

	return contestID
}

// JoinContestWithScore adds a user to a contest with an initial score.
func (env *FinalizationTestEnv) JoinContestWithScore(
	ctx context.Context,
	t *testing.T,
	contestID, userID string,
	qtyTotal int64,
	totalScore float64,
) {
	t.Helper()

	_, err := env.DB.ExecContext(ctx,
		`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available, total_score)
		 VALUES ($1, $2, $3, $3, $4)`,
		contestID, userID, qtyTotal, totalScore,
	)
	if err != nil {
		t.Fatalf("Failed to join contest: %v", err)
	}
}

// PopulateRedisLeaderboard populates the Redis leaderboard for a contest.
func (env *FinalizationTestEnv) PopulateRedisLeaderboard(
	ctx context.Context,
	t *testing.T,
	contestID string,
	scores map[string]float64,
) {
	t.Helper()

	key := "lb:" + contestID
	for userID, score := range scores {
		err := env.RedisClient.ZAdd(ctx, key, redis.Z{
			Score:  score,
			Member: userID,
		}).Err()
		if err != nil {
			t.Fatalf("Failed to add user %s to leaderboard: %v", userID, err)
		}
	}
}

// GetWalletBalance retrieves a user's wallet balance.
func (env *FinalizationTestEnv) GetWalletBalance(ctx context.Context, t *testing.T, userID string) int64 {
	t.Helper()

	var balance int64
	err := env.DB.QueryRowContext(ctx,
		`SELECT balance_cents FROM wallets WHERE user_id = $1`,
		userID,
	).Scan(&balance)
	if err != nil {
		t.Fatalf("Failed to get wallet balance for user %s: %v", userID, err)
	}
	return balance
}

// GetFinalizationState retrieves the finalization state for a contest.
func (env *FinalizationTestEnv) GetFinalizationState(ctx context.Context, contestID string) (*FinalizationState, error) {
	state := &FinalizationState{}

	err := env.DB.QueryRowContext(ctx, `
		SELECT
			contest_id, finalization_started_at,
			payouts_calculated, payouts_calculated_at,
			ranks_written, ranks_written_at,
			wallets_credited, wallets_credited_at,
			status_updated, status_updated_at,
			finalization_completed_at, error_message, last_error_at, retry_count
		FROM contest_finalization_state
		WHERE contest_id = $1
	`, contestID).Scan(
		&state.ContestID, &state.FinalizationStartedAt,
		&state.PayoutsCalculated, &state.PayoutsCalculatedAt,
		&state.RanksWritten, &state.RanksWrittenAt,
		&state.WalletsCredited, &state.WalletsCreditedat,
		&state.StatusUpdated, &state.StatusUpdatedAt,
		&state.FinalizationCompletedAt, &state.ErrorMessage, &state.LastErrorAt, &state.RetryCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return state, nil
}

// CreateFinalizationState creates a finalization state record for crash recovery simulation.
func (env *FinalizationTestEnv) CreateFinalizationState(
	ctx context.Context,
	t *testing.T,
	contestID string,
	state *FinalizationState,
) {
	t.Helper()

	_, err := env.DB.ExecContext(ctx, `
		INSERT INTO contest_finalization_state (
			contest_id, finalization_started_at,
			payouts_calculated, payouts_calculated_at,
			ranks_written, ranks_written_at,
			wallets_credited, wallets_credited_at,
			status_updated, status_updated_at,
			finalization_completed_at, error_message, retry_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`,
		contestID, state.FinalizationStartedAt,
		state.PayoutsCalculated, state.PayoutsCalculatedAt,
		state.RanksWritten, state.RanksWrittenAt,
		state.WalletsCredited, state.WalletsCreditedat,
		state.StatusUpdated, state.StatusUpdatedAt,
		state.FinalizationCompletedAt, state.ErrorMessage, state.RetryCount,
	)
	if err != nil {
		t.Fatalf("Failed to create finalization state: %v", err)
	}
}

// CountWalletLedgerEntries counts ledger entries for a specific type and contest.
func (env *FinalizationTestEnv) CountWalletLedgerEntries(
	ctx context.Context,
	userID string,
	ledgerType string,
	contestID string,
) (int, error) {
	var count int
	err := env.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wallet_ledger
		 WHERE user_id = $1 AND type = $2 AND ref_id = $3`,
		userID, ledgerType, contestID,
	).Scan(&count)
	return count, err
}

// GetParticipantFinalPrize retrieves the final prize for a participant.
func (env *FinalizationTestEnv) GetParticipantFinalPrize(
	ctx context.Context,
	contestID, userID string,
) (int64, error) {
	var prize sql.NullInt64
	err := env.DB.QueryRowContext(ctx,
		`SELECT final_prize_cents FROM contest_participants
		 WHERE contest_id = $1 AND user_id = $2`,
		contestID, userID,
	).Scan(&prize)
	if err != nil {
		return 0, err
	}
	if !prize.Valid {
		return 0, nil
	}
	return prize.Int64, nil
}

// FinalizationState represents the state of contest finalization.
type FinalizationState struct {
	ContestID               string
	FinalizationStartedAt   time.Time
	PayoutsCalculated       bool
	PayoutsCalculatedAt     sql.NullTime
	RanksWritten            bool
	RanksWrittenAt          sql.NullTime
	WalletsCredited         bool
	WalletsCreditedat       sql.NullTime
	StatusUpdated           bool
	StatusUpdatedAt         sql.NullTime
	FinalizationCompletedAt sql.NullTime
	ErrorMessage            sql.NullString
	LastErrorAt             sql.NullTime
	RetryCount              int
}

// TestFinalizationRecovery contains all finalization recovery integration tests.
func TestFinalizationRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	env := SetupFinalizationTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	t.Run("HappyPath_FullFinalizationCompletes", func(t *testing.T) {
		testHappyPathFinalization(t, ctx, env)
	})

	t.Run("CrashAfterRanksWritten_NoWalletCredit", func(t *testing.T) {
		testCrashAfterRanksWritten(t, ctx, env)
	})

	t.Run("CrashAfterPartialWalletCredit", func(t *testing.T) {
		testCrashAfterPartialWalletCredit(t, ctx, env)
	})

	t.Run("ConcurrentFinalizationAttempts", func(t *testing.T) {
		testConcurrentFinalization(t, ctx, env)
	})

	t.Run("IdempotencyKeyCollision", func(t *testing.T) {
		testIdempotencyKeyCollision(t, ctx, env)
	})
}

// testHappyPathFinalization tests the happy path where full finalization completes successfully.
func testHappyPathFinalization(t *testing.T, ctx context.Context, env *FinalizationTestEnv) {
	// Create a contest with entry fee
	contestID := env.CreateTestContestWithFees(ctx, t, "Happy Path Contest", "running", 1000, 1700) // $10 entry, 17% platform fee

	// Create 10 participants with different scores
	userIDs := make([]string, 10)
	scores := make(map[string]float64)
	for i := 0; i < 10; i++ {
		email := fmt.Sprintf("happy_user_%d_%s@test.com", i, uuid.New().String()[:8])
		userIDs[i] = env.CreateTestUser(ctx, t, email, "hash")
		score := float64(100 - i*10) // Scores: 100, 90, 80, ..., 10
		scores[userIDs[i]] = score
		env.JoinContestWithScore(ctx, t, contestID, userIDs[i], 100000, score)
	}

	// Populate Redis leaderboard
	env.PopulateRedisLeaderboard(ctx, t, contestID, scores)

	// Get initial wallet balances (should all be 0 due to auto-create trigger)
	initialBalances := make(map[string]int64)
	for _, userID := range userIDs {
		initialBalances[userID] = env.GetWalletBalance(ctx, t, userID)
		require.Equal(t, int64(0), initialBalances[userID], "Initial balance should be 0")
	}

	// Simulate finalization by crediting prizes
	// Prize pool: 10 participants * $10 = $100 gross
	// Net after 17% fee: $83
	// Winners (30% of 10 = 3): Rank 1, 2, 3

	// Manually simulate the finalization process
	tx, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Credit prizes to top 3 winners
	// Using typical distribution: Rank 1: 50%, Rank 2: 30%, Rank 3: 20% of net pool
	netPool := int64(8300) // $83 in cents
	prizes := map[int]int64{
		0: netPool * 50 / 100, // Rank 1: $41.50
		1: netPool * 30 / 100, // Rank 2: $24.90
		2: netPool * 20 / 100, // Rank 3: $16.60
	}

	for rank := 0; rank < 3; rank++ {
		prize := prizes[rank]
		_, err := env.walletService.CreditPrizeIdempotent(ctx, tx, userIDs[rank], contestID, rank+1, prize)
		require.NoError(t, err)
	}

	err = tx.Commit()
	require.NoError(t, err)

	// Verify wallet balances
	for i := 0; i < 10; i++ {
		balance := env.GetWalletBalance(ctx, t, userIDs[i])
		if i < 3 {
			// Winners should have prize money
			assert.Equal(t, prizes[i], balance,
				"Winner %d should have correct prize balance", i+1)
		} else {
			// Non-winners should have 0
			assert.Equal(t, int64(0), balance,
				"Non-winner %d should have 0 balance", i+1)
		}
	}

	// Verify ledger entries
	for i := 0; i < 3; i++ {
		count, err := env.CountWalletLedgerEntries(ctx, userIDs[i], "prize_credit", contestID)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Winner %d should have exactly 1 prize_credit ledger entry", i+1)
	}

	t.Log("Happy path finalization completed successfully")
}

// testCrashAfterRanksWritten tests recovery after crash when ranks are written but wallets not credited.
func testCrashAfterRanksWritten(t *testing.T, ctx context.Context, env *FinalizationTestEnv) {
	contestID := env.CreateTestContestWithFees(ctx, t, "Crash After Ranks Contest", "running", 1000, 1700)

	// Create 5 participants
	userIDs := make([]string, 5)
	scores := make(map[string]float64)
	for i := 0; i < 5; i++ {
		email := fmt.Sprintf("crash_ranks_user_%d_%s@test.com", i, uuid.New().String()[:8])
		userIDs[i] = env.CreateTestUser(ctx, t, email, "hash")
		score := float64(50 - i*10)
		scores[userIDs[i]] = score
		env.JoinContestWithScore(ctx, t, contestID, userIDs[i], 100000, score)
	}

	env.PopulateRedisLeaderboard(ctx, t, contestID, scores)

	// Simulate crash after ranks written but before wallet credits
	// Create finalization state showing ranks_written = true, wallets_credited = false
	crashState := &FinalizationState{
		ContestID:             contestID,
		FinalizationStartedAt: time.Now().UTC().Add(-5 * time.Minute),
		PayoutsCalculated:     true,
		PayoutsCalculatedAt:   sql.NullTime{Time: time.Now().UTC().Add(-4 * time.Minute), Valid: true},
		RanksWritten:          true,
		RanksWrittenAt:        sql.NullTime{Time: time.Now().UTC().Add(-3 * time.Minute), Valid: true},
		WalletsCredited:       false, // Not credited yet - simulating crash
		StatusUpdated:         false,
		RetryCount:            0,
	}
	env.CreateFinalizationState(ctx, t, contestID, crashState)

	// Update contest_participants with final ranks (simulating the partial work done before crash)
	for i := 0; i < 5; i++ {
		prize := int64(0)
		if i < 2 { // Top 2 winners (30% of 5 = 1.5, rounds up to 2)
			prize = int64(2075 - i*830) // Simple prize distribution
		}
		_, err := env.DB.ExecContext(ctx,
			`UPDATE contest_participants SET final_rank = $1, final_prize_cents = $2
			 WHERE contest_id = $3 AND user_id = $4`,
			i+1, prize, contestID, userIDs[i])
		require.NoError(t, err)
	}

	// Verify no wallet credits exist yet
	for _, userID := range userIDs {
		balance := env.GetWalletBalance(ctx, t, userID)
		assert.Equal(t, int64(0), balance, "No wallet credits should exist before recovery")
	}

	// Now simulate recovery - credit prizes idempotently
	tx, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Credit prizes to winners
	prizes := map[int]int64{
		0: 2075, // Rank 1
		1: 1245, // Rank 2
	}
	for rank := 0; rank < 2; rank++ {
		prize := prizes[rank]
		_, err := env.walletService.CreditPrizeIdempotent(ctx, tx, userIDs[rank], contestID, rank+1, prize)
		require.NoError(t, err)
	}

	err = tx.Commit()
	require.NoError(t, err)

	// Verify wallet balances after recovery
	for i := 0; i < 5; i++ {
		balance := env.GetWalletBalance(ctx, t, userIDs[i])
		if i < 2 {
			assert.Equal(t, prizes[i], balance, "Winner %d should have correct prize after recovery", i+1)
		} else {
			assert.Equal(t, int64(0), balance, "Non-winner should have 0 balance")
		}
	}

	// Now simulate a second recovery attempt (idempotency test)
	tx2, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	for rank := 0; rank < 2; rank++ {
		prize := prizes[rank]
		entry, err := env.walletService.CreditPrizeIdempotent(ctx, tx2, userIDs[rank], contestID, rank+1, prize)

		// Should return DuplicatePrizeCreditError
		var dupErr *wallet.DuplicatePrizeCreditError
		assert.True(t, errors.As(err, &dupErr), "Should get DuplicatePrizeCreditError on retry")
		assert.NotNil(t, entry, "Should return existing entry on duplicate")
	}

	err = tx2.Commit()
	require.NoError(t, err)

	// Verify balances haven't doubled
	for i := 0; i < 2; i++ {
		balance := env.GetWalletBalance(ctx, t, userIDs[i])
		assert.Equal(t, prizes[i], balance, "Balance should not have doubled on retry for winner %d", i+1)

		// Verify exactly 1 ledger entry per winner
		count, err := env.CountWalletLedgerEntries(ctx, userIDs[i], "prize_credit", contestID)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Should have exactly 1 ledger entry for winner %d", i+1)
	}

	t.Log("Crash after ranks written recovery completed successfully")
}

// testCrashAfterPartialWalletCredit tests recovery after crash when some winners are credited.
func testCrashAfterPartialWalletCredit(t *testing.T, ctx context.Context, env *FinalizationTestEnv) {
	contestID := env.CreateTestContestWithFees(ctx, t, "Partial Credit Contest", "running", 1000, 1700)

	// Create 10 participants (will have 3 winners)
	userIDs := make([]string, 10)
	scores := make(map[string]float64)
	for i := 0; i < 10; i++ {
		email := fmt.Sprintf("partial_credit_user_%d_%s@test.com", i, uuid.New().String()[:8])
		userIDs[i] = env.CreateTestUser(ctx, t, email, "hash")
		score := float64(100 - i*10)
		scores[userIDs[i]] = score
		env.JoinContestWithScore(ctx, t, contestID, userIDs[i], 100000, score)
	}

	env.PopulateRedisLeaderboard(ctx, t, contestID, scores)

	// Define prizes for top 3
	prizes := map[int]int64{
		0: 4150, // Rank 1: ~50%
		1: 2490, // Rank 2: ~30%
		2: 1660, // Rank 3: ~20%
	}

	// Simulate partial credit - only first winner credited before crash
	tx1, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Credit only first winner
	_, err = env.walletService.CreditPrizeIdempotent(ctx, tx1, userIDs[0], contestID, 1, prizes[0])
	require.NoError(t, err)

	err = tx1.Commit()
	require.NoError(t, err)

	// Verify partial state
	balance0 := env.GetWalletBalance(ctx, t, userIDs[0])
	assert.Equal(t, prizes[0], balance0, "First winner should be credited")
	balance1 := env.GetWalletBalance(ctx, t, userIDs[1])
	assert.Equal(t, int64(0), balance1, "Second winner should NOT be credited yet")
	balance2 := env.GetWalletBalance(ctx, t, userIDs[2])
	assert.Equal(t, int64(0), balance2, "Third winner should NOT be credited yet")

	// Create finalization state showing partial progress
	partialState := &FinalizationState{
		ContestID:             contestID,
		FinalizationStartedAt: time.Now().UTC().Add(-5 * time.Minute),
		PayoutsCalculated:     true,
		PayoutsCalculatedAt:   sql.NullTime{Time: time.Now().UTC().Add(-4 * time.Minute), Valid: true},
		RanksWritten:          true,
		RanksWrittenAt:        sql.NullTime{Time: time.Now().UTC().Add(-3 * time.Minute), Valid: true},
		WalletsCredited:       false, // Not fully credited - crashed mid-way
		StatusUpdated:         false,
		RetryCount:            1, // Retry count incremented on recovery
	}
	env.CreateFinalizationState(ctx, t, contestID, partialState)

	// Simulate recovery - credit all 3 winners (first one idempotently)
	tx2, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	for rank := 0; rank < 3; rank++ {
		prize := prizes[rank]
		_, err := env.walletService.CreditPrizeIdempotent(ctx, tx2, userIDs[rank], contestID, rank+1, prize)

		if rank == 0 {
			// First winner should get DuplicatePrizeCreditError (already credited)
			var dupErr *wallet.DuplicatePrizeCreditError
			assert.True(t, errors.As(err, &dupErr),
				"First winner should get DuplicatePrizeCreditError on retry")
		} else {
			// Other winners should be credited successfully
			assert.NoError(t, err, "Winner %d should be credited successfully", rank+1)
		}
	}

	err = tx2.Commit()
	require.NoError(t, err)

	// Verify final state - all 3 winners should have correct prizes
	for i := 0; i < 3; i++ {
		balance := env.GetWalletBalance(ctx, t, userIDs[i])
		assert.Equal(t, prizes[i], balance, "Winner %d should have correct final balance", i+1)

		// Verify exactly 1 ledger entry per winner
		count, err := env.CountWalletLedgerEntries(ctx, userIDs[i], "prize_credit", contestID)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Winner %d should have exactly 1 ledger entry", i+1)
	}

	// Verify non-winners have 0 balance
	for i := 3; i < 10; i++ {
		balance := env.GetWalletBalance(ctx, t, userIDs[i])
		assert.Equal(t, int64(0), balance, "Non-winner %d should have 0 balance", i+1)
	}

	t.Log("Crash after partial wallet credit recovery completed successfully")
}

// testConcurrentFinalization tests that concurrent finalization attempts are handled safely.
func testConcurrentFinalization(t *testing.T, ctx context.Context, env *FinalizationTestEnv) {
	contestID := env.CreateTestContestWithFees(ctx, t, "Concurrent Contest", "running", 1000, 1700)

	// Create 5 participants
	userIDs := make([]string, 5)
	scores := make(map[string]float64)
	for i := 0; i < 5; i++ {
		email := fmt.Sprintf("concurrent_user_%d_%s@test.com", i, uuid.New().String()[:8])
		userIDs[i] = env.CreateTestUser(ctx, t, email, "hash")
		score := float64(50 - i*10)
		scores[userIDs[i]] = score
		env.JoinContestWithScore(ctx, t, contestID, userIDs[i], 100000, score)
	}

	env.PopulateRedisLeaderboard(ctx, t, contestID, scores)

	prize := int64(2075) // Prize for winner

	// Track successful credits
	var successCount int32
	var duplicateCount int32
	var errorCount int32
	var wg sync.WaitGroup

	// Launch 5 concurrent goroutines trying to credit the same winner
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			// Each goroutine creates its own transaction
			tx, err := env.DB.BeginTx(ctx, nil)
			if err != nil {
				atomic.AddInt32(&errorCount, 1)
				t.Logf("Goroutine %d: Failed to begin tx: %v", goroutineID, err)
				return
			}

			_, err = env.walletService.CreditPrizeIdempotent(ctx, tx, userIDs[0], contestID, 1, prize)

			if err == nil {
				// First successful credit
				if commitErr := tx.Commit(); commitErr != nil {
					atomic.AddInt32(&errorCount, 1)
					t.Logf("Goroutine %d: Commit failed: %v", goroutineID, commitErr)
				} else {
					atomic.AddInt32(&successCount, 1)
					t.Logf("Goroutine %d: Successfully credited prize", goroutineID)
				}
			} else {
				var dupErr *wallet.DuplicatePrizeCreditError
				if errors.As(err, &dupErr) {
					// Duplicate credit detected - this is expected for concurrent attempts
					atomic.AddInt32(&duplicateCount, 1)
					t.Logf("Goroutine %d: Duplicate detected (idempotent success)", goroutineID)
					tx.Rollback()
				} else {
					// Unexpected error
					atomic.AddInt32(&errorCount, 1)
					t.Logf("Goroutine %d: Unexpected error: %v", goroutineID, err)
					tx.Rollback()
				}
			}
		}(g)
	}

	wg.Wait()

	// Verify results
	t.Logf("Success: %d, Duplicates: %d, Errors: %d", successCount, duplicateCount, errorCount)

	// Exactly one should succeed as the first credit
	assert.Equal(t, int32(1), successCount, "Exactly one credit should succeed initially")

	// Others should be duplicates (idempotent successes)
	assert.Equal(t, int32(4), duplicateCount, "Other attempts should be duplicates")

	// No errors expected
	assert.Equal(t, int32(0), errorCount, "No errors expected")

	// Verify final wallet balance is correct (not multiplied)
	balance := env.GetWalletBalance(ctx, t, userIDs[0])
	assert.Equal(t, prize, balance, "Balance should be exactly the prize amount, not multiplied")

	// Verify exactly 1 ledger entry
	count, err := env.CountWalletLedgerEntries(ctx, userIDs[0], "prize_credit", contestID)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly 1 ledger entry despite concurrent attempts")

	t.Log("Concurrent finalization test completed successfully")
}

// testIdempotencyKeyCollision tests that duplicate CreditPrize calls with same idempotency key are ignored.
func testIdempotencyKeyCollision(t *testing.T, ctx context.Context, env *FinalizationTestEnv) {
	contestID := env.CreateTestContestWithFees(ctx, t, "Idempotency Contest", "running", 1000, 1700)

	// Create a single participant
	email := fmt.Sprintf("idempotency_user_%s@test.com", uuid.New().String()[:8])
	userID := env.CreateTestUser(ctx, t, email, "hash")
	env.JoinContestWithScore(ctx, t, contestID, userID, 100000, 100.0)

	scores := map[string]float64{userID: 100.0}
	env.PopulateRedisLeaderboard(ctx, t, contestID, scores)

	prize := int64(5000)
	rank := 1

	// First credit
	tx1, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	entry1, err := env.walletService.CreditPrizeIdempotent(ctx, tx1, userID, contestID, rank, prize)
	require.NoError(t, err)
	require.NotNil(t, entry1)

	err = tx1.Commit()
	require.NoError(t, err)

	// Verify first credit
	balance1 := env.GetWalletBalance(ctx, t, userID)
	assert.Equal(t, prize, balance1, "First credit should be applied")

	// Attempt same credit again (should be idempotent)
	tx2, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	entry2, err := env.walletService.CreditPrizeIdempotent(ctx, tx2, userID, contestID, rank, prize)

	// Should get DuplicatePrizeCreditError
	var dupErr *wallet.DuplicatePrizeCreditError
	require.True(t, errors.As(err, &dupErr), "Should get DuplicatePrizeCreditError")
	require.NotNil(t, entry2, "Should return existing entry")

	// Verify idempotency key matches
	expectedKey := wallet.GeneratePrizeIdempotencyKey(contestID, userID, rank)
	assert.Equal(t, expectedKey, dupErr.IdempotencyKey, "Idempotency key should match")

	tx2.Rollback()

	// Verify balance hasn't changed
	balance2 := env.GetWalletBalance(ctx, t, userID)
	assert.Equal(t, prize, balance2, "Balance should remain unchanged after idempotent call")

	// Verify exactly 1 ledger entry
	count, err := env.CountWalletLedgerEntries(ctx, userID, "prize_credit", contestID)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Should have exactly 1 ledger entry")

	// Test with different prize amount (same idempotency key should still be rejected)
	tx3, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	differentPrize := int64(10000) // Different amount, same key
	entry3, err := env.walletService.CreditPrizeIdempotent(ctx, tx3, userID, contestID, rank, differentPrize)

	require.True(t, errors.As(err, &dupErr), "Should still get DuplicatePrizeCreditError with different amount")
	require.NotNil(t, entry3, "Should return existing entry")

	tx3.Rollback()

	// Balance should still be original amount
	balance3 := env.GetWalletBalance(ctx, t, userID)
	assert.Equal(t, prize, balance3, "Balance should remain the original prize amount")

	t.Log("Idempotency key collision test completed successfully")
}

// TestIdempotencyKeyGeneration tests the idempotency key generation.
func TestIdempotencyKeyGeneration(t *testing.T) {
	testCases := []struct {
		name      string
		contestID string
		userID    string
		rank      int
		expected  string
	}{
		{
			name:      "standard case",
			contestID: "contest-123",
			userID:    "user-456",
			rank:      1,
			expected:  "finalization:contest-123:user-456:1",
		},
		{
			name:      "rank 10",
			contestID: "abc",
			userID:    "xyz",
			rank:      10,
			expected:  "finalization:abc:xyz:10",
		},
		{
			name:      "UUID format",
			contestID: "550e8400-e29b-41d4-a716-446655440000",
			userID:    "7c9e6679-7425-40de-944b-e07fc1f90ae7",
			rank:      5,
			expected:  "finalization:550e8400-e29b-41d4-a716-446655440000:7c9e6679-7425-40de-944b-e07fc1f90ae7:5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := wallet.GeneratePrizeIdempotencyKey(tc.contestID, tc.userID, tc.rank)
			assert.Equal(t, tc.expected, key)
		})
	}

	// Test determinism - same inputs should always produce same key
	t.Run("deterministic", func(t *testing.T) {
		contestID := "test-contest"
		userID := "test-user"
		rank := 3

		key1 := wallet.GeneratePrizeIdempotencyKey(contestID, userID, rank)
		key2 := wallet.GeneratePrizeIdempotencyKey(contestID, userID, rank)
		assert.Equal(t, key1, key2, "Same inputs should produce same key")
	})

	// Different ranks should produce different keys
	t.Run("different ranks different keys", func(t *testing.T) {
		contestID := "test-contest"
		userID := "test-user"

		key1 := wallet.GeneratePrizeIdempotencyKey(contestID, userID, 1)
		key2 := wallet.GeneratePrizeIdempotencyKey(contestID, userID, 2)
		assert.NotEqual(t, key1, key2, "Different ranks should produce different keys")
	})
}
