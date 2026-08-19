@walking_skeleton @driving_port @driving_adapter @real-io @slice-01 @us-1 @env-clean
Feature: Value moves between two accounts and stays moved

  The one scenario that closes the loop end to end: a real client speaking to
  the real service over its real front door, through the real posting rules,
  into the real store, and back out as a balance anyone can read.

  Not a demonstration that the rules are right — the milestone scenarios and the
  property suite do that. A demonstration that the parts are connected at all.
  If this is red, nothing else being green means anything.

  Stakeholder litmus test: "an integrator creates two accounts, funds one, moves
  fifty from it to the other, and both balances say so." That is what users need.

  @contract-shape:bounded-change
  Scenario: An integrator moves fifty from one account to another
    Given the ledger is running against an empty store
    And a system account "treasury" exists
    And a wallet account "alice" exists
    And a wallet account "bob" exists
    And "alice" has been funded with 100.00 from "treasury"
    When the integrator moves 50.00 from "alice" to "bob" under key "ws-01"
    Then the transfer is accepted
    And the answer names one transaction with two legs of -50.00 and 50.00
    And the balance of "alice" reads 50.00
    And the balance of "bob" reads 50.00
    And the ledger holds 4 entries whose amounts sum to zero
