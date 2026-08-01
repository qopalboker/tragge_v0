package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Parsaeffatravesh/tragge/packages/scoring/prize"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
)

// PrizePoolTestEnv extends FinalizationTestEnv for prize pool integration tests.
type PrizePoolTestEnv struct {
	*FinalizationTestEnv
}

// SetupPrizePoolTestEnv creates a test environment for prize pool tests.
func SetupPrizePoolTestEnv(t *testing.T, ctx context.Context) *PrizePoolTestEnv {
	t.Helper()
	return &PrizePoolTestEnv{
		FinalizationTestEnv: SetupFinalizationTestEnv(t, ctx),
	}
}

// FundWallet credits a user's wallet with the given amount.
func (env *PrizePoolTestEnv) FundWallet(ctx context.Context, t *testing.T, userID string, amountCents int64) {
	t.Helper()

	tx, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	refType := wallet.LedgerRefTypeContest
	refID := "test-deposit-" + uuid.NewString()
	desc := "Test deposit"
	_, err = env.walletService.Credit(ctx, tx, userID, amountCents, wallet.LedgerTypeDeposit, &refType, &refID, &desc)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

// DeductEntryFee deducts entry fee from user's wallet for a contest.
func (env *PrizePoolTestEnv) DeductEntryFee(ctx context.Context, t *testing.T, userID, contestID string, entryFeeCents int64) {
	t.Helper()

	tx, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	_, err = env.walletService.DeductContestEntryFeeWithName(ctx, tx, userID, contestID, "Test Contest", entryFeeCents)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

// CreditPrize credits a prize to a user's wallet.
func (env *PrizePoolTestEnv) CreditPrize(ctx context.Context, t *testing.T, userID, contestID string, rank int, prizeCents int64) {
	t.Helper()

	tx, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)

	_, err = env.walletService.CreditPrizeIdempotent(ctx, tx, userID, contestID, rank, prizeCents)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

// CreateUsersWithWallets creates n users, each with a funded wallet.
func (env *PrizePoolTestEnv) CreateUsersWithWallets(ctx context.Context, t *testing.T, n int, balanceCents int64) []string {
	t.Helper()
	userIDs := make([]string, n)
	for i := 0; i < n; i++ {
		email := fmt.Sprintf("prize-user-%d-%s@test.com", i, uuid.NewString()[:8])
		userIDs[i] = env.CreateTestUser(ctx, t, email, "$argon2id$test")
		if balanceCents > 0 {
			env.FundWallet(ctx, t, userIDs[i], balanceCents)
		}
	}
	return userIDs
}

// ---------------------------------------------------------------------------
// TestPrizePool_10Users_EntryFee100K
// ---------------------------------------------------------------------------

func TestPrizePool_10Users_EntryFee100K(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := SetupPrizePoolTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	const (
		numUsers        = 10
		entryFeeCents   = 100000
		platformFeeBps  = 2000 // 20%
		commissionRate  = 0.20
	)

	// Create contest
	contestID := env.CreateTestContestWithFees(ctx, t, "Prize Pool 10 Users", "running", entryFeeCents, platformFeeBps)

	// Create 10 users, fund wallets, join contest
	userIDs := env.CreateUsersWithWallets(ctx, t, numUsers, entryFeeCents*2) // extra for fees
	for _, uid := range userIDs {
		env.DeductEntryFee(ctx, t, uid, contestID, entryFeeCents)
		env.JoinContest(ctx, t, contestID, uid, 100000)
	}

	// Verify prize pool calculations
	expectedGross := int64(numUsers) * entryFeeCents
	expectedNet, err := prize.CalculatePrizePool(numUsers, entryFeeCents, commissionRate)
	require.NoError(t, err, "CalculatePrizePool should not error")
	expectedCommission := expectedGross - expectedNet

	assert.Equal(t, int64(1_000_000), expectedGross, "gross pool should be 1,000,000")
	assert.Equal(t, int64(800_000), expectedNet, "net pool should be 800,000")
	assert.Equal(t, int64(200_000), expectedCommission, "commission should be 200,000")

	// Verify wallet deductions total
	var totalDeducted int64
	for _, uid := range userIDs {
		balance := env.GetWalletBalance(ctx, t, uid)
		deducted := (entryFeeCents * 2) - balance
		totalDeducted += deducted
	}
	assert.Equal(t, expectedGross, totalDeducted, "total deducted should equal gross pool")
}

// ---------------------------------------------------------------------------
// TestPrizePool_CommissionAmount
// ---------------------------------------------------------------------------

func TestPrizePool_CommissionAmount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := SetupPrizePoolTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	tests := []struct {
		name           string
		commissionRate float64
		platformFeeBps int
		expectedNet    int64 // for 5 users at 50000 cents each (gross = 250000)
	}{
		{"15_percent", 0.15, 1500, 212500},
		{"20_percent", 0.20, 2000, 200000},
		{"25_percent", 0.25, 2500, 187500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const (
				numUsers      = 5
				entryFeeCents = 50000
			)

			contestID := env.CreateTestContestWithFees(ctx, t, "Commission "+tt.name, "running", entryFeeCents, tt.platformFeeBps)
			userIDs := env.CreateUsersWithWallets(ctx, t, numUsers, entryFeeCents*2)

			for _, uid := range userIDs {
				env.DeductEntryFee(ctx, t, uid, contestID, entryFeeCents)
				env.JoinContest(ctx, t, contestID, uid, 100000)
			}

			netPool, poolErr := prize.CalculatePrizePool(numUsers, entryFeeCents, tt.commissionRate)
			require.NoError(t, poolErr, "CalculatePrizePool should not error for %s", tt.name)
			assert.Equal(t, tt.expectedNet, netPool, "net pool mismatch for %s", tt.name)

			gross := int64(numUsers) * entryFeeCents
			commission := gross - netPool
			assert.Equal(t, int64(float64(gross)*tt.commissionRate), commission,
				"commission mismatch for %s", tt.name)
		})
	}
}

// ---------------------------------------------------------------------------
// TestPrizePool_ConcurrentJoins
// ---------------------------------------------------------------------------

func TestPrizePool_ConcurrentJoins(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := SetupPrizePoolTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	const (
		numUsers      = 50
		entryFeeCents = 10000
	)

	contestID := env.CreateTestContestWithFees(ctx, t, "Concurrent Joins", "running", entryFeeCents, 2000)

	// Pre-create all users with funded wallets
	userIDs := env.CreateUsersWithWallets(ctx, t, numUsers, entryFeeCents*2)

	// Concurrently join, deducting entry fee
	var wg sync.WaitGroup
	errCh := make(chan error, numUsers)

	for _, uid := range userIDs {
		wg.Add(1)
		go func(userID string) {
			defer wg.Done()

			tx, err := env.DB.BeginTx(ctx, nil)
			if err != nil {
				errCh <- fmt.Errorf("begin tx for %s: %w", userID, err)
				return
			}

			_, err = env.walletService.DeductContestEntryFeeWithName(ctx, tx, userID, contestID, "Concurrent Test", entryFeeCents)
			if err != nil {
				tx.Rollback()
				errCh <- fmt.Errorf("deduct fee for %s: %w", userID, err)
				return
			}

			// Join contest
			_, err = tx.ExecContext(ctx,
				`INSERT INTO contest_participants (contest_id, user_id, qty_total, qty_available) VALUES ($1, $2, 100000, 100000)`,
				contestID, userID,
			)
			if err != nil {
				tx.Rollback()
				errCh <- fmt.Errorf("join contest for %s: %w", userID, err)
				return
			}

			if err := tx.Commit(); err != nil {
				errCh <- fmt.Errorf("commit for %s: %w", userID, err)
				return
			}
		}(uid)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent join error: %v", err)
	}

	// Verify participant count
	var count int
	err := env.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contest_participants WHERE contest_id = $1`, contestID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, numUsers, count, "all users should be participants")

	// Verify total wallet deductions
	var totalDeducted int64
	for _, uid := range userIDs {
		balance := env.GetWalletBalance(ctx, t, uid)
		totalDeducted += (entryFeeCents*2 - balance)
	}
	expectedGross := int64(numUsers) * entryFeeCents
	assert.Equal(t, expectedGross, totalDeducted, "total deductions should equal gross pool")
}

// ---------------------------------------------------------------------------
// TestPrizePool_PrizeDistribution_VaryingParticipants
// ---------------------------------------------------------------------------

func TestPrizePool_PrizeDistribution_VaryingParticipants(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tests := []struct {
		name           string
		participants   int
		expectedWinner int
	}{
		{"3_participants_1_winner", 3, 1},    // ceil(3*0.30) = 1
		{"10_participants_3_winners", 10, 3}, // ceil(10*0.30) = 3
		{"25_participants_8_winners", 25, 8}, // ceil(25*0.30) = 8
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const entryFeeCents = 10000
			const commissionRate = 0.20

			winnersCount := prize.GetWinnersCount(tt.participants)
			assert.Equal(t, tt.expectedWinner, winnersCount, "winners count mismatch")

			pool, poolErr := prize.CalculatePrizePool(tt.participants, entryFeeCents, commissionRate)
			require.NoError(t, poolErr, "CalculatePrizePool should not error")
			require.Greater(t, pool, int64(0), "pool should be positive")

			slots := prize.CalculatePrizeDistribution(tt.participants, pool)
			require.Len(t, slots, winnersCount, "should have correct number of prize slots")

			// Verify cent-perfect distribution
			var totalDistributed int64
			for _, s := range slots {
				totalDistributed += s.AmountCents
				assert.Greater(t, s.AmountCents, int64(0), "each winner should get > 0 cents")
			}
			assert.Equal(t, pool, totalDistributed, "distributed amount should equal net pool exactly")

			// Verify decreasing order
			for i := 1; i < len(slots); i++ {
				assert.GreaterOrEqual(t, slots[i-1].AmountCents, slots[i].AmountCents,
					"prizes should be non-increasing")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestPrizePool_MinParticipants_Threshold
// ---------------------------------------------------------------------------

func TestPrizePool_MinParticipants_Threshold(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := SetupPrizePoolTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	const entryFeeCents = 10000

	contestID := env.CreateTestContestWithFees(ctx, t, "Min Participants", "registration_open", entryFeeCents, 2000)

	// Set min_participants to 5
	_, err := env.DB.ExecContext(ctx,
		`UPDATE contests SET min_participants = 5 WHERE id = $1`, contestID)
	require.NoError(t, err)

	// Join 4 users
	userIDs := env.CreateUsersWithWallets(ctx, t, 5, entryFeeCents*2)
	for i := 0; i < 4; i++ {
		env.DeductEntryFee(ctx, t, userIDs[i], contestID, entryFeeCents)
		env.JoinContest(ctx, t, contestID, userIDs[i], 100000)
	}

	// Verify below threshold
	var participantCount, minParticipants int
	err = env.DB.QueryRowContext(ctx,
		`SELECT current_participants, min_participants FROM contests WHERE id = $1`, contestID,
	).Scan(&participantCount, &minParticipants)
	require.NoError(t, err)
	assert.Equal(t, 4, participantCount)
	assert.True(t, participantCount < minParticipants, "should be below threshold")

	// Join 5th user → meets threshold
	env.DeductEntryFee(ctx, t, userIDs[4], contestID, entryFeeCents)
	env.JoinContest(ctx, t, contestID, userIDs[4], 100000)

	err = env.DB.QueryRowContext(ctx,
		`SELECT current_participants FROM contests WHERE id = $1`, contestID,
	).Scan(&participantCount)
	require.NoError(t, err)
	assert.Equal(t, 5, participantCount, "should meet threshold exactly")
	assert.True(t, participantCount >= minParticipants, "should meet or exceed threshold")
}

// ---------------------------------------------------------------------------
// TestPrizePool_ZeroParticipants
// ---------------------------------------------------------------------------

func TestPrizePool_ZeroParticipants(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Verify that prize calculation with 0 participants returns 0
	pool, err := prize.CalculatePrizePool(0, 10000, 0.20)
	require.NoError(t, err, "CalculatePrizePool should not error for 0 participants")
	assert.Equal(t, int64(0), pool, "pool should be 0 for 0 participants")

	winners := prize.GetWinnersCount(0)
	assert.Equal(t, 0, winners, "winners count should be 0")

	slots := prize.CalculatePrizeDistribution(0, 0)
	assert.Nil(t, slots, "no slots for 0 participants")
}

// ---------------------------------------------------------------------------
// TestPrizePool_OneParticipant_Refund
// ---------------------------------------------------------------------------

func TestPrizePool_OneParticipant_Refund(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := SetupPrizePoolTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	const entryFeeCents = 50000

	contestID := env.CreateTestContestWithFees(ctx, t, "One Participant Refund", "running", entryFeeCents, 2000)

	// Create and join 1 user
	userIDs := env.CreateUsersWithWallets(ctx, t, 1, entryFeeCents*2)
	env.DeductEntryFee(ctx, t, userIDs[0], contestID, entryFeeCents)
	env.JoinContest(ctx, t, contestID, userIDs[0], 100000)

	// Verify balance after deduction
	balanceAfterEntry := env.GetWalletBalance(ctx, t, userIDs[0])
	assert.Equal(t, entryFeeCents, balanceAfterEntry, "should have initial - entry_fee left")

	// Refund the entry fee (simulating contest cancellation)
	tx, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = env.walletService.RefundContestEntryFee(ctx, tx, userIDs[0], contestID, entryFeeCents)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	// Verify wallet balance restored
	balanceAfterRefund := env.GetWalletBalance(ctx, t, userIDs[0])
	assert.Equal(t, entryFeeCents*2, balanceAfterRefund, "wallet should be fully restored")
}

// ---------------------------------------------------------------------------
// TestPrizePool_FreeTournament_SponsoredPrize
// ---------------------------------------------------------------------------

func TestPrizePool_FreeTournament_SponsoredPrize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := SetupPrizePoolTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	const sponsoredPool = int64(500000) // 5000 in major units

	// Create a free contest (entry_fee = 0)
	contestID := env.CreateTestContestWithFees(ctx, t, "Free Tournament", "running", 0, 0)

	// Mark as free
	_, err := env.DB.ExecContext(ctx,
		`UPDATE contests SET is_free = true WHERE id = $1`, contestID)
	require.NoError(t, err)

	// Create 5 users, join without wallet deduction
	userIDs := env.CreateUsersWithWallets(ctx, t, 5, 0)
	for _, uid := range userIDs {
		env.JoinContest(ctx, t, contestID, uid, 100000)
	}

	// Verify no wallet deduction occurred (balance should remain 0)
	for _, uid := range userIDs {
		balance := env.GetWalletBalance(ctx, t, uid)
		assert.Equal(t, int64(0), balance, "free contest should not deduct from wallet")
	}

	// Populate leaderboard
	scores := make(map[string]float64)
	for i, uid := range userIDs {
		scores[uid] = float64(500 - i*100)
	}
	env.PopulateRedisLeaderboard(ctx, t, contestID, scores)

	// Calculate prize distribution from sponsored pool
	winnersCount := prize.GetWinnersCount(5) // ceil(5*0.30) = 2
	assert.Equal(t, 2, winnersCount)

	slots := prize.CalculatePrizeDistribution(5, sponsoredPool)
	require.Len(t, slots, winnersCount)

	// Credit prizes from sponsored pool
	// Sort users by score descending to match rank
	rankedUsers := make([]string, len(userIDs))
	copy(rankedUsers, userIDs)

	for i, slot := range slots {
		env.CreditPrize(ctx, t, rankedUsers[i], contestID, slot.Rank, slot.AmountCents)
	}

	// Verify winners received prizes
	var totalPrizes int64
	for i := 0; i < winnersCount; i++ {
		balance := env.GetWalletBalance(ctx, t, rankedUsers[i])
		assert.Greater(t, balance, int64(0), "winner %d should have received a prize", i+1)
		totalPrizes += balance
	}
	assert.Equal(t, sponsoredPool, totalPrizes, "total prizes should equal sponsored pool")

	// Verify non-winners received nothing
	for i := winnersCount; i < len(rankedUsers); i++ {
		balance := env.GetWalletBalance(ctx, t, rankedUsers[i])
		assert.Equal(t, int64(0), balance, "non-winner should have 0 balance")
	}
}

// ---------------------------------------------------------------------------
// TestPrizePool_IdempotentPrizeCredit
// ---------------------------------------------------------------------------

func TestPrizePool_IdempotentPrizeCredit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	env := SetupPrizePoolTestEnv(t, ctx)
	defer env.Cleanup(t, ctx)

	contestID := env.CreateTestContestWithFees(ctx, t, "Idempotent Prize", "running", 10000, 2000)
	userIDs := env.CreateUsersWithWallets(ctx, t, 1, 0)
	env.JoinContest(ctx, t, contestID, userIDs[0], 100000)

	prizeCents := int64(50000)

	// Credit prize first time
	env.CreditPrize(ctx, t, userIDs[0], contestID, 1, prizeCents)
	balance1 := env.GetWalletBalance(ctx, t, userIDs[0])
	assert.Equal(t, prizeCents, balance1)

	// Credit same prize again (idempotent — should be rejected)
	tx, err := env.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = env.walletService.CreditPrizeIdempotent(ctx, tx, userIDs[0], contestID, 1, prizeCents)
	if err != nil {
		tx.Rollback()
		// Expected: duplicate prize credit error
		var dupErr *wallet.DuplicatePrizeCreditError
		if ok := errors.As(err, &dupErr); !ok {
			t.Errorf("expected DuplicatePrizeCreditError, got: %v", err)
		}
	} else {
		tx.Commit()
	}

	// Balance should still be the same (no double-credit)
	balance2 := env.GetWalletBalance(ctx, t, userIDs[0])
	assert.Equal(t, prizeCents, balance2, "balance should not change on duplicate credit")
}
