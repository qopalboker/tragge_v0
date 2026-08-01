# Verification Checklist

Temporary checklist tied to branch `claude/zen-brown-cQ0Dn`. Delete or migrate
to a permanent runbook once the changes have been verified end-to-end.

## Context

Static-only fixes were made for: gateway CSP (arcaptcha), token-refresh CSRF
header, frontend container exposure (`5173:8080` → `expose: 8080`), payment
redirect defaults, and a dead-code Google OAuth fallback. None of these were
runtime-verified — Docker was unavailable in the session that produced them.

## Post-docker verification

```bash
docker compose -f infra/docker/docker-compose.yml up -d
```

### Network / routing
- [ ] `curl -sI http://localhost:8080/user/login | grep -i "content-security"` →
      `script-src` and `connect-src` must include `https://widget.arcaptcha.co`
      (and `https://api.arcaptcha.co` in `connect-src`).
- [ ] `curl -sI http://localhost:5173/user/login` → connection refused
      (frontend container no longer publishes 5173).

### Gateway static asset proxying (step 7 — SPA white-screen regression fix)
- [ ] `curl -sI http://localhost:8080/user/login` → 200, `content-type: text/html`.
- [ ] In the response body (`curl -s http://localhost:8080/user/login | grep -oE '/assets/[^"]+' | sort -u`)
      every referenced asset returns 200:
      `for a in $(curl -s http://localhost:8080/user/login | grep -oE '/assets/[^"]+' | sort -u); do printf '%s ' "$a"; curl -so /dev/null -w '%{http_code}\n' "http://localhost:8080$a"; done`
      → all 200, none 404, none `content-type: text/html` (would indicate SPA fallback).
- [ ] `curl -sI http://localhost:8080/favicon.svg` → 200, `content-type: image/svg+xml`.
- [ ] `curl -sI http://localhost:8080/manifest.json` → 200, `content-type: application/json`.
- [ ] `curl -sI http://localhost:8080/icons/icon-72x72.png` → 200 (referenced by
      InstallPrompt.vue / IOSInstallModal.vue).
- [ ] `curl -sI http://localhost:8080/offline.html` → 200.
- [ ] `curl -sI http://localhost:8080/browserconfig.xml` → 200.
- [ ] `curl -sI http://localhost:8080/robots.txt http://localhost:8080/sitemap.xml http://localhost:8080/favicon.ico`
      → 200 (prod parity — previously worked only in dev).
- [ ] DevTools Network tab on `/user/login`: zero 404s, all `/assets/*` served
      with `Cache-Control: public, immutable`.
- [ ] `curl -X POST http://localhost:8080/api/user/auth/login -H "X-Requested-With: XMLHttpRequest" -H "Content-Type: application/json" -d '{"email":"x","password":"x"}'`
      → 400 / 401 (must NOT be 405 — that would mean request bypassed gateway —
      and must NOT be 403 — that would mean CSRF middleware rejected it).
- [ ] `curl -X POST http://localhost:8080/api/user/auth/refresh -H "X-Requested-With: XMLHttpRequest" -H "Content-Type: application/json" -d '{}'`
      → 401 (must NOT be 403 CSRF — that would mean the X-Requested-With fix
      did not take effect).

### Browser smoke
- [ ] `http://localhost:8080/user/login` → login form renders, ARCaptcha widget
      becomes visible.
- [ ] Register with new credentials → success.
- [ ] Login with the same credentials → success.
- [ ] Google OAuth → redirect to Google → back to `/user/auth/google/callback`
      → login success. **Important: do not assume OAuth was working before this
      branch. The original report of "Google doesn't work" may have been a
      downstream symptom of the CSP block (login form half-loaded → button
      never wired up). Test it cold; if it still fails, the bug is real and
      separate from CSP.**
- [ ] DevTools Console — check for SVG `<path d="…##…">` errors. Step 6 of
      this branch has not yet addressed them; expected to still fail until
      that step lands.

## Notes for follow-up

- **Vite dev proxy** in `apps/frontend/vite.config.ts` still targets
  `localhost:8081/8082/8083` directly (bypasses gateway). Fine for `pnpm dev`
  on host, but if any developer runs `pnpm dev` while BFFs are containerized
  without host port mappings, the proxy targets are unreachable. Out of scope
  for this branch — capture in a separate task if it becomes a problem.
- `packages/secrets/secrets.go:GetGoogleOAuthConfig` is dead code. Fallback was
  corrected in this branch, but the function itself can be removed in a
  cleanup pass.
- `docs/E2E_TESTING.md:139` references `E2E_USER_URL`, but Playwright actually
  reads `E2E_URL` (`playwright.config.ts:14`). Doc-only typo, unrelated to
  this branch.
- **`apps/gateway/nginx.prod.conf` has never been production-tested.** Prior to
  step 7 it was missing every static-asset location block including
  `/favicon.ico`, `/robots.txt`, `/sitemap.xml`, `/assets/`, `/favicon.svg`,
  `/manifest.json`, `/icons/`, `/fonts/`, `/splash/`, `/charting_library/`,
  `/browserconfig.xml`, `/offline.html`. Its catch-all `location /` returns
  404. Had anyone deployed the prod conf unchanged, the SPA would have been
  completely unreachable outside `/user`, `/trade`, `/admin`. Dev was less
  broken only because a few paths (`/favicon.ico`, `/robots.txt`,
  `/sitemap.xml`) happened to be proxied; `/assets/` was broken in both.
  Full verification pass against a prod-built image is required before the
  next release.
- **Vite dev-server paths behind the gateway are out of scope.** If `pnpm dev`
  is ever run behind `apps/gateway/nginx.conf` (rather than on `localhost:5173`
  directly), HMR paths `/src/*`, `/@vite/*`, `/@id/*`, `/@fs/*`, and the HMR
  WebSocket at `/__vite_hmr` will need their own proxy location blocks, and
  the `/assets/` block will need the `Upgrade`/`Connection` headers that
  `/user`, `/trade`, `/admin` already carry. Not a concern while the canonical
  dev flow is `pnpm dev` on host + gateway in Docker.
- **Public-file cross-path inconsistency.** `apps/frontend/public/manifest.json`
  and `apps/frontend/public/browserconfig.xml` reference icons at `/user/icons/*`,
  but `apps/frontend/src/modules/user/components/common/{InstallPrompt,IOSInstallModal}.vue`
  reference `/icons/*` (root). `/user/icons/*` hits `location /user`, proxies
  to the frontend container, hits its `try_files` fallback, and returns
  `index.html` with an HTML content-type — i.e. a broken icon tile for Windows
  and broken PWA icons. Preexisting; out of scope for the gateway asset fix.
  Follow-up: pick one convention (recommend root-absolute `/icons/*`) and
  update the public files.

## Known limitations after this branch merges

These are real issues that are **deliberately not fixed here** because they
fall outside the scope of the CSP / assets / 405 / redirect chain. They
should be tracked in their own branches.

- **`/admin` access by non-admin roles is not blocked at the frontend route
  level by any backend role check.** The router guard checks
  `auth.hasRole('admin')` client-side (`apps/frontend/src/router/index.ts:49`),
  but client-side guards are advisory. The real enforcement is
  `RequireAdminAccess` middleware on every admin-bff route
  (`apps/admin-bff/server/app.go:498, 529, 650, ...`), which should reject a
  non-admin JWT with 403.
  **Test after this branch is up:** log in as a plain `user`, manually
  navigate to `/admin/dashboard`, open DevTools → Network, and confirm that
  every `/api/admin/*` call returns **403** (not 200, not 401). If any 200
  comes back, that is a separate incident.

- **Admin authentication pipeline is not wired to the hardened backend
  endpoint.** The `/admin/login` UI submits through the shared user login
  path, bypassing nginx's `admin_auth_limit` zone, the admin IP whitelist,
  the per-IP failed-login tracker, and the 2FA challenge. Severity HIGH.
  Full analysis, line references, and proposed fix outline in
  [`docs/SECURITY_ISSUE_ADMIN_AUTH_WIRING.md`](SECURITY_ISSUE_ADMIN_AUTH_WIRING.md).
  Do **not** try to fix this in the same branch as gateway/asset changes —
  a partial fix (e.g. only the router redirect) makes the situation worse
  by giving the illusion of a working admin login while every server-side
  protection remains bypassed.

## SVG `##` icon bug — deferred

Static analysis could not localize the source of the `Expected number, "…0.444780743,15.7## C1.2341904,17…"` console error reported on the login screen.

Searched and ruled out:
- `grep '##' apps/frontend/src --include="*.vue,*.ts,*.css,*.scss"` → zero matches
- `grep '\${' apps/frontend/src --include="*.vue"` near `<svg`/`<path`/`d="` → only matches were unrelated (modal placeholder text, router-push URL)
- All inline SVG path strings in components are static (e.g. `getMarketIcon` in `ContestStatsDashboard.vue:49`)
- Dynamic SVG constructions are `polyline :points` (no `C` cubic Bézier), in `ProfilePage.vue:683` and `FinancialPage.vue:300`
- No SVG files in `apps/frontend/src/assets/`; static SVGs in `public/` are clean

Conclusion: source code is clean. The bug originates in either:
1. A vendor SVG inside `node_modules` (third-party icon library where a template placeholder was left literally)
2. A runtime path computation fed broken backend data

Localization plan (requires running app, not static):
1. `docker compose up -d` and load the page where the error appears
2. In DevTools console, set a conditional breakpoint:
   `Element.prototype.setAttribute` with condition `name === 'd' && String(value).includes('##')`
3. Reload the page; breakpoint hits → stack trace points to the constructing component
4. Capture: full stack trace, the broken element's `outerHTML`, and the parent component name
5. Open a new branch with that data; the fix is then trivial (either patch the vendor file or fix the data flow)

Severity: cosmetic (one icon renders broken). Not a release blocker per the original report.
