import assert from 'node:assert/strict';
import test from 'node:test';

import { unsafeLoggerConstructions } from './sec-005-redaction-check.mjs';

test('rejects an unwrapped production zap logger', () => {
  const findings = unsafeLoggerConstructions('logger, _ := zap.NewProduction()\nlogger.Info("ready")');
  assert.equal(findings.length, 1);
  assert.match(findings[0], /central wrapper/);
});

test('accepts an immediately wrapped production zap logger', () => {
  const findings = unsafeLoggerConstructions('logger, _ := zap.NewProduction()\nlogger = observability.WrapLogger(logger)');
  assert.deepEqual(findings, []);
});

test('rejects generic recovery and raw panic formatting', () => {
  const findings = unsafeLoggerConstructions('r.Use(middleware.Recoverer)\nlog.Printf("worker panicked: %v", recovered)');
  assert.equal(findings.length, 2);
});

test('accepts centralized recovery and explicit panic redaction', () => {
  const findings = unsafeLoggerConstructions('r.Use(obs.Middleware.Recovery)\nlog.Printf("worker panicked: %s", observability.RedactPanic(recovered))');
  assert.deepEqual(findings, []);
});
