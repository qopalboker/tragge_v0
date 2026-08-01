import { api } from './index';

// Types

export interface AdminAvatar {
  id: string;
  slug: string;
  display_name: string;
  category: string;
  bg_color: string;
  image_path: string;
  sort_order: number;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface AdminAvatarListResponse {
  avatars: AdminAvatar[];
  total: number;
}

export interface UpdateAvatarRequest {
  display_name?: string;
  category?: string;
  bg_color?: string;
  is_active?: boolean;
  sort_order?: number;
}

// API functions

export async function listAdminAvatars(): Promise<AdminAvatarListResponse> {
  const response = await api.get<AdminAvatarListResponse>('/api/admin/avatars');
  return response.data;
}

export async function createAvatar(formData: FormData): Promise<AdminAvatar> {
  const response = await api.post<AdminAvatar>('/api/admin/avatars', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
  return response.data;
}

export async function updateAvatar(id: string, data: UpdateAvatarRequest): Promise<AdminAvatar> {
  const response = await api.put<AdminAvatar>(`/api/admin/avatars/${id}`, data);
  return response.data;
}

export async function replaceAvatarImage(id: string, formData: FormData): Promise<{ image_path: string }> {
  const response = await api.post<{ image_path: string }>(`/api/admin/avatars/${id}/image`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
  return response.data;
}

export async function deleteAvatar(id: string, hard = false): Promise<void> {
  await api.delete(`/api/admin/avatars/${id}${hard ? '?hard=true' : ''}`);
}

export async function reorderAvatars(order: { id: string; sort_order: number }[]): Promise<void> {
  await api.post('/api/admin/avatars/reorder', { order });
}
