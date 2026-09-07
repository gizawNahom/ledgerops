# Slice 01 — Authorize a tenant pair. US-1, job J9, invariant I11.
# docs/feature/inter-tenant-transfer/slices/slice-01-authorize-a-tenant-pair.md

@slice-01 @us-1 @driving_port @env-clean
Feature: The operator authorizes a standing link between two tenants

  Before: no platform-level notion of "these two tenants may trade" exists
  -- every cross-tenant movement is either impossible or would require the
  operator's involvement every single time.
  After: the operator authorizes a pair once; their integrators can transfer
  to each other without coming back to the operator for every transfer (D9
  -- standing until revoked, never per-transfer).

  @real-io @adapter-integration @contract-shape:bounded-change
  Scenario: Authorizing a tenant pair creates a standing link
    Given the operator holds the platform-admin credential
    And a fresh store with no tenants provisioned
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    When the operator authorizes the pair "tnt_acme" and "tnt_beacon"
    Then a link is created between "tnt_acme" and "tnt_beacon" with status "active"
    And the link has no expiry or usage limit

  @real-io @contract-shape:bounded-change
  Scenario: Authorizing a second pair leaves the first pair's link unaffected
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And tenant "tnt_carter" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    When the operator authorizes the pair "tnt_acme" and "tnt_carter"
    Then a link is created between "tnt_acme" and "tnt_carter" with status "active"
    And the link between "tnt_acme" and "tnt_beacon" remains "active"

  @real-io @contract-shape:unbounded-preservation
  Scenario: A duplicate authorization is refused
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    When the operator authorizes the pair "tnt_acme" and "tnt_beacon" again
    Then the request is refused as a tenant link that already exists
    And the existing link between "tnt_acme" and "tnt_beacon" is unaffected

  @real-io @contract-shape:unbounded-preservation
  Scenario: Authorizing an unknown tenant is refused
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And no tenant named "tnt_ghost" has been provisioned
    When the operator authorizes the pair "tnt_acme" and "tnt_ghost"
    Then the request is refused as an unknown tenant
    And no link is created between "tnt_acme" and "tnt_ghost"

  @real-io @contract-shape:bounded-change
  Scenario: Revoking a link stops future alias registration from passing
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    When the operator revokes the link between "tnt_acme" and "tnt_beacon"
    Then the link between "tnt_acme" and "tnt_beacon" reads "revoked"
    And tenant "tnt_acme" cannot register a counterparty alias against that link

  @real-io @contract-shape:bounded-change
  Scenario: Re-authorizing a pair after revocation mints a new link
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    And the operator has revoked the link between "tnt_acme" and "tnt_beacon"
    When the operator authorizes the pair "tnt_acme" and "tnt_beacon" again
    Then a link is created between "tnt_acme" and "tnt_beacon" with status "active"
    And the newly created link id differs from the revoked one

  @real-io @contract-shape:unbounded-preservation
  Scenario: A non-admin credential cannot authorize a pair
    Given tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the caller holds tenant "tnt_acme"'s own credential, not the platform-admin credential
    When that caller attempts to authorize the pair "tnt_acme" and "tnt_beacon"
    Then the request is refused as unidentified
    And no link is created between "tnt_acme" and "tnt_beacon"

  @real-io @contract-shape:unbounded-preservation
  Scenario: An unissued credential cannot authorize a pair
    Given tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the caller presents an unissued credential
    When that caller attempts to authorize the pair "tnt_acme" and "tnt_beacon"
    Then the request is refused as unidentified
    And no link is created between "tnt_acme" and "tnt_beacon"
