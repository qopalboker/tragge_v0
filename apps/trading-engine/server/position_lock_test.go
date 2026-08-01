package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAcquireLockWithTimeout_Uncontended(t *testing.T) {
	plm := NewPositionLockManager(nil, nil, "test", nil)
	unlock, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "long")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	unlock()
}

func TestAcquireLockWithTimeout_Contended(t *testing.T) {
	plm := NewPositionLockManager(nil, nil, "test", nil)

	// Acquire the lock first
	unlock1, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "long")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Release after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		unlock1()
	}()

	// Second acquisition should succeed after waiting
	unlock2, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "long")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	unlock2()
}

func TestAcquireLockWithTimeout_Timeout(t *testing.T) {
	plm := NewPositionLockManager(nil, nil, "test", nil)

	// Acquire and hold the lock
	unlock1, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "long")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer unlock1()

	// Second acquisition with short timeout should fail
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = plm.AcquireLockWithTimeout(ctx, "c1", "u1", "AAPL", "long")
	if err != ErrLockTimeout {
		t.Fatalf("expected ErrLockTimeout, got %v", err)
	}
}

func TestAcquireLockForSymbolWithTimeout_Success(t *testing.T) {
	plm := NewPositionLockManager(nil, nil, "test", nil)

	unlock, err := plm.AcquireLockForSymbolWithTimeout(context.Background(), "c1", "u1", "AAPL")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	unlock()
}

func TestAcquireLockForSymbolWithTimeout_PartialFailure(t *testing.T) {
	plm := NewPositionLockManager(nil, nil, "test", nil)

	// Hold the "short" lock so AcquireLockForSymbolWithTimeout fails on the second lock
	unlockShort, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "short")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer unlockShort()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = plm.AcquireLockForSymbolWithTimeout(ctx, "c1", "u1", "AAPL")
	if err != ErrLockTimeout {
		t.Fatalf("expected ErrLockTimeout, got %v", err)
	}

	// Verify the "long" lock was properly released (not leaked) by acquiring it again
	unlock, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "long")
	if err != nil {
		t.Fatalf("long lock should be available after partial failure, got %v", err)
	}
	unlock()
}

func TestAcquireLockWithTimeout_ConcurrentTimeouts(t *testing.T) {
	plm := NewPositionLockManager(nil, nil, "test", nil)

	// Hold a lock and cause many timeouts
	unlock1, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "long")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			plm.AcquireLockWithTimeout(ctx, "c1", "u1", "AAPL", "long")
		}()
	}
	wg.Wait()

	// Release the lock — cleanup goroutines should resolve
	unlock1()

	// Give cleanup goroutines time to finish
	time.Sleep(100 * time.Millisecond)

	// Lock should be available again
	unlock2, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "long")
	if err != nil {
		t.Fatalf("lock should be available after cleanup, got %v", err)
	}
	unlock2()
}

func TestAcquireLockWithTimeout_DifferentKeys(t *testing.T) {
	plm := NewPositionLockManager(nil, nil, "test", nil)

	// Different keys should not block each other
	unlock1, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "AAPL", "long")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	defer unlock1()

	unlock2, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u1", "TSLA", "long")
	if err != nil {
		t.Fatalf("expected no error for different symbol, got %v", err)
	}
	defer unlock2()

	unlock3, err := plm.AcquireLockWithTimeout(context.Background(), "c1", "u2", "AAPL", "long")
	if err != nil {
		t.Fatalf("expected no error for different user, got %v", err)
	}
	defer unlock3()
}
