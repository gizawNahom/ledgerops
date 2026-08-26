# Evolution — ledger-core-console

**Status: DELIVERED (unit/component-level), live-browser dogfood deferred.**
All 4 stories (US-1..US-4) implemented and unit-tested. Two real integration
gaps found and closed at the Post-Merge Integration Gate. Live-browser/
live-Postgres manual dogfood (DoD items 2 and 4) was NOT run this session —
open item, not papered over.

This feature is the `web/console/` TypeScript SPA that `ledger-core`'s own
DELIVER run explicitly scoped out (DDR-2,
`docs/feature/ledger-core/distill/upstream-issues.md`), closing the one
remaining piece of that feature's original scope
(`docs/evolution/2026-08-21-ledger-core.md` § "What is still deferred").

---

## Feature summary and business context

**JTBD**: let the operator (P2) answer "does this add up?" from a browser,
in words, before any figures — and if not, find and explain the drift
without touching a terminal. Bridges directly to jobs J4/J5
(`docs/product/jobs.yaml`), no new job introduced.

**Four user stories**, all Must Have, shipped as one release:

| Story | Job | Status |
|---|---|---|
| US-1 — Console states the verdict | J4 | YES — walking skeleton + Feature-0 WS |
| US-2 — Console names the drifted accounts | J4 | YES |
| US-3 — Console traces a drifted account to its entries | J5 | YES |
| US-4 — Console failure does not strand the operator | J4 | YES |

Persona: P2 (platform operator), real named project owner, sole console
user. DoR passed 9/9 with no waiver — the first feature in this project to
clear items 2 (persona depth) and 3 (real data) without one, since real
dogfood data (`alice-demo`, `bob`, `treasury-demo`) already existed from
`ledger-core`'s own delivery.

No backend code changes were introduced by this feature's DELIVER pass
(zero `internal/domain` or `internal/adapters` changes); the static-file-
serving route (`internal/adapters/http/console_static.go`, router
auth-scoping) was built in the prior DEVOPS-wave session, not this one.

---

## Key decisions

### DISCUSS wave
- Walking Skeleton (D2, corrected): evaluated explicitly rather than
  defaulted to "No" — slice 01/US-1 serves double duty as both the
  story-map WS and this feature's Feature-0 WS; no separate WS slice added.
- UX research depth (D3, corrected): comprehensive Phase 2 redo, not a
  formalization of the existing `verify-the-books.yaml` journey — surfaced
  two new console-specific states (loading/freshness on US-1, a second
  failure branch on the entries fetch on US-3) that fed back into both
  stories' AC. Migrated: `docs/ux/ledger-core-console/journey-console.yaml`,
  `docs/ux/ledger-core-console/journey-console-visual.md`.
- Search/filter over accounts or entries (D9): evaluated against the
  Elephant Carpaccio taste tests and explicitly **deferred**, not silently
  omitted — materially redundant with US-2 and retires no risky assumption.
- Auth/authz (D10): confirmed out of scope; a companion feature
  (`operator-authentication`, tentative name) is required before real login
  is possible.

### DESIGN wave (three-architect Full-stack sequence)
- System-level (`nw-system-designer`): confirmed — no new deployment shape,
  no new infrastructure component. `brief.md`'s deployment shape ("one Go
  binary + static bundle + PostgreSQL") already anticipated this SPA.
- Domain-level (`nw-ddd-architect`): confirmed — no new bounded context,
  aggregate, or invariant. One glossary backfill only: "Verdict" added to
  the Ubiquitous Language table, sourced from `ledger-core`'s own DDD-21
  ("the verdict withholds; it never accuses"), not newly invented here.
- Application-level (`nw-solution-architect`): React 18 + Vite 5 (CRA
  rejected as deprecated; Next.js/Parcel rejected as unnecessary
  complexity). Client-side paste/`localStorage` API key delivery via a
  sole-effectful `apiClient`/`keyStorage` pair (ADR-010,
  `docs/product/architecture/adr-010-console-client-side-api-key.md`,
  already at its permanent location — not moved by this finalize). 8-second
  fixed `AbortController` timeout on every fetch (SA-D6).

### DEVOPS wave
- All 9 DEVOPS decision points inherited from `ledger-core`'s own DEVOPS
  wave rather than re-run, per the user's explicit choice.
- One new CI job (`console`) added to `.github/workflows/ci.yml`,
  probe-guarded against `web/console/` not yet existing at the time DEVOPS
  ran.
- Static-file-serving route resolved as a DEVOPS/infra concern, not a fifth
  console story — built ahead of this DELIVER session.
- `nightly-delta` mutation testing reconfirmed project-wide; a genuine
  TypeScript-side coverage gap was flagged (no vendored TS mutation tool,
  no file-glob branch in `nightly.yml`'s `mutation-delta` job) and remains
  open, not silently assumed covered.

### DISTILL wave
- No new Gherkin `.feature` file (DDR-2 held) — DISTILL produced 8 Vitest
  component/unit-test scaffolds instead, one per DESIGN-named unit.
- Self-Completeness Audit: 11/15, `ACCEPTABLE_WITH_DOCUMENTED_GAPS` — 4
  gaps (C4a, C4b, C6a, C6c), all `AT_GAP_IN_DELIVERY_SCOPE`, zero
  `SPECIFICATION_AMBIGUITY` blockers. All 4 closed during DELIVER.
- Final Wave Review Gate (4 parallel Haiku reviewers, one per wave): zero
  blockers, zero cross-wave contradictions. DISCUSS/DESIGN approved,
  DEVOPS/DISTILL conditionally approved (all conditions resolved in
  DELIVER — see Findings list in `feature-delta.md` § Final Wave Review
  Gate).

---

## Work completed (DELIVER)

8 DISTILL-scaffolded units GREENed via the 3-phase RED→GREEN→COMMIT canon,
across 6 roadmap steps (01-01 `keyStorage`, 01-02 `apiClient`, 01-03
`ConsoleApp`/`ApiKeyPrompt`/`VerdictBanner`, 02-01 `DriftTable`, 03-01
`EntryTrace`, 04-01 `VerdictFetchError`), plus all 4 documented
Self-Completeness gaps (C4a, C4b, C6a, C6c) closed as part of their owning
unit's step. 39/39 Vitest test cases green, `tsc --noEmit` 0 errors, Go side
(`go build`/`go vet`/`go test -run Console`) unchanged and passing.

**Two real gaps found only at the Post-Merge Integration Gate**, not by any
earlier wave or by any per-step TDD cycle, because each unit's own tests
passed in isolation and neither gap had a DISTILL-authored scaffold
covering it:

1. **`ConsoleApp` never wired `DriftTable`/`EntryTrace`/`VerdictFetchError`,
   and collapsed `AuthRejectedError`/`FetchFailedError` into one state** — a
   Tested-But-Unwired (TBU) defect invisible to component-level tests: a
   network failure would have incorrectly shown the key-entry prompt
   instead of `VerdictFetchError`. Step 01-03's roadmap criteria only
   specified the US-1 key-gate/verdict scenarios, not the full orchestrator
   responsibility DESIGN's own component table assigned `ConsoleApp`.
   Closed via orchestrator-added step 01-04.
2. **No Vite app entry point existed at all** (`index.html`/`src/main.tsx`)
   — `vite build` failed outright. DISTILL's 8 scaffolds cover every
   component/module DESIGN named, but DESIGN's component table does not
   separately name the composition-root file every Vite/React app needs.
   Closed via orchestrator-added step 01-05.

Both were additive, in-scope fixes consistent with what DESIGN already
specified — neither reopened DISCUSS/DESIGN/DEVOPS/DISTILL decisions.

**Adversarial review** (Phase 4, Haiku reviewer): returned
`NEEDS_REVISION` on one mechanical finding (39 tests vs. a 36-test budget
heuristic), while explicitly confirming zero testing-theater/correctness
issues. One revision pass consolidated 3 test cases (no coverage lost) to
land exactly at 36/36.

**Refactor** (L1-L6): centralized RTL cleanup into `setupTests.ts`.

**Mutation testing**: skipped this session, per project-wide `nightly-delta`
strategy (`CLAUDE.md`) — not run during feature delivery by design.

**Roadmap review** (Phase 1) returned one non-blocking finding (step 01-03
missing an oversized-step `@sizing-review-needed` tag for its 13
scenarios); the user explicitly chose to skip it rather than fix it, since
the reviewer itself called it non-blocking.

---

## What did NOT ship / open items, stated clearly

- **Live-browser/live-Postgres manual dogfood demo (DoD items 2 and 4) was
  NOT run this session.** This sandbox has no running Postgres —
  `go run ./cmd/api` fails at startup (`health.startup.refused`) before
  binding a port. What WAS verified directly: `npm run build` (exit 0,
  produces `web/console/dist/`), and `go test ./internal/adapters/http/...
  -run Console` (4/4 sub-tests PASS) against the real `dist/` this session
  produced. This is a genuine open item for whoever next has a live
  environment, not silently marked done.
- **Dockerfile's final stage does not yet copy `web/console/dist/`** — the
  compose-based deployment path will not actually serve the console until
  the image build also copies the built bundle in (flagged during DEVOPS,
  still open).
- **TypeScript-side mutation-testing coverage gap** — no vendored tool
  (e.g. Stryker), no file-glob branch in `nightly.yml`'s `mutation-delta`
  job for `*.ts`/`*.tsx`. `nightly-delta` is declared project-wide but not
  yet mechanically true for the TS code.
- **DoD item 9** (learning hypothesis confirmed/disproved per slice) was
  not re-visited this DELIVER session — tracked as open, not silently
  dropped.

---

## Lessons learned

- **TBU (Tested-But-Unwired) defects are structurally invisible to
  component-level unit tests.** `ConsoleApp` passed every DISTILL-scaffolded
  test while never actually rendering 3 of its 6 sibling components — each
  sibling's own tests were green in isolation, and nothing exercised the
  orchestrator's wiring responsibility until a Post-Merge Integration Gate
  step tried to use the whole app together. Worth generalizing: any
  DESIGN component table that assigns one component the "decides which of
  the others to render" responsibility should get an explicit
  integration-shaped test scaffold at DISTILL time, not just individual
  prop→render scaffolds per sibling.
- **DESIGN component tables can miss the composition root.** No file
  covers "the file that actually calls `ReactDOM.createRoot(...).render()`"
  in a component decomposition organized around business-meaningful units
  (`ConsoleApp`, `ApiKeyPrompt`, etc.) — it's implicit infrastructure every
  Vite/React app needs, not a DESIGN omission per se, but it is exactly the
  kind of gap invisible until someone actually runs `vite build`. Worth a
  standing DISTILL/DEVOPS checklist item: "does a runnable entry point
  exist for this toolchain," independent of the business-component roster.
- **Sandbox environment limits are a recurring, not one-off, constraint on
  this project's manual-dogfood DoD items.** Both this feature and its
  parent (`ledger-core`, first DELIVER pass) have had sessions where no
  live Postgres was available to complete the manual verification steps
  DDR-2 requires. This is now the second feature to close DELIVER with
  that gap explicitly flagged rather than silently skipped — worth
  deciding, at the project level, whether manual-dogfood DoD items should
  be split into "unit-verifiable now" vs. "requires a live-environment
  session" so a DELIVER pass can be scoped honestly against sandbox
  constraints from the start, rather than discovering the gap at the end
  each time.
- **Escalation-and-fix discipline held again** (consistent with the parent
  feature's own retrospective patterns): both Post-Merge Integration Gate
  gaps were named as orchestrator-added roadmap steps with an explicit
  deviation-from-DESIGN note ("none — DESIGN's component decomposition
  already specified this wiring, it was just under-scoped in the DELIVER
  roadmap's step 01-03"), not silently patched into an unrelated step.

---

## Migrated permanent artifacts

- `docs/architecture/ledger-core-console/slices/slice-01-console-states-the-verdict.md`
- `docs/architecture/ledger-core-console/slices/slice-02-console-names-the-drifted-accounts.md`
- `docs/architecture/ledger-core-console/slices/slice-03-console-traces-a-drifted-account.md`
- `docs/architecture/ledger-core-console/slices/slice-04-console-failure-does-not-strand-the-operator.md`
- `docs/ux/ledger-core-console/journey-console.yaml`
- `docs/ux/ledger-core-console/journey-console-visual.md`

All migrated from `docs/feature/ledger-core-console/slices/` and
`docs/feature/ledger-core-console/discuss/`, matching the `ledger-core`
finalize precedent (`docs/evolution/2026-08-21-ledger-core.md` § Migrated
permanent artifacts).

**Not migrated — already at permanent location, not moved**:
`docs/product/architecture/adr-010-console-client-side-api-key.md` (this
project's ADR convention places ADRs at `docs/product/architecture/`, not
`docs/adrs/`). `docs/product/architecture/brief.md` § "Console SPA
(`ledger-core-console`, confirmed 2026-08-25)" was already updated during
the DESIGN wave itself — no back-propagation needed at finalize, and no
"FUTURE DESIGN" label exists there to flip to "IMPLEMENTED" (checked by
grep — none found).

**Not migrated — no separate design docs to migrate.** This feature used
lean v3.14 single-file `feature-delta.md` narrative throughout DESIGN; there
is no `design/architecture-design.md`, `design/component-boundaries.md`,
etc. to migrate — that content lives inline in `feature-delta.md` under
`## Wave: DESIGN / ...` headings and was already cross-referenced into
`brief.md` during DESIGN itself.

**Discarded (per skill's discard table, not migrated)**:
`deliver/roadmap.json`, `deliver/execution-log.json`,
`deliver/.develop-progress.json`, `discuss/shared-artifacts-registry.md` —
all process scaffolding, captured above or in `feature-delta.md`.

**Left in the workspace, judgment call, not migrated and not discarded**
(matching the `ledger-core` precedent for the same file types):
- `distill/red-classification.md` — a point-in-time pre-DELIVER RED-gate
  record (1 walking-skeleton test enabled, 30 skipped, verified genuine
  RED). Value was consumed at gate-time; current suite state (39/39 green,
  later consolidated to 36/36) is the authoritative record in
  `feature-delta.md` § DELIVER. Historical trace, Phase-C discard
  candidate.
- `environments.yaml` — still the current environment matrix DISTILL
  parametrized scenarios over (`with-stored-key`, `without-stored-key`,
  `api-unreachable`); still needed verbatim by anyone extending this
  feature.

---

## KPI status at close

| KPI | Target | Status |
|---|---|---|
| KPI 1 (North Star) — books-balance check without a terminal | 100% of routine checks, 0 curl calls | **Unit-verified, not dogfood-confirmed** — component behavior matches AC; manual dogfood observation deferred (no live Postgres this session) |
| KPI 2 (Leading) — drift explanation without a terminal | 100% of corruption incidents, 0 manual curl calls | **Unit-verified, not dogfood-confirmed** — same sandbox limitation |
| KPI 3 (Guardrail) — console verdict agrees with `GET /health/trial-balance` | 100% agreement | **Structurally held** — both share `verdictHandler` (`router.go`); no client-side reconciliation logic exists to diverge (US-3 AC: "no client-side balance recomputation anywhere," verified in `DriftTable.test.tsx`/`EntryTrace.test.tsx`) |

None of the 3 KPIs have a telemetry/dashboard component by design — DISCUSS
scoped all three as manual dogfood observation / self-report, consistent
with DEVOPS's decision not to build instrumentation for a single-operator
tool with no hosted environment.

---

## Handoff to operations

Production-readiness status: unit/component-level complete, live-stack
validation outstanding. Before this feature is called fully complete:

1. Run the manual dogfood demo (DoD items 2, 4) against a freshly built
   `docker compose up -d --build` stack with real Postgres — confirm the
   verdict sentence, drift table, entry trace, and fetch-failure fallback
   all render correctly in an actual browser at `http://localhost:8080/console`.
2. Wire `web/console/dist/` into the `Dockerfile`'s final build stage so the
   compose deployment path actually serves the console.
3. Close the TypeScript mutation-testing coverage gap in `nightly.yml`'s
   `mutation-delta` job (vendor a tool, add a `*.ts`/`*.tsx` file-glob
   branch) if/when TS mutation testing becomes a priority.
4. Confirm the KPI 1/KPI 2 manual dogfood checklist items during the first
   real operator session, per § Monitoring contracts
   (`docs/product/kpi-contracts.yaml`, KPI-C1/C2/C3).

No database migration, no schema change, no rollback concern beyond
redeploying the previous image tag (Recreate strategy, inherited unchanged
from `ledger-core`).
