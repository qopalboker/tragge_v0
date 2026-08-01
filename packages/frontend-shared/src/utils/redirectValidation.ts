// Redirect validation hardened against open-redirect attacks.
//
// Each app bundles its own allowlist by calling `createRedirectValidator`
// — user-frontend passes `/user` / `/trade` prefixes; admin-frontend
// passes `/admin`. The shared code never sees the other panel's
// prefixes, so a user-frontend bundle cannot be coaxed into redirecting
// to an admin path and vice versa.

export interface RedirectValidatorConfig {
  // Fallback path when validation fails. Must itself satisfy the
  // allowlist or callers will loop.
  defaultRedirect: string;
  // Allowed pathname prefixes. A redirect is accepted if it equals a
  // prefix exactly or starts with `<prefix>/`.
  allowedPrefixes: string[];
  // Optional: accept absolute URLs pointing to hosts whose `hostname`
  // ends with one of these suffixes. Used in dev/Codespaces where
  // cross-port navigation is legitimate. Must still target an
  // allowed prefix.
  trustedHostnameSuffixes?: string[];
}

export interface RedirectValidator {
  validate: (redirect: string | null | undefined) => string;
  // Returns the validated absolute URL if the redirect targets one of
  // the allowed prefixes, else null. Use for same-origin cross-port
  // navigation where the caller needs a URL rather than a path.
  resolveTo: (redirect: string, targetPrefix: string) => string | null;
}

export function createRedirectValidator(
  config: RedirectValidatorConfig,
): RedirectValidator {
  const trustedSuffixes = config.trustedHostnameSuffixes ?? [];

  function isTrustedAbsoluteUrl(url: string): { ok: boolean; pathname: string } {
    try {
      const parsed = new URL(url);
      if (!trustedSuffixes.some((s) => parsed.hostname.endsWith(s))) {
        return { ok: false, pathname: '' };
      }
      const pathAllowed = config.allowedPrefixes.some(
        (p) => parsed.pathname === p || parsed.pathname.startsWith(p + '/'),
      );
      return { ok: pathAllowed, pathname: parsed.pathname };
    } catch {
      return { ok: false, pathname: '' };
    }
  }

  function validate(redirect: string | null | undefined): string {
    if (!redirect || typeof redirect !== 'string') return config.defaultRedirect;
    const cleaned = redirect.trim();
    if (!cleaned) return config.defaultRedirect;

    if (trustedSuffixes.length > 0) {
      const { ok } = isTrustedAbsoluteUrl(cleaned);
      if (ok) return cleaned;
    }

    // Must be a single-slash relative path. Reject protocol-relative
    // (//evil.com), absolute URLs (http://...), and other schemes
    // (javascript:, data:, etc.).
    if (!cleaned.startsWith('/') || cleaned.startsWith('//')) {
      return config.defaultRedirect;
    }
    if (cleaned.includes('://')) {
      return config.defaultRedirect;
    }

    const allowed = config.allowedPrefixes.some(
      (p) => cleaned === p || cleaned.startsWith(p + '/'),
    );
    if (!allowed) return config.defaultRedirect;

    return cleaned;
  }

  function resolveTo(redirect: string, targetPrefix: string): string | null {
    if (trustedSuffixes.length > 0) {
      const { ok, pathname } = isTrustedAbsoluteUrl(redirect);
      if (ok && pathname.startsWith(targetPrefix)) {
        try {
          return new URL(redirect).toString();
        } catch {
          return null;
        }
      }
    }

    if (!redirect.startsWith(targetPrefix)) return null;

    try {
      const url = new URL(redirect, window.location.origin);
      if (url.origin !== window.location.origin) return null;
      if (!url.pathname.startsWith(targetPrefix)) return null;
      return url.toString();
    } catch {
      return null;
    }
  }

  return { validate, resolveTo };
}
