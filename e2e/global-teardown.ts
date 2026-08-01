import { FullConfig } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

/**
 * Auth state directories created during the test run.
 */
const AUTH_STATE_DIRS = [
  'apps/user-frontend/e2e/.auth',
  'apps/admin-frontend/e2e/.auth',
];

/**
 * Remove auth state files generated during the test run.
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
 * Global teardown for Playwright tests
 * Runs once after all tests
 */
async function globalTeardown(config: FullConfig): Promise<void> {
  console.log('\n--- E2E Test Suite Completed ---');
  console.log(`Projects: ${config.projects.length}`);

  // Clean up auth state files
  const removed = cleanAuthState(config.rootDir);
  if (removed > 0) {
    console.log(`Cleaned up ${removed} auth state file(s)`);
  }

  console.log('--------------------------------\n');
}

export default globalTeardown;
