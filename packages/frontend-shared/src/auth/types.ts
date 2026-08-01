// Auth primitive types shared by user-frontend and admin-frontend.
// These are the minimal shapes the shared layer deals in — they do NOT
// describe a full session or user (each app defines its own, with
// audience-specific fields).

export interface AccessTokenBearer {
  access_token: string;
}

export interface RefreshResponse extends AccessTokenBearer {
  // Server may also return these, but the shared primitive only cares
  // about access_token. Refresh rotation happens via httpOnly cookie.
  refresh_token?: string;
  expires_in?: number;
}

export type RefreshErrorKind =
  // 401/403 — cookie was rejected, caller should clear session.
  | 'auth'
  // Network/CORS, 5xx, malformed payload — caller should preserve
  // session so a transient blip does not log the user out.
  | 'transient'
  // Well-formed 2xx without a usable access_token. Treat as transient.
  | 'unknown';

export interface RefreshError {
  kind: RefreshErrorKind;
  status?: number;
}

export type RefreshOutcome =
  | { ok: true; accessToken: string }
  | { ok: false; error: RefreshError };

export interface SessionHintConfig {
  // Non-sensitive cookie that signals "a session may exist — worth
  // attempting silent refresh on cold start". Server sets/clears it
  // alongside the httpOnly refresh cookie.
  cookieName: string;
  // Legacy localStorage key read as a one-release migration fallback.
  // Pass undefined if no migration is needed (e.g. admin, which never
  // had a legacy key).
  legacyLocalStorageKey?: string;
}
