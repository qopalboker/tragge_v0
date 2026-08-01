// Package wsregistry provides WebSocket connection registry for session affinity.
// It tracks which pod owns which user's WebSocket connection using Redis,
// enabling proper sticky sessions during scaling events.
package wsregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// DisconnectMessage is published via Redis Pub/Sub to notify a pod to close
// a specific user's WebSocket connection (e.g., on cross-pod takeover).
type DisconnectMessage struct {
	UserID    string `json:"user_id"`
	ContestID string `json:"contest_id"`
	Reason    string `json:"reason"`
}

// disconnectChannel returns the Redis Pub/Sub channel name for a pod.
func disconnectChannel(podName string) string {
	return "ws:disconnect:" + podName
}

// ConnectionInfo holds metadata about a WebSocket connection.
type ConnectionInfo struct {
	UserID      string    `json:"user_id"`
	ContestID   string    `json:"contest_id"`
	PodName     string    `json:"pod_name"`
	ConnectedAt time.Time `json:"connected_at"`
	cachedAt    time.Time // when this entry was last validated (unexported, not serialized)
}

// Registry tracks which pod owns which user's WebSocket connection.
// It uses Redis for distributed state and maintains a local cache for fast lookups.
type Registry struct {
	redis    redis.UniversalClient
	podName  string
	ttl      time.Duration
	cacheTTL time.Duration
	logger   *zap.Logger

	// Local cache for fast lookups
	local   map[string]*ConnectionInfo
	localMu sync.RWMutex

	// Metrics
	metrics *Metrics
}

// Metrics tracks registry operations.
type Metrics struct {
	Registrations   int64
	Unregistrations int64
	Conflicts       int64
	CacheHits       int64
	CacheMisses     int64
	mu              sync.RWMutex
}

// GetStats returns a copy of the current metrics.
func (m *Metrics) GetStats() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return map[string]int64{
		"registrations":   m.Registrations,
		"unregistrations": m.Unregistrations,
		"conflicts":       m.Conflicts,
		"cache_hits":      m.CacheHits,
		"cache_misses":    m.CacheMisses,
	}
}

// Config holds configuration for the Registry.
type Config struct {
	// Redis client for distributed state
	Redis redis.UniversalClient
	// PodName identifies this pod instance
	PodName string
	// TTL for connection registrations (default: 1 hour)
	TTL time.Duration
	// CacheTTL controls how long local cache entries are considered fresh
	// before re-validating against Redis (default: 30 seconds)
	CacheTTL time.Duration
	// Logger for structured logging (optional)
	Logger *zap.Logger
}

// New creates a new Registry instance.
func New(cfg Config) *Registry {
	if cfg.TTL == 0 {
		cfg.TTL = 1 * time.Hour
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	return &Registry{
		redis:    cfg.Redis,
		podName:  cfg.PodName,
		ttl:      cfg.TTL,
		cacheTTL: cfg.CacheTTL,
		logger:   cfg.Logger,
		local:    make(map[string]*ConnectionInfo),
		metrics:  &Metrics{},
	}
}

// key generates the Redis key for a user's connection.
// Format: ws:conn:{userID}:{contestID}
func (r *Registry) key(userID, contestID string) string {
	return fmt.Sprintf("ws:conn:%s:%s", userID, contestID)
}

// registerScript atomically checks and sets a connection registration.
// If a different pod owns the key, it returns that pod name for conflict handling.
// Otherwise it sets the key and returns empty string.
var registerScript = redis.NewScript(`
	local key = KEYS[1]
	local new_pod = ARGV[1]
	local ttl_ms = tonumber(ARGV[2])
	local current = redis.call("GET", key)
	if current and current ~= "" and current ~= new_pod then
		return current
	end
	redis.call("SET", key, new_pod, "PX", ttl_ms)
	return ""
`)

// unregisterScript atomically checks ownership and deletes a connection key.
// Only deletes if this pod is the current owner; returns 1 if deleted, 0 otherwise.
var unregisterScript = redis.NewScript(`
	if redis.call("get", KEYS[1]) == ARGV[1] then
		return redis.call("del", KEYS[1])
	end
	return 0
`)

// Register registers a user's connection to this pod.
// Returns an error if the registration fails.
func (r *Registry) Register(ctx context.Context, userID, contestID string) error {
	key := r.key(userID, contestID)
	ttlMs := r.ttl.Milliseconds()

	// Atomic check-and-set via Lua script
	result, err := registerScript.Run(ctx, r.redis, []string{key}, r.podName, ttlMs).Text()
	if err != nil {
		return fmt.Errorf("register connection: %w", err)
	}

	// If result is non-empty, another pod owns the connection
	if result != "" {
		r.metrics.mu.Lock()
		r.metrics.Conflicts++
		r.metrics.mu.Unlock()
		r.logger.Warn("Connection conflict detected, signaling old pod to disconnect",
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.String("existing_pod", result),
			zap.String("new_pod", r.podName),
		)
		// Signal the old pod to close the stale connection
		if pubErr := r.PublishDisconnect(ctx, result, userID, contestID, "takeover"); pubErr != nil {
			r.logger.Warn("Failed to publish disconnect to old pod",
				zap.String("target_pod", result),
				zap.Error(pubErr))
		}
		// Force registration now that we've signaled the old pod.
		// We use Set directly instead of registerScript because the old pod
		// may not have processed the disconnect yet, and registerScript would
		// detect the same conflict and return without setting.
		err = r.redis.Set(ctx, key, r.podName, r.ttl).Err()
		if err != nil {
			return fmt.Errorf("register connection after conflict: %w", err)
		}
	}

	// Update local cache
	now := time.Now()
	connInfo := &ConnectionInfo{
		UserID:      userID,
		ContestID:   contestID,
		PodName:     r.podName,
		ConnectedAt: now,
		cachedAt:    now,
	}

	r.localMu.Lock()
	r.local[key] = connInfo
	r.localMu.Unlock()

	r.metrics.mu.Lock()
	r.metrics.Registrations++
	r.metrics.mu.Unlock()

	r.logger.Debug("Connection registered",
		zap.String("user_id", userID),
		zap.String("contest_id", contestID),
		zap.String("pod", r.podName),
	)

	return nil
}

// Unregister removes a user's connection registration.
// Only removes if this pod is the current owner.
func (r *Registry) Unregister(ctx context.Context, userID, contestID string) error {
	key := r.key(userID, contestID)

	// Only delete if we own it (atomic check and delete using Lua script)
	deleted, err := unregisterScript.Run(ctx, r.redis, []string{key}, r.podName).Int()
	if err != nil {
		r.logger.Warn("Failed to unregister connection from Redis",
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.Error(err),
		)
	}

	// Remove from local cache regardless
	r.localMu.Lock()
	delete(r.local, key)
	r.localMu.Unlock()

	if deleted > 0 {
		r.metrics.mu.Lock()
		r.metrics.Unregistrations++
		r.metrics.mu.Unlock()

		r.logger.Debug("Connection unregistered",
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.String("pod", r.podName),
		)
	}

	return nil
}

// GetOwner returns the pod that owns the user's connection.
// Returns empty string if no owner is registered.
// Uses local cache with TTL-based freshness to avoid stale entries.
func (r *Registry) GetOwner(ctx context.Context, userID, contestID string) (string, error) {
	key := r.key(userID, contestID)

	// Check local cache first (with freshness check)
	r.localMu.RLock()
	if info, ok := r.local[key]; ok && time.Since(info.cachedAt) < r.cacheTTL {
		r.localMu.RUnlock()
		r.metrics.mu.Lock()
		r.metrics.CacheHits++
		r.metrics.mu.Unlock()
		return info.PodName, nil
	}
	r.localMu.RUnlock()

	r.metrics.mu.Lock()
	r.metrics.CacheMisses++
	r.metrics.mu.Unlock()

	// Check Redis
	pod, err := r.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		// Key gone from Redis — remove stale local cache entry
		r.localMu.Lock()
		delete(r.local, key)
		r.localMu.Unlock()
		return "", nil // No owner
	}
	if err != nil {
		return "", fmt.Errorf("get owner: %w", err)
	}

	// Update local cache if this pod still owns it
	if pod == r.podName {
		r.localMu.Lock()
		if info, ok := r.local[key]; ok {
			info.cachedAt = time.Now()
		}
		r.localMu.Unlock()
	} else {
		// Another pod took over — remove stale entry from local cache
		r.localMu.Lock()
		delete(r.local, key)
		r.localMu.Unlock()
	}

	return pod, nil
}

// GetConnectionInfo returns full connection info for a user.
// Uses local cache with TTL-based freshness to avoid stale entries.
func (r *Registry) GetConnectionInfo(ctx context.Context, userID, contestID string) (*ConnectionInfo, error) {
	key := r.key(userID, contestID)

	// Check local cache first (with freshness check)
	r.localMu.RLock()
	if info, ok := r.local[key]; ok && time.Since(info.cachedAt) < r.cacheTTL {
		r.localMu.RUnlock()
		return info, nil
	}
	r.localMu.RUnlock()

	// Check Redis - we only store pod name there
	pod, err := r.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		r.localMu.Lock()
		delete(r.local, key)
		r.localMu.Unlock()
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get connection info: %w", err)
	}

	return &ConnectionInfo{
		UserID:    userID,
		ContestID: contestID,
		PodName:   pod,
	}, nil
}

// IsOwnedByMe checks if this pod owns the connection or if there's no owner.
// Returns true if:
// - No connection is registered (new connection)
// - This pod owns the connection
func (r *Registry) IsOwnedByMe(ctx context.Context, userID, contestID string) bool {
	owner, err := r.GetOwner(ctx, userID, contestID)
	if err != nil {
		// On error, assume we can take ownership (fail-open for availability)
		r.logger.Warn("Failed to check ownership, allowing connection",
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.Error(err),
		)
		return true
	}
	return owner == "" || owner == r.podName
}

// Heartbeat refreshes the TTL for a connection.
func (r *Registry) Heartbeat(ctx context.Context, userID, contestID string) error {
	key := r.key(userID, contestID)

	// Only refresh if we own it
	currentPod, err := r.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return nil // Connection doesn't exist
		}
		return fmt.Errorf("heartbeat check: %w", err)
	}

	if currentPod != r.podName {
		return nil // We don't own this connection
	}

	return r.redis.Expire(ctx, key, r.ttl).Err()
}

// HeartbeatBatch refreshes TTLs for multiple connections efficiently using pipeline.
// Only refreshes keys owned by this pod (atomic ownership check per key).
func (r *Registry) HeartbeatBatch(ctx context.Context, connections []struct{ UserID, ContestID string }) error {
	if len(connections) == 0 {
		return nil
	}

	ttlSeconds := int(r.ttl.Seconds())
	pipe := r.redis.Pipeline()
	for _, conn := range connections {
		key := r.key(conn.UserID, conn.ContestID)
		pipe.Eval(ctx, `
			if redis.call("get", KEYS[1]) == ARGV[1] then
				return redis.call("expire", KEYS[1], ARGV[2])
			end
			return 0
		`, []string{key}, r.podName, ttlSeconds)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("batch heartbeat: %w", err)
	}

	return nil
}

// GetAllMyConnections returns all connections owned by this pod from local cache.
func (r *Registry) GetAllMyConnections() []*ConnectionInfo {
	r.localMu.RLock()
	defer r.localMu.RUnlock()

	connections := make([]*ConnectionInfo, 0, len(r.local))
	for _, info := range r.local {
		connections = append(connections, info)
	}
	return connections
}

// GetMyConnectionCount returns the number of connections owned by this pod.
func (r *Registry) GetMyConnectionCount() int {
	r.localMu.RLock()
	defer r.localMu.RUnlock()
	return len(r.local)
}

// CleanupAllMyConnections removes all connections owned by this pod.
// Used during graceful shutdown.
func (r *Registry) CleanupAllMyConnections(ctx context.Context) error {
	r.localMu.Lock()
	keys := make([]string, 0, len(r.local))
	for key := range r.local {
		keys = append(keys, key)
	}
	r.local = make(map[string]*ConnectionInfo)
	r.localMu.Unlock()

	if len(keys) == 0 {
		return nil
	}

	// Delete all keys in Redis using pipeline (atomic ownership check per key)
	pipe := r.redis.Pipeline()
	for _, key := range keys {
		pipe.Eval(ctx, `
			if redis.call("get", KEYS[1]) == ARGV[1] then
				return redis.call("del", KEYS[1])
			end
			return 0
		`, []string{key}, r.podName)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("cleanup connections: %w", err)
	}

	r.logger.Info("Cleaned up all connections on shutdown",
		zap.Int("count", len(keys)),
		zap.String("pod", r.podName),
	)

	return nil
}

// TryTakeOver attempts to take over a connection from another pod.
// This is used when a user reconnects and their previous pod may be unavailable.
// Returns true if takeover was successful.
func (r *Registry) TryTakeOver(ctx context.Context, userID, contestID string, forceTakeOver bool) (bool, error) {
	key := r.key(userID, contestID)

	// Check current owner
	currentPod, err := r.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		// No owner, we can take it
		return true, r.Register(ctx, userID, contestID)
	}
	if err != nil {
		return false, fmt.Errorf("check owner for takeover: %w", err)
	}

	// If we already own it, nothing to do
	if currentPod == r.podName {
		return true, nil
	}

	// If force takeover is enabled, take over regardless
	if forceTakeOver {
		r.logger.Info("Force taking over connection",
			zap.String("user_id", userID),
			zap.String("contest_id", contestID),
			zap.String("previous_pod", currentPod),
		)
		return true, r.Register(ctx, userID, contestID)
	}

	// Connection is owned by another pod
	return false, nil
}

// GetMetrics returns the current metrics.
func (r *Registry) GetMetrics() *Metrics {
	return r.metrics
}

// PodName returns the name of this pod.
func (r *Registry) PodName() string {
	return r.podName
}

// PublishDisconnect publishes a disconnect request to the pod that owns the connection.
// This signals the target pod to close the user's stale WebSocket connection.
func (r *Registry) PublishDisconnect(ctx context.Context, targetPod, userID, contestID, reason string) error {
	if targetPod == r.podName {
		return nil // Don't signal ourselves
	}
	msg := DisconnectMessage{
		UserID:    userID,
		ContestID: contestID,
		Reason:    reason,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal disconnect message: %w", err)
	}
	return r.redis.Publish(ctx, disconnectChannel(targetPod), data).Err()
}

// SubscribeDisconnects returns a channel that receives disconnect messages for this pod.
// The caller is responsible for calling Close() on the returned *redis.PubSub when done.
func (r *Registry) SubscribeDisconnects(ctx context.Context) (<-chan DisconnectMessage, *redis.PubSub, error) {
	pubsub := r.redis.Subscribe(ctx, disconnectChannel(r.podName))
	// Verify subscription is active
	_, err := pubsub.Receive(ctx)
	if err != nil {
		pubsub.Close()
		return nil, nil, fmt.Errorf("subscribe disconnect channel: %w", err)
	}

	msgCh := make(chan DisconnectMessage, 64)
	go func() {
		defer close(msgCh)
		defer func() {
			if rv := recover(); rv != nil {
				r.logger.Error("disconnect subscriber panicked",
					zap.Any("panic", rv))
			}
		}()
		ch := pubsub.Channel()
		for msg := range ch {
			var dm DisconnectMessage
			if err := json.Unmarshal([]byte(msg.Payload), &dm); err != nil {
				r.logger.Warn("Failed to unmarshal disconnect message",
					zap.Error(err))
				continue
			}
			select {
			case msgCh <- dm:
			default:
				r.logger.Warn("Disconnect message channel full, dropping",
					zap.String("user_id", dm.UserID),
					zap.String("contest_id", dm.ContestID))
			}
		}
	}()

	return msgCh, pubsub, nil
}
