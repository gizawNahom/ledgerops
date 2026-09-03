package postgres_test

import (
	"context"
	"errors"
	"testing"

	"ledgerops/internal/domain"
)

// TestAccountRepository_Create_TwoTenantsCanIndependentlyReuseTheSameAccountID
// covers I9's rescoping (step 02-02): account-name uniqueness moved from
// global id uniqueness to the composite (tenant_id, id) primary key
// (migration 0003). Two different tenants opening an account under the
// identical id must both succeed, and each tenant's Get must return only
// its own account.
func TestAccountRepository_Create_TwoTenantsCanIndependentlyReuseTheSameAccountID(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	const (
		tenantA = "tnt_acme"
		tenantB = "tnt_beacon"
	)
	provisionTenant(t, store, tenantA, "acme-i9")
	provisionTenant(t, store, tenantB, "beacon-i9")

	setup := beginUOW(t, store)
	acmeAccount, err := domain.NewAccount(tenantA, "wallet-1", domain.Wallet, zeroUSD(t))
	if err != nil {
		t.Fatalf("NewAccount(tenantA): %v", err)
	}
	if err := setup.Accounts().Create(ctx, tenantA, acmeAccount); err != nil {
		t.Fatalf("Create(tenantA, wallet-1): %v", err)
	}
	beaconAccount, err := domain.NewAccount(tenantB, "wallet-1", domain.Wallet, zeroUSD(t))
	if err != nil {
		t.Fatalf("NewAccount(tenantB): %v", err)
	}
	if err := setup.Accounts().Create(ctx, tenantB, beaconAccount); err != nil {
		t.Fatalf("Create(tenantB, wallet-1) was refused by a stale global-uniqueness constraint: %v", err)
	}
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	gotA, err := readUOW.Accounts().Get(ctx, tenantA, "wallet-1")
	if err != nil {
		t.Fatalf("Get(tenantA, wallet-1): %v", err)
	}
	gotB, err := readUOW.Accounts().Get(ctx, tenantB, "wallet-1")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("Get(tenantB, wallet-1): %v", err)
	}

	if gotA.TenantID() != tenantA {
		t.Fatalf("tenantA's Get returned TenantID() = %q, want %q", gotA.TenantID(), tenantA)
	}
	if gotB.TenantID() != tenantB {
		t.Fatalf("tenantB's Get returned TenantID() = %q, want %q", gotB.TenantID(), tenantB)
	}
}

// TestAccountRepository_Get_NeverReturnsAnotherTenantsAccount covers the
// tenant-scoped read half of I8/I9: a query for one tenant's account never
// returns another tenant's row, even when both hold an account under the
// same id.
func TestAccountRepository_Get_NeverReturnsAnotherTenantsAccount(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	const (
		tenantA = "tnt_alpha"
		tenantB = "tnt_bravo"
	)
	provisionTenant(t, store, tenantA, "alpha-scoped-read")
	provisionTenant(t, store, tenantB, "bravo-scoped-read")

	setup := beginUOW(t, store)
	account, err := domain.NewAccount(tenantA, "shared-name", domain.Wallet, zeroUSD(t))
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	if err := setup.Accounts().Create(ctx, tenantA, account); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	_, err = readUOW.Accounts().Get(ctx, tenantB, "shared-name")
	_ = readUOW.Rollback(ctx)

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.UnknownAccount {
		t.Fatalf("expected account_not_found for a cross-tenant read (even though the id exists under tenantA), got %v", err)
	}
}

func zeroUSD(t *testing.T) domain.Money {
	t.Helper()
	zero, err := domain.NewMoney(0, "USD")
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	return zero
}

func TestAccountRepository_CreateThenGet_RoundTripsThroughRealPostgres(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	seedAccount(t, ctx, uow, "wallet-create", domain.Wallet, 0)
	if err := uow.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	got, err := readUOW.Accounts().Get(ctx, testTenantID, "wallet-create")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	_ = readUOW.Rollback(ctx)

	if got.ID() != "wallet-create" {
		t.Fatalf("ID() = %q, want %q", got.ID(), "wallet-create")
	}
	if got.Kind() != domain.Wallet {
		t.Fatalf("Kind() = %v, want %v", got.Kind(), domain.Wallet)
	}
	if got.Balance().MinorUnits() != 0 {
		t.Fatalf("Balance().MinorUnits() = %d, want 0", got.Balance().MinorUnits())
	}
}

func TestAccountRepository_Get_UnknownAccountIsRefused(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	_, err := uow.Accounts().Get(ctx, testTenantID, "never-opened")
	_ = uow.Rollback(ctx)

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.UnknownAccount {
		t.Fatalf("expected account_not_found, got %v", err)
	}
}

func TestAccountRepository_LockForUpdate_ReturnsAscendingOrderAndSkipsUnknownIDs(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	setup := beginUOW(t, store)
	for _, id := range []string{"charlie", "alpha", "bravo"} {
		seedAccount(t, ctx, setup, id, domain.Wallet, 100)
	}
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Passed in descending order, plus one id that was never opened —
	// LockForUpdate must still return ascending order (DDD-6) and must
	// silently omit the unknown id (domain.Post is what names it, not the
	// repository).
	lockUOW := beginUOW(t, store)
	accounts, err := lockUOW.Accounts().LockForUpdate(ctx, testTenantID, []string{"charlie", "ghost", "bravo", "alpha"})
	_ = lockUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("LockForUpdate: %v", err)
	}

	if len(accounts) != 3 {
		t.Fatalf("got %d accounts, want 3 (unknown id must be skipped): %+v", len(accounts), accounts)
	}
	gotOrder := []string{accounts[0].ID(), accounts[1].ID(), accounts[2].ID()}
	wantOrder := []string{"alpha", "bravo", "charlie"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("order = %v, want %v", gotOrder, wantOrder)
		}
	}
}

func TestAccountRepository_ApplyDeltas_UpdatesStoredBalanceWithinOneTransaction(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	setup := beginUOW(t, store)
	seedAccount(t, ctx, setup, "wallet-delta", domain.Wallet, 500)
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	delta, err := domain.NewMoney(-125, "USD")
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	applyUOW := beginUOW(t, store)
	if _, err := applyUOW.Accounts().LockForUpdate(ctx, testTenantID, []string{"wallet-delta"}); err != nil {
		t.Fatalf("LockForUpdate: %v", err)
	}
	if err := applyUOW.Accounts().ApplyDeltas(ctx, testTenantID, []domain.BalanceDelta{
		{AccountID: "wallet-delta", Delta: delta},
	}); err != nil {
		t.Fatalf("ApplyDeltas: %v", err)
	}
	if err := applyUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	got, err := readUOW.Accounts().Get(ctx, testTenantID, "wallet-delta")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Balance().MinorUnits() != 375 {
		t.Fatalf("Balance().MinorUnits() = %d, want 375", got.Balance().MinorUnits())
	}
}

func TestAccountRepository_ApplyDeltas_RolledBackTransactionLeavesBalanceUnchanged(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	setup := beginUOW(t, store)
	seedAccount(t, ctx, setup, "wallet-rollback", domain.Wallet, 200)
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	delta, err := domain.NewMoney(-50, "USD")
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	applyUOW := beginUOW(t, store)
	if _, err := applyUOW.Accounts().LockForUpdate(ctx, testTenantID, []string{"wallet-rollback"}); err != nil {
		t.Fatalf("LockForUpdate: %v", err)
	}
	if err := applyUOW.Accounts().ApplyDeltas(ctx, testTenantID, []domain.BalanceDelta{
		{AccountID: "wallet-rollback", Delta: delta},
	}); err != nil {
		t.Fatalf("ApplyDeltas: %v", err)
	}
	if err := applyUOW.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	readUOW := beginUOW(t, store)
	got, err := readUOW.Accounts().Get(ctx, testTenantID, "wallet-rollback")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Balance().MinorUnits() != 200 {
		t.Fatalf("a rolled-back ApplyDeltas leaked: Balance().MinorUnits() = %d, want 200", got.Balance().MinorUnits())
	}
}
