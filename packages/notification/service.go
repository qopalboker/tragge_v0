package notification

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// ServiceConfig holds configuration for the unified notification service.
type ServiceConfig struct {
	// Discord configuration
	Discord DiscordConfig

	// Email configuration
	Email EmailConfig

	// Enabled is a global enable/disable switch
	Enabled bool

	// AsyncEnabled controls whether to use async sending
	AsyncEnabled bool

	// AsyncWorkers is the number of async workers (default: 5)
	AsyncWorkers int

	// AsyncQueueSize is the async queue buffer size (default: 100)
	AsyncQueueSize int

	// Environment is the deployment environment (development/staging/production)
	Environment string

	// ServiceName is the name of the service using notifications
	ServiceName string

	// EmailRecipients are the default email recipients for alerts
	EmailRecipients []string

	// ShutdownTimeout is the maximum time to wait for pending notifications during shutdown
	ShutdownTimeout time.Duration
}

// NotificationMetrics holds Prometheus metrics for the notification service.
type NotificationMetrics struct {
	sentTotal         *prometheus.CounterVec
	sendDuration      *prometheus.HistogramVec
	queueSize         prometheus.Gauge
	errorsTotal       *prometheus.CounterVec
	workersActive     prometheus.Gauge
	droppedTotal      prometheus.Counter
	syncFallbackTotal prometheus.Counter
}

// Service provides a unified interface for sending notifications
// through multiple channels (Discord, Email) with support for async sending,
// worker pools, and severity-based routing.
type Service struct {
	discord *DiscordNotifier
	email   *EmailNotifier
	logger  *zap.Logger
	metrics *NotificationMetrics
	config  ServiceConfig

	// Worker pool
	workers chan struct{} // semaphore for worker pool
	queue   chan asyncJob
	wg      sync.WaitGroup

	// Shutdown control
	mu         sync.RWMutex
	isShutdown bool
	stopCh     chan struct{}
	closeQueue sync.Once
}

// asyncJob represents an async notification job.
type asyncJob struct {
	ctx       context.Context
	alert     Alert
	timestamp time.Time
}

// BugAlertDetails contains details for a bug alert.
type BugAlertDetails struct {
	Title      string
	Message    string
	Severity   Severity
	Service    string
	StackTrace string
	TraceID    string
	SpanID     string
	Metadata   map[string]string
}

// ContestAlertDetails contains details for a contest alert.
type ContestAlertDetails struct {
	ContestID   string
	ContestName string
	Message     string
	Severity    Severity
	Metadata    map[string]string
}

// SystemAlertDetails contains details for a system alert.
type SystemAlertDetails struct {
	Title    string
	Message  string
	Severity Severity
	Service  string
	Metadata map[string]string
}

// Common errors for the notification service.
var (
	ErrServiceDisabled     = errors.New("notification service: service is disabled")
	ErrServiceShuttingDown = errors.New("notification service: service is shutting down")
	ErrQueueFullService    = errors.New("notification service: async queue is full")
	ErrNoChannelsEnabled   = errors.New("notification service: no notification channels enabled")
)

// NewService creates a new unified notification service.
func NewService(ctx context.Context, cfg ServiceConfig, logger *zap.Logger, registry prometheus.Registerer) (*Service, error) {
	// Apply defaults
	cfg = applyServiceDefaults(cfg)

	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("notification service: failed to create logger: %w", err)
		}
	}
	logger = ensureRedactingLogger(logger)
	logger = logger.With(zap.String("component", "notification-service"))

	// Create metrics
	metrics := newNotificationMetrics(cfg.ServiceName, registry)

	svc := &Service{
		logger:  logger,
		metrics: metrics,
		config:  cfg,
		workers: make(chan struct{}, cfg.AsyncWorkers),
		queue:   make(chan asyncJob, cfg.AsyncQueueSize),
		stopCh:  make(chan struct{}),
	}

	// Initialize Discord notifier if enabled
	if cfg.Discord.Enabled && cfg.Discord.WebhookURL != "" {
		discord, err := NewDiscordNotifier(cfg.Discord, logger)
		if err != nil {
			logger.Warn("failed to create Discord notifier, continuing without Discord",
				zap.Error(err))
		} else {
			svc.discord = discord
			logger.Info("Discord notifier initialized")
		}
	}

	// Initialize Email notifier if enabled
	if cfg.Email.Enabled && cfg.Email.APIKey != "" {
		email, err := NewEmailNotifier(cfg.Email, logger)
		if err != nil {
			logger.Warn("failed to create Email notifier, continuing without Email",
				zap.Error(err))
		} else {
			svc.email = email
			logger.Info("Email notifier initialized")
		}
	}

	// Start async workers if enabled
	if cfg.AsyncEnabled {
		svc.startWorkers(ctx)
	}

	logger.Info("notification service initialized",
		zap.Bool("enabled", cfg.Enabled),
		zap.Bool("async", cfg.AsyncEnabled),
		zap.Int("workers", cfg.AsyncWorkers),
		zap.String("environment", cfg.Environment),
		zap.Bool("discord_enabled", svc.discord != nil),
		zap.Bool("email_enabled", svc.email != nil))

	return svc, nil
}

// applyServiceDefaults applies default values to the service configuration.
func applyServiceDefaults(cfg ServiceConfig) ServiceConfig {
	if cfg.AsyncWorkers <= 0 {
		cfg.AsyncWorkers = 5
	}
	if cfg.AsyncQueueSize <= 0 {
		cfg.AsyncQueueSize = 100
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 30 * time.Second
	}
	return cfg
}

// newNotificationMetrics creates and registers Prometheus metrics.
func newNotificationMetrics(serviceName string, registry prometheus.Registerer) *NotificationMetrics {
	namespace := "tragge"
	if serviceName != "" {
		namespace = serviceName
	}

	m := &NotificationMetrics{
		sentTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "notification",
				Name:      "sent_total",
				Help:      "Total number of notifications sent",
			},
			[]string{"channel", "type", "status"},
		),
		sendDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "notification",
				Name:      "send_duration_seconds",
				Help:      "Duration of notification sends in seconds",
				Buckets:   []float64{.01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"channel"},
		),
		queueSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "notification",
				Name:      "queue_size",
				Help:      "Current size of the async notification queue",
			},
		),
		errorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "notification",
				Name:      "errors_total",
				Help:      "Total number of notification errors",
			},
			[]string{"channel", "error_type"},
		),
		workersActive: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "notification",
				Name:      "workers_active",
				Help:      "Number of currently active workers",
			},
		),
		droppedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "notification",
				Name:      "dropped_total",
				Help:      "Total number of notifications dropped due to full queue",
			},
		),
		syncFallbackTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "notification",
				Name:      "sync_fallback_total",
				Help:      "Total number of notifications sent synchronously due to full queue",
			},
		),
	}

	// Register metrics with the provided registry
	if registry != nil {
		collectors := []prometheus.Collector{
			m.sentTotal,
			m.sendDuration,
			m.queueSize,
			m.errorsTotal,
			m.workersActive,
			m.droppedTotal,
			m.syncFallbackTotal,
		}

		for _, c := range collectors {
			if err := registry.Register(c); err != nil {
				// Ignore already registered errors
				if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
					// Log but don't fail
				}
			}
		}
	}

	return m
}

// startWorkers starts the async worker goroutines.
func (s *Service) startWorkers(ctx context.Context) {
	for i := 0; i < s.config.AsyncWorkers; i++ {
		s.wg.Add(1)
		go s.worker(ctx, i)
	}
	s.logger.Info("async workers started", zap.Int("count", s.config.AsyncWorkers))
}

// worker is the main worker loop for processing async notifications.
func (s *Service) worker(ctx context.Context, id int) {
	defer s.wg.Done()

	s.logger.Debug("worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-s.stopCh:
			s.logger.Debug("worker stopping", zap.Int("worker_id", id))
			return
		case job, ok := <-s.queue:
			if !ok {
				return
			}

			s.metrics.workersActive.Inc()
			s.metrics.queueSize.Dec()

			s.processJob(job)

			s.metrics.workersActive.Dec()
		}
	}
}

// processJob processes a single async notification job.
func (s *Service) processJob(job asyncJob) {
	// Use job context if still valid, otherwise create a new timeout context
	ctx := job.ctx
	if ctx.Err() != nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := s.sendAlertSync(ctx, job.alert); err != nil {
		s.logger.Error("failed to send async notification",
			zap.String("alert_id", job.alert.ID),
			zap.String("title", job.alert.Title),
			zap.Duration("latency", time.Since(job.timestamp)),
			zap.Error(err))
	}
}

// SendAlert sends an alert through the appropriate channels based on severity.
// If async is enabled, it queues the alert and returns immediately.
func (s *Service) SendAlert(ctx context.Context, alert Alert) error {
	if !s.config.Enabled {
		s.logger.Debug("notification service disabled, skipping alert",
			zap.String("title", alert.Title))
		return nil
	}

	s.mu.RLock()
	if s.isShutdown {
		s.mu.RUnlock()
		return ErrServiceShuttingDown
	}
	s.mu.RUnlock()

	// Prepare alert
	alert = s.prepareAlert(alert)

	// In development, just log
	if s.config.Environment == "development" {
		s.logger.Info("development mode: logging notification instead of sending",
			zap.String("alert_id", alert.ID),
			zap.String("type", alert.Type.String()),
			zap.String("severity", alert.Severity.String()),
			zap.String("title", alert.Title),
			zap.String("message", alert.Message))
		return nil
	}

	if s.config.AsyncEnabled {
		return s.queueAlert(ctx, alert)
	}

	return s.sendAlertSync(ctx, alert)
}

// SendAlertAsync sends an alert asynchronously without blocking.
// It does not return errors - check metrics for failures.
func (s *Service) SendAlertAsync(alert Alert) {
	if !s.config.Enabled {
		return
	}

	s.mu.RLock()
	shutdown := s.isShutdown
	s.mu.RUnlock()

	if shutdown {
		s.metrics.droppedTotal.Inc()
		return
	}

	alert = s.prepareAlert(alert)

	// In development, just log
	if s.config.Environment == "development" {
		s.logger.Info("development mode: logging async notification",
			zap.String("alert_id", alert.ID),
			zap.String("type", alert.Type.String()),
			zap.String("title", alert.Title))
		return
	}

	// Queue without blocking - if queue is full, fall back to sync for
	// critical/high severity, otherwise drop the notification.
	select {
	case s.queue <- asyncJob{
		ctx:       context.Background(),
		alert:     alert,
		timestamp: time.Now(),
	}:
		s.metrics.queueSize.Inc()
	default:
		if alert.Severity == SeverityCritical || alert.Severity == SeverityHigh {
			s.metrics.syncFallbackTotal.Inc()
			s.logger.Warn("async queue full, sending critical/high alert synchronously",
				zap.String("alert_id", alert.ID),
				zap.String("severity", alert.Severity.String()),
				zap.String("title", alert.Title))
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_ = s.sendAlertSync(ctx, alert)
			cancel()
		} else {
			s.metrics.droppedTotal.Inc()
			s.logger.Warn("async queue full, dropping notification",
				zap.String("alert_id", alert.ID),
				zap.String("title", alert.Title))
		}
	}
}

// NotifyBug sends a bug alert notification.
func (s *Service) NotifyBug(ctx context.Context, bug BugAlertDetails) error {
	alert := Alert{
		Type:      AlertTypeBug,
		Severity:  bug.Severity,
		Title:     bug.Title,
		Message:   bug.Message,
		Service:   bug.Service,
		TraceID:   bug.TraceID,
		SpanID:    bug.SpanID,
		Timestamp: time.Now().UTC(),
		Metadata:  make(map[string]string),
	}

	// Copy metadata
	for k, v := range bug.Metadata {
		alert.Metadata[k] = v
	}

	// Add stack trace to metadata
	if bug.StackTrace != "" {
		alert.Metadata["stack_trace"] = bug.StackTrace
	}

	return s.SendAlert(ctx, alert)
}

// NotifyContestStart sends a contest start notification.
func (s *Service) NotifyContestStart(ctx context.Context, contest ContestAlertDetails) error {
	alert := Alert{
		Type:      AlertTypeContest,
		Severity:  contest.Severity,
		Title:     fmt.Sprintf("Contest Started: %s", contest.ContestName),
		Message:   contest.Message,
		Service:   s.config.ServiceName,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"contest_id":   contest.ContestID,
			"contest_name": contest.ContestName,
			"event":        "start",
		},
	}

	// Copy additional metadata
	for k, v := range contest.Metadata {
		alert.Metadata[k] = v
	}

	return s.SendAlert(ctx, alert)
}

// NotifyContestEnd sends a contest end notification.
func (s *Service) NotifyContestEnd(ctx context.Context, contest ContestAlertDetails) error {
	alert := Alert{
		Type:      AlertTypeContest,
		Severity:  contest.Severity,
		Title:     fmt.Sprintf("Contest Ended: %s", contest.ContestName),
		Message:   contest.Message,
		Service:   s.config.ServiceName,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]string{
			"contest_id":   contest.ContestID,
			"contest_name": contest.ContestName,
			"event":        "end",
		},
	}

	// Copy additional metadata
	for k, v := range contest.Metadata {
		alert.Metadata[k] = v
	}

	return s.SendAlert(ctx, alert)
}

// NotifySystemAlert sends a system alert notification.
func (s *Service) NotifySystemAlert(ctx context.Context, alert SystemAlertDetails) error {
	a := Alert{
		Type:      AlertTypeSystem,
		Severity:  alert.Severity,
		Title:     alert.Title,
		Message:   alert.Message,
		Service:   alert.Service,
		Timestamp: time.Now().UTC(),
		Metadata:  make(map[string]string),
	}

	// Copy metadata
	for k, v := range alert.Metadata {
		a.Metadata[k] = v
	}

	if a.Service == "" {
		a.Service = s.config.ServiceName
	}

	return s.SendAlert(ctx, a)
}

// Shutdown gracefully shuts down the notification service.
// It stops accepting new notifications and waits for pending ones to complete.
func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.isShutdown {
		s.mu.Unlock()
		return nil
	}
	s.isShutdown = true
	s.mu.Unlock()

	s.logger.Info("shutting down notification service")

	// Signal workers to stop
	close(s.stopCh)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	// Determine timeout
	timeout := s.config.ShutdownTimeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	// Close the queue safely via sync.Once to prevent double-close panics
	s.closeQueue.Do(func() { close(s.queue) })

	select {
	case <-done:
		s.logger.Info("notification service shutdown complete")
	case <-time.After(timeout):
		s.logger.Warn("notification service shutdown timed out, some notifications may be lost",
			zap.Duration("timeout", timeout))
	case <-ctx.Done():
		s.logger.Warn("notification service shutdown context cancelled")
		return ctx.Err()
	}

	// Drain any remaining items in the queue
	remaining := s.drainQueue()
	if remaining > 0 {
		s.logger.Warn("drained remaining notifications from queue",
			zap.Int("count", remaining))
	}

	return nil
}

// drainQueue drains remaining items from the queue during shutdown.
func (s *Service) drainQueue() int {
	count := 0
	for {
		select {
		case job, ok := <-s.queue:
			if !ok {
				return count
			}
			count++
			// Try to send with a short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.sendAlertSync(ctx, job.alert)
			cancel()
		default:
			return count
		}
	}
}

// prepareAlert prepares an alert with service metadata.
func (s *Service) prepareAlert(alert Alert) Alert {
	if alert.ID == "" {
		alert.ID = fmt.Sprintf("alert-%d", time.Now().UnixNano())
	}
	if alert.Service == "" && s.config.ServiceName != "" {
		alert.Service = s.config.ServiceName
	}
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now().UTC()
	}
	if alert.Metadata == nil {
		alert.Metadata = make(map[string]string)
	}
	alert.Metadata["environment"] = s.config.Environment

	return alert
}

// queueAlert adds an alert to the async queue.
// For Critical/High severity alerts, falls back to synchronous sending
// when the queue is full to ensure important alerts are never lost.
func (s *Service) queueAlert(ctx context.Context, alert Alert) error {
	s.mu.RLock()
	shutdown := s.isShutdown
	s.mu.RUnlock()

	if shutdown {
		s.metrics.droppedTotal.Inc()
		return ErrServiceShuttingDown
	}

	select {
	case s.queue <- asyncJob{
		ctx:       ctx,
		alert:     alert,
		timestamp: time.Now(),
	}:
		s.metrics.queueSize.Inc()
		return nil
	default:
		// Queue is full — fall back to sync for critical/high severity
		if alert.Severity == SeverityCritical || alert.Severity == SeverityHigh {
			s.metrics.syncFallbackTotal.Inc()
			s.logger.Warn("async queue full, sending critical/high alert synchronously",
				zap.String("alert_id", alert.ID),
				zap.String("severity", alert.Severity.String()),
				zap.String("title", alert.Title))
			syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			return s.sendAlertSync(syncCtx, alert)
		}
		s.metrics.droppedTotal.Inc()
		return ErrQueueFullService
	}
}

// sendAlertSync sends an alert synchronously to appropriate channels.
func (s *Service) sendAlertSync(ctx context.Context, alert Alert) error {
	// Determine which channels to use based on severity
	sendDiscord, sendEmail := s.routeAlert(alert.Severity)

	var discordErr, emailErr error
	var wg sync.WaitGroup
	var sentCount atomic.Int32

	// Send to Discord
	if sendDiscord && s.discord != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()

			start := time.Now()
			err := s.discord.SendAlert(ctx, alert)
			duration := time.Since(start)

			s.metrics.sendDuration.WithLabelValues("discord").Observe(duration.Seconds())

			if err != nil {
				discordErr = err
				s.metrics.sentTotal.WithLabelValues("discord", alert.Type.String(), "failure").Inc()
				s.recordError("discord", err)
			} else {
				s.metrics.sentTotal.WithLabelValues("discord", alert.Type.String(), "success").Inc()
				sentCount.Add(1)
			}
		}()
	}

	// Send to Email
	if sendEmail && s.email != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Set email recipients from alert metadata or config
			alertWithRecipients := alert
			if _, ok := alertWithRecipients.Metadata["email_recipients"]; !ok {
				if len(s.config.EmailRecipients) > 0 {
					if alertWithRecipients.Metadata == nil {
						alertWithRecipients.Metadata = make(map[string]string)
					}
					recipients := ""
					for i, r := range s.config.EmailRecipients {
						if i > 0 {
							recipients += ","
						}
						recipients += r
					}
					alertWithRecipients.Metadata["email_recipients"] = recipients
				}
			}

			start := time.Now()
			err := s.email.SendAlert(ctx, alertWithRecipients)
			duration := time.Since(start)

			s.metrics.sendDuration.WithLabelValues("email").Observe(duration.Seconds())

			if err != nil {
				emailErr = err
				s.metrics.sentTotal.WithLabelValues("email", alert.Type.String(), "failure").Inc()
				s.recordError("email", err)
			} else {
				s.metrics.sentTotal.WithLabelValues("email", alert.Type.String(), "success").Inc()
				sentCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// Log result
	s.logger.Info("notification sent",
		zap.String("alert_id", alert.ID),
		zap.String("type", alert.Type.String()),
		zap.String("severity", alert.Severity.String()),
		zap.Bool("discord", sendDiscord && s.discord != nil),
		zap.Bool("email", sendEmail && s.email != nil),
		zap.Int32("channels_succeeded", sentCount.Load()))

	// Return first error encountered
	if discordErr != nil {
		return discordErr
	}
	if emailErr != nil {
		return emailErr
	}

	return nil
}

// routeAlert determines which channels to use based on severity and environment.
// Returns (sendDiscord, sendEmail).
func (s *Service) routeAlert(severity Severity) (bool, bool) {
	switch severity {
	case SeverityCritical, SeverityHigh:
		// Critical/High: Both Discord AND Email
		return true, true
	case SeverityMedium:
		// Medium: Discord only
		return true, false
	case SeverityLow, SeverityInfo:
		// Low/Info: Discord only, but skip in non-production
		if s.config.Environment != "production" {
			return false, false
		}
		return true, false
	default:
		return true, false
	}
}

// recordError records an error to metrics.
func (s *Service) recordError(channel string, err error) {
	errorType := "unknown"

	switch {
	case errors.Is(err, ErrDiscordRateLimited):
		errorType = "rate_limited"
	case errors.Is(err, ErrDiscordCircuitOpen):
		errorType = "circuit_open"
	case errors.Is(err, ErrEmailCircuitOpen):
		errorType = "circuit_open"
	case errors.Is(err, ErrEmailNoRecipients):
		errorType = "no_recipients"
	case errors.Is(err, context.DeadlineExceeded):
		errorType = "timeout"
	case errors.Is(err, context.Canceled):
		errorType = "cancelled"
	default:
		errorType = "send_failed"
	}

	s.metrics.errorsTotal.WithLabelValues(channel, errorType).Inc()
}

// QueueSize returns the current size of the async queue.
func (s *Service) QueueSize() int {
	return len(s.queue)
}

// IsEnabled returns whether the service is enabled.
func (s *Service) IsEnabled() bool {
	return s.config.Enabled
}

// HasDiscord returns whether Discord is configured.
func (s *Service) HasDiscord() bool {
	return s.discord != nil
}

// HasEmail returns whether Email is configured.
func (s *Service) HasEmail() bool {
	return s.email != nil
}

// GetEmailNotifier returns the underlying EmailNotifier for direct email operations.
// Returns nil if email is not configured.
func (s *Service) GetEmailNotifier() *EmailNotifier {
	return s.email
}

// DiscordCircuitBreakerState returns the Discord circuit breaker state.
func (s *Service) DiscordCircuitBreakerState() string {
	if s.discord == nil {
		return "disabled"
	}
	return s.discord.CircuitBreakerState().String()
}

// EmailCircuitBreakerState returns the Email circuit breaker state.
func (s *Service) EmailCircuitBreakerState() string {
	if s.email == nil {
		return "disabled"
	}
	return s.email.CircuitBreakerState().String()
}

// ResetDiscordCircuitBreaker resets the Discord circuit breaker.
func (s *Service) ResetDiscordCircuitBreaker() {
	if s.discord != nil {
		s.discord.ResetCircuitBreaker()
	}
}

// ResetEmailCircuitBreaker resets the Email circuit breaker.
func (s *Service) ResetEmailCircuitBreaker() {
	if s.email != nil {
		s.email.ResetCircuitBreaker()
	}
}

// MetricsCollectors returns all Prometheus collectors for registration.
func (s *Service) MetricsCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		s.metrics.sentTotal,
		s.metrics.sendDuration,
		s.metrics.queueSize,
		s.metrics.errorsTotal,
		s.metrics.workersActive,
		s.metrics.droppedTotal,
		s.metrics.syncFallbackTotal,
	}
}
