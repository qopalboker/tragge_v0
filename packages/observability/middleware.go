package observability

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const CorrelationIDHeader = "X-Request-ID"

var correlationFallbackCounter uint64

// HTTPMiddleware provides observability middleware for HTTP handlers.
type HTTPMiddleware struct {
	logger  *Logger
	metrics *Metrics
	tracer  *Tracer
	service string
}

// NewHTTPMiddleware creates a new HTTP middleware with the given components.
func NewHTTPMiddleware(logger *Logger, metrics *Metrics, tracer *Tracer, service string) *HTTPMiddleware {
	return &HTTPMiddleware{
		logger:  logger,
		metrics: metrics,
		tracer:  tracer,
		service: service,
	}
}

// Middleware returns an http.Handler middleware that adds tracing, metrics, and logging.
func (m *HTTPMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract trace context from incoming request
		ctx := r.Context()
		propagator := otel.GetTextMapPropagator()
		ctx = propagator.Extract(ctx, propagation.HeaderCarrier(r.Header))

		// Start span
		var span trace.Span
		if m.tracer != nil && m.tracer.tracer != nil {
			spanName := r.Method + " " + normalizePath(r.URL.Path)
			ctx, span = m.tracer.tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.HTTPRoute(r.URL.Path),
					semconv.URLScheme(scheme(r)),
					attribute.String("http.host", r.Host),
					attribute.String("http.user_agent", r.UserAgent()),
				),
			)
			defer span.End()
		}

		// Extract or generate request ID
		requestID := normalizeCorrelationID(r.Header.Get(CorrelationIDHeader))
		if requestID == "" {
			requestID = normalizeCorrelationID(TraceIDFromContext(ctx))
		}
		if requestID == "" {
			requestID = generateCorrelationID()
		}
		r.Header.Set(CorrelationIDHeader, requestID)
		w.Header().Set(CorrelationIDHeader, requestID)
		ctx = ContextWithRequestID(ctx, requestID)

		// Wrap response writer to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Update request with new context
		r = r.WithContext(ctx)

		// Call the next handler
		next.ServeHTTP(wrapped, r)

		// Calculate duration
		duration := time.Since(start)
		statusCode := wrapped.statusCode
		statusStr := strconv.Itoa(statusCode)

		// Normalize path for metrics (avoid high cardinality)
		normalizedPath := normalizePath(r.URL.Path)

		// Record metrics
		if m.metrics != nil {
			m.metrics.RecordRequest(r.Method, normalizedPath, statusStr, duration.Seconds())
		}

		// Add span attributes
		if span != nil && span.IsRecording() {
			span.SetAttributes(
				semconv.HTTPResponseStatusCode(statusCode),
				attribute.Int64("http.response_content_length", int64(wrapped.bytesWritten)),
			)

			// Mark span as error if status >= 500
			if statusCode >= 500 {
				span.SetAttributes(attribute.Bool("error", true))
			}
		}

		// Log request
		if m.logger != nil {
			fields := []zap.Field{
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", statusCode),
				zap.Duration("duration", duration),
				zap.Int64("bytes", int64(wrapped.bytesWritten)),
				zap.String("remote_addr", r.RemoteAddr),
			}

			if requestID != "" {
				fields = append(fields, zap.String("request_id", requestID))
			}

			// Add trace context
			if traceID := TraceIDFromContext(ctx); traceID != "" {
				fields = append(fields, zap.String("trace_id", traceID))
			}

			// Log at appropriate level based on status code
			switch {
			case statusCode >= 500:
				m.logger.Logger.With(fields...).Error("HTTP request")
			case statusCode >= 400:
				m.logger.Logger.With(fields...).Warn("HTTP request")
			default:
				m.logger.Logger.With(fields...).Info("HTTP request")
			}
		}
	})
}

// Recovery catches handler panics inside the observability boundary, emits only
// sanitized diagnostics, and returns a generic response to the client.
func (m *HTTPMiddleware) Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				requestID := GetRequestID(r.Context())
				if requestID == "" {
					requestID = normalizeCorrelationID(r.Header.Get(CorrelationIDHeader))
				}
				if m.logger != nil {
					m.logger.Error("HTTP handler panic recovered",
						zap.String("request_id", requestID),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
						zap.String("panic", RedactPanic(recovered)),
						zap.String("stack", RedactText(string(debug.Stack()))),
					)
				}
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func normalizeCorrelationID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 8 || len(value) > 128 || RedactText(value) != value {
		return ""
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("-_.:", character) {
			continue
		}
		return ""
	}
	return value
}

func generateCorrelationID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	sequence := atomic.AddUint64(&correlationFallbackCounter, 1)
	return fmt.Sprintf("req-%x-%x", time.Now().UnixNano(), sequence)
}

// InjectTraceContext middleware extracts user context from auth headers/context
// and injects it into the observability context.
func (m *HTTPMiddleware) InjectTraceContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Inject contest_id from header if present
		if contestID := r.Header.Get("X-Contest-ID"); contestID != "" {
			ctx = ContextWithContestID(ctx, contestID)

			// Add to span
			if span := SpanFromContext(ctx); span.IsRecording() {
				span.SetAttributes(ContestIDAttr(contestID))
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	wroteHeader  bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Implement http.Flusher for streaming responses
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Implement http.Hijacker for WebSocket upgrades
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// normalizePath normalizes a URL path for metrics to avoid high cardinality.
// It replaces UUID-like segments and numeric IDs with placeholders.
func normalizePath(path string) string {
	// Common patterns to normalize
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "" {
			continue
		}
		// Replace UUID-like strings (36 chars with hyphens)
		if len(part) == 36 && strings.Count(part, "-") == 4 {
			parts[i] = ":id"
			continue
		}
		// Replace pure numeric IDs
		if _, err := strconv.ParseInt(part, 10, 64); err == nil {
			parts[i] = ":id"
			continue
		}
		// Replace hex strings that look like IDs (24+ chars of hex)
		if len(part) >= 24 && isHex(part) {
			parts[i] = ":id"
			continue
		}
	}
	return strings.Join(parts, "/")
}

// isHex returns true if the string contains only hexadecimal characters.
func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// scheme returns the request scheme (http or https).
func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}
