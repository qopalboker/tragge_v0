package notification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSecurityEmailProviderResponseFailuresAreBoundedAndSanitized(t *testing.T) {
	const (
		fixtureCredential = "fixture-provider-credential-never-log"
		fixtureCode       = "fixture-message-body-canary"
	)
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "bad request", status: http.StatusBadRequest, body: fixtureCode},
		{name: "rate limited", status: http.StatusTooManyRequests, body: fixtureCredential},
		{name: "server failure", status: http.StatusInternalServerError, body: strings.Repeat("x", securityEmailResponseLimit+1024)},
		{name: "malformed success", status: http.StatusOK, body: `{"id":`},
		{name: "missing acceptance fields", status: http.StatusOK, body: `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			provider, err := NewMailerinoSecurityEmailProvider(SecurityEmailHTTPConfig{
				BaseURL: server.URL, APIKey: fixtureCredential, From: "security@example.test",
				AllowHTTPForTesting: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			err = provider.SendSecurityEmail(context.Background(), SecurityEmailMessage{
				To: "recipient@example.test", Subject: "fixture", Text: fixtureCode,
			})
			if !errors.Is(err, ErrSecurityEmailDelivery) {
				t.Fatalf("expected sanitized delivery error, got %v", err)
			}
			if strings.Contains(err.Error(), fixtureCredential) || strings.Contains(err.Error(), fixtureCode) {
				t.Fatal("provider credential or message canary leaked through error")
			}
			if calls.Load() != 1 {
				t.Fatalf("adapter automatically retried an unproven-idempotent send: %d calls", calls.Load())
			}
		})
	}
}

func TestResendSecurityEmailProviderRejectsMalformedAcceptance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	provider, err := NewResendSecurityEmailProvider(SecurityEmailHTTPConfig{
		BaseURL: server.URL, APIKey: "fixture-resend-credential", From: "security@example.test",
		AllowHTTPForTesting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.SendSecurityEmail(context.Background(), SecurityEmailMessage{
		To: "recipient@example.test", Subject: "fixture", Text: "fixture",
	}); !errors.Is(err, ErrSecurityEmailDelivery) {
		t.Fatalf("malformed acceptance was not rejected: %v", err)
	}
}
