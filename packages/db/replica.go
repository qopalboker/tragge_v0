// Package db provides database utilities including read/write splitting
// for PostgreSQL primary-replica configurations.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Common errors
var (
	ErrNoPrimaryAvailable  = errors.New("no primary database available")
	ErrNoReplicaAvailable  = errors.New("no replica database available")
	ErrReplicationLagHigh  = errors.New("replication lag too high")
	ErrPoolClosed          = errors.New("connection pool is closed")
)

// Config holds the configuration for database pools.
type Config struct {
	// PrimaryDSN is the connection string for the primary (read-write) database.
	PrimaryDSN string

	// ReplicaDSNs are connection strings for replica (read-only) databases.
	// Multiple replicas will be load-balanced using round-robin.
	ReplicaDSNs []string

	// MaxOpenConns is the maximum number of open connections per pool.
	MaxOpenConns int

	// MaxIdleConns is the maximum number of idle connections per pool.
	MaxIdleConns int

	// ConnMaxLifetime is the maximum lifetime of a connection.
	ConnMaxLifetime time.Duration

	// ConnMaxIdleTime is the maximum idle time for a connection.
	ConnMaxIdleTime time.Duration

	// MaxReplicationLag is the maximum acceptable replication lag in seconds.
	// If a replica exceeds this lag, queries will fall back to primary.
	MaxReplicationLag time.Duration

	// LagCheckInterval is how often to check replica lag.
	LagCheckInterval time.Duration

	// RetryOnLag enables automatic retry on primary when replica has high lag.
	RetryOnLag bool
}

// DefaultConfig returns a configuration with sensible defaults for development.
func DefaultConfig() Config {
	return Config{
		MaxOpenConns:      25,
		MaxIdleConns:      5,
		ConnMaxLifetime:   5 * time.Minute,
		ConnMaxIdleTime:   1 * time.Minute,
		MaxReplicationLag: 10 * time.Second,
		LagCheckInterval:  5 * time.Second,
		RetryOnLag:        true,
	}
}

// HighConcurrencyConfig returns a configuration optimized for 1000+ concurrent users.
// This configuration is designed for production environments with heavy load.
// Make sure PostgreSQL max_connections is set appropriately (recommended: 200+).
func HighConcurrencyConfig() Config {
	return Config{
		MaxOpenConns:      100,                   // Increased from 25 for high concurrency
		MaxIdleConns:      25,                    // 25% of max open for connection reuse
		ConnMaxLifetime:   3 * time.Minute,       // Reduced for better load distribution
		ConnMaxIdleTime:   30 * time.Second,      // Recycle idle connections faster
		MaxReplicationLag: 10 * time.Second,
		LagCheckInterval:  5 * time.Second,
		RetryOnLag:        true,
	}
}

// ConfigFromEnv creates a configuration from environment variables.
// Supported variables:
//   - DB_MAX_OPEN_CONNS: Maximum open connections (default: 25, high-concurrency: 100)
//   - DB_MAX_IDLE_CONNS: Maximum idle connections (default: 5, high-concurrency: 25)
//   - DB_CONN_MAX_LIFETIME_SECONDS: Connection lifetime in seconds (default: 300)
//   - DB_CONN_MAX_IDLE_TIME_SECONDS: Idle time in seconds (default: 60)
//   - DB_HIGH_CONCURRENCY: Set to "true" to use high-concurrency defaults
func ConfigFromEnv(getenv func(string) string) Config {
	// Start with appropriate defaults
	var cfg Config
	if getenv("DB_HIGH_CONCURRENCY") == "true" {
		cfg = HighConcurrencyConfig()
	} else {
		cfg = DefaultConfig()
	}

	// Override with specific environment variables
	if v := getenv("DB_MAX_OPEN_CONNS"); v != "" {
		if n, err := parseIntEnv(v); err == nil && n > 0 {
			cfg.MaxOpenConns = n
		}
	}

	if v := getenv("DB_MAX_IDLE_CONNS"); v != "" {
		if n, err := parseIntEnv(v); err == nil && n > 0 {
			cfg.MaxIdleConns = n
		}
	}

	if v := getenv("DB_CONN_MAX_LIFETIME_SECONDS"); v != "" {
		if n, err := parseIntEnv(v); err == nil && n > 0 {
			cfg.ConnMaxLifetime = time.Duration(n) * time.Second
		}
	}

	if v := getenv("DB_CONN_MAX_IDLE_TIME_SECONDS"); v != "" {
		if n, err := parseIntEnv(v); err == nil && n > 0 {
			cfg.ConnMaxIdleTime = time.Duration(n) * time.Second
		}
	}

	if v := getenv("DB_MAX_REPLICATION_LAG_SECONDS"); v != "" {
		if n, err := parseIntEnv(v); err == nil && n > 0 {
			cfg.MaxReplicationLag = time.Duration(n) * time.Second
		}
	}

	return cfg
}

// parseIntEnv parses an integer from a string environment variable.
func parseIntEnv(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// Pool manages primary and replica database connections with automatic
// read/write splitting and replication lag monitoring.
type Pool struct {
	config Config

	primary  *sql.DB
	replicas []*replicaConn

	replicaIndex atomic.Uint64
	closed       atomic.Bool

	mu         sync.RWMutex
	lagChecker *time.Ticker
	stopLag    chan struct{}
	lagWg      sync.WaitGroup
}

// replicaConn wraps a replica connection with health status.
type replicaConn struct {
	db      *sql.DB
	dsn     string
	lag     atomic.Int64 // lag in milliseconds
	healthy atomic.Bool
}

// NewPool creates a new database pool with primary and replica connections.
func NewPool(ctx context.Context, cfg Config) (*Pool, error) {
	if cfg.PrimaryDSN == "" {
		return nil, fmt.Errorf("primary DSN is required")
	}

	// Apply defaults
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = DefaultConfig().MaxOpenConns
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = DefaultConfig().MaxIdleConns
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = DefaultConfig().ConnMaxLifetime
	}
	if cfg.MaxReplicationLag == 0 {
		cfg.MaxReplicationLag = DefaultConfig().MaxReplicationLag
	}
	if cfg.LagCheckInterval == 0 {
		cfg.LagCheckInterval = DefaultConfig().LagCheckInterval
	}

	pool := &Pool{
		config:  cfg,
		stopLag: make(chan struct{}),
	}

	// Connect to primary
	primary, err := openDB(ctx, cfg.PrimaryDSN, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to primary: %w", err)
	}
	pool.primary = primary

	// Connect to replicas
	for _, dsn := range cfg.ReplicaDSNs {
		replica, err := openDB(ctx, dsn, cfg)
		if err != nil {
			// Log warning but continue - replica is optional
			continue
		}

		rc := &replicaConn{
			db:  replica,
			dsn: dsn,
		}
		rc.healthy.Store(true)
		pool.replicas = append(pool.replicas, rc)
	}

	// Start lag monitoring if we have replicas
	if len(pool.replicas) > 0 {
		pool.lagChecker = time.NewTicker(cfg.LagCheckInterval)
		go pool.monitorReplicationLag()
	}

	return pool, nil
}

// openDB opens a database connection with the given configuration.
func openDB(ctx context.Context, dsn string, cfg Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Verify connection
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// Primary returns the primary database connection for write operations.
// Use this for INSERT, UPDATE, DELETE, and read-after-write queries.
// Returns the primary connection even if the pool is closed, to allow
// in-flight operations to complete gracefully (same as Replica behavior).
func (p *Pool) Primary() *sql.DB {
	return p.primary
}

// Replica returns a replica database connection for read operations.
// If no healthy replica is available or replication lag is too high,
// it falls back to the primary.
func (p *Pool) Replica() *sql.DB {
	if p.closed.Load() {
		return p.primary
	}

	// If no replicas configured, use primary
	if len(p.replicas) == 0 {
		return p.primary
	}

	// Round-robin selection with health check
	numReplicas := uint64(len(p.replicas))
	startIdx := p.replicaIndex.Add(1) - 1

	for i := uint64(0); i < numReplicas; i++ {
		idx := (startIdx + i) % numReplicas
		replica := p.replicas[idx]

		if replica.healthy.Load() {
			// Check replication lag
			lagMs := replica.lag.Load()
			if time.Duration(lagMs)*time.Millisecond <= p.config.MaxReplicationLag {
				return replica.db
			}
		}
	}

	// All replicas unhealthy or lagging, fall back to primary
	if p.config.RetryOnLag {
		return p.primary
	}

	// Return a random replica anyway if RetryOnLag is disabled
	idx := rand.Intn(len(p.replicas))
	return p.replicas[idx].db
}

// ReplicaWithLagCheck returns a replica only if its lag is below the threshold.
// Returns ErrReplicationLagHigh if all replicas are lagging.
func (p *Pool) ReplicaWithLagCheck(maxLag time.Duration) (*sql.DB, error) {
	if p.closed.Load() {
		return nil, ErrPoolClosed
	}

	if len(p.replicas) == 0 {
		return p.primary, nil
	}

	numReplicas := uint64(len(p.replicas))
	startIdx := p.replicaIndex.Add(1) - 1

	for i := uint64(0); i < numReplicas; i++ {
		idx := (startIdx + i) % numReplicas
		replica := p.replicas[idx]

		if replica.healthy.Load() {
			lagMs := replica.lag.Load()
			if time.Duration(lagMs)*time.Millisecond <= maxLag {
				return replica.db, nil
			}
		}
	}

	return nil, ErrReplicationLagHigh
}

// ReadWrite is an alias for Primary() for explicit intent.
func (p *Pool) ReadWrite() *sql.DB {
	return p.Primary()
}

// ReadOnly is an alias for Replica() for explicit intent.
func (p *Pool) ReadOnly() *sql.DB {
	return p.Replica()
}

// monitorReplicationLag periodically checks replication lag on all replicas.
func (p *Pool) monitorReplicationLag() {
	for {
		select {
		case <-p.stopLag:
			return
		case <-p.lagChecker.C:
			p.checkAllReplicaLag()
		}
	}
}

// checkAllReplicaLag checks the replication lag on all replicas.
func (p *Pool) checkAllReplicaLag() {
	for _, replica := range p.replicas {
		p.lagWg.Add(1)
		go func(r *replicaConn) {
			defer p.lagWg.Done()
			p.checkReplicaLag(r)
		}(replica)
	}
}

// checkReplicaLag checks the replication lag on a single replica.
func (p *Pool) checkReplicaLag(replica *replicaConn) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var lagSeconds sql.NullFloat64
	err := replica.db.QueryRowContext(ctx, `
		SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()))
		WHERE pg_is_in_recovery()
	`).Scan(&lagSeconds)

	if err != nil {
		// Query failed - mark as unhealthy
		replica.healthy.Store(false)
		return
	}

	replica.healthy.Store(true)

	if lagSeconds.Valid {
		// Convert to milliseconds
		replica.lag.Store(int64(lagSeconds.Float64 * 1000))
	} else {
		// Not in recovery mode - might be primary or lag unknown
		replica.lag.Store(0)
	}
}

// GetReplicationLag returns the current replication lag for each replica.
func (p *Pool) GetReplicationLag() map[string]time.Duration {
	result := make(map[string]time.Duration)

	for i, replica := range p.replicas {
		name := fmt.Sprintf("replica-%d", i)
		lagMs := replica.lag.Load()
		result[name] = time.Duration(lagMs) * time.Millisecond
	}

	return result
}

// HealthCheck performs a health check on all database connections.
func (p *Pool) HealthCheck(ctx context.Context) error {
	if p.closed.Load() {
		return ErrPoolClosed
	}

	// Check primary
	if err := p.primary.PingContext(ctx); err != nil {
		return fmt.Errorf("primary health check failed: %w", err)
	}

	// Check replicas (don't fail on replica issues)
	for i, replica := range p.replicas {
		if err := replica.db.PingContext(ctx); err != nil {
			replica.healthy.Store(false)
		} else {
			replica.healthy.Store(true)
		}
		_ = i // avoid unused variable warning
	}

	return nil
}

// Stats returns connection pool statistics.
func (p *Pool) Stats() PoolStats {
	stats := PoolStats{
		Primary: p.primary.Stats(),
	}

	for i, replica := range p.replicas {
		stats.Replicas = append(stats.Replicas, ReplicaStats{
			Index:   i,
			Stats:   replica.db.Stats(),
			Healthy: replica.healthy.Load(),
			LagMs:   replica.lag.Load(),
		})
	}

	return stats
}

// PoolStats contains statistics for all database pools.
type PoolStats struct {
	Primary  sql.DBStats
	Replicas []ReplicaStats
}

// ReplicaStats contains statistics for a single replica.
type ReplicaStats struct {
	Index   int
	Stats   sql.DBStats
	Healthy bool
	LagMs   int64
}

// Close closes all database connections.
func (p *Pool) Close() error {
	if p.closed.Swap(true) {
		return ErrPoolClosed
	}

	// Stop lag monitoring and wait for in-flight lag checks to finish
	if p.lagChecker != nil {
		p.lagChecker.Stop()
		close(p.stopLag)
		p.lagWg.Wait()
	}

	var errs []error

	if err := p.primary.Close(); err != nil {
		errs = append(errs, fmt.Errorf("failed to close primary: %w", err))
	}

	for i, replica := range p.replicas {
		if err := replica.db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close replica %d: %w", i, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// DB interface for database operations.
// This interface is satisfied by both *sql.DB and *Pool.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

// Querier is a minimal interface for read operations.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Execer is a minimal interface for write operations.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Transaction wraps a database transaction that uses the primary.
type Transaction struct {
	*sql.Tx
}

// Begin starts a new transaction on the primary database.
func (p *Pool) Begin(ctx context.Context) (*Transaction, error) {
	tx, err := p.primary.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &Transaction{Tx: tx}, nil
}

// BeginTx starts a new transaction with the given options on the primary database.
func (p *Pool) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Transaction, error) {
	tx, err := p.primary.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &Transaction{Tx: tx}, nil
}

// ReadOnlyTx starts a read-only transaction that can use a replica.
func (p *Pool) ReadOnlyTx(ctx context.Context) (*Transaction, error) {
	db := p.Replica()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("failed to begin read-only transaction: %w", err)
	}
	return &Transaction{Tx: tx}, nil
}

// QueryFunc runs a function with a database connection and handles connection errors.
// For reads, it uses replicas; for writes, it uses primary.
type QueryFunc func(ctx context.Context, db *sql.DB) error

// ReadWithRetry executes a read query with automatic retry on replica failure.
func (p *Pool) ReadWithRetry(ctx context.Context, fn QueryFunc) error {
	// Try replica first
	replica := p.Replica()
	if replica != p.primary {
		err := fn(ctx, replica)
		if err == nil {
			return nil
		}
		// Fall back to primary on error
	}

	// Use primary as fallback
	return fn(ctx, p.primary)
}

// WaitForReplication waits until replication lag is below the threshold.
// This is useful for read-after-write consistency when you want to read
// from a replica but need the data to be replicated first.
func (p *Pool) WaitForReplication(ctx context.Context, maxLag time.Duration) error {
	if len(p.replicas) == 0 {
		return nil // No replicas, nothing to wait for
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Check if any replica has acceptable lag
			for _, replica := range p.replicas {
				if replica.healthy.Load() {
					lagMs := replica.lag.Load()
					if time.Duration(lagMs)*time.Millisecond <= maxLag {
						return nil
					}
				}
			}
		}
	}
}

// MustNewPool creates a new pool and panics on error.
// Use only in main() or initialization code.
func MustNewPool(ctx context.Context, cfg Config) *Pool {
	pool, err := NewPool(ctx, cfg)
	if err != nil {
		panic(fmt.Sprintf("failed to create database pool: %v", err))
	}
	return pool
}
