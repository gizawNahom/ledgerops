# Walking skeleton — chains slice 01 into slice 02. feature-delta.md § Wave:
# DISCUSS / Story Map: "link authorized -> alias registered -> transfer sent
# -> all three legs settle -> GET /transfers/{id} reports settled."
# docs/feature/inter-tenant-transfer/slices/slice-02-send-a-transfer-to-a-named-counterparty.md

@walking_skeleton @driving_port @driving_adapter @real-io @slice-01 @slice-02 @us-1 @us-2 @us-8 @env-clean
Feature: A tenant sends value to another tenant it has been authorized to trade with, by name

  Before: a tenant's own credential can only ever touch its own accounts
  (I8) -- there is no way to move value to another tenant's wallet at all,
  authorized pair or not.
  After: the operator authorizes a pair, one tenant registers an alias for
  the other, sends a transfer by that alias, and polls until every leg has
  settled -- never handling the counterparty's raw tenant id.

  Stakeholder litmus test: "two businesses that already trust each other can
  move money to each other by name, and the platform proves it settled."
  That is what this feature exists to prove.

  Sync/async contract this scenario deliberately respects (brief.md § Sync
  vs. async settlement): the POST response reports "pending" with only leg1
  posted -- it is the subsequent GET that observes "settled". Asserting
  "settled" directly on the POST response would be asserting a contract this
  design does not make.

  @contract-shape:bounded-change
  Scenario: A transfer to a named counterparty settles across three ledgers
    Given the operator holds the platform-admin credential
    And a fresh store with no tenants provisioned
    And tenant "tnt_acme" has been provisioned
    And tenant "tnt_beacon" has been provisioned
    And the operator authorizes the pair "tnt_acme" and "tnt_beacon"
    And tenant "tnt_acme" opens a system account named "acme-treasury"
    And tenant "tnt_acme" opens a wallet account named "acme-wallet"
    And tenant "tnt_acme" funds "acme-wallet" with 500.00 from "acme-treasury"
    And tenant "tnt_beacon" opens a wallet account named "wallet-ops"
    And tenant "tnt_acme" registers the alias "beacon-payout" for tenant "tnt_beacon"'s account "wallet-ops"
    When tenant "tnt_acme" sends a transfer of 50.00 to "beacon-payout" with a fresh idempotency key
    Then the response reports status "pending" with leg1 posted
    And tenant "tnt_acme"'s wallet account "acme-wallet" balance reads 450.00
    When the transfer is polled until it reaches a terminal state
    Then the transfer's status is "settled"
    And all three legs report "posted"
    And tenant "tnt_beacon"'s wallet account "wallet-ops" balance reads 50.00
