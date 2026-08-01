import type { RouteRecordRaw } from 'vue-router'

export const userRoutes: RouteRecordRaw[] = [
  // Auth pages — standalone (no layout wrapper)
  { path: '/user/login', name: 'login', component: () => import('./views/LoginPage.vue'), meta: { isAuthPage: true } },
  { path: '/user/register', redirect: '/user/login' },
  { path: '/user/forgot-password', name: 'forgot-password', component: () => import('./views/ForgotPasswordPage.vue'), meta: { isAuthPage: true } },
  { path: '/user/verify-email', redirect: '/user/dashboard' },
  { path: '/user/auth/google/callback', name: 'oauth-callback', component: () => import('./views/OAuthCallbackPage.vue'), meta: { isAuthPage: true } },
  { path: '/user/invite', name: 'invite', component: () => import('./views/ReferralLandingPage.vue'), meta: { isAuthPage: true } },
  { path: '/payment/result', name: 'payment-result', component: () => import('./views/PaymentResultPage.vue') },
  { path: '/user/error', name: 'user-error', component: () => import('./views/ErrorPage.vue') },

  // Protected pages — wrapped in UserLayout (sidebar + bottom nav)
  {
    path: '/user',
    component: () => import('./components/layout/UserLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/user/dashboard' },
      { path: 'dashboard', name: 'dashboard', component: () => import('./views/DashboardPage.vue') },
      { path: 'contests', name: 'contests', component: () => import('./views/ContestsPage.vue') },
      { path: 'contests/:contestId', name: 'contest-details', component: () => import('./views/ContestDetailsPage.vue') },
      { path: 'contests/:contestId/results', name: 'contest-results', component: () => import('./views/ContestResultsPage.vue') },
      { path: 'my-tournaments', name: 'my-tournaments', component: () => import('./views/MyTournamentsPage.vue') },
      { path: 'my-contests', name: 'my-contests', component: () => import('./views/ContestHistoryPage.vue') },
      { path: 'leaderboard', name: 'leaderboard', component: () => import('./views/LeaderboardPage.vue') },
      { path: 'leaderboard/global', name: 'global-leaderboard', component: () => import('./views/GlobalLeaderboardPage.vue') },
      { path: 'profile', name: 'profile', component: () => import('./views/ProfilePage.vue') },
      { path: 'settings', name: 'settings', component: () => import('./views/SettingsPage.vue') },
      { path: 'notifications', name: 'notifications', component: () => import('./views/NotificationsPage.vue') },
      { path: 'wallet', name: 'wallet', component: () => import('./views/WalletPage.vue') },
      { path: 'affiliate', name: 'affiliate', component: () => import('./views/AffiliatePage.vue') },
      { path: 'profile/verify', name: 'verify-profile', component: () => import('./views/VerificationPage.vue') },
      { path: 'kyc/verify', name: 'kyc-verify', component: () => import('./views/KYCVerificationPage.vue') },
      { path: 'tickets', name: 'tickets', component: () => import('./views/TicketsPage.vue') },
      { path: 'tickets/new', name: 'ticket-new', component: () => import('./views/NewTicketPage.vue') },
      { path: 'tickets/:ticketId', name: 'ticket-chat', component: () => import('./views/TicketChatPage.vue') },
    ],
  },
]
