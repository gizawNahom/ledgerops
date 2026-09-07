# Slice 03 — Retry a stalled leg. US-3, job J10.
# docs/feature/inter-tenant-transfer/slices/slice-03-retry-a-stalled-leg.md
#
# Two distinguished idempotency mechanisms are exercised across this file
# (brief.md § For Acceptance Designer): per-leg replay via IdempotencyStore's
# synthesized keys (every scenario below), distinct from slice 02's
# transfer-level replay via transfer_state's own unique constraint.

@slice-03 @us-3 @driving_port @env-clean
Feature: A stalled leg retries until it settles, without the sender ever seeing a bare error

  Before: a transfer spanning three ledgers has no notion of a recoverable,
  in-progress failure -- a failed leg would surface as an undifferentiated
  error, indistinguishable from a permanent one.
  After: a stalled leg is visible as "retrying", then "settled" once the
  retry succeeds, within the fixed 5-attempt budget (1s/2s/4s/8s backoff).

  @real-io @adapter-integration @contract-shape:bounded-change
  Scenario: A stalled leg 2 retries to settlement
    Given a cross-tenant transfer from "tnt_acme" to "tnt_beacon" whose leg 1 has posted
    And leg 2's first attempt fails with a simulated transient fault
    When the transfer is queried immediately after the failed attempt
    Then the transfer's status is "retrying"
    And leg 1 remains "posted"
    When leg 2's retry succeeds
    Then the transfer's status becomes "settled"

  @real-io @contract-shape:bounded-change
  Scenario: A stalled leg 3 does not disturb already-posted legs 1 and 2
    Given a cross-tenant transfer from "tnt_acme" to "tnt_beacon" whose leg 1 and leg 2 have posted
    And leg 3's first attempt fails with a simulated transient fault
    When the transfer is queried
    Then the transfer's status is "retrying"
    And leg 1 and leg 2 remain "posted", unchanged

  @real-io @contract-shape:unbounded-preservation
  Scenario: Retries never produce a duplicate posted leg
    Given a cross-tenant transfer from "tnt_acme" to "tnt_beacon" whose leg 2 failed once and was retried successfully
    When the platform account's entry log is inspected
    Then exactly one posted transaction exists for leg 2 of that transfer

  @real-io @contract-shape:unbounded-preservation
  Scenario: The sender never sees a bare error while a leg is retrying within budget
    Given a cross-tenant transfer from "tnt_acme" to "tnt_beacon" whose leg 2 is retrying within its 5-attempt retry budget
    When tenant "tnt_acme" queries the transfer
    Then the response is 200 with status "retrying"
    And the response is never a 5xx or a bare error

  @real-io @contract-shape:unbounded-preservation
  Scenario: Funds stay parked, not lost, while a leg is retrying
    Given tenant "tnt_acme" sent 50.00 to "tnt_beacon" and leg 2 is retrying
    When the platform account's balance is inspected
    Then it reflects exactly 50.00 received from "tnt_acme" via leg 1, pending onward movement

  @real-io @adapter-integration @contract-shape:bounded-change
  Scenario: A crash before any Leg 2 attempt is recovered by the retry ticker alone
    Given a cross-tenant transfer from "tnt_acme" to "tnt_beacon" whose leg 1 has posted
    And leg 2's inline attempt never ran, simulating a process crash immediately after leg 1's commit
    When the retry ticker's next tick runs, with no inline attempt ever having occurred
    Then the transfer reaches "settled" or "reversed" within the bounded recovery latency
    And leg 2 was attempted exactly once by the ticker's own claim

  @real-io @contract-shape:bounded-change
  Scenario: The retry ticker claims at most its own batch limit of due transfers per tick
    Given more than the ticker's batch limit of cross-tenant transfers are simultaneously due for a retry attempt
    When one retry ticker tick runs
    Then exactly the batch limit of transfers were claimed and attempted
    And the remainder are claimed on a later tick, not left permanently unclaimed

  @real-io @contract-shape:unbounded-preservation
  Scenario: Exhausting attempt 1 and 2 before succeeding on attempt 3 is visible as retrying, then settled
    Given a cross-tenant transfer from "tnt_acme" to "tnt_beacon" whose leg 2 fails on its first two attempts
    When the transfer is queried after the second failed attempt
    Then the transfer's status is "retrying"
    When leg 2's third attempt succeeds
    Then the transfer's status is "settled"
