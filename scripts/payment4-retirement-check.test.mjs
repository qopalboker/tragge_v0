import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import {
	payment4Occurrences,
	permittedRetirementReferences,
	validatePayment4Retirement,
} from './payment4-retirement-check.mjs';

test('current repository contains only rationalized retirement evidence', () => {
	const result = validatePayment4Retirement();
	assert.deepEqual(result.failures, []);
	assert.deepEqual(result.occurrences, [...permittedRetirementReferences.keys()].sort());
});

test('occurrence scan rejects an active configuration spelling variation', () => {
	const fixture = fs.mkdtempSync(path.join(os.tmpdir(), 'payment-provider-retirement-'));
	fs.mkdirSync(path.join(fixture, 'infra'), { recursive: true });
	fs.writeFileSync(path.join(fixture, 'infra', 'config.env'), 'PAYMENT_4_API_KEY=unsafe-fixture\n');
	assert.deepEqual(payment4Occurrences(fixture), ['infra/config.env']);
});

test('retired runtime files are absent and remaining provider files exist', () => {
	assert.equal(fs.existsSync(new URL('../apps/payment-service/providers/payment4.go', import.meta.url)), false);
	assert.equal(fs.existsSync(new URL('../apps/payment-service/providers/nowpayments.go', import.meta.url)), true);
	assert.equal(fs.existsSync(new URL('../apps/payment-service/providers/jibit.go', import.meta.url)), true);
});
