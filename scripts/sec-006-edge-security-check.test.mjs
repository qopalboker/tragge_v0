import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import {
  findBrokenMarkdownLinks,
  missingPolicyClasses,
  unsafeProxyReads,
  validateSEC006Repository,
} from './sec-006-edge-security-check.mjs';

test('detects missing endpoint-class policies', () => {
  const source = 'ClassLogin EndpointClass = "login"\nPolicy{Class: ClassLogin}';
  assert.deepEqual(missingPolicyClasses(source, ['login', 'webhook']), ['webhook']);
});

test('detects generic and direct proxy-header trust', () => {
  const source = 'r.Use(middleware.RealIP)\nr.Header.Get("X-Forwarded-For")';
  assert.equal(unsafeProxyReads('fixture.go', source).length, 2);
  assert.deepEqual(unsafeProxyReads('fixture.go', 'validation.ExtractClientIP(r)'), []);
});

test('validates local Markdown links', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'sec006-links-'));
  fs.writeFileSync(path.join(directory, 'exists.md'), '# fixture');
  assert.deepEqual(findBrokenMarkdownLinks('[ok](exists.md)', directory, directory), []);
  assert.deepEqual(findBrokenMarkdownLinks('[bad](missing.md)', directory, directory), ['missing.md']);
});

test('current repository satisfies SEC-006 structural invariants', () => {
  assert.deepEqual(validateSEC006Repository(), []);
});
