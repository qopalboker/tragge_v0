import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright E2E Test Configuration for Tragge Trading Platform
 *
 * Post-split layout (step 9): user + trade tests live in
 * apps/user-frontend/e2e, admin tests in apps/admin-frontend/e2e.
 * Playwright's projects select by testDir so the shared setup file
 * runs against the right origin for each panel.
 *
 * @see https://playwright.dev/docs/test-configuration
 */

const CI = !!process.env.CI;
const USER_BASE_URL = process.env.E2E_USER_URL || 'http://localhost:5173';
const ADMIN_BASE_URL = process.env.E2E_ADMIN_URL || 'http://localhost:5174';
const SEC007_CHROME_PATH = process.env.SEC007_CHROME_PATH;
const E2E_CHROME_PATH = process.env.E2E_CHROME_PATH || SEC007_CHROME_PATH;
const chromiumLaunchOptions = E2E_CHROME_PATH
  ? { executablePath: E2E_CHROME_PATH }
  : undefined;

export default defineConfig({
  testMatch: '**/*.spec.ts',

  fullyParallel: true,
  forbidOnly: CI,
  retries: CI ? 2 : 0,
  workers: CI ? 2 : undefined,

  reporter: CI
    ? [
        ['github'],
        ['html', { open: 'never', outputFolder: 'playwright-report' }],
        ['junit', { outputFile: 'test-results/junit.xml' }],
      ]
    : [
        ['list'],
        ['html', { open: 'on-failure', outputFolder: 'playwright-report' }],
      ],

  timeout: 30000,
  expect: { timeout: 5000 },

  use: {
    trace: CI ? 'on-first-retry' : 'retain-on-failure',
    screenshot: 'only-on-failure',
    // A system-Chrome fallback does not imply a matching Playwright ffmpeg
    // bundle is installed. Video is evidence-only and disabled in that mode;
    // traces and screenshots remain available for failures.
    video: E2E_CHROME_PATH ? 'off' : CI ? 'on-first-retry' : 'retain-on-failure',
    extraHTTPHeaders: { Accept: 'application/json' },
    viewport: { width: 1280, height: 720 },
    actionTimeout: 10000,
    locale: 'en-US',
    timezoneId: 'UTC',
  },

  projects: [
    // Per-panel setup — populates the auth storage file for each panel's
    // base URL. Playwright picks the right one via project dependencies.
    {
      name: 'setup-user',
      testDir: './apps/user-frontend/e2e',
      testMatch: 'auth.setup.ts',
      use: { baseURL: USER_BASE_URL, launchOptions: chromiumLaunchOptions },
    },
    {
      name: 'setup-admin',
      testDir: './apps/admin-frontend/e2e',
      testMatch: 'auth.setup.ts',
      use: { baseURL: ADMIN_BASE_URL, launchOptions: chromiumLaunchOptions },
    },

    // User panel tests — user/contests/leaderboard/profile/tournaments
    {
      name: 'user-chromium',
      testDir: './apps/user-frontend/e2e',
      testMatch: [
        'auth.spec.ts',
        'leaderboard.spec.ts',
        'profile.spec.ts',
        'tournament-flows.spec.ts',
      ],
      dependencies: ['setup-user'],
      use: {
        ...devices['Desktop Chrome'],
        baseURL: USER_BASE_URL,
        storageState: './apps/user-frontend/e2e/.auth/user.json',
        launchOptions: chromiumLaunchOptions,
      },
    },

    // Trade tests — trading + websocket suites, user auth state
    {
      name: 'trade-chromium',
      testDir: './apps/user-frontend/e2e',
      testMatch: ['trading.spec.ts', 'websocket.spec.ts'],
      dependencies: ['setup-user'],
      use: {
        ...devices['Desktop Chrome'],
        baseURL: USER_BASE_URL,
        storageState: './apps/user-frontend/e2e/.auth/user.json',
        launchOptions: chromiumLaunchOptions,
      },
    },

    // Admin tests — audit, contests, shards
    {
      name: 'admin-chromium',
      testDir: './apps/admin-frontend/e2e',
      testMatch: ['audit.spec.ts', 'contests.spec.ts', 'shards.spec.ts'],
      dependencies: ['setup-admin'],
      use: {
        ...devices['Desktop Chrome'],
        baseURL: ADMIN_BASE_URL,
        storageState: './apps/admin-frontend/e2e/.auth/admin.json',
        launchOptions: chromiumLaunchOptions,
      },
    },
    {
      name: 'sec007-admin-mfa',
      testDir: './apps/admin-frontend/e2e',
      testMatch: 'admin_mfa.spec.ts',
      use: {
        ...devices['Desktop Chrome'],
        baseURL: ADMIN_BASE_URL,
        launchOptions: chromiumLaunchOptions,
        video: 'off',
      },
    },
  ],

  webServer: CI
    ? undefined
    : [
        {
          command: 'pnpm --filter @tragge/user-frontend dev',
          url: USER_BASE_URL,
          reuseExistingServer: true,
          timeout: 120000,
        },
        {
          command: 'pnpm --filter @tragge/admin-frontend dev',
          url: ADMIN_BASE_URL,
          reuseExistingServer: true,
          timeout: 120000,
        },
      ],

  outputDir: 'test-results',

  globalSetup: './e2e/global-setup.ts',
  globalTeardown: './e2e/global-teardown.ts',
});
