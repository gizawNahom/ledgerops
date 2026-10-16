# Slice 03 regression — fix-trial-balance-tenant-collision.
# docs/feature/fix-trial-balance-tenant-collision/rca.md
#
# Two tenants may each open an account under the same name (I9: an account is
# identified by its tenant AND its name). The operator's platform-wide books
# check is the aggregate across every tenant (multitenancy DESIGN, Domain
# Model), so it must weigh each tenant's namesake on its own: it may neither
# merge them into a figure no one holds, nor look at only one tenant's
# accounts. Before this fix two correctly funded tenants that both had a
# "treasury" read "Books balance: NO" (stored -100.00, computed -200.00), and
# drift in any tenant other than the pre-multitenancy ledger went unseen.
#
# Every scenario carries @pending (ADR-025): the crafter removes the tag one
# scenario at a time. Contract shape follows milestone-03's own read-only
# checks (the When is a read; the out-of-band alteration is a Given).
#
# Out of scope by decision: the unscoped entries read for a shared account name.

@slice-03 @us-3 @driving_port @env-two-tenant @regression @fix-trial-balance-tenant-collision
Feature: The operator's platform-wide books check weighs each tenant's accounts on their own

  @real-io @adapter-integration @contract-shape:pure-function
  Scenario: Books balance when the pre-multitenancy ledger and a tenant each hold a funded "treasury"
    Given the pre-multitenancy ledger is in use as tenant "Legacy Books"
    And tenant "Legacy Books" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    When the operator checks the trial balance unscoped
    Then the verdict states "Books balance: YES"
    And no account is listed as drifted

  @real-io @adapter-integration @contract-shape:pure-function
  Scenario: Drift in a tenant's "treasury" is attributed to that tenant, not its namesake in the pre-multitenancy ledger
    Given the pre-multitenancy ledger is in use as tenant "Legacy Books"
    And tenant "Legacy Books" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And the stored balance of tenant "Acme Wallet"'s "treasury" is altered by 5.00 out of band
    When the operator checks the trial balance unscoped
    Then the verdict states "Books balance: NO"
    And the drift listing names only "treasury" of tenant "Acme Wallet"

  @real-io @adapter-integration @contract-shape:pure-function
  Scenario: Drift in one of two ordinary tenants is found and attributed to that tenant alone
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And the stored balance of tenant "Beacon Marketplace"'s "treasury" is altered by 5.00 out of band
    When the operator checks the trial balance unscoped
    Then the verdict states "Books balance: NO"
    And the drift listing names only "treasury" of tenant "Beacon Marketplace"
