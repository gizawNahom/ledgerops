# Step 03-04 — Blocked by test-infrastructure dependency

Step: `inter-tenant-transfer/03-04` — "Wire remaining milestone-03 observational
scenarios" (5 scenarios in
`tests/acceptance/intertenanttransfer/milestone-03-retry-a-stalled-leg.feature`).

## RED (confirmed, `go test ./tests/acceptance/intertenanttransfer/... -run TestMain -count=1`)

Full-suite baseline unchanged at 24/41 passing (same as pre-step). Of the 5
target scenarios:

1. **"A stalled leg 3 does not disturb already-posted legs 1 and 2"** — RED.
   `Given ... whose leg 1 and leg 2 have posted` (steps file lines 164-167) is
   byte-identical to `Given ... whose leg 1 has posted` (line 155-158) — it
   never waits for leg 2 to actually post before the next Given step injects a
   leg-3 fault. `When the transfer is queried` (line 305) is a bare,
   non-polling query, unlike its sibling at line 319
   (`PollTransferUntilStatusLeaves`, added in 03-03's own back-propagation
   fix). Observed failure: leg2 itself still `"pending"` at query time — not a
   leg-3-specific bug in `attemptLeg`/`GetTransfer`.

2. **"Retries never produce a duplicate posted leg"** — RED, vacuous
   assertion. `Then exactly one posted transaction exists for leg 2` (line
   512-514) calls `AssertLegStatus(2, LegPosted)` against the transfer's own
   view — it never queries the platform account's entry log, so it can never
   prove "exactly one posted transaction." Matches the pre-DELIVER gate's own
   "5 vacuous-pass scenarios" classification
   (`docs/feature/inter-tenant-transfer/distill/red-classification.md`).

3. **"The sender never sees a bare error while a leg is retrying within
   budget"** — RED. `Given ... whose leg 2 is retrying within its 5-attempt
   retry budget` (line 178-181) never calls `InjectLegFault` — plain
   `seedSettlingTransfer`, no fault armed — so leg 2 posts normally and the
   transfer never reaches `"retrying"`.

4. **"Funds stay parked, not lost, while a leg is retrying"** — reports green,
   but vacuously: `Then it reflects exactly ... via leg 1, pending onward
   movement` (line 530-531) and both of its `When` steps (`the platform
   account's balance is inspected` / `entry log is inspected`, lines 663 and
   665) are `return nil` placeholders. No real balance assertion exists.

5. **"Exhausting attempt 1 and 2 before succeeding on attempt 3..."** — RED.
   `Given ... whose leg 2 fails on its first two attempts` (line 221-224) is
   again the plain `seedSettlingTransfer` with no fault injection — the
   transfer settles on the first attempt.

## Why this is not a production-code gap

`attemptLeg` / `consumeInjectedLegFault` / `GetTransfer` already handle any
leg number generically — proven by the already-passing "A stalled leg 2
retries to settlement" scenario, which exercises the identical code path with
real fault injection and a real bounded poll. No leg-number-specific branch
exists in `internal/app/transfer_coordinator.go` that treats leg 3
differently from leg 2. `internal/adapters/http/handlers.go`'s
`transferViewAnswer`/`getTransferHandler` already renders whatever status the
coordinator reports, unconditionally.

## Scope boundary

This step's boundary rules forbid editing
`tests/acceptance/intertenanttransfer/steps_intertenanttransfer_test.go` and
`world.go` except to escalate. All 5 gaps above live exclusively in those two
files (placeholder Given steps that don't inject the fault their own Gherkin
text describes, one missing bounded-poll `When` step, and vacuous `Then`/
`When` placeholders never wired to a real entries/balance query).

## Escalation

Route to `nw-acceptance-designer` to:

- Add a leg-parameterized seed/fault-injection Given step (or extend
  `seedSettlingTransfer`) that actually injects the described fault(s) and
  waits for the preceding leg to post before returning, for scenarios 1, 3,
  and 5.
- Wire scenario 2's `Then` step to a real query over the platform account's
  entry log (`GET /accounts/{id}/entries`, already an existing port) counting
  posted transactions for leg 2's synthesized idempotency key, instead of
  reusing `AssertLegStatus`.
- Wire scenario 4's `When`/`Then` steps to a real `GET /accounts/{id}`
  balance read instead of the current `return nil` placeholders.

No production code change was required or made for this step — GREEN could
not be reached because the preconditions/assertions themselves never
exercise the states they claim to.

## DES phase outcome

- RED: EXECUTED / PASS (failure reasons above confirmed against real Postgres
  + real HTTP)
- GREEN: EXECUTED / FAIL (no code change closes the gap; blocked upstream)
- COMMIT: this report, `Step-Id: 03-04`, `Task-Id: inter-tenant-transfer`
