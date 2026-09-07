# Slice 05 — Trace a transfer, and prove a third tenant cannot forge into it.
# US-5, job J10. docs/feature/inter-tenant-transfer/slices/slice-05-trace-and-isolate-a-transfer.md
#
# Every scenario in this file is a read or a refusal -- no scenario mutates
# ledger state -- so every scenario carries @contract-shape:unbounded-preservation
# (the GetTransfer driving port is a pure-function/return-only contract per
# brief.md's own per-method classification table; per Core Principle 14,
# pure-function scenarios are not authored at the acceptance layer -- the
# unbounded-preservation framing below is how this layer expresses "nothing
# changes" for a read/refusal contract instead).

@slice-05 @us-5 @driving_port @env-clean
Feature: Either party to a transfer can trace it, and no other tenant can observe or forge into it

  Before: GET /transfers/{transfer_id} (slice 02) has no hardened
  authorization boundary of its own -- a transfer spans two tenants, and
  neither the existing per-account I8 check nor a naive "any authenticated
  tenant_key" check is provably correct for it.
  After: either party's own credential, or the platform-admin credential,
  reads a transfer's full detail; every other tenant gets the identical
  refusal a nonexistent transfer_id would produce (US-5's own "no
  distinguishable signal" requirement).

  @real-io @adapter-integration @contract-shape:unbounded-preservation
  Scenario: Either party to a transfer can trace it
    Given a settled transfer "xfr_1" between "tnt_acme" and "tnt_beacon"
    When tenant "tnt_acme" queries "xfr_1"
    Then the response includes all three legs
    When tenant "tnt_beacon" queries "xfr_1"
    Then the response includes the identical all three legs

  @real-io @contract-shape:unbounded-preservation
  Scenario: The platform operator can trace any transfer
    Given a settled transfer "xfr_1" between "tnt_acme" and "tnt_beacon"
    When the platform-admin credential queries "xfr_1"
    Then the response includes all three legs

  @real-io @contract-shape:unbounded-preservation
  Scenario: A third tenant with no link to either party cannot trace the transfer
    Given a settled transfer "xfr_1" between "tnt_acme" and "tnt_beacon"
    And tenant "tnt_carter" has no link with "tnt_acme" or "tnt_beacon"
    When tenant "tnt_carter" queries "xfr_1"
    Then the request is refused as transfer not found

  @real-io @contract-shape:unbounded-preservation
  Scenario: A tenant linked to one party but not the other cannot trace the transfer
    Given a settled transfer "xfr_1" between "tnt_acme" and "tnt_beacon"
    And tenant "tnt_carter" has an active link with "tnt_acme" but none with "tnt_beacon"
    When tenant "tnt_carter" queries "xfr_1"
    Then the request is refused as transfer not found

  @real-io @contract-shape:unbounded-preservation
  Scenario: A forged counterparty alias resolves to nothing outside its own tenant's namespace
    Given tenant "tnt_beacon" has registered the alias "acme-payout" in its own namespace
    And tenant "tnt_carter" has never registered any alias named "acme-payout"
    And the operator has authorized the pair "tnt_carter" and "tnt_beacon"
    When tenant "tnt_carter" sends a transfer to "acme-payout"
    Then the request is refused as counterparty not found

  @real-io @contract-shape:unbounded-preservation
  Scenario Outline: No response distinguishes forbidden from nonexistent, across every caller and transfer id
    Given a settled transfer "xfr_1" between "tnt_acme" and "tnt_beacon"
    And tenant "tnt_carter" has no link with "tnt_acme" or "tnt_beacon"
    When "<caller>" queries "<transfer_id>"
    Then the response is byte-identical in shape to querying a nonexistent transfer_id with the same caller

    Examples: refusal-producing combinations only (the two acceptance rows -- authorized sender, authorized receiver, and the operator -- succeed and are covered by the three scenarios above, not by this table)
      | caller                | transfer_id           |
      | third-party tenant    | xfr_1                 |
      | third-party tenant    | a nonexistent id      |
