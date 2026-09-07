-- Migration 6: transfer_state gains the columns attemptLeg needs to be a
-- self-contained, resumable function (step 02-05, TransferCoordinator).
--
-- Migration 0005 already carries every column SendTransfer needs to
-- describe the SENDER side of a transfer (tenant_id, idempotency_key), but
-- attemptLeg -- called later, by the ticker (step 03-02), with nothing but a
-- transfer_id in hand -- also needs the RECEIVING side and the movement
-- amount, or it cannot rediscover which accounts to move value between
-- without a second, out-of-band lookup. Expand-only: new columns on an
-- existing table, no rewrite of existing history (this table has none yet
-- in production -- inter-tenant-transfer has not shipped).
--
-- counterparty_tenant_id: the receiving tenant (Leg 2's "To" side, Leg 3's
-- own tenant scope) -- brief.md § Inter-tenant transfer / Account bootstrap.
-- target_account_id: the receiver's own real account (Leg 3's "To" side) --
-- resolved once, by domain.ResolveCounterparty, inside SendTransfer, and
-- persisted here so attemptLeg never re-resolves it.
-- amount_minor/currency: the one movement amount shared by every leg
-- (I1 requires each leg's own entries to sum to zero, but the SAME amount
-- moves at every leg -- brief.md's own three-leg mapping) -- stored the same
-- shape accounts.balance_minor/currency already uses (migration 0003), no
-- new representation invented.
ALTER TABLE transfer_state
    ADD COLUMN IF NOT EXISTS counterparty_tenant_id text REFERENCES tenants (tenant_id),
    ADD COLUMN IF NOT EXISTS target_account_id      text,
    ADD COLUMN IF NOT EXISTS amount_minor           bigint,
    ADD COLUMN IF NOT EXISTS currency               text;
