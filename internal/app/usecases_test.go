// Package app_test exercises the shell through its driving ports — the
// exported methods of *app.Ledger — over fake repositories (permitted at this
// layer, see fakes_test.go). The real repositories are exercised for real
// against PostgreSQL via Testcontainers in internal/adapters/postgres
// (Mandate 6); this suite proves PostTransfer's ORCHESTRATION: lock, decide,
// write, all inside one unit of work.
//
// Property-based by default (pgregory.net/rapid) with state-delta matchers
// (tests/common/statedelta), per step 01-03's mandated TEST PARADIGM.
package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"pgregory.net/rapid"

	"ledgerops/internal/app"
	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
	"ledgerops/tests/common/statedelta"
)

func fixedClock() time.Time {
	return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
}

func sequentialIDs() ports.IDGenerator {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("txn-%d", n)
	}
}

func captureUniverse(store *fakeStore, from, to string) statedelta.Snapshot {
	return statedelta.Snapshot{
		"account.from.balance": store.accounts[from].Balance().MinorUnits(),
		"account.to.balance":   store.accounts[to].Balance().MinorUnits(),
		"ledger.entry_count":   len(store.entries),
		"idempotency.claimed":  len(store.claims) > 0,
	}
}

var universe = []string{
	"account.from.balance",
	"account.to.balance",
	"ledger.entry_count",
	"idempotency.claimed",
}

// TestProperty_PostTransfer_MovesValueAndClaimsTheKeyAtomically covers the
// bounded-change contract for PostTransfer's success path (step 01-03
// implementation_notes): the universe is the touched accounts' balances, the
// new transaction's entries, and the idempotency record — everything else
// declared must be unchanged, and here nothing else is declared, so the
// assertion is exact.
func TestProperty_PostTransfer_MovesValueAndClaimsTheKeyAtomically(t *testing.T) {
	// AssertStateDelta needs testing.TB, which *rapid.T does not implement
	// (TB is a sealed interface) — draws happen on rt, assertions on the
	// outer t, per rapid's own recommended pattern for interop with
	// TB-typed helpers.
	rapid.Check(t, func(rt *rapid.T) {
		fromStartMinor := rapid.Int64Range(1, 1_000_000).Draw(rt, "fromStart")
		amountMinor := rapid.Int64Range(1, fromStartMinor).Draw(rt, "amount")

		fromBalance, err := domain.NewMoney(fromStartMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount("from", domain.Wallet, fromBalance)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		zero, err := domain.NewMoney(0, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount("to", domain.System, zero)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		store := newFakeStore(fromAccount, toAccount)
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		amount, err := domain.NewMoney(amountMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		before := captureUniverse(store, "from", "to")
		result, err := ledger.PostTransfer(context.Background(), app.TransferRequest{
			From:           "from",
			To:             "to",
			Amount:         amount,
			IdempotencyKey: "idem-key",
			Fingerprint:    "fingerprint",
		})
		if err != nil {
			rt.Fatalf("unexpected refusal moving %d of %d available: %v", amountMinor, fromStartMinor, err)
		}
		after := captureUniverse(store, "from", "to")

		statedelta.AssertStateDelta(t, before, after, universe, map[string]statedelta.Predicate{
			"account.from.balance": statedelta.SetTo(fromStartMinor - amountMinor),
			"account.to.balance":   statedelta.SetTo(amountMinor),
			"ledger.entry_count":   statedelta.SetTo(2),
			"idempotency.claimed":  statedelta.SetTo(true),
		})

		if result.Replayed {
			rt.Fatalf("a first-time posting must not report Replayed")
		}
		if !store.committed {
			rt.Fatalf("PostTransfer succeeded without committing the unit of work")
		}
	})
}

// TestProperty_PostTransfer_RefusalLeavesEveryUniverseSlotUnchanged covers the
// unbounded-preservation side of the same contract: when domain.Post refuses
// a movement (insufficient funds), nothing in the universe changes — the
// Write step never runs, and the deferred Rollback is what makes that true
// even if it did.
func TestProperty_PostTransfer_RefusalLeavesEveryUniverseSlotUnchanged(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		fromStartMinor := rapid.Int64Range(0, 1_000).Draw(rt, "fromStart")
		amountMinor := rapid.Int64Range(fromStartMinor+1, fromStartMinor+1_000_000).Draw(rt, "amount")

		fromBalance, err := domain.NewMoney(fromStartMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount("from", domain.Wallet, fromBalance)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		zero, err := domain.NewMoney(0, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount("to", domain.System, zero)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		store := newFakeStore(fromAccount, toAccount)
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		amount, err := domain.NewMoney(amountMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		before := captureUniverse(store, "from", "to")
		_, err = ledger.PostTransfer(context.Background(), app.TransferRequest{
			From:           "from",
			To:             "to",
			Amount:         amount,
			IdempotencyKey: "idem-key",
			Fingerprint:    "fingerprint",
		})
		after := captureUniverse(store, "from", "to")

		var violation domain.Violation
		if err == nil {
			rt.Fatalf("expected insufficient_funds moving %d of %d available, got success", amountMinor, fromStartMinor)
		}
		if !errors.As(err, &violation) || violation.Kind() != domain.InsufficientFunds {
			rt.Fatalf("expected insufficient_funds, got %v", err)
		}

		// No expected map: every universe slot defaults to Unchanged().
		statedelta.AssertStateDelta(t, before, after, universe, map[string]statedelta.Predicate{})
	})
}

// TestProperty_PostTransfer_RepeatedApplicationEqualsOneApplication covers
// this step's TEST PARADIGM obligation: for any request and any repeat count
// N >= 1, submitting the SAME idempotency key and fingerprint N times against
// the same fake ports leaves the final state identical to what a single
// application produces. Call 1 is the real write; calls 2..N must all be
// replays — Replayed: true, the same TransactionID, and the universe
// unchanged from the state call 1 left behind.
func TestProperty_PostTransfer_RepeatedApplicationEqualsOneApplication(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		fromStartMinor := rapid.Int64Range(1, 1_000_000).Draw(rt, "fromStart")
		amountMinor := rapid.Int64Range(1, fromStartMinor).Draw(rt, "amount")
		repeatCount := rapid.IntRange(1, 5).Draw(rt, "repeatCount")

		fromBalance, err := domain.NewMoney(fromStartMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		fromAccount, err := domain.NewAccount("from", domain.Wallet, fromBalance)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}
		zero, err := domain.NewMoney(0, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}
		toAccount, err := domain.NewAccount("to", domain.System, zero)
		if err != nil {
			rt.Fatalf("NewAccount rejected a non-negative balance: %v", err)
		}

		store := newFakeStore(fromAccount, toAccount)
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		amount, err := domain.NewMoney(amountMinor, "USD")
		if err != nil {
			rt.Fatalf("NewMoney rejected a known currency: %v", err)
		}

		request := app.TransferRequest{
			From:           "from",
			To:             "to",
			Amount:         amount,
			IdempotencyKey: "idem-key",
			Fingerprint:    "fingerprint",
		}

		first, err := ledger.PostTransfer(context.Background(), request)
		if err != nil {
			rt.Fatalf("unexpected refusal on first application: %v", err)
		}
		if first.Replayed {
			rt.Fatalf("a first-time posting must not report Replayed")
		}

		afterFirst := captureUniverse(store, "from", "to")

		for i := 2; i <= repeatCount; i++ {
			repeat, err := ledger.PostTransfer(context.Background(), request)
			if err != nil {
				rt.Fatalf("unexpected refusal on repeat %d: %v", i, err)
			}
			if !repeat.Replayed {
				rt.Fatalf("repeat %d must report Replayed, got a fresh write", i)
			}
			if repeat.Posting.Transaction.ID() != first.Posting.Transaction.ID() {
				rt.Fatalf("repeat %d TransactionID = %q, want %q (same as first application)",
					i, repeat.Posting.Transaction.ID(), first.Posting.Transaction.ID())
			}

			afterRepeat := captureUniverse(store, "from", "to")
			statedelta.AssertStateDelta(t, afterFirst, afterRepeat, universe, map[string]statedelta.Predicate{})
		}
	})
}

// TestProperty_CreateAccount_OpensAtZeroBalance covers CreateAccount's
// contract: a newly opened account of either kind starts at zero, through the
// real AccountRepository.Create call (here, the fake honouring the same
// duplicate-id refusal the real one does).
func TestProperty_CreateAccount_OpensAtZeroBalance(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		accountID := rapid.StringMatching(`[a-z][a-z0-9_]{2,12}`).Draw(t, "accountID")
		kind := rapid.SampledFrom([]domain.AccountKind{domain.Wallet, domain.System}).Draw(t, "kind")

		store := newFakeStore()
		ledger := app.NewLedger(store, fixedClock, sequentialIDs())

		if err := ledger.CreateAccount(context.Background(), accountID, kind); err != nil {
			t.Fatalf("unexpected error opening account %q: %v", accountID, err)
		}

		account, ok := store.accounts[accountID]
		if !ok {
			t.Fatalf("account %q was not persisted", accountID)
		}
		if account.Kind() != kind {
			t.Fatalf("Kind() = %v, want %v", account.Kind(), kind)
		}
		if account.Balance().MinorUnits() != 0 {
			t.Fatalf("Balance().MinorUnits() = %d, want 0", account.Balance().MinorUnits())
		}
		if !store.committed {
			t.Fatalf("CreateAccount succeeded without committing the unit of work")
		}
	})
}

// TestGetBalance_ReadsStoredBalance covers GetBalance's contract: it reads
// through the repository and performs no I/O of its own beyond that read.
//
// bypass: single example, not a property — GetBalance is a pure pass-through
// read with no side effect to declare a universe over (nw-tdd-methodology
// exempt category "pure-function tests with single output and no side
// effects").
func TestGetBalance_ReadsStoredBalance(t *testing.T) {
	balance, err := domain.NewMoney(500, "USD")
	if err != nil {
		t.Fatalf("NewMoney rejected a known currency: %v", err)
	}
	account, err := domain.NewAccount("wallet-1", domain.Wallet, balance)
	if err != nil {
		t.Fatalf("NewAccount rejected a non-negative balance: %v", err)
	}
	store := newFakeStore(account)
	ledger := app.NewLedger(store, fixedClock, sequentialIDs())

	got, err := ledger.GetBalance(context.Background(), "wallet-1")
	if err != nil {
		t.Fatalf("unexpected error reading balance: %v", err)
	}
	if got.Balance().MinorUnits() != 500 {
		t.Fatalf("Balance().MinorUnits() = %d, want 500", got.Balance().MinorUnits())
	}
}

// TestGetBalance_UnknownAccountIsRefused covers the negative path: reading a
// balance for an account that was never opened is a refusal, not a panic or
// a zero value that could be mistaken for a real one.
func TestGetBalance_UnknownAccountIsRefused(t *testing.T) {
	store := newFakeStore()
	ledger := app.NewLedger(store, fixedClock, sequentialIDs())

	_, err := ledger.GetBalance(context.Background(), "ghost")

	var violation domain.Violation
	if !errors.As(err, &violation) || violation.Kind() != domain.UnknownAccount {
		t.Fatalf("expected account_not_found, got %v", err)
	}
}
