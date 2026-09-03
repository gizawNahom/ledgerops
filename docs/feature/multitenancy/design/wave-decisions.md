# Wave Decisions Summary — DESIGN (`multitenancy`)

Full stack DESIGN sequence: system (`nw-system-designer`) -> domain
(`nw-ddd-architect`) -> application (`nw-solution-architect`), all three
completed 2026-09-03. This file is the mandatory cross-architect summary the
nw-design skill requires regardless of this project's otherwise-lean,
single-file-narrative convention; the full per-architect narrative lives in
`docs/product/architecture/brief.md` and `docs/feature/multitenancy/feature-delta.md`.

## System Architecture (`nw-system-designer`)

**Finding: no new system-level architecture concern.** Verified against the
feature's own artifacts and the brownfield code, not assumed.

- Deployment topology unchanged: one Go binary, one PostgreSQL 16 instance, no
  hosted environment.
- Tenancy is a data-partitioning dimension (`tenant_id` as a scoping key), not
  sharding. Back-of-envelope: tens-to-low-hundreds of tenants (operator-driven
  provisioning only, D7), well within the existing 300 rps / single-instance
  capacity envelope already validated by `tests/load/transfers.js`.
- OPS-10's two-role model, backup/restore boundary, and connection-pooling
  posture all unchanged. No new startup probe — the existing wire-then-probe
  sequence already covers the substrate this feature's new table/queries
  depend on.
- Escape hatch documented, not built: a composite index on `(tenant_id,
  account_id)` if tenant/account volume grows past the low-hundreds/thousands
  range assumed.
- Open questions for the user: **none**.

Full narrative: `docs/product/architecture/brief.md` § System Architecture /
"Multitenancy"; `feature-delta.md` § Wave: DESIGN / System Architecture.

## Domain Model (`nw-ddd-architect`)

**Bounded context confirmed unchanged**: one context, Ledger — tenant is a
partition dimension, not a second context (no language divergence found).

- **Tenant is a new aggregate root**, not a value object: `{tenant_id, name,
  credential}`, referenced by ID from Account/Transaction, never embedded.
- **I9** (renumbered from DDD-18): account-name uniqueness rescoped from
  global to `(tenant_id, account_id)`. **I8** (new): tenant isolation,
  enforced **by construction** — `tenant_id` required on `Account`, required
  on every repository query/lock port, cross-checked inside `Post` before
  construction (the same gate I1/I4 occupy). **I10** (new): tenant-name
  uniqueness, structurally identical to I9 one aggregate level up.
- Cross-tenant account reference **reuses the sealed `account_not_found`
  ViolationKind** — no new taxonomy member for I8 specifically; this forces
  404, not a fresh wire-level style choice.
- Unscoped `GET /health/trial-balance` / `GET /console/verdict` means
  **"platform-wide aggregate over all entries"** — same computation as today,
  a different (or absent) entry-set filter, not a "designated default
  tenant" (rejected — no provisioning story exists for one).
- ES/CQRS reconfirmed not warranted for Tenant, more decisively than for
  Account/Transaction.
- Open questions for the user: **none** — every domain-modelling question
  this architect owned is resolved with evidence-grounded reasoning.

Full narrative: `docs/product/architecture/brief.md` § Domain Model /
"Multitenancy"; ADR: `adr-011-tenant-partition-not-context.md`;
`feature-delta.md` § Wave: DESIGN / Domain Model.

## Application Architecture (`nw-solution-architect`, this leg)

- **Credential mechanism (DDD-22)**: D8's shape evaluated against the actual
  `router.go` structure and found sound with one correction — `chi` nested
  route groups, not a single shared middleware, are required once different
  routes need different credential sets. `requireOperatorKey` is reused
  unmodified (mounted on `POST /tenants` too, and unchanged on
  `GET /console/verdict`/`GET /health/trial-balance`). Two new middlewares
  (`requireTenantKey`, `requireTenantKeyOrOperatorKey`) are composed from an
  extracted bearer-header primitive, not duplicated. `TenantScope` is a
  closed two-constructor type, never a nullable string, used only where
  "unscoped" is a legitimate domain answer (reads); every write-path port
  keeps `tenant_id` as a plain required parameter. Credentials are generated
  via the existing `IDGenerator` port and hashed with the already-imported
  `crypto/sha256` — zero new dependencies.
- **Migration shape (DDD-24)**: slice-02's SPIKE question answered directly —
  yes, expressible as a single expand-only migration, with one cascade the
  SPIKE's framing didn't name: `accounts`' bare `PRIMARY KEY` must become
  composite `(tenant_id, id)` (a global PK structurally forbids two tenants
  sharing an account name, which US-2 requires), which in turn invalidates
  `entries`' existing single-column FKs to `accounts(id)` — both become
  composite FKs, and `entries`/`transactions` each gain a `tenant_id` column.
  One migration file, one transaction, a migration-seeded `tnt_legacy_seed`
  sentinel backfills pre-existing rows before the composite PK is applied.
- **Wire mapping (DDD-25)**: two new sealed `ViolationKind` members,
  `tenant_already_exists` (409) and `tenant_not_found` (404) — I10's own
  pair, distinct from (not a contradiction of) ADR-011's "no new member for
  cross-tenant access" (which covers only I8's reuse of `account_not_found`).
  Cross-tenant `account_not_found` reuse confirmed mechanical at the
  HTTP-adapter layer — no new adapter code path.
- **Reuse Analysis (DDD-26)**: `requireOperatorKey`, `IDGenerator`, and
  `crypto/sha256` all EXTENDED (reused as-is); `TenantRepository`,
  `TenantKeyResolver`, `requireTenantKey`, `requireTenantKeyOrOperatorKey`
  CREATE NEW, justified by genuine mechanism difference, explicitly not by
  "the existing class has too many dependencies."
- **Contract-shape classification**: every new/changed component classified
  (pure-function / bounded-change) per Core Principle 12, with declared
  mutation sets/universes and the crafter-facing assertion mechanism for
  each — see `brief.md` § Multitenancy.

### DDD-23 — resolved 2026-09-03 (user decision, relayed by the coordinator; was the one open item across all three DESIGN legs)

Verified directly against `Makefile`: `demo-01`, `demo-02`, `demo-03`, and
`chaos-01` all authenticate `POST /accounts`/`POST /transfers`/`GET
/accounts/{id}` with the shared `OperatorKey` — the same three routes this
feature's credential mapping scopes to `tenant_key` only. This was exactly
the risk slice-01's own Learning Hypothesis named as a checkpoint before
slice 02, not an oversight discovered late. Two structurally sound options
were presented, with a genuine security-posture trade-off and no technically
superior answer:

- **Option A**: `OperatorKey` implicitly resolves to a seeded legacy tenant
  for these three routes too (a third dual-mode route family, same
  `requireTenantKeyOrOperatorKey` primitive already built for
  `GET /accounts/{id}/entries`). Zero `Makefile` changes. Cost: `OperatorKey`
  gains tenant-*write* authority, not just admin/read authority.
- **Option C**: seed a second fixed dev/demo credential bound to the same
  legacy tenant; update only the `Makefile`'s shared `AUTH` variable's
  *value*, leaving every demo/chaos/race recipe body byte-for-byte unchanged.
  Keeps `OperatorKey` conceptually pure (admin + unscoped reads only). Cost:
  touches `Makefile` (a DEVOPS/DELIVER-owned file) and requires seeding a
  second fixed secret at startup.

**Decision: Option C.** User's stated rationale — preserves the
admin/tenant-credential boundary DDD-22 already draws (`OperatorKey` never
gains a tenant's own write authority); consistent with I8's isolation ethos
and this project's correctness/auditability-ranked-first quality attributes;
accepts the named cost (a DEVOPS-owned file touched, one more fixed dev
secret seeded — precedented by `demo-operator-key` already being one).
Option A is rejected: it would have reused the `requireTenantKeyOrOperatorKey`
primitive already built for entries at zero Makefile-editing cost, but at
the cost of `OperatorKey` becoming a credential that can move money inside
one tenant's ledger, which the user judged not worth the convenience.

**What DESIGN decided vs. what DEVOPS/DELIVER builds**: this DESIGN pass's
job ends at recording the decision above and the requirement it creates —
mirrors how DDD-12's `exhaustive`-linter obligation is handed to DEVOPS
elsewhere in this project's `brief.md`. The actual `Makefile` edit (seeding
a second fixed demo tenant credential, e.g. `LEDGEROPS_DEMO_TENANT_KEY`
mirroring `LEDGEROPS_OPERATOR_KEY`'s existing pattern, and re-pointing the
`AUTH` variable's value) is a **DEVOPS/DELIVER implementation task**, not
something this DESIGN wave builds. Both options would have reused the
identical `tnt_legacy_seed` tenant the migration (DDD-24) already requires
for independent schema reasons, so no work already done is wasted by this
choice.

**Full record**: `docs/feature/multitenancy/feature-delta.md` § Wave: DESIGN
/ Application Architecture (Decisions table, DDD-23 row, and § Open
questions for the user).

All three DESIGN legs are now closed with zero open questions.

## Cross-cutting confirmations

- **Reuse Analysis gate**: satisfied for all three legs. System/domain legs
  found no reusable prior art (greenfield tenant concept, confirmed by
  Glob/Grep). Application leg's Reuse Analysis table (`brief.md` §
  Application Architecture, `feature-delta.md` § Wave: DESIGN / Application
  Architecture) extends the existing `requireOperatorKey` row and adds six
  new rows, none justified by "too many dependencies."
- **Outcome Collision Check**: performed by direct reasoning against
  `docs/product/outcomes/registry.yaml` (the `nwave-ai outcomes check-delta`
  CLI is broken in this install — no packaged `schema.json`, a
  previously-documented issue, not new). Result: clean. One new
  non-colliding outcome candidate (`ProvisionTenant`), three new
  non-colliding invariant candidates (I8, I9, I10), and five existing rows
  (OUT-1 through OUT-5) flagged for DISTILL to *extend* their shape fields
  rather than treat as new or leave stale.
- **C4 diagrams**: no new diagram required at System Context (L1) or
  Container (L2) level — both confirmed unchanged by `nw-system-designer`
  (no new deployable, no new box). No Component (L3) diagram added for the
  new HTTP-auth subsystem — three new middleware functions inside the
  existing `internal/adapters/http/` component do not constitute a "complex
  subsystem" warranting L3 per this project's own threshold (5+ components).
- **Peer review**: not invoked in this application-architecture leg per the
  orchestrator's explicit dispatch scope for this task (five named outputs,
  no peer-review step named). Flagged here so the omission is visible to
  whichever wave step performs the cross-architect DESIGN peer review, rather
  than silently absent.
