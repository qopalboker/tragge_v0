import assert from 'node:assert/strict';
import test from 'node:test';

import {
  findProhibitedSecurityCodePatterns,
  validateRepository,
} from './sec-003-otp-delivery-check.mjs';

test('detects structured OTP and reset credential logging', () => {
  const source = [
    'logger.Info("sent", zap.String("code", code))',
    'logger.Warn("reset", zap.String("reset_token", token))',
  ].join('\n');
  const findings = findProhibitedSecurityCodePatterns('apps/example/security.go', source);
  assert.equal(findings.length, 2);
  assert.match(findings[0], /security credential in structured log field/);
});

test('detects unkeyed small-space code hashing', () => {
  const findings = findProhibitedSecurityCodePatterns(
    'apps/example/security.go',
    'digest := sha256.Sum256([]byte(code))',
  );
  assert.equal(findings.length, 1);
  assert.match(findings[0], /unkeyed small-space code digest/);
});

test('does not flag HMAC or masked delivery diagnostics', () => {
  const source = [
    'mac := hmac.New(sha256.New, key)',
    'logger.Info("accepted", zap.String("destination_masked", masked))',
  ].join('\n');
  assert.deepEqual(findProhibitedSecurityCodePatterns('apps/example/security.go', source), []);
});

test('current repository satisfies SEC-003 structural invariants', () => {
  const result = validateRepository();
  assert.deepEqual(result.failures, []);
  assert.ok(result.scannedFiles > 0);
});
