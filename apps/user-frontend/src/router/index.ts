import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { userRoutes } from '@/modules/user/routes'
import { tradeRoutes } from '@/modules/trade/routes'

// user-frontend routes — user + trade modules only. Admin routes live
// in apps/admin-frontend and are served from a different origin.
const routes: RouteRecordRaw[] = [
  ...userRoutes,
  ...tradeRoutes,
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/modules/user/views/NotFoundPage.vue'),
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Global auth guard. No admin branch — an unauthenticated hit on any
// protected route goes to /user/login. The pre-split code picked
// between /user/login and /admin/login based on `to.path`, which is
// no longer meaningful here.
router.beforeEach(async (to) => {
  const requiresAuth = to.matched.some(r => r.meta.requiresAuth)
  const isAuthPage = to.matched.some(r => r.meta.isAuthPage)
  const roleRecord = to.matched.find(r => r.meta.requiresRole)
  const requiresRole = roleRecord?.meta.requiresRole

  // Only load the store when we actually need it (lazy import keeps the
  // landing-page chunk small). `bootstrap()` is deduped against the
  // call main.ts makes before mount, so in-app navigations are cheap.
  if (requiresAuth || requiresRole || isAuthPage) {
    const { useAuthStore } = await import('@/stores/auth')
    const auth = useAuthStore()
    if (!auth.ready) {
      await auth.bootstrap()
    }

    if (requiresAuth && !auth.isAuthenticated) {
      return { path: '/user/login', query: { redirect: to.fullPath } }
    }

    if (requiresRole && typeof requiresRole === 'string') {
      if (!auth.hasRole(requiresRole)) {
        return '/'
      }
    }

    // Redirect authenticated users away from auth pages (login,
    // register, forgot-password).
    if (isAuthPage && auth.isAuthenticated) {
      return '/user/dashboard'
    }
  }
})

export default router
