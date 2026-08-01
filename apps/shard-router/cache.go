package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/db"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	// Cache key prefixes
	shardAssignmentKeyPrefix = "shard:{assign}:"
	shardInfoKeyPrefix       = "shard:info:"
	shardListKey             = "shards:list"
)

// ShardCache provides caching for shard assignments using Redis.
type ShardCache struct {
	client *pkgredis.Client
	ttl    time.Duration
	logger *zap.Logger
}

// NewShardCache creates a new ShardCache instance.
func NewShardCache(client *pkgredis.Client, ttl time.Duration, logger *zap.Logger) *ShardCache {
	return &ShardCache{
		client: client,
		ttl:    ttl,
		logger: logger.With(zap.String("component", "shard-cache")),
	}
}

// GetAssignment retrieves a cached shard assignment for a contest ID.
func (c *ShardCache) GetAssignment(ctx context.Context, contestID string) (*ShardAssignment, error) {
	key := shardAssignmentKeyPrefix + contestID

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil // Cache miss
		}
		c.logger.Warn("failed to get shard assignment from cache",
			zap.String("contest_id", contestID),
			zap.Error(err),
		)
		return nil, err
	}

	var assignment ShardAssignment
	if err := json.Unmarshal(data, &assignment); err != nil {
		c.logger.Warn("failed to unmarshal cached shard assignment",
			zap.String("contest_id", contestID),
			zap.Error(err),
		)
		return nil, err
	}

	return &assignment, nil
}

// SetAssignment caches a shard assignment for a contest ID.
func (c *ShardCache) SetAssignment(ctx context.Context, contestID string, assignment *ShardAssignment) error {
	key := shardAssignmentKeyPrefix + contestID

	data, err := json.Marshal(assignment)
	if err != nil {
		return err
	}

	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		c.logger.Warn("failed to cache shard assignment",
			zap.String("contest_id", contestID),
			zap.Error(err),
		)
		return err
	}

	return nil
}

// InvalidateAssignment removes a cached shard assignment.
func (c *ShardCache) InvalidateAssignment(ctx context.Context, contestID string) error {
	key := shardAssignmentKeyPrefix + contestID
	return c.client.Del(ctx, key).Err()
}

// InvalidateAllAssignments removes all cached shard assignments.
// In Redis Cluster mode, SCAN only hits one node. Use ForEachMaster to
// scan all master nodes and collect keys from the entire cluster.
func (c *ShardCache) InvalidateAllAssignments(ctx context.Context) error {
	var keys []string

	if clusterClient, ok := c.client.Client().(*goredis.ClusterClient); ok {
		var mu sync.Mutex
		err := clusterClient.ForEachMaster(ctx, func(ctx context.Context, client *goredis.Client) error {
			iter := client.Scan(ctx, 0, shardAssignmentKeyPrefix+"*", 100).Iterator()
			for iter.Next(ctx) {
				mu.Lock()
				keys = append(keys, iter.Val())
				mu.Unlock()
			}
			return iter.Err()
		})
		if err != nil {
			return fmt.Errorf("cluster scan failed: %w", err)
		}
	} else {
		// Non-cluster mode: single SCAN works fine
		var cursor uint64
		for {
			var batch []string
			var err error
			batch, cursor, err = c.client.Scan(ctx, cursor, shardAssignmentKeyPrefix+"*", 100).Result()
			if err != nil {
				return err
			}
			keys = append(keys, batch...)
			if cursor == 0 {
				break
			}
		}
	}

	if len(keys) > 0 {
		// Delete in batches of 100 to avoid Redis protocol size limits
		const batchSize = 100
		for i := 0; i < len(keys); i += batchSize {
			end := i + batchSize
			if end > len(keys) {
				end = len(keys)
			}
			if err := c.client.Del(ctx, keys[i:end]...).Err(); err != nil {
				return fmt.Errorf("failed to delete keys batch [%d:%d]: %w", i, end, err)
			}
		}
		c.logger.Info("invalidated shard assignments",
			zap.Int("count", len(keys)),
		)
	}

	return nil
}

// GetShardInfo retrieves cached shard information.
func (c *ShardCache) GetShardInfo(ctx context.Context, shardID string) (*Shard, error) {
	key := shardInfoKeyPrefix + shardID

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil // Cache miss
		}
		return nil, err
	}

	var shard Shard
	if err := json.Unmarshal(data, &shard); err != nil {
		return nil, err
	}

	return &shard, nil
}

// SetShardInfo caches shard information.
func (c *ShardCache) SetShardInfo(ctx context.Context, shard *Shard) error {
	key := shardInfoKeyPrefix + shard.ID

	data, err := json.Marshal(shard)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, c.ttl).Err()
}

// InvalidateShardInfo removes cached shard information.
func (c *ShardCache) InvalidateShardInfo(ctx context.Context, shardID string) error {
	key := shardInfoKeyPrefix + shardID
	return c.client.Del(ctx, key).Err()
}

// GetShardList retrieves the cached list of all shards.
func (c *ShardCache) GetShardList(ctx context.Context) ([]*Shard, error) {
	data, err := c.client.Get(ctx, shardListKey).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return nil, nil // Cache miss
		}
		return nil, err
	}

	var shards []*Shard
	if err := json.Unmarshal(data, &shards); err != nil {
		return nil, err
	}

	return shards, nil
}

// SetShardList caches the list of all shards.
func (c *ShardCache) SetShardList(ctx context.Context, shards []*Shard) error {
	data, err := json.Marshal(shards)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, shardListKey, data, c.ttl).Err()
}

// InvalidateShardList removes the cached shard list.
func (c *ShardCache) InvalidateShardList(ctx context.Context) error {
	return c.client.Del(ctx, shardListKey).Err()
}

// HealthCheck performs a health check on the cache.
func (c *ShardCache) HealthCheck(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// CacheStats represents cache statistics.
type CacheStats struct {
	Mode    string `json:"mode"`
	Healthy bool   `json:"healthy"`
}

// Stats returns cache statistics.
func (c *ShardCache) Stats(ctx context.Context) *CacheStats {
	healthy := c.client.Ping(ctx).Err() == nil
	return &CacheStats{
		Mode:    string(c.client.Mode()),
		Healthy: healthy,
	}
}

// Close closes the cache client.
func (c *ShardCache) Close() error {
	return c.client.Close()
}

// CachedRouter wraps ShardRouter with caching and DB-pinned contest stickiness.
type CachedRouter struct {
	router *ShardRouter
	cache  *ShardCache
	db     *db.Pool
	logger *zap.Logger
}

// NewCachedRouter creates a new CachedRouter instance.
func NewCachedRouter(router *ShardRouter, cache *ShardCache, dbPool *db.Pool, logger *zap.Logger) *CachedRouter {
	return &CachedRouter{
		router: router,
		cache:  cache,
		db:     dbPool,
		logger: logger.With(zap.String("component", "cached-router")),
	}
}

// RouteTo returns the shard assignment for a contest, using cache if available.
// For active contests, it uses DB-pinned assignments to prevent mid-tournament
// re-routing when the hash ring changes.
func (cr *CachedRouter) RouteTo(ctx context.Context, contestID string) (*ShardAssignment, error) {
	// Try cache first
	assignment, err := cr.cache.GetAssignment(ctx, contestID)
	if err == nil && assignment != nil {
		cr.logger.Debug("cache hit for shard assignment",
			zap.String("contest_id", contestID),
			zap.String("shard_id", assignment.ShardID),
		)
		return assignment, nil
	}

	// Cache miss: use sticky routing (DB-pinned for active contests, hash ring for new)
	assignment, err = cr.router.RouteToSticky(ctx, cr.db.Replica(), contestID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	if cacheErr := cr.cache.SetAssignment(ctx, contestID, assignment); cacheErr != nil {
		cr.logger.Warn("failed to cache shard assignment",
			zap.String("contest_id", contestID),
			zap.Error(cacheErr),
		)
	}

	cr.logger.Debug("computed and cached shard assignment",
		zap.String("contest_id", contestID),
		zap.String("shard_id", assignment.ShardID),
	)

	return assignment, nil
}

// InvalidateContest invalidates the cache for a specific contest.
func (cr *CachedRouter) InvalidateContest(ctx context.Context, contestID string) error {
	return cr.cache.InvalidateAssignment(ctx, contestID)
}

// InvalidateAll invalidates all cached assignments.
func (cr *CachedRouter) InvalidateAll(ctx context.Context) error {
	if err := cr.cache.InvalidateAllAssignments(ctx); err != nil {
		return fmt.Errorf("failed to invalidate assignments: %w", err)
	}
	if err := cr.cache.InvalidateShardList(ctx); err != nil {
		return fmt.Errorf("failed to invalidate shard list: %w", err)
	}
	return nil
}
