#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

function read(relativePath) {
  return fs.readFileSync(path.join(root, relativePath), 'utf8');
}

function sourceFiles(directory, suffix) {
  const base = path.join(root, directory);
  if (!fs.existsSync(base)) return [];
  const files = [];
  const visit = (current) => {
    for (const entry of fs.readdirSync(current, { withFileTypes: true })) {
      if (['node_modules', 'dist', 'coverage', '.codex-cache'].includes(entry.name)) continue;
      const target = path.join(current, entry.name);
      if (entry.isDirectory()) visit(target);
      else if (entry.name.endsWith(suffix)) files.push(target);
    }
  };
  visit(base);
  return files;
}

export function unsafeLoggerConstructions(text) {
  const failures = [];
  const lines = text.split(/\r?\n/);
  lines.forEach((line, index) => {
    if (!line.includes('zap.NewProduction()')) return;
    const following = lines.slice(index + 1, index + 7).join('\n');
    if (!/(WrapLogger|ensureRedactingLogger)\s*\(/.test(following)) {
      failures.push(`line ${index + 1}: zap.NewProduction is not followed by the central wrapper`);
    }
  });
  if (/middleware\.(?:Logger|Recoverer)\b/.test(text)) {
    failures.push('generic chi request/recovery logger bypasses centralized redaction');
  }
  if (/log\.Printf\([^\n]*(?:panic|panicked)[^\n]*%[+]?v/i.test(text)) {
    failures.push('plain panic logging formats an unsanitized recovered value');
  }
  return failures;
}

export function validateRepository() {
  const failures = [];
  const requireText = (file, patterns) => {
    const absolute = path.join(root, file);
    if (!fs.existsSync(absolute)) {
      failures.push(`${file}: required path is missing`);
      return;
    }
    const text = fs.readFileSync(absolute, 'utf8');
    for (const pattern of patterns) {
      if (!pattern.test(text)) failures.push(`${file}: missing ${pattern}`);
    }
  };

  requireText('packages/observability/redaction.go', [/RedactedValue = "\[REDACTED\]"/, /func RedactText/, /func RedactValue/, /func RedactHeaders/, /func RedactURL/, /func InstallStandardLoggerRedaction/]);
  requireText('packages/observability/redacting_core.go', [/func NewRedactingCore/, /func WrapLogger/, /func RedactFields/]);
  requireText('packages/observability/middleware.go', [/func \(m \*HTTPMiddleware\) Recovery/, /generateCorrelationID/, /normalizeCorrelationID/]);
  requireText('packages/observability/sentry.go', [/func RedactSentryEvent/, /QueryString = ""/, /Cookies = RedactedValue/]);
  requireText('packages/audit/audit.go', [/sanitizedMetadata\(entry.Metadata\)/]);
  requireText('packages/secrets/secrets.go', [/func MaskSecret/, /return RedactedValue/]);
  requireText('packages/frontend-shared/src/utils/logger.ts', [/redactForLogging/, /installConsoleRedaction/, /REDACTED_VALUE/]);
  requireText('apps/user-frontend/src/main.ts', [/installConsoleRedaction\(\)/]);
  requireText('apps/admin-frontend/src/main.ts', [/installConsoleRedaction\(\)/]);
  requireText('docs/security/secure-observability-and-redaction.md', [/SEC-005/, /SEC-006/, /SEC-007/, /NO-GO/]);
  requireText('docs/codex/reports/SEC-005-local-execution-report.md', [/SEC-005 (?:PASS|FAIL)/]);

  for (const file of sourceFiles('apps', '.go').concat(sourceFiles('packages', '.go'))) {
    if (file.endsWith('_test.go')) continue;
    const relative = path.relative(root, file).replaceAll('\\', '/');
    for (const failure of unsafeLoggerConstructions(fs.readFileSync(file, 'utf8'))) {
      failures.push(`${relative}: ${failure}`);
    }
  }

  for (const file of [
    'apps/admin-bff/server/app.go',
    'apps/user-bff/server/app.go',
    'apps/trade-bff/server/app.go',
    'apps/payment-service/server/app.go',
  ]) {
    const text = read(file);
    const initCount = (text.match(/sentry\.Init\(/g) || []).length;
    const hookCount = (text.match(/BeforeSend:\s+observability\.RedactSentryEvent/g) || []).length;
    if (initCount !== hookCount) failures.push(`${file}: every Sentry client must use the central before-send hook`);
  }

  const userAuth = read('apps/user-frontend/src/stores/auth.ts');
  if (/Sentry\.setUser\([\s\S]{0,300}(?:email|username):\s*userData\./.test(userAuth)) {
    failures.push('apps/user-frontend/src/stores/auth.ts: direct personal fields remain in Sentry identity');
  }
  for (const laterReport of ['SEC-006-local-execution-report.md', 'SEC-007-local-execution-report.md', 'phase-1-exit-report.md']) {
    if (fs.existsSync(path.join(root, 'docs/codex/reports', laterReport))) failures.push(`docs/codex/reports/${laterReport}: later task/gate was started`);
  }
  return failures;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const failures = validateRepository();
  if (failures.length) {
    console.error(`SEC-005 structural validation failed (${failures.length} finding(s)):`);
    failures.forEach((failure) => console.error(`- ${failure}`));
    process.exit(1);
  }
  console.log('SEC-005 structural validation passed.');
}
