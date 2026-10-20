-- Reverses migration 1. Local/dev use only, per the same rationale as
-- migration 0's down step.

REVOKE SELECT, INSERT ON idempotency_keys FROM ledgerops_app;
DROP TABLE IF EXISTS idempotency_keys;
