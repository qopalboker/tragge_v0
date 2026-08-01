# Codex prompts

The [Codex execution protocol](../CODEX_EXECUTION_PROTOCOL.md) is the canonical
process authority. These files are reusable invocation controllers: they choose
a phase task or a gate; they do not redefine execution modes, evidence rules,
Git authorization, or production authority.

## How to use a controller

1. Select local or Git-backed mode from explicit authorization under the
   protocol. Git-specific wording in a controller is conditional on Git-backed
   mode; it is not permission to initialize Git or contact a remote.
2. Read the fixed policy, roadmap's complete selected task block, applicable
   Accepted ADRs, glossary, protocol, and direct dependency evidence.
3. Inspect repository evidence and select the first incomplete task whose
   dependencies are satisfied, unless the invocation explicitly selects that
   eligible task.
4. Implement exactly one task, create its mode-appropriate report, and stop.
5. After every phase task is complete, use a separate gate-only invocation.
   Never start the next phase automatically.
6. On a failed gate, use the remediation controller. Do not weaken the gate.

Local completion is proven by artifacts, acceptance evidence, focused tests,
completed-task regressions, later-task boundaries, and deterministic changed-file
scope. A local report alone is not proof and is not Git merge evidence.

## Controller index

- [Bootstrap inventory](00_BOOTSTRAP.md) ? read-only initial orientation.
- [Phase 0 ? Baseline](01_PHASE_0_BASELINE.md)
- [Phase 1 ? Security](02_PHASE_1_SECURITY.md)
- [Phase 2 ? Architecture](03_PHASE_2_ARCHITECTURE.md)
- [Phase 3 ? Data and Money](04_PHASE_3_DATA_MONEY.md)
- [Phase 4 ? Contest and Scheduler](05_PHASE_4_CONTEST_SCHEDULER.md)
- [Phase 5 ? Prize and Settlement](06_PHASE_5_PRIZE_SETTLEMENT.md)
- [Phase 6 ? Trading Engine](07_PHASE_6_TRADING_ENGINE.md)
- [Phase 7 ? Market Data](08_PHASE_7_MARKET_DATA.md)
- [Phase 8 ? Payments and KYC](09_PHASE_8_PAYMENTS_KYC.md)
- [Phase 9 ? Frontends](10_PHASE_9_FRONTENDS.md)
- [Phase 10 ? Production Engineering](11_PHASE_10_PRODUCTION_ENGINEERING.md)
- [Phase 11 ? Launch Qualification](12_PHASE_11_LAUNCH_QUALIFICATION.md)
- [Failed-gate remediation](13_FAILED_GATE_REMEDIATION.md)

## Source-of-truth rule

When controller process wording conflicts with the protocol, the protocol wins.
When a substantive source conflicts, use the precedence in the protocol and do
not silently change product policy. An explicit invocation may alter delivery
mechanics, such as selecting local mode, but cannot waive tests, evidence,
acceptance criteria, safety, or protected-production restrictions.
