package notification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/resilience/circuitbreaker"
	"github.com/gtuk/discordwebhook"
	"go.uber.org/zap"
)

// Discord embed color codes based on severity.
const (
	ColorCritical = 0xFF0000 // Red
	ColorHigh     = 0xFF6600 // Orange
	ColorMedium   = 0xFFFF00 // Yellow
	ColorLow      = 0x00FF00 // Green
	ColorInfo     = 0x0099FF // Blue
)

// Discord rate limit: 30 messages per minute.
const discordRateLimitPerMinute = 30

// Common errors for Discord notifier.
var (
	ErrDiscordWebhookEmpty = errors.New("discord: webhook URL is empty")
	ErrDiscordRateLimited  = errors.New("discord: rate limit exceeded")
	ErrDiscordCircuitOpen  = errors.New("discord: circuit breaker is open")
)

// DiscordEmbed represents a Discord embed message.
type DiscordEmbed struct {
	Title       string
	Description string
	Color       int
	Fields      []DiscordEmbedField
	Footer      string
	Timestamp   time.Time
}

// DiscordEmbedField represents a field in a Discord embed.
type DiscordEmbedField struct {
	Name   string
	Value  string
	Inline bool
}

// DiscordNotifier sends notifications to Discord via webhooks.
type DiscordNotifier struct {
	webhookURL     string
	username       string
	avatarURL      string
	logger         *zap.Logger
	circuitBreaker *circuitbreaker.CircuitBreaker
	rateLimiter    *discordRateLimiter
	hostname       string
}

// discordRateLimiter implements a token bucket rate limiter for Discord's 30 msg/min limit.
type discordRateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// newDiscordRateLimiter creates a new rate limiter for Discord.
func newDiscordRateLimiter(requestsPerMinute int) *discordRateLimiter {
	return &discordRateLimiter{
		tokens:     float64(requestsPerMinute),
		maxTokens:  float64(requestsPerMinute),
		refillRate: float64(requestsPerMinute) / 60.0, // tokens per second
		lastRefill: time.Now(),
	}
}

// allow checks if a request is allowed and consumes a token.
func (r *discordRateLimiter) allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastRefill).Seconds()

	// Refill tokens based on elapsed time
	r.tokens += elapsed * r.refillRate
	if r.tokens > r.maxTokens {
		r.tokens = r.maxTokens
	}
	r.lastRefill = now

	if r.tokens >= 1 {
		r.tokens--
		return true
	}
	return false
}

// NewDiscordNotifier creates a new Discord notifier.
func NewDiscordNotifier(cfg DiscordConfig, logger *zap.Logger) (*DiscordNotifier, error) {
	if cfg.WebhookURL == "" {
		return nil, ErrDiscordWebhookEmpty
	}

	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			return nil, fmt.Errorf("discord: failed to create logger: %w", err)
		}
	}
	logger = ensureRedactingLogger(logger)
	logger = logger.With(zap.String("component", "discord-notifier"))

	// Get hostname for footer
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	// Create circuit breaker with specified config
	cb := circuitbreaker.New(circuitbreaker.Config{
		Name:         "discord-webhook",
		MaxFailures:  3,
		ResetTimeout: 60 * time.Second,
		Timeout:      10 * time.Second,
		OnStateChange: func(name string, from, to circuitbreaker.State) {
			logger.Info("circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		},
	})

	// Apply defaults
	username := cfg.Username
	if username == "" {
		username = "tragge-notifications"
	}

	return &DiscordNotifier{
		webhookURL:     cfg.WebhookURL,
		username:       username,
		avatarURL:      cfg.AvatarURL,
		logger:         logger,
		circuitBreaker: cb,
		rateLimiter:    newDiscordRateLimiter(discordRateLimitPerMinute),
		hostname:       hostname,
	}, nil
}

// Channel returns the channel type (implements ChannelSender).
func (d *DiscordNotifier) Channel() Channel {
	return ChannelDiscord
}

// SendAlert sends an alert to Discord (implements ChannelSender).
func (d *DiscordNotifier) SendAlert(ctx context.Context, alert Alert) error {
	embed := d.alertToEmbed(alert)
	return d.SendEmbed(ctx, embed)
}

// SendInfo sends an info notification to Discord (implements ChannelSender).
func (d *DiscordNotifier) SendInfo(ctx context.Context, info Info) error {
	embed := d.infoToEmbed(info)
	return d.SendEmbed(ctx, embed)
}

// SendMessage sends a plain text message to Discord.
func (d *DiscordNotifier) SendMessage(ctx context.Context, content string) error {
	// Check rate limit
	if !d.rateLimiter.allow() {
		d.logger.Warn("rate limit exceeded for Discord message")
		return ErrDiscordRateLimited
	}

	// Execute through circuit breaker
	err := d.circuitBreaker.ExecuteWithContext(ctx, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		username := d.username
		message := discordwebhook.Message{
			Username: &username,
			Content:  &content,
		}

		if d.avatarURL != "" {
			message.AvatarUrl = &d.avatarURL
		}

		return discordwebhook.SendMessage(d.webhookURL, message)
	})

	if err != nil {
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
			d.logger.Warn("circuit breaker is open, skipping Discord message")
			return ErrDiscordCircuitOpen
		}
		d.logger.Error("failed to send Discord message", zap.Error(err))
		return fmt.Errorf("discord: failed to send message: %w", err)
	}

	return nil
}

// SendEmbed sends a rich embed message to Discord.
func (d *DiscordNotifier) SendEmbed(ctx context.Context, embed DiscordEmbed) error {
	// Check rate limit
	if !d.rateLimiter.allow() {
		d.logger.Warn("rate limit exceeded for Discord embed")
		return ErrDiscordRateLimited
	}

	// Execute through circuit breaker
	err := d.circuitBreaker.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return d.sendEmbedInternal(embed)
	})

	if err != nil {
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
			d.logger.Warn("circuit breaker is open, skipping Discord embed")
			return ErrDiscordCircuitOpen
		}
		d.logger.Error("failed to send Discord embed",
			zap.String("title", embed.Title),
			zap.Error(err))
		return fmt.Errorf("discord: failed to send embed: %w", err)
	}

	return nil
}

// sendEmbedInternal sends the embed without rate limiting or circuit breaker.
func (d *DiscordNotifier) sendEmbedInternal(embed DiscordEmbed) error {
	// Build Discord embed fields
	var fields []discordwebhook.Field
	for _, f := range embed.Fields {
		name := f.Name
		value := f.Value
		inline := f.Inline
		fields = append(fields, discordwebhook.Field{
			Name:   &name,
			Value:  &value,
			Inline: &inline,
		})
	}

	// Build footer with timestamp included
	var footer *discordwebhook.Footer
	footerText := embed.Footer
	if !embed.Timestamp.IsZero() {
		ts := embed.Timestamp.Format(time.RFC3339)
		if footerText != "" {
			footerText = fmt.Sprintf("%s | %s", footerText, ts)
		} else {
			footerText = ts
		}
	}
	if footerText != "" {
		footer = &discordwebhook.Footer{
			Text: &footerText,
		}
	}

	// Build embed
	title := embed.Title
	description := embed.Description
	// Color needs to be a string representation of the decimal color code
	color := fmt.Sprintf("%d", embed.Color)

	discordEmbed := discordwebhook.Embed{
		Title:       &title,
		Description: &description,
		Color:       &color,
		Fields:      &fields,
		Footer:      footer,
	}

	// Build message
	username := d.username
	message := discordwebhook.Message{
		Username: &username,
		Embeds:   &[]discordwebhook.Embed{discordEmbed},
	}

	if d.avatarURL != "" {
		message.AvatarUrl = &d.avatarURL
	}

	return discordwebhook.SendMessage(d.webhookURL, message)
}

// SendBugAlert sends a bug alert to Discord with a rich embed.
func (d *DiscordNotifier) SendBugAlert(ctx context.Context, alert Alert) error {
	if alert.Type == "" {
		alert.Type = AlertTypeBug
	}
	return d.SendAlert(ctx, alert)
}

// SendSystemAlert sends a system alert to Discord with a rich embed.
func (d *DiscordNotifier) SendSystemAlert(ctx context.Context, alert Alert) error {
	if alert.Type == "" {
		alert.Type = AlertTypeSystem
	}
	return d.SendAlert(ctx, alert)
}

// SendContestAlert sends a contest alert to Discord with a rich embed.
func (d *DiscordNotifier) SendContestAlert(ctx context.Context, alert Alert) error {
	if alert.Type == "" {
		alert.Type = AlertTypeContest
	}
	return d.SendAlert(ctx, alert)
}

// alertToEmbed converts an Alert to a DiscordEmbed.
func (d *DiscordNotifier) alertToEmbed(alert Alert) DiscordEmbed {
	embed := DiscordEmbed{
		Title:       fmt.Sprintf("[%s] %s", severityEmoji(alert.Severity), alert.Title),
		Description: alert.Message,
		Color:       severityToColor(alert.Severity),
		Timestamp:   alert.Timestamp,
		Footer:      fmt.Sprintf("Host: %s", d.hostname),
	}

	// Add standard fields
	var fields []DiscordEmbedField

	// Add type field
	fields = append(fields, DiscordEmbedField{
		Name:   "Type",
		Value:  alert.Type.String(),
		Inline: true,
	})

	// Add severity field
	fields = append(fields, DiscordEmbedField{
		Name:   "Severity",
		Value:  alert.Severity.String(),
		Inline: true,
	})

	// Add service field if present
	if alert.Service != "" {
		fields = append(fields, DiscordEmbedField{
			Name:   "Service",
			Value:  alert.Service,
			Inline: true,
		})
	}

	// Add environment from metadata
	if env, ok := alert.Metadata["environment"]; ok {
		fields = append(fields, DiscordEmbedField{
			Name:   "Environment",
			Value:  env,
			Inline: true,
		})
	}

	// Add shard ID from metadata
	if shardID, ok := alert.Metadata["shard_id"]; ok {
		fields = append(fields, DiscordEmbedField{
			Name:   "Shard ID",
			Value:  shardID,
			Inline: true,
		})
	}

	// Add trace ID if present
	if alert.TraceID != "" {
		fields = append(fields, DiscordEmbedField{
			Name:   "Trace ID",
			Value:  fmt.Sprintf("`%s`", alert.TraceID),
			Inline: false,
		})
	}

	// Add other metadata fields
	for key, value := range alert.Metadata {
		// Skip already processed fields
		if key == "environment" || key == "shard_id" {
			continue
		}
		fields = append(fields, DiscordEmbedField{
			Name:   formatFieldName(key),
			Value:  value,
			Inline: true,
		})
	}

	embed.Fields = fields
	return embed
}

// infoToEmbed converts an Info to a DiscordEmbed.
func (d *DiscordNotifier) infoToEmbed(info Info) DiscordEmbed {
	embed := DiscordEmbed{
		Title:       info.Title,
		Description: info.Message,
		Color:       ColorInfo,
		Timestamp:   info.Timestamp,
		Footer:      fmt.Sprintf("Host: %s", d.hostname),
	}

	var fields []DiscordEmbedField

	// Add service field if present
	if info.Service != "" {
		fields = append(fields, DiscordEmbedField{
			Name:   "Service",
			Value:  info.Service,
			Inline: true,
		})
	}

	// Add metadata fields
	for key, value := range info.Metadata {
		fields = append(fields, DiscordEmbedField{
			Name:   formatFieldName(key),
			Value:  value,
			Inline: true,
		})
	}

	embed.Fields = fields
	return embed
}

// severityToColor returns the Discord embed color for a severity level.
func severityToColor(severity Severity) int {
	switch severity {
	case SeverityCritical:
		return ColorCritical
	case SeverityHigh:
		return ColorHigh
	case SeverityMedium:
		return ColorMedium
	case SeverityLow:
		return ColorLow
	case SeverityInfo:
		return ColorInfo
	default:
		return ColorInfo
	}
}

// severityEmoji returns an emoji for a severity level.
func severityEmoji(severity Severity) string {
	switch severity {
	case SeverityCritical:
		return "CRITICAL"
	case SeverityHigh:
		return "HIGH"
	case SeverityMedium:
		return "MEDIUM"
	case SeverityLow:
		return "LOW"
	case SeverityInfo:
		return "INFO"
	default:
		return "INFO"
	}
}

// formatFieldName formats a metadata key as a field name.
func formatFieldName(key string) string {
	// Convert snake_case to Title Case
	result := ""
	capitalizeNext := true
	for _, c := range key {
		if c == '_' {
			result += " "
			capitalizeNext = true
		} else if capitalizeNext {
			if c >= 'a' && c <= 'z' {
				result += string(c - 32) // Convert to uppercase
			} else {
				result += string(c)
			}
			capitalizeNext = false
		} else {
			result += string(c)
		}
	}
	return result
}

// CircuitBreakerState returns the current state of the circuit breaker.
func (d *DiscordNotifier) CircuitBreakerState() circuitbreaker.State {
	return d.circuitBreaker.State()
}

// CircuitBreakerMetrics returns metrics from the circuit breaker.
func (d *DiscordNotifier) CircuitBreakerMetrics() circuitbreaker.Metrics {
	return d.circuitBreaker.Metrics()
}

// ResetCircuitBreaker resets the circuit breaker to closed state.
func (d *DiscordNotifier) ResetCircuitBreaker() {
	d.circuitBreaker.Reset()
}
