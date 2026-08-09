import { api } from './client';

export const SensitiveAdminAction = {
  WithdrawalComplete: 'withdrawal.complete',
  WalletAdjust: 'wallet.adjust',
  UserRolesUpdate: 'user.roles.update',
  ElevatedUserCreate: 'user.create.elevated',
  AdminMFAReset: 'admin.mfa.reset',
} as const;

export type SensitiveAdminAction = typeof SensitiveAdminAction[keyof typeof SensitiveAdminAction];

interface ReauthenticationResponse {
  grant: string;
  expires_at: string;
}

export async function withPasswordReauthentication<T>(input: {
  password: string;
  action: SensitiveAdminAction;
  resourceId: string;
}, operation: (grant: string) => Promise<T>): Promise<T> {
  const response = await api.post<ReauthenticationResponse>('/api/admin/reauthenticate', {
    password: input.password,
    action: input.action,
    resource_id: input.resourceId,
  });
  // The grant remains a function-local value: it is never persisted, logged,
  // placed in a URL, or reused by another action.
  return operation(response.data.grant);
}

export function reauthenticationHeaders(grant: string): Record<string, string> {
  return { 'X-Admin-Reauth-Grant': grant };
}
