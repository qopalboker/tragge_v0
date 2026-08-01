import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the Admin Frontend Audit Logs Page
 */
export class AuditPage {
  readonly page: Page;

  // Page elements
  readonly pageTitle: Locator;
  readonly auditTable: Locator;
  readonly auditRows: Locator;
  readonly loadingIndicator: Locator;
  readonly emptyState: Locator;

  // Filters
  readonly dateFromInput: Locator;
  readonly dateToInput: Locator;
  readonly actionFilter: Locator;
  readonly userFilter: Locator;
  readonly applyFiltersButton: Locator;
  readonly clearFiltersButton: Locator;

  // Export
  readonly exportButton: Locator;
  readonly exportCSVButton: Locator;
  readonly exportJSONButton: Locator;

  // Pagination
  readonly pagination: Locator;
  readonly prevButton: Locator;
  readonly nextButton: Locator;
  readonly pageNumbers: Locator;
  readonly pageInfo: Locator;

  // Table columns
  readonly timestampColumn: Locator;
  readonly actionColumn: Locator;
  readonly userColumn: Locator;
  readonly detailsColumn: Locator;

  constructor(page: Page) {
    this.page = page;

    // Page elements
    this.pageTitle = page.locator('h1, .page-title');
    this.auditTable = page.locator('.audit-table, table');
    this.auditRows = page.locator('.audit-row, tbody tr');
    this.loadingIndicator = page.locator('.loading, .spinner');
    this.emptyState = page.locator('.empty-state');

    // Filters
    this.dateFromInput = page.locator('input[name="dateFrom"], #dateFrom');
    this.dateToInput = page.locator('input[name="dateTo"], #dateTo');
    this.actionFilter = page.locator('select[name="action"], #actionFilter');
    this.userFilter = page.locator('input[name="user"], #userFilter, select[name="user"]');
    this.applyFiltersButton = page.locator('.apply-filters, button:has-text("Apply"), button:has-text("Filter")');
    this.clearFiltersButton = page.locator('.clear-filters, button:has-text("Clear")');

    // Export
    this.exportButton = page.locator('.export-btn, button:has-text("Export")');
    this.exportCSVButton = page.locator('.export-csv, button:has-text("CSV")');
    this.exportJSONButton = page.locator('.export-json, button:has-text("JSON")');

    // Pagination
    this.pagination = page.locator('.pagination');
    this.prevButton = page.locator('.pagination-prev, button:has-text("Previous"), button:has-text("Prev")');
    this.nextButton = page.locator('.pagination-next, button:has-text("Next")');
    this.pageNumbers = page.locator('.pagination-page, .page-number');
    this.pageInfo = page.locator('.page-info, .pagination-info');

    // Table columns
    this.timestampColumn = page.locator('th:has-text("Time"), th:has-text("Date")');
    this.actionColumn = page.locator('th:has-text("Action")');
    this.userColumn = page.locator('th:has-text("User")');
    this.detailsColumn = page.locator('th:has-text("Details")');
  }

  /**
   * Navigate to the audit logs page
   */
  async goto(): Promise<void> {
    await this.page.goto('/admin/audit');
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Get the number of audit log entries
   */
  async getLogCount(): Promise<number> {
    return this.auditRows.count();
  }

  /**
   * Get audit log row by index
   */
  getLogRow(index: number): Locator {
    return this.auditRows.nth(index);
  }

  /**
   * Get timestamp from row
   */
  async getTimestamp(index: number): Promise<string | null> {
    const row = this.auditRows.nth(index);
    const timestamp = row.locator('td:nth-child(1), .log-timestamp');
    return timestamp.textContent();
  }

  /**
   * Get action from row
   */
  async getAction(index: number): Promise<string | null> {
    const row = this.auditRows.nth(index);
    const action = row.locator('td:nth-child(2), .log-action');
    return action.textContent();
  }

  /**
   * Get user from row
   */
  async getUser(index: number): Promise<string | null> {
    const row = this.auditRows.nth(index);
    const user = row.locator('td:nth-child(3), .log-user');
    return user.textContent();
  }

  /**
   * Get details from row
   */
  async getDetails(index: number): Promise<string | null> {
    const row = this.auditRows.nth(index);
    const details = row.locator('td:nth-child(4), .log-details');
    return details.textContent();
  }

  /**
   * Filter by date range
   */
  async filterByDateRange(from: string, to: string): Promise<void> {
    if (await this.dateFromInput.isVisible()) {
      await this.dateFromInput.fill(from);
    }
    if (await this.dateToInput.isVisible()) {
      await this.dateToInput.fill(to);
    }
    await this.applyFilters();
  }

  /**
   * Filter by action type
   */
  async filterByAction(action: string): Promise<void> {
    if (await this.actionFilter.isVisible()) {
      await this.actionFilter.selectOption(action);
      await this.applyFilters();
    }
  }

  /**
   * Filter by user
   */
  async filterByUser(user: string): Promise<void> {
    if (await this.userFilter.isVisible()) {
      const tagName = await this.userFilter.evaluate((el) => el.tagName.toLowerCase());
      if (tagName === 'select') {
        await this.userFilter.selectOption(user);
      } else {
        await this.userFilter.fill(user);
      }
      await this.applyFilters();
    }
  }

  /**
   * Apply filters
   */
  async applyFilters(): Promise<void> {
    if (await this.applyFiltersButton.isVisible()) {
      await this.applyFiltersButton.click();
      await this.page.waitForTimeout(500);
    }
  }

  /**
   * Clear all filters
   */
  async clearFilters(): Promise<void> {
    if (await this.clearFiltersButton.isVisible()) {
      await this.clearFiltersButton.click();
      await this.page.waitForTimeout(300);
    }
  }

  /**
   * Export logs as CSV
   */
  async exportAsCSV(): Promise<void> {
    // Check if there's a dropdown or direct button
    if (await this.exportCSVButton.isVisible()) {
      await this.exportCSVButton.click();
    } else if (await this.exportButton.isVisible()) {
      await this.exportButton.click();
      // Wait for dropdown to appear
      await this.page.waitForTimeout(200);
      const csvOption = this.page.locator('button:has-text("CSV"), .export-option:has-text("CSV")');
      if (await csvOption.isVisible()) {
        await csvOption.click();
      }
    }
  }

  /**
   * Export logs as JSON
   */
  async exportAsJSON(): Promise<void> {
    if (await this.exportJSONButton.isVisible()) {
      await this.exportJSONButton.click();
    } else if (await this.exportButton.isVisible()) {
      await this.exportButton.click();
      await this.page.waitForTimeout(200);
      const jsonOption = this.page.locator('button:has-text("JSON"), .export-option:has-text("JSON")');
      if (await jsonOption.isVisible()) {
        await jsonOption.click();
      }
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
   * Get page info text
   */
  async getPageInfo(): Promise<string | null> {
    if (await this.pageInfo.isVisible()) {
      return this.pageInfo.textContent();
    }
    return null;
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

  /**
   * Wait for download to start (for export)
   */
  async waitForDownload(): Promise<void> {
    const [download] = await Promise.all([
      this.page.waitForEvent('download'),
      // The export action should have been triggered before this
    ]);
    await download.path();
  }

  /**
   * Get all audit log entries as objects
   */
  async getAllLogEntries(): Promise<Array<{
    timestamp: string | null;
    action: string | null;
    user: string | null;
    details: string | null;
  }>> {
    const entries = [];
    const count = await this.getLogCount();

    for (let i = 0; i < count; i++) {
      entries.push({
        timestamp: await this.getTimestamp(i),
        action: await this.getAction(i),
        user: await this.getUser(i),
        details: await this.getDetails(i),
      });
    }

    return entries;
  }
}
