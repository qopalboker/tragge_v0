import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { readFileSync } from 'node:fs';

const apiMock = vi.hoisted(() => ({
  post: vi.fn(),
  put: vi.fn(),
  get: vi.fn(),
}));

vi.mock('@/api/client', () => ({
  api: apiMock,
  setAccessToken: vi.fn(),
}));
vi.mock('@sentry/vue', () => ({ setUser: vi.fn() }));
vi.mock('@/composables/useToast', () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));
vi.mock('@/utils/errorHandler', () => ({
  getErrorMessage: () => 'fixture error',
}));
vi.mock('@/services/oauth', () => ({
  handleGoogleCallback: vi.fn(),
  clearStoredState: vi.fn(),
}));
vi.mock('@/i18n', () => ({ t: (key: string) => key }));
vi.mock('@tragge/frontend-shared', () => ({
  refreshAccessToken: vi.fn(),
  hasSessionHint: vi.fn(() => false),
  clearSessionHintCookie: vi.fn(),
  clearLegacySessionHint: vi.fn(),
  createCrossTabChannel: () => ({ broadcastLogout: vi.fn(), onRemoteLogout: vi.fn() }),
  createBootstrap: (callback: () => Promise<void>) => callback,
}));

import { normalizeRegistrationCountry, useAuthStore } from './auth';

const userFixture = {
  user_id: 'fixture-user',
  email: 'fixture@example.test',
  roles: ['USER'],
  email_verified: false,
  created_at: '2026-07-29T00:00:00Z',
};

describe('SEC-003 registration country contract', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    apiMock.post.mockResolvedValue({
      data: {
        access_token: 'fixture-access-token',
        requires_verification: true,
        available_methods: ['email'],
      },
    });
    apiMock.put.mockResolvedValue({ data: {} });
    apiMock.get.mockResolvedValue({ data: userFixture });
  });

  it('normalizes the initial registration country and never postpones it to profile update', async () => {
    const store = useAuthStore();
    const success = await store.register(
      'fixture@example.test',
      'fixture-password',
      { country: ' ca ', username: 'fixture' },
      true,
      true,
    );

    expect(success).toBe(true);
    expect(apiMock.post).toHaveBeenCalledWith('/api/user/auth/register', expect.objectContaining({
      country: 'CA',
      agree_terms: true,
      age_confirm: true,
    }));
    expect(apiMock.put).toHaveBeenCalled();
    expect(apiMock.post.mock.invocationCallOrder[0]).toBeLessThan(
      apiMock.put.mock.invocationCallOrder[0],
    );
  });

  it('rejects a missing or malformed country before any registration request', async () => {
    expect(normalizeRegistrationCountry(' ir ')).toBe('IR');
    expect(normalizeRegistrationCountry('')).toBeNull();
    expect(normalizeRegistrationCountry('Iran')).toBeNull();

    const store = useAuthStore();
    await expect(store.register(
      'fixture@example.test',
      'fixture-password',
      { country: '' },
      true,
      true,
    )).resolves.toBe(false);
    expect(apiMock.post).not.toHaveBeenCalled();
  });
});

describe('SEC-003 credential transport regressions', () => {
  it('keeps registration country independent of language and provider choice', () => {
    const loginPage = readFileSync(new URL('../modules/user/views/LoginPage.vue', import.meta.url), 'utf8');
    expect(loginPage).toContain('formData.value.country');
    expect(loginPage).toContain('country: formData.value.country');
    expect(loginPage).not.toMatch(/country:\s*i18nStore\.locale/);
    expect(loginPage).not.toMatch(/Mailerino|Resend|KaveNegar/);
  });

  it('keeps reset handles in memory and out of URLs, storage, and logging', () => {
    const resetPage = readFileSync(new URL('../modules/user/views/ForgotPasswordPage.vue', import.meta.url), 'utf8');
    for (const sensitiveName of ['resetToken', 'passwordSetToken']) {
      expect(resetPage).not.toMatch(new RegExp(`localStorage\\.(?:setItem|getItem)\\([^\\n]*${sensitiveName}`, 'i'));
      expect(resetPage).not.toMatch(new RegExp(`(?:router\\.(?:push|replace)|URLSearchParams)[^\\n]*${sensitiveName}`, 'i'));
      expect(resetPage).not.toMatch(new RegExp(`console\\.[a-z]+\\([^\\n]*${sensitiveName}`, 'i'));
    }
  });

  it('preserves User/Admin and URL-token isolation', () => {
    const authStore = readFileSync(new URL('./auth.ts', import.meta.url), 'utf8');
    expect(authStore).toContain('/api/user/auth/refresh');
    expect(authStore).not.toContain('/api/admin');
    expect(authStore).not.toMatch(/[?&](?:token|access_token|jwt)=/);
  });
});