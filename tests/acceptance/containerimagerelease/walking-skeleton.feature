@walking_skeleton @driving_adapter @driving_port @real-io @dedicated-cluster @slice-01a @slice-02 @slice-03 @US-1a @US-2 @KPI-R3 @env-fresh-pg16-cluster
Feature: A client prepares their own database and runs ledgerops with a password they chose

  The one scenario that closes this feature's loop through the real binary
  and a real PostgreSQL 16: a developer with only an empty database and a
  privileged login chooses an application password, prepares the database,
  starts ledgerops, and posts a first transfer with that password. This is
  the usable-release-candidate loop (slices 01a and 02, the path slice 03 then
  proves from a registry).

  Stakeholder litmus test: "I brought my own database and my own password, and
  ledgerops ran. The well-known demo password does not work."

  @contract-shape:bounded-change
  Scenario: Priya posts a first transfer on ledgerops running against her own database and password
    Given Priya has an empty database "kestrel_ledger" and a privileged role "kestrel_admin"
    And the app password is "T7!kq@9x%Zm"
    And the operator key is "op-9f3b71c2"
    And the database has been prepared with migrate
    And ledgerops is running
    When Priya posts a first transfer from "treasury" to "alice-01"
    Then the transfer is answered with a transaction id
    And ledgerops reports healthy
    And the app role is refused with "app-secret"
    And no output contains "T7!kq@9x%Zm"
