import type { BrowserContext, Page, Request, Route } from '@playwright/test';

import { TEST_USERS } from '../../../e2e/test-data';

export const USER_AUTH_STATE_FILE = './apps/user-frontend/e2e/.auth/user.json';
export const USER_BASE_URL = process.env.E2E_USER_URL || 'http://localhost:5173';
export const SYNTHETIC_OTP_CODE = '135790';

const SYNTHETIC_REFRESH_COOKIE = 'synthetic-user-refresh-cookie';
const SYNTHETIC_CAPTCHA_PROOF = 'synthetic-captcha-proof';
const SYNTHETIC_RESET_HANDLE = 'synthetic-reset-handle';
const SYNTHETIC_PASSWORD_SET_HANDLE = 'synthetic-password-set-handle';

export interface MockAuthRequest {
  method: string;
  path: string;
  body: Record<string, unknown>;
}

export interface MockAuthState {
  sessionValid: boolean;
  emailVerified: boolean;
  refreshRequests: number;
  loginRequests: MockAuthRequest[];
  registrationRequests: MockAuthRequest[];
  verificationSendRequests: MockAuthRequest[];
  verificationRequests: MockAuthRequest[];
  resetRequests: MockAuthRequest[];
  logoutRequests: number;
  browserOutput: string[];
  issuedAccessTokens: string[];
}

export function createMockAuthState(
  overrides: Partial<MockAuthState> = {},
): MockAuthState {
  return {
    sessionValid: false,
    emailVerified: true,
    refreshRequests: 0,
    loginRequests: [],
    registrationRequests: [],
    verificationSendRequests: [],
    verificationRequests: [],
    resetRequests: [],
    logoutRequests: 0,
    browserOutput: [],
    issuedAccessTokens: [],
    ...overrides,
  };
}

function encodeTokenPart(value: object): string {
  return Buffer.from(JSON.stringify(value)).toString('base64url');
}

function createSyntheticAccessToken(): string {
  const now = Math.floor(Date.now() / 1000);
  return [
    encodeTokenPart({ alg: 'HS256', typ: 'JWT' }),
    encodeTokenPart({
      sub: 'synthetic-user-id',
      user_id: 'synthetic-user-id',
      aud: 'tragge-user-frontend',
      iss: 'tragge-user-auth',
      iat: now,
      exp: now + 3600,
    }),
    'synthetic-signature',
  ].join('.');
}

function issueAccessToken(state: MockAuthState): string {
  const token = createSyntheticAccessToken();
  state.issuedAccessTokens.push(token);
  return token;
}

async function setUserSessionCookies(context: BrowserContext): Promise<void> {
  const host = new URL(USER_BASE_URL).hostname;
  await context.addCookies([
    {
      name: 'refresh_token_user',
      value: SYNTHETIC_REFRESH_COOKIE,
      domain: host,
      path: '/api/user/auth',
      httpOnly: true,
      secure: false,
      sameSite: 'Lax',
    },
    {
      name: 'tragge_session_hint_user',
      value: '1',
      domain: host,
      path: '/',
      httpOnly: false,
      secure: false,
      sameSite: 'Lax',
    },
  ]);
}

async function clearUserSessionCookies(context: BrowserContext): Promise<void> {
  await context.clearCookies({ name: /^(?:refresh_token_user|tragge_session_hint_user)$/ });
}

function requestBody(request: Request): Record<string, unknown> {
  try {
    return request.postDataJSON() as Record<string, unknown>;
  } catch {
    return {};
  }
}

async function json(
  route: Route,
  status: number,
  body: unknown,
): Promise<void> {
  await route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
}

function userRecord(state: MockAuthState): Record<string, unknown> {
  return {
    user_id: 'synthetic-user-id',
    email: TEST_USERS.standard.email,
    roles: ['USER'],
    email_verified: state.emailVerified,
    phone_verified: false,
    username: 'syntheticuser',
    display_name: TEST_USERS.standard.name,
    country: 'US',
    created_at: '2026-01-01T00:00:00Z',
    has_password: true,
  };
}

function requestRecord(request: Request, path: string): MockAuthRequest {
  return {
    method: request.method(),
    path,
    body: requestBody(request),
  };
}

export async function installCaptchaMock(page: Page): Promise<void> {
  await page.route('https://widget.arcaptcha.co/**', (route) =>
    route.fulfill({ status: 200, contentType: 'application/javascript', body: '' }),
  );
  await page.addInitScript((proof) => {
    let callback: ((value: string) => void) | undefined;
    const target = window as typeof window & {
      arcaptcha?: {
        render: (
          element: HTMLElement | null,
          options: { callback?: (value: string) => void },
        ) => string;
        execute: () => void;
        reset: () => void;
      };
    };
    target.arcaptcha = {
      render: (_element, options) => {
        callback = options.callback;
        return 'synthetic-widget';
      },
      execute: () => queueMicrotask(() => callback?.(proof)),
      reset: () => undefined,
    };
  }, SYNTHETIC_CAPTCHA_PROOF);
}

export async function installMockAuthBackend(
  page: Page,
  state: MockAuthState,
): Promise<void> {
  page.on('console', (message) => state.browserOutput.push(message.text()));
  page.on('pageerror', (error) => state.browserOutput.push(error.message));

  await page.route('**/api/user/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const body = requestBody(request);

    if (path === '/api/user/auth/login') {
      state.loginRequests.push(requestRecord(request, path));
      if (
        request.method() !== 'POST' ||
        body.email !== TEST_USERS.standard.email ||
        body.password !== TEST_USERS.standard.password
      ) {
        await json(route, 401, { error: 'invalid_credentials' });
        return;
      }
      state.sessionValid = true;
      state.emailVerified = true;
      await setUserSessionCookies(page.context());
      await json(route, 200, { access_token: issueAccessToken(state) });
      return;
    }

    if (path === '/api/user/auth/register') {
      state.registrationRequests.push(requestRecord(request, path));
      state.sessionValid = true;
      state.emailVerified = false;
      await setUserSessionCookies(page.context());
      await json(route, 200, {
        access_token: issueAccessToken(state),
        user_id: 'synthetic-user-id',
        requires_verification: true,
        available_methods: ['email'],
        masked_email: 't***@example.com',
      });
      return;
    }

    if (path === '/api/user/auth/refresh') {
      state.refreshRequests += 1;
      if (!state.sessionValid) {
        await clearUserSessionCookies(page.context());
        await json(route, 401, { error: 'unauthorized' });
        return;
      }
      await json(route, 200, { access_token: issueAccessToken(state) });
      return;
    }

    if (path === '/api/user/logout') {
      state.logoutRequests += 1;
      state.sessionValid = false;
      await clearUserSessionCookies(page.context());
      await json(route, 200, { message: 'logged out' });
      return;
    }

    if (path === '/api/user/auth/send-verification') {
      state.verificationSendRequests.push(requestRecord(request, path));
      await json(route, 200, {
        message: 'sent',
        destination_masked: 't***@example.com',
        expires_in_seconds: 600,
        resend_cooldown_seconds: 120,
      });
      return;
    }

    if (path === '/api/user/auth/verify-code') {
      state.verificationRequests.push(requestRecord(request, path));
      if (body.code !== SYNTHETIC_OTP_CODE) {
        await json(route, 400, { error: 'invalid_code', remaining_attempts: 4 });
        return;
      }
      state.emailVerified = true;
      await json(route, 200, { message: 'verified' });
      return;
    }

    if (path === '/api/user/auth/forgot-password/request') {
      state.resetRequests.push(requestRecord(request, path));
      await json(route, 200, {
        message: 'code sent',
        reset_token: SYNTHETIC_RESET_HANDLE,
        channel_hint: 'email',
        masked_destination: 't***@example.com',
        retry_after_seconds: 120,
      });
      return;
    }

    if (path === '/api/user/auth/forgot-password/verify') {
      state.resetRequests.push(requestRecord(request, path));
      if (
        body.reset_token !== SYNTHETIC_RESET_HANDLE ||
        body.code !== SYNTHETIC_OTP_CODE
      ) {
        await json(route, 400, { error: 'invalid_code', remaining_attempts: 4 });
        return;
      }
      await json(route, 200, { password_set_token: SYNTHETIC_PASSWORD_SET_HANDLE });
      return;
    }

    if (path === '/api/user/auth/forgot-password/reset') {
      state.resetRequests.push(requestRecord(request, path));
      if (body.password_set_token !== SYNTHETIC_PASSWORD_SET_HANDLE) {
        await json(route, 400, { error: 'invalid_reset' });
        return;
      }
      state.sessionValid = false;
      await clearUserSessionCookies(page.context());
      await json(route, 200, { message: 'password reset' });
      return;
    }

    if (path === '/api/user/me' && request.method() === 'GET') {
      if (!state.sessionValid) {
        await json(route, 401, { error: 'unauthorized' });
        return;
      }
      await json(route, 200, userRecord(state));
      return;
    }

    if (path === '/api/user/me/profile' && request.method() === 'PUT') {
      await json(route, 200, { updated: true });
      return;
    }

    if (path === '/api/user/me/stats') {
      await json(route, 200, {
        tragge_point: 0,
        total_contests: 0,
        total_wins: 0,
        win_rate: 0,
      });
      return;
    }

    if (path === '/api/user/global-leaderboard') {
      await json(route, 200, { entries: [], user_rank: null });
      return;
    }

    if (path === '/api/user/contests') {
      await json(route, 200, { contests: [] });
      return;
    }

    if (path === '/api/user/me/history') {
      await json(route, 200, { contests: [], total: 0 });
      return;
    }

    if (path.endsWith('/unread-count')) {
      await json(route, 200, { count: 0 });
      return;
    }

    await json(route, 200, {});
  });
}

export function sensitiveBrowserValues(state: MockAuthState): string[] {
  return [
    TEST_USERS.standard.password,
    SYNTHETIC_REFRESH_COOKIE,
    SYNTHETIC_CAPTCHA_PROOF,
    SYNTHETIC_RESET_HANDLE,
    SYNTHETIC_PASSWORD_SET_HANDLE,
    SYNTHETIC_OTP_CODE,
    ...state.issuedAccessTokens,
  ].filter((value) => value.length >= 6);
}
