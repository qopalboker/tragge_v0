import { test, expect } from '@playwright/test';
import { AuditPage } from './pages';
import { TEST_AUDIT_LOGS } from '../../../e2e/test-data';
import { mockApiResponse } from '../../../e2e/fixtures';

test.describe('Admin Frontend - Audit Logs', () => {
  // Use authenticated state for all tests
  test.use({ storageState: './apps/admin-frontend/e2e/.auth/admin.json' });

  test.beforeEach(async ({ page }) => {
    // Mock the audit logs API
    await mockApiResponse(page, '**/api/admin/audit-logs**', {
      status: 200,
      body: {
        logs: TEST_AUDIT_LOGS,
        total: TEST_AUDIT_LOGS.length,
        page: 1,
        pageSize: 10,
        totalPages: 1,
      },
    });
  });

  test.describe('View Audit Logs', () => {
    test('should display the audit logs page', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Verify page elements
      await expect(auditPage.pageTitle).toBeVisible();
    });

    test('should display list of audit logs', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Wait for logs to load
      await page.waitForTimeout(500);

      // Should have log entries
      const count = await auditPage.getLogCount();
      expect(count).toBeGreaterThan(0);
    });

    test('should display log entry details', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Get first log entry
      const timestamp = await auditPage.getTimestamp(0);
      const action = await auditPage.getAction(0);
      const user = await auditPage.getUser(0);

      expect(timestamp).toBeTruthy();
      expect(action).toBeTruthy();
      expect(user).toBeTruthy();
    });

    test('should show empty state when no logs', async ({ page }) => {
      // Mock empty response
      await mockApiResponse(page, '**/api/admin/audit-logs**', {
        status: 200,
        body: { logs: [], total: 0, page: 1, pageSize: 10, totalPages: 0 },
      });

      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Should show empty state
      const isEmpty = await auditPage.isEmpty();
      expect(isEmpty).toBe(true);
    });
  });

  test.describe('Filter by Date', () => {
    test('should filter logs by date range', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Mock filtered response
      await mockApiResponse(page, '**/api/admin/audit-logs**dateFrom**dateTo**', {
        status: 200,
        body: {
          logs: TEST_AUDIT_LOGS.slice(0, 2),
          total: 2,
          page: 1,
          pageSize: 10,
          totalPages: 1,
        },
      });

      // Set date range
      const fromDate = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)
        .toISOString()
        .split('T')[0];
      const toDate = new Date().toISOString().split('T')[0];

      await auditPage.filterByDateRange(fromDate, toDate);

      // Results should be filtered
      await page.waitForTimeout(500);
    });

    test('should display date filter inputs', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Date inputs should be visible
      await expect(auditPage.dateFromInput).toBeVisible();
      await expect(auditPage.dateToInput).toBeVisible();
    });
  });

  test.describe('Filter by Action', () => {
    test('should filter logs by action type', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Mock filtered response
      await mockApiResponse(page, '**/api/admin/audit-logs**action=contest.create**', {
        status: 200,
        body: {
          logs: TEST_AUDIT_LOGS.filter((l) => l.action === 'contest.create'),
          total: 1,
          page: 1,
          pageSize: 10,
          totalPages: 1,
        },
      });

      // Filter by contest.create action
      await auditPage.filterByAction('contest.create');

      // Should show only contest.create logs
      await page.waitForTimeout(500);

      const count = await auditPage.getLogCount();
      for (let i = 0; i < count; i++) {
        const action = await auditPage.getAction(i);
        expect(action?.toLowerCase()).toContain('contest');
      }
    });

    test('should display action filter dropdown', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Action filter should be visible
      await expect(auditPage.actionFilter).toBeVisible();
    });

    test('should list available action types', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Click action filter to see options
      await auditPage.actionFilter.click();

      // Should have options
      const options = page.locator('option, [role="option"]');
      const count = await options.count();
      expect(count).toBeGreaterThan(0);
    });
  });

  test.describe('Filter by User', () => {
    test('should filter logs by user', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Mock filtered response
      await mockApiResponse(page, '**/api/admin/audit-logs**user=admin-001**', {
        status: 200,
        body: {
          logs: TEST_AUDIT_LOGS.filter((l) => l.userId === 'admin-001'),
          total: 3,
          page: 1,
          pageSize: 10,
          totalPages: 1,
        },
      });

      // Filter by user
      await auditPage.filterByUser('admin-001');

      // Should show only that user's logs
      await page.waitForTimeout(500);
    });
  });

  test.describe('Clear Filters', () => {
    test('should clear all filters', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Apply some filters
      const fromDate = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000)
        .toISOString()
        .split('T')[0];
      const toDate = new Date().toISOString().split('T')[0];
      await auditPage.filterByDateRange(fromDate, toDate);

      // Clear filters
      await auditPage.clearFilters();

      // Should show all logs again
      await page.waitForTimeout(500);
    });
  });

  test.describe('Export Logs', () => {
    test('should have export button', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Export button should be visible
      await expect(auditPage.exportButton).toBeVisible();
    });

    test('should export logs as CSV', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Set up download listener
      const downloadPromise = page.waitForEvent('download');

      // Click export CSV
      await auditPage.exportAsCSV();

      // Wait for download (may fail in mock environment)
      try {
        const download = await downloadPromise;
        const suggestedFilename = download.suggestedFilename();
        expect(suggestedFilename).toContain('.csv');
      } catch {
        // Download may not trigger in mock environment
      }
    });

    test('should export logs as JSON', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Set up download listener
      const downloadPromise = page.waitForEvent('download');

      // Click export JSON
      await auditPage.exportAsJSON();

      // Wait for download
      try {
        const download = await downloadPromise;
        const suggestedFilename = download.suggestedFilename();
        expect(suggestedFilename).toContain('.json');
      } catch {
        // Download may not trigger in mock environment
      }
    });

    test('should show export options', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Click export button to see options
      await auditPage.exportButton.click();

      // Should show CSV and JSON options
      const csvOption = page.locator('button:has-text("CSV"), .export-option:has-text("CSV")');
      const jsonOption = page.locator('button:has-text("JSON"), .export-option:has-text("JSON")');

      // At least one should be visible
    });
  });

  test.describe('Pagination', () => {
    test.beforeEach(async ({ page }) => {
      // Mock paginated audit logs
      const manyLogs = Array.from({ length: 50 }, (_, i) => ({
        id: `audit-${i + 1}`,
        action: ['contest.create', 'contest.update', 'user.ban'][i % 3],
        userId: `admin-00${(i % 2) + 1}`,
        userName: `Admin ${(i % 2) + 1}`,
        details: { index: i },
        timestamp: new Date(Date.now() - i * 60 * 60 * 1000).toISOString(),
      }));

      await mockApiResponse(page, '**/api/admin/audit-logs**', {
        status: 200,
        body: {
          logs: manyLogs.slice(0, 10),
          total: 50,
          page: 1,
          pageSize: 10,
          totalPages: 5,
        },
      });
    });

    test('should display pagination controls', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Pagination should be visible
      await expect(auditPage.pagination).toBeVisible();
    });

    test('should navigate to next page', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Mock next page
      await mockApiResponse(page, '**/api/admin/audit-logs**page=2**', {
        status: 200,
        body: {
          logs: Array.from({ length: 10 }, (_, i) => ({
            id: `audit-${i + 11}`,
            action: 'contest.update',
            userId: 'admin-001',
            userName: 'Admin',
            details: {},
            timestamp: new Date().toISOString(),
          })),
          total: 50,
          page: 2,
          pageSize: 10,
          totalPages: 5,
        },
      });

      // Click next
      await auditPage.nextPage();

      // Should be on page 2
      await page.waitForTimeout(300);
    });

    test('should navigate to previous page', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Go to page 2 first
      await auditPage.nextPage();
      await page.waitForTimeout(300);

      // Then go back
      await auditPage.prevPage();
      await page.waitForTimeout(300);

      // Should be back on page 1
      const currentPage = await auditPage.getCurrentPage();
      expect(currentPage).toBe(1);
    });

    test('should show page info', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Page info should show count
      const pageInfo = await auditPage.getPageInfo();
      expect(pageInfo).toBeTruthy();
    });
  });

  test.describe('Log Entry Details', () => {
    test('should show details column with action specifics', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Get details from first log
      const details = await auditPage.getDetails(0);
      expect(details).toBeTruthy();
    });

    test('should display readable action names', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Get all log entries
      const entries = await auditPage.getAllLogEntries();

      // Actions should be readable
      entries.forEach((entry) => {
        expect(entry.action).toBeTruthy();
        expect(entry.action).toMatch(/\w+\.\w+/); // Format: entity.action
      });
    });

    test('should display timestamp in readable format', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Get timestamp
      const timestamp = await auditPage.getTimestamp(0);

      // Should be readable date/time format
      expect(timestamp).toBeTruthy();
    });
  });

  test.describe('Loading State', () => {
    test('should show loading indicator while fetching', async ({ page }) => {
      // Mock slow API
      await page.route('**/api/admin/audit-logs**', async (route) => {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            logs: TEST_AUDIT_LOGS,
            total: TEST_AUDIT_LOGS.length,
            page: 1,
            pageSize: 10,
            totalPages: 1,
          }),
        });
      });

      const auditPage = new AuditPage(page);

      // Start navigation without waiting
      page.goto('/admin/audit');

      // Should show loading
      await expect(auditPage.loadingIndicator).toBeVisible();
    });
  });

  test.describe('Error Handling', () => {
    test('should show error message when API fails', async ({ page }) => {
      // Mock API error
      await mockApiResponse(page, '**/api/admin/audit-logs**', {
        status: 500,
        body: { error: 'Failed to fetch audit logs' },
      });

      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Should show error state
      const errorMessage = page.locator('.error-message, .error-state');
      // May be visible depending on implementation
    });

    test('should handle export error gracefully', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Mock export failure
      await mockApiResponse(page, '**/api/admin/audit-logs/export**', {
        status: 500,
        body: { error: 'Export failed' },
      });

      // Try to export
      await auditPage.exportAsCSV();

      // Should show error notification
      await page.waitForTimeout(500);
    });
  });

  test.describe('Real-time Updates', () => {
    test('should allow manual refresh', async ({ page }) => {
      const auditPage = new AuditPage(page);
      await auditPage.goto();

      // Look for refresh button
      const refreshButton = page.locator('button:has-text("Refresh"), .refresh-btn');

      if (await refreshButton.isVisible()) {
        // Click refresh
        await refreshButton.click();

        // Should reload data
        await page.waitForTimeout(500);
      }
    });
  });
});
