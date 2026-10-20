-- Migration 0: schema, roles, append-only protection, unique account names.
--
-- Two roles, not one (OPS-10): ledgerops_migrate holds DDL and is the only
-- role that runs this file and every later one; ledgerops_app is what the
-- running service and every test connect as. D7 (append-only) is enforced
-- twice over on entries -- a trigger alone is not database-level enforcement,
-- because whatever role can ALTER TABLE ... DISABLE TRIGGER can still rewrite
-- history. The revoked privilege on the role the service actually uses is the
-- load-bearing control; the trigger is the second, independent layer.

CREATE TABLE IF NOT EXISTS accounts (
    -- id is the account name (DDD-18). Its uniqueness comes from this primary
    -- key, created with the table in this same statement -- migrations are
    -- expand-only, so adding the constraint later against history already
    -- carrying duplicates would fail.
    id            text PRIMARY KEY,
    kind          text NOT NULL CHECK (kind IN ('wallet', 'system')),
    balance_minor bigint NOT NULL,
    currency      text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS transactions (
    id          text PRIMARY KEY,
    recorded_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS entries (
    transaction_id  text NOT NULL REFERENCES transactions (id),
    account_id      text NOT NULL REFERENCES accounts (id),
    counterparty_id text NOT NULL REFERENCES accounts (id),
    amount_minor    bigint NOT NULL,
    currency        text NOT NULL,
    recorded_at     timestamptz NOT NULL,
    sequence        bigint NOT NULL,
    PRIMARY KEY (transaction_id, sequence)
);

CREATE INDEX IF NOT EXISTS entries_account_id_idx ON entries (account_id);

-- Append-only, layer one of two: a trigger that refuses UPDATE and DELETE
-- regardless of who holds it, so the schema documents the rule even for a
-- role broad enough to bypass the grant below.
CREATE OR REPLACE FUNCTION entries_are_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'entries are append-only (D7): % on entries is not permitted', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS entries_append_only ON entries;
CREATE TRIGGER entries_append_only
    BEFORE UPDATE OR DELETE ON entries
    FOR EACH ROW
    EXECUTE FUNCTION entries_are_append_only();

-- Roles. ledgerops_migrate is provisioned by the deployment already (it is
-- the connecting user this migration runs as); ledgerops_app is created here,
-- idempotently, since nothing else provisions it.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'ledgerops_app') THEN
        CREATE ROLE ledgerops_app LOGIN PASSWORD 'app-secret';
    END IF;
END
$$;

GRANT CONNECT ON DATABASE ledgerops TO ledgerops_app;
GRANT USAGE ON SCHEMA public TO ledgerops_app;

-- Append-only, layer two of two (OPS-10): the service's own credentials hold
-- only SELECT and INSERT on entries. UPDATE and DELETE are never granted, so
-- even a role broad enough to disable the trigger above still cannot rewrite
-- history without also holding a privilege it was never given.
GRANT SELECT, INSERT ON entries TO ledgerops_app;
REVOKE UPDATE, DELETE ON entries FROM ledgerops_app;

-- Accounts and transactions are not append-only: postings update stored
-- balances (ADR-003) and both tables gain new rows, but neither carries D7's
-- guarantee.
GRANT SELECT, INSERT, UPDATE ON accounts TO ledgerops_app;
GRANT SELECT, INSERT ON transactions TO ledgerops_app;
