import { api } from './index';

// ==================== Types ====================

export interface Ticket {
  id: string;
  subject: string;
  category: string;
  status: string;
  priority: string;
  last_message_preview?: string;
  last_message_at?: string;
  last_message_is_admin?: boolean;
  unread?: boolean;
  message_count: number;
  created_at: string;
  updated_at: string;
}

export interface TicketMessage {
  id: string;
  body: string;
  is_admin: boolean;
  sender_name: string;
  attachments: TicketAttachment[];
  created_at: string;
}

export interface TicketAttachment {
  id: string;
  file_name: string;
  file_size: number;
  content_type: string;
}

export interface TicketDetail {
  ticket: {
    id: string;
    subject: string;
    category: string;
    status: string;
    priority: string;
    closed_at?: string;
    created_at: string;
    updated_at: string;
  };
  messages: TicketMessage[];
}

export interface TicketListResponse {
  tickets: Ticket[];
  total: number;
  has_more: boolean;
}

// ==================== API ====================

export const ticketsApi = {
  async list(params?: { limit?: number; offset?: number; status?: string }): Promise<TicketListResponse> {
    const response = await api.get<TicketListResponse>('/api/user/me/tickets', { params });
    return response.data;
  },

  async get(ticketId: string): Promise<TicketDetail> {
    const response = await api.get<TicketDetail>(`/api/user/me/tickets/${ticketId}`);
    return response.data;
  },

  async create(formData: FormData): Promise<{ id: string }> {
    const response = await api.post<{ id: string }>('/api/user/me/tickets', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    return response.data;
  },

  async sendMessage(ticketId: string, formData: FormData): Promise<{ id: string }> {
    const response = await api.post<{ id: string }>(`/api/user/me/tickets/${ticketId}/messages`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    return response.data;
  },

  async close(ticketId: string): Promise<void> {
    await api.post(`/api/user/me/tickets/${ticketId}/close`);
  },

  async getUnreadCount(): Promise<{ count: number }> {
    const response = await api.get<{ count: number }>('/api/user/me/tickets/unread-count');
    return response.data;
  },

  getAttachmentUrl(attachmentId: string): string {
    return `/api/user/me/tickets/attachment/${attachmentId}`;
  },
};
