import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  buildInventory,
  documentedInventoryDeltas,
  expectedCurrentCounts,
  expectedFnd001Counts,
  extractMarkdownLinks,
  repositoryRoot,
  validateFindingRows,
  validateMarkdownFiles,
  verifyToolchains,
} from "./production-baseline.mjs";

test("inventory preserves FND-001 and applies only documented task deltas", () => {
  assert.deepEqual(documentedInventoryDeltas, {
    "SEC-001": { goFiles: 7, goTestFiles: 5 },
    "SEC-002": { goFiles: 2, goTestFiles: 1, typeScriptFiles: 2 },
    "SEC-003": { goFiles: 12, goTestFiles: 10, typeScriptFiles: 1 },
    "SEC-004": {
      goFiles: 5, goTestFiles: 3, typeScriptFiles: 2, sqlFiles: 2, upMigrations: 1,
    },
    "SEC-005": { goFiles: 8, goTestFiles: 4, typeScriptFiles: 1 },
    "SEC-006": { goFiles: 2, goTestFiles: 1 },
    "SEC-007": {
      goFiles: 4, goTestFiles: 2, typeScriptFiles: 2, sqlFiles: 2, upMigrations: 1,
    },
    "P1-REM-001": { typeScriptFiles: 1 },
  });
  assert.equal(expectedFnd001Counts.goFiles, 375);
  assert.equal(expectedFnd001Counts.goTestFiles, 99);

  const inventory = buildInventory(repositoryRoot);
  for (const [metric, expected] of Object.entries(expectedCurrentCounts)) {
    assert.equal(inventory.counts[metric], expected, metric);
  }
});

test("Markdown link extraction handles local and external targets", () => {
  assert.deepEqual(
    extractMarkdownLinks(
      "[local](../README.md) [web](https://example.com) [angle](<a path/file.md>)",
    ),
    ["../README.md", "https://example.com", "a path/file.md"],
  );
});

test("Markdown validation detects missing local files", () => {
  const temporaryRoot = fs.mkdtempSync(
    path.join(os.tmpdir(), "tragge-baseline-"),
  );
  try {
    fs.writeFileSync(
      path.join(temporaryRoot, "audit.md"),
      "[present](present.md) [missing](missing.md) [web](https://example.com)\n",
    );
    fs.writeFileSync(path.join(temporaryRoot, "present.md"), "present\n");

    const result = validateMarkdownFiles(["audit.md"], temporaryRoot);
    assert.equal(result.checkedLinks, 2);
    assert.deepEqual(result.missing, ["audit.md: missing.md"]);
  } finally {
    fs.rmSync(temporaryRoot, { recursive: true, force: true });
  }
});

test("current audit contains 35 evidenced P0/P1 findings", () => {
  const result = validateFindingRows(
    "docs/architecture/current-state-audit.md",
    repositoryRoot,
  );
  assert.equal(result.findingRows, 35);
  assert.deepEqual(result.errors, []);
});

test("toolchain baseline is compatible with repository declarations", () => {
  const result = verifyToolchains(repositoryRoot);
  assert.deepEqual(result.errors, []);
  assert.equal(result.versions.golang, "1.24.7");
  assert.equal(result.versions.nodejs, "20.19.0");
  assert.equal(result.versions.pnpm, "8.15.0");
});
