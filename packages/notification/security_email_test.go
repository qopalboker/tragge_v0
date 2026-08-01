package notification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMailerinoSecurityEmailProviderContract(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/send" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		authorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "api-key") {
			t.Fatal("provider credential was copied into request body")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fixture-message","status":"queued"}`))
	}))
	defer server.Close()

	provider, err := NewMailerinoSecurityEmailProvider(SecurityEmailHTTPConfig{
		BaseURL: server.URL, APIKey: "fake-mailerino-key", From: "security@example.test",
		AllowHTTPForTesting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = provider.SendSecurityEmail(context.Background(), SecurityEmailMessage{
		To: "recipient@example.test", Subject: "fixture", Text: "fixture body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer fake-mailerino-key" {
		t.Fatal("expected bearer authentication")
	}
}

func TestResendSecurityEmailProviderContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("Resend contract requires a User-Agent")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"fixture-message"}`))
	}))
	defer server.Close()

	provider, err := NewResendSecurityEmailProvider(SecurityEmailHTTPConfig{
		BaseURL: server.URL, APIKey: "fake-resend-key", From: "security@example.test",
		AllowHTTPForTesting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.SendSecurityEmail(context.Background(), SecurityEmailMessage{
		To: "recipient@example.test", Subject: "fixture", HTML: "<p>fixture</p>",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSecurityEmailProvidersFailClosedAndSanitizeErrors(t *testing.T) {
	const sensitiveProviderBody = "provider diagnostic with credential-like detail"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, sensitiveProviderBody, http.StatusUnauthorized)
	}))
	defer server.Close()

	provider, err := NewMailerinoSecurityEmailProvider(SecurityEmailHTTPConfig{
		BaseURL: server.URL, APIKey: "fake-key", From: "security@example.test",
		AllowHTTPForTesting: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = provider.SendSecurityEmail(context.Background(), SecurityEmailMessage{
		To: "recipient@example.test", Subject: "fixture", Text: "fixture",
	})
	if !errors.Is(err, ErrSecurityEmailDelivery) {
		t.Fatalf("expected sanitized delivery error, got %v", err)
	}
	if strings.Contains(err.Error(), sensitiveProviderBody) {
		t.Fatal("provider response leaked through error")
	}
}

type cancellationHTTPDoer struct{}

func (cancellationHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestSecurityEmailProviderHonorsCancellation(t *testing.T) {
	provider, err := NewMailerinoSecurityEmailProvider(SecurityEmailHTTPConfig{
		BaseURL: "https://mailerino.example.test", APIKey: "fake-key", From: "security@example.test",
		Client: cancellationHTTPDoer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := provider.SendSecurityEmail(ctx, SecurityEmailMessage{
		To: "recipient@example.test", Subject: "fixture", Text: "fixture",
	}); !errors.Is(err, ErrSecurityEmailDelivery) {
		t.Fatalf("expected sanitized cancellation failure, got %v", err)
	}
}
func TestRenderSecurityCodeEmail(t *testing.T) {
	for _, purpose := range []string{"email_verification", "password_reset"} {
		message, err := RenderSecurityCodeEmail(purpose, "123456", "en")
		if err != nil {
			t.Fatalf("%s: %v", purpose, err)
		}
		if message.Subject == "" || !strings.Contains(message.HTML, "123456") {
			t.Fatalf("%s message was not rendered", purpose)
		}
	}
}
