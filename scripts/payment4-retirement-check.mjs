#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const retiredPattern = /payment[ _-]?4|payment4/i;
const ignoredDirectories = new Set(['.git', '.tmp', 'coverage', 'dist', 'node_modules']);

export const permittedRetirementReferences = new Map([
	['apps/payment-service/handlers/payment_provider_retirement_test.go', 'Executable rejection fixture; the active handler never accepts the value.'],
	['apps/payment-service/server/payment_provider_retirement_test.go', 'Executable generic-404 fixture; the active router never registers the path.'],
	['docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md', 'Current SEC-006 task amendment records the approved retirement gate.'],
	['docs/codex/reports/SEC-006-git-execution-report.md', 'Historical failed-gate evidence and current remediation record must preserve the former provider name.'],
	['docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md', 'Current fixed policy explicitly marks the provider retired.'],
	['docs/product/canonical-domain-glossary-and-version-catalog.md', 'Version catalog identifies the current retirement decision.'],
	['docs/product/payment4-retirement-policy-amendment.md', 'Authoritative product decision and occurrence inventory.'],
	['package.json', 'Repository commands expose the focused retirement validator and tests.'],
	['scripts/domain-glossary.test.mjs', 'Version-catalog regression asserts the exact retirement-decision identifier.'],
	['scripts/payment4-retirement-check.mjs', 'Validator implementation must identify the prohibited provider.'],
	['scripts/payment4-retirement-check.test.mjs', 'Validator fixtures prove active references and missing files are rejected.'],
	['scripts/sec-006-edge-security-check.mjs', 'SEC-006 composes the focused retirement validator into its gate.'],
]);

function walk(directory, output = []) {
	for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
		if (entry.isDirectory() && ignoredDirectories.has(entry.name)) continue;
		const absolute = path.join(directory, entry.name);
		if (entry.isDirectory()) walk(absolute, output);
		else output.push(absolute);
	}
	return output;
}

function relative(absolute, repositoryRoot = root) {
	return path.relative(repositoryRoot, absolute).split(path.sep).join('/');
}

function textFile(absolute) {
	const buffer = fs.readFileSync(absolute);
	if (buffer.includes(0)) return null;
	return buffer.toString('utf8');
}

export function payment4Occurrences(repositoryRoot = root) {
	const occurrences = [];
	for (const absolute of walk(repositoryRoot)) {
		const source = textFile(absolute);
		if (source === null || !retiredPattern.test(source)) continue;
		occurrences.push(relative(absolute, repositoryRoot));
	}
	return occurrences.sort();
}

function requireText(relativePath, patterns, failures) {
	const absolute = path.join(root, relativePath);
	if (!fs.existsSync(absolute)) {
		failures.push(relativePath + ': required file is missing');
		return '';
	}
	const source = fs.readFileSync(absolute, 'utf8');
	for (const pattern of patterns) {
		if (!pattern.test(source)) failures.push(relativePath + ': missing ' + pattern);
	}
	return source;
}

export function validatePayment4Retirement() {
	const failures = [];
	const occurrences = payment4Occurrences();

	for (const file of occurrences) {
		if (!permittedRetirementReferences.has(file)) {
			failures.push(file + ': unapproved active or undocumented historical reference');
		}
	}
	for (const [file, rationale] of permittedRetirementReferences) {
		if (!rationale.trim()) failures.push(file + ': allowlist rationale is empty');
		if (!occurrences.includes(file)) failures.push(file + ': expected retirement evidence is missing');
	}

	for (const removed of [
		'apps/payment-service/providers/payment4.go',
		'apps/payment-service/providers/payment4_test.go',
		'apps/payment-service/providers/payment4_webhook_test.go',
		'apps/payment-service/providers/payment4_integration_test.go',
		'apps/payment-service/handlers/payment4_e2e_test.go',
	]) {
		if (fs.existsSync(path.join(root, removed))) failures.push(removed + ': retired implementation/test still exists');
	}

	const providers = requireText('apps/payment-service/providers/provider.go', [
		/ProviderNowPayments\s+ProviderType\s*=\s*"nowpayments"/,
		/ProviderJibit\s+ProviderType\s*=\s*"jibit"/,
	], failures);
	if (retiredPattern.test(providers)) failures.push('provider registry still accepts the retired provider');

	const deposit = requireText('apps/payment-service/handlers/deposit.go', [
		/if req\.Provider != "nowpayments"/,
		/invalid crypto payment provider/,
	], failures);
	if (retiredPattern.test(deposit)) failures.push('deposit handler retains retired-provider behavior');

	const app = requireText('apps/payment-service/server/app.go', [
		/registerPaymentWebhookRoutes\(r, webhookHandler\.HandleNowPaymentsWebhook\)/,
		/r\.Post\("\/nowpayments", nowPayments\)/,
	], failures);
	if (retiredPattern.test(app)) failures.push('payment-service startup or routes retain retired-provider behavior');

	for (const configPath of [
		'.env.example',
		'apps/gateway/nginx.conf',
		'apps/gateway/nginx.prod.conf',
		'infra/docker/docker-compose.yml',
		'infra/k8s/base/configmap.yaml',
		'infra/k8s/base/external-secrets.yaml',
		'infra/k8s/base/network-policies.yaml',
		'infra/k8s/base/secrets.yaml',
		'scripts/secrets/init-secrets.sh',
	]) {
		const source = requireText(configPath, [], failures);
		if (retiredPattern.test(source)) failures.push(configPath + ': active configuration retains retired-provider support');
	}

	for (const frontendRoot of ['apps/user-frontend/src', 'apps/admin-frontend/src']) {
		for (const absolute of walk(path.join(root, frontendRoot))) {
			const source = textFile(absolute);
			if (source !== null && retiredPattern.test(source)) {
				failures.push(relative(absolute) + ': frontend retains retired-provider option');
			}
		}
	}

	for (const databaseRoot of ['packages/db/migrations', 'packages/db/init']) {
		for (const absolute of walk(path.join(root, databaseRoot))) {
			const source = textFile(absolute);
			if (source !== null && retiredPattern.test(source)) {
				failures.push(relative(absolute) + ': clean database artifacts retain active provider data');
			}
		}
	}

	for (const absolute of walk(root)) {
		if (!['go.mod', 'go.sum', 'pnpm-lock.yaml'].includes(path.basename(absolute))) continue;
		const source = textFile(absolute);
		if (source !== null && retiredPattern.test(source)) {
			failures.push(relative(absolute) + ': module metadata retains a retired-provider dependency');
		}
	}

	return { failures, occurrences };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	const result = validatePayment4Retirement();
	if (result.failures.length) {
		console.error('Payment4 retirement validation failed (' + result.failures.length + ' finding(s)):');
		for (const failure of result.failures) console.error('- ' + failure);
		process.exit(1);
	}
	console.log('Payment4 retirement validation passed; ' + result.occurrences.length + ' allowlisted evidence file(s), zero active references.');
}
