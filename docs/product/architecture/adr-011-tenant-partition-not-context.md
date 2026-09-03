# ADR-011: Tenant is a partition dimension and a new aggregate, not a new bounded context

**Status**: Accepted, 2026-09-03
**Owner**: nw-ddd-architect (DESIGN wave, `multitenancy`)

## Context

`multitenancy` reactivates `jobs.yaml` J6/J7, deferred from `ledger-core` as
D8 ("single-tenant; tenant isolation deferred"). DISCUSS's own Scope
Assessment flagged, but did not settle, that this looks like "1 bounded
context — tenant is a partition dimension within it, not a new context" —
explicitly left for `nw-ddd-architect` to confirm. Three domain-modelling
questions were named as DESIGN pre-requisites, load-bearing in this order:

1. Does introducing tenants introduce a second bounded context, and what is
   Tenant in the model — aggregate, value object, something else?
2. `feature-delta.md` § Pre-requisites #1 — DDD-18 ("an account name
   identifies exactly one account") was written for a single-tenant world.
   Does its scope change, and does its identifier still fit?
3. `feature-delta.md` § Pre-requisites #2 — D9 recommends enforcing I8
   (tenant isolation) by construction rather than by after-the-fact
   verification, mirroring I1/I4/DDD-18's own precedent, but explicitly
   defers the decision to DESIGN.

Brownfield state, verified directly: `internal/domain/account.go` has no
tenant concept; `migrations/0001_init.up.sql` gives `accounts.id` a bare
global `PRIMARY KEY`; `router.go`'s `requireOperatorKey` checks one shared
secret for the whole deployment. Zero existing tenant scaffolding.

## Decision

**1. One bounded context.** Ledger stays the only bounded context. The
primary discovery heuristic — language divergence — finds none: Account,
Entry, Transaction, Posting, Trial balance, Drift, and Verdict mean the same
thing to every tenant's integrating developer and to the platform operator.
Tenant is a scoping *dimension* orthogonal to that vocabulary, not a second
vocabulary.

**2. Tenant is a new aggregate root**, not a value object. It has an
identity (`tenant_id`) that persists across its one lifecycle event
(provisioning), which is what makes it an entity per the tactical DDD design
rules rather than a value type compared by attributes. It owns exactly two
value-typed fields: `name` and `credential` (`tenant_key`, opaque to the
domain beyond existence/distinctness — verification mechanics are
`nw-solution-architect`'s). Root-only, value-typed properties — satisfies
Vernon's rule 2 (small aggregates) by inspection. Account and Transaction
reference `tenant_id` by value (Vernon rule 3); Tenant never embeds them,
which keeps posting a two-aggregate-type unit of work rather than three.

**3. DDD-18 is renumbered to I9, rescoped to `(tenant_id, account_id)`,
and two new invariants are added: I8 (tenant isolation) and I10 (tenant-name
uniqueness).** DDD-18 was a decision-ID borrowed to label a row in an
invariant table otherwise occupied by I-numbered invariants; the renumbering
fixes that friction, not just the scope. I10 mirrors I9's own pattern
(uniqueness checked in the domain core over a read snapshot, backed by a
database constraint) one aggregate level up, at Tenant instead of Account.

**4. I8 is enforced by construction**, agreeing with D9's recommendation
after evaluating rather than rubber-stamping it. Concretely:
- `tenant_id` becomes a required field on `Account` — no smart-constructor
  path produces one without it (DDD-15's pattern, transplanted).
- Repository query and lock-acquisition ports require `tenant_id` as a
  parameter, not an optional filter — a call site cannot compile a query
  that omits it.
- `Post` cross-checks that every touched account snapshot's `tenant_id`
  matches the command's own, refusing before constructing the `Transaction`
  — the same pre-construction gate I1 and I4 already occupy.

**5. Cross-tenant reference reuses the sealed `account_not_found`
`ViolationKind`.** No new taxonomy member. From a tenant-scoped caller's
point of view, another tenant's account is outside their observable
universe, which is exactly what `account_not_found` already means (DDD-17:
404). This resolves the domain half of the "404 vs 403" question named in
`feature-delta.md` § Pre-requisites #4.

**6. Unscoped trial-balance/verdict calls mean "platform-wide aggregate,"
not "a designated default tenant."** Same computation (sum over entries)
over a different entry-set filter — all entries, or entries scoped to one
`tenant_id` — not two domain concepts. Resolves the domain half of
§ Pre-requisites #5.

## Consequences

- `Account`, `Transaction` gain a `tenant_id` field; `Tenant` is a new
  aggregate root. Migration/schema mechanics (composite key shape,
  expand-only sequencing) are `nw-solution-architect`'s to design.
- The sealed `ViolationKind` taxonomy (DDD-12) does not grow a new member
  for cross-tenant access — `account_not_found`'s existing 404 mapping
  (DDD-17) already covers it, so the `exhaustive` linter's obligation
  (DEVOPS) does not grow either.
- ES/CQRS remains not warranted for this context, Tenant included — the
  four-question heuristic answers "no" more decisively for Tenant's
  provision-only lifecycle than it already did for Account/Transaction under
  `adr-003-stored-balances.md`.
- Credential verification mechanism (`tenant_key` vs `requireOperatorKey`)
  is explicitly not decided here — `feature-delta.md` § Pre-requisites #3,
  owned by `nw-solution-architect`.

## Rejected alternatives

- **Tenant as a second bounded context** — rejected; no language divergence,
  no organizational boundary, no independent deployability requirement. Would
  have forced a context map and an integration pattern (ACL, OHS, etc.) for
  a relationship that is actually just a foreign-key-shaped scoping value.
- **Tenant as a value object** — rejected; it has identity that persists
  across its lifecycle (provisioning), the defining trait of an entity, not
  a value type.
- **Tenant owning Accounts as child entities** — rejected; would create a
  three-aggregate-type unit of work for posting (Tenant, Transaction,
  Account) where two already required a deliberate, documented deviation
  from "one transaction, one aggregate." Reference by identity instead.
- **I8 enforced by verification (I3-style drift scan)** — rejected; I3's
  unenforced status has independent value (an intentional cross-check with
  its own reason to exist as a *check*, not merely a rule). I8 is a security
  property with a known, cheap construction-time fix; there is no
  independent value in occasionally discovering a leak after the fact.
- **A new `cross_tenant_forbidden` ViolationKind (403)** — rejected; grows
  both `exhaustive`-linted switch surfaces for no behavioral gain, and gives
  any caller a tenant-existence oracle by comparing 403 against 404 — the
  opposite of what I8 exists to guarantee.
- **"Designated default tenant" for unscoped calls** — rejected; no
  provisioning story exists for it (D7 is operator-driven-only), and it
  would make an already-shipped concept implicitly about whichever tenant
  got the designation, which is a modelling accident rather than a decision.

## References

- `docs/product/architecture/brief.md` § Domain Model / "Multitenancy
  (`multitenancy`, confirmed 2026-09-03)" — full rationale and the
  bounded-change contracts for Tenant, Account, and Transaction.
- `docs/feature/multitenancy/feature-delta.md` § Pre-requisites #1, #2, #4,
  #5; § Wave: DESIGN / Domain Model.
- `adr-008-refusal-taxonomy-boundary.md`, `adr-009-unavailability-is-not-a-refusal.md`,
  `adr-003-stored-balances.md` — precedent this decision extends.
