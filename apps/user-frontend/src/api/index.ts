// Barrel for the user-frontend's HTTP surface.
//
// - `api` is the single shared axios instance with auth-bearing
//   interceptor + 401 refresh-retry. Defined in ./client, re-exported
//   here for the `import { api } from '@/api'` call sites.
// - Trade and user module APIs re-exported so consumers have one
//   import path.

export { api, getAccessToken, setAccessToken } from './client';

// Trade API — order, position, leaderboard, contest endpoints.
export * from '../modules/trade/api/index';

// User API — wallet, KYC, profile, affiliate, etc.
export {
  walletApi,
  kycApi,
  profileApi,
  accountApi,
  affiliateApi,
  sessionsApi,
  userStatsApi,
  freeTournamentsApi,
  myTournamentsApi,
  referralApi,
  WalletApiError,
  isFieldRejected,
  getFieldRejectionMessage,
  type Wallet,
  type WalletTransaction,
  type WalletTransactionType,
  type FiatDepositResponse,
  type WalletCryptoDepositResponse,
  type WalletWithdrawRequest,
  type PayoutStatus,
  type KYCStatusResponse,
	type KYCPhoneVerifyResponse,
	type KYCFaceVerifyResponse,
	type KYCCardVerifyResponse,
	type UserStats,
  type GlobalLeaderboardEntry,
  type ScoreHistoryEntry,
  type MyTournamentEntry,
  type MyTournamentStatus,
  type MyTournamentCounts,
  type AffiliateStats,
  type AffiliateStatus,
  type Referral,
  type UpdateProfileRequest,
} from '../modules/user/api/index';
