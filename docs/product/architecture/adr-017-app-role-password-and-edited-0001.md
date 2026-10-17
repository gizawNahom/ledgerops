# ADR-017: Edit migration 0001 in place; `ledgerops_app` is born NOLOGIN; one ledgerops database per Postgres cluster; the operator's password is set Go-side as a SCRAM verifier

**Status**: Accepted, 2026-10-17, revised in review cycle 1 (DDD-27, DDD-28, DDD-29, DDD-41, DDD-47)
**Owner**: nw-solution-architect (DESIGN, `container-image-release`)

## Context

`0001_init.up.sql` creates `ledgerops_app LOGIN PASSWORD 'app-secret'` inside an
`IF NOT EXISTS` guard and runs `GRANT CONNECT ON DATABASE ledgerops`. Operators
must bring their own password (D5), on any database name (US-1), and the app
role must never accept a password the operator did not supply on a database a
`migrate` run created (Rule A). Databases created before this feature already
accept `app-secret`; that is existing state until a `migrate` run completes the
password step (Rule B).

A PostgreSQL role is cluster-global, `ledgerops_app` has a fixed name across
every migration's grants, and `ALTER ROLE ... PASSWORD` therefore affects every
database in the cluster that grants it access. Two ledgerops databases in one
cluster would share one password, and a "fresh" migrate against the second
would either inherit an existing role (possibly `LOGIN` with `app-secret`,
breaking Rule A) or rotate the first database's credential behind its back.

Verified: a new migration 0007 cannot help, because 0001 itself fails on any
database not named `ledgerops` before 0007 is reached. golang-migrate stores
only a version number and does no checksum, so an edited 0001 is never re-run
on a migrated database.

## Decision

1. **Edit 0001 once, in place.** The role is created `NOLOGIN` with no
   password; the `GRANT CONNECT` uses `current_database()` via dynamic SQL. A
   recorded exception to "applied migrations are immutable" (DDD-27).
2. **One ledgerops database per Postgres cluster (user decision, DDD-41).**
   When the target database has no ledgerops schema (no `schema_migrations`
   row) and the role `ledgerops_app` already exists in the cluster, `migrate`
   refuses before any change, naming the conflict (role exists in this
   cluster; run migrate against the database that owns it, or use a separate
   cluster). Rule A then holds by construction on every database `migrate`
   creates, and a rotation can never reach another database. A target that
   already carries the ledgerops schema (upgrade, rotation, re-run) is
   unaffected. The refusal has no override flag in the production binary.
   **Atomicity (DDD-48, review cycle 2).** A read-then-decide check is racy:
   two concurrent fresh migrates against different databases would both see
   "role absent", and the loser would then fail inside 0001 with a duplicate
   role (42710), leaving a dirty version 1. A cluster-scoped advisory lock was
   rejected because `pg_advisory_lock` keys are scoped to the current database
   (the lock tag carries the database OID), so migrates targeting different
   databases would not contend; locking in a fixed shared database such as
   `postgres` would need CONNECT there, which managed instances often
   withhold. The arbiter is instead the one cluster-global atomic operation
   already needed: **role creation**. When the role is absent, `migrate` first
   claims it in its own committed transaction: `CREATE ROLE ledgerops_app
   NOLOGIN` plus `COMMENT ON ROLE ledgerops_app IS 'ledgerops-owner-database=<target>'`.
   PostgreSQL's unique index on `pg_authid` lets exactly one concurrent
   creator win; the loser gets 42710, re-reads the marker, and refuses with
   the cluster-rule message (its schema was never touched, nothing is dirty).
   The claim precedes any schema step, and the role is NOLOGIN, so Rule A is
   unaffected. A later run against the database named in the marker resumes
   (an interrupted first run); a run against any other fresh database is
   refused. On a legacy role with no marker (created by the old 0001), a
   successful migrate of the database that carries the schema stamps the
   marker. The guarantee therefore holds by construction, with no lock.
   **Remedy for a dropped owning database (N3):** when the marker names a
   database that no longer exists, the refusal message says so and names the
   privileged remedy, `DROP ROLE ledgerops_app` (it succeeds once the owning
   database is gone), after which a fresh migrate may claim the role again.
   The same remedy text applies to an abandoned claim; the binary never drops
   a role itself.
3. **Password step is Go-side, after all SQL migrations, always applied**:
   `ALTER ROLE ledgerops_app LOGIN PASSWORD '<SCRAM-SHA-256 verifier>'`, on
   create and on rotate, including when the role already exists (DDD-28).
4. **Verifier construction (DDD-47)**: the password is SASLprep-normalised
   (RFC 4013, the same normalisation pgx's client applies, falling back to the
   raw bytes when normalisation fails, as PostgreSQL's `pg_saslprep` does)
   before PBKDF2-HMAC-SHA-256. Salt and iteration count (4096, the PostgreSQL
   default) are injected so unit tests are deterministic; production draws the
   salt from `crypto/rand`. A non-ASCII acceptance case (composed and
   decomposed forms, fullwidth characters) proves a login by a real client
   succeeds.
5. **Rule A** holds by construction: on a fresh database the role cannot log
   in at any instant, including after a crash and under `trust` pg_hba.
   **Rule B** holds by ordering: the password step is last; a failure before it
   leaves `app-secret` working and the output says the password was not changed.
6. **Dirty state (DDD-29).** Verified in golang-migrate v4.18.3
   (`database/postgres/postgres.go`): without `x-multi-statement`, `Run`
   executes each file in a single `ExecContext` on the lib/pq simple-query
   path, so each file is one implicit transaction; the dirty flag is set in a
   separate transaction before the file runs and cleared after. A kill during a
   file therefore leaves version N dirty with the schema either at N-1 (the
   implicit transaction rolled back, the overwhelmingly common case) or at N
   (killed between commit and the clearing write, a window of one statement).
   The binary state cannot be told from the version row alone, so `migrate`
   detects the dirty flag, exits non-zero naming version N and both candidate
   states, changes nothing, and has no `--force` (the image contains no
   recovery tool). The premise is guarded by a test: a fixture migration that
   sleeps mid-file is killed with `pg_terminate_backend`, and the test asserts
   no partial effect and `dirty = true`. The DSN must not carry
   `x-multi-statement`; a test asserts it.
   **Manual recovery** (documented in the quickstart, slice 08): connect with
   the privileged role, check whether migration N's declared objects exist,
   then `UPDATE schema_migrations SET version = <N-1 or N>, dirty = false`,
   then re-run `migrate`.
7. **Guards**: the old 0001 (and 0002-0006) are frozen as a test fixture before
   the edit lands; a checksum test pins 0001 (edited) through 0006.

## Alternatives considered

- **Keep `LOGIN PASSWORD 'app-secret'` in 0001, reset Go-side afterwards.**
  Fresh databases accept the demo password from the first statement. Violates
  Rule A. Rejected.
- **Go pre-creates the role with the supplied password, then runs SQL.**
  0001's grant still needs the edit; keeps a literal in 0001 and splits role
  creation across two places. Rejected.
- **Plaintext `ALTER ROLE ... PASSWORD %L` via `format()`.** Leaks under
  `log_statement=ddl`, and `ALTER ROLE` cannot take bind parameters. Rejected
  for the verifier.
- **Allow a second ledgerops database and rotate or reuse the existing role
  silently.** Either breaks Rule A (inherited `app-secret`) or changes the first
  database's credential unexpectedly. Rejected by the user.
- **Advisory lock for the check.** Per-database scope (above); does not serialise
  migrates on different databases. Rejected.
- **A per-database role name (`ledgerops_app_<db>`).** Every migration's grants,
  the probe and the compose files name `ledgerops_app`; changes the public
  contract. Rejected.
- **Auto-force a dirty version.** The true state is not derivable from the
  version row. Rejected.

## Consequences

- `Migrate`/`MigrateStep` gain the password parameter. Direct call sites (7 in
  6 files): `ledger_seeding.go` (2: `Migrate` and `MigrateStep`),
  `ledger_observations.go`, `multitenancy/world.go`, `intertenanttransfer/world.go`,
  `internal/adapters/postgres/testhelper_test.go`,
  `internal/adapters/http/trial_balance_tenant_scope_test.go`;
  `steps_ledger_test.go` reaches it through `MigrateFromZero`. The compiler
  finds every site.
- **Test-harness impact** (feature-delta § Test harness impact): the three
  acceptance worlds migrate one template database per test binary in a pristine
  cluster (role absent, allowed) and then clone it with `CREATE DATABASE ...
  TEMPLATE`, which never invokes `migrate`; they are unaffected. Scenarios that
  create the role, rotate its password, or test the refusal need a dedicated
  cluster (or, for the refusal scenario, an empty second database created in a
  cluster that already has the role). No test-only bypass exists in the
  production path. For the deferred `LEDGEROPS_TEST_PG_DSN` shared instance
  (user decision, review cycle 2): the first ledgerops database migrated there
  is the template and the only ledgerops database allowed in that cluster;
  later runs skip migrate (schema present) and clone per scenario;
  `@dedicated-cluster` scenarios get their own container.
- "Re-run completes" holds for a kill between migration files; the mid-file
  kill is the dirty refusal. DISTILL needs both scenarios.
- The verifier is not a secret-safe artifact: under `log_statement=ddl` it is
  written to the server log and can be attacked offline at 4096 iterations.
  Operators should use a high-entropy generated password and keep statement
  logs access-controlled. Plaintext never reaches the server.
- Expand-only discipline and append-only entries are unchanged.
