import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
export const REPO_ROOT = path.resolve(SCRIPT_DIR, '..');

function read(relativePath) {
  const absolute = path.join(REPO_ROOT, relativePath);
  if (!fs.existsSync(absolute)) throw new Error(`missing required file: ${relativePath}`);
  return fs.readFileSync(absolute, 'utf8');
}

function lineNumber(text, offset) {
  return text.slice(0, offset).split('\n').length;
}

export function findProhibitedSecurityCodePatterns(relativePath, text) {
  const findings = [];
  const patterns = [
    {
      label: 'security credential in structured log field',
      expression: /zap\.(?:String|Any|ByteString)\(\s*["'](?:code|otp|reset_token|password_set_token)["']/gi,
    },
    {
      label: 'security credential in formatted logging',
      expression: /(?:log|logger)\.(?:Printf|Println|Infof|Debugf|Warnf|Errorf)\([^\n]*(?:otp|reset[_ -]?(?:code|token)|verification[_ -]?code)[^\n]*\bcode\b/gi,
    },
    {
      label: 'unkeyed small-space code digest',
      expression: /sha256\.Sum256\(\s*\[\]byte\(\s*(?:code|req\.Code)\s*\)\s*\)/g,
    },
    {
      label: 'non-canonical active security-code TTL',
      expression: /(?:verificationCodeTTL|passwordResetCodeTTL|DefaultOTPTTL)\s*=\s*(?:2|15)\s*\*\s*time\.Minute/g,
    },
    {
      label: 'non-canonical active security-code cooldown',
      expression: /(?:verificationResendCooldown|passwordResetCooldown|DefaultOTPCooldown)\s*=\s*(?:90|120)\s*\*\s*time\.Second/g,
    },
  ];
  for (const { label, expression } of patterns) {
    for (const match of text.matchAll(expression)) {
      findings.push(`${relativePath}:${lineNumber(text, match.index)}: ${label}`);
    }
  }
  return findings;
}

function requireText(relativePath, requiredFragments, forbiddenFragments = []) {
  const text = read(relativePath);
  const failures = [];
  for (const fragment of requiredFragments) {
    if (!text.includes(fragment)) failures.push(`${relativePath}: missing required fragment: ${fragment}`);
  }
  for (const fragment of forbiddenFragments) {
    if (text.includes(fragment)) failures.push(`${relativePath}: forbidden fragment remains: ${fragment}`);
  }
  return failures;
}

export function validateRepository() {
  const failures = [];
  const securitySourceFiles = [
    'apps/user-bff/server/app.go',
    'apps/user-bff/server/verification_handlers.go',
    'apps/user-bff/server/forgot_password_handlers.go',
    'apps/user-bff/server/phone_auth.go',
    'apps/user-bff/server/security_codes.go',
    'packages/sms/kavenegar.go',
    'packages/sms/otp.go',
    'packages/notification/security_email.go',
  ];
  for (const file of securitySourceFiles) {
    failures.push(...findProhibitedSecurityCodePatterns(file, read(file)));
  }

  failures.push(...requireText('apps/user-bff/server/security_codes.go', [
    'securityCodeTTL         = 10 * time.Minute',
    'securityCodeCooldown    = 60 * time.Second',
    'securityCodeMaxAttempts = 5',
    'hmac.New(sha256.New, h.key)',
    'subtle.ConstantTimeCompare',
    'securityCodePurposeEmailVerification',
    'securityCodePurposePhoneVerification',
    'securityCodePurposePasswordReset',
    'normalizeSupportedCountry',
    'if country == "IR"',
    'provider = r.mailerino',
    'provider = r.resend',
    'resolveSecurityEnvironment',
    'return "production", nil',
    'SMSProviderMode), "kavenegar"',
  ]));

  failures.push(...requireText('apps/user-bff/server/verification_handlers.go', [
    'deliverVerificationCode(sendCtx',
    'activationTime.Add(securityCodeTTL)',
    'max_attempts = $2',
    '`UPDATE users SET phone_verified = TRUE WHERE id = $1`',
  ], [
    '2 * time.Minute',
    '90 * time.Second',
    'go a.deliverVerificationCode',
  ]));

  failures.push(...requireText('apps/user-bff/server/forgot_password_handlers.go', [
    'passwordSetTokenTTL      = 10 * time.Minute',
    'created_at > NOW() - INTERVAL \'60 seconds\'',
    'NOW() + INTERVAL \'10 minutes\'',
    'securityCodeMaxAttempts',
    'a.auth.Session.DeleteAllForUser(ctx, userID)',
    'auth:user:password-reset:',
    'deliverPasswordResetCode(sendCtx',
  ], [
    '15 * time.Minute',
    '120 * time.Second',
    'go a.deliverPasswordResetCode',
  ]));

  failures.push(...requireText('packages/sms/otp.go', [
    'CanonicalOTPTTL         = 10 * time.Minute',
    'CanonicalOTPCooldown    = 60 * time.Second',
    'CanonicalOTPMaxAttempts = 5',
    'hmac.New(sha256.New, s.key)',
    'subtle.ConstantTimeCompare',
    's.store.Reserve',
    's.provider.SendOTP',
    's.store.Activate',
    's.store.Consume',
  ]));

  failures.push(...requireText('packages/sms/kavenegar.go', [
    'ErrKaveNegarUnavailable',
    'k.api.Verify.Lookup',
    'intentionally no direct-message fallback',
  ], [
    'log.Printf',
  ]));
  const kaveNegarSource = read('packages/sms/kavenegar.go');
  const otpStart = kaveNegarSource.indexOf('func (k *KaveNegarProvider) SendOTP');
  const messageStart = kaveNegarSource.indexOf('func (k *KaveNegarProvider) SendMessage');
  if (otpStart < 0 || messageStart < 0 || kaveNegarSource.slice(otpStart, messageStart).includes('Message.Send')) {
    failures.push('packages/sms/kavenegar.go: SendOTP contains or cannot exclude a direct-message fallback');
  }

  failures.push(...requireText('packages/notification/security_email.go', [
    'DefaultMailerinoBaseURL = "https://api.mailerino.com"',
    'DefaultResendBaseURL    = "https://api.resend.com"',
    'context.Context',
    'io.LimitReader',
    'ErrSecurityEmailDelivery',
    'User-Agent',
  ], [
    'log.Printf',
    'io.ReadAll(resp.Body)',
  ]));

  failures.push(...requireText('apps/user-bff/server/auth_handlers.go', [
    'v.Required("country", req.Country)',
    'preferred_lang, country)',
    'req.Email, passwordHash, lang, req.Country',
    'a.issueVerificationCode(ctx, userID, "email")',
  ]));
  failures.push(...requireText('apps/user-frontend/src/stores/auth.ts', [
    'country: string;',
    'normalizeRegistrationCountry(profileData.country)',
    'country,',
  ]));
  failures.push(...requireText('apps/user-frontend/src/modules/user/views/LoginPage.vue', [
    "if (!data.country) e.country = t('auth.errorRequired')",
    ":value=\"formData.country\"",
    ":value=\"c.code\"",
  ]));
  failures.push(...requireText('apps/user-frontend/src/modules/user/views/ForgotPasswordPage.vue', [
    'const resetToken = ref',
    'const passwordSetToken = ref',
  ], [
    'localStorage.setItem',
    'console.log',
    'route.query',
  ]));

  const runtimeFakeReferences = [];
  for (const directory of ['apps', 'packages']) {
    const stack = [path.join(REPO_ROOT, directory)];
    while (stack.length > 0) {
      const current = stack.pop();
      for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
        const absolute = path.join(current, entry.name);
        if (entry.isDirectory()) {
          if (!['vendor', 'node_modules'].includes(entry.name)) stack.push(absolute);
        } else if (entry.name.endsWith('.go') && !entry.name.endsWith('_test.go')) {
          const relative = path.relative(REPO_ROOT, absolute).replaceAll('\\', '/');
          if (relative === 'packages/sms/mock.go') continue;
          const text = fs.readFileSync(absolute, 'utf8');
          if (/\b(?:NewFake|NewMock)\s*\(/.test(text)) runtimeFakeReferences.push(relative);
        }
      }
    }
  }
  if (runtimeFakeReferences.length > 0) {
    failures.push(`runtime construction references a fake/mock provider: ${runtimeFakeReferences.join(', ')}`);
  }

  failures.push(...requireText('infra/docker/docker-compose.yml', [
    'SECURITY_CODE_HASH_SECRET_FILE: /run/secrets/security_code_hash_secret',
    'MAILERINO_API_KEY_FILE: /run/secrets/mailerino_api_key',
    'MAILERINO_FROM_EMAIL: ${MAILERINO_FROM_EMAIL:-}',
    'RESEND_FROM_EMAIL: ${RESEND_FROM_EMAIL:-}',
    'SMS_PROVIDER: ${SMS_PROVIDER:-kavenegar}',
  ]));
  failures.push(...requireText('scripts/secrets/init-secrets.sh', [
    'security_code_hash_secret.txt',
    'mailerino_api_key.txt',
    'resend_api_key.txt',
    'kavenegar_api_key.txt',
  ]));
  failures.push(...requireText('docs/security/otp-and-reset-delivery.md', [
    'ten-minute validity',
    '60-second resend cooldown',
    'five verification attempts',
    'Mailerino',
    'Resend',
    'KaveNegar',
    'Paid-production status remains `NO-GO`',
  ]));

  return { failures, scannedFiles: securitySourceFiles.length };
}

function main() {
  const result = validateRepository();
  if (result.failures.length > 0) {
    console.error(`SEC-003 structural validation FAIL (${result.failures.length} finding(s))`);
    for (const failure of result.failures) console.error(`- ${failure}`);
    process.exitCode = 1;
    return;
  }
  console.log(`SEC-003 structural validation PASS (${result.scannedFiles} active security-code source files inspected)`);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
