# DESIGN Decisions — inter-tenant-transfer

Application-level leg (third of three: `nw-system-designer` → `nw-ddd-architect`
→ `nw-solution-architect`, this document). Full narrative:
`docs/product/architecture/brief.md` § Inter-tenant transfer (Application
Architecture). ADRs: `adr-015-transfer-coordinator-persistence-and-execution.md`,
`adr-016-dual-party-transfer-authorization.md`.

## Key Decisions

- **Sync/async settlement (Pre-requisite #5), settled concretely**: `POST
  /transfers` (cross-tenant variant) always responds `pending` after Leg 1
  settles; it never blocks for Legs 2/3 and never itself reports `settled`.
  Legs 2/3 attempt inline (immediately, in a detached goroutine spawned by
  the same handler) on the first try; only a failed first attempt hands off
  to the ticker-driven retry loop. One function (`attemptLeg`) serves both
  the inline and background paths.
- **Coordinator state persistence (Pre-requisite #2), settled**: a new
  Postgres table, `transfer_state`, one row per transfer, updated in place.
  `UPDATE` is deliberately granted to `ledgerops_app` here (and on
  `tenant_links`, for revocation) — an explained divergence from OPS-10's
  no-`UPDATE` posture, which governs ledger history (`entries`,
  `transactions`), not coordinator process state.
- **Retry-safety reuses `IdempotencyStore`, unmodified, with synthesized
  keys** (`{Idempotency-Key}:leg2`, `:leg3`, `:leg1:reverse`,
  `:leg2:reverse`) — I7's own mechanism, re-applied one layer up, per
  ADR-014's own framing. No new dedup invariant or table.
- **Fixed retry budget N = 5, exponential backoff 1s/2s/4s/8s + ~20%
  jitter** — matches US-3's own domain example, not left configurable.
- **A reserved internal tenant, `tnt_platform`**, seeded by migration
  (mirroring `tnt_legacy_seed`), owns one platform-mirror account per
  business tenant — the concrete answer to which tenant scope Leg 2's
  "platform ledger" movement lives under, so it remains a legal
  single-tenant `Post` call under I8.
- **Account bootstrap (settlement + platform-mirror) is idempotent,
  first-use, inside `SendTransfer`** — not at `AuthorizeTenantPair` time
  (slice 01's scope explicitly excludes posting).
- **`GET /transfers/{transfer_id}` gets a new middleware,
  `requireTransferParty`** — not a reuse of `requireTenantKeyOrOperatorKey`,
  which grants "admin OR any tenant" and is structurally wrong for a
  resource naming two *specific* tenants. The new middleware reads the
  resource once and refuses both "nonexistent" and "exists but forbidden"
  through the identical code path, satisfying US-5's "no distinguishable
  signal" requirement by construction.
- **Three new sealed `domain.ViolationKind` members**
  (`tenant_link_already_exists` 409, `tenant_link_not_found` 404,
  `counterparty_not_found` 404 reused/newly load-bearing), confirmed at the
  wire, extending both `exhaustive`-linted switch surfaces (DDD-12/DDD-17).
  `transfer_not_found` (404) is confirmed **not** a sealed member — no
  domain aggregate exists for it to violate (ADR-014) — wired via an
  ordinary `errors.Is` check the linter does not cover, flagged to DISTILL
  as needing its own explicit test coverage.
- **No new dependency**: `time.Ticker` (stdlib) drives the retry scan;
  a job-scheduling library or message queue was considered and rejected as
  unneeded machinery at this project's single-instance, single-digit-
  concurrent-retries scale.
- **Amendment (same session, found in user review of this handoff): the
  retry-due scan is unconditional over every non-terminal `transfer_state`
  row, not scoped to `status = 'retrying'`.** A row starts at `status =
  'pending'` the instant `SendTransfer` commits Leg 1 — before the detached
  goroutine's first Leg 2 attempt has even run. A `'retrying'`-only scan
  left that window permanently unrecoverable from a crash: Leg 1's money
  moved, and nothing would ever revisit the row. Fixed by decoupling the
  caller-facing status label from scan eligibility: `next_attempt_at` is set
  at row creation (never `NULL`), the claim predicate covers `status IN
  ('pending', 'retrying')`, and `attemptLeg` performs the atomic claim
  itself — inline and ticker-driven attempts now go through the identical
  claim, closing the crash gap and the pre-existing double-processing race
  concern with one mechanism. `TransferStateRepository.ClaimDueForRetry`
  renamed `ClaimDue`; the ticker's own `retryDueTransfers` renamed
  `processDueTransfers`. Full narrative: ADR-015 Amendment.
- **Amendment 2 (same session, five findings from a second, scale-focused
  review — `nw-system-designer-reviewer`, commissioned by the user,
  Opus — applied as directed, cheap/paper-level subset only)**:
  1. **Lease-vs-backoff conflict resolved**: the claim's 15s lease is a
     short-lived, in-flight-only placeholder; the documented 1/2/4/8s
     backoff ladder is what the row is actually left holding after every
     non-crash attempt outcome — stated explicitly, previously implicit.
  2. **Worst-case time-to-terminal computed**: ≈68s per leg's worst-case
     exhaustion; **≈204 seconds (≈3.4 minutes)** worst case overall
     (Leg 3 exhausts, then both reversals also each exhaust before
     succeeding) — a number US-3/US-4 pollers need, not previously stated.
  3. **`ClaimDue` split and batch-capped**: single-row atomic claim
     (renamed `ClaimOne`, used by `attemptLeg`) separated from a new,
     read-only, batch-limited discovery step (`ClaimDue(ctx, now,
     batchLimit)`, `batchLimit = 100`, ticker-only) — closes the
     unbounded-backlog-claim gap the original signature had.
  4. **Operational visibility added**: two new Prometheus gauges
     (`ledgerops_transfer_state_nonterminal_count`,
     `..._oldest_next_attempt_age_seconds`), extending the existing OPS-5
     `Metrics` component — not a new HTTP port.
  5. **Concurrency estimate re-derived** via Little's Law (was
     tenant-count-derived, the wrong methodology): bounded above at ≈512
     non-terminal transfers under an extreme, almost-certainly-unrealistic
     assumption; the realistic figure is genuinely unmeasured.

  **Explicitly not resolved, recorded as an open risk, not silently implied
  as settled**: whether the platform-as-hub model's lock-contention/hot-set
  concentration (3 transactions / 6 row locks per cross-tenant transfer vs.
  1/2 today, funneled through the shared `tnt_platform` mirror accounts)
  fits within real throughput headroom. `feature-delta.md`'s own "rides
  multitenancy's inherited 300 rps headroom" claim does not hold — that
  figure is from a load-test run that itself breached its own 500ms p95
  gate (`p(95)=923ms`), root-caused to the same class of row-lock
  contention this feature adds more of. Requires a real cross-tenant load
  test that does not exist yet (DEVOPS follow-up, out of scope here). Full
  narrative: ADR-015 Amendment 2; `brief.md` § System Architecture,
  "Concurrency estimate — corrected" and "Lock-contention headroom — open
  risk."

## Architecture Summary

No new top-level component, no new deployable, no new host. A
`TransferCoordinator` type joins the existing `internal/app/` component
alongside `Ledger` (own file, own goroutine/ticker lifecycle, not folded
into `Ledger` itself — it owns a scheduler dependency no other use case
needs). Three new driven ports (`TenantLinkRepository`,
`CounterpartyAliasRepository`, `TransferStateRepository`), each
`UnitOfWork`-scoped, mirroring `TenantRepository`'s existing
read-then-decide-then-write shape. Five new/extended HTTP routes:
`POST /tenant-links`, `DELETE /tenant-links/{link_id}` (operator-only,
reused `requireOperatorKey`), `POST /counterparties` (tenant-key-only,
reused `requireTenantKey`), `POST /transfers` (discriminated body, same
tenant-key-only group), `GET /transfers/{transfer_id}` (new dual-party
middleware). Three new small tables plus one seeded sentinel tenant row —
no new store, no sharding, no new credential class. Full C4 Container
annotation (the ticker drawn inside the existing deployable, no new box) and
contract-shape classification per component: `docs/product/architecture/brief.md`
§ Inter-tenant transfer.

## Reuse Analysis

Mandatory Reuse Analysis pass run against both the base and `multitenancy`
reuse tables before any new component was finalized (full table:
`docs/product/architecture/brief.md` § Reuse Analysis, `inter-tenant-transfer`
pass). Summary of the load-bearing reuse decisions, each challenged rather
than assumed:

| Existing component | Decision | Why (not just "it was available") |
|---|---|---|
| `requireOperatorKey` | EXTEND (two more routes) | Byte-identical admin gate US-1 needs |
| `requireTenantKey` | EXTEND (two more routes) | `POST /transfers` already in this group (DDD-23); `POST /counterparties` joins it |
| `bearerToken`/`isOperatorKey`/`hashBearerToken` | EXTEND | The new `requireTransferParty` middleware's identity-resolution step is identical to every existing middleware's |
| `IdempotencyStore` | EXTEND (synthesized keys) | The central reuse decision — I7's unique-constraint machinery, applied to every leg and reversal, instead of a bespoke dedup mechanism |
| `Post` (domain, unmodified) | EXTEND (called 3-5x per transfer lifecycle) | Zero domain-core change; the entire cross-tenant/retry/reversal mechanism is application-layer composition |
| `OpenAccount`/`AccountRepository.Create` | EXTEND (get-or-create calling convention, not a new port) | Bootstrap is internal/idempotent, a different meaning of "already exists" than the caller-facing `POST /accounts` conflict `OpenAccount` guards against — handled by checking first, not by teaching the domain function a second meaning |
| `IDGenerator`, `TenantRepository.ByID` | EXTEND | Same reuse shape as multitenancy's own `tnt_`/`tk_` prefix and courtesy-check patterns |

**Genuinely new, justified by "no existing alternative"**: `TransferCoordinator`
(no saga/orchestration component exists anywhere in this codebase — the
Brownfield finding in feature-delta.md confirmed this directly);
`requireTransferParty` (no existing middleware can decide a per-resource,
two-specific-tenant authorization question — `requireTenantKeyOrOperatorKey`
grants "any tenant," which is structurally wrong here, per ADR-016);
`TenantLinkRepository`/`CounterpartyAliasRepository`/`TransferStateRepository`
(no persistence exists for any of the three new record types).

**Outcome Collision Check**: `nwave-ai outcomes check-delta
docs/feature/inter-tenant-transfer/feature-delta.md` was attempted and could
not be run — this install's `nwave-ai outcomes` tooling is known-broken
(missing its own packaged `schema.json`; the existing
`docs/product/outcomes/registry.yaml` header already documents the sibling
`register` command failing the identical way). Fell back to a manual diff
against `registry.yaml`'s fourteen existing rows (OUT-1 through OUT-14): no
collision. Every existing outcome names an intra-tenant operation or
invariant (posting, balance, entries, trial balance, tenant provisioning,
I1/I4/I7/I8/I9/I10); this feature's own candidate outcomes (authorize/revoke
a tenant-pair link, register an alias, send/trace a cross-tenant transfer,
I11) name a disjoint set of operations and one new invariant number, with no
overlapping `artifact` path and no restated `summary`. Registry population
itself remains DISTILL-owned per the registry's own header comment; this
check only confirms no collision exists to resolve before DISTILL writes the
new rows.

## Technology Stack

No new dependency. `time.Ticker` (Go stdlib) for the retry scan — rejected
alternatives: `robfig/cron`, `asynq`, `machinery` (job-scheduling libraries;
unneeded machinery for a fixed single-instance scan on one instance, still
true after the re-derived concurrency bound below — none of these
libraries' value propositions apply to a single-instance service regardless
of scale), and a message broker (same rejection DISCUSS itself already
named, reaffirmed here). `sha256`/`crypto/rand`-backed `IDGenerator`, `pgx`
v5, `chi` v5, `golang-migrate` — all existing, unchanged. No new credential
type, no new database role. The existing OPS-5 Prometheus exposition (`GET
/metrics`) is extended with two new gauges (§ Constraints Established) — no
new HTTP port, no new authentication boundary.

## Constraints Established

- `POST /transfers` (cross-tenant variant) is contractually `pending`-only in
  its own response; `settled`/`retrying`/`reversed` are only ever observed
  via `GET /transfers/{transfer_id}`. DISTILL must not author a scenario
  asserting `settled` on the `POST` response itself.
- Retry budget is fixed at N = 5 with 1s/2s/4s/8s (+jitter) backoff — not
  configurable in this release.
- `transfer_not_found` is outside the sealed `domain.ViolationKind` taxonomy
  and outside the `exhaustive` linter's coverage — DISTILL owns its own
  explicit test coverage for this wire mapping, named so it is not mistaken
  for compiler-enforced.
- `UPDATE` on `transfer_state`/`tenant_links` is a deliberate, bounded
  divergence from OPS-10's append-only posture, scoped explicitly to
  coordinator/authorization process state, never to `entries`/`transactions`.
- Leg 2 of every cross-tenant transfer is scoped to the reserved
  `tnt_platform` tenant identity, never to either business tenant's own
  `tenant_id` — I8's construction-time check is unmodified and continues to
  apply to this scope exactly as to any other.
- Crash-recovery latency for any non-terminal transfer (including one whose
  Leg 2 was never attempted before the crash) is bounded by `lease_duration`
  (15s) plus one ticker interval (1s), ≈16s worst case — not indefinite, and
  not conditional on the transfer having already recorded a failed attempt.
  DISTILL must author a scenario proving this for the never-attempted case
  specifically (§ For Acceptance Designer, "Mandatory crash-recovery
  scenario"), not only for a transfer already at `retrying`.
- Worst-case wall-clock time from `POST /transfers` to a terminal state
  (`settled` or `reversed`) is ≈204 seconds (≈3.4 minutes) — a deliberately
  pessimistic bound assuming every attempt takes the full 10s timeout;
  typical case (per US-3's own domain examples) is seconds to tens of
  seconds. Assumes a compensating reversal itself succeeds within its own
  retry budget — the case where reversal itself exhausts its budget with no
  further fallback is a named, unresolved residual gap, smaller in scope
  than the lock-contention open risk below.
- `TransferStateRepository.ClaimDue` is discovery-only and batch-limited
  (`batchLimit = 100`); the actual claim is `ClaimOne`, single-row, called
  by `attemptLeg`. A crafter test obligation distinct from the crash-
  recovery one: seed more than `batchLimit` due rows, run one
  `processDueTransfers` tick, assert exactly `batchLimit` were claimed and
  the remainder are claimed on the next tick.
- **Open risk, explicitly not resolved by this DESIGN pass**: whether the
  platform-as-hub account model's lock-contention/hot-set concentration
  fits within real cross-tenant throughput headroom is unquantified.
  `feature-delta.md`'s "no new performance NFR" claim does not hold as
  stated (§ System Architecture, "Lock-contention headroom"). DISTILL and
  DEVOPS should treat this as an open item requiring a dedicated
  cross-tenant load test, not a settled fact — no number is asserted here
  in its place.

## Upstream Changes

- No SSOT file outside `docs/product/architecture/` and
  `docs/feature/inter-tenant-transfer/design/` was written by this leg.
- `docs/product/architecture/brief.md`: § Application Architecture's
  top-level Component decomposition, Driving ports, Driven ports, Refusal
  taxonomy, and Reuse Analysis tables extended in place (cumulative tables,
  not per-feature subsections, mirroring how the `multitenancy` pass
  extended them); new § Inter-tenant transfer subsection added; § For
  Acceptance Designer extended with this feature's own port/credential
  table and DISTILL obligations.
- Two new ADRs: `adr-015-transfer-coordinator-persistence-and-execution.md`,
  `adr-016-dual-party-transfer-authorization.md`.
- `docs/product/outcomes/registry.yaml`: not written by this leg (DISTILL-
  owned); Outcome Collision Check run and recorded above, no collision
  found.

## Peer Review

Trigger evaluated against nw-design SKILL.md's criteria: this leg introduces
both a first-ever background/async coordinator *and* a new authorization
boundary (`requireTransferParty`, US-5's isolation proof) — the same
combination (novel pattern + security boundary) that triggered review for
the `multitenancy` credential work (`adr-012`). Review triggered:
`nw-solution-architect-reviewer` (haiku), iteration 1.

**Verdict: conditionally_approved. 0 critical, 1 high, 4 medium issues.**
Both scrutiny points held up: the sync/async contract was confirmed
consistent with slice-02's UAT, and `requireTransferParty` was confirmed to
satisfy US-5's "no distinguishable signal" requirement by construction (one
database read, one refusal call, for both the nonexistent-id and
wrong-party-tenant cases). All Q1-Q4 priority-validation checks passed
(YES/ADEQUATE/CORRECT/JUSTIFIED); reuse-analysis rigor and contract-shape
classification were both called out as strengths, not gaps.

Issues addressed, iteration 1 (no re-review needed — all were
documentation/specificity gaps in an otherwise-sound design, not structural
concerns):

- **HIGH — `IdempotencyStore.Lookup`'s return shape unverified.** Verified
  directly against `internal/app/ports/ports.go:148,163-167`:
  `Lookup(ctx, key) (Claim, bool, error)` already returns
  `Claim.TransactionID` on a hit — no interface change needed. Added as an
  explicit "verified, not assumed" note in `brief.md` and ADR-015 point 4.
- **MEDIUM — no concrete per-attempt timeout or retry-claim lease value.**
  Settled: 10-second per-`attemptLeg`-call timeout (mirrors the console's
  own 8s convention); 15-second claim lease (the 10s timeout plus a 5s
  margin). Added to `brief.md` and as ADR-015 point 9.
- **MEDIUM — `tenant_links`/`counterparty_aliases` schemas not shown.**
  Added, mirroring `transfer_state`'s own illustrative-SQL treatment.
- **MEDIUM — test-ownership for `SendTransfer`'s two-part delta
  ambiguous.** Clarified in § For Acceptance Designer: Leg 1's own posting
  contract is the existing, unmodified `PostTransfer` coverage; DISTILL's
  new obligation is specifically the `transfer_state` row's existence and
  initial shape.
- **MEDIUM — dual idempotency mechanisms need distinguished scenario
  coverage.** Clarified in § For Acceptance Designer: transfer-level replay
  (the `UNIQUE(tenant_id, idempotency_key)` constraint) and per-leg replay
  (`IdempotencyStore`, synthesized keys) answer different questions and
  need separately-authored scenarios, not one scenario assumed to cover
  both.

Handoff readiness per the reviewer: **READY**, conditional on the above,
all now closed.

### Iteration 2 — crash-recovery gap found in user review, post-approval

After iteration-1 approval, the user reviewing this handoff identified a
real gap the iteration-1 review did not catch: the retry-due scan's
`status = 'retrying'`-only predicate left a crash between `SendTransfer`'s
commit and the inline goroutine's first Leg 2 attempt permanently
unrecovered (§ Key Decisions Amendment, above; full narrative: ADR-015
Amendment). This is a correctness-of-money-movement fix, not a
documentation clarification — it changes the driven-port interface
(`ClaimDueForRetry` → `ClaimDue`), the schema (partial index predicate,
`next_attempt_at` nullability), and adds a new mandatory DISTILL scenario —
so it clears this project's own bar for re-review ("re-run peer review if
the fix is substantive enough to warrant it"). `nw-solution-architect-reviewer`
(haiku) re-invoked, scoped to the fix specifically (a delta review, not a
full re-review — iteration 1's already-approved ground, e.g. the
sync/async contract, account bootstrap, dual-idempotency mechanism, and
component boundaries, was explicitly out of scope).

**Verdict: approved. 0 critical, 0 high, 0 medium, 0 low issues.** The
reviewer independently verified: the crash gap as described was real and
the fix closes it completely, including the previously-undiscussed
degenerate sub-case where the goroutine never spawns at all (covered for
free — the row is immediately due at insert time, no lease ever placed, so
the very next ticker tick, ≤1s, claims it; no separate handling needed);
the double-processing race guarantee is not merely preserved but
strengthened (every attempt, inline or ticker-driven, now goes through the
identical mandatory claim, where previously the inline path's first attempt
was unclaimed); the bounded-recovery-latency derivation is correct for both
crash windows (≤16s mid-attempt, ≤1s pre-first-claim); the rejected
`status = 'retrying'`-from-creation alternative's reasoning holds; and no
stale references to the old `ClaimDueForRetry` name or the old
`'retrying'`-only predicate remain anywhere across `brief.md`, ADR-015, or
this document. Handoff readiness: **READY**, conditional only on DISTILL
actually authoring the mandatory crash-recovery scenario (§ For Acceptance
Designer) — flagged by the reviewer as the one obligation not to skip,
since a scenario testing recovery from an already-`retrying` row would not
have caught the original gap.

### Iteration 3 — five paper-level fixes from the second, scale-focused review

Scoped narrowly, per standing discretion on this feature: the five fixes
above are mostly documentation/derivation/additive (worst-case number,
re-derived estimate, new metrics, open-risk recording), which would not on
their own clear this project's re-review bar — but one of them
(`ClaimDue`→`ClaimOne`/`ClaimDue` split) is a real interface-shape change a
crafter will implement, and another (the lease/backoff write-order
reconciliation) is a genuine behavioral clarification where getting the
"which write wins" direction wrong would reintroduce a bug of the same
class iteration 2 just closed. Judged sufficient to warrant a light,
delta-scoped verification pass rather than none. `nw-solution-architect-reviewer`
(haiku) re-invoked, scoped to items 1 and 3 specifically (the two with real
behavioral/interface content); items 2, 4, 5, and the open-risk recording
are documentation/derivation and not separately re-reviewed.

**Verdict: approved. 0 critical, 0 high, 1 low.** The reviewer independently
verified: the lease-vs-backoff sequencing is explicit and self-consistent
(the second, outcome-recording write happens in the same database
transaction as recording the leg's outcome, "before `attemptLeg` returns,"
not left to inference); the worst-case 204s figure correctly uses the 10s
timeout and the 1/2/4/8s backoff ladder in its arithmetic, not the 15s
lease; the `ClaimOne`/`ClaimDue` split is race-free (both the inline
goroutine and the ticker's dispatch loop converge on `ClaimOne`'s own
atomicity regardless of how a row was discovered — splitting discovery from
claiming does not reopen the double-processing race, since `ClaimDue` never
writes); `batchLimit = 100`'s sizing against the re-derived ≈512-transfer
extreme bound is sound (drains a pathological backlog in ≈6 ticks); and no
stale references to the old, unbounded, single `ClaimDueForRetry` signature
remain. One low-severity gap, closed immediately: `ClaimOne`'s doc comment
now states explicitly that `ok=false` (the losing side of a claim race)
means `attemptLeg` returns immediately without attempting anything — a
normal outcome, not an error, previously only inferable. Handoff readiness:
**READY**, no conditions outstanding.

## Next Wave

**Handoff to**: `nw-acceptance-designer` (DISTILL wave). External
integrations: none (no third-party API is introduced by this feature — same
finding as `ledger-core-console`'s own confirmation); no contract-testing
annotation needed for `nw-platform-architect`.

**Open risk carried forward to DEVOPS, not resolved here**: the
lock-contention/hot-set headroom question (§ Constraints Established, §
Key Decisions Amendment 2) requires a dedicated cross-tenant load test
before `feature-delta.md`'s "no new performance NFR" claim can be called
settled. `nw-platform-architect` should treat this as an open item for its
own DEVOPS-wave scope, not as pre-cleared by this DESIGN pass. The two new
Prometheus gauges (§ Technology Stack) are the operational instrument that
would surface this in practice once a hosted environment or a dedicated
load-test profile exists to observe them under real traffic.
