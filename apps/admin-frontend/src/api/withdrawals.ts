import { api } from './index';
import { reauthenticationHeaders } from './reauthentication';

export enum WithdrawalStatus {
  Pending = 'pending',
  Processing = 'processing',
  Succeeded = 'succeeded',
  Failed = 'failed',
  Cancelled = 'cancelled',
  Rejected = 'rejected',
}

export interface WithdrawalUser {
  id: string;
  email: string;
  username?: string;
}

export interface WithdrawalUserDetail {
  id: string;
  email: string;
  username?: string;
  full_name?: string;
  wallet_balance: number;
  kyc_status?: string;
}

export interface Withdrawal {
  id: string;
  user: WithdrawalUser;
  amount_cents: number;
  currency: string;
  status: WithdrawalStatus;
  destination_type?: string;
  destination_info?: Record<string, unknown>;
  admin_comment?: string;
  reviewed_by?: string;
  reviewed_at?: string;
  created_at: string;
  completed_at?: string;
}

export interface WithdrawalDetail {
  id: string;
  user: WithdrawalUserDetail;
  amount_cents: number;
  currency: string;
  status: WithdrawalStatus;
  provider?: string;
  destination_type?: string;
  destination_info?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  admin_comment?: string;
  reviewed_by?: string;
  reviewer_email?: string;
  reviewed_at?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  audit_history: WithdrawalAuditEntry[];
}

export interface WithdrawalAuditEntry {
  id: string;
  action: string;
  actor_id: string;
  actor_email: string;
  details: string;
  created_at: string;
}

export interface WithdrawalListResponse {
  withdrawals: Withdrawal[];
  total: number;
  page: number;
  per_page: number;
}

export interface WithdrawalListParams {
  status?: WithdrawalStatus | '';
  user_id?: string;
  page?: number;
  limit?: number;
}

export interface WithdrawalActionRequest {
  comment: string;
}

export interface PendingCountResponse {
  count: number;
}

// Get list of withdrawals with optional filters
export async function getWithdrawals(params: WithdrawalListParams = {}): Promise<WithdrawalListResponse> {
  const searchParams = new URLSearchParams();
  if (params.status) searchParams.append('status', params.status);
  if (params.user_id) searchParams.append('user_id', params.user_id);
  if (params.page) searchParams.append('page', params.page.toString());
  if (params.limit) searchParams.append('limit', params.limit.toString());

  const queryString = searchParams.toString();
  const url = `/api/admin/withdrawals${queryString ? `?${queryString}` : ''}`;

  const response = await api.get<WithdrawalListResponse>(url);
  return response.data;
}

// Get pending withdrawals count for badge
export async function getPendingWithdrawalsCount(): Promise<number> {
  const response = await api.get<PendingCountResponse>('/api/admin/withdrawals/pending-count');
  return response.data.count;
}

// Get single withdrawal details
export async function getWithdrawal(id: string): Promise<WithdrawalDetail> {
  const response = await api.get<WithdrawalDetail>(`/api/admin/withdrawals/${id}`);
  return response.data;
}

// Approve a withdrawal
export async function approveWithdrawal(id: string, data: WithdrawalActionRequest): Promise<void> {
  await api.post(`/api/admin/withdrawals/${id}/approve`, data);
}

// Reject a withdrawal
export async function rejectWithdrawal(id: string, data: WithdrawalActionRequest): Promise<void> {
  await api.post(`/api/admin/withdrawals/${id}/reject`, data);
}

// Add a comment to a withdrawal
export async function addWithdrawalComment(id: string, data: WithdrawalActionRequest): Promise<void> {
  await api.post(`/api/admin/withdrawals/${id}/comment`, data);
}

// Complete a processing withdrawal (mark as paid)
export async function completeWithdrawal(id: string, data: { comment: string; transaction_id: string }, grant: string): Promise<void> {
  await api.post(`/api/admin/withdrawals/${id}/complete`, data, { headers: reauthenticationHeaders(grant) });
}

// Mark a processing withdrawal as failed (refunds user)
export async function failWithdrawal(id: string, data: WithdrawalActionRequest): Promise<void> {
  await api.post(`/api/admin/withdrawals/${id}/fail`, data);
}

// Helper to format amount in cents to currency string
export function formatAmount(amountCents: number, currency: string = 'USD'): string {
  const amount = amountCents / 100;
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: currency,
  }).format(amount);
}

// Helper to get status badge color class
export function getStatusColor(status: WithdrawalStatus): string {
  switch (status) {
    case WithdrawalStatus.Pending:
      return 'warning';
    case WithdrawalStatus.Processing:
      return 'info';
    case WithdrawalStatus.Succeeded:
      return 'success';
    case WithdrawalStatus.Failed:
    case WithdrawalStatus.Rejected:
      return 'error';
    case WithdrawalStatus.Cancelled:
      return 'secondary';
    default:
      return 'secondary';
  }
}

// Helper to check if withdrawal can be approved
export function canApprove(status: WithdrawalStatus): boolean {
  return status === WithdrawalStatus.Pending;
}

// Helper to check if withdrawal can be rejected
export function canReject(status: WithdrawalStatus): boolean {
  return status === WithdrawalStatus.Pending || status === WithdrawalStatus.Processing;
}

// Helper to check if withdrawal can be completed (marked as paid)
export function canComplete(status: WithdrawalStatus): boolean {
  return status === WithdrawalStatus.Processing;
}

// Helper to check if withdrawal can be marked as failed
export function canFail(status: WithdrawalStatus): boolean {
  return status === WithdrawalStatus.Processing;
}
