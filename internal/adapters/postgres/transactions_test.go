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
	if !entries[0].RecordedAt().Before(entries[1].RecordedAt()) {
		t.Fatalf("entries not ordered by recorded_at: %+v", entries)
	}
	if entries[0].Amount().MinorUnits() != -100 || entries[1].Amount().MinorUnits() != -50 {
		t.Fatalf("unexpected amounts: %+v", entries)
	}
}

// TestTransactionRepository_EntriesFor_UnscopedSeesEveryTenantScopedSeesOnlyItsOwn
// covers the pure-function/return-only contract shape (brief.md § Multitenancy
// Contract-shape table): a scoped call narrows to one tenant's own entries; an
// unscoped call (ports.Unscoped()) stays platform-wide, byte-identical in
// shape to the pre-multitenancy contract every existing OperatorKey/console
// call path depends on.
func TestTransactionRepository_EntriesFor_UnscopedSeesEveryTenantScopedSeesOnlyItsOwn(t *testing.T) {
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
	// tenantB opens an account under the SAME id ("wallet-scoped") — I9's
	// rescoping is what makes this legal — so the unscoped query below has a
	// genuine collision to prove it does not narrow by tenant.
	seedAccountForTenant(t, ctx, setup, tenantB, "wallet-scoped", domain.System, 0)
	seedAccountForTenant(t, ctx, setup, tenantB, "system-scoped-b", domain.System, 0)
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	recordedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

	writeUOW := beginUOW(t, store)
	if err := writeUOW.Transactions().Append(ctx, tenantA,
		newPosting(t, "txn-entries-a", "wallet-scoped", "system-scoped-a", 100, recordedAt)); err != nil {
		t.Fatalf("Append tenantA: %v", err)
	}
	if err := writeUOW.Transactions().Append(ctx, tenantB,
		newPosting(t, "txn-entries-b", "system-scoped-b", "wallet-scoped", 25, recordedAt.Add(time.Minute))); err != nil {
		t.Fatalf("Append tenantB: %v", err)
	}
	if err := writeUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	readUOW := beginUOW(t, store)
	scoped, err := readUOW.Transactions().EntriesFor(ctx, ports.ScopedToTenant(tenantA), "wallet-scoped")
	if err != nil {
		t.Fatalf("EntriesFor(ScopedToTenant): %v", err)
	}
	unscoped, err := readUOW.Transactions().EntriesFor(ctx, ports.Unscoped(), "wallet-scoped")
	_ = readUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("EntriesFor(Unscoped): %v", err)
	}

	if len(scoped) != 1 {
		t.Fatalf("scoped EntriesFor for tenantA's own account: got %d entries, want 1 (tenantB's entry on the same account id must not appear): %+v", len(scoped), scoped)
	}
	if scoped[0].TransactionID() != "txn-entries-a" {
		t.Fatalf("scoped EntriesFor returned transaction %q, want txn-entries-a", scoped[0].TransactionID())
	}
	if len(unscoped) != 2 {
		t.Fatalf("unscoped EntriesFor: got %d entries, want 2 (both tenants' entries against the colliding account id — the pre-existing unscoped contract): %+v", len(unscoped), unscoped)
	}
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
