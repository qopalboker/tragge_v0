import type { OrderSide, OrderType } from './enums';

/**
 * Request to place a new order.
 */
export interface OrderRequest {
  order_id: string;
  user_id: string;
  contest_id: string;
  symbol: string;
  side: OrderSide;
  type: OrderType;
  qty: number;
  limit_price?: number;
  stop_price?: number;
  take_profit?: number;
  stop_loss?: number;
  client_ts: number;
}
