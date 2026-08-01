import { api } from './index';

// ==================== Types ====================

export interface AdminTicketUser {
  id: string;
  email: string;
  username: string;
}

export interface AdminTicket {
  id: string;
  subject: string;
  category: string;
  status: string;
  priority: string;
  user: AdminTicketUser;
  assigned_admin?: AdminTicketUser;
  message_count: number;
  last_message_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AdminTicketMessage {
  id: string;
  body: string;
  is_admin: boolean;
  sender_name: string;
  attachments: AdminTicketAttachment[];
  created_at: string;
}

export interface AdminTicketAttachment {
  id: string;
  file_name: string;
  file_size: number;
  content_type: string;
}

export interface AdminTicketDetail {
  ticket: {
    id: string;
    subject: string;
    category: string;
    status: string;
    priority: string;
    user: AdminTicketUser;
    assigned_admin?: AdminTicketUser;
    closed_at?: string;
    created_at: string;
    updated_at: string;
  };
  messages: AdminTicketMessage[];
}

export interface AdminTicketListResponse {
  tickets: AdminTicket[];
  total: number;
  has_more: boolean;
}

export interface AdminTicketStats {
  total: number;
  open: number;
  user_replied: number;
  answered: number;
  closed: number;
  resolved: number;
  avg_response_time_minutes: number;
}

export interface AdminTicketListParams {
  limit?: number;
  offset?: number;
  status?: string;
  category?: string;
  priority?: string;
  assigned_to?: string;
  search?: string;
  sort?: string;
}

// ==================== API ====================

export const adminTicketsApi = {
  async list(params?: AdminTicketListParams): Promise<AdminTicketListResponse> {
    const response = await api.get<AdminTicketListResponse>('/api/admin/tickets', { params });
    return response.data;
  },

  async get(id: string): Promise<AdminTicketDetail> {
    const response = await api.get<AdminTicketDetail>(`/api/admin/tickets/${id}`);
    return response.data;
  },

  async sendMessage(id: string, formData: FormData): Promise<{ id: string }> {
    const response = await api.post<{ id: string }>(`/api/admin/tickets/${id}/messages`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    });
    return response.data;
  },

  async updateStatus(id: string, status: string): Promise<void> {
    await api.put(`/api/admin/tickets/${id}/status`, { status });
  },

  async assign(id: string, adminId: string): Promise<void> {
    await api.put(`/api/admin/tickets/${id}/assign`, { admin_id: adminId });
  },

  async updatePriority(id: string, priority: string): Promise<void> {
    await api.put(`/api/admin/tickets/${id}/priority`, { priority });
  },

  async getStats(): Promise<AdminTicketStats> {
    const response = await api.get<AdminTicketStats>('/api/admin/tickets/stats');
    return response.data;
  },

  getAttachmentUrl(attachmentId: string): string {
    return `/api/admin/tickets/attachment/${attachmentId}`;
  },
};
