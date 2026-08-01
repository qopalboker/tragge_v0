import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import { api } from '@/api';

// Types matching the user-bff API responses
export interface ContestSymbol {
  symbol: string;
  enabled: boolean;
}

export type DurationType = 'rush_30min' | 'hourly' | 'four_hour' | 'daily' | 'weekly';
export type MarketType = 'crypto' | 'forex' | 'stocks' | 'mixed';

export interface Contest {
  id: string;
  name: string;
  description?: string;
  starts_at: string;
  ends_at: string;
  status: 'registration_open' | 'scheduled' | 'running' | 'paused' | 'settling' | 'completed' | 'cancelled';
  entry_fee_cents: number;
  qty_total: number;
  duration_type?: DurationType;
  market_type?: MarketType;
  rules?: Record<string, unknown>;
  symbols: ContestSymbol[];
  // Additional fields for contest discovery
  participant_count?: number;
  max_participants?: number;
  estimated_prize_pool_cents?: number;
  prize_winners_percentage?: number;
  is_free?: boolean;
}

export interface ContestFilters {
  duration_type?: DurationType;
  market_type?: MarketType;
  is_free?: boolean;
  min_entry?: number;
  max_entry?: number;
}

export interface JoinContestResponse {
  contest_id: string;
  user_id: string;
  joined_at: string;
  qty_total: number;
  qty_available: number;
  already_joined?: boolean;
}

export interface LeaderboardEntry {
  rank: number;
  user_id: string;
  total_score: number;
}

export interface LeaderboardResponse {
  contest_id: string;
  entries: LeaderboardEntry[];
}

export interface ContestHistoryEntry {
  contest_id: string;
  contest_name: string;
  status: string;
  joined_at: string;
  total_score: number;
  final_rank?: number;
  final_prize_cents?: number;
}

export interface ContestHistoryResponse {
  contests: ContestHistoryEntry[];
}

export const useContestsStore = defineStore('contests', () => {
  // State
  const contests = ref<Contest[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const joiningContestId = ref<string | null>(null);
  const joinedContestIds = ref<Set<string>>(new Set());

  // Leaderboard state
  const leaderboard = ref<LeaderboardEntry[]>([]);
  const leaderboardLoading = ref(false);
  const leaderboardError = ref<string | null>(null);
  const selectedContestId = ref<string | null>(null);

  // User history state
  const userHistory = ref<ContestHistoryEntry[]>([]);
  const historyLoading = ref(false);

  // Computed
  const liveContests = computed(() =>
    contests.value.filter(c => c.status === 'running')
  );

  const upcomingContests = computed(() =>
    contests.value.filter(c => c.status === 'registration_open' || c.status === 'scheduled')
  );

  // Actions
  async function fetchContests(filters?: ContestFilters): Promise<void> {
    loading.value = true;
    error.value = null;

    try {
      const params: Record<string, string> = {};
      if (filters?.duration_type) {
        params.duration_type = filters.duration_type;
      }
      if (filters?.market_type) {
        params.market_type = filters.market_type;
      }
      if (filters?.is_free) {
        params.is_free = 'true';
      }
      if (filters?.min_entry !== undefined) {
        params.min_entry = String(filters.min_entry);
      }
      if (filters?.max_entry !== undefined) {
        params.max_entry = String(filters.max_entry);
      }

      const response = await api.get<Contest[]>('/api/user/contests', { params });
      contests.value = response.data;
    } catch (err: unknown) {
      const message = getErrorMessage(err);
      error.value = message;
      throw new Error(message);
    } finally {
      loading.value = false;
    }
  }

  async function joinContest(contestId: string): Promise<JoinContestResponse> {
    joiningContestId.value = contestId;

    try {
      const response = await api.post<JoinContestResponse>(`/api/user/contests/${contestId}/join`);
      joinedContestIds.value.add(contestId);
      return response.data;
    } catch (err: unknown) {
      const message = getErrorMessage(err);
      throw new Error(message);
    } finally {
      joiningContestId.value = null;
    }
  }

  async function fetchLeaderboard(contestId: string, options?: { silent?: boolean }): Promise<void> {
    if (!options?.silent) {
      leaderboardLoading.value = true;
    }
    leaderboardError.value = null;
    selectedContestId.value = contestId;

    try {
      const response = await api.get<LeaderboardResponse>('/api/user/leaderboard', {
        params: { contest_id: contestId }
      });
      leaderboard.value = response.data.entries;
    } catch (err: unknown) {
      const message = getErrorMessage(err);
      leaderboardError.value = message;
      throw new Error(message);
    } finally {
      if (!options?.silent) {
        leaderboardLoading.value = false;
      }
    }
  }

  async function fetchUserHistory(): Promise<void> {
    historyLoading.value = true;

    try {
      const response = await api.get<ContestHistoryResponse>('/api/user/me/history');
      userHistory.value = response.data.contests;
      // Populate joined contest IDs from history
      for (const entry of response.data.contests) {
        joinedContestIds.value.add(entry.contest_id);
      }
    } catch (err: unknown) {
      // Silently fail for history - not critical
      console.error('Failed to fetch user history:', err);
    } finally {
      historyLoading.value = false;
    }
  }

  function isJoined(contestId: string): boolean {
    return joinedContestIds.value.has(contestId);
  }

  function isJoining(contestId: string): boolean {
    return joiningContestId.value === contestId;
  }

  function clearError(): void {
    error.value = null;
  }

  function clearLeaderboardError(): void {
    leaderboardError.value = null;
  }

  return {
    // State
    contests,
    loading,
    error,
    joiningContestId,
    joinedContestIds,
    leaderboard,
    leaderboardLoading,
    leaderboardError,
    selectedContestId,
    userHistory,
    historyLoading,
    // Computed
    liveContests,
    upcomingContests,
    // Actions
    fetchContests,
    joinContest,
    fetchLeaderboard,
    fetchUserHistory,
    isJoined,
    isJoining,
    clearError,
    clearLeaderboardError,
  };
});

function getErrorMessage(err: unknown): string {
  if (err && typeof err === 'object' && 'response' in err) {
    const axiosError = err as { response?: { data?: { error?: string; message?: string } } };
    return axiosError.response?.data?.error || axiosError.response?.data?.message || 'An error occurred';
  }
  return 'An error occurred';
}
