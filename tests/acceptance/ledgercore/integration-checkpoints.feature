@real-io @adapter-integration @slice-01 @env-ci
Feature: The store keeps the promises the application layer cannot

  Mandate 6 coverage: every driven adapter is exercised against a real store,
  never a double. These scenarios assert the guarantees that live below the
  application layer and would be invisible to any test that faked it —
  append-only enforcement, role privilege, lock ordering, and migration safety
  over history that may not be rewritten.

  D7 says append-only is enforced *at the store level*. A trigger alone is not
  that: whatever role can disable the trigger can rewrite history. So the
  refusal is asserted twice over — once for the revoked privilege and once for
  the trigger — as the service's own credentials, which is the only role whose
  refusal actually protects anything (OPS-10).

  Background:
    Given the store is a real PostgreSQL 16 instance with the schema migrated from zero
    And the service holds only the application credentials
    And a system account "treasury" exists

  @append-only @contract-shape:unbounded-preservation
  Scenario: The service cannot rewrite a recorded entry
    Given a ledger carrying 3 settled transfers
    When the service's own credentials attempt to alter a recorded entry
    Then the alteration is refused by the store
    And the ledger holds 6 entries whose amounts sum to zero

  @append-only @contract-shape:unbounded-preservation
  Scenario: The service cannot erase a recorded entry
    Given a ledger carrying 3 settled transfers
    When the service's own credentials attempt to erase a recorded entry
    Then the erasure is refused by the store
    And the ledger holds 6 entries whose amounts sum to zero

  @append-only @contract-shape:unbounded-preservation
  Scenario: The service cannot switch off the protection that stops it
    Given a ledger carrying 3 settled transfers
    When the service's own credentials attempt to disable the append-only protection
    Then the attempt is refused by the store

  @append-only @contract-shape:unbounded-preservation
  Scenario: The protection also holds for a privileged operator who leaves it on
    Given a ledger carrying 3 settled transfers
    When the privileged credentials attempt to alter a recorded entry with the protection left on
    Then the alteration is refused by the store

  @env-clean @contract-shape:bounded-change
  Scenario: The schema builds from nothing
    Given an empty store with no schema at all
    When the schema is migrated from zero
    Then the ledger is ready to accept a transfer
    And the ledger holds no entries

  @env-populated @contract-shape:bounded-change
  Scenario: A later migration runs over history it may not rewrite
    Given a ledger carrying 3 settled transfers
    When the newest schema change is applied over that history
    Then the ledger still holds 6 entries whose amounts sum to zero
    And every account's balance equals the sum of its own entries
    And no migration in the set erases an entry

  @env-contended @contract-shape:bounded-change
  Scenario: Two movements touching the same pair of accounts in opposite directions do not deadlock
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" funded with 100.00
    When 50 movements from "alice" to "bob" and 50 from "bob" to "alice" are attempted at the same moment
    Then every attempt is answered
    And no attempt is answered with a deadlock
    And every account's balance equals the sum of its own entries

  @env-clean @contract-shape:bounded-change
  Scenario: Recorded timestamps come from the injected clock, not from the store
    Given the clock is fixed at "2026-08-18T09:00:00Z"
    And a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 50.00 from "alice" to "bob" under key "t-1"
    Then every entry of that transaction is stamped "2026-08-18T09:00:00Z"

  @env-clean @contract-shape:bounded-change
  Scenario: Transaction identifiers come from the injected generator, not from the store
    Given the next generated identifier is "txn_0001"
    And a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 50.00 from "alice" to "bob" under key "t-1"
    Then the answer names the transaction "txn_0001"
