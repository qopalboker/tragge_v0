package server

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"runtime/debug"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/redis/go-redis/v9"
)

// ShardedLeaderboardWorker handles cross-shard leaderboard operations.
// It uses Redis Cluster with hash tags to ensure contest data stays on the same slot.
type ShardedLeaderboardWorker struct {
	redisClient *pkgredis.Client
	shardCount  int

	// Cache for aggregated results
	cacheMu      sync.RWMutex
	globalCache  map[string]*CachedLeaderboard
	cacheTTL     time.Duration
	cacheInvalid map[string]bool // Tracks contests with significant score changes

	// Global ranking cache (per-instance to avoid shared state between workers)
	globalRankingCache     []GlobalRankEntry
	globalRankingCacheTime time.Time
	globalRankingMu        sync.RWMutex
}

// CachedLeaderboard holds cached leaderboard data with expiration.
type CachedLeaderboard struct {
	Entries   []LeaderboardEntry
	CachedAt  time.Time
	ExpiresAt time.Time
}

// GlobalRankEntry represents a user's ranking across all contests.
type GlobalRankEntry struct {
	Rank        int     `json:"rank"`
	UserID      string  `json:"user_id"`
	ContestID   string  `json:"contest_id"`
	Score       float64 `json:"score"`
	ContestRank int     `json:"contest_rank"`
}

// ShardedLeaderboardConfig holds configuration for the sharded worker.
type ShardedLeaderboardConfig struct {
	ShardCount             int
	CacheTTL               time.Duration
	SignificantScoreChange float64 // Score delta that triggers cache invalidation
}

// DefaultShardedLeaderboardConfig returns sensible defaults.
func DefaultShardedLeaderboardConfig() ShardedLeaderboardConfig {
	return ShardedLeaderboardConfig{
		ShardCount:             6,
		CacheTTL:               30 * time.Second,
		SignificantScoreChange: 100.0, // $100 change invalidates cache
	}
}

// NewShardedLeaderboardWorker creates a new sharded leaderboard worker.
func NewShardedLeaderboardWorker(client *pkgredis.Client, cfg ShardedLeaderboardConfig) *ShardedLeaderboardWorker {
	return &ShardedLeaderboardWorker{
		redisClient:  client,
		shardCount:   cfg.ShardCount,
		globalCache:  make(map[string]*CachedLeaderboard),
		cacheTTL:     cfg.CacheTTL,
		cacheInvalid: make(map[string]bool),
	}
}

// LeaderboardKey returns the Redis key for a contest's leaderboard.
// Uses hash tags {contestID} to ensure all contest data stays on the same slot.
func LeaderboardKey(contestID string) string {
	return fmt.Sprintf("lb:{%s}", contestID)
}

// ShardLeaderboardKey returns the key for a shard-specific leaderboard.
// Uses hash tags to keep shard data together.
func ShardLeaderboardKey(contestID string, shardID int) string {
	return fmt.Sprintf("lb:{%s}:shard:%d", contestID, shardID)
}

// encodeTiebreaker encodes a time-based tiebreaker into the score.
// Earlier achievers get a slightly higher score offset, so they rank higher
// when the main score is identical. The offset is < 1e-10, which is negligible
// for display but sufficient for Redis sorted set ordering.
// A deterministic userID-based offset (< 1e-12) ensures that two users reaching
// the same score in the same nanosecond still get distinct tiebreaker values.
func encodeTiebreaker(score float64, userID string, ts time.Time) float64 {
	h := fnv.New32a()
	h.Write([]byte(userID))
	deterministicOffset := float64(h.Sum32()) / float64(math.MaxUint32) * 1e-12
	// Normalize timestamp: seconds since Unix epoch / 1e18 → always < 1e-8
	// Subtract from 1 so earlier times get higher offsets
	normalizedTs := float64(ts.UnixNano()) / 1e18
	return score + (1-normalizedTs)*1e-10 + deterministicOffset
}

// decodeTiebreaker removes the tiebreaker offset from an encoded score.
// The tiebreaker offset is < 1e-10, so rounding to 8 decimal places
// preserves the meaningful score while removing the tiebreaker noise.
// Using 8 decimal places avoids lossy rounding for close contests
// (e.g. scores like 123.456 won't be rounded to 123.46).
func decodeTiebreaker(encodedScore float64) float64 {
	return math.Round(encodedScore*1e8) / 1e8
}

// ActiveContestsKey returns the key for tracking active contests per shard.
func ActiveContestsKey(shardID int) string {
	return fmt.Sprintf("active_contests:{shard:%d}", shardID)
}

// GlobalLeaderboardCacheKey returns the cache key for global rankings.
func GlobalLeaderboardCacheKey() string {
	return "lb:global:cache"
}

// UpdateLeaderboard updates a user's score in the leaderboard for a specific contest and shard.
func (w *ShardedLeaderboardWorker) UpdateLeaderboard(
	ctx context.Context,
	contestID string,
	shardID int,
	entries []LeaderboardEntry,
) error {
	if len(entries) == 0 {
		return nil
	}

	// Use the main contest key with hash tags
	key := LeaderboardKey(contestID)

	// Build batch update using pipeline
	pipe := w.redisClient.Pipeline()

	// Add all entries to the sorted set
	zMembers := make([]redis.Z, len(entries))
	for i, entry := range entries {
		zMembers[i] = redis.Z{
			Score:  entry.Score,
			Member: entry.UserID,
		}
	}
	pipe.ZAdd(ctx, key, zMembers...)

	// Track this contest as active for the shard
	activeKey := ActiveContestsKey(shardID)
	pipe.SAdd(ctx, activeKey, contestID)

	// Set expiration on active contests set (cleanup after 24h)
	pipe.Expire(ctx, activeKey, 24*time.Hour)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update leaderboard: %w", err)
	}

	// Check if cache should be invalidated
	w.checkCacheInvalidation(contestID, entries)

	return nil
}

// UpdateScore updates a single user's score with delta tracking.
func (w *ShardedLeaderboardWorker) UpdateScore(
	ctx context.Context,
	contestID string,
	userID string,
	newScore float64,
	scoreDelta float64,
	significantThreshold float64,
) error {
	key := LeaderboardKey(contestID)

	// P1-3: Encode tiebreaker into score so earlier achievers rank higher.
	// Use a tiny time-based offset that won't affect the main score ordering.
	// normalizedTs is in [0,1] range, so the offset is < 1e-10.
	scoreWithTiebreaker := encodeTiebreaker(newScore, userID, time.Now())

	// Update the score
	pipe := w.redisClient.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  scoreWithTiebreaker,
		Member: userID,
	})
	// Set TTL so completed contest leaderboards auto-expire (24h after last write).
	// Active contests keep getting writes, so TTL keeps extending.
	pipe.Expire(ctx, key, 24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update score: %w", err)
	}

	// Invalidate cache if significant change
	if scoreDelta >= significantThreshold || scoreDelta <= -significantThreshold {
		w.invalidateCache(contestID)
	}

	return nil
}

// GetTop100 retrieves the top 100 entries from a contest's leaderboard.
func (w *ShardedLeaderboardWorker) GetTop100(ctx context.Context, contestID string) ([]LeaderboardEntry, error) {
	return w.GetTop(ctx, contestID, 100)
}

// GetTop retrieves the top N entries from a contest's leaderboard.
func (w *ShardedLeaderboardWorker) GetTop(ctx context.Context, contestID string, n int) ([]LeaderboardEntry, error) {
	// Check cache first
	if cached := w.getCachedLeaderboard(contestID); cached != nil {
		if n <= len(cached.Entries) {
			return cached.Entries[:n], nil
		}
		return cached.Entries, nil
	}

	key := LeaderboardKey(contestID)

	results, err := w.redisClient.ZRevRangeWithScores(ctx, key, 0, int64(n-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get leaderboard: %w", err)
	}

	entries := make([]LeaderboardEntry, 0, len(results))
	for i, z := range results {
		entries = append(entries, LeaderboardEntry{
			Rank:   i + 1,
			UserID: z.Member.(string),
			Score:  decodeTiebreaker(z.Score),
		})
	}

	// Cache the result
	w.cacheLeaderboard(contestID, entries)

	return entries, nil
}

// GetGlobalTop100 aggregates leaderboards from all active contests and returns global top 100.
func (w *ShardedLeaderboardWorker) GetGlobalTop100(ctx context.Context) ([]GlobalRankEntry, error) {
	return w.GetCrossShardRanking(ctx, 100)
}

// GetCrossShardRanking retrieves global ranking across all contests.
// This aggregates from all shards and merges results.
func (w *ShardedLeaderboardWorker) GetCrossShardRanking(ctx context.Context, limit int) ([]GlobalRankEntry, error) {
	// Check global cache
	if cached := w.getCachedGlobalRanking(); cached != nil && len(cached) >= limit {
		return cached[:limit], nil
	}

	// Collect all active contest IDs from all shards
	activeContests := make(map[string]struct{})

	for shardID := 0; shardID < w.shardCount; shardID++ {
		activeKey := ActiveContestsKey(shardID)
		contests, err := w.redisClient.SMembers(ctx, activeKey).Result()
		if err != nil {
			// Log but continue - some shards might be empty
			continue
		}
		for _, c := range contests {
			activeContests[c] = struct{}{}
		}
	}

	// Also scan for lb:* keys in case active_contests isn't up to date.
	// In cluster mode, SCAN only hits one node, so we use scanLeaderboardKeys
	// which runs SCAN on every master node via ForEachMaster.
	for _, key := range w.scanLeaderboardKeys(ctx) {
		// Extract contest ID from key format lb:{contestID}
		if len(key) > 4 && key[3] == '{' {
			end := len(key) - 1
			for i := 4; i < len(key); i++ {
				if key[i] == '}' {
					end = i
					break
				}
			}
			contestID := key[4:end]
			activeContests[contestID] = struct{}{}
		}
	}

	// Aggregate entries from all contests with bounded concurrency
	var allEntries []GlobalRankEntry
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fetch top entries from each contest concurrently (max 50 goroutines)
	entriesPerContest := limit
	if entriesPerContest < 100 {
		entriesPerContest = 100
	}

	sem := make(chan struct{}, 50) // Limit concurrent Redis queries

	for contestID := range activeContests {
		wg.Add(1)
		sem <- struct{}{} // Acquire semaphore
		go func(cid string) {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore
			defer func() {
				if r := recover(); r != nil {
					log.Printf("ERROR: GetGlobalRanking goroutine panicked for contest %s: %s\n%s", cid, observability.RedactPanic(r), observability.RedactText(string(debug.Stack())))
				}
			}()

			entries, err := w.GetTop(ctx, cid, entriesPerContest)
			if err != nil {
				return
			}

			mu.Lock()
			for _, e := range entries {
				allEntries = append(allEntries, GlobalRankEntry{
					UserID:      e.UserID,
					ContestID:   cid,
					Score:       e.Score,
					ContestRank: e.Rank,
				})
			}
			mu.Unlock()
		}(contestID)
	}

	wg.Wait()

	// Sort by score descending
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].Score > allEntries[j].Score
	})

	// Assign global ranks
	for i := range allEntries {
		allEntries[i].Rank = i + 1
	}

	// Trim to limit
	if len(allEntries) > limit {
		allEntries = allEntries[:limit]
	}

	// Cache the global ranking
	w.cacheGlobalRanking(allEntries)

	return allEntries, nil
}

// GetUserGlobalRank returns a user's best ranking across all active contests.
func (w *ShardedLeaderboardWorker) GetUserGlobalRank(ctx context.Context, userID string) (*GlobalRankEntry, error) {
	// Get all active contests
	activeContests := make(map[string]struct{})

	for shardID := 0; shardID < w.shardCount; shardID++ {
		activeKey := ActiveContestsKey(shardID)
		contests, err := w.redisClient.SMembers(ctx, activeKey).Result()
		if err != nil {
			continue
		}
		for _, c := range contests {
			activeContests[c] = struct{}{}
		}
	}

	var bestEntry *GlobalRankEntry
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Check each contest for the user
	for contestID := range activeContests {
		wg.Add(1)
		go func(cid string) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("ERROR: GetUserGlobalRank goroutine panicked for contest %s: %s\n%s", cid, observability.RedactPanic(r), observability.RedactText(string(debug.Stack())))
				}
			}()

			key := LeaderboardKey(cid)

			// Get user's score
			score, err := w.redisClient.ZScore(ctx, key, userID).Result()
			if err == redis.Nil {
				return // User not in this contest
			}
			if err != nil {
				return
			}

			// Get user's rank
			rank, err := w.redisClient.ZRevRank(ctx, key, userID).Result()
			if err != nil {
				return
			}

			cleanScore := decodeTiebreaker(score)
			mu.Lock()
			if bestEntry == nil || cleanScore > bestEntry.Score {
				bestEntry = &GlobalRankEntry{
					UserID:      userID,
					ContestID:   cid,
					Score:       cleanScore,
					ContestRank: int(rank) + 1,
				}
			}
			mu.Unlock()
		}(contestID)
	}

	wg.Wait()

	return bestEntry, nil
}

// GetContestLeaderboardWithSurrounding returns leaderboard entries around a user.
func (w *ShardedLeaderboardWorker) GetContestLeaderboardWithSurrounding(
	ctx context.Context,
	contestID string,
	userID string,
	before int,
	after int,
) ([]LeaderboardEntry, error) {
	key := LeaderboardKey(contestID)

	// Get user's rank first
	rank, err := w.redisClient.ZRevRank(ctx, key, userID).Result()
	if err == redis.Nil {
		return nil, nil // User not in leaderboard
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user rank: %w", err)
	}

	// Calculate range
	start := rank - int64(before)
	if start < 0 {
		start = 0
	}
	end := rank + int64(after)

	// Get entries in range
	results, err := w.redisClient.ZRevRangeWithScores(ctx, key, start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get surrounding entries: %w", err)
	}

	entries := make([]LeaderboardEntry, 0, len(results))
	for i, z := range results {
		entries = append(entries, LeaderboardEntry{
			Rank:   int(start) + i + 1,
			UserID: z.Member.(string),
			Score:  decodeTiebreaker(z.Score),
		})
	}

	return entries, nil
}

// BatchUpdateScores efficiently updates multiple scores in a single operation.
// Returns a list of failed user IDs (if any) alongside the error for partial failure visibility.
// For small batches (< 10 users), skips the pre-fetch of existing scores since the cost of
// a wasted ZADD is lower than the extra Redis round-trip. For larger batches, pre-fetches
// existing scores to skip unchanged values and prevent tiebreaker drift.
func (w *ShardedLeaderboardWorker) BatchUpdateScores(
	ctx context.Context,
	contestID string,
	updates map[string]float64,
) ([]string, error) {
	if len(updates) == 0 {
		return nil, nil
	}

	key := LeaderboardKey(contestID)
	now := time.Now()

	// For large batches, pre-fetch existing scores to skip unchanged values
	// and prevent tiebreaker drift. For small batches, the cost of the extra
	// Redis round-trip outweighs the savings from skipping unchanged scores.
	const prefetchThreshold = 10

	zMembers := make([]redis.Z, 0, len(updates))
	userIDs := make([]string, 0, len(updates))

	if len(updates) >= prefetchThreshold {
		// Fetch existing scores to detect unchanged values and avoid tiebreaker drift.
		existingScores := make(map[string]float64, len(updates))
		existingMembers := make(map[string]bool, len(updates))
		{
			pipe := w.redisClient.Pipeline()
			scoreCmds := make(map[string]*redis.FloatCmd, len(updates))
			for userID := range updates {
				scoreCmds[userID] = pipe.ZScore(ctx, key, userID)
			}
			if _, err := pipe.Exec(ctx); err != nil {
				// Log the error - individual command errors are checked below
				log.Printf("leaderboard: pipeline exec error fetching existing scores for contest %s: %v", contestID, err)
			}

			for userID, cmd := range scoreCmds {
				score, err := cmd.Result()
				if err == nil {
					existingMembers[userID] = true
					existingScores[userID] = decodeTiebreaker(score)
				}
			}
		}

		for userID, score := range updates {
			if existingMembers[userID] && math.Abs(existingScores[userID]-math.Round(score*100)/100) < 0.005 {
				continue
			}
			zMembers = append(zMembers, redis.Z{
				Score:  encodeTiebreaker(score, userID, now),
				Member: userID,
			})
			userIDs = append(userIDs, userID)
		}
	} else {
		// Small batch: skip pre-fetch, just encode and write all
		for userID, score := range updates {
			zMembers = append(zMembers, redis.Z{
				Score:  encodeTiebreaker(score, userID, now),
				Member: userID,
			})
			userIDs = append(userIDs, userID)
		}
	}

	if len(zMembers) == 0 {
		return nil, nil
	}

	// Execute ZADD
	if err := w.redisClient.ZAdd(ctx, key, zMembers...).Err(); err != nil {
		return userIDs, fmt.Errorf("failed to batch update scores: %w", err)
	}

	// Invalidate cache for this contest
	w.invalidateCache(contestID)

	return nil, nil
}

// GetLeaderboardSize returns the number of participants in a contest.
func (w *ShardedLeaderboardWorker) GetLeaderboardSize(ctx context.Context, contestID string) (int64, error) {
	key := LeaderboardKey(contestID)
	return w.redisClient.ZCard(ctx, key).Result()
}

// GetUserRank returns a user's rank in a specific contest.
func (w *ShardedLeaderboardWorker) GetUserRank(ctx context.Context, contestID, userID string) (*UserRank, error) {
	key := LeaderboardKey(contestID)

	// Get score and rank in pipeline
	pipe := w.redisClient.Pipeline()
	scoreCmd := pipe.ZScore(ctx, key, userID)
	rankCmd := pipe.ZRevRank(ctx, key, userID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user rank: %w", err)
	}

	score, err := scoreCmd.Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rank, err := rankCmd.Result()
	if err != nil {
		return nil, err
	}

	return &UserRank{
		UserID: userID,
		Rank:   int(rank) + 1,
		Score:  decodeTiebreaker(score),
	}, nil
}

// Cache management methods

func (w *ShardedLeaderboardWorker) getCachedLeaderboard(contestID string) *CachedLeaderboard {
	w.cacheMu.RLock()
	defer w.cacheMu.RUnlock()

	cached, ok := w.globalCache[contestID]
	if !ok {
		return nil
	}

	// Check if cache is still valid
	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	// Check if cache was invalidated
	if w.cacheInvalid[contestID] {
		return nil
	}

	return cached
}

func (w *ShardedLeaderboardWorker) cacheLeaderboard(contestID string, entries []LeaderboardEntry) {
	w.cacheMu.Lock()
	defer w.cacheMu.Unlock()

	now := time.Now()
	w.globalCache[contestID] = &CachedLeaderboard{
		Entries:   entries,
		CachedAt:  now,
		ExpiresAt: now.Add(w.cacheTTL),
	}
	delete(w.cacheInvalid, contestID)
}

func (w *ShardedLeaderboardWorker) invalidateCache(contestID string) {
	w.cacheMu.Lock()
	defer w.cacheMu.Unlock()

	w.cacheInvalid[contestID] = true
	delete(w.globalCache, contestID)
}

func (w *ShardedLeaderboardWorker) checkCacheInvalidation(contestID string, entries []LeaderboardEntry) {
	// Simple heuristic: invalidate if we received a batch update
	// More sophisticated logic could compare with cached values
	if len(entries) > 10 {
		w.invalidateCache(contestID)
	}
}

func (w *ShardedLeaderboardWorker) getCachedGlobalRanking() []GlobalRankEntry {
	w.globalRankingMu.RLock()
	defer w.globalRankingMu.RUnlock()

	if time.Since(w.globalRankingCacheTime) > w.cacheTTL {
		return nil
	}

	return w.globalRankingCache
}

func (w *ShardedLeaderboardWorker) cacheGlobalRanking(entries []GlobalRankEntry) {
	w.globalRankingMu.Lock()
	defer w.globalRankingMu.Unlock()

	w.globalRankingCache = entries
	w.globalRankingCacheTime = time.Now()
}

// InvalidateGlobalCache forces invalidation of the global ranking cache.
func (w *ShardedLeaderboardWorker) InvalidateGlobalCache() {
	w.globalRankingMu.Lock()
	defer w.globalRankingMu.Unlock()

	w.globalRankingCache = nil
	w.globalRankingCacheTime = time.Time{}
}

// scanLeaderboardKeys discovers lb:{*} keys across all Redis nodes.
// In cluster mode, a single SCAN only hits one node, so we use ForEachMaster
// to scan every master node individually.
func (w *ShardedLeaderboardWorker) scanLeaderboardKeys(ctx context.Context) []string {
	var keys []string

	// Check if running in cluster mode
	if clusterClient, ok := w.redisClient.Client().(*redis.ClusterClient); ok {
		var mu sync.Mutex
		_ = clusterClient.ForEachMaster(ctx, func(ctx context.Context, client *redis.Client) error {
			iter := client.Scan(ctx, 0, "lb:{*}", 1000).Iterator()
			for iter.Next(ctx) {
				mu.Lock()
				keys = append(keys, iter.Val())
				mu.Unlock()
			}
			return iter.Err()
		})
	} else {
		// Non-cluster mode: single SCAN works fine
		iter := w.redisClient.Scan(ctx, 0, "lb:{*}", 1000).Iterator()
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}
	}

	return keys
}

// CleanupExpiredCache removes expired entries from the cache.
// It limits deletions to 100 per cycle to avoid holding the lock for too long.
func (w *ShardedLeaderboardWorker) CleanupExpiredCache() {
	w.cacheMu.Lock()
	defer w.cacheMu.Unlock()

	now := time.Now()
	cleaned := 0
	maxPerCycle := 100
	for contestID, cached := range w.globalCache {
		if cleaned >= maxPerCycle {
			break
		}
		if now.After(cached.ExpiresAt) {
			delete(w.globalCache, contestID)
			delete(w.cacheInvalid, contestID)
			cleaned++
		}
	}

	// Also clean up expired global ranking cache to free memory
	w.globalRankingMu.Lock()
	if !w.globalRankingCacheTime.IsZero() && time.Since(w.globalRankingCacheTime) > w.cacheTTL {
		w.globalRankingCache = nil
		w.globalRankingCacheTime = time.Time{}
	}
	w.globalRankingMu.Unlock()
}

// StartCacheCleanup runs periodic cache cleanup.
func (w *ShardedLeaderboardWorker) StartCacheCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.CleanupExpiredCache()
		}
	}
}

// SerializeGlobalRanking serializes global ranking to JSON for API responses.
func SerializeGlobalRanking(entries []GlobalRankEntry) ([]byte, error) {
	return json.Marshal(entries)
}

// MergeLeaderboards merges multiple leaderboard entries maintaining score order.
func MergeLeaderboards(boards ...[]LeaderboardEntry) []LeaderboardEntry {
	// Calculate total capacity
	total := 0
	for _, b := range boards {
		total += len(b)
	}

	merged := make([]LeaderboardEntry, 0, total)
	for _, b := range boards {
		merged = append(merged, b...)
	}

	// Sort by score descending
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})

	// Re-assign ranks
	for i := range merged {
		merged[i].Rank = i + 1
	}

	return merged
}

// Helper to parse score from string (for Redis responses).
func parseScore(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
