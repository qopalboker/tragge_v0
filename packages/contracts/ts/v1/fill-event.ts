import type { OrderSide } from './enums';

/**
 * Order fill execution event.
 */
export interface FillEvent {
  fill_id: string;
  order_id: string;
  user_id: string;
  contest_id: string;
  symbol: string;
  side: OrderSide;
  qty: number;
  fill_price: number;
  ts: number;
}
