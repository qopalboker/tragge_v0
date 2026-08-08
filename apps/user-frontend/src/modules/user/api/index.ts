// User-module API functions. The shared axios instance + auth
// interceptor + 401 refresh retry live in @/api/client; this file
// re-uses that instance rather than standing up its own. The
// authFailureHandler shim is kept as a no-op for the one-release
// transition so external callers (App.vue) that still register a
// handler don't break — the shared client's onAuthFailure hook
// already handles the redirect / token clearing, so there's no work
// for the handler to do.
import { api } from '@/api/client';
import { t } from '@/i18n';

let authFailureHandler: (() => void) | null = null;
export function registerAuthFailureHandler(handler: () => void): void {
  authFailureHandler = handler;
}
// Keep a reference so linters don't flag the handler as unused; it is
// invoked by nothing today, callbacks from the shared client run via
// the `onAuthFailure` hook in @/api/client.
void authFailureHandler;

// ==================== Types ====================
export interface UserStats {
  user_id: string;
  total_contests: number;
  total_wins: number;
  total_top3: number;
  total_score: number;
  tragge_point: number;
  win_rate: number;
  avg_trade_duration_seconds: number;
  best_market?: string;
  best_market_pnl: number;
  total_trades: number;
  total_pnl: number;
}

export interface GlobalLeaderboardEntry {
  rank: number;
  user_id: string;
  username?: string;
  tragge_point: number;
  total_contests: number;
  total_wins: number;
  total_top3: number;
  win_rate: number;
}

export interface GlobalLeaderboardResponse {
  entries: GlobalLeaderboardEntry[];
  user_rank?: number;
  user_score?: number;
}

export interface ScoreHistoryEntry {
  contest_id: string;
  contest_name: string;
  rank: number;
  score: number;
  participants: number;
  pnl: number;
  trades_count: number;
  avg_trade_duration_seconds: number;
  top_symbol?: string;
  top_symbol_pnl: number;
  created_at: string;
}

// ==================== User Stats API ====================
export const userStatsApi = {
  /**
   * Get current user's stats
   */
  async getMyStats(): Promise<UserStats> {
    const response = await api.get<UserStats>('/api/user/me/stats');
    return response.data;
  },

  /**
   * Get a specific user's stats
   */
  async getUserStats(userId: string): Promise<UserStats> {
    const response = await api.get<UserStats>(`/api/user/stats/${userId}`);
    return response.data;
  },

  /**
   * Get global leaderboard
   */
  async getGlobalLeaderboard(params?: { limit?: number; offset?: number }): Promise<GlobalLeaderboardResponse> {
    const response = await api.get<GlobalLeaderboardResponse>('/api/user/global-leaderboard', { params });
    return response.data;
  },

  /**
   * Get current user's score history
   */
  async getMyScoreHistory(params?: { limit?: number; offset?: number }): Promise<{ entries: ScoreHistoryEntry[] }> {
    const response = await api.get<{ entries: ScoreHistoryEntry[] }>('/api/user/me/score-history', { params });
    return response.data;
  },

  /**
   * Get a specific user's score history
   */
  async getUserScoreHistory(userId: string, params?: { limit?: number; offset?: number }): Promise<{ entries: ScoreHistoryEntry[] }> {
    const response = await api.get<{ entries: ScoreHistoryEntry[] }>(`/api/user/score-history/${userId}`, { params });
    return response.data;
  },
};

// ==================== Free Tournaments API ====================
export interface FreeTournament {
  id: string;
  name: string;
  description?: string;
  starts_at: string;
  ends_at: string;
  status: 'registration_open' | 'scheduled' | 'running' | 'paused' | 'settling' | 'completed' | 'cancelled';
  entry_fee_cents: number;
  qty_total: number;
  rules?: Record<string, unknown>;
  symbols: { symbol: string; enabled: boolean }[];
  participant_count?: number;
}

export const freeTournamentsApi = {
  /**
   * Get free tournaments (running and scheduled)
   */
  async getFreeTournaments(limit = 5): Promise<FreeTournament[]> {
    const response = await api.get<FreeTournament[]>('/api/user/contests', {
      params: {
        is_free: true,
        status: 'running,scheduled,registration_open',
        limit,
      },
    });
    return response.data;
  },

  /**
   * Join a contest
   */
  async joinContest(contestId: string): Promise<{ contest_id: string; user_id: string; joined_at: string }> {
    const response = await api.post<{ contest_id: string; user_id: string; joined_at: string }>(
      `/api/user/contests/${contestId}/join`
    );
    return response.data;
  },
};

export interface KYCStatusResponse {
  status: 'none' | 'pending' | 'under_review' | 'verified' | 'rejected';
  verified_at?: string;
  expires_at?: string;
  rejection_reason?: string;
  // Rejection details
  rejection_fields?: string[];
  rejection_field_messages?: Record<string, string>;
  // Pre-populated data for rejected resubmission
  first_name?: string;
  last_name?: string;
  father_name?: string;
  national_code_manual?: string;
  date_of_birth?: string;
  phone?: string;
  province?: string;
  city?: string;
  address_line1?: string;
  postal_code?: string;
  document_type?: string;
  document_number?: string;
  birth_certificate_number?: string;
  birth_certificate_serial?: string;
}

export function isFieldRejected(status: KYCStatusResponse | null, fieldName: string): boolean {
  return status?.rejection_fields?.includes(fieldName) ?? false;
}

export function getFieldRejectionMessage(status: KYCStatusResponse | null, fieldName: string): string | undefined {
  return status?.rejection_field_messages?.[fieldName];
}

// ==================== KYC API ====================

export interface KYCPhoneVerifyRequest {
  national_code: string;
  mobile_number: string;
}

export interface KYCPhoneVerifyResponse {
  matched: boolean;
  message?: string;
}

export interface KYCFaceVerifyResponse {
  matched: boolean;
  liveness_score: number;
  similarity_score: number;
  message?: string;
}

export interface KYCCardVerifyResponse {
  matched: boolean;
  extracted_data: {
    first_name?: string;
    last_name?: string;
    national_code?: string;
    birth_date?: string;
  };
  message?: string;
}

export const kycApi = {
  /**
   * Get current KYC verification status
   */
  async getStatus(): Promise<KYCStatusResponse> {
    const response = await api.get<KYCStatusResponse>('/api/user/kyc/status');
    return response.data;
  },

  /**
   * Submit KYC verification documents (legacy)
   */
  async submit(formData: FormData): Promise<{ message: string }> {
    const response = await api.post<{ message: string }>('/api/user/kyc/submit', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
    return response.data;
  },

  /**
   * Step 1: Verify phone number matches national code (Jibit)
   */
  async verifyPhone(request: KYCPhoneVerifyRequest): Promise<KYCPhoneVerifyResponse> {
    const response = await api.post<KYCPhoneVerifyResponse>('/api/user/kyc/verify-phone', request);
    return response.data;
  },

  /**
   * Step 2: Verify face with selfie (Jibit liveness + face match)
   */
  async verifyFace(imageBlob: Blob, nationalCode: string): Promise<KYCFaceVerifyResponse> {
    const formData = new FormData();
    formData.append('selfie', imageBlob, 'selfie.jpg');
    formData.append('national_code', nationalCode);
    const response = await api.post<KYCFaceVerifyResponse>('/api/user/kyc/verify-face', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
    return response.data;
  },

  /**
   * Step 3: Verify national card image (Jibit OCR + data match)
   */
  async verifyCard(imageBlob: Blob, nationalCode: string): Promise<KYCCardVerifyResponse> {
    const formData = new FormData();
    formData.append('card_image', imageBlob, 'card.jpg');
    formData.append('national_code', nationalCode);
    const response = await api.post<KYCCardVerifyResponse>('/api/user/kyc/verify-card', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
    return response.data;
  },
};

// ==================== Referral Types ====================
export interface ReferralValidationResponse {
  valid: boolean;
  referrer_name?: string;
  error?: string;
}

export interface ReferrerInfo {
  name: string;
  referral_code: string;
}

// ==================== Referral API ====================
export const referralApi = {
  /**
   * Validate a referral code
   */
  async validate(code: string): Promise<ReferralValidationResponse> {
    const response = await api.get<ReferralValidationResponse>('/api/user/referral/validate', {
      params: { code },
    });
    return response.data;
  },

  /**
   * Get referrer info by code (for landing page)
   * Uses the validate endpoint which returns referrer_name when valid
   */
  async getReferrerInfo(code: string): Promise<ReferrerInfo | null> {
    try {
      const response = await api.get<{ valid: boolean; referrer_name?: string }>(
        '/api/user/referral/validate',
        { params: { code } }
      );

      if (response.data.valid && response.data.referrer_name) {
        return {
          name: response.data.referrer_name,
          referral_code: code,
        };
      }
      return null;
    } catch {
      return null;
    }
  },
};

// ==================== Affiliate Types ====================
export interface AffiliateStats {
  referral_code: string;
  total_referrals: number;
  qualified_referrals: number;
  total_earned: number;
  pending_earnings: number;
}

export type ReferralStatus = 'pending' | 'qualified';

export interface Referral {
  id: string;
  email: string;
  status: ReferralStatus;
  joined_at: string;
  qualified_at?: string;
}

// Affiliate activation status
export type AffiliateActivationStatus = 'inactive' | 'pending' | 'active' | 'rejected';

export interface AffiliateStatusStats {
  total_referrals: number;
  qualified_referrals: number;
  total_earned: number;
  pending_earnings: number;
}

export interface AffiliateStatus {
  status: AffiliateActivationStatus;
  code: string;
  stats?: AffiliateStatusStats;
  requested_at?: string;
  approved_at?: string;
}

// ==================== Profile Types ====================
export interface UserProfile {
  user_id: string;
  email: string;
  roles: string[];
  email_verified: boolean;
  username?: string;
  display_name?: string;
  avatar_url?: string;
  bio?: string;
  country?: string;
  phone?: string;
  created_at: string;
}

export interface UpdateProfileRequest {
  username?: string;
  display_name?: string;
  bio?: string;
  country?: string;
  phone?: string;
}

export interface UpdateProfileResponse {
  user_id: string;
  username?: string;
  display_name?: string;
  avatar_url?: string;
  bio?: string;
  country?: string;
  phone?: string;
}

export interface AvatarUploadResponse {
  avatar_url: string;
}

export interface SelectAvatarRequest {
  avatar_id: string;
}

export interface SelectAvatarResponse {
  avatar_id: string;
  avatar_url: string;
}

export interface PredefinedAvatar {
  id: string;
  slug: string;
  display_name: string;
  category: string;
  bg_color: string;
  path: string;
  sort_order: number;
}

export interface ListAvatarsResponse {
  avatars: PredefinedAvatar[];
}

// ==================== Profile API ====================
export const profileApi = {
  /**
   * Get current user profile
   */
  async getProfile(): Promise<UserProfile> {
    const response = await api.get<UserProfile>('/api/user/me');
    return response.data;
  },

  /**
   * Update user profile
   */
  async updateProfile(data: UpdateProfileRequest): Promise<UpdateProfileResponse> {
    const response = await api.put<UpdateProfileResponse>('/api/user/me/profile', data);
    return response.data;
  },

  /**
   * Upload avatar image
   */
  async uploadAvatar(file: File): Promise<AvatarUploadResponse> {
    const formData = new FormData();
    formData.append('avatar', file);
    const response = await api.post<AvatarUploadResponse>('/api/user/me/avatar', formData, {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
    });
    return response.data;
  },

  /**
   * Select a predefined avatar
   */
  async selectAvatar(avatarId: string): Promise<SelectAvatarResponse> {
    const response = await api.post<SelectAvatarResponse>('/api/user/me/avatar/select', {
      avatar_id: avatarId,
    });
    return response.data;
  },

  /**
   * Get list of available predefined avatars
   */
  async listAvatars(): Promise<ListAvatarsResponse> {
    const response = await api.get<ListAvatarsResponse>('/api/user/avatars');
    return response.data;
  },
};

// ==================== Account API ====================
export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
  confirm_password: string;
}

export const accountApi = {
  /**
   * Change user password
   */
  async changePassword(request: ChangePasswordRequest): Promise<{ message: string }> {
    const response = await api.put<{ message: string }>('/api/user/me/password', request);
    return response.data;
  },
};

// ==================== Affiliate API ====================
export const affiliateApi = {
  /**
   * Get affiliate stats and referral code
   */
  async getStats(): Promise<AffiliateStats> {
    const response = await api.get<AffiliateStats>('/api/user/affiliate');
    return response.data;
  },

  /**
   * Get referral list
   */
  async getReferrals(): Promise<Referral[]> {
    const response = await api.get<Referral[]>('/api/user/affiliate/referrals');
    return response.data;
  },

  /**
   * Get affiliate activation status
   */
  async getStatus(): Promise<AffiliateStatus> {
    const response = await api.get<AffiliateStatus>('/api/user/me/affiliate/status');
    return response.data;
  },

  /**
   * Request affiliate activation
   */
  async requestActivation(): Promise<{ message: string }> {
    const response = await api.post<{ message: string }>('/api/user/me/affiliate/request-activation');
    return response.data;
  },
};

// ==================== Sessions Types ====================
export interface Session {
  id: string;
  device: string;
  browser: string;
  ip_address: string;
  last_active: string;
  is_current: boolean;
}

// ==================== Sessions API ====================
export const sessionsApi = {
  /**
   * Get all active sessions for the current user
   */
  async getSessions(): Promise<Session[]> {
    const response = await api.get<{ sessions: Session[] }>('/api/user/me/sessions');
    return response.data.sessions || [];
  },

  /**
   * Revoke all sessions except the current one
   */
  async revokeAllOtherSessions(): Promise<{ message: string }> {
    const response = await api.delete<{ message: string }>('/api/user/me/sessions');
    return response.data;
  },

  /**
   * Revoke a specific session
   */
  async revokeSession(sessionId: string): Promise<{ message: string }> {
    const response = await api.delete<{ message: string }>(`/api/user/me/sessions/${sessionId}`);
    return response.data;
  },
};

// ==================== My Tournaments Types ====================
export type MyTournamentStatus = 'active' | 'upcoming' | 'completed' | 'cancelled';

export interface MyTournamentEntry {
  contest_id: string;
  contest_name: string;
  status: string;
  starts_at: string;
  ends_at: string;
  entry_fee_cents: number;
  total_score: number;
  final_rank?: number;
  final_prize_cents?: number;
  total_participants: number;
  asset_class?: string;
  duration_type?: string;
  is_free: boolean;
  qty_total: number;
  pnl_percent?: number;
}

export interface MyTournamentCounts {
  active: number;
  upcoming: number;
  completed: number;
  cancelled: number;
}

export interface MyTournamentsResponse {
  contests: MyTournamentEntry[];
  total: number;
  page: number;
  per_page: number;
  counts: MyTournamentCounts;
}

// ==================== My Tournaments API ====================
export const myTournamentsApi = {
  async getMyTournaments(params?: {
    status?: MyTournamentStatus;
    page?: number;
    per_page?: number;
  }): Promise<MyTournamentsResponse> {
    const response = await api.get<MyTournamentsResponse>('/api/user/me/tournaments', { params });
    return response.data;
  },
};

// ==================== Grouped Tournaments API ====================

export interface TournamentTierItem {
  contest_id: string;
  tier_label?: string;
  entry_fee_cents: number;
  is_free: boolean;
  prize_pool_cents: number;
  current_participants: number;
  max_participants?: number;
}

export interface TournamentGroup {
  template_id: string;
  name: string;
  description?: string;
  status: string;
  market_type: string;
  duration_type: string;
  duration_minutes: number;
  start_time: { iso: string; unix: number };
  end_time: { iso: string; unix: number };
  commission_rate: number;
  tiers: TournamentTierItem[];
}

export interface TournamentUngroupedItem {
  id: string;
  name: string;
  description?: string;
  status: string;
  market_type: string;
  duration_type: string;
  duration_minutes: number;
  start_time: { iso: string; unix: number };
  end_time: { iso: string; unix: number };
  entry_fee_cents: number;
  is_free: boolean;
  prize_pool_cents: number;
  current_participants: number;
  max_participants?: number;
  commission_rate: number;
}

export interface TournamentGroupedResponse {
  groups: TournamentGroup[];
  ungrouped: TournamentUngroupedItem[];
  total_count: number;
  server_time: { iso: string; unix: number };
}

export const groupedTournamentsApi = {
  async getGroupedTournaments(params?: {
    market_type?: string;
    duration_type?: string;
    status?: string;
  }): Promise<TournamentGroupedResponse> {
    const response = await api.get<TournamentGroupedResponse>('/api/user/tournaments', {
      params: { ...params, group_by: 'template' },
    });
    return response.data;
  },
};

export { api };

// ==================== Wallet Error Types ====================

export class WalletApiError extends Error {
  code: string;
  statusCode: number;

  constructor(message: string, code: string, statusCode: number = 500) {
    super(message);
    this.name = 'WalletApiError';
    this.code = code;
    this.statusCode = statusCode;
  }
}

// ==================== Wallet Types ====================

export type WalletStatus = 'active' | 'frozen' | 'closed';

export interface Wallet {
  user_id: string;
  balance_cents: number;
  currency: string;
  status: WalletStatus;
}

export type WalletTransactionType =
  | 'deposit'
  | 'withdrawal'
  | 'contest_entry'
  | 'contest_refund'
  | 'prize_credit'
  | 'adjustment'
  | 'affiliate_commission'
  | 'withdraw_fee'
  | 'withdrawal_refund'
  | 'withdraw_fee_refund';

export type WithdrawalStatus =
  | 'pending'
  | 'processing'
  | 'succeeded'
  | 'rejected'
  | 'failed'
  | 'cancelled';

export type WalletReasonCode =
  | 'CONTEST_ENTRY'
  | 'CONTEST_ENTRY_FREE'
  | 'CONTEST_REFUND_QUORUM'
  | 'CONTEST_REFUND_ADMIN'
  | 'CONTEST_REFUND_LEAVE'
  | 'CONTEST_PRIZE'
  | 'WALLET_TOPUP'
  | 'WALLET_WITHDRAW';

export interface WalletTransaction {
  id: string;
  type: WalletTransactionType;
  amount_cents: number;
  balance_after_cents: number;
  description: string;
  reason_code?: WalletReasonCode;
  ref_type: string;
  ref_id: string;
  created_at: string;
  status?: WithdrawalStatus;
  admin_comment?: string;
}

export interface PaymentHistory {
  transactions: WalletTransaction[];
  entries?: WalletTransaction[];
  total: number;
  has_more: boolean;
  balance_cents?: number;
  page?: number;
}

export interface HistoryParams {
  limit?: number;
  offset?: number;
  type?: WalletTransactionType;
}

// ==================== Wallet Deposit/Withdraw Types ====================

export interface FiatDepositResponse {
  payment_url: string;
  order_id: string;
}

export interface WalletCryptoDepositResponse {
  payment_url: string;
  order_id: string;
}

export interface WalletWithdrawRequest {
  amount_cents: number;
  destination_type: 'bank' | 'crypto';
  bank_details?: {
    account_holder: string;
    iban: string;
    bank_name: string;
  };
  crypto_details?: {
    address: string;
    network: string;
  };
}

export interface WalletWithdrawResponse {
  payout_id: string;
  status: string;
}

export type PayoutStatusValue =
  | 'pending'
  | 'processing'
  | 'completed'
  | 'failed'
  | 'cancelled';

export interface PayoutStatus {
  payout_id: string;
  status: PayoutStatusValue;
  amount_cents: number;
  created_at: string;
  processed_at?: string;
  failure_reason?: string;
}

// ==================== Wallet Error Handling ====================

function handleWalletApiError(err: unknown): never {
  if (err && typeof err === 'object' && 'response' in err) {
    const axiosError = err as {
      response?: {
        status?: number;
        data?: { error?: string; message?: string; code?: string };
      };
    };

    const status = axiosError.response?.status ?? 500;
    const data = axiosError.response?.data;
    const message = data?.error || data?.message || t('errors.defaultError');
    const code = data?.code || `HTTP_${status}`;

    // Handle 403 - KYC required
    if (status === 403) {
      throw new WalletApiError(
        t('wallet.kycRequiredError'),
        'KYC_REQUIRED',
        403
      );
    }

    throw new WalletApiError(message, code, status);
  }

  throw new WalletApiError(t('errors.unexpectedError'), 'UNKNOWN_ERROR', 500);
}

// ==================== Wallet API Functions ====================

export type FiatProvider = 'jibit';

async function getWallet(): Promise<Wallet> {
  try {
    const response = await api.get<Wallet>('/api/user/wallet');
    return response.data;
  } catch (err) {
    throw handleWalletApiError(err);
  }
}

async function getWalletHistory(params: HistoryParams = {}): Promise<PaymentHistory> {
  try {
    const response = await api.get<PaymentHistory>('/api/user/wallet/history', { params });
    const data = response.data;
    // Normalize: backend may return 'entries' or 'transactions'
    if (data.entries && !data.transactions) {
      data.transactions = data.entries;
    }
    return data;
  } catch (err) {
    throw handleWalletApiError(err);
  }
}

async function createFiatDeposit(
  amount_usd_cents: number,
  provider: FiatProvider = 'jibit'
): Promise<FiatDepositResponse> {
  try {
    const response = await api.post<FiatDepositResponse>('/api/payments/deposit/fiat/create', {
      amount_usd_cents,
      provider,
    });
    return response.data;
  } catch (err) {
    throw handleWalletApiError(err);
  }
}

export interface ExchangeRateResponse {
  usd_to_irr: number;
  usd_to_irt: number;
  source: string;
  fetched_at: string;
}

async function getExchangeRate(): Promise<ExchangeRateResponse> {
  try {
    const response = await api.get<ExchangeRateResponse>('/api/payments/exchange-rate');
    return response.data;
  } catch (err) {
    throw handleWalletApiError(err);
  }
}

async function createWalletCryptoDeposit(
	amount_cents: number,
	pay_currency: string = 'usdttrc20'
): Promise<WalletCryptoDepositResponse> {
	try {
		const body: Record<string, unknown> = { amount_cents, pay_currency };
    const response = await api.post<WalletCryptoDepositResponse>('/api/payments/deposit/crypto/create', body);
    return response.data;
  } catch (err) {
    throw handleWalletApiError(err);
  }
}

async function requestWithdraw(params: WalletWithdrawRequest): Promise<WalletWithdrawResponse> {
  try {
    const response = await api.post<WalletWithdrawResponse>('/api/payments/withdraw/request', params);
    return response.data;
  } catch (err) {
    throw handleWalletApiError(err);
  }
}

async function getWithdrawStatus(payout_id: string): Promise<PayoutStatus> {
  try {
    const response = await api.get<PayoutStatus>(`/api/payments/withdraw/${payout_id}/status`);
    return response.data;
  } catch (err) {
    throw handleWalletApiError(err);
  }
}

// ==================== Wallet API Object ====================

export const walletApi = {
  getWallet,
  getHistory: getWalletHistory,
  createFiatDeposit,
  createCryptoDeposit: createWalletCryptoDeposit,
  requestWithdraw,
  getWithdrawStatus,
  getExchangeRate,
};
