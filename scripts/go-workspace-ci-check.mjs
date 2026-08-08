#!/usr/bin/env node

import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

export function parseWorkspaceJSON(text) {
  const workspace = JSON.parse(text);
  if (!Array.isArray(workspace.Use)) {
    throw new Error('go work edit -json did not return a Use array');
  }
  return workspace.Use.map((entry) => entry?.DiskPath);
}

export function validateWorkspacePaths(modules, repositoryRoot = root) {
  const failures = [];
  if (modules.length === 0) failures.push('go.work contains no workspace modules');

  const seen = new Set();
  for (const modulePath of modules) {
    if (typeof modulePath !== 'string' || modulePath.trim() === '') {
      failures.push('go.work contains a workspace entry without DiskPath');
      continue;
    }
    if (seen.has(modulePath)) failures.push(`duplicate workspace module: ${modulePath}`);
    seen.add(modulePath);
    if (modulePath.includes('//') || modulePath.includes('+ health')) {
      failures.push(`workspace comment leaked into module path: ${modulePath}`);
    }
    if (!fs.existsSync(path.resolve(repositoryRoot, modulePath))) {
      failures.push(`workspace module directory does not exist: ${modulePath}`);
    }
  }

  for (const expected of [
    './packages/config',
    './packages/domain',
    './packages/notification',
    './packages/resilience',
    './packages/wallet',
  ]) {
    if (!seen.has(expected)) failures.push(`commented workspace entry was not parsed exactly: ${expected}`);
  }
  if (seen.has('./packages/config//+health')) {
    failures.push('legacy whitespace parser output remains: ./packages/config//+health');
  }

  return failures;
}

export function readStructuredWorkspaceDocument(repositoryRoot = root) {
  const result = spawnSync('go', ['work', 'edit', '-json'], {
    cwd: repositoryRoot,
    encoding: 'utf8',
    env: { ...process.env, GOTELEMETRY: 'off' },
  });
  if (result.status !== 0) {
    throw new Error(`go work edit -json failed: ${(result.stderr || '').trim()}`);
  }
  return JSON.parse(result.stdout);
}

export function readStructuredWorkspace(repositoryRoot = root) {
  return parseWorkspaceJSON(JSON.stringify(readStructuredWorkspaceDocument(repositoryRoot)));
}

export function validateWorkflowSource(source) {
  const failures = [];
  const required = [
    /go work edit -json \| jq -r '\.Use\[\]\.(?:DiskPath)'/,
    /mapfile -t modules/,
    /"\$\{#modules\[@\]\}" -eq 0/,
    /declare -A seen_modules/,
    /\[ ! -d "\$dir" \]/,
    /for dir in "\$\{modules\[@\]\}"/,
    /--new-from-rev="\$LINT_BASE_REF"/,
    /golangci-lint\/v2\.12\.2\/install\.sh/,
    /sh -s -- -b "\$\(go env GOPATH\)\/bin" v2\.12\.2/,
    /golangci-lint version/,
    /TEST_BASE_REF:/,
    /git diff --name-only --diff-filter=ACMR "\$TEST_BASE_REF\.\.\.HEAD"/,
    /selected_modules=\(\)/,
    /echo "Testing changed module \$dir\.\.\."/,
    /go test -short -race -count=1 \.\/\.\.\./,
    /ENVIRONMENT: test/,
    /echo "Building \$dir\.\.\."/,
    /go build \.\/\.\.\./,
  ];
  for (const pattern of required) {
    if (!pattern.test(source)) failures.push(`CI workflow is missing ${pattern}`);
  }
  if (/grep ['"]\^\\s\*\\\.\//.test(source) || /tr -d ['"]\\t ['"]/.test(source)) {
    failures.push('CI workflow retains text-based go.work module discovery');
  }
  const discoveryCount = source.match(/mapfile -t modules < <\(go work edit -json \| jq -r '\.Use\[\]\.DiskPath'\)/g)?.length ?? 0;
  if (discoveryCount < 3) {
    failures.push('CI workflow must use structured workspace discovery for lint, test, and build');
  }
  if (/go test[^\n]*\.\/packages\/\.\.\. [^\n]*\.\/apps\/\.\.\./.test(source)) {
    failures.push('CI workflow retains invalid workspace-root Go test globs');
  }
  if (/go test -race -count=1/.test(source)) {
    failures.push('CI workflow runs inherited integration suites in the unit/race job');
  }
  if (/packages\/notification/.test(source)) {
    failures.push('CI workflow hard-codes an unrelated module exclusion or inclusion');
  }
  if (/go build \.\/apps\//.test(source)) {
    failures.push('CI workflow retains hard-coded workspace-root Go build globs');
  }
  return failures;
}

export function validateCurrentRepository(repositoryRoot = root) {
  const modules = readStructuredWorkspace(repositoryRoot);
  const failures = validateWorkspacePaths(modules, repositoryRoot);
  const workflow = fs.readFileSync(path.join(repositoryRoot, '.github/workflows/ci.yml'), 'utf8');
  failures.push(...validateWorkflowSource(workflow));
  return { modules, failures };
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const { modules, failures } = validateCurrentRepository();
    if (failures.length > 0) {
      console.error(`Go workspace CI validation failed (${failures.length} finding(s)):`);
      for (const failure of failures) console.error(`- ${failure}`);
      process.exitCode = 1;
    } else {
      console.log(`Go workspace CI validation passed; ${modules.length} structured module(s), all unique and present.`);
    }
  } catch (error) {
    console.error(`Go workspace CI validation failed: ${error.message}`);
    process.exitCode = 1;
  }
}
