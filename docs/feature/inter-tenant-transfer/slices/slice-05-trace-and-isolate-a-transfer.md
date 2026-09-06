# Slice 05 — Trace a transfer by id, and prove a third tenant cannot forge into it

**Story**: US-5 | **job_id**: J10 | **persona**: P1 and P2

## Goal
Any party to a transfer (either tenant, or the operator) can look it up by
`transfer_id` and see every leg's state in one place — and a tenant with no
authorized link to either party cannot read, resolve into, or affect it.

## IN scope
- `GET /transfers/{transfer_id}` full detail: all legs, each leg's own
  transaction id, amount, status, and the transfer's overall status
  (this endpoint already exists functionally as of slice 02 — this slice
  hardens its authorization and adds the adversarial proof)
- Authorization: only a `tenant_key` belonging to one of the transfer's two
  tenants, or the platform-admin credential, may read it
- Adversarial scenario: a third tenant's `tenant_key`, or a forged
  `counterparty_alias` naming an unauthorized tenant, is refused exactly as
  if the transfer or counterparty did not exist

## OUT scope
- Any new UI/console surface for transfer tracing (mirrors the
  `ledger-core` -> `ledger-core-console` precedent: backend first, console is
  its own future feature if needed)
- Pagination or search across multiple transfers (this is single-id lookup only)

## Learning hypothesis
Disproves: "cross-tenant isolation (I8) automatically extends to a
correlation id that spans three ledgers, with no additional check." A
`transfer_id` is not itself scoped to one tenant the way an `account_id` is —
it names an event two tenants share. If the existing per-account I8 check is
insufficient to prevent a third tenant from reading or resolving into a
transfer between two *other* tenants, that is exactly the gap this slice
exists to close.

Confirms if it succeeds: a third tenant's credential, or a link-scoped alias
that does not include the caller's own tenant on either side, gets the
identical 404 shape a genuinely nonexistent transfer or counterparty would
produce — no distinguishable signal of the transfer's, the link's, or the
counterparty's existence leaks to an unauthorized party.

## Acceptance criteria
- [ ] Either party's own `tenant_key` can `GET /transfers/{transfer_id}` and see all three legs
- [ ] The platform-admin credential can read any transfer
- [ ] A third tenant's `tenant_key` (party to neither side) is refused `404`, identical in shape to a nonexistent `transfer_id`
- [ ] A forged `counterparty_alias` naming a tenant with no active link to the caller is refused `404 counterparty_not_found`, identical in shape to a genuinely unregistered alias
- [ ] No response distinguishes "exists but forbidden" from "does not exist" (matches ADR-011/ADR-009's minimal-information-leak posture)

## Dependencies
Slices 01–04 (a full transfer lifecycle, including reversal, must exist to trace).

## Effort estimate
≤1 day. Reference class: comparable to `multitenancy` slice 03's own
per-tenant-scoped read hardening, applied to a transfer-spanning id instead
of a single account id.

## Pre-slice SPIKE
Not required — direct precedent in `multitenancy`'s I8 construction-time
check and its `account_not_found`-reuse posture (ADR-011), extended here to
the two-tenant transfer shape.
