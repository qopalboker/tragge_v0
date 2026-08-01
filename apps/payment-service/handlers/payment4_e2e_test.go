//go:build e2e

package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Parsaeffatravesh/tragge/apps/payment-service/providers"
	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// ---------------------------------------------------------------------------
// Test infrastructure
// ---------------------------------------------------------------------------

// noopCircuit implements DatabaseCircuitExecutor as a pass-through.
type noopCircuit struct{}

func (n *noopCircuit) ExecuteDatabase(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// mockPayment stores state for a single mock payment.
type mockPayment struct {
	UID      string
	OrderID  string
	Amount   float64
	Currency string
	Status   string
}

// mockPayment4API creates an httptest server mimicking the Payment4 gateway.
func mockPayment4API(t *testing.T, apiKey string) (*httptest.Server, *sync.Map) {
	t.Helper()
	payments := &sync.Map{} // map[string]*mockPayment keyed by UID

	mux := http.NewServeMux()

	// POST /payment/create
	mux.HandleFunc("POST /payment/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Amount      float64 `json:"amount"`
			Currency    string  `json:"currency"`
			OrderID     string  `json:"orderId"`
			Description string  `json:"description"`
			CallbackURL string  `json:"callbackUrl"`
			SuccessURL  string  `json:"successUrl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		uid := "mock-pay-" + uuid.New().String()[:8]
		mp := &mockPayment{
			UID:      uid,
			OrderID:  req.OrderID,
			Amount:   req.Amount,
			Currency: req.Currency,
			Status:   "PENDING",
		}
		payments.Store(uid, mp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"paymentUid": uid,
			"paymentUrl": "https://mock-payment4.example.com/pay/" + uid,
			"status":     "PENDING",
			"amount":     req.Amount,
			"currency":   req.Currency,
			"expiresAt":  time.Now().Add(30 * time.Minute).Format(time.RFC3339),
		})
	})

	// GET /payment/{uid}/status
	mux.HandleFunc("GET /payment/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Parse UID from path: /payment/{uid}/status
		path := strings.TrimPrefix(r.URL.Path, "/payment/")
		uid := strings.TrimSuffix(path, "/status")
		if uid == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		val, ok := payments.Load(uid)
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		mp := val.(*mockPayment)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"paymentUid":  mp.UID,
			"orderId":     mp.OrderID,
			"status":      mp.Status,
			"amount":      mp.Amount,
			"paidAmount":  mp.Amount,
			"currency":    mp.Currency,
			"payCurrency": "USDT",
		})
	})

	// GET /status (health check)
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, payments
}

// e2eEnv holds all dependencies for an e2e test.
type e2eEnv struct {
	db             *sql.DB
	userID         string
	depositHandler *DepositHandler
	webhookHandler *WebhookHandler
	registry       *providers.ProviderRegistry
	payments       *sync.Map // mock server state
	mockServerURL  string
	ipnSecret      string
}

const testSchema = `
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL DEFAULT '',
    display_name VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DO $$ BEGIN CREATE TYPE wallet_status AS ENUM ('active','frozen','closed'); EXCEPTION WHEN duplicate_object THEN null; END $$;

CREATE TABLE IF NOT EXISTS wallets (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    balance_cents BIGINT NOT NULL DEFAULT 0,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status wallet_status NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_balance_non_negative CHECK (balance_cents >= 0)
);

DO $$ BEGIN CREATE TYPE ledger_type AS ENUM (
    'deposit','withdrawal','contest_entry','contest_refund','prize_credit',
    'adjustment','affiliate_commission','withdraw_fee','withdrawal_refund','withdraw_fee_refund'
); EXCEPTION WHEN duplicate_object THEN null; END $$;

DO $$ BEGIN CREATE TYPE ledger_ref_type AS ENUM (
    'payment_intent','payout','contest','admin_action','commission'
); EXCEPTION WHEN duplicate_object THEN null; END $$;

CREATE TABLE IF NOT EXISTS wallet_ledger (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type ledger_type NOT NULL,
    amount_cents BIGINT NOT NULL,
    balance_after_cents BIGINT NOT NULL,
    ref_type ledger_ref_type,
    ref_id UUID,
    description TEXT,
    reason_code VARCHAR(50),
    idempotency_key VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_amount_non_zero CHECK (amount_cents != 0),
    CONSTRAINT chk_balance_after_non_negative CHECK (balance_after_cents >= 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_e2e_wl_idempotency
    ON wallet_ledger(idempotency_key) WHERE idempotency_key IS NOT NULL;

DO $$ BEGIN CREATE TYPE payment_intent_status AS ENUM (
    'pending','processing','succeeded','failed','cancelled','refunded','expired'
); EXCEPTION WHEN duplicate_object THEN null; END $$;

CREATE TABLE IF NOT EXISTS payment_intents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    provider_payment_id VARCHAR(255),
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status payment_intent_status NOT NULL DEFAULT 'pending',
    metadata_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    CONSTRAINT chk_payment_amount_positive CHECK (amount_cents > 0)
);

CREATE INDEX IF NOT EXISTS idx_e2e_pi_provider ON payment_intents(provider, provider_payment_id);
`

// setupE2E creates the test schema, inserts a test user, and returns a ready-to-use env.
// It skips the test if POSTGRES_DSN is not set.
func setupE2E(t *testing.T) *e2eEnv {
	t.Helper()

	dsn := getDSN()
	if dsn == "" {
		t.Skip("POSTGRES_DSN not set, skipping e2e test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()

	// Apply schema
	if _, err := db.ExecContext(ctx, testSchema); err != nil {
		t.Fatalf("failed to apply test schema: %v", err)
	}

	// Create test user
	userID := uuid.New().String()
	_, err = db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, fmt.Sprintf("e2e-%s@test.com", userID[:8]))
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	// Create wallet for the test user
	_, err = db.ExecContext(ctx,
		`INSERT INTO wallets (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, userID)
	if err != nil {
		t.Fatalf("failed to insert test wallet: %v", err)
	}

	// Clean up test data when done
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM wallet_ledger WHERE user_id = $1`, userID)
		db.ExecContext(ctx, `DELETE FROM payment_intents WHERE user_id = $1`, userID)
		db.ExecContext(ctx, `DELETE FROM wallets WHERE user_id = $1`, userID)
		db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	// Set up mock Payment4 API
	const testAPIKey = "e2e-test-api-key"
	const testIPNSecret = "e2e-test-ipn-secret"
	mockSrv, payments := mockPayment4API(t, testAPIKey)

	// Register Payment4 provider pointing at mock server
	registry := providers.NewProviderRegistry()
	payment4 := providers.NewPayment4(providers.Payment4Config{
		APIKey:    testAPIKey,
		IPNSecret: testIPNSecret,
		BaseURL:   mockSrv.URL,
	})
	registry.Register(payment4)

	logger := zap.NewNop()
	circuit := &noopCircuit{}
	walletSvc := wallet.NewService(db)

	depositCfg := &DepositConfig{
		MinDepositCents:    100,  // $1.00 minimum
		MaxDepositCents:    100000, // $1000.00 maximum
		DefaultCurrency:    "USD",
		WebhookBaseURL:     mockSrv.URL,
		SuccessRedirectURL: "https://example.com/success",
		CancelRedirectURL:  "https://example.com/cancel",
	}
	depositHandler := NewDepositHandler(db, registry, nil, logger, depositCfg, circuit, nil)
	webhookHandler := NewWebhookHandler(db, walletSvc, registry, nil, logger,
		depositCfg.SuccessRedirectURL, depositCfg.CancelRedirectURL, circuit, nil)

	return &e2eEnv{
		db:             db,
		userID:         userID,
		depositHandler: depositHandler,
		webhookHandler: webhookHandler,
		registry:       registry,
		payments:       payments,
		mockServerURL:  mockSrv.URL,
		ipnSecret:      testIPNSecret,
	}
}

// getDSN returns the POSTGRES_DSN environment variable, checking common alternatives.
func getDSN() string {
	for _, key := range []string{"POSTGRES_DSN", "TEST_POSTGRES_DSN", "DATABASE_URL"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return ""
}

// withAuth injects the user ID into the request context for auth.GetUserID().
func withAuth(r *http.Request, userID string) *http.Request {
	ctx := context.WithValue(r.Context(), auth.UserIDKey, userID)
	return r.WithContext(ctx)
}

// createDeposit is a helper that creates a deposit and returns the response.
func (env *e2eEnv) createDeposit(t *testing.T, amountCents int64) CreateDepositResponse {
	t.Helper()
	body, _ := json.Marshal(CreateDepositRequest{
		AmountCents: amountCents,
		Currency:    "USD",
		Provider:    "payment4",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments/deposit/crypto/create", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAuth(req, env.userID)

	w := httptest.NewRecorder()
	env.depositHandler.HandleCreateCryptoDeposit(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("createDeposit: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp CreateDepositResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("createDeposit: failed to decode response: %v", err)
	}
	return resp
}

// computeSignature generates an HMAC-SHA256 signature for a Payment4 webhook payload.
func computeSignature(secret string, data map[string]interface{}) string {
	canonical := canonicalJSON(data)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// canonicalJSON produces a sorted JSON representation matching the provider's sortedJSON.
func canonicalJSON(v interface{}) string {
	var b strings.Builder
	writeCanonicalJSON(&b, v)
	return b.String()
}

func writeCanonicalJSON(b *strings.Builder, v interface{}) {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		// Sort keys
		for i := 1; i < len(keys); i++ {
			for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
				keys[j], keys[j-1] = keys[j-1], keys[j]
			}
		}
		b.WriteString("{")
		for i, k := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			keyJSON, _ := json.Marshal(k)
			b.Write(keyJSON)
			b.WriteString(":")
			writeCanonicalJSON(b, val[k])
		}
		b.WriteString("}")
	case []interface{}:
		b.WriteString("[")
		for i, elem := range val {
			if i > 0 {
				b.WriteString(",")
			}
			writeCanonicalJSON(b, elem)
		}
		b.WriteString("]")
	default:
		raw, _ := json.Marshal(v)
		b.Write(raw)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPayment4CreateCryptoDeposit(t *testing.T) {
	env := setupE2E(t)

	resp := env.createDeposit(t, 1000) // $10.00

	// Assert response fields
	if resp.PaymentIntentID == "" {
		t.Error("expected non-empty payment_intent_id")
	}
	if resp.PaymentURL == "" {
		t.Error("expected non-empty payment_url")
	}
	if !strings.HasPrefix(resp.PaymentURL, "https://mock-payment4.example.com/pay/") {
		t.Errorf("unexpected payment_url: %s", resp.PaymentURL)
	}

	// Assert payment_intents row in database
	var dbStatus, dbProvider string
	var dbProviderPaymentID sql.NullString
	var dbAmountCents int64
	err := env.db.QueryRowContext(context.Background(), `
		SELECT status, provider, provider_payment_id, amount_cents
		FROM payment_intents WHERE id = $1
	`, resp.PaymentIntentID).Scan(&dbStatus, &dbProvider, &dbProviderPaymentID, &dbAmountCents)
	if err != nil {
		t.Fatalf("failed to query payment_intents: %v", err)
	}
	if dbStatus != "processing" {
		t.Errorf("expected status 'processing', got %q", dbStatus)
	}
	if dbProvider != "payment4" {
		t.Errorf("expected provider 'payment4', got %q", dbProvider)
	}
	if !dbProviderPaymentID.Valid || dbProviderPaymentID.String == "" {
		t.Error("expected provider_payment_id to be set")
	}
	if dbAmountCents != 1000 {
		t.Errorf("expected amount_cents 1000, got %d", dbAmountCents)
	}
}

func TestPayment4WebhookProcessing(t *testing.T) {
	env := setupE2E(t)
	ctx := context.Background()

	// Step 1: Create a deposit
	deposit := env.createDeposit(t, 1000) // $10.00

	// Look up the provider_payment_id
	var providerPaymentID string
	err := env.db.QueryRowContext(ctx, `
		SELECT provider_payment_id FROM payment_intents WHERE id = $1
	`, deposit.PaymentIntentID).Scan(&providerPaymentID)
	if err != nil {
		t.Fatalf("failed to get provider_payment_id: %v", err)
	}

	// Record initial wallet balance
	var initialBalance int64
	err = env.db.QueryRowContext(ctx, `SELECT balance_cents FROM wallets WHERE user_id = $1`, env.userID).Scan(&initialBalance)
	if err != nil {
		t.Fatalf("failed to get initial balance: %v", err)
	}

	// Step 2: Build and send PAID webhook
	webhookData := map[string]interface{}{
		"paymentUid": providerPaymentID,
		"orderId":    deposit.PaymentIntentID,
		"status":     "PAID",
		"amount":     10.00,
		"paidAmount": 10.00,
		"currency":   "USD",
	}
	webhookBody, _ := json.Marshal(webhookData)
	sig := computeSignature(env.ipnSecret, webhookData)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/payment4", bytes.NewReader(webhookBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment4-Signature", sig)
	w := httptest.NewRecorder()

	env.webhookHandler.HandlePayment4Webhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("webhook: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Step 3: Assert payment_intents updated
	var updatedStatus string
	var completedAt sql.NullTime
	err = env.db.QueryRowContext(ctx, `
		SELECT status, completed_at FROM payment_intents WHERE id = $1
	`, deposit.PaymentIntentID).Scan(&updatedStatus, &completedAt)
	if err != nil {
		t.Fatalf("failed to query updated payment_intent: %v", err)
	}
	if updatedStatus != "succeeded" {
		t.Errorf("expected status 'succeeded', got %q", updatedStatus)
	}
	if !completedAt.Valid {
		t.Error("expected completed_at to be set")
	}

	// Step 4: Assert wallet balance increased
	var newBalance int64
	err = env.db.QueryRowContext(ctx, `SELECT balance_cents FROM wallets WHERE user_id = $1`, env.userID).Scan(&newBalance)
	if err != nil {
		t.Fatalf("failed to get new balance: %v", err)
	}
	if newBalance != initialBalance+1000 {
		t.Errorf("expected balance %d, got %d", initialBalance+1000, newBalance)
	}

	// Step 5: Assert wallet_ledger entry exists
	var ledgerCount int
	err = env.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM wallet_ledger
		WHERE user_id = $1 AND idempotency_key = $2
	`, env.userID, "deposit:"+deposit.PaymentIntentID).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("failed to count ledger entries: %v", err)
	}
	if ledgerCount != 1 {
		t.Errorf("expected 1 ledger entry, got %d", ledgerCount)
	}

	// Step 6: Idempotency — send the same webhook again
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/payment4", bytes.NewReader(webhookBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Payment4-Signature", sig)
	w2 := httptest.NewRecorder()

	env.webhookHandler.HandlePayment4Webhook(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("duplicate webhook: expected 200, got %d", w2.Code)
	}

	// Balance must not change
	var balanceAfterDup int64
	err = env.db.QueryRowContext(ctx, `SELECT balance_cents FROM wallets WHERE user_id = $1`, env.userID).Scan(&balanceAfterDup)
	if err != nil {
		t.Fatalf("failed to get balance after duplicate: %v", err)
	}
	if balanceAfterDup != newBalance {
		t.Errorf("duplicate webhook changed balance: expected %d, got %d", newBalance, balanceAfterDup)
	}

	// Ledger entry count must not change
	var ledgerCountAfterDup int
	err = env.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM wallet_ledger
		WHERE user_id = $1 AND idempotency_key = $2
	`, env.userID, "deposit:"+deposit.PaymentIntentID).Scan(&ledgerCountAfterDup)
	if err != nil {
		t.Fatalf("failed to count ledger entries after duplicate: %v", err)
	}
	if ledgerCountAfterDup != 1 {
		t.Errorf("duplicate webhook created extra ledger entries: expected 1, got %d", ledgerCountAfterDup)
	}
}

func TestPayment4StatusPolling(t *testing.T) {
	env := setupE2E(t)

	// Create a deposit
	deposit := env.createDeposit(t, 2000) // $20.00

	// Use a chi router to handle URL param extraction
	r := chi.NewRouter()
	r.Get("/api/payments/deposit/{id}/status", func(w http.ResponseWriter, req *http.Request) {
		env.depositHandler.HandleGetDepositStatus(w, req)
	})

	// Build request with auth
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/payments/deposit/%s/status", deposit.PaymentIntentID), nil)
	req = withAuth(req, env.userID)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status polling: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var statusResp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&statusResp); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
	}

	if statusResp["payment_intent_id"] != deposit.PaymentIntentID {
		t.Errorf("expected payment_intent_id %s, got %v", deposit.PaymentIntentID, statusResp["payment_intent_id"])
	}
	if statusResp["provider"] != "payment4" {
		t.Errorf("expected provider payment4, got %v", statusResp["provider"])
	}
	// Amount should be 2000 cents
	if amountCents, ok := statusResp["amount_cents"].(float64); !ok || int64(amountCents) != 2000 {
		t.Errorf("expected amount_cents 2000, got %v", statusResp["amount_cents"])
	}
}

func TestPayment4ErrorCases(t *testing.T) {
	env := setupE2E(t)

	t.Run("invalid_provider", func(t *testing.T) {
		body, _ := json.Marshal(CreateDepositRequest{
			AmountCents: 1000,
			Currency:    "USD",
			Provider:    "invalid_provider",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/payments/deposit/crypto/create", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withAuth(req, env.userID)

		w := httptest.NewRecorder()
		env.depositHandler.HandleCreateCryptoDeposit(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("amount_below_minimum", func(t *testing.T) {
		body, _ := json.Marshal(CreateDepositRequest{
			AmountCents: 1, // Below minimum of 100
			Currency:    "USD",
			Provider:    "payment4",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/payments/deposit/crypto/create", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withAuth(req, env.userID)

		w := httptest.NewRecorder()
		env.depositHandler.HandleCreateCryptoDeposit(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for amount below minimum, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("provider_unavailable", func(t *testing.T) {
		// Create a separate registry with a Payment4 provider pointing at a dead server
		deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(deadSrv.Close)

		deadRegistry := providers.NewProviderRegistry()
		deadPayment4 := providers.NewPayment4(providers.Payment4Config{
			APIKey:  "dead-key",
			BaseURL: deadSrv.URL,
		})
		deadRegistry.Register(deadPayment4)

		deadDepositHandler := NewDepositHandler(env.db, deadRegistry, nil,
			zap.NewNop(), &DepositConfig{
				MinDepositCents: 100,
				MaxDepositCents: 100000,
				DefaultCurrency: "USD",
			}, &noopCircuit{}, nil)

		body, _ := json.Marshal(CreateDepositRequest{
			AmountCents: 1000,
			Currency:    "USD",
			Provider:    "payment4",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/payments/deposit/crypto/create", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withAuth(req, env.userID)

		w := httptest.NewRecorder()
		deadDepositHandler.HandleCreateCryptoDeposit(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("expected 503 for unavailable provider, got %d: %s", w.Code, w.Body.String())
		}
	})
}
