import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

const scriptPath = fileURLToPath(import.meta.url);
export const repositoryRoot = path.resolve(path.dirname(scriptPath), "..");

const ignoredDirectories = new Set([
  ".git",
  ".gocache",
  ".pnpm-store",
  ".tmp",
  ".vite",
  "coverage",
  "dist",
  "node_modules",
  "playwright-report",
  "test-results",
]);

export const expectedFnd001Counts = Object.freeze({
  goFiles: 375,
  vueFiles: 211,
  typeScriptFiles: 178,
  sqlFiles: 202,
  upMigrations: 98,
  goTestFiles: 99,
});

// Keep the approved Phase 0 snapshot immutable while making later task deltas
// explicit and reviewable. SEC-001 adds seven Go files (five tests). SEC-002
// adds one Go implementation/test pair plus a TypeScript test and declaration.
// SEC-003 adds two Go implementation files, ten focused Go test files, and one
// TypeScript remediation test. SEC-004 adds five Go files (three tests), two
// TypeScript files, and one paired legacy compatibility migration. SEC-005 adds
// eight Go files (four tests) and one TypeScript test. SEC-006 has a final net
// addition of two Go test files: the provider retirement removes seven active
// Go files and adds two focused retirement tests. SEC-007 adds two Go
// implementation/test pairs in Admin auth, two TypeScript test files, and one
// paired MFA migration. P1-REM-001 adds one TypeScript Playwright mock helper.
// No completed task delta adds Vue files.
export const documentedInventoryDeltas = Object.freeze({
  "SEC-001": Object.freeze({ goFiles: 7, goTestFiles: 5 }),
  "SEC-002": Object.freeze({ goFiles: 2, goTestFiles: 1, typeScriptFiles: 2 }),
  "SEC-003": Object.freeze({ goFiles: 12, goTestFiles: 10, typeScriptFiles: 1 }),
  "SEC-004": Object.freeze({
    goFiles: 5, goTestFiles: 3, typeScriptFiles: 2, sqlFiles: 2, upMigrations: 1,
  }),
  "SEC-005": Object.freeze({ goFiles: 8, goTestFiles: 4, typeScriptFiles: 1 }),
  "SEC-006": Object.freeze({ goFiles: 2, goTestFiles: 1 }),
  "SEC-007": Object.freeze({
    goFiles: 4, goTestFiles: 2, typeScriptFiles: 2, sqlFiles: 2, upMigrations: 1,
  }),
  "P1-REM-001": Object.freeze({ typeScriptFiles: 1 }),
});

export const expectedCurrentCounts = Object.freeze(
  Object.entries(documentedInventoryDeltas).reduce(
    (counts, [, delta]) => {
      for (const [metric, amount] of Object.entries(delta)) {
        counts[metric] += amount;
      }
      return counts;
    },
    { ...expectedFnd001Counts },
  ),
);

function toRepositoryPath(absolutePath, root = repositoryRoot) {
  return path.relative(root, absolutePath).split(path.sep).join("/");
}

export function collectFiles(root = repositoryRoot) {
  const files = [];

  function visit(directory) {
    const entries = fs
      .readdirSync(directory, { withFileTypes: true })
      .sort((left, right) => left.name.localeCompare(right.name));

    for (const entry of entries) {
      if (entry.isDirectory() && ignoredDirectories.has(entry.name)) {
        continue;
      }

      const absolutePath = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        visit(absolutePath);
      } else if (entry.isFile()) {
        files.push(toRepositoryPath(absolutePath, root));
      }
    }
  }

  visit(root);
  return files.sort();
}

function immediateDirectories(relativeRoot, root = repositoryRoot) {
  const absoluteRoot = path.join(root, relativeRoot);
  return fs
    .readdirSync(absoluteRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => entry.name)
    .sort();
}

function countFilesUnder(files, prefix, predicate) {
  const normalizedPrefix = `${prefix.replaceAll("\\", "/")}/`;
  return files.filter(
    (file) => file.startsWith(normalizedPrefix) && predicate(file),
  ).length;
}

function moduleInventory(files, root = repositoryRoot) {
  const moduleRoots = [
    ...immediateDirectories("apps", root).map((name) => `apps/${name}`),
    ...immediateDirectories("packages", root).map(
      (name) => `packages/${name}`,
    ),
  ];

  return moduleRoots
    .map((modulePath) => {
      const goFiles = countFilesUnder(files, modulePath, (file) =>
        file.endsWith(".go"),
      );
      const goTestFiles = countFilesUnder(files, modulePath, (file) =>
        file.endsWith("_test.go"),
      );
      return { module: modulePath, goFiles, goTestFiles };
    })
    .filter((entry) => entry.goFiles > 0);
}

function applicationInventory(files, root = repositoryRoot) {
  return immediateDirectories("apps", root).map((name) => {
    const applicationPath = `apps/${name}`;
    return {
      application: applicationPath,
      goModule: files.includes(`${applicationPath}/go.mod`),
      nodePackage: files.includes(`${applicationPath}/package.json`),
      mainFiles: countFilesUnder(
        files,
        applicationPath,
        (file) => path.posix.basename(file) === "main.go",
      ),
      dockerfiles: countFilesUnder(files, applicationPath, (file) =>
        path.posix.basename(file).startsWith("Dockerfile"),
      ),
    };
  });
}

function packageInventory(files, root = repositoryRoot) {
  return immediateDirectories("packages", root).map((name) => {
    const packagePath = `packages/${name}`;
    return {
      package: packagePath,
      goModule: files.includes(`${packagePath}/go.mod`),
      nodePackage:
        files.includes(`${packagePath}/package.json`) ||
        files.some(
          (file) =>
            file.startsWith(`${packagePath}/`) &&
            path.posix.basename(file) === "package.json",
        ),
    };
  });
}

export function buildInventory(root = repositoryRoot) {
  const files = collectFiles(root);
  const modules = moduleInventory(files, root);
  // FND-001 is an immutable snapshot of the extracted legacy repository.
  // FND-004 validates its isolated target SQL independently.
  const baselineSqlFiles = files.filter(
    (file) =>
      file.endsWith(".sql") &&
      !file.startsWith("packages/db/init/target/") &&
      !file.startsWith("packages/db/migrations/target/"),
  );
  const migrationFiles = baselineSqlFiles.filter((file) =>
    file.startsWith("packages/db/migrations/"),
  );

  return {
    counts: {
      files: files.length,
      goFiles: files.filter((file) => file.endsWith(".go")).length,
      vueFiles: files.filter((file) => file.endsWith(".vue")).length,
      typeScriptFiles: files.filter(
        (file) => file.endsWith(".ts") || file.endsWith(".tsx"),
      ).length,
      sqlFiles: baselineSqlFiles.length,
      upMigrations: migrationFiles.filter((file) =>
        file.endsWith(".up.sql"),
      ).length,
      downMigrations: migrationFiles.filter((file) =>
        file.endsWith(".down.sql"),
      ).length,
      goTestFiles: files.filter((file) => file.endsWith("_test.go")).length,
      frontendTestFiles: files.filter((file) =>
        /(?:test|spec)\.(?:js|jsx|ts|tsx)$/.test(file),
      ).length,
    },
    applications: applicationInventory(files, root),
    packages: packageInventory(files, root),
    goModulesWithoutTests: modules
      .filter((entry) => entry.goTestFiles === 0)
      .map((entry) => entry.module),
  };
}

export function extractMarkdownLinks(markdown) {
  const links = [];
  const expression = /\[[^\]]+\]\(([^)]+)\)/g;
  let match;

  while ((match = expression.exec(markdown)) !== null) {
    let target = match[1].trim();
    if (target.startsWith("<") && target.endsWith(">")) {
      target = target.slice(1, -1);
    } else {
      target = target.split(/\s+["']/)[0];
    }
    links.push(target);
  }

  return links;
}

function isExternalOrAnchor(target) {
  return (
    target.startsWith("#") ||
    /^(?:data|https?|mailto|tel):/i.test(target)
  );
}

export function validateMarkdownFiles(markdownFiles, root = repositoryRoot) {
  const missing = [];
  let checkedLinks = 0;

  for (const markdownFile of markdownFiles) {
    const absoluteMarkdownPath = path.resolve(root, markdownFile);
    if (!fs.existsSync(absoluteMarkdownPath)) {
      missing.push(`${markdownFile}: document is missing`);
      continue;
    }

    const markdown = fs.readFileSync(absoluteMarkdownPath, "utf8");
    for (const rawTarget of extractMarkdownLinks(markdown)) {
      if (isExternalOrAnchor(rawTarget)) {
        continue;
      }

      const targetWithoutAnchor = rawTarget.split("#", 1)[0];
      if (!targetWithoutAnchor) {
        continue;
      }

      checkedLinks += 1;
      const decodedTarget = decodeURIComponent(targetWithoutAnchor);
      const absoluteTarget = path.resolve(
        path.dirname(absoluteMarkdownPath),
        decodedTarget,
      );
      if (!fs.existsSync(absoluteTarget)) {
        missing.push(`${markdownFile}: ${rawTarget}`);
      }
    }
  }

  return { checkedLinks, missing };
}

export function validateFindingRows(
  auditPath = "docs/architecture/current-state-audit.md",
  root = repositoryRoot,
) {
  const absoluteAuditPath = path.resolve(root, auditPath);
  if (!fs.existsSync(absoluteAuditPath)) {
    return { findingRows: 0, errors: [`${auditPath}: document is missing`] };
  }

  const rows = fs
    .readFileSync(absoluteAuditPath, "utf8")
    .split(/\r?\n/)
    .filter((line) => /^\|\s*P[01]-/.test(line));
  const errors = [];

  for (const row of rows) {
    const cells = row.split("|").map((cell) => cell.trim());
    const identifier = cells[1] ?? "unknown";
    const severity = cells[2] ?? "";
    const links = extractMarkdownLinks(row).filter(
      (target) => !isExternalOrAnchor(target),
    );

    if (!["P0", "P1"].includes(severity)) {
      errors.push(`${identifier}: missing P0/P1 severity`);
    }
    if (links.length === 0) {
      errors.push(`${identifier}: no repository evidence link`);
    }
  }

  if (rows.length !== 35) {
    errors.push(`expected 35 P0/P1 finding rows, found ${rows.length}`);
  }

  return { findingRows: rows.length, errors };
}

function parseToolVersions(root = repositoryRoot) {
  const values = {};
  const lines = fs
    .readFileSync(path.join(root, ".tool-versions"), "utf8")
    .split(/\r?\n/);
  for (const line of lines) {
    const [tool, version] = line.trim().split(/\s+/, 2);
    if (tool && version) {
      values[tool] = version;
    }
  }
  return values;
}

export function verifyToolchains(root = repositoryRoot) {
  const errors = [];
  const warnings = [];
  const versions = parseToolVersions(root);
  const goWork = fs.readFileSync(path.join(root, "go.work"), "utf8");
  const packageManifest = JSON.parse(
    fs.readFileSync(path.join(root, "package.json"), "utf8"),
  );
  const workflow = fs.readFileSync(
    path.join(root, ".github", "workflows", "ci.yml"),
    "utf8",
  );

  const goWorkVersion = goWork.match(/^go\s+(\S+)/m)?.[1];
  if (versions.golang !== goWorkVersion) {
    errors.push(
      `.tool-versions Go ${versions.golang} differs from go.work ${goWorkVersion}`,
    );
  }

  const packageManager = packageManifest.packageManager;
  if (packageManager !== `pnpm@${versions.pnpm}`) {
    errors.push(
      `.tool-versions pnpm ${versions.pnpm} differs from packageManager ${packageManager}`,
    );
  }

  if (!packageManifest.engines.node.includes(versions.nodejs)) {
    errors.push(
      `Node engine ${packageManifest.engines.node} does not contain baseline ${versions.nodejs}`,
    );
  }
  if (packageManifest.engines.pnpm !== versions.pnpm) {
    errors.push(
      `pnpm engine ${packageManifest.engines.pnpm} differs from baseline ${versions.pnpm}`,
    );
  }

  if (!workflow.includes('go-version: "1.24"')) {
    errors.push("CI does not select the Go 1.24 toolchain line");
  } else {
    warnings.push("CI selects Go 1.24 without the baseline patch version 1.24.7");
  }
  if (!workflow.includes('node-version: "20"')) {
    errors.push("CI does not select the Node 20 toolchain line");
  } else {
    warnings.push("CI selects Node 20 without the baseline patch version 20.19.0");
  }
  if (!/version:\s*8\b/.test(workflow)) {
    errors.push("CI does not select the pnpm 8 toolchain line");
  } else {
    warnings.push("CI selects pnpm 8 without the baseline patch version 8.15.0");
  }

  return { versions, errors, warnings };
}

function printInventory() {
  process.stdout.write(`${JSON.stringify(buildInventory(), null, 2)}\n`);
}

function verifyBaseline() {
  const firstInventory = buildInventory();
  const secondInventory = buildInventory();
  const failures = [];

  if (JSON.stringify(firstInventory) !== JSON.stringify(secondInventory)) {
    failures.push("inventory changed between two consecutive reads");
  } else {
    console.log("PASS inventory is reproducible across two consecutive reads");
  }

  for (const [metric, expected] of Object.entries(expectedCurrentCounts)) {
    const actual = firstInventory.counts[metric];
    if (actual !== expected) {
      failures.push(`${metric}: expected ${expected}, found ${actual}`);
    }
  }
  if (failures.length === 0) {
    console.log("PASS current inventory matches the FND-001 snapshot plus documented task deltas");
  }

  const findingValidation = validateFindingRows();
  failures.push(...findingValidation.errors);
  if (findingValidation.errors.length === 0) {
    console.log(
      `PASS ${findingValidation.findingRows} P0/P1 findings have severity and repository evidence links`,
    );
  }

  const markdownFiles = [
    "README.md",
    "docs/architecture/current-state-audit.md",
    "docs/codex/reports/FND-001-local-execution-report.md",
  ].filter((file) => fs.existsSync(path.join(repositoryRoot, file)));
  const markdownValidation = validateMarkdownFiles(markdownFiles);
  failures.push(...markdownValidation.missing);
  if (markdownValidation.missing.length === 0) {
    console.log(
      `PASS ${markdownValidation.checkedLinks} local Markdown links resolve`,
    );
  }

  const toolchains = verifyToolchains();
  failures.push(...toolchains.errors);
  if (toolchains.errors.length === 0) {
    console.log(
      `PASS local baseline toolchains agree with repository requirements: Go ${toolchains.versions.golang}, Node ${toolchains.versions.nodejs}, pnpm ${toolchains.versions.pnpm}`,
    );
  }
  for (const warning of toolchains.warnings) {
    console.log(`WARN ${warning}`);
  }

  if (failures.length > 0) {
    for (const failure of failures) {
      console.error(`FAIL ${failure}`);
    }
    process.exitCode = 1;
    return;
  }

  console.log("PASS FND-001 baseline verification completed");
}

function main() {
  const command = process.argv[2] ?? "inventory";
  if (command === "inventory") {
    printInventory();
    return;
  }
  if (command === "verify") {
    verifyBaseline();
    return;
  }

  console.error(
    `Unknown command "${command}". Use "inventory" or "verify".`,
  );
  process.exitCode = 2;
}

if (process.argv[1] && path.resolve(process.argv[1]) === scriptPath) {
  main();
}
