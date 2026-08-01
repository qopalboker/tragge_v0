import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname.replace(/^\/(?:[A-Za-z]:)/, value => value.slice(1))), '..');
const sourcePath = path.join(root, 'apps/admin-frontend/src/api/reauthentication.ts');
const source = fs.readFileSync(sourcePath, 'utf8');
const require = createRequire(import.meta.url);
const typescript = require(path.join(root, 'apps/admin-frontend/node_modules/typescript'));
const executableSource = source.replace("import { api } from './client';", 'const api = globalThis.__SEC004_API__;');
const transpiled = typescript.transpileModule(executableSource, {
  compilerOptions: { module: typescript.ModuleKind.ES2022, target: typescript.ScriptTarget.ES2022 },
  fileName: sourcePath,
  reportDiagnostics: true,
});
const errors = (transpiled.diagnostics ?? []).filter(item => item.category === typescript.DiagnosticCategory.Error);
assert.deepEqual(errors, []);

globalThis.__SEC004_API__ = { post: async () => { throw new Error('mock not configured'); } };
const moduleUrl = `data:text/javascript;base64,${Buffer.from(transpiled.outputText).toString('base64')}`;
const reauthentication = await import(moduleUrl);

function configureResponses(responses) {
  const calls = [];
  globalThis.__SEC004_API__.post = async (...args) => {
    calls.push(args);
    const response = responses.shift();
    if (!response) throw new Error('unexpected request');
    return response;
  };
  return calls;
}

test('executes the actual frontend helper with password only at the dedicated endpoint', async () => {
  const calls = configureResponses([{ data: { grant: 'opaque-test-grant', expires_at: '2026-07-29T12:05:00Z' } }]);
  let receivedGrant = '';
  const result = await reauthentication.withPasswordReauthentication({
    password: 'local-test-password',
    action: reauthentication.SensitiveAdminAction.WithdrawalComplete,
    resourceId: 'withdrawal-1',
  }, async grant => {
    receivedGrant = grant;
    return 'done';
  });
  assert.equal(result, 'done');
  assert.equal(receivedGrant, 'opaque-test-grant');
  assert.deepEqual(calls, [[
    '/api/admin/reauthenticate',
    { password: 'local-test-password', action: 'withdrawal.complete', resource_id: 'withdrawal-1' },
  ]]);
});

test('an expired operation cannot reuse its grant and a retry performs fresh reauthentication', async () => {
  const calls = configureResponses([
    { data: { grant: 'expired-test-grant', expires_at: '2026-07-29T12:00:00Z' } },
    { data: { grant: 'fresh-test-grant', expires_at: '2026-07-29T12:05:00Z' } },
  ]);
  await assert.rejects(
    reauthentication.withPasswordReauthentication({
      password: 'local-test-password',
      action: reauthentication.SensitiveAdminAction.WalletAdjust,
      resourceId: 'user-1',
    }, async () => { throw new Error('sensitive action denied'); }),
    /sensitive action denied/,
  );
  let freshGrant = '';
  await reauthentication.withPasswordReauthentication({
    password: 'local-test-password',
    action: reauthentication.SensitiveAdminAction.WalletAdjust,
    resourceId: 'user-1',
  }, async grant => { freshGrant = grant; });
  assert.equal(calls.length, 2);
  assert.equal(freshGrant, 'fresh-test-grant');
});

test('frontend source has no grant persistence, URL transport, logging, or TOTP UI contract', () => {
  assert.doesNotMatch(source, /localStorage|sessionStorage|URLSearchParams|console\./);
  assert.doesNotMatch(source, /totp|otpauth|recovery code/i);
  assert.match(source, /X-Admin-Reauth-Grant/);
  assert.match(source, /return operation\(response\.data\.grant\)/);
  assert.equal(pathToFileURL(sourcePath).protocol, 'file:');
});