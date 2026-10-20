-- Reverses migration 0. Local/dev use only -- there is no rollback path once
-- history exists in production (D7); this file exists so migration tooling
-- has a down step to pair with the up step, not because it is ever expected
-- to run over populated history.

REVOKE SELECT, INSERT, UPDATE ON accounts FROM ledgerops_app;
REVOKE SELECT, INSERT ON transactions FROM ledgerops_app;
REVOKE SELECT, INSERT ON entries FROM ledgerops_app;
REVOKE USAGE ON SCHEMA public FROM ledgerops_app;
REVOKE CONNECT ON DATABASE ledgerops FROM ledgerops_app;

DROP TRIGGER IF EXISTS entries_append_only ON entries;
DROP FUNCTION IF EXISTS entries_are_append_only();

DROP TABLE IF EXISTS entries;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS accounts;
