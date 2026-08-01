package sms

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestKaveNegarOTPUsesBoundedTemplateOnlyAndSanitizedErrors(t *testing.T) {
	const (
		fixtureCredential = "fixture-kavenegar-credential"
		fixtureCode       = "fixture-code-canary"
	)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, fixtureCode, http.StatusUnauthorized)
	}))
	defer server.Close()

	provider := newKaveNegar(Config{
		APIKey: fixtureCredential, Template: "fixture-template", Enabled: true,
	}, 100*time.Millisecond)
	baseURL, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	provider.client.BaseURL = baseURL
	err = provider.SendOTP(context.Background(), "+989120000000", fixtureCode)
	if !errors.Is(err, ErrKaveNegarUnavailable) {
		t.Fatalf("expected sanitized KaveNegar failure, got %v", err)
	}
	if strings.Contains(err.Error(), fixtureCredential) || strings.Contains(err.Error(), fixtureCode) {
		t.Fatal("KaveNegar error exposed provider or code material")
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected KaveNegar request count: %d", calls.Load())
	}
}

func TestKaveNegarOTPTimeoutAndCancellationFailClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := newKaveNegar(Config{
		APIKey: "fixture-kavenegar-credential", Template: "fixture-template", Enabled: true,
	}, 10*time.Millisecond)
	baseURL, _ := url.Parse(server.URL + "/")
	provider.client.BaseURL = baseURL
	if err := provider.SendOTP(context.Background(), "+989120000000", "fixture"); !errors.Is(err, ErrKaveNegarUnavailable) {
		t.Fatalf("timeout did not fail closed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.SendOTP(ctx, "+989120000000", "fixture"); !errors.Is(err, ErrKaveNegarUnavailable) {
		t.Fatalf("cancellation did not fail closed: %v", err)
	}
}
