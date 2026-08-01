/**
 * Resolve the user-frontend panel URL.
 * In Codespaces, each port gets a unique hostname (e.g. name-5174.app.github.dev),
 * so we dynamically swap the port in the hostname to target port 5173 (user-frontend).
 */
export function getUserPanelUrl(): string {
  const explicit = import.meta.env.VITE_USER_PANEL_URL;
  if (explicit) return explicit;

  const host = window.location.hostname;
  if (host.endsWith('.app.github.dev')) {
    const origin = window.location.origin;
    return origin.replace(/-\d+\.app\.github\.dev/, '-5173.app.github.dev') + '/user';
  }
  return '/user';
}

export const USER_PANEL_URL = getUserPanelUrl();
