# Slice 02 — Operate within a tenant, invisible to every other tenant

**Story**: US-2 | **job_id**: J6 | **persona**: P1 | **invariant**: I8

## Goal
A tenant's credential can open accounts and post transfers exactly as today's
single-tenant API already works, and no other tenant's credential can read or
affect them.

## IN scope
- `POST /accounts`, `POST /transfers`, `GET /accounts/{id}` scoped by the
  caller's `tenant_key`
- `GET /accounts/{id}/entries` — **dual-mode** (added 2026-09-03, was missing
  from this brief entirely; console-compatibility confirmation surfaced the
  gap): a `tenant_key` sees only that tenant's entries (new); the existing
  unscoped `OperatorKey` call — the exact shape the shipped console's
  `EntryTrace` component uses — keeps succeeding identically to today. Both
  modes verified by CI-gated acceptance scenarios in this slice, not by a
  manual checklist (see § Acceptance criteria and `feature-delta.md`
  § Driving ports)
- Account-name uniqueness rescoped from global to `(tenant, account_id)`
  (DDD-18 reinterpretation — requires DESIGN/DDD-architect sign-off per
  `feature-delta.md` § Pre-requisites #1)
- Cross-tenant read refused (`account_not_found`, per journey's provisional
  404 assumption — DESIGN confirms)
- Cross-tenant transfer refused (**permanent**, not merely unbuilt — a
  distinct, platform-mediated inter-tenant settlement mechanism is planned as
  a separate follow-on feature; see `feature-delta.md` § Changed Assumptions
  and `jobs.yaml` J8. Nothing in that mechanism ever involves a tenant's own
  credential writing across the boundary, so it does not reopen this AC)
- All four existing invariants (I1, I3, I4, I7) hold, unchanged, within one
  tenant's scope

## OUT scope
- Tenant provisioning itself (slice 01, dependency)
- Tenant-scoped trial-balance reporting (slice 03)
- Console/SPA *rendering* of any of this — the console's existing behavior
  must keep working (IN scope, above), but no console code changes here
- Changing any existing single-tenant response shape — the wire contract for
  a caller who only ever has one tenant must look identical to today
- Platform-mediated inter-tenant settlement (its own follow-on feature,
  `jobs.yaml` J8 — depends on this slice shipping first)

## Learning hypothesis
Disproves: "rescoping account-name uniqueness from global to per-tenant is a
narrow, additive change." If it turns out to ripple into every existing
handler, port, and adapter test in ways that risk regressing the already-
shipped `demo-01`..`demo-05` targets, that is signal this slice is actually
two slices (schema rescoping, then cross-tenant refusal), not one.

Confirms if it succeeds: an integrator who only ever has one tenant sees zero
behavioral difference from today, while a second tenant's credential is
provably unable to reach the first tenant's data.

## Acceptance criteria
- [ ] Account name uniqueness is scoped per tenant, not global
- [ ] A tenant's credential can never read or write another tenant's account, transaction, or entry
- [ ] A transfer naming accounts from two different tenants is refused, neither balance changes
- [ ] I1, I3, I4, I7 continue to hold, unchanged, within one tenant's scope
- [ ] Existing `demo-01`..`demo-05` targets pass unmodified
- [ ] **(added 2026-09-03, revised 2026-09-03 per peer-review iteration 2 — original wording gated on a non-CI-gated manual KPI, corrected)** An acceptance scenario calls `GET /accounts/{id}/entries` unscoped with `OperatorKey` (the console's existing call shape) and asserts the response is byte-identical in shape and status to today's single-tenant contract — CI-gated; `kpi-contracts.yaml` KPI-C2's manual dogfood check remains `ledger-core-console`'s own separate responsibility, not duplicated here

## Dependencies
Slice 01 (needs `tenant_key` issuance to exist). Requires DESIGN sign-off on
DDD-18 rescoping before implementation starts — the highest-risk item in
`feature-delta.md` § Pre-requisites.

## Effort estimate
≤1 day, contingent on DESIGN having already settled DDD-18's rescoping and
the credential-verification mechanism (this slice's dependency, not its own
scope creep). **Revised 2026-09-03 per peer-review iteration 2**: this
estimate now explicitly includes `GET /accounts/{id}/entries`'s dual-mode
behavior. Assumption: DESIGN's credential-verification mechanism resolves the
mode at the HTTP boundary (which credential was presented), not inside
domain logic — routing, not a new domain rule. **Risk, named rather than
absorbed silently**: if DESIGN's mechanism needs mode-aware behavior deeper
than the boundary, this slice is already carrying the feature's highest-risk
item (DDD-18 rescoping) and stacking dual-mode entries on top could push it
past ≤1 day — revisit splitting dual-mode entries into its own thin slice
then, not assumed away now.

## Pre-slice SPIKE
Recommended, narrowly scoped: confirm `(tenant_id, account_id)` composite
uniqueness is expressible as an expand-only migration against the existing
`accounts` table (`migrations/0001_init.up.sql`'s bare `PRIMARY KEY`) without
violating the project's expand-only migration discipline. This is a factual
question DESIGN should answer before this slice is estimated as ≤1 day, not
a design decision this brief is making.
