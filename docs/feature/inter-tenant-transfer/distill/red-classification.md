# Pre-DELIVER Fail-for-the-Right-Reason Gate — inter-tenant-transfer

Run: `go test ./tests/acceptance/intertenanttransfer/... -run TestMain` against
real PostgreSQL 16 (Testcontainers) and the real `chi` router, 2026-09-07.

**Result: 38 scenarios, 0 undefined steps, 33 failed, 5 passed (see below),
0 compile errors, 0 panics.**

## Classification summary

| Class | Count | Example |
|---|---|---|
| `MISSING_FUNCTIONALITY` (HTTP 501, `__SCAFFOLD__`) | 24 | Every scenario reaching `POST /tenant-links`, `POST /counterparties`, the cross-tenant `POST /transfers` branch, or `GET /transfers/{id}`'s handler body |
| `MISSING_FUNCTIONALITY` (disclosed fault-injection seam, `ErrFaultInjectionNotWired`) | 4 | milestone-03's crash-recovery and batch-limit scenarios, milestone-04's reversal-exhaustion scenarios |
| `MISSING_FUNCTIONALITY` (wrong-boundary middleware, disclosed) | 5 | milestone-05's isolation scenarios fail at `unidentified_caller` (401) rather than a clean `get_transfer` 501, because the temporary placeholder middleware (`requireTenantKeyOrOperatorKey`, see `atdd-infrastructure-policy.md`) does not resolve an unlinked third tenant the way the eventual `requireTransferParty` will — still a correct RED (the feature is genuinely unimplemented), the specific failure mode is just one layer earlier than the handler |
| **Vacuous-assertion pass** (flagged, not IMPORT_ERROR/FIXTURE_BROKEN) | 5 | Scenarios whose sole currently-unmet Then step is a documented placeholder returning `nil` unconditionally (e.g. "the link has no expiry or usage limit") — see `wave-decisions.md` § Pre-DELIVER gate |

Zero `IMPORT_ERROR` / `FIXTURE_BROKEN` / `SETUP_FAILURE`. Zero
`WRONG_ASSERTION` / `OBSERVABLE_NOT_AT_PORT` — every assertion reads a
port-exposed observable (HTTP response body field), never an internal
struct.

## The 5 passing scenarios — why, and what DELIVER should tighten

These pass today because every Then step they exercise happens to be a
documented no-op placeholder (`return nil` with a comment explaining why),
not because the feature works:

1. `milestone-01`: "the link has no expiry or usage limit" — no dedicated
   response field exists yet to assert against; D9 promises there never will
   be an expiry field, so this assertion may stay a documentation-only step
   permanently, or DELIVER may add a field DISTILL should then assert on.
2. `milestone-02`/`milestone-04`: "exactly one set of legs exists for that
   transfer id", "no new attempt is made on any leg" — read-only assertions
   this session did not wire to a real count query (would require a second
   driving-port read this suite does not yet make).
3. `milestone-04`: "the original legs remain exactly as posted", "the
   reversal appears as new, additional entries only" — need the entry-log
   read port (`GET /accounts/{id}/entries`, existing) wired into these
   specific steps; deferred, flagged rather than silently accepted.

None of these are `IMPORT_ERROR`/`FIXTURE_BROKEN` — the scenarios reach
their Given/When steps for real (real HTTP, real Postgres) and fail
elsewhere in the same scenario where a real assertion exists. Recorded here
so DELIVER's PREPARE phase does not mistake "5 passed" for "5 scenarios
already correctly implement the feature."

## Gate verdict

**PASS** — every failing scenario fails for a stated, correct reason
(missing implementation or a disclosed, named test-infrastructure gap).
Handoff to DELIVER is not blocked. The 5 vacuous-pass scenarios and the
fault-injection seam are carried forward as explicit DELIVER-phase
obligations (`wave-decisions.md` § Known gap), not silently accepted as
done.
