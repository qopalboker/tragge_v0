import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the Admin Frontend Contest Create/Edit Form Page
 */
export class ContestFormPage {
  readonly page: Page;

  // Page elements
  readonly pageTitle: Locator;
  readonly form: Locator;
  readonly submitButton: Locator;
  readonly cancelButton: Locator;

  // Form fields
  readonly nameInput: Locator;
  readonly descriptionInput: Locator;
  readonly prizePoolInput: Locator;
  readonly entryFeeInput: Locator;
  readonly maxParticipantsInput: Locator;
  readonly startDateInput: Locator;
  readonly endDateInput: Locator;
  readonly symbolsSelect: Locator;
  readonly statusSelect: Locator;

  // Validation messages
  readonly validationErrors: Locator;
  readonly successMessage: Locator;
  readonly errorMessage: Locator;

  constructor(page: Page) {
    this.page = page;

    // Page elements
    this.pageTitle = page.locator('h1, .page-title');
    this.form = page.locator('form');
    this.submitButton = page.locator('button[type="submit"], .submit-btn');
    this.cancelButton = page.locator('.cancel-btn, a:has-text("Cancel"), button:has-text("Cancel")');

    // Form fields
    this.nameInput = page.locator('input[name="name"], #name');
    this.descriptionInput = page.locator('textarea[name="description"], #description');
    this.prizePoolInput = page.locator('input[name="prizePool"], #prizePool');
    this.entryFeeInput = page.locator('input[name="entryFee"], #entryFee');
    this.maxParticipantsInput = page.locator('input[name="maxParticipants"], #maxParticipants');
    this.startDateInput = page.locator('input[name="startDate"], #startDate');
    this.endDateInput = page.locator('input[name="endDate"], #endDate');
    this.symbolsSelect = page.locator('select[name="symbols"], #symbols, .symbols-select');
    this.statusSelect = page.locator('select[name="status"], #status');

    // Messages
    this.validationErrors = page.locator('.validation-error, .field-error');
    this.successMessage = page.locator('.success-message, .toast-success');
    this.errorMessage = page.locator('.error-message, .toast-error');
  }

  /**
   * Navigate to the new contest form
   */
  async gotoNew(): Promise<void> {
    await this.page.goto('/admin/contests/new');
    await this.waitForPageLoad();
  }

  /**
   * Navigate to the edit contest form
   */
  async gotoEdit(contestId: string): Promise<void> {
    await this.page.goto(`/admin/contests/${contestId}`);
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await this.page.waitForLoadState('networkidle');
    await expect(this.form).toBeVisible();
  }

  /**
   * Fill the contest name
   */
  async fillName(name: string): Promise<void> {
    await this.nameInput.fill(name);
  }

  /**
   * Fill the contest description
   */
  async fillDescription(description: string): Promise<void> {
    await this.descriptionInput.fill(description);
  }

  /**
   * Fill the prize pool amount
   */
  async fillPrizePool(amount: number): Promise<void> {
    await this.prizePoolInput.fill(String(amount));
  }

  /**
   * Fill the entry fee
   */
  async fillEntryFee(amount: number): Promise<void> {
    await this.entryFeeInput.fill(String(amount));
  }

  /**
   * Fill max participants
   */
  async fillMaxParticipants(count: number): Promise<void> {
    await this.maxParticipantsInput.fill(String(count));
  }

  /**
   * Fill start date
   */
  async fillStartDate(date: string): Promise<void> {
    await this.startDateInput.fill(date);
  }

  /**
   * Fill end date
   */
  async fillEndDate(date: string): Promise<void> {
    await this.endDateInput.fill(date);
  }

  /**
   * Select symbols
   */
  async selectSymbols(symbols: string[]): Promise<void> {
    for (const symbol of symbols) {
      await this.symbolsSelect.selectOption(symbol);
    }
  }

  /**
   * Select status
   */
  async selectStatus(status: string): Promise<void> {
    await this.statusSelect.selectOption(status);
  }

  /**
   * Fill complete contest form
   */
  async fillForm(data: {
    name: string;
    description?: string;
    prizePool: number;
    entryFee: number;
    maxParticipants?: number;
    startDate: string;
    endDate: string;
    symbols?: string[];
    status?: string;
  }): Promise<void> {
    await this.fillName(data.name);

    if (data.description) {
      await this.fillDescription(data.description);
    }

    await this.fillPrizePool(data.prizePool);
    await this.fillEntryFee(data.entryFee);

    if (data.maxParticipants) {
      await this.fillMaxParticipants(data.maxParticipants);
    }

    await this.fillStartDate(data.startDate);
    await this.fillEndDate(data.endDate);

    if (data.symbols && data.symbols.length > 0) {
      await this.selectSymbols(data.symbols);
    }

    if (data.status) {
      await this.selectStatus(data.status);
    }
  }

  /**
   * Submit the form
   */
  async submit(): Promise<void> {
    await this.submitButton.click();
  }

  /**
   * Cancel and go back
   */
  async cancel(): Promise<void> {
    await this.cancelButton.click();
  }

  /**
   * Create a new contest
   */
  async createContest(data: {
    name: string;
    description?: string;
    prizePool: number;
    entryFee: number;
    maxParticipants?: number;
    startDate: string;
    endDate: string;
    symbols?: string[];
  }): Promise<void> {
    await this.fillForm(data);
    await this.submit();
  }

  /**
   * Check if there are validation errors
   */
  async hasValidationErrors(): Promise<boolean> {
    return (await this.validationErrors.count()) > 0;
  }

  /**
   * Get validation error messages
   */
  async getValidationErrors(): Promise<string[]> {
    const errors: string[] = [];
    const count = await this.validationErrors.count();

    for (let i = 0; i < count; i++) {
      const text = await this.validationErrors.nth(i).textContent();
      if (text) {
        errors.push(text);
      }
    }

    return errors;
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
   * Check if submit button is enabled
   */
  async isSubmitEnabled(): Promise<boolean> {
    return this.submitButton.isEnabled();
  }

  /**
   * Get current form values
   */
  async getFormValues(): Promise<{
    name: string;
    description: string;
    prizePool: string;
    entryFee: string;
    startDate: string;
    endDate: string;
  }> {
    return {
      name: await this.nameInput.inputValue(),
      description: await this.descriptionInput.inputValue(),
      prizePool: await this.prizePoolInput.inputValue(),
      entryFee: await this.entryFeeInput.inputValue(),
      startDate: await this.startDateInput.inputValue(),
      endDate: await this.endDateInput.inputValue(),
    };
  }
}
