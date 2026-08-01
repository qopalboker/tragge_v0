// Bridges the auth store and the API interceptor without a circular
// dependency. The auth store registers a token accessor at startup; the
// API client reads the live access token through this bridge instead of
// importing the store (which would pull in Pinia before it is ready).
//
// Each app instantiates ONE bridge and passes it to both sides. The
// shared `createApiClient` accepts `getAccessToken`/`setAccessToken`
// directly, so apps may also wire the bridge inline — but the helper
// below is convenient and mirrors today's pattern.

export interface TokenBridge {
  get(): string | null;
  set(token: string | null): void;
}

export function createTokenBridge(): TokenBridge {
  let current: string | null = null;
  return {
    get: () => current,
    set: (token: string | null) => {
      current = token;
    },
  };
}
