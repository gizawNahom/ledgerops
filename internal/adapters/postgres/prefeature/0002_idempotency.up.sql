-- Migration 1: idempotency_keys.
--
-- DDD-8: a claim is the key, the request fingerprint, and the transaction id
-- it produced. There is no cached response body -- a replay re-renders from
-- the stored transaction (ADR-005), so a later change to response shape
-- cannot leave old replays serving a stale shape. The primary key on `key`
-- is what makes I7 hold under concurrency: two concurrent claims for the same
-- key can only ever have one winner, enforced by Postgres, not by the
-- application.
--
-- Expand-only, consistent with migration 0: a new table, no rewrite of
-- existing history.
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key            text PRIMARY KEY,
    fingerprint    text NOT NULL,
    transaction_id text NOT NULL REFERENCES transactions (id),
    created_at     timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT ON idempotency_keys TO ledgerops_app;
