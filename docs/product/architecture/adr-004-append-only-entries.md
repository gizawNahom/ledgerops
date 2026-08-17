# ADR-004 — Entries are append-only, enforced by the database

**Status**: Accepted · 2026-08-18 · Feature: ledger-core · Decision: D7 / DDD-9

## Context

Raised during the DISCUSS wave while discussing how proof-of-balance would scale.
Every scaling strategy available later — checkpointed verification, period
closing, hash-chain tamper evidence — assumes that history does not change. A
checkpoint asserting "as of entry N the books balanced" is worthless if entry
N-1000 can be edited afterwards.

Immutability cannot be retrofitted. Once history has been mutated, there is no
way to establish what the correct prior state was.

## Decision

Rows in `entries` are **never updated or deleted**. Enforced at the database
level — revoked `UPDATE`/`DELETE` privileges for the application role, plus a
rule or trigger so the constraint holds regardless of connecting role.

Corrections are made by posting a **compensating transaction**, which is what
double-entry bookkeeping has always done: you do not erase a mistake, you post a
reversal that cancels it.

Application-level convention was explicitly rejected as insufficient. A rule that
depends on every future code path remembering it is not a guarantee.

## Alternatives considered

**Soft delete** — a `deleted_at` column with queries filtering it out. Rejected:
every query becomes a place to forget the filter, and a forgotten filter in a
ledger silently changes balances. It also does not deliver immutability, since
the flag itself is mutable.

**Versioned rows** — supersede an entry by inserting a new version. Rejected:
this is mutation with extra steps. Determining "the current set of entries"
becomes a query with a subselect, and I1 gets harder to state and to check.

**Application-level convention** — no repository method that updates entries.
Rejected as the weakest option: correctness would depend on discipline, and the
whole point of this feature is that money invariants should not depend on anyone
remembering anything.

## Consequences

- Slice 01's first migration must set this up. It is the one decision that cannot
  be deferred to a later slice.
- The slice-04 corruption demo becomes *more* compelling: performing it requires
  deliberately defeating a database constraint, so the demo narrative is "I had
  to disable a trigger to break this" rather than "I ran an UPDATE."
- Reversals and refunds, when they arrive, are already the natural shape rather
  than a special case.
- Storage grows monotonically. Acceptable, and the precondition for period
  closing if it ever matters.
- Test fixtures cannot clean up by deleting entries. They truncate or use
  per-test schemas instead.
