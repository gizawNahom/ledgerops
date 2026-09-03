@walking_skeleton @driving_port @driving_adapter @real-io @slice-01 @slice-02 @us-1 @us-2 @env-two-tenant
Feature: A second tenant shares the deployment with no path into the first tenant's ledger

  The one scenario that closes this feature's own loop end to end (not
  ledger-core's — feature-delta.md § Wave: DISCUSS / Wave decisions summary:
  "slice 01 alone is not sufficient to prove the feature's reason for
  existing"). It chains slice 01 (provision) into slice 02 (operate,
  invisible to every other tenant) through the real chi router, the real
  requireOperatorKey middleware, and a real PostgreSQL 16 — the same
  production composition root ledger-core's own walking skeleton uses.

  Stakeholder litmus test: "the operator onboards a second customer, that
  customer opens and funds an account, and the first customer's credential
  cannot see it." That is what this feature exists to prove.

  @contract-shape:bounded-change
  Scenario: A newly onboarded tenant operates invisibly to an existing tenant
    Given the operator holds the platform-admin credential
    And a fresh store with no tenants provisioned
    When the operator provisions a tenant named "Acme Wallet"
    And the operator provisions a tenant named "Beacon Marketplace"
    And tenant "Acme Wallet" opens a system account named "acme-treasury"
    And tenant "Acme Wallet" opens a wallet account named "alice"
    And tenant "Acme Wallet" funds "alice" with 100.00 from "acme-treasury"
    Then tenant "Acme Wallet"'s account "alice" balance reads 100.00
    When tenant "Beacon Marketplace" requests tenant "Acme Wallet"'s account "alice"
    Then the response is refused as an unknown account
