@ops-5 @slice-06 @env-clean
Feature: The ledger can be watched from outside without reading its source

  OPS-5 (feature-delta.md § Wave: DEVOPS / Observability stack) decided a
  Prometheus exposition at GET /metrics and structured per-request JSON logs,
  and the decision was never discharged: no scenario ever exercised either
  one, and the status table said "Exposed" and "Implemented in DELIVER" while
  the route answered 501 __SCAFFOLD__ and no request-scoped field was ever
  logged. Root cause and evidence:
  docs/analysis/2026-08-26-observability-status-false-claim-rca.md.

  GET /metrics is mounted unauthenticated — confirmed 2026-08-26, mirroring
  the unauthenticated precedent GET /console and its static assets already
  set (internal/adapters/http/console_static.go): a scrape target cannot
  easily present an operator bearer key, and Prometheus never will.

  Every scenario below carries @pending (ADR-025) — this fix inherits its
  walking skeleton from slice 01 rather than authoring a second one; DELIVER
  unskips these one at a time.

  Background:
    Given the ledger is running against an empty store
    And a system account "treasury" exists

  # --- metrics exposition --------------------------------------------------

  @driving_adapter @real-io @contract-shape:unbounded-preservation
  Scenario: An operator scrapes metrics without presenting any credentials
    When the operator scrapes the metrics endpoint with no credentials
    Then the scrape succeeds with metrics exposition text
    And the exposition names every declared series

  @driving_adapter @real-io @error @contract-shape:unbounded-preservation
  Scenario: Scraping metrics with a rejected operator key still succeeds
    When the operator scrapes the metrics endpoint presenting an operator key that was never issued
    Then the scrape succeeds with metrics exposition text

  @real-io @contract-shape:bounded-change
  # Fixed (fix-ledger-core-observability, 2026-08-28): the test-infrastructure
  # timing bug described above (baseline captured at serve()-time, before this
  # scenario's own funding Given ran) is resolved by re-capturing the baseline
  # immediately before the measured transfer — see the "the integrator moves
  # ... under key ..." step (steps_ledger_test.go) and
  # ledger_observability.go § CaptureMetricsBaseline. Not a production defect;
  # ObservePosting's instrumentation semantics were correct throughout.
  Scenario: A posted transfer is counted and timed
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 20.00 from "alice" to "bob" under key "obs-1"
    Then the transfer is accepted
    And the postings counter for outcome "posted" increased by 1
    And the posting duration was observed at least once

  @real-io @error @contract-shape:bounded-change
  Scenario: A transfer refused for insufficient funds is counted on both series
    Given a wallet account "alice" funded with 10.00
    And a wallet account "bob" exists
    When the integrator moves 50.00 from "alice" to "bob" under key "obs-2"
    Then the transfer is refused as insufficient funds
    And the postings counter for outcome "rejected" increased by 1
    And the insufficient-funds counter increased by 1

  @real-io @contract-shape:bounded-change
  Scenario: A replayed transfer is counted as a replay, not a fresh posting
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 20.00 from "alice" to "bob" under key "obs-3"
    And the integrator repeats the same request under key "obs-3"
    Then the repeat is answered as a replay
    And the postings counter for outcome "replayed" increased by 1
    And the idempotent-replay counter increased by 1

  @real-io @contract-shape:unbounded-preservation
  Scenario: Verifying the books is reflected in the trial-balance gauges
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 20.00 from "alice" to "bob" under key "obs-4"
    And the operator asks whether the books balance
    Then the verdict reads "Books balance: YES"
    And the trial-balance imbalance gauge reads 0.00
    And the trial-balance scan duration was observed at least once

  @real-io @error @contract-shape:unbounded-preservation
  Scenario: Corruption is reflected in the drifted-accounts gauge
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    And the stored balance of "alice" is altered by 5.00 out of band
    When the operator asks whether the books balance
    Then the verdict reads "Books balance: NO"
    And the drifted-accounts gauge reads 1

  @real-io @contract-shape:unbounded-preservation
  Scenario: A ledger with nothing drifted reports the drifted-accounts gauge at zero
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the operator asks whether the books balance
    Then the drifted-accounts gauge reads 0

  # --- structured per-request logging ---------------------------------------

  @real-io @contract-shape:unbounded-preservation
  Scenario: Every request is logged with its identifying fields
    Given a wallet account "alice" funded with 100.00
    When the operator traces "alice"
    Then a log line for that request carries a request id, its route, its status, and how long it took

  @real-io @contract-shape:unbounded-preservation
  Scenario: The logged route is the matched pattern, not the raw path a caller happened to use
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" funded with 100.00
    When the operator traces "alice"
    And the operator traces "bob"
    Then both requests are logged under the very same route

  @real-io @contract-shape:unbounded-preservation
  Scenario: A posting is logged with the movement it made
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 20.00 from "alice" to "bob" under key "obs-5"
    Then a log line for that request names the transaction, both accounts, the amount moved, and the currency

  @real-io @error @contract-shape:unbounded-preservation
  Scenario: A refused posting is logged with the reason it was refused
    Given a wallet account "alice" funded with 10.00
    And a wallet account "bob" exists
    When the integrator moves 50.00 from "alice" to "bob" under key "obs-6"
    Then a log line for that request carries the violation "insufficient_funds"

  @real-io @contract-shape:unbounded-preservation
  Scenario: A first-time posting under a key is logged as not replayed
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 20.00 from "alice" to "bob" under key "obs-7"
    Then a log line for that request carries a hashed idempotency key and states it was not a replay

  @real-io @contract-shape:unbounded-preservation
  Scenario: A retried posting is logged as a replay
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 20.00 from "alice" to "bob" under key "obs-8"
    And the integrator repeats the same request under key "obs-8"
    Then a log line for that request carries a hashed idempotency key and states it was a replay

  @real-io @error @contract-shape:unbounded-preservation
  Scenario: An unidentified caller's rejection is logged too
    Given the caller presents no operator key
    When the operator asks whether the books balance
    Then a log line for that request carries the violation "unidentified_caller"
    And the same log line carries the status 401

  # --- the release-blocking guarantee ---------------------------------------
  # Task brief, verbatim: the raw Authorization header value (operator bearer
  # key) and the raw idempotency key value must NEVER appear in any captured
  # log output, on ANY path including 4xx/5xx error-echo paths. Not a doc
  # note — real scenarios, real assertions against captured log output.

  @pending @real-io @security @error @release-blocking @contract-shape:unbounded-preservation
  Scenario: The operator's credential never appears in a captured log line, however the caller is answered
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 20.00 from "alice" to "bob" under key "obs-9"
    And the caller presents an operator key that was never issued
    And the operator asks whether the books balance
    And the caller presents no operator key
    And the operator asks whether the books balance
    Then a log line for that request carries a request id, its route, its status, and how long it took
    And no captured log line contains the operator's key in any form

  @pending @real-io @security @error @release-blocking @contract-shape:unbounded-preservation
  Scenario: The caller's idempotency key never appears in a captured log line, including on a request-echo error path
    Given a wallet account "alice" funded with 100.00
    And a wallet account "bob" exists
    When the integrator moves 20.00 from "alice" to "bob" under key "the-callers-own-secret-key"
    And the integrator repeats the same request under key "the-callers-own-secret-key" with its fields reordered and respaced
    Then no captured log line contains the raw idempotency key "the-callers-own-secret-key"
    And a log line for that request carries a hashed idempotency key instead

  # --- regression: the router restructuring this fix requires must not widen
  # the auth boundary it does not touch ---------------------------------------

  @real-io @regression @error @contract-shape:unbounded-preservation
  Scenario Outline: Every JSON endpoint except the metrics exposition still requires the operator key
    Given a wallet account "alice" exists
    When an unidentified caller requests "<endpoint>"
    Then the caller is refused as unidentified

    Examples:
      | endpoint                     |
      | POST /accounts                |
      | POST /transfers                |
      | GET /accounts/alice             |
      | GET /accounts/alice/entries      |
      | GET /health/trial-balance         |
      | GET /console/verdict               |
