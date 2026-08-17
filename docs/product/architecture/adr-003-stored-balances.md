# ADR-003 — Balances are stored and updated atomically, not derived

**Status**: Accepted · 2026-08-18 · Feature: ledger-core · Decision: DDD-7

## Context

An account's balance can either be **stored** as a column and maintained on every
posting, or **derived** on read as `SUM(entries.amount)`.

This is the decision that most affects what slice 04 demonstrates. I3 states that
a stored balance equals the sum of its entries — an invariant that only has
meaning if a stored balance exists to disagree with the entries.

## Decision

`accounts.balance` is stored, and updated inside the same database transaction
that writes the entries. It is never updated by any other path.

I3 is deliberately **not enforced**. It is *verified* — by the slice-04
reconciliation query, which compares the stored balance against the computed sum
and reports any account where they differ.

## Alternatives considered

**Derive on read** — balance is always `SUM(entries.amount)`. Genuinely
attractive: it makes drift structurally impossible and renders I3 vacuously true,
eliminating an entire bug class rather than detecting it.

Rejected for two reasons. First, reads degrade with history, and every balance
read pays the cost. Second and more importantly, it removes the independent
cross-check. A ledger with two representations that *can* disagree, plus a
mechanism that notices when they do, is a stronger correctness story than one
representation that cannot be checked against anything. Slice 04 exists to
demonstrate exactly that, and derivation would leave it with nothing to detect at
the per-account level.

Note that corruption would still be caught under derivation, via I1 at the
transaction level — so this is a trade of one detection mechanism for another,
not a trade of safety for speed.

**Event sourcing with projected balances** — treat entries as an event log and
project balances into a read model. Rejected as machinery without a matching
need. The entry table is already an append-only log of immutable facts (D7),
which delivers the auditability benefit that usually motivates event sourcing.
Adding projections, rebuild logic, and eventual consistency would slow every
slice and complicate the invariant story. Revisit only if a genuine read-model
requirement appears.

**Materialized view** — Postgres refresh semantics make the staleness window
explicit but nonzero, so I3 would be *expected* to fail transiently. That
destroys the meaning of the slice-04 verdict.

## Consequences

- Every posting writes entries *and* updates balances atomically. There is no
  code path that writes one without the other, and that is a rule the
  `TransactionRepository` adapter owns.
- Drift is possible in principle. That is the point: slice 04 detects it, and the
  corruption demo depends on it.
- Balance reads stay O(1).
- The slice-04 verification query remains O(total history) by design (D9), with
  `elapsed_ms` reported so degradation is observed rather than assumed.
