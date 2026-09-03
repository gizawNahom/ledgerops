# Slice 01 — Provision a tenant. US-1, job J7, invariant I10.
# docs/feature/multitenancy/slices/slice-01-provision-a-tenant.md

@slice-01 @us-1 @driving_port @env-clean
Feature: The operator provisions a tenant and receives a scoped credential

  Before: there is exactly one shared API key for the whole deployment.
  After: the operator provisions a named tenant and receives a credential
  that only ever sees that tenant's own accounts. Decision enabled: whether
  to hand the credential to the new customer and consider them onboarded.

  @real-io @adapter-integration @contract-shape:bounded-change
  Scenario: Provisioning a tenant issues a scoped credential
    Given the operator holds the platform-admin credential
    And a fresh store with no tenants provisioned
    When the operator provisions a tenant named "Acme Wallet"
    Then the response names tenant "Acme Wallet" with a distinct tenant id and tenant credential
    And the tenant credential is distinct from the platform-admin credential

  @real-io @contract-shape:bounded-change
  Scenario: Two tenants can be provisioned independently
    Given the operator holds the platform-admin credential
    And tenant "Acme Wallet" has been provisioned
    When the operator provisions a tenant named "Beacon Marketplace"
    Then the response names tenant "Beacon Marketplace" with a distinct tenant id and tenant credential
    And tenant "Acme Wallet" remains provisioned with its original credential

  @real-io @contract-shape:unbounded-preservation
  Scenario: A duplicate tenant name is refused
    Given the operator holds the platform-admin credential
    And tenant "Acme Wallet" has been provisioned
    When the operator provisions another tenant named "Acme Wallet"
    Then the response is refused as a tenant that already exists
    And no second tenant is created

  @real-io @contract-shape:unbounded-preservation
  Scenario: A tenant's own credential cannot provision another tenant
    Given tenant "Acme Wallet" has been provisioned
    And the caller holds tenant "Acme Wallet"'s own credential, not the platform-admin credential
    When that caller attempts to provision a tenant named "Beacon Marketplace"
    Then the response is refused as unidentified
    And no tenant is created

  @real-io @contract-shape:unbounded-preservation
  Scenario: An unissued credential cannot provision a tenant
    Given the caller presents an unissued credential
    When that caller attempts to provision a tenant named "Ghost Co"
    Then the response is refused as unidentified
    And no tenant is created
