import { api } from './index';

// ==================== Types ====================
export type NotificationType =
  | 'contest_starting'
  | 'contest_completed'
  | 'contest_cancelled'
  | 'prize_won'
  | 'withdrawal_update'
  | 'deposit_confirmed'
  | 'kyc_update'
  | 'system'
  | 'contest_ending'
  | 'contest_joined'
  | 'contest_left'
  | 'deposit_failed'
  | 'contest_started'
  | 'registration_closed'
  | 'contest_paused'
  | 'contest_resumed'
  | 'ticket_reply';

export interface InAppNotification {
  id: string;
  user_id: string;
  type: NotificationType;
  title: string;
  message: string;
  read_at: string | null;
  metadata?: Record<string, string | number | boolean>;
  created_at: string;
}

export interface NotificationsResponse {
  notifications: InAppNotification[];
  total: number;
  unread_count?: number;
}

export interface UnreadCountResponse {
  count: number;
}

export interface GetNotificationsParams {
  limit?: number;
  offset?: number;
  unread_only?: boolean;
}

// ==================== Notifications API ====================
export const notificationsApi = {
  /**
   * Get user notifications with pagination
   */
  async getNotifications(params?: GetNotificationsParams): Promise<NotificationsResponse> {
    const response = await api.get<NotificationsResponse>('/api/user/me/notifications', { params });
    return response.data;
  },

  /**
   * Get count of unread notifications
   */
  async getUnreadCount(): Promise<UnreadCountResponse> {
    const response = await api.get<UnreadCountResponse>('/api/user/me/notifications/unread-count');
    return response.data;
  },

  /**
   * Mark a notification as read
   */
  async markAsRead(id: string): Promise<void> {
    await api.post(`/api/user/me/notifications/${id}/read`);
  },

  /**
   * Mark all notifications as read
   */
  async markAllAsRead(): Promise<void> {
    await api.post('/api/user/me/notifications/read-all');
  },

  /**
   * Delete a notification
   */
  async deleteNotification(id: string): Promise<void> {
    await api.delete(`/api/user/me/notifications/${id}`);
  },
};
