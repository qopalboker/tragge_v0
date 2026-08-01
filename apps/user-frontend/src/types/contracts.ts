/**
 * Local type definitions for trading contracts
 * These mirror the types from @tragge/contracts/v1
 */

export type OrderSide = 'BUY' | 'SELL';
export type OrderStatus = 'PENDING' | 'ACCEPTED' | 'REJECTED' | 'FILLED' | 'CANCELLED';

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
  take_profit?: number;
  stop_loss?: number;
}

/**
 * Request body for updating position TP/SL.
 */
export interface UpdateTPSLRequest {
  take_profit?: number | null;
  stop_loss?: number | null;
}

/**
 * Response from update TP/SL API.
 */
export interface UpdateTPSLResponse {
  position_id: string;
  take_profit?: number;
  stop_loss?: number;
  message: string;
}

/**
 * User position update event.
 */
export interface PositionUpdate {
  user_id: string;
  contest_id: string;
  positions: Position[];
}

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

/**
 * PnL score change event for leaderboard updates.
 * Scoring uses Tralent-like formula:
 *   - LONG:  pct_change = (exit_price - entry_price) / entry_price * 100
 *   - SHORT: pct_change = (entry_price - exit_price) / entry_price * 100
 *   - trade_score = qty_used * pct_change
 *   - total_score = realized_score + unrealized_score
 */
export interface PnLDelta {
  user_id: string;
  contest_id: string;
  delta_score: number;        // Change in realized score from latest trade
  realized_score: number;     // Sum of all closed trade scores
  unrealized_score: number;   // Mark-to-market score for open positions
  total_score: number;        // realized_score + unrealized_score
  ts: number;
}

/**
 * Acknowledgment response for an order request.
 */
export interface OrderAck {
  order_id: string;
  status: OrderStatus;
  reason?: string;
}

/**
 * Response from close position API.
 */
export interface ClosePositionResponse {
  order_id: string;
  message: string;
}

/**
 * Response from cancel order API.
 */
export interface CancelOrderResponse {
  order_id: string;
  message: string;
}

/**
 * Cancel reason for order cancellation.
 */
export type CancelReason = 'user_requested' | 'contest_ended' | 'insufficient_funds' | 'expired';

/**
 * Order cancelled event from WebSocket.
 */
export interface OrderCancelledEvent {
  order_id: string;
  user_id: string;
  contest_id: string;
  symbol: string;
  qty_released: number;
  cancel_reason: CancelReason;
  ts: number;
}

/**
 * A single order history item.
 */
export interface OrderHistoryItem {
  order_id: string;
  symbol: string;
  side: 'BUY' | 'SELL';
  type: string;
  qty: number;
  status: string;
  created_at: string;
  fill_price?: number;
  fill_qty?: number;
  fill_time?: string;
  pnl: number;
}

/**
 * Response from order history API.
 */
export interface OrderHistoryResponse {
  orders: OrderHistoryItem[];
  total: number;
  limit: number;
  offset: number;
}

/**
 * Options for fetching order history.
 */
export interface OrderHistoryOptions {
  limit?: number;
  offset?: number;
  status?: string;
  symbol?: string;
}

/**
 * Response from balance/QTY allocation API.
 */
export interface BalanceResponse {
  contest_id: string;
  user_id: string;
  qty_total: number;
  qty_available: number;
  qty_used: number;
}

/**
 * A single leaderboard entry.
 */
export interface LeaderboardEntry {
  rank: number;
  user_id: string;
  username?: string;
  total_score: number;
  rank_change?: number;
}

/**
 * Response from leaderboard API.
 */
export interface LeaderboardResponse {
  contest_id: string;
  entries: LeaderboardEntry[];
  total?: number;
}

/**
 * Options for fetching leaderboard.
 */
export interface LeaderboardOptions {
  limit?: number;
  offset?: number;
}

/**
 * Contest status enum.
 */
export type ContestStatus = 'draft' | 'scheduled' | 'registration_open' | 'running' | 'paused' | 'completed' | 'cancelled';

/**
 * Contest duration type.
 */
export type ContestDurationType = 'rush_30min' | 'hourly' | 'four_hour' | 'daily' | 'weekly';

/**
 * Contest symbol info.
 */
export interface ContestSymbol {
  symbol: string;
  enabled: boolean;
}

/**
 * Contest response from API.
 */
export interface Contest {
  id: string;
  name: string;
  description?: string;
  starts_at: string;
  ends_at: string;
  status: ContestStatus;
  entry_fee_cents: number;
  qty_total: number;
  duration_type?: ContestDurationType;
  rules?: Record<string, unknown>;
  symbols?: ContestSymbol[];
  participants?: number;
  is_free?: boolean;
}

/**
 * Detailed contest information from GET /api/user/contests/:id.
 */
export interface ContestDetailsResponse {
  id: string;
  name: string;
  description?: string;
  status: ContestStatus;
  market_type: string;
  asset_class: string;
  duration_type: string;
  start_time: string;
  end_time: string;
  entry_fee_cents: number;
  is_free: boolean;
  prize_pool_cents: number;
  available_qty: number;
  max_participants?: number;
  current_participants: number;
  user_joined: boolean;
  symbols: string[];
  commission_rate: number;
  gross_prize_pool_cents: number;
  server_time: string;
}

/**
 * A single rank's prize in the preview.
 */
export interface PrizeRankPreview {
  rank: number;
  amount_cents: number;
  percentage: number;
}

/**
 * Prize preview response from GET /api/user/contests/:id/prize-preview.
 */
export interface PrizePreviewResponse {
  contest_id: string;
  current_participants: number;
  min_participants: number;
  quorum_met: boolean;
  entry_fee_cents: number;
  commission_rate: number;
  prize_pool_cents: number;
  winners_count: number;
  prizes: PrizeRankPreview[];
  status: string;
  message: string;
}

/**
 * User's joined contest with additional info.
 */
export interface UserContest {
  id: string;
  name: string;
  status: ContestStatus;
  joined_at: string;
  total_score: number;
  final_rank?: number;
  final_prize_cents?: number;
}

/**
 * Response from contests API.
 */
export interface ContestsResponse {
  contests: Contest[];
}

/**
 * Response from user's joined contests API.
 */
export interface UserContestsResponse {
  contests: UserContest[];
}
