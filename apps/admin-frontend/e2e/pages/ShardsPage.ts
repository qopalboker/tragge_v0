import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the Shards management page
 */
export class ShardsPage {
  readonly page: Page;
  readonly pageTitle: Locator;
  readonly shardGrid: Locator;
  readonly shardCards: Locator;
  readonly statusFilter: Locator;
  readonly refreshButton: Locator;
  readonly statsChart: Locator;
  readonly timeRangeSelector: Locator;
  readonly confirmModal: Locator;
  readonly confirmButton: Locator;
  readonly cancelButton: Locator;
  readonly errorMessage: Locator;
  readonly successMessage: Locator;

  constructor(page: Page) {
    this.page = page;
    this.pageTitle = page.locator('h1, .page-title, [data-testid="page-title"]');
    this.shardGrid = page.locator('.shard-grid, .shard-container, [data-testid="shard-grid"]');
    this.shardCards = page.locator('.shard-card, [data-testid="shard-card"]');
    this.statusFilter = page.locator('.status-filter, [data-testid="status-filter"], select[name="status"]');
    this.refreshButton = page.locator('.refresh-btn, button[aria-label="Refresh"], [data-testid="refresh"]');
    this.statsChart = page.locator('.stats-chart, [data-testid="stats-chart"], canvas');
    this.timeRangeSelector = page.locator('.time-range-selector, [data-testid="time-range"]');
    this.confirmModal = page.locator('.confirm-modal, .modal, [data-testid="confirm-modal"], [role="dialog"]');
    this.confirmButton = page.locator('.confirm-btn, button:has-text("Confirm"), button:has-text("Yes"), [data-testid="confirm"]');
    this.cancelButton = page.locator('.cancel-btn, button:has-text("Cancel"), button:has-text("No"), [data-testid="cancel"]');
    this.errorMessage = page.locator('.error-message, .alert-error, [data-testid="error"]');
    this.successMessage = page.locator('.success-message, .alert-success, [data-testid="success"]');
  }

  /**
   * Navigate to the shards page
   */
  async goto(): Promise<void> {
    await this.page.goto('/admin/shards');
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Get a specific shard card by index
   */
  getShardCard(index: number): Locator {
    return this.shardCards.nth(index);
  }

  /**
   * Get a shard card by shard ID
   */
  getShardCardById(shardId: number): Locator {
    return this.page.locator(`[data-shard-id="${shardId}"], .shard-card:has-text("shard-${shardId}")`);
  }

  /**
   * Get the count of shard cards
   */
  async getShardCount(): Promise<number> {
    return await this.shardCards.count();
  }

  /**
   * Get shard name from card
   */
  async getShardName(index: number): Promise<string | null> {
    const card = this.getShardCard(index);
    const nameElement = card.locator('.shard-name, h3, [data-testid="shard-name"]');
    return await nameElement.textContent();
  }

  /**
   * Get shard status from card
   */
  async getShardStatus(index: number): Promise<string | null> {
    const card = this.getShardCard(index);
    const statusElement = card.locator('.status-badge, [data-testid="status"]');
    return await statusElement.textContent();
  }

  /**
   * Get shard contest count
   */
  async getShardContestCount(index: number): Promise<string | null> {
    const card = this.getShardCard(index);
    const countElement = card.locator('.contest-count, [data-testid="contest-count"]');
    return await countElement.textContent();
  }

  /**
   * Get shard participant count
   */
  async getShardParticipantCount(index: number): Promise<string | null> {
    const card = this.getShardCard(index);
    const countElement = card.locator('.participant-count, [data-testid="participant-count"]');
    return await countElement.textContent();
  }

  /**
   * Get shard orders per second
   */
  async getShardOrdersPerSecond(index: number): Promise<string | null> {
    const card = this.getShardCard(index);
    const opsElement = card.locator('.orders-per-sec, [data-testid="orders-per-sec"]');
    return await opsElement.textContent();
  }

  /**
   * Filter shards by status
   */
  async filterByStatus(status: 'active' | 'draining' | 'inactive' | 'maintenance' | 'all'): Promise<void> {
    // Try dropdown select
    if (await this.statusFilter.isVisible()) {
      await this.statusFilter.selectOption(status);
      return;
    }

    // Try button filters
    const filterButton = this.page.locator(`button[data-filter="${status}"], button:has-text("${status}")`);
    if (await filterButton.isVisible()) {
      await filterButton.click();
      return;
    }

    // Try tabs
    const filterTab = this.page.locator(`[role="tab"]:has-text("${status}")`);
    if (await filterTab.isVisible()) {
      await filterTab.click();
    }
  }

  /**
   * Clear status filter
   */
  async clearFilter(): Promise<void> {
    await this.filterByStatus('all');
  }

  /**
   * Click drain button for a shard
   */
  async clickDrainShard(shardId: number): Promise<void> {
    const card = this.getShardCardById(shardId);
    const drainBtn = card.locator('.drain-btn, button:has-text("Drain"), button[aria-label="Drain"], [data-action="drain"]');

    if (await drainBtn.isVisible()) {
      await drainBtn.click();
    } else {
      // Try actions menu
      const actionsBtn = card.locator('.actions-btn, button[aria-label="Actions"], [data-testid="actions"]');
      if (await actionsBtn.isVisible()) {
        await actionsBtn.click();
        const drainOption = this.page.locator('text=Drain, [data-action="drain"]');
        await drainOption.click();
      }
    }
  }

  /**
   * Click activate button for a shard
   */
  async clickActivateShard(shardId: number): Promise<void> {
    const card = this.getShardCardById(shardId);
    const activateBtn = card.locator('.activate-btn, button:has-text("Activate"), button[aria-label="Activate"], [data-action="activate"]');

    if (await activateBtn.isVisible()) {
      await activateBtn.click();
    } else {
      // Try actions menu
      const actionsBtn = card.locator('.actions-btn, button[aria-label="Actions"], [data-testid="actions"]');
      if (await actionsBtn.isVisible()) {
        await actionsBtn.click();
        const activateOption = this.page.locator('text=Activate, [data-action="activate"]');
        await activateOption.click();
      }
    }
  }

  /**
   * Click on a shard card to view details
   */
  async clickShardCard(shardId: number): Promise<void> {
    const card = this.getShardCardById(shardId);
    await card.click();
  }

  /**
   * Confirm action in modal
   */
  async confirmAction(): Promise<void> {
    await expect(this.confirmModal).toBeVisible({ timeout: 5000 });
    await this.confirmButton.click();
  }

  /**
   * Cancel action in modal
   */
  async cancelAction(): Promise<void> {
    await expect(this.confirmModal).toBeVisible({ timeout: 5000 });
    await this.cancelButton.click();
  }

  /**
   * Refresh shard data
   */
  async refresh(): Promise<void> {
    if (await this.refreshButton.isVisible()) {
      await this.refreshButton.click();
      await this.page.waitForLoadState('networkidle');
    }
  }

  /**
   * Select time range for stats
   */
  async selectTimeRange(range: '1h' | '6h' | '24h' | '7d'): Promise<void> {
    if (await this.timeRangeSelector.isVisible()) {
      await this.timeRangeSelector.click();
      const option = this.page.locator(`text=${range}`);
      await option.click();
    }
  }

  /**
   * Check if success message is visible
   */
  async isSuccessMessageVisible(): Promise<boolean> {
    return await this.successMessage.isVisible();
  }

  /**
   * Check if error message is visible
   */
  async isErrorMessageVisible(): Promise<boolean> {
    return await this.errorMessage.isVisible();
  }

  /**
   * Get all shard statuses
   */
  async getAllShardStatuses(): Promise<string[]> {
    const statuses: string[] = [];
    const count = await this.getShardCount();

    for (let i = 0; i < count; i++) {
      const status = await this.getShardStatus(i);
      if (status) {
        statuses.push(status.trim().toLowerCase());
      }
    }

    return statuses;
  }

  /**
   * Wait for shards to load
   */
  async waitForShardsToLoad(): Promise<void> {
    await expect(this.shardCards.first()).toBeVisible({ timeout: 10000 });
  }
}
