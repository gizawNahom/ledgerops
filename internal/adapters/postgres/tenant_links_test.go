// Adapter integration test for the driven postgres TenantLinkRepository,
// real PostgreSQL 16 via Testcontainers (OPS-11, Mandate 6). A wiring test,
// not a property test, per the layered test discipline
// (nw-tdd-methodology § Layered test discipline): "Integration | UNCHANGED —
// single-example test verifies WIRING."
//
// This step (01-03) has no pre-authored acceptance test reaching this
// layer — HTTP wiring lands at step 01-04 — so this adapter test IS this
// step's own RED/GREEN obligation, mirroring how tenant_link_test.go's PBT
// unit tests were the RED/GREEN obligation for step 01-02's pure domain
// core.
package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"ledgerops/internal/domain"
)

// TestTenantLinkRepository_CreateThenByID_RoundTripsThroughRealPostgres
// covers the wiring itself: a link authorized via the pure
// domain.AuthorizeTenantPair constructor, persisted through Create, reads
// back byte-identical through ByID.
func TestTenantLinkRepository_CreateThenByID_RoundTripsThroughRealPostgres(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	provisionTenant(t, store, "tnt_acme", "acme")
	provisionTenant(t, store, "tnt_beacon", "beacon")

	link, err := domain.AuthorizeTenantPair("lnk_test-1", "tnt_acme", "tnt_beacon", nil, map[string]bool{
		"tnt_acme":   true,
		"tnt_beacon": true,
	})
	if err != nil {
		t.Fatalf("AuthorizeTenantPair: %v", err)
	}

	writeUOW := beginUOW(t, store)
	if err := writeUOW.TenantLinks().Create(ctx, link); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	got, err := readUOW.TenantLinks().ByID(ctx, "lnk_test-1")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}

	if got != link {
		t.Fatalf("ByID() = %+v, want %+v", got, link)
	}
}

// TestTenantLinkRepository_ByID_UnknownIDIsRefused mirrors
// TestTenantRepository_ByName_UnknownNameIsRefused's contract one aggregate
// over: an absent link_id answers domain.TenantLinkNotFound, the expected
// shape of "no", not an infrastructure error.
func TestTenantLinkRepository_ByID_UnknownIDIsRefused(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	_, err := uow.TenantLinks().ByID(ctx, "lnk_never-authorized")
	_ = uow.Rollback(ctx)

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.TenantLinkNotFound {
		t.Fatalf("expected tenant_link_not_found, got %v", err)
	}
}

// TestTenantLinkRepository_ActiveByPair_FindsBothArgumentOrders proves
// ActiveByPair's pair-direction tolerance against real Postgres: a stored,
// already-canonicalized row is found whichever order the caller names the
// pair in.
func TestTenantLinkRepository_ActiveByPair_FindsBothArgumentOrders(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	provisionTenant(t, store, "tnt_acme", "acme")
	provisionTenant(t, store, "tnt_beacon", "beacon")

	link, err := domain.AuthorizeTenantPair("lnk_test-2", "tnt_beacon", "tnt_acme", nil, map[string]bool{
		"tnt_acme":   true,
		"tnt_beacon": true,
	})
	if err != nil {
		t.Fatalf("AuthorizeTenantPair: %v", err)
	}

	writeUOW := beginUOW(t, store)
	if err := writeUOW.TenantLinks().Create(ctx, link); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	forwardUOW := beginUOW(t, store)
	forward, found, err := forwardUOW.TenantLinks().ActiveByPair(ctx, "tnt_acme", "tnt_beacon")
	_ = forwardUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ActiveByPair (forward): %v", err)
	}
	if !found || forward != link {
		t.Fatalf("ActiveByPair(acme, beacon) = %+v, %v, want %+v, true", forward, found, link)
	}

	reversedUOW := beginUOW(t, store)
	reversed, found, err := reversedUOW.TenantLinks().ActiveByPair(ctx, "tnt_beacon", "tnt_acme")
	_ = reversedUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ActiveByPair (reversed): %v", err)
	}
	if !found || reversed != link {
		t.Fatalf("ActiveByPair(beacon, acme) = %+v, %v, want %+v, true", reversed, found, link)
	}
}

// TestTenantLinkRepository_PartialUniqueIndex_RejectsDuplicateActivePairThenAdmitsAfterRevoke
// is this step's Earned Trust obligation: it empirically proves the partial
// unique index from migration 0004 (UNIQUE(tenant_a, tenant_b) WHERE
// status = 'active') is enforced by the database itself, not merely assumed
// from reading the SQL, and that it correctly excludes revoked rows.
//
// Both Create calls below go straight through the adapter, deliberately
// bypassing AuthorizeTenantPair's application-level courtesy check (the read
// of ActiveByPair before deciding), so the second insert's rejection can only
// be coming from the database constraint itself.
func TestTenantLinkRepository_PartialUniqueIndex_RejectsDuplicateActivePairThenAdmitsAfterRevoke(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	provisionTenant(t, store, "tnt_acme", "acme")
	provisionTenant(t, store, "tnt_beacon", "beacon")

	knownTenants := map[string]bool{"tnt_acme": true, "tnt_beacon": true}

	first, err := domain.AuthorizeTenantPair("lnk_first", "tnt_acme", "tnt_beacon", nil, knownTenants)
	if err != nil {
		t.Fatalf("AuthorizeTenantPair (first): %v", err)
	}

	firstUOW := beginUOW(t, store)
	if err := firstUOW.TenantLinks().Create(ctx, first); err != nil {
		t.Fatalf("Create (first): %v", err)
	}
	if err := firstUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (first): %v", err)
	}

	second, err := domain.AuthorizeTenantPair("lnk_second", "tnt_acme", "tnt_beacon", nil, knownTenants)
	if err != nil {
		t.Fatalf("AuthorizeTenantPair (second): %v", err)
	}

	// Attempt a second active row for the identical canonicalized pair while
	// the first is still active. The application-level courtesy check is
	// bypassed entirely here -- this call goes straight at the constraint.
	duplicateUOW := beginUOW(t, store)
	err = duplicateUOW.TenantLinks().Create(ctx, second)
	_ = duplicateUOW.Rollback(ctx)

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected a *pgconn.PgError from the partial unique index, got %v (%T)", err, err)
	}
	const uniqueViolation = "23505"
	if pgErr.Code != uniqueViolation {
		t.Fatalf("PgError.Code = %q, want %q (unique_violation)", pgErr.Code, uniqueViolation)
	}
	if pgErr.ConstraintName != "tenant_links_active_pair_idx" {
		t.Fatalf("PgError.ConstraintName = %q, want %q -- the partial index itself must be what rejected this, not some other constraint", pgErr.ConstraintName, "tenant_links_active_pair_idx")
	}

	// Revoke the first link: its slot in the partial index must now be free.
	revokeUOW := beginUOW(t, store)
	if err := revokeUOW.TenantLinks().Revoke(ctx, first.LinkID()); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if err := revokeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (revoke): %v", err)
	}

	// A fresh Create for the identical pair must now succeed -- the partial
	// index excludes revoked rows, it does not block the pair forever.
	admitUOW := beginUOW(t, store)
	if err := admitUOW.TenantLinks().Create(ctx, second); err != nil {
		t.Fatalf("Create (after revoke) unexpectedly failed: %v", err)
	}
	if err := admitUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (after revoke): %v", err)
	}

	// And the revoked link itself now reads back as revoked, not silently
	// reverted or deleted (migration 0004's UPDATE-in-place, retained row).
	verifyUOW := beginUOW(t, store)
	revokedRow, err := verifyUOW.TenantLinks().ByID(ctx, first.LinkID())
	_ = verifyUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ByID (revoked row): %v", err)
	}
	if revokedRow.Status() != domain.TenantLinkRevoked {
		t.Fatalf("Status() = %q, want %q", revokedRow.Status(), domain.TenantLinkRevoked)
	}
}

// TestTenantLinkRepository_Revoke_UnknownIDIsRefused mirrors the ByID
// absent-row contract: revoking an id nothing was ever created for answers
// domain.TenantLinkNotFound rather than silently succeeding with zero rows
// affected.
func TestTenantLinkRepository_Revoke_UnknownIDIsRefused(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	err := uow.TenantLinks().Revoke(ctx, "lnk_never-authorized")
	_ = uow.Rollback(ctx)

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.TenantLinkNotFound {
		t.Fatalf("expected tenant_link_not_found, got %v", err)
	}
}
