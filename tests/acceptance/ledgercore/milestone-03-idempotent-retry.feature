@slice-03 @us-3 @driving_port @real-io
Feature: Retry safely

  US-3 — as an integrating developer, I retry a timed-out transfer without
  double-charging.

  The trust-forming moment of the whole feature (journey post-a-transfer, S5:
  confidence peaks on the retry, not on the happy path). Three behaviours make
  it work: a repeat of the same request posts once, a reused key over a
  different request fails loudly rather than quietly doing the wrong thing, and
  a request with no key is refused outright — optional idempotency is
  idempotency nobody uses.

  The replayed answer is rebuilt from what was recorded, never served from a
  remembered reply (DDD-8). The scenario that pins this compares the replay
  against the entries actually in the ledger, which a stale remembered reply
  would fail.

  Background:
    Given the ledger is running against an empty store
    And a system account "treasury" exists
    And a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists

  @env-clean @contract-shape:bounded-change
  Scenario: Submitting the same transfer twice moves value once
    When the integrator moves 50.00 from "alice" to "bob" under key "k-1"
    And the integrator repeats the same request under key "k-1"
    Then the repeat is answered as a replay
    And both answers name the same transaction
    And both answers are identical
    And the balance of "alice" reads 50.00
    And the ledger holds 4 entries whose amounts sum to zero

  @env-clean @contract-shape:bounded-change
  Scenario: A replay reports the movement that is actually recorded
    When the integrator moves 50.00 from "alice" to "bob" under key "k-1"
    And the integrator repeats the same request under key "k-1"
    Then the legs in the replayed answer match the entries recorded for that transaction

  @error @env-clean @contract-shape:bounded-change
  Scenario: Reusing a key for a different movement fails loudly
    When the integrator moves 50.00 from "alice" to "bob" under key "k-1"
    And the integrator moves 25.00 from "alice" to "bob" under key "k-1"
    Then the second request is refused as a key conflict
    And the balance of "alice" reads 50.00
    And the ledger holds 4 entries whose amounts sum to zero

  @error @env-clean @contract-shape:bounded-change
  Scenario: Reusing a key for the same amount between different accounts fails loudly
    Given a wallet account "carol" exists
    When the integrator moves 50.00 from "alice" to "bob" under key "k-1"
    And the integrator moves 50.00 from "alice" to "carol" under key "k-1"
    Then the second request is refused as a key conflict
    And the balance of "carol" reads 0.00

  @env-clean @contract-shape:bounded-change
  Scenario: The same request written differently is still the same request
    When the integrator moves 50.00 from "alice" to "bob" under key "k-1"
    And the integrator repeats the same request under key "k-1" with its fields reordered and respaced
    Then the repeat is answered as a replay
    And both answers name the same transaction
    And the ledger holds 4 entries whose amounts sum to zero

  @error @env-clean @contract-shape:unbounded-preservation
  Scenario: A transfer submitted without a key is refused
    When the integrator moves 50.00 from "alice" to "bob" under no key
    Then the transfer is refused as missing a key
    And the balance of "alice" reads 100.00
    And the ledger holds 2 entries whose amounts sum to zero

  @error @env-clean @contract-shape:bounded-change
  Scenario: A refused transfer does not consume its key
    When the integrator moves 500.00 from "alice" to "bob" under key "k-1"
    Then the transfer is refused for insufficient funds
    When the integrator moves 50.00 from "alice" to "bob" under key "k-1"
    Then the transfer is accepted
    And the balance of "alice" reads 50.00

  @chaos @env-clean @contract-shape:bounded-change
  Scenario: A key and the movement it guards survive an interruption together or not at all
    When the ledger is killed partway through moving 50.00 from "alice" to "bob" under key "k-1"
    And the ledger is restarted against the same store
    And the integrator moves 50.00 from "alice" to "bob" under key "k-1"
    Then exactly 1 transaction was recorded for that key
    And exactly 1 pair of entries was recorded for that key
    And the balance of "alice" reads 50.00
    And every account's balance equals the sum of its own entries

  @env-contended @kpi-3 @contract-shape:bounded-change
  Scenario: Fifty simultaneous submissions of one key move value once
    When 50 integrators submit the same 50.00 transfer from "alice" to "bob" under key "k-1" at the same moment
    Then exactly 1 transaction was recorded for that key
    And exactly 1 pair of entries was recorded for that key
    And all 50 answers name the same transaction
    And at least 50 submissions were made
    And the balance of "alice" reads 50.00
