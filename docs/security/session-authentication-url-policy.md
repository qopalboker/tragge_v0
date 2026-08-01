# Session Authentication URL Policy

Status: Implemented for SEC-002 on 2026-07-26

## Authority and scope

This policy implements SEC-002 under the [fixed product and technical policies](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md), preserves the User/Admin trust boundaries defined by [ADR 0001](../adr/0001-target-runtime-architecture.md) and [the SEC-001 authentication-isolation design](user-admin-authentication-isolation.md), and does not implement SEC-003 through SEC-006. The roadmap remains authoritative for later security work in [PRODUCTION_ROADMAP_AND_CODEX_TASKS.md](../codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md).

Reusable User or Admin access and refresh JWTs are prohibited in URLs. This includes normal HTTP URLs, WebSocket URLs, redirects, protected-download URLs, logging fields, analytics, and telemetry.

## Supported normal HTTP authentication

Protected HTTP routes accept a context-specific `Authorization: Bearer` access token. Refresh tokens remain in the separate User and Admin secure HttpOnly cookies established by SEC-001. Query fields named `token`, `access_token`, `jwt`, `auth_token`, or `session_token` are rejected before token validation, even when a valid Authorization header is also present. The response is a generic HTTP 401 with the non-sensitive code `url_authentication_unsupported`; it does not echo or validate the supplied URL credential.

The enforcement is centralized in [packages/auth/middleware.go](../../packages/auth/middleware.go). User and Admin keys, issuers, audiences, contexts, sessions, revocation namespaces, refresh paths, cookies, and CSRF contexts remain distinct.

There is no compatibility flag that restores reusable session JWT query authentication.

## Trading WebSocket authentication

The browser trading flow uses a bounded ticket rather than a reusable session JWT:

1. The authenticated User client sends `POST /api/trade/ws-ticket` with its normal Authorization header and the target Contest ID in JSON.
2. The server revalidates the User context and active namespaced session.
3. The server creates a 256-bit opaque ticket and independent 256-bit binding value with a 10-second lifetime.
4. Redis stores only the SHA-256 ticket key and binding digest, plus the User ID, session ID, User authentication context, purpose, Contest ID, and expiration. Raw ticket and binding values are not persisted.
5. The response returns only the bounded ticket with `Cache-Control: no-store`. The independent binding is delivered in the `tragge_ws_ticket_bind_user` HttpOnly, SameSite=Strict cookie scoped to `/ws/trade`; production always sets Secure.
6. The browser connects with the bounded `ticket` query field. Gateway logs redact the complete WebSocket query string.
7. The server atomically consumes the ticket, compares the binding in constant time, revalidates context, purpose, Contest, expiration, User, and active session, and clears the binding cookie.
8. Replay, expiration, wrong binding, wrong Contest, wrong purpose, wrong authentication context, wrong User session, or revoked session fails closed.

Non-browser clients may authenticate the WebSocket handshake with the context-specific User Authorization header. That path also requires an active User session. Admin tokens fail under the User validator before Contest authorization. The public tournament feed remains unauthenticated and does not accept session query credentials.

The service and lifecycle are implemented in [apps/trade-bff/server/ws_ticket.go](../../apps/trade-bff/server/ws_ticket.go). The browser URL builder in [webSocketUrl.js](../../apps/user-frontend/src/modules/trade/composables/webSocketUrl.js) has no session-token input and rejects credential-like fields already present in a candidate URL. Every reconnect obtains a new bounded ticket.

## Protected downloads

User and Admin support-ticket attachments use authenticated XHR/fetch through their existing context-specific API clients and convert successful responses into browser Blob URLs. Direct links do not contain reusable session credentials. The server rechecks attachment ownership or Admin authorization and responds with `Cache-Control: private, no-store, max-age=0`, `X-Content-Type-Options: nosniff`, a restrictive Content Security Policy, and a sanitized filename.

Relevant implementations are:

- [User attachment client](../../apps/user-frontend/src/modules/user/views/TicketChatPage.vue)
- [Admin attachment client](../../apps/admin-frontend/src/modules/admin/views/TicketDetailPage.vue)
- [User attachment server](../../apps/user-bff/server/ticket_handlers.go)
- [Admin attachment server](../../apps/admin-bff/server/handlers_tickets.go)

No general download-ticket mechanism was added because the affected current flows can use authenticated Blob fetches.

## Logging, tracing, analytics, and error reporting

Before application observability and panic telemetry run, focused middleware removes Authorization and Cookie headers and replaces reusable credential-query values with `[REDACTED]`; it restores only the secure headers for downstream authentication. The trading service also moves the bounded WebSocket ticket into an unexported request context and redacts its URL value before telemetry. Application observability records normalized paths, not raw query strings. Gateway configurations use a sanitized request URI, suppress all WebSocket query strings, and omit referrers. The Kubernetes gateway logs method, normalized URI path, and protocol instead of the raw request line.

The focused protections are in:

- [development gateway configuration](../../apps/gateway/nginx.conf)
- [production gateway configuration](../../apps/gateway/nginx.prod.conf)
- [Kubernetes gateway configuration](../../infra/k8s/base/gateway.yaml)
- [HTTP observability middleware](../../packages/observability/middleware.go)

This is the focused SEC-002 protection, not the generalized SEC-005 redaction program. Tests and reports must use fixture labels and must not print complete credentials.

## Client migration and failure behavior

Frontend and backend SEC-002 changes should be released together. A ticket-exchange failure no longer falls back to a reusable JWT URL; the client stops the handshake, reports a generic local failure, and follows its bounded reconnect policy. Legacy clients that send a session credential only in a URL receive a generic unauthorized response. A URL credential is never echoed.

## Remaining security work

SEC-003 owns OTP/reset-delivery hardening. SEC-004 owns sensitive-action password reauthentication and privileged-action enforcement only. SEC-005 owns generalized secret/log redaction, and SEC-006 owns broader edge security, CORS, rate limiting, and abuse controls. Planned SEC-007 owns Super Admin MFA and is required before paid-production approval. Paid-production status remains `NO-GO` until every roadmap launch gate passes.