-- Migration 5: counterparty_aliases and transfer_state (US-5's cross-tenant
-- alias registry and the coordinator's own crash-recoverable progress
-- tracker, ADR-015/ADR-015-Amendment/ADR-015-Amendment-2). Both new tables
-- reference tenants, tenant_links (migration 0004), and accounts
-- (migration 0003) by foreign key -- no existing table's columns are
-- duplicated here.

-- counterparty_aliases: identity is the composite (tenant_id, alias), not a
-- synthetic id and not a bare alias string. Alias uniqueness is scoped to the
-- OWNING tenant's own namespace (tenant_id), never global -- two different
-- tenants may register the identical alias string with zero collision, which
-- is exactly US-5's own adversarial proof: a forged/cross-namespace alias
-- resolves to nothing outside its own tenant, because there is nothing in
-- this table's key space that would even let a lookup cross tenants by
-- accident. The PRIMARY KEY below IS that scoping, not merely a lookup
-- index on top of it.
--
-- tenant_link_id ties the registration back to the standing authorization
-- (tenant_links, migration 0004) it was made under -- the alias cannot
-- outlive proof of *why* the owning tenant was allowed to name this
-- particular counterparty account. target_tenant_id/target_account_id
-- together identify the counterparty's specific account; the composite FK
-- below reuses accounts' own composite primary key (tenant_id, id) from
-- migration 0003 rather than re-deriving a lookup path of its own.
--
-- This table is written once at registration and never mutated afterward --
-- no revoke/update operation exists for an alias in this feature's scope
-- (unlike tenant_links/transfer_state) -- so it stays standard append-only,
-- no UPDATE grant needed here.
--
-- Expand-only: a new table, no rewrite of existing history.
CREATE TABLE IF NOT EXISTS counterparty_aliases (
    tenant_id          text NOT NULL REFERENCES tenants (tenant_id),
    alias              text NOT NULL,
    tenant_link_id     text NOT NULL REFERENCES tenant_links (link_id),
    target_tenant_id   text NOT NULL REFERENCES tenants (tenant_id),
    target_account_id  text NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, alias),
    FOREIGN KEY (target_tenant_id, target_account_id) REFERENCES accounts (tenant_id, id)
);

GRANT SELECT, INSERT ON counterparty_aliases TO ledgerops_app;

-- transfer_state: one row per transfer_id -- the coordinator's own
-- read-model/progress-tracker, not a domain aggregate (ADR-014). This table
-- is read and written exclusively through app/ports.TransferStateRepository;
-- internal/domain never touches it.
--
-- UNIQUE(tenant_id, idempotency_key) is the transfer-level replay mechanism
-- -- distinct from the per-leg IdempotencyStore replay mechanism
-- (idempotency_keys, migration 0002), which guards individual leg postings,
-- not the coordinator's own multi-leg run.
--
-- status/leg1_status/leg2_status/leg3_status are plain text columns, not
-- CHECK-constrained enums like tenant_links.status in migration 0004 --
-- app.TransferStatus's full membership (including terminal-state variants)
-- is step 02-05/03-02's concern, not this migration's; locking a CHECK list
-- down here would make a later legitimate status addition a migration
-- rather than an application-layer change. reason is nullable text for a
-- terminal-state explanation (e.g. retry_budget_exhausted,
-- leg1_reversal_retry_budget_exhausted, leg2_reversal_retry_budget_exhausted)
-- -- absent while a transfer is still in flight.
--
-- leg{1,2,3}_attempts are explicit integer columns, one per leg, matching
-- this project's general preference for typed columns over a JSON/composite
-- blob for small structured state (no existing table in this schema uses
-- JSONB, so there is no precedent to follow instead) -- together they back
-- the fixed N=5 retry budget per leg.
--
-- next_attempt_at is NOT NULL with no DEFAULT -- ADR-015 Amendment requires
-- it set explicitly at row creation, never NULL. This is the crash-recovery
-- mechanism: a row is due-for-claim from the instant it is created, not only
-- once it reaches a 'retrying' label. Step 03-02's claim predicate --
-- status IN ('pending', 'retrying') AND next_attempt_at <= now() -- is
-- scoped by this column precisely because it is never NULL; a nullable
-- column would let a freshly created row silently escape that predicate
-- until something else set it.
--
-- Expand-only: a new table, no rewrite of existing history.
CREATE TABLE IF NOT EXISTS transfer_state (
    transfer_id      text PRIMARY KEY,
    tenant_id        text NOT NULL REFERENCES tenants (tenant_id),
    idempotency_key  text NOT NULL,
    status           text NOT NULL,
    leg1_status      text NOT NULL,
    leg2_status      text NOT NULL,
    leg3_status      text NOT NULL,
    leg1_attempts    integer NOT NULL DEFAULT 0,
    leg2_attempts    integer NOT NULL DEFAULT 0,
    leg3_attempts    integer NOT NULL DEFAULT 0,
    next_attempt_at  timestamptz NOT NULL,
    reason           text,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS transfer_state_tenant_idempotency_key_idx
    ON transfer_state (tenant_id, idempotency_key);

-- UPDATE is granted on transfer_state -- the second and last table in this
-- feature with this deliberate, documented divergence from OPS-10's
-- append-only posture (the first being tenant_links, migration 0004): the
-- coordinator mutates this row in place as legs progress (status,
-- per-leg status/attempts, next_attempt_at, reason) -- this is coordinator
-- progress state, not a ledger fact, and its current values ARE the thing
-- being modeled. No DELETE is granted -- a finished transfer's row is
-- retained, never removed.
GRANT SELECT, INSERT, UPDATE ON transfer_state TO ledgerops_app;
