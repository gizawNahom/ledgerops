# Evolution — multitenancy

**Status: DELIVERED.** All 12 roadmap steps DONE (`des-verify-integrity
docs/feature/multitenancy/deliver/` → exit 0, "All 12 steps have complete DES
traces"). L1-L6 refactor pass committed (`eb2af61`). Phase 4 adversarial
review: APPROVED, zero blocking issues. Full acceptance regression green.

---

## Feature summary and business context

**JTBD**: let more than one customer share a ledgerops deployment with no
path for one tenant's data to be read, written, or collided with by
another — provable, not promised.

This feature reactivates job **J6** (`docs/product/jobs.yaml`), dropped from
`ledger-core`'s original scope on 2026-08-17 as D8 ("single-tenant; tenant
isolation deferred... revisit before any third party's money is held"), and
adds **J7** (onboard/verify a tenant independent of every other tenant's
state). The deferral's own stated trigger condition is the evidence that
authorized revisiting it now — no new DISCOVER/DIVERGE research ran, the same
founder-estimated evidence basis `ledger-core`'s own jobs already carry.

**Three user stories, each ≤1 day, one per slice** (D5 dropped console/SPA
scope from this feature, mirroring the `ledger-core` → `ledger-core-console`
precedent exactly):

| Story | Job | Invariant | Slice |
|---|---|---|---|
| US-1 — Provision a tenant | J7 | I10 (tenant-name uniqueness) | 01 |
| US-2 — Operate within a tenant, invisible to every other tenant | J6 | I8 (tenant isolation), I9 (rescoped account uniqueness) | 02 |
| US-3 — Verify one tenant's books without seeing another's | J7 | — (extends I3's verification path) | 03 |

Personas: P1 (integrating developer, one instance per tenant) and P2
(platform operator, provisions and verifies tenants) — no new persona
introduced (`vision.md` rules out modeling a tenant's own end-customers).

**Hard constraint carried through delivery**: the console (`ledger-core-console`)
authenticates every call today with the single shared `OperatorKey`, unscoped.
A tenant-scoped API could have broken that path. The DISCUSS-wave
console-deferral confirmation (2026-09-03) made "the console must keep working
throughout, CI-gated" a locked, non-negotiable constraint — not an
aspiration — and it held through DELIVER (`console-compat` CI job, KPI-M4).

---

## Key decisions

### DISCUSS wave
- **D1-D9**: cross-cutting feature type; feature's own walking skeleton
  (no reuse from `ledger-core`); lightweight UX depth; JTBD mandatory;
  console/SPA excluded (D5); operator-driven-only provisioning (D7); interim
  auth reuses `OperatorKey` as platform-admin, adds `tenant_key` (D8,
  provisional); I8 recommended for construction-time enforcement (D9).
- **Changed Assumptions (2026-09-03 amendment)**: cross-tenant transfer
  under a tenant's own credential stays permanently unsupported; a distinct,
  deliberately-authorized *platform-mediated* inter-tenant settlement
  mechanism (`jobs.yaml` J8) is deferred to its own follow-on feature. This
  amendment also surfaced a genuine gap — `GET /accounts/{id}/entries` was
  missing entirely from the Driving Ports table — corrected, not merely
  reworded.
- **Console-deferral confirmation (2026-09-03)**: turned "console work is out
  of scope" into "console must keep working, CI-gated" — a real scope
  addition to slices 02/03, caught a second peer-review iteration
  (`rejected_pending_revisions` → remediated) that found the first draft
  gated new AC/DoD on non-CI-gated manual KPIs.

### DESIGN wave (system → domain → application)
- **System** (`nw-system-designer`): zero new system-level footprint — one
  Go binary, one PostgreSQL 16 instance, no hosted environment, tenancy is a
  data-partitioning dimension not sharding.
- **Domain** (`nw-ddd-architect`): `Tenant` is a new aggregate root. DDD-18
  renumbered to **I9** (account uniqueness rescoped `(tenant_id, account_id)`);
  **I8** (tenant isolation, construction-time enforcement) and **I10**
  (tenant-name uniqueness) added. Cross-tenant refusal reuses the sealed
  `account_not_found` member — no new taxonomy row for I8 itself. Unscoped
  trial-balance/verdict means "platform-wide aggregate."
- **Application** (`nw-solution-architect`, **DDD-22 through DDD-26**):
  `chi` nested route groups + `requireOperatorKey` (reused verbatim) +
  two new middlewares (`requireTenantKey`, `requireTenantKeyOrOperatorKey`)
  composed from an extracted bearer-header primitive (DDD-22). Migration:
  `accounts`' bare PK becomes composite `(tenant_id, id)`, cascading FKs on
  `entries`, plain `tenant_id` on `transactions`, one expand-only migration
  with a `tnt_legacy_seed` sentinel backfill (DDD-24). Wire mapping: two new
  sealed `ViolationKind` members, `tenant_already_exists`/`tenant_not_found`
  (DDD-25). Reuse analysis: `requireOperatorKey`/`IDGenerator`/`crypto/sha256`
  EXTENDED; `TenantRepository`/`TenantKeyResolver`/two new middlewares CREATE
  NEW, justified by genuine mechanism difference (DDD-26).
- **DDD-23 (Option C)** — the one open item across all three DESIGN legs,
  resolved by direct user decision: `POST /accounts`/`POST /transfers`/
  `GET /accounts/{id}` stay `tenant_key`-only; `OperatorKey` gains no
  tenant-write authority. Cost accepted: seed a second fixed demo/dev
  credential and split the `Makefile`'s `AUTH` variable, rather than let
  `OperatorKey` implicitly resolve to a legacy tenant (Option A, rejected).
  This decision is the direct cause of OPS-10/11/13 below.

### DEVOPS wave
- **OPS-1 through OPS-9**: unchanged carry-forwards from `ledger-core`'s own
  DEVOPS wave (no hosted environment, Docker Compose, GitHub Actions,
  scaffold-status observability, Recreate deployment, trunk-based, nightly-delta
  mutation testing) — confirmed applicable, not re-decided, since DESIGN's
  system leg found zero new footprint.
- **OPS-10** (DESIGN-recorded): `LEDGEROPS_DEMO_TENANT_KEY` env var + `Makefile`
  `AUTH`/`OPERATOR_AUTH` split, implementing DDD-23 Option C.
- **OPS-11 and OPS-13 — DEVOPS/DELIVER-discovered gaps DESIGN's own DDD-23
  record missed, not DESIGN/DEVOPS failures per se, but a real pattern worth
  naming** (see Lessons learned below):
  - **OPS-11** (found during DEVOPS's own direct-code verification, not
    assumed from the decision record's prose): `scripts/race/main.go`'s
    `RACE_OPERATOR_KEY` default is a separate credential path DDD-23's text
    never named — `race-02`/`race-03` would 401 the moment slice 02 landed.
    Fixed with a new `RACE_TENANT_KEY`.
  - **OPS-13** (found during DELIVER step 02-04's own regression run, a
    class DESIGN and DEVOPS both missed): `tests/acceptance/ledgercore/
    ledger_world.go` hardcodes `OperatorKey` for every request — a third,
    previously unnamed consumer of the now-tenant-key-only routes. This
    broke 80/93 scenarios in the project's CI-blocking `integration` job, a
    materially worse regression than OPS-10/11 (those are non-blocking
    convenience targets; this is a required job). Fixed in step 02-05 by
    mirroring OPS-11's pattern exactly.
- **OPS-12** (DESIGN-recorded, DELIVER-built): two new required CI jobs,
  `tenant-isolation-gates` and `console-compat`, mirroring how `invariant-gates`
  carries I1/I3/I4/I7 for the single-tenant case.
- **Rollback contract gains a time-boxed caveat** (this wave's own finding):
  DDD-24's composite-PK migration is expand-only in the D7 sense but not
  purely additive at the schema level. Rollback to a pre-multitenancy binary
  is safe only until a second tenant's accounts exist — after that, rollback
  requires a deliberate contracting-migration release, not Recreate's usual
  unconditional safety.
- New environments `two-tenant` and `legacy-backfilled` (additive to
  `ledger-core`'s matrix, no new concurrency-race environment — I8/I9/I10 are
  correctness gates via the normal acceptance suite, not contention
  properties).

### DISTILL wave
- 18 scenarios across 4 `.feature` files, one walking skeleton chaining
  slice 01→02. 44% error/edge-path ratio (above the 40% floor). Every
  scenario carries exactly one `@contract-shape:` tag.
- Pre-DELIVER RED gate: PASS — 8 passed, 10 failed, every failure
  `MISSING_FUNCTIONALITY`, zero `IMPORT_ERROR`/`FIXTURE_BROKEN`/`SETUP_FAILURE`.
- Final Wave Review Gate: four parallel Haiku reviewers, zero blockers across
  DISCUSS/DESIGN/DEVOPS/DISTILL. One HIGH finding (console-compat scenarios
  assert shape-match, not a byte-for-byte golden diff) accepted as a
  pre-existing, self-flagged DELIVER action item, not a fresh gap.

### DELIVER wave
- **Upstream Issues** (both user-confirmed, tracked not blocking):
  1. Step 02-01 gave `Account` an optional `tenantID` rather than a required
     constructor parameter, to stay within its declared `files_to_modify`.
     Closed by step 02-02, restoring the DESIGN-mandated "no smart-constructor
     path omits it" property before slice 02 shipped.
  2. OPS-13 (above).
- All 12 roadmap steps executed RED→GREEN→COMMIT, zero unresolved deviations
  at close.

---

## Work completed — 12 roadmap steps across 3 slices

Full step-by-step trace: `docs/feature/multitenancy/deliver/execution-log.json`,
`docs/feature/multitenancy/deliver/roadmap.json`.

**Slice 01 — Provision a tenant** (steps 01-01..01-04): composite-PK migration
with `tnt_legacy_seed` backfill; `Tenant` aggregate + I10 uniqueness + sealed
refusal taxonomy extension; `TenantRepository`/`ProvisionTenant` use case;
`POST /tenants` route wired, admin-gated, 5 milestone-01 scenarios un-skipped.

**Slice 02 — Operate within a tenant** (steps 02-01..02-05): I8
construction-time cross-tenant check inside `Post`; tenant-scoped
`AccountRepository`/`TransactionRepository` (I9 rescoping); `requireTenantKey`/
`requireTenantKeyOrOperatorKey` middlewares; tenant-scoped routes + dual-mode
`GET /accounts/{id}/entries` wired, walking skeleton + 7 milestone-02 scenarios
un-skipped; demo tenant credential seeding, `Makefile` `AUTH`/`OPERATOR_AUTH`
split, and the ledgercore suite fix (OPS-10/11/13, scope expanded 2026-09-04).

**Slice 03 — Verify one tenant's books** (steps 03-01..03-03): tenant-scoped
`GET /health/trial-balance`; console verdict dual-mode compat (shared
`verdictHandler` fix covers both routes at once); `demo-04` target and the two
new CI gate jobs (`tenant-isolation-gates`, `console-compat`).

**Post-12-steps**: L1-L6 refactor pass (commit `eb2af61`, extracted a shared
SHA-256-hex primitive into `internal/support`). Full regression: 18/18
multitenancy scenarios (102/102 steps), full `ledgercore` suite pass
(296.8s, CI-blocking `integration` job confirmed restored and holding).

---

## Demo evidence and KPI measurements (this session, 2026-09-04/05)

Full verbatim request/response capture:
`docs/feature/multitenancy/feature-delta.md` § Wave: DELIVER / Demo Evidence.

- **US-1 (provision)**: `POST /tenants` with `demo-operator-key` → `201`,
  distinct `tenant_id`/`tenant_key`.
- **US-2 (isolation)**: two tenants both open account `wallet-1` → both
  `201` — the single-tenant model's `409 account_already_exists` collision
  is structurally impossible, not merely avoided.
- **US-3 (verification)**: `GET /health/trial-balance?tenant_id=...` → `200`,
  scoped verdict names only that tenant.
- **KPI-M2 (tenant onboarding to first transfer)**: `make demo-04` →
  `tenant_onboard_transfer_seconds=0`, well under the 300s threshold
  (mirrors KPI-5).
- **KPI-M1 (cross-tenant access refusal) proof surface**: 18/18 multitenancy
  acceptance scenarios passing, including every adversarial cross-tenant
  read/write scenario in the `tenant-isolation-gates`-covered suite.
- **KPI-M3 (per-tenant trial-balance correctness) proof surface**: same
  18/18 scenario run, including the isolated-verdict scenarios (US-3).
- **KPI-M4 (console-compatibility guardrail) proof surface**: the 3
  `@console-compat` scenarios within the 18, plus the full `ledgercore`
  suite's restored 100% pass rate as the regression proof that unscoped
  `OperatorKey` calls kept working throughout.
- **Gate result: PASS.** All three non-infrastructure user stories
  demonstrated end-to-end against a clean environment; full regression green.

`docs/product/kpi-contracts.yaml` KPI-M1..M4 updated with these measured
baselines (see § SSOT updates below).

---

## Lessons learned

- **DELIVER-discovered gaps in a DESIGN-recorded credential-migration
  decision are a real pattern, not isolated bad luck.** DDD-23's own record
  verified `Makefile` and `router.go` directly, but missed two other
  consumers of the newly-restricted routes: `scripts/race/main.go`
  (OPS-11, found by DEVOPS's own verification) and
  `tests/acceptance/ledgercore/ledger_world.go` (OPS-13, found by DELIVER's
  regression run, and the more consequential of the two — it broke the
  project's CI-blocking `integration` job, not just non-blocking demo/race
  targets). Worth naming for future features: when a DESIGN decision narrows
  which credential a set of routes accepts, the search for "who currently
  calls these routes" needs to cover test-harness/support code
  (`tests/acceptance/*/world.go`-shaped files), not only production
  Makefile/script surfaces — those are exactly the kind of caller that is
  easy to verify-by-grep-and-miss because it isn't "infrastructure" in the
  usual DEVOPS sense.
- **Sub-agents backgrounding slow tests never resume** — a dispatched crafter
  that backgrounds a long-running test and waits for a notification stalls
  permanently; tests inside a dispatched agent must run in the foreground.
  Cost this session two stalled dispatches before being caught.
- **Two connection-drop resume incidents** occurred mid-run (steps 02-04 and
  03-03 each show a duplicate GREEN/COMMIT pair in `execution-log.json` —
  visible directly in the DES trace, not hidden). In each case the
  orchestrator independently re-verified actual repository state (build,
  test suite, git log) before resuming or re-logging, rather than trusting
  the interrupted agent's partial self-report — the same discipline
  `ledger-core`'s own evolution doc names as "Pattern 2."
- **`go test` cache staleness** surfaced during this run as a source of
  false-green readings — a step's test run reported PASS from a stale cached
  result rather than the just-edited code path. Caught by re-running with
  `-count=1`, which is why the final regression evidence in § Demo Evidence
  explicitly used `-count=1` rather than trusting a bare `go test` pass.
- **A dirty Docker volume from a prior run stalled step 03-03's GREEN phase**
  — a leftover `docker compose` volume from an earlier migration attempt left
  the database in a pre-DDD-24 schema shape, producing confusing FK errors
  that looked like a code defect rather than stale state. Resolved by a
  clean `make down` (removing named volumes) before re-running. Worth a
  standing reminder for any feature whose migration changes a table's key
  shape: verify against a genuinely fresh volume, not an assumed-clean one,
  before diagnosing a schema-shaped test failure as a code bug.

---

## Issues encountered (already tracked in feature-delta.md, summarized here)

1. Step 02-01's `Account.tenantID` shipped optional rather than required,
   deviating from the DESIGN-mandated construction-time guarantee to stay
   within `files_to_modify` scope — closed by step 02-02, user-confirmed.
2. OPS-13 — the CI-blocking `integration` job regression, found at step
   02-04, closed at step 02-05, user-confirmed.
3. Two connection-drop resume incidents (steps 02-04, 03-03) — independently
   re-verified, not blindly resumed.
4. `go test` cache staleness — mitigated with `-count=1` on the final
   regression run.
5. Dirty Docker volume at step 03-03 — resolved with a clean `make down`.

None of these left an open gap at close — all five are closed, not carried
forward.

---

## Migrated permanent artifacts

- `docs/architecture/multitenancy/slices/slice-01-provision-a-tenant.md`
- `docs/architecture/multitenancy/slices/slice-02-operate-within-a-tenant.md`
- `docs/architecture/multitenancy/slices/slice-03-verify-one-tenants-books.md`

Migrated from `docs/feature/multitenancy/slices/`, matching the `ledger-core`
finalize precedent (`docs/evolution/2026-08-21-ledger-core.md` § Migrated
permanent artifacts) — slice briefs have lasting scenario/architecture value
for any future continuation of this feature.

`docs/product/architecture/brief.md` § System Architecture / Domain Model /
Application Architecture "Multitenancy (confirmed 2026-09-03)" sections were
already written and cross-referenced during DESIGN itself — checked this
session and confirmed still accurate post-implementation (§ SSOT updates
below); no edit needed, matching the `ledger-core-console` precedent (that
feature's own DESIGN-time confirmation likewise needed no finalize-time
back-propagation).

ADRs (`adr-011-tenant-partition-not-context.md`,
`adr-012-tenant-credential-mechanism.md`,
`adr-013-multitenancy-migration-shape.md`) already live at their permanent
location, `docs/product/architecture/`, this project's established ADR
convention (not `docs/adrs/`) — verified present, not moved.

`docs/product/kpi-contracts.yaml` KPI-M1..M4 already existed (added during
DEVOPS); this session added measured-baseline evidence to each (§ SSOT
updates below).

---

## Not migrated — left in place, and why

- **`docs/feature/multitenancy/devops/environments.yaml`** — left in place,
  not migrated to a permanent location. This mirrors the established
  precedent from both prior features on this project: `ledger-core`'s own
  `docs/feature/ledger-core/devops/environments.yaml` and
  `ledger-core-console`'s `docs/feature/ledger-core-console/environments.yaml`
  were likewise left in their feature workspaces at finalize, not moved —
  this project has no permanent `docs/environments/` or `docs/devops/`
  location, and environments.yaml is explicitly a DISTILL/DELIVER-consumed
  machine artifact whose value is "still the current environment matrix,"
  not lasting reference documentation of the `docs/architecture/` kind. The
  file stays exactly where DEVOPS wrote it, additive to `ledger-core`'s own
  matrix as documented in its own header comment.

## Discarded (not migrated — captured elsewhere, per skill's discard table)

- `docs/feature/multitenancy/design/wave-decisions.md`
- `docs/feature/multitenancy/devops/wave-decisions.md`
- `docs/feature/multitenancy/distill/wave-decisions.md`
- `docs/feature/multitenancy/distill/red-classification.md`

All four are process scaffolding per the `nw-finalize` skill's discard table
(`*/wave-decisions.md` explicitly listed; `red-classification.md` is a
point-in-time pre-DELIVER RED-gate record whose value was consumed at
gate-time — the same judgment `ledger-core` and `ledger-core-console`'s own
finalize passes both made for their equivalent files). Their content is
fully captured in `feature-delta.md`'s own `## Wave: DESIGN|DEVOPS|DISTILL`
narrative sections and in this evolution document. "Discarded" here means
**not migrated to a permanent `docs/` location** — none of these four files
are deleted by this finalize pass; per the skill's own Phase C rule,
`docs/feature/{feature-id}/` wave artifacts (as opposed to session markers)
are not removed. See § Cleanup below for the exact list presented for
approval.

## Explicitly kept in place, not discarded (lean v3.14 permanent artifacts)

- `docs/feature/multitenancy/feature-delta.md` — the single-narrative
  wave record; this evolution document summarizes it, does not replace it.
- `docs/feature/multitenancy/deliver/roadmap.json` and `execution-log.json` —
  **kept as the permanent DES audit trail**, per lean v3.14's own convention
  (the `nw-deliver` skill declares these permanent machine artifacts under
  lean v3.14, postdating the `nw-finalize` skill's older legacy-mode discard
  guidance for these two specific files). Not superseded, not discarded.

---

## Handoff to operations

Production-validated surface (Docker Compose only — no hosted environment):
`POST /tenants` (admin-gated), `POST /accounts`/`POST /transfers`/
`GET /accounts/{id}` (tenant-key-gated), `GET /accounts/{id}/entries`
(dual-mode), `GET /health/trial-balance?tenant_id=` and `GET /console/verdict`
(dual-mode, shared handler). Rollback contract: Recreate, redeploy previous
image tag — **time-boxed**: safe only until a second tenant has accounts;
past that point, rollback requires a deliberate contracting-migration
release, not the previously-unconditional Recreate safety. Migration
`0003_tenants.up.sql` is expand-only in the D7 sense (no `DELETE` from the
entry table), with a `tnt_legacy_seed` backfill for every pre-existing row.

**Before this feature's scope is called fully complete**: the
platform-mediated inter-tenant settlement mechanism (`jobs.yaml` J8) and
console/SPA tenant-awareness (D5) are both deliberately deferred to their own
follow-on features, not gaps in this one.
