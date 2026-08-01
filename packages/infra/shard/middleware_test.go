package shard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestShardCache(t *testing.T) {
	t.Run("cache hit and miss", func(t *testing.T) {
		cache := newShardCache(1 * time.Minute)

		// Cache miss
		info := cache.get("contest-1")
		if info != nil {
			t.Error("expected cache miss, got hit")
		}

		// Set cache
		expected := &ShardInfo{
			ShardID:   "shard-1",
			Address:   "shard-1.example.com:8085",
			ContestID: "contest-1",
		}
		cache.set("contest-1", expected)

		// Cache hit
		info = cache.get("contest-1")
		if info == nil {
			t.Fatal("expected cache hit, got miss")
		}
		if info.ShardID != expected.ShardID {
			t.Errorf("expected shard_id %s, got %s", expected.ShardID, info.ShardID)
		}
	})

	t.Run("cache expiration", func(t *testing.T) {
		cache := newShardCache(10 * time.Millisecond)

		cache.set("contest-1", &ShardInfo{ShardID: "shard-1"})

		// Immediate hit
		if cache.get("contest-1") == nil {
			t.Error("expected cache hit immediately after set")
		}

		// Wait for expiration
		time.Sleep(15 * time.Millisecond)

		// Should be expired
		if cache.get("contest-1") != nil {
			t.Error("expected cache miss after expiration")
		}
	})

	t.Run("cache disabled with zero TTL", func(t *testing.T) {
		cache := newShardCache(0)

		cache.set("contest-1", &ShardInfo{ShardID: "shard-1"})

		// Should never cache
		if cache.get("contest-1") != nil {
			t.Error("expected no caching with zero TTL")
		}
	})

	t.Run("invalidate single", func(t *testing.T) {
		cache := newShardCache(1 * time.Minute)

		cache.set("contest-1", &ShardInfo{ShardID: "shard-1"})
		cache.set("contest-2", &ShardInfo{ShardID: "shard-2"})

		cache.invalidate("contest-1")

		if cache.get("contest-1") != nil {
			t.Error("expected contest-1 to be invalidated")
		}
		if cache.get("contest-2") == nil {
			t.Error("expected contest-2 to still be cached")
		}
	})

	t.Run("invalidate all", func(t *testing.T) {
		cache := newShardCache(1 * time.Minute)

		cache.set("contest-1", &ShardInfo{ShardID: "shard-1"})
		cache.set("contest-2", &ShardInfo{ShardID: "shard-2"})

		cache.invalidateAll()

		if cache.get("contest-1") != nil || cache.get("contest-2") != nil {
			t.Error("expected all entries to be invalidated")
		}
	})
}

func TestShardCacheCleanup(t *testing.T) {
	t.Run("expired entries are cleaned up", func(t *testing.T) {
		cache := newShardCache(50 * time.Millisecond)
		defer cache.stop()

		cache.set("contest-1", &ShardInfo{ShardID: "shard-1"})
		cache.set("contest-2", &ShardInfo{ShardID: "shard-2"})

		// Verify entries exist in the map
		cache.mu.Lock()
		if len(cache.items) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(cache.items))
		}
		cache.mu.Unlock()

		// Wait for entries to expire and cleanup to run
		time.Sleep(120 * time.Millisecond)

		// Expired entries should have been removed from the map
		cache.mu.Lock()
		remaining := len(cache.items)
		cache.mu.Unlock()

		if remaining != 0 {
			t.Errorf("expected 0 entries after cleanup, got %d", remaining)
		}
	})

	t.Run("non-expired entries survive cleanup", func(t *testing.T) {
		cache := newShardCache(500 * time.Millisecond)
		defer cache.stop()

		cache.set("contest-1", &ShardInfo{ShardID: "shard-1"})

		// Trigger manual eviction
		cache.evictExpired()

		if cache.get("contest-1") == nil {
			t.Error("expected entry to survive cleanup since it has not expired")
		}
	})

	t.Run("no cleanup goroutine when TTL is zero", func(t *testing.T) {
		cache := newShardCache(0)
		// With TTL=0, stopCh is created but no goroutine is started.
		// Calling stop() should not panic.
		cache.stop()
	})

	t.Run("stop is safe to call", func(t *testing.T) {
		cache := newShardCache(100 * time.Millisecond)
		cache.stop()
		// After stop, the goroutine should exit cleanly.
		time.Sleep(10 * time.Millisecond)
	})
}

func TestMiddlewareClose(t *testing.T) {
	t.Run("close stops cache cleanup", func(t *testing.T) {
		config := &Config{
			RouterAddr: "http://localhost:9999",
			Timeout:    1 * time.Second,
			CacheTTL:   100 * time.Millisecond,
		}
		m := NewMiddleware(config)
		m.Close()
	})

	t.Run("close is safe with zero TTL", func(t *testing.T) {
		config := &Config{
			RouterAddr: "http://localhost:9999",
			Timeout:    1 * time.Second,
			CacheTTL:   0,
		}
		m := NewMiddleware(config)
		m.Close()
	})
}

func TestContextHelpers(t *testing.T) {
	t.Run("get shard ID", func(t *testing.T) {
		ctx := context.Background()

		// Empty context
		if GetShardID(ctx) != "" {
			t.Error("expected empty shard ID from empty context")
		}

		// With shard ID
		ctx = context.WithValue(ctx, ShardIDKey, "shard-1")
		if GetShardID(ctx) != "shard-1" {
			t.Error("expected shard-1")
		}
	})

	t.Run("get shard address", func(t *testing.T) {
		ctx := context.Background()

		if GetShardAddress(ctx) != "" {
			t.Error("expected empty address from empty context")
		}

		ctx = context.WithValue(ctx, ShardAddressKey, "shard-1.example.com:8085")
		if GetShardAddress(ctx) != "shard-1.example.com:8085" {
			t.Error("expected shard-1.example.com:8085")
		}
	})

	t.Run("get contest ID", func(t *testing.T) {
		ctx := context.Background()

		if GetContestID(ctx) != "" {
			t.Error("expected empty contest ID from empty context")
		}

		ctx = context.WithValue(ctx, ContestIDKey, "contest-1")
		if GetContestID(ctx) != "contest-1" {
			t.Error("expected contest-1")
		}
	})

	t.Run("get shard context", func(t *testing.T) {
		ctx := context.Background()

		// Empty context
		if GetShardContext(ctx) != nil {
			t.Error("expected nil from empty context")
		}

		// Full context
		ctx = context.WithValue(ctx, ShardIDKey, "shard-1")
		ctx = context.WithValue(ctx, ShardAddressKey, "shard-1.example.com:8085")
		ctx = context.WithValue(ctx, ContestIDKey, "contest-1")

		info := GetShardContext(ctx)
		if info == nil {
			t.Fatal("expected non-nil shard context")
		}
		if info.ShardID != "shard-1" {
			t.Errorf("expected shard-1, got %s", info.ShardID)
		}
		if info.Address != "shard-1.example.com:8085" {
			t.Errorf("expected shard-1.example.com:8085, got %s", info.Address)
		}
		if info.ContestID != "contest-1" {
			t.Errorf("expected contest-1, got %s", info.ContestID)
		}
	})

	t.Run("has shard context", func(t *testing.T) {
		ctx := context.Background()

		if HasShardContext(ctx) {
			t.Error("expected false for empty context")
		}

		ctx = context.WithValue(ctx, ShardIDKey, "shard-1")
		if !HasShardContext(ctx) {
			t.Error("expected true when shard ID is set")
		}
	})
}

func TestMiddlewareInjectShardContext(t *testing.T) {
	// Create a mock shard-router server
	mockRouter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contestID := chi.URLParam(r, "contestID")
		if contestID == "" {
			// Parse from path manually for test server
			path := r.URL.Path
			if len(path) > len("/shards/") {
				contestID = path[len("/shards/"):]
			}
		}

		if contestID == "not-found" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}

		if contestID == "no-shards" {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"error": "no shards available"})
			return
		}

		response := ShardInfo{
			ShardID: "shard-1",
			Address: "trading-engine-0.trading-engine-headless:8085",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockRouter.Close()

	config := &Config{
		RouterAddr: mockRouter.URL,
		Timeout:    2 * time.Second,
		CacheTTL:   1 * time.Minute,
	}
	middleware := NewMiddleware(config)

	t.Run("injects shard context from query param", func(t *testing.T) {
		var capturedShardID string
		var capturedShardAddr string

		handler := middleware.InjectShardContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedShardID = GetShardID(r.Context())
			capturedShardAddr = GetShardAddress(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test?contest_id=contest-123", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if capturedShardID != "shard-1" {
			t.Errorf("expected shard-1, got %s", capturedShardID)
		}
		if capturedShardAddr != "trading-engine-0.trading-engine-headless:8085" {
			t.Errorf("expected trading-engine-0.trading-engine-headless:8085, got %s", capturedShardAddr)
		}
	})

	t.Run("continues without shard context when no contest_id", func(t *testing.T) {
		var capturedShardID string

		handler := middleware.InjectShardContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedShardID = GetShardID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if capturedShardID != "" {
			t.Errorf("expected empty shard ID, got %s", capturedShardID)
		}
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("continues without shard context on router error", func(t *testing.T) {
		// Use a different config pointing to invalid address
		errorConfig := &Config{
			RouterAddr: "http://localhost:1", // Invalid address
			Timeout:    100 * time.Millisecond,
			CacheTTL:   0,
		}
		errorMiddleware := NewMiddleware(errorConfig)

		var called bool
		handler := errorMiddleware.InjectShardContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test?contest_id=contest-123", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if !called {
			t.Error("expected handler to be called even on router error")
		}
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})
}

func TestMiddlewareRequireShardContext(t *testing.T) {
	middleware := NewMiddleware(DefaultConfig())

	t.Run("allows request with shard context", func(t *testing.T) {
		var called bool
		handler := middleware.RequireShardContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		ctx := context.WithValue(req.Context(), ShardIDKey, "shard-1")
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if !called {
			t.Error("expected handler to be called")
		}
		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("rejects request without shard context", func(t *testing.T) {
		var called bool
		handler := middleware.RequireShardContext(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if called {
			t.Error("expected handler NOT to be called")
		}
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", rr.Code)
		}
	})
}
