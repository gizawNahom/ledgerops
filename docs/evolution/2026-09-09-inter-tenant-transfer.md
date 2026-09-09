# Evolution — inter-tenant-transfer

**Status: DELIVERED.** All 21 roadmap steps complete (`des-verify-integrity
docs/feature/inter-tenant-transfer/deliver/` → exit 0, "All 21 steps have
complete DES traces"). L1-L6 refactor pass committed (`e5b3a4a`). Phase 4
adversarial review: APPROVED, zero blocking issues. All 41 acceptance
scenarios green, full repo regression clean.

---

## Feature summary and business context

**JTBD**: let two tenants that already trust each other move value directly,
by name, at whatever volume their business needs — without either learning
the other's raw tenant id, and without the platform operator personally
authorizing each transfer.

This feature builds job **J8** (`docs/product/jobs.yaml`), promoted
2026-09-06 out of `deferred:` where it had sat since `multitenancy`'s own
DISCUSS wave (2026-09-03) — the deferral's own stated dependency ("depends
on tenant isolation shipping first") was satisfied once `multitenancy`
shipped. Also builds **J9** (operator authorizes/revokes a standing
tenant-pair link) and **J10** (observe a transfer's exact lifecycle state).
A prior DISCUSS pass at this same feature-id had explored a *rare,
no-pre-established-trust* framing and was rejected as the wrong scope; this
session's own scope is the routine-trust case only (D5).

**Five user stories, one per slice**:

| Story | Job | Invariant | Slice |
|---|---|---|---|
| US-1 — Authorize a tenant pair | J9 | I11 (at most one active link per pair) | 01 |
| US-2 — Send a transfer to a named counterparty | J8 | I11, I8 (reused, unmodified) | 02 |
| US-3 — Watch a stalled leg retry automatically | J10 | — (extends the retry/backoff mechanism) | 03 |
| US-4 — See a transfer reverse after retry budget exhausted | J10 | D7 (append-only compensation) | 04 |
| US-5 — Trace a transfer, prove a third tenant cannot forge into it | J10 | — (extends dual-party authorization) | 05 |

Personas: P1 (integrating developer, now also the sender/receiver side of a
cross-tenant transfer) and P2 (platform operator, authorizes/revokes
standing pairs — never authorizes individual transfers). No new persona
introduced.

**Locked decisions carried through delivery** (full rationale in DISCUSS's
own section of `feature-delta.md`): 3-leg platform-as-hub account model
(2N accounts, never O(N²) — D7); orchestration saga, not choreography (D8);
standing trust until revoked, never per-transfer or expiry-bound (D9); four
caller-visible states — pending/settled/retrying/reversed — via one `GET
/transfers/{id}` (D10); recipient addressed by a tenant-scoped
`counterparty_alias`, never a raw tenant id (D11).

---

## Key decisions

### DISCUSS wave
D1-D12 (feature type, walking skeleton, UX depth, JTBD, scope, account
model, saga scheme, trust model, visibility, alias naming, scope-assessment
PASS) — see `feature-delta.md`'s own `## Wave: DISCUSS` sections, all
carried in unmodified through DELIVER.

### DESIGN wave (three legs: system → domain → application)
- **No new bounded context.** `TenantLink` and `CounterpartyAlias` are two
  new aggregate roots inside the existing **Ledger** context — "link" and
  "alias" are new nouns, not a competing model of anything already named.
- **`TenantLink`**: `{link_id, tenant_a, tenant_b (canonicalized pair),
  status}`. Canonicalization (sorting the pair before comparison) is
  load-bearing — it's what makes `AuthorizeTenantPair(B,A)` collide with an
  existing `AuthorizeTenantPair(A,B)` on the same uniqueness check.
- **`CounterpartyAlias`**: `{tenant_id, alias, tenant_link_id,
  target_tenant_id, target_account_id}`, identity `(tenant_id, alias)` —
  alias uniqueness is scoped to the *owning* tenant's own namespace, never
  global. `ResolveCounterparty` deliberately collapses "alias never
  existed" and "alias existed but its link died" into one refusal
  (`counterparty_not_found`) — an info-leak decision, not a shortcut,
  mirroring how `account_not_found` already closes the same class of leak
  for I8.
- **Rejected: a `Transfer` domain aggregate.** The candidate design most
  likely to look tempting. Rejected for three compounding reasons: (1) no
  true invariant lives there — each leg is already fully decided by `Post`
  the instant it succeeds; (2) it would force a cross-aggregate-type
  transaction spanning two tenants' own accounts, exactly what I8 exists to
  forbid; (3) D8 already named this a saga/coordinator, not an aggregate.
  `Transfer` stays application-layer coordinator state (`TransferState`,
  ADR-014) — which is also why `transfer_not_found` is **not** a sealed
  `domain.ViolationKind` member: there's no domain aggregate for it to
  violate.
- **Reversal is not a new domain operation.** A compensating movement is a
  fresh `Post` call with the original leg's `From`/`To` swapped — the same
  kind of fact run in the opposite direction, reusing I1/I4's own checking
  logic for free, and append-only (D7) by construction since `Post` never
  edits or deletes an `Entry`.
- **ADR-015** (transfer-coordinator persistence and execution) and three
  amendments: (1) the retry-due scan is unconditional over every
  non-terminal row, not scoped to `status = 'retrying'` — closes a
  crash-recovery gap where a row created but never attempted would be
  unrecoverable; (2) five scale-focused fixes (lease-vs-backoff
  reconciliation, worst-case timing ≈204s, `ClaimDue`/`ClaimOne` split,
  operational metrics, a corrected concurrency estimate); (3) the
  `reversal_failed` terminal state — a disjoint `TransferStatus` value, not
  a `reason` riding on `reversed`, so a caller checking only
  `status == "reversed"` cannot mistake an incomplete compensation for a
  completed one.
- **ADR-016** (dual-party transfer authorization): `requireTransferParty`,
  a new middleware — `requireTenantKeyOrOperatorKey` grants "any tenant,"
  structurally wrong for a resource naming two *specific* tenants. One
  database read, one refusal call, covering both "nonexistent" and
  "exists but forbidden" through the identical code path by construction.

### DISTILL wave
41 executable scenarios across 6 files (walking skeleton + 5 milestones),
zero contradictions found against DISCUSS/DESIGN. A dedicated
fault-injection/crash-simulation test seam was flagged as DELIVER's own to
design (`internal/adapters/http/testonly_faults.go`, built in step 03-01).

---

## Work completed

Five slices, 21 roadmap steps, RED→GREEN→COMMIT (ADR-025 3-phase canon)
throughout:

- **Slice 01** (4 steps) — `tenant_links` migration + `tnt_platform` seed;
  `TenantLink` domain aggregate; repository/adapter + use cases; HTTP wiring
  (`POST /tenant-links`, `DELETE /tenant-links/{link_id}`).
- **Slice 02** (6 steps) — `counterparty_aliases`/`transfer_state`
  migration; `CounterpartyAlias` domain aggregate; repository/adapter;
  `POST /counterparties`; **`TransferCoordinator` core** (`SendTransfer`,
  the 3-leg happy path, the walking skeleton) — the single riskiest bet in
  the feature, proven on the first real attempt; remaining refusal/
  idempotency scenarios.
- **Slice 03** (4 steps) — the test-only fault-injection seam; retry
  mechanics (backoff, budget, `ClaimOne`/`ClaimDue`, Prometheus gauges);
  ticker-only crash recovery + batch-limited claim; remaining observational
  scenarios.
- **Slice 04** (4 steps) — leg-1 reversal on leg-2 exhaustion; leg-2-then-
  leg-1 ordering + crash-mid-reversal recovery; the `reversal_failed`
  terminal state + its own counter; remaining preservation scenarios (this
  last step is where the race condition below was found).
- **Slice 05** (3 steps) — `requireTransferParty` middleware;
  `transfer_not_found` wire mapping + operator/third-party trace;
  forged-alias isolation + the byte-identical refusal-shape proof.

---

## Lessons learned

**1. Vacuous test placeholders can hide for a long time — audit
proactively, don't wait to find them reactively.** DISTILL's own
pre-DELIVER gate had already flagged "5 vacuous-pass scenarios" as a known,
disclosed carry-forward item (`distill/red-classification.md`). What wasn't
anticipated was how many *more* of the same class existed beyond that
initial five, and how they compounded: a `Given` step that claims to set up
"leg 2 has failed on all 5 attempts" but is a bare `seedSettlingTransfer`
call with no fault armed will pass its own scenario for the wrong reason
indefinitely, until something forces the real precondition to be exercised.
This pattern recurred across steps 03-02 through 05-03 — seven-plus separate
back-propagation fixes were needed, each found only when a crafter actually
traced *why* a scenario passed rather than trusting a green result. The
actionable lesson: a scenario passing is not evidence its `Given`/`Then`
steps do what their own Gherkin text claims — spot-check the step
*implementations*, not just the scenario outcomes, especially for anything
authored ahead of the production code it's meant to gate.

**2. Strengthening an "obviously passing" assertion can surface a real
bug — don't skip that work because the test already shows green.** Step
04-04's own target scenarios were passing at the Cucumber level with
placeholder `return nil` assertions. Wiring them to real entry-log
diffs and attempt-count checks (not for their own sake, but because they
were genuinely vacuous) surfaced an intermittent race: `SendTransfer` used
to spawn its forward-leg attempt goroutine *before* the HTTP handler wrote
its response, so the coordinator's own `inlineAttemptGraceWindow` (meant to
let a second, racing test-only HTTP call land first) started counting down
before the invariant it depended on was actually true. Under load, a leg
could complete for real *after* a transfer had already been reversed —
money moving after the caller was told it hadn't. Fixed by moving the
trigger to fire only after the response is written, with a `created bool`
guard against double-triggering on idempotent replay; the fix's own
`inlineAttemptGraceWindow` widening (50ms → 750ms) was based on directly
measured round-trip latency (18-113ms observed), not a guess. Verified
stable across 9+ consecutive full-suite runs before being trusted. The
actionable lesson: a placeholder assertion's green result is not proof of
anything — treat "this test technically passes" and "this test actually
tests the thing" as two separate questions, always.

**3. The retry ticker is a crash-recovery mechanism, not the normal
retry driver — a subtle distinction that caused real test flakiness before
it was named explicitly.** `attemptLeg`'s own failure path self-reschedules
via a real `time.Sleep`-based goroutine; the ticker (`ClaimDue`+`ClaimOne`)
exists solely to pick up a row whose self-rescheduling goroutine never got
to run at all (a genuine process crash). Test helpers that tried to use the
ticker to *drive* multi-attempt exhaustion (rather than just verify crash
recovery) were racing against a live self-rescheduling goroutine that was
already doing the same work — fixed by replacing tick-loops with real-time
bounded polls wherever the scenario wasn't actually simulating a crash.

---

## Retrospective — 5 Whys on the race condition (Phase 8)

A dedicated root-cause pass (`nw-troubleshooter`) went deeper than the
lessons above. Two independent root causes compounded rather than either
alone explaining the full picture:

**Root cause A (why the defect was possible)**: a cross-layer temporal
invariant ("the goroutine starts only after the HTTP response is written")
was encoded as an unverified assertion inside a tunable constant's own doc
comment (`inlineAttemptGraceWindow`), rather than as a structural code
dependency — so it could be silently false from the moment it was
introduced (step 03-01), with nothing making that falseness observable.
By contrast, this same file's *other two* detached goroutines
(`scheduleRetry`, `scheduleReversalRetry`) are safe by construction — they
re-enter through `ClaimOne`'s database-enforced atomic claim, not a timing
assumption. `TriggerForwardLegs` was the one goroutine whose safety
depended on a bare wall-clock sleep standing in for "another process's
request has landed."

**Root cause B (why it went undetected for an entire slice)**: the race
was actually observed once, at step 03-03, as flakiness — and fixed
*quantitatively* (widen the window 20ms→50ms) rather than by asking why an
ordering race existed at all. That made the failure rarer, not impossible.
Meanwhile the assertions positioned to observe exactly this failure mode
were vacuous `return nil` placeholders that could not distinguish "leg 2
correctly never posted" from "leg 2 posted for real and nothing checked."
Detection required both the rare race to fire *and* a real assertion
watching for it — from 03-01 through 04-03, the second condition was never
true.

**Process recommendations** (for future features in this codebase, not
generic advice):
1. Any goroutine whose correctness depends on "some other call already
   happened" must be gated by a claim/lock/idempotency check, never by a
   wall-clock sleep alone — a wall-clock window may only reduce
   contention/latency, never serve as the sole correctness mechanism. Worth
   codifying as an explicit rule in `adr-015` or a successor ADR.
2. A production constant widened purely to reduce an observed flake rate
   (as `inlineAttemptGraceWindow` was, once) should be treated as an
   incomplete investigation by definition — "flakiness went away when I
   added margin" should never be the terminal explanation on its own.
3. Any `Then`/`When` step encoding a *negative or preservation* assertion
   ("X never happens," "Y remains unchanged") deserves a mandatory
   spot-check at slice close, not just an end-of-feature audit — this is
   the assertion class that stayed silently vacuous longest in this
   feature's own delivery, because a placeholder here produces a false
   negative rather than an obviously-wrong false positive.

**Open follow-up item, not yet actioned**: `TriggerForwardLegs`
(`internal/app/transfer_coordinator.go`) remains the one goroutine in this
codebase whose safety still depends on a bounded wall-clock window (now
750ms, comfortable margin over a 113ms worst-observed round trip) rather
than a database-enforced claim — disclosed in its own code comment as an
accepted residual risk, but not documented anywhere at the architecture
level. Recommend either (a) capturing it as an explicit residual-risk line
in `adr-015`, or (b) implementing the disclosed alternative (an
acknowledgment channel the test-only fault-injection call signals,
replacing the timer entirely) as a follow-up hardening item.

---

## Issues encountered (beyond the two above)

- Session rate-limit interruptions occurred mid-step three times (steps
  01-03, 02-05, 03-02, 04-01); each was resumed from its own last-verified
  state per the resume-vs-restart discipline (GREEN-complete → resume) with
  no lost work, since `des-commit`'s own atomicity meant nothing was ever
  left half-committed.
- One accidental `git stash pop` during step 02-01 briefly introduced an
  unrelated, older stash's conflicting content into the working tree —
  caught and reverted within the same session before it could be committed;
  the stash itself (`stash@{0}`, a rejected prior DISCUSS pass at this
  feature-id) was never dropped and remains intact.
- The `pact_ffi` linker workaround (`libpact_ffi.so` copied to `/tmp`) went
  stale twice across the session (once after a fresh clone-equivalent
  environment reset, once from a corrupted copy) — both times diagnosed and
  fixed within one dispatch by re-copying from the canonical
  `~/.pact-go-lib/` source.

---

## Migrated / permanent artifacts

- `docs/product/architecture/brief.md` § Inter-tenant transfer — the
  cross-feature SSOT, already updated during this feature's own DESIGN
  wave (Component Inventory, Driving/Driven ports, Refusal taxonomy, Reuse
  Analysis tables all extended in place). This is the canonical permanent
  home for this feature's architecture; `design/wave-decisions.md` and
  `distill/wave-decisions.md` are discarded per the standard destination
  map (`*/wave-decisions.md` → extracted into this evolution doc + already
  captured in `brief.md`), not separately migrated.
- `docs/product/architecture/adr-015-transfer-coordinator-persistence-and-execution.md`,
  `adr-016-dual-party-transfer-authorization.md` — already permanent,
  standalone files; no migration needed.
- `docs/architecture/inter-tenant-transfer/slices/slice-01..05-*.md` —
  migrated from `docs/feature/inter-tenant-transfer/slices/`, mirroring
  `multitenancy`'s own precedent (`docs/architecture/multitenancy/slices/`).
