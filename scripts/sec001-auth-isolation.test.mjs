import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const read = (relative) => fs.readFileSync(path.join(root, relative), "utf8");

const runtimeFiles = [
  "apps/api-server/main.go",
  "apps/api-server/auth_contexts.go",
  "apps/user-bff/server/app.go",
  "apps/admin-bff/server/app.go",
  "apps/trade-bff/server/app.go",
  "apps/payment-service/server/app.go",
  "apps/payment-service/server/config.go",
  "apps/shard-router/config.go",
  "apps/shard-router/main.go",
  "apps/settlement-service/server/config.go",
  "apps/settlement-service/server/app.go",
];

test("merged API constructs and injects separate User/Admin contexts", () => {
  const main = read("apps/api-server/main.go");
  const construction = read("apps/api-server/auth_contexts.go");
  assert.match(main, /userAuth, adminAuth, err := buildAuthContexts/);
  assert.match(main, /userserver\.RunWithSharedDeps\([^\n]+userAuth\)/);
  assert.match(main, /adminserver\.RunWithSharedDeps\([^\n]+adminAuth\)/);
  assert.match(main, /paymentserver\.RunWithSharedDeps\([^\n]+userAuth\)/);
  assert.doesNotMatch(main, /authService := auth\.New|shared auth service created/);
  assert.match(construction, /userAuth == adminAuth/);
});

test("active User/Admin consumers reject wrong-context injection", () => {
  for (const relative of runtimeFiles.slice(2, 6)) {
    const source = read(relative);
    const expected = relative.includes("admin-bff") ? "ContextAdmin" : "ContextUser";
    assert.match(source, new RegExp(`sharedAuth\\.Context\\(\\) != auth\\.${expected}`), relative);
  }
});

test("standalone Admin-protected services construct only the explicit Admin context", () => {
  for (const relative of [
    "apps/shard-router/config.go",
    "apps/shard-router/main.go",
    "apps/settlement-service/server/config.go",
    "apps/settlement-service/server/app.go",
  ]) {
    const source = read(relative);
    assert.doesNotMatch(source, /auth\.DefaultConfig\(|JWTSecret|JWTRefreshSecret/, relative);
  }
  assert.match(read("apps/shard-router/config.go"), /AuthContext: authIsolation\.Admin/);
  assert.match(read("apps/shard-router/main.go"), /auth\.NewContext\(cfg\.AuthContext/);
  assert.match(read("apps/settlement-service/server/config.go"), /AuthContext: authIsolation\.Admin/);
  assert.match(read("apps/settlement-service/server/app.go"), /auth\.NewContext\(app\.config\.AuthContext/);
});
test("token validators enforce key purpose, issuer, exact audience, algorithm, and context", () => {
  const jwt = read("packages/auth/jwt.go");
  assert.match(jwt, /WithValidMethods/);
  assert.match(jwt, /WithIssuer/);
  assert.match(jwt, /len\(claims\.Audience\) != 1/);
  assert.match(jwt, /claims\.AuthContext != s\.config\.Context/);
  assert.match(jwt, /ValidateAccessToken/);
  assert.match(jwt, /ValidateRefreshToken/);
  const auth = read("packages/auth/auth.go");
  assert.ok(auth.indexOf("a.Token.ValidateRefreshToken") < auth.indexOf("a.Session.ValidateRefreshToken"));
});

test("production configuration defines four secrets and separated metadata", () => {
  const isolation = read("packages/auth/isolation.go");
  for (const name of [
    "JWT_SECRET_USER",
    "JWT_REFRESH_SECRET_USER",
    "JWT_SECRET_ADMIN",
    "JWT_REFRESH_SECRET_ADMIN",
    "JWT_ISSUER_USER",
    "JWT_ISSUER_ADMIN",
    "JWT_AUDIENCE_USER",
    "JWT_AUDIENCE_ADMIN",
    "USER_FRONTEND_ORIGIN",
    "ADMIN_FRONTEND_ORIGIN",
  ]) {
    assert.ok(isolation.includes(name), name);
  }
  assert.match(isolation, /at least 32 bytes/);
  assert.match(isolation, /does not meet production entropy checks/);
  assert.match(isolation, /must not use a default or placeholder value/);
});

test("cookies, sessions, revocation, login state, and CSRF are context-namespaced", () => {
  const isolation = read("packages/auth/isolation.go");
  for (const value of [
    "session:user:", "session:admin:",
    "jwt_blacklist:user:", "jwt_blacklist:admin:",
    "refresh_token_user", "refresh_token_admin",
    "csrf:user", "csrf:admin",
  ]) {
    assert.ok(isolation.includes(value), value);
  }
  const user = [
    read("apps/user-bff/server/auth_handlers.go"),
    read("apps/user-bff/server/forgot_password_handlers.go"),
    read("apps/user-bff/server/helpers.go"),
  ].join("\n");
  assert.match(user, /auth:user:/);
  const admin = read("apps/admin-bff/server/handlers_helpers.go");
  assert.match(admin, /auth:admin:/);
  const csrf = read("packages/validation/csrf.go");
  assert.match(csrf, /auth\.UserCSRFContext/);
  assert.match(csrf, /auth\.AdminCSRFContext/);
});

test("Docker runtime mounts four independently generated signing secrets", () => {
  const compose = read("infra/docker/docker-compose.yml");
  for (const secret of [
    "jwt_secret_user", "jwt_refresh_secret_user",
    "jwt_secret_admin", "jwt_refresh_secret_admin",
  ]) {
    assert.ok(compose.includes(secret), secret);
  }
  const init = read("scripts/secrets/init-secrets.sh");
  assert.match(init, /openssl rand -base64 48/);
  assert.match(init, /jwt_refresh_secret_user\.txt/);
  assert.match(init, /jwt_refresh_secret_admin\.txt/);
});

test("SEC-002 removes query-token auth without weakening SEC-001 isolation", () => {
  const middleware = read("packages/auth/middleware.go");
  assert.doesNotMatch(middleware, /r\.URL\.Query\(\)\.Get\("token"\)/);
  assert.match(middleware, /HasProhibitedCredentialQuery/);
  assert.match(read("docs/security/user-admin-authentication-isolation.md"), /session authentication URL policy/);
  assert.ok(fs.existsSync(path.join(root, "docs/security/session-authentication-url-policy.md")));
});
test("changed Markdown local links resolve", () => {
  const markdownFiles = [
    "README.md",
    "docs/security/user-admin-authentication-isolation.md",
    "infra/docker/secrets/README.md",
    "docs/codex/reports/SEC-001-local-execution-report.md",
  ];
  const failures = [];
  for (const relative of markdownFiles) {
    const source = read(relative);
    const base = path.dirname(path.join(root, relative));
    for (const match of source.matchAll(/\[[^\]]+\]\(([^)]+)\)/g)) {
      const target = match[1].trim().replace(/^<|>$/g, "").split("#", 1)[0];
      if (!target || /^[a-z][a-z0-9+.-]*:/i.test(target)) continue;
      if (!fs.existsSync(path.resolve(base, target))) failures.push(`${relative} -> ${target}`);
    }
  }
  assert.deepEqual(failures, []);
});

test("no compatibility flag, Finance role, active Second Chance, or unauthorized Git metadata was introduced", () => {
  const combined = runtimeFiles.map(read).join("\n") + read("packages/auth/isolation.go");
  assert.doesNotMatch(combined, /AUTH.*COMPAT|LEGACY.*AUTH.*ENABLE/i);
  assert.doesNotMatch(combined, /Finance role|ROLE_FINANCE|RoleFinance/);
  assert.doesNotMatch(read("docs/security/user-admin-authentication-isolation.md"), /Second Chance/);
  const gitPath = path.join(root, ".git");
  if (fs.existsSync(gitPath)) {
    const config = read(".git/config");
    assert.match(config, /url\s*=\s*https:\/\/github\.com\/qopalboker\/tragge_v0\.git/i);
  }
});
