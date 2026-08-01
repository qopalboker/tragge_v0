# PR Summary (draft)

> Working draft. Use as PR description after Docker verification passes.
> Delete this file before merging — it's a session artifact, not project docs.

## What this branch does

Fixes a chain of symptoms reported on the login screen (CSP block, 405 on
auth endpoints, 403 on token refresh, possibly broken Google sign-in) plus
related infrastructure issues. All changes are static — none have been
verified at runtime because Docker was unavailable in the session.

See `docs/VERIFICATION_CHECKLIST.md` for the runtime verification steps.

## Commits (4)

| SHA | Subject |
|-----|---------|
| `be52b29` | fix(frontend,gateway): allow arcaptcha in default CSP and send X-Requested-With on refresh |
| `c48f2f4` | fix(compose): make frontend container internal-only and route default URLs through gateway |
| `729a5a8` | fix(secrets): correct GOOGLE_REDIRECT_URI fallback in unused helper |
| `fa90d89` | docs: add post-docker verification checklist for branch changes |

### `be52b29` — gateway CSP + token-refresh CSRF header
- `apps/gateway/nginx.conf` and `apps/gateway/nginx.prod.conf`: default CSP fallback now whitelists `https://widget.arcaptcha.co` and `https://api.arcaptcha.co`. Vite-built JS chunks are served from `/assets/...` which doesn't match any of the `~^/user`, `~^/trade`, `~^/admin` map entries, so they fell through to the `default` CSP — which forbade arcaptcha. Result was that captcha widget couldn't be dynamically imported.
- `apps/frontend/src/modules/user/api/index.ts:46`: token-refresh call now sends `X-Requested-With: XMLHttpRequest` so `packages/validation/csrf.go` accepts it. Still uses bare `axios.post` (not the `api` instance) to avoid 401-refresh recursion.

### `c48f2f4` — frontend container internal-only
- `infra/docker/docker-compose.yml:584`: `ports: ["5173:8080"]` → `expose: ["8080"]`. The frontend container's nginx has no `/api/*` proxy, so any `POST /api/user/auth/login` hitting it directly fell through SPA fallback to a static `/index.html` and returned **405**. Removing the published port forces all traffic through the gateway, where the route exists.
- `infra/docker/docker-compose.yml:14`: added `:8080` (localhost / 127.0.0.1) to the default `ALLOWED_ORIGINS` so BFF CORS accepts requests originating from the gateway page.
- `infra/docker/docker-compose.yml:306-307`: `SUCCESS_REDIRECT_URL` / `CANCEL_REDIRECT_URL` defaults moved from `:5173` (now unreachable) to `:8080` (gateway). These are user-facing post-payment redirects (browser navigation), not webhooks — webhooks remain `WEBHOOK_BASE_URL` → `:8091` internal.
- `dev-env.sh:32-33`: same `:8080` addition to `ALLOWED_ORIGINS` and `CORS_ALLOWED_ORIGINS`, including the Codespaces hostname variant.

### `729a5a8` — dead-code OAuth fallback
- `packages/secrets/secrets.go:234`: `GetGoogleOAuthConfig` had a fallback of `:8080/api/user/auth/google/callback` (backend API path), but Google must redirect to the SPA route `/user/auth/google/callback`. The function is currently dead code (callers read `GOOGLE_REDIRECT_URI` directly via `os.Getenv` in `apps/user-bff/server/app.go:1295`), so this is preventive: if anyone wires it up later, it would silently break Google sign-in.

### `fa90d89` — verification checklist
- `docs/VERIFICATION_CHECKLIST.md` (new): post-Docker curl checks, browser smoke checklist, deferred-bug notes (SVG `##`, vite proxy concern, `E2E_USER_URL` doc typo), localization plan for the SVG bug.

## Files NOT changed (intentional)

- `.env` and `.env.example` — out of scope; values there were already correctly aligned.
- `apps/frontend/vite.config.ts` — proxy targets `localhost:8081/8082/8083` directly. Fine for `pnpm dev` on host, separate concern from container setup.
- `.devcontainer/devcontainer.json` — port-forward for 5173 still useful for `pnpm dev`; harmless when frontend container has no 5173 mapping.
- `docs/E2E_TESTING.md`, `docs/DEVELOPMENT.md`, `CLAUDE.md` — references to `:5173` describe the `pnpm dev` host workflow, still valid.
- `scripts/security/test-security-headers.sh` — tests CORS with `:5173` Origin (pnpm dev workflow), still valid.

## Open issues / risks

1. **Google OAuth not verified.** The original "Google doesn't work" report cannot be distinguished from a downstream symptom of the CSP block (login form half-loads → button never wires up). Architectural review found no code-path mismatches, but that does not prove the runtime works. Test cold per the checklist; if it still fails, file a separate issue.
2. **SVG `##` icon bug deferred.** Could not be localized via static analysis — neither `##` nor template literals near `<svg>`/`<path>` appear in source. Most likely origin: a vendor SVG inside `node_modules` or runtime-computed path with bad data. Localization plan (DevTools breakpoint on `setAttribute`) is in the checklist. Cosmetic only — not a release blocker.
3. **Vite dev proxy bypasses gateway.** `apps/frontend/vite.config.ts:23-37` proxies directly to BFF host ports. If a developer runs `pnpm dev` while BFFs are containerized without host port mappings, the proxy targets are unreachable. Out of scope for this branch — capture if it becomes a problem.
4. **`E2E_USER_URL` doc typo.** `docs/E2E_TESTING.md:139` documents `E2E_USER_URL`, but Playwright reads `E2E_URL` (`playwright.config.ts:14`). Doc-only fix, separate task.
5. **`packages/secrets/secrets.go:GetGoogleOAuthConfig` is dead code.** Fallback corrected here, but the function itself can be removed in a cleanup pass.

## Known issues discovered but not fixed in this PR

During discovery for the login-screen chain, two separate problems surfaced
that are **deliberately out of scope** for this branch:

1. **Admin authentication pipeline is not wired to its hardened backend
   endpoint.** The `/admin/login` UI submits through `authStore.login()`,
   which posts to `POST /api/user/auth/login`. The admin-bff path
   (`POST /api/admin/auth/login`) with its stricter rate limit, IP
   whitelist, per-IP failed-login tracker, and 2FA branch is never reached
   from the normal flow. **Severity: HIGH.** Full analysis, line
   references, and a proposed fix outline are in
   [`docs/SECURITY_ISSUE_ADMIN_AUTH_WIRING.md`](SECURITY_ISSUE_ADMIN_AUTH_WIRING.md).
   Fixing this requires its own branch, its own PR, and a security review
   before merge — a partial fix **makes the situation worse**, because the
   admin form would then succeed via the user endpoint while every
   admin-grade protection stays bypassed.

2. **Global router guard redirects all unauthenticated `/admin/**` hits to
   `/user/login`.** Related to (1) but narrower — the file to look at is
   `apps/frontend/src/router/index.ts:42`. Not fixed in isolation here
   because doing so without (1) would silently route admin credentials
   through user-grade protections. Tracked in the same security doc.

Both of these are real, but they are not regressions introduced by this
branch. Shipping the gateway / CSP / assets fixes does not make either
worse than it already was on `main`.

## Rollback

Each commit is independent and reverses cleanly:

```bash
# Roll back everything on this branch:
git revert --no-edit fa90d89 729a5a8 c48f2f4 be52b29
git push origin claude/zen-brown-cQ0Dn

# Or roll back individual commits if only one regresses:
git revert --no-edit <sha>
```

Per-commit failure modes if reverted:

- Revert `be52b29` → arcaptcha widget broken again on pages served via gateway with `default` CSP; token refresh returns 403 from CSRF middleware.
- Revert `c48f2f4` → frontend container reachable on `:5173` again (POST to `/api/*` returns 405); payment redirect URLs point at unreachable `:5173`; CORS rejects gateway-origin requests.
- Revert `729a5a8` → unused-function fallback regresses to wrong path. No runtime impact unless someone calls `GetGoogleOAuthConfig`.
- Revert `fa90d89` → checklist file removed. No runtime impact.

## Verification

See `docs/VERIFICATION_CHECKLIST.md` for the full curl + browser checklist.
Minimum acceptance: items in "Network / routing" all pass with the expected status codes.