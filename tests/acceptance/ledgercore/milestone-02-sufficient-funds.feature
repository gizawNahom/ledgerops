@slice-02 @us-2 @driving_port @real-io
Feature: Reject insufficient funds

  US-2 — as an integrating developer, I never ship a negative wallet balance,
  even under concurrent spending.

  A wallet may not go below zero. A system account may, by design: it is the
  counterparty value enters the ledger from, and its negative balance is the
  record of how much value exists. Asserting that difference is the point of
  this slice, not a side note.

  Background:
    Given the ledger is running against an empty store
    And a system account "treasury" exists

  @error @env-clean @contract-shape:unbounded-preservation
  Scenario: Spending more than a wallet holds is refused with the shortfall
    Given a wallet account "alice" funded with 10.00
    And a wallet account "bob" exists
    When the integrator moves 50.00 from "alice" to "bob" under key "t-1"
    Then the transfer is refused for insufficient funds
    And the refusal states 10.00 available against 50.00 requested

  @error @env-clean @contract-shape:unbounded-preservation
  Scenario: A refused transfer leaves the ledger exactly as it was
    Given a wallet account "alice" funded with 10.00
    And a wallet account "bob" exists
    When the integrator moves 50.00 from "alice" to "bob" under key "t-1"
    Then the transfer is refused for insufficient funds
    And the balance of "alice" reads 10.00
    And the balance of "bob" reads 0.00
    And the ledger holds 2 entries whose amounts sum to zero

  @pending @env-clean @contract-shape:bounded-change
  Scenario: Spending a wallet down to exactly zero is allowed
    Given a wallet account "alice" funded with 10.00
    And a wallet account "bob" exists
    When the integrator moves 10.00 from "alice" to "bob" under key "t-1"
    Then the transfer is accepted
    And the balance of "alice" reads 0.00

  @error @env-clean @contract-shape:unbounded-preservation
  Scenario: Spending one minor unit past a wallet's balance is refused
    Given a wallet account "alice" funded with 10.00
    And a wallet account "bob" exists
    When the integrator moves 10.01 from "alice" to "bob" under key "t-1"
    Then the transfer is refused for insufficient funds
    And the refusal states 10.00 available against 10.01 requested
    And the balance of "alice" reads 10.00

  @pending @env-clean @contract-shape:bounded-change
  Scenario: A system account is permitted to go negative
    Given a wallet account "alice" exists
    When the integrator moves 100.00 from "treasury" to "alice" under key "fund-1"
    Then the transfer is accepted
    And the balance of "treasury" reads -100.00
    And "treasury" is a system account

  @pending @error @env-clean @contract-shape:unbounded-preservation
  Scenario: A system account's counterparty is still held to its own balance
    Given a wallet account "alice" funded with 10.00
    When the integrator moves 50.00 from "alice" to "treasury" under key "t-1"
    Then the transfer is refused for insufficient funds
    And the balance of "treasury" reads -10.00

  @pending @env-contended @kpi-2 @contract-shape:bounded-change
  Scenario: Twenty spenders race one balance and only one of them wins
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When 20 integrators each move 100.00 from "alice" to "bob" at the same moment
    Then exactly 1 transfer is accepted
    And exactly 19 transfers are refused for insufficient funds
    And the balance of "alice" reads 0.00
    And the balance of "bob" reads 100.00

  @pending @env-contended @kpi-2 @contract-shape:bounded-change
  Scenario: A thousand contended spends never drive a wallet below zero
    Given a wallet account "alice" funded with 500.00
    And a wallet account "bob" exists
    When 1000 contended spends of 1.00 are attempted from "alice" to "bob"
    Then no wallet balance was ever observed below zero
    And at least 1000 attempts were made
    And the balance of "alice" reads 0.00
    And every account's balance equals the sum of its own entries
