# Super Admin TOTP deferral policy amendment

**Amendment date:** 2026-07-29

**Execution mode:** Local documentation-only amendment

**Amendment decision:** `PASS`

**Paid-production status:** `NO-GO`

## 1. Amendment date

The approved policy amendment was recorded on 2026-07-29 in the
`Asia/Tehran` timezone. The fixed-policy and roadmap versions are
`2026-07-29.1`.

## 2. Local documentation-only mode

Work occurred directly in the selected extracted project. Git metadata was
absent and was not initialized. No branch, commit, remote, push, pull request,
merge, CI, deployment, production credential, production target, or real-user
data was used. This report records documentation and validation evidence only;
it does not claim an implementation or runtime test passed.

## 3. Previous policy

The superseded fixed-policy text required password plus
Google-Authenticator-compatible TOTP for Super Admin login. The roadmap assigned
TOTP enrollment, secret storage, recovery, login challenge, and sensitive-action
password reauthentication to one task, `SEC-004`. The Phase 1 controller and
paid-launch gates repeated that combined responsibility.

Historical reports and current-state evidence retain their original statements
as evidence of the policy in force when those reports were written. This dated
amendment supersedes their forward-looking task-ownership statements; it does
not rewrite completed-task evidence.

## 4. Newly approved policy

The current policy is recorded in the
[fixed product and technical policies](../../product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md):

- Super Admin login may remain password-based at the current development stage
  inside the isolated Admin trust domain established by `SEC-001`.
- A valid Admin password and all existing Admin authorization, session,
  revocation, cookie, and CSRF controls remain mandatory.
- `SEC-004` must not implement, activate, require, or partially roll out login
  TOTP or another Super Admin login MFA mechanism.
- Password-only authentication is not sufficient paid-production evidence.
- Fresh password reauthentication remains mandatory for destructive financial
  and security-sensitive actions.
- Roles remain `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN`; no Finance role is
  introduced.
- Only Super Admin may execute approved destructive financial operations.
- Planned `SEC-007` must implement Super Admin MFA before paid-production
  approval can be reconsidered.

## 5. Security rationale and accepted temporary risk

The accepted temporary risk is that a Super Admin password remains the only
login factor during the current development stage. This increases account-
takeover impact relative to MFA and is explicitly unacceptable as final paid-
production posture.

Current mitigations remain the SEC-001 isolated Admin cryptographic/session
trust domain, explicit Admin authorization, session and revocation controls,
SEC-002 URL-credential prohibition, SEC-003 fail-closed identity-code handling,
and the mandatory action-specific password reauthentication assigned to
`SEC-004`. These controls reduce risk but do not replace MFA. Paid production
remains blocked until `SEC-007` and every other launch gate pass.

## 6. SEC-004 revised scope

The [revised SEC-004 roadmap block](../PRODUCTION_ROADMAP_AND_CODEX_TASKS.md)
is titled **Implement sensitive-action password reauthentication and
privileged-action enforcement**. It retains:

- fresh Admin-password verification for covered sensitive actions;
- short-lived, single-use reauthentication grants;
- Admin-context, actor, session, action, and resource binding;
- expiry, replay rejection, and invalidation after password, session, or
  permission changes;
- Super-Admin-only destructive financial operations;
- explicit Support Admin and Super Admin permissions without a Finance role;
- mandatory reasons where required;
- immutable success and safe authorization-denial audit events; and
- SEC-001 through SEC-003 regressions.

It contains no TOTP enrollment, QR enrollment, TOTP secret storage, TOTP login
challenge, recovery-code, TOTP reset/recovery, frontend MFA, or production MFA
configuration requirement. `SEC-004` implementation was not started here.

## 7. Future MFA task ID and title

The roadmap previously contained 97 unique tasks and the security sequence ended
at `SEC-006`. The next valid non-conflicting identifier is:

`SEC-007 — Implement Super Admin MFA before paid-production approval`

`SEC-007` is explicitly planned, not implemented, not started, and required
before paid-production approval can be reconsidered. Its task block covers
Google-Authenticator-compatible TOTP, secure enrollment, encrypted secret
storage, replay prevention, recovery codes, reset/recovery, MFA-only session
upgrade, immutable audit, production startup validation, frontend enrollment
and login flows, and real database/concurrency evidence.

The roadmap now contains 98 unique task IDs and the security sequence is exactly
`SEC-001` through `SEC-007`.

## 8. Paid-production impact

The policy change does not relax the launch gate. The fixed policy and roadmap
now require two separate completed controls:

1. sensitive-action password reauthentication and privileged-action enforcement
   from `SEC-004`; and
2. Super Admin MFA from `SEC-007`.

Password-only Super Admin login is approved only for the current development
stage. It is not evidence of paid-production readiness. Paid-production status
remains `NO-GO`.

## 9. Files changed

1. `docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md`
2. `docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md`
3. `docs/codex/prompts/02_PHASE_1_SECURITY.md`
4. `docs/product/canonical-domain-glossary-and-version-catalog.md`
5. `docs/security/user-admin-authentication-isolation.md`
6. `docs/security/session-authentication-url-policy.md`
7. `docs/security/otp-and-reset-delivery.md`
8. `docs/architecture/current-state-audit.md`
9. `docs/architecture/database-migration-reset-strategy.md`
10. `docs/architecture/migration-inventory.md`
11. `scripts/domain-glossary.test.mjs`
12. `scripts/super-admin-totp-deferral-policy.test.mjs`
13. `docs/codex/reports/super-admin-totp-deferral-policy-amendment.md`

No application, migration, runtime configuration, frontend, dependency, or
infrastructure behavior changed. The focused Node files validate documentation;
they do not implement product behavior.

## 10. Every command executed and exact result

All commands ran from the local project root. Read commands used UTF-8 where
text interpretation mattered.

| # | Command or operation | Exact result |
|---:|---|---|
| 1 | Required-path, protocol, `.git`, and SEC-004 report `Test-Path`/`rg` inventory | Exit `0`; all ten named authorities existed, `.git=False`, SEC-004 report absent; the initial protocol filename filter returned an empty object because Windows paths used backslashes. |
| 2 | `rg --files docs/codex | rg -i "protocol|execution|operating"` | Exit `0`; canonical protocol resolved to `docs/codex/CODEX_EXECUTION_PROTOCOL.md`. |
| 3 | Authority line/word/character measurement | Exit `0`; all eleven required documents/reports were present and sized for complete chunked reads. |
| 4 | Two chunked reads of the complete fixed policy | Both exit `0`; all policy sections read. |
| 5 | Thirteen chunked reads plus one exact line-count command for the complete roadmap | Reads exited `0`; actual count 4,841 lines. One oversized read output was truncated by the tool and was repeated in three bounded chunks so no content remained unread. |
| 6 | Complete canonical protocol read | Exit `0`; local documentation-only rules and reporting requirements confirmed. |
| 7 | Complete Phase 1 controller and ADR-0001 reads | Exit `0`; controller conflict and Accepted architecture status confirmed. |
| 8 | Complete glossary/version-catalog read | Exit `0`; no Admin-security glossary/version rows existed before amendment. |
| 9 | Complete Phase 0 exit-report read | Exit `0`; current decision `PASS`. |
| 10 | Complete SEC-001 report read | Exit `0`; `SEC-001 PASS`. |
| 11 | Complete SEC-002 report read | Exit `0`; `SEC-002 PASS`. |
| 12 | Complete SEC-003 report read | Exit `0`; current `SEC-003 PASS`, original failure preserved. |
| 13 | Complete SEC-003 remediation report read | Exit `0`; remediation `PASS`, cleanup and no-SEC-004 evidence present. |
| 14 | Roadmap task-ID inventory plus repository MFA/reauth reference scan | Exit `0`; 97/97 unique tasks before amendment, security IDs `SEC-001` through `SEC-006`; conflict locations inventoried. |
| 15 | Complete FND-003/FND-005 validator reads and focused SEC-003 validator excerpt | Exit `0`; exact validator assumptions identified. |
| 16 | Current security-document and migration-evidence reads | Exit `0`; one combined output was truncated after relevant matches, followed by exact targeted reads before edits. |
| 17 | Context scan over ten amendment targets | Exit `0`; exact policy, task, prompt, security, and migration fragments captured. |
| 18 | First exact reference scan using a PowerShell wildcard path | Exit `1`; Windows `rg` rejected `docs/security/*.md`. The scan was rerun later with directory/glob-safe arguments. |
| 19 | Exact UTF-8 SEC-004, fixed-policy Admin, and Phase 1 controller excerpts | Exit `0`; final replacement anchors confirmed. |
| 20 | Built-in `apply_patch` for the fixed policy | Failed before read/write; Windows split-root sandbox wrapper could not be enforced. No file changed. |
| 21 | Occurrence-counted fixed-policy update | Exit `0`; version, current Admin policy, sensitive-action policy, future MFA policy, and launch gates updated. |
| 22 | First roadmap update command | Parser exit `1`; ambiguous PowerShell variable interpolation in an error string. No write occurred. |
| 23 | Second roadmap update command | Exit `1`; a single-quoted newline anchor matched zero occurrences. No write occurred. |
| 24 | Corrected occurrence-counted roadmap update | Exit `0`; version/finding/gate summaries and SEC-004 updated, SEC-007 added, FE-009/risk/checklist aligned. |
| 25 | Occurrence-counted Phase 1 controller update | Exit `0`; task order, fixed decisions, evidence rules, E2E ownership, and phase gate aligned. |
| 26 | Occurrence-counted glossary/catalog update | Exit `0`; catalog version/date, five terms, collision rule, authority versions, and three security-state rows added. |
| 27 | Occurrence-counted security/architecture alignment across six files | Exit `0`; every target updated exactly once. |
| 28 | Occurrence-counted FND-003 validator update | Exit `0`; Admin terms, security catalog rows, and authority versions covered. |
| 29 | Create `scripts/super-admin-totp-deferral-policy.test.mjs` | Exit `0`; focused documentation-only validator created with no dependency. |
| 30 | `node --check` plus roadmap/task-ID/SEC-004 structural scan | Exit `0`; 98/98 unique tasks, security IDs 001-007, one SEC-007, zero prohibited SEC-004 implementation matches. |
| 31 | Directory/glob-safe contradiction scan | Exit `0`; matches were only explicit SEC-004 prohibitions, legacy schema evidence, and planned SEC-007 requirements. |
| 32 | Narrow focused contradiction matcher | Exit `0`; explicit prohibitions/SEC-007 adjacency no longer create false positives. |
| 33 | First FND-003 plus FND-005 regression command | Exit `1` overall: FND-003 passed 6/8 and exposed a case-sensitive assertion plus a merged Markdown table row; FND-005 passed 11/11. |
| 34 | Inspect exact glossary table and validator fragments | Exit `0`; row-boundary and case defects identified. |
| 35 | Correct catalog row boundary and case assertion | Exit `0`; exact occurrences updated. |
| 36 | `node scripts/domain-glossary.test.mjs` | Exit `0`; 8/8 passed. |
| 37 | Markdownlint availability probe | Exit `0`; `markdownlint` and `markdownlint-cli2` both unavailable. No Markdownlint pass is claimed. |
| 38 | Sequential eight-file Phase 0/SEC-001–003 regression wrapper | Exit `0`; 56/56 passed: 5 baseline, 4 architecture, 8 glossary, 10 migration/reset, 11 protocol, 10 SEC-001, 4 SEC-002, 4 SEC-003. |
| 39 | `node scripts/production-baseline.mjs verify` | Exit `0`; reproducible inventory, documented deltas, 35 P0/P1 evidence paths, 146 local links, and toolchains passed; three existing CI patch-version warnings remained. |
| 40 | Create this amendment report | Exit `0`; report created only under the required local report path. |
| 41 | Report-artifact inspection | Exit `0`; report existed at 17,220 bytes with all 21 required headings and two PASS markers. |
| 42 | First focused amendment-validator run | Exit `1`; 4/8 passed. The new validator exposed an invalid JavaScript `\z` end anchor, an over-strict intentional-hard-break check, and a whitespace-sensitive report assertion. This was validator failure, not policy or runtime evidence. |
| 43 | Focused validator and roadmap-block inspection | Exit `0`; source assertions and complete SEC-004/SEC-007 text were inspected. |
| 44 | Exact SEC-004/SEC-007 context scan | Exit `0`; Support Admin appeared at roadmap line 708 and session upgrade at lines 851/858, proving the failed assertions had truncated blocks. |
| 45 | Direct task-block extractor diagnostic | Exit `0`; both extracted blocks stopped immediately before the first lowercase `z`, confirming unsupported `\z` was interpreted as a literal. |
| 46 | Built-in `apply_patch` for validator fixes | Failed before read/write with the same Windows split-root sandbox-wrapper limitation. No file changed. |
| 47 | Occurrence-counted three-fragment validator correction | Exit `0`; EOF extraction, intentional two-space Markdown hard breaks, and wrapped report wording were corrected exactly once. |
| 48 | `node --check` plus second focused validator run | Syntax check passed; validator exit `1`, 7/8 passed. The only remaining failure was an assertion requiring “must not” while the roadmap used equivalent “Do not” wording. |
| 49 | Occurrence-counted SEC-004 prohibition-assertion correction | Exit `0`; validator now accepts either explicit “must not” or “do not” prohibition wording. |
| 50 | `node --check` plus third focused validator run | Exit `0`; 8/8 passed. |
| 51 | Current-document SEC-004/TOTP contradiction and roadmap-ID scan | Exit `0`; four matches were compliant separation/legacy-replacement statements; 98/98 task IDs unique, zero duplicates, one `SEC-007`. |
| 52 | Changed-file credential scan | Exit `0`; 13 files scanned with zero private-key, complete-JWT, bearer-credential, or credential-bearing database-URL findings. |
| 53 | First documentation-only scope/Git/later-task/status check | Exit `1`; all 13 scope files existed and were allowed, `.git=False`, and SEC-004/SEC-007 reports were absent, but the fixed policy did not yet contain the literal `NO-GO` status even though it prohibited paid launch. |
| 54 | Exact `NO-GO` status scan | Exit `0`; roadmap and report contained the status; the fixed policy did not. |
| 55 | Fixed-policy MFA and launch-gate excerpt inspection | Exit `0`; paid launch was prohibited but the amendment-required literal status was absent. |
| 56 | Occurrence-counted fixed-policy status update | Exit `0`; `Paid-production status remains NO-GO` was added exactly once without behavior change. |
| 57 | Focused validator plus corrected scope/Git/later-task/status check | Exit `0`; 8/8 passed; 13/13 declared files existed and were in scope; `.git=False`; no SEC-004/SEC-007 report existed; explicit `NO-GO` found. |
| 58 | Final nine-file regression wrapper | Exit `0`; 64/64 passed: 5 baseline, 4 architecture, 8 glossary, 10 migration/reset, 11 protocol, 10 SEC-001, 4 SEC-002, 4 SEC-003, and 8 amendment tests. |
| 59 | Final `node scripts/production-baseline.mjs verify` | Exit `0`; reproducible inventory, documented deltas, 35 evidenced P0/P1 findings, 146 resolving local links, and toolchain checks passed; three existing CI patch-version warnings remained. |
| 60 | Final report reconciliation | Exit `0`; the command ledger, validation totals, explicit fixed-policy status, and failed-attempt evidence were reconciled to final repository state. |
| 61 | First final secret/report self-validation wrapper | Exit `1` because the wrapper omitted parentheses around a PowerShell collection comparison; its printed evidence nevertheless showed amendment tests 8/8, zero credential findings, 21 headings, and every boolean confirmation `True`. |
| 62 | Corrected final changed-file secret scan and report/decision self-validation | Exit `0`; amendment tests 8/8, zero credential findings, all 21 sections present, current decision `PASS`, no implementation/runtime-pass claim, and all required stop/status confirmations present. |
No application unit, integration, E2E, build, migration-runtime, database, Redis,
provider, or deployment command ran because this amendment changes no behavior.

## 11. Link and terminology validation

The focused amendment validator and FND-003 validator confirm:

- the canonical roles are `USER`, `SUPPORT_ADMIN`, and `SUPER_ADMIN`;
- no Finance role was introduced;
- SEC-004 reauthentication and SEC-007 MFA are distinct;
- planned MFA is not described as implemented;
- password-only login is not described as paid-production sufficient;
- changed-document local links and referenced repository paths resolve; and
- changed Markdown has no tab or non-structural trailing-space finding; deliberate two-space CommonMark hard breaks remain valid.

Markdownlint was unavailable. Focused Markdown structure, style, link, path,
and terminology checks passed; no Markdownlint pass is claimed.

## 12. Roadmap task-ID validation

Before amendment: 97 unique roadmap IDs, with `SEC-001` through `SEC-006`.
After amendment: 98 unique roadmap IDs, with `SEC-001` through `SEC-007`.
There is one and only one `SEC-007`; no existing ID was reused or renumbered.
All task IDs referenced by changed documents resolve to roadmap headings.

## 13. Policy contradiction result

`PASS`. The fixed policy, roadmap, Phase 1 controller, glossary/catalog,
current security documentation, current-state audit, and migration/reset
architecture evidence agree:

- SEC-004 implements only sensitive-action password reauthentication and
  privileged-action enforcement;
- SEC-007 is planned and owns Super Admin MFA;
- no document treats legacy conditional TOTP as target implementation;
- paid production requires both controls and remains `NO-GO`.

ADR-0001 remains Accepted and unchanged. No new ADR was required because the
ultimate isolated Admin boundary and mandatory pre-paid-production MFA target
remain unchanged; this versioned policy amendment changes task sequencing and
ownership, not a runtime architecture or implemented security boundary.

## 14. Scope review

The mutation commands named only the 13 files in section 9. Ten are Markdown
policy/process/security/architecture documents, two are focused Node validation
files, and one is this report. No `apps/**`, migration SQL, runtime
configuration, frontend source, infrastructure, package manifest, lockfile, or
dependency file was a mutation target. No application behavior changed and no
runtime test result is claimed.

## 15. Known remaining security risk

Super Admin login remains single-factor during the current development stage.
A compromised password can therefore compromise the privileged login trust
until `SEC-007` is implemented and activated. SEC-004 action reauthentication
reduces destructive-action risk but is not MFA and is not yet implemented.
SEC-005, SEC-006, and SEC-007 also remain planned. This accepted temporary risk
blocks paid production.

## 16. Explicit amendment decision

**Amendment decision:** `PASS`

All authoritative current documents agree, SEC-004 has no mandatory-TOTP
implementation requirement, the unique planned SEC-007 task tracks future MFA,
available documentation checks pass, and no behavior changed.

## 17. SEC-004 implementation status

SEC-004 implementation was not started. Only its roadmap definition and aligned
documentation changed.

## 18. Future MFA task status

SEC-007 was not started. It is planned, not implemented, and required before
paid-production approval can be reconsidered.

## 19. Application behavior confirmation

No application, migration, runtime configuration, frontend, dependency, or
infrastructure behavior changed. No implementation or runtime pass is claimed.

## 20. Git and remote confirmation

No Git metadata was created. No branch, commit, remote connection, push, pull
request, merge, CI, release, or deployment operation occurred.

## 21. Paid-production status

Paid-production status remains `NO-GO`.