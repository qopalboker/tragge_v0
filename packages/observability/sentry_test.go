package observability

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestRedactSentryEvent(t *testing.T) {
	event := &sentry.Event{
		Message: "password=" + syntheticCredential,
		Extra: map[string]interface{}{
			"refresh_token": syntheticCredential,
			"outcome":       "denied",
		},
		Request: &sentry.Request{
			URL:         "https://example.invalid/private",
			QueryString: "token=" + syntheticCredential,
			Data:        "{\"password\":\"" + syntheticCredential + "\"}",
			Cookies:     "session=" + syntheticCredential,
			Headers:     map[string]string{"Authorization": "Bearer " + syntheticCredential, "X-Request-ID": "request-1234"},
		},
		User:      sentry.User{ID: "actor-1234", Email: "fixture@example.invalid", IPAddress: "192.0.2.1"},
		Exception: []sentry.Exception{{Value: "postgres://fixture:" + syntheticCredential + "@localhost/test"}},
	}
	redacted := RedactSentryEvent(event, nil)
	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	for _, prohibited := range []string{syntheticCredential, "fixture@example.invalid", "192.0.2.1"} {
		if strings.Contains(output, prohibited) {
			t.Fatalf("Sentry event leaked %q: %s", prohibited, output)
		}
	}
	for _, required := range []string{RedactedValue, "actor-1234", "request-1234", "denied"} {
		if !strings.Contains(output, required) {
			t.Fatalf("Sentry event lost %q: %s", required, output)
		}
	}
	if redacted.Request.QueryString != "" {
		t.Fatalf("Sentry query string retained: %q", redacted.Request.QueryString)
	}
}
