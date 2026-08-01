package server

import (
	"context"
	"database/sql"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// PositionLockManager manages per-position mutexes to prevent race conditions
// during concurrent position modifications.
type PositionLockManager struct {
	// locks stores mutexes keyed by "{contest_id}:{user_id}:{symbol}:{side}"
	locks sync.Map

	// metrics for monitoring lock contention
	contentionCounter *prometheus.CounterVec

	// db for cleanup queries
	db *sql.DB

	// cleanupInterval for periodic lock cleanup
	cleanupInterval time.Duration

	// stopCleanup channel to stop the cleanup goroutine
	stopCleanup chan struct{}

	// logger for structured logging
	logger *zap.Logger
}

// positionLock wraps a mutex with metadata for cleanup
type positionLock struct {
	mu       sync.Mutex
	lastUsed time.Time // last acquisition time (for stale cleanup)
	lockedAt time.Time // current holder's acquire time (for deadlock detection; zero when unlocked)
}

// NewPositionLockManager creates a new position lock manager.
func NewPositionLockManager(db *sql.DB, registry prometheus.Registerer, namespace string, logger *zap.Logger) *PositionLockManager {
	plm := &PositionLockManager{
		db:              db,
		cleanupInterval: 5 * time.Minute,
		stopCleanup:     make(chan struct{}),
		logger:          logger,
	}

	// Create and register the contention counter
	plm.contentionCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "position_lock_contentions_total",
		Help:      "Total number of times a position lock was contended (had to wait)",
	}, []string{"contest_id", "symbol"})

	if registry != nil {
		registry.MustRegister(plm.contentionCounter)
	}

	return plm
}

// RegisterMetrics registers the position lock manager's metrics with a Prometheus registry.
// This is useful when the registry is not available at construction time.
func (m *PositionLockManager) RegisterMetrics(registry prometheus.Registerer, namespace string) {
	if registry == nil || m.contentionCounter == nil {
		return
	}

	// Unregister any existing metric first (in case of re-registration)
	registry.Unregister(m.contentionCounter)

	// Create a new counter with the correct namespace if needed
	if namespace != "" {
		m.contentionCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "position_lock_contentions_total",
			Help:      "Total number of times a position lock was contended (had to wait)",
		}, []string{"contest_id", "symbol"})
	}

	registry.MustRegister(m.contentionCounter)
}

// buildLockKey creates a lock key from position identifiers.
// Key format: {contest_id}:{user_id}:{symbol}:{side}
func buildLockKey(contestID, userID, symbol, side string) string {
	return fmt.Sprintf("%s:%s:%s:%s", contestID, userID, symbol, side)
}

// defaultLockTimeout is the maximum time to wait for a position lock.
const defaultLockTimeout = 5 * time.Second

// contentionThreshold is the wait time after which a lock acquisition is
// considered contended. Set high enough to avoid false positives from OS
// scheduling jitter on loaded systems.
const contentionThreshold = 5 * time.Millisecond

// cleanupGoroutineTimeout is the maximum time a cleanup goroutine will wait
// for a timed-out lock acquisition to complete. This prevents permanent goroutine
// leaks when a lock is truly deadlocked.
const cleanupGoroutineTimeout = 30 * time.Second

// ErrLockTimeout is returned when a position lock acquisition times out.
var ErrLockTimeout = fmt.Errorf("position lock timeout")

// AcquireLock acquires the lock for a position and returns an unlock function.
// The caller MUST call the returned function to release the lock.
// Has a built-in timeout of 5 seconds to prevent permanent deadlocks.
//
// Deprecated: Use AcquireLockWithTimeout instead, which properly returns errors.
// This function swallows lock timeout errors and returns a no-op, causing callers
// to proceed without holding the lock.
func (m *PositionLockManager) AcquireLock(contestID, userID, symbol, side string) func() {
	unlock, err := m.AcquireLockWithTimeout(context.Background(), contestID, userID, symbol, side)
	if err != nil {
		// Should not happen with background context and 5s timeout under normal conditions.
		// Log and return no-op to avoid nil dereference; caller will see stale data at worst.
		if m.logger != nil {
			m.logger.Error("Position lock acquisition failed",
				zap.String("contest_id", contestID),
				zap.String("user_id", userID),
				zap.String("symbol", symbol),
				zap.String("side", side),
				zap.Error(err))
		}
		return func() {}
	}
	return unlock
}

// AcquireLockWithTimeout acquires the lock for a position with a timeout.
// Returns an unlock function and nil error on success, or ErrLockTimeout on timeout.
func (m *PositionLockManager) AcquireLockWithTimeout(ctx context.Context, contestID, userID, symbol, side string) (func(), error) {
	key := buildLockKey(contestID, userID, symbol, side)

	// Get or create the lock for this position
	lockI, _ := m.locks.LoadOrStore(key, &positionLock{})
	lock := lockI.(*positionLock)

	// Create timeout context if the parent doesn't already have a deadline
	lockCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		lockCtx, cancel = context.WithTimeout(ctx, defaultLockTimeout)
		defer cancel()
	}

	// Fast path: try to acquire without blocking (no goroutine needed)
	if lock.mu.TryLock() {
		now := time.Now()
		lock.lastUsed = now
		lock.lockedAt = now
		return func() {
			lock.lockedAt = time.Time{}
			lock.mu.Unlock()
		}, nil
	}

	// Slow path: lock is held — spawn goroutine and wait with timeout
	contentionStart := time.Now()
	acquired := make(chan struct{})
	infra.SafeGo(m.logger, "position-lock-acquire", func() {
		lock.mu.Lock()
		close(acquired)
	})

	select {
	case <-acquired:
		// Lock acquired after waiting — record contention if significant
		waited := time.Since(contentionStart)
		if waited >= contentionThreshold {
			if m.contentionCounter != nil {
				m.contentionCounter.WithLabelValues(contestID, symbol).Inc()
			}
			if m.logger != nil {
				m.logger.Debug("Position lock contention detected",
					zap.String("lock_key", key),
					zap.String("contest_id", contestID),
					zap.String("symbol", symbol),
					zap.Duration("waited", waited))
			}
		}
	case <-lockCtx.Done():
		// Timeout or context cancelled — launch bounded cleanup goroutine
		if m.logger != nil {
			m.logger.Warn("Position lock timeout",
				zap.String("lock_key", key),
				zap.String("contest_id", contestID),
				zap.String("user_id", userID),
				zap.String("symbol", symbol),
				zap.String("side", side))
		}
		// Bounded cleanup: wait up to cleanupGoroutineTimeout, then give up.
		// This prevents permanent goroutine leaks when a lock is truly deadlocked.
		infra.SafeGo(m.logger, "position-lock-cleanup", func() {
			cleanupTimer := time.NewTimer(cleanupGoroutineTimeout)
			defer cleanupTimer.Stop()
			select {
			case <-acquired:
				lock.lockedAt = time.Time{}
				lock.mu.Unlock()
			case <-cleanupTimer.C:
				if m.logger != nil {
					m.logger.Error("Position lock cleanup goroutine timed out — possible deadlock",
						zap.String("lock_key", key),
						zap.Duration("cleanup_timeout", cleanupGoroutineTimeout))
				}
			}
		})
		return nil, ErrLockTimeout
	}

	// Update last used time and lock acquisition time
	now := time.Now()
	lock.lastUsed = now
	lock.lockedAt = now

	return func() {
		lock.lockedAt = time.Time{}
		lock.mu.Unlock()
	}, nil
}

// AcquireLockForSymbol acquires locks for all possible sides of a position.
// This is used when we don't know the position side yet (e.g., new position creation).
// Returns an unlock function that releases all acquired locks.
//
// Deprecated: Use AcquireLockForSymbolWithTimeout instead, which properly returns errors.
// This function swallows lock timeout errors and returns a no-op, causing callers
// to proceed without holding the lock.
func (m *PositionLockManager) AcquireLockForSymbol(contestID, userID, symbol string) func() {
	// Acquire both long and short locks in lexicographic order to prevent deadlocks
	unlockLong := m.AcquireLock(contestID, userID, symbol, "long")
	unlockShort := m.AcquireLock(contestID, userID, symbol, "short")

	return func() {
		unlockShort()
		unlockLong()
	}
}

// AcquireLockForSymbolWithTimeout acquires locks for all sides with a timeout.
// Returns an unlock function and nil error on success, or ErrLockTimeout on timeout.
func (m *PositionLockManager) AcquireLockForSymbolWithTimeout(ctx context.Context, contestID, userID, symbol string) (func(), error) {
	// Acquire both long and short locks in lexicographic order to prevent deadlocks
	unlockLong, err := m.AcquireLockWithTimeout(ctx, contestID, userID, symbol, "long")
	if err != nil {
		return nil, err
	}
	unlockShort, err := m.AcquireLockWithTimeout(ctx, contestID, userID, symbol, "short")
	if err != nil {
		unlockLong() // Release already acquired lock
		return nil, err
	}

	return func() {
		unlockShort()
		unlockLong()
	}, nil
}

// StartCleanup starts the periodic cleanup goroutine.
func (m *PositionLockManager) StartCleanup(ctx context.Context) {
	go m.cleanupLoop(ctx)
}

// StopCleanup stops the cleanup goroutine.
func (m *PositionLockManager) StopCleanup() {
	close(m.stopCleanup)
}

// cleanupLoop periodically removes locks for positions that have been closed.
func (m *PositionLockManager) cleanupLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			m.logger.Error("position lock cleanupLoop panicked",
				zap.Any("panic", r),
				zap.String("stack", string(debug.Stack())))
		}
	}()
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCleanup:
			return
		case <-ticker.C:
			m.cleanupClosedPositionLocks(ctx)
		}
	}
}

// cleanupClosedPositionLocks removes locks for positions that no longer exist
// and detects locks held for too long (possible deadlock).
func (m *PositionLockManager) cleanupClosedPositionLocks(ctx context.Context) {
	if m.db == nil {
		return
	}

	// Detect locks held for too long (>30s warning)
	longHeldThreshold := time.Now().Add(-30 * time.Second)
	m.locks.Range(func(keyI, valueI interface{}) bool {
		key := keyI.(string)
		lock := valueI.(*positionLock)

		// Use lockedAt (set on acquire, zeroed on release) for deadlock detection.
		// A zero lockedAt means the lock is not currently held.
		if !lock.lockedAt.IsZero() && lock.lockedAt.Before(longHeldThreshold) {
			// Try to acquire - if we can't, it's still held by the same holder
			if !lock.mu.TryLock() {
				if m.logger != nil {
					m.logger.Warn("Position lock held for >30s (possible deadlock)",
						zap.String("lock_key", key),
						zap.Duration("held_for", time.Since(lock.lockedAt)))
				}
			} else {
				lock.mu.Unlock()
			}
		}
		return true
	})

	keysToDelete := make([]string, 0)
	staleThreshold := time.Now().Add(-10 * time.Minute)

	// Collect keys that are stale (not used recently)
	m.locks.Range(func(keyI, valueI interface{}) bool {
		key := keyI.(string)
		lock := valueI.(*positionLock)

		// Only consider cleaning up locks that haven't been used recently
		if lock.lastUsed.Before(staleThreshold) {
			// Try to acquire the lock without blocking
			if lock.mu.TryLock() {
				// We got the lock, so it's not in use
				keysToDelete = append(keysToDelete, key)
				lock.mu.Unlock()
			}
		}
		return true
	})

	// Batch check: collect parsed keys and query DB in one go
	type lockCandidate struct {
		key       string
		contestID string
		userID    string
		symbol    string
	}
	candidates := make([]lockCandidate, 0, len(keysToDelete))
	for _, key := range keysToDelete {
		contestID, userID, symbol, _ := parsePositionLockKey(key)
		if contestID == "" {
			continue
		}
		candidates = append(candidates, lockCandidate{key, contestID, userID, symbol})
	}

	if len(candidates) == 0 {
		return
	}

	// Build a batch query to check all positions at once.
	// Returns rows for positions that ARE open, so we can delete the rest.
	openSet := make(map[string]bool) // "contestID:userID:symbol" -> has open position

	// Build parameterized tuple query: WHERE (contest_id, user_id, symbol) IN (($1,$2,$3), ($4,$5,$6), ...)
	args := make([]interface{}, 0, len(candidates)*3)
	tuples := make([]string, 0, len(candidates))
	for i, c := range candidates {
		base := i*3 + 1
		tuples = append(tuples, fmt.Sprintf("($%d,$%d,$%d)", base, base+1, base+2))
		args = append(args, c.contestID, c.userID, c.symbol)
	}

	query := `SELECT DISTINCT contest_id, user_id, symbol FROM positions
		WHERE closed_at IS NULL
		AND (contest_id, user_id, symbol) IN (` + strings.Join(tuples, ",") + `)`

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		if m.logger != nil {
			m.logger.Warn("Batch position check failed, falling back to individual checks",
				zap.Error(err))
		}
		// Fallback: check individually
		for _, c := range candidates {
			hasOpen, checkErr := m.checkOpenPosition(ctx, c.contestID, c.userID, c.symbol)
			if checkErr != nil {
				continue
			}
			if !hasOpen {
				m.locks.Delete(c.key)
			}
		}
		return
	}
	defer rows.Close()

	for rows.Next() {
		var cID, uID, sym string
		if err := rows.Scan(&cID, &uID, &sym); err != nil {
			continue
		}
		openSet[cID+":"+uID+":"+sym] = true
	}

	// Delete locks for positions that are NOT open
	for _, c := range candidates {
		if !openSet[c.contestID+":"+c.userID+":"+c.symbol] {
			m.locks.Delete(c.key)
			if m.logger != nil {
				m.logger.Debug("Cleaned up lock for closed position",
					zap.String("lock_key", c.key),
					zap.String("contest_id", c.contestID),
					zap.String("user_id", c.userID),
					zap.String("symbol", c.symbol))
			}
		}
	}
}

// parsePositionLockKey parses a lock key back into its components.
func parsePositionLockKey(key string) (contestID, userID, symbol, side string) {
	// Key format: {contest_id}:{user_id}:{symbol}:{side}
	// We need to handle symbols that might contain colons (unlikely but possible)
	parts := make([]string, 0, 4)
	current := ""
	for _, c := range key {
		if c == ':' && len(parts) < 3 {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	parts = append(parts, current)

	if len(parts) >= 4 {
		return parts[0], parts[1], parts[2], parts[3]
	}
	return "", "", "", ""
}

// checkOpenPosition checks if there's an open position for the given parameters.
func (m *PositionLockManager) checkOpenPosition(ctx context.Context, contestID, userID, symbol string) (bool, error) {
	var exists bool
	err := m.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM positions
			WHERE contest_id = $1 AND user_id = $2 AND symbol = $3 AND closed_at IS NULL
		)
	`, contestID, userID, symbol).Scan(&exists)
	return exists, err
}

// GetLockCount returns the approximate number of locks currently held.
// This is useful for monitoring and debugging.
func (m *PositionLockManager) GetLockCount() int {
	count := 0
	m.locks.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
