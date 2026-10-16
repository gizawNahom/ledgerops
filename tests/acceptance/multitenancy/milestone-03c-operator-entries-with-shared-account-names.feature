# Slice 03 regression -- fix-unscoped-entries-tenant-merge.
# docs/feature/fix-unscoped-entries-tenant-merge/rca.md (decisions in section 7)
#
# Two tenants may each open an account under the same name (I9: an account is
# identified by its tenant AND its name). The operator's unscoped read of an
# account's entries used to return both tenants' rows interleaved, with
# nothing to tell them apart, and ran ONE balance across them -- a history no
# tenant holds. After the fix:
#   - every entry names the tenant it belongs to (all callers);
#   - the operator's unscoped read lists each tenant's entries together, and
#     the running balance restarts for each tenant;
#   - the operator may narrow the read to one tenant with ?tenant_id=;
#   - a tenant's own credential never widens or switches tenant: naming a
#     different tenant is refused exactly as reading another tenant's account
#     already is (account_not_found, ADR-009/ADR-011), and no row is returned.
#
# Fixture used throughout (both tenants hold "treasury"; postings interleave):
#   Acme Wallet:        funds wallet-1 with 100.00, later 25.00  -> treasury trail -100.00, -125.00
#   Beacon Marketplace: funds wallet-1 with  40.00               -> treasury trail  -40.00
# A merged read would run -100.00, -140.00, -165.00: a figure no one holds.
#
# ADR-025: scenarios were authored @pending and un-pended one at a time during
# delivery; none carries @pending now. Contract shape follows milestone-02's entries scenarios
# and milestone-03b: every When here is a read, so reads are pure-function and
# refusals are unbounded-preservation (nothing may change, nothing may leak).

@slice-02 @us-2 @driving_port @env-two-tenant @regression
Feature: The operator's entries read keeps each tenant's namesake account on its own

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:pure-function
  Scenario: The unscoped read names the tenant behind every entry and lists each tenant's entries together
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" holds a funded wallet "wallet-1" with 40.00 drawn from its system account "treasury"
    And tenant "Acme Wallet" funds "wallet-1" with 25.00 from "treasury"
    When the operator requests "treasury"'s entries with the platform-admin credential, unscoped
    Then tenant "Acme Wallet" is named on 2 entries
    And tenant "Beacon Marketplace" is named on 1 entry
    And the entries of each tenant are listed together

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:pure-function
  Scenario: The running balance restarts for each tenant in the unscoped read
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" holds a funded wallet "wallet-1" with 40.00 drawn from its system account "treasury"
    And tenant "Acme Wallet" funds "wallet-1" with 25.00 from "treasury"
    When the operator requests "treasury"'s entries with the platform-admin credential, unscoped
    Then tenant "Acme Wallet"'s entries run to balances -100.00 then -125.00
    And tenant "Beacon Marketplace"'s entries run to balance -40.00

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:pure-function
  Scenario Outline: The operator narrows the read of a shared account name to one tenant
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" holds a funded wallet "wallet-1" with 40.00 drawn from its system account "treasury"
    And tenant "Acme Wallet" funds "wallet-1" with 25.00 from "treasury"
    When the operator requests "treasury"'s entries for tenant "<tenant>" with the platform-admin credential
    Then only tenant "<tenant>"'s entries are returned
    And tenant "<tenant>"'s entries run to balances <balances>

    Examples:
      | tenant             | balances              |
      | Acme Wallet        | -100.00 then -125.00  |
      | Beacon Marketplace | -40.00                |

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:unbounded-preservation
  Scenario: Narrowing the read to a tenant that holds no such account is refused as an unknown account
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Corner Shop" has been provisioned
    When the operator requests "treasury"'s entries for tenant "Corner Shop" with the platform-admin credential
    Then the response is refused as an unknown account "treasury"
    And no entry is returned

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:unbounded-preservation
  Scenario: Narrowing the read to a tenant that was never provisioned is refused as an unknown account
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    When the operator requests "treasury"'s entries for tenant "Phantom Traders" with the platform-admin credential
    Then the response is refused as an unknown account "treasury"
    And no entry is returned

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:pure-function
  Scenario: A tenant reading its own entries sees only its own, each naming that tenant
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" holds a funded wallet "wallet-1" with 40.00 drawn from its system account "treasury"
    And tenant "Acme Wallet" funds "wallet-1" with 25.00 from "treasury"
    When tenant "Acme Wallet" requests its own entries for "treasury"
    Then only tenant "Acme Wallet"'s entries are returned
    And tenant "Acme Wallet"'s entries run to balances -100.00 then -125.00

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:pure-function
  Scenario: A tenant naming itself explicitly reads the same entries as when it names no tenant
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" holds a funded wallet "wallet-1" with 40.00 drawn from its system account "treasury"
    When tenant "Acme Wallet" requests "treasury"'s entries naming tenant "Acme Wallet"
    Then only tenant "Acme Wallet"'s entries are returned
    And tenant "Acme Wallet"'s entries run to balance -100.00

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:unbounded-preservation
  Scenario Outline: A tenant cannot read another tenant's entries by naming that tenant
    Given tenant "Acme Wallet" has been provisioned
    And tenant "Acme Wallet" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    And tenant "Beacon Marketplace" has been provisioned
    And tenant "Beacon Marketplace" holds a funded wallet "wallet-1" with 40.00 drawn from its system account "treasury"
    When tenant "Acme Wallet" requests "treasury"'s entries naming tenant "<named tenant>"
    Then the response is refused as an unknown account "treasury"
    And no entry is returned

    Examples:
      | named tenant       |
      | Beacon Marketplace |
      | Phantom Traders    |

  @fix-unscoped-entries-tenant-merge @real-io @adapter-integration @contract-shape:pure-function
  Scenario: A single-tenant ledger reads as before, apart from each entry naming its tenant
    Given the pre-multitenancy ledger is in use as tenant "Legacy Books"
    And tenant "Legacy Books" holds a funded wallet "wallet-1" with 100.00 drawn from its system account "treasury"
    When the operator requests "wallet-1"'s entries with the platform-admin credential, unscoped
    Then only tenant "Legacy Books"'s entries are returned
    And tenant "Legacy Books"'s entries run to balance 100.00
    And each entry carries today's fields plus the tenant it belongs to
