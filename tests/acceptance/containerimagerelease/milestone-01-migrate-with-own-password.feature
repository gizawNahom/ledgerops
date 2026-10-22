@real-io @dedicated-cluster @US-1a @slice-01a @env-fresh-pg16-cluster
Feature: Migrate a fresh database with my own app password

  The operator brings a privileged login and a password. After migrate the
  application role accepts exactly that password and never the demo one, at
  any instant, even if the run dies halfway. The password is never printed.

  # --- happy paths ------------------------------------------------------------

  @driving_port @kpi @KPI-R3 @contract-shape:bounded-change
  Scenario: A fresh database is ready with the password the operator chose
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    When the operator prepares the database with migrate
    Then migrate succeeds and reports schema version 6
    And the app role logs in with "T7!kq@9x%Zm"
    And the app role is refused with "app-secret"
    And the app role cannot update an entry
    And the cluster changed only by the new app role and the new ledger schema
    And no output contains "T7!kq@9x%Zm"

  @driving_port @nonroot-secret-files @contract-shape:bounded-change
  Scenario Outline: A password from a mounted secret file loses only its trailing line ending
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the password file holds "Zr4#mV8$wQ1" followed by <ending>
    When the operator prepares the database with migrate
    Then the app role logs in with "Zr4#mV8$wQ1"

    Examples:
      | ending                           |
      | a line feed                      |
      | a carriage return and line feed  |
      | no line ending                   |

  @image @driving_port @nonroot-secret-files @contract-shape:bounded-change
  Scenario: The published image prepares a database from a world-readable secret file
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the password file holds "Zr4#mV8$wQ1" followed by a line feed
    And the password file can be read by any user
    When the operator prepares the database with migrate
    Then migrate succeeds and reports schema version 6
    And the app role logs in with "Zr4#mV8$wQ1"

  # --- refusals before any change ----------------------------------------------

  @error @image @nonroot-secret-files @contract-shape:unbounded-preservation
  Scenario: A secret file the image user cannot read gives a named error
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the password file holds "Zr4#mV8$wQ1" followed by a line feed
    And the password file can be read only by its owner while the image runs as another user
    When the operator prepares the database with migrate
    Then migrate refuses naming "ledgerops_app_password" and "not readable"
    And the cluster is unchanged

  @error @driving_port @contract-shape:unbounded-preservation
  Scenario Outline: Migrate refuses without a usable password
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is <state>
    When the operator prepares the database with migrate
    Then migrate refuses naming "LEDGEROPS_APP_PASSWORD" and "LEDGEROPS_APP_PASSWORD_FILE"
    And no ledger data and no application login exist
    And the cluster is unchanged
    And the app role is refused with "app-secret"

    Examples:
      | state                  |
      | not set                |
      | set to an empty value  |
      | set to whitespace only |

  @error @driving_port @contract-shape:unbounded-preservation
  Scenario Outline: Setting both password sources is refused as ambiguous
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm" and the password file holds "<file content>"
    When the operator prepares the database with migrate
    Then migrate refuses naming "only one"
    And the cluster is unchanged

    Examples:
      | file content |
      | T7!kq@9x%Zm  |
      | Zr4#mV8$wQ1  |

  # --- Rule A: no moment when the demo password works ----------------------------

  @driving_port @kpi @KPI-R3 @contract-shape:bounded-change
  Scenario: A fresh database never accepts the demo password at any point during migrate
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    When the operator prepares the database with migrate while the demo password is tried continuously
    Then migrate succeeds and reports schema version 6
    And the demo password was refused at every observation
    And the app role logs in with "T7!kq@9x%Zm"

  @error @dirty-migration @kpi @KPI-R3 @contract-shape:bounded-change
  Scenario: A run killed between two migration files leaves no demo password behind
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    When a migration run is killed between two migration files
    Then the database stays at version 3 with no unfinished migration
    And the app role is refused with "app-secret"

  @error @dirty-migration @kpi @KPI-R3 @contract-shape:bounded-change
  Scenario: A run killed between two migration files completes when migrate is run again
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    And a previous run was killed between two migration files
    When the operator runs migrate again
    Then migrate succeeds and reports schema version 6
    And the app role logs in with "T7!kq@9x%Zm"

  @error @dirty-migration @kpi @KPI-R3 @contract-shape:unbounded-preservation
  Scenario: A run killed during a migration file leaves an unfinished migration and no partial change
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    When a migration run is killed during a migration file
    Then the interrupted migration 3 is flagged unfinished and left no partial change
    And the app role is refused with "app-secret"

  @error @dirty-migration @kpi @KPI-R3 @contract-shape:unbounded-preservation
  Scenario: Migrate refuses to continue from an interrupted migration
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    And a previous run was killed during a migration file
    When the operator runs migrate again
    Then migrate refuses, naming the unfinished migration 3
    And the cluster is unchanged

  # --- secrets never leak ---------------------------------------------------------

  @pending @error @driving_port @contract-shape:unbounded-preservation
  Scenario Outline: Migrate output never contains the password, whatever goes wrong
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    And <situation>
    When the operator prepares the database with migrate
    Then migrate refuses naming "<named>"
    And no output contains "T7!kq@9x%Zm"
    And no output contains "admin-secret"

    Examples:
      | situation                                                  | named            |
      | the privileged role cannot create roles                    | CREATEROLE       |
      | a previous run left an unfinished migration                | unfinished migration |
      | the database cannot be reached                             | no_such_database |
      | both password sources are set                              | only one         |
      | the cluster already holds another ledgerops database       | ledgerops_app    |

  @ddl-logging-cluster @kpi @KPI-R3 @contract-shape:bounded-change
  Scenario: The database server log never sees the plaintext password
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin" on a server that logs every statement
    And the app password is "T7!kq@9x%Zm"
    When the operator prepares the database with migrate
    Then migrate succeeds and reports schema version 6
    And the database server log never contains "T7!kq@9x%Zm"

  @scram-unicode @contract-shape:bounded-change
  Scenario Outline: The same password typed in another Unicode form still logs in
    Given the operator has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is typed in its <set form> form
    When the operator prepares the database with migrate
    Then the app role logs in with the password typed in its <login form> form

    Examples:
      | set form   | login form |
      | composed   | composed   |
      | composed   | decomposed |
      | decomposed | composed   |
      | fullwidth  | composed   |

  # --- wrong password at start ------------------------------------------------------

  @error @wrong-password-start @driving_port @US-2 @KPI-R9 @contract-shape:unbounded-preservation
  Scenario: A mismatched password is caught at app start, not at the first request
    Given the database "kestrel_ledger" was migrated with the app password "T7!kq@9x%Zm"
    And the app login uses the password "wrong-guess"
    And the operator key is "op-9f3b71c2"
    When ledgerops starts
    Then ledgerops refuses to start naming "ledgerops_app" and "authentication failed"
    And no output contains "T7!kq@9x%Zm"
    And no output contains "wrong-guess"
