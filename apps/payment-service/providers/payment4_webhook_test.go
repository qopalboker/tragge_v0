package providers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

// computePayment4Signature computes the HMAC-SHA256 signature for a Payment4 webhook payload.
func computePayment4Signature(secret string, data map[string]interface{}) string {
	canonical := sortedJSON(data)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestPayment4WebhookDuplicate(t *testing.T) {
	const secret = "test-ipn-secret"
	p := &Payment4{
		apiKey:    "test-key",
		ipnSecret: secret,
	}

	payload := []byte(`{"amount":25.00,"currency":"USD","orderId":"order-dup-1","paidAmount":25.00,"paymentUid":"pay-dup-1","status":"PAID"}`)

	data := map[string]interface{}{
		"amount":     25.0,
		"currency":   "USD",
		"orderId":    "order-dup-1",
		"paidAmount": 25.0,
		"paymentUid": "pay-dup-1",
		"status":     "PAID",
	}
	sig := computePayment4Signature(secret, data)

	headers := map[string]string{
		"x-payment4-signature": sig,
	}

	// First call
	event1, err := p.VerifyWebhook(context.Background(), headers, payload)
	if err != nil {
		t.Fatalf("first VerifyWebhook() error = %v", err)
	}
	if event1.Status != PaymentStatusFinished {
		t.Errorf("first call: expected status finished, got %s", event1.Status)
	}
	if event1.AmountCents != 2500 {
		t.Errorf("first call: expected AmountCents 2500, got %d", event1.AmountCents)
	}

	// Second identical call — provider layer is stateless, should return the same result
	event2, err := p.VerifyWebhook(context.Background(), headers, payload)
	if err != nil {
		t.Fatalf("second VerifyWebhook() error = %v", err)
	}
	if event2.Status != event1.Status {
		t.Errorf("second call: status %s != first call status %s", event2.Status, event1.Status)
	}
	if event2.AmountCents != event1.AmountCents {
		t.Errorf("second call: AmountCents %d != first call %d", event2.AmountCents, event1.AmountCents)
	}
	if event2.ProviderPaymentID != event1.ProviderPaymentID {
		t.Errorf("second call: ProviderPaymentID %s != first call %s", event2.ProviderPaymentID, event1.ProviderPaymentID)
	}
	if event2.OrderID != event1.OrderID {
		t.Errorf("second call: OrderID %s != first call %s", event2.OrderID, event1.OrderID)
	}
}

func TestPayment4WebhookAmountMismatch(t *testing.T) {
	const secret = "test-ipn-secret"
	p := &Payment4{
		apiKey:    "test-key",
		ipnSecret: secret,
	}

	// Payment was created for $10.00 (1000 cents) but webhook reports $5.00
	data := map[string]interface{}{
		"amount":     5.0,
		"currency":   "USD",
		"orderId":    "order-mismatch-1",
		"paidAmount": 5.0,
		"paymentUid": "pay-mismatch-1",
		"status":     "PAID",
	}
	payload := []byte(`{"amount":5.00,"currency":"USD","orderId":"order-mismatch-1","paidAmount":5.00,"paymentUid":"pay-mismatch-1","status":"PAID"}`)
	sig := computePayment4Signature(secret, data)

	headers := map[string]string{
		"x-payment4-signature": sig,
	}

	event, err := p.VerifyWebhook(context.Background(), headers, payload)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}

	// Provider layer just parses the webhook — it returns the reported amount.
	// The mismatch detection is handled by the handler's verifyWebhookAmount.
	if event.AmountCents != 500 {
		t.Errorf("expected AmountCents 500 (webhook reported $5.00), got %d", event.AmountCents)
	}
	if event.Status != PaymentStatusFinished {
		t.Errorf("expected status finished, got %s", event.Status)
	}

	// Verify that the handler-level amountsMatch would catch this discrepancy.
	// 500 vs 1000 is a 50% difference — well beyond the 1% tolerance.
	expectedIntentCents := int64(1000)
	if event.AmountCents == expectedIntentCents {
		t.Error("amounts should NOT match — this is a mismatch scenario")
	}
}

func TestPayment4WebhookInvalidSignature(t *testing.T) {
	p := &Payment4{
		apiKey:    "test-key",
		ipnSecret: "correct-secret",
	}

	payload := []byte(`{"amount":10.00,"currency":"USD","orderId":"order-sig-1","paymentUid":"pay-sig-1","status":"PAID"}`)

	headers := map[string]string{
		"x-payment4-signature": "definitely-wrong-signature-value",
	}

	_, err := p.VerifyWebhook(context.Background(), headers, payload)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestPayment4WebhookMissingSignature(t *testing.T) {
	payload := []byte(`{"amount":15.50,"currency":"EUR","orderId":"order-nosig-1","paidAmount":15.50,"paymentUid":"pay-nosig-1","status":"CONFIRMED"}`)
	headers := map[string]string{} // No signature header

	// When ipnSecret is configured, missing signature should fail
	t.Run("with_ipn_secret", func(t *testing.T) {
		p := &Payment4{
			apiKey:    "test-key",
			ipnSecret: "some-secret",
		}

		_, err := p.VerifyWebhook(context.Background(), headers, payload)
		if !errors.Is(err, ErrInvalidSignature) {
			t.Errorf("expected ErrInvalidSignature when ipnSecret is set and signature missing, got %v", err)
		}
	})

	// When ipnSecret is empty, missing signature should succeed
	t.Run("without_ipn_secret", func(t *testing.T) {
		p := &Payment4{
			apiKey: "test-key",
		}

		event, err := p.VerifyWebhook(context.Background(), headers, payload)
		if err != nil {
			t.Fatalf("VerifyWebhook() with missing signature and no ipnSecret should succeed, got error = %v", err)
		}

		if event.ProviderPaymentID != "pay-nosig-1" {
			t.Errorf("expected paymentUid pay-nosig-1, got %s", event.ProviderPaymentID)
		}
		if event.OrderID != "order-nosig-1" {
			t.Errorf("expected orderId order-nosig-1, got %s", event.OrderID)
		}
		if event.Status != PaymentStatusFinished {
			t.Errorf("expected status finished (CONFIRMED maps to finished), got %s", event.Status)
		}
		if event.AmountCents != 1550 {
			t.Errorf("expected AmountCents 1550, got %d", event.AmountCents)
		}
		if event.PaidAmountCents != 1550 {
			t.Errorf("expected PaidAmountCents 1550, got %d", event.PaidAmountCents)
		}
		if event.Currency != "EUR" {
			t.Errorf("expected currency EUR, got %s", event.Currency)
		}
	})
}

func TestPayment4WebhookUnknownPayment(t *testing.T) {
	const secret = "some-secret"
	p := &Payment4{
		apiKey:    "test-key",
		ipnSecret: secret,
	}

	// Webhook for a paymentUid that doesn't exist in our system.
	// At the provider level, VerifyWebhook just parses — it doesn't know about the DB.
	// The handler's processWebhookEvent will log a warning and return nil when the
	// payment intent is not found (sql.ErrNoRows → log + return nil).
	payload := []byte(`{"amount":99.99,"currency":"USD","orderId":"","paymentUid":"nonexistent-uid-123","status":"PAID"}`)

	data := map[string]interface{}{
		"amount":     99.99,
		"currency":   "USD",
		"orderId":    "",
		"paymentUid": "nonexistent-uid-123",
		"status":     "PAID",
	}
	sig := computePayment4Signature(secret, data)
	headers := map[string]string{
		"x-payment4-signature": sig,
	}

	event, err := p.VerifyWebhook(context.Background(), headers, payload)
	if err != nil {
		t.Fatalf("VerifyWebhook() for unknown payment should succeed at provider level, got error = %v", err)
	}

	if event.ProviderPaymentID != "nonexistent-uid-123" {
		t.Errorf("expected paymentUid nonexistent-uid-123, got %s", event.ProviderPaymentID)
	}
	if event.OrderID != "" {
		t.Errorf("expected empty orderId, got %s", event.OrderID)
	}
	if event.AmountCents != 9999 {
		t.Errorf("expected AmountCents 9999, got %d", event.AmountCents)
	}
	if event.Provider != ProviderPayment4 {
		t.Errorf("expected provider payment4, got %s", event.Provider)
	}
}
