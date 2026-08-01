package wallet

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testEnv holds the test environment.
type testEnv struct {
	db        *sql.DB
	container *postgres.PostgresContainer
}

// setupTestDB creates a PostgreSQL container and runs migrations.
func setupTestDB(t *testing.T) *testEnv {
	t.Helper()

	ctx := context.Background()

	// Start PostgreSQL container
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("Failed to get PostgreSQL connection string: %v", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("Failed to open PostgreSQL connection: %v", err)
	}

	// Run minimal schema for tests
	if err := runTestMigrations(ctx, db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return &testEnv{
		db:        db,
		container: container,
	}
}

// cleanup terminates the container.
func (env *testEnv) cleanup(t *testing.T) {
	t.Helper()

	if env.db != nil {
		env.db.Close()
	}

	if env.container != nil {
		if err := env.container.Terminate(context.Background()); err != nil {
			t.Logf("Warning: Failed to terminate container: %v", err)
		}
	}
}

// runTestMigrations creates the minimal schema needed for wallet tests.
func runTestMigrations(ctx context.Context, db *sql.DB) error {
	migrations := []string{
		// Extension for UUID generation
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`,

		// Users table (minimal)
		`CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			email VARCHAR(255) NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Wallet status enum
		`DO $$ BEGIN
			CREATE TYPE wallet_status AS ENUM ('active', 'frozen', 'closed');
		EXCEPTION
			WHEN duplicate_object THEN null;
		END $$`,

		// Wallets table
		`CREATE TABLE IF NOT EXISTS wallets (
			user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			balance_cents BIGINT NOT NULL DEFAULT 0,
			currency VARCHAR(3) NOT NULL DEFAULT 'USD',
			status wallet_status NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_balance_non_negative CHECK (balance_cents >= 0)
		)`,

		// Ledger type enum
		`DO $$ BEGIN
			CREATE TYPE ledger_type AS ENUM (
				'deposit', 'withdrawal', 'contest_entry', 'contest_refund',
				'prize_credit', 'adjustment', 'affiliate_commission'
			);
		EXCEPTION
			WHEN duplicate_object THEN null;
		END $$`,

		// Ledger ref type enum
		`DO $$ BEGIN
			CREATE TYPE ledger_ref_type AS ENUM (
				'payment_intent', 'payout', 'contest', 'admin_action', 'commission'
			);
		EXCEPTION
			WHEN duplicate_object THEN null;
		END $$`,

		// Wallet ledger table (with idempotency_key)
		`CREATE TABLE IF NOT EXISTS wallet_ledger (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			type ledger_type NOT NULL,
			amount_cents BIGINT NOT NULL,
			balance_after_cents BIGINT NOT NULL,
			ref_type ledger_ref_type,
			ref_id UUID,
			description TEXT,
			idempotency_key VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT chk_amount_non_zero CHECK (amount_cents != 0),
			CONSTRAINT chk_balance_after_non_negative CHECK (balance_after_cents >= 0)
		)`,

		// Unique index on idempotency_key
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_ledger_idempotency_key
		 ON wallet_ledger(idempotency_key)
		 WHERE idempotency_key IS NOT NULL`,

		// Payout status enum
		`DO $$ BEGIN
			CREATE TYPE payout_status AS ENUM ('pending', 'processing', 'succeeded', 'failed', 'cancelled', 'rejected');
		EXCEPTION
			WHEN duplicate_object THEN null;
		END $$`,

		// Payouts table (for withdrawal limit tests)
		`CREATE TABLE IF NOT EXISTS payouts (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			amount_cents BIGINT NOT NULL,
			currency VARCHAR(3) NOT NULL DEFAULT 'USD',
			status payout_status NOT NULL DEFAULT 'pending',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		// Withdrawal limits table (per-user overrides)
		`CREATE TABLE IF NOT EXISTS withdrawal_limits (
			user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			daily_amount_cents BIGINT,
			monthly_amount_cents BIGINT,
			daily_count INT,
			monthly_count INT,
			notes TEXT,
			updated_by UUID REFERENCES users(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}

	for _, m := range migrations {
		if _, err := db.ExecContext(ctx, m); err != nil {
			return err
		}
	}

	return nil
}

// createTestUser creates a test user and wallet.
func createTestUser(ctx context.Context, t *testing.T, db *sql.DB, email string) string {
	t.Helper()

	var userID string
	err := db.QueryRowContext(ctx,
		`INSERT INTO users (email) VALUES ($1) RETURNING id`,
		email,
	).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	// Create wallet for user
	_, err = db.ExecContext(ctx,
		`INSERT INTO wallets (user_id) VALUES ($1)`,
		userID,
	)
	if err != nil {
		t.Fatalf("Failed to create test wallet: %v", err)
	}

	return userID
}

func TestGeneratePrizeIdempotencyKey(t *testing.T) {
	tests := []struct {
		name      string
		contestID string
		userID    string
		rank      int
		expected  string
	}{
		{
			name:      "standard key generation",
			contestID: "contest-123",
			userID:    "user-456",
			rank:      1,
			expected:  "finalization:contest-123:user-456:1",
		},
		{
			name:      "different rank",
			contestID: "contest-123",
			userID:    "user-456",
			rank:      10,
			expected:  "finalization:contest-123:user-456:10",
		},
		{
			name:      "uuid format",
			contestID: "550e8400-e29b-41d4-a716-446655440000",
			userID:    "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			rank:      5,
			expected:  "finalization:550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GeneratePrizeIdempotencyKey(tt.contestID, tt.userID, tt.rank)
			if result != tt.expected {
				t.Errorf("GeneratePrizeIdempotencyKey() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCreditPrizeIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)

	// Create test user
	userID := createTestUser(ctx, t, env.db, "test@example.com")
	contestID := "contest-123"
	rank := 1
	prizeCents := int64(10000) // $100

	// Test 1: First credit should succeed
	t.Run("first credit succeeds", func(t *testing.T) {
		tx, err := env.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		entry, err := svc.CreditPrizeIdempotent(ctx, tx, userID, contestID, rank, prizeCents)
		if err != nil {
			tx.Rollback()
			t.Fatalf("CreditPrizeIdempotent() error = %v", err)
		}

		if entry == nil {
			tx.Rollback()
			t.Fatal("CreditPrizeIdempotent() returned nil entry")
		}

		if entry.AmountCents != prizeCents {
			tx.Rollback()
			t.Errorf("Entry amount = %d, want %d", entry.AmountCents, prizeCents)
		}

		expectedKey := GeneratePrizeIdempotencyKey(contestID, userID, rank)
		if entry.IdempotencyKey == nil || *entry.IdempotencyKey != expectedKey {
			tx.Rollback()
			t.Errorf("Entry idempotency key = %v, want %v", entry.IdempotencyKey, expectedKey)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Verify wallet balance
		balance, err := svc.GetBalance(ctx, userID)
		if err != nil {
			t.Fatalf("GetBalance() error = %v", err)
		}
		if balance != prizeCents {
			t.Errorf("Balance = %d, want %d", balance, prizeCents)
		}
	})

	// Test 2: Duplicate credit should return DuplicatePrizeCreditError
	t.Run("duplicate credit returns error with existing entry", func(t *testing.T) {
		tx, err := env.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		entry, err := svc.CreditPrizeIdempotent(ctx, tx, userID, contestID, rank, prizeCents)

		// Should return DuplicatePrizeCreditError
		var dupErr *DuplicatePrizeCreditError
		if err == nil {
			tx.Rollback()
			t.Fatal("Expected DuplicatePrizeCreditError, got nil")
		}

		// Use type assertion instead of errors.As since we need the pointer
		dupErr, ok := err.(*DuplicatePrizeCreditError)
		if !ok {
			tx.Rollback()
			t.Fatalf("Expected DuplicatePrizeCreditError, got %T: %v", err, err)
		}

		// The entry should be the existing one
		if entry == nil {
			tx.Rollback()
			t.Fatal("Expected existing entry to be returned")
		}

		if entry.AmountCents != prizeCents {
			tx.Rollback()
			t.Errorf("Existing entry amount = %d, want %d", entry.AmountCents, prizeCents)
		}

		expectedKey := GeneratePrizeIdempotencyKey(contestID, userID, rank)
		if dupErr.IdempotencyKey != expectedKey {
			tx.Rollback()
			t.Errorf("Error idempotency key = %v, want %v", dupErr.IdempotencyKey, expectedKey)
		}

		tx.Rollback() // Don't commit duplicate

		// Verify wallet balance hasn't changed
		balance, err := svc.GetBalance(ctx, userID)
		if err != nil {
			t.Fatalf("GetBalance() error = %v", err)
		}
		if balance != prizeCents {
			t.Errorf("Balance after duplicate = %d, want %d (unchanged)", balance, prizeCents)
		}
	})

	// Test 3: Different rank should succeed (different idempotency key)
	t.Run("different rank creates new entry", func(t *testing.T) {
		tx, err := env.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}

		differentRank := 2
		entry, err := svc.CreditPrizeIdempotent(ctx, tx, userID, contestID, differentRank, prizeCents)
		if err != nil {
			tx.Rollback()
			t.Fatalf("CreditPrizeIdempotent() for different rank error = %v", err)
		}

		if entry == nil {
			tx.Rollback()
			t.Fatal("CreditPrizeIdempotent() returned nil entry for different rank")
		}

		expectedKey := GeneratePrizeIdempotencyKey(contestID, userID, differentRank)
		if entry.IdempotencyKey == nil || *entry.IdempotencyKey != expectedKey {
			tx.Rollback()
			t.Errorf("Entry idempotency key = %v, want %v", entry.IdempotencyKey, expectedKey)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		// Verify wallet balance increased
		balance, err := svc.GetBalance(ctx, userID)
		if err != nil {
			t.Fatalf("GetBalance() error = %v", err)
		}
		expectedBalance := prizeCents * 2 // Two credits now
		if balance != expectedBalance {
			t.Errorf("Balance = %d, want %d", balance, expectedBalance)
		}
	})
}

func TestCreditPrizeIdempotent_ZeroPrize(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)

	userID := createTestUser(ctx, t, env.db, "test2@example.com")

	tx, err := env.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Zero prize should return nil, nil
	entry, err := svc.CreditPrizeIdempotent(ctx, tx, userID, "contest-123", 1, 0)
	if err != nil {
		t.Errorf("CreditPrizeIdempotent() with zero prize error = %v", err)
	}
	if entry != nil {
		t.Errorf("CreditPrizeIdempotent() with zero prize returned entry: %v", entry)
	}
}

func TestCreditPrizeIdempotent_NegativePrize(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)

	userID := createTestUser(ctx, t, env.db, "test3@example.com")

	tx, err := env.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Negative prize should return nil, nil (same as zero)
	entry, err := svc.CreditPrizeIdempotent(ctx, tx, userID, "contest-123", 1, -100)
	if err != nil {
		t.Errorf("CreditPrizeIdempotent() with negative prize error = %v", err)
	}
	if entry != nil {
		t.Errorf("CreditPrizeIdempotent() with negative prize returned entry: %v", entry)
	}
}

func TestCreditPrizeIdempotent_WalletNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)

	// Non-existent user
	nonExistentUserID := "00000000-0000-0000-0000-000000000000"

	tx, err := env.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	_, err = svc.CreditPrizeIdempotent(ctx, tx, nonExistentUserID, "contest-123", 1, 10000)
	if err == nil {
		t.Error("Expected error for non-existent user, got nil")
	}

	// Should be a WalletNotFoundError
	_, ok := err.(*WalletNotFoundError)
	if !ok {
		t.Errorf("Expected WalletNotFoundError, got %T: %v", err, err)
	}
}

// insertTestPayout inserts a payout record for testing withdrawal limits.
func insertTestPayout(ctx context.Context, t *testing.T, db *sql.DB, userID string, amountCents int64, status string, createdAt time.Time) {
	t.Helper()
	_, err := db.ExecContext(ctx,
		`INSERT INTO payouts (user_id, amount_cents, status, created_at) VALUES ($1, $2, $3::payout_status, $4)`,
		userID, amountCents, status, createdAt,
	)
	if err != nil {
		t.Fatalf("Failed to insert test payout: %v", err)
	}
}

func TestCheckWithdrawalLimits_WithinLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "limits-within@example.com")

	defaults := WithdrawalLimits{
		DailyAmountCents:   1000000, // $10,000
		MonthlyAmountCents: 5000000, // $50,000
		DailyCount:         3,
		MonthlyCount:       10,
	}

	// No payouts yet, should be within limits
	err := svc.CheckWithdrawalLimits(ctx, userID, 500000, defaults) // $5,000
	if err != nil {
		t.Errorf("CheckWithdrawalLimits() error = %v, want nil", err)
	}

	// Add one payout today
	insertTestPayout(ctx, t, env.db, userID, 300000, "pending", time.Now().UTC())

	// Still within limits
	err = svc.CheckWithdrawalLimits(ctx, userID, 500000, defaults)
	if err != nil {
		t.Errorf("CheckWithdrawalLimits() error = %v, want nil", err)
	}
}

func TestCheckWithdrawalLimits_DailyAmountExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "limits-daily-amt@example.com")

	defaults := WithdrawalLimits{
		DailyAmountCents:   1000000, // $10,000
		MonthlyAmountCents: 5000000,
		DailyCount:         10,
		MonthlyCount:       30,
	}

	// Add payouts totaling $8,000 today
	insertTestPayout(ctx, t, env.db, userID, 500000, "pending", time.Now().UTC())
	insertTestPayout(ctx, t, env.db, userID, 300000, "succeeded", time.Now().UTC())

	// Try to withdraw $3,000 — would total $11,000 > $10,000
	err := svc.CheckWithdrawalLimits(ctx, userID, 300000, defaults)
	if err == nil {
		t.Fatal("Expected WithdrawalLimitExceededError, got nil")
	}

	limitErr, ok := err.(*WithdrawalLimitExceededError)
	if !ok {
		t.Fatalf("Expected *WithdrawalLimitExceededError, got %T: %v", err, err)
	}
	if limitErr.LimitType != "daily_amount" {
		t.Errorf("LimitType = %q, want %q", limitErr.LimitType, "daily_amount")
	}
	if limitErr.LimitValue != 1000000 {
		t.Errorf("LimitValue = %d, want %d", limitErr.LimitValue, 1000000)
	}
}

func TestCheckWithdrawalLimits_MonthlyAmountExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "limits-monthly-amt@example.com")

	defaults := WithdrawalLimits{
		DailyAmountCents:   5000000,  // $50,000 daily (high so it doesn't trigger)
		MonthlyAmountCents: 1000000,  // $10,000 monthly
		DailyCount:         100,
		MonthlyCount:       100,
	}

	// Add payouts earlier this month (but not today)
	now := time.Now().UTC()
	earlier := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC)
	if earlier.After(now) {
		earlier = now.Add(-24 * time.Hour)
	}
	insertTestPayout(ctx, t, env.db, userID, 800000, "succeeded", earlier)

	// Try to withdraw $3,000 — would total $11,000 > $10,000
	err := svc.CheckWithdrawalLimits(ctx, userID, 300000, defaults)
	if err == nil {
		t.Fatal("Expected WithdrawalLimitExceededError, got nil")
	}

	limitErr, ok := err.(*WithdrawalLimitExceededError)
	if !ok {
		t.Fatalf("Expected *WithdrawalLimitExceededError, got %T: %v", err, err)
	}
	if limitErr.LimitType != "monthly_amount" {
		t.Errorf("LimitType = %q, want %q", limitErr.LimitType, "monthly_amount")
	}
}

func TestCheckWithdrawalLimits_DailyCountExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "limits-daily-cnt@example.com")

	defaults := WithdrawalLimits{
		DailyAmountCents:   99999999,
		MonthlyAmountCents: 99999999,
		DailyCount:         2,
		MonthlyCount:       100,
	}

	// Add 2 payouts today (at the daily count limit)
	insertTestPayout(ctx, t, env.db, userID, 1000, "pending", time.Now().UTC())
	insertTestPayout(ctx, t, env.db, userID, 1000, "succeeded", time.Now().UTC())

	// Third withdrawal should be blocked
	err := svc.CheckWithdrawalLimits(ctx, userID, 1000, defaults)
	if err == nil {
		t.Fatal("Expected WithdrawalLimitExceededError, got nil")
	}

	limitErr, ok := err.(*WithdrawalLimitExceededError)
	if !ok {
		t.Fatalf("Expected *WithdrawalLimitExceededError, got %T: %v", err, err)
	}
	if limitErr.LimitType != "daily_count" {
		t.Errorf("LimitType = %q, want %q", limitErr.LimitType, "daily_count")
	}
	if limitErr.LimitValue != 2 {
		t.Errorf("LimitValue = %d, want %d", limitErr.LimitValue, 2)
	}
	if limitErr.CurrentUsage != 2 {
		t.Errorf("CurrentUsage = %d, want %d", limitErr.CurrentUsage, 2)
	}
}

func TestCheckWithdrawalLimits_MonthlyCountExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "limits-monthly-cnt@example.com")

	defaults := WithdrawalLimits{
		DailyAmountCents:   99999999,
		MonthlyAmountCents: 99999999,
		DailyCount:         100,
		MonthlyCount:       2,
	}

	// Add 2 payouts this month (one earlier, one today)
	now := time.Now().UTC()
	earlier := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, time.UTC)
	if earlier.After(now) {
		earlier = now.Add(-24 * time.Hour)
	}
	insertTestPayout(ctx, t, env.db, userID, 1000, "succeeded", earlier)
	insertTestPayout(ctx, t, env.db, userID, 1000, "pending", now)

	// Third should be blocked by monthly count
	err := svc.CheckWithdrawalLimits(ctx, userID, 1000, defaults)
	if err == nil {
		t.Fatal("Expected WithdrawalLimitExceededError, got nil")
	}

	limitErr, ok := err.(*WithdrawalLimitExceededError)
	if !ok {
		t.Fatalf("Expected *WithdrawalLimitExceededError, got %T: %v", err, err)
	}
	if limitErr.LimitType != "monthly_count" {
		t.Errorf("LimitType = %q, want %q", limitErr.LimitType, "monthly_count")
	}
}

func TestCheckWithdrawalLimits_CancelledExcluded(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "limits-cancelled@example.com")

	defaults := WithdrawalLimits{
		DailyAmountCents:   1000000,
		MonthlyAmountCents: 5000000,
		DailyCount:         2,
		MonthlyCount:       10,
	}

	// Add cancelled and failed payouts — these should NOT count
	insertTestPayout(ctx, t, env.db, userID, 900000, "cancelled", time.Now().UTC())
	insertTestPayout(ctx, t, env.db, userID, 900000, "failed", time.Now().UTC())

	// Add one real payout
	insertTestPayout(ctx, t, env.db, userID, 100000, "pending", time.Now().UTC())

	// Should still be within limits (only the pending one counts)
	err := svc.CheckWithdrawalLimits(ctx, userID, 500000, defaults)
	if err != nil {
		t.Errorf("CheckWithdrawalLimits() error = %v, want nil (cancelled/failed excluded)", err)
	}
}

func TestCheckWithdrawalLimits_PerUserOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "limits-override@example.com")

	defaults := WithdrawalLimits{
		DailyAmountCents:   1000000, // $10,000
		MonthlyAmountCents: 5000000,
		DailyCount:         3,
		MonthlyCount:       10,
	}

	// Add payout of $9,000 today
	insertTestPayout(ctx, t, env.db, userID, 900000, "succeeded", time.Now().UTC())

	// Without override, trying to withdraw $2,000 exceeds daily $10,000
	err := svc.CheckWithdrawalLimits(ctx, userID, 200000, defaults)
	if err == nil {
		t.Fatal("Expected limit exceeded without override, got nil")
	}

	// Set per-user override: daily $20,000
	_, err = env.db.ExecContext(ctx,
		`INSERT INTO withdrawal_limits (user_id, daily_amount_cents) VALUES ($1, $2)`,
		userID, 2000000,
	)
	if err != nil {
		t.Fatalf("Failed to insert withdrawal limit override: %v", err)
	}

	// Now the same withdrawal should succeed
	err = svc.CheckWithdrawalLimits(ctx, userID, 200000, defaults)
	if err != nil {
		t.Errorf("CheckWithdrawalLimits() with override error = %v, want nil", err)
	}
}

func TestCheckWithdrawalLimits_NoOverrideRow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "limits-nooverride@example.com")

	defaults := WithdrawalLimits{
		DailyAmountCents:   500000, // $5,000
		MonthlyAmountCents: 2000000,
		DailyCount:         2,
		MonthlyCount:       5,
	}

	// Add payout totaling $4,000 today
	insertTestPayout(ctx, t, env.db, userID, 400000, "succeeded", time.Now().UTC())

	// $2,000 more would exceed daily $5,000 (total $6,000)
	err := svc.CheckWithdrawalLimits(ctx, userID, 200000, defaults)
	if err == nil {
		t.Fatal("Expected limit exceeded with defaults, got nil")
	}

	limitErr, ok := err.(*WithdrawalLimitExceededError)
	if !ok {
		t.Fatalf("Expected *WithdrawalLimitExceededError, got %T: %v", err, err)
	}
	if limitErr.LimitType != "daily_amount" {
		t.Errorf("LimitType = %q, want %q", limitErr.LimitType, "daily_amount")
	}
}

func TestCreditPrizeIdempotent_ConcurrentCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	svc := NewService(env.db)

	userID := createTestUser(ctx, t, env.db, "concurrent@example.com")
	contestID := "contest-concurrent"
	rank := 1
	prizeCents := int64(5000)

	// Run multiple concurrent credit attempts
	results := make(chan error, 5)
	entries := make(chan *LedgerEntry, 5)

	for i := 0; i < 5; i++ {
		go func() {
			tx, err := env.db.BeginTx(ctx, nil)
			if err != nil {
				results <- err
				entries <- nil
				return
			}

			entry, err := svc.CreditPrizeIdempotent(ctx, tx, userID, contestID, rank, prizeCents)

			if err != nil {
				// DuplicatePrizeCreditError is acceptable
				if _, ok := err.(*DuplicatePrizeCreditError); ok {
					tx.Rollback()
					results <- nil // Not a real error
					entries <- entry
					return
				}
				tx.Rollback()
				results <- err
				entries <- nil
				return
			}

			if commitErr := tx.Commit(); commitErr != nil {
				results <- commitErr
				entries <- nil
				return
			}

			results <- nil
			entries <- entry
		}()
	}

	// Collect results
	var realErrors int
	var successfulCommits int
	var duplicates int

	for i := 0; i < 5; i++ {
		err := <-results
		entry := <-entries
		if err != nil {
			realErrors++
		} else if entry != nil {
			successfulCommits++
		} else {
			duplicates++
		}
	}

	// Should have exactly one successful commit
	if successfulCommits != 1 {
		t.Errorf("Expected exactly 1 successful commit, got %d", successfulCommits)
	}

	// Should have no real errors (duplicates don't count)
	if realErrors > 0 {
		t.Errorf("Expected 0 real errors, got %d", realErrors)
	}

	// Verify final balance is correct (only one credit)
	balance, err := svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance != prizeCents {
		t.Errorf("Final balance = %d, want %d (single credit)", balance, prizeCents)
	}

	// Verify only one ledger entry exists
	var entryCount int
	err = env.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wallet_ledger WHERE user_id = $1`,
		userID,
	).Scan(&entryCount)
	if err != nil {
		t.Fatalf("Failed to count ledger entries: %v", err)
	}
	if entryCount != 1 {
		t.Errorf("Ledger entry count = %d, want 1", entryCount)
	}
}
