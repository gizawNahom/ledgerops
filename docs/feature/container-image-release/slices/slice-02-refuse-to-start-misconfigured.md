# Slice 02 — Know immediately when the image is misconfigured

**Story**: US-2 | **job_id**: J11 | **persona**: P1 | **Priority**: 2 (walking skeleton)

## Goal
Starting the image without its required settings fails at once with one clear
message listing everything missing, instead of starting and failing at the
first operator call.

## IN scope
- Required: `LEDGEROPS_APP_DSN`, `LEDGEROPS_OPERATOR_KEY` (D9 as revised 2026-10-17)
- Optional: `LEDGEROPS_DEMO_TENANT_KEY`; when unset or blank the demo-tenant mapping is off and nothing resolves to `tnt_legacy_seed` through it (existing behavior, `main.go:98-100`)
- Empty and whitespace-only count as unset
- All missing required variables reported together; exit 1 before any database connection
- `serve` refuses to start when `LEDGEROPS_MIGRATE_DSN` is present in its environment, naming the variable, before any database connection (the app must never hold the migration credential)
- No key or DSN value in any log line
- Demo compose already sets all three; `make up` unaffected

## OUT scope
- Password handling (slice 01a)
- Requiring `LEDGEROPS_DEMO_TENANT_KEY` (decided against, OQ-1 closed)
- Changing which routes the keys protect

## Learning hypothesis
Disproves: "the current silent-start behavior (unset operator key makes every
operator route 401, `router.go:291-293`) is acceptable for a published image."
If wiring the check at the entrypoint breaks the demo or any acceptance harness
that starts the server with a partial environment, the entrypoint is carrying
more implicit defaults than recorded.

## Acceptance criteria
- [ ] Each missing required variable causes exit 1 naming it
- [ ] Multiple missing required variables are named in one message
- [ ] Blank required values refused
- [ ] `LEDGEROPS_MIGRATE_DSN` present in the serve environment: exit 1 naming it, no database connection
- [ ] Unset or blank demo tenant key starts normally with the demo mapping off
- [ ] Fully configured start behaves exactly as before
- [ ] No secret in any log line

## Dependencies
None inside the feature. OQ-1 is closed (demo key optional).

## Production data
Run the real binary and the compose stack with each variable removed in turn.

## Effort estimate
At most half a day. Reference class: a startup validation function beside the
existing `probeStartup`.

## Pre-slice SPIKE
Not required.
