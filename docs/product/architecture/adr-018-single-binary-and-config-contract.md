# ADR-018: One `ledgerops` binary (serve | migrate | version) and a pure, secret-safe configuration contract

**Status**: Accepted, 2026-10-17, revised in review cycle 1 (DDD-30, DDD-31, DDD-32)
**Owner**: nw-solution-architect (DESIGN, `container-image-release`)

## Context

D4 requires a `ledgerops migrate` subcommand in the same image. Today the image
holds `ledgerops-api` only, `main.go` reads env ad hoc, an unset operator key or
app DSN starts silently, and the app must never hold the migrate role (OPS-10).

## Decision

1. **One binary `ledgerops`**: `serve` (also the no-argument default, so compose
   and `docker run` keep working), `migrate`, `version`
   (`ledgerops <version> (commit <short>)`; unstamped builds print
   `dev (commit unknown)`). The directory stays `cmd/api`; only the output
   binary is renamed. Argument dispatch is a pure function from argv to a
   command value.
2. **Variables**: `LEDGEROPS_MIGRATE_DSN` (privileged DSN, migrate only);
   `LEDGEROPS_APP_PASSWORD` or `LEDGEROPS_APP_PASSWORD_FILE` (migrate only);
   `LEDGEROPS_APP_DSN`, `LEDGEROPS_OPERATOR_KEY` required for serve;
   `LEDGEROPS_DEMO_TENANT_KEY` optional (D9 as revised).
3. **Rules**: both password sources set means refuse (even if equal); blank and
   whitespace-only count as unset; a password file loses exactly one trailing
   `\n` or `\r\n`, no other trimming; all problems are reported in one message;
   validation completes before any database connection.
4. **`serve` refuses to start if `LEDGEROPS_MIGRATE_DSN` is present** in its
   environment. This is the single, named exception to "only the migrate
   command reads the migrate variables": one function (`refuseIfMigrateDSNPresent`)
   in the serve command file checks presence with `os.LookupEnv`, never reads the
   value, and never passes it onward. The AST test allows exactly that site.
5. **Functional shape**: `LoadServeConfig` and `ParseMigrateConfig` are pure
   functions over an env-snapshot value returning config plus every issue; the
   shell reads env and files, prints, exits. Password files are read in the
   shell through an injected read function and passed as a `Secret`.
6. **Secrets**: a `Secret` type in `internal/support` redacts under `String`,
   `GoString` and `slog.LogValuer`; DSN-bearing errors pass through a pure
   redactor. A wrong app password at start is classified (SQLSTATE 28P01) by a
   pure classifier into the existing structured `health.startup.refused` event
   naming the role and "authentication failed", with no password in the line.
7. **Documentation steers to `LEDGEROPS_APP_PASSWORD_FILE`**: environment
   variables are visible through `docker inspect` and process listings of the
   container runtime; the file variant keeps the secret out of both. The env
   variants stay supported (demo compose, simple cases) with that caveat
   stated.

## Alternatives considered

- **Separate `ledgerops-migrate` binary.** Contradicts D4 and doubles the
  artifact. Rejected.
- **`LEDGEROPS_PRIVILEGED_DSN` as the variable name.** Clearer for a managed-DB
  admin, but the user stories, role name and `LEDGEROPS_APP_DSN` parallel favour
  `MIGRATE`. Rejected (user choice).
- **Rename `cmd/api` to `cmd/ledgerops`.** Churn for no behavioural gain.
  Rejected.
- **Validate lazily at first use.** Reproduces today's defect. Rejected.

## Consequences

- Sharing one env file between migrate and serve now fails fast on purpose.
- Enforcement: an AST test allows `LEDGEROPS_MIGRATE_DSN` and
  `LEDGEROPS_APP_PASSWORD[_FILE]` in the migrate command file, plus the one
  presence-only site above (via `LookupEnv`); a `Secret` rendering test;
  sentinel-password tests on every failure path.
- The acceptance suites for US-1/US-2 can drive config parsing as a pure
  function plus one process-level exit-code check.
