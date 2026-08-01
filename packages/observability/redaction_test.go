package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const syntheticCredential = "sec005-fixture-never-use"

func TestRedactTextCredentialClasses(t *testing.T) {
	jwt := "eyJ" + strings.Repeat("a", 12) + "." + strings.Repeat("b", 12) + "." + strings.Repeat("c", 12)
	inputs := []string{
		"Authorization: Bearer " + syntheticCredential,
		"password=" + syntheticCredential,
		"postgres://fixture:" + syntheticCredential + "@localhost/test",
		"token=" + jwt,
		"otp=" + syntheticCredential,
		"reset_token=" + syntheticCredential,
		"api_key=" + syntheticCredential,
		"provider_secret=" + syntheticCredential,
		"webhook_signature=" + syntheticCredential,
		"cookie=" + syntheticCredential,
		"national_code=" + syntheticCredential,
		"-----BEGIN PRIVATE KEY-----\n" + syntheticCredential + "\n-----END PRIVATE KEY-----",
	}
	for _, input := range inputs {
		redacted := RedactText(input)
		if strings.Contains(redacted, syntheticCredential) || strings.Contains(redacted, jwt) {
			t.Fatalf("credential remained after redaction: %q", redacted)
		}
		if !strings.Contains(redacted, RedactedValue) {
			t.Fatalf("stable marker missing: %q", redacted)
		}
	}
}

func TestRedactStructuredDataHeadersAndURL(t *testing.T) {
	metadata := map[string]any{
		"PaSsWoRd": syntheticCredential,
		"profile": map[string]any{
			"email":  "fixture@example.invalid",
			"status": "active",
			"items":  []any{map[string]any{"refresh_token": syntheticCredential, "attempt": 2}},
		},
		"error": errors.New("redis://:" + syntheticCredential + "@localhost/0"),
	}
	redacted := RedactValue(metadata).(map[string]any)
	encodedBytes, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(encodedBytes)
	if strings.Contains(encoded, syntheticCredential) || strings.Contains(encoded, "fixture@example.invalid") {
		t.Fatalf("structured credential remained: %s", encoded)
	}
	if !strings.Contains(encoded, "active") {
		t.Fatalf("safe diagnostic was removed: %s", encoded)
	}

	inputHeaders := make(http.Header)
	inputHeaders.Set("Authorization", "Bearer "+syntheticCredential)
	inputHeaders.Set("Cookie", "session="+syntheticCredential)
	inputHeaders.Set("Set-Cookie", "refresh="+syntheticCredential)
	inputHeaders.Set("X-API-Key", syntheticCredential)
	inputHeaders.Set("X-Webhook-Signature", syntheticCredential)
	inputHeaders.Set("X-Request-ID", "request-1234")
	headers := RedactHeaders(inputHeaders)
	for _, key := range []string{"Authorization", "Cookie", "Set-Cookie", "X-API-Key", "X-Webhook-Signature"} {
		if headers.Get(key) != RedactedValue {
			t.Fatalf("header %s was not redacted: %#v", key, headers)
		}
	}
	if headers.Get("X-Request-ID") != "request-1234" {
		t.Fatalf("unexpected header redaction: %#v", headers)
	}

	parsed, err := url.Parse("https://fixture:" + syntheticCredential + "@example.invalid/path?access_token=" + syntheticCredential + "&reset_code=" + syntheticCredential + "&otp=" + syntheticCredential + "&api_key=" + syntheticCredential + "&page=2")
	if err != nil {
		t.Fatal(err)
	}
	safeURL := RedactURL(parsed).String()
	if strings.Contains(safeURL, syntheticCredential) || !strings.Contains(safeURL, "page=2") {
		t.Fatalf("unexpected URL redaction: %s", safeURL)
	}
}

func TestRedactStructAndFormCompatibleValues(t *testing.T) {
	type providerResponse struct {
		ProviderSecret string         `json:"provider_secret"`
		Status         string         `json:"status"`
		Form           url.Values     `json:"form"`
		Nested         map[string]any `json:"nested"`
	}
	input := providerResponse{
		ProviderSecret: syntheticCredential,
		Status:         "rejected",
		Form:           url.Values{"reset_token": {syntheticCredential}, "result": {"retry"}},
		Nested:         map[string]any{"national_code": syntheticCredential},
	}
	encoded, err := json.Marshal(RedactValue(input))
	if err != nil {
		t.Fatal(err)
	}
	output := string(encoded)
	if strings.Contains(output, syntheticCredential) {
		t.Fatalf("struct or form-compatible value leaked: %s", output)
	}
	if !strings.Contains(output, RedactedValue) || !strings.Contains(output, "rejected") || !strings.Contains(output, "retry") {
		t.Fatalf("redaction removed safe struct evidence: %s", output)
	}
}

func TestRedactingCoreJSONAndConsole(t *testing.T) {
	for name, encoder := range map[string]zapcore.Encoder{
		"json":    zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		"console": zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			logger := zap.New(NewRedactingCore(zapcore.NewCore(encoder, zapcore.AddSync(&output), zapcore.DebugLevel)))
			logger.Error("failed with password="+syntheticCredential,
				zap.String("access_token", syntheticCredential),
				zap.Error(fmt.Errorf("wrapped provider error: %w", errors.New("postgres://fixture:"+syntheticCredential+"@localhost/test"))),
				zap.Any("metadata", map[string]any{"otp": syntheticCredential, "status": "denied"}),
			)
			captured := output.String()
			if strings.Contains(captured, syntheticCredential) {
				t.Fatalf("captured output leaked credential: %s", captured)
			}
			if !strings.Contains(captured, RedactedValue) || !strings.Contains(captured, "denied") {
				t.Fatalf("captured output lost expected evidence: %s", captured)
			}
		})
	}
}

func TestStandardWriterRedactsAndReportsOriginalLength(t *testing.T) {
	var output bytes.Buffer
	writer := NewRedactingWriter(&output)
	input := []byte("password=" + syntheticCredential)
	written, err := writer.Write(input)
	if err != nil || written != len(input) {
		t.Fatalf("write result=(%d,%v), want=(%d,nil)", written, err, len(input))
	}
	if strings.Contains(output.String(), syntheticCredential) || !strings.Contains(output.String(), RedactedValue) {
		t.Fatalf("standard writer output was unsafe: %s", output.String())
	}
}
