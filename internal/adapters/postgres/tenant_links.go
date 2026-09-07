package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// tenantLinkRepository is the real ports.TenantLinkRepository, scoped to one
// unit of work's transaction — the same atomic check-then-create shape as
// tenantRepository (tenants.go), mirrored deliberately one aggregate level
// over rather than given a new persistence idiom (DDD-26).
type tenantLinkRepository struct {
	tx pgx.Tx
}

var _ ports.TenantLinkRepository = tenantLinkRepository{}

// Create persists a newly authorized link. tenant_a/tenant_b arrive already
// canonicalized by domain.AuthorizeTenantPair; this adapter stores them
// as-is. The partial unique index on (tenant_a, tenant_b) WHERE
// status = 'active' (migration 0004) is the final backstop under
// concurrency — a duplicate-active-pair insert surfaces here as a wrapped
// unique_violation (pgconn.PgError, SQLState 23505), not a domain violation:
// the application-level courtesy check (AuthorizeTenantPair's
// tenant_link_already_exists refusal) is what normally intercepts this
// before a write is even attempted.
func (r tenantLinkRepository) Create(ctx context.Context, link domain.TenantLink) error {
	_, err := r.tx.Exec(ctx,
		`INSERT INTO tenant_links (link_id, tenant_a, tenant_b, status) VALUES ($1, $2, $3, $4)`,
		link.LinkID(), link.TenantA(), link.TenantB(), string(link.Status()))
	if err != nil {
		return fmt.Errorf("creating tenant link %q: %w", link.LinkID(), err)
	}
	return nil
}

// ActiveByPair reads the active link, if any, for an unordered tenant pair.
// Stored rows are already canonicalized (tenant_a < tenant_b) by the pure
// domain constructor that wrote them, but this read checks both directions
// so a caller need not canonicalize before asking — mirrors
// IdempotencyStore.Lookup's (value, found, error) shape: an absent active
// link is the expected shape of "no", not an error.
func (r tenantLinkRepository) ActiveByPair(ctx context.Context, tenantA, tenantB string) (domain.TenantLink, bool, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT link_id, tenant_a, tenant_b, status FROM tenant_links
		 WHERE status = 'active' AND ((tenant_a = $1 AND tenant_b = $2) OR (tenant_a = $2 AND tenant_b = $1))`,
		tenantA, tenantB)

	linkID, gotA, gotB, status, err := scanTenantLinkRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TenantLink{}, false, nil
	}
	if err != nil {
		return domain.TenantLink{}, false, fmt.Errorf("reading the active link for %q/%q: %w", tenantA, tenantB, err)
	}

	link, err := reconstructTenantLink(linkID, gotA, gotB, status)
	if err != nil {
		return domain.TenantLink{}, false, err
	}
	return link, true, nil
}

// ByID reads a link by its link_id, without locking — the read
// RevokeTenantLink's use case performs before calling the pure
// domain.RevokeTenantLink decision. An absent id answers
// domain.NewTenantLinkNotFound, mirroring tenantRepository.ByID's identical
// absent-row contract one aggregate over.
func (r tenantLinkRepository) ByID(ctx context.Context, linkID string) (domain.TenantLink, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT link_id, tenant_a, tenant_b, status FROM tenant_links WHERE link_id = $1`, linkID)

	gotID, gotA, gotB, status, err := scanTenantLinkRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TenantLink{}, domain.NewTenantLinkNotFound()
	}
	if err != nil {
		return domain.TenantLink{}, fmt.Errorf("reading tenant link %q: %w", linkID, err)
	}

	return reconstructTenantLink(gotID, gotA, gotB, status)
}

// Revoke transitions the named link's status to revoked in place — the one
// deliberate in-place mutation this schema grants (migration 0004), unlike
// the append-only ledger tables. Naming an id absent from the table answers
// domain.NewTenantLinkNotFound, the same expected shape of "no" ByID uses.
func (r tenantLinkRepository) Revoke(ctx context.Context, linkID string) error {
	tag, err := r.tx.Exec(ctx,
		`UPDATE tenant_links SET status = 'revoked' WHERE link_id = $1`, linkID)
	if err != nil {
		return fmt.Errorf("revoking tenant link %q: %w", linkID, err)
	}
	if tag.RowsAffected() == 0 {
		return domain.NewTenantLinkNotFound()
	}
	return nil
}

// scanTenantLinkRow is the shared column-scan behind ActiveByPair and ByID —
// both select the identical four columns in the identical order, so the scan
// itself is factored out rather than repeated.
func scanTenantLinkRow(row pgx.Row) (linkID, tenantA, tenantB, status string, err error) {
	err = row.Scan(&linkID, &tenantA, &tenantB, &status)
	return linkID, tenantA, tenantB, status, err
}

// reconstructTenantLink rebuilds a domain.TenantLink from a stored row.
// TenantLink has no general-purpose constructor — by design (I11), the only
// ways to produce one are the pure domain.AuthorizeTenantPair and
// domain.RevokeTenantLink functions themselves — so rehydrating a persisted
// row reuses those same two functions rather than reaching for an
// unexported field or a new constructor this step does not own (it must not
// touch internal/domain/tenant_link.go). domain.AuthorizeTenantPair applied
// to an already-canonicalized pair with both ids present in knownTenants is
// a pure, side-effect-free reconstruction of the stored {link_id, tenant_a,
// tenant_b, status: active} triple; domain.RevokeTenantLink then folds in a
// revoked status when that is what the row actually holds.
func reconstructTenantLink(linkID, tenantA, tenantB, status string) (domain.TenantLink, error) {
	active, err := domain.AuthorizeTenantPair(linkID, tenantA, tenantB, nil, map[string]bool{tenantA: true, tenantB: true})
	if err != nil {
		return domain.TenantLink{}, fmt.Errorf("reconstructing stored tenant link %q: %w", linkID, err)
	}
	if status == string(domain.TenantLinkRevoked) {
		return domain.RevokeTenantLink(linkID, []domain.TenantLink{active})
	}
	return active, nil
}
