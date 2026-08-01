import { test as base, expect, Page, BrowserContext } from '@playwright/test';

/**
 * Test user credentials
 */
export interface TestUser {
  email: string;
  password: string;
  name?: string;
  role?: 'user' | 'admin';
}

/**
 * Test contest data
 */
export interface TestContest {
  id: string;
  name: string;
  status: 'draft' | 'scheduled' | 'registration_open' | 'running' | 'paused' | 'completed' | 'cancelled';
  startDate: string;
  endDate: string;
  prizePool: number;
  entryFee: number;
}

/**
 * Test order data
 */
export interface TestOrder {
  symbol: string;
  side: 'buy' | 'sell';
  type: 'market' | 'limit';
  quantity: number;
  price?: number;
  takeProfit?: number;
  stopLoss?: number;
}

/**
 * Extended test fixtures
 */
export type TestFixtures = {
  /** Test user (regular user) */
  testUser: TestUser;
  /** Admin user */
  adminUser: TestUser;
  /** Test contest */
  testContest: TestContest;
  /** Login as test user */
  loginAsUser: () => Promise<void>;
  /** Login as admin */
  loginAsAdmin: () => Promise<void>;
  /** API helper for making authenticated API calls */
  apiHelper: ApiHelper;
  /** WebSocket helper for testing real-time features */
  wsHelper: WebSocketHelper;
};

/**
 * API Helper class for making authenticated API calls
 */
export class ApiHelper {
  constructor(private page: Page) {}

  /**
   * Make an authenticated API request
   */
  async request(
    method: 'GET' | 'POST' | 'PUT' | 'DELETE',
    endpoint: string,
    data?: Record<string, unknown>
  ): Promise<Response> {
    const token = await this.page.evaluate(() => localStorage.getItem('token'));

    const response = await this.page.request[method.toLowerCase() as 'get' | 'post' | 'put' | 'delete'](endpoint, {
      data,
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
    });

    return response;
  }

  /**
   * Get request
   */
  async get(endpoint: string): Promise<Response> {
    return this.request('GET', endpoint);
  }

  /**
   * Post request
   */
  async post(endpoint: string, data: Record<string, unknown>): Promise<Response> {
    return this.request('POST', endpoint, data);
  }

  /**
   * Put request
   */
  async put(endpoint: string, data: Record<string, unknown>): Promise<Response> {
    return this.request('PUT', endpoint, data);
  }

  /**
   * Delete request
   */
  async delete(endpoint: string): Promise<Response> {
    return this.request('DELETE', endpoint);
  }
}

/**
 * WebSocket Helper class for testing real-time features
 */
export class WebSocketHelper {
  private messages: unknown[] = [];
  private wsUrl: string | null = null;

  constructor(private page: Page) {}

  /**
   * Set up WebSocket message interception
   */
  async setup(wsUrlPattern: string | RegExp): Promise<void> {
    this.messages = [];

    // Listen for WebSocket connections
    this.page.on('websocket', (ws) => {
      const url = ws.url();
      const pattern = typeof wsUrlPattern === 'string' ? new RegExp(wsUrlPattern) : wsUrlPattern;

      if (pattern.test(url)) {
        this.wsUrl = url;

        ws.on('framereceived', (event) => {
          try {
            const data = JSON.parse(event.payload.toString());
            this.messages.push(data);
          } catch {
            this.messages.push(event.payload);
          }
        });
      }
    });
  }

  /**
   * Get all received messages
   */
  getMessages(): unknown[] {
    return [...this.messages];
  }

  /**
   * Get messages of a specific type
   */
  getMessagesByType(type: string): unknown[] {
    return this.messages.filter((msg) => {
      if (typeof msg === 'object' && msg !== null && 'type' in msg) {
        return (msg as { type: string }).type === type;
      }
      return false;
    });
  }

  /**
   * Wait for a message of a specific type
   */
  async waitForMessage(type: string, timeout = 5000): Promise<unknown> {
    const startTime = Date.now();

    while (Date.now() - startTime < timeout) {
      const messages = this.getMessagesByType(type);
      if (messages.length > 0) {
        return messages[messages.length - 1];
      }
      await this.page.waitForTimeout(100);
    }

    throw new Error(`Timeout waiting for message of type: ${type}`);
  }

  /**
   * Clear received messages
   */
  clearMessages(): void {
    this.messages = [];
  }

  /**
   * Get WebSocket URL
   */
  getWsUrl(): string | null {
    return this.wsUrl;
  }
}

/**
 * Mock API response helper
 */
export async function mockApiResponse(
  page: Page,
  urlPattern: string | RegExp,
  response: {
    status?: number;
    body?: unknown;
    headers?: Record<string, string>;
  }
): Promise<void> {
  await page.route(urlPattern, (route) => {
    route.fulfill({
      status: response.status ?? 200,
      contentType: 'application/json',
      headers: response.headers,
      body: JSON.stringify(response.body ?? {}),
    });
  });
}

/**
 * Mock WebSocket messages helper
 */
export async function mockWebSocketMessage(
  page: Page,
  message: unknown
): Promise<void> {
  // This would require a more complex setup with a mock WebSocket server
  // For now, we'll use page.evaluate to simulate receiving messages
  await page.evaluate((msg) => {
    // Dispatch a custom event that tests can listen for
    window.dispatchEvent(
      new CustomEvent('mock-ws-message', { detail: msg })
    );
  }, message);
}

// Create extended test with custom fixtures
export const test = base.extend<TestFixtures>({
  testUser: {
    email: 'test@example.com',
    password: 'Test123!@#',
    name: 'Test User',
    role: 'user',
  },

  adminUser: {
    email: 'admin@example.com',
    password: 'Admin123!@#',
    name: 'Admin User',
    role: 'admin',
  },

  testContest: {
    id: 'test-contest-001',
    name: 'Test Trading Contest',
    status: 'running',
    startDate: new Date().toISOString(),
    endDate: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
    prizePool: 10000,
    entryFee: 100,
  },

  loginAsUser: async ({ page, testUser }, use) => {
    const login = async () => {
      await page.goto('/user/login');
      await page.fill('input[type="email"]', testUser.email);
      await page.fill('input[type="password"]', testUser.password);
      await page.click('button[type="submit"]');
      await page.waitForURL(/\/user\/(dashboard|$)/);
    };
    await use(login);
  },

  loginAsAdmin: async ({ page, adminUser }, use) => {
    const login = async () => {
      await page.goto('/admin/login');
      await page.fill('input[type="email"]', adminUser.email);
      await page.fill('input[type="password"]', adminUser.password);
      await page.click('button[type="submit"]');
      await page.waitForURL(/\/admin\/(contests|$)/);
    };
    await use(login);
  },

  apiHelper: async ({ page }, use) => {
    const helper = new ApiHelper(page);
    await use(helper);
  },

  wsHelper: async ({ page }, use) => {
    const helper = new WebSocketHelper(page);
    await use(helper);
  },
});

export { expect };
