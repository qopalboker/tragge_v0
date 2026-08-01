import { FullConfig } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

/**
 * Auth state directories that are created during test setup.
 * These need to be cleaned before each run to avoid stale token issues.
 */
const AUTH_STATE_DIRS = [
  'apps/user-frontend/e2e/.auth',
  'apps/admin-frontend/e2e/.auth',
];

/**
 * Frontend dev server URLs to health-check before running tests.
 */
const FRONTEND_URLS: Record<string, string> = {
  'user-frontend': process.env.E2E_USER_URL || 'http://localhost:5173',
  'admin-frontend': process.env.E2E_ADMIN_URL || 'http://localhost:5174',
};

/**
 * Remove stale auth state JSON files from previous test runs.
 */
function cleanAuthState(rootDir: string): number {
  let removed = 0;
  for (const relDir of AUTH_STATE_DIRS) {
    const dir = path.join(rootDir, relDir);
    if (!fs.existsSync(dir)) continue;

    const files = fs.readdirSync(dir).filter((f) => f.endsWith('.json'));
    for (const file of files) {
      fs.unlinkSync(path.join(dir, file));
      removed++;
    }
  }
  return removed;
}

/**
 * Health-check frontend URLs. Logs status but does not fail —
 * tests can still run with API mocking even if backends are unreachable.
 */
async function healthCheckFrontends(): Promise<void> {
  for (const [name, url] of Object.entries(FRONTEND_URLS)) {
    try {
      const controller = new AbortController();
      const timeout = setTimeout(() => controller.abort(), 5000);
      const res = await fetch(url, { signal: controller.signal });
      clearTimeout(timeout);
      console.log(`  ✓ ${name} (${url}) — ${res.status}`);
    } catch {
      console.log(`  ✗ ${name} (${url}) — unreachable (tests will use mocks)`);
    }
  }
}

/**
 * Global setup for Playwright tests
 * Runs once before all tests
 */
async function globalSetup(config: FullConfig): Promise<void> {
  const rootDir = config.rootDir;

  console.log('\n--- Playwright E2E Test Suite ---');
  console.log(`Running ${config.projects.length} project(s)`);
  console.log(`Workers: ${config.workers}`);
  console.log(`Retries: ${config.projects[0]?.retries ?? 0}`);
  console.log('-----------------------------------');

  // Set up test environment
  process.env.TEST_ENV = 'e2e';

  // Log test mode
  if (process.env.E2E_INTEGRATION) {
    console.log('\nMode: INTEGRATION (tests will call real backend APIs)');
  } else {
    console.log('\nMode: MOCK (default — API responses are intercepted by Playwright)');
  }

  // Clean stale auth state from previous runs
  const removed = cleanAuthState(rootDir);
  if (removed > 0) {
    console.log(`Cleaned ${removed} stale auth state file(s)`);
  }

  // Health-check frontend dev servers
  console.log('\nFrontend health checks:');
  await healthCheckFrontends();

  console.log('');
}

export default globalSetup;
