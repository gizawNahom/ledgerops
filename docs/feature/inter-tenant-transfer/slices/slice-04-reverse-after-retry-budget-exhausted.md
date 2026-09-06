# Slice 04 — Reverse a transfer after the retry budget is exhausted

**Story**: US-4 | **job_id**: J10 | **persona**: P1

## Goal
When Leg 2 or Leg 3 keeps failing past the fixed retry count, the coordinator
compensates — reversing completed legs in reverse order — and the sender's
own wallet is made whole, with a terminal, explained status.

## IN scope
- Bounded fallback: after N failed retries (N fixed, from slice 03), run compensating legs in reverse order
- `GET /transfers/{transfer_id}` reports `status: reversed` with `reason: retry_budget_exhausted`
- Sender's own wallet balance restored to its pre-transfer value

## OUT scope
- Any automatic re-attempt of the same transfer after reversal
- Notifying the counterparty tenant of the failed attempt (no story asks for it; alias owner already knows via their own `GET`)

## Learning hypothesis
Disproves: "compensation can safely run without knowing exactly which legs
actually completed." If reversing ever attempts to reverse a leg that never
posted (or skips one that did), that is signal the coordinator's own state
tracking (slice 02/03) is not authoritative enough to drive compensation
safely — the fix belongs in slice 02/03's state shape, not here.

Confirms if it succeeds: an injected permanent failure (beyond the retry
budget) on Leg 2 or Leg 3 always ends in `reversed`, with the sender's wallet
balance identical to its value before the transfer was sent, and the ledger's
own entries proving it (not merely the reported balance).

## Acceptance criteria
- [ ] Exhausting retries on Leg 2 reverses Leg 1 only (Leg 2 never posted); sender's wallet returns to its pre-transfer balance
- [ ] Exhausting retries on Leg 3 reverses Leg 2 then Leg 1, in that order; sender's wallet returns to its pre-transfer balance; platform account returns to its pre-transfer balance
- [ ] `GET /transfers/{transfer_id}` reports `status: reversed` and `reason: retry_budget_exhausted`, never a bare error or a silent hang
- [ ] The reversed transfer's legs remain in the append-only entry log exactly as posted (D7) — reversal adds compensating entries, never edits or deletes
- [ ] A reversed transfer cannot be retried automatically; a new transfer requires a new `Idempotency-Key`

## Dependencies
Slice 03 (the fixed retry count and per-leg state must already exist).

## Effort estimate
≤1 day. Reference class: no direct precedent in this codebase (first
compensating-transaction logic); budgeted at the upper end of "≤1 day"
accordingly, with the pre-slice spike below reducing that risk.

## Pre-slice SPIKE
Recommended: confirm the exact reverse-order compensation sequence and its
interaction with D7 (append-only entries) before implementation — reversal
must be expressed as new entries, never as an edit to what slice 02/03 already
posted.
