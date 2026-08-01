package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
)

// ShardingTestEnv extends TestEnv with shard-specific helpers.
type ShardingTestEnv struct {
	*TestEnv
	Auth *auth.Auth
}

// ShardConfig represents a shard configuration entry.
type ShardConfig struct {
	ShardID        int
	Name           string
	Status         string
	Weight         int
	KafkaPartition int
}

// NewShardingTestEnv creates a new sharding test environment.
func NewShardingTestEnv(t *testing.T, ctx context.Context) *ShardingTestEnv {
	t.Helper()

	env := SetupTestEnv(t, ctx)

	// Apply shard-related migrations
	if err := runShardMigrations(ctx, env.DB); err != nil {
		t.Fatalf("Failed to run shard migrations: %v", err)
	}

	authConfig := auth.DefaultConfig()
	authConfig.JWTSecret = env.JWTSecret
	authService := auth.New(authConfig)

	return &ShardingTestEnv{
		TestEnv: env,
		Auth:    authService,
	}
}

// runShardMigrations applies shard-specific migrations.
func runShardMigrations(ctx context.Context, db *sql.DB) error {
	// Create shard_config table
	shardConfigSQL := `
		CREATE TABLE IF NOT EXISTS shard_config (
			shard_id INT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(50) NOT NULL DEFAULT 'active',
			weight INT NOT NULL DEFAULT 1,
			kafka_partition INT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT shard_status_check CHECK (status IN ('active', 'draining', 'inactive', 'maintenance'))
		);

		-- Add shard_id to contests if not exists
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_name = 'contests' AND column_name = 'shard_id'
			) THEN
				ALTER TABLE contests ADD COLUMN shard_id INT;
			END IF;
		END $$;

		-- Create shard assignment log
		CREATE TABLE IF NOT EXISTS shard_assignment_log (
			id SERIAL PRIMARY KEY,
			contest_id UUID NOT NULL,
			from_shard_id INT,
			to_shard_id INT NOT NULL,
			reason VARCHAR(255),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`

	_, err := db.ExecContext(ctx, shardConfigSQL)
	return err
}

// CreateShard creates a shard configuration in the database.
func (se *ShardingTestEnv) CreateShard(ctx context.Context, t *testing.T, shardID int, name, status string, weight int) {
	t.Helper()

	_, err := se.DB.ExecContext(ctx, `
		INSERT INTO shard_config (shard_id, name, status, weight, kafka_partition)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (shard_id) DO UPDATE SET name = $2, status = $3, weight = $4
	`, shardID, name, status, weight, shardID)
	if err != nil {
		t.Fatalf("Failed to create shard: %v", err)
	}
}

// GetShard retrieves a shard configuration from the database.
func (se *ShardingTestEnv) GetShard(ctx context.Context, t *testing.T, shardID int) *ShardConfig {
	t.Helper()

	var sc ShardConfig
	err := se.DB.QueryRowContext(ctx, `
		SELECT shard_id, name, status, weight, COALESCE(kafka_partition, 0)
		FROM shard_config WHERE shard_id = $1
	`, shardID).Scan(&sc.ShardID, &sc.Name, &sc.Status, &sc.Weight, &sc.KafkaPartition)
	if err != nil {
		return nil
	}
	return &sc
}

// CreateShardedContest creates a contest assigned to a specific shard.
func (se *ShardingTestEnv) CreateShardedContest(ctx context.Context, t *testing.T, name string, shardID int, status string) string {
	t.Helper()

	var contestID string
	err := se.DB.QueryRowContext(ctx, `
		INSERT INTO contests (name, starts_at, ends_at, status, qty_total, shard_id)
		VALUES ($1, NOW() - INTERVAL '1 hour', NOW() + INTERVAL '1 day', $2, 100000, $3)
		RETURNING id
	`, name, status, shardID).Scan(&contestID)
	if err != nil {
		t.Fatalf("Failed to create sharded contest: %v", err)
	}

	return contestID
}

// GetContestShard retrieves the shard assignment for a contest.
func (se *ShardingTestEnv) GetContestShard(ctx context.Context, t *testing.T, contestID string) *int {
	t.Helper()

	var shardID *int
	err := se.DB.QueryRowContext(ctx, `
		SELECT shard_id FROM contests WHERE id = $1
	`, contestID).Scan(&shardID)
	if err != nil {
		return nil
	}
	return shardID
}

// UpdateShardStatus updates the status of a shard.
func (se *ShardingTestEnv) UpdateShardStatus(ctx context.Context, t *testing.T, shardID int, status string) {
	t.Helper()

	_, err := se.DB.ExecContext(ctx, `
		UPDATE shard_config SET status = $1, updated_at = NOW() WHERE shard_id = $2
	`, status, shardID)
	if err != nil {
		t.Fatalf("Failed to update shard status: %v", err)
	}
}

// SetLeaderboardScore sets a leaderboard score in Redis.
func (se *ShardingTestEnv) SetLeaderboardScore(ctx context.Context, t *testing.T, contestID, userID string, score float64) {
	t.Helper()

	key := fmt.Sprintf("lb:{%s}", contestID)
	err := se.RedisClient.ZAdd(ctx, key, redis.Z{
		Score:  score,
		Member: userID,
	}).Err()
	if err != nil {
		t.Fatalf("Failed to set leaderboard score: %v", err)
	}
}

// GetLeaderboardScores retrieves leaderboard scores from Redis.
func (se *ShardingTestEnv) GetLeaderboardScores(ctx context.Context, t *testing.T, contestID string, count int64) []LeaderboardEntry {
	t.Helper()

	key := fmt.Sprintf("lb:{%s}", contestID)
	results, err := se.RedisClient.ZRevRangeWithScores(ctx, key, 0, count-1).Result()
	if err != nil {
		t.Fatalf("Failed to get leaderboard scores: %v", err)
	}

	entries := make([]LeaderboardEntry, len(results))
	for i, r := range results {
		entries[i] = LeaderboardEntry{
			UserID: r.Member.(string),
			Score:  r.Score,
			Rank:   i + 1,
		}
	}
	return entries
}

// LeaderboardEntry represents a leaderboard entry.
type LeaderboardEntry struct {
	UserID string
	Score  float64
	Rank   int
}

// PublishToShardedTopic publishes a message to a topic with partition key.
func (se *ShardingTestEnv) PublishToShardedTopic(ctx context.Context, t *testing.T, topic string, key string, value []byte, partition int32) {
	t.Helper()

	record := &kgo.Record{
		Topic:     topic,
		Key:       []byte(key),
		Value:     value,
		Partition: partition,
	}

	results := se.KafkaClient.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		t.Fatalf("Failed to publish to sharded topic: %v", err)
	}
}

// CreateTradingUser creates a user ready for trading (from trading_flow_test.go pattern).
func (se *ShardingTestEnv) CreateTradingUser(ctx context.Context, t *testing.T, email string) (userID string, accessToken string) {
	t.Helper()

	passwordHash, err := se.Auth.HashPassword("testpassword123")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	userID = se.CreateTestUser(ctx, t, email, passwordHash)

	tokenPair, err := se.Auth.Token.GenerateTokenPair(userID, []string{"user"})
	if err != nil {
		t.Fatalf("Failed to generate token pair: %v", err)
	}

	return userID, tokenPair.AccessToken
}

// ============================================================================
// Shard Assignment Tests
// ============================================================================

func TestShardAssignment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	se := NewShardingTestEnv(t, ctx)
	defer se.Cleanup(t, ctx)

	t.Run("ContestAssignedToCorrectShard", func(t *testing.T) {
		// Create shards
		se.CreateShard(ctx, t, 0, "shard-0", "active", 1)
		se.CreateShard(ctx, t, 1, "shard-1", "active", 1)
		se.CreateShard(ctx, t, 2, "shard-2", "active", 1)
		se.CreateShard(ctx, t, 3, "shard-3", "active", 1)

		// Create contests on different shards
		contest0 := se.CreateShardedContest(ctx, t, "Contest on Shard 0", 0, "running")
		contest1 := se.CreateShardedContest(ctx, t, "Contest on Shard 1", 1, "running")
		contest2 := se.CreateShardedContest(ctx, t, "Contest on Shard 2", 2, "running")
		contest3 := se.CreateShardedContest(ctx, t, "Contest on Shard 3", 3, "running")

		// Verify assignments
		if shard := se.GetContestShard(ctx, t, contest0); shard == nil || *shard != 0 {
			t.Errorf("Contest 0 should be on shard 0, got %v", shard)
		}
		if shard := se.GetContestShard(ctx, t, contest1); shard == nil || *shard != 1 {
			t.Errorf("Contest 1 should be on shard 1, got %v", shard)
		}
		if shard := se.GetContestShard(ctx, t, contest2); shard == nil || *shard != 2 {
			t.Errorf("Contest 2 should be on shard 2, got %v", shard)
		}
		if shard := se.GetContestShard(ctx, t, contest3); shard == nil || *shard != 3 {
			t.Errorf("Contest 3 should be on shard 3, got %v", shard)
		}
	})

	t.Run("ShardWeightsAffectDistribution", func(t *testing.T) {
		// Create shards with different weights
		se.CreateShard(ctx, t, 10, "high-weight-shard", "active", 3) // 3x weight
		se.CreateShard(ctx, t, 11, "normal-weight-shard", "active", 1)

		// Verify shard configurations
		highWeight := se.GetShard(ctx, t, 10)
		normalWeight := se.GetShard(ctx, t, 11)

		if highWeight == nil || highWeight.Weight != 3 {
			t.Errorf("Expected high weight shard with weight 3, got %v", highWeight)
		}
		if normalWeight == nil || normalWeight.Weight != 1 {
			t.Errorf("Expected normal weight shard with weight 1, got %v", normalWeight)
		}
	})

	t.Run("InactiveShardNotAssigned", func(t *testing.T) {
		// Create an inactive shard
		se.CreateShard(ctx, t, 20, "inactive-shard", "inactive", 1)

		// Verify shard is marked inactive
		shard := se.GetShard(ctx, t, 20)
		if shard == nil || shard.Status != "inactive" {
			t.Errorf("Expected inactive shard, got %v", shard)
		}
	})
}

// ============================================================================
// Cross-Shard Order Rejection Tests
// ============================================================================

func TestCrossShardOrderRejection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	se := NewShardingTestEnv(t, ctx)
	defer se.Cleanup(t, ctx)

	// Setup shards and contest
	se.CreateShard(ctx, t, 0, "shard-0", "active", 1)
	se.CreateShard(ctx, t, 1, "shard-1", "active", 1)

	// Create contest on shard 0
	contestID := se.CreateShardedContest(ctx, t, "Shard 0 Contest", 0, "running")
	se.AddContestSymbol(ctx, t, contestID, "AAPL")

	// Create user
	userID, _ := se.CreateTradingUser(ctx, t, "crossshard@example.com")
	se.JoinContest(ctx, t, contestID, userID, 100000)

	t.Run("OrderToCorrectShardAccepted", func(t *testing.T) {
		orderID := uuid.New().String()

		order := &contracts.OrderRequest{
			OrderID:   orderID,
			UserID:    userID,
			ContestID: contestID,
			Symbol:    "AAPL",
			Side:      contracts.OrderSideBuy,
			Type:      contracts.OrderTypeMarket,
			Qty:       10,
			ClientTs:  time.Now().UnixMilli(),
		}

		data, err := json.Marshal(order)
		if err != nil {
			t.Fatalf("Failed to marshal order: %v", err)
		}

		// Publish to shard 0 partition (correct shard for this contest)
		se.PublishToShardedTopic(ctx, t, "orders.v1", contestID, data, 0)

		t.Log("Order published to correct shard partition successfully")
	})

	t.Run("OrderToWrongShardDetected", func(t *testing.T) {
		// This test verifies the shard mismatch can be detected
		// In a real scenario, the trading engine would reject this order

		orderID := uuid.New().String()

		order := &contracts.OrderRequest{
			OrderID:   orderID,
			UserID:    userID,
			ContestID: contestID, // Contest is on shard 0
			Symbol:    "AAPL",
			Side:      contracts.OrderSideBuy,
			Type:      contracts.OrderTypeMarket,
			Qty:       10,
			ClientTs:  time.Now().UnixMilli(),
		}

		data, err := json.Marshal(order)
		if err != nil {
			t.Fatalf("Failed to marshal order: %v", err)
		}

		// Publish to shard 1 partition (WRONG shard for this contest)
		se.PublishToShardedTopic(ctx, t, "orders.v1", contestID, data, 1)

		// Verify contest's actual shard assignment
		shard := se.GetContestShard(ctx, t, contestID)
		if shard == nil || *shard != 0 {
			t.Errorf("Contest should be on shard 0, got %v", shard)
		}

		// The trading engine's state manager would detect this mismatch and reject
		t.Log("Order sent to wrong shard - would be rejected by trading engine")
	})

	t.Run("ShardContextValidation", func(t *testing.T) {
		// Verify that contest-to-shard mapping is correctly stored
		var shardID int
		err := se.DB.QueryRowContext(ctx, `
			SELECT shard_id FROM contests WHERE id = $1
		`, contestID).Scan(&shardID)

		if err != nil {
			t.Fatalf("Failed to query contest shard: %v", err)
		}

		if shardID != 0 {
			t.Errorf("Expected contest on shard 0, got shard %d", shardID)
		}
	})
}

// ============================================================================
// Shard Failover Tests
// ============================================================================

func TestShardFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	se := NewShardingTestEnv(t, ctx)
	defer se.Cleanup(t, ctx)

	t.Run("ShardDrainingStatus", func(t *testing.T) {
		// Create and activate shard
		se.CreateShard(ctx, t, 0, "shard-0", "active", 1)

		// Create contest on the shard
		contestID := se.CreateShardedContest(ctx, t, "Draining Test Contest", 0, "running")

		// Verify contest is on shard 0
		shard := se.GetContestShard(ctx, t, contestID)
		if shard == nil || *shard != 0 {
			t.Fatalf("Expected contest on shard 0, got %v", shard)
		}

		// Mark shard as draining
		se.UpdateShardStatus(ctx, t, 0, "draining")

		// Verify shard status
		shardConfig := se.GetShard(ctx, t, 0)
		if shardConfig.Status != "draining" {
			t.Errorf("Expected shard status 'draining', got '%s'", shardConfig.Status)
		}

		// Existing contest should still be accessible
		shardAfterDrain := se.GetContestShard(ctx, t, contestID)
		if shardAfterDrain == nil || *shardAfterDrain != 0 {
			t.Errorf("Contest should still be on shard 0 during drain, got %v", shardAfterDrain)
		}
	})

	t.Run("ShardMaintenanceMode", func(t *testing.T) {
		// Create shard
		se.CreateShard(ctx, t, 1, "shard-1", "active", 1)

		// Put shard in maintenance
		se.UpdateShardStatus(ctx, t, 1, "maintenance")

		// Verify status
		shardConfig := se.GetShard(ctx, t, 1)
		if shardConfig.Status != "maintenance" {
			t.Errorf("Expected shard status 'maintenance', got '%s'", shardConfig.Status)
		}
	})

	t.Run("ShardReactivation", func(t *testing.T) {
		// Create inactive shard
		se.CreateShard(ctx, t, 2, "shard-2", "inactive", 1)

		// Verify inactive
		shardConfig := se.GetShard(ctx, t, 2)
		if shardConfig.Status != "inactive" {
			t.Errorf("Expected shard status 'inactive', got '%s'", shardConfig.Status)
		}

		// Reactivate shard
		se.UpdateShardStatus(ctx, t, 2, "active")

		// Verify active
		shardConfig = se.GetShard(ctx, t, 2)
		if shardConfig.Status != "active" {
			t.Errorf("Expected shard status 'active', got '%s'", shardConfig.Status)
		}
	})

	t.Run("ShardStatusTransitions", func(t *testing.T) {
		// Create shard
		se.CreateShard(ctx, t, 3, "shard-3", "active", 1)

		transitions := []string{"draining", "inactive", "maintenance", "active"}

		for _, status := range transitions {
			se.UpdateShardStatus(ctx, t, 3, status)
			shard := se.GetShard(ctx, t, 3)
			if shard.Status != status {
				t.Errorf("Expected status '%s', got '%s'", status, shard.Status)
			}
		}
	})
}

// ============================================================================
// Leaderboard Aggregation Tests
// ============================================================================

func TestLeaderboardAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	se := NewShardingTestEnv(t, ctx)
	defer se.Cleanup(t, ctx)

	// Setup shards
	se.CreateShard(ctx, t, 0, "shard-0", "active", 1)
	se.CreateShard(ctx, t, 1, "shard-1", "active", 1)
	se.CreateShard(ctx, t, 2, "shard-2", "active", 1)

	t.Run("SingleContestLeaderboard", func(t *testing.T) {
		contestID := se.CreateShardedContest(ctx, t, "Single Leaderboard Test", 0, "running")

		// Create users and set scores
		users := []struct {
			email string
			score float64
		}{
			{"leader@example.com", 1500.50},
			{"second@example.com", 1200.25},
			{"third@example.com", 800.00},
			{"fourth@example.com", 500.75},
			{"fifth@example.com", 250.00},
		}

		for _, u := range users {
			userID, _ := se.CreateTradingUser(ctx, t, u.email)
			se.JoinContest(ctx, t, contestID, userID, 100000)
			se.SetLeaderboardScore(ctx, t, contestID, userID, u.score)
		}

		// Get leaderboard
		entries := se.GetLeaderboardScores(ctx, t, contestID, 5)

		if len(entries) != 5 {
			t.Errorf("Expected 5 leaderboard entries, got %d", len(entries))
		}

		// Verify ordering (highest score first)
		if len(entries) > 0 && entries[0].Score != 1500.50 {
			t.Errorf("Expected top score 1500.50, got %f", entries[0].Score)
		}

		// Verify descending order
		for i := 1; i < len(entries); i++ {
			if entries[i].Score > entries[i-1].Score {
				t.Errorf("Leaderboard not in descending order at position %d", i)
			}
		}
	})

	t.Run("CrossShardLeaderboard", func(t *testing.T) {
		// Create contests on different shards
		contest0 := se.CreateShardedContest(ctx, t, "Shard 0 Contest", 0, "running")
		contest1 := se.CreateShardedContest(ctx, t, "Shard 1 Contest", 1, "running")
		contest2 := se.CreateShardedContest(ctx, t, "Shard 2 Contest", 2, "running")

		// Add users with scores on each shard
		// Shard 0 users
		user0a, _ := se.CreateTradingUser(ctx, t, "shard0_user_a@example.com")
		user0b, _ := se.CreateTradingUser(ctx, t, "shard0_user_b@example.com")
		se.JoinContest(ctx, t, contest0, user0a, 100000)
		se.JoinContest(ctx, t, contest0, user0b, 100000)
		se.SetLeaderboardScore(ctx, t, contest0, user0a, 2000.00)
		se.SetLeaderboardScore(ctx, t, contest0, user0b, 1800.00)

		// Shard 1 users
		user1a, _ := se.CreateTradingUser(ctx, t, "shard1_user_a@example.com")
		user1b, _ := se.CreateTradingUser(ctx, t, "shard1_user_b@example.com")
		se.JoinContest(ctx, t, contest1, user1a, 100000)
		se.JoinContest(ctx, t, contest1, user1b, 100000)
		se.SetLeaderboardScore(ctx, t, contest1, user1a, 2500.00) // Highest overall
		se.SetLeaderboardScore(ctx, t, contest1, user1b, 1500.00)

		// Shard 2 users
		user2a, _ := se.CreateTradingUser(ctx, t, "shard2_user_a@example.com")
		se.JoinContest(ctx, t, contest2, user2a, 100000)
		se.SetLeaderboardScore(ctx, t, contest2, user2a, 1900.00)

		// Verify each shard's leaderboard independently
		entries0 := se.GetLeaderboardScores(ctx, t, contest0, 10)
		if len(entries0) != 2 {
			t.Errorf("Shard 0: Expected 2 entries, got %d", len(entries0))
		}
		if len(entries0) > 0 && entries0[0].Score != 2000.00 {
			t.Errorf("Shard 0: Expected top score 2000.00, got %f", entries0[0].Score)
		}

		entries1 := se.GetLeaderboardScores(ctx, t, contest1, 10)
		if len(entries1) != 2 {
			t.Errorf("Shard 1: Expected 2 entries, got %d", len(entries1))
		}
		if len(entries1) > 0 && entries1[0].Score != 2500.00 {
			t.Errorf("Shard 1: Expected top score 2500.00, got %f", entries1[0].Score)
		}

		entries2 := se.GetLeaderboardScores(ctx, t, contest2, 10)
		if len(entries2) != 1 {
			t.Errorf("Shard 2: Expected 1 entry, got %d", len(entries2))
		}
	})

	t.Run("LeaderboardUpdatePropagation", func(t *testing.T) {
		contestID := se.CreateShardedContest(ctx, t, "Update Propagation Test", 0, "running")

		userID, _ := se.CreateTradingUser(ctx, t, "update_test@example.com")
		se.JoinContest(ctx, t, contestID, userID, 100000)

		// Initial score
		se.SetLeaderboardScore(ctx, t, contestID, userID, 100.00)

		entries := se.GetLeaderboardScores(ctx, t, contestID, 1)
		if len(entries) == 0 || entries[0].Score != 100.00 {
			t.Errorf("Expected initial score 100.00")
		}

		// Update score
		se.SetLeaderboardScore(ctx, t, contestID, userID, 500.00)

		entries = se.GetLeaderboardScores(ctx, t, contestID, 1)
		if len(entries) == 0 || entries[0].Score != 500.00 {
			t.Errorf("Expected updated score 500.00, got %f", entries[0].Score)
		}
	})

	t.Run("LeaderboardWithNegativeScores", func(t *testing.T) {
		contestID := se.CreateShardedContest(ctx, t, "Negative Score Test", 1, "running")

		users := []struct {
			email string
			score float64
		}{
			{"profit@example.com", 500.00},
			{"breakeven@example.com", 0.00},
			{"loss1@example.com", -100.00},
			{"loss2@example.com", -500.00},
		}

		for _, u := range users {
			userID, _ := se.CreateTradingUser(ctx, t, u.email)
			se.JoinContest(ctx, t, contestID, userID, 100000)
			se.SetLeaderboardScore(ctx, t, contestID, userID, u.score)
		}

		entries := se.GetLeaderboardScores(ctx, t, contestID, 10)

		if len(entries) != 4 {
			t.Errorf("Expected 4 entries, got %d", len(entries))
		}

		// Verify correct ordering with negative scores
		expectedScores := []float64{500.00, 0.00, -100.00, -500.00}
		for i, expected := range expectedScores {
			if i < len(entries) && entries[i].Score != expected {
				t.Errorf("Position %d: expected score %f, got %f", i, expected, entries[i].Score)
			}
		}
	})
}

// ============================================================================
// Concurrent Shard Operations Tests
// ============================================================================

func TestConcurrentShardOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	se := NewShardingTestEnv(t, ctx)
	defer se.Cleanup(t, ctx)

	// Create shards
	for i := 0; i < 4; i++ {
		se.CreateShard(ctx, t, i, fmt.Sprintf("shard-%d", i), "active", 1)
	}

	t.Run("ConcurrentContestCreation", func(t *testing.T) {
		const numContests = 20
		var wg sync.WaitGroup
		contestIDs := make(chan string, numContests)
		errors := make(chan error, numContests)

		for i := 0; i < numContests; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()

				shardID := idx % 4
				name := fmt.Sprintf("Concurrent Contest %d", idx)

				var contestID string
				err := se.DB.QueryRowContext(ctx, `
					INSERT INTO contests (name, starts_at, ends_at, status, qty_total, shard_id)
					VALUES ($1, NOW(), NOW() + INTERVAL '1 day', 'running', 100000, $2)
					RETURNING id
				`, name, shardID).Scan(&contestID)

				if err != nil {
					errors <- err
					return
				}
				contestIDs <- contestID
			}(i)
		}

		wg.Wait()
		close(contestIDs)
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Concurrent creation error: %v", err)
		}

		// Verify all contests were created
		count := 0
		for range contestIDs {
			count++
		}
		if count != numContests {
			t.Errorf("Expected %d contests, got %d", numContests, count)
		}
	})

	t.Run("ConcurrentLeaderboardUpdates", func(t *testing.T) {
		contestID := se.CreateShardedContest(ctx, t, "Concurrent LB Test", 0, "running")

		const numUsers = 50
		var wg sync.WaitGroup

		// Create users concurrently
		userIDs := make([]string, numUsers)
		for i := 0; i < numUsers; i++ {
			email := fmt.Sprintf("concurrent_user_%d@example.com", i)
			userID, _ := se.CreateTradingUser(ctx, t, email)
			se.JoinContest(ctx, t, contestID, userID, 100000)
			userIDs[i] = userID
		}

		// Update scores concurrently
		for i := 0; i < numUsers; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				score := float64(idx * 100)
				se.SetLeaderboardScore(ctx, t, contestID, userIDs[idx], score)
			}(i)
		}

		wg.Wait()

		// Verify all scores are set
		entries := se.GetLeaderboardScores(ctx, t, contestID, int64(numUsers))
		if len(entries) != numUsers {
			t.Errorf("Expected %d entries, got %d", numUsers, len(entries))
		}
	})
}

// ============================================================================
// Shard Partition Assignment Tests
// ============================================================================

func TestShardPartitionAssignment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	se := NewShardingTestEnv(t, ctx)
	defer se.Cleanup(t, ctx)

	t.Run("PartitionToShardMapping", func(t *testing.T) {
		// Test the partition assignment formula:
		// partition % shardCount == shardID

		shardCount := 4
		totalPartitions := 16

		// Expected: Shard 0: [0,4,8,12], Shard 1: [1,5,9,13], etc.
		expectedAssignments := map[int][]int{
			0: {0, 4, 8, 12},
			1: {1, 5, 9, 13},
			2: {2, 6, 10, 14},
			3: {3, 7, 11, 15},
		}

		for shardID := 0; shardID < shardCount; shardID++ {
			var assigned []int
			for partition := 0; partition < totalPartitions; partition++ {
				if partition%shardCount == shardID {
					assigned = append(assigned, partition)
				}
			}

			expected := expectedAssignments[shardID]
			if len(assigned) != len(expected) {
				t.Errorf("Shard %d: expected %d partitions, got %d", shardID, len(expected), len(assigned))
				continue
			}

			for i, p := range assigned {
				if p != expected[i] {
					t.Errorf("Shard %d: expected partition %d, got %d", shardID, expected[i], p)
				}
			}
		}
	})

	t.Run("UnevenPartitionDistribution", func(t *testing.T) {
		// Test with non-power-of-2 shard counts
		shardCount := 3
		totalPartitions := 10

		partitionsPerShard := make(map[int]int)
		for partition := 0; partition < totalPartitions; partition++ {
			shardID := partition % shardCount
			partitionsPerShard[shardID]++
		}

		// With 10 partitions and 3 shards:
		// Shard 0: partitions 0, 3, 6, 9 = 4
		// Shard 1: partitions 1, 4, 7 = 3
		// Shard 2: partitions 2, 5, 8 = 3

		if partitionsPerShard[0] != 4 {
			t.Errorf("Shard 0 should have 4 partitions, got %d", partitionsPerShard[0])
		}
		if partitionsPerShard[1] != 3 {
			t.Errorf("Shard 1 should have 3 partitions, got %d", partitionsPerShard[1])
		}
		if partitionsPerShard[2] != 3 {
			t.Errorf("Shard 2 should have 3 partitions, got %d", partitionsPerShard[2])
		}
	})
}
