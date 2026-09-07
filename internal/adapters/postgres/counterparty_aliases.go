package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// counterpartyAliasRepository is the real ports.CounterpartyAliasRepository,
// scoped to one unit of work's transaction — the same atomic
// check-then-create shape as tenantLinkRepository (tenant_links.go),
// mirrored deliberately one aggregate level over rather than given a new
// persistence idiom (DDD-26).
type counterpartyAliasRepository struct {
	tx pgx.Tx
}

var _ ports.CounterpartyAliasRepository = counterpartyAliasRepository{}

// Create persists a newly registered alias. Every field arrives already
// decided by domain.RegisterCounterpartyAlias; this adapter stores them
// as-is. The composite primary key (tenant_id, alias) (migration 02-01) is
// the final backstop under concurrency — the application-level courtesy
// check (RegisterCounterpartyAlias's own uniqueness read) is what normally
// intercepts a collision before a write is even attempted.
func (r counterpartyAliasRepository) Create(ctx context.Context, alias domain.CounterpartyAlias) error {
	_, err := r.tx.Exec(ctx,
		`INSERT INTO counterparty_aliases (tenant_id, alias, tenant_link_id, target_tenant_id, target_account_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		alias.TenantID(), alias.Alias(), alias.TenantLinkID(), alias.TargetTenantID(), alias.TargetAccountID())
	if err != nil {
		return fmt.Errorf("creating counterparty alias %q/%q: %w", alias.TenantID(), alias.Alias(), err)
	}
	return nil
}

// ByTenantAndAlias reads a registered alias within its owning tenant's own
// namespace, without locking. An absent pair answers (zero value, false,
// nil) — the expected shape of "no", mirroring
// TenantLinkRepository.ActiveByPair's own contract one aggregate over.
func (r counterpartyAliasRepository) ByTenantAndAlias(ctx context.Context, tenantID, alias string) (domain.CounterpartyAlias, bool, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT tenant_id, alias, tenant_link_id, target_tenant_id, target_account_id
		 FROM counterparty_aliases WHERE tenant_id = $1 AND alias = $2`,
		tenantID, alias)

	gotTenantID, gotAlias, tenantLinkID, targetTenantID, targetAccountID, err := scanCounterpartyAliasRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CounterpartyAlias{}, false, nil
	}
	if err != nil {
		return domain.CounterpartyAlias{}, false, fmt.Errorf("reading counterparty alias %q/%q: %w", tenantID, alias, err)
	}

	reconstructed, err := reconstructCounterpartyAlias(gotTenantID, gotAlias, tenantLinkID, targetTenantID, targetAccountID)
	if err != nil {
		return domain.CounterpartyAlias{}, false, err
	}
	return reconstructed, true, nil
}

// scanCounterpartyAliasRow is the shared column-scan behind
// ByTenantAndAlias — factored out the same way scanTenantLinkRow is in
// tenant_links.go, even though this port has only the one reader today,
// for consistency with that established shape.
func scanCounterpartyAliasRow(row pgx.Row) (tenantID, alias, tenantLinkID, targetTenantID, targetAccountID string, err error) {
	err = row.Scan(&tenantID, &alias, &tenantLinkID, &targetTenantID, &targetAccountID)
	return tenantID, alias, tenantLinkID, targetTenantID, targetAccountID, err
}

// reconstructCounterpartyAlias rebuilds a domain.CounterpartyAlias from a
// stored row. CounterpartyAlias has no general-purpose constructor — by
// design, the only way to produce one is the pure
// domain.RegisterCounterpartyAlias function itself — so rehydrating a
// persisted row reuses that function rather than reaching for an unexported
// field or a new constructor this step does not own (it must not touch
// internal/domain/counterparty_alias.go). RegisterCounterpartyAlias only
// inspects its tenantLinkSnapshot's Status() and LinkID() (never tenant_a/
// tenant_b), so a placeholder active TenantLink carrying the stored
// tenant_link_id — built through domain.AuthorizeTenantPair exactly the way
// tenant_links.go's own reconstructTenantLink builds its snapshots — is a
// pure, side-effect-free way to satisfy that constructor's "link found and
// active" precondition on read, without asserting anything about whether
// the link is still active *now* (ByTenantAndAlias's callers, e.g.
// ResolveCounterparty, perform their own fresh TenantLinkRepository.ByID
// read for that).
func reconstructCounterpartyAlias(tenantID, alias, tenantLinkID, targetTenantID, targetAccountID string) (domain.CounterpartyAlias, error) {
	placeholderLink, err := domain.AuthorizeTenantPair(tenantLinkID, "placeholder-a", "placeholder-b", nil, map[string]bool{
		"placeholder-a": true,
		"placeholder-b": true,
	})
	if err != nil {
		return domain.CounterpartyAlias{}, fmt.Errorf("reconstructing stored counterparty alias %q/%q: %w", tenantID, alias, err)
	}

	reconstructed, err := domain.RegisterCounterpartyAlias(tenantID, alias, placeholderLink, true, targetTenantID, targetAccountID)
	if err != nil {
		return domain.CounterpartyAlias{}, fmt.Errorf("reconstructing stored counterparty alias %q/%q: %w", tenantID, alias, err)
	}
	return reconstructed, nil
}
