package server

import (
	"context"
	"math"
	"testing"
	"time"

	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/alicebob/miniredis/v2"
)

// setupTestRedis creates a miniredis instance and returns a redis client.
func setupTestRedis(t *testing.T) (*miniredis.Miniredis, *pkgredis.Client) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	cfg := pkgredis.Config{
		Mode: pkgredis.ModeStandalone,
		Addr: mr.Addr(),
	}

	client, err := pkgredis.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create redis client: %v", err)
	}

	return mr, client
}

func TestLeaderboardKey(t *testing.T) {
	tests := []struct {
		contestID string
		expected  string
	}{
		{"contest-123", "lb:{contest-123}"},
		{"abc", "lb:{abc}"},
		{"test-contest-with-long-id-12345", "lb:{test-contest-with-long-id-12345}"},
	}

	for _, tt := range tests {
		t.Run(tt.contestID, func(t *testing.T) {
			result := LeaderboardKey(tt.contestID)
			if result != tt.expected {
				t.Errorf("LeaderboardKey(%q) = %q, want %q", tt.contestID, result, tt.expected)
			}
		})
	}
}

func TestShardLeaderboardKey(t *testing.T) {
	tests := []struct {
		contestID string
		shardID   int
		expected  string
	}{
		{"contest-123", 0, "lb:{contest-123}:shard:0"},
		{"contest-123", 5, "lb:{contest-123}:shard:5"},
		{"abc", 1, "lb:{abc}:shard:1"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := ShardLeaderboardKey(tt.contestID, tt.shardID)
			if result != tt.expected {
				t.Errorf("ShardLeaderboardKey(%q, %d) = %q, want %q",
					tt.contestID, tt.shardID, result, tt.expected)
			}
		})
	}
}

func TestActiveContestsKey(t *testing.T) {
	tests := []struct {
		shardID  int
		expected string
	}{
		{0, "active_contests:{shard:0}"},
		{5, "active_contests:{shard:5}"},
		{10, "active_contests:{shard:10}"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := ActiveContestsKey(tt.shardID)
			if result != tt.expected {
				t.Errorf("ActiveContestsKey(%d) = %q, want %q", tt.shardID, result, tt.expected)
			}
		})
	}
}

func TestNewShardedLeaderboardWorker(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := ShardedLeaderboardConfig{
		ShardCount:             6,
		CacheTTL:               30 * time.Second,
		SignificantScoreChange: 100.0,
	}

	worker := NewShardedLeaderboardWorker(client, cfg)

	if worker == nil {
		t.Fatal("NewShardedLeaderboardWorker returned nil")
	}

	if worker.shardCount != 6 {
		t.Errorf("shardCount = %d, want 6", worker.shardCount)
	}

	if worker.cacheTTL != 30*time.Second {
		t.Errorf("cacheTTL = %v, want 30s", worker.cacheTTL)
	}
}

func TestShardedLeaderboardWorker_UpdateLeaderboard(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := DefaultShardedLeaderboardConfig()
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-123"
	shardID := 0

	entries := []LeaderboardEntry{
		{UserID: "user-1", Score: 1000.0},
		{UserID: "user-2", Score: 900.0},
		{UserID: "user-3", Score: 800.0},
	}

	err := worker.UpdateLeaderboard(ctx, contestID, shardID, entries)
	if err != nil {
		t.Fatalf("UpdateLeaderboard failed: %v", err)
	}

	// Verify the entries were stored correctly
	key := LeaderboardKey(contestID)
	score, err := client.ZScore(ctx, key, "user-1").Result()
	if err != nil {
		t.Fatalf("ZScore failed: %v", err)
	}
	if score != 1000.0 {
		t.Errorf("user-1 score = %f, want 1000.0", score)
	}
}

func TestShardedLeaderboardWorker_UpdateScore(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := DefaultShardedLeaderboardConfig()
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-456"
	userID := "user-test"

	// Update score
	err := worker.UpdateScore(ctx, contestID, userID, 500.0, 0.0, 100.0)
	if err != nil {
		t.Fatalf("UpdateScore failed: %v", err)
	}

	// Verify score
	rank, err := worker.GetUserRank(ctx, contestID, userID)
	if err != nil {
		t.Fatalf("GetUserRank failed: %v", err)
	}
	if rank == nil {
		t.Fatal("GetUserRank returned nil")
	}
	if math.Abs(rank.Score-500.0) > 1e-6 {
		t.Errorf("score = %f, want ~500.0 (within 1e-6)", rank.Score)
	}
}

func TestShardedLeaderboardWorker_GetTop(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := DefaultShardedLeaderboardConfig()
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-top"

	// Add entries
	entries := []LeaderboardEntry{
		{UserID: "alice", Score: 1500.0},
		{UserID: "bob", Score: 1200.0},
		{UserID: "charlie", Score: 1800.0},
		{UserID: "dave", Score: 900.0},
		{UserID: "eve", Score: 2000.0},
	}

	err := worker.UpdateLeaderboard(ctx, contestID, 0, entries)
	if err != nil {
		t.Fatalf("UpdateLeaderboard failed: %v", err)
	}

	// Get top 3
	top3, err := worker.GetTop(ctx, contestID, 3)
	if err != nil {
		t.Fatalf("GetTop failed: %v", err)
	}

	if len(top3) != 3 {
		t.Fatalf("got %d entries, want 3", len(top3))
	}

	// Verify order (highest score first)
	expectedOrder := []string{"eve", "charlie", "alice"}
	for i, entry := range top3 {
		if entry.UserID != expectedOrder[i] {
			t.Errorf("top3[%d].UserID = %s, want %s", i, entry.UserID, expectedOrder[i])
		}
		if entry.Rank != i+1 {
			t.Errorf("top3[%d].Rank = %d, want %d", i, entry.Rank, i+1)
		}
	}
}

func TestShardedLeaderboardWorker_GetTop100(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := DefaultShardedLeaderboardConfig()
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-100"

	// Add 150 entries
	entries := make([]LeaderboardEntry, 150)
	for i := 0; i < 150; i++ {
		entries[i] = LeaderboardEntry{
			UserID: "user-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Score:  float64(i * 100),
		}
	}

	err := worker.UpdateLeaderboard(ctx, contestID, 0, entries)
	if err != nil {
		t.Fatalf("UpdateLeaderboard failed: %v", err)
	}

	// Get top 100
	top100, err := worker.GetTop100(ctx, contestID)
	if err != nil {
		t.Fatalf("GetTop100 failed: %v", err)
	}

	if len(top100) != 100 {
		t.Errorf("got %d entries, want 100", len(top100))
	}

	// Verify highest score is first
	if top100[0].Score < top100[99].Score {
		t.Error("entries not sorted by score descending")
	}
}

func TestShardedLeaderboardWorker_GetUserRank(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := DefaultShardedLeaderboardConfig()
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-rank"

	entries := []LeaderboardEntry{
		{UserID: "first", Score: 1000.0},
		{UserID: "second", Score: 800.0},
		{UserID: "third", Score: 600.0},
	}

	err := worker.UpdateLeaderboard(ctx, contestID, 0, entries)
	if err != nil {
		t.Fatalf("UpdateLeaderboard failed: %v", err)
	}

	// Test user in leaderboard
	rank, err := worker.GetUserRank(ctx, contestID, "second")
	if err != nil {
		t.Fatalf("GetUserRank failed: %v", err)
	}
	if rank == nil {
		t.Fatal("GetUserRank returned nil for existing user")
	}
	if rank.Rank != 2 {
		t.Errorf("rank = %d, want 2", rank.Rank)
	}
	if rank.Score != 800.0 {
		t.Errorf("score = %f, want 800.0", rank.Score)
	}

	// Test user not in leaderboard
	noRank, err := worker.GetUserRank(ctx, contestID, "nonexistent")
	if err != nil {
		t.Fatalf("GetUserRank failed: %v", err)
	}
	if noRank != nil {
		t.Error("GetUserRank should return nil for nonexistent user")
	}
}

func TestShardedLeaderboardWorker_GetLeaderboardSize(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := DefaultShardedLeaderboardConfig()
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-size"

	// Empty leaderboard
	size, err := worker.GetLeaderboardSize(ctx, contestID)
	if err != nil {
		t.Fatalf("GetLeaderboardSize failed: %v", err)
	}
	if size != 0 {
		t.Errorf("empty leaderboard size = %d, want 0", size)
	}

	// Add entries
	entries := []LeaderboardEntry{
		{UserID: "user-1", Score: 100.0},
		{UserID: "user-2", Score: 200.0},
		{UserID: "user-3", Score: 300.0},
	}

	err = worker.UpdateLeaderboard(ctx, contestID, 0, entries)
	if err != nil {
		t.Fatalf("UpdateLeaderboard failed: %v", err)
	}

	size, err = worker.GetLeaderboardSize(ctx, contestID)
	if err != nil {
		t.Fatalf("GetLeaderboardSize failed: %v", err)
	}
	if size != 3 {
		t.Errorf("size = %d, want 3", size)
	}
}

func TestShardedLeaderboardWorker_GetContestLeaderboardWithSurrounding(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := DefaultShardedLeaderboardConfig()
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-surround"

	// Add 10 entries
	entries := make([]LeaderboardEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = LeaderboardEntry{
			UserID: "user-" + string(rune('a'+i)),
			Score:  float64((10 - i) * 100),
		}
	}

	err := worker.UpdateLeaderboard(ctx, contestID, 0, entries)
	if err != nil {
		t.Fatalf("UpdateLeaderboard failed: %v", err)
	}

	// Get surrounding for user-e (5th place)
	surrounding, err := worker.GetContestLeaderboardWithSurrounding(ctx, contestID, "user-e", 2, 2)
	if err != nil {
		t.Fatalf("GetContestLeaderboardWithSurrounding failed: %v", err)
	}

	// Should get 5 entries (2 before, user-e, 2 after)
	if len(surrounding) != 5 {
		t.Errorf("got %d entries, want 5", len(surrounding))
	}
}

func TestShardedLeaderboardWorker_BatchUpdateScores(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := DefaultShardedLeaderboardConfig()
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-batch"

	updates := map[string]float64{
		"user-1": 1000.0,
		"user-2": 2000.0,
		"user-3": 1500.0,
	}

	failedUsers, err := worker.BatchUpdateScores(ctx, contestID, updates)
	if err != nil {
		t.Fatalf("BatchUpdateScores failed: %v", err)
	}
	if len(failedUsers) != 0 {
		t.Fatalf("BatchUpdateScores returned unexpected failed users: %v", failedUsers)
	}

	// Verify all updates
	for userID, expectedScore := range updates {
		rank, err := worker.GetUserRank(ctx, contestID, userID)
		if err != nil {
			t.Fatalf("GetUserRank failed for %s: %v", userID, err)
		}
		if rank == nil {
			t.Fatalf("GetUserRank returned nil for %s", userID)
		}
		if math.Abs(rank.Score-expectedScore) > 1e-6 {
			t.Errorf("%s score = %f, want ~%f (within 1e-6)", userID, rank.Score, expectedScore)
		}
	}
}

func TestShardedLeaderboardWorker_CacheInvalidation(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := ShardedLeaderboardConfig{
		ShardCount:             6,
		CacheTTL:               1 * time.Second,
		SignificantScoreChange: 100.0,
	}
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-cache"

	// Add initial entries
	entries := []LeaderboardEntry{
		{UserID: "user-1", Score: 1000.0},
	}
	err := worker.UpdateLeaderboard(ctx, contestID, 0, entries)
	if err != nil {
		t.Fatalf("UpdateLeaderboard failed: %v", err)
	}

	// First call should populate cache
	_, err = worker.GetTop(ctx, contestID, 10)
	if err != nil {
		t.Fatalf("GetTop failed: %v", err)
	}

	// Verify cache exists
	cached := worker.getCachedLeaderboard(contestID)
	if cached == nil {
		t.Fatal("cache should be populated after GetTop")
	}

	// Invalidate cache
	worker.invalidateCache(contestID)

	// Verify cache is invalidated
	cached = worker.getCachedLeaderboard(contestID)
	if cached != nil {
		t.Error("cache should be nil after invalidation")
	}
}

func TestMergeLeaderboards(t *testing.T) {
	board1 := []LeaderboardEntry{
		{Rank: 1, UserID: "alice", Score: 1000.0},
		{Rank: 2, UserID: "bob", Score: 800.0},
	}

	board2 := []LeaderboardEntry{
		{Rank: 1, UserID: "charlie", Score: 1500.0},
		{Rank: 2, UserID: "dave", Score: 500.0},
	}

	merged := MergeLeaderboards(board1, board2)

	if len(merged) != 4 {
		t.Fatalf("merged length = %d, want 4", len(merged))
	}

	// Verify order and ranks
	expectedOrder := []string{"charlie", "alice", "bob", "dave"}
	for i, entry := range merged {
		if entry.UserID != expectedOrder[i] {
			t.Errorf("merged[%d].UserID = %s, want %s", i, entry.UserID, expectedOrder[i])
		}
		if entry.Rank != i+1 {
			t.Errorf("merged[%d].Rank = %d, want %d", i, entry.Rank, i+1)
		}
	}
}

func TestDefaultShardedLeaderboardConfig(t *testing.T) {
	cfg := DefaultShardedLeaderboardConfig()

	if cfg.ShardCount != 6 {
		t.Errorf("ShardCount = %d, want 6", cfg.ShardCount)
	}
	if cfg.CacheTTL != 30*time.Second {
		t.Errorf("CacheTTL = %v, want 30s", cfg.CacheTTL)
	}
	if cfg.SignificantScoreChange != 100.0 {
		t.Errorf("SignificantScoreChange = %f, want 100.0", cfg.SignificantScoreChange)
	}
}

func TestShardedLeaderboardWorker_CleanupExpiredCache(t *testing.T) {
	mr, client := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	cfg := ShardedLeaderboardConfig{
		ShardCount:             6,
		CacheTTL:               10 * time.Millisecond, // Very short TTL for testing
		SignificantScoreChange: 100.0,
	}
	worker := NewShardedLeaderboardWorker(client, cfg)

	ctx := context.Background()
	contestID := "contest-cleanup"

	// Add entries and populate cache
	entries := []LeaderboardEntry{
		{UserID: "user-1", Score: 1000.0},
	}
	err := worker.UpdateLeaderboard(ctx, contestID, 0, entries)
	if err != nil {
		t.Fatalf("UpdateLeaderboard failed: %v", err)
	}

	_, err = worker.GetTop(ctx, contestID, 10)
	if err != nil {
		t.Fatalf("GetTop failed: %v", err)
	}

	// Verify cache exists
	if worker.getCachedLeaderboard(contestID) == nil {
		t.Fatal("cache should exist")
	}

	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)

	// Run cleanup
	worker.CleanupExpiredCache()

	// Verify cache is cleaned up
	worker.cacheMu.RLock()
	_, exists := worker.globalCache[contestID]
	worker.cacheMu.RUnlock()

	if exists {
		t.Error("expired cache entry should be cleaned up")
	}
}

func TestEncodeTiebreaker_Deterministic(t *testing.T) {
	// Different users with the same score must get different tiebreaker values
	now := time.Now()
	s1 := encodeTiebreaker(100.0, "user-alice", now)
	s2 := encodeTiebreaker(100.0, "user-bob", now)

	if s1 == s2 {
		t.Error("encodeTiebreaker should produce different values for different users with the same score")
	}

	// Both should be very close to the original score (offset < 1e-6)
	if math.Abs(s1-100.0) > 1e-6 {
		t.Errorf("tiebreaker offset too large for alice: %e", s1-100.0)
	}
	if math.Abs(s2-100.0) > 1e-6 {
		t.Errorf("tiebreaker offset too large for bob: %e", s2-100.0)
	}
}

func TestEncodeTiebreaker_Ordering(t *testing.T) {
	// A higher base score must always rank higher regardless of tiebreaker
	now := time.Now()
	high := encodeTiebreaker(200.0, "user-late", now)
	low := encodeTiebreaker(100.0, "user-early", now)
	if high <= low {
		t.Errorf("higher score (%f) should rank above lower score (%f)", high, low)
	}
}

func TestEncodeTiebreaker_StableForSameTimestamp(t *testing.T) {
	// With the same timestamp, the same user and score should produce identical tiebreaker
	ts := time.Now()
	s1 := encodeTiebreaker(100.0, "user-alice", ts)
	s2 := encodeTiebreaker(100.0, "user-alice", ts)
	if s1 != s2 {
		t.Errorf("same user/score/timestamp should produce identical tiebreaker: %f != %f", s1, s2)
	}
}
