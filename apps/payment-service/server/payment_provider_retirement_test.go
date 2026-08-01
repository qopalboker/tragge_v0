package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/go-chi/chi/v5"
)

func TestRetiredPaymentProviderRouteIsNotRegistered(t *testing.T) {
	router := chi.NewRouter()
	called := false
	registerPaymentWebhookRoutes(router, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	retired := httptest.NewRecorder()
	router.ServeHTTP(retired, httptest.NewRequest(http.MethodPost, "/webhooks/payment4", nil))
	if retired.Code != http.StatusNotFound {
		t.Fatalf("retired provider route status=%d want=%d", retired.Code, http.StatusNotFound)
	}
	if called {
		t.Fatal("retired provider route reached the remaining-provider handler")
	}

	remaining := httptest.NewRecorder()
	router.ServeHTTP(remaining, httptest.NewRequest(http.MethodPost, "/webhooks/nowpayments", nil))
	if remaining.Code != http.StatusNoContent || !called {
		t.Fatalf("remaining provider route status=%d called=%v", remaining.Code, called)
	}
}

func TestRemainingPaymentProvidersInitializeWithoutRetiredConfiguration(t *testing.T) {
	registry := providers.NewProviderRegistry()
	registry.Register(providers.NewNowPayments(providers.NowPaymentsConfig{}))
	registry.Register(providers.NewJibit(providers.JibitConfig{}))

	for _, providerType := range []providers.ProviderType{
		providers.ProviderNowPayments,
		providers.ProviderJibit,
	} {
		provider, ok := registry.Get(providerType)
		if !ok || provider.Name() != providerType {
			t.Fatalf("remaining provider %q was not initialized", providerType)
		}
	}
}

func TestPaymentServiceConfigurationHasNoRetiredProviderSurface(t *testing.T) {
	configType := reflect.TypeOf(Config{})
	for index := 0; index < configType.NumField(); index++ {
		field := configType.Field(index)
		if strings.Contains(strings.ToLower(field.Name), "payment4") {
			t.Fatalf("retired provider configuration field remains: %s", field.Name)
		}
	}
}
