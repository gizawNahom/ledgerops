@slice-05 @us-5 @driving_port @real-io
Feature: Trace a balance to its entries

  US-5 — as the platform operator, I explain any balance from the entries that
  produced it.

  Slice 04 says *something is wrong*; this slice says *here is exactly what*.
  The running balance column is what makes the break point visible (journey
  verify-the-books, S4), so a listing without it would satisfy the letter of
  "show me the entries" and none of the value.

  Background:
    Given the ledger is running against an empty store
    And a system account "treasury" exists

  @env-clean @contract-shape:unbounded-preservation
  Scenario: An account nothing has happened to traces to an empty history
    Given a wallet account "alice" exists
    When the operator traces "alice"
    Then no entries are returned

  @env-clean @contract-shape:unbounded-preservation
  Scenario: An account with a single movement traces to a single row
    Given a wallet account "alice" funded with 100.00
    When the operator traces "alice"
    Then exactly 1 entry is returned
    And each row carries a running balance
    And the running balance on the last row reads the stored balance of "alice"

  @env-populated @contract-shape:unbounded-preservation
  Scenario: An account's history reads in a settled order
    Given a wallet account "alice" with 5 movements recorded against it
    When the operator traces "alice"
    Then the entries are returned oldest first
    And tracing "alice" a second time returns them in the very same order

  @env-populated @contract-shape:unbounded-preservation
  Scenario: The running balance lands on the stored balance
    Given a wallet account "alice" with 5 movements recorded against it
    When the operator traces "alice"
    Then each row carries a running balance
    And the running balance on the last row reads the stored balance of "alice"

  @env-populated @contract-shape:unbounded-preservation
  Scenario: Each movement is legible without cross-referencing anything
    Given a wallet account "alice" with 5 movements recorded against it
    When the operator traces "alice"
    Then every row names its transaction, its counterparty account, its amount, and when it happened
    And no row names "alice" as its own counterparty

  @env-populated @contract-shape:unbounded-preservation
  Scenario: Two movements in the same instant still read in a settled order
    Given two movements against "alice" recorded at the very same instant
    When the operator traces "alice"
    Then the entries are returned oldest first
    And tracing "alice" a second time returns them in the very same order

  @pending @error @env-corrupted @contract-shape:unbounded-preservation
  Scenario: On a drifted account the running balance parts company at the guilty row
    Given a wallet account "alice" with 5 movements recorded against it
    And the recorded amount of the third entry belonging to "alice" is altered by 5.00 out of band
    When the operator traces "alice"
    Then the running balance on the last row disagrees with the stored balance of "alice" by 5.00
    And the row where the running balance first parts company is the altered one

  @pending @env-corrupted @contract-shape:unbounded-preservation
  Scenario: The drift listing hands the operator straight to the entries
    Given a wallet account "alice" with 5 movements recorded against it
    And the recorded amount of the third entry belonging to "alice" is altered by 5.00 out of band
    When the operator asks whether the books balance
    And the operator follows the drift listing for "alice"
    Then the entries of "alice" are returned
    And the running balance on the last row disagrees with the stored balance of "alice" by 5.00

  @error @env-populated @contract-shape:unbounded-preservation
  Scenario: Tracing an account nobody opened is refused
    When the operator traces "nobody"
    Then the trace is refused as an unknown account
    And the refusal names the account "nobody"

  @error @env-populated @contract-shape:unbounded-preservation
  Scenario: An unidentified caller cannot trace an account
    Given a wallet account "alice" with 5 movements recorded against it
    And the caller presents no operator key
    When the operator traces "alice"
    Then the caller is refused as unidentified
