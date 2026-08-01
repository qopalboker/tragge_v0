import type { OrderSide } from './enums';

/**
 * A single trading position.
 */
export interface Position {
  position_id: string;
  symbol: string;
  side: OrderSide;
  qty: number;
  entry_price: number;
  mark_price: number;
  unrealized_pnl_pct: number;
  realized_pnl_pct: number;
  qty_used: number;
  /**
   * Unrealized P&L score using Tralent formula:
   * For LONG: pct_change = (mark_price - entry_price) / entry_price * 100
   * For SHORT: pct_change = (entry_price - mark_price) / entry_price * 100
   * unrealized_score = qty_used * pct_change
   */
  unrealized_score: number;
}

/**
 * User position update event.
 */
export interface PositionUpdate {
  user_id: string;
  contest_id: string;
  positions: Position[];
}
