@slice-01 @us-1 @driving_port @real-io @env-clean
Feature: Post a transfer

  US-1 — as an integrating developer, I move value between two accounts and
  trust it landed completely or not at all.

  Sufficient funds are deliberately NOT checked in this slice (slice 02) and
  retries are deliberately NOT deduplicated (slice 03). Balances may go negative
  here and a repeat submission may post twice; both are scoped, not defects.
  Every transfer nevertheless carries a key from this slice onward (DDR-1), so
  slice 03 tightens behaviour without rewriting a single scenario below.

  Background:
    Given the ledger is running against an empty store
    And a system account "treasury" exists

  @contract-shape:bounded-change
  Scenario: A new account starts empty
    When the integrator opens a wallet account "alice"
    Then the account is created
    And the balance of "alice" reads 0.00

  @pending @error @contract-shape:unbounded-preservation
  Scenario: Opening an account somebody already opened is refused and their account is untouched
    Given a wallet account "alice" funded with 100.00
    When the integrator opens a wallet account "alice"
    Then the account is refused as already open
    And the refusal names the account "alice"
    And the balance of "alice" reads 100.00
    And the ledger holds 2 entries whose amounts sum to zero

  @contract-shape:bounded-change
  Scenario: Value enters the ledger only as a movement from the system account
    Given a wallet account "alice" exists
    When the integrator moves 100.00 from "treasury" to "alice" under key "fund-1"
    Then the transfer is accepted
    And the balance of "alice" reads 100.00
    And the balance of "treasury" reads -100.00
    And the ledger holds 2 entries whose amounts sum to zero

  @contract-shape:bounded-change
  Scenario: A posted transfer records exactly two legs that cancel out
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 30.00 from "alice" to "bob" under key "t-1"
    Then the transfer is accepted
    And the answer names one transaction with two legs of -30.00 and 30.00
    And both legs belong to the same transaction
    And the two legs sum to zero

  @contract-shape:bounded-change
  Scenario: A balance is the sum of the entries that produced it
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 30.00 from "alice" to "bob" under key "t-1"
    And the integrator moves 20.00 from "alice" to "bob" under key "t-2"
    Then the balance of "alice" reads 50.00
    And the balance of "bob" reads 50.00
    And every account's balance equals the sum of its own entries

  @pending @error @contract-shape:unbounded-preservation
  Scenario: A transfer to an account nobody opened is refused and nothing moves
    Given a wallet account "alice" funded with 100.00
    When the integrator moves 25.00 from "alice" to "nobody" under key "t-1"
    Then the transfer is refused as an unknown account
    And the refusal names the account "nobody"
    And the balance of "alice" reads 100.00
    And the ledger holds 2 entries whose amounts sum to zero

  @pending @error @contract-shape:unbounded-preservation
  Scenario: A transfer out of an account nobody opened is refused and nothing moves
    Given a wallet account "bob" exists
    When the integrator moves 25.00 from "nobody" to "bob" under key "t-1"
    Then the transfer is refused as an unknown account
    And the refusal names the account "nobody"
    And the ledger holds no entries

  @contract-shape:bounded-change
  Scenario: The smallest amount the ledger can move is accepted
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 0.01 from "alice" to "bob" under key "t-1"
    Then the transfer is accepted
    And the balance of "alice" reads 99.99
    And the balance of "bob" reads 0.01
    And the ledger holds 4 entries whose amounts sum to zero

  @pending @error @contract-shape:unbounded-preservation
  Scenario Outline: A transfer of an amount that moves nothing is refused
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves <amount> from "alice" to "bob" under key "t-1"
    Then the transfer is refused as an invalid amount
    And the balance of "alice" reads 100.00

    Examples:
      | amount |
      | 0.00   |
      | -1.00  |

  @pending @error @contract-shape:unbounded-preservation
  Scenario Outline: An amount the ledger cannot hold exactly is refused and nothing moves
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves an amount written as "<amount>" from "alice" to "bob" under key "t-1"
    Then the transfer is refused as an invalid amount
    And the balance of "alice" reads 100.00
    And the ledger holds 2 entries whose amounts sum to zero

    Examples:
      | amount               |
      | 50.001               |
      | 92233720368547758.08 |

  @pending @error @driving_adapter @contract-shape:unbounded-preservation
  Scenario Outline: A request the ledger cannot read as a command is refused and nothing moves
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator submits a transfer request <malformation>
    Then the transfer is refused as a request that cannot be read
    And the balance of "alice" reads 100.00
    And the balance of "bob" reads 0.00
    And the ledger holds 2 entries whose amounts sum to zero

    Examples:
      | malformation                                                  |
      | that is not a request at all                                  |
      | that leaves out the amount                                    |
      | that leaves out the account it moves from                     |
      | that names a field the ledger does not know                   |
      | whose amount is not a number                                  |
      | whose amount is left empty                                    |
      | whose amount is sent as a bare number rather than written out |

  @pending @error @contract-shape:unbounded-preservation
  Scenario: An unidentified caller is refused before anything is read or written
    Given a wallet account "alice" funded with 100.00
    And the caller presents no operator key
    When the integrator moves 25.00 from "alice" to "treasury" under key "t-1"
    Then the caller is refused as unidentified
    And the balance of "alice" reads 100.00

  @pending @error @contract-shape:unbounded-preservation
  Scenario: A caller presenting the wrong operator key is refused
    Given a wallet account "alice" funded with 100.00
    And the caller presents an operator key that was never issued
    When the integrator moves 25.00 from "alice" to "treasury" under key "t-1"
    Then the caller is refused as unidentified
    And the balance of "alice" reads 100.00

  @pending @chaos @driving_adapter @env-clean @contract-shape:bounded-change
  Scenario: A transfer interrupted halfway leaves no half-applied movement
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the ledger is killed partway through moving 40.00 from "alice" to "bob"
    And the ledger is restarted against the same store
    Then the ledger holds either both legs of that movement or neither
    And the balance of "alice" reads either 100.00 or 60.00
    And every account's balance equals the sum of its own entries
    And the ledger holds no entries whose amounts fail to sum to zero
