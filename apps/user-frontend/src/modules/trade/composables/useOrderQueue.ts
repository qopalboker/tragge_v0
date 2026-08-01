import { ref, computed, onUnmounted, type Ref, type ComputedRef } from 'vue';
import { tradingLogger } from '@/utils/logger';
import type { PlaceOrderOptions, OrderAck } from './useTradingWebSocket';

// ============================================
// Types
// ============================================

/** Status of a queued order */
export type QueuedOrderStatus = 'queued' | 'sent' | 'acknowledged' | 'rejected' | 'expired' | 'discarded';

/** A queued order with metadata */
export interface QueuedOrder {
  /** Unique ID for this queued order */
  id: string;
  /** The order options to submit */
  options: PlaceOrderOptions;
  /** Contest ID this order belongs to */
  contestId: string;
  /** Current status of the queued order */
  status: QueuedOrderStatus;
  /** Timestamp when the order was queued */
  queuedAt: number;
  /** Timestamp when the order was sent (if sent) */
  sentAt?: number;
  /** Error message if rejected */
  errorMessage?: string;
}

/** Result of attempting to queue an order */
export interface QueueOrderResult {
  success: boolean;
  queuedOrder?: QueuedOrder;
  error?: string;
}

/** Options for the order queue composable */
export interface UseOrderQueueOptions {
  /** Maximum number of orders in the queue (default: 20) */
  maxQueueSize?: number;
  /** Maximum age in ms before an order is considered stale (default: 30000) */
  staleOrderAgeMs?: number;
  /** Delay between re-submitting orders in ms (default: 100) */
  resubmitDelayMs?: number;
}

/** Return type for the order queue composable */
export interface UseOrderQueueReturn {
  /** All queued orders */
  queue: Ref<QueuedOrder[]>;
  /** Number of orders waiting to be sent */
  pendingCount: ComputedRef<number>;
  /** Whether the queue is at max capacity */
  isQueueFull: ComputedRef<boolean>;
  /** Orders that need confirmation after reconnect */
  pendingConfirmation: Ref<QueuedOrder[]>;
  /** Whether confirmation dialog should be shown */
  showConfirmationDialog: Ref<boolean>;

  /** Add an order to the queue */
  queueOrder: (options: PlaceOrderOptions, contestId: string) => QueueOrderResult;
  /** Remove a specific order from the queue */
  removeOrder: (orderId: string) => void;
  /** Clear all queued orders */
  clearQueue: () => void;
  /** Mark an order as sent */
  markAsSent: (orderId: string) => void;
  /** Mark an order as acknowledged (received ack from server) */
  markAsAcknowledged: (orderId: string, orderAck: OrderAck) => void;
  /** Mark an order as rejected */
  markAsRejected: (orderId: string, errorMessage: string) => void;
  /** Get orders that are ready to send */
  getQueuedOrders: () => QueuedOrder[];
  /** Handle reconnection - filter stale orders and prepare for confirmation */
  handleReconnection: (activeContestIds: string[]) => void;
  /** Confirm and re-submit pending orders */
  confirmAndResubmit: () => QueuedOrder[];
  /** Cancel pending confirmation (discard orders) */
  cancelPendingConfirmation: () => void;
}

// ============================================
// Constants
// ============================================

const DEFAULT_MAX_QUEUE_SIZE = 20;
const DEFAULT_STALE_ORDER_AGE_MS = 30000; // 30 seconds

// ============================================
// Order Queue Composable
// ============================================

export function useOrderQueue(options: UseOrderQueueOptions = {}): UseOrderQueueReturn {
  const {
    maxQueueSize = DEFAULT_MAX_QUEUE_SIZE,
    staleOrderAgeMs = DEFAULT_STALE_ORDER_AGE_MS,
  } = options;

  // Queue state
  const queue = ref<QueuedOrder[]>([]);

  // Orders pending user confirmation after reconnect
  const pendingConfirmation = ref<QueuedOrder[]>([]);
  const showConfirmationDialog = ref(false);

  // Track cleanup timers for proper disposal
  const cleanupTimers: ReturnType<typeof setTimeout>[] = [];

  // Computed values
  const pendingCount = computed(() =>
    queue.value.filter(o => o.status === 'queued').length
  );

  const isQueueFull = computed(() =>
    queue.value.filter(o => o.status === 'queued' || o.status === 'sent').length >= maxQueueSize
  );

  /**
   * Generate a unique ID for a queued order
   */
  function generateOrderId(): string {
    return `qo-${Date.now()}-${Math.random().toString(36).substring(2, 9)}`;
  }

  /**
   * Add an order to the queue
   */
  function queueOrder(orderOptions: PlaceOrderOptions, contestId: string): QueueOrderResult {
    // Check queue capacity
    const activeOrders = queue.value.filter(o => o.status === 'queued' || o.status === 'sent');
    if (activeOrders.length >= maxQueueSize) {
      tradingLogger.warn('Order queue is full, rejecting new order');
      return {
        success: false,
        error: 'Order queue full. Please wait for connection to restore.',
      };
    }

    const queuedOrder: QueuedOrder = {
      id: generateOrderId(),
      options: orderOptions,
      contestId,
      status: 'queued',
      queuedAt: Date.now(),
    };

    queue.value.push(queuedOrder);
    tradingLogger.info(`Order queued: ${queuedOrder.id}`, {
      symbol: orderOptions.symbol,
      side: orderOptions.side,
    });

    return {
      success: true,
      queuedOrder,
    };
  }

  /**
   * Remove a specific order from the queue
   */
  function removeOrder(orderId: string): void {
    const index = queue.value.findIndex(o => o.id === orderId);
    if (index !== -1) {
      const removed = queue.value.splice(index, 1)[0];
      tradingLogger.info(`Order removed from queue: ${orderId}`, { status: removed.status });
    }
  }

  /**
   * Clear all queued orders
   */
  function clearQueue(): void {
    const queuedOrders = queue.value.filter(o => o.status === 'queued');
    queue.value = queue.value.filter(o => o.status !== 'queued');
    tradingLogger.info(`Cleared ${queuedOrders.length} orders from queue`);
  }

  /**
   * Mark an order as sent
   */
  function markAsSent(orderId: string): void {
    const order = queue.value.find(o => o.id === orderId);
    if (order) {
      order.status = 'sent';
      order.sentAt = Date.now();
      tradingLogger.debug(`Order marked as sent: ${orderId}`);
    }
  }

  /**
   * Mark an order as acknowledged (received ack from server)
   */
  function markAsAcknowledged(orderId: string, _orderAck: OrderAck): void {
    const order = queue.value.find(o => o.id === orderId);
    if (order) {
      order.status = 'acknowledged';
      tradingLogger.debug(`Order acknowledged: ${orderId}`);

      // Clean up acknowledged orders after a delay
      cleanupTimers.push(setTimeout(() => {
        removeOrder(orderId);
      }, 5000));
    }
  }

  /**
   * Mark an order as rejected
   */
  function markAsRejected(orderId: string, errorMessage: string): void {
    const order = queue.value.find(o => o.id === orderId);
    if (order) {
      order.status = 'rejected';
      order.errorMessage = errorMessage;
      tradingLogger.warn(`Order rejected: ${orderId}`, { error: errorMessage });

      // Clean up rejected orders after a delay
      cleanupTimers.push(setTimeout(() => {
        removeOrder(orderId);
      }, 10000));
    }
  }

  /**
   * Get orders that are ready to send
   */
  function getQueuedOrders(): QueuedOrder[] {
    return queue.value.filter(o => o.status === 'queued');
  }

  /**
   * Check if an order is stale (older than threshold)
   */
  function isOrderStale(order: QueuedOrder): boolean {
    return Date.now() - order.queuedAt > staleOrderAgeMs;
  }

  /**
   * Handle reconnection - filter stale orders and prepare for confirmation
   */
  function handleReconnection(activeContestIds: string[]): void {
    const queuedOrders = getQueuedOrders();

    if (queuedOrders.length === 0) {
      tradingLogger.debug('No queued orders to process on reconnection');
      return;
    }

    const validOrders: QueuedOrder[] = [];
    const discardedOrders: QueuedOrder[] = [];

    for (const order of queuedOrders) {
      // Check if order is stale (older than 30 seconds)
      if (isOrderStale(order)) {
        order.status = 'expired';
        order.errorMessage = 'Order expired (stale price)';
        discardedOrders.push(order);
        continue;
      }

      // Check if contest is still active
      if (!activeContestIds.includes(order.contestId)) {
        order.status = 'discarded';
        order.errorMessage = 'Contest ended';
        discardedOrders.push(order);
        continue;
      }

      validOrders.push(order);
    }

    tradingLogger.info(`Reconnection: ${validOrders.length} valid orders, ${discardedOrders.length} discarded`);

    // Clean up discarded orders after a delay
    for (const order of discardedOrders) {
      cleanupTimers.push(setTimeout(() => {
        removeOrder(order.id);
      }, 5000));
    }

    // If there are valid orders, prompt for confirmation
    if (validOrders.length > 0) {
      pendingConfirmation.value = validOrders;
      showConfirmationDialog.value = true;
    }
  }

  /**
   * Confirm and return orders to re-submit
   * The caller is responsible for actually sending the orders with delay
   */
  function confirmAndResubmit(): QueuedOrder[] {
    const ordersToResubmit = [...pendingConfirmation.value];

    // Clear confirmation state
    pendingConfirmation.value = [];
    showConfirmationDialog.value = false;

    tradingLogger.info(`User confirmed ${ordersToResubmit.length} orders for resubmission`);

    return ordersToResubmit;
  }

  /**
   * Cancel pending confirmation (discard orders)
   */
  function cancelPendingConfirmation(): void {
    for (const order of pendingConfirmation.value) {
      order.status = 'discarded';
      order.errorMessage = 'User cancelled';

      // Clean up after a delay
      cleanupTimers.push(setTimeout(() => {
        removeOrder(order.id);
      }, 5000));
    }

    tradingLogger.info(`User cancelled ${pendingConfirmation.value.length} pending orders`);

    pendingConfirmation.value = [];
    showConfirmationDialog.value = false;
  }

  onUnmounted(() => {
    cleanupTimers.forEach(clearTimeout);
  });

  return {
    // State
    queue,
    pendingCount,
    isQueueFull,
    pendingConfirmation,
    showConfirmationDialog,

    // Actions
    queueOrder,
    removeOrder,
    clearQueue,
    markAsSent,
    markAsAcknowledged,
    markAsRejected,
    getQueuedOrders,
    handleReconnection,
    confirmAndResubmit,
    cancelPendingConfirmation,
  };
}
