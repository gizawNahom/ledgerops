-- Reverses migration 3. Local/dev use only, per the same rationale as
-- migrations 0 and 1's down steps -- there is no rollback path once history
-- exists in production (D7); this exists so migration tooling has a down
-- step to pair with the up step.

ALTER TABLE entries DROP CONSTRAINT entries_account_id_fkey;
ALTER TABLE entries DROP CONSTRAINT entries_counterparty_id_fkey;

ALTER TABLE accounts DROP CONSTRAINT accounts_pkey;
ALTER TABLE accounts ADD CONSTRAINT accounts_pkey PRIMARY KEY (id);

ALTER TABLE entries
    ADD CONSTRAINT entries_account_id_fkey
    FOREIGN KEY (account_id) REFERENCES accounts (id);
ALTER TABLE entries
    ADD CONSTRAINT entries_counterparty_id_fkey
    FOREIGN KEY (counterparty_id) REFERENCES accounts (id);

ALTER TABLE accounts DROP COLUMN tenant_id;
ALTER TABLE entries DROP COLUMN tenant_id;
ALTER TABLE transactions DROP COLUMN tenant_id;

REVOKE SELECT, INSERT ON tenants FROM ledgerops_app;
DROP TABLE IF EXISTS tenants;
