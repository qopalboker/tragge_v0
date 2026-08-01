package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/infra"
	"github.com/go-chi/chi/v5"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// LeaderboardEntry represents a single entry in the leaderboard.
type LeaderboardEntry struct {
	Rank            int     `json:"rank"`
	UserID          string  `json:"user_id"`
	Username        string  `json:"username"`
	TotalScore      float64 `json:"total_score"`
	RealizedScore   float64 `json:"realized_score"`
	UnrealizedScore float64 `json:"unrealized_score"`
}

// LeaderboardResponse is the API response for leaderboard queries.
type LeaderboardResponse struct {
	ContestID         string             `json:"contest_id"`
	Entries           []LeaderboardEntry `json:"entries"`
	TotalParticipants int64              `json:"total_participants"`
	UserRank          *int               `json:"user_rank,omitempty"`
	UserScore         *float64           `json:"user_score,omitempty"`
	UpdatedAt         time.Time          `json:"updated_at"`
}

// ScoreBreakdown holds the detailed score components for a user.
type ScoreBreakdown struct {
	TotalScore      float64 `json:"total_score"`
	RealizedScore   float64 `json:"realized_score"`
	UnrealizedScore float64 `json:"unrealized_score"`
	UpdatedAt       int64   `json:"updated_at"`
}

// top10CacheEntry holds a cached top 10 leaderboard result for a contest.
type top10CacheEntry struct {
	entries           []LeaderboardEntry
	totalParticipants int64
	expiresAt         time.Time
}

// LeaderboardManager provides leaderboard functionality for trade-bff.
type LeaderboardManager struct {
	redis redis.UniversalClient
	pool  *sql.DB

	// Username cache (LRU, max 10,000 entries, thread-safe)
	usernameCache *lru.Cache[string, string]

	// In-memory cache for top 10 leaderboard per contest (5s TTL)
	top10CacheMu sync.RWMutex
	top10Cache   map[string]*top10CacheEntry
}

// NewLeaderboardManager creates a new leaderboard manager.
func NewLeaderboardManager(redis redis.UniversalClient, pool *sql.DB) *LeaderboardManager {
	usernameCache, _ := lru.New[string, string](10000)
	return &LeaderboardManager{
		redis:         redis,
		pool:          pool,
		usernameCache: usernameCache,
		top10Cache:    make(map[string]*top10CacheEntry),
	}
}

// StartCleanup runs a background goroutine that periodically removes expired
// entries from the top10Cache to prevent unbounded memory growth.
func (m *LeaderboardManager) StartCleanup(ctx context.Context, logger *zap.Logger) {
	infra.SafeGo(logger, "leaderboard-cache-cleanup", func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.top10CacheMu.Lock()
				now := time.Now()
				for key, entry := range m.top10Cache {
					if now.After(entry.expiresAt) {
						delete(m.top10Cache, key)
					}
				}
				m.top10CacheMu.Unlock()
			}
		}
	})
}

// leaderboardKey returns the Redis key for a contest's leaderboard.
func leaderboardKey(contestID string) string {
	return fmt.Sprintf("lb:{%s}", contestID)
}

// scoreBreakdownKey returns the Redis hash key for storing score breakdowns.
func scoreBreakdownKey(contestID string) string {
	return fmt.Sprintf("lb_breakdown:{%s}", contestID)
}

// getBreakdowns fetches score breakdowns from Redis for the given user IDs.
func (m *LeaderboardManager) getBreakdowns(ctx context.Context, breakdownKey string, userIDs []string) map[string]*ScoreBreakdown {
	breakdowns := make(map[string]*ScoreBreakdown)
	if len(userIDs) > 0 {
		breakdownsRaw, err := m.redis.HMGet(ctx, breakdownKey, userIDs...).Result()
		if err != nil {
			zap.L().Warn("Failed to fetch score breakdowns from Redis",
				zap.String("breakdown_key", breakdownKey),
				zap.Error(err))
		} else {
			for i, raw := range breakdownsRaw {
				if raw != nil {
					var bd ScoreBreakdown
					if jsonStr, ok := raw.(string); ok {
						if json.Unmarshal([]byte(jsonStr), &bd) == nil {
							breakdowns[userIDs[i]] = &bd
						}
					}
				}
			}
		}
	}
	return breakdowns
}

// GetLeaderboard retrieves the leaderboard with score breakdowns and usernames.
// Uses Redis pipelines to minimize round-trips.
func (m *LeaderboardManager) GetLeaderboard(
	ctx context.Context,
	contestID string,
	limit int,
	offset int,
	currentUserID string,
) (*LeaderboardResponse, error) {
	lbKey := leaderboardKey(contestID)
	breakdownKey := scoreBreakdownKey(contestID)

	// Pipeline 1: ZCard + ZRevRangeWithScores + optional user rank/score
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
	if err != nil && !errors.Is(err, redis.Nil) {
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
	usernames := m.getDisplayNames(ctx, userIDs)

	// Get score breakdowns from hash
	breakdowns := m.getBreakdowns(ctx, breakdownKey, userIDs)

	// Build response entries
	entries := make([]LeaderboardEntry, len(results))
	for i, z := range results {
		userID := z.Member.(string)
		entry := LeaderboardEntry{
			Rank:       offset + i + 1,
			UserID:     userID,
			Username:   usernames[userID],
			TotalScore: z.Score,
		}

		if bd, ok := breakdowns[userID]; ok {
			entry.RealizedScore = bd.RealizedScore
			entry.UnrealizedScore = bd.UnrealizedScore
		}

		entries[i] = entry
	}

	response := &LeaderboardResponse{
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
			response.UserRank = &r
			response.UserScore = &score
		}
	}

	return response, nil
}

// Top10Result holds the top 10 leaderboard entries and participant count for a contest.
type Top10Result struct {
	Entries           []LeaderboardEntry `json:"top_entries"`
	TotalParticipants int64              `json:"total_participants"`
}

// GetTop10Cached returns the top 10 leaderboard entries for a contest,
// using an in-memory cache with a 5-second TTL to avoid querying Redis
// on every PnL delta.
func (m *LeaderboardManager) GetTop10Cached(ctx context.Context, contestID string) (*Top10Result, error) {
	const cacheTTL = 5 * time.Second

	// Check cache first
	m.top10CacheMu.RLock()
	if entry, ok := m.top10Cache[contestID]; ok && time.Now().Before(entry.expiresAt) {
		m.top10CacheMu.RUnlock()
		return &Top10Result{
			Entries:           entry.entries,
			TotalParticipants: entry.totalParticipants,
		}, nil
	}
	m.top10CacheMu.RUnlock()

	// Cache miss or expired — query Redis with pipeline
	lbKey := leaderboardKey(contestID)
	breakdownKey := scoreBreakdownKey(contestID)

	// Pipeline: ZCard + ZRevRangeWithScores in single round-trip
	pipe := m.redis.Pipeline()
	cardCmd := pipe.ZCard(ctx, lbKey)
	rangeCmd := pipe.ZRevRangeWithScores(ctx, lbKey, 0, 9)
	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to get leaderboard data: %w", err)
	}

	totalParticipants, _ := cardCmd.Result()
	results, err := rangeCmd.Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get top 10 leaderboard: %w", err)
	}

	// Collect user IDs for batch lookup
	userIDs := make([]string, len(results))
	for i, z := range results {
		userIDs[i] = z.Member.(string)
	}

	// Get usernames in batch
	usernames := m.getDisplayNames(ctx, userIDs)

	// Get score breakdowns from hash
	breakdowns := m.getBreakdowns(ctx, breakdownKey, userIDs)

	// Build entries
	entries := make([]LeaderboardEntry, len(results))
	for i, z := range results {
		userID := z.Member.(string)
		entry := LeaderboardEntry{
			Rank:       i + 1,
			UserID:     userID,
			Username:   usernames[userID],
			TotalScore: z.Score,
		}
		if bd, ok := breakdowns[userID]; ok {
			entry.RealizedScore = bd.RealizedScore
			entry.UnrealizedScore = bd.UnrealizedScore
		}
		entries[i] = entry
	}

	// Update cache
	m.top10CacheMu.Lock()
	m.top10Cache[contestID] = &top10CacheEntry{
		entries:           entries,
		totalParticipants: totalParticipants,
		expiresAt:         time.Now().Add(cacheTTL),
	}
	m.top10CacheMu.Unlock()

	return &Top10Result{
		Entries:           entries,
		TotalParticipants: totalParticipants,
	}, nil
}

// GetUserRankAndScore returns a user's rank and score in a contest.
func (m *LeaderboardManager) GetUserRankAndScore(ctx context.Context, contestID, userID string) (int, float64, error) {
	lbKey := leaderboardKey(contestID)

	// Get score and rank in pipeline
	pipe := m.redis.Pipeline()
	scoreCmd := pipe.ZScore(ctx, lbKey, userID)
	rankCmd := pipe.ZRevRank(ctx, lbKey, userID)

	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
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

	return int(rank) + 1, score, nil
}

// GetUserPosition returns a user's position with surrounding entries.
// Uses a two-phase pipeline approach: first gets rank + total + score,
// then fetches surrounding entries.
func (m *LeaderboardManager) GetUserPosition(
	ctx context.Context,
	contestID string,
	userID string,
	before int,
	after int,
) (*LeaderboardResponse, error) {
	lbKey := leaderboardKey(contestID)
	breakdownKey := scoreBreakdownKey(contestID)

	// Pipeline 1: Get user's rank + score + total participants
	pipe1 := m.redis.Pipeline()
	rankCmd := pipe1.ZRevRank(ctx, lbKey, userID)
	scoreCmd := pipe1.ZScore(ctx, lbKey, userID)
	cardCmd := pipe1.ZCard(ctx, lbKey)
	_, err := pipe1.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("failed to get user rank: %w", err)
	}

	rank, err := rankCmd.Result()
	if errors.Is(err, redis.Nil) {
		return nil, fmt.Errorf("user not found in leaderboard")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user rank: %w", err)
	}

	userScore, _ := scoreCmd.Result()
	totalParticipants, _ := cardCmd.Result()

	// Calculate range
	start := rank - int64(before)
	if start < 0 {
		start = 0
	}
	end := rank + int64(after)

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
	usernames := m.getDisplayNames(ctx, userIDs)

	// Get score breakdowns
	breakdowns := m.getBreakdowns(ctx, breakdownKey, userIDs)

	// Build entries
	entries := make([]LeaderboardEntry, len(results))
	for i, z := range results {
		uid := z.Member.(string)
		entry := LeaderboardEntry{
			Rank:       int(start) + i + 1,
			UserID:     uid,
			Username:   usernames[uid],
			TotalScore: z.Score,
		}
		if bd, ok := breakdowns[uid]; ok {
			entry.RealizedScore = bd.RealizedScore
			entry.UnrealizedScore = bd.UnrealizedScore
		}
		entries[i] = entry
	}

	userRank := int(rank) + 1

	return &LeaderboardResponse{
		ContestID:         contestID,
		Entries:           entries,
		TotalParticipants: totalParticipants,
		UserRank:          &userRank,
		UserScore:         &userScore,
		UpdatedAt:         time.Now().UTC(),
	}, nil
}

// getDisplayNames retrieves usernames for a list of user IDs.
func (m *LeaderboardManager) getDisplayNames(ctx context.Context, userIDs []string) map[string]string {
	if len(userIDs) == 0 || m.pool == nil {
		result := make(map[string]string)
		for _, id := range userIDs {
			result[id] = "" // Empty username if no DB
		}
		return result
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

	// Query database for missing usernames - extract from email
	if len(missingIDs) > 0 {
		placeholders := make([]string, len(missingIDs))
		args := make([]interface{}, len(missingIDs))
		for i, id := range missingIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}
		query := fmt.Sprintf(`
			SELECT id::text, COALESCE(NULLIF(display_name, ''), NULLIF(username, ''), SPLIT_PART(email, '@', 1)) as username
			FROM users
			WHERE id::text IN (%s)
		`, strings.Join(placeholders, ", "))

		rows, err := m.pool.QueryContext(ctx, query, args...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id, name string
				if err := rows.Scan(&id, &name); err == nil {
					result[id] = name
				}
			}
		}

		// Only cache newly-fetched entries
		for _, id := range missingIDs {
			if name, ok := result[id]; ok {
				m.usernameCache.Add(id, name)
			}
		}
	}

	// Ensure all IDs have an entry (even if empty)
	for _, id := range userIDs {
		if _, ok := result[id]; !ok {
			result[id] = ""
		}
	}

	return result
}

// serveLeaderboard is the shared implementation for leaderboard HTTP handlers.
func (a *App) serveLeaderboard(w http.ResponseWriter, r *http.Request, contestID string) {
	w.Header().Set("Content-Type", "application/json")

	if contestID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": tradeMsg.ContestIDRequired,
		})
		return
	}

	// Parse limit and offset
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Get current user from auth context (if authenticated)
	currentUserID := ""
	if userID, ok := r.Context().Value("user_id").(string); ok {
		currentUserID = userID
	}

	// Check if leaderboard manager is initialized
	if a.leaderboardMgr == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error": tradeMsg.LeaderboardNotAvailable,
		})
		return
	}

	response, err := a.leaderboardMgr.GetLeaderboard(r.Context(), contestID, limit, offset, currentUserID)
	if err != nil {
		a.log().Error("Failed to get leaderboard", zap.Error(err))
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": tradeMsg.LeaderboardFailed,
		})
		return
	}

	json.NewEncoder(w).Encode(response)
}

// handleLeaderboard is the HTTP handler for GET /api/trade/leaderboard
func (a *App) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	a.serveLeaderboard(w, r, r.URL.Query().Get("contest_id"))
}

// handleLeaderboardContest is the HTTP handler for GET /api/trade/leaderboard/{contest_id}
func (a *App) handleLeaderboardContest(w http.ResponseWriter, r *http.Request) {
	a.serveLeaderboard(w, r, chi.URLParam(r, "contest_id"))
}

// handleLeaderboardPosition is the HTTP handler for GET /api/trade/leaderboard/{contest_id}/position
func (a *App) handleLeaderboardPosition(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	contestID := chi.URLParam(r, "contest_id")
	if contestID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": tradeMsg.ContestIDRequired,
		})
		return
	}

	// Get user_id from query or auth context
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		if authUserID, ok := r.Context().Value("user_id").(string); ok {
			userID = authUserID
		}
	}
	if userID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": tradeMsg.UserIDRequired,
		})
		return
	}

	// Parse before/after counts
	before := 5
	if beforeStr := r.URL.Query().Get("before"); beforeStr != "" {
		if b, err := strconv.Atoi(beforeStr); err == nil && b >= 0 && b <= 50 {
			before = b
		}
	}

	after := 5
	if afterStr := r.URL.Query().Get("after"); afterStr != "" {
		if a, err := strconv.Atoi(afterStr); err == nil && a >= 0 && a <= 50 {
			after = a
		}
	}

	if a.leaderboardMgr == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"error": tradeMsg.LeaderboardNotAvailable,
		})
		return
	}

	response, err := a.leaderboardMgr.GetUserPosition(r.Context(), contestID, userID, before, after)
	if err != nil {
		a.log().Error("Failed to get user position", zap.Error(err))
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": tradeMsg.UserPositionNotFound,
		})
		return
	}

	json.NewEncoder(w).Encode(response)
}
