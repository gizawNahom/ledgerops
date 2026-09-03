# DEVOPS Decisions — multitenancy

Full DEVOPS narrative lives in `docs/feature/multitenancy/feature-delta.md`
§§ Wave: DEVOPS (this project's single-narrative convention, confirmed by
the `ledger-core` precedent) — this file is the mandatory cross-reference
summary the `nw-devops` skill requires.

## Key Decisions

- [OPS-1..OPS-9] Deployment target, orchestration, CI/CD platform,
  observability stack, deployment strategy, continuous learning, branching
  strategy, and mutation testing strategy are all **unchanged carry-forwards**
  from `ledger-core`'s own DEVOPS wave — DESIGN's system-architecture leg
  confirmed zero new system-level footprint, so none of these needed
  re-deciding (see: `feature-delta.md` § OPS decisions)
- [OPS-10] DDD-23 Option C implementation spec: `LEDGEROPS_DEMO_TENANT_KEY`
  env var + `Makefile` `AUTH`/`OPERATOR_AUTH` split, seeded to
  `tnt_legacy_seed` by DDD-24's migration (see: `feature-delta.md` § OPS
  decisions, § Coexistence matrix)
- [OPS-11] `scripts/race/main.go`'s `RACE_OPERATOR_KEY` default is a gap
  DDD-23's text didn't cover — repointed to a new `RACE_TENANT_KEY`. This
  wave's own finding, verified directly against the script, not assumed from
  DESIGN's decision record (see: `feature-delta.md` § OPS decisions, OPS-11)
- [OPS-12] Two new required CI jobs, `tenant-isolation-gates` and
  `console-compat`, mirroring how `ledger-core`'s `invariant-gates` carries
  I1/I3/I4/I7 (see: `feature-delta.md` § CI/CD pipeline outline)
- New environments `legacy-backfilled` and `two-tenant`, additive to
  `ledger-core`'s matrix, no new concurrency-race environment introduced
  (see: `docs/feature/multitenancy/devops/environments.yaml`)
- New demo target `demo-04` (tenant onboarding → first transfer), instrumenting
  the second outcome KPI (see: `feature-delta.md` § Monitoring contracts)
- Rollback contract gains a time-boxed caveat: safe only until a second
  tenant's accounts exist, because DDD-24's composite-PK migration is a
  structural key-shape change, not purely additive (see: `feature-delta.md`
  § Deployment strategy)

## Infrastructure Summary

- **Deployment**: none hosted, Docker Compose locally, Recreate strategy —
  unchanged from `ledger-core`, with the new rollback time-boxing caveat above.
- **CI/CD**: GitHub Actions, ten required jobs on every push (was eight),
  trunk-based, unchanged branching model.
- **Observability**: `slog` JSON logs + Prometheus exposition, unchanged
  tools; one new log field (`tenant_id`), no new metric labels (deferred —
  no hosted environment to justify the cardinality cost).
- **Mutation testing**: nightly-delta, already locked in project `CLAUDE.md`,
  unchanged; scope automatically extends to the new `Tenant` aggregate.

## Constraints Established

- `LEDGEROPS_DEMO_TENANT_KEY` env var, mirroring `LEDGEROPS_OPERATOR_KEY`'s
  existing pattern, seeded to `tnt_legacy_seed`
- `Makefile`'s `AUTH` variable repointed to the demo tenant credential's
  value; recipe bodies of `demo-01..03`/`chaos-01` stay byte-for-byte
  unchanged; new `OPERATOR_AUTH` variable retains the unscoped `OperatorKey`
  path for provisioning and console-compat scenarios
- `scripts/race/main.go` gains `RACE_TENANT_KEY`, used by `race-02`/`race-03`
  for the now-`tenant_key`-only `POST /accounts`/`POST /transfers` calls
- Rollback to a pre-multitenancy binary is safe only until a second tenant
  has accounts — a new, previously-absent operational constraint
- Console byte-identical compatibility (unscoped `OperatorKey` to
  `GET /console/verdict`, `GET /accounts/{id}/entries`,
  `GET /health/trial-balance`) is now CI-gated (`console-compat` job), not
  manual-dogfood-only
- Mutation testing strategy (nightly-delta) confirmed applicable, no
  `CLAUDE.md` re-write needed — already correct project-wide

## Upstream Changes

One: the rollback time-boxing caveat (§ Deployment strategy in
`feature-delta.md`) is new information not present in DESIGN's DDD-24
migration-shape decision. This narrows an operational safety window rather
than requiring an architecture change, so no
`docs/feature/multitenancy/devops/upstream-changes.md` file was required
per the skill's back-propagation contract (that file is only mandatory when
infrastructure constraints require an architecture change — this does not).

## Judgment Calls Made This Wave (flagged for review before DISTILL)

- Exact new environment names: `two-tenant`, `legacy-backfilled`
- Exact new CI job names: `tenant-isolation-gates`, `console-compat`
- New demo target number: `demo-04` (next available after `demo-03`)
- The `< 300s` numeric threshold applied to the tenant-onboarding KPI —
  anchored to DISCUSS's own "mirrors KPI-5's pattern" framing, not
  independently invented
- OPS-11's `scripts/race/main.go` fix — a gap DESIGN's DDD-23 record did not
  name, surfaced by this wave's own direct code verification

## SSOT Updates (back-propagation)

- `docs/product/kpi-contracts.yaml` — extended with three new KPI entries
  (`feature: multitenancy`) plus a console-compatibility guardrail entry.
  See that file's own diff for field names, thresholds, and CI job mapping.
- `docs/product/architecture/brief.md` § deployment topology — **no-op,
  checked explicitly, not silently skipped.** DESIGN's system-architecture
  leg already confirmed and recorded "zero new system-level architecture
  concern" in that file (§ System Architecture / "Multitenancy, confirmed
  2026-09-03"); nothing in this DEVOPS wave changes the deployment topology
  it describes (one binary, one PostgreSQL instance, no hosted environment).
  No edit made.

## Peer Review

Not invoked. None of the per-wave `nw-platform-architect-reviewer` trigger
conditions apply: no novel deployment target, no new CI/CD framework
(GitHub Actions extended, not replaced), no observability-stack rewrite (one
log field added to an existing scaffold), no security-posture change beyond
DESIGN's already-decided credential mechanism (DDD-22/DDD-23). Default
(skip, proceed to DISTILL) taken per the skill's explicit trigger list.

## Contradiction Check

None found against DESIGN. DESIGN's own system-architecture leg confirmed
zero new system-level footprint before this wave began; every decision above
is either an unchanged carry-forward from `ledger-core`'s DEVOPS wave or a
mechanical implementation spec for a decision DESIGN already made (DDD-23),
plus two genuinely new findings (OPS-11's race-script gap, the rollback
time-boxing caveat) that narrow operational scope without contradicting any
architectural commitment.
