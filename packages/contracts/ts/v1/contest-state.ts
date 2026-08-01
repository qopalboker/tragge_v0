import type { ContestPhase } from './enums';

/**
 * Contest state change event.
 */
export interface ContestState {
  contest_id: string;
  phase: ContestPhase;
  ts: number;
}

/**
 * Rank prize brief for broadcast.
 */
export interface RankPrizeBrief {
  rank: number;
  amount_cents: number;
}

/**
 * Contest update payload broadcast when a participant joins or leaves.
 * Used for real-time prize pool recalculation on the frontend.
 */
export interface ContestUpdatePayload {
  type: 'contest_updated';
  contest_id: string;
  event: 'participant_joined' | 'participant_left';
  current_participants: number;
  prize_pool_cents: number;
  winners_count: number;
  first_prize_cents: number;
  total_prize_cents: number;
  top_3_prizes: RankPrizeBrief[];
  ts: number;
}
