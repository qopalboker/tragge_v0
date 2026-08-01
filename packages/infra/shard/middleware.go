// Package shard provides middleware for shard-aware request routing.
// It injects shard context (shard_id, shard_address) into requests based on contest_id,
// enabling BFF services to route traffic to the correct trading-engine shard.
package shard

import (
	"container/list"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// contextKey is a type for context keys to avoid collisions.
type contextKey string

const (
	// ShardIDKey is the context key for the shard ID.
	ShardIDKey contextKey = "shard_id"
	// ShardAddressKey is the context key for the shard address.
	ShardAddressKey contextKey = "shard_address"
	// ContestIDKey is the context key for the contest ID.
	ContestIDKey contextKey = "contest_id"
)

// ShardInfo represents information about a shard assignment.
type ShardInfo struct {
	ShardID   string `json:"shard_id"`
	Address   string `json:"shard_address"`
	ContestID string `json:"contest_id,omitempty"`
}

// Config holds configuration for the shard middleware.
type Config struct {
	// RouterAddr is the address of the shard-router service (e.g., "http://shard-router:8090")
	RouterAddr string
	// Timeout is the timeout for requests to the shard-router
	Timeout time.Duration
	// CacheTTL is how long to cache shard assignments (0 to disable caching)
	CacheTTL time.Duration
	// Logger is the logger to use
	Logger *zap.Logger
}

// DefaultConfig returns a default configuration.
func DefaultConfig() *Config {
	return &Config{
		RouterAddr: "http://shard-router:8090",
		Timeout:    2 * time.Second,
		CacheTTL:   30 * time.Second,
		Logger:     zap.NewNop(),
	}
}

// Middleware provides HTTP middleware for shard-aware request routing.
type Middleware struct {
	config     *Config
	httpClient *http.Client
	cache      *shardCache
	logger     *zap.Logger
}

// shardCache provides an LRU cache for shard assignments with TTL expiration.
// It uses a doubly-linked list + map for O(1) get, set, and eviction.
type shardCache struct {
	mu         sync.Mutex
	items      map[string]*list.Element
	order      *list.List // Front = most recently used, Back = least recently used
	ttl        time.Duration
	maxEntries int
	stopCh     chan struct{}
}

type cacheEntry struct {
	key       string // contestID, needed for map deletion during eviction
	info      *ShardInfo
	expiresAt time.Time
}

func newShardCache(ttl time.Duration, maxEntries ...int) *shardCache {
	max := 10000
	if len(maxEntries) > 0 && maxEntries[0] > 0 {
		max = maxEntries[0]
	}
	c := &shardCache{
		items:      make(map[string]*list.Element),
		order:      list.New(),
		ttl:        ttl,
		maxEntries: max,
		stopCh:     make(chan struct{}),
	}
	if ttl > 0 {
		go c.cleanupLoop(ttl)
	}
	return c
}

func (c *shardCache) get(contestID string) *ShardInfo {
	if c.ttl == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[contestID]
	if !ok {
		return nil
	}

	entry := el.Value.(*cacheEntry)
	if time.Now().After(entry.expiresAt) {
		// Expired: remove from both list and map
		c.order.Remove(el)
		delete(c.items, contestID)
		return nil
	}

	// Move to front (most recently used)
	c.order.MoveToFront(el)
	return entry.info
}

func (c *shardCache) set(contestID string, info *ShardInfo) {
	if c.ttl == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If key already exists, update and move to front
	if el, exists := c.items[contestID]; exists {
		c.order.MoveToFront(el)
		entry := el.Value.(*cacheEntry)
		entry.info = info
		entry.expiresAt = time.Now().Add(c.ttl)
		return
	}

	// Evict LRU entry if at capacity
	if len(c.items) >= c.maxEntries {
		c.evictLRU()
	}

	// Add new entry at front
	entry := &cacheEntry{
		key:       contestID,
		info:      info,
		expiresAt: time.Now().Add(c.ttl),
	}
	el := c.order.PushFront(entry)
	c.items[contestID] = el
}

// evictLRU removes the least recently used entry. Must be called with mu held.
func (c *shardCache) evictLRU() {
	back := c.order.Back()
	if back == nil {
		return
	}
	entry := back.Value.(*cacheEntry)
	c.order.Remove(back)
	delete(c.items, entry.key)
}

func (c *shardCache) invalidate(contestID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[contestID]; ok {
		c.order.Remove(el)
		delete(c.items, contestID)
	}
}

func (c *shardCache) invalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// cleanupLoop periodically evicts expired entries to prevent unbounded map growth.
func (c *shardCache) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.evictExpired()
		case <-c.stopCh:
			return
		}
	}
}

// evictExpired removes all entries that have passed their expiration time.
func (c *shardCache) evictExpired() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Walk the list from back (oldest access) to front
	for el := c.order.Back(); el != nil; {
		entry := el.Value.(*cacheEntry)
		prev := el.Prev()
		if now.After(entry.expiresAt) {
			c.order.Remove(el)
			delete(c.items, entry.key)
		}
		el = prev
	}
}

// stop signals the background cleanup goroutine to exit.
func (c *shardCache) stop() {
	close(c.stopCh)
}

// NewMiddleware creates a new shard middleware.
func NewMiddleware(config *Config) *Middleware {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	return &Middleware{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		cache:  newShardCache(config.CacheTTL, 10000),
		logger: config.Logger.With(zap.String("component", "shard-middleware")),
	}
}

// InjectShardContext returns a middleware that injects shard context into requests.
// It extracts the contest_id from URL parameters or query string, looks up the shard
// assignment from the shard-router, and injects shard_id and shard_address into the
// request context.
//
// IMPORTANT: This middleware gracefully degrades — if the shard-router is unreachable
// or returns an error, the request continues WITHOUT shard context. This is acceptable
// for read-only endpoints (e.g., /me, /balance, /orders/history).
//
// For trading-critical endpoints that MUST have shard context (e.g., order placement,
// position close, TP/SL modification), chain RequireShardContext after this middleware:
//
//	r.Use(shardMiddleware.InjectShardContext)
//	r.Use(shardMiddleware.RequireShardContext) // returns 503 if no shard context
func (m *Middleware) InjectShardContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to extract contest_id from URL params first, then query string
		contestID := chi.URLParam(r, "contestID")
		if contestID == "" {
			contestID = chi.URLParam(r, "contest_id")
		}
		if contestID == "" {
			contestID = r.URL.Query().Get("contest_id")
		}

		// If no contest_id, continue without shard context
		if contestID == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Get shard info (from cache or shard-router)
		shardInfo, err := m.getShardInfo(r.Context(), contestID)
		if err != nil {
			// Log at error level — for trading-critical endpoints, pair with RequireShardContext
			m.logger.Error("failed to get shard info, request continuing without shard context",
				zap.String("contest_id", contestID),
				zap.Error(err),
			)
			next.ServeHTTP(w, r)
			return
		}

		// Inject shard context
		ctx := r.Context()
		ctx = context.WithValue(ctx, ShardIDKey, shardInfo.ShardID)
		ctx = context.WithValue(ctx, ShardAddressKey, shardInfo.Address)
		ctx = context.WithValue(ctx, ContestIDKey, contestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireShardContext returns a middleware that requires shard context to be present.
// It returns a 503 Service Unavailable error if shard context is not available.
//
// Use this after InjectShardContext on endpoints where shard routing is critical
// (order placement, position close, TP/SL modification). Without this guard,
// InjectShardContext alone will silently allow requests through without shard context
// when the shard-router is unreachable.
func (m *Middleware) RequireShardContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetShardID(r.Context()) == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "shard context not available",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// getShardInfo retrieves shard information for a contest from cache or shard-router.
func (m *Middleware) getShardInfo(ctx context.Context, contestID string) (*ShardInfo, error) {
	// Check cache first
	if cached := m.cache.get(contestID); cached != nil {
		m.logger.Debug("shard info cache hit", zap.String("contest_id", contestID))
		return cached, nil
	}

	// Call shard-router
	url := fmt.Sprintf("%s/shards/%s", m.config.RouterAddr, contestID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call shard-router: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("contest not found: %s", contestID)
	}

	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, fmt.Errorf("no shards available")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("shard-router returned status %d", resp.StatusCode)
	}

	var shardInfo ShardInfo
	if err := json.NewDecoder(resp.Body).Decode(&shardInfo); err != nil {
		return nil, fmt.Errorf("failed to decode shard info: %w", err)
	}

	shardInfo.ContestID = contestID

	// Cache the result
	m.cache.set(contestID, &shardInfo)

	m.logger.Debug("shard info fetched",
		zap.String("contest_id", contestID),
		zap.String("shard_id", shardInfo.ShardID),
		zap.String("shard_address", shardInfo.Address),
	)

	return &shardInfo, nil
}

// GetShardInfo retrieves shard information for a contest.
// This is a public method for use outside of middleware context.
func (m *Middleware) GetShardInfo(ctx context.Context, contestID string) (*ShardInfo, error) {
	return m.getShardInfo(ctx, contestID)
}

// InvalidateCache invalidates the cache for a specific contest.
func (m *Middleware) InvalidateCache(contestID string) {
	m.cache.invalidate(contestID)
}

// InvalidateAllCache invalidates all cached entries.
func (m *Middleware) InvalidateAllCache() {
	m.cache.invalidateAll()
}

// Close stops the background cache cleanup goroutine.
// It should be called when the middleware is no longer needed.
func (m *Middleware) Close() {
	if m.cache != nil {
		m.cache.stop()
	}
}

// GetShardID extracts the shard ID from the context.
// Returns empty string if not available.
func GetShardID(ctx context.Context) string {
	if shardID, ok := ctx.Value(ShardIDKey).(string); ok {
		return shardID
	}
	return ""
}

// GetShardAddress extracts the shard address from the context.
// Returns empty string if not available.
func GetShardAddress(ctx context.Context) string {
	if addr, ok := ctx.Value(ShardAddressKey).(string); ok {
		return addr
	}
	return ""
}

// GetContestID extracts the contest ID from the context.
// Returns empty string if not available.
func GetContestID(ctx context.Context) string {
	if contestID, ok := ctx.Value(ContestIDKey).(string); ok {
		return contestID
	}
	return ""
}

// GetShardContext extracts the full shard context from the request context.
// Returns nil if shard context is not available.
func GetShardContext(ctx context.Context) *ShardInfo {
	shardID := GetShardID(ctx)
	if shardID == "" {
		return nil
	}

	return &ShardInfo{
		ShardID:   shardID,
		Address:   GetShardAddress(ctx),
		ContestID: GetContestID(ctx),
	}
}

// HasShardContext returns true if shard context is available in the request context.
func HasShardContext(ctx context.Context) bool {
	return GetShardID(ctx) != ""
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
