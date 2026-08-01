import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the Admin Frontend Login Page
 */
export class LoginPage {
  readonly page: Page;

  // Locators
  readonly logo: Locator;
  readonly loginTitle: Locator;
  readonly emailInput: Locator;
  readonly passwordInput: Locator;
  readonly submitButton: Locator;
  readonly errorMessage: Locator;
  readonly languageToggle: Locator;
  readonly loadingSpinner: Locator;

  constructor(page: Page) {
    this.page = page;

    this.logo = page.locator('.login-logo, .logo');
    this.loginTitle = page.locator('.login-title, h1');
    this.emailInput = page.locator('input[type="email"], input#email');
    this.passwordInput = page.locator('input[type="password"], input#password');
    this.submitButton = page.locator('button[type="submit"]');
    this.errorMessage = page.locator('.error-message');
    this.languageToggle = page.locator('.lang-toggle');
    this.loadingSpinner = page.locator('.spinner');
  }

  /**
   * Navigate to the admin login page
   */
  async goto(): Promise<void> {
    await this.page.goto('/admin/login');
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await expect(this.emailInput).toBeVisible();
    await expect(this.passwordInput).toBeVisible();
  }

  /**
   * Login with admin credentials
   */
  async login(email: string, password: string): Promise<void> {
    await this.emailInput.fill(email);
    await this.passwordInput.fill(password);
    await this.submitButton.click();
  }

  /**
   * Login and wait for navigation to admin dashboard
   */
  async loginAndWaitForDashboard(email: string, password: string): Promise<void> {
    await this.login(email, password);
    await this.page.waitForURL(/\/admin\/(contests|dashboard|$)/);
  }

  /**
   * Get error message
   */
  async getErrorMessage(): Promise<string | null> {
    if (await this.errorMessage.isVisible()) {
      return this.errorMessage.textContent();
    }
    return null;
  }

  /**
   * Toggle language
   */
  async toggleLanguage(): Promise<void> {
    await this.languageToggle.click();
  }

  /**
   * Check if submit button is enabled
   */
  async isSubmitEnabled(): Promise<boolean> {
    return this.submitButton.isEnabled();
  }
}
