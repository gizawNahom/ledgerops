package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
	"ledgerops/internal/support"
)

// tenantRepository is the real ports.TenantRepository, scoped to one unit of
// work's transaction — the same atomic check-then-create shape as
// accountRepository (accounts.go), mirrored deliberately one aggregate level
// up rather than given a new persistence idiom (DDD-26).
type tenantRepository struct {
	tx pgx.Tx
}

var _ ports.TenantRepository = tenantRepository{}

// ByName reads a tenant by its display name, without locking — used only for
// ProvisionTenant's I10 courtesy check ahead of the unique constraint on
// tenants.name (migration 0003) that is the final backstop under
// concurrency. An absent name answers domain.TenantNotFound, mirroring
// accountRepository.Get's UnknownAccount contract: the expected shape of
// "no", not an infrastructure error.
func (r tenantRepository) ByName(ctx context.Context, name string) (domain.Tenant, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT tenant_id, name FROM tenants WHERE name = $1`, name)

	var tenantID, gotName string
	err := row.Scan(&tenantID, &gotName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Tenant{}, domain.NewTenantNotFound(name)
	}
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("reading tenant named %q: %w", name, err)
	}

	// credential_hash is deliberately never scanned back into a domain.Tenant
	// here: the plaintext tenant_key cannot be recovered from its hash, and
	// ByName exists only for the existence check above — nothing downstream
	// inspects Credential() on this return value.
	tenant, err := domain.NewTenant(tenantID, gotName, "")
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("stored row for %q violates domain invariants: %w", name, err)
	}
	return tenant, nil
}

// ByID reads a tenant by its tenant_id, without locking — the existence
// check VerifyBooks (step 03-01) performs before running any trial-balance
// scan for a tenant-scoped call. Mirrors ByName's shape exactly (same table,
// same absent-row-is-not-an-infrastructure-error contract), keyed by the
// primary key instead of the unique name.
func (r tenantRepository) ByID(ctx context.Context, tenantID string) (domain.Tenant, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT tenant_id, name FROM tenants WHERE tenant_id = $1`, tenantID)

	var gotID, gotName string
	err := row.Scan(&gotID, &gotName)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Tenant{}, domain.NewTenantNotFound(tenantID)
	}
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("reading tenant %q: %w", tenantID, err)
	}

	// credential_hash is deliberately never scanned back here either — see
	// ByName's identical note above.
	tenant, err := domain.NewTenant(gotID, gotName, "")
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("stored row for %q violates domain invariants: %w", tenantID, err)
	}
	return tenant, nil
}

// Create persists a newly provisioned tenant. tenant.Credential() carries
// the plaintext tenant_key exactly as domain.ProvisionTenant produced it;
// this is the one place that plaintext's lifetime ends — only its SHA-256
// hash, hex-encoded, ever reaches the credential_hash column or a WHERE
// clause (brief.md § Multitenancy).
func (r tenantRepository) Create(ctx context.Context, tenant domain.Tenant) error {
	_, err := r.tx.Exec(ctx,
		`INSERT INTO tenants (tenant_id, name, credential_hash) VALUES ($1, $2, $3)`,
		tenant.TenantID(), tenant.Name(), hashCredential(tenant.Credential()))
	if err != nil {
		return fmt.Errorf("creating tenant %q: %w", tenant.TenantID(), err)
	}
	return nil
}

// hashCredential computes the tenant_key's SHA-256 digest, hex-encoded, via
// support.SHA256Hex — the same primitive internal/adapters/http/handlers.go
// uses for the idempotency key (DDD-26). The plaintext must never reach a
// WHERE clause or a log line — only this hash does.
func hashCredential(credential string) string {
	return support.SHA256Hex(credential)
}

// NewTenantKeyResolver returns a ports.TenantKeyResolver that resolves a
// presented bearer token's SHA-256 hash to the tenant_id it belongs to,
// querying tenants.credential_hash directly over appDSN and connecting fresh
// per call — matching this file's existing convention (store.go) for
// standalone helpers (AttemptOutOfBandChange, InterruptPostingMidWrite) that
// run outside any UnitOfWork. It runs ahead of any Ledger use case at the
// HTTP auth boundary (a later step, 02-03); this step only declares and
// implements the resolver function itself, not the middleware that will call
// it.
func NewTenantKeyResolver(appDSN string) ports.TenantKeyResolver {
	return func(ctx context.Context, credentialHash string) (string, bool, error) {
		conn, err := pgx.Connect(ctx, appDSN)
		if err != nil {
			return "", false, fmt.Errorf("connecting to resolve a tenant key: %w", err)
		}
		defer conn.Close(context.Background())

		var tenantID string
		err = conn.QueryRow(ctx,
			`SELECT tenant_id FROM tenants WHERE credential_hash = $1`, credentialHash,
		).Scan(&tenantID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("resolving a tenant key: %w", err)
		}
		return tenantID, true, nil
	}
}
