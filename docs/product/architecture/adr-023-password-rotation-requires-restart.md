# ADR-023: Password rotation is migrate-then-restart; hot reload is a recorded escape hatch

**Status**: Accepted, 2026-10-17 (DDD-40)
**Owner**: nw-solution-architect (DESIGN, `container-image-release`)

## Context

OQ-9: `ALTER ROLE ... PASSWORD` does not disturb connections already
authenticated, so a running `pgxpool` keeps working; new or recycled
connections (default max lifetime about one hour, or pool growth) use the DSN's
old password and fail.

## Decision

1. Rotation procedure: update the secret store, run `ledgerops migrate` with
   the new password, redeploy the app with the new DSN. A short outage is
   accepted under the existing Recreate strategy (brief § Deployment shape).
2. Runtime authentication failures remain "unavailable", not a refusal
   (ADR-009), logged without secret values. A wrong password at start is a
   named refusal (ADR-018).
3. Documentation (slice 08) states that a running app must be restarted after
   a rotation.
4. Escape hatch, not built: the app re-reads a mounted password file in a
   `BeforeConnect` hook for zero-downtime rotation.

## Alternatives considered

- **Hot reload now.** Adds an app-side variable and attack surface; no story
  asks for it; contradicts "secrets beyond env and file" being out of scope.
- **`/health` reports degraded on auth failure.** Does not heal: restarting
  with the old environment fails again.

## Consequences

- Operators rotating a live deployment see a brief window of intermittent
  failures if they migrate before restarting; the docs order the steps to keep
  it short.
