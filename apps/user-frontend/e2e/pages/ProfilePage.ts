import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the User Frontend Profile Page
 */
export class ProfilePage {
  readonly page: Page;

  // Page elements
  readonly pageTitle: Locator;
  readonly profileCard: Locator;
  readonly editButton: Locator;
  readonly saveButton: Locator;
  readonly cancelButton: Locator;

  // Profile info
  readonly avatar: Locator;
  readonly displayName: Locator;
  readonly email: Locator;
  readonly joinDate: Locator;

  // Edit form
  readonly nameInput: Locator;
  readonly emailInput: Locator;
  readonly bioInput: Locator;
  readonly avatarUpload: Locator;

  // Statistics
  readonly statsSection: Locator;
  readonly totalContests: Locator;
  readonly totalWins: Locator;
  readonly totalPnL: Locator;
  readonly winRate: Locator;

  // Settings
  readonly settingsSection: Locator;
  readonly languageSelect: Locator;
  readonly themeToggle: Locator;
  readonly notificationsToggle: Locator;

  // Messages
  readonly successMessage: Locator;
  readonly errorMessage: Locator;

  constructor(page: Page) {
    this.page = page;

    // Page elements
    this.pageTitle = page.locator('h1, .page-title');
    this.profileCard = page.locator('.profile-card, .profile-section');
    this.editButton = page.locator('.edit-btn, button:has-text("Edit")');
    this.saveButton = page.locator('.save-btn, button:has-text("Save")');
    this.cancelButton = page.locator('.cancel-btn, button:has-text("Cancel")');

    // Profile info
    this.avatar = page.locator('.avatar, .profile-avatar, img[alt*="avatar"]');
    this.displayName = page.locator('.display-name, .user-name, h2');
    this.email = page.locator('.user-email, [data-field="email"]');
    this.joinDate = page.locator('.join-date, [data-field="joinDate"]');

    // Edit form
    this.nameInput = page.locator('input[name="name"], input[name="displayName"], #name');
    this.emailInput = page.locator('input[name="email"], #email');
    this.bioInput = page.locator('textarea[name="bio"], #bio');
    this.avatarUpload = page.locator('input[type="file"]');

    // Statistics
    this.statsSection = page.locator('.stats-section, .statistics');
    this.totalContests = page.locator('[data-stat="contests"], .total-contests');
    this.totalWins = page.locator('[data-stat="wins"], .total-wins');
    this.totalPnL = page.locator('[data-stat="pnl"], .total-pnl');
    this.winRate = page.locator('[data-stat="winRate"], .win-rate');

    // Settings
    this.settingsSection = page.locator('.settings-section, .preferences');
    this.languageSelect = page.locator('select[name="language"], #language');
    this.themeToggle = page.locator('.theme-toggle, input[name="theme"]');
    this.notificationsToggle = page.locator('.notifications-toggle, input[name="notifications"]');

    // Messages
    this.successMessage = page.locator('.success-message, .toast-success');
    this.errorMessage = page.locator('.error-message, .toast-error');
  }

  /**
   * Navigate to the profile page
   */
  async goto(): Promise<void> {
    await this.page.goto('/user/profile');
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await this.page.waitForLoadState('networkidle');
    await expect(this.profileCard).toBeVisible();
  }

  /**
   * Get display name
   */
  async getDisplayName(): Promise<string | null> {
    return this.displayName.textContent();
  }

  /**
   * Get email address
   */
  async getEmail(): Promise<string | null> {
    return this.email.textContent();
  }

  /**
   * Enter edit mode
   */
  async enterEditMode(): Promise<void> {
    await this.editButton.click();
    await expect(this.saveButton).toBeVisible();
  }

  /**
   * Exit edit mode (cancel)
   */
  async exitEditMode(): Promise<void> {
    await this.cancelButton.click();
    await expect(this.editButton).toBeVisible();
  }

  /**
   * Update display name
   */
  async updateName(name: string): Promise<void> {
    await this.nameInput.fill(name);
  }

  /**
   * Update bio
   */
  async updateBio(bio: string): Promise<void> {
    await this.bioInput.fill(bio);
  }

  /**
   * Save profile changes
   */
  async saveChanges(): Promise<void> {
    await this.saveButton.click();
  }

  /**
   * Update profile with new data
   */
  async updateProfile(data: { name?: string; bio?: string }): Promise<void> {
    await this.enterEditMode();

    if (data.name) {
      await this.updateName(data.name);
    }

    if (data.bio) {
      await this.updateBio(data.bio);
    }

    await this.saveChanges();
  }

  /**
   * Change language
   */
  async changeLanguage(language: 'en' | 'fa'): Promise<void> {
    if (await this.languageSelect.isVisible()) {
      await this.languageSelect.selectOption(language);
    }
  }

  /**
   * Toggle theme
   */
  async toggleTheme(): Promise<void> {
    if (await this.themeToggle.isVisible()) {
      await this.themeToggle.click();
    }
  }

  /**
   * Toggle notifications
   */
  async toggleNotifications(): Promise<void> {
    if (await this.notificationsToggle.isVisible()) {
      await this.notificationsToggle.click();
    }
  }

  /**
   * Get total contests stat
   */
  async getTotalContests(): Promise<string | null> {
    return this.totalContests.textContent();
  }

  /**
   * Get total wins stat
   */
  async getTotalWins(): Promise<string | null> {
    return this.totalWins.textContent();
  }

  /**
   * Get total P&L stat
   */
  async getTotalPnL(): Promise<string | null> {
    return this.totalPnL.textContent();
  }

  /**
   * Get win rate stat
   */
  async getWinRate(): Promise<string | null> {
    return this.winRate.textContent();
  }

  /**
   * Check if success message is visible
   */
  async isSuccessMessageVisible(): Promise<boolean> {
    return this.successMessage.isVisible();
  }

  /**
   * Check if error message is visible
   */
  async isErrorMessageVisible(): Promise<boolean> {
    return this.errorMessage.isVisible();
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
   * Get error message text
   */
  async getErrorMessage(): Promise<string | null> {
    if (await this.errorMessage.isVisible()) {
      return this.errorMessage.textContent();
    }
    return null;
  }

  /**
   * Upload avatar
   */
  async uploadAvatar(filePath: string): Promise<void> {
    await this.avatarUpload.setInputFiles(filePath);
  }

  /**
   * Check if avatar is displayed
   */
  async isAvatarDisplayed(): Promise<boolean> {
    return this.avatar.isVisible();
  }
}
