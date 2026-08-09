import { expect, test, type Page } from '@playwright/test';

const accessToken = [
  btoa(JSON.stringify({ alg: 'none', typ: 'JWT' })),
  btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 900 })),
  'sec007-fixture-signature',
].join('.');

async function mockCommon(page: Page): Promise<void> {
  await page.route('**/api/admin/healthz', (route) => route.fulfill({ status: 200, json: { status: 'ok' } }));
  await page.route('**/api/admin/me', (route) => route.fulfill({ status: 200, json: { user_id: 'admin-1', email: 'root@example.test', roles: ['super_admin'], created_at: '2026-08-09T00:00:00Z' } }));
  await page.route('**/api/admin/me/permissions', (route) => route.fulfill({ status: 200, json: { role: 'super_admin', permissions: ['users.edit'], is_viewer: false, is_admin: true, is_super_admin: true } }));
}

test('Super Admin enrolls and receives recovery codes only after TOTP', async ({ page }) => {
  await mockCommon(page);
  await page.route('**/api/admin/auth/login', (route) => route.fulfill({ status: 202, json: { mfa_required: true, enrollment_required: true, challenge: 'opaque-password-challenge', expires_at: '2026-08-09T00:05:00Z' } }));
  await page.route('**/api/admin/auth/mfa/enrollment/start', (route) => route.fulfill({ status: 200, json: { challenge: 'opaque-enrollment-challenge', secret: 'TESTONLYBASE32VALUE', provisioning_uri: 'otpauth://totp/Tragge%20Admin%3Aroot%40example.test?secret=TESTONLYBASE32VALUE&issuer=Tragge+Admin' } }));
  await page.route('**/api/admin/auth/mfa/enrollment/verify', async (route) => {
    const body = route.request().postDataJSON();
    expect(body).toEqual({ challenge: 'opaque-enrollment-challenge', code: '123456' });
    await route.fulfill({ status: 200, json: { access_token: accessToken, recovery_codes: ['SAFEONLY1-CODE0001', 'SAFEONLY2-CODE0002'] } });
  });

  await page.goto('/admin/login');
  await page.getByLabel('Email').fill('root@example.test');
  await page.getByLabel('Password').fill('local-test-password');
  await page.locator('button[type="submit"]').click();
  await expect(page.getByTestId('mfa-provisioning')).toBeVisible();
  await expect(page.locator('input[type="password"]')).toHaveCount(0);
  await page.getByLabel('Authenticator code').fill('123456');
  await page.locator('button[type="submit"]').click();
  await expect(page.getByTestId('mfa-recovery-codes')).toContainText('SAFEONLY1-CODE0001');

  const storage = await page.evaluate(() => ({ local: JSON.stringify(localStorage), session: JSON.stringify(sessionStorage), url: location.href }));
  expect(storage.local + storage.session + storage.url).not.toContain('opaque-password-challenge');
  expect(storage.local + storage.session + storage.url).not.toContain('opaque-enrollment-challenge');
  expect(storage.local + storage.session + storage.url).not.toContain('TESTONLYBASE32VALUE');
  expect(storage.local + storage.session + storage.url).not.toContain('SAFEONLY1-CODE0001');
});

test('Super Admin can choose a one-time recovery-code challenge without URL leakage', async ({ page }) => {
  await mockCommon(page);
  await page.route('**/api/admin/auth/login', (route) => route.fulfill({ status: 202, json: { mfa_required: true, enrollment_required: false, challenge: 'opaque-login-challenge' } }));
  await page.route('**/api/admin/auth/mfa/verify', async (route) => {
    expect(route.request().postDataJSON()).toEqual({ challenge: 'opaque-login-challenge', recovery_code: 'RECOVERY-ONE' });
    await route.fulfill({ status: 200, json: { access_token: accessToken } });
  });
  await page.goto('/admin/login');
  await page.getByLabel('Email').fill('root@example.test');
  await page.getByLabel('Password').fill('local-test-password');
  await page.locator('button[type="submit"]').click();
  await page.getByRole('button', { name: 'Use a recovery code' }).click();
  await page.getByLabel('Recovery code').fill('RECOVERY-ONE');
  await page.locator('button[type="submit"]').click();
  await expect(page).not.toHaveURL(/challenge|recovery|code/i);
  const persisted = await page.evaluate(() => JSON.stringify({ localStorage, sessionStorage }));
  expect(persisted).not.toContain('RECOVERY-ONE');
  expect(persisted).not.toContain('opaque-login-challenge');
});

test('invalid or expired MFA fails generically and the flow renders LTR and RTL', async ({ page }) => {
  await page.route('**/api/admin/healthz', (route) => route.fulfill({ status: 200, json: { status: 'ok' } }));
  await page.route('**/api/admin/auth/login', (route) => route.fulfill({ status: 202, json: { mfa_required: true, enrollment_required: false, challenge: 'opaque-expired-challenge' } }));
  await page.route('**/api/admin/auth/mfa/verify', (route) => route.fulfill({ status: 401, json: { error: 'additional authentication failed' } }));
  await page.goto('/admin/login');
  await expect(page.locator('.login-page')).not.toHaveClass(/rtl/);
  await page.getByLabel('Email').fill('root@example.test');
  await page.getByLabel('Password').fill('local-test-password');
  await page.locator('button[type="submit"]').click();
  await page.getByLabel('Authenticator code').fill('000000');
  await page.locator('button[type="submit"]').click();
  await expect(page.locator('.error-message')).toBeVisible();
  await expect(page.locator('.error-message')).not.toContainText('opaque-expired-challenge');
  await page.locator('.lang-toggle').click();
  await expect(page.locator('.login-page')).toHaveClass(/rtl/);
  await expect(page.locator('html')).toHaveAttribute('dir', 'rtl');
});

test('MFA-upgraded Super Admin can perform an authorized audited reset flow', async ({ page }) => {
  await mockCommon(page);
  await page.route('**/api/admin/auth/login', (route) => route.fulfill({ status: 202, json: { mfa_required: true, enrollment_required: false, challenge: 'opaque-normal-challenge' } }));
  await page.route('**/api/admin/auth/mfa/verify', async (route) => {
    expect(route.request().postDataJSON()).toEqual({ challenge: 'opaque-normal-challenge', code: '654321' });
    await route.fulfill({ status: 200, json: { access_token: accessToken } });
  });
  await page.route('**/api/admin/users/admin-target', (route) => route.fulfill({
    status: 200,
    json: {
      user: { id: 'admin-target', email: 'target@example.test', created_at: '2026-08-09T00:00:00Z', email_verified: true, status: 'active' },
      roles: ['super_admin'], kyc: { status: 'none' }, wallet: { balance_cents: 0, currency: 'USD', status: 'active' },
      stats: { total_contests: 0, total_wins: 0, tragge_point: 0, total_trades: 0, total_pnl: 0 },
      recent_contests: [], recent_transactions: [], affiliate: { status: 'inactive', total_referrals: 0, total_earned: 0 }, sessions: [],
    },
  }));
  await page.route('**/api/admin/reauthenticate', async (route) => {
    expect(route.request().postDataJSON()).toEqual({ password: 'current-admin-password', action: 'admin.mfa.reset', resource_id: 'admin-target' });
    await route.fulfill({ status: 200, json: { grant: 'opaque-action-grant', expires_at: '2026-08-09T00:05:00Z' } });
  });
  let resetObserved = false;
  await page.route('**/api/admin/users/admin-target/mfa/reset', async (route) => {
    expect(route.request().postDataJSON()).toEqual({ reason: 'account recovery approved' });
    expect(route.request().headers()['x-admin-reauth-grant']).toBe('opaque-action-grant');
    resetObserved = true;
    await route.fulfill({ status: 200, json: { status: 'reset' } });
  });

  await page.goto('/admin/login');
  await page.getByLabel('Email').fill('root@example.test');
  await page.getByLabel('Password').fill('local-test-password');
  await page.locator('button[type="submit"]').click();
  await page.getByLabel('Authenticator code').fill('654321');
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(1100);
  await page.evaluate(async () => {
    const router = (await import('/src/router/index.ts')).default;
    await router.push('/admin/users/admin-target');
  });
  await expect(page.getByTestId('reset-super-admin-mfa')).toBeVisible();
  const answers = ['account recovery approved', 'current-admin-password'];
  page.on('dialog', async (dialog) => dialog.accept(answers.shift() ?? ''));
  await page.getByTestId('reset-super-admin-mfa').click();
  await expect.poll(() => resetObserved).toBe(true);
  const leakage = await page.evaluate(() => JSON.stringify({ localStorage, sessionStorage, href: location.href }));
  expect(leakage).not.toContain('opaque-action-grant');
  expect(leakage).not.toContain('current-admin-password');
});
