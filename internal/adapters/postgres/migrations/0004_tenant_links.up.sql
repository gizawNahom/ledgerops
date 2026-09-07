-- Migration 4: tenant_links (standing authorization between two tenants for
-- inter-tenant transfer, ADR-015/ADR-016) and the reserved tnt_platform
-- tenant that every cross-tenant transfer's Leg 2 posts under.
--
-- tenant_a/tenant_b hold a canonicalized (sorted) pair -- the domain layer
-- (TenantLink aggregate, step 01-02) sorts the pair before ever calling this
-- table's adapter, mirroring how tenants.go's Create/ByName/ByID never
-- transform their inputs themselves; this table stays a thin projection of
-- whatever the domain already validated and ordered. Canonicalizing at write
-- time is what makes AuthorizeTenantPair(B, A) collide with an existing
-- AuthorizeTenantPair(A, B) on the very same unique constraint below.
--
-- UNIQUE(tenant_a, tenant_b) applies only to active links, via a partial
-- unique index rather than a plain table constraint: a revoked link's slot
-- must stay reusable by a fresh AuthorizeTenantPair call that mints a new
-- link_id, so a revoked row and a new active row for the same pair need to
-- coexist as distinct rows. A plain UNIQUE constraint cannot express that;
-- WHERE status = 'active' can.
CREATE TABLE IF NOT EXISTS tenant_links (
    link_id   text PRIMARY KEY,
    tenant_a  text NOT NULL REFERENCES tenants (tenant_id),
    tenant_b  text NOT NULL REFERENCES tenants (tenant_id),
    status    text NOT NULL CHECK (status IN ('active', 'revoked'))
);

CREATE UNIQUE INDEX IF NOT EXISTS tenant_links_active_pair_idx
    ON tenant_links (tenant_a, tenant_b)
    WHERE status = 'active';

-- UPDATE is granted on tenant_links only -- a deliberate, documented
-- divergence from OPS-10's append-only posture (entries/transactions never
-- grant UPDATE/DELETE, migration 0001). Revoking a link is an in-place
-- status mutation (active -> revoked) by design, not a history rewrite: a
-- tenant_link is a standing-authorization record, not a ledger fact, and its
-- current status IS the thing being modeled. No DELETE is granted -- a
-- revoked row is retained, never removed, which is what leaves the partial
-- unique index above free to admit a fresh active row for the same pair
-- without destroying the audit trail of the prior authorization.
GRANT SELECT, INSERT, UPDATE ON tenant_links TO ledgerops_app;

-- tnt_platform: the reserved internal tenant identity Leg 2 of every
-- cross-tenant transfer posts under (its own settlement mirror accounts,
-- never either business tenant's raw tenant_id) -- keeping every leg
-- strictly intra-tenant per I8 (internal/domain/post.go:42-53, unmodified).
-- Seeded once, idempotently, mirroring tnt_legacy_seed's own seeding
-- precedent in migration 0003 -- ON CONFLICT DO NOTHING makes re-running
-- this migration (or a future migration re-seeding the same row) a no-op
-- rather than a unique-violation failure. credential_hash is a deterministic
-- placeholder, not a real credential, same rationale as tnt_legacy_seed's:
-- tnt_platform never authenticates an inbound request, it only receives
-- posts from the service's own internal transfer workflow.
INSERT INTO tenants (tenant_id, name, credential_hash)
VALUES ('tnt_platform', 'Platform Settlement Tenant', 'tnt_platform-placeholder-credential-hash')
ON CONFLICT (tenant_id) DO NOTHING;
