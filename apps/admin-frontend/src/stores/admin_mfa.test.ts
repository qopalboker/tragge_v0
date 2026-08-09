import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const storeSource = readFileSync(new URL('./auth.ts', import.meta.url), 'utf8');
const loginSource = readFileSync(new URL('../modules/admin/views/LoginPage.vue', import.meta.url), 'utf8');
const englishSource = readFileSync(new URL('../i18n/locales/en.ts', import.meta.url), 'utf8');
const persianSource = readFileSync(new URL('../i18n/locales/fa.ts', import.meta.url), 'utf8');
const userDetailSource = readFileSync(new URL('../modules/admin/views/UserDetailPage.vue', import.meta.url), 'utf8');
const usersAPISource = readFileSync(new URL('../api/users.ts', import.meta.url), 'utf8');
const reauthenticationSource = readFileSync(new URL('../api/reauthentication.ts', import.meta.url), 'utf8');

describe('SEC-007 Super Admin MFA frontend contract', () => {
  it('uses only body-bound Admin MFA endpoints', () => {
    expect(storeSource).toContain("'/api/admin/auth/mfa/enrollment/start'");
    expect(storeSource).toContain("'/api/admin/auth/mfa/enrollment/verify'");
    expect(storeSource).toContain("'/api/admin/auth/mfa/verify'");
    expect(storeSource).not.toMatch(/mfa[^\n]*(localStorage|sessionStorage)/i);
    expect(storeSource).not.toMatch(/[?&](challenge|code|recovery_code)=/);
  });

  it('clears password and one-time MFA inputs in the login flow', () => {
    expect(loginSource).toContain("password.value = ''");
    expect(loginSource).toContain("mfaCode.value = ''");
    expect(storeSource).toContain('mfaChallenge.value = null');
    expect(storeSource).toContain('recoveryCodes.value = []');
  });

  it('presents enrollment and one-time recovery codes without adding a TOTP dependency', () => {
    expect(loginSource).toContain('data-testid="mfa-provisioning"');
    expect(loginSource).toContain('data-testid="mfa-recovery-codes"');
    expect(loginSource).toContain('autocomplete="one-time-code"');
  });

  it('keeps English and Persian MFA copy aligned', () => {
    for (const key of ['mfaTitle', 'mfaPrompt', 'mfaEnrollInstructions', 'mfaRecoverySave', 'mfaError']) {
      expect(englishSource).toContain(`${key}:`);
      expect(persianSource).toContain(`${key}:`);
    }
    expect(persianSource).toContain('تأیید هویت مدیر ارشد');
  });

  it('resets MFA only through password reauthentication and an action-bound grant', () => {
    expect(reauthenticationSource).toContain("AdminMFAReset: 'admin.mfa.reset'");
    expect(userDetailSource).toContain('SensitiveAdminAction.AdminMFAReset');
    expect(userDetailSource).toContain('data-testid="reset-super-admin-mfa"');
    expect(usersAPISource).toContain("`/api/admin/users/${userId}/mfa/reset`");
    expect(usersAPISource).toContain('reauthenticationHeaders(grant)');
    expect(userDetailSource + usersAPISource).not.toMatch(/[?&](?:grant|password|challenge)=/i);
  });

  it('maps canonical Admin roles to the protected-panel access capability', () => {
    expect(storeSource).toContain("if (role === 'admin') return isAdminRole.value");
    expect(storeSource).toContain("adminRole.value === 'support_admin' || adminRole.value === 'super_admin'");
    expect(storeSource).not.toMatch(/adminRole\.value === ['"]finance['"]/i);
  });
});
