import { defineStore } from 'pinia';
import { ref, computed } from 'vue';
import axios from 'axios';
import * as Sentry from '@sentry/vue';
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
import { handleGoogleCallback, clearStoredState } from '@/services/oauth';
import type { GoogleCallbackResponse } from '@/services/oauth';
import { t } from '@/i18n';

// ---------------------------------------------------------------------------
// User-panel auth surface
// ---------------------------------------------------------------------------
//
// Deliberately narrow: no adminLogin, no fetchPermissions, no admin role
// computeds. A leaked user-frontend bundle cannot be coerced into
// logging into the admin panel because the admin endpoints and cookie
// names simply aren't in this file. The admin panel has its own store
// in apps/admin-frontend with its own endpoints and cookies.
//
// Step 7: suffixed cookie/hint/broadcast names so the admin panel —
// now served from a distinct origin — can share an eTLD+1 without
// browser cookie collisions. Dropping legacyLocalStorageKey is
// deliberate: it existed only to migrate from pre-cookie storage
// during the single-panel era and has no role across origin splits
// (a cross-origin admin tab cannot read the user panel's
// localStorage anyway).
const SESSION_HINT: SessionHintConfig = {
  cookieName: 'tragge_session_hint_user',
};
// Panel-scoped broadcast key: a user logout in one user-panel tab
// must NOT trigger an admin-panel tab to think it was logged out,
// and vice versa. The admin store uses tragge_auth_event_admin.
const CROSS_TAB_KEY = 'tragge_auth_event_user';
const crossTab = createCrossTabChannel(CROSS_TAB_KEY);

export type OAuthProvider = 'google' | 'github' | 'facebook' | 'apple' | 'discord';

export interface OAuthAccount {
  provider: OAuthProvider;
  provider_user_id: string;
  email?: string;
  linked_at: string;
}

interface User {
  id: string;
  email: string;
  roles: string[];
  email_verified: boolean;
  phone_verified?: boolean;
  username?: string;
  display_name?: string;
  avatar_url?: string;
  bio?: string;
  country?: string;
  phone?: string;
  created_at?: string;
  oauth_providers?: OAuthAccount[];
  has_password?: boolean;
}

interface LoginResponse {
  access_token: string;
  requires_verification?: boolean;
  available_methods?: string[];
  masked_email?: string;
  masked_phone?: string;
}

interface RegisterResponse {
  access_token: string;
  user_id: string;
  referred_by?: string;
  requires_verification?: boolean;
  available_methods?: string[];
  masked_email?: string;
  masked_phone?: string;
}

interface RegisterRequest {
  email: string;
  password: string;
  country: string;
  ref?: string;
  agree_terms: boolean;
  age_confirm: boolean;
  captcha_token?: string;
}

export function normalizeRegistrationCountry(value: string): string | null {
  const normalized = value.trim().toUpperCase();
  return /^[A-Z]{2}$/.test(normalized) ? normalized : null;
}
interface RegisterProfileData {
  username?: string;
  display_name?: string;
  country: string;
  phone?: string;
}

interface MeResponse {
  user_id: string;
  email: string;
  roles: string[];
  email_verified: boolean;
  phone_verified?: boolean;
  username?: string;
  display_name?: string;
  avatar_url?: string;
  bio?: string;
  country?: string;
  phone?: string;
  created_at: string;
  oauth_providers?: OAuthAccount[];
  has_password?: boolean;
}

interface JwtPayload {
  exp?: number;
  user_id?: string;
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
  const user = ref<User | null>(null);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const referredByName = ref<string | null>(null);
  // `ready` flips to true after bootstrap finishes (success or failure).
  // Router guards rely on this to avoid redirecting to /login while the
  // silent refresh from the httpOnly cookie is still in flight.
  const ready = ref(false);

  const lastAuthResponse = ref<{
    requires_verification?: boolean;
    available_methods?: string[];
    masked_email?: string;
    masked_phone?: string;
  } | null>(null);

  const oauthLoginResult = ref<{
    is_new_user: boolean;
    was_linked: boolean;
    has_password: boolean;
  } | null>(null);

  const isAuthenticated = computed(() => !!accessToken.value);

  function hasRole(role: string): boolean {
    return user.value?.roles?.includes(role) ?? false;
  }

  function setTokens(access: string | null): void {
    accessToken.value = access;
    setApiAccessToken(access);

    if (access) {
      // Success path: the server just issued a fresh session-hint
      // cookie in the same response. Evict the legacy localStorage
      // fallback so we stop consulting it.
      clearLegacySessionHint(SESSION_HINT);
    } else {
      clearSessionHintCookie(SESSION_HINT);
      clearLegacySessionHint(SESSION_HINT);
      crossTab.broadcastLogout();
    }
  }

  function setUser(userData: User | null): void {
    user.value = userData;
    if (userData) {
      Sentry.setUser({
        id: userData.id,
      });
    } else {
      Sentry.setUser(null);
    }
  }

  // Refresh the access token via the httpOnly cookie. Only invalidates
  // the session on explicit 401/403; transient failures (network,
  // 5xx) preserve cookies/hint so the next interaction can recover.
  async function refreshAccessToken(): Promise<boolean> {
    const result = await sharedRefresh({
      endpoint: '/api/user/auth/refresh',
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

  async function login(email: string, password: string): Promise<boolean> {
    const toast = useToast();
    loading.value = true;
    error.value = null;

    try {
      const response = await api.post<LoginResponse>('/api/user/auth/login', {
        email,
        password,
      });

      const {
        access_token,
        requires_verification,
        available_methods,
        masked_email,
        masked_phone,
      } = response.data;
      setTokens(access_token);

      lastAuthResponse.value = requires_verification
        ? { requires_verification, available_methods, masked_email, masked_phone }
        : null;

      await fetchUser(true);
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

  async function register(
    email: string,
    password: string,
    profileData: RegisterProfileData,
    agreeTerms?: boolean,
    ageConfirm?: boolean,
    referralCode?: string,
    captchaToken?: string,
  ): Promise<boolean> {
    const toast = useToast();
    loading.value = true;
    error.value = null;
    referredByName.value = null;

    try {
      const country = normalizeRegistrationCountry(profileData.country);
      if (!country) {
        error.value = t('auth.errorRequired');
        return false;
      }
      const payload: RegisterRequest = {
        email,
        password,
        country,
        agree_terms: agreeTerms ?? false,
        age_confirm: ageConfirm ?? false,
      };
      if (referralCode) payload.ref = referralCode;
      if (captchaToken) payload.captcha_token = captchaToken;

      const response = await api.post<RegisterResponse>('/api/user/auth/register', payload);

      const {
        access_token,
        referred_by,
        requires_verification,
        available_methods,
        masked_email,
        masked_phone,
      } = response.data;
      setTokens(access_token);

      lastAuthResponse.value = requires_verification
        ? { requires_verification, available_methods, masked_email, masked_phone }
        : null;

      if (referred_by) referredByName.value = referred_by;

      if (profileData) {
        try {
          await api.put('/api/user/me/profile', profileData);
          // If a phone was saved, the backend now has SMS as an
          // additional verification method. Reflect it in the modal
          // so the user sees the choice immediately rather than after
          // a refetch.
          if (profileData.phone && lastAuthResponse.value) {
            if (!lastAuthResponse.value.available_methods?.includes('sms')) {
              lastAuthResponse.value.available_methods = [
                ...(lastAuthResponse.value.available_methods || []),
                'sms',
              ];
            }
            const ph = profileData.phone;
            lastAuthResponse.value.masked_phone =
              ph.length <= 4 ? '***'
              : ph.length <= 7 ? ph.slice(0, 2) + '***' + ph.slice(-2)
              : ph.slice(0, 4) + '***' + ph.slice(-2);
          }
        } catch {
          // User is registered and logged in — they can update their
          // profile from the profile page later.
          console.warn('Profile update after registration failed');
        }
      }

      await fetchUser(true);
      toast.success(t('auth.registrationSuccess'));
      return true;
    } catch (err: unknown) {
      const message = getErrorMessage(err, t('auth.registrationError'));
      error.value = message;
      toast.error(message);
      return false;
    } finally {
      loading.value = false;
    }
  }

  async function fetchUser(throwOnError = false): Promise<void> {
    if (!accessToken.value) return;

    try {
      const response = await api.get<MeResponse>('/api/user/me');
      setUser({
        id: response.data.user_id,
        email: response.data.email,
        roles: response.data.roles,
        email_verified: response.data.email_verified,
        phone_verified: response.data.phone_verified,
        username: response.data.username,
        display_name: response.data.display_name,
        avatar_url: response.data.avatar_url,
        bio: response.data.bio,
        country: response.data.country,
        phone: response.data.phone,
        created_at: response.data.created_at,
        oauth_providers: response.data.oauth_providers,
        has_password: response.data.has_password,
      });
    } catch (err) {
      logout();
      if (throwOnError) throw err;
    }
  }

  async function loginWithGoogle(
    code: string,
    state: string,
  ): Promise<{
    success: boolean;
    is_new_user?: boolean;
    was_linked?: boolean;
    has_password?: boolean;
  }> {
    const toast = useToast();
    loading.value = true;
    error.value = null;
    oauthLoginResult.value = null;

    try {
      const storedReferralCode = localStorage.getItem('referral_code');
      const response: GoogleCallbackResponse = await handleGoogleCallback(code, state);

      const { access_token, is_new_user, was_linked, has_password } = response;
      setTokens(access_token);

      oauthLoginResult.value = {
        is_new_user: is_new_user ?? false,
        was_linked: was_linked ?? false,
        has_password: has_password ?? false,
      };

      await fetchUser(true);

      if (is_new_user && storedReferralCode) {
        try {
          await api.post('/api/user/referral/apply', { code: storedReferralCode });
          localStorage.removeItem('referral_code');
        } catch (refErr) {
          console.warn('Failed to apply referral code for OAuth signup:', refErr);
        }
      }

      const successMessage = is_new_user
        ? t('auth.googleAccountCreated')
        : was_linked
          ? t('auth.googleAccountLinked')
          : t('auth.loginSuccess');
      toast.success(successMessage);

      return { success: true, is_new_user, was_linked, has_password };
    } catch (err: unknown) {
      clearStoredState();
      const message = getErrorMessage(err, t('auth.googleLoginFailed'));
      error.value = message;
      toast.error(message);
      return { success: false };
    } finally {
      loading.value = false;
    }
  }

  function logout(): void {
    const currentToken = accessToken.value;

    setTokens(null);
    setUser(null);
    lastAuthResponse.value = null;
    oauthLoginResult.value = null;

    // Fire-and-forget backend logout so the server can invalidate the
    // session and clear the httpOnly cookie. If it fails, local state
    // is already cleared and the session will expire via TTL anyway.
    if (currentToken) {
      axios
        .post(
          '/api/user/logout',
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
          /* ignore */
        });
    }
  }

  function clearError(): void {
    error.value = null;
  }

  // Cross-tab logout propagation with self-heal.
  //
  // When a peer tab signals a logout, don't unconditionally wipe our
  // state — the peer may have seen a stale 401 while our refresh just
  // succeeded. Verify before acting: if our cookies still produce a
  // fresh access token, ignore the broadcast. Costs one /refresh
  // round-trip per peer per broadcast. Logout is rare; this is cheap.
  if (typeof window !== 'undefined') {
    crossTab.onRemoteLogout(async () => {
      if (!accessToken.value && !user.value) return;
      try {
        const ok = await ensureValidToken();
        if (ok && accessToken.value) return;
      } catch {
        /* fall through to logout */
      }

      // Clear in-memory state directly rather than routing through
      // setTokens(null), which would echo another broadcast back out.
      accessToken.value = null;
      setApiAccessToken(null);
      setUser(null);
      lastAuthResponse.value = null;
      oauthLoginResult.value = null;
      ready.value = true;

      const { default: router } = await import('@/router');
      const route = router.currentRoute.value;
      const onProtectedRoute = route.matched.some((r) => r.meta.requiresAuth);
      if (onProtectedRoute) {
        router.push({ name: 'login', query: { redirect: route.fullPath } });
      }
    });
  }

  // Restore the session from the httpOnly refresh cookie on app
  // startup / before router guards evaluate `isAuthenticated`.
  //
  // Deduped so concurrent callers (main.ts awaiting before mount +
  // router guard awaiting before the first navigation) share a single
  // in-flight promise. Safe to call from anywhere — always resolves,
  // never rejects.
  const bootstrap = createBootstrap(async () => {
    // Anonymous visitors (e.g. landing on /user/register from a cold
    // tab) have no session hint and no refresh cookie worth trying.
    // Skipping saves one guaranteed-401 POST per cold load.
    if (!hasSessionHint(SESSION_HINT)) {
      ready.value = true;
      return;
    }
    try {
      const valid = await ensureValidToken();
      if (!valid || !accessToken.value) {
        // Either a genuine 401 (state already cleared) or a transient
        // failure (state preserved). Either way, bail without
        // broadcasting; the next navigation will retry.
        return;
      }
      await fetchUser();
    } finally {
      ready.value = true;
    }
  });

  return {
    accessToken,
    user,
    loading,
    error,
    referredByName,
    lastAuthResponse,
    oauthLoginResult,
    ready,
    isAuthenticated,
    setTokens,
    login,
    register,
    loginWithGoogle,
    logout,
    fetchUser,
    clearError,
    refreshAccessToken,
    ensureValidToken,
    bootstrap,
    hasRole,
  };
});
