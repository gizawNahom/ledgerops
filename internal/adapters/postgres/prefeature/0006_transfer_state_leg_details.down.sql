-- Reverses migration 6.
ALTER TABLE transfer_state
    DROP COLUMN IF EXISTS counterparty_tenant_id,
    DROP COLUMN IF EXISTS target_account_id,
    DROP COLUMN IF EXISTS amount_minor,
    DROP COLUMN IF EXISTS currency;
