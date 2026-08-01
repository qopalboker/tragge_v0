# OTP and Password-Reset Delivery

Status: Implemented by `SEC-003` for the current User BFF runtime. Paid-production status remains `NO-GO`.

This document applies the [fixed security policy](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md), keeps User identity inside the Platform boundary from [ADR 0001](../adr/0001-target-runtime-architecture.md), preserves the [SEC-001 User/Admin authentication isolation](user-admin-authentication-isolation.md), and preserves the [SEC-002 prohibition on reusable session credentials in URLs](session-authentication-url-policy.md).

## Canonical lifecycle

All active User verification and password-reset codes follow one policy:

- six numeric digits generated from `crypto/rand`;
- ten-minute validity after provider acceptance;
- 60-second resend cooldown;
- five verification attempts;
- one active code per User, purpose, and destination context;
- HMAC-SHA-256 at rest with the dedicated `SECURITY_CODE_HASH_SECRET`;
- timing-safe comparison;
- binding to purpose, User identifier, normalized destination, channel, and reset-request context where applicable;
- atomic one-time consumption; replay and concurrent duplicate consumption fail;
- resend replaces the earlier active code;
- expiration, exhaustion, use, and replacement make the code unusable.

`SECURITY_CODE_HASH_SECRET` is independent of User/Admin JWT secrets and provider credentials. Production loads it through the approved secret loader, including `SECURITY_CODE_HASH_SECRET_FILE`. Production rejects missing, weak, placeholder, or reused material without logging its value.

Provider acceptance is synchronous. The lifecycle first records an expired reservation, sends through exactly one selected provider, and activates the ten-minute window only after an accepted response. A delivery rejection leaves the reservation expired/used and removes reset-session state. A post-delivery activation failure is reported as unavailable and never as successful delivery.

The 60-second abuse cooldown also applies after a failed issuance reservation. This limits repeated provider pressure; the reserved code remains unusable.

## Active flow inventory

| Flow and endpoint | Authentication | Destination and country | Provider | State | Policy and failure behavior |
|---|---|---|---|---|---|
| Registration `POST /api/user/auth/register` | Public, IP limited | Request email; validated request country is normalized and persisted in the User creation transaction before delivery | `IR`: Mailerino; supported non-`IR`: Resend | `verification_codes`, HMAC digest | Registration does not return session credentials until provider acceptance and activation. Failure returns a generic unavailable response and invalidates the just-created User session. |
| Email ownership send/resend `POST /api/user/auth/send-verification`, `POST /api/user/auth/resend-verification` | User | Stored email and stored canonical country | Country router | `verification_codes`, HMAC digest | 10 minutes, 60 seconds, five attempts; no provider fallback. |
| Email ownership consume `POST /api/user/auth/verify-code`, legacy-shaped `POST /api/user/auth/verify-email` | User | Bound stored destination | No delivery provider | Row lock and atomic `verified_at` update | Marks only email ownership; replay fails. The legacy-shaped endpoint delegates to the same lifecycle and does not write legacy token state. |
| Phone ownership send/consume via `POST /api/user/auth/send-verification` and `POST /api/user/auth/verify-code` | User | Stored phone | KaveNegar when enabled | `verification_codes`, HMAC digest | Marks only `phone_verified`; it cannot prove email ownership. |
| Phone authentication `POST /api/user/auth/send-otp`, `POST /api/user/auth/verify-otp`, `POST /api/user/auth/register-phone` | Public, IP limited | Validated Iranian phone; phone-created Users persist country `IR` | KaveNegar when enabled | Redis namespaced digest, cooldown, attempt counter, and reservation | Redis Lua scripts reserve, activate, count attempts, and consume atomically. Redis/provider failure fails closed. |
| Password-reset request `POST /api/user/auth/forgot-password/request` | Public, IP and User limited | Verified phone first; otherwise verified email plus stored country | SMS: KaveNegar; email: country router | `password_reset_codes` plus `auth:user:password-reset:*` Redis state | The same response shape is used for existing, absent, unavailable, and provider-failed cases. Only accepted delivery returns a usable opaque reset-session handle. |
| Password-reset verify/reset `POST /api/user/auth/forgot-password/verify`, `POST /api/user/auth/forgot-password/reset` | Opaque one-time handles | Bound to original User, destination, channel, and request context | No delivery provider | Row lock plus atomic Redis exchanges | Code and password-set handles are single use. A successful password change must delete every User session before commit; the separate Admin namespace is untouched. |
| Cleanup job | Internal worker loop | None | None | Legacy expired email-token cleanup plus current code cleanup | The legacy table is cleanup-only. Active issuance and verification no longer write or trust its tokens. Its schema removal remains owned by the migration roadmap. |

Password-change alerts sent after a completed reset are non-code notifications. They are not authority for the reset transaction and do not make a failed reset successful.

## Country routing

The only email-provider routing input is the canonical persisted ISO 3166-1 alpha-2 User country:

- `IR` routes only to Mailerino;
- every supported non-`IR` code routes only to Resend;
- lowercase input is normalized before persistence or routing;
- missing, malformed, or unsupported country fails closed;
- language, email domain, IP address, browser locale, and inferred geography are not routing inputs.

Mailerino and Resend adapters use bounded standard-library HTTP clients, context cancellation, HTTPS production endpoints, sanitized errors, bounded response consumption, and no automatic retry. The implementation contract was checked on 2026-07-28 against [Mailerino API documentation](https://mailerino.com/docs/) and [Resend Send Email documentation](https://resend.com/docs/api-reference/emails/send-email). Tests use local fake HTTP servers and never contact a provider.

## SMS behavior

`SMS_PROVIDER=kavenegar` is the only production mode. If `SMS_ENABLED=true`, production requires a non-placeholder `KAVENEGAR_API_KEY` and `SMS_TEMPLATE`. Missing configuration, Redis failure, circuit-breaker rejection, provider rejection, and initialization failure never select a mock, logging, console, no-op, email, or alternate SMS fallback.

`FakeProvider` is an in-memory, non-logging test double. Runtime construction does not select it, and production validation rejects fake/mock/logging/no-op or unsupported provider modes.

## Configuration

| Setting | Production requirement |
|---|---|
| `ENVIRONMENT` / `APP_ENV` | If both exist they must resolve identically. Missing values select fail-safe production validation; supported values are `development`, `local`, `test`, `staging`, and `production`. |
| `SECURITY_CODE_HASH_SECRET[_FILE]` | Required, independent, non-placeholder, and sufficiently unpredictable. |
| `MAILERINO_API_KEY[_FILE]` | Required for the `IR` route. |
| `MAILERINO_FROM_EMAIL` | Required provider-approved sender. |
| `MAILERINO_BASE_URL` | HTTPS; canonical default is `https://api.mailerino.com`. |
| `RESEND_API_KEY[_FILE]` | Required for supported non-`IR` routes. |
| `RESEND_FROM_EMAIL` | Required provider-approved sender; demonstration defaults are rejected. |
| `RESEND_BASE_URL` | HTTPS; canonical default is `https://api.resend.com`. |
| `SMS_ENABLED` | When true, activates only KaveNegar-backed endpoints. |
| `SMS_PROVIDER` | Must be `kavenegar` in production. |
| `KAVENEGAR_API_KEY[_FILE]` | Required when SMS is enabled. |
| `SMS_TEMPLATE` | Required when SMS is enabled. |

Development/test may use local-only HMAC material and explicit in-process fakes. Those values and provider modes are rejected by production validation. Fakes never write message bodies or codes to logs.

## Logging and client errors

Codes, reset-session handles, password-set handles, provider credentials, authorization headers, and provider bodies must not enter application logs, access logs, traces, metrics labels, panic text, or permanent reports. Structured diagnostics may include a User identifier, channel, masked destination, and generic acceptance/failure category. Client failures remain stable and generic and do not disclose account existence or provider details.

## Deployment and rollback

Deploy in this order:

1. provision independent security-code HMAC, Mailerino, Resend, and—when enabled—KaveNegar secrets;
2. provision both provider-approved email senders and the KaveNegar template;
3. mount secret files and configure HTTPS endpoints;
4. run production configuration tests and Compose rendering;
5. deploy backend lifecycle/provider changes;
6. deploy the registration client that includes the country in the initial request;
7. monitor provider acceptance/failure counters and generic errors without payload logging.

A rollback may disable an unavailable feature while keeping endpoints fail closed. It must not restore plaintext/unkeyed code storage, demonstration senders, asynchronous success acknowledgement, shared JWT secrets, URL session tokens, cross-provider fallback, or mock/logging providers in production. Active codes issued by this lifecycle remain HMAC-bound and cannot be validated by the legacy unkeyed path.

## Remaining security roadmap

SEC-004 owns sensitive-action password reauthentication and privileged-action enforcement only; it must not implement Super Admin login MFA. SEC-005 owns generalized repository-wide secret/log redaction, and SEC-006 owns the broader CORS, CSRF, edge-security, rate-limit, and abuse-control program. Planned SEC-007 owns Super Admin MFA and is required before paid-production approval. This document does not start those tasks or authorize paid production.
