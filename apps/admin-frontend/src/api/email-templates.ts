import { api } from './index';

// Types
export interface EmailTemplate {
  slug: string;
  description: string;
  variables: string;
  has_custom: boolean;
  updated_at: string;
  updated_by?: string;
}

export interface EmailTemplateDetail {
  slug: string;
  subject?: string;
  description: string;
  variables: string;
  html_content: string;
  is_default: boolean;
  updated_at: string;
  updated_by?: string;
}

export interface EmailTemplateListResponse {
  templates: EmailTemplate[];
}

export interface UpdateTemplateRequest {
  html_content: string;
}

export interface PreviewTemplateRequest {
  html_content?: string;
}

export interface PreviewTemplateResponse {
  rendered_html: string;
}

// API functions

/**
 * List all email templates
 */
export async function listEmailTemplates(): Promise<EmailTemplate[]> {
  const response = await api.get<EmailTemplateListResponse>('/api/admin/email-templates');
  return response.data.templates || [];
}

/**
 * Get a single email template by slug
 */
export async function getEmailTemplate(slug: string): Promise<EmailTemplateDetail> {
  const response = await api.get<EmailTemplateDetail>(`/api/admin/email-templates/${slug}`);
  return response.data;
}

/**
 * Update an email template
 */
export async function updateEmailTemplate(
  slug: string,
  htmlContent: string
): Promise<void> {
  await api.put(`/api/admin/email-templates/${slug}`, {
    html_content: htmlContent,
  });
}

/**
 * Reset an email template to its default
 */
export async function resetEmailTemplate(slug: string): Promise<void> {
  await api.post(`/api/admin/email-templates/${slug}/reset`);
}

/**
 * Preview an email template with sample data
 */
export async function previewEmailTemplate(
  slug: string,
  htmlContent?: string
): Promise<string> {
  const response = await api.post<PreviewTemplateResponse>(
    `/api/admin/email-templates/${slug}/preview`,
    { html_content: htmlContent || '' }
  );
  return response.data.rendered_html;
}

// ====== Version Management Types ======

export interface FontConfigEntry {
  family: string;
  weight: string;
  url: string;
}

export interface FontConfig {
  en: FontConfigEntry;
  fa: FontConfigEntry;
  [key: string]: FontConfigEntry; // allow other languages
}

export interface TemplateVersionListItem {
  id: string;
  slug: string;
  version_name: string;
  is_active: boolean;
  font_config: FontConfig;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

export interface TemplateVersionDetail {
  id: string;
  slug: string;
  version_name: string;
  html_body: string;
  css_content: string;
  font_config: FontConfig;
  is_active: boolean;
  created_by?: string;
  updated_by?: string;
  created_at: string;
  updated_at: string;
}

export interface TemplateVersionsResponse {
  versions: TemplateVersionListItem[];
  slug: string;
  max_versions: number;
}

export interface CreateTemplateVersionRequest {
  version_name: string;
  html_body: string;
  css_content: string;
  font_config: FontConfig;
  is_active: boolean;
}

export interface UpdateTemplateVersionRequest {
  version_name?: string;
  html_body?: string;
  css_content?: string;
  font_config?: FontConfig;
}

// ====== Version Management API Functions ======

/**
 * List all versions of a template
 */
export async function listTemplateVersions(slug: string): Promise<TemplateVersionsResponse> {
  const response = await api.get<TemplateVersionsResponse>(`/api/admin/email-templates/${slug}/versions`);
  return response.data;
}

/**
 * Get a specific template version
 */
export async function getTemplateVersion(slug: string, versionId: string): Promise<TemplateVersionDetail> {
  const response = await api.get<TemplateVersionDetail>(`/api/admin/email-templates/${slug}/versions/${versionId}`);
  return response.data;
}

/**
 * Create a new template version
 */
export async function createTemplateVersion(slug: string, data: CreateTemplateVersionRequest): Promise<TemplateVersionDetail> {
  const response = await api.post<TemplateVersionDetail>(`/api/admin/email-templates/${slug}/versions`, data);
  return response.data;
}

/**
 * Update an existing template version
 */
export async function updateTemplateVersion(slug: string, versionId: string, data: UpdateTemplateVersionRequest): Promise<TemplateVersionDetail> {
  const response = await api.put<TemplateVersionDetail>(`/api/admin/email-templates/${slug}/versions/${versionId}`, data);
  return response.data;
}

/**
 * Delete a template version
 */
export async function deleteTemplateVersion(slug: string, versionId: string): Promise<void> {
  await api.delete(`/api/admin/email-templates/${slug}/versions/${versionId}`);
}

/**
 * Activate a specific template version
 */
export async function activateTemplateVersion(slug: string, versionId: string): Promise<void> {
  await api.post(`/api/admin/email-templates/${slug}/versions/${versionId}/activate`);
}

/**
 * Preview a rendered template version
 */
export async function previewTemplateVersion(slug: string, versionId: string): Promise<string> {
  const response = await api.post<{ rendered_html: string }>(
    `/api/admin/email-templates/${slug}/versions/${versionId}/preview`
  );
  return response.data.rendered_html;
}
