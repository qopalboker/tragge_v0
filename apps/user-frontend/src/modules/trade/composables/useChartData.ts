import { ref, computed, type Ref } from 'vue';
import type { CandlestickData, Time } from 'lightweight-charts';
import { api } from '@/api';

export interface TradingViewBar {
  time: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface TradingViewCandlesResponse {
  bars: TradingViewBar[];
  noData: boolean;
}

export interface TickData {
  symbol: string;
  bid: number;
  ask: number;
  last: number;
  timestamp: number;
}

export interface CandlestickDataWithVolume extends CandlestickData {
  volume?: number;
}

interface CurrentBar {
  time: Time;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

/** Maps timeframe strings to milliseconds for bar boundary calculation. */
export const RESOLUTION_MAP: Record<string, number> = {
  '1m': 60_000,
  '5m': 300_000,
  '15m': 900_000,
  '30m': 1_800_000,
  '1h': 3_600_000,
  '4h': 14_400_000,
  '1d': 86_400_000,
  '1w': 604_800_000,
};

/** Per-timeframe history limits — how many candles to fetch for each resolution. */
export const HISTORY_LIMITS: Record<string, number> = {
  '1m': 2880,   // 2 days
  '5m': 1440,   // 5 days
  '15m': 1344,  // 14 days
  '30m': 1440,  // 30 days
  '1h': 1440,   // 60 days
  '4h': 1080,   // 180 days
  '1d': 365,    // 1 year
  '1w': 104,    // 2 years
};

export function getHistoryLimit(resolution: string): number {
  return HISTORY_LIMITS[resolution] ?? 500;
}

/** Maps internal resolution to TradingView format for the /api/trade/candles endpoint. */
const TV_RESOLUTION_MAP: Record<string, string> = {
  '1m': '1', '5m': '5', '15m': '15', '30m': '30',
  '1h': '60', '4h': '240', '1d': 'D', '1w': 'W',
};

export interface UseChartDataReturn {
  candles: Ref<CandlestickDataWithVolume[]>;
  isLoading: Ref<boolean>;
  error: Ref<string | null>;
  currentBar: Ref<CurrentBar | null>;
  /** Increments only on fetchHistory/reset — watch this instead of deep-watching candles. */
  dataVersion: Ref<number>;
  fetchHistory: (symbol: string, resolution?: string, limit?: number) => Promise<void>;
  updateWithTick: (tick: TickData) => CandlestickData | null;
  setResolution: (timeframe: string) => void;
  reset: () => void;
}

/**
 * Composable for managing chart candlestick data.
 * Handles historical data fetching and real-time tick aggregation.
 */
export function useChartData(): UseChartDataReturn {
  const candles = ref<CandlestickDataWithVolume[]>([]);
  const isLoading = ref(false);
  const error = ref<string | null>(null);
  const currentBar = ref<CurrentBar | null>(null);
  /** Bumped only on historical data load / reset — NOT on tick updates. */
  const dataVersion = ref(0);
  let resolutionMs = 60_000;

  /**
   * Set the bar resolution from a timeframe string (e.g. '5m', '1h').
   */
  function setResolution(timeframe: string): void {
    resolutionMs = RESOLUTION_MAP[timeframe] ?? 60_000;
  }

  /**
   * Get the bar time (start of the interval) for a given timestamp.
   */
  function getBarTime(timestamp: number): Time {
    const barStartMs = Math.floor(timestamp / resolutionMs) * resolutionMs;
    return Math.floor(barStartMs / 1000) as Time;
  }

  /**
   * Fetch historical candles from the TradingView-compatible API.
   */
  async function fetchHistory(
    symbol: string,
    resolution: string = '1m',
    limit?: number
  ): Promise<void> {
    const resolvedLimit = limit ?? getHistoryLimit(resolution);
    isLoading.value = true;
    error.value = null;

    try {
      // Compute from/to range from limit
      const resMs = RESOLUTION_MAP[resolution] ?? 60_000;
      const now = Date.now();
      const toTS = Math.floor(now / 1000);
      const fromTS = toTS - Math.floor((resolvedLimit * resMs) / 1000);
      const tvResolution = TV_RESOLUTION_MAP[resolution] ?? '1';

      const response = await api.get<TradingViewCandlesResponse>('/api/trade/candles', {
        params: { symbol, resolution: tvResolution, from: fromTS, to: toTS },
      });

      if (response.data.noData || !response.data.bars) {
        candles.value = [];
        error.value = 'Chart data unavailable';
        dataVersion.value++;
        return;
      }

      const historicalCandles = response.data.bars
        .filter((bar: TradingViewBar) =>
          bar.time != null && bar.open != null && bar.high != null && bar.low != null && bar.close != null &&
          isFinite(bar.open) && isFinite(bar.high) && isFinite(bar.low) && isFinite(bar.close) &&
          bar.open > 0 && bar.high > 0 && bar.low > 0 && bar.close > 0
        )
        .map((bar: TradingViewBar): CandlestickDataWithVolume => ({
          time: Math.floor(bar.time / 1000) as Time, // ms → seconds for lightweight-charts
          open: bar.open,
          high: bar.high,
          low: bar.low,
          close: bar.close,
          volume: bar.volume ?? 0,
        }));

      // Sort by time ascending
      historicalCandles.sort((a, b) => (a.time as number) - (b.time as number));
      candles.value = historicalCandles;
      dataVersion.value++;

      // Initialize current bar from last historical candle
      if (historicalCandles.length > 0) {
        const lastCandle = historicalCandles[historicalCandles.length - 1];
        currentBar.value = { ...lastCandle, volume: lastCandle.volume ?? 0 };
      }
    } catch (err) {
      console.warn('Chart history API not available:', err);
      candles.value = [];
      currentBar.value = null;
      error.value = 'Chart data unavailable';
      dataVersion.value++;
    } finally {
      isLoading.value = false;
    }
  }

  /**
   * Update chart with a new tick.
   * Returns the updated bar if the tick falls within the current bar,
   * or a new bar if we've crossed into a new interval.
   */
  function updateWithTick(tick: TickData): CandlestickData | null {
    const tickTime = getBarTime(tick.timestamp);
    const tickPrice = tick.last;

    // Skip ticks with invalid prices
    if (!tickPrice || !isFinite(tickPrice) || tickPrice <= 0) return null;

    if (!currentBar.value) {
      // First tick - create new bar
      currentBar.value = {
        time: tickTime,
        open: tickPrice,
        high: tickPrice,
        low: tickPrice,
        close: tickPrice,
        volume: 1,
      };
      return { ...currentBar.value };
    }

    const currentBarTime = currentBar.value.time as number;

    if ((tickTime as number) > currentBarTime) {
      // New bar started - finalize current bar and start new one
      const completedBar = { ...currentBar.value };

      // Add completed bar to candles — last element is almost always the match
      const lastIdx = candles.value.length - 1;
      if (lastIdx >= 0 && (candles.value[lastIdx].time as number) === currentBarTime) {
        candles.value[lastIdx] = completedBar;
      } else {
        candles.value.push(completedBar);
      }

      // Start new bar
      currentBar.value = {
        time: tickTime,
        open: tickPrice,
        high: tickPrice,
        low: tickPrice,
        close: tickPrice,
        volume: 1,
      };

      return { ...currentBar.value };
    } else {
      // Update current bar
      currentBar.value.high = Math.max(currentBar.value.high, tickPrice);
      currentBar.value.low = Math.min(currentBar.value.low, tickPrice);
      currentBar.value.close = tickPrice;
      currentBar.value.volume++;

      return { ...currentBar.value };
    }
  }

  /**
   * Reset all chart data.
   */
  function reset(): void {
    candles.value = [];
    currentBar.value = null;
    error.value = null;
    isLoading.value = false;
    dataVersion.value++;
  }

  return {
    candles: computed(() => candles.value),
    isLoading: computed(() => isLoading.value),
    error: computed(() => error.value),
    currentBar: computed(() => currentBar.value),
    dataVersion: computed(() => dataVersion.value),
    fetchHistory,
    updateWithTick,
    setResolution,
    reset,
  };
}
