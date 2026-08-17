# ADR-001 — Entries use signed int64 minor units

**Status**: Accepted · 2026-08-18 · Feature: ledger-core · Decision: DDD-5

## Context

An entry records one side of a value movement. Two questions had to be settled
together: how the *direction* of movement is represented, and how the *quantity*
is represented.

I1 requires that a transaction's entries sum to zero per currency. Whatever
representation is chosen has to make that assertion cheap and unambiguous,
because it is checked in the domain core, in the database, and in the slice-04
verification query.

## Decision

`entries.amount` is a **signed 64-bit integer in minor units** (cents). Money
leaving an account is negative; money arriving is positive. I1 becomes
`SUM(amount) = 0`.

Debit/credit terminology, if ever needed, is a presentation projection at the API
or console layer. It does not exist in storage.

## Alternatives considered

**Debit/credit columns** — positive amounts plus a direction enum, matching
accounting literature. Rejected: every aggregate query would need a `CASE` to
apply sign, and the I1 assertion becomes `SUM(CASE WHEN direction='debit' THEN
amount ELSE -amount END) = 0`. That is more surface for an error to hide in, and
the invariant check is the thing that must be beyond doubt. The accountant-facing
benefit is real but can be recovered as a projection.

**Decimal or numeric type** — Postgres `NUMERIC` avoids the minor-unit
convention. Rejected: Go has no native decimal, so this pushes a third-party
decimal type through the whole domain core, and comparisons and arithmetic
acquire library semantics. Integers in minor units are exact, comparable with
`==`, and hash predictably in tests.

**Floating point** — rejected without discussion. `0.1 + 0.2 != 0.3` is
disqualifying for money.

## Consequences

- I1 is a single SQL predicate and a single Go assertion.
- Currency must be stored alongside the amount; minor-unit scale is
  currency-dependent (JPY has none, most have two). Only one currency is in
  scope for now, but the column exists so I1's "per currency" phrasing stays
  honest.
- `int64` in minor units caps at roughly 92 quadrillion cents. Not a practical
  limit.
- Anyone reading the schema expecting debits and credits will need the ubiquitous
  language table in `brief.md`. Accepted cost.
