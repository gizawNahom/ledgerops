package postgres_test

import (
	"context"
	"testing"
	"time"

	"ledgerops/internal/domain"
)

func TestIdempotencyStore_ClaimThenLookup_RoundTripsThroughRealPostgres(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	setup := beginUOW(t, store)
	seedAccount(t, ctx, setup, "wallet-idem", domain.Wallet, 1000)
	seedAccount(t, ctx, setup, "system-idem", domain.System, 0)
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	recordedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	posting := newPosting(t, "txn-idem-1", "wallet-idem", "system-idem", 75, recordedAt)

	claimUOW := beginUOW(t, store)
	if err := claimUOW.Transactions().Append(ctx, testTenantID, posting); err != nil {
		t.Fatalf("Append: %v", err)
	}
	claim, err := claimUOW.Idempotency().Claim(ctx, "idem-key-1", "fingerprint-1", "txn-idem-1")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if claim.Key != "idem-key-1" || claim.Fingerprint != "fingerprint-1" || claim.TransactionID != "txn-idem-1" {
		t.Fatalf("Claim = %+v, want key/fingerprint/transaction as passed in", claim)
	}
	if err := claimUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	lookupUOW := beginUOW(t, store)
	got, found, err := lookupUOW.Idempotency().Lookup(ctx, "idem-key-1")
	_ = lookupUOW.Rollback(ctx)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !found {
		t.Fatalf("Lookup did not find a claim committed moments earlier")
	}
	if got.Fingerprint != "fingerprint-1" || got.TransactionID != "txn-idem-1" {
		t.Fatalf("Lookup = %+v, want fingerprint-1 / txn-idem-1", got)
	}
}

func TestIdempotencyStore_Lookup_UnusedKeyIsNotFound(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	uow := beginUOW(t, store)
	_, found, err := uow.Idempotency().Lookup(ctx, "never-claimed")
	_ = uow.Rollback(ctx)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if found {
		t.Fatalf("Lookup reported a claim for a key that was never claimed")
	}
}

func TestIdempotencyStore_Claim_SameKeyTwiceIsRefusedByTheUniqueConstraint(t *testing.T) {
	store := migratedStore(t)
	ctx := context.Background()

	setup := beginUOW(t, store)
	seedAccount(t, ctx, setup, "wallet-idem-2", domain.Wallet, 1000)
	seedAccount(t, ctx, setup, "system-idem-2", domain.System, 0)
	if err := setup.Commit(ctx); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	recordedAt := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

	firstUOW := beginUOW(t, store)
	if err := firstUOW.Transactions().Append(ctx, testTenantID, newPosting(t, "txn-idem-2a", "wallet-idem-2", "system-idem-2", 10, recordedAt)); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if _, err := firstUOW.Idempotency().Claim(ctx, "idem-key-conflict", "fp-a", "txn-idem-2a"); err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	if err := firstUOW.Commit(ctx); err != nil {
		t.Fatalf("Commit 1: %v", err)
	}

	secondUOW := beginUOW(t, store)
	if err := secondUOW.Transactions().Append(ctx, testTenantID, newPosting(t, "txn-idem-2b", "wallet-idem-2", "system-idem-2", 20, recordedAt)); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	_, err := secondUOW.Idempotency().Claim(ctx, "idem-key-conflict", "fp-b", "txn-idem-2b")
	_ = secondUOW.Rollback(ctx)
	if err == nil {
		t.Fatalf("expected the unique constraint on `key` to refuse a second claim, got nil error")
	}
}
