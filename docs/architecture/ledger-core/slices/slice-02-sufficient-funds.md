# Slice 02 — Sufficient funds

**Job**: J3 | **Invariant**: I4 | **Story**: US-2 | **WS strategy**: C (real local)

## Goal

Reject any transfer that would take a wallet account below zero, and hold that
rule under concurrent requests.

## Learning hypothesis

Disproves the naive read-then-write balance check. Under concurrency two requests
both read a sufficient balance and both commit, producing a negative balance.
This slice forces the transaction-isolation decision into the open, where it can
be tested rather than assumed.

## IN scope

- Sufficient-funds check enforced at posting time, inside the same database
  transaction as the write
- Account taxonomy: `wallet` accounts may not go negative; `system` accounts may,
  by design — the system account is the counterparty that funds enter from
- 422 response carrying `available` and `requested` so the caller can explain the
  rejection
- Concurrency demo: N simultaneous spends against a balance covering one

## OUT of scope

- Idempotency (slice 03)
- Holds, authorizations, or reserved balances
- Overdraft limits or credit lines

## Acceptance criteria

- [ ] Transfer exceeding available balance returns 422 and changes no state
- [ ] 422 body includes `available` and `requested`
- [ ] Given a wallet holding 100 and 20 concurrent transfers of 100 each, exactly
      one succeeds and 19 return 422 (I4)
- [ ] No wallet account balance is ever negative after any test run
- [ ] System accounts may go negative and this is asserted, not merely allowed
- [ ] The isolation level or locking strategy relied on is documented in the test
      that depends on it
- [ ] Property-based test: no generated sequence of transfers drives a wallet
      account negative
- [ ] `make demo-02` shows the rejection; `make race-02` runs the concurrency test

## Data

Synthetic, by documented exception (D6). Property-based generation covers
adversarial amounts including exact-balance and off-by-one-cent spends.

## Dependencies

Slice 01 (transfer posting must exist).

## Effort

≤1 day. Reference class: adding a guarded write. The concurrency test is the part
that takes the time, not the check itself.

## Pre-slice SPIKE

Not required if slice 01's SPIKE settled the transaction story. If it did not,
timebox 2 hours to determine whether the chosen isolation level actually prevents
the double-spend, or whether explicit row locking is needed.

## Note

Ordering choice: this slice precedes idempotency deliberately. Settling isolation
here makes the idempotency storage design in slice 03 simpler. Flipping the order
is viable but defers the harder concurrency question.
