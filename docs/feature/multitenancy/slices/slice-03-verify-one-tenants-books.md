# Slice 03 — Verify one tenant's books without seeing another's

**Story**: US-3 | **job_id**: J7 | **persona**: P2

## Goal
The operator can check whether one specific tenant's books balance, with the
answer scoped strictly to that tenant.

## IN scope
- `GET /health/trial-balance?tenant_id=...` (extends the existing endpoint)
- Response scoped to the named tenant's accounts and entries only
- Drift attribution (account, delta) within the scoped tenant, matching the
  existing single-tenant contract's behavior
- Unprovisioned `tenant_id` refused, not silently answered `YES`
- **(added 2026-09-03)** The existing unscoped call (no `tenant_id`) to
  `GET /health/trial-balance` and `GET /console/verdict` (same handler)
  continues to succeed with a meaningful verdict, using the `OperatorKey` —
  hard constraint, verified by a CI-gated acceptance scenario in this slice
  (see § Acceptance criteria), not merely "not broken by accident." What that
  verdict *means* once tenants exist (platform-wide aggregate vs. a
  designated default tenant) is DESIGN's call; that it must not error or
  require `tenant_id` is not

## OUT scope
- Tenant provisioning (slice 01, dependency)
- Account/transfer isolation itself (slice 02, dependency)
- Console/SPA *rendering* — the console's existing unscoped call must keep
  working (IN scope, above), but no console code changes here
- Choosing *which* semantics the unscoped call resolves to (platform-wide vs.
  default-tenant) — explicitly an open DESIGN question
  (`feature-delta.md` § Pre-requisites #5); only "it must not stop working"
  is settled by this brief, not "what it means"

## Learning hypothesis
Disproves: "reusing `VerifyBooks`'s existing full-scan verification unchanged,
filtered by tenant at the query layer, is sufficient." If tenant-scoping the
scan requires touching the pure `Post`/verification core rather than only the
storage query, that is signal I8's enforcement boundary is not as cleanly
separable from I3's verification boundary as `feature-delta.md`'s D9
recommendation assumed.

Confirms if it succeeds: one tenant's drift (or lack of it) is reported with
zero visibility into any other tenant's state, using the same verdict wording
("Books balance: YES/NO") the operator already trusts from `ledger-core`.

## Acceptance criteria
- [ ] A tenant-scoped trial-balance check reports only that tenant's verdict
- [ ] Drift attribution within one tenant matches the existing single-tenant contract exactly
- [ ] Checking an unprovisioned `tenant_id` is refused, not answered `YES`
- [ ] Existing `demo-01`..`demo-05` and slice 01/02 demo targets pass unmodified
- [ ] **(added 2026-09-03, revised 2026-09-03 per peer-review iteration 2 — original wording gated on non-CI-gated manual KPIs, corrected)** An acceptance scenario calls `GET /console/verdict` and `GET /health/trial-balance` unscoped with `OperatorKey` (the console's existing call shape) and asserts both responses are byte-identical in shape and status to today's single-tenant contract — CI-gated; `kpi-contracts.yaml` KPI-C1/KPI-C3's manual dogfood checks remain `ledger-core-console`'s own separate responsibility, not duplicated here

## Dependencies
Slices 01 (tenant must exist to be queried) and 02 (accounts/entries must be
tenant-scoped for the query to filter correctly).

## Effort estimate
≤1 day. Reference class: comparable to `ledger-core` slice 05 (extending an
existing read endpoint with a new filter), which shipped within budget.

## Pre-slice SPIKE
Not required, contingent on slice 02 having already settled the storage-layer
tenant-scoping mechanism this slice's query filter depends on.
