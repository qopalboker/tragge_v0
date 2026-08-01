// Package redis provides high-availability Redis client utilities for the tragge platform.
// It supports three modes:
// - Standalone: Single Redis instance (default/development)
// - Sentinel: Redis Sentinel for automatic failover
// - Cluster: Redis Cluster for horizontal scaling
package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Mode represents the Redis deployment mode
type Mode string

const (
	// ModeStandalone is a single Redis instance
	ModeStandalone Mode = "standalone"
	// ModeSentinel uses Redis Sentinel for high availability
	ModeSentinel Mode = "sentinel"
	// ModeCluster uses Redis Cluster for horizontal scaling
	ModeCluster Mode = "cluster"
)

// Config holds the Redis client configuration
type Config struct {
	// Mode determines which client type to create (standalone, sentinel, cluster)
	Mode Mode

	// Standalone configuration
	Addr     string // e.g., "localhost:6379"
	Password string
	DB       int // Database number (only for standalone/sentinel)

	// Sentinel configuration
	SentinelAddrs    []string // e.g., ["sentinel-1:26379", "sentinel-2:26379", "sentinel-3:26379"]
	SentinelMaster   string   // Master name, e.g., "mymaster"
	SentinelPassword string   // Sentinel password (if different from Redis password)

	// Cluster configuration
	ClusterAddrs []string // e.g., ["node-1:6379", "node-2:6379", ...]

	// Common options
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	PoolSize     int
	MinIdleConns int

	// Connection health options (high concurrency)
	PoolTimeout      time.Duration // Time to wait for connection from pool
	ConnMaxIdleTime  time.Duration // Max idle time before closing connection
	ConnMaxLifetime  time.Duration // Max lifetime of a connection
	MaxRetries       int           // Max retries for failed commands
	MaxRetryBackoff  time.Duration // Max backoff between retries
	HealthCheckFreq  time.Duration // Frequency of health checks (0 = disabled)

	// TLS configuration (optional)
	TLSConfig *tls.Config
}

// DefaultConfig returns a Config with sensible defaults for development
func DefaultConfig() Config {
	return Config{
		Mode:            ModeStandalone,
		Addr:            "localhost:6379",
		DB:              0,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		PoolSize:        10,
		MinIdleConns:    2,
		PoolTimeout:     4 * time.Second,
		ConnMaxIdleTime: 5 * time.Minute,
		ConnMaxLifetime: 0, // No limit in dev
		MaxRetries:      3,
		MaxRetryBackoff: 512 * time.Millisecond,
		HealthCheckFreq: 0, // Disabled in dev
	}
}

// HighConcurrencyConfig returns a Config optimized for 1000+ concurrent users.
// This configuration is designed for production environments with heavy load.
func HighConcurrencyConfig() Config {
	return Config{
		Mode:            ModeStandalone,
		Addr:            "localhost:6379",
		DB:              0,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		PoolSize:        100,                   // Increased from 10 for high concurrency
		MinIdleConns:    20,                    // Pre-allocate 20% of pool
		PoolTimeout:     4 * time.Second,       // Reasonable wait for connection
		ConnMaxIdleTime: 3 * time.Minute,       // Recycle idle connections
		ConnMaxLifetime: 10 * time.Minute,      // Prevent stale connections
		MaxRetries:      3,                     // Retry failed commands
		MaxRetryBackoff: 512 * time.Millisecond,
		HealthCheckFreq: 30 * time.Second,      // Enable health checks
	}
}

// ConfigFromEnv creates a Config from environment variable values.
// Expected environment variables:
//   - REDIS_MODE: "standalone", "sentinel", or "cluster"
//   - REDIS_ADDR: address for standalone mode
//   - REDIS_PASSWORD: Redis password
//   - REDIS_DB: database number (standalone/sentinel only)
//   - REDIS_SENTINEL_ADDRS: comma-separated sentinel addresses
//   - REDIS_SENTINEL_MASTER: sentinel master name
//   - REDIS_SENTINEL_PASSWORD: sentinel password
//   - REDIS_CLUSTER_ADDRS: comma-separated cluster node addresses
//   - REDIS_HIGH_CONCURRENCY: set to "true" to use high-concurrency defaults
//   - REDIS_POOL_SIZE: pool size (default: 10, high-concurrency: 100)
//   - REDIS_MIN_IDLE_CONNS: minimum idle connections (default: 2, high-concurrency: 20)
//   - REDIS_HEALTH_CHECK_FREQ_SECONDS: health check frequency in seconds (0 = disabled)
func ConfigFromEnv(getenv func(string) string) Config {
	// Start with appropriate defaults
	var cfg Config
	if getenv("REDIS_HIGH_CONCURRENCY") == "true" {
		cfg = HighConcurrencyConfig()
	} else {
		cfg = DefaultConfig()
	}

	if mode := getenv("REDIS_MODE"); mode != "" {
		cfg.Mode = Mode(mode)
	}

	if addr := getenv("REDIS_ADDR"); addr != "" {
		cfg.Addr = addr
	}

	if password := getenv("REDIS_PASSWORD"); password != "" {
		cfg.Password = password
	} else if passwordFile := getenv("REDIS_PASSWORD_FILE"); passwordFile != "" {
		if data, err := os.ReadFile(passwordFile); err == nil {
			cfg.Password = strings.TrimSpace(string(data))
		}
	}

	if db := getenv("REDIS_DB"); db != "" {
		// Parse DB number, default to 0 on error
		var dbNum int
		if _, err := fmt.Sscanf(db, "%d", &dbNum); err == nil {
			cfg.DB = dbNum
		}
	}

	// Sentinel configuration
	if addrs := getenv("REDIS_SENTINEL_ADDRS"); addrs != "" {
		cfg.SentinelAddrs = strings.Split(addrs, ",")
		if cfg.Mode == ModeStandalone {
			cfg.Mode = ModeSentinel
		}
	}

	if master := getenv("REDIS_SENTINEL_MASTER"); master != "" {
		cfg.SentinelMaster = master
	}

	if sentinelPwd := getenv("REDIS_SENTINEL_PASSWORD"); sentinelPwd != "" {
		cfg.SentinelPassword = sentinelPwd
	}

	// Cluster configuration
	if addrs := getenv("REDIS_CLUSTER_ADDRS"); addrs != "" {
		cfg.ClusterAddrs = strings.Split(addrs, ",")
		if cfg.Mode == ModeStandalone {
			cfg.Mode = ModeCluster
		}
	}

	// Pool configuration overrides
	if v := getenv("REDIS_POOL_SIZE"); v != "" {
		var poolSize int
		if _, err := fmt.Sscanf(v, "%d", &poolSize); err == nil && poolSize > 0 {
			cfg.PoolSize = poolSize
		}
	}

	if v := getenv("REDIS_MIN_IDLE_CONNS"); v != "" {
		var minIdle int
		if _, err := fmt.Sscanf(v, "%d", &minIdle); err == nil && minIdle >= 0 {
			cfg.MinIdleConns = minIdle
		}
	}

	if v := getenv("REDIS_HEALTH_CHECK_FREQ_SECONDS"); v != "" {
		var seconds int
		if _, err := fmt.Sscanf(v, "%d", &seconds); err == nil && seconds >= 0 {
			cfg.HealthCheckFreq = time.Duration(seconds) * time.Second
		}
	}

	if v := getenv("REDIS_POOL_TIMEOUT_SECONDS"); v != "" {
		var seconds int
		if _, err := fmt.Sscanf(v, "%d", &seconds); err == nil && seconds > 0 {
			cfg.PoolTimeout = time.Duration(seconds) * time.Second
		}
	}

	return cfg
}

// Client wraps redis.UniversalClient to provide a unified interface
// for standalone, sentinel, and cluster modes
type Client struct {
	redis.UniversalClient
	mode   Mode
	config Config
}

// NewClient creates a new Redis client based on the provided configuration
func NewClient(cfg Config) (*Client, error) {
	var client redis.UniversalClient

	switch cfg.Mode {
	case ModeStandalone:
		client = redis.NewClient(&redis.Options{
			Addr:            cfg.Addr,
			Password:        cfg.Password,
			DB:              cfg.DB,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			TLSConfig:       cfg.TLSConfig,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
			ConnMaxLifetime: cfg.ConnMaxLifetime,
			MaxRetries:      cfg.MaxRetries,
			MaxRetryBackoff: cfg.MaxRetryBackoff,
		})

	case ModeSentinel:
		if len(cfg.SentinelAddrs) == 0 {
			return nil, errors.New("sentinel addresses required for sentinel mode")
		}
		if cfg.SentinelMaster == "" {
			return nil, errors.New("sentinel master name required for sentinel mode")
		}

		client = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       cfg.SentinelMaster,
			SentinelAddrs:    cfg.SentinelAddrs,
			SentinelPassword: cfg.SentinelPassword,
			Password:         cfg.Password,
			DB:               cfg.DB,
			DialTimeout:      cfg.DialTimeout,
			ReadTimeout:      cfg.ReadTimeout,
			WriteTimeout:     cfg.WriteTimeout,
			PoolSize:         cfg.PoolSize,
			MinIdleConns:     cfg.MinIdleConns,
			TLSConfig:        cfg.TLSConfig,
			PoolTimeout:      cfg.PoolTimeout,
			ConnMaxIdleTime:  cfg.ConnMaxIdleTime,
			ConnMaxLifetime:  cfg.ConnMaxLifetime,
			MaxRetries:       cfg.MaxRetries,
			MaxRetryBackoff:  cfg.MaxRetryBackoff,
		})

	case ModeCluster:
		if len(cfg.ClusterAddrs) == 0 {
			return nil, errors.New("cluster addresses required for cluster mode")
		}

		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:           cfg.ClusterAddrs,
			Password:        cfg.Password,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			TLSConfig:       cfg.TLSConfig,
			PoolTimeout:     cfg.PoolTimeout,
			ConnMaxIdleTime: cfg.ConnMaxIdleTime,
			ConnMaxLifetime: cfg.ConnMaxLifetime,
			MaxRetries:      cfg.MaxRetries,
			MaxRetryBackoff: cfg.MaxRetryBackoff,

			// Cluster-specific options
			ReadOnly:       false, // Read from master only
			RouteByLatency: true,  // Route to lowest latency node
			RouteRandomly:  false, // Don't route randomly
			MaxRedirects:   8,     // Max MOVED/ASK redirects
		})

	default:
		return nil, fmt.Errorf("unsupported redis mode: %s", cfg.Mode)
	}

	return &Client{
		UniversalClient: client,
		mode:            cfg.Mode,
		config:          cfg,
	}, nil
}

// Ping checks connectivity to Redis with a timeout
func (c *Client) PingCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.config.DialTimeout)
	defer cancel()

	return c.UniversalClient.Ping(ctx).Err()
}

// Mode returns the current Redis mode
func (c *Client) Mode() Mode {
	return c.mode
}

// IsCluster returns true if the client is connected to a Redis Cluster
func (c *Client) IsCluster() bool {
	return c.mode == ModeCluster
}

// IsSentinel returns true if the client is connected via Sentinel
func (c *Client) IsSentinel() bool {
	return c.mode == ModeSentinel
}

// HealthCheck performs a comprehensive health check
func (c *Client) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	status := &HealthStatus{
		Mode:    c.mode,
		Healthy: false,
	}

	// Basic ping
	start := time.Now()
	if err := c.PingCheck(ctx); err != nil {
		status.Error = err.Error()
		return status, err
	}
	status.Latency = time.Since(start)

	// Mode-specific checks
	switch c.mode {
	case ModeCluster:
		clusterClient, ok := c.UniversalClient.(*redis.ClusterClient)
		if ok {
			// Check cluster state
			result := clusterClient.ClusterInfo(ctx)
			if result.Err() != nil {
				status.Error = result.Err().Error()
				return status, result.Err()
			}
			info := result.Val()
			if strings.Contains(info, "cluster_state:ok") {
				status.Healthy = true
				status.ClusterState = "ok"
			} else {
				status.ClusterState = "fail"
				status.Error = "cluster state is not ok"
			}
		}

	case ModeSentinel:
		// Verify the master is writable by performing a SET + DEL probe.
		// During failover, PING may succeed against a demoted replica,
		// but writes will fail with READONLY errors.
		healthKey := "__tragge:health_check"
		if err := c.UniversalClient.Set(ctx, healthKey, "1", 5*time.Second).Err(); err != nil {
			status.Error = fmt.Sprintf("sentinel write probe failed: %v", err)
			return status, err
		}
		// Clean up the probe key (best-effort, TTL ensures cleanup even if DEL fails)
		_ = c.UniversalClient.Del(ctx, healthKey)
		status.Healthy = true

	case ModeStandalone:
		_, err := c.UniversalClient.Info(ctx, "server", "memory").Result()
		if err != nil {
			status.Error = fmt.Sprintf("standalone info check failed: %v", err)
			return status, err
		}
		status.Healthy = true
	}

	return status, nil
}

// HealthStatus represents the health of the Redis connection
type HealthStatus struct {
	Mode         Mode          `json:"mode"`
	Healthy      bool          `json:"healthy"`
	Latency      time.Duration `json:"latency"`
	ClusterState string        `json:"cluster_state,omitempty"`
	Error        string        `json:"error,omitempty"`
}

// Close closes the Redis connection
func (c *Client) Close() error {
	return c.UniversalClient.Close()
}

// MustNewClient creates a new Redis client and panics on error
func MustNewClient(cfg Config) *Client {
	client, err := NewClient(cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to create redis client: %v", err))
	}
	return client
}

// NewClientFromEnv creates a new Redis client from environment variables
func NewClientFromEnv(getenv func(string) string) (*Client, error) {
	cfg := ConfigFromEnv(getenv)
	return NewClient(cfg)
}

// Client returns the underlying redis client for direct access
func (c *Client) Client() redis.UniversalClient {
        return c.UniversalClient
}
