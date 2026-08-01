# User and Admin Authentication Isolation

Status: Implemented by `SEC-001` for the current merged API runtime, the
User-facing BFF/adapters it constructs, and the Admin-protected Settlement and
Shard Router routes.

This document follows the
[fixed product and technical policies](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md),
[ADR 0001](../adr/0001-target-runtime-architecture.md), and the
[canonical glossary](../product/canonical-domain-glossary-and-version-catalog.md).
Authentication remains part of the Platform Modular Monolith; this change does
not introduce another bounded backend system.

## Trust boundaries

The User and Admin authentication contexts are separate cryptographic trust
domains. The merged `api-server` validates both configurations before opening
runtime resources, constructs two `auth.Auth` values, injects the User value
into `user-bff` and `payment-service`, and injects the Admin value into
`admin-bff`. `trade-bff` also constructs the explicit User context because it
validates User access tokens. Settlement and Shard Router validate the complete
pair at startup and construct only the explicit Admin context for their
Admin-protected routes. Each injectable consumer rejects a supplied `auth.Auth`
with the wrong context.

| Property | User context | Admin context |
| --- | --- | --- |
| Access signing key | `JWT_SECRET_USER` or `JWT_SECRET_USER_FILE` | `JWT_SECRET_ADMIN` or `JWT_SECRET_ADMIN_FILE` |
| Refresh signing key | `JWT_REFRESH_SECRET_USER` or `JWT_REFRESH_SECRET_USER_FILE` | `JWT_REFRESH_SECRET_ADMIN` or `JWT_REFRESH_SECRET_ADMIN_FILE` |
| Issuer | `JWT_ISSUER_USER` (`tragge-user-auth`) | `JWT_ISSUER_ADMIN` (`tragge-admin-auth`) |
| Audience | `JWT_AUDIENCE_USER` (`user`) | `JWT_AUDIENCE_ADMIN` (`admin`) |
| Context claim | `user` | `admin` |
| Session namespace | `session:user:` | `session:admin:` |
| Revocation namespace | `jwt_blacklist:user:` | `jwt_blacklist:admin:` |
| Refresh cookie | `refresh_token_user`, path `/api/user/auth` | `refresh_token_admin`, path `/api/admin/auth` |
| Session hint cookie | `tragge_session_hint_user` | `tragge_session_hint_admin` |
| CSRF context | `csrf:user` and `USER_FRONTEND_ORIGIN` | `csrf:admin` and `ADMIN_FRONTEND_ORIGIN` |

There is no active legacy dual-acceptance or compatibility flag. Existing
sessions and tokens from the shared trust domain are intentionally not accepted
by the new explicit contexts.

## Token validation

User and Admin validators independently require:

- an HS256 signature made with the context and token-purpose key;
- the exact configured issuer and a single exact audience;
- a matching `auth_context` claim;
- a recognized `access` or `refresh` purpose at the matching endpoint;
- valid expiration and not-before times; and
- the required subject/User ID claims.

Role claims are evaluated only after cryptographic validation. Changing a role
claim without a valid signature cannot cross the boundary. Cross-context
failures return the existing generic unauthorized response; validation errors
and configuration checks never include secret values.

Authorization role storage remains unchanged by SEC-001. The approved product
roles are `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN`; no Finance role is
introduced here.

## Refresh and session behavior

Access and refresh keys are distinct within a context and across contexts.
Refresh validation checks signature, issuer, audience, context, purpose, and
session binding before reading or mutating Redis. A cross-context refresh
therefore cannot trigger reuse detection or revoke the valid session in the
other context.

User and Admin sessions can coexist for the same subject because session,
rotation, revocation, login-state, OAuth-state, and password-reset keys carry
explicit context namespaces. Redis remains
a session/revocation mechanism, not a replacement identity source of truth.
Logout clears only the current context's cookies and session.

## Cookies and CSRF

Refresh cookies are host-only because no `Domain` is set. Their paths are
narrowed to their matching authentication routes. They are `HttpOnly`; the
non-secret session hint sibling is intentionally readable by the matching SPA.
Production always emits `Secure` cookies with `SameSite=None`. Explicit local
and test environments use `SameSite=Lax` without `Secure` on HTTP and use secure
settings when the request is HTTPS.

The existing CSRF mechanism is origin plus `X-Requested-With`, not a signed
CSRF-token system. SEC-001 gives User and Admin separate middleware instances,
context identifiers, and exact frontend origins. A User-origin request cannot
satisfy Admin CSRF validation and vice versa, and context-specific middleware
fails closed when Origin/Referer is absent. A broader CSRF-token program remains
owned by `SEC-006`.

## Production startup requirements

Production (including an empty or unknown environment name) refuses startup
when any required secret, issuer, audience, context, cookie name, namespace, or
frontend origin is missing or collides. All four signing keys must be pairwise
distinct. Each key must be at least 32 bytes, reject known placeholder/default
markers, and pass conservative variety and entropy checks so a long repeated
string is not accepted as strong.

Generate every signing key independently from at least 32 random bytes. The
local helper uses 48 random bytes per value:

```bash
./scripts/secrets/init-secrets.sh
```

The generated files are local-only and ignored. Do not paste production values
into environment examples, logs, reports, tickets, or source files. Local and
test invocations still require explicit access and refresh values; only the
canonical issuer and audience metadata may default in an explicitly local,
development, or test environment.

## Session credentials in URLs

SEC-002 removed reusable User and Admin session JWT authentication from URL
query parameters. Trading browsers now use a short-lived, single-use,
User/session/Contest/purpose-bound ticket plus a narrow HttpOnly binding cookie;
protected attachments use authenticated Blob fetches. See the
[session authentication URL policy](session-authentication-url-policy.md).
These additions preserve the SEC-001 trust-domain separation and do not broaden
refresh-cookie paths.

## Remaining security roadmap

SEC-003 owns OTP/reset-delivery hardening. SEC-004 owns sensitive-action
password reauthentication and privileged-action enforcement only; it must not
implement Super Admin login MFA. SEC-005 owns generalized secret/log redaction,
and SEC-006 owns broader edge security, CORS, rate limiting, abuse controls,
and the broader CSRF program. Planned SEC-007 owns Super Admin MFA and remains
required before paid-production approval.

Paid-production status remains `NO-GO`.