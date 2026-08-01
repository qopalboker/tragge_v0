package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/serialx/hashring"
	"go.uber.org/zap"
)

// ShardStatus represents the operational status of a shard.
type ShardStatus string

const (
	ShardStatusActive   ShardStatus = "active"
	ShardStatusDraining ShardStatus = "draining"
	ShardStatusInactive ShardStatus = "inactive"
)

// Shard represents a contest shard configuration.
type Shard struct {
	ID        string      `json:"shard_id"`
	Address   string      `json:"address"`
	Status    ShardStatus `json:"status"`
	Weight    int         `json:"weight"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// ShardAssignment represents the result of routing a contest to a shard.
type ShardAssignment struct {
	ShardID      string `json:"shard_id"`
	ShardAddress string `json:"address"`
}

// persistJob represents a shard assignment persistence task.
type persistJob struct {
	contestID  string
	oldShardID int
	newShardID string
	db         *sql.DB
}

// ShardRouter handles routing of contests to shards using consistent hashing.
type ShardRouter struct {
	mu           sync.RWMutex
	ring         *hashring.HashRing
	shards       map[string]*Shard
	virtualNodes int
	logger       *zap.Logger
	persistCh    chan persistJob
	persistDone  chan struct{}
}

// NewShardRouter creates a new ShardRouter instance.
func NewShardRouter(virtualNodes int, logger *zap.Logger) *ShardRouter {
	r := &ShardRouter{
		shards:       make(map[string]*Shard),
		virtualNodes: virtualNodes,
		logger:       logger.With(zap.String("component", "shard-router")),
		persistCh:    make(chan persistJob, 256),
		persistDone:  make(chan struct{}),
	}
	// Start a fixed pool of workers for persistence instead of spawning a goroutine per request
	const numWorkers = 4
	for i := 0; i < numWorkers; i++ {
		go r.persistWorker()
	}
	return r
}

// persistWorker drains the persistCh channel and writes assignments to DB.
func (r *ShardRouter) persistWorker() {
	for job := range r.persistCh {
		r.persistShardAssignment(job.contestID, job.oldShardID, job.newShardID, job.db)
	}
}

// StopPersistWorkers closes the persist channel and waits for in-flight jobs to drain.
func (r *ShardRouter) StopPersistWorkers() {
	close(r.persistCh)
}

// LoadShards loads shard configuration from the database.
// When merging with existing local state, locally-modified shards with a newer
// UpdatedAt timestamp are preserved to prevent stale replica reads from
// overwriting manual modifications (e.g. DrainShard).
func (r *ShardRouter) LoadShards(ctx context.Context, db *sql.DB) error {
	r.logger.Info("loading shards from database")

	query := `
		SELECT shard_id, address, status, weight, created_at, updated_at
		FROM shard_config
		WHERE status != 'inactive'
		ORDER BY shard_id
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	shards := make(map[string]*Shard)

	for rows.Next() {
		var shard Shard
		if err := rows.Scan(
			&shard.ID,
			&shard.Address,
			&shard.Status,
			&shard.Weight,
			&shard.CreatedAt,
			&shard.UpdatedAt,
		); err != nil {
			return err
		}
		shards[shard.ID] = &shard
	}

	if err := rows.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Merge: don't overwrite locally-modified shards with stale replica data.
	// If the local shard has a newer UpdatedAt, keep the local version.
	for id, existingShard := range r.shards {
		if newShard, ok := shards[id]; ok {
			if newShard.UpdatedAt.Before(existingShard.UpdatedAt) {
				shards[id] = existingShard
			}
		}
	}

	r.shards = shards
	r.rebuildRing()

	activeCount := 0
	for _, s := range r.shards {
		if s.Status == ShardStatusActive {
			activeCount++
		}
	}

	r.logger.Info("shards loaded",
		zap.Int("total_shards", len(shards)),
		zap.Int("active_shards", activeCount),
	)

	return nil
}

// RouteTo returns the shard assignment for a given contest ID.
func (r *ShardRouter) RouteTo(contestID string) (*ShardAssignment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.ring == nil {
		return nil, ErrNoShardsAvailable
	}

	shardID, ok := r.ring.GetNode(contestID)
	if !ok {
		return nil, ErrNoShardsAvailable
	}

	shard, exists := r.shards[shardID]
	if !exists {
		return nil, ErrShardNotFound
	}

	return &ShardAssignment{
		ShardID:      shard.ID,
		ShardAddress: shard.Address,
	}, nil
}

// activeContestStatuses are statuses where shard assignment must be sticky.
// Contests in these states should not be re-routed when the hash ring changes.
var activeContestStatuses = map[string]bool{
	"running":           true,
	"registration_open": true,
	"paused":            true,
}

// RouteToSticky returns the shard assignment for a contest, using a DB-pinned
// assignment for active contests to prevent mid-tournament re-routing.
// For new or unassigned contests, it falls back to the hash ring and persists
// the assignment to the database.
func (r *ShardRouter) RouteToSticky(ctx context.Context, db *sql.DB, contestID string) (*ShardAssignment, error) {
	// Step 1: Check if the contest has a pinned shard assignment in the DB
	var shardID int
	var contestStatus string
	err := db.QueryRowContext(ctx,
		`SELECT COALESCE(shard_id, 0), COALESCE(status, '') FROM contests WHERE id = $1`,
		contestID,
	).Scan(&shardID, &contestStatus)

	if err != nil && err != sql.ErrNoRows {
		r.logger.Warn("failed to query contest shard assignment, falling back to hash ring",
			zap.String("contest_id", contestID), zap.Error(err))
		// Fall through to hash ring on DB errors
	}

	// Step 2: If the contest has a valid pinned shard, use it
	if err == nil && shardID != 0 {
		shardIDStr := strconv.Itoa(shardID)
		r.mu.RLock()
		shard, exists := r.shards[shardIDStr]
		if exists && (shard.Status == ShardStatusActive || shard.Status == ShardStatusDraining) {
			r.mu.RUnlock()
			return &ShardAssignment{
				ShardID:      shard.ID,
				ShardAddress: shard.Address,
			}, nil
		}
		r.mu.RUnlock()
		// Pinned shard is unavailable — will reassign via hash ring below
		r.logger.Warn("pinned shard unavailable, reassigning via hash ring",
			zap.String("contest_id", contestID),
			zap.Int("old_shard_id", shardID))
	}

	// Step 3: Use hash ring for unassigned or new contests
	assignment, routeErr := r.RouteTo(contestID)
	if routeErr != nil {
		return nil, routeErr
	}

	// Step 4: Persist the assignment for active contests so they stick on future ring changes.
	// Send to bounded worker pool instead of spawning a goroutine per request.
	if activeContestStatuses[contestStatus] {
		select {
		case r.persistCh <- persistJob{contestID: contestID, oldShardID: shardID, newShardID: assignment.ShardID, db: db}:
		default:
			r.logger.Warn("persist channel full, dropping shard assignment persistence",
				zap.String("contest_id", contestID))
		}
	}

	return assignment, nil
}

// persistShardAssignment saves the shard assignment to the database and logs it
// in the shard_assignment_log for audit purposes.
func (r *ShardRouter) persistShardAssignment(contestID string, oldShardID int, newShardIDStr string, db *sql.DB) {
	newShardID, err := strconv.Atoi(newShardIDStr)
	if err != nil {
		r.logger.Error("invalid shard ID from hash ring", zap.String("shard_id", newShardIDStr))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		r.logger.Error("failed to begin tx for shard assignment", zap.Error(err))
		return
	}
	defer tx.Rollback()

	// Update contests.shard_id
	if _, err := tx.ExecContext(ctx,
		`UPDATE contests SET shard_id = $1 WHERE id = $2 AND (shard_id IS NULL OR shard_id != $1)`,
		newShardID, contestID,
	); err != nil {
		r.logger.Error("failed to persist shard assignment", zap.Error(err))
		return
	}

	// Insert audit log
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO shard_assignment_log (contest_id, old_shard_id, new_shard_id, reason) VALUES ($1, $2, $3, $4)`,
		contestID, oldShardID, newShardID, "hash_ring_assignment",
	); err != nil {
		r.logger.Error("failed to insert shard assignment log", zap.Error(err))
		return
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error("failed to commit shard assignment", zap.Error(err))
	}
}

// GetShard returns a shard by ID.
func (r *ShardRouter) GetShard(shardID string) (*Shard, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	shard, exists := r.shards[shardID]
	if !exists {
		return nil, ErrShardNotFound
	}

	// Return a copy to avoid race conditions
	shardCopy := *shard
	return &shardCopy, nil
}

// ListShards returns all shards.
func (r *ShardRouter) ListShards() []*Shard {
	r.mu.RLock()
	defer r.mu.RUnlock()

	shards := make([]*Shard, 0, len(r.shards))
	for _, shard := range r.shards {
		shardCopy := *shard
		shards = append(shards, &shardCopy)
	}
	return shards
}

// DrainShard marks a shard for draining.
func (r *ShardRouter) DrainShard(ctx context.Context, db *sql.DB, shardID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	shard, exists := r.shards[shardID]
	if !exists {
		return ErrShardNotFound
	}

	if shard.Status != ShardStatusActive {
		return ErrShardNotActive
	}

	// Update database FIRST — only modify local state after DB succeeds
	query := `
		UPDATE shard_config
		SET status = $1, updated_at = NOW()
		WHERE shard_id = $2
	`
	result, err := db.ExecContext(ctx, query, ShardStatusDraining, shardID)
	if err != nil {
		return fmt.Errorf("failed to update shard %s in database: %w", shardID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("shard %s not found in database", shardID)
	}

	// DB succeeded — now update local state
	shard.Status = ShardStatusDraining
	shard.UpdatedAt = time.Now()

	// Rebuild the hash ring without the draining shard
	r.rebuildRing()

	r.logger.Info("shard marked for draining",
		zap.String("shard_id", shardID),
	)

	return nil
}

// AddShard adds a new shard to the router.
func (r *ShardRouter) AddShard(ctx context.Context, db *sql.DB, shard *Shard) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.shards[shard.ID]; exists {
		return ErrShardAlreadyExists
	}

	// Insert into database
	query := `
		INSERT INTO shard_config (shard_id, address, status, weight, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
	`
	_, err := db.ExecContext(ctx, query, shard.ID, shard.Address, shard.Status, shard.Weight)
	if err != nil {
		return err
	}

	// Add to local state
	now := time.Now()
	shard.CreatedAt = now
	shard.UpdatedAt = now
	r.shards[shard.ID] = shard

	// Rebuild ring if shard is active
	if shard.Status == ShardStatusActive {
		r.rebuildRing()
	}

	r.logger.Info("shard added",
		zap.String("shard_id", shard.ID),
		zap.String("status", string(shard.Status)),
	)

	return nil
}

// RemoveShard removes a shard from the router.
func (r *ShardRouter) RemoveShard(ctx context.Context, db *sql.DB, shardID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.shards[shardID]; !exists {
		return ErrShardNotFound
	}

	// Update in database (mark as inactive rather than delete)
	query := `
		UPDATE shard_config
		SET status = $1, updated_at = NOW()
		WHERE shard_id = $2
	`
	_, err := db.ExecContext(ctx, query, ShardStatusInactive, shardID)
	if err != nil {
		return err
	}

	// Remove from local state
	delete(r.shards, shardID)

	// Rebuild the hash ring
	r.rebuildRing()

	r.logger.Info("shard removed",
		zap.String("shard_id", shardID),
	)

	return nil
}

// rebuildRing rebuilds the hash ring from the current shard state.
// Must be called with the lock held.
func (r *ShardRouter) rebuildRing() {
	weights := make(map[string]int)
	for _, shard := range r.shards {
		if shard.Status == ShardStatusActive {
			weights[shard.ID] = shard.Weight
		}
	}

	if len(weights) > 0 {
		r.ring = hashring.NewWithWeights(weights)
	} else {
		r.ring = nil
	}
}

// ShardCount returns the number of active shards.
func (r *ShardRouter) ShardCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, shard := range r.shards {
		if shard.Status == ShardStatusActive {
			count++
		}
	}
	return count
}

// IsHealthy returns true if there is at least one active shard.
func (r *ShardRouter) IsHealthy() bool {
	return r.ShardCount() > 0
}

// Errors
var (
	ErrNoShardsAvailable  = errors.New("no shards available")
	ErrShardNotFound      = errors.New("shard not found")
	ErrShardNotActive     = errors.New("shard is not active")
	ErrShardAlreadyExists = errors.New("shard already exists")
)
