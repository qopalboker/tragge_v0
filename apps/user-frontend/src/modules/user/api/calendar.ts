import { api } from './index';

// ==================== Calendar API Types ====================

export type ContestType = 'rush' | 'standard' | 'tournament' | 'championship' | 'practice';
export type AssetClass = 'forex' | 'crypto' | 'stocks' | 'mixed';
export type ContestStatus = 'scheduled' | 'registration_open' | 'running' | 'paused' | 'settling' | 'completed' | 'cancelled';

export interface CalendarParticipants {
  current: number;
  max?: number;
}

export interface CalendarContest {
  id: string;
  name: string;
  type: ContestType;
  asset_class: AssetClass;
  entry_fee: number; // in dollars
  duration_minutes: number;
  starts_at: string;
  ends_at: string;
  status: ContestStatus;
  participants: CalendarParticipants;
  user_registered: boolean;
}

export interface CalendarResponse {
  from: string;
  to: string;
  contests: CalendarContest[];
  total: number;
}

export interface CalendarGroup {
  key: string;
  label: string;
  contests: CalendarContest[];
  count: number;
}

export interface CalendarGroupedResponse {
  from: string;
  to: string;
  groups: CalendarGroup[];
  total: number;
}

export interface CalendarFilters {
  from?: string;
  to?: string;
  asset_class?: AssetClass;
  type?: ContestType;
  min_entry?: number;
  max_entry?: number;
  registered_only?: boolean;
  group_by?: 'date' | 'day' | 'asset_class';
}

// ==================== Calendar API ====================

export const calendarApi = {
  /**
   * Get calendar contests for a date range
   */
  async getCalendar(filters?: CalendarFilters): Promise<CalendarResponse> {
    const params: Record<string, string> = {};

    if (filters?.from) params.from = filters.from;
    if (filters?.to) params.to = filters.to;
    if (filters?.asset_class) params.asset_class = filters.asset_class;
    if (filters?.type) params.type = filters.type;
    if (filters?.min_entry !== undefined) params.min_entry = String(filters.min_entry);
    if (filters?.max_entry !== undefined) params.max_entry = String(filters.max_entry);
    if (filters?.registered_only) params.registered_only = 'true';

    const response = await api.get<CalendarResponse>('/api/user/contests/calendar', { params });
    return response.data;
  },

  /**
   * Get calendar contests grouped by date or asset class
   */
  async getCalendarGrouped(filters: CalendarFilters & { group_by: 'date' | 'asset_class' }): Promise<CalendarGroupedResponse> {
    const params: Record<string, string> = {
      group_by: filters.group_by,
    };

    if (filters.from) params.from = filters.from;
    if (filters.to) params.to = filters.to;
    if (filters.asset_class) params.asset_class = filters.asset_class;
    if (filters.type) params.type = filters.type;
    if (filters.min_entry !== undefined) params.min_entry = String(filters.min_entry);
    if (filters.max_entry !== undefined) params.max_entry = String(filters.max_entry);
    if (filters.registered_only) params.registered_only = 'true';

    const response = await api.get<CalendarGroupedResponse>('/api/user/contests/calendar', { params });
    return response.data;
  },

  /**
   * Get user's registered contests
   */
  async getMySchedule(filters?: { status?: 'upcoming' | 'live' | 'past' }): Promise<CalendarContest[]> {
    const params: Record<string, string> = {
      registered_only: 'true',
    };

    const now = new Date();
    if (filters?.status === 'upcoming') {
      params.from = now.toISOString().split('T')[0];
      params.to = new Date(now.getTime() + 30 * 24 * 60 * 60 * 1000).toISOString().split('T')[0];
    } else if (filters?.status === 'past') {
      params.from = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000).toISOString().split('T')[0];
      params.to = now.toISOString().split('T')[0];
    }

    const response = await api.get<CalendarResponse>('/api/user/contests/calendar', { params });
    return response.data.contests;
  },
};
