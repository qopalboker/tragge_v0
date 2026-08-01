import { ref, onUnmounted, type Ref } from 'vue';
import { decode as msgpackDecode } from '@msgpack/msgpack';
import { wsLogger } from '@/utils/logger';
import { buildWebSocketURL } from './webSocketUrl.js';

export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'error' | 'reconnecting';

/** Encoding type for WebSocket messages */
export type WebSocketEncoding = 'json' | 'msgpack';

export interface WebSocketMessage {
  type: string;
  data: unknown;
  timestamp: string;
  /** True if the message was decoded from MessagePack binary format */
  isBinary?: boolean;
}

export interface UseWebSocketOptions {

  /**
   * Async function to acquire a short-lived, single-use WebSocket ticket.
   * Called before each WebSocket connection attempt. The returned ticket is
   * passed via ?ticket= query parameter instead of the raw JWT.
   * If provided, takes precedence over the deprecated `token` option.
   */
  acquireTicket?: () => Promise<string | null>;
  /** Maximum number of reconnection attempts (default: 10) */
  maxReconnectAttempts?: number;
  /** Base delay in milliseconds for reconnection (default: 1000) */
  baseReconnectDelay?: number;
  /** Maximum delay in milliseconds for reconnection (default: 30000) */
  maxReconnectDelay?: number;
  /** Callback when server sends close with specific code */
  onServerClose?: (code: number, reason: string) => void;
  /**
   * Preferred encoding for tick_batch and state_delta messages.
   * 'msgpack' uses binary MessagePack encoding for ~40-50% bandwidth savings.
   * 'json' uses text JSON encoding (default, for debugging compatibility).
   * Default: 'msgpack'
   */
  encoding?: WebSocketEncoding;
}

export interface UseWebSocketReturn {
  status: Ref<ConnectionStatus>;
  lastMessage: Ref<WebSocketMessage | null>;
  lastMessageRaw: Ref<string | null>;
  reconnectAttempts: Ref<number>;
  connect: () => Promise<void>;
  disconnect: () => void;
  send: (data: unknown) => void;
  /** Reset reconnection state and attempt fresh connection */
  resetAndReconnect: () => void;
  /** Update the URL for future connections (call connect() after to use new URL) */
  setUrl: (newUrl: string | (() => string)) => void;
}

/** URL type can be a string or a getter function for dynamic URLs */
export type WebSocketUrl = string | (() => string);


// WebSocket close codes
const CLOSE_NORMAL = 1000;
const CLOSE_GOING_AWAY = 1001;
const CLOSE_CONFLICT = 4409; // Custom code for connection conflict

export function useWebSocket(initialUrl: WebSocketUrl, options: UseWebSocketOptions = {}): UseWebSocketReturn {
  const {
    acquireTicket,
    maxReconnectAttempts = 10,
    baseReconnectDelay = 1000,
    maxReconnectDelay = 30000,
    onServerClose,
    encoding = 'msgpack', // Default to MessagePack for bandwidth savings
  } = options;

  const status = ref<ConnectionStatus>('disconnected');
  const lastMessage = ref<WebSocketMessage | null>(null);
  const lastMessageRaw = ref<string | null>(null);
  const reconnectAttempts = ref(0);

  let ws: WebSocket | null = null;
  let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  let intentionalDisconnect = false;

  // Store URL as getter function to support dynamic URLs
  let urlGetter: () => string = typeof initialUrl === 'function' ? initialUrl : () => initialUrl;


  /**
   * Calculate exponential backoff delay with jitter
   * Formula: min(maxDelay, baseDelay * 2^attempts + random jitter)
   */
  function calculateBackoffDelay(): number {
    const exponentialDelay = baseReconnectDelay * Math.pow(2, reconnectAttempts.value);
    const jitter = Math.random() * 1000; // Add 0-1000ms random jitter
    return Math.min(maxReconnectDelay, exponentialDelay + jitter);
  }

  async function connect(): Promise<void> {
    if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) {
      return;
    }

    intentionalDisconnect = false;
    status.value = reconnectAttempts.value > 0 ? 'reconnecting' : 'connecting';

    // Get current URL (supports dynamic URLs via getter function)
    const url = urlGetter();

    // Determine WebSocket URL based on current location
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = window.location.host;
    let wsUrl = url.startsWith('ws') ? url : `${protocol}//${host}${url}`;

    // Browser-authenticated sockets obtain a fresh 10-second, single-use ticket
    // for every initial connection and reconnect. A failed exchange stops the
    // handshake; reusable session JWTs are never placed in the URL as fallback.
    let ticket: string | null = null;
    if (acquireTicket) {
      try {
        ticket = await acquireTicket();
      } catch {
        wsLogger.error('Failed to acquire WebSocket ticket');
      }
      if (!ticket) {
        wsLogger.warn('WebSocket ticket unavailable; connection rejected locally');
        status.value = 'error';
        if (!intentionalDisconnect) attemptReconnect();
        return;
      }
    }

    try {
      wsUrl = buildWebSocketURL(wsUrl, ticket, encoding);
    } catch {
      wsLogger.error('Unsafe WebSocket URL rejected');
      status.value = 'error';
      return;
    }
    try {
      ws = new WebSocket(wsUrl);
      // Enable binary message handling for MessagePack
      ws.binaryType = 'arraybuffer';

      ws.onopen = (): void => {
        status.value = 'connected';
        reconnectAttempts.value = 0; // Reset on successful connection
        wsLogger.info(`Connected successfully (encoding: ${encoding})`);
      };

      ws.onmessage = (event: MessageEvent): void => {
        // Detect binary messages (MessagePack encoded tick_batch/state_delta)
        if (event.data instanceof ArrayBuffer) {
          try {
            const uint8Array = new Uint8Array(event.data);
            const decoded = msgpackDecode(uint8Array) as Record<string, unknown>;
            lastMessageRaw.value = `[binary:${event.data.byteLength}bytes]`;
            lastMessage.value = {
              type: (decoded.type as string) || 'unknown',
              data: decoded,
              timestamp: new Date().toISOString(),
              isBinary: true,
            };
          } catch (err) {
            wsLogger.error('Failed to decode MessagePack message:', err);
            lastMessage.value = {
              type: 'error',
              data: { error: 'Failed to decode binary message', bytes: event.data.byteLength },
              timestamp: new Date().toISOString(),
              isBinary: true,
            };
          }
          return;
        }

        // Handle text messages (JSON encoded)
        lastMessageRaw.value = event.data;
        try {
          const parsed = JSON.parse(event.data);
          lastMessage.value = {
            type: parsed.type || 'unknown',
            data: parsed,
            timestamp: new Date().toISOString(),
            isBinary: false,
          };
        } catch {
          lastMessage.value = {
            type: 'raw',
            data: event.data,
            timestamp: new Date().toISOString(),
            isBinary: false,
          };
        }
      };

      ws.onclose = (event: CloseEvent): void => {
        const { code, reason, wasClean } = event;
        wsLogger.info(`Closed: code=${code}, reason=${reason}, clean=${wasClean}`);

        // Notify callback if provided
        if (onServerClose) {
          onServerClose(code, reason);
        }

        // Handle specific close codes
        if (code === CLOSE_NORMAL || code === CLOSE_GOING_AWAY) {
          // Normal closure - check if server is shutting down
          if (reason === 'Server shutting down') {
            // Server gracefully shutting down, reconnect with backoff
            status.value = 'reconnecting';
            attemptReconnect();
            return;
          }
          status.value = 'disconnected';
          return;
        }

        if (code === CLOSE_CONFLICT) {
          // Connection conflict - another session exists
          wsLogger.warn('Connection conflict detected, session exists elsewhere');
          status.value = 'error';
          // Don't automatically reconnect on conflict
          return;
        }

        // Abnormal closure or other error - attempt reconnect
        status.value = 'disconnected';
        if (!intentionalDisconnect) {
          attemptReconnect();
        }
      };

      ws.onerror = (event: Event): void => {
        wsLogger.error('Error:', event);
        status.value = 'error';
      };
    } catch (error) {
      wsLogger.error('Connection failed:', error);
      status.value = 'error';
      if (!intentionalDisconnect) {
        attemptReconnect();
      }
    }
  }

  function attemptReconnect(): void {
    if (intentionalDisconnect) {
      return;
    }

    if (reconnectAttempts.value >= maxReconnectAttempts) {
      wsLogger.warn('Max reconnect attempts reached');
      status.value = 'error';
      return;
    }

    const delay = calculateBackoffDelay();
    reconnectAttempts.value++;
    wsLogger.debug(`Reconnecting in ${Math.round(delay)}ms (attempt ${reconnectAttempts.value}/${maxReconnectAttempts})`);

    reconnectTimeout = setTimeout(() => {
      connect();
    }, delay);
  }

  function disconnect(): void {
    intentionalDisconnect = true;
    if (reconnectTimeout) {
      clearTimeout(reconnectTimeout);
      reconnectTimeout = null;
    }
    if (ws) {
      ws.close(CLOSE_NORMAL, 'Client disconnect');
      ws = null;
    }
    status.value = 'disconnected';
    reconnectAttempts.value = 0;
  }

  function resetAndReconnect(): void {
    disconnect();
    intentionalDisconnect = false;
    reconnectAttempts.value = 0;
    // Small delay before reconnecting
    setTimeout(() => {
      connect();
    }, 100);
  }

  function send(data: unknown): void {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(typeof data === 'string' ? data : JSON.stringify(data));
    } else {
      wsLogger.warn('Cannot send message - not connected');
    }
  }

  function setUrl(newUrl: WebSocketUrl): void {
    urlGetter = typeof newUrl === 'function' ? newUrl : () => newUrl;
  }

  onUnmounted(() => {
    disconnect();
  });

  return {
    status,
    lastMessage,
    lastMessageRaw,
    reconnectAttempts,
    connect,
    disconnect,
    send,
    resetAndReconnect,
    setUrl,
  };
}
