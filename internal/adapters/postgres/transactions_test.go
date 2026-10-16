package postgres_test

import (
	"context"
	"testing"
	"time"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

func newPosting(t *testing.T, transactionID, fromID, toID string, amountMinor int64, recordedAt time.Time) domain.Posting {
	t.Helper()
	amount, err := domain.NewMoney(amountMinor, "USD")
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	transaction, err := domain.NewTransaction(transactionID, recordedAt)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	fromEntry, err := domain.NewEntry(transactionID, fromID, toID, amount.Negate(), recordedAt, 0)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	toEntry, err := domain.NewEntry(transactionID, toID, fromID, amount, recordedAt, 1)
	if err != nil {
		t.Fatalf("NewEntry: %v", err)
	}
	return domain.Posting{
		Transaction: transaction,
		Entries:     []domain.Entry{fromEntry, toEntry},
		Deltas: []domain.BalanceDelta{
			{AccountID: fromID, Delta: amount.Negate()},
			{AccountID: toID, Delta: amount},
		},
	}
}

func TestTransactionRepository_Append_WritesTransactionAndEntriesTogether(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	setup := beginUOW(t, store)
	seedAccount(t, ctx, setup, "wallet-a", domain.Wallet, 1000)
	seedAccount(t, ctx, setup, "system-a", domain.System, 0)
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	recordedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	posting := newPosting(t, "txn-append-1", "wallet-a", "system-a", 250, recordedAt)

	writeUOW := beginUOW(t, store)
	if err := writeUOW.Transactions().Append(ctx, testTenantID, posting); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	got, err := readUOW.Transactions().Get(ctx, testTenantID, "txn-append-1")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Transaction.ID() != "txn-append-1" {
		t.Fatalf("Transaction.ID() = %q, want %q", got.Transaction.ID(), "txn-append-1")
	}
	if !got.Transaction.RecordedAt().Equal(recordedAt) {
		t.Fatalf("Transaction.RecordedAt() = %v, want %v", got.Transaction.RecordedAt(), recordedAt)
	}
	if len(got.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(got.Entries))
	}
	if got.Entries[0].AccountID() != "wallet-a" || got.Entries[0].Amount().MinorUnits() != -250 {
		t.Fatalf("entry 0 = %+v, want account wallet-a amount -250", got.Entries[0])
	}
	if got.Entries[1].AccountID() != "system-a" || got.Entries[1].Amount().MinorUnits() != 250 {
		t.Fatalf("entry 1 = %+v, want account system-a amount 250", got.Entries[1])
	}
}

func TestTransactionRepository_EntriesFor_ReturnsOneAccountsHistoryOrdered(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	setup := beginUOW(t, store)
	seedAccount(t, ctx, setup, "wallet-b", domain.Wallet, 1000)
	seedAccount(t, ctx, setup, "system-b", domain.System, 0)
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	first := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	second := first.Add(time.Minute)

	writeUOW := beginUOW(t, store)
	if err := writeUOW.Transactions().Append(ctx, testTenantID, newPosting(t, "txn-history-1", "wallet-b", "system-b", 100, first)); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := writeUOW.Transactions().Append(ctx, testTenantID, newPosting(t, "txn-history-2", "wallet-b", "system-b", 50, second)); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	entries, err := readUOW.Transactions().EntriesFor(ctx, ports.ScopedToTenant(testTenantID), "wallet-b")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("EntriesFor: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if !entries[0].Entry.RecordedAt().Before(entries[1].Entry.RecordedAt()) {
		t.Fatalf("entries not ordered by recorded_at: %+v", entries)
	}
	if entries[0].Entry.Amount().MinorUnits() != -100 || entries[1].Entry.Amount().MinorUnits() != -50 {
		t.Fatalf("unexpected amounts: %+v", entries)
	}
}

// TestTransactionRepository_EntriesFor_UnscopedAttributesEachEntryToItsTenantScopedSeesOnlyItsOwn
// replaces the pin that asserted the cross-tenant merge as the contract
// (fix-unscoped-entries-tenant-merge). Two tenants hold an account under the
// SAME id (I9). An unscoped call (ports.Unscoped()) still reaches every
// tenant's entries, but each one is returned as a ports.TenantEntry naming the
// tenant it belongs to, and the rows are tenant-contiguous (ordered by
// tenant, then recorded instant, then sequence) -- never interleaved by
// instant across tenants. A scoped call narrows to one tenant and names it.
//
// Written against the intended port shape (rca.md section 3):
// EntriesFor returns []ports.TenantEntry{TenantID, Entry}. It does not compile
// until the port changes -- that compile failure is the RED.
func TestTransactionRepository_EntriesFor_UnscopedAttributesEachEntryToItsTenantScopedSeesOnlyItsOwn(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	const (
		tenantA = "tnt_entries_a"
		tenantB = "tnt_entries_b"
	)
	provisionTenant(t, store, tenantA, "entries-scope-a")
	provisionTenant(t, store, tenantB, "entries-scope-b")

	setup := beginUOW(t, store)
	seedAccountForTenant(t, ctx, setup, tenantA, "wallet-scoped", domain.Wallet, 1000)
	seedAccountForTenant(t, ctx, setup, tenantA, "system-scoped-a", domain.System, 0)
	// tenantB opens an account under the SAME id ("wallet-scoped") -- I9's
	// rescoping is what makes this legal -- so the unscoped query below has a
	// genuine collision to attribute.
	seedAccountForTenant(t, ctx, setup, tenantB, "wallet-scoped", domain.System, 0)
	seedAccountForTenant(t, ctx, setup, tenantB, "system-scoped-b", domain.System, 0)
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	recordedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

	// Recorded order across tenants is A, B, A: an unscoped read ordered by
	// instant alone would interleave them; tenant-contiguous order is A, A, B.
	writeUOW := beginUOW(t, store)
	if err := writeUOW.Transactions().Append(ctx, tenantA,
		newPosting(t, "txn-entries-a1", "wallet-scoped", "system-scoped-a", 100, recordedAt)); err != nil {
		t.Fatalf("Append tenantA first: %v", err)
	}
	if err := writeUOW.Transactions().Append(ctx, tenantB,
		newPosting(t, "txn-entries-b1", "system-scoped-b", "wallet-scoped", 25, recordedAt.Add(time.Minute))); err != nil {
		t.Fatalf("Append tenantB: %v", err)
	}
	if err := writeUOW.Transactions().Append(ctx, tenantA,
		newPosting(t, "txn-entries-a2", "wallet-scoped", "system-scoped-a", 10, recordedAt.Add(2*time.Minute))); err != nil {
		t.Fatalf("Append tenantA second: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	scopedA, err := readUOW.Transactions().EntriesFor(ctx, ports.ScopedToTenant(tenantA), "wallet-scoped")
	if err != nil {
		t.Fatalf("EntriesFor(ScopedToTenant A): %v", err)
	}
	scopedB, err := readUOW.Transactions().EntriesFor(ctx, ports.ScopedToTenant(tenantB), "wallet-scoped")
	if err != nil {
		t.Fatalf("EntriesFor(ScopedToTenant B): %v", err)
	}
	unscoped, err := readUOW.Transactions().EntriesFor(ctx, ports.Unscoped(), "wallet-scoped")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("EntriesFor(Unscoped): %v", err)
	}

	describe := func(rows []ports.TenantEntry) []string {
		out := make([]string, 0, len(rows))
		for _, row := range rows {
			out = append(out, row.TenantID+"/"+row.Entry.TransactionID())
		}
		return out
	}

	wantScopedA := []string{tenantA + "/txn-entries-a1", tenantA + "/txn-entries-a2"}
	if got := describe(scopedA); !equalStrings(got, wantScopedA) {
		t.Fatalf("scoped to tenantA: got %v, want %v (tenantB's entry on the same account id must not appear, and every row names tenantA)", got, wantScopedA)
	}
	wantScopedB := []string{tenantB + "/txn-entries-b1"}
	if got := describe(scopedB); !equalStrings(got, wantScopedB) {
		t.Fatalf("scoped to tenantB: got %v, want %v", got, wantScopedB)
	}

	// Per-tenant attribution, tenant-contiguous: both of A's entries, then B's.
	wantUnscoped := []string{tenantA + "/txn-entries-a1", tenantA + "/txn-entries-a2", tenantB + "/txn-entries-b1"}
	if got := describe(unscoped); !equalStrings(got, wantUnscoped) {
		t.Fatalf("unscoped: got %v, want %v (each entry attributed to its own tenant, tenants not interleaved)", got, wantUnscoped)
	}
	distinct := map[string]bool{}
	for _, row := range unscoped {
		distinct[row.TenantID] = true
	}
	if len(distinct) != 2 {
		t.Fatalf("unscoped read names %d distinct tenant ids, want 2: %v", len(distinct), describe(unscoped))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func seedAccountForTenant(t *testing.T, ctx context.Context, uow ports.UnitOfWork, tenantID, id string, kind domain.AccountKind, startMinor int64) {
	t.Helper()
	balance, err := domain.NewMoney(startMinor, "USD")
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	account, err := domain.NewAccount(tenantID, id, kind, balance)
	if err != nil {
		t.Fatalf("NewAccount(%q, %q): %v", tenantID, id, err)
	}
	if err := uow.Accounts().Create(ctx, tenantID, account); err != nil {
		t.Fatalf("Create(%q, %q): %v", tenantID, id, err)
	}
}

func seedAccount(t *testing.T, ctx context.Context, uow ports.UnitOfWork, id string, kind domain.AccountKind, startMinor int64) {
	t.Helper()
	balance, err := domain.NewMoney(startMinor, "USD")
	if err != nil {
		t.Fatalf("NewMoney: %v", err)
	}
	account, err := domain.NewAccount(testTenantID, id, kind, balance)
	if err != nil {
		t.Fatalf("NewAccount(%q): %v", id, err)
	}
	if err := uow.Accounts().Create(ctx, testTenantID, account); err != nil {
		t.Fatalf("Create(%q): %v", id, err)
	}
}
