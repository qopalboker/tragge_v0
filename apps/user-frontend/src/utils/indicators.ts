import type { CandlestickData, LineData, Time } from 'lightweight-charts';
import { SMA, EMA, RSI, MACD, BollingerBands, Stochastic, ATR, PSAR, VWAP } from 'technicalindicators';

/**
 * Technical Indicators Utilities
 * Uses the technicalindicators package for reliable calculations
 */

export interface IndicatorData extends LineData {
  time: Time;
  value: number;
}

/**
 * Simple Moving Average (SMA)
 */
export function calculateSMA(data: CandlestickData[], period: number): IndicatorData[] {
  if (data.length < period) return [];

  const closes = data.map(d => d.close);
  const result = SMA.calculate({ period, values: closes });

  return result.map((value, i) => ({
    time: data[i + period - 1].time,
    value,
  }));
}

/**
 * Exponential Moving Average (EMA)
 */
export function calculateEMA(data: CandlestickData[], period: number): IndicatorData[] {
  if (data.length < period) return [];

  const closes = data.map(d => d.close);
  const result = EMA.calculate({ period, values: closes });

  return result.map((value, i) => ({
    time: data[i + period - 1].time,
    value,
  }));
}

/**
 * Relative Strength Index (RSI)
 */
export function calculateRSI(data: CandlestickData[], period: number = 14): IndicatorData[] {
  if (data.length < period + 1) return [];

  const closes = data.map(d => d.close);
  const result = RSI.calculate({ period, values: closes });

  return result.map((value, i) => ({
    time: data[i + period].time,
    value,
  }));
}

/**
 * MACD (Moving Average Convergence Divergence)
 */
export interface MACDData {
  time: Time;
  macd: number;
  signal: number;
  histogram: number;
}

export function calculateMACD(
  data: CandlestickData[],
  fastPeriod: number = 12,
  slowPeriod: number = 26,
  signalPeriod: number = 9
): MACDData[] {
  if (data.length < slowPeriod + signalPeriod) return [];

  const closes = data.map(d => d.close);
  const result = MACD.calculate({
    values: closes,
    fastPeriod,
    slowPeriod,
    signalPeriod,
    SimpleMAOscillator: false,
    SimpleMASignal: false,
  });

  // Filter out results where any value is undefined
  const validResults = result.filter(
    (r): r is { MACD: number; signal: number; histogram: number } =>
      r.MACD !== undefined && r.signal !== undefined && r.histogram !== undefined
  );

  // The MACD results align to the end of the data array
  const offset = data.length - result.length;
  const validOffset = offset + (result.length - validResults.length);

  return validResults.map((r, i) => ({
    time: data[i + validOffset].time,
    macd: r.MACD,
    signal: r.signal,
    histogram: r.histogram,
  }));
}

/**
 * Bollinger Bands
 */
export interface BollingerBandsData {
  time: Time;
  upper: number;
  middle: number;
  lower: number;
}

export function calculateBollingerBands(
  data: CandlestickData[],
  period: number = 20,
  stdDev: number = 2
): BollingerBandsData[] {
  if (data.length < period) return [];

  const closes = data.map(d => d.close);
  const result = BollingerBands.calculate({
    period,
    values: closes,
    stdDev,
  });

  return result.map((r, i) => ({
    time: data[i + period - 1].time,
    upper: r.upper,
    middle: r.middle,
    lower: r.lower,
  }));
}

/**
 * Volume Weighted Average Price (VWAP)
 */
export function calculateVWAP(data: (CandlestickData & { volume?: number })[]): IndicatorData[] {
  if (data.length === 0) return [];

  // If no volume data, fall back to manual calculation
  const hasVolume = data.some(d => d.volume && d.volume > 0);
  if (!hasVolume) {
    // Fallback: use typical price when no volume available
    const result: IndicatorData[] = [];
    let cumulativeTPV = 0;
    let cumulativeVolume = 0;

    for (const candle of data) {
      const typicalPrice = (candle.high + candle.low + candle.close) / 3;
      const volume = candle.volume || 0;
      cumulativeTPV += typicalPrice * volume;
      cumulativeVolume += volume;
      const vwap = cumulativeVolume > 0 ? cumulativeTPV / cumulativeVolume : typicalPrice;
      result.push({ time: candle.time, value: vwap });
    }
    return result;
  }

  const vwapResult = VWAP.calculate({
    high: data.map(d => d.high),
    low: data.map(d => d.low),
    close: data.map(d => d.close),
    volume: data.map(d => d.volume || 0),
  });

  return vwapResult.map((value, i) => ({
    time: data[i].time,
    value,
  }));
}

/**
 * Stochastic Oscillator
 */
export interface StochasticData {
  time: Time;
  k: number;
  d: number;
}

export function calculateStochastic(
  data: CandlestickData[],
  kPeriod: number = 14,
  dPeriod: number = 3
): StochasticData[] {
  if (data.length < kPeriod + dPeriod - 1) return [];

  const result = Stochastic.calculate({
    high: data.map(d => d.high),
    low: data.map(d => d.low),
    close: data.map(d => d.close),
    period: kPeriod,
    signalPeriod: dPeriod,
  });

  // Filter valid results
  const validResults = result.filter(
    (r): r is { k: number; d: number } => r.k !== undefined && r.d !== undefined
  );

  const offset = data.length - result.length;
  const validOffset = offset + (result.length - validResults.length);

  return validResults.map((r, i) => ({
    time: data[i + validOffset].time,
    k: r.k,
    d: r.d,
  }));
}

/**
 * Average True Range (ATR)
 */
export function calculateATR(data: CandlestickData[], period: number = 14): IndicatorData[] {
  if (data.length < period + 1) return [];

  const result = ATR.calculate({
    high: data.map(d => d.high),
    low: data.map(d => d.low),
    close: data.map(d => d.close),
    period,
  });

  const offset = data.length - result.length;

  return result.map((value, i) => ({
    time: data[i + offset].time,
    value,
  }));
}

/**
 * Parabolic SAR (Stop and Reverse)
 */
export function calculateParabolicSAR(
  data: CandlestickData[],
  accelerationFactor: number = 0.02,
  maxAcceleration: number = 0.2
): IndicatorData[] {
  if (data.length < 2) return [];

  const result = PSAR.calculate({
    high: data.map(d => d.high),
    low: data.map(d => d.low),
    step: accelerationFactor,
    max: maxAcceleration,
  });

  const offset = data.length - result.length;

  return result.map((value, i) => ({
    time: data[i + offset].time,
    value,
  }));
}
