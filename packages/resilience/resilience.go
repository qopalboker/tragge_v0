// Package resilience provides fault tolerance patterns for microservices.
// It combines circuit breakers, retries, timeouts, and bulkheads to prevent
// cascade failures when dependencies fail.
//
// Usage:
//
//	r := resilience.New(resilience.Config{ServiceName: "my-service", Logger: logger})
//	r.RegisterDependency("postgres", resilience.DatabaseDep)
//	r.RegisterDependency("redis", resilience.CacheDep)
//
//	// Execute with protection
//	result, err := r.Execute("postgres", func(ctx context.Context) (any, error) {
//	    return db.Query(ctx, "SELECT ...")
//	})
package resilience

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"go.uber.org/zap"
)

var (
	// ErrAllCircuitsOpen is returned when all fallback options have failed
	ErrAllCircuitsOpen = errors.New("all circuits are open, service degraded")
	// ErrDependencyNotRegistered is returned when trying to execute on unregistered dependency
	ErrDependencyNotRegistered = errors.New("dependency not registered")
	// ErrBulkheadFull is returned when the bulkhead semaphore is full
	ErrBulkheadFull = errors.New("bulkhead full, request rejected")
)

// DependencyType represents the type of external dependency
type DependencyType int

const (
	// DatabaseDep is for database connections (PostgreSQL, MySQL, etc.)
	DatabaseDep DependencyType = iota
	// DatabaseReplicaDep is for read replicas
	DatabaseReplicaDep
	// CacheDep is for cache systems (Redis, Memcached)
	CacheDep
	// CacheClusterDep is for Redis Cluster
	CacheClusterDep
	// MessageQueueDep is for message queues (Kafka, RabbitMQ)
	MessageQueueDep
	// MessageQueueConsumerDep is for message queue consumers
	MessageQueueConsumerDep
	// ExternalAPIDep is for external HTTP APIs
	ExternalAPIDep
	// InternalServiceDep is for internal microservice calls
	InternalServiceDep
	// WebSocketDep is for WebSocket connections
	WebSocketDep
)

// Config holds configuration for the Resilience manager
type Config struct {
	// ServiceName identifies this service in logs and metrics
	ServiceName string
	// Logger for logging circuit breaker events
	Logger *zap.Logger
	// OnStateChange is called when any circuit breaker changes state
	OnStateChange func(dependency string, from, to circuitbreaker.State)
	// EnableBulkhead enables bulkhead pattern for resource isolation
	EnableBulkhead bool
	// BulkheadSize is the maximum concurrent requests per dependency
	BulkheadSize int
}

// Dependency holds configuration and state for a single dependency
type Dependency struct {
	Name           string
	Type           DependencyType
	CircuitBreaker *circuitbreaker.CircuitBreaker
	Bulkhead       chan struct{} // Semaphore for bulkhead pattern
	Fallback       func(ctx context.Context) (any, error)
	Critical       bool // If true, affects health check status
}

// Resilience manages fault tolerance for all external dependencies
type Resilience struct {
	config       Config
	dependencies map[string]*Dependency
	mu           sync.RWMutex
	logger       *zap.Logger
}

// New creates a new Resilience manager
func New(cfg Config) *Resilience {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "unknown-service"
	}
	if cfg.BulkheadSize <= 0 {
		cfg.BulkheadSize = 100 // Default concurrent requests
	}

	logger := cfg.Logger
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	logger = observability.WrapLogger(logger)

	return &Resilience{
		config:       cfg,
		dependencies: make(map[string]*Dependency),
		logger:       logger,
	}
}

// RegisterDependency registers a new external dependency with appropriate circuit breaker config
func (r *Resilience) RegisterDependency(name string, depType DependencyType, opts ...DependencyOption) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get circuit breaker config based on dependency type
	cbConfig := r.getCircuitConfig(name, depType)

	// Apply state change handler
	if r.config.OnStateChange != nil {
		cbConfig.OnStateChange = r.config.OnStateChange
	} else {
		cbConfig.OnStateChange = func(n string, from, to circuitbreaker.State) {
			r.logger.Warn("Circuit breaker state changed",
				zap.String("service", r.config.ServiceName),
				zap.String("dependency", n),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		}
	}

	dep := &Dependency{
		Name:           name,
		Type:           depType,
		CircuitBreaker: circuitbreaker.New(cbConfig),
		Critical:       r.isCriticalDependency(depType),
	}

	// Setup bulkhead if enabled
	if r.config.EnableBulkhead {
		dep.Bulkhead = make(chan struct{}, r.config.BulkheadSize)
		initBulkheadMetrics(name, r.config.BulkheadSize)
	}

	// Apply options
	for _, opt := range opts {
		opt(dep)
	}

	// Initialize bulkhead metrics if custom size was set via options
	if dep.Bulkhead != nil && !r.config.EnableBulkhead {
		initBulkheadMetrics(name, cap(dep.Bulkhead))
	}

	r.dependencies[name] = dep
}

// DependencyOption is a function that configures a Dependency
type DependencyOption func(*Dependency)

// WithFallback sets a fallback function for when the circuit is open
func WithFallback(fallback func(ctx context.Context) (any, error)) DependencyOption {
	return func(d *Dependency) {
		d.Fallback = fallback
	}
}

// WithCritical marks the dependency as critical for health checks
func WithCritical(critical bool) DependencyOption {
	return func(d *Dependency) {
		d.Critical = critical
	}
}

// WithBulkheadSize sets a custom bulkhead size for this dependency
func WithBulkheadSize(size int) DependencyOption {
	return func(d *Dependency) {
		if size > 0 {
			d.Bulkhead = make(chan struct{}, size)
		}
	}
}

// Execute runs a function with circuit breaker protection
func (r *Resilience) Execute(depName string, fn func(ctx context.Context) (any, error)) (any, error) {
	return r.ExecuteWithContext(context.Background(), depName, fn)
}

// ExecuteWithContext runs a function with circuit breaker protection and context
func (r *Resilience) ExecuteWithContext(ctx context.Context, depName string, fn func(ctx context.Context) (any, error)) (any, error) {
	r.mu.RLock()
	dep, exists := r.dependencies[depName]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrDependencyNotRegistered, depName)
	}

	// Bulkhead check
	if dep.Bulkhead != nil {
		select {
		case dep.Bulkhead <- struct{}{}:
			recordBulkheadAcquire(depName)
			defer func() {
				<-dep.Bulkhead
				recordBulkheadRelease(depName)
			}()
		default:
			r.logger.Warn("Bulkhead full, rejecting request",
				zap.String("dependency", depName))
			recordBulkheadRejection(depName)
			return nil, ErrBulkheadFull
		}
	}

	// Execute with circuit breaker
	var result any
	err := dep.CircuitBreaker.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var fnErr error
		result, fnErr = fn(ctx)
		return fnErr
	})

	// Handle circuit open - try fallback
	if errors.Is(err, circuitbreaker.ErrCircuitOpen) && dep.Fallback != nil {
		r.logger.Info("Circuit open, using fallback",
			zap.String("dependency", depName))
		return dep.Fallback(ctx)
	}

	return result, err
}

// ExecuteVoid runs a function that returns only an error with circuit breaker protection
func (r *Resilience) ExecuteVoid(depName string, fn func(ctx context.Context) error) error {
	_, err := r.ExecuteWithContext(context.Background(), depName, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	})
	return err
}

// ExecuteVoidWithContext runs a function that returns only an error with circuit breaker protection
func (r *Resilience) ExecuteVoidWithContext(ctx context.Context, depName string, fn func(ctx context.Context) error) error {
	_, err := r.ExecuteWithContext(ctx, depName, func(ctx context.Context) (any, error) {
		return nil, fn(ctx)
	})
	return err
}

// GetDependency returns a registered dependency
func (r *Resilience) GetDependency(name string) (*Dependency, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	dep, ok := r.dependencies[name]
	return dep, ok
}

// IsHealthy returns true if all critical dependencies have closed circuits
func (r *Resilience) IsHealthy() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, dep := range r.dependencies {
		if dep.Critical && dep.CircuitBreaker.State() == circuitbreaker.StateOpen {
			return false
		}
	}
	return true
}

// GetStatus returns the status of all dependencies
func (r *Resilience) GetStatus() map[string]DependencyStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make(map[string]DependencyStatus)
	for name, dep := range r.dependencies {
		metrics := dep.CircuitBreaker.Metrics()
		status[name] = DependencyStatus{
			Name:         name,
			Type:         dep.Type.String(),
			State:        dep.CircuitBreaker.State().String(),
			FailureCount: dep.CircuitBreaker.FailureCount(),
			Critical:     dep.Critical,
			Metrics: DependencyMetrics{
				TotalRequests:   metrics.TotalRequests,
				TotalSuccesses:  metrics.TotalSuccesses,
				TotalFailures:   metrics.TotalFailures,
				TotalRejections: metrics.TotalRejections,
				TotalTimeouts:   metrics.TotalTimeouts,
				StateChanges:    metrics.StateChanges,
			},
		}
	}
	return status
}

// Reset resets all circuit breakers
func (r *Resilience) Reset() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, dep := range r.dependencies {
		dep.CircuitBreaker.Reset()
	}
	r.logger.Info("All circuit breakers reset",
		zap.String("service", r.config.ServiceName))
}

// ResetDependency resets a specific circuit breaker
func (r *Resilience) ResetDependency(name string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dep, exists := r.dependencies[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrDependencyNotRegistered, name)
	}

	dep.CircuitBreaker.Reset()
	r.logger.Info("Circuit breaker reset",
		zap.String("service", r.config.ServiceName),
		zap.String("dependency", name))
	return nil
}

// DependencyStatus holds the status of a single dependency
type DependencyStatus struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	State        string            `json:"state"`
	FailureCount int               `json:"failure_count"`
	Critical     bool              `json:"critical"`
	Metrics      DependencyMetrics `json:"metrics"`
}

// DependencyMetrics holds metrics for a dependency
type DependencyMetrics struct {
	TotalRequests   int64 `json:"total_requests"`
	TotalSuccesses  int64 `json:"total_successes"`
	TotalFailures   int64 `json:"total_failures"`
	TotalRejections int64 `json:"total_rejections"`
	TotalTimeouts   int64 `json:"total_timeouts"`
	StateChanges    int64 `json:"state_changes"`
}

// getCircuitConfig returns the appropriate circuit breaker config for a dependency type
func (r *Resilience) getCircuitConfig(name string, depType DependencyType) circuitbreaker.Config {
	switch depType {
	case DatabaseDep:
		return circuitbreaker.DatabaseCircuitConfig(name)
	case DatabaseReplicaDep:
		return circuitbreaker.DatabaseReplicaCircuitConfig(name)
	case CacheDep:
		return circuitbreaker.RedisCircuitConfig(name)
	case CacheClusterDep:
		return circuitbreaker.RedisClusterCircuitConfig(name)
	case MessageQueueDep:
		return circuitbreaker.KafkaCircuitConfig(name)
	case MessageQueueConsumerDep:
		return circuitbreaker.KafkaConsumerCircuitConfig(name)
	case ExternalAPIDep:
		return circuitbreaker.ExternalAPICircuitConfig(name)
	case InternalServiceDep:
		return circuitbreaker.ShardRouterCircuitConfig(name)
	case WebSocketDep:
		return circuitbreaker.WebSocketCircuitConfig(name)
	default:
		return circuitbreaker.Config{Name: name}
	}
}

// isCriticalDependency determines if a dependency type is critical
func (r *Resilience) isCriticalDependency(depType DependencyType) bool {
	switch depType {
	case DatabaseDep, MessageQueueDep:
		return true
	default:
		return false
	}
}

// String returns the string representation of DependencyType
func (d DependencyType) String() string {
	switch d {
	case DatabaseDep:
		return "database"
	case DatabaseReplicaDep:
		return "database_replica"
	case CacheDep:
		return "cache"
	case CacheClusterDep:
		return "cache_cluster"
	case MessageQueueDep:
		return "message_queue"
	case MessageQueueConsumerDep:
		return "message_queue_consumer"
	case ExternalAPIDep:
		return "external_api"
	case InternalServiceDep:
		return "internal_service"
	case WebSocketDep:
		return "websocket"
	default:
		return "unknown"
	}
}
