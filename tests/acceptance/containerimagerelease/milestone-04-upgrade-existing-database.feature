@real-io @dedicated-cluster @US-4 @slice-04 @env-legacy-demo-db
Feature: Upgrade a demo database in place and set my own password

  A database built by the demo stack (frozen pre-feature migrations, role
  accepting "app-secret") is brought current without touching history, and
  every failure leaves it exactly as it was.

  @pending @driving_port @kpi @KPI-R4 @contract-shape:bounded-change
  Scenario Outline: An existing demo database is brought current without touching history
    Given a database created by the demo stack at schema version <from> with 1240 entries
    And the app role of that database accepts "app-secret"
    And the app password is "Zr4#mV8$wQ1"
    When the operator upgrades the database with migrate
    Then migrate succeeds and reports schema version 6
    And the history is untouched and the trial balance is zero
    And the app role logs in with "Zr4#mV8$wQ1"

    Examples:
      | from |
      | 4    |
      | 6    |

  @pending @driving_port @kpi @KPI-R3 @contract-shape:bounded-change
  Scenario: The old well-known password stops working
    Given a database created by the demo stack at schema version 6 with 40 entries
    And the app password is "Zr4#mV8$wQ1"
    When the operator upgrades the database with migrate
    Then the app role logs in with "Zr4#mV8$wQ1"
    And the app role is refused with "app-secret"

  @pending @driving_port @contract-shape:unbounded-preservation
  Scenario: Running migrate again with the same password changes nothing
    Given a database created by the demo stack at schema version 6 with 40 entries
    And the app password is "Zr4#mV8$wQ1"
    And the operator upgrades the database with migrate
    When the operator runs migrate again
    Then migrate reports "no pending migrations"
    And the cluster is unchanged

  @pending @driving_port @contract-shape:bounded-change
  Scenario: A later run with a different password rotates it
    Given a database created by the demo stack at schema version 6 with 40 entries
    And the app password is "Zr4#mV8$wQ1"
    And the operator upgrades the database with migrate
    And the app password is "Lm2&pT6@sD9"
    When the operator runs migrate again
    Then the app role logs in with "Lm2&pT6@sD9"
    And the app role is refused with "Zr4#mV8$wQ1"

  @pending @driving_port @contract-shape:unbounded-preservation
  Scenario: History stays append-only after the upgrade
    Given a database created by the demo stack at schema version 4 with 40 entries
    And the app password is "Zr4#mV8$wQ1"
    And the operator upgrades the database with migrate
    Then the app role can neither update nor delete an entry

  # --- Rule B -----------------------------------------------------------------------

  @pending @error @rule-b-interrupted @kpi @KPI-R3 @contract-shape:unbounded-preservation
  Scenario: A failed schema step leaves the old password working and says so
    Given a database created by the demo stack at schema version 4 with 40 entries
    And a later migration is made to fail
    And the app password is "Zr4#mV8$wQ1"
    When the operator upgrades the database with migrate
    Then migrate refuses, saying the password was not changed
    And the app role logs in with "app-secret"

  @pending @error @rule-b-interrupted @kpi @KPI-R3 @contract-shape:unbounded-preservation
  Scenario: A run killed before the password step leaves the old password working
    Given a database created by the demo stack at schema version 4 with 40 entries
    And the app password is "Zr4#mV8$wQ1"
    When a migration run is killed before the password step
    Then the app role logs in with "app-secret"

  # --- privilege --------------------------------------------------------------------

  @pending @managed-pg-nonsuperuser @driving_port @contract-shape:bounded-change
  Scenario: A privileged role holding exactly the supported minimum succeeds without being a superuser
    Given a privileged role that holds exactly the supported minimum
    And the app password is "T7!kq@9x%Zm"
    When the operator prepares the database with migrate
    Then migrate succeeds and reports schema version 6
    And the app role logs in with "T7!kq@9x%Zm"

  @pending @managed-pg-nonsuperuser @driving_port @contract-shape:bounded-change
  Scenario: A role that does not own the database but may grant connect on it succeeds
    Given a privileged role that does not own the database but may grant connect on it
    And the app password is "T7!kq@9x%Zm"
    When the operator prepares the database with migrate
    Then migrate succeeds and reports schema version 6

  @error @managed-pg-nonsuperuser @contract-shape:unbounded-preservation
  Scenario Outline: A privileged role missing one privilege fails clearly before any change
    Given a privileged role that <gap>
    And the app password is "Zr4#mV8$wQ1"
    When the operator prepares the database with migrate
    Then migrate refuses naming the missing privilege and "no changes applied"
    And the cluster is unchanged
    And no output contains "Zr4#mV8$wQ1"

    Examples:
      | gap                                                     |
      | cannot create roles                                     |
      | has no admin option on the app role                     |
      | neither owns the database nor may grant connect on it   |
      | cannot create objects in the public schema              |
      | does not own a ledger table that must be altered        |

  @pending @error @managed-pg-nonsuperuser @rule-b-interrupted @contract-shape:unbounded-preservation
  Scenario: An upgrade refused for a missing privilege leaves the old password working
    Given a privileged role that has no admin option on the app role
    And the app password is "Zr4#mV8$wQ1"
    When the operator prepares the database with migrate
    Then migrate refuses naming the missing privilege and "no changes applied"
    And the app role logs in with "app-secret"

  @guard @contract-shape:unbounded-preservation @pending
  Scenario: The frozen demo database really does accept the demo password
    Given a database created by the demo stack at schema version 6 with 40 entries
    Then the app role logs in with "app-secret"
