import type { RefreshOutcome } from './types';

export interface RefreshOptions {
  // Full URL or absolute path of the refresh endpoint, e.g.
  //   '/api/user/auth/refresh' or '/api/admin/auth/refresh'.
  endpoint: string;

  // Optional extra headers. These are applied BEFORE the mandatory
  // `X-Requested-With: XMLHttpRequest` header, so callers cannot
  // accidentally override the CSRF guard.
  extraHeaders?: Record<string, string>;
}

// Exchange an httpOnly refresh cookie for a new access token.
//
// Uses `fetch` (not axios) to stay independent of any axios instance —
// this primitive must be callable from the axios response interceptor
// without risking recursion.
//
// ALWAYS sends `X-Requested-With: XMLHttpRequest`. The BFF's CSRF
// middleware rejects requests without it with 403 `csrf_header_missing`,
// which bootstrap then misreads as a logout signal (see PR #936). This
// header is applied LAST so `extraHeaders` cannot override it.
//
// Status taxonomy:
//   - 401/403      → RefreshError { kind: 'auth' }     — cookie rejected
//   - network/CORS → RefreshError { kind: 'transient' }— preserve session
//   - 5xx, 4xx-not-auth → same as transient
//   - 2xx without access_token → { kind: 'unknown' }    — treat as transient
export async function refreshAccessToken(
  options: RefreshOptions,
): Promise<RefreshOutcome> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.extraHeaders ?? {}),
    // Last. Do not move. Do not remove. See above.
    'X-Requested-With': 'XMLHttpRequest',
  };

  let response: Response;
  try {
    response = await fetch(options.endpoint, {
      method: 'POST',
      credentials: 'include',
      headers,
      body: '{}',
    });
  } catch {
    // Network / CORS / AbortError — cookie state unknown, caller keeps
    // whatever it had and retries on the next interaction.
    return { ok: false, error: { kind: 'transient' } };
  }

  if (response.ok) {
    let data: unknown;
    try {
      data = await response.json();
    } catch {
      return { ok: false, error: { kind: 'unknown', status: response.status } };
    }
    if (
      data &&
      typeof data === 'object' &&
      typeof (data as Record<string, unknown>).access_token === 'string'
    ) {
      return {
        ok: true,
        accessToken: (data as { access_token: string }).access_token,
      };
    }
    return { ok: false, error: { kind: 'unknown', status: response.status } };
  }

  if (response.status === 401 || response.status === 403) {
    return { ok: false, error: { kind: 'auth', status: response.status } };
  }

  // 4xx-other or 5xx — treat as transient. A 400 on refresh is almost
  // certainly a backend bug, not a signal that the user's session is
  // dead.
  return { ok: false, error: { kind: 'transient', status: response.status } };
}
