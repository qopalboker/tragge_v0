# Edge security and abuse controls

Status: implemented target for SEC-006 on the current Platform API runtimes.
Paid-production status: **NO-GO**.

This document is subordinate to the [fixed policies](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md), [ADR 0001](../adr/0001-target-runtime-architecture.md), and [canonical execution protocol](../codex/CODEX_EXECUTION_PROTOCOL.md). It describes the current SEC-006 boundary; it does not claim that a managed production edge platform exists.

## Trust boundaries and implementation

User, Admin, Trade, and Payment retain separate middleware construction. Browser origins are exact scheme/host/port values. Admin never inherits User origin wildcards. The socket peer is authoritative unless that immediate peer is in **TRUSTED_PROXY_CIDRS**.

The shared implementation is in:

- [validation middleware](../../packages/validation/middleware.go);
- [trusted client IP handling](../../packages/validation/ip.go);
- [CORS policy](../../packages/validation/cors.go);
- [CSRF policy](../../packages/validation/csrf.go);
- [startup validation](../../packages/validation/edge_config.go);
- [distributed policy limiter](../../packages/resilience/ratelimit/policy.go);
- [distributed login lockout](../../packages/resilience/ratelimit/login_lockout.go);
- [payment webhook freshness and replay control](../../apps/payment-service/handlers/webhook_security.go).

Redis is an expiring coordination store for counters, lockouts, and replay markers. It is not an identity or financial source of truth. High-risk store failure fails closed with 503. Threshold rejection uses 429 and bounded Retry-After.

## Entry-point and abuse-policy matrix

Every route receives a catch-all public-read policy; longer method/path policies win. Metrics remain protected by InternalOnlyMiddleware. Provider webhooks do not use browser CORS or CSRF as authentication.

| Class | Surface and representative route | Access | Body/content | Origin/CSRF | Distributed policy | Additional control |
| --- | --- | --- | --- | --- | --- | --- |
| Operational/public read | all services health/readiness and public reads | public; metrics internal | empty/default 1 MiB | no credential; exact CORS if Origin exists | health 300/min plus 30/sec; catch-all 120/min plus 20/sec | 503 on Redis failure; security headers |
| User login | User BFF POST /api/user/auth/login | public | 1 MiB JSON/form | exact User origin and User CSRF | 10/10 min plus 3/min by trusted IP | IP/account lockout: 8 failures/hour, 15-minute lock |
| Admin login | Admin BFF POST /api/admin/auth/login | public Admin context | 1 MiB JSON | exact Admin origin and Admin CSRF | 5/10 min plus 2/min by trusted IP | IP/account lockout: 5 failures/2 hours, 30-minute lock |
| Registration | User BFF registration routes | public | 1 MiB JSON | exact User origin and CSRF | 5/10 min plus 2/min by IP | generic response; existing CAPTCHA retained |
| OTP request | User BFF send-OTP and security-code issue | public/authenticated by route | 1 MiB JSON | exact User origin and CSRF | 3/10 min plus 1/min by IP | Redis destination/purpose scope and 60-second cooldown |
| OTP verify | User BFF verify-OTP/security-code routes | public/authenticated by route | 1 MiB JSON | exact User origin and CSRF | 10/10 min plus 3/min by IP | five attempts, ten-minute expiry, atomic one-time consume |
| Password reset | User BFF forgot-password routes | public | 1 MiB JSON | exact User origin and CSRF | 5/10 min plus 2/min by IP | SEC-003 purpose-bound state and generic errors |
| Session operations | User/Admin refresh, logout, session routes | context-specific | default/empty | context origin/CSRF; bearer-only exempt | service default and actor dimension | SEC-001 namespaces and SEC-002 transport retained |
| Contest join | User BFF POST /api/user/contests/{id}/join | authenticated User | 1 MiB | exact User origin and CSRF | 15/min plus 3/sec by IP and actor | existing per-user limiter retained |
| Order placement | Trade BFF POST /api/trade/orders | authenticated User | 1 MiB JSON | exact Trade origin and User CSRF | 60/min plus 10/sec by IP and actor | Trading Engine remains authoritative |
| Order cancellation | Trade BFF DELETE /api/trade/orders/{id} | authenticated User | empty/default | exact Trade origin and User CSRF | 90/min plus 15/sec by IP and actor | UUID validation |
| WebSocket connect | Trade BFF /ws/trade and /ws/tournaments | SEC-002 ticket/User or public feed | handshake | exact Trade origin required | 20/min plus 3/sec by IP | bounded ticket and connection limits retained |
| Deposit initiation | Payment POST /api/payments/deposit/* | authenticated User | 1 MiB JSON | exact Payment/User origin | 10/10 min plus 2/min by IP and actor | current financial checks retained |
| Withdrawal initiation | Payment POST /api/payments/withdraw/request | authenticated User | 1 MiB JSON | exact Payment/User origin | 5/10 min plus 2/min by IP and actor | KYC/wallet checks retained |
| Privileged Admin | Admin /api/admin/* and SEC-004 actions | Admin role/permission | 1 MiB; uploads 35 MiB | exact Admin origin and Admin CSRF | 120/min plus 20/sec by IP and actor; reauth 5/5 min plus 2/min | SEC-004 grant/reason/audit mandatory |
| Provider webhook | Payment POST /webhooks/nowpayments | provider authentication | strict 1 MiB JSON | no browser authorization | 120/min plus 20/sec by IP | signature, five-minute freshness, replay marker |
| Jibit callback | Payment GET/POST /callback/jibit | provider callback | empty/default | no browser authorization | 120/min plus 20/sec by IP | configured CIDR defense in depth |
| Static/frontend | gateway public assets | public | no body | surface-specific exact error CORS | gateway request/connection limits | immutable cache only for hashed assets |

User API default is 180/min plus 30/sec, Trade API 240/min plus 40/sec, Payment API 180/min plus 30/sec, and Admin 120/min plus 20/sec. These are implementation defaults, not product policy. Counters expire at twice their decision window.

## Request framing, sizes, and content

Go servers cap request-line/header bytes at 16 KiB, use a five-second header timeout, and retain 15-second read/write and 60-second idle timeouts. Default body limit is 1 MiB; only existing approved uploads opt into 35 MiB. Startup bounds are 1 KiB to 8 MiB for default, default to 64 MiB for uploads, and 8 to 64 KiB for headers.

Known Content-Length is checked, invalid transfer encodings are rejected, and http.MaxBytesReader prevents chunked/deceptive bypass. Strict JSON decoding permits one value. Non-empty state changes accept only application/json, form URL encoding, multipart/form-data, or application/octet-stream; handlers choose the narrower applicable type. Rejected bodies are never logged.

The gateway retains smaller auth limits and existing upload limits justified by current KYC/avatar contracts. The Go boundary remains authoritative if development traffic bypasses the gateway.

## Trusted proxy and client IP

Empty proxy configuration trusts none; production rejects empty or malformed proxy configuration. Docker Compose pins platform_net to 172.30.0.0/24 and uses that only as a local development/test default. The checked-in production Nginx baseline also trusts no forwarding source: deployment automation must generate exact ingress set_real_ip_from entries. Broad RFC1918 ranges are prohibited.

For trusted peers, X-Forwarded-For is walked right-to-left until the first untrusted hop. Invalid entries fall back to the socket peer. One valid X-Real-IP is considered only without X-Forwarded-For. Forwarded, CF-Connecting-IP, and provider headers are ignored. IPv4/IPv6 and ports are normalized. Policy keys hold only SHA-256 digests, never raw IPs, identifiers, tokens, cookies, OTPs, or bodies.

## CORS, CSRF, and WebSockets

Separate configuration keys are **USER_CORS_ALLOWED_ORIGINS**, **ADMIN_CORS_ALLOWED_ORIGINS**, **TRADE_CORS_ALLOWED_ORIGINS**, and **PAYMENT_CORS_ALLOWED_ORIGINS**. Values must be exact HTTP(S) origins without path, query, fragment, userinfo, opaque/null value, suffix match, or wildcard. Production User and Admin origin sets may not overlap. Credentialed wildcard configuration fails construction. A supplied disallowed origin gets 403; absent Origin is defined as same-origin/non-browser HTTP and proceeds to authentication. Preflight validates method/headers and sets required Vary fields.

Cookie state changes require an exact context Origin (or valid Referer origin) and X-Requested-With. User and Admin contexts remain distinct. Bearer-only requests are exempt from cookie-specific CSRF. Provider callbacks are separated on the Payment surface, not by a configurable User/Admin skip. SameSite complements this control.

WebSocket upgrades require an exact Trade origin. Missing, wildcard, cross-context, and malformed origins fail. Session JWT query authentication remains prohibited by SEC-002.

## Security headers

API success and middleware errors receive nosniff, frame denial, restrictive API CSP, Referrer-Policy, Permissions-Policy, COOP, CORP, and private no-store caching. The obsolete browser XSS auditor is disabled at the gateway. HSTS appears only for direct TLS or a trusted proxy reporting HTTPS; local HTTP receives none. Go deliberately omits COEP because compatibility is not established for every frontend/provider asset; the pre-existing gateway credentialless setting remains subject to frontend regression.

## Login lockout and OTP

User/Admin lockouts use separate Redis namespaces, hash trusted-IP and normalized-account identities, update atomically, expire, and return generic responses. Success clears relevant dimensions. Missing storage fails closed. No administrative unlock was invented.

SEC-003 OTP controls remain authoritative: CSPRNG codes, HMAC-digested destination/purpose state, ten-minute expiry, 60-second resend cooldown, five attempts, atomic concurrent issue/consume, one-time use, provider acceptance before activation, and fail-closed Redis/provider errors. SEC-006 adds trusted-IP endpoint policy without replacing destination/purpose controls.

## Payment webhooks

NOWPayments must pass provider signature verification before freshness/replay checks. Production requires the NOWPayments IPN secret and a Jibit CIDR allowlist. Signed events must be within **PAYMENT_WEBHOOK_MAX_AGE** (default five minutes), at most one minute in the future, and unseen in Redis. Replay keys digest provider/event/body and contain no signature or payload. Duplicate delivery gets a generic acknowledgement without repeated mutation; store failure gets 503.

IP allowlisting is Jibit defense in depth only. Browser CORS never authenticates callbacks. Existing transaction/idempotency checks remain the financial mutation boundary.

## Observability and failure behavior

Denials log only low-cardinality policy class and safe reason, such as window_exceeded, burst_exceeded, or storage_unavailable. SEC-005 correlation IDs and centralized redaction remain before application observability. Responses contain no private counter key or cryptographic detail.

Gateway limits and legacy token buckets are process-local defense in depth. SEC-006 policy counters, login locks, OTP state, and webhook replay markers are distributed through Redis and were tested with isolated Redis 7.4.5. High-risk Redis failure fails closed.

## Configuration, deployment, rollback

Safe examples are in [.env.example](../../.env.example) and [Docker Compose](../../infra/docker/docker-compose.yml). Production must provide deployed exact origins, exact ingress CIDRs, provider secrets through existing secret files, and Jibit CIDRs.

Deploy frontend/gateway origin changes before backend tightening and Redis readiness before application replicas. Roll back application and gateway together. Never disable store-failure handling, signature verification, SEC-001 isolation, SEC-002 transport, SEC-003 delivery, SEC-004 reauthentication, or SEC-005 redaction to restore traffic.

## Validation and limitations

Focused validation is in [the SEC-006 validator](../../scripts/sec-006-edge-security-check.mjs) and [its tests](../../scripts/sec-006-edge-security-check.test.mjs). Results are in the [SEC-006 report](../codex/reports/SEC-006-git-execution-report.md).

Known limitations:

- production CDN/ingress behavior, live provider ranges, and real signatures were not tested with live credentials;
- Nginx limits are per process; Redis policy is distributed enforcement;
- no WAF/CDN/bot platform was introduced;
- full browser E2E depends on the available frontend runtime;
- Phase 1 Exit Gate has not run.

SEC-007 implements the separate Super Admin MFA control without changing these
SEC-006 edge policies. This does not approve paid production; paid-production
status remains **NO-GO**.
