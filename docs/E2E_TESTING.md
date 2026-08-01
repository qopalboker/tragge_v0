# E2E Testing Guide

This document describes how to run and write E2E (End-to-End) tests for the Tragge Trading Platform using Playwright.

## Overview

The E2E test suite covers two Vue.js frontend panels, each with its
own origin and test directory:

| Panel | Description | Test Location |
|-------|-------------|---------------|
| **user-frontend** | User dashboard, contests, leaderboard, profile | `apps/user-frontend/e2e/` |
| **user-frontend** | Trading interface with WebSocket | `apps/user-frontend/e2e/` |
| **admin-frontend** | Contest management, audit logs, shards | `apps/admin-frontend/e2e/` |

## Prerequisites

- Node.js 18+
- pnpm 8+
- Frontend development servers running (or use auto-start)

## Installation

### Install Dependencies

```bash
# Install all dependencies including Playwright
pnpm install

# Install Playwright browsers
make e2e-install
# or
pnpm e2e:install
```

### Verify Installation

```bash
npx playwright --version
```

## Running Tests

### Quick Start

```bash
# Run all E2E tests
make e2e

# Run tests for a specific frontend
make e2e-user    # User frontend tests
make e2e-trade   # Trade frontend tests
make e2e-admin   # Admin frontend tests
```

### Interactive Mode

```bash
# Open Playwright UI for interactive test selection and debugging
make e2e-ui
```

### Debug Mode

```bash
# Run tests with step-by-step debugging
make e2e-debug
```

### Headed Mode

```bash
# Run tests with visible browser windows
make e2e-headed
```

### View Test Report

After running tests, view the HTML report:

```bash
make e2e-report
```

## Test Structure

```
tragge/
├── playwright.config.ts          # Main Playwright configuration
├── e2e/                          # Shared test utilities
│   ├── fixtures.ts               # Test fixtures and helpers
│   ├── test-data.ts              # Test data factory
│   ├── global-setup.ts           # Global setup (runs once)
│   └── global-teardown.ts        # Global teardown (runs once)
├── apps/
│   ├── frontend/e2e/
│   │   ├── pages/                # Page Objects
│   │   │   ├── LoginPage.ts
│   │   │   ├── DashboardPage.ts
│   │   │   ├── ContestsPage.ts
│   │   │   ├── LeaderboardPage.ts
│   │   │   ├── ProfilePage.ts
│   │   │   └── index.ts
│   │   ├── .auth/                # Stored auth state
│   │   ├── auth.setup.ts         # Authentication setup
│   │   ├── auth.spec.ts          # Authentication tests
│   │   ├── contests.spec.ts      # Contests tests
│   │   ├── leaderboard.spec.ts   # Leaderboard tests
│   │   └── profile.spec.ts       # Profile tests
│   ├── frontend/e2e/
│   │   ├── pages/
│   │   │   ├── LoginPage.ts
│   │   │   ├── TradingPage.ts
│   │   │   └── index.ts
│   │   ├── .auth/
│   │   ├── auth.setup.ts
│   │   ├── trading.spec.ts       # Trading tests
│   │   └── websocket.spec.ts     # WebSocket tests
│   └── frontend/e2e/
│       ├── pages/
│       │   ├── LoginPage.ts
│       │   ├── ContestsPage.ts
│       │   ├── ContestFormPage.ts
│       │   ├── AuditPage.ts
│       │   └── index.ts
│       ├── .auth/
│       ├── auth.setup.ts
│       ├── contests.spec.ts      # Contest management tests
│       └── audit.spec.ts         # Audit logs tests
```

## Configuration

### Environment Variables

Create a `.env.test` file or set environment variables:

```bash
# Frontend URL (single origin — the SPA serves /user, /trade, /admin
# modules from the same Vite server). Maps to BASE_URL in playwright.config.ts.
E2E_URL=http://localhost:5173

# API URLs (optional, for backend mocking)
E2E_API_URL=http://localhost:8080
```

### Playwright Projects

The configuration includes multiple projects for cross-browser testing:

| Project | Browser | Description |
|---------|---------|-------------|
| `user-chromium` | Chrome | User frontend on Chrome |
| `user-firefox` | Firefox | User frontend on Firefox |
| `user-webkit` | Safari | User frontend on Safari |
| `user-mobile` | iPhone | User frontend on mobile |
| `trade-chromium` | Chrome | Trade frontend on Chrome |
| `trade-firefox` | Firefox | Trade frontend on Firefox |
| `admin-chromium` | Chrome | Admin frontend on Chrome |
| `admin-firefox` | Firefox | Admin frontend on Firefox |

### CI Configuration

For CI environments, set:

```bash
CI=true
```

This enables:
- Headless browser mode
- Retry on failure (2 retries)
- Video/trace recording on failure
- JUnit XML report generation

## Writing Tests

### Using Page Objects

Page Objects encapsulate UI interactions:

```typescript
import { test, expect } from '@playwright/test';
import { LoginPage, DashboardPage } from './pages';

test('should login and navigate to dashboard', async ({ page }) => {
  const loginPage = new LoginPage(page);
  await loginPage.goto();
  await loginPage.login('user@example.com', 'password123');

  const dashboardPage = new DashboardPage(page);
  await expect(dashboardPage.welcomeMessage).toBeVisible();
});
```

### Using Test Fixtures

Fixtures provide reusable test data and helpers:

```typescript
import { test, expect } from '../../../e2e/fixtures';

test('should display user contests', async ({ page, testUser, testContest }) => {
  // testUser and testContest are automatically available
  await page.goto('/user/contests');
  await expect(page.locator('.contest-card')).toBeVisible();
});
```

### Mocking API Responses

Use the `mockApiResponse` helper:

```typescript
import { mockApiResponse } from '../../../e2e/fixtures';

test('should handle API error', async ({ page }) => {
  await mockApiResponse(page, '**/api/contests', {
    status: 500,
    body: { error: 'Server error' }
  });

  await page.goto('/user/contests');
  await expect(page.locator('.error-message')).toBeVisible();
});
```

### Testing WebSocket

Use the `WebSocketHelper`:

```typescript
import { WebSocketHelper } from '../../../e2e/fixtures';

test('should receive price updates', async ({ page }) => {
  const wsHelper = new WebSocketHelper(page);
  await wsHelper.setup(/ws\/trade/);

  await page.goto('/trade/contest-123');

  // Wait for tick data
  const tickMessage = await wsHelper.waitForMessage('tick_snapshot');
  expect(tickMessage).toBeTruthy();
});
```

### Test Data Factory

Use the test data factory for dynamic test data:

```typescript
import { generateTestEmail, generateContestName } from '../../../e2e/test-data';

test('should create new contest', async ({ page }) => {
  const contestName = generateContestName();
  // Creates unique name like "Grand Trading Cup 1704067200000"
});
```

## Test Scenarios

### User Frontend

| File | Scenarios |
|------|-----------|
| `auth.spec.ts` | Login, logout, session persistence, password reset, language toggle |
| `contests.spec.ts` | View contests, filter by status, view details, join contest |
| `leaderboard.spec.ts` | View rankings, pagination, own rank highlight, real-time updates |
| `profile.spec.ts` | View profile, update info, change language, statistics display |

### Trade Frontend

| File | Scenarios |
|------|-----------|
| `trading.spec.ts` | Load chart, switch symbols, place market/limit orders, manage positions |
| `websocket.spec.ts` | Connection status, price updates, position updates, reconnection |

### Admin Frontend

| File | Scenarios |
|------|-----------|
| `contests.spec.ts` | Create, edit, start/stop, view participants, delete contests |
| `audit.spec.ts` | View logs, filter by date/action, export logs, pagination |

## Best Practices

### 1. Use Page Objects

Encapsulate page-specific logic in Page Objects:

```typescript
// pages/LoginPage.ts
export class LoginPage {
  constructor(private page: Page) {}

  async login(email: string, password: string) {
    await this.page.fill('#email', email);
    await this.page.fill('#password', password);
    await this.page.click('button[type="submit"]');
  }
}
```

### 2. Use Test Fixtures for Auth

Use setup projects and stored auth state:

```typescript
// auth.setup.ts
setup('authenticate', async ({ page }) => {
  await page.goto('/login');
  await page.fill('#email', 'test@example.com');
  await page.fill('#password', 'password');
  await page.click('button[type="submit"]');
  await page.context().storageState({ path: '.auth/user.json' });
});
```

### 3. Mock External Dependencies

Always mock API responses for deterministic tests:

```typescript
test.beforeEach(async ({ page }) => {
  await mockApiResponse(page, '**/api/contests', {
    status: 200,
    body: { contests: TEST_CONTESTS }
  });
});
```

### 4. Use Meaningful Test Names

```typescript
test('should display error message when login fails with invalid credentials', async ({ page }) => {
  // ...
});
```

### 5. Keep Tests Independent

Each test should be able to run independently:

```typescript
test.beforeEach(async ({ page }) => {
  // Reset state before each test
});
```

## Debugging

### Visual Debugging

```bash
# Open test in debug mode
make e2e-debug

# Or use UI mode
make e2e-ui
```

### Traces

View traces for failed tests:

```bash
# After test failure, traces are saved to test-results/
npx playwright show-trace test-results/<test-name>/trace.zip
```

### Screenshots

Screenshots are automatically captured on failure. Find them in:

```
test-results/<test-name>/
├── test-failed-1.png
└── trace.zip
```

### Video Recording

Videos are recorded on failure in CI. Find them in:

```
test-results/<test-name>/video.webm
```

## CI/CD Integration

### GitHub Actions

Example workflow:

```yaml
name: E2E Tests

on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v2
        with:
          version: 8
      - uses: actions/setup-node@v4
        with:
          node-version: 18
          cache: 'pnpm'

      - name: Install dependencies
        run: pnpm install

      - name: Install Playwright
        run: pnpm e2e:install

      - name: Build frontends
        run: pnpm build

      - name: Run E2E tests
        run: pnpm e2e
        env:
          CI: true

      - uses: actions/upload-artifact@v3
        if: always()
        with:
          name: playwright-report
          path: playwright-report/
          retention-days: 30
```

### Test Results

CI generates:
- HTML report in `playwright-report/`
- JUnit XML in `test-results/junit.xml`
- Screenshots/videos in `test-results/`

## Troubleshooting

### Tests fail to find elements

1. Check if the selector is correct
2. Add appropriate waits:
   ```typescript
   await expect(locator).toBeVisible({ timeout: 10000 });
   ```

### Authentication issues

1. Ensure auth setup runs before tests
2. Check that `.auth/` directory has stored state
3. Verify credentials in test-data.ts

### WebSocket tests fail

1. Ensure backend WebSocket server is running
2. Use appropriate timeouts for connection
3. Consider mocking WebSocket messages

### Flaky tests

1. Add explicit waits instead of `waitForTimeout`
2. Use `expect().toBeVisible()` instead of checking immediately
3. Mock external dependencies
4. Increase timeout for slow operations

## Resources

- [Playwright Documentation](https://playwright.dev/docs/intro)
- [Page Object Model](https://playwright.dev/docs/pom)
- [Test Fixtures](https://playwright.dev/docs/test-fixtures)
- [Network Mocking](https://playwright.dev/docs/mock)
- [Visual Comparisons](https://playwright.dev/docs/test-snapshots)
