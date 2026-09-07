-- Reverses migration 4. Local/dev use only, per the same rationale as every
-- other migration's down step -- there is no rollback path once history
-- exists in production (D7); this exists so migration tooling has a down
-- step to pair with the up step.

REVOKE SELECT, INSERT, UPDATE ON tenant_links FROM ledgerops_app;
DROP INDEX IF EXISTS tenant_links_active_pair_idx;
DROP TABLE IF EXISTS tenant_links;

DELETE FROM tenants WHERE tenant_id = 'tnt_platform';
