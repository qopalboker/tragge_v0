import type { RouteRecordRaw } from 'vue-router'

export const tradeRoutes: RouteRecordRaw[] = [
  { path: '/trade/:contestId', name: 'trading', component: () => import('./views/TradingPage.vue'), meta: { requiresAuth: true } },
  { path: '/trade/:contestId/leaderboard', name: 'trade-leaderboard', component: () => import('./views/LeaderboardPage.vue'), meta: { requiresAuth: true } },
  { path: '/trade/error', name: 'trade-error', component: () => import('./views/ErrorPage.vue') },
]
