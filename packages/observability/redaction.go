package observability

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

var standardLoggerMu sync.Mutex

// RedactedValue is stable and intentionally preserves no reconstruction aid.
const RedactedValue = "[REDACTED]"

var (
	jwtPattern           = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}(?:\.[A-Za-z0-9_-]{8,})?\b`)
	authPattern          = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[^\s,;]+`)
	credentialURLPattern = regexp.MustCompile(`(?i)\b((?:postgres(?:ql)?|redis(?:s)?|https?)://)[^\s/@:]*(?::[^\s/@]*)?@`)
	privateKeyPattern    = regexp.MustCompile(`(?s)-----BEGIN [^-\r\n]*PRIVATE KEY-----.*?-----END [^-\r\n]*PRIVATE KEY-----`)
	assignmentPattern    = regexp.MustCompile(`(?i)\b(authorization|cookie|password|passwd|passphrase|access[-_]?token|refresh[-_]?token|session[-_]?token|jwt|api[-_]?key|client[-_]?secret|private[-_]?key|provider[-_]?secret|webhook[-_]?secret|webhook[-_]?signature|csrf[-_]?token|otp|verification[-_]?code|reset[-_]?code|reset[-_]?token|security[-_]?code|national[-_]?code|reauth(?:entication)?[-_]?grant|ticket)\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;&}]+)`)
)

// IsSensitiveKey identifies structured fields that must never reach a sink.
func IsSensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(key)
	for _, fragment := range []string{
		"authorization", "cookie", "password", "passwd", "passphrase", "token", "jwt",
		"secret", "api_key", "private_key", "encryption_key", "signature", "otp", "code",
		"grant", "ticket", "request_body", "response_body", "payment_payload", "kyc_document",
		"document_bytes", "email", "phone", "national", "full_name", "first_name", "last_name",
		"iban", "card_number", "account_number",
	} {
		if key == fragment || strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

// RedactText removes credentials embedded in messages, errors, URLs, and panics.
func RedactText(value string) string {
	value = privateKeyPattern.ReplaceAllString(value, RedactedValue)
	value = jwtPattern.ReplaceAllString(value, RedactedValue)
	value = authPattern.ReplaceAllStringFunc(value, func(match string) string {
		return strings.Fields(match)[0] + " " + RedactedValue
	})
	value = credentialURLPattern.ReplaceAllString(value, `${1}`+RedactedValue+`@`)
	return assignmentPattern.ReplaceAllStringFunc(value, func(match string) string {
		if separator := strings.IndexAny(match, ":="); separator >= 0 {
			return strings.TrimSpace(match[:separator]) + "=" + RedactedValue
		}
		return RedactedValue
	})
}

func RedactError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(RedactText(err.Error()))
}

func RedactPanic(value any) string { return RedactText(fmt.Sprint(value)) }

type redactingWriter struct{ destination io.Writer }

func (writer *redactingWriter) Write(data []byte) (int, error) {
	_, err := io.WriteString(writer.destination, RedactText(string(data)))
	return len(data), err
}

// NewRedactingWriter protects standard-library and transitional text sinks.
func NewRedactingWriter(destination io.Writer) io.Writer {
	if destination == nil {
		destination = io.Discard
	}
	if _, ok := destination.(*redactingWriter); ok {
		return destination
	}
	return &redactingWriter{destination: destination}
}

// InstallStandardLoggerRedaction protects package-level log calls in the
// current process while legacy call sites migrate to structured logging.
func InstallStandardLoggerRedaction() {
	standardLoggerMu.Lock()
	defer standardLoggerMu.Unlock()
	if _, ok := log.Writer().(*redactingWriter); !ok {
		log.SetOutput(NewRedactingWriter(log.Writer()))
	}
}

// RedactHeaders copies headers and removes credentials before telemetry sees them.
func RedactHeaders(headers http.Header) http.Header {
	if headers == nil {
		return nil
	}
	result := headers.Clone()
	for key, values := range result {
		if IsSensitiveKey(key) {
			result[key] = []string{RedactedValue}
			continue
		}
		for i := range values {
			result[key][i] = RedactText(values[i])
		}
	}
	return result
}

// RedactURL copies a URL and removes user-info and credential query values.
func RedactURL(input *url.URL) *url.URL {
	if input == nil {
		return nil
	}
	result := *input
	if result.User != nil {
		result.User = url.User(RedactedValue)
	}
	query := result.Query()
	for key, values := range query {
		if IsSensitiveKey(key) {
			query[key] = []string{RedactedValue}
			continue
		}
		for i := range values {
			query[key][i] = RedactText(values[i])
		}
	}
	result.RawQuery = query.Encode()
	return &result
}

// RedactValue recursively sanitizes structured values before logging,
// telemetry emission, or audit persistence.
func RedactValue(value any) any { return redactValue(value, 0) }

func redactValue(value any, depth int) any {
	if value == nil {
		return nil
	}
	if depth > 12 {
		return RedactedValue
	}
	switch typed := value.(type) {
	case string:
		return RedactText(typed)
	case []byte:
		return RedactText(string(typed))
	case error:
		return RedactText(typed.Error())
	case http.Header:
		return RedactHeaders(typed)
	case *url.URL:
		return RedactURL(typed)
	case url.URL:
		return RedactURL(&typed)
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if IsSensitiveKey(key) {
				result[key] = RedactedValue
			} else {
				result[key] = redactValue(item, depth+1)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i := range typed {
			result[i] = redactValue(typed[i], depth+1)
		}
		return result
	}
	reflected := reflect.ValueOf(value)
	if reflected.IsValid() && isPlainScalar(reflected.Kind()) {
		return value
	}
	if encoded, err := json.Marshal(value); err == nil {
		var decoded any
		if json.Unmarshal(encoded, &decoded) == nil {
			return redactValue(decoded, depth+1)
		}
	}
	return RedactText(fmt.Sprint(value))
}

func isPlainScalar(kind reflect.Kind) bool {
	switch kind {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}
