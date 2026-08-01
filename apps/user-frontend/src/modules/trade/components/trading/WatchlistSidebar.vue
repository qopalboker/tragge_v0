<template>
  <div class="tp-sb" :style="{ width: sidebarWidth + 'px', minWidth: sidebarWidth + 'px' }" ref="sidebarRef">
    <!-- Resize handle -->
    <div
      class="tp-sb-resize"
      :class="{ active: isResizing }"
      @mousedown="startResize"
    >
      <div class="tp-sb-resize-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <path d="M18 8l4 4-4 4"/>
          <path d="M6 8l-4 4 4 4"/>
          <path d="M2 12h20"/>
        </svg>
      </div>
      <div class="tp-sb-resize-grip">
        <span></span>
        <span></span>
        <span></span>
        <span></span>
        <span></span>
      </div>
    </div>

    <!-- Tabs -->
    <div class="tp-stabs">
      <button
        class="tp-stab"
        :class="{ active: activeTab === 'all' }"
        @click="activeTab = 'all'"
      >
        <svg viewBox="0 0 24 24" fill="currentColor">
          <path d="M3 13h2v-2H3v2zm0 4h2v-2H3v2zm0-8h2V7H3v2zm4 4h14v-2H7v2zm0 4h14v-2H7v2zM7 7v2h14V7H7z"/>
        </svg>
        <span>{{ t('watchlist.all') }}</span>
      </button>
      <button
        class="tp-stab"
        :class="{ active: activeTab === 'favorites' }"
        @click="activeTab = 'favorites'"
      >
        <svg viewBox="0 0 24 24" fill="currentColor">
          <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
        </svg>
        <span>{{ t('watchlist.favorites') }}</span>
      </button>
      <button
        class="tp-stab"
        :class="{ active: activeTab === 'movers' }"
        @click="activeTab = 'movers'"
      >
        <svg viewBox="0 0 24 24" fill="currentColor">
          <path d="M13.5.67s.74 2.65.74 4.8c0 2.06-1.35 3.73-3.41 3.73-2.07 0-3.63-1.67-3.63-3.73l.03-.36C5.21 7.51 4 10.62 4 14c0 4.42 3.58 8 8 8s8-3.58 8-8C20 8.61 17.41 3.8 13.5.67z"/>
        </svg>
        <span>{{ t('watchlist.topMovers') }}</span>
      </button>
      <button class="tp-ssearch" @click="showSearch = !showSearch">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
          <circle cx="11" cy="11" r="7"/>
          <path d="m21 21-4.3-4.3"/>
        </svg>
      </button>
    </div>

    <!-- Search (if visible) -->
    <div v-if="showSearch" class="tp-wl-search">
      <input
        type="text"
        v-model="searchQuery"
        :placeholder="t('watchlist.searchPlaceholder')"
        class="tp-wl-search-input"
      />
    </div>

    <!-- Column headers -->
    <div class="tp-wlh">
      <span>{{ t('watchlist.symbol') }}</span>
      <span>{{ t('watchlist.price') }}</span>
      <span>{{ t('watchlist.daily') }}</span>
    </div>

    <!-- Watchlist items -->
    <div class="tp-wll scrollbar-thin">
      <template v-for="item in filteredSymbols" :key="item.symbol">
        <div
          class="tp-wli"
          :class="{ selected: item.symbol === selectedSymbol }"
          @click="selectSymbol(item.symbol)"
        >
          <div class="tp-wli-l">
            <div class="tp-wli-f">
              <SymbolFlag :symbol="item.symbol" :base="item.base" :quote="item.quote" />
            </div>
            <div>
              <div class="tp-wli-s">{{ item.symbol }}</div>
              <div class="tp-wli-sub">
                <span class="tp-wli-dot"></span>
                <span>{{ t('watchlist.opens') }}: {{ item.sessionTime }}</span>
              </div>
            </div>
          </div>
          <div class="tp-wli-p" :class="{ 'tp-wli-p-loading': item.price === 0 }">
            {{ item.price > 0 ? formatPrice(item.price, item.decimals) : '—' }}
          </div>
          <div class="tp-wli-c" :class="item.change >= 0 ? 'up' : 'down'">
            <span class="arrow">{{ item.change >= 0 ? '&#9650;' : '&#9660;' }}</span>
            {{ Math.abs(item.change).toFixed(2) }}%
          </div>
        </div>

        <!-- Quick trade panel (only for selected symbol) -->
        <div v-if="item.symbol === selectedSymbol" class="tp-qtp">
          <div class="tp-qtrow">
            <button class="tp-qtsb" @click="placeTrade('sell', item)">
              <span class="tp-qt-l">{{ t('order.sell') }}</span>
              <span class="tp-qt-p">{{ item.bid > 0 ? formatPrice(item.bid, item.decimals) : '—' }}</span>
            </button>

            <div class="tp-qtlot">
              <input
                type="number"
                class="tp-qtlot-i"
                :value="quantity"
                @input="updateQuantity($event)"
                min="1"
                :max="maxQty"
                step="1"
              />
              <div class="tp-qtlot-u">QTY: {{ quantity }} / {{ maxQty }}</div>
              <div class="tp-qtlot-b">
                <button class="tp-qtpm" @click="adjustQty(-1)">−</button>
                <button class="tp-qtpm" @click="adjustQty(1)">+</button>
              </div>
            </div>

            <button class="tp-qtbb" @click="placeTrade('buy', item)">
              <span class="tp-qt-l">{{ t('order.buy') }}</span>
              <span class="tp-qt-p">{{ item.ask > 0 ? formatPrice(item.ask, item.decimals) : '—' }}</span>
            </button>
          </div>

          <div class="tp-qtfooter">
            <button class="tp-qtadv" @click="emit('openAdvancedOrder', item.symbol)">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="3" width="7" height="7"/>
                <rect x="14" y="3" width="7" height="7"/>
                <rect x="14" y="14" width="7" height="7"/>
                <rect x="3" y="14" width="7" height="7"/>
              </svg>
              {{ t('order.advanced') }}
            </button>
            <div class="tp-qtfi">
              <button class="tp-qtfib" :title="t('watchlist.info')">ⓘ</button>
              <button
                class="tp-qtfib"
                :class="{ starred: isFavorite(item.symbol) }"
                @click="toggleFavorite(item.symbol)"
                :title="t('watchlist.addFavorite')"
              >★</button>
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { ref, computed, onUnmounted } from 'vue'
import SymbolFlag from './SymbolFlag.vue'

export interface WatchlistItem {
  symbol: string
  base: string
  quote: string | null
  price: number
  bid: number
  ask: number
  change: number
  decimals: number
  spread: number
  sessionTime: string
  openPrice?: number
}

const props = defineProps<{
  symbols: WatchlistItem[]
  selectedSymbol: string
  quantity: number
  maxQty: number
  favorites: string[]
}>()

const emit = defineEmits<{
  (e: 'selectSymbol', symbol: string): void
  (e: 'trade', side: 'buy' | 'sell', symbol: string, qty: number): void
  (e: 'updateQuantity', qty: number): void
  (e: 'toggleFavorite', symbol: string): void
  (e: 'openAdvancedOrder', symbol: string): void
  (e: 'resize', width: number): void
}>()

const sidebarRef = ref<HTMLElement | null>(null)
const activeTab = ref<'all' | 'favorites' | 'movers'>('all')
const showSearch = ref(false)
const searchQuery = ref('')
const sidebarWidth = ref(392)
const isResizing = ref(false)
const startX = ref(0)
const startWidth = ref(0)

const filteredSymbols = computed(() => {
  let items = [...props.symbols]

  // Filter by tab
  if (activeTab.value === 'all') {
    items = items.sort((a, b) => a.symbol.localeCompare(b.symbol))
  } else if (activeTab.value === 'favorites') {
    items = items.filter(item => props.favorites.includes(item.symbol))
  } else if (activeTab.value === 'movers') {
    items = items.sort((a, b) => Math.abs(b.change) - Math.abs(a.change))
  }

  // Filter by search
  if (searchQuery.value) {
    const query = searchQuery.value.toUpperCase()
    items = items.filter(item => item.symbol.includes(query))
  }

  return items
})

function formatPrice(price: number, decimals: number): string {
  return price.toFixed(decimals)
}

function selectSymbol(symbol: string) {
  emit('selectSymbol', symbol)
}

function placeTrade(side: 'buy' | 'sell', item: WatchlistItem) {
  emit('trade', side, item.symbol, props.quantity)
}

function updateQuantity(event: Event) {
  const target = event.target as HTMLInputElement
  const value = parseInt(target.value) || 1
  emit('updateQuantity', Math.max(1, Math.min(props.maxQty, value)))
}

function adjustQty(delta: number) {
  const newQty = Math.max(1, Math.min(props.maxQty, props.quantity + delta))
  emit('updateQuantity', newQty)
}

function isFavorite(symbol: string): boolean {
  return props.favorites.includes(symbol)
}

function toggleFavorite(symbol: string) {
  emit('toggleFavorite', symbol)
}

// Resize handling
function startResize(e: MouseEvent) {
  isResizing.value = true
  startX.value = e.clientX
  startWidth.value = sidebarWidth.value
  document.body.classList.add('col-resizing')
  document.addEventListener('mousemove', onResize)
  document.addEventListener('mouseup', stopResize)
}

function onResize(e: MouseEvent) {
  if (!isResizing.value) return
  const isRTL = document.documentElement.dir === 'rtl'
  const dx = isRTL ? startX.value - e.clientX : e.clientX - startX.value
  const newWidth = Math.min(600, Math.max(200, startWidth.value + dx))
  sidebarWidth.value = newWidth
  emit('resize', newWidth)
}

function stopResize() {
  isResizing.value = false
  document.body.classList.remove('col-resizing')
  document.removeEventListener('mousemove', onResize)
  document.removeEventListener('mouseup', stopResize)
}

onUnmounted(() => {
  document.removeEventListener('mousemove', onResize)
  document.removeEventListener('mouseup', stopResize)
})
</script>

<style scoped>
.tp-wl-search {
  padding: 8px 14px;
  border-bottom: 1px solid var(--tp-bd);
}

.tp-wl-search-input {
  width: 100%;
  padding: 8px 12px;
  background: var(--tp-bg-inp);
  border: 1px solid var(--tp-bd);
  border-radius: 6px;
  color: var(--tp-tw);
  font-size: 13px;
  outline: none;
}

.tp-wl-search-input:focus {
  border-color: var(--tp-bl);
}

.tp-wl-search-input::placeholder {
  color: var(--tp-t2);
}

.tp-wli-p-loading {
  color: var(--tp-t2);
  animation: pulse 1.5s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}
</style>
