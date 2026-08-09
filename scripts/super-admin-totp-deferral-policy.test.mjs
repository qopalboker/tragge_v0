import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";

import {
  repositoryRoot,
  validateMarkdownFiles,
} from "./production-baseline.mjs";

const policyPath = "docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md";
const roadmapPath = "docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md";
const phasePromptPath = "docs/codex/prompts/02_PHASE_1_SECURITY.md";
const glossaryPath = "docs/product/canonical-domain-glossary-and-version-catalog.md";
const reportPath = "docs/codex/reports/super-admin-totp-deferral-policy-amendment.md";
const alignedMarkdownPaths = [
  policyPath,
  roadmapPath,
  phasePromptPath,
  glossaryPath,
  "docs/security/user-admin-authentication-isolation.md",
  "docs/security/session-authentication-url-policy.md",
  "docs/security/otp-and-reset-delivery.md",
  "docs/architecture/current-state-audit.md",
  "docs/architecture/database-migration-reset-strategy.md",
  "docs/architecture/migration-inventory.md",
  reportPath,
];

function read(relativePath) {
  return fs.readFileSync(path.join(repositoryRoot, relativePath), "utf8");
}

function taskBlock(roadmap, taskId) {
  const expression = new RegExp(
    `^### ${taskId} [^\\n]*\\n[\\s\\S]*?(?=^### [A-Z]+-\\d{3} |^## Phase |(?![\\s\\S]))`,
    "m",
  );
  const match = roadmap.match(expression);
  assert.ok(match, `missing task block ${taskId}`);
  return match[0];
}

function compact(value) {
  return value.replace(/\s+/g, " ").trim();
}

test("fixed policy supersedes the temporary deferral with the implemented MFA gate", () => {
  const policy = read(policyPath);
  assert.match(policy, /\*\*Policy version:\*\* `2026-08-09\.1`/);
  assert.match(policy, /Super Admin login requires password verification followed by the Admin-only MFA/);
  assert.match(policy, /Password verification alone must not issue a Super Admin session/);
  assert.match(policy, /`SEC-007` implements the versioned `super_admin_totp_v1` contract/);
  assert.match(policy, /Password-only Super Admin authentication is prohibited/);
  assert.match(policy, /Only Super Admin may execute approved destructive\s+financial operations/);
  assert.match(policy, /Support Admin remains limited to explicitly approved\s+support and KYC permissions/);
  assert.match(policy, /Super Admin MFA from `SEC-007` is enforced and validated/);
  assert.match(policy, /Sensitive-action password reauthentication from `SEC-004` is enforced/);
});

test("SEC-004 contains only sensitive-action reauthentication and privilege enforcement", () => {
  const roadmap = read(roadmapPath);
  const block = taskBlock(roadmap, "SEC-004");
  assert.match(block, /Implement sensitive-action password reauthentication and privileged-action enforcement/);
  for (const required of [
    "short-lived, single-use",
    "Admin-context-specific",
    "actor",
    "session",
    "action",
    "resource",
    "Only Super Admin",
    "Support Admin",
    "no Finance role",
    "immutable",
    "safe authorization-denial audit",
    "SEC-001 through SEC-003 regressions",
  ]) {
    assert.ok(compact(block).includes(compact(required)), `SEC-004 missing ${required}`);
  }
  assert.doesNotMatch(block, /Add TOTP|TOTP enrollment|TOTP vector|valid TOTP|QR-code|recovery-code handling|require totp for super admins/i);
  assert.match(block, /(?:must|do) not implement, activate, require, or partially roll out Super Admin login MFA/i);
});

test("SEC-007 remains the unique security task and records its implementation contract", () => {
  const roadmap = read(roadmapPath);
  const ids = [...roadmap.matchAll(/^### ([A-Z]+-\d{3})\b/gm)].map((match) => match[1]);
  assert.equal(ids.length, new Set(ids).size, "duplicate roadmap task ID");
  assert.deepEqual(
    ids.filter((id) => id.startsWith("SEC-")),
    ["SEC-001", "SEC-002", "SEC-003", "SEC-004", "SEC-005", "SEC-006", "SEC-007"],
  );
  const block = taskBlock(roadmap, "SEC-007");
  for (const required of [
    "Implemented by the SEC-007 task change set",
    "Required before paid-production approval can be reconsidered.",
    "super_admin_totp_v1",
    "Google-Authenticator-compatible TOTP",
    "secure enrollment",
    "encrypted TOTP-secret storage",
    "replay prevention",
    "recovery codes",
    "reset/recovery procedures",
    "session upgrade",
    "audit",
    "production startup",
    "frontend",
    "Real database",
    "concurrency",
  ]) {
    assert.ok(compact(block).toLowerCase().includes(compact(required).toLowerCase()), `SEC-007 missing ${required}`);
  }
});

test("Phase 1 controller separates SEC-004 from implemented SEC-007", () => {
  const prompt = read(phasePromptPath);
  assert.match(prompt, /`SEC-004` — Implement sensitive-action password reauthentication and privileged-action enforcement/);
  assert.match(prompt, /`SEC-007` — Implement Super Admin MFA before paid-production approval/);
  assert.match(prompt, /`SEC-004` must not implement, activate, require, or partially roll out Super\s+Admin login MFA/);
  assert.match(prompt, /`SEC-007` owns the implemented Google-Authenticator-compatible TOTP/);
  assert.match(prompt, /completed Super Admin MFA under `SEC-007`/);
  assert.doesNotMatch(prompt, /`SEC-004` —[^\n]*(?:TOTP|MFA)/i);
});

test("glossary and version catalog distinguish the two implemented security controls", () => {
  const glossary = read(glossaryPath);
  assert.match(glossary, /\*\*Catalog version:\*\* `2026-08-09\.1`/);
  assert.match(glossary, /\*\*Super Admin\*\*[^\n]*super_admin_totp_v1/);
  assert.match(glossary, /\*\*Sensitive-Action Password Reauthentication\*\*[^\n]*`SEC-004`/);
  assert.match(glossary, /\*\*Reauthentication Grant\*\*[^\n]*short-lived, single-use/);
  assert.match(glossary, /\*\*Super Admin MFA\*\*[^\n]*implemented `SEC-007`/);
  assert.match(glossary, /User\/Admin authentication isolation \| No public contract version assigned; implemented boundary recorded by `SEC-001` \| current implementation/);
  assert.match(glossary, /Sensitive-action reauthentication contract \| Not assigned \(current local implementation; no public contract version\) \| current local implementation/);
  assert.match(glossary, /Super Admin MFA contract \| `super_admin_totp_v1` \| current implementation/);
});

test("aligned current documentation has no SEC-004 TOTP ownership claim", () => {
  for (const relativePath of alignedMarkdownPaths.filter((file) => file !== reportPath)) {
    const markdown = read(relativePath);
    assert.doesNotMatch(
      markdown,
      /SEC-004 owns (?:mandatory )?Super Admin TOTP|SEC-004 (?:implements|provides|requires) (?:mandatory )?(?:Super Admin )?(?:TOTP|login MFA)|(?:TOTP|login MFA) (?:is|are) (?:owned|implemented|required|provided) by SEC-004/i,
      relativePath,
    );
  }
});

test("all changed Markdown links, repository paths, style, and task references resolve", () => {
  for (const relativePath of alignedMarkdownPaths) {
    assert.ok(fs.existsSync(path.join(repositoryRoot, relativePath)), `missing ${relativePath}`);
    const markdown = read(relativePath);
    assert.doesNotMatch(markdown, /\t/, `${relativePath} contains a tab`);
    for (const [index, line] of markdown.split(/\r?\n/).entries()) {
      const trailingSpaces = line.match(/ +$/)?.[0].length ?? 0;
      assert.ok(
        trailingSpaces === 0 || trailingSpaces === 2,
        `${relativePath}:${index + 1} has non-structural trailing whitespace`,
      );
    }
  }
  const result = validateMarkdownFiles(alignedMarkdownPaths, repositoryRoot);
  assert.deepEqual(result.missing, [], `missing links:\n${result.missing.join("\n")}`);

  const roadmap = read(roadmapPath);
  const references = new Set(
    alignedMarkdownPaths.flatMap((file) => read(file).match(/\b(?:SEC|FND|ARCH|DATA|CON|PRIZE|ENG|MD|FE|SCH|PAY|OPS|REL)-\d{3}\b/g) ?? []),
  );
  for (const id of references) {
    assert.match(roadmap, new RegExp(`^### ${id}\\b`, "m"), `unknown roadmap task ${id}`);
  }
});

test("the amendment remains documentation-only and paid production stays NO-GO", () => {
  const report = read(reportPath);
  assert.match(report, /Amendment decision:\*\* `PASS`/);
  assert.match(report, /SEC-004 implementation was not started/);
  assert.match(report, /SEC-007 was not started/);
  assert.match(report, /No application, migration, runtime configuration, frontend, dependency, or\s+infrastructure behavior changed/);
  assert.match(report, /Paid-production status remains `NO-GO`/);
});
