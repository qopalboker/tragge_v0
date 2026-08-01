package db

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxOpenConns != 25 {
		t.Errorf("expected MaxOpenConns=25, got %d", cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns != 5 {
		t.Errorf("expected MaxIdleConns=5, got %d", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 5*time.Minute {
		t.Errorf("expected ConnMaxLifetime=5m, got %v", cfg.ConnMaxLifetime)
	}
	if cfg.MaxReplicationLag != 10*time.Second {
		t.Errorf("expected MaxReplicationLag=10s, got %v", cfg.MaxReplicationLag)
	}
	if cfg.LagCheckInterval != 5*time.Second {
		t.Errorf("expected LagCheckInterval=5s, got %v", cfg.LagCheckInterval)
	}
	if !cfg.RetryOnLag {
		t.Error("expected RetryOnLag=true")
	}
}

func TestNewPoolRequiresPrimaryDSN(t *testing.T) {
	ctx := context.Background()
	cfg := Config{}

	_, err := NewPool(ctx, cfg)
	if err == nil {
		t.Error("expected error for missing primary DSN")
	}
}

func TestPoolStatsStructure(t *testing.T) {
	// Test that PoolStats and ReplicaStats have expected fields
	stats := PoolStats{
		Primary: sql.DBStats{
			MaxOpenConnections: 25,
			OpenConnections:    5,
			InUse:              2,
			Idle:               3,
		},
		Replicas: []ReplicaStats{
			{
				Index:   0,
				Healthy: true,
				LagMs:   100,
			},
			{
				Index:   1,
				Healthy: false,
				LagMs:   5000,
			},
		},
	}

	if stats.Primary.MaxOpenConnections != 25 {
		t.Error("unexpected MaxOpenConnections")
	}
	if len(stats.Replicas) != 2 {
		t.Errorf("expected 2 replicas, got %d", len(stats.Replicas))
	}
	if !stats.Replicas[0].Healthy {
		t.Error("expected first replica to be healthy")
	}
	if stats.Replicas[1].Healthy {
		t.Error("expected second replica to be unhealthy")
	}
	if stats.Replicas[0].LagMs != 100 {
		t.Errorf("expected lag 100ms, got %d", stats.Replicas[0].LagMs)
	}
}

func TestReplicationLagMap(t *testing.T) {
	// Create a mock pool to test GetReplicationLag
	pool := &Pool{
		replicas: []*replicaConn{
			{},
			{},
		},
	}
	pool.replicas[0].lag.Store(100)
	pool.replicas[1].lag.Store(5000)

	lagMap := pool.GetReplicationLag()

	if len(lagMap) != 2 {
		t.Errorf("expected 2 entries, got %d", len(lagMap))
	}
	if lagMap["replica-0"] != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", lagMap["replica-0"])
	}
	if lagMap["replica-1"] != 5*time.Second {
		t.Errorf("expected 5s, got %v", lagMap["replica-1"])
	}
}

func TestPoolClosed(t *testing.T) {
	pool := &Pool{}
	pool.closed.Store(true)

	// Primary should return nil when closed
	if pool.Primary() != nil {
		t.Error("expected nil from Primary() when closed")
	}

	// Replica should return primary (nil) when closed
	primary := &sql.DB{}
	pool.primary = primary
	if pool.Replica() != primary {
		t.Error("expected primary from Replica() when closed")
	}
}

func TestReplicaFallbackToPrimary(t *testing.T) {
	primary := &sql.DB{}
	pool := &Pool{
		primary:  primary,
		replicas: []*replicaConn{}, // No replicas
		config:   DefaultConfig(),
	}

	// Should return primary when no replicas configured
	result := pool.Replica()
	if result != primary {
		t.Error("expected fallback to primary when no replicas")
	}
}

func TestReplicaSelectionWithUnhealthyReplica(t *testing.T) {
	primary := &sql.DB{}
	replica1DB := &sql.DB{}
	replica2DB := &sql.DB{}

	replica1 := &replicaConn{db: replica1DB}
	replica1.healthy.Store(false)
	replica1.lag.Store(0)

	replica2 := &replicaConn{db: replica2DB}
	replica2.healthy.Store(true)
	replica2.lag.Store(100) // 100ms lag

	pool := &Pool{
		primary:  primary,
		replicas: []*replicaConn{replica1, replica2},
		config:   DefaultConfig(),
	}

	// Should skip unhealthy replica1 and return replica2
	result := pool.Replica()
	if result == primary {
		// First call might hit replica1, second should hit replica2
		result = pool.Replica()
	}

	// Eventually should get replica2
	foundReplica2 := false
	for i := 0; i < 10; i++ {
		if pool.Replica() == replica2DB {
			foundReplica2 = true
			break
		}
	}
	if !foundReplica2 {
		t.Error("expected to eventually get healthy replica2")
	}
}

func TestReplicaWithLagCheck(t *testing.T) {
	primary := &sql.DB{}
	replicaDB := &sql.DB{}

	replica := &replicaConn{db: replicaDB}
	replica.healthy.Store(true)
	replica.lag.Store(15000) // 15 seconds lag

	pool := &Pool{
		primary:  primary,
		replicas: []*replicaConn{replica},
		config:   DefaultConfig(),
	}

	// Request max lag of 5 seconds - should fail
	_, err := pool.ReplicaWithLagCheck(5 * time.Second)
	if err != ErrReplicationLagHigh {
		t.Errorf("expected ErrReplicationLagHigh, got %v", err)
	}

	// Request max lag of 20 seconds - should succeed
	db, err := pool.ReplicaWithLagCheck(20 * time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if db != replicaDB {
		t.Error("expected replica database")
	}
}

func TestReadWriteAlias(t *testing.T) {
	primary := &sql.DB{}
	pool := &Pool{
		primary: primary,
		config:  DefaultConfig(),
	}

	if pool.ReadWrite() != pool.Primary() {
		t.Error("ReadWrite() should return Primary()")
	}
}

func TestReadOnlyAlias(t *testing.T) {
	primary := &sql.DB{}
	pool := &Pool{
		primary: primary,
		config:  DefaultConfig(),
	}

	if pool.ReadOnly() != pool.Replica() {
		t.Error("ReadOnly() should return Replica()")
	}
}

func TestPoolClosedError(t *testing.T) {
	pool := &Pool{}
	pool.closed.Store(true)

	err := pool.Close()
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestHealthCheckReturnsPoolClosed(t *testing.T) {
	pool := &Pool{}
	pool.closed.Store(true)

	err := pool.HealthCheck(context.Background())
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestReplicaWithLagCheckPoolClosed(t *testing.T) {
	pool := &Pool{}
	pool.closed.Store(true)

	_, err := pool.ReplicaWithLagCheck(time.Second)
	if err != ErrPoolClosed {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

func TestReplicaWithLagCheckNoReplicas(t *testing.T) {
	primary := &sql.DB{}
	pool := &Pool{
		primary:  primary,
		replicas: []*replicaConn{},
		config:   DefaultConfig(),
	}

	db, err := pool.ReplicaWithLagCheck(time.Second)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if db != primary {
		t.Error("expected primary when no replicas")
	}
}

// Interface compliance tests
var _ DB = (*sql.DB)(nil)
var _ Querier = (*sql.DB)(nil)
var _ Execer = (*sql.DB)(nil)
