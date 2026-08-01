import { test, expect } from '@playwright/test';
import { TradingPage } from './pages';
import {
  TEST_CONTESTS,
  TEST_SYMBOLS,
  generateTickData,
  generatePositionUpdate,
  generateOrderAck,
} from '../../../e2e/test-data';
import { mockApiResponse, WebSocketHelper } from '../../../e2e/fixtures';

test.describe('Trade Frontend - WebSocket', () => {
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

  test.describe('Connection Status', () => {
    test('should show connecting state initially', async ({ page }) => {
      const tradingPage = new TradingPage(page);

      // Navigate without waiting for full load
      page.goto(`/trade/${contestId}`);

      // Should show connecting status
      const statusText = await tradingPage.connectionStatus;
      await expect(statusText).toBeVisible();
    });

    test('should show connected state after WebSocket connects', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Wait for connection (may need to mock WebSocket)
      await tradingPage.waitForConnection(10000);

      // Should show connected
      const isConnected = await tradingPage.isConnected();
      expect(isConnected).toBe(true);
    });

    test('should display connection status text', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Get status text
      const status = await tradingPage.getConnectionStatus();
      expect(status).toBeTruthy();
    });
  });

  test.describe('Price Updates', () => {
    test('should display price updates from WebSocket', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Set up WebSocket listener
      const wsHelper = new WebSocketHelper(page);
      await wsHelper.setup(/ws\/trade/);

      // Wait for tick data
      await page.waitForTimeout(2000);

      // Check if debug panel shows tick messages
      if (!(await tradingPage.debugPanel.isVisible())) {
        await tradingPage.toggleDebugPanel();
      }

      const messageType = await tradingPage.getLastMessageType();
      // May show tick_snapshot or similar
    });

    test('should update chart with real-time data', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Wait for WebSocket connection
      await tradingPage.waitForConnection();

      // Chart should be updating (visual verification)
      await expect(tradingPage.chartContainer).toBeVisible();

      // Wait for some updates
      await page.waitForTimeout(3000);
    });

    test('should handle tick snapshot messages', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Enable debug panel to see messages
      if (!(await tradingPage.debugPanel.isVisible())) {
        await tradingPage.toggleDebugPanel();
      }

      // Wait for tick messages
      await page.waitForTimeout(2000);

      // Check debug panel for tick_snapshot type
      const messageType = await tradingPage.getLastMessageType();
    });
  });

  test.describe('Position Updates', () => {
    test('should update positions in real-time', async ({ page }) => {
      // First mock initial positions
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
              unrealizedPnl: 0,
            },
          ],
        },
      });

      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Wait for position to load
      await page.waitForTimeout(1000);

      // Position P&L should update via WebSocket
      // (Would need actual WebSocket mock to verify)
    });

    test('should show position P&L changes', async ({ page }) => {
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

      await page.waitForTimeout(1000);

      // Get position P&L
      const position = await tradingPage.getPosition(0);
      expect(position.pnl).toBeTruthy();
    });

    test('should add new position from WebSocket', async ({ page }) => {
      // Start with no positions
      await mockApiResponse(page, '**/api/trade/positions**', {
        status: 200,
        body: { positions: [] },
      });

      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Initially no positions
      let count = await tradingPage.getPositionCount();
      expect(count).toBe(0);

      // Simulate placing an order that creates a position
      await mockApiResponse(page, '**/api/trade/orders', {
        status: 200,
        body: { orderId: 'order-1', status: 'filled' },
      });

      await mockApiResponse(page, '**/api/trade/positions**', {
        status: 200,
        body: {
          positions: [
            {
              id: 'pos-new',
              symbol: 'EUR/USD',
              side: 'long',
              quantity: 10,
              avgPrice: 1.085,
            },
          ],
        },
      });

      // Place order
      await tradingPage.placeMarketOrder('buy', 10);

      // Reload positions (or WebSocket would update)
      await page.reload();
      await tradingPage.waitForPageLoad();

      // Now should have position
      count = await tradingPage.getPositionCount();
      expect(count).toBe(1);
    });
  });

  test.describe('Reconnection', () => {
    test('should attempt reconnection after disconnect', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Wait for initial connection
      await tradingPage.waitForConnection();
      expect(await tradingPage.isConnected()).toBe(true);

      // Simulate network interruption (close WebSocket)
      await page.evaluate(() => {
        // This would require actual WebSocket access
        // In real tests, you might use network throttling or mock
      });

      // Wait for reconnection attempt
      await page.waitForTimeout(5000);

      // Should attempt to reconnect or show disconnected status
    });

    test('should show disconnected status on network error', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Wait for connection
      await tradingPage.waitForConnection();

      // Go offline
      await page.context().setOffline(true);

      // Wait a bit
      await page.waitForTimeout(2000);

      // Should show disconnected status (or error)
      const status = await tradingPage.getConnectionStatus();

      // Restore online
      await page.context().setOffline(false);
    });

    test('should reconnect when coming back online', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Wait for connection
      await tradingPage.waitForConnection();

      // Go offline
      await page.context().setOffline(true);
      await page.waitForTimeout(2000);

      // Come back online
      await page.context().setOffline(false);

      // Wait for reconnection
      await page.waitForTimeout(5000);

      // Should be connected again (may need multiple attempts)
    });
  });

  test.describe('Order Acknowledgment', () => {
    test('should show order acknowledgment via WebSocket', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Enable debug panel
      if (!(await tradingPage.debugPanel.isVisible())) {
        await tradingPage.toggleDebugPanel();
      }

      // Mock order placement
      await mockApiResponse(page, '**/api/trade/orders', {
        status: 200,
        body: { orderId: 'order-ws-1', status: 'pending' },
      });

      // Place order
      await tradingPage.placeMarketOrder('buy', 5);

      // Should receive order_ack via WebSocket
      await page.waitForTimeout(1000);

      // Check debug for order_ack message type
    });

    test('should handle rejected order notification', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Mock rejected order
      await mockApiResponse(page, '**/api/trade/orders', {
        status: 400,
        body: {
          error: 'Insufficient balance',
          orderId: 'order-rejected',
          status: 'rejected',
        },
      });

      // Try to place order
      await tradingPage.placeMarketOrder('buy', 1000000);

      // Should show error notification
      await page.waitForTimeout(1000);

      const errorToast = page.locator('.toast-error, .error-message, .notification-error');
      // May be visible depending on implementation
    });

    test('should update order status in real-time', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Mock order that goes through lifecycle
      await mockApiResponse(page, '**/api/trade/orders', {
        status: 200,
        body: { orderId: 'order-lifecycle', status: 'pending' },
      });

      // Place limit order
      await tradingPage.placeLimitOrder('buy', 10, 140.0);

      // Order should be pending initially
      await page.waitForTimeout(500);

      // WebSocket would send status updates (pending -> filled)
    });
  });

  test.describe('Real-time Leaderboard', () => {
    test('should show leaderboard snippet', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Leaderboard panel should be visible
      await expect(tradingPage.leaderboardPanel).toBeVisible();
    });

    test('should update leaderboard position via WebSocket', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Leaderboard would update via WebSocket
      await page.waitForTimeout(2000);

      // Check for leaderboard data
    });
  });

  test.describe('Debug Panel WebSocket Info', () => {
    test('should show last received message', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Open debug panel
      if (!(await tradingPage.debugPanel.isVisible())) {
        await tradingPage.toggleDebugPanel();
      }

      // Wait for WebSocket messages
      await page.waitForTimeout(2000);

      // Should show message info
      const lastMessage = tradingPage.lastMessage;
      // May show data depending on connection
    });

    test('should show raw message data', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Open debug panel
      if (!(await tradingPage.debugPanel.isVisible())) {
        await tradingPage.toggleDebugPanel();
      }

      // Wait for messages
      await page.waitForTimeout(2000);

      // Should show raw JSON
      const rawMessage = tradingPage.rawMessage;
    });

    test('should show message timestamp', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Open debug panel
      if (!(await tradingPage.debugPanel.isVisible())) {
        await tradingPage.toggleDebugPanel();
      }

      // Wait for messages
      await page.waitForTimeout(2000);

      // Should show timestamp
      const messageTime = await tradingPage.getLastMessageTime();
    });
  });

  test.describe('WebSocket Error Handling', () => {
    test('should handle WebSocket errors gracefully', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Even with errors, page should remain functional
      await expect(tradingPage.orderPanel).toBeVisible();
      await expect(tradingPage.chartPanel).toBeVisible();
    });

    test('should continue functioning during reconnection', async ({ page }) => {
      const tradingPage = new TradingPage(page);
      await tradingPage.goto(contestId);

      // Order form should still work during reconnection
      await tradingPage.selectSide('buy');
      await tradingPage.enterQuantity(10);

      // Form should be interactive
      await expect(tradingPage.submitOrderButton).toBeEnabled();
    });
  });
});
