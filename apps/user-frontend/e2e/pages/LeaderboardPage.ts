import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the User Frontend Leaderboard Page
 */
export class LeaderboardPage {
  readonly page: Page;

  // Page elements
  readonly pageTitle: Locator;
  readonly contestSelector: Locator;
  readonly leaderboardTable: Locator;
  readonly leaderboardRows: Locator;
  readonly loadingIndicator: Locator;
  readonly emptyState: Locator;

  // Table columns
  readonly rankColumn: Locator;
  readonly userColumn: Locator;
  readonly pnlColumn: Locator;
  readonly tradesColumn: Locator;

  // Pagination
  readonly pagination: Locator;
  readonly prevButton: Locator;
  readonly nextButton: Locator;
  readonly pageNumbers: Locator;

  // User's own row
  readonly ownRow: Locator;

  // Real-time indicator
  readonly liveIndicator: Locator;
  readonly lastUpdated: Locator;

  constructor(page: Page) {
    this.page = page;

    // Page elements
    this.pageTitle = page.locator('h1, .page-title');
    this.contestSelector = page.locator('.contest-selector, select[name="contest"]');
    this.leaderboardTable = page.locator('.leaderboard-table, table');
    this.leaderboardRows = page.locator('.leaderboard-row, tbody tr');
    this.loadingIndicator = page.locator('.loading, .spinner');
    this.emptyState = page.locator('.empty-state');

    // Table columns (headers)
    this.rankColumn = page.locator('th:has-text("Rank"), th:has-text("#")');
    this.userColumn = page.locator('th:has-text("User"), th:has-text("Trader")');
    this.pnlColumn = page.locator('th:has-text("P&L"), th:has-text("PnL"), th:has-text("Profit")');
    this.tradesColumn = page.locator('th:has-text("Trades"), th:has-text("Orders")');

    // Pagination
    this.pagination = page.locator('.pagination');
    this.prevButton = page.locator('.pagination-prev, button:has-text("Previous"), button:has-text("Prev")');
    this.nextButton = page.locator('.pagination-next, button:has-text("Next")');
    this.pageNumbers = page.locator('.pagination-page, .page-number');

    // User's own row (highlighted)
    this.ownRow = page.locator('.own-row, .highlighted-row, tr.current-user');

    // Real-time indicator
    this.liveIndicator = page.locator('.live-indicator, .realtime-badge');
    this.lastUpdated = page.locator('.last-updated, .update-time');
  }

  /**
   * Navigate to the leaderboard page
   */
  async goto(): Promise<void> {
    await this.page.goto('/user/leaderboard');
    await this.waitForPageLoad();
  }

  /**
   * Navigate to leaderboard for a specific contest
   */
  async gotoContest(contestId: string): Promise<void> {
    await this.page.goto(`/user/leaderboard?contest=${contestId}`);
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Get the number of rows in the leaderboard
   */
  async getRowCount(): Promise<number> {
    return this.leaderboardRows.count();
  }

  /**
   * Get leaderboard row by index
   */
  getRow(index: number): Locator {
    return this.leaderboardRows.nth(index);
  }

  /**
   * Get rank from a row
   */
  async getRank(index: number): Promise<string | null> {
    const row = this.leaderboardRows.nth(index);
    const rank = row.locator('td:first-child, .rank');
    return rank.textContent();
  }

  /**
   * Get username from a row
   */
  async getUsername(index: number): Promise<string | null> {
    const row = this.leaderboardRows.nth(index);
    const user = row.locator('.user-name, td:nth-child(2)');
    return user.textContent();
  }

  /**
   * Get P&L from a row
   */
  async getPnL(index: number): Promise<string | null> {
    const row = this.leaderboardRows.nth(index);
    const pnl = row.locator('.pnl, td:nth-child(3)');
    return pnl.textContent();
  }

  /**
   * Get number of trades from a row
   */
  async getTrades(index: number): Promise<string | null> {
    const row = this.leaderboardRows.nth(index);
    const trades = row.locator('.trades, td:nth-child(4)');
    return trades.textContent();
  }

  /**
   * Select a contest from the dropdown
   */
  async selectContest(contestName: string): Promise<void> {
    if (await this.contestSelector.isVisible()) {
      await this.contestSelector.selectOption({ label: contestName });
      await this.page.waitForTimeout(500); // Wait for data to load
    }
  }

  /**
   * Go to next page
   */
  async nextPage(): Promise<void> {
    if (await this.nextButton.isEnabled()) {
      await this.nextButton.click();
      await this.page.waitForTimeout(300);
    }
  }

  /**
   * Go to previous page
   */
  async prevPage(): Promise<void> {
    if (await this.prevButton.isEnabled()) {
      await this.prevButton.click();
      await this.page.waitForTimeout(300);
    }
  }

  /**
   * Go to a specific page
   */
  async goToPage(pageNumber: number): Promise<void> {
    const pageButton = this.pageNumbers.filter({ hasText: String(pageNumber) });
    if (await pageButton.isVisible()) {
      await pageButton.click();
      await this.page.waitForTimeout(300);
    }
  }

  /**
   * Get current page number
   */
  async getCurrentPage(): Promise<number> {
    const activePage = this.pageNumbers.filter({ has: this.page.locator('.active, [aria-current="page"]') });
    const text = await activePage.textContent();
    return parseInt(text || '1', 10);
  }

  /**
   * Check if own row is visible and highlighted
   */
  async isOwnRowVisible(): Promise<boolean> {
    return this.ownRow.isVisible();
  }

  /**
   * Get own rank
   */
  async getOwnRank(): Promise<string | null> {
    if (await this.ownRow.isVisible()) {
      const rank = this.ownRow.locator('td:first-child, .rank');
      return rank.textContent();
    }
    return null;
  }

  /**
   * Check if live indicator is visible
   */
  async isLiveIndicatorVisible(): Promise<boolean> {
    return this.liveIndicator.isVisible();
  }

  /**
   * Get last updated time
   */
  async getLastUpdatedTime(): Promise<string | null> {
    if (await this.lastUpdated.isVisible()) {
      return this.lastUpdated.textContent();
    }
    return null;
  }

  /**
   * Wait for leaderboard update (simulated real-time)
   */
  async waitForUpdate(timeout = 5000): Promise<void> {
    const initialTime = await this.getLastUpdatedTime();
    const startTime = Date.now();

    while (Date.now() - startTime < timeout) {
      const currentTime = await this.getLastUpdatedTime();
      if (currentTime !== initialTime) {
        return;
      }
      await this.page.waitForTimeout(100);
    }
  }

  /**
   * Check if loading indicator is visible
   */
  async isLoading(): Promise<boolean> {
    return this.loadingIndicator.isVisible();
  }

  /**
   * Check if empty state is visible
   */
  async isEmpty(): Promise<boolean> {
    return this.emptyState.isVisible();
  }
}
