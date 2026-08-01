<script setup lang="ts">
import { ref, computed, watch } from 'vue';
import { t } from '@/i18n';
import type { DurationType, MarketType, ContestFilters } from '@/stores/contests';

const props = defineProps<{
  modelValue: ContestFilters;
}>();

const emit = defineEmits<{
  'update:modelValue': [filters: ContestFilters];
  'filter': [filters: ContestFilters];
}>();

// Market type options with icons
const marketTypes: Array<{ value: MarketType; icon: string }> = [
  { value: 'crypto', icon: '\u20BF' },
  { value: 'forex', icon: '\uD83D\uDCB1' },
  { value: 'stocks', icon: '\uD83D\uDCC8' },
  { value: 'mixed', icon: '\uD83C\uDFAF' },
];

// Duration type options with icons
const durationTypes: Array<{ value: DurationType; icon: string }> = [
  { value: 'rush_30min', icon: '\u26A1' },
  { value: 'hourly', icon: '\u23F1\uFE0F' },
  { value: 'four_hour', icon: '\uD83D\uDD53' },
  { value: 'daily', icon: '\uD83D\uDCC5' },
  { value: 'weekly', icon: '\uD83D\uDCC6' },
];

// Local state
const selectedMarket = ref<MarketType | undefined>(props.modelValue.market_type);
const selectedDuration = ref<DurationType | undefined>(props.modelValue.duration_type);
const freeOnly = ref(props.modelValue.is_free ?? false);
const entryFeeRange = ref<[number, number]>([
  props.modelValue.min_entry ?? 0,
  props.modelValue.max_entry ?? 10000,
]);
const showEntryFeeSlider = ref(false);

// Watch for external changes
watch(() => props.modelValue, (newVal) => {
  selectedMarket.value = newVal.market_type;
  selectedDuration.value = newVal.duration_type;
  freeOnly.value = newVal.is_free ?? false;
  if (newVal.min_entry !== undefined || newVal.max_entry !== undefined) {
    entryFeeRange.value = [newVal.min_entry ?? 0, newVal.max_entry ?? 10000];
  }
}, { deep: true });

// Computed active filter count
const activeFilterCount = computed(() => {
  let count = 0;
  if (selectedMarket.value) count++;
  if (selectedDuration.value) count++;
  if (freeOnly.value) count++;
  if (entryFeeRange.value[0] > 0 || entryFeeRange.value[1] < 10000) count++;
  return count;
});

const hasActiveFilters = computed(() => activeFilterCount.value > 0);

// Market label getter
function getMarketLabel(type: MarketType): string {
  return t(`filters.market.${type}`);
}

// Duration label getter
function getDurationLabel(type: DurationType): string {
  return t(`filters.duration.${type}`);
}

// Toggle market type (single select)
function toggleMarket(type: MarketType): void {
  if (selectedMarket.value === type) {
    selectedMarket.value = undefined;
  } else {
    selectedMarket.value = type;
  }
  emitFilters();
}

// Toggle duration type (single select)
function toggleDuration(type: DurationType): void {
  if (selectedDuration.value === type) {
    selectedDuration.value = undefined;
  } else {
    selectedDuration.value = type;
  }
  emitFilters();
}

// Toggle free only
function toggleFreeOnly(): void {
  freeOnly.value = !freeOnly.value;
  emitFilters();
}

// Update entry fee range
function updateEntryFeeRange(): void {
  emitFilters();
}

// Clear all filters
function clearFilters(): void {
  selectedMarket.value = undefined;
  selectedDuration.value = undefined;
  freeOnly.value = false;
  entryFeeRange.value = [0, 10000];
  showEntryFeeSlider.value = false;
  emitFilters();
}

// Emit filter changes
function emitFilters(): void {
  const filters: ContestFilters = {};

  if (selectedMarket.value) {
    filters.market_type = selectedMarket.value;
  }
  if (selectedDuration.value) {
    filters.duration_type = selectedDuration.value;
  }
  if (freeOnly.value) {
    filters.is_free = true;
  }
  if (entryFeeRange.value[0] > 0) {
    filters.min_entry = entryFeeRange.value[0];
  }
  if (entryFeeRange.value[1] < 10000) {
    filters.max_entry = entryFeeRange.value[1];
  }

  emit('update:modelValue', filters);
  emit('filter', filters);
}

// Format currency
function formatCurrency(cents: number): string {
  return `$${(cents / 100).toFixed(0)}`;
}
</script>

<template>
  <div class="contest-filters">
    <!-- Filter Header with Badge -->
    <div class="filters-header">
      <span class="filters-label">
        {{ t('filters.title') }}
        <span v-if="hasActiveFilters" class="filter-badge">{{ activeFilterCount }}</span>
      </span>
      <button
        v-if="hasActiveFilters"
        class="clear-btn"
        @click="clearFilters"
      >
        {{ t('filters.clearAll') }}
      </button>
    </div>

    <!-- Market Type Chips -->
    <div class="filter-section">
      <span class="section-label">{{ t('filters.market.label') }}</span>
      <div class="chips-container">
        <button
          v-for="mt in marketTypes"
          :key="mt.value"
          :class="['chip', { 'chip-active': selectedMarket === mt.value }]"
          @click="toggleMarket(mt.value)"
        >
          <span class="chip-icon">{{ mt.icon }}</span>
          <span class="chip-label">{{ getMarketLabel(mt.value) }}</span>
        </button>
      </div>
    </div>

    <!-- Duration Type Chips -->
    <div class="filter-section">
      <span class="section-label">{{ t('filters.duration.label') }}</span>
      <div class="chips-container">
        <button
          v-for="dt in durationTypes"
          :key="dt.value"
          :class="['chip', { 'chip-active': selectedDuration === dt.value }]"
          @click="toggleDuration(dt.value)"
        >
          <span class="chip-icon">{{ dt.icon }}</span>
          <span class="chip-label">{{ getDurationLabel(dt.value) }}</span>
        </button>
      </div>
    </div>

    <!-- Free Only Toggle -->
    <div class="filter-section">
      <label class="toggle-container">
        <input
          type="checkbox"
          :checked="freeOnly"
          @change="toggleFreeOnly"
        />
        <span class="toggle-slider"></span>
        <span class="toggle-label">{{ t('filters.freeOnly') }}</span>
      </label>
    </div>

    <!-- Entry Fee Range (Collapsible) -->
    <div class="filter-section">
      <button
        class="section-toggle"
        @click="showEntryFeeSlider = !showEntryFeeSlider"
      >
        <span>{{ t('filters.entryFee') }}</span>
        <svg
          :class="['toggle-icon', { 'toggle-icon-open': showEntryFeeSlider }]"
          width="16"
          height="16"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
        >
          <path d="M6 9l6 6 6-6" />
        </svg>
      </button>

      <div v-if="showEntryFeeSlider" class="entry-fee-slider">
        <div class="range-labels">
          <span>{{ formatCurrency(entryFeeRange[0]) }}</span>
          <span>{{ formatCurrency(entryFeeRange[1]) }}</span>
        </div>
        <div class="dual-range">
          <input
            type="range"
            :value="entryFeeRange[0]"
            min="0"
            max="10000"
            step="100"
            class="range-input range-min"
            @input="entryFeeRange[0] = Math.min(Number(($event.target as HTMLInputElement).value), entryFeeRange[1] - 100); updateEntryFeeRange()"
          />
          <input
            type="range"
            :value="entryFeeRange[1]"
            min="0"
            max="10000"
            step="100"
            class="range-input range-max"
            @input="entryFeeRange[1] = Math.max(Number(($event.target as HTMLInputElement).value), entryFeeRange[0] + 100); updateEntryFeeRange()"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.contest-filters {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  padding: var(--spacing-md);
  background-color: var(--color-bg-primary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
}

.filters-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.filters-label {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-primary);
}

.filter-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 20px;
  height: 20px;
  padding: 0 var(--spacing-xs);
  font-size: var(--font-size-xs);
  font-weight: 600;
  background-color: var(--color-primary);
  color: white;
  border-radius: var(--radius-full);
}

.clear-btn {
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-secondary);
  background: none;
  border: none;
  cursor: pointer;
  transition: color var(--transition-fast);
}

.clear-btn:hover {
  color: var(--color-danger);
}

.filter-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
}

.section-label {
  font-size: var(--font-size-xs);
  font-weight: 500;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.chips-container {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-xs);
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
  scrollbar-width: none;
  -ms-overflow-style: none;
}

.chips-container::-webkit-scrollbar {
  display: none;
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
  background-color: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-full);
  cursor: pointer;
  white-space: nowrap;
  transition: all var(--transition-fast);
}

.chip:hover {
  background-color: var(--color-bg-tertiary);
  border-color: var(--color-primary-light);
}

.chip-active {
  background-color: var(--color-primary);
  border-color: var(--color-primary);
  color: white;
}

.chip-active:hover {
  background-color: var(--color-primary-dark);
  border-color: var(--color-primary-dark);
}

.chip-icon {
  font-size: var(--font-size-md);
  line-height: 1;
}

.chip-label {
  line-height: 1.2;
}

/* Toggle Switch */
.toggle-container {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  cursor: pointer;
}

.toggle-container input {
  display: none;
}

.toggle-slider {
  position: relative;
  width: 40px;
  height: 22px;
  background-color: var(--color-bg-tertiary);
  border-radius: var(--radius-full);
  transition: background-color var(--transition-fast);
}

.toggle-slider::after {
  content: '';
  position: absolute;
  top: 3px;
  left: 3px;
  width: 16px;
  height: 16px;
  background-color: white;
  border-radius: 50%;
  transition: transform var(--transition-fast);
  box-shadow: var(--shadow-sm);
}

.toggle-container input:checked + .toggle-slider {
  background-color: var(--color-primary);
}

.toggle-container input:checked + .toggle-slider::after {
  transform: translateX(18px);
}

[dir="rtl"] .toggle-slider::after {
  left: auto;
  right: 3px;
}

[dir="rtl"] .toggle-container input:checked + .toggle-slider::after {
  transform: translateX(-18px);
}

.toggle-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

/* Section Toggle */
.section-toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: var(--spacing-xs) 0;
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-primary);
  background: none;
  border: none;
  cursor: pointer;
}

.toggle-icon {
  transition: transform var(--transition-fast);
  color: var(--color-text-secondary);
}

.toggle-icon-open {
  transform: rotate(180deg);
}

/* Entry Fee Slider */
.entry-fee-slider {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) 0;
}

.range-labels {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}

.dual-range {
  position: relative;
  height: 20px;
}

.range-input {
  position: absolute;
  width: 100%;
  height: 4px;
  background: transparent;
  pointer-events: none;
  appearance: none;
  -webkit-appearance: none;
}

.range-input::-webkit-slider-runnable-track {
  width: 100%;
  height: 4px;
  background: var(--color-bg-tertiary);
  border-radius: 2px;
}

.range-input::-webkit-slider-thumb {
  appearance: none;
  -webkit-appearance: none;
  width: 16px;
  height: 16px;
  background: var(--color-primary);
  border-radius: 50%;
  cursor: pointer;
  pointer-events: all;
  margin-top: -6px;
  box-shadow: var(--shadow-sm);
}

.range-input::-moz-range-track {
  width: 100%;
  height: 4px;
  background: var(--color-bg-tertiary);
  border-radius: 2px;
}

.range-input::-moz-range-thumb {
  width: 16px;
  height: 16px;
  background: var(--color-primary);
  border-radius: 50%;
  cursor: pointer;
  pointer-events: all;
  border: none;
  box-shadow: var(--shadow-sm);
}

.range-min {
  z-index: 1;
}

.range-max {
  z-index: 2;
}

/* Mobile horizontal scrolling */
@media (max-width: 767px) {
  .chips-container {
    flex-wrap: nowrap;
    padding-bottom: var(--spacing-xs);
  }

  .contest-filters {
    border-radius: var(--radius-md);
  }
}
</style>
