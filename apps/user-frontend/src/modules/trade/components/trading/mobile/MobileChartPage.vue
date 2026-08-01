<template>
  <div class="tp-mchart">
    <!-- Symbol selector -->
    <div class="tp-mchart-sym" @click="showSymbolPicker = true">
      <SymbolFlag :symbol="selectedSymbol.symbol" :base="selectedSymbol.base" :quote="selectedSymbol.quote" />
      <div class="tp-mchart-sym-info">
        <span class="tp-mchart-sym-name">{{ selectedSymbol.symbol }}</span>
        <span class="tp-mchart-sym-price" :class="selectedSymbol.price > 0 ? (selectedSymbol.change >= 0 ? 'up' : 'down') : ''">
          {{ selectedSymbol.price > 0 ? formatPrice(selectedSymbol.price, selectedSymbol.decimals) : '—' }}
          <span class="tp-mchart-sym-chg">
            {{ selectedSymbol.change >= 0 ? '+' : '' }}{{ selectedSymbol.change.toFixed(2) }}%
          </span>
        </span>
      </div>
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="6 9 12 15 18 9"/>
      </svg>
    </div>

    <!-- Chart area using MarketChart -->
    <div class="tp-mchart-area">
      <MarketChart
        :symbol="selectedSymbol.symbol"
        :ticks="ticks"
        :show-position-lines="true"
        :contest-id="contestId"
      />
    </div>

    <!-- Quick trade buttons -->
    <div class="tp-mchart-trade">
      <button class="tp-mchart-sell" @click="emit('trade', 'sell')">
        <span class="tp-mchart-trade-l">{{ t('order.sell') }}</span>
        <span class="tp-mchart-trade-p">{{ formatPrice(selectedSymbol.bid, selectedSymbol.decimals) }}</span>
      </button>

      <div class="tp-mchart-qty">
        <button class="tp-mchart-qty-btn" @click="adjustQty(-1)">−</button>
        <input
          v-model.number="quantity"
          type="number"
          class="tp-mchart-qty-inp"
          min="1"
          :max="maxQty"
        />
        <button class="tp-mchart-qty-btn" @click="adjustQty(1)">+</button>
      </div>

      <button class="tp-mchart-buy" @click="emit('trade', 'buy')">
        <span class="tp-mchart-trade-l">{{ t('order.buy') }}</span>
        <span class="tp-mchart-trade-p">{{ formatPrice(selectedSymbol.ask, selectedSymbol.decimals) }}</span>
      </button>
    </div>

    <!-- Symbol picker modal -->
    <Teleport to="body">
      <div v-if="showSymbolPicker" class="tp-mpicker-overlay" @click="showSymbolPicker = false">
        <div class="tp-mpicker" @click.stop>
          <div class="tp-mpicker-hdr">
            <h3>{{ t('mobile.selectSymbol') }}</h3>
            <button class="tp-mpicker-close" @click="showSymbolPicker = false">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M18 6L6 18M6 6l12 12"/>
              </svg>
            </button>
          </div>

          <div class="tp-mpicker-search">
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('watchlist.searchPlaceholder')"
            />
          </div>

          <div class="tp-mpicker-list scrollbar-thin">
            <div
              v-for="item in filteredSymbols"
              :key="item.symbol"
              class="tp-mpicker-item"
              :class="{ selected: item.symbol === selectedSymbol.symbol }"
              @click="selectSymbol(item)"
            >
              <SymbolFlag :symbol="item.symbol" :base="item.base" :quote="item.quote" />
              <div class="tp-mpicker-item-info">
                <span class="tp-mpicker-item-sym">{{ item.symbol }}</span>
                <span class="tp-mpicker-item-price">
                  {{ item.price > 0 ? formatPrice(item.price, item.decimals) : '—' }}
                </span>
              </div>
              <span class="tp-mpicker-item-chg" :class="item.change >= 0 ? 'up' : 'down'">
                {{ item.change >= 0 ? '+' : '' }}{{ item.change.toFixed(2) }}%
              </span>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { ref, computed } from 'vue'
import SymbolFlag from '../SymbolFlag.vue'
import MarketChart from '@/components/MarketChart.vue'
import type { WatchlistItem } from '../WatchlistSidebar.vue'
import type { TickData } from '@/composables/useChartData'

const props = defineProps<{
  symbols: WatchlistItem[]
  selectedSymbol: WatchlistItem
  ticks: TickData[]
  maxQty: number
  contestId: string
}>()

const emit = defineEmits<{
  (e: 'selectSymbol', symbol: string): void
  (e: 'trade', side: 'buy' | 'sell'): void
  (e: 'updateQuantity', qty: number): void
}>()

const showSymbolPicker = ref(false)
const searchQuery = ref('')
const quantity = ref(1)

const filteredSymbols = computed(() => {
  if (!searchQuery.value) return props.symbols
  const query = searchQuery.value.toUpperCase()
  return props.symbols.filter(s => s.symbol.includes(query))
})

function formatPrice(price: number, decimals: number): string {
  return price.toFixed(decimals)
}

function selectSymbol(item: WatchlistItem) {
  emit('selectSymbol', item.symbol)
  showSymbolPicker.value = false
}

function adjustQty(delta: number) {
  const newQty = Math.max(1, Math.min(props.maxQty, quantity.value + delta))
  quantity.value = newQty
  emit('updateQuantity', newQty)
}
</script>

<style scoped>
.tp-mchart {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--tp-bg);
}

.tp-mchart-sym {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--tp-bd);
  cursor: pointer;
}

.tp-mchart-sym svg {
  width: 20px;
  height: 20px;
  color: var(--tp-t2);
  margin-left: auto;
}

.tp-mchart-sym-info {
  display: flex;
  flex-direction: column;
}

.tp-mchart-sym-name {
  font-size: 16px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-mchart-sym-price {
  font-size: 14px;
  font-weight: 500;
}

.tp-mchart-sym-price.up {
  color: var(--tp-gn);
}

.tp-mchart-sym-price.down {
  color: var(--tp-rd);
}

.tp-mchart-sym-chg {
  margin-left: 8px;
  font-size: 12px;
}

.tp-mchart-area {
  flex: 1;
  position: relative;
  min-height: 200px;
  /* Chart must render LTR regardless of app direction */
  direction: ltr;
}

.tp-mchart-trade {
  display: flex;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid var(--tp-bd);
  background: var(--tp-bg);
}

.tp-mchart-sell,
.tp-mchart-buy {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 12px;
  border: none;
  border-radius: 8px;
  font-weight: 600;
  cursor: pointer;
}

.tp-mchart-sell {
  background: var(--tp-rd);
  color: #fff;
}

.tp-mchart-buy {
  background: var(--tp-gn);
  color: #fff;
}

.tp-mchart-trade-l {
  font-size: 12px;
  text-transform: uppercase;
}

.tp-mchart-trade-p {
  font-size: 16px;
}

.tp-mchart-qty {
  display: flex;
  align-items: center;
  gap: 4px;
}

.tp-mchart-qty-btn {
  width: 32px;
  height: 32px;
  border: 1px solid var(--tp-bd);
  background: var(--tp-bg-2);
  color: var(--tp-tw);
  font-size: 18px;
  border-radius: 6px;
  cursor: pointer;
}

.tp-mchart-qty-inp {
  width: 48px;
  padding: 8px;
  text-align: center;
  border: 1px solid var(--tp-bd);
  background: var(--tp-bg-inp);
  color: var(--tp-tw);
  font-size: 14px;
  font-weight: 600;
  border-radius: 6px;
}

/* Symbol picker modal */
.tp-mpicker-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 1000;
  display: flex;
  align-items: flex-end;
}

.tp-mpicker {
  width: 100%;
  max-height: 80vh;
  background: var(--tp-bg);
  border-radius: 16px 16px 0 0;
  display: flex;
  flex-direction: column;
}

.tp-mpicker-hdr {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px;
  border-bottom: 1px solid var(--tp-bd);
}

.tp-mpicker-hdr h3 {
  font-size: 18px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-mpicker-close {
  width: 32px;
  height: 32px;
  border: none;
  background: var(--tp-bg-2);
  border-radius: 8px;
  color: var(--tp-tw);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
}

.tp-mpicker-close svg {
  width: 20px;
  height: 20px;
}

.tp-mpicker-search {
  padding: 12px 16px;
}

.tp-mpicker-search input {
  width: 100%;
  padding: 12px;
  border: 1px solid var(--tp-bd);
  background: var(--tp-bg-inp);
  color: var(--tp-tw);
  font-size: 14px;
  border-radius: 8px;
}

.tp-mpicker-list {
  flex: 1;
  overflow-y: auto;
  padding-bottom: env(safe-area-inset-bottom, 16px);
}

.tp-mpicker-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
  border-bottom: 1px solid var(--tp-bd);
  cursor: pointer;
}

.tp-mpicker-item.selected {
  background: var(--tp-bg-h);
}

.tp-mpicker-item-info {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.tp-mpicker-item-sym {
  font-size: 14px;
  font-weight: 600;
  color: var(--tp-tw);
}

.tp-mpicker-item-price {
  font-size: 12px;
  color: var(--tp-t2);
}

.tp-mpicker-item-chg {
  font-size: 13px;
  font-weight: 500;
}

.tp-mpicker-item-chg.up {
  color: var(--tp-gn);
}

.tp-mpicker-item-chg.down {
  color: var(--tp-rd);
}
</style>
