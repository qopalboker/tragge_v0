package observability

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func testHTTPMiddleware(output *bytes.Buffer) *HTTPMiddleware {
	core := NewRedactingCore(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(output),
		zapcore.DebugLevel,
	))
	return NewHTTPMiddleware(&Logger{Logger: zap.New(core)}, nil, nil, "test")
}

func TestCorrelationIDPropagationAndGeneration(t *testing.T) {
	for _, test := range []struct {
		name     string
		incoming string
		wantSame bool
	}{
		{name: "valid", incoming: "request-1234", wantSame: true},
		{name: "missing"},
		{name: "unsafe", incoming: "Bearer " + syntheticCredential},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			middleware := testHTTPMiddleware(&output)
			var received string
			handler := middleware.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received = GetRequestID(r.Context())
				if received != r.Header.Get(CorrelationIDHeader) {
					t.Errorf("context/header mismatch: %q != %q", received, r.Header.Get(CorrelationIDHeader))
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodGet, "/resource?token="+syntheticCredential, nil)
			if test.incoming != "" {
				req.Header.Set(CorrelationIDHeader, test.incoming)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)

			if received == "" || recorder.Header().Get(CorrelationIDHeader) != received {
				t.Fatalf("correlation ID was not propagated: %q", received)
			}
			if test.wantSame && received != test.incoming {
				t.Fatalf("valid correlation ID changed: %q", received)
			}
			if !test.wantSame && test.incoming != "" && received == test.incoming {
				t.Fatalf("unsafe correlation ID was retained: %q", received)
			}
			if strings.Contains(received, syntheticCredential) || strings.Contains(output.String(), syntheticCredential) {
				t.Fatalf("credential influenced correlation/log output: id=%q log=%s", received, output.String())
			}
		})
	}
}

func TestRecoverySanitizesPanicAndReturnsGenericError(t *testing.T) {
	var output bytes.Buffer
	middleware := testHTTPMiddleware(&output)
	handler := middleware.Middleware(middleware.Recovery(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("password=" + syntheticCredential)
	})))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want=%d", recorder.Code, http.StatusInternalServerError)
	}
	combined := recorder.Body.String() + output.String()
	if strings.Contains(combined, syntheticCredential) {
		t.Fatalf("panic credential leaked: %s", combined)
	}
	if !strings.Contains(output.String(), RedactedValue) || !strings.Contains(output.String(), "request_id") {
		t.Fatalf("safe panic evidence missing: %s", output.String())
	}
}
