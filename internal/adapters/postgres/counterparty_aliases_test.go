// Adapter integration test for the driven postgres
// CounterpartyAliasRepository, real PostgreSQL 16 via Testcontainers
// (OPS-11, Mandate 6). A wiring test, not a property test, per the layered
// test discipline (nw-tdd-methodology § Layered test discipline):
// "Integration | UNCHANGED — single-example test verifies WIRING."
//
// This step (02-03) has no pre-authored acceptance test reaching this
// layer — HTTP wiring for RegisterCounterpartyAlias/ResolveCounterparty
// lands in a later step — so this adapter test IS this step's own
// RED/GREEN obligation, mirroring tenant_links_test.go's own note for
// step 01-03.
package postgres_test

import (
	"context"
	"testing"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// seedTargetAccount commits one Wallet account under the given tenant, in
// its own unit of work — counterparty_aliases.target_account_id carries a
// composite FK to accounts (tenant_id, id) (migration 02-01), so any alias
// naming a target account must seed that row first, mirroring
// provisionTenant's own FK-satisfying role for tenant_id columns.
func seedTargetAccount(t *testing.T, store ports.Store, tenantID, accountID string) {
	t.Helper()
	ctx := context.Background()
	uow := beginUOW(t, store)
	seedAccountForTenant(t, ctx, uow, tenantID, accountID, domain.System, 0)
	if err := uow.Commit(ctx); err != nil {
		t.Fatalf("committing target account %q/%q: %v", tenantID, accountID, err)
	}
}

// TestCounterpartyAliasRepository_CreateThenByTenantAndAlias_RoundTripsThroughRealPostgres
// covers the wiring itself: an alias registered via the pure
// domain.RegisterCounterpartyAlias constructor, persisted through Create,
// reads back byte-identical through ByTenantAndAlias.
func TestCounterpartyAliasRepository_CreateThenByTenantAndAlias_RoundTripsThroughRealPostgres(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	provisionTenant(t, store, "tnt_acme", "acme")
	provisionTenant(t, store, "tnt_beacon", "beacon")
	seedTargetAccount(t, store, "tnt_beacon", "acc_beacon-payout")

	link, err := domain.AuthorizeTenantPair("lnk_alias-1", "tnt_acme", "tnt_beacon", nil, map[string]bool{
		"tnt_acme":   true,
		"tnt_beacon": true,
	})
	if err != nil {
		t.Fatalf("AuthorizeTenantPair: %v", err)
	}
	linkUOW := beginUOW(t, store)
	if err := linkUOW.TenantLinks().Create(ctx, link); err != nil {
		t.Fatalf("TenantLinks().Create: %v", err)
	}
	if err := linkUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (link): %v", err)
	}

	alias, err := domain.RegisterCounterpartyAlias("tnt_acme", "beacon-payout", link, true, "tnt_beacon", "acc_beacon-payout")
	if err != nil {
		t.Fatalf("RegisterCounterpartyAlias: %v", err)
	}

	writeUOW := beginUOW(t, store)
	if err := writeUOW.CounterpartyAliases().Create(ctx, alias); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	got, found, err := readUOW.CounterpartyAliases().ByTenantAndAlias(ctx, "tnt_acme", "beacon-payout")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("ByTenantAndAlias: %v", err)
	}
	if !found {
		t.Fatalf("ByTenantAndAlias() found = false, want true")
	}
	if got != alias {
		t.Fatalf("ByTenantAndAlias() = %+v, want %+v", got, alias)
	}
}

// TestCounterpartyAliasRepository_ByTenantAndAlias_AbsentPairIsNotFoundNotError
// proves the expected shape of "no": an alias never registered under the
// given (tenant_id, alias) pair answers (zero value, false, nil), not an
// error — mirroring TenantLinkRepository.ActiveByPair's own contract.
func TestCounterpartyAliasRepository_ByTenantAndAlias_AbsentPairIsNotFoundNotError(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	provisionTenant(t, store, "tnt_acme", "acme")

	uow := beginUOW(t, store)
	got, found, err := uow.CounterpartyAliases().ByTenantAndAlias(ctx, "tnt_acme", "never-registered")
	_ = uow.Rollback(ctx)

	if err != nil {
		t.Fatalf("ByTenantAndAlias unexpectedly errored: %v", err)
	}
	if found {
		t.Fatalf("ByTenantAndAlias() found = true, want false for an absent pair")
	}
	if got != (domain.CounterpartyAlias{}) {
		t.Fatalf("ByTenantAndAlias() = %+v, want zero value", got)
	}
}

// TestCounterpartyAliasRepository_AliasNamespaceIsScopedToOwningTenant is
// this step's Earned Trust proof of the composite primary key's own job:
// two different tenants registering the identical alias string under two
// different target counterparties collide with nothing — each is its own
// row, and each tenant's ByTenantAndAlias resolves only to its own
// registration, never the other's.
func TestCounterpartyAliasRepository_AliasNamespaceIsScopedToOwningTenant(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	provisionTenant(t, store, "tnt_acme", "acme")
	provisionTenant(t, store, "tnt_beacon", "beacon")
	provisionTenant(t, store, "tnt_carter", "carter")
	seedTargetAccount(t, store, "tnt_beacon", "acc_beacon-target")
	seedTargetAccount(t, store, "tnt_carter", "acc_carter-target")

	acmeBeaconLink, err := domain.AuthorizeTenantPair("lnk_acme-beacon", "tnt_acme", "tnt_beacon", nil, map[string]bool{
		"tnt_acme":   true,
		"tnt_beacon": true,
	})
	if err != nil {
		t.Fatalf("AuthorizeTenantPair (acme/beacon): %v", err)
	}
	carterBeaconLink, err := domain.AuthorizeTenantPair("lnk_carter-beacon", "tnt_carter", "tnt_beacon", nil, map[string]bool{
		"tnt_carter": true,
		"tnt_beacon": true,
	})
	if err != nil {
		t.Fatalf("AuthorizeTenantPair (carter/beacon): %v", err)
	}

	linkUOW := beginUOW(t, store)
	if err := linkUOW.TenantLinks().Create(ctx, acmeBeaconLink); err != nil {
		t.Fatalf("TenantLinks().Create (acme/beacon): %v", err)
	}
	if err := linkUOW.TenantLinks().Create(ctx, carterBeaconLink); err != nil {
		t.Fatalf("TenantLinks().Create (carter/beacon): %v", err)
	}
	if err := linkUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit (links): %v", err)
	}

	acmeAlias, err := domain.RegisterCounterpartyAlias("tnt_acme", "shared-name", acmeBeaconLink, true, "tnt_beacon", "acc_beacon-target")
	if err != nil {
		t.Fatalf("RegisterCounterpartyAlias (acme): %v", err)
	}
	carterAlias, err := domain.RegisterCounterpartyAlias("tnt_carter", "shared-name", carterBeaconLink, true, "tnt_carter", "acc_carter-target")
	if err != nil {
		t.Fatalf("RegisterCounterpartyAlias (carter): %v", err)
	}

	writeUOW := beginUOW(t, store)
	if err := writeUOW.CounterpartyAliases().Create(ctx, acmeAlias); err != nil {
		t.Fatalf("Create (acme): %v", err)
	}
	if err := writeUOW.CounterpartyAliases().Create(ctx, carterAlias); err != nil {
		t.Fatalf("Create (carter) unexpectedly collided with acme's row: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	acmeReadUOW := beginUOW(t, store)
	gotAcme, found, err := acmeReadUOW.CounterpartyAliases().ByTenantAndAlias(ctx, "tnt_acme", "shared-name")
	_ = acmeReadUOW.Rollback(ctx)
	if err != nil || !found {
		t.Fatalf("ByTenantAndAlias (acme) = %+v, %v, %v", gotAcme, found, err)
	}
	if gotAcme.TargetAccountID() != "acc_beacon-target" {
		t.Fatalf("acme's alias resolved to %q, want acc_beacon-target", gotAcme.TargetAccountID())
	}

	carterReadUOW := beginUOW(t, store)
	gotCarter, found, err := carterReadUOW.CounterpartyAliases().ByTenantAndAlias(ctx, "tnt_carter", "shared-name")
	_ = carterReadUOW.Rollback(ctx)
	if err != nil || !found {
		t.Fatalf("ByTenantAndAlias (carter) = %+v, %v, %v", gotCarter, found, err)
	}
	if gotCarter.TargetAccountID() != "acc_carter-target" {
		t.Fatalf("carter's alias resolved to %q, want acc_carter-target", gotCarter.TargetAccountID())
	}
}
