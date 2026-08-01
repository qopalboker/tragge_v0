import { describe, expect, it, vi } from 'vitest';
import {
  REDACTED_VALUE,
  logger,
  redactForLogging,
  redactTextForLogging,
} from '@tragge/frontend-shared';

const credential = 'sec005-frontend-fixture-never-use';

describe('SEC-005 frontend logging redaction', () => {
  it('redacts case-insensitive nested fields and arrays while preserving safe facts', () => {
    const redacted = redactForLogging({
      Authorization: `Bearer ${credential}`,
      nested: [{ refresh_TOKEN: credential, result: 'denied' }],
      profile: { email: 'fixture@example.invalid', actor_id: 'actor-1234' },
    });
    const output = JSON.stringify(redacted);
    expect(output).not.toContain(credential);
    expect(output).not.toContain('fixture@example.invalid');
    expect(output).toContain(REDACTED_VALUE);
    expect(output).toContain('denied');
    expect(output).toContain('actor-1234');
  });

  it('redacts text credentials and credential-bearing URLs', () => {
    const output = redactTextForLogging(`password=${credential} provider_secret=${credential} national_code=${credential} postgres://fixture:${credential}@localhost/test`);
    expect(output).not.toContain(credential);
    expect(output).toContain(REDACTED_VALUE);
  });

  it('redacts form, URL-search, and error payloads', () => {
    const form = new FormData();
    form.set('reset_token', credential);
    form.set('result', 'retry');
    const output = JSON.stringify(redactForLogging({
      form,
      query: new URLSearchParams({ api_key: credential, page: '2' }),
      error: new Error(`Authorization: Bearer ${credential}`),
    }));
    expect(output).not.toContain(credential);
    expect(output).toContain(REDACTED_VALUE);
    expect(output).toContain('retry');
    expect(output).toContain('2');
  });

  it('redacts actual logger arguments before console emission', () => {
    const capture = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    logger.error('provider failed', { webhook_signature: credential, result: 'rejected' });
    const output = JSON.stringify(capture.mock.calls);
    expect(output).not.toContain(credential);
    expect(output).toContain(REDACTED_VALUE);
    expect(output).toContain('rejected');
    capture.mockRestore();
  });
});
