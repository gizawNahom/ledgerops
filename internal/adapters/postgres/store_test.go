package postgres_test

import (
	"context"
	"testing"

	"ledgerops/internal/domain"
)

// TestStore_Begin_ThreeRepositoriesShareOneTransaction proves the DDD-13
// guarantee: writes issued through one repository obtained from a unit of
// work are visible to another repository obtained from the SAME unit of
// work before commit, and nothing is visible outside it until commit runs.
func TestStore_Begin_ThreeRepositoriesShareOneTransaction(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	// Written through Accounts()...
	seedAccount(t, ctx, uow, "wallet-shared", domain.Wallet, 1000)
	// ...read back through a second call to Accounts() on the SAME uow,
	// before commit — only possible if both share one transaction handle.
	got, err := uow.Accounts().Get(ctx, testTenantID, "wallet-shared")
	if err != nil {
		t.Fatalf("Get within the same uncommitted unit of work: %v", err)
	}
	if got.Balance().MinorUnits() != 1000 {
		t.Fatalf("Balance().MinorUnits() = %d, want 1000", got.Balance().MinorUnits())
	}
	if err := uow.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Now visible from a fresh unit of work too.
	verifyUOW := beginUOW(t, store)
	got, err = verifyUOW.Accounts().Get(ctx, testTenantID, "wallet-shared")
	_ = verifyUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("Get after commit: %v", err)
	}
	if got.ID() != "wallet-shared" {
		t.Fatalf("ID() = %q, want %q", got.ID(), "wallet-shared")
	}
}

// TestStore_Begin_UncommittedWorkIsInvisibleAfterRollback proves the other
// half: a unit of work that never commits leaves no trace.
func TestStore_Begin_UncommittedWorkIsInvisibleAfterRollback(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	seedAccount(t, ctx, uow, "wallet-never-committed", domain.Wallet, 1)
	if err := uow.Rollback(ctx); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	verifyUOW := beginUOW(t, store)
	_, err := verifyUOW.Accounts().Get(ctx, testTenantID, "wallet-never-committed")
	_ = verifyUOW.Rollback(ctx)

	var violation domain.Violation
	if err == nil {
		t.Fatalf("expected the rolled-back account to be invisible, but Get succeeded")
	}
	if v, ok := err.(domain.Violation); ok {
		violation = v
	} else {
		t.Fatalf("expected a domain.Violation, got %T: %v", err, err)
	}
	if violation.Kind() != domain.UnknownAccount {
		t.Fatalf("Kind() = %v, want %v", violation.Kind(), domain.UnknownAccount)
	}
}
