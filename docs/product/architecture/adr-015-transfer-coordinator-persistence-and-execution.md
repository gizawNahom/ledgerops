# ADR-015: Transfer coordinator persistence and execution model —
persisted state row, inline-then-ticker retry sharing one function, and a
reserved-tenant account-bootstrap scope

**Status**: Accepted, 2026-09-07
**Owner**: nw-solution-architect (DESIGN wave, `inter-tenant-transfer`,
application leg)

## Context

`inter-tenant-transfer`'s DISCUSS wave named D8 (orchestration-style,
per-transfer coordinator; retry-until-settled primary, bounded compensate
fallback) and left two DESIGN pre-requisites open: #2 (coordinator state
persistence shape) and #5 (sync vs. async settlement contract). The system
architecture leg (`brief.md` § System Architecture / Inter-tenant transfer)
confirmed the coordinator stays in-process — no new deployable, no message
broker — and narrowed #5 to "legs 2/3 attempt inline on the first try, and
only a failed first attempt hands off to a background retry loop," without
settling the concrete request/response contract. The domain leg
(`adr-014-tenant-link-and-counterparty-alias.md`) confirmed `Transfer` is
not a domain aggregate and that reversal is a specific use of the unmodified
`Post` function, explicitly handing this architect the coordinator's
persistence shape, the double-application guard's physical mechanism, and
(newly surfaced during this leg's own design work) the account scope Leg 2's
"platform ledger" movement actually lives under — D7 names "one platform
account per tenant" without naming whose tenant scope those accounts share,
which matters because I8/`Post` requires every account touched by one call
to share a single `tenant_id`.

## Decision

**1. `POST /transfers` (cross-tenant variant) always responds `pending`
after Leg 1, never blocking for Legs 2/3 and never itself reporting
`settled`.** This settles Pre-requisite #5 concretely. Leg 1 is synchronous,
using the existing unmodified single-tenant `PostTransfer` path. The moment
it commits, the handler responds and returns. A goroutine spawned by that
same handler, detached from the request's own context, immediately attempts
Leg 2 then Leg 3 via `attemptLeg` — the identical function the retry ticker
later calls for every subsequent attempt. One function serves both the
"inline" first attempt and every background attempt, so the two are
structurally incapable of behaving differently from each other.

**2. Coordinator state is a new Postgres table, `transfer_state`, one row
per transfer, updated in place — not an in-memory-only design.** This
settles Pre-requisite #2. Execution is in-process (goroutines, a
`time.Ticker`); state is persisted, which is what makes a crashed process's
in-flight, non-terminal transfers resumable on the next process's startup:
the ticker's own steady-state scan query is the same query a restart needs,
so no separate recovery path exists to fall out of sync with the
steady-state one. **This resumability claim covers every non-terminal row,
not only ones that had already reached `retrying` — see the Amendment
below, which corrects an initial version of this decision that only made
the claim true for the `retrying` subset.**

**3. `UPDATE` is granted to `ledgerops_app` on `transfer_state` (and on
`tenant_links`, for revocation) — a deliberate, explained divergence from
OPS-10's no-`UPDATE` posture elsewhere.** D7's append-only discipline
governs the ledger's own value-movement history (`entries`, `transactions`)
— facts that must never be rewritten. `transfer_state` is the coordinator's
own mutable process state (D8's "saga/process manager" vocabulary),
structurally the persisted form of an in-memory saga's own fields. Every
fact it ever points at (a posted Transaction, its Entries) remains
append-only, untouched by any `UPDATE` here.

**4. Retry-safety reuses the existing `IdempotencyStore` port, unmodified,
with synthesized keys — not a new dedup mechanism.** Verified directly
against `internal/app/ports/ports.go:148,163-167`: `Lookup(ctx, key) (Claim,
bool, error)` already returns `Claim.TransactionID` on a hit — exactly the
value a short-circuited retry needs to recover the already-posted
transaction id without re-calling `Post`. No interface change is required. Leg 1 claims the
caller's own `Idempotency-Key` header through the existing, unmodified
single-tenant path. Legs 2 and 3 claim `{Idempotency-Key}:leg2` /
`{Idempotency-Key}:leg3`. Compensating reversals claim
`{Idempotency-Key}:leg2:reverse` / `{Idempotency-Key}:leg1:reverse`. Every
retry of a leg claims the *same* key each time, so a retry that finds the
key already claimed (a prior attempt posted but the coordinator crashed
before recording it) resolves via `IdempotencyStore.Lookup` instead of
re-posting. `transfer_state` carries a separate, simpler `UNIQUE
(tenant_id, idempotency_key)` constraint for *request*-level replay (has
this `POST /transfers` call been made before), answering a different
question than the per-leg mechanism above and intentionally not merged with
it.

**5. Fixed retry budget N = 5, exponential backoff 1s/2s/4s/8s + ~20%
jitter, not configurable.** N = 5 matches US-3's own domain example 3
("within a 5-attempt budget"); the backoff shape is this project's own
documented Retry-with-Exponential-Backoff pattern. Simplest-solution-first:
no story asks for tuning, so none is built.

**6. A reserved internal tenant, `tnt_platform`, seeded once by migration
(mirroring `tnt_legacy_seed`'s own precedent), owns one "platform mirror"
account per business tenant (`platform-{tenant_id}`).** This settles the
scope question D7 left open: Leg 2 (`Platform[sender] → Platform[receiver]`)
is a legal `Post` call only if both accounts share one `tenant_id` that is
neither business tenant's own — `tnt_platform` is that shared scope. Each
business tenant separately owns its own `settlement` account, under its own
`tenant_id`, used by Legs 1 and 3. Both account types are System-kind (D7),
so I4's wallet floor never applies to them.

**7. Both account types are bootstrapped idempotently, inside
`SendTransfer`, on first use — not at `AuthorizeTenantPair` time (slice
01).** Slice 01's own scope explicitly excludes "anything that posts a
transfer," and a link may stand authorized for a long time before either
party transacts (D9). `SendTransfer` checks existence via
`AccountRepository.Get` before Leg 1's own lock acquisition; on not-found,
it calls `OpenAccount`/`Create` directly, treating "already exists" as the
expected successful case rather than the caller-facing conflict
`OpenAccount`'s existing `alreadyOpen` parameter exists to refuse.

**8. `time.Ticker` (stdlib), no new dependency.** One goroutine, one
1-second interval, wired at `cmd/api/`'s composition root alongside the
existing `probeStartup`/`NewRouter` wiring, claims due rows via an atomic
`UPDATE ... WHERE status IN ('pending', 'retrying') AND next_attempt_at <=
$2 RETURNING *` (a lease, not a bare `SELECT`) and calls `attemptLeg` for
each — see the Amendment below for why the predicate covers `'pending'`
too, not `'retrying'` alone.

**9. Two concrete timing constants, not left to the crafter to invent.**
Every `attemptLeg` call — inline or ticker-driven — runs under its own
10-second context timeout (matching this project's existing "generous local
co-located-process latency budget" convention, the console's own 8s
rationale). The retry-due claim's lease is 15 seconds (the 10s attempt
timeout plus a 5s scheduling margin), independent of the 1s ticker interval.

## Consequences

- `internal/app/transfer_coordinator.go` (new file) joins `internal/app/` —
  no new top-level component.
- `transfer_state`, `tenant_links`, `counterparty_aliases` are new tables;
  `tnt_platform` is a new seeded row in the existing `tenants` table.
- `TenantLinkRepository`, `CounterpartyAliasRepository`,
  `TransferStateRepository` are new driven ports, each `UnitOfWork`-scoped.
  `TransferStateRepository.ClaimOne` (split out of an earlier, single
  `ClaimDueForRetry` per Amendment 2 below) is the mandatory single-row
  claim every leg attempt — inline or ticker-dispatched — must go through;
  `ClaimDue` is the ticker's own separate, batch-limited discovery step.
- No new external dependency, no new credential class, no new deployable.
- The double-application guard for reversals is I7's own mechanism,
  re-applied — no new invariant number, per ADR-014's own framing.
- Recovery latency after a crash is bounded (≤ `lease_duration` + one tick
  interval, ≈16s) uniformly for every non-terminal transfer, including one
  whose Leg 2 was never attempted even once before the crash — not only for
  transfers that had already recorded a failure. See Amendment.

## Amendment (2026-09-07, same session) — the retry-due scan's original
scope left a crash-recovery gap

Point 8's original wording (and the schema's original
`transfer_state_retry_due_idx ON transfer_state (next_attempt_at) WHERE
status = 'retrying'`) claimed the ticker's steady-state scan query and its
startup-recovery query were "the same query," and treated that as
sufficient for the resumability claim in point 2. That claim was only true
for rows that had already reached `status = 'retrying'` — i.e., rows whose
Leg 2 (or Leg 3) had already failed at least once. It was not true for a
row still at its initial `status = 'pending'`: `SendTransfer` inserts that
row (in the same transaction as Leg 1's commit) and only *afterward* spawns
the detached goroutine that makes Leg 2's first attempt. A process crash in
that window — after Leg 1's money has moved, before the goroutine's first
status write — left the row permanently outside the `'retrying'`-scoped
scan. Nothing would ever revisit it: not the ticker, not a restart. This is
the same failure class ("money moved, transfer never completes") US-4's
reversal path exists to prevent for the *retry-exhausted* case, but
occurring upstream of retry exhaustion, where no retry had even been
counted.

**Fix**: the claim mechanism is made unconditional over every non-terminal
row, and the inline goroutine's first attempt is routed through the
identical claim every ticker-driven attempt uses — there is no longer an
unclaimed "pre-claim" phase for a crash to land in.

1. `next_attempt_at` is set to the row's own creation instant at `INSERT`
   time (never left `NULL`, which would never satisfy a `<= now()`
   comparison and would silently exclude the row forever — the identical
   bug class one level down).
2. The partial index and every claim query cover `status IN ('pending',
   'retrying')`. `'pending'` and `'retrying'` differ only in whether a
   failure has been recorded yet — a caller-facing reporting distinction
   (US-3's own "the sender sees `retrying`... when a leg stalls"), not a
   scan-eligibility one.
3. `attemptLeg` performs the atomic claim (point 8's `UPDATE ... RETURNING
   *`) internally, and this becomes the *only* permitted way to attempt a
   leg — inline or ticker-driven. (Amendment 2, below, later splits this
   single-row claim out as its own named method, `ClaimOne`, distinct from
   the ticker's own batch discovery step, `ClaimDue` — a refinement made
   necessary by adding a batch cap, not a reversal of this point.)

This closes the crash gap and the double-processing race (point 8's own
original concern) with one mechanism instead of two: a crash at any point
after the row exists leaves it re-claimable after at most one
`lease_duration`, because no phase of a transfer's lifecycle — not the
initial insert, not any individual attempt — was ever exempt from the
claim.

**Rejected alternative — insert the row at `status = 'retrying'` from
creation.** A one-line fix that sidesteps the scan-predicate question
entirely. Rejected: it corrupts `'retrying'`'s own meaning (it should mean
"a failure was recorded," not "default starting state," per US-3's own
ubiquitous language) and risks a `GET` response reporting `retrying` before
Leg 2 has been attempted even once — a distinction no UAT scenario licenses
away and a future scenario could reasonably assert against. Decoupling the
caller-facing status label from the scan-eligibility predicate (the fix
adopted above) costs one broadened `WHERE` clause and keeps both concerns
honest, rather than trading a real correctness gap for a cheap fix that
introduces a new, subtler one.

Full narrative and the bounded-recovery-latency reasoning:
`docs/product/architecture/brief.md` § Inter-tenant transfer / "Crash
recovery for a transfer that never reached `retrying`."

## Amendment 2 (2026-09-07, same session) — four cheap, paper-level gaps
found by a second, scale-focused review pass

The user commissioned a second review of the System Architecture leg
(`nw-system-designer-reviewer`), specifically to check whether this
feature's own throughput claim holds up under real cross-tenant load. That
review returned `revisions_needed` with several findings; the user directed
a specific subset — the cheap, paper-level fixes below — applied now, and
the larger open question (lock-contention headroom, below) left as a named,
unquantified open risk rather than resolved on paper.

**1. Backoff-vs-lease conflict, previously unstated.** Two writes to
`next_attempt_at` coexist (the claim's `+lease_duration` and the documented
backoff ladder's `+backoff(attempt)`), and nothing previously said which one
governs the row's resting value between attempts — a real ambiguity, since a
reader could reasonably conclude effective retry gaps were 15s/15s/15s/15s
(the lease value), not the documented 1/2/4/8s ladder. Resolved: the lease
write is a short-lived, in-flight-only placeholder, always overwritten by a
second write — setting the real backoff value — before `attemptLeg` returns
in every non-crash outcome. The lease value is observed at rest only in the
crash case Amendment 1 already covers. Full reasoning:
`docs/product/architecture/brief.md` § Inter-tenant transfer, "Reconciling
the claim lease and the backoff ladder."

**2. No stated worst-case time-to-terminal.** The ≈16s bound above is a
*per-attempt* recovery bound, not a *per-transfer* one — no artifact stated
the number a US-3/US-4 polling caller actually needs. Computed: **≈68
seconds** per leg's worst-case exhaustion (5 attempts × 10s timeout + the
backoff ladder, up to ~18s with jitter), and **≈204 seconds (≈3.4 minutes)**
for the worst overall case (Leg 3 exhausts, then both compensating
reversals — Leg 2, then Leg 1 — each also exhaust their own retry budget
before finally succeeding), against a typical case of a few seconds to tens
of seconds for an ordinary simulated-transient-fault failure. This bound
assumes a reversal itself eventually succeeds within its own retry budget —
what happens if a reversal exhausts its own budget with no further fallback
is a separate, smaller residual gap, named but not resolved here. Full
derivation: `docs/product/architecture/brief.md` § Inter-tenant transfer,
"Worst-case wall-clock time from `POST /transfers` to a terminal state."

**3. `ClaimDue` had no batch limit.** Its original signature claimed every
due row in one call — unbounded, and after any process downtime (or under
ordinary load, now that Amendment 1 broadened scan scope to `status IN
('pending','retrying')`), one tick could claim the entire non-terminal
backlog. Fixed by splitting the single-row atomic claim (renamed
`ClaimOne`, used by `attemptLeg` for a specific, already-known
`transfer_id`) from a separate, read-only, batch-limited discovery step
(`ClaimDue(ctx, now, batchLimit)`, used only by `processDueTransfers`).
Rows beyond `batchLimit` are left unclaimed, remaining due for a later
tick. `batchLimit = 100`, sized against the re-derived concurrency estimate
below. Full interface: `docs/product/architecture/brief.md` § Inter-tenant
transfer, `TransferStateRepository`.

**4. No operational visibility into any of the above.** Added two
Prometheus gauges — `ledgerops_transfer_state_nonterminal_count` and
`ledgerops_transfer_state_oldest_next_attempt_age_seconds` — refreshed by
`processDueTransfers` each tick, extending the existing `Metrics` component
(OPS-5, `GET /metrics`) rather than a new HTTP port: cheaper, no new
authentication boundary, matches this codebase's own `entry_count`/
`elapsed_ms` precedent (`GET /health/trial-balance`). Full detail:
`docs/product/architecture/brief.md` § Inter-tenant transfer, "Operational
visibility."

**5. Concurrency estimate re-derived.** The original "order of 1-10... not
thousands" figure (§ System Architecture, this brief) was tenant-count-
derived, the wrong methodology for a queueing question, and was silent on
which population it covered. Re-derived via Little's Law (L = λ × W),
scoped to the post-Amendment-1 population (every `pending`-or-`retrying`
row, not only `retrying` ones), using this project's own already-measured
`POST /transfers` p95 (923ms) and the `capacity` k6 profile's own measured
throughput ceiling (276.5 rps) as inputs — bounded above at **≈512**
concurrently non-terminal transfers under an extreme, almost-certainly-
unrealistic assumption (100% of the service's measured throughput ceiling
is cross-tenant traffic, sustained continuously), with the realistic figure
genuinely unmeasured (§ below). Full derivation:
`docs/product/architecture/brief.md` § System Architecture, "Concurrency
estimate — corrected."

**Explicitly not resolved by this amendment — an open, unquantified risk,
named plainly rather than left silently implied as settled.** The
reviewer's larger finding — whether the platform-as-hub account model's
lock-contention/hot-set concentration (3 transactions / 6 row locks per
cross-tenant transfer, funneled through the shared `tnt_platform`-scoped
mirror accounts, versus 1 transaction / 2 locks for an intra-tenant
transfer today) fits within real throughput headroom — is **not** answered
here. `feature-delta.md`'s own claim ("no new performance NFR, rides
multitenancy's inherited 300 rps headroom") does not hold: that 300 rps
figure is from a load-test run that itself breached its own 500ms p95
latency gate (`p(95)=923ms`), root-caused to the same class of row-lock
contention this feature's design adds more of
(`docs/feature/ledger-core/feature-delta.md` § OPS-12 amendment 2). This
requires a real cross-tenant load test that does not exist yet — DEVOPS
follow-up work, explicitly out of scope for this DESIGN-wave fix. No number
is invented in its place. Full record:
`docs/product/architecture/brief.md` § System Architecture, "Lock-
contention headroom — open risk."

## Rejected alternatives

- **`POST /transfers` blocks until settlement or a bounded window elapses**
  — rejected; makes happy-path latency a function of two more database round
  trips for no UAT-required benefit, and needs a second code path for the
  timeout case, reintroducing exactly the inline/background divergence risk
  point 1 above exists to avoid.
- **A job-scheduling library (`robfig/cron`, `asynq`, `machinery`) or a
  message queue** — rejected as unneeded machinery for a fixed 1-second scan
  over a table on one instance with no hosted environment; still true after
  Amendment 2's re-derived concurrency bound (≈512 non-terminal transfers at
  an extreme, almost-certainly-unrealistic ceiling, realistically far lower
  but genuinely unmeasured) — none of these libraries' actual value
  propositions (distributed queues, worker pools across machines) apply to a
  single-instance service regardless of which bound turns out to be closer
  to reality; `ClaimDue`'s own batch limit (Amendment 2, point 3) is the
  right-sized lever for bounding one tick's work, not a scheduling library.
- **Provisioning settlement/platform-mirror accounts at `AuthorizeTenantPair`
  time** — rejected; violates slice 01's own scope boundary and would create
  rows for authorized-but-never-used pairs.
- **A bespoke dedup table for leg/reversal retries** — rejected; would
  duplicate `IdempotencyStore`'s existing, already-tested unique-constraint
  mechanism for zero behavioral gain.
- **`OpenAccount`'s existing `alreadyOpen`-refuses-hard contract, called
  directly for bootstrap** — rejected; conflates a caller-facing conflict
  signal with an internal idempotent-bootstrap success case, which would
  force the coordinator to swallow a domain refusal it did not actually mean
  to trigger.

## References

- `docs/product/architecture/brief.md` § Inter-tenant transfer — full
  narrative, schema shape, retry table, C4 annotation.
- `docs/feature/inter-tenant-transfer/feature-delta.md` § Pre-requisites #2,
  #5; § Locked decisions D6-D9.
- `adr-014-tenant-link-and-counterparty-alias.md` — the domain-modelling
  decisions this ADR's persistence and execution model build on.
- `adr-013-multitenancy-migration-shape.md` — the `tnt_legacy_seed`
  reserved-tenant precedent this ADR's `tnt_platform` mirrors.
- `docs/feature/ledger-core/feature-delta.md` § OPS-12 amendment 2 — the
  row-lock-contention root cause and the measured `p(95)=923ms` figure
  Amendment 2's re-derivations build on.
- `nw-system-designer-reviewer`'s scale-focused review (2026-09-07,
  commissioned by the user, Opus) — the source of Amendment 2's five
  findings; full review artifact not separately filed, summarized in
  Amendment 2 and `docs/feature/inter-tenant-transfer/design/wave-decisions.md`.
