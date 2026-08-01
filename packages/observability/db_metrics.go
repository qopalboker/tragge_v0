package observability

import (
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// DBStatsProvider is implemented by anything that exposes sql.DBStats.
// Both *sql.DB and custom pool wrappers can satisfy this interface.
type DBStatsProvider interface {
	Stats() sql.DBStats
}

// sqlDBAdapter wraps *sql.DB to satisfy DBStatsProvider.
type sqlDBAdapter struct {
	db *sql.DB
}

func (a *sqlDBAdapter) Stats() sql.DBStats { return a.db.Stats() }

// DBPoolMetrics exposes Go's sql.DB connection pool statistics as
// Prometheus gauges.  Multiple databases (e.g. primary + replicas)
// can be registered; their stats are summed.
//
// Metrics registered (all under the provided namespace):
//
//	db_pool_connections{state="open"}              — open connections (in-use + idle)
//	db_pool_connections{state="in_use"}            — connections currently executing queries
//	db_pool_connections{state="idle"}              — idle connections in pool
//	db_pool_max_open                               — configured MaxOpenConns
//	db_pool_wait_count_total                       — cumulative waits for a connection
//	db_pool_wait_duration_seconds_total            — cumulative time blocked waiting
type DBPoolMetrics struct {
	connections *prometheus.GaugeVec
	maxOpen     prometheus.Gauge
	waitCount   prometheus.Gauge
	waitDur     prometheus.Gauge

	providers []DBStatsProvider
	mu        sync.Mutex
	stopCh    chan struct{}
}

// NewDBPoolMetrics creates and registers DB connection pool metrics.
// The namespace is typically the service name (e.g. "trading_engine").
func NewDBPoolMetrics(registerer prometheus.Registerer, namespace string) *DBPoolMetrics {
	m := &DBPoolMetrics{
		connections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_pool_connections",
				Help:      "Number of database connections by state.",
			},
			[]string{"state"},
		),
		maxOpen: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_pool_max_open",
				Help:      "Configured maximum number of open connections.",
			},
		),
		waitCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_pool_wait_count_total",
				Help:      "Total number of connections waited for (monotonically increasing).",
			},
		),
		waitDur: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "db_pool_wait_duration_seconds_total",
				Help:      "Total time blocked waiting for a new connection (monotonically increasing).",
			},
		),
		stopCh: make(chan struct{}),
	}

	registerer.MustRegister(m.connections, m.maxOpen, m.waitCount, m.waitDur)
	return m
}

// AddDB adds a *sql.DB whose stats will be collected.
func (m *DBPoolMetrics) AddDB(db *sql.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers = append(m.providers, &sqlDBAdapter{db: db})
}

// AddProvider adds a custom stats provider (e.g. a db.Pool wrapper).
func (m *DBPoolMetrics) AddProvider(p DBStatsProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers = append(m.providers, p)
}

// Start begins periodic collection of DB stats at the given interval.
// Call Stop() to halt the goroutine.
func (m *DBPoolMetrics) Start(interval time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ERROR] db_metrics panic: %s", RedactPanic(r))
			}
		}()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		m.collect()

		for {
			select {
			case <-ticker.C:
				m.collect()
			case <-m.stopCh:
				return
			}
		}
	}()
}

// Stop halts the periodic collection goroutine.
func (m *DBPoolMetrics) Stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
}

func (m *DBPoolMetrics) collect() {
	m.mu.Lock()
	providers := make([]DBStatsProvider, len(m.providers))
	copy(providers, m.providers)
	m.mu.Unlock()

	var (
		open, inUse, idle int
		maxOpen           int
		waitCount         int64
		waitDur           time.Duration
	)

	for _, p := range providers {
		s := p.Stats()
		open += s.OpenConnections
		inUse += s.InUse
		idle += s.Idle
		maxOpen += s.MaxOpenConnections
		waitCount += s.WaitCount
		waitDur += s.WaitDuration
	}

	m.connections.WithLabelValues("open").Set(float64(open))
	m.connections.WithLabelValues("in_use").Set(float64(inUse))
	m.connections.WithLabelValues("idle").Set(float64(idle))
	m.maxOpen.Set(float64(maxOpen))
	m.waitCount.Set(float64(waitCount))
	m.waitDur.Set(waitDur.Seconds())
}
