#!/usr/bin/env node

import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const roleFile = join(repositoryRoot, "packages", "db", "init", "target", "01-cluster-roles.sql");
const migrationDirectory = join(repositoryRoot, "packages", "db", "migrations", "target");
const seedDirectory = join(repositoryRoot, "packages", "db", "init", "target");

const allowedEnvironments = new Set([
  "dev",
  "development",
  "local",
  "test",
  "staging",
  "preproduction",
  "preprod",
]);
const productionEnvironments = new Set(["prod", "production"]);
const localHosts = new Set(["localhost", "127.0.0.1", "[::1]", "::1"]);
const databaseNamePattern =
  /^(?:tragge|app)_(?:dev|development|local|test|staging|preprod|preproduction|fnd[0-9]+)(?:_[a-z0-9]+)*$/;
const destructiveConfirmation = "I_UNDERSTAND_THIS_DESTROYS_DATA";

export function parseArguments(argv) {
  const options = { execute: false, confirmDatabase: "", allowHost: "", environment: "" };
  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];
    if (argument === "--execute") {
      options.execute = true;
    } else if (argument === "--confirm-database") {
      options.confirmDatabase = argv[index + 1] ?? "";
      index += 1;
    } else if (argument === "--allow-host") {
      options.allowHost = argv[index + 1] ?? "";
      index += 1;
    } else if (argument === "--environment") {
      options.environment = argv[index + 1] ?? "";
      index += 1;
    } else if (argument === "--dry-run") {
      options.execute = false;
    } else {
      throw new Error(`Unknown or incomplete argument: ${argument}`);
    }
  }
  return options;
}

export function identifyTarget(rawUrl) {
  if (!rawUrl) throw new Error("TRAGGE_TARGET_DATABASE_URL is required");
  let parsed;
  try {
    parsed = new URL(rawUrl);
  } catch {
    throw new Error("TRAGGE_TARGET_DATABASE_URL is not a valid URL");
  }
  if (!["postgres:", "postgresql:"].includes(parsed.protocol)) {
    throw new Error("target URL must use postgres:// or postgresql://");
  }
  const database = decodeURIComponent(parsed.pathname.replace(/^\/+/, ""));
  if (!parsed.hostname || !parsed.username || !database) {
    throw new Error("target connection must explicitly identify hostname, username, and database");
  }
  if (!databaseNamePattern.test(database)) {
    throw new Error(`database "${database}" does not match the approved development/test naming pattern`);
  }
  return {
    database,
    host: parsed.hostname,
    port: parsed.port || "5432",
    username: decodeURIComponent(parsed.username),
    targetUrl: parsed,
  };
}

export function validateGuards({
  allowHost,
  confirmDatabase,
  confirmation,
  environment,
  execute,
  target,
}) {
  const normalizedEnvironment = environment.toLowerCase();
  if (!normalizedEnvironment) throw new Error("an explicit --environment value is required");
  if (
    productionEnvironments.has(normalizedEnvironment) ||
    !allowedEnvironments.has(normalizedEnvironment)
  ) {
    throw new Error(`environment "${environment}" is prohibited or not explicitly approved`);
  }
  const normalizedHost = target.host.toLowerCase();
  if (!localHosts.has(normalizedHost) && allowHost !== target.host) {
    throw new Error(`non-local host "${target.host}" requires --allow-host with the exact hostname`);
  }
  if (execute) {
    if (confirmDatabase !== target.database) {
      throw new Error("--confirm-database must exactly match the target database name");
    }
    if (confirmation !== destructiveConfirmation) {
      throw new Error(
        "TRAGGE_DATABASE_RESET_CONFIRM must contain the required destructive confirmation",
      );
    }
  }
}

export function orderedSqlFiles(directory, suffixPattern) {
  if (!existsSync(directory)) throw new Error(`required directory does not exist: ${directory}`);
  const files = readdirSync(directory)
    .filter((name) => suffixPattern.test(name))
    .sort((left, right) => left.localeCompare(right, "en"));
  const identifiers = files.map((name) => name.slice(0, 4));
  if (new Set(identifiers).size !== identifiers.length) {
    throw new Error(`duplicate SQL migration identifiers in ${directory}`);
  }
  return files.map((name) => join(directory, name));
}

export function loadPlan() {
  if (!existsSync(roleFile)) throw new Error(`required role file does not exist: ${roleFile}`);
  const migrations = orderedSqlFiles(
    migrationDirectory,
    /^\d{4}_[a-z0-9_]+\.up\.sql$/,
  );
  const seeds = orderedSqlFiles(seedDirectory, /^\d{2}_[a-z0-9_]+\.seed\.sql$/);
  if (migrations.length === 0) throw new Error("target baseline contains no up migration");
  for (const file of [roleFile, ...migrations, ...seeds]) {
    if (!readFileSync(file, "utf8").trim()) throw new Error(`SQL file is empty: ${file}`);
  }
  return { migrations, roleFile, seeds };
}

export function psqlArguments(connectionUrl, args) {
  return ["-X", ...args, connectionUrl.toString()];
}

export function psqlFailureMessage(error) {
  const detail = Number.isInteger(error.status)
    ? `exit code ${error.status}`
    : error.code ? `process error ${error.code}` : "unknown process error";
  return `psql failed with ${detail}`;
}

function runPsql(connectionUrl, args, capture = false) {
  try {
    return execFileSync("psql", psqlArguments(connectionUrl, args), {
      cwd: repositoryRoot,
      encoding: capture ? "utf8" : undefined,
      env: {
        ...process.env,
        PGCONNECT_TIMEOUT: "5",
        PGOPTIONS: "-c statement_timeout=30000 -c lock_timeout=5000",
      },
      stdio: capture ? ["ignore", "pipe", "inherit"] : "inherit",
    });
  } catch (error) {
    throw new Error(psqlFailureMessage(error));
  }
}

function quoteIdentifier(identifier) {
  if (!/^[a-z0-9_]+$/.test(identifier)) throw new Error(`unsafe database identifier: ${identifier}`);
  return `"${identifier}"`;
}

function executeReset(target, plan) {
  const maintenanceUrl = new URL(target.targetUrl);
  maintenanceUrl.pathname = "/postgres";
  runPsql(maintenanceUrl, ["-v", "ON_ERROR_STOP=1", "-f", plan.roleFile]);
  runPsql(maintenanceUrl, [
    "-v",
    "ON_ERROR_STOP=1",
    "-c",
    `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${target.database}' AND pid <> pg_backend_pid();`,
  ]);
  runPsql(maintenanceUrl, [
    "-v",
    "ON_ERROR_STOP=1",
    "-c",
    `DROP DATABASE IF EXISTS ${quoteIdentifier(target.database)};`,
  ]);
  runPsql(maintenanceUrl, [
    "-v",
    "ON_ERROR_STOP=1",
    "-c",
    `CREATE DATABASE ${quoteIdentifier(target.database)};`,
  ]);
  for (const migration of plan.migrations) {
    runPsql(target.targetUrl, ["-v", "ON_ERROR_STOP=1", "-f", migration]);
  }
  for (const seed of plan.seeds) {
    runPsql(target.targetUrl, ["-v", "ON_ERROR_STOP=1", "-f", seed]);
  }
  runPsql(
    target.targetUrl,
    [
      "-v",
      "ON_ERROR_STOP=1",
      "-c",
      `DO $validation$
DECLARE
    ownership text[];
    isolation text;
BEGIN
    SELECT array_agg(n.nspname || ':' || r.rolname ORDER BY n.nspname)
      INTO ownership
      FROM pg_namespace n
      JOIN pg_roles r ON r.oid = n.nspowner
      WHERE n.nspname IN ('platform','engine','market_data');
    IF ownership IS DISTINCT FROM ARRAY[
        'engine:engine_owner',
        'market_data:market_data_owner',
        'platform:platform_owner'
    ] THEN
        RAISE EXCEPTION 'schema ownership validation failed: %', ownership;
    END IF;
    SELECT has_schema_privilege('platform','platform','USAGE')::int || ':' || has_schema_privilege('platform','engine','USAGE')::int || ':' || has_schema_privilege('platform','market_data','USAGE')::int || '|' || has_schema_privilege('engine','platform','USAGE')::int || ':' || has_schema_privilege('engine','engine','USAGE')::int || ':' || has_schema_privilege('engine','market_data','USAGE')::int || '|' || has_schema_privilege('market_data','platform','USAGE')::int || ':' || has_schema_privilege('market_data','engine','USAGE')::int || ':' || has_schema_privilege('market_data','market_data','USAGE')::int
      INTO isolation;
    IF isolation IS DISTINCT FROM '1:0:0|0:1:0|0:0:1' THEN
        RAISE EXCEPTION 'runtime schema isolation validation failed: %', isolation;
    END IF;
END
$validation$;`,
    ],
  );
}

export function formatPlan(target, environment, plan, execute) {
  const relative = (file) => file.slice(repositoryRoot.length + 1).replaceAll("\\", "/");
  return [
    `mode=${execute ? "EXECUTE" : "DRY_RUN"}`,
    `environment=${environment.toLowerCase()}`,
    `target=${target.username}@${target.host}:${target.port}/${target.database}`,
    `role_file=${relative(plan.roleFile)}`,
    `migrations=${plan.migrations.map(relative).join(",")}`,
    `seeds=${plan.seeds.map(relative).join(",") || "none"}`,
  ].join("\n");
}

export function selectEnvironment(options, environment) {
  const signals = [
    ["--environment", options.environment],
    ["TRAGGE_ENV", environment.TRAGGE_ENV || ""],
    ["APP_ENV", environment.APP_ENV || ""],
  ].filter(([, value]) => value);
  for (const [name, value] of signals) {
    if (productionEnvironments.has(value.toLowerCase())) {
      throw new Error(`${name} "${value}" is prohibited for database reset`);
    }
  }
  const normalized = new Set(signals.map(([, value]) => value.toLowerCase()));
  if (normalized.size > 1) {
    throw new Error("conflicting database reset environment signals are prohibited");
  }
  return options.environment || environment.TRAGGE_ENV || environment.APP_ENV || "";
}

export function main(argv = process.argv.slice(2), environment = process.env) {
  const options = parseArguments(argv);
  const target = identifyTarget(environment.TRAGGE_TARGET_DATABASE_URL);
  const selectedEnvironment = selectEnvironment(options, environment);
  validateGuards({
    ...options,
    confirmation: environment.TRAGGE_DATABASE_RESET_CONFIRM || "",
    environment: selectedEnvironment,
    target,
  });
  const plan = loadPlan();
  console.log(formatPlan(target, selectedEnvironment, plan, options.execute));
  if (!options.execute) {
    console.log("DRY_RUN_OK: no database command was executed");
    return;
  }
  executeReset(target, plan);
  console.log("RESET_OK: target foundation and structural validation completed");
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main();
  } catch (error) {
    console.error(`RESET_REFUSED_OR_FAILED: ${error.message}`);
    process.exitCode = 1;
  }
}
