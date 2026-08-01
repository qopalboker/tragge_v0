import { api } from './index';

// Types
export interface UserMetrics {
  total: number;
  new_today: number;
  new_this_week: number;
  new_this_month: number;
  verified_count: number;
}

export interface ContestMetrics {
  total: number;
  active_now: number;
  scheduled: number;
  completed_today: number;
}

export interface FinancialMetrics {
  total_deposits_today_cents: number;
  total_withdrawals_today_cents: number;
  pending_withdrawals_count: number;
  total_revenue_cents: number;
}

export interface TradingMetrics {
  active_traders_now: number;
  orders_today: number;
  trades_today: number;
}

export interface KYCMetrics {
  pending_count: number;
}

export interface AffiliateMetrics {
  pending_activation_count: number;
}

export interface DashboardMetrics {
  users: UserMetrics;
  contests: ContestMetrics;
  financial: FinancialMetrics;
  trading: TradingMetrics;
  kyc: KYCMetrics;
  affiliate: AffiliateMetrics;
}

// API functions
export async function getDashboardMetrics(): Promise<DashboardMetrics> {
  const response = await api.get<DashboardMetrics>('/api/admin/dashboard');
  return response.data;
}
