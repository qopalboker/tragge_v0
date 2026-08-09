import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the User Frontend Dashboard Page
 */
export class DashboardPage {
  readonly page: Page;

  // Layout locators
  readonly header: Locator;
  readonly sidebar: Locator;
  readonly bottomNav: Locator;
  readonly mainContent: Locator;

  // Header locators
  readonly userMenu: Locator;
  readonly notificationBell: Locator;
  readonly languageToggle: Locator;

  // Sidebar locators
  readonly dashboardLink: Locator;
  readonly tournamentsLink: Locator;
  readonly myTournamentsLink: Locator;
  readonly profileLink: Locator;
  readonly logoutButton: Locator;

  // Dashboard content locators
  readonly welcomeMessage: Locator;
  readonly statCards: Locator;
  readonly contestCarousel: Locator;
  readonly leaderboardPreview: Locator;

  constructor(page: Page) {
    this.page = page;

    // Layout
    this.header = page.locator('.app-header');
    this.sidebar = page.locator('.app-sidebar');
    this.bottomNav = page.locator('.bottom-nav');
    this.mainContent = page.locator('main');

    // Header elements
    this.userMenu = page.locator('.user-menu');
    this.notificationBell = page.locator('.notification-bell, [data-testid="notifications"]');
    this.languageToggle = page.locator('.lang-toggle, [data-testid="language-toggle"]');

    // Sidebar navigation
    this.dashboardLink = page.locator('[data-nav="dashboard"], a[href*="/dashboard"]');
    this.tournamentsLink = page.locator('[data-nav="tournaments"], a[href*="/contests"]');
    this.myTournamentsLink = page.locator('[data-nav="my-tournaments"], a[href*="/my-tournaments"]');
    this.profileLink = page.locator('[data-nav="profile"], a[href*="/profile"]');
    this.logoutButton = page.locator('[data-action="logout"], .logout-btn');

    // Dashboard content
    this.welcomeMessage = page.locator('.welcome-message, h1');
    this.statCards = page.locator('.stat-card');
    this.contestCarousel = page.locator('.contest-carousel');
    this.leaderboardPreview = page.locator('.leaderboard-preview');
  }

  /**
   * Navigate to the dashboard
   */
  async goto(): Promise<void> {
    await this.page.goto('/user/dashboard');
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await this.page.waitForLoadState('networkidle');
    await expect(this.mainContent).toBeVisible();
  }

  /**
   * Get the welcome message text
   */
  async getWelcomeMessage(): Promise<string | null> {
    return this.welcomeMessage.textContent();
  }

  /**
   * Get the number of stat cards displayed
   */
  async getStatCardsCount(): Promise<number> {
    return this.statCards.count();
  }

  /**
   * Get stat card value by index
   */
  async getStatCardValue(index: number): Promise<string | null> {
    const card = this.statCards.nth(index);
    const value = card.locator('.stat-value');
    return value.textContent();
  }

  /**
   * Navigate to tournaments page
   */
  async navigateToTournaments(): Promise<void> {
    await this.tournamentsLink.click();
    await this.page.waitForURL(/\/contests/);
  }

  /**
   * Navigate to my tournaments page
   */
  async navigateToMyTournaments(): Promise<void> {
    await this.myTournamentsLink.click();
    await this.page.waitForURL(/\/my-tournaments/);
  }

  /**
   * Navigate to profile page
   */
  async navigateToProfile(): Promise<void> {
    await this.profileLink.click();
    await this.page.waitForURL(/\/profile/);
  }

  /**
   * Logout from the application
   */
  async logout(): Promise<void> {
    await this.logoutButton.click();
    await this.page.waitForURL(/\/login/);
  }

  /**
   * Toggle language
   */
  async toggleLanguage(): Promise<void> {
    await this.languageToggle.click();
  }

  /**
   * Check if contest carousel is visible
   */
  async isContestCarouselVisible(): Promise<boolean> {
    return this.contestCarousel.isVisible();
  }

  /**
   * Check if leaderboard preview is visible
   */
  async isLeaderboardPreviewVisible(): Promise<boolean> {
    return this.leaderboardPreview.isVisible();
  }

  /**
   * Check if user is authenticated (page is accessible)
   */
  async isAuthenticated(): Promise<boolean> {
    try {
      await expect(this.mainContent).toBeVisible({ timeout: 3000 });
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if current page is in mobile view (bottom nav visible)
   */
  async isMobileView(): Promise<boolean> {
    return this.bottomNav.isVisible();
  }
}
