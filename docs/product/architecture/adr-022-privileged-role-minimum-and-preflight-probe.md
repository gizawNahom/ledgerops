# ADR-022: Supported minimum privilege set for the migrate role; migration is a pure plan over gathered facts

**Status**: Accepted, 2026-10-17, revised in review cycle 1 (DDD-39, DDD-42)
**Owner**: nw-solution-architect (DESIGN, `container-image-release`)

## Context

OQ-8: which privileges `migrate` needs on bring-your-own and managed Postgres.
The code does not require the name `ledgerops_migrate`. PostgreSQL 16 changed
`CREATEROLE`: a role may only manage roles it created or holds `ADMIN OPTION`
on. The first revision of this ADR proposed one rolled-back `ALTER ROLE` as the
preflight, which proves only that single privilege and is a side-effecting
preview.

## Decision

1. **Supported, tested minimum** (explicit checklist; each item is a fact the
   preflight gathers and the plan reasons over):
   - connected as a role that owns the target database, or holds `CONNECT`
     with grant option on it, or is superuser (needed for `GRANT CONNECT`);
   - `CREATE` on schema `public` (not implied on PostgreSQL 15 and later unless
     owner);
   - `CREATEROLE` when `ledgerops_app` does not exist;
   - on PostgreSQL 16, `ADMIN OPTION` on an existing `ledgerops_app` (or
     superuser) to alter it;
   - membership in (or being) the owner of every existing ledgerops table that
     pending migrations `ALTER` or `GRANT` on;
   - the cluster rule (ADR-017): no `ledgerops_app` role in the cluster when the
     target has no ledgerops schema; and no foreign table named like a ledgerops
     table in a database that has no ledgerops schema (a collision that
     `CREATE TABLE IF NOT EXISTS` would silently adopt).
2. **Facts, then a pure plan (DDD-42).** Two separately named ports gather
   them. `FactReader` is read-only (catalog queries: `pg_roles`,
   `pg_auth_members.admin_option`, `pg_database.datdba`,
   `shobj_description` for the owner marker,
   `has_database_privilege(... 'CONNECT WITH GRANT OPTION')`,
   `has_schema_privilege`, `pg_has_role(owner, 'MEMBER')`,
   `schema_migrations`). `PreflightProber` is **explicitly write-capable**
   (its statements are DDL) and runs **empirical probes inside one transaction
   that is rolled back, never committed**: `CREATE TABLE` in `public`, `ALTER ROLE
   ledgerops_app` (existing role) or `CREATE ROLE` of a throwaway name (absent
   role), and `GRANT CONNECT` on the database to `PUBLIC` (already the default,
   so a no-op even if it were ever committed). Catalog inference alone can
   mis-judge inherited or default privileges; the empirical statements are the
   Earned-Trust check. `PlanMigration(facts, config)` is a pure function that
   returns either `Refuse(reason, privilege, "no changes applied")` or an
   ordered `Plan` (`ClaimRole(target)` when the role is absent, ADR-017;
   `ApplySchema(pending versions)`; `SetAppRolePassword`). The
   shell executes the plan. Runtime SQLSTATE 42501 is still mapped, stating
   whether schema changes were applied; it is the backstop.
3. **Split capabilities (DDD-42).** `SchemaMigrator` (applies embedded SQL to
   head, detects dirty state) and `RolePasswordSetter` (accepts only a finished
   SCRAM verifier, touches only `ledgerops_app`'s login attribute and password)
   are separate driven ports; neither can do the other's job, and the password
   setter never sees a plaintext password.
4. **Contract shape of `PreflightProber`**: bounded-change with zero net change
   (write-capable statements, never committed). `FactReader` is bounded-change
   with an empty delta (read-only). The
   test takes a catalog snapshot before and after (`pg_authid`,
   `pg_auth_members`, `pg_default_acl`, `pg_namespace`, `pg_class`,
   `pg_database` ACL) and asserts it is byte-identical. OID and transaction-id
   consumption are not observable state and are excluded.
5. **Fault-injection tests**: a role with no `CREATEROLE`; no `ADMIN OPTION` on
   the existing role; not database owner; no `CREATE` on `public`; a table owned
   by another role; a second database in a cluster that already has
   `ledgerops_app`; two concurrent fresh migrates against different databases
   (exactly one wins, the other refuses, nothing dirty). Each must refuse naming the privilege and leave the old
   password working.

## Alternatives considered

- **Require superuser.** Simplest; excludes managed Postgres and violates least
  privilege.
- **Document only, no probe.** The failure then occurs mid-run after schema
  changes.
- **One rolled-back `ALTER ROLE` only** (the first revision). Proves one
  privilege; misses `GRANT CONNECT`, `CREATE` on `public`, and object ownership.
- **Catalog queries only, no empirical statements.** Cheap, but wrong for
  inherited roles and default privileges; empirical checks are the
  Earned-Trust requirement.
- **Fixed role name `ledgerops_migrate`.** Needless coupling.

## Consequences

- Acceptance gains non-superuser role fixtures (US-4 needs them anyway).
- The plan being pure makes every refusal reason a table-test, not an
  integration test.
- A probe can still mis-judge exotic grants; hence the mapped-error backstop.
