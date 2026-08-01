package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetiredCryptoProviderFailsBeforeRuntimeLookup(t *testing.T) {
	handler := &DepositHandler{}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/payments/deposit/crypto/create",
		strings.NewReader(`{"amount_cents":500,"provider":"payment4","pay_currency":"usdttrc20"}`),
	)

	handler.HandleCreateCryptoDeposit(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("retired provider status=%d want=%d", recorder.Code, http.StatusBadRequest)
	}
	if strings.Contains(strings.ToLower(recorder.Body.String()), "payment4") {
		t.Fatalf("response reveals retired provider history: %q", recorder.Body.String())
	}
}
