# Slice 02 — Send a transfer to a named counterparty. US-2, job J8.
# Walking skeleton lives in its own file (walking-skeleton.feature) and is
# NOT duplicated here — these are the additional scenarios slice 02 needs
# beyond the one end-to-end path the skeleton already proves.
# docs/feature/inter-tenant-transfer/slices/slice-02-send-a-transfer-to-a-named-counterparty.md

@slice-02 @us-2 @driving_port @env-clean
Feature: A tenant addresses a transfer to a trusted counterparty by alias, never by raw tenant id

  Before: a tenant's own credential cannot name another tenant's account at
  all -- there is no wire vocabulary for "the account I mean belongs to a
  different tenant."
  After: a tenant registers a name for a counterparty it already has a
  standing link with, and every subsequent transfer to that counterparty
  uses only that name (D11).

  @real-io @adapter-integration @contract-shape:bounded-change
  Scenario: Registering an alias against an active link succeeds
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    And tenant "tnt_beacon" opens a wallet account named "wallet-ops"
    When tenant "tnt_acme" registers the alias "beacon-payout" for tenant "tnt_beacon"'s account "wallet-ops"
    Then the alias "beacon-payout" is registered in tenant "tnt_acme"'s own namespace

  @real-io @contract-shape:unbounded-preservation
  Scenario: Registering an alias against a revoked link is refused
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    And the operator has revoked the link between "tnt_acme" and "tnt_beacon"
    And tenant "tnt_beacon" opens a wallet account named "wallet-ops"
    When tenant "tnt_acme" registers the alias "beacon-payout" for tenant "tnt_beacon"'s account "wallet-ops"
    Then the request is refused as a tenant link that was not found
    And no alias "beacon-payout" is registered in tenant "tnt_acme"'s namespace

  @real-io @contract-shape:unbounded-preservation
  Scenario: Registering an alias with no standing link at all is refused
    Given tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And tenant "tnt_beacon" opens a wallet account named "wallet-ops"
    When tenant "tnt_acme" registers the alias "beacon-payout" for tenant "tnt_beacon"'s account "wallet-ops"
    Then the request is refused as a tenant link that was not found

  @real-io @contract-shape:unbounded-preservation
  Scenario: Sending to an unregistered alias is refused before any leg posts
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_acme" opens a system account named "acme-treasury"
    And tenant "tnt_acme" opens a wallet account named "acme-wallet"
    And tenant "tnt_acme" funds "acme-wallet" with 500.00 from "acme-treasury"
    And tenant "tnt_acme" has registered no alias named "unknown-partner"
    When tenant "tnt_acme" sends a transfer of 50.00 to "unknown-partner" with a fresh idempotency key
    Then the request is refused as counterparty not found
    And no leg posts
    And tenant "tnt_acme"'s wallet account "acme-wallet" balance reads 500.00

  @real-io @contract-shape:unbounded-preservation
  Scenario: Insufficient sender funds refuses before any leg posts
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    And tenant "tnt_acme" opens a system account named "acme-treasury"
    And tenant "tnt_acme" opens a wallet account named "acme-wallet"
    And tenant "tnt_acme" funds "acme-wallet" with 10.00 from "acme-treasury"
    And tenant "tnt_beacon" opens a wallet account named "wallet-ops"
    And tenant "tnt_acme" has registered the alias "beacon-payout" for tenant "tnt_beacon"'s account "wallet-ops"
    When tenant "tnt_acme" sends a transfer of 50.00 to "beacon-payout" with a fresh idempotency key
    Then the request is refused as insufficient funds
    And tenant "tnt_acme"'s wallet account "acme-wallet" balance reads 10.00
    And no leg posts

  @real-io @contract-shape:unbounded-preservation
  Scenario: Retrying the same idempotency key returns the original transfer, no duplicate legs
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    And tenant "tnt_acme" opens a system account named "acme-treasury"
    And tenant "tnt_acme" opens a wallet account named "acme-wallet"
    And tenant "tnt_acme" funds "acme-wallet" with 500.00 from "acme-treasury"
    And tenant "tnt_beacon" opens a wallet account named "wallet-ops"
    And tenant "tnt_acme" has registered the alias "beacon-payout" for tenant "tnt_beacon"'s account "wallet-ops"
    And tenant "tnt_acme" has already sent a transfer of 50.00 to "beacon-payout" with idempotency key "k1"
    When tenant "tnt_acme" resends the identical request with idempotency key "k1"
    Then the response reports the same transfer id as before
    And exactly one set of legs exists for that transfer id

  @real-io @contract-shape:bounded-change
  Scenario: Two transfers to the same alias with two different idempotency keys are independent
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    And tenant "tnt_acme" opens a system account named "acme-treasury"
    And tenant "tnt_acme" opens a wallet account named "acme-wallet"
    And tenant "tnt_acme" funds "acme-wallet" with 500.00 from "acme-treasury"
    And tenant "tnt_beacon" opens a wallet account named "wallet-ops"
    And tenant "tnt_acme" has registered the alias "beacon-payout" for tenant "tnt_beacon"'s account "wallet-ops"
    When tenant "tnt_acme" sends a transfer of 20.00 to "beacon-payout" with idempotency key "k1"
    And tenant "tnt_acme" sends a transfer of 20.00 to "beacon-payout" with idempotency key "k2"
    Then the two transfer ids reported are distinct
    And tenant "tnt_acme"'s wallet account "acme-wallet" balance reads 460.00

  @real-io @contract-shape:unbounded-preservation
  Scenario: The cross-tenant response never reports settled before polling
    Given the operator holds the platform-admin credential
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator has authorized the pair "tnt_acme" and "tnt_beacon"
    And tenant "tnt_acme" opens a system account named "acme-treasury"
    And tenant "tnt_acme" opens a wallet account named "acme-wallet"
    And tenant "tnt_acme" funds "acme-wallet" with 500.00 from "acme-treasury"
    And tenant "tnt_beacon" opens a wallet account named "wallet-ops"
    And tenant "tnt_acme" has registered the alias "beacon-payout" for tenant "tnt_beacon"'s account "wallet-ops"
    When tenant "tnt_acme" sends a transfer of 50.00 to "beacon-payout" with a fresh idempotency key
    Then the response reports status "pending", never "settled"
    And only leg1 is reported on the response
