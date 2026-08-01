package wsregistry

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// mockRedisClient creates a test Redis client.
// In CI, use miniredis; for local testing, connects to localhost:6379.
func getTestRedisClient(t *testing.T) redis.UniversalClient {
	t.Helper()

	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use a separate DB for tests
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Redis not available for testing: %v", err)
	}

	// Clean up test DB
	client.FlushDB(ctx)

	return client
}

func TestRegistry_RegisterAndGetOwner(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// Register a connection
	err := registry.Register(ctx, "user-123", "contest-456")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Get owner should return our pod
	owner, err := registry.GetOwner(ctx, "user-123", "contest-456")
	if err != nil {
		t.Fatalf("GetOwner failed: %v", err)
	}
	if owner != "pod-1" {
		t.Errorf("Expected owner 'pod-1', got '%s'", owner)
	}

	// IsOwnedByMe should return true
	if !registry.IsOwnedByMe(ctx, "user-123", "contest-456") {
		t.Error("Expected IsOwnedByMe to return true")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// Register and then unregister
	registry.Register(ctx, "user-123", "contest-456")
	err := registry.Unregister(ctx, "user-123", "contest-456")
	if err != nil {
		t.Fatalf("Unregister failed: %v", err)
	}

	// Owner should be empty
	owner, err := registry.GetOwner(ctx, "user-123", "contest-456")
	if err != nil {
		t.Fatalf("GetOwner failed: %v", err)
	}
	if owner != "" {
		t.Errorf("Expected empty owner after unregister, got '%s'", owner)
	}
}

func TestRegistry_UnregisterOnlyOwner(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	// Create two registries for different pods
	registry1 := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	registry2 := New(Config{
		Redis:   client,
		PodName: "pod-2",
		TTL:     5 * time.Minute,
	})

	// pod-1 registers the connection
	registry1.Register(ctx, "user-123", "contest-456")

	// pod-2 tries to unregister it - should not delete
	registry2.Unregister(ctx, "user-123", "contest-456")

	// Owner should still be pod-1
	owner, _ := registry1.GetOwner(ctx, "user-123", "contest-456")
	if owner != "pod-1" {
		t.Errorf("Expected owner 'pod-1' after failed unregister by pod-2, got '%s'", owner)
	}
}

func TestRegistry_ConflictDetection(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry1 := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	registry2 := New(Config{
		Redis:   client,
		PodName: "pod-2",
		TTL:     5 * time.Minute,
	})

	// pod-1 registers
	registry1.Register(ctx, "user-123", "contest-456")

	// pod-2 registers same user/contest - should increment conflict
	registry2.Register(ctx, "user-123", "contest-456")

	// pod-2 should now own it (overwrites)
	owner, _ := registry2.GetOwner(ctx, "user-123", "contest-456")
	if owner != "pod-2" {
		t.Errorf("Expected owner 'pod-2' after overwrite, got '%s'", owner)
	}

	// Conflict count should be incremented
	metrics := registry2.GetMetrics().GetStats()
	if metrics["conflicts"] != 1 {
		t.Errorf("Expected 1 conflict, got %d", metrics["conflicts"])
	}
}

func TestRegistry_IsOwnedByMe_NoOwner(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// No registration - should return true (new connection allowed)
	if !registry.IsOwnedByMe(ctx, "user-123", "contest-456") {
		t.Error("Expected IsOwnedByMe to return true for non-existent connection")
	}
}

func TestRegistry_IsOwnedByMe_OtherPod(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry1 := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	registry2 := New(Config{
		Redis:   client,
		PodName: "pod-2",
		TTL:     5 * time.Minute,
	})

	// pod-1 registers
	registry1.Register(ctx, "user-123", "contest-456")

	// pod-2 checks ownership - should return false
	if registry2.IsOwnedByMe(ctx, "user-123", "contest-456") {
		t.Error("Expected IsOwnedByMe to return false for connection owned by other pod")
	}
}

func TestRegistry_Heartbeat(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// Register a connection
	registry.Register(ctx, "user-123", "contest-456")

	// Heartbeat should succeed
	err := registry.Heartbeat(ctx, "user-123", "contest-456")
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	// Check TTL was refreshed
	key := registry.key("user-123", "contest-456")
	ttl, err := client.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL check failed: %v", err)
	}
	if ttl < 4*time.Minute {
		t.Errorf("Expected TTL > 4 minutes, got %v", ttl)
	}
}

func TestRegistry_HeartbeatOtherPod(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry1 := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	registry2 := New(Config{
		Redis:   client,
		PodName: "pod-2",
		TTL:     5 * time.Minute,
	})

	// pod-1 registers
	registry1.Register(ctx, "user-123", "contest-456")

	// pod-2 tries heartbeat - should not error but should not refresh
	err := registry2.Heartbeat(ctx, "user-123", "contest-456")
	if err != nil {
		t.Fatalf("Heartbeat by other pod should not error: %v", err)
	}
}

func TestRegistry_GetAllMyConnections(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// Register multiple connections
	registry.Register(ctx, "user-1", "contest-1")
	registry.Register(ctx, "user-2", "contest-1")
	registry.Register(ctx, "user-1", "contest-2")

	connections := registry.GetAllMyConnections()
	if len(connections) != 3 {
		t.Errorf("Expected 3 connections, got %d", len(connections))
	}
}

func TestRegistry_GetMyConnectionCount(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	if registry.GetMyConnectionCount() != 0 {
		t.Error("Expected 0 connections initially")
	}

	registry.Register(ctx, "user-1", "contest-1")
	registry.Register(ctx, "user-2", "contest-1")

	if registry.GetMyConnectionCount() != 2 {
		t.Errorf("Expected 2 connections, got %d", registry.GetMyConnectionCount())
	}
}

func TestRegistry_CleanupAllMyConnections(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// Register multiple connections
	registry.Register(ctx, "user-1", "contest-1")
	registry.Register(ctx, "user-2", "contest-1")

	// Cleanup
	err := registry.CleanupAllMyConnections(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Local cache should be empty
	if registry.GetMyConnectionCount() != 0 {
		t.Error("Expected 0 connections after cleanup")
	}

	// Redis should also be cleaned
	owner, _ := registry.GetOwner(ctx, "user-1", "contest-1")
	if owner != "" {
		t.Error("Expected empty owner in Redis after cleanup")
	}
}

func TestRegistry_TryTakeOver_NoOwner(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// TryTakeOver with no owner should succeed
	success, err := registry.TryTakeOver(ctx, "user-123", "contest-456", false)
	if err != nil {
		t.Fatalf("TryTakeOver failed: %v", err)
	}
	if !success {
		t.Error("Expected TryTakeOver to succeed with no owner")
	}

	// Should now be registered
	owner, _ := registry.GetOwner(ctx, "user-123", "contest-456")
	if owner != "pod-1" {
		t.Errorf("Expected owner 'pod-1', got '%s'", owner)
	}
}

func TestRegistry_TryTakeOver_OtherOwner(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry1 := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	registry2 := New(Config{
		Redis:   client,
		PodName: "pod-2",
		TTL:     5 * time.Minute,
	})

	// pod-1 registers
	registry1.Register(ctx, "user-123", "contest-456")

	// pod-2 tries to take over without force - should fail
	success, err := registry2.TryTakeOver(ctx, "user-123", "contest-456", false)
	if err != nil {
		t.Fatalf("TryTakeOver failed: %v", err)
	}
	if success {
		t.Error("Expected TryTakeOver to fail without force flag")
	}

	// Owner should still be pod-1
	owner, _ := registry1.GetOwner(ctx, "user-123", "contest-456")
	if owner != "pod-1" {
		t.Errorf("Expected owner 'pod-1', got '%s'", owner)
	}
}

func TestRegistry_TryTakeOver_Force(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry1 := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	registry2 := New(Config{
		Redis:   client,
		PodName: "pod-2",
		TTL:     5 * time.Minute,
	})

	// pod-1 registers
	registry1.Register(ctx, "user-123", "contest-456")

	// pod-2 force takes over
	success, err := registry2.TryTakeOver(ctx, "user-123", "contest-456", true)
	if err != nil {
		t.Fatalf("TryTakeOver failed: %v", err)
	}
	if !success {
		t.Error("Expected TryTakeOver to succeed with force flag")
	}

	// Owner should now be pod-2
	owner, _ := registry2.GetOwner(ctx, "user-123", "contest-456")
	if owner != "pod-2" {
		t.Errorf("Expected owner 'pod-2', got '%s'", owner)
	}
}

func TestRegistry_CacheHitsMisses(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:    client,
		PodName:  "pod-1",
		TTL:      5 * time.Minute,
		CacheTTL: 5 * time.Minute, // Long cache TTL so entries stay fresh during test
	})

	// Register - puts in local cache
	registry.Register(ctx, "user-123", "contest-456")

	// First GetOwner - should be cache hit
	registry.GetOwner(ctx, "user-123", "contest-456")

	// Second GetOwner - should be cache hit
	registry.GetOwner(ctx, "user-123", "contest-456")

	// Check for non-existent - should be cache miss
	registry.GetOwner(ctx, "user-999", "contest-999")

	metrics := registry.GetMetrics().GetStats()
	if metrics["cache_hits"] != 2 {
		t.Errorf("Expected 2 cache hits, got %d", metrics["cache_hits"])
	}
	if metrics["cache_misses"] != 1 {
		t.Errorf("Expected 1 cache miss, got %d", metrics["cache_misses"])
	}
}

func TestRegistry_HeartbeatBatch(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// Register connections
	registry.Register(ctx, "user-1", "contest-1")
	registry.Register(ctx, "user-2", "contest-1")
	registry.Register(ctx, "user-3", "contest-2")

	// Batch heartbeat
	connections := []struct{ UserID, ContestID string }{
		{"user-1", "contest-1"},
		{"user-2", "contest-1"},
		{"user-3", "contest-2"},
	}

	err := registry.HeartbeatBatch(ctx, connections)
	if err != nil {
		t.Fatalf("HeartbeatBatch failed: %v", err)
	}
}

func TestRegistry_PodName(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	registry := New(Config{
		Redis:   client,
		PodName: "test-pod-name",
		TTL:     5 * time.Minute,
	})

	if registry.PodName() != "test-pod-name" {
		t.Errorf("Expected PodName 'test-pod-name', got '%s'", registry.PodName())
	}
}

func TestRegistry_DefaultTTL(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	// Create registry without explicit TTL
	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
	})

	// Default TTL should be 1 hour
	if registry.ttl != 1*time.Hour {
		t.Errorf("Expected default TTL of 1 hour, got %v", registry.ttl)
	}

	// Default CacheTTL should be 30 seconds
	if registry.cacheTTL != 30*time.Second {
		t.Errorf("Expected default CacheTTL of 30s, got %v", registry.cacheTTL)
	}
}

func TestRegistry_CleanupAllMyConnections_VerifiesRedisCleanup(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     1 * time.Hour, // Long TTL so keys won't expire during test
	})

	// Register multiple connections
	registry.Register(ctx, "user-1", "contest-1")
	registry.Register(ctx, "user-2", "contest-1")
	registry.Register(ctx, "user-3", "contest-2")

	// Verify keys exist in Redis before cleanup
	for _, tc := range []struct{ userID, contestID string }{
		{"user-1", "contest-1"},
		{"user-2", "contest-1"},
		{"user-3", "contest-2"},
	} {
		key := registry.key(tc.userID, tc.contestID)
		val, err := client.Get(ctx, key).Result()
		if err != nil {
			t.Fatalf("Expected key %s to exist before cleanup, got error: %v", key, err)
		}
		if val != "pod-1" {
			t.Fatalf("Expected key %s to be 'pod-1', got '%s'", key, val)
		}
	}

	// Cleanup
	err := registry.CleanupAllMyConnections(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Verify ALL keys are actually deleted from Redis
	for _, tc := range []struct{ userID, contestID string }{
		{"user-1", "contest-1"},
		{"user-2", "contest-1"},
		{"user-3", "contest-2"},
	} {
		key := registry.key(tc.userID, tc.contestID)
		_, err := client.Get(ctx, key).Result()
		if err != redis.Nil {
			t.Errorf("Expected key %s to be deleted from Redis after cleanup, got err: %v", key, err)
		}
	}
}

func TestRegistry_CleanupAllMyConnections_DoesNotDeleteOtherPodKeys(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry1 := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     1 * time.Hour,
	})

	registry2 := New(Config{
		Redis:   client,
		PodName: "pod-2",
		TTL:     1 * time.Hour,
	})

	// pod-1 registers a connection
	registry1.Register(ctx, "user-1", "contest-1")

	// pod-2 force-takes over the connection
	registry2.TryTakeOver(ctx, "user-1", "contest-1", true)

	// Verify Redis shows pod-2 as owner
	key := registry1.key("user-1", "contest-1")
	val, _ := client.Get(ctx, key).Result()
	if val != "pod-2" {
		t.Fatalf("Expected Redis key to show 'pod-2' after takeover, got '%s'", val)
	}

	// pod-1's local cache still has the key (stale), so cleanup will try to delete it.
	// But the Lua script should NOT delete because pod-2 now owns it.
	// Note: pod-1's local cache was cleared by the takeover's Register call on pod-2,
	// so we manually insert a stale entry to simulate a missed disconnect.
	registry1.localMu.Lock()
	registry1.local[key] = &ConnectionInfo{
		UserID:    "user-1",
		ContestID: "contest-1",
		PodName:   "pod-1",
	}
	registry1.localMu.Unlock()

	// pod-1 cleanup should not delete pod-2's key
	err := registry1.CleanupAllMyConnections(ctx)
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	// Redis key should still exist and be owned by pod-2
	val, err = client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("Expected key to still exist after pod-1 cleanup, got error: %v", err)
	}
	if val != "pod-2" {
		t.Errorf("Expected key owner to still be 'pod-2', got '%s'", val)
	}
}

func TestRegistry_HeartbeatBatch_SkipsNonOwnedKeys(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	// Register two connections
	registry.Register(ctx, "user-1", "contest-1")
	registry.Register(ctx, "user-2", "contest-1")

	// Simulate pod-2 taking over user-2's connection by directly setting in Redis
	key2 := registry.key("user-2", "contest-1")
	client.Set(ctx, key2, "pod-2", 5*time.Minute)

	// Set a short TTL on the taken-over key to detect if HeartbeatBatch extends it
	client.Expire(ctx, key2, 30*time.Second)

	// pod-1 does HeartbeatBatch for both connections
	connections := []struct{ UserID, ContestID string }{
		{"user-1", "contest-1"},
		{"user-2", "contest-1"},
	}
	err := registry.HeartbeatBatch(ctx, connections)
	if err != nil {
		t.Fatalf("HeartbeatBatch failed: %v", err)
	}

	// user-1's key should have been refreshed (TTL close to 5 minutes)
	key1 := registry.key("user-1", "contest-1")
	ttl1, _ := client.TTL(ctx, key1).Result()
	if ttl1 < 4*time.Minute {
		t.Errorf("Expected user-1 TTL > 4 minutes (refreshed), got %v", ttl1)
	}

	// user-2's key should NOT have been refreshed (TTL should still be ~30 seconds)
	ttl2, _ := client.TTL(ctx, key2).Result()
	if ttl2 > 1*time.Minute {
		t.Errorf("Expected user-2 TTL to stay short (~30s, not refreshed), got %v", ttl2)
	}
}

func TestRegistry_GetOwner_CacheExpiry(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry := New(Config{
		Redis:    client,
		PodName:  "pod-1",
		TTL:      5 * time.Minute,
		CacheTTL: 100 * time.Millisecond, // Short cache TTL for testing
	})

	// Register a connection (populates local cache)
	registry.Register(ctx, "user-1", "contest-1")

	// GetOwner should return from cache (pod-1)
	owner, err := registry.GetOwner(ctx, "user-1", "contest-1")
	if err != nil {
		t.Fatalf("GetOwner failed: %v", err)
	}
	if owner != "pod-1" {
		t.Errorf("Expected owner 'pod-1', got '%s'", owner)
	}

	// Directly overwrite in Redis to simulate another pod taking over
	key := registry.key("user-1", "contest-1")
	client.Set(ctx, key, "pod-2", 5*time.Minute)

	// GetOwner immediately should still return pod-1 from cache (not expired yet)
	owner, _ = registry.GetOwner(ctx, "user-1", "contest-1")
	if owner != "pod-1" {
		t.Errorf("Expected cached owner 'pod-1' before cache expiry, got '%s'", owner)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// GetOwner should now fall through to Redis and return pod-2
	owner, err = registry.GetOwner(ctx, "user-1", "contest-1")
	if err != nil {
		t.Fatalf("GetOwner after cache expiry failed: %v", err)
	}
	if owner != "pod-2" {
		t.Errorf("Expected owner 'pod-2' after cache expiry, got '%s'", owner)
	}

	// Stale entry should have been removed from local cache
	registry.localMu.RLock()
	_, exists := registry.local[key]
	registry.localMu.RUnlock()
	if exists {
		t.Error("Expected stale cache entry to be removed after ownership change")
	}
}

func TestRegistry_ConflictRetryForcesSET(t *testing.T) {
	client := getTestRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	registry1 := New(Config{
		Redis:   client,
		PodName: "pod-1",
		TTL:     5 * time.Minute,
	})

	registry2 := New(Config{
		Redis:   client,
		PodName: "pod-2",
		TTL:     5 * time.Minute,
	})

	// pod-1 registers
	registry1.Register(ctx, "user-1", "contest-1")

	// pod-2 registers the same connection (triggers conflict + force-set)
	err := registry2.Register(ctx, "user-1", "contest-1")
	if err != nil {
		t.Fatalf("Register with conflict failed: %v", err)
	}

	// Verify Redis actually shows pod-2 as owner (force-set worked)
	key := registry1.key("user-1", "contest-1")
	val, err := client.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("Failed to get key from Redis: %v", err)
	}
	if val != "pod-2" {
		t.Errorf("Expected Redis to show 'pod-2' after conflict retry, got '%s'", val)
	}
}
