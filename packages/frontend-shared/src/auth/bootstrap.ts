import type { SessionHintConfig } from './types';

// ---------------------------------------------------------------------------
// Session hint helpers
// ---------------------------------------------------------------------------
//
// The session hint is a non-HttpOnly cookie the server sets alongside the
// HttpOnly refresh cookie. Its presence indicates "a refresh is worth
// attempting on cold start". Without it, cold loads of anonymous pages
// (e.g. /login from a fresh tab) would fire a guaranteed-401 refresh POST
// per load.
//
// A legacy localStorage key is honoured as a one-release migration
// fallback for users whose hint predates the cookie rollout. It is never
// evicted on read — eviction runs only when the outcome of the refresh
// attempt is known (in bootstrap's caller via setAccessToken).

export function hasSessionHint(config: SessionHintConfig): boolean {
  try {
    const cookieValue = `${config.cookieName}=1`;
    const cookiePresent = document.cookie
      .split('; ')
      .some((c) => c === cookieValue || c.startsWith(`${cookieValue};`));
    if (cookiePresent) return true;
    if (config.legacyLocalStorageKey) {
      return localStorage.getItem(config.legacyLocalStorageKey) === '1';
    }
    return false;
  } catch {
    return false;
  }
}

export function clearSessionHintCookie(config: SessionHintConfig): void {
  try {
    // Mirror the server's Path=/ so we actually overwrite the cookie.
    // A mismatched path would create a zombie cookie that keeps
    // triggering refresh attempts after logout.
    document.cookie = `${config.cookieName}=; Max-Age=0; Path=/; SameSite=Lax`;
  } catch {
    /* noop — document.cookie can throw in very old embedded WebViews */
  }
}

export function clearLegacySessionHint(config: SessionHintConfig): void {
  if (!config.legacyLocalStorageKey) return;
  try {
    localStorage.removeItem(config.legacyLocalStorageKey);
  } catch {
    /* noop */
  }
}

// ---------------------------------------------------------------------------
// Bootstrap dedup
// ---------------------------------------------------------------------------

export type BootstrapFn = () => Promise<void>;

// Wrap an async bootstrap routine so concurrent callers share a single
// in-flight promise. Preserves the dedup that kept `main.ts`'s pre-mount
// bootstrap and the first router guard from racing two refresh requests
// on cold start.
//
// The promise is memoized for the process lifetime — once bootstrap
// resolves, every subsequent call is an instantaneous no-op. The inner
// fn is expected to swallow its own errors (or settle in a finally
// block) so callers can treat bootstrap as always-succeeding; this is
// how today's auth store works, and we preserve it.
export function createBootstrap(fn: BootstrapFn): BootstrapFn {
  let promise: Promise<void> | null = null;
  return () => {
    if (!promise) {
      promise = (async () => {
        await fn();
      })();
    }
    return promise;
  };
}
