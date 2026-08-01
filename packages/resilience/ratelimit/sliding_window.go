package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// SlidingWindowLimiter implements a sliding window rate limiter using Redis.
// It uses a sorted set to track request timestamps and a Lua script for
// atomic check-and-increment operations. This provides more accurate rate
// limiting than token bucket for per-minute limits.
type SlidingWindowLimiter struct {
	client      redis.UniversalClient
	config      Config
	metrics     *Metrics
	script      *redis.Script
	checkScript *redis.Script
}

// Lua script for atomic sliding window rate limiting.
// Uses a sorted set where:
// - Score = timestamp in milliseconds
// - Member = unique request ID (timestamp + random suffix)
// Returns: [allowed (0/1), remaining, oldest_timestamp]
const slidingWindowScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local count = tonumber(ARGV[4])

-- Remove expired entries (older than window)
local window_start = now - window
redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

-- Count current entries
local current = redis.call('ZCARD', key)

-- Check if we can add more
if current + count > limit then
    -- Get the oldest entry to calculate retry time
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local oldest_time = 0
    if #oldest > 0 then
        oldest_time = tonumber(oldest[2])
    end
    return {0, limit - current, oldest_time}
end

-- Add new entries
for i = 1, count do
    local member = now .. ':' .. i .. ':' .. math.random(1000000)
    redis.call('ZADD', key, now, member)
end

-- Set TTL on the key
redis.call('PEXPIRE', key, window)

return {1, limit - current - count, 0}
`

// Lua script for checking rate limit without consuming tokens.
const checkOnlyScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])

-- Remove expired entries
local window_start = now - window
redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

-- Count current entries
local current = redis.call('ZCARD', key)

-- Get oldest entry for retry calculation
local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
local oldest_time = 0
if #oldest > 0 then
    oldest_time = tonumber(oldest[2])
end

local allowed = 0
if current < limit then
    allowed = 1
end

return {allowed, limit - current, oldest_time}
`

// NewSlidingWindowLimiter creates a new Redis-backed sliding window rate limiter.
func NewSlidingWindowLimiter(client redis.UniversalClient, cfg Config) *SlidingWindowLimiter {
	cfg = cfg.WithDefaults()

	return &SlidingWindowLimiter{
		client:      client,
		config:      cfg,
		script:      redis.NewScript(slidingWindowScript),
		checkScript: redis.NewScript(checkOnlyScript),
	}
}

// NewSlidingWindowLimiterWithMetrics creates a limiter with Prometheus metrics.
func NewSlidingWindowLimiterWithMetrics(client redis.UniversalClient, cfg Config, metrics *Metrics) *SlidingWindowLimiter {
	limiter := NewSlidingWindowLimiter(client, cfg)
	limiter.metrics = metrics
	return limiter
}

// Allow checks if a single request is allowed for the given key.
func (sw *SlidingWindowLimiter) Allow(key string) bool {
	allowed, _ := sw.AllowCtx(context.Background(), key)
	return allowed
}

// AllowN checks if n requests are allowed for the given key.
func (sw *SlidingWindowLimiter) AllowN(key string, n int) bool {
	allowed, _ := sw.AllowNCtx(context.Background(), key, n)
	return allowed
}

// AllowCtx checks if a single request is allowed, with context support.
func (sw *SlidingWindowLimiter) AllowCtx(ctx context.Context, key string) (bool, error) {
	return sw.AllowNCtx(ctx, key, 1)
}

// AllowNCtx checks if n requests are allowed, with context support.
func (sw *SlidingWindowLimiter) AllowNCtx(ctx context.Context, key string, n int) (bool, error) {
	if key == "" || n <= 0 {
		return false, ErrInvalidKey
	}

	redisKey := sw.config.KeyPrefix + key
	now := time.Now().UnixMilli()
	windowMs := sw.config.Window.Milliseconds()

	result, err := sw.script.Run(ctx, sw.client, []string{redisKey},
		now, windowMs, sw.config.Rate, n).Slice()
	if err != nil {
		if sw.metrics != nil {
			sw.metrics.RecordError(sw.config.KeyPrefix, "redis_error")
		}
		return false, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}

	allowed := result[0].(int64) == 1

	if sw.metrics != nil {
		if allowed {
			sw.metrics.RecordAllowed(sw.config.KeyPrefix, key)
		} else {
			sw.metrics.RecordExceeded(sw.config.KeyPrefix, key)
		}
	}

	return allowed, nil
}

// Reset clears the rate limit state for the given key.
func (sw *SlidingWindowLimiter) Reset(key string) {
	_ = sw.ResetCtx(context.Background(), key)
}

// ResetCtx clears the rate limit state with context support.
func (sw *SlidingWindowLimiter) ResetCtx(ctx context.Context, key string) error {
	redisKey := sw.config.KeyPrefix + key
	return sw.client.Del(ctx, redisKey).Err()
}

// Remaining returns the number of remaining requests for the given key.
func (sw *SlidingWindowLimiter) Remaining(key string) int {
	remaining, _ := sw.RemainingCtx(context.Background(), key)
	return remaining
}

// RemainingCtx returns the remaining requests with context support.
func (sw *SlidingWindowLimiter) RemainingCtx(ctx context.Context, key string) (int, error) {
	info, err := sw.CheckCtx(ctx, key)
	if err != nil {
		return 0, err
	}
	return info.Remaining, nil
}

// RetryAfter returns the duration until the next request is allowed.
func (sw *SlidingWindowLimiter) RetryAfter(key string) time.Duration {
	retry, _ := sw.RetryAfterCtx(context.Background(), key)
	return retry
}

// RetryAfterCtx returns the retry duration with context support.
func (sw *SlidingWindowLimiter) RetryAfterCtx(ctx context.Context, key string) (time.Duration, error) {
	info, err := sw.CheckCtx(ctx, key)
	if err != nil {
		return 0, err
	}
	return info.RetryAfter, nil
}

// Check returns rate limit info without consuming tokens.
func (sw *SlidingWindowLimiter) Check(key string) LimitInfo {
	info, _ := sw.CheckCtx(context.Background(), key)
	return info
}

// CheckCtx returns rate limit info with context support.
func (sw *SlidingWindowLimiter) CheckCtx(ctx context.Context, key string) (LimitInfo, error) {
	redisKey := sw.config.KeyPrefix + key
	now := time.Now().UnixMilli()
	windowMs := sw.config.Window.Milliseconds()

	result, err := sw.checkScript.Run(ctx, sw.client, []string{redisKey},
		now, windowMs, sw.config.Rate).Slice()
	if err != nil {
		return LimitInfo{}, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}

	allowed := result[0].(int64) == 1
	remaining := int(result[1].(int64))
	oldestTime := result[2].(int64)

	var retryAfter time.Duration
	if !allowed && oldestTime > 0 {
		// Calculate when the oldest entry will expire
		expireTime := oldestTime + windowMs
		retryAfter = time.Duration(expireTime-now) * time.Millisecond
		if retryAfter < 0 {
			retryAfter = 0
		}
	}

	return LimitInfo{
		Allowed:    allowed,
		Limit:      sw.config.Rate,
		Remaining:  remaining,
		ResetAt:    time.Now().Add(sw.config.Window),
		RetryAfter: retryAfter,
	}, nil
}

// Close releases any resources held by the limiter.
func (sw *SlidingWindowLimiter) Close() error {
	// Client lifecycle is managed externally
	return nil
}

// FixedWindowLimiter implements a simpler fixed window rate limiter using Redis.
// This is more efficient but less accurate at window boundaries.
type FixedWindowLimiter struct {
	client  redis.UniversalClient
	config  Config
	metrics *Metrics
	script  *redis.Script
}

// Lua script for fixed window rate limiting.
const fixedWindowScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local count = tonumber(ARGV[3])

local raw = redis.call('GET', key)
local current = tonumber(raw or '0')
if current == nil then
    redis.call('DEL', key)
    current = 0
end

if current + count > limit then
    local ttl = redis.call('PTTL', key)
    return {0, limit - current, ttl}
end

current = redis.call('INCRBY', key, count)
if current == count then
    redis.call('PEXPIRE', key, window)
end

return {1, limit - current, 0}
`

// NewFixedWindowLimiter creates a new Redis-backed fixed window rate limiter.
func NewFixedWindowLimiter(client redis.UniversalClient, cfg Config) *FixedWindowLimiter {
	cfg = cfg.WithDefaults()

	return &FixedWindowLimiter{
		client: client,
		config: cfg,
		script: redis.NewScript(fixedWindowScript),
	}
}

// NewFixedWindowLimiterWithMetrics creates a limiter with Prometheus metrics.
func NewFixedWindowLimiterWithMetrics(client redis.UniversalClient, cfg Config, metrics *Metrics) *FixedWindowLimiter {
	limiter := NewFixedWindowLimiter(client, cfg)
	limiter.metrics = metrics
	return limiter
}

// Allow checks if a request is allowed.
func (fw *FixedWindowLimiter) Allow(key string) bool {
	return fw.AllowN(key, 1)
}

// AllowN checks if n requests are allowed.
func (fw *FixedWindowLimiter) AllowN(key string, n int) bool {
	allowed, _ := fw.AllowNCtx(context.Background(), key, n)
	return allowed
}

// AllowCtx checks if a request is allowed with context.
func (fw *FixedWindowLimiter) AllowCtx(ctx context.Context, key string) (bool, error) {
	return fw.AllowNCtx(ctx, key, 1)
}

// AllowNCtx checks if n requests are allowed with context.
func (fw *FixedWindowLimiter) AllowNCtx(ctx context.Context, key string, n int) (bool, error) {
	if key == "" || n <= 0 {
		return false, ErrInvalidKey
	}

	redisKey := fw.config.KeyPrefix + key
	windowMs := fw.config.Window.Milliseconds()

	result, err := fw.script.Run(ctx, fw.client, []string{redisKey},
		fw.config.Rate, windowMs, n).Slice()
	if err != nil {
		if fw.metrics != nil {
			fw.metrics.RecordError(fw.config.KeyPrefix, "redis_error")
		}
		return false, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}

	allowed := result[0].(int64) == 1

	if fw.metrics != nil {
		if allowed {
			fw.metrics.RecordAllowed(fw.config.KeyPrefix, key)
		} else {
			fw.metrics.RecordExceeded(fw.config.KeyPrefix, key)
		}
	}

	return allowed, nil
}

// Reset clears the rate limit for the key.
func (fw *FixedWindowLimiter) Reset(key string) {
	_ = fw.ResetCtx(context.Background(), key)
}

// ResetCtx clears the rate limit with context.
func (fw *FixedWindowLimiter) ResetCtx(ctx context.Context, key string) error {
	redisKey := fw.config.KeyPrefix + key
	return fw.client.Del(ctx, redisKey).Err()
}

// Remaining returns remaining requests.
func (fw *FixedWindowLimiter) Remaining(key string) int {
	remaining, _ := fw.RemainingCtx(context.Background(), key)
	return remaining
}

// RemainingCtx returns remaining requests with context.
func (fw *FixedWindowLimiter) RemainingCtx(ctx context.Context, key string) (int, error) {
	redisKey := fw.config.KeyPrefix + key
	val, err := fw.client.Get(ctx, redisKey).Result()
	if err == redis.Nil {
		return fw.config.Rate, nil
	}
	if err != nil {
		return 0, err
	}

	current, _ := strconv.Atoi(val)
	return fw.config.Rate - current, nil
}

// RetryAfter returns time until window resets.
func (fw *FixedWindowLimiter) RetryAfter(key string) time.Duration {
	retry, _ := fw.RetryAfterCtx(context.Background(), key)
	return retry
}

// RetryAfterCtx returns retry duration with context.
func (fw *FixedWindowLimiter) RetryAfterCtx(ctx context.Context, key string) (time.Duration, error) {
	redisKey := fw.config.KeyPrefix + key
	ttl, err := fw.client.PTTL(ctx, redisKey).Result()
	if err != nil {
		return 0, err
	}
	if ttl < 0 {
		return 0, nil
	}
	return ttl, nil
}

// Close releases resources.
func (fw *FixedWindowLimiter) Close() error {
	return nil
}

// SlidingWindowLogLimiter implements a precise sliding window using a log of timestamps.
// This provides the most accurate rate limiting but uses more memory.
type SlidingWindowLogLimiter struct {
	client       redis.UniversalClient
	config       Config
	metrics      *Metrics
	precision    time.Duration
	script       *redis.Script
	allowNScript *redis.Script
}

// Lua script for atomic AllowN in sliding window log.
// Checks capacity for all n entries before adding any, preventing partial consumption.
const slidingLogAllowNScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local precision = tonumber(ARGV[4])
local count = tonumber(ARGV[5])

-- Truncate timestamp to precision
local truncated = math.floor(now / precision) * precision

-- Remove old entries
local cutoff = truncated - window
redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)

-- Count requests in window
local current = redis.call('ZCARD', key)

if current + count > limit then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry = 0
    if #oldest > 0 then
        retry = tonumber(oldest[2]) + window - truncated
    end
    return {0, limit - current, retry}
end

-- Add all entries atomically
for i = 1, count do
    redis.call('ZADD', key, truncated, truncated .. ':' .. i .. ':' .. math.random(1000000))
end
redis.call('PEXPIRE', key, window + precision)

return {1, limit - current - count, 0}
`

// NewSlidingWindowLogLimiter creates a new log-based sliding window limiter.
func NewSlidingWindowLogLimiter(client redis.UniversalClient, cfg Config, precision time.Duration) *SlidingWindowLogLimiter {
	cfg = cfg.WithDefaults()
	if precision == 0 {
		precision = time.Millisecond
	}

	return &SlidingWindowLogLimiter{
		client:       client,
		config:       cfg,
		precision:    precision,
		script:       redis.NewScript(slidingLogScript),
		allowNScript: redis.NewScript(slidingLogAllowNScript),
	}
}

// Lua script for precise sliding window log.
const slidingLogScript = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local precision = tonumber(ARGV[4])

-- Truncate timestamp to precision
local truncated = math.floor(now / precision) * precision

-- Remove old entries
local cutoff = truncated - window
redis.call('ZREMRANGEBYSCORE', key, '-inf', cutoff)

-- Count requests in window
local count = redis.call('ZCARD', key)

if count >= limit then
    local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
    local retry = 0
    if #oldest > 0 then
        retry = tonumber(oldest[2]) + window - truncated
    end
    return {0, limit - count, retry}
end

-- Add request
redis.call('ZADD', key, truncated, truncated .. ':' .. math.random(1000000))
redis.call('PEXPIRE', key, window + precision)

return {1, limit - count - 1, 0}
`

// Allow checks if a request is allowed.
func (sl *SlidingWindowLogLimiter) Allow(key string) bool {
	allowed, _ := sl.AllowCtx(context.Background(), key)
	return allowed
}

// AllowN checks if n requests are allowed atomically.
// All n entries are added in a single Redis operation, preventing partial consumption.
func (sl *SlidingWindowLogLimiter) AllowN(key string, n int) bool {
	allowed, _ := sl.AllowNCtx(context.Background(), key, n)
	return allowed
}

// AllowNCtx checks if n requests are allowed atomically, with context support.
func (sl *SlidingWindowLogLimiter) AllowNCtx(ctx context.Context, key string, n int) (bool, error) {
	if key == "" {
		return false, ErrInvalidKey
	}
	if n <= 0 {
		return false, ErrInvalidKey
	}
	if n == 1 {
		return sl.AllowCtx(ctx, key)
	}

	redisKey := sl.config.KeyPrefix + key
	now := time.Now().UnixMilli()
	windowMs := sl.config.Window.Milliseconds()
	precisionMs := sl.precision.Milliseconds()

	result, err := sl.allowNScript.Run(ctx, sl.client, []string{redisKey},
		now, windowMs, sl.config.Rate, precisionMs, n).Slice()
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}

	allowed := result[0].(int64) == 1

	if sl.metrics != nil {
		if allowed {
			sl.metrics.RecordAllowed(sl.config.KeyPrefix, key)
		} else {
			sl.metrics.RecordExceeded(sl.config.KeyPrefix, key)
		}
	}

	return allowed, nil
}

// AllowCtx checks if a request is allowed with context.
func (sl *SlidingWindowLogLimiter) AllowCtx(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, ErrInvalidKey
	}

	redisKey := sl.config.KeyPrefix + key
	now := time.Now().UnixMilli()
	windowMs := sl.config.Window.Milliseconds()
	precisionMs := sl.precision.Milliseconds()

	result, err := sl.script.Run(ctx, sl.client, []string{redisKey},
		now, windowMs, sl.config.Rate, precisionMs).Slice()
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrRedisUnavailable, err)
	}

	allowed := result[0].(int64) == 1

	if sl.metrics != nil {
		if allowed {
			sl.metrics.RecordAllowed(sl.config.KeyPrefix, key)
		} else {
			sl.metrics.RecordExceeded(sl.config.KeyPrefix, key)
		}
	}

	return allowed, nil
}

// Reset clears the rate limit.
func (sl *SlidingWindowLogLimiter) Reset(key string) {
	_ = sl.client.Del(context.Background(), sl.config.KeyPrefix+key).Err()
}

// Remaining returns remaining requests.
func (sl *SlidingWindowLogLimiter) Remaining(key string) int {
	redisKey := sl.config.KeyPrefix + key
	ctx := context.Background()

	// Clean up old entries first
	now := time.Now().UnixMilli()
	cutoff := now - sl.config.Window.Milliseconds()
	sl.client.ZRemRangeByScore(ctx, redisKey, "-inf", strconv.FormatInt(cutoff, 10))

	count, err := sl.client.ZCard(ctx, redisKey).Result()
	if err != nil {
		return 0
	}

	return sl.config.Rate - int(count)
}

// RetryAfter returns time until rate limit resets.
func (sl *SlidingWindowLogLimiter) RetryAfter(key string) time.Duration {
	redisKey := sl.config.KeyPrefix + key
	ctx := context.Background()

	// Get oldest entry
	result, err := sl.client.ZRangeWithScores(ctx, redisKey, 0, 0).Result()
	if err != nil || len(result) == 0 {
		return 0
	}

	oldest := int64(result[0].Score)
	now := time.Now().UnixMilli()
	expiry := oldest + sl.config.Window.Milliseconds()

	if expiry <= now {
		return 0
	}

	return time.Duration(expiry-now) * time.Millisecond
}

// Close releases resources.
func (sl *SlidingWindowLogLimiter) Close() error {
	return nil
}
