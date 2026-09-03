# Slice 03 — Verify one tenant's books without seeing another's.
# US-3, job J7.
# docs/feature/multitenancy/slices/slice-03-verify-one-tenants-books.md

@slice-03 @us-3 @driving_port @env-two-tenant
Feature: The operator checks whether one tenant's books balance, scoped to that tenant alone

  Before: GET /health/trial-balance answers for the whole deployment. After:
  the operator asks the question about one customer's slice of it, and
  another customer's drift or mid-transaction state has no effect on the
  answer. Decision enabled: whether to trust tenant 1's books right now,
  independent of any other tenant's state.

  @pending @real-io @adapter-integration @contract-shape:pure-function
  Scenario: A tenant's clean books report YES independent of another tenant's state
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a wallet account named "alice"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" opens a wallet account named "bob"
    And the stored balance of "bob" is altered by 5.00 out of band
    When the operator checks the trial balance for tenant "Acme Wallet"
    Then the verdict states "Books balance: YES"
    And no mention of tenant "Beacon Marketplace" appears in the response

  @pending @real-io @adapter-integration @contract-shape:pure-function
  Scenario: A tenant's drift is named without exposing other tenants
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a wallet account named "alice"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" opens a wallet account named "bob"
    And the stored balance of "bob" is altered by 5.00 out of band
    When the operator checks the trial balance for tenant "Beacon Marketplace"
    Then the verdict states "Books balance: NO"
    And the response names "bob" in the drift listing
    And no account belonging to tenant "Acme Wallet" appears in the response

  @pending @real-io @contract-shape:unbounded-preservation
  Scenario: Checking an unprovisioned tenant is refused
    Given no tenant named "Ghost Co" has been provisioned
    When the operator checks the trial balance for tenant "Ghost Co"
    Then the response is refused as an unknown tenant

  @pending @real-io @adapter-integration @console-compat @contract-shape:pure-function
  Scenario: The existing unscoped trial-balance call keeps succeeding once tenants exist
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a wallet account named "alice"
    When the operator checks the trial balance unscoped
    Then the response succeeds with a stated verdict, unchanged in shape from today's single-tenant contract

  @pending @real-io @adapter-integration @console-compat @contract-shape:pure-function
  Scenario: The existing unscoped console verdict call keeps succeeding once tenants exist
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a wallet account named "alice"
    When the operator checks the console verdict
    Then the response succeeds with a stated verdict, unchanged in shape from today's single-tenant contract
