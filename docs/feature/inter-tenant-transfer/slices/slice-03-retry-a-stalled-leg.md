# Slice 03 — Retry a stalled leg until it settles

**Story**: US-3 | **job_id**: J10 | **persona**: P1

## Goal
When Leg 2 or Leg 3 fails once (simulated transient failure), the
coordinator retries automatically and the sender sees `status: retrying`,
never a bare error, while funds stay parked safely in the platform account.

## IN scope
- Coordinator retry loop for Leg 2 and Leg 3, up to a fixed retry count
- `GET /transfers/{transfer_id}` reports `status: retrying` with per-leg detail while retries are in flight
- Funds remain in the platform's own ledger (Leg 2's destination) between attempts — never double-posted, never lost

## OUT scope
- What happens once the fixed retry count is exhausted (slice 04)
- Any user-facing control to cancel or force-retry manually

## Learning hypothesis
Disproves: "retry-until-settled can be implemented as a naive re-call of the
failed leg with no idempotency guard." If a retried Leg 2 or Leg 3 can produce
a second posted transaction for the same `transfer_id`, that disproves the
approach — I7's idempotency-key discipline must extend to the coordinator's
own retries, not just to the original caller's request.

Confirms if it succeeds: an injected transient failure on Leg 2 or Leg 3 is
retried to a successful `settled` state with exactly one posted transaction
per leg, no duplicates, no manual intervention.

## Acceptance criteria
- [ ] An injected Leg 2 failure results in `status: retrying`, then `status: settled` once the retry succeeds
- [ ] An injected Leg 3 failure behaves identically, with Leg 1 and Leg 2 already settled and unaffected
- [ ] No retry produces a duplicate posted transaction for any leg
- [ ] The sender never sees a bare error response while retries remain within budget
- [ ] Funds are provably present in the platform account (Leg 2's destination) throughout retrying, not double-counted or missing

## Dependencies
Slice 02 (the coordinator and the three-leg happy path must exist).

## Effort estimate
≤1 day. Reference class: comparable to `ledger-core` slice 03
(`idempotent-retry`) — same I7 discipline, applied to a coordinator-driven
retry instead of a caller-driven one.

## Pre-slice SPIKE
Not required — direct precedent in slice 03 of `ledger-core`
(idempotency-key-based dedup), reused rather than reinvented.
