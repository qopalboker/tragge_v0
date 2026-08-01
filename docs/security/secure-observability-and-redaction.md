# Secure Observability and Redaction

Status: Implemented for SEC-005 on 2026-08-01

## Authority and boundaries

This design implements SEC-005 under the [fixed product and technical policies](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md), the [production roadmap](../codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md), and [ADR 0001](../adr/0001-target-runtime-architecture.md). It preserves the authentication, URL-credential, OTP/reset, and privileged-action boundaries established by SEC-001 through SEC-004.

It does not implement SEC-006 edge security, rate limiting, CORS/CSRF redesign, WAF, or abuse controls. It does not implement the planned SEC-007 Super Admin MFA/TOTP work. Paid-production status remains `NO-GO`.

## Inspected logging inventory

| Area | Active paths inspected | Pre-SEC-005 finding | SEC-005 treatment |
|---|---|---|---|
| Platform merged API runtime | [`apps/api-server/main.go`](../../apps/api-server/main.go), User/Admin/payment server composition | Standard-library startup and dependency errors plus raw production zap construction | Install the standard writer before startup output and wrap transitional zap construction |
| Admin BFF and password reauthentication | [`apps/admin-bff/server/app.go`](../../apps/admin-bff/server/app.go), Admin handler/audit paths, circuit fallback | Structured observability plus Sentry; generic recovery and one plain background panic path could bypass sanitization | Central zap core, Sentry before-send hook, sanitized recovery, sanitized background panic, audit metadata sanitation |
| User and Trade BFFs | [`apps/user-bff/server/app.go`](../../apps/user-bff/server/app.go), [`apps/trade-bff/server/app.go`](../../apps/trade-bff/server/app.go) | SEC-002 credential-removal middleware existed; generic recovery and plain WebSocket-worker panic output remained | Preserve SEC-002 restoration order; add centralized recovery and panic sanitation |
| Workers and backend services | Worker, scheduler, generator, leaderboard, settlement, trading, ingestor, and shard-router entry points under [`apps/`](../../apps/) | Most used structured observability, but settlement used chi text logging, all routers used generic recovery, and fallback loggers were not uniformly wrapped | All active routers use observability request/recovery middleware; production zap constructors are wrapped |
| Authentication/session and OTP/reset | [`packages/auth`](../../packages/auth), [`packages/sms`](../../packages/sms), User/Admin handlers | Focused SEC-002/SEC-003 guards removed telemetry credentials and prohibited code logging; errors can still flow through outer sinks | Central message/error/field redaction supplies defense in depth without changing authentication decisions |
| Payments, webhooks, providers, and KYC | Payment handlers/providers, notification adapters, KYC handlers, market providers | Provider/database errors and structured metadata could contain credential-bearing URLs or nested private fields | Central field/text redaction, payment request credential removal, notification fallback wrapping, Sentry body/query omission |
| Error and panic telemetry | Sentry clients in Admin, User, Trade, and payment services; chi recovery; background `recover` sites | Error values, Sentry query strings, and recovered values were not governed by one policy | Typed Sentry before-send sanitation, redacting zap core, sanitized HTTP recovery, focused plain-log migration |
| Audit-adjacent records | [`packages/audit/audit.go`](../../packages/audit/audit.go) and privileged-action audit code | Arbitrary metadata maps were logged and serialized without a shared recursive sanitizer | Sanitize a copy before both logging and database persistence; preserve allowed result/action identifiers |
| Secrets diagnostics | [`packages/secrets/secrets.go`](../../packages/secrets/secrets.go) | `MaskSecret` exposed secret prefixes and suffixes | Any non-empty secret becomes the stable non-reconstructable marker |
| Frontend logging/telemetry | Shared frontend logger, User/Admin entry points, User Sentry identity | Logger arguments and direct console errors could serialize Axios errors or private fields; Sentry user identity included email/username | Shared recursive frontend sanitizer, early console wrapper, safe Sentry pseudonymous ID only |

No request/response logger was found that intentionally needs raw bodies or raw query strings. Application HTTP logs keep method, normalized path, status, duration, byte count, safe request ID, and trace ID. They do not log the raw query, headers, or body.

## Central APIs and sink enforcement

The canonical Go implementation is [`packages/observability/redaction.go`](../../packages/observability/redaction.go). `RedactText`, `RedactValue`, `RedactHeaders`, `RedactURL`, `RedactError`, and `RedactPanic` all use `[REDACTED]`. [`redacting_core.go`](../../packages/observability/redacting_core.go) applies the policy immediately before zap encodes JSON or console output. `InstallStandardLoggerRedaction` protects transitional `log` calls, and `WrapLogger` protects constructors that cannot yet use the canonical logger directly.

[`sentry.go`](../../packages/observability/sentry.go) is the mandatory before-send hook for existing Go Sentry clients. It removes request query strings, bodies, cookies, and direct personal identity fields, and recursively sanitizes errors, breadcrumbs, contexts, tags, headers, and extras.

The browser implementation is [`packages/frontend-shared/src/utils/logger.ts`](../../packages/frontend-shared/src/utils/logger.ts). It sanitizes nested objects, arrays, `Error`, `URL`, `Headers`, `URLSearchParams`, and `FormData`; both frontend entry points install console sanitation before application startup.

## Sensitive-field taxonomy

Case-insensitive matching covers:

- authorization headers, JWTs, access/refresh/session/reset tokens, CSRF material, passwords and hashes, API/client secrets, signed grants, tickets, and cookies;
- OTP, reset/security codes, legacy TOTP secrets, recovery/provisioning material, and signed authentication material;
- payment/provider API credentials, webhook secrets/signatures, bank/card/account private fields, and provider-private payloads;
- KYC identity/document fields, document bytes/references, email, phone, national identifiers, and other explicitly private profile fields;
- credential-bearing PostgreSQL, Redis, HTTP(S), cloud, and provider URLs; encryption/private keys and secret environment values.

New credential-bearing fields must be added to the centralized key and text taxonomy and covered by both unit and capture tests. Call sites must not partially mask, hash, fingerprint, prefix, or suffix a credential for logging. Safe operational facts—pseudonymous actor ID, action, resource ID, result, reason code, timestamp, and correlation ID—remain available when policy permits.

## Correlation IDs

`X-Request-ID` is accepted only after trimming, a length check of 8–128 characters, a safe-character allowlist, and confirmation that text redaction would not change it. A valid OpenTelemetry trace ID is the next choice. Otherwise the middleware generates 128 random bits with a non-secret time/counter fallback only if the operating-system random source fails.

The chosen value is written to the request header, response header, request context, normal request log, and sanitized panic log. It is diagnostic only and never grants authority. Validation middleware now propagates its generated UUID to downstream observability so one request keeps one ID.

## Error, panic, and audit behavior

HTTP panics are caught inside the observability request boundary. Clients receive only the generic HTTP 500 status text. Logs retain method, normalized path, safe correlation ID, sanitized panic text, and sanitized stack. Background recovery paths either use the protected zap core or explicitly sanitize before plain-text emission.

Error values are converted to redacted text before encoding. Opaque zap object/array marshalers are conservatively replaced because their nested output cannot be inspected safely. Sentry drops raw queries, request bodies, cookies, and direct personal identity data. Audit metadata is copied and recursively sanitized before any logger or database sink sees it; required audit facts remain, and SEC-004 transactional audit requirements are unchanged.

## Safe logging rules

- Log an action category, safe resource identifier, result, reason code, and correlation ID.
- Pass errors to a protected structured logger; do not concatenate headers, bodies, credentials, or provider payloads.
- Never log complete requests, responses, URLs with queries, form data, maps, or structs unless they first pass the central sanitizer.
- Never use a credential as a request ID, metric label, trace attribute, error message, panic value, audit field, or frontend telemetry field.
- Wrap every fallback production zap logger and install the shared frontend console policy before application startup.
- Treat structural-validator findings as failures; do not add exclusions merely to silence a finding.

## Validation model

Go unit and capture tests cover key variations, nested maps/arrays, headers, queries, credential URLs, text, wrapped errors, JSON/console zap output, standard text output, Sentry events, HTTP recovery, audit metadata, and correlation IDs. Frontend tests exercise nested argument sanitation and actual logger capture. [`scripts/sec-005-redaction-check.mjs`](../../scripts/sec-005-redaction-check.mjs) rejects unwrapped production zap construction, generic recovery/logging middleware, raw panic formatting, missing Sentry hooks, missing frontend installation, and later-task report evidence.

Fixtures are synthetic and must never be copied from a real environment. Captured output and task reports are scanned after tests.

## Access, retention, and incident response

SEC-005 changes application emission, not the future production sink or retention architecture. A paid-production logging platform must restrict access by least privilege, encrypt transport/storage, define approved retention and deletion periods, record administrative access, and prevent public indexing or analytics export of private events. Those controls remain launch work; this document does not claim them implemented.

If a credential is found in any log or telemetry sink:

1. stop or restrict the affected emission and sink access without destroying required evidence;
2. identify the exposed credential class and affected time range without copying the value into tickets or reports;
3. rotate/revoke the credential, invalidate associated sessions or grants, and assess dependent providers;
4. remove or quarantine exposed records according to legal/retention authority while preserving a sanitized incident audit trail;
5. add a regression fixture for the field shape and rerun capture/static scans before restoring emission.

## Known limitations and later work

- Static analysis is intentionally focused and cannot prove that arbitrary future third-party sinks are safe; all new sinks require capture tests and the central adapters.
- Sink RBAC, retention automation, production alert routing, and cross-service end-to-end trace/log operations are not established by this task.
- SEC-006 owns broader edge, CORS/CSRF, WAF, rate-limit, abuse, and bot controls.
- Planned SEC-007 owns Super Admin MFA/TOTP and remains not started.

These limitations and all remaining roadmap launch gates keep paid-production status at `NO-GO`.
