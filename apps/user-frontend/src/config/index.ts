/**
 * Application configuration
 * URLs and settings are loaded from environment variables with fallback defaults
 */

import { wsConfig, getWebSocketEncoding } from './websocket';

export const config = {
  /**
   * URL for the user panel (user-frontend)
   * In production: served from /user on the same origin
   * In development: typically http://localhost:5173
   */
  userPanelUrl: import.meta.env.VITE_USER_PANEL_URL || '/user',

  /**
   * URL for the trade panel (trade-frontend)
   * In production: served from /trade on the same origin
   * In development: typically http://localhost:5174/trade
   */
  tradePanelUrl: import.meta.env.VITE_TRADE_PANEL_URL || '/trade',

  /**
   * Base URL for API requests
   * Usually empty (same origin) or a specific API gateway URL
   */
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || '',

  /**
   * WebSocket configuration
   */
  ws: wsConfig,
};

// Re-export websocket config for direct access
export { wsConfig, getWebSocketEncoding };
