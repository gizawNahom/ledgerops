package postgres_test

import (
	"context"
	"testing"
	"time"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// TestTransactionRepository_ComputedBalances_UnscopedKeepsTwoTenantsSharedAccountIdsApart
// is the adapter-level regression for fix-trial-balance-tenant-collision: two
// tenants each fund a wallet "wallet-1" from a system account "treasury" (the
// same two ids in both -- legal since I9 / migration 0003 made account
// identity (tenant_id, id)). The platform-wide computed balances must be one
// value per (tenant, account) -- exactly four keys, each carrying its own
// tenant's figure -- not one merged figure per bare id.
//
// The tenants fund different amounts (100.00 and 70.00) so that a swapped or
// merged attribution cannot pass by coincidence. The scoped read is asserted
// first, as the fixture's own proof: a scoped call is correct today, so a
// failure of the unscoped half below cannot be a broken fixture.
func TestTransactionRepository_ComputedBalances_UnscopedKeepsTwoTenantsSharedAccountIdsApart(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	const (
		tenantA = "tnt_collide_a"
		tenantB = "tnt_collide_b"

		fundingA int64 = 100_00
		fundingB int64 = 70_00
	)
	provisionTenant(t, store, tenantA, "collide-a")
	provisionTenant(t, store, tenantB, "collide-b")

	setup := beginUOW(t, store)
	for tenantID, funding := range map[string]int64{tenantA: fundingA, tenantB: fundingB} {
		seedAccountForTenant(t, ctx, setup, tenantID, "treasury", domain.System, -funding)
		seedAccountForTenant(t, ctx, setup, tenantID, "wallet-1", domain.Wallet, funding)
	}
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	recordedAt := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	write := beginUOW(t, store)
	if err := write.Transactions().Append(ctx, tenantA,
		newPosting(t, "txn-collide-a", "treasury", "wallet-1", fundingA, recordedAt)); err != nil {
		t.Fatalf("Append tenantA: %v", err)
	}
	if err := write.Transactions().Append(ctx, tenantB,
		newPosting(t, "txn-collide-b", "treasury", "wallet-1", fundingB, recordedAt.Add(time.Minute))); err != nil {
		t.Fatalf("Append tenantB: %v", err)
	}
	if err := write.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	read := beginUOW(t, store)
	defer func() { _ = read.Rollback(ctx) }()

	scoped, err := read.Transactions().ComputedBalances(ctx, ports.ScopedToTenant(tenantA))
	if err != nil {
		t.Fatalf("ComputedBalances(ScopedToTenant): %v", err)
	}
	wantScoped := map[ports.AccountKey]int64{
		{TenantID: tenantA, AccountID: "treasury"}: -fundingA,
		{TenantID: tenantA, AccountID: "wallet-1"}: fundingA,
	}
	assertComputedBalances(t, "fixture check: tenantA's scoped computed balances", scoped, wantScoped)

	unscoped, err := read.Transactions().ComputedBalances(ctx, ports.Unscoped())
	if err != nil {
		t.Fatalf("ComputedBalances(Unscoped): %v", err)
	}
	wantUnscoped := map[ports.AccountKey]int64{
		{TenantID: tenantA, AccountID: "treasury"}: -fundingA,
		{TenantID: tenantB, AccountID: "treasury"}: -fundingB,
		{TenantID: tenantA, AccountID: "wallet-1"}: fundingA,
		{TenantID: tenantB, AccountID: "wallet-1"}: fundingB,
	}
	assertComputedBalances(t, "unscoped computed balances (shared account ids must not merge across tenants)", unscoped, wantUnscoped)
}

// assertComputedBalances requires exactly the wanted keys, each mapped to its
// wanted minor-unit figure.
func assertComputedBalances(t *testing.T, label string, got map[ports.AccountKey]domain.Money, want map[ports.AccountKey]int64) {
	t.Helper()
	for key, wantMinor := range want {
		balance, ok := got[key]
		if !ok {
			t.Errorf("%s: missing key %+v", label, key)
			continue
		}
		if balance.MinorUnits() != wantMinor {
			t.Errorf("%s: %+v = %d, want %d", label, key, balance.MinorUnits(), wantMinor)
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("%s: unexpected key %+v", label, key)
		}
	}
}
