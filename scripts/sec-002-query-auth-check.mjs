import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
export const REPO_ROOT = path.resolve(SCRIPT_DIR, '..');
const SOURCE_EXTENSIONS = new Set(['.go', '.ts', '.tsx', '.vue', '.js', '.mjs', '.cjs', '.conf', '.yaml', '.yml']);
const SKIP_DIRS = new Set(['node_modules', 'vendor', '.git', 'dist', 'coverage']);
const CREDENTIAL_NAMES = '(?:token|access_token|jwt|auth_token|session_token)';

function walk(directory) {
  const files = [];
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    if (SKIP_DIRS.has(entry.name)) continue;
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...walk(absolute));
    else if (SOURCE_EXTENSIONS.has(path.extname(entry.name))) files.push(absolute);
  }
  return files;
}

function relative(file) {
  return path.relative(REPO_ROOT, file).replaceAll('\\', '/');
}

function lineNumber(text, offset) {
  return text.slice(0, offset).split('\n').length;
}

export function findProhibitedPatterns(file, text) {
  const normalized = file.replaceAll('\\', '/');
  const findings = [];
  const isTest = /(?:_test\.go|\.(?:test|spec)\.[jt]sx?)$/.test(normalized) || normalized.includes('/e2e/');
  const checks = [];

  if (!isTest && normalized.endsWith('.go')) {
    checks.push({
      label: 'backend credential query read',
      pattern: new RegExp(`(?:URL\\.Query\\(\\)|Query\\(\\))\\.Get\\(["']${CREDENTIAL_NAMES}["']\\)`, 'gi'),
    });
  }
  if (!isTest && (normalized.includes('-frontend/src/') || normalized.includes('packages/frontend-shared/src/'))) {
    checks.push(
      { label: 'frontend credential query construction', pattern: new RegExp(`[?&]${CREDENTIAL_NAMES}=`, 'gi') },
      { label: 'frontend credential search-param construction', pattern: new RegExp(`searchParams\\.(?:set|append)\\(["']${CREDENTIAL_NAMES}["']`, 'gi') },
      { label: 'legacy WebSocket token option', pattern: /WebSocketToken|token\?\s*:\s*WebSocketToken/gi },
    );
  }

  for (const check of checks) {
    for (const match of text.matchAll(check.pattern)) {
      findings.push(`${normalized}:${lineNumber(text, match.index)}: ${check.label}`);
    }
  }
  return findings;
}

function requireText(relativePath, requiredFragments, forbiddenFragments = []) {
  const absolute = path.join(REPO_ROOT, relativePath);
  if (!fs.existsSync(absolute)) throw new Error(`missing required file: ${relativePath}`);
  const text = fs.readFileSync(absolute, 'utf8');
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
  const sourceFiles = ['apps', 'packages', 'infra'].flatMap((entry) => walk(path.join(REPO_ROOT, entry)));
  for (const file of sourceFiles) {
    const text = fs.readFileSync(file, 'utf8');
    failures.push(...findProhibitedPatterns(relative(file), text));
  }

  failures.push(...requireText('packages/auth/middleware.go', [
    'HasProhibitedCredentialQuery',
    'url_authentication_unsupported',
    'extracts a token only from the Authorization header',
    'RedactSecurityCredentialsForTelemetry',
    'RestoreSecurityCredentialsAfterTelemetry',
    'clone.Header.Del("Authorization")',
    'clone.Header.Del("Cookie")',
  ], [
    'r.URL.Query().Get("token")',
    'Fallback: check query parameter',
  ]));

  failures.push(...requireText('apps/trade-bff/server/ws_ticket.go', [
    'defaultWSTicketTTL        = 10 * time.Second',
    'wsTicketPurpose           = "trade_websocket_handshake"',
    'Context:     s.context',
    'SessionID:   sessionID',
    'BindingHash: credentialDigest(binding)',
    'GetDel(ctx, key)',
    'ws_ticket:" + string(s.context) + ":" + credentialDigest(ticket)',
    'SameSite: http.SameSiteStrictMode',
    'HttpOnly: true',
    'redactWSTicketForTelemetry',
    'query.Set("ticket", auth.RedactedCredentialValue)',
  ]));

  for (const service of [
    'apps/user-bff/server/app.go',
    'apps/admin-bff/server/app.go',
    'apps/trade-bff/server/app.go',
  ]) {
    failures.push(...requireText(service, [
      'auth.RedactSecurityCredentialsForTelemetry',
      'auth.RestoreSecurityCredentialsAfterTelemetry',
    ]));
  }

  failures.push(...requireText('apps/user-frontend/src/modules/trade/composables/useWebSocket.ts', [
    'connection rejected locally',
    'buildWebSocketURL(wsUrl, ticket, encoding)',
  ], [
    'token=${encodeURIComponent',
    'WebSocketToken',
    'falling back to token query param',
  ]));

  for (const gateway of ['apps/gateway/nginx.conf', 'apps/gateway/nginx.prod.conf']) {
    failures.push(...requireText(gateway, [
      '(token|access_token|jwt|auth_token|session_token|ticket)=',
      '$sanitized_request_uri',
      '"$uri?[REDACTED]"',
    ], [
      '$http_referer',
    ]));
  }
  failures.push(...requireText('infra/k8s/base/gateway.yaml', [
    '$request_method $uri $server_protocol',
  ], [
    '"$request"',
  ]));

  for (const download of [
    'apps/user-bff/server/ticket_handlers.go',
    'apps/admin-bff/server/handlers_tickets.go',
  ]) {
    failures.push(...requireText(download, ['Cache-Control", "private, no-store, max-age=0']));
  }
  for (const frontend of [
    'apps/user-frontend/src/modules/user/views/TicketChatPage.vue',
    'apps/admin-frontend/src/modules/admin/views/TicketDetailPage.vue',
  ]) {
    failures.push(...requireText(frontend, [
      "responseType: 'blob'",
      'URL.createObjectURL',
    ]));
  }

  const permittedNonSessionURLTokens = new Set([
    'apps/market-ingestor/server/finnhub_provider.go',
    'packages/notification/email.go',
  ]);
  const literalPattern = /[?&](?:token|access_token|jwt)=/gi;
  for (const file of sourceFiles) {
    const normalized = relative(file);
    if (/(?:_test\.go|\.(?:test|spec)\.[jt]sx?)$/.test(normalized) || normalized.includes('/e2e/')) continue;
    const text = fs.readFileSync(file, 'utf8');
    if (literalPattern.test(text) && !permittedNonSessionURLTokens.has(normalized)) {
      failures.push(`${normalized}: unapproved literal credential-like URL construction`);
    }
    literalPattern.lastIndex = 0;
  }

  return {
    failures,
    scannedFiles: sourceFiles.length,
    justifiedExclusions: [
      'apps/market-ingestor/server/finnhub_provider.go: third-party Finnhub provider API key contract, not a Tragge session credential',
      'packages/notification/email.go: one-time password-reset link fixture, not a reusable session credential',
    ],
  };
}

function main() {
  const result = validateRepository();
  if (result.failures.length > 0) {
    console.error(`SEC-002 structural validation FAIL (${result.failures.length} finding(s))`);
    for (const failure of result.failures) console.error(`- ${failure}`);
    process.exitCode = 1;
    return;
  }
  console.log(`SEC-002 structural validation PASS (${result.scannedFiles} relevant source/config files scanned)`);
  for (const exclusion of result.justifiedExclusions) console.log(`JUSTIFIED EXCLUSION: ${exclusion}`);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();