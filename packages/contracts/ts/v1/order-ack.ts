import type { OrderStatus } from './enums';

/**
 * Rate limit metadata included in rejection responses.
 */
export interface RateLimitInfo {
  /** Rate limit scope: "user", "contest", or "global" */
  scope: 'user' | 'contest' | 'global';
  /** Maximum requests allowed in the window */
  limit: number;
  /** Window duration (e.g., "1s", "1m") */
  window: string;
  /** Milliseconds until next request allowed */
  retry_after_ms: number;
}

/**
 * Acknowledgment response for an order request.
 */
export interface OrderAck {
  order_id: string;
  status: OrderStatus;
  reason?: string;
  /** Present only for RATE_LIMITED rejections */
  rate_limit?: RateLimitInfo;
}
