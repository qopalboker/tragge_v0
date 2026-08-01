# SEC-003 failed-gate remediation

## 1. Original SEC-003 failure

The original local report correctly recorded `SEC-003 FAIL`. The implementation checks available at that time passed, but real PostgreSQL, real Redis, the combined User BFF runtime, the pinned frontend toolchain, and a real Bash parser had not executed. This remediation addresses only those five evidence gaps and defects directly exposed by them.

## 2. Execution date and local mode

- Date: 2026-07-29 (Asia/Tehran).
- Mode: local extracted-project remediation.
- Working directory: `work/tragge-main` beneath the selected local workspace.
- Git metadata was absent and was not initialized.
- No remote source-control, production, staging, or real-user target was used.

## 3. Authoritative sources

The remediation followed the fixed policy, roadmap SEC-003 block, canonical Codex execution protocol, Accepted ADR-0001, canonical glossary, Phase 0 report, SEC-001 through SEC-003 reports, security delivery document, Phase 1 controller, failed-gate prompt, current SEC-003 files, integration tests, workspace manifests, and changed secret scripts named in the invocation. Explicit remediation authority took precedence over the planning-only default in the reusable failed-gate prompt.

## 4. Pre-remediation repository state

Phase 0, SEC-001, and SEC-002 each said `PASS`; SEC-003 said `FAIL`; no SEC-004 report or implementation evidence existed; `.git` was absent; root and frontend `node_modules` were absent. The existing focused Go and structural tests passed. The environment-gated tests were `TestSEC003RedisOTPLifecycle` and `TestSEC003IntegrationPostgresRedisLifecycle`, using `SEC003_REDIS_ADDR` and `SEC003_POSTGRES_DSN`. Provider calls in those tests were in-process fakes.

## 5. Exact remaining blockers

1. Real isolated PostgreSQL evidence.
2. Real isolated Redis evidence.
3. Combined User BFF behavior against both runtimes.
4. Frozen frontend install plus lint, typecheck, Vitest, and production build.
5. `bash -n` for both changed secret scripts.

All five are now executed and passing.

## 6. PostgreSQL preflight

- Docker CLI: available; daemon `29.4.3` on Docker Desktop.
- Method: dedicated local Docker container.
- Image: local `postgres:16-alpine`, content ID `sha256:890480b08124ce7f79960a9bb16fe39729aa302bd384bfd7c408fee6c8f7adb7`.
- Container: `tragge-sec003-postgres-remediation`.
- Host/port: loopback `127.0.0.1:55433` only.
- Database/user: `tragge_test_sec003` / `tragge_sec003_admin`.
- Volume: `tragge_sec003_postgres_data_20260728`.
- Credential: a newly generated 64-character CSPRNG test-only value stored outside the repository and never printed.
- Native `psql`/`postgres`: unavailable; container-local `psql` used.
- Preflight confirmed the database name contains the approved `test` marker, ports were unused, no inherited DSN was reused, and no external database was targeted.

## 7. Redis preflight

- Method: dedicated local Docker container.
- Image: local `redis:7-alpine`, content ID `sha256:6ab0b6e7381779332f97b8ca76193e45b0756f38d4c0dcda72dbb3c32061ab99`.
- Container: `tragge-sec003-redis-remediation`.
- Host/port: loopback `127.0.0.1:56380` only.
- Database index: default isolated index `0`.
- Authentication: no password on the disposable loopback-only container.
- Namespace: `security-code:sms:phone-auth:` plus SEC-001 User/Admin session namespaces.
- Volume: `tragge_sec003_redis_data_20260728`; persistence was not relied upon.
- Native Redis tools were unavailable; container-local `redis-cli` was used.
- Initial `DBSIZE` was zero; the runtime was never shared or external.

## 8. Frontend-toolchain preflight

- Node: `v22.19.0`.
- Corepack: `0.34.0`.
- Declared and executed pnpm: `pnpm@8.15.0`.
- Configured registry host: `registry.npmmirror.com`; no registry credential was printed.
- `node_modules` did not exist before remediation.
- Lockfile SHA-256 before install: `B548E14AD7D030502231EF18F316F7161EA7AB3358E0FBADCD995C7579171427`.
- The first frozen install timed out after 124 seconds with only partial package material. The identical command resumed and completed; no alternate package manager, dependency upgrade, or unlocked install was used.

## 9. Bash preflight

Windows Git Bash existed at `C:/Program Files/Git/bin/bash.exe`. The sandboxed parser process failed before parsing because MSYS could not create a signal pipe. The same non-executing parser command ran outside the sandbox and returned zero.

## 10. Sanitized isolated PostgreSQL identity

The server reported `16.14|tragge_test_sec003|tragge_sec003_admin`. The target was a new disposable database in the dedicated container. No password or credential-bearing URL appears in this report.

## 11. Sanitized isolated Redis identity

The dedicated server reported Redis `7.4.9` and was reachable only at loopback port `56380`. It contained no unrelated keys before testing.

## 12. Runtime versions and immutable image identities

PostgreSQL executed as server `16.14` from the recorded local image content ID. Redis executed as server `7.4.9` from its recorded local image content ID. Docker Compose was `v5.1.3`; Docker Engine was `29.4.3`.

## 13. Test environment-variable names

The runtime tests used only `SEC003_POSTGRES_DSN` and `SEC003_REDIS_ADDR`. Commands also set `ENVIRONMENT=test` and `APP_ENV=test`. The DSN was assembled in-process from the temporary credential, sanitized in evidence, removed from the environment immediately after each command, and never persisted in the repository.

## 14. Migration and initialization commands

The blank PostgreSQL container created `tragge_test_sec003`. A deterministic PowerShell sort of every `packages/db/migrations/*.up.sql` piped each file to container-local `psql -X -q -v ON_ERROR_STOP=1`. Exactly 98 up migrations applied. A temporary filename/SHA manifest contained 98 entries, 98 unique names, zero duplicates, and SHA-256 `0432A61E51168D1725F21BC966F1187E5288D20FA834A4477208BA1ECB07FE17`. The current legacy set has no canonical migration tracking table; the manifest was remediation evidence and was deleted at cleanup. No migration was edited. The focused integration test then created only its SEC-003 fixture rows/tables using columns required by current migrations; no production seed was needed.

## 15. PostgreSQL readiness evidence

Container readiness passed before migration. Server version, database, and user were queried directly. The deterministic migration loop returned `MIGRATIONS_OK applied=98`; the credential-corrected recreation returned the same result.

## 16. Redis readiness evidence

Container readiness and `PING` passed. The server version was confirmed and the initial database was empty. Tests used only the disposable instance and flushed only that positively identified database.

## 17. Real Redis integration results

`TestSEC003RedisOTPLifecycle` and `TestSEC003RedisOTPFailureAndBindingMatrix` ran without skips and passed. Real Redis proved reservation/nonce activation, failed-delivery cancellation, TTL, cooldown, five-attempt exhaustion, expiration, one-time consumption, replay rejection, destination/purpose/channel binding, resend replacement, stale activation rejection, exactly-one concurrent issue/consume behavior, namespace confinement, digest-only storage, and fail-closed command errors. The race-detector rerun passed.

## 18. Real PostgreSQL integration results

The expanded `TestSEC003IntegrationPostgresRedisLifecycle` ran without skips and passed against real PostgreSQL. It proved normalized country persistence before provider selection; IR/Mailerino and non-IR/Resend routing; missing/invalid-country rejection; inactive reservation before provider acceptance; activation and provider-failure compensation; digest/TTL/attempt state; email/phone ownership isolation; five-attempt exhaustion; resend replacement; replay rejection; exactly-one concurrent consume under row locking; reset anti-enumeration; reset digest/TTL; provider compensation; one-time password-set handle; password change; unused-code invalidation; User-session invalidation without Admin-session invalidation; and transaction rollback.

## 19. Combined User BFF integration results

The combined test constructed the real User auth context, isolated Admin auth context, real DB pool, real Redis client, country router, HMAC hasher, SMS OTP service, and actual registration, verification, and reset handlers. It exercised both runtimes concurrently with only in-process fake Mailerino, Resend, and SMS providers. SEC-001 namespace isolation remained intact and the SEC-002 Go/frontend regressions passed. Missing/invalid production provider and HMAC configuration remained covered by executed configuration tests.

## 20. Controlled failure and recovery

Executed controlled failures included provider rejection, reservation deletion between provider acceptance and activation, stale Redis reservation activation, unavailable Redis commands, missing registration role inside a transaction, and invalid startup configuration fixtures. Each failed closed; no usable code or false success remained. A clean rerun passed. During infrastructure setup, an unsupported PowerShell RNG API and then an incorrectly interpolated container password were detected before acceptance evidence; the PostgreSQL container and dedicated volume were destroyed and recreated with a verified CSPRNG credential before migrations/tests were accepted.

## 21. Concurrency and race results

Real Redis concurrent issuance left one active code and one cooldown rejection; concurrent consumption produced one success. Real PostgreSQL concurrent verification produced one success under `FOR UPDATE`. `go test -race` passed both the Redis runtime matrix and combined User BFF matrix.

## 22. Fake-provider isolation evidence

All delivery providers were in-process recording/failure fakes. No test process received a real provider base URL or credential. No Mailerino, Resend, KaveNegar, production, staging, or real-recipient request occurred.

## 23. Frontend frozen-install result

The first `pnpm install --frozen-lockfile` attempt timed out; the resumed identical command completed successfully with 268 lockfile-resolved packages. The lockfile was unchanged.

## 24. Lockfile before/after checksum

Before and after SHA-256 were both `B548E14AD7D030502231EF18F316F7161EA7AB3358E0FBADCD995C7579171427`.

## 25. Frontend lint result

The first lint run exposed four errors: two legacy Vue declaration generics and two useless initial CAPTCHA-token assignments in the SEC-003 reset page. Minimal type/initializer corrections were made. The final lint command exited zero with warnings only.

## 26. Frontend typecheck result

The first typecheck exposed one unsupported Vitest matcher in the new focused test. It was replaced with supported invocation-order assertions. The final `vue-tsc --noEmit` exited zero.

## 27. Frontend test result

Vitest executed three files and eight tests: all passed. Five focused SEC-003 tests prove initial normalized country submission, fail-before-request for absent/malformed country, no locale/provider inference, reset handles absent from URLs/storage/logging, and preserved User/Admin and URL-token isolation. Three SEC-002 WebSocket tests also passed.

## 28. Frontend build result

The production command ran `vue-tsc --noEmit && vite build`, transformed 864 modules, and exited zero. Vite emitted only existing chunking warnings.

## 29. Bash syntax result

`bash -n scripts/secrets/init-secrets.sh scripts/secrets/migrate-from-env.sh` executed through Git Bash and returned exit code zero. It parsed only; it did not create, migrate, or print secrets.

## 30. Compose/configuration result

`docker compose -f infra/docker/docker-compose.yml config --no-interpolate --quiet` returned zero. Production security configuration tests and secret-loader tests passed.
## 31. Regression results and command ledger

All final required regressions passed. Commands containing a database credential are shown with `<temporary-password>`; the real value was never printed.

| Command | Exact sanitized result |
|---|---|
| `Get-Content -Raw <attached remediation request>` | Exit 0; complete replacement invocation read. |
| Required `Get-Content -Raw`, `rg`, `Select-String`, and `Test-Path` authority/state inventory | Exit 0 except documented Windows wildcard retries; Phase 0/SEC-001/SEC-002 PASS, SEC-003 FAIL, SEC-004 absent, `.git` absent. |
| Starting `go test ./packages/sms ./packages/notification ./packages/secrets ./apps/user-bff/server -count=1` | Sandbox cache creation failed with access denied; identical outside-sandbox run exited 0 and all four packages passed. |
| `node scripts/sec-003-otp-delivery-check.mjs` and focused Node self-test | Initial focused checks passed: structural PASS and 4/4 tests. |
| Docker/WSL/native-tool/package-manager preflight commands | Exit 0; Docker CLI/daemon/Compose available; WSL available; host PostgreSQL/Redis tools absent; Git Bash, Node, Corepack, pnpm available; no inherited external targets or provider credentials. |
| Exact local-image inspection for `postgres:16-alpine` and `redis:7-alpine` | Exit 0; recorded content IDs, platform, and creation metadata. |
| `docker run` for the named loopback PostgreSQL/Redis containers | Redis started. First PostgreSQL credential generation used an unsupported API and was discarded; container/volume removed before tests. Corrected CSPRNG recreation succeeded. |
| PostgreSQL credential verification using container-local `psql` | First recreation exposed an environment interpolation error and authentication failed. That container/volume was removed; corrected recreation and password-authenticated `SELECT 1` succeeded without printing the password. |
| Readiness/version commands | Exit 0; PostgreSQL `16.14`; Redis `7.4.9`; sanitized database/user/ports matched preflight. |
| Deterministically sorted migration loop piped to `psql -X -q -v ON_ERROR_STOP=1` | Exit 0; 98/98 up migrations applied. Corrected runtime recreation repeated 98/98. |
| Temporary migration-manifest validation | Exit 0; 98 entries, 98 unique names, zero duplicates; recorded SHA-256. |
| First real `go test ./packages/sms -run '^TestSEC003RedisOTPLifecycle$' -count=1 -v` | Exit 0; PASS, no skip. |
| First combined User BFF run | Exit 1 before application behavior because the discarded PostgreSQL container used an invalid password injection. Runtime was recreated. |
| Next combined User BFF run | Exit 1; test fixture extracted an HTML numeric entity instead of the rendered code. |
| Focused extractor correction plus regression test; rerun | Extractor regression passed; run then exposed missing migrated `email_verified_at` in the focused schema fixture. |
| Add fixture column from migration `0022`; rerun | Exit 0; original four combined subtests plus extractor regression passed. |
| Expanded real Redis matrix | Exit 0; both runtime tests and all subtests passed without skips. |
| Expanded real PostgreSQL/combined matrix | Exit 0; registration, verification, SMS, resend, concurrency, failure compensation, and reset subtests passed without skips. |
| `go test -race ./packages/sms -run '^TestSEC003RedisOTP' -count=1 -v` | Exit 0; both tests and all subtests passed. A sandbox-only Go telemetry upload-token warning followed the successful test result. |
| `go test -race ./apps/user-bff/server -run 'Test(SEC003IntegrationPostgresRedisLifecycle|ExtractRenderedSecurityCodeIgnoresHTMLNumericEntities)$' -count=1 -v` | Exit 0; all subtests passed. Controlled missing-role failure emitted only `sql: no rows in result set`; no credential. |
| First `pnpm install --frozen-lockfile` | Timed out after 124 seconds; partial install was not accepted as evidence. |
| Resumed `pnpm install --frozen-lockfile` | Exit 0; 268 packages, completed in 68.3 seconds. |
| First `pnpm --filter @tragge/user-frontend lint` | Exit 1; four errors and 225 warnings. Minimal declaration/CAPTCHA initializer corrections followed. |
| Final `pnpm --filter @tragge/user-frontend lint` | Exit 0; zero errors, 224 warnings. |
| First `pnpm --filter @tragge/user-frontend typecheck` | Exit 2; one unsupported test matcher. |
| Final `pnpm --filter @tragge/user-frontend typecheck` | Exit 0; no output beyond script banner. |
| `pnpm --filter @tragge/user-frontend test` | Exit 0; three files, eight tests, all passed. Existing Vue lifecycle warnings only. |
| `pnpm --filter @tragge/user-frontend build` | Exit 0; typecheck plus Vite production build, 864 modules transformed. Existing chunk warnings only. |
| Sandboxed Git Bash `bash -n ...` | Exit `-1073741502`; MSYS signal-pipe access failure, parser did not run. |
| Outside-sandbox Git Bash `bash -n scripts/secrets/init-secrets.sh scripts/secrets/migrate-from-env.sh` | Exit 0; real Bash parser passed. |
| Final targeted four-package Go suite | Exit 0; `packages/sms`, `packages/notification`, `packages/secrets`, and User BFF passed. |
| SEC-001/SEC-002 six-package Go regression | Exit 0; auth, validation, User BFF, Admin BFF, API server, and Trade BFF passed. |
| `go vet` over touched Go packages/apps | Exit 0; no vet finding. Sandbox-only telemetry warning did not change exit status. |
| `go build -buildvcs=false` over touched packages/apps | Exit 0; no build finding. |
| `gofmt -d` over both changed Go tests | Exit 0; no diff. |
| SEC-003 structural validator after frontend normalization | First run found one stale expected fragment. Validator updated to the normalized helper; final run PASS over eight files. |
| Focused structural self-test inside sandbox | Spawn `EPERM`; outside-sandbox rerun exit 0, 4/4. |
| First eight-file Phase 0/SEC Node regression | Exit 1; 54/56 passed. Only expected TypeScript and Go-import inventory counts were stale after new tests. |
| Corrected eight-file Phase 0/SEC Node regression | Exit 0; 56/56 passed, zero skipped. |
| `docker compose -f infra/docker/docker-compose.yml config --no-interpolate --quiet` | Exit 0; no output. |
| `node scripts/production-baseline.mjs inventory` | Exit 0; Go 396, Vue 211, TypeScript/TSX 181, SQL 202, up migrations 98, down migrations 99, Go tests 115, frontend tests/specs 12. |
| `node scripts/production-baseline.mjs verify` | Exit 0; reproducible inventory, 35 evidence rows, 146 local links, and toolchains passed; three CI patch-version warnings retained. |
| Lockfile/toolchain after-check | Exit 0; Node/Corepack/pnpm matched preflight and lockfile checksum was unchanged. |
| Read-only final runtime identity/manifest query | Exit 0; PostgreSQL identity correct; manifest 98 unique entries; Redis dedicated `DBSIZE` reflected test keys only. |
| `docker rm -f` for the two named test containers; `docker volume rm` for two named volumes | Exit 0; all four exact disposable targets removed. |
| First sandbox filesystem cleanup | Non-terminating access errors while removing elevated pnpm files; not accepted as cleanup success. Other eligible targets were attempted. |
| Elevated native PowerShell cleanup passes over verified `node_modules` | First two passes encountered transient pnpm link/missing-path errors. A long-path-aware final pass exited 0 and `node_modules` was absent. |
| Elevated exact `C:/tmp/tragge-sec003*` cleanup | Exit 0; credential, manifest, logs, and two SEC-003 Go caches removed. |
| Final Docker/container/volume/port verification | Exit 0; container count 0, volume count 0, listener count 0. |
| Final local artifact check | Exit 0; `node_modules=False`, frontend `dist=False`, temporary SEC-003 items 0, `.git=False`, lockfile checksum unchanged. |
| Final report structure/decision validation | Exit 0; all three report/architecture files existed, remediation had 42 expected numbered sections, current remediation and task decisions were PASS, original FAIL history remained, and no standalone six-digit value appeared outside documented SHA-256 digests. |
| Focused changed-file credential scan | Exit 0; 13 files scanned, with zero credential-bearing database URLs, private keys, bearer credentials, or complete JWTs. |
| Final `node scripts/sec-003-otp-delivery-check.mjs` after cleanup | Exit 0; PASS over eight active security-code source files. |
| Final `node scripts/production-baseline.mjs verify` after cleanup | Exit 0; reproducibility, documented deltas, 35 P0/P1 evidence paths, 146 local Markdown links, and toolchains passed; three existing CI patch-version warnings remained. |
| Final eight-file Phase 0/security Node regression after cleanup | Exit 0; 34/34 tests passed, zero failed, zero skipped. |

## 32. Secret and captured-log scan result

Runtime credentials were kept outside the repository, never printed, and deleted. The captured outputs contained no generated OTP, reset handle, HMAC secret, PostgreSQL password, credential-bearing DSN, provider key, authorization header, or message body. Exact runtime canaries were compared in memory rather than written to the report. Changed-source scans found only clearly marked test fixtures, not real credentials. The controlled database failure log exposed only a generic SQL no-row error.

## 33. Files changed

1. `apps/user-bff/server/security_code_integration_test.go`
2. `packages/sms/otp_redis_integration_test.go`
3. `apps/user-frontend/src/stores/auth.ts`
4. `apps/user-frontend/src/stores/auth.sec003.test.ts`
5. `apps/user-frontend/src/env.d.ts`
6. `apps/user-frontend/src/modules/user/views/ForgotPasswordPage.vue`
7. `scripts/sec-003-otp-delivery-check.mjs`
8. `scripts/production-baseline.mjs`
9. `scripts/production-baseline.test.mjs`
10. `scripts/target-architecture.test.mjs`
11. `docs/architecture/target-architecture-import-review.md`
12. `docs/codex/reports/SEC-003-failed-gate-remediation.md`
13. `docs/codex/reports/SEC-003-local-execution-report.md`

No migration, lockfile, dependency declaration, provider production code, SEC-004 file, or infrastructure design changed.

## 34. Runtime-discovered defects and smallest corrections

- Test-only email code extraction matched an HTML numeric entity. It now extracts the styled rendered-code span; a focused regression protects it.
- The focused PostgreSQL fixture omitted migrated `email_verified_at`. The fixture now includes that existing migration column.
- Existing runtime coverage was too narrow. Focused real Redis and PostgreSQL matrices were expanded without changing production policy or constants.
- The frontend store did not normalize direct registration callers. It now trims/uppercases and rejects malformed alpha-2 country input before the initial request; Vitest covers this.
- Two pre-existing frontend toolchain errors were minimally corrected: the Vue declaration uses `DefineComponent` defaults, and CAPTCHA variables no longer have unused initial values.
- The new test used a matcher unavailable in pinned Vitest; it now uses supported invocation-order data.
- Structural and inventory validators were updated for one new TypeScript test and one new internal Go import. No architecture edge changed.
- The unsupported PowerShell RNG call and incorrect Docker environment interpolation were remediation setup defects. Both runtimes were destroyed and recreated before accepted evidence; no repository source changed for them.

The built-in patch helper could not operate under the Windows split-root sandbox and the local helper executable was inaccessible. After explicit user authorization, exact occurrence-counted mechanical file replacements were used, then formatting, typechecking, and tests verified every edit.

## 35. Known untested behavior

- Real Mailerino, Resend, and KaveNegar delivery was intentionally prohibited.
- A full standalone User BFF process including unrelated storage, KYC, OAuth, and network listener startup was not launched. The actual SEC-003 App contexts, constructors, handlers, database, Redis, routing, configuration validation, and session boundaries were executed in the focused combined integration test.
- Browser-driven E2E was not required for these evidence gaps; the actual pinned Vitest configuration and production build executed.
- Paid-production launch, production secrets, and production data remain untested and unauthorized.

## 36. Cleanup evidence

The PostgreSQL and Redis containers were stopped/removed. Both dedicated volumes were removed. Final Docker inspection found zero matching containers and volumes. Loopback ports `55433` and `56380` had zero listeners. Temporary credentials, migration manifest, logs, Go caches, frontend `node_modules`, and generated `dist` were removed. No `C:/tmp/tragge-sec003*` item remains. The lockfile is unchanged and `.git` remains absent.

## 37. Explicit remediation decision

`SEC-003 PASS`

Every mandatory failed-gate evidence gap executed and passed. No test reported `SKIP`; runtime concurrency/failure compensation passed; frontend frozen install/lint/typecheck/test/build passed; Bash syntax passed; all required backend and prerequisite regressions passed; cleanup completed.

## 38. SEC-004 status

`SEC-004` was not started. The Phase 1 exit gate was not run.

## 39. Git metadata

No Git metadata was created.

## 40. Remote source-control operation

No remote source-control connection or operation occurred.

## 41. Real provider contact

No real provider, production infrastructure, staging environment, or real recipient was contacted.

## 42. Paid-production status

Paid-production status remains `NO-GO`.