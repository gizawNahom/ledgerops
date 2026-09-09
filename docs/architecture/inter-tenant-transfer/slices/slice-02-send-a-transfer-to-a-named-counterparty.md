# Slice 02 — Send a transfer to a named counterparty (walking skeleton)

**Story**: US-2 | **job_id**: J8 | **persona**: P1

## Goal
A tenant can register an alias for a counterparty on an already-authorized
link, then send a transfer by that alias that lands, end to end, in the
counterparty's own wallet — the three-leg happy path, no retry or failure
handling yet.

## IN scope
- `POST /counterparties {alias, tenant_link_id, to_account}` (own tenant_key)
- `POST /transfers {counterparty_alias, amount, Idempotency-Key}` (own tenant_key)
- Leg 1 (sender's own ledger, intra-tenant), Leg 2 (platform ledger,
  Platform[sender] -> Platform[receiver]), Leg 3 (receiver's own ledger,
  intra-tenant) all post successfully in the happy path
- Shared `transfer_id` correlating all three legs
- `GET /transfers/{transfer_id}` returns `status: settled` once all three legs post

## OUT scope
- Retry-until-settled when a leg fails (slice 03)
- Bounded compensation / reversal (slice 04)
- The `pending`/`retrying` intermediate states as anything other than a
  transient value on the way to `settled` (their full meaning is slice 03/04)
- The adversarial cross-tenant-forgery proof (slice 05 — needs the audit
  lookup this slice's GET endpoint establishes)

## Learning hypothesis
Disproves: "the 3-leg platform-as-hub model can be expressed as one atomic
unit of work the way a single-tenant transfer is." If Leg 1/2/3 cannot each
stay a plain intra-tenant `Post` call (locked constraint: I8 forbids any
single `Post` spanning two tenants) without inventing new cross-cutting
machinery in the domain core itself, that is signal the saga boundary is in
the wrong layer.

Confirms if it succeeds: three ordinary intra-tenant `Post` calls, correlated
only by an application-layer `transfer_id`, are sufficient for the entire
happy path — no domain-core change beyond what multitenancy already shipped.

## Acceptance criteria
- [ ] Registering an alias against a revoked or nonexistent link is refused `404 tenant_link_not_found`
- [ ] Sending to an unregistered alias is refused `404 counterparty_not_found`
- [ ] A happy-path transfer produces three posted legs, each intra-tenant, sharing one `transfer_id`
- [ ] `GET /transfers/{transfer_id}` reports `status: settled` and all three legs, once complete
- [ ] Insufficient funds in the sender's own wallet refuses with `422 insufficient_funds`, and no leg posts (all-or-nothing at Leg 1)
- [ ] Retrying the same `Idempotency-Key` returns the identical `transfer_id`, no duplicate legs (I7 precedent, applied at the transfer level)

## Dependencies
Slice 01 (an active tenant-link and a registered alias must exist).

## Effort estimate
≤1 day. Reference class: comparable to `ledger-core` slice 01 (`post-a-transfer`)
run three times with an orchestrating layer on top — the individual `Post`
calls are unchanged; the new work is the orchestrator and the alias lookup.

## Pre-slice SPIKE
Recommended, not required: a short spike on where the per-transfer
coordinator's state lives (in-process vs. a small persisted state row) before
committing slice 03/04's retry logic to a shape. This slice can ship with the
simplest persisted-row option and be revisited if slice 03 finds it
insufficient.
