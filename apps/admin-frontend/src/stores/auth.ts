import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import axios from 'axios';
import {
  refreshAccessToken as sharedRefresh,
  hasSessionHint,
  clearSessionHintCookie,
  clearLegacySessionHint,
  createCrossTabChannel,
  createBootstrap,
  type SessionHintConfig,
} from '@tragge/frontend-shared';
import { api, setAccessToken as setApiAccessToken } from '@/api/client';
import { useToast } from '@/composables/useToast';
import { getErrorMessage } from '@/utils/errorHandler';
import { t } from '@/i18n';

// ---------------------------------------------------------------------------
// Admin-panel auth surface
// ---------------------------------------------------------------------------
//
// Deliberately narrow: no register, no OAuth, no social login. A leaked
// admin bundle has only login + 2FA endpoints and cannot be coaxed into
// creating user accounts or running through a user-only OAuth dance. The
// user-panel has its own store in apps/user-frontend.
//
// Step 7: suffixed cookie/hint/broadcast names so the user panel Ã¢â‚¬â€
// now served from a distinct origin Ã¢â‚¬â€ can share an eTLD+1 without
// browser cookie collisions. No legacyLocalStorageKey: the admin
// panel never participated in the single-panel localStorage hint
// migration, so there's nothing to migrate from.
const SESSION_HINT: SessionHintConfig = {
  cookieName: 'tragge_session_hint_admin',
};
// Panel-scoped broadcast key so an admin logout does NOT cascade
// into a user-panel logout (and vice versa).
const CROSS_TAB_KEY = 'tragge_auth_event_admin';
const crossTab = createCrossTabChannel(CROSS_TAB_KEY);

interface AdminUser {
  id: string;
  email: string;
  roles: string[];
  username?: string;
  display_name?: string;
  avatar_url?: string;
  created_at?: string;
}

interface LoginResponse {
  access_token?: string;
  mfa_required?: boolean;
  enrollment_required?: boolean;
  challenge?: string;
  expires_at?: string;
  recovery_codes?: string[];
}

type MFAStage = 'password' | 'enroll' | 'enroll_verify' | 'verify' | 'recovery_codes';

interface MeResponse {
  user_id: string;
  email: string;
  roles: string[];
  username?: string;
  display_name?: string;
  avatar_url?: string;
  created_at: string;
}

interface PermissionsResponse {
  role: string;
  permissions: string[];
  is_viewer: boolean;
  is_admin: boolean;
  is_super_admin: boolean;
}

interface JwtPayload {
  exp?: number;
}

function decodeJwt(token: string): JwtPayload | null {
  try {
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    const payload = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(atob(payload)) as JwtPayload;
  } catch {
    return null;
  }
}

function isTokenExpired(token: string, bufferSeconds = 300): boolean {
  const decoded = decodeJwt(token);
  if (!decoded?.exp) return true;
  const now = Date.now() / 1000;
  return decoded.exp < now + bufferSeconds;
}

export const useAuthStore = defineStore('auth', () => {
  const accessToken = ref<string | null>(null);
  const user = ref<AdminUser | null>(null);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const ready = ref(false);
  const mfaStage = ref<MFAStage>('password');
  const mfaChallenge = ref<string | null>(null);
  const mfaSecret = ref<string | null>(null);
  const mfaProvisioningUri = ref<string | null>(null);
  const recoveryCodes = ref<string[]>([]);

  // Admin role + granular permissions sourced from /api/admin/me/permissions.
  const adminRole = ref<string>(''); // 'super_admin' | 'support_admin' | ''
  const adminPermissions = ref<string[]>([]);

  const isAuthenticated = computed(() => !!accessToken.value);
  const isSuperAdmin = computed(() => adminRole.value === 'super_admin');
  const isAdminRole = computed(
    () => adminRole.value === 'support_admin' || adminRole.value === 'super_admin',
  );
  const isViewer = computed(() => false);

  function hasRole(role: string): boolean {
    // Protected Admin routes use `admin` as a panel-access capability, not as
    // an identity role. Identity remains restricted to the canonical
    // SUPPORT_ADMIN / SUPER_ADMIN roles returned by the Admin API.
    if (role === 'admin') return isAdminRole.value;
    return user.value?.roles?.includes(role) ?? false;
  }

  function hasPermission(permission: string): boolean {
    if (adminRole.value === 'super_admin') return true;
    return adminPermissions.value.includes(permission);
  }

  function setTokens(access: string | null): void {
    accessToken.value = access;
    setApiAccessToken(access);

    if (access) {
      clearLegacySessionHint(SESSION_HINT);
    } else {
      clearSessionHintCookie(SESSION_HINT);
      clearLegacySessionHint(SESSION_HINT);
      crossTab.broadcastLogout();
    }
  }

  function setUser(userData: AdminUser | null): void {
    user.value = userData;
  }

  async function refreshAccessToken(): Promise<boolean> {
    const result = await sharedRefresh({
      endpoint: '/api/admin/auth/refresh',
    });
    if (result.ok) {
      setTokens(result.accessToken);
      return true;
    }
    if (result.error.kind === 'auth') {
      setTokens(null);
    }
    return false;
  }

  async function ensureValidToken(): Promise<boolean> {
    const currentAccessToken = accessToken.value;
    if (!currentAccessToken || isTokenExpired(currentAccessToken)) {
      return await refreshAccessToken();
    }
    return true;
  }

  async function fetchUser(throwOnError = false): Promise<void> {
    if (!accessToken.value) return;
    try {
      const response = await api.get<MeResponse>('/api/admin/me');
      setUser({
        id: response.data.user_id,
        email: response.data.email,
        roles: response.data.roles,
        username: response.data.username,
        display_name: response.data.display_name,
        avatar_url: response.data.avatar_url,
        created_at: response.data.created_at,
      });
    } catch (err) {
      logout();
      if (throwOnError) throw err;
    }
  }

  async function fetchPermissions(): Promise<void> {
    if (!accessToken.value) {
      adminRole.value = '';
      adminPermissions.value = [];
      return;
    }
    try {
      const response = await api.get<PermissionsResponse>('/api/admin/me/permissions');
      adminRole.value = response.data.role;
      adminPermissions.value = response.data.permissions || [];
    } catch {
      adminRole.value = '';
      adminPermissions.value = [];
    }
  }

  async function login(email: string, password: string): Promise<boolean> {
    const toast = useToast();
    loading.value = true;
    error.value = null;

    try {
      const response = await api.post<LoginResponse>('/api/admin/auth/login', { email, password });
      if (response.data.mfa_required && response.data.challenge) {
        mfaChallenge.value = response.data.challenge;
        mfaStage.value = response.data.enrollment_required ? 'enroll' : 'verify';
        return false;
      }
      if (!response.data.access_token) throw new Error('authentication response was incomplete');
      setTokens(response.data.access_token);
      await Promise.all([fetchUser(true), fetchPermissions()]);
      toast.success(t('auth.loginSuccess'));
      return true;
    } catch (err: unknown) {
      const message = getErrorMessage(err, t('auth.loginError'));
      error.value = message;
      toast.error(message);
      return false;
    } finally {
      loading.value = false;
    }
  }

  async function startMFAEnrollment(): Promise<boolean> {
    if (mfaStage.value !== 'enroll' || !mfaChallenge.value) return false;
    loading.value = true;
    error.value = null;
    try {
      const response = await api.post<{ challenge: string; secret: string; provisioning_uri: string }>(
        '/api/admin/auth/mfa/enrollment/start',
        { challenge: mfaChallenge.value },
      );
      mfaChallenge.value = response.data.challenge;
      mfaSecret.value = response.data.secret;
      mfaProvisioningUri.value = response.data.provisioning_uri;
      mfaStage.value = 'enroll_verify';
      return true;
    } catch (err: unknown) {
      error.value = getErrorMessage(err, t('auth.mfaError'));
      return false;
    } finally {
      loading.value = false;
    }
  }

  async function verifyMFA(value: string, recovery = false): Promise<boolean> {
    if (!mfaChallenge.value || !['enroll_verify', 'verify'].includes(mfaStage.value)) return false;
    loading.value = true;
    error.value = null;
    try {
      const enrollment = mfaStage.value === 'enroll_verify';
      const endpoint = enrollment ? '/api/admin/auth/mfa/enrollment/verify' : '/api/admin/auth/mfa/verify';
      const response = await api.post<LoginResponse>(endpoint, {
        challenge: mfaChallenge.value,
        ...(recovery ? { recovery_code: value } : { code: value }),
      });
      if (!response.data.access_token) throw new Error('authentication response was incomplete');
      setTokens(response.data.access_token);
      recoveryCodes.value = response.data.recovery_codes ?? [];
      mfaChallenge.value = null;
      mfaSecret.value = null;
      mfaProvisioningUri.value = null;
      mfaStage.value = recoveryCodes.value.length > 0 ? 'recovery_codes' : 'password';
      await Promise.all([fetchUser(true), fetchPermissions()]);
      return true;
    } catch (err: unknown) {
      error.value = getErrorMessage(err, t('auth.mfaError'));
      return false;
    } finally {
      loading.value = false;
    }
  }

  function acknowledgeRecoveryCodes(): void {
    recoveryCodes.value = [];
    mfaStage.value = 'password';
  }

  function cancelMFA(): void {
    mfaStage.value = 'password';
    mfaChallenge.value = null;
    mfaSecret.value = null;
    mfaProvisioningUri.value = null;
    recoveryCodes.value = [];
    error.value = null;
  }

  function logout(): void {
    const currentToken = accessToken.value;

    setTokens(null);
    setUser(null);
    adminRole.value = '';
    adminPermissions.value = [];
    cancelMFA();

    if (currentToken) {
      axios
        .post(
          '/api/admin/logout',
          {},
          {
            withCredentials: true,
            headers: {
              Authorization: `Bearer ${currentToken}`,
              'X-Requested-With': 'XMLHttpRequest',
            },
          },
        )
        .catch(() => {
          /* ignore Ã¢â‚¬â€ TTL will eventually invalidate */
        });
    }
  }

  function clearError(): void {
    error.value = null;
  }

  // Cross-tab logout propagation with self-heal: verify our own
  // session is actually dead before acting on a peer's broadcast.
  if (typeof window !== 'undefined') {
    crossTab.onRemoteLogout(async () => {
      if (!accessToken.value && !user.value) return;
      try {
        const ok = await ensureValidToken();
        if (ok && accessToken.value) return;
      } catch {
        /* fall through */
      }

      accessToken.value = null;
      setApiAccessToken(null);
      setUser(null);
      adminRole.value = '';
      adminPermissions.value = [];
      ready.value = true;

      const { default: router } = await import('@/router');
      const route = router.currentRoute.value;
      const onProtectedRoute = route.matched.some((r) => r.meta.requiresAuth);
      if (onProtectedRoute) {
        router.push({ name: 'admin-login', query: { redirect: route.fullPath } });
      }
    });
  }

  // Restore the session from the httpOnly refresh cookie on app
  // startup, deduped so main.ts and the first router guard don't
  // fire two parallel refresh requests.
  const bootstrap = createBootstrap(async () => {
    if (!hasSessionHint(SESSION_HINT)) {
      ready.value = true;
      return;
    }
    try {
      const valid = await ensureValidToken();
      if (!valid || !accessToken.value) return;
      await Promise.all([fetchUser(), fetchPermissions()]);
    } finally {
      ready.value = true;
    }
  });

  return {
    accessToken,
    user,
    loading,
    error,
    ready,
    mfaStage,
    mfaSecret,
    mfaProvisioningUri,
    recoveryCodes,
    adminRole,
    adminPermissions,
    isAuthenticated,
    isSuperAdmin,
    isAdminRole,
    isViewer,
    setTokens,
    login,
    startMFAEnrollment,
    verifyMFA,
    acknowledgeRecoveryCodes,
    cancelMFA,
    logout,
    fetchUser,
    fetchPermissions,
    refreshAccessToken,
    ensureValidToken,
    bootstrap,
    hasRole,
    hasPermission,
    clearError,
  };
});
