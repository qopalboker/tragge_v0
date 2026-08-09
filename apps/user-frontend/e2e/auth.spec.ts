import { expect, test as base, type Page } from '@playwright/test';

import { TEST_USERS, generateTestEmail } from '../../../e2e/test-data';
import { mockApiResponse } from '../../../e2e/fixtures';
import {
  SYNTHETIC_OTP_CODE,
  USER_AUTH_STATE_FILE,
  createMockAuthState,
  installCaptchaMock,
  installMockAuthBackend,
  sensitiveBrowserValues,
  type MockAuthState,
} from './auth-mocks';
import { DashboardPage, LoginPage } from './pages';

type AuthFixtures = {
  initialSession: boolean;
  mockAuth: MockAuthState;
};

const test = base.extend<AuthFixtures>({
  initialSession: [false, { option: true }],
  mockAuth: async ({ page, initialSession }, use) => {
    const state = createMockAuthState({ sessionValid: initialSession });
    await installCaptchaMock(page);
    await installMockAuthBackend(page, state);
    await use(state);

    const output = state.browserOutput.join('\n');
    if (sensitiveBrowserValues(state).some((value) => output.includes(value))) {
      throw new Error('browser output contained a controlled sensitive fixture value');
    }
  },
});

async function fillRegistration(page: Page, email: string): Promise<void> {
  await page.locator('.toggle-link').click();
  const form = page.locator('.auth-form.signup');
  await expect(form).toBeVisible();

  const rows = form.locator('.form-row');
  await rows.nth(0).locator('input').nth(0).fill('Synthetic');
  await rows.nth(0).locator('input').nth(1).fill('User');
  await rows.nth(1).locator('select').selectOption('US');
  await rows.nth(1).locator('input').fill('syntheticuser');
  await rows.nth(2).locator('input[type="email"]').fill(email);
  await rows.nth(3).locator('input').nth(0).fill(TEST_USERS.newUser.password);
  await rows.nth(3).locator('input').nth(1).fill(TEST_USERS.newUser.password);
  await form.locator('.checkbox-label').nth(0).click();
  await form.locator('.checkbox-label').nth(1).click();
  await form.locator('button[type="submit"]').click();
}

async function enterOtp(page: Page, selector: string): Promise<void> {
  const inputs = page.locator(selector);
  await expect(inputs).toHaveCount(6);
  for (const [index, digit] of [...SYNTHETIC_OTP_CODE].entries()) {
    await inputs.nth(index).fill(digit);
  }
}

async function completePasswordReset(page: Page): Promise<void> {
  await page.goto('/user/forgot-password');
  await page.locator('#identifier').fill(TEST_USERS.standard.email);
  await page.locator('form button[type="submit"]').click();
  await expect(page.locator('.otp-container')).toBeVisible();
  await enterOtp(page, '.otp-container .otp-input');
  await expect(page.locator('#new-password')).toBeVisible();
  await page.locator('#new-password').fill('ChangedPassword123!');
  await page.locator('#confirm-password').fill('ChangedPassword123!');
  await page.locator('form button[type="submit"]').click();
  await expect(page.locator('.success-card')).toBeVisible();
}

test.describe('User authentication module contract', () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test('loads shared ESM helpers and both User page objects', async ({ page }) => {
    expect(LoginPage).toBeDefined();
    expect(DashboardPage).toBeDefined();
    expect(typeof mockApiResponse).toBe('function');
    expect(generateTestEmail('module-contract')).toMatch(
      /^module-contract-[^-]+-[a-z0-9]+@example\.com$/,
    );
    expect(new LoginPage(page)).toBeInstanceOf(LoginPage);
    expect(new DashboardPage(page)).toBeInstanceOf(DashboardPage);
  });
});

test.describe('Anonymous User authentication journeys', () => {
  test.use({
    storageState: { cookies: [], origins: [] },
    initialSession: false,
  });

  test('displays the current User login form', async ({ page }) => {
    const loginPage = new LoginPage(page);
    await loginPage.goto();

    await expect(loginPage.logo).toBeVisible();
    await expect(loginPage.loginTitle).toBeVisible();
    await expect(loginPage.emailInput).toBeVisible();
    await expect(loginPage.passwordInput).toBeVisible();
    await expect(loginPage.submitButton).toBeVisible();
    await expect(loginPage.forgotPasswordLink).toBeVisible();
    await expect(loginPage.registerLink).toBeVisible();
    await expect(loginPage.languageToggle).toBeVisible();
  });

  test('logs in through the current User endpoint and reaches the dashboard', async ({
    page,
    mockAuth,
  }) => {
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.loginAndWaitForDashboard(
      TEST_USERS.standard.email,
      TEST_USERS.standard.password,
    );

    await expect(page.locator('main')).toBeVisible();
    expect(mockAuth.loginRequests).toHaveLength(1);
    expect(mockAuth.loginRequests[0]).toMatchObject({
      method: 'POST',
      path: '/api/user/auth/login',
    });
  });

  test('rejects invalid login without creating a session', async ({ page, mockAuth }) => {
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(TEST_USERS.standard.email, 'IncorrectPassword123!');

    await expect(loginPage.errorMessage).toBeVisible();
    await expect(page).toHaveURL(/\/user\/login/);
    expect(mockAuth.sessionValid).toBe(false);
  });

  test('preserves password visibility and form validation behavior', async ({ page }) => {
    const loginPage = new LoginPage(page);
    await loginPage.goto();

    expect(await loginPage.isSubmitEnabled()).toBe(false);
    await loginPage.fillEmail('invalid-email');
    await loginPage.fillPassword(TEST_USERS.standard.password);
    expect(await loginPage.emailInput.evaluate((input: HTMLInputElement) => input.validity.valid)).toBe(false);
    await loginPage.fillEmail(TEST_USERS.standard.email);
    expect(await loginPage.isSubmitEnabled()).toBe(true);
    await expect(loginPage.passwordInput).toHaveAttribute('type', 'password');
    await loginPage.togglePasswordVisibility();
    await expect(page.locator('.auth-form:not(.signup) input[type="text"]')).toHaveValue(
      TEST_USERS.standard.password,
    );
  });

  test('toggles the login page into Persian RTL mode', async ({ page }) => {
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await page.locator('.lang-btn', { hasText: 'FA' }).click();
    expect(await loginPage.isRtl()).toBe(true);
  });

  test('registers with the current contract and opens email ownership verification', async ({
    page,
    mockAuth,
  }) => {
    const email = generateTestEmail('registration');
    await page.goto('/user/login');
    await fillRegistration(page, email);

    await expect(page.locator('.method-card .email-icon')).toBeVisible();
    expect(mockAuth.registrationRequests).toHaveLength(1);
    expect(mockAuth.registrationRequests[0]).toMatchObject({
      method: 'POST',
      path: '/api/user/auth/register',
    });
    expect(mockAuth.registrationRequests[0].body).toMatchObject({
      email,
      country: 'US',
      agree_terms: true,
      age_confirm: true,
    });
  });

  test('verifies registration email ownership through the current OTP contract', async ({
    page,
    mockAuth,
  }) => {
    await page.goto('/user/login');
    await fillRegistration(page, generateTestEmail('email-ownership'));

    await page.locator('.method-card', { has: page.locator('.email-icon') }).click();
    await expect(page.locator('.modal-content .otp-group')).toBeVisible();
    await enterOtp(page, '.modal-content .otp-input');
    await expect(page.locator('.continue-btn')).toBeVisible();
    await page.locator('.continue-btn').click();

    await expect(page).toHaveURL(/\/user\/dashboard/);
    expect(mockAuth.verificationSendRequests).toHaveLength(1);
    expect(mockAuth.verificationSendRequests[0].body).toEqual({ method: 'email' });
    expect(mockAuth.verificationRequests).toHaveLength(1);
    expect(mockAuth.emailVerified).toBe(true);
  });

  test('requests password reset through the current anti-enumeration flow', async ({
    page,
    mockAuth,
  }) => {
    await page.goto('/user/forgot-password');
    await page.locator('#identifier').fill(TEST_USERS.standard.email);
    await page.locator('form button[type="submit"]').click();

    await expect(page.locator('.otp-container')).toBeVisible();
    expect(mockAuth.resetRequests[0]).toMatchObject({
      method: 'POST',
      path: '/api/user/auth/forgot-password/request',
    });
    expect(mockAuth.resetRequests[0].body).toHaveProperty('captcha_token');
  });

  test('verifies reset OTP and displays the new-password step', async ({ page, mockAuth }) => {
    await page.goto('/user/forgot-password');
    await page.locator('#identifier').fill(TEST_USERS.standard.email);
    await page.locator('form button[type="submit"]').click();
    await enterOtp(page, '.otp-container .otp-input');

    await expect(page.locator('#new-password')).toBeVisible();
    expect(mockAuth.resetRequests.map((request) => request.path)).toContain(
      '/api/user/auth/forgot-password/verify',
    );
  });

  test('completes the password update without putting reset handles in the URL', async ({
    page,
    mockAuth,
  }) => {
    await completePasswordReset(page);

    expect(mockAuth.resetRequests.map((request) => request.path)).toEqual([
      '/api/user/auth/forgot-password/request',
      '/api/user/auth/forgot-password/verify',
      '/api/user/auth/forgot-password/reset',
    ]);
    expect(page.url()).not.toMatch(/[?&](?:token|access_token|jwt|reset_token)=/i);
  });
});

test.describe('Authenticated User session journeys', () => {
  test.use({
    storageState: USER_AUTH_STATE_FILE,
    initialSession: true,
  });

  test('refreshes the current User session and preserves it across reload', async ({
    page,
    mockAuth,
  }) => {
    const dashboard = new DashboardPage(page);
    await dashboard.goto();
    await expect(dashboard.mainContent).toBeVisible();
    expect(mockAuth.refreshRequests).toBe(1);

    await page.reload();
    await expect(page).toHaveURL(/\/user\/dashboard/);
    await expect(dashboard.mainContent).toBeVisible();
    expect(mockAuth.refreshRequests).toBe(2);
  });

  test('logout clears only the User session and protects the dashboard', async ({
    page,
    mockAuth,
  }) => {
    const dashboard = new DashboardPage(page);
    await dashboard.goto();
    await dashboard.logout();

    await expect(page).toHaveURL(/\/user\/login/);
    await expect.poll(() => mockAuth.logoutRequests).toBe(1);
    expect(mockAuth.sessionValid).toBe(false);
    await page.goto('/user/dashboard');
    await expect(page).toHaveURL(/\/user\/login/);
  });

  test('rejects the old User session after password reset completes', async ({
    browser,
  }, testInfo) => {
    const state = createMockAuthState({ sessionValid: true });
    const baseURL = String(testInfo.project.use.baseURL);
    const oldSession = await browser.newContext({
      baseURL,
      storageState: USER_AUTH_STATE_FILE,
    });
    const resetSession = await browser.newContext({
      baseURL,
      storageState: { cookies: [], origins: [] },
    });
    const oldPage = await oldSession.newPage();
    const resetPage = await resetSession.newPage();

    try {
      await installMockAuthBackend(oldPage, state);
      await installCaptchaMock(resetPage);
      await installMockAuthBackend(resetPage, state);

      await oldPage.goto('/user/dashboard');
      await expect(oldPage.locator('main')).toBeVisible();
      await completePasswordReset(resetPage);
      expect(state.sessionValid).toBe(false);

      await oldPage.reload();
      await expect(oldPage).toHaveURL(/\/user\/login/);
    } finally {
      await oldSession.close();
      await resetSession.close();
    }
  });
});
