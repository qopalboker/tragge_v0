import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the Admin Frontend Contests Management Page
 */
export class ContestsPage {
  readonly page: Page;

  // Page elements
  readonly pageTitle: Locator;
  readonly createButton: Locator;
  readonly contestsTable: Locator;
  readonly contestRows: Locator;
  readonly searchInput: Locator;
  readonly filterDropdown: Locator;
  readonly loadingIndicator: Locator;
  readonly emptyState: Locator;

  // Table columns
  readonly nameColumn: Locator;
  readonly statusColumn: Locator;
  readonly participantsColumn: Locator;
  readonly prizePoolColumn: Locator;
  readonly actionsColumn: Locator;

  // Action buttons (row level)
  readonly editButton: Locator;
  readonly startButton: Locator;
  readonly stopButton: Locator;
  readonly deleteButton: Locator;
  readonly viewParticipantsButton: Locator;

  // Modals
  readonly confirmModal: Locator;
  readonly confirmButton: Locator;
  readonly cancelButton: Locator;

  // Success/Error messages
  readonly successMessage: Locator;
  readonly errorMessage: Locator;

  constructor(page: Page) {
    this.page = page;

    // Page elements
    this.pageTitle = page.locator('h1, .page-title');
    this.createButton = page.locator('.create-btn, button:has-text("Create"), a:has-text("New Contest")');
    this.contestsTable = page.locator('.contests-table, table');
    this.contestRows = page.locator('.contest-row, tbody tr');
    this.searchInput = page.locator('input[type="search"], .search-input');
    this.filterDropdown = page.locator('.filter-dropdown, select[name="status"]');
    this.loadingIndicator = page.locator('.loading, .spinner');
    this.emptyState = page.locator('.empty-state');

    // Table columns
    this.nameColumn = page.locator('th:has-text("Name")');
    this.statusColumn = page.locator('th:has-text("Status")');
    this.participantsColumn = page.locator('th:has-text("Participants")');
    this.prizePoolColumn = page.locator('th:has-text("Prize")');
    this.actionsColumn = page.locator('th:has-text("Actions")');

    // Action buttons (generic, use with row context)
    this.editButton = page.locator('.edit-btn, button[aria-label="Edit"]');
    this.startButton = page.locator('.start-btn, button:has-text("Start")');
    this.stopButton = page.locator('.stop-btn, button:has-text("Stop"), button:has-text("Pause")');
    this.deleteButton = page.locator('.delete-btn, button[aria-label="Delete"]');
    this.viewParticipantsButton = page.locator('.view-participants, button:has-text("Participants")');

    // Modals
    this.confirmModal = page.locator('.confirm-modal, [role="dialog"]');
    this.confirmButton = page.locator('.confirm-modal .confirm-btn, [role="dialog"] button:has-text("Confirm")');
    this.cancelButton = page.locator('.confirm-modal .cancel-btn, [role="dialog"] button:has-text("Cancel")');

    // Messages
    this.successMessage = page.locator('.success-message, .toast-success');
    this.errorMessage = page.locator('.error-message, .toast-error');
  }

  /**
   * Navigate to the contests management page
   */
  async goto(): Promise<void> {
    await this.page.goto('/admin/contests');
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await this.page.waitForLoadState('networkidle');
  }

  /**
   * Click create new contest button
   */
  async clickCreateContest(): Promise<void> {
    await this.createButton.click();
    await this.page.waitForURL(/\/admin\/contests\/new/);
  }

  /**
   * Get the number of contests in the table
   */
  async getContestCount(): Promise<number> {
    return this.contestRows.count();
  }

  /**
   * Get contest row by index
   */
  getContestRow(index: number): Locator {
    return this.contestRows.nth(index);
  }

  /**
   * Get contest row by name
   */
  getContestRowByName(name: string): Locator {
    return this.contestRows.filter({ hasText: name }).first();
  }

  /**
   * Get contest name from row
   */
  async getContestName(index: number): Promise<string | null> {
    const row = this.contestRows.nth(index);
    const name = row.locator('td:nth-child(1), .contest-name');
    return name.textContent();
  }

  /**
   * Get contest status from row
   */
  async getContestStatus(index: number): Promise<string | null> {
    const row = this.contestRows.nth(index);
    const status = row.locator('.status-badge, td:nth-child(2)');
    return status.textContent();
  }

  /**
   * Click edit button for a contest
   */
  async clickEditContest(index: number): Promise<void> {
    const row = this.contestRows.nth(index);
    const editBtn = row.locator('.edit-btn, button[aria-label="Edit"]');
    await editBtn.click();
  }

  /**
   * Click edit button by contest name
   */
  async editContestByName(name: string): Promise<void> {
    const row = this.getContestRowByName(name);
    const editBtn = row.locator('.edit-btn, button[aria-label="Edit"]');
    await editBtn.click();
  }

  /**
   * Start a contest
   */
  async startContest(index: number): Promise<void> {
    const row = this.contestRows.nth(index);
    const startBtn = row.locator('.start-btn, button:has-text("Start")');
    await startBtn.click();

    // Confirm if modal appears
    if (await this.confirmModal.isVisible()) {
      await this.confirmButton.click();
    }
  }

  /**
   * Stop (pause) a contest
   */
  async stopContest(index: number): Promise<void> {
    const row = this.contestRows.nth(index);
    const stopBtn = row.locator('.stop-btn, button:has-text("Stop"), button:has-text("Pause")');
    await stopBtn.click();

    // Confirm if modal appears
    if (await this.confirmModal.isVisible()) {
      await this.confirmButton.click();
    }
  }

  /**
   * Delete a contest
   */
  async deleteContest(index: number): Promise<void> {
    const row = this.contestRows.nth(index);
    const deleteBtn = row.locator('.delete-btn, button[aria-label="Delete"]');
    await deleteBtn.click();

    // Confirm deletion
    if (await this.confirmModal.isVisible()) {
      await this.confirmButton.click();
    }
  }

  /**
   * View contest participants
   */
  async viewParticipants(index: number): Promise<void> {
    const row = this.contestRows.nth(index);
    const viewBtn = row.locator('.view-participants, button:has-text("Participants")');
    await viewBtn.click();
  }

  /**
   * Search for contests
   */
  async searchContests(query: string): Promise<void> {
    if (await this.searchInput.isVisible()) {
      await this.searchInput.fill(query);
      await this.page.waitForTimeout(500); // Debounce
    }
  }

  /**
   * Filter contests by status
   */
  async filterByStatus(status: string): Promise<void> {
    if (await this.filterDropdown.isVisible()) {
      await this.filterDropdown.selectOption(status);
      await this.page.waitForTimeout(300);
    }
  }

  /**
   * Check if success message is visible
   */
  async isSuccessMessageVisible(): Promise<boolean> {
    return this.successMessage.isVisible();
  }

  /**
   * Get success message text
   */
  async getSuccessMessage(): Promise<string | null> {
    if (await this.successMessage.isVisible()) {
      return this.successMessage.textContent();
    }
    return null;
  }

  /**
   * Check if error message is visible
   */
  async isErrorMessageVisible(): Promise<boolean> {
    return this.errorMessage.isVisible();
  }

  /**
   * Get error message text
   */
  async getErrorMessage(): Promise<string | null> {
    if (await this.errorMessage.isVisible()) {
      return this.errorMessage.textContent();
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
}
