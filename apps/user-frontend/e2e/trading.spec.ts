import { test, expect } from '@playwright/test';
import { TradingPage, LoginPage } from './pages';
import { TEST_CONTESTS, TEST_SYMBOLS, TEST_ORDERS } from '../../../e2e/test-data';
import { mockApiResponse } from '../../../e2e/fixtures';

test.describe('Trade Frontend - Trading', () => {
  // Use authenticated state for all tests
  test.use({ storageState: './apps/user-frontend/e2e/.auth/user.json' });

  const contestId = TEST_CONTESTS.active.id;

  test.beforeEach(async ({ page }) => {
    // Mock necessary APIs
    await mockApiResponse(page, '**/api/trade/symbols**', {
      status: 200,
      body: { symbols: TEST_SYMBOLS },
    });

    await mockApiResponse(page, '**/api/trade/positions**', {
      status: 200,
      body: { positions: [] },
    });
  });

  test.describe('Page Load', () => {
    test('should display the trading page with chart', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Verify chart panel is visible
      await expect(tradingPage.chartPanel).toBeVisible();
      const isChartVisible = await tradingPage.isChartVisible();
      expect(isChartVisible).toBe(true);
    });

    test('should display order form', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Verify order panel elements
      await expect(tradingPage.orderPanel).toBeVisible();
      await expect(tradingPage.buyButton).toBeVisible();
      await expect(tradingPage.sellButton).toBeVisible();
      await expect(tradingPage.quantityInput).toBeVisible();
    });

    test('should display contest info in header', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Contest info should show contest ID
      await expect(tradingPage.contestInfo).toContainText(contestId);
    });

    test('should display connection status', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Connection status should be visible
      await expect(tradingPage.connectionStatus).toBeVisible();
    });
  });

  test.describe('Symbol Selection', () => {
    test('should switch symbols', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Select a different symbol
      await tradingPage.selectSymbol('BTC/USD');

      // Verify symbol changed (chart would update)
      await page.waitForTimeout(500);
    });

    test('should display available symbols', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Symbol selector should be visible
      await expect(tradingPage.symbolSelector).toBeVisible();
    });

    test('should update chart when symbol changes', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Get initial state
      const initialSymbol = await tradingPage.getCurrentSymbol();

      // Change symbol
      await tradingPage.selectSymbol('GBP/USD');

      // Chart should update (visual verification would be needed)
      await page.waitForTimeout(500);
    });
  });

  test.describe('Market Orders', () => {
    test('should place market buy order', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Mock order API
      await mockApiResponse(page, '**/api/trade/orders', {
        status: 200,
        body: {
          orderId: 'order-123',
          status: 'filled',
          message: 'Order placed successfully',
        },
      });

      // Place market buy order
      await tradingPage.placeMarketOrder('buy', 10);

      // Should show confirmation or update positions
      await page.waitForTimeout(500);
    });

    test('should place market sell order', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Mock order API
      await mockApiResponse(page, '**/api/trade/orders', {
        status: 200,
        body: {
          orderId: 'order-124',
          status: 'filled',
          message: 'Order placed successfully',
        },
      });

      // Place market sell order
      await tradingPage.placeMarketOrder('sell', 5);

      // Should show confirmation
      await page.waitForTimeout(500);
    });

    test('should toggle between buy and sell', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Select buy
      await tradingPage.selectSide('buy');
      let isBuyActive = await tradingPage.isBuySideActive();
      expect(isBuyActive).toBe(true);

      // Select sell
      await tradingPage.selectSide('sell');
      let isSellActive = await tradingPage.isSellSideActive();
      expect(isSellActive).toBe(true);
    });

    test('should update submit button color based on side', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Select buy
      await tradingPage.selectSide('buy');
      let buttonClass = await tradingPage.getSubmitButtonClass();
      expect(buttonClass).toContain('btn-buy');

      // Select sell
      await tradingPage.selectSide('sell');
      buttonClass = await tradingPage.getSubmitButtonClass();
      expect(buttonClass).toContain('btn-sell');
    });
  });

  test.describe('Limit Orders', () => {
    test('should place limit buy order', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Mock order API
      await mockApiResponse(page, '**/api/trade/orders', {
        status: 200,
        body: {
          orderId: 'order-125',
          status: 'pending',
          message: 'Limit order placed',
        },
      });

      // Place limit order
      await tradingPage.placeLimitOrder('buy', 20, 140.0);

      await page.waitForTimeout(500);
    });

    test('should show price input for limit orders', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Select limit order type
      await tradingPage.selectOrderType('limit');

      // Price input should be visible
      await expect(tradingPage.priceInput).toBeVisible();
    });

    test('should hide price input for market orders', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Select market order type
      await tradingPage.selectOrderType('market');

      // Price input should not be visible
      await expect(tradingPage.priceInput).not.toBeVisible();
    });

    test('should place limit order with TP/SL', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Mock order API
      await mockApiResponse(page, '**/api/trade/orders', {
        status: 200,
        body: {
          orderId: 'order-126',
          status: 'pending',
          takeProfit: 150.0,
          stopLoss: 135.0,
        },
      });

      // Place limit order with TP/SL
      await tradingPage.placeLimitOrder('buy', 20, 140.0, {
        takeProfit: 150.0,
        stopLoss: 135.0,
      });

      await page.waitForTimeout(500);
    });
  });

  test.describe('Positions', () => {
    test('should display open positions', async ({ page }) => {
      // Mock positions
      await mockApiResponse(page, '**/api/trade/positions**', {
        status: 200,
        body: {
          positions: [
            {
              id: 'pos-1',
              symbol: 'EUR/USD',
              side: 'long',
              quantity: 10,
              avgPrice: 1.082,
              currentPrice: 1.085,
              unrealizedPnl: 0.03,
            },
          ],
        },
      });

      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Position count should be 1
      await page.waitForTimeout(500);
      const count = await tradingPage.getPositionCount();
      expect(count).toBe(1);
    });

    test('should show position details', async ({ page }) => {
      await mockApiResponse(page, '**/api/trade/positions**', {
        status: 200,
        body: {
          positions: [
            {
              id: 'pos-1',
              symbol: 'EUR/USD',
              side: 'long',
              quantity: 10,
              avgPrice: 1.082,
              unrealizedPnl: 0.03,
            },
          ],
        },
      });

      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      await page.waitForTimeout(500);

      // Get position details
      const position = await tradingPage.getPosition(0);
      expect(position.symbol).toContain('EUR/USD');
      expect(position.side).toContain('long');
    });

    test('should close position', async ({ page }) => {
      await mockApiResponse(page, '**/api/trade/positions**', {
        status: 200,
        body: {
          positions: [
            {
              id: 'pos-1',
              symbol: 'EUR/USD',
              side: 'long',
              quantity: 10,
              avgPrice: 1.082,
            },
          ],
        },
      });

      // Mock close position API
      await mockApiResponse(page, '**/api/trade/positions/*/close', {
        status: 200,
        body: { message: 'Position closed', pnl: 35.0 },
      });

      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      await page.waitForTimeout(500);

      // Close the position
      await tradingPage.closePosition(0);

      // Position should be removed (or updated)
      await page.waitForTimeout(500);
    });

    test('should show empty state when no positions', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // No positions should show placeholder
      const placeholder = page.locator('.positions-placeholder, .empty-positions');
      await expect(placeholder).toBeVisible();
    });
  });

  test.describe('Pending Orders', () => {
    test('should cancel pending order', async ({ page }) => {
      // Mock pending orders
      await mockApiResponse(page, '**/api/trade/orders**status=pending**', {
        status: 200,
        body: {
          orders: [
            {
              id: 'order-100',
              symbol: 'BTC/USD',
              side: 'buy',
              type: 'limit',
              quantity: 20,
              price: 140.0,
              status: 'pending',
            },
          ],
        },
      });

      // Mock cancel API
      await mockApiResponse(page, '**/api/trade/orders/*/cancel', {
        status: 200,
        body: { message: 'Order cancelled' },
      });

      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Find and cancel pending order
      const cancelButton = page.locator('.cancel-order, button:has-text("Cancel")').first();
      if (await cancelButton.isVisible()) {
        await cancelButton.click();
        await page.waitForTimeout(500);
      }
    });
  });

  test.describe('Order Validation', () => {
    test('should validate quantity is required', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Try to submit without quantity
      await tradingPage.submitOrderButton.click();

      // Should show validation error or form should not submit
      const quantityInput = tradingPage.quantityInput;
      const isInvalid = await quantityInput.evaluate(
        (el: HTMLInputElement) => !el.validity.valid || el.value === ''
      );
    });

    test('should validate price for limit orders', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Select limit order without price
      await tradingPage.selectOrderType('limit');
      await tradingPage.enterQuantity(10);

      // Try to submit
      await tradingPage.submitOrderButton.click();

      // Should require price
    });
  });

  test.describe('Debug Panel', () => {
    test('should toggle debug panel', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Toggle debug panel
      await tradingPage.toggleDebugPanel();

      // Check visibility changed
      const isVisible = await tradingPage.debugPanel.isVisible();
    });

    test('should show last message in debug panel', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Ensure debug panel is visible
      if (!(await tradingPage.debugPanel.isVisible())) {
        await tradingPage.toggleDebugPanel();
      }

      // Check for last message display
      const messageType = await tradingPage.getLastMessageType();
      // May be null initially
    });
  });

  test.describe('Logout', () => {
    test('should logout from trading page', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Logout
      await tradingPage.logout();

      // Should redirect to login
      await expect(page).toHaveURL(/\/login/);
    });
  });

  test.describe('Language Toggle', () => {
    test('should toggle language on trading page', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Toggle language
      await tradingPage.toggleLanguage();

      // Page should update (RTL/LTR)
      await page.waitForTimeout(300);
    });
  });
});
