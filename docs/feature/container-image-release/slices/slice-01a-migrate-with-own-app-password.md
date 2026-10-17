# Slice 01a — Migrate a fresh database with my own app password

**Story**: US-1a | **job_id**: J11 | **persona**: P1 | **Priority**: 1 (walking skeleton)

## Goal
An operator can run the image's `migrate` command against an empty Postgres and
end up with a migrated database whose `ledgerops_app` password is the one they
supplied.

## IN scope
- `migrate` command in the same image, using the embedded SQL (`postgres.Migrate`)
- Password from `LEDGEROPS_APP_PASSWORD` or `LEDGEROPS_APP_PASSWORD_FILE`; trailing newline trimmed
- Refusal (non-zero, before any change) when no usable password; no fallback to `app-secret`
- Database name no longer fixed to `ledgerops` (D5 `current_database()`)
- No window in which `ledgerops_app` accepts an unsupplied password
- `docker-compose.yml`, loadtest compose, test helper and CI set `LEDGEROPS_APP_PASSWORD=app-secret` explicitly; compose `migrate` service moves to the new command
- Wrong password at app start gives a clear refusal with no secret in the log
- A run killed between two migration files leaves no unsupplied password; a run killed during a file leaves a dirty version that `migrate` refuses to continue from, naming it, changing nothing
- Preflight privilege probe: insufficient privilege fails before any schema change ("no changes applied")
- A password file unreadable by the non-root image user gives a named error

## OUT scope
- The one-ledgerops-database-per-cluster refusal and its concurrency guarantee (slice 01b)
- Existing-database upgrade and rotation (slice 04)
- Version output, console, publishing (later slices)
- Any change to the app's runtime role model or grants

## Learning hypothesis
Disproves: "the app password can be made configurable without breaking the
demo path or the append-only guarantee, even though it means changing an
applied migration." If the compose stack or the append-only acceptance suite
goes red, the migration approach (OQ-2) is wrong before anything is published.

Confirms if it succeeds: Go-side password setting is viable and the shipped
migration set can drop its literal.

## Acceptance criteria
- [ ] Empty database ends at schema version 6 with the supplied password working
- [ ] Missing or empty password: non-zero exit, both variables named, database untouched
- [ ] `app-secret` refused unless the operator supplied it
- [ ] Password absent from all output
- [ ] Demo path and CI gates pass unmodified
- [ ] Wrong app password at start exits non-zero without echoing it
- [ ] Dirty version after a mid-file kill: re-run refuses naming the version, changes nothing, `app-secret` still refused
- [ ] Unreadable password file: named error, database unchanged

## Dependencies
`ledger-core` migrations 0001-0006 (shipped). DESIGN answers OQ-2, OQ-3, OQ-8.

## Production data
Run against a real Postgres 16 container (Testcontainers) and against the
compose stack Gizaw uses daily (`make up`); dogfood the same day.

## Effort estimate
At most 1 day. Reference class: `postgres.Migrate` and the test helper already
exist; the new work is a subcommand, one secret read, and one role statement.

## Pre-slice SPIKE
Optional, 1 hour: confirm an edited `0001` is not re-run on an already
migrated database (golang-migrate version bookkeeping), before slice 04 relies
on it.
