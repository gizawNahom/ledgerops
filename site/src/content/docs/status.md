---
title: Status, limits and operating
description: What ledgerops supports today, what it doesn't do yet, how it performed under load, and what running it takes.
---

ledgerops is pre-1.0. The core is complete and covered by property-based, race
and crash tests, but it isn't ready for production yet and has not been run in
production.

## Supported today

- Accounts (`wallet` and `system`) and two-leg transfers between them, with
  overdraft protection, idempotent retries and append-only history.
- Multiple tenants with isolated data and their own API keys, plus
  dual-authorized transfers between tenants.
- An on-demand check that the books balance, with per-account drift detection.
- Prometheus metrics, structured JSON request logs and an operator console.
- Expand-only database migrations, which only add to the schema, so rolling
  back the service never needs a schema rollback.

## Not yet

- **One currency.** The API accepts USD amounts only.
- **No holds.** There is no reserve-then-capture or void flow.
- **No transactions with more than two legs.** Paying a seller and a platform
  fee atomically takes two transfers today.
- **No metadata on transfers.** There is no field for your own order or
  reference ids.
- **No webhooks or event stream.** Integrations have to poll.
- **No OpenAPI spec, client SDKs or stable release.** A signed release-candidate
  image is published (`ghcr.io/gizawnahom/ledgerops:0.2.0-rc.1`), but the
  [quickstart](../quickstart/) builds from source.
- **No tenant key rotation** or tenant offboarding.
- **No TLS in the service itself.** Run it behind a TLS-terminating proxy.
- **Not tested with more than one app instance.**
- **Hot-account write contention.** See below.
- **No independent verification.** The books check is run by the service
  itself. See [Guarantees](../guarantees/#what-these-checks-dont-cover).

## Performance

**In short:** tested at about 276 requests per second on a small,
resource-capped stack with no errors. Concurrent transfers on the same account
queue behind each other, so a single very busy account, such as a treasury or
fee account, is the first limit you'll hit.

The load test uses k6 in four profiles (`load`, `stress`, `soak` and
`capacity`), triggered manually through a GitHub Actions workflow. It never runs
on push. The `capacity` profile holds a flat 300 requests per second against a
CPU- and memory-capped stack, with 80% of traffic aimed at 100 "hot" accounts
out of 500. It passes only if throughput, p95 latency (under 500 ms) and errors
(under 1%) all hold.

In the last recorded run (2026-09-02), throughput (276.5 requests per second)
and errors (0%) held, but p95 latency reached 923 ms. CPU, memory and the
connection pool were each ruled out. During the run, 41 to 50 PostgreSQL
connections were waiting on row locks.

Every transfer locks its sender and recipient rows (`SELECT … FOR UPDATE`), which
is what makes the no-overdraft guarantee hold under concurrency. Transfers that
touch the same account therefore queue behind each other. Spreading the hot
traffic across more accounts didn't relieve it, and reads never take these
locks. The latency threshold was kept strict instead of loosened to pass. The
full investigation is in the
[load-test notes](https://github.com/gizawNahom/ledgerops/blob/main/docs/feature/ledger-core/feature-delta.md).

## Operating it

- **TLS.** The service doesn't terminate TLS. Put it behind a TLS-terminating
  proxy.
- **Observability.** `GET /metrics` serves Prometheus metrics and is
  unauthenticated by design, so scrapers don't need a key. Request logs are
  structured JSON. The demo stack also runs Prometheus on `:9090` and Grafana on
  `:3000`.
- **One binary.** `ledgerops` has two working commands: `serve` (the default)
  and `migrate`. A `version` command is planned but isn't implemented yet.
- **Configuration is environment variables.**
  - `serve` needs `LEDGEROPS_APP_DSN` and `LEDGEROPS_OPERATOR_KEY`.
  - `migrate` uses `LEDGEROPS_MIGRATE_DSN` (a privileged connection string) and
    the app role's password, from `LEDGEROPS_APP_PASSWORD` or
    `LEDGEROPS_APP_PASSWORD_FILE`.
  - Don't give `serve` the `LEDGEROPS_MIGRATE_DSN` variable. The design
    (ADR-018) has it refuse to start if that variable is set, but that guard
    isn't wired up yet.
- **Two database roles.** Migrations run as a privileged role and the service
  runs as `ledgerops_app`. The migrate role needs a documented minimum set of
  privileges, and `migrate` checks for them before it changes anything.
- **One ledgerops database per PostgreSQL cluster.**
- **Expand-only migrations.** Rolling the service back never needs a schema
  rollback.
- **Rotating the app password.** Update your secret store, run
  `ledgerops migrate` with the new password, then redeploy the app. A running
  app must be restarted after a rotation, which means a short outage.
- **A single instance.** It hasn't been tested with more than one app instance.

The reasoning is in
[ADR-017](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-017-app-role-password-and-edited-0001.md),
[ADR-018](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-018-single-binary-and-config-contract.md),
[ADR-022](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-022-privileged-role-minimum-and-preflight-probe.md)
and
[ADR-023](https://github.com/gizawNahom/ledgerops/blob/main/docs/product/architecture/adr-023-password-rotation-requires-restart.md).

## Roadmap

1. **Adoptable.** Versioned releases with a published container image, an
   OpenAPI spec, and metadata on transfers.
2. **Wallet essentials.** Holds (reserve, capture, void), transactions with
   more than two legs, and webhooks driven by an outbox, so only committed
   postings are announced.
3. **Tamper evidence.** Hash-chained entries, signed checkpoints, and a
   standalone `ledgerops verify` command that an auditor can run against a copy
   of the database without trusting the running service.
4. **Performance.** Shorter lock hold times on the write path, then group
   commit for busy accounts.
