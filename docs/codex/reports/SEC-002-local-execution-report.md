# SEC-002 Local Execution Report

## 1. Task and decision

- **Task:** `SEC-002 — Remove session JWT support from URL query parameters`
- **Date/mode:** 2026-07-26; local extracted-project execution without Git
- **Decision:** **SEC-002 PASS**
- **Stop:** `SEC-003` and later tasks were not started.
- **Paid-production status:** `NO-GO`.

Normal session middleware no longer reads credentials from URLs. The affected
browser WebSocket now uses a 10-second, single-use, purpose-bound ticket plus an
HttpOnly binding cookie. The frontend has no reusable session-token URL input
or fallback. Focused telemetry and gateway logging remove credential-bearing
values. Go tests, builds, vet, race tests, the exact production WebSocket URL
module, and the 807-file structural regression pass.

Frontend pnpm commands remain unavailable because this host cannot spawn Node
(`EPERM`). They are not reported as passed. The exact production URL module has
executed direct tests, while an executed source invariant covers the complete
composable and proves there is no session-token input or fallback. This is the
reliable executed evidence used for the affected frontend behavior.

## 2. Dependency verification

| Dependency | Evidence | Result |
|---|---|---|
| Phase 0 | Exit report has current `PASS` after PostgreSQL remediation. | PASS |
| FND-002 | ADR 0001 exists and is Accepted. | PASS |
| SEC-001 | Local report records `SEC-001 PASS`. | PASS |
| Trust separation | Separate User/Admin contexts and all SEC-001 regressions pass. | PASS |
| Local mode | `.git` is absent; no Git/remote operation occurred. | PASS |

The fixed policy, roadmap and complete SEC-002 block, Codex protocol, ADR 0001,
glossary, Phase 0 report, SEC-001 report, and Phase 1 controller were read first.

## 3. Pre-change inventory

- `packages/auth/middleware.go` was the only shared normal-session query read:
  `extractBearerToken` fell back to `?token=`. Because SEC-001 constructs this
  middleware in both contexts, the fallback affected User and Admin routes.
- No active reads of `access_token`, `jwt`, `auth_token`, or `session_token`
  existed; all are now proactively rejected aliases.
- Trade BFF issued a short-lived WebSocket ticket, but it lacked independent
  binding, explicit context/purpose/session binding, hashed storage, and active
  session revalidation.
- User frontend `useWebSocket.ts` fell back to a reusable JWT URL after ticket
  acquisition failure; `TradingPage.vue` supplied that token.
- `/ws/tournaments` is public and did not consume session credentials. No Admin
  authenticated WebSocket consumer was found.
- User/Admin support-ticket attachments already used context-specific
  authenticated Axios Blob downloads, server ownership/role checks, and
  `Cache-Control: private, no-store, max-age=0`; no tokenized download URL was
  found.
- App observability used normalized paths, but Sentry could observe raw headers
  and queries. Gateway Nginx logged raw URIs/referrers in some paths; Kubernetes
  logged `$request`.
- `packages/auth/middleware_test.go` was the only test requiring accepted query
  authentication.

Justified exclusions: Finnhub's third-party provider API-key query contract and
the one-time password-reset fixture in notification email code. Neither is a
reusable Tragge session credential; SEC-003 owns reset-delivery hardening.

## 4. Final HTTP authentication

`RequireAuth` rejects query names `token`, `access_token`, `jwt`, `auth_token`,
and `session_token` case-insensitively before value validation. It returns a
generic 401/code `url_authentication_unsupported`, never echoes the value, and
extracts access credentials only from `Authorization: Bearer`. A valid header
plus a prohibited query still fails closed. SEC-001's separate User/Admin
refresh/logout cookies, CSRF contexts, validators, sessions, and revocation
namespaces are preserved. No switch restores reusable query JWT support.

## 5. Final WebSocket authentication

An authenticated User posts the Contest ID to `/api/trade/ws-ticket`. The server
checks the User access token and active User session, generates independent
256-bit ticket/binding values, and stores only SHA-256(ticket), a binding digest,
User/session, User context, purpose `trade_websocket_handshake`, Contest, and
expiration. TTL is 10 seconds; the response is `no-store`.

The binding is delivered in `tragge_ws_ticket_bind_user`, HttpOnly,
SameSite=Strict, path `/ws/trade`, and Secure in production/HTTPS. The browser
URL contains only the bounded ticket. Gateway logs redact it and Trade BFF moves
it to unexported request context before telemetry. Redis `GETDEL` enforces
single use; constant-time binding plus context, purpose, Contest, User, session,
expiry, and active-session validation prevent replay/cross-use. The cookie is
cleared on consumption. The ticket cannot become an access/refresh token.

Non-browser clients may use the context-specific User Authorization header plus
active-session check. Admin tokens fail in this User WebSocket context. Missing,
malformed, expired, replayed, wrong-binding/resource/purpose/context/User/session,
and revoked-session attempts fail generically.
## 6. Protected downloads

No affected protected download used query JWTs, so no speculative download
service/ticket was added. User/Admin support attachments retain authenticated
Blob fetch, context authorization, ownership/role checks, non-public caching,
sanitary filenames, `nosniff`, and restrictive CSP. The structural checker
verifies this pattern; both server packages pass. A live object-store download
was not executed.

## 7. Logging and analytics protections

Before observability/Sentry in User BFF, Admin BFF, and Trade BFF, focused
middleware clones the request, removes Authorization/Cookie, and redacts
credential queries. Only secure headers are restored downstream. Trade BFF also
moves the bounded ticket into unexported context and redacts its URL before
telemetry.

Nginx suppresses complete WebSocket queries, sanitizes URIs containing
credential-like names or `ticket`, and omits referrers. Kubernetes logs method,
normalized path, and protocol—not raw `$request`—and omits referrers. This is
focused SEC-002 protection, not SEC-005's generalized redaction program.

## 8. Frontend migration

User trading frontend removed the token option and JWT fallback, obtains a new
bounded ticket for every connection/reconnection, fails locally on ticket
acquisition failure, and uses `webSocketUrl.js`. The builder rejects candidate
URLs that already contain session-credential aliases. Frontend and ticket
backend should deploy together. Legacy URL-auth clients receive only a generic
unauthorized response.

## 9. Configuration and dependencies

No environment variable, startup flag, compatibility mode, dependency, package
manifest, or lockfile changed. New internal constants are the binding cookie,
`/ws/trade` path, 10-second TTL, purpose, and
`ws_ticket:user:<sha256(ticket)>` namespace. SEC-001 configuration is unchanged.

## 10. Files changed

Authentication/services:

1. `packages/auth/middleware.go`
2. `packages/auth/middleware_test.go`
3. `apps/trade-bff/server/app.go`
4. `apps/trade-bff/server/ws_ticket.go` (new)
5. `apps/trade-bff/server/ws_ticket_test.go` (new)
6. `apps/user-bff/server/app.go`
7. `apps/admin-bff/server/app.go`

Frontend:

8. `apps/user-frontend/src/modules/trade/composables/useWebSocket.ts`
9. `apps/user-frontend/src/modules/trade/composables/useWebSocket.test.ts` (new)
10. `apps/user-frontend/src/modules/trade/composables/webSocketUrl.js` (new)
11. `apps/user-frontend/src/modules/trade/composables/webSocketUrl.d.ts` (new)
12. `apps/user-frontend/src/modules/trade/composables/webSocketUrl.test.mjs` (new)
13. `apps/user-frontend/src/modules/trade/views/TradingPage.vue`

Gateway/logging:

14. `apps/gateway/nginx.conf`
15. `apps/gateway/nginx.prod.conf`
16. `infra/k8s/base/gateway.yaml`

Validation/documentation:

17. `scripts/sec-002-query-auth-check.mjs` (new)
18. `scripts/sec-002-query-auth-check.test.mjs` (new)
19. `docs/security/session-authentication-url-policy.md` (new)
20. `docs/security/user-admin-authentication-isolation.md`

Prerequisite evidence updated solely for the legitimate delta (two Go files,
one Go test, two TypeScript/TSX files, two reviewed imports) and to replace the
obsolete “SEC-002 deferred” assertion:

21. `scripts/production-baseline.mjs`
22. `scripts/production-baseline.test.mjs`
23. `docs/architecture/current-state-audit.md`
24. `scripts/target-architecture.test.mjs`
25. `docs/architecture/target-architecture-import-review.md`
26. `scripts/sec001-auth-isolation.test.mjs`
27. `docs/codex/reports/SEC-002-local-execution-report.md` (new)

Gateway/Kubernetes paths are justified because they captured query/referrer
data. Architecture/baseline evidence paths are justified by deterministic
prerequisite regressions. No unrelated behavior or later task changed.

## 11. Tests added/updated

- Auth: secure-header success; every alias rejected; query-only,
  malformed/expired fixture, invalid-header-plus-query, valid-header-plus-query;
  generic response and telemetry non-leak; downstream header restoration;
  User/Admin isolation.
- WebSocket: hashed storage, TTL, single/concurrent use, wrong
  binding/resource/purpose/context/User/session, revocation/expiry, cookie
  attributes, User success/Admin rejection, alias rejection, unauthenticated
  failure, ticket replay, endpoint no-store, telemetry redaction.
- Frontend: exact builder tests ticket-only construction, replacement on
  reconnect, and rejection of aliases. Vitest source also covers fail-closed
  acquisition and fresh ticket per reconnect.
- Structural: mutation tests detect backend query reads/frontend credential URL
  construction; current scan enforces auth, ticket, frontend, download,
  telemetry, and gateway invariants across 807 files.
## 12. Every command and exact result

Secrets and complete credentials were never printed. Editing used
assertion-guarded PowerShell `[IO.File]::WriteAllText`; expected old fragments
were checked before writes.

### 12.1 Read/inspect/edit iterations

| Command | Exact result |
|---|---|
| `Get-Location; Get-ChildItem -Force; rg --files ...` | Exit 0; chose `work/tragge-main`; authorities/layout found; `.git` absent. |
| `Get-Content` for all eight authorities plus the Phase 1 controller | Exit 0; Phase 0 `PASS`, SEC-001 `PASS`, ADR Accepted, complete SEC-002 contract read. |
| Roadmap extraction from `### SEC-002` to `### SEC-003` | Exit 0; full task block read. |
| Inventory `rg -n`/`rg --files` batches for query aliases, auth construction, WebSocket, downloads, frontend URL builders, proxy/access logs, Sentry, analytics, and tests | Exit 0 or expected no-match 1; findings are in section 3. |
| Initial `apply_patch` | Failed before read/write: Windows restricted-token sandbox could not enforce split roots. |
| Assertion-guarded PowerShell rewrites for the final 26 pre-report files | Final exits 0; all expected old text/count assertions matched. |
| Early middleware rewrite | First assertion failed; no write. Corrected rewrite exit 0. |
| Early Trade app rewrite | First script parser error; no write. Corrected rewrite exit 0. |
| Early frontend rewrite | First assertion detected missed token destructuring; corrected rewrite exit 0. |
| Early import evidence rewrite | Expected count mismatch exposed three imports; reviewed correction exit 0. |
| `gofmt -w packages/auth/middleware.go packages/auth/middleware_test.go apps/trade-bff/server/app.go apps/trade-bff/server/ws_ticket.go apps/trade-bff/server/ws_ticket_test.go apps/user-bff/server/app.go apps/admin-bff/server/app.go` | Exit 0. |
| `gofmt -d` on those seven Go files | Exit 0, no diff. |
| First `go test ./packages/auth ./apps/trade-bff/server` | Exit 1; missing `strings` import found and corrected. |
| Subsequent same command | Exit 0; both packages passed. |
| `node --test scripts/sec-002-query-auth-check.test.mjs` | Exit 1; test-runner child process `spawn EPERM`. |
| `node scripts/sec-002-query-auth-check.test.mjs` | Exit 0; 4/4 passed in-process. |
| First combined race run | Exit 1; compiler reported insufficient disk. |
| Recursive cache-removal attempt | Not executed; shell tool required unavailable destructive approval. |
| task-local `go clean -cache` | Exit 0. |
| `fsutil volume diskfree C:` | Exit 1; access denied. |
| Race reruns with task-local `GOCACHE`/`GOTMPDIR` | Exit 0; detailed below. |
| First report write command | Not executed; Windows `CreateProcessAsUserW` command-length error 206. |
| Four bounded report write/append commands | Exit 0. |

### 12.2 Backend validation

| Exact command | Exact result |
|---|---|
| `go test ./packages/auth ./packages/validation ./apps/user-bff/server ./apps/admin-bff/server ./apps/api-server ./apps/trade-bff/server -count=1` | Exit 0; all 6 packages passed: 2.327s, 1.064s, 0.550s, 0.539s, 0.540s, 2.095s. |
| `go test -v ./packages/auth -run 'Test(RequireAuthRejectsCredentialQueryParameters\|RequireAuthHeaderStillSucceedsWithoutCredentialQuery\|TelemetryMiddlewareRedactsAndRestoresSecurityCredentials\|UserAdmin\|Isolation\|SessionRefresh)' -count=1` | Exit 0; focused auth/query/telemetry/isolation tests passed. |
| `go test -v ./apps/trade-bff/server -run 'Test(WSTicket\|AuthenticateWebSocket\|HandleWSTicket)' -count=1` | Exit 0; focused ticket/handshake tests passed. |
| `go vet ./packages/auth ./apps/trade-bff/server ./apps/user-bff/server ./apps/admin-bff/server` | Exit 0. |
| `go build -buildvcs=false ./apps/api-server ./apps/user-bff/server ./apps/admin-bff/server ./apps/trade-bff/server` | Exit 0. |
| `go test -race ./packages/auth -run 'Test(RequireAuthRejectsCredentialQueryParameters\|TelemetryMiddlewareRedactsAndRestoresSecurityCredentials)' -count=1` | Exit 0; passed in 2.465s. |
| `go test -race ./apps/trade-bff/server -run 'Test(WSTicket\|AuthenticateWebSocket)' -count=1` | Exit 0; passed in 1.309s. |

Successful Go commands emitted a nonfatal host warning that the Go telemetry
upload-token file under AppData could not be created (`Access is denied`). Exit
codes remained 0 and no credential value was printed.

### 12.3 Frontend/Node/gateway validation

| Exact command | Exact result |
|---|---|
| `pnpm --version` | Exit 0; 8.15.0. |
| `Test-Path apps/user-frontend/node_modules` | Exit 0; `False`. |
| Targeted user-frontend `pnpm test` | Exit 1 before tests; Node `spawnSync ... EPERM`. |
| User-frontend `pnpm lint` | Exit 1 before lint; same EPERM. |
| User-frontend `pnpm typecheck` | Exit 1 before typecheck; same EPERM. |
| User-frontend `pnpm build` | Exit 1 before build; same EPERM. |
| `node apps/user-frontend/src/modules/trade/composables/webSocketUrl.test.mjs` | Exit 0; 3/3 passed. |
| `node scripts/sec-002-query-auth-check.mjs` | Exit 0; PASS, 807 files, two justified exclusions. |
| `node scripts/sec-002-query-auth-check.test.mjs` | Exit 0; 4/4 passed. |
| `node --check` on the two SEC-002 scripts and two production-builder files | Exit 0 for all four. |
| `Get-Command nginx -ErrorAction SilentlyContinue` | No result; `nginx -t` unexecuted. |
| `docker version --format '{{json .Server.Version}}'` | Exit 1; daemon pipe unavailable/denied. |
| `docker compose -f infra/docker/docker-compose.yml config --quiet` | Exit 0; static Compose parse passed. |
| SEC-002 structural gateway assertions | Exit 0; sanitized URI, WS query suppression, omitted referrers, no raw K8s `$request`. |
### 12.4 Prerequisite/repository regressions

| Exact command | Exact result |
|---|---|
| Initial `node scripts/production-baseline.test.mjs` | Exit 1; legitimate SEC-002 Go count 384 vs 382; evidence updated. |
| Initial `node scripts/target-architecture.test.mjs` | Exit 1; reviewed imports 464 vs 462; evidence updated. |
| Initial `node scripts/sec001-auth-isolation.test.mjs` | Exit 1; obsolete “SEC-002 deferred” assertion; replaced with completed invariant. |
| Final `node scripts/production-baseline.test.mjs` | Exit 0; 5/5. |
| Final `node scripts/target-architecture.test.mjs` | Exit 0; 4/4. |
| `node scripts/domain-glossary.test.mjs` | Exit 0; 8/8. |
| `node scripts/database-migration-reset.test.mjs` | Exit 0; 10/10. |
| `node scripts/codex-execution-protocol.test.mjs` | Exit 0; 11/11. |
| Final `node scripts/sec001-auth-isolation.test.mjs` | Exit 0; 10/10. |
| `node scripts/sec-002-query-auth-check.test.mjs` | Exit 0; 4/4. |
| Combined focused Node regression | 52/52 passed. |
| `node scripts/production-baseline.mjs verify` | Exit 0; reproducible inventory/deltas, 35 P0/P1 paths, 146 links, toolchains passed; retained CI patch-version warnings. |
| First final regression wrapper | Exit 1 before commands; PowerShell variable delimiter parser error. |
| Second wrapper | Baseline/architecture passed; then exit 1 from incorrect nonexistent filename `canonical-domain-glossary.test.mjs`; discovered `domain-glossary.test.mjs` then passed. |
| Default-cache `go test ./packages/db/migrations_test.go -v` | Exit 1 at setup from host default Go-cache access error. |
| Same command with task-local `GOCACHE`/`GOTMPDIR` | Exit 0; 5/5, 98 pairs plus documented orphan. |
| `go vet ./packages/db/migrations_test.go` with local cache/temp | Exit 0. |

### 12.5 Markdown, scope, and credential checks

| Command | Exact result |
|---|---|
| `Get-Command markdownlint` and `markdownlint-cli2` | No results; unavailable, not passed. |
| `validateMarkdownFiles` on four changed docs before report | Exit 0; 162 local links resolved. |
| Post-change `rg` query-auth/URL/log inventory | Only security tests/checker, bounded ticket, Finnhub exclusion, and reset fixture remained; no reusable session URL builder/session query read. |
| Focused PowerShell credential scan of 26 pre-report files | Exit 0; all present, no private key, compact JWT, provider key, or credential-bearing URL. |
| `Test-Path .git` | Exit 0; False. |
| SEC-003 artifact check | Exit 0; zero new matching paths. |
| Final report self-validation | See section 16. |

## 13. Validation summary

| Area | Result |
|---|---|
| Session query-token removal and SEC-001 isolation | PASS |
| WebSocket lifecycle/handshake/race | PASS |
| Downloads | PASS structural/server regression; no affected URL-token flow |
| Exact frontend URL builder | PASS 3/3 |
| Frontend package test/lint/typecheck/build | UNEXECUTED: host EPERM |
| Telemetry/gateway protection | PASS |
| Native Nginx syntax | UNEXECUTED: binary unavailable |
| Go test/vet/build/race | PASS |
| Prohibited-pattern scan | PASS, 807 files |
| FND/SEC-001 regressions | PASS, 52/52 Node plus 5/5 migration and vet |
| Markdownlint | UNAVAILABLE |
| Focused links/structure and credential scan | PASS |

## 14. Acceptance criteria

- [x] Normal session middleware never reads JWTs from query parameters.
- [x] User/Admin URL JWTs and all discovered/proactive aliases fail closed.
- [x] Header access auth and SEC-001 refresh/logout/session/CSRF isolation work.
- [x] The affected WebSocket uses a bounded, short-lived, purpose/resource/
  context/session-bound, non-recoverably stored, single-use ticket.
- [x] Protected downloads use authenticated Blob fetch; no JWT URL exists.
- [x] Frontend has no session-token URL input/fallback; reconnect gets a ticket.
- [x] Access logs/telemetry do not receive session credentials or ticket values.
- [x] Query rejection, handshake, lifecycle, race, and isolation tests pass.
- [x] Touched backend services build, test, vet, and race-test.
- [x] Affected frontend behavior has executed direct-module and structural proof.
- [x] No credentials, dependency, compatibility flag, or later task was added.
- [x] Paid-production remains `NO-GO`.

## 15. Known untested behavior and risks

- Full frontend Vitest/lint/typecheck/build could not start due pnpm/Node EPERM
  and absent local `node_modules`; direct production-module and full source
  structural evidence ran.
- No live browser-through-Nginx upgrade, live Redis, live Sentry destination, or
  live object-store download ran. Go lifecycle/handler/middleware tests and
  structural proxy/download checks cover the implemented boundaries.
- `nginx -t` and Docker runtime validation were unavailable; Compose parse and
  structural checks passed.
- Markdownlint was unavailable; focused Markdown checks ran.
- SEC-003 through SEC-006 retain their roadmap-owned security work.

These limitations do not leave reusable User/Admin session JWT query support
active and do not authorize deployment.

## 16. Final report self-validation

The final PowerShell/Node validation command exited 0:

- all 27 changed files exist;
- five changed Markdown documents have 162 resolving local links;
- the SEC-002 structural checker still passes across 807 files with only the
  two documented non-session exclusions;
- the credential-candidate scan passes across all 27 files;
- `.git` remains absent; and
- no SEC-003 report artifact exists.

The subsequent report-only replacement command exited 0. A final credential and
Markdown-link recheck after that replacement also exited 0.

## 17. Rollback

Rolling this change back would restore a known vulnerability. Recovery should
roll forward by deploying the ticket backend with/before the frontend or pausing
the affected WebSocket journey—not by restoring query JWTs. Ticket backend,
frontend caller, cookie/storage policy, middleware, tests, and logging rules form
one rollback unit. SEC-001 boundaries must remain.

## 18. Final confirmations

**SEC-002 PASS**

- `SEC-003` and later tasks were not started.
- Phase 0 remains `PASS`; SEC-001 remains complete.
- No Git metadata or source-control remote operation occurred.
- No real secret or complete credential was persisted in files/report output.
- Paid-production status remains **`NO-GO`**.