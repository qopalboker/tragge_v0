import type { RouteRecordRaw } from 'vue-router'
import AdminLayout from '@/components/layout/AdminLayout.vue'

export const adminRoutes: RouteRecordRaw[] = [
  // Auth — standalone (no sidebar)
  { path: '/admin/login', name: 'admin-login', component: () => import('./views/LoginPage.vue'), meta: { isAuthPage: true } },
  { path: '/admin/error', name: 'admin-error', component: () => import('./views/ErrorPage.vue') },

  // Protected — wrapped in AdminLayout
  {
    path: '/admin',
    component: AdminLayout,
    meta: { requiresAuth: true, requiresRole: 'admin' },
    children: [
      { path: '', redirect: '/admin/dashboard' },
      { path: 'dashboard', name: 'admin-dashboard', component: () => import('./views/DashboardPage.vue') },
      { path: 'contests', name: 'admin-contests', component: () => import('./views/ContestsPage.vue') },
      { path: 'contests/new', name: 'admin-contest-new', component: () => import('./views/ContestFormPage.vue') },
      { path: 'contests/:id', name: 'admin-contest-edit', component: () => import('./views/ContestFormPage.vue') },
      { path: 'contests/:id/detail', name: 'admin-contest-detail', component: () => import('./views/ContestDetailPage.vue') },
      { path: 'symbols', name: 'admin-symbols', component: () => import('./views/SymbolsPage.vue') },
      { path: 'contest-templates', name: 'admin-templates', component: () => import('./views/ContestTemplatesPage.vue') },
      { path: 'auto-scheduling', name: 'admin-scheduling', component: () => import('./views/AutoSchedulingPage.vue') },
      { path: 'tournament-templates', name: 'admin-tournament-templates', component: () => import('./views/TournamentTemplatesPage.vue') },
      { path: 'users', name: 'admin-users', component: () => import('./views/UsersPage.vue') },
      { path: 'users/:id', name: 'admin-user-detail', component: () => import('./views/UserDetailPage.vue') },
      { path: 'kyc-review', name: 'admin-kyc', component: () => import('./views/KYCReviewPage.vue') },
      { path: 'withdrawals', name: 'admin-withdrawals', component: () => import('./views/WithdrawalsPage.vue') },
      { path: 'financial', name: 'admin-financial', component: () => import('./views/FinancialPage.vue') },
      { path: 'email-templates', name: 'admin-email', component: () => import('./views/EmailTemplatesPage.vue') },
      { path: 'avatars', name: 'admin-avatars', component: () => import('./views/AvatarsPage.vue') },
      { path: 'shards', name: 'admin-shards', component: () => import('./views/ShardsPage.vue') },
      { path: 'tickets', name: 'admin-tickets', component: () => import('./views/TicketsPage.vue') },
      { path: 'tickets/:id', name: 'admin-ticket-detail', component: () => import('./views/TicketDetailPage.vue') },
      { path: 'audit', name: 'admin-audit', component: () => import('./views/AuditPage.vue') },
      { path: 'market-data', name: 'admin-market', component: () => import('./views/MarketDataPage.vue') },
    ],
  },
]
