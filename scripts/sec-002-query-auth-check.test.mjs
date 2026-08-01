import assert from 'node:assert/strict';
import test from 'node:test';

import { findProhibitedPatterns, validateRepository } from './sec-002-query-auth-check.mjs';

test('detects backend session credential query reads', () => {
  const findings = findProhibitedPatterns('apps/example/server/auth.go', 'value := r.URL.Query().Get("access_token")');
  assert.equal(findings.length, 1);
  assert.match(findings[0], /backend credential query read/);
});

test('detects frontend session credential URL construction', () => {
  const findings = findProhibitedPatterns('apps/example-frontend/src/ws.ts', 'const url = `/ws?token=${session}`;');
  assert.equal(findings.length, 1);
  assert.match(findings[0], /frontend credential query construction/);
});

test('does not flag bounded tickets or non-sensitive query parameters', () => {
  const findings = findProhibitedPatterns('apps/example-frontend/src/ws.ts', 'const url = `/ws?contest_id=${id}&ticket=${ticket}`;');
  assert.deepEqual(findings, []);
});

test('current repository satisfies SEC-002 structural invariants', () => {
  const result = validateRepository();
  assert.deepEqual(result.failures, []);
  assert.ok(result.scannedFiles > 0);
});