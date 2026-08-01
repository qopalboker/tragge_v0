import axios, {
  type AxiosError,
  type AxiosInstance,
  type InternalAxiosRequestConfig,
} from 'axios';
import { refreshAccessToken } from '../auth/refresh';

export interface ApiClientConfig {
  // Base URL for all requests. Each app passes its own —
  // typically import.meta.env.VITE_API_BASE_URL. No default; shared
  // code never guesses at endpoint shapes.
  baseURL: string;

  // Where to POST to refresh the access token. Must match the cookie
  // audience (user endpoint for the user panel, admin endpoint for the
  // admin panel) — a cross-audience refresh is expected to 401.
  refreshEndpoint: string;

  // Access token accessors. Wire to the app's auth store or a
  // tokenBridge instance from `@tragge/frontend-shared`.
  getAccessToken: () => string | null;
  setAccessToken: (token: string | null) => void;

  // Optional Accept-Language provider. If present, its return value is
  // sent on every request.
  getLocale?: () => string;

  // Called when a 401 cannot be recovered via refresh (definitive
  // auth failure, as distinct from transient network blips). The app
  // uses this hook to clear its auth store and, if desired, redirect
  // to its own login page — shared code does not know the login URL.
  onAuthFailure: () => void;

  // Paths whose 401 must NOT trigger the silent-refresh-then-retry
  // flow. Login endpoints in particular: a wrong-password 401 on
  // /login would otherwise try to refresh, fail, and bounce the user
  // off the login page they are already on. Paths are matched on
  // pathname (trailing slashes stripped).
  loginEndpoints?: string[];

  // Request timeout in milliseconds. Defaults to 30_000.
  timeoutMs?: number;
}

function normalizePath(url: string | undefined): string | null {
  if (!url) return null;
  try {
    const pathname = new URL(url, 'http://x').pathname.replace(/\/+$/, '');
    return pathname || '/';
  } catch {
    return null;
  }
}

// Build an axios client with auth-bearing request interceptor and a
// response interceptor that silently refreshes on 401 and retries the
// original request. Preserves the request-queue dedup that keeps
// concurrent 401s from firing N parallel refresh POSTs — the first 401
// starts the refresh; every subsequent 401 while it is in flight joins a
// queue and resumes with the fresh token once the refresh resolves.
export function createApiClient(cfg: ApiClientConfig): AxiosInstance {
  const client = axios.create({
    baseURL: cfg.baseURL,
    timeout: cfg.timeoutMs ?? 30_000,
    withCredentials: true,
    headers: {
      'Content-Type': 'application/json',
      // Every request carries this; the BFF's CSRF middleware requires
      // it. Setting it here means callers cannot forget it on ad-hoc
      // requests made through the client.
      'X-Requested-With': 'XMLHttpRequest',
    },
  });

  const loginPaths = new Set(
    (cfg.loginEndpoints ?? [])
      .map(normalizePath)
      .filter((p): p is string => p !== null),
  );
  const refreshPath = normalizePath(cfg.refreshEndpoint);

  type Pending = {
    resolve: (token: string) => void;
    reject: (err: unknown) => void;
  };
  let isRefreshing = false;
  let failedQueue: Pending[] = [];

  function drainQueue(error: unknown, token: string | null): void {
    const queue = failedQueue;
    failedQueue = [];
    for (const p of queue) {
      if (error || !token) {
        p.reject(error ?? new Error('Refresh failed'));
      } else {
        p.resolve(token);
      }
    }
  }

  client.interceptors.request.use((config) => {
    const token = cfg.getAccessToken();
    if (token && config.headers) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    if (cfg.getLocale && config.headers) {
      config.headers['Accept-Language'] = cfg.getLocale();
    }
    return config;
  });

  client.interceptors.response.use(
    (r) => r,
    async (error: AxiosError) => {
      const original = error.config as
        | (InternalAxiosRequestConfig & { _retry?: boolean })
        | undefined;
      const status = error.response?.status;
      const path = normalizePath(original?.url);

      const isLoginRequest = path !== null && loginPaths.has(path);
      const isRefreshRequest = refreshPath !== null && path === refreshPath;

      if (
        status !== 401 ||
        !original ||
        original._retry ||
        isLoginRequest ||
        isRefreshRequest
      ) {
        return Promise.reject(error);
      }

      if (isRefreshing) {
        return new Promise<string>((resolve, reject) => {
          failedQueue.push({ resolve, reject });
        }).then((token) => {
          original._retry = true;
          if (original.headers) {
            original.headers.Authorization = `Bearer ${token}`;
          }
          return client(original);
        });
      }

      original._retry = true;
      isRefreshing = true;

      try {
        const result = await refreshAccessToken({
          endpoint: cfg.refreshEndpoint,
        });
        if (result.ok) {
          cfg.setAccessToken(result.accessToken);
          if (original.headers) {
            original.headers.Authorization = `Bearer ${result.accessToken}`;
          }
          drainQueue(null, result.accessToken);
          return client(original);
        }

        drainQueue(new Error('Refresh failed'), null);
        if (result.error.kind === 'auth') {
          cfg.onAuthFailure();
        }
        // Transient errors: preserve session, let the caller see the
        // original 401 and retry on its own cadence.
        return Promise.reject(error);
      } finally {
        isRefreshing = false;
      }
    },
  );

  return client;
}
