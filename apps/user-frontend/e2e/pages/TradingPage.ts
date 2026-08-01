import { Page, Locator, expect } from '@playwright/test';

/**
 * Page Object for the Trade Frontend Trading Page
 */
export class TradingPage {
  readonly page: Page;

  // Header elements
  readonly header: Locator;
  readonly logo: Locator;
  readonly contestInfo: Locator;
  readonly connectionStatus: Locator;
  readonly statusDot: Locator;
  readonly statusText: Locator;
  readonly languageToggle: Locator;
  readonly debugToggle: Locator;
  readonly logoutButton: Locator;

  // Chart section
  readonly chartPanel: Locator;
  readonly symbolSelector: Locator;
  readonly symbolDropdown: Locator;
  readonly chartContainer: Locator;

  // Order form
  readonly orderPanel: Locator;
  readonly buyButton: Locator;
  readonly sellButton: Locator;
  readonly marketTypeButton: Locator;
  readonly limitTypeButton: Locator;
  readonly quantityInput: Locator;
  readonly priceInput: Locator;
  readonly takeProfitInput: Locator;
  readonly stopLossInput: Locator;
  readonly submitOrderButton: Locator;

  // Positions panel
  readonly positionsPanel: Locator;
  readonly positionsList: Locator;
  readonly positionRows: Locator;
  readonly closePositionButton: Locator;

  // Leaderboard panel
  readonly leaderboardPanel: Locator;

  // Debug panel
  readonly debugPanel: Locator;
  readonly lastMessage: Locator;
  readonly rawMessage: Locator;

  constructor(page: Page) {
    this.page = page;

    // Header
    this.header = page.locator('.trading-header');
    this.logo = page.locator('.logo');
    this.contestInfo = page.locator('.contest-info');
    this.connectionStatus = page.locator('.connection-status');
    this.statusDot = page.locator('.status-dot');
    this.statusText = page.locator('.status-text');
    this.languageToggle = page.locator('.btn-ghost:has-text("EN"), .btn-ghost:has-text("فا")');
    this.debugToggle = page.locator('.btn-ghost:has-text("Debug")');
    this.logoutButton = page.locator('.btn-secondary:has-text("Logout"), .btn-secondary:has-text("خروج")');

    // Chart section
    this.chartPanel = page.locator('.chart-panel');
    this.symbolSelector = page.locator('.symbol-selector');
    this.symbolDropdown = page.locator('.symbol-dropdown, select');
    this.chartContainer = page.locator('.chart-body, .chart-container');

    // Order form
    this.orderPanel = page.locator('.order-panel');
    this.buyButton = page.locator('.buy-btn, button:has-text("Buy")');
    this.sellButton = page.locator('.sell-btn, button:has-text("Sell")');
    this.marketTypeButton = page.locator('.type-btn:has-text("Market")');
    this.limitTypeButton = page.locator('.type-btn:has-text("Limit")');
    this.quantityInput = page.locator('.order-panel input[type="number"]').first();
    this.priceInput = page.locator('.order-panel input[type="number"]').nth(1);
    this.takeProfitInput = page.locator('input[name="takeProfit"], input[placeholder*="TP"]');
    this.stopLossInput = page.locator('input[name="stopLoss"], input[placeholder*="SL"]');
    this.submitOrderButton = page.locator('.submit-order-btn');

    // Positions
    this.positionsPanel = page.locator('.positions-panel');
    this.positionsList = page.locator('.positions-list');
    this.positionRows = page.locator('.position-row, .positions-panel tr');
    this.closePositionButton = page.locator('.close-position, button:has-text("Close")');

    // Leaderboard
    this.leaderboardPanel = page.locator('.leaderboard-panel');

    // Debug
    this.debugPanel = page.locator('.debug-panel');
    this.lastMessage = page.locator('.debug-message');
    this.rawMessage = page.locator('.message-raw');
  }

  /**
   * Navigate to the trading page
   */
  async goto(contestId?: string): Promise<void> {
    const url = contestId ? `/trade/${contestId}` : '/trade';
    await this.page.goto(url);
    await this.waitForPageLoad();
  }

  /**
   * Wait for the page to fully load
   */
  async waitForPageLoad(): Promise<void> {
    await expect(this.chartPanel).toBeVisible();
    await expect(this.orderPanel).toBeVisible();
  }

  /**
   * Wait for WebSocket connection
   */
  async waitForConnection(timeout = 10000): Promise<void> {
    await expect(this.statusDot).toHaveClass(/connected/, { timeout });
  }

  /**
   * Get connection status
   */
  async getConnectionStatus(): Promise<string | null> {
    return this.statusText.textContent();
  }

  /**
   * Check if connected
   */
  async isConnected(): Promise<boolean> {
    const className = await this.statusDot.getAttribute('class');
    return className?.includes('connected') ?? false;
  }

  /**
   * Select a symbol
   */
  async selectSymbol(symbol: string): Promise<void> {
    await this.symbolSelector.click();
    const option = this.page.locator(`.symbol-option:has-text("${symbol}"), option[value="${symbol}"]`);
    await option.click();
  }

  /**
   * Get current symbol
   */
  async getCurrentSymbol(): Promise<string | null> {
    const selectedSymbol = this.symbolSelector.locator('.selected, .current');
    return selectedSymbol.textContent();
  }

  /**
   * Select order side (buy/sell)
   */
  async selectSide(side: 'buy' | 'sell'): Promise<void> {
    if (side === 'buy') {
      await this.buyButton.click();
    } else {
      await this.sellButton.click();
    }
  }

  /**
   * Select order type (market/limit)
   */
  async selectOrderType(type: 'market' | 'limit'): Promise<void> {
    if (type === 'market') {
      await this.marketTypeButton.click();
    } else {
      await this.limitTypeButton.click();
    }
  }

  /**
   * Enter quantity
   */
  async enterQuantity(quantity: number): Promise<void> {
    await this.quantityInput.fill(String(quantity));
  }

  /**
   * Enter limit price
   */
  async enterPrice(price: number): Promise<void> {
    await this.priceInput.fill(String(price));
  }

  /**
   * Enter take profit
   */
  async enterTakeProfit(price: number): Promise<void> {
    if (await this.takeProfitInput.isVisible()) {
      await this.takeProfitInput.fill(String(price));
    }
  }

  /**
   * Enter stop loss
   */
  async enterStopLoss(price: number): Promise<void> {
    if (await this.stopLossInput.isVisible()) {
      await this.stopLossInput.fill(String(price));
    }
  }

  /**
   * Place a market order
   */
  async placeMarketOrder(side: 'buy' | 'sell', quantity: number): Promise<void> {
    await this.selectSide(side);
    await this.selectOrderType('market');
    await this.enterQuantity(quantity);
    await this.submitOrderButton.click();
  }

  /**
   * Place a limit order
   */
  async placeLimitOrder(
    side: 'buy' | 'sell',
    quantity: number,
    price: number,
    options?: { takeProfit?: number; stopLoss?: number }
  ): Promise<void> {
    await this.selectSide(side);
    await this.selectOrderType('limit');
    await this.enterQuantity(quantity);
    await this.enterPrice(price);

    if (options?.takeProfit) {
      await this.enterTakeProfit(options.takeProfit);
    }

    if (options?.stopLoss) {
      await this.enterStopLoss(options.stopLoss);
    }

    await this.submitOrderButton.click();
  }

  /**
   * Get number of open positions
   */
  async getPositionCount(): Promise<number> {
    return this.positionRows.count();
  }

  /**
   * Close position at index
   */
  async closePosition(index = 0): Promise<void> {
    const row = this.positionRows.nth(index);
    const closeBtn = row.locator('.close-position, button:has-text("Close")');
    await closeBtn.click();
  }

  /**
   * Get position details
   */
  async getPosition(index: number): Promise<{
    symbol: string | null;
    side: string | null;
    quantity: string | null;
    pnl: string | null;
  }> {
    const row = this.positionRows.nth(index);
    return {
      symbol: await row.locator('.position-symbol, td:nth-child(1)').textContent(),
      side: await row.locator('.position-side, td:nth-child(2)').textContent(),
      quantity: await row.locator('.position-qty, td:nth-child(3)').textContent(),
      pnl: await row.locator('.position-pnl, td:nth-child(4)').textContent(),
    };
  }

  /**
   * Toggle debug panel
   */
  async toggleDebugPanel(): Promise<void> {
    await this.debugToggle.click();
  }

  /**
   * Get last WebSocket message type
   */
  async getLastMessageType(): Promise<string | null> {
    if (await this.debugPanel.isVisible()) {
      const messageType = this.debugPanel.locator('.message-type');
      return messageType.textContent();
    }
    return null;
  }

  /**
   * Get last WebSocket message timestamp
   */
  async getLastMessageTime(): Promise<string | null> {
    if (await this.debugPanel.isVisible()) {
      const messageTime = this.debugPanel.locator('.message-time');
      return messageTime.textContent();
    }
    return null;
  }

  /**
   * Logout
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
   * Check if chart is visible
   */
  async isChartVisible(): Promise<boolean> {
    return this.chartContainer.isVisible();
  }

  /**
   * Check if buy button is active
   */
  async isBuySideActive(): Promise<boolean> {
    const className = await this.buyButton.getAttribute('class');
    return className?.includes('active') ?? false;
  }

  /**
   * Check if sell button is active
   */
  async isSellSideActive(): Promise<boolean> {
    const className = await this.sellButton.getAttribute('class');
    return className?.includes('active') ?? false;
  }

  /**
   * Get submit button text
   */
  async getSubmitButtonText(): Promise<string | null> {
    return this.submitOrderButton.textContent();
  }

  /**
   * Get submit button class (for buy/sell color)
   */
  async getSubmitButtonClass(): Promise<string | null> {
    return this.submitOrderButton.getAttribute('class');
  }
}
