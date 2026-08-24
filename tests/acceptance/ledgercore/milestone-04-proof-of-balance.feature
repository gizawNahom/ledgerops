@slice-04 @us-4 @driving_port @real-io
Feature: Prove the books balance

  US-4 — as the platform operator, I see at a glance whether the ledger is sound.

  The operator asks one question first — "does this add up?" — and wants a yes or
  no before any figures (journey verify-the-books, S2). Detail is what they need
  second, once the answer is no.

  A NO with no attribution would satisfy a detection-only check and still fail
  the operator, so every corruption scenario asserts the account name and the
  delta, not merely that something was caught (KPI-4).

  Scope note (DDR-2): these scenarios drive the console's verdict contract, not
  a browser. The verdict is asserted on both the operator's console surface and
  the health surface, and asserted to agree — so the *answer* is covered on both
  paths the operator can take. The SPA's rendering is a documented untested seam
  (`docs/architecture/atdd-infrastructure-policy.md` § Known gap).

  Background:
    Given the ledger is running against an empty store
    And a system account "treasury" exists

  @env-populated @contract-shape:unbounded-preservation
  Scenario: A healthy ledger states its verdict in words before any figures
    Given a ledger carrying 3 settled transfers
    When the operator asks whether the books balance
    Then the verdict reads "Books balance: YES"
    And the verdict is stated before any per-account figures

  @env-clean @contract-shape:unbounded-preservation
  Scenario: A ledger holding nothing balances, and says so
    When the operator asks whether the books balance
    Then the verdict reads "Books balance: YES"
    And the trial balance is 0.00
    And the verdict reports 0 entries scanned
    And no account is listed as drifted

  @env-populated @kpi-1 @contract-shape:unbounded-preservation
  Scenario: Every entry in a healthy ledger cancels out
    Given a ledger carrying 3 settled transfers
    When the operator asks whether the books balance
    Then the trial balance is 0.00
    And no account is listed as drifted

  @env-populated @kpi-1 @contract-shape:unbounded-preservation
  Scenario: The verdict reports what it scanned and how long it took
    Given a ledger carrying 3 settled transfers
    When the operator asks whether the books balance
    Then the verdict reports 6 entries scanned
    And the verdict reports how long the scan took

  @env-populated @contract-shape:unbounded-preservation
  Scenario: The console and the health check give the operator the same answer
    Given a ledger carrying 3 settled transfers
    When the operator asks whether the books balance on the console surface
    And the operator asks whether the books balance on the health surface
    Then both surfaces give the same verdict
    And both surfaces report the same trial balance

  @error @env-corrupted @kpi-4 @contract-shape:unbounded-preservation
  Scenario: A tampered entry turns the verdict red and names the account
    Given a ledger carrying 3 settled transfers
    And the recorded amount of one entry belonging to "alice" is altered by 5.00 out of band
    When the operator asks whether the books balance
    Then the verdict reads "Books balance: NO"
    And "alice" is listed as drifted
    And the drift entry states the stored balance, the computed balance, and a delta of 5.00

  @error @env-corrupted @kpi-4 @contract-shape:unbounded-preservation
  Scenario: Healthy accounts are not swept up with the drifted one
    Given a ledger carrying 3 settled transfers
    And the recorded amount of one entry belonging to "alice" is altered by 5.00 out of band
    When the operator asks whether the books balance
    Then exactly 1 account is listed as drifted
    And "bob" is not listed as drifted

  @error @env-corrupted @kpi-4 @contract-shape:unbounded-preservation
  Scenario: A tampered stored balance is caught as readily as a tampered entry
    Given a ledger carrying 3 settled transfers
    And the stored balance of "bob" is altered by 7.00 out of band
    When the operator asks whether the books balance
    Then the verdict reads "Books balance: NO"
    And "bob" is listed as drifted
    And the drift entry states the stored balance, the computed balance, and a delta of 7.00

  @pending @error @env-corrupted @kpi-4 @contract-shape:unbounded-preservation
  Scenario: Two damaged accounts are both named, not just the first one found
    Given a ledger carrying 3 settled transfers
    And the recorded amount of one entry belonging to "alice" is altered by 5.00 out of band
    And the stored balance of "bob" is altered by 7.00 out of band
    When the operator asks whether the books balance
    Then the verdict reads "Books balance: NO"
    And exactly 2 accounts are listed as drifted
    And "alice" is listed as drifted
    And "bob" is listed as drifted

  @pending @env-corrupted @real-io @adapter-integration @contract-shape:unbounded-preservation
  Scenario: Tampering is only possible for a privileged operator, never for the service
    Given a ledger carrying 3 settled transfers
    When the service's own credentials attempt to alter a recorded entry
    Then the alteration is refused by the store
    And the verdict still reads "Books balance: YES"

  @pending @error @env-populated @contract-shape:unbounded-preservation
  Scenario: An unidentified caller cannot ask whether the books balance
    Given a ledger carrying 3 settled transfers
    And the caller presents no operator key
    When the operator asks whether the books balance
    Then the caller is refused as unidentified
