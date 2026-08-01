import assert from 'node:assert/strict';
import test from 'node:test';

import {
  findCredentialHandlingViolations,
  validateSEC004Repository,
} from './sec-004-sensitive-action-check.mjs';

test('detects grant URLs, browser persistence, and credential logging', () => {
  const source = [
    "url.searchParams.set('grant', grant)",
    "localStorage.setItem('reauth_grant', grant)",
    'logger.Info("value", zap.String("password", password))',
  ].join('\n');
  const failures = findCredentialHandlingViolations('example.ts', source);
  assert.equal(failures.length, 3);
});

test('allows request-body password and dedicated grant header', () => {
  const source = [
    "api.post('/api/admin/reauthenticate', { password, action, resource_id })",
    "headers['X-Admin-Reauth-Grant'] = grant",
  ].join('\n');
  assert.deepEqual(findCredentialHandlingViolations('safe.ts', source), []);
});

test('current repository satisfies SEC-004 structural invariants', () => {
  const result = validateSEC004Repository();
  assert.deepEqual(result.failures, []);
  assert.ok(result.scannedFiles >= 10);
});
