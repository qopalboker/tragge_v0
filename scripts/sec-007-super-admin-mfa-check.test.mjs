import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import {
  findBrokenMarkdownLinks,
  unsafeMFAFrontendPersistence,
  validateSEC007Repository,
} from './sec-007-super-admin-mfa-check.mjs';

test('detects MFA persistence and URL leakage', () => {
  assert.equal(unsafeMFAFrontendPersistence('localStorage.setItem("mfaChallenge", value)').length, 1);
  assert.equal(unsafeMFAFrontendPersistence('fetch(`/verify?challenge=${value}`)').length, 1);
  assert.deepEqual(unsafeMFAFrontendPersistence('const mfaChallenge = ref<string | null>(null)'), []);
});

test('validates local Markdown links', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'sec007-links-'));
  fs.writeFileSync(path.join(directory, 'exists.md'), '# fixture');
  assert.deepEqual(findBrokenMarkdownLinks('[ok](exists.md)', directory, directory), []);
  assert.deepEqual(findBrokenMarkdownLinks('[bad](missing.md)', directory, directory), ['missing.md']);
});

test('current repository satisfies SEC-007 structural invariants', () => {
  assert.deepEqual(validateSEC007Repository(), []);
});
