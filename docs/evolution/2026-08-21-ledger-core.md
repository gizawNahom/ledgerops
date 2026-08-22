# Evolution — ledger-core

**Status: PARTIALLY DELIVERED BY DECISION. This feature is NOT complete.**
Every claim below about what shipped is scoped to slices 01-03 plus the
platform layer. Slices 04 (proof of balance) and 05 (entry traceability),
and the console SPA, were deliberately excluded from this DELIVER run and
remain unbuilt. Read the whole "What did NOT ship" section before treating
any part of this document as a completion record.

---

## Feature summary and business context

**JTBD**: move value between accounts so that it can never be created or
destroyed by failure, retry, or contention — and prove it.

**Five user stories**, each tied to a job and an invariant:

| Story | Job | Invariant | Shipped this run |
|---|---|---|---|
| US-1 — Post a transfer | J1 | I1 (entries sum to zero per currency) | YES — slice 01 |
| US-2 — Reject insufficient funds | J3 | I4 (no negative wallet) | YES — slice 02 |
| US-3 — Retry safely | J2 | I7 (retry applies once) | YES — slice 03 |
| US-4 — Prove the books balance | J4 | I3 (stored balance matches entries) | **NO** — slice 04 out of scope |
| US-5 — Trace a balance to its entries | J5 | none (supports I3 investigation) | **NO** — slice 05 out of scope |

Personas: P1 (integrating developer, archetype, unresearched — DoR item 2
waived) and P2 (platform operator, real named person, sole consumer at this
stage). Both DoR waivers (persona depth, real-data examples) are recorded in
`feature-delta.md` § DoR Waiver and remain in force; neither survives contact
with a third-party integrator per D8's same trigger.

Three of five stories reached production-shaped code with full acceptance
coverage. Two did not exist as code at all when this run ended — their
scenarios remain `@pending` and their driving ports answer `501 __SCAFFOLD__`.

---

## Key decisions

### DISCUSS wave

- **D1-D9** (`feature-delta.md` § Locked decisions) established: cross-cutting
  fullstack scope, walking skeleton as slice 01, comprehensive UX depth, JTBD
  mandatory, five-slice split, synthetic data with PBT mitigation, append-only
  entries enforced at the database level, single-tenant deferred, full-scan
  verification deferred checkpointing.
- **DoR waiver** (2026-08-18, recorded retroactively): items 2 (persona depth)
  and 3 (real data) waived on the strength of D6's PBT mitigation and P2's
  real accountability. The waiver explicitly does not survive a third-party
  integrator.

### DESIGN wave

- **DDD-16** (supersedes DDD-10, reversed 2026-08-18): paradigm is functional,
  not OOP — pure domain core, effect shell, immutable types. Reversal driven
  by the fact that DDD-1 already specified a pure core with `Clock`/
  `IDGenerator` as ports; the OOP label was a language-default artifact, not
  a considered choice. Routed implementation to
  `@nw-functional-software-crafter`.
- **DDD-17..21** (`adr-008`, `adr-009`, added 2026-08-19): closed three
  `SPECIFICATION_AMBIGUITY` gaps DISTILL's completeness audit surfaced — the
  refusal taxonomy has three decision sites sharing one wire vocabulary
  (DDD-17), duplicate account-open is refused `409` not replayed (DDD-18),
  malformed payloads split at the purity boundary (DDD-19), store
  unavailability is an outcome, not a refusal — `503`, never a trial-balance
  `NO` (DDD-20, DDD-21).
- **OPS-10** (Changed Assumption, 2026-08-18): "revoked privileges" made
  concrete as a two-database-role split (`ledgerops_app` vs
  `ledgerops_migrate`) — a trigger alone is not database-level enforcement if
  the same role can disable it.

### DELIVER-wave scope decisions (recorded outside the repo, in project
memory, at the DELIVER Phase-1 gate — not in any wave artifact until this
evolution document)

1. **R-2 closed in DELIVER** (step 05-04) — no HTTP status code had ever been
   asserted anywhere in the acceptance suite, a HIGH reviewer condition left
   open at DISTILL by user decision. Closed by adding a `statusFor
   (RefusalKind) int` table transcribed from ADR-008 and folding status
   assertions into the existing refusal/accept/replay checks — zero new
   `.feature` changes, zero new step decorators.
2. **The DEVOPS platform layer, never built by DEVOPS, was built in DELIVER**
   (steps 05-01..05-03) — Docker Compose, Makefile demo/chaos/race targets,
   the `exhaustive` linter over both violation-kind switches, and the CI/
   nightly GitHub Actions workflows. Rationale: no DEVOPS wave artifact
   existed to build from; DELIVER carried the obligation forward rather than
   leave platform readiness unaddressed.
3. **Scope is walking skeleton + slices 01-03 only.** Slices 04
   (proof-of-balance) and 05 (entry-traceability), and the console SPA, are
   **OUT**. Their acceptance scenarios stay `@pending`. This is the decision
   that makes "Wave Completion Enforcement" (all ATs green, nothing
   `@pending`) unsatisfiable by this run — the rule is not being claimed as
   satisfied, because it is not.

---

## Work completed — 18 roadmap steps, grouped by phase

**Phase 01 — Walking skeleton (real HTTP to real store)**
- 01-01: Pure domain core — `Money`, `Account`, `Post`, sealed violation
  taxonomy, property-tested (I1, I4).
- 01-02: Migration 0 — schema, two-role split, append-only privilege
  revocation, unique account-name constraint.
- 01-03: Effect shell — `AccountRepository`/`TransactionRepository`/
  `IdempotencyStore` under one `UnitOfWork`, lock ordering by ascending
  account id.
- 01-04: HTTP adapter + `cmd/api` wiring, startup probe refuses to serve
  unless the app role's `UPDATE` on entries is genuinely refused.

**Phase 02 — Slice 01 (refusals, auth, chaos, integration checkpoints)**
- 02-01: Happy-path completions — open, fund, transfer, balance-sum, smallest
  accepted amount.
- 02-02: Refusal taxonomy — unknown account, duplicate open, bad amounts,
  seven malformed-payload shapes, `currency_mismatch` declared and
  deliberately unreachable through driving ports.
- 02-03: Auth refusals (missing/wrong operator key) and kill-mid-write chaos
  proving atomicity.
- 02-04: Integration checkpoints — append-only tamper resistance, injected
  Clock/IDGenerator, migration-over-history, 50+50 opposing-direction
  deadlock-free contention. Includes a legitimate escalation/resume trace
  (see below).

**Phase 03 — Slice 02 (insufficient funds, system accounts, contention)**
- 03-01: Insufficient-funds refusal states the exact shortfall.
- 03-02: Wallet-to-zero accepted; system-account asymmetry (system accounts
  bypass the I4 floor by design).
- 03-03: I4 under real PostgreSQL concurrency — 20 racers, exactly 1 accepted;
  1000 contended spends, 0 negative-balance observations.

**Phase 04 — Slice 03 (idempotent retry)**
- 04-01: Idempotency key required; replay re-rendered at 200 from the stored
  transaction, never a cached body.
- 04-02: Key reused for a different request is refused `409`, first
  transaction untouched; a refused attempt does not consume its key.
- 04-03: Idempotency under chaos and 50-way contention — exactly one
  transaction survives a mid-write kill-and-retry and 50 simultaneous
  same-key submissions.

**Phase 05 — Platform readiness + R-2**
- 05-01: `docker-compose.yml` + Makefile `demo-01..03`, `chaos-01`,
  `race-02`, `race-03` targets (demo-04/05 and corrupt-04 deliberately out —
  slices 04/05 deferred).
- 05-02: `.golangci.yml` `exhaustive` linter over both violation-kind
  switches (`internal/domain/`, `internal/adapters/http/`).
- 05-03: `.github/workflows/ci.yml` (8 required jobs) and `nightly.yml`
  (mutation-delta, non-blocking), slices 04-05 explicitly excluded from
  `invariant-gates`.
- 05-04: R-2 closed — HTTP status assertions added to the acceptance suite's
  existing refusal/accept/replay vocabulary.

Post-18-steps: L1-L6 refactor pass completed, 49/49 acceptance scenarios
stayed green throughout.

---

## Real bugs found and fixed during DELIVER

This run's TDD discipline surfaced genuine defects, not just activation
theater:

1. **Clock/IDGenerator wiring bug** — fixture overrides for deterministic
   timestamps and ids never reached the running HTTP server; found while
   closing integration checkpoints (02-04).
2. **UTC-vs-local timezone encoding bug** — timestamp JSON serialization
   encoded in local time instead of UTC, found and fixed in the same step
   (02-04, commit `cec5e39`).
3. **ID-fixture collision** — a test harness's default id sequence collided
   with a hand-picked literal used elsewhere in the suite (commit `4c4ad17`).
4. **Concurrent idempotency-claim race** — under 50-way contention on the
   same idempotency key, the system surfaced raw `500`s instead of resolving
   every submission to a replay of the single winning transaction; fixed in
   04-03.

---

## Escalation pattern worth recording

Step 02-04's GREEN phase found that all 8 reactivated integration-checkpoint
scenarios failed identically at their `Given` step — the
`integration-checkpoints.feature` Background omitted "a system account
treasury exists", present in every sibling feature's Background. This was a
test-harness/support-code defect in `tests/acceptance/ledgercore/`, outside
the implementing crafter's `files_to_modify` scope (which held only
production postgres adapter files, inspected and found defect-free). Rather
than silently expand scope or weaken the assertion, the crafter escalated to
`nw-acceptance-designer`, recorded the blocker as `SKIPPED:
BLOCKED_BY_DEPENDENCY` in the execution log, and re-dispatched once the
Background fix landed (commit `085794d`). The execution log's SKIPPED-then-
EXECUTED sequence for 02-04 is this legitimate resume trace, not a defect —
confirmed by `des-verify-integrity`, which reports all 18 steps with
complete DES traces.

This discipline — escalate rather than silently expand scope or weaken
assertions when a gap is found outside the assigned `files_to_modify` — held
throughout the run and is worth naming as a pattern that worked.

---

## What did NOT ship, stated clearly

**Slice 04 — prove the books balance (US-4, invariant I3).** Not built.
`GET /health/trial-balance` remains `501 __SCAFFOLD__`. All 11
`milestone-04-proof-of-balance.feature` scenarios remain `@pending`,
including the corruption-detection demo (`make corrupt-04`, KPI-4) and the
console verdict contract.

**Slice 05 — entry traceability (US-5).** Not built. `/console/verdict`
remains `501 __SCAFFOLD__`. All 10 `milestone-05-entry-traceability.feature`
scenarios remain `@pending`. A partial `GET /accounts/{id}/entries` exists —
added incidentally at step 02-01, driven by a slice-01 acceptance test's own
persistence-verification need — but it does not implement a running balance
or any part of the full slice-05 contract (deterministic tie-breaking,
counterparty naming, console drill-down).

**Console SPA** (`web/console/`, DDD-4 / ADR-006). Never created. No
TypeScript SPA exists anywhere in the repository.

**KPI-1 and KPI-4** (trial balance integrity, corruption detection) have no
live measurement — both depend on slice 04. KPI-6 (slice cycle time) is
reported, not gated, per design. KPI-2 and KPI-3 (negative balances,
idempotent posting) are live and green — see Demo Evidence in
`feature-delta.md`.

The Wave Completion Enforcement rule — all ATs green, nothing `@pending` —
**cannot be satisfied by this run and is not reported as satisfied.** 49 of
70 total scenario blocks are green; 21 (all of milestone-04 and
milestone-05) remain `@pending` by the scope decision above, not by failure.

---

## Migrated permanent artifacts

- `docs/architecture/ledger-core/slices/slice-01-post-a-transfer.md`
- `docs/architecture/ledger-core/slices/slice-02-sufficient-funds.md`
- `docs/architecture/ledger-core/slices/slice-03-idempotent-retry.md`
- `docs/architecture/ledger-core/slices/slice-04-proof-of-balance.md`
- `docs/architecture/ledger-core/slices/slice-05-entry-traceability.md`

All five slice briefs migrated from `docs/feature/ledger-core/slices/` —
including the two unbuilt ones (04, 05), which are exactly what a future
continuation of this feature needs as its specification.

ADRs already live at `docs/product/architecture/adr-00{1..9}.md` — verified
present and current, not moved.

`docs/product/architecture/brief.md` § Functional modeling decisions updated:
the `exhaustive`-linter compensating control (DDD-12/DDD-17) status corrected
from "OUTSTANDING, not discharged" to "DISCHARGED, verified 2026-08-22" —
`.golangci.yml` and `.github/workflows/ci.yml` now exist and were verified
present, matching steps 05-02/05-03. No slice-04/05 design content was
touched; there was none marked as implemented to begin with.

---

## Left in the workspace, and why

- `docs/feature/ledger-core/feature-delta.md` — stays. Lean-format
  consolidated wave record; this evolution document summarizes it, does not
  replace it.
- `docs/feature/ledger-core/deliver/roadmap.json` and `execution-log.json` —
  stay. Process scaffolding captured above; not deleted (deletion is Phase C,
  not run here).
- `docs/feature/ledger-core/devops/environments.yaml` — stays; still the
  environment matrix DISTILL parametrized scenarios over, still current for
  slices 01-03 and still needed verbatim if slices 04/05 resume.
- `docs/feature/ledger-core/distill/red-classification.md` — **judgment:
  pure process scaffolding, not migrated.** It is a point-in-time
  pre-DELIVER RED-gate record (2026-08-19, 53/54 scenarios) whose value was
  consumed at gate-time; current suite state (49/49 green) is already the
  authoritative record in `feature-delta.md` § DELIVER Demo Evidence. Left
  in place as historical trace, a Phase-C discard candidate.
- `docs/feature/ledger-core/distill/upstream-issues.md` — **judgment:
  lasting decision-rationale value, recommend NOT discarding, but not
  migrated by this pass** (Phase B scope was the slice briefs; this file's
  disposition needs the orchestrator's call since it is a closer case).
  It holds the full reasoning behind three user rulings (DDR-1 idempotency
  key from slice 01, DDR-2 console over HTTP not browser, DDR-3 replay
  answers 200) and two closed findings (U-1, U-2) that are only summarized,
  not fully explained, in `feature-delta.md`. **One correction owed**: this
  file's own R-2 entry still reads "Status: OPEN — deferred by user
  decision, not dropped," but R-2 was closed in this DELIVER run at step
  05-04 (commit `5d3d3ac`). The file is stale on its own headline claim.
  Since editing wave artifacts is not this task's mandate, the staleness is
  reported here rather than silently corrected.

---

## Lessons learned (retrospective)

- **Escalation discipline over scope creep**: naming a gap and stopping at
  the boundary of `files_to_modify`, rather than quietly fixing it, kept the
  crafter's contract legible and made the fix traceable to its own commit
  (`085794d`) instead of being buried inside an unrelated step.
- **Contract-shape tagging catches drift mechanically**: `@contract-shape:`
  tags gave every scenario a machine-checkable universe declaration; R-1
  (untagged scenarios) was caught by a reviewer specifically because the tag
  was absent and checkable, not because someone happened to notice.
- **Vacuous-pass discipline held**: DISTILL flagged "the schema builds from
  nothing" as a vacuous pass (`MigrationStatements()` returning empty) and
  told DELIVER not to count it; migration 0 (step 01-02) is the step that
  made it real, and nothing in the roadmap or KPI accounting counted the
  earlier green.
- **Scope decisions taken outside wave artifacts are a process risk**: the
  three DELIVER-wave scope decisions that shaped this entire run were made
  at a Phase-1 gate and recorded only in project memory, not in
  `feature-delta.md`. This evolution document is the first place they are
  written into the repository. Future features should record scope-narrowing
  decisions in-repo at the moment they are made, not reconstruct them at
  finalize time.

---

## KPI status at close

| KPI | Target | Status |
|---|---|---|
| Trial balance integrity | 100% of CI runs zero | **Not measurable — slice 04 unbuilt** |
| Negative wallet balances | 0 over 1000 iterations | **PASS** — `iterations=1000`, `negative_balance_observations=0` |
| Idempotent posting | exactly 1 txn from 50 submissions | **PASS** — `submissions=50`, `distinct_transaction_ids=1` |
| Corruption detection | 100% of injected corruptions caught | **Not measurable — slice 04 unbuilt** |
| Time to first demo | < 5 min | Reported by `demo` CI job (`demo_first_green_seconds`), not independently re-verified in this document |
| Slice cycle time | ≤ 1 day per slice | Reported (KPI-6), ungated by design |

---

## Handoff to operations

Production-validated surface: `POST /accounts`, `POST /transfers`,
`GET /accounts/{id}`, partial `GET /accounts/{id}/entries` (slice-01 shape
only). No hosted environment exists yet (OPS-1) — Docker Compose is the
entire deployment target. Rollback contract: Recreate, redeploy previous
image tag, migrations expand-only (`feature-delta.md` § Deployment
strategy). Observability: `slog` JSON logs + Prometheus exposition at
`/metrics`, nothing scraping yet.

**Before this feature is called complete**: slice 04, slice 05, and the
console SPA need their own DELIVER pass against the slice briefs now at
`docs/architecture/ledger-core/slices/slice-04-*.md` and `slice-05-*.md`,
which already carry full acceptance criteria and dependency notes.

---

## Retrospective — two recurring patterns worth naming

This run was not clean: several steps required escalation or re-dispatch
before landing. Neither pattern below is a defect in the delivered code —
both are process observations worth carrying into the next DELIVER run on
this project.

**Pattern 1 — test-harness gaps kept surfacing outside the implementing
crafter's file scope.** Steps 02-03, 02-04 (three times), and 04-03 each
found a genuine bug in `tests/acceptance/ledgercore/` support code
(`ledger_world.go`, `ledger_observations.go`, `ledger_assertions.go`,
`ledger_seeding.go`) while implementing production code against it — a
missing `Background` fixture line, a Clock/IDGenerator wiring bug, an
id-fixture collision, an authentication read-back bug, an assertion helper
that only worked for one scenario shape. Five Whys: the crafter found the
bug → because the acceptance test failed for a reason unrelated to the
production code it was implementing → because the test harness was authored
once, upfront, during DISTILL, before any of these scenarios had ever
actually executed against real production code → because DISTILL activates
scenarios by removing `@pending` one at a time and most of the suite's Given
clauses never reached their Then clause until DELIVER exercised them for
real (recorded in `feature-delta.md`'s own DISTILL section: "5 of 53 reach
their Then... the other 48 stop in a Given") → because Mandate 1 (seed
through real driving ports, not backdoors) makes early scenarios structurally
unable to prove later scenarios' harness code correct. This is arguably
inherent to one-scenario-at-a-time DELIVER discipline, not a fixable defect
in either wave's process — but the ESCALATION discipline (crafter declines
to touch harness code outside its `files_to_modify`, routes to
`nw-acceptance-designer` instead of silently patching or weakening an
assertion) held every time and is worth keeping as a hard rule, not
softening it to let crafters "just fix the test."

**Pattern 2 — several dispatched agents hit API timeouts or connection
drops mid-task**, most often after completing the real work but before
logging DES phases or committing. In each case the orchestrator verified the
actual repository state independently (build, full test suite, git log)
before either finishing the commit/logging itself or re-dispatching a
narrower continuation, rather than trusting the interrupted agent's partial
self-report. Five Whys: the agent got cut off → because some steps in this
run (02-04 especially) required 5+ rounds of investigation, fix, and
re-verification, pushing individual dispatches past what a single API call
window reliably completes → because the underlying bugs were genuinely
subtle (a timezone-rendering bug, a fixture-collision bug) and took real
back-and-forth to isolate → no deeper cause found; this reads as ordinary
variance in task size versus timeout budget, not a process defect. The
mitigation that worked: never trust a report the harness flags as
early-terminated without independently re-verifying git state and test
results before proceeding.
