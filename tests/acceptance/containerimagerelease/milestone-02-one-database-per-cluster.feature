@real-io @dedicated-cluster @US-1b @slice-01b @env-role-exists-other-db
Feature: One ledgerops database per cluster

  The application role is cluster-global, so a second ledgerops database in
  the same cluster is refused before anything changes, and two simultaneous
  first migrations cannot both claim the role.

  @driving_port @contract-shape:bounded-change
  Scenario: The first ledgerops database in a cluster can have any name
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    When the operator prepares the database with migrate
    Then the app role logs in with "T7!kq@9x%Zm"

  @error @kpi @KPI-R8 @contract-shape:unbounded-preservation
  Scenario: A second ledgerops database in the same cluster is refused
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And the cluster also holds an empty database "kestrel_ledger_2"
    When the operator prepares "kestrel_ledger_2" with migrate
    Then migrate refuses naming "ledgerops_app" and "separate cluster" and "DROP ROLE ledgerops_app"
    And the cluster is unchanged
    And the app role logs in with "T7!kq@9x%Zm" on "kestrel_ledger"

  @kpi @KPI-R8 @contract-shape:bounded-change
  Scenario: Running migrate again against the owning database is not refused
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And the cluster also holds an empty database "kestrel_ledger_2"
    When the operator prepares "kestrel_ledger" with migrate
    Then migrate succeeds and reports schema version 6

  @error @kpi @KPI-R8 @contract-shape:unbounded-preservation
  Scenario: The refusal names the privileged remedy when the owning database no longer exists
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And the cluster also holds an empty database "kestrel_ledger_2"
    And the owning database "kestrel_ledger" has been dropped
    When the operator prepares "kestrel_ledger_2" with migrate
    Then migrate refuses naming "DROP ROLE ledgerops_app"
    And the cluster is unchanged

  @error @kpi @KPI-R8 @contract-shape:unbounded-preservation
  Scenario: A first run abandoned after claiming the role refuses a different database
    Given a first migrate of "kestrel_ledger" was abandoned after claiming the role, and the cluster also holds "kestrel_ledger_2"
    And the app password is "T7!kq@9x%Zm"
    When the operator prepares "kestrel_ledger_2" with migrate
    Then migrate refuses naming "DROP ROLE ledgerops_app"
    And the cluster is unchanged

  @kpi @KPI-R8 @contract-shape:bounded-change
  Scenario: A first run abandoned after claiming the role resumes against its own database
    Given a first migrate of "kestrel_ledger" was abandoned after claiming the role, and the cluster also holds "kestrel_ledger_2"
    And the app password is "T7!kq@9x%Zm"
    When the operator prepares "kestrel_ledger" with migrate
    Then migrate succeeds and reports schema version 6

  @kpi @KPI-R8 @contract-shape:bounded-change
  Scenario: Two simultaneous first migrations in one cluster cannot both claim the role
    Given two empty databases "kestrel_ledger" and "kestrel_ledger_2" in one cluster
    And the app password is "T7!kq@9x%Zm"
    When two migrate runs start at the same time, one for each database
    Then exactly one succeeds and the other refuses with the cluster-rule message
    And neither database is left in a dirty migration state
    And only one database owns the app role

  @same-db-concurrent @kpi @KPI-R8 @contract-shape:bounded-change
  Scenario: Two simultaneous migrations of the same database end consistent
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    When two migrate runs start at the same time, both for "kestrel_ledger"
    Then at least one succeeds
    And neither database is left in a dirty migration state
    And the app role logs in with "T7!kq@9x%Zm"

  @error @legacy-demo-db @contract-shape:unbounded-preservation
  Scenario: A second database is refused even when the first was created by the demo stack
    Given a database created by the demo stack at schema version 6 with 40 entries
    And the cluster also holds an empty database "kestrel_ledger_2"
    And the app password is "Zr4#mV8$wQ1"
    When the operator prepares "kestrel_ledger_2" with migrate
    Then migrate refuses naming "ledgerops_app"
    And the cluster is unchanged
