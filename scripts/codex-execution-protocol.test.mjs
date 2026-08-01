import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

import {
  repositoryRoot,
  validateMarkdownFiles,
} from "./production-baseline.mjs";

const protocolPath = "docs/codex/CODEX_EXECUTION_PROTOCOL.md";
const templatePath = "docs/codex/templates/ROADMAP_TASK_TEMPLATE.md";
const promptReadmePath = "docs/codex/prompts/README.md";
const dryRunPath = "docs/codex/examples/EXAMPLE-DOC-001-local-dry-run.md";
const contributingPath = "CONTRIBUTING.md";
const prTemplatePath = ".github/pull_request_template.md";
const taskIssuePath = ".github/ISSUE_TEMPLATE/roadmap-task.md";
const bugIssuePath = ".github/ISSUE_TEMPLATE/bug-report.md";
const securityIssuePath = ".github/ISSUE_TEMPLATE/security-sensitive.md";
const roadmapPath = "docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md";
const policyPath = "docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md";
const adrPath = "docs/adr/0001-target-runtime-architecture.md";
const failedGatePath = "docs/codex/prompts/13_FAILED_GATE_REMEDIATION.md";

const phasePromptPaths = Array.from(
  { length: 12 },
  (_, index) => {
    const prefix = String(index + 1).padStart(2, "0");
    const entry = fs
      .readdirSync(path.join(repositoryRoot, "docs/codex/prompts"))
      .find((name) => name.startsWith(`${prefix}_`) && name.endsWith(".md"));
    assert.ok(entry, `missing phase prompt ${prefix}`);
    return `docs/codex/prompts/${entry}`;
  },
);

const processMarkdownPaths = [
  protocolPath,
  templatePath,
  promptReadmePath,
  dryRunPath,
  contributingPath,
  prTemplatePath,
  taskIssuePath,
  bugIssuePath,
  securityIssuePath,
  "docs/codex/prompts/00_BOOTSTRAP.md",
  ...phasePromptPaths,
  failedGatePath,
];

function read(relativePath) {
  return fs.readFileSync(path.join(repositoryRoot, relativePath), "utf8");
}

function compact(value) {
  return value.replace(/\s+/g, " ").trim().toLowerCase();
}

function requireText(document, values, label) {
  const normalizedDocument = compact(document);
  for (const value of values) {
    assert.ok(normalizedDocument.includes(compact(value)), `${label} missing: ${value}`);
  }
}

function requireHeadings(document, headings, label) {
  for (const heading of headings) {
    assert.match(
      document,
      new RegExp(`^#{1,3} ${heading.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`, "m"),
      `${label} missing heading: ${heading}`,
    );
  }
}

test("canonical protocol contains every required lifecycle section", () => {
  const protocol = read(protocolPath);
  requireHeadings(
    protocol,
    [
      "Purpose and process authority",
      "Invariant: one task, one goal, one scoped change set",
      "Execution mode decision",
      "Task selection and dependency verification",
      "Task-start checklist",
      "Implementation rules",
      "Testing and evidence rules",
      "Local execution mode",
      "Future Git-backed repository flow",
      "Task-completion checklist",
      "Required task report",
      "Phase-controller and phase-gate behavior",
      "Protected production and security behavior",
    ],
    "protocol",
  );
  requireText(protocol, ["single process authority", "one scoped change set", "stops after that task's report"], "protocol");
});

test("task template has all impact, test, report, and stop fields", () => {
  const template = read(templatePath);
  requireHeadings(
    template,
    [
      "Task ID and title",
      "Goal",
      "Non-goals",
      "Dependencies",
      "Authoritative policies",
      "Primary scope",
      "Allowed files/modules",
      "Forbidden scope",
      "Required implementation",
      "Data/schema impact",
      "Security impact",
      "Financial impact",
      "Contract/API impact",
      "Migration impact",
      "Observability impact",
      "Rollback strategy",
      "Unit tests",
      "Integration tests",
      "E2E tests",
      "Regression tests",
      "Verification commands",
      "Acceptance criteria",
      "Documentation updates",
      "Known limitations",
      "Required final report",
      "Stop condition",
    ],
    "task template",
  );
  requireText(
    template,
    [
      "Unrelated cleanup",
      "broad refactor",
      "Dependency addition or upgrade without documented approval",
      "Product-policy",
      "Silent architecture-boundary change",
      "Weakening, skipping, deleting, or quarantining tests",
      "Claiming an unavailable or unexecuted check passed",
      "Any second or future roadmap task",
    ],
    "task template",
  );
});

test("local mode works without Git and requires independently verified evidence", () => {
  const protocol = read(protocolPath);
  requireText(
    protocol,
    [
      "absence of `.git` must not block implementation",
      "must not initialize Git unless explicitly authorized",
      "must not connect to GitHub or any other remote service unless explicitly authorized",
      "docs/codex/reports/<TASK-ID>-local-execution-report.md",
      "Local completion is not equivalent to merge",
      "report's claim is necessary but never sufficient",
      "finds no unexpected changed file",
    ],
    "local mode",
  );
});

test("Git-backed flow protects main and gates push and merge", () => {
  const protocol = read(protocolPath);
  requireText(
    protocol,
    [
      "test/development repository",
      "canonical main repository",
      "protected `main`",
      "codex/<task-id-lower>-<short-kebab-slug>",
      "working tree must be clean",
      "Use one roadmap task per branch",
      "One Conventional Commit is the default",
      "Push is permitted only after dependencies are merged",
      "Merge is permitted only when",
      "all required CI checks pass on the final commit",
      "all review comments are resolved",
      "Squash merge is the default",
      "Prefer a new reviewed `revert` commit",
    ],
    "Git-backed flow",
  );
  for (const prefix of ["feat", "fix", "refactor", "test", "docs", "chore", "build", "ci", "perf", "revert"]) {
    assert.ok(protocol.includes("`" + prefix + "`"), `missing Conventional Commit prefix ${prefix}`);
  }
});

test("testing distinguishes behavioral, documentation-only, and gate work", () => {
  const protocol = read(protocolPath);
  requireText(
    protocol,
    [
      "For every behavior-changing task",
      "focused unit tests",
      "add integration tests",
      "critical user/Admin E2E journeys",
      "run lint, typecheck, and build/compile",
      "For a documentation-only task, do not add artificial application unit tests",
      "focused structural, local-link, terminology, repository-path, task-ID",
      "At the end of every Epic or Phase",
      "Numeric coverage is evidence, not proof of correctness",
      "Never convert a static check into a runtime-pass claim",
    ],
    "testing rules",
  );
});

test("dependency, ADR, report, and production safeguards are complete", () => {
  const protocol = read(protocolPath);
  requireText(
    protocol,
    [
      "If human approval is required and absent, stop and report a blocker",
      "system or module boundaries",
      "database ownership",
      "public API or WebSocket contracts",
      "cross-system command/event envelopes",
      "canonical data representations",
      "security boundaries",
      "consistency or concurrency model",
      "persistence, replay, backup, or recovery model",
      "provider-selection strategy",
      "deployment architecture",
      "Do not create an ADR for a trivial implementation detail",
      "Every exact command and result",
      "Known untested behavior",
      "Remaining risks and unresolved process/technical ambiguity",
      "must never fabricate commit hashes",
      "deploy paid production without an explicitly authorized launch task",
      "approve Market Data licensing or redistribution rights",
      "change Wallet balances or Settlement outcomes outside approved, audited",
    ],
    "safeguards",
  );
});

test("phase controllers stop after one task and failed gates do not weaken evidence", () => {
  const promptReadme = read(promptReadmePath);
  requireText(promptReadme, ["Implement exactly one task", "Never start the next phase automatically", "Do not weaken the gate"], "prompt README");
  for (const phasePath of phasePromptPaths) {
    const prompt = read(phasePath);
    assert.ok(prompt.includes("Codex execution protocol"), `${phasePath} lacks protocol authority`);
    assert.match(prompt, /Implement exactly that one task/i, `${phasePath} lacks one-task instruction`);
    assert.match(prompt, /Never start the next phase automatically/i, `${phasePath} can auto-start next phase`);
    assert.match(prompt, /Phase exit gate/i, `${phasePath} lacks explicit gate`);
  }
  const failedGate = read(failedGatePath);
  requireText(failedGate, ["Do not implement code in this invocation", "Do not weaken the gate", "Do not claim the phase passes"], "failed gate prompt");
});

test("contributing and future repository templates require complete evidence", () => {
  const contributing = read(contributingPath);
  const pr = read(prTemplatePath);
  const taskIssue = read(taskIssuePath);
  const bugIssue = read(bugIssuePath);
  const security = read(securityIssuePath);
  requireText(contributing, ["Fixed Product and Technical Policies", "Production Roadmap", "Canonical domain glossary", "ADR-0001", "Codex execution protocol", "roadmap task template", "local mode", "Git-backed mode", "Security reporting"], "CONTRIBUTING");
  requireText(pr, ["Task ID", "Dependencies", "Files/modules in scope", "Acceptance criteria", "Exact command", "Coverage impact", "Migration/data impact", "Security/privacy impact", "Financial/ledger/settlement impact", "Rollback and recovery", "Unresolved risks", "No unrelated cleanup"], "PR template");
  requireText(taskIssue, ["Task ID and title", "Dependencies and evidence", "Scope and allowed files", "Tests and verification commands", "Stop condition"], "task issue");
  requireText(bugIssue, ["Reproduction steps", "Expected behavior", "Actual behavior", "Required regression"], "bug issue");
  requireText(security, ["Do not submit sensitive details in a public issue", "approved private security-reporting channel", "Codex does not approve risk acceptance"], "security guidance");
});

test("fictional local dry run demonstrates the complete stop boundary", () => {
  const dryRun = read(dryRunPath);
  requireText(
    dryRun,
    [
      "Fictional documentation-only task; not a roadmap task",
      "Execution mode:** Local extracted project",
      "selects EXAMPLE-DOC-001",
      "does not combine EXAMPLE-DOC-002",
      "Scoped-files demonstration",
      "Artificial application unit, integration, or E2E tests are not required",
      "Example final report structure",
      "No real roadmap task or next phase was started",
      "created no branch, commit, push, PR, merge, CI, or deployment evidence",
      "Stop-after-one-task demonstration",
    ],
    "dry run",
  );
});

test("all process Markdown links, paths, styles, and task IDs resolve", () => {
  const linkResult = validateMarkdownFiles(processMarkdownPaths, repositoryRoot);
  assert.deepEqual(linkResult.missing, [], `missing links:\n${linkResult.missing.join("\n")}`);
  assert.ok(linkResult.checkedLinks >= 40, `expected at least 40 checked links, got ${linkResult.checkedLinks}`);

  for (const file of processMarkdownPaths) {
    assert.ok(fs.existsSync(path.join(repositoryRoot, file)), `missing process path ${file}`);
    const markdown = read(file);
    assert.doesNotMatch(markdown, /\t/, `${file} contains a tab`);
    for (const [index, line] of markdown.split(/\r?\n/).entries()) {
      assert.doesNotMatch(line, /[ \t]+$/, `${file}:${index + 1} has trailing whitespace`);
    }
  }

  const roadmap = read(roadmapPath);
  const combined = processMarkdownPaths.map(read).join("\n").replaceAll("EXAMPLE-DOC-001", "").replaceAll("EXAMPLE-DOC-002", "");
  const taskIds = new Set(combined.match(/\b[A-Z]+-\d{3}\b/g) ?? []);
  taskIds.delete("ADR-0001");
  for (const identifier of taskIds) {
    assert.match(roadmap, new RegExp(`^### ${identifier}\\b`, "m"), `unknown roadmap task ${identifier}`);
  }
});

test("process rules preserve fixed policy and ADR-0001 boundaries", () => {
  const protocol = read(protocolPath);
  const policy = read(policyPath);
  const adr = read(adrPath);
  const normalizedProtocol = compact(protocol);
  for (const authority of ["Platform Modular Monolith", "Trading Engine", "Market Data Service"]) {
    assert.ok(compact(policy).includes(compact(authority)), `policy missing ${authority}`);
    assert.ok(compact(adr).includes(compact(authority)), `ADR missing ${authority}`);
    assert.ok(normalizedProtocol.includes(compact(authority)), `protocol missing ${authority}`);
  }
  assert.match(normalizedProtocol, /removed second chance capability must not be introduced as active behavior/);
  assert.match(normalizedProtocol, /product-level participant capacity must not be created/);
  assert.match(normalizedProtocol, /paid production remains `no-go`/);
  assert.doesNotMatch(normalizedProtocol, /second chance (?:is|as) (?:active|planned|supported)/);
});
