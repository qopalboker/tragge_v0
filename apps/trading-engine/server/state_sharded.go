package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shopspring/decimal"
	"github.com/redis/go-redis/v9"
)

// Common errors for shard management
var (
	ErrContestNotAssigned = errors.New("contest not assigned to this shard")
	ErrShardNotReady      = errors.New("shard not ready")
)

// ShardStats contains statistics for the shard.
type ShardStats struct {
	ShardID              int           `json:"shard_id"`
	ShardCount           int           `json:"shard_count"`
	AssignedContests     int           `json:"assigned_contests"`
	LoadedContests       int           `json:"loaded_contests"`
	TotalUsers           int           `json:"total_users"`
	TotalPositions       int           `json:"total_positions"`
	TotalPendingOrders   int           `json:"total_pending_orders"`
	LastAssignmentReload time.Time     `json:"last_assignment_reload"`
	WarmUpDuration       time.Duration `json:"warmup_duration"`
	Ready                bool          `json:"ready"`
}

// Prometheus metrics for shard monitoring
var (
	shardAssignedContests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "trading_engine_shard_assigned_contests",
			Help: "Number of contests assigned to this shard",
		},
		[]string{"shard_id"},
	)

	shardLoadedContests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "trading_engine_shard_loaded_contests",
			Help: "Number of contests currently loaded in memory",
		},
		[]string{"shard_id"},
	)

	shardContestRejections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "trading_engine_shard_contest_rejections_total",
			Help: "Total number of orders rejected due to wrong shard",
		},
		[]string{"shard_id"},
	)

	shardWarmupDuration = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "trading_engine_shard_warmup_duration_seconds",
			Help: "Duration of the last warmup operation in seconds",
		},
		[]string{"shard_id"},
	)

	shardCacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "trading_engine_shard_cache_hits_total",
			Help: "Total number of cache hits for shard assignments",
		},
		[]string{"shard_id"},
	)

	shardCacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "trading_engine_shard_cache_misses_total",
			Help: "Total number of cache misses for shard assignments",
		},
		[]string{"shard_id"},
	)
)

// ShardedStateManager manages contest states with shard isolation.
// Each trading engine instance only handles contests assigned to its shard.
type ShardedStateManager struct {
	shardID    int
	shardCount int

	// Contest states - only for assigned contests
	contests map[string]*ContestState
	mu       sync.RWMutex

	// Assigned contests loaded from database
	assignedContests map[string]bool
	assignmentMu     sync.RWMutex

	// Dependencies
	dbPool      *db.Pool
	redisClient redis.UniversalClient

	// State
	ready                bool
	lastAssignmentReload time.Time
	warmUpDuration       time.Duration

	// Redis key prefix for caching
	cacheKeyPrefix string
	cacheTTL       time.Duration
}

// NewShardedStateManager creates a new sharded state manager.
func NewShardedStateManager(shardID, shardCount int, dbPool *db.Pool, redisClient redis.UniversalClient) *ShardedStateManager {
	sm := &ShardedStateManager{
		shardID:          shardID,
		shardCount:       shardCount,
		contests:         make(map[string]*ContestState),
		assignedContests: make(map[string]bool),
		dbPool:           dbPool,
		redisClient:      redisClient,
		cacheKeyPrefix:   "contest:shard:",
		cacheTTL:         1 * time.Hour,
	}

	// Initialize metrics with shard ID
	shardLabel := fmt.Sprintf("%d", shardID)
	shardAssignedContests.WithLabelValues(shardLabel).Set(0)
	shardLoadedContests.WithLabelValues(shardLabel).Set(0)

	return sm
}

// IsAssigned checks if a contest is assigned to this shard.
func (sm *ShardedStateManager) IsAssigned(contestID string) bool {
	// First check in-memory cache
	sm.assignmentMu.RLock()
	assigned, exists := sm.assignedContests[contestID]
	sm.assignmentMu.RUnlock()

	if exists {
		return assigned
	}

	// Cache miss - check Redis cache
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	shardLabel := fmt.Sprintf("%d", sm.shardID)

	if sm.redisClient != nil {
		key := sm.cacheKeyPrefix + contestID
		cachedShard, err := sm.redisClient.Get(ctx, key).Int()
		if err == nil {
			shardCacheHits.WithLabelValues(shardLabel).Inc()
			isAssigned := cachedShard == sm.shardID
			// Update in-memory cache
			sm.assignmentMu.Lock()
			sm.assignedContests[contestID] = isAssigned
			sm.assignmentMu.Unlock()
			return isAssigned
		}
		if !errors.Is(err, redis.Nil) {
			// Log error but continue with DB lookup
		}
	}

	shardCacheMisses.WithLabelValues(shardLabel).Inc()

	// Cache miss - check database
	isAssigned, err := sm.checkAssignmentInDB(ctx, contestID)
	if err != nil {
		// On error, calculate based on hash
		isAssigned = sm.calculateShardAssignment(contestID) == sm.shardID
	}

	// Update caches
	sm.assignmentMu.Lock()
	sm.assignedContests[contestID] = isAssigned
	sm.assignmentMu.Unlock()

	if sm.redisClient != nil && isAssigned {
		sm.cacheAssignment(contestID, sm.shardID)
	}

	return isAssigned
}

// checkAssignmentInDB checks if a contest is assigned to this shard in the database.
func (sm *ShardedStateManager) checkAssignmentInDB(ctx context.Context, contestID string) (bool, error) {
	if sm.dbPool == nil {
		return false, errors.New("database pool not available")
	}

	var assignedShard int
	err := sm.dbPool.Primary().QueryRowContext(ctx, `
		SELECT COALESCE(shard_id, -1)
		FROM contests
		WHERE id = $1
	`, contestID).Scan(&assignedShard)

	if err == sql.ErrNoRows {
		// Contest doesn't exist - calculate based on hash
		return sm.calculateShardAssignment(contestID) == sm.shardID, nil
	}
	if err != nil {
		return false, fmt.Errorf("query contest shard: %w", err)
	}

	// If shard_id is -1 (NULL), calculate based on hash
	if assignedShard == -1 {
		return sm.calculateShardAssignment(contestID) == sm.shardID, nil
	}

	return assignedShard == sm.shardID, nil
}

// calculateShardAssignment calculates which shard a contest belongs to using consistent hashing.
func (sm *ShardedStateManager) calculateShardAssignment(contestID string) int {
	// Simple hash-based assignment
	var hash uint32
	for _, c := range contestID {
		hash = hash*31 + uint32(c)
	}
	return int(hash % uint32(sm.shardCount))
}

// LoadAssignments loads contest assignments from the database.
func (sm *ShardedStateManager) LoadAssignments(ctx context.Context) error {
	if sm.dbPool == nil {
		return errors.New("database pool not available")
	}

	// Query all contests assigned to this shard
	rows, err := sm.dbPool.Primary().QueryContext(ctx, `
		SELECT id
		FROM contests
		WHERE (shard_id = $1 OR shard_id IS NULL)
		  AND status IN ('scheduled', 'registration_open', 'running', 'paused')
	`, sm.shardID)
	if err != nil {
		return fmt.Errorf("query assigned contests: %w", err)
	}
	defer rows.Close()

	assignments := make(map[string]bool)
	for rows.Next() {
		var contestID string
		if err := rows.Scan(&contestID); err != nil {
			return fmt.Errorf("scan contest_id: %w", err)
		}

		// For contests with NULL shard_id, check if they belong to us based on hash
		if sm.calculateShardAssignment(contestID) == sm.shardID {
			assignments[contestID] = true
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate contest rows: %w", err)
	}

	// Also query explicitly assigned contests
	rowsExplicit, err := sm.dbPool.Primary().QueryContext(ctx, `
		SELECT id
		FROM contests
		WHERE shard_id = $1
		  AND status IN ('scheduled', 'registration_open', 'running', 'paused')
	`, sm.shardID)
	if err != nil {
		return fmt.Errorf("query explicitly assigned contests: %w", err)
	}
	defer rowsExplicit.Close()

	for rowsExplicit.Next() {
		var contestID string
		if err := rowsExplicit.Scan(&contestID); err != nil {
			return fmt.Errorf("scan explicit contest_id: %w", err)
		}
		assignments[contestID] = true
	}

	// Update assignments
	sm.assignmentMu.Lock()
	sm.assignedContests = assignments
	sm.lastAssignmentReload = time.Now()
	sm.assignmentMu.Unlock()

	// Update metrics
	shardLabel := fmt.Sprintf("%d", sm.shardID)
	shardAssignedContests.WithLabelValues(shardLabel).Set(float64(len(assignments)))

	// Cache all assignments in Redis
	if sm.redisClient != nil {
		for contestID := range assignments {
			sm.cacheAssignment(contestID, sm.shardID)
		}
	}

	return nil
}

// cacheAssignment caches a contest-to-shard assignment in Redis.
func (sm *ShardedStateManager) cacheAssignment(contestID string, shardID int) {
	if sm.redisClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	key := sm.cacheKeyPrefix + contestID
	sm.redisClient.Set(ctx, key, shardID, sm.cacheTTL)
}

// GetOrCreateContest returns the contest state for an assigned contest, creating if needed.
func (sm *ShardedStateManager) GetOrCreateContest(contestID string) (*ContestState, error) {
	if !sm.IsAssigned(contestID) {
		return nil, ErrContestNotAssigned
	}

	sm.mu.RLock()
	cs, exists := sm.contests[contestID]
	sm.mu.RUnlock()

	if exists {
		return cs, nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Double-check after acquiring write lock
	if cs, exists = sm.contests[contestID]; exists {
		return cs, nil
	}

	cs = &ContestState{
		ContestID: contestID,
		Users:     make(map[string]*UserState),
	}
	sm.contests[contestID] = cs

	// Update metrics
	shardLabel := fmt.Sprintf("%d", sm.shardID)
	shardLoadedContests.WithLabelValues(shardLabel).Set(float64(len(sm.contests)))

	return cs, nil
}

// RejectIfNotAssigned returns an error if the contest is not assigned to this shard.
func (sm *ShardedStateManager) RejectIfNotAssigned(contestID string) error {
	if !sm.ready {
		return ErrShardNotReady
	}

	if !sm.IsAssigned(contestID) {
		shardLabel := fmt.Sprintf("%d", sm.shardID)
		shardContestRejections.WithLabelValues(shardLabel).Inc()
		return fmt.Errorf("%w: contest %s should be on shard %d, not %d",
			ErrContestNotAssigned, contestID, sm.calculateShardAssignment(contestID), sm.shardID)
	}

	return nil
}

// WarmUp loads active contest states from the database.
func (sm *ShardedStateManager) WarmUp(ctx context.Context) error {
	startTime := time.Now()

	// First, load assignments
	if err := sm.LoadAssignments(ctx); err != nil {
		return fmt.Errorf("load assignments: %w", err)
	}

	sm.assignmentMu.RLock()
	contestIDs := make([]string, 0, len(sm.assignedContests))
	for contestID := range sm.assignedContests {
		contestIDs = append(contestIDs, contestID)
	}
	sm.assignmentMu.RUnlock()

	// Load state for each assigned contest
	for _, contestID := range contestIDs {
		if err := sm.warmUpContest(ctx, contestID); err != nil {
			// Log error but continue with other contests
			continue
		}
	}

	sm.warmUpDuration = time.Since(startTime)
	sm.ready = true

	// Update metrics
	shardLabel := fmt.Sprintf("%d", sm.shardID)
	shardWarmupDuration.WithLabelValues(shardLabel).Set(sm.warmUpDuration.Seconds())

	return nil
}

// warmUpContest loads the state for a single contest.
func (sm *ShardedStateManager) warmUpContest(ctx context.Context, contestID string) error {
	if sm.dbPool == nil {
		return errors.New("database pool not available")
	}

	// Get or create contest state
	contestState, err := sm.GetOrCreateContest(contestID)
	if err != nil {
		return err
	}

	// Load all participants for this contest
	rows, err := sm.dbPool.Primary().QueryContext(ctx, `
		SELECT user_id, qty_total, qty_available, total_score
		FROM contest_participants
		WHERE contest_id = $1
	`, contestID)
	if err != nil {
		return fmt.Errorf("query participants: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID string
		var qtyTotal, qtyAvailable int64
		var totalScore float64

		if err := rows.Scan(&userID, &qtyTotal, &qtyAvailable, &totalScore); err != nil {
			continue
		}

		// Create user state
		userState := contestState.GetOrCreateUser(userID, qtyTotal, qtyAvailable, totalScore)
		userState.mu.Lock()
		userState.RealizedScoreDecimal = decimal.NewFromFloat(totalScore)
		userState.mu.Unlock()

		// Load open positions for this user
		posRows, err := sm.dbPool.Primary().QueryContext(ctx, `
			SELECT position_id, symbol, side, qty_open, entry_price, qty_used, realized_score
			FROM positions
			WHERE contest_id = $1 AND user_id = $2 AND closed_at IS NULL
		`, contestID, userID)
		if err != nil {
			continue
		}

		for posRows.Next() {
			var pos DBPosition
			var sideStr string
			if err := posRows.Scan(&pos.PositionID, &pos.Symbol, &sideStr,
				&pos.QtyOpen, &pos.EntryPrice, &pos.QtyUsed, &pos.RealizedScore); err != nil {
				continue
			}

			posState := &PositionState{
				PositionID:           pos.PositionID,
				Symbol:               pos.Symbol,
				QtyOpen:              pos.QtyOpen,
				EntryPrice:           pos.EntryPrice,
				EntryPriceDecimal:    decimal.NewFromFloat(pos.EntryPrice),
				QtyUsed:              pos.QtyUsed,
				RealizedScore:        pos.RealizedScore,
				RealizedScoreDecimal: decimal.NewFromFloat(pos.RealizedScore),
			}

			// Convert side string to OrderSide
			posState.Side = PositionSideToOrderSide(sideStr)

			userState.SetPosition(posState)
		}
		posRows.Close()

		// Load pending orders for this user
		pendingRows, err := sm.dbPool.Primary().QueryContext(ctx, `
			SELECT order_id, symbol, side, type, qty, qty_filled, limit_price, stop_price
			FROM orders
			WHERE contest_id = $1 AND user_id = $2 AND status = 'open'
		`, contestID, userID)
		if err != nil {
			continue
		}

		for pendingRows.Next() {
			var orderID, symbol, sideStr, typeStr string
			var qty, qtyFilled int64
			var limitPrice, stopPrice sql.NullFloat64

			if err := pendingRows.Scan(&orderID, &symbol, &sideStr, &typeStr,
				&qty, &qtyFilled, &limitPrice, &stopPrice); err != nil {
				continue
			}

			pending := &PendingOrder{
				OrderID:   orderID,
				Symbol:    symbol,
				Qty:       qty,
				QtyFilled: qtyFilled,
			}

			// Convert side
			pending.Side = DBOrderSideToOrderSide(sideStr)

			// Convert type
			switch typeStr {
			case "limit":
				pending.Type = contracts.OrderTypeLimit
			case "stop":
				pending.Type = contracts.OrderTypeStop
			}

			if limitPrice.Valid {
				lp := limitPrice.Float64
				pending.LimitPrice = &lp
			}
			if stopPrice.Valid {
				sp := stopPrice.Float64
				pending.StopPrice = &sp
			}

			userState.AddPendingOrder(pending)
		}
		pendingRows.Close()
	}

	return rows.Err()
}

// EvictContest removes a contest from the in-memory state.
func (sm *ShardedStateManager) EvictContest(contestID string) {
	sm.mu.Lock()
	delete(sm.contests, contestID)
	sm.mu.Unlock()

	sm.assignmentMu.Lock()
	delete(sm.assignedContests, contestID)
	sm.assignmentMu.Unlock()

	// Update metrics
	sm.mu.RLock()
	shardLabel := fmt.Sprintf("%d", sm.shardID)
	shardLoadedContests.WithLabelValues(shardLabel).Set(float64(len(sm.contests)))
	sm.mu.RUnlock()

	// Remove from Redis cache
	if sm.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		sm.redisClient.Del(ctx, sm.cacheKeyPrefix+contestID)
	}
}

// GetStats returns statistics about the shard.
func (sm *ShardedStateManager) GetStats() ShardStats {
	sm.mu.RLock()
	loadedContests := len(sm.contests)

	var totalUsers, totalPositions, totalPendingOrders int
	for _, cs := range sm.contests {
		cs.mu.RLock()
		totalUsers += len(cs.Users)
		for _, us := range cs.Users {
			us.mu.RLock()
			totalPositions += len(us.Positions)
			totalPendingOrders += len(us.PendingOrders)
			us.mu.RUnlock()
		}
		cs.mu.RUnlock()
	}
	sm.mu.RUnlock()

	sm.assignmentMu.RLock()
	assignedContests := len(sm.assignedContests)
	lastReload := sm.lastAssignmentReload
	sm.assignmentMu.RUnlock()

	return ShardStats{
		ShardID:              sm.shardID,
		ShardCount:           sm.shardCount,
		AssignedContests:     assignedContests,
		LoadedContests:       loadedContests,
		TotalUsers:           totalUsers,
		TotalPositions:       totalPositions,
		TotalPendingOrders:   totalPendingOrders,
		LastAssignmentReload: lastReload,
		WarmUpDuration:       sm.warmUpDuration,
		Ready:                sm.ready,
	}
}

// GetShardID returns the shard ID.
func (sm *ShardedStateManager) GetShardID() int {
	return sm.shardID
}

// GetShardCount returns the total number of shards.
func (sm *ShardedStateManager) GetShardCount() int {
	return sm.shardCount
}

// IsReady returns whether the shard is ready to process requests.
func (sm *ShardedStateManager) IsReady() bool {
	return sm.ready
}

// RefreshAssignments reloads contest assignments from the database.
func (sm *ShardedStateManager) RefreshAssignments(ctx context.Context) error {
	return sm.LoadAssignments(ctx)
}

// AddContest manually adds a contest to this shard's assignments.
// This is useful for dynamic contest creation.
func (sm *ShardedStateManager) AddContest(contestID string) {
	sm.assignmentMu.Lock()
	sm.assignedContests[contestID] = true
	sm.assignmentMu.Unlock()

	// Update metrics
	sm.assignmentMu.RLock()
	shardLabel := fmt.Sprintf("%d", sm.shardID)
	shardAssignedContests.WithLabelValues(shardLabel).Set(float64(len(sm.assignedContests)))
	sm.assignmentMu.RUnlock()

	// Cache in Redis
	sm.cacheAssignment(contestID, sm.shardID)
}

// GetContest returns the contest state if it exists and is assigned.
func (sm *ShardedStateManager) GetContest(contestID string) (*ContestState, bool) {
	if !sm.IsAssigned(contestID) {
		return nil, false
	}

	sm.mu.RLock()
	cs, exists := sm.contests[contestID]
	sm.mu.RUnlock()

	return cs, exists
}

// ForEachContest iterates over all loaded contests in this shard.
func (sm *ShardedStateManager) ForEachContest(fn func(contestID string, cs *ContestState)) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for contestID, cs := range sm.contests {
		fn(contestID, cs)
	}
}

// GetAssignedContestIDs returns a list of all contest IDs assigned to this shard.
func (sm *ShardedStateManager) GetAssignedContestIDs() []string {
	sm.assignmentMu.RLock()
	defer sm.assignmentMu.RUnlock()
	ids := make([]string, 0, len(sm.assignedContests))
	for contestID := range sm.assignedContests {
		ids = append(ids, contestID)
	}
	return ids
}

// StateManagerAdapter wraps ShardedStateManager to provide backward compatibility
// with the original StateManager interface.
type StateManagerAdapter struct {
	sharded *ShardedStateManager
}

// NewStateManagerAdapter creates an adapter for ShardedStateManager.
func NewStateManagerAdapter(sm *ShardedStateManager) *StateManagerAdapter {
	return &StateManagerAdapter{sharded: sm}
}

// GetOrCreateContest provides backward-compatible access to contest state.
// It returns the contest state if assigned, or nil if not.
func (a *StateManagerAdapter) GetOrCreateContest(contestID string) *ContestState {
	cs, err := a.sharded.GetOrCreateContest(contestID)
	if err != nil {
		return nil
	}
	return cs
}

// RemoveContest removes a contest's state via the sharded manager's EvictContest.
func (a *StateManagerAdapter) RemoveContest(contestID string) {
	a.sharded.EvictContest(contestID)
}
