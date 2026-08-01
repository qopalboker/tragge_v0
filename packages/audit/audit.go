// Package audit provides async security audit logging to both structured logs and database.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"github.com/Parsaeffatravesh/tragge/packages/validation"
	"go.uber.org/zap"
)

// EventType represents the type of security audit event.
type EventType string

const (
	EventLogin              EventType = "LOGIN"
	EventLoginFailed        EventType = "LOGIN_FAILED"
	EventLogout             EventType = "LOGOUT"
	EventPasswordChange     EventType = "PASSWORD_CHANGE"
	Event2FAEnable          EventType = "2FA_ENABLE"
	Event2FADisable         EventType = "2FA_DISABLE"
	EventWithdrawalRequest  EventType = "WITHDRAWAL_REQUEST"
	EventWithdrawalApproved EventType = "WITHDRAWAL_APPROVED"
	EventAdminAction        EventType = "ADMIN_ACTION"
	EventSessionRevoked     EventType = "SESSION_REVOKED"
	EventRoleChange         EventType = "ROLE_CHANGE"
)

// Entry represents a single audit log entry.
type Entry struct {
	UserID    string
	EventType EventType
	IPAddress string
	UserAgent string
	Metadata  map[string]interface{}
}

// Logger provides async security audit logging.
type Logger struct {
	db     *sql.DB
	logger *zap.Logger
	ch     chan Entry
	done   chan struct{}
}

// Config holds configuration for the audit logger.
type Config struct {
	DB            *sql.DB
	Logger        *zap.Logger
	BufferSize    int // Buffered channel size (default: 1000)
	RetentionDays int // Auto-delete logs older than this (default: 365)
}

// New creates a new audit logger with async processing.
func New(cfg Config) *Logger {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1000
	}
	if cfg.RetentionDays <= 0 {
		if v := os.Getenv("AUDIT_RETENTION_DAYS"); v != "" {
			if d, err := strconv.Atoi(v); err == nil && d > 0 {
				cfg.RetentionDays = d
			}
		}
		if cfg.RetentionDays <= 0 {
			cfg.RetentionDays = 365
		}
	}

	l := &Logger{
		db:     cfg.DB,
		logger: cfg.Logger,
		ch:     make(chan Entry, cfg.BufferSize),
		done:   make(chan struct{}),
	}

	go l.processLoop(cfg.RetentionDays)
	return l
}

// Log queues an audit entry for async processing.
// If the buffer is full, the entry is logged via zap only (not dropped silently).
func (l *Logger) Log(entry Entry) {
	entry.Metadata = sanitizedMetadata(entry.Metadata)
	entry.UserAgent = observability.RedactText(entry.UserAgent)
	// Always log to structured logger immediately
	l.logger.Info("security_audit",
		zap.String("event_type", string(entry.EventType)),
		zap.String("user_id", entry.UserID),
		zap.String("ip_address", entry.IPAddress),
		zap.Any("metadata", entry.Metadata),
	)

	// Queue for database persistence (non-blocking)
	select {
	case l.ch <- entry:
	default:
		l.logger.Warn("audit log buffer full, entry logged to zap only",
			zap.String("event_type", string(entry.EventType)))
	}
}

func sanitizedMetadata(metadata map[string]interface{}) map[string]interface{} {
	if len(metadata) == 0 {
		return metadata
	}
	if sanitized, ok := observability.RedactValue(metadata).(map[string]interface{}); ok {
		return sanitized
	}
	return map[string]interface{}{"redaction_status": observability.RedactedValue}
}

// LogFromRequest creates and queues an audit entry extracting IP and User-Agent from the request.
func (l *Logger) LogFromRequest(r *http.Request, userID string, eventType EventType, metadata map[string]interface{}) {
	ip := validation.ExtractClientIP(r)

	l.Log(Entry{
		UserID:    userID,
		EventType: eventType,
		IPAddress: ip,
		UserAgent: r.Header.Get("User-Agent"),
		Metadata:  metadata,
	})
}

// Shutdown drains the buffer and stops processing.
func (l *Logger) Shutdown() {
	close(l.ch)
	<-l.done
}

func (l *Logger) processLoop(retentionDays int) {
	defer close(l.done)

	// Run retention cleanup once at startup
	l.cleanup(retentionDays)

	// Schedule daily cleanup
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer cleanupTicker.Stop()

	for {
		select {
		case entry, ok := <-l.ch:
			if !ok {
				return // Channel closed, shutdown
			}
			l.persist(entry)
		case <-cleanupTicker.C:
			l.cleanup(retentionDays)
		}
	}
}

func (l *Logger) persist(entry Entry) {
	if l.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var metadataBytes []byte
	if len(entry.Metadata) > 0 {
		metadataBytes, _ = json.Marshal(entry.Metadata)
	}

	_, err := l.db.ExecContext(ctx,
		`INSERT INTO security_audit_log (user_id, event_type, ip_address, user_agent, metadata)
		 VALUES ($1, $2, $3::inet, $4, $5::jsonb)`,
		nilIfEmpty(entry.UserID),
		string(entry.EventType),
		nilIfEmpty(entry.IPAddress),
		nilIfEmpty(entry.UserAgent),
		nilIfEmptyBytes(metadataBytes),
	)
	if err != nil {
		l.logger.Warn("failed to persist audit log entry",
			zap.String("event_type", string(entry.EventType)),
			zap.Error(err))
	}
}

func (l *Logger) cleanup(retentionDays int) {
	if l.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := l.db.ExecContext(ctx,
		`DELETE FROM security_audit_log WHERE created_at < NOW() - make_interval(days => $1)`,
		retentionDays,
	)
	if err != nil {
		l.logger.Warn("failed to cleanup old audit logs", zap.Error(err))
		return
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		l.logger.Info("cleaned up old audit logs",
			zap.Int64("deleted", rows),
			zap.Int("retention_days", retentionDays))
	}
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nilIfEmptyBytes(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
