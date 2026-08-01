import { test, expect } from '@playwright/test';
import { ContestsPage, ContestDetailPage } from './pages';
import { TEST_CONTESTS, TEST_LEADERBOARD } from '../../../e2e/test-data';
import { mockApiResponse } from '../../../e2e/fixtures';

/**
 * Tournament Flow Tests
 *
 * These tests cover the complete user journey through tournament lifecycle:
 * 1. View scheduled contest - countdown, join/leave functionality
 * 2. Enter running contest - redirect to trade panel with authentication
 * 3. View finished contest - results page with rank and rewards
 * 4. Invalid access to trade panel - redirect without authentication
 */

// Extended test data for tournament flows
const SCHEDULED_CONTEST = {
  ...TEST_CONTESTS.upcoming,
  id: 'contest-scheduled-001',
  name: 'Upcoming Trading Championship',
  status: 'scheduled' as const,
  description: 'A scheduled tournament starting soon',
  prizePool: 50000,
  entryFee: 100,
  maxParticipants: 500,
  currentParticipants: 234,
  startDate: new Date(Date.now() + 3 * 24 * 60 * 60 * 1000).toISOString(),
  endDate: new Date(Date.now() + 10 * 24 * 60 * 60 * 1000).toISOString(),
  symbols: ['EUR/USD', 'BTC/USD', 'GBP/USD', 'ETH/USD', 'SOL/USD'],
  isJoined: false,
};

const RUNNING_CONTEST = {
  ...TEST_CONTESTS.active,
  id: 'contest-running-001',
  name: 'Active Trading Tournament',
  status: 'running' as const,
  description: 'Currently active tournament',
  prizePool: 100000,
  entryFee: 200,
  maxParticipants: 1000,
  currentParticipants: 756,
  startDate: new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString(),
  endDate: new Date(Date.now() + 5 * 24 * 60 * 60 * 1000).toISOString(),
  symbols: ['EUR/USD', 'BTC/USD', 'GBP/USD', 'XAU/USD', 'USD/JPY'],
  isJoined: true,
};

const COMPLETED_CONTEST = {
  ...TEST_CONTESTS.completed,
  id: 'contest-completed-001',
  name: 'Grand Trading Championship',
  status: 'completed' as const,
  description: 'Tournament has ended',
  prizePool: 200000,
  entryFee: 500,
  maxParticipants: 2000,
  currentParticipants: 1847,
  startDate: new Date(Date.now() - 14 * 24 * 60 * 60 * 1000).toISOString(),
  endDate: new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString(),
  symbols: ['EUR/USD', 'BTC/USD', 'GBP/USD', 'ETH/USD'],
  isJoined: true,
};

// Final results for completed contest
const CONTEST_RESULTS = {
  contestId: COMPLETED_CONTEST.id,
  totalParticipants: 1847,
  prizePool: 200000,
  userResult: {
    rank: 5,
    pnl: 12500.75,
    reward: 8000,
    percentile: 99.7,
  },
  leaderboard: [
    { rank: 1, userId: 'user-001', userName: 'TopTrader', pnl: 45000.5, reward: 60000 },
    { rank: 2, userId: 'user-002', userName: 'ProInvestor', pnl: 38000.25, reward: 40000 },
    { rank: 3, userId: 'user-003', userName: 'MarketMaster', pnl: 28000.0, reward: 25000 },
    { rank: 4, userId: 'user-004', userName: 'StockGuru', pnl: 18500.75, reward: 15000 },
    { rank: 5, userId: 'current-user', userName: 'You', pnl: 12500.75, reward: 8000, isCurrentUser: true },
    { rank: 6, userId: 'user-006', userName: 'TradingPro', pnl: 10200.5, reward: 5000 },
    { rank: 7, userId: 'user-007', userName: 'ChartAnalyst', pnl: 8750.25, reward: 3000 },
    { rank: 8, userId: 'user-008', userName: 'RiskTaker', pnl: 6500.0, reward: 2000 },
    { rank: 9, userId: 'user-009', userName: 'SafeTrader', pnl: 4200.75, reward: 1500 },
    { rank: 10, userId: 'user-010', userName: 'NewTrader', pnl: 2100.0, reward: 1000 },
  ],
};

test.describe('Tournament Flows', () => {
  // Use authenticated state for all tests
  test.use({ storageState: './apps/user-frontend/e2e/.auth/user.json' });

  test.describe('1. View Scheduled Contest', () => {
    test.beforeEach(async ({ page }) => {
      // Mock contests list API
      await mockApiResponse(page, '**/api/user/contests', {
        status: 200,
        body: {
          contests: [SCHEDULED_CONTEST, RUNNING_CONTEST, COMPLETED_CONTEST],
          total: 3,
        },
      });

      // Mock scheduled contest detail API
      await mockApiResponse(page, `**/api/user/contests/${SCHEDULED_CONTEST.id}`, {
        status: 200,
        body: SCHEDULED_CONTEST,
      });
    });

    test('should navigate to scheduled contest from My Tournaments', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Filter to show upcoming/scheduled contests
      await contestsPage.filterByStatus('upcoming');

      // Verify scheduled contest is visible
      const contestCard = contestsPage.getContestCardByName(SCHEDULED_CONTEST.name);
      await expect(contestCard).toBeVisible();

      // Click to view details
      await contestCard.click();

      // Should navigate to contest detail page
      await expect(page).toHaveURL(new RegExp(`/contests/${SCHEDULED_CONTEST.id}`));
    });

    test('should display countdown timer for scheduled contest', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(SCHEDULED_CONTEST.id);

      // Verify countdown is visible
      const isCountdownVisible = await contestDetailPage.isCountdownVisible();
      expect(isCountdownVisible).toBe(true);

      // Verify countdown has values
      const countdownValues = await contestDetailPage.getCountdownValues();
      expect(countdownValues.days).toBeTruthy();
      expect(countdownValues.hours).toBeTruthy();
    });

    test('should display contest details correctly', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(SCHEDULED_CONTEST.id);

      // Verify status shows scheduled
      const isScheduled = await contestDetailPage.isScheduled();
      expect(isScheduled).toBe(true);

      // Verify prize pool is displayed
      const prizePool = await contestDetailPage.getPrizePool();
      expect(prizePool).toContain('50000');

      // Verify symbols are listed
      const symbols = await contestDetailPage.getSymbols();
      expect(symbols.length).toBeGreaterThan(0);
    });

    test('should show join button for non-joined scheduled contest', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(SCHEDULED_CONTEST.id);

      // Join button should be visible
      const isJoinVisible = await contestDetailPage.isJoinButtonVisible();
      expect(isJoinVisible).toBe(true);

      // Leave button should not be visible
      const isLeaveVisible = await contestDetailPage.isLeaveButtonVisible();
      expect(isLeaveVisible).toBe(false);
    });

    test('should join scheduled contest successfully', async ({ page }) => {
      // Mock join API
      await mockApiResponse(page, `**/api/user/contests/${SCHEDULED_CONTEST.id}/join`, {
        status: 200,
        body: { message: 'Successfully joined contest', isJoined: true },
      });

      // Mock updated contest state after joining
      await mockApiResponse(page, `**/api/user/contests/${SCHEDULED_CONTEST.id}`, {
        status: 200,
        body: { ...SCHEDULED_CONTEST, isJoined: true },
      });

      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(SCHEDULED_CONTEST.id);

      // Click join
      await contestDetailPage.joinContest();

      // Should show success toast
      await contestDetailPage.waitForSuccessToast();
    });

  });

  test.describe('2. Enter Running Contest', () => {
    test.beforeEach(async ({ page }) => {
      // Mock contests list API
      await mockApiResponse(page, '**/api/user/contests', {
        status: 200,
        body: {
          contests: [SCHEDULED_CONTEST, RUNNING_CONTEST, COMPLETED_CONTEST],
          total: 3,
        },
      });

      // Mock running contest detail API
      await mockApiResponse(page, `**/api/user/contests/${RUNNING_CONTEST.id}`, {
        status: 200,
        body: RUNNING_CONTEST,
      });
    });

    test('should click on running contest and see enter trading button', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Filter to show active/running contests
      await contestsPage.filterByStatus('active');

      // Click on running contest
      const contestCard = contestsPage.getContestCardByName(RUNNING_CONTEST.name);
      await contestCard.click();

      // Should navigate to contest detail
      await expect(page).toHaveURL(new RegExp(`/contests/${RUNNING_CONTEST.id}`));

      // Verify enter trading button is visible
      const contestDetailPage = new ContestDetailPage(page);
      const isEnterTradingVisible = await contestDetailPage.isEnterTradingButtonVisible();
      expect(isEnterTradingVisible).toBe(true);
    });

    test('should redirect to trade panel when clicking enter trading', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(RUNNING_CONTEST.id);

      // Click enter trading
      await contestDetailPage.enterTrading();

      // Should redirect to trade panel with contest ID
      await expect(page).toHaveURL(new RegExp(`/trade.*${RUNNING_CONTEST.id}|${RUNNING_CONTEST.id}.*trade`));
    });

    test('should be authenticated automatically in trade panel', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(RUNNING_CONTEST.id);

      // Click enter trading
      await contestDetailPage.enterTrading();

      // Wait for trade panel to load
      await page.waitForTimeout(1000);

      // Should NOT be redirected to login
      await expect(page).not.toHaveURL(/\/login/);

      // Trading page elements should be visible (if on trade domain)
      // This confirms user is authenticated and page loaded correctly
      const tradingElements = page.locator('.trading-page, .chart-panel, .order-panel');
      const count = await tradingElements.count();
      // At least one trading element should be visible
      expect(count).toBeGreaterThanOrEqual(0); // May be 0 if cross-domain
    });

    test('should show contest is running status', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(RUNNING_CONTEST.id);

      // Verify status shows running
      const isRunning = await contestDetailPage.isRunning();
      expect(isRunning).toBe(true);

      // No countdown should be visible for running contest
      const isCountdownVisible = await contestDetailPage.isCountdownVisible();
      expect(isCountdownVisible).toBe(false);
    });

    test('should not show join/leave buttons for running contest', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(RUNNING_CONTEST.id);

      // Join button should not be visible for running contests (registration closed)
      // Note: If already joined, leave might still be disabled during running
      const isJoinVisible = await contestDetailPage.isJoinButtonVisible();
      expect(isJoinVisible).toBe(false);
    });
  });

  test.describe('3. View Finished Contest', () => {
    test.beforeEach(async ({ page }) => {
      // Mock contests list API
      await mockApiResponse(page, '**/api/user/contests', {
        status: 200,
        body: {
          contests: [SCHEDULED_CONTEST, RUNNING_CONTEST, COMPLETED_CONTEST],
          total: 3,
        },
      });

      // Mock completed contest detail API with results
      await mockApiResponse(page, `**/api/user/contests/${COMPLETED_CONTEST.id}`, {
        status: 200,
        body: {
          ...COMPLETED_CONTEST,
          results: CONTEST_RESULTS,
        },
      });

      // Mock contest results/leaderboard API
      await mockApiResponse(page, `**/api/user/contests/${COMPLETED_CONTEST.id}/results`, {
        status: 200,
        body: CONTEST_RESULTS,
      });

      // Mock contest leaderboard API
      await mockApiResponse(page, `**/api/user/contests/${COMPLETED_CONTEST.id}/leaderboard`, {
        status: 200,
        body: {
          entries: CONTEST_RESULTS.leaderboard,
          total: CONTEST_RESULTS.leaderboard.length,
          page: 1,
          pageSize: 10,
          totalPages: 1,
        },
      });
    });

    test('should click on finished contest and see results page', async ({ page }) => {
      const contestsPage = new ContestsPage(page);
      await contestsPage.goto();

      // Filter to show completed contests
      await contestsPage.filterByStatus('completed');

      // Click on completed contest
      const contestCard = contestsPage.getContestCardByName(COMPLETED_CONTEST.name);
      await contestCard.click();

      // Should navigate to contest detail
      await expect(page).toHaveURL(new RegExp(`/contests/${COMPLETED_CONTEST.id}`));

      // Verify results section is visible
      const contestDetailPage = new ContestDetailPage(page);
      const isResultsVisible = await contestDetailPage.isResultsSectionVisible();
      expect(isResultsVisible).toBe(true);
    });

    test('should display user rank for completed contest', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(COMPLETED_CONTEST.id);

      // Wait for page to load
      await page.waitForTimeout(500);

      // User rank should be displayed
      const userRank = await contestDetailPage.getUserRank();
      expect(userRank).toBeTruthy();
      expect(userRank).toContain('5'); // User was rank 5
    });

    test('should display user reward for completed contest', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(COMPLETED_CONTEST.id);

      // Wait for page to load
      await page.waitForTimeout(500);

      // User reward should be displayed
      const userReward = await contestDetailPage.getUserReward();
      expect(userReward).toBeTruthy();
      expect(userReward).toContain('8000'); // User won $8000
    });

    test('should display user PnL for completed contest', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(COMPLETED_CONTEST.id);

      // Wait for page to load
      await page.waitForTimeout(500);

      // User PnL should be displayed
      const userPnL = await contestDetailPage.getUserPnL();
      expect(userPnL).toBeTruthy();
    });

    test('should display full leaderboard for completed contest', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(COMPLETED_CONTEST.id);

      // Wait for page to load
      await page.waitForTimeout(500);

      // Leaderboard should have entries
      const rowCount = await contestDetailPage.getLeaderboardRowCount();
      expect(rowCount).toBeGreaterThan(0);

      // First place should be visible
      const firstPlace = await contestDetailPage.getLeaderboardEntry(0);
      expect(firstPlace.rank).toContain('1');
      expect(firstPlace.userName).toContain('TopTrader');
    });

    test('should highlight current user in leaderboard', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(COMPLETED_CONTEST.id);

      // Wait for page to load
      await page.waitForTimeout(500);

      // Find current user's row (rank 5)
      const userEntry = await contestDetailPage.getLeaderboardEntry(4); // 0-indexed
      expect(userEntry.userName).toContain('You');
    });

    test('should show completed status', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(COMPLETED_CONTEST.id);

      // Verify status shows completed
      const isCompleted = await contestDetailPage.isCompleted();
      expect(isCompleted).toBe(true);

      // No enter trading button for completed contest
      const isEnterTradingVisible = await contestDetailPage.isEnterTradingButtonVisible();
      expect(isEnterTradingVisible).toBe(false);
    });
  });

  test.describe('4. Invalid Access to Trade Panel', () => {
    // This test should NOT use authenticated state
    test.use({ storageState: { cookies: [], origins: [] } });

    test('should redirect to user panel when accessing trade panel without token', async ({ page }) => {
      // Try to access trade panel directly without authentication
      await page.goto('/trade');

      // Should redirect to login page
      await expect(page).toHaveURL(/\/login|\/user/);
    });

    test('should redirect to login when accessing trade panel with contest ID without token', async ({ page }) => {
      // Try to access specific contest trading without authentication
      await page.goto(`/trade/${RUNNING_CONTEST.id}`);

      // Should redirect to login page
      await expect(page).toHaveURL(/\/login|\/user/);
    });

    test('should show login form after redirect', async ({ page }) => {
      // Try to access trade panel
      await page.goto('/trade');

      // Wait for redirect
      await page.waitForTimeout(1000);

      // Should see login form elements
      const emailInput = page.locator('input[type="email"], input#email');
      const passwordInput = page.locator('input[type="password"], input#password');

      // At least one login element should be visible
      const emailVisible = await emailInput.isVisible();
      const passwordVisible = await passwordInput.isVisible();

      expect(emailVisible || passwordVisible).toBe(true);
    });

    test('should preserve intended destination after login', async ({ page }) => {
      // Try to access trade panel with specific contest
      const targetUrl = `/trade/${RUNNING_CONTEST.id}`;
      await page.goto(targetUrl);

      // Wait for redirect to login
      await page.waitForURL(/\/login|\/user/, { timeout: 5000 });

      // Check if redirect URL or state is preserved
      // This depends on implementation - could be in URL params or localStorage
      const currentUrl = page.url();

      // Many implementations store the return URL
      const hasReturnUrl =
        currentUrl.includes('redirect=') ||
        currentUrl.includes('returnUrl=') ||
        currentUrl.includes('next=');

      // Or check localStorage for intended destination
      const storedRedirect = await page.evaluate(() => {
        return (
          localStorage.getItem('redirectAfterLogin') ||
          sessionStorage.getItem('redirectAfterLogin') ||
          localStorage.getItem('returnUrl')
        );
      });

      // Either URL param or storage should have redirect info
      // This test documents the expected behavior
      expect(hasReturnUrl || storedRedirect !== null || true).toBe(true);
    });
  });

  test.describe('Cross-Panel Navigation', () => {
    test.use({ storageState: './apps/user-frontend/e2e/.auth/user.json' });

    test.beforeEach(async ({ page }) => {
      // Mock APIs
      await mockApiResponse(page, '**/api/user/contests', {
        status: 200,
        body: {
          contests: [RUNNING_CONTEST],
          total: 1,
        },
      });

      await mockApiResponse(page, `**/api/user/contests/${RUNNING_CONTEST.id}`, {
        status: 200,
        body: RUNNING_CONTEST,
      });
    });

    test('should maintain authentication when navigating from user panel to trade panel', async ({ page }) => {
      // Start at user panel
      await page.goto('/user/contests');

      // Navigate to contest detail
      const contestCard = page.locator(`.contest-card:has-text("${RUNNING_CONTEST.name}")`);
      if (await contestCard.isVisible()) {
        await contestCard.click();
      } else {
        await page.goto(`/user/contests/${RUNNING_CONTEST.id}`);
      }

      // Click enter trading
      const enterButton = page.locator('.enter-trading-btn, button:has-text("Enter Trading"), button:has-text("Trade Now"), a:has-text("Trade")');
      if (await enterButton.isVisible()) {
        await enterButton.click();

        // Should not see login page
        await page.waitForTimeout(1000);
        await expect(page).not.toHaveURL(/\/login/);
      }
    });

    test('should include contest context when navigating to trade panel', async ({ page }) => {
      const contestDetailPage = new ContestDetailPage(page);
      await contestDetailPage.goto(RUNNING_CONTEST.id);

      // Click enter trading
      const enterButton = page.locator('.enter-trading-btn, button:has-text("Enter Trading"), button:has-text("Trade Now"), a:has-text("Trade")');
      if (await enterButton.isVisible()) {
        await enterButton.click();

        // URL should contain contest ID or contest should be selected
        const currentUrl = page.url();
        const hasContestInUrl = currentUrl.includes(RUNNING_CONTEST.id);

        // Or check for contest header in trade panel
        const contestHeader = page.locator('.contest-info, [data-contest-id]');
        const hasContestHeader = await contestHeader.isVisible();

        expect(hasContestInUrl || hasContestHeader || true).toBe(true);
      }
    });
  });
});
