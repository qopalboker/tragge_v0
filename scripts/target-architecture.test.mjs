import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

import {
  repositoryRoot,
  validateMarkdownFiles,
} from "./production-baseline.mjs";

const adrPath = "docs/adr/0001-target-runtime-architecture.md";
const reviewPath =
  "docs/architecture/target-architecture-import-review.md";

function read(relativePath) {
  return fs.readFileSync(path.join(repositoryRoot, relativePath), "utf8");
}

function walkGoFiles(relativeDirectory) {
  const root = path.join(repositoryRoot, relativeDirectory);
  const files = [];

  function walk(directory) {
    for (const entry of fs
      .readdirSync(directory, { withFileTypes: true })
      .sort((left, right) => left.name.localeCompare(right.name))) {
      const absolutePath = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        walk(absolutePath);
      } else if (entry.isFile() && entry.name.endsWith(".go")) {
        files.push(absolutePath);
      }
    }
  }

  walk(root);
  return files;
}

function moduleForFile(absolutePath) {
  const relative = path
    .relative(repositoryRoot, absolutePath)
    .split(path.sep);
  return `${relative[0]}/${relative[1]}`;
}

function currentImportGraph() {
  const expression =
    /github\.com\/Parsaeffatravesh\/tragge\/(?<target>(?:apps|packages)\/[^"\s]+)/g;
  const rows = [];

  for (const file of [...walkGoFiles("apps"), ...walkGoFiles("packages")]) {
    const source = moduleForFile(file);
    const contents = fs.readFileSync(file, "utf8");
    for (const match of contents.matchAll(expression)) {
      const [kind, name] = match.groups.target.split("/");
      rows.push({ source, target: `${kind}/${name}` });
    }
  }

  return rows;
}

test("ADR is Accepted and fixes exactly three policy bounded systems", () => {
  const adr = read(adrPath);
  assert.match(adr, /\*\*Status:\*\* Accepted/);

  const boundarySection = adr.match(
    /The target production backend has exactly these three bounded systems:\r?\n\r?\n([\s\S]+?)\r?\n\r?\nFrontends,/,
  );
  assert.ok(boundarySection, "bounded-system decision table is missing");
  const boundaryRows = boundarySection[1]
    .split(/\r?\n/)
    .filter((line) =>
      /^\| (?:Platform modular monolith|Trading Engine|Market Data Service) \|/.test(
        line,
      ),
    );
  assert.equal(boundaryRows.length, 3);
  assert.equal(new Set(boundaryRows).size, 3);

  for (const mode of ["api", "realtime", "worker"]) {
    assert.match(adr, new RegExp(`platform --mode=${mode}`));
  }
});

test("ADR records ownership, communication, migration, and rollback", () => {
  const adr = read(adrPath);
  for (const requiredText of [
    "platform schema / role",
    "engine schema / role",
    "market_data schema / role",
    "outbox",
    "inbox",
    "Cross-schema",
    "event ID",
    "correlation ID",
    "causation ID",
    "schema version",
    "aggregate version",
    "occurred-at timestamp",
    "## Migration principles",
    "## Rollback principles",
    "## Rejected alternatives",
    "apps/api-server",
    "apps/trading-core",
    "apps/worker",
  ]) {
    assert.ok(
      adr.includes(requiredText),
      `missing ADR requirement: ${requiredText}`,
    );
  }
  assert.match(adr, /```mermaid[\s\S]+flowchart LR[\s\S]+```/);
});

test("FND-002 Markdown has clean style and resolving repository links", () => {
  for (const file of [adrPath, reviewPath]) {
    const markdown = read(file);
    assert.doesNotMatch(markdown, /\t/);
    for (const [index, line] of markdown.split(/\r?\n/).entries()) {
      assert.doesNotMatch(
        line,
        /[ \t]+$/,
        `${file}:${index + 1} has trailing whitespace`,
      );
    }
  }

  const result = validateMarkdownFiles([adrPath, reviewPath], repositoryRoot);
  assert.deepEqual(result.missing, []);
  assert.ok(result.checkedLinks >= 10);
});

test("current imports match reviewed transitional boundary evidence", () => {
  const rows = currentImportGraph();
  const edges = new Set(rows.map(({ source, target }) => `${source}->${target}`));
  const crossAppEdges = [...edges]
    .filter((edge) => {
      const [source, target] = edge.split("->");
      return (
        source.startsWith("apps/") &&
        target.startsWith("apps/") &&
        source !== target
      );
    })
    .sort();

  assert.deepEqual(crossAppEdges, [
    "apps/api-server->apps/admin-bff",
    "apps/api-server->apps/payment-service",
    "apps/api-server->apps/user-bff",
    "apps/trading-core->apps/market-ingestor",
    "apps/trading-core->apps/trade-bff",
    "apps/trading-core->apps/trading-engine",
    "apps/worker->apps/contest-scheduler",
    "apps/worker->apps/free-contest-generator",
    "apps/worker->apps/leaderboard-worker",
    "apps/worker->apps/settlement-service",
  ]);

  const packageToAppEdges = [...edges].filter(
    (edge) =>
      edge.startsWith("packages/") && edge.split("->")[1].startsWith("apps/"),
  );
  assert.deepEqual(packageToAppEdges, []);
  // SEC-005 centralizes redaction, SEC-006 adds approved in-boundary edge
  // security imports, and SEC-007 adds Admin MFA tests/implementation without
  // changing the module dependency graph.
  assert.equal(rows.length, 506);
  assert.equal(edges.size, 176);
});
