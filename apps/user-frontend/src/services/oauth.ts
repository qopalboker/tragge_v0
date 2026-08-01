import { api } from '@/api';

// ============================================================================
// TypeScript Interfaces for OAuth Responses
// ============================================================================

/**
 * Response from GET /api/user/auth/google
 * Contains the Google OAuth authorization URL and state
 */
export interface GoogleAuthUrlResponse {
  auth_url: string;
  state: string;
}

/**
 * Request body for POST /api/user/auth/google/callback
 */
export interface GoogleCallbackRequest {
  code: string;
  state: string;
}

/**
 * Response from POST /api/user/auth/google/callback
 * Contains JWT tokens on successful authentication
 */
export interface GoogleCallbackResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  is_new_user: boolean;
  was_linked: boolean;
  has_password: boolean;
}

/**
 * OAuth error response from the backend
 */
export interface OAuthErrorResponse {
  error: string;
  message?: string;
}

/**
 * Supported OAuth providers
 */
export type OAuthProvider = 'google' | 'github' | 'facebook' | 'apple' | 'discord';

/**
 * OAuth account info returned when listing linked accounts
 */
export interface OAuthAccountInfo {
  id: string;
  provider: OAuthProvider;
  email?: string;
  linked_at: string;
  updated_at: string;
}

// ============================================================================
// Constants
// ============================================================================

/** SessionStorage key for OAuth state */
const OAUTH_STATE_KEY = 'oauth_state';

/** SessionStorage key for OAuth state timestamp */
const OAUTH_STATE_TIMESTAMP_KEY = 'oauth_state_timestamp';

/** State validity duration in milliseconds (5 minutes, matching backend) */
const STATE_VALIDITY_MS = 5 * 60 * 1000;

// ============================================================================
// State Management Functions (CSRF Protection)
// ============================================================================

/**
 * Generates a cryptographically secure random state string for CSRF protection.
 * Stores the state in sessionStorage for later validation.
 *
 * @returns The generated state string (32 characters hex)
 */
export function generateState(): string {
  // Generate 16 random bytes and convert to hex string (32 chars)
  const array = new Uint8Array(16);
  crypto.getRandomValues(array);
  const state = Array.from(array)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');

  // Store state and timestamp in sessionStorage
  sessionStorage.setItem(OAUTH_STATE_KEY, state);
  sessionStorage.setItem(OAUTH_STATE_TIMESTAMP_KEY, Date.now().toString());

  return state;
}

/**
 * Validates the OAuth state parameter against the stored value.
 * This provides CSRF protection by ensuring the callback matches our initiated request.
 *
 * @param state - The state parameter received from the OAuth callback
 * @returns true if the state is valid and not expired, false otherwise
 */
export function validateState(state: string): boolean {
  if (!state) {
    return false;
  }

  const storedState = sessionStorage.getItem(OAUTH_STATE_KEY);
  const storedTimestamp = sessionStorage.getItem(OAUTH_STATE_TIMESTAMP_KEY);

  // Check if state matches
  if (!storedState || storedState !== state) {
    return false;
  }

  // Check if state is not expired
  if (storedTimestamp) {
    const timestamp = parseInt(storedTimestamp, 10);
    if (Date.now() - timestamp > STATE_VALIDITY_MS) {
      // State has expired
      clearStoredState();
      return false;
    }
  }

  return true;
}

/**
 * Clears the stored OAuth state from sessionStorage.
 * Should be called after successful validation or when starting a new OAuth flow.
 */
export function clearStoredState(): void {
  sessionStorage.removeItem(OAUTH_STATE_KEY);
  sessionStorage.removeItem(OAUTH_STATE_TIMESTAMP_KEY);
}

// ============================================================================
// OAuth API Functions
// ============================================================================

/**
 * Fetches the Google OAuth authorization URL from the backend.
 * The URL includes the CSRF state parameter and redirect URI.
 *
 * @returns Promise resolving to the Google OAuth response with URL and state
 * @throws Error if the backend returns an error or is unavailable
 */
export async function getGoogleAuthUrl(): Promise<GoogleAuthUrlResponse> {
  const response = await api.get<GoogleAuthUrlResponse>('/api/user/auth/google');
  return response.data;
}

/**
 * Handles the Google OAuth callback by exchanging the authorization code for tokens.
 * This should be called after the user is redirected back from Google.
 *
 * @param code - The authorization code from Google's OAuth callback
 * @param state - The state parameter for CSRF validation
 * @returns Promise resolving to the authentication response with tokens
 * @throws Error if validation fails or token exchange fails
 */
export async function handleGoogleCallback(
  code: string,
  state: string
): Promise<GoogleCallbackResponse> {
  // Validate state before making the API call (defense in depth)
  // Note: Backend also validates state, this is an additional client-side check
  if (!validateState(state)) {
    throw new Error('Invalid OAuth state. Please try again.');
  }

  const request: GoogleCallbackRequest = {
    code,
    state,
  };

  const response = await api.post<GoogleCallbackResponse>(
    '/api/user/auth/google/callback',
    request
  );

  // Clear stored state after successful callback
  clearStoredState();

  return response.data;
}

/**
 * Initiates the Google OAuth login flow.
 * This is a convenience function that:
 * 1. Clears any existing state
 * 2. Fetches the OAuth URL and state from the backend
 * 3. Stores the backend's state in sessionStorage
 * 4. Redirects the user to Google's OAuth consent screen
 *
 * @returns Promise that resolves when redirect is initiated (never resolves if redirect succeeds)
 * @throws Error if fetching the OAuth URL fails
 */
export async function initiateGoogleLogin(): Promise<void> {
  // Clear any existing state from previous attempts
  clearStoredState();

  // Get the OAuth URL and state from the backend
  const authResponse = await getGoogleAuthUrl();

  // Store the backend's state for CSRF protection validation during callback
  sessionStorage.setItem(OAUTH_STATE_KEY, authResponse.state);
  sessionStorage.setItem(OAUTH_STATE_TIMESTAMP_KEY, Date.now().toString());

  // Redirect to Google's OAuth consent screen
  window.location.href = authResponse.auth_url;
}

// ============================================================================
// OAuth Service Object (for consistency with other API modules)
// ============================================================================

export const oauthService = {
  /**
   * Generate and store a CSRF state for OAuth flow
   */
  generateState,

  /**
   * Validate the OAuth state parameter
   */
  validateState,

  /**
   * Clear stored OAuth state
   */
  clearStoredState,

  /**
   * Get the Google OAuth authorization URL
   */
  getGoogleAuthUrl,

  /**
   * Handle the Google OAuth callback
   */
  handleGoogleCallback,

  /**
   * Initiate the Google OAuth login flow (convenience method)
   */
  initiateGoogleLogin,
};

export default oauthService;
