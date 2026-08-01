import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the Contest Detail Page
 * Handles scheduled, running, and completed contest views
 */
export class ContestDetailPage {
  readonly page: Page;

  // Page elements
  readonly pageTitle: Locator;
  readonly contestName: Locator;
  readonly contestStatus: Locator;
  readonly contestDescription: Locator;
  readonly prizePool: Locator;
  readonly entryFee: Locator;
  readonly participantCount: Locator;
  readonly maxParticipants: Locator;

  // Countdown (for scheduled contests)
  readonly countdown: Locator;
  readonly countdownDays: Locator;
  readonly countdownHours: Locator;
  readonly countdownMinutes: Locator;
  readonly countdownSeconds: Locator;

  // Action buttons
  readonly joinButton: Locator;
  readonly enterTradingButton: Locator;
  readonly viewResultsButton: Locator;

  // Contest dates
  readonly startDate: Locator;
  readonly endDate: Locator;
  readonly registrationDeadline: Locator;

  // Symbols list
  readonly symbolsList: Locator;
  readonly symbolItems: Locator;

  // Results section (for completed contests)
  readonly resultsSection: Locator;
  readonly userRank: Locator;
  readonly userReward: Locator;
  readonly userPnL: Locator;
  readonly finalLeaderboard: Locator;
  readonly leaderboardRows: Locator;
  readonly prizeDistribution: Locator;

  // Loading and error states
  readonly loadingIndicator: Locator;
  readonly errorMessage: Locator;
  readonly notFoundMessage: Locator;

  // Toast notifications
  readonly successToast: Locator;
  readonly errorToast: Locator;

  constructor(page: Page) {
    this.page = page;

    // Page elements
    this.pageTitle = page.locator('h1, .page-title, .contest-detail-title');
    this.contestName = page.locator('.contest-name, .contest-title, h1');
    this.contestStatus = page.locator('.contest-status, .status-badge');
    this.contestDescription = page.locator('.contest-description, .description');
    this.prizePool = page.locator('.prize-pool, [data-prize-pool]');
    this.entryFee = page.locator('.entry-fee, [data-entry-fee]');
    this.participantCount = page.locator('.participant-count, [data-participants]');
    this.maxParticipants = page.locator('.max-participants, [data-max-participants]');

    // Countdown elements
    this.countdown = page.locator('.countdown, .countdown-timer');
    this.countdownDays = page.locator('.countdown-days, [data-countdown-days]');
    this.countdownHours = page.locator('.countdown-hours, [data-countdown-hours]');
    this.countdownMinutes = page.locator('.countdown-minutes, [data-countdown-minutes]');
    this.countdownSeconds = page.locator('.countdown-seconds, [data-countdown-seconds]');

    // Action buttons
    this.joinButton = page.locator('.join-btn, button:has-text("Join"), button:has-text("Register")');
    this.enterTradingButton = page.locator('.enter-trading-btn, button:has-text("Enter Trading"), button:has-text("Trade Now"), a:has-text("Trade")');
    this.viewResultsButton = page.locator('.view-results-btn, button:has-text("View Results"), a:has-text("Results")');

    // Contest dates
    this.startDate = page.locator('.start-date, [data-start-date]');
    this.endDate = page.locator('.end-date, [data-end-date]');
    this.registrationDeadline = page.locator('.registration-deadline, [data-reg-deadline]');

    // Symbols
    this.symbolsList = page.locator('.symbols-list, .available-symbols');
    this.symbolItems = page.locator('.symbol-item, .symbols-list li, .symbol-badge');

    // Results section
    this.resultsSection = page.locator('.results-section, .contest-results');
    this.userRank = page.locator('.user-rank, .your-rank, [data-user-rank]');
    this.userReward = page.locator('.user-reward, .your-reward, [data-user-reward]');
    this.userPnL = page.locator('.user-pnl, .your-pnl, [data-user-pnl]');
    this.finalLeaderboard = page.locator('.final-leaderboard, .results-leaderboard');
    this.leaderboardRows = page.locator('.leaderboard-row, .results-leaderboard tbody tr');
    this.prizeDistribution = page.locator('.prize-distribution, .payout-table');

    // Loading and error states
    this.loadingIndicator = page.locator('.loading, .spinner');
    this.errorMessage = page.locator('.error-message, .error');
    this.notFoundMessage = page.locator('.not-found, .contest-not-found');

    // Toast notifications
    this.successToast = page.locator('.success-message, .toast-success, [data-toast="success"]');
    this.errorToast = page.locator('.error-message, .toast-error, [data-toast="error"]');
  }

  /**
   * Navigate to a specific contest detail page
   */
  async goto(contestId: string): Promise<void> {
    await this.page.goto(`/user/contests/${contestId}`);
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await this.page.waitForLoadState('networkidle');
    // Wait for contest name to be visible
    await expect(this.contestName).toBeVisible({ timeout: 10000 });
  }

  /**
   * Get contest status text
   */
  async getStatus(): Promise<string | null> {
    return this.contestStatus.textContent();
  }

  /**
   * Check if contest is scheduled (has countdown)
   */
  async isScheduled(): Promise<boolean> {
    const status = await this.getStatus();
    return status?.toLowerCase().includes('scheduled') ?? false;
  }

  /**
   * Check if contest is running
   */
  async isRunning(): Promise<boolean> {
    const status = await this.getStatus();
    return status?.toLowerCase().includes('running') ?? false;
  }

  /**
   * Check if contest is completed
   */
  async isCompleted(): Promise<boolean> {
    const status = await this.getStatus();
    return status?.toLowerCase().includes('completed') ?? false;
  }

  /**
   * Check if countdown is visible (for scheduled contests)
   */
  async isCountdownVisible(): Promise<boolean> {
    return this.countdown.isVisible();
  }

  /**
   * Get countdown values
   */
  async getCountdownValues(): Promise<{
    days: string | null;
    hours: string | null;
    minutes: string | null;
    seconds: string | null;
  }> {
    return {
      days: await this.countdownDays.textContent(),
      hours: await this.countdownHours.textContent(),
      minutes: await this.countdownMinutes.textContent(),
      seconds: await this.countdownSeconds.textContent(),
    };
  }

  /**
   * Check if join button is visible
   */
  async isJoinButtonVisible(): Promise<boolean> {
    return this.joinButton.isVisible();
  }

  /**
   * Check if enter trading button is visible
   */
  async isEnterTradingButtonVisible(): Promise<boolean> {
    return this.enterTradingButton.isVisible();
  }

  /**
   * Join the contest
   */
  async joinContest(): Promise<void> {
    await this.joinButton.click();
    await this.page.waitForTimeout(500);
  }

  /**
   * Enter trading (for running contests)
   */
  async enterTrading(): Promise<void> {
    await this.enterTradingButton.click();
  }

  /**
   * Get prize pool value
   */
  async getPrizePool(): Promise<string | null> {
    return this.prizePool.textContent();
  }

  /**
   * Get entry fee value
   */
  async getEntryFee(): Promise<string | null> {
    return this.entryFee.textContent();
  }

  /**
   * Get participant count
   */
  async getParticipantCount(): Promise<string | null> {
    return this.participantCount.textContent();
  }

  /**
   * Get available symbols
   */
  async getSymbols(): Promise<string[]> {
    const symbols: string[] = [];
    const count = await this.symbolItems.count();
    for (let i = 0; i < count; i++) {
      const text = await this.symbolItems.nth(i).textContent();
      if (text) symbols.push(text.trim());
    }
    return symbols;
  }

  /**
   * Check if results section is visible
   */
  async isResultsSectionVisible(): Promise<boolean> {
    return this.resultsSection.isVisible();
  }

  /**
   * Get user's final rank (for completed contests)
   */
  async getUserRank(): Promise<string | null> {
    return this.userRank.textContent();
  }

  /**
   * Get user's reward (for completed contests)
   */
  async getUserReward(): Promise<string | null> {
    return this.userReward.textContent();
  }

  /**
   * Get user's final P&L (for completed contests)
   */
  async getUserPnL(): Promise<string | null> {
    return this.userPnL.textContent();
  }

  /**
   * Get number of rows in final leaderboard
   */
  async getLeaderboardRowCount(): Promise<number> {
    return this.leaderboardRows.count();
  }

  /**
   * Get leaderboard entry by index
   */
  async getLeaderboardEntry(index: number): Promise<{
    rank: string | null;
    userName: string | null;
    pnl: string | null;
    reward: string | null;
  }> {
    const row = this.leaderboardRows.nth(index);
    return {
      rank: await row.locator('td:nth-child(1), .rank').textContent(),
      userName: await row.locator('td:nth-child(2), .user-name').textContent(),
      pnl: await row.locator('td:nth-child(3), .pnl').textContent(),
      reward: await row.locator('td:nth-child(4), .reward').textContent(),
    };
  }

  /**
   * Check if success toast is visible
   */
  async isSuccessToastVisible(): Promise<boolean> {
    return this.successToast.isVisible();
  }

  /**
   * Check if error toast is visible
   */
  async isErrorToastVisible(): Promise<boolean> {
    return this.errorToast.isVisible();
  }

  /**
   * Wait for success toast
   */
  async waitForSuccessToast(timeout = 5000): Promise<void> {
    await expect(this.successToast).toBeVisible({ timeout });
  }

  /**
   * Wait for error toast
   */
  async waitForErrorToast(timeout = 5000): Promise<void> {
    await expect(this.errorToast).toBeVisible({ timeout });
  }

  /**
   * Check if loading
   */
  async isLoading(): Promise<boolean> {
    return this.loadingIndicator.isVisible();
  }

  /**
   * Check if not found
   */
  async isNotFound(): Promise<boolean> {
    return this.notFoundMessage.isVisible();
  }
}
