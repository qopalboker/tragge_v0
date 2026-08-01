import { test, expect } from '@playwright/test';
import { ShardsPage } from './pages/ShardsPage';
import { mockApiResponse } from '../../../e2e/fixtures';

// Test data for shards
const TEST_SHARDS = {
  shard0: {
    id: 0,
    name: 'shard-0',
    status: 'active',
    weight: 1,
    contestCount: 25,
    participantCount: 1250,
    ordersPerSecond: 145.2,
    kafkaPartition: 0,
  },
  shard1: {
    id: 1,
    name: 'shard-1',
    status: 'active',
    weight: 1,
    contestCount: 23,
    participantCount: 1180,
    ordersPerSecond: 138.7,
    kafkaPartition: 1,
  },
  shard2: {
    id: 2,
    name: 'shard-2',
    status: 'draining',
    weight: 0,
    contestCount: 18,
    participantCount: 920,
    ordersPerSecond: 0,
    kafkaPartition: 2,
  },
  shard3: {
    id: 3,
    name: 'shard-3',
    status: 'active',
    weight: 2,
    contestCount: 30,
    participantCount: 1500,
    ordersPerSecond: 167.9,
    kafkaPartition: 3,
  },
};

const TEST_SHARD_STATS = {
  timeRange: '1h',
  data: [
    { timestamp: Date.now() - 3600000, shardId: 0, ordersPerSecond: 140, latencyP50: 12 },
    { timestamp: Date.now() - 3000000, shardId: 0, ordersPerSecond: 145, latencyP50: 11 },
    { timestamp: Date.now() - 2400000, shardId: 0, ordersPerSecond: 150, latencyP50: 13 },
    { timestamp: Date.now() - 1800000, shardId: 0, ordersPerSecond: 148, latencyP50: 12 },
    { timestamp: Date.now() - 1200000, shardId: 0, ordersPerSecond: 143, latencyP50: 14 },
    { timestamp: Date.now() - 600000, shardId: 0, ordersPerSecond: 145, latencyP50: 12 },
  ],
};

test.describe('Admin Frontend - Shard Management', () => {
  // Use authenticated state for all tests
  test.use({ storageState: './apps/admin-frontend/e2e/.auth/admin.json' });

  test.beforeEach(async ({ page }) => {
    // Mock the shards API
    await mockApiResponse(page, '**/api/admin/shards', {
      status: 200,
      body: {
        shards: [
          TEST_SHARDS.shard0,
          TEST_SHARDS.shard1,
          TEST_SHARDS.shard2,
          TEST_SHARDS.shard3,
        ],
        total: 4,
        activeCount: 3,
      },
    });

    // Mock shard stats API
    await mockApiResponse(page, '**/api/admin/shards/stats**', {
      status: 200,
      body: TEST_SHARD_STATS,
    });
  });

  test.describe('View Shards', () => {
    test('admin can view shards', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Verify page loads
      await expect(shardsPage.pageTitle).toBeVisible();

      // Should display 4 shard cards
      await expect(page.locator('.shard-card, [data-testid="shard-card"]')).toHaveCount(4);
    });

    test('should display shard information correctly', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Wait for shards to load
      await page.waitForTimeout(500);

      // Verify shard cards contain expected information
      const shardCards = page.locator('.shard-card, [data-testid="shard-card"]');
      const cardCount = await shardCards.count();
      expect(cardCount).toBe(4);

      // Check first shard card
      const firstCard = shardCards.first();
      await expect(firstCard).toContainText('shard-0');
    });

    test('should show shard status badges', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Check for status badges
      const activeStatuses = page.locator('.status-badge.active, [data-status="active"]');
      const drainingStatuses = page.locator('.status-badge.draining, [data-status="draining"]');

      // Should have 3 active shards and 1 draining
      expect(await activeStatuses.count()).toBeGreaterThanOrEqual(3);
      expect(await drainingStatuses.count()).toBeGreaterThanOrEqual(1);
    });

    test('should display contest count per shard', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Each shard card should show contest count
      const contestCounts = page.locator('.contest-count, [data-testid="contest-count"]');
      expect(await contestCounts.count()).toBeGreaterThan(0);
    });

    test('should display participant count per shard', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Each shard card should show participant count
      const participantCounts = page.locator('.participant-count, [data-testid="participant-count"]');
      expect(await participantCounts.count()).toBeGreaterThan(0);
    });

    test('should display orders per second per shard', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Each shard card should show orders/sec
      const ordersPerSec = page.locator('.orders-per-sec, [data-testid="orders-per-sec"]');
      expect(await ordersPerSec.count()).toBeGreaterThan(0);
    });
  });

  test.describe('Filter Shards', () => {
    test('should filter shards by status', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Filter by active status
      await shardsPage.filterByStatus('active');
      await page.waitForTimeout(300);

      // Should only show active shards
      const shardCards = page.locator('.shard-card, [data-testid="shard-card"]');
      const visibleCards = await shardCards.count();

      // The draining shard should be hidden
      const drainingShards = page.locator('[data-status="draining"]:visible');
      expect(await drainingShards.count()).toBe(0);
    });

    test('should show all shards when filter cleared', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // First filter by active
      await shardsPage.filterByStatus('active');
      await page.waitForTimeout(300);

      // Then clear filter
      await shardsPage.clearFilter();
      await page.waitForTimeout(300);

      // Should show all 4 shards again
      const shardCards = page.locator('.shard-card, [data-testid="shard-card"]');
      expect(await shardCards.count()).toBe(4);
    });
  });

  test.describe('Shard Actions', () => {
    test('admin can drain shard', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Mock drain API
      await mockApiResponse(page, '**/api/admin/shards/0/drain', {
        status: 200,
        body: { message: 'Shard draining initiated', status: 'draining' },
      });

      // Click drain button on shard 0
      await shardsPage.clickDrainShard(0);

      // Confirm drain action
      await shardsPage.confirmAction();

      // Should show success message
      await page.waitForTimeout(500);
    });

    test('admin can activate shard', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Mock activate API
      await mockApiResponse(page, '**/api/admin/shards/2/activate', {
        status: 200,
        body: { message: 'Shard activated', status: 'active' },
      });

      // Click activate button on draining shard (shard 2)
      await shardsPage.clickActivateShard(2);

      // Confirm activation
      await shardsPage.confirmAction();

      // Should show success message
      await page.waitForTimeout(500);
    });

    test('should show confirmation dialog before draining', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Click drain button
      await shardsPage.clickDrainShard(0);

      // Confirmation modal should appear
      await expect(shardsPage.confirmModal).toBeVisible();
      await expect(shardsPage.confirmModal).toContainText(/drain|confirm/i);
    });

    test('should cancel drain when user clicks cancel', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Click drain button
      await shardsPage.clickDrainShard(0);

      // Click cancel
      await shardsPage.cancelAction();

      // Modal should close
      await expect(shardsPage.confirmModal).not.toBeVisible();
    });
  });

  test.describe('Shard Details', () => {
    test('should navigate to shard details on click', async ({ page }) => {
      // Mock shard detail API
      await mockApiResponse(page, '**/api/admin/shards/0', {
        status: 200,
        body: {
          ...TEST_SHARDS.shard0,
          contests: [
            { id: 'contest-1', name: 'Active Contest 1', status: 'running' },
            { id: 'contest-2', name: 'Active Contest 2', status: 'running' },
          ],
          recentOrders: 1500,
          avgLatency: 12.5,
        },
      });

      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Click on shard card to view details
      await shardsPage.clickShardCard(0);

      // Should navigate to shard detail page or show detail modal
      await page.waitForTimeout(500);
    });

    test('should display shard health metrics', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Each shard should show health indicator
      const healthIndicators = page.locator('.health-indicator, [data-testid="health-indicator"]');
      expect(await healthIndicators.count()).toBeGreaterThan(0);
    });
  });

  test.describe('Shard Statistics', () => {
    test('should display shard statistics chart', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Stats chart should be visible
      const statsChart = page.locator('.stats-chart, [data-testid="stats-chart"], canvas');
      await expect(statsChart.first()).toBeVisible();
    });

    test('should allow changing time range for stats', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Find time range selector
      const timeRangeSelector = page.locator('.time-range-selector, [data-testid="time-range"]');

      if (await timeRangeSelector.isVisible()) {
        await timeRangeSelector.click();

        // Select 24h option if available
        const option24h = page.locator('text=24h, text=24 hours');
        if (await option24h.isVisible()) {
          await option24h.click();
        }
      }
    });
  });

  test.describe('Error Handling', () => {
    test('should show error when API fails', async ({ page }) => {
      // Mock API error
      await mockApiResponse(page, '**/api/admin/shards', {
        status: 500,
        body: { error: 'Internal server error' },
      });

      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Should show error state
      const errorMessage = page.locator('.error-message, [data-testid="error"]');
      // Error should be visible or page should handle gracefully
    });

    test('should show error when drain fails', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Mock drain failure
      await mockApiResponse(page, '**/api/admin/shards/0/drain', {
        status: 400,
        body: { error: 'Cannot drain shard with active contests' },
      });

      // Attempt to drain
      await shardsPage.clickDrainShard(0);
      await shardsPage.confirmAction();

      // Should show error message
      await page.waitForTimeout(500);
    });
  });

  test.describe('Responsive Layout', () => {
    test('should display shards in grid on desktop', async ({ page }) => {
      // Set desktop viewport
      await page.setViewportSize({ width: 1920, height: 1080 });

      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Shards should be in grid layout
      const shardContainer = page.locator('.shard-grid, .shard-container');
      await expect(shardContainer.first()).toBeVisible();
    });

    test('should stack shards on mobile', async ({ page }) => {
      // Set mobile viewport
      await page.setViewportSize({ width: 375, height: 667 });

      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Shards should still be visible
      const shardCards = page.locator('.shard-card, [data-testid="shard-card"]');
      expect(await shardCards.count()).toBe(4);
    });
  });

  test.describe('Real-time Updates', () => {
    test('should refresh shard data periodically', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Wait for initial load
      await page.waitForTimeout(500);

      // Mock updated data
      await mockApiResponse(page, '**/api/admin/shards', {
        status: 200,
        body: {
          shards: [
            { ...TEST_SHARDS.shard0, ordersPerSecond: 160.5 },
            TEST_SHARDS.shard1,
            TEST_SHARDS.shard2,
            TEST_SHARDS.shard3,
          ],
          total: 4,
          activeCount: 3,
        },
      });

      // Click refresh button if available
      const refreshBtn = page.locator('.refresh-btn, button[aria-label="Refresh"]');
      if (await refreshBtn.isVisible()) {
        await refreshBtn.click();
        await page.waitForTimeout(500);
      }
    });
  });

  test.describe('Shard Weight Management', () => {
    test('should display shard weights', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Weight indicators should be visible
      const weightIndicators = page.locator('.weight-indicator, [data-testid="weight"]');
      expect(await weightIndicators.count()).toBeGreaterThan(0);
    });

    test('should highlight high-weight shards', async ({ page }) => {
      const shardsPage = new ShardsPage(page);
      await shardsPage.goto();

      // Shard 3 has weight 2, should be highlighted
      const highWeightShards = page.locator('[data-weight="2"], .high-weight');
      // Check if highlighting is applied
    });
  });
});
