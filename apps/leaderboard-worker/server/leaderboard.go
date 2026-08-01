package server

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// LeaderboardEntry represents a single entry in the leaderboard.
type LeaderboardEntry struct {
	Rank   int     `json:"rank"`
	UserID string  `json:"user_id"`
	Score  float64 `json:"score"`
}

// LeaderboardSnapshot represents a point-in-time leaderboard snapshot.
type LeaderboardSnapshot struct {
	ContestID string             `json:"contest_id"`
	TakenAt   time.Time          `json:"taken_at"`
	Entries   []LeaderboardEntry `json:"entries"`
}

// UserRank represents a user's rank and score in a leaderboard.
type UserRank struct {
	UserID string  `json:"user_id"`
	Rank   int     `json:"rank"`
	Score  float64 `json:"score"`
}

// leaderboardKey returns the Redis key for a contest's leaderboard.
// Uses hash tags {contestID} to be consistent with the sharded leaderboard
// key format (LeaderboardKey), ensuring both paths read/write the same keys.
func leaderboardKey(contestID string) string {
	return "lb:{" + contestID + "}"
}

// GetTop100 retrieves the top 100 entries from a contest's leaderboard.
// This is a convenience wrapper around GetTop.
func GetTop100(ctx context.Context, rdb redis.Cmdable, contestID string) ([]LeaderboardEntry, error) {
	return GetTop(ctx, rdb, contestID, 100)
}

// GetTop retrieves the top N entries from a contest's leaderboard.
// Entries are sorted by score descending (highest score = rank 1).
func GetTop(ctx context.Context, rdb redis.Cmdable, contestID string, n int) ([]LeaderboardEntry, error) {
	key := leaderboardKey(contestID)

	// ZREVRANGE with WITHSCORES returns members from highest to lowest score
	results, err := rdb.ZRevRangeWithScores(ctx, key, 0, int64(n-1)).Result()
	if err != nil {
		return nil, err
	}

	entries := make([]LeaderboardEntry, 0, len(results))
	for i, z := range results {
		entries = append(entries, LeaderboardEntry{
			Rank:   i + 1,
			UserID: z.Member.(string),
			Score:  z.Score,
		})
	}

	return entries, nil
}

// GetUserRank retrieves a user's rank and score in a contest's leaderboard.
// Returns nil if the user is not in the leaderboard.
func GetUserRank(ctx context.Context, rdb redis.Cmdable, contestID, userID string) (*UserRank, error) {
	key := leaderboardKey(contestID)

	// Get user's score
	score, err := rdb.ZScore(ctx, key, userID).Result()
	if err == redis.Nil {
		return nil, nil // User not in leaderboard
	}
	if err != nil {
		return nil, err
	}

	// Get user's rank (0-indexed, highest score = 0)
	rank, err := rdb.ZRevRank(ctx, key, userID).Result()
	if err == redis.Nil {
		return nil, nil // User not in leaderboard
	}
	if err != nil {
		return nil, err
	}

	return &UserRank{
		UserID: userID,
		Rank:   int(rank) + 1, // Convert to 1-indexed
		Score:  score,
	}, nil
}

// GetLeaderboardSize returns the number of participants in a contest's leaderboard.
func GetLeaderboardSize(ctx context.Context, rdb redis.Cmdable, contestID string) (int64, error) {
	key := leaderboardKey(contestID)
	return rdb.ZCard(ctx, key).Result()
}

// GetScoreRange retrieves entries within a specific score range.
// This can be useful for finding participants near a certain score.
func GetScoreRange(ctx context.Context, rdb redis.Cmdable, contestID string, minScore, maxScore float64, limit int) ([]LeaderboardEntry, error) {
	key := leaderboardKey(contestID)

	// Get members in the score range, ordered by score descending
	results, err := rdb.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
		Min:   formatScore(minScore),
		Max:   formatScore(maxScore),
		Count: int64(limit),
	}).Result()
	if err != nil {
		return nil, err
	}

	entries := make([]LeaderboardEntry, 0, len(results))
	for _, z := range results {
		// We need to get the actual rank for each entry
		rank, err := rdb.ZRevRank(ctx, key, z.Member.(string)).Result()
		if err != nil {
			continue
		}
		entries = append(entries, LeaderboardEntry{
			Rank:   int(rank) + 1,
			UserID: z.Member.(string),
			Score:  z.Score,
		})
	}

	return entries, nil
}

// GetNearbyRanks retrieves entries around a user's rank.
// Returns entries from (userRank - before) to (userRank + after).
func GetNearbyRanks(ctx context.Context, rdb redis.Cmdable, contestID, userID string, before, after int) ([]LeaderboardEntry, error) {
	key := leaderboardKey(contestID)

	// Get user's rank first
	rank, err := rdb.ZRevRank(ctx, key, userID).Result()
	if err == redis.Nil {
		return nil, nil // User not in leaderboard
	}
	if err != nil {
		return nil, err
	}

	// Calculate range
	start := rank - int64(before)
	if start < 0 {
		start = 0
	}
	end := rank + int64(after)

	// Get entries in range
	results, err := rdb.ZRevRangeWithScores(ctx, key, start, end).Result()
	if err != nil {
		return nil, err
	}

	entries := make([]LeaderboardEntry, 0, len(results))
	for i, z := range results {
		entries = append(entries, LeaderboardEntry{
			Rank:   int(start) + i + 1,
			UserID: z.Member.(string),
			Score:  z.Score,
		})
	}

	return entries, nil
}

// formatScore formats a float64 score for Redis range queries.
func formatScore(score float64) string {
	return strconv.FormatFloat(score, 'f', -1, 64)
}
