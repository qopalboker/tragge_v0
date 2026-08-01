import { api } from './index';

// Data point for time series
export interface FinancialDataPoint {
  date: string;
  amount_cents: number;
}

// Aggregated totals
export interface FinancialTotals {
  total_deposits_cents: number;
  total_withdrawals_cents: number;
  total_entry_fees_cents: number;
  total_prizes_paid_cents: number;
  net_revenue_cents: number;
}

// Financial summary response
export interface FinancialSummaryResponse {
  deposits: FinancialDataPoint[];
  withdrawals: FinancialDataPoint[];
  entry_fees: FinancialDataPoint[];
  prizes_paid: FinancialDataPoint[];
  net_revenue: FinancialDataPoint[];
  totals: FinancialTotals;
}

// Summary query parameters
export interface FinancialSummaryParams {
  from?: string;
  to?: string;
  granularity?: 'day' | 'week' | 'month';
}

// Deposit item
export interface DepositUser {
  id: string;
  email: string;
  username?: string;
}

export interface Deposit {
  id: string;
  user: DepositUser;
  amount_cents: number;
  currency: string;
  provider: string;
  status: string;
  created_at: string;
  completed_at?: string;
}

export interface DepositListResponse {
  deposits: Deposit[];
  total: number;
  page: number;
  per_page: number;
}

export interface DepositListParams {
  status?: string;
  user_id?: string;
  page?: number;
  limit?: number;
}

// Transaction item
export interface TransactionUser {
  id: string;
  email: string;
  username?: string;
}

export interface Transaction {
  id: string;
  user: TransactionUser;
  type: string;
  amount_cents: number;
  ref_type?: string;
  ref_id?: string;
  description?: string;
  created_at: string;
}

export interface TransactionListResponse {
  transactions: Transaction[];
  total: number;
  page: number;
  per_page: number;
}

export interface TransactionListParams {
  type?: string;
  user_id?: string;
  page?: number;
  limit?: number;
}

// API functions
export async function getFinancialSummary(params: FinancialSummaryParams = {}): Promise<FinancialSummaryResponse> {
  const searchParams = new URLSearchParams();
  if (params.from) searchParams.append('from', params.from);
  if (params.to) searchParams.append('to', params.to);
  if (params.granularity) searchParams.append('granularity', params.granularity);

  const queryString = searchParams.toString();
  const url = `/api/admin/financial/summary${queryString ? `?${queryString}` : ''}`;

  const response = await api.get<FinancialSummaryResponse>(url);
  return response.data;
}

export async function getDeposits(params: DepositListParams = {}): Promise<DepositListResponse> {
  const searchParams = new URLSearchParams();
  if (params.status) searchParams.append('status', params.status);
  if (params.user_id) searchParams.append('user_id', params.user_id);
  if (params.page) searchParams.append('page', params.page.toString());
  if (params.limit) searchParams.append('limit', params.limit.toString());

  const queryString = searchParams.toString();
  const url = `/api/admin/financial/deposits${queryString ? `?${queryString}` : ''}`;

  const response = await api.get<DepositListResponse>(url);
  return response.data;
}

export async function getTransactions(params: TransactionListParams = {}): Promise<TransactionListResponse> {
  const searchParams = new URLSearchParams();
  if (params.type) searchParams.append('type', params.type);
  if (params.user_id) searchParams.append('user_id', params.user_id);
  if (params.page) searchParams.append('page', params.page.toString());
  if (params.limit) searchParams.append('limit', params.limit.toString());

  const queryString = searchParams.toString();
  const url = `/api/admin/financial/transactions${queryString ? `?${queryString}` : ''}`;

  const response = await api.get<TransactionListResponse>(url);
  return response.data;
}

// Helper to format amount in cents to currency string
export function formatAmount(amountCents: number, currency: string = 'USD'): string {
  const amount = amountCents / 100;
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: currency,
  }).format(amount);
}

// Helper to format compact amount
export function formatCompactAmount(amountCents: number, currency: string = 'USD'): string {
  const amount = amountCents / 100;
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: currency,
    notation: 'compact',
    maximumFractionDigits: 1,
  }).format(amount);
}

// Helper to get transaction type color
export function getTransactionTypeColor(type: string): string {
  switch (type) {
    case 'deposit':
      return 'success';
    case 'withdrawal':
      return 'error';
    case 'contest_entry':
      return 'warning';
    case 'prize':
      return 'success';
    case 'admin_charge':
      return 'info';
    case 'refund':
      return 'info';
    default:
      return 'secondary';
  }
}

// Helper to get deposit status color
export function getDepositStatusColor(status: string): string {
  switch (status) {
    case 'succeeded':
      return 'success';
    case 'pending':
      return 'warning';
    case 'processing':
      return 'info';
    case 'failed':
      return 'error';
    case 'cancelled':
      return 'secondary';
    default:
      return 'secondary';
  }
}
