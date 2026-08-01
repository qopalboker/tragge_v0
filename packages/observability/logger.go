package observability

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LogConfig holds configuration for the logger.
type LogConfig struct {
	// Service is the name of the service (required)
	Service string
	// Env is the environment (e.g., "production", "staging", "development")
	Env string
	// Version is the service version
	Version string
	// Level is the minimum log level (default: "info")
	Level string
	// Development enables development mode (pretty printing, stack traces)
	Development bool
}

// Logger wraps zap.Logger with additional context-aware methods.
type Logger struct {
	*zap.Logger
	service string
	env     string
	version string
}

// NewLogger creates a new structured JSON logger.
func NewLogger(cfg LogConfig) (*Logger, error) {
	// Parse log level
	level := zapcore.InfoLevel
	if cfg.Level != "" {
		if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
			return nil, err
		}
	}

	// Build encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create encoder based on development mode
	var encoder zapcore.Encoder
	if cfg.Development {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Create core
	core := NewRedactingCore(zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		level,
	))

	// Build logger with base fields
	baseFields := []zap.Field{
		zap.String("service", cfg.Service),
	}
	if cfg.Env != "" {
		baseFields = append(baseFields, zap.String("env", cfg.Env))
	}
	if cfg.Version != "" {
		baseFields = append(baseFields, zap.String("version", cfg.Version))
	}

	zapLogger := zap.New(core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.Fields(baseFields...),
	)

	if cfg.Development {
		zapLogger = zapLogger.WithOptions(zap.AddStacktrace(zapcore.ErrorLevel))
	}

	return &Logger{
		Logger:  zapLogger,
		service: cfg.Service,
		env:     cfg.Env,
		version: cfg.Version,
	}, nil
}

// WithContext returns a logger with trace context fields extracted.
func (l *Logger) WithContext(ctx context.Context) *zap.Logger {
	fields := extractContextFields(ctx)
	return l.Logger.With(fields...)
}

// WithTraceID returns a logger with the given trace ID.
func (l *Logger) WithTraceID(traceID string) *zap.Logger {
	return l.Logger.With(zap.String("trace_id", traceID))
}

// WithUserID returns a logger with the given user ID.
func (l *Logger) WithUserID(userID string) *zap.Logger {
	return l.Logger.With(zap.String("user_id", userID))
}

// WithContestID returns a logger with the given contest ID.
func (l *Logger) WithContestID(contestID string) *zap.Logger {
	return l.Logger.With(zap.String("contest_id", contestID))
}

// WithOrderID returns a logger with the given order ID.
func (l *Logger) WithOrderID(orderID string) *zap.Logger {
	return l.Logger.With(zap.String("order_id", orderID))
}

// WithSymbol returns a logger with the given symbol.
func (l *Logger) WithSymbol(symbol string) *zap.Logger {
	return l.Logger.With(zap.String("symbol", symbol))
}

// WithFields returns a logger with the given fields.
func (l *Logger) WithFields(fields ...zap.Field) *zap.Logger {
	return l.Logger.With(fields...)
}

// Context keys for logging fields
type contextKey string

const (
	UserIDKey    contextKey = "user_id"
	ContestIDKey contextKey = "contest_id"
	OrderIDKey   contextKey = "order_id"
	SymbolKey    contextKey = "symbol"
	RequestIDKey contextKey = "request_id"
)

// ContextWithUserID adds a user ID to the context.
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// ContextWithContestID adds a contest ID to the context.
func ContextWithContestID(ctx context.Context, contestID string) context.Context {
	return context.WithValue(ctx, ContestIDKey, contestID)
}

// ContextWithOrderID adds an order ID to the context.
func ContextWithOrderID(ctx context.Context, orderID string) context.Context {
	return context.WithValue(ctx, OrderIDKey, orderID)
}

// ContextWithSymbol adds a symbol to the context.
func ContextWithSymbol(ctx context.Context, symbol string) context.Context {
	return context.WithValue(ctx, SymbolKey, symbol)
}

// ContextWithRequestID adds a request ID to the context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetUserID extracts the user ID from context.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(UserIDKey).(string); ok {
		return v
	}
	return ""
}

// GetContestID extracts the contest ID from context.
func GetContestID(ctx context.Context) string {
	if v, ok := ctx.Value(ContestIDKey).(string); ok {
		return v
	}
	return ""
}

// GetOrderID extracts the order ID from context.
func GetOrderID(ctx context.Context) string {
	if v, ok := ctx.Value(OrderIDKey).(string); ok {
		return v
	}
	return ""
}

// GetSymbol extracts the symbol from context.
func GetSymbol(ctx context.Context) string {
	if v, ok := ctx.Value(SymbolKey).(string); ok {
		return v
	}
	return ""
}

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(RequestIDKey).(string); ok {
		return v
	}
	return ""
}

// extractContextFields extracts logging fields from context.
func extractContextFields(ctx context.Context) []zap.Field {
	var fields []zap.Field

	// Extract trace ID from OpenTelemetry span context
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.HasTraceID() {
		fields = append(fields, zap.String("trace_id", spanCtx.TraceID().String()))
	}
	if spanCtx.HasSpanID() {
		fields = append(fields, zap.String("span_id", spanCtx.SpanID().String()))
	}

	// Extract custom context fields
	if userID := GetUserID(ctx); userID != "" {
		fields = append(fields, zap.String("user_id", userID))
	}
	if contestID := GetContestID(ctx); contestID != "" {
		fields = append(fields, zap.String("contest_id", contestID))
	}
	if orderID := GetOrderID(ctx); orderID != "" {
		fields = append(fields, zap.String("order_id", orderID))
	}
	if symbol := GetSymbol(ctx); symbol != "" {
		fields = append(fields, zap.String("symbol", symbol))
	}
	if requestID := GetRequestID(ctx); requestID != "" {
		fields = append(fields, zap.String("request_id", requestID))
	}

	return fields
}

// Sync flushes any buffered log entries.
func (l *Logger) Sync() error {
	return l.Logger.Sync()
}
