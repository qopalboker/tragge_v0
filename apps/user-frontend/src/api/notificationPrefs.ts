import { api } from '../modules/user/api/index';

export interface NotificationPref {
  category: string;
  channel: string;
  enabled: boolean;
}

export interface NotificationPrefsResponse {
  preferences: NotificationPref[];
  categories: string[];
  channels: string[];
}

export const notificationPrefsApi = {
  async getPreferences(): Promise<NotificationPrefsResponse> {
    const response = await api.get<NotificationPrefsResponse>('/api/user/me/notification-preferences');
    return response.data;
  },

  async updatePreferences(preferences: NotificationPref[]): Promise<NotificationPrefsResponse> {
    const response = await api.put<NotificationPrefsResponse>('/api/user/me/notification-preferences', {
      preferences,
    });
    return response.data;
  },
};
