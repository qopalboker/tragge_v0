# Super Admin MFA security contract

**Contract:** `super_admin_totp_v1`

**Owner:** Platform Admin identity boundary

**Implemented by:** `SEC-007`
**Paid-production status:** `NO-GO`

This contract applies only to the canonical `SUPER_ADMIN` role inside the
isolated Admin authentication trust domain established by `SEC-001`. It does
not modify User authentication and does not grant `SUPPORT_ADMIN` any elevated
permission.

## Login and session assurance

A valid Super Admin password establishes only a short-lived pre-session
challenge. It never creates an access token, refresh token, or Redis session.
The server issues those artifacts only after a valid Admin-only TOTP or unused
recovery code and stamps both the signed tokens and server-side session with
`super_admin_totp_v1`. Admin middleware rejects a Super Admin session without
that exact assurance. Refresh rotation preserves and cross-checks the assurance
against the session; a client cannot request or manufacture it.

Support Admin continues to authenticate with the isolated Admin password flow
and its explicit support/KYC permissions. It cannot obtain the Super Admin MFA
assurance or use it to gain privileges.

## Enrollment and stored state

The first successful Super Admin password step returns an opaque enrollment
challenge. Enrollment produces an RFC 6238 SHA-1, six-digit, 30-second TOTP
secret compatible with Google Authenticator and equivalent applications. The
setup secret and `otpauth://` provisioning URI are returned only during that
pending enrollment. The credential becomes active only after a valid code.

The database stores the secret only as an `enc:admin-mfa:v1:` AES-256-GCM
envelope with authenticated context. Plaintext and legacy/shared TOTP fallback
are forbidden. The Admin-only tables introduced by migration
[`0100_admin_super_mfa.up.sql`](../../packages/db/migrations/0100_admin_super_mfa.up.sql)
are distinct from the legacy User columns introduced by migration `0050`.

The highest accepted TOTP counter is updated with one conditional SQL mutation.
This makes the accepted clock window explicit while preventing the same code
from succeeding twice, including under concurrent requests. The allowed clock
window is the current 30-second counter plus or minus one; codes outside it fail
closed.

## Challenges and recovery

Pre-session challenges are 256-bit opaque random values. Redis keys contain
only SHA-256 digests, bind the challenge to the Admin actor, authoritative role
and permission state, purpose, client IP/User-Agent digest, and a maximum
five-minute lifetime, and use atomic single-use consumption. They never appear
in URLs, logs, browser storage, analytics, or traces.

Enrollment generates ten independent recovery codes. Only keyed HMAC-SHA-256
digests using `ADMIN_MFA_RECOVERY_PEPPER` are stored. Plaintext codes are shown
once after enrollment. Each code is consumed by a conditional database update,
cannot be replayed, and cannot be exchanged for a User credential.

MFA reset is an audited destructive security action. It requires an
MFA-authenticated acting Super Admin, the `users.edit` permission, and the
short-lived actor/session/action/resource-bound password reauthentication grant
from `SEC-004`, plus a reason. Credential deletion and audit insertion share one
database transaction. Audit failure rolls the deletion back. A successful reset
revokes the target's Admin sessions, pending MFA challenges, and sensitive-
action reauthentication grants; the target must enroll again after the next
valid password login.

## Configuration and operations

The Admin runtime requires independently generated 32-byte hexadecimal values:

- `ADMIN_MFA_ENCRYPTION_KEY` (or `_FILE`) for AES-256-GCM;
- `ADMIN_MFA_RECOVERY_PEPPER` (or `_FILE`) for recovery-code HMAC; and
- `ADMIN_MFA_ISSUER`, whose approved production value is `Tragge Admin`.

Missing, malformed, repeated-byte, placeholder, equal-domain, or unapproved
production configuration fails startup without logging secret material. Local
secret generation is provided by `scripts/secrets/init-secrets.sh`; generated
files remain ignored and must never be committed.

Audit and diagnostic events contain only actor/resource IDs, safe result or
failure categories, action, assurance identifier, request metadata, and approved
reset reason. They never contain passwords, TOTP or recovery codes, setup
secrets, challenge values, JWTs, refresh tokens, cookies, request bodies, or
encryption keys. Existing `SEC-005` redaction and `SEC-006` edge limits remain
mandatory defense in depth.

The implementation and passing tests satisfy the SEC-007 task boundary, but do
not by themselves approve paid production. Every remaining launch gate,
operational readiness check, legal/provider approval, reconciliation control,
and the Phase 1 exit gate remain independently required.
