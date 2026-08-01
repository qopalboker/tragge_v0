import { test as setup, expect } from '@playwright/test';
import { TEST_USERS } from '../../../e2e/test-data';

const authFile = './apps/admin-frontend/e2e/.auth/admin.json';

/**
 * Authentication setup for admin tests
 * This runs before all admin tests and saves the auth state
 */
setup('authenticate as admin', async ({ page }) => {
  // Navigate to admin login page
  await page.goto('/admin/login');

  // Wait for the login form to be visible
  await expect(page.locator('input[type="email"]')).toBeVisible();

  // Fill in admin credentials
  await page.locator('input[type="email"]').fill(TEST_USERS.admin.email);
  await page.locator('input[type="password"]').fill(TEST_USERS.admin.password);

  // Submit the form
  await page.locator('button[type="submit"]').click();

  // Wait for successful navigation to admin dashboard
  await page.waitForURL(/\/admin\/(contests|dashboard|$)/, { timeout: 10000 });

  // Verify we're logged in as admin
  await expect(page.locator('.admin-layout, .main-content, .app-layout')).toBeVisible({ timeout: 5000 });

  // Save the authentication state
  await page.context().storageState({ path: authFile });
});
