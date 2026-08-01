# Contributing to Tragge

Tragge changes are roadmap-driven and evidence-driven. Before changing files,
follow the [canonical Codex execution protocol](docs/codex/CODEX_EXECUTION_PROTOCOL.md),
even when the work is performed manually rather than by Codex.

## Authoritative sources

Read the sources that govern the change:

1. [Fixed Product and Technical Policies](docs/product/FIXED_PRODUCT_AND_TECHNICAL_POLICIES.md)
2. [Production Roadmap and Codex Tasks](docs/codex/PRODUCTION_ROADMAP_AND_CODEX_TASKS.md)
3. Applicable Accepted ADRs, beginning with
   [ADR-0001](docs/adr/0001-target-runtime-architecture.md)
4. [Canonical domain glossary and version catalog](docs/product/canonical-domain-glossary-and-version-catalog.md)
5. [Codex execution protocol](docs/codex/CODEX_EXECUTION_PROTOCOL.md)

The protocol defines precedence and conflict handling. Do not create a competing
process rule in a task, issue, or pull request.

## One roadmap task per change

Use the [roadmap task template](docs/codex/templates/ROADMAP_TASK_TEMPLATE.md).
Keep one task, one goal, and one scoped change set. Do not mix unrelated cleanup,
future work, broad refactors, policy changes, architecture changes, or dependency
upgrades. An ADR or dependency approval may be required before implementation.

## Execution modes

- In **local mode**, Git metadata is optional. Do not initialize Git or contact a
  remote without explicit authorization. Write
  `docs/codex/reports/<TASK-ID>-local-execution-report.md` and prove completion
  from artifacts, tests, regressions, acceptance evidence, and file scope.
- In **Git-backed mode**, work from the verified protected base on one task
  branch, use Conventional Commits, open a reviewable pull request, pass required
  CI, resolve review, and merge through protected `main`. Direct production or
  protected-branch changes are prohibited.

Local completion is not merge evidence. Preserve local reports and validation
when importing work into a Git-backed repository.

## Testing and evidence

Use the protocol's [testing and evidence rules](docs/codex/CODEX_EXECUTION_PROTOCOL.md#testing-and-evidence-rules).
Behavior changes require proportional unit, integration, E2E, regression, lint,
typecheck, and build checks. Documentation-only changes require focused
structure, terminology, path, policy, and link checks rather than artificial
application tests. Every report and pull request records exact commands and
results; unavailable checks remain explicit limitations.

At each Epic or Phase end, run the complete relevant suites, migrations,
critical E2E journeys, and coverage/evidence required by the gate. Numeric
coverage alone does not prove correctness. Phase reports live under
`docs/codex/reports/` and must state `PASS` or `FAIL`.

## Pull requests and rollback

Use the repository pull-request template. Push and merge only after dependencies,
acceptance criteria, required tests, documentation, secret checks, rollback
analysis, reviews, and CI satisfy the protocol. Squash merge is the default for
a single-task branch. Revert merged code through a reviewed `revert` or the
approved data/contract recovery plan; do not rewrite protected history.

## Security reporting

Do not put secrets, credentials, tokens, private keys, unredacted KYC/payment
data, or exploit details in a public issue. Stop drafting the public report and
use the repository owner's approved private security-reporting channel. If no
private channel is configured, ask the repository owner to establish one without
sharing the sensitive detail publicly. The
[security-sensitive issue guidance](.github/ISSUE_TEMPLATE/security-sensitive.md)
contains the public-safe triage rule.

Codex cannot independently approve legal requirements, Market Data rights,
financial launch risk, staged launch, or paid-production deployment. Paid
production remains `NO-GO` until the approved launch process changes it.
