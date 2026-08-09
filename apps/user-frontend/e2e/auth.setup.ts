import { mkdir } from 'node:fs/promises';
import { dirname } from 'node:path';

import { test as setup, expect } from '@playwright/test';

import { TEST_USERS } from '../../../e2e/test-data';
import {
  USER_AUTH_STATE_FILE,
  createMockAuthState,
  installMockAuthBackend,
} from './auth-mocks';
import { LoginPage } from './pages';

/**
 * Create a User-only storage state through the current User login contract.
 *
 * The mocked response installs the same context-specific refresh and session
 * hint cookie names used by the User BFF. No Admin route, fixture, cookie, or
 * credential participates in this setup.
 */
setup('authenticate as user', async ({ page }) => {
  const state = createMockAuthState();
  await installMockAuthBackend(page, state);

  const loginPage = new LoginPage(page);
  await loginPage.goto();
  await loginPage.loginAndWaitForDashboard(
    TEST_USERS.standard.email,
    TEST_USERS.standard.password,
  );

  await expect(page.locator('main')).toBeVisible();
  expect(state.loginRequests).toHaveLength(1);
  expect(state.loginRequests[0].path).toBe('/api/user/auth/login');
  expect(state.loginRequests[0].method).toBe('POST');

  await mkdir(dirname(USER_AUTH_STATE_FILE), { recursive: true });
  await page.context().storageState({ path: USER_AUTH_STATE_FILE });
});
