#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { validatePayment4Retirement } from './payment4-retirement-check.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function read(relativePath) {
  return fs.readFileSync(path.join(root, relativePath), 'utf8');
}

export function policyClasses(source) {
  return new Set(Array.from(source.matchAll(/Class[A-Za-z]+\s+EndpointClass\s*=\s*"([^"]+)"/g), (match) => match[1]));
}

export function missingPolicyClasses(source, required) {
  const declarations = new Map(Array.from(
    source.matchAll(/(Class[A-Za-z]+)\s+EndpointClass\s*=\s*"([^"]+)"/g),
    (match) => [match[2], match[1]],
  ));
  return required.filter((name) => {
    const symbol = declarations.get(name);
    return !symbol || (source.match(new RegExp('\\b' + symbol + '\\b', 'g')) || []).length < 2;
  });
}

export function unsafeProxyReads(relativePath, source) {
  const findings = [];
  if (/\b(?:Use|With)\(\s*middleware\.RealIP\b/.test(source)) findings.push(relativePath + ': generic RealIP middleware trusts forwarding headers');
  if (/Header\.Get\("(?:X-Forwarded-For|X-Real-IP|Forwarded|CF-Connecting-IP)"\)/.test(source)) {
    findings.push(relativePath + ': public service reads a proxy identity header directly');
  }
  return findings;
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

export function validateSEC006Repository() {
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

  const policy = requireText('packages/resilience/ratelimit/policy.go', [
    /type EndpointClass string/, /func \(m \*PolicyMiddleware\) Handler/, /func \(m \*PolicyMiddleware\) ActorHandler/,
    /sec006:edge:/, /storage_unavailable/, /Retry-After/, /PoliciesForService/,
  ]);
  const requiredClasses = [
    'public_read', 'login', 'registration', 'otp_request', 'otp_verify', 'password_reset',
    'contest_join', 'order', 'cancel', 'deposit', 'withdrawal', 'admin', 'webhook', 'websocket',
  ];
  for (const name of missingPolicyClasses(policy, requiredClasses)) failures.push('rate policy missing endpoint class: ' + name);

  requireText('packages/resilience/ratelimit/login_lockout.go', [/type LoginLockout struct/, /loginFailureScript/, /sec006:lockout:/, /func \(l \*LoginLockout\) Success/]);
  requireText('packages/resilience/ratelimit/policy_test.go', [/TestPolicyMiddlewareLimitAndSafeKeys/, /TestActorPolicyUsesAuthenticatedContext/, /TestRedisPolicyAndLoginLockoutIntegration/]);
  requireText('packages/validation/ip.go', [/TRUSTED_PROXY_CIDRS/, /ExtractClientIPWithProxies/, /for i := len\(chain\) - 1; i >= 0; i--/, /IsSecureRequest/]);
  requireText('packages/validation/edge_config.go', [/LoadAndValidateEdgeEnvironment/, /EDGE_MAX_BODY_BYTES/, /EDGE_MAX_UPLOAD_BYTES/, /EDGE_MAX_HEADER_BYTES/, /production TRUSTED_PROXY_CIDRS is required/]);
  requireText('packages/validation/cors.go', [/USER_CORS_ALLOWED_ORIGINS/, /ADMIN_CORS_ALLOWED_ORIGINS/, /TRADE_CORS_ALLOWED_ORIGINS/, /PAYMENT_CORS_ALLOWED_ORIGINS/, /credentialed wildcard origin/, /CORS_ORIGIN_DENIED/]);
  requireText('packages/validation/csrf.go', [/UserBFFCSRFConfig/, /AdminBFFCSRFConfig/, /CookieNames/, /CSRF_ORIGIN_MISSING/]);
  requireText('packages/validation/middleware.go', [/MaxBytesReader/, /MaxBytesMiddleware/, /ContentTypeMiddleware/, /MaxHeaderBytesError|PAYLOAD_TOO_LARGE/, /Cross-Origin-Opener-Policy/, /IsSecureRequest/]);
  requireText('packages/validation/edge_security_test.go', [/TestTrustedProxyClientIPBoundary/, /TestRequestLimitsFramingAndContentType/, /TestUserAndAdminCORSContextsAreExactAndDistinct/, /TestCSRFBrowserAndBearerContexts/]);

  for (const service of ['user-bff', 'admin-bff', 'trade-bff', 'payment-service']) {
    const file = 'apps/' + service + '/server/app.go';
    const text = requireText(file, [/LoadAndValidateEdgeEnvironment/, /NewPolicyMiddleware/, /SecurityHeadersMiddleware/, /ContentTypeMiddleware/, /MaxBytesMiddleware/]);
    for (const finding of unsafeProxyReads(file, text)) failures.push(finding);
    const headersAt = text.indexOf('SecurityHeadersMiddleware');
    const rateAt = text.indexOf('edgePolicy.Handler');
    if (headersAt < 0 || rateAt < 0 || headersAt > rateAt) failures.push(file + ': security headers must wrap rate-limit rejection');
  }

  requireText('apps/trade-bff/server/ws_origin.go', [/TradeBFFCORSConfig/, /origin == ""/, /return false/]);
  requireText('apps/trade-bff/server/ws_origin_test.go', [/no origin header is rejected/, /wildcard origin pattern is rejected/, /explicit codespace origin is allowed/]);
  for (const failure of validatePayment4Retirement().failures) {
    failures.push('Payment4 retirement: ' + failure);
  }
  requireText('apps/payment-service/handlers/webhook_security.go', [/requireTimestamp/, /errWebhookReplay/, /SetNX/, /sec006:webhook:replay:/]);
  requireText('apps/payment-service/handlers/webhook_security_test.go', [/TestWebhookSecurityFreshnessReplayAndSafeKey/, /TestWebhookSecurityRedisReplayIntegration/]);

  requireText('packages/sms/otp.go', [/CanonicalOTPCooldown/, /CanonicalOTPMaxAttempts/, /phoneAuthPurpose/, /RecordFailure/, /Consume/]);
  requireText('packages/sms/otp_security_properties_test.go', [/TestOTPConcurrentIssueCreatesOneActiveCode/]);
  requireText('packages/sms/otp_redis_integration_test.go', [/purpose/i, /replay/i]);
  requireText('apps/user-bff/server/app.go', [/const userSecurityContext = "user"/, /PoliciesForService\(userSecurityContext\)/, /Namespace: userSecurityContext/, /distributedLoginLockout/]);
  requireText('apps/admin-bff/server/app.go', [/distributedLoginLockout/, /edgePolicy\.ActorHandler/]);

  const compose = requireText('infra/docker/docker-compose.yml', [/USER_CORS_ALLOWED_ORIGINS/, /ADMIN_CORS_ALLOWED_ORIGINS/, /TRUSTED_PROXY_CIDRS/, /EDGE_MAX_HEADER_BYTES/, /172\.30\.0\.0\/24/]);
  if (/ALLOWED_ORIGINS:.*\*/.test(compose)) failures.push('Docker Compose retains a wildcard browser origin');
  requireText('.env.example', [/USER_CORS_ALLOWED_ORIGINS=/, /ADMIN_CORS_ALLOWED_ORIGINS=/, /TRUSTED_PROXY_CIDRS=/, /EDGE_MAX_BODY_BYTES=/, /PAYMENT_WEBHOOK_MAX_AGE=/]);
  requireText('apps/gateway/includes/security-headers.conf', [/X-XSS-Protection "0"/, /Strict-Transport-Security \$hsts_header/]);
  const productionGateway = requireText('apps/gateway/nginx.prod.conf', [/broad RFC1918 trust is prohibited/, /\$admin_cors_origin/, /\$user_cors_origin/]);
  if (/set_real_ip_from\s+(?:10\.0\.0\.0\/8|172\.16\.0\.0\/12|192\.168\.0\.0\/16)/.test(productionGateway)) {
    failures.push('production gateway retains broad private-network proxy trust');
  }
  requireText('docs/security/edge-security-and-abuse-controls.md', [/Endpoint and abuse-policy matrix|Entry-point and abuse-policy matrix/, /Trusted proxy/, /Payment webhooks/, /SEC-005/, /SEC-007 implements the separate Super Admin MFA control/, /NO-GO/]);
  requireText('docs/codex/reports/SEC-006-git-execution-report.md', [/SEC-006 (?:PASS|FAIL)/, /Every command executed/, /Pull-request URL and number/, /Paid-production status/]);

  const prohibitedReports = ['phase-1-exit-report.md'];
  for (const report of prohibitedReports) {
    if (fs.existsSync(path.join(root, 'docs/codex/reports', report))) failures.push('later task or gate was started: ' + report);
  }

  for (const file of [
    'docs/security/edge-security-and-abuse-controls.md',
    'docs/product/payment4-retirement-policy-amendment.md',
    'docs/codex/reports/SEC-006-git-execution-report.md',
  ]) {
    const absolute = path.join(root, file);
    if (!fs.existsSync(absolute)) continue;
    for (const target of findBrokenMarkdownLinks(fs.readFileSync(absolute, 'utf8'), path.dirname(absolute))) {
      failures.push(file + ': broken local link ' + target);
    }
    const roadmap = read('docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md');
    for (const match of fs.readFileSync(absolute, 'utf8').matchAll(/\b(?:FND|SEC|ARCH|DATA|OPS|PRIZE|PAY|MD|TRD|CONTEST)-\d{3}\b/g)) {
      if (!roadmap.includes(match[0])) failures.push(file + ': unknown roadmap task ' + match[0]);
    }
  }
  return failures;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const failures = validateSEC006Repository();
  if (failures.length) {
    console.error('SEC-006 structural validation failed (' + failures.length + ' finding(s)):');
    failures.forEach((failure) => console.error('- ' + failure));
    process.exit(1);
  }
  console.log('SEC-006 structural validation passed.');
}
