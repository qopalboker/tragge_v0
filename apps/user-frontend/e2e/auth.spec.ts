import { test, expect, Page } from '@playwright/test';
import { LoginPage, DashboardPage } from './pages';
import { TEST_USERS, generateTestEmail } from '../../../e2e/test-data';
import { mockApiResponse } from '../../../e2e/fixtures';

test.describe('User Frontend - Authentication', () => {
  test.describe('Login', () => {
    test('should display login form correctly', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Verify all form elements are present
      await expect(loginPage.logo).toBeVisible();
      await expect(loginPage.loginTitle).toBeVisible();
      await expect(loginPage.emailInput).toBeVisible();
      await expect(loginPage.passwordInput).toBeVisible();
      await expect(loginPage.submitButton).toBeVisible();
      await expect(loginPage.forgotPasswordLink).toBeVisible();
      await expect(loginPage.registerLink).toBeVisible();
      await expect(loginPage.languageToggle).toBeVisible();
    });

    test('should login with valid credentials', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Mock the API response for login
      await mockApiResponse(page, '**/api/user/login', {
        status: 200,
        body: {
          access_token: 'mock-jwt-token',
          refresh_token: 'mock-refresh-token',
          user: {
            id: 'user-123',
            email: TEST_USERS.standard.email,
            name: TEST_USERS.standard.name,
          },
        },
      });

      // Perform login
      await loginPage.login(TEST_USERS.standard.email, TEST_USERS.standard.password);

      // Verify navigation to dashboard
      await expect(page).toHaveURL(/\/user\/(dashboard|$)/);
    });

    test('should show error message with invalid credentials', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Mock the API response for failed login
      await mockApiResponse(page, '**/api/user/login', {
        status: 401,
        body: {
          error: 'Invalid email or password',
        },
      });

      // Attempt login with wrong password
      await loginPage.login(TEST_USERS.standard.email, 'wrongpassword');

      // Verify error message is displayed
      await expect(loginPage.errorMessage).toBeVisible();
      const errorText = await loginPage.getErrorMessage();
      expect(errorText).toBeTruthy();
    });

    test('should toggle password visibility', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Initially password should be hidden
      await expect(loginPage.passwordInput).toHaveAttribute('type', 'password');

      // Click toggle to show password
      await loginPage.togglePasswordVisibility();
      await expect(loginPage.passwordInput).toHaveAttribute('type', 'text');

      // Click toggle again to hide password
      await loginPage.togglePasswordVisibility();
      await expect(loginPage.passwordInput).toHaveAttribute('type', 'password');
    });

    test('should disable submit button when form is empty', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Submit button should be disabled initially
      const isEnabled = await loginPage.isSubmitEnabled();
      expect(isEnabled).toBe(false);
    });

    test('should enable submit button when form is filled', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Fill in the form
      await loginPage.fillEmail(TEST_USERS.standard.email);
      await loginPage.fillPassword(TEST_USERS.standard.password);

      // Submit button should be enabled
      const isEnabled = await loginPage.isSubmitEnabled();
      expect(isEnabled).toBe(true);
    });

    test('should show loading state during login', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Mock slow API response
      await page.route('**/api/user/login', async (route) => {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            access_token: 'mock-token',
            refresh_token: 'mock-refresh',
          }),
        });
      });

      // Fill form and submit
      await loginPage.fillEmail(TEST_USERS.standard.email);
      await loginPage.fillPassword(TEST_USERS.standard.password);
      await loginPage.submit();

      // Check for loading state
      await expect(loginPage.loadingSpinner).toBeVisible();
    });
  });

  test.describe('Session Persistence', () => {
    // Use authenticated state
    test.use({ storageState: './apps/user-frontend/e2e/.auth/user.json' });

    test('should persist session after page refresh', async ({ page }) => {
      const dashboardPage = new DashboardPage(page);
      await dashboardPage.goto();

      // Verify we're on the dashboard
      await expect(dashboardPage.mainContent).toBeVisible();

      // Refresh the page
      await page.reload();

      // Should still be on dashboard (not redirected to login)
      await expect(page).not.toHaveURL(/\/login/);
      await expect(dashboardPage.mainContent).toBeVisible();
    });

    test('should redirect to login after logout', async ({ page }) => {
      const dashboardPage = new DashboardPage(page);
      await dashboardPage.goto();

      // Mock logout API
      await mockApiResponse(page, '**/api/user/logout', {
        status: 200,
        body: { message: 'Logged out successfully' },
      });

      // Perform logout
      await dashboardPage.logout();

      // Should be redirected to login
      await expect(page).toHaveURL(/\/login/);
    });
  });

  test.describe('Logout Flow', () => {
    test.use({ storageState: './apps/user-frontend/e2e/.auth/user.json' });

    test('should logout and clear session', async ({ page }) => {
      const dashboardPage = new DashboardPage(page);
      await dashboardPage.goto();

      // Mock logout API
      await mockApiResponse(page, '**/api/user/logout', {
        status: 200,
        body: { message: 'Logged out' },
      });

      // Logout
      await dashboardPage.logout();

      // Verify redirected to login
      await expect(page).toHaveURL(/\/login/);

      // Try to access protected route
      await page.goto('/user/dashboard');

      // Should redirect back to login
      await expect(page).toHaveURL(/\/login/);
    });
  });

  test.describe('Password Reset Flow', () => {
    test('should navigate to forgot password from login page', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Click forgot password link
      await loginPage.clickForgotPassword();

      await expect(page).toHaveURL(/\/forgot-password/);
    });

    test('should show step 1: identifier input', async ({ page }) => {
      await page.goto('/user/forgot-password');

      // New flow uses text input for username/phone/email
      const identifierInput = page.locator('#identifier');
      const submitButton = page.locator('button[type="submit"]');

      await expect(identifierInput).toBeVisible();
      await expect(submitButton).toBeVisible();
    });

    test('should submit identifier and move to OTP step', async ({ page }) => {
      await page.goto('/user/forgot-password');

      // Mock the new OTP request endpoint
      await mockApiResponse(page, '**/api/user/auth/forgot-password/request', {
        status: 200,
        body: {
          message: 'code sent',
          reset_token: 'test-reset-token',
          channel_hint: 'sms',
          masked_destination: '0912***6789',
        },
      });

      // Fill identifier (can be username, phone, or email)
      await page.locator('#identifier').fill(TEST_USERS.standard.email);
      await page.locator('button[type="submit"]').click();

      // Should show OTP input (step 2)
      await expect(page.locator('.otp-container')).toBeVisible();
    });

    test('should verify OTP and move to password step', async ({ page }) => {
      await page.goto('/user/forgot-password');

      // Mock step 1
      await mockApiResponse(page, '**/api/user/auth/forgot-password/request', {
        status: 200,
        body: {
          message: 'code sent',
          reset_token: 'test-reset-token',
          channel_hint: 'sms',
          masked_destination: '0912***6789',
        },
      });

      await page.locator('#identifier').fill(TEST_USERS.standard.email);
      await page.locator('button[type="submit"]').click();
      await expect(page.locator('.otp-container')).toBeVisible();

      // Mock step 2
      await mockApiResponse(page, '**/api/user/auth/forgot-password/verify', {
        status: 200,
        body: { password_set_token: 'test-set-token' },
      });

      // Fill OTP digits
      const otpInputs = page.locator('.otp-input');
      for (let i = 0; i < 6; i++) {
        await otpInputs.nth(i).fill(String(i + 1));
      }

      // Should move to step 3 (new password)
      await expect(page.locator('#new-password')).toBeVisible();
    });

    test('should set new password and redirect to login', async ({ page }) => {
      await page.goto('/user/forgot-password');

      // Mock step 1
      await mockApiResponse(page, '**/api/user/auth/forgot-password/request', {
        status: 200,
        body: {
          message: 'code sent',
          reset_token: 'test-reset-token',
          channel_hint: 'sms',
          masked_destination: '0912***6789',
        },
      });

      await page.locator('#identifier').fill(TEST_USERS.standard.email);
      await page.locator('button[type="submit"]').click();

      // Mock step 2
      await mockApiResponse(page, '**/api/user/auth/forgot-password/verify', {
        status: 200,
        body: { password_set_token: 'test-set-token' },
      });

      const otpInputs = page.locator('.otp-input');
      for (let i = 0; i < 6; i++) {
        await otpInputs.nth(i).fill(String(i + 1));
      }

      await expect(page.locator('#new-password')).toBeVisible();

      // Mock step 3
      await mockApiResponse(page, '**/api/user/auth/forgot-password/reset', {
        status: 200,
        body: { message: 'Password reset successful' },
      });

      // Fill new password and confirm
      await page.locator('#new-password').fill('NewStr0ng!Pass');
      await page.locator('#confirm-password').fill('NewStr0ng!Pass');
      await page.locator('button[type="submit"]').click();

      // Should show success state and redirect to login
      await expect(page.locator('.success-card, .success-icon')).toBeVisible();
    });
  });

  test.describe('Language Toggle', () => {
    test('should toggle language on login page', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Get initial language toggle text
      const initialText = await loginPage.getLanguageToggleText();
      expect(initialText).toBeTruthy();

      // Toggle language
      await loginPage.toggleLanguage();

      // Wait for re-render
      await page.waitForTimeout(300);

      // Language toggle text should change
      const newText = await loginPage.getLanguageToggleText();
      expect(newText).not.toBe(initialText);
    });

    test('should switch to RTL mode for Farsi', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // If starting in English, toggle to Farsi
      const initialText = await loginPage.getLanguageToggleText();
      if (initialText?.includes('فارسی')) {
        await loginPage.toggleLanguage();
      }

      // Verify RTL mode
      const isRtl = await loginPage.isRtl();
      expect(isRtl).toBe(true);
    });
  });

  test.describe('Form Validation', () => {
    test('should validate email format', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Enter invalid email
      await loginPage.fillEmail('invalid-email');
      await loginPage.fillPassword('password123');
      await loginPage.submit();

      // Check for validation (either browser validation or custom)
      const emailInput = loginPage.emailInput;
      const isInvalid = await emailInput.evaluate(
        (el: HTMLInputElement) => !el.validity.valid
      );
      expect(isInvalid).toBe(true);
    });

    test('should require password', async ({ page }) => {
      const loginPage = new LoginPage(page);
      await loginPage.goto();

      // Enter email but no password
      await loginPage.fillEmail(TEST_USERS.standard.email);

      // Submit should be disabled or form should not submit
      const isEnabled = await loginPage.isSubmitEnabled();
      expect(isEnabled).toBe(false);
    });
  });
});
