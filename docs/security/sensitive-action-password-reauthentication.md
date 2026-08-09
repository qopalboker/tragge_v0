# Sensitive-action password reauthentication

**Status:** SEC-004 implemented locally; paid-production status remains `NO-GO`

**Security contract:** no public version assigned; current local implementation recorded by `SEC-004`

This control follows the [fixed product and technical policies](../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md), the [accepted runtime ADR](../adr/0001-target-runtime-architecture.md), and the [SEC-004 roadmap task](../codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md). It preserves the separate Admin cryptographic/session trust domain established by SEC-001.

## Current login and MFA status

Current Admin login requires the Admin password and the isolated Admin token,
audience, cookie, session, revocation, and CSRF controls. `SEC-007` additionally
requires the Admin-only `super_admin_totp_v1` assurance before a Super Admin
session is issued. The retired shared-user TOTP verifier is unregistered and
fails closed; it is not a login path.

Password reauthentication remains an independent sensitive-action control and
is not replaced by login MFA. Completing both controls is still not, by itself,
evidence of paid-production readiness.

## Canonical authorization

The only canonical roles are `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN`.
Deprecated or unknown elevated roles fail closed at Admin login and middleware.
There is no Finance role.

Support Admin receives only explicit KYC permissions plus the existing support-
ticket boundary. Only Super Admin with the action's explicit permission can
obtain or consume a sensitive-action grant. Migration
[`0099_admin_canonical_roles.up.sql`](../../packages/db/migrations/0099_admin_canonical_roles.up.sql)
adds the transitional legacy `support_admin` role, limits it to KYC permissions,
and converts legacy `admin` assignments. Its development-only down counterpart
is [`0099_admin_canonical_roles.down.sql`](../../packages/db/migrations/0099_admin_canonical_roles.down.sql).
The bridge is classified for folding into the later Platform-owned auth baseline;
it is not a new target-architecture boundary.

## Protected active actions

| Action | Route | Permission | Resource binding | Required reason/reference | Mandatory mutation audit |
|---|---|---|---|---|---|
| Withdrawal completion | `POST /api/admin/withdrawals/{id}/complete` | `withdrawals.manage` | exact Withdrawal ID | comment/reason and transaction reference | `withdrawal.completed`, in the payout transaction |
| Wallet adjustment | `POST /api/admin/users/{user_id}/wallet/charge` | `users.wallet.charge` | exact User ID | reason; debit confirmation when negative | `user.wallet.charged`, in the ledger transaction |
| Elevated role change | `PATCH` or `PUT /api/admin/users/{user_id}/roles` | `users.edit` | exact target User ID | reason | `user.roles.updated`, in the role transaction |
| Elevated account creation | `POST /api/admin/users` when assigning Support/Super Admin | `users.edit` | normalized target email | reason | `user.created_by_admin`, in the creation transaction |

Withdrawal rejection currently releases/refunds funds; an active rejected-
withdrawal deduction does not exist and SEC-004 did not create it. There is no
active permission-management endpoint. Non-elevated user creation is not a
privileged-role mutation and does not consume a grant.

## Grant lifecycle

`POST /api/admin/reauthenticate` accepts the current Admin password, a registered
action, and exact resource identifier. Cookie-authenticated production requests
remain protected by the existing Admin CSRF middleware. The endpoint uses the
existing Admin authentication rate limiter.

After loading the current authoritative password hash, roles, permissions, and
session, the server verifies the password with the existing Argon2id verifier and
issues 32 random bytes encoded as an opaque grant. The raw grant is returned once
and is never written to PostgreSQL. Redis stores only a SHA-256-keyed record under
`reauth:admin:` for at most five minutes. The record contains a digest of the
session ID and a digest fingerprint of current password/role/permission state,
not those raw values.

Consumption is atomic. The grant is bound to Admin context, actor, active Admin
session, action, exact resource, and current security fingerprint. A first
presentation deletes the live record and leaves a short replay marker. Expiry,
replay, wrong actor/session/action/resource, password change, role change,
permission change, logout, session revocation, malformed input, or Redis failure
fails closed. Role changes also physically revoke the target actor's outstanding
grants.

The grant is accepted only in `X-Admin-Reauth-Grant`. It is never accepted in a
URL, request body mutation payload, cookie, local storage, or session storage,
and it cannot be exchanged for session credentials.

## Audit and transactional behavior

Safe audit evidence covers password verification success/failure, issuance,
consumption, expiry/replay, wrong-session/action/resource, security-state/storage categories, role/permission and mandatory-reason denials,
and the final protected action. Audit payloads can contain actor, action,
resource, permission, timestamp, safe failure category, and approved reason.
They never contain passwords, hashes, grants, complete session IDs, JWTs,
refresh tokens, cookies, authorization headers, or request bodies.

A grant is unusable unless issuance audit succeeds. Consumption audit is required
before a handler runs. Each financial or role mutation writes its final action
audit in the same PostgreSQL transaction; an audit insertion failure rolls back
the mutation.

## Frontend flow

The Admin frontend asks for the current password immediately before the selected
operation, sends it only to `/api/admin/reauthenticate`, keeps the returned grant
in a function-local value, immediately supplies it in the dedicated header, and
discards it on completion or failure. It does not persist or log either value and
does not persist or transport the separate SEC-007 MFA material.

A deployment must update the backend before or together with the Admin frontend
so protected operations do not temporarily call a grant-enforcing endpoint from
an older client.

## Remaining security work

- SEC-005 owns generalized log/credential redaction beyond these focused paths.
- SEC-006 owns broader edge-security and abuse controls.
- SEC-007 implements Super Admin TOTP enrollment, login challenge, recovery,
  replay prevention, audited session upgrade, database/concurrency validation,
  and production configuration.
- Later Platform data/architecture tasks replace the shared-public legacy role
  migration with the final owner-isolated authentication schema.

No production deployment or paid-production approval is provided by SEC-004.
