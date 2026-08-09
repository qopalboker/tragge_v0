import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the User Frontend Login Page.
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
  readonly forgotPasswordLink: Locator;
  readonly registerLink: Locator;

  constructor(page: Page) {
    this.page = page;

    this.logo = page.locator('.logo-box');
    this.loginTitle = page.locator('.brand-title');
    this.emailInput = page.locator('.auth-form:not(.signup) input[type="email"]');
    this.passwordInput = page.locator('.auth-form:not(.signup) input[type="password"]');
    this.submitButton = page.locator('.auth-form:not(.signup) button[type="submit"]');
    this.errorMessage = page.locator('.auth-error');
    this.languageToggle = page.locator('.lang-buttons');
    this.loadingSpinner = this.submitButton.locator('.spinner');
    this.forgotPasswordLink = page.locator('.forgot-link');
    this.registerLink = page.locator('.toggle-link');
  }

  /**
   * Navigate to the User login page.
   */
  async goto(): Promise<void> {
    await this.page.goto('/user/login');
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
   * Login with User credentials.
   */
  async login(email: string, password: string): Promise<void> {
    await this.emailInput.fill(email);
    await this.passwordInput.fill(password);
    await this.submitButton.click();
  }

  /**
   * Login and wait for navigation to the User dashboard.
   */
  async loginAndWaitForDashboard(email: string, password: string): Promise<void> {
    await this.login(email, password);
    await this.page.waitForURL(/\/user\/(dashboard|$)/);
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
    const englishActive = await this.page.locator('.lang-btn.active').textContent();
    await this.page.locator('.lang-btn', {
      hasText: englishActive?.trim() === 'EN' ? 'FA' : 'EN',
    }).click();
  }

  async togglePasswordVisibility(): Promise<void> {
    await this.page.locator('.auth-form:not(.signup) .eye-btn').click();
  }

  async clickForgotPassword(): Promise<void> {
    await this.forgotPasswordLink.click();
  }

  async clickRegister(): Promise<void> {
    await this.registerLink.click();
  }

  async getLanguageToggleText(): Promise<string | null> {
    return this.page.locator('.lang-btn.active').textContent();
  }

  async isRtl(): Promise<boolean> {
    return this.page.locator('.login-root').evaluate(
      (element) => window.getComputedStyle(element).direction === 'rtl',
    );
  }

  async fillEmail(email: string): Promise<void> {
    await this.emailInput.fill(email);
  }

  async fillPassword(password: string): Promise<void> {
    await this.passwordInput.fill(password);
  }

  async submit(): Promise<void> {
    await this.submitButton.click();
  }

  /**
   * Check if submit button is enabled
   */
  async isSubmitEnabled(): Promise<boolean> {
    return this.submitButton.isEnabled();
  }
}
