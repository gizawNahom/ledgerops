@US-8 @slice-08 @structural
Feature: Follow a quickstart that uses the published image

  The quickstart is the tested artifact; these scenarios read what it must say.

  @pending @contract-shape:pure-function
  Scenario: The README leads with the published image
    Then the README quickstart leads with the published image
    And the README no longer lists "no published container image" as a limitation

  @pending @error @contract-shape:pure-function
  Scenario: Secrets are treated as secrets
    Then the quickstart generates the password and keys, steers to the password file and warns against demo credentials
    And the quickstart warns that statement logging exposes the password verifier to offline attack

  @pending @contract-shape:pure-function
  Scenario: The upgrade and rotation order is documented
    Then the quickstart tells the operator to migrate before restarting the app

  @pending @contract-shape:pure-function
  Scenario: The verification commands are documented with the real identity
    Then the quickstart shows how to verify the signature and the per-platform software bill of materials with the project identity

  @pending @error @contract-shape:pure-function
  Scenario: The docs cover the traps a careful operator would hit
    Then the quickstart explains recovery from an interrupted migration
    And the quickstart states the one-database-per-cluster rule and its remedy

  @pending @contract-shape:pure-function
  Scenario: The docs recommend pinning by digest
    Then the quickstart recommends pinning by digest and explains the registry cannot enforce tag immutability

  @pending @requires_external @deferred @contract-shape:bounded-change
  Scenario: The documented commands work as written
    Given the real release pipeline has published "0.2.0-rc.1"
    When the quickstart commands are run in order against that release
    Then the quickstart's first transfer is answered with a transaction id
