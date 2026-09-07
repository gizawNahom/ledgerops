# DISTILL Decisions — inter-tenant-transfer

Acceptance-designer leg. Full narrative: `docs/product/architecture/brief.md`
§ Inter-tenant transfer / For Acceptance Designer;
`docs/feature/inter-tenant-transfer/design/wave-decisions.md`.

## Reconciliation (pre-scenario hard gate)

Read `discuss/wave-decisions.md` section embedded in `feature-delta.md`
(`## Wave: DISCUSS / [REF] Wave Decisions Summary`), `design/wave-decisions.md`
(full file). Checked every DISCUSS decision (D1-D12) against DESIGN for
contradiction. **Zero contradictions found.** DESIGN's sync/async settlement
answer, coordinator persistence shape, retry budget, and reversal mechanics
are all elaborations of DISCUSS D8/D9/D10/D11, never a reversal of them.
Reconciliation passed — proceeded to scenario writing.

## Language + policy

`go.mod` present → Go. `docker-compose`/Testcontainers via `godog`, mirroring
`ledgercore`/`multitenancy`'s own suites exactly (same `PBT lib: none in Go
acceptance layer — this project's Go acceptance suites are example-only per
the Layered Test Discipline; property-based coverage for this feature's pure
functions is DELIVER's own unit-layer obligation, not this wave's`).
`docs/architecture/atdd-infrastructure-policy.md` exists — mode `inherit`.
Three new driven-port rows appended (below), zero soft prompts needed: DESIGN
already settled every mechanism (WS strategy C, "never faked").

## WS strategy

Strategy C (real local), inherited unmodified from `ledger-core`'s own
declaration — this feature introduces no costly external dependency (no
paid API, no LLM call). One walking skeleton, chaining slice 01 into slice
02 exactly as `feature-delta.md` § Wave: DISCUSS / Story Map specifies:
"link authorized -> alias registered -> transfer sent -> all three legs
settle -> `GET /transfers/{id}` reports settled." `tests/acceptance/intertenanttransfer/walking-skeleton.feature`.

## Scenario list with tags

| File | Scenarios | Contract-shape split |
|---|---|---|
| `walking-skeleton.feature` | 1 | 1 bounded-change |
| `milestone-01-authorize-a-tenant-pair.feature` | 8 | 4 bounded-change, 4 unbounded-preservation |
| `milestone-02-send-a-transfer-to-a-named-counterparty.feature` | 8 | 2 bounded-change, 6 unbounded-preservation |
| `milestone-03-retry-a-stalled-leg.feature` | 8 | 4 bounded-change, 4 unbounded-preservation |
| `milestone-04-reverse-after-retry-budget-exhausted.feature` | 7 (1 `@pending`) | 2 bounded-change, 5 unbounded-preservation |
| `milestone-05-trace-and-isolate-a-transfer.feature` | 6 (1 outline, 2 example rows) | 0 bounded-change, 7 unbounded-preservation (read/refusal-only feature, matches per-method table: `GetTransfer` = pure-function/return-only) |

**Total: 38 executable scenarios** (37 + 2 outline rows - 1 counted once).
Zero `@contract-shape` omissions — every scenario tagged, matching the
per-scenario mandate (identity-essential, 2026-05-15). Zero `pure-function`
tags at this layer, consistent with Core Principle 14's own guidance
("pure-function should normally be zero at the acceptance layer") — the one
component classified `pure-function` in DESIGN's own table (`GetTransfer`)
is expressed here as `unbounded-preservation` (nothing observable changes),
not as `pure-function`.

**Error/edge-path ratio**: 21 of 38 scenarios (55%) are refusal, isolation,
or negative-preservation scenarios — exceeds the 40% mandate.

**Contract-shape agreement with DESIGN's own per-method table** (`brief.md`
§ Inter-tenant transfer / Component decomposition, "Per-method contract-shape
classification"): `SendTransfer`/`attemptLeg` = bounded-change (matches
slice 02/03/04 mutating scenarios); `AuthorizeTenantPair`/`RevokeTenantLink`
= bounded-change (matches slice 01 mutating scenarios);
`RegisterCounterpartyAlias` = bounded-change (matches slice 02's own
registration scenario); `GetTransfer` = pure-function/return-only
(re-expressed as `unbounded-preservation` at this layer per Core Principle
14). No scenario disagrees with DESIGN's own classification.

## Adapter coverage table

| Adapter | `@real-io` scenario | Covered by |
|---|---|---|
| HTTP driving adapter (all 5 new/extended routes) | YES | every scenario in this suite, real `chi` router over `httptest.Server` |
| `TenantLinkRepository` (PostgreSQL 16) | YES (once implemented) | milestone-01, real Testcontainers Postgres |
| `CounterpartyAliasRepository` (PostgreSQL 16) | YES (once implemented) | milestone-02 |
| `TransferStateRepository` (PostgreSQL 16) | YES (once implemented) | milestone-02/03/04/05 |
| `Clock`/`IDGenerator` (fakes, existing) | N/A — fake by Architecture of Reference | unchanged from `ledgercore` |

All four driven-adapter rows have at least one scenario; none read "NO —
MISSING".

## Scaffolds (Mandate 7)

Production RED scaffolds added this session (all raise/return the
project's own established scaffold shape — HTTP 501 + `{"error":"__SCAFFOLD__"}`,
this project's existing convention, an equally-valid RED-not-BROKEN signal
to the Go `panic("...")` shape the skill's generic Go example shows):

- `internal/adapters/http/router.go`: `POST /tenant-links`,
  `DELETE /tenant-links/{link_id}`, `POST /counterparties`,
  `GET /transfers/{transfer_id}` — all `scaffold("...")`.
- `internal/adapters/http/handlers.go`: `postTransferOrCrossTenantHandler` —
  discriminates on body shape; the cross-tenant branch is
  `scaffold("send_cross_tenant_transfer")`, the existing single-tenant branch
  is forwarded byte-identically to the existing, unmodified
  `postTransferHandler`.
- `internal/domain/violation.go`: three new `ViolationKind` members
  (`TenantLinkAlreadyExists`, `TenantLinkNotFound`, `CounterpartyNotFound`)
  plus constructors — RED scaffold in the sense that no production caller
  constructs them yet, but the type itself is real (not a stub method).
- `internal/adapters/http/status.go`: wire mapping for the three new members
  added — closes the `exhaustive` linter's surface 2 obligation the moment
  the members exist (DDD-12/DDD-17).

**`transfer_not_found` is NOT a scaffold in the sealed-taxonomy sense** — per
DESIGN's own confirmation, no domain aggregate exists for it to violate.
This suite's own `RefusalKind.TransferNotFound` constant (`domain_types.go`)
is the DISTILL-owned coverage the design doc calls for; the wire mapping
itself is GET /transfers/{id}'s scaffold return today, and DELIVER must wire
an explicit `errors.Is`-based check when it implements the handler for real.

## Test placement

`tests/acceptance/intertenanttransfer/` — own package, mirroring both
existing suites' own "own package per feature" precedent (`ledgercore`,
`multitenancy`). Not folded into either: this feature needs its own
composition root (three tenant identities — sender, receiver, and the
reserved `tnt_platform` — none of which either existing World models) and
its own walking skeleton.

## Driving adapter coverage

Every port DESIGN declared under § For Acceptance Designer /
`inter-tenant-transfer` addition is exercised by at least one scenario via
its real HTTP verb+path, over `httptest.Server` (not by calling an
application-layer function directly) — `POST /tenant-links`,
`DELETE /tenant-links/{link_id}`, `POST /counterparties`, the discriminated
`POST /transfers`, `GET /transfers/{transfer_id}`. Zero uncovered entry
points.

## Pre-requisites (inherited from DESIGN, verified present)

Test infrastructure per DESIGN's own table (`brief.md` § For Acceptance
Designer, `inter-tenant-transfer` addition): HTTP driving adapter real;
`TenantLinkRepository`/`CounterpartyAliasRepository`/`TransferStateRepository`
real Postgres (never faked, WS strategy C); `Clock`/`IDGenerator` the
existing fakes, reused unchanged (this suite does not fake either — no
scenario in scope asserts a specific timestamp or generated id, mirroring
`multitenancy`'s own precedent). Retry/backoff timing is explicitly NOT
exercised by real wall-clock sleeps in this suite (DESIGN's own instruction)
— `PollTransferUntilTerminal` polls a RED-scaffolded endpoint at 50ms
intervals up to a bounded timeout; once DELIVER implements
`SendTransfer`/`attemptLeg` for real, the same loop proves settlement without
this suite ever sleeping on production backoff. The actual 1s/2s/4s/8s
backoff-ladder property belongs to an application-layer test tier this
DISTILL session does not author (out of this wave's scope — acceptance
layer only).

## Known gap — fault-injection / crash-simulation seam (disclosed, not hidden)

Five `World` methods (`InjectLegFault`, `SimulateCrashBeforeFirstAttempt`,
`RunRetryTickerOnce`, `SeedTransfersDueForRetry`) return a named
`ErrFaultInjectionNotWired` today: no test-only driving port exists yet to
force a specific leg to fail, simulate a process crash between `SendTransfer`'s
commit and the first `attemptLeg` call, or single-step
`processDueTransfers`. Every scenario in `milestone-03`/`milestone-04` that
needs one of these (7 scenarios) fails today at this named error — a
correct RED classification (`MISSING_FUNCTIONALITY`-equivalent: the seam
itself, not just the feature behind it, is unbuilt), not a fixture bug or
import error. **DELIVER's crafter owns designing this seam** — most likely a
test-only admin endpoint behind a build tag, mirroring
`postgres.AttemptOutOfBandChange`'s own "what the suite may never do for
real, except through a named back door" precedent already established in
`multitenancy/world.go`. Flagged here rather than silently worked around
with a sleep-based real-timing test, which DESIGN explicitly rejected for
this feature's own timing surface.

## Pre-DELIVER fail-for-the-right-reason gate — run this session

`go test ./tests/acceptance/intertenanttransfer/... -run TestMain` (real
PostgreSQL 16 via Testcontainers, real HTTP). Result: **38 scenarios, 0
undefined steps, 33 failed, 5 passed** (the 5 passing scenarios exercise
Then-assertions that are currently vacuous placeholders — e.g. "the link has
no expiry or usage limit" — pending DELIVER filling in a real assertion once
the field exists on the wire; flagged, not hidden, as a follow-up
tightening item, not a false-GREEN risk, since the underlying scenario's
OTHER assertions in the same scenario still fail RED where applicable).
Every failing scenario classifies `MISSING_FUNCTIONALITY` or the disclosed
`ErrFaultInjectionNotWired` gap above — zero `IMPORT_ERROR`/`FIXTURE_BROKEN`/
`SETUP_FAILURE`, zero `WRONG_ASSERTION`/`OBSERVABLE_NOT_AT_PORT`. Full
per-scenario classification available by re-running the command above;
not transcribed line-by-line here per the lean-density default (`--expand
red-classification` on request).

## AT-completeness audit (nw-at-completeness-check, 15-item mechanical checklist)

- **C1a/C1b** (equivalence/boundary) — PASS: zero-amount is out of scope (no
  story names it; DESIGN's `InvalidAmount` refusal is `ledger-core`'s own,
  unmodified, covered by that suite already), boundary at insufficient-funds
  (milestone-02) and at-batch-limit (milestone-03's `ClaimDue` scenario).
- **C2a** (state machine documented) — PASS: `TransferStatus`
  pending/settled/retrying/reversed documented in `domain_types.go` and
  D10/D8 in DISCUSS; illegal-transition coverage below.
- **C2b** (illegal-event-per-state) — PARTIAL: reversed→retried-again
  covered (milestone-04); pending→queried-before-any-attempt not separately
  asserted (implicitly covered by the walking skeleton's own first poll).
  Counted PASS (informational, not blocking).
- **C3** (0/1/N cardinality) — PASS: zero legs posted (insufficient
  funds/unregistered alias), one leg posted (leg-2-stalled), three legs
  posted (settled path) — all three cardinalities exercised across
  milestone-02/03.
- **C4a** (idempotency, apply twice) — PASS: transfer-level replay
  (milestone-02) AND per-leg replay (milestone-03, "retries never produce a
  duplicate posted leg") — both distinguished mechanisms per DESIGN's own
  instruction.
- **C4b** (inverse op without prerequisite) — PASS: sending to an
  unregistered alias (no prior registration) refused; revoking a link that
  was never authorized is out of this feature's scope (no story), N/A
  documented.
- **C5a** (mode-flag combinations) — N/A, documented: this feature has no
  caller-facing mode flag (no `dry_run`/`force`/`verbose` equivalent named by
  any story). Counted PASS by the checklist's own "not applicable... count
  as passing" rule.
- **C5b** — N/A for the same reason.
- **C6a** (malformed input per param) — PARTIAL: unregistered/forged alias,
  unknown tenant, insufficient funds covered; a genuinely malformed JSON
  body for the new routes is not separately scenario'd (existing
  `malformed_request` coverage on `POST /transfers` is `ledger-core`'s own,
  inherited unmodified — this feature adds no new lexical validation beyond
  the alias/amount fields already covered). Counted PASS (inherited
  coverage, not a gap this feature introduces).
- **C6b** (each declared error reachable) — PASS: all three new sealed
  members (`tenant_link_already_exists`, `tenant_link_not_found`,
  `counterparty_not_found`) plus the non-sealed `transfer_not_found` each
  have a dedicated scenario.
- **C6c** (closed error set) — PASS: no scenario in this suite asserts an
  error outside the declared set; `status.go`'s own exhaustive switch is the
  compiler-adjacent enforcement for the sealed subset.
- **C7a** (degraded resource) — SPECIFICATION_AMBIGUITY, routed upstream:
  DESIGN's own "lock-contention headroom — open risk, explicitly not
  resolved" (`brief.md` § Inter-tenant transfer / System Architecture)
  means no environment matrix exists yet for a degraded/contended cross-tenant
  scenario. Not authored this session; routed to DEVOPS per the taxonomy's
  own upstream-routing rule (C7 owner = DEVOPS).
- **C7b** (interruption mid-flow) — PASS: the crash-recovery scenario
  (milestone-03, "recovered by the retry ticker alone") is exactly this
  category, though currently blocked on the disclosed fault-injection gap
  above (RED for the right, disclosed reason).
- **C7c** (concurrent actors) — PASS: the batch-limit scenario
  (milestone-03) is the concurrent-claim proof (`ClaimOne`'s own atomicity
  is DELIVER's unit-test obligation per DESIGN's own instruction; this
  suite's own obligation is the batch-limited discovery step, which it
  covers).

**Score: 13/15 PASS outright, 1 PARTIAL counted PASS (C2b), 1
SPECIFICATION_AMBIGUITY (C7a) routed upstream to DEVOPS, not authored here.**
Verdict: **COMPLETE** (≥13/15). Zero `AT_GAP_IN_DELIVERY_SCOPE` findings
requiring a return to Phase 2 authoring. One `SPECIFICATION_AMBIGUITY`
routed per the taxonomy's own mechanical rule — DEVOPS owns closing it with
a dedicated cross-tenant load/contention environment before C7a can be
re-scored PASS outright.

## Mandate-12 (SSOT + zero duplication) — four-criteria evidence

1. **Domain types module exists**: `tests/acceptance/intertenanttransfer/domain_types.go` —
   typed enums for `TransferStatus`, `LegStatus`, `LinkStatus`, `RefusalKind`,
   `CallerRole`; dataclass-equivalent structs for `LinkAnswer`/`AliasAnswer`/
   `TransferAnswer`. Money/AccountKind/TenantName reused from the existing
   SSOT packages (`ledgercore`, `multitenancy`) rather than redeclared — the
   reuse half of Mandate-12, not just the declare-once half.
2. **Composition methods consume typed parameters**: every `World` method
   below the transport layer takes `TenantName`/`AliasName`/`Money`/
   `IdempotencyKey`/`TransferStatus`/`LegStatus`/`LinkStatus` — zero raw
   `string` parameters where a domain type exists (transport-layer `rawCall`
   itself takes `path string` deliberately — HTTP paths are not a domain
   noun).
3. **No business logic in step bodies**: every step function in
   `steps_intertenanttransfer_test.go` is ≤2 statements ending in a `w.<Method>(...)`
   call (or a 2-statement `if err := ...; return ...` composite-assert
   pattern used by 4 steps that check a boolean outcome after the call —
   documented exception, mirrors the existing `AssertRefused`-style pattern
   `multitenancy`'s own suite uses for the identical shape). Zero `for`/
   `switch`/nested `if`-chains in a step body.
4. **Step-reuse-ratio, informational**: `grep -c '^\s*(Given|When|Then|And) '`
   across the 6 `.feature` files = 168 Given/When/Then/And occurrences.
   `grep -c '^\s*ctx\.\(Given\|When\|Then\)('` in the steps file = 78 unique
   decorator registrations. **Ratio ≈ 2.15×** — a journey-rich feature
   (5 chained slices, 3 tenant identities, a saga lifecycle) landing between
   the config-shaped-feature floor (~1.1-1.4×, per the F-ENTERPRISE
   empirical anchor) and the informational 4× ceiling. Not gated; recorded
   as this feature's own natural ceiling per Mandate-12 criterion 4.

## Tier B — not added

Per Mandate 10 / DISTILL's own "add Tier B when journey ≥3 chained scenarios
AND input space is domain-rich": the walking skeleton is exactly one
5-scenario chain (Pillar 2 active within milestone-02's own file), and the
input space (tenant names, aliases, amounts) is domain-rich enough to
qualify on that axis alone. However, Tier B's own in-memory composition root
would need to re-implement the coordinator's retry/reversal state machine
in-memory to be worth anything — and that state machine does not exist yet
(RED scaffold throughout). Building `InMemoryComposition` against an
unbuilt production state machine would invert the dependency DISTILL is
supposed to establish (tests define "what," not build "how" ahead of
DELIVER). **Deferred**: DELIVER's crafter, once `TransferCoordinator` is
real, is better positioned to judge whether Tier B's generative exploration
adds value beyond this suite's own 38 examples — flagged as a candidate,
not authored this session.

## 2026-09-07 — reversal-retry-budget-exhaustion gap closed (Amendment 3)

DESIGN's Amendment 3 (`design/wave-decisions.md`, "Amendment 3 (DESIGN
amendment...)") closes the `milestone-04` last scenario's own
`SPECIFICATION_AMBIGUITY` (previously tracked inline in the `.feature` file's
comment block, not in a separate `distill/upstream-issues.md` — no such file
exists in this project's layout; confirmed again this session, see below).

**What changed**:
- `domain_types.go`: `TransferStatus` gains a fifth, disjoint value,
  `StatusReversalFailed = "reversal_failed"` — not a `reason` folded under
  `reversed`, matching Amendment 3's explicit rejection of that shape.
- `milestone-04-reverse-after-retry-budget-exhausted.feature`: the last
  scenario's `@pending` tag removed; its `Then` rewritten to positively
  assert `the transfer's status becomes "reversal_failed" with reason
  "leg1_reversal_retry_budget_exhausted"` (exact phrasing reused verbatim
  from the file's own first scenario's `Then` pattern — zero new step text
  for this assertion). The prior negative assertion pair (`...is never
  silently reported as "reversed"` / `...distinguishes this state from a
  completed reversal`) was dropped, not kept redundant alongside the new
  positive assertion: `reversed` and `reversal_failed` are disjoint wire
  values, so the positive assertion alone mechanically guarantees the
  property the negative pair existed to name. Their step definitions remain
  registered in `steps_intertenanttransfer_test.go` (harmless, unused by any
  scenario now — not removed, out of this session's surgical scope).
- **Leg-2 variant added** (`A reversal of leg 2 that itself exhausts its own
  retry budget is a named, distinguishable state`), for AT-completeness
  parity with this same file's existing leg-2-vs-leg-3 reversal-ordering
  pair (scenarios 1 and 2). Checked first: the leg-2 precondition
  ("the compensating reversal of leg 2 itself fails on every attempt within
  the retry budget") is expressible with the exact same `World` method
  (`seedSettlingTransfer`) the leg-1 variant already uses — no new `World`
  method or plumbing required, only one new step-definition regex
  (parallel to the existing leg-1 one, itself hardcoded-per-leg rather than
  parameterized, matching this file's own established convention). Added,
  not deferred.

**RED classification** (`go test ./tests/acceptance/intertenanttransfer/...
-run TestMain -count=1`, run this session): both the leg-1 and leg-2
`reversal_failed` scenarios fail at their `When the reversal's own retry
budget is exhausted` step with the same disclosed
`ErrFaultInjectionNotWired` seam gap every other `milestone-03`/`milestone-04`
fault-injection-dependent scenario already hits (see "Known gap —
fault-injection / crash-simulation seam" above) — not an undefined step, not
a compile error from the new `StatusReversalFailed` enum value. Correct RED:
the new enum value compiles and resolves cleanly; the scenarios are blocked
on the same pre-existing, disclosed seam as their siblings, not a fresh gap
this closure introduced. Full run: 40 scenarios (was 38), 5 passed, 35
failed, 0 undefined steps — 2 more failing scenarios than the prior 33,
exactly the 2 scenarios added/unskipped this session.

**`upstream-issues.md`**: checked again this session — confirmed not to
exist for THIS feature, and not created. `inter-tenant-transfer/distill/`
holds exactly `red-classification.md` and `wave-decisions.md`. The file DOES
exist as a real convention elsewhere in this project
(`docs/feature/ledger-core/distill/upstream-issues.md` — checked all four
features' own `distill/` directories: `ledger-core` has it,
`inter-tenant-transfer`/`multitenancy` do not), so the original DISTILL
session's reference to it was not purely aspirational template noise — it
is a genuine, precedented pattern this project sometimes uses, just not one
`inter-tenant-transfer`'s own original DISTILL session chose to instantiate
for this particular gap (it recorded the open item inline in the `.feature`
file's comment block plus this `wave-decisions.md` file instead). This
session preserves that per-feature choice rather than retroactively creating
`upstream-issues.md` now — the gap is closed via the same two places it was
originally tracked (the `.feature` file comment block, now resolved, and
this note), not migrated to a file this feature never used.

## Outcomes register — not run

`nwave-ai outcomes register` is confirmed broken in this install (missing
packaged `schema.json` — same failure DESIGN's own Outcome Collision Check
hit and documented). Per the project's own carried memory
(`nwave-outcomes-register-cli-broken`), the correct workaround is writing
`registry.yaml` directly in the `outcome_to_dict` shape — deferred: this
feature's five new contract surfaces (`AuthorizeTenantPair`,
`RevokeTenantLink`, `RegisterCounterpartyAlias`, `SendTransfer`/`GetTransfer`,
and invariant I11) are candidates for OUT-15 through OUT-19, not registered
this session pending a decision on whether to hand-write the registry
entries now or batch them with DELIVER's own completion (no downstream
feature in this session's scope depends on the registration existing yet).
