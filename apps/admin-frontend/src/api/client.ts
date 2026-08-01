import type { AxiosError } from 'axios';
import {
  createApiClient,
  createTokenBridge,
  getErrorMessage,
  useToast,
} from '@tragge/frontend-shared';
import { getLocale } from '@/i18n';

// The admin-frontend's single axios instance. Separate from the
// user-frontend's client so a leaked admin bundle cannot be coerced
// into calling /api/user/* endpoints â€” neither the base URL nor the
// refresh endpoint overlap with the user client.
const tokenBridge = createTokenBridge();

export const getAccessToken = tokenBridge.get;
export const setAccessToken = tokenBridge.set;

export const api = createApiClient({
  baseURL: import.meta.env.VITE_API_BASE_URL || '',
  refreshEndpoint: '/api/admin/auth/refresh',
  getAccessToken: tokenBridge.get,
  setAccessToken: tokenBridge.set,
  getLocale: () => getLocale(),
  // Login failures must not enter the refresh-retry path.
  loginEndpoints: ['/api/admin/auth/login'],
  onAuthFailure: () => {
    // Definitive auth failure â€” clear local state and bounce to the
    // admin login page. Skip the bounce if we're already there, else
    // a 401 on a resource the login page itself fetches would loop.
    setAccessToken(null);
    const path = window.location.pathname;
    const onLoginPage =
      path === '/admin/login' || path.startsWith('/admin/login/');
    if (onLoginPage) return;

    const redirect = encodeURIComponent(window.location.href);
    window.location.href = `/admin/login?redirect=${redirect}`;
  },
});

// Surface non-auth API errors as toasts. Mirrors the user-frontend
// policy; admin views can opt out by catching the error themselves.
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
