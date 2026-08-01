package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Parsaeffatravesh/tragge/packages/auth"
	"github.com/Parsaeffatravesh/tragge/packages/db"
	"github.com/Parsaeffatravesh/tragge/packages/observability"
	pkgredis "github.com/Parsaeffatravesh/tragge/packages/redis"
	"github.com/Parsaeffatravesh/tragge/packages/wallet"
	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	redis "github.com/redis/go-redis/v9"
)

func requireSEC004Runtime(t *testing.T) (string, string) {
	t.Helper()
	dsn := os.Getenv("SEC004_POSTGRES_DSN")
	redisAddr := os.Getenv("SEC004_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Skip("SEC004_POSTGRES_DSN and SEC004_REDIS_ADDR are required for isolated runtime validation")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "localhost") || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatalf("refusing non-local or non-test PostgreSQL DSN")
	}
	if !strings.HasPrefix(redisAddr, "127.0.0.1:") && !strings.HasPrefix(redisAddr, "localhost:") {
		t.Fatalf("refusing non-local Redis address")
	}
	return dsn, redisAddr
}

func sec004Context(parent context.Context, actorID, sessionID string) context.Context {
	ctx := context.WithValue(parent, auth.UserIDKey, actorID)
	ctx = context.WithValue(ctx, auth.SessionIDKey, sessionID)
	ctx = context.WithValue(ctx, auth.RolesKey, []string{auth.RoleSuperAdmin})
	ctx = context.WithValue(ctx, auth.PermissionsKey, []string{"withdrawals.manage", "users.edit", "users.wallet.charge"})
	return ctx
}

func sec004RouteRequest(t *testing.T, ctx context.Context, method, path, param, value, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add(param, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, routeCtx))
}

func TestSEC004PostgresRedisRuntime(t *testing.T) {
	dsn, redisAddr := requireSEC004Runtime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	redisClient := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 14})
	defer redisClient.Close()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	if err := redisClient.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	defer redisClient.FlushDB(context.Background())
	redisConfig := pkgredis.DefaultConfig()
	redisConfig.Addr = redisAddr
	redisConfig.DB = 14
	applicationRedis, err := pkgredis.NewClient(redisConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer applicationRedis.Close()

	statements := []string{
		`DROP TABLE IF EXISTS wallet_ledger, wallets, audit_logs, payouts, role_permissions, user_roles, permissions, roles, users CASCADE`,
		`CREATE TABLE users (id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text, email TEXT UNIQUE, password_hash TEXT NOT NULL, status TEXT NOT NULL, is_system_account BOOLEAN NOT NULL DEFAULT FALSE, email_verified BOOLEAN NOT NULL DEFAULT FALSE, terms_accepted_at TIMESTAMPTZ, display_name TEXT)`,
		`CREATE TABLE roles (id SERIAL PRIMARY KEY, name TEXT UNIQUE NOT NULL)`,
		`CREATE TABLE permissions (id SERIAL PRIMARY KEY, name TEXT UNIQUE NOT NULL)`,
		`CREATE TABLE user_roles (user_id TEXT REFERENCES users(id), role_id INTEGER REFERENCES roles(id), PRIMARY KEY(user_id, role_id))`,
		`CREATE TABLE role_permissions (role_id INTEGER REFERENCES roles(id), permission_id INTEGER REFERENCES permissions(id), PRIMARY KEY(role_id, permission_id))`,
		`CREATE TABLE audit_logs (id BIGSERIAL PRIMARY KEY, actor_user_id TEXT, action TEXT NOT NULL, target_type TEXT NOT NULL, target_id TEXT NOT NULL, payload_json JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE payouts (id TEXT PRIMARY KEY, status TEXT NOT NULL, user_id TEXT NOT NULL, amount_cents BIGINT NOT NULL, currency TEXT NOT NULL, admin_comment TEXT, reviewed_by TEXT, reviewed_at TIMESTAMPTZ, updated_at TIMESTAMPTZ, completed_at TIMESTAMPTZ)`,
		`CREATE TABLE wallets (user_id TEXT PRIMARY KEY REFERENCES users(id), balance_cents BIGINT NOT NULL DEFAULT 0 CHECK (balance_cents >= 0), currency TEXT NOT NULL DEFAULT 'USD', status TEXT NOT NULL DEFAULT 'active', created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE wallet_ledger (id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id), type TEXT NOT NULL, amount_cents BIGINT NOT NULL, balance_after_cents BIGINT NOT NULL, ref_type TEXT, ref_id TEXT, description TEXT, reason_code TEXT, idempotency_key TEXT UNIQUE, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
	}
	for _, statement := range statements {
		if _, err := sqlDB.ExecContext(ctx, statement); err != nil {
			t.Fatalf("schema setup: %v", err)
		}
	}
	defer sqlDB.ExecContext(context.Background(), `DROP TABLE IF EXISTS wallet_ledger, wallets, audit_logs, payouts, role_permissions, user_roles, permissions, roles, users CASCADE`)

	password := "Local-SEC004-Test-Password-Only"
	passwordHash, err := auth.HashPassword(password, auth.DefaultArgon2idParams())
	if err != nil {
		t.Fatal(err)
	}
	seedStatements := []struct {
		query string
		args  []interface{}
	}{
		{`INSERT INTO users(id,email,password_hash,status) VALUES
		 ('admin-1','admin@test.invalid',$1,'active'),
		 ('support-1','support@test.invalid',$1,'active'),
		 ('user-1','user@test.invalid',$1,'active')`, []interface{}{passwordHash}},
		{`INSERT INTO roles(name) VALUES ('user'),('support_admin'),('super_admin')`, nil},
		{`INSERT INTO permissions(name) VALUES ('withdrawals.manage'),('users.edit'),('users.wallet.charge'),('kyc.review')`, nil},
		{`INSERT INTO user_roles(user_id,role_id) SELECT 'admin-1',id FROM roles WHERE name='super_admin'`, nil},
		{`INSERT INTO user_roles(user_id,role_id) SELECT 'support-1',id FROM roles WHERE name='support_admin'`, nil},
		{`INSERT INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM roles r CROSS JOIN permissions p WHERE r.name='super_admin'`, nil},
		{`INSERT INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM roles r JOIN permissions p ON p.name='kyc.review' WHERE r.name='support_admin'`, nil},
		{`INSERT INTO payouts(id,status,user_id,amount_cents,currency) VALUES ('withdrawal-1','processing','user-1',5000,'USD')`, nil},
		{`INSERT INTO wallets(user_id,balance_cents) VALUES ('user-1',10000)`, nil},
	}
	for _, seed := range seedStatements {
		if _, err := sqlDB.ExecContext(ctx, seed.query, seed.args...); err != nil {
			t.Fatal(err)
		}
	}

	authConfig := auth.DefaultConfig()
	authConfig.Context = auth.ContextAdmin
	authConfig.JWTSecret = "local-sec004-admin-access-key-material-only"
	authConfig.JWTRefreshSecret = "local-sec004-admin-refresh-key-material-only"
	authConfig.JWTIssuer = "sec004-local-test"
	authConfig.JWTAudience = auth.AudienceAdmin
	authConfig.Redis = redisClient
	authConfig.SessionPrefix = auth.AdminSessionPrefix
	authConfig.RevocationPrefix = auth.AdminRevocationPrefix
	authConfig.SessionTTL = time.Hour
	authService := auth.New(authConfig)
	sessionStore := authService.Session
	sessionID, err := sessionStore.Create(ctx, &auth.Session{UserID: "admin-1", Roles: []string{auth.RoleSuperAdmin}})
	if err != nil {
		t.Fatal(err)
	}
	grantService, err := auth.NewReauthenticationService(auth.NewRedisReauthenticationGrantStore(redisClient, auth.AdminReauthenticationPrefix), 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	obs, err := observability.New(ctx, observability.Config{Service: "sec004-test", Env: "test", LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	defer obs.Shutdown(context.Background())
	app := &App{
		pool: db.NewPoolFromDB(sqlDB), auth: authService, reauthentication: grantService,
		obs: obs, circuits: NewCircuitBreakers(CircuitBreakerConfig{Logger: obs.Logger.Logger}),
		redis: applicationRedis, walletService: wallet.NewService(sqlDB),
	}
	adminCtx := sec004Context(ctx, "admin-1", sessionID)

	issueViaHandler := func(t *testing.T, action, resource, suppliedPassword string) string {
		t.Helper()
		body, _ := json.Marshal(adminReauthenticationRequest{Password: suppliedPassword, Action: action, ResourceID: resource})
		req := httptest.NewRequest(http.MethodPost, "/api/admin/reauthenticate", bytes.NewReader(body)).WithContext(adminCtx)
		rec := httptest.NewRecorder()
		app.handleAdminReauthenticate(rec, req)
		if suppliedPassword != password {
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("wrong password status=%d body=%s", rec.Code, rec.Body.String())
			}
			return ""
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("issue status=%d body=%s", rec.Code, rec.Body.String())
		}
		var response struct {
			Grant string `json:"grant"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response.Grant == "" {
			t.Fatalf("decode grant: %v", err)
		}
		return response.Grant
	}

	installAuditFailure := func(t *testing.T, action string) func() {
		t.Helper()
		if strings.ContainsAny(action, "'\"") {
			t.Fatal("unsafe test audit action")
		}
		statement := fmt.Sprintf(`
			CREATE OR REPLACE FUNCTION sec004_reject_selected_audit() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN IF NEW.action=TG_ARGV[0] THEN RAISE EXCEPTION 'controlled audit failure'; END IF; RETURN NEW; END $$;
			DROP TRIGGER IF EXISTS sec004_reject_selected_audit ON audit_logs;
			CREATE TRIGGER sec004_reject_selected_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION sec004_reject_selected_audit('%s')`, action)
		if _, err := sqlDB.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
		return func() {
			_, _ = sqlDB.ExecContext(context.Background(), `DROP TRIGGER IF EXISTS sec004_reject_selected_audit ON audit_logs; DROP FUNCTION IF EXISTS sec004_reject_selected_audit()`)
		}
	}

	t.Run("password and canonical authorization", func(t *testing.T) {
		issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", "wrong-password")
		state, category, err := app.validateSensitiveActor(ctx, "support-1", "withdrawals.manage")
		if err == nil || category != "role_denied" || state.hasRole(auth.RoleSuperAdmin) {
			t.Fatalf("Support Admin sensitive action accepted: category=%s err=%v", category, err)
		}
	})

	t.Run("single-use concurrency and non-recoverable storage", func(t *testing.T) {
		grant := issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		var successes atomic.Int32
		var wg sync.WaitGroup
		for range 24 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(adminCtx)
				req.Header.Set(adminReauthenticationHeader, grant)
				if app.consumeSensitiveGrant(req, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage") == nil {
					successes.Add(1)
				}
			}()
		}
		wg.Wait()
		if successes.Load() != 1 {
			t.Fatalf("successful concurrent consumes=%d, want 1", successes.Load())
		}
		keys, _ := redisClient.Keys(ctx, auth.AdminReauthenticationPrefix+"*").Result()
		for _, key := range keys {
			value, _ := redisClient.Get(ctx, key).Result()
			if strings.Contains(value, grant) || strings.Contains(value, password) {
				t.Fatalf("plaintext credential persisted in Redis key %s", key)
			}
		}
	})

	t.Run("real Redis expiry and safe Support Admin denial audit", func(t *testing.T) {
		state, _, err := app.validateSensitiveActor(ctx, "admin-1", "withdrawals.manage")
		if err != nil {
			t.Fatal(err)
		}
		expiringService, err := auth.NewReauthenticationService(
			auth.NewRedisReauthenticationGrantStore(redisClient, "test:sec004:expiry:"), 100*time.Millisecond,
		)
		if err != nil {
			t.Fatal(err)
		}
		expiringGrant, _, err := expiringService.Issue(ctx, app.reauthenticationExpectation(adminCtx, actionWithdrawalComplete, "withdrawal-1", state))
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
		if err := expiringService.Consume(ctx, expiringGrant, app.reauthenticationExpectation(adminCtx, actionWithdrawalComplete, "withdrawal-1", state)); !errors.Is(err, auth.ErrReauthenticationExpired) {
			t.Fatalf("real Redis expiry error=%v", err)
		}

		supportSession, err := sessionStore.Create(ctx, &auth.Session{UserID: "support-1", Roles: []string{auth.RoleSupportAdmin}})
		if err != nil {
			t.Fatal(err)
		}
		grant := issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		supportCtx := sec004Context(ctx, "support-1", supportSession)
		req := httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(supportCtx)
		req.Header.Set(adminReauthenticationHeader, grant)
		if app.consumeSensitiveGrant(req, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage") == nil {
			t.Fatal("Support Admin destructive action accepted")
		}
		var denials int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE actor_user_id='support-1' AND action='admin.sensitive_action.denied' AND payload_json->>'failure_category'='role_denied'`).Scan(&denials); err != nil || denials != 1 {
			t.Fatalf("Support Admin denial audit count=%d err=%v", denials, err)
		}
	})

	t.Run("binding and invalidation", func(t *testing.T) {
		state, _, err := app.validateSensitiveActor(ctx, "admin-1", "withdrawals.manage")
		if err != nil {
			t.Fatal(err)
		}
		grant, _, err := grantService.Issue(ctx, app.reauthenticationExpectation(adminCtx, actionWithdrawalComplete, "withdrawal-1", state))
		if err != nil {
			t.Fatal(err)
		}
		wrongSessionID, err := sessionStore.Create(ctx, &auth.Session{UserID: "admin-1", Roles: []string{auth.RoleSuperAdmin}})
		if err != nil {
			t.Fatal(err)
		}
		wrongCtx := sec004Context(ctx, "admin-1", wrongSessionID)
		wrongReq := httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(wrongCtx)
		wrongReq.Header.Set(adminReauthenticationHeader, grant)
		if app.consumeSensitiveGrant(wrongReq, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage") == nil {
			t.Fatal("wrong session accepted")
		}
		assertDenialCategory := func(category string) {
			t.Helper()
			var count int
			if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE actor_user_id='admin-1' AND action='admin.sensitive_action.denied' AND payload_json->>'failure_category'=$1`, category).Scan(&count); err != nil || count < 1 {
				t.Fatalf("denial category %s count=%d err=%v", category, count, err)
			}
		}
		assertDenialCategory("wrong_session_grant")

		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		wrongActionReq := httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(adminCtx)
		wrongActionReq.Header.Set(adminReauthenticationHeader, grant)
		if app.consumeSensitiveGrant(wrongActionReq, actionWalletAdjust, "withdrawal-1", "withdrawals.manage") == nil {
			t.Fatal("wrong action accepted")
		}
		assertDenialCategory("wrong_action_grant")

		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		wrongResourceReq := httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(adminCtx)
		wrongResourceReq.Header.Set(adminReauthenticationHeader, grant)
		if app.consumeSensitiveGrant(wrongResourceReq, actionWithdrawalComplete, "withdrawal-2", "withdrawals.manage") == nil {
			t.Fatal("wrong resource accepted")
		}
		assertDenialCategory("wrong_resource_grant")

		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		replayReq := httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(adminCtx)
		replayReq.Header.Set(adminReauthenticationHeader, grant)
		if err := app.consumeSensitiveGrant(replayReq, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage"); err != nil {
			t.Fatalf("valid grant rejected before replay test: %v", err)
		}
		if app.consumeSensitiveGrant(replayReq, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage") == nil {
			t.Fatal("replayed grant accepted")
		}
		assertDenialCategory("grant_replay")

		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		newHash, _ := auth.HashPassword("Changed-Local-Password-Only", auth.DefaultArgon2idParams())
		if _, err := sqlDB.ExecContext(ctx, `UPDATE users SET password_hash=$1 WHERE id='admin-1'`, newHash); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(adminCtx)
		req.Header.Set(adminReauthenticationHeader, grant)
		if app.consumeSensitiveGrant(req, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage") == nil {
			t.Fatal("password change did not invalidate grant")
		}
		if _, err := sqlDB.ExecContext(ctx, `UPDATE users SET password_hash=$1 WHERE id='admin-1'`, passwordHash); err != nil {
			t.Fatal(err)
		}

		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		if _, err := sqlDB.ExecContext(ctx, `DELETE FROM role_permissions rp USING roles r, permissions p WHERE rp.role_id=r.id AND rp.permission_id=p.id AND r.name='super_admin' AND p.name='withdrawals.manage'`); err != nil {
			t.Fatal(err)
		}
		req = httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(adminCtx)
		req.Header.Set(adminReauthenticationHeader, grant)
		if app.consumeSensitiveGrant(req, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage") == nil {
			t.Fatal("permission change did not invalidate grant")
		}
		if _, err := sqlDB.ExecContext(ctx, `INSERT INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM roles r,permissions p WHERE r.name='super_admin' AND p.name='withdrawals.manage'`); err != nil {
			t.Fatal(err)
		}

		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		if _, err := sqlDB.ExecContext(ctx, `DELETE FROM user_roles ur USING roles r WHERE ur.role_id=r.id AND ur.user_id='admin-1' AND r.name='super_admin'`); err != nil {
			t.Fatal(err)
		}
		req = httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(adminCtx)
		req.Header.Set(adminReauthenticationHeader, grant)
		if app.consumeSensitiveGrant(req, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage") == nil {
			t.Fatal("role change did not invalidate grant")
		}
		if _, err := sqlDB.ExecContext(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT 'admin-1',id FROM roles WHERE name='super_admin'`); err != nil {
			t.Fatal(err)
		}

		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		if err := sessionStore.Delete(ctx, sessionID); err != nil {
			t.Fatal(err)
		}
		req = httptest.NewRequest(http.MethodPost, "/consume", nil).WithContext(adminCtx)
		req.Header.Set(adminReauthenticationHeader, grant)
		if app.consumeSensitiveGrant(req, actionWithdrawalComplete, "withdrawal-1", "withdrawals.manage") == nil {
			t.Fatal("session revocation did not invalidate grant")
		}
		newSession, err := sessionStore.Create(ctx, &auth.Session{UserID: "admin-1", Roles: []string{auth.RoleSuperAdmin}})
		if err != nil {
			t.Fatal(err)
		}
		sessionID = newSession
		adminCtx = sec004Context(ctx, "admin-1", sessionID)
	})

	t.Run("wallet adjustment grant reason audit rollback and replay", func(t *testing.T) {
		invoke := func(grant string) *httptest.ResponseRecorder {
			req := sec004RouteRequest(t, adminCtx, http.MethodPost, "/api/admin/users/user-1/wallet/charge", "user_id", "user-1", `{"amount":-1000,"reason":"controlled correction","confirm_debit":true}`)
			if grant != "" {
				req.Header.Set(adminReauthenticationHeader, grant)
			}
			rec := httptest.NewRecorder()
			app.requireSensitiveAction(actionWalletAdjust, "users.wallet.charge", "user_id")(http.HandlerFunc(app.handleChargeUserWallet)).ServeHTTP(rec, req)
			return rec
		}

		cleanupFailure := installAuditFailure(t, "user.wallet.charged")
		grant := issueViaHandler(t, actionWalletAdjust, "user-1", password)
		if rec := invoke(grant); rec.Code != http.StatusInternalServerError {
			t.Fatalf("wallet audit failure status=%d body=%s", rec.Code, rec.Body.String())
		}
		var balance int64
		var ledgerCount int
		if err := sqlDB.QueryRowContext(ctx, `SELECT balance_cents FROM wallets WHERE user_id='user-1'`).Scan(&balance); err != nil || balance != 10000 {
			t.Fatalf("wallet audit failure did not roll back balance=%d err=%v", balance, err)
		}
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM wallet_ledger WHERE user_id='user-1'`).Scan(&ledgerCount); err != nil || ledgerCount != 0 {
			t.Fatalf("wallet audit failure did not roll back ledger=%d err=%v", ledgerCount, err)
		}
		cleanupFailure()

		grant = issueViaHandler(t, actionWalletAdjust, "user-1", password)
		if rec := invoke(grant); rec.Code != http.StatusOK {
			t.Fatalf("wallet adjustment status=%d body=%s", rec.Code, rec.Body.String())
		}
		if err := sqlDB.QueryRowContext(ctx, `SELECT balance_cents FROM wallets WHERE user_id='user-1'`).Scan(&balance); err != nil || balance != 9000 {
			t.Fatalf("wallet adjustment balance=%d err=%v", balance, err)
		}
		if rec := invoke(grant); rec.Code != http.StatusForbidden {
			t.Fatalf("wallet grant replay status=%d body=%s", rec.Code, rec.Body.String())
		}
		if rec := invoke(""); rec.Code != http.StatusForbidden {
			t.Fatalf("wallet missing grant status=%d body=%s", rec.Code, rec.Body.String())
		}
		grant = issueViaHandler(t, actionWalletAdjust, "user-1", password)
		missingReasonReq := sec004RouteRequest(t, adminCtx, http.MethodPost, "/api/admin/users/user-1/wallet/charge", "user_id", "user-1", `{"amount":-100,"reason":"","confirm_debit":true}`)
		missingReasonReq.Header.Set(adminReauthenticationHeader, grant)
		missingReasonRec := httptest.NewRecorder()
		app.requireSensitiveAction(actionWalletAdjust, "users.wallet.charge", "user_id")(http.HandlerFunc(app.handleChargeUserWallet)).ServeHTTP(missingReasonRec, missingReasonReq)
		if missingReasonRec.Code != http.StatusBadRequest {
			t.Fatalf("wallet missing reason status=%d body=%s", missingReasonRec.Code, missingReasonRec.Body.String())
		}
		var reasonDenials int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='admin.sensitive_action.denied' AND target_id='user-1' AND payload_json->>'action'=$1 AND payload_json->>'failure_category'='mandatory_reason_denied'`, actionWalletAdjust).Scan(&reasonDenials); err != nil || reasonDenials < 1 {
			t.Fatalf("wallet mandatory reason audit count=%d err=%v", reasonDenials, err)
		}
	})

	t.Run("role change grant reason audit rollback and session invalidation", func(t *testing.T) {
		targetSessionStore := auth.NewSessionStore(&auth.SessionStoreConfig{Redis: redisClient, KeyPrefix: auth.UserSessionPrefix, TTL: time.Hour})
		targetSessionID, err := targetSessionStore.Create(ctx, &auth.Session{UserID: "user-1", Roles: []string{auth.RoleUser}})
		if err != nil {
			t.Fatal(err)
		}
		invoke := func(grant string) *httptest.ResponseRecorder {
			req := sec004RouteRequest(t, adminCtx, http.MethodPatch, "/api/admin/users/user-1/roles", "user_id", "user-1", `{"roles":["user","support_admin"],"reason":"approved support assignment"}`)
			if grant != "" {
				req.Header.Set(adminReauthenticationHeader, grant)
			}
			rec := httptest.NewRecorder()
			app.requireSensitiveAction(actionUserRolesUpdate, "users.edit", "user_id")(http.HandlerFunc(app.handleUpdateUserRoles)).ServeHTTP(rec, req)
			return rec
		}

		cleanupFailure := installAuditFailure(t, "user.roles.updated")
		grant := issueViaHandler(t, actionUserRolesUpdate, "user-1", password)
		if rec := invoke(grant); rec.Code != http.StatusInternalServerError {
			t.Fatalf("role audit failure status=%d body=%s", rec.Code, rec.Body.String())
		}
		var supportRoleCount int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id='user-1' AND r.name='support_admin'`).Scan(&supportRoleCount); err != nil || supportRoleCount != 0 {
			t.Fatalf("role audit failure did not roll back role count=%d err=%v", supportRoleCount, err)
		}
		if _, err := targetSessionStore.Get(ctx, targetSessionID); err != nil {
			t.Fatalf("failed role update invalidated session: %v", err)
		}
		cleanupFailure()

		grant = issueViaHandler(t, actionUserRolesUpdate, "user-1", password)
		if rec := invoke(grant); rec.Code != http.StatusOK {
			t.Fatalf("role update status=%d body=%s", rec.Code, rec.Body.String())
		}
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id='user-1' AND r.name='support_admin'`).Scan(&supportRoleCount); err != nil || supportRoleCount != 1 {
			t.Fatalf("role update support count=%d err=%v", supportRoleCount, err)
		}
		if _, err := targetSessionStore.Get(ctx, targetSessionID); err == nil {
			t.Fatal("role update did not invalidate target session")
		}
		if rec := invoke(grant); rec.Code != http.StatusForbidden {
			t.Fatalf("role grant replay status=%d body=%s", rec.Code, rec.Body.String())
		}
		grant = issueViaHandler(t, actionUserRolesUpdate, "user-1", password)
		missingReasonReq := sec004RouteRequest(t, adminCtx, http.MethodPatch, "/api/admin/users/user-1/roles", "user_id", "user-1", `{"roles":["user","support_admin"],"reason":""}`)
		missingReasonReq.Header.Set(adminReauthenticationHeader, grant)
		missingReasonRec := httptest.NewRecorder()
		app.requireSensitiveAction(actionUserRolesUpdate, "users.edit", "user_id")(http.HandlerFunc(app.handleUpdateUserRoles)).ServeHTTP(missingReasonRec, missingReasonReq)
		if missingReasonRec.Code != http.StatusBadRequest {
			t.Fatalf("role missing reason status=%d body=%s", missingReasonRec.Code, missingReasonRec.Body.String())
		}
		var reasonDenials int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='admin.sensitive_action.denied' AND target_id='user-1' AND payload_json->>'action'=$1 AND payload_json->>'failure_category'='mandatory_reason_denied'`, actionUserRolesUpdate).Scan(&reasonDenials); err != nil || reasonDenials < 1 {
			t.Fatalf("role mandatory reason audit count=%d err=%v", reasonDenials, err)
		}
	})

	t.Run("elevated account creation grant reason and mandatory audit", func(t *testing.T) {
		const targetEmail = "new-support@test.invalid"
		invoke := func(grant string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(`{"email":"new-support@test.invalid","password":"Local-Support-Password-Only1!","roles":["user","support_admin"],"reason":"approved support staffing"}`)).WithContext(adminCtx)
			if grant != "" {
				req.Header.Set(adminReauthenticationHeader, grant)
			}
			rec := httptest.NewRecorder()
			app.handleAdminCreateUser(rec, req)
			return rec
		}

		cleanupFailure := installAuditFailure(t, "user.created_by_admin")
		grant := issueViaHandler(t, actionElevatedUserCreate, targetEmail, password)
		if rec := invoke(grant); rec.Code != http.StatusInternalServerError {
			t.Fatalf("creation audit failure status=%d body=%s", rec.Code, rec.Body.String())
		}
		var created int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email=$1`, targetEmail).Scan(&created); err != nil || created != 0 {
			t.Fatalf("creation audit failure did not roll back user count=%d err=%v", created, err)
		}
		cleanupFailure()

		grant = issueViaHandler(t, actionElevatedUserCreate, targetEmail, password)
		if rec := invoke(grant); rec.Code != http.StatusCreated {
			t.Fatalf("elevated creation status=%d body=%s", rec.Code, rec.Body.String())
		}
		var auditCount int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles r ON r.id=ur.role_id WHERE u.email=$1 AND r.name='support_admin'`, targetEmail).Scan(&created); err != nil || created != 1 {
			t.Fatalf("elevated creation support assignment=%d err=%v", created, err)
		}
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='user.created_by_admin' AND payload_json->>'reason'='approved support staffing'`).Scan(&auditCount); err != nil || auditCount != 1 {
			t.Fatalf("elevated creation audit count=%d err=%v", auditCount, err)
		}
		if rec := invoke(grant); rec.Code != http.StatusForbidden {
			t.Fatalf("elevated creation grant replay status=%d body=%s", rec.Code, rec.Body.String())
		}
		grant = issueViaHandler(t, actionElevatedUserCreate, "another-support@test.invalid", password)
		missingReasonReq := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(`{"email":"another-support@test.invalid","password":"Local-Support-Password-Only2!","roles":["user","support_admin"],"reason":""}`)).WithContext(adminCtx)
		missingReasonReq.Header.Set(adminReauthenticationHeader, grant)
		missingReasonRec := httptest.NewRecorder()
		app.handleAdminCreateUser(missingReasonRec, missingReasonReq)
		if missingReasonRec.Code != http.StatusBadRequest {
			t.Fatalf("elevated creation missing reason status=%d body=%s", missingReasonRec.Code, missingReasonRec.Body.String())
		}
		var reasonDenials int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='admin.sensitive_action.denied' AND target_id='another-support@test.invalid' AND payload_json->>'action'=$1 AND payload_json->>'failure_category'='mandatory_reason_denied'`, actionElevatedUserCreate).Scan(&reasonDenials); err != nil || reasonDenials < 1 {
			t.Fatalf("elevated creation mandatory reason audit count=%d err=%v", reasonDenials, err)
		}
	})
	t.Run("transactional audit rollback and recovery", func(t *testing.T) {
		grant := issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		missingReasonReq := sec004RouteRequest(t, adminCtx, http.MethodPost, "/api/admin/withdrawals/withdrawal-1/complete", "id", "withdrawal-1", `{"comment":"","transaction_id":"local-test-ref"}`)
		missingReasonReq.Header.Set(adminReauthenticationHeader, grant)
		missingReasonRec := httptest.NewRecorder()
		app.requireSensitiveAction(actionWithdrawalComplete, "withdrawals.manage", "id")(http.HandlerFunc(app.handleCompleteWithdrawal)).ServeHTTP(missingReasonRec, missingReasonReq)
		if missingReasonRec.Code != http.StatusBadRequest {
			t.Fatalf("withdrawal missing reason status=%d body=%s", missingReasonRec.Code, missingReasonRec.Body.String())
		}
		var reasonDenials int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='admin.sensitive_action.denied' AND target_id='withdrawal-1' AND payload_json->>'action'=$1 AND payload_json->>'failure_category'='mandatory_reason_denied'`, actionWithdrawalComplete).Scan(&reasonDenials); err != nil || reasonDenials < 1 {
			t.Fatalf("withdrawal mandatory reason audit count=%d err=%v", reasonDenials, err)
		}

		if _, err := sqlDB.ExecContext(ctx, `
			CREATE OR REPLACE FUNCTION sec004_reject_completion_audit() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN IF NEW.action='withdrawal.completed' THEN RAISE EXCEPTION 'controlled audit failure'; END IF; RETURN NEW; END $$;
			CREATE TRIGGER sec004_reject_completion_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION sec004_reject_completion_audit()`); err != nil {
			t.Fatal(err)
		}
		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		req := sec004RouteRequest(t, adminCtx, http.MethodPost, "/api/admin/withdrawals/withdrawal-1/complete", "id", "withdrawal-1", `{"comment":"manual payout confirmed","transaction_id":"local-test-ref"}`)
		req.Header.Set(adminReauthenticationHeader, grant)
		rec := httptest.NewRecorder()
		app.requireSensitiveAction(actionWithdrawalComplete, "withdrawals.manage", "id")(http.HandlerFunc(app.handleCompleteWithdrawal)).ServeHTTP(rec, req)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("controlled audit failure status=%d body=%s", rec.Code, rec.Body.String())
		}
		var status string
		if err := sqlDB.QueryRowContext(ctx, `SELECT status FROM payouts WHERE id='withdrawal-1'`).Scan(&status); err != nil || status != "processing" {
			t.Fatalf("mutation not rolled back: status=%s err=%v", status, err)
		}
		if _, err := sqlDB.ExecContext(ctx, `DROP TRIGGER sec004_reject_completion_audit ON audit_logs; DROP FUNCTION sec004_reject_completion_audit()`); err != nil {
			t.Fatal(err)
		}

		grant = issueViaHandler(t, actionWithdrawalComplete, "withdrawal-1", password)
		req = sec004RouteRequest(t, adminCtx, http.MethodPost, "/api/admin/withdrawals/withdrawal-1/complete", "id", "withdrawal-1", `{"comment":"manual payout confirmed","transaction_id":"local-test-ref"}`)
		req.Header.Set(adminReauthenticationHeader, grant)
		rec = httptest.NewRecorder()
		app.requireSensitiveAction(actionWithdrawalComplete, "withdrawals.manage", "id")(http.HandlerFunc(app.handleCompleteWithdrawal)).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("recovery status=%d body=%s", rec.Code, rec.Body.String())
		}
		var auditCount int
		if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action='withdrawal.completed' AND target_id='withdrawal-1'`).Scan(&auditCount); err != nil || auditCount != 1 {
			t.Fatalf("completion audit count=%d err=%v", auditCount, err)
		}
	})

	var leaked int
	if err := sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE payload_json::text LIKE '%Local-SEC004-Test-Password-Only%' OR payload_json::text LIKE '%opaque%'`).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("credential-like audit payloads=%d err=%v", leaked, err)
	}
	var payoutStatus string
	if err := sqlDB.QueryRowContext(ctx, `SELECT status FROM payouts WHERE id='withdrawal-1'`).Scan(&payoutStatus); err != nil || payoutStatus != "succeeded" {
		t.Fatalf("final payout status=%s err=%v", payoutStatus, err)
	}
	t.Logf("runtime evidence: PostgreSQL and Redis validated, final payout=%s", payoutStatus)
}

func TestSEC004CanonicalRoleMigrationPostgres(t *testing.T) {
	dsn, _ := requireSEC004Runtime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	reset := func() {
		if _, err := database.ExecContext(ctx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public`); err != nil {
			t.Fatal(err)
		}
	}
	reset()
	defer reset()

	migration := func(name string) {
		path := filepath.Join("..", "..", "..", "packages", "db", "migrations", name)
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.ExecContext(ctx, string(contents)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	migration("0001_init.up.sql")
	migration("0024_admin_roles.up.sql")
	var userID string
	if err := database.QueryRowContext(ctx, `INSERT INTO users(email,password_hash) VALUES ('legacy-admin@test.invalid','test-hash') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE name='admin'`, userID); err != nil {
		t.Fatal(err)
	}
	migration("0099_admin_canonical_roles.up.sql")

	var supportAssignments, legacyAssignments, nonKYCGrants, financeRoles int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND r.name='support_admin'`, userID).Scan(&supportAssignments); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND r.name='admin'`, userID).Scan(&legacyAssignments); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM role_permissions rp JOIN roles r ON r.id=rp.role_id JOIN permissions p ON p.id=rp.permission_id WHERE r.name='support_admin' AND p.name NOT IN ('kyc.view','kyc.review')`).Scan(&nonKYCGrants); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles WHERE lower(name)='finance'`).Scan(&financeRoles); err != nil {
		t.Fatal(err)
	}
	if supportAssignments != 1 || legacyAssignments != 0 || nonKYCGrants != 0 || financeRoles != 0 {
		t.Fatalf("up migration mismatch support=%d legacy=%d non_kyc=%d finance=%d", supportAssignments, legacyAssignments, nonKYCGrants, financeRoles)
	}

	migration("0099_admin_canonical_roles.down.sql")
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND r.name='admin'`, userID).Scan(&legacyAssignments); err != nil {
		t.Fatal(err)
	}
	if legacyAssignments != 1 {
		t.Fatalf("development down did not restore legacy assignment: %d", legacyAssignments)
	}
	migration("0099_admin_canonical_roles.up.sql")
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=$1 AND r.name='support_admin'`, userID).Scan(&supportAssignments); err != nil {
		t.Fatal(err)
	}
	if supportAssignments != 1 {
		t.Fatalf("reapplication not deterministic: support assignments=%d", supportAssignments)
	}
	t.Log("migration evidence: 0001 + 0024 + 0099 up/down/up succeeded on PostgreSQL 16")
}

func TestCanonicalAdminRoleAndPermissionPolicy(t *testing.T) {
	cases := []struct {
		name      string
		roles     []string
		canonical bool
		admin     bool
	}{
		{"Support Admin", []string{auth.RoleUser, auth.RoleSupportAdmin}, true, true},
		{"Super Admin", []string{auth.RoleSuperAdmin}, true, true},
		{"User", []string{auth.RoleUser}, true, false},
		{"legacy admin", []string{"admin"}, false, false},
		{"legacy viewer", []string{"viewer"}, false, false},
		{"Finance role", []string{"finance"}, false, false},
		{"unknown", []string{"unknown"}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canonical, admin := canonicalAdminRoles(tc.roles)
			if canonical != tc.canonical || admin != tc.admin {
				t.Fatalf("canonical=%t admin=%t", canonical, admin)
			}
		})
	}
	state := adminSecurityState{Roles: []string{auth.RoleSupportAdmin}, Permissions: []string{"kyc.review", "users.edit", "support.tickets"}}
	if got := fmt.Sprint(effectiveAdminPermissions(state)); got != "[kyc.review support.tickets]" {
		t.Fatalf("Support Admin permissions=%s", got)
	}
}
