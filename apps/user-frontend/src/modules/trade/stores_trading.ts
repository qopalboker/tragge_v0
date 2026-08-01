import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import type { Position, FillEvent, PnLDelta, OrderHistoryItem, OrderHistoryOptions } from '@/types/contracts';
import { closePosition as closePositionApi, cancelOrder as cancelOrderApi, updateTPSL as updateTPSLApi, getOrderHistory as getOrderHistoryApi, getBalance as getBalanceApi } from '@/api';

export interface SymbolPrice {
  symbol: string;
  bid: number;
  ask: number;
  last: number;
}

export interface Order {
  order_id: string;
  symbol: string;
  side: string;
  type: string;
  qty: number;
  limit_price?: number;
  stop_price?: number;
  status: string;
  created_at: number;
}

export interface LeaderboardEntry {
  rank: number;
  user_id: string;
  username?: string;
  total_score: number;
  rank_change?: number;
  previous_rank?: number;
}

export interface RankHistoryEntry {
  rank: number;
  timestamp: number;
}

export interface RankMilestone {
  type: 'top_10' | 'prize_zone' | 'first_place' | 'rank_up' | 'rank_down';
  rank: number;
  previousRank?: number;
  timestamp: number;
}

export interface RankState {
  current: number | null;
  previous: number | null;
  change: number;
  highest: number | null;
  lowest: number | null;
  history: RankHistoryEntry[];
  lastMilestone: RankMilestone | null;
}

export const useTradingStore = defineStore('trading', () => {
  // Positions indexed by position_id
  const positions = ref<Map<string, Position>>(new Map());

  // Orders indexed by order_id
  const orders = ref<Map<string, Order>>(new Map());

  // Live prices indexed by symbol
  const prices = ref<Map<string, SymbolPrice>>(new Map());

  // Recent fills (limited to last 50)
  const fills = ref<FillEvent[]>([]);
  const maxFills = 50;

  // PnL tracking - split into realized and unrealized components
  const realizedScore = ref<number>(0);
  const unrealizedScore = ref<number>(0);
  const currentPnL = ref<number>(0);  // Total = realized + unrealized
  const pnlHistory = ref<PnLDelta[]>([]);
  const maxPnLHistory = 100;

  // Leaderboard tracking
  const leaderboardEntries = ref<LeaderboardEntry[]>([]);
  const userRank = ref<number | null>(null);
  const previousRanks = ref<Map<string, number>>(new Map());
  const userRankChange = ref<number>(0);

  // Enhanced rank tracking
  const rankHistory = ref<RankHistoryEntry[]>([]);
  const maxRankHistory = 100;
  const highestRank = ref<number | null>(null);
  const lowestRank = ref<number | null>(null);
  const lastRankMilestone = ref<RankMilestone | null>(null);
  const totalParticipants = ref<number>(0);
  const prizeZoneThreshold = ref<number>(0.3); // Top 30% by default

  // QTY tracking for contest (from API)
  const totalQTY = ref<number>(0);
  const qtyAvailable = ref<number>(0);
  const qtyUsed = ref<number>(0);
  const balanceLoading = ref<boolean>(false);
  const balanceError = ref<string | null>(null);
  const balanceLoaded = ref<boolean>(false);

  // Loading state for closing positions
  const closingPositionId = ref<string | null>(null);

  // Loading state for cancelling orders
  const cancellingOrderId = ref<string | null>(null);

  // Loading state for updating TP/SL
  const updatingTPSLPositionId = ref<string | null>(null);

  // Order history state
  const orderHistory = ref<OrderHistoryItem[]>([]);
  const historyTotal = ref<number>(0);
  const historyLoading = ref<boolean>(false);
  const historyError = ref<string | null>(null);
  const historyLimit = 20;
  const historyOffset = ref<number>(0);

  // Computed: QTY used (from API value, fallback to positions if not set)
  const usedQTY = computed(() => {
    if (qtyUsed.value > 0) {
      return qtyUsed.value;
    }
    // Fallback: calculate from positions
    let used = 0;
    positions.value.forEach((position) => {
      used += position.qty_used || position.qty;
    });
    return used;
  });

  // Computed: Available QTY (from API, includes pending orders)
  const availableQTY = computed(() => {
    if (balanceLoaded.value) {
      return qtyAvailable.value;
    }
    // Before API loads, fallback to calculation from positions
    return Math.max(0, totalQTY.value - usedQTY.value);
  });

  function updatePositions(positionUpdate: { positions: Position[] }): void {
    // Clear existing positions and replace with new data
    positions.value.clear();

    positionUpdate.positions.forEach((position) => {
      positions.value.set(position.position_id, position);
    });
  }

  function updatePrices(ticks: SymbolPrice[]): void {
    const newPrices = new Map(prices.value);
    for (const tick of ticks) {
      newPrices.set(tick.symbol, tick);
    }
    prices.value = newPrices;
  }

  function addOrder(order: Order): void {
    orders.value.set(order.order_id, order);
  }

  function updateOrderStatus(orderId: string, status: string): void {
    const order = orders.value.get(orderId);
    if (order) {
      order.status = status;
      orders.value.set(orderId, order);
    }
  }

  function removeOrder(orderId: string): void {
    orders.value.delete(orderId);
  }

  function addFill(fill: FillEvent): void {
    // Add to beginning of array
    fills.value.unshift(fill);

    // Limit size
    if (fills.value.length > maxFills) {
      fills.value = fills.value.slice(0, maxFills);
    }

    // Update order status if it exists
    updateOrderStatus(fill.order_id, 'FILLED');
  }

  function updatePnL(pnlDelta: PnLDelta): void {
    // Update all score components
    realizedScore.value = pnlDelta.realized_score ?? realizedScore.value;
    unrealizedScore.value = pnlDelta.unrealized_score ?? unrealizedScore.value;
    currentPnL.value = pnlDelta.total_score;

    // Add to history
    pnlHistory.value.unshift(pnlDelta);

    // Limit size
    if (pnlHistory.value.length > maxPnLHistory) {
      pnlHistory.value = pnlHistory.value.slice(0, maxPnLHistory);
    }
  }

  function updateLeaderboard(entries: LeaderboardEntry[], currentUserId?: string): void {
    // Update total participants count
    totalParticipants.value = entries.length;

    // Calculate rank changes based on previous ranks
    const updatedEntries = entries.map(entry => {
      const prevRank = previousRanks.value.get(entry.user_id);
      let rankChange = 0;

      if (prevRank !== undefined && prevRank !== entry.rank) {
        rankChange = prevRank - entry.rank; // Positive means moved up
      }

      return {
        ...entry,
        rank_change: rankChange,
        previous_rank: prevRank,
      };
    });

    leaderboardEntries.value = updatedEntries;

    // Replace previousRanks with only current entries to prevent unbounded growth
    const newPreviousRanks = new Map<string, number>();
    for (const entry of entries) {
      newPreviousRanks.set(entry.user_id, entry.rank);
    }
    previousRanks.value = newPreviousRanks;

    // Find user's rank and rank change if userId is provided
    if (currentUserId) {
      const userEntry = updatedEntries.find(entry => entry.user_id === currentUserId);
      if (userEntry) {
        const prevUserRank = userRank.value;
        userRank.value = userEntry.rank;

        // Calculate user's rank change
        if (prevUserRank !== null && prevUserRank !== userEntry.rank) {
          userRankChange.value = prevUserRank - userEntry.rank;
        }

        // Update rank history and milestones
        updateRankTracking(userEntry.rank, prevUserRank);
      } else {
        userRank.value = null;
        userRankChange.value = 0;
      }
    }
  }

  /**
   * Update rank tracking: history, highest/lowest, and milestones.
   */
  function updateRankTracking(newRank: number, previousRank: number | null): void {
    const now = Date.now();

    // Add to history
    rankHistory.value.unshift({ rank: newRank, timestamp: now });
    if (rankHistory.value.length > maxRankHistory) {
      rankHistory.value = rankHistory.value.slice(0, maxRankHistory);
    }

    // Update highest/lowest ranks
    if (highestRank.value === null || newRank < highestRank.value) {
      highestRank.value = newRank;
    }
    if (lowestRank.value === null || newRank > lowestRank.value) {
      lowestRank.value = newRank;
    }

    // Detect milestones
    const milestone = detectRankMilestone(newRank, previousRank);
    if (milestone) {
      lastRankMilestone.value = milestone;
    }
  }

  /**
   * Detect if a rank change triggers a milestone notification.
   */
  function detectRankMilestone(newRank: number, previousRank: number | null): RankMilestone | null {
    const now = Date.now();
    const participants = totalParticipants.value;

    // First place milestone
    if (newRank === 1 && previousRank !== 1) {
      return { type: 'first_place', rank: newRank, previousRank: previousRank ?? undefined, timestamp: now };
    }

    // Top 10 milestone (entering top 10)
    if (newRank <= 10 && (previousRank === null || previousRank > 10)) {
      return { type: 'top_10', rank: newRank, previousRank: previousRank ?? undefined, timestamp: now };
    }

    // Prize zone milestone (entering top 30%)
    if (participants > 0) {
      const prizeZoneRank = Math.ceil(participants * prizeZoneThreshold.value);
      const wasInPrizeZone = previousRank !== null && previousRank <= prizeZoneRank;
      const isInPrizeZone = newRank <= prizeZoneRank;

      if (isInPrizeZone && !wasInPrizeZone) {
        return { type: 'prize_zone', rank: newRank, previousRank: previousRank ?? undefined, timestamp: now };
      }
    }

    // Rank up/down milestones (only if significant change)
    if (previousRank !== null && previousRank !== newRank) {
      const change = previousRank - newRank;
      if (change > 0) {
        return { type: 'rank_up', rank: newRank, previousRank, timestamp: now };
      } else {
        return { type: 'rank_down', rank: newRank, previousRank, timestamp: now };
      }
    }

    return null;
  }

  /**
   * Check if user is currently in the prize zone.
   */
  function isInPrizeZone(): boolean {
    if (userRank.value === null || totalParticipants.value === 0) return false;
    const prizeZoneRank = Math.ceil(totalParticipants.value * prizeZoneThreshold.value);
    return userRank.value <= prizeZoneRank;
  }

  /**
   * Get the rank state object for external use.
   */
  function getRankState(): RankState {
    return {
      current: userRank.value,
      previous: rankHistory.value[1]?.rank ?? null,
      change: userRankChange.value,
      highest: highestRank.value,
      lowest: lowestRank.value,
      history: rankHistory.value,
      lastMilestone: lastRankMilestone.value,
    };
  }

  /**
   * Clear the last milestone (after it has been displayed).
   */
  function clearLastMilestone(): void {
    lastRankMilestone.value = null;
  }

  /**
   * Set the prize zone threshold (0-1, default 0.3 = top 30%).
   */
  function setPrizeZoneThreshold(threshold: number): void {
    prizeZoneThreshold.value = Math.max(0, Math.min(1, threshold));
  }

  function setTotalQTY(qty: number): void {
    totalQTY.value = qty;
  }

  /**
   * Update QTY values from balance response.
   */
  function updateBalance(total: number, available: number, used: number): void {
    totalQTY.value = total;
    qtyAvailable.value = available;
    qtyUsed.value = used;
    balanceLoaded.value = true;
  }

  /**
   * Fetch QTY balance for a contest from API.
   * @param contestId - The contest ID to fetch balance for
   */
  async function fetchBalance(contestId: string): Promise<void> {
    balanceLoading.value = true;
    balanceError.value = null;

    try {
      const response = await getBalanceApi(contestId);
      updateBalance(response.qty_total, response.qty_available, response.qty_used);
    } catch (error) {
      balanceError.value = error instanceof Error ? error.message : 'Failed to fetch balance';
    } finally {
      balanceLoading.value = false;
    }
  }

  function clearAll(): void {
    positions.value.clear();
    orders.value.clear();
    prices.value.clear();
    fills.value = [];
    pnlHistory.value = [];
    realizedScore.value = 0;
    unrealizedScore.value = 0;
    currentPnL.value = 0;
    leaderboardEntries.value = [];
    userRank.value = null;
    previousRanks.value.clear();
    userRankChange.value = 0;
    // Clear rank tracking state
    rankHistory.value = [];
    highestRank.value = null;
    lowestRank.value = null;
    lastRankMilestone.value = null;
    totalParticipants.value = 0;
    totalQTY.value = 0;
    qtyAvailable.value = 0;
    qtyUsed.value = 0;
    balanceLoading.value = false;
    balanceError.value = null;
    balanceLoaded.value = false;
    closingPositionId.value = null;
    cancellingOrderId.value = null;
    updatingTPSLPositionId.value = null;
    orderHistory.value = [];
    historyTotal.value = 0;
    historyLoading.value = false;
    historyError.value = null;
    historyOffset.value = 0;
  }

  // Computed: check if a specific position is being closed
  const isClosingPosition = computed(() => (positionId: string) => {
    return closingPositionId.value === positionId;
  });

  // Computed: check if a specific order is being cancelled
  const isCancellingOrder = computed(() => (orderId: string) => {
    return cancellingOrderId.value === orderId;
  });

  // Computed: check if a specific position's TP/SL is being updated
  const isUpdatingTPSL = computed(() => (positionId: string) => {
    return updatingTPSLPositionId.value === positionId;
  });

  /**
   * Close a position (fully or partially).
   * Position will be updated via WebSocket after successful close.
   * @param positionId - The ID of the position to close
   * @param qty - Optional quantity for partial close
   * @returns The order_id from the close request
   * @throws Error if the close request fails
   */
  async function closePosition(positionId: string, qty?: number): Promise<string> {
    closingPositionId.value = positionId;
    try {
      const response = await closePositionApi(positionId, qty);
      return response.order_id;
    } finally {
      closingPositionId.value = null;
    }
  }

  /**
   * Cancel a pending order.
   * Order will be removed via WebSocket after successful cancellation.
   * @param orderId - The ID of the order to cancel
   * @returns The order_id from the cancel request
   * @throws Error if the cancel request fails
   */
  async function cancelOrder(orderId: string): Promise<string> {
    cancellingOrderId.value = orderId;
    try {
      const response = await cancelOrderApi(orderId);
      return response.order_id;
    } finally {
      cancellingOrderId.value = null;
    }
  }

  /**
   * Update TP/SL for a position.
   * @param positionId - The ID of the position to update
   * @param takeProfit - New take profit price (null to remove)
   * @param stopLoss - New stop loss price (null to remove)
   * @throws Error if the update request fails
   */
  async function updateTPSL(
    positionId: string,
    takeProfit?: number | null,
    stopLoss?: number | null
  ): Promise<void> {
    updatingTPSLPositionId.value = positionId;
    try {
      const response = await updateTPSLApi(positionId, takeProfit, stopLoss);
      // Update local position with new TP/SL values
      const position = positions.value.get(positionId);
      if (position) {
        position.take_profit = response.take_profit;
        position.stop_loss = response.stop_loss;
        positions.value.set(positionId, position);
      }
    } finally {
      updatingTPSLPositionId.value = null;
    }
  }

  /**
   * Fetch order history for a contest.
   * Replaces existing history with fresh data.
   * @param contestId - The contest ID to fetch orders for
   * @param options - Optional query parameters (status, symbol)
   */
  async function fetchOrderHistory(
    contestId: string,
    options?: Omit<OrderHistoryOptions, 'limit' | 'offset'>
  ): Promise<void> {
    historyLoading.value = true;
    historyError.value = null;
    historyOffset.value = 0;

    try {
      const response = await getOrderHistoryApi(contestId, {
        ...options,
        limit: historyLimit,
        offset: 0,
      });
      orderHistory.value = response.orders;
      historyTotal.value = response.total;
    } catch (error) {
      historyError.value = error instanceof Error ? error.message : 'Failed to fetch order history';
      orderHistory.value = [];
      historyTotal.value = 0;
    } finally {
      historyLoading.value = false;
    }
  }

  /**
   * Load more order history (pagination).
   * Appends to existing history.
   * @param contestId - The contest ID to fetch orders for
   * @param options - Optional query parameters (status, symbol)
   */
  async function loadMoreHistory(
    contestId: string,
    options?: Omit<OrderHistoryOptions, 'limit' | 'offset'>
  ): Promise<void> {
    // Don't load more if already loading or no more items
    if (historyLoading.value || orderHistory.value.length >= historyTotal.value) {
      return;
    }

    historyLoading.value = true;
    historyError.value = null;
    const nextOffset = historyOffset.value + historyLimit;

    try {
      const response = await getOrderHistoryApi(contestId, {
        ...options,
        limit: historyLimit,
        offset: nextOffset,
      });
      orderHistory.value = [...orderHistory.value, ...response.orders];
      historyTotal.value = response.total;
      historyOffset.value = nextOffset;
    } catch (error) {
      historyError.value = error instanceof Error ? error.message : 'Failed to load more history';
    } finally {
      historyLoading.value = false;
    }
  }

  return {
    positions,
    orders,
    prices,
    fills,
    realizedScore,
    unrealizedScore,
    currentPnL,
    pnlHistory,
    leaderboardEntries,
    userRank,
    userRankChange,
    // Enhanced rank tracking exports
    rankHistory,
    highestRank,
    lowestRank,
    lastRankMilestone,
    totalParticipants,
    prizeZoneThreshold,
    totalQTY,
    usedQTY,
    availableQTY,
    balanceLoading,
    balanceError,
    balanceLoaded,
    closingPositionId,
    cancellingOrderId,
    updatingTPSLPositionId,
    orderHistory,
    historyTotal,
    historyLoading,
    historyError,
    isClosingPosition,
    isCancellingOrder,
    isUpdatingTPSL,
    updatePositions,
    updatePrices,
    addOrder,
    updateOrderStatus,
    removeOrder,
    addFill,
    updatePnL,
    updateLeaderboard,
    setTotalQTY,
    updateBalance,
    fetchBalance,
    clearAll,
    closePosition,
    cancelOrder,
    updateTPSL,
    // Enhanced rank tracking functions
    isInPrizeZone,
    getRankState,
    clearLastMilestone,
    setPrizeZoneThreshold,
    fetchOrderHistory,
    loadMoreHistory,
  };
});
