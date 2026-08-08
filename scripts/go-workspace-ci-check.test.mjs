import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import {
  parseWorkspaceJSON,
  readStructuredWorkspace,
  readStructuredWorkspaceDocument,
  validateCurrentRepository,
  validateWorkflowSource,
  validateWorkspacePaths,
} from './go-workspace-ci-check.mjs';

test('parses official go work JSON without leaking inline comments', () => {
  const modules = parseWorkspaceJSON(JSON.stringify({
    Use: [
      { DiskPath: './packages/config' },
      { DiskPath: './packages/domain' },
      { DiskPath: './packages/notification' },
      { DiskPath: './packages/resilience' },
      { DiskPath: './packages/wallet' },
    ],
  }));
  assert.deepEqual(modules, [
    './packages/config',
    './packages/domain',
    './packages/notification',
    './packages/resilience',
    './packages/wallet',
  ]);
  assert.ok(!modules.includes('./packages/config//+health'));
});

test('rejects duplicate, comment-tainted, and missing module paths', () => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'go-workspace-ci-'));
  try {
    for (const name of ['config', 'domain', 'notification', 'resilience', 'wallet']) {
      fs.mkdirSync(path.join(directory, 'packages', name), { recursive: true });
    }
    const failures = validateWorkspacePaths([
      './packages/config',
      './packages/config',
      './packages/domain',
      './packages/notification',
      './packages/resilience',
      './packages/wallet',
      './packages/config//+health',
      './packages/missing',
    ], directory);
    assert.ok(failures.some((failure) => failure.includes('duplicate workspace module')));
    assert.ok(failures.some((failure) => failure.includes('comment leaked')));
    assert.ok(failures.some((failure) => failure.includes('directory does not exist')));
    assert.ok(failures.some((failure) => failure.includes('legacy whitespace parser output')));
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
  }
});

test('workflow uses structured discovery and the pinned linter', () => {
  const valid = `
    mapfile -t modules < <(go work edit -json | jq -r '.Use[].DiskPath')
    if [ "\${#modules[@]}" -eq 0 ]; then exit 1; fi
    declare -A seen_modules=()
    for dir in "\${modules[@]}"; do
      if [ ! -d "$dir" ]; then exit 1; fi
      golangci-lint run --new-from-rev="$LINT_BASE_REF" ./...
    done
    https://raw.githubusercontent.com/golangci/golangci-lint/v2.12.2/install.sh
    sh -s -- -b "$(go env GOPATH)/bin" v2.12.2
    golangci-lint version
    mapfile -t modules < <(go work edit -json | jq -r '.Use[].DiskPath')
    ENVIRONMENT: test
    TEST_BASE_REF: base-sha
    git diff --name-only --diff-filter=ACMR "$TEST_BASE_REF...HEAD" -- '*.go'
    selected_modules=()
    for dir in "\${modules[@]}"; do
      echo "Testing changed module $dir..."
      go test -short -race -count=1 ./...
    done
    mapfile -t modules < <(go work edit -json | jq -r '.Use[].DiskPath')
    for dir in "\${modules[@]}"; do
      echo "Building $dir..."
      go build ./...
    done
  `;
  assert.deepEqual(validateWorkflowSource(valid), []);
  assert.notDeepEqual(validateWorkflowSource("grep '^\\s*\\./' go.work | tr -d '\\t '"), []);
  assert.notDeepEqual(validateWorkflowSource('go test -race -count=1 ./packages/... ./apps/...'), []);
});

test('actual workspace structured output matches every declared Use entry', () => {
  const modules = readStructuredWorkspace();
  const workspace = readStructuredWorkspaceDocument();
  assert.equal(modules.length, workspace.Use.length);
});

test('current repository satisfies Go workspace CI invariants', () => {
  const { modules, failures } = validateCurrentRepository();
  assert.equal(new Set(modules).size, modules.length);
  assert.deepEqual(failures, []);
});
