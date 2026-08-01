import { api } from './index';

// Types
export interface EntryTier {
  id: string;
  template_id: string;
  entry_fee: number;
  label?: string;
  sort_order: number;
  is_active: boolean;
  is_free: boolean;
  qty_total_override?: number;
  max_participants_override?: number;
  commission_rate_override?: number;
  has_prize_override: boolean;
  prize_distributions?: PrizeDistribution[];
  created_at: string;
  updated_at: string;
}

export interface PrizeDistribution {
  id: string;
  rank: number;
  percentage: number;
  min_participants: number;
}

export interface CreateTierRequest {
  entry_fee: number;
  label?: string;
  sort_order: number;
  is_free: boolean;
  qty_total_override?: number;
  max_participants_override?: number;
  commission_rate_override?: number;
  has_prize_override?: boolean;
  prize_distributions?: { rank: number; percentage: number; min_participants: number }[];
}

export interface UpdateTierRequest {
  entry_fee?: number;
  label?: string;
  sort_order?: number;
  is_active?: boolean;
  is_free?: boolean;
  qty_total_override?: number;
  max_participants_override?: number;
  commission_rate_override?: number;
  has_prize_override?: boolean;
  prize_distributions?: { rank: number; percentage: number; min_participants: number }[];
}

// API functions

export async function listTiers(templateId: string): Promise<{ tiers: EntryTier[]; total: number }> {
  const response = await api.get<{ tiers: EntryTier[]; total: number }>(
    `/api/admin/templates/${templateId}/tiers`
  );
  return response.data;
}

export async function createTier(templateId: string, data: CreateTierRequest): Promise<EntryTier> {
  const response = await api.post<EntryTier>(
    `/api/admin/templates/${templateId}/tiers`,
    data
  );
  return response.data;
}

export async function bulkCreateTiers(
  templateId: string,
  tiers: CreateTierRequest[]
): Promise<{ tiers: EntryTier[]; created: number }> {
  const response = await api.post<{ tiers: EntryTier[]; created: number }>(
    `/api/admin/templates/${templateId}/tiers/bulk`,
    { tiers }
  );
  return response.data;
}

export async function updateTier(tierId: string, data: UpdateTierRequest): Promise<EntryTier> {
  const response = await api.put<EntryTier>(`/api/admin/tiers/${tierId}`, data);
  return response.data;
}

export async function deleteTier(tierId: string): Promise<void> {
  await api.delete(`/api/admin/tiers/${tierId}`);
}
