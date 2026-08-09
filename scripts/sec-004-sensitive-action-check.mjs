import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const read = relative => fs.readFileSync(path.join(root, relative), 'utf8');

function requireFragments(file, required, forbidden = []) {
  const text = read(file);
  const failures = [];
  for (const fragment of required) if (!text.includes(fragment)) failures.push(`${file}: missing ${fragment}`);
  for (const fragment of forbidden) if (text.includes(fragment)) failures.push(`${file}: forbidden ${fragment}`);
  return failures;
}

export function findCredentialHandlingViolations(file, text) {
  const failures = [];
  const patterns = [
    ['grant in URL/query', /(?:searchParams\.set\s*\(\s*['"](?:grant|reauth(?:entication)?_grant)['"]|[?&](?:grant|reauth(?:entication)?_grant)=|query\.(?:grant|reauth))/gi],
    ['grant persisted in browser storage', /(?:localStorage|sessionStorage)\s*\.\s*(?:setItem|\w+)\s*\([^\n]*(?:grant|reauth)/gi],
    ['password or grant logged', /(?:console\.|zap\.(?:String|Any)|log\.(?:Print|Info|Warn|Error))[^(]*\([^\n]*(?:password|reauthentication[_ ]?grant|grant\b)/gi],
  ];
  for (const [label, expression] of patterns) {
    for (const match of text.matchAll(expression)) {
      const line = text.slice(0, match.index).split('\n').length;
      failures.push(`${file}:${line}: ${label}`);
    }
  }
  return failures;
}

export function validateSEC004Repository() {
  const failures = [];
  failures.push(...requireFragments('packages/auth/reauthentication.go', [
    'MaxReauthenticationTTL      = 5 * time.Minute',
    'Context:             expectation.Context',
    'SessionDigest:       ReauthenticationBindingDigest',
    'SecurityFingerprint',
    'rand.Read(raw)',
    'consumeReauthenticationScript',
    'ErrReauthenticationReplayed',
  ]));
  failures.push(...requireFragments('packages/auth/middleware.go', [
    'RequireRole(RoleSupportAdmin, RoleSuperAdmin)',
    'HasRole(ctx, RoleSupportAdmin) || HasRole(ctx, RoleSuperAdmin)',
  ], ['RequireRole("viewer", "admin", "super_admin")']));
  failures.push(...requireFragments('apps/admin-bff/server/app.go', [
    'Post("/reauthenticate", app.handleAdminReauthenticate)',
    'requireSensitiveAction(actionWithdrawalComplete',
    'requireSensitiveAction(actionWalletAdjust',
    'requireSensitiveAction(actionUserRolesUpdate',
  ], ['Post("/2fa/login"']));
  failures.push(...requireFragments('apps/admin-bff/server/reauthentication.go', [
    'adminReauthenticationHeader = "X-Admin-Reauth-Grant"',
    'actionElevatedUserCreate',
    'a.auth.VerifyPassword(req.Password, state.PasswordHash)',
    'a.auth.Session.Get',
    'admin.reauthentication.grant_consumed',
    'admin.sensitive_action.denied',
    'state.hasRole(auth.RoleSuperAdmin)',
    'state.hasPermission(permission)',
    'wrong_session_grant',
    'wrong_action_grant',
    'wrong_resource_grant',
  ], ['RoleFinance', '"finance"']));
  failures.push(...requireFragments('apps/admin-bff/server/handlers_user_management.go', [
    'actionUserRolesUpdate, userID, "mandatory_reason_denied"',
    'actionElevatedUserCreate, req.Email, "mandatory_reason_denied"',
  ]));
  failures.push(...requireFragments('apps/admin-bff/server/handlers_withdrawal.go', [
    'actionWalletAdjust, userID, "mandatory_reason_denied"',
    'actionWithdrawalComplete, withdrawalID, "mandatory_reason_denied"',
  ]));
  failures.push(...requireFragments('apps/admin-bff/server/handlers_helpers.go', [
    'Super Admin password verification establishes only the first factor',
    'admin_mfa_credentials',
    'LoginWithPermissions',
  ], ['requires_2fa']));
  failures.push(...requireFragments('apps/admin-frontend/src/api/reauthentication.ts', [
    "'/api/admin/reauthenticate'",
    "'X-Admin-Reauth-Grant'",
    'return operation(response.data.grant)',
  ], ['localStorage', 'sessionStorage', 'console.']));
  failures.push(...requireFragments('apps/admin-frontend/src/stores/auth.ts', [
    "api.post<LoginResponse>('/api/admin/auth/login', { email, password })",
    "adminRole.value === 'support_admin'",
  ], ['/api/admin/auth/2fa/login', 'requires_2fa']));
  failures.push(...requireFragments('packages/db/migrations/0099_admin_canonical_roles.up.sql', [
    "VALUES ('support_admin')",
    "p.name IN ('kyc.view', 'kyc.review')",
    "legacy.name = 'admin'",
  ], ['finance']));
  failures.push(...requireFragments('docs/security/sensitive-action-password-reauthentication.md', [
    'requires the Admin-only `super_admin_totp_v1` assurance',
    '`SEC-007`',
    'paid-production status remains `NO-GO`',
  ]));

  const credentialFiles = [
    'packages/auth/reauthentication.go',
    'apps/admin-bff/server/reauthentication.go',
    'apps/admin-frontend/src/api/reauthentication.ts',
    'apps/admin-frontend/src/modules/admin/views/WithdrawalsPage.vue',
    'apps/admin-frontend/src/modules/admin/views/UserDetailPage.vue',
    'apps/admin-frontend/src/modules/admin/views/UsersPage.vue',
  ];
  for (const file of credentialFiles) failures.push(...findCredentialHandlingViolations(file, read(file)));

  const source = credentialFiles.map(read).join('\n');
  if (/otpauth:\/\//i.test(source) || /totp[_ -]?(?:enrollment|challenge|secret)/i.test(source)) {
    failures.push('SEC-004 changed source contains an active-looking TOTP feature');
  }
  return { failures, scannedFiles: credentialFiles.length + 9 };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const result = validateSEC004Repository();
  if (result.failures.length) {
    for (const failure of result.failures) console.error(`FAIL ${failure}`);
    process.exitCode = 1;
  } else {
    console.log(`PASS SEC-004 structural validation (${result.scannedFiles} files)`);
  }
}
