<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, provide } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { t } from '@/i18n';
import { useAuthStore } from '@/stores/auth';
import { useI18nStore } from '@/stores/i18n';
import { useThemeStore } from '@/stores/theme';
import { useTradingStore } from '@/stores/trading';
import { useTradingWebSocket } from '@/composables/useTradingWebSocket';
import type { OrderSide, OrderType } from '@/composables/useTradingWebSocket';
import { wsConfig } from '@/config';
import { useRankNotifications } from '@/composables/useRankNotifications';
import type { PositionUpdate, FillEvent, PnLDelta, ContestDetailsResponse, PrizePreviewResponse } from '@/types/contracts';
import { tradingLogger } from '@/utils/logger';
import { getErrorMessage } from '@/utils/errorHandler';
import { getContestDetails, getPrizePreview, getLeaderboard } from '@/api';
import { getSymbolMetadata } from '@/utils/symbolMetadata';

// New trading panel components
import TradingNavbar from '@/components/trading/TradingNavbar.vue';
import WatchlistSidebar from '@/components/trading/WatchlistSidebar.vue';
import MarketChart from '@/components/MarketChart.vue';
import type { TickData } from '@/composables/useChartData';
import BottomPanel from '@/components/trading/BottomPanel.vue';
import DesktopLeaderboard from '@/components/trading/DesktopLeaderboard.vue';

// Mobile components
import MobileHeader from '@/components/trading/mobile/MobileHeader.vue';
import MobileBottomNav from '@/components/trading/mobile/MobileBottomNav.vue';
import MobileChartPage from '@/components/trading/mobile/MobileChartPage.vue';
import MobileOrdersPage from '@/components/trading/mobile/MobileOrdersPage.vue';
import MobileLeaderboardPage from '@/components/trading/mobile/MobileLeaderboardPage.vue';
import MobileDetailsPage from '@/components/trading/mobile/MobileDetailsPage.vue';

import type { WatchlistItem } from '@/modules/trade/components/trading/WatchlistSidebar.vue';

const route = useRoute();
const router = useRouter();
const authStore = useAuthStore();
const i18nStore = useI18nStore();
const themeStore = useThemeStore();
const tradingStore = useTradingStore();

// Initialize rank notifications
useRankNotifications({
  soundEnabled: false,
  minRankChangeForNotification: 1,
});

const contestId = computed(() => route.params.contestId as string);
const direction = computed(() => i18nStore.direction);

// Layout state
const activeView = ref<'trade' | 'leaderboard'>('trade');
const sidebarCollapsed = ref(false);
const bottomPanelHeight = ref(200);
const sidebarWidth = ref(392);

// Mobile state
const isMobile = ref(false);
const mobileActiveTab = ref<'chart' | 'orders' | 'leaderboard' | 'details'>('chart');

// Symbol and trading state
const selectedSymbol = ref('');
const quantity = ref(1);
const maxQty = ref(100);

// Loading / error state
const isLoading = ref(false);
const loadError = ref<string | null>(null);

// Contest info from API
const contestInfo = ref<ContestDetailsResponse | null>(null);
const prizePreview = ref<PrizePreviewResponse | null>(null);

// Real-time tick data for MarketChart component
const chartTicks = ref<TickData[]>([]);

// Server time drift for countdown synchronization
const serverTimeDrift = ref(0);
const currentTime = ref(Date.now());

// Remaining seconds computed from real contest end time
const remainingSeconds = computed(() => {
  if (!contestInfo.value) return 0;
  const endMs = new Date(contestInfo.value.end_time).getTime();
  const adjustedNow = currentTime.value + serverTimeDrift.value;
  return Math.max(0, Math.floor((endMs - adjustedNow) / 1000));
});

// Duration minutes computed from start/end times
const durationMinutes = computed(() => {
  if (!contestInfo.value) return 0;
  const startMs = new Date(contestInfo.value.start_time).getTime();
  const endMs = new Date(contestInfo.value.end_time).getTime();
  return Math.max(0, Math.round((endMs - startMs) / 60000));
});

// Account info derived from trading store
const account = computed(() => ({
  balance: tradingStore.totalQTY,
  equity: tradingStore.totalQTY + tradingStore.currentPnL,
  margin: tradingStore.usedQTY,
  freeMargin: tradingStore.availableQTY,
  unrealizedPnl: tradingStore.unrealizedScore,
  realizedPnl: tradingStore.realizedScore,
}));

// Watchlist symbols - populated from contest details
const watchlistSymbols = ref<WatchlistItem[]>([]);

const favorites = ref<string[]>([]);

// Positions and orders — computed from tradingStore to keep UI in sync
const openPositions = computed(() => {
  return Array.from(tradingStore.positions.values()).map(pos => {
    const meta = getSymbolMetadata(pos.symbol);
    const price = tradingStore.prices.get(pos.symbol);

    // Use bid for LONG exit, ask for SHORT exit (matches backend GetExitPrice)
    const currentPrice = price
      ? (pos.side === 'BUY' ? (price.bid || price.last) : (price.ask || price.last))
      : pos.mark_price;

    // Recalculate P&L % from live tick prices (real-time, not stale server value)
    const pnlPct = pos.entry_price > 0
      ? (pos.side === 'BUY'
          ? ((currentPrice - pos.entry_price) / pos.entry_price) * 100
          : ((pos.entry_price - currentPrice) / pos.entry_price) * 100)
      : 0;

    // Use qty_used for score consistency with backend (fallback to qty)
    const qtyUsed = pos.qty_used || pos.qty;

    return {
      id: pos.position_id,
      symbol: pos.symbol,
      base: meta.base,
      quote: meta.quote,
      side: (pos.side === 'BUY' ? 'long' : 'short') as 'long' | 'short',
      qty: pos.qty,
      entryPrice: pos.entry_price,
      currentPrice,
      pnl: qtyUsed * pnlPct, // This is the contest score, not dollar P&L
      pnlPct,
      takeProfit: pos.take_profit,
      stopLoss: pos.stop_loss,
      decimals: meta.decimals,
    };
  });
});

const pendingOrders = computed(() => {
  return Array.from(tradingStore.orders.values())
    .filter(o => o.status !== 'FILLED' && o.status !== 'CANCELLED')
    .map(order => {
      const meta = getSymbolMetadata(order.symbol);
      return {
        id: order.order_id,
        symbol: order.symbol,
        base: meta.base,
        quote: meta.quote,
        side: order.side.toLowerCase() as 'buy' | 'sell',
        type: order.type,
        qty: order.qty,
        limitPrice: order.limit_price,
        stopPrice: order.stop_price,
        status: order.status,
        decimals: meta.decimals,
      };
    });
});

const closedPositions = ref<Array<{
  id: string;
  symbol: string;
  base: string;
  quote: string | null;
  side: 'long' | 'short';
  qty: number;
  entryPrice: number;
  exitPrice: number;
  pnl: number;
  decimals: number;
  closedAt: Date;
}>>([]);

// Track previous position IDs to detect closures in onPositionUpdate
const previousPositionIds = ref<Set<string>>(new Set());

// Track recent fill events by position_id for accurate closed position P&L
// Falls back to symbol-based lookup when position_id is unavailable
const recentFills = ref<Map<string, { fill_price: number; side: string; qty: number; ts: number }>>(new Map());

// Leaderboard - fetched from API
const leaderboardParticipants = ref<Array<{ userId: string; username: string; pnl: number; prize: number }>>([]);

// Computed
const selectedSymbolData = computed(() => {
  return watchlistSymbols.value.find(s => s.symbol === selectedSymbol.value) || watchlistSymbols.value[0];
});


const currentUserRank = computed(() => {
  return tradingStore.userRank ?? 0;
});

// Prize pool in dollars for display
const prizePoolDollars = computed(() => {
  if (contestInfo.value) return contestInfo.value.prize_pool_cents / 100;
  return 0;
});

// Prizes as dollar amounts for MobileDetailsPage
const prizeDollars = computed(() => {
  if (!prizePreview.value) return [];
  return prizePreview.value.prizes.map(p => p.amount_cents / 100);
});

// WebSocket connection using the high-level trading composable
// Provides: tick buffering (100ms debounce), sequence gap detection, WS-based order
// placement with Promise tracking, rate limiting, and offline order queue
const {
  status: wsStatus,
  lastMessage,
  connect,
  disconnect,
  placeOrder,
  // Rate limit and order queue state — available for UI integration:
  // rateLimit, orderQueue, orderQueuePendingCount, isOrderQueueFull,
  // showOrderQueueConfirmation, confirmQueuedOrders, cancelQueuedOrders
  updateContestId,
} = useTradingWebSocket('/ws/trade', {
  contestId: contestId.value,
  acquireTicket: async () => {
    try {
      const response = await fetch('/api/trade/ws-ticket', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authStore.accessToken}`,
          'X-Requested-With': 'XMLHttpRequest',
        },
        body: JSON.stringify({ contest_id: contestId.value }),
      });
      if (!response.ok) return null;
      const data = await response.json();
      return data.ticket;
    } catch {
      return null;
    }
  },
  encoding: wsConfig.encoding,

  // Price updates → sync to watchlist UI + store (fires after 100ms tick buffer flush)
  onPricesUpdate: (priceMap) => {
    const ticks = Array.from(priceMap.values());
    ticks.forEach(tick => {
      const sym = watchlistSymbols.value.find(s => s.symbol === tick.symbol);
      if (sym) {
        if (!sym.openPrice) sym.openPrice = tick.last;
        sym.price = tick.last;
        sym.bid = tick.bid;
        sym.ask = tick.ask;
        sym.change = sym.openPrice ? ((tick.last - sym.openPrice) / sym.openPrice) * 100 : 0;
      }
    });
    tradingStore.updatePrices(ticks);

    // Feed real-time ticks to MarketChart for candle updates
    const fallbackNow = Date.now();
    chartTicks.value = ticks.map(tick => ({
      symbol: tick.symbol,
      bid: tick.bid,
      ask: tick.ask,
      last: tick.last,
      timestamp: tick.timestamp || fallbackNow,
    }));
  },

  // Store dispatch callbacks for messages not natively processed by useTradingWebSocket
  onPositionUpdate: (data) => {
    const d = data as Record<string, unknown>;
    // Backend WSMessage wraps data in "payload" field; tick_batch uses "data" field
    const inner = (d.payload ?? d.data ?? d) as Record<string, unknown>;
    const positions = (inner.positions ?? (inner as unknown as PositionUpdate | undefined)?.positions) as PositionUpdate['positions'] | undefined;
    if (positions) {
      // Detect closed positions before the store replaces them
      const incomingIds = new Set(positions.map(p => p.position_id));
      for (const [id, oldPos] of tradingStore.positions.entries()) {
        if (!incomingIds.has(id)) {
          const meta = getSymbolMetadata(oldPos.symbol);
          // Use actual fill price from recent fill events if available
          // Try position_id key first, then symbol-based fallback
          const recentFill = recentFills.value.get(id) || recentFills.value.get(`sym:${oldPos.symbol}`);
          const price = tradingStore.prices.get(oldPos.symbol);
          const exitPrice = recentFill ? recentFill.fill_price : (price?.last ?? oldPos.mark_price);
          // Calculate P&L using actual exit price
          const isLong = oldPos.side === 'BUY';
          const pnlPct = isLong
            ? ((exitPrice - oldPos.entry_price) / oldPos.entry_price) * 100
            : ((oldPos.entry_price - exitPrice) / oldPos.entry_price) * 100;
          closedPositions.value.unshift({
            id: oldPos.position_id,
            symbol: oldPos.symbol,
            base: meta.base,
            quote: meta.quote,
            side: isLong ? 'long' : 'short',
            qty: oldPos.qty,
            entryPrice: oldPos.entry_price,
            exitPrice,
            pnl: pnlPct * (oldPos.qty_used || oldPos.qty),
            decimals: meta.decimals,
            closedAt: new Date(),
          });
          // Clean up used fill (try both keys)
          if (recentFill) {
            recentFills.value.delete(id);
            recentFills.value.delete(`sym:${oldPos.symbol}`);
          }
        }
      }
      previousPositionIds.value = incomingIds;
      tradingStore.updatePositions({ positions });
    }
  },
  onFillEvent: (data) => {
    const d = data as Record<string, unknown>;
    const fill = (d.payload ?? d.data ?? d) as FillEvent;
    tradingStore.addFill(fill);
    // Track recent fills by position_id for accurate closed position P&L
    // Falls back to symbol key if position_id is missing
    const fillKey = (fill as unknown as Record<string, unknown>).position_id as string || `sym:${fill.symbol}`;
    recentFills.value.set(fillKey, {
      fill_price: fill.fill_price,
      side: fill.side,
      qty: fill.qty,
      ts: fill.ts,
    });
    showToast(t('toast.orderFilled'), 'success');
  },
  onPnLDelta: (data) => {
    const d = data as Record<string, unknown>;
    tradingStore.updatePnL((d.payload ?? d.data ?? d) as PnLDelta);
  },
  onBalanceUpdate: (data) => {
    const d = data as Record<string, unknown>;
    const b = (d.payload ?? d.data ?? d) as { qty_total?: number; qty_available?: number; qty_used?: number };
    if (b.qty_total !== undefined && b.qty_available !== undefined && b.qty_used !== undefined) {
      tradingStore.updateBalance(b.qty_total, b.qty_available, b.qty_used);
    }
  },

  // Order event callbacks
  onOrderAccepted: () => {
    showToast(t('toast.orderPlaced'), 'success');
  },
  onOrderRejected: (reject) => {
    tradingLogger.warn('Order rejected', { requestId: reject.request_id, code: reject.code });
    showToast(reject.message || t('toast.orderRejected'), 'error');
  },
  onOrderCancelled: (cancelled) => {
    tradingStore.removeOrder(cancelled.order_id);
    showToast(t('toast.orderCancelled'), 'info');
  },
  onRateLimited: (retryAfterMs) => {
    tradingLogger.warn(`Rate limited for ${retryAfterMs}ms`);
  },
});

// Handle WebSocket leaderboard_updated messages
watch(lastMessage, (msg) => {
  if (!msg) return;
  try {
    // msg is a WebSocketMessage { type, data, timestamp } — check both wrapper and inner type
    const msgData = (msg as unknown as Record<string, unknown>).data as Record<string, unknown> | undefined;
    const msgType = (msg as unknown as Record<string, unknown>).type as string | undefined;
    const innerType = msgData?.type as string | undefined;

    if (msgType === 'leaderboard_updated' || innerType === 'leaderboard_updated') {
      const payload = msgData || msg;
      const entries = (payload as Record<string, unknown>).top_entries || (payload as Record<string, unknown>).entries || [];
      if (Array.isArray(entries) && entries.length > 0) {
        tradingStore.updateLeaderboard(entries, authStore.user?.id);
        updateLeaderboardFromEntries(entries);
      }
    }
  } catch {
    // Not parseable, ignore
  }
});

watch(contestId, (newId) => {
  disconnect();
  updateContestId(newId);
  connect();
});

// Start leaderboard polling only when WebSocket is disconnected
watch(wsStatus, (status) => {
  if (status !== 'connected') {
    if (!leaderboardPollInterval) {
      leaderboardPollInterval = window.setInterval(fetchLeaderboardData, 30000);
    }
  } else {
    if (leaderboardPollInterval) {
      clearInterval(leaderboardPollInterval);
      leaderboardPollInterval = 0;
    }
  }
});

watch(direction, (dir) => {
  document.documentElement.dir = dir;
}, { immediate: true });


// Toast notifications
const toasts = ref<Array<{ id: number; message: string; type: 'success' | 'error' | 'info' }>>([]);
let toastId = 0;

function showToast(message: string, type: 'success' | 'error' | 'info'): void {
  const id = ++toastId;
  toasts.value.push({ id, message, type });
  setTimeout(() => {
    toasts.value = toasts.value.filter(t => t.id !== id);
  }, 3000);
}

// Event handlers
function handleTabChange(tab: 'trade' | 'leaderboard'): void {
  activeView.value = tab;
}

function handleSymbolSelect(symbol: string): void {
  selectedSymbol.value = symbol;
}

async function handleTrade(side: 'buy' | 'sell', symbol: string, qty: number): Promise<void> {
  if (!symbol || qty <= 0) {
    showToast(t('toast.error'), 'error');
    tradingLogger.warn('Invalid trade parameters', { side, symbol, qty });
    return;
  }

  try {
    const result = await placeOrder({
      symbol,
      side: side.toUpperCase() as OrderSide,
      orderType: 'MARKET' as OrderType,
      qty,
    });
    // Check if order was queued (offline) vs sent
    if (result && 'status' in result && result.status === 'queued') {
      showToast(t('toast.orderQueued'), 'info');
      tradingLogger.info('Order queued while offline', { side, symbol, qty });
    }
    // Success toast for sent orders is handled by onOrderAccepted callback
  } catch (error) {
    if (error instanceof Error && error.message === 'WebSocket not connected') {
      showToast(t('connection.disconnectedMessage'), 'error');
    }
    // OrderReject errors are handled by the onOrderRejected callback
  }
}

function handleUpdateQuantity(qty: number): void {
  quantity.value = qty;
}

function handleToggleFavorite(symbol: string): void {
  const index = favorites.value.indexOf(symbol);
  if (index > -1) {
    favorites.value.splice(index, 1);
  } else {
    favorites.value.push(symbol);
  }
}

function handleOpenAdvancedOrder(symbol: string): void {
  tradingLogger.info('Opening advanced order for', symbol);
  // Open advanced order modal
}

function handleSidebarResize(width: number): void {
  sidebarWidth.value = width;
}

function handleBottomPanelResize(height: number): void {
  bottomPanelHeight.value = height;
}

function handleToggleSidebar(): void {
  sidebarCollapsed.value = !sidebarCollapsed.value;
}

function handleEditPosition(pos: { id: string }): void {
  tradingLogger.info('Editing position', pos.id);
}

async function handleClosePosition(pos: { id: string }): Promise<void> {
  tradingLogger.info('Closing position', pos.id);
  try {
    await tradingStore.closePosition(pos.id);
    showToast(t('toast.positionClosed'), 'success');
  } catch (err) {
    showToast(getErrorMessage(err), 'error');
  }
}

async function handleCancelOrder(order: { id: string }): Promise<void> {
  tradingLogger.info('Cancelling order', order.id);
  try {
    await tradingStore.cancelOrder(order.id);
    showToast(t('toast.orderCancelled'), 'info');
  } catch (err) {
    showToast(getErrorMessage(err), 'error');
  }
}

function handleShowInfo(): void {
  // Show contest info modal
}

function handleMobileBack(): void {
  router.push('/user/tournaments');
}

// Mobile detection
function checkMobile(): void {
  isMobile.value = window.innerWidth < 768;
}

// Timer countdown — updates currentTime each second
let timerInterval: number = 0;

function startTimer(): void {
  timerInterval = window.setInterval(() => {
    currentTime.value = Date.now();
  }, 1000);
}


// Fetch leaderboard data from API
async function fetchLeaderboardData(): Promise<void> {
  if (!contestId.value) return;
  try {
    const response = await getLeaderboard(contestId.value, { limit: 50 });
    const entries = response.entries || [];
    tradingStore.updateLeaderboard(entries, authStore.user?.id);
    updateLeaderboardFromEntries(entries);
  } catch {
    // Leaderboard fetch failed — keep existing data
  }
}

// Map leaderboard entries to display format
function updateLeaderboardFromEntries(entries: Array<{ rank: number; user_id: string; username?: string; total_score: number }>): void {
  const prizes = prizePreview.value?.prizes || [];
  leaderboardParticipants.value = entries.map(entry => {
    const prizeEntry = prizes.find(p => p.rank === entry.rank);
    return {
      userId: entry.user_id,
      username: entry.username || `User ${entry.rank}`,
      pnl: entry.total_score,
      prize: prizeEntry ? prizeEntry.amount_cents / 100 : 0,
    };
  });
}

// Leaderboard polling interval (0 = not running)
let leaderboardPollInterval: number = 0;

// Lifecycle
onMounted(async () => {
  // Non-async setup
  checkMobile();
  window.addEventListener('resize', checkMobile);
  startTimer();
  connect();

  // Guard: need contestId to load data
  if (!contestId.value) return;

  isLoading.value = true;
  loadError.value = null;

  try {
    // Fetch contest details, balance, and prize preview in parallel
    const [contestDetails, , prizeData] = await Promise.all([
      getContestDetails(contestId.value),
      tradingStore.fetchBalance(contestId.value),
      getPrizePreview(contestId.value).catch(() => null),
    ]);

    contestInfo.value = contestDetails;
    prizePreview.value = prizeData;

    // Calculate server time drift for accurate countdown
    if (contestDetails.server_time) {
      const serverMs = new Date(contestDetails.server_time).getTime();
      serverTimeDrift.value = serverMs - Date.now();
    }

    // Build watchlist from contest symbols
    const symbols = contestDetails.symbols || [];
    watchlistSymbols.value = symbols.map(sym => {
      const meta = getSymbolMetadata(sym);
      return {
        symbol: sym,
        base: meta.base,
        quote: meta.quote,
        price: 0,
        bid: 0,
        ask: 0,
        change: 0,
        decimals: meta.decimals,
        spread: 0,
        sessionTime: meta.sessionTime ?? '',
      };
    });

    // Set initial selected symbol and max qty
    if (symbols.length > 0) {
      selectedSymbol.value = symbols[0];
    }
    maxQty.value = contestDetails.available_qty || 100;

    // Fetch initial leaderboard (polling starts only when WS is disconnected)
    await fetchLeaderboardData();
  } catch (err) {
    loadError.value = err instanceof Error ? err.message : 'Failed to load contest data';
    tradingLogger.error('Failed to load contest data', { error: loadError.value });
  } finally {
    isLoading.value = false;
  }
});

onUnmounted(() => {
  // disconnect() is handled by useTradingWebSocket's own onUnmounted hook
  window.removeEventListener('resize', checkMobile);
  clearInterval(timerInterval);
  clearInterval(leaderboardPollInterval);
});

// Provide theme to child components
provide('isDark', computed(() => themeStore.isDark));

// Provide WebSocket state to child components (e.g., LeaderboardPanel, MiniLeaderboard)
provide('wsLastMessage', lastMessage);
provide('wsStatus', wsStatus);
</script>

<template>
  <div class="tp-root" :class="{ 'tp-dark': themeStore.isDark, 'tp-rtl': direction === 'rtl' }">
    <!-- Loading state -->
    <div v-if="isLoading" class="tp-loading">
      <div class="tp-spinner"></div>
      <span>{{ t('common.loading') }}</span>
    </div>

    <!-- Error state -->
    <div v-else-if="loadError" class="tp-error">
      <p>{{ loadError }}</p>
      <button @click="loadError = null; router.go(0)">{{ t('common.retry') }}</button>
    </div>

    <!-- Desktop Layout -->
    <template v-else-if="!isMobile">
      <TradingNavbar
        :contest-id="contestId"
        :start-time="contestInfo ? new Date(contestInfo.start_time) : new Date()"
        :end-time="contestInfo ? new Date(contestInfo.end_time) : new Date()"
        :participant-count="contestInfo?.current_participants ?? 0"
        :max-positions="contestInfo?.available_qty ?? 0"
        :duration-minutes="durationMinutes"
        :remaining-seconds="remainingSeconds"
        :is-free="contestInfo?.is_free ?? false"
        @tab-change="handleTabChange"
        @show-info="handleShowInfo"
      />

      <div class="tp-main">
        <!-- Trading View -->
        <template v-if="activeView === 'trade'">
          <!-- Sidebar -->
          <WatchlistSidebar
            v-if="!sidebarCollapsed"
            :symbols="watchlistSymbols"
            :selected-symbol="selectedSymbol"
            :quantity="quantity"
            :max-qty="maxQty"
            :favorites="favorites"
            @select-symbol="handleSymbolSelect"
            @trade="handleTrade"
            @update-quantity="handleUpdateQuantity"
            @toggle-favorite="handleToggleFavorite"
            @open-advanced-order="handleOpenAdvancedOrder"
            @resize="handleSidebarResize"
          />

          <!-- Center Content -->
          <div class="tp-content">
            <button
              v-if="sidebarCollapsed"
              class="tp-sidebar-toggle"
              @click="handleToggleSidebar"
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="9 18 15 12 9 6" />
              </svg>
            </button>
            <MarketChart
              :symbol="selectedSymbol"
              :ticks="chartTicks"
              :show-position-lines="true"
              :contest-id="contestId"
            />

            <BottomPanel
              :positions="openPositions"
              :pending-orders="pendingOrders"
              :closed-positions="closedPositions"
              :account="account"
              @edit-position="handleEditPosition"
              @close-position="handleClosePosition"
              @cancel-order="handleCancelOrder"
              @resize="handleBottomPanelResize"
            />
          </div>
        </template>

        <!-- Leaderboard View -->
        <template v-else>
          <DesktopLeaderboard
            :participants="leaderboardParticipants"
            :current-user-id="authStore.user?.id || ''"
            :prize-pool="prizePoolDollars"
          />
        </template>
      </div>
    </template>

    <!-- Mobile Layout -->
    <template v-else>
      <MobileHeader
        :contest-id="contestId"
        :duration-minutes="durationMinutes"
        :balance="account.balance"
        :equity="account.equity"
        :pnl="account.unrealizedPnl"
        :rank="currentUserRank"
        :remaining-seconds="remainingSeconds"
        :is-free="contestInfo?.is_free ?? false"
        @show-info="handleShowInfo"
        @back="handleMobileBack"
      />

      <div class="tp-mobile-content">
        <MobileChartPage
          v-if="mobileActiveTab === 'chart'"
          :symbols="watchlistSymbols"
          :selected-symbol="selectedSymbolData"
          :ticks="chartTicks"
          :max-qty="maxQty"
          :contest-id="contestId"
          @select-symbol="handleSymbolSelect"
          @trade="(side: 'buy' | 'sell') => handleTrade(side, selectedSymbol, quantity)"
          @update-quantity="handleUpdateQuantity"
        />

        <MobileOrdersPage
          v-if="mobileActiveTab === 'orders'"
          :open-positions="openPositions"
          :pending-orders="pendingOrders"
          :closed-positions="closedPositions"
          @edit-position="handleEditPosition"
          @close-position="handleClosePosition"
          @cancel-order="handleCancelOrder"
        />

        <MobileLeaderboardPage
          v-if="mobileActiveTab === 'leaderboard'"
          :participants="leaderboardParticipants"
          :current-user-id="authStore.user?.id || ''"
        />

        <MobileDetailsPage
          v-if="mobileActiveTab === 'details'"
          :contest-id="contestId"
          :contest-status="contestInfo?.status ?? 'running'"
          :duration-minutes="durationMinutes"
          :start-time="contestInfo ? new Date(contestInfo.start_time) : new Date()"
          :end-time="contestInfo ? new Date(contestInfo.end_time) : new Date()"
          :remaining-seconds="remainingSeconds"
          :participant-count="contestInfo?.current_participants ?? 0"
          :max-positions="contestInfo?.available_qty ?? 0"
          :starting-balance="tradingStore.totalQTY"
          :leverage="1"
          :prize-pool="prizePoolDollars"
          :prizes="prizeDollars"
          :rules="[]"
          :available-symbols="contestInfo?.symbols ?? []"
          :is-free="contestInfo?.is_free ?? false"
        />
      </div>

      <MobileBottomNav
        :active-tab="mobileActiveTab"
        :open-positions-count="openPositions.length"
        @tab-change="(tab: 'chart' | 'orders' | 'leaderboard' | 'details') => mobileActiveTab = tab"
      />
    </template>

    <!-- Toast Container -->
    <div class="tp-toasts">
      <div
        v-for="toast in toasts"
        :key="toast.id"
        class="tp-toast"
        :class="[`tp-toast-${toast.type}`]"
      >
        <svg v-if="toast.type === 'success'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M5 13l4 4L19 7"/>
        </svg>
        <svg v-else-if="toast.type === 'error'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M6 18L18 6M6 6l12 12"/>
        </svg>
        <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10"/>
          <path d="M12 16v-4M12 8h.01"/>
        </svg>
        <span>{{ toast.message }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tp-root {
  display: flex;
  flex-direction: column;
  height: 100vh;
  overflow: hidden;
  background: var(--tp-bg);
  color: var(--tp-tw);
}

.tp-main {
  flex: 1;
  display: flex;
  overflow: hidden;
}

.tp-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}

.tp-sidebar-toggle {
  position: absolute;
  top: 8px;
  left: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 40px;
  background: var(--color-surface, #1e1e2e);
  border: 1px solid var(--color-border, rgba(255,255,255,0.1));
  border-left: none;
  border-radius: 0 6px 6px 0;
  color: var(--color-text-secondary, rgba(255,255,255,0.6));
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}

.tp-sidebar-toggle:hover {
  background: var(--color-surface-hover, #2a2a3e);
  color: var(--color-text-primary, #fff);
}

.tp-mobile-content {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.tp-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100vh;
  gap: 16px;
  color: var(--tp-tw);
}

.tp-spinner {
  width: 32px;
  height: 32px;
  border: 3px solid rgba(128, 128, 128, 0.3);
  border-top-color: var(--tp-tw);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

.tp-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100vh;
  gap: 16px;
  color: var(--tp-tw);
}

.tp-error button {
  padding: 8px 24px;
  border-radius: 6px;
  border: 1px solid var(--tp-tw);
  background: transparent;
  color: var(--tp-tw);
  cursor: pointer;
  font-size: 14px;
}

.tp-error button:hover {
  background: rgba(128, 128, 128, 0.1);
}

.tp-toasts {
  position: fixed;
  bottom: 80px;
  right: 16px;
  z-index: 1000;
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 320px;
}

@media (min-width: 768px) {
  .tp-toasts {
    bottom: 16px;
  }
}

.tp-toast {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: 10px;
  font-size: 13px;
  font-weight: 500;
  color: #fff;
  animation: slideIn 0.3s ease;
}

.tp-toast svg {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}

.tp-toast-success {
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
}

.tp-toast-error {
  background: linear-gradient(135deg, #ef4444 0%, #dc2626 100%);
}

.tp-toast-info {
  background: linear-gradient(135deg, #06b6d4 0%, #0891b2 100%);
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateX(100%);
  }
  to {
    opacity: 1;
    transform: translateX(0);
  }
}
</style>
