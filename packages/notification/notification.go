// Package notification provides a notification service for sending alerts and
// informational messages through various channels (Discord, Email).
//
// The package supports:
//   - Multiple notification channels (Discord webhooks, Resend email)
//   - Severity-based filtering (critical, high, medium, low, info)
//   - Both synchronous and asynchronous sending
//   - Rate limiting to prevent notification spam
//   - Graceful shutdown with queue draining
//
// Basic usage:
//
//	svc, err := notification.New(notification.Config{
//	    Service: "my-service",
//	    Discord: notification.DiscordConfig{
//	        WebhookURL: "https://discord.com/api/webhooks/...",
//	    },
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer svc.Shutdown(context.Background())
//
//	// Send an alert
//	alert := notification.SystemAlert(notification.SeverityHigh, "Service Degraded", "High latency detected")
//	err = svc.SendAlert(ctx, alert)
//
//	// Send info
//	info := notification.NewInfo("Deployment Complete", "Version 1.2.3 deployed successfully")
//	err = svc.SendInfo(ctx, info)
package notification

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Common errors returned by the notification package.
var (
	ErrNotEnabled       = errors.New("notification: service not enabled")
	ErrNoChannels       = errors.New("notification: no channels configured")
	ErrQueueFull        = errors.New("notification: async queue is full")
	ErrShuttingDown     = errors.New("notification: service is shutting down")
	ErrInvalidSeverity  = errors.New("notification: invalid severity level")
	ErrInvalidRateLimit = errors.New("notification: invalid rate limit")
	ErrInvalidQueueSize = errors.New("notification: invalid queue size")
	ErrRateLimited      = errors.New("notification: rate limit exceeded")
)

// Notifier defines the interface for sending notifications.
//
// Deprecated: Use *Service (from NewService) instead, which provides worker pools,
// Prometheus metrics, severity-based routing, and sync fallback for critical alerts.
type Notifier interface {
	// SendAlert sends an alert notification through configured channels.
	// If async mode is enabled, this returns immediately after queuing.
	// Returns ErrNotEnabled if the service is disabled.
	// Returns ErrRateLimited if rate limit is exceeded.
	SendAlert(ctx context.Context, alert Alert) error

	// SendInfo sends an informational notification through configured channels.
	// If async mode is enabled, this returns immediately after queuing.
	// Returns ErrNotEnabled if the service is disabled.
	// Returns ErrRateLimited if rate limit is exceeded.
	SendInfo(ctx context.Context, info Info) error

	// SendAlertSync sends an alert synchronously, ignoring async setting.
	// Blocks until the notification is sent or context is cancelled.
	SendAlertSync(ctx context.Context, alert Alert) error

	// SendInfoSync sends info synchronously, ignoring async setting.
	// Blocks until the notification is sent or context is cancelled.
	SendInfoSync(ctx context.Context, info Info) error

	// Shutdown gracefully shuts down the notification service.
	// It drains the async queue and waits for pending notifications.
	Shutdown(ctx context.Context) error
}

// ChannelSender defines the interface for individual notification channels.
type ChannelSender interface {
	// SendAlert sends an alert through this channel.
	SendAlert(ctx context.Context, alert Alert) error

	// SendInfo sends info through this channel.
	SendInfo(ctx context.Context, info Info) error

	// Channel returns the channel type.
	Channel() Channel
}

// notificationItem represents an item in the async queue.
type notificationItem struct {
	isAlert bool
	alert   Alert
	info    Info
}

// NotificationService implements the Notifier interface.
//
// Deprecated: Use *Service (from NewService) instead. NotificationService uses a single
// async worker and lacks Prometheus metrics. See ConfigToServiceConfig for migration.
type NotificationService struct {
	config  Config
	logger  *zap.Logger
	senders []ChannelSender

	// Async queue
	queue    chan notificationItem
	wg       sync.WaitGroup
	shutdown chan struct{}
	done     chan struct{}

	// Rate limiting
	rateLimiter *rateLimiter

	mu         sync.RWMutex
	isShutdown bool
}

// New creates a new NotificationService with the given configuration.
//
// Deprecated: Use NewService with ServiceConfig instead for worker pools and metrics.
func New(cfg Config) (*NotificationService, error) {
	return NewWithLogger(cfg, nil)
}

// NewWithLogger creates a new NotificationService with a custom logger.
//
// Deprecated: Use NewService with ServiceConfig instead for worker pools and metrics.
func NewWithLogger(cfg Config, logger *zap.Logger) (*NotificationService, error) {
	// Apply defaults
	cfg = applyDefaults(cfg)

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Create logger if not provided
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, err
		}
	}
	logger = ensureRedactingLogger(logger)
	logger = logger.With(zap.String("component", "notification"))

	svc := &NotificationService{
		config:      cfg,
		logger:      logger,
		senders:     make([]ChannelSender, 0),
		queue:       make(chan notificationItem, cfg.QueueSize),
		shutdown:    make(chan struct{}),
		done:        make(chan struct{}),
		rateLimiter: newRateLimiter(cfg.RateLimitPerMinute),
	}

	// Start async worker if enabled
	if cfg.Async {
		svc.wg.Add(1)
		go svc.asyncWorker()
	}

	return svc, nil
}

// RegisterSender registers a channel sender with the service.
func (s *NotificationService) RegisterSender(sender ChannelSender) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.senders = append(s.senders, sender)
	s.logger.Info("registered notification channel",
		zap.String("channel", sender.Channel().String()))
}

// SendAlert sends an alert notification.
func (s *NotificationService) SendAlert(ctx context.Context, alert Alert) error {
	if !s.config.Enabled {
		return ErrNotEnabled
	}

	// Check severity filter
	if !s.shouldSend(alert.Severity) {
		return nil
	}

	// Apply rate limiting
	if !s.rateLimiter.allow() {
		s.logger.Warn("rate limit exceeded for alert",
			zap.String("title", alert.Title),
			zap.String("severity", alert.Severity.String()))
		return ErrRateLimited
	}

	// Ensure ID and service are set
	alert = s.prepareAlert(alert)

	if s.config.Async {
		return s.queueNotification(notificationItem{isAlert: true, alert: alert})
	}
	return s.sendAlertToChannels(ctx, alert)
}

// SendInfo sends an informational notification.
func (s *NotificationService) SendInfo(ctx context.Context, info Info) error {
	if !s.config.Enabled {
		return ErrNotEnabled
	}

	// Apply rate limiting
	if !s.rateLimiter.allow() {
		s.logger.Warn("rate limit exceeded for info",
			zap.String("title", info.Title))
		return ErrRateLimited
	}

	// Ensure ID and service are set
	info = s.prepareInfo(info)

	if s.config.Async {
		return s.queueNotification(notificationItem{isAlert: false, info: info})
	}
	return s.sendInfoToChannels(ctx, info)
}

// SendAlertSync sends an alert synchronously.
func (s *NotificationService) SendAlertSync(ctx context.Context, alert Alert) error {
	if !s.config.Enabled {
		return ErrNotEnabled
	}

	if !s.shouldSend(alert.Severity) {
		return nil
	}

	if !s.rateLimiter.allow() {
		return ErrRateLimited
	}

	alert = s.prepareAlert(alert)
	return s.sendAlertToChannels(ctx, alert)
}

// SendInfoSync sends info synchronously.
func (s *NotificationService) SendInfoSync(ctx context.Context, info Info) error {
	if !s.config.Enabled {
		return ErrNotEnabled
	}

	if !s.rateLimiter.allow() {
		return ErrRateLimited
	}

	info = s.prepareInfo(info)
	return s.sendInfoToChannels(ctx, info)
}

// Shutdown gracefully shuts down the notification service.
func (s *NotificationService) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	if s.isShutdown {
		s.mu.Unlock()
		return nil
	}
	s.isShutdown = true
	s.mu.Unlock()

	s.logger.Info("shutting down notification service")

	// Signal shutdown
	close(s.shutdown)

	// Wait for async worker to finish with timeout
	done := make(chan struct{})
	go func() {
		defer func() { recover() }()
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("notification service shutdown complete")
		return nil
	case <-ctx.Done():
		s.logger.Warn("notification service shutdown timed out")
		return ctx.Err()
	}
}

// prepareAlert ensures the alert has required fields set.
func (s *NotificationService) prepareAlert(alert Alert) Alert {
	if alert.ID == "" {
		alert.ID = uuid.New().String()
	}
	if alert.Service == "" && s.config.Service != "" {
		alert.Service = s.config.Service
	}
	if alert.Timestamp.IsZero() {
		alert.Timestamp = time.Now().UTC()
	}
	return alert
}

// prepareInfo ensures the info has required fields set.
func (s *NotificationService) prepareInfo(info Info) Info {
	if info.ID == "" {
		info.ID = uuid.New().String()
	}
	if info.Service == "" && s.config.Service != "" {
		info.Service = s.config.Service
	}
	if info.Timestamp.IsZero() {
		info.Timestamp = time.Now().UTC()
	}
	return info
}

// shouldSend checks if a notification should be sent based on severity.
func (s *NotificationService) shouldSend(severity Severity) bool {
	return severityLevel(severity) >= severityLevel(s.config.MinSeverity)
}

// severityLevel returns a numeric level for severity comparison.
func severityLevel(s Severity) int {
	switch s {
	case SeverityCritical:
		return 5
	case SeverityHigh:
		return 4
	case SeverityMedium:
		return 3
	case SeverityLow:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

// queueNotification adds a notification to the async queue.
func (s *NotificationService) queueNotification(item notificationItem) error {
	s.mu.RLock()
	if s.isShutdown {
		s.mu.RUnlock()
		return ErrShuttingDown
	}
	s.mu.RUnlock()

	select {
	case s.queue <- item:
		return nil
	default:
		return ErrQueueFull
	}
}

// asyncWorker processes the notification queue.
func (s *NotificationService) asyncWorker() {
	defer s.wg.Done()
	defer close(s.done)

	for {
		select {
		case <-s.shutdown:
			// Drain remaining items
			s.drainQueue()
			return
		case item := <-s.queue:
			ctx, cancel := context.WithTimeout(context.Background(), s.config.Timeout)
			if item.isAlert {
				if err := s.sendAlertToChannels(ctx, item.alert); err != nil {
					s.logger.Error("failed to send alert",
						zap.String("id", item.alert.ID),
						zap.String("title", item.alert.Title),
						zap.Error(err))
				}
			} else {
				if err := s.sendInfoToChannels(ctx, item.info); err != nil {
					s.logger.Error("failed to send info",
						zap.String("id", item.info.ID),
						zap.String("title", item.info.Title),
						zap.Error(err))
				}
			}
			cancel()
		}
	}
}

// drainQueue processes remaining items in the queue during shutdown.
func (s *NotificationService) drainQueue() {
	for {
		select {
		case item := <-s.queue:
			ctx, cancel := context.WithTimeout(context.Background(), s.config.Timeout)
			if item.isAlert {
				_ = s.sendAlertToChannels(ctx, item.alert)
			} else {
				_ = s.sendInfoToChannels(ctx, item.info)
			}
			cancel()
		default:
			return
		}
	}
}

// sendAlertToChannels sends an alert to all registered channels.
func (s *NotificationService) sendAlertToChannels(ctx context.Context, alert Alert) error {
	s.mu.RLock()
	senders := s.senders
	s.mu.RUnlock()

	if len(senders) == 0 {
		s.logger.Debug("no channels configured, skipping alert",
			zap.String("id", alert.ID),
			zap.String("title", alert.Title))
		return nil
	}

	var errs []error
	for _, sender := range senders {
		if err := sender.SendAlert(ctx, alert); err != nil {
			s.logger.Error("channel send failed",
				zap.String("channel", sender.Channel().String()),
				zap.String("alert_id", alert.ID),
				zap.Error(err))
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// sendInfoToChannels sends info to all registered channels.
func (s *NotificationService) sendInfoToChannels(ctx context.Context, info Info) error {
	s.mu.RLock()
	senders := s.senders
	s.mu.RUnlock()

	if len(senders) == 0 {
		s.logger.Debug("no channels configured, skipping info",
			zap.String("id", info.ID),
			zap.String("title", info.Title))
		return nil
	}

	var errs []error
	for _, sender := range senders {
		if err := sender.SendInfo(ctx, info); err != nil {
			s.logger.Error("channel send failed",
				zap.String("channel", sender.Channel().String()),
				zap.String("info_id", info.ID),
				zap.Error(err))
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// rateLimiter implements a simple token bucket rate limiter.
type rateLimiter struct {
	mu       sync.Mutex
	limit    int
	tokens   int
	lastTick time.Time
}

// newRateLimiter creates a new rate limiter.
func newRateLimiter(limitPerMinute int) *rateLimiter {
	return &rateLimiter{
		limit:    limitPerMinute,
		tokens:   limitPerMinute,
		lastTick: time.Now(),
	}
}

// allow checks if a request is allowed and consumes a token.
func (r *rateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(r.lastTick)
	tokensToAdd := int(elapsed.Minutes() * float64(r.limit))
	if tokensToAdd > 0 {
		r.tokens = min(r.limit, r.tokens+tokensToAdd)
		r.lastTick = now
	}

	if r.tokens > 0 {
		r.tokens--
		return true
	}
	return false
}

// NoopNotifier is a no-op implementation of Notifier for testing/disabled mode.
type NoopNotifier struct{}

// SendAlert does nothing and returns nil.
func (n *NoopNotifier) SendAlert(ctx context.Context, alert Alert) error { return nil }

// SendInfo does nothing and returns nil.
func (n *NoopNotifier) SendInfo(ctx context.Context, info Info) error { return nil }

// SendAlertSync does nothing and returns nil.
func (n *NoopNotifier) SendAlertSync(ctx context.Context, alert Alert) error { return nil }

// SendInfoSync does nothing and returns nil.
func (n *NoopNotifier) SendInfoSync(ctx context.Context, info Info) error { return nil }

// Shutdown does nothing and returns nil.
func (n *NoopNotifier) Shutdown(ctx context.Context) error { return nil }

// NewNoop creates a no-op notifier that discards all notifications.
func NewNoop() Notifier {
	return &NoopNotifier{}
}
