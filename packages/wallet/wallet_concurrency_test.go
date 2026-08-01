package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// ensureReasonCodeColumn adds the reason_code column to wallet_ledger if missing.
// The production schema includes this column, but the minimal test migrations may not.
func ensureReasonCodeColumn(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx,
		`ALTER TABLE wallet_ledger ADD COLUMN IF NOT EXISTS reason_code VARCHAR(50)`)
	return err
}

func TestConcurrentCredits_Idempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	if err := ensureReasonCodeColumn(ctx, env.db); err != nil {
		t.Fatalf("Failed to ensure reason_code column: %v", err)
	}

	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "concurrent-credit@example.com")

	// Fund the wallet first so we have a starting balance of 0
	// (wallet is created with 0 balance by default).

	const goroutines = 10
	const creditCents int64 = 10000 // $100
	idempotencyKey := "test-concurrent-credit-key"

	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	// Launch 10 goroutines that all try to credit $100 with the SAME idempotency key.
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			tx, err := env.db.BeginTx(ctx, nil)
			if err != nil {
				errs[idx] = fmt.Errorf("begin tx: %w", err)
				return
			}

			_, err = svc.CreditIdempotent(
				ctx, tx, userID, creditCents,
				LedgerTypePrizeCredit, nil, nil, nil, idempotencyKey,
			)
			if err != nil {
				// DuplicateCreditError is expected for all but the first goroutine.
				var dupErr *DuplicateCreditError
				if errors.As(err, &dupErr) {
					tx.Rollback()
					return
				}
				tx.Rollback()
				errs[idx] = fmt.Errorf("credit: %w", err)
				return
			}

			if err := tx.Commit(); err != nil {
				errs[idx] = fmt.Errorf("commit: %w", err)
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
	}

	// Assert final balance is exactly $100 (not $1000).
	balance, err := svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if balance != creditCents {
		t.Errorf("expected balance %d, got %d", creditCents, balance)
	}

	// Assert only 1 ledger entry was created.
	var ledgerCount int
	err = env.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wallet_ledger WHERE user_id = $1`, userID,
	).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("count ledger entries error: %v", err)
	}
	if ledgerCount != 1 {
		t.Errorf("expected 1 ledger entry, got %d", ledgerCount)
	}
}

func TestConcurrentDebitAndCredit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	if err := ensureReasonCodeColumn(ctx, env.db); err != nil {
		t.Fatalf("Failed to ensure reason_code column: %v", err)
	}

	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "debit-credit@example.com")

	// Fund the wallet with $500.
	{
		tx, err := env.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		_, err = svc.Credit(ctx, tx, userID, 50000, LedgerTypeDeposit, nil, nil, nil)
		if err != nil {
			tx.Rollback()
			t.Fatalf("initial credit: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit initial credit: %v", err)
		}
	}

	const debitGoroutines = 5
	const creditGoroutines = 5
	const debitCents int64 = 10000 // $100 each → $500 total debit
	const creditCents int64 = 5000 // $50 each → $250 total credit

	var wg sync.WaitGroup
	errs := make([]error, debitGoroutines+creditGoroutines)

	// Launch 5 goroutines that debit $100 each (contest entry fees).
	for i := 0; i < debitGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			tx, err := env.db.BeginTx(ctx, nil)
			if err != nil {
				errs[idx] = fmt.Errorf("begin tx: %w", err)
				return
			}

			_, err = svc.Debit(ctx, tx, userID, debitCents, LedgerTypeContestEntry, nil, nil, nil)
			if err != nil {
				tx.Rollback()
				errs[idx] = fmt.Errorf("debit: %w", err)
				return
			}

			if err := tx.Commit(); err != nil {
				errs[idx] = fmt.Errorf("commit: %w", err)
			}
		}(i)
	}

	// Launch 5 goroutines that credit $50 each (prizes).
	for i := 0; i < creditGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			tx, err := env.db.BeginTx(ctx, nil)
			if err != nil {
				errs[debitGoroutines+idx] = fmt.Errorf("begin tx: %w", err)
				return
			}

			_, err = svc.Credit(ctx, tx, userID, creditCents, LedgerTypePrizeCredit, nil, nil, nil)
			if err != nil {
				tx.Rollback()
				errs[debitGoroutines+idx] = fmt.Errorf("credit: %w", err)
				return
			}

			if err := tx.Commit(); err != nil {
				errs[debitGoroutines+idx] = fmt.Errorf("commit: %w", err)
			}
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d failed: %v", i, err)
		}
	}

	// Assert final balance: $500 - $500 + $250 = $250.
	balance, err := svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	expectedBalance := int64(50000) - int64(debitGoroutines)*debitCents + int64(creditGoroutines)*creditCents
	if balance != expectedBalance {
		t.Errorf("expected balance %d, got %d", expectedBalance, balance)
	}

	// Assert exactly 11 ledger entries (1 initial deposit + 5 debits + 5 credits).
	var ledgerCount int
	err = env.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wallet_ledger WHERE user_id = $1`, userID,
	).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("count ledger entries error: %v", err)
	}
	expectedEntries := 1 + debitGoroutines + creditGoroutines
	if ledgerCount != expectedEntries {
		t.Errorf("expected %d ledger entries, got %d", expectedEntries, ledgerCount)
	}
}

func TestCreditFrozenWallet_PrizeOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := setupTestDB(t)
	defer env.cleanup(t)

	ctx := context.Background()
	if err := ensureReasonCodeColumn(ctx, env.db); err != nil {
		t.Fatalf("Failed to ensure reason_code column: %v", err)
	}

	svc := NewService(env.db)
	userID := createTestUser(ctx, t, env.db, "frozen-wallet@example.com")

	// Freeze the wallet.
	if err := svc.FreezeWallet(ctx, userID); err != nil {
		t.Fatalf("FreezeWallet error: %v", err)
	}

	// 1. Credit with LedgerTypePrizeCredit — should succeed.
	t.Run("prize credit succeeds on frozen wallet", func(t *testing.T) {
		tx, err := env.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}

		_, err = svc.Credit(ctx, tx, userID, 10000, LedgerTypePrizeCredit, nil, nil, nil)
		if err != nil {
			tx.Rollback()
			t.Fatalf("prize credit on frozen wallet should succeed: %v", err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	})

	// 2. Credit with LedgerTypeContestRefund — should succeed.
	t.Run("contest refund succeeds on frozen wallet", func(t *testing.T) {
		tx, err := env.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}

		_, err = svc.Credit(ctx, tx, userID, 5000, LedgerTypeContestRefund, nil, nil, nil)
		if err != nil {
			tx.Rollback()
			t.Fatalf("contest refund on frozen wallet should succeed: %v", err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	})

	// 3. Credit with LedgerTypeDeposit — should fail with WalletFrozenError.
	t.Run("deposit fails on frozen wallet", func(t *testing.T) {
		tx, err := env.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}

		_, err = svc.Credit(ctx, tx, userID, 5000, LedgerTypeDeposit, nil, nil, nil)
		tx.Rollback()

		if err == nil {
			t.Fatal("expected WalletFrozenError for deposit on frozen wallet, got nil")
		}

		var frozenErr *WalletFrozenError
		if !errors.As(err, &frozenErr) {
			t.Fatalf("expected WalletFrozenError, got %T: %v", err, err)
		}
	})

	// Assert balance reflects only the two successful credits: $100 + $50 = $150.
	balance, err := svc.GetBalance(ctx, userID)
	if err != nil {
		t.Fatalf("GetBalance error: %v", err)
	}
	if balance != 15000 {
		t.Errorf("expected balance 15000, got %d", balance)
	}
}
