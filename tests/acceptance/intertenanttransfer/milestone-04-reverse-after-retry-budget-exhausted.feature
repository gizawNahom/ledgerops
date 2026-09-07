# Slice 04 — Reverse after the retry budget is exhausted. US-4, job J10.
# docs/feature/inter-tenant-transfer/slices/slice-04-reverse-after-retry-budget-exhausted.md

@slice-04 @us-4 @driving_port @env-clean
Feature: A transfer that cannot complete reverses, leaving the sender's wallet whole

  Before: retry-until-settled has no defined endpoint -- a permanently
  failing leg would either retry forever or leave the sender's funds parked
  with no path back.
  After: exhausting the fixed retry budget (N=5) reverses every leg already
  posted, in reverse order, restoring every touched balance to its
  pre-transfer value (D7 -- compensating entries only, nothing edited or
  deleted).

  # Cross-reference (2026-09-07 follow-up fix 3): the "zero legs posted"
  # case is NOT re-scenario'd here. Leg 1 has no retry budget of its own --
  # it posts synchronously inside SendTransfer or the request is refused
  # outright, so there is never a leg-1-exhausted-its-retries state for
  # this file's reversal machinery to reverse FROM. That case is already
  # covered, and belongs, in milestone-02's own
  # "Insufficient sender funds refuses before any leg posts" scenario
  # (tests/acceptance/intertenanttransfer/milestone-02-send-a-transfer-to-a-named-counterparty.feature) --
  # its own Then already asserts "no leg posts", which is exactly the
  # guarantee that no transfer_state/coordinator row is ever created for a
  # transfer whose leg 1 itself fails outright. This file's scope starts
  # one leg later: reversing what DID post once a later leg's retry budget
  # is exhausted.

  @real-io @adapter-integration @contract-shape:bounded-change
  Scenario: Exhausting retries on leg 2 reverses leg 1 only
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon" and leg 2 has failed on all 5 attempts of its retry budget
    When the retry budget is exhausted
    Then the transfer's status becomes "reversed" with reason "retry_budget_exhausted"
    And leg 2 was attempted exactly 5 times before the transfer reversed
    And leg 1 is reversed
    And tenant "tnt_acme"'s wallet balance returns to its pre-transfer value
    And tenant "tnt_beacon"'s wallet balance is unchanged
    And every touched account's trial balance holds after the reversal

  @real-io @contract-shape:bounded-change
  Scenario: Exhausting retries on leg 3 reverses leg 2 then leg 1, in order
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon", leg 1 and leg 2 have posted, and leg 3 has failed on all 5 attempts of its retry budget
    When the retry budget is exhausted
    Then leg 3 was attempted exactly 5 times before reversal began
    And leg 2 is reversed before leg 1 is reversed
    And tenant "tnt_acme"'s wallet balance and the platform account balance both return to their pre-transfer values
    And tenant "tnt_beacon"'s wallet balance is unchanged
    And every touched account's trial balance holds after the reversal

  # Added (2026-09-07 follow-up fix 4): reversal's own exactly-once
  # guarantee under a crash, mirroring milestone-03's forward-leg
  # crash-recovery scenario ("A crash before any Leg 2 attempt is recovered
  # by the retry ticker alone") but on the compensating path -- the crash
  # window this scenario targets sits between leg 2's reversal committing
  # and leg 1's reversal ever being attempted. Uses its own
  # SimulateCrashBeforeReversalAttempt seam (split from the forward-path
  # SimulateCrashBeforeForwardLegAttempt seam in the 2026-09-07 follow-up
  # fix 2 -- the two crash windows are semantically distinct) plus the same
  # RunRetryTickerOnce seam, rather than a fresh primitive.
  @real-io @adapter-integration @contract-shape:bounded-change
  Scenario: A crash between reversing leg 2 and reversing leg 1 resumes without double-reversing leg 2
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon", leg 1 and leg 2 have posted, and leg 3 has failed on all 5 attempts of its retry budget
    And leg 2's reversal has posted and leg 1's reversal attempt never ran, simulating a process crash between the two compensating entries
    When the retry ticker's next tick runs, with no inline attempt ever having occurred
    Then the transfer's status becomes "reversed" with reason "retry_budget_exhausted"
    And leg 2 was reversed exactly once
    And leg 1 is reversed
    And tenant "tnt_acme"'s wallet balance returns to its pre-transfer value
    And every touched account's trial balance holds after the reversal

  @real-io @contract-shape:unbounded-preservation
  Scenario: Reversal never edits or deletes an existing entry
    Given a transfer from "tnt_acme" to "tnt_beacon" that has been reversed
    When the entry log for every account the transfer touched is inspected
    Then the original legs remain exactly as posted
    And the reversal appears as new, additional entries only

  @real-io @contract-shape:unbounded-preservation
  Scenario: A reversed transfer is not automatically retried
    Given a transfer from "tnt_acme" to "tnt_beacon" that has reached status "reversed"
    When the retry ticker runs any number of further ticks
    Then the transfer's status remains "reversed"
    And a new transfer to the same counterparty requires a fresh idempotency key

  @real-io @contract-shape:unbounded-preservation
  Scenario: A resend of the same idempotency key after reversal is treated as the original request
    Given a transfer from "tnt_acme" to "tnt_beacon" that has reached status "reversed" under idempotency key "k1"
    When tenant "tnt_acme" resends the identical request with idempotency key "k1"
    Then the response reports the same, already-reversed transfer
    And no new attempt is made on any leg

  @real-io @contract-shape:unbounded-preservation
  Scenario: A reversed transfer's terminal state names the reason
    Given a transfer from "tnt_acme" to "tnt_beacon" that has reached status "reversed"
    When the transfer is queried
    Then the response includes reason "retry_budget_exhausted"

  # RESOLVED (was SPECIFICATION_AMBIGUITY, nw-at-completeness-check
  # taxonomy): DESIGN's Amendment 3 (design/wave-decisions.md) closes the
  # residual gap brief.md § Retry and reversal mechanics named but did not
  # resolve. Named terminal state: a fifth, disjoint app.TransferStatus
  # value, "reversal_failed" -- not a reason riding on "reversed" -- so a
  # caller checking only status == "reversed" cannot mistake an incomplete
  # compensation for a completed one. The reason string names which leg's
  # compensation exhausted its budget ("leg1_reversal_retry_budget_exhausted"
  # / "leg2_reversal_retry_budget_exhausted"); the coordinator halts once one
  # reversal step exhausts, it does not attempt the next leg's reversal.
  @real-io @contract-shape:bounded-change
  Scenario: A reversal that itself exhausts its own retry budget is a named, distinguishable state
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon" and the compensating reversal of leg 1 itself has failed on all 5 attempts of its own retry budget
    When the reversal's own retry budget is exhausted
    Then the transfer's status becomes "reversal_failed" with reason "leg1_reversal_retry_budget_exhausted"
    And the reversal of leg 1 was attempted exactly 5 times before failing

  # Added for AT-completeness parity with this file's own leg 2 vs leg 3
  # ordering pair (scenarios 1 and 2 above) -- Amendment 3 names
  # leg1_reversal_retry_budget_exhausted and leg2_reversal_retry_budget_exhausted
  # as the two distinguishable reasons, since compensation reverses leg 2 then
  # leg 1 in sequence (US-4) and either step can independently exhaust its own
  # budget. Fits the existing Given/When vocabulary unchanged -- no new World
  # method required, same seeding + ticker seam the leg 1 variant above uses.
  @real-io @contract-shape:bounded-change
  Scenario: A reversal of leg 2 that itself exhausts its own retry budget is a named, distinguishable state
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon" and the compensating reversal of leg 2 itself has failed on all 5 attempts of its own retry budget
    When the reversal's own retry budget is exhausted
    Then the transfer's status becomes "reversal_failed" with reason "leg2_reversal_retry_budget_exhausted"
    And the reversal of leg 2 was attempted exactly 5 times before failing
