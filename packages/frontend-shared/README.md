# @tragge/frontend-shared

Primitives shared between `apps/user-frontend` and `apps/admin-frontend`.

## Scope

This package deliberately exposes **primitives**, not finished auth
stores or API clients. Each frontend app assembles its own auth store
and API client from these primitives so the security boundary between
the panels is physical (what ships in the bundle) rather than a runtime
`audience` flag.

## Layout

- `auth/` — refresh primitive, session-hint helpers, cross-tab channel,
  bootstrap dedup wrapper, token bridge.
- `api/client.ts` — `createApiClient(config)` factory. No hardcoded
  endpoints; each app supplies its own `refreshEndpoint`, login path
  allowlist, and `onAuthFailure` hook.
- `stores/` — `useThemeStore`, `useI18nStore`. Both genuinely shared.
- `i18n/` — `createI18n`-style `initializeI18n(appMessages)` helper
  that deep-merges a per-app locale tree over a shared `common` tree
  (generic errors, validation, etc.).
- `composables/useToast.ts` — toast queue shared across both panels.
- `utils/` — formatters, errorHandler (uses shared i18n singleton),
  logger, `createRedirectValidator` factory.
- `styles/main.css` — global CSS + design tokens.

## Usage

```ts
import {
  createApiClient,
  createTokenBridge,
  initializeI18n,
  useThemeStore,
} from '@tragge/frontend-shared';
import '@tragge/frontend-shared/styles/main.css';

const tokens = createTokenBridge();
const api = createApiClient({
  baseURL: import.meta.env.VITE_API_BASE_URL,
  refreshEndpoint: '/api/user/auth/refresh',
  getAccessToken: tokens.get,
  setAccessToken: tokens.set,
  loginEndpoints: ['/api/user/auth/login', '/api/user/auth/2fa/login'],
  onAuthFailure: () => { /* app-specific: clear store + redirect */ },
});

initializeI18n({ en: userEnMessages, fa: userFaMessages });
```

## Security notes

- `refreshAccessToken` ALWAYS sends `X-Requested-With: XMLHttpRequest`.
  The BFF CSRF middleware requires it; silently dropping it would
  regress PR #936.
- `createRedirectValidator` takes per-app prefix allowlists. The
  user-frontend bundle cannot validate `/admin/*` targets because it
  never imports the admin prefix, and vice versa.
- No login/logout/register/OAuth logic is provided — each app writes
  its own auth store.
