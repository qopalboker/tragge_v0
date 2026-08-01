import assert from 'node:assert/strict';
import test from 'node:test';

import { buildWebSocketURL } from './webSocketUrl.js';

const aliases = ['token', 'access_token', 'jwt', 'auth_token', 'session_token'];

function assertNoSessionCredential(value) {
  const url = new URL(value);
  for (const alias of aliases) assert.equal(url.searchParams.has(alias), false);
}

test('builds a ticket-only authenticated WebSocket URL', () => {
  const built = buildWebSocketURL(
    'wss://app.example.invalid/ws/trade?contest_id=contest-1',
    'bounded-ticket-fixture',
    'msgpack',
  );
  const url = new URL(built);
  assert.equal(url.searchParams.get('contest_id'), 'contest-1');
  assert.equal(url.searchParams.get('ticket'), 'bounded-ticket-fixture');
  assert.equal(url.searchParams.get('encoding'), 'msgpack');
  assertNoSessionCredential(built);
});

test('reconnect construction replaces the bounded ticket without adding credentials', () => {
  const first = buildWebSocketURL('wss://app.example.invalid/ws/trade?contest_id=contest-1', 'ticket-1', 'json');
  const second = buildWebSocketURL(first, 'ticket-2', 'json');
  assert.equal(new URL(second).searchParams.get('ticket'), 'ticket-2');
  assertNoSessionCredential(second);
});

test('rejects every reusable session credential alias already present in a URL', () => {
  for (const alias of aliases) {
    assert.throws(
      () => buildWebSocketURL(`wss://app.example.invalid/ws/trade?${alias}=credential-fixture`, null, 'json'),
      /forbidden/,
    );
  }
});