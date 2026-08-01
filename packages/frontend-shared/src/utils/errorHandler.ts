import { AxiosError } from 'axios';
import { t } from '../i18n';

// Extract a user-friendly error message from axios errors, native
// Error instances, and loosely-typed API responses. Fallback strings
// come from the shared i18n common locale (see i18n/common/en.ts) so
// both user-frontend and admin-frontend get identical generic messages
// without duplicating translation keys.
export function getErrorMessage(error: unknown, defaultMessage?: string): string {
  if (typeof error === 'string') {
    return error;
  }

  if (error instanceof AxiosError) {
    const response = error.response;
    if (response?.data) {
      const data = response.data as Record<string, unknown>;
      if (typeof data.error === 'string') return data.error;
      if (typeof data.message === 'string') return data.message;
      if (typeof data.detail === 'string') return data.detail;
    }

    if (error.code === 'ERR_NETWORK') {
      return t('errors.networkError');
    }
    if (error.code === 'ECONNABORTED') {
      return t('errors.requestTimeout');
    }

    if (response?.status) {
      switch (response.status) {
        case 400:
          return t('errors.invalidRequest');
        case 401:
          return t('errors.sessionExpired');
        case 403:
          return t('errors.accessDenied');
        case 404:
          return t('errors.notFound');
        case 422:
          return t('errors.validationError');
        case 429:
          return t('errors.tooManyRequests');
        case 500:
          return t('errors.serverError');
        case 502:
          return t('errors.backendServiceUnavailable');
        case 503:
          return t('errors.serviceUnavailable');
        case 504:
          return t('errors.gatewayTimeout');
      }
    }
  }

  if (error instanceof Error) {
    return error.message || defaultMessage || t('errors.defaultError');
  }

  if (error && typeof error === 'object') {
    const obj = error as Record<string, unknown>;
    if (typeof obj.error === 'string') return obj.error;
    if (typeof obj.message === 'string') return obj.message;
  }

  return defaultMessage || t('errors.defaultError');
}

export function isNetworkError(error: unknown): boolean {
  if (error instanceof AxiosError) {
    return error.code === 'ERR_NETWORK' || error.code === 'ECONNABORTED';
  }
  return false;
}

export function isAuthError(error: unknown): boolean {
  if (error instanceof AxiosError) {
    return error.response?.status === 401;
  }
  return false;
}

export function isBackendUnavailable(error: unknown): boolean {
  if (error instanceof AxiosError) {
    const status = error.response?.status;
    return status === 502 || status === 503 || status === 504;
  }
  return false;
}
