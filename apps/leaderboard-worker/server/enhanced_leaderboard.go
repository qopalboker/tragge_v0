package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/redis/go-redis/v9"
)

// EnhancedLeaderboardEntry represents a leaderboard entry with full score breakdown.
// This matches the Tralent-like leaderboard requirements.
type EnhancedLeaderboardEntry struct {
	Rank            int     `json:"rank"`
	UserID          string  `json:"user_id"`
	Username        string  `json:"username"`
	TotalScore      float64 `json:"total_score"`
	RealizedScore   float64 `json:"realized_score"`
	UnrealizedScore float64 `json:"unrealized_score"`
}

// EnhancedLeaderboardResponse is the full API response for leaderboard queries.
type EnhancedLeaderboardResponse struct {
	ContestID         string                     `json:"contest_id"`
	Entries           []EnhancedLeaderboardEntry `json:"entries"`
	TotalParticipants int64                      `json:"total_participants"`
	UserRank          *int                       `json:"user_rank,omitempty"`
	UserScore         *float64                   `json:"user_score,omitempty"`
	UpdatedAt         time.Time                  `json:"updated_at"`
}

// ScoreBreakdown holds the detailed score components for a user.
type ScoreBreakdown struct {
	TotalScore      float64 `json:"total_score"`
	RealizedScore   float64 `json:"realized_score"`
	UnrealizedScore float64 `json:"unrealized_score"`
	UpdatedAt       int64   `json:"updated_at"`
}

// usernameLRUCache wraps an LRU cache for usernames. Thread-safe via the
// underlying hashicorp/golang-lru implementation (O(1) eviction).
type usernameLRUCache struct {
	cache *lru.Cache[string, string]
}

func newUsernameLRUCache(maxSize int) *usernameLRUCache {
	c, _ := lru.New[string, string](maxSize)
	return &usernameLRUCache{cache: c}
}

// Get returns the cached username if present.
func (c *usernameLRUCache) Get(userID string) (string, bool) {
	return c.cache.Get(userID)
}

// Set adds or updates a username in the cache.
func (c *usernameLRUCache) Set(userID, name string) {
	c.cache.Add(userID, name)
}

// EnhancedLeaderboardManager manages leaderboard data with score breakdowns.
type EnhancedLeaderboardManager struct {
	redis redis.UniversalClient
	db    *sql.DB

	// Username cache (LRU, O(1) eviction, thread-safe)
	usernameCache *usernameLRUCache

	// Score breakdown cache
	scoreBreakdownCacheMu  sync.RWMutex
	scoreBreakdownCache    map[string]map[string]*ScoreBreakdown // contestID -> userID -> breakdown
	scoreBreakdownTTL      time.Duration                        // TTL for breakdown entries
	maxCachedContestScores int                                   // Max number of contests to cache
}

// NewEnhancedLeaderboardManager creates a new enhanced leaderboard manager.
func NewEnhancedLeaderboardManager(redis redis.UniversalClient, db *sql.DB) *EnhancedLeaderboardManager {
	return &EnhancedLeaderboardManager{
		redis:                  redis,
		db:                     db,
		usernameCache:          newUsernameLRUCache(10000),
		scoreBreakdownCache:    make(map[string]map[string]*ScoreBreakdown),
		scoreBreakdownTTL:      10 * time.Minute,
		maxCachedContestScores: 500,
	}
}

// ScoreBreakdownKey returns the Redis hash key for storing score breakdowns.
func ScoreBreakdownKey(contestID string) string {
	return fmt.Sprintf("lb_breakdown:{%s}", contestID)
}

// UpdateScoreWithBreakdown updates a user's score breakdown in the hash.
// Note: sorted set (lb:{contestID}) is written by ShardedLeaderboardWorker.UpdateScore()
// with proper tiebreaker encoding. We only maintain the breakdown hash here.
func (m *EnhancedLeaderboardManager) UpdateScoreWithBreakdown(
	ctx context.Context,
	contestID string,
	userID string,
	totalScore float64,
	realizedScore float64,
	unrealizedScore float64,
) error {
	// Store score breakdown in hash
	breakdownKey := ScoreBreakdownKey(contestID)
	breakdown := ScoreBreakdown{
		TotalScore:      totalScore,
		RealizedScore:   realizedScore,
		UnrealizedScore: unrealizedScore,
		UpdatedAt:       time.Now().UnixMilli(),
	}
	breakdownJSON, err := json.Marshal(breakdown)
	if err != nil {
		return fmt.Errorf("failed to marshal breakdown: %w", err)
	}

	if err := m.redis.HSet(ctx, breakdownKey, userID, breakdownJSON).Err(); err != nil {
		return fmt.Errorf("failed to update breakdown hash: %w", err)
	}

	// Update in-memory cache
	m.scoreBreakdownCacheMu.Lock()
	if m.scoreBreakdownCache[contestID] == nil {
		m.scoreBreakdownCache[contestID] = make(map[string]*ScoreBreakdown)
	}
	m.scoreBreakdownCache[contestID][userID] = &breakdown
	m.scoreBreakdownCacheMu.Unlock()

	return nil
}

// BatchUpdateScoresWithBreakdown updates multiple users' score breakdowns using a Redis pipeline.
// Only maintains the breakdown hash — sorted set writes happen via ShardedLeaderboardWorker.
// The pipe parameter is optional: when non-nil, commands are added to the existing pipeline
// (allowing the caller to batch with other operations). When nil, creates and executes its own pipeline.
func (m *EnhancedLeaderboardManager) BatchUpdateScoresWithBreakdown(
	ctx context.Context,
	contestID string,
	updates []ScoreUpdate,
	pipe ...redis.Pipeliner,
) error {
	if len(updates) == 0 {
		return nil
	}

	breakdownKey := ScoreBreakdownKey(contestID)
	nowMs := time.Now().UnixMilli()

	// Use provided pipeline or create a new one
	var p redis.Pipeliner
	ownPipeline := len(pipe) == 0 || pipe[0] == nil
	if ownPipeline {
		p = m.redis.Pipeline()
	} else {
		p = pipe[0]
	}

	for _, u := range updates {
		breakdown := ScoreBreakdown{
			TotalScore:      u.TotalScore,
			RealizedScore:   u.RealizedScore,
			UnrealizedScore: u.UnrealizedScore,
			UpdatedAt:       nowMs,
		}
		breakdownJSON, err := json.Marshal(breakdown)
		if err != nil {
			continue
		}
		p.HSet(ctx, breakdownKey, u.UserID, breakdownJSON)
	}

	// Only execute if we own the pipeline
	if ownPipeline {
		if _, err := p.Exec(ctx); err != nil {
			return fmt.Errorf("failed to batch update scores with breakdown: %w", err)
		}
	}

	// Update in-memory cache
	m.scoreBreakdownCacheMu.Lock()
	if m.scoreBreakdownCache[contestID] == nil {
		m.scoreBreakdownCache[contestID] = make(map[string]*ScoreBreakdown)
	}
	for _, u := range updates {
		m.scoreBreakdownCache[contestID][u.UserID] = &ScoreBreakdown{
			TotalScore:      u.TotalScore,
			RealizedScore:   u.RealizedScore,
			UnrealizedScore: u.UnrealizedScore,
			UpdatedAt:       nowMs,
		}
	}
	m.scoreBreakdownCacheMu.Unlock()

	return nil
}

// ScoreUpdate holds the data for a single score update in a batch.
type ScoreUpdate struct {
	UserID          string
	TotalScore      float64
	RealizedScore   float64
	UnrealizedScore float64
}

// GetEnhancedLeaderboard retrieves the leaderboard with full score breakdowns and usernames.
// Uses Redis pipelines to minimize round-trips: one pipeline for ZCard + ZRevRangeWithScores
// (+ optional user rank), then a second for HMGet breakdowns (needs userIDs from first).
func (m *EnhancedLeaderboardManager) GetEnhancedLeaderboard(
	ctx context.Context,
	contestID string,
	limit int,
	offset int,
	currentUserID string,
) (*EnhancedLeaderboardResponse, error) {
	lbKey := LeaderboardKey(contestID)
	breakdownKey := ScoreBreakdownKey(contestID)

	// Pipeline 1: ZCard + ZRevRangeWithScores (+ optional user rank/score)
	pipe := m.redis.Pipeline()
	cardCmd := pipe.ZCard(ctx, lbKey)
	rangeCmd := pipe.ZRevRangeWithScores(ctx, lbKey, int64(offset), int64(offset+limit-1))

	var userScoreCmd *redis.FloatCmd
	var userRankCmd *redis.IntCmd
	if currentUserID != "" {
		userScoreCmd = pipe.ZScore(ctx, lbKey, currentUserID)
		userRankCmd = pipe.ZRevRank(ctx, lbKey, currentUserID)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get leaderboard data: %w", err)
	}

	totalParticipants, _ := cardCmd.Result()
	results, err := rangeCmd.Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get leaderboard: %w", err)
	}

	// Collect user IDs for batch lookup
	userIDs := make([]string, len(results))
	for i, z := range results {
		userIDs[i] = z.Member.(string)
	}

	// Get usernames in batch
	usernames, err := m.getDisplayNames(ctx, userIDs)
	if err != nil {
		usernames = make(map[string]string)
	}

	// Get score breakdowns: check in-memory cache first, then Redis for missing
	breakdowns := m.getBreakdownsCached(ctx, contestID, breakdownKey, userIDs)

	// Build response entries
	entries := make([]EnhancedLeaderboardEntry, len(results))
	for i, z := range results {
		userID := z.Member.(string)
		entry := EnhancedLeaderboardEntry{
			Rank:       offset + i + 1,
			UserID:     userID,
			Username:   usernames[userID],
			TotalScore: decodeTiebreaker(z.Score),
		}

		if bd, ok := breakdowns[userID]; ok {
			entry.RealizedScore = bd.RealizedScore
			entry.UnrealizedScore = bd.UnrealizedScore
		}

		entries[i] = entry
	}

	response := &EnhancedLeaderboardResponse{
		ContestID:         contestID,
		Entries:           entries,
		TotalParticipants: totalParticipants,
		UpdatedAt:         time.Now().UTC(),
	}

	// Extract user rank from pipeline results
	if currentUserID != "" && userScoreCmd != nil && userRankCmd != nil {
		score, scoreErr := userScoreCmd.Result()
		rank, rankErr := userRankCmd.Result()
		if scoreErr == nil && rankErr == nil {
			r := int(rank) + 1
			s := decodeTiebreaker(score)
			response.UserRank = &r
			response.UserScore = &s
		}
	}

	return response, nil
}

// GetUserRankAndScore returns a user's rank and score in a contest.
func (m *EnhancedLeaderboardManager) GetUserRankAndScore(ctx context.Context, contestID, userID string) (int, float64, error) {
	lbKey := LeaderboardKey(contestID)

	// Get score and rank in pipeline
	pipe := m.redis.Pipeline()
	scoreCmd := pipe.ZScore(ctx, lbKey, userID)
	rankCmd := pipe.ZRevRank(ctx, lbKey, userID)

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return 0, 0, fmt.Errorf("failed to get user rank: %w", err)
	}

	score, err := scoreCmd.Result()
	if err == redis.Nil {
		return 0, 0, nil // User not in leaderboard
	}
	if err != nil {
		return 0, 0, err
	}

	rank, err := rankCmd.Result()
	if err != nil {
		return 0, score, nil
	}

	return int(rank) + 1, decodeTiebreaker(score), nil
}

// getDisplayNames retrieves usernames for a list of user IDs.
// Uses database query with caching.
func (m *EnhancedLeaderboardManager) getDisplayNames(ctx context.Context, userIDs []string) (map[string]string, error) {
	if len(userIDs) == 0 {
		return make(map[string]string), nil
	}

	result := make(map[string]string)
	var missingIDs []string

	// Check cache first
	for _, id := range userIDs {
		if name, ok := m.usernameCache.Get(id); ok {
			result[id] = name
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// Query database for missing usernames
	if len(missingIDs) > 0 {
		dbNames, err := m.queryDisplayNames(ctx, missingIDs)
		if err != nil {
			return result, err
		}

		// Update cache and result
		for id, name := range dbNames {
			m.usernameCache.Set(id, name)
			result[id] = name
		}
	}

	return result, nil
}

// queryDisplayNames queries usernames from the database.
// Extracts username from email (part before @).
func (m *EnhancedLeaderboardManager) queryDisplayNames(ctx context.Context, userIDs []string) (map[string]string, error) {
	if m.db == nil {
		return make(map[string]string), nil
	}

	// Build query with placeholders - extract username from email
	query := `
		SELECT id::text, SPLIT_PART(email, '@', 1) as username
		FROM users
		WHERE id::text = ANY($1)
	`

	rows, err := m.db.QueryContext(ctx, query, userIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query usernames: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		result[id] = name
	}

	return result, rows.Err()
}

// GetUserLeaderboardPosition returns a user's position with surrounding entries.
// Uses a two-phase pipeline approach: first gets rank, then fetches surrounding data.
func (m *EnhancedLeaderboardManager) GetUserLeaderboardPosition(
	ctx context.Context,
	contestID string,
	userID string,
	beforeCount int,
	afterCount int,
) (*EnhancedLeaderboardResponse, error) {
	lbKey := LeaderboardKey(contestID)
	breakdownKey := ScoreBreakdownKey(contestID)

	// Pipeline 1: Get user's rank + score + total participants
	pipe1 := m.redis.Pipeline()
	rankCmd := pipe1.ZRevRank(ctx, lbKey, userID)
	scoreCmd := pipe1.ZScore(ctx, lbKey, userID)
	cardCmd := pipe1.ZCard(ctx, lbKey)
	_, err := pipe1.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get user rank: %w", err)
	}

	rank, err := rankCmd.Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("user not found in leaderboard")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user rank: %w", err)
	}

	rawUserScore, _ := scoreCmd.Result()
	totalParticipants, _ := cardCmd.Result()

	// Calculate range
	start := rank - int64(beforeCount)
	if start < 0 {
		start = 0
	}
	end := rank + int64(afterCount)

	// Get entries in range
	results, err := m.redis.ZRevRangeWithScores(ctx, lbKey, start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get surrounding entries: %w", err)
	}

	// Collect user IDs
	userIDs := make([]string, len(results))
	for i, z := range results {
		userIDs[i] = z.Member.(string)
	}

	// Get usernames
	usernames, _ := m.getDisplayNames(ctx, userIDs)

	// Get score breakdowns: check in-memory cache first, then Redis for missing
	breakdowns := m.getBreakdownsCached(ctx, contestID, breakdownKey, userIDs)

	// Build entries
	entries := make([]EnhancedLeaderboardEntry, len(results))
	for i, z := range results {
		uid := z.Member.(string)
		entry := EnhancedLeaderboardEntry{
			Rank:       int(start) + i + 1,
			UserID:     uid,
			Username:   usernames[uid],
			TotalScore: decodeTiebreaker(z.Score),
		}
		if bd, ok := breakdowns[uid]; ok {
			entry.RealizedScore = bd.RealizedScore
			entry.UnrealizedScore = bd.UnrealizedScore
		}
		entries[i] = entry
	}

	cleanUserScore := decodeTiebreaker(rawUserScore)
	userRank := int(rank) + 1

	return &EnhancedLeaderboardResponse{
		ContestID:         contestID,
		Entries:           entries,
		TotalParticipants: totalParticipants,
		UserRank:          &userRank,
		UserScore:         &cleanUserScore,
		UpdatedAt:         time.Now().UTC(),
	}, nil
}

// getBreakdownsCached retrieves score breakdowns using in-memory cache first,
// falling back to Redis HMGET for cache misses. This avoids a Redis round-trip
// when all breakdowns are already cached in memory (common for active contests).
func (m *EnhancedLeaderboardManager) getBreakdownsCached(
	ctx context.Context,
	contestID string,
	breakdownKey string,
	userIDs []string,
) map[string]*ScoreBreakdown {
	breakdowns := make(map[string]*ScoreBreakdown, len(userIDs))
	if len(userIDs) == 0 {
		return breakdowns
	}

	// Check in-memory cache first
	var missingIDs []string
	m.scoreBreakdownCacheMu.RLock()
	contestCache := m.scoreBreakdownCache[contestID]
	if contestCache != nil {
		nowMs := time.Now().UnixMilli()
		ttlMs := m.scoreBreakdownTTL.Milliseconds()
		for _, uid := range userIDs {
			if bd, ok := contestCache[uid]; ok && nowMs-bd.UpdatedAt < ttlMs {
				breakdowns[uid] = bd
			} else {
				missingIDs = append(missingIDs, uid)
			}
		}
	} else {
		missingIDs = userIDs
	}
	m.scoreBreakdownCacheMu.RUnlock()

	// Fetch missing breakdowns from Redis
	if len(missingIDs) > 0 {
		breakdownsRaw, err := m.redis.HMGet(ctx, breakdownKey, missingIDs...).Result()
		if err == nil {
			for i, raw := range breakdownsRaw {
				if raw != nil {
					var bd ScoreBreakdown
					if jsonStr, ok := raw.(string); ok {
						if json.Unmarshal([]byte(jsonStr), &bd) == nil {
							breakdowns[missingIDs[i]] = &bd
						}
					}
				}
			}
		}
	}

	return breakdowns
}

// CleanupUsernameCache evicts stale entries from the score breakdown cache.
// The username LRU cache handles its own eviction automatically.
func (m *EnhancedLeaderboardManager) CleanupUsernameCache() {
	m.cleanupScoreBreakdownCache()
}

// cleanupScoreBreakdownCache removes contest entries from the score breakdown
// cache where all user breakdowns have expired (UpdatedAt older than TTL).
// Also enforces a max contest count to prevent unbounded memory growth.
func (m *EnhancedLeaderboardManager) cleanupScoreBreakdownCache() {
	m.scoreBreakdownCacheMu.Lock()
	defer m.scoreBreakdownCacheMu.Unlock()

	nowMs := time.Now().UnixMilli()
	ttlMs := m.scoreBreakdownTTL.Milliseconds()

	// Phase 1: TTL-based eviction — remove contests where ALL breakdowns are expired
	for contestID, users := range m.scoreBreakdownCache {
		allExpired := true
		for _, bd := range users {
			if nowMs-bd.UpdatedAt < ttlMs {
				allExpired = false
				break
			}
		}
		if allExpired {
			delete(m.scoreBreakdownCache, contestID)
		}
	}

	// Phase 2: Size-based eviction — if still over max, evict oldest contests
	if len(m.scoreBreakdownCache) > m.maxCachedContestScores {
		type contestAge struct {
			id       string
			newestAt int64
		}
		ages := make([]contestAge, 0, len(m.scoreBreakdownCache))
		for cid, users := range m.scoreBreakdownCache {
			var newest int64
			for _, bd := range users {
				if bd.UpdatedAt > newest {
					newest = bd.UpdatedAt
				}
			}
			ages = append(ages, contestAge{id: cid, newestAt: newest})
		}
		sort.Slice(ages, func(i, j int) bool {
			return ages[i].newestAt < ages[j].newestAt
		})
		evictCount := len(ages) - m.maxCachedContestScores
		for i := 0; i < evictCount; i++ {
			delete(m.scoreBreakdownCache, ages[i].id)
		}
	}
}
