# ADR-002 — Sufficient funds enforced with row locks in deterministic order

**Status**: Accepted · 2026-08-18 · Feature: ledger-core · Decision: DDD-6

## Context

I4 requires that no wallet account balance goes negative. The naive
implementation — read the balance, check it, then write — passes twice under
concurrency: two requests both read a sufficient balance before either writes.

Slice 02's acceptance criterion is explicit: given a wallet holding 100 and 20
concurrent transfers of 100 each, exactly one succeeds. The mechanism has to be
provable by a test that reliably reproduces contention.

## Decision

Inside the posting transaction, acquire `SELECT … FOR UPDATE` row locks on every
account the transaction touches, **in ascending account-id order**, before
reading balances or writing entries.

Deterministic ordering is what makes this deadlock-free: two concurrent postings
touching the same pair of accounts request the locks in the same sequence, so one
waits rather than both holding half of what the other needs.

## Alternatives considered

**SERIALIZABLE isolation** — let PostgreSQL detect the conflict and abort the
loser. Architecturally cleaner: the guarantee lives in the engine rather than in
application discipline. Rejected on testability, which ranks second among quality
attributes. Serialization failures require an application retry loop, which means
the slice-02 demo has to explain "some of these were retried" rather than
"exactly one succeeded." The failure mode is also probabilistic, making the race
test flakier to assert on.

Worth revisiting if lock contention ever becomes a throughput problem. It is a
better answer at scale; it is a worse answer for demonstrating the invariant.

**Optimistic concurrency with a version column** — compare-and-swap on
`accounts.version`. Rejected: the retry logic is hand-written, and the failure
mode under contention is subtler to test than a lock wait. No contention benefit
that matters at current volume.

**Application-level mutex** — rejected outright. Correctness would depend on
running exactly one process, which is not a property worth building on.

## Consequences

- Lock ordering is a rule the posting code must never break. It belongs in one
  place — the `AccountRepository` adapter — and must be covered by a test that
  posts two transfers touching the same accounts in opposite argument order.
- Locks are held for the duration of the posting transaction, so transactions
  must stay short. No external calls inside the transaction, ever.
- The slice-02 race test is deterministic in outcome (exactly one success) even
  though timing is not.
- Throughput on a hot account is bounded by lock wait. Acceptable, and measurable
  before it matters.
