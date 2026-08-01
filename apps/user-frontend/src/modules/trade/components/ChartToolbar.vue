<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { getGroupedTools, QUICK_ACCESS_TOOLS, type DrawingToolGroup } from '@/utils/drawingTools';
import { t } from '@/i18n';

export interface IndicatorConfig {
  type: string;
  enabled: boolean;
  color: string;
  period?: number;
  fastPeriod?: number;
  slowPeriod?: number;
  signalPeriod?: number;
  stdDev?: number;
}

const emit = defineEmits<{
  (e: 'indicatorToggle', indicator: IndicatorConfig): void;
  (e: 'drawingToolSelect', tool: string | null): void;
  (e: 'clearDrawings'): void;
  (e: 'timeframeChange', timeframe: string): void;
  (e: 'positionLinesToggle', visible: boolean): void;
  (e: 'pendingOrderLinesToggle', visible: boolean): void;
}>();

// LocalStorage keys for line visibility preferences
const POSITION_LINES_KEY = 'tragge:chart:showPositionLines';
const PENDING_ORDER_LINES_KEY = 'tragge:chart:showPendingOrderLines';

// Position lines toggle state (persisted to localStorage)
function getInitialPositionLinesState(): boolean {
  if (typeof window !== 'undefined') {
    const stored = localStorage.getItem(POSITION_LINES_KEY);
    return stored !== null ? stored === 'true' : true;
  }
  return true;
}
const showPositionLines = ref<boolean>(getInitialPositionLinesState());

// Pending order lines toggle state (persisted to localStorage)
function getInitialPendingOrderLinesState(): boolean {
  if (typeof window !== 'undefined') {
    const stored = localStorage.getItem(PENDING_ORDER_LINES_KEY);
    return stored !== null ? stored === 'true' : true;
  }
  return true;
}
const showPendingOrderLines = ref<boolean>(getInitialPendingOrderLinesState());

// Toolbar state
const showIndicators = ref(false);
const showDrawingTools = ref(false);
const showTimeframes = ref(false);
const activeDrawingTool = ref<string | null>(null);

// Drawing tool groups from plugin registry
const toolGroups = ref<DrawingToolGroup[]>([]);

// Indicator configurations
const indicators = ref<IndicatorConfig[]>([
  { type: 'SMA', enabled: false, color: '#3B82F6', period: 20 },
  { type: 'EMA', enabled: false, color: '#10B981', period: 12 },
  { type: 'RSI', enabled: false, color: '#8B5CF6', period: 14 },
  { type: 'MACD', enabled: false, color: '#F59E0B', fastPeriod: 12, slowPeriod: 26, signalPeriod: 9 },
  { type: 'BB', enabled: false, color: '#EC4899', period: 20, stdDev: 2 },
  { type: 'VWAP', enabled: false, color: '#14B8A6' },
  { type: 'Stochastic', enabled: false, color: '#6366F1', period: 14 },
  { type: 'ATR', enabled: false, color: '#F97316', period: 14 },
  { type: 'SAR', enabled: false, color: '#06B6D4' },
]);

// Timeframes
const timeframes = [
  { value: '1m', label: '1m' },
  { value: '5m', label: '5m' },
  { value: '15m', label: '15m' },
  { value: '30m', label: '30m' },
  { value: '1h', label: '1H' },
  { value: '4h', label: '4H' },
  { value: '1d', label: '1D' },
  { value: '1w', label: '1W' },
];

const selectedTimeframe = ref('1m');

/**
 * Toggle indicator on/off
 */
function toggleIndicator(indicator: IndicatorConfig): void {
  indicator.enabled = !indicator.enabled;
  emit('indicatorToggle', indicator);
}

/**
 * Select a drawing tool
 */
function selectDrawingTool(toolType: string): void {
  if (activeDrawingTool.value === toolType) {
    activeDrawingTool.value = null;
    emit('drawingToolSelect', null);
  } else {
    activeDrawingTool.value = toolType;
    emit('drawingToolSelect', toolType);
  }
  showDrawingTools.value = false;
}

/**
 * Clear all drawings
 */
function clearDrawings(): void {
  emit('clearDrawings');
  showDrawingTools.value = false;
}

/**
 * Change timeframe
 */
function changeTimeframe(timeframe: string): void {
  selectedTimeframe.value = timeframe;
  emit('timeframeChange', timeframe);
  showTimeframes.value = false;
}

/**
 * Toggle position lines visibility
 */
function togglePositionLines(): void {
  showPositionLines.value = !showPositionLines.value;
  if (typeof window !== 'undefined') {
    localStorage.setItem(POSITION_LINES_KEY, String(showPositionLines.value));
  }
  emit('positionLinesToggle', showPositionLines.value);
}

/**
 * Toggle pending order lines visibility
 */
function togglePendingOrderLines(): void {
  showPendingOrderLines.value = !showPendingOrderLines.value;
  if (typeof window !== 'undefined') {
    localStorage.setItem(PENDING_ORDER_LINES_KEY, String(showPendingOrderLines.value));
  }
  emit('pendingOrderLinesToggle', showPendingOrderLines.value);
}

/**
 * Get localized tool name
 */
function getToolName(toolType: string): string {
  const key = `chart.tool.${toolType}`;
  const translated = t(key);
  // If no translation exists, fall back to the tool type formatted nicely
  if (translated === key) {
    return toolType
      .split('-')
      .map(w => w.charAt(0).toUpperCase() + w.slice(1))
      .join(' ');
  }
  return translated;
}

/**
 * Check if a tool is a quick-access tool
 */
function isQuickAccessTool(toolType: string): boolean {
  return (QUICK_ACCESS_TOOLS as readonly string[]).includes(toolType);
}

/**
 * Close all dropdowns
 */
function closeAllDropdowns(): void {
  showIndicators.value = false;
  showDrawingTools.value = false;
  showTimeframes.value = false;
}

// Close dropdowns when clicking outside
function handleOutsideClick(e: MouseEvent): void {
  const target = e.target as HTMLElement;
  if (!target.closest('.toolbar-dropdown')) {
    closeAllDropdowns();
  }
}

// Initialize tool groups and emit initial state on mount
onMounted(() => {
  toolGroups.value = getGroupedTools();
  emit('positionLinesToggle', showPositionLines.value);
  emit('pendingOrderLinesToggle', showPendingOrderLines.value);
  window.addEventListener('click', handleOutsideClick);
});

onUnmounted(() => {
  window.removeEventListener('click', handleOutsideClick);
});
</script>

<template>
  <div class="chart-toolbar">
    <!-- Timeframes -->
    <div class="toolbar-section toolbar-dropdown">
      <button
        class="toolbar-button"
        @click.stop="showTimeframes = !showTimeframes"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 6v6l4 2" />
        </svg>
        <span>{{ selectedTimeframe }}</span>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>

      <div v-if="showTimeframes" class="dropdown-menu">
        <div class="dropdown-header">{{ t('chart.timeframe') }}</div>
        <button
          v-for="tf in timeframes"
          :key="tf.value"
          :class="['dropdown-item', { active: selectedTimeframe === tf.value }]"
          @click="changeTimeframe(tf.value)"
        >
          {{ tf.label }}
        </button>
      </div>
    </div>

    <!-- Indicators -->
    <div class="toolbar-section toolbar-dropdown">
      <button
        class="toolbar-button"
        @click.stop="showIndicators = !showIndicators"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 3v18h18" />
          <path d="m19 9-5 5-4-4-3 3" />
        </svg>
        <span>{{ t('chart.indicators') }}</span>
      </button>

      <div v-if="showIndicators" class="dropdown-menu indicators-menu">
        <div class="dropdown-header">{{ t('chart.technicalIndicators') }}</div>
        <label
          v-for="indicator in indicators"
          :key="indicator.type"
          class="indicator-item"
        >
          <input
            type="checkbox"
            :checked="indicator.enabled"
            @change="toggleIndicator(indicator)"
          >
          <span class="indicator-name">{{ indicator.type }}</span>
          <span
            class="indicator-color"
            :style="{ backgroundColor: indicator.color }"
          ></span>
        </label>
      </div>
    </div>

    <!-- Quick-access drawing tool buttons -->
    <div class="toolbar-section toolbar-divider"></div>
    <div
      v-for="toolType in QUICK_ACCESS_TOOLS"
      :key="toolType"
      class="toolbar-section"
    >
      <button
        class="toolbar-button toolbar-button-icon"
        :class="{ active: activeDrawingTool === toolType }"
        :title="getToolName(toolType)"
        @click="selectDrawingTool(toolType)"
      >
        <!-- Trend Line -->
        <svg v-if="toolType === 'trend-line'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="m3 20 18-16" />
        </svg>
        <!-- Horizontal Line -->
        <svg v-else-if="toolType === 'horizontal-line'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 12h18" />
        </svg>
        <!-- Fib Retracement -->
        <svg v-else-if="toolType === 'fib-retracement'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M3 20h18M3 16h18M3 12h18M3 8h18M3 4h18" />
        </svg>
        <!-- Rectangle -->
        <svg v-else-if="toolType === 'rectangle'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="3" y="3" width="18" height="18" rx="2" />
        </svg>
        <!-- Text Annotation -->
        <svg v-else-if="toolType === 'text-annotation'" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M4 7V4h16v3M9 20h6M12 4v16" />
        </svg>
      </button>
    </div>

    <!-- More Drawing Tools Dropdown -->
    <div class="toolbar-section toolbar-dropdown">
      <button
        class="toolbar-button"
        :class="{ active: activeDrawingTool !== null && !isQuickAccessTool(activeDrawingTool) }"
        @click.stop="showDrawingTools = !showDrawingTools"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="m12 19 7-7 3 3-7 7-3-3z" />
          <path d="m18 13-1.5-7.5L2 2l3.5 14.5L13 18l5-5z" />
          <path d="m2 2 7.586 7.586" />
          <circle cx="11" cy="11" r="2" />
        </svg>
        <span>{{ t('chart.drawingTools') }}</span>
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>

      <div v-if="showDrawingTools" class="dropdown-menu drawing-tools-menu">
        <template v-for="(group, groupIndex) in toolGroups" :key="group.label">
          <div v-if="groupIndex > 0" class="dropdown-divider"></div>
          <div class="dropdown-header">{{ t(`chart.toolGroup.${group.label.toLowerCase()}`) || group.label }}</div>
          <button
            v-for="tool in group.tools"
            :key="tool.type"
            :class="['dropdown-item', { active: activeDrawingTool === tool.type }]"
            @click="selectDrawingTool(tool.type)"
          >
            {{ getToolName(tool.type) }}
          </button>
        </template>

        <div class="dropdown-divider"></div>

        <button
          class="dropdown-item danger"
          @click="clearDrawings"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
          </svg>
          {{ t('chart.clearDrawings') }}
        </button>
      </div>
    </div>

    <!-- Position Lines Toggle -->
    <div class="toolbar-section toolbar-divider"></div>
    <div class="toolbar-section">
      <button
        class="toolbar-button position-lines-toggle"
        :class="{ active: showPositionLines }"
        :title="t('chart.showPositionLines')"
        @click="togglePositionLines"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="3" y="3" width="18" height="18" rx="2" />
          <path d="M3 9h18" />
          <path d="M3 15h18" />
          <circle cx="9" cy="9" r="1.5" fill="currentColor" />
          <circle cx="15" cy="15" r="1.5" fill="currentColor" />
        </svg>
        <span>{{ t('chart.positionLines') }}</span>
      </button>
    </div>

    <!-- Pending Order Lines Toggle -->
    <div class="toolbar-section">
      <button
        class="toolbar-button pending-order-lines-toggle"
        :class="{ active: showPendingOrderLines }"
        :title="t('chart.showPendingOrderLines')"
        @click="togglePendingOrderLines"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 6v6l4 2" />
          <path d="M3 20h18" stroke-dasharray="3 2" />
        </svg>
        <span>{{ t('chart.pendingOrderLines') }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.chart-toolbar {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: var(--color-bg-secondary);
  border-bottom: 1px solid var(--color-border);
}

.toolbar-section {
  position: relative;
}

.toolbar-divider {
  width: 1px;
  height: 20px;
  background: var(--color-border);
  margin: 0 var(--spacing-xs);
}

.toolbar-button {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: transparent;
  border: 1px solid var(--color-border);
  border-radius: var(--border-radius);
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition: all 0.2s ease;
}

.toolbar-button-icon {
  padding: var(--spacing-xs);
}

.toolbar-button:hover {
  background: var(--color-bg-tertiary);
  border-color: var(--color-primary);
}

.toolbar-button.active {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: white;
}

/* Position lines toggle specific styling */
.position-lines-toggle.active {
  background: rgba(16, 185, 129, 0.15);
  border-color: rgba(16, 185, 129, 0.5);
  color: #10B981;
}

.position-lines-toggle.active:hover {
  background: rgba(16, 185, 129, 0.25);
}

/* Pending order lines toggle specific styling */
.pending-order-lines-toggle.active {
  background: rgba(59, 130, 246, 0.15);
  border-color: rgba(59, 130, 246, 0.5);
  color: #3B82F6;
}

.pending-order-lines-toggle.active:hover {
  background: rgba(59, 130, 246, 0.25);
}

.toolbar-button svg {
  flex-shrink: 0;
}

.dropdown-menu {
  position: absolute;
  top: calc(100% + 4px);
  inset-inline-start: 0;
  min-width: 200px;
  background: var(--color-bg-secondary);
  border: 1px solid var(--color-border);
  border-radius: var(--border-radius);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  z-index: 100;
  max-height: 400px;
  overflow-y: auto;
}

.drawing-tools-menu {
  min-width: 240px;
  max-height: 500px;
}

.dropdown-header {
  padding: var(--spacing-sm);
  font-size: var(--font-size-xs);
  font-weight: 600;
  color: var(--color-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  border-bottom: 1px solid var(--color-border);
}

.dropdown-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  width: 100%;
  padding: var(--spacing-sm);
  background: transparent;
  border: none;
  color: var(--color-text-primary);
  font-size: var(--font-size-sm);
  text-align: start;
  cursor: pointer;
  transition: background 0.2s ease;
}

.dropdown-item:hover {
  background: var(--color-bg-tertiary);
}

.dropdown-item.active {
  background: var(--color-primary-dim);
  color: var(--color-primary);
}

.dropdown-item.danger {
  color: var(--color-danger);
}

.dropdown-item.danger:hover {
  background: rgba(239, 68, 68, 0.1);
}

.dropdown-divider {
  height: 1px;
  background: var(--color-border);
  margin: var(--spacing-xs) 0;
}

/* Indicators menu */
.indicators-menu {
  min-width: 220px;
}

.indicator-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm);
  cursor: pointer;
  transition: background 0.2s ease;
}

.indicator-item:hover {
  background: var(--color-bg-tertiary);
}

.indicator-item input[type="checkbox"] {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

.indicator-name {
  flex: 1;
  font-size: var(--font-size-sm);
  color: var(--color-text-primary);
}

.indicator-color {
  width: 24px;
  height: 4px;
  border-radius: 2px;
}

/* Scrollbar styling */
.dropdown-menu::-webkit-scrollbar {
  width: 6px;
}

.dropdown-menu::-webkit-scrollbar-track {
  background: var(--color-bg-secondary);
}

.dropdown-menu::-webkit-scrollbar-thumb {
  background: var(--color-border);
  border-radius: 3px;
}

.dropdown-menu::-webkit-scrollbar-thumb:hover {
  background: var(--color-text-muted);
}

/* Mobile responsive */
@media (max-width: 768px) {
  .chart-toolbar {
    flex-wrap: wrap;
  }

  .toolbar-button span {
    display: none;
  }

  .dropdown-menu {
    min-width: 180px;
  }
}
</style>
