package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Common lock errors
var (
	ErrLockNotAcquired = errors.New("lock not acquired")
	ErrLockNotOwned    = errors.New("lock not owned by this instance")
)

// DistributedLock provides distributed locking using Redis.
// It implements a simple lock with automatic expiration to prevent deadlocks.
type DistributedLock struct {
	client     redis.UniversalClient
	logger     *zap.Logger
	instanceID string
	ttl        time.Duration
}

// LockConfig contains configuration for distributed locking.
type LockConfig struct {
	TTL           time.Duration // Lock expiration time
	RetryInterval time.Duration // Interval between lock acquisition retries
	InstanceID    string        // Unique identifier for this instance
}

// NewDistributedLock creates a new distributed lock manager.
func NewDistributedLock(client redis.UniversalClient, cfg LockConfig, logger *zap.Logger) *DistributedLock {
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.TTL == 0 {
		cfg.TTL = 60 * time.Second
	}
	if cfg.InstanceID == "" {
		cfg.InstanceID = generateInstanceID()
	}

	return &DistributedLock{
		client:     client,
		logger:     logger,
		instanceID: cfg.InstanceID,
		ttl:        cfg.TTL,
	}
}

// lockKey returns the Redis key for a contest lock.
func lockKey(contestID string) string {
	return fmt.Sprintf("contest:lock:%s", contestID)
}

// Acquire attempts to acquire a lock for the given contest.
// Returns true if the lock was acquired, false otherwise.
func (dl *DistributedLock) Acquire(ctx context.Context, contestID string) (bool, error) {
	key := lockKey(contestID)

	// SET key value NX EX ttl
	// NX = only set if not exists
	// EX = set expiration in seconds
	success, err := dl.client.SetNX(ctx, key, dl.instanceID, dl.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if success {
		dl.logger.Debug("Lock acquired",
			zap.String("contest_id", contestID),
			zap.String("instance_id", dl.instanceID))
	}

	return success, nil
}

// AcquireWithRetry attempts to acquire a lock with retries.
// It will retry until the context is cancelled or the lock is acquired.
func (dl *DistributedLock) AcquireWithRetry(ctx context.Context, contestID string, retryInterval time.Duration) (bool, error) {
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()

	// Try immediately first
	acquired, err := dl.Acquire(ctx, contestID)
	if err != nil || acquired {
		return acquired, err
	}

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
			acquired, err := dl.Acquire(ctx, contestID)
			if err != nil {
				return false, err
			}
			if acquired {
				return true, nil
			}
		}
	}
}

// Release releases a lock for the given contest.
// It only releases the lock if this instance owns it.
func (dl *DistributedLock) Release(ctx context.Context, contestID string) error {
	key := lockKey(contestID)

	// Use Lua script to atomically check ownership and delete
	// This prevents releasing a lock that has been acquired by another instance
	script := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, dl.client, []string{key}, dl.instanceID).Int64()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}

	if result == 0 {
		dl.logger.Warn("Attempted to release lock not owned by this instance",
			zap.String("contest_id", contestID),
			zap.String("instance_id", dl.instanceID))
		return ErrLockNotOwned
	}

	dl.logger.Debug("Lock released",
		zap.String("contest_id", contestID),
		zap.String("instance_id", dl.instanceID))

	return nil
}

// Extend extends the TTL of a lock that this instance owns.
// Returns true if the lock was extended, false if it's not owned by this instance.
func (dl *DistributedLock) Extend(ctx context.Context, contestID string) (bool, error) {
	key := lockKey(contestID)

	// Use Lua script to atomically check ownership and extend TTL
	script := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, dl.client, []string{key}, dl.instanceID, int64(dl.ttl/time.Millisecond)).Int64()
	if err != nil {
		return false, fmt.Errorf("failed to extend lock: %w", err)
	}

	return result == 1, nil
}

// IsHeldByMe checks if the lock is currently held by this instance.
func (dl *DistributedLock) IsHeldByMe(ctx context.Context, contestID string) (bool, error) {
	key := lockKey(contestID)

	value, err := dl.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check lock: %w", err)
	}

	return value == dl.instanceID, nil
}

// ForceRelease forcefully releases a lock regardless of ownership.
// Use with caution - only for administrative purposes.
func (dl *DistributedLock) ForceRelease(ctx context.Context, contestID string) error {
	key := lockKey(contestID)

	if err := dl.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to force release lock: %w", err)
	}

	dl.logger.Warn("Lock force released",
		zap.String("contest_id", contestID),
		zap.String("by_instance", dl.instanceID))

	return nil
}

// GetLockInfo returns information about who holds a lock.
func (dl *DistributedLock) GetLockInfo(ctx context.Context, contestID string) (*LockInfo, error) {
	key := lockKey(contestID)

	pipe := dl.client.Pipeline()
	getCmd := pipe.Get(ctx, key)
	ttlCmd := pipe.TTL(ctx, key)

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to get lock info: %w", err)
	}

	holder, err := getCmd.Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Lock not held
		}
		return nil, err
	}

	ttl, _ := ttlCmd.Result()

	return &LockInfo{
		ContestID: contestID,
		Holder:    holder,
		TTL:       ttl,
		IsHeldBy:  holder == dl.instanceID,
	}, nil
}

// LockInfo contains information about a lock.
type LockInfo struct {
	ContestID string
	Holder    string
	TTL       time.Duration
	IsHeldBy  bool // True if held by this instance
}

// ReleaseAll releases all locks held by this instance.
// This is useful during graceful shutdown.
func (dl *DistributedLock) ReleaseAll(ctx context.Context) error {
	// Scan for all contest locks held by this instance
	var cursor uint64
	var releasedCount int

	for {
		keys, nextCursor, err := dl.client.Scan(ctx, cursor, "contest:lock:*", 100).Result()
		if err != nil {
			return fmt.Errorf("failed to scan for locks: %w", err)
		}

		for _, key := range keys {
			// Check if we own this lock
			holder, err := dl.client.Get(ctx, key).Result()
			if err != nil {
				continue
			}

			if holder == dl.instanceID {
				// Extract contest ID from key (remove "contest:lock:" prefix)
				contestID := key[14:]
				if err := dl.Release(ctx, contestID); err == nil {
					releasedCount++
				}
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	if releasedCount > 0 {
		dl.logger.Info("Released all locks during shutdown",
			zap.Int("count", releasedCount),
			zap.String("instance_id", dl.instanceID))
	}

	return nil
}

// generateInstanceID generates a unique instance identifier.
func generateInstanceID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
