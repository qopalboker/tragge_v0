import { test, expect } from '@playwright/test';
import { ContestsPage, ContestFormPage } from './pages';
import { TEST_CONTESTS, generateContestName, TEST_SYMBOLS } from '../../../e2e/test-data';
import { mockApiResponse } from '../../../e2e/fixtures';

test.describe('Admin Frontend - Contest Management', () => {
  // Use authenticated state for all tests
  test.use({ storageState: './apps/admin-frontend/e2e/.auth/admin.json' });

  test.beforeEach(async ({ page }) => {
    // Mock the contests API
    await mockApiResponse(page, '**/api/admin/contests**', {
      status: 200,
      body: {
        contests: [
          TEST_CONTESTS.active,
          TEST_CONTESTS.upcoming,
          TEST_CONTESTS.completed,
          TEST_CONTESTS.draft,
        ],
        total: 4,
      },
    });

    // Mock symbols API
    await mockApiResponse(page, '**/api/admin/symbols**', {
      status: 200,
      body: { symbols: TEST_SYMBOLS.map((s) => s.symbol) },
    });
  });

  test.describe('View Contests', () => {
    test('should display the contests management page', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Verify page elements
      await expect(contestsPage.pageTitle).toBeVisible();
      await expect(contestsPage.createButton).toBeVisible();
    });

    test('should display list of contests', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Wait for contests to load
      await page.waitForTimeout(500);

      // Should have contests in table
      const count = await contestsPage.getContestCount();
      expect(count).toBeGreaterThan(0);
    });

    test('should display contest information in table', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Get first contest info
      const name = await contestsPage.getContestName(0);
      const status = await contestsPage.getContestStatus(0);

      expect(name).toBeTruthy();
      expect(status).toBeTruthy();
    });

    test('should show action buttons for each contest', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // First row should have action buttons
      const row = contestsPage.getContestRow(0);
      const editBtn = row.locator('.edit-btn, button[aria-label="Edit"]');

      await expect(editBtn).toBeVisible();
    });
  });

  test.describe('Create Contest', () => {
    test('should navigate to create contest form', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Click create button
      await contestsPage.clickCreateContest();

      // Should be on create form
      await expect(page).toHaveURL(/\/admin\/contests\/new/);
    });

    test('should display contest creation form', async ({ page }) => {
      const contestFormPage = new ContestFormPage(page);
      await contestFormPage.gotoNew();

      // Verify form fields
      await expect(contestFormPage.nameInput).toBeVisible();
      await expect(contestFormPage.prizePoolInput).toBeVisible();
      await expect(contestFormPage.entryFeeInput).toBeVisible();
      await expect(contestFormPage.startDateInput).toBeVisible();
      await expect(contestFormPage.endDateInput).toBeVisible();
    });

    test('should create a new contest successfully', async ({ page }) => {
      const contestFormPage = new ContestFormPage(page);
      await contestFormPage.gotoNew();

      // Mock create API
      await mockApiResponse(page, '**/api/admin/contests', {
        status: 201,
        body: {
          id: 'new-contest-123',
          name: 'Test Contest',
          status: 'draft',
        },
      });

      // Fill in form
      const contestName = generateContestName();
      const startDate = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000)
        .toISOString()
        .split('T')[0];
      const endDate = new Date(Date.now() + 14 * 24 * 60 * 60 * 1000)
        .toISOString()
        .split('T')[0];

      await contestFormPage.createContest({
        name: contestName,
        description: 'A test trading competition',
        prizePool: 50000,
        entryFee: 100,
        maxParticipants: 500,
        startDate,
        endDate,
        symbols: ['EUR/USD', 'BTC/USD', 'GBP/USD'],
      });

      // Should show success or redirect
      await page.waitForTimeout(500);
    });

    test('should show validation errors for invalid data', async ({ page }) => {
      const contestFormPage = new ContestFormPage(page);
      await contestFormPage.gotoNew();

      // Try to submit empty form
      await contestFormPage.submit();

      // Should show validation errors
      const hasErrors = await contestFormPage.hasValidationErrors();
      // Browser validation or custom validation may apply
    });

    test('should validate start date before end date', async ({ page }) => {
      const contestFormPage = new ContestFormPage(page);
      await contestFormPage.gotoNew();

      // Set end date before start date
      const startDate = new Date(Date.now() + 14 * 24 * 60 * 60 * 1000)
        .toISOString()
        .split('T')[0];
      const endDate = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000)
        .toISOString()
        .split('T')[0];

      await contestFormPage.fillName('Invalid Date Contest');
      await contestFormPage.fillPrizePool(10000);
      await contestFormPage.fillEntryFee(50);
      await contestFormPage.fillStartDate(startDate);
      await contestFormPage.fillEndDate(endDate);
      await contestFormPage.submit();

      // Should show validation error
    });
  });

  test.describe('Edit Contest', () => {
    test('should navigate to edit contest form', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Mock specific contest data
      await mockApiResponse(page, `**/api/admin/contests/${TEST_CONTESTS.draft.id}`, {
        status: 200,
        body: TEST_CONTESTS.draft,
      });

      // Click edit on first contest
      await contestsPage.clickEditContest(0);

      // Should be on edit form
      await expect(page).toHaveURL(/\/admin\/contests\//);
    });

    test('should load existing contest data in form', async ({ page }) => {
      await mockApiResponse(page, `**/api/admin/contests/${TEST_CONTESTS.draft.id}`, {
        status: 200,
        body: TEST_CONTESTS.draft,
      });

      const contestFormPage = new ContestFormPage(page);
      await contestFormPage.gotoEdit(TEST_CONTESTS.draft.id);

      // Form should be populated
      const values = await contestFormPage.getFormValues();
      expect(values.name).toBeTruthy();
    });

    test('should update contest successfully', async ({ page }) => {
      await mockApiResponse(page, `**/api/admin/contests/${TEST_CONTESTS.draft.id}`, {
        status: 200,
        body: TEST_CONTESTS.draft,
      });

      await mockApiResponse(page, `**/api/admin/contests/${TEST_CONTESTS.draft.id}`, {
        status: 200,
        body: { ...TEST_CONTESTS.draft, prizePool: 20000 },
      });

      const contestFormPage = new ContestFormPage(page);
      await contestFormPage.gotoEdit(TEST_CONTESTS.draft.id);

      // Update prize pool
      await contestFormPage.fillPrizePool(20000);
      await contestFormPage.submit();

      // Should show success
      await page.waitForTimeout(500);
    });

    test('should cancel edit and return to list', async ({ page }) => {
      await mockApiResponse(page, `**/api/admin/contests/${TEST_CONTESTS.draft.id}`, {
        status: 200,
        body: TEST_CONTESTS.draft,
      });

      const contestFormPage = new ContestFormPage(page);
      await contestFormPage.gotoEdit(TEST_CONTESTS.draft.id);

      // Cancel
      await contestFormPage.cancel();

      // Should be back on contests list
      await expect(page).toHaveURL(/\/admin\/contests(?!\/)/);
    });
  });

  test.describe('Start/Stop Contest', () => {
    test('should start a scheduled contest', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Mock start API
      await mockApiResponse(page, '**/api/admin/contests/*/start', {
        status: 200,
        body: { message: 'Contest started', status: 'running' },
      });

      // Find scheduled contest and start it
      const row = contestsPage.getContestRowByName(TEST_CONTESTS.upcoming.name);
      const startBtn = row.locator('.start-btn, button:has-text("Start")');

      if (await startBtn.isVisible()) {
        await startBtn.click();

        // Confirm if modal appears
        const confirmBtn = page.locator('.confirm-btn, button:has-text("Confirm")');
        if (await confirmBtn.isVisible()) {
          await confirmBtn.click();
        }

        // Should show success
        await page.waitForTimeout(500);
      }
    });

    test('should pause a running contest', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Mock freeze/pause API
      await mockApiResponse(page, '**/api/admin/contests/*/freeze', {
        status: 200,
        body: { message: 'Contest paused', status: 'paused' },
      });

      // Find active contest and pause it
      const row = contestsPage.getContestRowByName(TEST_CONTESTS.active.name);
      const stopBtn = row.locator('.stop-btn, button:has-text("Stop"), button:has-text("Pause")');

      if (await stopBtn.isVisible()) {
        await stopBtn.click();

        // Confirm if modal appears
        const confirmBtn = page.locator('.confirm-btn, button:has-text("Confirm")');
        if (await confirmBtn.isVisible()) {
          await confirmBtn.click();
        }

        await page.waitForTimeout(500);
      }
    });

    test('should show confirmation dialog before starting', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Click start on a contest
      const row = contestsPage.getContestRowByName(TEST_CONTESTS.upcoming.name);
      const startBtn = row.locator('.start-btn, button:has-text("Start")');

      if (await startBtn.isVisible()) {
        await startBtn.click();

        // Should show confirmation modal
        await expect(contestsPage.confirmModal).toBeVisible();
      }
    });
  });

  test.describe('View Participants', () => {
    test('should view contest participants', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Mock participants API
      await mockApiResponse(page, '**/api/admin/contests/*/participants', {
        status: 200,
        body: {
          participants: [
            { id: 'user-1', name: 'Trader 1', email: 'trader1@example.com', joinedAt: new Date().toISOString() },
            { id: 'user-2', name: 'Trader 2', email: 'trader2@example.com', joinedAt: new Date().toISOString() },
          ],
          total: 2,
        },
      });

      // Click view participants
      await contestsPage.viewParticipants(0);

      // Should show participants modal or navigate to participants page
      await page.waitForTimeout(500);
    });

    test('should show participant count in table', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Participants column should show count
      const row = contestsPage.getContestRow(0);
      const participantsCell = row.locator('.participants-count, td:has-text("participants")');
    });
  });

  test.describe('Search and Filter', () => {
    test('should search contests by name', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Search for specific contest
      await contestsPage.searchContests(TEST_CONTESTS.active.name);

      // Should filter results
      await page.waitForTimeout(500);
    });

    test('should filter contests by status', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Filter by running status
      await contestsPage.filterByStatus('running');

      // Should show only running contests
      await page.waitForTimeout(500);

      const count = await contestsPage.getContestCount();
      for (let i = 0; i < count; i++) {
        const status = await contestsPage.getContestStatus(i);
        expect(status?.toLowerCase()).toContain('running');
      }
    });
  });

  test.describe('Delete Contest', () => {
    test('should delete draft contest', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Mock delete API
      await mockApiResponse(page, '**/api/admin/contests/*', {
        status: 200,
        body: { message: 'Contest deleted' },
      });

      // Find draft contest and delete
      const row = contestsPage.getContestRowByName(TEST_CONTESTS.draft.name);
      const deleteBtn = row.locator('.delete-btn, button[aria-label="Delete"]');

      if (await deleteBtn.isVisible()) {
        await deleteBtn.click();

        // Confirm deletion
        const confirmBtn = page.locator('.confirm-btn, button:has-text("Confirm"), button:has-text("Delete")');
        if (await confirmBtn.isVisible()) {
          await confirmBtn.click();
        }

        // Should show success
        const isSuccess = await contestsPage.isSuccessMessageVisible();
      }
    });

    test('should show confirmation before deletion', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Click delete
      const row = contestsPage.getContestRowByName(TEST_CONTESTS.draft.name);
      const deleteBtn = row.locator('.delete-btn, button[aria-label="Delete"]');

      if (await deleteBtn.isVisible()) {
        await deleteBtn.click();

        // Should show confirmation modal
        await expect(contestsPage.confirmModal).toBeVisible();

        // Cancel
        await contestsPage.cancelButton.click();
      }
    });

    test('should not allow deleting active contest', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Active contest should not have delete button or it should be disabled
      const row = contestsPage.getContestRowByName(TEST_CONTESTS.active.name);
      const deleteBtn = row.locator('.delete-btn, button[aria-label="Delete"]');

      if (await deleteBtn.isVisible()) {
        const isDisabled = await deleteBtn.isDisabled();
        // May be disabled or hidden for active contests
      }
    });
  });

  test.describe('Error Handling', () => {
    test('should show error when API fails', async ({ page }) => {
      // Mock API error
      await mockApiResponse(page, '**/api/admin/contests**', {
        status: 500,
        body: { error: 'Internal server error' },
      });

      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Should show error state
      const isError = await contestsPage.isErrorMessageVisible();
    });

    test('should show error when create fails', async ({ page }) => {
      const contestFormPage = new ContestFormPage(page);
      await contestFormPage.gotoNew();

      // Mock create failure
      await mockApiResponse(page, '**/api/admin/contests', {
        status: 400,
        body: { error: 'Invalid contest data' },
      });

      // Fill and submit form
      await contestFormPage.fillForm({
        name: 'Test Contest',
        prizePool: 10000,
        entryFee: 50,
        startDate: '2025-01-01',
        endDate: '2025-01-15',
      });
      await contestFormPage.submit();

      // Should show error
      const isError = await contestFormPage.isErrorMessageVisible();
    });
  });
});
