package postgres_test

import (
	"context"
	"sort"
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
// value per (tenant, account) -- four figures, treasury -100 twice and wallet
// +100 twice -- not one merged figure per bare id (treasury -200, wallet +200),
// which is what made the operator's books check report false drift.
//
// The assertion is written against what is observable of the result in either
// port shape -- how many balances come back and what each one is -- so it
// compiles before and after the tenant-qualified key replaces the bare
// account-id key. The scoped read is asserted first, as the fixture's own
// proof: a scoped call is correct today, so a failure of the unscoped half
// below cannot be a broken fixture.
func TestTransactionRepository_ComputedBalances_UnscopedKeepsTwoTenantsSharedAccountIdsApart(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	const (
		tenantA = "tnt_collide_a"
		tenantB = "tnt_collide_b"
	)
	provisionTenant(t, store, tenantA, "collide-a")
	provisionTenant(t, store, tenantB, "collide-b")

	setup := beginUOW(t, store)
	for _, tenantID := range []string{tenantA, tenantB} {
		seedAccountForTenant(t, ctx, setup, tenantID, "treasury", domain.System, -100_00)
		seedAccountForTenant(t, ctx, setup, tenantID, "wallet-1", domain.Wallet, 100_00)
	}
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	recordedAt := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	write := beginUOW(t, store)
	if err := write.Transactions().Append(ctx, tenantA,
		newPosting(t, "txn-collide-a", "treasury", "wallet-1", 100_00, recordedAt)); err != nil {
		t.Fatalf("Append tenantA: %v", err)
	}
	if err := write.Transactions().Append(ctx, tenantB,
		newPosting(t, "txn-collide-b", "treasury", "wallet-1", 100_00, recordedAt.Add(time.Minute))); err != nil {
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
	if got := minorUnitsOf(scoped); !equalMinorUnits(got, []int64{-100_00, 100_00}) {
		t.Fatalf("fixture check: tenantA's scoped computed balances = %v, want [-10000 10000]", got)
	}

	unscoped, err := read.Transactions().ComputedBalances(ctx, ports.Unscoped())
	if err != nil {
		t.Fatalf("ComputedBalances(Unscoped): %v", err)
	}
	if got, want := minorUnitsOf(unscoped), []int64{-100_00, -100_00, 100_00, 100_00}; !equalMinorUnits(got, want) {
		t.Fatalf("unscoped computed balances = %v, want one per (tenant, account) %v -- shared account ids must not merge across tenants", got, want)
	}
}

// minorUnitsOf lists a computed-balances result's figures in ascending order,
// whatever the result is keyed by.
func minorUnitsOf[K comparable](balances map[K]domain.Money) []int64 {
	figures := make([]int64, 0, len(balances))
	for _, balance := range balances {
		figures = append(figures, balance.MinorUnits())
	}
	sort.Slice(figures, func(i, j int) bool { return figures[i] < figures[j] })
	return figures
}

func equalMinorUnits(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
