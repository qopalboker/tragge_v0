<template>
  <div class="tp-bpanel" :style="{ height: panelHeight + 'px' }" ref="panelRef">
    <!-- Tabs -->
    <div class="tp-bptabs">
      <button
        class="tp-bpt"
        :class="{ active: activeTab === 'positions' }"
        @click="activeTab = 'positions'"
      >
        <span class="tp-bpt-ic">
          <span class="bk"></span>
          <span class="bk"></span>
        </span>
        <span>{{ t('positions.open') }}</span>
        <span v-if="positions.length" class="tp-bpt-count">({{ positions.length }})</span>
      </button>

      <button
        class="tp-bpt"
        :class="{ active: activeTab === 'pending' }"
        @click="activeTab = 'pending'"
      >
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2"/>
        </svg>
        <span>{{ t('orders.pending') }}</span>
        <span v-if="pendingOrders.length" class="tp-bpt-count">({{ pendingOrders.length }})</span>
      </button>

      <button
        class="tp-bpt"
        :class="{ active: activeTab === 'closed' }"
        @click="activeTab = 'closed'"
      >
        <span class="tc"></span>
        <span>{{ t('positions.closed') }}</span>
      </button>

      <button
        class="tp-bpt"
        :class="{ active: activeTab === 'finance' }"
        @click="activeTab = 'finance'"
      >
        <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M12 1v22M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6"/>
        </svg>
        <span>{{ t('account.finance') }}</span>
      </button>

      <div class="tp-bptr">
        <button class="tp-bpib" :title="t('common.filter')">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3"/>
          </svg>
        </button>
        <button class="tp-bpib" :title="t('common.export')">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3"/>
          </svg>
        </button>
      </div>
    </div>

    <!-- Content -->
    <div class="tp-bpcontent scrollbar-thin">
      <!-- Open Positions -->
      <template v-if="activeTab === 'positions'">
        <template v-if="positions.length">
          <div class="tp-bp-table">
            <div class="tp-bp-thead">
              <div class="tp-bp-th">{{ t('positions.symbol') }}</div>
              <div class="tp-bp-th">{{ t('positions.side') }}</div>
              <div class="tp-bp-th">{{ t('positions.qty') }}</div>
              <div class="tp-bp-th">{{ t('positions.entryPrice') }}</div>
              <div class="tp-bp-th">{{ t('positions.currentPrice') }}</div>
              <div class="tp-bp-th">{{ t('positions.pnl') }}</div>
              <div class="tp-bp-th">{{ t('positions.tpsl') }}</div>
              <div class="tp-bp-th"></div>
            </div>
            <div
              v-for="pos in positions"
              :key="pos.id"
              class="tp-bp-row"
            >
              <div class="tp-bp-td tp-bp-symbol">{{ pos.symbol }}</div>
              <div class="tp-bp-td" :class="pos.side === 'long' ? 'tp-bp-long' : 'tp-bp-short'">
                {{ pos.side === 'long' ? 'BUY' : 'SELL' }}
              </div>
              <div class="tp-bp-td">{{ pos.qty }}</div>
              <div class="tp-bp-td">{{ formatPrice(pos.entryPrice, pos.decimals) }}</div>
              <div class="tp-bp-td">{{ formatPrice(pos.currentPrice, pos.decimals) }}</div>
              <div class="tp-bp-td" :class="pos.pnl >= 0 ? 'tp-bp-profit' : 'tp-bp-loss'">
                {{ pos.pnl >= 0 ? '+' : '' }}{{ formatMoney(pos.pnl) }}
              </div>
              <div class="tp-bp-td tp-bp-tpsl">
                <span v-if="pos.takeProfit">TP: {{ formatPrice(pos.takeProfit, pos.decimals) }}</span>
                <span v-if="pos.stopLoss">SL: {{ formatPrice(pos.stopLoss, pos.decimals) }}</span>
              </div>
              <div class="tp-bp-td tp-bp-actions">
                <button class="tp-bp-edit" @click="emit('editPosition', pos)" :title="t('positions.editTpsl')">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/>
                    <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/>
                  </svg>
                </button>
                <button class="tp-bp-close" @click="emit('closePosition', pos)" :title="t('positions.close')">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="18" y1="6" x2="6" y2="18"/>
                    <line x1="6" y1="6" x2="18" y2="18"/>
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </template>
        <template v-else>
          <div class="tp-bp-empty">
            <span class="tp-bp-gb">{{ t('positions.noPositions') }}</span>
            <p class="tp-bp-et">{{ t('positions.emptyMessage') }}</p>
            <p class="tp-bp-es">{{ t('positions.emptyHint') }}</p>
          </div>
        </template>
      </template>

      <!-- Pending Orders -->
      <template v-if="activeTab === 'pending'">
        <template v-if="pendingOrders.length">
          <div class="tp-bp-table">
            <div class="tp-bp-thead">
              <div class="tp-bp-th">{{ t('orders.symbol') }}</div>
              <div class="tp-bp-th">{{ t('orders.type') }}</div>
              <div class="tp-bp-th">{{ t('orders.side') }}</div>
              <div class="tp-bp-th">{{ t('orders.qty') }}</div>
              <div class="tp-bp-th">{{ t('orders.price') }}</div>
              <div class="tp-bp-th">{{ t('orders.status') }}</div>
              <div class="tp-bp-th"></div>
            </div>
            <div
              v-for="order in pendingOrders"
              :key="order.id"
              class="tp-bp-row"
            >
              <div class="tp-bp-td tp-bp-symbol">{{ order.symbol }}</div>
              <div class="tp-bp-td">{{ order.type }}</div>
              <div class="tp-bp-td" :class="order.side === 'buy' ? 'tp-bp-long' : 'tp-bp-short'">
                {{ order.side.toUpperCase() }}
              </div>
              <div class="tp-bp-td">{{ order.qty }}</div>
              <div class="tp-bp-td">{{ formatPrice(order.limitPrice || order.stopPrice || 0, order.decimals) }}</div>
              <div class="tp-bp-td tp-bp-status">{{ order.status }}</div>
              <div class="tp-bp-td tp-bp-actions">
                <button class="tp-bp-close" @click="emit('cancelOrder', order)" :title="t('orders.cancel')">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <line x1="18" y1="6" x2="6" y2="18"/>
                    <line x1="6" y1="6" x2="18" y2="18"/>
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </template>
        <template v-else>
          <div class="tp-bp-empty">
            <span class="tp-bp-gb">{{ t('orders.noPending') }}</span>
            <p class="tp-bp-et">{{ t('orders.emptyMessage') }}</p>
          </div>
        </template>
      </template>

      <!-- Closed Positions -->
      <template v-if="activeTab === 'closed'">
        <template v-if="closedPositions.length">
          <div class="tp-bp-table">
            <div class="tp-bp-thead">
              <div class="tp-bp-th">{{ t('positions.symbol') }}</div>
              <div class="tp-bp-th">{{ t('positions.side') }}</div>
              <div class="tp-bp-th">{{ t('positions.qty') }}</div>
              <div class="tp-bp-th">{{ t('positions.entryPrice') }}</div>
              <div class="tp-bp-th">{{ t('positions.exitPrice') }}</div>
              <div class="tp-bp-th">{{ t('positions.pnl') }}</div>
              <div class="tp-bp-th">{{ t('positions.closedAt') }}</div>
            </div>
            <div
              v-for="pos in closedPositions"
              :key="pos.id"
              class="tp-bp-row"
            >
              <div class="tp-bp-td tp-bp-symbol">{{ pos.symbol }}</div>
              <div class="tp-bp-td" :class="pos.side === 'long' ? 'tp-bp-long' : 'tp-bp-short'">
                {{ pos.side === 'long' ? 'BUY' : 'SELL' }}
              </div>
              <div class="tp-bp-td">{{ pos.qty }}</div>
              <div class="tp-bp-td">{{ formatPrice(pos.entryPrice, pos.decimals) }}</div>
              <div class="tp-bp-td">{{ formatPrice(pos.exitPrice, pos.decimals) }}</div>
              <div class="tp-bp-td" :class="pos.pnl >= 0 ? 'tp-bp-profit' : 'tp-bp-loss'">
                {{ pos.pnl >= 0 ? '+' : '' }}{{ formatMoney(pos.pnl) }}
              </div>
              <div class="tp-bp-td">{{ formatDateTime(pos.closedAt) }}</div>
            </div>
          </div>
        </template>
        <template v-else>
          <div class="tp-bp-empty">
            <p class="tp-bp-et">{{ t('positions.noClosedPositions') }}</p>
          </div>
        </template>
      </template>

      <!-- Finance -->
      <template v-if="activeTab === 'finance'">
        <div class="tp-bp-finance">
          <div class="tp-bp-fin-row">
            <span class="tp-bp-fin-label">{{ t('account.balance') }}</span>
            <span class="tp-bp-fin-value">{{ formatMoney(account.balance) }}</span>
          </div>
          <div class="tp-bp-fin-row">
            <span class="tp-bp-fin-label">{{ t('account.equity') }}</span>
            <span class="tp-bp-fin-value">{{ formatMoney(account.equity) }}</span>
          </div>
          <div class="tp-bp-fin-row">
            <span class="tp-bp-fin-label">{{ t('account.margin') }}</span>
            <span class="tp-bp-fin-value">{{ formatMoney(account.margin) }}</span>
          </div>
          <div class="tp-bp-fin-row">
            <span class="tp-bp-fin-label">{{ t('account.freeMargin') }}</span>
            <span class="tp-bp-fin-value">{{ formatMoney(account.freeMargin) }}</span>
          </div>
          <div class="tp-bp-fin-row">
            <span class="tp-bp-fin-label">{{ t('account.unrealizedPnl') }}</span>
            <span class="tp-bp-fin-value" :class="account.unrealizedPnl >= 0 ? 'tp-bp-profit' : 'tp-bp-loss'">
              {{ account.unrealizedPnl >= 0 ? '+' : '' }}{{ formatMoney(account.unrealizedPnl) }}
            </span>
          </div>
          <div class="tp-bp-fin-row">
            <span class="tp-bp-fin-label">{{ t('account.realizedPnl') }}</span>
            <span class="tp-bp-fin-value" :class="account.realizedPnl >= 0 ? 'tp-bp-profit' : 'tp-bp-loss'">
              {{ account.realizedPnl >= 0 ? '+' : '' }}{{ formatMoney(account.realizedPnl) }}
            </span>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { t } from '@/i18n'

export interface Position {
  id: string
  symbol: string
  base?: string
  quote?: string | null
  side: 'long' | 'short'
  qty: number
  entryPrice: number
  currentPrice: number
  pnl: number
  takeProfit?: number
  stopLoss?: number
  decimals: number
}

export interface ClosedPosition extends Omit<Position, 'currentPrice' | 'takeProfit' | 'stopLoss'> {
  exitPrice: number
  closedAt: Date
}

export interface Order {
  id: string
  symbol: string
  base?: string
  quote?: string | null
  type: string
  side: 'buy' | 'sell'
  qty: number
  limitPrice?: number
  stopPrice?: number
  status: string
  decimals: number
}

export interface Account {
  balance: number
  equity: number
  margin: number
  freeMargin: number
  unrealizedPnl: number
  realizedPnl: number
}

withDefaults(defineProps<{
  positions: Position[]
  pendingOrders: Order[]
  closedPositions: ClosedPosition[]
  account: Account
  panelHeight?: number
}>(), {
  panelHeight: 200
})

const emit = defineEmits<{
  (e: 'editPosition', position: Position): void
  (e: 'closePosition', position: Position): void
  (e: 'cancelOrder', order: Order): void
  (e: 'resize', height: number): void
}>()

const panelRef = ref<HTMLElement | null>(null)
const activeTab = ref<'positions' | 'pending' | 'closed' | 'finance'>('positions')

function formatPrice(price: number, decimals: number): string {
  return price.toFixed(decimals)
}

function formatMoney(amount: number): string {
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount)
}

function formatDateTime(date: Date): string {
  return new Intl.DateTimeFormat('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}
</script>

<style scoped>
.tp-bp-table {
  width: 100%;
  font-size: 12px;
}

.tp-bp-thead {
  display: grid;
  grid-template-columns: 1fr 80px 60px 100px 100px 100px 120px 80px;
  padding: 8px 16px;
  font-size: 11px;
  font-weight: 600;
  color: var(--tp-tm);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  border-bottom: 1px solid var(--tp-bd);
  background: var(--tp-bg-pnl);
}

.tp-bp-row {
  display: grid;
  grid-template-columns: 1fr 80px 60px 100px 100px 100px 120px 80px;
  padding: 10px 16px;
  align-items: center;
  border-bottom: 1px solid var(--tp-bd);
  transition: background 0.1s;
}

.tp-bp-row:hover {
  background: var(--tp-bg-hov);
}

.tp-bp-td {
  font-family: var(--font-family-mono);
}

.tp-bp-symbol {
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-bp-long {
  color: var(--tp-g);
  font-weight: 600;
}

.tp-bp-short {
  color: var(--tp-r);
  font-weight: 600;
}

.tp-bp-profit {
  color: var(--tp-g);
}

.tp-bp-loss {
  color: var(--tp-r);
}

.tp-bp-tpsl {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 10px;
  color: var(--tp-t2);
}

.tp-bp-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.tp-bp-edit,
.tp-bp-close {
  width: 28px;
  height: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: 1px solid var(--tp-bd);
  border-radius: 4px;
  color: var(--tp-t2);
  cursor: pointer;
  transition: all 0.1s;
}

.tp-bp-edit:hover {
  background: var(--tp-bg-hov);
  color: var(--tp-bl);
  border-color: var(--tp-bl);
}

.tp-bp-close:hover {
  background: var(--tp-rd);
  color: var(--tp-r);
  border-color: var(--tp-r);
}

.tp-bp-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 12px;
}

.tp-bp-count {
  font-size: 11px;
  color: var(--tp-bl);
  margin-left: 4px;
}

.tp-bp-status {
  text-transform: capitalize;
  color: var(--tp-or);
}

.tp-bp-finance {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 20px;
  max-width: 400px;
  margin: 0 auto;
}

.tp-bp-fin-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid var(--tp-bd);
}

.tp-bp-fin-label {
  font-size: 13px;
  color: var(--tp-t2);
}

.tp-bp-fin-value {
  font-size: 14px;
  font-weight: 600;
  font-family: var(--font-family-mono);
  color: var(--tp-tw);
}
</style>
