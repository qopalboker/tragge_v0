<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, computed, type PropType } from 'vue';
import {
  createChart,
  type IChartApi,
  type ISeriesApi,
  type CandlestickData,
  type HistogramData,
  type LineData,
  type IPriceLine,
  ColorType,
  CrosshairMode,
  LineStyle,
  CandlestickSeries,
  HistogramSeries,
  LineSeries,
} from 'lightweight-charts';
import { useChartData, type TickData, type CandlestickDataWithVolume } from '@/composables/useChartData';
import { useTradingStore, type Order } from '@/stores/trading';
import type { Position } from '@/types/contracts';
import { t } from '@/i18n';
import { getSymbolMetadata } from '@/utils/symbolMetadata';
import ChartToolbar, { type IndicatorConfig } from './ChartToolbar.vue';
import {
  DrawingManager,
  saveDrawings,
  loadDrawings,
} from '@/utils/drawingTools';
import {
  calculateSMA,
  calculateEMA,
  calculateRSI,
  calculateMACD,
  calculateBollingerBands,
  calculateVWAP,
  calculateStochastic,
  calculateATR,
  calculateParabolicSAR,
} from '@/utils/indicators';

const props = defineProps({
  symbol: {
    type: String,
    required: true,
  },
  resolution: {
    type: String,
    default: '1m',
  },
  ticks: {
    type: Array as PropType<TickData[]>,
    default: () => [],
  },
  showPositionLines: {
    type: Boolean,
    default: true,
  },
  contestId: {
    type: String,
    default: '',
  },
});

const emit = defineEmits<{
  (e: 'priceUpdate', price: number): void;
}>();

// Chart container ref
const chartContainer = ref<HTMLDivElement | null>(null);

// Chart instances
let chart: IChartApi | null = null;
let candleSeries: ISeriesApi<'Candlestick'> | null = null;
let volumeSeries: ISeriesApi<'Histogram'> | null = null;
let drawingManager: DrawingManager | null = null;

// Indicator series
const indicatorSeries = ref<Map<string, ISeriesApi<'Line'>>>(new Map());
const activeIndicators = ref<IndicatorConfig[]>([]);

// Drawing tool state
const activeDrawingTool = ref<string | null>(null);

// Position lines visibility (controlled by ChartToolbar)
const positionLinesVisible = ref(true);

// Trading store for position data
const tradingStore = useTradingStore();

// Position price lines tracking
interface PositionLines {
  entry: IPriceLine;
  tp?: IPriceLine;
  sl?: IPriceLine;
}
const positionLinesMap = new Map<string, PositionLines>();

// Pending order lines visibility (controlled by ChartToolbar)
const pendingOrderLinesVisible = ref(true);

// Pending order price lines tracking
interface PendingOrderLines {
  trigger: IPriceLine;
}
const pendingOrderLinesMap = new Map<string, PendingOrderLines>();

// Computed: pending orders for current symbol
const pendingOrders = computed(() => {
  const result: Order[] = [];
  tradingStore.orders.forEach((order) => {
    if (order.status === 'PENDING' && order.symbol === props.symbol) {
      result.push(order);
    }
  });
  return result;
});

// Computed: serialized pending order state for change detection
const pendingOrderStateKey = computed(() => {
  return pendingOrders.value
    .map(o => `${o.order_id}:${o.type}:${o.qty}:${o.limit_price ?? 'null'}:${o.stop_price ?? 'null'}`)
    .sort()
    .join('|');
});

// Computed: positions for current symbol as array (more reliable reactivity than watching Map)
const symbolPositions = computed(() => {
  const result: Position[] = [];
  tradingStore.positions.forEach((position) => {
    if (position.symbol === props.symbol) {
      result.push(position);
    }
  });
  return result;
});

// Computed: serialized position state for change detection
const positionStateKey = computed(() => {
  return symbolPositions.value
    .map(p => `${p.position_id}:${p.entry_price}:${p.qty}:${p.take_profit ?? 'null'}:${p.stop_loss ?? 'null'}`)
    .sort()
    .join('|');
});

// Chart data composable
const { candles, isLoading, error, dataVersion, fetchHistory, updateWithTick, setResolution, reset } = useChartData();

// Current price display
const currentPrice = ref<number | null>(null);
const priceChange = ref<number>(0);
const priceChangePercent = ref<number>(0);

// Chart colors matching the design system
const chartColors = {
  background: 'transparent',
  textColor: 'rgba(255, 255, 255, 0.7)',
  gridColor: 'rgba(255, 255, 255, 0.05)',
  upColor: '#10B981',
  downColor: '#EF4444',
  borderUpColor: '#10B981',
  borderDownColor: '#EF4444',
  wickUpColor: '#10B981',
  wickDownColor: '#EF4444',
};

/**
 * Initialize the chart with proper configuration.
 */
function initChart(): void {
  if (!chartContainer.value) return;

  // Clean up existing chart and position/pending order lines
  clearPositionLines();
  clearPendingOrderLines();
  if (drawingManager) {
    drawingManager.detach();
    drawingManager = null;
  }
  if (chart) {
    chart.remove();
    chart = null;
    candleSeries = null;
    volumeSeries = null;
  }

  const containerWidth = chartContainer.value.clientWidth;
  const containerHeight = chartContainer.value.clientHeight;

  chart = createChart(chartContainer.value, {
    width: containerWidth,
    height: containerHeight,
    layout: {
      background: { type: ColorType.Solid, color: chartColors.background },
      textColor: chartColors.textColor,
    },
    grid: {
      vertLines: { color: chartColors.gridColor },
      horzLines: { color: chartColors.gridColor },
    },
    crosshair: {
      mode: CrosshairMode.Normal,
      vertLine: {
        width: 1,
        color: 'rgba(59, 130, 246, 0.5)',
        style: 2,
      },
      horzLine: {
        width: 1,
        color: 'rgba(59, 130, 246, 0.5)',
        style: 2,
      },
    },
    rightPriceScale: {
      borderColor: 'rgba(255, 255, 255, 0.1)',
      scaleMargins: {
        top: 0.1,
        bottom: 0.1,
      },
    },
    timeScale: {
      borderColor: 'rgba(255, 255, 255, 0.1)',
      timeVisible: true,
      secondsVisible: false,
      fixLeftEdge: true,
      fixRightEdge: true,
    },
    handleScroll: {
      vertTouchDrag: false,
    },
  });

  // v5: use chart.addSeries(CandlestickSeries, options)
  candleSeries = chart.addSeries(CandlestickSeries, {
    upColor: chartColors.upColor,
    downColor: chartColors.downColor,
    borderUpColor: chartColors.borderUpColor,
    borderDownColor: chartColors.borderDownColor,
    wickUpColor: chartColors.wickUpColor,
    wickDownColor: chartColors.wickDownColor,
  });

  // Initialize with empty data to prevent "Value is null" crash
  // when the render loop fires before fetchHistory() completes
  candleSeries.setData([]);

  // Volume histogram at the bottom of the chart
  volumeSeries = chart.addSeries(HistogramSeries, {
    priceFormat: { type: 'volume' },
    priceScaleId: 'volume',
  });
  volumeSeries.priceScale().applyOptions({
    scaleMargins: { top: 0.8, bottom: 0 },
  });
  volumeSeries.setData([]);

  // Subscribe to crosshair move for tooltip
  chart.subscribeCrosshairMove((param) => {
    if (!param.time || !candleSeries) return;

    const data = param.seriesData.get(candleSeries) as CandlestickData | undefined;
    if (data) {
      currentPrice.value = data.close;
    }
  });

  // Initialize drawing manager (lightweight-charts-drawing plugin)
  if (chartContainer.value && chart && candleSeries) {
    drawingManager = new DrawingManager();
    drawingManager.attach(chart, candleSeries, chartContainer.value);

    // Set up drawing persistence events
    const persistDrawings = () => {
      if (drawingManager && props.contestId && props.symbol) {
        saveDrawings(drawingManager, props.contestId, props.symbol);
      }
    };
    drawingManager.on('drawing:added', persistDrawings);
    drawingManager.on('drawing:updated', persistDrawings);
    drawingManager.on('drawing:removed', persistDrawings);

  }
}

/**
 * Update chart with new candle data.
 */
function updateChartData(newCandles: CandlestickData[]): void {
  if (!candleSeries || newCandles.length === 0) return;

  // Filter out any candles with null/invalid OHLC to prevent lightweight-charts crash
  const validCandles = newCandles.filter(c =>
    c.open != null && c.high != null && c.low != null && c.close != null &&
    isFinite(c.open as number) && isFinite(c.high as number) && isFinite(c.low as number) && isFinite(c.close as number)
  );
  if (validCandles.length === 0) return;

  candleSeries.setData(validCandles);

  // Update volume histogram
  if (volumeSeries) {
    const volumeData = validCandles.map((c: CandlestickDataWithVolume) => ({
      time: c.time,
      value: (c as CandlestickDataWithVolume).volume ?? 0,
      color: c.close >= c.open
        ? 'rgba(16, 185, 129, 0.3)'  // green (up candle)
        : 'rgba(239, 68, 68, 0.3)',  // red (down candle)
    }));
    volumeSeries.setData(volumeData);
  }

  // Set initial visible range to show the most recent candles
  if (chart && validCandles.length > 0) {
    const barsToShow = Math.min(80, validCandles.length);
    chart.timeScale().setVisibleLogicalRange({
      from: validCandles.length - barsToShow,
      to: validCandles.length + 5,
    });
  }

  // Update current price from last candle
  const lastCandle = newCandles[newCandles.length - 1];
  if (lastCandle) {
    currentPrice.value = lastCandle.close;

    // Calculate price change from first candle
    const firstCandle = newCandles[0];
    if (firstCandle) {
      priceChange.value = lastCandle.close - firstCandle.open;
      priceChangePercent.value = ((lastCandle.close - firstCandle.open) / firstCandle.open) * 100;
    }
  }
}

/**
 * Handle window resize to make chart responsive.
 */
function handleResize(): void {
  if (!chart || !chartContainer.value) return;

  const containerWidth = chartContainer.value.clientWidth;
  const containerHeight = chartContainer.value.clientHeight;

  chart.applyOptions({
    width: containerWidth,
    height: containerHeight,
  });
}

// Debounced resize handler
let resizeTimeout: ReturnType<typeof setTimeout> | null = null;
function debouncedResize(): void {
  if (resizeTimeout) {
    clearTimeout(resizeTimeout);
  }
  resizeTimeout = setTimeout(handleResize, 100);
}

/**
 * Handle indicator toggle from toolbar
 */
function handleIndicatorToggle(indicator: IndicatorConfig): void {
  if (indicator.enabled) {
    addIndicator(indicator);
  } else {
    removeIndicator(indicator.type);
  }
}

/**
 * Add an indicator to the chart
 */
function addIndicator(config: IndicatorConfig): void {
  if (!chart || candles.value.length === 0) return;

  // Remove existing indicator if present
  removeIndicator(config.type);

  let series: ISeriesApi<'Line'> | null = null;
  let data: LineData[] = [];

  try {
    switch (config.type) {
      case 'SMA':
        data = calculateSMA(candles.value, config.period || 20);
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 2,
          title: `SMA(${config.period})`,
        });
        break;

      case 'EMA':
        data = calculateEMA(candles.value, config.period || 12);
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 2,
          title: `EMA(${config.period})`,
        });
        break;

      case 'RSI':
        data = calculateRSI(candles.value, config.period || 14);
        // RSI needs a separate pane
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 2,
          title: `RSI(${config.period})`,
          priceScaleId: 'rsi',
        });
        series.priceScale().applyOptions({
          scaleMargins: {
            top: 0.8,
            bottom: 0,
          },
        });
        break;

      case 'MACD': {
        const macdData = calculateMACD(
          candles.value,
          config.fastPeriod || 12,
          config.slowPeriod || 26,
          config.signalPeriod || 9
        );
        const macdLine = macdData.map(d => ({ time: d.time, value: d.macd }));
        const signalLine = macdData.map(d => ({ time: d.time, value: d.signal }));

        // MACD line
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 2,
          title: 'MACD',
          priceScaleId: 'macd',
        });
        series.setData(macdLine);

        // Signal line
        const signalSeries = chart.addSeries(LineSeries, {
          color: '#F59E0B',
          lineWidth: 1,
          lineStyle: LineStyle.Dashed,
          title: 'Signal',
          priceScaleId: 'macd',
        });
        signalSeries.setData(signalLine);

        series.priceScale().applyOptions({
          scaleMargins: {
            top: 0.8,
            bottom: 0,
          },
        });

        indicatorSeries.value.set(`${config.type}_signal`, signalSeries);
        break;
      }

      case 'BB': {
        const bbData = calculateBollingerBands(
          candles.value,
          config.period || 20,
          config.stdDev || 2
        );

        // Upper band
        const upperSeries = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 1,
          lineStyle: LineStyle.Dashed,
          title: 'BB Upper',
        });
        upperSeries.setData(bbData.map(d => ({ time: d.time, value: d.upper })));
        indicatorSeries.value.set(`${config.type}_upper`, upperSeries);

        // Middle band (SMA)
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 2,
          title: 'BB Middle',
        });
        data = bbData.map(d => ({ time: d.time, value: d.middle }));

        // Lower band
        const lowerSeries = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 1,
          lineStyle: LineStyle.Dashed,
          title: 'BB Lower',
        });
        lowerSeries.setData(bbData.map(d => ({ time: d.time, value: d.lower })));
        indicatorSeries.value.set(`${config.type}_lower`, lowerSeries);
        break;
      }

      case 'VWAP':
        data = calculateVWAP(candles.value);
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 2,
          title: 'VWAP',
        });
        break;

      case 'Stochastic': {
        const stochData = calculateStochastic(candles.value, config.period || 14);
        const kLine = stochData.map(d => ({ time: d.time, value: d.k }));
        const dLine = stochData.map(d => ({ time: d.time, value: d.d }));

        // %K line
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 2,
          title: '%K',
          priceScaleId: 'stoch',
        });
        series.setData(kLine);

        // %D line
        const dSeries = chart.addSeries(LineSeries, {
          color: '#6366F1',
          lineWidth: 1,
          lineStyle: LineStyle.Dashed,
          title: '%D',
          priceScaleId: 'stoch',
        });
        dSeries.setData(dLine);

        series.priceScale().applyOptions({
          scaleMargins: {
            top: 0.8,
            bottom: 0,
          },
        });

        indicatorSeries.value.set(`${config.type}_d`, dSeries);
        break;
      }

      case 'ATR':
        data = calculateATR(candles.value, config.period || 14);
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 2,
          title: `ATR(${config.period})`,
          priceScaleId: 'atr',
        });
        series.priceScale().applyOptions({
          scaleMargins: {
            top: 0.8,
            bottom: 0,
          },
        });
        break;

      case 'SAR':
        data = calculateParabolicSAR(candles.value);
        series = chart.addSeries(LineSeries, {
          color: config.color,
          lineWidth: 1,
          lineStyle: LineStyle.Dotted,
          title: 'Parabolic SAR',
        });
        break;
    }

    if (series && data.length > 0) {
      series.setData(data);
      indicatorSeries.value.set(config.type, series);
      activeIndicators.value.push(config);
    }
  } catch (err) {
    console.error(`Failed to add indicator ${config.type}:`, err);
  }
}

/**
 * Remove an indicator from the chart
 */
function removeIndicator(type: string): void {
  const series = indicatorSeries.value.get(type);
  if (series && chart) {
    chart.removeSeries(series);
    indicatorSeries.value.delete(type);
  }

  // Remove auxiliary series (for MACD, BB, Stochastic)
  const auxSeries = indicatorSeries.value.get(`${type}_signal`);
  if (auxSeries && chart) {
    chart.removeSeries(auxSeries);
    indicatorSeries.value.delete(`${type}_signal`);
  }

  const upperSeries = indicatorSeries.value.get(`${type}_upper`);
  if (upperSeries && chart) {
    chart.removeSeries(upperSeries);
    indicatorSeries.value.delete(`${type}_upper`);
  }

  const lowerSeries = indicatorSeries.value.get(`${type}_lower`);
  if (lowerSeries && chart) {
    chart.removeSeries(lowerSeries);
    indicatorSeries.value.delete(`${type}_lower`);
  }

  const dSeries = indicatorSeries.value.get(`${type}_d`);
  if (dSeries && chart) {
    chart.removeSeries(dSeries);
    indicatorSeries.value.delete(`${type}_d`);
  }

  // Remove from active indicators
  activeIndicators.value = activeIndicators.value.filter(i => i.type !== type);
}

/**
 * Update all active indicators with new data
 */
function updateIndicators(): void {
  for (const config of activeIndicators.value) {
    addIndicator(config);
  }
}

// Position line colors with proper opacity
const POSITION_COLORS = {
  long: '#10B981',
  short: '#F43F5E',
  tp: 'rgba(16, 185, 129, 0.85)',
  sl: 'rgba(239, 68, 68, 0.85)',
};

// Pending order line colors per order type
const PENDING_ORDER_COLORS: Record<string, string> = {
  BUY_LIMIT: '#3B82F6',
  SELL_LIMIT: '#8B5CF6',
  BUY_STOP: '#F59E0B',
  SELL_STOP: '#EC4899',
};

/**
 * Clear all position lines from the chart
 */
function clearPositionLines(): void {
  if (!candleSeries) return;

  positionLinesMap.forEach((lines) => {
    candleSeries!.removePriceLine(lines.entry);
    if (lines.tp) candleSeries!.removePriceLine(lines.tp);
    if (lines.sl) candleSeries!.removePriceLine(lines.sl);
  });
  positionLinesMap.clear();
}

/**
 * Clear all pending order lines from the chart
 */
function clearPendingOrderLines(): void {
  if (!candleSeries) return;

  pendingOrderLinesMap.forEach((lines) => {
    candleSeries!.removePriceLine(lines.trigger);
  });
  pendingOrderLinesMap.clear();
}

/**
 * Create price lines for a position with polished styling
 */
function createPositionLines(position: Position): PositionLines {
  if (!candleSeries) throw new Error('Candle series not initialized');

  const isLong = position.side === 'BUY';
  const entryColor = isLong ? POSITION_COLORS.long : POSITION_COLORS.short;

  const entrySymbol = isLong ? '▲' : '▼';
  const sideLabel = isLong ? t('chart.longEntry') : t('chart.shortEntry');
  const entryLabel = `${entrySymbol} ${sideLabel} ${position.qty}`;

  const entry = candleSeries.createPriceLine({
    price: position.entry_price,
    color: entryColor,
    lineWidth: 2,
    lineStyle: LineStyle.Solid,
    axisLabelVisible: true,
    title: entryLabel,
  });

  const lines: PositionLines = { entry };

  if (position.take_profit !== undefined && position.take_profit !== null) {
    lines.tp = candleSeries.createPriceLine({
      price: position.take_profit,
      color: POSITION_COLORS.tp,
      lineWidth: 1,
      lineStyle: LineStyle.Dashed,
      axisLabelVisible: true,
      title: `◎ ${t('chart.takeProfit')}`,
    });
  }

  if (position.stop_loss !== undefined && position.stop_loss !== null) {
    lines.sl = candleSeries.createPriceLine({
      price: position.stop_loss,
      color: POSITION_COLORS.sl,
      lineWidth: 1,
      lineStyle: LineStyle.Dashed,
      axisLabelVisible: true,
      title: `⊗ ${t('chart.stopLoss')}`,
    });
  }

  return lines;
}

/**
 * Update position lines on the chart
 */
function updatePositionLines(): void {
  if (!candleSeries || !props.symbol) return;

  if (!positionLinesVisible.value || !props.showPositionLines) {
    clearPositionLines();
    return;
  }

  const currentPositions = symbolPositions.value;
  const activePositionIds = new Set(currentPositions.map(p => p.position_id));

  // Remove lines for positions that no longer exist
  const toRemove: string[] = [];
  positionLinesMap.forEach((lines, positionId) => {
    if (!activePositionIds.has(positionId)) {
      candleSeries!.removePriceLine(lines.entry);
      if (lines.tp) candleSeries!.removePriceLine(lines.tp);
      if (lines.sl) candleSeries!.removePriceLine(lines.sl);
      toRemove.push(positionId);
    }
  });
  toRemove.forEach(id => positionLinesMap.delete(id));

  // Create or update lines for current positions
  for (const position of currentPositions) {
    const existingLines = positionLinesMap.get(position.position_id);

    if (existingLines) {
      const isLong = position.side === 'BUY';
      const entryColor = isLong ? POSITION_COLORS.long : POSITION_COLORS.short;

      const entrySymbol = isLong ? '▲' : '▼';
      const sideLabel = isLong ? t('chart.longEntry') : t('chart.shortEntry');
      const entryLabel = `${entrySymbol} ${sideLabel} ${position.qty}`;

      existingLines.entry.applyOptions({
        price: position.entry_price,
        color: entryColor,
        title: entryLabel,
      });

      // Handle take profit line
      if (position.take_profit !== undefined && position.take_profit !== null) {
        if (existingLines.tp) {
          existingLines.tp.applyOptions({
            price: position.take_profit,
            title: `◎ ${t('chart.takeProfit')}`,
          });
        } else {
          existingLines.tp = candleSeries!.createPriceLine({
            price: position.take_profit,
            color: POSITION_COLORS.tp,
            lineWidth: 1,
            lineStyle: LineStyle.Dashed,
            axisLabelVisible: true,
            title: `◎ ${t('chart.takeProfit')}`,
          });
        }
      } else if (existingLines.tp) {
        candleSeries!.removePriceLine(existingLines.tp);
        existingLines.tp = undefined;
      }

      // Handle stop loss line
      if (position.stop_loss !== undefined && position.stop_loss !== null) {
        if (existingLines.sl) {
          existingLines.sl.applyOptions({
            price: position.stop_loss,
            title: `⊗ ${t('chart.stopLoss')}`,
          });
        } else {
          existingLines.sl = candleSeries!.createPriceLine({
            price: position.stop_loss,
            color: POSITION_COLORS.sl,
            lineWidth: 1,
            lineStyle: LineStyle.Dashed,
            axisLabelVisible: true,
            title: `⊗ ${t('chart.stopLoss')}`,
          });
        }
      } else if (existingLines.sl) {
        candleSeries!.removePriceLine(existingLines.sl);
        existingLines.sl = undefined;
      }
    } else {
      const lines = createPositionLines(position);
      positionLinesMap.set(position.position_id, lines);
    }
  }
}

/**
 * Get the trigger price for a pending order based on its type
 */
function getPendingOrderPrice(order: Order): number | null {
  switch (order.type) {
    case 'BUY_LIMIT':
    case 'SELL_LIMIT':
      return order.limit_price ?? null;
    case 'BUY_STOP':
    case 'SELL_STOP':
      return order.stop_price ?? null;
    default:
      return null;
  }
}

/**
 * Create a price line for a pending order
 */
function createPendingOrderLine(order: Order): PendingOrderLines | null {
  if (!candleSeries) return null;

  const price = getPendingOrderPrice(order);
  if (price === null) return null;

  const color = PENDING_ORDER_COLORS[order.type] || '#6B7280';
  const typeLabel = order.type.replace('_', ' ');
  const decimals = getSymbolMetadata(order.symbol).decimals;
  const label = `⏳ ${typeLabel} ${order.qty} @ ${price.toFixed(decimals)}`;

  const trigger = candleSeries.createPriceLine({
    price,
    color,
    lineWidth: 1,
    lineStyle: LineStyle.Dashed,
    axisLabelVisible: true,
    title: label,
  });

  return { trigger };
}

/**
 * Update pending order lines on the chart
 */
function updatePendingOrderLines(): void {
  if (!candleSeries || !props.symbol) return;

  if (!pendingOrderLinesVisible.value) {
    clearPendingOrderLines();
    return;
  }

  const currentOrders = pendingOrders.value;
  const activeOrderIds = new Set(currentOrders.map(o => o.order_id));

  // Remove lines for orders that no longer exist
  const toRemove: string[] = [];
  pendingOrderLinesMap.forEach((lines, orderId) => {
    if (!activeOrderIds.has(orderId)) {
      candleSeries!.removePriceLine(lines.trigger);
      toRemove.push(orderId);
    }
  });
  toRemove.forEach(id => pendingOrderLinesMap.delete(id));

  // Create or update lines for current pending orders
  for (const order of currentOrders) {
    const price = getPendingOrderPrice(order);
    if (price === null) continue;

    const existingLines = pendingOrderLinesMap.get(order.order_id);

    if (existingLines) {
      const color = PENDING_ORDER_COLORS[order.type] || '#6B7280';
      const typeLabel = order.type.replace('_', ' ');
      const decimals = getSymbolMetadata(order.symbol).decimals;
      const label = `⏳ ${typeLabel} ${order.qty} @ ${price.toFixed(decimals)}`;

      existingLines.trigger.applyOptions({
        price,
        color,
        title: label,
      });
    } else {
      const lines = createPendingOrderLine(order);
      if (lines) {
        pendingOrderLinesMap.set(order.order_id, lines);
      }
    }
  }
}

/**
 * Handle drawing tool selection from toolbar
 */
function handleDrawingToolSelect(toolType: string | null): void {
  activeDrawingTool.value = toolType;
  if (drawingManager) {
    drawingManager.setActiveTool(toolType);
  }
}

/**
 * Clear all drawings
 */
function handleClearDrawings(): void {
  if (drawingManager) {
    drawingManager.clearAll();
    // Clear persisted drawings
    if (props.contestId && props.symbol) {
      saveDrawings(drawingManager, props.contestId, props.symbol);
    }
  }
}

/**
 * Handle timeframe change
 */
function handleTimeframeChange(timeframe: string): void {
  if (props.symbol) {
    setResolution(timeframe);
    reset();
    fetchHistory(props.symbol, timeframe);
  }
}

/**
 * Handle position lines toggle from toolbar
 */
function handlePositionLinesToggle(visible: boolean): void {
  positionLinesVisible.value = visible;
  if (visible) {
    updatePositionLines();
  } else {
    clearPositionLines();
  }
}

/**
 * Handle pending order lines toggle from toolbar
 */
function handlePendingOrderLinesToggle(visible: boolean): void {
  pendingOrderLinesVisible.value = visible;
  if (visible) {
    updatePendingOrderLines();
  } else {
    clearPendingOrderLines();
  }
}

// Watch for symbol changes
watch(
  () => props.symbol,
  async (newSymbol) => {
    if (newSymbol) {
      clearPositionLines();
      clearPendingOrderLines();
      reset();
      await fetchHistory(newSymbol, props.resolution);
      updateChartData(candles.value);
      updatePositionLines();
      updatePendingOrderLines();

      // Load drawings for new symbol
      if (drawingManager && props.contestId) {
        drawingManager.clearAll();
        await loadDrawings(drawingManager, props.contestId, newSymbol);
      }
    }
  }
);

// Watch for position changes from trading store
watch(
  positionStateKey,
  () => {
    updatePositionLines();
  },
  { immediate: true }
);

// Watch for pending order changes from trading store
watch(
  pendingOrderStateKey,
  () => {
    updatePendingOrderLines();
  },
  { immediate: true }
);

// Watch for historical data loads (NOT tick updates) via version counter.
watch(
  dataVersion,
  () => {
    const data = candles.value;
    if (data.length > 0) {
      updateChartData(data);
      if (activeIndicators.value.length > 0) {
        updateIndicators();
      }
    }
  },
);

// Watch for incoming ticks
watch(
  () => props.ticks,
  (newTicks) => {
    if (!newTicks || newTicks.length === 0) return;

    const symbolTick = newTicks.find(tick => tick.symbol === props.symbol);
    if (!symbolTick) return;

    const updatedBar = updateWithTick(symbolTick);
    if (updatedBar && candleSeries) {
      candleSeries.update(updatedBar);

      // Update volume histogram with current bar volume
      if (volumeSeries) {
        volumeSeries.update({
          time: updatedBar.time,
          value: (updatedBar as CandlestickDataWithVolume).volume ?? 0,
          color: updatedBar.close >= updatedBar.open
            ? 'rgba(16, 185, 129, 0.3)'
            : 'rgba(239, 68, 68, 0.3)',
        });
      }

      currentPrice.value = updatedBar.close;
      emit('priceUpdate', updatedBar.close);

      if (candles.value.length > 0) {
        const firstCandle = candles.value[0];
        priceChange.value = updatedBar.close - firstCandle.open;
        priceChangePercent.value = ((updatedBar.close - firstCandle.open) / firstCandle.open) * 100;
      }
    }
  },
);

// ResizeObserver for container size changes
let resizeObserver: ResizeObserver | null = null;

onMounted(async () => {
  initChart();

  setResolution(props.resolution);

  await fetchHistory(props.symbol, props.resolution);
  updateChartData(candles.value);

  updatePositionLines();
  updatePendingOrderLines();

  // Load saved drawings (must be awaited to avoid race condition)
  if (drawingManager && props.contestId && props.symbol) {
    await loadDrawings(drawingManager, props.contestId, props.symbol);
  }

  if (chartContainer.value) {
    resizeObserver = new ResizeObserver(debouncedResize);
    resizeObserver.observe(chartContainer.value);
  }

  window.addEventListener('resize', debouncedResize);
});

onUnmounted(() => {
  if (resizeTimeout) {
    clearTimeout(resizeTimeout);
  }

  if (resizeObserver) {
    resizeObserver.disconnect();
  }

  window.removeEventListener('resize', debouncedResize);

  if (drawingManager) {
    drawingManager.detach();
    drawingManager = null;
  }

  clearPositionLines();
  clearPendingOrderLines();

  if (chart) {
    chart.remove();
    chart = null;
    candleSeries = null;
    volumeSeries = null;
  }

  indicatorSeries.value.clear();
  activeIndicators.value = [];
});

// Computed for price display formatting
const symbolDecimals = computed(() => getSymbolMetadata(props.symbol).decimals);

const formattedPrice = computed(() => {
  if (currentPrice.value === null) return '--';
  return currentPrice.value.toFixed(symbolDecimals.value);
});

const formattedChange = computed(() => {
  const sign = priceChange.value >= 0 ? '+' : '';
  return `${sign}${priceChange.value.toFixed(symbolDecimals.value)} (${sign}${priceChangePercent.value.toFixed(2)}%)`;
});

const changeClass = computed(() => {
  if (priceChange.value > 0) return 'positive';
  if (priceChange.value < 0) return 'negative';
  return '';
});
</script>

<template>
  <div class="market-chart">
    <!-- Chart Toolbar -->
    <ChartToolbar
      @indicator-toggle="handleIndicatorToggle"
      @drawing-tool-select="handleDrawingToolSelect"
      @clear-drawings="handleClearDrawings"
      @timeframe-change="handleTimeframeChange"
      @position-lines-toggle="handlePositionLinesToggle"
      @pending-order-lines-toggle="handlePendingOrderLinesToggle"
    />

    <!-- Price header -->
    <div class="chart-header">
      <div class="symbol-info">
        <span class="symbol-name">{{ symbol }}</span>
        <span class="symbol-price">{{ formattedPrice }}</span>
        <span :class="['price-change', changeClass]">{{ formattedChange }}</span>
      </div>
      <div v-if="isLoading" class="loading-indicator">
        <span class="loading-spinner"></span>
        <span class="loading-text">{{ t('common.loading') }}</span>
      </div>
      <div v-if="activeDrawingTool" class="drawing-hint">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 16v-4M12 8h.01" />
        </svg>
        <span>{{ t('chart.drawingActive') }}</span>
      </div>
    </div>

    <!-- Chart container -->
    <div ref="chartContainer" class="chart-container">
      <div v-if="error && candles.length === 0" class="chart-error">
        <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
          <path d="M3 3v18h18"/>
          <path d="M7 16l4-4 4 4 4-7" stroke-dasharray="4 2"/>
        </svg>
        <p>{{ t('chart.noData') }}</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.market-chart {
  display: flex;
  flex-direction: column;
  width: 100%;
  height: 100%;
  min-height: 300px;
}

.chart-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-sm) var(--spacing-md);
  border-bottom: 1px solid var(--color-border);
}

.symbol-info {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-md);
}

.symbol-name {
  font-size: var(--font-size-lg);
  font-weight: 700;
  color: var(--color-text-primary);
}

.symbol-price {
  font-size: var(--font-size-xl);
  font-weight: 600;
  color: var(--color-text-primary);
  font-family: var(--font-family-mono);
}

.price-change {
  font-size: var(--font-size-sm);
  font-family: var(--font-family-mono);
  color: var(--color-text-secondary);
}

.price-change.positive {
  color: var(--color-buy);
}

.price-change.negative {
  color: var(--color-sell);
}

.loading-indicator {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  color: var(--color-text-muted);
  font-size: var(--font-size-sm);
}

.drawing-hint {
  display: flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xs) var(--spacing-sm);
  background: var(--color-primary-dim);
  border: 1px solid var(--color-primary);
  border-radius: var(--border-radius);
  color: var(--color-primary);
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.loading-spinner {
  width: 14px;
  height: 14px;
  border: 2px solid var(--color-border);
  border-top-color: var(--color-primary);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.chart-container {
  flex: 1;
  position: relative;
  min-height: 0;
  /* Chart rendering must always be LTR regardless of app direction */
  direction: ltr;
}

.chart-error {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
  color: var(--color-text-muted);
  text-align: center;
}

.chart-error p {
  font-size: var(--font-size-sm);
  color: var(--color-danger);
}
</style>
