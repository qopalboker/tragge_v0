import { api } from './index';
import { reauthenticationHeaders } from './reauthentication';

// Types
export interface User {
  id: string;
  email: string;
  roles: string[];
  status: string;
  kyc_status?: string;
  created_at: string;
}

// Comprehensive user detail types
export interface UserBasicInfo {
  id: string;
  email: string;
  username?: string;
  display_name?: string;
  avatar_url?: string;
  created_at: string;
  email_verified: boolean;
  status: string;
  country?: string;
}

export interface UserKYCInfo {
  status: string;
  submitted_at?: string;
  reviewed_at?: string;
}

export interface UserWalletInfo {
  balance_cents: number;
  currency: string;
  status: string;
}

export interface UserStats {
  total_contests: number;
  total_wins: number;
  tragge_point: number;
  total_trades: number;
  total_pnl: number;
}

export interface UserContestEntry {
  id: string;
  name: string;
  rank?: number;
  pnl: number;
  date: string;
}

export interface UserTransaction {
  id: string;
  type: string;
  amount: number;
  date: string;
  description?: string;
  reason_code?: string;
  ref_type?: string;
  ref_id?: string;
  balance_after: number;
}

export interface AdminWalletHistoryEntry {
  id: string;
  type: string;
  amount_cents: number;
  balance_after_cents: number;
  description?: string;
  reason_code?: string;
  ref_type?: string;
  ref_id?: string;
  idempotency_key?: string;
  created_at: string;
}

export interface AdminWalletHistoryResponse {
  entries: AdminWalletHistoryEntry[];
  total: number;
  balance_cents: number;
  currency: string;
  wallet_status: string;
  page: number;
  has_more: boolean;
}

export interface AdminWalletHistoryParams {
  limit?: number;
  offset?: number;
  page?: number;
  type?: string;
}

export interface UserAffiliateInfo {
  code?: string;
  status: string;
  total_referrals: number;
  total_earned: number;
}

export interface UserSessionInfo {
  id: string;
  device: string;
  ip: string;
  last_active: string;
}

export interface UserDetail {
  user: UserBasicInfo;
  roles: string[];
  kyc: UserKYCInfo;
  wallet: UserWalletInfo;
  stats: UserStats;
  recent_contests: UserContestEntry[];
  recent_transactions: UserTransaction[];
  affiliate: UserAffiliateInfo;
  sessions: UserSessionInfo[];
}

// Legacy UserDetail for backwards compatibility with UsersPage
export interface UserDetailLegacy {
  id: string;
  email: string;
  roles: string[];
  status: string;
  kyc_status?: string;
  kyc_verified_at?: string;
  contest_count: number;
  total_pnl: number;
  last_login_at?: string;
  created_at: string;
}

export interface UserListResponse {
  users: User[];
  total: number;
  limit: number;
  offset: number;
}

export interface UserListParams {
  limit?: number;
  offset?: number;
  search?: string;
  role?: string;
  status?: string;
}

export interface UpdateRolesRequest {
  roles: string[];
  reason: string;
}

export interface UpdateStatusRequest {
  status: 'active' | 'suspended';
  reason?: string;
}

export interface BanUserRequest {
  reason: string;
  duration: 'permanent' | '7d' | '30d';
}

export interface UnbanUserRequest {
  reason?: string;
}

export interface ChargeWalletRequest {
  amount: number; // Amount in cents
  reason: string;
}

export interface ChargeWalletResponse {
  message: string;
  new_balance: number;
  ledger_entry_id?: string;
}

export interface CreateUserRequest {
  email: string;
  password?: string;
  display_name?: string;
  roles: string[];
  email_verified: boolean;
  reason?: string;
}

export interface CreateUserResponse {
  user_id: string;
  email: string;
  roles: string[];
  temporary_password?: string;
  message: string;
}

// API functions

export async function createUser(data: CreateUserRequest, grant?: string): Promise<CreateUserResponse> {
  const response = await api.post<CreateUserResponse>('/api/admin/users', data, grant ? { headers: reauthenticationHeaders(grant) } : undefined);
  return response.data;
}

export async function getUsers(params: UserListParams = {}): Promise<UserListResponse> {
  const searchParams = new URLSearchParams();
  if (params.limit) searchParams.append('limit', params.limit.toString());
  if (params.offset) searchParams.append('offset', params.offset.toString());
  if (params.search) searchParams.append('search', params.search);
  if (params.role) searchParams.append('role', params.role);
  if (params.status) searchParams.append('status', params.status);

  const queryString = searchParams.toString();
  const url = queryString ? `/api/admin/users?${queryString}` : '/api/admin/users';

  const response = await api.get<UserListResponse>(url);
  return response.data;
}

export async function getUser(userId: string): Promise<UserDetail> {
  const response = await api.get<UserDetail>(`/api/admin/users/${userId}`);
  return response.data;
}

export async function updateUserRoles(userId: string, data: UpdateRolesRequest, grant: string): Promise<void> {
  await api.patch(`/api/admin/users/${userId}/roles`, data, { headers: reauthenticationHeaders(grant) });
}

export async function resetSuperAdminMFA(userId: string, reason: string, grant: string): Promise<void> {
  await api.post(`/api/admin/users/${userId}/mfa/reset`, { reason }, { headers: reauthenticationHeaders(grant) });
}

export async function updateUserStatus(userId: string, data: UpdateStatusRequest): Promise<void> {
  await api.patch(`/api/admin/users/${userId}/status`, data);
}

export async function banUser(userId: string, data: BanUserRequest): Promise<void> {
  await api.post(`/api/admin/users/${userId}/ban`, data);
}

export async function unbanUser(userId: string, data?: UnbanUserRequest): Promise<void> {
  await api.post(`/api/admin/users/${userId}/unban`, data || {});
}

export async function terminateUserSessions(userId: string): Promise<{ sessions_terminated: number }> {
  const response = await api.post<{ message: string; sessions_terminated: number }>(
    `/api/admin/users/${userId}/sessions/terminate`
  );
  return response.data;
}

export async function chargeUserWallet(userId: string, data: ChargeWalletRequest, grant: string): Promise<ChargeWalletResponse> {
  const response = await api.post<ChargeWalletResponse>(`/api/admin/users/${userId}/wallet/charge`, data, {
    headers: reauthenticationHeaders(grant),
  });
  return response.data;
}

export async function getUserWalletHistory(
  userId: string,
  params: AdminWalletHistoryParams = {}
): Promise<AdminWalletHistoryResponse> {
  const { data } = await api.get<AdminWalletHistoryResponse>(
    `/api/admin/users/${userId}/wallet/history`,
    { params }
  );
  return data;
}
