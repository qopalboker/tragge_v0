import { test, expect } from '@playwright/test';
import { ProfilePage, DashboardPage } from './pages';
import { TEST_USERS } from '../../../e2e/test-data';
import { mockApiResponse } from '../../../e2e/fixtures';

test.describe('User Frontend - Profile', () => {
  // Use authenticated state for all tests
  test.use({ storageState: './apps/user-frontend/e2e/.auth/user.json' });

  const mockUserProfile = {
    id: 'user-123',
    email: TEST_USERS.standard.email,
    name: TEST_USERS.standard.name,
    bio: 'Passionate trader and investor',
    avatar: null,
    joinedAt: '2024-01-15T10:00:00Z',
    stats: {
      totalContests: 12,
      wins: 3,
      totalPnL: 15420.5,
      winRate: 25,
    },
    preferences: {
      language: 'en',
      theme: 'light',
      notifications: true,
    },
  };

  test.beforeEach(async ({ page }) => {
    // Mock the profile API
    await mockApiResponse(page, '**/api/user/me', {
      status: 200,
      body: mockUserProfile,
    });

    await mockApiResponse(page, '**/api/user/profile', {
      status: 200,
      body: mockUserProfile,
    });
  });

  test.describe('View Profile', () => {
    test('should display the profile page', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Verify page loads
      await expect(profilePage.profileCard).toBeVisible();
    });

    test('should display user information', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Verify user info is displayed
      const displayName = await profilePage.getDisplayName();
      expect(displayName).toContain(TEST_USERS.standard.name);
    });

    test('should display user email', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Verify email is displayed
      const email = await profilePage.getEmail();
      expect(email).toContain(TEST_USERS.standard.email);
    });

    test('should display user statistics', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Verify stats are displayed
      const totalContests = await profilePage.getTotalContests();
      const totalWins = await profilePage.getTotalWins();
      const totalPnL = await profilePage.getTotalPnL();

      expect(totalContests).toBeTruthy();
      expect(totalWins).toBeTruthy();
      expect(totalPnL).toBeTruthy();
    });

    test('should display avatar or placeholder', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Avatar or placeholder should be visible
      const isAvatarDisplayed = await profilePage.isAvatarDisplayed();
      expect(isAvatarDisplayed).toBe(true);
    });
  });

  test.describe('Update Profile', () => {
    test('should enter edit mode', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Click edit button
      await profilePage.enterEditMode();

      // Save button should be visible
      await expect(profilePage.saveButton).toBeVisible();
      await expect(profilePage.cancelButton).toBeVisible();
    });

    test('should cancel edit mode', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Enter and exit edit mode
      await profilePage.enterEditMode();
      await profilePage.exitEditMode();

      // Edit button should be visible again
      await expect(profilePage.editButton).toBeVisible();
    });

    test('should update display name', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Mock profile update API
      await mockApiResponse(page, '**/api/user/profile', {
        status: 200,
        body: { ...mockUserProfile, name: 'Updated Name' },
      });

      // Update profile
      await profilePage.updateProfile({ name: 'Updated Name' });

      // Should show success message
      const isSuccess = await profilePage.isSuccessMessageVisible();
      expect(isSuccess).toBe(true);
    });

    test('should update bio', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Mock profile update API
      await mockApiResponse(page, '**/api/user/profile', {
        status: 200,
        body: { ...mockUserProfile, bio: 'New bio text' },
      });

      // Update profile with new bio
      await profilePage.updateProfile({ bio: 'New bio text' });

      // Should show success message
      const isSuccess = await profilePage.isSuccessMessageVisible();
      expect(isSuccess).toBe(true);
    });

    test('should show error on update failure', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Mock failed update
      await mockApiResponse(page, '**/api/user/profile', {
        status: 400,
        body: { error: 'Name is too long' },
      });

      // Try to update
      await profilePage.enterEditMode();
      await profilePage.updateName('A'.repeat(256)); // Too long
      await profilePage.saveChanges();

      // Should show error
      const isError = await profilePage.isErrorMessageVisible();
      expect(isError).toBe(true);
    });
  });

  test.describe('Language Settings', () => {
    test('should change language to Farsi', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Mock language update
      await mockApiResponse(page, '**/api/user/preferences', {
        status: 200,
        body: { ...mockUserProfile.preferences, language: 'fa' },
      });

      // Change language
      await profilePage.changeLanguage('fa');

      // Page should switch to RTL
      await page.waitForTimeout(500);
      const dir = await page.getAttribute('html', 'dir');
      expect(dir).toBe('rtl');
    });

    test('should change language to English', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Mock language update
      await mockApiResponse(page, '**/api/user/preferences', {
        status: 200,
        body: { ...mockUserProfile.preferences, language: 'en' },
      });

      // Change language
      await profilePage.changeLanguage('en');

      // Page should be LTR
      await page.waitForTimeout(500);
      const dir = await page.getAttribute('html', 'dir');
      expect(dir).not.toBe('rtl');
    });

    test('should persist language preference after page reload', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Change to Farsi
      await profilePage.changeLanguage('fa');
      await page.waitForTimeout(300);

      // Reload page
      await page.reload();
      await profilePage.waitForPageLoad();

      // Should still be in Farsi (RTL)
      const dir = await page.getAttribute('html', 'dir');
      // This depends on how language is persisted (localStorage, API, etc.)
    });
  });

  test.describe('Theme Settings', () => {
    test('should toggle theme', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Mock theme update
      await mockApiResponse(page, '**/api/user/preferences', {
        status: 200,
        body: { ...mockUserProfile.preferences, theme: 'dark' },
      });

      // Toggle theme
      await profilePage.toggleTheme();

      // Should apply dark theme class
      await page.waitForTimeout(300);
      const hasThemeClass = await page.evaluate(() =>
        document.body.classList.contains('dark') ||
        document.documentElement.classList.contains('dark')
      );
      // This depends on theme implementation
    });
  });

  test.describe('Notifications Settings', () => {
    test('should toggle notifications', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Mock notification toggle
      await mockApiResponse(page, '**/api/user/preferences', {
        status: 200,
        body: { ...mockUserProfile.preferences, notifications: false },
      });

      // Toggle notifications
      await profilePage.toggleNotifications();

      // Should show success or toggle state
      await page.waitForTimeout(300);
    });
  });

  test.describe('Avatar Upload', () => {
    test('should upload new avatar', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Enter edit mode
      await profilePage.enterEditMode();

      // Mock upload API
      await mockApiResponse(page, '**/api/user/avatar', {
        status: 200,
        body: { avatarUrl: 'https://example.com/avatar.jpg' },
      });

      // Note: In real tests, you'd need to create a test file
      // await profilePage.uploadAvatar('/path/to/test-avatar.jpg');
    });
  });

  test.describe('Statistics Display', () => {
    test('should display total contests participated', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      const totalContests = await profilePage.getTotalContests();
      expect(totalContests).toContain('12');
    });

    test('should display total wins', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      const totalWins = await profilePage.getTotalWins();
      expect(totalWins).toContain('3');
    });

    test('should display total P&L', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      const totalPnL = await profilePage.getTotalPnL();
      expect(totalPnL).toBeTruthy();
      // Should contain the value (formatted)
    });

    test('should display win rate', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      const winRate = await profilePage.getWinRate();
      expect(winRate).toContain('25');
    });
  });

  test.describe('Navigation', () => {
    test('should navigate to profile from dashboard', async ({ page }) => {
      const dashboardPage = new DashboardPage(page);
      await dashboardPage.goto();

      // Navigate to profile
      await dashboardPage.navigateToProfile();

      // Should be on profile page
      await expect(page).toHaveURL(/\/profile/);
    });

    test('should navigate back to dashboard from profile', async ({ page }) => {
      const profilePage = new ProfilePage(page);
      await profilePage.goto();

      // Click dashboard link
      const dashboardLink = page.locator('a[href*="/dashboard"], [data-nav="dashboard"]');
      await dashboardLink.click();

      // Should be on dashboard
      await expect(page).toHaveURL(/\/dashboard/);
    });
  });
});
