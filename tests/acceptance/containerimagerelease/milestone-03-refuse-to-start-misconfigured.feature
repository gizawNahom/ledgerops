@real-io @US-2 @slice-02 @env-serve-config
Feature: Know immediately when ledgerops is misconfigured

  Startup validation reports every problem at once, before any database
  connection, and never prints a key or a password.

  @pending @error @driving_port @kpi @KPI-R9 @contract-shape:unbounded-preservation
  Scenario Outline: A missing or blank required setting is refused at start
    Given no database is reachable
    And <setting> is <state>
    When ledgerops starts
    Then ledgerops refuses to start naming "<variable>"
    And no connection to the database was attempted
    And no output contains "dsn-sentinel-5c1e"

    Examples:
      | setting           | state           | variable                |
      | the operator key  | unset           | LEDGEROPS_OPERATOR_KEY  |
      | the operator key  | empty           | LEDGEROPS_OPERATOR_KEY  |
      | the operator key  | whitespace-only | LEDGEROPS_OPERATOR_KEY  |
      | the app connection setting       | unset           | LEDGEROPS_APP_DSN       |
      | the app connection setting       | empty           | LEDGEROPS_APP_DSN       |
      | the app connection setting       | whitespace-only | LEDGEROPS_APP_DSN       |

  @pending @error @driving_port @kpi @KPI-R9 @contract-shape:unbounded-preservation
  Scenario: All missing settings are reported together
    Given no database is reachable
    And the operator key is unset
    And the app connection setting is unset
    When ledgerops starts
    Then ledgerops refuses to start naming "LEDGEROPS_OPERATOR_KEY" and "LEDGEROPS_APP_DSN" and "config invalid"

  @pending @error @driving_port @kpi @KPI-R9 @contract-shape:unbounded-preservation
  Scenario: The serving process refuses to hold the migration credential
    Given no database is reachable
    And the migration credential is present in the environment
    When ledgerops starts
    Then ledgerops refuses to start naming "LEDGEROPS_MIGRATE_DSN"
    And no connection to the database was attempted
    And no output contains "migrate-sentinel-77"

  @pending @error @image @driving_port @kpi @KPI-R9 @contract-shape:unbounded-preservation
  Scenario: The published image refuses to start with the migration credential in its environment
    Given no database is reachable
    And the migration credential is present in the environment
    When ledgerops starts
    Then ledgerops refuses to start naming "LEDGEROPS_MIGRATE_DSN"
    And no connection to the database was attempted

  @pending @error @image @driving_port @contract-shape:unbounded-preservation
  Scenario: The published image started with no settings lists what is missing
    Given no database is reachable
    And the operator key is unset
    And the app connection setting is unset
    When ledgerops starts
    Then ledgerops refuses to start naming "LEDGEROPS_OPERATOR_KEY" and "LEDGEROPS_APP_DSN"

  @pending @driving_port @contract-shape:bounded-change
  Scenario: A correctly configured ledgerops starts and answers health without credentials
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And the operator key is "op-9f3b71c2"
    When ledgerops starts
    Then ledgerops reports healthy
    And the health answer carries no ledger data
    And the operator key is accepted for provisioning a tenant

  @pending @driving_port @contract-shape:bounded-change
  Scenario Outline: The demo tenant key is optional and its mapping stays off when unset
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And the operator key is "op-9f3b71c2"
    And the demo tenant key is <state>
    When ledgerops starts
    Then ledgerops reports healthy
    And the token "demo-tenant-key" is refused as unidentified

    Examples:
      | state  |
      | unset  |
      | empty  |

  @pending @driving_port @contract-shape:bounded-change
  Scenario: A set demo tenant key still works for the demo stack
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And the operator key is "op-9f3b71c2"
    And the demo tenant key is "demo-tenant-key"
    When ledgerops starts
    Then the token "demo-tenant-key" resolves to the pre-existing demo tenant

  @pending @error @driving_port @kpi @KPI-R9 @contract-shape:unbounded-preservation
  Scenario Outline: Key and connection values are never printed when ledgerops refuses
    Given no database is reachable
    And the operator key is "op-9f3b71c2"
    And <fault>
    When ledgerops starts
    Then ledgerops refuses to start naming "<named>"
    And no output contains "op-9f3b71c2"
    And no output contains "dsn-sentinel-5c1e"
    And no output contains "migrate-sentinel-77"

    Examples:
      | fault                                                | named                  |
      | the app connection setting is unset                                 | LEDGEROPS_APP_DSN      |
      | the migration credential is present in the environment | LEDGEROPS_MIGRATE_DSN |

  @pending @error @driving_port @contract-shape:unbounded-preservation
  Scenario: An unknown subcommand is refused with the list of commands
    When the operator runs "frobnicate"
    Then the command refuses naming "serve" and "migrate" and "version"

  @pending @compose @contract-shape:pure-function
  Scenario: The demo stack gives the migration credential to migrate only
    Then the demo compose file runs "ledgerops migrate" with the demo app password set explicitly
    And the demo app service never receives the migration credential
    And the load-test compose file keeps the app without the migration credential
