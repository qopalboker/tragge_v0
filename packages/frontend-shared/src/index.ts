// Barrel exports for @tragge/frontend-shared.
// Consumers import from '@tragge/frontend-shared' (root barrel) or
// '@tragge/frontend-shared/styles/main.css' (global CSS).

// Auth primitives
export type {
  AccessTokenBearer,
  RefreshResponse,
  RefreshErrorKind,
  RefreshError,
  RefreshOutcome,
  SessionHintConfig,
} from './auth/types';
export type { TokenBridge } from './auth/tokenBridge';
export { createTokenBridge } from './auth/tokenBridge';
export type { RefreshOptions } from './auth/refresh';
export { refreshAccessToken } from './auth/refresh';
export type { CrossTabChannel } from './auth/crossTab';
export { createCrossTabChannel } from './auth/crossTab';
export type { BootstrapFn } from './auth/bootstrap';
export {
  hasSessionHint,
  clearSessionHintCookie,
  clearLegacySessionHint,
  createBootstrap,
} from './auth/bootstrap';

// API client
export type { ApiClientConfig } from './api/client';
export { createApiClient } from './api/client';

// Shared stores
export { useThemeStore, themes } from './stores/theme';
export type { Theme, ResolvedTheme, ThemeId, ThemeColors } from './stores/theme';
export { useI18nStore } from './stores/i18n';

// i18n
export type { Locale, Direction, LocaleMessages, LocaleMessageTree } from './i18n/types';
export { t, setLocale, getLocale, direction, initializeI18n, i18n, state as i18nState } from './i18n';

// Composables
export { useToast } from './composables/useToast';
export type { Toast, ToastType } from './composables/useToast';

// Utils
export {
  formatScore,
  formatCurrency,
  formatPercent,
  formatNumber,
  formatRank,
  getPnLColorClass,
  getPnLGlowClass,
  getPnLBgClass,
} from './utils/formatters';
export type { FormatOptions } from './utils/formatters';
export {
  getErrorMessage,
  isNetworkError,
  isAuthError,
  isBackendUnavailable,
} from './utils/errorHandler';
export {
  logger,
  REDACTED_VALUE,
  redactForLogging,
  redactTextForLogging,
  installConsoleRedaction,
} from './utils/logger';
export type { RedirectValidator, RedirectValidatorConfig } from './utils/redirectValidation';
export { createRedirectValidator } from './utils/redirectValidation';
