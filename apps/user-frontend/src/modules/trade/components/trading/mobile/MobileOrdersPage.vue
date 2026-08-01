<template>
  <div class="tp-morders">
    <!-- Tabs -->
    <div class="tp-morders-tabs">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        class="tp-morders-tab"
        :class="{ active: activeTab === tab.id }"
        @click="activeTab = tab.id"
      >
        {{ t(tab.label) }}
        <span v-if="tab.count > 0" class="tp-morders-tab-count">{{ tab.count }}</span>
      </button>
    </div>

    <!-- Open positions -->
    <div v-if="activeTab === 'open'" class="tp-morders-list scrollbar-thin">
      <div v-if="openPositions.length === 0" class="tp-morders-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <rect x="3" y="3" width="18" height="18" rx="2"/>
          <path d="M3 9h18M9 21V9"/>
        </svg>
        <span>{{ t('mobile.noOpenPositions') }}</span>
      </div>

      <div v-else v-for="pos in openPositions" :key="pos.id" class="tp-morders-item">
        <div class="tp-morders-item-top">
          <div class="tp-morders-item-sym">
            <SymbolFlag :symbol="pos.symbol" :base="pos.base" :quote="pos.quote" />
            <span class="tp-morders-item-name">{{ pos.symbol }}</span>
            <span class="tp-morders-item-side" :class="pos.side === 'long' ? 'buy' : 'sell'">{{ pos.side === 'long' ? 'BUY' : 'SELL' }}</span>
          </div>
          <div class="tp-morders-item-pnl" :class="pos.pnl >= 0 ? 'up' : 'down'">
            {{ pos.pnl >= 0 ? '+' : '' }}${{ pos.pnl.toFixed(2) }}
          </div>
        </div>

        <div class="tp-morders-item-row">
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.quantity') }}</span>
            <span class="tp-morders-item-v">{{ pos.qty }}</span>
          </div>
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.entry') }}</span>
            <span class="tp-morders-item-v">{{ pos.entryPrice.toFixed(pos.decimals) }}</span>
          </div>
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.current') }}</span>
            <span class="tp-morders-item-v">{{ pos.currentPrice.toFixed(pos.decimals) }}</span>
          </div>
        </div>

        <div class="tp-morders-item-row" v-if="pos.takeProfit || pos.stopLoss">
          <div class="tp-morders-item-col" v-if="pos.takeProfit">
            <span class="tp-morders-item-l">TP</span>
            <span class="tp-morders-item-v up">{{ pos.takeProfit.toFixed(pos.decimals) }}</span>
          </div>
          <div class="tp-morders-item-col" v-if="pos.stopLoss">
            <span class="tp-morders-item-l">SL</span>
            <span class="tp-morders-item-v down">{{ pos.stopLoss.toFixed(pos.decimals) }}</span>
          </div>
        </div>

        <div class="tp-morders-item-actions">
          <button class="tp-morders-btn tp-morders-btn-edit" @click="emit('editPosition', pos)">
            {{ t('order.edit') }}
          </button>
          <button class="tp-morders-btn tp-morders-btn-close" @click="emit('closePosition', pos)">
            {{ t('order.close') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Pending orders -->
    <div v-if="activeTab === 'pending'" class="tp-morders-list scrollbar-thin">
      <div v-if="pendingOrders.length === 0" class="tp-morders-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <circle cx="12" cy="12" r="10"/>
          <polyline points="12 6 12 12 16 14"/>
        </svg>
        <span>{{ t('mobile.noPendingOrders') }}</span>
      </div>

      <div v-else v-for="order in pendingOrders" :key="order.id" class="tp-morders-item">
        <div class="tp-morders-item-top">
          <div class="tp-morders-item-sym">
            <SymbolFlag :symbol="order.symbol" :base="order.base" :quote="order.quote" />
            <span class="tp-morders-item-name">{{ order.symbol }}</span>
            <span class="tp-morders-item-side" :class="order.side">{{ order.side.toUpperCase() }}</span>
            <span class="tp-morders-item-type">{{ order.type }}</span>
          </div>
        </div>

        <div class="tp-morders-item-row">
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.quantity') }}</span>
            <span class="tp-morders-item-v">{{ order.qty }}</span>
          </div>
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.price') }}</span>
            <span class="tp-morders-item-v">{{ (order.limitPrice || order.stopPrice || 0).toFixed(order.decimals) }}</span>
          </div>
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.status') }}</span>
            <span class="tp-morders-item-v">{{ order.status }}</span>
          </div>
        </div>

        <div class="tp-morders-item-actions">
          <button class="tp-morders-btn tp-morders-btn-cancel" @click="emit('cancelOrder', order)">
            {{ t('order.cancel') }}
          </button>
        </div>
      </div>
    </div>

    <!-- History -->
    <div v-if="activeTab === 'history'" class="tp-morders-list scrollbar-thin">
      <div v-if="closedPositions.length === 0" class="tp-morders-empty">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M12 8v4l3 3"/>
          <circle cx="12" cy="12" r="10"/>
        </svg>
        <span>{{ t('mobile.noHistory') }}</span>
      </div>

      <div v-else v-for="pos in closedPositions" :key="pos.id" class="tp-morders-item tp-morders-item-closed">
        <div class="tp-morders-item-top">
          <div class="tp-morders-item-sym">
            <SymbolFlag :symbol="pos.symbol" :base="pos.base" :quote="pos.quote" />
            <span class="tp-morders-item-name">{{ pos.symbol }}</span>
            <span class="tp-morders-item-side" :class="pos.side === 'long' ? 'buy' : 'sell'">{{ pos.side === 'long' ? 'BUY' : 'SELL' }}</span>
          </div>
          <div class="tp-morders-item-pnl" :class="pos.pnl >= 0 ? 'up' : 'down'">
            {{ pos.pnl >= 0 ? '+' : '' }}${{ pos.pnl.toFixed(2) }}
          </div>
        </div>

        <div class="tp-morders-item-row">
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.entry') }}</span>
            <span class="tp-morders-item-v">{{ pos.entryPrice.toFixed(pos.decimals) }}</span>
          </div>
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.exit') }}</span>
            <span class="tp-morders-item-v">{{ pos.exitPrice.toFixed(pos.decimals) }}</span>
          </div>
          <div class="tp-morders-item-col">
            <span class="tp-morders-item-l">{{ t('order.time') }}</span>
            <span class="tp-morders-item-v">{{ formatTime(pos.closedAt) }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { t } from '@/i18n'
import SymbolFlag from '../SymbolFlag.vue'

interface Position {
  id: string
  symbol: string
  base: string
  quote: string | null
  side: 'long' | 'short'
  qty: number
  entryPrice: number
  currentPrice: number
  pnl: number
  takeProfit?: number
  stopLoss?: number
  decimals: number
}

interface ClosedPosition extends Omit<Position, 'currentPrice' | 'takeProfit' | 'stopLoss'> {
  exitPrice: number
  closedAt: Date
}

interface Order {
  id: string
  symbol: string
  base: string
  quote: string | null
  side: 'buy' | 'sell'
  type: string
  qty: number
  limitPrice?: number
  stopPrice?: number
  status: string
  decimals: number
}

const props = defineProps<{
  openPositions: Position[]
  pendingOrders: Order[]
  closedPositions: ClosedPosition[]
}>()

const emit = defineEmits<{
  (e: 'editPosition', pos: Position): void
  (e: 'closePosition', pos: Position): void
  (e: 'cancelOrder', order: Order): void
}>()

const activeTab = ref<'open' | 'pending' | 'history'>('open')

const tabs = computed(() => [
  { id: 'open' as const, label: 'order.openPositions', count: props.openPositions.length },
  { id: 'pending' as const, label: 'order.pendingOrders', count: props.pendingOrders.length },
  { id: 'history' as const, label: 'order.history', count: 0 }
])

function formatTime(date: Date): string {
  return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`
}
</script>

<style scoped>
.tp-morders {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--tp-bg);
}

.tp-morders-tabs {
  display: flex;
  border-bottom: 1px solid var(--tp-bd);
}

.tp-morders-tab {
  flex: 1;
  padding: 14px;
  border: none;
  background: none;
  color: var(--tp-t2);
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
}

.tp-morders-tab.active {
  color: var(--tp-bl);
}

.tp-morders-tab.active::after {
  content: '';
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  height: 2px;
  background: var(--tp-bl);
}

.tp-morders-tab-count {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  background: var(--tp-bg-3);
  border-radius: 9px;
  font-size: 11px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.tp-morders-tab.active .tp-morders-tab-count {
  background: var(--tp-bl);
  color: #fff;
}

.tp-morders-list {
  flex: 1;
  overflow-y: auto;
  padding: 12px 16px;
}

.tp-morders-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  color: var(--tp-t2);
  text-align: center;
}

.tp-morders-empty svg {
  width: 48px;
  height: 48px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.tp-morders-item {
  background: var(--tp-bg-2);
  border-radius: 12px;
  padding: 14px;
  margin-bottom: 12px;
}

.tp-morders-item-closed {
  opacity: 0.8;
}

.tp-morders-item-top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.tp-morders-item-sym {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tp-morders-item-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-morders-item-side {
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
}

.tp-morders-item-side.buy {
  background: rgba(16, 185, 129, 0.2);
  color: var(--tp-gn);
}

.tp-morders-item-side.sell {
  background: rgba(239, 68, 68, 0.2);
  color: var(--tp-rd);
}

.tp-morders-item-type {
  padding: 2px 6px;
  background: var(--tp-bg-3);
  border-radius: 4px;
  font-size: 10px;
  font-weight: 500;
  color: var(--tp-t2);
  text-transform: uppercase;
}

.tp-morders-item-pnl {
  font-size: 16px;
  font-weight: 600;
}

.tp-morders-item-pnl.up {
  color: var(--tp-gn);
}

.tp-morders-item-pnl.down {
  color: var(--tp-rd);
}

.tp-morders-item-row {
  display: flex;
  gap: 16px;
  margin-bottom: 10px;
}

.tp-morders-item-col {
  flex: 1;
}

.tp-morders-item-l {
  display: block;
  font-size: 11px;
  color: var(--tp-t2);
  margin-bottom: 2px;
}

.tp-morders-item-v {
  font-size: 13px;
  font-weight: 500;
  color: var(--tp-tw);
}

.tp-morders-item-v.up {
  color: var(--tp-gn);
}

.tp-morders-item-v.down {
  color: var(--tp-rd);
}

.tp-morders-item-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--tp-bd);
}

.tp-morders-btn {
  flex: 1;
  padding: 10px;
  border: none;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}

.tp-morders-btn-edit {
  background: var(--tp-bg-3);
  color: var(--tp-tw);
}

.tp-morders-btn-close {
  background: var(--tp-rd);
  color: #fff;
}

.tp-morders-btn-cancel {
  background: var(--tp-bg-3);
  color: var(--tp-rd);
}
</style>
