const DEFAULT_REDIRECT = '/user/dashboard';
const ALLOWED_PREFIXES = ['/user', '/trade'];

/**
 * Check if a URL is a trusted Codespaces origin for this workspace.
 * Codespaces URLs follow the pattern: https://<name>-<port>.app.github.dev
 */
function isTrustedCodespacesUrl(url: string): boolean {
  try {
    const parsed = new URL(url);
    if (!parsed.hostname.endsWith('.app.github.dev')) return false;
    // Must target /trade or /user path
    return ALLOWED_PREFIXES.some(p => parsed.pathname === p || parsed.pathname.startsWith(p + '/'));
  } catch {
    return false;
  }
}

/**
 * Validate a redirect path to prevent open redirect attacks.
 * Allows relative paths starting with allowed prefixes (/user, /trade)
 * and trusted Codespaces absolute URLs.
 * Returns the default dashboard path if validation fails.
 */
export function validateRedirectPath(redirect: string | null | undefined): string {
  if (!redirect || typeof redirect !== 'string') {
    return DEFAULT_REDIRECT;
  }

  const cleaned = redirect.trim();

  if (!cleaned) {
    return DEFAULT_REDIRECT;
  }

  // Allow trusted Codespaces absolute URLs (cross-port redirect)
  if (isTrustedCodespacesUrl(cleaned)) {
    return cleaned;
  }

  // Must start with a single forward slash (relative path).
  // Block protocol-relative URLs (//evil.com), absolute URLs (http://),
  // and other schemes (javascript:, data:, etc.)
  if (!cleaned.startsWith('/') || cleaned.startsWith('//')) {
    return DEFAULT_REDIRECT;
  }

  // Block URLs containing protocol indicators anywhere
  if (cleaned.includes('://')) {
    return DEFAULT_REDIRECT;
  }

  // Must start with one of the allowed prefixes (exact match or followed by /)
  const hasAllowedPrefix = ALLOWED_PREFIXES.some(
    prefix => cleaned === prefix || cleaned.startsWith(prefix + '/')
  );
  if (!hasAllowedPrefix) {
    return DEFAULT_REDIRECT;
  }

  return cleaned;
}

/**
 * If the redirect path targets the trade-frontend, return the validated URL
 * without any token in the query string. Both frontends share localStorage
 * on the same origin, so no token transfer is needed in the URL.
 *
 * Returns null if the redirect is not a valid trade path.
 */
export function getTradeRedirect(redirectPath: string): string | null {
  // Handle absolute Codespaces URLs (cross-port redirect)
  if (isTrustedCodespacesUrl(redirectPath)) {
    try {
      const url = new URL(redirectPath);
      if (url.pathname.startsWith('/trade')) {
        return url.toString();
      }
    } catch { /* fall through */ }
    return null;
  }

  if (!redirectPath.startsWith('/trade')) {
    return null;
  }

  try {
    const url = new URL(redirectPath, window.location.origin);

    // Verify same-origin (defense in depth)
    if (url.origin !== window.location.origin) {
      return null;
    }

    // Verify pathname still targets trade after URL normalization (catches path traversal)
    if (!url.pathname.startsWith('/trade')) {
      return null;
    }

    return url.toString();
  } catch {
    return null;
  }
}

const DEFAULT_ADMIN_REDIRECT = '/admin/dashboard';
const ADMIN_ALLOWED_PREFIXES = ['/admin'];

/**
 * Validate a redirect path for the admin panel.
 * Allows relative paths starting with /admin.
 * Returns the default admin dashboard path if validation fails.
 */
export function validateAdminRedirectPath(redirect: string | null | undefined): string {
  if (!redirect || typeof redirect !== 'string') {
    return DEFAULT_ADMIN_REDIRECT;
  }

  const cleaned = redirect.trim();

  if (!cleaned) {
    return DEFAULT_ADMIN_REDIRECT;
  }

  // Must start with a single forward slash (relative path)
  if (!cleaned.startsWith('/') || cleaned.startsWith('//')) {
    return DEFAULT_ADMIN_REDIRECT;
  }

  // Block URLs containing protocol indicators
  if (cleaned.includes('://')) {
    return DEFAULT_ADMIN_REDIRECT;
  }

  // Must start with /admin
  const hasAllowedPrefix = ADMIN_ALLOWED_PREFIXES.some(
    prefix => cleaned === prefix || cleaned.startsWith(prefix + '/')
  );
  if (!hasAllowedPrefix) {
    return DEFAULT_ADMIN_REDIRECT;
  }

  return cleaned;
}
