# SEC-003 Local Execution Report

## 1. Task ID and title

- **Task:** `SEC-003 — Make OTP and reset delivery fail closed in production`
- **Execution date:** 2026-07-28; failed-gate remediation completed 2026-07-29.
- **Current result:** `SEC-003 PASS`
- **Reason:** the original `SEC-003 FAIL` is preserved in section 47. The dated remediation in section 53 and the linked remediation report add passing real PostgreSQL, real Redis, combined User BFF, frontend-toolchain, Bash, regression, secret-scan, and cleanup evidence.

## 2. Local execution mode

Work was performed directly in the extracted local project. Git was not initialized or used. No remote source-control operation, production deployment, production credential, real recipient, or real provider endpoint was used. Local evidence does not imply branch, commit, pull-request, CI, or merge status.

## 3. Phase 0, SEC-001, and SEC-002 dependency verification

| Dependency | Direct evidence | Result |
|---|---|---|
| Phase 0 | `phase-0-exit-report.md` says current decision `PASS`; FND regressions also passed here. | PASS |
| FND-002 / ADR 0001 | ADR exists and remains Accepted; Platform retains User identity ownership. | PASS |
| SEC-001 | Report says `PASS`; explicit User/Admin construction, namespaces, and validators remain active. | PASS |
| SEC-002 | Report says `PASS`; prohibited-query validator and Go regressions pass. | PASS |
| SEC-003 prior state | No report existed; active paths still contained the weaknesses below. | INCOMPLETE before this execution |
| SEC-004 | No SEC-004 task artifact was created or modified. Existing legacy TOTP code was not treated as SEC-004 implementation. | NOT STARTED |

## 4. Current pre-change OTP and reset inventory

| Flow | Endpoint(s) | Source | Pre-change state | Final state/evidence |
|---|---|---|---|---|
| Registration email | `POST /api/user/auth/register` | Public; request email/country | Country not initially persisted; 2-minute TTL; async acknowledgement. | Country required, normalized, persisted before synchronous routed delivery. DB integration skipped. |
| Email ownership send/resend | `/send-verification`, `/resend-verification` | User; stored email/country | 2-minute TTL, 90-second cooldown, unkeyed digest. | HMAC, 10 minutes, 60 seconds, five attempts, one-time consume. |
| Email ownership consume | `/verify-code`, legacy-shaped `/verify-email` | User; stored destination | Duplicate path could diverge. | Both delegate to one HMAC/row-lock lifecycle and mark only email ownership. |
| Phone ownership | `/send-verification`, `/verify-code` | User; stored phone | Provider/config semantics were not uniformly fail closed. | KaveNegar-only; five attempts; marks only `phone_verified`. |
| Public phone OTP auth | `/send-otp`, `/verify-otp`, `/register-phone` | Public; normalized Iranian phone | Redis default 2 minutes; mock selectable. | Namespaced Redis HMAC/Lua lifecycle, canonical policy. Real Redis test skipped. |
| Password-reset request | `/forgot-password/request` | Public; verified phone else verified email/country | 15-minute code, 120-second cooldown, unkeyed digest, logging fallback, async send. | Generic response, synchronous selected-provider acceptance, expired reservation until activation. Runtime integration skipped. |
| Password-reset verify/reset | `/forgot-password/verify`, `/forgot-password/reset` | Opaque one-time handles | Conflicting constants/attempts; incomplete session evidence. | Canonical HMAC lifecycle; one-use handle; all User sessions invalidated; Admin namespace untouched. |
| Legacy email token/link | Historical schema/cleanup | No new issuer | Historical table remained. | Active paths no longer write/trust it; cleanup remains; later migration owns removal. |
| Provider health/cleanup | Internal | None | Availability inconsistently described. | Sanitized health; expired-row cleanup; failure never returns delivered success. |

No code, reset handle, password-set handle, provider credential, authorization header, or provider body is intentionally logged by final security-code paths.

## 5. Verification of each known baseline hypothesis

| Hypothesis | Verification | Resolution |
|---|---|---|
| Signup TTL 2 minutes | Confirmed. | 10 minutes. |
| Signup resend 90 seconds | Confirmed. | 60 seconds. |
| SMS default TTL 2 minutes | Confirmed. | 10 minutes. |
| Reset TTL 15 minutes | Confirmed. | 10 minutes. |
| Reset cooldown 120 seconds | Confirmed. | 60 seconds. |
| Missing SMS config selected mock | Confirmed. | Runtime mock selection removed; production only configured KaveNegar. |
| Fallbacks logged codes | Confirmed. | Code-bearing fallback logging removed. |
| Security email only Resend | Confirmed. | Mailerino for `IR`, Resend for supported non-`IR`. |
| No Mailerino adapter | Confirmed. | Bounded standard-library adapter added. |
| Async send acknowledged early | Confirmed. | Synchronous acceptance before activation/success. |
| Unkeyed SHA-256 | Confirmed. | Dedicated context-bound HMAC-SHA-256. |
| Frontend country not initially persisted | Confirmed. | Initial request/insert include normalized country. |
| Country absent for first send | Confirmed. | Required before creation/delivery. |
| Conflicting lifecycle policy | Confirmed. | Active flows share canonical constants/binding rules. |

## 6. Final security-code lifecycle

Canonical policy: cryptographically generated six numeric digits; 10-minute validity beginning after provider acceptance; 60-second cooldown including failed reservation; maximum five attempts; one active code per User/purpose/destination/channel; replacement invalidates old code; success, expiry, exhaustion, and replacement are terminal; timing-safe comparison; atomic one-time consumption.

Database flows create an expired reservation, deliver synchronously, then activate. Redis flows reserve with a nonce, deliver, then activate via Lua. Delivery failure expires/cancels state. Activation failure is unavailable, never success.

## 7. Final code-hashing design

Codes are HMAC-SHA-256 digests under `SECURITY_CODE_HASH_SECRET`. Length-prefixed input binds purpose, User ID, normalized destination, channel, optional reset-request context, and code. Comparison is constant-time. The secret is independent of User JWT/provider keys, supports `_FILE`, is never logged, and production rejects missing, placeholder, low-diversity, or reused material. No reversible encryption or unkeyed digest is used.

## 8. Canonical country source and normalization

The source is persisted User `country`. Registration requires ISO 3166-1 alpha-2, uppercases and validates it, then stores it in the initial insert. Routing never uses language, email domain, IP, locale, or guess. Missing/malformed/unsupported country fails closed.

## 9. Mailerino routing design

`IR` selects only Mailerino. Official documentation was checked 2026-07-28. The adapter uses bearer-authenticated `POST /v1/send`, 10-second default timeout, context cancellation, 64 KiB response limit, exact accepted-response validation, no automatic retry, and generic errors. Tests use fake HTTP servers only.

## 10. Resend routing design

Supported non-`IR` selects only Resend. Official send-email documentation was checked 2026-07-28. The adapter uses bearer-authenticated `POST /emails`, bounded response parsing, 200/201 plus provider ID acceptance, cancellation, timeout, no retry, and sanitized errors. Demonstration sender is rejected in production. No fallback occurs.

## 11. SMS/KaveNegar design

Production SMS is `kavenegar` only. Template delivery has context timeout; direct-send fallback was removed. Missing key/template, Redis failure, initialization failure, circuit rejection, or provider rejection fails closed. `FakeProvider` is a non-logging process-local test double never selected by runtime; production rejects fake/mock/logging/no-op/unknown modes.

## 12. Provider startup and feature validation

One centralized path rejects missing/contradictory environment identity, Mailerino/Resend credentials/senders, default/example values, unsafe URLs, missing/weak/reused HMAC secret, ambiguous sender, SMS without KaveNegar key/template, and non-KaveNegar production SMS. Rejected values are not included in errors. Explicit local/test fixtures remain non-production only.

## 13. Delivery acknowledgement and failure compensation

Handlers cannot report delivered success before one selected provider accepts. Activation follows acceptance. Failure invalidates the reservation and returns a stable generic failure, or the unchanged anti-enumeration response for reset. Email providers do not cross-fallback; SMS does not fall back to email/logging; automatic retry is absent.
## 14. Signup email-verification behavior

Registration persists country in the User-creation transaction, reserves verification, selects Mailerino for `IR` or Resend otherwise, waits for acceptance, activates, then returns the normal result. Failure returns generic unavailable and revokes the newly created User session.

## 15. Signup phone-verification behavior

Phone ownership uses configured KaveNegar and the canonical policy. Consumption marks `phone_verified` only and cannot mark `email_verified`. Public phone authentication uses the isolated Redis HMAC lifecycle.

## 16. Password-reset behavior

Request selects verified phone first, otherwise verified email and stored country. State is reserved before delivery and activated after acceptance. Code and password-set handles are context-bound and single-use. Resend replaces active state. Success changes password, invalidates unused codes, and invalidates User sessions. Provider/config/state failure is closed.

## 17. Anti-enumeration behavior

The unauthenticated reset request preserves a stable generic response for absent account, unavailable destination, provider rejection, and state failure. Provider/country details and account existence are not disclosed. Diagnostics contain no credential.

## 18. Session invalidation behavior

`Session.DeleteAllForUser` must succeed before the password transaction commits. Failure aborts the change. Only the SEC-001 User session prefix is affected; Admin state is untouched. `password_changed_at` remains defense in depth.

## 19. Logging and telemetry protections

Active paths do not log codes, message bodies, reset handles, provider bodies/keys, or authorization headers. Errors are fixed sentinels. The structural validator detects credential-like logging, unkeyed digest, old constants, production mock construction, provider fallback, and SMS-to-email ownership mistakes.

## 20. Configuration variables added, changed, removed, or deprecated

| Variable | Status |
|---|---|
| `SECURITY_CODE_HASH_SECRET[_FILE]` | Added; required independent HMAC key. |
| `MAILERINO_API_KEY[_FILE]` | Added; required for Iranian security email. |
| `MAILERINO_FROM_EMAIL` | Added; required sender. |
| `MAILERINO_BASE_URL` | Added; HTTPS default documented. |
| `RESEND_API_KEY[_FILE]` | Retained and used for security email. |
| `RESEND_FROM_EMAIL` | Added as canonical Resend sender. |
| `RESEND_BASE_URL` | Added; HTTPS default documented. |
| `EMAIL_FROM` | Compatibility input for operational mail; conflict with `RESEND_FROM_EMAIL` rejected in production. |
| `SMS_ENABLED` | Retained; absence/failure cannot select mock. |
| `SMS_PROVIDER` | Added/clarified; production must be `kavenegar`. |
| `KAVENEGAR_API_KEY[_FILE]` | Explicit production validation/mount. |
| `SMS_TEMPLATE` | Required when SMS security delivery is enabled. |

No setting restores a production code-logging/mock fallback.

## 21. Secret files or mounts added or changed

Secret docs, initialization, migration helper, and User BFF Compose now cover `security_code_hash_secret`, `mailerino_api_key`, `resend_api_key`, and `kavenegar_api_key` file/mount names. Only names and safe descriptions are stored. No value was created or recorded.

## 22. Compatibility or mock-provider status

No production compatibility mode. No console/logging/no-op provider. `FakeProvider` is explicit, non-logging, process-local test infrastructure only. Historical migrations were not edited.

## 23. Files changed

Modified:

- `.env.example`
- `apps/user-bff/server/app.go`
- `apps/user-bff/server/auth_handlers.go`
- `apps/user-bff/server/forgot_password_handlers.go`
- `apps/user-bff/server/phone_auth.go`
- `apps/user-bff/server/verification_handlers.go`
- `apps/user-frontend/src/modules/user/views/LoginPage.vue`
- `apps/user-frontend/src/stores/auth.ts`
- `docs/architecture/target-architecture-import-review.md`
- `infra/docker/docker-compose.yml`
- `infra/docker/secrets/README.md`
- `packages/sms/kavenegar.go`
- `packages/sms/mock.go`
- `packages/sms/otp.go`
- `packages/sms/otp_test.go`
- `scripts/production-baseline.mjs`
- `scripts/production-baseline.test.mjs`
- `scripts/secrets/init-secrets.sh`
- `scripts/secrets/migrate-from-env.sh`
- `scripts/target-architecture.test.mjs`

Added:

- `apps/user-bff/server/password_reset_security_test.go`
- `apps/user-bff/server/security_code_clock_test.go`
- `apps/user-bff/server/security_code_integration_test.go`
- `apps/user-bff/server/security_codes.go`
- `apps/user-bff/server/security_codes_extended_test.go`
- `apps/user-bff/server/security_codes_test.go`
- `docs/security/otp-and-reset-delivery.md`
- `docs/codex/reports/SEC-003-local-execution-report.md`
- `packages/notification/security_email.go`
- `packages/notification/security_email_failure_test.go`
- `packages/notification/security_email_test.go`
- `packages/sms/kavenegar_test.go`
- `packages/sms/otp_redis_integration_test.go`
- `packages/sms/otp_security_properties_test.go`
- `scripts/sec-003-otp-delivery-check.mjs`
- `scripts/sec-003-otp-delivery-check.test.mjs`

Total: 36 files including this report.

## 24. Every scope expansion and its evidence

- `apps/user-frontend/**`: initial registration needed country before first routing.
- `scripts/secrets/**`, `infra/docker/**`, `.env.example`: independent HMAC/provider secrets needed the established `_FILE`/Compose mechanism and safe documentation.
- `scripts/production-baseline*`, `scripts/target-architecture.test.mjs`, `docs/architecture/target-architecture-import-review.md`: SEC-003 added Go/test imports, so prerequisite inventory/import evidence had to remain truthful. No ADR boundary changed.
- `docs/security/**`: focused design/deployment documentation.

No unrelated application or migration changed.

## 25. Dependencies added

None. Mailerino and Resend adapters use `net/http` and existing packages, avoiding license, maintenance, supply-chain, and rollback impact.

## 26. Tests added or updated

- Mailerino/Resend contract, failure, cancellation, and response-bound tests.
- KaveNegar template-only, timeout, cancellation, and sanitized-error tests.
- HMAC binding, routing, environment, production validation, clock, policy, and reset-namespace tests.
- In-memory OTP lifecycle/property/concurrency tests.
- Environment-gated isolated Redis and PostgreSQL+Redis tests.
- Structural repository validator and self-tests.
- Updated inventory/import regression fixtures.

Tests use fake local servers/credentials. None was deleted, weakened, quarantined, or changed to call a real provider.
## 27. Every command executed and exact result

The ledger records executable checks/mutations. Authority/inventory reads used `Get-Content`, `Get-ChildItem`, `Test-Path`, and `rg`; all exited 0 except Windows `rg` wildcard forms, which exited 1 because PowerShell did not expand them. They were rerun with `-g '*_test.go'` and succeeded. Secret values were never requested or printed.

| Command | Exact result |
|---|---|
| `Get-Content -Raw <attached pasted-text.txt>` | Exit 0; complete 1,212-line request read. |
| `Get-Content -Raw` for all required authorities/task blocks | Exit 0; all authorities read. |
| `Test-Path .git` | Exit 0; `False`. |
| Dependency/artifact `Test-Path` and `rg` | Exit 0; prerequisite PASS evidence present; SEC-003 report absent before work. |
| Repository-wide `rg` OTP/reset/provider/log/country/constants/tests inventory | Exit 0; confirmed all fourteen hypotheses. |
| Scoped patch/edit operations | Implementation changes succeeded. Final built-in `apply_patch` comment edit failed before modification because Windows split-root sandbox preparation failed; local `apply_patch.bat` also returned `Access is denied`. Guarded `.NET WriteAllText` was used; a literal newline-escape artifact was detected/corrected immediately, then `gofmt` and tests verified the file. |
| Initial one-command report write | Process did not start: Windows error 206, command line too long. Report was then written in bounded chunks. |
| `gofmt -d` over 21 changed Go files | Exit 0; no output. |
| `gofmt -w apps/user-bff/server/app.go` then `gofmt -d` | Exit 0; corrected a line-ending-only diff; no remaining output. |
| `go test ./packages/sms ./packages/notification ./packages/secrets ./apps/user-bff/server -count=1` | Exit 0; four packages passed: `1.296s`, `15.750s`, `0.525s`, `0.219s`. |
| Focused User BFF tests after final comment edit | Exit 0; passed in `0.221s`. |
| Focused SMS race command | Exit 0; passed. |
| Touched-package `go vet` | Sandbox retry once exited 1 from `C:\tmp` cache access denial; outside-sandbox rerun exit 0, no output. |
| Initial touched-package `go build` | Exit 1 because VCS status was unavailable in extracted local mode. |
| `go build -buildvcs=false` for touched packages/apps | Exit 0, no output; final rerun with vet passed. |
| SEC-001/002 six-package Go regression | Sandbox attempt exit 1 only from cache denial; outside-sandbox rerun exit 0, six packages passed: `2.391s`, `1.018s`, `0.443s`, `0.283s`, `0.293s`, `2.053s`. |
| `node scripts/sec-003-otp-delivery-check.mjs` | Exit 0; PASS, 8 active files inspected. |
| `node --test scripts/sec-003-otp-delivery-check.test.mjs` | Sandbox attempt exit 1 `spawn EPERM`; outside-sandbox rerun exit 0, 4/4. |
| Full eight-file Node regression command | Exit 0; 56 tests, 56 pass, 0 fail, 0 skip. |
| First full Node regression before updating metadata | Exit 1; 54/56; expected inventory/import counts were stale. Updated focused evidence; rerun 56/56. |
| Inventory module command | Exit 0; Go 396, Vue 211, TS/TSX 180, SQL 202, up 98, down 99, Go test 115, frontend test 11. |
| Isolated Redis integration test | Exit 0 but **SKIP**: `SEC003_REDIS_ADDR is required for the isolated Redis runtime test`. Not passing runtime evidence. |
| Isolated PostgreSQL+Redis User BFF test | Exit 0 but **SKIP**: both test-only connection variables required. Not passing runtime evidence. |
| Docker version/info/default-context info | CLI exists; daemon queries timed out (34–124 seconds), no server result. |
| Docker Desktop status/restart | Timed out (19/124 seconds); no daemon established. |
| Process/WSL checks and controlled restart attempts | No container started. Final `wsl.exe --list --verbose` exit 0: `docker-desktop` Stopped, WSL 2. |
| `Get-Command psql/postgres/redis-server` | Exit 0; all unavailable. |
| Local port check 5432/6379 | Exit 0; no listeners. |
| Compose render command | Exit 0, no output. |
| `bash -n` for edited secret scripts | Exit 1; WSL relay `/bin/bash` unavailable. Syntax check did not run. |
| `go test ./packages/secrets -count=1 -v` | Exit 0; all tests passed with fake fixtures. |
| `pnpm install --frozen-lockfile --offline` | Exit 1; cached package tarball unavailable. |
| `pnpm install --frozen-lockfile` | Exit 1 after 140 seconds; registry mirror timed out; no lockfile change. |
| Combined sandbox frontend typecheck/lint/test/build | Exit 1 for all due `spawnSync node.EXE EPERM`. |
| Each frontend command outside sandbox | Exit 1: `node_modules` absent; `vue-tsc`, `eslint`, `vitest`, `vue-tsc` unavailable respectively. |
| Markdown link validation for 3 changed docs | Exit 0; no broken local links. |
| `Get-Command markdownlint` / `markdownlint-cli2` | Unavailable; focused structure/link tests used instead. |
| High-confidence credential scan over 35 implementation files | Exit 0; 0 findings; values not printed. |
| Focused coverage command | Tests exit 0: SMS 55.4%, notification 3.5%, User BFF 1.3%. Coverage files were already absent at cleanup, causing three non-terminating not-found warnings; no artifact remains. |
| `Test-Path node_modules` | Exit 0; `False` after partial-install cleanup. |
| Final location/Git/node_modules check | Exit 0; correct path; `.git=False`, `node_modules=False`. |

| Report section/decision/no-complete-code validator | Exit 0; sections 1–52 consecutive, required decisions present, no complete six-digit numeric value. |
| Final Markdown link check for 4 changed docs | Exit 0; no broken local links. |
| Final high-confidence credential scan over 36 changed files | Exit 0; 0 findings; values not printed. |
| Final structural/scope check | Exit 0; 8 active files pass; `.git=False`, `node_modules=False`, SEC-004 report absent, no migration modified today. |
Implementation inspection/report chunk commands returned exit 0 unless listed otherwise. No failure/skip is claimed as a pass.
## 28. Unit-test results

PASS. Generation, HMAC storage/bindings, immediate/pre-expiry/expiry, cooldown, five attempts, exhaustion, replay, resend replacement, provider compensation, storage errors, wrong context, and production validation are covered. Final four-package run passed.

## 29. Integration-test results

**UNEXECUTED / TASK-BLOCKING.** Real isolated Redis and PostgreSQL+Redis tests skipped because explicit test-only connections were absent. Docker had no usable daemon and no local runtime commands/ports existed. Unit/in-memory/fake-server tests are not substitutes.

## 30. Provider-adapter test results

PASS. Mailerino, Resend, and KaveNegar fake-server tests cover request form, authorization presence without logging, acceptance, non-success/malformed/oversized response, timeout, cancellation, bounded reads, sanitized errors, and no fallback/automatic retry. No real endpoint was contacted.

## 31. Configuration-validation results

PASS. Table tests reject missing/weak/placeholder/reused HMAC material; missing provider keys/senders; demonstration sender; KaveNegar key/template absence; ambiguous environment/sender; unsafe URLs; and fake/mock/logging/no-op/unknown production SMS. A valid isolated fake fixture is accepted without exposing values.

## 32. Race and concurrency results

PASS for executable evidence. Focused Go race passed; in-memory concurrency proves one consume and one active resend. Real Redis Lua concurrency skipped and contributes to FAIL.

## 33. Go formatting, vet, and build results

PASS after sandbox retries: 21 changed Go files formatted; touched packages vet; libraries/User BFF/merged API build with `-buildvcs=false`, appropriate because Git is intentionally absent. No Go dependency changed.

## 34. Frontend lint, typecheck, test, and build results

**UNAVAILABLE.** All four were attempted. Outside sandbox they failed because `node_modules` and binaries are absent. Online install timed out; offline install lacked cache. Focused Node structural validation proves country is required/sent initially and reset credentials are not put in URL/local storage/log calls, but this is not a frontend toolchain pass.

## 35. Migration and fresh-database results

No migration added: existing `varchar(64)` digest columns fit HMAC. Migration count/order tests pass. Mandatory controlled PostgreSQL state integration was still applicable and skipped, contributing to FAIL.

## 36. Compose/configuration rendering results

Compose rendering PASS; secret mount/reference structural checks PASS. Bash syntax validation unavailable; secret loader Go tests pass. No service was started.

## 37. Structural prohibited-pattern scan result

PASS: 8 active source files inspected; 4/4 self-tests pass. No active code logging, unkeyed digest, old TTL/cooldown, production mock, country-less email route, cross-provider fallback, raw body logging, or SMS-to-email ownership pattern.

## 38. Secret and captured-log scan result

PASS for available evidence. Zero private-key, JWT, common live-key, long Resend-key, or credential-bearing PostgreSQL URL signatures across 35 implementation files. Canary tests find no credential in errors/logs. Terminal output contains no generated code/secret. This report contains no complete security credential.

## 39. SEC-001 and SEC-002 regression results

PASS. Six-package Go suite and SEC-001/002 Node structural regressions pass. Separate User/Admin contexts/namespaces remain active; query session JWTs remain prohibited.

## 40. Relevant FND regression results

PASS. Baseline, architecture, glossary, migration reset, and execution protocol are in the final 56/56 suite. Inventory: Go 396, Vue 211, TS/TSX 180, SQL 202, up 98, down 99, Go tests 115, frontend tests 11. Reviewed imports changed 464→471 because seven SEC-003 Go imports were added; ADR boundaries/edge count did not change.

## 41. Coverage results for affected critical packages

- `packages/sms`: 55.4%
- `packages/notification`: 3.5% (large unrelated package; security adapter branches have focused tests)
- `apps/user-bff/server`: 1.3% (large app package; selected helpers/handlers only)

Coverage is execution evidence, not correctness proof. Skipped runtime integration remains decisive.
## 42. Acceptance-criteria checklist

| Criterion | Status | Evidence |
|---|---|---|
| Every active path inventoried | PASS | Sections 4–5/design doc. |
| No credential/code logging in tested paths | PASS | Canary/structural/credential scans. |
| No production mock/log/no-op fallback | PASS | Construction/validation/tests. |
| Missing provider config fails closed | PASS | Production tests. |
| Missing/weak HMAC rejected | PASS | Config tests. |
| `IR` only Mailerino | PASS | Router tests. |
| Supported non-`IR` only Resend | PASS | Router tests. |
| Stored country drives route | PASS | Handler/router/static evidence. |
| Country before first send; invalid/missing fails | Runtime unverified | Unit/static pass; DB integration skipped. |
| All TTLs 10 minutes | PASS | Constants/tests. |
| All cooldowns 60 seconds | PASS | Constants/tests. |
| Five attempts | PASS | Unit/handler tests. |
| HMAC/context binding | PASS | Property tests. |
| One consume, replay failure, resend invalidation | Runtime unverified | In-memory pass; DB/Redis skipped. |
| Concurrent exactly-once | Runtime unverified | Unit/race pass; real Redis skipped. |
| Failed provider leaves no usable code | Runtime unverified | Unit store pass; real state skipped. |
| No success before acceptance | PASS | Handler/adapter/static evidence. |
| Reset anti-enumeration | PASS | Handler/static response evidence. |
| Reset invalidates User not Admin sessions | Runtime unverified | Focused evidence passes; integration skipped. |
| SMS cannot prove email ownership | PASS | Handler/structural tests. |
| SEC-001/002 green | PASS | Go/Node regressions. |
| Provider tests without external calls | PASS | Fake servers. |
| Backend format/vet/build/race | PASS | Sections 27, 32–33. |
| Affected frontend has full executed evidence | FAIL | Structural pass; toolchain unavailable. |
| Compose/configuration | PARTIAL | Compose/Go pass; Bash unavailable. |
| Real PostgreSQL/Redis behavior | FAIL | Both tests skipped. |
| Structural/secret scans | PASS | Sections 37–38. |
| No later task | PASS | Scope review. |
| Paid-production `NO-GO` | PASS | Preserved. |

## 43. Known untested behavior

- Real PostgreSQL locking, activation, exhaustion, replay, compensation.
- Real Redis Lua reservation/activation/concurrent consumption/resend.
- Combined User BFF startup against isolated PostgreSQL/Redis with fake providers.
- Frontend compile, typecheck, lint, test, build.
- Bash syntax of edited secret scripts.
- Live provider delivery and production startup with real credentials (intentionally prohibited).

## 44. Remaining security risks

The decisive risk is missing real-state database/Redis evidence. Frontend toolchain and Bash syntax evidence are also missing. Generalized redaction, Super Admin TOTP/sensitive-action reauthentication, and edge security remain SEC-004–SEC-006. Historical schema cleanup remains later migration work. Paid production remains blocked.

## 45. Deployment-order notes

After remediation/approval: provision independent HMAC and provider secrets; configure approved senders/template/HTTPS endpoints; run production config/Compose and isolated database/Redis tests; deploy backend lifecycle/providers; deploy frontend country contract; monitor generic delivery counters without payloads. No production step ran here.

## 46. Rollback notes

Rollback must not restore unkeyed/plain storage, old TTL/cooldown, code logging, demonstration senders, async success, mock/no-op selection, provider fallback, shared User/Admin trust, or URL credentials. Safe rollback may disable endpoints fail closed. HMAC-issued codes must expire/be invalidated, never accepted by a legacy unkeyed path.

## 47. Original explicit decision (preserved)

`SEC-003 FAIL`

Implementation and available focused checks pass, but mandatory PostgreSQL/Redis integration did not execute and the frontend toolchain checks were unavailable. Task decision rules require FAIL when required behavior lacks reliable executed evidence.

## 48. SEC-004 status

`SEC-004` was not started. No Phase 1 exit gate ran.

## 49. Git metadata

No Git metadata was created. `.git` remains absent.

## 50. Remote source-control operation

No remote connection, branch, commit, push, pull request, merge, or other source-control operation occurred.

## 51. Real provider delivery

No real Mailerino, Resend, KaveNegar, production infrastructure, or real recipient was contacted. Tests used local fake servers only.

## 52. Paid-production status

Paid-production status remains `NO-GO`.
## 53. Failed-gate remediation and current decision

On 2026-07-29, the five evidence gaps recorded above were remediated. The complete sanitized evidence is in the [SEC-003 failed-gate remediation report](SEC-003-failed-gate-remediation.md).

Executed results:

- real isolated PostgreSQL: PASS, no skip;
- real isolated Redis: PASS, no skip;
- combined User BFF PostgreSQL/Redis behavior: PASS, no skip;
- runtime concurrency and controlled failure compensation: PASS;
- frozen `pnpm@8.15.0` install: PASS with unchanged lockfile;
- frontend lint, typecheck, eight Vitest tests, and production build: PASS;
- real Git Bash syntax parsing: PASS;
- targeted Go, vet, build, SEC-001/SEC-002, and 56-test Phase 0/SEC Node regressions: PASS;
- secret/captured-output checks and required cleanup: PASS.

The original failure in section 47 remains historical evidence. The current task decision is:

`SEC-003 PASS`

`SEC-004` was not started, the Phase 1 exit gate was not run, no Git or remote operation occurred, no real provider was contacted, and paid-production status remains `NO-GO`.
