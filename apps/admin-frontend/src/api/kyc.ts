import { api } from './index';

export enum KYCStatus {
  Pending = 'pending',
  UnderReview = 'under_review',
  Approved = 'approved',
  Rejected = 'rejected',
  MoreInfoRequired = 'more_info_required',
}

export type DocumentType = 'passport' | 'national_id' | 'drivers_license' | 'birth_certificate';

export const DocumentType = {
  Passport: 'passport' as const,
  NationalId: 'national_id' as const,
  DriversLicense: 'drivers_license' as const,
  BirthCertificate: 'birth_certificate' as const,
};

export interface KYCDocument {
  id: string;
  type: DocumentType;
  document_number: string;
  front_image_url: string;
  back_image_url?: string;
  selfie_image_url: string;
  selfie_with_doc_url?: string;
}

export interface KYCSubmission {
  id: string;
  user_id: string;
  user_email: string;
  full_name: string;
  date_of_birth: string;
  nationality: string;
  address: string;
  document: KYCDocument;
  status: KYCStatus;
  submitted_at: string;
  reviewed_at?: string;
  reviewed_by?: string;
  rejection_reason?: string;
  notes?: string;
  // Jibit verification fields
  shahkar_verified?: boolean;
  face_verified?: boolean;
  face_match_score?: number;
  liveness_score?: number;
  liveness_result?: string;
  card_ocr_verified?: boolean;
  national_code?: string;
  auto_approved?: boolean;
  // Iranian manual KYC fields
  father_name?: string;
  national_code_manual?: string;
  province?: string;
}

export interface KYCAuditEntry {
  id: string;
  submission_id: string;
  action: string;
  performed_by: string;
  performed_at: string;
  details?: string;
}

export interface KYCListResponse {
  submissions: KYCSubmission[];
  total: number;
  page: number;
  per_page: number;
}

export interface KYCDetailResponse {
  submission: KYCSubmission;
  audit_history: KYCAuditEntry[];
  previous_submissions: KYCSubmission[];
}

export interface ApproveRequest {
  notes?: string;
}

export interface RejectRequest {
  reason: string;
  rejected_fields?: string[];
  field_messages?: Record<string, string>;
}

export interface RequestInfoRequest {
  message: string;
}

export async function getKYCSubmissions(
  status?: KYCStatus,
  page = 1,
  perPage = 20
): Promise<KYCListResponse> {
  const params = new URLSearchParams();
  if (status) params.append('status', status);
  params.append('page', page.toString());
  params.append('per_page', perPage.toString());
  params.append('sort', 'oldest');

  const response = await api.get<KYCListResponse>(`/api/admin/kyc?${params.toString()}`);
  return response.data;
}

export async function getKYCSubmission(id: string): Promise<KYCDetailResponse> {
  const response = await api.get<KYCDetailResponse>(`/api/admin/kyc/${id}`);
  return response.data;
}

export async function approveKYC(id: string, data: ApproveRequest): Promise<void> {
  await api.post(`/api/admin/kyc/${id}/approve`, data);
}

export async function rejectKYC(id: string, data: RejectRequest): Promise<void> {
  await api.post(`/api/admin/kyc/${id}/reject`, data);
}

export async function requestMoreInfo(id: string, data: RequestInfoRequest): Promise<void> {
  await api.post(`/api/admin/kyc/${id}/request-info`, data);
}

export interface BulkAutoApproveResponse {
  message: string;
  approved: number;
  total: number;
  failed?: string[];
}

export async function bulkAutoApproveKYC(): Promise<BulkAutoApproveResponse> {
  const response = await api.post<BulkAutoApproveResponse>('/api/admin/kyc/bulk-auto-approve');
  return response.data;
}

// Helper to get document type display label
export function getDocumentTypeLabel(type: DocumentType): string {
  const labels: Record<DocumentType, string> = {
    passport: 'Passport',
    national_id: 'National ID / کارت ملی',
    drivers_license: "Driver's License",
    birth_certificate: 'Birth Certificate / شناسنامه',
  };
  return labels[type] || type;
}
