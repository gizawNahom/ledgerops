# Slice 02 — Operate within a tenant, invisible to every other tenant.
# US-2, job J6, invariant I8/I9.
# docs/feature/multitenancy/slices/slice-02-operate-within-a-tenant.md

@slice-02 @us-2 @driving_port @env-two-tenant
Feature: A tenant's credential can never read or write another tenant's ledger

  Before: every account name is globally unique and reachable by the one
  shared key. After: a tenant opens and funds accounts exactly as today's
  single-tenant API already works, and no other tenant's credential can see
  or touch them. Decision enabled: trust that onboarding a second customer
  cannot corrupt or expose the first customer's ledger.

  @real-io @contract-shape:bounded-change
  Scenario: Two tenants independently reuse the same account name
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Beacon Marketplace" has been provisioned
    When tenant "Acme Wallet" opens a wallet account named "wallet-1"
    And tenant "Beacon Marketplace" opens a wallet account named "wallet-1"
    Then both accounts are opened successfully
    And tenant "Acme Wallet"'s account "wallet-1" balance reads 0.00
    And tenant "Beacon Marketplace"'s account "wallet-1" balance reads 0.00

  @real-io @adapter-integration @contract-shape:unbounded-preservation
  Scenario: A tenant's account is invisible to every other tenant
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a system account named "acme-treasury"
    And tenant "Acme Wallet" opens a wallet account named "alice"
    And tenant "Acme Wallet" funds "alice" with 100.00 from "acme-treasury"
    And tenant "Beacon Marketplace" has been provisioned
    When tenant "Beacon Marketplace" requests tenant "Acme Wallet"'s account "alice"
    Then the response is refused as an unknown account

  @real-io @contract-shape:unbounded-preservation
  Scenario: A tenant cannot post a transfer touching another tenant's account
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a wallet account named "alice"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" opens a wallet account named "bob"
    When tenant "Acme Wallet" moves 10.00 from "alice" to tenant "Beacon Marketplace"'s account "bob"
    Then the transfer is refused
    And tenant "Acme Wallet"'s account "alice" balance reads 0.00

  @real-io @contract-shape:bounded-change
  Scenario: Existing single-tenant invariants still hold within one tenant
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a wallet account named "alice"
    When tenant "Acme Wallet" moves 50.00 from "alice" to "acme-treasury"
    Then the transfer is refused
    And tenant "Acme Wallet"'s account "alice" balance reads 0.00

  @real-io @contract-shape:unbounded-preservation
  Scenario: A malformed or missing tenant credential is refused before any tenant logic runs
    Given no caller credential is presented
    When that caller attempts to open an account
    Then the response is refused as unidentified

  @real-io @adapter-integration @contract-shape:pure-function
  Scenario: A tenant's entries are scoped to that tenant alone
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a system account named "acme-treasury"
    And tenant "Acme Wallet" opens a wallet account named "alice"
    And tenant "Acme Wallet" funds "alice" with 100.00 from "acme-treasury"
    And tenant "Beacon Marketplace" has been provisioned
    When tenant "Beacon Marketplace" requests tenant "Acme Wallet"'s entries for "alice"
    Then the response is refused as an unknown account

  @real-io @adapter-integration @console-compat @contract-shape:pure-function
  Scenario: The existing unscoped entries call keeps working for the console's own credential
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" opens a wallet account named "alice"
    When the operator requests "alice"'s entries with the platform-admin credential, unscoped
    Then the response succeeds, unchanged in shape from today's single-tenant entries contract
