import { readFileSync } from 'node:fs';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { post } = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('./client', () => ({ api: { post } }));

import {
  SensitiveAdminAction,
  reauthenticationHeaders,
  withPasswordReauthentication,
} from './reauthentication';

describe('SEC-004 Admin password reauthentication', () => {
  beforeEach(() => {
    post.mockReset();
  });

  it('sends the password only to the dedicated endpoint and uses the opaque grant once', async () => {
    post.mockResolvedValue({ data: { grant: 'opaque-one-time-grant', expires_at: '2026-07-29T12:05:00Z' } });
    const operation = vi.fn().mockResolvedValue('done');

    const result = await withPasswordReauthentication({
      password: 'local-test-password',
      action: SensitiveAdminAction.WithdrawalComplete,
      resourceId: 'withdrawal-1',
    }, operation);

    expect(result).toBe('done');
    expect(post).toHaveBeenCalledWith('/api/admin/reauthenticate', {
      password: 'local-test-password',
      action: 'withdrawal.complete',
      resource_id: 'withdrawal-1',
    });
    expect(operation).toHaveBeenCalledTimes(1);
    expect(operation).toHaveBeenCalledWith('opaque-one-time-grant');
    const source = readFileSync(new URL('./reauthentication.ts', import.meta.url), 'utf8');
    expect(source).not.toContain('localStorage');
    expect(source).not.toContain('sessionStorage');
    expect(JSON.stringify(post.mock.calls)).not.toContain('?');
  });

  it('transmits a grant only in the dedicated header', () => {
    expect(reauthenticationHeaders('opaque-one-time-grant')).toEqual({
      'X-Admin-Reauth-Grant': 'opaque-one-time-grant',
    });
  });
});
