/**
 * PnL score change event for leaderboard updates.
 */
export interface PnLDelta {
  user_id: string;
  contest_id: string;
  delta_score: number;
  total_score: number;
  ts: number;
}
