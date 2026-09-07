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

  @real-io @adapter-integration @contract-shape:bounded-change
  Scenario: Exhausting retries on leg 2 reverses leg 1 only
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon" and leg 2 has failed on all 5 attempts of its retry budget
    When the retry budget is exhausted
    Then the transfer's status becomes "reversed" with reason "retry_budget_exhausted"
    And leg 2 was attempted exactly 5 times before the transfer reversed
    And leg 1 is reversed
    And tenant "tnt_acme"'s wallet balance returns to its pre-transfer value

  @real-io @contract-shape:bounded-change
  Scenario: Exhausting retries on leg 3 reverses leg 2 then leg 1, in order
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon", leg 1 and leg 2 have posted, and leg 3 has failed on all 5 attempts of its retry budget
    When the retry budget is exhausted
    Then leg 3 was attempted exactly 5 times before reversal began
    And leg 2 is reversed before leg 1 is reversed
    And tenant "tnt_acme"'s wallet balance and the platform account balance both return to their pre-transfer values

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
  @real-io @contract-shape:unbounded-preservation
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
  @real-io @contract-shape:unbounded-preservation
  Scenario: A reversal of leg 2 that itself exhausts its own retry budget is a named, distinguishable state
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon" and the compensating reversal of leg 2 itself has failed on all 5 attempts of its own retry budget
    When the reversal's own retry budget is exhausted
    Then the transfer's status becomes "reversal_failed" with reason "leg2_reversal_retry_budget_exhausted"
    And the reversal of leg 2 was attempted exactly 5 times before failing
