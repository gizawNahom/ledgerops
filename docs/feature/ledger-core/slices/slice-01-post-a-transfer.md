# Slice 01 — Post a transfer (Walking Skeleton)

**Job**: J1 | **Invariant**: I1 | **Story**: US-1 | **WS strategy**: C (real local)

## Goal

Post one balanced two-leg transfer end-to-end — HTTP in, domain, real database,
balances readable — and prove it either lands completely or not at all.

## Learning hypothesis

Disproves that the chosen stack can perform an atomic multi-row write end-to-end
in a day. If this slice overruns, the persistence or transaction story is wrong
and every later slice inherits the problem.

## IN scope

- `POST /accounts` — create an account, balance starts at zero
- `POST /transfers` — one transfer, exactly two legs, same currency
- `GET /accounts/{id}` — read a balance
- A system account that funding originates from, so value never enters the ledger
  unaccounted for
- Transaction table + entry table, written in a single database transaction
- Entries are append-only, enforced at the database level (D7)
- API-key auth on all endpoints (single seeded operator key)
- Chaos demo: kill the process mid-write, restart, show no half-applied state

## OUT of scope

- Sufficient-funds checking (slice 02) — balances may go negative here
- Idempotency (slice 03) — retries may double-post here
- Operations console (slice 04) — verification is by API only
- Multi-currency, reversals, N-leg transactions, tenancy

## Acceptance criteria

- [ ] `POST /transfers` creates exactly one transaction with exactly two entries
- [ ] The two entries sum to zero (I1)
- [ ] Both entries commit together or neither commits — verified by killing the
      process mid-transaction and asserting post-restart state is unchanged
- [ ] `GET /accounts/{id}` reflects the sum of that account's entries
- [ ] Funding an account is itself a transfer from the system account, not an
      assignment
- [ ] Transfer to an unknown account returns 404 and changes no state
- [ ] `UPDATE` and `DELETE` against the entry table are refused by the database,
      not merely avoided by application code (D7) — asserted by a test that
      attempts both and expects failure
- [ ] Property-based test generates adversarial amounts (zero, fractional,
      boundary, very large) and asserts I1 holds for every generated transfer
- [ ] `make demo-01` runs the happy path; `make chaos-01` runs the kill test

## Data

Synthetic, by documented exception (see feature-delta, D6). No production data
source exists pre-launch. Mitigation is the property-based test above, which
generates adversarial amounts rather than relying on hand-picked fixtures.

## Dependencies

None. This is the first slice.

## Effort

≤1 day. Reference class: a CRUD endpoint plus a two-table transactional write.
The risk is not the code volume but the first-time setup of stack, migrations,
and test harness.

## Pre-slice SPIKE

Recommended, timeboxed to 2 hours: confirm the chosen database and language give
you a straightforward way to assert "process died mid-transaction, nothing
persisted." If that test is awkward to write, the chaos demo — the thing that
makes this slice worth anything — will not exist.
