import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import { adminRoutes } from '@/modules/admin/routes'
import { useAuthStore } from '@/stores/auth'
import { navProgress } from '@/utils/nav-progress'

// admin-frontend routes — admin module only. User/trade routes live
// in apps/user-frontend and are served from a different origin.
const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/admin/dashboard',
  },
  ...adminRoutes,
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    redirect: '/admin/dashboard',
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Global auth guard. Unauthenticated hits on protected routes go to
// /admin/login — there is no user-login fallback because this panel
// only serves the admin module.
router.beforeEach(async (to) => {
  // Start the progress bar before any awaits so the click registers
  // visually even if the next chunk takes time to download.
  navProgress.start()

  const requiresAuth = to.matched.some(r => r.meta.requiresAuth)
  const isAuthPage = to.matched.some(r => r.meta.isAuthPage)
  const roleRecord = to.matched.find(r => r.meta.requiresRole)
  const requiresRole = roleRecord?.meta.requiresRole

  // `bootstrap()` is deduped against the call main.ts makes before
  // mount, so in-app navigations are free.
  if (requiresAuth || requiresRole || isAuthPage) {
    const auth = useAuthStore()
    if (!auth.ready) {
      await auth.bootstrap()
    }

    if (requiresAuth && !auth.isAuthenticated) {
      return { path: '/admin/login', query: { redirect: to.fullPath } }
    }

    if (requiresRole && typeof requiresRole === 'string') {
      if (!auth.hasRole(requiresRole)) {
        return '/admin/login'
      }
    }

    if (isAuthPage && auth.isAuthenticated) {
      return '/admin/dashboard'
    }
  }
})

router.afterEach(() => {
  navProgress.done()
})

router.onError(() => {
  navProgress.done()
})

export default router
