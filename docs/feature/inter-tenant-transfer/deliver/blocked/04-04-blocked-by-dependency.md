# Step 04-04 — Verification confirms 4/4 pass, but 3/4 rely on a vacuous-assertion dependency

Step: `inter-tenant-transfer/04-04` — "Wire remaining milestone-04 preservation
scenarios" (4 scenarios in
`tests/acceptance/intertenanttransfer/milestone-04-reverse-after-retry-budget-exhausted.feature`).

## RED (confirmed, `go test ./tests/acceptance/intertenanttransfer/... -run TestMain -count=1 -v`, real Testcontainers Postgres + real chi router)

Full suite: 41 scenarios, 36 passed, 5 failed. All 5 failures are pre-existing
milestone-05 isolation gaps (`unidentified_caller` vs. `transfer_not_found`/
`counterparty_not_found` — an earlier-boundary-middleware ordering issue),
unrelated to this step's scope. All 4 target scenarios are among the 36
passing:

1. **"Reversal never edits or deletes an existing entry"** (feature line 72) —
   passes, but **both** `Then` steps are vacuous placeholders in
   `steps_intertenanttransfer_test.go`: `the original legs remain exactly as
   posted` (line 802) and `the reversal appears as new, additional entries
   only` (line 804) are `func() error { return nil }`. Already flagged in
   `docs/feature/inter-tenant-transfer/distill/red-classification.md` § "The 5
   passing scenarios" (item 3).

2. **"A reversed transfer is not automatically retried"** (feature line 79) —
   `Then the transfer's status remains "reversed"` (line 808) is genuine
   (`w.AssertTransferStatus`). But `And a new transfer to the same
   counterparty requires a fresh idempotency key` (line 806) is also
   `return nil` — **not** previously named in red-classification.md's list of
   5 vacuous-pass scenarios; a new finding of the same class.

3. **"A resend of the same idempotency key after reversal is treated as the
   original request"** (feature line 86) — `Then the response reports the
   same, already-reversed transfer` (line 733) is genuine
   (`w.AssertTransferStatus(StatusReversed)`). But `And no new attempt is made
   on any leg` (line 739) is `return nil` — already flagged in
   red-classification.md (item 2, "no new attempt is made on any leg").

4. **"A reversed transfer's terminal state names the reason"** (feature line
   93) — fully genuine: `Then the response includes reason
   "retry_budget_exhausted"` (line 680) calls `w.AssertReason(reason)`. No gap.

## Why this is not a production-code gap

`handlers.go`'s `getTransferHandler`/`transferViewAnswer` already renders the
`reason` field correctly (proven by scenario 4 passing genuinely), and
`Post`/the coordinator's append-only write discipline (built in 04-01/04-02)
never edits or deletes existing entries by construction — there is no leg- or
entry-mutation code path in `internal/app/transfer_coordinator.go` or
`internal/adapters/http/handlers.go` to change. The remaining 3 gaps are
entirely in the un-editable test-infrastructure file: `Then` steps not yet
wired to the existing entry-log read port (`GET /accounts/{id}/entries`) or to
a leg-attempt-count / idempotency-replay check.

## Scope boundary

This step's boundary rules forbid editing
`tests/acceptance/intertenanttransfer/steps_intertenanttransfer_test.go` and
`world.go` except to escalate. All 3 gaps above live exclusively in that file.

## Escalation

Route to `nw-acceptance-designer` to:

- Wire scenario 1's two `Then` steps (lines 802, 804) to the existing
  `GET /accounts/{id}/entries` port: assert the pre-reversal legs are present
  unchanged, and that reversal entries appear as additional rows (not
  mutations) via `w.InspectEntryLog` or an equivalent composition method.
- Wire scenario 2's `Then` step (line 806) to a real check that a fresh
  idempotency key is required — e.g. issue a second `POST /transfers` to the
  same counterparty reusing the original idempotency key and assert it is
  rejected/distinguished, or assert a fresh key is accepted as a new transfer
  id.
- Wire scenario 3's `Then` step (line 739) to a real leg-attempt-count
  assertion (no new attempt recorded after the resend), mirroring the
  existing `leg 2 was attempted exactly 5 times` convention once that seam is
  wired.

No production code change was required or made for this step — the 4 target
scenarios already report green at the Cucumber level (matching the
orchestrator's prior full-suite verification), but 3 of the 4 do so partly
via undischarged vacuous assertions rather than fully proven behavior.

## DES phase outcome

- RED: EXECUTED / PASS (all 4 target scenarios confirmed passing at Cucumber
  level; zero regressions on the 36 previously-passing scenarios)
- GREEN: EXECUTED / PASS (verification-only; no production code change
  required — `go build ./...` clean, `git diff` empty for
  `internal/adapters/http/handlers.go`)
- COMMIT: this report, `Step-Id: 04-04`, `Task-Id: inter-tenant-transfer`
