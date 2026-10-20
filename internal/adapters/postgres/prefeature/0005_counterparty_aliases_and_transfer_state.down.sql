-- Reverses migration 5. Local/dev use only, per the same rationale as every
-- other migration's down step -- there is no rollback path once history
-- exists in production (D7); this exists so migration tooling has a down
-- step to pair with the up step.

REVOKE SELECT, INSERT, UPDATE ON transfer_state FROM ledgerops_app;
DROP INDEX IF EXISTS transfer_state_tenant_idempotency_key_idx;
DROP TABLE IF EXISTS transfer_state;

REVOKE SELECT, INSERT ON counterparty_aliases FROM ledgerops_app;
DROP TABLE IF EXISTS counterparty_aliases;
