# Slice 01b — One ledgerops database per cluster

**Story**: US-1b | **job_id**: J11 | **persona**: P1 (Tomasz, P2, is the main actor in the examples) | **Priority**: 2 (after 01a; off the walking-skeleton path)

## Goal
`migrate` refuses, before any change and with a message naming the remedies, to
create a second ledgerops database in a Postgres cluster whose `ledgerops_app`
role already exists, and two simultaneous first migrations cannot both succeed.

## IN scope
- First ledgerops database in a cluster may have any name
- Fresh migrate of an empty database refused when `ledgerops_app` already exists in the cluster; role, password and both databases unchanged
- A database already carrying the ledgerops schema (upgrade, rotation, re-run) is not refused
- Refusal message names the remedies: migrate the owning database, use a separate cluster, or privileged `DROP ROLE ledgerops_app` when the owning database no longer exists
- Atomic role claim (DDD-48): exactly one of two simultaneous first migrations succeeds, neither left dirty

## OUT scope
- Password handling, dirty-state refusal, preflight probe (slice 01a)
- Any override flag for sharing a role across databases (decided against)
- Upgrade of existing databases (slice 04)

## Learning hypothesis
Disproves: "a cluster-global role can be claimed safely by a Go-side check
without a race." If two concurrent migrations both pass the existence check, or
one leaves a dirty state, the claim must move into the database as a single
atomic statement before slice 04 relies on the role-ownership rule.

Confirms if it succeeds: the cluster rule is enforceable at migrate time with
no extra operator input.

## Acceptance criteria
- [ ] First database in a cluster works with a non-default name (`kestrel_ledger`)
- [ ] Second empty database in a cluster with the role: non-zero, names the conflict and the remedies, role/password/both databases unchanged
- [ ] Already-migrated database in the same cluster is not refused
- [ ] Two simultaneous first migrations: exactly one succeeds, the other refuses with the cluster-rule message, neither dirty

## Dependencies
Slice 01a (the `migrate` command and password handling). Blocks nothing on the
walking skeleton; slice 04 should follow it.

## Production data
Run against a real Postgres 16 container (Testcontainers) in a dedicated
cluster, and against the compose stack's Postgres with a second database
created by hand; dogfood the same day.

## Effort estimate
At most 1 day. Reference class: one catalog existence check plus an atomic
claim, on top of 01a's command; concurrency test is the long pole.

## Pre-slice SPIKE
Not required.
