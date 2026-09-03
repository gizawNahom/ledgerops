package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
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

// hashCredential computes the tenant_key's SHA-256 digest, hex-encoded — the
// same digest/encoding shape internal/adapters/http/handlers.go already uses
// for the idempotency key (hashIdempotencyKey), the same approach reused here
// rather than reinvented (DDD-26). Pure function: input in, digest out, no
// side effects. The plaintext must never reach a WHERE clause or a log line
// — only this hash does.
func hashCredential(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
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
