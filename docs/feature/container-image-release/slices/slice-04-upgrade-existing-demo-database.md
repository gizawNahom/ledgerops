# Slice 04 — Upgrade a demo database in place and set my own password

**Story**: US-4 | **job_id**: J13 | **persona**: P2 | **Priority**: 4

## Goal
An operator with a database created by the demo stack (role password
`app-secret`) can run the published image's `migrate` against it, get every
pending migration, and replace the app password, with no change to history.

## IN scope
- Existing `ledgerops_app` role gets the supplied password (always applied)
- Pending migrations applied over existing entries and balances
- Re-run with same inputs is a successful no-op; later run with a new password rotates
- Append-only guarantee holds after upgrade
- Insufficient privilege on the privileged role fails clearly, old password intact, a preflight privilege probe fails before any schema change and the message states "no changes applied"
- Doc note: a running app must be restarted after rotation (OQ-9)
- Upgrade leg added to the release smoke test

## OUT scope
- Downgrade or rollback of schema (migrations are expand-only; rollback is the previous image)
- Zero-downtime rotation for a running app
- Migrating from non-ledgerops data

## Learning hypothesis
Disproves: "the D8 upgrade promise holds for databases created before this
feature." If a database seeded with `app-secret` at schema version 4 loses
entries, breaks the trial balance, or ends with the append-only guard weakened
after `migrate`, the edited `0001` or the Go-side role change is unsafe (OQ-2).

## Acceptance criteria
- [ ] Demo-seeded database at version 4 reaches version 6 with entries, balances and zero trial balance unchanged
- [ ] `app-secret` refused and the new password accepted afterwards
- [ ] Idempotent re-run; successful rotation to a third password
- [ ] UPDATE and DELETE on entries refused for `ledgerops_app` after upgrade
- [ ] Missing role privilege: non-zero exit naming it before any schema change, "no changes applied", old password still works

## Dependencies
Slice 01 (command and password handling). Slice 03 for the smoke leg.

## Production data
Seed the starting database with the actual demo stack at the previous commit
(`make up`, `demo-01`, `demo-04`), snapshot it, and upgrade the snapshot; not a
synthetic fixture.

## Effort estimate
At most 1 day. Reference class: slice 01 plus an existing-role branch and
fixture seeding.

## Pre-slice SPIKE
Not required if slice 01's optional spike on edited `0001` was done.
