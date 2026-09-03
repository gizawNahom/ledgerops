// Adapter integration test for the driven postgres TenantRepository, real
// PostgreSQL 16 via Testcontainers (OPS-11, Mandate 6). A wiring test, not a
// property test, per the layered test discipline (nw-tdd-methodology §
// Layered test discipline): "Integration | UNCHANGED — single-example test
// verifies WIRING."
package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/domain"
)

func TestTenantRepository_CreateThenByName_RoundTripsThroughRealPostgres(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	tenant, err := domain.NewTenant("tnt_test-1", "acme", "tk_plaintext-not-stored")
	if err != nil {
		t.Fatalf("NewTenant: %v", err)
	}

	writeUOW := beginUOW(t, store)
	if err := writeUOW.Tenants().Create(ctx, tenant); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	got, err := readUOW.Tenants().ByName(ctx, "acme")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ByName: %v", err)
	}

	if got.TenantID() != "tnt_test-1" {
		t.Fatalf("TenantID() = %q, want %q", got.TenantID(), "tnt_test-1")
	}
	if got.Name() != "acme" {
		t.Fatalf("Name() = %q, want %q", got.Name(), "acme")
	}
}

func TestTenantRepository_ByName_UnknownNameIsRefused(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	_, err := uow.Tenants().ByName(ctx, "never-provisioned")
	_ = uow.Rollback(ctx)

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.TenantNotFound {
		t.Fatalf("expected tenant_not_found, got %v", err)
	}
}

// TestTenantRepository_Create_NeverStoresThePlaintextCredential covers the
// credential-format acceptance criterion (brief.md § Multitenancy): only
// sha256(tenant_key) hex-encoded ever reaches storage. Queried directly via a
// raw connection because ports.TenantRepository exposes no credential_hash
// read at all — by design, this hash is only ever compared, never returned
// through the domain-facing port.
func TestTenantRepository_Create_NeverStoresThePlaintextCredential(t *testing.T) {
	store, appDSN := migratedStoreWithDSN(t)
	ctx := context.Background()

	const plaintextKey = "tk_super-secret-plaintext"
	tenant, err := domain.NewTenant("tnt_test-2", "wayne-enterprises", plaintextKey)
	if err != nil {
		t.Fatalf("NewTenant: %v", err)
	}

	uow := beginUOW(t, store)
	if err := uow.Tenants().Create(ctx, tenant); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := uow.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	wantHash := hashCredentialForTest(plaintextKey)
	gotHash := storedCredentialHash(t, appDSN, "tnt_test-2")
	if gotHash != wantHash {
		t.Fatalf("stored credential_hash = %q, want sha256(tenant_key) = %q", gotHash, wantHash)
	}
	if gotHash == plaintextKey {
		t.Fatalf("credential_hash equals the plaintext key — it must never be stored unhashed")
	}
}

// hashCredentialForTest mirrors tenants.go's unexported hashCredential — this
// file lives in package postgres_test (external test package, matching this
// package's convention), so it cannot call the unexported production helper
// directly; duplicating the two-line digest here is cheaper than exporting a
// hash function the domain/production code has no other reason to expose.
func hashCredentialForTest(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}

// storedCredentialHash reads credential_hash directly — no driven port
// exposes it, deliberately (ports.TenantRepository never returns a hash to
// the domain-facing side), so this test reaches past the port with its own
// connection, exactly like the other raw-SQL assertions in this package
// (e.g. TestMigration_TenantSchema).
func storedCredentialHash(t *testing.T, appDSN, tenantID string) string {
	t.Helper()
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, appDSN)
	if err != nil {
		t.Fatalf("connecting to read credential_hash: %v", err)
	}
	defer conn.Close(context.Background())

	var hash string
	if err := conn.QueryRow(ctx,
		`SELECT credential_hash FROM tenants WHERE tenant_id = $1`, tenantID,
	).Scan(&hash); err != nil {
		t.Fatalf("reading credential_hash for %q: %v", tenantID, err)
	}
	return hash
}
