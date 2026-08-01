/**
 * Utility for navigation to the trade view.
 *
 * After the frontend consolidation the trade module is mounted at
 * `/trade/*` on the same origin as the user module, so the common case is
 * now same-origin. The cross-origin ticket flow is retained as a fallback
 * for deployments that still serve the trade panel from a separate origin
 * via `VITE_TRADE_PANEL_URL`.
 */

import { api } from '@/api';
import { config } from '@/config';

/**
 * Get the trade panel base URL from config.
 *
 * Prefers an explicit absolute URL from `VITE_TRADE_PANEL_URL` (useful when
 * the trade panel is deployed separately). Otherwise returns the relative
 * `/trade` path so navigation stays same-origin — which is the only way
 * the httpOnly refresh cookie (scoped to the issuing origin) survives.
 */
function getTradeBaseUrl(): string {
  if (config.tradePanelUrl && config.tradePanelUrl.startsWith('http')) {
    return config.tradePanelUrl;
  }
  return config.tradePanelUrl || '/trade';
}

/**
 * Check if the trade-frontend URL is on the same origin.
 * When same-origin, both frontends share localStorage so no token transfer is needed.
 */
function isSameOrigin(): boolean {
  const baseUrl = getTradeBaseUrl();
  // Relative paths are always same-origin
  if (!baseUrl.startsWith('http')) {
    return true;
  }
  try {
    const tradeOrigin = new URL(baseUrl).origin;
    return tradeOrigin === window.location.origin;
  } catch {
    return false;
  }
}

/**
 * Build the trade panel URL for a contest (no token in URL).
 */
export function buildTradeUrl(contestId: string): string {
  const baseUrl = getTradeBaseUrl();

  // Handle both relative paths and full URLs
  if (baseUrl.startsWith('http')) {
    return new URL(`${baseUrl}/${contestId}`).toString();
  }
  return new URL(`${baseUrl}/${contestId}`, window.location.origin).toString();
}

/**
 * Request a short-lived auth ticket from the backend.
 * The ticket can be exchanged by the trade-frontend for an access token.
 * Returns null if the ticket could not be created.
 */
async function requestAuthTicket(): Promise<string | null> {
  try {
    const response = await api.post<{ ticket: string }>('/api/user/auth/ticket');
    return response.data.ticket;
  } catch {
    console.warn('Failed to create auth ticket for cross-origin navigation');
    return null;
  }
}

/**
 * Redirect to trade panel for a specific contest.
 *
 * Same-origin: navigates directly (localStorage is shared).
 * Cross-origin: obtains a short-lived ticket and appends it to the URL.
 *
 * @param contestId - The contest ID to enter
 */
export async function redirectToTrade(contestId: string): Promise<void> {
  const tradeUrl = buildTradeUrl(contestId);

  if (isSameOrigin()) {
    // Same origin — trade-frontend will restore session via httpOnly cookie silent refresh
    window.location.href = tradeUrl;
    return;
  }

  // Cross-origin — obtain a short-lived ticket
  const ticket = await requestAuthTicket();
  if (ticket) {
    const url = new URL(tradeUrl);
    url.searchParams.set('ticket', ticket);
    window.location.href = url.toString();
  } else {
    // Fallback: navigate without auth — trade-frontend will redirect to login
    window.location.href = tradeUrl;
  }
}
