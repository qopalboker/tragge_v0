/**
 * Order side (buy or sell).
 */
export type OrderSide = 'BUY' | 'SELL';

/**
 * Order execution mode.
 * - MARKET: Execute immediately at current price
 * - PENDING: Execute when condition is met
 */
export type OrderMode = 'MARKET' | 'PENDING';

/**
 * Order type.
 * - MARKET: Immediate execution at current price
 * - BUY_LIMIT: Pending order, trigger when ask <= limit_price
 * - SELL_LIMIT: Pending order, trigger when bid >= limit_price
 * - BUY_STOP: Pending order, trigger when ask >= stop_price
 * - SELL_STOP: Pending order, trigger when bid <= stop_price
 * - LIMIT/STOP: Legacy types (deprecated)
 */
export type OrderType =
  | 'MARKET'
  | 'BUY_LIMIT'
  | 'SELL_LIMIT'
  | 'BUY_STOP'
  | 'SELL_STOP'
  | 'LIMIT'  // Deprecated
  | 'STOP';  // Deprecated

/**
 * Check if an order type is a pending order.
 */
export function isPendingOrderType(type: OrderType): boolean {
  return type === 'BUY_LIMIT' || type === 'SELL_LIMIT' || type === 'BUY_STOP' || type === 'SELL_STOP';
}

/**
 * Get the order mode for a given order type.
 */
export function getOrderMode(type: OrderType): OrderMode {
  return isPendingOrderType(type) ? 'PENDING' : 'MARKET';
}

/**
 * Order acknowledgment status.
 */
export type OrderStatus = 'ACCEPTED' | 'REJECTED';

/**
 * Contest phase.
 */
export type ContestPhase = 'UPCOMING' | 'LIVE' | 'FROZEN' | 'ENDED';

/**
 * Asset class for contests.
 */
export type AssetClass = 'forex' | 'crypto' | 'stocks' | 'mixed';

/**
 * Check if an asset class is valid.
 */
export function isValidAssetClass(value: string): value is AssetClass {
  return ['forex', 'crypto', 'stocks', 'mixed'].includes(value);
}

/**
 * Contest duration type.
 */
export type ContestDurationType = 'rush_30min' | 'hourly' | 'four_hour' | 'daily' | 'weekly';

/**
 * Check if a duration type is valid.
 */
export function isValidDurationType(value: string): value is ContestDurationType {
  return ['rush_30min', 'hourly', 'four_hour', 'daily', 'weekly'].includes(value);
}

/**
 * Get the duration in minutes for a duration type.
 */
export function getDurationMinutes(type: ContestDurationType): number {
  const durations: Record<ContestDurationType, number> = {
    rush_30min: 30,
    hourly: 60,
    four_hour: 240,
    daily: 1440,
    weekly: 10080,
  };
  return durations[type];
}

/**
 * Get the default QTY allocation for a duration type.
 */
export function getDefaultQtyAllocation(type: ContestDurationType): number {
  const allocations: Record<ContestDurationType, number> = {
    rush_30min: 50000,  // 10 QTY
    hourly: 100000,
    four_hour: 150000,
    daily: 200000,      // 20 QTY
    weekly: 500000,
  };
  return allocations[type];
}
