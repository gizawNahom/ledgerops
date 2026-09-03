# Slice 01 — Provision a tenant

**Story**: US-1 | **job_id**: J7 | **persona**: P2

## Goal
The operator can create a new tenant and receive a credential scoped to it,
using only the existing platform-admin credential.

## IN scope
- `POST /tenants {name}` (illustrative shape — DESIGN settles exact port)
- Response: `{tenant_id, name, tenant_key}`
- Duplicate tenant name refused `409 tenant_already_exists`
- Non-admin credential refused `401 unidentified_caller`

## OUT scope
- Anything that reads or writes an account, transaction, or entry (slice 02)
- Tenant-scoped trial balance (slice 03)
- Tenant renaming, offboarding, or credential rotation
- Console/SPA surface for provisioning

## Learning hypothesis
Disproves: "a tenant credential can be bolted onto the existing single
`OperatorKey` model without introducing a new identity concept." If issuing
and verifying a second credential type turns out to require restructuring the
existing `requireOperatorKey` middleware in ways that touch already-shipped,
non-tenant-scoped surfaces (`/metrics`, `/console/*`), that is signal the
interim auth decision (D8) needs revisiting before slice 02, not after.

Confirms if it succeeds: the platform-admin credential can mint an
independent, distinct `tenant_key` per tenant with no change to any
already-shipped route's behavior.

## Acceptance criteria
- [ ] Provisioning with the platform-admin credential returns a distinct `tenant_id` and `tenant_key`
- [ ] Provisioning two tenants never reuses or collides a `tenant_id` or `tenant_key`
- [ ] A duplicate tenant name is refused `409 tenant_already_exists`, first tenant unaffected
- [ ] A tenant-scoped `tenant_key` cannot call the provisioning endpoint
- [ ] Existing `demo-01`..`demo-05` targets pass unmodified (regression guard)

## Dependencies
None — first slice. Establishes the `tenant_key` credential type that
slices 02 and 03 both consume.

## Effort estimate
≤1 day. Reference class: comparable to `ledger-core` slice 01 (new HTTP route
+ new credential check + a new storage table), which shipped within budget.

## Pre-slice SPIKE
Not required — `POST /accounts`'s existing `account_already_exists` /
DDD-18 precedent gives a direct pattern to follow for tenant-name uniqueness
and credential issuance; no unresolved technical uncertainty blocks starting.
