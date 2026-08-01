import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/utils/logger', () => ({
  wsLogger: {
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  },
}));

import { useWebSocket } from './useWebSocket';

class MockWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readonly url: string;
  readyState = MockWebSocket.CONNECTING;
  binaryType: BinaryType = 'blob';
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;

  constructor(url: string | URL) {
    this.url = String(url);
    MockWebSocket.instances.push(this);
  }

  send(): void {}

  close(): void {
    this.readyState = MockWebSocket.CLOSED;
  }
}

function expectNoSessionCredential(url: string): void {
  const parsed = new URL(url);
  for (const name of ['token', 'access_token', 'jwt', 'auth_token', 'session_token']) {
    expect(parsed.searchParams.has(name)).toBe(false);
  }
}

describe('useWebSocket URL authentication', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal('window', { location: { protocol: 'https:', host: 'app.example.invalid' } });
    vi.stubGlobal('WebSocket', MockWebSocket);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('uses only a bounded ticket and non-sensitive parameters', async () => {
    const ticket = 'ticket-fixture-not-a-session-jwt';
    const socket = useWebSocket('/ws/trade?contest_id=contest-1', {
      acquireTicket: async () => ticket,
      encoding: 'msgpack',
      maxReconnectAttempts: 0,
    });

    await socket.connect();

    expect(MockWebSocket.instances).toHaveLength(1);
    const url = MockWebSocket.instances[0].url;
    const parsed = new URL(url);
    expect(parsed.searchParams.get('ticket')).toBe(ticket);
    expect(parsed.searchParams.get('contest_id')).toBe('contest-1');
    expect(parsed.searchParams.get('encoding')).toBe('msgpack');
    expectNoSessionCredential(url);
  });

  it('fails locally when ticket acquisition fails instead of falling back to a JWT URL', async () => {
    const socket = useWebSocket('/ws/trade?contest_id=contest-1', {
      acquireTicket: async () => null,
      maxReconnectAttempts: 0,
    });

    await socket.connect();

    expect(socket.status.value).toBe('error');
    expect(MockWebSocket.instances).toHaveLength(0);
  });

  it('acquires a fresh ticket on reconnect without introducing session credentials', async () => {
    vi.useFakeTimers();
    let ticketNumber = 0;
    const socket = useWebSocket('/ws/trade?contest_id=contest-1', {
      acquireTicket: async () => `bounded-ticket-${++ticketNumber}`,
      encoding: 'json',
      baseReconnectDelay: 1,
      maxReconnectDelay: 1,
      maxReconnectAttempts: 1,
    });

    await socket.connect();
    const first = MockWebSocket.instances[0];
    first.readyState = MockWebSocket.CLOSED;
    first.onclose?.({ code: 1006, reason: '', wasClean: false } as CloseEvent);
    await vi.runAllTimersAsync();

    expect(MockWebSocket.instances).toHaveLength(2);
    expect(new URL(MockWebSocket.instances[0].url).searchParams.get('ticket')).toBe('bounded-ticket-1');
    expect(new URL(MockWebSocket.instances[1].url).searchParams.get('ticket')).toBe('bounded-ticket-2');
    MockWebSocket.instances.forEach((instance) => expectNoSessionCredential(instance.url));
  });
});