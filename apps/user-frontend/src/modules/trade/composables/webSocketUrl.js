const prohibitedSessionCredentialNames = [
  'token',
  'access_token',
  'jwt',
  'auth_token',
  'session_token',
];

/**
 * Build a WebSocket URL from non-sensitive route parameters and an optional
 * bounded handshake ticket. Reusable session-credential fields fail closed.
 *
 * @param {string} rawUrl absolute WebSocket URL
 * @param {string | null} ticket short-lived, single-use WebSocket ticket
 * @param {'json' | 'msgpack'} encoding requested payload encoding
 * @returns {string}
 */
export function buildWebSocketURL(rawUrl, ticket, encoding) {
  const url = new URL(rawUrl);
  for (const name of prohibitedSessionCredentialNames) {
    if (url.searchParams.has(name)) {
      throw new Error('Reusable session credentials are forbidden in WebSocket URLs');
    }
  }
  if (ticket) url.searchParams.set('ticket', ticket);
  if (encoding === 'msgpack') url.searchParams.set('encoding', 'msgpack');
  else url.searchParams.delete('encoding');
  return url.toString();
}