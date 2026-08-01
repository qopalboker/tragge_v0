/**
 * WebSocket configuration for trade-frontend
 *
 * Controls encoding format for high-frequency tick messages.
 * MessagePack provides ~40-50% bandwidth savings over JSON.
 */

import type { WebSocketEncoding } from '@/composables/useWebSocket';

/**
 * Get the configured WebSocket encoding from environment variable.
 * Defaults to 'msgpack' for bandwidth optimization.
 * Set VITE_WS_ENCODING=json for debugging (readable messages in dev tools).
 */
export function getWebSocketEncoding(): WebSocketEncoding {
  const envEncoding = import.meta.env.VITE_WS_ENCODING as string | undefined;

  if (envEncoding === 'json') {
    return 'json';
  }

  // Default to MessagePack for bandwidth savings
  return 'msgpack';
}

/**
 * WebSocket configuration object
 */
export const wsConfig = {
  /**
   * Message encoding format.
   * 'msgpack' - Binary MessagePack encoding (~40-50% bandwidth savings)
   * 'json' - Text JSON encoding (for debugging compatibility)
   */
  encoding: getWebSocketEncoding(),

  /**
   * Check if binary encoding is enabled
   */
  get isBinaryEnabled(): boolean {
    return this.encoding === 'msgpack';
  },
};

export default wsConfig;
