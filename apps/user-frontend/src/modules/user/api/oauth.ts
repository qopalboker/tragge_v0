import axios from 'axios';

// ==================== OAuth Types ====================

export interface GoogleAuthResponse {
  auth_url: string;
  state: string;
}

export interface GoogleCallbackRequest {
  code: string;
  state: string;
}

export interface GoogleCallbackResponse {
  access_token: string;
  refresh_token: string;
  expires_at?: string;
  is_new_user?: boolean;
  was_linked?: boolean;
  has_password?: boolean;
}

export interface OAuthError {
  error: string;
  message?: string;
}

// ==================== OAuth API ====================

/**
 * OAuth API service for handling Google (and other OAuth providers) authentication.
 * This service handles the OAuth flow:
 * 1. getGoogleAuthUrl() - Get the Google OAuth URL to redirect the user
 * 2. handleGoogleCallback() - Exchange the authorization code for tokens
 */
export const oauthApi = {
  /**
   * Get the Google OAuth authorization URL.
   * The frontend should redirect the user to this URL.
   * @returns Promise with auth_url and state
   */
  async getGoogleAuthUrl(): Promise<GoogleAuthResponse> {
    const response = await axios.get<GoogleAuthResponse>('/api/user/auth/google');
    return response.data;
  },

  /**
   * Handle the Google OAuth callback.
   * Exchange the authorization code for access and refresh tokens.
   * @param code - The authorization code from Google
   * @param state - The state parameter for CSRF protection
   * @returns Promise with tokens and user info
   */
  async handleGoogleCallback(code: string, state: string): Promise<GoogleCallbackResponse> {
    const response = await axios.post<GoogleCallbackResponse>('/api/user/auth/google/callback', {
      code,
      state,
    });
    return response.data;
  },
};

export default oauthApi;
