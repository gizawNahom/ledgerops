-- Migration 3: tenants, and the cascading correction multitenancy forces on
-- accounts/entries/transactions (ADR-013, DDD-24).
--
-- accounts.id was a bare, globally-unique PRIMARY KEY. US-2 requires two
-- different tenants to independently open an account named "wallet-1", which
-- a global unique constraint on id alone structurally forbids. So accounts'
-- PRIMARY KEY becomes composite (tenant_id, id) -- not avoidable, not
-- deferrable. Once accounts.id is no longer independently unique, Postgres
-- requires entries.account_id/counterparty_id (both REFERENCES accounts(id))
-- to widen to composite FKs too, since an FK target must be a unique/PK
-- column set. That is the cascade ADR-013 names as not visible from the
-- SPIKE question alone.
--
-- Every new tenant_id column here -- on accounts, entries, and transactions
-- alike -- carries DEFAULT 'tnt_legacy_seed', not merely NOT NULL. ADR-013
-- names this explicitly for accounts: the DEFAULT is what keeps a
-- hypothetical still-running pre-multitenancy binary's INSERT (which never
-- mentions tenant_id) succeeding against the new schema. The same reasoning
-- applies unchanged to entries and transactions: their own INSERT statements
-- (TransactionRepository.Append, InterruptPostingMidWrite) are not updated by
-- this step -- that is step 01-02's cascading SQL update, listed as a
-- consequence in ADR-013, not this one -- so those INSERTs still omit
-- tenant_id today. Without a DEFAULT here too, every already-shipped posting
-- path would start failing a NOT NULL constraint the moment this migration
-- applies, which the expand-only discipline (brief.md § Deployment shape:
-- "every schema change must leave the previous binary able to run against
-- the new schema") forbids just as much for entries/transactions as for
-- accounts.
--
-- Expand-only, checked against its literal text: no table is dropped, no
-- row's content is deleted or rewritten -- ADD COLUMN ... DEFAULT backfills
-- every pre-existing row to the sentinel value as part of adding the column,
-- which is what satisfies "backfill every pre-existing accounts row to
-- tnt_legacy_seed before the composite PK constraint is applied" without a
-- separate UPDATE statement. Two FK constraints are widened from a stricter
-- (global) to a looser (tenant-scoped) shape; nothing is narrowed.

CREATE TABLE IF NOT EXISTS tenants (
    tenant_id      text PRIMARY KEY,
    name           text UNIQUE NOT NULL,
    credential_hash text UNIQUE NOT NULL
);

-- No UPDATE/DELETE grant, mirroring OPS-10's least-privilege posture -- a
-- free consequence of rename/rotation/offboarding being out of scope
-- (ADR-013 §1).
GRANT SELECT, INSERT ON tenants TO ledgerops_app;

-- The sentinel tenant every pre-existing row backfills to. credential_hash
-- here is a deterministic placeholder, not a real credential -- step 02-05
-- wires the actual seeded demo credential at composition-root startup; this
-- migration only needs the tenant row and the sentinel id to exist so the
-- FK/backfill below has somewhere to point.
INSERT INTO tenants (tenant_id, name, credential_hash)
VALUES ('tnt_legacy_seed', 'Legacy Seed Tenant', 'tnt_legacy_seed-placeholder-credential-hash');

-- accounts: add tenant_id (backfilling every existing row to the sentinel via
-- DEFAULT), then replace the bare PRIMARY KEY with a composite one. The old
-- FKs from entries must be dropped first -- Postgres will not let a PK
-- referenced by an inbound FK be dropped otherwise.
ALTER TABLE accounts
    ADD COLUMN tenant_id text NOT NULL DEFAULT 'tnt_legacy_seed' REFERENCES tenants (tenant_id);

ALTER TABLE entries
    ADD COLUMN tenant_id text NOT NULL DEFAULT 'tnt_legacy_seed' REFERENCES tenants (tenant_id);

ALTER TABLE transactions
    ADD COLUMN tenant_id text NOT NULL DEFAULT 'tnt_legacy_seed' REFERENCES tenants (tenant_id);

ALTER TABLE entries DROP CONSTRAINT entries_account_id_fkey;
ALTER TABLE entries DROP CONSTRAINT entries_counterparty_id_fkey;

ALTER TABLE accounts DROP CONSTRAINT accounts_pkey;
ALTER TABLE accounts ADD CONSTRAINT accounts_pkey PRIMARY KEY (tenant_id, id);

ALTER TABLE entries
    ADD CONSTRAINT entries_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES accounts (tenant_id, id);
ALTER TABLE entries
    ADD CONSTRAINT entries_counterparty_id_fkey
    FOREIGN KEY (tenant_id, counterparty_id) REFERENCES accounts (tenant_id, id);
