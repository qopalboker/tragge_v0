import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  identifyTarget,
  loadPlan,
  main,
  orderedSqlFiles,
  psqlArguments,
  psqlFailureMessage,
  selectEnvironment,
  validateGuards,
} from "./database-reset.mjs";
import {
  repositoryRoot,
  validateMarkdownFiles,
} from "./production-baseline.mjs";

const migrationDirectory = path.join(repositoryRoot, "packages", "db", "migrations");
const targetMigrationDirectory = path.join(migrationDirectory, "target");
const inventoryPath = "docs/architecture/migration-inventory.md";
const strategyPath = "docs/architecture/database-migration-reset-strategy.md";
const roadmapPath = "docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md";
const adrPath = "docs/adr/0001-target-runtime-architecture.md";
const rolePath = "packages/db/init/target/01-cluster-roles.sql";
const seedPath = "packages/db/init/target/02_reference_data.seed.sql";
const targetUpPath = "packages/db/migrations/target/0001_schema_ownership.up.sql";
const squashPath = "scripts/squash-migrations.sh";

function read(relativePath) {
  return fs.readFileSync(path.join(repositoryRoot, relativePath), "utf8");
}

function topLevelMigrationNames(suffix) {
  return fs
    .readdirSync(migrationDirectory, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(suffix))
    .map((entry) => entry.name)
    .sort((left, right) => left.localeCompare(right, "en"));
}

function manifestRows() {
  return read(inventoryPath)
    .split(/\r?\n/)
    .filter((line) => /^\| \d+ \| `\d{4}_[a-z0-9_]+\.up\.sql` \|/.test(line))
    .map((line) => {
      const cells = line
        .split("|")
        .slice(1, -1)
        .map((cell) => cell.trim());
      assert.equal(cells.length, 8, `manifest row has eight columns: ${line}`);
      return {
        order: Number(cells[0]),
        filename: cells[1].slice(1, -1),
        down: cells[2],
        operation: cells[3],
        dependency: cells[4],
        conflict: cells[5],
        classification: cells[6],
        rationale: cells[7],
      };
    });
}

function classificationTotals(rows) {
  const totals = {
    KEEP: 0,
    FOLD_INTO_BASELINE: 0,
    REPLACE: 0,
    DELETE_AFTER_CUTOVER: 0,
  };
  for (const row of rows) totals[row.classification] += 1;
  return totals;
}

test("legacy migration filenames, identifiers, order, and pairs are deterministic", () => {
  const up = topLevelMigrationNames(".up.sql");
  const down = topLevelMigrationNames(".down.sql");
  assert.equal(up.length, 100);
  assert.equal(down.length, 101);
  assert.match(up[0], /^0001_/);
  assert.match(up.at(-1), /^0100_/);
  const identifiers = up.map((name) => Number(name.slice(0, 4)));
  assert.equal(new Set(identifiers).size, identifiers.length);
  assert.deepEqual(identifiers, Array.from({ length: 100 }, (_, index) => index + 1));
  const upBases = new Set(up.map((name) => name.replace(".up.sql", "")));
  const pairedDown = down.filter((name) => name !== "0000_baseline.down.sql");
  assert.deepEqual(
    pairedDown.map((name) => name.replace(".down.sql", "")),
    up.map((name) => name.replace(".up.sql", "")),
  );
  assert.deepEqual(
    down.filter((name) => !upBases.has(name.replace(".down.sql", ""))),
    ["0000_baseline.down.sql"],
  );
  for (const name of [...up, ...down]) {
    assert.match(name, /^\d{4}_[a-z0-9_]+\.(?:up|down)\.sql$/);
  }
});

test("every legacy up migration is classified exactly once with no unknown row", () => {
  const up = topLevelMigrationNames(".up.sql");
  const rows = manifestRows();
  assert.equal(rows.length, 100);
  assert.deepEqual(rows.map((row) => row.order), Array.from({ length: 100 }, (_, index) => index + 1));
  assert.deepEqual(rows.map((row) => row.filename), up);
  assert.equal(new Set(rows.map((row) => row.filename)).size, 100);
  for (const row of rows) {
    assert.equal(row.down, "yes");
    assert.ok(row.operation.length > 10);
    assert.ok(row.dependency.length > 0);
    assert.ok(row.conflict.length > 10);
    assert.ok(row.rationale.length > 10);
    assert.ok(
      ["KEEP", "FOLD_INTO_BASELINE", "REPLACE", "DELETE_AFTER_CUTOVER"].includes(
        row.classification,
      ),
    );
  }
  assert.deepEqual(classificationTotals(rows), {
    KEEP: 0,
    FOLD_INTO_BASELINE: 25,
    REPLACE: 57,
    DELETE_AFTER_CUTOVER: 18,
  });
});

test("duplicate target migration identifiers are rejected", () => {
  const temporaryDirectory = fs.mkdtempSync(path.join(os.tmpdir(), "tragge-fnd004-"));
  try {
    fs.writeFileSync(path.join(temporaryDirectory, "0001_platform_a.up.sql"), "SELECT 1;\n");
    fs.writeFileSync(path.join(temporaryDirectory, "0001_engine_b.up.sql"), "SELECT 1;\n");
    assert.throws(
      () => orderedSqlFiles(temporaryDirectory, /^\d{4}_[a-z0-9_]+\.up\.sql$/),
      /duplicate SQL migration identifiers/,
    );
  } finally {
    fs.rmSync(temporaryDirectory, { force: true, recursive: true });
  }
});

test("target foundation is paired, ordered, owner-isolated, and domain-table free", () => {
  const plan = loadPlan();
  assert.equal(plan.migrations.length, 1);
  assert.equal(path.basename(plan.migrations[0]), "0001_schema_ownership.up.sql");
  assert.equal(plan.seeds.length, 1);
  const targetUp = read(targetUpPath);
  const targetDown = path.join(targetMigrationDirectory, "0001_schema_ownership.down.sql");
  assert.ok(fs.existsSync(targetDown));
  assert.doesNotMatch(targetUp, /\bCREATE\s+TABLE\b/i);
  assert.doesNotMatch(targetUp, /\b(?:commission_rate|max_participants|participant_capacity)\b/i);
  assert.doesNotMatch(targetUp, /second[ _-]?chance/i);
  for (const [schema, owner, runtime] of [
    ["platform", "platform_owner", "platform"],
    ["engine", "engine_owner", "engine"],
    ["market_data", "market_data_owner", "market_data"],
  ]) {
    assert.match(targetUp, new RegExp(`CREATE SCHEMA IF NOT EXISTS ${schema} AUTHORIZATION ${owner};`));
    assert.match(targetUp, new RegExp(`GRANT USAGE ON SCHEMA ${schema} TO ${runtime};`));
  }
  assert.doesNotMatch(targetUp, /GRANT\s+USAGE\s+ON\s+SCHEMA\s+engine\s+TO\s+platform/i);
  assert.doesNotMatch(targetUp, /GRANT\s+USAGE\s+ON\s+SCHEMA\s+market_data\s+TO\s+platform/i);
  assert.doesNotMatch(targetUp, /GRANT\s+USAGE\s+ON\s+SCHEMA\s+platform\s+TO\s+engine/i);
  assert.match(read(rolePath), /NOLOGIN/);
  assert.doesNotMatch(read(rolePath), /\bPASSWORD\b/i);
  assert.match(read(seedPath), /intentionally seeds no domain rows/i);
  const squash = read(squashPath);
  assert.match(squash, /REFUSED: the legacy schema-squash workflow is retired/);
  assert.match(squash, /exit 1/);
  assert.ok(squash.indexOf("exit 1") < squash.indexOf("pg_dump"));
});

test("psql options precede the URL and failures redact credentials", () => {
  const url = new URL("postgresql://admin:secret@localhost/tragge_test_phase0");
  const args = psqlArguments(url, ["-v", "ON_ERROR_STOP=1", "-c", "SELECT 1"]);
  assert.deepEqual(
    args.slice(0, -1),
    ["-X", "-v", "ON_ERROR_STOP=1", "-c", "SELECT 1"],
  );
  assert.equal(args.at(-1), url.toString());
  const message = psqlFailureMessage({ status: 2, message: `failed: ${url}` });
  assert.equal(message, "psql failed with exit code 2");
  assert.doesNotMatch(message, /secret|postgresql:/);
});

test("reset runner refuses production, ambiguous targets, and missing confirmation", () => {
  assert.throws(
    () => identifyTarget("postgresql://admin@localhost/app"),
    /approved development\/test naming pattern/,
  );
  const localTarget = identifyTarget("postgresql://admin@localhost/tragge_fnd004_test");
  assert.throws(
    () => validateGuards({
      allowHost: "",
      confirmDatabase: localTarget.database,
      confirmation: "I_UNDERSTAND_THIS_DESTROYS_DATA",
      environment: "production",
      execute: true,
      target: localTarget,
    }),
    /prohibited or not explicitly approved/,
  );
  assert.throws(
    () => validateGuards({
      allowHost: "",
      confirmDatabase: "",
      confirmation: "",
      environment: "test",
      execute: true,
      target: localTarget,
    }),
    /confirm-database/,
  );
  const remoteTarget = identifyTarget("postgresql://admin@db.staging.internal/tragge_staging_reset");
  assert.throws(
    () => validateGuards({
      allowHost: "",
      confirmDatabase: remoteTarget.database,
      confirmation: "I_UNDERSTAND_THIS_DESTROYS_DATA",
      environment: "staging",
      execute: true,
      target: remoteTarget,
    }),
    /requires --allow-host/,
  );
  assert.doesNotThrow(() => validateGuards({
    allowHost: "db.staging.internal",
    confirmDatabase: remoteTarget.database,
    confirmation: "I_UNDERSTAND_THIS_DESTROYS_DATA",
    environment: "staging",
    execute: true,
    target: remoteTarget,
  }));
  assert.throws(
    () => selectEnvironment(
      { environment: "test" },
      { APP_ENV: "production" },
    ),
    /APP_ENV "production" is prohibited/,
  );
  assert.throws(
    () => selectEnvironment(
      { environment: "test" },
      { TRAGGE_ENV: "development" },
    ),
    /conflicting database reset environment signals are prohibited/,
  );

});

test("documented dry-run command validates the plan without invoking PostgreSQL", () => {
  const outputLines = [];
  const originalLog = console.log;
  try {
    console.log = (...values) => outputLines.push(values.join(" "));
    main(
      ["--dry-run", "--environment", "test"],
      {
        TRAGGE_TARGET_DATABASE_URL:
          "postgresql://local_admin@localhost/tragge_fnd004_test?sslmode=disable",
      },
    );
  } finally {
    console.log = originalLog;
  }
  const output = outputLines.join("\n");
  assert.match(output, /mode=DRY_RUN/);
  assert.match(output, /target=local_admin@localhost:5432\/tragge_fnd004_test/);
  assert.match(output, /DRY_RUN_OK: no database command was executed/);
  assert.doesNotMatch(output, /password/i);
});

test("declared target financial and domain rules match fixed policy", () => {
  const strategy = read(strategyPath);
  assert.match(strategy, /one canonical `platform_fee_bps = 2000`/i);
  assert.match(strategy, /There is no\s+`commission_rate` Contest source/i);
  assert.match(strategy, /There is one Platform Fee source and one Prize Pool source/i);
  assert.match(strategy, /There is no[\s\S]{0,120}product Participant\s+Capacity field/i);
  assert.match(strategy, /Second Chance model/i);
  assert.match(strategy, /money uses signed `BIGINT` integer minor units/i);
  assert.match(strategy, /prices, rates, P&L, and T-Score use signed fixed-point integer units/i);
  assert.match(strategy, /`REAL`, `FLOAT`, and\s+`DOUBLE PRECISION` are prohibited/i);
  const legacySql = topLevelMigrationNames(".up.sql")
    .map((name) => fs.readFileSync(path.join(migrationDirectory, name), "utf8"))
    .join("\n");
  assert.doesNotMatch(legacySql, /second[ _-]?chance/i);
});

test("architecture ownership and all referenced roadmap IDs agree with authorities", () => {
  const strategy = read(strategyPath);
  const adr = read(adrPath);
  for (const term of [
    "Platform Modular Monolith",
    "Trading Engine",
    "Market Data Service",
    "`platform`",
    "`engine`",
    "`market_data`",
  ]) {
    assert.ok(strategy.toLowerCase().includes(term.toLowerCase()), `strategy missing ${term}`);
    assert.ok(adr.toLowerCase().includes(term.toLowerCase()), `ADR missing ${term}`);
  }
  const roadmap = read(roadmapPath);
  const source = [strategy, read(inventoryPath), read(rolePath), read(seedPath)].join("\n");
  const identifiers = new Set(
    source.match(/\b(?:FND|ARCH|DATA|CON|PRIZE|ENG|MD|SCH|SEC|OPS)-\d{3}\b/g) ?? [],
  );
  assert.ok(identifiers.size > 0);
  for (const identifier of identifiers) {
    assert.match(roadmap, new RegExp(`^### ${identifier}\\b`, "m"), `unknown roadmap task ${identifier}`);
  }
});

test("FND-004 Markdown links and style are valid", () => {
  const markdownFiles = [inventoryPath, strategyPath, "packages/db/README.md"];
  const links = validateMarkdownFiles(markdownFiles);
  assert.deepEqual(links.missing, []);
  assert.ok(links.checkedLinks >= 20);
  for (const markdownFile of markdownFiles) {
    const contents = read(markdownFile);
    assert.doesNotMatch(contents, /\t/);
    assert.doesNotMatch(contents, /[ \t]+$/m);
  }
});
