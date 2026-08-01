// User-frontend redirect validator. Admin prefixes are deliberately
// absent — the user-frontend never needs to redirect to /admin, and
// not shipping those strings means a leaked bundle can't be coerced
// into doing so either.
//
// The underlying primitive lives in @tragge/frontend-shared so the
// admin-frontend can assemble its own validator from the same code
// with its own (admin-only) prefixes.
import { createRedirectValidator } from '@tragge/frontend-shared';

const validator = createRedirectValidator({
  defaultRedirect: '/user/dashboard',
  allowedPrefixes: ['/user', '/trade'],
  trustedHostnameSuffixes: ['.app.github.dev'],
});

export function validateRedirectPath(redirect: string | null | undefined): string {
  return validator.validate(redirect);
}

// If the redirect targets the trade panel, return the validated URL
// (absolute for cross-port Codespaces hops; relative otherwise). Used
// by LoginPage / OAuthCallback to decide whether to hop to the trade
// SPA after login.
export function getTradeRedirect(redirect: string): string | null {
  return validator.resolveTo(redirect, '/trade');
}
