import { ref, computed, onUnmounted, watch, type Ref, type ComputedRef, type WatchStopHandle } from 'vue';
import { useWebSocket, type UseWebSocketOptions, type ConnectionStatus, type WebSocketMessage } from './useWebSocket';
import { useOrderQueue, type QueuedOrder, type UseOrderQueueOptions } from './useOrderQueue';
import { tradingLogger } from '@/utils/logger';
import { useNotificationStore } from '@/stores/notifications';
import { useToast } from '../composables/useToast';
import { useNotificationRenderer } from '@/composables/useNotificationRenderer';

// Re-export types from useOrderQueue for convenience
export type { QueuedOrder, QueuedOrderStatus } from './useOrderQueue';

// ============================================
// Types for Batched Messages
// ============================================

/** Symbol tick data from the server */
export interface SymbolTick {
  symbol: string;
  bid: number;
  ask: number;
  last: number;
  timestamp?: number; // Unix milliseconds from server; may be absent for legacy messages
}

/** Batched message wrapper with sequence tracking */
export interface BatchedMessage<T> {
  type: string;
  seq: number;   // Sequence number for gap detection
  n: number;     // Number of items in batch
  data: T;       // The batched data
  ts: number;    // Timestamp of batch creation
}

/** Tick batch data structure */
export interface TickBatchData {
  symbols: SymbolTick[];
}

/** Position snapshot for state delta */
export interface PositionSnapshot {
  id: string;
  symbol: string;
  side: 'long' | 'short';
  unrealizedPnL: number;
  currentPrice: number;
  qtyOpen: number;
  avgPrice: number;
}

/** Balance snapshot */
export interface Balance {
  available: number;
  total: number;
  equity: number;
}

// ============================================
// Types for Order Handling
// ============================================

/** Order side */
export type OrderSide = 'BUY' | 'SELL';

/** Order type */
export type OrderType = 'MARKET' | 'BUY_LIMIT' | 'SELL_LIMIT' | 'BUY_STOP' | 'SELL_STOP';

/** Order request to be sent via WebSocket */
export interface OrderRequest {
  type: 'order_request';
  request_id: string;
  symbol: string;
  side: OrderSide;
  order_type: OrderType;
  qty: number;
  limit_price?: number;
  stop_price?: number;
  take_profit?: number;
  stop_loss?: number;
}

/** Order acknowledgment from server */
export interface OrderAck {
  type: 'order_ack';
  request_id: string;
  order_id: string;
  status: 'accepted';
}

/** Rate limit metadata from server */
export interface RateLimitInfo {
  scope: 'user' | 'contest' | 'global';
  limit: number;
  window: string;
  retry_after_ms: number;
}

/** Order rejection from server */
export interface OrderReject {
  type: 'order_reject';
  request_id: string;
  order_id?: string; // Present when rejection comes from Kafka consumer
  code: string;
  message: string;
  rate_limit?: RateLimitInfo; // Present only for RATE_LIMITED rejections
}

/** Order cancelled event from server */
export interface OrderCancelled {
  type: 'order_cancelled';
  order_id: string;
}

/** Pending order request with callbacks */
interface PendingOrder {
  request: OrderRequest;
  resolve: (orderAck: OrderAck) => void;
  reject: (error: OrderReject | Error) => void;
  timeout: ReturnType<typeof setTimeout>;
}

/** Position delta representing changes */
export interface PositionDelta {
  id: string;
  c: Record<string, unknown>; // Changes object
}

/** State delta message for position updates */
export interface StateDelta {
  type: 'state_delta';
  ts: number;
  p?: PositionDelta[];          // Position changes
  b?: Partial<Balance>;         // Balance changes
  full?: boolean;               // Full sync flag
}

// ============================================
// Configuration
// ============================================

/** Default buffer interval for UI updates (ms) - debounces rapid updates */
const DEFAULT_TICK_BUFFER_INTERVAL = 100;

/** Maximum sequence gap before requesting resync */
const MAX_SEQUENCE_GAP = 5;

/** Order request timeout (ms) */
const ORDER_TIMEOUT = 10000;

// ============================================
// Trading WebSocket Composable
// ============================================

export interface UseTradingWebSocketOptions extends UseWebSocketOptions {
  /** Contest ID for trading session */
  contestId: string;
  /** Callback when a sequence gap is detected */
  onSequenceGap?: (expected: number, received: number) => void;
  /** Callback when positions are updated */
  onPositionsUpdate?: (positions: Map<string, PositionSnapshot>) => void;
  /** Callback when prices are updated */
  onPricesUpdate?: (prices: Map<string, SymbolTick>) => void;
  /** Callback when an order is accepted */
  onOrderAccepted?: (ack: OrderAck) => void;
  /** Callback when an order is rejected */
  onOrderRejected?: (reject: OrderReject) => void;
  /** Callback when an order is cancelled */
  onOrderCancelled?: (cancelled: OrderCancelled) => void;
  /** Callback when an order is rate limited - returns retry_after_ms */
  onRateLimited?: (retryAfterMs: number, rateLimit: RateLimitInfo) => void;
  /** Callback when a position_update message is received (full position list) */
  onPositionUpdate?: (data: unknown) => void;
  /** Callback when a fill_event message is received */
  onFillEvent?: (data: unknown) => void;
  /** Callback when a pnl_delta message is received */
  onPnLDelta?: (data: unknown) => void;
  /** Callback when a balance_update message is received */
  onBalanceUpdate?: (data: unknown) => void;
  /** Enable offline order queue (default: true) */
  enableOrderQueue?: boolean;
  /** Order queue options */
  orderQueueOptions?: UseOrderQueueOptions;
  /** Active contest IDs for queue filtering on reconnect */
  activeContestIds?: string[];
  /** Tick buffer interval in ms — lower for desktops, higher for weak devices (default: 100) */
  tickBufferInterval?: number;
}

/** Options for placing an order */
export interface PlaceOrderOptions {
  symbol: string;
  side: OrderSide;
  orderType: OrderType;
  qty: number;
  limitPrice?: number;
  stopPrice?: number;
  takeProfit?: number;
  stopLoss?: number;
}

/** Rate limit state for UI display */
export interface RateLimitState {
  /** Whether orders are currently rate limited */
  isLimited: Ref<boolean>;
  /** Milliseconds until rate limit expires */
  retryAfterMs: Ref<number>;
  /** Last rate limit info received */
  lastRateLimitInfo: Ref<RateLimitInfo | null>;
}

export interface UseTradingWebSocketReturn {
  // Connection state
  status: Ref<ConnectionStatus>;
  reconnectAttempts: Ref<number>;

  // Raw message forwarding (for provide/inject to child components)
  lastMessage: Ref<WebSocketMessage | null>;

  // Trading data
  prices: Ref<Map<string, SymbolTick>>;
  positions: Ref<Map<string, PositionSnapshot>>;
  balance: Ref<Balance | null>;

  // Sequence tracking
  lastSequence: Ref<number>;
  sequenceGaps: Ref<number>;

  // Rate limit state
  rateLimit: RateLimitState;

  // Order queue state
  orderQueue: Ref<QueuedOrder[]>;
  orderQueuePendingCount: ComputedRef<number>;
  isOrderQueueFull: ComputedRef<boolean>;
  orderQueuePendingConfirmation: Ref<QueuedOrder[]>;
  showOrderQueueConfirmation: Ref<boolean>;

  // Computed values
  pricesArray: ComputedRef<SymbolTick[]>;
  positionsArray: ComputedRef<PositionSnapshot[]>;

  // Actions
  connect: () => void;
  disconnect: () => void;
  send: (data: unknown) => void;
  resetAndReconnect: () => void;

  // Manual resync
  requestResync: () => void;

  // Contest switching
  updateContestId: (newContestId: string) => void;

  // Order placement
  placeOrder: (options: PlaceOrderOptions) => Promise<OrderAck | { status: 'queued'; queuedOrder: QueuedOrder }>;

  // Order queue actions
  removeQueuedOrder: (orderId: string) => void;
  clearOrderQueue: () => void;
  confirmQueuedOrders: () => void;
  cancelQueuedOrders: () => void;
}

export function useTradingWebSocket(
  baseUrl: string,
  options: UseTradingWebSocketOptions
): UseTradingWebSocketReturn {
  const {
    contestId,
    onSequenceGap,
    onPositionsUpdate,
    onPricesUpdate,
    onOrderAccepted,
    onOrderRejected,
    onOrderCancelled,
    onRateLimited,
    onPositionUpdate,
    onFillEvent,
    onPnLDelta,
    onBalanceUpdate,
    enableOrderQueue = true,
    orderQueueOptions,
    activeContestIds,
    tickBufferInterval = DEFAULT_TICK_BUFFER_INTERVAL,
    ...wsOptions
  } = options;

  // Initialize order queue
  const orderQueueComposable = useOrderQueue(orderQueueOptions);

  // Track queued order ID to pending order mapping
  const queuedOrderToPendingMap = new Map<string, string>();

  // Build URL with contest ID
  const url = `${baseUrl}?contest_id=${encodeURIComponent(contestId)}`;

  // Trading data state
  const prices = ref<Map<string, SymbolTick>>(new Map());
  const positions = ref<Map<string, PositionSnapshot>>(new Map());
  const balance = ref<Balance | null>(null);

  // Sequence tracking for gap detection
  const lastSequence = ref(0);
  const sequenceGaps = ref(0);

  // Buffering for smooth UI updates
  const tickBuffer = ref<Map<string, SymbolTick>>(new Map());
  let tickFlushTimer: ReturnType<typeof setTimeout> | null = null;

  // Pending order requests waiting for ack/reject
  const pendingOrders = new Map<string, PendingOrder>();

  // Rate limit state
  const isRateLimited = ref(false);
  const rateLimitRetryAfterMs = ref(0);
  const lastRateLimitInfo = ref<RateLimitInfo | null>(null);
  let rateLimitTimer: ReturnType<typeof setTimeout> | null = null;

  // Use the base WebSocket composable
  const {
    status,
    lastMessage,
    reconnectAttempts,
    connect: baseConnect,
    disconnect: baseDisconnect,
    send,
    resetAndReconnect: baseResetAndReconnect,
    setUrl,
  } = useWebSocket(url, wsOptions);

  // ============================================
  // Message Handlers
  // ============================================

  /**
   * Handle tick batch messages - batched price updates
   */
  function handleTickBatch(batch: BatchedMessage<TickBatchData>): void {
    // Check for sequence gaps
    if (lastSequence.value > 0 && batch.seq > lastSequence.value + 1) {
      const gap = batch.seq - lastSequence.value - 1;
      tradingLogger.warn(`Sequence gap detected: ${lastSequence.value} -> ${batch.seq} (${gap} missed)`);
      sequenceGaps.value += gap;

      if (onSequenceGap) {
        onSequenceGap(lastSequence.value + 1, batch.seq);
      }

      // If gap is too large, request resync
      if (gap > MAX_SEQUENCE_GAP) {
        tradingLogger.warn('Large sequence gap, requesting resync');
        requestResync();
      }
    }
    lastSequence.value = batch.seq;

    // Buffer ticks for debounced UI updates
    if (batch.data?.symbols) {
      for (const tick of batch.data.symbols) {
        tickBuffer.value.set(tick.symbol, tick);
      }
    }

    // Debounce UI updates to prevent excessive re-renders
    if (!tickFlushTimer) {
      tickFlushTimer = setTimeout(() => {
        flushTickBuffer();
        tickFlushTimer = null;
      }, tickBufferInterval);
    }
  }

  /**
   * Flush buffered ticks to the prices map
   */
  function flushTickBuffer(): void {
    if (tickBuffer.value.size === 0) return;

    // Apply all buffered ticks at once (single reactive update)
    const newPrices = new Map(prices.value);
    for (const [symbol, tick] of tickBuffer.value) {
      newPrices.set(symbol, tick);
    }
    prices.value = newPrices;
    tickBuffer.value.clear();

    if (onPricesUpdate) {
      onPricesUpdate(prices.value);
    }
  }

  /**
   * Handle state delta messages - position and balance updates
   */
  function handleStateDelta(delta: StateDelta): void {
    const newPositions = delta.full
      ? new Map<string, PositionSnapshot>()
      : new Map(positions.value);

    // Apply position changes
    if (delta.p) {
      for (const pd of delta.p) {
        // Check if position is closed
        if (pd.c.closed) {
          newPositions.delete(pd.id);
          continue;
        }

        const existing = newPositions.get(pd.id);

        if (pd.c.new || !existing) {
          // New position - create from changes
          newPositions.set(pd.id, {
            id: pd.id,
            symbol: pd.c.s as string,
            side: (pd.c.side as 'long' | 'short') || 'long',
            unrealizedPnL: pd.c.pnl as number || 0,
            currentPrice: pd.c.cp as number || 0,
            qtyOpen: pd.c.qty as number || 0,
            avgPrice: pd.c.avg as number || 0,
          });
        } else {
          // Update existing position with only changed fields
          const updated = { ...existing };
          if (pd.c.pnl !== undefined) updated.unrealizedPnL = pd.c.pnl as number;
          if (pd.c.cp !== undefined) updated.currentPrice = pd.c.cp as number;
          if (pd.c.qty !== undefined) updated.qtyOpen = pd.c.qty as number;
          if (pd.c.avg !== undefined) updated.avgPrice = pd.c.avg as number;
          if (pd.c.side !== undefined) updated.side = pd.c.side as 'long' | 'short';
          newPositions.set(pd.id, updated);
        }
      }
    }

    positions.value = newPositions;

    // Apply balance changes
    if (delta.b) {
      const currentBalance = balance.value || { available: 0, total: 0, equity: 0 };
      balance.value = {
        available: delta.b.available ?? currentBalance.available,
        total: delta.b.total ?? currentBalance.total,
        equity: delta.b.equity ?? currentBalance.equity,
      };
    }

    if (onPositionsUpdate) {
      onPositionsUpdate(positions.value);
    }
  }

  /**
   * Handle legacy tick_snapshot messages (backwards compatibility).
   * Legacy messages may lack a timestamp field, so we inject the batch-level
   * server timestamp (msg.ts) when available, falling back to Date.now().
   */
  function handleTickSnapshot(payload: { symbols: SymbolTick[]; ts?: number }): void {
    if (!payload?.symbols) return;

    // payload.ts originates from TickSnapshot.Ts (Unix seconds) — convert to ms
    const fallbackTs = payload.ts ? payload.ts * 1000 : Date.now();
    const newPrices = new Map(prices.value);
    for (const tick of payload.symbols) {
      if (!tick.timestamp) {
        tick.timestamp = fallbackTs;
      }
      newPrices.set(tick.symbol, tick);
    }
    prices.value = newPrices;

    if (onPricesUpdate) {
      onPricesUpdate(prices.value);
    }
  }

  /**
   * Handle order acknowledgment from server
   */
  function handleOrderAck(ack: OrderAck): void {
    const pending = pendingOrders.get(ack.request_id);
    if (pending) {
      clearTimeout(pending.timeout);
      pendingOrders.delete(ack.request_id);
      pending.resolve(ack);
      tradingLogger.info(`Order accepted: ${ack.order_id}`);

      // Update queued order status if applicable
      const queuedOrderId = queuedOrderToPendingMap.get(ack.request_id);
      if (queuedOrderId) {
        orderQueueComposable.markAsAcknowledged(queuedOrderId, ack);
        queuedOrderToPendingMap.delete(ack.request_id);
      }
    }

    if (onOrderAccepted) {
      onOrderAccepted(ack);
    }
  }

  /**
   * Handle order rejection from server
   */
  function handleOrderReject(reject: OrderReject): void {
    const pending = pendingOrders.get(reject.request_id);
    if (pending) {
      clearTimeout(pending.timeout);
      pendingOrders.delete(reject.request_id);
      pending.reject(reject);
      tradingLogger.warn(`Order rejected: ${reject.code} - ${reject.message}`);

      // Update queued order status if applicable
      const queuedOrderId = queuedOrderToPendingMap.get(reject.request_id);
      if (queuedOrderId) {
        orderQueueComposable.markAsRejected(queuedOrderId, reject.message);
        queuedOrderToPendingMap.delete(reject.request_id);
      }
    }

    // Handle rate limiting - set state and timer
    if (reject.code === 'RATE_LIMITED' && reject.rate_limit) {
      const retryMs = reject.rate_limit.retry_after_ms;

      // Update rate limit state
      isRateLimited.value = true;
      rateLimitRetryAfterMs.value = retryMs;
      lastRateLimitInfo.value = reject.rate_limit;

      // Clear any existing timer
      if (rateLimitTimer) {
        clearTimeout(rateLimitTimer);
      }

      // Set timer to clear rate limit state
      rateLimitTimer = setTimeout(() => {
        isRateLimited.value = false;
        rateLimitRetryAfterMs.value = 0;
        rateLimitTimer = null;
      }, retryMs);

      tradingLogger.warn(`Rate limited: ${reject.rate_limit.scope} scope, retry after ${retryMs}ms`);

      // Call rate limit callback if provided
      if (onRateLimited) {
        onRateLimited(retryMs, reject.rate_limit);
      }
    }

    if (onOrderRejected) {
      onOrderRejected(reject);
    }
  }

  /**
   * Handle order cancelled event from server
   */
  function handleOrderCancelled(cancelled: OrderCancelled): void {
    tradingLogger.info(`Order cancelled: ${cancelled.order_id}`);

    if (onOrderCancelled) {
      onOrderCancelled(cancelled);
    }
  }

  /**
   * Handle notification messages from WebSocket (prize_won, contest events, etc.)
   */
  function handleNotificationMessage(payload: Record<string, unknown>): void {
    try {
      const notificationStore = useNotificationStore();
      const toast = useToast();

      // Refresh unread count for badge update
      notificationStore.fetchUnreadCount();

      // If notification list is already loaded, refresh it
      if (notificationStore.initialized) {
        notificationStore.fetchNotifications();
      }

      // Show toast notification using i18n renderer
      const { renderNotification } = useNotificationRenderer();
      const fakeNotification = {
        id: '',
        user_id: '',
        type: ((payload?.type as string) || 'system') as import('@/api/notifications').NotificationType,
        title: '',
        message: '',
        read_at: null,
        created_at: new Date().toISOString(),
        metadata: (payload as Record<string, string | number | boolean>) || {},
      };
      const rendered = renderNotification(fakeNotification);
      toast.info(rendered.title);
    } catch (error) {
      tradingLogger.error('Failed to handle notification message:', error);
    }
  }

  /**
   * Process incoming WebSocket messages
   */
  function processMessage(data: unknown): void {
    if (!data || typeof data !== 'object') return;

    const msg = data as Record<string, unknown>;
    const type = msg.type as string;

    try {
      switch (type) {
        case 'tick_batch':
          handleTickBatch(msg as unknown as BatchedMessage<TickBatchData>);
          break;
        case 'state_delta':
          handleStateDelta(msg as unknown as StateDelta);
          break;
        case 'tick_snapshot':
          // Legacy support — pass ts from the message envelope so ticks get a server timestamp
          handleTickSnapshot({ ...(msg.payload as { symbols: SymbolTick[] }), ts: msg.ts as number | undefined });
          break;
        case 'welcome':
        case 'contest_state':
          // Server welcome message - connection established
          // Backend sends 'contest_state' with phase: 'CONNECTING' as the welcome message
          tradingLogger.info('Received welcome message', { type });
          break;
        case 'order_ack':
          handleOrderAck(msg as unknown as OrderAck);
          break;
        case 'order_reject':
          handleOrderReject(msg as unknown as OrderReject);
          break;
        case 'order_cancelled':
          handleOrderCancelled(msg as unknown as OrderCancelled);
          break;
        case 'fill':
        case 'fill_event':
          tradingLogger.debug(`Received ${type} message`);
          if (onFillEvent) onFillEvent(msg);
          break;
        case 'position_update':
          tradingLogger.debug('Received position_update message');
          if (onPositionUpdate) onPositionUpdate(msg);
          break;
        case 'pnl_delta':
          tradingLogger.debug('Received pnl_delta message');
          if (onPnLDelta) onPnLDelta(msg);
          break;
        case 'balance_update':
          tradingLogger.debug('Received balance_update message');
          if (onBalanceUpdate) onBalanceUpdate(msg);
          break;
        case 'contest_cancelled':
          tradingLogger.warn('Contest cancelled', msg.payload as Record<string, unknown>);
          break;
        case 'notification':
          handleNotificationMessage(msg.payload as Record<string, unknown>);
          break;
        default:
          tradingLogger.debug(`Unknown message type: ${type}`);
      }
    } catch (error) {
      tradingLogger.error(`Failed to process message type: ${type}`, error);
    }
  }

  // Watch for new messages from the base WebSocket
  let stopWatchMessage: WatchStopHandle | null = null;

  function setupMessageWatcher(): void {
    // Stop any existing watcher before setting up a new one
    if (stopWatchMessage) {
      stopWatchMessage();
      stopWatchMessage = null;
    }

    // Use Vue's watch() to reactively respond to new messages
    // This is more efficient than polling with setInterval and ensures
    // messages are processed in order as they arrive
    stopWatchMessage = watch(
      lastMessage,
      (newMessage: WebSocketMessage | null) => {
        if (newMessage) {
          processMessage(newMessage.data);
        }
      },
      { flush: 'sync' } // Process messages synchronously to preserve arrival order
    );
  }

  // ============================================
  // Actions
  // ============================================

  /**
   * Connect to the trading WebSocket
   */
  function connect(): void {
    // Reset sequence tracking on new connection
    lastSequence.value = 0;
    sequenceGaps.value = 0;

    setupMessageWatcher();
    baseConnect();
  }

  /**
   * Disconnect from the trading WebSocket
   */
  function disconnect(): void {
    if (stopWatchMessage) {
      stopWatchMessage();
      stopWatchMessage = null;
    }
    if (tickFlushTimer) {
      clearTimeout(tickFlushTimer);
      tickFlushTimer = null;
    }
    if (rateLimitTimer) {
      clearTimeout(rateLimitTimer);
      rateLimitTimer = null;
    }
    baseDisconnect();
  }

  /**
   * Update the contest ID and rebuild the WebSocket URL
   */
  function updateContestId(newContestId: string): void {
    const newUrl = `${baseUrl}?contest_id=${encodeURIComponent(newContestId)}`;
    setUrl(newUrl);
  }

  /**
   * Reset and reconnect (clears all state)
   */
  function resetAndReconnect(): void {
    prices.value = new Map();
    positions.value = new Map();
    balance.value = null;
    lastSequence.value = 0;
    sequenceGaps.value = 0;
    tickBuffer.value.clear();
    // Clear tick flush timer to prevent stale flush after reconnect
    if (tickFlushTimer) {
      clearTimeout(tickFlushTimer);
      tickFlushTimer = null;
    }

    // Clear pending orders with timeout error
    for (const [, pending] of pendingOrders) {
      clearTimeout(pending.timeout);
      pending.reject(new Error('Connection reset'));
    }
    pendingOrders.clear();

    if (stopWatchMessage) {
      stopWatchMessage();
      stopWatchMessage = null;
    }

    setupMessageWatcher();
    baseResetAndReconnect();
  }

  /**
   * Request a full resync from the server
   */
  function requestResync(): void {
    send({ type: 'resync_request' });
    tradingLogger.info('Resync requested');
  }

  /**
   * Send an order request via WebSocket
   * @internal Used by placeOrder and queue resubmission
   */
  function sendOrderRequest(options: PlaceOrderOptions, queuedOrderId?: string): Promise<OrderAck> {
    return new Promise((resolve, reject) => {
      // Generate unique request ID
      const requestId = crypto.randomUUID();

      // Track queued order mapping if provided
      if (queuedOrderId) {
        queuedOrderToPendingMap.set(requestId, queuedOrderId);
      }

      // Build order request
      const orderRequest: OrderRequest = {
        type: 'order_request',
        request_id: requestId,
        symbol: options.symbol,
        side: options.side,
        order_type: options.orderType,
        qty: options.qty,
        limit_price: options.limitPrice,
        stop_price: options.stopPrice,
        take_profit: options.takeProfit,
        stop_loss: options.stopLoss,
      };

      // Set timeout for order response
      const timeout = setTimeout(() => {
        pendingOrders.delete(requestId);
        queuedOrderToPendingMap.delete(requestId);
        if (queuedOrderId) {
          orderQueueComposable.markAsRejected(queuedOrderId, 'Order request timed out');
        }
        reject(new Error('Order request timed out'));
      }, ORDER_TIMEOUT);

      // Track pending order
      pendingOrders.set(requestId, {
        request: orderRequest,
        resolve,
        reject,
        timeout,
      });

      // Send order request
      send(orderRequest);
      tradingLogger.info(`Order request sent: ${requestId}`, {
        symbol: options.symbol,
        side: options.side,
        orderType: options.orderType,
        qty: options.qty,
        queuedOrderId,
      });
    });
  }

  /**
   * Place an order via WebSocket
   * If disconnected and queue is enabled, the order will be queued for later submission.
   * @param options Order parameters
   * @returns Promise that resolves with OrderAck on success or rejects with OrderReject/Error on failure
   */
  function placeOrder(options: PlaceOrderOptions): Promise<OrderAck | { status: 'queued'; queuedOrder: QueuedOrder }> {
    // If connected, send immediately
    if (status.value === 'connected') {
      return sendOrderRequest(options);
    }

    // If queue is disabled, reject immediately
    if (!enableOrderQueue) {
      return Promise.reject(new Error('WebSocket not connected'));
    }

    // Queue the order for later
    const result = orderQueueComposable.queueOrder(options, contestId);

    if (!result.success) {
      return Promise.reject(new Error(result.error || 'Failed to queue order'));
    }

    tradingLogger.info(`Order queued for later submission: ${result.queuedOrder!.id}`);

    // Return a resolved promise indicating the order was queued (not an error)
    return Promise.resolve({ status: 'queued' as const, queuedOrder: result.queuedOrder! });
  }

  /**
   * Process queued orders after reconnection confirmation
   */
  async function processQueuedOrders(): Promise<void> {
    const ordersToSubmit = orderQueueComposable.confirmAndResubmit();

    if (ordersToSubmit.length === 0) {
      return;
    }

    tradingLogger.info(`Processing ${ordersToSubmit.length} queued orders`);

    for (let i = 0; i < ordersToSubmit.length; i++) {
      const queuedOrder = ordersToSubmit[i];

      // Mark as sent before submitting
      orderQueueComposable.markAsSent(queuedOrder.id);

      try {
        // Wait for delay between orders (except for first one)
        if (i > 0) {
          await new Promise(resolve => setTimeout(resolve, 100));
        }

        // Check if still connected
        if (status.value !== 'connected') {
          tradingLogger.warn('Lost connection while processing queue, stopping');
          // Re-queue remaining orders
          for (let j = i; j < ordersToSubmit.length; j++) {
            const remaining = ordersToSubmit[j];
            remaining.status = 'queued';
          }
          break;
        }

        // Send the order
        await sendOrderRequest(queuedOrder.options, queuedOrder.id);
      } catch (error) {
        tradingLogger.warn(`Failed to send queued order ${queuedOrder.id}:`, error);
        // Already marked as rejected in sendOrderRequest timeout
      }
    }
  }

  // Computed arrays for easier iteration
  const pricesArray = computed(() => Array.from(prices.value.values()));
  const positionsArray = computed(() => Array.from(positions.value.values()));

  // Track previous connection status for reconnection detection
  let previousStatus: ConnectionStatus = 'disconnected';

  // Watch for reconnection and handle queued orders
  const stopWatchStatus = watch(status, (newStatus) => {
    // Detect reconnection (was disconnected/reconnecting, now connected)
    if (newStatus === 'connected' && (previousStatus === 'disconnected' || previousStatus === 'reconnecting' || previousStatus === 'error')) {
      // Check if there are queued orders to process
      const queuedOrders = orderQueueComposable.getQueuedOrders();
      if (queuedOrders.length > 0 && enableOrderQueue) {
        tradingLogger.info(`Reconnected with ${queuedOrders.length} queued orders`);

        // Get active contest IDs (current contest + any provided)
        const activeContests = activeContestIds
          ? [...new Set([contestId, ...activeContestIds])]
          : [contestId];

        // Handle reconnection - filter stale/invalid orders and prompt for confirmation
        orderQueueComposable.handleReconnection(activeContests);
      }
    }

    previousStatus = newStatus;
  });

  // Cleanup on unmount — stop the status watcher created outside setup lifecycle
  onUnmounted(() => {
    stopWatchStatus();
  });

  // Order queue action wrappers
  function removeQueuedOrder(orderId: string): void {
    orderQueueComposable.removeOrder(orderId);
  }

  function clearOrderQueue(): void {
    orderQueueComposable.clearQueue();
  }

  function confirmQueuedOrders(): void {
    processQueuedOrders();
  }

  function cancelQueuedOrders(): void {
    orderQueueComposable.cancelPendingConfirmation();
  }

  // Cleanup on unmount
  onUnmounted(() => {
    // Clear pending orders
    for (const [, pending] of pendingOrders) {
      clearTimeout(pending.timeout);
      pending.reject(new Error('Component unmounted'));
    }
    pendingOrders.clear();

    // Clear rate limit timer
    if (rateLimitTimer) {
      clearTimeout(rateLimitTimer);
      rateLimitTimer = null;
    }

    disconnect();
  });

  return {
    // Connection state
    status,
    reconnectAttempts,

    // Raw message forwarding (for provide/inject to child components)
    lastMessage,

    // Trading data
    prices,
    positions,
    balance,

    // Sequence tracking
    lastSequence,
    sequenceGaps,

    // Rate limit state
    rateLimit: {
      isLimited: isRateLimited,
      retryAfterMs: rateLimitRetryAfterMs,
      lastRateLimitInfo,
    },

    // Order queue state
    orderQueue: orderQueueComposable.queue,
    orderQueuePendingCount: orderQueueComposable.pendingCount,
    isOrderQueueFull: orderQueueComposable.isQueueFull,
    orderQueuePendingConfirmation: orderQueueComposable.pendingConfirmation,
    showOrderQueueConfirmation: orderQueueComposable.showConfirmationDialog,

    // Computed values
    pricesArray,
    positionsArray,

    // Actions
    connect,
    disconnect,
    send,
    resetAndReconnect,
    requestResync,
    updateContestId,

    // Order placement
    placeOrder,

    // Order queue actions
    removeQueuedOrder,
    clearOrderQueue,
    confirmQueuedOrders,
    cancelQueuedOrders,
  };
}
