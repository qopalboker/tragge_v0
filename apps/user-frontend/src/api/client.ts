import type { AxiosError } from 'axios';
import {
  createApiClient,
  createTokenBridge,
  getErrorMessage,
  useToast,
} from '@tragge/frontend-shared';
import { getLocale } from '@/i18n';

// The user-frontend's single axios instance. The token bridge lets the
// auth store push the in-memory access token in without creating a
// circular import (the store reads/sets through this module; the
// interceptor reads from the same bridge).
const tokenBridge = createTokenBridge();

export const getAccessToken = tokenBridge.get;
export const setAccessToken = tokenBridge.set;

export const api = createApiClient({
  baseURL: import.meta.env.VITE_API_BASE_URL || '',
  refreshEndpoint: '/api/user/auth/refresh',
  getAccessToken: tokenBridge.get,
  setAccessToken: tokenBridge.set,
  getLocale: () => getLocale(),
  // Paths whose 401 must NOT trigger refresh-retry. Exact-match on
  // URL.pathname per PR #933 — the shared client normalises both
  // sides, so trailing slashes and absolute/relative URLs all line up.
  // User-frontend only ships user login paths; a leaked bundle cannot
  // be coaxed into whitelisting /api/admin/auth/login.
  loginEndpoints: [
    '/api/user/auth/login',
    '/api/user/auth/2fa/login',
    '/api/user/auth/register',
  ],
  onAuthFailure: () => {
    // Definitive auth failure — clear local state and bounce to the
    // user login page. Don't bounce if we're already on login; that
    // would cause a reload loop when a 401 fires for a resource the
    // login page itself requests.
    setAccessToken(null);
    const path = window.location.pathname;
    const onLoginPage =
      path === '/user/login' ||
      path.startsWith('/user/login/') ||
      path === '/trade/login';
    if (onLoginPage) return;

    const redirect = encodeURIComponent(window.location.href);
    const loginPath = '/user/login';

    // Codespaces: each Vite dev port is a distinct hostname. A 401 on
    // localhost:5174 (trade) should bounce to the user-frontend's
    // hostname on 5173, not to /user/login on the current port (which
    // 404s). In prod this branch is unreachable.
    const host = window.location.hostname;
    if (host.endsWith('.app.github.dev')) {
      const base = window.location.origin.replace(
        /-\d+\.app\.github\.dev/,
        '-5173.app.github.dev',
      );
      window.location.href = `${base}${loginPath}?redirect=${redirect}`;
      return;
    }
    window.location.href = `${loginPath}?redirect=${redirect}`;
  },
});

// Surface non-auth API errors as toasts. The pre-split code did this
// inline in the response interceptor; keeping it here preserves the
// UX without bloating the shared client (admin-frontend may want a
// different policy — e.g. silent errors on dashboards that render
// their own error state).
api.interceptors.response.use(
  (r) => r,
  (error: AxiosError) => {
    const status = error.response?.status;
    if (status !== 401) {
      useToast().error(getErrorMessage(error));
    }
    return Promise.reject(error);
  },
);

