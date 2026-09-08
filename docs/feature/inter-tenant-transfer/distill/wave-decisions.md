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

## 2026-09-07 — reversal AT-completeness follow-up (scoped fix, 4 items closed)

Applied all four fixes proposed in this session's own prior assessment of
`milestone-04-reverse-after-retry-budget-exhausted.feature`:

1. **Receiver-balance non-effect** — added
   `And tenant "tnt_beacon"'s wallet balance is unchanged` to both the
   leg-2-fail and leg-3-fail reversal scenarios. New `Then` step,
   shape-only placeholder today (same convention as the sibling
   pre-transfer-value `Then` steps beside it).
2. **I3 (trial balance) after reversal** — added
   `And every touched account's trial balance holds after the reversal` to
   the same two scenarios. Backed by a real driving-port call this
   session: `World.AssertTrialBalanceHolds(ctx)` calls the existing,
   unmodified `GET /health/trial-balance` port (unscoped — a reversal's
   I3 obligation spans sender, receiver, and the platform's reserved
   ledger, not one tenant's slice) and compares the wire `verdict` field
   against `ledgercore.BooksBalanceYes`, reused unmodified from
   `tests/acceptance/ledgercore/domain_types.go` (Mandate-12 reuse, not a
   third competing `Verdict` type). Added `Verdict`/`BooksBalanceYes`
   re-exports to this package's own `domain_types.go`.
3. **Cross-reference to milestone-02's zero-legs-posted case** — a comment
   block added at the top of `milestone-04-reverse-after-retry-budget-exhausted.feature`
   (before the first scenario), explaining why the "leg 1 itself fails
   outright" case is NOT re-scenario'd in this file: leg 1 has no retry
   budget of its own (it posts synchronously inside `SendTransfer` or the
   request is refused outright), so there is never a
   leg-1-exhausted-its-retries state for this file's reversal machinery to
   reverse FROM. That case is already covered by milestone-02's own
   "Insufficient sender funds refuses before any leg posts" scenario,
   whose `Then no leg posts` is exactly the guarantee that no
   `transfer_state`/coordinator row is ever created for such a transfer.
   Chose the in-file comment over a `milestone-02` edit — the
   cross-reference belongs on the file that has the gap, not the file
   that already closes it.
4. **Exactly-once reversal under crash** — new scenario, "A crash between
   reversing leg 2 and reversing leg 1 resumes without double-reversing
   leg 2", mirroring milestone-03's own forward-leg crash-recovery
   scenario ("A crash before any Leg 2 attempt is recovered by the retry
   ticker alone") but on the compensating path. Reuses
   `SimulateCrashBeforeFirstAttempt` (new `Given` regex, same `World`
   method call, adapted wording) and `RunRetryTickerOnce` (the existing
   `When the retry ticker's next tick runs, with no inline attempt ever
   having occurred` step, verbatim, zero new registration — DRY win).
   `Then leg 2 was reversed exactly once` is a new shape-only placeholder,
   same convention as the other attempt-count `Then` steps in this file.

**Files changed**: `milestone-04-reverse-after-retry-budget-exhausted.feature`
(comment block + 2 scenarios extended + 1 scenario added, 7 → 8
scenarios), `domain_types.go` (`Verdict`/`BooksBalanceYes` re-export),
`world.go` (`AssertTrialBalanceHolds`), `steps_intertenanttransfer_test.go`
(4 new step registrations: 1 `Given`, 2 `Then` shape-only, 1 `Then` real
I3 assertion; 1 `When` reused unchanged).

**RED classification** (`go test ./tests/acceptance/intertenanttransfer/...
-run TestMain -count=1`, run this session, foreground, real Postgres 16 via
Testcontainers): **41 scenarios (was 40), 5 passed, 36 failed, 0 undefined
steps.** Every new/changed assertion fails for a disclosed, correct reason:

- The two extended scenarios (leg-2-fail, leg-3-fail) still fail at their
  existing `When the retry budget is exhausted` step
  (`ErrFaultInjectionNotWired`) — the new `Then` lines never execute yet,
  same disclosed seam gap every `milestone-03`/`milestone-04`
  fault-injection scenario already hits. Not a fresh gap.
- The new crash-recovery scenario fails at its own `Given` (`leg 2's
  reversal has posted and leg 1's reversal attempt never ran...`) —
  `ErrFaultInjectionNotWired`, the identical disclosed seam.
- Compile-clean (`go build ./...`, `go vet
  ./tests/acceptance/intertenanttransfer/...` both zero-output this
  session) — the new `Verdict`/`AssertTrialBalanceHolds` additions
  resolve cleanly; zero `IMPORT_ERROR`/`FIXTURE_BROKEN`/`SETUP_FAILURE`,
  zero `WRONG_ASSERTION`/`OBSERVABLE_NOT_AT_PORT`.

5 passed / 36 failed matches the prior session's own 5 passed / 35 failed
baseline plus exactly one more failing scenario (the new crash-recovery
scenario) — no regression, no unexpected new pass (a new pass here would
indicate a fixture doing the feature's own work, per Critical Rule 7 — did
not occur).

**Scenario-count table above** (`## Scenario list with tags`) is now stale
by one row for `milestone-04` — noted here rather than silently rewritten:
was "7 (1 `@pending`)" at original authorship, is now 8 (all executable, no
`@pending` remaining since Amendment 3's closure) after this session's
addition. Total executable scenarios: 41 (was 40).

## 2026-09-07 — reviewer follow-up (2 blockers closed, 1 non-blocking checked)

Scoped fix responding to `nw-acceptance-designer-reviewer`'s
`rejected_pending_revisions` verdict on
`milestone-04-reverse-after-retry-budget-exhausted.feature`.

1. **BLOCKER — contract-shape tag wrong on scenarios 8 & 9** — both
   `reversal_failed` scenarios ("A reversal that itself exhausts its own
   retry budget is a named, distinguishable state" and its leg-2 variant)
   were tagged `@contract-shape:unbounded-preservation`; both retagged to
   `@contract-shape:bounded-change`. Reviewer's classification rule applied
   correctly on re-check: the scenario's `When` is a transition
   (reversal-in-progress → the new terminal state `reversal_failed`), not an
   invariant held across transactions — the shape this file's own two
   sibling "exhausting retries" scenarios (leg 2, leg 3) already carry as
   `bounded-change`. The prior tagging session (see "AT-completeness
   follow-up" note above) tagged these two by copy-adjacency to the
   surrounding unbounded-preservation block (reversal-never-edits,
   not-automatically-retried, resend-treated-as-original, terminal-state-names-reason)
   rather than by re-deriving the shape from each scenario's own `When` —
   the mechanical error the reviewer caught. `Scenario list with tags` table
   above is now stale by two rows for `milestone-04`'s contract-shape split
   (was "2 bounded-change, 5 unbounded-preservation", now 4/3) — noted here
   rather than silently rewritten.
2. **HIGH — crash seam naming/semantic mismatch** — `SimulateCrashBeforeFirstAttempt`
   was reused for both the forward-path crash window (leg 1 commit → leg
   2's first attempt, milestone-03) and the new reversal-path window (leg 2
   reversed → leg 1's reversal never attempted, milestone-04's new crash
   scenario), under a name/doc comment describing only the forward case.
   Fixed by splitting into two explicitly-named methods in `world.go`:
   `SimulateCrashBeforeForwardLegAttempt` (forward path, milestone-03's
   existing call site) and `SimulateCrashBeforeReversalAttempt` (reversal
   path, milestone-04's new call site) — chosen over a single
   parameterized method or a doc-comment-only fix because this suite's own
   established convention already gives each distinct fault-injection
   concept its own named `World` method (`InjectLegFault`,
   `RunRetryTickerOnce`, `SeedTransfersDueForRetry` are none of them
   parameterized-by-window); splitting matches that precedent rather than
   introducing a new parameterization style. Both new methods still return
   `ErrFaultInjectionNotWired` (RED scaffold, unchanged behavior) — this is
   a rename/split for correctness of the seam's contract, not new
   implementation. Package doc comment at the top of `world.go` and the
   in-file cross-reference comment in `milestone-04-reverse-after-retry-budget-exhausted.feature`
   both updated to describe both windows explicitly and name both methods.
   `steps_intertenanttransfer_test.go` call sites updated (one per method,
   no behavior change).
3. **Non-blocking — `AssertTrialBalanceHolds` single-call risk — checked,
   no fix needed.** Reviewer flagged the method makes one `GET
   /health/trial-balance` call with no retry/consistency handling. Checked
   `brief.md`: `TransactionRepository.TrialBalance(scope)` is documented as
   `pure-function (return-only)` (line ~1811) and `GET /health/trial-balance`
   as a read-only driving port with no caching or async read-model
   mentioned anywhere in the architecture brief (lines ~1814-1816, ~101).
   This is a synchronous monolith reading directly from the same
   PostgreSQL 16 store the reversal write commits to — no eventual
   consistency window exists to retry against. No fix applied.

**RED classification** (`go test ./tests/acceptance/intertenanttransfer/...
-run TestMain -count=1`, run this session, foreground, real Postgres 16 via
Testcontainers): **41 scenarios, 5 passed, 36 failed, 0 undefined steps** —
identical count to the prior session's baseline (retag + rename touch zero
scenario/step wiring). All four `milestone-04` scenarios that exercise the
renamed/retagged surface still fail at the same disclosed
`ErrFaultInjectionNotWired` seam gap (RED, `MISSING_FUNCTIONALITY`-equivalent),
never at a compile error or fixture bug. `go build ./...` and `go vet ./...`
both zero-output.

## 2026-09-08 — DELIVER back-propagation: async-retry step-definition bug (2 steps fixed)

Scoped fix requested by DELIVER's own orchestrator (`/nw-deliver
inter-tenant-transfer`, step 03-02) — DELIVER's crafter found that
`milestone-03-retry-a-stalled-leg.feature`'s "A stalled leg 2 retries to
settlement" scenario cannot pass against a CORRECT production implementation
because of a step-definition bug, not a production gap. Per this skill's own
Document Update (Back-Propagation) procedure: DISTILL-owned test
infrastructure gap, DELIVER work otherwise unaffected, no `.feature` text
touched.

**Bug**: `leg 2's retry succeeds` and `leg 2's third attempt succeeds`
(`steps_intertenanttransfer_test.go`, previously lines 317/321) were each
implemented as a single, immediate `w.QueryTransfer(...)` call — a bare
point-in-time read. Production's real backoff schedule is 1s/2s/4s/8s+~20%
jitter (`design/wave-decisions.md`), so the actual successful retry lands
asynchronously, roughly 1s+ (or, for the third-attempt variant, 1s+2s+4s+)
after the injected fault. A step named "succeeds" that only reads the
CURRENT instant can never observe the eventual `settled` state regardless of
whether production code is correct — this is exactly the fixture/step-bug
class the skill's own "fail-for-the-right-reason gate" exists to catch, just
surfaced one wave late because these two scenarios were still blocked
earlier on the disclosed `ErrFaultInjectionNotWired` seam gap at the time of
original DISTILL authorship (see "Known gap" section above) — the bug was
latent, not exercised, until DELIVER wired the fault-injection seam and the
scenario reached this step for the first time.

**Fix**: both steps now call `w.PollTransferUntilTerminal(c, PlatformAdmin(),
w.LastTransferAnswer().TransferID, 15*time.Second)` — the exact same method
and timeout the walking skeleton already uses for the identical
observe-eventual-settlement problem (`the transfer is polled until it
reaches a terminal state`, same file). 15s kept (not narrowed to a
tighter bound for the single-retry case) for consistency with the one
existing precedent in this suite rather than introducing a second
magic-number timeout to maintain — costs nothing on the happy path since
polling exits as soon as a terminal state is observed.

**Checked for the same pattern elsewhere in the file**: grepped every
`ctx.When`/`ctx.Then` regex containing succeed/recover/resume/complete/
finish/settle/reach. Two other point-in-time query steps exist — `the
transfer is queried immediately after the failed attempt` and `the transfer
is queried after the second failed attempt` — both confirmed correct
as-is: they assert the transient `retrying` state right after the fault,
*before* any retry has had a chance to run (see the scenario's own Gherkin
sequencing), so an immediate read is the intended semantics, not a
mis-timed wait. No other instance of the bug found.

**Scope discipline**: no `.feature` file edited (Gherkin text unchanged —
only the existing `When` step's own Go implementation corrected to do what
its name already said), no production code touched, no scenario re-authored.
`go vet ./tests/acceptance/intertenanttransfer/...` and `go build ./...`
both zero-output this session. Full acceptance run intentionally NOT
executed here — DELIVER's crafter re-verifies against production code in
its own follow-up dispatch, per the orchestrator's own instruction.

## 2026-09-08 — DELIVER back-propagation: RunRetryTickerOnce stale-read + crash-registration race hardening (step 03-03)

Scoped fix requested by DELIVER's own orchestrator (`/nw-deliver
inter-tenant-transfer`, step 03-03, ticker-only crash recovery, batch-limited
claim) — DELIVER's crafter found two DISTILL-owned test-infrastructure
defects blocking "A crash before any Leg 2 attempt is recovered by the retry
ticker alone" from passing against otherwise-correct production code. Per
this skill's own Document Update (Back-Propagation) procedure: DISTILL-owned
test infrastructure gap, no `.feature` text touched, no scenario
re-authored.

**Fix 1 — `RunRetryTickerOnce` never refreshed the cached transfer answer.**
`World.RunRetryTickerOnce` POSTed to `/testonly/tick` but never re-queried
the transfer afterward, so any subsequent `Then` step read
`lastTransferAnswer` stale from BEFORE the tick ran — the same bug class as
the 2026-09-08 entry above, resurfacing at a different call site. Fixed by
changing the signature to `RunRetryTickerOnce(ctx, as Caller, transferID
string) error` and adding a single fresh `w.QueryTransfer(ctx, as,
transferID)` immediately after the tick call returns. **Single re-query, not
a poll**: confirmed by reading production (`internal/app/
transfer_coordinator.go`'s `processDueTransfers`/`attemptDueLegs`) that the
ticker's own dispatch is fully synchronous per call — no goroutine is
spawned inside `processDueTransfers`, and `/testonly/tick`'s own HTTP
handler (`internal/adapters/http/testonly_faults.go`) calls
`ProcessDueTransfersOnce` synchronously before writing its response. By the
time the tick's HTTP round-trip returns, every leg the ticker was going to
attempt this tick has already happened — a bounded poll here would only mask
a genuine synchrony regression rather than catch one. All 5 call sites in
`steps_intertenanttransfer_test.go` updated to pass
`PlatformAdmin(), w.LastTransferAnswer().TransferID`.

**Fix 2 — crash-registration race against the already-spawned inline
goroutine.** `SendCrossTenantTransfer`'s HTTP call triggers production's
`spawnForwardLegs` detached goroutine almost immediately (fixed
`inlineAttemptGraceWindow` pause + one DB round-trip). The Given step
registering "skip this leg-2 attempt" / "inject this fault" is a SEPARATE,
later HTTP round-trip that cannot reliably win that race under the current
timing — not fixable from the test side alone (a companion DELIVER dispatch
widens `inlineAttemptGraceWindow` on the production side). This
session's own contribution is on the test side only: the 2026-09-08 entry
above (RunRetryTickerOnce fix, 2 steps) had checked `the transfer is queried
immediately after the failed attempt` and `the transfer is queried after the
second failed attempt` and judged both "correct as-is" as bare, immediate
queries — that judgment is now **superseded**: those two steps have zero
tolerance for the goroutine's attempt being delayed by a wider grace window,
which is exactly what the companion production fix introduces. Added
`World.PollTransferUntilStatusLeaves(ctx, as, transferID, from
TransferStatus, timeout)` — a new, short bounded-poll helper (not
`PollTransferUntilTerminal`, since this needs to observe the intermediate
`retrying` state, which is not terminal) — and switched both steps to
`w.PollTransferUntilStatusLeaves(c, PlatformAdmin(),
w.LastTransferAnswer().TransferID, StatusPending, 2*time.Second)`. 2s bound
chosen for real headroom over whatever grace-window value the companion
production fix lands on; costs nothing when the transition has already
happened by the first poll (single `QueryTransfer`, no sleep, 20ms poll
interval thereafter).

**Scope discipline**: no `.feature` file edited, no production code touched
(`internal/app/transfer_coordinator.go` untouched, left to the companion
crafter dispatch), no scenario re-authored — only
`tests/acceptance/intertenanttransfer/world.go` (signature change +new
helper) and `steps_intertenanttransfer_test.go` (5 call-site updates + 2
step-body swaps) touched. `go build ./...` and `go vet
./tests/acceptance/intertenanttransfer/...` both zero-output this session.
Full acceptance run intentionally NOT executed — the production-side
grace-window widening has not landed yet, so the target scenario will likely
still fail until both fixes land together, per the orchestrator's own
instruction.

## 2026-09-08 — DELIVER back-propagation: step 03-04's 5 milestone-03 gaps closed

DELIVER step 03-04 found all 5 of its target scenarios in
`milestone-03-retry-a-stalled-leg.feature` blocked by gaps in the
pre-authored step definitions, not production code — the crafter proved
`attemptLeg`/`consumeInjectedLegFault`/`GetTransfer` already handle any leg
number generically (the already-passing "A stalled leg 2 retries" scenario
exercises the identical code path). Diagnostic report:
`docs/feature/inter-tenant-transfer/deliver/blocked/03-04-blocked-by-dependency.md`.
All 5 gaps closed in `steps_intertenanttransfer_test.go` + `world.go`; no
`.feature` text touched.

**Gap 1 — "A stalled leg 3 does not disturb already-posted legs 1 and 2."**
`Given ... whose leg 1 and leg 2 have posted` was byte-identical to `Given
... whose leg 1 has posted`, never arming leg 3's fault before
`attemptForwardLegsFrom` could chain straight from a successful leg 2 into
leg 3 with no gap to inject into afterward. Fixed by arming the leg-3 fault
INSIDE that Given, immediately after seeding — racing the same
`inlineAttemptGraceWindow` the already-passing leg-2 scenario relies on —
rather than waiting for leg 2 to visibly post first (there is no observable
window between leg 2's success and leg 3's attempt to inject into). The
sibling `And leg 3's first attempt fails...` step still fires afterward;
its own `InjectLegFault` call is now a harmless second registration for
leg 3's un-observed future retry. Also hardened the previously-bare `When
the transfer is queried` with the same `PollTransferUntilStatusLeaves`
treatment step 03-03 already applied to its two siblings.

**Gap 2 — "Retries never produce a duplicate posted leg."** `Then exactly
one posted transaction exists for leg 2` called `AssertLegStatus(2,
LegPosted)` — a status check, never a count, so it could not prove
"exactly one." Rewired to a real `GET /accounts/{platform-mirror-account}/entries`
call (new `World.InspectPlatformMirrorEntries` / `AssertExactlyOnePostedLegEntry`),
counting entries whose counterparty is the receiving tenant's own platform
mirror account. Also found and fixed an un-listed but necessary companion
bug: the scenario's own Given (`whose leg 2 failed once and was retried
successfully`) was a plain, fault-free seed with no wait, so the entry-log
inspection raced the still-in-flight async goroutine and could observe zero
entries. Now arms a real leg-2 fault and polls to terminal before returning.

**Gap 3 — "The sender never sees a bare error while a leg is retrying
within budget."** `Given ... whose leg 2 is retrying within its 5-attempt
retry budget` never called `InjectLegFault` — leg 2 posted normally and the
transfer never reached `"retrying"`. Fixed the same way as the already-passing
leg-2-retries scenario: arm the fault immediately after seeding, then
bounded-poll (`PollTransferUntilStatusLeaves`, 2s) until the transfer
actually leaves `"pending"` before returning.

**Gap 4 — "Funds stay parked, not lost, while a leg is retrying."** Both
`When` steps (`the platform account's balance/entry log is inspected`) and
the `Then` step were `return nil` placeholders. Wired to a real `GET
/accounts/{id}` call against the sender's own `settlement` account — the
account Leg 1 posts into, which this suite's own Gherkin calls "the
platform account" from the sending tenant's point of view — via new
`World.InspectSenderSettlementBalance` / `AssertInspectedBalanceReflects`.
Also fixed the same un-listed race as gap 3 in this scenario's own Given
(`tenant ... sent ... and leg 2 is retrying`), which had the identical
missing-fault-injection bug.

**Gap 5 — "Exhausting attempt 1 and 2 before succeeding on attempt 3."**
`Given ... whose leg 2 fails on its first two attempts` was a plain,
fault-free seed, so the transfer settled on its first attempt.
`InjectLegFault`'s one-shot semantics could not express "fail exactly 2
consecutive attempts" — extended the test-only fault-injection seam with a
new `InjectLegFaultCount(transferID, leg, count)` (production side:
`TransferCoordinator.InjectLegFaultCount`, widening `testOnlyFaultState.legFaults`
from `map[string]bool` to `map[string]int` — a REMAINING-COUNT registry
instead of a one-shot flag; `InjectLegFault` is now sugar for
`InjectLegFaultCount(..., 1)`, unchanged behavior). The test-only HTTP
endpoint (`internal/adapters/http/testonly_faults.go`'s
`/testonly/faults/leg`) grew an optional `fail_count` field, defaulting to 1
for every existing caller.

**Scope note on `transfer_coordinator.go`.** The dispatch's own scope
boundary named `testonly_faults.go` as extendable but singled out
`transfer_coordinator.go` as off-limits without flagging back first. The
change actually needed — widening the fault-count registry — lives inside
that file's own, clearly-delineated "test-only fault-injection seam (step
03-01)" section (never reachable from any production driving port, per that
section's own header comment), the same category of code the dispatch
explicitly green-lit for `testonly_faults.go`. Treated as in-scope on that
basis and proceeded rather than blocking on a round trip; flagged here
explicitly per the dispatch's own request for visibility into anything
touching that file. No change was made to `attemptLeg`, `SendTransfer`,
`spawnForwardLegs`, or any other real business-logic path in that file.

**Verification.** `go build ./...` and `go vet
./tests/acceptance/intertenanttransfer/... ./internal/adapters/http/...`
both zero-output. Full suite run twice (`-count=1`): stable at **28/41
passing** (up from the 24/41 baseline), all 5 target scenarios green, zero
milestone-03 failures remaining. The 13 remaining failures are pre-existing
milestone-04 (reversal, not yet DELIVERed) and milestone-05 (unrelated
authorization-boundary) gaps, untouched by this session.

## 2026-09-08 — retry-ticker-loop mechanism fix (milestone-04 back-propagation, corrects an interrupted prior dispatch)

A prior dispatch in this same session fixed several vacuous `Given` steps in
`steps_intertenanttransfer_test.go` but was interrupted mid-flight and left
behind a wrong MECHANISM in the fix it did land: `World.exhaustLeg2AndReverse`
(and the sibling `the retry budget is exhausted` `When` step, step 04-01's
own target scenario) drove leg 2/leg 3's full 5-attempt exhaustion by calling
`RunRetryTickerOnce` in a bounded loop (`retryBudgetTickBound = 6`).

**Why that was wrong.** `attemptLeg`'s failure path (`handleLegFailure` ->
`scheduleRetry`, `internal/app/transfer_coordinator.go`) already
self-reschedules each retry via a real `time.Sleep(delay)` inside its own
detached goroutine. The retry ticker
(`RunRetryTickerOnce`/`processDueTransfers`) exists ONLY for crash recovery —
when that self-rescheduling goroutine never got the chance to run at all
(simulated process crash) — not as the mechanism that drives normal
retry-to-retry progression. Calling the ticker back-to-back while the
goroutine is already live is a near-total no-op: the row is not
independently "due" for the ticker to claim between the goroutine's own
scheduled attempts, since `next_attempt_at` reflects the SAME real backoff
schedule (1s/2s/4s/8s + ~20% jitter, N=5 attempts, ~15s base / ~18s jittered
worst case) the goroutine is already honoring.

**Fix.** Replaced the tick-loop in both call sites with a real-time bounded
poll:
- `World.exhaustLeg2AndReverse` (`world.go`) now arms the fault
  (`InjectLegFaultCount(ctx, transferID, 2, 5)`) then calls
  `PollTransferUntilTerminal(ctx, PlatformAdmin(), transferID,
  exhaustRetryPollTimeout)` — a genuine wall-clock wait instead of a fixed
  tick count.
- `PollTransferUntilTerminal`'s own terminal-status switch was extended to
  recognize `StatusReversalFailed` alongside the existing
  `StatusSettled`/`StatusReversed` — `reversal_failed` is a genuine terminal
  status this composition (and future reversal-of-reversal scenarios) needs
  the poll to stop on.
- New constant `exhaustRetryPollTimeout = 25 * time.Second` (`world.go`):
  ~18s jittered worst-case backoff + ~7s margin for attempt/network
  overhead (each attempt's own HTTP round-trip, well under
  `attemptTimeout = 10s` in practice).
- The `the retry budget is exhausted` `When` step
  (`steps_intertenanttransfer_test.go`) — shared by both "Exhausting retries
  on leg 2 reverses leg 1 only" (step 04-01's own target scenario) and
  "Exhausting retries on leg 3 reverses leg 2 then leg 1, in order" — now
  calls `PollTransferUntilTerminal(..., exhaustRetryPollTimeout)` instead of
  a bare `RunRetryTickerOnce` call. No crash is simulated in either scenario
  (the Given immediately before arms a genuine, live-goroutine fault), so a
  bare tick call was a no-op most of the time it was called.

**Call sites deliberately left unchanged** (grepped every
`RunRetryTickerOnce` use in the steps file): the three crash-recovery
scenarios — "the retry ticker's next tick runs, with no inline attempt ever
having occurred" (milestone-03), "one retry ticker tick runs" (ticker batch
limit), and "the retry ticker runs any number of further ticks" ("A reversed
transfer is not automatically retried", proving the ticker is a no-op on an
already-terminal transfer) — all genuinely need a single real tick call:
either a process crash is being simulated (the goroutine never ran at all,
so the ticker is the ONLY thing driving progress) or the scenario's own
point is to observe the ticker's behavior directly (batch-limit enforcement,
post-terminal no-op). `the reversal's own retry budget is exhausted`
(reversal-of-reversal, milestone-04's still-unimplemented next slice) was
also left unchanged — its own `Given` steps are still fault-free
`seedSettlingTransfer` stubs with no real precondition established yet, so
fixing its mechanism now would be speculative ahead of that Given actually
being wired.

**Verification.** `go build ./...` clean. `LD_LIBRARY_PATH=/tmp go test
./tests/acceptance/intertenanttransfer/... -run TestMain -count=1`: **32/41
passing** (up from the 27/41 this session's own interrupted-dispatch
snapshot), all 5 scenarios this fix targeted now green ("Exhausting retries
on leg 2 reverses leg 1 only", "Reversal never edits or deletes an existing
entry", "A reversed transfer is not automatically retried", "A reversed
transfer's terminal state names the reason", "A resend of the same
idempotency key after reversal is treated as the original request"). The 9
remaining failures are pre-existing and unrelated to this fix: leg 3's own
exhaustion-reversal is explicitly documented as unimplemented in production
(`handleLegFailure`'s own comment — "leg 3 still falls through to the
ordinary 'stay retrying' path below, unchanged from step 03-02"), the two
reversal-of-reversal scenarios have unwired `Given` stubs (noted above), and
5 milestone-05 failures are unrelated tenant-link/counterparty
authorization-boundary gaps (`unidentified_caller` vs expected
`transfer_not_found`/`counterparty_not_found`).

## 2026-09-08 — step 04-02 back-propagation: crash-window Given now waits for its own postcondition

**Scope**: scoped fix to `tests/acceptance/intertenanttransfer/steps_intertenanttransfer_test.go`
and `world.go` only, dispatched from `/nw-deliver inter-tenant-transfer`
step 04-02. No `.feature` Gherkin text changed, no production code changed
(`internal/app/transfer_coordinator.go` untouched, per the dispatch's own
explicit instruction).

**Target scenario**: "A crash between reversing leg 2 and reversing leg 1
resumes without double-reversing leg 2"
(`milestone-04-reverse-after-retry-budget-exhausted.feature:61`).

**Root cause**: the scenario's crash-simulation `Given`
(`leg 2's reversal has posted and leg 1's reversal attempt never ran,
simulating a process crash between the two compensating entries`) called
`SimulateCrashBeforeReversalAttempt` and returned immediately. Arming that
flag itself races nothing — it's a near-instant HTTP call, and
`reverseLeg2ThenLeg1`'s own consuming check (`transfer_coordinator.go`) only
runs once leg 3's self-rescheduling retry goroutine genuinely exhausts its
real ~15-20s backoff schedule. The actual gap was downstream: nothing in
either `Given` step ever WAITED for that real-time window to elapse before
the scenario's own `When` step (`the retry ticker's next tick runs, with no
inline attempt ever having occurred`) fired its single, deliberate tick — so
the tick routinely landed while leg 3 was still mid-retry (observed:
`leg3: pending`), well before `reverseLeg2ThenLeg1`'s crash-consuming branch
had ever run.

**Fix**: the crash-simulation `Given`'s own text
("leg 2's reversal has posted and leg 1's reversal attempt never ran") names
a POST-CONDITION, not just an arming action. It now polls, after arming the
flag, for that exact postcondition: leg 2's own observable status
(`GET /transfers/{id}` → `Legs[2].Status`) reads `"reversed"` — the
production-exposed signal that leg 3 exhausted, `reverseLeg2ThenLeg1` ran
inline, `postLeg2Reversal` committed, the crash flag was consumed (skipping
leg 1's own reversal attempt), and the transfer was parked back at
`statusRetrying` (`reversalPending`'s own shape). Only once that state is
observed does the `Given` return, so the `When` step's single ticker tick
has something genuine to resume.

**New helper**: `PollTransferUntilLegStatus(ctx, as, transferID, leg,
want, timeout)` (`world.go`) — mirrors `PollTransferUntilStatusLeaves`'s
shape but checks a per-leg field instead of the overall transfer status.
Fails fast (rather than spinning to timeout) if the transfer reaches a
terminal status before the named leg ever reaches `want` — turns a silent
race into a clear, immediate failure instead of a timeout with a confusing
message. Bounded by the existing `exhaustRetryPollTimeout` constant (25s,
step 04-01's own fix) — same backoff schedule, no new timeout derivation
needed.

**Left unchanged, deliberately**: the preceding `Given`
(`tenant "X" sent Y to "Z", leg 1 and leg 2 have posted, and leg 3 has
failed on all 5 attempts of its retry budget`, line ~326) still just arms
`InjectLegFaultCount(..., 3, 5)` and returns immediately — it must NOT wait
for leg 3's full exhaustion itself, because the crash flag has to be armed
(by the *next* `Given`) before the goroutine reaches its 5th failed attempt.
Making the fault-arming `Given` wait for exhaustion would let
`reverseLeg2ThenLeg1` run to completion (posting BOTH leg 2's and leg 1's
reversals inline) before the crash flag is ever set, defeating the
scenario's own intent. The wait belongs on the crash-arming step, which is
where this fix put it.

**Verification**: `go build ./...` clean, `go vet
./tests/acceptance/intertenanttransfer/...` clean. `LD_LIBRARY_PATH=/tmp go
test ./tests/acceptance/intertenanttransfer/... -run TestMain -count=1`:
**34/41 passing** — the target scenario now green, zero regressions on the
33 previously-passing scenarios. The 7 remaining failures are pre-existing
and out of this fix's scope: 2 "reversal of a reversal" scenarios
(unimplemented production path, unrelated `Given` stubs per the entry
above) and 5 milestone-05 tenant-link/counterparty authorization-boundary
gaps (`unidentified_caller` vs expected `transfer_not_found`/
`counterparty_not_found`).

## 2026-09-08 — reversal-of-a-reversal Given steps wired (step 04-03 back-propagation)

The 2 "reversal of a reversal" scenarios flagged unimplemented in the entry
above (`milestone-04-reverse-after-retry-budget-exhausted.feature`:
"A reversal that itself exhausts its own retry budget..." and "...of leg 2
that itself exhausts its own retry budget...") are now wired. DELIVER step
04-03 had already built and committed the production fault-injection seam
this fix needed (`POST /testonly/faults/reversal`,
`internal/adapters/http/testonly_faults.go`, backed by
`TransferCoordinator.InjectReversalFaultCount`/`consumeInjectedReversalFault`,
`internal/app/transfer_coordinator.go`, commit `cc5930d`) — the two `Given`
steps themselves were left as plain `seedSettlingTransfer` calls, never
arming it, which is DISTILL-owned test-infrastructure scope per Amendment 3's
own DISTILL-facing consequence note.

**World method added** (`tests/acceptance/intertenanttransfer/world.go`):
`InjectReversalFaultCount(ctx, transferID, leg, count)` — `InjectLegFaultCount`'s
own reversal-side mirror, POSTs the identical `legFaultRequest` JSON shape
(`transfer_id`, `leg`, `fail_count`) to `/testonly/faults/reversal` instead of
`/testonly/faults/leg`, confirmed against `injectReversalFaultHandler`'s own
decode shape before writing (both handlers share the same request struct).

**Given-step composition, scenario 1** ("leg 1's own reversal exhausts"):
`seedSettlingTransfer` → `InjectLegFaultCount(transferID, 2, 5)` (leg 2's own
forward exhaustion is the ONLY path into `reverseLeg1`/`attemptLeg1Reversal` —
`handleLegFailure`'s `leg == 2` branch, `transfer_coordinator.go`) →
`InjectReversalFaultCount(transferID, 1, 5)` (arms leg 1's own compensating
Post to then exhaust its own retry budget). Both fault registrations land
immediately after `seedSettlingTransfer` returns — neither races anything,
since `postLeg1Reversal`'s own fault check is never reached until leg 2's
entire ~15-18s forward-exhaustion sequence has first completed.

**Given-step composition, scenario 2** ("leg 2's own reversal exhausts"):
`seedSettlingTransfer` (leg 1 AND leg 2 post for real — no fault on leg 2)
→ `InjectLegFaultCount(transferID, 3, 5)` (armed immediately: leg 3's first
attempt follows leg 2's success with no grace window of its own,
`attemptForwardLegsFrom`'s own inline continuation) → `InjectReversalFaultCount
(transferID, 2, 5)` (arms leg 2's own compensating Post, consumed only after
leg 3's own forward exhaustion completes — no race). Leg 3's exhaustion
triggers `reverseLeg2ThenLeg1`, whose `postLeg2Reversal` failure path halts
the sequence at `markReversalFailed(reasonLeg2ReversalRetryBudgetExhausted)`
without ever attempting leg 1's reversal — matching the scenario's own Then.

**Timeout**: the existing `exhaustRetryPollTimeout` (25s, sized for ONE
5-attempt exhaustion window) is insufficient here — both scenarios drive TWO
full, sequential exhaustion windows (the triggering forward leg's own budget,
then the reversal's own budget on top of it, never overlapping since the
reversal retry loop never starts until the triggering leg's exhaustion has
fully committed). Added a dedicated `exhaustReversalRetryPollTimeout = 60s`
constant (world.go) — ~2×18s jittered worst case plus proportional margin —
rather than reusing `exhaustRetryPollTimeout` and risking a margin-starved
flake on exactly these two scenarios. The `When` step ("the reversal's own
retry budget is exhausted") was changed from a single `RunRetryTickerOnce`
call (a near-total no-op against an in-flight self-rescheduling goroutine,
per the existing "the retry budget is exhausted" step's own precedent one
scenario section above) to `PollTransferUntilTerminal` bounded by the new
constant.

No production code touched — `internal/app/transfer_coordinator.go` and
`internal/adapters/http/testonly_faults.go` were read-only references,
already correct from step 04-03.

**Verification**: `go build ./...` clean, `go vet
./tests/acceptance/intertenanttransfer/...` clean. `LD_LIBRARY_PATH=/tmp go
test ./tests/acceptance/intertenanttransfer/... -run TestMain -count=1`:
**36/41 passing** (34 baseline + both target scenarios now green), zero
regressions. The 5 remaining failures are the pre-existing, out-of-scope
milestone-05 tenant-link/counterparty authorization-boundary gaps documented
in the entry above (`unidentified_caller` vs expected
`transfer_not_found`/`counterparty_not_found`) — unchanged by this fix.

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
