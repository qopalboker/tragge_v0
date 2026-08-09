#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function read(relativePath) {
  return fs.readFileSync(path.join(root, relativePath), 'utf8');
}

export function findBrokenMarkdownLinks(text, baseDir, repositoryRoot = root) {
  const failures = [];
  for (const match of text.matchAll(/\[[^\]]+\]\(([^)]+)\)/g)) {
    const target = match[1].trim().replace(/^<|>$/g, '').split('#')[0];
    if (!target || /^(?:https?:|mailto:)/.test(target)) continue;
    const resolved = path.resolve(baseDir, decodeURIComponent(target));
    if (!resolved.startsWith(repositoryRoot + path.sep) || !fs.existsSync(resolved)) failures.push(target);
  }
  return failures;
}

export function unsafeMFAFrontendPersistence(source) {
  const findings = [];
  const credential = '(?:mfa|challenge|recovery(?:Code|Codes)?|provisioning(?:Uri)?)';
  const storage = '(?:localStorage|sessionStorage)';
  if (new RegExp(storage + '[^\\n]{0,120}' + credential, 'i').test(source) ||
      new RegExp(credential + '[^\\n]{0,120}' + storage, 'i').test(source)) {
    findings.push('MFA credential material is coupled to browser persistence');
  }
  if (/[?&](?:mfa|challenge|code|recovery_code)=/i.test(source)) {
    findings.push('MFA credential material is placed in a URL');
  }
  return findings;
}

export function validateSEC007Repository() {
  const failures = [];
  const requireText = (file, patterns) => {
    const absolute = path.join(root, file);
    if (!fs.existsSync(absolute)) {
      failures.push(file + ': required path is missing');
      return '';
    }
    const text = fs.readFileSync(absolute, 'utf8');
    for (const pattern of patterns) {
      if (!pattern.test(text)) failures.push(file + ': missing ' + pattern);
    }
    return text;
  };

  const authMFA = requireText('packages/auth/admin_mfa.go', [
    /AdminMFAConfig/, /enc:admin-mfa:v1:/, /aes\.NewCipher/, /cipher\.NewGCM/,
    /GenerateAdminTOTPSecret/, /MatchAdminTOTPCounter/, /GenerateAdminMFARecoveryCodes/,
    /Consume\(ctx context\.Context/, /redis\.NewScript/, /replayed/,
  ]);
  if (/DecryptTOTPSecret\(/.test(authMFA)) failures.push('Admin MFA uses the legacy shared-domain decryptor');

  requireText('packages/auth/jwt.go', [
    /super_admin_totp_v1/, /MFAAssurance/, /GenerateTokenPairWithSessionPermissionsAndMFA/,
  ]);
  requireText('packages/auth/session.go', [/MFAAssurance/, /mfa_assurance/]);
  requireText('packages/auth/middleware.go', [/RequireSuperAdminMFA/, /MFAAssuranceSuperAdminTOTPV1/, /RequireAdminAccess/]);

  const app = requireText('apps/admin-bff/server/app.go', [
    /\/mfa\/enrollment\/start/, /\/mfa\/enrollment\/verify/, /\/mfa\/verify/,
    /\/mfa\/reset/, /ADMIN_MFA_ENCRYPTION_KEY/, /ADMIN_MFA_RECOVERY_PEPPER/, /ADMIN_MFA_ISSUER/,
  ]);
  if (/\.Post\([^\n]*handleAdmin2FALoginVerify/.test(app)) failures.push('retired shared-user TOTP handler is registered');
  const legacy = requireText('apps/admin-bff/server/handlers_helpers.go', [
    /handleAdmin2FALoginVerify/, /http\.StatusNotFound/, /plaintextSecret := ""/,
    /admin_mfa_credentials/, /stage := "verify"/, /stage = "enroll"/,
  ]);
  if (/DecryptTOTPSecret\(/.test(legacy)) failures.push('legacy Admin login path retains plaintext-compatible secret decryption');

  requireText('apps/admin-bff/server/admin_mfa.go', [
    /handleAdminMFAEnrollmentStart/, /handleAdminMFAEnrollmentVerify/, /handleAdminMFAVerify/,
    /last_totp_counter/, /used_at IS NULL/, /handleAdminMFAReset/,
    /RevokeUser/, /reauthentication\.RevokeActor/, /Session\.DeleteAllForUser/,
  ]);
  requireText('apps/admin-bff/server/admin_mfa_integration_test.go', [
    /TestSEC007SuperAdminMFAPostgresRedisRuntime/, /t\.Run\("TOTP counter is single-use under concurrency"/,
    /t\.Run\("recovery code is single-use under concurrency"/, /t\.Run\("audited reset rolls back on audit failure then revokes state"/,
  ]);

  const migration = requireText('packages/db/migrations/0100_admin_super_mfa.up.sql', [
    /CREATE TABLE admin_mfa_credentials/, /secret_ciphertext/, /last_totp_counter/,
    /CREATE TABLE admin_mfa_recovery_codes/, /code_digest BYTEA/, /used_at/,
  ]);
  if (/\b(?:REAL|DOUBLE PRECISION)\b/i.test(migration)) failures.push('Admin MFA migration uses floating-point state');
  requireText('packages/db/migrations/0100_admin_super_mfa.down.sql', [/DROP TABLE IF EXISTS admin_mfa_recovery_codes/, /DROP TABLE IF EXISTS admin_mfa_credentials/]);

  const frontendStore = requireText('apps/admin-frontend/src/stores/auth.ts', [
    /mfaStage/, /mfaChallenge/, /startMFAEnrollment/, /verifyMFA/, /cancelMFA/,
  ]);
  failures.push(...unsafeMFAFrontendPersistence(frontendStore).map((finding) => 'apps/admin-frontend/src/stores/auth.ts: ' + finding));
  requireText('apps/admin-frontend/src/modules/admin/views/LoginPage.vue', [
    /startMFAEnrollment/, /verifyMFA/, /recoveryCodes|acknowledgeRecoveryCodes/, /mfaProvisioningUri/,
  ]);
  requireText('apps/admin-frontend/src/modules/admin/views/UserDetailPage.vue', [
    /handleResetSuperAdminMFA/, /SensitiveAdminAction\.AdminMFAReset/, /reset-super-admin-mfa/,
  ]);
  requireText('apps/admin-frontend/src/api/users.ts', [/resetSuperAdminMFA/, /\/mfa\/reset/, /reauthenticationHeaders\(grant\)/]);
  requireText('apps/admin-frontend/e2e/admin_mfa.spec.ts', [
    /Super Admin enrolls and receives recovery codes only after TOTP/, /Super Admin can choose a one-time recovery-code challenge without URL leakage/,
    /localStorage/, /sessionStorage/,
  ]);

  requireText('infra/docker/docker-compose.yml', [
    /ADMIN_MFA_ENCRYPTION_KEY_FILE/, /ADMIN_MFA_RECOVERY_PEPPER_FILE/, /ADMIN_MFA_ISSUER/,
    /admin_mfa_encryption_key:/, /admin_mfa_recovery_pepper:/,
  ]);
  requireText('scripts/secrets/init-secrets.sh', [/admin_mfa_encryption_key\.txt/, /admin_mfa_recovery_pepper\.txt/]);
  requireText('.env.example', [/ADMIN_MFA_ENCRYPTION_KEY/, /ADMIN_MFA_RECOVERY_PEPPER/, /ADMIN_MFA_ISSUER/]);

  const policy = requireText('docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md', [
    /Super Admin.*(?:TOTP|MFA)/i, /super_admin_totp_v1/, /Paid-production status remains `NO-GO`/,
  ]);
  if (/Super Admin login may remain password-based/i.test(policy)) failures.push('fixed policy still permits password-only Super Admin login');
  requireText('docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md', [/SEC-007/, /Implement Super Admin MFA/, /super_admin_totp_v1/]);
  requireText('docs/product/canonical-domain-glossary-and-version-catalog.md', [/Super Admin/, /super_admin_totp_v1/, /implemented/i]);
  requireText('docs/security/super-admin-mfa.md', [/Google Authenticator/, /AES-256-GCM/, /recovery code/i, /replay/i, /NO-GO/]);

  for (const file of [
    'docs/security/super-admin-mfa.md',
    'docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md',
    'docs/product/canonical-domain-glossary-and-version-catalog.md',
  ]) {
    const absolute = path.join(root, file);
    if (!fs.existsSync(absolute)) continue;
    for (const target of findBrokenMarkdownLinks(fs.readFileSync(absolute, 'utf8'), path.dirname(absolute))) {
      failures.push(file + ': broken local link ' + target);
    }
  }

  const roadmap = read('docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md');
  const referencedDocs = ['docs/security/super-admin-mfa.md', 'docs/product/canonical-domain-glossary-and-version-catalog.md'];
  for (const file of referencedDocs) {
    for (const match of read(file).matchAll(/\b(?:FND|SEC|ARCH|DATA|OPS|PRIZE|PAY|MD|TRD|CONTEST)-\d{3}\b/g)) {
      if (!roadmap.includes(match[0])) failures.push(file + ': unknown roadmap task ' + match[0]);
    }
  }
  if (/\bFINANCE(?:_ADMIN)?\b/.test([authMFA, app, migration, frontendStore].join('\n'))) failures.push('SEC-007 active scope introduces a Finance role');
  if (fs.existsSync(path.join(root, 'docs/codex/reports/phase-1-exit-report.md'))) failures.push('Phase 1 Exit Gate was started');

  return failures;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const failures = validateSEC007Repository();
  if (failures.length) {
    console.error('SEC-007 structural validation failed (' + failures.length + ' finding(s)):');
    failures.forEach((failure) => console.error('- ' + failure));
    process.exit(1);
  }
  console.log('SEC-007 structural validation passed.');
}
