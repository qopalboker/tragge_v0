import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import { i18n, initializeI18n } from './i18n'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore } from '@/stores/theme'
import { installConsoleRedaction } from '@tragge/frontend-shared'
// Ensure the API client module loads so its interceptors are wired
// before any code calls the auth store (which uses `api` internally).
import '@/api/client'
import '@tragge/frontend-shared/styles/main.css'
import '@/styles/nav-progress.css'

installConsoleRedaction()
initializeI18n()

async function start() {
  const app = createApp(App)
  const pinia = createPinia()

  app.use(pinia)
  app.use(router)
  app.use(i18n)

  // Apply the persisted theme to <html> on first paint and wire cross-tab
  // sync. Must run after pinia is installed.
  useThemeStore().initTheme()

  // Restore the admin session from the httpOnly refresh cookie before
  // the first router guard runs. Without this, a hard refresh on a
  // protected admin page evaluates `isAuthenticated` against an empty
  // Pinia store and bounces to /admin/login even when the cookie is
  // still valid.
  const auth = useAuthStore()
  await auth.bootstrap()
  await router.isReady()

  app.mount('#app')
}

start()
