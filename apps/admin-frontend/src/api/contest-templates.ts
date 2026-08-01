import { api } from './index';

// Types
export interface ContestTemplate {
  key: string;
  name: string;
  description: string;
  asset_class: string;
  duration_type: string;
  duration_minutes: number;
  default_symbols: string[];
  qty_total: number;
  entry_fee_cents: number;
  min_participants: number;
  max_participants: number | null;
  platform_fee_bps: number;
  is_free: boolean;
}

// API functions

export async function getContestTemplates(): Promise<ContestTemplate[]> {
  const response = await api.get<ContestTemplate[]>('/api/admin/contests/templates');
  return response.data;
}

export async function getContestTemplate(key: string): Promise<ContestTemplate> {
  const response = await api.get<ContestTemplate>(`/api/admin/contests/templates/${encodeURIComponent(key)}`);
  return response.data;
}
