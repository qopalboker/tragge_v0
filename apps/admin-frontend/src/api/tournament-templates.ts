import { api } from './index';

export interface TournamentTemplate {
  id: string;
  name: string;
  description?: string;
  duration_minutes: number;
  market_type?: string;
  template_duration_type?: string;
  entry_fee: number;
  entry_fee_cents: number;
  qty_total: number;
  symbols_json: string;
  has_prize: boolean;
  is_active: boolean;
  is_free: boolean;
  asset_class: string;
  commission_rate: number;
  min_participants: number;
  max_participants?: number;
  auto_create: boolean;
  create_cron?: string;
  template_key?: string;
  created_at: string;
  updated_at: string;
}

export interface TournamentTemplateDetail extends TournamentTemplate {
  schedules: unknown[];
  prize_distributions: unknown[];
  tiers: unknown[];
  tier_count: number;
}

export interface ListTemplatesParams {
  page?: number;
  per_page?: number;
  market_type?: string;
  duration_type?: string;
  is_active?: string;
  search?: string;
  has_tiers?: string;
  sort?: string;
}

export async function listTournamentTemplates(params?: ListTemplatesParams): Promise<{
  templates: TournamentTemplate[];
  total: number;
  page: number;
  per_page: number;
}> {
  const response = await api.get('/api/admin/templates', { params });
  return response.data;
}

export async function getTournamentTemplate(id: string): Promise<TournamentTemplateDetail> {
  const response = await api.get(`/api/admin/templates/${id}`);
  return response.data;
}
