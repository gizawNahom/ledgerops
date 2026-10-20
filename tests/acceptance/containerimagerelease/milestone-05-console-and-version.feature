@US-5 @slice-05 @real-io
Feature: Open the console and see the version from the image

  @pending @image @dedicated-cluster @driving_port @kpi @KPI-R6 @contract-shape:bounded-change
  Scenario: The console loads from the image with no files mounted
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And the operator key is "op-9f3b71c2"
    When ledgerops starts
    Then the console page is returned
    And the console verdict needs the operator key

  @pending @driving_port @contract-shape:pure-function
  Scenario: The binary reports the release and commit it was built from
    Given the binary is built for release "0.2.0" at commit "3f9c2ab"
    When the operator runs "version"
    Then it prints "ledgerops 0.2.0 (commit 3f9c2ab)"

  @pending @driving_port @contract-shape:pure-function
  Scenario: A local build is recognisably unreleased and needs no settings
    Given the binary is built without version information
    When the operator runs "version"
    Then it prints "dev (commit unknown)"

  @pending @error @console-embed-build @contract-shape:unbounded-preservation
  Scenario Outline: An image build without a console bundle fails
    Given a console bundle that is <state>
    When the binary is built for the image
    Then the build fails

    Examples:
      | state  |
      | absent |
      | empty  |

  @pending @error @console-embed-build @dedicated-cluster @contract-shape:unbounded-preservation
  Scenario: A console bundle without an index page refuses to start
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And a console bundle that is without an index page
    And the binary is built for the image
    And the operator key is "op-9f3b71c2"
    And an empty working directory
    When ledgerops starts
    Then ledgerops refuses to start naming "index.html"

  @pending @console-embed-build @dedicated-cluster @contract-shape:bounded-change
  Scenario: An embedded console is served from any working directory
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And a console bundle that is present
    And the binary is built for the image
    And the operator key is "op-9f3b71c2"
    And an empty working directory
    When ledgerops starts
    Then the console page is returned

  @pending @console-embed-build @dedicated-cluster @contract-shape:bounded-change
  Scenario: A developer build without the embed tag still serves the bundle found in the working directory
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And an untagged build with a console bundle in the working directory
    And the operator key is "op-9f3b71c2"
    When ledgerops starts
    Then the console page is returned

  @pending @error @structural @contract-shape:pure-function
  Scenario: A failed console build can never yield an image
    Then the image build copies the console bundle from its own stage and is tagged to embed it
    And the image build context excludes sources and dependencies it does not need
