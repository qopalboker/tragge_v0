// Re-export shared error helpers + a user-frontend-specific health
// probe. The generic helpers live in @tragge/frontend-shared so the
// admin-frontend can share them; the health endpoint path differs per
// panel, so checkBackendHealth stays local.
export {
  getErrorMessage,
  isNetworkError,
  isAuthError,
  isBackendUnavailable,
} from '@tragge/frontend-shared';

// Ping the user-bff's liveness endpoint. Used by some UI code to
// distinguish "user is offline" from "our backend is down". Returns
// false on any non-2xx response or network failure.
export async function checkBackendHealth(baseUrl = ''): Promise<boolean> {
  try {
    const response = await fetch(`${baseUrl}/api/user/healthz`, {
      method: 'GET',
      signal: AbortSignal.timeout(5000),
    });
    return response.ok;
  } catch {
    return false;
  }
}
