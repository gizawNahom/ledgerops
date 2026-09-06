# Slice 01 — Authorize a tenant pair

**Story**: US-1 | **job_id**: J9 | **persona**: P2

## Goal
The operator can grant a standing authorization between two existing tenants,
using only the existing platform-admin credential, and revoke it later.

## IN scope
- `POST /tenant-links {tenant_a, tenant_b}` (illustrative — DESIGN settles exact port)
- Response: `{link_id, tenant_a, tenant_b, status: active}`
- `DELETE /tenant-links/{link_id}` (or equivalent revoke) sets status to `revoked`
- Duplicate link for the same unordered pair refused `409 tenant_link_already_exists`
- Either named tenant unknown refused `404 tenant_not_found` (existing member, reused)
- Non-admin credential refused `401 unidentified_caller`

## OUT scope
- Anything that resolves a counterparty alias (slice 02)
- Anything that posts a transfer (slice 02)
- Expiry, usage limits, or any per-transfer approval step (locked scope: standing until revoked)
- Self-service link requests initiated by a tenant itself (operator-driven only, mirrors D7)

## Learning hypothesis
Disproves: "a pair-level authorization can be bolted onto the existing
tenant/tenant_key model without inventing a new aggregate." If granting and
revoking a link turns out to require touching Account or Transaction's own
construction path (rather than being checked ahead of Leg 2, alongside I8),
that is signal this feature's account model needs revisiting before slice 02.

Confirms if it succeeds: a link is a small, independent, revocable record —
provision-only in shape, exactly like Tenant's own lifecycle in multitenancy
slice 01 — with no coupling into Account or Transaction construction.

## Acceptance criteria
- [ ] Authorizing an unordered pair `{tenant_a, tenant_b}` returns a `link_id` with `status: active`
- [ ] Revoking a link sets its status to `revoked`; no other link is affected
- [ ] A duplicate authorization for the same pair is refused `409 tenant_link_already_exists`, existing link unaffected
- [ ] Naming an unknown tenant is refused `404 tenant_not_found`
- [ ] A `tenant_key` (non-admin) credential cannot call this endpoint
- [ ] Revoking a link does not affect any already-`settled` transfer between that pair (history is immutable, D7 precedent)

## Dependencies
`multitenancy` (tenant identity, platform-admin credential) — shipped.

## Effort estimate
≤1 day. Reference class: comparable to multitenancy slice 01 (new HTTP route,
new small provision-only aggregate, existing credential reused).

## Pre-slice SPIKE
Not required — direct precedent in multitenancy slice 01's `ProvisionTenant`
shape (provision + uniqueness-refusal + admin-only gate).
