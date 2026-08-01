# Admin authentication pipeline not wired to backend

**Severity:** HIGH
**Status:** Open — deferred out of the current branch, requires its own PR and security review.
**Discovered:** 2026-04-19 during unrelated gateway/CSP/assets fixes on `claude/zen-brown-cQ0Dn`.

## Summary

The frontend has a dedicated admin login UI (`/admin/login` route, standalone
`LoginPage.vue` with admin branding and 2FA-ready query handling), and the
backend has a dedicated admin auth stack (`POST /api/admin/auth/login` on
admin-bff, port 8083, with a stricter nginx rate-limit zone, an IP whitelist,
a per-IP failed-login tracker, and a TOTP/2FA branch).

**The two halves are not connected.** The admin login UI submits through the
shared `authStore.login()` method, which calls the **user** endpoint
`POST /api/user/auth/login` on user-bff (port 8081). Every backend admin
protection therefore bypasses.

## Discovery findings (line-accurate)

### Gap 1 — router guard hard-codes user login

`apps/frontend/src/router/index.ts:42`

```ts
if (requiresAuth && !auth.isAuthenticated) {
  return { path: '/user/login', query: { redirect: to.fullPath } }
}
```

Single guard for all three modules (user / trade / admin). Unauthenticated
access to any `/admin/**` path redirects to `/user/login`, not `/admin/login`.

### Gap 2 — admin login form submits through the shared user method

`apps/frontend/src/modules/admin/views/LoginPage.vue:60`

```ts
const success = await authStore.login(email.value, password.value);
```

`apps/frontend/src/shared/stores/auth.ts:306`

```ts
const response = await api.post<LoginResponse>('/api/user/auth/login', { ... })
```

No frontend file references `/api/admin/auth/login`:

```
$ grep -rn "/api/admin/auth" apps/frontend/src/
(no matches)
```

Admin token refresh has the same issue:
`apps/frontend/src/modules/admin/api/index.ts:39` calls
`/api/user/auth/refresh`, not an admin-specific refresh.

### Gap 3 — role failure does not surface the admin error page

`apps/frontend/src/router/index.ts:50`

```ts
if (requiresRole && typeof requiresRole === 'string') {
  if (!auth.hasRole(requiresRole)) {
    return '/'
  }
}
```

`LoginPage.vue:45` already handles `?error=admin_required`, but the guard
never routes there — it sends the user home, so that UX branch is dead code
reachable only via manual URL entry.

## Impact

Every backend-side admin protection is bypassable in the normal login flow:

| Protection | Location | Bypassed because |
|------------|----------|------------------|
| Stricter rate limit (`admin_auth_limit` 3 r/m, burst 3) | `apps/gateway/nginx.conf:64, 722` | Traffic hits `auth_limit` (10 r/m, burst 5) on `/api/user/auth/login` |
| IP whitelist (`AdminIPWhitelist`) | `apps/admin-bff/server/app.go:72`, enforced at `handlers_helpers.go:1795` | Never checked by user-bff |
| Per-IP failed-login tracker (account lockout with `Retry-After`) | `app.go:100`, `handlers_helpers.go:1805, 1847, 1884` | Not invoked by user-bff login path |
| TOTP / 2FA challenge for admin role holders | `handlers_helpers.go:1942` | User login does not gate on `totp_enabled` the same way |
| Admin-scoped audit events (`admin.login.success/failed/blocked`) | `handlers_helpers.go:1798, 1851, 1889, 1926, 1996` | Events logged only as generic user auth events |
| System-account block for admin login | `handlers_helpers.go:1872` | Not enforced on user endpoint |

The user endpoint still verifies the password with Argon2id, still issues
JWTs with the correct role claims, and `RequireAdminAccess` middleware on
admin-bff routes still gates API calls by role. So the admin **API surface**
is not open — an authenticated non-admin cannot call admin endpoints. But
the **authentication step itself** runs under user-grade protections, which
is the layer that matters for brute-force, credential stuffing, and
IP-restriction policies.

## Current mitigations (defense in depth, not a substitute)

- Default password policy is `MinLength: 10` + uppercase + lowercase + digit
  + special (`packages/validation/validation.go:419`). This raises the cost
  of online guessing, but does not replace rate-limiting or IP restriction
  against a determined attacker with a leaked or credential-stuffed
  password.
- Admin role assignment is gated: only `super_admin` can grant `admin` or
  `super_admin` (`apps/admin-bff/server/handlers_user_management.go:484,
  1677`).
- `RequireAdminAccess` middleware on all admin-bff routes still enforces
  role (`viewer`/`admin`/`super_admin`) — the admin API surface is closed
  to non-admins regardless of where they logged in.

## Proposed fix outline (no implementation in this branch)

Intentionally high-level — the actual patch should be designed in its own
branch after threat-modeling review.

1. **Split the auth store.** Add `authStore.adminLogin(email, password)`
   that posts to `/api/admin/auth/login` and handles the `requires_2fa`
   response shape. Do not add an `isAdmin` flag to the existing user
   `login()` — separate call sites are clearer and harder to mis-wire.

2. **Separate admin API client for auth.** New
   `apps/frontend/src/modules/admin/api/auth.ts` wrapping the admin auth
   endpoints (`/login`, `/2fa/login`, `/refresh` if a separate admin
   refresh is added). Admin module should not import from the user auth
   store for login/refresh.

3. **Router guard must branch on module.** Replace the single
   `/user/login` redirect with logic that inspects
   `to.matched.some(r => r.path.startsWith('/admin'))` (or an explicit
   `meta.authRealm: 'admin' | 'user' | 'trade'`) and redirects to the
   matching login page. Role-check failures on `/admin/**` should go to
   `/admin/error?error=admin_required` so the existing UX branch in
   `LoginPage.vue:45` actually fires.

4. **Verify backend reads client IP correctly behind gateway.** The admin
   IP whitelist and failed-login tracker use `getAdminClientIP(r)`
   (`apps/admin-bff/server/handlers_helpers.go:1791`). Confirm it resolves
   through `X-Forwarded-For` / `X-Real-IP` as set by the gateway
   (`apps/gateway/nginx.conf:589–590`) rather than using
   `r.RemoteAddr` (which would always be the gateway pod IP inside the
   Docker network — whitelist pointless, lockout global).

5. **Consider a separate admin refresh endpoint.** Currently even a
   correct `/api/admin/auth/login` would fall back to
   `/api/user/auth/refresh` on token expiry, meaning re-auth skips all
   admin protections. Either add `/api/admin/auth/refresh` with the same
   IP whitelist + failed-attempt tracking, or document explicitly why
   refresh under user-grade protections is acceptable.

6. **E2E coverage.** Playwright spec covering:
   - unauthenticated `/admin/dashboard` → redirects to `/admin/login` (not
     `/user/login`)
   - login as non-admin user → `/admin/error?error=admin_required`
   - login as admin with 2FA disabled → dashboard
   - login as admin with 2FA enabled → TOTP challenge → dashboard
   - 2FA ticket expiry / rate-limit (admin-bff issues `2fa_admin_pending`
     key, max 10 pending per user per 15 min)
   - IP whitelist denial (if configured in test env)

## Out-of-scope note

This issue was discovered during an unrelated branch that fixes gateway
CSP, asset proxying, and the login 405/403 chain. Wiring admin auth
correctly touches:

- the shared `authStore` (high blast radius — every module depends on it)
- token storage / refresh semantics (potentially different for admin)
- the global router guard (affects user / trade flows too)
- backend middleware for `X-Forwarded-For` handling if currently incorrect
- E2E test fixtures (admin login not covered today)

It needs its own branch, its own PR, explicit security review before
merge, and should not be bundled with frontend/infra fixes that have
already been verified independently. Fixing any subset in isolation
(e.g. Gap 1 alone) **makes the situation worse** — the admin login form
would submit and succeed, but through the user endpoint with user-grade
protections, hiding the fact that the hardened admin path remains
unused.

## References

- Admin login backend: `apps/admin-bff/server/handlers_helpers.go:1788`
  (`handleAdminLogin`), route `apps/admin-bff/server/app.go:525`.
- Admin rate-limit zone: `apps/gateway/nginx.conf:64, 721`.
- User rate-limit zone for comparison: `apps/gateway/nginx.conf:61, 440` etc.
- Admin UI LoginPage: `apps/frontend/src/modules/admin/views/LoginPage.vue`.
- Router guard: `apps/frontend/src/router/index.ts:31`.
- Shared auth store: `apps/frontend/src/shared/stores/auth.ts:300`.
- Admin API refresh path: `apps/frontend/src/modules/admin/api/index.ts:36`.
