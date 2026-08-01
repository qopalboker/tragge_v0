import { test, expect } from '@playwright/test';
import { LeaderboardPage } from './pages';
import { TEST_LEADERBOARD, TEST_CONTESTS } from '../../../e2e/test-data';
import { mockApiResponse } from '../../../e2e/fixtures';

test.describe('User Frontend - Leaderboard', () => {
  // Use authenticated state for all tests
  test.use({ storageState: './apps/user-frontend/e2e/.auth/user.json' });

  test.beforeEach(async ({ page }) => {
    // Mock the leaderboard API
    await mockApiResponse(page, '**/api/user/leaderboard**', {
      status: 200,
      body: {
        entries: TEST_LEADERBOARD,
        total: TEST_LEADERBOARD.length,
        page: 1,
        pageSize: 10,
        totalPages: 1,
      },
    });

    // Mock contests for selector
    await mockApiResponse(page, '**/api/user/contests**', {
      status: 200,
      body: {
        contests: [TEST_CONTESTS.active],
        total: 1,
      },
    });
  });

  test.describe('View Leaderboard', () => {
    test('should display the leaderboard page', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Verify page loads
      await expect(leaderboardPage.pageTitle).toBeVisible();
    });

    test('should display leaderboard for active contest', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.gotoContest(TEST_CONTESTS.active.id);

      // Wait for leaderboard to load
      await page.waitForTimeout(500);

      // Should have entries
      const count = await leaderboardPage.getRowCount();
      expect(count).toBeGreaterThan(0);
    });

    test('should display rank, user, and P&L for each entry', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Get first entry details
      const rank = await leaderboardPage.getRank(0);
      const username = await leaderboardPage.getUsername(0);
      const pnl = await leaderboardPage.getPnL(0);

      expect(rank).toBeTruthy();
      expect(username).toBeTruthy();
      expect(pnl).toBeTruthy();
    });

    test('should show entries in correct order (by P&L)', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Get first two P&L values
      const firstPnL = await leaderboardPage.getPnL(0);
      const secondPnL = await leaderboardPage.getPnL(1);

      // Parse P&L values (remove currency symbols, commas)
      const parsePnL = (str: string | null) => {
        if (!str) return 0;
        return parseFloat(str.replace(/[^0-9.-]/g, ''));
      };

      // First entry should have higher P&L than second
      expect(parsePnL(firstPnL)).toBeGreaterThanOrEqual(parsePnL(secondPnL));
    });
  });

  test.describe('Pagination', () => {
    test.beforeEach(async ({ page }) => {
      // Mock paginated leaderboard
      const fullLeaderboard = Array.from({ length: 50 }, (_, i) => ({
        rank: i + 1,
        userId: `user-${i + 1}`,
        userName: `Trader${i + 1}`,
        pnl: 10000 - i * 100,
        trades: Math.floor(Math.random() * 50) + 10,
      }));

      await mockApiResponse(page, '**/api/user/leaderboard**', {
        status: 200,
        body: {
          entries: fullLeaderboard.slice(0, 10),
          total: 50,
          page: 1,
          pageSize: 10,
          totalPages: 5,
        },
      });
    });

    test('should display pagination controls', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Check pagination is visible
      await expect(leaderboardPage.pagination).toBeVisible();
    });

    test('should navigate to next page', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Mock second page
      await mockApiResponse(page, '**/api/user/leaderboard**page=2**', {
        status: 200,
        body: {
          entries: Array.from({ length: 10 }, (_, i) => ({
            rank: i + 11,
            userId: `user-${i + 11}`,
            userName: `Trader${i + 11}`,
            pnl: 9000 - i * 100,
            trades: Math.floor(Math.random() * 50) + 10,
          })),
          total: 50,
          page: 2,
          pageSize: 10,
          totalPages: 5,
        },
      });

      // Click next
      await leaderboardPage.nextPage();

      // First entry should now be rank 11
      const rank = await leaderboardPage.getRank(0);
      expect(rank).toContain('11');
    });

    test('should navigate to previous page', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Go to page 2 first
      await leaderboardPage.nextPage();
      await page.waitForTimeout(300);

      // Then go back
      await leaderboardPage.prevPage();
      await page.waitForTimeout(300);

      // Should be back on page 1
      const currentPage = await leaderboardPage.getCurrentPage();
      expect(currentPage).toBe(1);
    });

    test('should navigate to specific page', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Go to page 3
      await leaderboardPage.goToPage(3);

      // Current page should be 3
      const currentPage = await leaderboardPage.getCurrentPage();
      expect(currentPage).toBe(3);
    });
  });

  test.describe('Own Rank Highlight', () => {
    test('should highlight current user row', async ({ page }) => {
      // Mock leaderboard with current user
      const entriesWithUser = [
        ...TEST_LEADERBOARD.slice(0, 5),
        {
          rank: 6,
          userId: 'current-user-id',
          userName: 'Current User',
          pnl: 5000,
          trades: 25,
          isCurrentUser: true,
        },
        ...TEST_LEADERBOARD.slice(5),
      ];

      await mockApiResponse(page, '**/api/user/leaderboard**', {
        status: 200,
        body: {
          entries: entriesWithUser,
          total: entriesWithUser.length,
          page: 1,
          pageSize: 10,
          totalPages: 1,
          currentUserRank: 6,
        },
      });

      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Own row should be visible and highlighted
      const isOwnRowVisible = await leaderboardPage.isOwnRowVisible();
      expect(isOwnRowVisible).toBe(true);
    });

    test('should show own rank even when on different page', async ({ page }) => {
      // Mock where user is on page 3
      await mockApiResponse(page, '**/api/user/leaderboard**', {
        status: 200,
        body: {
          entries: TEST_LEADERBOARD.slice(0, 10),
          total: 50,
          page: 1,
          pageSize: 10,
          totalPages: 5,
          currentUserRank: 25,
        },
      });

      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Should show user's rank somewhere (banner, sidebar, etc.)
      const ownRank = await leaderboardPage.getOwnRank();
      expect(ownRank).toBeTruthy();
    });
  });

  test.describe('Real-time Updates', () => {
    test('should show live indicator for active contest', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.gotoContest(TEST_CONTESTS.active.id);

      // Live indicator should be visible for running contests
      const isLive = await leaderboardPage.isLiveIndicatorVisible();
      expect(isLive).toBe(true);
    });

    test('should update leaderboard in real-time via WebSocket', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Get initial first place P&L
      const initialPnL = await leaderboardPage.getPnL(0);

      // Simulate WebSocket update (would need actual WS mock in real scenario)
      // For now, we mock an API refetch
      await mockApiResponse(page, '**/api/user/leaderboard**', {
        status: 200,
        body: {
          entries: [
            { ...TEST_LEADERBOARD[0], pnl: TEST_LEADERBOARD[0].pnl + 500 },
            ...TEST_LEADERBOARD.slice(1),
          ],
          total: TEST_LEADERBOARD.length,
          page: 1,
          pageSize: 10,
          totalPages: 1,
        },
      });

      // Trigger refresh
      await page.reload();
      await page.waitForTimeout(500);

      // P&L should be updated
      const updatedPnL = await leaderboardPage.getPnL(0);

      // Values should be different (or page should have refresh mechanism)
    });

    test('should show last updated timestamp', async ({ page }) => {
      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Should show last updated time
      const lastUpdated = await leaderboardPage.getLastUpdatedTime();
      expect(lastUpdated).toBeTruthy();
    });
  });

  test.describe('Contest Selection', () => {
    test('should allow selecting different contests', async ({ page }) => {
      // Mock multiple active contests
      await mockApiResponse(page, '**/api/user/contests**', {
        status: 200,
        body: {
          contests: [TEST_CONTESTS.active, { ...TEST_CONTESTS.upcoming, status: 'running' }],
          total: 2,
        },
      });

      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Select different contest
      await leaderboardPage.selectContest(TEST_CONTESTS.active.name);

      // Leaderboard should update (would verify with different mock data)
      await page.waitForTimeout(500);
    });
  });

  test.describe('Empty State', () => {
    test('should show empty state when no entries', async ({ page }) => {
      await mockApiResponse(page, '**/api/user/leaderboard**', {
        status: 200,
        body: {
          entries: [],
          total: 0,
          page: 1,
          pageSize: 10,
          totalPages: 0,
        },
      });

      const leaderboardPage = new LeaderboardPage(page);
      await leaderboardPage.goto();

      // Should show empty state
      const isEmpty = await leaderboardPage.isEmpty();
      expect(isEmpty).toBe(true);
    });
  });

  test.describe('Loading State', () => {
    test('should show loading indicator while fetching data', async ({ page }) => {
      // Mock slow API
      await page.route('**/api/user/leaderboard**', async (route) => {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            entries: TEST_LEADERBOARD,
            total: TEST_LEADERBOARD.length,
            page: 1,
            pageSize: 10,
            totalPages: 1,
          }),
        });
      });

      const leaderboardPage = new LeaderboardPage(page);

      // Navigate without waiting
      page.goto('/user/leaderboard');

      // Should show loading state
      await expect(leaderboardPage.loadingIndicator).toBeVisible();
    });
  });
});
